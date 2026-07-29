package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/portpool"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/preview"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// deliverables_test.go — the S15.2 deliverables family (P3-B6-3B, R7–R12 + the
// F slice). The battery composes the REAL S13 stores: a migrated platform.db,
// the real event log, the real review store (so anchors are validated by the
// landed ladder rather than by a fake), the real project topology over
// file:// git fixtures in t.TempDir(), the real effect journal, and the real
// preview manager with routing DISABLED.
//
// $0 AND HERMETIC. Nothing here dials a network, spawns an engine, prices a
// call or touches host state. The one push seam is a fake that RECORDS the
// broker request so the CAS arguments are assertable without a remote, and the
// preview manager is composed exactly as internal/preview's own harness
// composes it — NewCaddyClient("", "") is routing-disabled, which is what keeps
// the live front chain out of this package's reach.

const dlvPIN = "hunter2hunter"

// dlvEnv is the shared part-B fixture.
type dlvEnv struct {
	t       *testing.T
	b       *backend
	ctx     context.Context
	rev     *review.Store
	proj    *project.Store
	journal *gates.Journal
	acc     *accept.Accepter
	prev    *preview.Manager
	push    *fakePusher
	// remote + baseSHA are set by prepareProject.
	remote  string
	baseSHA string
	// portLo is this package's own disjoint portpool range: internal/preview
	// owns 47700-47719 and internal/shell 47800-47819, and all three test
	// packages can run at once against one OS loopback table.
	portLo int
}

// fakePusher records the broker CAS request instead of pushing. Rejected
// drives the S13.6 step-4 lease-rejection path.
type fakePusher struct {
	reqs     []broker.Request
	rejected bool
}

func (p *fakePusher) Push(req broker.Request) (broker.PushResult, error) {
	p.reqs = append(p.reqs, req)
	if p.rejected {
		return broker.PushResult{Rejected: true}, nil
	}
	return broker.PushResult{}, nil
}

func newDlvEnv(t *testing.T) *dlvEnv {
	t.Helper()
	ctx := context.Background()
	b := newBackend(t)
	e := &dlvEnv{t: t, b: b, ctx: ctx, portLo: 47900}

	// The three identities, with real PINs: the operator (bootstrap), and two
	// members whose rows must stay disjoint through every scope assertion.
	if err := b.store.CreateUser(ctx, "", auth.User{ID: "op", DisplayName: "Op", Role: auth.RoleOperator}, dlvPIN); err != nil {
		t.Fatalf("create operator: %v", err)
	}
	// The display names are deliberately DIFFERENT from the ids: the accept
	// authors its squash commit under the person's name, and a fixture where
	// the two coincide could not tell a resolved name from a raw id.
	for id, name := range map[string]string{"alice": "Alice Member", "bob": "Bob Member"} {
		if err := b.store.CreateUser(ctx, "op", auth.User{ID: id, DisplayName: name, Role: auth.RoleMember}, dlvPIN); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}

	e.rev = &review.Store{DB: b.db, Log: b.log, Settings: b.reg, Root: filepath.Join(t.TempDir(), "review")}
	proj, err := project.New(project.Config{DB: b.db, Log: b.log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	e.proj = proj
	journal, err := gates.NewJournal(gates.JournalConfig{DB: b.db, Settings: b.reg})
	if err != nil {
		t.Fatalf("gates.NewJournal: %v", err)
	}
	e.journal = journal
	e.push = &fakePusher{}
	acc, err := accept.New(accept.Config{
		Project: proj, Journal: journal, Push: e.push, Review: e.rev,
		ActiveRuns: func(context.Context, string) ([]run.SiblingAcceptRun, error) {
			return []run.SiblingAcceptRun{{RunID: "r-sibling", CheckpointTime: time.Unix(1, 0)}}, nil
		},
		Freshness: b.reg,
	})
	if err != nil {
		t.Fatalf("accept.New: %v", err)
	}
	e.acc = acc
	return e
}

// withPreview composes the real preview manager. Routing is DISABLED
// (NewCaddyClient("", "")), which is internal/preview's own dev/test default
// and the reason no test in this package can reach the live admin API.
// concurrency drives the ⚙ preview.max_concurrent cap through the narrow
// Settings seam, so the at-capacity state is reachable without 3 launches.
func (e *dlvEnv) withPreview(concurrency int64) *dlvEnv {
	e.t.Helper()
	ports, err := portpool.New(portpool.Config{
		Dir: filepath.Join(e.t.TempDir(), "portpool"), Lo: e.portLo, Hi: e.portLo + 19,
	})
	if err != nil {
		e.t.Fatalf("portpool.New: %v", err)
	}
	m, err := preview.New(preview.Config{
		Reviews:  e.rev,
		Projects: e.proj,
		Ports:    ports,
		Caddy:    preview.NewCaddyClient("", ""), // routing disabled — the host-hazard posture
		Events:   e.b.log,
		Settings: dlvPreviewSettings{cap: concurrency},
		Scratch:  filepath.Join(e.t.TempDir(), "preview-clones"),
	})
	if err != nil {
		e.t.Fatalf("preview.New: %v", err)
	}
	e.prev = m
	e.t.Cleanup(func() {
		for _, s := range m.List() {
			_ = m.Stop(context.Background(), s.ID)
		}
	})
	return e
}

// dlvPreviewSettings is the narrow ⚙ seam the manager reads its two keys
// through. The cap is driven so the at-capacity disposition is a one-launch
// fixture rather than a race against the ratified default.
type dlvPreviewSettings struct{ cap int64 }

func (s dlvPreviewSettings) Int(string) (int64, error)              { return s.cap, nil }
func (s dlvPreviewSettings) Duration(string) (time.Duration, error) { return 15 * time.Minute, nil }

func (e *dlvEnv) server(who string) *api.Server {
	e.t.Helper()
	return api.New(api.Config{
		Log:      e.b.log,
		Sessions: e.b.store,
		Auth:     fixedIdentity{who},
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true, Mode: "running", Version: "test"} },
		DB:       e.b.db,
		Meter:    fakeMeter{},
		Review:   e.rev,
		Accept:   e.acc,
		Preview:  e.prev,
	})
}

// do drives one request as `who` and returns the status and the body.
func (e *dlvEnv) do(t *testing.T, who, method, path, body string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	e.server(who).Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

func (e *dlvEnv) mustDo(t *testing.T, who, method, path, body string) string {
	t.Helper()
	code, out := e.do(t, who, method, path, body)
	if code != http.StatusOK {
		t.Fatalf("%s %s as %s: status %d: %s", method, path, who, code, out)
	}
	return out
}

// ── fixtures ────────────────────────────────────────────────────────────────

// mkRun inserts a task and a run, plus the two provenance rows the accept card
// derives its trailers from: the routing record the stage skeleton emits and
// the engine session's substrate. Both are PLATFORM facts — nothing here is a
// model-authored string.
func (e *dlvEnv) mkRun(taskID, runID, owner string) {
	e.t.Helper()
	e.exec(`INSERT OR IGNORE INTO tasks (task_id, user_id, title, kanban_status, created_ts) VALUES (?, ?, ?, 'new', ?)`,
		taskID, owner, taskID, dlvNow())
	runs := run.NewStore(e.b.db, e.b.log)
	if _, err := runs.Create(e.ctx, run.NewRun{
		ID: runID, UserID: owner, TaskID: taskID, Substrate: "claude-cli", Lane: "anthropic",
	}); err != nil {
		e.t.Fatalf("create run %s: %v", runID, err)
	}
	e.exec(`INSERT INTO engine_sessions (run_id, user_id, substrate, engine_session_id, created_ts, updated_ts)
	        VALUES (?, ?, 'claude-cli', ?, ?, ?)`, runID, owner, runID+"-sess", dlvNow(), dlvNow())
	if _, err := e.b.log.Append(e.ctx, eventlog.Append{
		RunID: runID, UserID: owner, Type: "routing.decided", SchemaVersion: 1,
		Payload: json.RawMessage(`{"model":"opus-4-8","lane":"anthropic","worker":"generalist"}`),
	}); err != nil {
		e.t.Fatalf("append routing.decided: %v", err)
	}
}

// mkDeliverable mints a content-pinned deliverable with the given files. snap
// non-empty fills the S13.5 snapshot-commit pin, which is what makes a revision
// repo-backed (and therefore acceptable).
func (e *dlvEnv) mkDeliverable(id, owner, taskID, runID, projectID, dtype string, files map[string]string, snap string) review.Deliverable {
	e.t.Helper()
	d, err := e.rev.EnsureDeliverable(e.ctx, review.EnsureInput{
		ID: id, Owner: owner, TaskID: taskID, ProjectID: projectID, Type: dtype,
	})
	if err != nil {
		e.t.Fatalf("EnsureDeliverable %s: %v", id, err)
	}
	if _, err := e.rev.MintRevision(e.ctx, review.MintInput{
		DeliverableID: id, N: 1, RunID: runID, AttemptRef: runID + "#round-1",
		Files: files, SnapshotSHA: snap,
	}); err != nil {
		e.t.Fatalf("MintRevision %s: %v", id, err)
	}
	d, err = e.rev.Deliverable(e.ctx, id)
	if err != nil {
		e.t.Fatalf("read deliverable %s: %v", id, err)
	}
	return d
}

// mintNext mints the next revision of an existing deliverable.
func (e *dlvEnv) mintNext(id, runID string, n int, files map[string]string, snap string) review.Revision {
	e.t.Helper()
	r, err := e.rev.MintRevision(e.ctx, review.MintInput{
		DeliverableID: id, N: n, RunID: runID, AttemptRef: fmt.Sprintf("%s#round-%d", runID, n),
		Files: files, SnapshotSHA: snap,
	})
	if err != nil {
		e.t.Fatalf("MintRevision %s/%d: %v", id, n, err)
	}
	return r
}

func (e *dlvEnv) exec(q string, args ...any) {
	e.t.Helper()
	if err := e.b.db.WriteTx(e.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(e.ctx, q, args...)
		return err
	}); err != nil {
		e.t.Fatalf("exec %q: %v", q, err)
	}
}

func (e *dlvEnv) count(typ string) int {
	e.t.Helper()
	var n int
	if err := e.b.db.QueryRowContext(e.ctx, `SELECT COUNT(*) FROM run_events WHERE type = ?`, typ).Scan(&n); err != nil {
		e.t.Fatalf("count %s: %v", typ, err)
	}
	return n
}

func dlvNow() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// ── git fixtures (hermetic, file:// only, §23) ──────────────────────────────

func dlvGitEnv() []string {
	env := []string{
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL=file", "HOME=/nonexistent",
		"GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@test.invalid",
		"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@test.invalid",
	}
	if p := os.Getenv("PATH"); p != "" {
		env = append(env, "PATH="+p)
	}
	return env
}

func (e *dlvEnv) git(args ...string) string {
	e.t.Helper()
	cmd := osexec.Command("git", args...)
	cmd.Env = dlvGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// prepareProject seeds a bare file:// remote, onboards + approves it, and
// returns the project id. Everything lives under t.TempDir().
func (e *dlvEnv) prepareProject(projectID, owner string, files map[string]string) string {
	e.t.Helper()
	src := filepath.Join(e.t.TempDir(), "seed")
	e.git("init", "-q", "-b", "main", src)
	for rel, body := range files {
		p := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			e.t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			e.t.Fatal(err)
		}
	}
	e.git("-C", src, "add", "-A")
	e.git("-C", src, "commit", "-q", "-m", "base")
	bare := filepath.Join(e.t.TempDir(), "remote.git")
	e.git("init", "--bare", "-q", "-b", "main", bare)
	e.git("-C", src, "push", "file://"+bare, "HEAD:refs/heads/main")
	e.remote = bare
	e.baseSHA = e.git("-C", src, "rev-parse", "HEAD")

	_, draft, err := e.proj.Onboard(e.ctx, project.OnboardInput{
		ProjectID: projectID, Owner: owner, Name: projectID, Source: bare, RemoteURL: "file://" + bare,
	})
	if err != nil {
		e.t.Fatalf("Onboard: %v", err)
	}
	// Clear the scan-drafted preview command so the S13.8 heuristic lanes are
	// what the preview battery exercises (the internal/preview harness posture);
	// the registry-command precedence path is that package's own test.
	draft.Commands.Preview = ""
	if _, err := e.proj.Approve(e.ctx, projectID, owner, &draft); err != nil {
		e.t.Fatalf("Approve: %v", err)
	}
	return projectID
}

// snapshotCandidate writes a candidate body into the project's pipeline
// workspace and returns its platform snapshot commit — the S13.5 revision raw
// material the accept pushes.
func (e *dlvEnv) snapshotCandidate(projectID, pipeline, rel, body string) string {
	e.t.Helper()
	ws, err := e.proj.EnsureWorkspace(e.ctx, projectID, pipeline)
	if err != nil {
		e.t.Fatalf("EnsureWorkspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ws.Path, rel), []byte(body), 0o600); err != nil {
		e.t.Fatal(err)
	}
	sha, err := e.proj.Snapshot(e.ctx, ws.Path)
	if err != nil {
		e.t.Fatalf("Snapshot: %v", err)
	}
	return sha
}

// ── R7: list + detail, three ways ───────────────────────────────────────────

func TestDeliverableListIsOwnerScopedThreeWays(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	e.mkRun("t-b", "r-b", "bob")
	e.mkDeliverable("d-alice", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "alice\n"}, "")
	e.mkDeliverable("d-bob", "bob", "t-b", "r-b", "", "markdown", map[string]string{"b.md": "bob\n"}, "")

	ids := func(who string) map[string]bool {
		out := map[string]bool{}
		var list struct {
			Deliverables []struct {
				DeliverableID string `json:"deliverable_id"`
			} `json:"deliverables"`
		}
		if err := json.Unmarshal([]byte(e.mustDo(t, who, "GET", "/api/deliverables", "")), &list); err != nil {
			t.Fatalf("decode list as %s: %v", who, err)
		}
		for _, d := range list.Deliverables {
			out[d.DeliverableID] = true
		}
		return out
	}
	// The non-tautological direction FIRST: a member's own row must be PRESENT,
	// so an all-empty handler cannot pass this test.
	if a := ids("alice"); !a["d-alice"] || a["d-bob"] {
		t.Errorf("alice must see her own deliverable and not bob's, got %v", a)
	}
	if b := ids("bob"); !b["d-bob"] || b["d-alice"] {
		t.Errorf("bob must see his own deliverable and not alice's, got %v", b)
	}
	if o := ids("op"); !o["d-alice"] || !o["d-bob"] {
		t.Errorf("the operator reads across owners, got %v", o)
	}
}

func TestDeliverableDetailIs404BeforeThen403(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-alice", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "alice\n"}, "")

	if code, out := e.do(t, "bob", "GET", "/api/deliverables/d-alice", ""); code != http.StatusForbidden {
		t.Errorf("another member reading alice's deliverable: want 403, got %d: %s", code, out)
	}
	// An id that does not exist must 404 for everybody — an unknown id can never
	// become an existence oracle for another owner's work.
	for _, who := range []string{"alice", "bob", "op"} {
		if code, _ := e.do(t, who, "GET", "/api/deliverables/d-nope", ""); code != http.StatusNotFound {
			t.Errorf("unknown deliverable as %s: want 404, got %d", who, code)
		}
	}
	if code, out := e.do(t, "alice", "GET", "/api/deliverables/d-alice", ""); code != http.StatusOK {
		t.Errorf("the owner's own read must succeed: %d %s", code, out)
	}
}

func TestDeliverableDetailServesImmutableRevisionsWithPins(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "one\n"}, "")
	e.mintNext("d-a", "r-a", 2, map[string]string{"a.md": "two\n"}, "")
	if err := e.rev.SetVerdictRef(e.ctx, "d-a", 2, 4242); err != nil {
		t.Fatalf("SetVerdictRef: %v", err)
	}

	var detail struct {
		Revisions []review.Revision `json:"revisions"`
	}
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "GET", "/api/deliverables/d-a", "")), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if len(detail.Revisions) != 2 {
		t.Fatalf("want the full lineage 1..N, never compressed; got %d revisions", len(detail.Revisions))
	}
	for i, r := range detail.Revisions {
		if r.N != i+1 {
			t.Errorf("revision %d is numbered %d — the lineage is contiguous (S13.1)", i, r.N)
		}
		if r.ContentSHA256 == "" || r.PlatformRef == "" {
			t.Errorf("revision %d serves no content pin / platform ref: %+v", r.N, r)
		}
		if r.RunID != "r-a" || r.AttemptRef == "" {
			t.Errorf("revision %d serves no minting run/attempt: %+v", r.N, r)
		}
	}
	if detail.Revisions[1].VerdictRef != 4242 {
		t.Errorf("the verification verdict ref must ride the revision, got %d", detail.Revisions[1].VerdictRef)
	}
}

// ── R7/OQ3: doors-as-data, in both agreement directions ─────────────────────

func doorByVerb(t *testing.T, body, verb string) api.Door {
	t.Helper()
	var detail struct {
		Doors []api.Door `json:"doors"`
	}
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatalf("decode doors: %v", err)
	}
	for _, d := range detail.Doors {
		if d.Verb == verb {
			return d
		}
	}
	t.Fatalf("no %q door on the detail — a surface must name the live verbs it has (D3); doors were %+v", verb, detail.Doors)
	return api.Door{}
}

func TestDoorsNameTheLiveRoutesForAnInReviewDeliverable(t *testing.T) {
	e := newDlvEnv(t).withPreview(3)
	e.prepareProject("proj", "alice", map[string]string{"a.txt": "base\n"})
	snap := e.snapshotCandidate("proj", "pipe", "a.txt", "candidate\n")
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "proj", "markdown", map[string]string{"a.txt": "candidate\n"}, snap)

	body := e.mustDo(t, "alice", "GET", "/api/deliverables/d-a", "")
	acceptDoor := doorByVerb(t, body, "accept")
	if !acceptDoor.Available || acceptDoor.Route != "/api/deliverables/d-a/accept" {
		t.Errorf("an in-review repo-backed deliverable must offer the accept door: %+v", acceptDoor)
	}
	if acceptDoor.PinFrom == "" {
		t.Error("the accept door must name where its payload_hash comes from (S15.2 retry-safety)")
	}
	// OQ3: in review with no open rework card, the request-revision door is
	// CLOSED and says which door opens when the platform asks.
	revise := doorByVerb(t, body, "request-revision")
	if revise.Available {
		t.Errorf("no rework card is open, so the request-revision door must be closed: %+v", revise)
	}
	if !strings.Contains(revise.Reason, "revise_with_guidance") {
		t.Errorf("the closed door must still name the landed verb it will use: %q", revise.Reason)
	}
	// With no concrete target the door names NO route: the verb it will use
	// answers a card that has not been issued, and the follow-up route belongs
	// to the finished door alone. A route that cannot open from this state is
	// the dead-door shape (D3/D9).
	if revise.Route != "" || revise.Method != "" {
		t.Errorf("a door with no concrete target must name no route, got %q %q", revise.Method, revise.Route)
	}
	for _, verb := range []string{"follow-up", "preview", "preview-compare"} {
		if d := doorByVerb(t, body, verb); !d.Available {
			t.Errorf("the %s door must be live for a minted deliverable: %+v", verb, d)
		}
	}
}

func TestReworkCardOpensTheReviseWithGuidanceDoorWithItsPin(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "one\n"}, "")
	// The landed S07.7 escalation card shape: its `choices` are what make it a
	// rework card, which is the producer's own word for it.
	snapshot := `{"kind":"escalation","category":"CAP-HIT","task_id":"t-a","run_id":"r-a",` +
		`"choices":["accept_best_effort","revise_with_guidance","cancel"]}`
	e.exec(`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts) VALUES (?, ?, ?, ?, 'open', ?)`,
		"ask-1", "r-a", "alice", snapshot, dlvNow())

	revise := doorByVerb(t, e.mustDo(t, "alice", "GET", "/api/deliverables/d-a", ""), "request-revision")
	if !revise.Available || revise.AskID != "ask-1" || revise.Answer != "revise_with_guidance" {
		t.Fatalf("a parked rework card must open the S07.7 answer door: %+v", revise)
	}
	if revise.Route != "/api/approvals/ask:ask-1/answer" {
		t.Errorf("the door must name the LANDED answer route, got %q", revise.Route)
	}
	// The pin must be the ONE canonicalization, so the door hands over a hash
	// the route it names will actually accept.
	want, err := gates.CanonicalHash(json.RawMessage(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	if revise.PayloadHash != want {
		t.Errorf("the door's pin %q is not gates.CanonicalHash of the card %q", revise.PayloadHash, want)
	}
}

// TestVerifyEscalationCardListsItsActions is the D6 regression: the inbox
// derives a verify-escalation card's action vocabulary from the TOP-LEVEL
// `choices` its producer marshals (internal/verify's escalation Card), which is
// the same field the landed answer validator checks a choice against. Reading a
// key nothing writes derived an EMPTY actions list, so the inbox rendered those
// cards with no controls at all.
func TestVerifyEscalationCardListsItsActions(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	// Producer-shaped: exactly what verify/escalate.go marshals into
	// asks.snapshot for a CAP-HIT escalation.
	snapshot := `{"kind":"escalation","category":"CAP-HIT","task_id":"t-a","run_id":"r-a",` +
		`"issued_ts":"` + dlvNow() + `","summary":"the rework cap was reached",` +
		`"choices":["accept_best_effort","revise_with_guidance","cancel"],"ask_id":"ask-esc"}`
	e.exec(`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts) VALUES (?, ?, ?, ?, 'open', ?)`,
		"ask-esc", "r-a", "alice", snapshot, dlvNow())

	list := decodeList(t, e.mustDo(t, "alice", "GET", "/api/approvals", ""))
	item, ok := itemByID(list, "ask:ask-esc")
	if !ok {
		t.Fatalf("the escalation card is not in the inbox: %+v", list)
	}
	if len(item.Actions) == 0 {
		t.Fatal("the card lists NO actions — an inbox card a person cannot act on is the defect this pins")
	}
	got := map[string]bool{}
	for _, a := range item.Actions {
		got[a] = true
	}
	for _, want := range []string{"accept_best_effort", "revise_with_guidance", "cancel"} {
		if !got[want] {
			t.Errorf("the card must list its landed choice %q, got %v", want, item.Actions)
		}
	}
}

func TestFinishedDeliverableRoutesRevisionThroughTheFollowUpPreset(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "one\n"}, "")
	if _, err := e.rev.Accept(e.ctx, "d-a", "alice"); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	body := e.mustDo(t, "alice", "GET", "/api/deliverables/d-a", "")
	revise := doorByVerb(t, body, "request-revision")
	if !revise.Available || revise.Preset != "revision" || revise.Route != "/api/deliverables/d-a/follow-up" {
		t.Fatalf("a finished deliverable revises through the S13.9 follow-up preset: %+v", revise)
	}
	// The other direction of the agreement: the accept door is now CLOSED and
	// says why, rather than going missing.
	acceptDoor := doorByVerb(t, body, "accept")
	if acceptDoor.Available || !strings.Contains(acceptDoor.Reason, "accepted") {
		t.Errorf("an accepted deliverable must show a closed accept door with its reason: %+v", acceptDoor)
	}
}

// ── R8: compare ─────────────────────────────────────────────────────────────

func TestCompareServesTheUnifiedGitDiffAndTheRoundDefault(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "one\n"}, "")
	e.mintNext("d-a", "r-a", 2, map[string]string{"a.md": "two\n"}, "")

	// No params at all: the FC-v1 §2 round-over-round view is the DEFAULT read
	// (new = current, old = new−1).
	var cmp review.Comparison
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "GET", "/api/deliverables/d-a/compare", "")), &cmp); err != nil {
		t.Fatalf("decode comparison: %v", err)
	}
	if cmp.OldN != 1 || cmp.NewN != 2 {
		t.Fatalf("the default read is round-over-round (old=new−1), got old=%d new=%d", cmp.OldN, cmp.NewN)
	}
	if cmp.Surface != review.SurfaceLineDiff {
		t.Fatalf("a markdown deliverable compares on the line-diff surface, got %q", cmp.Surface)
	}
	// The wire product is the host-side UNIFIED GIT DIFF text — react-diff-view
	// parses that natively and highlights CLIENT-side, and no server-side
	// highlighter exists in the platform (FC-v1 §2).
	if !strings.Contains(cmp.Unified, "@@") || !strings.Contains(cmp.Unified, "-one") || !strings.Contains(cmp.Unified, "+two") {
		t.Fatalf("want a unified diff carrying both sides, got %q", cmp.Unified)
	}
	for _, marker := range []string{"<span", "<div", "class=", "&lt;"} {
		if strings.Contains(cmp.Unified, marker) {
			t.Errorf("the diff carries markup %q — the server ships no highlighter and no HTML (FC-v1 §2)", marker)
		}
	}
	// Revision-over-revision navigation is the same read with an explicit pair.
	var base review.Comparison
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "GET", "/api/deliverables/d-a/compare?old=0&new=1", "")), &base); err != nil {
		t.Fatalf("decode base comparison: %v", err)
	}
	if base.OldN != 0 || base.NewN != 1 {
		t.Fatalf("old=0 is the pre-task base, got old=%d new=%d", base.OldN, base.NewN)
	}
	if !strings.Contains(base.Unified, "+one") {
		t.Errorf("the empty-base comparison must still present revision 1's content, got %q", base.Unified)
	}
}

// TestCompareRefusesNoTypeAndLabelsTheFallback walks every S13.2 type row: no
// type is refused, and a type without a rich surface gets the honest fallback
// VISIBLY LABELED.
func TestCompareRefusesNoTypeAndLabelsTheFallback(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	png := map[string][]byte{"shot.png": []byte("\x89PNG\r\n\x1a\nOLD")}
	pngNew := map[string][]byte{"shot.png": []byte("\x89PNG\r\n\x1a\nNEW")}

	cases := []struct {
		dtype       string
		wantSurface string
		wantLabeled bool
	}{
		{"code", review.SurfaceLineDiff, false},
		{"spreadsheet", review.SurfaceExtractedText, true}, // any-other-text-extractable → labeled fallback
	}
	for _, tc := range cases {
		id := "d-" + tc.dtype
		e.mkDeliverable(id, "alice", "t-a", "r-a", "", tc.dtype, map[string]string{"f.txt": "one\n"}, "")
		e.mintNext(id, "r-a", 2, map[string]string{"f.txt": "two\n"}, "")
		var cmp review.Comparison
		if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "GET", "/api/deliverables/"+id+"/compare", "")), &cmp); err != nil {
			t.Fatalf("decode %s comparison: %v", tc.dtype, err)
		}
		if cmp.Surface != tc.wantSurface {
			t.Errorf("type %q: want surface %q, got %q", tc.dtype, tc.wantSurface, cmp.Surface)
		}
		if tc.wantLabeled && (!cmp.Fallback || cmp.Label == "") {
			t.Errorf("type %q: the honest fallback must be VISIBLY labeled, got %+v", tc.dtype, cmp)
		}
	}

	// Image and binary deliverables compare on their object surfaces, with the
	// changed-by-hash verdict — never a refusal.
	for _, tc := range []struct{ dtype, want string }{
		{"image", review.SurfaceImagePair},
		{"binary", review.SurfaceBinaryCards},
		{"widget", review.SurfaceBinaryCards}, // unknown opaque type → the labeled fallback
	} {
		id := "d-obj-" + tc.dtype
		if _, err := e.rev.EnsureDeliverable(e.ctx, review.EnsureInput{
			ID: id, Owner: "alice", TaskID: "t-a", Type: tc.dtype,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.rev.MintRevision(e.ctx, review.MintInput{DeliverableID: id, N: 1, RunID: "r-a", Objects: png}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.rev.MintRevision(e.ctx, review.MintInput{DeliverableID: id, N: 2, RunID: "r-a", Objects: pngNew}); err != nil {
			t.Fatal(err)
		}
		var cmp review.Comparison
		if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "GET", "/api/deliverables/"+id+"/compare", "")), &cmp); err != nil {
			t.Fatalf("decode %s comparison: %v", tc.dtype, err)
		}
		if cmp.Surface != tc.want {
			t.Errorf("type %q: want surface %q, got %q", tc.dtype, tc.want, cmp.Surface)
		}
		if !cmp.Changed || len(cmp.OldObjects) == 0 || len(cmp.NewObjects) == 0 {
			t.Errorf("type %q: want per-side object refs and the by-hash verdict, got %+v", tc.dtype, cmp)
		}
		if tc.dtype == "widget" && (!cmp.Fallback || cmp.Label == "") {
			t.Errorf("an unknown type gets the LABELED fallback, got %+v", cmp)
		}
	}

	// PDF is extracted-text-diff only at v0 (G2 Def.13), and when extraction
	// cannot run it DEGRADES to the metadata cards with the reason on the label
	// — never a refusal. The fixture's bytes are not a real PDF, which is
	// exactly the degrade path a corrupt or unsupported document takes.
	if _, err := e.rev.EnsureDeliverable(e.ctx, review.EnsureInput{
		ID: "d-pdf", Owner: "alice", TaskID: "t-a", Type: "pdf",
	}); err != nil {
		t.Fatal(err)
	}
	// Ordered, not a map range: revisions are 1..N with every hop persisted, so
	// minting them in a random order is a mint of n=2 onto current=0.
	for n, body := range []string{"not-a-pdf-old", "not-a-pdf-new"} {
		if _, err := e.rev.MintRevision(e.ctx, review.MintInput{
			DeliverableID: "d-pdf", N: n + 1, RunID: "r-a",
			Objects: map[string][]byte{"doc.pdf": []byte(body)},
			Types:   map[string]string{"doc.pdf": "application/pdf"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	var pdf review.Comparison
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "GET", "/api/deliverables/d-pdf/compare", "")), &pdf); err != nil {
		t.Fatalf("decode pdf comparison: %v", err)
	}
	if pdf.Surface != review.SurfaceBinaryCards || !pdf.Fallback || pdf.Label == "" {
		t.Errorf("an unextractable PDF must degrade to LABELED metadata cards, got %+v", pdf)
	}
	if !strings.Contains(strings.ToLower(pdf.Label), "extraction") {
		t.Errorf("the label must say WHY the rich surface is unavailable, got %q", pdf.Label)
	}
	if !pdf.Changed || len(pdf.OldObjects) == 0 || len(pdf.NewObjects) == 0 {
		t.Errorf("the degraded PDF surface still carries both sides and the by-hash verdict, got %+v", pdf)
	}
}

func TestCompareIsOwnerScoped(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "one\n"}, "")
	e.mintNext("d-a", "r-a", 2, map[string]string{"a.md": "two\n"}, "")
	if code, _ := e.do(t, "bob", "GET", "/api/deliverables/d-a/compare", ""); code != http.StatusForbidden {
		t.Error("another member must not read alice's diff")
	}
	if code, _ := e.do(t, "op", "GET", "/api/deliverables/d-a/compare", ""); code != http.StatusOK {
		t.Error("the operator reads across owners")
	}
	if code, _ := e.do(t, "alice", "GET", "/api/deliverables/d-a/compare?old=2&new=2", ""); code != http.StatusBadRequest {
		t.Error("comparing a revision with itself must be a 400, not a silent empty diff")
	}
}

// ── R9/R10: comments ────────────────────────────────────────────────────────

func TestCommentCreateValidatesTheAnchorServerSide(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	files := map[string]string{"a.md": "alpha\nbravo\ncharlie\n"}
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "", "markdown", files, "")

	// The claimed line is WRONG (the quote lives on line 2, the claim says 3).
	// The landed ladder decides where it lands; the client's assertion is never
	// placement authority.
	body := `{"body":"this line needs work","severity":"blocker",` +
		`"anchor":{"file_path":"a.md","side":"new","line_no":3,"line_text":"bravo"}}`
	var c review.Comment
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/deliverables/d-a/comments", body)), &c); err != nil {
		t.Fatalf("decode comment: %v", err)
	}
	if c.Owner != "alice" {
		t.Errorf("a comment is attributed to the authenticated identity (15.6), got owner %q", c.Owner)
	}
	if c.BornStatus == review.AnchorExact {
		t.Errorf("a claim at the wrong line must never come back `exact` — got %q", c.BornStatus)
	}
	if c.BornStatus != review.AnchorDrifted && c.BornStatus != review.AnchorFile {
		t.Errorf("want a drifted/file placement from the ladder, got %q", c.BornStatus)
	}
	if c.Status != review.StatusOpen {
		t.Errorf("a fresh comment is open, got %q", c.Status)
	}

	// The severity vocabulary is closed.
	if code, out := e.do(t, "alice", "POST", "/api/deliverables/d-a/comments",
		`{"body":"x","severity":"catastrophic"}`); code != http.StatusBadRequest {
		t.Errorf("an unknown severity must be a 400, got %d: %s", code, out)
	}
	if code, _ := e.do(t, "alice", "POST", "/api/deliverables/d-a/comments", `{"body":"   "}`); code != http.StatusBadRequest {
		t.Error("an empty body must be a 400")
	}
	// Non-tautological control: the honest claim lands exact.
	good := `{"body":"ok","anchor":{"file_path":"a.md","side":"new","line_no":2,"line_text":"bravo"}}`
	var exact review.Comment
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/deliverables/d-a/comments", good)), &exact); err != nil {
		t.Fatal(err)
	}
	if exact.BornStatus != review.AnchorExact {
		t.Errorf("a correct claim must land exact, got %q", exact.BornStatus)
	}
}

func TestPlacedCommentsRecomputeAndKeepOrphansVisible(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "", "markdown",
		map[string]string{"a.md": "alpha\nbravo\ncharlie\n"}, "")
	anchored := `{"body":"about bravo","anchor":{"file_path":"a.md","side":"new","line_no":2,"line_text":"bravo"}}`
	e.mustDo(t, "alice", "POST", "/api/deliverables/d-a/comments", anchored)
	// Revision 2 DELETES the anchored line: the comment must survive as a
	// file-level or orphan placement, keeping its quote and its original
	// revision link. No comment may exist without a render location (P-T12-2).
	e.mintNext("d-a", "r-a", 2, map[string]string{"a.md": "alpha\ncharlie\n"}, "")

	var placed struct {
		RevisionN  int                `json:"revision_n"`
		Comments   []review.Comment   `json:"comments"`
		Placements []review.Placement `json:"placements"`
	}
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "GET", "/api/deliverables/d-a/comments", "")), &placed); err != nil {
		t.Fatalf("decode placed comments: %v", err)
	}
	if placed.RevisionN != 2 {
		t.Fatalf("the default render revision is the current one, got %d", placed.RevisionN)
	}
	if len(placed.Comments) != 1 || len(placed.Placements) != 1 {
		t.Fatalf("the comment must still be listed on revision 2, got %d comments / %d placements",
			len(placed.Comments), len(placed.Placements))
	}
	c, p := placed.Comments[0], placed.Placements[0]
	if c.Anchor.LineText != "bravo" {
		t.Errorf("the kept quote must ride the comment, got %q", c.Anchor.LineText)
	}
	if c.RevisionN != 1 {
		t.Errorf("the original-revision link must ride the comment, got %d", c.RevisionN)
	}
	if p.Status == review.AnchorExact || p.Status == review.AnchorMapped {
		t.Errorf("a deleted line cannot place exactly on revision 2, got %q", p.Status)
	}
	// Placement is recomputed on EVERY render and the stored row must agree —
	// a disagreement is loud inside the store, so a second identical read is
	// itself the agreement assertion.
	second := e.mustDo(t, "alice", "GET", "/api/deliverables/d-a/comments?revision=2", "")
	var again struct {
		Placements []review.Placement `json:"placements"`
	}
	if err := json.Unmarshal([]byte(second), &again); err != nil {
		t.Fatal(err)
	}
	if len(again.Placements) != 1 || again.Placements[0].Status != p.Status {
		t.Errorf("re-render disagreed with the recorded placement: %+v vs %+v", again.Placements, p)
	}
	// Revision 1 still places it exactly — the lineage is not rewritten.
	var atOne struct {
		Placements []review.Placement `json:"placements"`
	}
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "GET", "/api/deliverables/d-a/comments?revision=1", "")), &atOne); err != nil {
		t.Fatal(err)
	}
	if len(atOne.Placements) != 1 || atOne.Placements[0].Status != review.AnchorExact {
		t.Errorf("on its own revision the comment still places exactly, got %+v", atOne.Placements)
	}
}

func TestConsumedCommentsCarryTheBatchStamp(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "alpha\n"}, "")
	e.mustDo(t, "alice", "POST", "/api/deliverables/d-a/comments", `{"body":"fix the ending","severity":"blocker"}`)
	// THE drain is the one consumption path; the transport never stamps.
	if _, err := e.rev.Drain(e.ctx, review.DrainRequest{DeliverableID: "d-a", AttemptRef: "r-a#round-2"}); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	var placed struct {
		Comments []review.Comment `json:"comments"`
	}
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "GET", "/api/deliverables/d-a/comments", "")), &placed); err != nil {
		t.Fatal(err)
	}
	if len(placed.Comments) != 1 {
		t.Fatalf("want the consumed comment still listed, got %d", len(placed.Comments))
	}
	c := placed.Comments[0]
	if c.Status != review.StatusConsumed || c.ConsumedTS == "" || c.ConsumedBy == "" || c.FindingNumber < 1 {
		t.Errorf("a consumed comment must carry the batch stamp, the attempt ref and its finding number: %+v", c)
	}
}

func TestCommentWritesFollowReadVisibility(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "alpha\n"}, "")
	if code, _ := e.do(t, "bob", "POST", "/api/deliverables/d-a/comments", `{"body":"nope"}`); code != http.StatusForbidden {
		t.Error("another member must not comment on alice's deliverable")
	}
	if code, _ := e.do(t, "bob", "POST", "/api/deliverables/d-nope/comments", `{"body":"nope"}`); code != http.StatusNotFound {
		t.Error("an unknown deliverable must 404 before it 403s")
	}
	// The operator may comment, and the comment is attributed to THEM.
	var c review.Comment
	if err := json.Unmarshal([]byte(e.mustDo(t, "op", "POST", "/api/deliverables/d-a/comments", `{"body":"operator note"}`)), &c); err != nil {
		t.Fatal(err)
	}
	if c.Owner != "op" {
		t.Errorf("a comment is attributed to its actor either way, got %q", c.Owner)
	}
}

// ── R11/OQ2: the route-table NEGATIVE ───────────────────────────────────────

func TestCommentsShipNoUpdateAndNoDeleteVerb(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "alpha\n"}, "")
	var created review.Comment
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/deliverables/d-a/comments", `{"body":"first"}`)), &created); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("%d", created.ID)

	// S13.3's lifecycle is open → consumed and nothing else: no edit verb, no
	// delete verb, at any of the shapes one might be reached through.
	for _, probe := range []struct{ method, path string }{
		{"PUT", "/api/deliverables/d-a/comments"},
		{"PATCH", "/api/deliverables/d-a/comments"},
		{"DELETE", "/api/deliverables/d-a/comments"},
		{"PUT", "/api/deliverables/d-a/comments/" + id},
		{"PATCH", "/api/deliverables/d-a/comments/" + id},
		{"DELETE", "/api/deliverables/d-a/comments/" + id},
		{"POST", "/api/deliverables/d-a/comments/" + id},
	} {
		if code, out := e.do(t, "alice", probe.method, probe.path, `{"body":"edited"}`); code == http.StatusOK {
			t.Errorf("%s %s answered 200 — a comment is immutable and never deleted (S13.3; P-T12-2): %s",
				probe.method, probe.path, out)
		}
	}
	// Non-tautological control: the verbs that DO exist still answer.
	if code, _ := e.do(t, "alice", "GET", "/api/deliverables/d-a/comments", ""); code != http.StatusOK {
		t.Error("the read verb must still answer — the negative above must not pass by everything being broken")
	}
	// And the landed trigger still stands underneath the transport: raw SQL
	// cannot rewrite the body either.
	err := e.b.db.WriteTx(e.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(e.ctx, `UPDATE review_comments SET body = 'rewritten' WHERE comment_id = ?`, created.ID)
		return err
	})
	if err == nil {
		t.Error("the 0007 immutability trigger must abort a raw comment rewrite")
	}
	err = e.b.db.WriteTx(e.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(e.ctx, `DELETE FROM review_comments WHERE comment_id = ?`, created.ID)
		return err
	})
	if err == nil {
		t.Error("the 0007 never-deleted trigger must abort a raw comment delete")
	}
}

// ── the F block: not-wired, session-required, versioning ────────────────────

// deliverablesRoutes is this packet's route inventory, used by the walks below.
var deliverablesRoutes = []struct{ method, path, body string }{
	{"GET", "/api/deliverables", ""},
	{"GET", "/api/deliverables/d-x", ""},
	{"GET", "/api/deliverables/d-x/compare", ""},
	{"GET", "/api/deliverables/d-x/comments", ""},
	{"POST", "/api/deliverables/d-x/comments", `{"body":"x"}`},
	{"GET", "/api/deliverables/d-x/accept-card", ""},
	{"POST", "/api/deliverables/d-x/accept", `{"payload_hash":"x","pin":"` + dlvPIN + `"}`},
	{"POST", "/api/deliverables/d-x/preview", `{}`},
	{"POST", "/api/deliverables/d-x/preview/compare", `{}`},
	{"GET", "/api/previews", ""},
	{"POST", "/api/previews/p-x/stop", `{}`},
}

// TestEveryDeliverablesRouteIsRoutedAndNamesItsAuditRow is the route-table
// review: every route in the inventory above is actually reachable (a 405 means
// it was never registered), and every MUTATING one names the landed canonical
// row it produces — there is no mutating verb here whose audit story is
// "nothing". The rows themselves are driven for real by the mutation walk in
// previewapi_test.go; this is the inventory that keeps the walk honest.
func TestEveryDeliverablesRouteIsRoutedAndNamesItsAuditRow(t *testing.T) {
	audits := map[string]string{
		"POST /api/deliverables/{deliverable}/comments":        "review.comment (S13.3, minted inside AddComment's one transaction)",
		"POST /api/deliverables/{deliverable}/accept":          "deliverable.accepted + the class-A effect row (S13.6/S02.7)",
		"POST /api/deliverables/{deliverable}/preview":         "preview.started for a BACKED session; a non-backed disposition started nothing and records nothing (S13.8)",
		"POST /api/deliverables/{deliverable}/preview/compare": "preview.started per backed side (S13.8)",
		"POST /api/previews/{session}/stop":                    "preview.stopped (S13.8)",
	}
	e := newDlvEnv(t).withPreview(3)
	declared := map[string]bool{}
	for _, r := range deliverablesRoutes {
		code, out := e.do(t, "alice", r.method, r.path, r.body)
		if code == http.StatusMethodNotAllowed {
			t.Errorf("%s %s is not routed (405): %s", r.method, r.path, out)
		}
		if r.method == http.MethodGet {
			continue
		}
		// The inventory keys use the pattern form, so a path parameter renamed in
		// api.go shows up here rather than silently passing.
		key := r.method + " " + strings.Replace(
			strings.Replace(r.path, "/d-x", "/{deliverable}", 1), "/p-x", "/{session}", 1)
		declared[key] = true
		if audits[key] == "" {
			t.Errorf("%s has no named audit row — a mutating verb whose audit story is nothing is exactly what §29 forbids", key)
		}
	}
	for key := range audits {
		if !declared[key] {
			t.Errorf("%q is declared with an audit row but is not in the route inventory — the enumeration must stay honest both ways", key)
		}
	}
	// Nothing above was a legitimate act, so the walk co-minted nothing.
	if got := e.count(api.EventDecisionRecorded); got != 0 {
		t.Errorf("the route walk minted %d decision.recorded rows", got)
	}
}

func TestDeliverablesRoutesRefuseWhenNotWired(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "op", auth.RoleOperator)
	srv := api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{"op"}, Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} }, DB: b.db, Meter: fakeMeter{},
		// Review, Accept and Preview deliberately nil.
	})
	for _, r := range deliverablesRoutes {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest(r.method, r.path, strings.NewReader(r.body)))
		if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "not_wired") {
			t.Errorf("%s %s with no surface wired: want 503 not_wired, got %d: %s", r.method, r.path, rr.Code, rr.Body.String())
		}
	}
}

func TestDeliverablesRoutesAreSessionRequiredAndUnversioned(t *testing.T) {
	e := newDlvEnv(t).withPreview(3)
	// No Auth and no DevPosture: every route must fail closed.
	closed := api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store, Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} }, DB: e.b.db, Meter: fakeMeter{},
		Review: e.rev, Accept: e.acc, Preview: e.prev,
	})
	for _, r := range deliverablesRoutes {
		rr := httptest.NewRecorder()
		closed.Handler().ServeHTTP(rr, httptest.NewRequest(r.method, r.path, strings.NewReader(r.body)))
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with no identity: want 401, got %d", r.method, r.path, rr.Code)
		}
	}
	// The API is unversioned at v0: SPA and API ship in one binary (S15.2).
	for _, p := range []string{"/v1/api/deliverables", "/api/v1/deliverables", "/v1/api/previews"} {
		if code, _ := e.do(t, "op", "GET", p, ""); code != http.StatusNotFound {
			t.Errorf("%s: a version prefix must not exist, got %d", p, code)
		}
	}
}

// TestDeliverablesShapesNeverPercent extends the §30 never-percent negative over every
// shape this packet adds. Progress is a story, not a number.
func TestDeliverablesShapesNeverPercent(t *testing.T) {
	e := newDlvEnv(t).withPreview(3)
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "", "markdown", map[string]string{"a.md": "alpha\n"}, "")
	e.mintNext("d-a", "r-a", 2, map[string]string{"a.md": "beta\n"}, "")
	e.mustDo(t, "alice", "POST", "/api/deliverables/d-a/comments", `{"body":"note"}`)

	keys := map[string]bool{}
	for _, path := range []string{
		"/api/deliverables", "/api/deliverables/d-a", "/api/deliverables/d-a/compare",
		"/api/deliverables/d-a/comments", "/api/deliverables/d-a/accept-card", "/api/previews",
	} {
		var v any
		if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "GET", path, "")), &v); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		collectKeys(v, keys)
	}
	var launched any
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/deliverables/d-a/preview", `{}`)), &launched); err != nil {
		t.Fatal(err)
	}
	collectKeys(launched, keys)
	if len(keys) == 0 {
		t.Fatal("collected no keys — the scan proves nothing")
	}
	forbidden := map[string]bool{
		"percent": true, "percentage": true, "percent_complete": true,
		"fraction": true, "complete_fraction": true, "ratio": true,
		"progress": true, "pct": true, "complete": true, "completion": true,
		"eta": true, "eta_s": true, "eta_seconds": true,
	}
	if !forbidden["percent"] {
		t.Fatal("the scan cannot detect its own probe")
	}
	for k := range keys {
		lk := strings.ToLower(k)
		if forbidden[lk] || strings.Contains(lk, "percent") {
			t.Errorf("key %q is a completion estimate — no percent, fraction, ratio or ETA field exists here (§30 R12)", k)
		}
		for _, shape := range []string{"_pct", "_ratio", "_fraction", "utiliz"} {
			if strings.Contains(lk, shape) {
				t.Errorf("key %q is ratio-shaped and undeclared", k)
			}
		}
	}
}

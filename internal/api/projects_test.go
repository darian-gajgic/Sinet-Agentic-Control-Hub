package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
)

// projects_test.go — P3-RW-2 acceptance tests for the S13.7/S15.2 projects
// HTTP family, COMMITTED RED at grounding (CONVENTIONS §3 Amendment-A: the
// routes do not exist yet — every test here fails on the mux's own 404 — and
// the packet's implementation commit closes the window).
//
// WHAT BINDS AND WHAT DOES NOT: the ASSERTIONS bind — they are derived from
// Spec S13.7 (registry content + pending→active lifecycle + owner/member
// visibility), S15.2 (server-side authority; the browser is a display) and the
// landed PinForIntake refusal discipline (an unknown id and an invisible entry
// are ONE answer). The response field NAMES are deliberately not pinned — the
// checks are substring-level so the executor owns the wire shapes within the
// brief's proposed routes (P3/briefs/P3-RW-2.md §3). The env builder below is
// a SEAM: the executor extends `server()` with whatever Config field the
// onboard door needs (the IntakeSurface wiring precedent) and may compose the
// real stage skeleton for the POST test — the observable effects asserted
// against the store/DB are the door's contract and may not be faked.

// projEnv is the projects-family fixture: a migrated backend, the four
// identities the visibility rules need (operator, owner, invited member,
// member of nothing), and the real S13.7 registry store.
type projEnv struct {
	t    *testing.T
	b    *backend
	ctx  context.Context
	proj *project.Store
	// onboard is the create door's seam, composed the shell's way (see
	// onboardSeam). It is the executor's one wiring seam (brief §8 item 3).
	onboard *onboardSeam
}

func newProjEnv(t *testing.T) *projEnv {
	t.Helper()
	ctx := context.Background()
	b := newBackend(t)
	if err := b.store.CreateUser(ctx, "", auth.User{ID: "op", DisplayName: "Op", Role: auth.RoleOperator}, dlvPIN); err != nil {
		t.Fatalf("create operator: %v", err)
	}
	for _, id := range []string{"alice", "bob", "carol"} {
		if err := b.store.CreateUser(ctx, "op", auth.User{ID: id, DisplayName: id, Role: auth.RoleMember}, dlvPIN); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	proj, err := project.New(project.Config{DB: b.db, Log: b.log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	return &projEnv{t: t, b: b, ctx: ctx, proj: proj, onboard: &onboardSeam{t: t, b: b, proj: proj}}
}

// server builds the per-caller server. EXECUTOR SEAM: wire the onboard-door
// surface (and anything else the family's Config needs) HERE, in one place.
//
// `who` == auth.DevUserID resolves the §7 dev-posture fallback identity, which
// the create door refuses (brief OQ8) and the reads admit.
func (e *projEnv) server(who string) *api.Server {
	e.t.Helper()
	var caller api.Authenticator = fixedIdentity{who}
	if who == auth.DevUserID {
		caller = devIdentity{}
	}
	return api.New(api.Config{
		Log:      e.b.log,
		Sessions: e.b.store,
		Auth:     caller,
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true, Mode: "running", Version: "test"} },
		DB:       e.b.db,
		Meter:    fakeMeter{},
		// The S13.7 onboarding door and the landed ask-answer path, both served
		// by the one seam below — one composition, two surfaces, exactly as the
		// shell composes *stage.Surface for IntakeSurface/CancelSurface/…
		Onboard: e.onboard,
		Intake:  e.onboard,
	})
}

// onboardSeam is the create door's seam, composed the way the composition root
// composes it: a closure over the REAL project.Store (register → init → scan →
// draft, with real git) plus the durable task/run rows the onboarding task IS.
// NOTHING at the data layer is faked — brief §8 item 3 forbids that, and every
// assertion below reads the real store and the real DB.
//
// What it stands in for is internal/stage's run-substrate half: an api test
// cannot compose a stage.Skeleton (stage.New wants the whole runtime — ledger,
// adapters, checkpoints, a bound scheduler), so the two lines of it this family
// needs — the deterministic `onboard-<id>` / `onboard:<id>` names and the task
// row StartOnboarding writes before enqueueing — are mirrored here, with the
// sentinel translation the shell does (project_seams.go:88 pinRefusal).
type onboardSeam struct {
	t    *testing.T
	b    *backend
	proj *project.Store
	// starts counts the calls that REACHED the store, so the R7 no-double-fire
	// invariant is observable at the seam and not only in its effects.
	starts int
}

var _ api.OnboardSurface = (*onboardSeam)(nil)
var _ api.IntakeSurface = (*onboardSeam)(nil)

func (o *onboardSeam) OnboardRefs(projectID string) api.OnboardRefs {
	// internal/stage's own deterministic names (stage/onboard.go:36–39).
	return api.OnboardRefs{TaskID: "onboard-" + projectID, AskRef: "onboard:" + projectID}
}

func (o *onboardSeam) StartOnboarding(ctx context.Context, owner, projectID, name, remoteURL string) (api.OnboardRefs, error) {
	o.starts++
	if _, err := o.proj.OnboardStart(ctx, project.OnboardInput{
		ProjectID: projectID, Owner: owner, Name: name, RemoteURL: remoteURL,
	}); err != nil {
		return api.OnboardRefs{}, onboardSeamErr(err)
	}
	refs := o.OnboardRefs(projectID)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	exec(o.t, o.b, `INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES (?, ?, ?, ?)
	                ON CONFLICT (task_id) DO NOTHING`, refs.TaskID, owner, "Onboard "+name, now)
	exec(o.t, o.b, `INSERT INTO runs (run_id, user_id, task_id, state, lane, generation, created_ts, updated_ts)
	                VALUES (?, ?, ?, 'queued', 'test', 0, ?, ?) ON CONFLICT (run_id) DO NOTHING`,
		refs.TaskID+".onboard", owner, refs.TaskID, now, now)
	return refs, nil
}

// onboardSeamErr is the shell's sentinel translation (project_seams.go): the
// project store's refusals cross the wall as TYPED transport errors, never as
// text this or any other layer matches on (§38).
func onboardSeamErr(err error) error {
	switch {
	case errors.Is(err, project.ErrAlreadyRegistered):
		return &api.SurfaceError{Status: http.StatusConflict, Code: "already_registered", Msg: err.Error()}
	case errors.Is(err, project.ErrBadInput):
		return &api.SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: err.Error()}
	default:
		return err
	}
}

// Answer is the LANDED activation door's other half: Surface.Answer routes an
// `onboard:` ask to AnswerOnboarding (stage/surface.go:145–154), which is D10 —
// the ask's owner answers, a non-owner is refused 403 — and activates the entry
// through OnboardApprove. The activation itself is the real project store's.
func (o *onboardSeam) Answer(ctx context.Context, userID, askID string, _ json.RawMessage, _ bool) (json.RawMessage, error) {
	projectID, ok := strings.CutPrefix(askID, "onboard:")
	if !ok {
		return nil, &api.SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: "unknown ask"}
	}
	var owner, status string
	if err := o.b.db.QueryRowContext(ctx,
		`SELECT user_id, status FROM asks WHERE ask_id = ?`, askID).Scan(&owner, &status); err != nil {
		return nil, &api.SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: "unknown ask"}
	}
	if userID != owner {
		return nil, &api.SurfaceError{Status: http.StatusForbidden, Code: "not_owner",
			Msg: "only the project owner may approve onboarding (D10)"}
	}
	if err := o.proj.OnboardApprove(ctx, projectID, userID, nil); err != nil {
		return nil, err
	}
	exec(o.t, o.b, `UPDATE asks SET status = 'answered', answered_ts = ? WHERE ask_id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), askID)
	return json.RawMessage(`{"task_id":"` + o.OnboardRefs(projectID).TaskID + `"}`), nil
}

// The rest of api.IntakeSurface is not what this fixture is about: the projects
// family routes none of it, and a fixture that quietly answered would hide a
// route reaching a path it has no business on.
func (o *onboardSeam) notThisFixture() error {
	return &api.SurfaceError{Status: http.StatusNotImplemented, Code: "not_here",
		Msg: "the projects fixture serves the onboarding door only"}
}

func (o *onboardSeam) Submit(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, o.notThisFixture()
}
func (o *onboardSeam) Task(context.Context, string) (json.RawMessage, error) {
	return nil, o.notThisFixture()
}
func (o *onboardSeam) Artifacts(context.Context, string) (json.RawMessage, error) {
	return nil, o.notThisFixture()
}
func (o *onboardSeam) Receipt(context.Context, string) (json.RawMessage, error) {
	return nil, o.notThisFixture()
}
func (o *onboardSeam) Advance(context.Context, string, string) (json.RawMessage, error) {
	return nil, o.notThisFixture()
}

func (e *projEnv) do(t *testing.T, who, method, path, body string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	e.server(who).Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

// seedActive registers, captures and activates one entry at the data layer
// (the pin_test.go precedent — Register → Capture → Activate needs no git and
// takes MEMBERS, which the git-backed Onboard path does not).
func (e *projEnv) seedActive(t *testing.T, id, owner string, members []string, in project.CaptureInput) {
	t.Helper()
	if _, err := e.proj.Register(e.ctx, project.RegisterInput{
		ProjectID: id, Owner: owner, Name: id,
		StorePath: filepath.Join(e.t.TempDir(), id), Members: members,
	}); err != nil {
		t.Fatalf("Register %s: %v", id, err)
	}
	in.ProjectID, in.By = id, owner
	if _, err := e.proj.Capture(e.ctx, in); err != nil {
		t.Fatalf("Capture %s: %v", id, err)
	}
	if _, err := e.proj.Activate(e.ctx, id, owner); err != nil {
		t.Fatalf("Activate %s: %v", id, err)
	}
}

// TestProjectsListIsCallerScoped — Spec S13.7 ("owning user, invited members")
// + S15.2 (visibility enforced server-side): GET /api/projects answers each
// caller with the entries they own or belong to, and NEVER with anybody
// else's. Both directions are non-tautological: the caller's own entry must be
// PRESENT (an empty answer cannot pass for a scoped one, §38).
func TestProjectsListIsCallerScoped(t *testing.T) {
	e := newProjEnv(t)
	e.seedActive(t, "p-alpha", "alice", []string{"bob"}, project.CaptureInput{
		Conventions: []string{"p-alpha-convention: tabs, never spaces"},
		Commands:    project.Commands{Test: "go test ./..."},
		DangerZones: []project.DangerZone{{Path: "deploy/", Rule: "never touch deploy"}},
	})
	e.seedActive(t, "p-gamma", "carol", nil, project.CaptureInput{
		Conventions: []string{"p-gamma-convention"},
	})

	for _, tc := range []struct {
		who       string
		sees      string
		neverSees string
	}{
		{"alice", "p-alpha", "p-gamma"}, // owner
		{"bob", "p-alpha", "p-gamma"},   // invited member
		{"carol", "p-gamma", "p-alpha"}, // owner of the other; member of nothing here
	} {
		code, out := e.do(t, tc.who, "GET", "/api/projects", "")
		if code != http.StatusOK {
			t.Fatalf("GET /api/projects as %s: status %d (want 200): %s", tc.who, code, out)
		}
		if !strings.Contains(out, tc.sees) {
			t.Errorf("as %s the list omits %s — the caller's own entry must be present: %s", tc.who, tc.sees, out)
		}
		if strings.Contains(out, tc.neverSees) {
			t.Errorf("as %s the list leaks %s — visibility is owner-or-member, server-side (S15.2): %s", tc.who, tc.neverSees, out)
		}
	}
}

// TestProjectDetailServesCapturedContentAndRefusesWithOneAnswer — Spec S13.7:
// the entry's captured conventions, commands and danger zones are the detail a
// card renders (product map §3 Projects); the OWNER sees the capture. The
// refusal discipline is the landed PinForIntake / §38 shape: an id the caller
// cannot see answers exactly like an id that does not exist (404, no
// existence oracle), and no captured content rides a refusal.
func TestProjectDetailServesCapturedContentAndRefusesWithOneAnswer(t *testing.T) {
	e := newProjEnv(t)
	e.seedActive(t, "p-alpha", "alice", []string{"bob"}, project.CaptureInput{
		Conventions: []string{"p-alpha-convention: tabs, never spaces"},
		Commands:    project.Commands{Test: "go test ./..."},
		DangerZones: []project.DangerZone{{Path: "deploy/", Rule: "never touch deploy"}},
	})

	// The owner reads the full S13.7 capture. (Member DEPTH is brief OQ4; the
	// member's 200 on the ENTRY is asserted, its capture depth is not.)
	code, out := e.do(t, "alice", "GET", "/api/projects/p-alpha", "")
	if code != http.StatusOK {
		t.Fatalf("owner detail: status %d (want 200): %s", code, out)
	}
	for _, want := range []string{"p-alpha-convention", "go test ./...", "deploy/"} {
		if !strings.Contains(out, want) {
			t.Errorf("owner detail omits %q — the S13.7 capture is the card's content: %s", want, out)
		}
	}
	if code, out := e.do(t, "bob", "GET", "/api/projects/p-alpha", ""); code != http.StatusOK || !strings.Contains(out, "p-alpha") {
		t.Errorf("invited member must read the entry (S13.7 members): status %d: %s", code, out)
	}

	// One refusal for stranger and for unknown — told apart by nothing.
	strangerCode, strangerOut := e.do(t, "carol", "GET", "/api/projects/p-alpha", "")
	unknownCode, _ := e.do(t, "carol", "GET", "/api/projects/p-nosuch", "")
	if strangerCode != http.StatusNotFound || unknownCode != http.StatusNotFound {
		t.Errorf("stranger=%d unknown=%d — both must be 404: an id that answers differently is an existence oracle (S15.2)", strangerCode, unknownCode)
	}
	if strings.Contains(strangerOut, "p-alpha-convention") {
		t.Errorf("a refusal carries captured content: %s", strangerOut)
	}
}

// TestOnboardStartDoorDraftsAPendingEntryWithItsOnboardingTask — Spec S13.7:
// "onboarding a repository is itself a task the platform performs" — the door
// runs register → clone/init → scan → draft over the EXISTING StartOnboarding
// seam and answers with the drafted entry and its approval reference. The
// observable contract asserted here is the door's real effect: a PENDING
// registry entry with its first capture, and the onboarding task attributed to
// the CALLER (identity from the session, never from the body — 15.6).
func TestOnboardStartDoorDraftsAPendingEntryWithItsOnboardingTask(t *testing.T) {
	e := newProjEnv(t)
	code, out := e.do(t, "alice", "POST", "/api/projects", `{"project_id":"p-new","name":"New Proj"}`)
	if code != http.StatusOK {
		t.Fatalf("POST /api/projects: status %d (want 200): %s", code, out)
	}
	if !strings.Contains(out, "p-new") {
		t.Errorf("the answer does not name the drafted entry: %s", out)
	}
	entry, err := e.proj.Get(e.ctx, "p-new")
	if err != nil {
		t.Fatalf("the door answered 200 but registered nothing: %v", err)
	}
	if entry.State != project.StatePending {
		t.Errorf("state %q — the entry activates only on the owner's D10 approval (S13.7)", entry.State)
	}
	if entry.CaptureVersion < 1 {
		t.Errorf("capture_version %d — the scan's draft is the entry's first capture", entry.CaptureVersion)
	}
	if entry.Owner != "alice" {
		t.Errorf("owner %q — the caller is the owner, from the session (15.6)", entry.Owner)
	}
	var taskOwner string
	if err := e.b.db.QueryRowContext(e.ctx,
		`SELECT user_id FROM tasks WHERE task_id = ?`, "onboard-p-new").Scan(&taskOwner); err != nil {
		t.Fatalf("no onboarding task row — onboarding is a task the platform performs (S13.7): %v", err)
	}
	if taskOwner != "alice" {
		t.Errorf("onboarding task owner %q, want the caller", taskOwner)
	}
}

// ── executor-written acceptance tests (brief §8 items 4–11) ─────────────────

// seedPending registers and captures an entry WITHOUT activating it: the
// in-flight onboarding state. pending→active is one-way and the owner's D10
// approval is the only thing that crosses it (Spec S13.7).
func (e *projEnv) seedPending(t *testing.T, id, owner string, members []string, in project.CaptureInput) {
	t.Helper()
	if _, err := e.proj.Register(e.ctx, project.RegisterInput{
		ProjectID: id, Owner: owner, Name: id,
		StorePath: filepath.Join(e.t.TempDir(), id), Members: members,
	}); err != nil {
		t.Fatalf("Register %s: %v", id, err)
	}
	in.ProjectID, in.By = id, owner
	if _, err := e.proj.Capture(e.ctx, in); err != nil {
		t.Fatalf("Capture %s: %v", id, err)
	}
}

// count is a one-value probe over the platform DB.
func (e *projEnv) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.b.db.QueryRowContext(e.ctx, q, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", q, err)
	}
	return n
}

// errorCode reads the machine code off a *SurfaceError answer.
func errorCode(t *testing.T, body string) string {
	t.Helper()
	var out struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode error body %q: %v", body, err)
	}
	return out.Error
}

// TestPendingEntriesInTheOwnersOwnList — brief OQ2(a): an entry whose
// onboarding is in flight appears in the lists of the people it belongs to,
// carrying `pending`, because the Projects tab is where the onboarding was
// started and an invisible in-flight project is a worse surface than an honest
// one (Spec S13.7 lifecycle). The other direction is absolute: ANOTHER
// person's pending entry is invisible, exactly like their active one.
func TestPendingEntriesInTheOwnersOwnList(t *testing.T) {
	e := newProjEnv(t)
	e.seedPending(t, "p-draft", "alice", []string{"bob"}, project.CaptureInput{
		Conventions: []string{"p-draft-convention"},
	})
	e.seedActive(t, "p-gamma", "carol", nil, project.CaptureInput{})

	for _, who := range []string{"alice", "bob"} {
		code, out := e.do(t, who, "GET", "/api/projects", "")
		if code != http.StatusOK {
			t.Fatalf("list as %s: %d: %s", who, code, out)
		}
		if !strings.Contains(out, "p-draft") {
			t.Errorf("as %s the in-flight entry is missing from the list (OQ2): %s", who, out)
		}
		if !strings.Contains(out, "pending") {
			t.Errorf("as %s the list does not say the entry is pending (S13.7 lifecycle): %s", who, out)
		}
		if code, out := e.do(t, who, "GET", "/api/projects/p-draft", ""); code != http.StatusOK {
			t.Errorf("as %s the pending detail must read: %d %s", who, code, out)
		}
	}
	// The other direction, both routes.
	if _, out := e.do(t, "carol", "GET", "/api/projects", ""); strings.Contains(out, "p-draft") {
		t.Errorf("another person's PENDING entry leaked into carol's list: %s", out)
	}
	if code, _ := e.do(t, "carol", "GET", "/api/projects/p-draft", ""); code != http.StatusNotFound {
		t.Errorf("a stranger's read of a pending entry must 404 like any other: %d", code)
	}
}

// TestProjectsOperatorDirection — brief OQ3(a): this family has NO operator
// limb. A registry capture is project CONTENT (the §40-C/§44 content line, the
// same one that keeps memory and chat owner-scoped), and the operator's
// oversight rides the telemetry surfaces, not this door. Both directions are
// pinned: the operator's own entry is present (so an empty answer cannot pass
// for a scoped one), and another person's entry is absent and 404s exactly like
// an id that does not exist.
func TestProjectsOperatorDirection(t *testing.T) {
	e := newProjEnv(t)
	e.seedActive(t, "p-alpha", "alice", []string{"bob"}, project.CaptureInput{
		Conventions: []string{"p-alpha-convention"},
	})
	e.seedActive(t, "p-ops", "op", nil, project.CaptureInput{Conventions: []string{"p-ops-convention"}})

	code, out := e.do(t, "op", "GET", "/api/projects", "")
	if code != http.StatusOK {
		t.Fatalf("operator list: %d: %s", code, out)
	}
	if !strings.Contains(out, "p-ops") {
		t.Errorf("the operator's OWN entry is missing — the absence below would prove nothing: %s", out)
	}
	if strings.Contains(out, "p-alpha") {
		t.Errorf("the operator read another person's project content (OQ3): %s", out)
	}
	strangerCode, strangerOut := e.do(t, "op", "GET", "/api/projects/p-alpha", "")
	unknownCode, unknownOut := e.do(t, "op", "GET", "/api/projects/p-nosuch", "")
	if strangerCode != http.StatusNotFound || unknownCode != http.StatusNotFound {
		t.Errorf("operator on another's entry=%d, on an unknown id=%d — both must be 404", strangerCode, unknownCode)
	}
	if strangerOut != unknownOut {
		t.Errorf("the two refusals differ, which makes the door an existence oracle:\n %s\n %s", strangerOut, unknownOut)
	}
	// The control: the owner still reads their own.
	if code, _ := e.do(t, "alice", "GET", "/api/projects/p-alpha", ""); code != http.StatusOK {
		t.Errorf("the owner's own detail must read: %d", code)
	}
}

// TestOnboardStartRepeatDoesNotDoubleFire — R7 + brief OQ7(a): the phone-retry
// reading of S15.2's retry-safe writes. A repeated POST for an in-flight
// onboarding answers with the entry that already exists and its references —
// it never re-clones, never files a second task or run, and never touches the
// drafted capture. Once the entry is ACTIVE the same POST is a 409: the id is
// taken and the caller resolves it by reading.
func TestOnboardStartRepeatDoesNotDoubleFire(t *testing.T) {
	e := newProjEnv(t)
	if code, out := e.do(t, "alice", "POST", "/api/projects", `{"project_id":"p-new","name":"New Proj"}`); code != http.StatusOK {
		t.Fatalf("first POST: %d %s", code, out)
	}
	before, err := e.proj.Get(e.ctx, "p-new")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	code, out := e.do(t, "alice", "POST", "/api/projects", `{"project_id":"p-new","name":"New Proj"}`)
	if code != http.StatusOK {
		t.Fatalf("the repeat must answer with the in-flight state (OQ7): %d %s", code, out)
	}
	for _, want := range []string{"p-new", "onboard-p-new", "onboard:p-new"} {
		if !strings.Contains(out, want) {
			t.Errorf("the repeat's answer omits %q — it carries the existing entry and its refs: %s", want, out)
		}
	}
	if e.onboard.starts != 1 {
		t.Errorf("the onboarding seam was entered %d times — a retry must never re-run register/clone/scan", e.onboard.starts)
	}
	after, err := e.proj.Get(e.ctx, "p-new")
	if err != nil {
		t.Fatalf("read back after the repeat: %v", err)
	}
	if after.CaptureVersion != before.CaptureVersion || after.UpdatedTS != before.UpdatedTS {
		t.Errorf("the repeat moved the entry: v%d@%s → v%d@%s",
			before.CaptureVersion, before.UpdatedTS, after.CaptureVersion, after.UpdatedTS)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM repo_registry_captures WHERE project_id = ?`, "p-new"); n != 1 {
		t.Errorf("%d captures — a retry re-scanned the store", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM tasks WHERE task_id = ?`, "onboard-p-new"); n != 1 {
		t.Errorf("%d onboarding tasks", n)
	}
	if n := e.count(t, `SELECT COUNT(*) FROM runs WHERE task_id = ?`, "onboard-p-new"); n != 1 {
		t.Errorf("%d onboarding runs", n)
	}

	// Once ACTIVE the same request is a conflict, not a second onboarding.
	if _, err := e.proj.Activate(e.ctx, "p-new", "alice"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	code, out = e.do(t, "alice", "POST", "/api/projects", `{"project_id":"p-new","name":"New Proj"}`)
	if code != http.StatusConflict || errorCode(t, out) != "already_registered" {
		t.Errorf("POST over an ACTIVE entry: want 409 already_registered, got %d %s", code, out)
	}
	if e.onboard.starts != 1 {
		t.Errorf("the refused POST still entered the onboarding seam (%d)", e.onboard.starts)
	}
}

// TestOnboardStartSourceSemantics — brief OQ5(b)+(c): `source` must be EMPTY.
// Over HTTP a source path is a host-filesystem-read primitive — any path the
// platform user can read would become cloneable into a project store and then
// readable back through this family's own read door — so it is refused outright
// and every onboarding initializes a FRESH store (S13.7: "a v0 project without
// a repo gets one created/registered by the onboarding task"). `remote_url` is
// accepted and STORED as data, never dialed (S13.7; §23 never-dials).
func TestOnboardStartSourceSemantics(t *testing.T) {
	e := newProjEnv(t)
	code, out := e.do(t, "alice", "POST", "/api/projects",
		`{"project_id":"p-src","name":"Src","source":"`+t.TempDir()+`"}`)
	if code != http.StatusBadRequest || errorCode(t, out) != "bad_source" {
		t.Fatalf("a source path must be refused 400 bad_source: %d %s", code, out)
	}
	if !strings.Contains(out, "empty") {
		t.Errorf("the refusal does not name the constraint it enforces: %s", out)
	}
	if _, err := e.proj.Get(e.ctx, "p-src"); !errors.Is(err, project.ErrNotFound) {
		t.Errorf("the refused request registered something: %v", err)
	}
	if e.onboard.starts != 0 {
		t.Errorf("the refused request reached the onboarding seam (%d)", e.onboard.starts)
	}

	// The accepted form: no source, a stored remote.
	const remote = "https://example.invalid/thing.git"
	code, out = e.do(t, "alice", "POST", "/api/projects",
		`{"project_id":"p-fresh","name":"Fresh","remote_url":"`+remote+`"}`)
	if code != http.StatusOK {
		t.Fatalf("the accepted form: %d %s", code, out)
	}
	entry, err := e.proj.Get(e.ctx, "p-fresh")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if entry.RemoteURL != remote {
		t.Errorf("remote_url %q — it is accepted and STORED as data (OQ5c)", entry.RemoteURL)
	}
	if _, err := os.Stat(filepath.Join(entry.StorePath, ".git")); err != nil {
		t.Errorf("no fresh store was initialized at %s: %v", entry.StorePath, err)
	}
	// Presence is served; the URL itself is not, because a remote URL can carry
	// an embedded credential and presence is what the card needs (R4).
	_, detail := e.do(t, "alice", "GET", "/api/projects/p-fresh", "")
	if !strings.Contains(detail, "has_remote") {
		t.Errorf("the detail does not carry the remote's presence: %s", detail)
	}
	if strings.Contains(detail, "example.invalid") {
		t.Errorf("the detail serves the remote URL itself: %s", detail)
	}
}

// TestOnboardStartRefusesWhatItMust — brief OQ6/OQ8: the caller-fault limbs.
// A missing field is a 400 that NAMES the field; a taken id is a 409 that reads
// the same whoever owns the entry (an ownership oracle would be the same leak
// the read door refuses); the dev-posture identity is refused, because 15.6
// attributes a project to a person and `dev` is not one.
func TestOnboardStartRefusesWhatItMust(t *testing.T) {
	e := newProjEnv(t)
	e.seedActive(t, "p-mine", "alice", nil, project.CaptureInput{})
	e.seedActive(t, "p-theirs", "carol", nil, project.CaptureInput{})

	for _, tc := range []struct{ body, field string }{
		{`{"name":"No Id"}`, "project_id"},
		{`{"project_id":"p-noname"}`, "name"},
	} {
		code, out := e.do(t, "alice", "POST", "/api/projects", tc.body)
		if code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d %s", tc.body, code, out)
		}
		if !strings.Contains(out, tc.field) {
			t.Errorf("the refusal of %s does not name %q: %s", tc.body, tc.field, out)
		}
	}

	mineCode, mineOut := e.do(t, "alice", "POST", "/api/projects", `{"project_id":"p-mine","name":"Mine"}`)
	theirsCode, theirsOut := e.do(t, "alice", "POST", "/api/projects", `{"project_id":"p-theirs","name":"Theirs"}`)
	if mineCode != http.StatusConflict || theirsCode != http.StatusConflict {
		t.Errorf("a taken id must be 409 either way: own=%d other=%d", mineCode, theirsCode)
	}
	if errorCode(t, mineOut) != "already_registered" || errorCode(t, theirsOut) != errorCode(t, mineOut) {
		t.Errorf("the two conflicts must read alike — an id's OWNER is not this door's to disclose:\n %s\n %s", mineOut, theirsOut)
	}
	if before, err := e.proj.Get(e.ctx, "p-theirs"); err != nil || before.Owner != "carol" || before.CaptureVersion != 1 {
		t.Errorf("the refused POST disturbed another person's entry: %+v %v", before, err)
	}

	code, out := e.do(t, auth.DevUserID, "POST", "/api/projects", `{"project_id":"p-dev","name":"Dev"}`)
	if code != http.StatusForbidden {
		t.Fatalf("the dev identity must be refused at the create door (OQ8): %d %s", code, out)
	}
	if !strings.Contains(strings.ToLower(out), "sign in") {
		t.Errorf("the dev refusal does not say what to do about it: %s", out)
	}
	if _, err := e.proj.Get(e.ctx, "p-dev"); !errors.Is(err, project.ErrNotFound) {
		t.Errorf("the dev POST registered something: %v", err)
	}
	// Reads stay dev-accessible (the family norm): a dev process browses.
	if code, out := e.do(t, auth.DevUserID, "GET", "/api/projects", ""); code != http.StatusOK {
		t.Errorf("the dev identity reads: %d %s", code, out)
	}
}

// TestOnboardApprovalStillActivatesThroughTheOneDoor — R6: this packet routes
// NO activation verb. The card lands in the landed inbox, the landed
// `POST /api/asks/{ask}/answer` activates (D10 — the owner, and only the
// owner), and the projects detail then reads `active`. No route of this family
// participates in any of it.
func TestOnboardApprovalStillActivatesThroughTheOneDoor(t *testing.T) {
	e := newProjEnv(t)
	if code, out := e.do(t, "alice", "POST", "/api/projects", `{"project_id":"p-new","name":"New Proj"}`); code != http.StatusOK {
		t.Fatalf("POST: %d %s", code, out)
	}
	// What the scheduler's dispatch of the onboarding run writes: the durable
	// owner-approval ask (stage/onboard.go dispatchOnboard).
	exec(t, e.b, `INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts) VALUES (?, ?, ?, ?, 'open', ?)`,
		"onboard:p-new", "onboard-p-new.onboard", "alice",
		`{"kind":"onboard","project_id":"p-new","owner":"alice","draft":{}}`,
		time.Now().UTC().Format(time.RFC3339Nano))

	code, out := e.do(t, "alice", "GET", "/api/approvals", "")
	if code != http.StatusOK || !strings.Contains(out, "onboard:p-new") {
		t.Fatalf("the onboarding card must be in the ONE inbox: %d %s", code, out)
	}
	if code, out := e.do(t, "bob", "POST", "/api/asks/onboard:p-new/answer", `{"answer":{"approve":true}}`); code != http.StatusForbidden {
		t.Errorf("a non-owner answer must stay 403 (D10): %d %s", code, out)
	}
	if code, out := e.do(t, "alice", "POST", "/api/asks/onboard:p-new/answer", `{"answer":{"approve":true}}`); code != http.StatusOK {
		t.Fatalf("the owner's approval: %d %s", code, out)
	}
	code, out = e.do(t, "alice", "GET", "/api/projects/p-new", "")
	if code != http.StatusOK || !strings.Contains(out, `"state":"active"`) {
		t.Errorf("after approval the entry must read active: %d %s", code, out)
	}
}

// TestProjectsRouteTableIsTheThreeDoors — R6 as a structural fact rather than a
// promise: the family registers exactly the read pair and the create door, all
// session-required, and NO approve / rescan / member / delete verb exists at
// any shape under /api/projects. Re-scan on demand (S13.7) is real and is a
// deliberate absence here; activation lives on the landed ask-answer route.
func TestProjectsRouteTableIsTheThreeDoors(t *testing.T) {
	e := newProjEnv(t)
	srv := e.server("alice")
	_ = srv.Handler()
	got := map[string]bool{}
	for _, r := range srv.Routes() {
		if !strings.HasPrefix(r.Path, "/api/projects") {
			continue
		}
		if !r.Session {
			t.Errorf("%s %s is not session-required (S01.9)", r.Method, r.Path)
		}
		if strings.Contains(r.Path, "/v1") {
			t.Errorf("%s %s carries a version prefix (S15.2: unversioned at v0)", r.Method, r.Path)
		}
		got[r.Method+" "+r.Path] = true
	}
	want := map[string]bool{
		"GET /api/projects":           true,
		"POST /api/projects":          true,
		"GET /api/projects/{project}": true,
	}
	for w := range want {
		if !got[w] {
			t.Errorf("%s is not registered", w)
		}
	}
	for g := range got {
		if !want[g] {
			t.Errorf("%s is a verb this packet does not own — no second activation door, no rescan, no member or delete verb (R6)", g)
		}
	}
}

// TestProjectsVisibilitySweepHTTP is the property sweep lifted to the
// transport: every requester class × entry state × route, with ONE invariant —
// no answer ever contains an entry the caller neither owns nor belongs to, and
// every refusal is the one 404 that an unknown id gets. The classes include the
// operator, whose limb is deliberately absent (OQ3), and the states include
// pending, which is visible to its own people and to nobody else (OQ2).
func TestProjectsVisibilitySweepHTTP(t *testing.T) {
	e := newProjEnv(t)
	capture := project.CaptureInput{
		Conventions: []string{"swept-convention"},
		Commands:    project.Commands{Test: "go test ./..."},
		DangerZones: []project.DangerZone{{Path: "deploy/", Rule: "never touch deploy"}},
	}
	e.seedActive(t, "p-sweep-active", "alice", []string{"bob"}, capture)
	e.seedPending(t, "p-sweep-pending", "alice", []string{"bob"}, capture)
	e.seedActive(t, "p-sweep-carol", "carol", nil, capture)
	e.seedActive(t, "p-sweep-op", "op", nil, capture)

	entries := []struct {
		id      string
		owner   string
		members []string
	}{
		{"p-sweep-active", "alice", []string{"bob"}},
		{"p-sweep-pending", "alice", []string{"bob"}},
		{"p-sweep-carol", "carol", nil},
		{"p-sweep-op", "op", nil},
	}
	belongs := func(who string, i int) bool {
		if entries[i].owner == who {
			return true
		}
		for _, m := range entries[i].members {
			if m == who {
				return true
			}
		}
		return false
	}

	seen := 0
	for _, who := range []string{"alice", "bob", "carol", "op"} {
		listCode, list := e.do(t, who, "GET", "/api/projects", "")
		if listCode != http.StatusOK {
			t.Fatalf("list as %s: %d %s", who, listCode, list)
		}
		unknownCode, unknown := e.do(t, who, "GET", "/api/projects/p-sweep-nothing", "")
		if unknownCode != http.StatusNotFound {
			t.Fatalf("an unknown id must 404 for %s: %d", who, unknownCode)
		}
		for i := range entries {
			want := belongs(who, i)
			if got := strings.Contains(list, entries[i].id); got != want {
				t.Errorf("list as %s: %s present=%v, want %v: %s", who, entries[i].id, got, want, list)
			}
			code, out := e.do(t, who, "GET", "/api/projects/"+entries[i].id, "")
			switch {
			case want && code != http.StatusOK:
				t.Errorf("detail %s as %s: %d (want 200): %s", entries[i].id, who, code, out)
			case !want && code != http.StatusNotFound:
				t.Errorf("detail %s as %s: %d (want 404 — not owned, not a member)", entries[i].id, who, code)
			case !want && out != unknown:
				t.Errorf("detail %s as %s differs from the unknown-id refusal — an existence oracle:\n %s\n %s",
					entries[i].id, who, out, unknown)
			}
			if !want && strings.Contains(out, "swept-convention") {
				t.Errorf("a refusal carried captured content to %s: %s", who, out)
			}
			seen++
		}
	}
	if seen != 16 {
		t.Fatalf("the sweep covered %d requester×entry cells over two routes; the table is not exhaustive", seen)
	}
}

// TestOnboardStartMintsNothingOfItsOwn — R9 / §39 OQ8: the create door records
// nothing ON TOP of what the layers below already record. After a POST the log
// carries the registry's OWN two events and nothing else: no api-minted
// `registry.*`, and zero `decision.recorded`, which is what a transport
// co-minting somebody else's act looks like. (In production the run substrate
// adds its own run-lifecycle rows for the same reason — they are its record of
// its own act, not this door's.)
func TestOnboardStartMintsNothingOfItsOwn(t *testing.T) {
	e := newProjEnv(t)
	head := e.count(t, `SELECT COALESCE(MAX(event_seq), 0) FROM run_events`)
	if code, out := e.do(t, "alice", "POST", "/api/projects", `{"project_id":"p-new","name":"New Proj"}`); code != http.StatusOK {
		t.Fatalf("POST: %d %s", code, out)
	}
	rows, err := e.b.db.QueryContext(e.ctx,
		`SELECT type, COUNT(*) FROM run_events WHERE event_seq > ? GROUP BY type ORDER BY type`, head)
	if err != nil {
		t.Fatalf("walk the log: %v", err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var typ string
		var n int
		if err := rows.Scan(&typ, &n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[typ] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("walk the log: %v", err)
	}
	want := map[string]int{"registry.registered": 1, "registry.captured": 1}
	if len(got) != len(want) {
		t.Errorf("the POST minted %v — the registry's own rows ARE the audit and nothing is added to them", got)
	}
	for typ, n := range want {
		if got[typ] != n {
			t.Errorf("%s: %d rows, want %d", typ, got[typ], n)
		}
	}
	if got["decision.recorded"] != 0 {
		t.Errorf("%d decision.recorded rows — the create door co-minted a decision (§39 OQ8)", got["decision.recorded"])
	}
}

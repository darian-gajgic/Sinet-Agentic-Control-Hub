package stage_test

// bootstrap_gf5_test.go — P3-GF5 at the STAGE boundary: what the owner's HTTP
// commands write actually changes for a task's verification.
//
// TWO WORLDS, IDENTICAL EXCEPT FOR ONE HTTP CALL. Both register the same
// command-less project and walk the same give-work journey. In the first,
// nothing is captured and the drain lands under the GF4 bootstrap posture with
// no rung executed. In the second, the OWNER posts the project's commands
// through the real HTTP door before the journey, and the same drain resolves
// the real ladder and runs them. That difference is r4-F1b at drain grade: the
// wall the operator hit was that no door existed to make the second world
// reachable.
//
// COMMITTED RED (CONVENTIONS §3 Amendment-A): the commands door and the store's
// EditCommands verb do not exist yet, so this file fails to compile against the
// pre-GF5 tree; the packet's implementation commit closes the window. A NEW
// file, because the packet may not modify pre-existing test files —
// bootstrap_gf4_test.go and its rig stay byte-unmodified, and their green is
// what proves the per-round pickup this write feeds.
//
// WHAT IS REAL AND WHAT STANDS IN. Real: the whole journey, the real project
// store, the real HTTP handler with its real validation, the real drain, the
// real verdict rows. Standing in: the composition root's two adapters — the
// api-facing commands seam (mirrored the way projects_test.go mirrors the
// onboarding door) and the capture→pack projection, which lives in
// internal/shell and is pinned there against the real registry
// (checkpack_gf4_test.go, commandsdoor_gf5_test.go). What this file adds that
// neither can: the DRAIN's behaviour on either side of the write.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// gf5Owner is who walkToVerify submits as, so the project's owner and the
// requester are the same person — the r4-F1 journey's own shape.
const gf5Owner = "u-operator"

// gf5ProjectID is the registered project the walked task is attached to.
const gf5ProjectID = "p-scaffold"

// gf5CappedSettings is the harness registry with the two S07.6 round keys
// lowered, so the drain parks after ONE judged round instead of three.
//
// A WRAPPER, NOT A REGISTRY WRITE: the pair carries a cross-key invariant
// (patience <= rework_rounds, settings/index.go), so both move together or
// neither does, and overriding at the seam keeps this file from writing ⚙
// state a later test in the same package could read. The values are the
// FLOOR of the declared range, not new numbers.
type gf5CappedSettings struct {
	base  stage.Settings
	rooms map[string]int64
}

func (s gf5CappedSettings) Int(key string) (int64, error) {
	if v, ok := s.rooms[key]; ok {
		return v, nil
	}
	return s.base.Int(key)
}

func (s gf5CappedSettings) Float(key string) (float64, error) {
	if v, ok := s.rooms[key]; ok {
		return float64(v), nil
	}
	return s.base.Float(key)
}

func (s gf5CappedSettings) FloatFor(key, userID string) (float64, error) {
	if v, ok := s.rooms[key]; ok {
		return float64(v), nil
	}
	return s.base.FloatFor(key, userID)
}

func (s gf5CappedSettings) Duration(key string) (time.Duration, error) { return s.base.Duration(key) }
func (s gf5CappedSettings) String(key string) (string, error)          { return s.base.String(key) }
func (s gf5CappedSettings) Bool(key string) (bool, error)              { return s.base.Bool(key) }
func (s gf5CappedSettings) Strings(key string) ([]string, error)       { return s.base.Strings(key) }

// gf5ReviseJudge forces VerdictRevise every round, without ever claiming a
// criterion failed.
//
// It passes each AC with evidence that is NOT a substring of the artifact, so
// the landed extractive-evidence rule forces the verdict to Unknown
// ("unknown: non-extractive evidence", verify/v2.go) and the Unknown ESCAPE
// synthesizes a blocker — the S07.5 rule that an undecided criterion can never
// dissolve into SHIP. That is the honest way to hold the drain open: the judge
// is not asserting a defect, it is failing to decide, which is exactly the
// state the rework ladder exists for.
type gf5ReviseJudge struct{}

func (gf5ReviseJudge) Compliance(_ context.Context, in verify.JudgeInput) (verify.Axis1Result, error) {
	var out verify.Axis1Result
	for _, ac := range in.ACs {
		out.Verdicts = append(out.Verdicts, verify.ACVerdict{
			Key: fmt.Sprintf("AC-%d", ac.N), Pass: true,
			Evidence: "this quotation appears nowhere in the artifact",
		})
	}
	return out, nil
}

func (gf5ReviseJudge) Sanity(context.Context, verify.JudgeInput) (verify.Axis2Result, error) {
	return verify.Axis2Result{ProbeNotes: map[verify.Probe]string{
		verify.ProbeReasonableUser:       "the note reads as asked",
		verify.ProbeImplicitExpectations: "nothing obvious is missing",
		verify.ProbeSideEffects:          "no unrequested changes",
		verify.ProbeExpertStandard:       "competent",
	}}, nil
}

func (gf5ReviseJudge) Meta() verify.JudgeMeta {
	return verify.JudgeMeta{Model: "gf5-revise-judge-1", SelfFamily: true}
}

// gf5PostureMember is the verdict row's posture member, matched as the exact
// JSON pair rather than as the bare word. The GF4 rig's judge is named
// `bootstrap-judge-1`, so a substring search for "bootstrap" matches EVERY
// verdict row this file produces — which would make the control below vacuous
// and the escape assertion below that permanently red.
const gf5PostureMember = `"posture":"` + string(verify.PostureBootstrap) + `"`

// gf5PassingRunner is this file's check runner: every rung exits 0. What is
// under test is WHICH ladder the drain resolves, not what a real toolchain says
// about a fixture repo — a runner that could fail would make the verdict depend
// on toolchain weather rather than on the capture.
type gf5PassingRunner struct{ ran []string }

func (r *gf5PassingRunner) RunCheck(_ context.Context, req verify.CheckRequest) (verify.CheckResult, error) {
	r.ran = append(r.ran, req.Check.ID)
	return verify.CheckResult{ExitCode: 0}, nil
}

// gf5CommandsSeam mirrors the composition root's commandsDoor (the onboardSeam
// precedent): the REAL store verb does the work, and its sentinels are
// translated exactly as internal/shell translates them (§38: on the sentinel,
// never on the message text).
type gf5CommandsSeam struct{ proj *project.Store }

var _ api.ProjectCommandsSurface = gf5CommandsSeam{}

func (s gf5CommandsSeam) SetCommands(ctx context.Context, caller, projectID string, c api.ProjectCommands) (bool, error) {
	_, minted, err := s.proj.EditCommands(ctx, projectID, caller, project.Commands{
		Build: c.Build, Test: c.Test, Lint: c.Lint, Run: c.Run, Preview: c.Preview,
	})
	switch {
	case err == nil:
		return minted, nil
	case errors.Is(err, project.ErrNotFound):
		return false, &api.SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: "project not found"}
	case errors.Is(err, project.ErrNotOwner):
		return false, &api.SurfaceError{Status: http.StatusForbidden, Code: "not_owner", Msg: err.Error()}
	case errors.Is(err, project.ErrNotActive):
		return false, &api.SurfaceError{Status: http.StatusConflict, Code: "not_active", Msg: err.Error()}
	case errors.Is(err, project.ErrBadInput):
		return false, &api.SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: err.Error()}
	}
	return false, err
}

// gf5Identity is the session identity of the person driving the door.
type gf5Identity struct{ id string }

func (f gf5Identity) Authenticate(*http.Request) (api.Identity, error) {
	return api.Identity{UserID: f.id}, nil
}

// gf5Harness is outageHarness with the two seams it has no parameter for: a
// check runner (a resolved pack requires one) and a project store the resolver
// reads its answer out of. Mirrored rather than extended, because the packet may
// not modify the GF4 rig.
type gf5Harness struct {
	*harness
	proj   *project.Store
	runner *gf5PassingRunner
	srv    func(who string) *api.Server
}

func newGF5Harness(t *testing.T) *gf5Harness {
	t.Helper()
	return newGF5HarnessWith(t, bootstrapJudge{}, nil)
}

// newGF5HarnessWith is the same rig with the judge and the S07.6 round keys
// under the test's control, so a drain can be held open to a card.
func newGF5HarnessWith(t *testing.T, judge verify.Judge, rooms map[string]int64) *gf5Harness {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	runs := run.NewStore(db, log)
	cps := gates.NewCheckpoints(db, log)
	led := ledger.NewStore(db, log)
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	root := t.TempDir()
	rev := &review.Store{DB: db, Log: log, Settings: reg, Root: filepath.Join(root, "review")}
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(root, "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	runner := &gf5PassingRunner{}

	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Ledger: led,
		Settings: gf5CappedSettings{base: reg, rooms: rooms},
		Adapters: map[string]adapters.Adapter{
			adapters.SubstrateClaudeCLI: &claudecli.Adapter{
				Binary: self, HookCmd: "/opt/sinet/bin/sinet engine-hook", Settings: reg,
				Env:         append(os.Environ(), "SINET_STAGE_FAKE=1"),
				CancelGrace: 500 * time.Millisecond,
			},
		},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
		CopyAsideDir: filepath.Join(root, "copy-aside"),
		Review:       rev,
		Judge:        judge,
		CheckPackFor: gf5PackResolver(proj),
		CheckRunner:  runner,
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	priceTable := metering.NewEffectiveDatedTable("empty-v0")
	exceptions := metering.NoMeteredExceptions()
	sched, err := scheduler.New(scheduler.Config{
		DB: db, Runs: runs, Settings: reg, Dispatcher: sk,
		Receipts:     metering.NewReceipts(db, metering.NewLedger(db, priceTable, exceptions, reg), exceptions),
		LeaseTTL:     time.Minute,
		PollInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("scheduler.New: %v", err)
	}
	sk.Bind(sched)

	sessions := auth.New(db, log)
	if err := sessions.CreateUser(ctx, "",
		auth.User{ID: gf5Owner, DisplayName: "Owner", Role: auth.RoleOperator}, "hunter2hunter"); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	h := &harness{t: t, db: db, log: log, runs: runs, cps: cps, led: led,
		sk: sk, sched: sched, sur: sk.Surface(), review: rev,
		artifactRoot: filepath.Join(root, "artifacts")}
	return &gf5Harness{
		harness: h, proj: proj, runner: runner,
		srv: func(who string) *api.Server {
			return api.New(api.Config{
				Log: log, Sessions: sessions, Auth: gf5Identity{who},
				Settings: reg, HealthFn: func() api.Health { return api.Health{Ready: true} },
				DB: db, ProjectCommands: gf5CommandsSeam{proj: proj},
			})
		},
	}
}

// gf5PackResolver is the shape of the composition root's resolver over the REAL
// registry: it reads the CURRENT capture on every judged round (the GF4
// contract) and projects it onto the S07.3 rungs. The production projection
// itself is internal/shell's and is pinned in that package; what matters here is
// that the answer is READ FROM THE STORE each time, which is what makes a write
// through the door observable to the very next round.
func gf5PackResolver(proj *project.Store) func(ctx context.Context, domain, taskID string) (*verify.CheckPack, error) {
	return func(ctx context.Context, domain, taskID string) (*verify.CheckPack, error) {
		if !verify.LaunchDomain(domain) {
			return nil, nil
		}
		e, err := proj.Get(ctx, gf5ProjectID)
		if err != nil {
			return nil, err
		}
		var checks []verify.Check
		for _, r := range []struct {
			id    string
			stage verify.LadderStage
			cmd   string
		}{
			{"lint", verify.StageStatic, e.Capture.Commands.Lint},
			{"build", verify.StageStatic, e.Capture.Commands.Build},
			{"test", verify.StageUnit, e.Capture.Commands.Test},
		} {
			if strings.TrimSpace(r.cmd) == "" {
				continue
			}
			checks = append(checks, verify.Check{
				ID: r.id, Stage: r.stage, Argv: []string{"/bin/sh", "-lc", r.cmd},
				FindingCategory: verify.CatACBlocker,
			})
		}
		if len(checks) == 0 {
			return verify.BootstrapPack(domain, e.Capture.Version), nil
		}
		capturedAt, err := time.Parse(time.RFC3339Nano, e.Capture.CapturedTS)
		if err != nil {
			return nil, err
		}
		return &verify.CheckPack{Domain: domain, Version: e.Capture.Version, VerifiedOn: capturedAt, Checks: checks}, nil
	}
}

// seedCommandless registers, captures with NO command and activates the
// project — the r4-F1 fresh-scaffold shape.
func (h *gf5Harness) seedCommandless(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := h.proj.Register(ctx, project.RegisterInput{
		ProjectID: gf5ProjectID, Owner: gf5Owner, Name: "scaffold", StorePath: filepath.Join(t.TempDir(), "store"),
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := h.proj.Capture(ctx, project.CaptureInput{
		ProjectID: gf5ProjectID, By: gf5Owner, ScanHash: "scan-v1", Family: project.FamilySoftware,
	}); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if _, err := h.proj.Activate(ctx, gf5ProjectID, gf5Owner); err != nil {
		t.Fatalf("Activate: %v", err)
	}
}

func (h *gf5Harness) post(t *testing.T, who, path, body string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	h.srv(who).Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

// gf5RoundPayloads returns the run's verdict.recorded payloads ONE PER ROW, in
// order. roundRowsFor concatenates them, which cannot answer "did the LAST
// round carry the posture" — the question the escape sequence turns on.
func gf5RoundPayloads(t *testing.T, h *gf5Harness, runID string) []string {
	t.Helper()
	rows, err := h.db.QueryContext(context.Background(),
		`SELECT payload FROM run_events WHERE run_id = ? AND type = ? ORDER BY event_seq`, runID, verify.EventRound)
	if err != nil {
		t.Fatalf("read verdict rows: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan verdict row: %v", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("verdict rows: %v", err)
	}
	return out
}

// TestGF5EscapeFromBootstrapMidLifecycle [drain r1 F3] — the operator's own
// journey, in one test, with the pack flip coming from the DOOR and nothing
// else.
//
//	round 1 verifies under the BOOTSTRAP posture, through the real seam reading
//	the real registry  →  the run parks on an answerable card  →  the OWNER
//	captures the project's commands over HTTP, mid-lifecycle  →  the card is
//	answered  →  the resumed drain RE-RESOLVES the pack, runs the real ladder,
//	and its verdict row carries no bootstrap posture.
//
// NO CLOSURE FLIPS. `gf5PackResolver` is installed once at composition and
// never touched again; the only thing that changes between round 1 and round 2
// is the capture the HTTP door wrote. That is the difference from GF4's retry
// test, whose seam is flipped by the test itself — there the pack change was
// simulated, here it is caused.
//
// WHY THE CARD IS CAP-HIT AND THE VERB IS `revise_with_guidance` — the one
// place this departs from the finding's literal wording, because the literal
// wording is not reachable. `retry` exists in exactly ONE card vocabulary,
// `infraChoices` (verify/escalate.go), and that card is raised only for an
// `Escalation.Infrastructure`, whose only builder fires on a
// `*verify.PreambleRefusal` — an OUTAGE. A bootstrap resolution is not an
// error: it is a pack, so it raises no infrastructure card and there is no
// `retry` to answer. Manufacturing one would mean making the resolver fail,
// which is the closure flip this finding forbids. The reachable terminal after
// a bootstrap round the judge cannot decide is the S07.6 CAP-HIT card, and its
// `revise_with_guidance` answer resumes through `ResumeWithGuidance` →
// `validateInput` → `resolvePack` → a FRESH `CheckPackFor` call. Same
// mechanism, same proof, reachable vocabulary.
func TestGF5EscapeFromBootstrapMidLifecycle(t *testing.T) {
	ctx := context.Background()
	// One judged round, then the card. The convergence key moves with it: the
	// two carry a cross-key invariant and a patience above the cap is not a
	// state the registry accepts.
	h := newGF5HarnessWith(t, gf5ReviseJudge{}, map[string]int64{
		"verification.rework_rounds":               1,
		"verification.convergence_patience_rounds": 1,
	})
	h.seedCommandless(t)

	taskID := walkToVerify(t, h.harness, "software")
	verifyRun := taskID + ".verify"

	// ── round 1: the bootstrap posture, through the real seam ────────────
	rounds := gf5RoundPayloads(t, h, verifyRun)
	if len(rounds) != 1 {
		t.Fatalf("judged rounds before the capture = %d, want exactly 1: %v", len(rounds), rounds)
	}
	if !strings.Contains(rounds[0], gf5PostureMember) {
		t.Fatalf("round 1 did not run under the bootstrap posture — this is not the world under test:\n%s", rounds[0])
	}
	if len(h.runner.ran) != 0 {
		t.Fatalf("the bootstrap round executed rungs %v — bootstrap invents nothing on a project's behalf", h.runner.ran)
	}
	if got := h.runState(t, verifyRun); got != "parked" {
		t.Fatalf("verify run is %q after the bootstrap round, want parked on its card", got)
	}
	asks := openAskIDs(t, h.harness, taskID)
	if len(asks) != 1 {
		t.Fatalf("open asks = %v, want exactly the round-cap card", asks)
	}
	askID := asks[0]

	// ── the owner captures the commands, mid-lifecycle, over HTTP ────────
	code, body := h.post(t, gf5Owner, "/api/projects/"+gf5ProjectID+"/commands",
		`{"commands":{"build":"true","test":"true","lint":"true"}}`)
	if code != http.StatusOK {
		t.Fatalf("owner commands write = %d; body: %s", code, body)
	}
	e, err := h.proj.Get(ctx, gf5ProjectID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e.CaptureVersion != 2 {
		t.Fatalf("capture version v%d after the write, want v2", e.CaptureVersion)
	}

	// ── the card is answered; the resumed drain re-resolves the pack ─────
	if _, err := h.sur.Answer(ctx, gf5Owner, askID,
		[]byte(`{"choice":"revise_with_guidance","guidance":[{"text":"quote the artifact when you cite a criterion","criterion":"AC-1"}]}`),
		false); err != nil {
		t.Fatalf("Answer(revise_with_guidance): %v", err)
	}

	after := gf5RoundPayloads(t, h, verifyRun)
	if len(after) < 2 {
		t.Fatalf("no further judged round after the answer: %d rows", len(after))
	}
	last := after[len(after)-1]
	if strings.Contains(last, gf5PostureMember) {
		t.Fatalf("the round after the capture STILL carries the bootstrap posture — the door's write never reached the pack resolution:\n%s", last)
	}
	if len(h.runner.ran) == 0 {
		t.Fatal("no rung ran after the capture — the resumed drain did not run the real ladder")
	}
	if strings.Contains(last, verify.BootstrapAttribution) {
		t.Fatalf("the round after the capture still carries the advisory %q marking:\n%s", verify.BootstrapAttribution, last)
	}
}

// TestGF5WithoutTheDoorTheDrainStaysAdvisory is the CONTROL world: nothing is
// captured, so the drain lands under the bootstrap posture and no rung runs.
// Without it the test below could pass in a world where every software task runs
// a ladder regardless of the registry.
func TestGF5WithoutTheDoorTheDrainStaysAdvisory(t *testing.T) {
	h := newGF5Harness(t)
	h.seedCommandless(t)

	taskID := walkToVerify(t, h.harness, "software")
	verifyRun := taskID + ".verify"
	if got := h.runState(t, verifyRun); got != "completed" {
		t.Fatalf("verify run is %q, want completed — the bootstrap drain reaches a verdict (Spec S07.8)", got)
	}
	if !strings.Contains(roundRowsFor(t, h.harness, verifyRun), gf5PostureMember) {
		t.Fatal("no verdict row carries the bootstrap posture in the command-less world")
	}
	if len(h.runner.ran) != 0 {
		t.Fatalf("the bootstrap round executed rungs %v — bootstrap invents nothing on a project's behalf", h.runner.ran)
	}
}

// TestGF5EscapeFromBootstrapThroughTheDoor [brief T7]: the same world, plus one
// HTTP call. The owner captures the project's commands through the real door,
// and the drain then RUNS them — the verdict row carries no bootstrap posture
// and no advisory marking, with zero verify-side changes between the two worlds.
func TestGF5EscapeFromBootstrapThroughTheDoor(t *testing.T) {
	ctx := context.Background()
	h := newGF5Harness(t)
	h.seedCommandless(t)

	code, body := h.post(t, gf5Owner, "/api/projects/"+gf5ProjectID+"/commands",
		`{"commands":{"build":"true","test":"true","lint":"true"}}`)
	if code != http.StatusOK {
		t.Fatalf("owner commands write = %d; body: %s", code, body)
	}
	e, err := h.proj.Get(ctx, gf5ProjectID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e.CaptureVersion != 2 {
		t.Fatalf("capture version v%d after the write, want v2 (a NEW immutable version)", e.CaptureVersion)
	}
	// The carry-forward survived the door: dropping the scan hash would leave
	// DriftCheck comparing against nothing, and dropping the family would send
	// every later task in this project to the wrong question set.
	if e.Capture.ScanHash != "scan-v1" || e.Capture.Family != project.FamilySoftware {
		t.Fatalf("the edit dropped carried-forward content: scan_hash=%q family=%q", e.Capture.ScanHash, e.Capture.Family)
	}

	taskID := walkToVerify(t, h.harness, "software")
	verifyRun := taskID + ".verify"
	if got := h.runState(t, verifyRun); got != "completed" {
		t.Fatalf("verify run is %q, want completed", got)
	}
	if len(h.runner.ran) == 0 {
		t.Fatal("no rung ran after the capture — the write never reached the round's pack resolution, and r4-F1b is not fixed")
	}
	rows := roundRowsFor(t, h.harness, verifyRun)
	if strings.Contains(rows, gf5PostureMember) {
		t.Fatalf("a verdict row still carries the bootstrap posture after the capture:\n%s", rows)
	}
	if strings.Contains(rows, verify.BootstrapAttribution) {
		t.Fatalf("a verdict row still carries the advisory %q marking after the capture:\n%s", verify.BootstrapAttribution, rows)
	}
}

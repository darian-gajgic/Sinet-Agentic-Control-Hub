package shell

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// P3-RW-2 drain r1 (D3): the PRODUCTION onboarding seam under test.
//
// onboardDoor is this packet's one new production seam and the api suite drives
// a hand-mirrored fixture of it, so until this file the real door's two-call
// composition and its sentinel translation had never executed — and a fixture
// that mirrors an untested original is free to drift from it. This is the
// pinRefusal coverage precedent one seam over, colocated in a NEW file because
// the packet may not modify pre-existing test files.
//
// The composition here is REAL end to end: the real project.Store (register →
// git init → scan → draft, with real git), the real stage.Skeleton, the real
// scheduler behind its Enqueue ingress, and the real onboardDoor/onboardRefusal
// over both. The ONE fixture is the engine adapter, which stage.New requires and
// which nothing in this file can reach: no onboarding run is ever dispatched
// (the scheduler's claim loop is never started), so the dispatch half —
// dispatchOnboard's ask row and AnswerOnboarding's activation — stays outside
// what this file proves. It is proven at the transport in internal/api's
// TestOnboardApprovalStillActivatesThroughTheOneDoor and inside internal/stage.

// onboardSeamEngine satisfies stage.New's adapter requirement. It refuses
// loudly rather than answering: a test in this file that reached an engine
// would be dispatching a run it never meant to dispatch.
type onboardSeamEngine struct{}

func (onboardSeamEngine) Substrate() string { return adapters.SubstrateClaudeCLI }

func (onboardSeamEngine) Start(context.Context, adapters.StartRequest) (adapters.Session, error) {
	return nil, errors.New("the onboarding seam battery never dispatches a run")
}

func (onboardSeamEngine) Resume(context.Context, adapters.ParkRecord, *adapters.Answer) (adapters.Session, error) {
	return nil, errors.New("the onboarding seam battery never resumes a run")
}

// onboardRig composes the door the composition root composes (shell.go:594),
// with the scheduler held back so the run half can be made to fail on demand.
type onboardRig struct {
	ctx   context.Context
	db    *storage.DB
	proj  *project.Store
	runs  *run.Store
	sk    *stage.Skeleton
	sched *scheduler.Scheduler
	root  string
	door  onboardDoor
}

func newOnboardRig(t *testing.T) *onboardRig {
	t.Helper()
	ctx := context.Background()
	db, log, reg := seamDB(t)
	if err := reg.Attach(ctx, db, log); err != nil {
		t.Fatalf("settings Attach: %v", err)
	}
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO users (user_id, role, created_ts) VALUES ('alice','member',?)`,
			time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("user: %v", err)
	}
	root := filepath.Join(t.TempDir(), "projects")
	proj, err := project.New(project.Config{DB: db, Log: log, Root: root})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	runs := run.NewStore(db, log)
	stateDir := t.TempDir()
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs,
		Checkpoints:  gates.NewCheckpoints(db, log),
		Ledger:       ledger.NewStore(db, log),
		Settings:     reg,
		Adapters:     map[string]adapters.Adapter{adapters.SubstrateClaudeCLI: onboardSeamEngine{}},
		ArtifactRoot: filepath.Join(stateDir, "artifacts"),
		RunRoot:      filepath.Join(stateDir, "runs"),
		// The S13.7 onboarding-as-task seams, closed over internal/project
		// exactly as shell.Run closes them (shell.go:524–530).
		OnboardStart: func(ctx context.Context, projectID, owner, name, source string) (json.RawMessage, error) {
			return proj.OnboardStart(ctx, project.OnboardInput{ProjectID: projectID, Owner: owner, Name: name, Source: source})
		},
		OnboardApprove: func(ctx context.Context, projectID, owner string, edited json.RawMessage) error {
			return proj.OnboardApprove(ctx, projectID, owner, edited)
		},
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	sched, err := scheduler.New(scheduler.Config{
		DB: db, Runs: runs, Settings: reg, Dispatcher: sk,
		LeaseTTL:     time.Minute,
		PollInterval: time.Hour, // manual only: nothing ticks in this file
		Logger:       testLogger(),
	})
	if err != nil {
		t.Fatalf("scheduler.New: %v", err)
	}
	return &onboardRig{
		ctx: ctx, db: db, proj: proj, runs: runs, sk: sk, sched: sched, root: root,
		door: onboardDoor{proj: proj, sk: sk},
	}
}

// bind completes the composition. Until it is called the skeleton has no
// scheduler, which is how this file produces a run half that fails after the
// registry half has already committed.
func (r *onboardRig) bind() { r.sk.Bind(r.sched) }

func (r *onboardRig) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := r.db.QueryRowContext(r.ctx, q, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", q, err)
	}
	return n
}

// gitOutput runs one read-only git command in dir under the hermetic env the
// platform's own git uses (§23: no user, system or global config is consulted).
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "HOME=/nonexistent")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestOnboardDoorComposesOneOnboarding: the two calls ARE one onboarding. The
// registry half registers and drafts, the run half files the task and its run,
// the refs come back with the stage layer's own names — and the remote URL the
// door exists to carry (the deviation's whole reason) reaches the stored entry
// as DATA: nothing dials it and the store is not even configured with it.
func TestOnboardDoorComposesOneOnboarding(t *testing.T) {
	r := newOnboardRig(t)
	r.bind()
	const remote = "https://example.invalid/shop.git"

	refs, err := r.door.StartOnboarding(r.ctx, "alice", "shop", "Shop", remote)
	if err != nil {
		t.Fatalf("StartOnboarding: %v", err)
	}
	if refs.TaskID != stage.OnboardTaskID("shop") || refs.AskRef != stage.OnboardAskID("shop") {
		t.Fatalf("refs = %+v, want the stage layer's own names for the task and the ask", refs)
	}
	if named := r.door.OnboardRefs("shop"); named != refs {
		t.Errorf("the naming half disagrees with the performing half: %+v vs %+v", named, refs)
	}

	e, err := r.proj.Get(r.ctx, "shop")
	if err != nil {
		t.Fatalf("the registry half registered nothing: %v", err)
	}
	if e.State != project.StatePending {
		t.Errorf("state %q — the entry activates only on the owner's D10 approval (S13.7)", e.State)
	}
	if e.Owner != "alice" {
		t.Errorf("owner %q, want the caller (15.6)", e.Owner)
	}
	if e.CaptureVersion < 1 {
		t.Errorf("capture_version %d — the scan's draft is the entry's first capture", e.CaptureVersion)
	}
	if e.RemoteURL != remote {
		t.Errorf("remote_url %q, want %q stored as data (S13.7)", e.RemoteURL, remote)
	}
	if _, err := os.Stat(filepath.Join(e.StorePath, ".git")); err != nil {
		t.Errorf("no store was initialized at %s: %v", e.StorePath, err)
	}
	// STORED, NEVER DIALED: the URL is a registry column, not a transport the
	// platform configures — the fresh store has no git remote at all (§23).
	if remotes := gitOutput(t, e.StorePath, "remote"); remotes != "" {
		t.Errorf("the store carries git remotes %q — remote_url is recorded, never wired", remotes)
	}

	// The run half: the onboarding task and its run, attributed to the owner.
	var taskOwner string
	if err := r.db.QueryRowContext(r.ctx,
		`SELECT user_id FROM tasks WHERE task_id = ?`, refs.TaskID).Scan(&taskOwner); err != nil {
		t.Fatalf("no onboarding task row — onboarding is a task the platform performs: %v", err)
	}
	if taskOwner != "alice" {
		t.Errorf("onboarding task owner %q, want the caller", taskOwner)
	}
	if n := r.count(t, `SELECT COUNT(*) FROM runs WHERE run_id = ?`, refs.TaskID+stage.RunSuffixOnboard); n != 1 {
		t.Errorf("%d onboarding runs, want exactly one", n)
	}
	if n := r.count(t, `SELECT COUNT(*) FROM repo_registry_captures WHERE project_id = ?`, "shop"); n != 1 {
		t.Errorf("%d captures for one onboarding — the second call re-scanned the store", n)
	}
}

// TestOnboardDoorTornStateHeals (drain r1 D1, at the seam): the composition is
// two commits, not one, so a run half that fails leaves a pending entry with no
// onboarding task. The repair is the seam's OWN idempotence — called again, the
// registry half returns the existing draft rather than re-cloning and the run
// half files what is missing. This is the healing the create door's pre-read
// must not foreclose, proven where it actually happens.
func TestOnboardDoorTornStateHeals(t *testing.T) {
	r := newOnboardRig(t)

	// The run half cannot commit: the skeleton has no scheduler yet.
	if _, err := r.door.StartOnboarding(r.ctx, "alice", "torn", "Torn", ""); err == nil {
		t.Fatal("the run half must fail here — the torn state is this test's premise")
	}
	before, err := r.proj.Get(r.ctx, "torn")
	if err != nil {
		t.Fatalf("the registry half did not commit, so nothing is torn: %v", err)
	}
	if before.State != project.StatePending || before.CaptureVersion < 1 {
		t.Fatalf("the registry half left %q at v%d, want a pending entry with its draft",
			before.State, before.CaptureVersion)
	}
	if n := r.count(t, `SELECT COUNT(*) FROM tasks WHERE task_id = ?`, stage.OnboardTaskID("torn")); n != 0 {
		t.Fatalf("%d onboarding tasks — the state is not torn and the test proves nothing", n)
	}

	r.bind()
	refs, err := r.door.StartOnboarding(r.ctx, "alice", "torn", "Torn", "")
	if err != nil {
		t.Fatalf("the retry must heal the torn state: %v", err)
	}
	if n := r.count(t, `SELECT COUNT(*) FROM tasks WHERE task_id = ?`, refs.TaskID); n != 1 {
		t.Errorf("%d onboarding tasks after the heal, want the one that was missing", n)
	}
	if n := r.count(t, `SELECT COUNT(*) FROM runs WHERE task_id = ?`, refs.TaskID); n != 1 {
		t.Errorf("%d onboarding runs after the heal", n)
	}
	// Healing is not re-onboarding.
	if n := r.count(t, `SELECT COUNT(*) FROM repo_registry_captures WHERE project_id = ?`, "torn"); n != 1 {
		t.Errorf("%d captures — the heal re-scanned the store", n)
	}
	after, err := r.proj.Get(r.ctx, "torn")
	if err != nil {
		t.Fatalf("read back after the heal: %v", err)
	}
	if after.UpdatedTS != before.UpdatedTS || after.CaptureVersion != before.CaptureVersion {
		t.Errorf("the heal moved the entry: v%d@%s → v%d@%s",
			before.CaptureVersion, before.UpdatedTS, after.CaptureVersion, after.UpdatedTS)
	}
}

// TestOnboardDoorSurfacesTheEnqueueRefusal: the other way the run half fails.
// The scheduler is the sole run ingress and it refuses a run past `queued`, so
// once the onboarding run has parked on its approval card a further call is
// refused. The refusal is UNMARKED — it is a platform state, not a caller's
// fault, so it stays itself and reaches the transport as a 500 with the cause in
// the ops log rather than as a 4xx blaming whoever asked. Nothing is disturbed.
func TestOnboardDoorSurfacesTheEnqueueRefusal(t *testing.T) {
	r := newOnboardRig(t)
	r.bind()
	refs, err := r.door.StartOnboarding(r.ctx, "alice", "shop", "Shop", "")
	if err != nil {
		t.Fatalf("StartOnboarding: %v", err)
	}
	runID := refs.TaskID + stage.RunSuffixOnboard
	for _, to := range []run.State{run.StateClaimed, run.StateRunning, run.StateParked} {
		if _, err := r.runs.Transition(r.ctx, runID, to, run.TransitionOptions{
			Reason: "park the onboarding run on its approval card", Actor: run.ActorPlatform,
		}); err != nil {
			t.Fatalf("transition %s: %v", to, err)
		}
	}

	_, err = r.door.StartOnboarding(r.ctx, "alice", "shop", "Shop", "")
	if err == nil {
		t.Fatal("a second launch over a parked onboarding run must be refused at the ingress")
	}
	var se *api.SurfaceError
	if errors.As(err, &se) {
		t.Errorf("the enqueue refusal was typed as a caller-facing refusal (%d %s) — it is a platform state",
			se.Status, se.Code)
	}
	if n := r.count(t, `SELECT COUNT(*) FROM repo_registry_captures WHERE project_id = ?`, "shop"); n != 1 {
		t.Errorf("%d captures — the refused call re-scanned the store", n)
	}
	if n := r.count(t, `SELECT COUNT(*) FROM runs WHERE task_id = ?`, refs.TaskID); n != 1 {
		t.Errorf("%d onboarding runs — the refused call filed a second one", n)
	}
}

// TestOnboardRefusalMapsStoreSentinels: the translation the api fixture mirrors,
// proven on the real thing over REAL store refusals. Errors are read by SENTINEL
// and never by message text (§38), a conflict answers as a conflict, caller
// fault answers 400, and anything unmarked stays itself.
func TestOnboardRefusalMapsStoreSentinels(t *testing.T) {
	r := newOnboardRig(t)
	r.bind()

	// Drain r1 D2: a store directory with no registry entry behind it — what a
	// half-finished or overlapping onboarding of the same id leaves — is the
	// CONFLICT it is, and its refusal carries no host filesystem path (the path
	// the read doors deliberately serve to nobody, brief OQ4, must not escape
	// through the error channel either — S11 host hygiene).
	if err := os.MkdirAll(filepath.Join(r.root, "stores", "residue"), 0o700); err != nil {
		t.Fatalf("stage the residue: %v", err)
	}
	_, err := r.door.StartOnboarding(r.ctx, "alice", "residue", "Residue", "")
	var se *api.SurfaceError
	if !errors.As(err, &se) {
		t.Fatalf("the store-exists refusal did not cross the seam as a transport error: %v", err)
	}
	if se.Status != http.StatusConflict || se.Code != "already_registered" {
		t.Errorf("the store-exists refusal = %d %s, want 409 already_registered", se.Status, se.Code)
	}
	if strings.Contains(se.Msg, r.root) {
		t.Errorf("the refusal carries a host filesystem path: %s", se.Msg)
	}

	// Genuine caller fault stays caller fault.
	_, err = r.door.StartOnboarding(r.ctx, "alice", "noname", "", "")
	if !errors.As(err, &se) {
		t.Fatalf("a malformed onboarding did not cross the seam as a transport error: %v", err)
	}
	if se.Status != http.StatusBadRequest || se.Code != "bad_request" {
		t.Errorf("the missing-name refusal = %d %s, want 400 bad_request", se.Status, se.Code)
	}

	// The registration conflict two racing onboardings produce.
	if err := onboardRefusal(fmt.Errorf("%w: %q", project.ErrAlreadyRegistered, "shop")); !errors.As(err, &se) ||
		se.Status != http.StatusConflict || se.Code != "already_registered" {
		t.Errorf("ErrAlreadyRegistered mapped to %v, want 409 already_registered", err)
	}

	// Unmarked errors are the platform's own: they stay themselves, so the
	// transport answers 500 and the ops log carries the cause.
	boom := errors.New("the database is on fire")
	got := onboardRefusal(boom)
	if !errors.Is(got, boom) {
		t.Errorf("an unmarked error was rewritten: %v", got)
	}
	if errors.As(got, &se) {
		t.Errorf("an unmarked error was typed as a caller-facing refusal (%d %s)", se.Status, se.Code)
	}
	if onboardRefusal(nil) != nil {
		t.Error("onboardRefusal invented an error out of nil")
	}
}

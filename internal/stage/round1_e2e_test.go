package stage_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// A project-wired composition harness: the walking-skeleton spine PLUS the
// S13.5/S13.7 git seams over a REAL internal/project store, wired exactly as
// the shell's projectSeams does (F8/F9). The onboarding seams (F1) ride it too.

// pseams mirrors shell.projectSeams: run→project via the DURABLE intake match,
// worktree scoped to execute legs, snapshot resolves an EXISTING worktree only.
type pseams struct {
	proj *project.Store
	runs *run.Store
	pipe *intake.Pipeline
}

func (p *pseams) projectForTask(ctx context.Context, taskID string) (string, error) {
	if p.pipe == nil || taskID == "" {
		return "", nil
	}
	st, err := p.pipe.LoadState(ctx, taskID)
	if err != nil {
		return "", nil
	}
	if st.Registry == nil {
		return "", nil
	}
	return st.Registry.Project, nil
}

func (p *pseams) WorkspaceCwd(ctx context.Context, runID string) (string, bool, error) {
	if !strings.HasSuffix(runID, stage.RunSuffixExecute) {
		return "", false, nil
	}
	r, err := p.runs.Get(ctx, runID)
	if err != nil {
		return "", false, err
	}
	pid, err := p.projectForTask(ctx, r.TaskID)
	if err != nil || pid == "" {
		return "", false, err
	}
	ws, err := p.proj.EnsureWorkspace(ctx, pid, r.TaskID)
	if err != nil {
		return "", false, err
	}
	_ = p.runs.SetWorkspaceRef(ctx, runID, ws.Path)
	return ws.Path, true, nil
}

func (p *pseams) Snapshot(ctx context.Context, runID string) (string, error) {
	r, err := p.runs.Get(ctx, runID)
	if err != nil {
		return "", err
	}
	pid, err := p.projectForTask(ctx, r.TaskID)
	if err != nil || pid == "" {
		return "", err
	}
	path, ok, err := p.proj.ExistingWorkspace(ctx, pid, r.TaskID)
	if err != nil || !ok {
		return "", err
	}
	return p.proj.Snapshot(ctx, path)
}

func (p *pseams) CreateRevisionRef(ctx context.Context, runID, ref, sha string) error {
	r, err := p.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	pid, err := p.projectForTask(ctx, r.TaskID)
	if err != nil || pid == "" {
		return err
	}
	return p.proj.CreateRevisionRef(ctx, pid, ref, sha)
}

type projHarness struct {
	*harness
	proj *project.Store
	root string
}

func newProjectHarness(t *testing.T) *projHarness {
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
	self, _ := os.Executable()
	root := t.TempDir()
	rev := &review.Store{DB: db, Log: log, Settings: reg, Root: filepath.Join(root, "review")}
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(root, "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	ps := &pseams{proj: proj, runs: runs}

	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Ledger: led, Settings: reg,
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
		Registry:     registryOver{proj: proj},
		Snapshot:     ps.Snapshot, CreateRevisionRef: ps.CreateRevisionRef, WorkspaceCwd: ps.WorkspaceCwd,
		OnboardStart: func(ctx context.Context, pid, owner, name, source string) (json.RawMessage, error) {
			return proj.OnboardStart(ctx, project.OnboardInput{ProjectID: pid, Owner: owner, Name: name, Source: source})
		},
		OnboardApprove: func(ctx context.Context, pid, owner string, edited json.RawMessage) error {
			return proj.OnboardApprove(ctx, pid, owner, edited)
		},
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	ps.pipe = sk.Pipeline()
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
	h := &harness{t: t, db: db, log: log, runs: runs, cps: cps, led: led,
		sk: sk, sched: sched, sur: sk.Surface(), review: rev, artifactRoot: filepath.Join(root, "artifacts")}
	return &projHarness{harness: h, proj: proj, root: root}
}

// registryOver is the production intake.Registry over repo_registry (the
// shell's registrySeam), inlined for the composition test.
type registryOver struct{ proj *project.Store }

func (r registryOver) Match(ctx context.Context, req intake.Request) (intake.RegistrySlice, bool, error) {
	// Mirrors the shell seam's submitted-pin pass-through (P3-RW-1), refusal
	// mapping included — the production edge is proven in internal/shell.
	if pin := req.Project; pin != "" {
		e, err := r.proj.PinForIntake(ctx, pin, req.UserID)
		if err != nil {
			switch {
			case errors.Is(err, project.ErrNotActive):
				return intake.RegistrySlice{}, false, fmt.Errorf("%w: %q", intake.ErrPinNotActive, pin)
			case errors.Is(err, project.ErrNotFound):
				return intake.RegistrySlice{}, false, fmt.Errorf("%w: %q", intake.ErrPinUnknown, pin)
			}
			return intake.RegistrySlice{}, false, err
		}
		return registrySliceOver(e), true, nil
	}
	e, ok, err := r.proj.MatchForIntake(ctx, project.MatchHint{UserID: req.UserID, Title: req.Title, Text: req.Text})
	if err != nil || !ok {
		return intake.RegistrySlice{}, false, err
	}
	return registrySliceOver(e), true, nil
}

func registrySliceOver(e project.Entry) intake.RegistrySlice {
	var zones []intake.DangerZone
	for _, z := range e.Capture.DangerZones {
		zones = append(zones, intake.DangerZone{Path: z.Path, Rule: z.Rule, SourceHash: z.SourceHash})
	}
	return intake.RegistrySlice{Project: e.ProjectID, Ref: e.ProjectID, DangerZones: zones}
}

// gitFixture builds a local git repo to clone during onboarding.
func gitFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "src")
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "HOME=/nonexistent",
		"GIT_AUTHOR_NAME=fx", "GIT_AUTHOR_EMAIL=fx@t.invalid", "GIT_COMMITTER_NAME=fx", "GIT_COMMITTER_EMAIL=fx@t.invalid")
	g := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir, c.Env = dir, env
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	os.MkdirAll(dir, 0o700)
	g("init", "-q", "--initial-branch=main", dir)
	for rel, c := range files {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o700)
		os.WriteFile(p, []byte(c), 0o600)
	}
	g("add", "-A")
	g("commit", "-q", "-m", "seed")
	return dir
}

// TestOnboardingAsTaskE2E (F1): register → clone → scan → draft on a durable
// ask → owner (D10) approves → entry active; a non-owner answer is refused.
func TestOnboardingAsTaskE2E(t *testing.T) {
	h := newProjectHarness(t)
	ctx := context.Background()
	const owner = "alice"
	src := gitFixture(t, map[string]string{"go.mod": "module shop\n", ".env": "SECRET=1\n"})

	taskID, err := h.sk.StartOnboarding(ctx, owner, "shop", "shop", src)
	if err != nil {
		t.Fatalf("StartOnboarding: %v", err)
	}
	// The pending entry + draft exist (register → clone → scan → draft).
	e, err := h.proj.Get(ctx, "shop")
	if err != nil || e.State != project.StatePending || e.CaptureVersion != 1 {
		t.Fatalf("onboarded entry = %+v err=%v, want pending w/ a draft", e, err)
	}
	// Dispatch: the run parks on a durable owner-approval ask.
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the onboarding run", n)
	}
	// The durable onboarding ask is open on the onboard run, carrying the draft.
	var askID, snapshot, status string
	if err := h.db.QueryRowContext(ctx,
		`SELECT a.ask_id, a.snapshot, a.status FROM asks a JOIN runs r ON r.run_id = a.run_id
		 WHERE r.task_id = ? AND a.status = 'open'`, taskID).Scan(&askID, &snapshot, &status); err != nil {
		t.Fatalf("no durable onboarding ask: %v", err)
	}
	if !strings.HasPrefix(askID, "onboard:") {
		t.Fatalf("ask %q is not an onboarding ask", askID)
	}
	if !strings.Contains(snapshot, "danger_zones") {
		t.Fatalf("onboarding ask does not carry the drafted danger zones: %s", snapshot)
	}

	// A NON-OWNER answer is refused (D10).
	if _, err := h.sur.Answer(ctx, "mallory", askID, json.RawMessage(`{"approve":true}`), false); err == nil {
		t.Fatal("a non-owner onboarding approval must be refused (D10)")
	}
	e, _ = h.proj.Get(ctx, "shop")
	if e.Active() {
		t.Fatal("a refused approval activated the entry")
	}

	// The OWNER approves → the entry activates.
	if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"approve":true}`), false); err != nil {
		t.Fatalf("owner Answer: %v", err)
	}
	e, _ = h.proj.Get(ctx, "shop")
	if !e.Active() {
		t.Fatal("entry not active after owner approval")
	}
}

// TestRegisteredProjectChainAndSnapshot drives the REAL composition on a
// registered project: repo_registry → registrySeam.Match → ledger §2 danger
// zones (F8); execute-leg worktree + snapshot → the minted revision's
// snapshot_sha + platform ref through the REAL reviewSink + real project seams
// (F9); and the intake leg stays workspace-less (F4).
func TestRegisteredProjectChainAndSnapshot(t *testing.T) {
	h := newProjectHarness(t)
	ctx := context.Background()
	const owner = "u-operator"

	// Onboard + activate a project "shop" (its scan drafts a .env danger zone).
	src := gitFixture(t, map[string]string{"go.mod": "module shop\n", ".env": "SECRET=1\n"})
	if _, err := h.proj.OnboardStart(ctx, project.OnboardInput{ProjectID: "shop", Owner: owner, Name: "shop", Source: src}); err != nil {
		t.Fatalf("OnboardStart: %v", err)
	}
	if err := h.proj.OnboardApprove(ctx, "shop", owner, nil); err != nil {
		t.Fatalf("OnboardApprove: %v", err)
	}

	// Submit a request naming the project → the registry matches at S06.2.
	raw, err := h.sur.Submit(ctx, owner, json.RawMessage(
		`{"title":"shop note","text":"Write a short appreciation note in the shop project."}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	taskID := decodeView(t, raw).TaskID

	// Intake dispatch → interview card. The intake leg is WORKSPACE-LESS (F4):
	// no worktree exists for the task yet.
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("intake tick dispatched %d", n)
	}
	if _, ok, _ := h.proj.ExistingWorkspace(ctx, "shop", taskID); ok {
		t.Fatal("the intake leg materialized a worktree — it must stay workspace-less (F4)")
	}

	// Force-proceed → approval card; approve (PIN step-up).
	raw, _ = h.sur.Answer(ctx, owner, decodeView(t, mustTask(t, h, taskID)).OpenAskID, json.RawMessage(`{"force_proceed":true}`), false)
	if _, err := h.sur.Answer(ctx, owner, decodeView(t, raw).OpenAskID, json.RawMessage(`{"action":"approve"}`), true); err != nil {
		t.Fatalf("approve: %v", err)
	}

	// F8: the registry danger zones are PINNED in ledger §2 (real chain).
	doc, _, err := h.led.Current(ctx, taskID)
	if err != nil {
		t.Fatalf("ledger Current: %v", err)
	}
	var sawEnv bool
	for _, z := range doc.Constraints.DangerZones {
		if z.Path == ".env" && z.SourceHash != "" {
			sawEnv = true
		}
	}
	if !sawEnv {
		t.Fatalf("registry .env danger zone not pinned in ledger §2: %+v", doc.Constraints.DangerZones)
	}

	// Execute + verify: the execute leg gets the worktree; the verify handoff
	// mints the revision with the snapshot sha + creates the platform ref.
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("execute tick dispatched %d", n)
	}
	if _, ok, _ := h.proj.ExistingWorkspace(ctx, "shop", taskID); !ok {
		t.Fatal("the execute leg did not materialize the project worktree (R14)")
	}
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("verify tick dispatched %d", n)
	}

	// F9: the minted revision carries a real snapshot_sha, and its platform ref
	// resolves to that commit in the project store.
	rev, err := h.review.RevisionAt(ctx, "dlv-"+taskID, 1)
	if err != nil {
		t.Fatalf("RevisionAt: %v", err)
	}
	if rev.SnapshotSHA == "" {
		t.Fatal("a registered-project deliverable minted with NULL snapshot_sha (F9)")
	}
	e, _ := h.proj.Get(ctx, "shop")
	refSHA := gitRevParse(t, e.StorePath, review.RevisionRef("dlv-"+taskID, 1))
	if refSHA != rev.SnapshotSHA {
		t.Fatalf("platform ref → %q, want the snapshot %q", refSHA, rev.SnapshotSHA)
	}
}

func mustTask(t *testing.T, h *projHarness, taskID string) json.RawMessage {
	t.Helper()
	raw, err := h.sur.Task(context.Background(), taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	return raw
}

func gitRevParse(t *testing.T, dir, ref string) string {
	t.Helper()
	c := exec.Command("git", "rev-parse", ref)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "HOME=/nonexistent")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse %s: %v\n%s", ref, err, out)
	}
	return strings.TrimSpace(string(out))
}

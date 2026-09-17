package stage_test

// restoreseam_tq2_test.go — P3-TQ-2 drain r1 [F2]: a fork that NEEDS a restore
// and has no restore seam is a corpse, never a silent re-drive.
//
// The resume point says where the successor belongs: on the tree of the last
// step its lineage finished (Spec S02.5 step 2, S02.4 (d)). A process wired
// without `Config.RestoreWorkspace` cannot put it there. Skipping the restore
// and driving anyway would land the successor on whatever the dead run left on
// disk — its partial step and its untracked residue — which is exactly the
// witnessed defect this packet closes (TQ-F2). So the boundary refuses: the run
// crashes with a cause naming the missing restore, and the ladder can classify
// it like any other corpse.
//
// Production wiring makes this unreachable today (the shell composition root
// wires the seam beside WorkspaceCwd). The boundary is still asserted here,
// because "unreachable today" is a fact about one composition root and not a
// property of the dispatch (CONVENTIONS §2: validate at the boundary).
//
// The world is the tq2 spine with ONE seam removed, so the difference between
// this test and the resume e2e beside it is exactly the thing under test.
// Zero paid calls.

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// newTQ2NoRestoreWorld is newTQ2World's repo-backed spine with
// Config.RestoreWorkspace left unwired: snapshots are taken (so finished steps
// carry their evidence and the resume point is real), but nothing can put the
// worktree back.
func newTQ2NoRestoreWorld(t *testing.T, n, crashAt int) *tq2World {
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
	journal, err := gates.NewJournal(gates.JournalConfig{DB: db, Settings: reg})
	if err != nil {
		t.Fatalf("gates.NewJournal: %v", err)
	}
	root := t.TempDir()
	rev := &review.Store{DB: db, Log: log, Settings: reg, Root: filepath.Join(root, "review")}
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(root, "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	ps := &tq2Seams{proj: proj, runs: runs}
	w := &tq2World{proj: proj, clock: time.Now(),
		adapter: &tq2Adapter{t: t, crashAt: crashAt, crashMode: tq2CrashMid}}

	cfg := stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Ledger: led, Settings: reg,
		Adapters:     map[string]adapters.Adapter{adapters.SubstrateClaudeCLI: w.adapter},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
		CopyAsideDir: filepath.Join(root, "copy-aside"),
		Model:        "fake-model-1",
		Planner:      tq2Planner{n: n},
		Review:       rev,
		Registry:     registryOver{proj: proj},
	}
	cfg.Snapshot, cfg.CreateRevisionRef, cfg.WorkspaceCwd = ps.Snapshot, ps.CreateRevisionRef, ps.WorkspaceCwd
	cfg.RepoFacts = func(ctx context.Context, taskID string) (string, string, error) {
		return proj.SnapshotAndBase(ctx, "shop", taskID)
	}
	// THE POINT OF THIS WORLD: cfg.RestoreWorkspace stays nil.

	sk, err := stage.New(cfg)
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
	ladder, err := recovery.New(recovery.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Effects: journal, Settings: reg,
		Now: func() time.Time { return w.clock },
	})
	if err != nil {
		t.Fatalf("recovery.New: %v", err)
	}
	if err := runs.SetTerminalHook(func(_ context.Context, _ *sql.Tx, r run.Run) error {
		if w.failCompleted != "" && r.ID == w.failCompleted && r.State == run.StateCompleted {
			w.failCompleted = ""
			return errors.New("tq2: process died inside the completed transition (simulated)")
		}
		return nil
	}); err != nil {
		t.Fatalf("SetTerminalHook: %v", err)
	}
	w.ladder = ladder
	w.harness = &harness{t: t, db: db, log: log, runs: runs, cps: cps, led: led,
		sk: sk, sched: sched, sur: sk.Surface(), review: rev, artifactRoot: filepath.Join(root, "artifacts")}
	src := gitFixture(t, map[string]string{"go.mod": "module shop\n"})
	if _, err := proj.OnboardStart(ctx, project.OnboardInput{ProjectID: "shop", Owner: "u-tq2", Name: "shop", Source: src}); err != nil {
		t.Fatalf("OnboardStart: %v", err)
	}
	if err := proj.OnboardApprove(ctx, "shop", "u-tq2", nil); err != nil {
		t.Fatalf("OnboardApprove: %v", err)
	}
	return w
}

// TestTQ2ForkWithoutRestoreSeamCrashesLoudly: a repo-backed lineage whose first
// step finished (k=1, so the successor owes a restore to that step's snapshot)
// forked into a process with no restore seam. The successor must crash saying
// so, drive no session, and leave the dead run's partial work untouched — never
// re-drive the plan over a dirty tree.
func TestTQ2ForkWithoutRestoreSeamCrashesLoudly(t *testing.T) {
	const n = 2
	ctx := context.Background()
	w := newTQ2NoRestoreWorld(t, n, 2) // crash mid-step S-2, so S-1 is finished
	taskID := w.walkToExecute(ctx, true)
	parent := taskID + ".execute"
	if got := w.state(parent); got != run.StateCrashed {
		t.Fatalf("crash window: %s is %s, want crashed", parent, got)
	}
	// The resume point is real: S-1 finished and carries its close snapshot, so
	// this fork genuinely owes a restore.
	doc, _, err := w.led.Current(ctx, taskID)
	if err != nil {
		t.Fatalf("ledger Current: %v", err)
	}
	evidence := ""
	for _, it := range doc.State.Items {
		if it.ID == "S-1" {
			evidence = it.EvidenceRef
		}
	}
	if evidence == "" {
		t.Fatal("fixture: S-1 carries no evidence_ref, so this fork would owe no restore")
	}

	fork := w.forkOf(ctx, parent)
	if n := w.tick(ctx); n != 1 {
		t.Fatalf("fork tick dispatched %d", n)
	}

	if got := w.state(fork); got != run.StateCrashed {
		t.Fatalf("fork %s is %s, want crashed — a successor that cannot be put on its resume tree must not run", fork, got)
	}
	if starts := w.adapter.execStarts(fork); len(starts) != 0 {
		var steps []string
		for _, s := range starts {
			steps = append(steps, s.Step)
		}
		t.Errorf("the fork drove %v with no restore wired — this is the dirty re-drive the packet closes", steps)
	}
	// The corpse names WHY, so the ladder's next reader is not left guessing.
	crash := w.stateEvent(fork, run.StateCrashed)
	if crash == nil {
		t.Fatal("no crash transition recorded for the fork")
	}
	cause, _ := detailOf(crash)["cause"].(string)
	if !strings.Contains(cause, "restore") {
		t.Errorf("crash cause = %q, want it to name the missing workspace restore", cause)
	}
	if !strings.Contains(cause, evidence) {
		t.Errorf("crash cause = %q, want it to name the snapshot %s the run owed a restore to", cause, evidence)
	}
	// Nothing was seeded over the parent's record: the fork died before its
	// ledger write, so S-1 is still finished for whoever forks next.
	var seeded int
	if err := w.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_events WHERE run_id = ? AND type = ?`,
		fork, ledger.EventLedgerUpdate).Scan(&seeded); err != nil {
		t.Fatal(err)
	}
	if seeded != 0 {
		t.Errorf("the fork wrote %d ledger updates before crashing, want 0 — the restore is refused BEFORE the seed", seeded)
	}
	after, _, err := w.led.Current(ctx, taskID)
	if err != nil {
		t.Fatalf("ledger Current after: %v", err)
	}
	for _, it := range after.State.Items {
		if it.ID == "S-1" && (it.Status != ledger.StatusDoneUnverified || it.EvidenceRef != evidence) {
			t.Errorf("S-1 is now %s/%q, want done_unverified on %s — a refused fork must not disturb the record", it.Status, it.EvidenceRef, evidence)
		}
	}
}

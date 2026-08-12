package stage_test

// abortverify_e2e_test.go — P3-RW-10 T5(a): the S07.7 answer beat owes the same
// context-independent posture as the intake beats. A resumed drain whose rework
// seam dies WITH ITS CALLER leaves a classifiable corpse for the recovery
// ladder, with the answered card closed behind it — never a verify run stranded
// `running` with nobody driving it and no finding to show for the round.
//
// T5(b) — a FINISHED drain's outcome landing despite the abort — lives beside
// `verifyTerminal` itself (abort_internal_test.go): the drain's own writes are
// not detached by this packet, so the only point at which "the drain succeeded
// and then the caller died" can be injected honestly is that seam.
//
// Zero paid calls: the judge is the fake engine, the rework seam is this file's.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// abortRevise is the S07.6 fresh-session rework seam as a test double. Until it
// is ARMED it reworks normally (persisting each revision, as the engine seam
// must — the card's best-effort pin has to denote retrievable content); armed,
// it cancels the REQUEST's own context and fails, which is the browser-abort
// shape one layer down from the intake beats.
type abortRevise struct {
	mu           sync.Mutex
	armed        bool
	cancel       context.CancelFunc
	artifactRoot string
}

func (a *abortRevise) revise(ctx context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.armed {
		a.armed = false
		a.cancel()
		return verify.Deliverable{}, fmt.Errorf("rework session: %w", ctx.Err())
	}
	d := pkg.Deliverable
	d.PrevContent = d.Content
	d.Content = fmt.Sprintf("%s\nreworked for round %d\n", d.Content, pkg.Round)
	d.Revision = pkg.Deliverable.Revision + 1
	d.Diff = ""
	dir := filepath.Join(a.artifactRoot, d.TaskID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return verify.Deliverable{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("deliverable-rev%d.md", d.Revision)), []byte(d.Content), 0o600); err != nil {
		return verify.Deliverable{}, err
	}
	return d, nil
}

func (a *abortRevise) arm() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.armed = true
}

// newReviseSeamHarness is the e2e harness with the S07.6 rework seam INJECTED
// (the composition root's own dev/test seam; the production path spawns an
// engine session). Everything else is the standard walking-skeleton wiring, so
// the drive to the CAP-HIT card is the battery's.
func newReviseSeamHarness(t *testing.T, seam *abortRevise, extraEnv ...string) *harness {
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
	seam.artifactRoot = filepath.Join(root, "artifacts")
	rev := &review.Store{DB: db, Log: log, Settings: reg, Root: filepath.Join(root, "review")}
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Ledger: led, Settings: reg,
		Adapters: map[string]adapters.Adapter{
			adapters.SubstrateClaudeCLI: &claudecli.Adapter{
				Binary:      self,
				HookCmd:     "/opt/sinet/bin/sinet engine-hook",
				Settings:    reg,
				Env:         append(append(os.Environ(), "SINET_STAGE_FAKE=1"), extraEnv...),
				CancelGrace: 500 * time.Millisecond,
			},
		},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
		CopyAsideDir: filepath.Join(root, "copy-aside"),
		Review:       rev,
		Revise:       seam.revise,
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
		PollInterval: time.Hour, // manual Ticks only
	})
	if err != nil {
		t.Fatalf("scheduler.New: %v", err)
	}
	sk.Bind(sched)
	return &harness{t: t, db: db, log: log, runs: runs, cps: cps, led: led,
		sk: sk, sched: sched, sur: sk.Surface(), review: rev,
		artifactRoot: filepath.Join(root, "artifacts")}
}

func TestVerifyAbortParity(t *testing.T) {
	const owner = "u-operator"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seam := &abortRevise{cancel: cancel}
	h := newReviseSeamHarness(t, seam, "SINET_STAGE_FAKE_JUDGE=always-fail")

	taskID, verifyRunID, askID, _ := driveToOpenCapHit(t, h, owner)

	// The requester answers the card and their page goes away mid-rework: the
	// resumed drain's session dies because its caller did.
	seam.arm()
	answer := json.RawMessage(`{"choice":"revise_with_guidance","guidance":[{"text":"apply the agreed marker line","criterion":"AC-1"}]}`)
	_, err := h.sur.Answer(ctx, owner, askID, answer, false)
	if err == nil {
		t.Fatal("the resumed drain must fail loudly — the request really did fail")
	}
	if ctx.Err() == nil {
		t.Fatal("the request context is still live — this test is not pinning the abort shape")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("resumed-verify error = %v, want the aborted rework named", err)
	}

	if got := h.runState(t, verifyRunID); got != string(run.StateCrashed) {
		t.Fatalf("verify run is %s, want crashed — a dead resumed drain leaves the ladder a corpse (R5b/§16)", got)
	}
	if status, _ := h.askStatus(t, askID); status != "answered" {
		t.Fatalf("ask status %q, want answered — the answered card stays closed behind the crash", status)
	}
	var openAsks int
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM asks a JOIN runs r ON r.run_id = a.run_id
		  WHERE r.task_id = ? AND a.status = 'open'`, taskID).Scan(&openAsks); err != nil {
		t.Fatalf("count open asks: %v", err)
	}
	if openAsks != 0 {
		t.Fatalf("%d open asks after the aborted rework, want none", openAsks)
	}
}

package stage_test

// forkheal_e2e_test.go — P3-RW-6 §7 packet A: a crashed intake run's
// recovery fork must actually SUPERSEDE its parent (Spec S02.5 steps 2–4).
//
// The defect these tests pin: `dispatchIntake` transitioned the FORK to running
// and then drove the pipeline by TASK id, so the pipeline kept driving the
// state's baked `<task>.intake` — the crashed parent. Every generation's
// dispatch died with "run is not running", the ladder re-forked, and the
// lineage burned to a tombstone with everything needed to heal sitting in the
// event log (the live-world t-80eff shape).
//
// The harness is the real spine end to end: real intake pipeline, real
// scheduler claim+dispatch (Tick), real recovery ladder (ReconcilePass with a
// fake clock past the S02.5 reap bound, the CONVENTIONS §54 fixture note), real
// run FSM. The crash WINDOW is produced by real code too: the planning seam
// fails once mid-advance, exactly as a process death mid-advance leaves the run
// — running, no open gate, nobody driving it.
//
// Zero paid calls: the planning seam is a fake, the critic rides the package's
// fake engine (SINET_STAGE_FAKE), and the local duty seams ride local's fake
// server ($0 by construction).

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// ---- capture seams (S06.10) ----

// capturePlanner is the Stage-1 seam as a recorder: it captures the CONSUMING
// run id the pipeline hands it (P3-RW-6 R7 — on a forked run that must be the
// fork, never the superseded parent) and can fail its first N calls, which is
// how these tests produce an honest crash window.
type capturePlanner struct {
	mu        sync.Mutex
	failFirst int
	draftRuns []string
	revRuns   []string
	draftErr  error
}

func (p *capturePlanner) Draft(_ context.Context, in intake.DraftInput) (intake.Pair, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failFirst > 0 {
		p.failFirst--
		p.draftErr = errors.New("planning session died mid-advance (simulated process death)")
		return intake.Pair{}, p.draftErr
	}
	p.draftRuns = append(p.draftRuns, in.RunID)
	return forkPair(in), nil
}

func (p *capturePlanner) Revise(_ context.Context, in intake.ReviseInput) (intake.Pair, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.revRuns = append(p.revRuns, in.RunID)
	return in.Pair, nil
}

func (p *capturePlanner) runs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append(append([]string{}, p.draftRuns...), p.revRuns...)
}

// forkPair is an honest S06.6 pair that carries the interview's ANSWERED slots
// forward, so a re-drafted phase can be checked for lost answers (R10).
func forkPair(in intake.DraftInput) intake.Pair {
	var assumptions []intake.Assumption
	for _, r := range in.Resolutions {
		switch r.How {
		case intake.ResolvedAssumption:
			assumptions = append(assumptions, intake.Assumption{Text: r.Assumption, Origin: "slot:" + r.SlotID})
		case intake.ResolvedAnswered:
			assumptions = append(assumptions, intake.Assumption{
				Text: "answered " + r.SlotID + ": " + r.Value, Origin: "slot:" + r.SlotID})
		}
	}
	return intake.Pair{
		Spec: intake.Spec{
			Provenance:  "capture-planner v1",
			Restatement: "Requester wants: " + in.Request.Title,
			Outcome:     []string{"the requester recognizes their goal"},
			ACs: []intake.AC{
				{N: 1, Plain: "the note exists", Structured: "WHEN read THEN it names SQLite", StructuredKind: "ears"},
				{N: 2, Plain: "the result is verified"},
			},
			Constraints: []string{"stay within the repo"},
			Assumptions: assumptions,
			OutOfScope:  []string{"no deploys"},
		},
		Plan: intake.Plan{
			Provenance: "capture-planner v1",
			// The [A15] approach is required at the boundary (P3-GF8 R11).
			Steps: []intake.Step{
				{ID: "S-1", Title: "Write the note", DoneWhen: "the note exists", Class: "C1", WriteSet: []string{"src/**"},
					Approach: "I write the note into the workspace file and leave everything else alone."},
				{ID: "S-2", Title: "Verify against the criteria", DoneWhen: "all ACs demonstrably hold", Class: "C1",
					Approach: "I read each criterion against what the checks report."},
			},
			Coverage: map[string][]string{"AC-1": {"S-1"}, "AC-2": {"S-2"}},
			Risks:    []string{"estimate may be off"},
			Est:      intake.Estimate{SizeClass: "S", USD: 1.0, Known: true, Basis: "fake"},
		},
	}
}

// captureRouter records the consuming run id the S08.8 selection query carries
// (R7: the tie-break's D7 row rides it).
type captureRouter struct {
	mu   sync.Mutex
	seen []string
}

func (r *captureRouter) RouteTask(_ context.Context, q intake.RouteQuery) (intake.RouteBlock, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, q.RunID)
	return intake.RouteBlock{
		Cause: "no-fit-generalist", Generalist: true, Model: "fake-model", Lane: "anthropic",
		WindowTokens: 200000, PlainReason: "test posture",
	}, nil
}

func (r *captureRouter) runs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.seen...)
}

// ---- the crash-fork harness (packet A owns it; packet B reuses it) ----

type forkHarness struct {
	*harness
	reg     *settings.Registry
	ladder  *recovery.Ladder
	planner *capturePlanner
	router  *captureRouter
	fake    *local.FakeServer
	clock   time.Time

	// onboarding seam counters (T-A5): the S13.7 register/clone/scan/draft and
	// the activation, stubbed — this packet tests the ASK's identity, not git.
	onboardStarts    int
	onboardApprovals int
}

func newForkHarness(t *testing.T) *forkHarness {
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

	// The S12.1 class-(b) duty seams against local's fake /v1 — $0 by
	// construction, and their D7 rows are the evidence that the utility and
	// spot-check calls ride the CONSUMING run (R7).
	fake := local.NewFakeServer()
	t.Cleanup(fake.Close)
	fake.SetModelResponse("Qwen3.5-9B", local.FakeResponse{
		Content: `{"what":"w","wrong":"x","recommend":"y"}`, InputTokens: 40, OutputTokens: 8})
	fake.SetModelResponse("Qwen3.5-4B", local.FakeResponse{
		Content: `{"reason":"covered","uncovered":[],"abstain":false}`, InputTokens: 30, OutputTokens: 4})
	duty := local.NewDuty(local.DutyDeps{
		Registry: local.NewRegistry(reg), Client: local.NewClient(fake.URL),
		Checkpoints: cps, Events: log,
	})

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	root := t.TempDir()
	planner := &capturePlanner{}
	fh := &forkHarness{reg: reg, planner: planner, router: &captureRouter{}, fake: fake, clock: time.Now()}
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Ledger: led, Settings: reg,
		Adapters: map[string]adapters.Adapter{
			adapters.SubstrateClaudeCLI: &claudecli.Adapter{
				Binary:      self,
				HookCmd:     "/opt/sinet/bin/sinet engine-hook",
				Settings:    reg,
				Env:         append(os.Environ(), "SINET_STAGE_FAKE=1"),
				CancelGrace: 500 * time.Millisecond,
			},
		},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
		CopyAsideDir: filepath.Join(root, "copy-aside"),
		// Planner is the recorder; the CRITIC stays unwired so the real
		// EngineCritic runs (its session's D7 row names the run it composed).
		Planner:   planner,
		Utility:   stage.NewLocalUtility(duty),
		SpotCheck: stage.NewLocalSpotCheck(duty),
		OnboardStart: func(_ context.Context, projectID, _, _, _ string) (json.RawMessage, error) {
			fh.onboardStarts++
			return json.RawMessage(`{"project":"` + projectID + `","conventions":["c"]}`), nil
		},
		OnboardApprove: func(context.Context, string, string, json.RawMessage) error {
			fh.onboardApprovals++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	sk.Pipeline().Router = fh.router

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

	ladder, err := recovery.New(recovery.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Effects: journal, Settings: reg,
		Now: func() time.Time { return fh.clock },
	})
	if err != nil {
		t.Fatalf("recovery.New: %v", err)
	}
	fh.ladder = ladder
	fh.harness = &harness{t: t, db: db, log: log, runs: runs, cps: cps, led: led,
		sk: sk, sched: sched, sur: sk.Surface(), artifactRoot: filepath.Join(root, "artifacts")}
	return fh
}

// forkOf drives ONE recovery pass past the S02.5 reap bound (CONVENTIONS §54:
// a pass asserting a reap places its clock past max(lease, lastEvent +
// ⚙ recovery.dead_after) + grace) and returns the fork the ladder created.
func (h *forkHarness) forkOf(runID string) string {
	h.t.Helper()
	h.clock = h.clock.Add(30 * time.Minute) // > dead_after(300s) + wake_grace(120s), < stale_finalize(24h)
	rpt, err := h.ladder.ReconcilePass(context.Background())
	if err != nil {
		h.t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.Forked != 1 {
		h.t.Fatalf("pass forked %d runs, want exactly 1 (report %+v)", rpt.Forked, rpt)
	}
	parent, err := h.runs.Get(context.Background(), runID)
	if err != nil {
		h.t.Fatalf("parent run: %v", err)
	}
	if parent.State != run.StateCrashed {
		h.t.Fatalf("parent %s is %s, want crashed", runID, parent.State)
	}
	forkID := fmt.Sprintf("%s.g%d", runID, parent.Generation)
	fork, err := h.runs.Get(context.Background(), forkID)
	if err != nil {
		h.t.Fatalf("fork %s: %v", forkID, err)
	}
	if fork.State != run.StateQueued || fork.ParentRunID != runID {
		h.t.Fatalf("fork %s state=%s parent=%q, want queued with parent %s", forkID, fork.State, fork.ParentRunID, runID)
	}
	return forkID
}

// ---- small readers ----

func (h *forkHarness) runState(runID string) run.State {
	h.t.Helper()
	r, err := h.runs.Get(context.Background(), runID)
	if err != nil {
		h.t.Fatalf("run %s: %v", runID, err)
	}
	return r.State
}

// firstStateEvent returns the FIRST intake.state event on a run — on a fork
// that is the rebind (R2).
func (h *forkHarness) firstStateEvent(runID string) (payload map[string]any, generation int64, ok bool) {
	h.t.Helper()
	var raw string
	err := h.db.QueryRowContext(context.Background(),
		`SELECT payload, generation FROM run_events WHERE run_id = ? AND type = ?
		  ORDER BY event_seq LIMIT 1`, runID, intake.EventState).Scan(&raw, &generation)
	if err != nil {
		return nil, 0, false
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		h.t.Fatalf("decode intake.state: %v", err)
	}
	return payload, generation, true
}

func (h *forkHarness) countEvents(runID, typ string) int {
	h.t.Helper()
	var n int
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM run_events WHERE run_id = ? AND type = ?`, runID, typ).Scan(&n); err != nil {
		h.t.Fatalf("count events: %v", err)
	}
	return n
}

func (h *forkHarness) crashEvents(runID string) int {
	h.t.Helper()
	var n int
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM run_events WHERE run_id = ? AND type = ?
		   AND json_extract(payload, '$.to') = 'crashed'`, runID, run.EventState).Scan(&n); err != nil {
		h.t.Fatalf("count crash events: %v", err)
	}
	return n
}

func (h *forkHarness) checkpoints(runID string) int {
	h.t.Helper()
	var n int
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM checkpoints WHERE run_id = ?`, runID).Scan(&n); err != nil {
		h.t.Fatalf("count checkpoints: %v", err)
	}
	return n
}

func (h *forkHarness) askRunID(askID string) string {
	h.t.Helper()
	var runID string
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT run_id FROM asks WHERE ask_id = ?`, askID).Scan(&runID); err != nil {
		h.t.Fatalf("ask %s: %v", askID, err)
	}
	return runID
}

func (h *forkHarness) openAskID(taskID string) string {
	h.t.Helper()
	raw, err := h.sur.Task(context.Background(), taskID)
	if err != nil {
		h.t.Fatalf("Task: %v", err)
	}
	return decodeView(h.t, raw).OpenAskID
}

// walkToCrashWindow submits a request, answers the first interview card with a
// real answer PLUS force-proceed, and lets the planning seam die once — the
// exact beat the incident died on. Since P3-RW-9 R4 the surface leaves the
// corpse itself, so the window opens CRASHED (with its cause) rather than
// stranded `running`; the ladder forks it either way.
// Returns (taskID, intakeRunID, answeredValue).
func (h *forkHarness) walkToCrashWindow(ctx context.Context, owner string) (string, string, string) {
	h.t.Helper()
	raw, err := h.sur.Submit(ctx, owner, json.RawMessage(
		`{"title":"SQLite note","text":"Write a short appreciation note about the SQLite database engine."}`))
	if err != nil {
		h.t.Fatalf("Submit: %v", err)
	}
	v := decodeView(h.t, raw)
	taskID := v.TaskID
	if n := h.tick(ctx); n != 1 {
		h.t.Fatalf("tick dispatched %d, want the intake run", n)
	}
	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		h.t.Fatalf("Task: %v", err)
	}
	raw = clearFamilyGate(h.t, ctx, h.sur, owner, raw) // P3-RW-11: the family question comes first
	v = decodeView(h.t, raw)
	if v.OpenCard.Kind != "interview" || len(v.OpenCard.Questions) == 0 {
		h.t.Fatalf("expected an open interview card, got %s", raw)
	}
	const answered = "the-answer-from-card-one"
	answer := map[string]any{
		"answers":       []map[string]string{{"id": v.OpenCard.Questions[0].ID, "value": answered}},
		"force_proceed": true,
	}
	body, err := json.Marshal(answer)
	if err != nil {
		h.t.Fatal(err)
	}
	// The planning seam dies on this advance: the ask is already closed and the
	// run already resumed (both committed by closeAndResume), so the drive dies
	// with the run past the resume commit — the crash window.
	//
	// P3-RW-9 R4 changed what that window LOOKS like, not that it exists: the
	// stage surface now leaves the corpse itself instead of returning the error
	// and stranding the run `running`, driver-less and unscannable. The ladder
	// still forks it (crashed runs are swept every pass, Spec S02.5 step 3), so
	// every fork assertion below is unchanged — only the state the window opens
	// in moved, from "running with nobody driving it" to "crashed with its
	// cause".
	h.planner.failFirst = 1
	if _, err := h.sur.Answer(ctx, owner, v.OpenAskID, body, false); err == nil {
		h.t.Fatal("expected the answer's advance to fail with the dying planning seam")
	}
	intakeRun := taskID + ".intake"
	if got := h.runState(intakeRun); got != run.StateCrashed {
		h.t.Fatalf("crash window: run %s is %s, want crashed (P3-RW-9 R4: the dead advance leaves a corpse)", intakeRun, got)
	}
	return taskID, intakeRun, answered
}

// ---- T-A1 (checklist 1/2/3/6) ----

func TestIntakeCrashForkHealsEndToEnd(t *testing.T) {
	h := newForkHarness(t)
	ctx := context.Background()
	const owner = "u-fork"

	taskID, intakeRun, answered := h.walkToCrashWindow(ctx, owner)
	parentBefore, err := h.runs.Get(ctx, intakeRun)
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	parentGen := parentBefore.Generation

	// The ladder reaps the interrupted run and forks it (S02.5 steps 2–3).
	forkID := h.forkOf(intakeRun)
	forkRun, err := h.runs.Get(ctx, forkID)
	if err != nil {
		t.Fatalf("fork: %v", err)
	}

	// Fence (R5/checklist 6): the parent's stale keeper renews NOTHING — its
	// row generation was bumped by the fork tx and it is crashed.
	renewed := false
	if err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		ok, err := h.runs.RenewLeaseTx(ctx, tx, intakeRun, "intake-advance", h.clock.Add(time.Hour), parentGen)
		renewed = ok
		return err
	}); err != nil {
		t.Fatalf("RenewLeaseTx: %v", err)
	}
	if renewed {
		t.Fatal("the crashed parent's stale keeper renewed its lease — the S02.5 step-4 fence is not holding")
	}

	// The fork is claimed and dispatched: it REBINDS and issues the next card.
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the fork", n)
	}
	if got := h.crashEvents(forkID); got != 0 {
		t.Fatalf("the fork crashed %d times — it must take over, not re-die", got)
	}
	if got := h.runState(forkID); got != run.StateParked {
		t.Fatalf("fork state = %s, want parked on its new card", got)
	}

	// R2: ONE rebind state event, on the FORK's run id, at the FORK's
	// generation, whose payload names the fork.
	payload, gen, ok := h.firstStateEvent(forkID)
	if !ok {
		t.Fatal("no intake.state event on the fork — the pipeline never rebound onto it")
	}
	if payload["run_id"] != forkID {
		t.Fatalf("rebind event payload run_id = %v, want %s", payload["run_id"], forkID)
	}
	if gen != forkRun.Generation {
		t.Fatalf("rebind event generation = %d, want the fork's %d", gen, forkRun.Generation)
	}

	// Checklist 1: the NEXT card continues the numbering and belongs to the fork.
	// Card 1 is the P3-RW-11 family question and card 2 the interview this walk
	// answered, so the continuing card is 3 — the property under test is that the
	// numbering CONTINUES across the fork, and it is still asserted exactly.
	askID := h.openAskID(taskID)
	if want := fmt.Sprintf("intake:%s:3", taskID); askID != want {
		t.Fatalf("open ask = %q, want the continuing card %q", askID, want)
	}
	if got := h.askRunID(askID); got != forkID {
		t.Fatalf("ask row run_id = %q, want the fork %q", got, forkID)
	}

	// Checklist 2: answering it completes the interview on the FORK.
	raw, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"action":"approve"}`), true)
	if err != nil {
		t.Fatalf("Answer(approve): %v", err)
	}
	v := decodeView(t, raw)
	if v.Phase != "approved" {
		t.Fatalf("phase = %q, want approved", v.Phase)
	}
	if got := h.runState(forkID); got != run.StateCompleted {
		t.Fatalf("fork state = %s, want completed", got)
	}
	if got := h.runState(intakeRun); got != run.StateCrashed {
		t.Fatalf("parent state = %s, want crashed (superseded, never revived)", got)
	}
	var queueStatus string
	if err := h.db.QueryRowContext(ctx,
		`SELECT status FROM queue WHERE run_id = ?`, forkID).Scan(&queueStatus); err != nil {
		t.Fatalf("fork queue row: %v", err)
	}
	if queueStatus != "done" {
		t.Fatalf("fork queue row = %q, want done (settled at run end, S10.1)", queueStatus)
	}
	execRun := taskID + ".execute"
	if got := h.runState(execRun); got != run.StateQueued {
		t.Fatalf("execute run state = %s, want queued (launched by the healed intake)", got)
	}

	// Checklist 3 (R10): the answer given on card 1 survived the fork — it is in
	// the APPROVED spec, and it was never asked again.
	pair, _, err := h.sk.Pipeline().ApprovedPair(ctx, taskID)
	if err != nil {
		t.Fatalf("ApprovedPair: %v", err)
	}
	found := false
	for _, a := range pair.Spec.Assumptions {
		if strings.Contains(a.Text, answered) {
			found = true
		}
	}
	if !found {
		t.Fatalf("the answer from card 1 is missing from the approved spec: %+v", pair.Spec.Assumptions)
	}
	var reasked int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM asks WHERE ask_id LIKE ? AND json_extract(snapshot, '$.kind') = 'interview'`,
		"intake:"+taskID+":%").Scan(&reasked); err != nil {
		t.Fatalf("count interview cards: %v", err)
	}
	if reasked != 1 {
		t.Fatalf("%d interview cards exist — the healed fork re-asked answered questions", reasked)
	}
}

// ---- T-A2 (checklist 4) ----

func TestPostApprovalCrashForkFinishesIntake(t *testing.T) {
	h := newForkHarness(t)
	ctx := context.Background()
	const owner = "u-postapproval"

	taskID, intakeRun, _ := h.walkToCrashWindow(ctx, owner)
	// Heal once so the interview reaches its approval card (the fork drives it).
	forkA := h.forkOf(intakeRun)
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the fork", n)
	}
	askID := h.openAskID(taskID)

	// Approve through the PIPELINE only: the approval transaction commits and
	// the process dies before finishIntake — the run is left RUNNING at
	// PhaseApproved, which is the second broken shape (§1).
	st, err := h.sk.Pipeline().Answer(ctx, owner, askID, json.RawMessage(`{"action":"approve"}`))
	if err != nil {
		t.Fatalf("pipeline Answer(approve): %v", err)
	}
	if st.Phase != intake.PhaseApproved {
		t.Fatalf("phase = %s, want approved", st.Phase)
	}
	if got := h.runState(forkA); got != run.StateRunning {
		t.Fatalf("post-approval window: %s is %s, want running", forkA, got)
	}

	forkB := h.forkOf(forkA)
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the post-approval fork", n)
	}

	// The FORK completes itself; the parent stays crashed; nothing is tombstoned.
	if got := h.runState(forkB); got != run.StateCompleted {
		t.Fatalf("fork %s is %s, want completed (it finished its own intake)", forkB, got)
	}
	if got := h.runState(forkA); got != run.StateCrashed {
		t.Fatalf("superseded parent %s is %s, want crashed", forkA, got)
	}
	if got := h.runState(taskID + ".execute"); got != run.StateQueued {
		t.Fatalf("execute run is %s, want queued (execution launched from the fork)", got)
	}
	var tombstoned int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM runs WHERE state = 'tombstoned'`).Scan(&tombstoned); err != nil {
		t.Fatalf("count tombstones: %v", err)
	}
	if tombstoned != 0 {
		t.Fatalf("%d tombstoned runs — the ladder burned the lineage", tombstoned)
	}
}

// ---- T-A4 (checklist 7) ----

func TestForkEngineSessionsRideForkRunID(t *testing.T) {
	h := newForkHarness(t)
	ctx := context.Background()
	const owner = "u-seams"

	taskID, intakeRun, _ := h.walkToCrashWindow(ctx, owner)
	parentCheckpoints := h.checkpoints(intakeRun)

	forkID := h.forkOf(intakeRun)
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the fork", n)
	}
	baked := taskID + ".intake"

	// Planner + router seams: the consuming run id is the fork's, never the
	// suffix-composed parent.
	for _, id := range h.planner.runs() {
		if id != forkID {
			t.Fatalf("planner seam consumed run id %q, want the fork %q", id, forkID)
		}
	}
	if len(h.planner.runs()) == 0 {
		t.Fatal("the planner seam was never called on the fork")
	}
	routed := h.router.runs()
	if len(routed) == 0 {
		t.Fatal("the routing seam was never called on the fork")
	}
	for _, id := range routed {
		if id != forkID {
			t.Fatalf("routing query carried run id %q, want the fork %q (never %q)", id, forkID, baked)
		}
	}

	// Critic (a real engine session) + the local utility/spot-check duties: their
	// D7 rows are written on the run they name. Post-fork work must land on the
	// fork, and the superseded parent must gain nothing.
	if got := h.checkpoints(forkID); got == 0 {
		t.Fatal("no checkpoint on the fork — no seam wrote its D7 row on the run that did the work")
	}
	if got := h.checkpoints(intakeRun); got != parentCheckpoints {
		t.Fatalf("the superseded parent gained %d checkpoints after the fork", got-parentCheckpoints)
	}
	if got := h.countEvents(forkID, local.EventLocalUnmeteredDefect); got != 0 {
		t.Fatalf("%d local.unmetered_defect events — a duty call named an uncheckpointable run", got)
	}
	var defects int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_events WHERE type = ?`, local.EventLocalUnmeteredDefect).Scan(&defects); err != nil {
		t.Fatalf("count defects: %v", err)
	}
	if defects != 0 {
		t.Fatalf("%d local.unmetered_defect events platform-wide", defects)
	}
}

// ---- T-A5 (checklist 8) ----

func TestOnboardForkAskRebindsAndAnswers(t *testing.T) {
	h := newForkHarness(t)
	ctx := context.Background()
	const owner = "u-onboard"

	taskID, err := h.sk.StartOnboarding(ctx, owner, "shop", "shop", "")
	if err != nil {
		t.Fatalf("StartOnboarding: %v", err)
	}
	onboardRun := taskID + ".onboard"
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the onboarding run", n)
	}
	askID := stage.OnboardAskID("shop")
	if got := h.askRunID(askID); got != onboardRun {
		t.Fatalf("ask run_id = %q, want %q", got, onboardRun)
	}

	// The platform resumed the parked run and then died mid-dispatch: the run is
	// running with its ask still open — the window the ladder reaps.
	if _, err := h.runs.Transition(ctx, onboardRun, run.StateRunning, run.TransitionOptions{
		Reason: "test: the platform resumed this run and then died", Actor: run.ActorPlatform,
	}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	forkID := h.forkOf(onboardRun)
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the onboarding fork", n)
	}

	// R11: the re-issued ask is bound to the FORK.
	if got := h.askRunID(askID); got != forkID {
		t.Fatalf("ask row run_id = %q, want the fork %q (the upsert never rebound it)", got, forkID)
	}
	if _, err := h.sk.AnswerOnboarding(ctx, owner, askID, json.RawMessage(`{"approve":true}`)); err != nil {
		t.Fatalf("AnswerOnboarding: %v", err)
	}
	if h.onboardApprovals != 1 {
		t.Fatalf("entry activated %d times, want 1", h.onboardApprovals)
	}
	if got := h.runState(forkID); got != run.StateCompleted {
		t.Fatalf("fork %s is %s, want completed by the answer", forkID, got)
	}
}

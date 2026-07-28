package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// hint_pause_test.go — the S15.5 drag hint and the S10.4 pause switch, at the
// place both of them actually bite: the claim pass (P3-B6-2B, R10/R11/R12).

// ── fixtures ────────────────────────────────────────────────────────────────

// fakeBudgets / fakePause are the two B6-2B seams, hermetic and in-memory. Both
// are tiny on purpose: the behavior under test is the SCHEDULER's, and a store
// with logic of its own would blur which side was being proven.
type fakeBudgets map[string]metering.Budget

func (f fakeBudgets) Budget(_ context.Context, userID, lane string) (metering.Budget, error) {
	if b, ok := f[userID+"/"+lane]; ok {
		return b, nil
	}
	return metering.UndeclaredBudget(), nil
}

type fakePause map[string]bool

func (f fakePause) Paused(_ context.Context, userID string) (bool, error) { return f[userID], nil }

// holdingScheduler builds a scheduler whose dispatcher BLOCKS in `running`, so
// lane-slot occupancy is observable and a claim pass admits exactly as many runs
// as there are free slots.
func holdingScheduler(t *testing.T, e *schedEnv, budgets BudgetReader, pause PauseReader) (*Scheduler, *fakeDispatcher) {
	t.Helper()
	d := &fakeDispatcher{runs: e.runs, cps: e.cps, hold: true, release: make(chan struct{})}
	s, err := New(Config{
		DB: e.db, Runs: e.runs, Settings: e.reg, Dispatcher: d,
		Pressure: metering.NewPressureGauge(e.db, e.reg),
		Budgets:  budgets, Pause: pause,
		Logger: discardLogger(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		select {
		case <-d.release:
		default:
			close(d.release)
		}
		s.WaitInFlight()
	})
	return s, d
}

// queueRow reads a queue row's full claim-loop state — the bytes P-T08-4 says a
// pause must not disturb.
type queueRow struct {
	status   string
	lane     string
	enqueued string
	hintRank int64
}

func readQueueRow(t *testing.T, e *schedEnv, runID string) queueRow {
	t.Helper()
	var q queueRow
	if err := e.db.QueryRowContext(context.Background(),
		`SELECT status, priority_lane, enqueued_ts, hint_rank FROM queue WHERE run_id = ?`, runID).
		Scan(&q.status, &q.lane, &q.enqueued, &q.hintRank); err != nil {
		t.Fatalf("read queue row %s: %v", runID, err)
	}
	return q
}

// ── R10: the drag hint reorders one person's OWN same-class queued work ─────

// TestDragHintMovesARowInTheClaimOrder is the claim-order proof, run as a
// CONTROL and a TREATMENT over identical fixtures: two of alice's background
// runs on a one-slot lane, r-first enqueued before r-second. Without a hint the
// older one is claimed; with a hint on the younger one, the younger one is
// claimed. The hint is what moved the row, and nothing else differs.
func TestDragHintMovesARowInTheClaimOrder(t *testing.T) {
	for _, tc := range []struct {
		name      string
		hintRun   string
		wantFirst string
	}{
		{"control: no hint, the older run is claimed", "", "r-first"},
		{"treatment: the hint promotes the younger run", "r-second", "r-second"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newSchedEnv(t)
			s, _ := holdingScheduler(t, e, nil, nil)
			ctx := context.Background()

			e.newRun(t, "r-first", "alice", "lane-a")
			if err := s.Enqueue(ctx, "r-first", ClassBackground); err != nil {
				t.Fatal(err)
			}
			// A real gap in enqueue time, so the aging score genuinely favors
			// r-first and the control is not decided by a tiebreak.
			backdateEnqueue(t, e, "r-first", -time.Hour)
			e.newRun(t, "r-second", "alice", "lane-a")
			if err := s.Enqueue(ctx, "r-second", ClassBackground); err != nil {
				t.Fatal(err)
			}
			if tc.hintRun != "" {
				applied, err := s.SetPriorityHint(ctx, tc.hintRun, -5)
				if err != nil {
					t.Fatal(err)
				}
				if !applied {
					t.Fatal("SetPriorityHint reported no queued row — the fixture run is queued")
				}
			}
			n, err := s.Tick(ctx)
			if err != nil {
				t.Fatal(err)
			}
			// One slot (defaultLaneConcurrency), so exactly one run is admitted
			// and WHICH one is the whole question.
			if n != 1 {
				t.Fatalf("claim pass dispatched %d runs, want 1 (the lane holds one slot)", n)
			}
			waitState(t, e.runs, tc.wantFirst, run.StateRunning)
			other := "r-first"
			if tc.wantFirst == "r-first" {
				other = "r-second"
			}
			r, err := e.runs.Get(ctx, other)
			if err != nil {
				t.Fatal(err)
			}
			if r.State != run.StateQueued {
				t.Fatalf("%s is %s, want queued — the other run must still be waiting", other, r.State)
			}
		})
	}
}

// backdateEnqueue rewrites a queue row's enqueued_ts so a test can create a real
// age difference without sleeping.
func backdateEnqueue(t *testing.T, e *schedEnv, runID string, delta time.Duration) {
	t.Helper()
	if err := e.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`UPDATE queue SET enqueued_ts = ? WHERE run_id = ?`,
			time.Now().Add(delta).UTC().Format(time.RFC3339Nano), runID)
		return err
	}); err != nil {
		t.Fatalf("backdate %s: %v", runID, err)
	}
}

// TestDragHintNeverCrossesClassOrOwner is the pair of negatives the hint's
// safety rests on, asserted against the ordering function itself so the claim
// order cannot disagree with it.
func TestDragHintNeverCrossesClassOrOwner(t *testing.T) {
	now := time.Now()
	fresh := now.Add(-time.Minute)
	old := now.Add(-6 * time.Hour)

	// (a) A maximally-hinted BACKGROUND run never outranks interactive work.
	// Interactive is never starved by automation (3.3), and a person dragging
	// their own board cannot promote automation past it.
	hintedBackground := candidate{queueID: 1, runID: "r-bg", userID: "alice", lane: "l",
		class: ClassBackground, enqueuedTS: old, hintRank: -hintFloorProbe}
	freshInteractive := candidate{queueID: 2, runID: "r-int", userID: "alice", lane: "l",
		class: ClassInteractive, enqueuedTS: fresh}
	if priorityLess(hintedBackground, freshInteractive, now) {
		t.Error("a hinted background run outranked interactive work — the hint crossed the S10.7 class ladder")
	}
	if !priorityLess(freshInteractive, hintedBackground, now) {
		t.Error("interactive lost to a hinted background run — 3.3 says interactive is never starved by automation")
	}

	// (b) A maximally-hinted run of ANOTHER owner never reorders against mine.
	// Both are background, so only the hint could move them — and it must not,
	// because the hint orders one person's own queue and nobody else's.
	bobHinted := candidate{queueID: 3, runID: "r-bob", userID: "bob", lane: "l",
		class: ClassBackground, enqueuedTS: fresh, hintRank: -hintFloorProbe}
	aliceOld := candidate{queueID: 4, runID: "r-alice", userID: "alice", lane: "l",
		class: ClassBackground, enqueuedTS: old}
	if priorityLess(bobHinted, aliceOld, now) {
		t.Error("bob's hint outranked alice's older run — a drag reached across owners")
	}
	if !priorityLess(aliceOld, bobHinted, now) {
		t.Error("alice's older run lost to bob's hint — cross-owner ordering must still be the aging score")
	}
}

// hintFloorProbe is the strongest hint the transport admits, used here to prove
// the negatives hold even at the extreme.
const hintFloorProbe = 1000

// TestPriorityHintOnANonQueuedRunIsAnHonestNoOp: a claimed run has left the
// queue the hint sorts, so the write matches no row and says so.
func TestPriorityHintOnANonQueuedRunIsAnHonestNoOp(t *testing.T) {
	e := newSchedEnv(t)
	s, _ := holdingScheduler(t, e, nil, nil)
	ctx := context.Background()

	e.newRun(t, "r-1", "alice", "lane-a")
	if err := s.Enqueue(ctx, "r-1", ClassBackground); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	waitState(t, e.runs, "r-1", run.StateRunning)

	applied, err := s.SetPriorityHint(ctx, "r-1", -5)
	if err != nil {
		t.Fatalf("a hint on a moved-on run must not error: %v", err)
	}
	if applied {
		t.Error("the hint claimed to apply to a run that is no longer queued")
	}
	if got := readQueueRow(t, e, "r-1").hintRank; got != 0 {
		t.Errorf("hint_rank = %d on a claimed row, want 0 — a hint must never touch a leased row", got)
	}
}

// ── R12 / P-T08-4: pause stops admission and PRESERVES everything ───────────

// TestPausePreservesEverythingQueuedAndParked is the mandatory P-T08-4 test. A
// pause must stop admitting and change NOTHING else: the queue row is compared
// field-by-field before and after, and the parked run's state and generation are
// compared too. Then the resume proves the preserved work still runs.
func TestPausePreservesEverythingQueuedAndParked(t *testing.T) {
	e := newSchedEnv(t)
	pause := fakePause{}
	s, _ := holdingScheduler(t, e, nil, pause)
	ctx := context.Background()

	// One queued background run and one parked run, both alice's.
	e.newRun(t, "r-queued", "alice", "lane-a")
	if err := s.Enqueue(ctx, "r-queued", ClassBackground); err != nil {
		t.Fatal(err)
	}
	e.newRun(t, "r-parked", "alice", "lane-b")
	parkFixture(t, e, "r-parked")
	parkedBefore, err := e.runs.Get(ctx, "r-parked")
	if err != nil {
		t.Fatal(err)
	}
	before := readQueueRow(t, e, "r-queued")

	pause["alice"] = true
	n, err := s.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("claim pass dispatched %d runs for a paused person, want 0 (S10.4 admission stop)", n)
	}
	// PRESERVED: the queue row is byte-identical and the run never left queued.
	if after := readQueueRow(t, e, "r-queued"); after != before {
		t.Errorf("the pause disturbed the queue row: before %+v, after %+v — a pause preserves, it does not dequeue (P-T08-4)", before, after)
	}
	if r, _ := e.runs.Get(ctx, "r-queued"); r.State != run.StateQueued {
		t.Errorf("queued run is %s after a pause, want queued", r.State)
	}
	parkedAfter, err := e.runs.Get(ctx, "r-parked")
	if err != nil {
		t.Fatal(err)
	}
	if parkedAfter.State != parkedBefore.State || parkedAfter.Generation != parkedBefore.Generation {
		t.Errorf("the pause moved a parked run: %s/gen%d → %s/gen%d — nothing parked is touched (P-T08-4)",
			parkedBefore.State, parkedBefore.Generation, parkedAfter.State, parkedAfter.Generation)
	}

	// …and the preserved work proceeds the moment the switch comes back.
	pause["alice"] = false
	n, err = s.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("after the resume the claim pass dispatched %d runs, want 1 — the preserved queue must proceed", n)
	}
	waitState(t, e.runs, "r-queued", run.StateRunning)
}

// TestPauseNeverGatesInteractiveWork is S10.4's headroom rule: the switch stops
// AUTOMATION. A person who paused their automation has not locked themselves out
// of their own platform, and the claim pass must still admit their interactive
// work — including while background work of theirs is being held back.
func TestPauseNeverGatesInteractiveWork(t *testing.T) {
	e := newSchedEnv(t)
	pause := fakePause{"alice": true}
	s, _ := holdingScheduler(t, e, nil, pause)
	ctx := context.Background()

	e.newRun(t, "r-bg", "alice", "lane-a")
	if err := s.Enqueue(ctx, "r-bg", ClassBackground); err != nil {
		t.Fatal(err)
	}
	e.newRun(t, "r-int", "alice", "lane-b")
	if err := s.Enqueue(ctx, "r-int", ClassInteractive); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	waitState(t, e.runs, "r-int", run.StateRunning)
	if r, _ := e.runs.Get(ctx, "r-bg"); r.State != run.StateQueued {
		t.Errorf("background run is %s while its owner is paused, want queued", r.State)
	}
}

// TestPauseIsPerPerson: one person's switch never reaches another's work.
func TestPauseIsPerPerson(t *testing.T) {
	e := newSchedEnv(t)
	pause := fakePause{"alice": true}
	s, _ := holdingScheduler(t, e, nil, pause)
	ctx := context.Background()

	e.newRun(t, "r-alice", "alice", "lane-a")
	if err := s.Enqueue(ctx, "r-alice", ClassBackground); err != nil {
		t.Fatal(err)
	}
	e.newRun(t, "r-bob", "bob", "lane-a")
	if err := s.Enqueue(ctx, "r-bob", ClassBackground); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	waitState(t, e.runs, "r-bob", run.StateRunning)
	if r, _ := e.runs.Get(ctx, "r-alice"); r.State != run.StateQueued {
		t.Errorf("alice's run is %s, want queued — her pause must not free-ride on bob's admission", r.State)
	}
}

// ── R11: a declared budget makes the S10.4 gauge REAL at admission ──────────

// TestDeclaredBudgetMakesTheBackgroundAdmissionStopReal is the pre/post-
// declaration pressure test. The SAME consumption and the SAME scheduler admit
// background work with no budget declared — because with no denominator there is
// nothing to gate against (D4) — and refuse it once a budget makes the pressure
// meaningful. The gauge itself is unchanged; only where its Budget comes from
// changed.
func TestDeclaredBudgetMakesTheBackgroundAdmissionStopReal(t *testing.T) {
	consume := func(t *testing.T, e *schedEnv, runID string) {
		t.Helper()
		// A checkpoint is only writable on a running run (S02.4), so the fixture
		// walks the ratified edges to get there rather than poking the row.
		for _, to := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
			if _, err := e.runs.Transition(context.Background(), runID, to, run.TransitionOptions{
				Reason: "fixture", Actor: run.ActorPlatform,
			}); err != nil {
				t.Fatalf("fixture %s → %s: %v", runID, to, err)
			}
		}
		if _, err := e.cps.Write(context.Background(), gates.NewCheckpoint{
			RunID: runID, ModelID: "claude-haiku-4-5", SessionSubstrate: "claude-cli",
			Usage: json.RawMessage(`{"input_tokens":900,"output_tokens":100}`),
		}); err != nil {
			t.Fatalf("checkpoint: %v", err)
		}
		// …and it ends, so it holds no lane slot: the only thing under test is
		// whether the DENOMINATOR gates the next run, never concurrency.
		if _, err := e.runs.Transition(context.Background(), runID, run.StateCompleted, run.TransitionOptions{
			Reason: "fixture", Actor: run.ActorPlatform,
		}); err != nil {
			t.Fatalf("fixture %s → completed: %v", runID, err)
		}
	}
	for _, tc := range []struct {
		name           string
		budgets        fakeBudgets
		wantDispatched int
	}{
		{
			name:           "pre-declaration: nothing to gate against, so background admits",
			budgets:        fakeBudgets{},
			wantDispatched: 1,
		},
		{
			// 1000 weighted units consumed against a 1000-unit budget is
			// pressure 1.0, past ⚙ pressure.bg_admit_stop (0.7).
			name:           "post-declaration: the gauge is applicable and the stop bites",
			budgets:        fakeBudgets{"alice/lane-a": {PeriodTokens: 1000, Declared: true}},
			wantDispatched: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newSchedEnv(t)
			s, _ := holdingScheduler(t, e, tc.budgets, nil)
			ctx := context.Background()

			// A prior run of alice's on the same lane, already consumed.
			e.newRun(t, "r-past", "alice", "lane-a")
			consume(t, e, "r-past")

			e.newRun(t, "r-next", "alice", "lane-a")
			if err := s.Enqueue(ctx, "r-next", ClassBackground); err != nil {
				t.Fatal(err)
			}
			n, err := s.Tick(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if n != tc.wantDispatched {
				t.Fatalf("dispatched %d, want %d", n, tc.wantDispatched)
			}
		})
	}
}

// parkFixture puts a fresh run into `parked` through the ratified edges, so the
// preservation test compares against a real parked run rather than a hand-poked
// row.
func parkFixture(t *testing.T, e *schedEnv, runID string) {
	t.Helper()
	ctx := context.Background()
	for _, to := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning, run.StateParked} {
		if _, err := e.runs.Transition(ctx, runID, to, run.TransitionOptions{
			Reason: "fixture", Actor: run.ActorPlatform,
		}); err != nil {
			t.Fatalf("park fixture %s → %s: %v", runID, to, err)
		}
	}
}

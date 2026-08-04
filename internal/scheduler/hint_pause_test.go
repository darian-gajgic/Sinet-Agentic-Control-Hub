package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
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

// ── D1: the ordering is a permutation, and these are its properties ────────
//
// The hint used to be a comparator limb, which was not a strict weak ordering:
// a hint order disagreeing with the aging order, plus an outsider scoring
// between the two, produced a genuine CYCLE, and sort.SliceStable on a cyclic
// relation is undefined — three initial row orders gave three different claim
// winners, one of them the other owner's run. The tests below pin the four
// properties the replacement is built to have, each of them a property of the
// CLAIM ORDER rather than of a pairwise comparison.

// hintFloorProbe is the strongest hint the transport admits, used here so the
// properties are asserted at the extreme rather than at a gentle nudge.
const hintFloorProbe = 1000

// orderingFixture is a scenario plus the claim order it produces.
type orderingCand struct {
	id      string
	user    string
	class   WorkloadClass
	ageHrs  float64
	hint    int64
	queueID int64
}

func buildCands(specs []orderingCand, now time.Time) []candidate {
	out := make([]candidate, 0, len(specs))
	for i, s := range specs {
		qid := s.queueID
		if qid == 0 {
			qid = int64(i + 1)
		}
		out = append(out, candidate{
			queueID: qid, runID: s.id, userID: s.user, lane: "l", class: s.class,
			enqueuedTS: now.Add(-time.Duration(s.ageHrs * float64(time.Hour))), hintRank: s.hint,
		})
	}
	return out
}

// claimOrder sorts a COPY of the scenario and returns the run ids in order.
func claimOrder(specs []orderingCand, now time.Time) []string {
	cands := buildCands(specs, now)
	sortCandidates(cands, now)
	return runIDs(cands)
}

// TestClaimOrderIsIndependentOfRowOrder is property (a): the ordering is
// transitive, so the claim order is a function of the CANDIDATES and not of the
// order the rows happened to come back from SQLite in. This is the property the
// comparator limb did not have, and it is asserted over the exact fixture that
// broke it — plus every permutation of it.
func TestClaimOrderIsIndependentOfRowOrder(t *testing.T) {
	now := time.Now()
	// The F1 cycle fixture: within (alice, background) the hint order disagrees
	// with the aging order, and bob's run scores BETWEEN the two.
	specs := []orderingCand{
		{id: "alice-old", user: "alice", class: ClassBackground, ageHrs: 3},
		{id: "alice-new", user: "alice", class: ClassBackground, ageHrs: 1, hint: -5},
		{id: "bob-mid", user: "bob", class: ClassBackground, ageHrs: 2},
	}
	want := claimOrder(specs, now)
	// Under the old comparator this fixture produced a different winner per row
	// order; the winner must now be one run, whichever order the rows arrive in.
	for _, perm := range permutations(len(specs)) {
		shuffled := make([]orderingCand, len(specs))
		for i, p := range perm {
			shuffled[i] = specs[p]
		}
		if got := claimOrder(shuffled, now); !equalStrings(got, want) {
			t.Fatalf("row order %v produced claim order %v, want %v — the ordering is not transitive", perm, got, want)
		}
	}
	// …and the hint did what it was asked to do: alice's hinted run took her
	// group's best position.
	if want[0] != "alice-new" {
		t.Fatalf("claim order = %v, want alice's hinted run first", want)
	}
}

// TestHintIsHonoredWithinTheOwnersOwnGroup is property (d).
func TestHintIsHonoredWithinTheOwnersOwnGroup(t *testing.T) {
	now := time.Now()
	specs := []orderingCand{
		{id: "a1", user: "alice", class: ClassBackground, ageHrs: 5},
		{id: "a2", user: "alice", class: ClassBackground, ageHrs: 3},
		{id: "a3", user: "alice", class: ClassBackground, ageHrs: 1, hint: -hintFloorProbe},
	}
	// Baseline: oldest first.
	if got := claimOrder([]orderingCand{specs[0], specs[1], {id: "a3", user: "alice", class: ClassBackground, ageHrs: 1}}, now); got[0] != "a1" {
		t.Fatalf("baseline claim order = %v, want the oldest first", got)
	}
	got := claimOrder(specs, now)
	if got[0] != "a3" {
		t.Fatalf("claim order = %v, want the hinted run first within its owner's own group", got)
	}
	// The rest keep their own relative order: a hint moves ONE run, it does not
	// scramble the queue.
	if got[1] != "a1" || got[2] != "a2" {
		t.Fatalf("claim order = %v, want the un-hinted runs still oldest-first behind it", got)
	}
}

// TestHintCannotMoveAnotherOwnersPosition is property (b), the cross-user
// SET-invariance: a group's multiset of keys is preserved, so every other owner
// faces the identical field of competitors and their positions cannot move.
func TestHintCannotMoveAnotherOwnersPosition(t *testing.T) {
	now := time.Now()
	base := []orderingCand{
		{id: "alice-1", user: "alice", class: ClassBackground, ageHrs: 6},
		{id: "bob-1", user: "bob", class: ClassBackground, ageHrs: 5},
		{id: "alice-2", user: "alice", class: ClassBackground, ageHrs: 4},
		{id: "bob-2", user: "bob", class: ClassBackground, ageHrs: 3},
		{id: "alice-3", user: "alice", class: ClassBackground, ageHrs: 2},
		{id: "bob-3", user: "bob", class: ClassBackground, ageHrs: 1},
	}
	baseline := claimOrder(base, now)

	hinted := append([]orderingCand(nil), base...)
	hinted[4].hint = -hintFloorProbe // alice drags her newest run to the top
	hinted[0].hint = hintFloorProbe  // …and her oldest to the bottom
	after := claimOrder(hinted, now)

	for i := range baseline {
		wasBob := strings.HasPrefix(baseline[i], "bob-")
		isBob := strings.HasPrefix(after[i], "bob-")
		if wasBob != isBob || (wasBob && baseline[i] != after[i]) {
			t.Fatalf("alice's drag moved bob's work: baseline %v, after %v (position %d)", baseline, after, i)
		}
	}
}

// TestHintNeverCrossesAClassBoundary is property (c), for interactive AND
// non-interactive neighbours.
//
// GUARD-REMOVAL PROBE: the permutation groups by (user, CLASS). Group by user
// alone — the plausible mistake — and alice's hinted background run would take
// the key of her own scheduled run and jump it, which is exactly what these
// assertions catch.
func TestHintNeverCrossesAClassBoundary(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name      string
		neighbour WorkloadClass
	}{
		{"interactive is never starved by automation (3.3)", ClassInteractive},
		{"a hinted background run never jumps its owner's human-blocked work", ClassHumanBlocked},
		{"a hinted background run never jumps its owner's scheduled work", ClassScheduled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			specs := []orderingCand{
				// The neighbour is BRAND NEW, so only a boundary crossing could
				// put a background run ahead of it.
				{id: "neighbour", user: "alice", class: tc.neighbour, ageHrs: 0},
				{id: "bg-old", user: "alice", class: ClassBackground, ageHrs: 0.5},
				{id: "bg-hinted", user: "alice", class: ClassBackground, ageHrs: 0.1, hint: -hintFloorProbe},
			}
			got := claimOrder(specs, now)
			if got[0] != "neighbour" {
				t.Fatalf("claim order = %v, want the %s run first — a hint may never cross a class boundary", got, tc.neighbour)
			}
			// …and the hint still worked INSIDE the background group.
			if got[1] != "bg-hinted" {
				t.Fatalf("claim order = %v, want the hinted background run ahead of its own group", got)
			}
		})
	}
}

// TestPermutationWithNoHintsIsTheIdentity: an un-dragged queue admits exactly as
// it did before B6-2B. Without this the new machinery could silently change the
// ordering of every platform that has never used the feature.
func TestPermutationWithNoHintsIsTheIdentity(t *testing.T) {
	now := time.Now()
	specs := []orderingCand{
		{id: "i", user: "alice", class: ClassInteractive, ageHrs: 0},
		{id: "bg-old", user: "alice", class: ClassBackground, ageHrs: 5},
		{id: "sched", user: "bob", class: ClassScheduled, ageHrs: 0},
		{id: "bg-new", user: "bob", class: ClassBackground, ageHrs: 0},
		{id: "hb", user: "alice", class: ClassHumanBlocked, ageHrs: 0},
	}
	cands := buildCands(specs, now)
	permuteHints(cands, now)
	for _, c := range cands {
		if c.key != c.naturalKey(now) {
			t.Errorf("%s: key moved with no hint anywhere (%v → %v)", c.runID, c.naturalKey(now), c.key)
		}
	}
}

func permutations(n int) [][]int {
	if n == 0 {
		return [][]int{{}}
	}
	var out [][]int
	for _, rest := range permutations(n - 1) {
		for pos := 0; pos <= len(rest); pos++ {
			p := append([]int(nil), rest[:pos]...)
			p = append(p, n-1)
			p = append(p, rest[pos:]...)
			out = append(out, p)
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

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

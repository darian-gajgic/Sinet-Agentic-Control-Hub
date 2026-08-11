package recovery_test

// lease_liveness_test.go — P3-RW-5 acceptance battery (brief §8 T1–T3, T6–T9).
//
// The defect these pin: a live, human-paced interview run was reaped as DEAD
// while it was demonstrably advancing, because the lease is written once at
// claim time and classification let that dead lease outrank fresh event-cursor
// evidence. Spec S02.5 step 2 reads DEAD as "unit gone/failed with NOTHING
// NEWER to harvest"; Spec S02.3 derives stalled from the event cursor; the S18
// row for ⚙ recovery.dead_after is cursor-silence by its own help text. So
// under an unknowable body (UnitGone/UnitUnknown) DEAD is a CONJUNCTION —
// lease dead or absent AND cursor stale past ⚙ recovery.dead_after (+ the
// pass's wake grace) AND nothing terminal to harvest — while a failed unit
// corpse stays sufficient on its own (the body is conclusive).
//
// "Grace-neutral" below = one throwaway pass first, then clock deltas kept
// under 2× ⚙ recovery.sweep_interval, so ⚙ recovery.wake_grace is not applied
// (recovery.go's clock-jump wake detection).

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// The ratified G2 Def.2 numbers this battery reasons in (registry defaults;
// read here as literals only to compute expected instants, never consumed as
// behaviour).
const (
	deadAfter     = 300 * time.Second
	heartbeat     = 60 * time.Second
	wakeGrace     = 120 * time.Second
	sweepInterval = 300 * time.Second
)

// claimHolderName mirrors the scheduler's claim holder string; the value is
// opaque to the ladder.
const claimHolderName = "sinet-scheduler"

// seedLease writes the run's lease block exactly as scheduler.claimOne does
// (holder, wall-clock deadline, heartbeat cursor 0).
func seedLease(t *testing.T, s *stack, runID, holder string, deadline time.Time) {
	t.Helper()
	err := s.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		return s.runs.SetLeaseTx(context.Background(), tx, runID, holder, deadline, 0)
	})
	if err != nil {
		t.Fatalf("seed lease on %s: %v", runID, err)
	}
}

// beat renews the run's lease the way a driving holder does (run.LeaseKeeper's
// per-tick write): fenced at the run's current generation, deadline = the
// holder's now + ⚙ recovery.dead_after, no event append.
func beat(t *testing.T, s *stack, runID string, now time.Time) bool {
	t.Helper()
	r := getRun(t, s, runID)
	applied := false
	err := s.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		applied, err = s.runs.RenewLeaseTx(context.Background(), tx, runID, claimHolderName,
			now.Add(deadAfter), r.Generation)
		return err
	})
	if err != nil {
		t.Fatalf("renew lease on %s: %v", runID, err)
	}
	return applied
}

// appendEventAt puts one cursor-advancing event on the run at a chosen
// wall-clock instant (the ladder reads ts only as a staleness input; ordering
// stays event_seq, P-T07-4).
func appendEventAt(t *testing.T, s *stack, runID, typ string, at time.Time) {
	t.Helper()
	r := getRun(t, s, runID)
	if _, err := s.log.Append(context.Background(), eventlog.Append{
		RunID:         r.ID,
		Generation:    r.Generation,
		UserID:        r.UserID,
		Type:          typ,
		SchemaVersion: 1,
		Payload:       json.RawMessage(`{}`),
		Time:          at,
	}); err != nil {
		t.Fatalf("append %s on %s: %v", typ, runID, err)
	}
}

// mkClaimedRunning drives a run to running through the ratified edges with the
// scheduler's claim-time lease in place (deadline = claim + ⚙
// recovery.dead_after, heartbeat cursor 0 — exactly claimOne's write).
func mkClaimedRunning(t *testing.T, s *stack, id string, claimedAt time.Time) run.Run {
	t.Helper()
	ctx := context.Background()
	if _, err := s.runs.Create(ctx, run.NewRun{ID: id, UserID: "u1", Substrate: "mock", Lane: "mock"}); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
	for _, to := range []run.State{run.StateQueued, run.StateClaimed} {
		if _, err := s.runs.Transition(ctx, id, to, run.TransitionOptions{}); err != nil {
			t.Fatalf("%s→%s: %v", id, to, err)
		}
	}
	seedLease(t, s, id, claimHolderName, claimedAt.Add(deadAfter))
	r, err := s.runs.Transition(ctx, id, run.StateRunning, run.TransitionOptions{})
	if err != nil {
		t.Fatalf("%s→running: %v", id, err)
	}
	return r
}

// successorCount reports how many fork successors a run has.
func successorCount(t *testing.T, s *stack, runID string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM runs WHERE parent_run_id = ?`, runID).Scan(&n); err != nil {
		t.Fatalf("count successors of %s: %v", runID, err)
	}
	return n
}

// pass runs one reconcile pass and fails the test on error.
func pass(t *testing.T, l *recovery.Ladder) recovery.Report {
	t.Helper()
	rpt, err := l.ReconcilePass(context.Background())
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	return rpt
}

// ── T1 ──────────────────────────────────────────────────────────────────────

// TestReconcile_AdvanceOutlivesClaimLease_Regression replays the incident: a
// run claimed with claimOne's one-shot lease is still advancing when that lease
// expires. It died 11 s past the lease with a checkpoint 14 s old — the event
// cursor was provably fresh at the kill. S02.5's DEAD is "nothing newer to
// harvest"; a checkpoint 14 s ago is something newer by any honest reading.
func TestReconcile_AdvanceOutlivesClaimLease_Regression(t *testing.T) {
	s := newStack(t)
	t0 := time.Now()
	clock := &fakeClock{t: t0}
	mkClaimedRunning(t, s, "r1", t0)
	l := newLadder(t, s, clock, nil, nil) // B0 probes: UnitGone, nothing to harvest

	pass(t, l) // throwaway: the next pass is grace-neutral

	// The advance kept writing: a checkpoint 3 s before the lease deadline,
	// i.e. 14 s before the pass that used to kill it.
	deadline := t0.Add(deadAfter)
	appendEventAt(t, s, "r1", gates.EventCheckpoint, deadline.Add(-3*time.Second))
	clock.t = deadline.Add(11 * time.Second)

	rpt := pass(t, l)
	if rpt.Alive != 1 {
		t.Errorf("report = %+v, want 1 alive — the cursor advanced 14 s ago (S02.5 step 2 ALIVE)", rpt)
	}
	if rpt.Forked != 0 || rpt.Finalized != 0 || rpt.Tombstoned != 0 {
		t.Errorf("report = %+v, want nothing reaped — this is the incident", rpt)
	}
	if r := getRun(t, s, "r1"); r.State != run.StateRunning {
		t.Errorf("state = %s, want running (a live interview must survive its claim lease)", r.State)
	}
	if n := successorCount(t, s, "r1"); n != 0 {
		t.Errorf("%d successor(s) forked from a live run", n)
	}
	if got := countType(t, s, "r1", run.EventState); got != 3 {
		t.Errorf("run.state_changed events = %d, want 3 (queued, claimed, running — no crash)", got)
	}
}

// ── T2 ──────────────────────────────────────────────────────────────────────

// TestReconcile_StillReapsTheDead pins the other half: with the body gone, the
// lease dead or absent AND the cursor stale past ⚙ recovery.dead_after, the run
// is genuinely dead and is crashed + forked in one pass (S02.5 steps 2–3).
func TestReconcile_StillReapsTheDead(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lease bool // (a) an expired claim lease / (b) no lease row at all
	}{
		{"expired lease", true},
		{"leaseless corpse row", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStack(t)
			t0 := time.Now()
			clock := &fakeClock{t: t0}
			if tc.lease {
				mkClaimedRunning(t, s, "r1", t0)
			} else {
				mustRunning(t, s, "r1")
			}
			if _, err := s.cps.Write(context.Background(), mkCheckpoint("r1")); err != nil {
				t.Fatalf("checkpoint: %v", err)
			}
			l := newLadder(t, s, clock, nil, nil)
			pass(t, l) // throwaway → grace-neutral

			// Past the lease AND past dead_after of cursor silence.
			clock.t = t0.Add(500 * time.Second)
			rpt := pass(t, l)
			if rpt.Forked != 1 {
				t.Fatalf("report = %+v, want 1 forked — no holder, no progress, no body", rpt)
			}
			parent := getRun(t, s, "r1")
			if parent.State != run.StateCrashed || parent.Generation != 1 {
				t.Fatalf("parent = %s gen %d, want crashed gen 1 (the takeover fence)", parent.State, parent.Generation)
			}
			succ := getRun(t, s, "r1.g1")
			if succ.State != run.StateQueued || succ.Generation != 1 || succ.ParentRunID != "r1" {
				t.Fatalf("successor = %+v, want queued gen 1 superseding r1", succ)
			}
		})
	}
}

// ── T3 ──────────────────────────────────────────────────────────────────────

// TestReconcile_ReapBound sweeps the S02.5 bound rather than sampling one
// example: a scanned run under an unknowable body is NOT dead on a pass at
// max(lease deadline, last event + ⚙ recovery.dead_after) − ε and IS dead on
// the first pass after it. The bound is the conjunction made explicit.
func TestReconcile_ReapBound(t *testing.T) {
	const eps = time.Second
	for _, tc := range []struct {
		name      string
		leaseIn   time.Duration // 0 = no lease at all
		lastEvent time.Duration // extra cursor advance at t0+lastEvent
	}{
		{"lease and cursor coincide", 300 * time.Second, 0},
		{"cursor binds (short lease)", 100 * time.Second, 0},
		{"lease binds (long lease)", 400 * time.Second, 0},
		{"cursor binds with no lease", 0, 60 * time.Second},
		{"lease binds past a later cursor", 500 * time.Second, 100 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStack(t)
			t0 := time.Now()
			clock := &fakeClock{t: t0}
			mustRunning(t, s, "r1")
			if tc.leaseIn > 0 {
				seedLease(t, s, "r1", claimHolderName, t0.Add(tc.leaseIn))
			}
			l := newLadder(t, s, clock, nil, nil)
			pass(t, l) // throwaway → grace-neutral from here

			lastEvent := t0
			if tc.lastEvent > 0 {
				lastEvent = t0.Add(tc.lastEvent)
				appendEventAt(t, s, "r1", gates.EventCheckpoint, lastEvent)
			}
			bound := lastEvent.Add(deadAfter)
			if lease := t0.Add(tc.leaseIn); tc.leaseIn > 0 && lease.After(bound) {
				bound = lease
			}

			// Just inside the bound: not reapable.
			clock.t = bound.Add(-eps)
			rpt := pass(t, l)
			if rpt.Forked != 0 || rpt.Finalized != 0 || rpt.Tombstoned != 0 {
				t.Fatalf("at bound−ε report = %+v, want nothing reaped", rpt)
			}
			if r := getRun(t, s, "r1"); r.State != run.StateRunning {
				t.Fatalf("at bound−ε state = %s, want running", r.State)
			}

			// The first pass past it: reaped.
			clock.t = bound.Add(eps)
			rpt = pass(t, l)
			if rpt.Forked != 1 {
				t.Fatalf("at bound+ε report = %+v, want 1 forked", rpt)
			}
			if r := getRun(t, s, "r1"); r.State != run.StateCrashed {
				t.Fatalf("at bound+ε state = %s, want crashed", r.State)
			}
		})
	}
}

// ── T6 ──────────────────────────────────────────────────────────────────────

// TestReconcile_HumanPacedInterviewSurvivesIndefinitely is the headline: an
// interview paced by a person — arbitrary pauses at ask cards, multi-minute
// in-process advances, including one advance segment longer than ⚙
// recovery.dead_after with no event appends at all — survives every sweep on a
// healthy world. Three full cycles over hours of clock.
func TestReconcile_HumanPacedInterviewSurvivesIndefinitely(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	t0 := time.Now()
	clock := &fakeClock{t: t0}
	mkClaimedRunning(t, s, "r1", t0)
	l := newLadder(t, s, clock, nil, nil) // UnitGone throughout: no unit machinery at v0

	seen := map[run.State]bool{}
	sweep := func() {
		rpt := pass(t, l)
		r := getRun(t, s, "r1")
		seen[r.State] = true
		switch r.State {
		case run.StateClaimed, run.StateRunning, run.StateParked, run.StateCompleted:
		default:
			t.Fatalf("run left the healthy set: %s (report %+v)", r.State, rpt)
		}
		if rpt.Forked != 0 || rpt.Tombstoned != 0 || rpt.Finalized != 0 {
			t.Fatalf("a healthy interview was reaped: %+v", rpt)
		}
	}

	// advance simulates the driving holder for d: it steps the clock in
	// heartbeat increments, renewing the lease at each step (R3 holder
	// liveness), appends progress every progressEvery (0 = a QUIET segment:
	// one long engine call whose stream says nothing), and sweeps on the
	// ladder's own cadence.
	advance := func(d, progressEvery time.Duration) {
		for elapsed := time.Duration(0); elapsed < d; elapsed += heartbeat {
			clock.advance(heartbeat)
			beat(t, s, "r1", clock.t)
			if progressEvery > 0 && (elapsed+heartbeat)%progressEvery == 0 {
				appendEventAt(t, s, "r1", gates.EventCheckpoint, clock.t)
			}
			if (elapsed+heartbeat)%sweepInterval == 0 {
				sweep()
			}
		}
	}
	// park simulates the person walking away: no holder, no heartbeats, the
	// lease left to expire while the run sits at its card. Parked runs are not
	// scanned at all (R10), so this must be safe indefinitely.
	park := func(d time.Duration) {
		if _, err := s.runs.Transition(ctx, "r1", run.StateParked, run.TransitionOptions{
			Reason: "open ask: gates wait (S06.1)", Actor: run.ActorPlatform,
		}); err != nil {
			t.Fatalf("park: %v", err)
		}
		for elapsed := time.Duration(0); elapsed < d; elapsed += sweepInterval {
			clock.advance(sweepInterval)
			sweep()
			if r := getRun(t, s, "r1"); r.State != run.StateParked {
				t.Fatalf("a parked run was touched by the sweep: %s", r.State)
			}
		}
	}
	unpark := func() {
		if _, err := s.runs.Transition(ctx, "r1", run.StateRunning, run.TransitionOptions{
			Reason: "card answered: pipeline resumes in place (4.3)", Actor: run.ActorPlatform,
		}); err != nil {
			t.Fatalf("unpark: %v", err)
		}
		// R5: the holder's first heartbeat lands immediately on taking the run,
		// so no sweep can ever observe an expired lease on a resumed run.
		if !beat(t, s, "r1", clock.t) {
			t.Fatal("the resume heartbeat was fenced out — the un-park instant is unprotected (R5)")
		}
	}

	// Cycle 1: a chatty advance, then a long think at the card.
	advance(10*time.Minute, 2*time.Minute)
	park(90 * time.Minute)

	// Cycle 2: the case the lease alone cannot cover — one advance segment
	// longer than dead_after with ZERO event appends, only heartbeats.
	unpark()
	advance(9*time.Minute, 0)
	appendEventAt(t, s, "r1", gates.EventCheckpoint, clock.t)
	park(3 * time.Hour)

	// Cycle 3: back to work, then the run ends by its own completion.
	unpark()
	advance(6*time.Minute, 3*time.Minute)
	if _, err := s.runs.Transition(ctx, "r1", run.StateCompleted, run.TransitionOptions{
		Reason: "intake approved", Actor: run.ActorPlatform,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	clock.advance(sweepInterval)
	sweep()

	if !seen[run.StateRunning] || !seen[run.StateParked] {
		t.Fatalf("the walk never exercised running and parked: %v", seen)
	}
	if got := countType(t, s, "r1", run.EventForked); got != 0 {
		t.Errorf("fork events = %d, want 0", got)
	}
	if n := successorCount(t, s, "r1"); n != 0 {
		t.Errorf("%d successor(s) exist — a healthy interview was superseded", n)
	}
	if r := getRun(t, s, "r1"); r.State != run.StateCompleted {
		t.Errorf("final state = %s, want completed (the run ended by its own act)", r.State)
	}
}

// ── T7 ──────────────────────────────────────────────────────────────────────

// TestReconcile_WakeGraceStillHolds keeps the suspend-aware behaviour exactly
// as ratified (S02.5 step 4, P-T07-4): on a wake pass, a lease that expired
// inside ⚙ recovery.wake_grace is not dead, and the run is left pending for the
// next sweep rather than classified either way.
func TestReconcile_WakeGraceStillHolds(t *testing.T) {
	s := newStack(t)
	t0 := time.Now()
	mustRunning(t, s, "r1")
	deadline := t0.Add(time.Hour)
	seedLease(t, s, "r1", claimHolderName, deadline)

	// The nap: the wall clock jumps past the deadline by less than the grace,
	// and the cursor has been silent for longer than dead_after but inside
	// dead_after + grace.
	clock := &fakeClock{t: deadline.Add(60 * time.Second)}
	l := newLadder(t, s, clock, nil, nil)
	rpt := pass(t, l)
	if !rpt.GraceApplied {
		t.Fatal("the first pass of a process must apply ⚙ recovery.wake_grace")
	}
	if rpt.Pending != 1 || rpt.Forked != 0 || rpt.Wedged != 0 {
		t.Fatalf("report = %+v, want 1 pending — nothing is decided inside the grace", rpt)
	}
	if r := getRun(t, s, "r1"); r.State != run.StateRunning {
		t.Fatalf("state = %s, want running (never declared dead inside the grace)", r.State)
	}
}

// ── T8 ──────────────────────────────────────────────────────────────────────

// TestReconcile_CorpseFailedOutranksFreshCursor guards against over-rotating
// the cursor rule into "any fresh event blocks DEAD". A failed unit corpse is
// conclusive: P-T07-2's wrapper exit record is itself a RECENT append, and it
// is death evidence, not life. Cursor freshness gates DEAD only under
// UnitGone/UnitUnknown.
func TestReconcile_CorpseFailedOutranksFreshCursor(t *testing.T) {
	s := newStack(t)
	t0 := time.Now()
	clock := &fakeClock{t: t0}
	mustRunning(t, s, "r1")
	seedLease(t, s, "r1", claimHolderName, t0.Add(time.Hour)) // lease still live, too
	// The wrapper's own exit record: a RECENT append that is death evidence.
	appendEventAt(t, s, "r1", gates.EventCheckpoint, t0.Add(-5*time.Second))
	l := newLadder(t, s, clock, fakeUnits{"r1": {State: recovery.UnitCorpseFailed, Result: "failed"}}, nil)

	rpt := pass(t, l)
	if rpt.Forked != 1 {
		t.Fatalf("report = %+v, want 1 forked — the body outranks the cursor", rpt)
	}
	if r := getRun(t, s, "r1"); r.State != run.StateCrashed {
		t.Fatalf("state = %s, want crashed", r.State)
	}
}

// ── T9 ──────────────────────────────────────────────────────────────────────

// TestReconcile_WedgedInProcess is S02.5's WEDGED translated to the in-process
// world, where there is no unit to be "active": a heartbeating holder (a live
// lease) whose cursor has stalled past ⚙ recovery.dead_after is WEDGED —
// flagged once per stall episode and NEVER auto-killed (D1.3, S10.8). The
// watchdog's silence rule stays the containment owner; this flag is the
// ladder's own observation.
func TestReconcile_WedgedInProcess(t *testing.T) {
	s := newStack(t)
	t0 := time.Now()
	clock := &fakeClock{t: t0}
	mkClaimedRunning(t, s, "r1", t0)
	l := newLadder(t, s, clock, nil, nil)
	pass(t, l) // throwaway → grace-neutral

	// The holder is alive and beating; progress has stopped.
	clock.t = t0.Add(400 * time.Second)
	if !beat(t, s, "r1", clock.t) {
		t.Fatal("the holder's heartbeat did not apply")
	}
	rpt := pass(t, l)
	if rpt.Wedged != 1 {
		t.Fatalf("report = %+v, want 1 wedged (holder alive, progress stopped)", rpt)
	}
	if rpt.Forked != 0 || rpt.Finalized != 0 || rpt.Tombstoned != 0 {
		t.Fatalf("report = %+v, want nothing reaped — a wedge is pause-and-flag, never a kill", rpt)
	}
	if r := getRun(t, s, "r1"); r.State != run.StateRunning {
		t.Fatalf("state = %s, want running (wedged is derived, never stored)", r.State)
	}
	if got := countType(t, s, "r1", run.EventWedged); got != 1 {
		t.Fatalf("run.wedged events = %d, want 1", got)
	}

	// Once per stall episode, not once per sweep.
	clock.advance(sweepInterval)
	if !beat(t, s, "r1", clock.t) {
		t.Fatal("the holder's second heartbeat did not apply")
	}
	rpt = pass(t, l)
	if rpt.Forked != 0 {
		t.Fatalf("report = %+v, want the wedged run still alive", rpt)
	}
	if got := countType(t, s, "r1", run.EventWedged); got != 1 {
		t.Fatalf("run.wedged events after a second sweep = %d, want 1", got)
	}
	if r := getRun(t, s, "r1"); r.State != run.StateRunning {
		t.Fatalf("state = %s, want running", r.State)
	}
}

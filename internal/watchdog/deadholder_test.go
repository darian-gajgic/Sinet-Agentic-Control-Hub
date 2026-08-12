package watchdog

// deadholder_test.go — P3-RW-10 T6: the silence rule stops HIDING driver-less
// runs from the one component ratified to classify them.
//
// The silence rule was the last liveness observer that read only half the
// evidence (state, open asks, newest event ts). A run whose holder is gone is
// not WEDGED — there is no live work to contain and no next paid call to refuse
// — it is the recovery pass's DEAD candidate (S02.5 step 2, the §54
// conjunction). Parking it hides it there forever: a parked run is never
// scanned, and a park with no resume-time waits on a person.
//
// FIXTURE DISCIPLINE (drain r1, F1): every regime here is one the running
// platform actually produces. The sweep runs every 30s against a silence budget
// that defaults to the ⚙ recovery.dead_after seed (300s), so a holder that dies
// is first SEEN silent at ~300–330s — a fixture that only trips at 700s pins a
// state the cadence cannot reach, and would let a rule that never abstains in
// production pass green.

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// setLease writes a lease block onto a run (the claim-time semantics, S10.7).
func (e *env) setLease(id, holder string, deadline time.Time) {
	e.t.Helper()
	ctx := context.Background()
	if err := e.db.WriteTx(ctx, func(tx *sql.Tx) error {
		return e.runs.SetLeaseTx(ctx, tx, id, holder, deadline, 0)
	}); err != nil {
		e.t.Fatalf("set lease on %s: %v", id, err)
	}
}

// The default regime: no ⚙ watchdog.silence_budget entry (the map ships EMPTY),
// so the budget IS the recovery.dead_after seed — 300s — and one sweep after it
// the run is silent 310s. Its holder died 10s ago, which is also INSIDE ⚙
// recovery.wake_grace: abstaining there is correct, because the recovery pass
// collects the run when its own grace expires, while a park would re-hide it.
func TestSilenceRuleLeavesADeadHolderToTheLadder(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name  string
		lease func(e *env, id string)
	}{
		{"lease expired 10s ago", func(e *env, id string) {
			e.setLease(id, "stage-drain", time.Now().Add(-10*time.Second))
		}},
		{"no lease at all", func(*env, string) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const id = "holderless-run"
			e := newEnv(t)
			w := e.wd()
			e.runningRun(id, "alice")
			e.staleActivity(id, 310)
			tc.lease(e, id)

			if err := w.sweepSilence(ctx); err != nil {
				t.Fatalf("sweepSilence: %v", err)
			}
			fs := e.flagsFor(id)
			if len(fs) != 1 || fs[0].Rule != RuleSilence {
				t.Fatalf("the dead-holder run raised no silence card: %+v", fs)
			}
			if fs[0].Parked {
				t.Errorf("the flag claims a park that must not have happened: %+v", fs[0])
			}
			if got := e.state(id); got != run.StateRunning {
				t.Fatalf("run state = %s, want still running — a park hides it from the pass that would heal it", got)
			}
		})
	}
}

// A LIVE lease is containment's own case, untouched: a beating holder with a
// stalled cursor is WEDGED, and S14.4 parks it with its card.
func TestSilenceRuleStillParksALiveHolder(t *testing.T) {
	ctx := context.Background()
	const id = "wedged-run"
	e := newEnv(t)
	w := e.wd()
	e.runningRun(id, "alice")
	e.staleActivity(id, 310)
	e.setLease(id, "stage-drain", time.Now().Add(5*time.Minute))

	if err := w.sweepSilence(ctx); err != nil {
		t.Fatalf("sweepSilence: %v", err)
	}
	fs := e.flagsFor(id)
	if len(fs) != 1 || fs[0].Rule != RuleSilence {
		t.Fatalf("the wedged run raised no silence card: %+v", fs)
	}
	if !fs[0].Parked {
		t.Errorf("a live-lease silence must still park: %+v", fs[0])
	}
	if got := e.state(id); got != run.StateParked {
		t.Fatalf("run state = %s, want parked (S14.4 containment, byte-identical)", got)
	}
}

// The cursor conjunct, on its own: a SHORT per-run-type budget trips silence
// while the run is still appending inside ⚙ recovery.dead_after. However long
// dead the lease is, the recovery pass would not take this run — so abstaining
// would leave it uncontained, and the ordinary park stands.
func TestSilenceRuleParksAFreshCursorWhateverTheLease(t *testing.T) {
	ctx := context.Background()
	const id = "early-silent-run"
	e := newEnv(t)
	setMap(t, e.reg, keySilenceBudget, map[string]string{"anthropic": "120"})
	w := e.wd()
	e.runningRun(id, "alice")
	e.staleActivity(id, 200) // past the 120s budget, inside the 300s dead bound
	e.setLease(id, "stage-drain", time.Now().Add(-2*time.Hour))

	if err := w.sweepSilence(ctx); err != nil {
		t.Fatalf("sweepSilence: %v", err)
	}
	fs := e.flagsFor(id)
	if len(fs) != 1 || !fs[0].Parked {
		t.Fatalf("the fresh-cursor run was not parked-and-flagged: %+v", fs)
	}
	if got := e.state(id); got != run.StateParked {
		t.Fatalf("run state = %s, want parked — nothing else would contain it yet", got)
	}
}

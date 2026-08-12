package watchdog

// deadholder_test.go — P3-RW-10 T6: the silence rule stops HIDING driver-less
// runs from the one component ratified to classify them.
//
// The silence rule is the last liveness observer that read only half the
// evidence (state, open asks, newest event ts). A run whose holder is gone is
// not WEDGED — there is no live work to contain, and no next paid call to
// refuse — it is the recovery ladder's DEAD candidate (S02.5 step 2, the §54
// conjunction). Parking it hides it from the ladder forever, because a parked
// run is never scanned, and a park with no resume-time only a human can release.
//
// So: raise the card, leave the run. A candidate with a LIVE lease keeps today's
// park-and-flag exactly (S14.4 containment), and the suite still transitions
// nothing else, ever.

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

// silenceEnv is a watched run silent for silentFor seconds under a 120s budget.
// The ⚙ defaults around it: recovery.dead_after 300s, recovery.wake_grace 120s.
func silenceEnv(t *testing.T, id string, silentFor int) (*env, *Watchdog) {
	t.Helper()
	e := newEnv(t)
	setMap(t, e.reg, keySilenceBudget, map[string]string{"anthropic": "120"})
	e.runningRun(id, "alice")
	e.staleActivity(id, silentFor)
	return e, e.wd()
}

func TestSilenceRuleLeavesDeadHolderToTheLadder(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name  string
		lease func(e *env, id string)
	}{
		{"no lease at all", func(*env, string) {}},
		{"lease expired past the wake grace", func(e *env, id string) {
			e.setLease(id, "stage-drain", time.Now().Add(-10*time.Minute))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const id = "holderless-run"
			e, w := silenceEnv(t, id, 700) // past the budget AND past dead_after + wake_grace
			tc.lease(e, id)

			if err := w.sweepSilence(ctx); err != nil {
				t.Fatalf("sweepSilence: %v", err)
			}
			fs := e.flagsFor(id)
			if len(fs) != 1 || fs[0].Rule != RuleSilence {
				t.Fatalf("the dead-holder run raised no silence card: %+v", fs)
			}
			if fs[0].Parked {
				t.Errorf("the flag claims the run was parked — the card must say what actually happened")
			}
			if got := e.state(id); got != run.StateRunning {
				t.Fatalf("run state = %s, want still running — parking it hides it from the ladder forever", got)
			}
		})
	}
}

// A LIVE lease is containment's own case and is untouched: a beating holder with
// a stalled cursor is WEDGED, and S14.4 parks it with its card.
func TestSilenceRuleStillParksALiveHolder(t *testing.T) {
	ctx := context.Background()
	const id = "wedged-run"
	e, w := silenceEnv(t, id, 700)
	e.setLease(id, "stage-drain", time.Now().Add(10*time.Minute))

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

// The ambiguous window: silent past its own (shorter) budget but not yet past
// the ladder's dead bound. The ladder would NOT take this run, so abstaining
// would leave it uncontained — containment wins the tie.
func TestSilenceRuleParksWhenTheLadderWouldNotTakeItYet(t *testing.T) {
	ctx := context.Background()
	const id = "early-silent-run"
	e, w := silenceEnv(t, id, 200) // past the 120s budget, inside dead_after + wake_grace

	if err := w.sweepSilence(ctx); err != nil {
		t.Fatalf("sweepSilence: %v", err)
	}
	if fs := e.flagsFor(id); len(fs) != 1 || !fs[0].Parked {
		t.Fatalf("the early-silent run was not parked-and-flagged: %+v", fs)
	}
	if got := e.state(id); got != run.StateParked {
		t.Fatalf("run state = %s, want parked — nothing else would contain it yet", got)
	}
}

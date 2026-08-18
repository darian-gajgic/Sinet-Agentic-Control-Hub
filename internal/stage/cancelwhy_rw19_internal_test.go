package stage

// cancelwhy_rw19_internal_test.go — P3-RW-19 executor half, verb-path mint
// (T8 mint limb, T9 sibling limb). Committed RED before the implementation
// (Amendment-A carve-out, CONVENTIONS §3).
//
// The transport half is asserted in internal/api (the body field is read,
// bounded and handed over); what is asserted HERE is the other end of the same
// thread — the person's own words reach the durable record, on EVERY transition
// the verb mints, through the one constructor and no other spelling.

import (
	"context"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// rw19DetailReason reads the human reason off a run's newest cancel
// transition, reporting whether the key is there at all — absence is the
// contract when nobody gave a reason.
func (e *cancelEnv) rw19DetailReason(t *testing.T, runID string) (string, bool) {
	t.Helper()
	m := e.lastTransition(t, runID)
	detail, ok := m["detail"].(map[string]any)
	if !ok {
		t.Fatalf("run %s: the cancel transition carries no structured detail: %v", runID, m)
	}
	if got, _ := detail["cause"].(string); got != run.CancelCauseHuman {
		t.Fatalf("run %s: detail.cause = %q, want the frozen %q", runID, got, run.CancelCauseHuman)
	}
	v, present := detail["reason"]
	s, _ := v.(string)
	return s, present
}

// TestVerbCancelCarriesTheHumanReasonOntoTheRecord — T8, mint limb: the reason
// captured at the affordance rides the ONE constructor onto `$.detail.reason`,
// beside the frozen keys, and the transition's own mechanical sentence is
// untouched (the human words are ADDITIVE, they replace nothing).
func TestVerbCancelCarriesTheHumanReasonOntoTheRecord(t *testing.T) {
	e := newCancelEnv(t)
	ctx := context.Background()
	r := e.seedRun(t, "t-rw19-a.execute", "t-rw19-a", "alice", run.StateRunning)
	e.sk.cancels.register(r.ID, &fakeSession{})

	if _, err := e.sk.CancelRun(ctx, "alice", r.ID, "taking a different approach"); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	got, present := e.rw19DetailReason(t, r.ID)
	if !present || got != "taking a different approach" {
		t.Errorf("detail.reason = %q (present=%v), want the person's own words", got, present)
	}
	if m := e.lastTransition(t, r.ID); m["reason"] != "cancelled by alice (4.5)" {
		t.Errorf("the transition's mechanical sentence became %v — the human reason is additive", m["reason"])
	}

	// The parked limb takes a different edge and a different transaction, and
	// it must carry the reason just the same.
	p := e.seedRun(t, "t-rw19-b.verify", "t-rw19-b", "alice", run.StateParked)
	if _, err := e.sk.CancelRun(ctx, "alice", p.ID, "the answer stopped mattering"); err != nil {
		t.Fatalf("CancelRun(parked): %v", err)
	}
	if got, present := e.rw19DetailReason(t, p.ID); !present || got != "the answer stopped mattering" {
		t.Errorf("parked cancel detail.reason = %q (present=%v), want the person's own words", got, present)
	}

	// No reason given: the key is ABSENT, never present-and-blank — a blank
	// reason would claim a motive nobody gave (the ask_id precedent).
	n := e.seedRun(t, "t-rw19-c.execute", "t-rw19-c", "alice", run.StateRunning)
	if _, err := e.sk.CancelRun(ctx, "alice", n.ID, ""); err != nil {
		t.Fatalf("CancelRun(no reason): %v", err)
	}
	if got, present := e.rw19DetailReason(t, n.ID); present {
		t.Errorf("a reason-less cancel recorded reason %q — absence must be absent", got)
	}
}

// TestTaskCancelWritesTheSameReasonOnEverySiblingTransition — T9, verb limb:
// one act ends several runs, and each ending is its own record, so the reason
// the person gave rides EVERY one of them. A reason on only the first would
// leave the other endings unexplained on the same task page.
func TestTaskCancelWritesTheSameReasonOnEverySiblingTransition(t *testing.T) {
	e := newCancelEnv(t)
	ctx := context.Background()
	e.seedRun(t, "t-rw19-d.intake", "t-rw19-d", "alice", run.StateCompleted) // already ended: untouched
	e.seedRun(t, "t-rw19-d.execute", "t-rw19-d", "alice", run.StateRunning)
	e.seedRun(t, "t-rw19-d.verify", "t-rw19-d", "alice", run.StateParked)

	const why = "we found a better tool for this"
	out, err := e.sk.CancelTask(ctx, "alice", "t-rw19-d", why)
	if err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	if len(out.Runs) != 3 || !out.Applied {
		t.Fatalf("task cancel outcome = %+v", out)
	}
	for _, id := range []string{"t-rw19-d.execute", "t-rw19-d.verify"} {
		got, present := e.rw19DetailReason(t, id)
		if !present || got != why {
			t.Errorf("run %s: detail.reason = %q (present=%v), want %q on every transition the one act minted",
				id, got, present, why)
		}
	}
	// The run that had already ended minted nothing, so its newest transition
	// is still the fixture's — the cancel did not rewrite a finished record.
	if m := e.lastTransition(t, "t-rw19-d.intake"); m["detail"] != nil {
		t.Errorf("an already-ended run gained a cancel detail: %v", m)
	}
}

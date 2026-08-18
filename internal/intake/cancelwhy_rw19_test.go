package intake_test

// cancelwhy_rw19_test.go — P3-RW-19 executor half, intake card mints (T11).
// Committed RED before the implementation (Amendment-A carve-out,
// CONVENTIONS §3).
//
// R6: the intake cards gain the same one channel the verify/ladder cards
// already had — an additive `note` on the answer — honored on the TWO
// cancel-shaped answers, the approval card's `cancel` and the SPEC-DOUBT
// card's `rethink`. Everywhere else it is ignored at v0 (ratified OQ2), and
// the last test here is what keeps that honest: a note on an answer that
// cancels nothing must leave no motive anywhere in the record.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// rw19IntakeCancelReason reads the human reason off an intake run's structured
// cancel transition, reporting whether the key rode at all.
func rw19IntakeCancelReason(t *testing.T, f *fix, runID string) (string, bool) {
	t.Helper()
	var raw string
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT COALESCE(json_extract(payload, '$.detail'), '')
		   FROM run_events
		  WHERE run_id = ? AND type = ? AND json_extract(payload, '$.detail.cause') = ?
		  ORDER BY event_seq DESC LIMIT 1`,
		runID, run.EventState, run.CancelCauseHuman).Scan(&raw); err != nil {
		t.Fatalf("no structured cancel transition on %s: %v", runID, err)
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(raw), &detail); err != nil {
		t.Fatalf("decode cancel detail %q: %v", raw, err)
	}
	v, present := detail["reason"]
	s, _ := v.(string)
	return s, present
}

// TestIntakeApprovalCancelNoteBecomesTheHumanReason — T11, approval limb: the
// requester cancels at their approval card and says why in the card's note.
func TestIntakeApprovalCancelNoteBecomesTheHumanReason(t *testing.T) {
	f := newFix(t)
	const note = "I described the wrong thing"
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{ForceProceed: true}) // reach the approval card
	askID, _ = f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionCancel, Note: note})
	if st.Phase != intake.PhaseCancelled {
		t.Fatalf("phase %s, want cancelled", st.Phase)
	}
	if got, present := rw19IntakeCancelReason(t, f, st.RunID); !present || got != note {
		t.Errorf("detail.reason = %q (present=%v), want the card note %q", got, present, note)
	}
}

// TestIntakeRethinkNoteBecomesTheHumanReason — T11, SPEC-DOUBT limb: rethink is
// a cancel that does not say "cancel", and it carries a motive just the same.
func TestIntakeRethinkNoteBecomesTheHumanReason(t *testing.T) {
	f := newFix(t)
	const note = "the tool I already have does this"
	f.critic.verdicts = []intake.Verdict{{Kind: intake.VerdictSpecDoubt, Doubt: "the goal duplicates an existing tool"}}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardSpecDoubt {
		t.Fatalf("expected the SPEC-DOUBT card, got %q", st.OpenAskKind)
	}
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Choice: intake.ChoiceRethink, Note: note})
	if st.Phase != intake.PhaseCancelled {
		t.Fatalf("phase %s, want cancelled after rethink", st.Phase)
	}
	if got, present := rw19IntakeCancelReason(t, f, st.RunID); !present || got != note {
		t.Errorf("detail.reason = %q (present=%v), want the card note %q", got, present, note)
	}
}

// TestIntakeCancelWithoutANoteRecordsNoReason — the absence posture at this
// mint: no note means no reason key, never a blank one.
func TestIntakeCancelWithoutANoteRecordsNoReason(t *testing.T) {
	f := newFix(t)
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{ForceProceed: true})
	askID, _ = f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionCancel})
	if got, present := rw19IntakeCancelReason(t, f, st.RunID); present {
		t.Errorf("a note-less intake cancel recorded reason %q — absence must be absent", got)
	}
}

// TestIntakeVerbCancelCarriesTheHumanReason — ratified OQ4 / R13: the intake
// cancel VERB reaches the one constructor with a reason like the three
// affordance-backed paths do. Nothing routes to it today, which is exactly why
// the parameter is threaded now: mint parity is the constructor's contract, and
// the path that could not carry a motive would be the one serving a blank why
// the day it is routed.
func TestIntakeVerbCancelCarriesTheHumanReason(t *testing.T) {
	f := newFix(t)
	const why = "I no longer need this built"
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)

	st, err := f.p.Cancel(context.Background(), "u1", st.TaskID, why)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if st.Phase != intake.PhaseCancelled {
		t.Fatalf("phase %s, want cancelled", st.Phase)
	}
	if got, present := rw19IntakeCancelReason(t, f, st.RunID); !present || got != why {
		t.Errorf("detail.reason = %q (present=%v), want %q", got, present, why)
	}
}

// TestIntakeNoteIsIgnoredOutsideTheCancelShapedAnswers — ratified OQ2: the
// field exists on every intake answer because one struct carries them all, but
// v0 honors it only where it is a cancel's why. An answer that cancels nothing
// must leave no motive in the record at all.
func TestIntakeNoteIsIgnoredOutsideTheCancelShapedAnswers(t *testing.T) {
	f := newFix(t)
	const forceNote = "a note riding a force-proceed"
	const replanNote = "a note riding a re-interview"
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{ForceProceed: true, Note: forceNote})
	askID, _ = f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionReInterview, Note: replanNote})

	// The control that makes the assertions below mean something: the answers
	// were really applied, not refused for carrying a field v0 ignores.
	if st.Phase != intake.PhaseInterview {
		t.Fatalf("phase %s, want the re-interview the second answer asked for", st.Phase)
	}
	// No cancel record exists — scoped to the cancel discriminator, because an
	// ordinary lifecycle payload may carry a `detail.reason` of its own
	// (`run.created` does) and this test is about the cancel's why.
	var n int
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM run_events
		  WHERE run_id = ? AND json_extract(payload, '$.detail.cause') = ?`,
		st.RunID, run.CancelCauseHuman).Scan(&n); err != nil {
		t.Fatalf("count cancel records: %v", err)
	}
	if n != 0 {
		t.Errorf("%d cancel record(s) exist for answers that cancelled nothing", n)
	}
	// And the notes themselves reached NOTHING: not a transition detail, not a
	// ledger decision's prose. This is the limb that can actually fail — folding
	// the note into every intake mint's wording is the tempting symmetry with
	// verify's noteSuffix, and it is exactly what OQ2 held back.
	//
	// The answer snapshot on the `asks` row is deliberately not judged: an
	// answer is recorded as the caller sent it, which is the record, not a use.
	for _, note := range []string{forceNote, replanNote} {
		var hits int
		if err := f.db.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM run_events WHERE run_id = ? AND instr(payload, ?) > 0`,
			st.RunID, note).Scan(&hits); err != nil {
			t.Fatalf("scan for the note: %v", err)
		}
		if hits != 0 {
			t.Errorf("the note %q reached %d record(s) — v0 honors it only at the two cancel-shaped answers (OQ2)", note, hits)
		}
	}
}

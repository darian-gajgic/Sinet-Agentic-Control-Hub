package intake_test

// cancelmint_rw18_test.go — P3-RW-18 D1-R1 and OQ1, pinned at the PRODUCER.
//
// Two things must hold at every intake cancel mint, and both are invisible from
// inside internal/intake, which is why they need a test that says so out loud:
//
//  1. The transition names the HUMAN (15.6) and carries the ONE shared
//     structured detail (run.CancelDetail). Before this packet it said
//     `platform` and carried nothing, and the task detail could not tell a
//     reader who had cancelled their work.
//  2. The ledger entry's text opens with "requester cancelled" / "requester:
//     rethink". internal/api's decisions derive discriminates the pre-parity
//     ledger record BY THAT PREFIX (cancelLedgerPrefixes in reads.go), because
//     `ledger.decide` also carries retries, accepts and revisions and the
//     entry has no typed kind to read instead. The prefix is therefore a
//     cross-package contract with no compiler behind it — so it gets a test.
//     Reword a mint and this fails, which is the point: the alternative is the
//     derive silently going quiet (P3-RW-18 OQ1; a typed `kind` on ledger
//     decisions is the durable fix, left to a future ledger packet).

import (
	"context"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// humanCancelTransition reads the structured cancel transition of a run: the
// transition actor, the detail's actor, and the answered card's id.
func humanCancelTransition(t *testing.T, f *fix, runID string) (actor, detailActor, askID string) {
	t.Helper()
	err := f.db.QueryRowContext(context.Background(),
		`SELECT COALESCE(json_extract(payload, '$.actor'), ''),
		        COALESCE(json_extract(payload, '$.detail.actor'), ''),
		        COALESCE(json_extract(payload, '$.detail.ask_id'), '')
		   FROM run_events
		  WHERE run_id = ? AND type = ? AND json_extract(payload, '$.detail.cause') = ?
		  ORDER BY event_seq DESC LIMIT 1`,
		runID, run.EventState, run.CancelCauseHuman).Scan(&actor, &detailActor, &askID)
	if err != nil {
		t.Fatalf("no human-cancel transition on %s (the derive reads $.detail.cause): %v", runID, err)
	}
	return actor, detailActor, askID
}

// humanLedgerCancelText returns the newest human-authored ledger entry whose
// text reads as a cancel — the same shape the api derive's leg B looks for.
func humanLedgerCancelText(t *testing.T, f *fix, taskID string) string {
	t.Helper()
	doc, found, err := f.led.Current(context.Background(), taskID)
	if err != nil || !found {
		t.Fatalf("ledger Current(%s): found=%v err=%v", taskID, found, err)
	}
	for i := len(doc.Decisions) - 1; i >= 0; i-- {
		d := doc.Decisions[i]
		if d.Author == ledger.AuthorHuman && strings.Contains(strings.ToLower(d.Text), "cancel") {
			return d.Text
		}
	}
	t.Fatalf("no human cancel decision in the ledger of %s: %+v", taskID, doc.Decisions)
	return ""
}

// TestIntakeCardCancelRecordsTheHumanAndKeepsItsLedgerPrefix — the card leg
// (closeAndResume's cancel branch): the requester cancels at their approval
// card, and both records name them.
func TestIntakeCardCancelRecordsTheHumanAndKeepsItsLedgerPrefix(t *testing.T) {
	f := newFix(t)
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{ForceProceed: true}) // reach the approval card
	askID, _ = f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionCancel})
	if st.Phase != intake.PhaseCancelled {
		t.Fatalf("phase %s, want cancelled", st.Phase)
	}

	actor, detailActor, gotAsk := humanCancelTransition(t, f, st.RunID)
	if actor != "u1" || detailActor != "u1" {
		t.Errorf("cancel transition actor = %q / detail.actor = %q, want the requester (15.6)", actor, detailActor)
	}
	if gotAsk != askID {
		t.Errorf("cancel detail names card %q, want the answered card %q (S2.4 card id)", gotAsk, askID)
	}
	if text := humanLedgerCancelText(t, f, st.TaskID); !strings.HasPrefix(text, "requester cancelled") {
		t.Errorf("intake card cancel ledgers %q — internal/api/reads.go cancelLedgerPrefixes reads the prefix %q", text, "requester cancelled")
	}
}

// TestIntakeVerbCancelRecordsTheHumanAndKeepsItsLedgerPrefix — the verb leg
// (Pipeline.Cancel): cancel is always available, with or without an open card.
func TestIntakeVerbCancelRecordsTheHumanAndKeepsItsLedgerPrefix(t *testing.T) {
	f := newFix(t)
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)

	st, err := f.p.Cancel(context.Background(), "u1", st.TaskID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if st.Phase != intake.PhaseCancelled {
		t.Fatalf("phase %s, want cancelled", st.Phase)
	}
	actor, detailActor, _ := humanCancelTransition(t, f, st.RunID)
	if actor != "u1" || detailActor != "u1" {
		t.Errorf("cancel transition actor = %q / detail.actor = %q, want the requester (15.6)", actor, detailActor)
	}
	if text := humanLedgerCancelText(t, f, st.TaskID); !strings.HasPrefix(text, "requester cancelled") {
		t.Errorf("intake verb cancel ledgers %q — the derive reads the prefix %q", text, "requester cancelled")
	}
}

// TestIntakeRethinkKeepsItsLedgerPrefix — the SPEC-DOUBT rethink is a cancel
// that does NOT say "cancelled" first, which is why the derive carries a second
// prefix. If this text ever loses its "requester: rethink" opening, that cancel
// stops being served.
func TestIntakeRethinkKeepsItsLedgerPrefix(t *testing.T) {
	f := newFix(t)
	f.critic.verdicts = []intake.Verdict{{Kind: intake.VerdictSpecDoubt, Doubt: "the goal duplicates an existing tool"}}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardSpecDoubt {
		t.Fatalf("expected the SPEC-DOUBT card, got %q", st.OpenAskKind)
	}
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Choice: intake.ChoiceRethink})
	if st.Phase != intake.PhaseCancelled {
		t.Fatalf("phase %s, want cancelled after rethink", st.Phase)
	}
	text := humanLedgerCancelText(t, f, st.TaskID)
	if !strings.HasPrefix(text, "requester: rethink") {
		t.Errorf("the rethink ledgers %q — internal/api/reads.go reads the prefix %q", text, "requester: rethink")
	}
}

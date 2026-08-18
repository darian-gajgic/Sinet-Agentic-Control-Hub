package stage_test

// cancelwhy_rw19_test.go — P3-RW-19 executor half, CARD-path mints (T10, and
// T9's ladder-card limb). Committed RED before the implementation
// (Amendment-A carve-out, CONVENTIONS §3).
//
// R6: no second field is minted for the reason. The verify/ladder cards already
// carry a `note`, and that note IS the reason channel — it keeps riding the
// ledger decision text exactly as it landed, and it now ALSO reaches the
// structured detail the S02.2 reconstruction reads. One channel per surface,
// one constructor for the record.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// rw19CardCancelReason reads the human reason (and the answered card) off a
// run's structured cancel transition.
func rw19CardCancelReason(t *testing.T, h *harness, runID string) (reason, askID string) {
	t.Helper()
	err := h.db.QueryRowContext(context.Background(),
		`SELECT COALESCE(json_extract(payload, '$.detail.reason'), ''),
		        COALESCE(json_extract(payload, '$.detail.ask_id'), '')
		   FROM run_events
		  WHERE run_id = ? AND type = ? AND json_extract(payload, '$.detail.cause') = ?
		  ORDER BY event_seq DESC LIMIT 1`,
		runID, run.EventState, run.CancelCauseHuman).Scan(&reason, &askID)
	if err != nil {
		t.Fatalf("no structured cancel transition on %s: %v", runID, err)
	}
	return reason, askID
}

// TestVerifyCardCancelNoteBecomesTheHumanReason — T10: the requester ends the
// work at a verification card and says why in the card's own note field. The
// motive lands on the transition beside the card it was answered at, and the
// landed ledger record still carries it in prose — the note is not moved, it is
// ALSO recorded where a reader can find it without scraping a sentence.
func TestVerifyCardCancelNoteBecomesTheHumanReason(t *testing.T) {
	h := newHarness(t, "SINET_STAGE_FAKE_JUDGE=always-fail")
	ctx := context.Background()
	const owner = "u-operator"
	const note = "not worth more rounds"
	taskID, verifyRunID, askID, _ := driveToOpenCapHit(t, h, owner)

	if _, err := h.sur.Answer(ctx, owner, askID,
		json.RawMessage(`{"choice":"cancel","note":"`+note+`"}`), false); err != nil {
		t.Fatalf("Answer(cancel): %v", err)
	}

	reason, gotAsk := rw19CardCancelReason(t, h, verifyRunID)
	if reason != note {
		t.Errorf("detail.reason = %q, want the card note %q — the note IS the reason channel (R6)", reason, note)
	}
	if gotAsk != askID {
		t.Errorf("cancel detail names card %q, want the answered card %q", gotAsk, askID)
	}
	// The landed record is untouched: the note still rides the ledger text.
	if text := humanLedgerCancelTextRW18(t, h, taskID); !strings.Contains(text, note) {
		t.Errorf("the ledger entry lost the note (%q) — this packet adds a record, it moves none", text)
	}
}

// TestLadderCardCancelNoteReachesEverySiblingTransition — T9, card limb: the
// ladder-terminal card's cancel ends the whole task through CancelTaskAtCard,
// so the note the person typed must reach every sibling the act ended. The
// card's own run had already ended and is left exactly as it was.
func TestLadderCardCancelNoteReachesEverySiblingTransition(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	const owner = "u-operator"
	const taskID = "t-rw19-ladder-cancel"
	const note = "this attempt is not worth repeating"
	askID, _ := seedTombstonedLineage(t, h, taskID, owner)
	sibling := liveSibling(t, h, taskID, owner)

	if _, err := h.sur.Answer(ctx, owner, askID,
		json.RawMessage(`{"choice":"cancel","note":"`+note+`"}`), false); err != nil {
		t.Fatalf("Answer(cancel): %v", err)
	}

	if reason, _ := rw19CardCancelReason(t, h, sibling); reason != note {
		t.Errorf("the sibling's cancel transition carries reason %q, want the card note %q — "+
			"one act ends several runs and each ending is its own record", reason, note)
	}
	if text := humanLedgerCancelTextRW18(t, h, taskID); !strings.Contains(text, note) {
		t.Errorf("the ladder ledger entry lost the note (%q)", text)
	}
}

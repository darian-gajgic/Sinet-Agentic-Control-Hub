package stage_test

// cancelmint_rw18_test.go — P3-RW-18 D1-R1 and OQ1, pinned at the PRODUCER.
//
// The stage half of the same contract the intake half pins
// (internal/intake/cancelmint_rw18_test.go):
//
//  1. A cancel's transition names the HUMAN who took it and carries the one
//     shared structured detail (run.CancelDetail). The verify-card answer used
//     to record `platform` with no detail at all, so the served task detail
//     could not say who had ended the work — the walk-proven gap.
//  2. The human ledger entry keeps its opening words. internal/api's decisions
//     derive recognises a pre-parity cancel BY THAT PREFIX
//     (cancelLedgerPrefixes in reads.go) because `ledger.decide` carries
//     retries and accepts too and the entry has no typed kind. Reword a mint
//     and this test fails, rather than the reader's cancel quietly vanishing
//     from a surface nobody tests from this side (P3-RW-18 OQ1).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

func humanCancelTransitionRW18(t *testing.T, h *harness, runID string) (actor, detailActor, askID string) {
	t.Helper()
	err := h.db.QueryRowContext(context.Background(),
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

func humanLedgerCancelTextRW18(t *testing.T, h *harness, taskID string) string {
	t.Helper()
	doc, found, err := h.led.Current(context.Background(), taskID)
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

// TestVerifyCardCancelRecordsTheHumanAndKeepsItsLedgerPrefix — the walk's own
// shape: a requester ends the work at a verification card.
func TestVerifyCardCancelRecordsTheHumanAndKeepsItsLedgerPrefix(t *testing.T) {
	h := newHarness(t, "SINET_STAGE_FAKE_JUDGE=always-fail")
	ctx := context.Background()
	const owner = "u-operator"
	taskID, verifyRunID, askID, _ := driveToOpenCapHit(t, h, owner)

	if _, err := h.sur.Answer(ctx, owner, askID,
		json.RawMessage(`{"choice":"cancel","note":"not worth more rounds"}`), false); err != nil {
		t.Fatalf("Answer(cancel): %v", err)
	}

	actor, detailActor, gotAsk := humanCancelTransitionRW18(t, h, verifyRunID)
	if actor != owner || detailActor != owner {
		t.Errorf("card-cancel transition actor = %q / detail.actor = %q, want %q (15.6)", actor, detailActor, owner)
	}
	if gotAsk != askID {
		t.Errorf("cancel detail names card %q, want the answered card %q (S2.4 card id)", gotAsk, askID)
	}
	if text := humanLedgerCancelTextRW18(t, h, taskID); !strings.HasPrefix(text, "requester cancelled") {
		t.Errorf("the verify card cancel ledgers %q — internal/api/reads.go reads the prefix %q", text, "requester cancelled")
	}
}

// TestLadderCardCancelKeepsItsLedgerPrefixAndRecordsTheHuman — the recovery
// ladder's terminal card. The card's own run already ended, so the record that
// carries this cancel forward is the ledger entry plus the transitions of the
// live siblings the cancel ends through the verb path.
func TestLadderCardCancelKeepsItsLedgerPrefixAndRecordsTheHuman(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	const owner = "u-operator"
	const taskID = "t-rw18-ladder-cancel"
	askID, _ := seedTombstonedLineage(t, h, taskID, owner)
	sibling := liveSibling(t, h, taskID, owner)

	if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"choice":"cancel"}`), false); err != nil {
		t.Fatalf("Answer(cancel): %v", err)
	}

	// The verb path ends the live sibling, and that transition carries the
	// human — the leg that has always written a detail, kept honest here.
	actor, detailActor, _ := humanCancelTransitionRW18(t, h, sibling)
	if actor != owner || detailActor != owner {
		t.Errorf("sibling cancel transition actor = %q / detail.actor = %q, want %q (15.6)", actor, detailActor, owner)
	}
	if text := humanLedgerCancelTextRW18(t, h, taskID); !strings.HasPrefix(text, "requester cancelled") {
		t.Errorf("the ladder card cancel ledgers %q — internal/api/reads.go reads the prefix %q", text, "requester cancelled")
	}
}

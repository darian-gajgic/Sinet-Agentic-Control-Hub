package stage_test

// gf14detach_e2e_test.go — P3-GF14 at the two surfaces a person actually
// touches:
//
//   - R1 for the S07.7 answer beat: a resumed verification drain is PAID work
//     the requester bought with their answer, so it runs to its own end and
//     lands its verdict even though the page that answered went away (Spec
//     S07.7 "no finding dies silently"; D7).
//   - R4.5's PIN interplay: the lowering answer takes whatever step-up the
//     card's CURRENT tier demands — no exemption in either direction — and once
//     it lands, the refreshed card is what the next answer is measured against.
//
// $0: the judge is the fake engine, the rework seam is the test's.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

func TestGF14ResumedVerifyDrainOutlivesItsCaller(t *testing.T) {
	const owner = "u-operator"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	seam := &abortRevise{cancel: cancel, surviveCancel: true}
	h := newReviseSeamHarness(t, seam, "SINET_STAGE_FAKE_JUDGE=always-fail")

	taskID, verifyRunID, askID, _ := driveToOpenCapHit(t, h, owner)

	// The requester answers with guidance and their page goes away while the
	// bought rework is running.
	seam.arm()
	answer := json.RawMessage(`{"choice":"revise_with_guidance","guidance":[{"text":"apply the agreed marker line","criterion":"AC-1"}]}`)
	// The RESPONSE may die with the caller — it is read on the caller's own
	// context and there is nobody left to send it to. What must not die is the
	// work, so the answer must never come back as the crashed-and-recovering
	// corpse this packet exists to stop minting.
	_, err := h.sur.Answer(ctx, owner, askID, answer, false)
	if err != nil {
		if se := new(api.SurfaceError); errors.As(err, &se) && se.Code == strandCodeRecovering {
			t.Fatalf("the resumed drain was crashed for the ladder because its caller went away: %v", err)
		}
	}
	if ctx.Err() == nil {
		t.Fatal("staging defect: the armed seam never ran, so nothing was cancelled")
	}

	if got := h.runState(t, verifyRunID); got == string(run.StateCrashed) {
		t.Fatal("the verify run crashed — a drain that ran to its end must never end at the ladder")
	}
	if status, _ := h.askStatus(t, askID); status != "answered" {
		t.Fatalf("ask status %q, want answered", status)
	}
	var openAsks int
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM asks a JOIN runs r ON r.run_id = a.run_id
		  WHERE r.task_id = ? AND a.status = 'open'`, taskID).Scan(&openAsks); err != nil {
		t.Fatalf("count open asks: %v", err)
	}
	if openAsks != 1 {
		t.Fatalf("%d open asks after the detached rework, want the drain's own next card", openAsks)
	}
}

// TestGF14LowerStakesTakesTheCardsOwnStepUp is R4.5 as the coordinator settled
// it: the act that REMOVES a high-stakes protection is proven to the PIN holder
// exactly like every other answer on that card, and once the tier has settled
// the refreshed snapshot is what the next answer is measured against.
func TestGF14LowerStakesTakesTheCardsOwnStepUp(t *testing.T) {
	h := newForkHarness(t)
	ctx := context.Background()
	const owner = "u-gf14-pin"

	taskID, _, askID, body := walkToOpenCard(t, h, owner)
	if _, err := h.sur.Answer(ctx, owner, askID, body, false); err != nil {
		t.Fatalf("Answer(interview): %v", err)
	}
	approvalAsk := h.openAskID(taskID)
	if approvalAsk == "" || approvalAsk == askID {
		t.Fatalf("expected a fresh approval card, got %q", approvalAsk)
	}

	// No classifier is wired here, so the task stands at the fail-closed HIGH:
	// the approval card demands the step-up, and the lowering answer is an
	// answer on that card like any other.
	lower := json.RawMessage(`{"action":"lower_stakes","tier":"standard"}`)
	if _, err := h.sur.Answer(ctx, owner, approvalAsk, lower, false); !errors.Is(err, api.ErrPINRequired) {
		t.Fatalf("lowering at high tier without the step-up = %v, want the PIN demand — the act that "+
			"removes a high-stakes protection is proven to the PIN holder (S06.4 explicit requester action)", err)
	}
	if _, err := h.sur.Answer(ctx, owner, approvalAsk, lower, true); err != nil {
		t.Fatalf("lowering with the step-up: %v", err)
	}
	if got := h.openAskID(taskID); got != approvalAsk {
		t.Fatalf("open ask = %q, want the approval card still standing at %q", got, approvalAsk)
	}

	// The snapshot moved with the tier, so the next answer reads the settled
	// stakes rather than the ones the card was written with.
	if _, err := h.sur.Answer(ctx, owner, approvalAsk, json.RawMessage(`{"action":"approve"}`), false); err != nil {
		t.Fatalf("approving after the lowering: %v — the step-up must follow the settled tier, not the stale snapshot", err)
	}
}

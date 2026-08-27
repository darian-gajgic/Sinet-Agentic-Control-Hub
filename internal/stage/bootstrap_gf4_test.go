package stage_test

// bootstrap_gf4_test.go — P3-GF4 at the STAGE boundary (Spec S07.8 bootstrap
// posture, A14 2026-08-27). The wedge test one file over pins that a launch
// domain with NO pack machinery still parks on an answerable card; this file
// pins the case A14 defines instead: a REGISTERED project holding no
// build/test/lint command verifies, completes, and is handed to the requester
// with the posture named in plain words.
//
// The operator's actual wall (P3/design/b6-gate-operator-findings-r4:
// "the retry can never succeed") is the second test: answering `retry` on a
// pre-GF4 infrastructure card now lands in the bootstrap posture instead of
// re-parking on the identical refusal.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// bootstrapJudge passes every frozen criterion with the artifact itself as the
// extractive evidence quote (a substring by construction), so the drain's
// terminal is decided by the POSTURE under test and not by judge weather.
type bootstrapJudge struct{}

func (bootstrapJudge) Compliance(_ context.Context, in verify.JudgeInput) (verify.Axis1Result, error) {
	var out verify.Axis1Result
	for _, ac := range in.ACs {
		out.Verdicts = append(out.Verdicts, verify.ACVerdict{
			Key: fmt.Sprintf("AC-%d", ac.N), Pass: true, Evidence: in.Artifact,
		})
	}
	return out, nil
}

func (bootstrapJudge) Sanity(context.Context, verify.JudgeInput) (verify.Axis2Result, error) {
	return verify.Axis2Result{ProbeNotes: map[verify.Probe]string{
		verify.ProbeReasonableUser:       "the note reads as asked",
		verify.ProbeImplicitExpectations: "nothing obvious is missing",
		verify.ProbeSideEffects:          "no unrequested changes",
		verify.ProbeExpertStandard:       "competent",
	}}, nil
}

func (bootstrapJudge) Meta() verify.JudgeMeta {
	return verify.JudgeMeta{Model: "bootstrap-judge-1", SelfFamily: true}
}

// openAskIDs returns the task's open ask ids.
func openAskIDs(t *testing.T, h *harness, taskID string) []string {
	t.Helper()
	rows, err := h.db.QueryContext(context.Background(),
		`SELECT a.ask_id FROM asks a JOIN runs r ON r.run_id = a.run_id
		  WHERE r.task_id = ? AND a.status = 'open'`, taskID)
	if err != nil {
		t.Fatalf("read open asks: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestGF4BootstrapEndToEnd [R1, R6, R7, R12]: the full give-work journey on a
// software task whose registered project holds no commands. The run does not
// park; the deliverable lands in review for the human who must decide it; no
// ledger item is flipped to verified on the advisory verdict; and the receipt
// says why in plain words.
func TestGF4BootstrapEndToEnd(t *testing.T) {
	ctx := context.Background()
	h := outageHarness(t, bootstrapJudge{}, func(context.Context, string, string) (*verify.CheckPack, error) {
		return verify.BootstrapPack(verify.DomainSoftware, 1), nil
	})
	taskID := walkToVerify(t, h, "software")

	verifyRun := taskID + ".verify"
	if got := h.runState(t, verifyRun); got != "completed" {
		t.Fatalf("verify run is %q, want completed — the bootstrap drain runs to a verdict and never parks on pack absence (Spec S07.8)", got)
	}
	if asks := openAskIDs(t, h, taskID); len(asks) != 0 {
		for _, id := range asks {
			card := h.openCard(t, id)
			t.Errorf("open ask %s: %s — %s", id, card.Category, card.Summary)
		}
		t.Fatal("the bootstrap drain left an open card")
	}
	raw, err := h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	if got := decodeView(t, raw).Kanban; got != "done" {
		t.Fatalf("kanban = %q, want done (the drain reached a verdict; the review is the human's next act)", got)
	}

	// The requester's review is the real gate: the deliverable waits in review
	// and no code path accepted it (Spec S07.8 mandatory V3, S13.6 accept).
	dlv, err := h.review.Deliverable(ctx, stage.TaskDeliverableID(taskID))
	if err != nil {
		t.Fatalf("Deliverable: %v", err)
	}
	if dlv.State != review.StateInReview {
		t.Fatalf("deliverable state %q, want %q — nothing auto-accepts a bootstrap-verified deliverable", dlv.State, review.StateInReview)
	}

	// The advisory verdict flipped nothing to verified (OQ2).
	doc, found, err := h.led.Current(ctx, taskID)
	if err != nil || !found {
		t.Fatalf("ledger doc: %v found=%v", err, found)
	}
	for _, it := range doc.State.Items {
		if it.Status == ledger.StatusVerified {
			t.Fatalf("work item %s is verified although no check ran (Spec S05.1/S07.11)", it.ID)
		}
	}

	// The verdict row carries the posture, and the receipt says it in words.
	if !strings.Contains(roundRowsFor(t, h, verifyRun), string(verify.PostureBootstrap)) {
		t.Fatal("no verdict.recorded row carries the bootstrap posture")
	}
	receipt, err := h.sur.Receipt(ctx, verifyRun)
	if err != nil {
		t.Fatalf("Receipt: %v", err)
	}
	if !strings.Contains(string(receipt), "restores the full ladder") {
		t.Fatalf("the receipt does not name the bootstrap posture in plain words (Spec S07.8/S07.11): %s", receipt)
	}
}

// roundRowsFor returns the run's verdict.recorded payloads concatenated.
func roundRowsFor(t *testing.T, h *harness, runID string) string {
	t.Helper()
	rows, err := h.db.QueryContext(context.Background(),
		`SELECT payload FROM run_events WHERE run_id = ? AND type = ? ORDER BY event_seq`, runID, verify.EventRound)
	if err != nil {
		t.Fatalf("read verdict rows: %v", err)
	}
	defer rows.Close()
	var all strings.Builder
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		all.WriteString(p)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return all.String()
}

// TestGF4RetryOnPreGF4CardLandsInBootstrap [R1]: the operator's wall. A run
// parked on a pre-GF4 infrastructure card is answered `retry` while the
// project is STILL command-less — and instead of re-parking on the identical
// refusal, the resumed drain runs under the bootstrap posture and finishes.
func TestGF4RetryOnPreGF4CardLandsInBootstrap(t *testing.T) {
	ctx := context.Background()
	const owner = "u-operator"
	bootstrap := false
	h := outageHarness(t, bootstrapJudge{}, func(context.Context, string, string) (*verify.CheckPack, error) {
		if bootstrap {
			return verify.BootstrapPack(verify.DomainSoftware, 1), nil
		}
		// The card that parked this run: a surviving S07.7 integrity refusal
		// (no registered project at all — OQ1), whose door the operator then
		// walks through by registering the project and attaching the task.
		return nil, fmt.Errorf("%w: this software task is not attached to a registered project, so the platform has no build or test commands to check it with",
			verify.ErrNoCheckPack)
	})
	taskID := walkToVerify(t, h, "software")

	verifyRun := taskID + ".verify"
	if got := h.runState(t, verifyRun); got != "parked" {
		t.Fatalf("verify run is %q, want parked on the pre-GF4 infrastructure card", got)
	}
	asks := openAskIDs(t, h, taskID)
	if len(asks) != 1 {
		t.Fatalf("open asks = %v, want exactly the infrastructure card", asks)
	}
	askID := asks[0]
	if !strings.HasPrefix(askID, verify.InfraAskPrefix) {
		t.Fatalf("open ask %q is not the infrastructure card shape %q", askID, verify.InfraAskPrefix)
	}

	// The A14 landing: the seam now answers bootstrap, and `retry` succeeds.
	bootstrap = true
	if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"choice":"retry"}`), false); err != nil {
		t.Fatalf("Answer(retry): %v", err)
	}
	if got := h.runState(t, verifyRun); got != "completed" {
		t.Fatalf("verify run is %q after retry, want completed — the retry that could never succeed now lands in the bootstrap posture", got)
	}
	if asks := openAskIDs(t, h, taskID); len(asks) != 0 {
		t.Fatalf("retry re-parked on %v — the wall is still there", asks)
	}
	if !strings.Contains(roundRowsFor(t, h, verifyRun), string(verify.PostureBootstrap)) {
		t.Fatal("the resumed drain recorded no bootstrap round")
	}
}

// TestGF4NonBootstrapReceiptIsUnchanged [R7]: a run whose drain never touched
// the posture serves the receipt row byte for byte as it was materialized —
// the posture member is composed in, never a rewrite of everyone's receipt.
func TestGF4NonBootstrapReceiptIsUnchanged(t *testing.T) {
	ctx := context.Background()
	h := outageHarness(t, bootstrapJudge{}, nil)
	taskID := walkToVerify(t, h, "generic")

	verifyRun := taskID + ".verify"
	if got := h.runState(t, verifyRun); got != "completed" {
		t.Fatalf("verify run is %q, want completed", got)
	}
	var stored string
	if err := h.db.QueryRowContext(ctx,
		`SELECT usage_json FROM receipts WHERE run_id = ?`, verifyRun).Scan(&stored); err != nil {
		t.Fatalf("read the materialized receipt: %v", err)
	}
	served, err := h.sur.Receipt(ctx, verifyRun)
	if err != nil {
		t.Fatalf("Receipt: %v", err)
	}
	if string(served) != stored {
		t.Fatalf("served receipt differs from the materialized row:\n served = %s\n stored = %s", served, stored)
	}
	if strings.Contains(string(served), "restores the full ladder") {
		t.Fatalf("a full-posture run's receipt carries the bootstrap statement: %s", served)
	}
}

package stage_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// tq5_route_e2e_test.go — P3-TQ-5 acceptance at the real call site (Spec S08.8
// step 3 + accountability; gate record P3/gates/rework-sitting-gate.md B7).
// With nothing commissioned (the routed harness composes no commissioned
// lane), the execution duty's first choices — K3 on kimi-cli, then K3 on
// kimi — are not available, so execution resolves to claude-opus-5 on the
// anthropic lane: never an error, never a park, and the routing reason names
// the skipped first choices in plain words. The run row names the seat that
// actually ran (P3-LN-9's stamp; here a no-op because the decided lane IS the
// configured one).

func TestTQ5InterimExecuteRunsOnOpus5AndNamesTheSkippedFirstChoice(t *testing.T) {
	h := newRoutedHarness(t)
	ctx := context.Background()
	const owner = "u-tq5"

	taskID, askID, cardJSON := h.walkToApproval(ctx, owner)

	// Visible before execution (S08.8): the approval card's routing block
	// already says the first choice was skipped and why.
	if !strings.Contains(cardJSON, `"routing"`) {
		t.Fatalf("approval card missing the routing block: %s", cardJSON)
	}
	if !strings.Contains(cardJSON, "kimi-cli") {
		t.Errorf("the card's routing reason does not name the skipped first choice (kimi-cli): %s", cardJSON)
	}

	if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"action":"approve"}`), false); err != nil {
		t.Fatalf("Answer(approve): %v", err)
	}
	if n := h.tick(ctx); n < 1 {
		t.Fatalf("execute tick dispatched %d", n)
	}

	execRun := taskID + ".execute"
	decided := eventsOfType(t, h.db, execRun, "routing.decided")
	if len(decided) != 1 {
		t.Fatalf("routing.decided events = %d, want exactly 1", len(decided))
	}
	d := decided[0]
	if d["model"] != "claude-opus-5" || d["lane"] != adapters.LaneAnthropic {
		t.Fatalf("routing.decided seat = %v/%v, want claude-opus-5 on anthropic (the interim executor until a Kimi key is placed)", d["model"], d["lane"])
	}
	if d["gap_advice"] != nil {
		t.Fatalf("the interim must never be a subscription gap: %v", d["gap_advice"])
	}
	reason, _ := d["plain_reason"].(string)
	for _, skipped := range []string{"kimi-cli", "kimi"} {
		if !strings.Contains(reason, skipped) {
			t.Errorf("plain_reason does not name the skipped lane %q: %q", skipped, reason)
		}
	}
	if strings.Contains(reason, "Not covered by a subscription") {
		t.Errorf("plain_reason carries the 2.7 gap sentence on a resolved seat: %q", reason)
	}

	// The run row tells the truth about the seat that ran (S10.1; LN-9 R9).
	var lane, substrate string
	if err := h.db.QueryRowContext(ctx, `SELECT lane, substrate FROM runs WHERE run_id = ?`, execRun).Scan(&lane, &substrate); err != nil {
		t.Fatalf("run row: %v", err)
	}
	if lane != adapters.LaneAnthropic || substrate != adapters.SubstrateClaudeCLI {
		t.Errorf("runs.lane/substrate = %s/%s, want %s/%s — the row must name what ran", lane, substrate, adapters.LaneAnthropic, adapters.SubstrateClaudeCLI)
	}
}

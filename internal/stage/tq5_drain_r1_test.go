package stage_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// tq5_drain_r1_test.go — P3-TQ-5 drain r1, F1 (Spec S07.11: the verdict rows
// are keep-forever and a run may hold several). The receipt discloses the
// check the requester is actually being handed, which is the LAST one that
// names a judge — a rework round judged by a different seat must move the
// line, and a later row that names no judge must not erase it.
//
// The name states the rule the code implements: last row WITH a judge wins.
// A row carrying no judge_model discloses nothing, so it leaves the last
// judged round standing rather than blanking the receipt (the "an absent key
// is absent, never invented" reading the receipt already takes for
// RequestIDs).
func TestTQ5ReceiptJudgeLineTakesTheLastRowWithAJudge(t *testing.T) {
	ctx := context.Background()
	h := outageHarness(t, tq5Judge{}, func(context.Context, string, string) (*verify.CheckPack, error) {
		return verify.BootstrapPack(verify.DomainSoftware, 1), nil
	})
	taskID := walkToVerify(t, h, "software")
	verifyRun := taskID + ".verify"

	r, err := h.runs.Get(ctx, verifyRun)
	if err != nil {
		t.Fatalf("read the verify run: %v", err)
	}
	appendRound := func(payload string) {
		t.Helper()
		if _, err := h.log.Append(ctx, eventlog.Append{
			RunID: verifyRun, Generation: r.Generation, UserID: r.UserID,
			Type: verify.EventRound, SchemaVersion: 1,
			Payload: json.RawMessage(payload),
		}); err != nil {
			t.Fatalf("append a verdict row: %v", err)
		}
	}
	judgeOf := func(what string) *metering.JudgeLine {
		t.Helper()
		raw, err := h.sur.Receipt(ctx, verifyRun)
		if err != nil {
			t.Fatalf("Receipt (%s): %v", what, err)
		}
		var rec metering.Receipt
		if err := json.Unmarshal(raw, &rec); err != nil {
			t.Fatalf("decode receipt (%s): %v", what, err)
		}
		if rec.Judge == nil {
			t.Fatalf("no judge line on the receipt (%s): %s", what, raw)
		}
		return rec.Judge
	}

	// The drain's own round: the interim seat, self-family.
	if got := judgeOf("the drain's round"); got.Model != "claude-opus-5" || !got.SelfFamily {
		t.Fatalf("first judge line = %+v, want claude-opus-5 self-family", got)
	}

	// A later round judged by a DIFFERENT seat, cross-family. The receipt must
	// follow it: disclosing the first round's judge would describe a check the
	// requester is not being handed.
	appendRound(`{"round":2,"judge_model":"kimi-judge-1","self_family_judge":false}`)
	got := judgeOf("after a second, cross-family round")
	if got.Model != "kimi-judge-1" {
		t.Errorf("judge model = %q, want the LAST judged round's kimi-judge-1", got.Model)
	}
	if got.SelfFamily {
		t.Errorf("self_family stayed true although the last round was judged cross-family: %+v", got)
	}
	if !strings.Contains(got.Note, "different model family") {
		t.Errorf("the note did not follow the flag: %q", got.Note)
	}

	// A later row that names NO judge discloses nothing and leaves the last
	// judged round standing.
	appendRound(`{"round":3}`)
	if got := judgeOf("after a row naming no judge"); got.Model != "kimi-judge-1" || got.SelfFamily {
		t.Errorf("judge line = %+v after a judge-less row, want the last JUDGED round's kimi-judge-1", got)
	}
}

package stage_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// tq5_receiptjudge_test.go — P3-TQ-5 acceptance checklist 11 (Spec S07.5 /
// G1 Def.1 "self-family judging is always flagged on the receipt"; S07.11;
// S10.10). The flag has lived on the verdict row since B2-4 and reached no
// receipt. Here the whole path runs: a drained task's receipt carries the
// judge line, composed at the serving side from the run's keep-forever
// verdict rows, and the two records agree.
//
// The judge is injected, so the line under test is the COMPOSITION and not
// the seat resolution (which internal tests in this package pin directly).

type tq5Judge struct{}

func (tq5Judge) Compliance(_ context.Context, in verify.JudgeInput) (verify.Axis1Result, error) {
	var out verify.Axis1Result
	for _, ac := range in.ACs {
		out.Verdicts = append(out.Verdicts, verify.ACVerdict{
			Key: fmt.Sprintf("AC-%d", ac.N), Pass: true, Evidence: in.Artifact,
		})
	}
	return out, nil
}

func (tq5Judge) Sanity(context.Context, verify.JudgeInput) (verify.Axis2Result, error) {
	return verify.Axis2Result{ProbeNotes: map[verify.Probe]string{
		verify.ProbeReasonableUser:       "the note reads as asked",
		verify.ProbeImplicitExpectations: "nothing obvious is missing",
		verify.ProbeSideEffects:          "no unrequested changes",
		verify.ProbeExpertStandard:       "competent",
	}}, nil
}

// The interim seat: until a Kimi key is placed the same model does the work
// and checks it, so the flag is honestly true.
func (tq5Judge) Meta() verify.JudgeMeta {
	return verify.JudgeMeta{Model: "claude-opus-5", SelfFamily: true}
}

func TestTQ5ReceiptCarriesTheJudgeLine(t *testing.T) {
	ctx := context.Background()
	h := outageHarness(t, tq5Judge{}, func(context.Context, string, string) (*verify.CheckPack, error) {
		return verify.BootstrapPack(verify.DomainSoftware, 1), nil
	})
	taskID := walkToVerify(t, h, "software")
	verifyRun := taskID + ".verify"

	raw, err := h.sur.Receipt(ctx, verifyRun)
	if err != nil {
		t.Fatalf("Receipt: %v", err)
	}
	var r metering.Receipt
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("decode receipt: %v (%s)", err, raw)
	}
	if r.Judge == nil {
		t.Fatalf("the receipt carries no judge line although the run reached a verdict: %s", raw)
	}
	if r.Judge.Model != "claude-opus-5" {
		t.Errorf("receipt judge model = %q, want the model that judged", r.Judge.Model)
	}
	if !r.Judge.SelfFamily {
		t.Error("receipt judge line says the check was independent although the same family did both")
	}
	if r.Judge.Note == "" {
		t.Error("the judge line carries no plain-words note — the flag exists so a requester can read it")
	}
	for _, cite := range []string{"S07", "S10", "G1 Def", "(D1"} {
		if strings.Contains(r.Judge.Note, cite) {
			t.Errorf("the judge note cites %q on a requester surface: %q", cite, r.Judge.Note)
		}
	}

	// The two records are one fact: the verdict row and the receipt agree.
	rows := roundRowsFor(t, h, verifyRun)
	if !strings.Contains(rows, `"judge_model":"claude-opus-5"`) {
		t.Errorf("no verdict row names the judge model the receipt served: %s", rows)
	}
	if !strings.Contains(rows, `"self_family_judge":true`) {
		t.Errorf("no verdict row carries the self-family flag the receipt served: %s", rows)
	}
}

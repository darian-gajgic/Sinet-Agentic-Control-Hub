package stage_test

// nothink_test.go — PH-1 F1: which duties send the think phase away, and the
// promise that fixing the phrase seat did NOT mean buying more tokens.
//
// `NoThink` used to be `Classification`, so the two DRAFTING duties on the
// utility alias — card phrasing and 13.5 help — kept a reasoning model's think
// phase on while asking it for engine-constrained JSON. The think phase is
// emitted BEFORE the constrained region, so it spent the whole cap and the
// JSON region never opened: 4000 output tokens, zero content, every card of
// cold walk 1 (P3/design/ph1-phrase-fallback-diagnosis-2026-08-17.md).

import (
	"context"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
)

// localFake is a short, schema-shaped reply that finishes well under any cap.
func localFake(content string) local.FakeResponse {
	return local.FakeResponse{Content: content, InputTokens: 120, OutputTokens: 40}
}

// TestConstrainedDutiesSuppressTheThinkPhase (PH-1 F1): every duty that asks
// the engine for schema-constrained JSON — drafting ones included — sends
// enable_thinking=false, and the caps are exactly the ones that were already
// there. The fix is a budget the JSON can reach, never a bigger budget.
func TestConstrainedDutiesSuppressTheThinkPhase(t *testing.T) {
	ctx := context.Background()

	t.Run("phrase (drafting)", func(t *testing.T) {
		duty, fake, _, _ := localSeamEnv(t)
		seat := stage.NewLocalUtilitySeat(duty)
		fake.SetModelResponse("Qwen3.5-9B", localFake(`{"reason":"r","summary":"s","behavior":"b","technology_stack":"t"}`))
		if _, err := seat.PhraseAndSummarize(ctx, phraseInput("t1.intake")); err != nil {
			t.Fatalf("PhraseAndSummarize: %v", err)
		}
		req := fake.LastRequest()
		if !req.NoThink {
			t.Error("the phrase duty left the think phase ON — the cap is then spent on reasoning and the schema region never opens (PH-1 F1)")
		}
		if req.MaxTokens != 4000 {
			t.Errorf("phrase cap = %d, want the UNCHANGED 4000 — PH-1's fix is not another cap raise (S18: no ⚙ key)", req.MaxTokens)
		}
	})

	t.Run("help (drafting)", func(t *testing.T) {
		duty, fake, _, _ := localSeamEnv(t)
		fake.SetModelResponse("Qwen3.5-9B", localFake(`{"what":"w","wrong":"x","recommend":"y"}`))
		pair := intake.Pair{Spec: intake.Spec{TaskID: "t1", Restatement: "do the thing"}}
		if _, err := stage.NewLocalUtility(duty).Help(ctx, pair); err != nil {
			t.Fatalf("Help: %v", err)
		}
		if req := fake.LastRequest(); !req.NoThink {
			t.Error("the help duty left the think phase ON — it failed identically to phrase on cold walk 1 (cp 13, 700/700 tokens)")
		}
	})

	// The classification duties were already correct and must stay BYTE-identical:
	// their think phase was off because Classification was true, and it is still
	// off now that the two questions are separate fields.
	t.Run("classify (unchanged)", func(t *testing.T) {
		duty, fake, _, _ := localSeamEnv(t)
		fake.SetModelResponse("Qwen3.5-4B", localFake(`{"family":"software","stakes":"low","size":"small","data_bearing":false,"abstain":false}`))
		if _, err := stage.NewLocalClassifier(duty).Classify(ctx, intake.TriageInput{
			RunID:   "t1.intake",
			Request: intake.Request{TaskID: "t1", Title: "webshop", Text: "Create a simple webshop for car parts"},
		}); err != nil {
			t.Fatalf("Classify: %v", err)
		}
		req := fake.LastRequest()
		if !req.NoThink {
			t.Error("classify stopped suppressing the think phase — decoupling the flags must change nothing here")
		}
		if !req.Logprobs || req.MaxTokens != 512 || req.Temperature != 0 {
			t.Errorf("classify request shape drifted: logprobs=%v cap=%d temp=%v (want true/512/0)", req.Logprobs, req.MaxTokens, req.Temperature)
		}
	})
}

package stage_test

// triagefamily_test.go — P3-RW-11 T7 (brief §7; R5): the intake-triage adapter
// has exactly one silent-generic path left in it, and this closes it. An
// INVALID family label off the model becomes UNRESOLVED (so the family question
// fires), never a quiet `generic` the requester was never told about. Abstain
// and a valid label keep their landed behavior.
//
// Driven through the real seam against the tier-F fake /v1 (the §10 posture:
// behavior asserted, never the parse function in isolation).

import (
	"context"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
)

func TestParseTriageInvalidFamilyUnresolved(t *testing.T) {
	ctx := context.Background()
	req := intake.Request{TaskID: "t1", Title: "fix", Text: "fix the parser"}

	t.Run("invalid label is unresolved, not generic", func(t *testing.T) {
		duty, fake, _, _ := localSeamEnv(t)
		fake.SetModelResponse("Qwen3.5-4B", local.FakeResponse{
			Content:     `{"family":"bogus","stakes":"low","size":"small","data_bearing":false,"abstain":false}`,
			InputTokens: 40, OutputTokens: 6,
		})
		p, err := stage.NewLocalClassifier(duty).Classify(ctx, req, nil)
		if err != nil {
			t.Fatalf("Classify: %v", err)
		}
		if p.Family != "" {
			t.Errorf("family = %q, want \"\" (unresolved — an invalid label must ASK, never silently generic, R5)", p.Family)
		}
		if p.Tier != intake.TierLow {
			t.Errorf("tier = %q, want low — the family defect must not disturb the stakes reading", p.Tier)
		}
	})

	t.Run("abstain still errors", func(t *testing.T) {
		duty, fake, _, _ := localSeamEnv(t)
		fake.SetModelResponse("Qwen3.5-4B", local.FakeResponse{Content: `{"abstain":true}`, InputTokens: 5, OutputTokens: 1})
		if _, err := stage.NewLocalClassifier(duty).Classify(ctx, req, nil); err == nil {
			t.Error("abstain must still error so the pipeline fails closed to high (S06.2, unchanged)")
		}
	})

	t.Run("valid label still resolves", func(t *testing.T) {
		duty, fake, _, _ := localSeamEnv(t)
		fake.SetModelResponse("Qwen3.5-4B", local.FakeResponse{
			Content:     `{"family":"software","stakes":"standard","size":"small","data_bearing":false,"abstain":false}`,
			InputTokens: 40, OutputTokens: 6,
		})
		p, err := stage.NewLocalClassifier(duty).Classify(ctx, req, nil)
		if err != nil {
			t.Fatalf("Classify: %v", err)
		}
		if p.Family != intake.FamilySoftware {
			t.Errorf("family = %q, want software", p.Family)
		}
	})
}

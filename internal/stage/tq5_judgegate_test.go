package stage_test

import (
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// tq5_judgegate_test.go — P3-TQ-5 sequencing tripwire (Spec S07.9 P-T06-5:
// "any judge-model change gates on a golden-set re-run before unsupervised
// judging resumes"). The shipped judge seat and the shipped rubric's measured
// pin must agree at EVERY commit: stage.newVerifier refuses with a P-T06-5
// decision card otherwise, which would turn every verify leg into a card. This
// is GREEN at grounding (v2 ↔ claude-opus-4-8) and goes red on any tree that
// flips the judge seat before the re-measured rubric v3 lands beside it.
func TestTQ5ShippedJudgeSeatIsTheRubricsMeasuredPin(t *testing.T) {
	seat := worker.DefaultDutyMap()[worker.DutyJudge].Model
	if err := verify.UnsupervisedJudgingGate(verify.SeedSoftwareRubric(), seat); err != nil {
		t.Fatalf("the shipped judge seat %q cannot drive unsupervised judging under the shipped rubric: %v — "+
			"the seat flip and the re-measured rubric land TOGETHER, never the seat first", seat, err)
	}
}

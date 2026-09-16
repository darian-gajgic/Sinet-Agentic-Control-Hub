package stage

import (
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// tq5_judge_internal_test.go — P3-TQ-5 acceptance (Spec S07.5 judge routing,
// S07.11 "judge model" on every verdict row). The judge names the JUDGE seat.
// Today Meta() answers modelFor(""), which falls through to the PLANNING seat:
// equal by data on the shipped map, and a lie the moment the two rows differ.

func TestTQ5JudgeMetaNamesTheJudgeSeatNotThePlanningSeat(t *testing.T) {
	sk := &Skeleton{cfg: Config{}, dutyMap: worker.DutyMap{
		worker.DutyExecution: {Model: "claude-opus-5", Lane: "anthropic", WindowTokens: worker.DefaultWindowTokens},
		worker.DutyPlanning:  {Model: "planning-seat-model", Lane: "anthropic", WindowTokens: worker.DefaultWindowTokens},
		worker.DutyJudge:     {Model: "judge-seat-model", Lane: "anthropic", WindowTokens: worker.DefaultWindowTokens},
	}}
	j := &EngineJudge{s: sk}
	if got := j.Meta().Model; got != "judge-seat-model" {
		t.Fatalf("EngineJudge.Meta().Model = %q, want the judge seat's model (got the planning seat's)", got)
	}
}

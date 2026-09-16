package stage

import (
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// tq5_selffamily_internal_test.go — P3-TQ-5 acceptance (Spec S07.5 / G1 Def.1:
// "self-family judging is always flagged on the receipt"; S07.11). The flag is
// COMPUTED from the executor that ran this task and the judge seat — never a
// constant. Until a Kimi key is placed the executor IS claude-opus-5 and the
// flag is honestly true; once K3 executes and Opus 5 judges it is false.
//
// RED-BY-COMPILE at grounding: newEngineJudge (the judge-construction seam
// taking the executor's model) and sameFamily (the pure rule) do not exist yet.

func TestTQ5SelfFamilyIsComputedFromTheExecutorThatRan(t *testing.T) {
	sk := &Skeleton{cfg: Config{}, dutyMap: worker.DutyMap{
		worker.DutyExecution: {Model: "claude-opus-5", Lane: "anthropic", WindowTokens: worker.DefaultWindowTokens},
		worker.DutyPlanning:  {Model: "claude-opus-5", Lane: "anthropic", WindowTokens: worker.DefaultWindowTokens},
		worker.DutyJudge:     {Model: "claude-opus-5", Lane: "anthropic", WindowTokens: worker.DefaultWindowTokens},
	}}
	cases := []struct {
		executor string
		want     bool
	}{
		{"claude-opus-5", true},   // the interim: the same model checks its own work
		{"claude-sonnet-5", true}, // the same vendor family
		{"k3", false},             // K3 did the work, Opus 5 judges it: cross-vendor
		{"glm-5.3", false},
		{"", true}, // no recorded selection: the configured execution seat is what would have run
	}
	for _, c := range cases {
		m := newEngineJudge(sk, c.executor).Meta()
		if m.Model != "claude-opus-5" {
			t.Errorf("executor %q: judge model = %q, want the judge seat claude-opus-5", c.executor, m.Model)
		}
		if m.SelfFamily != c.want {
			t.Errorf("executor %q: self_family = %v, want %v", c.executor, m.SelfFamily, c.want)
		}
	}
}

func TestTQ5SameFamilyRule(t *testing.T) {
	cases := []struct {
		exec, judge string
		want        bool
	}{
		{"claude-opus-5", "claude-opus-5", true},
		{"claude-sonnet-5", "claude-opus-5", true},
		{"claude-opus-4-8", "claude-opus-5", true},
		{"k3", "claude-opus-5", false},
		{"kimi-for-coding", "claude-opus-5", false},
		{"glm-5.3", "claude-opus-5", false},
		{"", "claude-opus-5", false},
		{"claude-opus-5", "", false},
	}
	for _, c := range cases {
		if got := sameFamily(c.exec, c.judge); got != c.want {
			t.Errorf("sameFamily(%q, %q) = %v, want %v", c.exec, c.judge, got, c.want)
		}
	}
}

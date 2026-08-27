package intake

import (
	"errors"
	"strings"
	"testing"
)

// gf8impl_internal_test.go — the P3-GF8 requirements whose surface is
// package-internal: the structural caps on the [A15] members (brief R11 / §6)
// and the approved method reaching execution through the plan seam (R15).

// a15Step returns a step whose [A15] members are complete, for a caps test to
// spoil one field at a time.
func a15Step() Step {
	return Step{
		ID: "S-1", Title: "Do the work", DoneWhen: "tests pass", Class: "C1",
		Approach: "I edit the one module involved and leave its public surface alone.",
		Decisions: []StepDecision{{
			Decision:     "edit in place rather than rewrite",
			Alternatives: []string{"rewrite the module from scratch"},
			Why:          "the module is small and a rewrite risks the parts that already work",
		}},
		OrderingRationale: "the change must exist before anything can verify it",
	}
}

func a15Plan(step Step) Plan {
	return Plan{
		TaskID: "t1", Owner: "u1", Version: 1, SpecVersion: 1, Status: StatusDraft,
		Steps: []Step{step}, Coverage: map[string][]string{},
	}
}

// TestGF8ApproachCapsRefuseAtTheBoundary (brief R11, §6 structural constants):
// each [A15] member carries a named cap, and the boundary — not a later
// renderer — is where an over-long one is refused. The bounds are read from the
// constants themselves, so the test cannot drift from the rule (§43).
func TestGF8ApproachCapsRefuseAtTheBoundary(t *testing.T) {
	complete := a15Plan(a15Step())
	if err := complete.Validate(); err != nil {
		t.Fatalf("the complete step must validate: %v", err)
	}

	cases := []struct {
		name  string
		spoil func(s *Step)
	}{
		{"approach over its cap", func(s *Step) {
			s.Approach = strings.Repeat("é", approachMaxRunes+1)
		}},
		{"ordering rationale over its cap", func(s *Step) {
			s.OrderingRationale = strings.Repeat("é", orderingMaxRunes+1)
		}},
		{"one decision field over its cap", func(s *Step) {
			s.Decisions[0].Why = strings.Repeat("é", decisionFieldMaxRunes+1)
		}},
		{"one alternative over its cap", func(s *Step) {
			s.Decisions[0].Alternatives = []string{strings.Repeat("é", decisionFieldMaxRunes+1)}
		}},
		{"more decisions than a step should hold", func(s *Step) {
			for len(s.Decisions) <= maxStepDecisions {
				s.Decisions = append(s.Decisions, s.Decisions[0])
			}
		}},
		{"an alternative that says nothing", func(s *Step) {
			s.Decisions[0].Alternatives = []string{"   "}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := a15Step()
			tc.spoil(&s)
			p := a15Plan(s)
			if err := p.Validate(); !errors.Is(err, ErrBadArtifact) {
				t.Errorf("the boundary accepted %s; got %v", tc.name, err)
			}
		})
	}

	// Runes, not bytes: a cap-length approach of multi-byte characters is
	// exactly at the bound and passes.
	atCap := a15Plan(a15Step())
	atCap.Steps[0].Approach = strings.Repeat("é", approachMaxRunes)
	if err := atCap.Validate(); err != nil {
		t.Errorf("an approach exactly at the cap must pass (the cap counts runes): %v", err)
	}
}

// TestGF8StepContractCarriesTheApprovedApproach (brief R15; OQ3 ratified YES):
// the executing stage receives the approved HOW instead of re-deriving it. The
// lines are GUARDED, so a pre-A15 plan's step contract is byte-identical to
// what it was.
func TestGF8StepContractCarriesTheApprovedApproach(t *testing.T) {
	step := a15Step()
	plan := a15Plan(step)

	got := stepContract(&plan.Steps[0], &plan)
	for _, want := range []string{
		"I edit the one module involved",
		"edit in place rather than rewrite",
		"rewrite the module from scratch",
		"the module is small and a rewrite risks",
		"the change must exist before anything can verify it",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the step contract does not carry %q — execution would re-derive the approved method (brief R15):\n%s", want, got)
		}
	}

	bare := Step{ID: "S-1", Title: "Do the work", DoneWhen: "tests pass", Class: "C1"}
	barePlan := a15Plan(bare)
	bareGot := stepContract(&barePlan.Steps[0], &barePlan)
	for _, forbidden := range []string{"Approach", "Decision", "Ordering"} {
		if strings.Contains(bareGot, forbidden) {
			t.Errorf("a pre-A15 step's contract gained a %q line: %s", forbidden, bareGot)
		}
	}
}

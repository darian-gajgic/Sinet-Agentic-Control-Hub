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

// gf8TargetPair is a two-step pair to resolve contest targets against.
func gf8TargetPair() *Pair {
	return &Pair{
		Spec: Spec{
			TaskID: "t1", Owner: "u1", Version: 1, Status: StatusDraft,
			Restatement: "Fix the widget.",
			Outcome:     []string{"the widget works"},
			ACs:         []AC{{N: 1, Plain: "the change works"}},
			Constraints: []string{"stay within the repo"},
			OutOfScope:  []string{"no deploys"},
		},
		Plan: Plan{
			TaskID: "t1", Owner: "u1", Version: 1, SpecVersion: 1, Status: StatusDraft,
			Steps: []Step{
				{ID: "S-1", Title: "Implement", DoneWhen: "tests pass", Class: "C1",
					Approach: "I edit the widget module in place."},
				{ID: "S-2", Title: "Verify", DoneWhen: "ACs hold", Class: "C1",
					Approach: "I run the project's checks."},
			},
			Coverage: map[string][]string{"AC-1": {"S-1"}},
			Risks:    []string{"the estimate may be off"},
		},
	}
}

// TestGF8KeywordPrefixedFreeTextFolds (drain r1 F2): a target is POSITIONAL
// only when its suffix is actually a key. `approach: do it simpler overall` is
// a sentence a requester typed that happens to begin with a keyword, and it
// folds like every other free-text target (OQ1's ratified permissive posture,
// and the rule target.go states for the indexed families). The refusal is
// reserved for a grammar-shaped target that names nothing — which is the whole
// point of validating: `approach:S-9` against a two-step plan is a re-plan
// round that would be spent on a misunderstanding.
//
// `confinement:` matters twice over: refusing free text there was also a
// silent behavior change against the landed baseline, which folded everything.
func TestGF8KeywordPrefixedFreeTextFolds(t *testing.T) {
	pair := gf8TargetPair()
	cases := []struct {
		target  string
		grammar bool
		ok      bool
		text    string
	}{
		// Free words behind a keyword: not this grammar, so the fold takes it.
		{"outcome: make it faster overall", false, false, ""},
		{"approach: do it simpler overall", false, false, ""},
		{"confinement: this is too loose", false, false, ""},
		{"risk: I am worried about the data", false, false, ""},
		{"S- something", false, false, ""},
		{"AC- the second one", false, false, ""},
		{"the colors are wrong", false, false, ""},
		{"assumption:the changelog is the source of truth", false, false, ""},
		// Grammar-shaped, naming nothing: refused.
		{"approach:S-9", true, false, ""},
		{"confinement:S-9", true, false, ""},
		{"out_of_scope:99", true, false, ""},
		{"S-9", true, false, ""},
		{"AC-9", true, false, ""},
		// Grammar-shaped and present: resolved.
		{"approach:S-1", true, true, "I edit the widget module in place."},
		{"confinement:S-2", true, true, ""},
		{"out_of_scope:1", true, true, "no deploys"},
		{"restatement", true, true, "Fix the widget."},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			ref, grammar, ok := resolveTarget(pair, tc.target)
			if grammar != tc.grammar || ok != tc.ok {
				t.Fatalf("resolveTarget(%q) = (grammar %v, ok %v), want (%v, %v)", tc.target, grammar, ok, tc.grammar, tc.ok)
			}
			if ref.Text != tc.text {
				t.Errorf("resolveTarget(%q).Text = %q, want %q", tc.target, ref.Text, tc.text)
			}
		})
	}
}

// TestGF8FreeTextContestsReachTheReviser (drain r1 F2, the door's own end): a
// keyword-prefixed sentence is carried to the reviser as the requester wrote
// it, and only a grammar-shaped target that names nothing refuses.
func TestGF8FreeTextContestsReachTheReviser(t *testing.T) {
	pair := gf8TargetPair()

	_, findings, err := replanContest(Answer{
		Action: ActionRePlan,
		Contests: []ContestRef{
			{Target: "approach: do it simpler overall"},
			{Target: "confinement: this is too loose", Note: "it can write anywhere"},
		},
	}, pair)
	if err != nil {
		t.Fatalf("a keyword-prefixed sentence must fold as the requester's own words: %v", err)
	}
	want := []string{
		"approach: do it simpler overall",
		"confinement: this is too loose: it can write anywhere",
	}
	if len(findings) != len(want) {
		t.Fatalf("findings = %v, want %v", findings, want)
	}
	for i, w := range want {
		if findings[i] != w {
			t.Errorf("finding %d = %q, want %q — the fold must not rewrite what a person typed", i, findings[i], w)
		}
	}

	for _, target := range []string{"approach:S-9", "confinement:S-9"} {
		_, _, err := replanContest(Answer{Action: ActionRePlan, Contests: []ContestRef{{Target: target}}}, pair)
		if !errors.Is(err, ErrBadAnswer) {
			t.Errorf("%q names no step on a two-step plan and must refuse; got %v", target, err)
		}
	}
}

// TestGF8OfferedTargetsNameTheWholeVocabulary (drain r1 F3): the refusal's
// "it offers" list is what a person reads to fix their contest, so it names
// every positional family the pair actually holds — the step-keyed ones
// included.
func TestGF8OfferedTargetsNameTheWholeVocabulary(t *testing.T) {
	got := offeredTargets(gf8TargetPair())
	for _, want := range []string{
		"restatement", "AC-1", "S-1..S-2", "outcome:1", "constraint:1",
		"out_of_scope:1", "risk:1", "approach:S-1..approach:S-2",
		"confinement:S-1..confinement:S-2",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the offered list omits %q — a refusal must name what it takes: %s", want, got)
		}
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

	// The guards, pinned against what stepContract ACTUALLY emits. Forbidding
	// words the emitter never writes ("Approach", "Decision") would be a limb
	// that cannot fail: a mutation removing the guards left it passing. These
	// are the emitted phrases (plansource.go), so a pre-A15 step contract that
	// gained any [A15] line fails here.
	bare := Step{ID: "S-1", Title: "Do the work", DoneWhen: "tests pass", Class: "C1"}
	barePlan := a15Plan(bare)
	bareGot := stepContract(&barePlan.Steps[0], &barePlan)
	for _, forbidden := range []string{"Approved approach:", "Approved decision:", "Why this step sits here:"} {
		if strings.Contains(bareGot, forbidden) {
			t.Errorf("a pre-A15 step's contract gained a %q line: %s", forbidden, bareGot)
		}
	}
	// A step carrying ONLY an approach must not mint the decision or ordering
	// lines either — each guard stands on its own member, not on the block.
	approachOnly := a15Plan(Step{
		ID: "S-1", Title: "Do the work", DoneWhen: "tests pass", Class: "C1",
		Approach: "I edit the one module involved.",
	})
	partial := stepContract(&approachOnly.Steps[0], &approachOnly)
	for _, forbidden := range []string{"Approved decision:", "Why this step sits here:"} {
		if strings.Contains(partial, forbidden) {
			t.Errorf("a step with only an approach gained a %q line: %s", forbidden, partial)
		}
	}
}

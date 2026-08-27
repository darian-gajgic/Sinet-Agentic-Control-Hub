package stage

// gf12prompt_test.go — P3-GF12 prompt-contract acceptance, committed RED with
// the grounding brief (P3/briefs/P3-GF12.md §9; Amendment-A carve-out,
// CONVENTIONS §5). The gf8prompt_test.go precedent: `pairSchema` is the ONE
// emission contract both Draft and Revise wrap, so what it fails to say, no
// emission was ever asked for.
//
// The GF9 evidence world proved the gap live: GF8 landed the A15 caps as
// boundary validation (approachMaxRunes = 1200 and kin, Plan.Validate) but the
// contract STATED only the duty, never the bounds — so a frontier planner that
// writes richly overshot the cap on eleven witnessed emissions (1246..1569
// runes against 1200) and every one crashed the drive. The prompt is the
// boundary's other half (Spec S06.6 [A15]; the boundary refuses, the contract
// forewarns); and the same contract said nothing about SETTLED facts, so the
// content-family planner re-emitted NEEDS-CLARIFICATION markers re-confirming
// answered facts, four witnessed rounds (S06.5: a resolved slot must never
// re-ask).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// TestGF12PairSchemaStatesTheA15Bounds (brief R1): the emission contract must
// state the A15 structural caps in plain words — the numbers themselves,
// spelled from the intake package's own exported constants so the prompt and
// the validator can never drift apart (one spelling, §65 D4). RED pre-GF12:
// pairSchema states the approach duty richly and no length bound at all.
func TestGF12PairSchemaStatesTheA15Bounds(t *testing.T) {
	for _, want := range []string{
		// The approach cap, in the reader's units ("characters", the same
		// plain word the refusal itself uses — never "runes" in a prompt).
		fmt.Sprintf("%d characters", intake.ApproachMaxRunes),
		// The per-decision-field cap (decision/alternative/why, and the
		// ordering rationale shares the same number).
		fmt.Sprintf("%d characters", intake.DecisionFieldMaxRunes),
		// The decisions-per-step cap.
		fmt.Sprintf("%d decisions", intake.MaxStepDecisions),
	} {
		if !strings.Contains(pairSchema, want) {
			t.Errorf("pairSchema does not state %q — the planner cannot honor a bound it was never told, and the boundary refusing what the contract never stated is the witnessed crash class (S06.6 [A15]; GF9 evidence)", want)
		}
	}
}

// TestGF12PairSchemaSaysSettledFactsStaySettled (brief R4): the contract must
// state that a fact the requester answered, supplied, or confirmed is SETTLED
// — a NEEDS-CLARIFICATION entry may never re-ask, re-confirm, or restate one;
// clarifications are only for new consequential ambiguities nothing in the
// input resolves. RED pre-GF12: the contract's only marker sentence
// ("unresolved consequential ambiguities become NEEDS-CLARIFICATION entries")
// says nothing about the settled class, and the witnessed planner used
// markers as confirmations of settled facts.
func TestGF12PairSchemaSaysSettledFactsStaySettled(t *testing.T) {
	for _, want := range []string{
		"SETTLED",
		"re-confirm",
		"new consequential ambiguities",
	} {
		if !strings.Contains(pairSchema, want) {
			t.Errorf("pairSchema does not say %q — the settled-facts rule is the prompt half of the confirm-loop fix (S06.5 resolution rules; GF9 evidence, asks 4-6 of t-4b8ed0297f821433)", want)
		}
	}
}

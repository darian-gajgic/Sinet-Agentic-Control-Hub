package stage

import (
	"strings"
	"testing"
)

// gf8prompt_test.go — P3-GF8 R13: the planner-prompt contract for the [A15]
// per-step approach (Spec S06.6 "Per-step approach [A15]"; S00.9 row A15,
// which requires the implementing packet to annotate its PLANNER-PROMPT and
// artifact-schema sites).
//
// `pairSchema` is the ONE emission contract both Draft and Revise wrap — each
// appends it to its own instruction string — so binding the requirement here
// binds both verbs at once. That matters: the approach is REQUIRED at the
// artifact boundary (Plan.Validate), so a member the prompt never asked for
// would make every real emission bounce, on the revise leg as surely as on the
// draft one.

// TestGF8PairSchemaCarriesTheApproachContract (brief R13): the shared schema
// asks for the [A15] members by their wire names and states the rule in words
// the planning model can act on.
func TestGF8PairSchemaCarriesTheApproachContract(t *testing.T) {
	for _, want := range []string{
		`"approach":string`,
		`"decisions":`,
		`"decision":string`,
		`"alternatives":`,
		`"why":string`,
		`"ordering_rationale":string`,
	} {
		if !strings.Contains(pairSchema, want) {
			t.Errorf("pairSchema does not name %s — the planner cannot emit a member it was never asked for (S06.6 [A15])", want)
		}
	}
	for _, want := range []string{
		"HOW it will be built",
		"alternatives",
		"ordering",
		"only its outcome is invalid",
	} {
		if !strings.Contains(pairSchema, want) {
			t.Errorf("pairSchema's rules do not say %q — the A15 duty is the PLANNING model's (S06.10 duty table)", want)
		}
	}
	// The S00.9 A15 annotation duty: the prompt site carries the marker, so a
	// later reader finds this text from the changelog row.
	if !strings.Contains(pairSchema, "[A15]") {
		t.Error("pairSchema carries no [A15] marker (S00.9 A15: the implementing packet annotates its planner-prompt site)")
	}
}

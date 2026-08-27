package intake

import (
	"fmt"
	"strconv"
	"strings"
)

// The field-target grammar (Spec S06.9: the structured Re-plan entry and the
// ADDED / MODIFIED / REMOVED delta vocabulary; P3-GF8 R5).
//
// ONE list, two readers (CONVENTIONS §43): `replanContest` validates a
// contested target against the open card's pair and expands it with the
// field's current text, and `diffPairs` mints the SAME keys for a
// post-approval delta. Declaring them once is what makes "the thing you
// contested" and "the thing that changed" the same name — a requester who
// contests `out_of_scope:1` before approval and reads `out_of_scope:1` on a
// delta card afterwards is looking at one field, not two vocabularies that
// happen to agree today.
//
// Two kinds of target live here:
//
//   - POSITIONAL targets name a field by where it sits in the served artifact
//     version — `restatement`, `outcome:<n>`, `constraint:<n>`,
//     `out_of_scope:<n>`, `risk:<n>`, `approach:S-<n>`, plus the landed
//     `AC-<n>`, `S-<n>` and `confinement:S-<n>`. Because the position is
//     checkable, a positional target that names nothing is REFUSED. The index
//     is sound as a key because a contest always addresses the OPEN card's
//     pair version, and a delta item carries the Old and New text with it.
//   - CONTENT targets name a field by its own text — `assumption:<text>`.
//     There is no position to check: the text IS the identity, so these keep
//     the permissive free-text fold every other requester-worded target has
//     (P3-GF8 OQ1; the landed §11 OQ-4 posture).
const (
	targetRestatement = "restatement"

	prefixOutcome     = "outcome:"
	prefixConstraint  = "constraint:"
	prefixOutOfScope  = "out_of_scope:"
	prefixRisk        = "risk:"
	prefixApproach    = "approach:"
	prefixConfinement = "confinement:"
	prefixAC          = "AC-"
	prefixStep        = "S-"
)

// Minting helpers — the delta side's half of the one list.
func targetOutcome(n int) string             { return prefixOutcome + strconv.Itoa(n) }
func targetConstraint(n int) string          { return prefixConstraint + strconv.Itoa(n) }
func targetOutOfScope(n int) string          { return prefixOutOfScope + strconv.Itoa(n) }
func targetRisk(n int) string                { return prefixRisk + strconv.Itoa(n) }
func targetApproach(stepID string) string    { return prefixApproach + stepID }
func targetConfinement(stepID string) string { return prefixConfinement + stepID }
func targetStep(n int) string                { return prefixStep + strconv.Itoa(n) }
func targetAC(n int) string                  { return prefixAC + strconv.Itoa(n) }

// targetRef is one resolved positional target.
type targetRef struct {
	// Text is the field's CURRENT text on the pair, carried into the revise
	// finding so field identity AND content reach the reviser (S06.9's
	// structured entry; operator record r5 §C rule 7). It is empty for the
	// landed structural targets — `AC-<n>`, `S-<n>`, `confinement:S-<n>` —
	// whose finding wording is pinned by a landed test and stays exactly as it
	// was: expanding them is sanctioned but optional, and a landed assertion
	// outranks an optional improvement.
	Text string
}

// resolveTarget resolves a contest/delta target against a pair. It answers
// three ways, and the caller needs all three:
//
//	grammar=false            → not a positional target (free text, or
//	                           `assumption:<text>`): the permissive fold
//	grammar=true, ok=false   → positional, but names nothing on this pair:
//	                           refuse, and say what the card does offer
//	grammar=true, ok=true    → resolved; ref.Text is the field's current words
func resolveTarget(pair *Pair, target string) (ref targetRef, grammar, ok bool) {
	if pair == nil {
		return targetRef{}, false, false
	}
	if target == targetRestatement {
		return targetRef{Text: pair.Spec.Restatement}, true, pair.Spec.Restatement != ""
	}
	switch {
	case strings.HasPrefix(target, prefixOutcome):
		return indexed(pair.Spec.Outcome, strings.TrimPrefix(target, prefixOutcome))
	case strings.HasPrefix(target, prefixConstraint):
		return indexed(pair.Spec.Constraints, strings.TrimPrefix(target, prefixConstraint))
	case strings.HasPrefix(target, prefixOutOfScope):
		return indexed(pair.Spec.OutOfScope, strings.TrimPrefix(target, prefixOutOfScope))
	case strings.HasPrefix(target, prefixRisk):
		return indexed(pair.Plan.Risks, strings.TrimPrefix(target, prefixRisk))
	case strings.HasPrefix(target, prefixApproach):
		suffix := strings.TrimPrefix(target, prefixApproach)
		if !stepKeyed(suffix) {
			return targetRef{}, false, false
		}
		step := pair.Plan.Step(suffix)
		if step == nil {
			return targetRef{}, true, false
		}
		return targetRef{Text: step.Approach}, true, true
	case strings.HasPrefix(target, prefixConfinement):
		// Landed structural target: validated against the plan, never expanded.
		suffix := strings.TrimPrefix(target, prefixConfinement)
		if !stepKeyed(suffix) {
			return targetRef{}, false, false
		}
		return targetRef{}, true, pair.Plan.Step(suffix) != nil
	case strings.HasPrefix(target, prefixAC):
		if _, err := strconv.Atoi(strings.TrimPrefix(target, prefixAC)); err != nil {
			return targetRef{}, false, false
		}
		return targetRef{}, true, pair.Spec.AC(target) != nil
	case strings.HasPrefix(target, prefixStep):
		if _, err := strconv.Atoi(strings.TrimPrefix(target, prefixStep)); err != nil {
			return targetRef{}, false, false
		}
		return targetRef{}, true, pair.Plan.Step(target) != nil
	}
	return targetRef{}, false, false
}

// stepKeyed reports whether a suffix is a step key ("S-3") rather than
// somebody's own words. It is the step-keyed families' half of the rule the
// indexed families get from indexed(): `approach: do it simpler overall` and
// `confinement: this is too loose` are free text a requester typed, and they
// fold with every other target the grammar does not claim (OQ1). Only a
// grammar-shaped key that names nothing — `approach:S-9` against a two-step
// plan — is a refusal.
func stepKeyed(suffix string) bool {
	if !strings.HasPrefix(suffix, prefixStep) {
		return false
	}
	_, err := strconv.Atoi(strings.TrimPrefix(suffix, prefixStep))
	return err == nil
}

// indexed resolves a 1-based index into a served list. A suffix that is not a
// number is not this grammar at all — it is somebody's own words that happen to
// start with a keyword, and the permissive fold takes it.
func indexed(list []string, suffix string) (targetRef, bool, bool) {
	n, err := strconv.Atoi(suffix)
	if err != nil {
		return targetRef{}, false, false
	}
	if n < 1 || n > len(list) {
		return targetRef{}, true, false
	}
	return targetRef{Text: list[n-1]}, true, true
}

// offeredTargets names the positional keys THIS pair actually holds, so a
// refused contest says what it takes rather than only what it rejected
// (CONVENTIONS §59 — the refusal a person reads is the platform's own list,
// derived from the artifact, never a hand-kept sentence).
func offeredTargets(pair *Pair) string {
	if pair == nil {
		return "nothing yet — no plan is drafted"
	}
	parts := []string{}
	if pair.Spec.Restatement != "" {
		parts = append(parts, targetRestatement)
	}
	steps := len(pair.Plan.Steps)
	// ONE table over the WHOLE positional vocabulary, including the two
	// step-keyed families. They sit in the table rather than in a tail of
	// special cases so a family cannot be answered by resolveTarget and then
	// left out of the refusal that is supposed to name it.
	for _, r := range []struct {
		prefix string
		n      int
	}{
		{prefixAC, len(pair.Spec.ACs)},
		{prefixStep, steps},
		{prefixOutcome, len(pair.Spec.Outcome)},
		{prefixConstraint, len(pair.Spec.Constraints)},
		{prefixOutOfScope, len(pair.Spec.OutOfScope)},
		{prefixRisk, len(pair.Plan.Risks)},
		{prefixApproach + prefixStep, steps},
		{prefixConfinement + prefixStep, steps},
	} {
		if s := countedKeys(r.prefix, r.n); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return "nothing yet — no plan is drafted"
	}
	return strings.Join(parts, ", ")
}

// countedKeys renders one family's available keys compactly: "risk:1" for a
// single entry, "AC-1..AC-4" for several, nothing for none.
func countedKeys(prefix string, n int) string {
	switch {
	case n <= 0:
		return ""
	case n == 1:
		return prefix + "1"
	default:
		return fmt.Sprintf("%s1..%s%d", prefix, prefix, n)
	}
}

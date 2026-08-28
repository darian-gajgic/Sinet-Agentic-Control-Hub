// gf13distinct_internal_test.go — P3-GF13 drain r1 F8: the verdict note names
// WHICH research question on WHICH step in plain words, and two rules on ONE
// step stay two visibly different facts.
//
// The Anchor is `step:<id>` and carries the cross-round suppression semantics,
// so review's dedupe is anchor-AND-body: distinctness has to live in the TEXT.
// Before P3-GF13 the rule id did that work; the id has left the sentence, so
// this test holds the replacement — including the corner the evaluation caught,
// where NEITHER a seed class nor a query exists to name a rule by.
package verify

import (
	"regexp"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// gf13DistinctJargonClass is this file's own copy of the citation classes —
// the red file's scan is over source literals, this one is over rendered text.
var gf13DistinctJargonClass = regexp.MustCompile(`\(S[0-9]+\.|\(Spec S|\(D[0-9]+|P47-|\([0-9]+\.[0-9]+\)`)

func TestGF13ResearchSubjectsStayDistinctPerRuleOnOneStep(t *testing.T) {
	for _, tc := range []struct {
		name  string
		nodes []intake.ResearchNode
	}{
		{"two seeded rules on one step", []intake.ResearchNode{
			{RuleID: "P47-1", StepID: "S-1"},
			{RuleID: "P47-8", StepID: "S-1"},
		}},
		{"two unseeded rules with their own queries", []intake.ResearchNode{
			{RuleID: "house-1", StepID: "S-1", Query: "the shop's opening hours"},
			{RuleID: "house-2", StepID: "S-1", Query: "the bank holiday calendar"},
		}},
		// The F8 corner: nothing names either rule, so the numbering is the
		// only thing keeping two facts from collapsing into one finding.
		{"two unseeded rules with no query at all", []intake.ResearchNode{
			{RuleID: "house-1", StepID: "S-1"},
			{RuleID: "house-2", StepID: "S-1"},
		}},
		{"a seeded rule beside an unnamable one", []intake.ResearchNode{
			{RuleID: "P47-1", StepID: "S-1"},
			{RuleID: "house-1", StepID: "S-1"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subjects := researchSubjects(tc.nodes)
			a := researchSubjectOf(subjects, tc.nodes[0])
			b := researchSubjectOf(subjects, tc.nodes[1])
			if a == "" || b == "" {
				t.Fatalf("a node was left unnamed: %q / %q", a, b)
			}
			if a == b {
				t.Errorf("two rules on one step render the SAME subject %q — the served texts become "+
					"byte-identical, dedupe folds them into one finding, and a fact the platform holds "+
					"stops reaching the requester (the P3-RW-18 D3-R2 lesson)", a)
			}
			for _, got := range []string{a, b} {
				if gf13DistinctJargonClass.MatchString(got) {
					t.Errorf("the subject %q quotes an internal id at the requester", got)
				}
			}
		})
	}
}

// A single unnamable lookup on a step is not numbered: "1 of the 1" would be
// noise, and honest absence needs no counter.
func TestGF13LoneUnnamableResearchSubjectIsNotNumbered(t *testing.T) {
	nodes := []intake.ResearchNode{{RuleID: "house-1", StepID: "S-1"}}
	got := researchSubjectOf(researchSubjects(nodes), nodes[0])
	if strings.ContainsAny(got, "0123456789") {
		t.Errorf("the only unnamable lookup on a step was numbered anyway: %q", got)
	}
	if got == "" {
		t.Error("an unnamable lookup must still be described, not left blank")
	}
}

// A seeded rule is named by its plain CLASS, never by its id.
func TestGF13SeededResearchSubjectIsThePlainClass(t *testing.T) {
	nodes := []intake.ResearchNode{{RuleID: "P47-1", StepID: "S-1"}}
	got := researchSubjectOf(researchSubjects(nodes), nodes[0])
	if want := strings.ToLower(intake.ResearchSubject("P47-1")); got != want {
		t.Errorf("subject = %q, want the seed table's plain class %q", got, want)
	}
	if strings.Contains(got, "P47") {
		t.Errorf("the rule id reached requester text: %q", got)
	}
}

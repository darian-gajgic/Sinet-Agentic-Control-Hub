// gf13jargon_red_internal_test.go — P3-GF13 RED tests: server-authored text on
// requester surfaces speaks plain words; spec citations live in code comments
// and ops logs, never on the wire to a requester (operator-ratified rule,
// r4 §F5/RA-5..RA-13 + r5 §C rule 4 + WALK-F1 W4/W6).
//
// These tests are written RED by the grounding stage (P3/briefs/P3-GF13.md) and
// turn green when the executor rewrites the copy. They never dictate the
// replacement wording — they assert the jargon CLASSES are gone and the
// register duties are met (the bar: internal/verify/bootstrap.go
// BootstrapPostureNote, which says everything it needs in plain words).
package intake

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"
)

// gf13JargonClass matches the wire-side jargon classes the walks enumerated:
// "(S07.3)"-style spec refs, "(Spec S13.6)", "(D5)"-style decision refs,
// P47 rule ids, and bare "(1.9)"/"(2.5)"/"(4.3)" feature-list refs.
var gf13JargonClass = regexp.MustCompile(`\(S[0-9]+\.|\(Spec S|\(D[0-9]+|P47-|\([0-9]+\.[0-9]+\)`)

// gf13StringLiterals parses ONE Go source file of this package and returns the
// concatenation of its string literals — comments never enter, so a spec
// citation in a comment (house style, expressly kept) can never trip the test.
func gf13StringLiterals(t *testing.T, file string) string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	var b strings.Builder
	ast.Inspect(f, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			b.WriteString(lit.Value)
			b.WriteString("\n")
		}
		return true
	})
	return b.String()
}

// TestGF13SizeDeltaIncomparableSpeaksPlainWords — the approval card's size
// note (Layer1.SizeNote, buildApprovalCard) is requester copy; today the
// unpriced branch ends "— surfaced, never silent (2.5)".
func TestGF13SizeDeltaIncomparableSpeaksPlainWords(t *testing.T) {
	_, detail := sizeDelta(Estimate{}, Estimate{}, 2.0)
	if detail == "" {
		t.Fatal("the incomparable branch must still explain itself — an empty detail is not the fix")
	}
	if m := gf13JargonClass.FindString(detail); m != "" {
		t.Errorf("the size note served to the requester carries the spec-ref %q; requester copy speaks plain words (the citation belongs in a comment): %q", m, detail)
	}
}

// TestGF13IntakeServedCopyDropsTheSpecRefLiterals — the four wire-bound
// literals WALK-F1 W4 quoted from intake's own served copy, asserted GONE from
// the files' string literals (comments are free to keep the citations):
//   - pipeline.go: the plan-estimate cost line "(API-equivalent, D5)" and the
//     research card's help "required by policy for data-bearing tasks (1.9)"
//     and the park reason "gates wait (S06.1)";
//   - answer.go: the resume reason "resumes in place (4.3)".
func TestGF13IntakeServedCopyDropsTheSpecRefLiterals(t *testing.T) {
	pipeline := gf13StringLiterals(t, "pipeline.go")
	answer := gf13StringLiterals(t, "answer.go")
	for _, tc := range []struct{ file, src, fragment string }{
		{"pipeline.go", pipeline, "(API-equivalent, D5)"},
		{"pipeline.go", pipeline, "data-bearing tasks (1.9)"},
		{"pipeline.go", pipeline, "gates wait (S06.1)"},
		{"answer.go", answer, "resumes in place (4.3)"},
	} {
		if strings.Contains(tc.src, tc.fragment) {
			t.Errorf("%s still serves the requester-surface literal %q — the copy speaks plain words on the wire; the spec citation moves to a code comment", tc.file, tc.fragment)
		}
	}
}

// TestGF13UnderstoodItemCarriesTheChosenLabel — the W6/RA-7 family: a settled
// interview row whose answer was an option pick must serve the option's plain
// LABEL (what the requester actually chose), while the machine value stays on
// the wire untouched — the value is also the machine key the edit/answer
// round-trip and the FE's edit seeding read, so the label arrives ALONGSIDE,
// never by corrupting the value.
func TestGF13UnderstoodItemCarriesTheChosenLabel(t *testing.T) {
	// Find the real seed slot that owns the "graceful" option, so the test
	// tracks the taxonomy rather than pinning a family.
	var (
		tax   *Taxonomy
		slot  string
		label string
	)
	for _, tx := range SeedTaxonomies() {
		for _, s := range tx.Slots {
			for _, o := range s.Options {
				if o.Value == "graceful" {
					tax, slot, label = tx, s.ID, o.Label
				}
			}
		}
	}
	if tax == nil {
		t.Skip("no seed slot carries a 'graceful' option any more — re-point this test at any option-valued slot")
	}
	st := &State{Resolutions: []SlotResolution{{SlotID: slot, How: ResolvedAnswered, Value: "graceful"}}}
	block := understoodBlock(st, tax)
	if block == nil || len(block.Items) != 1 {
		t.Fatalf("understoodBlock served %+v, want the one resolved item", block)
	}
	raw, err := json.Marshal(block)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "graceful") {
		t.Errorf("the machine value %q left the wire — it is the round-trip key (FE edit seeding, answer fold) and must stay: %s", "graceful", raw)
	}
	if !strings.Contains(string(raw), label) {
		t.Errorf("the understood item serves only the machine token %q; the requester picked %q and the served item must carry that label: %s", "graceful", label, raw)
	}
}

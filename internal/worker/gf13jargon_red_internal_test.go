// gf13jargon_red_internal_test.go — P3-GF13 RED test: the S08.8 selection's
// requester-facing sentences (PlainReason, the pin refusal, the subscription-
// gap advice) speak plain words on the wire (r4 RA-6 verbatim family; WALK-F1
// W4 "WHO DOES IT" quote). Written RED by grounding (P3/briefs/P3-GF13.md).
//
// The scan reads routing.go's STRING LITERALS via the AST, so the package's
// spec citations in comments — house style, expressly kept — can never trip
// it. It asserts exact known-served fragments GONE rather than a class over
// the whole file, because routing.go also holds internal error strings whose
// audience the executor adjudicates per the brief's sweep duty.
package worker

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

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

// TestGF13RoutingPlainReasonDropsTheSpecRefLiterals — every fragment below is
// today part of a sentence served on the WHO-DOES-IT surface, the lane-pin
// refusal, or the subscription-gap advice.
func TestGF13RoutingPlainReasonDropsTheSpecRefLiterals(t *testing.T) {
	src := gf13StringLiterals(t, "routing.go")
	for _, fragment := range []string{
		"generalist-with-injected-knowledge",         // the r4 RA-6 headline token (worker jargon on a requester card)
		"the default for one-offs, S08.8",            // its parenthetical
		"(S08.7)",                                    // the degraded-marked variant's citation
		"flat-rate coverage bound (D5)",              // the seat sentence's tail
		"search-capable lane (S08.8 step 4)",         // the research-node rider
		"(S08.8 step 3)",                             // the pin refusal citations (both sites)
		"Subscription gap (2.7)",                     // the gap advice's lead
		"(S08.8 visible-and-overridable [S00.9 A13]", // the honored-pin sentence
	} {
		if strings.Contains(src, fragment) {
			t.Errorf("routing.go still serves the requester-surface fragment %q — the sentence speaks plain words on the wire; the spec citation moves to a code comment", fragment)
		}
	}
}

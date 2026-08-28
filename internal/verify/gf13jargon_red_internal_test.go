// gf13jargon_red_internal_test.go — P3-GF13 RED test: the verification
// family's served copy — the research-not-run escalation summary and the
// verdict notes a requester reads on the review surface — speaks plain words
// (WALK-F1 W4 quoted the verdict note verbatim). Written RED by grounding
// (P3/briefs/P3-GF13.md).
//
// AST string-literal scan: comments keep their citations, only wire-bound
// literals are asserted. Exact fragments, not a class over the whole file —
// verify also holds internal sentinels whose audience the executor
// adjudicates per the brief's sweep duty.
package verify

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

// TestGF13VerifyServedCopyDropsTheSpecRefLiterals — the walk's verbatim
// verdict note ("did-research-actually-run undecidable for S-1 [P47-1]:
// per-step usage counters not wired (S10 seam; B2-4)") and the escalation
// summary's "(1.9)". The replacement must still name WHICH research question
// on WHICH step is undecided — two rules on one step were two facts (the
// P3-RW-18 D3-R2 lesson) — but in the rule's plain class words, not its id.
func TestGF13VerifyServedCopyDropsTheSpecRefLiterals(t *testing.T) {
	pipeline := gf13StringLiterals(t, "pipeline.go")
	v1 := gf13StringLiterals(t, "v1.go")
	for _, tc := range []struct{ file, src, fragment string }{
		{"pipeline.go", pipeline, "did-research-actually-run"},
		{"pipeline.go", pipeline, "research tool (1.9)"},
		{"pipeline.go", pipeline, "(re-run seam not wired; B2-4)"},
		{"v1.go", v1, "(S10 seam; B2-4)"},
	} {
		if strings.Contains(tc.src, tc.fragment) {
			t.Errorf("%s still serves the requester-surface fragment %q — the note speaks plain words on the wire; the seam citation moves to a code comment", tc.file, tc.fragment)
		}
	}
}

// gf13jargon_red_internal_test.go — P3-GF13 RED test: the always-on danger
// zone RULE sentences are served verbatim on the onboarding approval card
// (stage/onboard.go renders "Danger zone: <path> (<action>) — <rule>"), which
// makes them requester copy; today they end "(D2)" and "(Spec S13.6)"
// (WALK-F1 W4). Written RED by grounding (P3/briefs/P3-GF13.md).
//
// The PATH globs (e.g. "**/*credential*") are machine values and drift-hash
// inputs — the brief forbids changing them; only the Rule prose is in scope.
package project

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

func TestGF13DangerZoneRulesSpeakPlainWords(t *testing.T) {
	src := gf13StringLiterals(t, "scan.go")
	for _, fragment := range []string{
		"in a workspace (D2)",
		"accepts only (Spec S13.6)",
	} {
		if strings.Contains(src, fragment) {
			t.Errorf("scan.go still captures the danger-zone rule fragment %q — the sentence the onboarding card serves speaks plain words; the citation moves to a code comment", fragment)
		}
	}
}

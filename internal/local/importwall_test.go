package local_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// importwall_test.go — the R24 import walls, AST-pinned (the §15/§17/§22–§24
// precedent). internal/local imports storage/eventlog/gates/settings + stdlib
// ONLY; it NEVER imports adapters/stage/intake/worker/verify/memory/broker/
// scheduler/review/project/metering (the duty-seam adapters live in
// internal/stage, which imports those — internal/local imports neither intake
// nor worker). And internal/worker NEVER imports internal/local (the
// TieBreaker arrives via the existing interface seam).

const modPrefix = "github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/"

func TestLocalImportWall(t *testing.T) {
	allowed := map[string]bool{}
	for _, p := range []string{"storage", "eventlog", "gates", "settings"} {
		allowed[modPrefix+p] = true
	}
	for _, path := range internalImports(t, ".") {
		if !allowed[path] {
			t.Errorf("internal/local imports %q — outside the R24 allowed set {storage,eventlog,gates,settings}", path)
		}
	}
}

func TestWorkerNeverImportsLocal(t *testing.T) {
	for _, path := range internalImports(t, filepath.Join("..", "worker")) {
		if path == modPrefix+"local" {
			t.Error("internal/worker imports internal/local — forbidden (R24: the TieBreaker arrives via the interface seam)")
		}
	}
}

// internalImports returns the module-internal imports of the non-test .go
// files in dir.
func internalImports(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var out []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range parsed.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(path, modPrefix) {
				out = append(out, path)
			}
		}
	}
	return out
}

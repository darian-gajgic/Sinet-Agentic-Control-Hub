package portpool

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestImportGraphStdlibOnly pins the portpool core's isolation (F14; CONVENTIONS
// §25 "imports stdlib only, plus the settings interface at most"): no internal
// package is imported except (optionally) the settings interface — the allocator
// is a leaf so previews and the daemon both rest on it. AST-inspected on every
// run, never assumed.
func TestImportGraphStdlibOnly(t *testing.T) {
	allowedInternal := map[string]bool{
		"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings": true,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
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
			if strings.Contains(path, "Sinet-Agentic-Control-Hub/internal/") && !allowedInternal[path] {
				t.Errorf("%s imports internal package %q — the portpool core is stdlib(+settings) only (F14/§25)", name, path)
			}
		}
	}
}

package accept

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// STANDING conformance (brief R29 / F8: test-pinned where a new wall is
// introduced — broker/backup/project all carry one). The accept composition
// package imports EXACTLY internal/{project,gates,broker,review,run} + stdlib —
// it sits above the storage-only packages (R27) and never reaches sideways into
// engine-facing packages (memory/adapters/stage/intake) or the storage/eventlog
// layer directly.
func TestImportWallHolds(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	allowedInternal := map[string]bool{
		"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project": true,
		"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates":   true,
		"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker":  true,
		"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review":  true,
		"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run":     true,
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
				t.Errorf("%s imports internal package %q — accept composes project+gates+broker+review+run only (R29/F8)", name, path)
			}
		}
	}
}

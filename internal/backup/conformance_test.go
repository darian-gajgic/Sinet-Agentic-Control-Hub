package backup

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// STANDING conformance (brief R29; the §15/§17/§22/§23 import-graph precedent):
// the backup package's isolation is proven by inspection on every run. It
// imports storage + eventlog + broker (client) + the adopted age/zstd modules +
// stdlib, and NEVER internal/{memory,adapters,stage,intake} — so no
// engine-facing capability wall is crossed and no engine session can reach the
// snapshot lane.
func TestImportWallsHold(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"internal/memory", "internal/adapters", "internal/stage",
		"internal/intake", "internal/sandbox", "internal/scheduler",
		"internal/verify", "internal/worker", "internal/ledger",
	}
	allowedInternal := map[string]bool{
		"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage":  true,
		"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog": true,
		"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker":   true,
		// settings is a config composition dependency of the one-shot mode
		// entry (mode.go) — a low-level config package, never engine-facing.
		"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings": true,
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
			for _, f := range forbidden {
				if strings.Contains(path, f) {
					t.Errorf("%s imports %q — forbidden (R29: no engine-facing package)", name, path)
				}
			}
			if strings.Contains(path, "Sinet-Agentic-Control-Hub/internal/") && !allowedInternal[path] {
				t.Errorf("%s imports internal package %q — only storage+eventlog+broker allowed (R29)", name, path)
			}
		}
	}
}

package watchdog_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// importwall_test.go — the HARD INVARIANT proofs (brief R16, rubric 11 + §10
// walls). An AST proof that internal/watchdog invokes NO kill/terminate/
// tombstone/gate primitive (the watchdog contains-and-parks, always), and the
// reverse wall that internal/local never imports internal/watchdog (no cycle).

// forbiddenNames are the kill/terminate/tombstone/gate primitives the watchdog
// must NEVER reference (Spec S14.4 / G1 D1.3): the recovery-side terminal
// states, the OS kill primitives, and any tombstone/terminate identifier. Park
// (StateParked) is the ONLY transition target the suite uses on a watched run;
// the synthetic dead-man canary self-terminates via StateCompleted/StateFinalized
// (its OWN synthetic run, not a watched run — not a kill primitive).
var forbiddenNames = []string{
	"StateTombstoned", "StateCrashed", "StateDiedAtGate",
	"Tombstone", "Terminate", "SIGKILL", "SIGTERM",
}

// forbiddenImports are packages a killer would need but the watchdog must not.
var forbiddenImports = []string{"os/exec", "syscall"}

func TestNoAutoKillPrimitives(t *testing.T) {
	fset := token.NewFileSet()
	files := sourceFiles(t, ".")
	if len(files) == 0 {
		t.Fatal("no watchdog source files found")
	}
	for _, path := range files {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		// No forbidden imports.
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbiddenImports {
				if p == bad {
					t.Errorf("%s imports %q — a kill/terminate primitive path is forbidden in the watchdog (D1.3)", filepath.Base(path), p)
				}
			}
		}
		// No forbidden identifier anywhere (Ident + SelectorExpr .Sel).
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				checkName(t, path, x.Name)
			case *ast.SelectorExpr:
				checkName(t, path, x.Sel.Name)
			}
			return true
		})
	}
}

func checkName(t *testing.T, path, name string) {
	t.Helper()
	for _, bad := range forbiddenNames {
		if name == bad {
			t.Errorf("%s references %q — the watchdog NEVER kills/terminates/tombstones a run; it contains-and-parks (S14.4/D1.3)", filepath.Base(path), name)
		}
		// also catch a "Kill" method/call by suffix (Process.Kill, etc.)
		if bad == "Terminate" && name == "Kill" {
			t.Errorf("%s references Kill — forbidden (D1.3)", filepath.Base(path))
		}
	}
	if name == "Kill" {
		t.Errorf("%s references Kill — the watchdog NEVER kills a run (S14.4/D1.3)", filepath.Base(path))
	}
}

// TestLocalDoesNotImportWatchdog is the reverse import wall (brief §10): the
// $0 Tier-1 seat (internal/local) must never import internal/watchdog, so no
// cycle is possible and internal/local stays a read-only consumer.
func TestLocalDoesNotImportWatchdog(t *testing.T) {
	fset := token.NewFileSet()
	const watchdogPath = "github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchdog"
	for _, path := range sourceFiles(t, "../local") {
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range f.Imports {
			if strings.Trim(imp.Path.Value, `"`) == watchdogPath {
				t.Errorf("%s imports internal/watchdog — the seam rule bars it (no cycle; local stays read-only)", filepath.Base(path))
			}
		}
	}
}

// sourceFiles returns the non-test .go files under dir.
func sourceFiles(t *testing.T, dir string) []string {
	t.Helper()
	all, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	var out []string
	for _, p := range all {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		out = append(out, p)
	}
	return out
}

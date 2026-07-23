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
// walls; drain D10). An AST proof that internal/watchdog invokes NO kill/
// terminate/tombstone/gate primitive (the watchdog contains-and-parks, always),
// the reverse wall that internal/local never imports internal/watchdog (no
// cycle), and a FORWARD import allowlist that pins the package's exact internal
// import set so adding scheduler/recovery/etc. must be a deliberate wall update.

// forbiddenNames are the kill/terminate/tombstone/gate IDENTIFIERS the watchdog
// must NEVER reference (Spec S14.4 / G1 D1.3): the recovery-side terminal
// states, the OS kill primitives, and any tombstone/terminate identifier. Park
// (StateParked) is the ONLY transition target the suite uses on a watched run.
var forbiddenNames = []string{
	"StateTombstoned", "StateCrashed", "StateDiedAtGate",
	"Tombstone", "Terminate", "SIGKILL", "SIGTERM",
}

// deadmanOnlyStates are the terminal transition targets the SYNTHETIC dead-man
// canary uses to self-terminate its OWN run (completed/finalized). They are
// sanctioned ONLY in deadman.go (drain D10b): any OTHER watchdog file naming
// them would be transitioning a WATCHED run to a terminal — a kill-adjacent act.
var deadmanOnlyStates = []string{"StateCompleted", "StateFinalized"}

// forbiddenStateStrings are the recovery-owned state VALUES as string literals:
// scanned so run.State("tombstoned") cannot construct a forbidden state past the
// ident/selector check (drain D10a).
var forbiddenStateStrings = []string{"tombstoned", "crashed", "died_at_gate"}

// forbiddenKillStrings are kill-verb string literals a killer might use (a raw
// syscall/exec verb) — matched case-insensitively (exact for the ambiguous
// short verbs, substring for the unambiguous signal names).
var (
	forbiddenKillExact     = []string{"kill", "terminate", "tombstone"}
	forbiddenKillSubstring = []string{"sigkill", "sigterm"}
)

// forbiddenImports are packages a killer would need but the watchdog must not.
var forbiddenImports = []string{"os/exec", "syscall"}

const modPrefix = "github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/"

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
		base := filepath.Base(path)
		// No forbidden imports.
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbiddenImports {
				if p == bad {
					t.Errorf("%s imports %q — a kill/terminate primitive path is forbidden in the watchdog (D1.3)", base, p)
				}
			}
		}
		// No forbidden identifier (Ident + SelectorExpr .Sel), no forbidden
		// state-value / kill-verb string literal (D10a), and no StateCompleted/
		// StateFinalized ident outside deadman.go (D10b).
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				checkName(t, base, x.Name)
			case *ast.SelectorExpr:
				checkName(t, base, x.Sel.Name)
			case *ast.BasicLit:
				if x.Kind == token.STRING {
					checkStringLit(t, base, x.Value)
				}
			}
			return true
		})
	}
}

func checkName(t *testing.T, base, name string) {
	t.Helper()
	for _, bad := range forbiddenNames {
		if name == bad {
			t.Errorf("%s references %q — the watchdog NEVER kills/terminates/tombstones a run; it contains-and-parks (S14.4/D1.3)", base, name)
		}
		// also catch a "Kill" method/call by suffix (Process.Kill, etc.)
		if bad == "Terminate" && name == "Kill" {
			t.Errorf("%s references Kill — forbidden (D1.3)", base)
		}
	}
	if name == "Kill" {
		t.Errorf("%s references Kill — the watchdog NEVER kills a run (S14.4/D1.3)", base)
	}
	if base != "deadman.go" {
		for _, bad := range deadmanOnlyStates {
			if name == bad {
				t.Errorf("%s references %q — completed/finalized are terminal transitions sanctioned ONLY in deadman.go's synthetic-canary self-termination (D10b); a watched run must never be moved there", base, name)
			}
		}
	}
}

// checkStringLit scans a string literal for a forbidden state VALUE or kill verb
// (D10a) so a construction like run.State("tombstoned") cannot bypass the ident
// wall. It matches the trimmed literal, so struct tags / detail prose that merely
// mention a word (json:"completed", "canary complete") are unaffected.
func checkStringLit(t *testing.T, base, quoted string) {
	t.Helper()
	s := strings.Trim(quoted, "`\"")
	for _, bad := range forbiddenStateStrings {
		if s == bad {
			t.Errorf("%s contains the string %q — a forbidden state must not be constructed as a string literal (run.State(%q) would bypass the ident wall, D10a/D1.3)", base, s, s)
		}
	}
	ls := strings.ToLower(s)
	for _, bad := range forbiddenKillExact {
		if ls == bad {
			t.Errorf("%s contains the kill-verb string %q — forbidden (D10a/D1.3)", base, s)
		}
	}
	for _, bad := range forbiddenKillSubstring {
		if strings.Contains(ls, bad) {
			t.Errorf("%s contains the kill-verb string %q — forbidden (D10a/D1.3)", base, s)
		}
	}
}

// TestWatchdogImportAllowlist is the FORWARD import wall (drain D10c; the
// internal/local importwall is the in-repo precedent): the watchdog's internal
// imports are pinned to their exact current set, so adding internal/scheduler,
// internal/recovery, or anything new trips this test — a deliberate wall update,
// never a silent new edge.
func TestWatchdogImportAllowlist(t *testing.T) {
	allowed := map[string]bool{}
	for _, p := range []string{"eventlog", "local", "metering", "run", "storage"} {
		allowed[modPrefix+p] = true
	}
	fset := token.NewFileSet()
	for _, path := range sourceFiles(t, ".") {
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(p, modPrefix) && !allowed[p] {
				t.Errorf("%s imports %q — outside the watchdog's pinned internal set {eventlog,local,metering,run,storage}; a new edge (scheduler/recovery/…) must update this allowlist deliberately (D10c)", filepath.Base(path), p)
			}
		}
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

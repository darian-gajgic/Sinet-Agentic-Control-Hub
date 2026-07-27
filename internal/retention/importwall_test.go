package retention_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// importwall_test.go — the OQ4 import wall (the §31/§32/§33/§34/§35 precedent),
// matched exact-or-prefix so a SUBPACKAGE of a banned package is caught too,
// with a FORWARD allowlist so a new edge is deliberate, and a non-tautology
// probe: a wall that cannot fail guarantees nothing.

const modulePath = "github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/"

// allowed is the sanctioned forward allowlist for internal/retention: storage,
// eventlog and settings + stdlib. internal/settings is PERMITTED but unused —
// ⚙ values ride the package's own narrow Settings interface by dotted key, the
// house pattern (§2, and the eventlog/storage/watchlist precedent) — so the
// actual import set is a strict subset of what the disposition allows.
var allowed = map[string]string{
	"internal/storage":  "the DB seam (migration 0015's tables are this package's)",
	"internal/eventlog": "the S14.2 family registry the keep-forever set derives from, and the append surface",
	"internal/settings": "sanctioned by the OQ4 disposition; NOT imported today — ⚙ rides the narrow Settings interface",
}

// forbidden names representative packages internal/retention must never import,
// with the reason each edge is barred. The forward allowlist above is the real
// gate; this list makes the most tempting edges fail with an explanation.
var forbidden = map[string]string{
	"internal/run":       "the run-end summary rides run.Store's terminal hook, a func seam composed at the shell root — a leaf must not depend on the FSM package",
	"internal/local":     "the narrative pass rides the Narrator func seam; an absent stack is classified by StackAbsentMarker, pinned to the sentinel in internal/shell (§28/§33/§34)",
	"internal/api":       "the S14.10 surfaces DERIVE; internal/api reads these tables without an import in either direction (§31)",
	"internal/backup":    "the 11.3 exporter boundary is a VIEW, not a call — internal/storage reads it and this package only seeds the allowlist it joins",
	"internal/watchdog":  "detection is S14.4's; this package retains and indexes, it never judges",
	"internal/benchmark": "the benchmark records' keep-forever class is derived from the FAMILY registry, not from the producer package (which pre-marks it; the mapping is the authority)",
	"internal/verify":    "verdict texts are read from run_events by their canonical type string, never through the judging package",
	"internal/stage":     "no pipeline stage, no engine, no spawn",
	"internal/scheduler": "the shell owns WHEN; this package owns the LOGIC",
	"internal/metering":  "receipts are read from the receipts table, not through the meter",
}

func forbiddenImport(path string) (string, bool) {
	if !strings.HasPrefix(path, modulePath) {
		return "", false
	}
	rel := strings.TrimPrefix(path, modulePath)
	for banned, why := range forbidden {
		if rel == banned || strings.HasPrefix(rel, banned+"/") {
			return why, true
		}
	}
	return "", false
}

func inForwardAllowlist(path string) bool {
	if !strings.HasPrefix(path, modulePath) {
		return true // stdlib and third-party are governed by components.lock, not here
	}
	rel := strings.TrimPrefix(path, modulePath)
	for ok := range allowed {
		if rel == ok || strings.HasPrefix(rel, ok+"/") {
			return true
		}
	}
	return false
}

func packageImports(t *testing.T) map[string][]string {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			for _, imp := range file.Imports {
				out[filepath.Base(name)] = append(out[filepath.Base(name)], strings.Trim(imp.Path.Value, `"`))
			}
		}
	}
	return out
}

func TestImportWall(t *testing.T) {
	byFile := packageImports(t)
	if len(byFile) == 0 {
		t.Fatal("the import wall scanned no files — it would pass vacuously")
	}
	for file, imports := range byFile {
		for _, path := range imports {
			if why, bad := forbiddenImport(path); bad {
				t.Errorf("%s imports %s — forbidden: %s", file, path, why)
			}
			if !inForwardAllowlist(path) {
				t.Errorf("%s imports %s, which is outside the sanctioned forward allowlist (storage/eventlog/settings + stdlib) — adding an edge is a deliberate wall update, never a silent one", file, path)
			}
		}
	}
}

// TestImportWallCatchesSubpackages is the non-tautology probe: the wall must
// FAIL on a forbidden import including a SUBPACKAGE of a banned package, must
// not reject an allowed edge, and must not reject a package whose name merely
// starts with a banned one.
func TestImportWallCatchesSubpackages(t *testing.T) {
	for _, path := range []string{
		modulePath + "internal/run",
		modulePath + "internal/local",
		modulePath + "internal/api",
		modulePath + "internal/adapters/claudecli",
		modulePath + "internal/benchmark",
		modulePath + "internal/watchdog",
	} {
		if _, bad := forbiddenImport(path); !bad && inForwardAllowlist(path) {
			t.Errorf("the wall admits %s — it would pass vacuously", path)
		}
	}
	for _, path := range []string{
		modulePath + "internal/storage",
		modulePath + "internal/eventlog",
		"context", "database/sql", "time",
	} {
		if _, bad := forbiddenImport(path); bad {
			t.Errorf("the wall wrongly rejects the allowed edge %s", path)
		}
		if !inForwardAllowlist(path) {
			t.Errorf("the forward allowlist wrongly rejects %s", path)
		}
	}
	// A package whose name merely starts with a banned one is not banned.
	if _, bad := forbiddenImport(modulePath + "internal/runtimefake"); bad {
		t.Error("the wall matched a package that merely shares a prefix with internal/run")
	}
}

// TestSettingsRideTheNarrowInterface: the ⚙ seam is this package's own narrow
// interface, so internal/settings is not actually imported (the house pattern,
// §2) even though the OQ4 disposition permits it.
func TestSettingsRideTheNarrowInterface(t *testing.T) {
	for file, imports := range packageImports(t) {
		for _, path := range imports {
			if path == modulePath+"internal/settings" {
				t.Errorf("%s imports internal/settings — ⚙ values ride retention.Settings by dotted key", file)
			}
		}
	}
}

// TestPackageOwnsNoTicker: the shell owns WHEN (§31/§34/§35). The package owns
// the logic and holds no clock of its own.
func TestPackageOwnsNoTicker(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			files++
			ast.Inspect(file, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if sel.Sel.Name == "NewTicker" || sel.Sel.Name == "NewTimer" || sel.Sel.Name == "Tick" {
					t.Errorf("%s calls time.%s — the SHELL owns WHEN; this package exposes idempotent verbs", filepath.Base(name), sel.Sel.Name)
				}
				return true
			})
		}
	}
	if files == 0 {
		t.Fatal("the ticker scan read no files — it would pass vacuously")
	}
}

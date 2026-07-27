package benchmark_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// importwall_test.go is the package's structural boundary: the import wall (the
// §31/§32/§33/§34 precedent, matched exact-or-prefix so a SUBPACKAGE of a banned
// package is caught too, with a non-tautology probe), the FORWARD allowlist so a
// new edge is a deliberate wall update rather than a silent one, and the no-kill
// wall — because BENCH-REG §12 says nothing is auto-killed and running work is
// untouched, and a benchmark that can park a run is not an observer.

const modulePath = "github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/"

const pkgDir = "internal/benchmark"

// forbidden names the packages internal/benchmark must never import, with the
// reason each edge is barred. The wall's REASON is the §33/§34 one: keep the
// evals→verify→intake→adapters graph out of an observability leaf.
var forbidden = map[string]string{
	"internal/evals":     "limb (d) reads the S14.8 floor surfaces through a func seam composed at the shell root, so evals (which drags verify→intake→adapters) stays out of this leaf",
	"internal/verify":    "the domain vocabulary is translated by one pinned map here; this package records and evaluates, it never judges",
	"internal/intake":    "the §2 frozen statement rides a func seam composed at the shell root; the trivial-band tier is read from the log",
	"internal/review":    "the sampling hook DERIVES from deliverable.accepted rows; internal/review is not edited and not imported",
	"internal/accept":    "same as review — the accept is observed, never called",
	"internal/api":       "cards DERIVE from the log and the 0014 tables; internal/api reads them without an import in either direction",
	"internal/settings":  "the one ⚙ key rides this package's own narrow Settings interface, by dotted key",
	"internal/worker":    "the routing/worker-template domain fallback is a SQL read, not an import edge",
	"internal/stage":     "the benchmark is not a pipeline stage",
	"internal/adapters":  "the direct arm is dispatched through the ordinary run+queue surfaces; engine specifics are the adapters'",
	"internal/watchdog":  "the alarm is the benchmark family's own surface and never poaches the watchdog's types",
	"internal/watchlist": "the alarm is the benchmark family's own surface and never poaches drift.finding or canary.result",
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

func TestImportWall(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filepath.Join(repoRoot, pkgDir), func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			files++
			for _, imp := range file.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if why, bad := forbiddenImport(path); bad {
					t.Errorf("%s imports %s — forbidden: %s", filepath.Base(name), path, why)
				}
			}
		}
	}
	if files == 0 {
		t.Fatal("the import wall scanned no files — it would pass vacuously")
	}
}

// TestImportWallCatchesSubpackages is the non-tautology probe: the wall must
// FAIL when fed a forbidden import, including a subpackage of a banned package,
// and must NOT reject the allowed edges or a package whose name merely starts
// with a banned one.
func TestImportWallCatchesSubpackages(t *testing.T) {
	for _, path := range []string{
		modulePath + "internal/evals",
		modulePath + "internal/verify",
		modulePath + "internal/intake",
		modulePath + "internal/review",
		modulePath + "internal/accept",
		modulePath + "internal/api",
		modulePath + "internal/settings",
		modulePath + "internal/adapters/claudecli",
		modulePath + "internal/worker/automation",
	} {
		if _, bad := forbiddenImport(path); !bad {
			t.Errorf("the wall let %s through", path)
		}
	}
	for _, path := range []string{
		modulePath + "internal/storage",
		modulePath + "internal/eventlog",
		// run + scheduler: the direct arm rides the ORDINARY run-creation and
		// admission surfaces, and neither imports evals/verify/intake, so the
		// wall's reason still holds.
		modulePath + "internal/run",
		modulePath + "internal/scheduler",
		// metering: the §14 consumption readings, read-only.
		modulePath + "internal/metering",
		modulePath + "internal/apiary", // a name that merely starts with "api"
		modulePath + "internal/reviewer",
		"math/big",
		"strings",
	} {
		if why, bad := forbiddenImport(path); bad {
			t.Errorf("the wall wrongly rejected %s: %s", path, why)
		}
	}
}

// TestForwardImportAllowlist pins the package's internal imports to their exact
// current set (the §31 D10c / internal/local precedent), so adding a new edge is
// a deliberate wall update and never a silent one.
func TestForwardImportAllowlist(t *testing.T) {
	allowed := map[string]bool{}
	for _, p := range []string{"eventlog", "metering", "run", "scheduler", "storage"} {
		allowed[modulePath+"internal/"+p] = true
	}
	fset := token.NewFileSet()
	scanned := 0
	for _, path := range sourceFiles(t) {
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		scanned++
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(p, modulePath) && !allowed[p] {
				t.Errorf("%s imports %q — outside the pinned internal set {eventlog,metering,run,scheduler,storage}; a new edge must update this allowlist deliberately",
					filepath.Base(path), p)
			}
		}
	}
	if scanned == 0 {
		t.Fatal("the forward allowlist scanned no files — it would pass vacuously")
	}
}

// TestApiDoesNotImportBenchmark is the reverse wall: cards DERIVE from the log
// and the 0014 tables, so internal/api reads them without an import edge in
// either direction (the §31/§32/§34 pattern).
func TestApiDoesNotImportBenchmark(t *testing.T) {
	const benchPath = modulePath + "internal/benchmark"
	fset := token.NewFileSet()
	dir := filepath.Join(repoRoot, "internal/api")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for _, imp := range f.Imports {
			if strings.Trim(imp.Path.Value, `"`) == benchPath {
				t.Errorf("internal/api/%s imports internal/benchmark — cards derive from the log, they never import the producer", e.Name())
			}
		}
	}
	if scanned == 0 {
		t.Fatal("the reverse wall scanned no files — it would pass vacuously")
	}
}

// ── the no-kill wall ─────────────────────────────────────────────────────────

// forbiddenKillIdents are the identifiers a killer would need. BENCH-REG §12 is
// explicit: "Nothing is auto-killed; running work is untouched (D1.3)." The
// alarm is a card and a visible freeze — breadth is an operator act, so the
// freeze binds by visibility and by gate limb (c), never by terminating work.
// The gate is the same: S14.7 says this section evaluates and displays, it sets
// nothing.
var forbiddenKillIdents = []string{
	"Kill", "Terminate", "Tombstone", "Cancel", "Park",
	"StateTombstoned", "StateCrashed", "StateDiedAtGate", "StateParked",
	"Transition", "TransitionTx", "StopAdmission", "ParkInFlightRuns",
}

// forbiddenStateStrings are the state VALUES, so a construction like
// run.State("tombstoned") cannot slip past the identifier check (the §31 D10a
// precedent).
var forbiddenStateStrings = []string{"tombstoned", "crashed", "died-at-gate", "parked", "draining"}

// forbiddenKillImports are the paths a process-killer would need.
var forbiddenKillImports = []string{"os/exec", "syscall", "os/signal"}

func TestBenchmarkNeverKillsOrParksARun(t *testing.T) {
	fset := token.NewFileSet()
	files := sourceFiles(t)
	if len(files) == 0 {
		t.Fatal("no benchmark source files found — the no-kill wall would pass vacuously")
	}
	for _, path := range files {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		base := filepath.Base(path)
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbiddenKillImports {
				if p == bad {
					t.Errorf("%s imports %q — a process-kill primitive is forbidden: nothing is auto-killed and running work is untouched (BENCH-REG §12; D1.3)", base, p)
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				checkKillName(t, base, x.Name)
			case *ast.SelectorExpr:
				checkKillName(t, base, x.Sel.Name)
			case *ast.BasicLit:
				if x.Kind == token.STRING {
					checkKillString(t, base, x.Value)
				}
			}
			return true
		})
	}
}

func checkKillName(t *testing.T, base, name string) {
	t.Helper()
	for _, bad := range forbiddenKillIdents {
		if name == bad {
			t.Errorf("%s references %q — the benchmark practice NEVER kills, parks or transitions a run; the alarm is a card plus a visible freeze (BENCH-REG §12; S14.7 sets nothing)", base, name)
		}
	}
}

func checkKillString(t *testing.T, base, quoted string) {
	t.Helper()
	s := strings.Trim(quoted, "`\"")
	for _, bad := range forbiddenStateStrings {
		if s == bad {
			t.Errorf("%s contains the run-state literal %q — run.State(%q) would bypass the identifier wall (D10a/D1.3)", base, s, s)
		}
	}
}

// TestNoKillWallIsNonTautological is the probe: the wall's own matchers must
// reject a planted violation, or the assertions above guarantee nothing.
func TestNoKillWallIsNonTautological(t *testing.T) {
	planted := `package benchmark

import "os/exec"

func boom() {
	_ = exec.Command
	s := "tombstoned"
	_ = s
	kill := Kill
	_ = kill
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "planted.go")
	if err := os.WriteFile(path, []byte(planted), 0o600); err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var idents, strs, imports int
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		for _, bad := range forbiddenKillImports {
			if p == bad {
				imports++
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			for _, bad := range forbiddenKillIdents {
				if x.Name == bad {
					idents++
				}
			}
		case *ast.BasicLit:
			if x.Kind == token.STRING {
				for _, bad := range forbiddenStateStrings {
					if strings.Trim(x.Value, "`\"") == bad {
						strs++
					}
				}
			}
		}
		return true
	})
	if imports == 0 {
		t.Error("the wall does not catch a forbidden import")
	}
	if idents == 0 {
		t.Error("the wall does not catch a forbidden identifier")
	}
	if strs == 0 {
		t.Error("the wall does not catch a forbidden state literal")
	}
}

// TestNoLiveHostIsDialled is the tripwire the observability packages share (the
// internal/preview/caddy precedent): this packet spends $0 and dials nothing.
// The direct arm is created and enqueued; it never reaches an engine here.
func TestNoLiveHostIsDialled(t *testing.T) {
	dir := filepath.Join(repoRoot, pkgDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	fset := token.NewFileSet()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			v := strings.Trim(lit.Value, "`\"")
			if host, ok := urlHost(v); ok {
				t.Errorf("%s carries a routable URL literal %q (host %q) — this packet dials nothing and spends $0",
					e.Name(), v, host)
			}
			return true
		})
	}
	if scanned == 0 {
		t.Fatal("the live-host tripwire scanned no files — it would pass vacuously")
	}
}

// sourceFiles returns the package's non-test source files.
func sourceFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, pkgDir))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		out = append(out, filepath.Join(repoRoot, pkgDir, e.Name()))
	}
	return out
}

// urlHost extracts the host of an absolute http(s) URL literal, reporting
// ok=false for anything that is not one — including the bare scheme strings
// this tripwire itself contains (the internal/watchlist precedent).
func urlHost(v string) (string, bool) {
	rest, ok := strings.CutPrefix(v, "http://")
	if !ok {
		if rest, ok = strings.CutPrefix(v, "https://"); !ok {
			return "", false
		}
	}
	host, _, _ := strings.Cut(rest, "/")
	if host == "" {
		return "", false
	}
	return host, true
}

// TestNoBareAsksRow is acceptance item 22's negative and the §31/§32/§34
// standing rule: a run-less card is NEVER a bare `asks` row (`asks.run_id` is
// NOT NULL, and a benchmark pair has no run of its own). Verdict forms and
// alarm cards DERIVE — from the 0014 tables and from the benchmark.alarm rows.
// Asserted over the package's SQL string literals, where the hazard actually
// lives.
func TestNoBareAsksRow(t *testing.T) {
	fset := token.NewFileSet()
	scanned := 0
	for _, path := range sourceFiles(t) {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		scanned++
		base := filepath.Base(path)
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			sql := strings.ToLower(strings.Trim(lit.Value, "`\""))
			if !strings.Contains(sql, "select") && !strings.Contains(sql, "insert") &&
				!strings.Contains(sql, "update") {
				return true
			}
			for _, clause := range []string{"into asks", "from asks", "update asks", " asks "} {
				if strings.Contains(sql, clause) {
					t.Errorf("%s has SQL touching the asks table (%q) — a benchmark card is never a bare asks row (asks.run_id is NOT NULL)", base, clause)
				}
			}
			return true
		})
	}
	if scanned == 0 {
		t.Fatal("the asks-row scan read no files — it would pass vacuously")
	}
}

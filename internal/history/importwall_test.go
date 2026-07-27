package history_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
)

// importwall_test.go — the OQ4 import wall (the §31/§32/§33/§34/§35/§36
// precedent), matched exact-or-prefix so a SUBPACKAGE of a banned package is
// caught too, with a FORWARD allowlist so a new edge is deliberate, and a
// non-tautology probe: a wall that cannot fail guarantees nothing.

const modulePath = "github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/"

// allowed is the sanctioned forward allowlist for internal/history, per the
// ratified OQ4 disposition.
var allowed = map[string]string{
	"internal/storage":  "the DB seam — migration 0016's views and 0015's FTS corpus are read through it",
	"internal/eventlog": "the S14.2 family registry, so the view's type literals are pinned to the registry rather than copied",
	"internal/settings": "sanctioned by the OQ4 disposition; NOT imported today — this package declares no ⚙ and reads none",
	// The two DECLARED edges, each with its reason.
	"internal/local":  "the DUTY-CALLING layers: Layer-1 intent classification and slot fill ride alias intent-filling through *local.Duty (nil-safe), and the S12.5 margin/calibration types are that package's. Composing this through a func seam would put the schema shape, the margin extraction and the calibration key in the shell, which is composition, not logic",
	"internal/redact": "codor C2 (§30): the SEARCH QUERY is redacted before it is matched and every excerpt is redacted on the way out. A stdlib-only leaf with no in-repo imports — B5-2 OQ2 built it for exactly this reuse — so the edge adds no transitive dependency and cannot cycle",
}

// forbidden names representative packages internal/history must never import.
// The forward allowlist above is the real gate; this list makes the most
// tempting edges fail with an explanation.
var forbidden = map[string]string{
	"internal/metering":  "THE LOAD-BEARING ONE. S14.10 ¶1: the assistant SELECTS a priced view and never computes money by generation. Importing the meter would put the price table one call away from the query layer",
	"internal/retention": "B5-8A's package owns the corpus and its own refs-only read; this package reads the same tables. The C2 property belongs to the CORPUS, not to either reader",
	"internal/api":       "ONE DIRECTION ONLY. internal/api HOLDS *history.Store on its Config for the S15 assistant (the Preview precedent), so api→history exists and is deliberate; history→api must never, or the leaf acquires a transport dependency (§31)",
	"internal/run":       "runs are read as ROWS; a query surface must not depend on the FSM package",
	"internal/stage":     "no pipeline stage, no engine, no spawn — the advisory run rides the AdvisoryRun func seam",
	"internal/verify":    "verdict texts are read from run_events by their canonical type string, never through the judging package",
	"internal/watchdog":  "detection is S14.4's; this package queries what detection recorded",
	"internal/watchlist": "drift findings are read as rows, never through the watcher",
	"internal/benchmark": "benchmark records are rows too",
	"internal/scheduler": "the shell owns WHEN; this package owns the LOGIC",
	"internal/backup":    "the 11.3 exporter boundary is B5-8A's view; this package neither exports nor dumps",
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
				t.Errorf("%s imports %s, which is outside the sanctioned forward allowlist — adding an edge is a deliberate wall update, never a silent one", file, path)
			}
		}
	}
}

// TestImportWallCatchesSubpackages is the non-tautology probe.
func TestImportWallCatchesSubpackages(t *testing.T) {
	for _, path := range []string{
		modulePath + "internal/metering",
		modulePath + "internal/retention",
		modulePath + "internal/api",
		modulePath + "internal/adapters/claudecli",
		modulePath + "internal/stage",
		modulePath + "internal/watchdog",
	} {
		if _, bad := forbiddenImport(path); !bad && inForwardAllowlist(path) {
			t.Errorf("the wall admits %s — it would pass vacuously", path)
		}
	}
	for _, path := range []string{
		modulePath + "internal/storage",
		modulePath + "internal/eventlog",
		modulePath + "internal/local",
		modulePath + "internal/redact",
		"context", "database/sql", "regexp",
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

// TestSettingsRideNoInterfaceAtAll: this package declares and reads NO ⚙ key.
// The three number-shaped choices (row bounds, excerpt bound, duty caps) are
// STRUCTURAL CONSTANTS with their reasons (OQ6 posture), so internal/settings
// is permitted by the wall and unused in fact — and the ⚙ tally stays 118/33.
func TestSettingsRideNoInterfaceAtAll(t *testing.T) {
	for file, imports := range packageImports(t) {
		for _, path := range imports {
			if path == modulePath+"internal/settings" {
				t.Errorf("%s imports internal/settings — this package declares and reads no ⚙ key", file)
			}
		}
	}
}

// TestPackageOwnsNoTicker: the shell owns WHEN (§31/§34/§35/§36).
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

// TestTheQueryLayerNeverWrites — the query surface executes no write. Every
// statement in the package and every statement in the catalog is a SELECT, and
// nothing here opens a write transaction or an Exec.
//
// HONESTLY NARROWED BY THE C HALF (B5-8B sitting 2). The B half could say "the
// package never writes at all"; Layer 2 makes that false in one specific and
// mandated way — S14.10 ¶3 requires every generated query to be audit-logged,
// so layer2.go appends ONE event type. The scan therefore asserts the sharper
// property: no file executes SQL of its own, and the ONLY append is the audit
// row. `guard.go` is exempt from the write-verb literal scan for the obvious
// reason that its whole job is to NAME those verbs in order to refuse them —
// and TestGuardNamesEveryWriteVerbItRefuses below is the positive limb that
// keeps the exemption honest.
func TestTheQueryLayerNeverWrites(t *testing.T) {
	files := 0
	for _, name := range goFiles(t, ".") {
		files++
		src := readFile(t, name)
		for _, verb := range []string{"WriteTx", "ExecContext"} {
			if strings.Contains(src, verb) {
				t.Errorf("%s contains %q — the query surface reads and never writes", name, verb)
			}
		}
		if strings.HasSuffix(name, "guard.go") {
			continue
		}
		for _, verb := range []string{"INSERT ", "UPDATE ", "DELETE ", "CREATE ", "DROP ", "ALTER "} {
			if strings.Contains(src, verb) {
				t.Errorf("%s contains %q — the query surface reads and never writes", name, verb)
			}
		}
	}
	if files == 0 {
		t.Fatal("the write scan read no files — it would pass vacuously")
	}
	// Non-tautology probe.
	if !strings.Contains(`db.WriteTx(ctx, func(tx *sql.Tx) error { return nil })`, "WriteTx") {
		t.Fatal("the write scan cannot detect its own probe")
	}
}

// TestGuardNamesEveryWriteVerbItRefuses — the positive limb behind guard.go's
// exemption above. A guard that stopped naming the write verbs would have
// stopped refusing them, and the exemption would then be hiding the regression
// it was granted to permit.
func TestGuardNamesEveryWriteVerbItRefuses(t *testing.T) {
	src := readFile(t, filepath.Join(".", "guard.go"))
	for _, verb := range []string{"insert", "update", "delete", "drop", "alter", "create", "attach", "pragma"} {
		if !strings.Contains(src, `"`+verb+`"`) {
			t.Errorf("guard.go no longer names %q as a refused statement word", verb)
		}
		stmt := strings.ToUpper(verb) + " something"
		if _, err := history.Guard(stmt, history.Scope{Operator: true}, 10); err == nil {
			t.Errorf("Guard accepted %q", stmt)
		}
	}
}

// TestLimitEventTypesArePinnedToTheRegistry — the migration's limit_event_history
// view holds two TYPE LITERALS. They are pinned here to the eventlog registry,
// so a retype fails loudly instead of silently emptying the view. The literals
// live in SQL because a view cannot read a Go registry; this is the coupling
// that keeps them honest (the §36 marker-pinning precedent).
func TestLimitEventTypesArePinnedToTheRegistry(t *testing.T) {
	sql := migrationSQLOnly(t)
	for _, typ := range []string{"limit.event", "engine.rate_limit"} {
		fam, known := eventlog.Classify(typ)
		if !known {
			t.Errorf("the view names %q, which the S14.2 registry does not know", typ)
			continue
		}
		if fam != eventlog.FamilyUsageLimits {
			t.Errorf("%q classifies to family %q, not the usage/limits family the view claims", typ, fam)
		}
		if !strings.Contains(sql, "'"+typ+"'") {
			t.Errorf("migration 0016's limit_event_history does not name %q", typ)
		}
	}
	// The verdict type and its registered legacy alias are pinned the same way,
	// for routing_quality's outcome join.
	for _, typ := range []string{"verdict.recorded", "verify.round"} {
		if fam, known := eventlog.Classify(typ); !known || fam != eventlog.FamilyVerificationVerdict {
			t.Errorf("%q does not classify to the verification-verdict family (known=%v fam=%q)", typ, known, fam)
		}
		if !strings.Contains(sql, "'"+typ+"'") {
			t.Errorf("migration 0016 does not name the verdict type %q", typ)
		}
	}
	// And routing.decided, the view's own subject.
	if fam, known := eventlog.Classify("routing.decided"); !known || fam != eventlog.FamilyRouting {
		t.Errorf("routing.decided does not classify to the routing family (known=%v fam=%q)", known, fam)
	}
}

// TestExactlyOneEventTypeIsMinted — the §29 inventory, honestly restated by the
// C half. The B half (Layers 0/1 + search) registered NOTHING and left the
// registry at B5-8A's 94. The C half registers exactly ONE type,
// history.query_audited, because S14.10 ¶3 requires every generated query to be
// audit-logged — so 95 registered, and the append appears in exactly one file.
//
// The Layer 0/1 surface stays read-only, which is the part worth keeping: an
// append anywhere but layer2.go is still a defect.
func TestExactlyOneEventTypeIsMinted(t *testing.T) {
	if n := len(eventlog.Registry().Types()); n != 95 {
		t.Errorf("the S14.2 registry holds %d types, want 95 (B5-8A's 94 + history.query_audited)", n)
	}
	if _, ok := eventlog.Registry().TypeSpec(history.EventQueryAudited); !ok {
		t.Errorf("%q is not registered in the S14.2 contract (CONVENTIONS §29)", history.EventQueryAudited)
	}
	appenders := 0
	for _, name := range goFiles(t, ".") {
		src := readFile(t, name)
		if !strings.Contains(src, "eventlog.Append{") {
			continue
		}
		appenders++
		if !strings.HasSuffix(name, "layer2.go") {
			t.Errorf("%s appends an event — only the Layer-2 audit record may (S14.10 ¶3)", name)
		}
	}
	if appenders != 1 {
		t.Errorf("%d files append events, want exactly 1 (layer2.go's audit record)", appenders)
	}
	// The package's one other eventlog use is the registry, for the type pins.
	_ = history.CatalogNames
}

package preview

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// STANDING conformance battery (Spec S13.8; CONVENTIONS §25; the §17/§23
// import-graph precedent): the preview module's isolation and the host-hazard
// discipline are proven by inspection on every run, never assumed.

// TestImportGraph pins that internal/preview is a READ-ONLY consumer of settled
// substrate (R24): it never imports internal/{adapters,stage,intake,memory,
// broker,gates,verify,worker,scheduler} — previews are not engine/effect
// machinery — and imports only the allowed ceiling (project/review/sandbox/
// units/eventlog/portpool/storage/settings) among internal packages.
func TestImportGraph(t *testing.T) {
	forbidden := []string{
		"internal/adapters", "internal/stage", "internal/intake", "internal/memory",
		"internal/broker", "internal/gates", "internal/verify", "internal/worker",
		"internal/scheduler",
	}
	allowedInternal := map[string]bool{}
	for _, p := range []string{"project", "review", "sandbox", "units", "eventlog", "portpool", "storage", "settings"} {
		allowedInternal["github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/"+p] = true
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
			for _, f := range forbidden {
				if strings.Contains(path, f) {
					t.Errorf("%s imports %q — forbidden (R24: previews are read-only consumers, not engine/effect machinery)", name, path)
				}
			}
			if strings.Contains(path, "Sinet-Agentic-Control-Hub/internal/") && !allowedInternal[path] {
				t.Errorf("%s imports internal package %q — outside the R24 ceiling", name, path)
			}
		}
	}
}

// TestHostHazardTripwire pins that no TEST or fixture DIALS/BINDS the LIVE front
// chain (Spec S13.8 host hazard; §10 live-host guard): no test string literal
// references the live Caddy admin API (127.0.0.1:2019), binds/dials ports
// 8481/8482, or shells tailscale/systemctl. It inspects STRING LITERALS only
// (via AST) so the source's host-hazard WARNINGS — which live in comments — are
// not flagged; this file is skipped since it names the banned tokens by
// necessity. Tests use loopback httptest doubles and t.TempDir() sockets only.
func TestHostHazardTripwire(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	banned := []string{":8481", ":8482", "8481:", "8482:", "tailscale", "systemctl", ":2019"}
	fset := token.NewFileSet()
	for _, name := range files {
		if name == "conformance_test.go" {
			continue // this file defines the banned list
		}
		parsed, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			val, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, b := range banned {
				if strings.Contains(val, b) {
					t.Errorf("%s has a string literal %q referencing the LIVE front chain / host state — host hazard", name, val)
				}
			}
			return true
		})
	}
}

// TestNoNewMigration pins that the preview packet adds NO migration (Spec S13.8:
// previews are disposable — no durable preview table; live-preview state is
// in-memory + the event trace, R23). The migrations dir stays 0001–0009.
func TestNoNewMigration(t *testing.T) {
	sqls, err := filepath.Glob("../storage/migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range sqls {
		name := filepath.Base(p)
		// The preview packet (B4-4) adds NO migration — disposable previews have
		// no durable table (R23). Later packets added their own: 0010 by B4-7
		// (S12.5/S12.9 calibration + battery records), 0011 by B5-4 (S14.5
		// conformance registry), 0012 by B5-5 (S14.8 eval floors +
		// revalidation stamps), 0013 by B5-6A (the S14.6 watch-row config store),
		// 0014 by B5-7 (the S14.7 benchmark-practice state), 0015 by B5-8A
		// (the S14.9 retention substrate), 0016 by B5-8B (the S14.10 Layer-0
		// query views) and 0017 by B6-2B (the S10.4 durable budgets + pause
		// switch and the S15.5 queue-row drag hint) and 0018 by B6-2C (the
		// BENCH-REG §2 direct-arm capture column) and 0019 by B6-3A (the
		// S10.3 price table's durable home, `price_rows`) and 0020 by B6-7
		// (the S15.7 assistant's sessions, transcript, turns and exchange
		// manifest); anything beyond those would be an unexpected
		// preview-side add.
		if name >= "0022" {
			t.Errorf("unexpected migration %q — the preview packet adds none (R23; disposable previews have no durable table)", name)
		}
	}
}

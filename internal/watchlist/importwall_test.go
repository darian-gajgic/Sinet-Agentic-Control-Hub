package watchlist_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// importwall_test.go — the R1 import wall (the §31/§32/§33 precedent), matched
// exact-or-prefix so a SUBPACKAGE of a banned package is caught too, with a
// non-tautology probe: a wall that cannot fail guarantees nothing.

const modulePath = "github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/"

// forbidden names the packages internal/watchlist must never import, with the
// reason each edge is barred.
var forbidden = map[string]string{
	"internal/api":      "cards DERIVE from the log; internal/api reads drift.finding rows without an import in either direction (§31)",
	"internal/adapters": "the watchlist observes the outside world; it never touches the engine contract",
	"internal/worker":   "the S14.8 flag surface rides a func seam composed at the shell root (§28/§33)",
	"internal/stage":    "the watchlist is not a pipeline stage and has no run",
	"internal/verify":   "the quality gate is S07's; this package records and detects, it never judges",
	"internal/evals":    "the revalidation runbook rides a func seam composed at the shell root, so the evals→verify→intake→adapters graph stays out of this package (§28/§33)",
	"internal/settings": "⚙ values ride this package's own narrow Settings interface, by dotted key (§2)",
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
	pkgs, err := parser.ParseDir(fset, filepath.Join(repoRoot, "internal/watchlist"), func(fi os.FileInfo) bool {
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
		modulePath + "internal/api",
		modulePath + "internal/adapters/claudecli",
		modulePath + "internal/worker/automation",
		modulePath + "internal/stage",
		modulePath + "internal/verify",
		modulePath + "internal/evals",
		modulePath + "internal/settings",
	} {
		if _, bad := forbiddenImport(path); !bad {
			t.Errorf("the wall let %s through", path)
		}
	}
	for _, path := range []string{
		modulePath + "internal/storage",
		modulePath + "internal/eventlog",
		modulePath + "internal/local",
		modulePath + "internal/metering",
		modulePath + "internal/apiary", // a name that merely starts with "api"
		"strings",
	} {
		if why, bad := forbiddenImport(path); bad {
			t.Errorf("the wall wrongly rejected %s: %s", path, why)
		}
	}
}

// TestNoLiveHostIsDialled is the AST/string-literal tripwire (the
// internal/preview/caddy.go precedent): no source or test file in this package
// may name a routable live host. Every outbound call in the suites goes to an
// httptest stub bound to loopback, and the seeded provider URLs are DATA in
// seed.go — never a literal a test dials. Sources are exempt from the seed-data
// scan; test files are not.
func TestNoLiveHostIsDialled(t *testing.T) {
	dir := filepath.Join(repoRoot, "internal/watchlist")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	fset := token.NewFileSet()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), "_test.go") {
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
			host, ok := urlHost(v)
			if !ok {
				return true
			}
			// Loopback (an httptest stub) and the reserved .invalid TLD (a name
			// that can never resolve) are the only hosts a test may name.
			switch {
			case host == "127.0.0.1", host == "[::1]", host == "localhost",
				strings.HasSuffix(host, ".invalid"):
				return true
			}
			t.Errorf("%s carries a routable URL literal %q — tests never dial a live host (CONVENTIONS §3; the preview/caddy tripwire)", e.Name(), v)
			return true
		})
	}
	if scanned == 0 {
		t.Fatal("the live-host tripwire scanned no test files — it would pass vacuously")
	}
}

// urlHost extracts the host of an absolute http(s) URL literal, reporting
// ok=false for anything that is not one (including the bare scheme strings this
// tripwire itself contains).
func urlHost(v string) (string, bool) {
	rest, ok := strings.CutPrefix(v, "http://")
	if !ok {
		if rest, ok = strings.CutPrefix(v, "https://"); !ok {
			return "", false
		}
	}
	if rest == "" {
		return "", false
	}
	host, _, _ := strings.Cut(rest, "/")
	host, _, _ = strings.Cut(host, "?")
	if h, _, found := strings.Cut(host, ":"); found && !strings.HasPrefix(host, "[") {
		host = h
	}
	if host == "" {
		return "", false
	}
	return host, true
}

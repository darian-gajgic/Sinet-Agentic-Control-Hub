package push_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestTheOnlyOutboundDestinationIsAStoredEndpoint is the S01.8 register-row-2
// wall, as a checkable negative rather than a promise.
//
// The register admits exactly one new observable from this packet: push timing
// and volume reaching the browser vendor's own relay. So the package's
// production code must contain NO absolute URL at all — no third-party asset,
// no probe, no analytics, no telemetry endpoint, no vendor API host. The only
// address anything here dials is `sub.Endpoint`, which came out of a row a
// person's own browser enrolled.
//
// The scan reads STRING LITERALS from the parsed AST rather than the raw text,
// so a URL inside a comment (this file's own explanation, for one) is not a
// hit, and a URL split across a concatenation still is — the halves are two
// literals and the prefix half trips.
func TestTheOnlyOutboundDestinationIsAStoredEndpoint(t *testing.T) {
	hits, scanned := scanForAbsoluteURLs(t, packageFiles(t, false))
	if scanned == 0 {
		t.Fatal("the scan read no files: it cannot be enforcing anything")
	}
	if len(hits) != 0 {
		t.Errorf("internal/push production code carries absolute URLs, which the S01.8 register does not admit:\n  %s",
			strings.Join(hits, "\n  "))
	}

	// The scan must be able to fail, or "no hits" proves nothing. Both a bare
	// literal and a concatenated one are caught.
	planted := map[string]string{
		"probe.go":  `package push` + "\n" + `const probe = "https://telemetry.example.com/collect"`,
		"split.go":  `package push` + "\n" + `const split = "https://" + "cdn.example.com/font.woff2"`,
		"clean1.go": `package push` + "\n" + `// https://webkit.org/blog/16535 is a citation, not a destination` + "\n" + `const ok = "aes128gcm"`,
		"clean2.go": `package push` + "\n" + `const rel = "/inbox/"`,
	}
	dir := t.TempDir()
	var paths []string
	for name, body := range planted {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("plant: %v", err)
		}
		paths = append(paths, p)
	}
	plantedHits, _ := scanForAbsoluteURLs(t, paths)
	if len(plantedHits) != 2 {
		t.Errorf("the planted probe produced %d hits, want 2 (the bare literal and the concatenated one): %v", len(plantedHits), plantedHits)
	}
	for _, want := range []string{"probe.go", "split.go"} {
		found := false
		for _, h := range plantedHits {
			if strings.Contains(h, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("the scan missed the violation planted in %s", want)
		}
	}
	for _, notWant := range []string{"clean1.go", "clean2.go"} {
		for _, h := range plantedHits {
			if strings.Contains(h, notWant) {
				t.Errorf("the scan flagged %s, which carries no outbound destination: %s", notWant, h)
			}
		}
	}
}

// TestNoTickerOrTimerDrivesThisPackage: WHEN an evaluation happens is the
// shell's business (§32, the dead-man precedent) and dueness derives from
// stored state. A timer here would be a second, competing schedule.
func TestNoTickerOrTimerDrivesThisPackage(t *testing.T) {
	banned := []string{"time.NewTicker", "time.NewTimer", "time.Tick(", "time.AfterFunc", "go func()"}
	var hits []string
	files := packageFiles(t, false)
	if len(files) == 0 {
		t.Fatal("the scan read no files")
	}
	for _, p := range files {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		for _, b := range banned {
			if strings.Contains(string(raw), b) {
				hits = append(hits, filepath.Base(p)+": "+b)
			}
		}
	}
	if len(hits) != 0 {
		t.Errorf("internal/push drives its own schedule: %v", hits)
	}
	// The probe, so the assertion is shown able to fire.
	if !strings.Contains("x := time.NewTicker(time.Second)", banned[0]) {
		t.Fatal("the ticker predicate cannot match its own token")
	}
}

// packageFiles lists internal/push's .go files. `withTests` false is the
// production surface, which is what the walls above are about.
func packageFiles(t *testing.T, withTests bool) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if !withTests && strings.HasSuffix(name, "_test.go") {
			continue
		}
		out = append(out, name)
	}
	return out
}

// scanForAbsoluteURLs reports every string literal that names a scheme-bearing
// destination. It parses rather than greps, so prose citing a URL is not a hit
// and a concatenated one still is.
func scanForAbsoluteURLs(t *testing.T, paths []string) (hits []string, scanned int) {
	t.Helper()
	fset := token.NewFileSet()
	for _, p := range paths {
		f, err := parser.ParseFile(fset, p, nil, 0) // comments dropped: prose may cite what code may not dial
		if err != nil {
			t.Fatalf("parse %s: %v", p, err)
		}
		scanned++
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			v, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, scheme := range []string{"http://", "https://", "ws://", "wss://"} {
				if strings.Contains(v, scheme) {
					hits = append(hits, filepath.Base(p)+": "+lit.Value)
					return true
				}
			}
			return true
		})
	}
	return hits, scanned
}

package broker

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// STANDING conformance (brief R11/R29, P-T12-1): the broker is DB-FREE and
// imports stdlib + golang.org/x/sys ONLY — proven by inspection on every run.
// A DB import would put decrypted credential state one refactor away from the
// large-attack-surface persistence layer; the git verbs added at P3-B4-3 run
// git/ssh-keygen via os/exec (stdlib), never a DB.
func TestBrokerIsDBFreeStdlibOnly(t *testing.T) {
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
			if strings.Contains(path, "Sinet-Agentic-Control-Hub/internal/") {
				t.Errorf("%s imports internal package %q — the broker holds no platform packages, least of all a DB (P-T12-1)", name, path)
			}
			// The only permitted third-party import is golang.org/x/sys.
			if strings.Contains(path, ".") && strings.Contains(path, "/") &&
				!strings.HasPrefix(path, "golang.org/x/sys") {
				t.Errorf("%s imports third-party %q — the broker is stdlib + golang.org/x/sys only (R29)", name, path)
			}
		}
	}
}

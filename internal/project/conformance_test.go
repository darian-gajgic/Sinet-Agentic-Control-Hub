package project

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// STANDING conformance battery (brief R35; CONVENTIONS §17/§23; the §15 import-graph
// precedent): the git-topology/registry package's isolation is proven by
// inspection on every run, never assumed.

// TestImportGraphStorageEventlogOnly pins that internal/project imports
// storage + eventlog (+ stdlib) ONLY — it never imports internal/memory,
// internal/adapters, internal/stage, internal/intake, or internal/review, so
// the S09.1 capability walls stay transitively clean and the package is
// consumed only through narrow interfaces at the composition root (R35).
func TestImportGraphStorageEventlogOnly(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"internal/memory", "internal/adapters", "internal/stage",
		"internal/intake", "internal/review", "internal/ledger",
		"internal/verify", "internal/worker", "internal/scheduler",
	}
	allowedInternal := map[string]bool{
		"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage":  true,
		"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog": true,
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
					t.Errorf("%s imports %q — forbidden (R35: storage+eventlog+stdlib only)", name, path)
				}
			}
			if strings.Contains(path, "Sinet-Agentic-Control-Hub/internal/") && !allowedInternal[path] {
				t.Errorf("%s imports internal package %q — only storage+eventlog allowed (R35)", name, path)
			}
		}
	}
}

// TestNoBannedGitMechanisms pins "the net is the commits — never" (Spec S13.5,
// R24): the package never uses engine checkpoints, `git stash`, or jj as a
// safety/capture mechanism, and never amends or force-pushes (Spec S13.5 R15).
// A source-level tripwire complements the behavioural never-force test.
func TestNoBannedGitMechanisms(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	banned := []string{`"stash"`, `"--amend"`, `"--force"`, `"push"`, `"jj"`, "git stash"}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		src := string(body)
		for _, b := range banned {
			if strings.Contains(src, b) {
				t.Errorf("%s contains banned git mechanism %q (Spec S13.5: the net is the commits — never amend/force/stash/jj; pushes are S13.6/B4-3)", name, b)
			}
		}
	}
}

package lockfile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func validDoc() string {
	return `{
  "schema": 1,
  "governed_by": "test fixture",
  "components": [
    {
      "name": "example/component",
      "kind": "library",
      "pin": "1.2.3",
      "license": {"spdx": "MIT", "scope": "repo root", "checked": "2026-07-19"},
      "role": {"summary": "example role", "section": "S16"},
      "replacement": "rebuild in-tree",
      "abandonment": "default (settings registry seam)",
      "watch": ["quarterly pass"],
      "last_review": "2026-07-19",
      "modules": ["example.com/mod"]
    }
  ]
}`
}

func mustParse(t *testing.T, doc string) *File {
	t.Helper()
	f, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return f
}

func TestValidDocumentPasses(t *testing.T) {
	f := mustParse(t, validDoc())
	if problems := f.Validate(); len(problems) != 0 {
		t.Fatalf("Validate: unexpected problems: %v", problems)
	}
}

func TestUnknownFieldRejected(t *testing.T) {
	doc := strings.Replace(validDoc(), `"schema": 1,`, `"schema": 1, "surprise": true,`, 1)
	if _, err := Parse([]byte(doc)); err == nil {
		t.Fatal("Parse accepted an unknown top-level field")
	}
	doc = strings.Replace(validDoc(), `"kind": "library",`, `"kind": "library", "vibe": "good",`, 1)
	if _, err := Parse([]byte(doc)); err == nil {
		t.Fatal("Parse accepted an unknown entry field")
	}
}

func TestValidationCatchesFieldViolations(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(string) string
		wantHit string
	}{
		{"bad schema", func(d string) string { return strings.Replace(d, `"schema": 1`, `"schema": 2`, 1) }, "schema"},
		{"empty name", func(d string) string { return strings.Replace(d, `"name": "example/component"`, `"name": ""`, 1) }, "name is empty"},
		{"bad kind", func(d string) string { return strings.Replace(d, `"kind": "library"`, `"kind": "framework"`, 1) }, "kind"},
		{"latest pin", func(d string) string { return strings.Replace(d, `"pin": "1.2.3"`, `"pin": "latest"`, 1) }, "floating"},
		{"range pin", func(d string) string { return strings.Replace(d, `"pin": "1.2.3"`, `"pin": "^1.2.3"`, 1) }, "range"},
		{"empty spdx", func(d string) string { return strings.Replace(d, `"spdx": "MIT"`, `"spdx": ""`, 1) }, "license.spdx"},
		{"empty scope", func(d string) string { return strings.Replace(d, `"scope": "repo root"`, `"scope": ""`, 1) }, "license.scope"},
		{"bad checked date", func(d string) string { return strings.Replace(d, `"checked": "2026-07-19"`, `"checked": "July 19"`, 1) }, "license.checked"},
		{"empty replacement", func(d string) string {
			return strings.Replace(d, `"replacement": "rebuild in-tree"`, `"replacement": ""`, 1)
		}, "replacement"},
		{"empty abandonment", func(d string) string {
			return strings.Replace(d, `"abandonment": "default (settings registry seam)"`, `"abandonment": ""`, 1)
		}, "abandonment"},
		{"no watch rows", func(d string) string { return strings.Replace(d, `"watch": ["quarterly pass"]`, `"watch": []`, 1) }, "watch"},
		{"bad last_review", func(d string) string {
			return strings.Replace(d, `"last_review": "2026-07-19"`, `"last_review": "soon"`, 1)
		}, "last_review"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := mustParse(t, tc.mutate(validDoc()))
			problems := f.Validate()
			if len(problems) == 0 {
				t.Fatal("Validate found no problems")
			}
			if !anyContains(problems, tc.wantHit) {
				t.Fatalf("problems %v lack %q", problems, tc.wantHit)
			}
		})
	}
}

func TestDuplicateNamesAndModulesRejected(t *testing.T) {
	entry := `{
      "name": "example/component",
      "kind": "library",
      "pin": "1.2.3",
      "license": {"spdx": "MIT", "scope": "repo root", "checked": "2026-07-19"},
      "role": {"summary": "example role", "section": "S16"},
      "replacement": "rebuild in-tree",
      "abandonment": "default",
      "watch": ["quarterly pass"],
      "last_review": "2026-07-19",
      "modules": ["example.com/mod"]
    }`
	doc := fmt.Sprintf(`{"schema": 1, "governed_by": "t", "components": [%s, %s]}`, entry, entry)
	f := mustParse(t, doc)
	problems := f.Validate()
	if !anyContains(problems, "duplicate entry name") {
		t.Errorf("problems %v lack duplicate-name detection", problems)
	}
	if !anyContains(problems, "already claimed") {
		t.Errorf("problems %v lack duplicate-module detection", problems)
	}
}

func TestCheckGoModules(t *testing.T) {
	f := mustParse(t, validDoc())
	if problems := f.CheckGoModules([]string{"example.com/mod"}); len(problems) != 0 {
		t.Fatalf("covered module reported: %v", problems)
	}
	problems := f.CheckGoModules([]string{"example.com/mod", "example.com/uncovered"})
	if len(problems) != 1 || !strings.Contains(problems[0], "example.com/uncovered") {
		t.Fatalf("uncovered module not reported: %v", problems)
	}
}

func TestCheckGoDirective(t *testing.T) {
	doc := strings.Replace(validDoc(), `"name": "example/component"`, `"name": "Go toolchain"`, 1)
	doc = strings.Replace(doc, `"kind": "library"`, `"kind": "toolchain"`, 1)
	f := mustParse(t, doc)
	if problems := f.CheckGoDirective("1.2.3"); len(problems) != 0 {
		t.Fatalf("matching directive reported: %v", problems)
	}
	if problems := f.CheckGoDirective("1.2.4"); len(problems) != 1 {
		t.Fatalf("mismatched directive not reported: %v", problems)
	}
	noToolchain := mustParse(t, validDoc())
	if problems := noToolchain.CheckGoDirective("1.2.3"); len(problems) != 1 {
		t.Fatalf("missing toolchain entry not reported: %v", problems)
	}
}

func TestScanWorkflowUses(t *testing.T) {
	yml := []byte(`
jobs:
  ci:
    steps:
      - uses: actions/checkout@0000000000000000000000000000000000000000 # v1.0.0
      - name: local
        uses: ./composite/thing
      - uses: "actions/setup-go@v5"
`)
	uses := ScanWorkflowUses("ci.yml", yml)
	got := make([]string, len(uses))
	for i, u := range uses {
		got[i] = u.Value
	}
	want := []string{
		"actions/checkout@0000000000000000000000000000000000000000",
		"./composite/thing",
		"actions/setup-go@v5",
	}
	if len(got) != len(want) {
		t.Fatalf("scan got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("scan[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCheckWorkflowUses(t *testing.T) {
	doc := strings.Replace(validDoc(), `"name": "example/component"`, `"name": "actions/checkout"`, 1)
	f := mustParse(t, doc)
	sha := strings.Repeat("0", 40)

	if problems := f.CheckWorkflowUses([]Use{{Source: "ci.yml", Value: "actions/checkout@" + sha}}); len(problems) != 0 {
		t.Fatalf("pinned+covered use reported: %v", problems)
	}
	if problems := f.CheckWorkflowUses([]Use{{Source: "ci.yml", Value: "./local/action"}}); len(problems) != 0 {
		t.Fatalf("repo-local use reported: %v", problems)
	}
	for _, tc := range []struct {
		value   string
		wantHit string
	}{
		{"actions/checkout@v7", "not a full 40-hex commit SHA"},
		{"actions/checkout", "unpinned"},
		{"actions/setup-go@" + sha, "no components.lock entry"},
		{"docker://alpine:3", "docker"},
	} {
		problems := f.CheckWorkflowUses([]Use{{Source: "ci.yml", Value: tc.value}})
		if len(problems) != 1 || !strings.Contains(problems[0], tc.wantHit) {
			t.Errorf("%q: problems %v lack %q", tc.value, problems, tc.wantHit)
		}
	}
}

// TestRepositoryLockIsValid validates the real components.lock and workflow
// files, so `go test ./...` trips on manifest breakage even before the
// lockgate step runs.
func TestRepositoryLockIsValid(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "components.lock"))
	if err != nil {
		t.Fatalf("read components.lock: %v", err)
	}
	lock, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if problems := lock.Validate(); len(problems) != 0 {
		t.Fatalf("components.lock invalid: %v", problems)
	}

	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	m := regexp.MustCompile(`(?m)^go\s+(\S+)$`).FindSubmatch(goMod)
	if m == nil {
		t.Fatal("go.mod has no go directive")
	}
	if problems := lock.CheckGoDirective(string(m[1])); len(problems) != 0 {
		t.Fatalf("go directive vs lock: %v", problems)
	}

	workflows, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(workflows) == 0 {
		t.Fatal("no workflow files found under .github/workflows")
	}
	var uses []Use
	for _, wf := range workflows {
		content, err := os.ReadFile(wf)
		if err != nil {
			t.Fatal(err)
		}
		uses = append(uses, ScanWorkflowUses(filepath.Base(wf), content)...)
	}
	if len(uses) == 0 {
		t.Fatal("no uses: references found in workflows — scanner or workflow broken")
	}
	if problems := lock.CheckWorkflowUses(uses); len(problems) != 0 {
		t.Fatalf("workflow uses vs lock: %v", problems)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func anyContains(problems []string, substr string) bool {
	for _, p := range problems {
		if strings.Contains(p, substr) {
			return true
		}
	}
	return false
}

package verify

// contract_internal_test.go — the decision's own boundary cases, at the unit
// the acceptance battery reaches only through the drain: a malformed pattern
// in the plan (plan text is a boundary input and must never crash the drain),
// the segment-wise "**" matcher, and the normalizations both classes share.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// tree writes files (slash path → content) under a fresh temp dir and
// indexes it.
func tree(t *testing.T, files map[string]string) *treeIndex {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	idx, err := indexTree(root)
	if err != nil {
		t.Fatalf("indexTree: %v", err)
	}
	return idx
}

// TestContractMalformedPatternIsRecordedNotFatal: an unterminated character
// class is plan text the platform cannot read as a pattern. It is recorded
// undecidable, attributed to the plan member it came from — never a FAIL
// against the work, and never a panic in the drain.
func TestContractMalformedPatternIsRecordedNotFatal(t *testing.T) {
	idx := tree(t, map[string]string{"src/app.ts": "export {}\n"})
	for _, tc := range []struct {
		name string
		step intake.Step
		want string
	}{
		{
			name: "in the write set",
			step: intake.Step{ID: "S-1", DoneWhen: "it builds", WriteSet: []string{"src/[unterminated"}},
			want: attrPlanWriteSet,
		},
		{
			name: "in the done-when line",
			step: intake.Step{ID: "S-2", DoneWhen: "the file `src/[unterminated` is written"},
			want: attrPlanDoneWhen,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sc := decideFromTree(tc.step, idx, nil)
			if sc.State != ContractUnverifiable {
				t.Fatalf("state %q, want UNVERIFIABLE-HERE — a pattern the platform cannot read decides nothing about the work", sc.State)
			}
			if sc.AttributedTo != tc.want {
				t.Fatalf("attributed to %q, want %q", sc.AttributedTo, tc.want)
			}
			if sc.Detail == "" {
				t.Fatal("no reason recorded")
			}
		})
	}
}

// TestContractMalformedPatternNeverMasksARefutation: precedence — a pattern
// that IS refuted still FAILs even when a sibling pattern is malformed.
func TestContractMalformedPatternNeverMasksARefutation(t *testing.T) {
	idx := tree(t, map[string]string{"src/app.ts": "export {}\n"})
	step := intake.Step{ID: "S-1", DoneWhen: "photos land in `public/images/**` and `src/[unterminated` is written"}
	sc := decideFromTree(step, idx, nil)
	if sc.State != ContractFail {
		t.Fatalf("state %q, want FAIL", sc.State)
	}
	if sc.AttributedTo != contractTreeAttribution+"public/images/**" {
		t.Fatalf("attributed to %q, want the refuted pattern", sc.AttributedTo)
	}
}

// TestTreeMatchSpansSegments pins the matcher the two classes share: "**"
// spans zero or more segments, an ordinary segment spans exactly one, and a
// metacharacter-free directory name matches everything under it.
func TestTreeMatchSpansSegments(t *testing.T) {
	idx := tree(t, map[string]string{
		"public/images/a.jpg":       "x",
		"public/images/deep/b.jpg":  "x",
		"public/robots.txt":         "x",
		"src/data/products.json":    "[]",
		"src/data/nested/more.json": "{}",
	})
	for _, tc := range []struct {
		pattern string
		want    int
	}{
		{"public/images/**", 2},
		{"public/**", 3},
		{"public/*", 1},       // one segment only
		{"public/*/*.jpg", 1}, // a.jpg, not deep/b.jpg
		{"**/*.json", 2},      // ** spans zero or more leading segments
		{"public/images", 2},  // a directory names everything under it
		{"src/data/products.json", 1},
		{"public/images/none/**", 0},
	} {
		got, err := idx.match(tc.pattern)
		if err != nil {
			t.Fatalf("%s: %v", tc.pattern, err)
		}
		if len(got) != tc.want {
			t.Errorf("%s matched %d (%v), want %d", tc.pattern, len(got), got, tc.want)
		}
	}
}

// TestContractNamedFilePresentButEmptyIsRefuted: "there is a file there" is
// not the structural fact the line asked for when the file holds nothing.
func TestContractNamedFilePresentButEmptyIsRefuted(t *testing.T) {
	idx := tree(t, map[string]string{"src/data/products.json": ""})
	sc := decideFromTree(intake.Step{ID: "S-1", DoneWhen: "product data written to `src/data/products.json`"}, idx, nil)
	if sc.State != ContractFail {
		t.Fatalf("state %q, want FAIL for an empty named file", sc.State)
	}
	// A glob is not judged on emptiness — only a named file is.
	sc = decideFromTree(intake.Step{ID: "S-2", DoneWhen: "data lands under `src/data/**`"}, idx, nil)
	if sc.State != ContractPass {
		t.Fatalf("state %q, want PASS — a glob asks for a match, not for bytes", sc.State)
	}
}

// TestNamedPathsReadsOnlyPathShapedSpans: the parser is conservative by
// design — prose, commands, versions and URLs in backticks stay the judge's.
func TestNamedPathsReadsOnlyPathShapedSpans(t *testing.T) {
	paths, removal := namedPaths("`npm run build` passes, `react@19.2` is pinned, `https://x.test/a/b` responds, `README` exists, and `docs/guide.md` is written")
	if removal {
		t.Fatal("no removal wording in the line")
	}
	if len(paths) != 1 || paths[0] != "docs/guide.md" {
		t.Fatalf("named paths = %v, want only docs/guide.md", paths)
	}
}

// TestNormalizePatternSharesOneShape: both classes read a trailing slash as
// "everything under here" and ignore a leading ./ or /.
func TestNormalizePatternSharesOneShape(t *testing.T) {
	for in, want := range map[string]string{
		"./src/app.ts": "src/app.ts",
		"/public/":     "public/**",
		"legacy/":      "legacy/**",
		"src/**":       "src/**",
		"/":            "",
	} {
		if got := normalizePattern(in); got != want {
			t.Errorf("normalizePattern(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCoveringCriterionTakesTheLowestNumber: the blocker cites one criterion
// deterministically, whatever order the coverage map iterates in.
func TestCoveringCriterionTakesTheLowestNumber(t *testing.T) {
	cov := map[string][]string{"AC-10": {"S-2"}, "AC-2": {"S-2"}, "AC-1": {"S-1"}}
	for i := 0; i < 8; i++ {
		if got := coveringCriterion("S-2", cov); got != "AC-2" {
			t.Fatalf("criterion %q, want AC-2 (the lowest-numbered covering criterion)", got)
		}
	}
	if got := coveringCriterion("S-9", cov); got != "" {
		t.Fatalf("criterion %q for an uncovered step, want none", got)
	}
}

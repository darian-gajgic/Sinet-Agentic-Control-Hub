package review

import (
	"fmt"
	"strings"
	"testing"
)

// Rung-precise ladder tests over hand-built contents whose diff hunks are
// controlled (edits far apart stay separate hunks under --unified=3).

func longFile(n int, edits map[int]string, insertTop ...string) string {
	var sb strings.Builder
	for _, l := range insertTop {
		sb.WriteString(l + "\n")
	}
	for i := 1; i <= n; i++ {
		if v, ok := edits[i]; ok {
			if v != "" {
				sb.WriteString(v + "\n")
			}
			continue
		}
		fmt.Fprintf(&sb, "line-%02d\n", i)
	}
	return sb.String()
}

func files(content string) map[string]string {
	return map[string]string{"f.md": content}
}

func TestPortAnchorLadderRungs(t *testing.T) {
	src := longFile(20, nil)
	a := AnchorRecord{FilePath: "f.md", Side: SideNew, LineNo: 4, LineText: "line-04"}

	// Rung 1+2, unchanged position: an edit far below leaves the anchor
	// line outside every hunk at its own number → EXACT.
	dst := longFile(20, map[int]string{18: "edited"})
	status, placed := portAnchor(files(src), files(dst), a, 2)
	if status != AnchorExact || placed.LineNo != 4 {
		t.Fatalf("exact rung: %s %+v", status, placed)
	}

	// Rung 1+2, shifted: one line inserted at the top moves everything by
	// +1; the diff map tracks it → MAPPED at 5.
	dst = longFile(20, nil, "inserted-top")
	status, placed = portAnchor(files(src), files(dst), a, 2)
	if status != AnchorMapped || placed.LineNo != 5 || placed.LineText != "line-04" {
		t.Fatalf("mapped rung: %s %+v", status, placed)
	}

	// Rung 3: the anchored line swaps with its neighbor INSIDE a hunk —
	// the mapped guess misses, the ±drift search finds the quote →
	// DRIFTED.
	dst = strings.Replace(src, "line-03\nline-04\n", "line-04\nline-03\n", 1)
	status, placed = portAnchor(files(src), files(dst), a, 2)
	if status != AnchorDrifted || placed.LineNo != 3 {
		t.Fatalf("drifted rung: %s %+v", status, placed)
	}

	// ⚙ review.anchor_drift_lines = 0 tightens rung 3 away → FILE.
	status, _ = portAnchor(files(src), files(dst), a, 0)
	if status != AnchorFile {
		t.Fatalf("drift 0 degrades: %s", status)
	}

	// Rung 4: the quote is deleted, the file remains → FILE with the file
	// kept for the strip.
	dst = longFile(20, map[int]string{4: ""})
	status, placed = portAnchor(files(src), files(dst), a, 2)
	if status != AnchorFile || placed.FilePath != "f.md" {
		t.Fatalf("file rung: %s %+v", status, placed)
	}

	// Rung 5: the FILE is gone → ORPHAN, explicit.
	status, _ = portAnchor(files(src), map[string]string{"other.md": "x\n"}, a, 2)
	if status != AnchorOrphan {
		t.Fatalf("orphan rung: %s", status)
	}

	// Unavailable source content (old-side of revision 1): the map is
	// skipped, the text steps still decide — honest degradation, never a
	// drop.
	status, placed = portAnchor(nil, files(src), a, 2)
	if status != AnchorExact || placed.LineNo != 4 {
		t.Fatalf("nil source degrades to text steps: %s %+v", status, placed)
	}
}

func TestValidateBirth(t *testing.T) {
	content := files("alpha\nbravo\ncharlie\n")

	status, placed := validateBirth(content, AnchorRecord{FilePath: "f.md", Side: SideNew, LineNo: 2, LineText: "bravo"}, 2)
	if status != AnchorExact || placed.LineNo != 2 {
		t.Fatalf("exact birth: %s %+v", status, placed)
	}
	status, placed = validateBirth(content, AnchorRecord{FilePath: "f.md", Side: SideNew, LineNo: 3, LineText: "alpha"}, 2)
	if status != AnchorDrifted || placed.LineNo != 1 {
		t.Fatalf("drifted birth corrects the position: %s %+v", status, placed)
	}
	status, _ = validateBirth(content, AnchorRecord{FilePath: "f.md", Side: SideNew, LineNo: 1, LineText: "nope"}, 2)
	if status != AnchorFile {
		t.Fatalf("unfound quote degrades: %s", status)
	}
	status, _ = validateBirth(content, AnchorRecord{FilePath: "missing.md", Side: SideNew, LineNo: 1, LineText: "alpha"}, 2)
	if status != AnchorFile {
		t.Fatalf("missing file degrades: %s", status)
	}
}

func TestSearchNearNeverMatchesBlank(t *testing.T) {
	lines := []string{"a", "", "b", "", "c"}
	if _, ok := searchNear(lines, 2, "", 2); ok {
		t.Fatalf("a blank quote is no identity")
	}
	if n, ok := searchNear(lines, 2, "b", 2); !ok || n != 3 {
		t.Fatalf("search finds the nearest hit: %d %v", n, ok)
	}
}

func TestParseFindingAnchorShapes(t *testing.T) {
	fs := files("alpha\nbravo\n")
	if a, ok := ParseFindingAnchor(fs, "f.md:2"); !ok || a.LineNo != 2 || a.LineText != "bravo" {
		t.Fatalf("positional parse: %+v %v", a, ok)
	}
	for _, raw := range []string{"unknown:AC-1", "f.md:99", "other.md:1", "section 3", "", "f.md:"} {
		if _, ok := ParseFindingAnchor(fs, raw); ok {
			t.Fatalf("%q must not parse positional", raw)
		}
	}
}

func TestLineMapThroughRealDiff(t *testing.T) {
	oldC := longFile(20, nil)
	newC := longFile(20, map[int]string{10: "changed"}, "top-insert")
	unified, err := gitDiff("src/app.ts", oldC, newC)
	if err != nil {
		t.Fatalf("gitDiff: %v", err)
	}
	hs, err := parseHunks(unified)
	if err != nil {
		t.Fatalf("parseHunks: %v", err)
	}
	// Line 20 sits far below every hunk: direct map, shifted by the one
	// top insertion.
	mapped, direct := lineMap(hs, 20)
	if !direct || mapped != 21 {
		t.Fatalf("tail map: %d %v", mapped, direct)
	}
	// Line 1 above/inside the first hunk region still yields a usable
	// position ≥ 1.
	mapped, _ = lineMap(hs, 1)
	if mapped < 1 {
		t.Fatalf("head map: %d", mapped)
	}
}

func TestGitDiffIdentical(t *testing.T) {
	u, err := gitDiff("src/app.ts", "same\n", "same\n")
	if err != nil || u != "" {
		t.Fatalf("identical contents: %q err %v", u, err)
	}
}

// TestGitDiffHeadersNameTheLogicalPathAndNothingElse pins the B6-8 correction.
//
// The headers used to name the throwaway temp files, which made the served diff
// carry a host path and differ on EVERY read of the same immutable revision
// pair. Both halves are asserted, because either alone would pass over the bug:
// the real path is present, AND no temp-directory name survives anywhere in the
// output.
func TestGitDiffHeadersNameTheLogicalPathAndNothingElse(t *testing.T) {
	u, err := gitDiff("site/release.tsx", "one\ntwo\n", "one\ntwo\nthree\n")
	if err != nil {
		t.Fatalf("gitDiff: %v", err)
	}
	for _, want := range []string{
		"diff --git a/site/release.tsx b/site/release.tsx",
		"--- a/site/release.tsx",
		"+++ b/site/release.tsx",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("the diff headers must name the logical path — missing %q in:\n%s", want, u)
		}
	}
	if strings.Contains(u, "sinet-review-diff") {
		t.Errorf("a temp directory name reached the served diff:\n%s", u)
	}

	// Byte-stability across two runs is the property the golden fixtures rest on:
	// an immutable revision pair has ONE diff, not one per call.
	again, err := gitDiff("site/release.tsx", "one\ntwo\n", "one\ntwo\nthree\n")
	if err != nil {
		t.Fatalf("gitDiff again: %v", err)
	}
	if again != u {
		t.Errorf("the same contents diffed differently twice:\n%s\n%s", u, again)
	}

	// An empty name leaves git's own output alone — the honest no-label path,
	// asserted so the rewrite is known to be conditional rather than assumed.
	unlabeled, err := gitDiff("", "one\n", "two\n")
	if err != nil {
		t.Fatalf("gitDiff unlabeled: %v", err)
	}
	if !strings.Contains(unlabeled, "sinet-review-diff") {
		t.Errorf("with no name the output is git's own, temp paths included:\n%s", unlabeled)
	}
}

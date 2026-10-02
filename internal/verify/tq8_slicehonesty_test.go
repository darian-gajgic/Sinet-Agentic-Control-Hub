package verify_test

// tq8_slicehonesty_test.go — P3-TQ-8 drain round 1: the judge's slice says
// what it holds, and every rule the acceptance battery left unpinned is
// pinned here (Spec S07.5 the artifact + its diff; S07.11 recording;
// CONVENTIONS §38 plain words, §78 "bounded, and honest about the bound").
//
// Round-1 findings covered: F1 (a text row whose served diff is EMPTY is
// named omitted with a cause, never "all shown" and never "shown in part"),
// F4 (a file emptied in place is named), F6 (the report item's place on the
// manifest), F7 (the claims label rides the wire), F8 (the content section is
// a contiguous path-order prefix), F9 (the bound is inclusive at the cap and
// the shown==0 arm reads right), F10 (the renderer states causes in its own
// judge-facing words; review's page sentences never enter the slice).

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// tq8ReviewTwoSidedReason is review's own sentence for a file whose two
// sides are both over its per-file read cap: it serves NO diff text, marks
// the row truncated and says why — addressed to a person on a review page
// (review/tree.go fileDiff). The judge can open nothing, so this sentence
// must never reach the slice (F10).
const tq8ReviewTwoSidedReason = "src/big.go is 2.0 MB, which is too large to compare here — open the file to read it"

func tq8Filler(tag string, n int) string {
	line := tag + " " + strings.Repeat("x", 60) + "\n"
	var sb strings.Builder
	for sb.Len() < n {
		sb.WriteString(line)
	}
	return sb.String()[:n]
}

// TestTQ8ATextRowWithNoServedDiffIsNamedOmitted — F1. A non-binary row whose
// diff body comes back EMPTY is a file the judge did not see: review's
// two-sided over-cap arm serves exactly that shape, and so does a change to a
// file's mode or path alone. The row must be NAMED among the omitted with a
// cause, the record must say the slice was cut, and the artifact must never
// claim that every file with text is there in full — nor that this one is
// "shown only in part", when none of it is shown at all.
func TestTQ8ATextRowWithNoServedDiffIsNamedOmitted(t *testing.T) {
	added := verify.ChangedFile{
		Path: "src/small.go", Kind: verify.KindAdded, NewSize: 20, Additions: 1,
		Diff: "diff --git a/src/small.go b/src/small.go\nnew file mode 100644\n--- /dev/null\n+++ b/src/small.go\n@@ -0,0 +1 @@\n+package app\n",
	}
	overCap := verify.ChangedFile{
		Path: "src/big.go", Kind: verify.KindModified, OldSize: 2 << 20, NewSize: 2<<20 + 40, Additions: 2, Deletions: 1,
		DiffTruncated: true, DiffReason: tq8ReviewTwoSidedReason,
	}
	modeOnly := verify.ChangedFile{
		Path: "tools/run.sh", Kind: verify.KindModified, OldSize: 12, NewSize: 12,
		Content: "#!/bin/sh\nls\n",
	}

	cases := []struct {
		name  string
		files []verify.ChangedFile
		// omitted is the exact DiffsOmitted the slice must record.
		omitted []string
		shown   int
	}{
		{
			name:    "the over-cap file is the only text row",
			files:   []verify.ChangedFile{overCap},
			omitted: []string{"src/big.go"},
		},
		{
			name:    "an over-cap file beside a file that IS shown",
			files:   []verify.ChangedFile{overCap, added},
			omitted: []string{"src/big.go"},
			shown:   1,
		},
		{
			name:    "a change to the file mode alone has no diff text either",
			files:   []verify.ChangedFile{added, modeOnly},
			omitted: []string{"tools/run.sh"},
			shown:   1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc := verify.RevisionChange{OldN: 1, NewN: 2, OldPin: "s1", NewPin: "s2", Files: tc.files}
			artifact, diff, saw := verify.RenderChangeSlice(rc)

			if got := strings.Join(saw.DiffsOmitted, ","); got != strings.Join(tc.omitted, ",") {
				t.Fatalf("DiffsOmitted = %v, want %v — a text row with no diff body is a file the judge did not see (Spec S07.5; §78)", saw.DiffsOmitted, tc.omitted)
			}
			if saw.DiffsShown != tc.shown {
				t.Fatalf("DiffsShown = %d, want %d", saw.DiffsShown, tc.shown)
			}
			if !saw.Truncated {
				t.Fatal("JudgeSaw.Truncated = false although a file's changes were left out (Spec S07.11: the record says what was checked)")
			}
			for _, p := range tc.omitted {
				if n := strings.Count(artifact, p); n < 2 {
					t.Fatalf("%q appears %d times in the artifact, want ≥ 2 (inventory row + named as omitted):\n%s", p, n, artifact)
				}
			}
			// Never "all of them are here" and never "shown only in part"
			// about a file none of which is shown.
			if strings.Contains(artifact, "are there in full, in the order listed above.\n") && saw.DiffsShown != 0 {
				if strings.Contains(artifact, fmt.Sprintf("all %d", saw.DiffsShown)) {
					t.Fatalf("the artifact claims every file with text is shown in full while %v were not:\n%s", saw.DiffsOmitted, artifact)
				}
			}
			if strings.Contains(artifact, "No file in this change has text to compare") {
				t.Fatalf("a file WITH text whose diff was not served is not a change without text:\n%s", artifact)
			}
			if strings.Contains(artifact, "shown only in part") {
				t.Fatalf("nothing of the omitted file is shown, so it is not shown in part:\n%s", artifact)
			}
			// F10: review's page sentence is written for a person who can open
			// the file; it never reaches a judge that cannot.
			if strings.Contains(artifact, tq8ReviewTwoSidedReason) || strings.Contains(artifact, "open the file to read it") {
				t.Fatalf("review's review-page sentence reached the judge's slice:\n%s", artifact)
			}
			if tc.shown == 0 && diff != "" {
				t.Fatalf("no diff was served, so the diff item must be empty: %q", diff)
			}
		})
	}
}

// TestTQ8AFileEmptiedInPlaceIsNamedOnTheWire — F4. A modified file whose new
// content is empty (emptied in place) is in the content section's accounting
// and is named on the wire: the judge must not be left to infer the file from
// silence. R9's content clause: every modified/renamed row is shown or named.
func TestTQ8AFileEmptiedInPlaceIsNamedOnTheWire(t *testing.T) {
	rc := verify.RevisionChange{
		OldN: 1, NewN: 2, OldPin: "s1", NewPin: "s2",
		Files: []verify.ChangedFile{{
			Path: "notes.md", Kind: verify.KindModified, OldSize: 120, NewSize: 0, Deletions: 6,
			Diff:    "diff --git a/notes.md b/notes.md\n--- a/notes.md\n+++ b/notes.md\n@@ -1,6 +0,0 @@\n-old\n",
			Content: "",
		}},
	}
	artifact, _, saw := verify.RenderChangeSlice(rc)

	if saw.ContentShown+len(saw.ContentOmitted) != 1 {
		t.Fatalf("the emptied file is in neither ContentShown (%d) nor ContentOmitted (%v) — R9's content clause",
			saw.ContentShown, saw.ContentOmitted)
	}
	if n := strings.Count(artifact, "notes.md"); n < 2 {
		t.Fatalf("%q appears %d times in the artifact, want ≥ 2 (inventory row + the content section):\n%s", "notes.md", n, artifact)
	}
	if !strings.Contains(artifact, "no text") {
		t.Fatalf("the artifact never says the file holds no text at this version:\n%s", artifact)
	}
	if saw.ContentBytes != 0 {
		t.Fatalf("ContentBytes = %d for an empty file", saw.ContentBytes)
	}
}

// TestTQ8ContentSectionIsAContiguousPathOrderPrefix — F8. R3's "same rule"
// for the content section: the first content that does not fit stops it, and
// every later file's content is omitted even when it would have fit. A
// smaller file shown after a bigger one was cut would make "the first N"
// false and let the bound reorder what the judge reads.
func TestTQ8ContentSectionIsAContiguousPathOrderPrefix(t *testing.T) {
	mod := func(path string, content int) verify.ChangedFile {
		return verify.ChangedFile{
			Path: path, Kind: verify.KindModified, OldSize: int64(content), NewSize: int64(content), Additions: 1,
			Diff:    "diff --git a/" + path + " b/" + path + "\n@@ -1 +1 @@\n-a\n+b\n",
			Content: tq8Filler(" "+path, content),
		}
	}
	rc := verify.RevisionChange{OldN: 1, NewN: 2, OldPin: "s1", NewPin: "s2", Files: []verify.ChangedFile{
		mod("a.go", 100<<10), // fits
		mod("b.go", 100<<10), // does not fit: the cut
		mod("c.go", 1<<10),   // would fit the remainder — and must NOT be shown
	}}
	artifact, _, saw := verify.RenderChangeSlice(rc)

	if saw.ContentShown != 1 || strings.Join(saw.ContentOmitted, ",") != "b.go,c.go" {
		t.Fatalf("JudgeSaw content = shown %d omitted %v, want 1 shown and b.go,c.go omitted (a contiguous path-order prefix, R3)",
			saw.ContentShown, saw.ContentOmitted)
	}
	if !strings.Contains(artifact, rc.Files[0].Content) {
		t.Fatal("a.go's content fits and must be shown")
	}
	if strings.Contains(artifact, rc.Files[2].Content) {
		t.Fatalf("c.go's content was shown after the content cut — the section must stay a contiguous prefix:\n%s", artifact[:min(len(artifact), 2000)])
	}
	if saw.DiffBytes+saw.ContentBytes > verify.JudgeArtifactBytesCap {
		t.Fatalf("bodies %d exceed the bound %d", saw.DiffBytes+saw.ContentBytes, verify.JudgeArtifactBytesCap)
	}
}

// TestTQ8TheBoundIsInclusiveAtTheCap — F9. A body of exactly
// JudgeArtifactBytesCap bytes is INSIDE the bound (the cap is what the judge
// reads under, not one byte less), one byte more is omitted whole, and when
// nothing fits the artifact does not announce "the first 0 of N in full".
func TestTQ8TheBoundIsInclusiveAtTheCap(t *testing.T) {
	one := func(n int) verify.RevisionChange {
		return verify.RevisionChange{OldN: 1, NewN: 2, OldPin: "s1", NewPin: "s2", Files: []verify.ChangedFile{{
			Path: "a.txt", Kind: verify.KindAdded, NewSize: int64(n), Additions: 1, Diff: tq8Filler("+a", n),
		}}}
	}
	_, diff, saw := verify.RenderChangeSlice(one(verify.JudgeArtifactBytesCap))
	if saw.DiffsShown != 1 || saw.DiffBytes != verify.JudgeArtifactBytesCap || saw.Truncated || len(diff) != verify.JudgeArtifactBytesCap {
		t.Fatalf("a body of exactly the cap must be shown whole: %+v (diff %d bytes)", saw, len(diff))
	}

	artifact, diff, saw := verify.RenderChangeSlice(one(verify.JudgeArtifactBytesCap + 1))
	if saw.DiffsShown != 0 || len(saw.DiffsOmitted) != 1 || !saw.Truncated || diff != "" {
		t.Fatalf("one byte over the cap must be omitted whole: %+v (diff %d bytes)", saw, len(diff))
	}
	if strings.Contains(artifact, "the first 0 of") {
		t.Fatalf("nothing was shown, so there is no \"first 0\" to announce:\n%s", artifact)
	}
	if n := strings.Count(artifact, "a.txt"); n < 2 {
		t.Fatalf("the omitted file is named %d times, want ≥ 2:\n%s", n, artifact)
	}

	// The content section's own boundary: content that exactly fills what the
	// diffs left is shown.
	d := "diff --git a/m.go b/m.go\n@@ -1 +1 @@\n-a\n+b\n"
	rc := verify.RevisionChange{OldN: 1, NewN: 2, OldPin: "s1", NewPin: "s2", Files: []verify.ChangedFile{{
		Path: "m.go", Kind: verify.KindModified, OldSize: 4, NewSize: 4, Additions: 1,
		Diff: d, Content: tq8Filler(" m", verify.JudgeArtifactBytesCap-len(d)),
	}}}
	artifact, _, saw = verify.RenderChangeSlice(rc)
	if saw.ContentShown != 1 || saw.Truncated || saw.DiffBytes+saw.ContentBytes != verify.JudgeArtifactBytesCap {
		t.Fatalf("content filling the remainder exactly must be shown: %+v", saw)
	}
	if !strings.Contains(artifact, rc.Files[0].Content) {
		t.Fatal("m.go's content exactly fills the remainder and must be shown")
	}
}

// TestTQ8TheReportItemIsLabelledClaimsAndSitsAfterTheDiff — F6 + F7. R5's
// manifest: the executor's report rides AFTER verify/diff and BEFORE
// verify/rubric, and it rides LABELLED — the judge reads a claim as a claim,
// and the label names the two items a passing quote may come from (Spec
// S07.9 P-T06-3; CONVENTIONS §74).
func TestTQ8TheReportItemIsLabelledClaimsAndSitsAfterTheDiff(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	ctx := context.Background()
	d := deliverable("t1", "r1")
	const report = "# Step report\n\nI wired the header.\n"

	in, err := verify.BuildJudgeInput(ctx, f.ledger, d,
		verify.JudgeSlice{Artifact: "ARTIFACT BODY", Diff: "DIFF BODY", Report: report},
		verify.SeedSoftwareRubric(), nil, nil, 1)
	if err != nil {
		t.Fatalf("BuildJudgeInput: %v", err)
	}

	at := func(id string) int {
		for i, e := range in.Brief.Manifest {
			if e.ItemID == id {
				return i
			}
		}
		t.Fatalf("manifest holds no %q item: %+v", id, in.Brief.Manifest)
		return -1
	}
	artifact, diff, rep, rubric := at("verify/artifact"), at("verify/diff"), at("verify/executor-report"), at("verify/rubric")
	if !(artifact < diff && diff < rep && rep < rubric) {
		t.Fatalf("manifest order artifact=%d diff=%d report=%d rubric=%d, want the report after the diff and before the rubric (R5)",
			artifact, diff, rep, rubric)
	}
	// The same order on the wire the judge actually reads.
	wArtifact := strings.Index(in.BriefText, "verify/artifact")
	wDiff := strings.Index(in.BriefText, "verify/diff")
	wReport := strings.Index(in.BriefText, "verify/executor-report")
	if !(wArtifact < wDiff && wDiff < wReport) {
		t.Fatalf("wire order artifact=%d diff=%d report=%d:\n%s", wArtifact, wDiff, wReport, in.BriefText)
	}

	var block string
	for _, b := range in.Brief.Blocks {
		if b.ItemID == "verify/executor-report" {
			block = b.Content
		}
	}
	if block == "" {
		t.Fatal("no verify/executor-report block on the assembled brief")
	}
	if block == report || strings.HasPrefix(block, report) {
		t.Fatalf("the report rides UNLABELLED: a claim must reach the judge as a claim (R1):\n%s", block)
	}
	label := strings.TrimSuffix(block, report)
	for _, want := range []string{"not the work itself", "verify/artifact", "verify/diff"} {
		if !strings.Contains(label, want) {
			t.Fatalf("the claims label misses %q — it must say a claim is not evidence and name where a quote may come from:\n%s", want, label)
		}
	}
	if !strings.HasSuffix(block, report) {
		t.Fatalf("the report body must follow its label verbatim:\n%s", block)
	}
	if in.Report != report {
		t.Fatalf("JudgeInput.Report = %q, want the report verbatim", in.Report)
	}
}

// TestTQ8ASkippedContentLeavesTheServedDiffShown — F3 refinement, renderer
// side. A row whose diff was served but whose content the seam could not fit
// carries ContentSkipped: its content is named omitted, its diff stays on
// the wire, and the diffs after it are not cut.
func TestTQ8ASkippedContentLeavesTheServedDiffShown(t *testing.T) {
	rc := verify.RevisionChange{OldN: 1, NewN: 2, OldPin: "s1", NewPin: "s2", Files: []verify.ChangedFile{
		{Path: "a.go", Kind: verify.KindModified, OldSize: 9, NewSize: 9, Additions: 1, Deletions: 1,
			Diff: "diff --git a/a.go b/a.go\n@@ -1 +1 @@\n-x\n+A-HUNK\n", ContentSkipped: true},
		{Path: "b.go", Kind: verify.KindAdded, NewSize: 7, Additions: 1,
			Diff: "diff --git a/b.go b/b.go\n@@ -0,0 +1 @@\n+B-ADDED\n"},
	}}
	artifact, diff, saw := verify.RenderChangeSlice(rc)
	if saw.DiffsShown != 2 || len(saw.DiffsOmitted) != 0 || !strings.Contains(diff, "+A-HUNK") || !strings.Contains(diff, "+B-ADDED") {
		t.Fatalf("a served diff was dropped for its skipped content: %+v\n%s", saw, diff)
	}
	if saw.ContentShown != 0 || strings.Join(saw.ContentOmitted, ",") != "a.go" || !saw.Truncated {
		t.Fatalf("JudgeSaw content = %+v, want a.go's content named omitted", saw)
	}
	if !strings.Contains(artifact, "Content NOT shown, by file: a.go") {
		t.Fatalf("the omitted content is not named:\n%s", artifact)
	}
}

// TestTQ8TheDiffBoundSentenceOnlyWhenTheBoundCutADiff — F1/F3 together. A
// diff left out for having no text (a mode-only change) is not a diff the
// bound cut, and a content the bound cut is not one either: the diff
// paragraph must not say "what did not fit ... was left out" when no diff
// failed to fit.
func TestTQ8TheDiffBoundSentenceOnlyWhenTheBoundCutADiff(t *testing.T) {
	rc := verify.RevisionChange{OldN: 1, NewN: 2, OldPin: "s1", NewPin: "s2", Files: []verify.ChangedFile{
		{Path: "a.go", Kind: verify.KindModified, OldSize: 9, NewSize: 9, Additions: 1, Deletions: 1,
			Diff: "diff --git a/a.go b/a.go\n@@ -1 +1 @@\n-x\n+y\n", ContentSkipped: true},
		{Path: "run.sh", Kind: verify.KindModified, OldSize: 3, NewSize: 3, Content: "ls\n"},
	}}
	artifact, _, saw := verify.RenderChangeSlice(rc)
	if strings.Join(saw.DiffsOmitted, ",") != "run.sh" || strings.Join(saw.ContentOmitted, ",") != "a.go,run.sh" {
		t.Fatalf("JudgeSaw = %+v", saw)
	}
	if strings.Contains(artifact, "was left out whole, and so was everything after it") {
		t.Fatalf("the diff paragraph blames the bound although no diff failed to fit:\n%s", artifact)
	}
}

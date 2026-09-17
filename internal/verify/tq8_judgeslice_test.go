package verify_test

// tq8_judgeslice_test.go — P3-TQ-8 acceptance (Spec S07.5 judge input slice;
// S07.9 P-T06-3; S07.11 recording; S13.1/S13.2 revisions and their diff),
// committed RED by grounding (Amendment-A carve-out, CONVENTIONS §3).
//
// For a repo-backed deliverable the judge's artifact is the tree's CHANGE —
// the file inventory, the per-file diffs (an added file's diff is its
// content) and the full content of modified files — read from the platform
// store at the pinned refs through the verify.ChangeSource seam. The
// executor's step report is demoted to labelled claims: the judge still reads
// it, but a quote from it never satisfies a PASS. The slice is bounded at a
// FILE boundary, says so on the wire, and the round record says what the
// judge saw. A content-pinned deliverable's slice is byte-identical to
// today's. The seam is faked here over an in-memory change (the sit1Trees
// pattern); the real composition over a real git store is pinned in
// internal/stage (tq8_changesource_test.go).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// tq8Change is the fake ChangeSource: one scripted RevisionChange (or error)
// and a record of every call.
type tq8Change struct {
	rc    verify.RevisionChange
	err   error
	calls []tq8Call
}

type tq8Call struct {
	rev    int
	budget int
}

func (c *tq8Change) RevisionChange(_ context.Context, d verify.Deliverable, bodyBudget int) (verify.RevisionChange, error) {
	c.calls = append(c.calls, tq8Call{rev: d.Revision, budget: bodyBudget})
	if c.err != nil {
		return verify.RevisionChange{}, c.err
	}
	return c.rc, nil
}

const (
	tq8AppBase = "package app\n\nfunc Run() {}\n"
	tq8AppRev1 = "package app\n\nfunc Run() {\n\tserve()\n}\n"
	tq8Cart    = "package app\n\nfunc Cart() {}\n"
	tq8Readme  = "# shop\n"
	tq8PNG     = "\x89PNG\r\n\x1a\n\x00\x00IHDR"
	// tq8Report is the executor's step report: claims, never the artifact.
	// "wired the header" is the sentinel that must never enter the quotable
	// slice; "public/images" names a directory the tree does NOT hold — the
	// webshop lie the report-reading judge could not see.
	tq8Report = "# Step report\n\nI added the cart page, put the logo under public/images and wired the header.\n"
)

func tq8UnifiedAdded(path, content string) string {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	var sb strings.Builder
	fmt.Fprintf(&sb, "diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", path, path, path, len(lines))
	for _, l := range lines {
		sb.WriteString("+" + l + "\n")
	}
	return sb.String()
}

func tq8UnifiedDeleted(path, content string) string {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	var sb strings.Builder
	fmt.Fprintf(&sb, "diff --git a/%s b/%s\ndeleted file mode 100644\n--- a/%s\n+++ /dev/null\n@@ -1,%d +0,0 @@\n", path, path, path, len(lines))
	for _, l := range lines {
		sb.WriteString("-" + l + "\n")
	}
	return sb.String()
}

// tq8Tree is revision 1 against the pre-task base in the webshop's miniature:
// README.md deleted, assets/logo.png added (binary), src/app.go modified,
// src/cart.go added — path order.
func tq8Tree() verify.RevisionChange {
	return verify.RevisionChange{
		OldN: 0, NewN: 1, OldPin: "s0", NewPin: "s1", OldIsBase: true,
		Files: []verify.ChangedFile{
			{Path: "README.md", Kind: verify.KindDeleted, OldSize: int64(len(tq8Readme)), Deletions: 1,
				Diff: tq8UnifiedDeleted("README.md", tq8Readme)},
			{Path: "assets/logo.png", Kind: verify.KindAdded, NewSize: int64(len(tq8PNG)), Binary: true},
			{Path: "src/app.go", Kind: verify.KindModified, OldSize: int64(len(tq8AppBase)), NewSize: int64(len(tq8AppRev1)), Additions: 1,
				Diff:    "diff --git a/src/app.go b/src/app.go\n--- a/src/app.go\n+++ b/src/app.go\n@@ -1,3 +1,5 @@\n package app\n \n-func Run() {}\n+func Run() {\n+\tserve()\n+}\n",
				Content: tq8AppRev1},
			{Path: "src/cart.go", Kind: verify.KindAdded, NewSize: int64(len(tq8Cart)), Additions: 3,
				Diff: tq8UnifiedAdded("src/cart.go", tq8Cart)},
		},
	}
}

// tq8Deliverable is the repo-backed revision 1 whose artifact-of-record text
// is the executor's report (the live shape: stage.verifyInput hands the
// report as Content, the pins from RepoFacts).
func tq8Deliverable(taskID, runID string) verify.Deliverable {
	d := deliverable(taskID, runID)
	d.Type = "markdown"
	d.Content = tq8Report
	d.SnapshotSHA, d.BaseSHA, d.WriteClaimed = "s1", "s0", true
	return d
}

// tq8BuildOnlyPack binds no AC to a check, so every axis-1 verdict is the
// judge's own (nothing FromV1).
func tq8BuildOnlyPack() *verify.CheckPack {
	return &verify.CheckPack{
		Domain: verify.DomainSoftware, Version: 1, VerifiedOn: time.Now().Add(-24 * time.Hour),
		Checks: []verify.Check{
			{ID: "build", Stage: verify.StageStatic, Argv: []string{"true"}, StepID: "S-1", FindingCategory: verify.CatACBlocker},
		},
	}
}

func tq8ManifestIDs(in verify.JudgeInput) map[string]bool {
	ids := map[string]bool{}
	for _, e := range in.Brief.Manifest {
		ids[e.ItemID] = true
	}
	return ids
}

// TestTQ8RepoBackedJudgeSeesTheChangeNotTheReport — (1) the artifact is the
// tree's change: every inventory row, the diff of every file, the full
// content of the modified file; the report is labelled claims the judge
// reads but cannot quote; nothing rides the wire twice; the cost shape is
// still one compliance + one sanity call (Spec S07.11).
func TestTQ8RepoBackedJudgeSeesTheChangeNotTheReport(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{}
	v := f.verifier(j, &scriptRunner{}, passPack())
	fc := &tq8Change{rc: tq8Tree()}
	v.Change = fc

	out, err := v.Verify(context.Background(), input(tq8Deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(j.inputs) == 0 {
		t.Fatal("the judge was never called")
	}
	in := j.inputs[0]

	// The inventory is the table of contents: every row, by path, in the
	// quotable artifact — the binary one by name only, never its bytes.
	for _, row := range tq8Tree().Files {
		if !strings.Contains(in.Artifact, row.Path) {
			t.Fatalf("inventory row %q missing from the judge's artifact (Spec S07.5: the artifact + its diff; §78: the tree IS the deliverable):\n%s", row.Path, in.Artifact)
		}
	}
	if strings.Contains(in.Artifact, tq8PNG) || strings.Contains(in.Diff, tq8PNG) {
		t.Fatal("binary bytes reached the judge")
	}
	// The diff of every text file, whole: the deleted README, the modified
	// app.go hunk, the added cart.go (an added file IS its diff).
	for _, want := range []string{"-# shop", "+\tserve()", "+func Cart() {}", "diff --git a/src/cart.go b/src/cart.go"} {
		if !strings.Contains(in.Diff, want) {
			t.Fatalf("judge diff misses %q (Spec S13.2 host-side diff between revision pins):\n%s", want, in.Diff)
		}
	}
	// The modified file's full content is quotable (the diff alone shows
	// hunks); the added file's content is NOT repeated outside its diff.
	if !strings.Contains(in.Artifact, tq8AppRev1) {
		t.Fatalf("modified file's full content missing from the artifact:\n%s", in.Artifact)
	}
	if strings.Count(in.BriefText, "func Cart() {}") != 1 {
		t.Fatalf("the added file's content rides the wire %d times, want exactly once (its diff)", strings.Count(in.BriefText, "func Cart() {}"))
	}
	// The report is claims: labelled on the wire, carried on the input, and
	// absent from the quotable artifact and diff.
	if in.Report != tq8Report {
		t.Fatalf("JudgeInput.Report = %q, want the executor's report", in.Report)
	}
	if strings.Contains(in.Artifact, "wired the header") || strings.Contains(in.Diff, "wired the header") {
		t.Fatal("the executor's report leaked into the quotable artifact/diff (Spec S07.9 P-T06-3: claims are not the artifact)")
	}
	ids := tq8ManifestIDs(in)
	if !ids["verify/executor-report"] {
		t.Fatalf("no verify/executor-report item on the manifest; ids %v", ids)
	}
	if !strings.Contains(in.BriefText, "wired the header") {
		t.Fatal("the judge must still READ the executor's claims (as claims)")
	}
	if strings.Count(in.BriefText, "wired the header") != 1 {
		t.Fatalf("the report rides the wire %d times, want once", strings.Count(in.BriefText, "wired the header"))
	}
	// The seam was asked once for this revision with the judge's own bound,
	// never the §78 wire caps.
	if len(fc.calls) != 1 || fc.calls[0].rev != 1 || fc.calls[0].budget != verify.JudgeArtifactBytesCap {
		t.Fatalf("seam calls %+v, want one call for rev 1 at budget %d", fc.calls, verify.JudgeArtifactBytesCap)
	}
	// Cost shape unchanged: a bigger prompt is allowance, not a call.
	if j.complianceCalls != 1 || j.sanityCalls != 1 {
		t.Fatalf("judge calls compliance=%d sanity=%d, want 1/1 (Spec S07.11 ≤ rounds × 2)", j.complianceCalls, j.sanityCalls)
	}
	if out.Verdict != verify.VerdictShip {
		t.Fatalf("verdict %s (evidence quoted from the artifact must stand)", out.Verdict)
	}
	// Recorded: what the judge saw.
	saw := out.Rounds[0].JudgeSaw
	if saw == nil || saw.Kind != "tree" {
		t.Fatalf("round record JudgeSaw = %+v, want kind tree", saw)
	}
	if saw.Files != 4 || saw.DiffsShown != 3 || saw.ContentShown != 1 || saw.Truncated || !saw.OldIsBase || saw.NewPin != "s1" {
		t.Fatalf("JudgeSaw = %+v", saw)
	}

	t.Run("an empty change at revision 2 is said, never hidden behind the report", func(t *testing.T) {
		f := newFix(t)
		f.seedTask("t1", "r1")
		j := &fakeJudge{}
		v := f.verifier(j, &scriptRunner{}, passPack())
		v.Change = &tq8Change{rc: verify.RevisionChange{OldN: 1, NewN: 2, OldPin: "s1", NewPin: "s1", Files: []verify.ChangedFile{}}}
		d := tq8Deliverable("t1", "r1")
		d.Revision, d.PrevContent = 2, "# report rev 1\n"
		out, err := v.Verify(context.Background(), input(d))
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		in := j.inputs[0]
		if in.Artifact == tq8Report || strings.Contains(in.Artifact, "wired the header") {
			t.Fatal("an empty tree change fell back to the report as the artifact")
		}
		if in.Report != tq8Report {
			t.Fatal("the report must still ride as claims")
		}
		saw := out.Rounds[0].JudgeSaw
		if saw == nil || saw.Kind != "tree" || saw.Files != 0 || saw.OldN != 1 || saw.NewN != 2 {
			t.Fatalf("JudgeSaw = %+v, want an empty tree slice of rev 2 vs rev 1", saw)
		}
	})
}

// TestTQ8QuoteFromTheReportDoesNotSatisfyAPass — (3) axis-1 evidence is
// validated against the tree slice: a quote from an added file's diff stands,
// a quote that exists only in the executor's report is non-extractive and
// forces Unknown (Spec S07.5/S07.10).
func TestTQ8QuoteFromTheReportDoesNotSatisfyAPass(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{
		compliance: func(in verify.JudgeInput) (verify.Axis1Result, error) {
			return verify.Axis1Result{Verdicts: []verify.ACVerdict{
				{Key: "AC-1", Pass: true, Evidence: "wired the header"}, // report only
				{Key: "AC-2", Pass: true, Evidence: "func Cart() {}"},   // the added file's diff
			}}, nil
		},
	}
	v := f.verifier(j, &scriptRunner{}, tq8BuildOnlyPack())
	v.Change = &tq8Change{rc: tq8Tree()}

	out, err := v.Verify(context.Background(), input(tq8Deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	byKey := map[string]verify.ACVerdict{}
	for _, vd := range out.Rounds[0].Axis1 {
		byKey[vd.Key] = vd
	}
	ac1, ac2 := byKey["AC-1"], byKey["AC-2"]
	if !ac1.Unknown || ac1.Forced != "unknown: non-extractive evidence" {
		t.Fatalf("AC-1 quoted the executor's report and must be forced Unknown (the report is not the artifact): %+v", ac1)
	}
	if ac2.Unknown || !ac2.Pass || ac2.FromV1 {
		t.Fatalf("AC-2 quoted the added file's diff and must stand: %+v", ac2)
	}
}

// tq8WideTree is a change whose diff bodies exceed the judge's bound: six
// added 50 KiB files a01..a06 and one modified file m01.go (1 KiB diff,
// 10 KiB content). In path order the first three diffs fit under 192 KiB;
// a04's does not, so a04..a06 and m01's diffs are omitted (the cut is a
// contiguous path-order prefix, the §78 shape); m01's content then fits the
// remainder.
func tq8WideTree() (verify.RevisionChange, []string) {
	body := func(tag string, n int) string {
		line := tag + " " + strings.Repeat("x", 60) + "\n" // 64 bytes + tag
		var sb strings.Builder
		for sb.Len() < n {
			sb.WriteString(line)
		}
		return sb.String()[:n]
	}
	rc := verify.RevisionChange{OldN: 1, NewN: 2, OldPin: "s1", NewPin: "s2"}
	for i := 1; i <= 6; i++ {
		p := fmt.Sprintf("a%02d.txt", i)
		d := body("+"+p, 50<<10)
		rc.Files = append(rc.Files, verify.ChangedFile{Path: p, Kind: verify.KindAdded, NewSize: int64(len(d)), Diff: d})
	}
	rc.Files = append(rc.Files, verify.ChangedFile{
		Path: "m01.go", Kind: verify.KindModified, OldSize: 9 << 10, NewSize: 10 << 10,
		Diff: body("+m01", 1<<10), Content: body(" m01c", 10<<10),
	})
	return rc, []string{"a04.txt", "a05.txt", "a06.txt", "m01.go"}
}

// TestTQ8SliceIsBoundedAtAFileBoundaryAndSaysSo — (2) the bound: bodies stop
// at JudgeArtifactBytesCap on a FILE boundary; the inventory stays whole; the
// omitted files are NAMED on the wire; the round record and the verify.round
// row say what was shown and what was cut (S07.11) — never a silent cut.
func TestTQ8SliceIsBoundedAtAFileBoundaryAndSaysSo(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{}
	v := f.verifier(j, &scriptRunner{}, passPack())
	rc, omitted := tq8WideTree()
	v.Change = &tq8Change{rc: rc}
	d := tq8Deliverable("t1", "r1")
	d.Revision, d.PrevContent = 2, "# report rev 1\n"

	out, err := v.Verify(context.Background(), input(d))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	in := j.inputs[0]
	saw := out.Rounds[0].JudgeSaw
	if saw == nil {
		t.Fatal("JudgeSaw not recorded")
	}
	// Whole files or nothing: the first three diffs verbatim, the rest absent.
	for i, row := range rc.Files {
		shown := strings.Contains(in.Diff, row.Diff)
		if i < 3 && !shown {
			t.Fatalf("%s fits under the bound and must be shown whole", row.Path)
		}
		if i >= 3 && shown {
			t.Fatalf("%s is past the cut and must not be shown", row.Path)
		}
		if i >= 3 && strings.Contains(in.Diff, row.Diff[:512]) {
			t.Fatalf("%s was cut mid-file (a partial diff is an artefact, §78)", row.Path)
		}
	}
	if saw.DiffBytes+saw.ContentBytes > verify.JudgeArtifactBytesCap {
		t.Fatalf("bodies %d+%d exceed the bound %d", saw.DiffBytes, saw.ContentBytes, verify.JudgeArtifactBytesCap)
	}
	if saw.DiffBytes != 3*(50<<10) {
		t.Fatalf("diff bytes %d, want exactly the three whole diffs %d", saw.DiffBytes, 3*(50<<10))
	}
	// The inventory is whole and the omitted files are named a second time,
	// on the omitted line, so the judge KNOWS what it did not see.
	for _, p := range omitted {
		if n := strings.Count(in.Artifact, p); n < 2 {
			t.Fatalf("%q appears %d times in the artifact, want ≥ 2 (inventory row + named as omitted):\n%s", p, n, in.Artifact)
		}
	}
	if !strings.Contains(in.Artifact, rc.Files[6].Content) {
		t.Fatal("m01.go's content fits the remaining budget and must be shown")
	}
	if !saw.Truncated || saw.Files != 7 || saw.DiffsShown != 3 || saw.ContentShown != 1 || len(saw.ContentOmitted) != 0 {
		t.Fatalf("JudgeSaw = %+v", saw)
	}
	if strings.Join(saw.DiffsOmitted, ",") != strings.Join(omitted, ",") {
		t.Fatalf("DiffsOmitted %v, want %v", saw.DiffsOmitted, omitted)
	}
	if saw.ArtifactBytes != len(in.Artifact) || saw.ReportBytes != len(tq8Report) {
		t.Fatalf("recorded sizes %+v vs artifact %d / report %d", saw, len(in.Artifact), len(tq8Report))
	}
	// The verify.round row carries the same record (S07.11 keep-forever).
	rows := f.events("verify.round")
	if len(rows) != 1 {
		t.Fatalf("verify.round rows %d, want 1", len(rows))
	}
	var payload struct {
		JudgeSaw *verify.JudgeSaw `json:"judge_saw"`
	}
	if err := json.Unmarshal(rows[0].Payload, &payload); err != nil {
		t.Fatalf("decode verify.round: %v", err)
	}
	if payload.JudgeSaw == nil || !payload.JudgeSaw.Truncated || payload.JudgeSaw.Kind != "tree" ||
		strings.Join(payload.JudgeSaw.DiffsOmitted, ",") != strings.Join(omitted, ",") {
		t.Fatalf("verify.round judge_saw = %+v", payload.JudgeSaw)
	}
}

// TestTQ8ContentPinnedSliceIsByteIdenticalAndRecorded — (5)/(7b) a
// content-pinned deliverable (no snapshot pin: the golden-set shape, every
// non-project task, composer definitions) keeps today's slice byte for byte
// on the wire, with or without the seam wired; the record says so.
func TestTQ8ContentPinnedSliceIsByteIdenticalAndRecorded(t *testing.T) {
	pack := passPack()
	drive := func(t *testing.T, taskID, runID string, wire bool) (verify.JudgeInput, verify.Outcome) {
		t.Helper()
		f := newFix(t)
		f.seedTask(taskID, runID)
		j := &fakeJudge{}
		v := f.verifier(j, &scriptRunner{}, pack)
		if wire {
			v.Change = &tq8Change{rc: verify.RevisionChange{AbsentReason: "version 1 is not stored as a snapshot of the project's files"}}
		}
		out, err := v.Verify(context.Background(), input(deliverable(taskID, runID)))
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		return j.inputs[0], out
	}
	bare, _ := drive(t, "t1", "r1", false)
	wired, out := drive(t, "t1", "r1", true)

	d := deliverable("t1", "r1")
	for name, in := range map[string]verify.JudgeInput{"bare": bare, "wired": wired} {
		if in.Artifact != d.Content || in.Diff != "" || in.Report != "" {
			t.Fatalf("%s: content-pinned slice changed: artifact=%q diff=%q report=%q", name, in.Artifact, in.Diff, in.Report)
		}
		ids := tq8ManifestIDs(in)
		if ids["verify/executor-report"] {
			t.Fatalf("%s: a content-pinned deliverable has no separate report item", name)
		}
	}
	if bare.BriefText != wired.BriefText {
		t.Fatalf("the wire differs with the seam wired:\n--- bare ---\n%s\n--- wired ---\n%s", bare.BriefText, wired.BriefText)
	}
	saw := out.Rounds[0].JudgeSaw
	if saw == nil || saw.Kind != "content" || saw.AbsentReason == "" || saw.ArtifactBytes != len(d.Content) {
		t.Fatalf("JudgeSaw = %+v, want kind content with the seam's absent reason recorded", saw)
	}
}

// TestTQ8ALostPinFailsTheRoundLoudly — (4) the diff comes from the platform
// store at the pinned refs; a pin the store no longer holds is content drift
// and the round fails BEFORE any paid judge call — never a fall-through to
// the report (§78 F1).
func TestTQ8ALostPinFailsTheRoundLoudly(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{}
	v := f.verifier(j, &scriptRunner{}, passPack())
	drift := errors.New("content drift: the project store no longer holds the saved files that dlv-t1 pins at s1")
	v.Change = &tq8Change{err: drift}

	_, err := v.Verify(context.Background(), input(tq8Deliverable("t1", "r1")))
	if !errors.Is(err, drift) {
		t.Fatalf("Verify err = %v, want the seam's drift error surfaced", err)
	}
	if j.complianceCalls != 0 || j.sanityCalls != 0 {
		t.Fatalf("judge called %d/%d times on a lost pin — the paid call must not happen", j.complianceCalls, j.sanityCalls)
	}
	if n := len(f.events("verify.round")); n != 0 {
		t.Fatalf("%d verify.round rows written for a round that did not judge", n)
	}
}

// TestTQ8PropSliceInvariants — (7) for any revision change the rendered
// slice holds every inventory row; every diffable file is either shown whole
// or named as omitted (never cut); the shown diffs are a contiguous
// path-order prefix; bodies never exceed the bound; binary bytes never
// appear; the inventory count is recorded.
func TestTQ8PropSliceInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	kinds := []string{verify.KindAdded, verify.KindModified, verify.KindDeleted, verify.KindRenamed}
	blob := func(tag string, n int) string {
		var sb strings.Builder
		for i := 0; sb.Len() < n; i++ {
			fmt.Fprintf(&sb, "%s line %d %s\n", tag, i, strings.Repeat("y", rng.Intn(40)))
		}
		return sb.String()
	}
	for iter := 0; iter < 150; iter++ {
		n := 1 + rng.Intn(12)
		rc := verify.RevisionChange{OldN: rng.Intn(3), NewN: 1 + rng.Intn(3), OldPin: "o", NewPin: "n"}
		rc.NewN = rc.OldN + 1
		rc.OldIsBase = rc.OldN == 0
		for i := 0; i < n; i++ {
			p := fmt.Sprintf("f%02d/%c.go", i, 'a'+rune(rng.Intn(26)))
			row := verify.ChangedFile{Path: p, Kind: kinds[rng.Intn(len(kinds))]}
			if row.Kind == verify.KindRenamed {
				row.OldPath = "old/" + p
			}
			if rng.Intn(6) == 0 {
				row.Binary = true
				rc.Files = append(rc.Files, row)
				continue
			}
			size := rng.Intn(8 << 10)
			if rng.Intn(4) == 0 {
				size = (100 << 10) + rng.Intn(120<<10) // some over half the bound
			}
			row.Diff = "diff --git a/" + p + " b/" + p + "\n" + blob("+"+p, size)
			if row.Kind == verify.KindModified || row.Kind == verify.KindRenamed {
				row.Content = blob(" "+p, rng.Intn(20<<10))
			}
			rc.Files = append(rc.Files, row)
		}

		artifact, diff, saw := verify.RenderChangeSlice(rc)

		if saw.Files != len(rc.Files) {
			t.Fatalf("iter %d: JudgeSaw.Files %d, want %d", iter, saw.Files, len(rc.Files))
		}
		omitted := map[string]bool{}
		for _, p := range saw.DiffsOmitted {
			omitted[p] = true
		}
		contentOmitted := map[string]bool{}
		for _, p := range saw.ContentOmitted {
			contentOmitted[p] = true
		}
		var shown, diffBytes, contentBytes int
		seenOmitted := false
		for _, row := range rc.Files {
			if !strings.Contains(artifact, row.Path) {
				t.Fatalf("iter %d: inventory row %q missing from the artifact", iter, row.Path)
			}
			if row.Binary {
				if omitted[row.Path] {
					t.Fatalf("iter %d: binary %q named among omitted diffs (it has none)", iter, row.Path)
				}
				continue
			}
			whole := strings.Contains(diff, row.Diff)
			switch {
			case whole && omitted[row.Path]:
				t.Fatalf("iter %d: %q shown AND named omitted", iter, row.Path)
			case whole:
				if seenOmitted {
					t.Fatalf("iter %d: %q shown after an omitted file — the cut must be a contiguous path-order prefix", iter, row.Path)
				}
				shown++
				diffBytes += len(row.Diff)
			case omitted[row.Path]:
				seenOmitted = true
				if strings.Count(artifact, row.Path) < 2 {
					t.Fatalf("iter %d: omitted %q not NAMED on the wire beside its inventory row", iter, row.Path)
				}
				if len(row.Diff) > 256 && strings.Contains(diff, row.Diff[:256]) {
					t.Fatalf("iter %d: %q cut mid-file", iter, row.Path)
				}
			default:
				t.Fatalf("iter %d: %q neither shown whole nor named omitted", iter, row.Path)
			}
			if row.Content != "" {
				switch {
				case strings.Contains(artifact, row.Content):
					contentBytes += len(row.Content)
					if contentOmitted[row.Path] {
						t.Fatalf("iter %d: content of %q shown AND named omitted", iter, row.Path)
					}
				case contentOmitted[row.Path]:
				default:
					t.Fatalf("iter %d: content of %q neither shown nor named omitted", iter, row.Path)
				}
			}
		}
		if shown != saw.DiffsShown || diffBytes != saw.DiffBytes || contentBytes != saw.ContentBytes {
			t.Fatalf("iter %d: recorded shown=%d diff=%d content=%d, observed %d/%d/%d", iter, saw.DiffsShown, saw.DiffBytes, saw.ContentBytes, shown, diffBytes, contentBytes)
		}
		if saw.DiffBytes+saw.ContentBytes > verify.JudgeArtifactBytesCap {
			t.Fatalf("iter %d: bodies %d exceed the bound", iter, saw.DiffBytes+saw.ContentBytes)
		}
		if saw.Truncated != (len(saw.DiffsOmitted)+len(saw.ContentOmitted) > 0) {
			t.Fatalf("iter %d: Truncated=%v with %d/%d omitted", iter, saw.Truncated, len(saw.DiffsOmitted), len(saw.ContentOmitted))
		}
		if saw.ArtifactBytes != len(artifact) || saw.Kind != "tree" {
			t.Fatalf("iter %d: JudgeSaw %+v vs artifact %d bytes", iter, saw, len(artifact))
		}
	}
}

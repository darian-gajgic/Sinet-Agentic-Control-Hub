package review_test

// tree_drain_test.go — P3-SIT-1 drain r1 at the data layer.
//
// F3  a model-emitted anchor path never reaches exec; it degrades down the
//     S13.3 ladder instead of failing the findings record.
// F5  each of the three caps is exercised EXACTLY at its boundary, and the
//     hunk-boundary cut is driven with a diff that has hunks to cut between.
// F6  a repo-backed revision whose tree source is not composed answers the
//     honest absence, not a 400 blaming the caller.
// F7  that stated absence survives a type whose own lane sets a label.
// F8  a file too large to read in full is never diffed from two prefixes: the
//     answer is an inventory row and a reason, never a diff that says nothing
//     changed about a file that changed.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// drainWorld seeds a repo-backed deliverable over one base tree and one
// revision tree, with the tree source wired.
func drainWorld(t *testing.T, dtype string, base, rev1 map[string]string) *fix {
	t.Helper()
	f := newFix(t)
	ctx := context.Background()
	f.task("t1", "u1")
	f.run("r1", "t1")
	if _, err := f.store.EnsureDeliverable(ctx, review.EnsureInput{
		ID: "dlv-t1", Owner: "u1", TaskID: "t1", Type: dtype,
	}); err != nil {
		t.Fatalf("EnsureDeliverable: %v", err)
	}
	sit1Mint(t, f, "dlv-t1", 1, "s1")
	f.store.Tree = sit1Trees{base: "s0", trees: map[string]map[string]string{"s0": base, "s1": rev1}}
	return f
}

// padToExactly grows the last line of body with spaces until the served diff of
// (absent → body) is EXACTLY want bytes. Each space is one more byte on the
// one "+" line it sits in, so the search is a single measurement plus one
// adjustment; anything else would mean the diff is not a function of the
// content and the test says so rather than settling for "close enough".
func padToExactly(t *testing.T, name string, want int, lines int) string {
	t.Helper()
	body := strings.Repeat("the quick brown fox jumps over it\n", lines)
	// The measurement uses the REAL file name, because the path is interpolated
	// into the diff's four header lines: sizing against another name lands short
	// by four times the difference.
	measure := func(b string) int {
		f := drainWorld(t, "code", map[string]string{}, map[string]string{name: b})
		cmp, err := f.store.CompareFile(context.Background(), "dlv-t1", 0, 1, name)
		if err != nil {
			t.Fatalf("CompareFile while sizing: %v", err)
		}
		return len(cmp.Unified)
	}
	got := measure(body)
	if got > want {
		t.Fatalf("the %d-line body already diffs to %d bytes, past the %d target — use fewer lines", lines, got, want)
	}
	// The body ends with a newline, so the padding goes on a final line of its
	// own: one "+" and one "\n" of overhead plus the spaces themselves.
	pad := want - got - 2
	if pad < 0 {
		t.Fatalf("cannot pad from %d to %d: the per-line overhead overshoots", got, want)
	}
	body += strings.Repeat(" ", pad) + "\n"
	if final := measure(body); final != want {
		t.Fatalf("padding landed on %d bytes, want exactly %d", final, want)
	}
	return body
}

// TestDrainContentExactlyAtTheCapIsNotTruncated (F5): a file of exactly
// TreeFileBytesCap is served whole. The cap is a limit, not a trigger, and an
// off-by-one here would mark a file truncated while serving all of it.
func TestDrainContentExactlyAtTheCapIsNotTruncated(t *testing.T) {
	ctx := context.Background()
	// 32-byte lines divide 1 MiB exactly, so the body is the cap to the byte.
	line := strings.Repeat("x", 31) + "\n"
	body := strings.Repeat(line, review.TreeFileBytesCap/len(line))
	if len(body) != review.TreeFileBytesCap {
		t.Fatalf("fixture is %d bytes, want exactly the %d cap", len(body), review.TreeFileBytesCap)
	}
	f := drainWorld(t, "code", map[string]string{}, map[string]string{"exact.txt": body, "over.txt": body + line})

	at, err := f.store.RevisionFile(ctx, "dlv-t1", 1, "exact.txt")
	if err != nil {
		t.Fatalf("RevisionFile: %v", err)
	}
	if at.Truncated || at.TruncationReason != "" || at.Content != body || at.Size != int64(len(body)) {
		t.Fatalf("a file of exactly the cap was cut: truncated=%v reason=%q served %d of %d",
			at.Truncated, at.TruncationReason, len(at.Content), len(body))
	}
	// One byte past it IS truncated — the boundary is real in both directions.
	over, err := f.store.RevisionFile(ctx, "dlv-t1", 1, "over.txt")
	if err != nil {
		t.Fatalf("RevisionFile over: %v", err)
	}
	if !over.Truncated || over.TruncationReason == "" || len(over.Content) > review.TreeFileBytesCap {
		t.Fatalf("a file one line past the cap was not marked truncated: %+v", over.Truncated)
	}
}

// TestDrainFileDiffExactlyAtTheCapIsNotTruncated (F5): a per-file diff of
// exactly TreeFileDiffBytesCap is served whole.
func TestDrainFileDiffExactlyAtTheCapIsNotTruncated(t *testing.T) {
	ctx := context.Background()
	body := padToExactly(t, "big.txt", review.TreeFileDiffBytesCap, 7000)
	f := drainWorld(t, "code", map[string]string{}, map[string]string{"big.txt": body})
	cmp, err := f.store.CompareFile(ctx, "dlv-t1", 0, 1, "big.txt")
	if err != nil {
		t.Fatalf("CompareFile: %v", err)
	}
	if len(cmp.Unified) != review.TreeFileDiffBytesCap {
		t.Fatalf("the sized diff is %d bytes, want exactly %d", len(cmp.Unified), review.TreeFileDiffBytesCap)
	}
	if cmp.Truncated || cmp.TruncationReason != "" {
		t.Fatalf("a diff of exactly the cap was marked truncated: %q", cmp.TruncationReason)
	}
}

// TestDrainChangeDiffExactlyAtTheCapIsNotTruncated (F5): the whole-change text
// at exactly TreeDiffBytesCap holds every file and is not marked truncated.
func TestDrainChangeDiffExactlyAtTheCapIsNotTruncated(t *testing.T) {
	ctx := context.Background()
	half := review.TreeDiffBytesCap / 2
	a := padToExactly(t, "a.txt", half, 7000)
	b := padToExactly(t, "b.txt", review.TreeDiffBytesCap-half, 7000)
	f := drainWorld(t, "code", map[string]string{}, map[string]string{"a.txt": a, "b.txt": b})
	cmp, err := f.store.Compare(ctx, "dlv-t1", 0, 1)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if len(cmp.Unified) != review.TreeDiffBytesCap {
		t.Fatalf("the two sized diffs total %d bytes, want exactly %d", len(cmp.Unified), review.TreeDiffBytesCap)
	}
	if cmp.Truncated || cmp.TruncationReason != "" {
		t.Fatalf("a whole change of exactly the cap was marked truncated: %q", cmp.TruncationReason)
	}
	if cmp.Change == nil || len(cmp.Change.Files) != 2 {
		t.Fatalf("both files must be in the inventory: %+v", cmp.Change)
	}
}

// hunkSpans parses a unified diff's hunk headers into (oldLines, newLines) and
// counts the body lines that follow each, so a served text can be checked for
// WHOLE hunks rather than merely for a plausible ending.
func hunkSpans(t *testing.T, unified string) (complete int, incomplete int) {
	t.Helper()
	lines := strings.Split(unified, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var wantOld, wantNew, gotOld, gotNew int
	open := false
	flush := func() {
		if !open {
			return
		}
		if gotOld == wantOld && gotNew == wantNew {
			complete++
		} else {
			incomplete++
		}
		open = false
	}
	for _, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "@@ "):
			flush()
			f := strings.Fields(ln)
			wantOld = spanCount(t, strings.TrimPrefix(f[1], "-"))
			wantNew = spanCount(t, strings.TrimPrefix(f[2], "+"))
			gotOld, gotNew, open = 0, 0, true
		case !open:
			// File headers before the first hunk.
		case strings.HasPrefix(ln, "--- "), strings.HasPrefix(ln, "+++ "),
			strings.HasPrefix(ln, "diff --git "), strings.HasPrefix(ln, "index "):
			flush() // the next file's headers end this one's body
		case strings.HasPrefix(ln, " "):
			gotOld, gotNew = gotOld+1, gotNew+1
		case strings.HasPrefix(ln, "-"):
			gotOld++
		case strings.HasPrefix(ln, "+"):
			gotNew++
		}
	}
	flush()
	return complete, incomplete
}

func spanCount(t *testing.T, s string) int {
	t.Helper()
	if i := strings.IndexByte(s, ','); i >= 0 {
		n, err := strconv.Atoi(s[i+1:])
		if err != nil {
			t.Fatalf("hunk span %q: %v", s, err)
		}
		return n
	}
	return 1
}

// TestDrainOverCapDiffIsCutAtAHunkBoundary (F5): a many-hunk diff past the cap
// is cut BETWEEN hunks, so every hunk served is a whole hunk. A cut at an
// arbitrary line boundary leaves the last hunk promising body lines that are
// not there, which is a body no diff parser can trust.
func TestDrainOverCapDiffIsCutAtAHunkBoundary(t *testing.T) {
	ctx := context.Background()
	// Edits far enough apart to land in separate hunks under --unified=3, and
	// enough of them that the whole diff is well past the per-file cap.
	var before, after strings.Builder
	// 20 lines per block with the edit in the middle leaves 13 unchanged lines
	// between one hunk's last context line and the next hunk's first — more than
	// the 2x3 git merges across, so these are SEPARATE hunks. At 9 lines apart
	// they merged into one, and a single hunk has no boundary to cut at.
	for block := 0; block < 1200; block++ {
		for line := 0; line < 20; line++ {
			fmt.Fprintf(&before, "block %05d line %02d of the file\n", block, line)
			if line == 10 {
				fmt.Fprintf(&after, "block %05d line %02d CHANGED here\n", block, line)
				continue
			}
			fmt.Fprintf(&after, "block %05d line %02d of the file\n", block, line)
		}
	}
	f := drainWorld(t, "code",
		map[string]string{"many.txt": before.String()},
		map[string]string{"many.txt": after.String()})
	// Revision 1's old side is the base tree, which holds the "before" body.
	cmp, err := f.store.CompareFile(ctx, "dlv-t1", 0, 1, "many.txt")
	if err != nil {
		t.Fatalf("CompareFile: %v", err)
	}
	if !cmp.Truncated || cmp.TruncationReason == "" {
		t.Fatalf("a diff past the cap was not marked truncated: %d bytes", len(cmp.Unified))
	}
	if len(cmp.Unified) > review.TreeFileDiffBytesCap {
		t.Fatalf("the cut diff is %d bytes, past the %d cap", len(cmp.Unified), review.TreeFileDiffBytesCap)
	}
	complete, incomplete := hunkSpans(t, cmp.Unified)
	if complete < 2 {
		t.Fatalf("the fixture produced %d whole hunks — with fewer than two there is no boundary to cut at", complete)
	}
	if incomplete != 0 {
		t.Fatalf("the diff was cut inside a hunk: %d whole, %d partial", complete, incomplete)
	}
	if !strings.Contains(cmp.TruncationReason, "hunk") {
		t.Errorf("the reason should say where the text stops: %q", cmp.TruncationReason)
	}
}

// TestDrainTooLargeToDiffServesNoDiffRatherThanAnArtefact (F8): when a file
// cannot be read in full, the answer is its inventory row and a reason — never
// a diff computed from two prefixes.
//
// The fixture is the exact trap: two files that are IDENTICAL for the first
// megabyte and differ only after it. Diffing the prefixes yields an empty diff,
// so the old behaviour would have served "nothing changed" about a file that
// changed — a lie a truncation flag does not redeem.
func TestDrainTooLargeToDiffServesNoDiffRatherThanAnArtefact(t *testing.T) {
	ctx := context.Background()
	common := strings.Repeat("shared line that is the same on both sides\n", 30000)
	if len(common) <= review.TreeFileBytesCap {
		t.Fatalf("the shared prefix is %d bytes — it must exceed the %d read cap to set the trap", len(common), review.TreeFileBytesCap)
	}
	f := drainWorld(t, "code",
		map[string]string{"huge.txt": common + "OLD TAIL\n"},
		map[string]string{"huge.txt": common + "NEW TAIL\n"})

	cmp, err := f.store.CompareFile(ctx, "dlv-t1", 0, 1, "huge.txt")
	if err != nil {
		t.Fatalf("CompareFile: %v", err)
	}
	if cmp.Unified != "" {
		t.Fatalf("a diff was computed from two truncated prefixes:\n%.300s", cmp.Unified)
	}
	if !cmp.Truncated || cmp.TruncationReason == "" {
		t.Fatal("a file too large to compare must SAY so rather than silently serving nothing")
	}
	if !strings.Contains(cmp.TruncationReason, "huge.txt") {
		t.Errorf("the reason should name the file: %q", cmp.TruncationReason)
	}
	// The inventory still carries the row: the change is visible even when its
	// text is not.
	if cmp.Change == nil || len(cmp.Change.Files) != 1 || cmp.Change.Files[0].Kind != review.KindModified {
		t.Fatalf("the inventory must still report the modification: %+v", cmp.Change)
	}
	// And the whole-change lane stops honestly rather than serving the artefact.
	whole, err := f.store.Compare(ctx, "dlv-t1", 0, 1)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if !whole.Truncated || whole.TruncationReason == "" {
		t.Fatal("the whole-change text must say it holds nothing of this change")
	}
}

// TestDrainNilTreeSourceIsAnAbsenceNotABadRequest (F6): narrowing a comparison
// on a repo-backed revision whose tree source is not composed is the PLATFORM's
// absence. Answering 4xx told a requester to fix a request that was correct.
func TestDrainNilTreeSourceIsAnAbsenceNotABadRequest(t *testing.T) {
	ctx := context.Background()
	f := drainWorld(t, "code", map[string]string{}, map[string]string{"src/app.go": "package app\n"})
	f.store.Tree = nil

	cmp, err := f.store.CompareFile(ctx, "dlv-t1", 0, 1, "src/app.go")
	if err != nil {
		t.Fatalf("CompareFile with no tree source: %v — an absence is an answer, not a refusal", err)
	}
	if cmp.Change == nil || cmp.Change.AbsentReason == "" {
		t.Fatalf("the absence must be stated the way the detail states it: %+v", cmp.Change)
	}
	if cmp.Unified != "" {
		t.Fatalf("no tree source means no diff text: %q", cmp.Unified)
	}
	// A CONTENT-pinned revision is a different fact: there is no file tree to
	// narrow, and that IS the caller's to fix.
	g := newFix(t)
	g.seed()
	g.mint(1, "one\n")
	g.mint(2, "one\ntwo\n")
	if _, err := g.store.CompareFile(ctx, "dlv-t1", 1, 2, "deliverable.md"); err == nil {
		t.Fatal("narrowing a content-pinned comparison must be refused as a bad request")
	}
}

// TestDrainStatedAbsenceSurvivesATypesOwnLabel (F7): the per-type arms set
// labels of their own, and a statement about what is MISSING must not be the
// thing that goes missing.
func TestDrainStatedAbsenceSurvivesATypesOwnLabel(t *testing.T) {
	ctx := context.Background()
	for _, dtype := range []string{"notebook", "spreadsheet", "widget"} {
		f := drainWorld(t, dtype, map[string]string{}, map[string]string{"x": "y\n"})
		f.store.Tree = nil
		cmp, err := f.store.Compare(ctx, "dlv-t1", 0, 1)
		if err != nil {
			t.Fatalf("%s: Compare: %v", dtype, err)
		}
		if !strings.Contains(cmp.Label, "file store is not available") {
			t.Errorf("%s: the stated absence was overwritten by the type's own label: %q", dtype, cmp.Label)
		}
	}
}

// TestDrainUnsafeAnchorPathNeverReachesExec (F3): a finding's anchor is MODEL
// output, and one carrying a NUL failed the whole findings record at fork/exec.
// The point still lands; only its position degrades (P-T12-2: delivery is never
// conditional on anchoring).
func TestDrainUnsafeAnchorPathNeverReachesExec(t *testing.T) {
	ctx := context.Background()
	f := drainWorld(t, "code", map[string]string{}, map[string]string{"src/app.go": "package app\n\nfunc Run() {}\n"})

	unsafe := []string{
		"src/app.go\x00:3",
		"src/app.go \x1b:3",
		"/etc/passwd:1",
		"../../etc/passwd:1",
		"  src/app.go  :3",
	}
	in := make([]review.FindingInput, 0, len(unsafe)+1)
	for i, raw := range unsafe {
		in = append(in, review.FindingInput{
			Author: "u1", RunID: "r1", Severity: review.SeverityNote,
			Body: fmt.Sprintf("point %d", i), RawAnchor: raw,
		})
	}
	// The non-tautological control: a GOOD anchor in the same batch still
	// resolves exactly, so the batch is not passing because nothing anchors.
	in = append(in, review.FindingInput{
		Author: "u1", RunID: "r1", Severity: review.SeverityNote,
		Body: "the good one", RawAnchor: "src/app.go:3",
	})

	ids, err := f.store.AddFindings(ctx, "dlv-t1", 1, in)
	if err != nil {
		t.Fatalf("AddFindings must never fail on a malformed anchor: %v", err)
	}
	if len(ids) != len(in) {
		t.Fatalf("recorded %d of %d findings — a malformed anchor dropped a point", len(ids), len(in))
	}
	for i := range unsafe {
		c, err := f.store.CommentByID(ctx, ids[i])
		if err != nil {
			t.Fatalf("CommentByID: %v", err)
		}
		if c.BornStatus != review.AnchorFile {
			t.Errorf("finding %d with anchor %q was born %q, want the file-level degrade", i, unsafe[i], c.BornStatus)
		}
	}
	good, err := f.store.CommentByID(ctx, ids[len(ids)-1])
	if err != nil {
		t.Fatalf("CommentByID: %v", err)
	}
	if good.BornStatus != review.AnchorExact || good.Anchor.LineNo != 3 {
		t.Fatalf("the control anchor did not resolve, so the degrades above prove nothing: %q %+v", good.BornStatus, good.Anchor)
	}

	// The same guard on the human ingress: a comment whose claimed path cannot
	// be a path degrades rather than reaching exec. (The transport refuses it
	// outright with a 400 — this is the backstop underneath that.)
	c, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", RevisionN: 1, Author: "u1", Body: "on a path that cannot be one",
		Anchor: &review.AnchorRecord{FilePath: "src/app.go\x00", Side: review.SideNew, LineNo: 1, LineText: "package app"},
	})
	if err != nil {
		t.Fatalf("AddComment must not fail at exec on a malformed path: %v", err)
	}
	if c.BornStatus != review.AnchorFile {
		t.Errorf("a comment on an impossible path was born %q, want the file-level degrade", c.BornStatus)
	}
}

// sitDriftTrees is a tree source whose pins are all gone: it reports the lost
// pin the way internal/project does, through the structural contract.
type sitDriftTrees struct{}

type sitLostPin struct{}

func (sitLostPin) Error() string        { return "fixture: the store holds no such commit" }
func (sitLostPin) TreePinMissing() bool { return true }
func (sitDriftTrees) TreeBase(context.Context, string) (string, bool, error) {
	return "s0", true, nil
}

func (sitDriftTrees) TreeChanges(context.Context, string, string, string) ([]review.ChangedFile, error) {
	return nil, sitLostPin{}
}

func (sitDriftTrees) TreeBlob(context.Context, string, string, string, int64) ([]byte, int64, bool, error) {
	return nil, 0, false, sitLostPin{}
}

// TestDrainLostPinIsContentDrift (F1, data layer): a seam reporting a lost pin
// becomes ErrContentDrift — the platform's own sentence naming the pin — on
// every read, and the file read does NOT fall through to the round report.
func TestDrainLostPinIsContentDrift(t *testing.T) {
	ctx := context.Background()
	f := drainWorld(t, "code", map[string]string{}, map[string]string{"src/app.go": "package app\n"})
	f.store.Tree = sitDriftTrees{}

	if _, err := f.store.Change(ctx, "dlv-t1", 0, 1); !isDrift(err) {
		t.Errorf("Change on a lost pin: %v, want ErrContentDrift", err)
	}
	if _, err := f.store.Compare(ctx, "dlv-t1", 0, 1); !isDrift(err) {
		t.Errorf("Compare on a lost pin: %v, want ErrContentDrift", err)
	}
	if _, err := f.store.CompareFile(ctx, "dlv-t1", 0, 1, "src/app.go"); !isDrift(err) {
		t.Errorf("CompareFile on a lost pin: %v, want ErrContentDrift", err)
	}
	_, err := f.store.RevisionFile(ctx, "dlv-t1", 1, "src/app.go")
	if !isDrift(err) {
		t.Errorf("RevisionFile on a lost pin: %v, want ErrContentDrift (never the companion report)", err)
	}
	// THE regression: the companion object must not stand in for a code file.
	if fc, ferr := f.store.RevisionFile(ctx, "dlv-t1", 1, "deliverable.md"); ferr == nil && fc.Content != "" {
		t.Error("the round report was served while the revision's own files are lost")
	}
	// The wire sentence is the platform's, not git's.
	if err != nil && strings.Contains(err.Error(), "fixture:") {
		t.Errorf("the seam's own error text reached the wire: %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "s1") {
		t.Errorf("the drift error should name the lost pin: %v", err)
	}
}

func isDrift(err error) bool {
	return errors.Is(err, review.ErrContentDrift)
}

package review_test

// sit1_tree_test.go — P3-SIT-1 R2–R7 at the data layer, committed RED by
// grounding (Amendment-A carve-out, CONVENTIONS §3).
//
// A repo-backed revision IS the tree at its snapshot pin (Spec S13.1); the
// reviewable change is computed host-side by `git diff` between revision pins
// (Spec S13.2); revision 1 presents against the pre-task base and any pair is
// diffable on demand (Spec S13.1). The platform store is reached through the
// TreeSource seam — faked here over in-memory trees, exactly as fakeBase fakes
// the base-content seam — so what is under test is the review store's own
// lane selection, inventory, bounding, file serving and anchoring, over the
// real object dir and the real gitDiff.

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// sit1Trees is the fake TreeSource: sha → path → content, plus the base pin.
type sit1Trees struct {
	base  string
	trees map[string]map[string]string
}

func (f sit1Trees) TreeBase(context.Context, string) (string, bool, error) {
	return f.base, f.base != "", nil
}

func (f sit1Trees) TreeChanges(_ context.Context, _, oldSHA, newSHA string) ([]review.ChangedFile, error) {
	o, ok := f.trees[oldSHA]
	n, ok2 := f.trees[newSHA]
	if !ok || !ok2 {
		return nil, fmt.Errorf("sit1Trees: unknown pin %q/%q", oldSHA, newSHA)
	}
	paths := map[string]bool{}
	for p := range o {
		paths[p] = true
	}
	for p := range n {
		paths[p] = true
	}
	var out []review.ChangedFile
	for p := range paths {
		oc, inOld := o[p]
		nc, inNew := n[p]
		row := review.ChangedFile{Path: p, OldSize: int64(len(oc)), NewSize: int64(len(nc)),
			Binary: strings.ContainsRune(oc, 0) || strings.ContainsRune(nc, 0)}
		switch {
		case inOld && inNew && oc == nc:
			continue
		case inOld && inNew:
			row.Kind = review.KindModified
		case inNew:
			row.Kind = review.KindAdded
		default:
			row.Kind = review.KindDeleted
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (f sit1Trees) TreeBlob(_ context.Context, _, sha, path string, limit int64) ([]byte, int64, bool, error) {
	t, ok := f.trees[sha]
	if !ok {
		return nil, 0, false, fmt.Errorf("sit1Trees: unknown pin %q", sha)
	}
	c, ok := t[path]
	if !ok {
		return nil, 0, false, nil
	}
	data := []byte(c)
	size := int64(len(data))
	if limit > 0 && size > limit {
		data = data[:limit]
	}
	return data, size, true, nil
}

const (
	sit1AppBase = "package app\n\nfunc Run() {}\n"
	sit1AppRev1 = "package app\n\nfunc Run() {\n\tserve()\n}\n"
	sit1AppRev2 = "package app\n\nfunc Run() {\n\tserve()\n\tlisten()\n}\n"
	sit1AppRev3 = "// header\npackage app\n\nfunc Run() {\n\tserve()\n\tlisten()\n}\n"
	sit1Cart    = "package app\n\nfunc Cart() {}\n"
	sit1Readme  = "# shop\n"
	sit1PNG     = "\x89PNG\r\n\x1a\n\x00\x00IHDR"
)

// sit1World seeds a repo-backed deliverable dlv-t1 (dtype code, three
// revisions, each carrying the companion report and a snapshot pin) over the
// trees s0 (base) → s1 → s2 → s3.
func sit1World(t *testing.T) (*fix, sit1Trees) {
	t.Helper()
	f := newFix(t)
	f.task("t1", "u1")
	f.run("r1", "t1")
	if _, err := f.store.EnsureDeliverable(context.Background(), review.EnsureInput{
		ID: "dlv-t1", Owner: "u1", TaskID: "t1", Type: "code",
	}); err != nil {
		t.Fatalf("EnsureDeliverable: %v", err)
	}
	trees := sit1Trees{base: "s0", trees: map[string]map[string]string{
		"s0": {"src/app.go": sit1AppBase, "README.md": sit1Readme},
		"s1": {"src/app.go": sit1AppRev1, "README.md": sit1Readme, "src/cart.go": sit1Cart},
		"s2": {"src/app.go": sit1AppRev2, "src/cart.go": sit1Cart, "assets/logo.png": sit1PNG},
		"s3": {"src/app.go": sit1AppRev3, "src/cart.go": sit1Cart, "assets/logo.png": sit1PNG},
	}}
	for n, sha := range []string{"s1", "s2", "s3"} {
		sit1Mint(t, f, "dlv-t1", n+1, sha)
	}
	f.store.Tree = trees
	return f, trees
}

func sit1Mint(t *testing.T, f *fix, id string, n int, sha string) {
	t.Helper()
	if _, err := f.store.MintRevision(context.Background(), review.MintInput{
		DeliverableID: id, N: n, RunID: "r1", AttemptRef: fmt.Sprintf("r1#round-%d", n),
		Files:       map[string]string{"deliverable.md": fmt.Sprintf("# report rev %d\n", n)},
		SnapshotSHA: sha,
	}); err != nil {
		t.Fatalf("MintRevision %s/%d: %v", id, n, err)
	}
}

func sit1Kinds(files []review.ChangedFile) map[string]string {
	out := map[string]string{}
	for _, f := range files {
		out[f.Path] = f.Kind
	}
	return out
}

// TestSIT1CompareRepoBackedServesTheTree — R2/R3. The default round-over-round
// comparison of a repo-backed deliverable is the TREE diff between the two
// snapshot pins: the inventory names what changed, the unified text carries
// the per-file diffs, the companion report is nowhere in it, and a binary file
// has an inventory row and no diff text.
func TestSIT1CompareRepoBackedServesTheTree(t *testing.T) {
	f, _ := sit1World(t)
	cmp, err := f.store.Compare(context.Background(), "dlv-t1", 1, 2)
	if err != nil {
		t.Fatalf("Compare(1,2): %v", err)
	}
	if cmp.Surface != review.SurfaceLineDiff {
		t.Fatalf("surface = %q, want %q (code/text = line diff, Spec S13.2)", cmp.Surface, review.SurfaceLineDiff)
	}
	if cmp.Change == nil {
		t.Fatal("a repo-backed comparison served no change inventory")
	}
	want := map[string]string{"README.md": review.KindDeleted, "assets/logo.png": review.KindAdded, "src/app.go": review.KindModified}
	if got := sit1Kinds(cmp.Change.Files); !reflect.DeepEqual(got, want) {
		t.Fatalf("inventory kinds = %v, want %v", got, want)
	}
	if cmp.Change.OldPin != "s1" || cmp.Change.NewPin != "s2" || cmp.Change.OldIsBase {
		t.Fatalf("pins = %q → %q (base %v), want s1 → s2 (not the base)", cmp.Change.OldPin, cmp.Change.NewPin, cmp.Change.OldIsBase)
	}
	for _, need := range []string{"diff --git a/src/app.go b/src/app.go", "+\tlisten()", "-# shop"} {
		if !strings.Contains(cmp.Unified, need) {
			t.Errorf("unified diff lacks %q:\n%s", need, cmp.Unified)
		}
	}
	for _, never := range []string{"deliverable.md", "logo.png", "report rev"} {
		if strings.Contains(cmp.Unified, never) {
			t.Errorf("unified diff carries %q — the report is a companion, a binary has no diff text:\n%s", never, cmp.Unified)
		}
	}
	for _, row := range cmp.Change.Files {
		if row.Path == "assets/logo.png" && !row.Binary {
			t.Errorf("logo.png not flagged binary: %+v", row)
		}
	}
	if cmp.Truncated {
		t.Fatalf("a small change was marked truncated: %s", cmp.TruncationReason)
	}
}

// TestSIT1RevisionOneAgainstThePreTaskBase — R3. old=0 is the recorded
// pre-task base (Spec S13.1): an unchanged base file is NOT in the inventory
// and the diff shows only what the task changed — never "everything added".
func TestSIT1RevisionOneAgainstThePreTaskBase(t *testing.T) {
	f, _ := sit1World(t)
	ctx := context.Background()
	ch, err := f.store.Change(ctx, "dlv-t1", 0, 1)
	if err != nil {
		t.Fatalf("Change(0,1): %v", err)
	}
	if !ch.OldIsBase || ch.OldPin != "s0" || ch.NewPin != "s1" {
		t.Fatalf("Change(0,1) pins: old %q base=%v new %q, want the base s0 → s1", ch.OldPin, ch.OldIsBase, ch.NewPin)
	}
	want := map[string]string{"src/app.go": review.KindModified, "src/cart.go": review.KindAdded}
	if got := sit1Kinds(ch.Files); !reflect.DeepEqual(got, want) {
		t.Fatalf("Change(0,1) kinds = %v, want %v (README.md is unchanged from the base)", got, want)
	}
	cmp, err := f.store.Compare(ctx, "dlv-t1", 0, 1)
	if err != nil {
		t.Fatalf("Compare(0,1): %v", err)
	}
	// The old side really is the BASE, stated positively: the base's own line is
	// shown being replaced. That line is absent exactly when the old side was
	// empty, which is the failure this guards. It is asserted this way because
	// the negative form cannot be: `src/cart.go` is genuinely added here and its
	// first line is "package app", so forbidding "+package app" fires on a
	// correct answer (P3-SIT-1, sanctioned).
	if !strings.Contains(cmp.Unified, "+\tserve()") || !strings.Contains(cmp.Unified, "-func Run() {}") {
		t.Fatalf("Compare(0,1) is not base → rev 1 — the base line being replaced is missing, so the old side was not the base:\n%s", cmp.Unified)
	}
	// And an UNCHANGED base file is not a change: README.md is byte-identical in
	// s0 and s1, so it appears in no header and on no line.
	if strings.Contains(cmp.Unified, "README.md") || strings.Contains(cmp.Unified, "# shop") {
		t.Fatalf("a base file unchanged by the task rendered in the change:\n%s", cmp.Unified)
	}
}

// TestSIT1AnyRevisionPairOnDemand — R3. Revision-over-revision navigation is a
// schema capability (Spec S13.1): 1→3 and 0→3 are answered like the default.
func TestSIT1AnyRevisionPairOnDemand(t *testing.T) {
	f, _ := sit1World(t)
	ctx := context.Background()
	cmp, err := f.store.Compare(ctx, "dlv-t1", 1, 3)
	if err != nil {
		t.Fatalf("Compare(1,3): %v", err)
	}
	if cmp.Change == nil {
		t.Fatal("Compare(1,3) served no change inventory")
	}
	want := map[string]string{"README.md": review.KindDeleted, "assets/logo.png": review.KindAdded, "src/app.go": review.KindModified}
	if got := sit1Kinds(cmp.Change.Files); !reflect.DeepEqual(got, want) {
		t.Fatalf("Compare(1,3) kinds = %v, want %v", got, want)
	}
	if !strings.Contains(cmp.Unified, "+// header") || !strings.Contains(cmp.Unified, "+\tlisten()") {
		t.Fatalf("Compare(1,3) misses the cumulative change:\n%s", cmp.Unified)
	}
	ch, err := f.store.Change(ctx, "dlv-t1", 0, 3)
	if err != nil {
		t.Fatalf("Change(0,3): %v", err)
	}
	if !ch.OldIsBase || len(ch.Files) != 4 {
		t.Fatalf("Change(0,3) = base %v, %d rows, want the base and 4 rows (app modified, cart+logo added, README deleted): %+v", ch.OldIsBase, len(ch.Files), ch.Files)
	}
}

// TestSIT1CompareFileAndFileContent — R4/R5. One file's diff and one file's
// content at a pin, byte-exact; a binary is flagged and never served inline; a
// deleted path is not found at the revision it is gone from; the companion
// report is reachable by name through the same read.
func TestSIT1CompareFileAndFileContent(t *testing.T) {
	f, _ := sit1World(t)
	ctx := context.Background()

	one, err := f.store.CompareFile(ctx, "dlv-t1", 1, 2, "src/app.go")
	if err != nil {
		t.Fatalf("CompareFile: %v", err)
	}
	if one.Change == nil || len(one.Change.Files) != 1 || one.Change.Files[0].Path != "src/app.go" {
		t.Fatalf("CompareFile inventory = %+v, want the one row", one.Change)
	}
	if !strings.HasPrefix(one.Unified, "diff --git a/src/app.go b/src/app.go") || strings.Contains(one.Unified, "README") {
		t.Fatalf("CompareFile unified is not that file's diff alone:\n%s", one.Unified)
	}
	if _, err := f.store.CompareFile(ctx, "dlv-t1", 1, 2, "src/cart.go"); !errors.Is(err, review.ErrNotFound) {
		t.Fatalf("CompareFile on an unchanged path: err %v, want ErrNotFound (the change does not cover it)", err)
	}

	got, err := f.store.RevisionFile(ctx, "dlv-t1", 2, "src/app.go")
	if err != nil {
		t.Fatalf("RevisionFile: %v", err)
	}
	if got.Content != sit1AppRev2 || got.Size != int64(len(sit1AppRev2)) || got.Binary || got.Truncated || got.Pin != "s2" {
		t.Fatalf("RevisionFile(2, src/app.go) = %+v, want the pinned bytes verbatim", got)
	}
	if old, err := f.store.RevisionFile(ctx, "dlv-t1", 1, "README.md"); err != nil || old.Content != sit1Readme {
		t.Fatalf("RevisionFile(1, README.md) = %+v err %v, want the rev-1 bytes", old, err)
	}
	if _, err := f.store.RevisionFile(ctx, "dlv-t1", 2, "README.md"); !errors.Is(err, review.ErrNotFound) {
		t.Fatalf("RevisionFile(2, README.md): err %v, want ErrNotFound (deleted at rev 2)", err)
	}
	bin, err := f.store.RevisionFile(ctx, "dlv-t1", 2, "assets/logo.png")
	if err != nil {
		t.Fatalf("RevisionFile binary: %v", err)
	}
	if !bin.Binary || bin.Content != "" || bin.Size != int64(len(sit1PNG)) {
		t.Fatalf("binary file served as %+v, want binary=true, no inline content, the real size", bin)
	}
	rep, err := f.store.RevisionFile(ctx, "dlv-t1", 2, "deliverable.md")
	if err != nil {
		t.Fatalf("RevisionFile companion: %v", err)
	}
	if rep.Content != "# report rev 2\n" {
		t.Fatalf("companion report = %q, want the revision's own object", rep.Content)
	}
	if _, err := f.store.RevisionFile(ctx, "dlv-t1", 2, "nope.txt"); !errors.Is(err, review.ErrNotFound) {
		t.Fatalf("unknown path: err %v, want ErrNotFound", err)
	}
}

// TestSIT1BoundsAreHonest — R6. A file past the caps is served up to the cap,
// on a line boundary, marked truncated WITH its reason — never silently cut,
// never refused. The whole-change text stops at a file boundary.
func TestSIT1BoundsAreHonest(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.task("t1", "u1")
	f.run("r1", "t1")
	if _, err := f.store.EnsureDeliverable(ctx, review.EnsureInput{ID: "dlv-t1", Owner: "u1", TaskID: "t1", Type: "code"}); err != nil {
		t.Fatalf("EnsureDeliverable: %v", err)
	}
	var sb strings.Builder
	for i := 0; sb.Len() <= review.TreeFileBytesCap+4096; i++ {
		fmt.Fprintf(&sb, "line %06d of the big file\n", i)
	}
	big := sb.String()
	trees := sit1Trees{base: "b0", trees: map[string]map[string]string{
		"b0": {},
		"b1": {"big.txt": big, "small.txt": "one\n"},
	}}
	sit1Mint(t, f, "dlv-t1", 1, "b1")
	f.store.Tree = trees

	one, err := f.store.CompareFile(ctx, "dlv-t1", 0, 1, "big.txt")
	if err != nil {
		t.Fatalf("CompareFile big: %v", err)
	}
	if !one.Truncated || one.TruncationReason == "" {
		t.Fatalf("a %d-byte diff over the %d cap was not marked truncated with a reason: %+v", len(big), review.TreeFileDiffBytesCap, one.Truncated)
	}
	if len(one.Unified) > review.TreeFileDiffBytesCap || !strings.HasSuffix(one.Unified, "\n") || !strings.HasPrefix(one.Unified, "diff --git a/big.txt b/big.txt") {
		t.Fatalf("truncated diff: %d bytes (cap %d), line-bounded %v, headed %v", len(one.Unified), review.TreeFileDiffBytesCap,
			strings.HasSuffix(one.Unified, "\n"), strings.HasPrefix(one.Unified, "diff --git"))
	}
	if !strings.Contains(one.Unified, "\n@@ ") {
		t.Fatalf("truncated diff holds no hunk header:\n%.200s", one.Unified)
	}

	whole, err := f.store.Compare(ctx, "dlv-t1", 0, 1)
	if err != nil {
		t.Fatalf("Compare whole: %v", err)
	}
	if !whole.Truncated || whole.TruncationReason == "" || len(whole.Unified) > review.TreeDiffBytesCap {
		t.Fatalf("whole change over the cap: truncated=%v reason=%q bytes=%d (cap %d)", whole.Truncated, whole.TruncationReason, len(whole.Unified), review.TreeDiffBytesCap)
	}
	if whole.Change == nil || len(whole.Change.Files) != 2 {
		t.Fatalf("the inventory must stay WHOLE under truncation: %+v", whole.Change)
	}

	body, err := f.store.RevisionFile(ctx, "dlv-t1", 1, "big.txt")
	if err != nil {
		t.Fatalf("RevisionFile big: %v", err)
	}
	if !body.Truncated || body.TruncationReason == "" || body.Size != int64(len(big)) {
		t.Fatalf("big content: truncated=%v reason=%q size=%d (want %d)", body.Truncated, body.TruncationReason, body.Size, len(big))
	}
	if len(body.Content) > review.TreeFileBytesCap || !strings.HasSuffix(body.Content, "\n") || !strings.HasPrefix(big, body.Content) {
		t.Fatalf("truncated content is not a line-bounded prefix within the cap: %d bytes", len(body.Content))
	}
	small, err := f.store.RevisionFile(ctx, "dlv-t1", 1, "small.txt")
	if err != nil || small.Truncated || small.Content != "one\n" {
		t.Fatalf("small file beside the big one: %+v err %v", small, err)
	}
}

// TestSIT1AnchorsResolveAgainstTheTree — R7 (Spec S13.3). file_path is the
// repo-relative path in the pinned tree, line_no the 1-based line in that file
// at that revision. Birth validation, a finding's "path:line" anchor and the
// port ladder all work over tree files — and a comment on the companion report
// still anchors.
func TestSIT1AnchorsResolveAgainstTheTree(t *testing.T) {
	f, _ := sit1World(t)
	ctx := context.Background()
	c, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", RevisionN: 2, Author: "u1", Body: "listen before serve?",
		Anchor: &review.AnchorRecord{FilePath: "src/app.go", Side: review.SideNew, LineNo: 5, LineText: "\tlisten()"},
	})
	if err != nil {
		t.Fatalf("AddComment on a tree file: %v", err)
	}
	if c.BornStatus != review.AnchorExact {
		t.Fatalf("tree-file comment born %q, want exact (src/app.go:5 is \"\\tlisten()\" at rev 2)", c.BornStatus)
	}
	ids, err := f.store.AddFindings(ctx, "dlv-t1", 2, []review.FindingInput{{
		Author: "u1", RunID: "r1", Severity: review.SeverityNote, Body: "cart is empty", RawAnchor: "src/cart.go:3",
	}})
	if err != nil || len(ids) != 1 {
		t.Fatalf("AddFindings: %v (%d)", err, len(ids))
	}
	fc, err := f.store.CommentByID(ctx, ids[0])
	if err != nil {
		t.Fatalf("CommentByID: %v", err)
	}
	if fc.BornStatus != review.AnchorExact || fc.Anchor.LineNo != 3 || fc.Anchor.LineText != "func Cart() {}" {
		t.Fatalf("finding anchor src/cart.go:3 resolved as %q %+v, want exact with the tree's line 3", fc.BornStatus, fc.Anchor)
	}
	rep, err := f.store.AddComment(ctx, review.CommentInput{
		DeliverableID: "dlv-t1", RevisionN: 2, Author: "u1", Body: "on the report",
		Anchor: &review.AnchorRecord{FilePath: "deliverable.md", Side: review.SideNew, LineNo: 1, LineText: "# report rev 2"},
	})
	if err != nil || rep.BornStatus != review.AnchorExact {
		t.Fatalf("companion-report comment: %+v err %v, want exact", rep.BornStatus, err)
	}

	// Port to rev 3, whose app.go gained a header line: the tree comment maps
	// through the file's own diff (ladder step 1) to line 6.
	_, placements, err := f.store.PlacedComments(ctx, "dlv-t1", 3)
	if err != nil {
		t.Fatalf("PlacedComments(3): %v", err)
	}
	var ported *review.Placement
	for i := range placements {
		if placements[i].CommentID == c.ID {
			ported = &placements[i]
		}
	}
	if ported == nil || ported.Status != review.AnchorMapped || ported.Anchor.LineNo != 6 {
		t.Fatalf("tree comment ported to rev 3 as %+v, want mapped at line 6", ported)
	}
}

// TestSIT1ContentPinnedLanesDoNotMove — R8. A content-pinned deliverable (no
// snapshot) compares byte-identically whether or not a tree source is wired,
// and serves no change inventory.
func TestSIT1ContentPinnedLanesDoNotMove(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.seed()
	f.mint(1, "one\ntwo\n")
	f.mint(2, "one\ntwo\nthree\n")
	before, err := f.store.Compare(ctx, "dlv-t1", 1, 2)
	if err != nil {
		t.Fatalf("Compare (no seam): %v", err)
	}
	f.store.Tree = sit1Trees{base: "x", trees: map[string]map[string]string{"x": {}}}
	after, err := f.store.Compare(ctx, "dlv-t1", 1, 2)
	if err != nil {
		t.Fatalf("Compare (seam wired): %v", err)
	}
	if !reflect.DeepEqual(before, after) || after.Change != nil {
		t.Fatalf("a content-pinned comparison moved when a tree source was wired:\n%+v\nvs\n%+v", before, after)
	}
	if _, err := f.store.RevisionFile(ctx, "dlv-t1", 2, "deliverable.md"); err != nil {
		t.Fatalf("RevisionFile on a content-pinned revision must serve its object: %v", err)
	}
}

// TestSIT1SittingShapeIsRepoBackedByItsPinNotItsType — R9. The sitting world's
// row is `markdown` with a snapshot pin, and its dtype cannot be rewritten
// (identity-immutable, migration 0007). Repo-backed is DERIVED on read from
// snapshot_sha, so the old row serves the tree without a data migration.
func TestSIT1SittingShapeIsRepoBackedByItsPinNotItsType(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.task("t1", "u1")
	f.run("r1", "t1")
	if _, err := f.store.EnsureDeliverable(ctx, review.EnsureInput{ID: "dlv-t1", Owner: "u1", TaskID: "t1", Type: "markdown"}); err != nil {
		t.Fatalf("EnsureDeliverable: %v", err)
	}
	sit1Mint(t, f, "dlv-t1", 1, "s1")
	// The migration route is closed by the schema itself (GREEN half).
	f.mustAbort(`UPDATE deliverables SET dtype = 'code' WHERE deliverable_id = 'dlv-t1'`)

	f.store.Tree = sit1Trees{base: "s0", trees: map[string]map[string]string{
		"s0": {"src/app.go": sit1AppBase},
		"s1": {"src/app.go": sit1AppRev1, "src/cart.go": sit1Cart},
	}}
	cmp, err := f.store.Compare(ctx, "dlv-t1", 0, 1)
	if err != nil {
		t.Fatalf("Compare(0,1) on the sitting shape: %v", err)
	}
	if cmp.Change == nil || len(cmp.Change.Files) != 2 || !strings.Contains(cmp.Unified, "+func Cart()") {
		t.Fatalf("a markdown row WITH a snapshot pin must serve the tree (derived from the pin), got %+v\n%s", cmp.Change, cmp.Unified)
	}
	if d, _ := f.store.Deliverable(ctx, "dlv-t1"); d.Type != "markdown" {
		t.Fatalf("the read rewrote history: dtype %q", d.Type)
	}
}

// TestSIT1NilTreeSourceKeepsTheCompanionDiffLabeled — R2. With no tree source
// composed, a repo-backed revision still compares (today's companion-object
// diff) — but the answer SAYS the tree is not reachable, rather than posing as
// the reviewable change.
func TestSIT1NilTreeSourceKeepsTheCompanionDiffLabeled(t *testing.T) {
	f, _ := sit1World(t)
	f.store.Tree = nil
	cmp, err := f.store.Compare(context.Background(), "dlv-t1", 1, 2)
	if err != nil {
		t.Fatalf("Compare (nil seam): %v", err)
	}
	if cmp.Change != nil || !strings.Contains(cmp.Unified, "report rev 2") {
		t.Fatalf("nil seam must fall back to the companion diff with no inventory: %+v", cmp)
	}
	if cmp.Label == "" {
		t.Fatal("nil seam on a repo-backed revision served the companion diff with no label — an absence must be stated")
	}
	ch, err := f.store.Change(context.Background(), "dlv-t1", 1, 2)
	if err != nil || ch.AbsentReason == "" || len(ch.Files) != 0 {
		t.Fatalf("Change with nil seam = %+v err %v, want an empty inventory with its absent_reason", ch, err)
	}
}

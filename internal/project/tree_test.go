package project

// tree_test.go — P3-SIT-1 R14: the read-only tree verbs, against real commits
// of a real store.
//
// What a repo-backed deliverable IS rests on these three answers (Spec S13.1),
// so each one is bound to facts the test itself wrote: the inventory against
// the files it moved, the blob against the bytes it committed, and the base
// against the ref EnsureWorkspace recorded. The reads are also asserted to be
// reads — the §59 rule that asking a question must not manufacture its answer.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	sitAppV1 = "package app\n\nfunc Run() {}\n"
	sitAppV2 = "package app\n\nfunc Run() {\n\tserve()\n}\n"
	sitPNG   = "\x89PNG\r\n\x1a\n\x00\x00IHDR-one"
	sitPNG2  = "\x89PNG\r\n\x1a\n\x00\x00IHDR-two-longer"
)

// sitTreeWorld activates a project, opens its run-branch worktree, and takes
// two platform snapshots: the second adds a file, modifies another, deletes a
// third, renames a fourth and rewrites a binary.
func sitTreeWorld(t *testing.T) (*fix, string, string, string) {
	t.Helper()
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{
		"README.md":    "# shop\n",
		"src/app.go":   sitAppV1,
		"docs/old.md":  "moved verbatim\nline two\nline three\nline four\n",
		"assets/l.png": sitPNG,
	})
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "t-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	first, err := f.store.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot 1: %v", err)
	}
	if first == "" {
		// Nothing was written since the base, so the snapshot is the base tip.
		first = ws.Base
	}
	if err := os.Remove(filepath.Join(ws.Path, "README.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(ws.Path, "docs/old.md"), filepath.Join(ws.Path, "docs/new.md")); err != nil {
		t.Fatal(err)
	}
	f.writeFiles(ws.Path, map[string]string{
		"src/app.go":   sitAppV2,
		"assets/l.png": sitPNG2,
		"src/cart.go":  "package app\n\nfunc Cart() {}\n",
	})
	second, err := f.store.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot 2: %v", err)
	}
	return f, ws.Path, first, second
}

func sitRows(rows []TreeChange) map[string]TreeChange {
	out := map[string]TreeChange{}
	for _, r := range rows {
		out[r.Path] = r
	}
	return out
}

// TestSITTreeChangesNamesEveryKindWithItsSizes: the inventory carries the four
// kinds, both sides' sizes, git's binary verdict and the line counts — and it
// arrives in path order, because the served diff is concatenated in that order
// and a stable read of an immutable pair must not depend on map iteration.
func TestSITTreeChangesNamesEveryKindWithItsSizes(t *testing.T) {
	f, _, first, second := sitTreeWorld(t)
	ctx := context.Background()
	rows, err := f.store.TreeChanges(ctx, "shop", first, second)
	if err != nil {
		t.Fatalf("TreeChanges: %v", err)
	}
	byPath := sitRows(rows)

	app, ok := byPath["src/app.go"]
	if !ok || app.Kind != kindModified {
		t.Fatalf("src/app.go = %+v, want a modification", app)
	}
	if app.OldSize != int64(len(sitAppV1)) || app.NewSize != int64(len(sitAppV2)) {
		t.Errorf("src/app.go sizes = %d → %d, want %d → %d", app.OldSize, app.NewSize, len(sitAppV1), len(sitAppV2))
	}
	if app.Binary || app.Additions == 0 {
		t.Errorf("a modified source file must carry line counts and no binary verdict: %+v", app)
	}
	if app.OldPath != "" {
		t.Errorf("a non-rename row carries an old path: %+v", app)
	}
	if cart := byPath["src/cart.go"]; cart.Kind != kindAdded || cart.OldSize != 0 || cart.NewSize == 0 {
		t.Errorf("src/cart.go = %+v, want an addition with no old side", cart)
	}
	if readme := byPath["README.md"]; readme.Kind != kindDeleted || readme.NewSize != 0 || readme.OldSize == 0 {
		t.Errorf("README.md = %+v, want a deletion with no new side", readme)
	}
	// A rename is ONE row covering two paths: the surface has to be able to say
	// "this moved" rather than "one file vanished and another appeared".
	moved, ok := byPath["docs/new.md"]
	if !ok || moved.Kind != kindRenamed || moved.OldPath != "docs/old.md" {
		t.Fatalf("the rename is not one row: %+v (all rows %+v)", moved, rows)
	}
	if moved.OldSize == 0 || moved.NewSize == 0 {
		t.Errorf("a rename must carry both sides' sizes: %+v", moved)
	}
	png, ok := byPath["assets/l.png"]
	if !ok || !png.Binary {
		t.Fatalf("assets/l.png = %+v, want git's binary verdict", png)
	}
	if png.Additions != 0 || png.Deletions != 0 {
		t.Errorf("a binary has no line counts to report: %+v", png)
	}
	if png.OldSize != int64(len(sitPNG)) || png.NewSize != int64(len(sitPNG2)) {
		t.Errorf("binary sizes = %d → %d, want %d → %d", png.OldSize, png.NewSize, len(sitPNG), len(sitPNG2))
	}

	for i := 1; i < len(rows); i++ {
		if rows[i-1].Path >= rows[i].Path {
			t.Fatalf("the inventory is not in path order: %q then %q", rows[i-1].Path, rows[i].Path)
		}
	}
	// An unchanged file is not a change. The whole point of the base comparison
	// is that revision 1 shows what the task did, never the whole repository.
	if _, present := byPath["docs/old.md"]; present {
		t.Error("the rename's old path is a second row — it is covered by the rename row")
	}
	// Identical pins: an empty inventory, not an error.
	same, err := f.store.TreeChanges(ctx, "shop", second, second)
	if err != nil || len(same) != 0 {
		t.Errorf("a pin compared with itself = %d rows, %v; want an empty change", len(same), err)
	}
}

// TestSITTreeChangesRefusesAPinTheStoreDoesNotHold: a sha this store never had
// is a LOUD refusal, because the alternative is an empty inventory that reads
// as "nothing changed" about work the platform has lost track of.
func TestSITTreeChangesRefusesAPinTheStoreDoesNotHold(t *testing.T) {
	f, _, first, second := sitTreeWorld(t)
	ctx := context.Background()
	absent := "0123456789abcdef0123456789abcdef01234567"
	for _, c := range []struct{ old, new string }{{absent, second}, {first, absent}, {"", second}, {first, ""}} {
		if _, err := f.store.TreeChanges(ctx, "shop", c.old, c.new); err == nil {
			t.Errorf("TreeChanges(%q, %q) answered without error", c.old, c.new)
		}
	}
}

// TestSITTreeReadsCreateNothing (§59 "reading a fact must not create it"): the
// inventory and the blob reads leave the store's refs, HEAD and object graph
// exactly as they found them, and repeat reads agree. A verb that commits in
// order to answer would be manufacturing the evidence it reports.
func TestSITTreeReadsCreateNothing(t *testing.T) {
	f, ws, first, second := sitTreeWorld(t)
	ctx := context.Background()
	e, err := f.store.Get(ctx, "shop")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	refsBefore := f.git(e.StorePath, "show-ref")
	headBefore := f.git(ws, "rev-parse", "HEAD")
	countBefore := f.git(e.StorePath, "rev-list", "--all", "--count")

	// An uncommitted file in the worktree must not move any answer either.
	f.writeFiles(ws, map[string]string{"scratch.tmp": "not committed\n"})

	rows1, err := f.store.TreeChanges(ctx, "shop", first, second)
	if err != nil {
		t.Fatalf("TreeChanges: %v", err)
	}
	rows2, err := f.store.TreeChanges(ctx, "shop", first, second)
	if err != nil {
		t.Fatalf("TreeChanges again: %v", err)
	}
	if len(rows1) != len(rows2) {
		t.Fatalf("repeat reads disagree: %d vs %d rows", len(rows1), len(rows2))
	}
	for i := range rows1 {
		if rows1[i] != rows2[i] {
			t.Fatalf("repeat read differs at %d: %+v vs %+v", i, rows1[i], rows2[i])
		}
	}
	if _, _, _, err := f.store.TreeBlob(ctx, "shop", second, "src/app.go", 0); err != nil {
		t.Fatalf("TreeBlob: %v", err)
	}
	if got := f.git(e.StorePath, "show-ref"); got != refsBefore {
		t.Errorf("a read moved a ref:\nbefore %s\nafter  %s", refsBefore, got)
	}
	if got := f.git(ws, "rev-parse", "HEAD"); got != headBefore {
		t.Errorf("a read moved HEAD: %s → %s", headBefore, got)
	}
	if got := f.git(e.StorePath, "rev-list", "--all", "--count"); got != countBefore {
		t.Errorf("a read created a commit: %s → %s", countBefore, got)
	}
	// And the uncommitted file is still uncommitted — no read staged it.
	if status := f.git(ws, "status", "--porcelain"); !strings.Contains(status, "scratch.tmp") {
		t.Errorf("the uncommitted file left the worktree: %q", status)
	}
}

// TestSITTreeBlobIsByteExactAndBounded: the bytes come back verbatim —
// no added trailing newline, no trimming, empty files stay empty — a limit is
// honoured while Size still reports the whole file, and a path the tree does
// not hold is an absence rather than a failure.
func TestSITTreeBlobIsByteExactAndBounded(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"seed.txt": "seed\n"})
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "t-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	bodies := map[string]string{
		"plain.txt":     "one\ntwo\nthree\n",
		"nonewline.txt": "no trailing newline",
		"empty.txt":     "",
		"blanks.txt":    "\n\n  indented\n\n",
	}
	f.writeFiles(ws.Path, bodies)
	sha, err := f.store.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	for name, want := range bodies {
		data, size, ok, err := f.store.TreeBlob(ctx, "shop", sha, name, 0)
		if err != nil || !ok {
			t.Fatalf("TreeBlob %s: ok=%v err=%v", name, ok, err)
		}
		if string(data) != want {
			t.Errorf("TreeBlob %s = %q, want %q (byte-exact)", name, data, want)
		}
		if size != int64(len(want)) {
			t.Errorf("TreeBlob %s size = %d, want %d", name, size, len(want))
		}
	}
	// A limit bounds the BYTES READ and never the size reported: an honest
	// truncation needs both halves.
	data, size, ok, err := f.store.TreeBlob(ctx, "shop", sha, "plain.txt", 4)
	if err != nil || !ok {
		t.Fatalf("bounded TreeBlob: ok=%v err=%v", ok, err)
	}
	if string(data) != "one\n" || size != int64(len(bodies["plain.txt"])) {
		t.Errorf("bounded read = %q size %d, want %q and the full size %d", data, size, "one\n", len(bodies["plain.txt"]))
	}
	// A limit larger than the file reads the file.
	if data, _, _, err := f.store.TreeBlob(ctx, "shop", sha, "plain.txt", 1<<20); err != nil || string(data) != bodies["plain.txt"] {
		t.Errorf("over-large limit = %q, %v", data, err)
	}
	if _, _, ok, err := f.store.TreeBlob(ctx, "shop", sha, "nope.txt", 0); ok || err != nil {
		t.Errorf("a path the tree does not hold = ok %v, err %v; want an absence", ok, err)
	}
	if _, _, ok, err := f.store.TreeBlob(ctx, "shop", sha, "src", 0); ok || err != nil {
		t.Errorf("a directory is no blob: ok %v, err %v", ok, err)
	}
}

// TestSITBaseSHAIsTheRecordedBase: the base a revision-1 comparison rests on is
// the ref EnsureWorkspace recorded, and it is "" — never a guess — before the
// workspace exists.
func TestSITBaseSHAIsTheRecordedBase(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("shop", "alice", "shop", map[string]string{"README.md": "# shop\n"})

	before, err := f.store.BaseSHA(ctx, "shop", "t-1")
	if err != nil {
		t.Fatalf("BaseSHA before: %v", err)
	}
	if before != "" {
		t.Fatalf("a pipeline with no workspace reported a base %q — the basis is unknown and must not be guessed", before)
	}
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "t-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	after, err := f.store.BaseSHA(ctx, "shop", "t-1")
	if err != nil {
		t.Fatalf("BaseSHA after: %v", err)
	}
	if after != ws.Base {
		t.Fatalf("BaseSHA = %q, want the workspace's recorded base %q", after, ws.Base)
	}
	if head := f.git(e.StorePath, "rev-parse", "refs/heads/"+e.DefaultBranch); after != head {
		t.Fatalf("attempt 1's base %q is not the default branch HEAD at creation %q", after, head)
	}
	// Snapshots accumulate on the run branch; the recorded base does not move.
	f.writeFiles(ws.Path, map[string]string{"new.txt": "x\n"})
	if _, err := f.store.Snapshot(ctx, ws.Path); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if moved, _ := f.store.BaseSHA(ctx, "shop", "t-1"); moved != after {
		t.Fatalf("the recorded base moved with a snapshot: %q → %q", after, moved)
	}
	// An unknown project is a refusal, not an empty answer.
	if _, err := f.store.BaseSHA(ctx, "nosuch", "t-1"); err == nil {
		t.Error("BaseSHA on an unregistered project answered without error")
	}
}

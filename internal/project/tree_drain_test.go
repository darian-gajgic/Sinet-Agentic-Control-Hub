package project

// tree_drain_test.go — P3-SIT-1 drain r1: the three tree-verb defects the
// evaluation proved, each bound to the condition that produced it.
//
// F1  a pin the store no longer holds is a TYPED failure, distinct from "that
//     path is not in this tree" — one is lost work, the other is an ordinary
//     absence, and collapsing them served a 404 over the round report for a
//     revision whose files were gone.
// F2  a directory resolves as an object and has a size, so a blob read that
//     only asked "does this resolve" handed git's own error text to a requester.
// F10 a file literally named ":x" is pathspec MAGIC to ls-tree, so its size row
//     never came back.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// sitPinnedWorld activates a project, writes one file into its workspace and
// pins it with a real snapshot commit.
func sitPinnedWorld(t *testing.T, files map[string]string) (*fix, Entry, string) {
	t.Helper()
	ctx := context.Background()
	f := newFix(t)
	e := f.activeProject("shop", "alice", "shop", map[string]string{"README.md": "# shop\n"})
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "t-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	f.writeFiles(ws.Path, files)
	sha, err := f.store.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if sha == "" {
		t.Fatal("no snapshot commit for a worktree that was written to")
	}
	return f, e, sha
}

// dropObject deletes a loose git object from the store, which is what a lost
// pin looks like: the revision row still names the commit and the store no
// longer has it. The object directory is asked of git rather than assumed,
// because the project store is a non-bare clone and its objects live under
// .git (CONVENTIONS §23: behaviour is read off this host's git, never guessed).
func dropObject(t *testing.T, f *fix, storePath, sha string) {
	t.Helper()
	objects := f.git(storePath, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	loose := filepath.Join(objects, sha[:2], sha[2:])
	if err := os.Remove(loose); err != nil {
		t.Fatalf("remove loose object %s: %v", sha, err)
	}
}

// TestSITLostPinIsTypedNotAnAbsence (F1): with the pinned commit gone, both
// verbs report ErrPinMissing and answer the structural TreePinMissing contract
// that consumers outside this package's import reach recognise it by. A path
// that is simply not in an EXISTING tree stays an ordinary absence — the
// distinction is the whole point.
func TestSITLostPinIsTypedNotAnAbsence(t *testing.T) {
	ctx := context.Background()
	f, e, sha := sitPinnedWorld(t, map[string]string{"src/app.go": "package app\n"})

	// The control, taken BEFORE the object is dropped: both verbs answer here.
	if _, _, ok, err := f.store.TreeBlob(ctx, "shop", sha, "src/app.go", 0); !ok || err != nil {
		t.Fatalf("the fixture proves nothing — the blob does not read before the drop: ok %v err %v", ok, err)
	}
	// An absent path, while the commit is present, is ok=false and NO error.
	if _, _, ok, err := f.store.TreeBlob(ctx, "shop", sha, "never-there.txt", 0); ok || err != nil {
		t.Fatalf("a path absent from a present tree = ok %v err %v; want an ordinary absence", ok, err)
	}

	dropObject(t, f, e.StorePath, sha)

	_, _, ok, err := f.store.TreeBlob(ctx, "shop", sha, "src/app.go", 0)
	if ok {
		t.Fatal("TreeBlob answered from a commit the store no longer holds")
	}
	if !errors.Is(err, ErrPinMissing) {
		t.Fatalf("TreeBlob on a lost pin: err %v, want ErrPinMissing — a lost pin read as an absence is how it became a 404", err)
	}
	if _, cerr := f.store.TreeChanges(ctx, "shop", sha, sha); !errors.Is(cerr, ErrPinMissing) {
		t.Fatalf("TreeChanges on a lost pin: err %v, want ErrPinMissing", cerr)
	}
	// The STRUCTURAL contract, which is what internal/review recognises it by:
	// the two packages cannot share a sentinel across the §23 import wall.
	var missing interface{ TreePinMissing() bool }
	if !errors.As(err, &missing) || !missing.TreePinMissing() {
		t.Fatalf("the lost-pin error does not answer TreePinMissing, so review cannot map it to drift: %v", err)
	}
	// And the error carries the pin rather than git's stderr.
	if got := err.Error(); got == "" {
		t.Fatal("the lost-pin error says nothing")
	}
}

// TestSITDirectoryIsNotAFile (F2): a real directory in a real tree is an
// absence, not an error and not a blob — with and without a trailing slash, and
// with the leading "./" form F10 normalises.
func TestSITDirectoryIsNotAFile(t *testing.T) {
	ctx := context.Background()
	f, _, sha := sitPinnedWorld(t, map[string]string{
		"src/app.go":       "package app\n",
		"src/lib/util.go":  "package lib\n",
		"docs/notes/a.txt": "note\n",
	})
	for _, dir := range []string{"src", "src/", "src/lib", "docs/notes", "./src"} {
		data, size, ok, err := f.store.TreeBlob(ctx, "shop", sha, dir, 0)
		if ok || err != nil || data != nil || size != 0 {
			t.Errorf("TreeBlob(%q) = %q size %d ok %v err %v; a directory is not a file", dir, data, size, ok, err)
		}
	}
	// Non-vacuity: the files under those directories DO read, so the probes
	// above are naming paths this tree really contains.
	for _, file := range []string{"src/app.go", "src/lib/util.go", "docs/notes/a.txt"} {
		if _, _, ok, err := f.store.TreeBlob(ctx, "shop", sha, file, 0); !ok || err != nil {
			t.Errorf("%s does not read, so the directory probes prove nothing: ok %v err %v", file, ok, err)
		}
	}
}

// TestSITLeadingDotSlashNamesTheSameFile (F10): "./x" and "x" are one entry.
func TestSITLeadingDotSlashNamesTheSameFile(t *testing.T) {
	ctx := context.Background()
	f, _, sha := sitPinnedWorld(t, map[string]string{"src/app.go": "package app\n\nfunc Run() {}\n"})
	plain, size, ok, err := f.store.TreeBlob(ctx, "shop", sha, "src/app.go", 0)
	if !ok || err != nil {
		t.Fatalf("TreeBlob: ok %v err %v", ok, err)
	}
	dotted, dsize, dok, derr := f.store.TreeBlob(ctx, "shop", sha, "./src/app.go", 0)
	if !dok || derr != nil {
		t.Fatalf("TreeBlob(\"./src/app.go\") = ok %v err %v", dok, derr)
	}
	if string(dotted) != string(plain) || dsize != size {
		t.Fatalf("./src/app.go served %q (%d), want src/app.go's %q (%d)", dotted, dsize, plain, size)
	}
}

// TestSITPathspecMagicNamesAFileNotAPattern (F10): a file whose name begins
// with ':' is a literal path, and its inventory row carries its real size. Read
// as pathspec magic it matches nothing and the size silently comes back 0.
func TestSITPathspecMagicNamesAFileNotAPattern(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.activeProject("shop", "alice", "shop", map[string]string{"README.md": "# shop\n"})
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "t-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	first, err := f.store.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot 1: %v", err)
	}
	if first == "" {
		first = ws.Base
	}
	const body = "colon-named\nsecond line\n"
	f.writeFiles(ws.Path, map[string]string{":x": body, "*star.txt": "star\n"})
	second, err := f.store.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot 2: %v", err)
	}
	rows := sitRows(mustChanges(t, f, first, second))
	colon, ok := rows[":x"]
	if !ok {
		t.Fatalf("the file named \":x\" is not in the inventory at all: %+v", rows)
	}
	if colon.NewSize != int64(len(body)) {
		t.Fatalf(":x size = %d, want %d — its pathspec was read as magic, so ls-tree matched no entry", colon.NewSize, len(body))
	}
	if star, ok := rows["*star.txt"]; !ok || star.NewSize == 0 {
		t.Fatalf("the file named \"*star.txt\" = %+v, want a row with its size", star)
	}
	// And it reads as a file, by its literal name.
	if data, _, ok, err := f.store.TreeBlob(ctx, "shop", second, ":x", 0); !ok || err != nil || string(data) != body {
		t.Fatalf("TreeBlob(\":x\") = %q ok %v err %v", data, ok, err)
	}
}

func mustChanges(t *testing.T, f *fix, oldSHA, newSHA string) []TreeChange {
	t.Helper()
	rows, err := f.store.TreeChanges(context.Background(), "shop", oldSHA, newSHA)
	if err != nil {
		t.Fatalf("TreeChanges: %v", err)
	}
	return rows
}

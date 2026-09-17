package api_test

// sit1_drain_test.go — P3-SIT-1 drain r1 at the transport, over the same REAL
// world the acceptance battery uses (a project onboarded from a file:// fixture,
// its worktree, real snapshot commits, revisions minted on them).
//
// F1 a revision whose pinned commit the store no longer holds answers
//    500 content_drift on EVERY read — never 500 internal carrying git's own
//    words, never a 404, and never the round report standing in for a code file.
// F2 a directory is not a file: 404, not git's "bad file" at 500.
// F3 a comment's anchor path is boundary-validated like every other path.
// F5 narrowing a comparison on a content-pinned version is a 400.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dropPinnedCommit removes the loose object a revision pins, which is what a
// lost pin is: the row still names the commit, the store no longer has it.
func dropPinnedCommit(t *testing.T, e *dlvEnv, projectID, sha string) {
	t.Helper()
	entry, err := e.proj.Get(e.ctx, projectID)
	if err != nil {
		t.Fatalf("project.Get: %v", err)
	}
	objects := e.git("-C", entry.StorePath, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err := os.Remove(filepath.Join(objects, sha[:2], sha[2:])); err != nil {
		t.Fatalf("remove loose object %s: %v", sha, err)
	}
}

// TestSIT1DrainLostPinAnswersContentDrift (F1): the four reads that can reach a
// revision's files all answer 500 content_drift, in the platform's own words.
func TestSIT1DrainLostPinAnswersContentDrift(t *testing.T) {
	w := newSIT1World(t)
	e := w.e
	filesPath := sit1FilesPath("dlv-t-shop", 2, "src/app.go")

	// The control: every read answers BEFORE the object is dropped, so the
	// statuses below are the drop's doing and not the fixture's.
	for _, p := range []string{
		"/api/deliverables/dlv-t-shop",
		"/api/deliverables/dlv-t-shop/compare?old=1&new=2",
		"/api/deliverables/dlv-t-shop/compare?old=1&new=2&path=src%2Fapp.go",
		filesPath,
	} {
		if code, body := e.do(t, "alice", "GET", p, ""); code != http.StatusOK {
			t.Fatalf("the fixture proves nothing — GET %s is %d before the drop: %s", p, code, body)
		}
	}

	dropPinnedCommit(t, e, "shop", w.pins[1])

	for _, p := range []string{
		"/api/deliverables/dlv-t-shop",                                       // the detail's change inventory
		"/api/deliverables/dlv-t-shop/compare?old=1&new=2",                   // the whole change
		"/api/deliverables/dlv-t-shop/compare?old=1&new=2&path=src%2Fapp.go", // one file's diff
		filesPath, // one file's bytes
	} {
		code, body := e.do(t, "alice", "GET", p, "")
		if code != http.StatusInternalServerError {
			t.Errorf("GET %s after the pin was lost: %d, want 500 (a lost pin is not an absence and not a bad request): %s", p, code, body)
			continue
		}
		if !strings.Contains(body, "content_drift") {
			t.Errorf("GET %s: %s — want the content_drift code, which is what says the platform lost the work", p, body)
		}
		// The platform's own sentence. git's stderr and this package's internal
		// vocabulary are not things a requester is shown (§30/§38).
		for _, leak := range []string{"holds no commit", "cat-file", "fatal:", "exit status", "project:"} {
			if strings.Contains(body, leak) {
				t.Errorf("GET %s leaked %q to the wire: %s", p, leak, body)
			}
		}
	}

	// THE regression this finding is named for: the files read must not quietly
	// answer with the round report when the code file it was asked for is gone.
	code, body := e.do(t, "alice", "GET", filesPath, "")
	if code == http.StatusNotFound {
		t.Error("a lost pin answered 404 — the read fell through to the revision's companion objects")
	}
	if strings.Contains(body, "report rev") {
		t.Errorf("the round report was served in place of a lost code file: %s", body)
	}
}

// TestSIT1DrainDirectoryIsNotAFile (F2): asking for a folder is a 404 in plain
// words, not git's "bad file" at 500.
func TestSIT1DrainDirectoryIsNotAFile(t *testing.T) {
	w := newSIT1World(t)
	e := w.e
	for _, dir := range []string{"src", "src/", "assets"} {
		code, body := e.do(t, "alice", "GET", sit1FilesPath("dlv-t-shop", 2, dir), "")
		if code != http.StatusNotFound {
			t.Errorf("GET files?path=%q: %d, want 404: %s", dir, code, body)
		}
		for _, leak := range []string{"cat-file", "bad file", "fatal:", "read 0 of"} {
			if strings.Contains(body, leak) {
				t.Errorf("path=%q leaked %q: %s", dir, leak, body)
			}
		}
	}
	// Non-vacuity: a real file under those directories still reads.
	if code, _ := e.do(t, "alice", "GET", sit1FilesPath("dlv-t-shop", 2, "src/app.go"), ""); code != http.StatusOK {
		t.Fatalf("src/app.go does not read, so the directory probes prove nothing: %d", code)
	}
}

// TestSIT1DrainCommentAnchorPathIsValidated (F3): an anchor's file_path is
// handed to git to read that file, so it is refused at the boundary the same
// way the files read refuses one — a 400 the caller can act on, never a 500
// from fork/exec.
func TestSIT1DrainCommentAnchorPathIsValidated(t *testing.T) {
	w := newSIT1World(t)
	e := w.e
	post := func(body string) (int, string) {
		return e.do(t, "alice", "POST", "/api/deliverables/dlv-t-shop/comments", body)
	}
	anchored := func(path string) string {
		b, err := json.Marshal(map[string]any{
			"body":       "a point about this line",
			"revision_n": 2,
			"anchor":     map[string]any{"file_path": path, "side": "new", "line_no": 1, "line_text": "package app"},
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	for _, bad := range []string{
		"src/app.go\x00",   // a NUL reached fork/exec and 500'd
		"src/app.go\n",     // a newline could forge a diff header line
		"/etc/passwd",      // absolute
		"../../etc/passwd", // steps outside the project
		"src/app.go ",      // trailing space: names a file that is not there
		" src/app.go",      // leading space, the same
		"src/\x1bapp.go",   // a control byte
	} {
		if code, body := post(anchored(bad)); code != http.StatusBadRequest {
			t.Errorf("anchor file_path %q: %d, want 400 (only the caller can fix it): %s", bad, code, body)
		}
	}
	// file_level is the same field at a coarser grain and takes the same rule.
	bad, err := json.Marshal(map[string]any{"body": "x", "revision_n": 2, "file_level": "../escape"})
	if err != nil {
		t.Fatal(err)
	}
	if code, body := post(string(bad)); code != http.StatusBadRequest {
		t.Errorf("file_level with a parent step: %d, want 400: %s", code, body)
	}
	// The control: a legitimate anchor on the same revision still lands, so the
	// refusals above are about the paths and not about the route.
	if code, body := post(anchored("src/app.go")); code != http.StatusOK {
		t.Fatalf("a valid anchored comment was refused: %d %s", code, body)
	}
}

// TestSIT1DrainNarrowingAContentPinnedComparison (F5): there is no file tree to
// narrow on a content-pinned version, and that is the caller's to fix.
func TestSIT1DrainNarrowingAContentPinnedComparison(t *testing.T) {
	w := newSIT1World(t)
	e := w.e
	e.mkRun("t-note", "r-note", "alice")
	e.mkDeliverable("dlv-t-note", "alice", "t-note", "r-note", "", "markdown",
		map[string]string{"deliverable.md": "one\n"}, "")
	e.mintNext("dlv-t-note", "r-note", 2, map[string]string{"deliverable.md": "one\ntwo\n"}, "")

	code, body := e.do(t, "alice", "GET", "/api/deliverables/dlv-t-note/compare?old=1&new=2&path=deliverable.md", "")
	if code != http.StatusBadRequest {
		t.Errorf("?path= on a content-pinned comparison: %d, want 400: %s", code, body)
	}
	// And the unnarrowed comparison of the same pair still answers.
	if code, _ := e.do(t, "alice", "GET", "/api/deliverables/dlv-t-note/compare?old=1&new=2", ""); code != http.StatusOK {
		t.Errorf("the content-pinned comparison itself must still answer: %d", code)
	}
}

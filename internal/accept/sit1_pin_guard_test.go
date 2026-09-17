package accept_test

// sit1_pin_guard_test.go — P3-SIT-1 R16, a GREEN GUARD committed by grounding.
//
// Spec S13.6: the content pin is what gets accepted, and for repo-backed work
// that pin is the snapshot commit (S13.1). P3-SIT-1 changes what a repo-backed
// revision SERVES (its tree, its inventory, its per-file diffs) and what its
// row is TYPED as; it must change nothing the accept reads. This scan pins the
// claim structurally: the accept orchestration reads a revision's
// SnapshotSHA and never its objects, its content files, its comparison or
// its tree — so the served surfaces can grow without the accept moving.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var sit1AcceptNeverReads = []string{
	"RevisionFiles(", "RevisionFile(", ".Objects", "Compare(", "CompareFile(", ".Change(", "TreeChanges", "TreeBlob", "ContentSHA256",
}

func sit1AcceptSources(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		out[name] = string(raw)
	}
	if len(out) == 0 {
		t.Fatal("no accept sources found — the scan is running in the wrong directory")
	}
	return out
}

func sit1Offenders(src string) []string {
	var hits []string
	for _, token := range sit1AcceptNeverReads {
		if strings.Contains(src, token) {
			hits = append(hits, token)
		}
	}
	return hits
}

func TestSIT1AcceptReadsOnlyTheSnapshotPin(t *testing.T) {
	srcs := sit1AcceptSources(t)
	var sawPin bool
	for name, src := range srcs {
		if strings.Contains(src, "SnapshotSHA") {
			sawPin = true
		}
		if hits := sit1Offenders(src); len(hits) > 0 {
			t.Errorf("%s reads a revision surface P3-SIT-1 changes: %v — the accept pins the snapshot commit and nothing else (Spec S13.6)", name, hits)
		}
	}
	if !sawPin {
		t.Fatal("no accept source reads SnapshotSHA — the scan would pass over a package that reads nothing")
	}
	// The probe: the scan can fail.
	if hits := sit1Offenders("rev, _ := reviews.RevisionFiles(ctx, id, n)"); len(hits) != 1 {
		t.Fatalf("planted violation not caught: %v", hits)
	}
}

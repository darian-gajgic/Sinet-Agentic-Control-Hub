package verify_test

// tq8_sliceempty_test.go — P3-TQ-8 drain round 2, the renderer's own words.
//
// N2: a file added or removed EMPTY has no text and nothing of it is missing:
// it is not "the same text with a new mode or path", it is not a cut, and it
// is not a "file with text" the judge was denied.
// N4: a diff the source could serve only in part is never shown (R3: "never a
// diff of a partially read file"); it is named with the true reason.

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

func TestTQ8AnEmptyFileAddedOrRemovedIsNotACut(t *testing.T) {
	shown := verify.ChangedFile{
		Path: "src/a.go", Kind: verify.KindAdded, NewSize: 12, Additions: 1,
		Diff: "diff --git a/src/a.go b/src/a.go\nnew file mode 100644\n--- /dev/null\n+++ b/src/a.go\n@@ -0,0 +1 @@\n+package app\n",
	}
	for _, tc := range []struct {
		name, want string
		row        verify.ChangedFile
	}{
		{"added empty", "the file was added empty", verify.ChangedFile{Path: "empty.txt", Kind: verify.KindAdded}},
		{"removed empty", "the file was empty when it was removed", verify.ChangedFile{Path: "empty.txt", Kind: verify.KindDeleted}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rc := verify.RevisionChange{OldN: 1, NewN: 2, OldPin: "s1", NewPin: "s2", Files: []verify.ChangedFile{tc.row, shown}}
			artifact, _, saw := verify.RenderChangeSlice(rc)
			if len(saw.DiffsOmitted) != 0 || saw.Truncated || saw.DiffsShown != 1 {
				t.Fatalf("shown %d omitted %v truncated %v: nothing is missing", saw.DiffsShown, saw.DiffsOmitted, saw.Truncated)
			}
			if !strings.Contains(artifact, tc.want) {
				t.Fatalf("the row does not say the file is empty:\n%s", artifact)
			}
			if strings.Contains(artifact, "mode or its path") || strings.Contains(artifact, "NOT shown") {
				t.Fatalf("an empty file is neither a mode/path change nor left out:\n%s", artifact)
			}
			if !strings.Contains(artifact, "all 1 file with text are there in full") {
				t.Fatalf("the one file with text is shown in full and the wire must say so:\n%s", artifact)
			}
		})
	}
	t.Run("only an empty file", func(t *testing.T) {
		rc := verify.RevisionChange{OldN: 1, NewN: 2, OldPin: "s1", NewPin: "s2",
			Files: []verify.ChangedFile{{Path: "empty.txt", Kind: verify.KindAdded}}}
		artifact, diff, saw := verify.RenderChangeSlice(rc)
		if saw.Truncated || diff != "" || !strings.Contains(artifact, "No file in this change has text to compare") {
			t.Fatalf("truncated %v diff %q:\n%s", saw.Truncated, diff, artifact)
		}
	})
}

func TestTQ8ADiffServedOnlyInPartIsNeverShown(t *testing.T) {
	partial := verify.ChangedFile{
		Path: "src/big.go", Kind: verify.KindModified, OldSize: 300 << 10, NewSize: 300<<10 + 5, Additions: 1,
		Diff:          "diff --git a/src/big.go b/src/big.go\n@@ -1 +1 @@\n-PARTIAL-HUNK\n",
		DiffTruncated: true, DiffReason: "the comparison is cut — open the file to read it",
	}
	after := verify.ChangedFile{
		Path: "src/z.go", Kind: verify.KindAdded, NewSize: 12, Additions: 1,
		Diff: "diff --git a/src/z.go b/src/z.go\nnew file mode 100644\n--- /dev/null\n+++ b/src/z.go\n@@ -0,0 +1 @@\n+package app\n",
	}
	rc := verify.RevisionChange{OldN: 1, NewN: 2, OldPin: "s1", NewPin: "s2", Files: []verify.ChangedFile{partial, after}}
	artifact, diff, saw := verify.RenderChangeSlice(rc)
	if strings.Contains(diff, "PARTIAL-HUNK") || saw.DiffsShown != 1 || strings.Join(saw.DiffsOmitted, ",") != "src/big.go" || !saw.Truncated {
		t.Fatalf("shown %d omitted %v truncated %v diff %q: a partial diff is never shown", saw.DiffsShown, saw.DiffsOmitted, saw.Truncated, diff)
	}
	if !strings.Contains(diff, "+package app") {
		t.Fatal("the file after it is not a cut and must still be shown")
	}
	if !strings.Contains(artifact, "too large to be compared in one piece") || strings.Contains(artifact, "only the START") {
		t.Fatalf("the omitted partial diff must say the true reason:\n%s", artifact)
	}
}

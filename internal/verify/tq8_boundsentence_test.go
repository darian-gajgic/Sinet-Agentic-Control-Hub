package verify_test

// tq8_boundsentence_test.go — P3-TQ-8 post-cap, N9. Since a content after
// the diff cut is shown (the N1 fix), the bound paragraph must not say that
// "everything after it" was left out: only the CHANGES of the files after
// the cut are, and the content section says which contents are there.

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

const tq8BoundSentence = "was left out whole, and so were the changes of every file after it."

func TestTQ8TheBoundSentenceScopesTheCutToTheDiffs(t *testing.T) {
	rc := verify.RevisionChange{OldN: 1, NewN: 2, OldPin: "s1", NewPin: "s2", Files: []verify.ChangedFile{
		{Path: "a.txt", Kind: verify.KindAdded, NewSize: 2, Additions: 1,
			Diff: "diff --git a/a.txt b/a.txt\n@@ -0,0 +1 @@\n+a\n"},
		{Path: "b.txt", Kind: verify.KindAdded, NewSize: 200 << 10, Additions: 1, BodySkipped: true},
		{Path: "src/app.go", Kind: verify.KindModified, OldSize: 3, NewSize: 4, Additions: 1, Deletions: 1,
			BodySkipped: true, Content: "app\n"},
	}}
	artifact, _, saw := verify.RenderChangeSlice(rc)
	if saw.ContentShown != 1 || strings.Join(saw.DiffsOmitted, ",") != "b.txt,src/app.go" {
		t.Fatalf("JudgeSaw = %+v", saw)
	}
	if strings.Contains(artifact, "everything after it") {
		t.Fatalf("the bound paragraph says everything after the cut is left out, yet a content after it is shown:\n%s", artifact)
	}
	if !strings.Contains(artifact, tq8BoundSentence) {
		t.Fatalf("the bound paragraph does not scope the cut to the diffs:\n%s", artifact)
	}

	// F1/F3 still hold under the new wording: no diff failed to fit, so the
	// bound sentence is not said.
	rc = verify.RevisionChange{OldN: 1, NewN: 2, OldPin: "s1", NewPin: "s2", Files: []verify.ChangedFile{
		{Path: "a.go", Kind: verify.KindModified, OldSize: 9, NewSize: 9, Additions: 1, Deletions: 1,
			Diff: "diff --git a/a.go b/a.go\n@@ -1 +1 @@\n-x\n+y\n", ContentSkipped: true},
		{Path: "run.sh", Kind: verify.KindModified, OldSize: 3, NewSize: 3, Content: "ls\n"},
	}}
	if artifact, _, _ = verify.RenderChangeSlice(rc); strings.Contains(artifact, "was left out whole") {
		t.Fatalf("the diff paragraph blames the bound although no diff failed to fit:\n%s", artifact)
	}
}

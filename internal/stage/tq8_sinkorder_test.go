package stage_test

// tq8_sinkorder_test.go — P3-TQ-8 drain round 2, at the REAL seam (the
// git-backed world of tq8_changesource_test.go).
//
// N1: after the first diff that cannot fit, the sink used to keep reading and
// CHARGING the later diffs, which the renderer then omits as past the cut
// (R3: the shown diffs are a contiguous path-order prefix). A later modified
// file's content was then skipped against the sink's remainder while the
// renderer had ~90 KB of room left, and the wire said "it did not fit the
// 192 KB" — false of what the judge reads. The judge got no body for the one
// in-place change (Spec S07.5 "the artifact + its diff"; R3 "the content ...
// under the REMAINING budget").
//
// N2: a file added EMPTY has no text: git's diff is empty, and the wire must
// say the file is empty, not that "both versions hold the same text".

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

func TestTQ8ADiffPastTheCutNeverCostsAShownContentItsPlace(t *testing.T) {
	w := newTQ8World(t)
	ctx := w.ctx
	cs, ok := stage.ChangeSourceOf(w.sk)
	if !ok {
		t.Fatal("the stage review sink does not implement verify.ChangeSource")
	}
	// Revision 3: src/app.go grows to ~25 KB. Revision 4 adds README.md and
	// three big text files ahead of it in path order and appends one line to
	// it: a.txt fits, b.txt does not, c.txt is past the cut.
	app := tq8BigText("app", 25<<10) + "\n"
	w.mintTree(map[string]string{
		"src/app.go": app, "src/cart.go": tq8Cart, "assets/logo.png": tq8PNG,
	})
	w.mintTree(map[string]string{
		"README.md":   tq8Readme,
		"a.txt":       tq8BigText("a", 100<<10),
		"b.txt":       tq8BigText("b", 100<<10),
		"c.txt":       tq8BigText("c", 70<<10),
		"src/app.go":  app + "tail()\n",
		"src/cart.go": tq8Cart, "assets/logo.png": tq8PNG,
	})

	rc, err := cs.RevisionChange(ctx, w.deliverable(4), verify.JudgeArtifactBytesCap)
	if err != nil {
		t.Fatalf("RevisionChange rev 4: %v", err)
	}
	rows := tq8Rows(rc.Files)
	if !rows["b.txt"].BodySkipped {
		t.Fatalf("b.txt cannot fit after a.txt and must be skipped: %+v", rows["b.txt"].BodySkipped)
	}
	bodies := 0
	for _, r := range rc.Files {
		bodies += len(r.Diff) + len(r.Content)
	}
	if bodies > verify.JudgeArtifactBytesCap {
		t.Fatalf("the seam served %d body bytes for a %d-byte budget", bodies, verify.JudgeArtifactBytesCap)
	}
	if a := rows["src/app.go"]; a.ContentSkipped || a.Content != app+"tail()\n" {
		t.Fatalf("src/app.go's 25 KB content fits beside the shown diffs and must be served: ContentSkipped=%v content=%d bytes",
			a.ContentSkipped, len(a.Content))
	}

	artifact, _, saw := verify.RenderChangeSlice(rc)
	if saw.DiffsShown != 2 || strings.Join(saw.DiffsOmitted, ",") != "b.txt,c.txt,src/app.go" {
		t.Fatalf("JudgeSaw diffs = shown %d omitted %v, want README.md,a.txt shown (the contiguous prefix)", saw.DiffsShown, saw.DiffsOmitted)
	}
	if saw.ContentShown != 1 || len(saw.ContentOmitted) != 0 {
		t.Fatalf("JudgeSaw content = shown %d omitted %v: the judge must get src/app.go's content", saw.ContentShown, saw.ContentOmitted)
	}
	if !strings.Contains(artifact, "tail()\n") {
		t.Fatal("src/app.go's content is not on the wire")
	}
	if saw.DiffBytes+saw.ContentBytes > verify.JudgeArtifactBytesCap {
		t.Fatalf("rendered bodies %d exceed the bound", saw.DiffBytes+saw.ContentBytes)
	}
	// Every "did not fit" on the wire is true of what the judge reads: the
	// one file said not to fit really does not fit beside what is shown.
	room := verify.JudgeArtifactBytesCap - saw.DiffBytes - saw.ContentBytes
	if n := strings.Count(artifact, "did not fit the 192 KB of file text this judge reads under"); n != 2 {
		// once on b.txt's row, once in the bound paragraph
		t.Fatalf("\"did not fit\" said %d times, want 2 (b.txt's row + the bound paragraph):\n%s", n, artifact[:min(len(artifact), 4000)])
	}
	if bLen := int(rows["b.txt"].NewSize); bLen <= room {
		t.Fatalf("b.txt (%d bytes) would fit the %d bytes left: its \"did not fit\" is false", bLen, room)
	}
}

func TestTQ8AFileAddedEmptyIsSaidToBeEmpty(t *testing.T) {
	w := newTQ8World(t)
	cs, ok := stage.ChangeSourceOf(w.sk)
	if !ok {
		t.Fatal("the stage review sink does not implement verify.ChangeSource")
	}
	w.mintTree(map[string]string{
		"src/app.go": tq8AppRev2, "src/cart.go": tq8Cart, "assets/logo.png": tq8PNG,
		"empty.txt": "",
	})
	rc, err := cs.RevisionChange(w.ctx, w.deliverable(3), verify.JudgeArtifactBytesCap)
	if err != nil {
		t.Fatalf("RevisionChange rev 3: %v", err)
	}
	e, ok := tq8Rows(rc.Files)["empty.txt"]
	if !ok || e.Kind != verify.KindAdded || e.Diff != "" || e.BodySkipped {
		t.Fatalf("empty.txt row: %+v (inventory %+v)", e, rc.Files)
	}
	artifact, _, saw := verify.RenderChangeSlice(rc)
	if len(saw.DiffsOmitted) != 0 || saw.Truncated {
		t.Fatalf("an empty added file is not a cut: omitted %v truncated %v", saw.DiffsOmitted, saw.Truncated)
	}
	if strings.Contains(artifact, "same text") || strings.Contains(artifact, "mode or its path") {
		t.Fatalf("an added empty file is not a mode or path change:\n%s", artifact)
	}
	if !strings.Contains(artifact, "the file was added empty") {
		t.Fatalf("the wire does not say the file is empty:\n%s", artifact)
	}
}

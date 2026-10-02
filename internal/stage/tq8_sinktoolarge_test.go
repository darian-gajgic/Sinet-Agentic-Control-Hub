package stage_test

// tq8_sinktoolarge_test.go — P3-TQ-8 post-cap, at the REAL seam (the
// git-backed world of tq8_changesource_test.go).
//
// N6: review serves a two-sided over-cap diff as a whole-hunk PREFIX marked
// Truncated, often well under the judge's bound. The renderer never shows a
// partial diff (it names the file "too large to be compared"), so a sink that
// served and CHARGED that prefix priced every later body against bytes the
// judge never reads: b.txt's diff and big.go's content both said "it did not
// fit the 192 KB" with the whole bound free, and the judge got no body at all
// (Spec S07.5 "the artifact + its diff"; §78 honest about the bound).
//
// N8: a one-sided over-cap add is served as a ≥256 KiB Truncated prefix that
// no order could fit. Marking it a BOUND cut lost every later diff ("already
// cut at an earlier file"), a 4-byte file included. A review-truncated diff
// is a "too large" row, never a cut.
//
// N7: the cut row's OWN content is still read and shown — the content is
// what the judge can quote of a file whose diff did not fit.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

const tq8DidNotFit = "did not fit the 192 KB of file text this judge reads under"

// tq8Lines is n numbered 68-byte lines tagged tag.
func tq8Lines(tag string, from, n int) string {
	var sb strings.Builder
	for i := from; i < from+n; i++ {
		fmt.Fprintf(&sb, "%s %06d %s\n", tag, i, strings.Repeat("y", 58))
	}
	return sb.String()
}

// tq8BigGo is a 2,410-line file in two regions split by ten unchanged lines:
// rewriting both regions makes a two-hunk diff of ~320 KB, which review can
// serve only as its first hunk (~110 KB), marked Truncated.
func tq8BigGo(p, q string) string {
	return tq8Lines(p, 0, 800) + tq8Lines("sep", 0, 10) + tq8Lines(q, 0, 1600)
}

func TestTQ8AReviewTruncatedDiffIsNeverChargedNorACut(t *testing.T) {
	w := newTQ8World(t)
	cs, ok := stage.ChangeSourceOf(w.sk)
	if !ok {
		t.Fatal("the stage review sink does not implement verify.ChangeSource")
	}
	w.mintTree(map[string]string{
		"a/big.go": tq8BigGo("p", "q"), "src/app.go": tq8AppRev2, "src/cart.go": tq8Cart, "assets/logo.png": tq8PNG,
	})
	newBig := tq8BigGo("P", "Q")
	w.mintTree(map[string]string{
		"a/big.go": newBig, "b.txt": tq8BigText("b", 100<<10),
		"src/app.go": tq8AppRev2, "src/cart.go": tq8Cart, "assets/logo.png": tq8PNG,
	})
	rc, err := cs.RevisionChange(w.ctx, w.deliverable(4), verify.JudgeArtifactBytesCap)
	if err != nil {
		t.Fatalf("RevisionChange rev 4: %v", err)
	}
	rows := tq8Rows(rc.Files)
	big, b := rows["a/big.go"], rows["b.txt"]
	if !big.DiffTruncated || big.Diff != "" || big.BodySkipped {
		t.Fatalf("a/big.go: review can compare it only in part, so it is a too-large row, never served nor cut: DiffTruncated=%v diff=%d bytes BodySkipped=%v",
			big.DiffTruncated, len(big.Diff), big.BodySkipped)
	}
	if b.BodySkipped || !strings.Contains(b.Diff, "+b ") {
		t.Fatalf("b.txt's 104 KB diff fits the whole free bound and must be served: BodySkipped=%v diff=%d bytes", b.BodySkipped, len(b.Diff))
	}

	artifact, diff, saw := verify.RenderChangeSlice(rc)
	if saw.DiffsShown != 1 || strings.Join(saw.DiffsOmitted, ",") != "a/big.go" {
		t.Fatalf("JudgeSaw diffs = shown %d omitted %v, want b.txt shown and a/big.go named", saw.DiffsShown, saw.DiffsOmitted)
	}
	if !strings.Contains(diff, "+b ") {
		t.Fatal("b.txt's diff is not in the diff item")
	}
	if !strings.Contains(artifact, "too large to be compared in one piece") {
		t.Fatalf("a/big.go is not named too large to compare:\n%s", artifact[:min(len(artifact), 4000)])
	}
	// "did not fit" only where true: big.go's whole content (~164 KB) cannot
	// fit beside b.txt's shown diff; nothing else is said not to fit.
	room := verify.JudgeArtifactBytesCap - saw.DiffBytes - saw.ContentBytes
	if strings.Join(saw.ContentOmitted, ",") != "a/big.go" || len(newBig) <= room {
		t.Fatalf("content omitted %v, a/big.go %d bytes against %d left", saw.ContentOmitted, len(newBig), room)
	}
	if n := strings.Count(artifact, tq8DidNotFit); n != 1 {
		t.Fatalf("\"did not fit\" said %d times, want once (a/big.go's content row):\n%s", n, artifact[:min(len(artifact), 4000)])
	}
}

func TestTQ8AnOverCapAddIsTooLargeNotACut(t *testing.T) {
	w := newTQ8World(t)
	cs, ok := stage.ChangeSourceOf(w.sk)
	if !ok {
		t.Fatal("the stage review sink does not implement verify.ChangeSource")
	}
	w.mintTree(map[string]string{
		"huge.txt": tq8BigText("h", 300<<10), "z.txt": "zzz\n",
		"src/app.go": tq8AppRev2, "src/cart.go": tq8Cart, "assets/logo.png": tq8PNG,
	})
	rc, err := cs.RevisionChange(w.ctx, w.deliverable(3), verify.JudgeArtifactBytesCap)
	if err != nil {
		t.Fatalf("RevisionChange rev 3: %v", err)
	}
	rows := tq8Rows(rc.Files)
	if h := rows["huge.txt"]; !h.DiffTruncated || h.Diff != "" || h.BodySkipped {
		t.Fatalf("huge.txt: DiffTruncated=%v diff=%d bytes BodySkipped=%v, want a too-large row", h.DiffTruncated, len(h.Diff), h.BodySkipped)
	}
	artifact, diff, saw := verify.RenderChangeSlice(rc)
	if saw.DiffsShown != 1 || strings.Join(saw.DiffsOmitted, ",") != "huge.txt" || !strings.Contains(diff, "+zzz") {
		t.Fatalf("JudgeSaw diffs = shown %d omitted %v; z.txt's diff must be shown", saw.DiffsShown, saw.DiffsOmitted)
	}
	if strings.Contains(artifact, tq8DidNotFit) || strings.Contains(artifact, "already cut") {
		t.Fatalf("no body was cut by the bound, yet the wire says so:\n%s", artifact)
	}
	if !strings.Contains(artifact, "too large to be compared in one piece") {
		t.Fatalf("huge.txt is not named too large to compare:\n%s", artifact)
	}
}

func TestTQ8TheCutRowsOwnContentIsStillShown(t *testing.T) {
	w := newTQ8World(t)
	cs, ok := stage.ChangeSourceOf(w.sk)
	if !ok {
		t.Fatal("the stage review sink does not implement verify.ChangeSource")
	}
	w.mintTree(map[string]string{
		"src/app.go": tq8BigText("old", 25<<10), "src/cart.go": tq8Cart, "assets/logo.png": tq8PNG,
	})
	newApp := tq8BigText("new", 10<<10)
	w.mintTree(map[string]string{
		"a.txt": tq8BigText("a", 160<<10), "src/app.go": newApp, "src/cart.go": tq8Cart, "assets/logo.png": tq8PNG,
	})
	rc, err := cs.RevisionChange(w.ctx, w.deliverable(4), verify.JudgeArtifactBytesCap)
	if err != nil {
		t.Fatalf("RevisionChange rev 4: %v", err)
	}
	app := tq8Rows(rc.Files)["src/app.go"]
	if !app.BodySkipped {
		t.Fatalf("src/app.go's ~35 KB diff cannot fit after a.txt's 160 KB and must be the cut: %+v", app.BodySkipped)
	}
	if app.ContentSkipped || app.Content != newApp {
		t.Fatalf("the cut row's own 10 KB content fits and must be served: ContentSkipped=%v content=%d bytes", app.ContentSkipped, len(app.Content))
	}
	artifact, _, saw := verify.RenderChangeSlice(rc)
	if saw.ContentShown != 1 || len(saw.ContentOmitted) != 0 || !strings.Contains(artifact, newApp) {
		t.Fatalf("JudgeSaw content = shown %d omitted %v: src/app.go's content must be on the wire", saw.ContentShown, saw.ContentOmitted)
	}
}

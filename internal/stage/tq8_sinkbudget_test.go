package stage_test

// tq8_sinkbudget_test.go — P3-TQ-8 drain round 1, R2's body budget at the
// REAL seam: "body reads stop once bodyBudget bytes are read (rows past it
// keep their inventory data and carry BodySkipped); the inventory is always
// whole". The acceptance battery drove only changes small enough that the
// budget never bit, so the rule it rests on was unpinned (F2) — and the rule
// as first written spent the budget on a body the renderer then omitted,
// which cost every LATER file its bodies for nothing (F3).
//
// The world is the same REAL one as tq8_changesource_test.go: a project
// onboarded from a git fixture, platform snapshot pins, revisions minted on
// them, the review store reading the project store's trees.

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

const tq8AppRev3 = "package app\n\nfunc Run() {\n\tserve()\n\tlisten()\n\tclose()\n}\n"

// tq8BigText is n bytes of line-shaped text: a file big enough that its
// unified diff alone is a sizeable share of the judge's whole bound.
func tq8BigText(tag string, n int) string {
	line := tag + " " + strings.Repeat("x", 60) + "\n"
	var sb strings.Builder
	for sb.Len() < n {
		sb.WriteString(line)
	}
	return sb.String()[:n]
}

// TestTQ8TheSinkStopsAtTheBodyBudgetAndKeepsReadingPastWhatCannotFit — F2/F3.
// Revision 3 adds two ~100 KB files around a tiny modification: their two
// diffs together are more than the 192 KB of bodies the judge reads under, so
// the second one cannot be served. What the seam must do is stop spending the
// budget on THAT file — not on the rest of the change: the small modified
// file after it still reaches the judge, which is what the bound was sized
// for (Spec S05.3; R2/R3; §78 bounded and honest about the bound).
func TestTQ8TheSinkStopsAtTheBodyBudgetAndKeepsReadingPastWhatCannotFit(t *testing.T) {
	w := newTQ8World(t)
	ctx := w.ctx
	cs, ok := stage.ChangeSourceOf(w.sk)
	if !ok {
		t.Fatal("the stage review sink does not implement verify.ChangeSource")
	}
	// Revision 3: README.md back, two ~100 KB files added, src/app.go touched.
	// In path order: README.md, a.txt, b.txt, src/app.go.
	w.mintTree(map[string]string{
		"README.md":       tq8Readme,
		"a.txt":           tq8BigText("a", 100<<10),
		"b.txt":           tq8BigText("b", 100<<10),
		"src/app.go":      tq8AppRev3,
		"src/cart.go":     tq8Cart,
		"assets/logo.png": tq8PNG,
	})

	rc, err := cs.RevisionChange(ctx, w.deliverable(3), verify.JudgeArtifactBytesCap)
	if err != nil {
		t.Fatalf("RevisionChange rev 3: %v", err)
	}
	rows := tq8Rows(rc.Files)
	if len(rows) != 4 {
		t.Fatalf("rev 3 inventory: %+v", rc.Files)
	}
	// The inventory is whole whatever the budget does (R2).
	for _, p := range []string{"README.md", "a.txt", "b.txt", "src/app.go"} {
		if _, ok := rows[p]; !ok {
			t.Fatalf("inventory row %q missing: %+v", p, rc.Files)
		}
	}
	bodies := 0
	for _, r := range rc.Files {
		bodies += len(r.Diff) + len(r.Content)
	}
	if bodies > verify.JudgeArtifactBytesCap {
		t.Fatalf("the seam read %d body bytes for a %d-byte budget — body reads must STOP at the budget (R2)",
			bodies, verify.JudgeArtifactBytesCap)
	}
	if !strings.Contains(rows["a.txt"].Diff, "+a ") {
		t.Fatalf("the first big file fits and must be served: %d diff bytes", len(rows["a.txt"].Diff))
	}
	if !rows["b.txt"].BodySkipped || rows["b.txt"].Diff != "" {
		t.Fatalf("the second big file cannot fit what is left and must be skipped whole: BodySkipped=%v diff=%d bytes",
			rows["b.txt"].BodySkipped, len(rows["b.txt"].Diff))
	}
	// F3: the budget was not spent on the body that could not fit, so the
	// small modified file after it is still read — diff AND content.
	app := rows["src/app.go"]
	if app.BodySkipped || !strings.Contains(app.Diff, "+\tclose()") || app.Content != tq8AppRev3 {
		t.Fatalf("the small modified file after a skipped body must still be read: %+v", app)
	}

	// End to end through the renderer: the judge is shown that file's content,
	// and every file it did not see is named.
	artifact, _, saw := verify.RenderChangeSlice(rc)
	if !strings.Contains(artifact, tq8AppRev3) {
		t.Fatalf("src/app.go's content fits the bound and must reach the judge:\n%s", artifact)
	}
	if saw.DiffsShown != 2 || strings.Join(saw.DiffsOmitted, ",") != "b.txt,src/app.go" {
		t.Fatalf("JudgeSaw diffs = shown %d omitted %v, want 2 shown and b.txt,src/app.go omitted", saw.DiffsShown, saw.DiffsOmitted)
	}
	if saw.ContentShown != 1 || len(saw.ContentOmitted) != 0 || !saw.Truncated {
		t.Fatalf("JudgeSaw content = shown %d omitted %v truncated %v, want the small file's content shown and the cut recorded",
			saw.ContentShown, saw.ContentOmitted, saw.Truncated)
	}
	if saw.DiffBytes+saw.ContentBytes > verify.JudgeArtifactBytesCap {
		t.Fatalf("rendered bodies %d exceed the bound %d", saw.DiffBytes+saw.ContentBytes, verify.JudgeArtifactBytesCap)
	}
	for _, p := range saw.DiffsOmitted {
		if n := strings.Count(artifact, p); n < 2 {
			t.Fatalf("%q appears %d times in the artifact, want ≥ 2 (inventory row + named as omitted)", p, n)
		}
	}

	t.Run("a budget no body fits leaves the whole inventory and no bodies", func(t *testing.T) {
		rc, err := cs.RevisionChange(ctx, w.deliverable(3), 1)
		if err != nil {
			t.Fatalf("RevisionChange: %v", err)
		}
		if len(rc.Files) != 4 {
			t.Fatalf("the inventory is never bounded: %+v", rc.Files)
		}
		for _, r := range rc.Files {
			if r.Binary {
				continue
			}
			if !r.BodySkipped || r.Diff != "" || r.Content != "" {
				t.Fatalf("%s: no body fits a 1-byte budget: %+v", r.Path, r)
			}
		}
	})
}

// TestTQ8AContentThatCannotFitNeverCostsItsFileTheDiff — F3 refinement at the
// real seam. A big file changed by one line has a SMALL diff and a BIG new
// content: the diff fits, the content does not. Only the content may be left
// out. Marking the whole row skipped would drop a diff that had been read
// and fits, and with it every later diff, so the judge would lose the one
// hunk it was asked to judge (Spec S07.5 "the artifact + its diff"; R2/R3).
func TestTQ8AContentThatCannotFitNeverCostsItsFileTheDiff(t *testing.T) {
	w := newTQ8World(t)
	ctx := w.ctx
	cs, ok := stage.ChangeSourceOf(w.sk)
	if !ok {
		t.Fatal("the stage review sink does not implement verify.ChangeSource")
	}
	big := tq8BigText("g", 150<<10) + "\n"
	// Revision 3 brings in the big file; revision 4 changes it by one line,
	// adds a 60 KB file before it and a small file after it in path order.
	w.mintTree(map[string]string{
		"src/app.go": tq8AppRev2, "src/cart.go": tq8Cart, "assets/logo.png": tq8PNG,
		"src/big.go": big,
	})
	const small = "package app\n\nfunc Z() {}\n"
	w.mintTree(map[string]string{
		"src/app.go": tq8AppRev2, "src/cart.go": tq8Cart, "assets/logo.png": tq8PNG,
		"src/big.go": big + "tail\n",
		"a.txt":      tq8BigText("a", 60<<10),
		"src/z.go":   small,
	})

	rc, err := cs.RevisionChange(ctx, w.deliverable(4), verify.JudgeArtifactBytesCap)
	if err != nil {
		t.Fatalf("RevisionChange rev 4: %v", err)
	}
	rows := tq8Rows(rc.Files)
	g := rows["src/big.go"]
	if g.BodySkipped || !strings.Contains(g.Diff, "+tail") {
		t.Fatalf("src/big.go's one-line diff fits and must be served: BodySkipped=%v diff=%d bytes", g.BodySkipped, len(g.Diff))
	}
	if !g.ContentSkipped || g.Content != "" {
		t.Fatalf("src/big.go's 150 KB content cannot fit what is left and must be skipped: ContentSkipped=%v content=%d bytes",
			g.ContentSkipped, len(g.Content))
	}

	artifact, diff, saw := verify.RenderChangeSlice(rc)
	if saw.DiffsShown != 3 || len(saw.DiffsOmitted) != 0 {
		t.Fatalf("JudgeSaw diffs = shown %d omitted %v, want all 3 diffs shown", saw.DiffsShown, saw.DiffsOmitted)
	}
	if !strings.Contains(diff, "+tail") || !strings.Contains(diff, "+func Z() {}") {
		t.Fatal("the diff item lost a diff that was served and fits")
	}
	if saw.ContentShown != 0 || strings.Join(saw.ContentOmitted, ",") != "src/big.go" || !saw.Truncated {
		t.Fatalf("JudgeSaw content = shown %d omitted %v truncated %v, want src/big.go's content named omitted",
			saw.ContentShown, saw.ContentOmitted, saw.Truncated)
	}
	if !strings.Contains(artifact, "Content NOT shown, by file: src/big.go") {
		t.Fatalf("the omitted content is not named on the wire:\n%s", artifact[:min(len(artifact), 3000)])
	}
}

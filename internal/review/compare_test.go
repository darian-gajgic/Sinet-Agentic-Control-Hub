package review_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
)

// Per-type comparison behavior (Spec S13.2): the PDF extracted-text
// surface, the image two-revision surface with the optional pixel-diff
// seam, and the labeled fallbacks. Line diff and binary cards are covered
// by store_test; NO type refused is proven across all of them.

func TestPDFExtractedTextDiff(t *testing.T) {
	f := newFix(t)
	f.task("tp", "u1")
	f.run("rp", "tp")
	ctx := context.Background()
	if _, err := f.store.EnsureDeliverable(ctx, review.EnsureInput{
		ID: "dlv-tp", Owner: "u1", TaskID: "tp", Type: "pdf",
	}); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	mint := func(n int, text string) {
		t.Helper()
		if _, err := f.store.MintRevision(ctx, review.MintInput{
			DeliverableID: "dlv-tp", N: n, RunID: "rp", AttemptRef: "rp#round-1",
			Objects: map[string][]byte{"report.pdf": minimalPDF(text)},
			Types:   map[string]string{"report.pdf": "application/pdf"},
		}); err != nil {
			t.Fatalf("mint pdf %d: %v", n, err)
		}
	}
	mint(1, "The quarterly total is 40")
	mint(2, "The quarterly total is 42")

	cmp, err := f.store.Compare(ctx, "dlv-tp", 1, 2)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if cmp.Surface != review.SurfaceExtractedText || cmp.Label == "" {
		t.Fatalf("pdf surface labeled: %+v", cmp)
	}
	if !strings.Contains(cmp.Unified, "-The quarterly total is 40") ||
		!strings.Contains(cmp.Unified, "+The quarterly total is 42") {
		t.Fatalf("extracted-text diff:\n%s", cmp.Unified)
	}
}

func TestPDFExtractionFailureDegradesHonestly(t *testing.T) {
	f := newFix(t)
	f.task("tq", "u1")
	f.run("rq", "tq")
	ctx := context.Background()
	if _, err := f.store.EnsureDeliverable(ctx, review.EnsureInput{
		ID: "dlv-tq", Owner: "u1", TaskID: "tq", Type: "pdf",
	}); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	mint := func(n int, data []byte) {
		t.Helper()
		if _, err := f.store.MintRevision(ctx, review.MintInput{
			DeliverableID: "dlv-tq", N: n, RunID: "rq", AttemptRef: "rq#round-1",
			Objects: map[string][]byte{"broken.pdf": data},
		}); err != nil {
			t.Fatalf("mint %d: %v", n, err)
		}
	}
	mint(1, []byte("%PDF-1.4 this is not a real pdf"))
	mint(2, []byte("%PDF-1.4 still not a real pdf"))

	cmp, err := f.store.Compare(ctx, "dlv-tq", 1, 2)
	if err != nil {
		t.Fatalf("no type is refused (Spec S13.2): %v", err)
	}
	if cmp.Surface != review.SurfaceBinaryCards || !cmp.Fallback || cmp.Label == "" {
		t.Fatalf("labeled metadata-card degrade: %+v", cmp)
	}
	if !cmp.Changed {
		t.Fatalf("hash verdict still works: %+v", cmp)
	}
}

type fakePixelDiff struct{ calls int }

func (p *fakePixelDiff) PixelDiff(_ context.Context, oldPath, newPath string) (review.PixelDiffResult, error) {
	p.calls++
	return review.PixelDiffResult{ChangedRatio: 0.25}, nil
}

func TestImagePairSurfaceAndPixelDiffSeam(t *testing.T) {
	f := newFix(t)
	f.task("ti", "u1")
	f.run("ri", "ti")
	ctx := context.Background()
	if _, err := f.store.EnsureDeliverable(ctx, review.EnsureInput{
		ID: "dlv-ti", Owner: "u1", TaskID: "ti", Type: "image",
	}); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	mint := func(n int, payload string) {
		t.Helper()
		if _, err := f.store.MintRevision(ctx, review.MintInput{
			DeliverableID: "dlv-ti", N: n, RunID: "ri", AttemptRef: "ri#round-1",
			Objects: map[string][]byte{"logo.png": []byte(payload)},
			Types:   map[string]string{"logo.png": "image/png"},
		}); err != nil {
			t.Fatalf("mint image %d: %v", n, err)
		}
	}
	mint(1, "PNG-A")
	mint(2, "PNG-B")

	// Without the seam: the two-revision surface alone (the S15 trio's
	// data), no pixel diff.
	cmp, err := f.store.Compare(ctx, "dlv-ti", 1, 2)
	if err != nil || cmp.Surface != review.SurfaceImagePair || !cmp.Changed || cmp.PixelDiff != nil {
		t.Fatalf("image pair surface: %+v err %v", cmp, err)
	}
	if len(cmp.OldObjects) != 1 || len(cmp.NewObjects) != 1 {
		t.Fatalf("two-revision surface refs: %+v", cmp)
	}

	// With the seam wired, the aid attaches.
	aid := &fakePixelDiff{}
	f.store.PixelDiff = aid
	cmp, err = f.store.Compare(ctx, "dlv-ti", 1, 2)
	if err != nil || cmp.PixelDiff == nil || cmp.PixelDiff.ChangedRatio != 0.25 || aid.calls != 1 {
		t.Fatalf("pixel-diff aid: %+v calls %d err %v", cmp, aid.calls, err)
	}
}

func TestUnknownTypeGetsLabeledFallback(t *testing.T) {
	f := newFix(t)
	f.task("tu", "u1")
	f.run("ru", "tu")
	ctx := context.Background()
	// A type nobody special-cased: content-pinned → labeled fallback text
	// diff; NO refusal path exists.
	if _, err := f.store.EnsureDeliverable(ctx, review.EnsureInput{
		ID: "dlv-tu", Owner: "u1", TaskID: "tu", Type: "spreadsheet-csv",
	}); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	mint := func(n int, body string) {
		t.Helper()
		if _, err := f.store.MintRevision(ctx, review.MintInput{
			DeliverableID: "dlv-tu", N: n, RunID: "ru", AttemptRef: "ru#round-1",
			Files: map[string]string{"data.csv": body},
		}); err != nil {
			t.Fatalf("mint %d: %v", n, err)
		}
	}
	mint(1, "a,b\n1,2\n")
	mint(2, "a,b\n1,3\n")

	cmp, err := f.store.Compare(ctx, "dlv-tu", 1, 2)
	if err != nil {
		t.Fatalf("no type refused: %v", err)
	}
	if cmp.Surface != review.SurfaceExtractedText || !cmp.Fallback || !strings.Contains(cmp.Label, "fallback") {
		t.Fatalf("labeled fallback: %+v", cmp)
	}
	if !strings.Contains(cmp.Unified, "+1,3") {
		t.Fatalf("fallback still diffs:\n%s", cmp.Unified)
	}
}

func TestEscapeFirstContractIsData(t *testing.T) {
	c := review.EscapeFirst()
	if !c.EscapedTextFirst {
		t.Fatalf("escape-first is the contract")
	}
	if len(c.SanctionedRawHTML) != 1 {
		t.Fatalf("exactly ONE sanctioned raw-HTML channel exists (FC-v1 §2): %v", c.SanctionedRawHTML)
	}
}

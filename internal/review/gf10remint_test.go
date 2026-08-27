package review_test

import (
	"context"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// gf10remint_test.go — P3-GF10 property P2 (R8), written RED before the
// implementation. Brief: P3/briefs/P3-GF10.md §4 R8.
//
// The S07.7 resume re-enters on the pinned revision and re-mints it, and the
// producing run it offers is resolved fresh — so it can name a lineage tip that
// advanced since the first mint (a recovery fork). Which attempt produced
// revision N is a per-revision fact FIXED when the content was frozen: the
// re-mint must stay the no-op it is, keeping the first recorded producer rather
// than erroring on the newer id or rewriting the row with it.

// TestGF10ReMintKeepsTheFirstProducingRun mints revision 1 with one producing
// run, then re-mints the SAME content with an advanced lineage tip.
//
// RED until GF10 lands: ProducedBy is inert type surface today, so the first
// mint records no producer at all.
func TestGF10ReMintKeepsTheFirstProducingRun(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.seed()

	mint := func(producer string) (review.Revision, error) {
		return f.store.MintRevision(ctx, review.MintInput{
			DeliverableID: "dlv-t1", N: 1, RunID: "r1", AttemptRef: "r1#round-1",
			ProducedBy: producer,
			Files:      map[string]string{"deliverable.md": "the frozen content\n"},
		})
	}

	first, err := mint("t1.execute")
	if err != nil {
		t.Fatalf("MintRevision: %v", err)
	}
	if first.ProducedBy != "t1.execute" {
		t.Errorf("the mint must record the producing run, got %q", first.ProducedBy)
	}

	// The lineage advanced (a recovery fork) between the mint and the resume.
	again, err := mint("t1.execute.fork-1")
	if err != nil {
		t.Fatalf("the idempotent re-mint of identical content must stay a no-op, got: %v", err)
	}
	if again.ProducedBy != "t1.execute" {
		t.Errorf("first-write-wins: the re-mint returned producer %q, want the recorded %q", again.ProducedBy, "t1.execute")
	}

	// The ROW is what the accept resolves through, so read it back rather than
	// trusting the returned value.
	read, err := f.store.RevisionAt(ctx, "dlv-t1", 1)
	if err != nil {
		t.Fatalf("RevisionAt: %v", err)
	}
	if read.ProducedBy != "t1.execute" {
		t.Errorf("the recorded producing run was rewritten to %q, want %q", read.ProducedBy, "t1.execute")
	}
	if read.RunID != "r1" {
		t.Errorf("the minting run must be untouched by the new member, got %q", read.RunID)
	}
	// One revision, one mint event: the no-op minted nothing a second time.
	if n := len(f.events(review.EventMinted)); n != 1 {
		t.Errorf("the re-mint appended a second minted event (%d total)", n)
	}
}

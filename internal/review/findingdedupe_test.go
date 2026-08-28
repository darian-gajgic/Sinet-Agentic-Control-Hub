package review_test

// P3-RW-18 D3 acceptance tests — committed RED with the brief (Amendment-A;
// red window closed by the packet's implementation commit).
//
// Walk-proven defect: verify filed the identical finding twice — same text,
// same nanosecond created_ts (comment_id 8/9 on dlv-t-1e211253dfa21c28: two
// research nodes share step S-1, the note text names only the step, and
// neither validateFindings nor AddFindings dedupes within one batch, so one
// RecordFindings call inserted the same row twice under one `now`). A render-
// side dedupe masked one surface; the other showed both. The dedupe belongs at
// MINT: AddFindings is idempotent over byte-identical findings — within the
// batch, and against already-filed open rows of the same revision.

import (
	"context"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

func countComments(f *fix, deliverableID string, revisionN int) int {
	f.t.Helper()
	var n int
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM review_comments WHERE deliverable_id = ? AND revision_n = ?`,
		deliverableID, revisionN).Scan(&n); err != nil {
		f.t.Fatalf("count comments: %v", err)
	}
	return n
}

// TestIdenticalFindingsInOneBatchFileOnce — the walk pair, reproduced: one
// AddFindings batch carrying the same finding twice files ONE durable row.
func TestIdenticalFindingsInOneBatchFileOnce(t *testing.T) {
	f := newFix(t)
	f.seed()
	f.mint(1, "the shipping policy\n")

	dup := review.FindingInput{
		Author: "u1", RunID: "r1", Severity: review.SeverityNote,
		Category:  "RESEARCH-NOT-RUN",
		Body:      "There is no record proving step S-1 actually did its research on prices & costs: the platform does not yet keep a per-step record of what was looked up, so this could not be checked either way — recorded, not silently passed",
		RawAnchor: "step:S-1",
	}
	if _, err := f.store.AddFindings(context.Background(), "dlv-t1", 1, []review.FindingInput{dup, dup}); err != nil {
		t.Fatalf("AddFindings: %v", err)
	}
	if n := countComments(f, "dlv-t1", 1); n != 1 {
		t.Fatalf("one batch with a byte-identical pair filed %d rows, want 1 — the walk-proven double-file (RW-18 D3; S07.6)", n)
	}
}

// TestRefilingTheSameFindingIsIdempotent — a re-entered drain leg re-filing the
// same round's findings must not double the record: the mint-level dedupe also
// holds against already-filed rows of the same revision.
func TestRefilingTheSameFindingIsIdempotent(t *testing.T) {
	f := newFix(t)
	f.seed()
	f.mint(1, "the shipping policy\n")

	one := review.FindingInput{
		Author: "u1", RunID: "r1", Severity: review.SeverityNote,
		Category:  "RESEARCH-NOT-RUN",
		Body:      "There is no record proving step S-1 actually did its research on prices & costs: the platform does not yet keep a per-step record of what was looked up, so this could not be checked either way — recorded, not silently passed",
		RawAnchor: "step:S-1",
	}
	if _, err := f.store.AddFindings(context.Background(), "dlv-t1", 1, []review.FindingInput{one}); err != nil {
		t.Fatalf("AddFindings first: %v", err)
	}
	if _, err := f.store.AddFindings(context.Background(), "dlv-t1", 1, []review.FindingInput{one}); err != nil {
		t.Fatalf("AddFindings second: %v", err)
	}
	if n := countComments(f, "dlv-t1", 1); n != 1 {
		t.Fatalf("re-filing the identical finding produced %d rows, want 1", n)
	}
}

// TestDistinctFindingsAllFile — the dedupe is byte-identity, never similarity:
// two findings differing in any recorded field are two facts and both file.
// (The walk pair SHOULD have been distinct — two different research rules on
// one step — which is D3's companion fix in internal/verify: the note text
// names the rule, so honest distinct facts stop colliding into identity.)
func TestDistinctFindingsAllFile(t *testing.T) {
	f := newFix(t)
	f.seed()
	f.mint(1, "the shipping policy\n")

	a := review.FindingInput{
		Author: "u1", RunID: "r1", Severity: review.SeverityNote,
		Category:  "RESEARCH-NOT-RUN",
		Body:      "There is no record proving step S-1 actually did its research on prices & costs: no per-step record",
		RawAnchor: "step:S-1",
	}
	b := a
	b.Body = "There is no record proving step S-1 actually did its research on locality cues: no per-step record"
	if _, err := f.store.AddFindings(context.Background(), "dlv-t1", 1, []review.FindingInput{a, b}); err != nil {
		t.Fatalf("AddFindings: %v", err)
	}
	if n := countComments(f, "dlv-t1", 1); n != 2 {
		t.Fatalf("two distinct findings filed %d rows, want 2 — dedupe must be identity, not similarity", n)
	}
}

package verify

import (
	"context"
	"fmt"
)

// ReviewSink is the S13 review-schema seam (B4): the verification handoff
// that mints revisions (Spec S13.1 "a revision is minted when a candidate
// passes to review — one per round"), the landing point for a judged
// round's findings as durable review comments (Spec S07.6 "notes ride
// along to the requester as review comments"; one schema with human
// comments, Spec S13.1), the S07.7 guidance ingress, and THE S13.4 drain
// that composes the retry package's numbered findings.
//
// NIL IS THE PRE-S13 POSTURE (dev/test): findings stay in-memory and the
// retry package carries the round's blockers exactly as B2-3 shipped it.
// The composition root always wires the sink (stage adapts
// review.Store); with it wired, the package findings come from the ONE
// drain path — every open comment collected, numbered [F1..Fn],
// batch-consumed to the attempt, severity carried (notes travel; blockers
// alone trigger rounds).
type ReviewSink interface {
	// MintCandidate mints the round's candidate revision at the
	// verification handoff (idempotent for the resume path's re-entry on
	// the pinned revision). The minting attempt ref is MintRef(run, round).
	MintCandidate(ctx context.Context, d Deliverable, round int) error
	// RecordFindings lands a judged round's findings as open review
	// comments against the candidate revision. The caller passes
	// deliverable-defect findings only — CHECK-INTEGRITY findings are
	// suite defects whose sink is the decision card + quarantine, never
	// the review stream (CONVENTIONS §15) — plus the bootstrap posture
	// disclosure, the one integrity-category finding that is a statement to
	// the requester rather than a suite defect (see reviewable).
	RecordFindings(ctx context.Context, d Deliverable, findings []Finding) error
	// RecordVerdict fills the revision's verification-verdict ref with the
	// round's verify.round event seq (fill-once; Spec S13.1).
	RecordVerdict(ctx context.Context, d Deliverable, eventSeq int64) error
	// RecordGuidance lands revise_with_guidance points as open blocker
	// comments from the requester (the 6.2 ingress; Spec S07.6 "requester
	// comments enter the retry through the same numbered-anchored-point
	// channel").
	RecordGuidance(ctx context.Context, d Deliverable, author string, comments []RequesterComment) error
	// DrainOpen executes the S13.4 drain for the next attempt and returns
	// the numbered points (anchor/degradation + severity + description +
	// optional suggested change), orphaned and file-degraded findings
	// included — delivery is never conditional on anchoring (P-T12-2).
	DrainOpen(ctx context.Context, d Deliverable, attemptRef string) ([]Finding, error)
}

// AttemptRef names the rework attempt a drained batch feeds — the
// consuming-attempt ref of the S13.3 batch stamp, matching the stage
// session identity of the rework round (`revise-r<n>` on the verify run).
func AttemptRef(runID string, round int) string {
	return fmt.Sprintf("%s#revise-r%d", runID, round)
}

// MintRef names the verification-handoff attempt that minted a round's
// candidate revision (Spec S13.1 "minting run/attempt ref").
func MintRef(runID string, round int) string {
	return fmt.Sprintf("%s#round-%d", runID, round)
}

// reviewable filters the findings that enter the review stream: every
// deliverable-defect finding, any severity (notes ride to the requester),
// EXCLUDING check-integrity — a suite defect routes to its card +
// quarantine and never re-enters the deliverable's rework channel
// (Spec S07.7; CONVENTIONS §15).
//
// The bootstrap posture disclosure is the one exception, admitted by its
// stable identity and never by its category (isPostureDisclosure): it is not
// a suite defect but the requester's answer to "why was nothing checked", and
// the review surface is where the mandatory V3 decision it exists for is
// actually made (Spec S07.8). Quarantine skips and runner failures still stay
// out.
func reviewable(fs []Finding) []Finding {
	out := make([]Finding, 0, len(fs))
	for _, f := range fs {
		if f.Category == CatCheckIntegrity && !isPostureDisclosure(f) {
			continue
		}
		out = append(out, f)
	}
	return out
}

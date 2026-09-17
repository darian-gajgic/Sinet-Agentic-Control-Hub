package stage

import (
	"context"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// export_test.go — test-only surface. The per-duty length caps stay unexported
// structural constants (S18 ratifies no ⚙ key); the live tripwire needs the
// phrase cap to assert the real call lands UNDER it with headroom, and reading
// the number from the same constant the caller sends is what makes that
// assertion honest (PH-1 F4).

// PhraseMaxTokens is the phrase duty's length cap, exposed to the live leg.
const PhraseMaxTokens = phraseMaxTokens

// HelpMaxTokens is the 13.5 help drafting cap, exposed to the live leg.
const HelpMaxTokens = helpMaxTokens

// ChangeSourceOf exposes the skeleton's review sink as the verify.ChangeSource
// seam (P3-TQ-8: the judge's tree slice is read through the one S13 adapter
// that owns the task → deliverable identity mapping). ok=false when the sink
// does not implement the seam.
func ChangeSourceOf(s *Skeleton) (verify.ChangeSource, bool) {
	cs, ok := any(reviewSink{s: s}).(verify.ChangeSource)
	return cs, ok
}

// VerifierChangeSeamWired reports whether newVerifier composes the Change seam
// beside the review sink — the wiring pinned by a test that drives the real
// composition (the §78 precedent).
func VerifierChangeSeamWired(ctx context.Context, s *Skeleton, domain, taskID string) (bool, error) {
	v, err := s.newVerifier(ctx, domain, taskID)
	if err != nil {
		return false, err
	}
	return v.Change != nil, nil
}

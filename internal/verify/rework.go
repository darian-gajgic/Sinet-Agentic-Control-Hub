package verify

import (
	"fmt"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
)

// Bounded rework (Spec S07.6): only blockers trigger another round; notes
// ride along to the requester as review comments (Spec S13) and never spin
// loops. Each round's executor is a FRESH session built from the retry
// package (fork-don't-poison); re-review judges against the ORIGINAL
// criteria — same frozen ACs, same rubric version, prior findings in scope.
// No judge-every-round continuation gating exists: the smart stopper costs
// 2.3× tokens for zero quality gain.

// RetryPackage is EXACTLY the ratified retry input (Spec S07.6): the
// original SPEC + the frozen ACs + the numbered findings [F1..Fn] with
// their anchors + the deliverable. Nothing else — no transcript, no
// overlay, no history; findings are carried verbatim as numbered points and
// the retry fixes named problems, never regenerates blind.
type RetryPackage struct {
	Spec        intake.Spec                  `json:"spec"`
	ACs         []ledger.AcceptanceCriterion `json:"acs"`
	Findings    []Finding                    `json:"findings"`
	Deliverable Deliverable                  `json:"deliverable"`
	// Round is the round this package feeds; the executor session is fresh
	// per round (Spec S07.6; G1 D1.1 fresh-context-per-stage).
	Round int `json:"round"`
}

// BuildRetryPackage assembles the retry package for the next round.
// Blocker findings only — notes never enter the loop (Spec S07.6); the
// requester-comment channel merges through AddRequesterFindings before this
// is built.
func BuildRetryPackage(spec intake.Spec, acs []ledger.AcceptanceCriterion, findings []Finding, d Deliverable, nextRound int) RetryPackage {
	return RetryPackage{
		Spec:        spec,
		ACs:         acs,
		Findings:    blockers(findings),
		Deliverable: d,
		Round:       nextRound,
	}
}

// AddRequesterFindings merges requester comments (6.2) into a round's
// finding set through the same numbered-anchored-point channel as judge
// findings (Spec S07.6). Requester points enter as blockers when contested
// (they name a problem to fix) and are re-numbered with the round set by
// the caller's validateFindings pass.
func AddRequesterFindings(fs []Finding, comments []RequesterComment) []Finding {
	for _, c := range comments {
		sev := SeverityNote
		if c.Blocking {
			sev = SeverityBlocker
		}
		fs = append(fs, Finding{
			Severity:  sev,
			Category:  CatSanityBlocker,
			Criterion: c.Criterion,
			Anchor:    c.Anchor,
			Text:      c.Text,
			Requester: true,
		})
	}
	return fs
}

// RequesterComment is one requester review comment entering the rework
// channel (comment mechanics are Spec S13's; this is the S07.6 ingress
// shape).
type RequesterComment struct {
	Text      string `json:"text"`
	Anchor    string `json:"anchor,omitempty"`
	Criterion string `json:"criterion,omitempty"`
	Blocking  bool   `json:"blocking,omitempty"`
}

// RoundRecord is one verification round's durable record (Spec S07.11:
// every verdict recorded with its reasons, per round).
type RoundRecord struct {
	Round   int     `json:"round"`
	Verdict Verdict `json:"verdict"`
	// Revision is the deliverable revision this round judged. Together with
	// ContentSHA it is the card's best-effort pin: the resume path (Spec
	// S07.7 "answering resumes the pipeline in place") reads the pinned
	// revision back from the artifact store and verifies the hash.
	Revision int `json:"revision,omitempty"`

	V0    *V0Result    `json:"v0,omitempty"`
	V1    *V1Result    `json:"v1,omitempty"`
	Axis1 []ACVerdict  `json:"axis1,omitempty"`
	Axis2 *Axis2Result `json:"axis2,omitempty"`
	// Axis2Skipped records the stakes-gating decision when axis 2 did not
	// run (Spec S07.8) — recorded, never silent.
	Axis2Skipped string `json:"axis2_skipped,omitempty"`

	Findings        []Finding `json:"findings,omitempty"`
	SuppressedNotes int       `json:"suppressed_notes,omitempty"`

	ContentSHA string `json:"content_sha256"`
	// EventSeq is the verify.round event row of this record.
	EventSeq int64 `json:"event_seq,omitempty"`
}

// stopState tracks the S07.6 stop rules across rounds.
type stopState struct {
	prevBlockerKeys map[FindingKey]bool
	prevContent     string
	recurRounds     int // consecutive rounds with the same unresolved blocker keys
	similarRounds   int // consecutive rounds with cosmetic-only edits
}

// observe folds one completed round into the stop state and reports
// convergence per the ratified rule: the same finding keys recur unresolved
// OR artifact similarity holds, for ⚙ convergence_patience_rounds
// consecutive rounds (Spec S07.6 "model inertia").
func (s *stopState) observe(roundFindings []Finding, content string, patience int64) (converged bool, why string) {
	keys := keySet(blockers(roundFindings))
	if s.prevBlockerKeys != nil {
		if len(keys) > 0 && sameKeys(keys, s.prevBlockerKeys) {
			s.recurRounds++
		} else {
			s.recurRounds = 0
		}
		if similarity(s.prevContent, content) >= convergenceSimilarity {
			s.similarRounds++
		} else {
			s.similarRounds = 0
		}
	}
	s.prevBlockerKeys = keys
	s.prevContent = content
	switch {
	case int64(s.recurRounds) >= patience:
		return true, fmt.Sprintf("the same blocker finding keys recurred unresolved for %d consecutive rounds", s.recurRounds)
	case int64(s.similarRounds) >= patience:
		return true, fmt.Sprintf("artifact similarity ≥ %.2f held for %d consecutive rounds (cosmetic edits in response to findings)", convergenceSimilarity, s.similarRounds)
	}
	return false, ""
}

func sameKeys(a, b map[FindingKey]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

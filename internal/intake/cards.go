package intake

import (
	"encoding/json"
	"fmt"
)

// Intake cards are platform asks: each open gate is one durable `asks` row
// (Spec S02.2) whose snapshot is the full card. Rendering is Spec S15's
// (B6) and inbox mechanics S3.2's; this file owns the content contracts.
// Gates wait — no intake gate auto-proceeds on a timer, at any tier (Spec
// S06.1); answering resumes the pipeline in place (4.3).

// CardKind names the intake card kinds.
type CardKind string

const (
	// CardInterview is a batched option card: up to 4 questions, each with
	// 2–4 labeled options plus free text, highest-weight unresolved slots
	// first (Spec S06.5).
	CardInterview CardKind = "interview"
	// CardClarification asks the open NEEDS-CLARIFICATION markers of a
	// drafted SPEC — an artifact with open markers cannot reach approval
	// (Spec S06.6).
	CardClarification CardKind = "clarification"
	// CardEscalation is the 1.7 single-question escalation (Spec S06.5).
	CardEscalation CardKind = "escalation"
	// CardCoverage is the Stage-2 decision card after exhausted coverage
	// auto-fix rounds — an agreed criterion never disappears silently
	// (Spec S06.7(a)).
	CardCoverage CardKind = "decision.coverage"
	// CardResearch is the Stage-2 decision card after the missing-research-
	// node bounce (Spec S06.7(d)).
	CardResearch CardKind = "decision.research"
	// CardSpecDoubt is the mandatory SPEC-DOUBT decision card — the P45
	// antidote, never absorbed, never softened into a note (Spec S06.8).
	CardSpecDoubt CardKind = "decision.spec_doubt"
	// CardApproval is the Stage-4 approval card (Spec S06.9).
	CardApproval CardKind = "approval"
	// CardDelta is the post-approval delta-only re-approval card (Spec
	// S06.9).
	CardDelta CardKind = "approval.delta"
)

// Question is one card question.
type Question struct {
	ID      string   `json:"id"` // slot id, or "marker-<n>" on clarification cards
	Text    string   `json:"text"`
	Options []Option `json:"options,omitempty"` // 2–4 labeled options; free text always available
	Weight  int      `json:"weight,omitempty"`
}

// maxQuestionsPerCard is spec-structural, not ⚙: Spec S06.5 fixes "up to 4
// questions per card" in the ratified text; S18 declares no key for it.
const maxQuestionsPerCard = 4

// Card is one ask snapshot. Exactly one Body field is set, per Kind.
type Card struct {
	Kind      CardKind `json:"kind"`
	TaskID    string   `json:"task_id"`
	RunID     string   `json:"run_id"`
	Version   int      `json:"version"` // card version, cited by the approval record
	IssuedTS  string   `json:"issued_ts"`
	Clearance float64  `json:"clearance"`
	Tier      Tier     `json:"tier"`

	Questions []Question    `json:"questions,omitempty"` // interview / clarification / escalation
	Decision  *DecisionBody `json:"decision,omitempty"`  // coverage / research / spec_doubt
	Approval  *ApprovalBody `json:"approval,omitempty"`
	Delta     *DeltaBody    `json:"delta,omitempty"`
}

// DecisionBody carries a decision card: what happened, the enumerated
// choices, and the 13.5 help block.
type DecisionBody struct {
	Summary string    `json:"summary"`
	Detail  []string  `json:"detail,omitempty"`
	Choices []Option  `json:"choices"`
	Help    HelpBlock `json:"help"`
}

// ApprovalBody is the S06.9 card content contract.
type ApprovalBody struct {
	Layer1 ApprovalLayer1 `json:"layer1"`
	Layer2 ApprovalLayer2 `json:"layer2"`
	// Routing is the S08.8 selection block (B3-3): the selected worker and
	// its plain-language reason, visible and overridable PRE-execution —
	// re-route/pin ride the approval answer. On no-fit it is the two-stage
	// offer card content (generalist default / compose-when-earned / gap
	// advice). Nil only in the test-only no-router posture.
	Routing *RouteBlock `json:"routing,omitempty"`
	// Actions: Approve · Re-plan (structured contest) · Re-interview;
	// Cancel is always available (4.5).
	Actions []string `json:"actions"`
	// StaleFlag is the S06.9 freshness flag: "assumptions may be stale",
	// with a one-click re-plan. It never blocks approving.
	StaleFlag    bool     `json:"stale_flag,omitempty"`
	StaleReasons []string `json:"stale_reasons,omitempty"`
}

// ApprovalLayer1 is one phone screen (Spec S06.9).
type ApprovalLayer1 struct {
	Restatement string   `json:"restatement"` // what I understood
	Deliverable []string `json:"deliverable"` // what you'll get
	Steps       []string `json:"steps"`       // what I'll do, numbered plain
	WillNotDo   []string `json:"will_not_do"`
	// Assumptions are the centerpiece — the one RCT-evidenced overreliance
	// mitigation; no additional forcing functions are stacked (R03 §2.4).
	Assumptions []Assumption `json:"assumptions"`
	Risks       []string     `json:"risks,omitempty"`
	CostTime    string       `json:"cost_time"`
	Clearance   float64      `json:"clearance"`
	SizeClass   string       `json:"size_class,omitempty"`
	SizeNote    string       `json:"size_note,omitempty"` // size-delta finding, stakes-gated display (2.5)
	Help        HelpBlock    `json:"help"`                // the 13.5 block
	Uncovered   []string     `json:"uncovered,omitempty"` // requester-accepted coverage gaps
	OpenFinds   []string     `json:"open_findings,omitempty"`
}

// ApprovalLayer2 is the expandable layer.
type ApprovalLayer2 struct {
	ACs           []AC                `json:"acs"` // both phrasings
	Steps         []Step              `json:"steps"`
	Coverage      map[string][]string `json:"coverage"`
	Verdicts      []VerdictRecord     `json:"verdicts,omitempty"`
	ResearchNodes []ResearchNode      `json:"research_nodes,omitempty"`
	Estimate      Estimate            `json:"estimate"`
	SpecRef       *ArtifactRef        `json:"spec_ref,omitempty"`
	PlanRef       *ArtifactRef        `json:"plan_ref,omitempty"`
}

// Approval card actions.
const (
	ActionApprove     = "approve"
	ActionRePlan      = "replan"
	ActionReInterview = "reinterview"
	ActionCancel      = "cancel"
	// ActionCompose is the no-fit stage-2 compose-a-worker verb (Spec
	// S08.6 compose-when-earned; offered only while the routing block
	// reports the gap earned and no composition ran for this task). It
	// does NOT close the card: the composition ceremony runs as its own
	// billed run while the approval stays open — composition never rides
	// approval, and never the zero-interaction band.
	ActionCompose = "compose"
)

// DeltaKind is the S06.9 delta vocabulary (OpenSpec pattern).
type DeltaKind string

const (
	DeltaAdded    DeltaKind = "ADDED"
	DeltaModified DeltaKind = "MODIFIED"
	DeltaRemoved  DeltaKind = "REMOVED"
)

// DeltaItem is one change against the frozen artifacts.
type DeltaItem struct {
	Kind   DeltaKind `json:"kind"`
	Target string    `json:"target"` // "AC-2" | "S-3" | "assumption:<text>" | "confinement:S-2" | …
	Old    string    `json:"old,omitempty"`
	New    string    `json:"new,omitempty"`
}

// DeltaBody is the delta-only card: exactly what changed, nothing else —
// a silently disappearing criterion is structurally impossible (Spec
// S06.9).
type DeltaBody struct {
	Origin string      `json:"origin"` // e.g. "freshness_revalidation" | "sibling_collision" | "contested_card" | "confinement_widening"
	Items  []DeltaItem `json:"items"`
	Help   HelpBlock   `json:"help"`
}

// VerdictRecord is one recorded critique outcome.
type VerdictRecord struct {
	Round    int         `json:"round"`
	Kind     VerdictKind `json:"kind"`
	Findings []string    `json:"findings,omitempty"`
	Doubt    string      `json:"doubt,omitempty"`
	Proposed Tier        `json:"proposed_tier,omitempty"`
	TS       string      `json:"ts"`
}

// ---- Answer payloads ----

// Answer is the uniform answer envelope; per-kind fields mirror the card.
type Answer struct {
	// Interview / clarification cards.
	Answers      []SlotAnswer `json:"answers,omitempty"`
	Assume       []SlotAnswer `json:"assume,omitempty"`
	ForceProceed bool         `json:"force_proceed,omitempty"`

	// Escalation card.
	Text string `json:"text,omitempty"`

	// Decision cards.
	Choice   string         `json:"choice,omitempty"`
	Criteria []string       `json:"criteria,omitempty"` // coverage: contested/dropped AC keys
	Facts    []SuppliedFact `json:"facts,omitempty"`    // research: requester-supplied facts

	// Approval / delta cards.
	Action  string      `json:"action,omitempty"`
	Contest *ContestRef `json:"contest,omitempty"`
	// Route is the S08.8 re-route/pin entry, applied with Approve (the
	// pre-execution override surface; recorded with its actor).
	Route *RouteOverride `json:"route,omitempty"`
}

// SlotAnswer answers or assumes one slot/marker.
type SlotAnswer struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

// ContestRef is the structured Re-plan entry: tap the AC, assumption, or
// step being contested (Spec S06.9).
type ContestRef struct {
	Target string `json:"target"`
	Note   string `json:"note,omitempty"`
}

// Decision-card choices.
const (
	ChoiceReplan           = "replan"
	ChoiceDropCriterion    = "drop_criterion"
	ChoiceProceedUncovered = "proceed_uncovered"
	ChoiceSupplyFact       = "supply_fact"
	ChoiceProceedAnyway    = "proceed_anyway"
	ChoiceAdjustSpec       = "adjust_spec"
	ChoiceRethink          = "rethink"
	ChoiceApproveDelta     = "approve"
	ChoiceRejectDelta      = "reject"
)

func decodeAnswer(raw json.RawMessage) (Answer, error) {
	var a Answer
	if err := json.Unmarshal(raw, &a); err != nil {
		return Answer{}, fmt.Errorf("%w: %v", ErrBadAnswer, err)
	}
	return a, nil
}

// defaultHelp drafts the 13.5 help block deterministically from the
// artifacts when no Utility seam is configured (Spec S06.10: the utility
// model phrases; content is platform-owned).
func defaultHelp(pair *Pair) HelpBlock {
	h := HelpBlock{
		What:      "Approving starts the work exactly as planned; nothing runs before you approve.",
		Wrong:     "If an assumption below is wrong, the result will miss what you actually wanted.",
		Recommend: "Read the restatement and the assumptions. If they match your intent, approve; if anything is off, contest it via Re-plan — reviewing now costs seconds, a wrong run costs hours.",
	}
	if pair != nil && len(pair.Plan.Risks) > 0 {
		h.Wrong = pair.Plan.Risks[0]
	}
	return h
}

package intake

import (
	"encoding/json"
	"fmt"
	"strings"
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
	// CardFamily is the pre-round family question: when Stage 0 leaves the task
	// family UNRESOLVED — no registered project declared one and no classifier
	// answered — the interview ASKS rather than assuming (P3-RW-11 R4; Spec
	// S06.5 + 1.7 ask-don't-assume). It is a DECISION card, not an interview
	// question, because six choices exceed S06.5's ratified 2–4 option bound on
	// questions while decision choices are unbounded (the SPEC-DOUBT precedent).
	CardFamily CardKind = "decision.family"
	// CardEmission is the honest landing after the planner seat's bounded
	// re-emission exhausts on a contract-invalid emission (the A15-cap class:
	// "step S-1 approach is 1294 characters (cap 1200)") — a served decision
	// card in place of the witnessed crash→fork→tombstone (P3-GF12; Spec
	// S06.6 [A15]; CONVENTIONS §60: content is never trimmed, so an emission
	// the platform must refuse ends in a human door, never a dead lineage).
	// Choices reuse the existing vocabulary: ChoiceReplan grants one more
	// paid round (the S06.7(a) requester-granted-round precedent);
	// ChoiceRethink cancels (the SPEC-DOUBT mapping).
	//
	// A new kind STRING was unavoidable where a new card SHAPE was not: it is
	// an ordinary DecisionBody rendered by the ordinary decision form, but every
	// existing kind carries wrong semantics on its face, and lying on the kind
	// line is worse than one additive word of vocabulary.
	CardEmission CardKind = "decision.emission"
)

// plainCardKind names a card the way the person waiting on it would name it.
// The KIND STRING itself is a machine value and stays one — it is the wire
// contract the surfaces switch on — so only the prose that mentions a card
// (park and resume reasons, which a requester reads as the task's own activity
// line) goes through here (P3-GF13 R1). An unknown kind falls through as itself
// rather than vanishing: a token beats a blank.
func plainCardKind(k CardKind) string {
	switch k {
	case CardInterview:
		return "interview questions"
	case CardClarification:
		return "open points on the draft"
	case CardEscalation:
		return "question raised while planning"
	case CardCoverage:
		return "decision about the criteria the plan does not cover"
	case CardResearch:
		return "decision about the research this task needs"
	case CardSpecDoubt:
		return "doubt raised against the plan"
	case CardFamily:
		return "question about what kind of task this is"
	case CardEmission:
		return "decision about a plan the platform had to refuse"
	case CardApproval:
		return "plan approval card"
	case CardDelta:
		return "approval of the changed plan"
	default:
		return string(k)
	}
}

// Question is one card question.
type Question struct {
	ID   string `json:"id"` // slot id, or "marker-<n>" on clarification cards
	Text string `json:"text"`
	// Phrased is the utility model's per-request rewording of Text (Spec
	// S06.5: the seat "phrases and summarizes but does not decide what must
	// be asked"; S06.10 duty row). Additive and OPTIONAL: Text stays the
	// canonical taxonomy question and remains the durable record, so an
	// absent or erroring phrase seam leaves the card byte-identical to the
	// taxonomy's own words (P3-RW-12 R6/R12). A surface renders Phrased when
	// present and Text otherwise.
	Phrased string   `json:"phrased,omitempty"`
	Options []Option `json:"options,omitempty"` // 2–4 labeled options; free text always available
	Weight  int      `json:"weight,omitempty"`
	// Recommended is the slot's AUTHORED default: the option value the platform
	// recommends when no per-goal suggestion was made (P3-GF7 R7; taxonomy
	// Slot.Recommended). The recommended option's own Effect says why it is the
	// recommendation. SuggestedOption, when the seat produced one, is the
	// per-goal recommendation and overrides this at render time — the look_feel
	// round-3 proof, where a recommendation written for THIS project is what the
	// operator praised. Additive and optional.
	Recommended string `json:"recommended,omitempty"`
	// Why is the slot's plain-words reason line (P3-GF3-BE1 R1/R8; taxonomy
	// Slot.Why), served so the card can say why the question is worth
	// answering. Additive and optional.
	Why string `json:"why,omitempty"`
	// Suggested is the utility seat's one-line task-grounded proposed answer
	// for this question (P3-GF3-BE1 R5; design note §2.B), folded by slot id
	// under the same containment as Phrased. Absent whenever the seat
	// degrades — the honest-absence posture, no new failure mode.
	Suggested string `json:"suggested,omitempty"`
	// SuggestedOption names the EXISTING option value the suggestion
	// corresponds to; a value that names no option of this question is
	// dropped by the fold (the Suggested text is kept).
	SuggestedOption string `json:"suggested_option,omitempty"`
	// Resolution is the slot's CURRENT resolution on a re-interview review
	// card (P3-GF3-BE1 R8; design note §2.D): how it settled (registry /
	// answered / assumption), so the surface can render review-and-adjust.
	// Nil on ordinary interview cards and for unresolved slots. Composed by
	// platform code from State.Resolutions — never by a model.
	Resolution *UnderstoodItem `json:"resolution,omitempty"`
}

// UnderstoodItem is one resolved must-know slot, rendered back to the
// requester as part of the per-round understanding block (P3-RW-12 R8). It
// is composed by platform code from State.Resolutions — never by a model —
// so "here is what I understood" can never claim more than the record holds.
type UnderstoodItem struct {
	SlotID string `json:"slot_id"`
	// Name is the plain-language name of what was settled: a taxonomy slot's
	// Name, or — on an escalation item — the question that was asked.
	Name string `json:"name"`
	// How is the origin label: ResolvedRegistry | ResolvedAnswered |
	// ResolvedAssumption for taxonomy slots, UnderstoodEscalation for an
	// answered 1.7 single-question escalation.
	How string `json:"how"`
	// Value is the MACHINE token the record stores — the option's Value, or
	// the requester's own words when they typed an answer. It is also the
	// round-trip key (the answer fold matches option values; the edit box is
	// seeded from it), so it stays byte-for-byte what State.Resolutions holds.
	Value string `json:"value,omitempty"`
	// Label is what the requester actually CLICKED: the plain wording of the
	// option whose Value this row carries, resolved at composition time from
	// the slot's own option list (P3-GF13 R9; WALK-F1 W6 saw "graceful" and
	// "simplest" served where a person had chosen a sentence). It rides
	// ALONGSIDE Value and never replaces it. Empty is honest absence, and it
	// means one of two things: the answer was the requester's own words (the
	// value IS the words), or the slot is carried over from another family's
	// set and has no options here to match against.
	Label      string `json:"label,omitempty"`
	Assumption string `json:"assumption,omitempty"`
}

// UnderstoodEscalation labels a recap item that came from an answered 1.7
// ask-don't-assume escalation rather than from a taxonomy slot (Spec S06.5;
// P3-RW-12 R9). It is a CARD vocabulary value and deliberately NOT a fourth
// SlotResolution kind: an escalation settles a consequential ambiguity raised
// mid-planning, not a must-know slot, so Clearance must keep ignoring it.
const UnderstoodEscalation = "escalation"

// UnderstoodBlock is the "here is what I understood so far" block carried by
// interview and clarification cards, and by the approval card's layer 1
// beside the planner's restatement (Spec S06.5 "summarizes"; S06.1's 1.3
// restate-and-confirm realization). Items are DETERMINISTIC — the platform's
// own record of every resolved slot. Text is the optional utility-phrased
// prose; empty whenever no phrase seam answered, and never synthesized from
// the items (the §26 honest-degradation rule: deterministic items are never
// presented as model prose).
type UnderstoodBlock struct {
	Items []UnderstoodItem `json:"items,omitempty"`
	Text  string           `json:"text,omitempty"`
}

// maxQuestionsPerCard is spec-structural, not ⚙: Spec S06.5 fixes "up to 4
// questions per card" in the ratified text; S18 declares no key for it. It
// bounds interview DELIVERY — the fresh asking of unresolved slots below the
// floor, highest-weight-first; the S06.9 Re-interview verb's review card
// re-presents the whole set with its current answers and is not bound by it
// (P3-GF3-BE1 §11 OQ-3).
const maxQuestionsPerCard = 4

// Card is one ask snapshot. Exactly one Body field is set, per Kind.
type Card struct {
	Kind      CardKind `json:"kind"`
	TaskID    string   `json:"task_id"`
	RunID     string   `json:"run_id"`
	Version   int      `json:"version"` // card version, cited by the approval record
	IssuedTS  string   `json:"issued_ts"`
	Clearance float64  `json:"clearance"`
	// ClearanceFloor is the tier's ⚙ clearance floor, served next to the
	// computed Clearance so a surface can say where the questions stop
	// (P3-GF3-BE1 R11; ⚙ intake.clearance_floor.*, read at card issue). The
	// trivial tier has no floor and omits it. Served, never derived — the
	// floor VALUE and its consumption stay exactly as landed.
	ClearanceFloor float64 `json:"clearance_floor,omitempty"`
	Tier           Tier    `json:"tier"`

	// Family and FamilySource carry the task's family and what resolved it onto
	// every issued card (P3-GF7 R9; harvest H18, where a silent family guess sent
	// a whole interview down the wrong template and the only cue was a chip).
	// They are PASSIVE DATA on cards that already exist: the classifier path
	// gains no card, no click and no new ask (RW-13 zero-touch), and rendering
	// the chip plus its correction affordance is the surface's (GF9).
	//
	// The family card omits them, because family is precisely what is unresolved
	// there. Additive and omitempty, so an ask snapshot written before this
	// packet decodes exactly as it did.
	Family       Family `json:"family,omitempty"`
	FamilySource string `json:"family_source,omitempty"`

	// Stakes is the platform-authored stakes block (P3-GF14 R4.3) on the cards
	// a person decides at: what the tier is, WHO set it, why in plain words,
	// any pending downward proposal, and whether the requester's one downward
	// move is legal right now. It rides beside Tier rather than replacing it —
	// an old reader keeps reading Tier — and it exists because one card served
	// two stakes truths: the chip said high while the plan's own prose said the
	// task was being treated as light, because a critique's tier objection had
	// been handed to the planner to "resolve" in words.
	Stakes *Stakes `json:"stakes,omitempty"`

	Questions []Question    `json:"questions,omitempty"` // interview / clarification / escalation
	Decision  *DecisionBody `json:"decision,omitempty"`  // coverage / research / spec_doubt
	Approval  *ApprovalBody `json:"approval,omitempty"`
	Delta     *DeltaBody    `json:"delta,omitempty"`

	// Understood is the per-round understanding block on interview and
	// clarification cards (P3-RW-12 R8, OQ5). Escalation and family cards
	// carry none: a single mid-planning question and a pre-round
	// nothing-is-known question have nothing to summarize. Nil before the
	// first slot resolves and before any phrase seam answers.
	Understood *UnderstoodBlock `json:"understood,omitempty"`
}

// Stakes is one card's whole stakes truth, composed by platform code from the
// state's own record — never by a model (P3-GF14 R4.3).
type Stakes struct {
	Tier Tier `json:"tier"`
	// Origin is what set the standing tier: TierSourceFailClosed,
	// TierSourceClassifier, TierSourceRequester or TierSourceFloor. Empty on a
	// task whose state predates the record — an honest absence, never a guess.
	Origin      string `json:"origin,omitempty"`
	PlainReason string `json:"plain_reason"`
	// ProposedLower is a critique's pending opinion that the tier is too high.
	// It is a proposal TO THE REQUESTER and moves nothing by itself: no
	// downward verdict exists (Spec S06.4/S06.8), and the planner never sees it.
	ProposedLower Tier `json:"proposed_lower,omitempty"`
	// CanLower reports whether the S06.4 explicit requester action is legal
	// right now: above the floor, not into the rule-decided band, pre-approval.
	CanLower bool `json:"can_lower"`
}

// stakesBlock composes the card's stakes truth from the state (P3-GF14 R4.3).
func stakesBlock(st *State) *Stakes {
	return &Stakes{
		Tier:          st.Tier,
		Origin:        st.TierSource,
		PlainReason:   stakesReason(st),
		ProposedLower: pendingLowerProposal(st),
		CanLower:      canLowerTier(st),
	}
}

// canLowerTier reports whether Pipeline.LowerTier would accept a lowering
// right now. It reads the same three walls the verb enforces — the floor, the
// rule-decided band, and the pre-approval phase — so the card offers exactly
// what the answer path accepts (§71: one rule, two readers).
func canLowerTier(st *State) bool {
	if st.Phase == PhaseApproved || st.Phase == PhaseCancelled {
		return false
	}
	lowest := TierLow // the band is never re-entered by hand (S06.4)
	if st.FloorTier != "" {
		lowest = maxTier(lowest, st.FloorTier)
	}
	return tierRank[st.Tier] > tierRank[lowest]
}

// pendingLowerProposal returns the most recent critique proposal that ranks
// BELOW the standing tier, or "" when none stands. It is read from the
// recorded verdicts, which is where a tier opinion belongs (Spec S06.8).
func pendingLowerProposal(st *State) Tier {
	for i := len(st.Verdicts) - 1; i >= 0; i-- {
		if p := st.Verdicts[i].Proposed; ValidTier(p) && tierRank[p] < tierRank[st.Tier] {
			return p
		}
	}
	return ""
}

// stakesReason says, in the words of the person reading the card, why the
// platform is being as careful as it is (CONVENTIONS §59).
func stakesReason(st *State) string {
	switch st.TierSource {
	case TierSourceFailClosed:
		return "I could not read this request well enough to judge how careful to be, so I am treating it as high-stakes until you tell me otherwise."
	case TierSourceFloor:
		return "This task " + plainFloorClauses(st.FloorReasons) + ", so it counts as high-stakes and cannot be set lower."
	case TierSourceRequester:
		return "You chose this yourself: I am treating the task as " + plainTier(st.Tier) + "."
	case TierSourceClassifier:
		return "Reading the request, I am treating this as " + plainTier(st.Tier) + "."
	default:
		return "I am treating this as " + plainTier(st.Tier) + "."
	}
}

// plainTier names a stakes tier the way the person answering the card would
// name it. The tier TOKEN stays the machine value every surface switches on;
// only the prose goes through here (the plainCardKind precedent).
func plainTier(t Tier) string {
	switch t {
	case TierTrivial:
		return "a small routine job"
	case TierLow:
		return "low-stakes work"
	case TierStandard:
		return "ordinary-stakes work"
	case TierHigh:
		return "high-stakes work"
	default:
		return string(t)
	}
}

// plainFloorClass names one deterministic floor class in plain words; an
// unknown class returns "" and is left out rather than printed as a token.
func plainFloorClass(class string) string {
	switch class {
	case FloorOutwardEffect:
		return "sends something out beyond this machine"
	case FloorNewSpend:
		return "spends money"
	case FloorCredentialTouch:
		return "uses a saved login"
	case FloorSharedAssetWrite:
		return "changes something other people share"
	case FloorRegulatedDomain:
		return "touches an area with rules of its own"
	default:
		return ""
	}
}

// plainFloorClauses joins the distinct tripped floor classes into one readable
// clause. Nothing nameable leaves the general sentence, which is still true.
func plainFloorClauses(reasons []FloorReason) string {
	seen := make(map[string]bool, len(reasons))
	parts := make([]string, 0, len(reasons))
	for _, r := range reasons {
		if seen[r.Class] {
			continue
		}
		seen[r.Class] = true
		if plain := plainFloorClass(r.Class); plain != "" {
			parts = append(parts, plain)
		}
	}
	switch len(parts) {
	case 0:
		return "has something in it that always counts as high stakes"
	case 1:
		return parts[0]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
	}
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
	// Understood is the FULL slot-by-slot recap — every resolution including
	// band and force-proceed conversions, each origin-labeled — beside the
	// planner's prose Restatement (P3-RW-12 R9). It complements the
	// restatement rather than replacing it: the restatement is what the
	// planner understood, this is what the platform recorded. It adds no
	// card and no click; approval is still the confirmation (Spec S06.1).
	Understood *UnderstoodBlock `json:"understood,omitempty"`
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
	// Constraints and Supplied complete the served understanding on the
	// drafted-plan surface (P3-GF8; operator record r5 §B.1/§C rule 7): the
	// SPEC's constraints and the requester-supplied inputs were the two §B.1
	// understanding fields the card did not serve. Additive and omitempty —
	// an ask snapshot written before this packet decodes exactly as it did.
	Constraints []string       `json:"constraints,omitempty"`
	Supplied    []SuppliedFact `json:"supplied,omitempty"`
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
	// ActionLowerStakes is S06.4's one downward move made reachable: an
	// EXPLICIT requester action, carrying the tier to move to, routed to
	// Pipeline.LowerTier and refused by that verb's own rules (never below a
	// floor, never into the rule-decided band, never after approval). It does
	// NOT close the card — the requester still owes the approval decision, and
	// the card re-serves at the settled tier so every later answer, its
	// step-up included, follows the tier the card now carries.
	ActionLowerStakes = "lower_stakes"
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
	Kind DeltaKind `json:"kind"`
	// Target names the changed field in the ONE declared vocabulary
	// (target.go): "AC-2" | "S-3" | "confinement:S-2" | "restatement" |
	// "outcome:1" | "constraint:1" | "out_of_scope:1" | "risk:1" |
	// "approach:S-2" | "assumption". It is the same vocabulary a Re-plan
	// contest names, so the field a requester contested before approval and
	// the field that changed after it carry one name.
	Target string `json:"target"`
	Old    string `json:"old,omitempty"`
	New    string `json:"new,omitempty"`
}

// DeltaBody is the delta-only card: exactly what changed, nothing else —
// a silently disappearing criterion is structurally impossible (Spec
// S06.9).
type DeltaBody struct {
	Origin string      `json:"origin"` // e.g. "freshness_revalidation" | "sibling_collision" | "contested_card" | "confinement_widening"
	Items  []DeltaItem `json:"items"`
	// Actions is the card's OWN answer vocabulary — Approve · Reject — for the
	// same reason ApprovalBody carries one: a surface renders controls from the
	// card, so a card that declared none was structurally unanswerable from any
	// surface but the one that hard-coded its verbs. It is built from the same
	// ChoiceApproveDelta/ChoiceRejectDelta constants applyDeltaAnswer validates
	// against, so the offered set and the accepted set are one list.
	Actions []string  `json:"actions"`
	Help    HelpBlock `json:"help"`
}

// DeltaActions is the delta card's answer vocabulary, from the constants the
// answer path itself reads. One list, two readers.
func DeltaActions() []string { return []string{ChoiceApproveDelta, ChoiceRejectDelta} }

// familyLabels are the plain-language names of the six families, written for a
// non-programmer: the person answering this card is being asked what KIND of
// thing they want done, not to pick a taxonomy id.
var familyLabels = map[Family]string{
	FamilySoftware: "Build or change software",
	FamilyResearch: "Find something out",
	FamilyContent:  "Write or create content",
	FamilyData:     "Work with data",
	FamilyChore:    "A routine chore",
	FamilyGeneric:  "Something else",
}

// FamilyChoices is the family card's answer vocabulary, built from the SAME
// Families() list applyFamilyAnswer validates against — so the set the card
// offers and the set the pipeline accepts are one list rather than two that
// agree today (CONVENTIONS §43, the DeltaActions precedent).
func FamilyChoices() []Option {
	fams := Families()
	out := make([]Option, 0, len(fams))
	for _, f := range fams {
		out = append(out, Option{Label: familyLabels[f], Value: string(f)})
	}
	return out
}

// familyVocabularySentence names the accepted values for a refusal, so a
// refused answer says what this card actually takes.
func familyVocabularySentence() string {
	quoted := make([]string, 0, len(Families()))
	for _, f := range Families() {
		quoted = append(quoted, fmt.Sprintf("%q", string(f)))
	}
	return strings.Join(quoted, ", ")
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
	// Family CORRECTS the family shown on the card (P3-GF7 R9): a requester who
	// sees "I am treating this as software work (my guess)" and disagrees says so
	// in the same answer, instead of discovering the misclassification through
	// four wrong questions. Validated against the SAME ValidFamily vocabulary the
	// family card offers (§43: one list, two readers).
	//
	// The answers on the body are applied FIRST, against the set that was
	// actually asked, and the switch follows: every resolution already given is
	// retained on the record and simply stops counting toward Clearance if the
	// new set does not carry that slot. Silently discarding them is the harvest's
	// H17, rejected.
	Family string `json:"family,omitempty"`

	// Escalation card.
	Text string `json:"text,omitempty"`

	// Decision cards.
	Choice   string         `json:"choice,omitempty"`
	Criteria []string       `json:"criteria,omitempty"` // coverage: contested/dropped AC keys
	Facts    []SuppliedFact `json:"facts,omitempty"`    // research: requester-supplied facts

	// Approval / delta cards.
	Action  string      `json:"action,omitempty"`
	Contest *ContestRef `json:"contest,omitempty"`
	// Contests is the multi-contest Re-plan entry (P3-GF3-BE1 R10; design
	// note §2.E; S06.9 structured entry — named targets, not a cardinality of
	// one): a SET of contested targets, back-compatible with the single
	// Contest above. All merge into one bounded delta re-plan.
	Contests []ContestRef `json:"contests,omitempty"`
	// Route is the S08.8 re-route/pin entry, applied with Approve (the
	// pre-execution override surface; recorded with its actor).
	Route *RouteOverride `json:"route,omitempty"`
	// Tier is the target of ActionLowerStakes — the tier the requester is
	// moving the task DOWN to (Spec S06.4). Read on that action alone.
	Tier Tier `json:"tier,omitempty"`

	// Note is the person's own words, the same channel the verify/ladder cards
	// carry (P3-RW-19 R6). One Answer type serves every intake card, so the
	// field is reachable everywhere — it is honored on exactly three answers:
	// the two cancel-shaped ones, ActionCancel on the approval card and
	// ChoiceRethink on the SPEC-DOUBT card, where it becomes the cancel's
	// `$.detail.reason`; and ActionRePlan, where it is the free-text contest
	// channel ("what I want different, in my words") and becomes a target-less
	// finding on the one bounded delta re-plan (P3-GF3-BE1 §11 OQ-2, which
	// EXTENDS the ratified OQ2 reading rather than contradicting it).
	// Everywhere else it is ignored: honoring it would rewrite the wording of
	// every intake ledger mint, which is a different packet's blast radius.
	Note string `json:"note,omitempty"`
}

// SlotAnswer answers or assumes one slot/marker.
type SlotAnswer struct {
	ID    string `json:"id"`
	Value string `json:"value"`
	// Skip is the per-slot "take the recommendation and move on" arm
	// (P3-GF3-BE1 R6; design note §2.C; S06.5's explicit-assumption
	// resolution arm): {id, skip:true} converts THAT slot to an explicit
	// assumption carrying the card's served suggestion when one exists. Valid
	// only for a slot asked on this card, and never together with a value.
	Skip bool `json:"skip,omitempty"`
}

// ContestRef is the structured Re-plan entry: tap the AC, assumption, or
// step being contested (Spec S06.9). Target is quoted from the card's own
// served content in the ONE declared vocabulary (target.go); a POSITIONAL
// target the served pair does not hold is refused, and the requester's own
// words keep the permissive fold.
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

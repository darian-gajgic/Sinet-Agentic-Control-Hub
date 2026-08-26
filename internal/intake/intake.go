// Package intake is the Spec S06 intake pipeline: everything between "a
// request arrives" and "an approved plan exists" — triage, the
// clarity-driven interview, the specification-with-numbered-AC contract,
// the plan artifact, the deterministic spine, plan self-attack, and the
// approval/freshness contract, with ceremony scaled by stakes (Spec S06.1).
//
// The pipeline is platform code composed over the landed seams: durable
// state and the run FSM (internal/run, Spec S02), the event log
// (internal/eventlog), and the Task Context Ledger (internal/ledger, Spec
// S05) — spec confirmation and plan approval enter the ledger through its
// control-plane ops, per the S05/S06 boundary (S06 owns HOW they enter).
// Model duties stay behind named seams per the S06.10 engine table:
// Classifier and SpotCheck (local duty aliases, Spec S12, B4), Planner and
// Critic (planning model, wired to real engine sessions at B2-4), Utility
// (utility model), Registry (project registry, Spec S13/S1.6). Absent
// seams degrade exactly as the spec directs: classifier failure fails
// closed to high stakes (S06.2); optional duties are skipped, never faked.
//
// Pipeline control state is event-sourced as `intake.state` run-events
// (full-state payloads, current = latest by event_seq — the ledger_update
// precedent of Spec S02.2); SPEC/PLAN artifacts are durable files under the
// artifact root (Spec S06.6), referenced from the ledger. An interrupted
// intake therefore resumes from its last artifact, never by replaying
// conversation (D7; Spec S06.1).
package intake

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Tier is the S06.4 stakes tier. Automatic adjustments are monotone upward
// within a task; lowering happens only by explicit requester action and
// never below a floor.
type Tier string

const (
	TierTrivial  Tier = "trivial" // the zero-interaction band (G1 P11)
	TierLow      Tier = "low"
	TierStandard Tier = "standard"
	TierHigh     Tier = "high"
)

var tierRank = map[Tier]int{TierTrivial: 0, TierLow: 1, TierStandard: 2, TierHigh: 3}

// ValidTier reports whether t is one of the four S06.4 tiers.
func ValidTier(t Tier) bool { _, ok := tierRank[t]; return ok }

// maxTier returns the higher-stakes of a and b (monotone-upward merges).
func maxTier(a, b Tier) Tier {
	if tierRank[b] > tierRank[a] {
		return b
	}
	return a
}

// Family is the S06.2 task family (feature 2.3). v0 ships the software
// question set plus the generic fallback; further family sets arrive with
// their domains (Spec S00.2, S19).
type Family string

const (
	FamilySoftware Family = "software"
	FamilyResearch Family = "research"
	FamilyContent  Family = "content"
	FamilyData     Family = "data"
	FamilyChore    Family = "chore"
	FamilyGeneric  Family = "generic" // unmatched requests (S06.5 fallback set)
)

// Families is the six-value family vocabulary in its ratified order. It is the
// ONE list the family card offers, the answer path accepts, and the
// composition root pins internal/project's cross-wall duplicate against
// (CONVENTIONS §43: one list, two readers).
func Families() []Family {
	return []Family{FamilySoftware, FamilyResearch, FamilyContent, FamilyData, FamilyChore, FamilyGeneric}
}

// ValidFamily reports whether f is one of the six ratified families.
func ValidFamily(f Family) bool {
	for _, known := range Families() {
		if f == known {
			return true
		}
	}
	return false
}

// FamilySource records WHAT resolved a task's family, so "generic" is always
// attributable and a silent generic is structurally impossible (P3-RW-11 R5).
// The precedence is registry > classifier > requester-asked > default, and
// "default" on a non-band task is precisely the state the family question
// exists to eliminate.
const (
	FamilySourceRegistry   = "registry"   // the registered project's captured family (S13.7)
	FamilySourceClassifier = "classifier" // the S12.4 intake-triage duty resolved it
	FamilySourceRequester  = "requester"  // the requester answered the family question (S06.5, 1.7)
	FamilySourceDefault    = "default"    // nothing resolved it: the fail-closed generic baseline
)

// Input is one requester-supplied input handed in with the request — a file the
// person gave the assistant and then handed to the task they started by
// chatting (Spec S15.7: "files handed to a launched task enter as task inputs
// through intake"; S06.3/S06.6 record what the requester supplied).
//
// It is a DESCRIPTOR, not bytes and not a mount. The object itself stays in the
// requester's control-plane exchange folder; what travels into the pipeline is
// its identity — which is what keeps S11.3's allowlist-only confinement intact
// (no sandbox ever mounts the exchange).
type Input struct {
	// Ref is the exchange-manifest id. It is resolved against the REQUESTER's
	// own folder before it ever reaches here, so a ref naming another person's
	// object never becomes an input (P3-B6-7 OQ8).
	Ref       string `json:"ref"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

// Request is one arriving request (feature 1.1: natural-language intake).
type Request struct {
	TaskID string `json:"task_id,omitempty"` // generated when empty
	UserID string `json:"user_id"`           // the requester — the D10 approver
	Title  string `json:"title"`
	Text   string `json:"text"`
	// Inputs are the requester-supplied inputs, additive-first (S15.2). The
	// State carrying this Request is marshaled whole onto every intake.state
	// event, so recording them here IS recording them on the intake record.
	Inputs []Input `json:"inputs,omitempty"`
	// Project OPTIONALLY pins the request to a registered project by registry
	// id (P3-RW-1). The Projects-tab door hands the id, so a pinned request
	// resolves its registry slice DIRECTLY — the S06.2 name-token scan of the
	// request text is not consulted. Empty keeps that scan exactly as before
	// (additive-first, S15.2). Durable for free: the State carrying this
	// Request is marshaled whole onto every intake.state event.
	Project string `json:"project,omitempty"`
	// PinnedLane OPTIONALLY pins this task to one lane, declared at CREATION
	// ahead of the first selection rather than only as a re-route on the
	// approval card (S00.9 A13; the Project precedent above). Selection
	// honors it in place of the S08.8 step-3 consumption-pressure comparison.
	// The lane is validated server-side against what this platform can
	// actually dispatch to, and a pin it cannot honor REFUSES the submission
	// rather than quietly dropping it — an operator who names a lane asked
	// for THAT lane. Empty is the ordinary case and changes nothing.
	// Durable for free, by the same marshaling as Project.
	PinnedLane string `json:"pinned_lane,omitempty"`
}

// LanePinOption is one lane a request may name in Request.PinnedLane, with the
// verdict already computed by the layer that owns it (S00.9 A13).
//
// The verdict is CARRIED, never re-derived here: internal/intake cannot import
// internal/worker (the S06.10 seam wall), so what crosses is data — the
// PlanWindowRecord shape, for the same reason. A rule spelled twice drifts.
type LanePinOption struct {
	// Lane is the lane name a request may name.
	Lane string
	// Pinnable reports that selection would honor a pin naming this lane.
	Pinnable bool
	// NotPinnable is the selection layer's own sentence saying why it would
	// not — quoted verbatim, so the refusal a person reads is the refusal the
	// platform computed.
	NotPinnable string
}

// Deterministic floor classes (Spec S06.2): any of these forces the high
// tier regardless of classifier output and requester convenience settings.
// Floors size ceremony only — D7 gates every outward effect regardless.
const (
	FloorOutwardEffect    = "outward_effect"
	FloorNewSpend         = "new_spend"
	FloorCredentialTouch  = "credential_touch"
	FloorSharedAssetWrite = "shared_asset_write"
	FloorRegulatedDomain  = "regulated_domain"
)

// FloorReason is one tripped deterministic floor.
type FloorReason struct {
	Class  string `json:"class"`
	Source string `json:"source"` // "request" | "registry" | "classifier" | "plan"
	Detail string `json:"detail"`
}

// TriggerHit is one P47 research-trigger hit (Spec S06.3). Hits set the
// data-bearing flag and oblige research nodes in the PLAN; a hit is
// dismissible only by the requester supplying or owning the fact.
type TriggerHit struct {
	RuleID string `json:"rule_id"`
	Class  string `json:"class"`
	Cue    string `json:"cue"`
	Source string `json:"source"` // "rule" | "classifier" — the classifier may only ADD
}

// SuppliedFact is a requester-supplied input dismissing a P47 hit,
// recorded on the SPEC (Spec S06.3).
type SuppliedFact struct {
	RuleID string `json:"rule_id"`
	Fact   string `json:"fact"`
	TS     string `json:"ts"`
}

// Estimate is a size/cost guess in API-equivalent USD (D5 reporting
// currency; pricing is Spec S10's — an absent price table leaves Known
// false, the UNPRICED honesty posture, never a silent $0).
type Estimate struct {
	SizeClass string  `json:"size_class,omitempty"`
	USD       float64 `json:"usd,omitempty"`
	Known     bool    `json:"known"`
	Basis     string  `json:"basis,omitempty"`
}

// RegistrySlice is the matched project/repository registry entry injected
// at triage (S1.6): conventions, commands, danger zones — so the interview
// never asks what the platform already knows; danger zones feed the stakes
// floor (Spec S06.2). The registry home is Spec S13 (B4); this is the
// consuming shape.
type RegistrySlice struct {
	Project       string            `json:"project"`
	Ref           string            `json:"ref"`              // registry-slice ref for the intake record
	Family        Family            `json:"family,omitempty"` // the registered project's task family
	Conventions   []string          `json:"conventions,omitempty"`
	Commands      []string          `json:"commands,omitempty"`
	DangerZones   []DangerZone      `json:"danger_zones,omitempty"`
	ResolvedSlots map[string]string `json:"resolved_slots,omitempty"` // taxonomy slot id → registry-supplied value
}

// DangerZone mirrors the registry danger-zone row consumed at triage and
// pinned into ledger §2 at approval (Spec S06.9, G1 Def.12).
type DangerZone struct {
	Path       string `json:"path"`
	Rule       string `json:"rule"`
	SourceHash string `json:"source_hash,omitempty"`
}

// TriageProposal is the local classifier's Stage-0 proposal (Spec S06.2).
// Floors and data-bearing hits are add-only: the deterministic layers run
// first and a classifier can never remove what a rule found.
type TriageProposal struct {
	Family   Family
	Tier     Tier
	Est      Estimate
	ReadOnly bool // conservative band input: no writes outside the run workspace expected
	NewNeeds bool // new tools, workers, grants, or automations expected (band condition 4)
	Floors   []FloorReason
	DataHits []TriggerHit
}

// TriageInput is everything the Stage-0 classification duty receives (Spec
// S06.2). It is a struct rather than positional arguments for the RunID's
// sake: see the field.
type TriageInput struct {
	// RunID is the CONSUMING run — the run the pipeline is driving right now,
	// which after a recovery-fork rebind is the FORK (the DraftInput.RunID
	// precedent, CONVENTIONS §16 + §26). The classification duty's mandatory
	// $0 D7 row rides it, and that row is only writable while the run exists
	// and is checkpointable, which is why the pipeline classifies on the
	// RUNNING intake run rather than at Start (Spec S06.2, S02.3).
	RunID string

	Request  Request
	Registry *RegistrySlice
}

// Classifier is the S06.10 "local duty aliases" seam for family, stakes,
// size, and data-bearing classification (Spec S12, B4). A nil classifier
// or a classify error fails closed: the task is treated as high-stakes
// with an unknown estimate and no band membership (Spec S06.2).
type Classifier interface {
	Classify(ctx context.Context, in TriageInput) (TriageProposal, error)
}

// Registry is the project-registry match seam (S1.6; registry home Spec
// S13, B4). ok=false means no registry entry matched.
type Registry interface {
	Match(ctx context.Context, req Request) (RegistrySlice, bool, error)
}

// SlotResolution resolves one must-know slot (Spec S06.5): a slot is
// resolved when registry-supplied, requester-answered, or converted to an
// explicit assumption.
type SlotResolution struct {
	SlotID     string `json:"slot_id"`
	How        string `json:"how"` // "registry" | "answered" | "assumption"
	Value      string `json:"value,omitempty"`
	Assumption string `json:"assumption,omitempty"`
}

const (
	ResolvedRegistry   = "registry"
	ResolvedAnswered   = "answered"
	ResolvedAssumption = "assumption"
)

// EscalationAnswer records one answered 1.7 single-question escalation.
type EscalationAnswer struct {
	From     string `json:"from"` // "draft" | "revise" | "critique"
	Question string `json:"question"`
	Answer   string `json:"answer"`
	TS       string `json:"ts"`
}

// DraftInput is everything the Stage-1 planning session receives to emit
// the artifact pair (Spec S06.5–S06.6). The taxonomy carries the interview
// coverage; the resolutions carry its outcome. Data-bearing hits oblige
// research nodes by policy — present before any model drafted the plan.
type DraftInput struct {
	// RunID is the CONSUMING run — the run the pipeline is driving right now,
	// which after a recovery-fork rebind is the FORK and never the superseded
	// parent. It is carried on the seam input rather than re-derived from the
	// task id, because a run id is composed exactly once, at birth: every later
	// consumer reads the run's current identity (CONVENTIONS §16 run-id rule,
	// extended by P3-RW-6 OQ4). Sessions, checkpoints and ledger writes made by
	// an implementation of this seam ride it.
	RunID string

	Request     Request
	Family      Family
	Tier        Tier
	Registry    *RegistrySlice
	Taxonomy    *Taxonomy
	Resolutions []SlotResolution
	DataHits    []TriggerHit
	Supplied    []SuppliedFact
	Escalations []EscalationAnswer
	SpecVersion int   // version the emitted SPEC must carry
	PlanVersion int   // version the emitted PLAN must carry
	Prior       *Pair // set on re-interview: artifacts stay intact (S06.9 Re-interview)
}

// Revision reasons (ReviseInput.Reason).
const (
	ReviseCoverage  = "coverage_autofix" // S06.7(a): uncovered ACs named in Findings
	ReviseResearch  = "research_node"    // S06.7(d): missing research nodes named in Findings
	ReviseCritique  = "critique_revise"  // S06.8 REVISE: numbered findings
	ReviseContest   = "replan_contest"   // S06.9 Re-plan: contested item named in Findings
	ReviseMarkers   = "markers"          // S06.6: NEEDS-CLARIFICATION resolutions attached
	ReviseInterview = "interview_update" // new interview answers after a pair exists
	ReviseDrop      = "drop_criterion"   // coverage decision card: explicit requester drop
)

// ReviseInput is one bounded revision round against the existing pair.
// Criteria do not drift: a revision addresses exactly the named findings
// or resolutions (Spec S06.7(a), S06.8).
type ReviseInput struct {
	// RunID is the consuming run (see DraftInput.RunID).
	RunID string

	Pair        Pair
	Reason      string
	Findings    []string
	Resolutions []SlotResolution
	Escalations []EscalationAnswer
	SpecVersion int
	PlanVersion int
}

// Planner is the Stage-1 seam: the requester's designated planning model
// (Spec S06.10). At B2-4 the implementation drives a real engine session
// in a C1 read-only sandbox (S06.6, P-T05-1); the pipeline validates
// whatever comes back — trust is in the spine, not the seam.
type Planner interface {
	Draft(ctx context.Context, in DraftInput) (Pair, error)
	Revise(ctx context.Context, in ReviseInput) (Pair, error)
}

// VerdictKind is the S06.8 terminal verdict set.
type VerdictKind string

const (
	VerdictPass      VerdictKind = "pass"
	VerdictRevise    VerdictKind = "revise"
	VerdictSpecDoubt VerdictKind = "spec_doubt" // the P45 antidote — never absorbed
	VerdictTierUp    VerdictKind = "tier_up"
)

// Verdict is one critique outcome (Spec S06.8).
type Verdict struct {
	Kind         VerdictKind `json:"kind"`
	Findings     []string    `json:"findings,omitempty"` // numbered blocker findings (REVISE)
	Doubt        string      `json:"doubt,omitempty"`    // plain-language why (SPEC-DOUBT)
	ProposedTier Tier        `json:"proposed_tier,omitempty"`
}

// Critic is the Stage-3 self-attack seam: planning-model class, fresh
// context, artifact-only input — the signature admits nothing but the
// pair, making context separation structural (Spec S06.8). Local models
// never hold this duty. Recheck re-judges only the named findings.
type Critic interface {
	Critique(ctx context.Context, pair Pair) (Verdict, error)
	Recheck(ctx context.Context, pair Pair, findings []string) (Verdict, error)
}

// HelpBlock is the 13.5 help content of the approval card (Spec S06.9):
// what this decision does, what could go wrong, what the platform
// recommends and why — non-IT reading level.
type HelpBlock struct {
	What      string `json:"what"`
	Wrong     string `json:"wrong"`
	Recommend string `json:"recommend"`
}

// Utility is the utility-model phrasing seam (Spec S06.10). Optional: a
// nil Utility falls back to deterministic text drafted from the artifacts.
type Utility interface {
	Help(ctx context.Context, pair Pair) (HelpBlock, error)
}

// PhraseQuestion is one ALREADY-SELECTED interview question offered to the
// phrase seam: the slot id it belongs to and the taxonomy's canonical text.
// Options are deliberately absent — labels are authored plain-language
// content and their values are canonical vocabulary, so v0 phrases the
// question text and nothing else (P3-RW-12 OQ4).
type PhraseQuestion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	// Options are the slot's labeled options (P3-GF3-BE1 R3): the suggestion
	// decoration needs the seat to SEE them so it can name an existing option
	// value in SuggestedOptions. Label phrasing is still not a v0 duty — the
	// options ride along as context, never as something to rewrite.
	Options []Option `json:"options,omitempty"`
}

// PhraseInput is one card's phrase-and-summarize request (Spec S06.5, the
// S06.10 utility duty row). The pipeline has ALREADY chosen the questions
// from the taxonomy when this is built: the seat receives that selection and
// may only reword it — it never learns which slots were passed over, and its
// answer cannot add, drop, or merge a question (the fold is by slot id, in
// platform code; R03 §2.1 forbids prompt-level containment).
type PhraseInput struct {
	// RunID is the CONSUMING run — the run the pipeline is driving right
	// now, which after a recovery-fork rebind is the FORK (the
	// DraftInput.RunID precedent, CONVENTIONS §16 + §26): the ONE $0 D7 row
	// an implementation writes rides it.
	RunID string

	Request   Request
	Family    Family
	Tier      Tier
	Questions []PhraseQuestion
	// Understood is what the platform has already resolved — the material
	// the summary is drawn from. Deterministic and platform-composed; the
	// seat renders it as prose, it does not decide its contents.
	Understood []UnderstoodItem
}

// PhraseResult is the seat's answer: rewordings keyed by slot id, plus the
// one-paragraph "here is what I understood so far" summary. A missing id
// leaves that question's canonical text standing; an id that was not asked
// is dropped by the fold.
type PhraseResult struct {
	Phrasings map[string]string `json:"phrasings,omitempty"`
	Summary   string            `json:"summary,omitempty"`
	// Suggestions are one-line task-grounded proposed answers keyed by slot
	// id (P3-GF3-BE1 R3; design note §2.B). Folded by the caller under the
	// same containment as Phrasings: an id that was not asked is dropped, an
	// absent id leaves the question undecorated.
	Suggestions map[string]string `json:"suggestions,omitempty"`
	// SuggestedOptions name, per slot id, the EXISTING option value the
	// suggestion corresponds to; the fold drops any value that names no
	// option of the asked question.
	SuggestedOptions map[string]string `json:"suggested_options,omitempty"`
}

// Phraser is the S06.5 phrase-and-summarize seat (Spec S06.10: "question/card
// phrasing, 13.5 help drafting, summaries → utility model"). It is separate
// from Utility rather than a second method on it because Help is pair-bound
// and phrasing runs before any pair exists — and because a landed seam
// contract stays byte-stable (P3-RW-12 OQ1). The one utility alias holds both
// duties: the local adapter implements both interfaces on one type.
//
// OPTIONAL: a nil Phraser, or any error from it, leaves the card carrying the
// taxonomy's verbatim questions with zero added clicks — never a faked
// rewording (CONVENTIONS §14/§26 degradation rule).
type Phraser interface {
	PhraseAndSummarize(ctx context.Context, in PhraseInput) (PhraseResult, error)
}

// SpotCheck is the advisory local-model semantic coverage check of Spec
// S06.7(a) — advisory-only, never a gate. Optional.
type SpotCheck interface {
	Check(ctx context.Context, pair Pair) ([]string, error)
}

// Escalation is the 1.7 ask-don't-assume escalation: a seam call may
// return it (as an error) to raise exactly one clarifying question as a
// single-question card; the run blocks-not-fails and the call is re-issued
// after the answer (Spec S06.5). Consequential means: would change an AC,
// the declared write-set, an outward effect, the stakes tier, or the cost
// estimate beyond the S06.7 size-delta factor.
type Escalation struct {
	Question string
}

func (e *Escalation) Error() string { return "intake: consequential ambiguity: " + e.Question }

// Settings is this package's view of the settings registry (CONVENTIONS
// §6: ⚙ values are read by dotted key, never hardcoded).
type Settings interface {
	Int(key string) (int64, error)
	Float(key string) (float64, error)
	FloatFor(key, userID string) (float64, error)
	Duration(key string) (time.Duration, error)
	String(key string) (string, error)
}

// ⚙ keys consumed here. intake.approval_stale_hours (Spec S06.9) is the
// folded alias of freshness.max_age per S18.4 R2 — the registry declares
// the freshness key; this package consumes it for pending-approval
// staleness.
const (
	keyZeroInteractionCost   = "intake.zero_interaction_cost_usd" // per-user (G1 P11 + D1.7)
	keyClearanceFloorLow     = "intake.clearance_floor.low"
	keyClearanceFloorStd     = "intake.clearance_floor.standard"
	keyClearanceFloorHigh    = "intake.clearance_floor.high"
	keySizeRecheckFactor     = "intake.size_recheck_factor"
	keyCoverageAutofixRounds = "intake.coverage_autofix_rounds"
	keyCritiqueReviseRounds  = "intake.critique_revise_rounds"
	keyApprovalStale         = "freshness.max_age"    // folded alias (S18.4 R2)
	keyClaimsDefaultWrite    = "claims.default_write" // consumed at plan approval (Spec S02.8)
)

// Errors callers branch on.
var (
	// ErrNotRunning rejects stage work while the intake run is not running:
	// admission is the scheduler's (Spec S10); intake never self-admits.
	ErrNotRunning = errors.New("intake: run is not running")
	// ErrNoState is returned for a task with no intake pipeline state.
	ErrNoState = errors.New("intake: task has no intake state")
	// ErrLineage refuses a dispatch-entry rebind onto a run that is not a
	// descendant of the state's current run (Spec S02.5 step 3 lineage,
	// migration 0002): a rebind MOVES the pipeline's identity, so it is granted
	// only along a real `runs.parent_run_id` chain within the same task.
	ErrLineage = errors.New("intake: dispatched run is not a recovery descendant of this task's intake run")
	// ErrGateOpen rejects advancing while a card waits — gates wait, at
	// every tier (Spec S06.1); answering resumes the pipeline in place.
	ErrGateOpen = errors.New("intake: a gate is open and waiting (S06.1: gates wait)")
	// ErrUnknownAsk is returned for an answer to an unknown or closed ask.
	ErrUnknownAsk = errors.New("intake: unknown or closed ask")
	// ErrBadAnswer covers answers that do not match the card's contract.
	ErrBadAnswer = errors.New("intake: answer does not match the card")
	// ErrNotRequester enforces D10: the requester approves their own spec
	// and plan; nothing self-approves (5.8).
	ErrNotRequester = errors.New("intake: only the requester may answer this card (D10)")
	// ErrMarkersOpen enforces S06.6: an artifact with open
	// NEEDS-CLARIFICATION markers cannot reach approval.
	ErrMarkersOpen = errors.New("intake: open NEEDS-CLARIFICATION markers cannot reach approval (S06.6)")
	// ErrPhase rejects an operation invalid in the current pipeline phase.
	ErrPhase = errors.New("intake: operation invalid in this pipeline phase")
	// ErrSeamMissing reports a required seam absent for the tier's ceremony
	// (e.g. no Critic at standard tier, where critique is mandatory).
	ErrSeamMissing = errors.New("intake: required seam not configured")
	// ErrBelowFloor rejects an explicit tier lowering below a deterministic
	// floor (Spec S06.2/S06.4).
	ErrBelowFloor = errors.New("intake: tier cannot drop below a deterministic floor")
	// ErrTaxonomy covers invalid taxonomy or trigger files.
	ErrTaxonomy = errors.New("intake: invalid taxonomy")
	// ErrPinRefused is the class of Request.Project refusals (P3-RW-1). A
	// requester who pins a project asked for THAT project: an invalid pin is
	// refused loudly at Submit and no task is born believing it got one
	// (S15.2 — the browser is a display, never an authority). It is the only
	// registry-seam error Start does not swallow: the name-token scan's
	// degrade posture (no match, no slice) is unchanged.
	ErrPinRefused = errors.New("intake: submitted project pin refused")
	// ErrPinUnknown covers a pinned project that does not exist AND one the
	// requester may not see — deliberately ONE refusal, because telling them
	// apart would leak the existence of another person's project (S13.7
	// visibility; S15.2 server-side authority).
	ErrPinUnknown = fmt.Errorf("%w: no such project for this requester", ErrPinRefused)
	// ErrPinNotActive covers a pinned project the requester CAN see that has
	// not been owner-approved yet (S13.7: only active entries feed intake
	// resolution). Visible, so it may be named honestly.
	ErrPinNotActive = fmt.Errorf("%w: project is registered but not active yet", ErrPinRefused)
	// ErrLanePinRefused is the class of Request.PinnedLane refusals (P3-LN-9;
	// S00.9 A13). It is the ErrPinRefused posture on a second axis: a
	// requester who pins a LANE asked for THAT lane, so a pin the platform
	// cannot honor is refused loudly at Submit and no task is born believing
	// it got one. Refusing — rather than degrading onto whatever selection
	// would have chosen — is the inversion that matters: routing degrades
	// when the PLATFORM picked a lane it cannot use, and refuses when a
	// PERSON did (§19; §12 — the only default is consent).
	//
	// ONE code for every unhonorable pin, with the DETAIL distinguishing the
	// cases. Unlike the project pin there is no existence-oracle concern:
	// lane names ship in the lane documents and are not secret, so the
	// message may name the lanes that ARE pinnable.
	ErrLanePinRefused = errors.New("intake: submitted lane pin refused")
	// ErrCitationUnresolved reports a PLAN citing a project-truth knowledge
	// entry that cannot be resolved to an active version at approval (Spec
	// S09.6): a loud capture failure, never a silent no-op that would escape
	// every future drift check (F10).
	ErrCitationUnresolved = errors.New("intake: cited project-truth entry cannot be resolved at approval (S09.6)")
)

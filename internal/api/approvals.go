package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/redact"
)

// approvals.go is the S15.6 approval inbox: the ONE queue where "the platform
// needs you" lives, and the hash-pinned answer verbs behind it.
//
// It PROJECTS over machinery that already exists — the B5-2..B5-7 inbox
// derivations (projection.go), the durable `asks` rows and their stored card
// snapshots, the S02.7 effect journal — and rebuilds none of it. Four contract
// rules bind every route here:
//
//   - Retry-safety is structural (S15.2; G2 D2.1): an answer is pinned to the
//     payload hash it was shown for. A stale hash fires nothing, a repeat
//     returns the already-resolved state, and a conflicting answer on a
//     resolved item is refused — so a phone retry can never double-fire.
//   - Risk tiers decide batching and step-up (S15.6): Low batches, Medium is
//     individual, High is never batched and re-prompts the PIN in the same
//     request (verify-at-act, S01.9 — no elevation is ever inherited).
//   - Owner scope is server-side and fail-closed (S01.9), 404-before-403.
//   - Payload-DERIVED card strings redact at this serving edge (§30 R19); the
//     ask snapshots and effect payloads are S06/S02 decision CONTENT and go
//     out unwrapped, exactly as the task detail does (§38 ruling (b)).

// The two ⚙ keys this file consumes, by dotted key (never a constant).
const (
	// keyApprovalExpiry is the ask/approval expiry horizon (G2 Def.2). The
	// S18 cross-check ties the same horizon to asks ("an answerable ask never
	// outlives its session"), so the countdown a card displays is derived from
	// it rather than from a second number.
	keyApprovalExpiry = "effects.approval_expiry"
	// keyFreshnessMaxAge is the pending-staleness threshold — the ratified fold
	// of intake.approval_stale_hours (S18.4 R2). A card pending past it flags
	// "assumptions may be stale" with one-click re-plan (S06.9, S15.6); the
	// flag never blocks approving.
	keyFreshnessMaxAge = "freshness.max_age"
)

// approvalsListCap bounds the ranked inbox. Structural, not ⚙ (S18 ratifies no
// such key — the §7 sseBatchSize precedent, interim under the standing
// settings-tab directive), with its reason: an inbox is a queue a person works
// through, and a page is the unit a person works. The bound changes how many
// cards arrive at once, never which cards are eligible — and it is the same
// number as readPageDefault because this is one read surface with one page size.
const approvalsListCap = readPageDefault

// answerBatchCap bounds one Low-tier batch (S15.6 "one action answers a
// selected set"). Structural, with its reason: a batch is a transport
// convenience over answers that are still applied INDIVIDUALLY, each in its own
// transaction on the control plane's single writer connection (S02.1), so the
// bound is what keeps one request from monopolizing the writer everything else
// shares. It is not a semantic merge and never becomes one.
const answerBatchCap = 100

// The inbox card kinds. Each names the landed derivation it is served from;
// internal/api imports no producer package, so these are the transport's own
// vocabulary over projections that already exist.
const (
	approvalKindAsk         = "ask"               // durable asks rows (S02.2) + their stored card
	approvalKindEffect      = "effect"            // proposed effects (S02.7 journal)
	approvalKindWatchdog    = "watchdog_flag"     // open watchdog flags (B5-3)
	approvalKindConformance = "conformance_card"  // RED conformance rows (B5-4)
	approvalKindDrift       = "drift_card"        // open drift incidents (B5-6A)
	approvalKindVerdict     = "benchmark_verdict" // pending blind-pair verdicts (B5-7)
	approvalKindAlarm       = "benchmark_alarm"   // standing BENCH-REG §12 alarms (B5-7)
	// The NINTH kind (B6-6 OQ1): the S09.7 memory-conflict question cards, from
	// the knowledge_conflicts rows through internal/memory's own browse read.
	approvalKindMemoryConflict = "memory_conflict"
)

// memoryActionResolve is the ninth kind's action vocabulary — the landed
// POST /api/memory/conflicts/{conflict}/resolve verb, named by the card exactly
// as the oversight kinds name theirs.
const memoryActionResolve = "resolve"

// The S06.4 risk tiers, as the inbox ranks and gates them (S15.6). The four
// intake tiers fold onto three inbox tiers: `trivial` is the zero-interaction
// band and batches with Low.
const (
	tierLow    = "low"
	tierMedium = "medium"
	tierHigh   = "high"
)

// tierRank orders the inbox: High first. A card outside the vocabulary ranks
// with Medium — the safe direction, since Medium is individual and un-stepped.
var tierRank = map[string]int{tierHigh: 0, tierMedium: 1, tierLow: 2}

// EffectApprovals is the D10 co-approval state of a proposed effect, DERIVED
// from the decision.recorded rows rather than stored: the single `approved_by`
// column records only the FINAL approver, so who else has already said yes has
// to come from the log (S15.6 "both approver states visible on the card").
type EffectApprovals struct {
	// PlatformLevel marks an effect that needs the operator's co-approval as
	// well as its owner's (S15.6; D10).
	PlatformLevel      bool   `json:"platform_level"`
	OwnerApproved      bool   `json:"owner_approved"`
	OperatorApproved   bool   `json:"operator_approved"`
	OwnerApprovedBy    string `json:"owner_approved_by,omitempty"`
	OperatorApprovedBy string `json:"operator_approved_by,omitempty"`
}

// ApprovalItem is one inbox card, ranked by risk and carrying everything an
// answer needs: the pin it must quote back, its expiry data, its staleness, its
// action vocabulary, and the card content itself.
//
// There is no percent, fraction, ratio or ETA field here and none may be added
// (§30 R12).
type ApprovalItem struct {
	// ID is the routable id the answer verb takes: "<kind>:<native id>".
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Owner string `json:"owner"`
	RunID string `json:"run_id,omitempty"`
	Tier  string `json:"tier"`

	// Answerable is false for the kinds whose verbs are not this packet's; the
	// reason names where the verb lives rather than pretending the card is inert.
	Answerable          bool   `json:"answerable"`
	NotAnswerableReason string `json:"not_answerable_reason,omitempty"`
	// Batchable is Low-tier only (S15.6): Medium and High are individual.
	Batchable bool `json:"batchable"`
	// StepUpRequired demands the actor's PIN in the same request (S01.9
	// verify-at-act; High tier).
	StepUpRequired bool `json:"step_up_required"`

	// PayloadHash is the pin an answer must quote back (S15.2). For an ask it
	// is the canonical hash of the stored card; for an effect it is the
	// journal's OWN pinned hash, served verbatim.
	PayloadHash string    `json:"payload_hash,omitempty"`
	ObservedTS  time.Time `json:"observed_ts"`
	// ExpiryAt is the countdown a card displays: observed + ⚙
	// effects.approval_expiry. EngineExpiryTS is the engine's own deadline
	// where the ask carries one, served verbatim beside it.
	ExpiryAt       *time.Time `json:"expiry_at,omitempty"`
	EngineExpiryTS string     `json:"engine_expiry_ts,omitempty"`
	// Stale is DERIVED AT READ — pending age past ⚙ freshness.max_age, OR'd
	// with the flag the card itself already carries. No sweep, no ticker.
	Stale        bool     `json:"stale"`
	StaleReasons []string `json:"stale_reasons,omitempty"`

	// Actions is the card's OWN action vocabulary, verbatim. No verb is
	// invented here: S15.2's family-level "approve / deny / answer / re-plan"
	// is what these map into.
	Actions   []string         `json:"actions,omitempty"`
	Approvals *EffectApprovals `json:"approvals,omitempty"`

	// Card is the card content: the stored ask snapshot (the S06.9 Layer-1 /
	// Layer-2 bodies including the 13.5 help block), the effect payload, or the
	// landed projection row of the other five kinds.
	Card json.RawMessage `json:"card,omitempty"`
}

// ApprovalList is the ranked inbox with its S15.3 tail cursor.
type ApprovalList struct {
	Items     []ApprovalItem `json:"items"`
	Cursor    int64          `json:"cursor"`
	Truncated bool           `json:"truncated"`
}

// ── GET /api/approvals ──────────────────────────────────────────────────────

func (s *Server) handleApprovalList(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	scope := s.readScope(r)
	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	// The ranked derivation is SHARED with the S15.11 notifier (notifier.go):
	// "which cards are pending for identity X" has exactly one expression, so
	// the badge on a phone and the queue on this surface cannot disagree about
	// what is waiting (B6-9 OQ3/OQ5; the §40-C D3 twin hazard).
	items, _, err := s.rankedApprovals(r.Context(), scope)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	out := ApprovalList{Items: items, Cursor: cursor}
	if len(out.Items) > approvalsListCap {
		out.Items, out.Truncated = out.Items[:approvalsListCap], true
	}
	// The response is NOT wrapped as a whole: ask snapshots and effect payloads
	// are S06/S02 decision CONTENT, structurally exempt from the observability
	// primitive (§7-C2·2, §38 ruling (b)) — an approval card is what the person
	// is deciding about, not a projection of a trace. The cards that ARE lifted
	// out of run_events payload bodies redact per string VALUE first (R3).
	s.writeReadJSON(w, out)
}

// approvalHorizons reads the two ⚙ horizons this surface serves. Both keys are
// declared (S18), so a read failure is a composition defect, not a runtime
// condition: it fails loudly rather than serving a card with an invented
// countdown.
func (s *Server) approvalHorizons() (expiry, stale time.Duration, err error) {
	if s.settings == nil {
		return 0, 0, errors.New("api: no settings registry wired: the approval inbox serves expiry and staleness from ⚙ and will not invent them")
	}
	if expiry, err = s.settings.Duration(keyApprovalExpiry); err != nil {
		return 0, 0, fmt.Errorf("read ⚙ %s: %w", keyApprovalExpiry, err)
	}
	if stale, err = s.settings.Duration(keyFreshnessMaxAge); err != nil {
		return 0, 0, fmt.Errorf("read ⚙ %s: %w", keyFreshnessMaxAge, err)
	}
	return expiry, stale, nil
}

// approvalItems folds the seven landed inbox derivations into the one ranked
// list, enriching the two ANSWERABLE kinds with their pin, expiry and tier.
// The five oversight kinds are carried verbatim from their projections — their
// verbs are later packets, and the card says so rather than going missing.
func (p *projector) approvalItems(ctx context.Context, scope ownerScope, snap InboxSnapshot, expiry, stale time.Duration) ([]ApprovalItem, error) {
	now := p.clock()
	out := []ApprovalItem{}

	asks, err := p.askCards(ctx, snap.Asks)
	if err != nil {
		return nil, err
	}
	for _, a := range asks {
		it := ApprovalItem{
			ID: approvalKindAsk + ":" + a.AskID, Kind: approvalKindAsk, Owner: a.Owner,
			RunID: a.RunID, Tier: a.Tier,
			PayloadHash: a.Hash, ObservedTS: a.ObservedTS,
			EngineExpiryTS: a.EngineExpiryTS, Actions: a.Actions, Card: a.Snapshot,
		}
		setAnswerable(&it, mayAnswerAsk(scope, a.Owner),
			"the card's owner answers it (D10); you can read it but not decide it")
		applyTierRules(&it)
		applyExpiryAndStaleness(&it, expiry, stale, now, a.StoredStale, a.StoredStaleReasons)
		out = append(out, it)
	}

	effects, err := p.effectCards(ctx, snap.Approvals)
	if err != nil {
		return nil, err
	}
	for _, e := range effects {
		it := ApprovalItem{
			ID: approvalKindEffect + ":" + e.EffectID, Kind: approvalKindEffect, Owner: e.Owner,
			RunID: e.RunID,
			// Every S02.7 effect approval is High by definition (S15.6: outward
			// or irreversible). The tier is not read off the row because an
			// effect that could downgrade itself out of the PIN re-prompt is
			// exactly the hole the tier exists to close.
			Tier:        tierHigh,
			PayloadHash: e.PayloadHash, ObservedTS: e.CreatedTS,
			Actions: []string{effectActionApprove, effectActionDeny},
			Card:    e.Payload,
		}
		appr, aerr := p.effectApprovals(ctx, e, expiry)
		if aerr != nil {
			return nil, aerr
		}
		it.Approvals = &appr
		setAnswerable(&it, mayAnswerEffect(scope, e),
			"this effect is its owner's decision; the operator co-approves only a platform-level one (D10)")
		applyTierRules(&it)
		applyExpiryAndStaleness(&it, expiry, stale, now, false, nil)
		out = append(out, it)
	}

	// The five oversight kinds. THREE of them are answerable as of B6-2B — the
	// suppress, dismiss and acknowledge verbs are live (oversight.go) — and their
	// cards say so PER CALLER, computed from the same authority rules those verbs
	// enforce, so the inbox never renders a control whose use would be refused
	// (the part-A D9 principle, extended). The two benchmark kinds stay
	// not-answerable-HERE with a reason naming part C: an inbox that hides what
	// it cannot yet answer is worse than one that says where the answer lives.
	//
	// None of the three runs applyTierRules, and that is deliberate: their tier
	// is a RANKING (severity/flag-now), not a gate. Batching and PIN step-up are
	// the S15.6 answer-path semantics for asks and effects; a suppress demands no
	// PIN, and a card claiming otherwise would describe a door that does not
	// exist.
	for _, f := range snap.WatchdogFlags {
		it := oversightItem(approvalKindWatchdog, f.RunID+"\x1f"+f.AnomalyClass, f.Owner, f.RunID,
			severityTier(f.Severity), f.FlaggedTS, redactWatchdogFlag(f))
		it.Actions = []string{oversightActionSuppress}
		// S14.4 makes "resume — I was wrong" first-class ON the card, and
		// `actions` is the vocabulary a surface renders from — so the door has
		// to be IN it (B6-6 OQ9(b)). A surface that derived the control from
		// `run_id` instead would be synthesizing a verb out of a card field,
		// which is exactly the invention `actions` exists to make unnecessary.
		// It is added only where the verb can apply: the resume route is
		// run-scoped, so a run-less platform flag has no run to resume, and its
		// authority is the same rule this card's `answerable` already carries.
		if f.RunID != "" {
			it.Actions = append(it.Actions, oversightActionResume)
		}
		setAnswerable(&it, mayAnswerWatchdogFlag(scope, f.Owner),
			"suppressing a flag is its owner's or the operator's (POST /api/watchdog/flags/suppress, S14.4)")
		out = append(out, it)
	}
	for _, c := range snap.ConformanceCards {
		tier := tierMedium
		if c.FlagNow {
			tier = tierHigh
		}
		it := oversightItem(approvalKindConformance, c.RowID, platformOwner, "", tier, c.LastRunTS, c)
		it.Actions = []string{oversightActionAcknowledge}
		setAnswerable(&it, mayAnswerPlatformCard(scope),
			"a conformance card is platform-scope: the operator acknowledges it (POST /api/approvals/{id}/acknowledge, S14.5)")
		out = append(out, it)
	}
	for _, d := range snap.DriftCards {
		it := oversightItem(approvalKindDrift, d.Fingerprint, platformOwner, "",
			severityTier(d.Severity), d.FirstTS, redactDriftCard(d))
		it.Actions = []string{oversightActionDismiss}
		setAnswerable(&it, mayAnswerPlatformCard(scope),
			"a drift card is platform-scope: the operator dismisses it (POST /api/approvals/{id}/dismiss, S14.6)")
		out = append(out, it)
	}
	for _, v := range snap.BenchmarkVerdicts {
		it := oversightItem(approvalKindVerdict, v.PairID, v.Owner, "",
			// A blind-pair verdict is the requester's own judgement call: never
			// batched (the mandatory arm-guess is per pair and structural,
			// BENCH-REG §3.3) and never step-up (it releases nothing outward).
			tierMedium, v.SampledTS, v)
		it.Actions = []string{benchmarkActionVerdict, benchmarkActionDecline}
		setAnswerable(&it, mayAnswerVerdict(scope, v.Owner),
			"a blind-pair verdict is its REQUESTER's own judgement and is not delegable — you can see the card and cannot vote it (POST /api/approvals/{id}/verdict, BENCH-REG §3.3)")
		out = append(out, it)
	}
	for _, f := range snap.BenchmarkFailedPairs {
		// The eighth kind (drain r1). Low tier: nothing is at risk and nothing is
		// blocked — the pair is already over, and the card exists so a person can
		// close it rather than so they must hurry. It rides the SAME verdict-card
		// id space, because it is the same pair and the verb that closes it is the
		// landed decline.
		it := oversightItem(approvalKindVerdict, f.PairID, f.Owner, "", tierLow, f.SampledTS, f)
		it.Actions = []string{benchmarkActionDecline}
		setAnswerable(&it, mayAnswerVerdict(scope, f.Owner),
			"closing a failed benchmark pair is its REQUESTER's own act (POST /api/approvals/{id}/decline, BENCH-REG §4.2.5)")
		out = append(out, it)
	}
	for _, a := range snap.BenchmarkAlarms {
		it := oversightItem(approvalKindAlarm, a.Domain+"#"+a.EpochID, platformOwner, "",
			tierHigh, a.RaisedTS, redactBenchmarkAlarm(a))
		it.Actions = []string{benchmarkActionDispose}
		setAnswerable(&it, mayDisposeAlarm(scope),
			"a benchmark alarm is platform-scope: the operator dispositions it (POST /api/approvals/{id}/dispose, BENCH-REG §12)")
		out = append(out, it)
	}
	return out, nil
}

// memoryConflictCards folds the NINTH kind into the one inbox (B6-6 OQ1): the
// S09.7 question cards, one per OPEN knowledge conflict addressed to the caller.
//
// ── The stated deviation, and why it is the right one ───────────────────────
//
// Every other kind here is operator-sees-all. This one is not: a conflict card
// reaches its ADDRESSEE and nobody else, the operator included. That is not a
// narrowing invented for the inbox — it is the landed B6-3C visibility line
// carried onto the surface. `mayAnswerConflict` is the ONE expression of both
// who sees the edge and who may resolve it, the entry-detail page already hides
// the edge block from third parties and the non-affected operator, and the
// resolve verb already refuses everyone but the addressee. An inbox that showed
// the operator another person's memory question would put a card on screen whose
// door refuses them, which is the exact D9 failure the `answerable` field exists
// to prevent — and it would leak the question itself, which is the thing S09
// scopes. So the browse read is addressee-scoped and the card's `answerable` is
// still computed through the same predicate: the read cannot hand over a card
// the rule would refuse, and the card still says the rule out loud.
//
// ── The three things this card deliberately does NOT carry ──────────────────
//
//   - No payload hash. The resolve verb is path-only and idempotent by design —
//     a repeat answers 200 with the already-closed detail — so there is no pin
//     to quote back and inventing one would describe a check nothing performs.
//     The generic answer verb therefore refuses this kind by its own default,
//     naming where the verb lives, exactly as it does for the oversight kinds.
//   - No expiry. `effects.approval_expiry` is the horizon of an approval that
//     releases something outward; a memory question releases nothing and does
//     not lapse. A countdown here would be a deadline nothing enforces.
//   - No staleness. The stored flag's served reason names one-click re-plan
//     (S06.9), which is meaningless on a conflict — an old question is still
//     exactly the question.
//
// The tier IS run through applyTierRules, unlike the oversight kinds: Medium
// here is a CLASSIFICATION (answered individually, releases nothing, no
// step-up), which is what that function's Low/Medium/High semantics express —
// not a severity ranking, which is why those kinds skip it.
//
// A process with no memory store simply has no ninth kind. It is not a 503: the
// other eight are readable, and failing the whole inbox because one optional
// subsystem is unwired would be worse than serving what exists.
func (s *Server) memoryConflictCards(ctx context.Context, scope ownerScope) ([]ApprovalItem, error) {
	if s.memory == nil {
		return nil, nil
	}
	conflicts, err := s.memory.OpenConflictsFor(ctx, scope.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]ApprovalItem, 0, len(conflicts))
	for _, c := range conflicts {
		// Unwrapped, like the rest of this response: a conflict row is S09
		// memory CONTENT — the person's own stored knowledge and the question
		// put to them about it — and the memory family serves that content
		// unwrapped everywhere else (§40-C). It is not lifted out of a
		// run_events payload, so the R3 per-value edge does not apply.
		it := oversightItem(approvalKindMemoryConflict, strconv.FormatInt(c.ID, 10),
			c.Affected, "", tierMedium, parseTS(c.DetectedTS), c)
		it.Actions = []string{memoryActionResolve}
		setAnswerable(&it, mayAnswerConflict(c, scope.UserID),
			"a memory conflict is answered by the person it was addressed to (POST /api/memory/conflicts/{conflict}/resolve, S09.7)")
		applyTierRules(&it)
		out = append(out, it)
	}
	return out, nil
}

// The oversight card action vocabulary (B6-2B). One verb per kind, named by the
// card so a surface renders the control the route actually accepts.
const (
	oversightActionSuppress    = "suppress"
	oversightActionDismiss     = "dismiss"
	oversightActionAcknowledge = "acknowledge"
	// oversightActionResume is the S14.4 "resume — I was wrong" door, carried on
	// the run-scoped watchdog card (B6-6 OQ9(b)). Its verb is the landed
	// POST /api/runs/{run}/resume, named by the card like every other action.
	oversightActionResume = "resume"
)

// The two benchmark card kinds' action vocabulary (B6-2C). The verdict card
// carries BOTH of its acts, because §4.2.5's decline is a first-class answer to
// the same card and a form that offered only "vote" would make declining look
// like ignoring — which is the silence §4.2.5 exists to make visible.
const (
	benchmarkActionVerdict = "verdict"
	benchmarkActionDecline = "decline"
	benchmarkActionDispose = "dispose"
)

// platformOwner is the reserved platform-scope owner id (S02.2 15.6): the
// operator-only oversight kinds are attributed to it, exactly as their
// projections are scoped.
const platformOwner = "platform"

// severityTier maps the S14.4 alert-routing class onto the inbox ranking. It
// is a RANKING, not a re-derivation: the severity is carried from the producer
// that computed it (§34 "severity derived never judged").
func severityTier(severity string) string {
	if severity == "flag-now" {
		return tierHigh
	}
	return tierMedium
}

func oversightItem(kind, nativeID, owner, runID, tier string, ts time.Time, card any) ApprovalItem {
	raw, err := json.Marshal(card)
	if err != nil {
		raw = nil
	}
	return ApprovalItem{
		ID: kind + ":" + nativeID, Kind: kind, Owner: owner, RunID: runID, Tier: tier,
		ObservedTS: ts, Card: raw,
	}
}

// ── who may answer what: ONE authority rule, read by the card and the verb ──
//
// The list and the verb MUST agree, or the inbox renders a button that 403s
// (drain D9). These two predicates are therefore the single source both read:
// the card's `answerable` is computed from them per CALLER, and the answer path
// authorizes with them too.

// mayAnswerAsk: D10 — the ask's OWNER answers. The role bit decides visibility,
// never authorship of somebody else's decision.
func mayAnswerAsk(scope ownerScope, owner string) bool { return scope.UserID == owner }

// mayAnswerEffect: the owner always; the operator additionally on a
// platform-level effect, which is exactly the co-approval D10 gives them.
func mayAnswerEffect(scope ownerScope, e effectCard) bool {
	return scope.UserID == e.Owner || (scope.Operator && e.PlatformLevel)
}

// mayAnswerWatchdogFlag: S14.4 suppress — the flag's owner, or the operator.
// A run-less platform-scope flag is attributed to `platform`, so only the
// operator matches it, which is the landed visibility rule stated as authority:
// you can suppress exactly what you can see.
func mayAnswerWatchdogFlag(scope ownerScope, owner string) bool {
	return scope.Operator || scope.UserID == owner
}

// mayAnswerPlatformCard: the S14.6 drift dismiss and the S14.5 conformance
// acknowledge are operator-only, exactly as the projections that produce those
// cards are scoped.
func mayAnswerPlatformCard(scope ownerScope) bool { return scope.Operator }

// setAnswerable records the per-caller verdict WITH its reason, so a surface
// that cannot answer a card knows why rather than rendering a dead control.
func setAnswerable(it *ApprovalItem, ok bool, why string) {
	it.Answerable = ok
	if !ok {
		it.NotAnswerableReason = why
	}
}

// applyTierRules is the ONE place the S15.6 tier semantics are expressed:
// Low batches, Medium is individual, High is never batched and always steps up.
func applyTierRules(it *ApprovalItem) {
	it.Batchable = it.Answerable && it.Tier == tierLow
	it.StepUpRequired = it.Answerable && it.Tier == tierHigh
}

// applyExpiryAndStaleness derives the countdown and the staleness flag AT READ.
// Nothing sweeps and nothing ticks: the flag is a function of the row's own
// observed time and the ⚙ horizon, so it is correct the instant it is read and
// cannot go stale in a side store (S14.1 derive-from-log).
func applyExpiryAndStaleness(it *ApprovalItem, expiry, stale time.Duration, now time.Time, storedStale bool, storedReasons []string) {
	if !it.ObservedTS.IsZero() && expiry > 0 {
		at := it.ObservedTS.Add(expiry)
		it.ExpiryAt = &at
	}
	it.StaleReasons = append([]string{}, storedReasons...)
	it.Stale = storedStale
	if !it.ObservedTS.IsZero() && stale > 0 && now.Sub(it.ObservedTS) > stale {
		it.Stale = true
		it.StaleReasons = append(it.StaleReasons,
			fmt.Sprintf("pending longer than ⚙ %s — assumptions may be stale; re-plan is one click (S06.9)", keyFreshnessMaxAge))
	}
	if len(it.StaleReasons) == 0 {
		it.StaleReasons = nil
	}
}

// ── the two answerable kinds, enriched ──────────────────────────────────────

// askCard is one open ask with the content an answer needs.
type askCard struct {
	AskID              string
	RunID              string
	Owner              string
	Tier               string
	Hash               string
	ObservedTS         time.Time
	EngineExpiryTS     string
	Actions            []string
	Snapshot           json.RawMessage
	StoredStale        bool
	StoredStaleReasons []string
}

// cardShape is the slice of a stored ask snapshot the inbox reads: the risk
// tier and the action vocabulary the card itself declares. Everything else in
// the snapshot is served through verbatim — the 13.5 help block included,
// which is precisely why the whole snapshot travels rather than a summary.
type cardShape struct {
	Kind     string `json:"kind"`
	Tier     string `json:"tier"`
	Approval *struct {
		Actions      []string `json:"actions"`
		StaleFlag    bool     `json:"stale_flag"`
		StaleReasons []string `json:"stale_reasons"`
	} `json:"approval"`
	Delta *struct {
		Origin  string   `json:"origin"`
		Actions []string `json:"actions"`
	} `json:"delta"`
	Decision *struct {
		Choices []struct {
			// The wire key is `value`, and `id` — which this read until B6-6 —
			// is a key NO producer writes. internal/intake's S06.7 decision
			// cards marshal []Option{Label, Value} (intake/cards.go), and the
			// answer validator compares the answer's `choice` against VALUE
			// (intake/answer.go). Reading `id` derived one empty string per
			// choice, so every coverage / research / spec-doubt card served an
			// action list of blanks: a surface rendering from it would have
			// drawn unlabelled buttons for verbs it could not name. Same
			// finding as the B6-3B `verbs`/`choices` fix, on the other decision
			// family — the Go field name was fine, the wire key was not.
			Value string `json:"value"`
		} `json:"choices"`
	} `json:"decision"`
	// Verify cards (S07.7) declare their answer verbs at the TOP LEVEL under
	// `choices` — the field internal/verify's escalation Card marshals into
	// asks.snapshot (verify/escalate.go) and the same one the landed answer
	// validator checks a choice against (verify/answer.go). Nothing in the tree
	// writes `verbs`, which is what this tag read until the B6-3B drain and why
	// verify-escalation cards derived an EMPTY action list. The Go field keeps
	// its name; the wire key is what was wrong.
	Verbs []string `json:"choices"`
}

// askCards reads the stored snapshot of each listed ask and derives its pin,
// tier and action vocabulary. The projection deliberately does NOT carry the
// snapshot (it is the resume input, not a stream summary field, §30), so the
// inbox reads it here — an extension of the landed derivation, not a second one.
func (p *projector) askCards(ctx context.Context, asks []InboxAsk) ([]askCard, error) {
	out := make([]askCard, 0, len(asks))
	for _, a := range asks {
		var (
			c            askCard
			snapshot     string
			owner        string
			engineExpiry sql.NullString
		)
		err := p.db.QueryRowContext(ctx,
			`SELECT user_id, snapshot, engine_expiry_ts FROM asks WHERE ask_id = ?`, a.AskID).
			Scan(&owner, &snapshot, &engineExpiry)
		if errors.Is(err, sql.ErrNoRows) {
			// The ask ROW is gone, which is not the same thing as answered — an
			// answered ask keeps its row (status/answered_ts move, §14). The only
			// way a row disappears is deletion, so this is the honest "the
			// subject vanished under the projection" case: skip it rather than
			// fail the whole inbox for one missing card (drain D12).
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("projection: ask card %q: %w", a.AskID, err)
		}
		c.AskID, c.RunID, c.Owner, c.ObservedTS = a.AskID, a.RunID, owner, a.ObservedTS
		c.Snapshot = json.RawMessage(snapshot)
		c.EngineExpiryTS = engineExpiry.String
		if c.Hash, err = gates.CanonicalHash(c.Snapshot); err != nil {
			return nil, fmt.Errorf("projection: pin ask %q: %w", a.AskID, err)
		}
		c.Tier, c.Actions, c.StoredStale, c.StoredStaleReasons = readCardShape(c.Snapshot)
		out = append(out, c)
	}
	return out, nil
}

// readCardShape derives the inbox-relevant facts from a stored card.
//
// A card that declares NO tier reads as Medium: the untiered cards at v0 are
// the S07.7 verify escalations and the S13.7 onboarding approval, which are
// answered individually by their owner and release nothing outward (§16 — the
// effects gate is untouched by a verify decision). Medium is therefore both
// correct for them and the safe default: it never batches an unclassified card
// and never demands a step-up the landed answer path does not demand.
func readCardShape(snapshot json.RawMessage) (tier string, actions []string, stale bool, reasons []string) {
	var c cardShape
	if err := json.Unmarshal(snapshot, &c); err != nil {
		return tierMedium, nil, false, nil
	}
	switch c.Tier {
	case "high":
		tier = tierHigh
	case "low", "trivial": // trivial is the zero-interaction band (S06.4)
		tier = tierLow
	case "standard":
		tier = tierMedium
	default:
		tier = tierMedium
	}
	switch {
	case c.Approval != nil:
		actions, stale, reasons = c.Approval.Actions, c.Approval.StaleFlag, c.Approval.StaleReasons
	case len(c.Verbs) > 0:
		actions = c.Verbs
	case c.Decision != nil:
		for _, ch := range c.Decision.Choices {
			actions = append(actions, ch.Value)
		}
	case c.Delta != nil:
		// The delta card's own two verbs. Until the B6-6 drain the producer
		// declared none, so a pending delta served an EMPTY action list and was
		// unanswerable from any surface that renders controls from the card —
		// while it held the task's OpenAskID open. The fix landed in the
		// producer (internal/intake issues the vocabulary from the same
		// constants its answer path validates against); this limb reads it,
		// exactly as the other three read theirs. Naming the verbs HERE instead
		// would have put the pipeline's answer vocabulary in the transport,
		// which is the copy this derivation exists to avoid.
		actions = c.Delta.Actions
	}
	return tier, actions, stale, reasons
}

// effectCard is one proposed effect with its journal-pinned hash.
type effectCard struct {
	EffectID    string
	RunID       string
	Owner       string
	Class       string
	PayloadHash string
	Payload     json.RawMessage
	CreatedTS   time.Time
	// PlatformLevel: the effect is attributed to no run.
	//
	// The reading, with its reason: every run belongs to one person's task, so
	// a run-attributed effect is that person's own outward act and its owner is
	// the whole decision (D10). An effect with no run attribution is not
	// anybody's task — it is the platform acting on shared ground, which is
	// exactly what S15.6 means by "platform-level" and what D10 gives the
	// operator a co-approval over. It is derived from data the journal already
	// stores; the alternative (a new payload field producers would have to set)
	// would have invented a contract no landed producer speaks.
	PlatformLevel bool
}

func (p *projector) effectCards(ctx context.Context, approvals []InboxApproval) ([]effectCard, error) {
	out := make([]effectCard, 0, len(approvals))
	for _, a := range approvals {
		c, found, err := p.effectCard(ctx, a.EffectID)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func (p *projector) effectCard(ctx context.Context, effectID string) (effectCard, bool, error) {
	var (
		c       effectCard
		runID   sql.NullString
		payload string
		created string
		state   string
	)
	err := p.db.QueryRowContext(ctx,
		`SELECT effect_id, run_id, user_id, class, payload, payload_hash, state, created_ts
		   FROM effects WHERE effect_id = ?`, effectID).
		Scan(&c.EffectID, &runID, &c.Owner, &c.Class, &payload, &c.PayloadHash, &state, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return effectCard{}, false, nil
	}
	if err != nil {
		return effectCard{}, false, fmt.Errorf("projection: effect card %q: %w", effectID, err)
	}
	c.RunID, c.Payload, c.CreatedTS = runID.String, json.RawMessage(payload), parseTS(created)
	c.PlatformLevel = !runID.Valid || runID.String == ""
	return c, true, nil
}

// effectApprovals derives the D10 co-approval state from the decision.recorded
// rows for the effect. Derive-from-log, no side store: the effects row records
// only the FINAL approver, and "who has already said yes" is a question about
// the decision record.
//
// The derivation is CYCLE-SCOPED (drain D3), and that is what makes it correct
// across the journal's own re-approval reversions. S02.7 sends an effect back
// to `proposed` on approval EXPIRY or on payload DRIFT, and a signature from
// the cycle before must not silently carry into the cycle after — otherwise an
// expired owner-yes would let the operator alone complete a platform-level
// conjunction, and a re-approval would dedup against the old row and fire
// gates.Approve with no fresh audit trail at all. So a row counts IFF:
//
//   - its recorded payload_hash equals the effect's CURRENT canonical hash —
//     a drift reversion changes the hash, so pre-drift signatures hash out; and
//   - its decided_at is within ⚙ effects.approval_expiry of now — a time
//     reversion happens exactly when that window lapses, so they age out.
//
// The same predicate bounds the SCAN (the `ts >=` limb): rows that cannot count
// are not read (N14).
func (p *projector) effectApprovals(ctx context.Context, e effectCard, expiry time.Duration) (EffectApprovals, error) {
	appr := EffectApprovals{PlatformLevel: e.PlatformLevel}
	now := p.clock().UTC()
	rows, err := p.db.QueryContext(ctx,
		`SELECT payload FROM run_events WHERE type = ? AND ts >= ? ORDER BY event_seq`,
		EventDecisionRecorded, now.Add(-expiry).Format(time.RFC3339Nano))
	if err != nil {
		return appr, fmt.Errorf("projection: effect approvals %q: %w", e.EffectID, err)
	}
	defer rows.Close()
	cardID := approvalKindEffect + ":" + e.EffectID
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return appr, err
		}
		var d decisionPayload
		if err := json.Unmarshal([]byte(payload), &d); err != nil {
			continue // a non-conformant payload never breaks the inbox (S14.2 rule 3)
		}
		if d.CardID != cardID || d.Decision != effectActionApprove {
			continue
		}
		if !d.countsForCycle(e.PayloadHash, now, expiry) {
			continue
		}
		if d.Actor == e.Owner {
			appr.OwnerApproved, appr.OwnerApprovedBy = true, d.Actor
		}
		if d.ActorIsOperator {
			appr.OperatorApproved, appr.OperatorApprovedBy = true, d.Actor
		}
	}
	return appr, rows.Err()
}

// countsForCycle is the cycle predicate (drain D3): a signature belongs to the
// effect's CURRENT approval cycle only if it was given for the payload that is
// on the row now, and given inside the still-live approval window.
func (d decisionPayload) countsForCycle(currentHash string, now time.Time, expiry time.Duration) bool {
	if d.PayloadHash == "" || d.PayloadHash != currentHash {
		return false
	}
	decided, err := time.Parse(time.RFC3339Nano, d.DecidedAt)
	if err != nil {
		return false
	}
	return now.Sub(decided.UTC()) <= expiry
}

// ── the payload-derived cards, redacted per VALUE at this edge (R3) ─────────

func redactWatchdogFlag(f InboxWatchdogFlag) InboxWatchdogFlag {
	f.Rule, f.AnomalyClass = redact.Redact(f.Rule), redact.Redact(f.AnomalyClass)
	f.Severity, f.Detail = redact.Redact(f.Severity), redact.Redact(f.Detail)
	return f
}

func redactDriftCard(d InboxDriftCard) InboxDriftCard {
	d.Source, d.ChangeClass = redact.Redact(d.Source), redact.Redact(d.ChangeClass)
	d.Severity, d.Summary = redact.Redact(d.Severity), redact.Redact(d.Summary)
	d.RevalidationNote = redact.Redact(d.RevalidationNote)
	lanes := make([]string, len(d.Lanes))
	for i, l := range d.Lanes {
		lanes[i] = redact.Redact(l)
	}
	d.Lanes = lanes
	return d
}

func redactBenchmarkAlarm(a InboxBenchmarkAlarm) InboxBenchmarkAlarm {
	a.Severity, a.Summary = redact.Redact(a.Severity), redact.Redact(a.Summary)
	a.Domain, a.EpochID = redact.Redact(a.Domain), redact.Redact(a.EpochID)
	return a
}

// ── the answer verbs ────────────────────────────────────────────────────────

// The effect action vocabulary (S15.2 family-level "approve / deny").
const (
	effectActionApprove = "approve"
	effectActionDeny    = "deny"
)

// approvalAnswerBody is the one answer envelope (S15.2): the pin the card was
// shown for, the optional same-request PIN (S01.9 step-up), and the answer in
// the card's own vocabulary.
type approvalAnswerBody struct {
	PayloadHash string          `json:"payload_hash"`
	PIN         string          `json:"pin,omitempty"`
	Answer      json.RawMessage `json:"answer"`
}

// ApprovalAnswerResult is one applied (or already-resolved) answer.
type ApprovalAnswerResult struct {
	ID string `json:"id"`
	// Applied is false on a repeat: the item was already resolved and this
	// request changed nothing.
	Applied bool `json:"applied"`
	// State is the item's state after the call — the already-resolved state on
	// a repeat, which is exactly what makes a phone retry safe.
	State  string          `json:"state"`
	Result json.RawMessage `json:"result,omitempty"`
	// Approvals carries the D10 co-approval state back when an effect still
	// needs another signature.
	Approvals *EffectApprovals `json:"approvals,omitempty"`
	Detail    string           `json:"detail,omitempty"`
}

// errStalePayload is the (a) limb of the retry-safety rule: the card moved
// under the answerer. Nothing fires and the fresh card comes back with it.
type staleAnswerError struct {
	item ApprovalItem
}

func (e *staleAnswerError) Error() string {
	return "the card changed since it was shown: re-read it and answer the current card (S15.2 payload-hash pin)"
}

func (s *Server) handleApprovalAnswer(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body approvalAnswerBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	id, _ := IdentityFrom(r.Context())
	res, err := s.applyApprovalAnswer(r.Context(), id, s.readScope(r), r.PathValue("id"), body, false)
	if err != nil {
		s.writeApprovalErr(w, err)
		return
	}
	s.writeReadJSON(w, res)
}

// ApprovalBatchItem is one member of a Low-tier batch: individually pinned,
// individually answered.
type ApprovalBatchItem struct {
	ID          string          `json:"id"`
	PayloadHash string          `json:"payload_hash"`
	Answer      json.RawMessage `json:"answer"`
}

// ApprovalBatchOutcome is one member's own outcome. A batch is a transport
// convenience, never a semantic merge — so every item carries its own status
// and a refusal of one leaves the rest untouched.
type ApprovalBatchOutcome struct {
	ID     string                `json:"id"`
	Status int                   `json:"status"`
	Code   string                `json:"code,omitempty"`
	Detail string                `json:"detail,omitempty"`
	Result *ApprovalAnswerResult `json:"result,omitempty"`
}

// ApprovalBatchResult is the batch response: one outcome per item, in the order
// submitted, plus the cursor a client tails the resulting events from.
type ApprovalBatchResult struct {
	Outcomes []ApprovalBatchOutcome `json:"outcomes"`
	Cursor   int64                  `json:"cursor"`
}

func (s *Server) handleApprovalAnswerBatch(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body struct {
		Items []ApprovalBatchItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	switch {
	case len(body.Items) == 0:
		s.writeSurface(w, nil, badRequest(`missing "items": a batch answers a selected set (S15.6)`))
		return
	case len(body.Items) > answerBatchCap:
		s.writeSurface(w, nil, badRequest(fmt.Sprintf("batch of %d exceeds the %d-item bound", len(body.Items), answerBatchCap)))
		return
	}
	id, _ := IdentityFrom(r.Context())
	scope := s.readScope(r)
	out := ApprovalBatchResult{Outcomes: make([]ApprovalBatchOutcome, 0, len(body.Items))}
	for _, item := range body.Items {
		oc := ApprovalBatchOutcome{ID: item.ID, Status: http.StatusOK}
		res, err := s.applyApprovalAnswer(r.Context(), id, scope,
			item.ID, approvalAnswerBody{PayloadHash: item.PayloadHash, Answer: item.Answer}, true)
		if err != nil {
			se := s.approvalSurfaceErr(err)
			oc.Status, oc.Code, oc.Detail = se.Status, se.Code, se.Msg
		} else {
			oc.Result = &res
		}
		out.Outcomes = append(out.Outcomes, oc)
	}
	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	out.Cursor = cursor
	s.writeReadJSON(w, out)
}

// applyApprovalAnswer is the ONE application path — the single verb the direct
// route and every batch member run through, so the hash check, the scope check
// and the tier gate cannot diverge between them.
func (s *Server) applyApprovalAnswer(ctx context.Context, id Identity, scope ownerScope, routeID string, body approvalAnswerBody, batched bool) (ApprovalAnswerResult, error) {
	kind, native, ok := strings.Cut(routeID, ":")
	if !ok || native == "" {
		return ApprovalAnswerResult{}, badRequest(fmt.Sprintf("bad approval id %q: want %q or %q",
			routeID, approvalKindAsk+":<ask id>", approvalKindEffect+":<effect id>"))
	}
	expiry, stale, err := s.approvalHorizons()
	if err != nil {
		return ApprovalAnswerResult{}, err
	}

	switch kind {
	case approvalKindAsk:
		return s.answerAsk(ctx, id, scope, native, body, batched, expiry, stale)
	case approvalKindEffect:
		return s.answerEffect(ctx, id, scope, native, body, batched)
	}
	return ApprovalAnswerResult{}, &SurfaceError{Status: http.StatusBadRequest, Code: "not_answerable",
		Msg: fmt.Sprintf("cards of kind %q are not answered through this verb; the inbox item names where its verb lives", kind)}
}

// answerAsk applies an answer to a durable ask. The routing spine is the LANDED
// one: Surface.Answer owns onboard / verify / intake semantics and this verb
// invents no ask vocabulary — it adds the pin, the tier gate and the scope.
func (s *Server) answerAsk(ctx context.Context, id Identity, scope ownerScope, askID string, body approvalAnswerBody, batched bool, expiry, stale time.Duration) (ApprovalAnswerResult, error) {
	var (
		owner, status, snapshot string
		answeredTS, answer      sql.NullString
		observed                string
	)
	err := s.proj.db.QueryRowContext(ctx,
		`SELECT user_id, status, snapshot, observed_ts, answered_ts, answer FROM asks WHERE ask_id = ?`, askID).
		Scan(&owner, &status, &snapshot, &observed, &answeredTS, &answer)
	found := !errors.Is(err, sql.ErrNoRows)
	if err != nil && found {
		return ApprovalAnswerResult{}, fmt.Errorf("read ask %q: %w", askID, err)
	}
	// D10: the ask's OWNER answers. The role bit decides VISIBILITY, never
	// authorship of somebody else's decision — so the operator reading another
	// person's card still cannot answer it, which is what the landed pipeline
	// already enforces and what this transport must not undercut.
	if code, cerr := authorizeOwner(ownerScope{UserID: scope.UserID}, owner, found, "approval"); cerr != nil {
		return ApprovalAnswerResult{}, &SurfaceError{Status: code, Code: httpCode(code), Msg: cerr.Error()}
	}
	routeID := approvalKindAsk + ":" + askID

	// (b) repeat / (c) conflict: an already-resolved item never re-applies.
	if answeredTS.Valid && answeredTS.String != "" {
		return resolvedAnswer(routeID, status, json.RawMessage(answer.String), body.Answer)
	}

	tier, actions, storedStale, storedReasons := readCardShape(json.RawMessage(snapshot))
	hash, err := gates.CanonicalHash(json.RawMessage(snapshot))
	if err != nil {
		return ApprovalAnswerResult{}, fmt.Errorf("pin ask %q: %w", askID, err)
	}
	item := ApprovalItem{
		ID: routeID, Kind: approvalKindAsk, Owner: owner, Tier: tier, Answerable: true,
		PayloadHash: hash, ObservedTS: parseTS(observed), Actions: actions,
		Card: json.RawMessage(snapshot),
	}
	applyTierRules(&item)
	applyExpiryAndStaleness(&item, expiry, stale, s.proj.clock(), storedStale, storedReasons)
	if err := checkPin(item, body.PayloadHash); err != nil {
		return ApprovalAnswerResult{}, err
	}
	if err := checkBatchable(item, batched); err != nil {
		return ApprovalAnswerResult{}, err
	}
	pinVerified, err := s.stepUp(ctx, id, item, body.PIN)
	if err != nil {
		return ApprovalAnswerResult{}, err
	}
	if s.intake == nil {
		return ApprovalAnswerResult{}, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the pipeline surface that owns ask answers is not wired in this process"}
	}
	if len(body.Answer) == 0 {
		return ApprovalAnswerResult{}, badRequest(`missing "answer" object`)
	}
	payload, err := s.intake.Answer(ctx, scope.UserID, askID, body.Answer, pinVerified)
	if err != nil {
		return ApprovalAnswerResult{}, err
	}
	// No decision.recorded here: an answered ask already mints its own canonical
	// audit row (the intake pipeline's intake.state / intake.delta_decision, the
	// S07.7 verify events, the ledger's human-decision entry). Minting a second
	// row would make one decision look like two (OQ8).
	return ApprovalAnswerResult{ID: routeID, Applied: true, State: "answered", Result: payload}, nil
}

// answerEffect applies approve/deny to a proposed effect through the S02.7
// journal. Nothing executes here — approval and execution stay different
// recorded facts, and execution is still the journal's own two-phase path (D7).
func (s *Server) answerEffect(ctx context.Context, id Identity, scope ownerScope, effectID string, body approvalAnswerBody, batched bool) (ApprovalAnswerResult, error) {
	if s.effects == nil {
		return ApprovalAnswerResult{}, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S02.7 effect journal is not wired in this process"}
	}
	card, found, err := s.proj.effectCard(ctx, effectID)
	if err != nil {
		return ApprovalAnswerResult{}, err
	}
	routeID := approvalKindEffect + ":" + effectID
	// Scope: the owner always; the operator additionally on a platform-level
	// effect, which is precisely the co-approval D10 gives them.
	visible := ownerScope{UserID: scope.UserID, Operator: scope.Operator && card.PlatformLevel}
	if code, cerr := authorizeOwner(visible, card.Owner, found, "approval"); cerr != nil {
		return ApprovalAnswerResult{}, &SurfaceError{Status: code, Code: httpCode(code), Msg: cerr.Error()}
	}

	var act struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if len(body.Answer) > 0 {
		if err := json.Unmarshal(body.Answer, &act); err != nil {
			return ApprovalAnswerResult{}, badRequest("bad answer object: " + err.Error())
		}
	}
	if act.Action != effectActionApprove && act.Action != effectActionDeny {
		return ApprovalAnswerResult{}, badRequest(fmt.Sprintf(`bad answer action %q: an effect card takes %q or %q`,
			act.Action, effectActionApprove, effectActionDeny))
	}

	state, result, err := s.effectState(ctx, effectID)
	if err != nil {
		return ApprovalAnswerResult{}, err
	}
	if state != string(gates.EffectProposed) {
		return resolvedEffect(routeID, state, act.Action, result)
	}

	item := ApprovalItem{
		ID: routeID, Kind: approvalKindEffect, Owner: card.Owner, Tier: tierHigh, Answerable: true,
		PayloadHash: card.PayloadHash, ObservedTS: card.CreatedTS, Card: card.Payload,
	}
	applyTierRules(&item)
	if err := checkPin(item, body.PayloadHash); err != nil {
		return ApprovalAnswerResult{}, err
	}
	if err := checkBatchable(item, batched); err != nil {
		return ApprovalAnswerResult{}, err
	}
	if _, err := s.stepUp(ctx, id, item, body.PIN); err != nil {
		return ApprovalAnswerResult{}, err
	}

	expiry, _, err := s.approvalHorizons()
	if err != nil {
		return ApprovalAnswerResult{}, err
	}
	appr, err := s.proj.effectApprovals(ctx, card, expiry)
	if err != nil {
		return ApprovalAnswerResult{}, err
	}
	actorIsOperator := scope.Operator
	// The dedup runs on the SAME cycle-scoped derivation as the card (drain D3),
	// so a signature that aged out or hashed out is not "already recorded": a
	// re-approval after a journal reversion mints a FRESH row instead of firing
	// gates.Approve on the strength of a lapsed one.
	alreadyRecorded := (scope.UserID == card.Owner && appr.OwnerApproved) ||
		(actorIsOperator && appr.OperatorApproved)

	decision := decisionPayload{
		Actor: scope.UserID, ActorIsOperator: actorIsOperator,
		CardID: routeID, CardType: approvalKindEffect, Decision: act.Action,
		// The pin the signature was given FOR: it is what makes the row
		// cycle-scoped when it is read back (drain D3).
		PayloadHash: card.PayloadHash,
		Subject:     card.RunID, Reason: act.Reason,
		Ref: "S15.6 approval inbox; S02.7 effect journal",
	}

	if act.Action == effectActionDeny {
		// Either party may stop a proposal: a co-approval is a conjunction, so
		// one refusal ends it. The journal act commits FIRST and the decision row
		// follows it (drain D6) — a row written before a Deny that then failed
		// would be an audit record of a refusal that never took effect, and the
		// effect would still be sitting in the inbox contradicting it.
		e, derr := s.effects.Deny(ctx, effectID, scope.UserID, act.Reason)
		if derr != nil {
			return ApprovalAnswerResult{}, derr
		}
		if err := s.recordDecision(ctx, decision, card.Owner, card.CreatedTS); err != nil {
			return ApprovalAnswerResult{}, err
		}
		return ApprovalAnswerResult{ID: routeID, Applied: true, State: string(e.State), Result: e.Result,
			Detail: "denied: the effect is terminal and was never executed (S02.7)"}, nil
	}

	// An approval is a SIGNATURE. Whether it also completes the conjunction
	// decides the order the two facts are written in, and both orders are
	// honest: a signature that completes nothing is recorded on its own (it
	// happened, and the effect legitimately stays proposed); a signature that
	// completes the conjunction is recorded only once gates.Approve has
	// actually approved, so no row ever claims an approval that did not take.
	ownerSigned := appr.OwnerApproved || scope.UserID == card.Owner
	operatorSigned := appr.OperatorApproved || actorIsOperator
	if !ownerSigned || (appr.PlatformLevel && !operatorSigned) {
		if !alreadyRecorded {
			if err := s.recordDecision(ctx, decision, card.Owner, card.CreatedTS); err != nil {
				return ApprovalAnswerResult{}, err
			}
			if scope.UserID == card.Owner {
				appr.OwnerApproved, appr.OwnerApprovedBy = true, scope.UserID
			}
			if actorIsOperator {
				appr.OperatorApproved, appr.OperatorApprovedBy = true, scope.UserID
			}
		}
		return ApprovalAnswerResult{ID: routeID, Applied: true, State: string(gates.EffectProposed),
			Approvals: &appr,
			Detail:    "recorded: a platform-level effect needs both the owner's and the operator's approval before it is approved (S15.6; D10)"}, nil
	}
	if scope.UserID == card.Owner {
		appr.OwnerApproved, appr.OwnerApprovedBy = true, scope.UserID
	}
	if actorIsOperator {
		appr.OperatorApproved, appr.OperatorApprovedBy = true, scope.UserID
	}
	e, err := s.effects.Approve(ctx, effectID, scope.UserID)
	if err != nil {
		return ApprovalAnswerResult{}, err
	}
	if !alreadyRecorded {
		if err := s.recordDecision(ctx, decision, card.Owner, card.CreatedTS); err != nil {
			return ApprovalAnswerResult{}, err
		}
	}
	return ApprovalAnswerResult{ID: routeID, Applied: true, State: string(e.State), Approvals: &appr,
		Detail: "approved: execution is still the journal's two-phase path, never this verb (D7)"}, nil
}

// effectState reads the journal row's state AND its result. The result is not
// decoration: `failed` is reached by two entirely different acts — a human
// denial (OQ2) and a provider execution failure — and only the recorded result
// tells them apart (drain D10).
func (s *Server) effectState(ctx context.Context, effectID string) (string, json.RawMessage, error) {
	var state string
	var result sql.NullString
	err := s.proj.db.QueryRowContext(ctx,
		`SELECT state, result FROM effects WHERE effect_id = ?`, effectID).Scan(&state, &result)
	if err != nil {
		return "", nil, fmt.Errorf("read effect %q: %w", effectID, err)
	}
	if result.Valid {
		return state, json.RawMessage(result.String), nil
	}
	return state, nil, nil
}

// checkPin is the (a) limb: an OPEN item whose current canonical hash differs
// from the one the answerer quoted is rejected and nothing fires. The check
// happens in ONE place, before any verb dispatch.
func checkPin(item ApprovalItem, given string) error {
	if strings.TrimSpace(given) == "" {
		return badRequest(`missing "payload_hash": an answer is pinned to the payload hash it was shown for (S15.2)`)
	}
	if given != item.PayloadHash {
		return &staleAnswerError{item: item}
	}
	return nil
}

func checkBatchable(item ApprovalItem, batched bool) error {
	if batched && !item.Batchable {
		return &SurfaceError{Status: http.StatusConflict, Code: "not_batchable",
			Msg: fmt.Sprintf("a %s-tier card is answered individually, never in a batch (S15.6)", item.Tier)}
	}
	return nil
}

// stepUp enforces the S01.9 High-tier re-prompt: the actor's own PIN, in the
// same request, verified at the act. There is no stored elevation — an idle
// session never carries approval identity (G3 Def.1) — and the dev fallback
// identity can never step up.
func (s *Server) stepUp(ctx context.Context, id Identity, item ApprovalItem, pin string) (bool, error) {
	if !item.StepUpRequired {
		return false, nil
	}
	if pin == "" {
		return false, ErrPINRequired
	}
	if id.Dev {
		return false, &SurfaceError{Status: http.StatusForbidden, Code: "dev_identity",
			Msg: "the dev identity cannot step up (S01.9); log in as a real user"}
	}
	if s.sessions == nil {
		return false, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "no session store is wired: a High-tier answer cannot be stepped up"}
	}
	if err := s.sessions.VerifyPIN(ctx, id.UserID, pin, "High-tier approval step-up (S01.9; S15.6)"); err != nil {
		return false, &SurfaceError{Status: http.StatusUnauthorized, Code: "pin_rejected", Msg: "PIN verification failed"}
	}
	return true, nil
}

// resolvedAnswer is the (b)/(c) limb for asks: the same answer read back at
// 200, a different one refused as a conflict. Comparison runs over the
// CANONICAL form, so key order and whitespace never manufacture a conflict.
func resolvedAnswer(routeID, state string, stored, given json.RawMessage) (ApprovalAnswerResult, error) {
	same, err := sameAnswer(stored, given)
	if err != nil {
		return ApprovalAnswerResult{}, badRequest("bad answer object: " + err.Error())
	}
	if !same {
		return ApprovalAnswerResult{}, &SurfaceError{Status: http.StatusConflict, Code: "already_answered",
			Msg: "this card was already answered differently; the recorded answer stands and is never re-applied (S15.2)"}
	}
	return ApprovalAnswerResult{ID: routeID, Applied: false, State: state, Result: stored,
		Detail: "already answered: the recorded answer is returned and nothing fired again (S15.2 retry-safety)"}, nil
}

// resolvedEffect is the (b)/(c) limb for effects, read off the journal state
// AND the act that produced it.
//
// State alone is not enough (drain D10): `failed` is where a human DENIAL lands
// (OQ2) and also where a provider execution failure lands, and the two are
// different answers to "was this card already decided the way you are deciding
// it?". Reading an approve-after-deny back as "already approved" would tell the
// answerer their approval stood when a refusal is what is on the record. The
// stored result disambiguates, so it is what this reads.
func resolvedEffect(routeID, state, action string, result json.RawMessage) (ApprovalAnswerResult, error) {
	denied := state == string(gates.EffectFailed) && isDenialResult(result)
	// The states an APPROVAL has already passed through: approved, and every
	// state reachable only by executing an approved effect.
	approved := map[string]bool{
		string(gates.EffectApproved): true, string(gates.EffectExecuting): true,
		string(gates.EffectSucceeded): true, string(gates.EffectUnknown): true,
	}[state] || (state == string(gates.EffectFailed) && !denied)

	switch {
	case action == effectActionApprove && approved:
	case action == effectActionDeny && denied:
	case action == effectActionApprove && denied:
		return ApprovalAnswerResult{}, &SurfaceError{Status: http.StatusConflict, Code: "already_answered",
			Msg: "this effect was DENIED; a denial is terminal and is never converted into an approval (S15.2; OQ2)"}
	default:
		return ApprovalAnswerResult{}, &SurfaceError{Status: http.StatusConflict, Code: "already_answered",
			Msg: fmt.Sprintf("this effect is already %s; the recorded decision stands and is never re-applied (S15.2)", state)}
	}
	return ApprovalAnswerResult{ID: routeID, Applied: false, State: state, Result: result,
		Detail: "already decided: the recorded state is returned and nothing fired again (S15.2 retry-safety)"}, nil
}

// isDenialResult reports the OQ2 denial marker on a `failed` row.
func isDenialResult(result json.RawMessage) bool {
	if len(result) == 0 {
		return false
	}
	var r struct {
		Denied bool `json:"denied"`
	}
	return json.Unmarshal(result, &r) == nil && r.Denied
}

func sameAnswer(stored, given json.RawMessage) (bool, error) {
	if len(given) == 0 {
		return false, errors.New(`missing "answer" object`)
	}
	g, err := gates.CanonicalHash(given)
	if err != nil {
		return false, err
	}
	if len(stored) == 0 {
		return false, nil
	}
	sh, err := gates.CanonicalHash(stored)
	if err != nil {
		return false, nil
	}
	return sh == g, nil
}

// ── the S14.2 family-5 co-production ────────────────────────────────────────

// EventDecisionRecorded is the S14.2 canonical generic human-decision type
// (family 5). internal/api joins internal/benchmark as a CO-PRODUCER of it
// here, which is exactly what the contract row pre-authorized: "B6 joins as a
// co-producer when the generic approval-inbox verbs land (S15)". No new type
// and no new family — the inventory is unchanged (CONVENTIONS §29).
//
// It covers exactly the acts with NO landed canonical audit row of their own:
// the effect approve and the effect deny, whose journal transitions are
// effects-row updates rather than run_events appends (§8). Answers that already
// mint their own row (intake, verify) and cancels (carried by
// run.state_changed) are never double-minted (OQ8).
const EventDecisionRecorded = "decision.recorded"

const decisionSchemaVersion = 1

// decisionPayload is the S14.2 Human-decision contract minimum verbatim —
// {actor, card id + type, decision, presented-at → decided-at (latency)} —
// plus the optional context members the family admits.
type decisionPayload struct {
	Actor       string `json:"actor"`
	CardID      string `json:"card_id"`
	CardType    string `json:"card_type"`
	Decision    string `json:"decision"`
	PresentedAt string `json:"presented_at"`
	DecidedAt   string `json:"decided_at"`
	LatencyS    int64  `json:"latency_s"`
	Reason      string `json:"reason,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Ref         string `json:"ref,omitempty"`
	// PayloadHash is the pin the decision was given FOR. It is what makes a
	// recorded signature CYCLE-SCOPED when it is read back (drain D3): after a
	// journal reversion to `proposed` on drift, the hash no longer matches and
	// the old signature stops counting toward the D10 conjunction.
	PayloadHash string `json:"payload_hash,omitempty"`
	// ActorIsOperator distinguishes the D10 co-approval limbs: the owner's
	// approval and the operator's are two different facts about one card, and
	// the co-approval state is derived from these rows.
	ActorIsOperator bool `json:"actor_is_operator,omitempty"`
	// Old / New carry the before-and-after of an EDIT (B6-2B): the budget verb,
	// the pause switch and the drag hint change a stored value, and "who set
	// what, from what" is the whole audit obligation for that class of act
	// (OQ4). They are optional context members the family admits; the acts that
	// decide a card rather than edit a value leave them empty.
	Old json.RawMessage `json:"old,omitempty"`
	New json.RawMessage `json:"new,omitempty"`
}

// recordDecision appends the family-5 row. presentedAt is the card's own issue
// time, so the P-T05-2 decision-latency datum rides every decision rather than
// needing a separate measurement.
//
// The row is owner-attributed (15.6) and carries no run id: the approvals
// family answers cards that are not always run-scoped, and a run-scoped append
// is generation-fenced against a run this transport does not own the generation
// of. The run travels in the payload instead (the intake.followup_spawned
// precedent).
// now is the server's clock: time.Now in production, injected where a test has
// to drive a real minting verb and compare its output (drain r1 D3).
func (s *Server) now() time.Time { return s.clock() }

func (s *Server) recordDecision(ctx context.Context, d decisionPayload, owner string, presentedAt time.Time) error {
	now := s.now().UTC()
	if presentedAt.IsZero() {
		presentedAt = now
	}
	d.PresentedAt = presentedAt.UTC().Format(time.RFC3339Nano)
	d.DecidedAt = now.Format(time.RFC3339Nano)
	if latency := now.Sub(presentedAt.UTC()); latency > 0 {
		d.LatencyS = int64(latency.Seconds())
	}
	payload, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", EventDecisionRecorded, err)
	}
	if _, err := s.log.Append(ctx, eventlog.Append{
		UserID: owner, Type: EventDecisionRecorded, SchemaVersion: decisionSchemaVersion, Payload: payload,
		Time: now,
	}); err != nil {
		return fmt.Errorf("append %s: %w", EventDecisionRecorded, err)
	}
	return nil
}

// ── error mapping ───────────────────────────────────────────────────────────

// writeApprovalErr serves a stale-hash refusal WITH the fresh card, so the
// answerer's next act is a re-read rather than a guess; everything else maps
// through the shared status mapping.
func (s *Server) writeApprovalErr(w http.ResponseWriter, err error) {
	var stale *staleAnswerError
	if errors.As(err, &stale) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(struct {
			Error   string       `json:"error"`
			Detail  string       `json:"detail"`
			Current ApprovalItem `json:"current"`
		}{"stale_payload", stale.Error(), stale.item})
		return
	}
	s.writeSurfaceErr(w, s.approvalSurfaceErr(err))
}

// approvalSurfaceErr maps every answer-path error onto its transport status.
func (s *Server) approvalSurfaceErr(err error) *SurfaceError {
	var se *SurfaceError
	if errors.As(err, &se) {
		return se
	}
	var stale *staleAnswerError
	switch {
	case errors.As(err, &stale):
		return &SurfaceError{Status: http.StatusConflict, Code: "stale_payload", Msg: stale.Error()}
	case errors.Is(err, gates.ErrEffectNotFound):
		return &SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: err.Error()}
	case errors.Is(err, gates.ErrBadState):
		return &SurfaceError{Status: http.StatusConflict, Code: "conflict", Msg: err.Error()}
	case errors.Is(err, gates.ErrApprovalExpired), errors.Is(err, gates.ErrPayloadDrift):
		return &SurfaceError{Status: http.StatusConflict, Code: "stale_payload", Msg: err.Error()}
	}
	s.logger.Error("approvals", "err", err)
	return &SurfaceError{Status: http.StatusInternalServerError, Code: "internal", Msg: err.Error()}
}

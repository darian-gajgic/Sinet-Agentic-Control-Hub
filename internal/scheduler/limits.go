package scheduler

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// The five-class limit-event taxonomy (Spec S10.5, feature 3.2, D4). Limit
// events are NORMAL, recoverable scheduling events, not errors (D4). The wire's
// signals sort into five classes; each selects a fixed action. This classifier
// is the TESTED component the spec demands (per-lane fixtures, limits_test.go):
// budget, policy, and auth events masquerade as one another on the wire, and
// misclassifying an auth event as depletion (retry-parking a revoked lane) is
// the named worst case (P-T08-2). Adapters forward the raw signals as data
// (S03.1); the taxonomy and scheduling live here (S10.5).

// Lane names as they appear on runs.lane (Spec S03.2). Kept local so the
// scheduler stays decoupled from the adapters package (the Dispatcher seam).
const (
	laneAnthropic = "anthropic"
	laneZAI       = "zai"
	laneLocal     = "local"
	laneKimi      = "kimi"
)

// The documented-class vocabulary a lane document may put on a signal (the
// OQ1(a) exemption set; the same four strings the opencode lane document
// declares, pinned to agree by internal/shell because neither package may
// import the other).
//
// The set is CLOSED and has no `auth` and no `balance` member on purpose:
// those are the two directions in which a document edit could suppress a lane
// freeze. A value outside this set is INERT — it classifies exactly as no
// class at all, so no document can ever invent a verdict.
const (
	// DocumentedTransient: capacity shed — retry in place, never park.
	DocumentedTransient = "transient"
	// DocumentedDepletion: an allowance window is spent — park.
	DocumentedDepletion = "depletion"
	// DocumentedModelDrift: the account does not serve the model asked for.
	DocumentedModelDrift = "model-drift"
	// DocumentedEndpointDefect: the request reached the wrong path.
	DocumentedEndpointDefect = "endpoint-defect"
)

// The Z.AI wire codes the taxonomy names (Spec S10.5's signal set). They are
// the classifier's own vocabulary, not lane configuration: the endpoint and the
// model ids are DATA with dates because they move, while these are the fixed
// points S10.5 sorts on. Everything the set does NOT name classifies on its
// SIGNAL instead — see isDepletion/isDepletionNoSignal.
const (
	zaiRateLimit          = "1302" // rate limit reached for requests
	zaiOverloaded         = "1305" // service temporarily overloaded
	zaiUsageLimit         = "1308" // usage limit reached, with next_flush_time
	zaiPeriodLimit        = "1310" // weekly/monthly limit exhausted, with next_flush_time
	zaiInsufficientCredit = "1113" // insufficient balance / no resource package
	zaiUnknownModel       = "1211" // unknown model — drift, not depletion
)

// LimitClass is one of the five wire-observable failure kinds (Spec S10.5).
type LimitClass int

const (
	ClassTransientShed     LimitClass = 1 // retry in place, never park
	ClassDepletionSignal   LimitClass = 2 // park blocked_quota, resume at provider signal
	ClassDepletionNoSignal LimitClass = 3 // park, jittered probe schedule
	ClassAuthPolicy        LimitClass = 4 // lane freeze + operator alert, NEVER retry-park
	ClassEngineCeiling     LimitClass = 5 // died-at-gate handling
	ClassNone              LimitClass = 0 // not a limit event
)

// ActionKind is the fixed action a class selects (Spec S10.5).
type ActionKind string

const (
	ActionContinue     ActionKind = "continue"           // not a limit event
	ActionRetryInPlace ActionKind = "retry-in-place"     // Class 1
	ActionParkQuota    ActionKind = "park-blocked-quota" // Class 2
	ActionParkProbe    ActionKind = "park-probe"         // Class 3
	ActionLaneFreeze   ActionKind = "lane-freeze"        // Class 4
	ActionDiedAtGate   ActionKind = "died-at-gate"       // Class 5
)

// LimitSignal is the normalized wire signal an adapter forwards (Spec S03.1;
// B1-1 already emits engine.rate_limit observations). It is deliberately raw:
// the classifier, not the adapter, decides the class (D4).
type LimitSignal struct {
	Lane string

	HTTPStatus int    // 529, 429, 401, 402, 403, ...
	ErrorCode  string // Z.AI 1302/1305/1308/1310/1113; provider codes

	// RateLimitStatus is the Anthropic stream rate_limit_event status
	// ("allowed", "allowed_warning", "rejected", ...).
	RateLimitStatus string
	// ResetAt is the provider-SIGNALED resume time when present
	// (rate_limit_event.resetsAt / Z.AI next_flush_time / opencode retry.next).
	ResetAt time.Time

	// BodyText carries policy-ban text that arrives on VALID credentials
	// (S10.5): a policy event, not a depletion event.
	BodyText string
	// OnValidCredentials is the auth-canary fact (P-T17-1): the credentials
	// are known-good, so a 401/402/403 or ban text is a POLICY/BUDGET event,
	// not a stale-credential failure. Distinguishing these is the P-T08-2 job.
	OnValidCredentials bool

	// EngineBudgetExhausted is Sinet's OWN engine backstop tripping
	// (error_max_budget_usd / budget_exhausted) — Class 5 (S10.5, S10.8).
	EngineBudgetExhausted bool

	// EndpointVerified reports that the lane's CONFIGURED endpoint was checked
	// and is the subscription endpoint (S10.5's Class-3 row: "Z.AI 1113 after
	// endpoint self-check"). It is an INPUT, never a lookup: Classify stays
	// pure and total, so the fact is established by whoever owns the lane
	// config and handed here as data.
	//
	// It matters because one code carries two unrelated meanings. On the
	// subscription endpoint, "insufficient balance" means the plan is spent —
	// park and probe. On the general endpoint, the subscription does not apply
	// at all and the same code means the lane is MISCONFIGURED; parking that
	// on a probe schedule would wait forever for a balance nobody is going to
	// top up (P-T08-2's failure class).
	EndpointVerified bool

	// DocumentedClass is what the LANE DOCUMENT names this exact
	// (HTTP status, message) pair, resolved by whoever holds the document and
	// forwarded here as data — never looked up, because Classify is pure and
	// total (S10.5), exactly as EndpointVerified above.
	//
	// It exists because one lane's wire does not sort by status at all. Kimi
	// publishes quota exhaustion on 403 AND 429 and entitlement failures on
	// 401, so the unconditional Class-4 status rule would freeze that lane and
	// page the operator with a suspected policy revocation every time its
	// weekly window emptied — routine, recurring, and indistinguishable at the
	// alert from the real thing.
	DocumentedClass string
}

// SurfaceKind names a NON-limit condition worth an operator's attention. The
// five classes are for limit events; these are not limit events, schedule
// nothing, and exist so a real defect is not silently indistinguishable from a
// healthy "carry on".
type SurfaceKind string

const (
	SurfaceNone SurfaceKind = ""
	// SurfaceEndpointDefect: the lane is pointed at the wrong endpoint.
	SurfaceEndpointDefect SurfaceKind = "endpoint-misconfigured"
	// SurfaceModelDrift: the provider does not know the model that was asked
	// for — the P-T17-3 drift symptom, and the model-list canary's input.
	SurfaceModelDrift SurfaceKind = "model-drift"
)

// WirePayload is the shape an adapter forwards on a rate_limit event (S03.1:
// adapters forward the raw signals as DATA; the taxonomy lives here).
//
// It exists as its own type, with its own tags and an EXPLICIT conversion,
// rather than as json tags on LimitSignal, because the two are not the same
// thing: LimitSignal also carries facts no adapter can know (OnValidCredentials
// is the auth canary's, EngineBudgetExhausted is Sinet's own backstop). Letting
// a decode fill the whole struct would silently zero those — and a zeroed
// EndpointVerified turns a genuine depletion into an "endpoint misconfigured"
// verdict, which is exactly the mistranslation this type is here to prevent.
type WirePayload struct {
	Lane             string `json:"lane"`
	ErrorCode        string `json:"error_code"`
	HTTPStatus       int    `json:"http_status"`
	ResetAt          string `json:"reset_at"`
	BodyText         string `json:"body_text"`
	EndpointVerified bool   `json:"endpoint_verified"`
	Known            bool   `json:"known"`
	DocumentedClass  string `json:"documented_class"`
}

// SignalFromPayload decodes one forwarded rate_limit payload into a signal.
// The caller adds what only it knows (OnValidCredentials, EngineBudgetExhausted)
// before classifying. An absent reset_at means NO signalled resume time —
// never a zero time presented as one.
func SignalFromPayload(raw []byte) (LimitSignal, error) {
	var w WirePayload
	if err := json.Unmarshal(raw, &w); err != nil {
		return LimitSignal{}, fmt.Errorf("scheduler: decode forwarded limit signal: %w", err)
	}
	sig := LimitSignal{
		Lane:             w.Lane,
		ErrorCode:        w.ErrorCode,
		HTTPStatus:       w.HTTPStatus,
		BodyText:         w.BodyText,
		EndpointVerified: w.EndpointVerified,
		DocumentedClass:  w.DocumentedClass,
	}
	if w.ResetAt != "" {
		t, err := time.Parse(time.RFC3339, w.ResetAt)
		if err != nil {
			// A resume time nobody can read is no resume time: the signal is
			// still forwarded and classifies in the safe direction (a probe
			// park), rather than the whole event being discarded.
			return sig, fmt.Errorf("scheduler: forwarded limit signal for lane %q carries an unparseable reset_at %q: %w",
				w.Lane, w.ResetAt, err)
		}
		sig.ResetAt = t
	}
	return sig, nil
}

// Action is the classifier's verdict: the class and the fixed scheduling
// action, with the ⚙-derived parameters the action needs.
type Action struct {
	Class  LimitClass
	Kind   ActionKind
	Reason string

	// Surface names a non-limit condition to show an operator (see
	// SurfaceKind). Empty on every limit event.
	Surface SurfaceKind

	// Class 1 (retry in place): full jitter within the per-request cap and the
	// per-lane retry budget (Spec S10.5). The live retry loop + circuit
	// breaker ride the dispatch path; these carry the ratified policy.
	RetryCap         int64
	RetryBudgetRatio float64

	// Class 2 (depletion + signal): park blocked_quota, auto-resume at the
	// PROVIDER-signaled time (Spec S10.5).
	ResumeAt time.Time

	// Class 3 (depletion − signal): park, jittered probe schedule capped at
	// ⚙ limit.probe_interval_max; probe resumes are zero-cost and never count
	// as attempts or spend (Spec S10.5).
	ProbeIntervalMax time.Duration
}

// LimitConfig carries the three S10.5 ⚙ values, loaded once from the registry
// so Classify stays a pure, fixture-testable function.
type LimitConfig struct {
	RetryCap         int64
	RetryBudgetRatio float64
	ProbeIntervalMax time.Duration
}

// ⚙ keys of the S10.5 limit taxonomy, owned by Spec S10.
const (
	keyRetryCap         = "limit.retry_cap"
	keyRetryBudgetRatio = "limit.retry_budget_ratio"
	keyProbeIntervalMax = "limit.probe_interval_max"
)

// LoadLimitConfig reads the three limit ⚙ values (Spec S10.5).
func LoadLimitConfig(s Settings) (LimitConfig, error) {
	cap, err := s.Int(keyRetryCap)
	if err != nil {
		return LimitConfig{}, fmt.Errorf("scheduler: read ⚙ %s: %w", keyRetryCap, err)
	}
	ratio, err := s.Float(keyRetryBudgetRatio)
	if err != nil {
		return LimitConfig{}, fmt.Errorf("scheduler: read ⚙ %s: %w", keyRetryBudgetRatio, err)
	}
	probe, err := s.Duration(keyProbeIntervalMax)
	if err != nil {
		return LimitConfig{}, fmt.Errorf("scheduler: read ⚙ %s: %w", keyProbeIntervalMax, err)
	}
	return LimitConfig{RetryCap: cap, RetryBudgetRatio: ratio, ProbeIntervalMax: probe}, nil
}

// Classify sorts a wire signal into its class and action (Spec S10.5). The
// order of tests is deliberate and load-bearing:
//
//	Class 4 (auth/policy) is checked FIRST so an auth/policy event can NEVER
//	fall through to a retry-park (P-T08-2, the named worst case);
//	Class 5 (engine ceiling) next — Sinet's own backstop;
//	Class 1 (transient shed) — retry in place;
//	Class 2 (depletion + provider signal) — park with the signaled resume;
//	Class 3 (depletion, no signal) — park with a probe schedule.
func Classify(sig LimitSignal, cfg LimitConfig) Action {
	// The narrow, document-driven EXEMPTION to the Class-4 status rule below.
	//
	// It is an exemption LIST and never a re-ordering: it fires only where a
	// lane's own document explicitly names this exact (status, message) pair as
	// depletion or model drift, so an unmatched bare 401/402/403 still freezes
	// and the safe default survives intact. Genuine-revocation rows carry no
	// documented class at all, which is why they cannot be suppressed by a data
	// edit — and the vocabulary itself has no `auth` member, so no document can
	// claim an auth failure is something else.
	//
	// It exists because one lane's wire does not sort by status. Kimi publishes
	// weekly quota exhaustion on 403 and membership-tier gates on 401, so
	// without this the lane would freeze itself and page the operator with a
	// suspected policy revocation every time its weekly window emptied —
	// routine, recurring, and indistinguishable at the alert from the real
	// thing. (OQ1(a); the fuller ratified `semantics` member is the
	// pre-registered later path when a third coded lane arrives.)
	if exemptsAuthStatusFreeze(sig) {
		if act, ok := classifyDocumentedSignal(sig, cfg); ok {
			return act
		}
	}

	// Class 4: auth / policy. 401/402/403, or policy-ban text on VALID
	// credentials. NEVER retry-park a lane that is frozen for auth/policy
	// (S10.5; the P-T17-1 canary drives the freeze).
	if sig.HTTPStatus == 401 || sig.HTTPStatus == 402 || sig.HTTPStatus == 403 {
		return Action{Class: ClassAuthPolicy, Kind: ActionLaneFreeze,
			Reason: fmt.Sprintf("auth/policy HTTP %d — lane freeze, never retry-park (P-T08-2)", sig.HTTPStatus)}
	}
	// Class 5: Sinet's own engine ceiling backstop tripped (S10.5, S10.8).
	if sig.EngineBudgetExhausted {
		return Action{Class: ClassEngineCeiling, Kind: ActionDiedAtGate,
			Reason: "engine budget/ceiling backstop tripped — died-at-gate handling (S10.8)"}
	}

	// The zai lane's CODED signals are classified from the code, and they run
	// BEFORE the policy-ban text heuristic below. That order is load-bearing:
	// the provider ships the word "violations" inside the documented message
	// of its own usage-limit band (1311-1321, "Various subscription/usage
	// limit violations"), so a keyword scan would read an ordinary depletion
	// as a revocation and freeze a lane that is merely out of credits — the
	// mirror image of P-T08-2 and just as wrong.
	if sig.Lane == laneZAI && sig.ErrorCode != "" {
		if act, ok := classifyZAICode(sig, cfg); ok {
			return act
		}
	}

	// A MESSAGE-keyed lane's documented signals, on every status the exemption
	// above did not already claim. They run here, in the zai coded branch's
	// place in the ladder, and for the same reason: a documented row is what the
	// provider published, and a text heuristic guessing at it afterwards is
	// strictly worse information arriving later.
	if act, ok := classifyDocumentedSignal(sig, cfg); ok {
		return act
	}

	// Policy-ban text on VALID credentials. It stays a heuristic and stays
	// narrow: a false negative falls through to depletion handling, which is
	// recoverable, and the P-T17-1 auth canary is the authoritative freeze
	// trigger either way (S10.5, S03.6).
	if sig.OnValidCredentials && sig.BodyText != "" && looksLikePolicyBan(sig.BodyText) {
		return Action{Class: ClassAuthPolicy, Kind: ActionLaneFreeze,
			Reason: "policy-ban text on valid credentials — lane freeze, never retry-park (P-T08-2)"}
	}

	// Class 1: transient shed — retry in place, never park, never count
	// against quota (S10.5).
	if isTransient(sig) {
		return Action{Class: ClassTransientShed, Kind: ActionRetryInPlace,
			RetryCap: cfg.RetryCap, RetryBudgetRatio: cfg.RetryBudgetRatio,
			Reason: "transient shed — retry in place with full jitter (S10.5)"}
	}

	// Class 2: depletion WITH a provider-signaled resume time (S10.5).
	if !sig.ResetAt.IsZero() && isDepletion(sig) {
		return Action{Class: ClassDepletionSignal, Kind: ActionParkQuota, ResumeAt: sig.ResetAt,
			Reason: "depletion with provider signal — park blocked_quota, resume at signal (S10.5)"}
	}

	// Class 3: depletion WITHOUT a signal — park with a jittered probe
	// schedule (S10.5). Z.AI 1113 after endpoint self-check; undocumented
	// concurrency caps.
	if isDepletionNoSignal(sig) {
		return Action{Class: ClassDepletionNoSignal, Kind: ActionParkProbe, ProbeIntervalMax: cfg.ProbeIntervalMax,
			Reason: "depletion without signal — park, jittered probe schedule (S10.5)"}
	}

	// Not a limit event (e.g. an "allowed" rate_limit_event observation).
	return Action{Class: ClassNone, Kind: ActionContinue, Reason: "not a limit event"}
}

// exemptsAuthStatusFreeze reports whether a documented row lifts the Class-4
// status rule for this signal.
//
// THREE narrowings carry the whole safety argument, and the third is a BELT
// added at P3-LN-3 drain r1 after a probe defeated the first two.
//
//  1. Only an AUTH-SHAPED status is ever exempted.
//  2. And only by a row naming DEPLETION or MODEL DRIFT — a documented
//     transient or endpoint-defect row on a 401/403 is deliberately not
//     enough, because those are not readings a provider publishes for an
//     auth-shaped status and accepting them would widen the hole for nothing.
//  3. And only when the body carries NO revocation- or policy-ban-shaped
//     text, whatever row matched.
//
// (3) exists because a real body is not one phrase. A provider that suspends an
// account can say so in a sentence that ALSO contains an ordinary depletion or
// entitlement phrase — "Access terminated. Your account was suspended; you have
// also reached your usage limit for this billing cycle." — and a substring match
// over the whole body then hands a genuine revocation an exemption. Probed
// through the real shipped document, that body parked instead of freezing, and
// "…has been suspended for a policy violation and does not have access to this
// service." classified as model drift and simply continued. Reordering document
// rows fixes one of those and not the other; only a belt on the EXEMPTION
// ITSELF is a net, because it does not care which row won.
//
// The belt has no OnValidCredentials precondition, deliberately: it only
// DECLINES an exemption, handing the signal back to the ratified Class-4 status
// rule, which is itself unconditional. Requiring the auth canary to have run
// first would leave the hole open exactly when nothing else is watching. Its
// false-positive direction is a freeze on a lane that was merely depleted —
// recoverable, operator-visible, and the safe half of P-T08-2.
func exemptsAuthStatusFreeze(sig LimitSignal) bool {
	switch sig.HTTPStatus {
	case 401, 402, 403:
	default:
		return false
	}
	if sig.DocumentedClass != DocumentedDepletion && sig.DocumentedClass != DocumentedModelDrift {
		return false
	}
	if sig.BodyText != "" && (looksLikeRevocation(sig.BodyText) || looksLikePolicyBan(sig.BodyText)) {
		return false
	}
	return true
}

// classifyDocumentedSignal is the whole of a message-keyed lane's taxonomy: the
// verdict its own document names for this exact (status, message) pair.
//
// It reports ok=false for a signal carrying no documented class — including one
// carrying a value outside the closed vocabulary, which is INERT by
// construction, so no document edit can ever mint a verdict the classifier does
// not already define.
func classifyDocumentedSignal(sig LimitSignal, cfg LimitConfig) (Action, bool) {
	switch sig.DocumentedClass {
	case DocumentedModelDrift:
		// The account does not serve the model asked for — a membership tier
		// that no longer carries it, or a renamed id. Parking a run for that
		// waits forever for something nobody is going to change, and freezing
		// the lane hides the one fact worth acting on: the account's observed
		// model list is the authority (P-T17-3), and its canary is what acts.
		return Action{Class: ClassNone, Kind: ActionContinue, Surface: SurfaceModelDrift,
			Reason: "the provider's own documented message says this account does not serve the requested model — " +
				"model drift, not a limit event and not a revocation: the observed model list is the authority (P-T17-3)"}, true

	case DocumentedEndpointDefect:
		return Action{Class: ClassNone, Kind: ActionContinue, Surface: SurfaceEndpointDefect,
			Reason: "the provider's own documented message says this request reached a path it does not serve — a " +
				"configuration defect, not a limit event: fix the endpoint, do not retry and do not park (P-T08-2)"}, true

	case DocumentedDepletion:
		if !sig.ResetAt.IsZero() {
			return Action{Class: ClassDepletionSignal, Kind: ActionParkQuota, ResumeAt: sig.ResetAt,
				Reason: "a documented depletion message carrying a provider resume time — park blocked_quota, " +
					"resume at signal (S10.5 Class 2)"}, true
		}
		return Action{Class: ClassDepletionNoSignal, Kind: ActionParkProbe, ProbeIntervalMax: cfg.ProbeIntervalMax,
			Reason: "a documented depletion message with no published resume time — park, jittered probe schedule " +
				"(S10.5 Class 3). This lane's allowance windows split across several HTTP statuses, so the message " +
				"is what says the plan is spent rather than that the credential is bad"}, true

	case DocumentedTransient:
		// The ONLY branch here from which a retry is reachable, so the
		// revocation belt is folded in AHEAD of it — a lane that has been
		// suspended does not become healthy because the provider also happened
		// to shed the request (S10.5 Class 4 outranks Class 1; P-T08-2).
		if sig.OnValidCredentials && sig.BodyText != "" && looksLikeRevocation(sig.BodyText) {
			return Action{Class: ClassAuthPolicy, Kind: ActionLaneFreeze,
				Reason: "a documented transient message whose body carries explicit revocation text on VALID " +
					"credentials — lane freeze, never retry (S10.5 Class 4 outranks Class 1; P-T08-2)"}, true
		}
		return Action{Class: ClassTransientShed, Kind: ActionRetryInPlace,
			RetryCap: cfg.RetryCap, RetryBudgetRatio: cfg.RetryBudgetRatio,
			Reason: "a documented transient-shed message — retry in place with full jitter (S10.5 Class 1)"}, true
	}
	return Action{}, false
}

// zaiAuthCodes are the lane's authentication/permission codes. They are
// classified by CODE as well as by status, because the status is a fact the
// engine may not have surfaced — and an auth event that arrives without one
// must still freeze rather than fall through to a park (P-T08-2).
var zaiAuthCodes = map[string]bool{"1000": true, "1001": true, "1003": true, "1220": true}

// classifyZAICode is the whole of the zai lane's code-driven taxonomy, in one
// place and ahead of every text heuristic. It reports ok=false only when the
// code says nothing at all, and then the general ladder applies.
func classifyZAICode(sig LimitSignal, cfg LimitConfig) (Action, bool) {
	switch {
	case zaiAuthCodes[sig.ErrorCode]:
		return Action{Class: ClassAuthPolicy, Kind: ActionLaneFreeze,
			Reason: fmt.Sprintf("zai %s is an authentication/permission code — lane freeze, never retry-park (P-T08-2)",
				sig.ErrorCode)}, true

	case sig.ErrorCode == zaiUnknownModel:
		// An unknown MODEL is drift, not depletion. Parking a run because the
		// provider renamed a model would hide the one fact worth acting on.
		return Action{Class: ClassNone, Kind: ActionContinue, Surface: SurfaceModelDrift,
			Reason: "the provider does not recognise the requested model — model drift, not a limit event: " +
				"the account's observed model list is the authority (P-T17-3)"}, true

	case sig.ErrorCode == zaiInsufficientCredit && !sig.EndpointVerified:
		// "Insufficient balance" on a lane that is not pointed at the
		// subscription endpoint is the documented symptom of the
		// MISCONFIGURATION, not of a spent plan: the subscription does not
		// apply on the general endpoint, so the pay-as-you-go balance answers
		// instead. Probe-parking that would wait forever for a top-up nobody
		// is going to make — the P-T08-2 failure class exactly (R11).
		return Action{Class: ClassNone, Kind: ActionContinue, Surface: SurfaceEndpointDefect,
			Reason: "this lane reported insufficient balance and its configured endpoint is NOT the " +
				"verified subscription endpoint — a configuration defect, not a limit event: fix the endpoint, " +
				"do not retry and do not park (S10.5 Class-3 self-check)"}, true

	case sig.ErrorCode == zaiRateLimit || sig.ErrorCode == zaiOverloaded:
		// A policy signal must NEVER reach a retry (P-T08-2's named worst
		// case), and this is the only zai branch where that is reachable —
		// every other verdict below is a park. So the revocation test is
		// folded in AHEAD of the transient verdict rather than left to run
		// after the coded taxonomy: a lane that has been suspended does not
		// become healthy because the provider also happened to shed the
		// request.
		if sig.OnValidCredentials && sig.BodyText != "" && looksLikeRevocation(sig.BodyText) {
			return Action{Class: ClassAuthPolicy, Kind: ActionLaneFreeze,
				Reason: fmt.Sprintf("zai %s is a transient code, but its body carries explicit revocation text on "+
					"VALID credentials — lane freeze, never retry (S10.5 Class 4 outranks Class 1; P-T08-2)",
					sig.ErrorCode)}, true
		}
		return Action{Class: ClassTransientShed, Kind: ActionRetryInPlace,
			RetryCap: cfg.RetryCap, RetryBudgetRatio: cfg.RetryBudgetRatio,
			Reason: "transient shed — retry in place with full jitter (S10.5)"}, true

	case sig.ErrorCode == zaiInsufficientCredit:
		return Action{Class: ClassDepletionNoSignal, Kind: ActionParkProbe, ProbeIntervalMax: cfg.ProbeIntervalMax,
			Reason: "insufficient plan balance on the verified subscription endpoint — park, jittered probe " +
				"schedule (S10.5 Class 3, after the endpoint self-check)"}, true

	case !sig.ResetAt.IsZero():
		// Depletion WITH a provider signal, whatever the code number: the
		// class is defined by the signal, not by a number the provider may
		// not have published yet (S10.5's own class definitions).
		return Action{Class: ClassDepletionSignal, Kind: ActionParkQuota, ResumeAt: sig.ResetAt,
			Reason: "depletion with provider signal — park blocked_quota, resume at signal (S10.5)"}, true
	}

	// Everything else this lane can say. A code the taxonomy does not name —
	// including one Z.AI adds after this seed's verified-on date, and one whose
	// HTTP status the engine never surfaced — parks on the bounded probe
	// schedule. Never Class 1 (a retry storm against a plan that may be spent)
	// and never Class 4 (freezing a lane that is merely out of credits): the
	// two directions with no safe failure. The indistinguishable case — an
	// unnamed code that is REALLY a revocation — is exactly what the P-T17-1
	// auth canary exists to catch, and it is the authoritative freeze trigger
	// (S03.6, S14.6).
	return Action{Class: ClassDepletionNoSignal, Kind: ActionParkProbe, ProbeIntervalMax: cfg.ProbeIntervalMax,
		Reason: fmt.Sprintf("zai %s is not in the taxonomy's signal set — parking on the probe schedule, the "+
			"safe direction for an unnamed code; the auth canary remains the revocation detector (S10.5, P-T08-2)",
			sig.ErrorCode)}, true
}

// isTransient matches Class-1 wire signals per lane (Spec S10.5).
func isTransient(sig LimitSignal) bool {
	switch sig.Lane {
	case laneAnthropic:
		// 529 overloaded, or a subscriber transient-429 that carries no
		// depletion reset signal.
		if sig.HTTPStatus == 529 {
			return true
		}
		if sig.HTTPStatus == 429 && sig.ResetAt.IsZero() && sig.RateLimitStatus != "rejected" {
			return true
		}
	case laneZAI:
		// Every CODED zai signal was already decided by classifyZAICode, so
		// only a codeless one reaches here — a bare 429 from a probe or a
		// canary, with no code to say it is transient. Retrying that against a
		// plan that may be spent is the one direction with no safe failure, so
		// it falls through to the depletion ladder instead (R13).
		return false
	case laneKimi:
		// Every DOCUMENTED kimi signal was already decided above, so only an
		// unmatched one reaches here — a message this seed's verified-on date
		// does not name, or a bare status from a probe. Same reasoning as zai:
		// retrying an unrecognised signal against a plan that may be spent has
		// no safe failure, so it falls to the depletion ladder.
		return false
	}
	return false
}

// isDepletion matches Class-2 depletion signals (with reset) per lane (S10.5).
func isDepletion(sig LimitSignal) bool {
	switch sig.Lane {
	case laneAnthropic:
		return sig.RateLimitStatus == "rejected" || sig.HTTPStatus == 429
	case laneZAI, laneKimi:
		// Coded (zai) and documented (kimi) signals never reach here — their
		// own branches own them. A codeless/undocumented 429 that nonetheless
		// carries a resume time is still depletion-with-signal: the CLASS
		// follows the signal, not a number or a phrase nobody published yet.
		return sig.HTTPStatus == 429
	default:
		// opencode/other: a bare retry.next reset is a depletion signal.
		return true
	}
}

// isDepletionNoSignal matches Class-3 signals (no reset) per lane (S10.5).
func isDepletionNoSignal(sig LimitSignal) bool {
	switch sig.Lane {
	case laneZAI, laneKimi:
		// Codeless/undocumented again: a bare 429 with no resume time parks on
		// the probe schedule rather than being read as "not a limit event",
		// which is what a canary-shaped signal (status only, no body) produces.
		return sig.HTTPStatus == 429
	}
	// A rejected/429 depletion with no reset time is depletion-without-signal
	// on any lane (undocumented concurrency caps).
	return sig.RateLimitStatus == "rejected" || sig.HTTPStatus == 429
}

// looksLikeRevocation is the NARROW test used where a limit code and a
// revocation are competing readings of one signal (the zai transient branch).
//
// The bare "violat" stem that looksLikePolicyBan accepts is deliberately
// ABSENT here, and that omission is the whole point: Z.AI documents its own
// usage-limit band as "Various subscription/usage limit violations", so on this
// wire that stem is LIMIT vocabulary, not revocation vocabulary. Reading it as
// a ban is exactly the mistake that froze healthy lanes before drain r1; only
// words a limit message has no reason to use may promote a shed into a freeze.
// "policy" and "terminated" fail that same criterion ("rate limit policy",
// "connection terminated due to overload" are ordinary limit/overload prose)
// and are excluded too; a missed revocation falls through to a recoverable
// verdict and is caught by the auth codes, the 401/403 status rule, or the
// P-T17-1 canary — the authoritative detectors.
func looksLikeRevocation(body string) bool {
	low := strings.ToLower(body)
	for _, kw := range []string{"suspend", "banned", "revoke", "prohibit", "deactivat"} {
		if strings.Contains(low, kw) {
			return true
		}
	}
	return false
}

// looksLikePolicyBan is a conservative match for policy-ban text arriving on
// valid credentials (Spec S10.5). It is intentionally narrow: a false negative
// falls through to depletion handling (recoverable), while the auth canary
// (P-T17-1, Spec S03.6) is the authoritative freeze trigger.
func looksLikePolicyBan(body string) bool {
	low := strings.ToLower(body)
	for _, kw := range []string{"policy", "banned", "ban", "violat", "prohibit", "suspend"} {
		if strings.Contains(low, kw) {
			return true
		}
	}
	return false
}

// JitterInterval applies full jitter to a base interval, capped at max (Spec
// S10.5 "full jitter"; used by Class-3 probe scheduling). rng may be nil.
func JitterInterval(base, max time.Duration, rng *rand.Rand) time.Duration {
	if base > max {
		base = max
	}
	if base <= 0 {
		return 0
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	// Full jitter: uniform in [0, base].
	return time.Duration(rng.Int63n(int64(base) + 1))
}

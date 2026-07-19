package scheduler

import (
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
}

// Action is the classifier's verdict: the class and the fixed scheduling
// action, with the ⚙-derived parameters the action needs.
type Action struct {
	Class  LimitClass
	Kind   ActionKind
	Reason string

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
	// Class 4: auth / policy. 401/402/403, or policy-ban text on VALID
	// credentials. NEVER retry-park a lane that is frozen for auth/policy
	// (S10.5; the P-T17-1 canary drives the freeze).
	if sig.HTTPStatus == 401 || sig.HTTPStatus == 402 || sig.HTTPStatus == 403 {
		return Action{Class: ClassAuthPolicy, Kind: ActionLaneFreeze,
			Reason: fmt.Sprintf("auth/policy HTTP %d — lane freeze, never retry-park (P-T08-2)", sig.HTTPStatus)}
	}
	if sig.OnValidCredentials && sig.BodyText != "" && looksLikePolicyBan(sig.BodyText) {
		return Action{Class: ClassAuthPolicy, Kind: ActionLaneFreeze,
			Reason: "policy-ban text on valid credentials — lane freeze, never retry-park (P-T08-2)"}
	}

	// Class 5: Sinet's own engine ceiling backstop tripped (S10.5, S10.8).
	if sig.EngineBudgetExhausted {
		return Action{Class: ClassEngineCeiling, Kind: ActionDiedAtGate,
			Reason: "engine budget/ceiling backstop tripped — died-at-gate handling (S10.8)"}
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
		return sig.ErrorCode == "1302" || sig.ErrorCode == "1305"
	}
	return false
}

// isDepletion matches Class-2 depletion signals (with reset) per lane (S10.5).
func isDepletion(sig LimitSignal) bool {
	switch sig.Lane {
	case laneAnthropic:
		return sig.RateLimitStatus == "rejected" || sig.HTTPStatus == 429
	case laneZAI:
		return sig.ErrorCode == "1308" || sig.ErrorCode == "1310"
	default:
		// opencode/other: a bare retry.next reset is a depletion signal.
		return true
	}
}

// isDepletionNoSignal matches Class-3 signals (no reset) per lane (S10.5).
func isDepletionNoSignal(sig LimitSignal) bool {
	switch sig.Lane {
	case laneZAI:
		return sig.ErrorCode == "1113"
	}
	// A rejected/429 depletion with no reset time is depletion-without-signal
	// on any lane (undocumented concurrency caps).
	return sig.RateLimitStatus == "rejected" || sig.HTTPStatus == 429
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

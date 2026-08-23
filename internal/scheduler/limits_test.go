package scheduler

import (
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

// The five-class taxonomy is a TESTED component with per-lane fixtures (Spec
// S10.5, P-T08-2): budget/policy/auth events masquerade as one another on the
// wire, and misclassifying an auth event as depletion (retry-parking a revoked
// lane) is the named worst case.

func TestClassifyPerLaneFixtures(t *testing.T) {
	cfg := LimitConfig{RetryCap: 3, RetryBudgetRatio: 0.10, ProbeIntervalMax: 30 * time.Minute}
	reset := time.Now().Add(90 * time.Minute)

	cases := []struct {
		name      string
		sig       LimitSignal
		wantClass LimitClass
		wantKind  ActionKind
	}{
		// Class 1 — transient shed (retry in place, never park).
		{"anthropic 529", LimitSignal{Lane: laneAnthropic, HTTPStatus: 529}, ClassTransientShed, ActionRetryInPlace},
		{"anthropic transient 429", LimitSignal{Lane: laneAnthropic, HTTPStatus: 429}, ClassTransientShed, ActionRetryInPlace},
		{"zai 1302", LimitSignal{Lane: laneZAI, ErrorCode: "1302"}, ClassTransientShed, ActionRetryInPlace},
		{"zai 1305", LimitSignal{Lane: laneZAI, ErrorCode: "1305"}, ClassTransientShed, ActionRetryInPlace},

		// Class 2 — depletion WITH provider signal (park blocked_quota).
		{"anthropic rejected + reset", LimitSignal{Lane: laneAnthropic, RateLimitStatus: "rejected", ResetAt: reset}, ClassDepletionSignal, ActionParkQuota},
		{"zai 1308 + next_flush", LimitSignal{Lane: laneZAI, ErrorCode: "1308", ResetAt: reset}, ClassDepletionSignal, ActionParkQuota},
		{"zai 1310 + next_flush", LimitSignal{Lane: laneZAI, ErrorCode: "1310", ResetAt: reset}, ClassDepletionSignal, ActionParkQuota},
		{"opencode retry.next", LimitSignal{Lane: "opencode", ResetAt: reset}, ClassDepletionSignal, ActionParkQuota},

		// Class 3 — depletion WITHOUT signal (park + probe schedule).
		// The self-check is now an INPUT rather than a name (LN-2A/R11): the
		// same code on an unverified endpoint is a configuration defect, so
		// the depletion case has to say which one it is.
		{"zai 1113 self-check", LimitSignal{Lane: laneZAI, ErrorCode: "1113", EndpointVerified: true}, ClassDepletionNoSignal, ActionParkProbe},
		{"anthropic rejected no reset", LimitSignal{Lane: laneAnthropic, RateLimitStatus: "rejected"}, ClassDepletionNoSignal, ActionParkProbe},

		// Class 4 — auth / policy (lane freeze, NEVER retry-park; P-T08-2).
		{"http 401", LimitSignal{Lane: laneAnthropic, HTTPStatus: 401}, ClassAuthPolicy, ActionLaneFreeze},
		{"http 402 budget-as-401", LimitSignal{Lane: laneAnthropic, HTTPStatus: 402}, ClassAuthPolicy, ActionLaneFreeze},
		{"http 403", LimitSignal{Lane: laneZAI, HTTPStatus: 403}, ClassAuthPolicy, ActionLaneFreeze},
		{"policy ban on valid creds", LimitSignal{Lane: laneAnthropic, OnValidCredentials: true, BodyText: "account suspended for policy violation"}, ClassAuthPolicy, ActionLaneFreeze},

		// Class 5 — Sinet's own engine ceiling backstop.
		{"engine budget exhausted", LimitSignal{Lane: laneAnthropic, EngineBudgetExhausted: true}, ClassEngineCeiling, ActionDiedAtGate},

		// Not a limit event.
		{"allowed observation", LimitSignal{Lane: laneAnthropic, RateLimitStatus: "allowed"}, ClassNone, ActionContinue},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.sig, cfg)
			if got.Class != c.wantClass {
				t.Fatalf("class = %d, want %d (%s)", got.Class, c.wantClass, got.Reason)
			}
			if got.Kind != c.wantKind {
				t.Fatalf("action = %q, want %q", got.Kind, c.wantKind)
			}
			// The named worst case: an auth/policy event is NEVER retry-parked.
			if got.Class == ClassAuthPolicy {
				if got.Kind == ActionRetryInPlace || got.Kind == ActionParkQuota || got.Kind == ActionParkProbe {
					t.Fatalf("auth/policy event routed to retry-park (%q) — the P-T08-2 worst case", got.Kind)
				}
			}
		})
	}
}

func TestClassifyCarriesRatifiedParameters(t *testing.T) {
	cfg := LimitConfig{RetryCap: 3, RetryBudgetRatio: 0.10, ProbeIntervalMax: 30 * time.Minute}

	// Class 1 carries the retry policy.
	a1 := Classify(LimitSignal{Lane: laneAnthropic, HTTPStatus: 529}, cfg)
	if a1.RetryCap != 3 || a1.RetryBudgetRatio != 0.10 {
		t.Errorf("class-1 params = cap %d ratio %v, want 3 / 0.10", a1.RetryCap, a1.RetryBudgetRatio)
	}
	// Class 2 carries the provider-signaled resume time.
	reset := time.Now().Add(time.Hour)
	a2 := Classify(LimitSignal{Lane: laneAnthropic, RateLimitStatus: "rejected", ResetAt: reset}, cfg)
	if !a2.ResumeAt.Equal(reset) {
		t.Errorf("class-2 resume = %v, want the provider signal %v", a2.ResumeAt, reset)
	}
	// Class 3 carries the probe interval cap.
	a3 := Classify(LimitSignal{Lane: laneZAI, ErrorCode: "1113", EndpointVerified: true}, cfg)
	if a3.ProbeIntervalMax != 30*time.Minute {
		t.Errorf("class-3 probe cap = %v, want 30m", a3.ProbeIntervalMax)
	}
}

func TestLoadLimitConfigReadsRatifiedDefaults(t *testing.T) {
	reg := settings.New()
	cfg, err := LoadLimitConfig(reg)
	if err != nil {
		t.Fatalf("LoadLimitConfig: %v", err)
	}
	// ⚙ limit.retry_cap=3, limit.retry_budget_ratio=0.10, limit.probe_interval_max=30m.
	if cfg.RetryCap != 3 || cfg.RetryBudgetRatio != 0.10 || cfg.ProbeIntervalMax != 30*time.Minute {
		t.Errorf("config = %+v, want 3 / 0.10 / 30m", cfg)
	}
}

func TestJitterInterval(t *testing.T) {
	for i := 0; i < 100; i++ {
		d := JitterInterval(10*time.Minute, 30*time.Minute, nil)
		if d < 0 || d > 10*time.Minute {
			t.Fatalf("jitter %v out of [0, base]", d)
		}
	}
	// Capped at max.
	if d := JitterInterval(time.Hour, 30*time.Minute, nil); d > 30*time.Minute {
		t.Fatalf("jitter %v exceeds cap", d)
	}
}

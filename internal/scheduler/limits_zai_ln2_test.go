package scheduler

// limits_zai_ln2_test.go — LN-2A: the Z.AI lane's classification, including
// the endpoint self-check S10.5 names and the safe-direction property over the
// whole observed code space. Misclassifying an auth event as depletion, or
// probe-parking a run forever because the lane is pointed at the wrong
// endpoint, are the two named worst cases (P-T08-2).

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func ln2Config() LimitConfig {
	return LimitConfig{RetryCap: 3, RetryBudgetRatio: 0.10, ProbeIntervalMax: 30 * time.Minute}
}

// ── spec 10 · 1113 needs the endpoint self-check ─────────────────────────────

func TestClassify1113RequiresEndpointSelfCheck(t *testing.T) {
	cfg := ln2Config()

	verified := Classify(LimitSignal{Lane: laneZAI, ErrorCode: "1113", HTTPStatus: 429, EndpointVerified: true}, cfg)
	if verified.Class != ClassDepletionNoSignal || verified.Kind != ActionParkProbe {
		t.Fatalf("verified endpoint + 1113 = class %d / %q, want class 3 / park-probe", verified.Class, verified.Kind)
	}
	if verified.ProbeIntervalMax != cfg.ProbeIntervalMax {
		t.Errorf("probe interval = %v, want the ⚙ limit.probe_interval_max value %v", verified.ProbeIntervalMax, cfg.ProbeIntervalMax)
	}
	if verified.Surface != SurfaceNone {
		t.Errorf("a genuine depletion was surfaced as %q", verified.Surface)
	}

	defect := Classify(LimitSignal{Lane: laneZAI, ErrorCode: "1113", HTTPStatus: 429, EndpointVerified: false}, cfg)
	if defect.Class != ClassNone {
		t.Fatalf("unverified endpoint + 1113 = class %d, want 0 — it is not a limit event", defect.Class)
	}
	switch defect.Kind {
	case ActionRetryInPlace, ActionParkQuota, ActionParkProbe, ActionLaneFreeze, ActionDiedAtGate:
		t.Fatalf("a configuration defect selected the scheduling action %q — it must not retry, park or freeze", defect.Kind)
	}
	if defect.Surface != SurfaceEndpointDefect {
		t.Errorf("surface = %q, want %q", defect.Surface, SurfaceEndpointDefect)
	}
	if !strings.Contains(strings.ToLower(defect.Reason), "endpoint") {
		t.Errorf("reason = %q, want plain language naming the wrong endpoint", defect.Reason)
	}
	if !defect.ResumeAt.IsZero() || defect.ProbeIntervalMax != 0 || defect.RetryCap != 0 {
		t.Errorf("a configuration defect carries scheduling parameters: %+v", defect)
	}
}

// ── spec 11 · auth codes freeze the lane ─────────────────────────────────────

func TestClassifyZAIAuthCodesFreezeLane(t *testing.T) {
	cfg := ln2Config()
	for _, c := range []struct {
		code   string
		status int
	}{{"1000", 401}, {"1001", 401}, {"1003", 401}, {"1220", 403}} {
		for _, valid := range []bool{false, true} {
			for _, verified := range []bool{false, true} {
				sig := LimitSignal{Lane: laneZAI, ErrorCode: c.code, HTTPStatus: c.status,
					OnValidCredentials: valid, EndpointVerified: verified}
				got := Classify(sig, cfg)
				if got.Class != ClassAuthPolicy || got.Kind != ActionLaneFreeze {
					t.Errorf("zai %s (HTTP %d, valid=%v, verified=%v) = class %d / %q, want class 4 / lane-freeze",
						c.code, c.status, valid, verified, got.Class, got.Kind)
				}
			}
		}
	}
}

// ── spec 12 · the safe direction, as a property over the code space ──────────

func TestClassifyZAIUnknown429AlwaysParks(t *testing.T) {
	cfg := ln2Config()
	named := map[string]bool{"1302": true, "1305": true, "1308": true, "1310": true, "1113": true}
	reset := time.Now().Add(2 * time.Hour)

	// The band's OWN shipped message contains "violations", and the auth
	// canary sets OnValidCredentials — so a text heuristic running ahead of
	// the coded taxonomy would read an ordinary depletion as a revocation and
	// freeze a healthy lane. Both are varied here for exactly that reason (D6).
	bodies := []string{
		"",
		"Various subscription/usage limit violations",
		"Usage limit reached for `20000` `credits`",
		"your account has been suspended for a policy violation",
	}
	// The status is varied too: an engine that never surfaced one leaves 0,
	// and a code Z.AI adds after this seed's date is not in any table (D7).
	statuses := []int{0, 429}
	codes := []string{}
	for code := 1300; code <= 1399; code++ {
		if s := fmt.Sprint(code); !named[s] {
			codes = append(codes, s)
		}
	}
	codes = append(codes, "1315", "1321", "1400", "9999", "2000")

	for _, s := range codes {
		for _, withReset := range []bool{false, true} {
			for _, body := range bodies {
				for _, valid := range []bool{false, true} {
					for _, status := range statuses {
						sig := LimitSignal{Lane: laneZAI, ErrorCode: s, HTTPStatus: status,
							EndpointVerified: true, BodyText: body, OnValidCredentials: valid}
						if withReset {
							sig.ResetAt = reset
						}
						got := Classify(sig, cfg)
						ctx := fmt.Sprintf("zai %s (reset=%v status=%d valid=%v body=%q)", s, withReset, status, valid, body)
						if got.Kind == ActionRetryInPlace {
							t.Fatalf("%s retries in place — a retry storm against a plan that may be spent", ctx)
						}
						if got.Kind == ActionLaneFreeze || got.Class == ClassAuthPolicy {
							t.Fatalf("%s freezes the lane — an unnamed limit code is not a revocation, and the "+
								"auth canary is the revocation detector (D6/R13)", ctx)
						}
						if got.Kind != ActionParkQuota && got.Kind != ActionParkProbe {
							t.Fatalf("%s = %q, want a park — never 'not a limit event' (D7)", ctx, got.Kind)
						}
						// The sanctioned signal-based fallback: the CLASS
						// follows the signal, not the code (S10.5's own class
						// definitions).
						wantClass := ClassDepletionNoSignal
						if withReset {
							wantClass = ClassDepletionSignal
						}
						if got.Class != wantClass {
							t.Fatalf("%s = class %d, want %d", ctx, got.Class, wantClass)
						}
					}
				}
			}
		}
	}
}

// TestClassifyZAIAuthCodesFreezeWithoutAStatus is D7's other half: the engine
// may not surface an HTTP status at all, and an auth code that arrives without
// one must still freeze rather than fall into the unnamed-code park.
func TestClassifyZAIAuthCodesFreezeWithoutAStatus(t *testing.T) {
	cfg := ln2Config()
	for _, code := range []string{"1000", "1001", "1003", "1220"} {
		got := Classify(LimitSignal{Lane: laneZAI, ErrorCode: code, HTTPStatus: 0, EndpointVerified: true}, cfg)
		if got.Class != ClassAuthPolicy || got.Kind != ActionLaneFreeze {
			t.Errorf("zai %s with no surfaced status = class %d / %q, want the Class-4 freeze", code, got.Class, got.Kind)
		}
	}
}

// TestClassifyBareZAI429Parks is D17: the canary-shaped signal — a status and
// a body, no provider error code at all (internal/watchlist/canary_auth.go's
// probe shape). It must park, not fall through to "not a limit event".
func TestClassifyBareZAI429Parks(t *testing.T) {
	cfg := ln2Config()
	got := Classify(LimitSignal{Lane: laneZAI, HTTPStatus: 429,
		BodyText: "Too Many Requests", OnValidCredentials: true}, cfg)
	if got.Class != ClassDepletionNoSignal || got.Kind != ActionParkProbe {
		t.Errorf("bare zai 429 = class %d / %q, want class 3 / park-probe", got.Class, got.Kind)
	}
	reset := time.Now().Add(time.Hour)
	withSignal := Classify(LimitSignal{Lane: laneZAI, HTTPStatus: 429, ResetAt: reset, OnValidCredentials: true}, cfg)
	if withSignal.Class != ClassDepletionSignal || !withSignal.ResumeAt.Equal(reset) {
		t.Errorf("bare zai 429 + reset = class %d resume %v, want class 2 at the signalled time",
			withSignal.Class, withSignal.ResumeAt)
	}
	// A genuine policy ban with no code still freezes: the heuristic is not
	// removed, it is merely ordered after the coded taxonomy.
	ban := Classify(LimitSignal{Lane: laneZAI, HTTPStatus: 429, OnValidCredentials: true,
		BodyText: "this account is suspended for a policy violation"}, cfg)
	if ban.Class != ClassAuthPolicy || ban.Kind != ActionLaneFreeze {
		t.Errorf("codeless policy-ban text = class %d / %q, want the Class-4 freeze", ban.Class, ban.Kind)
	}
}

// ── spec 13 · 1211 is model drift, not a limit event ─────────────────────────

func TestClassify1211IsNotALimitEvent(t *testing.T) {
	got := Classify(LimitSignal{Lane: laneZAI, ErrorCode: "1211", HTTPStatus: 400, EndpointVerified: true}, ln2Config())
	if got.Class != ClassNone {
		t.Fatalf("1211 = class %d, want 0", got.Class)
	}
	switch got.Kind {
	case ActionRetryInPlace, ActionParkQuota, ActionParkProbe, ActionLaneFreeze, ActionDiedAtGate:
		t.Fatalf("1211 selected the scheduling action %q — an unknown MODEL is drift, not depletion", got.Kind)
	}
	if got.Surface != SurfaceModelDrift {
		t.Errorf("surface = %q, want %q (P-T17-3's input)", got.Surface, SurfaceModelDrift)
	}
}

// ── spec 14 · Classify stays pure and total ──────────────────────────────────

func TestClassifyStaysPureAndTotal(t *testing.T) {
	cfg := ln2Config()
	lanes := []string{laneAnthropic, laneZAI, laneLocal, "opencode", "", "made-up"}
	statuses := []int{0, 200, 400, 401, 402, 403, 429, 500, 529}
	codes := []string{"", "1000", "1113", "1211", "1302", "1308", "1310", "1399", "9999", "not-a-number"}
	resets := []time.Time{{}, time.Now().Add(time.Hour)}

	for _, lane := range lanes {
		for _, status := range statuses {
			for _, code := range codes {
				for _, reset := range resets {
					for _, verified := range []bool{false, true} {
						sig := LimitSignal{Lane: lane, HTTPStatus: status, ErrorCode: code,
							ResetAt: reset, EndpointVerified: verified}
						got := Classify(sig, cfg)
						if got.Kind == "" {
							t.Fatalf("Classify(%+v) returned no action kind — the classifier is not total", sig)
						}
						if got.Reason == "" {
							t.Fatalf("Classify(%+v) returned no reason", sig)
						}
						// Purity: the same input classifies the same way, and
						// the input is never mutated.
						again := Classify(sig, cfg)
						if again != got {
							t.Fatalf("Classify(%+v) is not deterministic: %+v vs %+v", sig, got, again)
						}
						if sig.Lane != lane || sig.ErrorCode != code || sig.HTTPStatus != status {
							t.Fatalf("Classify mutated its input: %+v", sig)
						}
					}
				}
			}
		}
	}
}

// ── the Class-4-first ordering survives, as a property ───────────────────────

func TestAuthNeverReachesARetryPark(t *testing.T) {
	cfg := ln2Config()
	codes := []string{"", "1000", "1001", "1003", "1113", "1211", "1220", "1302", "1305", "1308", "1310", "1315", "9999"}
	for _, lane := range []string{laneAnthropic, laneZAI, laneLocal} {
		for _, status := range []int{401, 402, 403} {
			for _, code := range codes {
				for _, reset := range []time.Time{{}, time.Now().Add(time.Hour)} {
					got := Classify(LimitSignal{Lane: lane, HTTPStatus: status, ErrorCode: code, ResetAt: reset}, cfg)
					if got.Class != ClassAuthPolicy || got.Kind != ActionLaneFreeze {
						t.Fatalf("lane %s HTTP %d code %q = class %d / %q, want the Class-4-first freeze (P-T08-2)",
							lane, status, code, got.Class, got.Kind)
					}
				}
			}
		}
	}
}

// TestZAINamedCodesAgainstBanAndLimitText is drain r2 / N1: the property over
// the NAMED codes, where a transient verdict and a revocation are competing
// readings of one signal.
//
// Both directions are pinned because both have a named failure. A policy
// signal reaching a retry is P-T08-2's worst case; a lane frozen because the
// provider's own usage-limit band says "violations" is the mirror image, and
// the reason the revocation test excludes that stem.
func TestZAINamedCodesAgainstBanAndLimitText(t *testing.T) {
	cfg := ln2Config()
	const (
		bandText   = "Various subscription/usage limit violations"
		limitText  = "Usage limit reached for `20000` `credits`"
		banText    = "your account has been suspended for a policy violation"
		plainBan   = "this account is banned"
		emptyText  = ""
		unrelated  = "the service may be temporarily overloaded, please try again later"
		revocation = "API access revoked for this organization"
		// Ordinary limit/overload prose that happens to contain "policy" or
		// "terminated" — stems looksLikeRevocation deliberately excludes:
		// promoting these into a freeze is the healthy-lane failure R13 names.
		policyProse = "Rate limit reached for requests. See our rate limit policy for details."
		termProse   = "The upstream connection was terminated due to overload; retry shortly."
	)
	transient := []string{"1302", "1305"}
	limits := []string{"1308", "1310", "1315", "1113"}

	// 1302/1305 + explicit revocation on VALID credentials ⇒ Class 4 freeze.
	for _, code := range transient {
		for _, body := range []string{banText, plainBan, revocation} {
			got := Classify(LimitSignal{Lane: laneZAI, ErrorCode: code, HTTPStatus: 429,
				EndpointVerified: true, BodyText: body, OnValidCredentials: true}, cfg)
			if got.Class != ClassAuthPolicy || got.Kind != ActionLaneFreeze {
				t.Errorf("zai %s + %q on valid credentials = class %d / %q, want the Class-4 freeze — a policy "+
					"signal must never reach a retry (P-T08-2)", code, body, got.Class, got.Kind)
			}
		}
	}
	// 1302/1305 + band or ordinary limit text ⇒ Class 1, whatever the
	// credential assurance: the band's own word is not a revocation.
	for _, code := range transient {
		for _, body := range []string{bandText, limitText, emptyText, unrelated, policyProse, termProse} {
			for _, valid := range []bool{false, true} {
				got := Classify(LimitSignal{Lane: laneZAI, ErrorCode: code, HTTPStatus: 429,
					EndpointVerified: true, BodyText: body, OnValidCredentials: valid}, cfg)
				if got.Class != ClassTransientShed || got.Kind != ActionRetryInPlace {
					t.Errorf("zai %s + %q (valid=%v) = class %d / %q, want class 1 / retry-in-place",
						code, body, valid, got.Class, got.Kind)
				}
			}
		}
	}
	// The limit codes never freeze on TEXT alone, including explicit ban text:
	// their class comes from the code and the signal, and the auth canary is
	// the authoritative revocation detector (S03.6, S14.6).
	for _, code := range limits {
		for _, body := range []string{bandText, limitText, banText, plainBan, revocation, emptyText} {
			for _, valid := range []bool{false, true} {
				got := Classify(LimitSignal{Lane: laneZAI, ErrorCode: code, HTTPStatus: 429,
					EndpointVerified: true, BodyText: body, OnValidCredentials: valid}, cfg)
				if got.Class == ClassAuthPolicy || got.Kind == ActionLaneFreeze {
					t.Errorf("zai %s + %q (valid=%v) froze the lane on text alone", code, body, valid)
				}
				if got.Kind != ActionParkQuota && got.Kind != ActionParkProbe {
					t.Errorf("zai %s + %q (valid=%v) = %q, want a park", code, body, valid, got.Kind)
				}
			}
		}
	}
	// The auth codes still outrank everything, and the codeless case still
	// reaches the general policy-ban heuristic unchanged.
	if got := Classify(LimitSignal{Lane: laneZAI, ErrorCode: "1000", HTTPStatus: 401,
		BodyText: bandText, OnValidCredentials: true}, cfg); got.Class != ClassAuthPolicy {
		t.Errorf("zai 1000 = class %d, want the Class-4 freeze", got.Class)
	}
	if got := Classify(LimitSignal{Lane: laneZAI, HTTPStatus: 429,
		BodyText: banText, OnValidCredentials: true}, cfg); got.Class != ClassAuthPolicy {
		t.Errorf("codeless zai + ban text = class %d, want the Class-4 freeze", got.Class)
	}
}

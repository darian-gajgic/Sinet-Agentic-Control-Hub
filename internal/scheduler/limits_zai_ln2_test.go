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

	for code := 1300; code <= 1399; code++ {
		s := fmt.Sprint(code)
		if named[s] {
			continue
		}
		for _, withReset := range []bool{false, true} {
			sig := LimitSignal{Lane: laneZAI, ErrorCode: s, HTTPStatus: 429, EndpointVerified: true}
			if withReset {
				sig.ResetAt = reset
			}
			got := Classify(sig, cfg)
			if got.Kind == ActionRetryInPlace {
				t.Fatalf("zai %s (reset=%v) retries in place — a retry storm against a depleted plan", s, withReset)
			}
			if got.Kind == ActionLaneFreeze {
				t.Fatalf("zai %s (reset=%v) freezes a healthy lane", s, withReset)
			}
			if got.Kind != ActionParkQuota && got.Kind != ActionParkProbe {
				t.Fatalf("zai %s (reset=%v) = %q, want a park", s, withReset, got.Kind)
			}
			// The sanctioned signal-based fallback: the CLASS follows the
			// signal, not the code (S10.5's own class definitions).
			wantClass := ClassDepletionNoSignal
			if withReset {
				wantClass = ClassDepletionSignal
			}
			if got.Class != wantClass {
				t.Fatalf("zai %s (reset=%v) = class %d, want %d", s, withReset, got.Class, wantClass)
			}
		}
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

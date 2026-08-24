package scheduler

// limits_kimi_ln3_test.go — P3-LN-3 §6 specs 6-12 (S10.5, P-T08-2, P-T17-1/3).
//
// DOCUMENTED-NOT-OBSERVED. Every message string below is quoted from the
// vendor's published error reference as captured on 2026-08-24 in
// P3/measurements/2026-08-24-kimi-lane-gate-audit.md — none of them is a body
// this platform has seen on the wire. The audit could not verify the JSON shape
// of a Kimi error body, whether it carries a code field, or whether any reset
// header exists (U3/U4/U5), so the ceremony's live single-request probe (one
// real 401, one real 429, one real 403 if reachable) precedes treating this
// classifier as final. The P-T17-1 auth canary remains the authoritative
// revocation detector for whatever the message grammar misses.
//
// This lane's shape is the reason the packet exists: quota exhaustion arrives
// on 403 AND 429, entitlement failures arrive on 401, and HTTP status alone is
// therefore NOT a classifier here. Wired naively the lane would freeze itself
// and page the operator with a suspected policy revocation every time its
// weekly window emptied.

import (
	"fmt"
	"testing"
	"time"
)

// The vendor's own strings, verbatim (audit B4). They reach Classify already
// resolved to a documented class by the lane document — Classify performs no
// lookup and stays pure and total (S10.5).
const (
	kimiWeekly403   = "You've reached your usage limit for this billing cycle"
	kimiTerminated  = "Access terminated"
	kimiTierK3      = "Your current subscription does not have access to k3"
	kimiTierSpeed   = "Your current subscription does not have access to kimi-for-coding-highspeed"
	kimiCtx256K     = "Your current plan supports only kimi-k3 up to 256K context"
	kimiBadKey      = "The API Key appears to be invalid or may have expired"
	kimiBadAuth     = "Invalid Authentication"
	kimiOverloaded  = "The engine is currently overloaded, please try again later"
	kimiTooMany     = "We're receiving too many requests at the moment"
	kimiPeriod429   = "You've reached your usage limit for this period"
	kimiMonthly429  = "You've reached kimi monthly usage limit for this billing cycle"
	kimiNoMethod    = "method not found"
	kimiNoSuchModel = "Not found the model kimi-for-coding or Permission denied"
)

// kimiSignal builds the signal the adapter forwards for one documented row.
func kimiSignal(status int, class, body string) LimitSignal {
	return LimitSignal{Lane: laneKimi, HTTPStatus: status, DocumentedClass: class, BodyText: body, EndpointVerified: true}
}

// ── spec 6 · THE HEADLINE — a 403 weekly depletion parks and never freezes ───

func TestKimi403WeeklyDepletionParksAndNeverFreezes(t *testing.T) {
	cfg := ln2Config()
	// Both credential assurances, because the canary's answer must not change
	// an ordinary depletion into a revocation.
	for _, valid := range []bool{false, true} {
		sig := kimiSignal(403, DocumentedDepletion, kimiWeekly403)
		sig.OnValidCredentials = valid
		got := Classify(sig, cfg)

		if got.Class == ClassAuthPolicy || got.Kind == ActionLaneFreeze {
			t.Fatalf("valid=%v: the weekly window emptying FROZE the lane (%+v) — this fires every week, is routine, "+
				"and is indistinguishable at the alert from a real revocation", valid, got)
		}
		if got.Kind != ActionParkProbe && got.Kind != ActionParkQuota {
			t.Errorf("valid=%v: action = %q, want a park — the plan is spent and the lane is healthy", valid, got.Kind)
		}
		if got.Class != ClassDepletionNoSignal && got.Class != ClassDepletionSignal {
			t.Errorf("valid=%v: class = %d, want a depletion class", valid, got.Class)
		}
		if got.Kind == ActionRetryInPlace {
			t.Errorf("valid=%v: a depleted plan reached a retry — a retry storm against a spent plan", valid)
		}
	}
	// With no documented reset time (the audit found none, U4) the park is the
	// bounded PROBE schedule rather than a wait on a time nobody published.
	got := Classify(kimiSignal(403, DocumentedDepletion, kimiWeekly403), cfg)
	if got.Class != ClassDepletionNoSignal || got.ProbeIntervalMax != cfg.ProbeIntervalMax {
		t.Errorf("no-signal depletion = %+v, want the Class-3 probe park at ⚙ limit.probe_interval_max", got)
	}
	// And the OTHER direction: strip the documented class and the same status
	// still freezes. The guard is an EXEMPTION LIST, never a re-ordering.
	bare := Classify(LimitSignal{Lane: laneKimi, HTTPStatus: 403, BodyText: kimiWeekly403}, cfg)
	if bare.Class != ClassAuthPolicy || bare.Kind != ActionLaneFreeze {
		t.Fatalf("an UNMATCHED bare 403 = %+v, want the Class-4 freeze — the safe default must survive the guard", bare)
	}
}

// ── spec 7 · the genuine revocation still freezes ────────────────────────────

func TestKimi403AccessTerminatedFreezesLane(t *testing.T) {
	cfg := ln2Config()
	// "Access terminated" carries NO documented class in the shipped document,
	// deliberately: a revocation row must never be suppressible.
	for _, valid := range []bool{false, true} {
		sig := LimitSignal{Lane: laneKimi, HTTPStatus: 403, BodyText: kimiTerminated, EndpointVerified: true}
		sig.OnValidCredentials = valid
		got := Classify(sig, cfg)
		if got.Class != ClassAuthPolicy || got.Kind != ActionLaneFreeze {
			t.Fatalf("valid=%v: account suspension = %+v, want the Class-4 freeze", valid, got)
		}
		if got.Kind == ActionParkProbe || got.Kind == ActionParkQuota || got.Kind == ActionRetryInPlace {
			t.Errorf("valid=%v: a revoked lane reached a park/retry — P-T08-2's named worst case", valid)
		}
	}
	// The genuine 401 auth strings freeze too, in both directions.
	for _, body := range []string{kimiBadKey, kimiBadAuth} {
		got := Classify(LimitSignal{Lane: laneKimi, HTTPStatus: 401, BodyText: body}, cfg)
		if got.Class != ClassAuthPolicy || got.Kind != ActionLaneFreeze {
			t.Errorf("%q = %+v, want the Class-4 freeze", body, got)
		}
	}
}

// ── spec 8 · a 401 entitlement failure is model drift, not revocation ────────

func TestKimi401EntitlementIsModelDriftNotRevocation(t *testing.T) {
	cfg := ln2Config()
	// A PROPERTY over the entitlement message set, not two examples: the tier
	// gate is real and enforced on the wire, and the thing that acts on it is
	// the model-list-diff canary (P-T17-3).
	entitlement := []string{kimiTierK3, kimiTierSpeed, kimiCtx256K}
	for _, body := range entitlement {
		for _, valid := range []bool{false, true} {
			sig := kimiSignal(401, DocumentedModelDrift, body)
			sig.OnValidCredentials = valid
			got := Classify(sig, cfg)
			if got.Class == ClassAuthPolicy || got.Kind == ActionLaneFreeze {
				t.Errorf("%q (valid=%v) froze the lane — a membership tier that does not serve a model is drift, "+
					"and freezing the lane hides the one fact worth acting on", body, valid)
			}
			if got.Surface != SurfaceModelDrift {
				t.Errorf("%q (valid=%v) surfaced %q, want %q", body, valid, got.Surface, SurfaceModelDrift)
			}
			if got.Kind == ActionParkProbe || got.Kind == ActionParkQuota {
				t.Errorf("%q (valid=%v) parked — parking a run because a plan does not carry a model waits forever "+
					"for something nobody is going to change", body, valid)
			}
		}
	}
	// The control: the genuine-auth 401s are in the SAME status and freeze.
	for _, body := range []string{kimiBadKey, kimiBadAuth} {
		got := Classify(LimitSignal{Lane: laneKimi, HTTPStatus: 401, BodyText: body}, cfg)
		if got.Class != ClassAuthPolicy {
			t.Errorf("%q = class %d, want the Class-4 freeze — otherwise the exemption is about the status, not about the row", body, got.Class)
		}
	}
}

// ── spec 9 · 429 splits into transient and depletion ─────────────────────────

func TestKimi429TransientRetriesAndQuotaParks(t *testing.T) {
	cfg := ln2Config()
	for _, body := range []string{kimiOverloaded, kimiTooMany} {
		got := Classify(kimiSignal(429, DocumentedTransient, body), cfg)
		if got.Class != ClassTransientShed || got.Kind != ActionRetryInPlace {
			t.Errorf("%q = %+v, want Class 1 / retry-in-place", body, got)
		}
		if got.RetryCap != cfg.RetryCap || got.RetryBudgetRatio != cfg.RetryBudgetRatio {
			t.Errorf("%q carries retry policy %d/%v, want the ⚙ values", body, got.RetryCap, got.RetryBudgetRatio)
		}
	}
	for _, body := range []string{kimiPeriod429, kimiMonthly429} {
		got := Classify(kimiSignal(429, DocumentedDepletion, body), cfg)
		if got.Kind != ActionParkProbe && got.Kind != ActionParkQuota {
			t.Errorf("%q = %+v, want a park — the window is spent", body, got)
		}
		if got.Kind == ActionRetryInPlace {
			t.Errorf("%q retried against a spent window", body)
		}
		if got.Class == ClassAuthPolicy {
			t.Errorf("%q froze the lane", body)
		}
	}
	// A transient row on an AUTH-SHAPED status is NOT exempted: only depletion
	// and model-drift rows may lift the Class-4 status rule (§8 constraint i).
	if got := Classify(kimiSignal(403, DocumentedTransient, kimiOverloaded), cfg); got.Class != ClassAuthPolicy {
		t.Errorf("a transient row on a 403 = class %d, want the Class-4 freeze — the exemption list is depletion and model-drift ONLY", got.Class)
	}
	// A transient code whose body carries explicit revocation text on VALID
	// credentials freezes: Class 4 outranks Class 1, whatever the row says.
	rev := kimiSignal(429, DocumentedTransient, "your account has been suspended for a policy violation")
	rev.OnValidCredentials = true
	if got := Classify(rev, cfg); got.Class != ClassAuthPolicy || got.Kind != ActionLaneFreeze {
		t.Errorf("a transient row with revocation text on valid credentials = %+v, want the Class-4 freeze", got)
	}
}

// ── spec 10 · the wrong endpoint is a surfaced defect, never a park ──────────

func TestKimi404MethodNotFoundSurfacesEndpointDefect(t *testing.T) {
	cfg := ln2Config()
	got := Classify(kimiSignal(404, DocumentedEndpointDefect, kimiNoMethod), cfg)
	if got.Surface != SurfaceEndpointDefect {
		t.Errorf("surface = %q, want %q — the wrong-endpoint case by name", got.Surface, SurfaceEndpointDefect)
	}
	if got.Kind == ActionParkProbe || got.Kind == ActionParkQuota {
		t.Error("a wrong-endpoint defect was parked — probe-parking that waits forever for a configuration nobody is going to fix is the P-T08-2 failure class")
	}
	if got.Class != ClassNone {
		t.Errorf("class = %d, want ClassNone — a configuration defect is not a limit event", got.Class)
	}
	if got.Reason == "" {
		t.Error("the endpoint-defect verdict carries no reason")
	}

	// The sibling 404 is model drift, and it is also never parked.
	drift := Classify(kimiSignal(404, DocumentedModelDrift, kimiNoSuchModel), cfg)
	if drift.Surface != SurfaceModelDrift {
		t.Errorf("surface = %q, want %q", drift.Surface, SurfaceModelDrift)
	}
	if drift.Kind == ActionParkProbe || drift.Kind == ActionParkQuota {
		t.Error("a model-drift 404 was parked")
	}
}

// ── spec 11 · an unrecognised message never freezes and never retries ────────

func TestKimiUnknownSignalNeverFreezesAndNeverRetries(t *testing.T) {
	cfg := ln2Config()
	unknown := []string{
		"", "something nobody documented",
		"total message size 9999 exceeds limit 2097152",
		"The request was rejected because it was considered high risk",
		"a message added after this seed's verified-on date",
	}
	// A property over the NON-auth statuses. An unmatched message carries no
	// documented class at all, because the guard is an exemption list.
	for _, status := range []int{0, 400, 404, 429, 500, 529} {
		for _, body := range unknown {
			for _, valid := range []bool{false, true} {
				sig := LimitSignal{Lane: laneKimi, HTTPStatus: status, BodyText: body,
					OnValidCredentials: valid, EndpointVerified: true}
				got := Classify(sig, cfg)
				ctx := fmt.Sprintf("status=%d body=%q valid=%v", status, body, valid)
				if got.Class == ClassAuthPolicy || got.Kind == ActionLaneFreeze {
					if !(valid && body != "" && looksLikePolicyBan(body)) {
						t.Errorf("%s froze the lane on an unrecognised message — freezing a healthy lane is one of the two directions with no safe failure", ctx)
					}
				}
				if got.Class == ClassTransientShed || got.Kind == ActionRetryInPlace {
					t.Errorf("%s retried an unrecognised signal — a retry storm against a plan that may be spent", ctx)
				}
			}
		}
	}
	// A bare 429 with nothing documented parks on the bounded probe schedule,
	// which is the safe direction rather than "not a limit event".
	if got := Classify(LimitSignal{Lane: laneKimi, HTTPStatus: 429}, cfg); got.Kind != ActionParkProbe {
		t.Errorf("a bare kimi 429 = %+v, want the Class-3 probe park", got)
	}
	// §8 constraint (i), pinned in the OTHER direction: an unmatched bare
	// 401/402/403 STILL freezes. The safe default is preserved.
	for _, status := range []int{401, 402, 403} {
		for _, body := range unknown {
			got := Classify(LimitSignal{Lane: laneKimi, HTTPStatus: status, BodyText: body}, cfg)
			if got.Class != ClassAuthPolicy || got.Kind != ActionLaneFreeze {
				t.Errorf("an unmatched kimi %d (%q) = %+v, want the Class-4 freeze — the guard exempts documented rows, it never re-orders the ladder", status, body, got)
			}
		}
	}
}

// ── spec 12 (half) · the documented vocabulary is CLOSED ─────────────────────

func TestUnknownDocumentedClassIsInert(t *testing.T) {
	cfg := ln2Config()
	// A class the classifier does not name must change nothing: the signal
	// classifies exactly as it would with no class at all. Anything else would
	// let a document edit invent a verdict.
	for _, class := range []string{"auth", "balance", "revocation", "made-up", "DEPLETION"} {
		for _, status := range []int{401, 403, 429} {
			with := Classify(LimitSignal{Lane: laneKimi, HTTPStatus: status, DocumentedClass: class}, cfg)
			without := Classify(LimitSignal{Lane: laneKimi, HTTPStatus: status}, cfg)
			if with != without {
				t.Errorf("documented_class %q on HTTP %d changed the verdict (%+v vs %+v) — the vocabulary is closed, and a value outside it is inert",
					class, status, with, without)
			}
		}
	}
	// And the zai lane is untouched by the whole mechanism: its document
	// declares no documented_class, so every zai verdict is what it was.
	for _, code := range []string{"1000", "1113", "1211", "1302", "1308", "1310", "9999"} {
		for _, status := range []int{0, 401, 403, 429} {
			for _, reset := range []time.Time{{}, time.Now().Add(time.Hour)} {
				sig := LimitSignal{Lane: laneZAI, ErrorCode: code, HTTPStatus: status, ResetAt: reset, EndpointVerified: true}
				if Classify(sig, cfg) != Classify(sig, cfg) {
					t.Fatalf("zai %s/%d is not deterministic", code, status)
				}
			}
		}
	}
}

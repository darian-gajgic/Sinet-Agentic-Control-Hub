package shell

// kimi_ln3_test.go — P3-LN-3 §6 spec 18 + the end-to-end classifier proof
// (S03.2, S03.6, S08.8, S10.5).
//
// internal/shell is the only place where the lane DOCUMENT and the scheduler's
// TAXONOMY are both in scope, so it is where the two are proven to agree. A
// test of the classifier alone cannot see a document that never produces the
// class it classifies on — the §63 drain-r2 R1 lesson, applied to a vocabulary
// instead of an argument.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// laneByName selects a seed document by LANE, never by position: the seed set
// is sorted by lane name, so an index says whatever the alphabet says.
func laneByName(t *testing.T, lanes []opencode.LaneConfig, name string) opencode.LaneConfig {
	t.Helper()
	for _, l := range lanes {
		if l.Lane == name {
			return l
		}
	}
	t.Fatalf("no seed lane document for %q (have %d)", name, len(lanes))
	return opencode.LaneConfig{}
}

func commissionedLane(t *testing.T, lanes []opencode.LaneConfig, name string) map[string]opencode.ProviderConfig {
	t.Helper()
	return map[string]opencode.ProviderConfig{"alice": laneByName(t, lanes, name).Providers()}
}

// ── spec 18 · the kimi derivations at the composition root ───────────────────

func TestKimiLaneDerivationsAtTheCompositionRoot(t *testing.T) {
	lanes := seedLanes(t)
	kimi := laneByName(t, lanes, adapters.LaneKimi)

	// NOTHING COMMISSIONED — the pre-ceremony world is byte-identical.
	if got := commissionedLanes(lanes, nil); len(got) != 0 {
		t.Errorf("commissionedLanes = %v with nothing placed, want none", got)
	}
	if got := laneSubstrates(lanes, nil); got != nil {
		t.Errorf("laneSubstrates = %v with nothing placed, want nil — a lane nobody holds cannot be dispatched to", got)
	}
	if seats := laneAlternateSeats(lanes, nil); len(seats[worker.DutyExecution]) != 0 {
		t.Errorf("execution alternates = %v with nothing placed, want none", seats[worker.DutyExecution])
	}

	// The CONFIGURED model list is supplied regardless of commissioning: it is
	// the config side of the P-T17-3 diff, not a claim that the lane is live.
	models := laneConfiguredModels(lanes)
	if len(models[adapters.LaneKimi]) != len(kimi.Models) {
		t.Errorf("configured kimi models = %v, want all %d of the document's", models[adapters.LaneKimi], len(kimi.Models))
	}
	if len(models[adapters.LaneZAI]) == 0 {
		t.Error("the zai lane lost its configured model list when a second document landed")
	}

	// COMMISSIONED — coverage, substrate and seat.
	live := commissionedLane(t, lanes, adapters.LaneKimi)
	if got := commissionedLanes(lanes, live); len(got) != 1 || got[0] != adapters.LaneKimi {
		t.Errorf("commissionedLanes = %v, want [kimi]", got)
	}
	subs := laneSubstrates(lanes, live)
	if subs[adapters.LaneKimi] != adapters.SubstrateOpencode {
		t.Errorf("laneSubstrates = %v, want kimi→opencode (the document's own substrate — the @ai-sdk/anthropic protocol is a DATA difference, not a second substrate)", subs)
	}
	if _, ok := subs[adapters.LaneZAI]; ok {
		t.Error("the zai lane appears in the map while only kimi is commissioned")
	}
	seats := laneAlternateSeats(lanes, live)
	exec := seats[worker.DutyExecution]
	if len(exec) != 1 {
		t.Fatalf("execution alternates = %v, want exactly one kimi seat", exec)
	}
	if exec[0].Lane != adapters.LaneKimi || exec[0].Model != kimi.DefaultModel {
		t.Errorf("seat = %+v, want the document's own lane and default model %q", exec[0], kimi.DefaultModel)
	}
	if exec[0].WindowTokens <= 0 {
		t.Error("the composed seat carries no context window")
	}
	// Planning and judge gain NO second-lane seat: no kimi model has been
	// measured against the B3 mix or the S07.5 capability bar, and seating one
	// would be inventing a ratification.
	for _, duty := range []string{worker.DutyPlanning, worker.DutyJudge} {
		if len(seats[duty]) != 0 {
			t.Errorf("duty %q gained a second-lane seat — no kimi model has been measured against the B3/S07.5 bars", duty)
		}
	}

	// BOTH commissioned — two flat lanes, each on its own substrate row.
	both := map[string]opencode.ProviderConfig{
		"alice": laneByName(t, lanes, adapters.LaneKimi).Providers(),
		"bob":   laneByName(t, lanes, adapters.LaneZAI).Providers(),
	}
	if got := commissionedLanes(lanes, both); len(got) != 2 || got[0] != adapters.LaneKimi || got[1] != adapters.LaneZAI {
		t.Errorf("commissionedLanes = %v, want [kimi zai] sorted", got)
	}
	if seats := laneAlternateSeats(lanes, both); len(seats[worker.DutyExecution]) != 2 {
		t.Errorf("execution alternates = %v, want one seat per commissioned lane", seats[worker.DutyExecution])
	}
}

// ── the document's vocabulary and the classifier's are the SAME vocabulary ───

func TestKimiDocumentedClassesReachTheClassifier(t *testing.T) {
	// The two packages cannot import each other, so nothing but this test
	// stops the document from emitting a word the classifier never reads.
	for _, pair := range []struct{ doc, sched string }{
		{opencode.DocumentedTransient, scheduler.DocumentedTransient},
		{opencode.DocumentedDepletion, scheduler.DocumentedDepletion},
		{opencode.DocumentedModelDrift, scheduler.DocumentedModelDrift},
		{opencode.DocumentedEndpointDefect, scheduler.DocumentedEndpointDefect},
	} {
		if pair.doc != pair.sched {
			t.Errorf("the lane document says %q where the classifier reads %q — a documented row that names a class nobody classifies on is inert, and would silently re-open the false-freeze", pair.doc, pair.sched)
		}
	}
}

// TestKimiSignalRoundTripsFromTheDocumentIntoTheClassifier drives the REAL
// shipped document: the vendor's own published message, through ExtractSignal,
// through the wire payload, into Classify. This is the assertion that actually
// says the weekly window emptying does not page the operator.
//
// DOCUMENTED-NOT-OBSERVED: the bodies below are the vendor's published strings
// wrapped in a plausible envelope, not bodies this platform has seen. The
// ceremony's live probe closes that (audit U3).
func TestKimiSignalRoundTripsFromTheDocumentIntoTheClassifier(t *testing.T) {
	kimi := laneByName(t, seedLanes(t), adapters.LaneKimi)
	cfg := scheduler.LimitConfig{RetryCap: 3, RetryBudgetRatio: 0.1, ProbeIntervalMax: 30 * time.Minute}

	classify := func(t *testing.T, body string, status int, valid bool) scheduler.Action {
		t.Helper()
		sig, ok := kimi.ExtractSignal(body, status)
		if !ok {
			t.Fatalf("the lane produced no signal for %q on HTTP %d", body, status)
		}
		sig.EndpointVerified = true
		raw, err := json.Marshal(sig)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		decoded, err := scheduler.SignalFromPayload(raw)
		if err != nil {
			t.Fatalf("SignalFromPayload: %v (%s)", err, raw)
		}
		decoded.EndpointVerified = true
		decoded.OnValidCredentials = valid
		return scheduler.Classify(decoded, cfg)
	}
	envelope := func(msg string) string { return `{"error":{"message":` + jsonString(msg) + `}}` }

	for _, tc := range []struct {
		name       string
		msg        string
		status     int
		wantFreeze bool
		wantSurf   scheduler.SurfaceKind
		wantPark   bool
		wantRetry  bool
	}{
		{name: "weekly depletion on a 403", msg: "You've reached your usage limit for this billing cycle", status: 403, wantPark: true},
		{name: "account suspension on a 403", msg: "Access terminated", status: 403, wantFreeze: true},
		{name: "tier gate on a 401", msg: "Your current subscription does not have access to k3", status: 401, wantSurf: scheduler.SurfaceModelDrift},
		{name: "context entitlement on a 401", msg: "Your current plan supports only kimi-k3 up to 256K context", status: 401, wantSurf: scheduler.SurfaceModelDrift},
		{name: "invalid key on a 401", msg: "The API Key appears to be invalid or may have expired", status: 401, wantFreeze: true},
		{name: "overload on a 429", msg: "The engine is currently overloaded, please try again later", status: 429, wantRetry: true},
		{name: "period limit on a 429", msg: "You've reached your usage limit for this period", status: 429, wantPark: true},
		{name: "wrong endpoint on a 404", msg: "method not found", status: 404, wantSurf: scheduler.SurfaceEndpointDefect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, valid := range []bool{false, true} {
				got := classify(t, envelope(tc.msg), tc.status, valid)
				froze := got.Class == scheduler.ClassAuthPolicy || got.Kind == scheduler.ActionLaneFreeze
				if froze != tc.wantFreeze {
					t.Fatalf("valid=%v: froze=%v want %v (%+v)", valid, froze, tc.wantFreeze, got)
				}
				if tc.wantSurf != "" && got.Surface != tc.wantSurf {
					t.Errorf("valid=%v: surface = %q, want %q", valid, got.Surface, tc.wantSurf)
				}
				parked := got.Kind == scheduler.ActionParkProbe || got.Kind == scheduler.ActionParkQuota
				if tc.wantPark && !parked {
					t.Errorf("valid=%v: action = %q, want a park", valid, got.Kind)
				}
				if !tc.wantPark && !tc.wantFreeze && parked {
					t.Errorf("valid=%v: action = %q parked where it must not", valid, got.Kind)
				}
				if (got.Kind == scheduler.ActionRetryInPlace) != tc.wantRetry {
					t.Errorf("valid=%v: retry=%v want %v", valid, got.Kind == scheduler.ActionRetryInPlace, tc.wantRetry)
				}
			}
		})
	}

	// The zai document, through the same path, is unchanged.
	zai := laneByName(t, seedLanes(t), adapters.LaneZAI)
	zaiBody := `{"error":{"code":"1308","message":` +
		jsonString("Usage limit reached for 20 credits. Your limit will reset at 2026-08-25 09:00:00") + `}}`
	sig, ok := zai.ExtractSignal(zaiBody, 429)
	if !ok {
		t.Fatal("the zai document stopped extracting its own coded signal")
	}
	if sig.DocumentedClass != "" {
		t.Errorf("a zai signal carries documented_class %q — the zai document declares none", sig.DocumentedClass)
	}
	sig.EndpointVerified = true
	raw, _ := json.Marshal(sig)
	decoded, err := scheduler.SignalFromPayload(raw)
	if err != nil {
		t.Fatalf("SignalFromPayload(zai): %v", err)
	}
	decoded.EndpointVerified = true
	if act := scheduler.Classify(decoded, cfg); act.Class != scheduler.ClassDepletionSignal {
		t.Errorf("the zai 1308 round trip classified %d (%s), want class 2 unchanged", act.Class, act.Reason)
	}
}

// jsonString quotes a message for the JSON envelope the engine wraps.
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// ── the C5 rider has no enforcement seam, and the tree says so ───────────────

func TestKimiDataPolicyRiderIsRecordedNotEnforced(t *testing.T) {
	kimi := laneByName(t, seedLanes(t), adapters.LaneKimi)
	if kimi.DataPolicy.Statement == "" {
		t.Fatal("the kimi lane carries no data-policy statement")
	}
	if kimi.DataPolicy.Enforced {
		t.Error("the lane claims its routing rider is enforced; no per-lane data-policy enforcement point exists in the tree")
	}
	// The honest sentence the operator reads at the gate. "Applied" would let a
	// reader assume the code stops it, which it does not.
	if strings.Contains(strings.ToLower(kimi.DataPolicy.EnforcementNote), "applied") {
		t.Errorf("the enforcement note claims the rider is applied: %q", kimi.DataPolicy.EnforcementNote)
	}
	for _, needle := range []string{"NOT MACHINE-ENFORCED", "routing-policy seam"} {
		if !strings.Contains(kimi.DataPolicy.EnforcementNote, needle) {
			t.Errorf("the enforcement note does not carry %q: %q", needle, kimi.DataPolicy.EnforcementNote)
		}
	}
}

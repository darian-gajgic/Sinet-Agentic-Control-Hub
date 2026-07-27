package watchlist_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/conformance"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchlist"
)

// canary_test.go — the S14.6 ¶3 API canary battery. Hermetic: every outbound
// call goes to an httptest stub bound to loopback, no binary is installed, no
// paid lane is dialled, and the suite spends $0 by construction.

// canaries wires a canary layer onto the harness's clock and emitter.
func (h *harness) canaries(t *testing.T, clk *clock) *watchlist.Canaries {
	t.Helper()
	c := watchlist.NewCanaries(h.db, h.log, h.reg)
	c.Emitter, c.Now = h.em, clk.now
	h.em.Now = clk.now
	return c
}

// canaryEvents reads every canary.result payload in seq order.
func (h *harness) canaryEvents(t *testing.T) []watchlist.CanaryPayload {
	t.Helper()
	rows, err := h.db.QueryContext(context.Background(),
		`SELECT payload FROM run_events WHERE type = ? ORDER BY event_seq`, watchlist.EventCanaryResult)
	if err != nil {
		t.Fatalf("read canary results: %v", err)
	}
	defer rows.Close()
	var out []watchlist.CanaryPayload
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&body); err != nil {
			t.Fatal(err)
		}
		var p watchlist.CanaryPayload
		if err := json.Unmarshal(body, &p); err != nil {
			t.Fatalf("canary payload is not valid JSON: %v", err)
		}
		out = append(out, p)
	}
	return out
}

// ── R23: the auth canary is scheduler.Classify's first production caller ────

// TestAuthCanaryClassFourFreezesAndFlags is the load-bearing positive: an
// auth-shaped answer on VALID credentials classifies Class 4 → lane freeze, and
// raises a flag-now card. The lane-freeze verdict is RECORDED, never acted on.
func TestAuthCanaryClassFourFreezesAndFlags(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}
	c := h.canaries(t, clk)
	c.Auth = &watchlist.AuthCanary{
		Lanes: []string{watchlist.LaneAnthropic},
		Probe: func(ctx context.Context, lane string) (scheduler.LimitSignal, error) {
			return scheduler.LimitSignal{HTTPStatus: 403, OnValidCredentials: true,
				BodyText: "your organization's access has been suspended for policy reasons"}, nil
		},
	}

	sweep, err := c.RunDue(context.Background())
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if sweep.Ran != 1 || sweep.Failures != 1 {
		t.Fatalf("sweep = %+v, want one run and one failure", sweep)
	}

	got := h.canaryEvents(t)
	if len(got) != 1 {
		t.Fatalf("canary.result rows = %d, want 1", len(got))
	}
	p := got[0]
	if p.CanaryKind != watchlist.CanaryAuth || p.Lane != watchlist.LaneAnthropic {
		t.Errorf("payload identity = %q/%q, want auth/anthropic", p.CanaryKind, p.Lane)
	}
	if p.Result != watchlist.CanaryFail {
		t.Errorf("result = %q, want fail", p.Result)
	}
	if p.LimitClass != int(scheduler.ClassAuthPolicy) {
		t.Errorf("limit_class = %d, want %d (ClassAuthPolicy)", p.LimitClass, scheduler.ClassAuthPolicy)
	}
	if p.Action != string(scheduler.ActionLaneFreeze) {
		t.Errorf("action = %q, want %q", p.Action, scheduler.ActionLaneFreeze)
	}
	if p.PurposeTag != string(watchlist.CanaryPurposeTag) || p.WorkloadClass != string(watchlist.CanaryWorkloadClass) {
		t.Errorf("metering posture = %q/%q, want probe/probe", p.PurposeTag, p.WorkloadClass)
	}

	cards := h.findings(t)
	if len(cards) != 1 {
		t.Fatalf("drift.finding rows = %d, want 1 (a canary failure raises a card)", len(cards))
	}
	if cards[0].Severity != watchlist.SeverityFlagNow {
		t.Errorf("severity = %q, want %q — an auth-canary failure on an active lane is flag-now (S14.4)",
			cards[0].Severity, watchlist.SeverityFlagNow)
	}
	if cards[0].ChangeClass != watchlist.ClassTerms {
		t.Errorf("change class = %q, want %q", cards[0].ChangeClass, watchlist.ClassTerms)
	}
}

// TestAuthCanaryNeverRetryParks is the explicit NEGATIVE the spec names
// (P-T08-2's worst case): no auth-shaped signal may ever produce a park action.
func TestAuthCanaryNeverRetryParks(t *testing.T) {
	authShaped := []scheduler.LimitSignal{
		{HTTPStatus: 401, OnValidCredentials: true},
		{HTTPStatus: 402, OnValidCredentials: true},
		{HTTPStatus: 403, OnValidCredentials: true},
		{HTTPStatus: 200, OnValidCredentials: true, BodyText: "account banned for policy violation"},
	}
	parking := map[string]bool{
		string(scheduler.ActionParkQuota): true,
		string(scheduler.ActionParkProbe): true,
	}
	for i, sig := range authShaped {
		t.Run(fmt.Sprintf("signal%d", i), func(t *testing.T) {
			h := newHarness(t)
			clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}
			c := h.canaries(t, clk)
			c.Auth = &watchlist.AuthCanary{
				Lanes: []string{watchlist.LaneAnthropic},
				Probe: func(ctx context.Context, lane string) (scheduler.LimitSignal, error) { return sig, nil },
			}
			if _, err := c.RunDue(context.Background()); err != nil {
				t.Fatalf("RunDue: %v", err)
			}
			got := h.canaryEvents(t)
			if len(got) != 1 {
				t.Fatalf("canary.result rows = %d, want 1", len(got))
			}
			if parking[got[0].Action] {
				t.Fatalf("an auth-shaped signal produced a PARK action (%q) — a policy event that dies in a retry loop is a platform defect (P-T08-2)", got[0].Action)
			}
			if got[0].Action != string(scheduler.ActionLaneFreeze) {
				t.Errorf("action = %q, want lane-freeze", got[0].Action)
			}
		})
	}
}

// TestAuthCanaryPassesOnAHealthyLane: a non-auth answer is a pass and raises no
// card. A depletion during a canary means the lane is alive and credentialed.
func TestAuthCanaryPassesOnAHealthyLane(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}
	c := h.canaries(t, clk)
	c.Auth = &watchlist.AuthCanary{
		Lanes: []string{watchlist.LaneZAI},
		Probe: func(ctx context.Context, lane string) (scheduler.LimitSignal, error) {
			return scheduler.LimitSignal{HTTPStatus: 200}, nil
		},
	}
	if _, err := c.RunDue(context.Background()); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if got := h.canaryEvents(t); len(got) != 1 || got[0].Result != watchlist.CanaryPass {
		t.Fatalf("want one passing canary.result, got %+v", got)
	}
	if cards := h.findings(t); len(cards) != 0 {
		t.Errorf("a passing canary raised %d cards, want 0", len(cards))
	}
}

// TestHTTPAuthProbeNormalizesTheWire proves the real-request leg against a stub:
// a 403 becomes a Class-4 signal carrying the ban text and OnValidCredentials.
func TestHTTPAuthProbeNormalizesTheWire(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-test-key")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "access suspended: policy violation")
	}))
	defer srv.Close()

	probe := watchlist.HTTPAuthProbe(srv.Client(), srv.URL, "x-test-key", func() string { return "secret-value" })
	sig, err := probe(context.Background(), watchlist.LaneZAI)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if gotKey != "secret-value" {
		t.Errorf("credential header = %q, want the accessor's value", gotKey)
	}
	if sig.HTTPStatus != http.StatusForbidden || !sig.OnValidCredentials {
		t.Fatalf("signal = %+v, want a 403 on valid credentials", sig)
	}
	act := scheduler.Classify(sig, scheduler.LimitConfig{})
	if act.Class != scheduler.ClassAuthPolicy || act.Kind != scheduler.ActionLaneFreeze {
		t.Errorf("Classify = %v/%v, want ClassAuthPolicy/lane-freeze", act.Class, act.Kind)
	}
}

// TestDisarmedLegsRunNothing: with no probe wired the layer records nothing —
// the $0 posture is a real absence, not a faked pass.
func TestDisarmedLegsRunNothing(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}
	c := h.canaries(t, clk)
	c.Auth = watchlist.NewAuthCanary(nil)
	c.ModelList = watchlist.NewModelListCanary(nil, nil)
	c.Behavioral = watchlist.NewBehavioralCanary(nil)

	sweep, err := c.RunDue(context.Background())
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if sweep.Ran != 0 || sweep.Disarmed != 6 {
		t.Fatalf("sweep = %+v, want 0 ran and 6 disarmed (3 legs × 2 paid lanes)", sweep)
	}
	if got := h.canaryEvents(t); len(got) != 0 {
		t.Errorf("a disarmed layer recorded %d canary results, want 0", len(got))
	}
}

// ── R25: the behavioral canary ─────────────────────────────────────────────

func TestBehavioralCanaryBaselineThenDrop(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}
	c := h.canaries(t, clk)

	rate := 1.0
	c.Behavioral = &watchlist.BehavioralCanary{
		Lanes: []string{watchlist.LaneAnthropic},
		Run: func(ctx context.Context, lane string) (watchlist.BehavioralOutcome, error) {
			return watchlist.BehavioralOutcome{
				Runner: "promptfoo", RunnerVersion: "0.121.19", Suite: "bump-probe-v1",
				Cases: 5, PassRate: rate,
			}, nil
		},
	}

	// First run: baseline, no card.
	if _, err := c.RunDue(context.Background()); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if got := h.canaryEvents(t); len(got) != 1 || !got[0].Baseline || got[0].Result != watchlist.CanaryPass {
		t.Fatalf("first run should be a passing baseline, got %+v", got)
	}
	if cards := h.findings(t); len(cards) != 0 {
		t.Fatalf("a baseline raised %d cards, want 0", len(cards))
	}

	// A drop past the tolerance on the next cadence: a card.
	rate = 0.6
	clk.advance(8 * 24 * time.Hour)
	if _, err := c.RunDue(context.Background()); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	got := h.canaryEvents(t)
	if len(got) != 2 {
		t.Fatalf("canary.result rows = %d, want 2", len(got))
	}
	if got[1].Result != watchlist.CanaryFail {
		t.Fatalf("a 0.4 pass-rate drop did not fail the canary: %+v", got[1])
	}
	if got[1].Delta >= 0 {
		t.Errorf("delta = %v, want negative", got[1].Delta)
	}
	cards := h.findings(t)
	if len(cards) != 1 {
		t.Fatalf("drift.finding rows = %d, want 1", len(cards))
	}
	if cards[0].ChangeClass != watchlist.ClassModels {
		t.Errorf("change class = %q, want models", cards[0].ChangeClass)
	}
	// A subscription lane carries no model id, so the finding must RECORD that
	// no revalidation fired rather than faking one (OQ4(a)).
	if cards[0].Revalidation.Triggered {
		t.Error("a subscription-lane behavioral drop must not claim a revalidation — it carries no model-id subject")
	}
	if cards[0].Revalidation.Reason == "" {
		t.Error("the card must record WHY no revalidation was triggered")
	}
}

// TestBehavioralCanaryToleratesSmallMovement: a within-tolerance move passes.
func TestBehavioralCanaryToleratesSmallMovement(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}
	c := h.canaries(t, clk)

	rate := 1.0
	c.Behavioral = &watchlist.BehavioralCanary{
		Lanes: []string{watchlist.LaneAnthropic},
		Run: func(ctx context.Context, lane string) (watchlist.BehavioralOutcome, error) {
			return watchlist.BehavioralOutcome{Suite: "s", Cases: 5, PassRate: rate}, nil
		},
	}
	if _, err := c.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	rate = 0.85
	clk.advance(8 * 24 * time.Hour)
	if _, err := c.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := h.canaryEvents(t)
	if len(got) != 2 || got[1].Result != watchlist.CanaryPass {
		t.Fatalf("a 0.15 move (inside the 0.2 tolerance) should pass, got %+v", got)
	}
	if cards := h.findings(t); len(cards) != 0 {
		t.Errorf("a within-tolerance move raised %d cards, want 0", len(cards))
	}
}

// ── R26: the logprob canary is LOCAL LANE ONLY ─────────────────────────────

// TestLogprobCanaryRefusesANonLocalLane is the checkable negative the brief
// requires: Z.AI exposes no logprobs endpoint-wide and the wrapped Anthropic
// CLI exposes none either (S03.7), so the canary refuses those lanes outright.
func TestLogprobCanaryRefusesANonLocalLane(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}
	c := h.canaries(t, clk)
	lp := watchlist.NewLogprobCanary(nil, nil)

	for _, lane := range []string{watchlist.LaneAnthropic, watchlist.LaneZAI, "xai", ""} {
		_, err := lp.Run(context.Background(), c, lane)
		if !errors.Is(err, watchlist.ErrLaneNotLocal) {
			t.Errorf("logprob canary on lane %q: err = %v, want ErrLaneNotLocal (S03.7)", lane, err)
		}
	}
	// And the refusal is not vacuous: the local lane gets past the lane gate
	// and fails only on the absent stack.
	if _, err := lp.Run(context.Background(), c, watchlist.LaneLocal); errors.Is(err, watchlist.ErrLaneNotLocal) {
		t.Error("the local lane was refused — the lane gate is inverted")
	}
}

// TestLogprobCanaryOnlyEverSchedulesTheLocalLane: the layer never puts a
// logprob job on a paid lane in the first place.
func TestLogprobCanaryOnlyEverSchedulesTheLocalLane(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}
	c := h.canaries(t, clk)
	c.Logprob = watchlist.NewLogprobCanary(nil, nil)

	sweep, err := c.RunDue(context.Background())
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	// One job, on the local lane, disarmed (no duty stack).
	if sweep.Disarmed != 1 || sweep.Ran != 0 {
		t.Fatalf("sweep = %+v, want exactly one (local, disarmed) logprob job", sweep)
	}
}

// ── R27: model-list drift, the first producer of a model-id subject ─────────

func TestModelListCanaryCarriesModelIDsIntoRevalidation(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}

	var flagged []string
	h.em.Revalidate = func(ctx context.Context, modelID, reason string) ([]string, error) {
		flagged = append(flagged, modelID)
		return []string{"tmpl-" + modelID}, nil
	}
	c := h.canaries(t, clk)

	observed := []string{"claude-opus-4-8", "claude-sonnet-4-6"}
	c.ModelList = &watchlist.ModelListCanary{
		Lanes: []string{watchlist.LaneAnthropic},
		Probe: func(ctx context.Context, lane string) ([]string, error) { return observed, nil },
	}

	// First observation is the baseline: no config is registered at v0.
	if _, err := c.RunDue(context.Background()); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if got := h.canaryEvents(t); len(got) != 1 || !got[0].Baseline {
		t.Fatalf("first run should baseline, got %+v", got)
	}
	if len(flagged) != 0 {
		t.Fatalf("a baseline flagged %v, want nothing", flagged)
	}

	// The account loses one model and gains one.
	observed = []string{"claude-opus-4-8", "claude-haiku-4-6"}
	clk.advance(48 * time.Hour)
	if _, err := c.RunDue(context.Background()); err != nil {
		t.Fatalf("RunDue: %v", err)
	}

	got := h.canaryEvents(t)
	if len(got) != 2 || got[1].Result != watchlist.CanaryFail {
		t.Fatalf("model-list drift did not fail the canary: %+v", got)
	}
	if got[1].Delta != 2 {
		t.Errorf("delta = %v, want 2 (one removed, one added)", got[1].Delta)
	}
	if got[1].VerifiedOn.IsZero() {
		t.Error("the observed list must carry a verified-on date (S03.6)")
	}

	// Both model ids reached the S14.8 runbook — this is the packet's whole
	// point: 6A's sources name no model id and this canary does.
	want := map[string]bool{"claude-sonnet-4-6": true, "claude-haiku-4-6": true}
	if len(flagged) != 2 {
		t.Fatalf("revalidation subjects = %v, want the removed and the added model id", flagged)
	}
	for _, m := range flagged {
		if !want[m] {
			t.Errorf("revalidation fired for an unexpected subject %q", m)
		}
	}

	cards := h.findings(t)
	if len(cards) != 2 {
		t.Fatalf("drift.finding rows = %d, want one per model id", len(cards))
	}
	if cards[0].Fingerprint != cards[1].Fingerprint {
		t.Errorf("the two findings carry different fingerprints (%q vs %q) — they must fold into ONE card",
			cards[0].Fingerprint, cards[1].Fingerprint)
	}
	for _, f := range cards {
		if f.ChangeClass != watchlist.ClassModels {
			t.Errorf("change class = %q, want models", f.ChangeClass)
		}
		if !f.Revalidation.Triggered {
			t.Errorf("finding for %q did not trigger revalidation despite naming a model id", f.Revalidation.Subject)
		}
	}
}

// TestModelListCanaryDiffsAgainstConfigWhenRegistered: with a configured list
// the diff is observed-vs-config (S03.6), not observed-vs-previous.
func TestModelListCanaryDiffsAgainstConfigWhenRegistered(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}
	c := h.canaries(t, clk)
	c.ModelList = watchlist.NewModelListCanary(
		func(ctx context.Context, lane string) ([]string, error) { return []string{"m-a"}, nil },
		map[string][]string{watchlist.LaneAnthropic: {"m-a", "m-b"}},
	)
	c.ModelList.Lanes = []string{watchlist.LaneAnthropic}

	if _, err := c.RunDue(context.Background()); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	got := h.canaryEvents(t)
	if len(got) != 1 {
		t.Fatalf("canary.result rows = %d, want 1", len(got))
	}
	if got[0].Baseline {
		t.Fatal("a configured list means there is no baseline to establish — the diff runs on the first observation")
	}
	if got[0].Result != watchlist.CanaryFail || len(got[0].Subjects) != 1 || got[0].Subjects[0] != "m-b" {
		t.Fatalf("want a failure naming the missing configured model m-b, got %+v", got[0])
	}
}

// TestModelListCanaryIgnoresReordering: a provider reordering its catalogue is
// not drift.
func TestModelListCanaryIgnoresReordering(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}
	c := h.canaries(t, clk)
	list := []string{"a", "b", "c"}
	c.ModelList = &watchlist.ModelListCanary{
		Lanes: []string{watchlist.LaneAnthropic},
		Probe: func(ctx context.Context, lane string) ([]string, error) { return list, nil },
	}
	if _, err := c.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	list = []string{"c", "a", "b"}
	clk.advance(48 * time.Hour)
	if _, err := c.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := h.canaryEvents(t)
	if len(got) != 2 || got[1].Result != watchlist.CanaryPass {
		t.Fatalf("a reorder was reported as drift: %+v", got)
	}
}

// TestHTTPModelListProbeReadsAnOpenAICompatibleCatalogue proves the real leg
// against a stub, and that a refusal is an ERROR rather than an empty list —
// reporting "no models" for a 403 would raise a removal finding for every model
// the account has.
func TestHTTPModelListProbeReadsAnOpenAICompatibleCatalogue(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"object":"list","data":[{"id":"glm-4.7"},{"id":"glm-4.7-air"}]}`)
	}))
	defer srv.Close()

	probe := watchlist.HTTPModelListProbe(srv.Client(), srv.URL, "authorization", func() string { return "Bearer x" })
	ids, err := probe(context.Background(), watchlist.LaneZAI)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if len(ids) != 2 || ids[0] != "glm-4.7" {
		t.Fatalf("ids = %v, want the two catalogue entries", ids)
	}

	status = http.StatusForbidden
	if _, err := probe(context.Background(), watchlist.LaneZAI); err == nil {
		t.Fatal("a 403 returned no error — a refused catalogue must never read as an empty one")
	}
}

// ── R28 / OQ5(a): the conformance canary is a dueness surfacer ──────────────

func TestConformanceCanarySurfacesDueAdapterRowsOncePerWindow(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}

	reg := conformance.NewStore(h.db, h.log, h.reg)
	if _, err := reg.EnsureSeeded(ctx); err != nil {
		t.Fatalf("seed conformance registry: %v", err)
	}
	c := h.canaries(t, clk)
	c.Conformance = watchlist.NewConformanceCanary(reg)

	sweep, err := c.RunDue(ctx)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	// Exactly the two weekly lane-affecting adapter rows — the narrowing of
	// B5-4's "dueness raises nothing at v0" (OQ5(a)).
	if sweep.Cards != 2 {
		t.Fatalf("conformance cards = %d, want 2 (adapter-anthropic + adapter-local)", sweep.Cards)
	}
	cards := h.findings(t)
	seen := map[string]bool{}
	for _, f := range cards {
		seen[f.RowID] = true
		if f.Severity != watchlist.SeverityDigest {
			t.Errorf("row %q severity = %q, want daily-digest — a due suite is a reminder, not a stall",
				f.RowID, f.Severity)
		}
		if f.ChangeClass != watchlist.ClassUnclear {
			t.Errorf("row %q change class = %q, want unclear (nothing has been observed)", f.RowID, f.ChangeClass)
		}
	}
	for _, want := range []string{"adapter-anthropic", "adapter-local"} {
		if !seen[want] {
			t.Errorf("no card for the due weekly lane-affecting row %q", want)
		}
	}

	// Re-sweeping inside the window folds into the open card rather than
	// re-carding: the row stays due until an operator runs it.
	before := len(h.findings(t))
	clk.advance(time.Hour)
	if _, err := c.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if after := len(h.findings(t)); after != before {
		t.Errorf("a re-sweep inside the window raised %d more findings, want 0", after-before)
	}

	// Past the window, the reminder repeats.
	clk.advance(8 * 24 * time.Hour)
	if _, err := c.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if after := len(h.findings(t)); after != before+2 {
		t.Errorf("findings after the window = %d, want %d", after, before+2)
	}
}

// TestConformanceCanaryRecordsThroughTheRegistryVerb: the RUN is an operator/CI
// act and lands through the existing RecordResult into the existing row — no
// new row, no new event type.
func TestConformanceCanaryRecordsThroughTheRegistryVerb(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	reg := conformance.NewStore(h.db, h.log, h.reg)
	if _, err := reg.EnsureSeeded(ctx); err != nil {
		t.Fatal(err)
	}
	cc := watchlist.NewConformanceCanary(reg)

	res := conformance.Result{
		RowID: "adapter-anthropic", SuiteVersion: "claudecli-conformance@2.1.215",
		AssetID: "engine:claude-cli", AssetVersion: "2.1.215",
		Runner: "go test ./internal/adapters/claudecli/", RunnerVersion: "go1.26.5",
		Metrics: map[string]any{"cases_total": 42, "cases_passed": 42}, Passed: true,
	}
	if err := cc.RecordAdapterSuite(ctx, res); err != nil {
		t.Fatalf("RecordAdapterSuite: %v", err)
	}

	rows, err := reg.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range rows {
		if r.ID == "adapter-anthropic" {
			found = true
			if r.LastResult != conformance.ResultGreen {
				t.Errorf("last_result = %q, want green", r.LastResult)
			}
			if r.Due {
				t.Error("a just-recorded weekly row is still due")
			}
		}
	}
	if !found {
		t.Fatal("adapter-anthropic row missing")
	}
	// No canary.result is minted for a conformance run: its recording home is
	// the already-minted eval.score_recorded.
	if got := h.canaryEvents(t); len(got) != 0 {
		t.Errorf("a conformance run minted %d canary.result rows, want 0", len(got))
	}

	// The verb refuses a row it does not own.
	res.RowID = "kill9-harness"
	if err := cc.RecordAdapterSuite(ctx, res); err == nil {
		t.Error("RecordAdapterSuite accepted a row that is not a weekly lane-affecting adapter row")
	}
}

// ── Dueness derives from stored state (no ticker, restart/suspend safe) ─────

func TestCanaryDuenessDerivesFromTheLogAcrossARestart(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}

	runs := 0
	build := func() *watchlist.Canaries {
		c := h.canaries(t, clk)
		c.Auth = &watchlist.AuthCanary{
			Lanes: []string{watchlist.LaneAnthropic},
			Probe: func(ctx context.Context, lane string) (scheduler.LimitSignal, error) {
				runs++
				return scheduler.LimitSignal{HTTPStatus: 200}, nil
			},
		}
		return c
	}

	if _, err := build().RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("runs = %d, want 1", runs)
	}

	// A brand-new layer (the restart) an hour later must NOT re-run: dueness is
	// the last canary.result in the log, not an in-memory ticker.
	clk.advance(time.Hour)
	if _, err := build().RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("a restart re-ran the canary inside its cadence (runs = %d)", runs)
	}

	// Past ⚙ canary.auth_interval (default 24 h) it runs again — including
	// across a wall-clock jump, which a ticker would have slept through.
	clk.advance(25 * time.Hour)
	if _, err := build().RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if runs != 2 {
		t.Fatalf("runs = %d after the cadence elapsed, want 2", runs)
	}
}

// TestCanaryIntervalsAreReadLive proves live-apply for BOTH declared ⚙ keys: a
// mid-run change to the interval takes effect on the next sweep without a
// restart.
func TestCanaryIntervalsAreReadLive(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		key string
		// advance is a gap INSIDE the key's default cadence but OUTSIDE its
		// clamp floor, so the same movement is not-due before the change and
		// due after it — which is what proves the read is live.
		advance time.Duration
		tighten string
		wire    func(*watchlist.Canaries, *int)
	}{
		{"canary.auth_interval", 7 * time.Hour, `21600`, func(c *watchlist.Canaries, n *int) {
			c.Auth = &watchlist.AuthCanary{
				Lanes: []string{watchlist.LaneAnthropic},
				Probe: func(ctx context.Context, lane string) (scheduler.LimitSignal, error) {
					*n++
					return scheduler.LimitSignal{HTTPStatus: 200}, nil
				},
			}
		}},
		{"canary.behavioral_interval", 48 * time.Hour, `86400`, func(c *watchlist.Canaries, n *int) {
			c.Behavioral = &watchlist.BehavioralCanary{
				Lanes: []string{watchlist.LaneAnthropic},
				Run: func(ctx context.Context, lane string) (watchlist.BehavioralOutcome, error) {
					*n++
					return watchlist.BehavioralOutcome{Suite: "s", Cases: 5, PassRate: 1}, nil
				},
			}
		}},
	} {
		t.Run(tc.key, func(t *testing.T) {
			h := newHarness(t)
			clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}
			c := h.canaries(t, clk)
			runs := 0
			tc.wire(c, &runs)

			if _, err := c.RunDue(ctx); err != nil {
				t.Fatal(err)
			}
			if runs != 1 {
				t.Fatalf("runs = %d, want 1", runs)
			}

			// Inside the key's DEFAULT cadence, so nothing runs.
			clk.advance(tc.advance)
			if _, err := c.RunDue(ctx); err != nil {
				t.Fatal(err)
			}
			if runs != 1 {
				t.Fatalf("the canary ran inside its default cadence (runs = %d)", runs)
			}

			// The operator tightens the key to its clamp floor MID-RUN. No
			// restart, no re-composition: the same layer re-reads the key.
			if err := h.reg.Set(ctx, settings.SetRequest{
				Key: tc.key, Value: json.RawMessage(tc.tighten),
				Actor: settings.Actor{Kind: "operator", ID: "op"}, Reason: "tighten the canary cadence",
			}); err != nil {
				t.Fatalf("Set %s: %v", tc.key, err)
			}
			if _, err := c.RunDue(ctx); err != nil {
				t.Fatal(err)
			}
			if runs != 2 {
				t.Fatalf("a live ⚙ change to %s did not apply without a restart (runs = %d)", tc.key, runs)
			}
		})
	}
}

// ── Metering posture (R24) ─────────────────────────────────────────────────

// TestCanaryMeteringPostureIsTheExistingProbeClass: the canary layer reuses the
// existing S10.1 purpose tag and S10.7 workload class and invents neither.
func TestCanaryMeteringPostureIsTheExistingProbeClass(t *testing.T) {
	if string(watchlist.CanaryPurposeTag) != "probe" {
		t.Errorf("purpose tag = %q, want the existing metering.PurposeProbe", watchlist.CanaryPurposeTag)
	}
	if string(watchlist.CanaryWorkloadClass) != "probe" {
		t.Errorf("workload class = %q, want the existing scheduler.ClassProbe", watchlist.CanaryWorkloadClass)
	}
	if watchlist.CanaryWorkloadClass != scheduler.ClassProbe {
		t.Error("the canary workload class is not the scheduler's own ClassProbe constant")
	}
	if watchlist.CanaryPurposeTag != metering.PurposeProbe {
		t.Error("the canary purpose tag is not metering's own PurposeProbe constant")
	}
}

// ── R29 / the arm ──────────────────────────────────────────────────────────

func TestCanaryArmedReadsTheOperatorEnv(t *testing.T) {
	t.Setenv(watchlist.CanaryArmEnv, "")
	if watchlist.CanaryArmed() {
		t.Error("the canary legs are armed with no operator act — the $0 posture is the default")
	}
	for _, on := range []string{"1", "true", "TRUE", "yes", "on"} {
		t.Setenv(watchlist.CanaryArmEnv, on)
		if !watchlist.CanaryArmed() {
			t.Errorf("%s=%q did not arm the legs", watchlist.CanaryArmEnv, on)
		}
	}
	t.Setenv(watchlist.CanaryArmEnv, "0")
	if watchlist.CanaryArmed() {
		t.Error("an explicit 0 armed the legs")
	}
}

// TestProbeErrorIsRecordedHonestly: a probe that could not complete proves
// nothing about sanction — it fails the canary as `unclear` and routes to the
// digest, never fabricating a policy revocation.
func TestProbeErrorIsRecordedHonestly(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-07-27T09:00:00Z")}
	c := h.canaries(t, clk)
	c.Auth = &watchlist.AuthCanary{
		Lanes: []string{watchlist.LaneAnthropic},
		Probe: func(ctx context.Context, lane string) (scheduler.LimitSignal, error) {
			return scheduler.LimitSignal{}, errors.New("dial tcp: connection refused")
		},
	}
	if _, err := c.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := h.canaryEvents(t)
	if len(got) != 1 || got[0].Result != watchlist.CanaryFail {
		t.Fatalf("want one failing canary.result, got %+v", got)
	}
	if got[0].ChangeClass != watchlist.ClassUnclear {
		t.Errorf("change class = %q, want unclear", got[0].ChangeClass)
	}
	if got[0].Error == "" {
		t.Error("the probe error was not recorded")
	}
	cards := h.findings(t)
	if len(cards) != 1 || cards[0].Severity != watchlist.SeverityDigest {
		t.Fatalf("an unreachable lane must raise a DIGEST card, got %+v", cards)
	}
}

package watchlist_test

// zai_ln2b_test.go — P3-LN-2B §9 specs 20-23 (S14.6, S03.7, P-T17-1, P-T17-3).
//
// Hermetic and $0: every probe terminates on an httptest stub bound to
// loopback. The real-request legs stay DISARMED in production — arming them is
// LN-CEREMONY's — so what is asserted here is the SHAPE of the zai legs and
// what they do with an answer, never a live provider.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/watchlist"
)

// spec 20 — the zai auth canary is scheduled on ⚙ canary.auth_interval, an
// auth-shaped failure freezes the lane and flags now, and the record carries
// its metering posture.
func TestZAIAuthCanaryRegisteredAndFreezesOnAuthShape(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-08-23T09:00:00Z")}
	ctx := context.Background()

	if !laneIn(watchlist.NewAuthCanary(nil).Lanes, watchlist.LaneZAI) {
		t.Fatal("the auth canary does not carry lane zai — the paid lane whose sanction can be revoked is not probed")
	}

	// A 403 on valid credentials: the Class-4 shape (S10.5).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":"1220","message":"You do not have permission to access chat/completions"}}`)
	}))
	defer srv.Close()

	c := h.canaries(t, clk)
	c.Auth = watchlist.NewAuthCanary(
		watchlist.HTTPAuthProbe(srv.Client(), srv.URL, "Authorization", func() string { return "Bearer sentinel" }))
	c.Auth.Lanes = []string{watchlist.LaneZAI}

	sweep, err := c.RunDue(ctx)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if sweep.Ran != 1 {
		t.Fatalf("ran %d legs, want 1 (the zai auth canary)", sweep.Ran)
	}

	events := h.canaryEvents(t)
	if len(events) != 1 {
		t.Fatalf("recorded %d canary results, want 1", len(events))
	}
	got := events[0]
	if got.CanaryKind != watchlist.CanaryAuth || got.Lane != watchlist.LaneZAI {
		t.Fatalf("result = %s/%s, want auth/zai", got.CanaryKind, got.Lane)
	}
	if got.LimitClass != int(scheduler.ClassAuthPolicy) {
		t.Errorf("limit class = %d, want %d (auth/policy)", got.LimitClass, scheduler.ClassAuthPolicy)
	}
	if got.Action != string(scheduler.ActionLaneFreeze) {
		t.Errorf("action = %q, want %q — an auth-shaped failure NEVER retry-parks (S10.5 Class 4)",
			got.Action, scheduler.ActionLaneFreeze)
	}
	if got.Action == string(scheduler.ActionParkProbe) || got.Action == string(scheduler.ActionRetryInPlace) {
		t.Error("the zai auth canary reached a park/retry — the P-T08-2 failure class")
	}
	if got.Result != watchlist.CanaryFail {
		t.Errorf("result = %q, want fail (the card is flag-now)", got.Result)
	}
	// Consumption is metered like everything else and itemized as a probe (D4).
	if got.PurposeTag != string(metering.PurposeProbe) {
		t.Errorf("purpose_tag = %q, want %q — canary consumption is metered (D4)", got.PurposeTag, metering.PurposeProbe)
	}
	if got.WorkloadClass == "" {
		t.Error("the canary record carries no workload class")
	}
	if got.VerifiedOn.IsZero() {
		t.Error("the canary record carries no verified-on stamp")
	}
	if len(h.findings(t)) == 0 {
		t.Error("no drift card was raised for a frozen lane — flag-now is the whole point")
	}

	// The cadence is ⚙ canary.auth_interval's, not a constant: immediately
	// after a run nothing is due, and after the ratified default it is.
	if again, err := c.RunDue(ctx); err != nil || again.Ran != 0 {
		t.Fatalf("re-ran %d legs immediately (err=%v) — the ⚙ interval is not honored", again.Ran, err)
	}
	clk.advance(25 * time.Hour)
	if later, err := c.RunDue(ctx); err != nil || later.Ran != 1 {
		t.Fatalf("after the ⚙ auth interval %d legs ran (err=%v), want 1", later.Ran, err)
	}
}

// spec 21 — the model-list canary is registered for zai and tolerates an
// endpoint that does not serve /models, honestly and dated (P-T17-3, §1).
func TestZAIModelListCanaryRegisteredAndToleratesAbsence(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-08-23T09:00:00Z")}
	ctx := context.Background()

	if !laneIn(watchlist.NewModelListCanary(nil, nil).Lanes, watchlist.LaneZAI) {
		t.Fatal("the model-list canary does not carry lane zai")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	probe := watchlist.HTTPModelListProbe(srv.Client(), srv.URL, "Authorization", func() string { return "Bearer sentinel" })
	if _, err := probe(ctx, watchlist.LaneZAI); !errors.Is(err, watchlist.ErrModelListUnavailable) {
		t.Fatalf("a 404 on /models yielded %v, want ErrModelListUnavailable — the coding endpoint's model list is undocumented and its absence must be reportable", err)
	}

	c := h.canaries(t, clk)
	c.ModelList = watchlist.NewModelListCanary(probe, map[string][]string{
		watchlist.LaneZAI: {"glm-4.7", "glm-5-turbo", "glm-5.3"},
	})
	c.ModelList.Lanes = []string{watchlist.LaneZAI}

	if _, err := c.RunDue(ctx); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	events := h.canaryEvents(t)
	if len(events) != 1 {
		t.Fatalf("recorded %d canary results, want 1 — an unavailable list is REPORTED, not swallowed", len(events))
	}
	got := events[0]
	if got.Lane != watchlist.LaneZAI || got.CanaryKind != watchlist.CanaryModelList {
		t.Fatalf("result = %s/%s, want model-list/zai", got.CanaryKind, got.Lane)
	}
	if got.Result != watchlist.CanaryPass {
		t.Errorf("result = %q — an endpoint that serves no model list is not a canary FAILURE, it is an honest absence", got.Result)
	}
	if len(got.Observed) != 0 || got.ObservedCount != 0 {
		t.Errorf("observed = %v (count %d) — nothing was observed and nothing may be fabricated", got.Observed, got.ObservedCount)
	}
	if got.Delta != 0 {
		t.Errorf("delta = %v, want 0 — no diff was possible", got.Delta)
	}
	if !strings.Contains(strings.ToLower(got.Summary), "unavailable") {
		t.Errorf("summary = %q — it must SAY the observed list was unavailable", got.Summary)
	}
	if got.VerifiedOn.IsZero() {
		t.Error("the unavailable result carries no verified-on date (S03.6)")
	}
	if n := len(h.findings(t)); n != 0 {
		t.Errorf("an unavailable model list raised %d cards, want 0", n)
	}
}

// spec 22 — zai has a behavioral canary and NO logprob canary, with the reason
// explicit in code rather than an accidental omission (S03.7; V11).
func TestZAIHasBehavioralCanaryAndNoLogprobCanary(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-08-23T09:00:00Z")}
	ctx := context.Background()

	if !laneIn(watchlist.NewBehavioralCanary(nil).Lanes, watchlist.LaneZAI) {
		t.Fatal("the behavioral canary does not carry lane zai — it is the lane's ONLY drift detection (S03.7)")
	}

	c := h.canaries(t, clk)
	c.Behavioral = watchlist.NewBehavioralCanary(func(_ context.Context, lane string) (watchlist.BehavioralOutcome, error) {
		return watchlist.BehavioralOutcome{Runner: "fixture", RunnerVersion: "0", Suite: "zai-mini", Cases: 4, PassRate: 1}, nil
	})
	c.Behavioral.Lanes = []string{watchlist.LaneZAI}
	c.Logprob = watchlist.NewLogprobCanary(nil, nil)

	if _, err := c.RunDue(ctx); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	sawBehavioral := false
	for _, ev := range h.canaryEvents(t) {
		if ev.CanaryKind == watchlist.CanaryLogprob && ev.Lane == watchlist.LaneZAI {
			t.Error("a logprob canary ran on lane zai — the coding endpoint exposes no logprobs (S03.7)")
		}
		if ev.CanaryKind == watchlist.CanaryBehavioral && ev.Lane == watchlist.LaneZAI {
			sawBehavioral = true
		}
	}
	if !sawBehavioral {
		t.Error("no behavioral canary result for lane zai")
	}

	// The refusal is structural, not incidental.
	if _, err := c.Logprob.Run(ctx, c, watchlist.LaneZAI); !errors.Is(err, watchlist.ErrLaneNotLocal) {
		t.Errorf("the logprob canary accepted lane zai (err = %v)", err)
	}
	// And the reason is written down where a reader will find it.
	src := readWatchlistSource(t, "canary_logprob.go")
	for _, needle := range []string{"S03.7", "logprob"} {
		if !strings.Contains(src, needle) {
			t.Errorf("canary_logprob.go does not record %q as the reason zai has no logprob canary", needle)
		}
	}
}

// spec 23 — the zai watch rows carry the URLs re-verified in the brief's §1,
// each with its date. A row nobody re-checked is a row that decays silently.
func TestZAIWatchRowURLsVerified(t *testing.T) {
	want := map[string]string{
		"t1-zai-devpack":       "https://docs.z.ai/devpack/overview",
		"t1-zai-release-notes": "https://docs.z.ai/release-notes/new-released",
		"t4-hn-zai":            "https://hnrss.org/newest?q=Z.ai+GLM&points=50",
	}
	seen := map[string]bool{}
	for _, r := range watchlist.SeedRows() {
		url, ok := want[r.ID]
		if !ok {
			continue
		}
		seen[r.ID] = true
		if r.URL != url {
			t.Errorf("row %s points at %q, want the verified %q", r.ID, r.URL, url)
		}
		if r.Lane != watchlist.LaneZAI {
			t.Errorf("row %s carries lane %q, want zai", r.ID, r.Lane)
		}
		if !strings.Contains(r.Notes, "2026-08-23") {
			t.Errorf("row %s carries no re-verification date in its notes: %q", r.ID, r.Notes)
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("the seed carries no zai watch row %q", id)
		}
	}
}

func readWatchlistSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func laneIn(lanes []string, want string) bool {
	for _, l := range lanes {
		if l == want {
			return true
		}
	}
	return false
}

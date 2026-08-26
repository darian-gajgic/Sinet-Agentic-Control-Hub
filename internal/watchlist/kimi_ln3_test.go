package watchlist_test

// kimi_ln3_test.go — P3-LN-3 §6 specs 13-17 (S14.6, S03.6, P-T17-1, P-T17-3, R02 §6).
//
// Hermetic and $0: every probe terminates on an httptest stub bound to
// loopback. The real-request legs stay DISARMED in production — arming them is
// LN-CEREMONY's — so what is asserted here is the SHAPE of the kimi legs and
// what they do with an answer, never a live provider. URLs are compared as
// host+path PARTS, never as routable literals (the package's own tripwire).

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/conformance"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/watchlist"
)

// ── spec 13 · the kimi auth canary is registered and freezes on an auth shape ─

func TestKimiAuthCanaryRegisteredAndFreezesOnAuthShape(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-08-24T09:00:00Z")}
	ctx := context.Background()

	if !laneIn(watchlist.NewAuthCanary(nil).Lanes, watchlist.LaneKimi) {
		t.Fatal("the auth canary does not carry lane kimi — a paid lane whose sanction can be revoked is not probed")
	}

	// DOCUMENTED-NOT-OBSERVED (§8 (iv)). The body below is the vendor's
	// PUBLISHED account-suspension string as captured in the 2026-08-24 audit,
	// wrapped in a plausible envelope — not a response this platform has seen.
	// The audit could not verify the JSON shape of a Kimi error body or whether
	// it carries a code field (U3), so the ceremony's live single-request probe
	// is what turns this fixture into an observation.
	//
	// It is the ONE 403 on this lane that is genuinely an auth event, and the
	// auth canary is the authoritative revocation detector for whatever the
	// message grammar misses (P-T17-1).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"message":"Access terminated"}}`)
	}))
	defer srv.Close()

	c := h.canaries(t, clk)
	c.Auth = watchlist.NewAuthCanary(
		watchlist.HTTPAuthProbe(srv.Client(), srv.URL, "Authorization", func() string { return "Bearer sentinel" }))
	c.Auth.Lanes = []string{watchlist.LaneKimi}

	sweep, err := c.RunDue(ctx)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if sweep.Ran != 1 {
		t.Fatalf("ran %d legs, want 1 (the kimi auth canary)", sweep.Ran)
	}

	events := h.canaryEvents(t)
	if len(events) != 1 {
		t.Fatalf("recorded %d canary results, want 1", len(events))
	}
	got := events[0]
	if got.CanaryKind != watchlist.CanaryAuth || got.Lane != watchlist.LaneKimi {
		t.Fatalf("result = %s/%s, want auth/kimi", got.CanaryKind, got.Lane)
	}
	if got.LimitClass != int(scheduler.ClassAuthPolicy) {
		t.Errorf("limit class = %d, want %d (auth/policy)", got.LimitClass, scheduler.ClassAuthPolicy)
	}
	if got.Action != string(scheduler.ActionLaneFreeze) {
		t.Errorf("action = %q, want %q — an auth-shaped failure NEVER retry-parks (S10.5 Class 4)", got.Action, scheduler.ActionLaneFreeze)
	}
	if got.Action == string(scheduler.ActionParkProbe) || got.Action == string(scheduler.ActionRetryInPlace) {
		t.Error("the kimi auth canary reached a park/retry — the P-T08-2 failure class")
	}
	if got.Result != watchlist.CanaryFail {
		t.Errorf("result = %q, want fail (the card is flag-now)", got.Result)
	}
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

	// The cadence is ⚙ canary.auth_interval's, in both directions.
	if again, err := c.RunDue(ctx); err != nil || again.Ran != 0 {
		t.Fatalf("re-ran %d legs immediately (err=%v) — the ⚙ interval is not honored", again.Ran, err)
	}
	clk.advance(25 * time.Hour)
	if later, err := c.RunDue(ctx); err != nil || later.Ran != 1 {
		t.Fatalf("after the ⚙ auth interval %d legs ran (err=%v), want 1", later.Ran, err)
	}
}

// ── spec 14 · the model-list canary tolerates absence AND catches the tier gate ─

func TestKimiModelListCanaryToleratesAbsenceAndCatchesTierGate(t *testing.T) {
	ctx := context.Background()
	configured := map[string][]string{
		watchlist.LaneKimi: {"k3", "k3-256k", "kimi-for-coding", "kimi-for-coding-highspeed"},
	}

	if !laneIn(watchlist.NewModelListCanary(nil, nil).Lanes, watchlist.LaneKimi) {
		t.Fatal("the model-list canary does not carry lane kimi — the tier gate is exactly what it exists to catch")
	}

	t.Run("an absent model list is an honest absence", func(t *testing.T) {
		h := newHarness(t)
		clk := &clock{t: mustTime(t, "2026-08-24T09:00:00Z")}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}))
		defer srv.Close()

		probe := watchlist.HTTPModelListProbe(srv.Client(), srv.URL, "Authorization", func() string { return "Bearer sentinel" })
		if _, err := probe(ctx, watchlist.LaneKimi); !errors.Is(err, watchlist.ErrModelListUnavailable) {
			t.Fatalf("a 404 on /models yielded %v, want ErrModelListUnavailable", err)
		}
		c := h.canaries(t, clk)
		c.ModelList = watchlist.NewModelListCanary(probe, configured)
		c.ModelList.Lanes = []string{watchlist.LaneKimi}
		if _, err := c.RunDue(ctx); err != nil {
			t.Fatalf("RunDue: %v", err)
		}
		events := h.canaryEvents(t)
		if len(events) != 1 {
			t.Fatalf("recorded %d canary results, want 1", len(events))
		}
		got := events[0]
		if got.Result != watchlist.CanaryPass {
			t.Errorf("result = %q — an endpoint that serves no model list is an honest absence, not a failure", got.Result)
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
			t.Error("the unavailable result carries no verified-on date")
		}
		if n := len(h.findings(t)); n != 0 {
			t.Errorf("an unavailable model list raised %d cards, want 0", n)
		}
	})

	t.Run("a membership tier that no longer serves k3 is model drift, not a freeze", func(t *testing.T) {
		h := newHarness(t)
		clk := &clock{t: mustTime(t, "2026-08-24T09:00:00Z")}
		c := h.canaries(t, clk)
		// The account's OBSERVED list is the authority (P-T17-3). A downgrade
		// below Moderato is exactly this: k3 and k3-256k stop being served.
		c.ModelList = watchlist.NewModelListCanary(
			func(context.Context, string) ([]string, error) {
				return []string{"kimi-for-coding"}, nil
			}, configured)
		c.ModelList.Lanes = []string{watchlist.LaneKimi}

		if _, err := c.RunDue(ctx); err != nil {
			t.Fatalf("RunDue: %v", err)
		}
		events := h.canaryEvents(t)
		if len(events) != 1 {
			t.Fatalf("recorded %d canary results, want 1", len(events))
		}
		got := events[0]
		if got.ChangeClass != watchlist.ClassModels {
			t.Errorf("change class = %q, want models — a tier gate that stops serving k3 is DRIFT", got.ChangeClass)
		}
		if got.Action == string(scheduler.ActionLaneFreeze) {
			t.Error("a tier downgrade froze the lane — the lane is healthy, it simply stopped serving a model, and freezing hides the one fact worth acting on")
		}
		if got.Result != watchlist.CanaryFail {
			t.Errorf("result = %q, want fail so the card is raised", got.Result)
		}
		named := strings.Join(got.Subjects, ",")
		if !strings.Contains(named, "k3") {
			t.Errorf("subjects = %v — the drift must NAME the models that left, so worker revalidation can act on them", got.Subjects)
		}
		if len(h.findings(t)) == 0 {
			t.Error("model drift raised no card")
		}
	})
}

// ── spec 15 · behavioral yes, logprob refused ────────────────────────────────

func TestKimiBehavioralCanaryAndNoLogprobCanary(t *testing.T) {
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-08-24T09:00:00Z")}
	ctx := context.Background()

	if !laneIn(watchlist.NewBehavioralCanary(nil).Lanes, watchlist.LaneKimi) {
		t.Fatal("the behavioral canary does not carry lane kimi — it is this lane's ONLY drift detection (S03.7 posture)")
	}

	c := h.canaries(t, clk)
	c.Behavioral = watchlist.NewBehavioralCanary(func(_ context.Context, lane string) (watchlist.BehavioralOutcome, error) {
		return watchlist.BehavioralOutcome{Runner: "fixture", RunnerVersion: "0", Suite: "kimi-mini", Cases: 4, PassRate: 1}, nil
	})
	c.Behavioral.Lanes = []string{watchlist.LaneKimi}
	c.Logprob = watchlist.NewLogprobCanary(nil, nil)

	if _, err := c.RunDue(ctx); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	sawBehavioral := false
	for _, ev := range h.canaryEvents(t) {
		if ev.CanaryKind == watchlist.CanaryLogprob && ev.Lane == watchlist.LaneKimi {
			t.Error("a logprob canary ran on lane kimi — the audit found no logprob support on the coding endpoint, and the S03.7 paid-lane posture applies unchanged")
		}
		if ev.CanaryKind == watchlist.CanaryBehavioral && ev.Lane == watchlist.LaneKimi {
			sawBehavioral = true
		}
	}
	if !sawBehavioral {
		t.Error("no behavioral canary result for lane kimi")
	}
	// The refusal is STRUCTURAL: every paid lane is refused BY NAME, and the
	// local lane is the positive control that the refusal is about the lane.
	if !laneIn(watchlist.PaidLanes(), watchlist.LaneKimi) {
		t.Fatal("PaidLanes() does not carry kimi — the real-request legs would never schedule it")
	}
	for _, lane := range watchlist.PaidLanes() {
		if _, err := c.Logprob.Run(ctx, c, lane); !errors.Is(err, watchlist.ErrLaneNotLocal) {
			t.Errorf("the logprob canary accepted paid lane %s (err = %v)", lane, err)
		}
	}
	if _, err := c.Logprob.Run(ctx, c, watchlist.LaneLocal); errors.Is(err, watchlist.ErrLaneNotLocal) {
		t.Error("the logprob canary refused the LOCAL lane as non-local — the refusal would be vacuous")
	}
}

// ── spec 16 · the watch rows the R02 §6 tier-1 set requires ──────────────────

func TestKimiWatchRowsVerified(t *testing.T) {
	// Host and path as PARTS, never a routable literal (the package tripwire).
	want := map[string][2]string{
		"t1-kimi-coding":             {"kimi.com", "/coding"},
		"t1-kimi-membership-pricing": {"www.kimi.com", "/en/help/membership/membership-pricing"},
		"t1-kimi-plan-pricing":       {"www.kimi.com", "/membership/pricing"},
		"t2-modelsdev-kimi-provider": {"models.dev", "/providers/kimi-for-coding"},
		"t1-kimi-error-reference":    {"www.kimi.com", "/code/docs/en/kimi-code/error-reference.html"},
		"t1-kimi-whats-new":          {"www.kimi.com", "/code/docs/en/kimi-code/whats-new.html"},
		"t1-kimi-benefits":           {"www.kimi.com", "/en/help/kimi-code/benefits"},
		"t1-kimi-extra-usage":        {"www.kimi.com", "/en/help/membership/membership-extra-usage"},
		"t1-kimi-price-k3":           {"platform.kimi.ai", "/docs/pricing/chat-k3"},
		"t1-kimi-tos-platform":       {"platform.kimi.ai", "/docs/agreement/modeluse"},
		"t1-kimi-tos-assistant":      {"www.kimi.com", "/user/agreement/modelUse"},
		"t4-hn-kimi":                 {"hnrss.org", "/newest"},
	}
	// The JS-shell rows: a text diff on a shell is near-empty and must never be
	// read as "no change". The audit RE-CONFIRMED both shells on 2026-08-24.
	// Both were RE-CONFIRMED as JavaScript shells on 2026-08-24. A near-empty
	// diff on a shell must never be read as "no change".
	jsShell := map[string]bool{"t1-kimi-coding": true, "t1-kimi-plan-pricing": true}

	seen := map[string]bool{}
	for _, r := range watchlist.SeedRows() {
		wantURL, ok := want[r.ID]
		if !ok {
			continue
		}
		seen[r.ID] = true
		u, err := url.Parse(r.URL)
		if err != nil {
			t.Errorf("row %s carries an unparseable URL %q: %v", r.ID, r.URL, err)
			continue
		}
		if u.Host != wantURL[0] || u.Path != wantURL[1] {
			t.Errorf("row %s points at %s%s, want the verified %s%s", r.ID, u.Host, u.Path, wantURL[0], wantURL[1])
		}
		if r.ID == "t4-hn-kimi" {
			q := u.Query()
			kw := strings.ToLower(q.Get("q"))
			if !strings.Contains(kw, "kimi") && !strings.Contains(kw, "moonshot") {
				t.Errorf("row %s searches for %q, which names neither the provider nor its product", r.ID, q.Get("q"))
			}
			if q.Get("points") == "" {
				t.Errorf("row %s carries no points threshold — the feed is unfiltered noise without one", r.ID)
			}
		}
		if r.Lane != watchlist.LaneKimi {
			t.Errorf("row %s carries lane %q, want kimi — until it does, severityFor routes every price/terms/models/endpoint hit on this lane to the DAILY DIGEST instead of flag-now", r.ID, r.Lane)
		}
		if !strings.Contains(r.Notes, "2026-08-24") {
			t.Errorf("row %s carries no re-verification date in its notes: %q", r.ID, r.Notes)
		}
		// The STAMP, not the word: a row may explain that it used to be a
		// watch-only candidate, but it may not still carry the stamp.
		if strings.Contains(r.Notes, "(candidate)") {
			t.Errorf("row %s is still stamped (candidate); the lane is onboarded: %q", r.ID, r.Notes)
		}
		if jsShell[r.ID] && !strings.Contains(strings.ToLower(r.Notes), "empty diff") {
			t.Errorf("row %s is a JS shell and its notes do not warn that a near-empty diff is not 'no change': %q", r.ID, r.Notes)
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("the seed carries no kimi watch row %q", id)
		}
	}
	// The lane's row COUNT is pinned so prose about it cannot drift from the
	// seed: 10 tier-1 pages, the tier-2 provider record, and the keyword feed.
	lane := 0
	for _, r := range watchlist.SeedRows() {
		if r.Lane == watchlist.LaneKimi {
			lane++
		}
	}
	// 13 since P3-LN-7 added the Community Guidelines page — the SANCTION
	// surface, on the kimi lane because the clause binds the membership and the
	// audit it corrects is this lane's. It is not in `want` above, which pins
	// the twelve rows the LN-3 audit produced.
	if lane != 13 {
		t.Errorf("%d rows carry lane kimi, want 13 — the twelve rows in `want` above plus the community-guidelines "+
			"page added at P3-LN-7, and none unaccounted for", lane)
	}

	// The classifier's constrained-decoding grammar must be able to EMIT the
	// lane, or no hit is ever attributed to it.
	schema := string(watchlist.WatchlistSchema())
	if !strings.Contains(schema, `"kimi"`) {
		t.Error("the watchlist classification schema's lane enum does not name kimi — it is a constrained-decoding grammar, so until it does the local classifier literally cannot emit the lane")
	}
}

// ── spec 25 (half) · the conformance canary resolves the kimi row's lane ─────

func TestKimiConformanceRowRecords(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	reg := conformance.NewStore(h.db, h.log, h.reg)
	if _, err := reg.EnsureSeeded(ctx); err != nil {
		t.Fatal(err)
	}
	cc := watchlist.NewConformanceCanary(reg)

	res := conformance.Result{
		RowID: "adapter-kimi", SuiteVersion: "kimi-lane-conformance@1",
		AssetID: "engine:opencode", AssetVersion: "1.18.3",
		Runner: "go test ./internal/adapters/opencode/", RunnerVersion: "go1.26.5",
		Metrics: map[string]any{"cases_total": 1, "cases_passed": 1}, Passed: true,
	}
	// Without a conformanceRowLane arm the row resolves to "" and the verb
	// refuses it outright — the row would be unrecordable.
	if err := cc.RecordAdapterSuite(ctx, res); err != nil {
		t.Fatalf("RecordAdapterSuite(adapter-kimi): %v — the conformance canary's lane map does not resolve the kimi row", err)
	}
	rows, err := reg.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if r.ID == "adapter-kimi" {
			found = true
			if r.LastResult != conformance.ResultGreen {
				t.Errorf("last_result = %q, want green", r.LastResult)
			}
		}
	}
	if !found {
		t.Fatal("adapter-kimi row missing from the registry")
	}
}

// ── spec 17 · the sweep counts move with the third paid lane ─────────────────

func TestKimiSweepCountsMoveWithTheThirdPaidLane(t *testing.T) {
	if n := len(watchlist.PaidLanes()); n != 3 {
		t.Fatalf("PaidLanes() = %v, want three (anthropic, zai, kimi)", watchlist.PaidLanes())
	}
	h := newHarness(t)
	clk := &clock{t: mustTime(t, "2026-08-24T09:00:00Z")}
	c := h.canaries(t, clk)
	c.Auth = watchlist.DisarmedAuthCanary("no credential is placed until LN-CEREMONY")
	c.ModelList = watchlist.DisarmedModelListCanary("no credential is placed until LN-CEREMONY")
	c.Behavioral = watchlist.DisarmedBehavioralCanary("no pinned runner is installed")

	sweep, err := c.RunDue(context.Background())
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if sweep.Disarmed != 9 || sweep.Ran != 0 {
		t.Fatalf("sweep = %+v, want 9 disarmed (3 legs x 3 paid lanes) and 0 ran", sweep)
	}
	// One named reason per LEG KIND, deduped across lanes — a third lane adds
	// legs, never reasons.
	if len(sweep.Reasons) != 3 {
		t.Fatalf("reasons = %v, want one per leg kind", sweep.Reasons)
	}
}

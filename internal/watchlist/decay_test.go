package watchlist_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/watchlist"
)

// deadSource is an origin that always fails, so a poll always feeds the streak.
func deadSource(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestDeadWatchAnnouncesItself (R18/P-T11-3): a fetch-failure streak reaching
// ⚙ watchlist.fetch_fail_streak raises a card and advances the ladder. Below the
// threshold nothing is raised — the streak is what announces, not one failure.
func TestDeadWatchAnnouncesItself(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	srv := deadSource(t)
	h.seedOne(t, feedRow("dead", srv.URL))

	clk := &clock{t: mustTime(t, "2026-07-01T00:00:00Z")}
	x := h.exec(clk, watchlist.Deps{HTTPClient: srv.Client()})

	// The declared default is 3 cycles: two failures must raise nothing. Each
	// pass advances past the row's continuous cadence — a failing source is
	// retried on cadence, never in a hot loop.
	for i := 0; i < 2; i++ {
		if _, err := x.RunDue(ctx); err != nil {
			t.Fatal(err)
		}
		clk.advance(time.Hour)
	}
	if got := len(h.findings(t)); got != 0 {
		t.Fatalf("%d findings after 2 failures — the streak must reach the ⚙ threshold first", got)
	}
	if r := h.row(t, "dead"); r.FailStreak != 2 {
		t.Fatalf("fail streak = %d, want 2", r.FailStreak)
	}

	if _, err := x.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	f := h.findings(t)
	if len(f) != 1 {
		t.Fatalf("%d findings after the streak reached the threshold, want 1 — a dead watch MUST announce itself", len(f))
	}
	if f[0].ChangeClass != watchlist.ClassEndpoints {
		t.Errorf("a dead-watch finding class = %q, want %q", f[0].ChangeClass, watchlist.ClassEndpoints)
	}
	if !strings.Contains(f[0].Detail, "watchlist.fetch_fail_streak") {
		t.Error("the card does not name the ⚙ key that fired it")
	}
	// A feed row has no browser rung: it goes straight to DECAYED.
	if r := h.row(t, "dead"); r.DecayStage != watchlist.DecayDecayed {
		t.Errorf("feed row decay stage = %d, want %d (no browser rung exists for a feed)", r.DecayStage, watchlist.DecayDecayed)
	}
	if !strings.Contains(f[0].Detail, "propose an alternative source") {
		t.Error("the decayed card does not propose an alternative source")
	}
}

// TestPageRowEscalatesBeforeDecaying (R18): the page tier has the extra rung —
// plain → browser fetcher → decayed — and the card states plainly that the
// browser leg is the ORGAN's fact, not a claim Sinet makes.
func TestPageRowEscalatesBeforeDecaying(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	stub := newOrganStub()
	organ := stub.serve(t)
	h.seedOne(t, pageRow("p", "https://example.invalid/pricing"))

	clk := &clock{t: mustTime(t, "2026-07-01T00:00:00Z")}
	// A healthy pass reconciles the row onto the organ and records its uuid.
	x := h.exec(clk, watchlist.Deps{CDIO: watchlist.NewCDIO(organ.URL, stubAPIKey, organ.Client())})
	if _, err := x.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	// The organ then goes away: every subsequent poll of the page row fails.
	broken := h.exec(clk, watchlist.Deps{CDIO: watchlist.NewCDIO(organ.URL+"/wrong-base", stubAPIKey, organ.Client())})
	for i := 0; i < 3; i++ {
		clk.advance(8 * 24 * time.Hour) // past the row's weekly cadence
		if _, err := broken.RunDue(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if r := h.row(t, "p"); r.DecayStage != watchlist.DecayEscalated {
		t.Fatalf("page row decay stage = %d, want %d (escalate to the browser fetcher first)", r.DecayStage, watchlist.DecayEscalated)
	}
	f := h.findings(t)
	if len(f) == 0 {
		t.Fatal("the escalation raised no card")
	}
	last := f[len(f)-1]
	if !strings.Contains(last.Detail, "PLAYWRIGHT_DRIVER_URL") {
		t.Error("the escalation card does not state that the browser leg is the organ's external CDP service")
	}
	if !last.MigrateBias {
		t.Error("the card does not carry the P-T11-3 migrate bias of a scrape target")
	}
}

// TestFetchFailStreakIsReadLiveByDottedKey is acceptance item 19: the ⚙ value is
// read at evaluation time, so an operator change applies mid-run without a
// restart. Proven by moving the value between passes.
func TestFetchFailStreakIsReadLiveByDottedKey(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	srv := deadSource(t)
	h.seedOne(t, feedRow("dead", srv.URL))
	clk := &clock{t: mustTime(t, "2026-07-01T00:00:00Z")}
	x := h.exec(clk, watchlist.Deps{HTTPClient: srv.Client()})

	// Raise the threshold to its clamp ceiling: four failures still raise nothing.
	if err := h.reg.Set(ctx, settings.SetRequest{
		Key: "watchlist.fetch_fail_streak", Value: json.RawMessage(`10`),
		Actor: settings.Actor{Kind: "operator", ID: "op"}, Reason: "raise the streak threshold",
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if _, err := x.RunDue(ctx); err != nil {
			t.Fatal(err)
		}
		clk.advance(time.Hour)
	}
	if got := len(h.findings(t)); got != 0 {
		t.Fatalf("%d findings at threshold 10 after 4 failures", got)
	}

	// Lower it MID-RUN to the clamp floor: the very next pass announces.
	if err := h.reg.Set(ctx, settings.SetRequest{
		Key: "watchlist.fetch_fail_streak", Value: json.RawMessage(`2`),
		Actor: settings.Actor{Kind: "operator", ID: "op"}, Reason: "lower the streak threshold mid-run",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := x.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if got := len(h.findings(t)); got != 1 {
		t.Errorf("%d findings after lowering the ⚙ mid-run, want 1 — the value is not read live", got)
	}
}

// TestNoSettingsLiteralAppearsInCode is acceptance item 19's negative: no
// numeric literal of a consumed ⚙ default may appear in the package source.
func TestNoSettingsLiteralAppearsInCode(t *testing.T) {
	src := readPackageSource(t)
	for key, literal := range map[string]string{
		"watchlist.fetch_fail_streak":    "fetch_fail_streak = 3",
		"adoption.price_data_stale_days": "price_data_stale_days = 60",
	} {
		if strings.Contains(src, literal) {
			t.Errorf("⚙ %s appears as a hardcoded constant", key)
		}
	}
	for _, key := range []string{"watchlist.fetch_fail_streak", "adoption.price_data_stale_days"} {
		if !strings.Contains(src, key) {
			t.Errorf("⚙ %s is not read by dotted key anywhere", key)
		}
	}
	// The canary interval keys are the sibling API-canary layer's (B5-6B) and
	// are now consumed in this package too — by dotted key, live, never as a
	// literal. Their DEFAULTS must still never appear as constants.
	for key, literal := range map[string]string{
		"canary.auth_interval":       "auth_interval = 86400",
		"canary.behavioral_interval": "behavioral_interval = 604800",
	} {
		if strings.Contains(src, literal) {
			t.Errorf("⚙ %s appears as a hardcoded constant", key)
		}
		if !strings.Contains(src, `"`+key+`"`) {
			t.Errorf("⚙ %s is not read by dotted key anywhere", key)
		}
	}
}

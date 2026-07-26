package watchlist_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchlist"
)

// executor_test.go — the D4 first-boot flood locks.

// TestFeedFirstObservationRaisesNoCards is the D4 lock: a fresh boot must not
// flood the inbox with every entry of every seeded feed (each of which would
// also burn a local classify call). The first observation establishes the
// baseline silently — a feed's existing window predates the watch, so there was
// no prior state for it to have drifted from — and only genuinely NEW entries
// afterwards raise cards. This is the same rule the API tier already applied.
func TestFeedFirstObservationRaisesNoCards(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	body := atomBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	h.seedOne(t, feedRow("f1", srv.URL))
	clk := &clock{t: mustTime(t, "2026-07-01T00:00:00Z")}
	x := h.exec(clk, watchlist.Deps{HTTPClient: srv.Client()})

	pass, err := x.RunDue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pass.Hits != 0 {
		t.Fatalf("the first observation raised %d cards, want 0 — a fresh boot must not flood the inbox", pass.Hits)
	}
	if n := len(h.findings(t)); n != 0 {
		t.Fatalf("%d findings appended on first observation, want 0", n)
	}
	// The baseline was still recorded, so the pre-existing entries can never
	// re-raise later.
	if got := h.row(t, "f1"); len(got.SeenEntries) != 2 {
		t.Fatalf("the baseline recorded %d entry hashes, want 2", len(got.SeenEntries))
	}

	// An unchanged refetch stays quiet.
	clk.advance(time.Hour)
	if pass, err = x.RunDue(ctx); err != nil || pass.Hits != 0 {
		t.Fatalf("an unchanged refetch raised %d cards (err %v)", pass.Hits, err)
	}

	// A genuinely new entry DOES raise exactly one card.
	body = strings.Replace(atomBody, "<entry>", `<entry>
    <id>tag:github.com,2008:Repository/1/v2.9.0</id>
    <title>v2.9.0</title>
    <updated>2026-07-26T09:00:00Z</updated>
    <link rel="alternate" href="https://example.invalid/releases/v2.9.0"/>
  </entry>
  <entry>`, 1)
	clk.advance(time.Hour)
	pass, err = x.RunDue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pass.Hits != 1 {
		t.Fatalf("a new entry raised %d cards, want exactly 1", pass.Hits)
	}
}

// TestWholeSeedSetDoesNotFloodOnFirstBoot is the D4 scale check: every pollable
// seeded row is driven against a stub on its FIRST observation at once — the
// production first-boot shape — and the inbox must stay empty.
func TestWholeSeedSetDoesNotFloodOnFirstBoot(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "json") {
			_, _ = w.Write([]byte(`{"models":{"a":{"ctx":1}}}`))
			return
		}
		_, _ = w.Write([]byte(atomBody))
	}))
	defer srv.Close()

	// Re-point every pollable seeded row at the stub; nothing dials outward.
	var rows []watchlist.Row
	for _, r := range watchlist.SeedRows() {
		if r.Kind == watchlist.KindFeed {
			r.URL = srv.URL + "/feed.atom"
			r.ParserHint = "atom"
			rows = append(rows, r)
		}
	}
	if len(rows) < 8 {
		t.Fatalf("only %d feed rows in the seed set — the flood check would be weak", len(rows))
	}
	if _, err := h.st.EnsureSeeded(ctx, rows); err != nil {
		t.Fatal(err)
	}
	clk := &clock{t: mustTime(t, "2026-07-01T00:00:00Z")}
	x := h.exec(clk, watchlist.Deps{HTTPClient: srv.Client()})
	pass, err := x.RunDue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pass.Polled != len(rows) {
		t.Errorf("polled %d rows, want %d", pass.Polled, len(rows))
	}
	if pass.Hits != 0 || len(h.findings(t)) != 0 {
		t.Errorf("first boot over %d feeds raised %d cards — the inbox must stay empty", len(rows), pass.Hits)
	}
}

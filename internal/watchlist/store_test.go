package watchlist_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchlist"
)

// TestMigrationWallsAreTriggers is the raw-SQL attempted-violation battery (the
// §22/§23/§32 pattern): the structure-immutable and no-delete walls must hold
// even against a caller with a direct SQL handle, not merely against the Go API.
func TestMigrationWallsAreTriggers(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seedOne(t, feedRow("w1", "https://example.invalid/feed.atom"))

	structural := []struct {
		name string
		stmt string
		args []any
	}{
		{"kind", `UPDATE watch_rows SET kind = 'page' WHERE row_id = ?`, []any{"w1"}},
		{"url", `UPDATE watch_rows SET url_or_feed = 'https://elsewhere.invalid/' WHERE row_id = ?`, []any{"w1"}},
		{"tier", `UPDATE watch_rows SET tier = 4 WHERE row_id = ?`, []any{"w1"}},
		{"cadence", `UPDATE watch_rows SET cadence = 'weekly' WHERE row_id = ?`, []any{"w1"}},
		{"enabled", `UPDATE watch_rows SET enabled = 0 WHERE row_id = ?`, []any{"w1"}},
		{"notes", `UPDATE watch_rows SET notes = 'rewritten' WHERE row_id = ?`, []any{"w1"}},
		{"row_id", `UPDATE watch_rows SET row_id = 'w2' WHERE row_id = ?`, []any{"w1"}},
		{"user_id", `UPDATE watch_rows SET user_id = 'alice' WHERE row_id = ?`, []any{"w1"}},
		{"migrate_bias", `UPDATE watch_rows SET migrate_bias = 1 WHERE row_id = ?`, []any{"w1"}},
	}
	for _, c := range structural {
		if err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, c.stmt, c.args...)
			return err
		}); err == nil {
			t.Errorf("structural column %s was mutable — the immutability trigger did not fire", c.name)
		}
	}

	// The fetch-state set stays mutable — the trigger must not over-fire.
	if err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE watch_rows SET last_etag = 'W/"x"', fail_streak = 2, decay_stage = 1 WHERE row_id = ?`, "w1")
		return err
	}); err != nil {
		t.Errorf("fetch-state update was refused — the trigger over-fires: %v", err)
	}

	if err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM watch_rows WHERE row_id = ?`, "w1")
		return err
	}); err == nil {
		t.Error("a watch row was deleted — removal is a dated funeral edit (S16.2), never a delete")
	}

	// Platform-scope by CHECK: the rows ride the S02.9 durable set and every
	// S13.10 archive by construction.
	if err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO watch_rows (row_id, kind, url_or_feed, tier, cadence, user_id)
			 VALUES ('w9','feed','https://x.invalid/f',2,'weekly','alice')`)
		return err
	}); err == nil {
		t.Error("a non-platform watch row was accepted — user_id is CHECK-pinned to 'platform'")
	}
}

// TestSeedIsIdempotent: re-seeding never rewrites a committed row (INSERT OR
// IGNORE) and never trips the immutability trigger.
func TestSeedIsIdempotent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	rows := watchlist.SeedRows()
	n1, err := h.st.EnsureSeeded(ctx, rows)
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if _, err := h.st.EnsureSeeded(ctx, rows); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	all, err := h.st.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != n1 {
		t.Errorf("after re-seed: %d rows in store, want %d", len(all), n1)
	}
}

// TestDuenessDerivesFromStoredState is acceptance item 20: the loop resumes
// correctly across a simulated RESTART (a fresh Store over the same DB, with no
// in-memory state at all) and across a wall-clock JUMP. No time.Ticker drives
// cadence — the clock seam is the only input.
func TestDuenessDerivesFromStoredState(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seedOne(t, watchlist.Row{
		ID: "weekly-row", Kind: watchlist.KindAPI, URL: "https://example.invalid/api.json",
		ParserHint: "json", Tier: 3, Cadence: watchlist.CadenceWeekly, Enabled: true,
		Group: watchlist.GroupReport02, Notes: "test row",
	})

	base := mustTime(t, "2026-07-01T00:00:00Z")
	h.st.Now = func() time.Time { return base }
	due, err := h.st.Due(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 {
		t.Fatalf("a never-fetched row must be due, got %d", len(due))
	}

	st := due[0].FetchState
	st.LastFetchAt = base
	if err := h.st.RecordSuccess(ctx, "weekly-row", st); err != nil {
		t.Fatal(err)
	}

	// A RESTART: a brand-new Store over the same DB, no carried state.
	fresh := watchlist.NewStore(h.db)
	fresh.Now = func() time.Time { return base.Add(6 * 24 * time.Hour) }
	if due, err := fresh.Due(ctx); err != nil || len(due) != 0 {
		t.Errorf("6 days after a weekly poll the row must not be due (got %d, err %v) — dueness survived the restart only if it read stored state", len(due), err)
	}

	// A wall-clock JUMP past the cadence (a suspend/resume): the row is due
	// immediately, not one interval after the process woke.
	fresh.Now = func() time.Time { return base.Add(40 * 24 * time.Hour) }
	if due, err := fresh.Due(ctx); err != nil || len(due) != 1 {
		t.Errorf("after a clock jump past the cadence the row must be due (got %d, err %v)", len(due), err)
	}
}

// TestStudyRowsAreNeverPolledButAreReviewable: a study row is a standing review
// obligation, not a fetch. It never enters the poll set, and it surfaces through
// the S16.7 read surface instead (R21).
func TestStudyRowsAreNeverPolledButAreReviewable(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seedOne(t, watchlist.Row{
		ID: "s1", Kind: watchlist.KindStudy, Tier: 3, Cadence: watchlist.CadenceQuarterly,
		Enabled: true, Group: watchlist.GroupLockReview, Notes: "test row",
	})
	if due, err := h.st.Due(ctx); err != nil || len(due) != 0 {
		t.Errorf("a study row must never be polled (got %d, err %v)", len(due), err)
	}
	pending, err := h.st.PendingReview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != "s1" {
		t.Errorf("PendingReview = %+v, want the study row", pending)
	}
}

// TestSeenEntriesRoundTrip: the per-entry hash set persists across a reload and
// stays bounded.
func TestSeenEntriesRoundTrip(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seedOne(t, feedRow("f1", "https://example.invalid/feed.atom"))
	st := h.row(t, "f1").FetchState
	st.LastFetchAt = time.Now()
	st.SeenEntries = []string{"aaaa", "bbbb", "cccc"}
	if err := h.st.RecordSuccess(ctx, "f1", st); err != nil {
		t.Fatal(err)
	}
	got := h.row(t, "f1")
	if strings.Join(got.SeenEntries, ",") != "aaaa,bbbb,cccc" {
		t.Errorf("seen entries round-trip = %v", got.SeenEntries)
	}
}

// TestUnknownRowIsRefused: every mutating verb refuses an unknown id loudly.
func TestUnknownRowIsRefused(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.st.SetExternalRef(ctx, "nope", "uuid"); err == nil {
		t.Error("SetExternalRef accepted an unknown row")
	}
	if _, err := h.st.RecordFailure(ctx, "nope", time.Now()); err == nil {
		t.Error("RecordFailure accepted an unknown row")
	}
	if _, err := h.st.Row(ctx, "nope"); err == nil {
		t.Error("Row accepted an unknown id")
	}
}

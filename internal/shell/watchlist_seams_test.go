package shell

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchdog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchlist"
)

// watchlist_seams_test.go — the B5-6A composition-root coverage. Hermetic: the
// organ is deliberately UNCONFIGURED (its host install is a B5-gate act), so no
// socket is opened and nothing is dialled.

// testLogger discards output: these suites assert composition, not log text.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func watchlistTestDeps(t *testing.T) (*storage.DB, *eventlog.Log, *settings.Registry) {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	if err := reg.Attach(ctx, db, log); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if err := db.ReapplySettings(ctx); err != nil {
		t.Fatalf("ReapplySettings: %v", err)
	}
	return db, log, reg
}

// TestBuildWatchlistSurfaceSeedsAndDegradesHonestly: the surface composes with
// every optional seam absent — no local tier, no advisory meter, no runbook, no
// organ — seeds its rows unconditionally, and reports the page tier as absent.
func TestBuildWatchlistSurfaceSeedsAndDegradesHonestly(t *testing.T) {
	ctx := context.Background()
	db, log, reg := watchlistTestDeps(t)
	t.Setenv(watchlist.CDIOURLEnv, "")

	wl, err := buildWatchlistSurface(ctx, db, log, reg, nil, nil, nil, testLogger())
	if err != nil {
		t.Fatalf("buildWatchlistSurface: %v", err)
	}
	if wl.Rows != len(watchlist.SeedRows()) {
		t.Errorf("seeded %d rows, want %d", wl.Rows, len(watchlist.SeedRows()))
	}
	if wl.PageTier {
		t.Error("the page tier reported configured with no " + watchlist.CDIOURLEnv)
	}

	// Idempotent across a second boot.
	again, err := buildWatchlistSurface(ctx, db, log, reg, nil, nil, nil, testLogger())
	if err != nil {
		t.Fatalf("second boot: %v", err)
	}
	rows, err := again.Store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != wl.Rows {
		t.Errorf("after a second boot the store holds %d rows, want %d", len(rows), wl.Rows)
	}

	// The organ-liveness seam is wired and honestly reports DOWN.
	organs := watchlistOrgans(wl)
	if organs == nil {
		t.Fatal("the organ-liveness seam is nil — it was nil until this packet and must now be wired")
	}
	st, err := organs(ctx)
	if err != nil {
		t.Fatalf("organ probe: %v", err)
	}
	if len(st) != 1 || st[0].Organ != watchlist.OrganName {
		t.Fatalf("organ statuses = %+v, want one %q entry", st, watchlist.OrganName)
	}
	if st[0].Up || st[0].Note == "" {
		t.Errorf("an unconfigured organ must be DOWN with a reason: %+v", st[0])
	}
}

// TestWatchlistOrgansIsNilWithoutASurface: a nil surface leaves the watchdog
// seam nil, so the check stays honestly absent rather than reporting a fake
// organ.
func TestWatchlistOrgansIsNilWithoutASurface(t *testing.T) {
	if watchlistOrgans(nil) != nil {
		t.Error("watchlistOrgans(nil) must be nil")
	}
}

// TestWatchlistRevalidateHookIsNilWithoutARunbook: with no B5-5 runbook composed
// the seam is nil, and the emitter records that the edge is unwired rather than
// implying it ran.
func TestWatchlistRevalidateHookIsNilWithoutARunbook(t *testing.T) {
	if watchlistRevalidateHook(nil) != nil {
		t.Error("watchlistRevalidateHook(nil) must be nil")
	}
}

// TestAbsentOrganRaisesTheOrganAbsenceFlag is acceptance item 15 end to end: the
// watchlist's liveness probe, wired into the watchdog's previously-nil organ
// seam, turns an absent changedetection.io into a
// `watchdog.organ_absence:watchlist` degraded digest flag.
func TestAbsentOrganRaisesTheOrganAbsenceFlag(t *testing.T) {
	ctx := context.Background()
	db, log, reg := watchlistTestDeps(t)
	t.Setenv(watchlist.CDIOURLEnv, "")

	wl, err := buildWatchlistSurface(ctx, db, log, reg, nil, nil, nil, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	wd := watchdog.New(watchdog.Deps{
		DB: db, Log: log, Runs: run.NewStore(db, log), Settings: reg,
		Organs: watchlistOrgans(wl), Logger: testLogger(),
	})
	if err := wd.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	rows, err := db.QueryContext(ctx,
		`SELECT payload FROM run_events WHERE type = 'watchdog.flagged' AND run_id IS NULL`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var found bool
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		var p struct {
			AnomalyClass string `json:"anomaly_class"`
			Severity     string `json:"severity"`
			Detail       string `json:"detail"`
		}
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			t.Fatal(err)
		}
		if p.AnomalyClass != "watchdog.organ_absence:"+watchlist.OrganName {
			continue
		}
		found = true
		if p.Severity != watchdog.SeverityDailyDigest {
			t.Errorf("an absent organ is DEGRADED, not an emergency: severity = %q, want %q", p.Severity, watchdog.SeverityDailyDigest)
		}
		if !strings.Contains(p.Detail, watchlist.CDIOURLEnv) {
			t.Errorf("the flag does not say WHY the organ is absent: %q", p.Detail)
		}
	}
	if !found {
		t.Fatal("no watchdog.organ_absence:watchlist flag — the organ seam was nil until this packet and must now fire")
	}
}

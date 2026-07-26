package shell

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/evals"
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

// TestRevalidateHookDrivesTheRealRunbookEndToEnd is the D6 lock, and it guards
// the exact five lines the packet's import-wall deviation rests on: the hook is
// the ONLY thing standing between a watchlist drift finding and
// worker.Store.FlagByModel, and testing it for nil alone would let a
// Subject/Reason swap — or a wrong TriggerKind — pass the whole suite.
//
// It composes a REAL *evals.Runbook over a spy hook and drives a models-class
// finding through the real emitter, asserting the model id and the drift
// provenance arrive intact.
func TestRevalidateHookDrivesTheRealRunbookEndToEnd(t *testing.T) {
	ctx := context.Background()
	db, log, _ := watchlistTestDeps(t)

	var gotModel, gotReason string
	var calls int
	rb := &evals.Runbook{
		Hooks: evals.Hooks{
			FlagByModel: func(ctx context.Context, model, reason string) ([]string, error) {
				calls++
				gotModel, gotReason = model, reason
				return []string{"tmpl-alpha", "tmpl-beta"}, nil
			},
			// If the hook ever built the WRONG trigger kind, one of these would
			// fire instead of FlagByModel and the assertions below would catch it.
			FlagByEnginePin: func(ctx context.Context, pin, reason string) ([]string, error) {
				t.Errorf("a drift finding reached FlagByEnginePin — the trigger kind is wrong")
				return nil, nil
			},
		},
	}

	em := watchlist.NewEmitter(db, log)
	em.Revalidate = watchlistRevalidateHook(rb)
	em.Logger = testLogger()

	got, emitted, err := em.Emit(ctx, watchlist.Hit{
		RowID: "t3-modelsdev-api", Kind: watchlist.KindAPI,
		Source: "https://example.invalid/api.json", Lane: "anthropic",
		Class: watchlist.ClassModels, Subject: "claude-haiku-4-5",
		Summary: "the observed model list moved",
	})
	if err != nil || !emitted {
		t.Fatalf("emit: %v (emitted=%v)", err, emitted)
	}
	if calls != 1 {
		t.Fatalf("FlagByModel called %d times, want exactly 1", calls)
	}
	if gotModel != "claude-haiku-4-5" {
		t.Errorf("FlagByModel received subject %q — the model id must arrive intact, not the reason or the summary", gotModel)
	}
	if !strings.Contains(gotReason, "drift") {
		t.Errorf("FlagByModel received reason %q — it must carry the drift provenance", gotReason)
	}
	if gotReason == gotModel {
		t.Error("subject and reason are the same value — they are swapped or duplicated")
	}
	if !got.Revalidation.Triggered {
		t.Error("the card does not record the revalidation as triggered")
	}
	if len(got.Revalidation.Flagged) != 2 {
		t.Errorf("the flagged set was not carried onto the card: %+v", got.Revalidation.Flagged)
	}

	// And the negative through the SAME composed seam: a non-model class must
	// not reach the runbook at all.
	calls = 0
	if _, _, err := em.Emit(ctx, watchlist.Hit{
		RowID: "t1-anthropic-pricing", Kind: watchlist.KindPage,
		Source: "https://example.invalid/pricing", Lane: "anthropic",
		Class: watchlist.ClassPrice, Subject: "claude-haiku-4-5", Summary: "price moved",
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Errorf("a price-class finding called FlagByModel %d times — only `models` may (OQ4(a))", calls)
	}
}

// TestWatchRowOverrideIsReachableAtBoot is the D9 lock: R3's operator override
// was machinery with no door — LoadRows/WriteSeed had no production consumer at
// all. It is now loaded at composition, additively, and a malformed file fails
// the boot LOUDLY rather than silently degrading to the in-code set.
func TestWatchRowOverrideIsReachableAtBoot(t *testing.T) {
	ctx := context.Background()

	t.Run("absent override is silent", func(t *testing.T) {
		db, log, reg := watchlistTestDeps(t)
		t.Setenv(watchlist.WatchRowsOverrideEnv, filepath.Join(t.TempDir(), "nope.json"))
		wl, err := buildWatchlistSurface(ctx, db, log, reg, nil, nil, nil, testLogger())
		if err != nil {
			t.Fatalf("an absent override must be silent, got: %v", err)
		}
		if wl.Rows != len(watchlist.SeedRows()) {
			t.Errorf("seeded %d rows, want the in-code set of %d", wl.Rows, len(watchlist.SeedRows()))
		}
	})

	t.Run("present override adds rows", func(t *testing.T) {
		db, log, reg := watchlistTestDeps(t)
		path := filepath.Join(t.TempDir(), "watch-rows.json")
		body := `[{"id":"op-extra-feed","kind":"feed","url":"https://ops.invalid/f.atom",
		  "parser_hint":"atom","tier":2,"cadence":"continuous","enabled":true,
		  "notes":"operator-added row proving the override reaches boot"}]`
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(watchlist.WatchRowsOverrideEnv, path)

		wl, err := buildWatchlistSurface(ctx, db, log, reg, nil, nil, nil, testLogger())
		if err != nil {
			t.Fatalf("buildWatchlistSurface: %v", err)
		}
		if wl.Rows != len(watchlist.SeedRows())+1 {
			t.Errorf("seeded %d rows, want the in-code set plus the override row", wl.Rows)
		}
		if _, err := wl.Store.Row(ctx, "op-extra-feed"); err != nil {
			t.Errorf("the operator's row did not reach the store: %v", err)
		}
		// The override is ADDITIVE: a standing obligation cannot vanish by
		// omission from the operator's file.
		if _, err := wl.Store.Row(ctx, "s168-awesome-harness-engineering"); err != nil {
			t.Errorf("an S16.8 standing row vanished under an override: %v", err)
		}
	})

	t.Run("malformed override fails the boot loudly", func(t *testing.T) {
		db, log, reg := watchlistTestDeps(t)
		path := filepath.Join(t.TempDir(), "watch-rows.json")
		if err := os.WriteFile(path, []byte(`[{"id":"x","kind":"feed","url":"https://e.invalid/f","tier":2,"cadence":"weekly","notes":"n","typo":1}]`), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(watchlist.WatchRowsOverrideEnv, path)
		if _, err := buildWatchlistSurface(ctx, db, log, reg, nil, nil, nil, testLogger()); err == nil {
			t.Error("a malformed override was accepted — a typo must fail the boot, never degrade silently")
		}
	})
}

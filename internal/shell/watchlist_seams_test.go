package shell

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/evals"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/watchdog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/watchlist"
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

	wl, err := buildWatchlistSurface(ctx, db, log, reg, nil, nil, nil, nil, testLogger())
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
	again, err := buildWatchlistSurface(ctx, db, log, reg, nil, nil, nil, nil, testLogger())
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

	wl, err := buildWatchlistSurface(ctx, db, log, reg, nil, nil, nil, nil, testLogger())
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
		wl, err := buildWatchlistSurface(ctx, db, log, reg, nil, nil, nil, nil, testLogger())
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

		wl, err := buildWatchlistSurface(ctx, db, log, reg, nil, nil, nil, nil, testLogger())
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
		if _, err := buildWatchlistSurface(ctx, db, log, reg, nil, nil, nil, nil, testLogger()); err == nil {
			t.Error("a malformed override was accepted — a typo must fail the boot, never degrade silently")
		}
	})
}

// ── B5-6B: the API canary layer's composition-root posture ─────────────────

// TestCanaryLayerShipsDisarmed is the $0 proof at the composition root: with no
// operator arm, the three legs that would dial a provider are nil, so the layer
// physically cannot spend. It is not a policy comment — it is the wiring.
func TestCanaryLayerShipsDisarmed(t *testing.T) {
	ctx := context.Background()
	db, log, reg := watchlistTestDeps(t)
	t.Setenv(watchlist.CDIOURLEnv, "")
	t.Setenv(watchlist.CanaryArmEnv, "")

	wl, err := buildWatchlistSurface(ctx, db, log, reg, nil, nil, nil, nil, testLogger())
	if err != nil {
		t.Fatalf("buildWatchlistSurface: %v", err)
	}
	if wl.CanaryArmed {
		t.Fatal("the canary legs composed ARMED with no operator act")
	}
	if wl.Canaries == nil {
		t.Fatal("the canary layer was not composed at all")
	}
	// Each real-request leg is CONSTRUCTED but holds NO probe: constructed so
	// the sweep schedules, counts and explains it (drain D1); probe-less so it
	// physically cannot spend.
	if wl.Canaries.Auth == nil || wl.Canaries.ModelList == nil || wl.Canaries.Behavioral == nil {
		t.Fatal("a real-request leg is nil — a nil leg is silently skipped and vanishes from the sweep accounting")
	}
	for name, hasProbe := range map[string]bool{
		"auth":       wl.Canaries.Auth.Probe != nil,
		"model-list": wl.Canaries.ModelList.Probe != nil,
		"behavioral": wl.Canaries.Behavioral.Run != nil,
	} {
		if hasProbe {
			t.Errorf("the %s canary leg holds a probe while disarmed — it could issue a real request", name)
		}
	}
	for name, reason := range map[string]string{
		"auth":       wl.Canaries.Auth.Unavailable,
		"model-list": wl.Canaries.ModelList.Unavailable,
		"behavioral": wl.Canaries.Behavioral.Unavailable,
	} {
		if !strings.Contains(reason, watchlist.CanaryArmEnv) {
			t.Errorf("the %s leg's disarmed reason %q does not name the arm env", name, reason)
		}
	}

	// A disarmed sweep RECORDS nothing and SPENDS nothing, and every leg it
	// could not run is counted with its reason.
	sweep, err := wl.Canaries.RunDue(ctx)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if sweep.Ran != 0 {
		t.Errorf("a disarmed layer ran %d canaries, want 0", sweep.Ran)
	}
	if sweep.Disarmed != 6 {
		t.Errorf("disarmed count = %d, want 6 (3 legs × 2 paid lanes) — a skipped leg must still be accounted for", sweep.Disarmed)
	}
	if len(sweep.Reasons) != 3 {
		t.Errorf("sweep reasons = %v, want one per leg kind", sweep.Reasons)
	}
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_events WHERE type = ?`, watchlist.EventCanaryResult).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("a disarmed layer wrote %d canary.result rows, want 0", n)
	}
}

// TestArmingWiresNoLegWithoutItsDependency: arming alone does not conjure a
// runner. With no promptfoo binary the behavioral leg stays absent — the
// carried B5-5 dependency is honoured, not faked.
func TestArmingWiresNoLegWithoutItsDependency(t *testing.T) {
	ctx := context.Background()
	db, log, reg := watchlistTestDeps(t)
	t.Setenv(watchlist.CDIOURLEnv, "")
	t.Setenv(watchlist.CanaryArmEnv, "1")
	// Point discovery at a path that does not exist, so the resolution fails
	// deterministically whether or not the host happens to have the binary.
	t.Setenv(evals.PromptfooPathEnv, filepath.Join(t.TempDir(), "no-such-promptfoo"))

	wl, err := buildWatchlistSurface(ctx, db, log, reg, nil, nil, nil, nil, testLogger())
	if err != nil {
		t.Fatalf("buildWatchlistSurface: %v", err)
	}
	if !wl.CanaryArmed {
		t.Fatal("the arm env did not arm the legs")
	}
	if wl.Canaries.Behavioral == nil {
		t.Fatal("the behavioral leg is nil — it must exist to account for itself")
	}
	if wl.Canaries.Behavioral.Run != nil {
		t.Error("the behavioral leg holds a runner without a pinned binary — SANCTIONED SKIP (CONVENTIONS §10) is the honest state until the B5-gate install")
	}
	if !strings.Contains(wl.Canaries.Behavioral.Unavailable, "B5-gate") {
		t.Errorf("the behavioral leg's reason %q does not name the carried B5-5 install dependency", wl.Canaries.Behavioral.Unavailable)
	}

	// Arming is necessary but NOT sufficient: on this tree the auth and
	// model-list legs still need gate-time endpoints and a broker credential
	// accessor, and they say so rather than pretending to be armed.
	for name, leg := range map[string]string{
		"auth":       wl.Canaries.Auth.Unavailable,
		"model-list": wl.Canaries.ModelList.Unavailable,
	} {
		if wl.Canaries.Auth.Probe != nil || wl.Canaries.ModelList.Probe != nil {
			t.Errorf("the %s leg composed a probe with no endpoint or credential source in the tree", name)
		}
		if !strings.Contains(leg, "B5-gate install material") {
			t.Errorf("the %s leg's armed reason %q does not name what it is still waiting for", name, leg)
		}
	}

	// Armed but uncomposed legs are still COUNTED, so the gap is visible in
	// every sweep line rather than only in a doc.
	sweep, err := wl.Canaries.RunDue(ctx)
	if err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	if sweep.Disarmed != 6 {
		t.Errorf("armed-but-uncomposed disarmed count = %d, want 6", sweep.Disarmed)
	}
}

// TestBehavioralCanaryHookReusesThePinnedRunnerSeam: the hook consumes the
// S14.8 Runner interface and the committed probe battery; it constructs no
// second runner and invents no eval case content.
func TestBehavioralCanaryHookReusesThePinnedRunnerSeam(t *testing.T) {
	spy := &spyRunner{}
	hook := behavioralCanaryHook(spy)
	out, err := hook(context.Background(), "anthropic")
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	if spy.cfg.Provider != "anthropic" {
		t.Errorf("provider = %q, want the lane", spy.cfg.Provider)
	}
	suite := evals.SeedProbeSuite()
	if spy.cfg.Suite != suite.Version || len(spy.cfg.Cases) != len(suite.Tasks) {
		t.Errorf("the hook ran %q with %d cases, want the committed %q battery (%d tasks)",
			spy.cfg.Suite, len(spy.cfg.Cases), suite.Version, len(suite.Tasks))
	}
	if out.Runner != "promptfoo" || out.RunnerVersion != "0.121.19" {
		t.Errorf("outcome identity = %q %q, want the runner's own reported identity", out.Runner, out.RunnerVersion)
	}
	if out.PassRate != 0.5 || out.Cases != 2 {
		t.Errorf("outcome = %+v, want the runner's parsed pass rate", out)
	}
}

// spyRunner is an evals.Runner that records its config and returns a fixed
// outcome. It runs no process and dials nothing.
type spyRunner struct{ cfg evals.RunConfig }

func (s *spyRunner) Identity(ctx context.Context) (string, string, error) {
	return "promptfoo", "0.121.19", nil
}

func (s *spyRunner) Run(ctx context.Context, cfg evals.RunConfig) (evals.RunOutcome, error) {
	s.cfg = cfg
	return evals.RunOutcome{
		Suite: cfg.Suite, Provider: cfg.Provider,
		Cases: []evals.CaseOutcome{{ID: "a", Pass: true}, {ID: "b", Pass: false}},
	}, nil
}

// TestBehavioralCanaryRealRunnerLeg is the CONVENTIONS §10 tier-R leg for the
// B5-6B behavioral canary: it auto-runs when the pinned runner is installed on
// the host and prints the sanctioned skip otherwise. It asserts only that the
// composed hook resolves a runner identity — it issues NO eval and therefore
// makes NO paid call, because arming the paid leg is an operator act with a
// pre-registered projection, not a test's to take.
//
// This packet installed nothing: `npm install -g promptfoo@<pin>` is a B5-gate
// HOST act inherited from B5-5.
func TestBehavioralCanaryRealRunnerLeg(t *testing.T) {
	t.Setenv(evals.PromptfooPathEnv, "")
	if _, err := exec.LookPath("promptfoo"); err != nil {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): promptfoo is not installed on this host, so the behavioral canary leg cannot be exercised against a real binary")
	}
	runner, err := evals.FindPromptfoo()
	if err != nil {
		t.Fatal(err)
	}
	if hook := behavioralCanaryHook(runner); hook == nil {
		t.Fatal("the behavioral canary hook is nil with a resolved runner")
	}
	name, version, err := runner.Identity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if name == "" {
		t.Error("the runner reported no identity")
	}
	if version != evals.PromptfooPin {
		t.Errorf("installed promptfoo %s != pin %s — a pin↔installed delta is reported LOUDLY and never silently retargeted (CONVENTIONS §10)",
			version, evals.PromptfooPin)
	}
}

package shell

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/evals"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchdog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchlist"
)

// The Spec S14.6 watchlist executor, composed at the root (B5-6A).
//
// internal/watchlist never imports internal/evals: the S14.8 revalidation edge
// rides a func seam satisfied here, exactly as the S12.10 swap gate's
// FlagByModelFunc and the B5-5 eval seams do (CONVENTIONS §28/§33). Composition
// is wiring only — no package gains a forbidden edge.

// watchlistLoopInterval is how often the shell asks the executor which rows are
// DUE. It is not the cadence: cadence lives on each row and dueness derives
// from stored state (`last_fetch_at`), so a restart or a suspend cannot skip or
// double-fire a poll. Structural constant, not ⚙ — S18 ratifies no
// watchlist-cadence key (the sseBatchSize precedent, §7).
const watchlistLoopInterval = time.Minute

// watchlistSurface holds the composed S14.6 machinery.
type watchlistSurface struct {
	Store *watchlist.Store
	Exec  *watchlist.Executor
	// Rows is the size of the seed set applied at this boot.
	Rows int
	// PageTier records whether the changedetection.io organ is configured at
	// all. False is the honest default: the host install is a B5-gate act.
	PageTier bool
}

// buildWatchlistSurface composes the S14.6 executor and seeds the watch rows.
//
// Seeding is unconditional (platform obligations need no D10 holder, the
// conformance-registry posture) and idempotent. The organ is DISCOVERED, never
// installed: with no SINET_CDIO_URL the page tier is simply absent and the feed
// and API tiers keep working.
func buildWatchlistSurface(ctx context.Context, db *storage.DB, log *eventlog.Log,
	reg *settings.Registry, duty *local.Duty, meter stage.AdvisoryMeter,
	runbook *evals.Runbook, logger *slog.Logger) (*watchlistSurface, error) {

	store := watchlist.NewStore(db)
	seed := watchlist.SeedRows()

	// The R3 operator override, reachable at boot. It is ADDITIVE and loaded
	// AFTER the in-code set, so an override can add rows the platform does not
	// ship but can never delete a standing obligation (the S16.8 registrations
	// and the per-lock-entry review rows) by omission. A present-but-malformed
	// override fails the boot LOUDLY rather than degrading to "no rows" — the
	// §14/§15/§17 strict-seed discipline.
	override, found, err := watchlist.LoadRowsFile(os.Getenv(watchlist.WatchRowsOverrideEnv))
	if err != nil {
		return nil, fmt.Errorf("shell: load S14.6 watch-row override (%s): %w", watchlist.WatchRowsOverrideEnv, err)
	}
	if found {
		seed = append(seed, override...)
		logger.Info("watchlist: operator watch-row override loaded",
			"path", os.Getenv(watchlist.WatchRowsOverrideEnv), "rows", len(override))
	}

	rows, err := store.EnsureSeeded(ctx, seed)
	if err != nil {
		return nil, fmt.Errorf("shell: seed S14.6 watch rows: %w", err)
	}

	emitter := watchlist.NewEmitter(db, log)
	emitter.Classifier = &watchlist.Classifier{
		Duty: duty,
		// A watch hit has no run, so every $0 second-pass call rides the
		// advisory platform-run seam (the B5-3 Tier-1 OQ4 precedent). With no
		// meter the pass is skipped honestly rather than issued unmetered.
		Meter: watchlist.AdvisoryMeter(meter),
	}
	emitter.Revalidate = watchlistRevalidateHook(runbook)
	emitter.Logger = logger

	cdio := watchlist.DiscoverCDIO(nil)
	return &watchlistSurface{
		Store: store,
		Exec: watchlist.New(watchlist.Deps{
			Store: store, Emitter: emitter, Settings: reg, CDIO: cdio, Logger: logger,
		}),
		Rows:     rows,
		PageTier: cdio != nil,
	}, nil
}

// watchlistRevalidateHook satisfies the S14.8 revalidation seam with the B5-5
// runbook. The watchlist calls it ONLY for a models-class finding carrying a
// model-id subject (coordinator disposition OQ4(a)); every other class records
// on its card that no revalidation was triggered. Constructing the
// evals.Trigger here is what keeps internal/watchlist free of an evals import.
func watchlistRevalidateHook(rb *evals.Runbook) watchlist.Revalidate {
	if rb == nil {
		return nil
	}
	return func(ctx context.Context, modelID, reason string) ([]string, error) {
		return rb.Flag(ctx, evals.Trigger{
			Kind: evals.TriggerDrift, Subject: modelID, Reason: reason,
		})
	}
}

// watchlistOrgans satisfies the watchdog's organ-liveness seam (S14.4
// registered check 3), which was nil until this packet. A down organ surfaces
// as a `watchdog.organ_absence:watchlist` degraded digest flag.
func watchlistOrgans(wl *watchlistSurface) watchdog.OrganLiveness {
	if wl == nil {
		return nil
	}
	return func(ctx context.Context) ([]watchdog.OrganStatus, error) {
		up, note := wl.Exec.OrganLiveness(ctx)
		return []watchdog.OrganStatus{{Organ: watchlist.OrganName, Up: up, Note: note}}, nil
	}
}

// watchlistLoop drives the S14.6 executor — the shell owns WHEN (Spec S10.7 /
// CONVENTIONS §7/§8, the WAL/recovery/watchdog-loop precedent). It ticks on a
// structural interval and polls only the rows whose stored cadence has elapsed,
// so the pass survives restart and suspend; NO ticker drives a row's cadence. A
// pass error is logged and the next tick retries — the watchlist never wedges
// the process.
func watchlistLoop(ctx context.Context, wl *watchlistSurface, logger *slog.Logger) {
	t := time.NewTicker(watchlistLoopInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pass, err := wl.Exec.RunDue(ctx)
			if err != nil {
				logger.Warn("watchlist: pass failed", "err", err)
				continue
			}
			if pass.Polled > 0 || pass.Hits > 0 {
				logger.Info("watchlist: pass complete",
					"polled", pass.Polled, "hits", pass.Hits, "failures", pass.Failures,
					"page_tier_absent", pass.PageTierAbsent)
			}
		}
	}
}

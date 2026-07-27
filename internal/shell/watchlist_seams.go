package shell

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/conformance"
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

// canaryLoopInterval is how often the shell asks the canary layer which
// canaries are DUE. Like watchlistLoopInterval it is the TICK, never the
// cadence: each canary's cadence is its ⚙ (or its structural interval) and
// dueness derives from the last canary.result in the log, so a restart or a
// suspend can neither skip nor double-fire a canary. Structural constant, not ⚙.
// It is coarser than the executor's tick because the tightest canary cadence is
// the clamp floor of ⚙ canary.auth_interval (6 h) — a minute-grained tick would
// buy nothing but wake-ups.
const canaryLoopInterval = 15 * time.Minute

// watchlistSurface holds the composed S14.6 machinery.
type watchlistSurface struct {
	Store *watchlist.Store
	Exec  *watchlist.Executor
	// Canaries is the S14.6 ¶3 API canary layer (B5-6B).
	Canaries *watchlist.Canaries
	// Rows is the size of the seed set applied at this boot.
	Rows int
	// PageTier records whether the changedetection.io organ is configured at
	// all. False is the honest default: the host install is a B5-gate act.
	PageTier bool
	// CanaryArmed records whether the operator armed the real-request canary
	// legs. False is the honest default: the packet that built them spent $0.
	CanaryArmed bool
}

// buildWatchlistSurface composes the S14.6 executor and seeds the watch rows.
//
// Seeding is unconditional (platform obligations need no D10 holder, the
// conformance-registry posture) and idempotent. The organ is DISCOVERED, never
// installed: with no SINET_CDIO_URL the page tier is simply absent and the feed
// and API tiers keep working.
func buildWatchlistSurface(ctx context.Context, db *storage.DB, log *eventlog.Log,
	reg *settings.Registry, duty *local.Duty, meter stage.AdvisoryMeter,
	runbook *evals.Runbook, conf *conformance.Store, logger *slog.Logger) (*watchlistSurface, error) {

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
	canaries, armed := buildCanaryLayer(db, log, reg, duty, meter, emitter, conf, logger)
	return &watchlistSurface{
		Store: store,
		Exec: watchlist.New(watchlist.Deps{
			Store: store, Emitter: emitter, Settings: reg, CDIO: cdio, Logger: logger,
		}),
		Canaries:    canaries,
		Rows:        rows,
		PageTier:    cdio != nil,
		CanaryArmed: armed,
	}, nil
}

// buildCanaryLayer composes the S14.6 ¶3 API canary layer (B5-6B).
//
// THE $0 POSTURE IS COMPOSED HERE, not hidden in the package: the three legs
// that dial a provider — auth, behavioral, model-list — are wired ONLY when the
// operator has armed them (watchlist.CanaryArmEnv). Unarmed, they are nil and
// the layer skips them honestly rather than recording a canary that never ran.
// The logprob canary is local-tier and costs no allowance, so it is wired
// whenever the local stack is.
//
// The behavioral leg additionally needs the pinned runner, which is not
// installed on the host until the B5-gate act: with no binary it stays nil and
// the leg is absent for that second, independent reason.
func buildCanaryLayer(db *storage.DB, log *eventlog.Log, reg *settings.Registry,
	duty *local.Duty, meter stage.AdvisoryMeter, emitter *watchlist.Emitter,
	conf *conformance.Store, logger *slog.Logger) (*watchlist.Canaries, bool) {

	c := watchlist.NewCanaries(db, log, reg)
	c.Emitter = emitter
	c.Logger = logger
	if duty != nil {
		c.Logprob = watchlist.NewLogprobCanary(duty, watchlist.AdvisoryMeter(meter))
	}
	c.Conformance = watchlist.NewConformanceCanary(conf)

	armed := watchlist.CanaryArmed()
	if !armed {
		logger.Info("watchlist: API canary layer wired with the real-request legs DISARMED (B5-6B)",
			"arm_with", watchlist.CanaryArmEnv,
			"disarmed", "auth, behavioral, model-list",
			"live", "logprob (local tier, $0), conformance dueness")
		return c, false
	}

	// Armed. The auth and model-list probes need per-lane endpoints and a
	// credential accessor, which the credential broker owns and which settle
	// with the B5-gate install; until they are composed the armed layer runs
	// whatever legs it can and says which it could not.
	if runner, err := evals.FindPromptfoo(); err == nil {
		c.Behavioral = watchlist.NewBehavioralCanary(behavioralCanaryHook(runner))
	} else {
		logger.Warn("watchlist: behavioral canary armed but no pinned runner is installed — the leg stays absent",
			"pin", evals.PromptfooPin, "override", evals.PromptfooPathEnv, "err", err)
	}
	logger.Info("watchlist: API canary layer ARMED (B5-6B)", "behavioral", c.Behavioral != nil)
	return c, true
}

// behavioralCanaryHook satisfies the S14.6 ¶3 behavioral seam with the B5-5
// pinned runner over the committed bump-probe battery. Building the
// evals.RunConfig here is what keeps internal/watchlist free of an evals import
// (the watchlistRevalidateHook precedent above); no second runner is
// constructed and no eval case content is invented — the battery is B5-5's.
func behavioralCanaryHook(runner evals.Runner) watchlist.BehavioralRun {
	if runner == nil {
		return nil
	}
	return func(ctx context.Context, lane string) (watchlist.BehavioralOutcome, error) {
		name, version, err := runner.Identity(ctx)
		if err != nil {
			return watchlist.BehavioralOutcome{}, err
		}
		suite := evals.SeedProbeSuite()
		out, err := runner.Run(ctx, evals.RunConfig{
			Suite:    suite.Version,
			Provider: lane,
			Cases:    suite.Tasks,
		})
		if err != nil {
			return watchlist.BehavioralOutcome{}, err
		}
		return watchlist.BehavioralOutcome{
			Runner: name, RunnerVersion: version, Suite: suite.Version,
			Cases: len(out.Cases), PassRate: out.PassRate(),
		}, nil
	}
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

// canaryLoop drives the S14.6 ¶3 API canary layer — the shell owns WHEN, as it
// does for the executor half. It ticks on a structural interval and runs only
// the canaries whose cadence has elapsed, derived from the last canary.result
// in the log, so the layer survives restart and suspend and no ticker drives a
// canary's cadence. A sweep error is logged and the next tick retries.
func canaryLoop(ctx context.Context, wl *watchlistSurface, logger *slog.Logger) {
	t := time.NewTicker(canaryLoopInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sweep, err := wl.Canaries.RunDue(ctx)
			if err != nil {
				logger.Warn("watchlist: canary sweep failed", "err", err)
				continue
			}
			if sweep.Ran > 0 || sweep.Cards > 0 {
				logger.Info("watchlist: canary sweep complete",
					"ran", sweep.Ran, "failures", sweep.Failures,
					"disarmed", sweep.Disarmed, "conformance_cards", sweep.Cards)
			}
		}
	}
}

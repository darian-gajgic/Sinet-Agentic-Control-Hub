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

// The named reasons a real-request leg is not composed. Every leg that cannot
// run carries one, so a sweep accounts for it out loud instead of skipping a
// nil (drain D1).
const (
	// canaryNoCredentialedEndpoint is the honest state of the auth and
	// model-list legs on THIS tree. Both v0 paid lanes ride wrapped CLIs
	// (claude-cli, opencode) — there is no per-lane HTTP endpoint and no
	// broker credential accessor composed anywhere yet, and inventing either
	// would be new structural config this packet has no mandate for. The
	// probes themselves are built and stub-tested
	// (watchlist.HTTPAuthProbe / HTTPModelListProbe); what is missing is
	// gate-time material, so composing them is a B5-gate item.
	canaryNoCredentialedEndpoint = "no credentialed per-lane endpoint is composed: the endpoint set and the " +
		"broker credential accessor are B5-gate install material (D2/S11.5); the probe itself is built and stub-tested"
	// canaryNoPinnedRunner is the carried B5-5 dependency.
	canaryNoPinnedRunner = "no pinned runner is installed (" + evals.PromptfooPathEnv +
		" or PATH): the exact-version install is a B5-gate HOST act"
)

// buildCanaryLayer composes the S14.6 ¶3 API canary layer (B5-6B).
//
// THE $0 POSTURE IS COMPOSED HERE, not hidden in the package: no leg that dials
// a provider gets a probe unless the operator has armed the layer
// (watchlist.CanaryArmEnv), so unarmed the layer physically cannot spend.
//
// Every real-request leg is CONSTRUCTED either way, carrying either a probe or
// a NAMED reason it has none. That is deliberate: a nil leg is silently skipped
// and disappears from the sweep accounting, whereas a constructed-but-disarmed
// leg is scheduled, counted, and reports why it did not run (drain D1).
//
// Arming is necessary but NOT sufficient. On this tree the arm activates the
// behavioral leg only, and only when the pinned runner is installed; the auth
// and model-list legs additionally need per-lane endpoints and a broker
// credential accessor that do not exist until the B5-gate install. The armed
// log line names every leg it could not compose.
//
// The logprob canary is local-tier and costs no allowance, so it is never gated
// by the arm — it is wired whenever the local stack is.
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
		reason := watchlist.DisarmedReasonNotArmed
		c.Auth = watchlist.DisarmedAuthCanary(reason)
		c.ModelList = watchlist.DisarmedModelListCanary(reason)
		c.Behavioral = watchlist.DisarmedBehavioralCanary(reason)
		logger.Info("watchlist: API canary layer wired, real-request legs DISARMED (B5-6B)",
			"arm_with", watchlist.CanaryArmEnv,
			"disarmed", "auth, model-list, behavioral",
			"live", "logprob (local tier, $0), conformance dueness")
		return c, false
	}

	// Armed: compose what this tree can, and NAME what it cannot.
	uncomposed := []string{}
	c.Auth = watchlist.DisarmedAuthCanary(canaryNoCredentialedEndpoint)
	c.ModelList = watchlist.DisarmedModelListCanary(canaryNoCredentialedEndpoint)
	uncomposed = append(uncomposed, "auth ("+canaryNoCredentialedEndpoint+")",
		"model-list ("+canaryNoCredentialedEndpoint+")")

	if runner, err := evals.FindPromptfoo(); err == nil {
		c.Behavioral = watchlist.NewBehavioralCanary(behavioralCanaryHook(runner))
	} else {
		c.Behavioral = watchlist.DisarmedBehavioralCanary(canaryNoPinnedRunner)
		uncomposed = append(uncomposed, "behavioral ("+canaryNoPinnedRunner+": "+err.Error()+")")
	}

	logger.Info("watchlist: API canary layer ARMED (B5-6B)",
		"composed", armedComposedLegs(c),
		"not_composed", uncomposed)
	return c, true
}

// armedComposedLegs names the real-request legs that actually hold a probe.
func armedComposedLegs(c *watchlist.Canaries) []string {
	out := []string{}
	if c.Auth != nil && c.Auth.Probe != nil {
		out = append(out, "auth")
	}
	if c.ModelList != nil && c.ModelList.Probe != nil {
		out = append(out, "model-list")
	}
	if c.Behavioral != nil && c.Behavioral.Run != nil {
		out = append(out, "behavioral")
	}
	return out
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
					"disarmed", sweep.Disarmed, "stack_absent", sweep.StackAbsent,
					"conformance_cards", sweep.Cards, "skipped", sweep.Reasons)
			}
		}
	}
}

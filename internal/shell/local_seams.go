package shell

import (
	"context"
	"log/slog"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/units"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// local_seams.go — the S12 local-tier composition (brief R12/R20–R22). The
// local duty surface is built at the composition root from the production
// stores and the structural config (SINET_LOCAL_* — R28, the SINET_SRT_PATH
// precedent); when the stack is UNCONFIGURED (the dev default) every seam is
// nil and the pipeline/routing degrade exactly per the S12.4/S06 rows (R17).
// The eager-unload surface is compiled in and reachable (card DATA for the B6
// endpoints — internal/api is UNCHANGED at v0, §4; the shell holds it, exactly
// as buildAcceptSurface is held for B6). The stage duty-seam ADAPTERS live in
// internal/stage (bridging into intake/worker); internal/local imports neither
// (R24).

// localDeps are the production stores the local surface composes over.
type localDeps struct {
	Settings    *settings.Registry
	Checkpoints *gates.Checkpoints
	Events      *eventlog.Log
	Log         *slog.Logger
}

// localSurface holds the composed S12 local surface: the duty caller, the
// eager-unload surface (card DATA for B6), the class-(b) seams wired into
// stage.Config, the effective DutyMap (with the utility seat), and the
// ⚙-gated GameMode hook. When the stack is unconfigured, Available is false
// and every seam is nil.
type localSurface struct {
	Duty       *local.Duty
	Surface    *local.Surface
	Classifier intake.Classifier
	Utility    intake.Utility
	SpotCheck  intake.SpotCheck
	TieBreak   worker.TieBreaker
	DutyMap    worker.DutyMap
	Available  bool
	GameMode   *local.GameModeHook
}

// buildLocalSurface composes the S12 local surface (R12/R20–R22). It never
// errors on an unconfigured stack — that is the sanctioned dev default (an
// inert surface, seams nil). An error is a genuine composition failure (⚙ read).
func buildLocalSurface(d localDeps) (*localSurface, error) {
	cfg := local.StackFromEnv()
	if !cfg.Configured() {
		return &localSurface{}, nil // dev default: unconfigured → the seams degrade (R17)
	}
	reg := local.NewRegistry(d.Settings)
	duty := local.NewDuty(local.DutyDeps{
		Registry:    reg,
		Client:      local.NewClient(cfg.Endpoint),
		Checkpoints: d.Checkpoints,
		Events:      d.Events,
		Logger:      d.Log,
	})
	surface := local.NewSurface(local.NewEagerUnload(duty, d.Events, d.Log))

	// The effective DutyMap gains the utility seat (local lane, manifest
	// window) when configured; Coverage.LocalAvailable flips true (R22). The
	// class-(a) engine dispatch onto it still degrades to the paid seat (no
	// v0 consumer — the BINDING reading; routing.go's refined reason).
	dutyMap := worker.DefaultDutyMap()
	if seat, err := reg.SeatFor(local.AliasUtility); err == nil {
		dutyMap[worker.DutyUtility] = worker.Seat{Model: seat.Model, Lane: local.LaneLocal, WindowTokens: seat.ContextLen}
	}

	ls := &localSurface{
		Duty:       duty,
		Surface:    surface,
		Classifier: stage.NewLocalClassifier(duty),
		Utility:    stage.NewLocalUtility(duty),
		SpotCheck:  stage.NewLocalSpotCheck(duty),
		TieBreak:   stage.NewLocalTieBreaker(duty),
		DutyMap:    dutyMap,
		Available:  true,
	}

	// The ⚙-gated GameMode hook: the scripts leg (rendered snippet, operator
	// installs) + the busctl subscription (probe-bound — deferred-with-finding
	// unless the operator session-bus address is supplied, R13/R14). The two
	// legs call the eager-unload verbs.
	gm, err := local.NewGameModeHook(d.Settings, units.DefaultBinaryPath, cfg.GameModeBus,
		func(ctx context.Context) { _ = surface.Engage(ctx, "gamemode") },
		func(ctx context.Context) { _ = surface.Resume(ctx, "gamemode") },
		d.Log)
	if err != nil {
		return nil, err
	}
	ls.GameMode = gm
	return ls, nil
}

// startGameMode launches the ⚙-gated GameMode D-Bus subscription in the
// background (the scripts leg is operator-installed config, not a runtime
// goroutine). It is inert when ⚙ off or when the probe-bound bus address is
// unset (deferred-with-finding, R14).
func (ls *localSurface) startGameMode(ctx context.Context) {
	if ls == nil || ls.GameMode == nil || !ls.GameMode.Enabled || ls.GameMode.Subscription == nil {
		return
	}
	go func() { _ = ls.GameMode.Subscription.Run(ctx) }()
}

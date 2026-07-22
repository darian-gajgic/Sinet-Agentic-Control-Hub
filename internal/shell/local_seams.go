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
	// VRAM is the S12.7 admitter (B4-6): the scheduler's GPU-admission policy
	// hook (nil-safe; wired-but-dormant) AND the live duty pre-flight (OQ2). Nil
	// when the stack is unconfigured.
	VRAM *local.VRAMAdmitter
	// LocalLane is the lane the scheduler GPU-gates ("local" when configured, ""
	// otherwise) — passed to scheduler.Config as a bare string (no import of
	// internal/local from internal/scheduler, §26).
	LocalLane string
	// SandboxBroker is the S12.6 GPU-broker sandboxed-plane surface (B4-6): the
	// token registry + the /v1 bridge handler. Composed + held DORMANT — no
	// class-(a) consumer mints a token this cut (BINDING), so the handler is
	// never served to a live sandbox; it is a hermetically-tested seam.
	SandboxBroker *local.SandboxBroker
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
	// The S12.7 VRAM admitter (B4-6): live sysfs+nvidia-smi reads, sleep-gated,
	// honest-uncalibrated (OQ1). Backs the scheduler hook (R13) AND the duty
	// pre-flight (OQ2).
	vram := local.NewVRAMAdmitter(local.VRAMDeps{Settings: d.Settings, Logger: d.Log})
	duty := local.NewDuty(local.DutyDeps{
		Registry:    reg,
		Client:      local.NewClient(cfg.Endpoint),
		Checkpoints: d.Checkpoints,
		Events:      d.Events,
		// OQ2: the live GPU-admission pre-flight before a duty call triggers a
		// llama-swap load (operator-wins holds live).
		Admitter: vram,
		// The durable cross-process admission flag (F4/F11): the CLI resolves
		// the same path from SINET_LOCAL_STATE via StackFromEnv, so an engaged
		// stop survives a restart and the CLI verb + surface share it.
		AdmissionsFlag: cfg.AdmissionsFlag(),
		Logger:         d.Log,
	})
	// The S12.7 kill-not-freeze preemptor (B4-6/R11) wired BEHIND the eager-unload
	// verb: after llama-swap's unload, a wedged backend still holding VRAM is
	// SIGTERM→grace→SIGKILL-ed and recovery verified.
	preempt := local.NewPreemptor(local.PreemptorDeps{Settings: d.Settings, Admitter: vram, Logger: d.Log})
	eager := local.NewEagerUnload(duty, d.Events, d.Log)
	eager.SetPreemptor(preempt)
	surface := local.NewSurface(eager)

	// The S12.6 GPU-broker sandboxed plane (B4-6): the per-run token registry +
	// the /v1 bridge handler. Composed + held DORMANT — no v0 consumer mints a
	// token (BINDING), so the handler is never served to a live sandbox.
	sandboxBroker := local.NewSandboxBroker(local.SandboxBrokerDeps{
		Tokens:      local.NewTokenRegistry(),
		Registry:    reg,
		Client:      local.NewClient(cfg.Endpoint),
		Checkpoints: d.Checkpoints,
		Events:      d.Events,
		Settings:    d.Settings,
		Logger:      d.Log,
	})

	// The effective DutyMap gains the utility seat (local lane, manifest
	// window) when configured; Coverage.LocalAvailable flips true (R22). The
	// class-(a) engine dispatch onto it still degrades to the paid seat (no
	// v0 consumer — the BINDING reading; routing.go's refined reason).
	dutyMap := worker.DefaultDutyMap()
	if seat, err := reg.SeatFor(local.AliasUtility); err == nil {
		dutyMap[worker.DutyUtility] = worker.Seat{Model: seat.Model, Lane: local.LaneLocal, WindowTokens: seat.ContextLen}
	}

	ls := &localSurface{
		Duty:          duty,
		Surface:       surface,
		Classifier:    stage.NewLocalClassifier(duty),
		Utility:       stage.NewLocalUtility(duty),
		SpotCheck:     stage.NewLocalSpotCheck(duty),
		TieBreak:      stage.NewLocalTieBreaker(duty),
		DutyMap:       dutyMap,
		Available:     true,
		VRAM:          vram,
		LocalLane:     local.LaneLocal,
		SandboxBroker: sandboxBroker,
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

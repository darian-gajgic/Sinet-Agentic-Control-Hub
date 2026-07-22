package local

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// gamemode.go — the S12.2 / G2 Def.6 GameMode hook (brief R13), gated whole by
// ⚙ local.gamemode_hook. Two legs:
//
//	(a) a GENERATED gamemode.ini [custom] start/end snippet calling the two
//	    eager-unload verbs (via the `sinet local` CLI). gamemode.ini is USER
//	    config — the snippet is RENDERED OUTPUT + a documented operator install
//	    step, NEVER written into any home directory by the packet (R13a).
//	(b) a control-plane D-Bus subscription to GameRegistered/GameUnregistered,
//	    implemented WITHOUT a new Go dependency — a supervised subprocess of
//	    host systemd's own `busctl` in monitor mode (§8 reading 8), riding the
//	    existing OS-mechanism/host-systemd posture. A Go D-Bus library would be
//	    a net-new S16.4 adoption (stop-and-flag), not taken.
//
// The R14 probe (P3/measurements/2026-07-22-gamemode-user-context-probe.md)
// BINDS leg (b): GameMode signals are on the per-user SESSION bus, unreachable
// from the `sinet` system user without the operator's DBUS_SESSION_BUS_ADDRESS
// — so the subscription runs ONLY when that address is supplied (bring-up
// config); absent it, the subscription is DEFERRED-WITH-FINDING and the
// scripts leg (a) carries pause/resume (it runs inside the operator's game
// session, bus-scope-independent). Both outcomes are spec-satisfying.

// GameModeSnippet renders the gamemode.ini [custom] section that fires the two
// eager-unload verbs (R13a). binary is the installed `sinet` path (default
// "sinet" on PATH). The platform NEVER writes this into a home directory — the
// caller emits it for the operator to merge into ~/.config/gamemode.ini.
func GameModeSnippet(binary string) string {
	if binary == "" {
		binary = "sinet"
	}
	return fmt.Sprintf(`# Sinet-generated GameMode hook (Spec S12.2; brief R13a). Merge into the
# operator's ~/.config/gamemode.ini [custom] section. GENERATED OUTPUT — the
# platform never writes this file. These scripts run inside the operator's
# game session and free the GPU for the game, bus-scope-independent (the R14
# probe: session-bus signals do not reach the system-scope control plane).
[custom]
start=%s local unload
end=%s local resume
`, binary, binary)
}

// GameModeSubscription is the leg-(b) supervised busctl monitor subprocess.
// Nil-safe; disabled (⚙ off) or unaddressed (probe-bound) it is inert.
type GameModeSubscription struct {
	// BusAddr is the operator session-bus address (DBUS_SESSION_BUS_ADDRESS)
	// the subscription monitors. "" ⇒ deferred-with-finding (the R14 verdict:
	// unreachable from system scope without it). Structural config, bring-up.
	BusAddr string
	// OnStart/OnEnd fire on GameRegistered/GameUnregistered.
	OnStart func(ctx context.Context)
	OnEnd   func(ctx context.Context)
	Logger  *slog.Logger
}

// dbus signal member names on com.feralinteractive.GameMode (probe-confirmed,
// 2026-07-22): the busctl monitor prints "Member=GameRegistered" /
// "Member=GameUnregistered" lines.
const (
	memberGameRegistered   = "GameRegistered"
	memberGameUnregistered = "GameUnregistered"
	gameModeService        = "com.feralinteractive.GameMode"
)

// Run supervises the busctl monitor subprocess until ctx is cancelled. When
// BusAddr is unset it logs the R14 finding and returns nil (deferred, NOT an
// error — the scripts leg carries the duty). Restarts the monitor on exit so
// the subscription covers the daemon's own restarts (R13b).
func (g *GameModeSubscription) Run(ctx context.Context) error {
	logger := g.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if g.BusAddr == "" {
		logger.Warn("local: GameMode D-Bus subscription DEFERRED-WITH-FINDING — GameMode signals are session-scope and unreachable from the sinet system user without the operator's DBUS_SESSION_BUS_ADDRESS (R14 probe: P3/measurements/2026-07-22-gamemode-user-context-probe.md). The gamemode.ini scripts leg carries pause/resume regardless (R13a).")
		return nil
	}
	logger.Info("local: GameMode D-Bus subscription active (busctl monitor subprocess; zero new Go deps)", "bus", g.BusAddr, "service", gameModeService)
	backoff := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		err := g.monitorOnce(ctx, logger)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			logger.Warn("local: GameMode busctl monitor exited; restarting (covers gamemoded/control-plane restarts)", "err", err, "backoff", backoff)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// monitorOnce runs one busctl monitor and dispatches signals until it exits.
func (g *GameModeSubscription) monitorOnce(ctx context.Context, logger *slog.Logger) error {
	cmd := exec.CommandContext(ctx, "busctl", "--user", "monitor", gameModeService)
	cmd.Env = append(cmd.Environ(), "DBUS_SESSION_BUS_ADDRESS="+g.BusAddr)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("busctl stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("busctl start: %w", err)
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.Contains(line, "Member="+memberGameRegistered):
			logger.Info("local: GameMode GameRegistered — engaging eager-unload (game wants the GPU)")
			if g.OnStart != nil {
				g.OnStart(ctx)
			}
		case strings.Contains(line, "Member="+memberGameUnregistered):
			logger.Info("local: GameMode GameUnregistered — resuming local-lane admissions")
			if g.OnEnd != nil {
				g.OnEnd(ctx)
			}
		}
	}
	return cmd.Wait()
}

// GameModeHook ties the ⚙ gate + the two legs. Enabled reflects ⚙
// local.gamemode_hook; Snippet is the rendered scripts leg; Subscription is
// leg (b) (may be deferred-with-finding). Constructed at the shell composition
// root when the local stack is configured.
type GameModeHook struct {
	Enabled      bool
	Snippet      string
	Subscription *GameModeSubscription
}

// NewGameModeHook reads ⚙ local.gamemode_hook and builds the hook. binary is
// the installed sinet path (scripts leg); busAddr is the operator session-bus
// address (leg b; "" ⇒ deferred-with-finding per the R14 probe). onStart/onEnd
// wire the eager-unload verbs.
func NewGameModeHook(settings Settings, binary, busAddr string, onStart, onEnd func(ctx context.Context), logger *slog.Logger) (*GameModeHook, error) {
	enabled, err := settings.Bool(keyGameModeHook)
	if err != nil {
		return nil, fmt.Errorf("local: read ⚙ %s: %w", keyGameModeHook, err)
	}
	h := &GameModeHook{Enabled: enabled}
	if !enabled {
		return h, nil // ⚙ off gates the whole behavior (R13)
	}
	h.Snippet = GameModeSnippet(binary)
	h.Subscription = &GameModeSubscription{BusAddr: busAddr, OnStart: onStart, OnEnd: onEnd, Logger: logger}
	return h, nil
}

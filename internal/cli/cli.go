// Package cli implements mode dispatch for the sinet multi-call binary.
//
// Spec S01.5: the control plane, the credential broker, and the port-pool
// daemon are one static binary invoked in dedicated modes. This package owns
// the mode table; daemon modes not yet built are reserved stubs, so the
// invocation shape and future unit ExecStart lines are stable from the
// first build. Non-daemon subcommands (version, units, help) are tools of
// the same binary, not modes: they never get a unit file.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/backup"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/buildinfo"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/portpool"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/sandbox"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/shell"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/units"
)

// Exit codes.
const (
	exitOK    = 0
	exitError = 1 // mode exists but failed (including: not implemented in this build)
	exitUsage = 2 // unknown mode or bad invocation
)

const usage = `sinet — Sinet platform binary (one artifact, invoked per mode; Spec S01.5)

Usage:

  sinet <mode> [arguments]

Modes:

  control    platform control plane (sinet-control.service)
  broker     credential broker (sinet-broker.service; Spec S11.5)
  portpool   preview port-pool daemon (sinet-portpool.service; Spec S13.8)

Tools:

  units        render the systemd unit set (generated, never installed)
  local        local-tier tool (Spec S12): unload|resume|status (the eager-
               unload verb, S12.2) + config|manifest|gamemode-snippet renders
               (GENERATED, never installed). Management verbs are loopback-only
  snapshot     take one platform-state snapshot (Spec S13.10; the one-shot
               driven by the generated sinet-snapshot.timer)
  restore-drill run one verified-restore drill (Spec S13.10; the one-shot
               driven by the generated sinet-restore-drill.timer)
  run-launch   per-run sandbox launcher — the fixed ExecStart of the
               sinet-run@ template (Spec S11.8); reads a spool record and
               composes the S11.1 stack. System-invoked, never by hand.
  engine-hook  engine hook for the Claude lane (invoked by the engine per
               Sinet-lowered settings, never by hand): PreToolUse gate
               (Spec S03.4), with --session-start the SessionStart
               pinned-context re-injection (Spec S05.7), or with
               --post-tool-use the PostToolUse recitation delivery valve
               (Spec S05.3)
  version      print build and version information
  help         print this usage text
`

// reserved names the daemon modes of Spec S01.2 ahead of their
// implementations. The broker mode is implemented at B1-3 (Spec S11.5) and the
// portpool mode at B4-4 (Spec S13.8); both left the table. It is empty now —
// every S01.2 daemon mode is built — but the map stays as the reserved-mode
// seam so a future daemon fails with "not implemented" rather than "unknown".
var reserved = map[string]string{}

// Run executes one invocation of the sinet binary and returns its exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	mode := args[0]
	switch mode {
	case "control":
		return shell.Main(args[1:], stdout, stderr)
	case "broker":
		return broker.Main(args[1:], stdout, stderr)
	case "portpool":
		return portpool.Main(args[1:], stdout, stderr)
	case "run-launch":
		return sandbox.RunLaunch(args[1:], stdout, stderr)
	case "snapshot":
		return backup.SnapshotMode(args[1:], stdout, stderr)
	case "restore-drill":
		return backup.DrillMode(args[1:], stdout, stderr)
	case "units":
		return runUnits(args[1:], stdout, stderr)
	case "local":
		return runLocal(args[1:], stdout, stderr)
	case "engine-hook":
		return runEngineHook(args[1:], stdout, stderr)
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "sinet %s\n", buildinfo.String())
		return exitOK
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return exitOK
	}
	if _, ok := reserved[mode]; ok {
		fmt.Fprintf(stderr, "sinet %s: not implemented in this build\n", mode)
		return exitError
	}
	fmt.Fprintf(stderr, "sinet: unknown mode %q\n\n", mode)
	fmt.Fprint(stderr, usage)
	return exitUsage
}

// runEngineHook executes one engine-hook invocation for the Claude lane:
// the PreToolUse gate (Spec S03.4, `sinet engine-hook --ctl <dir>`), with
// --session-start the SessionStart pinned-context re-injection (Spec S05.7
// step 3; B1-4 spike mechanics), or with --post-tool-use the PostToolUse
// recitation delivery valve (Spec S05.3 over the S03.4 ctl-dir airlock;
// Research/18 §7-C1). Stdin carries the hook payload, stdout the hook
// JSON. A tool subcommand, not a mode (no unit file).
func runEngineHook(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sinet engine-hook", flag.ContinueOnError)
	fs.SetOutput(stderr)
	ctl := fs.String("ctl", "", "gate control directory compiled into the lowered settings")
	sessionStart := fs.String("session-start", "", "pinned-context file to re-emit as SessionStart additionalContext (Spec S05.7)")
	postToolUse := fs.Bool("post-tool-use", false, "recitation delivery valve: consume the pending recitation, if any, as PostToolUse additionalContext (Spec S05.3)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "sinet engine-hook: unexpected argument %q\n", fs.Arg(0))
		return exitUsage
	}
	if *sessionStart != "" && *postToolUse {
		fmt.Fprintf(stderr, "sinet engine-hook: --session-start and --post-tool-use are exclusive (one hook event per invocation)\n")
		return exitUsage
	}
	var err error
	switch {
	case *sessionStart != "":
		err = claudecli.RunSessionStartHook(os.Stdin, stdout, *ctl, *sessionStart)
	case *postToolUse:
		err = claudecli.RunPostToolUseHook(os.Stdin, stdout, *ctl)
	default:
		err = claudecli.RunHook(os.Stdin, stdout, *ctl)
	}
	if err != nil {
		fmt.Fprintf(stderr, "sinet engine-hook: %v\n", err)
		return exitError
	}
	return exitOK
}

// runLocal is the `sinet local` tool subcommand (Spec S12; brief R12 — a
// tool, never a mode: no unit file, not in the reserved table). Verbs:
// unload/resume/status (the eager-unload verb, S12.2), and config/manifest/
// gamemode-snippet renders (GENERATED, never installed). Management verbs are
// loopback-only (S12.6); the unload leg reaches llama-swap directly, and the
// admission-stop leg rides the control-plane surface (B6) — the dev posture is
// documented honestly (R12).
func runLocal(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "sinet local: subcommand required (unload|resume|status|config|manifest|gamemode-snippet)")
		return exitUsage
	}
	ctx := context.Background()
	endpoint := os.Getenv("SINET_LOCAL_ENDPOINT")
	switch args[0] {
	case "unload":
		if endpoint == "" {
			fmt.Fprintln(stderr, "sinet local unload: SINET_LOCAL_ENDPOINT unset (the local stack is not configured)")
			return exitError
		}
		if err := local.UnloadAllDirect(ctx, endpoint); err != nil {
			fmt.Fprintf(stderr, "sinet local unload: %v\n", err)
			return exitError
		}
		fmt.Fprintln(stdout, "local: unloaded all models (direct llama-swap leg, S12.2).")
		fmt.Fprintln(stdout, "note: stopping local-lane admissions is a control-plane act — it rides the eager-unload surface (B6 endpoint); this CLI does the unload leg only (S12.6 dev posture).")
		return exitOK
	case "resume":
		fmt.Fprintln(stdout, "local: resume rides the control-plane eager-unload surface (B6 endpoint); models reload on demand at llama-swap regardless. Standalone this leg is a documented no-op (S12.6 dev posture).")
		return exitOK
	case "status":
		fmt.Fprintf(stdout, "local: endpoint=%q llama-swap-pin=%s llama.cpp-pin=%s\n", endpoint, local.LlamaSwapPin, local.LlamaCppPin)
		if endpoint == "" {
			fmt.Fprintln(stdout, "local: stack UNCONFIGURED (dev default) — the duty seams degrade per the S12.4/S06 rows (R17).")
		}
		return exitOK
	case "manifest":
		b, err := local.ManifestJSON()
		if err != nil {
			fmt.Fprintf(stderr, "sinet local manifest: %v\n", err)
			return exitError
		}
		fmt.Fprintln(stdout, string(b))
		return exitOK
	case "gamemode-snippet":
		fmt.Fprint(stdout, local.GameModeSnippet(units.DefaultBinaryPath))
		return exitOK
	case "config":
		return runLocalConfig(stdout, stderr)
	default:
		fmt.Fprintf(stderr, "sinet local: unknown subcommand %q\n", args[0])
		return exitUsage
	}
}

// runLocalConfig renders the llama-swap config from the manifest + ⚙ defaults
// + the live-read GPU UUID (Spec S12.2; brief R10). GENERATED, never
// installed. ⚙ values render from the declared defaults (registry unattached,
// like `sinet units`); paths are the SINET_LOCAL_* structural config.
func runLocalConfig(stdout, stderr io.Writer) int {
	reg := settings.New()
	ttlFast, err := reg.Int("local.ttl.fast_s")
	if err != nil {
		fmt.Fprintf(stderr, "sinet local config: %v\n", err)
		return exitError
	}
	ttlWork, err := reg.Int("local.ttl.workhorse_s")
	if err != nil {
		fmt.Fprintf(stderr, "sinet local config: %v\n", err)
		return exitError
	}
	grace, err := reg.Int("local.unload.term_grace_s")
	if err != nil {
		fmt.Fprintf(stderr, "sinet local config: %v\n", err)
		return exitError
	}
	sc := local.StackFromEnv()
	uuids, err := local.GPUUUIDs(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "sinet local config: %v\n", err)
		return exitError
	}
	llamaServer := sc.LlamaServer
	if llamaServer == "" {
		llamaServer = "/usr/local/bin/llama-server"
	}
	cache := sc.ModelCache
	if cache == "" {
		cache = "/var/lib/sinet/models"
	}
	out, err := local.GenerateConfig(local.ConfigParams{
		ModelCacheDir: cache, GPUUUIDs: uuids, LlamaServer: llamaServer,
		TTLFastS: ttlFast, TTLWorkhorseS: ttlWork, UnloadGraceS: grace,
	})
	if err != nil {
		fmt.Fprintf(stderr, "sinet local config: %v\n", err)
		return exitError
	}
	fmt.Fprint(stdout, out)
	fmt.Fprintln(stdout, "# GENERATED — installing the config + the sinet-llamaswap.service unit is a B4-gate operator decision.")
	return exitOK
}

// runUnits renders the Spec S01.2 unit set to stdout, or to an
// operator-chosen directory with --out. It never touches the host:
// installation (files under /etc, systemctl) is a B0-gate operator decision.
func runUnits(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sinet units", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "write one file per unit into this directory (created if absent); default: stdout")
	binary := fs.String("binary", units.DefaultBinaryPath, "installed binary path rendered into ExecStart= lines")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "sinet units: unexpected argument %q\n", fs.Arg(0))
		return exitUsage
	}
	// ⚙ values render from the declared defaults (registry unattached):
	// no DB is opened — sinet-control is the sole writer of a live
	// platform.db (Spec S02.1) and generation must never contend with it.
	files, err := units.Files(settings.New(), units.Params{BinaryPath: *binary})
	if err != nil {
		fmt.Fprintf(stderr, "sinet units: %v\n", err)
		return exitError
	}
	if *out == "" {
		for _, f := range files {
			draft := ""
			if f.Draft {
				draft = " (DRAFT — not installable until B1)"
			}
			fmt.Fprintf(stdout, "# ── %s%s ──\n%s\n", f.Name, draft, f.Content)
		}
		return exitOK
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintf(stderr, "sinet units: %v\n", err)
		return exitError
	}
	for _, f := range files {
		path := filepath.Join(*out, f.Name)
		if err := os.WriteFile(path, []byte(f.Content), 0o644); err != nil {
			fmt.Fprintf(stderr, "sinet units: %v\n", err)
			return exitError
		}
		draft := ""
		if f.Draft {
			draft = "  (DRAFT — not installable until B1)"
		}
		fmt.Fprintf(stdout, "wrote %s%s\n", path, draft)
	}
	fmt.Fprintln(stdout, "generated only — installing units is a B0-gate operator decision")
	return exitOK
}

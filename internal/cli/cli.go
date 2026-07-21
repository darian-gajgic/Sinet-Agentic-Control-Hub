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
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/buildinfo"
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
  portpool   preview port-pool daemon (sinet-portpool.service) [not yet implemented]

Tools:

  units        render the systemd unit set (generated, never installed)
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
// implementations. The broker mode is implemented at B1-3 (Spec S11.5) and
// left the table.
var reserved = map[string]string{
	"portpool": "sinet-portpool",
}

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
	case "run-launch":
		return sandbox.RunLaunch(args[1:], stdout, stderr)
	case "units":
		return runUnits(args[1:], stdout, stderr)
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

// Package cli implements mode dispatch for the sinet multi-call binary.
//
// Spec S01.5: the control plane, the credential broker, and the port-pool
// daemon are one static binary invoked in dedicated modes. This package owns
// the mode table; the daemon modes are reserved stubs until their owning
// build packets land, so the invocation shape and future unit ExecStart
// lines are stable from the first build.
package cli

import (
	"fmt"
	"io"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/buildinfo"
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

  control    platform control plane (sinet-control.service)    [not yet implemented]
  broker     credential broker (sinet-broker.service)          [not yet implemented]
  portpool   preview port-pool daemon (sinet-portpool.service) [not yet implemented]
  version    print build and version information
  help       print this usage text
`

// reserved names the daemon modes of Spec S01.2 ahead of their
// implementations.
var reserved = map[string]string{
	"control":  "sinet-control",
	"broker":   "sinet-broker",
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

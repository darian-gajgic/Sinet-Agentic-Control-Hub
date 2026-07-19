package cli

import (
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut strings.Builder
	code = Run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersionMode(t *testing.T) {
	for _, arg := range []string{"version", "--version", "-v"} {
		code, out, _ := run(t, arg)
		if code != exitOK {
			t.Errorf("%s: exit %d, want %d", arg, code, exitOK)
		}
		if !strings.HasPrefix(out, "sinet v") {
			t.Errorf("%s: output %q, want prefix %q", arg, out, "sinet v")
		}
	}
}

func TestHelpMode(t *testing.T) {
	code, out, _ := run(t, "help")
	if code != exitOK {
		t.Fatalf("help: exit %d, want %d", code, exitOK)
	}
	for _, mode := range []string{"control", "broker", "portpool", "version"} {
		if !strings.Contains(out, mode) {
			t.Errorf("help output lacks mode %q", mode)
		}
	}
}

func TestNoArgsPrintsUsage(t *testing.T) {
	code, _, errOut := run(t)
	if code != exitUsage {
		t.Fatalf("no args: exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "Usage:") {
		t.Fatalf("no args: stderr %q lacks usage text", errOut)
	}
}

func TestUnknownMode(t *testing.T) {
	code, _, errOut := run(t, "frobnicate")
	if code != exitUsage {
		t.Fatalf("unknown mode: exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, `unknown mode "frobnicate"`) {
		t.Fatalf("unknown mode: stderr %q lacks diagnosis", errOut)
	}
}

func TestReservedModesReportUnimplemented(t *testing.T) {
	for _, mode := range []string{"control", "broker", "portpool"} {
		code, _, errOut := run(t, mode)
		if code != exitError {
			t.Errorf("%s: exit %d, want %d", mode, code, exitError)
		}
		if !strings.Contains(errOut, "not implemented") {
			t.Errorf("%s: stderr %q lacks not-implemented notice", mode, errOut)
		}
	}
}

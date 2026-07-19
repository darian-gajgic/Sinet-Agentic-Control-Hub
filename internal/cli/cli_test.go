package cli

import (
	"os"
	"path/filepath"
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
	for _, mode := range []string{"broker", "portpool"} {
		code, _, errOut := run(t, mode)
		if code != exitError {
			t.Errorf("%s: exit %d, want %d", mode, code, exitError)
		}
		if !strings.Contains(errOut, "not implemented") {
			t.Errorf("%s: stderr %q lacks not-implemented notice", mode, errOut)
		}
	}
}

func TestControlModeRejectsBadInvocation(t *testing.T) {
	// The control mode is real (Spec S01.6); a bad invocation must fail
	// before any startup work.
	code, _, _ := run(t, "control", "--no-such-flag")
	if code != exitUsage {
		t.Errorf("control --no-such-flag: exit %d, want %d", code, exitUsage)
	}
	code, _, errOut := run(t, "control", "stray-arg")
	if code != exitUsage {
		t.Errorf("control stray-arg: exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "unexpected argument") {
		t.Errorf("control stray-arg: stderr %q lacks diagnosis", errOut)
	}
}

func TestUnitsToStdout(t *testing.T) {
	code, out, errOut := run(t, "units")
	if code != exitOK {
		t.Fatalf("units: exit %d, stderr %s", code, errOut)
	}
	for _, want := range []string{
		"sinet-control.service",
		"sinet-broker.service",
		"sinet-engine@.service",
		"sinet-run@.service",
		"sinet-portpool.service",
		"journald-sinet.conf",
		"WatchdogSec=30",
		"DRAFT",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("units stdout lacks %q", want)
		}
	}
}

func TestUnitsToDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "units")
	code, out, errOut := run(t, "units", "--out", dir)
	if code != exitOK {
		t.Fatalf("units --out: exit %d, stderr %s", code, errOut)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read out dir: %v", err)
	}
	if len(entries) != 6 {
		t.Fatalf("%d files written, want 6", len(entries))
	}
	body, err := os.ReadFile(filepath.Join(dir, "sinet-control.service"))
	if err != nil {
		t.Fatalf("read control unit: %v", err)
	}
	if !strings.Contains(string(body), "ExecStart=/usr/local/bin/sinet control") {
		t.Error("written control unit lacks its ExecStart")
	}
	if !strings.Contains(out, "operator decision") {
		t.Errorf("units --out stdout %q lacks the never-installed notice", out)
	}
}

func TestUnitsRejectsStrayArgs(t *testing.T) {
	code, _, _ := run(t, "units", "stray")
	if code != exitUsage {
		t.Errorf("units stray: exit %d, want %d", code, exitUsage)
	}
}

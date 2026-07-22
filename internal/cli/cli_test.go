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

func TestReservedTableEmpty(t *testing.T) {
	// broker (S11.5) + run-launch (S11.8) at B1-3, portpool (S13.8) at B4-4 —
	// every S01.2 daemon mode is now built and left the reserved table. The map
	// stays as the reserved-mode seam so a future daemon fails "not implemented"
	// rather than "unknown mode".
	if len(reserved) != 0 {
		t.Errorf("reserved table is non-empty (%v) — every S01.2 daemon mode should be built", reserved)
	}
}

// TestDaemonModesWired proves the daemon modes are recognized (not "unknown
// mode") without starting a daemon (which would block): a bad flag returns the
// usage exit code from each mode's flag parser.
func TestDaemonModesWired(t *testing.T) {
	for _, mode := range []string{"broker", "run-launch", "portpool"} {
		code, _, errOut := run(t, mode, "--definitely-not-a-flag")
		if code != exitUsage {
			t.Errorf("%s --badflag: exit %d, want %d (stderr: %q)", mode, code, exitUsage, errOut)
		}
		if strings.Contains(errOut, "unknown mode") {
			t.Errorf("%s: reported as unknown mode (should be wired)", mode)
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

// TestSnapshotAndDrillModesExist: `sinet snapshot` and `sinet restore-drill`
// are recognized one-shot modes (Spec S13.10, R21/R22) — they dispatch to the
// backup pipeline and fail with a config error, never "unknown mode".
func TestSnapshotAndDrillModesExist(t *testing.T) {
	for _, mode := range []string{"snapshot", "restore-drill"} {
		code, _, errOut := run(t, mode)
		if strings.Contains(errOut, "unknown mode") {
			t.Errorf("%q reported unknown mode: %s", mode, errOut)
		}
		// Missing --snapshot-remote (a bring-up config) → a usage error, not a
		// crash, and definitely a recognized mode.
		if code == exitOK {
			t.Errorf("%q with no config unexpectedly succeeded", mode)
		}
		if !strings.Contains(errOut, "snapshot-remote") {
			t.Errorf("%q did not name the required --snapshot-remote: %s", mode, errOut)
		}
	}
	// The usage text lists them as tools.
	_, out, _ := run(t, "help")
	for _, want := range []string{"snapshot", "restore-drill"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage text missing %q", want)
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
	if len(entries) != 10 {
		t.Fatalf("%d files written, want 10", len(entries))
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

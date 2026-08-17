package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

// TestComposeCreatesHandedWorkDirOnFirstUse pins P3-RW-15: on a FRESH world the
// platform-owned scratch dir (S03.5) does not exist yet — the shell hands
// `<state>/check-work` to the V1 SandboxCheckRunner and nothing has created it.
// The srt path writes its config file into that dir, so compose must create it
// on demand; before the fix the very first check run on every fresh world died
// at `write srt config: ... no such file or directory` and escalated as
// CHECK-INTEGRITY noise instead of running the checks.
//
// Hermetic: faked caps + the srt stub, tempdir only (the
// TestSrtStubReceivesConfigAndArgv precedent) — $0, no real srt, no bwrap.
func TestComposeCreatesHandedWorkDirOnFirstUse(t *testing.T) {
	stub := writeSrtStub(t)
	co := NewComposer(settings.New(), nil)
	co.caps = Capabilities{Bwrap: true, Userns: true, Srt: true, SrtPath: stub}

	// Exactly the shell's shape: a state dir that exists, with the check-work
	// child that does NOT (shell.go: filepath.Join(stateDir, "check-work")).
	stateDir := t.TempDir()
	workDir := filepath.Join(stateDir, "check-work")
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("precondition: %q must not exist yet (%v)", workDir, err)
	}

	cmd, cleanup, err := co.Confine(
		adapters.StartRequest{RunID: "run-fresh", Class: string(C2), WorkDir: workDir},
		adapters.SpawnSpec{Argv: []string{"/bin/echo", "check"}, Env: []string{"PATH=/usr/bin:/bin"}, Workspace: t.TempDir()},
	)
	if err != nil {
		t.Fatalf("compose with a not-yet-created WorkDir must create it, got: %v", err)
	}
	defer cleanup()

	// The config landed inside the handed dir (not a temp-dir fallback).
	cfgPath := cmd.Args[indexOf(cmd.Args, "--settings")+1]
	if filepath.Dir(cfgPath) != workDir {
		t.Fatalf("srt config went to %q, want it inside the handed WorkDir %q", cfgPath, workDir)
	}

	// 0700, the state-dir hygiene the rest of the codebase keeps.
	fi, err := os.Stat(workDir)
	if err != nil {
		t.Fatalf("stat created WorkDir: %v", err)
	}
	if !fi.IsDir() {
		t.Fatalf("%q is not a directory", workDir)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Fatalf("created WorkDir perms %04o, want 0700 (state-dir hygiene)", perm)
	}

	// Second use of the now-existing dir still works (idempotent).
	cmd2, cleanup2, err := co.Confine(
		adapters.StartRequest{RunID: "run-fresh-2", Class: string(C2), WorkDir: workDir},
		adapters.SpawnSpec{Argv: []string{"/bin/echo", "check"}, Env: []string{"PATH=/usr/bin:/bin"}, Workspace: t.TempDir()},
	)
	if err != nil {
		t.Fatalf("second compose into the existing WorkDir: %v", err)
	}
	cleanup2()
	_ = cmd2
}

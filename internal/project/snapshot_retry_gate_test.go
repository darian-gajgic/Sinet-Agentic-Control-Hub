package project

// Companion pins for the snapshot's staging retry (Spec S02.4d, S13.5): the
// GATE, not the happy path. The retry exists only for git's die code — the
// vanishing-untracked-path class the engine's atomic writes produce — so any
// other non-zero exit and any spawn/OS failure must surface on the FIRST
// attempt, unchanged in kind. tq1_snapshot_race_test.go pins the retry itself.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// installExitCodeShim installs a `git` ahead of the real one on PATH that
// fails the platform's `add -A` with a fixed exit code, counting every
// interception; every other invocation execs the real git.
func installExitCodeShim(t *testing.T, code int) *gitShim {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("real git: %v", err)
	}
	dir := t.TempDir()
	counter := filepath.Join(dir, "attempts")
	script := fmt.Sprintf(`#!/bin/sh
prev=''
hit=0
for a in "$@"; do
  if [ "$prev" = 'add' ] && [ "$a" = '-A' ]; then hit=1; fi
  prev="$a"
done
if [ "$hit" = 1 ]; then
  n=0
  if [ -f '%[1]s' ]; then n=$(cat '%[1]s'); fi
  echo $((n + 1)) > '%[1]s'
  echo 'fatal: shim refuses to stage' >&2
  exit %[2]d
fi
exec '%[3]s' "$@"
`, counter, code, real)
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &gitShim{counter: counter}
}

// A non-128 exit is not the transient class: one attempt, then loud.
func TestSnapshotStagingNonDieExitIsNotRetried(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "task-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	f.writeFiles(ws.Path, map[string]string{"src/App.jsx": "export default function App() {}\n"})
	before := f.git(ws.Path, "rev-parse", "HEAD")
	shim := installExitCodeShim(t, 129)

	_, err = f.store.Snapshot(ctx, ws.Path)
	if err == nil {
		t.Fatal("a staging failure git did not die on must still fail LOUD")
	}
	if !strings.Contains(err.Error(), "exit 129") {
		t.Fatalf("the error must carry git's exit code: %v", err)
	}
	if n := shim.attempts(t); n != 1 {
		t.Fatalf("add -A attempts = %d, want 1 (only exit 128 is retried)", n)
	}
	if after := f.git(ws.Path, "rev-parse", "HEAD"); after != before {
		t.Fatalf("a failed snapshot committed: %s -> %s", before, after)
	}
}

// A spawn/OS failure is returned as it is, never folded into the retry bound.
func TestSnapshotStagingSpawnFailureIsNotRetried(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "task-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	f.writeFiles(ws.Path, map[string]string{"src/App.jsx": "export default function App() {}\n"})
	dead, cancel := context.WithCancel(ctx)
	cancel()

	_, err = f.store.Snapshot(dead, ws.Path)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a spawn failure must surface as itself, got: %v", err)
	}
	if strings.Contains(err.Error(), "attempts") {
		t.Fatalf("a spawn failure must not be reported as an exhausted retry bound: %v", err)
	}
}

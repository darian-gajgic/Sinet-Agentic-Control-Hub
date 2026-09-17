package project

// Companion pins for the snapshot's staging retry (Spec S02.4d, S13.5): the
// GATE and the reporting, not the happy path. The retry exists only for git's
// die code — the vanishing-untracked-path class the engine's atomic writes
// produce — so any other non-zero exit and any spawn/OS failure must surface
// on the FIRST attempt, unchanged in kind and unreported against a bound that
// never applied to them. tq1_snapshot_race_test.go pins the retry itself.

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
// fails the platform's `add -A` with a fixed exit code, writing message to
// stderr (nothing at all when it is empty) and counting every interception;
// every other invocation execs the real git.
func installExitCodeShim(t *testing.T, code int, message string) *gitShim {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("real git: %v", err)
	}
	dir := t.TempDir()
	counter := filepath.Join(dir, "attempts")
	say := ""
	if message != "" {
		say = fmt.Sprintf("  printf '%%s\\n' '%s' >&2\n", strings.ReplaceAll(message, "'", `'\''`))
	}
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
%[2]sexit %[3]d
fi
exec '%[4]s' "$@"
`, counter, say, code, real)
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &gitShim{counter: counter}
}

// stagingFixture is an active project with one produced file on disk and a
// workspace ready to snapshot.
func stagingFixture(t *testing.T) (*fix, Workspace) {
	t.Helper()
	f := newFix(t)
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, err := f.store.EnsureWorkspace(context.Background(), "shop", "task-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	f.writeFiles(ws.Path, map[string]string{"src/App.jsx": "export default function App() {}\n"})
	return f, ws
}

// A non-128 exit is not the transient class: one attempt, then loud — and it
// is never reported against a bound that did not apply to it.
func TestSnapshotStagingNonDieExitIsNotRetried(t *testing.T) {
	f, ws := stagingFixture(t)
	before := f.git(ws.Path, "rev-parse", "HEAD")
	shim := installExitCodeShim(t, 129, "fatal: shim refuses to stage")

	_, err := f.store.Snapshot(context.Background(), ws.Path)
	if err == nil {
		t.Fatal("a staging failure git did not die on must still fail LOUD")
	}
	if !strings.Contains(err.Error(), "exit 129") {
		t.Fatalf("the error must carry git's exit code: %v", err)
	}
	if strings.Contains(err.Error(), "attempts") {
		t.Fatalf("a class that is never retried must not be reported against the bound: %v", err)
	}
	if n := shim.attempts(t); n != 1 {
		t.Fatalf("add -A attempts = %d, want 1 (only exit 128 is retried)", n)
	}
	if after := f.git(ws.Path, "rev-parse", "HEAD"); after != before {
		t.Fatalf("a failed snapshot committed: %s -> %s", before, after)
	}
}

// A spawn/OS failure is returned as it is, never retried and never folded
// into the retry bound.
func TestSnapshotStagingSpawnFailureIsNotRetried(t *testing.T) {
	f, ws := stagingFixture(t)
	dead, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := f.store.Snapshot(dead, ws.Path)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a spawn failure must surface as itself, got: %v", err)
	}
	if strings.Contains(err.Error(), "attempts") {
		t.Fatalf("a spawn failure must not be reported as an exhausted retry bound: %v", err)
	}
}

// The past-the-bound error WRAPS the last git error (%w): the die line stays
// reachable through the chain, not merely flattened into a string, so a
// caller can inspect the cause and the operator reads git's words verbatim.
func TestSnapshotStagingBoundErrorWrapsTheGitError(t *testing.T) {
	f, ws := stagingFixture(t)
	installGitShim(t, ws.Path, 1<<30, nil, gitVanishMessage(recordedVanish))

	_, err := f.store.Snapshot(context.Background(), ws.Path)
	if err == nil {
		t.Fatal("a tree git cannot stage on every attempt must fail LOUD")
	}
	inner := errors.Unwrap(err)
	if inner == nil {
		t.Fatalf("the exhausted-bound error must wrap the last git error with %%w: %v", err)
	}
	if !strings.Contains(inner.Error(), "unable to stat '"+recordedVanish+"'") {
		t.Fatalf("the wrapped error must be git's own, verbatim: %v", inner)
	}
}

// A silent git leaves no dangling separator in the message.
func TestSnapshotStagingSilentFailureHasNoTrailingSeparator(t *testing.T) {
	f, ws := stagingFixture(t)
	installExitCodeShim(t, 129, "")

	_, err := f.store.Snapshot(context.Background(), ws.Path)
	if err == nil {
		t.Fatal("a staging failure must fail LOUD even when git said nothing")
	}
	if strings.HasSuffix(err.Error(), ": ") || strings.HasSuffix(err.Error(), ":") {
		t.Fatalf("empty git stderr left a dangling separator: %q", err.Error())
	}
}

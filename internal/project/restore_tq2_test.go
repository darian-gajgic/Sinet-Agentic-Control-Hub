package project

// restore_tq2_test.go — P3-TQ-2 grounding (committed RED, SKILL.md amendment
// A): the worktree state at fork-from-last-checkpoint.
//
// Spec S02.5 step 2: DEAD → "supersede by fork-from-last-checkpoint". Spec
// S02.4 (d): the Claude-lane checkpoint artifact ref is "a platform-owned
// snapshot commit in the run worktree". Spec S13.5: snapshot commits are the
// net; the run branch only ever fast-forwards (never amend / force / stash /
// jj); junk is excluded by platform ignore rules. CONVENTIONS §23 pins the
// fast-forward-only property structurally.
//
// The witnessed defect (P3/design/taskquality-webshop-findings-2026-09-16.md
// TQ-F2): the successor `execute.g1` re-drove the plan on the SAME worktree,
// dirty with the dead run's S-1..S-4 output. The rule this file pins: the
// fork's first session starts on a worktree whose tree IS the resume snapshot's
// tree — restored FORWARD as a new snapshot commit, so nothing the dead run
// snapshotted is lost (it stays reachable as an ancestor) and the branch never
// rewinds.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tq2Worktree builds an active project with one run worktree, takes the
// "step-1 close" snapshot, then lays down the dead run's partial step-2 work:
// a tracked file edited, a tracked file deleted, an untracked partial, ignored
// junk, one more per-call snapshot of that partial state, and finally residue
// that never reached any snapshot. Returns the worktree, the step-1 snapshot
// (the resume target) and the dead run's last per-call snapshot.
func tq2Worktree(t *testing.T, f *fix) (ws Workspace, stepOne, partial string) {
	t.Helper()
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "task-tq2")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	f.writeFiles(ws.Path, map[string]string{"step-1.txt": "one\n"})
	if stepOne, err = f.store.Snapshot(ctx, ws.Path); err != nil {
		t.Fatalf("step-1 close snapshot: %v", err)
	}
	// The dead run's partial step 2, as the per-call checkpoints saw it.
	f.writeFiles(ws.Path, map[string]string{
		"step-1.txt":     "one, rewritten by the interrupted step\n",
		"step-2.partial": "half of step two\n",
		"debug.log":      "junk the platform ignore rules exclude\n",
	})
	if err := os.Remove(filepath.Join(ws.Path, "main.go")); err != nil {
		t.Fatal(err)
	}
	if partial, err = f.store.Snapshot(ctx, ws.Path); err != nil {
		t.Fatalf("partial snapshot: %v", err)
	}
	if partial == stepOne {
		t.Fatal("fixture: the partial snapshot did not advance past the step-1 close")
	}
	// Residue after the last per-call snapshot: what a crash mid-step leaves
	// behind with no snapshot at all (the S-4/msg21 shape's tail).
	f.writeFiles(ws.Path, map[string]string{"step-2.uncommitted": "never snapshotted\n"})
	return ws, stepOne, partial
}

func TestTQ2RestoreSnapshotRestoresTreeForward(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	ws, stepOne, partial := tq2Worktree(t, f)

	head, err := f.store.RestoreSnapshot(ctx, ws.Path, stepOne)
	if err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}

	// (a) The tree IS the resume snapshot's tree — the invariant the fork's
	// first session starts on (Spec S02.4 (d) via S02.5 step 2).
	if got, want := f.git(ws.Path, "rev-parse", "HEAD^{tree}"), f.git(ws.Path, "rev-parse", stepOne+"^{tree}"); got != want {
		t.Errorf("HEAD tree %s != resume snapshot tree %s", got, want)
	}
	if got := f.git(ws.Path, "rev-parse", "HEAD"); got != head {
		t.Errorf("returned head %s but HEAD is %s", head, got)
	}
	// (b) Files: the tracked edit is undone, the deleted file is back, the
	// partial and the never-snapshotted residue are gone.
	if b, err := os.ReadFile(filepath.Join(ws.Path, "step-1.txt")); err != nil || string(b) != "one\n" {
		t.Errorf("step-1.txt = %q, %v; want the step-1 close content", b, err)
	}
	if _, err := os.Stat(filepath.Join(ws.Path, "main.go")); err != nil {
		t.Errorf("main.go (deleted by the interrupted step) was not restored: %v", err)
	}
	for _, gone := range []string{"step-2.partial", "step-2.uncommitted"} {
		if _, err := os.Stat(filepath.Join(ws.Path, gone)); err == nil {
			t.Errorf("%s survived the restore — the dirty tree must never be re-driven as-is", gone)
		}
	}
	// (c) Ignored junk is left alone: it was never in any snapshot, so it is
	// neither restored nor removed (Spec S13.5 junk rules are the excludes).
	if _, err := os.Stat(filepath.Join(ws.Path, "debug.log")); err != nil {
		t.Errorf("ignored junk debug.log was removed by the restore: %v", err)
	}
	// (d) Clean under the platform excludes: nothing staged, nothing untracked.
	code, out, _, err := f.store.gitRaw(ctx, ws.Path, platformIdentity,
		"-c", "core.excludesFile="+f.store.excludes, "status", "--porcelain")
	if err != nil || code != 0 {
		t.Fatalf("git status: code %d err %v", code, err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("worktree not clean after restore:\n%s", out)
	}
	// (e) Forward, never a rewind: the partial snapshot and the step-1 close
	// are both ANCESTORS of the new HEAD, the run branch moved with it, and
	// the discarded partial content is still reachable (nothing is lost).
	if head == partial || head == stepOne {
		t.Errorf("restore returned an existing commit %s — a restore over a changed tree must be a NEW snapshot commit", head)
	}
	for _, anc := range []string{stepOne, partial} {
		ok, err := f.store.isAncestor(ctx, ws.Path, anc, head)
		if err != nil || !ok {
			t.Errorf("%s is not an ancestor of the restored HEAD %s (err %v): the branch was rewound (Spec S13.5 fast-forward only)", anc, head, err)
		}
	}
	if got := f.git(ws.Path, "rev-parse", "refs/heads/sinet/run/task-tq2"); got != head {
		t.Errorf("run branch at %s, want the restored HEAD %s", got, head)
	}
	if code := f.gitCode(ws.Path, "cat-file", "-e", partial+":step-2.partial"); code != 0 {
		t.Errorf("the discarded partial content is no longer reachable under the dead run's snapshot %s", partial)
	}
	if an := f.git(ws.Path, "log", "-1", "--format=%an"); an != "Sinet platform" {
		t.Errorf("restore commit author = %q, want the platform identity", an)
	}
}

func TestTQ2RestoreSnapshotNoOpWhenTreeAlreadyMatches(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "task-tq2")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	f.writeFiles(ws.Path, map[string]string{"step-1.txt": "one\n"})
	stepOne, err := f.store.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	// A crash exactly at a step boundary leaves the tree equal to the last
	// close snapshot: the restore must return HEAD unchanged — never an
	// --allow-empty commit (Spec S13.5, the Snapshot no-change rule).
	head, err := f.store.RestoreSnapshot(ctx, ws.Path, stepOne)
	if err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}
	if head != stepOne {
		t.Errorf("restore over an already-matching tree returned %s, want the unchanged HEAD %s", head, stepOne)
	}
	if n := f.git(ws.Path, "rev-list", "--count", "HEAD"); n != f.git(ws.Path, "rev-list", "--count", stepOne) {
		t.Errorf("a no-op restore added a commit (rev-list count %s)", n)
	}
}

func TestTQ2RestoreSnapshotRefusesUnknownTarget(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})
	ws, err := f.store.EnsureWorkspace(ctx, "shop", "task-tq2")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	f.writeFiles(ws.Path, map[string]string{"step-1.txt": "one\n"})
	before, err := f.store.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	f.writeFiles(ws.Path, map[string]string{"residue.txt": "dirty\n"})

	// A target that resolves to nothing is refused LOUDLY and changes nothing:
	// HEAD, the run branch, and the working tree stay exactly as they were, and
	// no index.lock is left behind (the §72 loud-failure posture).
	const bogus = "0123456789abcdef0123456789abcdef01234567"
	if _, err := f.store.RestoreSnapshot(ctx, ws.Path, bogus); err == nil {
		t.Fatal("RestoreSnapshot accepted a target that does not exist")
	} else if strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("RestoreSnapshot is still the grounding stub: %v", err)
	}
	if got := f.git(ws.Path, "rev-parse", "HEAD"); got != before {
		t.Errorf("HEAD moved to %s on a refused restore (was %s)", got, before)
	}
	if _, err := os.Stat(filepath.Join(ws.Path, "residue.txt")); err != nil {
		t.Errorf("a refused restore altered the working tree: %v", err)
	}
	// A worktree's .git is a FILE pointer (CONVENTIONS §59): resolve the real
	// git path rather than probing under a file.
	lock := f.git(ws.Path, "rev-parse", "--git-path", "index.lock")
	if !filepath.IsAbs(lock) {
		lock = filepath.Join(ws.Path, lock)
	}
	if _, err := os.Stat(lock); err == nil {
		t.Errorf("index.lock left behind by a refused restore at %s", lock)
	}
}

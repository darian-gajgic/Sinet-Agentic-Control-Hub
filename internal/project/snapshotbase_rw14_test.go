package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// P3-RW-14 R7 — the two durable facts the S07.2 wrote-nothing verdict rests
// on, read against a REAL repository.
//
// The live defect: an execute leg ran 7 stage sessions, reported every one
// "completed", billed $1.11, and left the worktree holding nothing but .git —
// every checkpoint pinned the store-init base commit. The platform could have
// known at $0, because both terms were already durable: where the tree stands,
// and where it started. This binds that read — including that ASKING must not
// CHANGE the answer, which is why it is not Snapshot().

func TestRW14SnapshotAndBaseReadsWithoutWriting(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})

	ws, err := f.store.EnsureWorkspace(ctx, "shop", "t-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}

	// Nothing written yet: the tree still stands exactly on the recorded base
	// — the shape that must read as "wrote nothing".
	snapshot, base, err := f.store.SnapshotAndBase(ctx, "shop", "t-1")
	if err != nil {
		t.Fatalf("SnapshotAndBase: %v", err)
	}
	if base == "" {
		t.Fatal("no recorded base for the attempt — the gate would have no basis to compare against")
	}
	if snapshot != base {
		t.Fatalf("snapshot %q != base %q on an untouched worktree", snapshot, base)
	}
	if base != ws.Base {
		t.Errorf("base %q is not the workspace's own recorded base %q", base, ws.Base)
	}

	// Reading is not writing: the answer must be stable under repetition, and
	// no commit may appear because verification asked a question. Snapshot()
	// would stage and commit here; that is the whole reason this exists.
	countCommits := func() string { return f.git(ws.Path, "rev-list", "--count", "HEAD") }
	before := countCommits()
	for i := 0; i < 3; i++ {
		s2, b2, err := f.store.SnapshotAndBase(ctx, "shop", "t-1")
		if err != nil {
			t.Fatalf("SnapshotAndBase repeat: %v", err)
		}
		if s2 != snapshot || b2 != base {
			t.Fatalf("repeat %d changed the answer: %q/%q vs %q/%q", i, s2, b2, snapshot, base)
		}
	}
	if after := countCommits(); after != before {
		t.Fatalf("reading the facts created commits (%s -> %s) — the platform would be manufacturing the evidence it judges", before, after)
	}
	// Uncommitted work must not move the snapshot either: what the gate reads
	// is the committed tree the execute leg's stage-close snapshot produced.
	if err := os.WriteFile(filepath.Join(ws.Path, "scratch.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s3, _, err := f.store.SnapshotAndBase(ctx, "shop", "t-1"); err != nil || s3 != snapshot {
		t.Fatalf("an uncommitted file moved the read snapshot: %q (err %v)", s3, err)
	}

	// Real work, committed the way the execute leg's stage close commits it:
	// the snapshot leaves the base behind and the gate must fall silent.
	if _, err := f.store.Snapshot(ctx, ws.Path); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	moved, base2, err := f.store.SnapshotAndBase(ctx, "shop", "t-1")
	if err != nil {
		t.Fatalf("SnapshotAndBase after work: %v", err)
	}
	if moved == base2 {
		t.Fatal("the tree moved but snapshot still equals base — real work would be killed as wrote-nothing")
	}
	if base2 != base {
		t.Errorf("the recorded base moved under us: %q -> %q", base, base2)
	}
}

// A task with no worktree (never project-backed, or execute never made one)
// answers with honest emptiness rather than a guess — the gate declines to
// fire, which is the difference between "no evidence" and "evidence of guilt".
func TestRW14SnapshotAndBaseIsEmptyWithoutAWorktree(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.activeProject("shop", "alice", "shop", map[string]string{"main.go": "package main\n"})

	snapshot, base, err := f.store.SnapshotAndBase(ctx, "shop", "t-never-ran")
	if err != nil {
		t.Fatalf("SnapshotAndBase: %v", err)
	}
	if snapshot != "" || base != "" {
		t.Fatalf("invented facts for a task with no workspace: snapshot=%q base=%q", snapshot, base)
	}
}

package shell

// restorefork_tq2_test.go — P3-TQ-2 §7 (executor-added): the composition half
// of the fork's worktree restore.
//
// `RestoreWorkspace` is the seam `stage.dispatchExecute` calls once, before a
// recovery successor's seed and first session, to put the task worktree back on
// the tree of the last completed step's snapshot (Spec S02.5 step 2, S02.4 (d),
// S13.5). Two properties are the shell's own, and neither is reachable from the
// stage package: a fork id `<task>.execute.g1` must resolve to the SAME worktree
// its parent had (the P3-RW-6 lane rule, CONVENTIONS §55), and the seam must
// resolve an EXISTING worktree only — never create one, the F4 rule `Snapshot`
// already holds, so a run with no worktree answers an honest absence instead of
// materializing a workspace nothing asked for.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

func TestTQ2RestoreWorkspaceResolvesTheForksExistingWorktree(t *testing.T) {
	db, log, reg := seamDB(t)
	ctx := context.Background()
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	if _, _, err := proj.Onboard(ctx, project.OnboardInput{ProjectID: "shop", Owner: "alice", Name: "shop"}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if _, err := proj.Approve(ctx, "shop", "alice", nil); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	runs := run.NewStore(db, log)
	pipe := &intake.Pipeline{
		DB: db, Log: log, Runs: runs, Ledger: ledger.NewStore(db, log), Settings: reg,
		ArtifactRoot: filepath.Join(t.TempDir(), "artifacts"),
		Registry:     registrySeam{proj: proj},
	}
	start := func(taskID string) {
		t.Helper()
		st, err := pipe.Start(ctx, intake.Request{
			TaskID: taskID, UserID: "alice", Title: "change the shop",
			Text: "adjust the checkout copy", Project: "shop",
		})
		if err != nil {
			t.Fatalf("intake Start %s: %v", taskID, err)
		}
		if st.Registry == nil || st.Registry.Project != "shop" {
			t.Fatalf("%s carries no project match: %+v", taskID, st.Registry)
		}
	}
	start("t-rs")
	start("t-rs-untouched")
	// A task with no project at all: the workspace-less lane.
	if _, err := pipe.Start(ctx, intake.Request{
		TaskID: "t-rs-plain", UserID: "alice", Title: "think about it", Text: "no repo here",
	}); err != nil {
		t.Fatalf("intake Start (no project): %v", err)
	}
	for id, taskID := range map[string]string{
		"t-rs.execute":              "t-rs",
		"t-rs.execute.g1":           "t-rs",
		"t-rs-untouched.execute.g1": "t-rs-untouched",
		"t-rs-plain.execute.g1":     "t-rs-plain",
	} {
		if _, err := runs.Create(ctx, run.NewRun{ID: id, UserID: "alice", TaskID: taskID}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	ps := &projectSeams{proj: proj, runs: runs, db: db, pipe: pipe}

	// The parent's worktree, with a step-close snapshot and then a partial step
	// on top of it — the shape a dead run leaves behind.
	parentPath, ok, err := ps.WorkspaceCwd(ctx, "t-rs.execute")
	if err != nil || !ok || parentPath == "" {
		t.Fatalf("WorkspaceCwd(execute) = %q, %v, %v — want the task worktree", parentPath, ok, err)
	}
	writeSeamFile(t, parentPath, "step-1.txt", "one\n")
	stepOne, err := proj.Snapshot(ctx, parentPath)
	if err != nil {
		t.Fatalf("step-1 close snapshot: %v", err)
	}
	writeSeamFile(t, parentPath, "step-2.partial", "half of step two\n")
	if _, err := proj.Snapshot(ctx, parentPath); err != nil {
		t.Fatalf("partial snapshot: %v", err)
	}

	// The fork resolves its PARENT's worktree and restores it there.
	head, err := ps.RestoreWorkspace(ctx, "t-rs.execute.g1", stepOne)
	if err != nil {
		t.Fatalf("RestoreWorkspace(fork): %v", err)
	}
	if head == "" {
		t.Fatal("the fork's restore answered an absence — it did not find the worktree its parent had (CONVENTIONS §55)")
	}
	if _, err := os.Stat(filepath.Join(parentPath, "step-1.txt")); err != nil {
		t.Errorf("the finished step's file is not in the parent's worktree after the fork's restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parentPath, "step-2.partial")); err == nil {
		t.Error("the interrupted step's partial survived in the parent's worktree — the fork restored somewhere else")
	}

	// EXISTING only: a run whose worktree was never created gets an honest
	// absence, and no workspace is materialized behind its back (F4).
	got, err := ps.RestoreWorkspace(ctx, "t-rs-untouched.execute.g1", stepOne)
	if err != nil || got != "" {
		t.Errorf("RestoreWorkspace on a task with no worktree = %q, %v; want \"\", nil", got, err)
	}
	if _, created, err := proj.ExistingWorkspace(ctx, "shop", "t-rs-untouched"); err != nil || created {
		t.Errorf("the restore seam created a worktree (%v, %v) — it resolves, never materializes", created, err)
	}

	// A workspace-less run has no project to restore in.
	if got, err := ps.RestoreWorkspace(ctx, "t-rs-plain.execute.g1", stepOne); err != nil || got != "" {
		t.Errorf("RestoreWorkspace on a workspace-less run = %q, %v; want \"\", nil", got, err)
	}
}

func writeSeamFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

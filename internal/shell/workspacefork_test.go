package shell

// workspacefork_test.go — P3-RW-6 §7 T-A6 (checklist 9): a recovery fork of an
// execute run keeps its project workspace lane.
//
// `WorkspaceCwd` gates on the execute role (F4: intake/verify/compose ceremony
// sessions get no worktree). The gate matched the run id's raw suffix, so a fork
// id `<task>.execute.g1` — a legal S02.5 successor — failed the gate and the
// resumed execution silently lost its worktree and snapshot lane, writing into
// the plain run dir instead. The lane is per (project, task), so the fork must
// answer with the SAME worktree its parent had.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

func TestExecuteForkKeepsWorkspaceLane(t *testing.T) {
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
	// A task PINNED to the registered project: its durable intake match is what
	// resolves the workspace (never a re-match).
	st, err := pipe.Start(ctx, intake.Request{
		TaskID: "t-ws", UserID: "alice", Title: "change the shop",
		Text: "adjust the checkout copy", Project: "shop",
	})
	if err != nil {
		t.Fatalf("intake Start: %v", err)
	}
	if st.Registry == nil || st.Registry.Project != "shop" {
		t.Fatalf("intake state carries no project match: %+v", st.Registry)
	}
	ps := &projectSeams{proj: proj, runs: runs, db: db, pipe: pipe}

	for _, id := range []string{"t-ws.execute", "t-ws.execute.g1", "t-ws.intake.g1"} {
		if _, err := runs.Create(ctx, run.NewRun{ID: id, UserID: "alice", TaskID: "t-ws"}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}

	parentPath, ok, err := ps.WorkspaceCwd(ctx, "t-ws.execute")
	if err != nil || !ok || parentPath == "" {
		t.Fatalf("WorkspaceCwd(execute) = %q, %v, %v — want the task worktree", parentPath, ok, err)
	}
	forkPath, ok, err := ps.WorkspaceCwd(ctx, "t-ws.execute.g1")
	if err != nil {
		t.Fatalf("WorkspaceCwd(execute fork): %v", err)
	}
	if !ok {
		t.Fatal("the execute FORK was refused a workspace — it lost its worktree lane (R12)")
	}
	if forkPath != parentPath {
		t.Fatalf("fork worktree = %q, want the parent's %q (the lane is per project+task)", forkPath, parentPath)
	}
	// The gate still refuses non-execute roles, fork suffix or not.
	if _, ok, err := ps.WorkspaceCwd(ctx, "t-ws.intake.g1"); ok || err != nil {
		t.Fatalf("WorkspaceCwd(intake fork) = %v, %v — ceremony legs get no worktree (F4)", ok, err)
	}
}

package stage

// verifycwd_internal_test.go — P3-RW-6 drain r1 F1: the verification checks run
// in the cwd of the run that ACTUALLY produced the deliverable.
//
// Work dirs are per run id, so after a recovery fork of the execute leg
// (`<task>.execute.g1`) the deliverable was produced in the FORK's cwd while
// verifyInput handed the checks the superseded parent's — stale where it exists
// at all. The fix reads the current identity of the execute lineage
// (`runs.parent_run_id`) rather than re-deriving it from the task id, which is
// the ratified rule this packet records; a suffix strip cannot help here,
// because it removes a fork suffix and cannot reconstruct one.
//
// This is an INTERNAL test because verifyInput is internal: the assertion is
// about the directory handed to the check runner, not about a public surface.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// ---- the two S06.10 seams, faked (no engine is spawned by this test) ----

type cwdPlanner struct{}

func (cwdPlanner) Draft(_ context.Context, in intake.DraftInput) (intake.Pair, error) {
	return cwdPair(in.Request.TaskID), nil
}

func (cwdPlanner) Revise(_ context.Context, in intake.ReviseInput) (intake.Pair, error) {
	return in.Pair, nil
}

type cwdCritic struct{}

func (cwdCritic) Critique(context.Context, intake.Pair) (intake.Verdict, error) {
	return intake.Verdict{Kind: intake.VerdictPass}, nil
}

func (cwdCritic) Recheck(context.Context, intake.Pair, []string) (intake.Verdict, error) {
	return intake.Verdict{Kind: intake.VerdictPass}, nil
}

func cwdPair(taskID string) intake.Pair {
	return intake.Pair{
		Spec: intake.Spec{
			Provenance: "cwd-fake v1", Restatement: "produce a note for " + taskID,
			Outcome: []string{"a note exists"},
			ACs: []intake.AC{
				{N: 1, Plain: "the note exists", Structured: "WHEN read THEN it is non-empty", StructuredKind: "ears"},
			},
			Constraints: []string{"stay in the repo"},
			OutOfScope:  []string{"no deploys"},
		},
		Plan: intake.Plan{
			Provenance: "cwd-fake v1",
			Steps: []intake.Step{
				{ID: "S-1", Title: "Write the note", DoneWhen: "the note exists", Class: "C1", WriteSet: []string{"src/**"}},
			},
			Coverage: map[string][]string{"AC-1": {"S-1"}},
			Est:      intake.Estimate{SizeClass: "S", USD: 1.0, Known: true, Basis: "fake"},
		},
	}
}

// approvedSkeleton builds a skeleton whose task has an APPROVED plan and a
// persisted deliverable revision 1 — everything verifyInput reads before it
// resolves the execution cwd.
func approvedSkeleton(t *testing.T) (*Skeleton, *run.Store, string, string) {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	runs := run.NewStore(db, log)
	root := t.TempDir()
	s, err := New(Config{
		DB: db, Log: log, Runs: runs, Checkpoints: gates.NewCheckpoints(db, log),
		Ledger: ledger.NewStore(db, log), Settings: reg,
		Adapters: map[string]adapters.Adapter{
			// Registered because New requires one; nothing in this test spawns a
			// session (both model seams are fakes above).
			adapters.SubstrateClaudeCLI: &claudecli.Adapter{Binary: "/nonexistent", Settings: reg},
		},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
		Planner:      cwdPlanner{},
		Critic:       cwdCritic{},
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}

	const owner = "u-cwd"
	st, err := s.pipe.Start(ctx, intake.Request{
		UserID: owner, Title: "note", Text: "write a short note about SQLite",
	})
	if err != nil {
		t.Fatalf("intake Start: %v", err)
	}
	// Admission is the scheduler's; the dev harness walks the ratified edges.
	for _, to := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, st.RunID, to, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("admit %s: %v", to, err)
		}
	}
	st, err = s.pipe.Advance(ctx, st.TaskID)
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if st.OpenAskID == "" {
		t.Fatal("expected an open interview card")
	}
	if st, err = s.pipe.Answer(ctx, owner, st.OpenAskID, json.RawMessage(`{"force_proceed":true}`)); err != nil {
		t.Fatalf("Answer(force_proceed): %v", err)
	}
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("expected the approval card, got %q", st.OpenAskKind)
	}
	if st, err = s.pipe.Answer(ctx, owner, st.OpenAskID, json.RawMessage(`{"action":"approve"}`)); err != nil {
		t.Fatalf("Answer(approve): %v", err)
	}
	if st.Phase != intake.PhaseApproved {
		t.Fatalf("phase = %s, want approved", st.Phase)
	}
	if _, _, err := s.writeDeliverable(st.TaskID, 1, "a short note about SQLite\n"); err != nil {
		t.Fatalf("writeDeliverable: %v", err)
	}
	return s, runs, st.TaskID, owner
}

// mkExecuteRun creates an execute-lineage run row and seeds its cwd with a file
// naming it, so the directory handed to the checks is identifiable by content.
func mkExecuteRun(t *testing.T, s *Skeleton, runs *run.Store, id, parent, taskID, owner string) string {
	t.Helper()
	if _, err := runs.Create(context.Background(), run.NewRun{
		ID: id, UserID: owner, TaskID: taskID, ParentRunID: parent,
	}); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
	cwd := filepath.Join(s.cfg.RunRoot, sanitizePathComponent(id), "cwd")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", cwd, err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "produced-by.txt"), []byte(id), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	return cwd
}

func TestVerifyInputUsesTheProducingExecuteRunCwd(t *testing.T) {
	ctx := context.Background()
	s, runs, taskID, owner := approvedSkeleton(t)

	parentCwd := mkExecuteRun(t, s, runs, taskID+RunSuffixExecute, "", taskID, owner)
	forkCwd := mkExecuteRun(t, s, runs, taskID+RunSuffixExecute+".g1", taskID+RunSuffixExecute, taskID, owner)

	in, err := s.verifyInput(ctx, taskID+RunSuffixVerify, taskID, 1)
	if err != nil {
		t.Fatalf("verifyInput: %v", err)
	}
	if in.Workspace != forkCwd {
		t.Fatalf("checks run in %q, want the FORK's cwd %q (the run that produced the deliverable)", in.Workspace, forkCwd)
	}
	marker, err := os.ReadFile(filepath.Join(in.Workspace, "produced-by.txt"))
	if err != nil {
		t.Fatalf("read marker in the workspace handed to the checks: %v", err)
	}
	if got := string(marker); got != taskID+RunSuffixExecute+".g1" {
		t.Fatalf("the workspace was produced by %q, want the fork", got)
	}
	if in.Workspace == parentCwd {
		t.Fatal("the checks were handed the superseded parent's stale cwd")
	}

	// A SECOND fork moves the answer again — the lineage is walked, not guessed.
	deepCwd := mkExecuteRun(t, s, runs, taskID+RunSuffixExecute+".g1.g2", taskID+RunSuffixExecute+".g1", taskID, owner)
	in, err = s.verifyInput(ctx, taskID+RunSuffixVerify, taskID, 1)
	if err != nil {
		t.Fatalf("verifyInput (deep): %v", err)
	}
	if in.Workspace != deepCwd {
		t.Fatalf("checks run in %q, want the newest fork's cwd %q", in.Workspace, deepCwd)
	}
}

func TestVerifyInputNonForkPathUnchanged(t *testing.T) {
	ctx := context.Background()
	s, runs, taskID, owner := approvedSkeleton(t)

	// The ordinary shape: one execute run, no lineage beyond it.
	parentCwd := mkExecuteRun(t, s, runs, taskID+RunSuffixExecute, "", taskID, owner)
	in, err := s.verifyInput(ctx, taskID+RunSuffixVerify, taskID, 1)
	if err != nil {
		t.Fatalf("verifyInput: %v", err)
	}
	if in.Workspace != parentCwd {
		t.Fatalf("non-fork workspace = %q, want %q (byte-equivalent to the pre-drain composition)", in.Workspace, parentCwd)
	}

	// And a task whose execute run does not exist yet still answers with the
	// birth composition — exactly what the old code returned.
	want := filepath.Join(s.cfg.RunRoot, sanitizePathComponent("t-absent"+RunSuffixExecute), "cwd")
	if got := filepath.Join(s.cfg.RunRoot, sanitizePathComponent(s.executeRunID(ctx, "t-absent")), "cwd"); got != want {
		t.Fatalf("absent-lineage cwd = %q, want the birth composition %q", got, want)
	}
}

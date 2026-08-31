package stage

// gf14detach_internal_test.go — P3-GF14 R1 at the stage layer's own seam: the
// answer CONTINUATION. `afterIntake` is the leg that turns an approved plan
// into a completed intake run and a launched execution; it runs on the run the
// answer just resumed, so a requester whose page went away mid-approval must
// not be left with a `running` intake run and no execution — the same strand
// the drive's detach ends one layer up.
//
// The detach lives inside the seam (the §56 doctrine `crash` established), so
// the proof is the seam itself: entered with a context that is already dead,
// every write still lands.
//
// $0: no engine, no session, no network.

import (
	"context"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

func TestAfterIntakeSurvivesCanceledContext(t *testing.T) {
	e := newCancelEnv(t)
	const taskID, owner = "t-gf14-continuation", "u-gf14"
	runID := taskID + ".intake"
	e.seedRun(t, runID, taskID, owner, run.StateRunning)

	st := &intake.State{
		Phase: intake.PhaseApproved, TaskID: taskID, RunID: runID, Owner: owner,
	}
	if err := e.sk.afterIntake(deadContext(), st); err != nil {
		t.Fatalf("afterIntake on a dead context: %v", err)
	}

	if got := e.state(t, runID); got != run.StateCompleted {
		t.Fatalf("intake run is %s, want completed — the approved plan's ending outlives its request", got)
	}
	if got := e.kanban(t, taskID); got != "executing" {
		t.Fatalf("kanban = %q, want executing", got)
	}
	execRun, err := e.runs.Get(context.Background(), taskID+".execute")
	if err != nil {
		t.Fatalf("the execution run was never launched: %v", err)
	}
	if execRun.State != run.StateQueued {
		t.Fatalf("execution run is %s, want queued — admitted, not merely created", execRun.State)
	}
}

func TestAfterIntakeParkedGateSurvivesCanceledContext(t *testing.T) {
	e := newCancelEnv(t)
	const taskID, owner = "t-gf14-gate", "u-gf14"
	runID := taskID + ".intake"
	e.seedRun(t, runID, taskID, owner, run.StateParked)

	st := &intake.State{
		Phase: intake.PhaseInterview, TaskID: taskID, RunID: runID, Owner: owner,
		OpenAskID: "intake:" + taskID + ":1", OpenAskKind: intake.CardInterview,
	}
	if err := e.sk.afterIntake(deadContext(), st); err != nil {
		t.Fatalf("afterIntake on a dead context: %v", err)
	}
	if got := e.state(t, runID); got != run.StateParked {
		t.Fatalf("run is %s, want still parked — an open gate waits, whatever the caller did", got)
	}
}

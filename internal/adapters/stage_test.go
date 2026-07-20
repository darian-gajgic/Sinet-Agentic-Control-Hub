package adapters_test

// DriveStage (B2-4, Spec S05.3): a stage session on an already-running run
// persists events + checkpoints and NEVER touches the FSM; the OnEvent
// observer fires per persisted event.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

func TestDriveStageRequiresRunningRun(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.claimedRun(t, "sr1") // claimed, not running
	a := &fakeAdapter{outcome: adapters.Outcome{Kind: adapters.OutcomeCompleted}}
	_, err := e.drv.DriveStage(ctx, a, adapters.StartRequest{RunID: "sr1", UserID: "u1"})
	if !errors.Is(err, adapters.ErrNotDrivable) {
		t.Fatalf("err = %v, want ErrNotDrivable (checkpoints are writable only in running/draining, S02.4)", err)
	}
	if a.started != nil {
		t.Fatal("engine spawned for a non-running run")
	}
}

func TestDriveStagePersistsWithoutFSMTransition(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.claimedRun(t, "sr2")
	if _, err := e.runs.Transition(ctx, "sr2", run.StateRunning, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
		t.Fatalf("to running: %v", err)
	}
	usage := &adapters.Usage{ModelID: "m-test", InputTokens: 10, OutputTokens: 2, MessageID: "msg1", MessageIndex: 0}
	a := &fakeAdapter{
		script: []adapters.Event{
			{Kind: adapters.KindMessage, Payload: json.RawMessage(`{"excerpt":"hi"}`)},
			{Kind: adapters.KindUsage, Usage: usage, Payload: json.RawMessage(`{"u":1}`)},
			{Kind: adapters.KindDone, Payload: json.RawMessage(`{"done":true}`)},
		},
		cursor:  adapters.Cursor{Substrate: adapters.SubstrateClaudeCLI, SessionID: "sess-1"},
		outcome: adapters.Outcome{Kind: adapters.OutcomeCompleted, ResultText: "full text"},
	}
	var observed []adapters.EventKind
	out, err := e.drv.DriveStage(ctx, a, adapters.StartRequest{
		RunID: "sr2", UserID: "u1",
		OnEvent: func(ev adapters.Event) { observed = append(observed, ev.Kind) },
	})
	if err != nil {
		t.Fatalf("DriveStage: %v", err)
	}
	if out.Kind != adapters.OutcomeCompleted || out.ResultText != "full text" {
		t.Fatalf("outcome = %+v", out)
	}
	// No FSM transition: the run stays running — the caller owns the
	// run-level disposition (Spec S05.3 stage sessions within one run).
	r, _ := e.runs.Get(ctx, "sr2")
	if r.State != run.StateRunning {
		t.Fatalf("run state %s after stage session, want running", r.State)
	}
	// The paid call checkpointed against the running run.
	if cp, ok, err := e.cps.Last(ctx, "sr2"); err != nil || !ok || cp.SessionID != "sess-1" {
		t.Fatalf("checkpoint: ok=%v err=%v cp=%+v", ok, err, cp)
	}
	// The observer saw every persisted event, in order.
	want := []adapters.EventKind{adapters.KindMessage, adapters.KindUsage, adapters.KindDone}
	if len(observed) != len(want) {
		t.Fatalf("observed %v, want %v", observed, want)
	}
	for i := range want {
		if observed[i] != want[i] {
			t.Fatalf("observed %v, want %v", observed, want)
		}
	}
}

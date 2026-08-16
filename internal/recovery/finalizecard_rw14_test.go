package recovery_test

// finalizecard_rw14_test.go — P3-RW-14 Cut A, brief spec 4 (the finalize arm
// of R1/R2) plus the PROPERTY the brief marks as spec-stated: no ladder
// terminal without an open ask in the same transition.
//
// Spec S02.3 names the terminal "finalized (finalize-with-card)" and S02.5
// step 7 "tombstone-review cards". Every branch of the step-3 bound decision
// that ENDS a lineage owes its door; the branch that does NOT end one (the
// ordinary fork) owes nothing — the ladder is still driving that work, and a
// card there would be an inbox item for something nobody has to decide.

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// seedTaskRun creates a task at the given kanban column plus one run of it,
// driven to running through the ratified edges (the mustRunning shape, with
// the task link the door needs).
func seedTaskRun(t *testing.T, s *stack, taskID, runID, kanban string) run.Run {
	t.Helper()
	ctx := context.Background()
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks(task_id, user_id, title, kanban_status, created_ts)
			 VALUES (?, 'u1', 'door probe', ?, '2026-08-16T00:00:00Z')`, taskID, kanban)
		return err
	})
	if err != nil {
		t.Fatalf("seed task %s: %v", taskID, err)
	}
	if _, err := s.runs.Create(ctx, run.NewRun{
		ID: runID, UserID: "u1", TaskID: taskID, Substrate: "mock", Lane: "mock",
	}); err != nil {
		t.Fatalf("create run %s: %v", runID, err)
	}
	for _, to := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := s.runs.Transition(ctx, runID, to, run.TransitionOptions{}); err != nil {
			t.Fatalf("→%s: %v", to, err)
		}
	}
	return getRun(t, s, runID)
}

// openAsks counts the OPEN asks joined to a task's runs — the door as the
// operator's inbox sees it.
func openAsks(t *testing.T, s *stack, taskID string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM asks a JOIN runs r ON r.run_id = a.run_id
		  WHERE r.task_id = ? AND a.status = 'open'`, taskID).Scan(&n); err != nil {
		t.Fatalf("count open asks: %v", err)
	}
	return n
}

func kanbanOf(t *testing.T, s *stack, taskID string) string {
	t.Helper()
	var k string
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT kanban_status FROM tasks WHERE task_id = ?`, taskID).Scan(&k); err != nil {
		t.Fatalf("read kanban: %v", err)
	}
	return k
}

// TestRW14FinalizeWithCardLeavesAnAnswerableDoor: the stale-interruption
// finalize (the TestStaleInterruptionFinalized setup plus a task row) is a
// LINEAGE ENDING — Spec S02.3 calls it finalize-with-card — so it owes the
// same durable, answerable door the tombstone owes (R1 finalize arm + R2).
func TestRW14FinalizeWithCardLeavesAnAnswerableDoor(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	seedTaskRun(t, s, "t-rw14-fin", "t-rw14-fin.execute", "executing")

	clock := &fakeClock{t: time.Now().Add(25 * time.Hour)} // past ⚙ recovery.stale_finalize
	rpt, err := newLadder(t, s, clock, nil, nil).ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.Finalized != 1 {
		t.Fatalf("report = %+v, want 1 finalized", rpt)
	}
	if r := getRun(t, s, "t-rw14-fin.execute"); r.State != run.StateFinalized {
		t.Fatalf("state = %s, want finalized", r.State)
	}
	if got := openAsks(t, s, "t-rw14-fin"); got == 0 {
		t.Fatal("finalize-with-card left NO open ask — the terminal is a card (Spec S02.3), not a payload string")
	}
	if got := kanbanOf(t, s, "t-rw14-fin"); got != "attention" {
		t.Fatalf("kanban = %q after finalize, want %q (S15.5 blocked-on-a-human)", got, "attention")
	}

	// The card is a full reconstruction snapshot (Spec S02.2): it names the
	// lineage, the ground, and the two verbs a human can answer with.
	var snapshot string
	if err := s.db.QueryRowContext(ctx,
		`SELECT snapshot FROM asks WHERE ask_id = ?`,
		recovery.LadderAskID("t-rw14-fin.execute")).Scan(&snapshot); err != nil {
		t.Fatalf("read card: %v", err)
	}
	var card recovery.LadderCard
	if err := json.Unmarshal([]byte(snapshot), &card); err != nil {
		t.Fatalf("decode card: %v", err)
	}
	switch {
	case card.Card != recovery.CardFinalizeWithCard:
		t.Fatalf("card kind = %q, want %q", card.Card, recovery.CardFinalizeWithCard)
	case card.TaskID != "t-rw14-fin" || card.RunID != "t-rw14-fin.execute":
		t.Fatalf("card identity = %+v", card)
	case card.LineageBirthRunID != "t-rw14-fin.execute":
		t.Fatalf("lineage birth = %q", card.LineageBirthRunID)
	case card.Ground == "" || card.Summary == "":
		t.Fatalf("card carries no ground/summary: %+v", card)
	case len(card.Choices) != 2 || card.Choices[0] != recovery.VerbRetry || card.Choices[1] != recovery.VerbCancel:
		t.Fatalf("card choices = %v, want [retry cancel] (OQ2)", card.Choices)
	case card.SLA.Class != "approval" || card.SLA.RemindAfterHours == 0 || card.SLA.PushAfterHours == 0:
		t.Fatalf("card SLA = %+v, want the approval-class ⚙ numbers (OQ5)", card.SLA)
	}
}

// TestRW14EveryLadderTerminalLeavesADoor is the property, not an example:
// EVERY branch of the S02.5 step-3 bound decision that ends a lineage leaves
// exactly one open ask and moves the task to "attention"; the branch that
// keeps the lineage alive (fork) leaves neither.
func TestRW14EveryLadderTerminalLeavesADoor(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name     string
		ends     bool // a LINEAGE ENDING (owes a door) vs a fork (owes none)
		attempts int64
		after    time.Duration
		noFork   bool
		want     run.State
	}{
		{name: "tombstone-repeat-offender", ends: true, attempts: 3, after: 10 * time.Minute, want: run.StateTombstoned},
		{name: "stale-finalize", ends: true, after: 25 * time.Hour, want: run.StateFinalized},
		{name: "no-fork-class-finalize", ends: true, after: 10 * time.Minute, noFork: true, want: run.StateFinalized},
		{name: "ordinary-fork", ends: false, after: 10 * time.Minute, want: run.StateCrashed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newStack(t)
			taskID := "t-" + c.name
			runID := taskID + ".execute"
			seedTaskRun(t, s, taskID, runID, "executing")
			if c.attempts > 0 {
				err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
					_, err := tx.ExecContext(ctx,
						`UPDATE runs SET recovery_attempts = ? WHERE run_id = ?`, c.attempts, runID)
					return err
				})
				if err != nil {
					t.Fatalf("seed attempts: %v", err)
				}
			}
			clock := &fakeClock{t: time.Now().Add(c.after)}
			l := newLadder(t, s, clock, nil, nil)
			if c.noFork {
				l = noForkLadderAt(t, s, func(string) bool { return true }, clock)
			}
			if _, err := l.ReconcilePass(ctx); err != nil {
				t.Fatalf("ReconcilePass: %v", err)
			}
			if got := getRun(t, s, runID); got.State != c.want {
				t.Fatalf("state = %s, want %s", got.State, c.want)
			}
			asks, kanban := openAsks(t, s, taskID), kanbanOf(t, s, taskID)
			if c.ends {
				if asks != 1 {
					t.Fatalf("open asks = %d, want exactly 1 — a lineage ending owes exactly one door (S02.5 step 7 / S02.3)", asks)
				}
				if kanban != "attention" {
					t.Fatalf("kanban = %q, want attention — the board must not show a task working while nothing drives it", kanban)
				}
				return
			}
			if asks != 0 {
				t.Fatalf("open asks = %d — a FORKED lineage is still being driven; it owes no human decision", asks)
			}
			if kanban != "executing" {
				t.Fatalf("kanban = %q, want the untouched stage column — a fork is not a human's problem", kanban)
			}
		})
	}
}

// TestRW14TerminalDoorNeverOverwritesAFinishedTask: a ladder terminal on a
// leftover run of a task that already ended (done / cancelled) never rewrites
// how it ended (R2's explicit exclusion).
func TestRW14TerminalDoorNeverOverwritesAFinishedTask(t *testing.T) {
	ctx := context.Background()
	for _, column := range []string{"done", "cancelled"} {
		t.Run(column, func(t *testing.T) {
			s := newStack(t)
			taskID, runID := "t-rw14-"+column, "t-rw14-"+column+".execute"
			seedTaskRun(t, s, taskID, runID, column)
			clock := &fakeClock{t: time.Now().Add(25 * time.Hour)}
			if _, err := newLadder(t, s, clock, nil, nil).ReconcilePass(ctx); err != nil {
				t.Fatalf("ReconcilePass: %v", err)
			}
			if got := kanbanOf(t, s, taskID); got != column {
				t.Fatalf("kanban = %q, want the untouched %q", got, column)
			}
			// The door still exists — the ENDING is still recorded and
			// answerable; only the board column of a finished task is sacred.
			if got := openAsks(t, s, taskID); got != 1 {
				t.Fatalf("open asks = %d, want 1", got)
			}
		})
	}
}

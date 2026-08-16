package stage_test

// recoveredlineage_rw14a_test.go — P3-RW-14A drain round 2, R-2.
//
// The probe the evaluator ran: a task whose EXECUTE lineage tombstoned, whose
// owner answered the card with `retry`, whose successor then COMPLETED, and
// whose pipeline moved on to verification. The served board read "attention"
// over a stored "verifying" for the whole verify phase — the dead parent
// shouting over a card that had been answered and work that was running.
//
// A successor exists only because a human answered the card (Spec S02.5 step 2
// fork lineage), and `completed` is the one thing a tombstone can stop being:
// the lineage RECOVERED. The overlay must fall silent then, and stay silent
// while the next leg runs — not only once `done` is stored.

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

// seedTombstonedRole drives ONE role lineage of a task to the repeat-offender
// tombstone through the real recovery ladder, leaving the task in the column
// the pipeline had stored for that phase.
func seedTombstonedRole(t *testing.T, h *harness, taskID, role, column, owner string) (askID, runID string) {
	t.Helper()
	ctx := context.Background()
	runID = taskID + "." + role
	err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks(task_id, user_id, title, kanban_status, created_ts)
			 VALUES (?, ?, 'recovered lineage probe', ?, '2026-08-16T00:00:00Z')`, taskID, owner, column)
		return err
	})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := h.runs.Create(ctx, run.NewRun{
		ID: runID, UserID: owner, TaskID: taskID, Substrate: "claude-cli", Lane: "anthropic",
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, to := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := h.runs.Transition(ctx, runID, to, run.TransitionOptions{}); err != nil {
			t.Fatalf("→%s: %v", to, err)
		}
	}
	err = h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE runs SET recovery_attempts = 3 WHERE run_id = ?`, runID)
		return err
	})
	if err != nil {
		t.Fatalf("seed attempts: %v", err)
	}
	reg := settings.New()
	jrnl, err := gates.NewJournal(gates.JournalConfig{DB: h.db, Settings: reg})
	if err != nil {
		t.Fatalf("gates.NewJournal: %v", err)
	}
	at := time.Now().Add(10 * time.Minute)
	l, err := recovery.New(recovery.Config{
		DB: h.db, Log: h.log, Runs: h.runs, Checkpoints: h.cps, Effects: jrnl,
		Settings: reg, Now: func() time.Time { return at },
		Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("recovery.New: %v", err)
	}
	if rpt, err := l.ReconcilePass(ctx); err != nil || rpt.Tombstoned != 1 {
		t.Fatalf("ReconcilePass: %+v err=%v", rpt, err)
	}
	return recovery.LadderAskID(runID), runID
}

// TestRW14ARecoveredLineageStopsShoutingAttention: the evaluator's probe shape,
// end to end through the served view.
func TestRW14ARecoveredLineageStopsShoutingAttention(t *testing.T) {
	ctx := context.Background()
	const owner = "u-operator"
	h := newHarness(t)
	const taskID = "t-rw14a-recovered"
	askID, tombstoned := seedTombstonedRole(t, h, taskID, "execute", "executing", owner)

	// The door is open: nothing drives the task and the board says so.
	if got := viewKanban(t, h, taskID); got != "attention" {
		t.Fatalf("kanban with an unanswered card = %q, want attention", got)
	}
	if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"choice":"retry"}`), false); err != nil {
		t.Fatalf("Answer(retry): %v", err)
	}

	// The fresh attempt runs…
	successor := tombstoned + ".g1"
	for _, to := range []run.State{run.StateClaimed, run.StateRunning} {
		if _, err := h.runs.Transition(ctx, successor, to, run.TransitionOptions{}); err != nil {
			t.Fatalf("successor →%s: %v", to, err)
		}
	}
	if got := viewKanban(t, h, taskID); got != "executing" {
		t.Fatalf("kanban while the retried execute leg runs = %q, want executing", got)
	}

	// …and SUCCEEDS. The lineage recovered; the pipeline moves to verification
	// exactly as the landed execute-leg tail does (complete, store the next
	// column, launch the verify run).
	if _, err := h.runs.Transition(ctx, successor, run.StateCompleted, run.TransitionOptions{
		Reason: "execution complete: deliverable produced", Actor: run.ActorPlatform,
	}); err != nil {
		t.Fatalf("successor →completed: %v", err)
	}
	err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE tasks SET kanban_status = 'verifying' WHERE task_id = ?`, taskID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.runs.Create(ctx, run.NewRun{
		ID: taskID + ".verify", UserID: owner, TaskID: taskID, Substrate: "claude-cli", Lane: "anthropic",
	}); err != nil {
		t.Fatalf("create verify run: %v", err)
	}
	for _, to := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := h.runs.Transition(ctx, taskID+".verify", to, run.TransitionOptions{}); err != nil {
			t.Fatalf("verify →%s: %v", to, err)
		}
	}

	// THE PROBE: mid-pipeline, with the task neither done nor cancelled, the
	// served column must be the STORED one.
	if got := viewKanban(t, h, taskID); got != "verifying" {
		t.Fatalf("SERVED kanban = %q over stored \"verifying\" — a recovered lineage is still shouting attention through the whole verify phase (drain r2 R-2)", got)
	}
	// The tombstoned parent is still there, still a fact, still not the story.
	if r, err := h.runs.Get(ctx, tombstoned); err != nil || r.State != run.StateTombstoned {
		t.Fatalf("tombstoned parent = %v (err %v), want it left exactly as it ended", r.State, err)
	}
	// And an UNRECOVERED tombstone still speaks: end the verify leg the same
	// way and the board must go back to attention.
	if _, err := h.runs.Transition(ctx, taskID+".verify", run.StateCrashed, run.TransitionOptions{
		Reason: "fixture crash", Actor: run.ActorPlatform,
	}); err != nil {
		t.Fatalf("verify →crashed: %v", err)
	}
	if _, err := h.runs.Transition(ctx, taskID+".verify", run.StateTombstoned, run.TransitionOptions{
		Reason: "fixture tombstone", Actor: run.ActorPlatform,
	}); err != nil {
		t.Fatalf("verify →tombstoned: %v", err)
	}
	if got := viewKanban(t, h, taskID); got != "attention" {
		t.Fatalf("SERVED kanban with a fresh unrecovered tombstone = %q, want attention — the overlay must still do its one job", got)
	}
}

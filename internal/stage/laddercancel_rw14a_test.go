package stage_test

// laddercancel_rw14a_test.go — P3-RW-14A drain round 1, items D1 and D2.
//
// The card's own answers were contradicting the board and the cancel mapping:
//
//   D1 — the tombstone overlay in the served view was successor-blind and
//        task-state-blind, so a task CANCELLED at the card read "attention"
//        forever, and a RETRIED lineage read "attention" while its successor
//        worked (evaluator probe E);
//   D2 — cancel wrote `cancelled` on the task row in isolation, leaving live
//        sibling runs billing under a cancelled column (probe B), and retry
//        would happily resurrect an already-ended task from a stale surviving
//        card.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// liveSibling adds a second, RUNNING run of the same task, parked on its own
// open ask — the probe-B shape: a card on a dead lineage while another run of
// the task is still alive and billing.
func liveSibling(t *testing.T, h *harness, taskID, owner string) string {
	t.Helper()
	ctx := context.Background()
	runID := taskID + ".execute"
	if _, err := h.runs.Create(ctx, run.NewRun{
		ID: runID, UserID: owner, TaskID: taskID, Substrate: "claude-cli", Lane: "anthropic",
	}); err != nil {
		t.Fatalf("create sibling: %v", err)
	}
	for _, to := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning, run.StateParked} {
		if _, err := h.runs.Transition(ctx, runID, to, run.TransitionOptions{}); err != nil {
			t.Fatalf("sibling →%s: %v", to, err)
		}
	}
	err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts)
			 VALUES (?, ?, ?, '{"kind":"decision_card","summary":"sibling card"}', 'open', '2026-08-16T00:00:00Z')`,
			"ask-verify-sibling", runID, owner)
		return err
	})
	if err != nil {
		t.Fatalf("seed sibling ask: %v", err)
	}
	return runID
}

func viewKanban(t *testing.T, h *harness, taskID string) string {
	t.Helper()
	raw, err := h.sur.Task(context.Background(), taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	return decodeView(t, raw).Kanban
}

// TestRW14ACancelAtTheCardEndsTheWholeTaskAndTheViewSaysSo covers D2(a) and the
// cancel half of D1: the answer ends every live sibling through the ratified
// mapping, closes their asks, and the SERVED view reads `cancelled` — not the
// "attention" the old overlay pinned there forever.
func TestRW14ACancelAtTheCardEndsTheWholeTaskAndTheViewSaysSo(t *testing.T) {
	ctx := context.Background()
	const owner = "u-operator"
	h := newHarness(t)
	const taskID = "t-rw14a-cancel"
	askID, tombstoned := seedTombstonedLineage(t, h, taskID, owner)
	sibling := liveSibling(t, h, taskID, owner)

	// Before: the door is open and the board says so.
	if got := viewKanban(t, h, taskID); got != "attention" {
		t.Fatalf("kanban before the answer = %q, want attention", got)
	}

	if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"choice":"cancel"}`), false); err != nil {
		t.Fatalf("Answer(cancel): %v", err)
	}

	// The live sibling ENDED (a cancelled task may never keep a billing run)…
	sib, err := h.runs.Get(ctx, sibling)
	if err != nil {
		t.Fatal(err)
	}
	if !run.IsTerminal(sib.State) {
		t.Fatalf("sibling run is %s — a cancelled task cannot keep a live run (S02.3 via CONVENTIONS §14 reading 9)", sib.State)
	}
	// …and its card closed with it: a parked run's ask must not outlive the run.
	var open int
	if err := h.db.QueryRowContext(ctx,
		`SELECT count(*) FROM asks a JOIN runs r ON r.run_id = a.run_id
		  WHERE r.task_id = ? AND a.status = 'open'`, taskID).Scan(&open); err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("open asks after the cancel = %d, want 0", open)
	}
	// The tombstoned lineage is untouched — a terminal is a fact.
	if r, err := h.runs.Get(ctx, tombstoned); err != nil || r.State != run.StateTombstoned {
		t.Fatalf("tombstoned run = %v (err %v), want it left alone", r.State, err)
	}
	// THE PROBE: stored and served agree.
	var stored string
	if err := h.db.QueryRowContext(ctx,
		`SELECT kanban_status FROM tasks WHERE task_id = ?`, taskID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "cancelled" {
		t.Fatalf("stored kanban = %q, want cancelled", stored)
	}
	if got := viewKanban(t, h, taskID); got != "cancelled" {
		t.Fatalf("SERVED kanban = %q against stored %q — the overlay is still overriding an answered door (D1)", got, stored)
	}
}

// TestRW14ARetryLeavesAttentionAndRefusesAnEndedTask covers the retry half of
// D1 (the board follows the successor) and D2(b) (a stale card cannot
// resurrect a task its owner already ended).
func TestRW14ARetryLeavesAttentionAndRefusesAnEndedTask(t *testing.T) {
	ctx := context.Background()
	const owner = "u-operator"
	h := newHarness(t)
	const taskID = "t-rw14a-retry"
	askID, tombstoned := seedTombstonedLineage(t, h, taskID, owner)

	if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"choice":"retry"}`), false); err != nil {
		t.Fatalf("Answer(retry): %v", err)
	}
	// The successor is queued and being driven, so the board follows the WORK.
	if got := viewKanban(t, h, taskID); got != "verifying" {
		t.Fatalf("SERVED kanban after retry = %q, want verifying — the superseded tombstone is history, not an open door", got)
	}
	// It keeps following it as the successor advances.
	successor := tombstoned + ".g1"
	for _, to := range []run.State{run.StateClaimed, run.StateRunning} {
		if _, err := h.runs.Transition(ctx, successor, to, run.TransitionOptions{}); err != nil {
			t.Fatalf("successor →%s: %v", to, err)
		}
	}
	if got := viewKanban(t, h, taskID); got != "verifying" {
		t.Fatalf("SERVED kanban with the successor running = %q, want verifying", got)
	}
	// And when the work FINISHES, the finished column is the truth (the shape
	// that read "attention" forever before D1).
	if _, err := h.runs.Transition(ctx, successor, run.StateCompleted, run.TransitionOptions{}); err != nil {
		t.Fatalf("successor →completed: %v", err)
	}
	err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE tasks SET kanban_status = 'done' WHERE task_id = ?`, taskID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := viewKanban(t, h, taskID); got != "done" {
		t.Fatalf("SERVED kanban after the retried lineage finished = %q, want done", got)
	}

	// D2(b): a SECOND ladder card on the same task, still open after the task
	// ended, must refuse retry rather than resurrect it.
	staleAsk, _ := seedTombstonedLineage(t, h, "t-rw14a-stale", owner)
	err = h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE tasks SET kanban_status = 'cancelled' WHERE task_id = 't-rw14a-stale'`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.sk.AnswerLadderAsk(ctx, owner, staleAsk,
		json.RawMessage(`{"choice":"retry"}`)); !errors.Is(err, verify.ErrNotResumable) {
		t.Fatalf("retry on a cancelled task returned %v, want ErrNotResumable", err)
	}
	// Nothing was enqueued and nothing moved.
	if _, err := h.runs.Get(ctx, "t-rw14a-stale.verify.g1"); err == nil {
		t.Fatal("a successor was created for a cancelled task — the card resurrected ended work")
	}
	var column string
	if err := h.db.QueryRowContext(ctx,
		`SELECT kanban_status FROM tasks WHERE task_id = 't-rw14a-stale'`).Scan(&column); err != nil {
		t.Fatal(err)
	}
	if column != "cancelled" {
		t.Fatalf("kanban = %q after the refused retry, want cancelled", column)
	}
	// The refused answer consumed nothing: the door is still there to cancel.
	var status string
	if err := h.db.QueryRowContext(ctx, `SELECT status FROM asks WHERE ask_id = ?`, staleAsk).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "open" {
		t.Fatalf("ask status = %q after a refused retry, want open", status)
	}
}

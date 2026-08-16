package stage_test

// ladderdoor_rw14_test.go — P3-RW-14 Cut A, brief spec 5 (R3): the ladder
// terminal's card is ANSWERABLE.
//
// A door nobody can walk through is not a door. The recovery ladder's
// tombstone-review card (Spec S02.5 step 7) routes through the same
// prefix-dispatched answer surface every other card uses (CONVENTIONS §16),
// and carries exactly two verbs (P3-RW-14 OQ2, ratified):
//
//   - retry  — a fresh generation-bumped attempt of the terminated lineage
//     with attempts reset: "an answer is an explicit human grant of a fresh
//     bounded budget" (the §16 / S06.7(a) precedent);
//   - cancel — the task ends, under the ratified S02.3 mapping (CONVENTIONS
//     §14 reading 9).
//
// Every other verb is refused LOUDLY (ErrUnsupportedAnswer) — a card never
// silently absorbs an answer it cannot mean.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// seedTombstonedLineage drives one task's verify lineage to the repeat-offender
// tombstone through the REAL recovery ladder (never by hand-writing the ask),
// so what this test answers is exactly what production files.
func seedTombstonedLineage(t *testing.T, h *harness, taskID, owner string) (askID, runID string) {
	t.Helper()
	ctx := context.Background()
	runID = taskID + ".verify"
	err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks(task_id, user_id, title, kanban_status, created_ts)
			 VALUES (?, ?, 'wedge probe', 'verifying', '2026-08-16T00:00:00Z')`, taskID, owner)
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
		_, err := tx.ExecContext(ctx,
			`UPDATE runs SET recovery_attempts = 3 WHERE run_id = ?`, runID)
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
	at := time.Now().Add(10 * time.Minute) // past the §54 reap bound
	l, err := recovery.New(recovery.Config{
		DB: h.db, Log: h.log, Runs: h.runs, Checkpoints: h.cps, Effects: jrnl,
		Settings: reg, Now: func() time.Time { return at },
		Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("recovery.New: %v", err)
	}
	rpt, err := l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.Tombstoned != 1 {
		t.Fatalf("report = %+v, want 1 tombstoned", rpt)
	}
	return recovery.LadderAskID(runID), runID
}

func askStatus(t *testing.T, h *harness, askID string) string {
	t.Helper()
	var status string
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT status FROM asks WHERE ask_id = ?`, askID).Scan(&status); err != nil {
		t.Fatalf("read ask %s: %v", askID, err)
	}
	return status
}

// TestRW14TombstoneRetryAndCancel: the tombstone-review card's two verbs do
// what they say, and nothing else is admitted.
func TestRW14TombstoneRetryAndCancel(t *testing.T) {
	ctx := context.Background()
	const owner = "u-operator"

	t.Run("cancel ends the task", func(t *testing.T) {
		h := newHarness(t)
		askID, runID := seedTombstonedLineage(t, h, "t-rw14-cancel", owner)

		if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"choice":"cancel"}`), false); err != nil {
			t.Fatalf("Answer(cancel): %v", err)
		}
		if got := askStatus(t, h, askID); got != "answered" {
			t.Fatalf("ask status = %q, want answered — the card is consumed by the answer", got)
		}
		var kanban string
		if err := h.db.QueryRowContext(ctx,
			`SELECT kanban_status FROM tasks WHERE task_id = 't-rw14-cancel'`).Scan(&kanban); err != nil {
			t.Fatal(err)
		}
		if kanban != "cancelled" {
			t.Fatalf("kanban = %q, want cancelled (S02.3 via CONVENTIONS §14 reading 9)", kanban)
		}
		// The ENDED lineage is not rewritten: cancel is a decision about the
		// task, not a resurrection of a dead run.
		if r, err := h.runs.Get(ctx, runID); err != nil || r.State != run.StateTombstoned {
			t.Fatalf("run state = %v (err %v), want tombstoned", r.State, err)
		}
	})

	t.Run("retry grants a fresh bounded budget", func(t *testing.T) {
		h := newHarness(t)
		askID, runID := seedTombstonedLineage(t, h, "t-rw14-retry", owner)

		if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"choice":"retry"}`), false); err != nil {
			t.Fatalf("Answer(retry): %v", err)
		}
		if got := askStatus(t, h, askID); got != "answered" {
			t.Fatalf("ask status = %q, want answered", got)
		}
		successor := runID + ".g1"
		s, err := h.runs.Get(ctx, successor)
		if err != nil {
			t.Fatalf("no generation-bumped successor %s: %v", successor, err)
		}
		switch {
		case s.State != run.StateQueued:
			t.Fatalf("successor state = %s, want queued", s.State)
		case s.Generation != 1:
			t.Fatalf("successor generation = %d, want 1 (the takeover fence, S02.5 step 4)", s.Generation)
		case s.RecoveryAttempts != 0:
			t.Fatalf("successor recovery_attempts = %d, want 0 — an answer grants a FRESH bounded budget", s.RecoveryAttempts)
		case s.ParentRunID != runID:
			t.Fatalf("successor parent = %q, want %q", s.ParentRunID, runID)
		}
		if r, err := h.runs.Get(ctx, runID); err != nil || r.State != run.StateTombstoned {
			t.Fatalf("tombstoned parent = %v (err %v) — the retry supersedes it, never rewrites it", r.State, err)
		}
		// …and it DISPATCHES: the successor is admitted through the one
		// ingress (Spec S16.6), so the claim loop picks it up.
		if n := h.tick(ctx); n != 1 {
			t.Fatalf("tick dispatched %d, want the retried run", n)
		}
		if got, err := h.runs.Get(ctx, successor); err != nil {
			t.Fatal(err)
		} else if got.State == run.StateQueued {
			t.Fatal("the retried run was never taken off the queue — a card that answers into nothing is not a door")
		}
	})

	t.Run("every other verb is refused loudly", func(t *testing.T) {
		h := newHarness(t)
		askID, _ := seedTombstonedLineage(t, h, "t-rw14-verb", owner)

		for _, verb := range []string{"accept_best_effort", "revise_with_guidance", "waive_check", "proceed_anyway"} {
			_, err := h.sk.AnswerLadderAsk(ctx, owner, askID,
				json.RawMessage(`{"choice":"`+verb+`"}`))
			if !errors.Is(err, verify.ErrUnsupportedAnswer) {
				t.Fatalf("verb %q answered with %v, want ErrUnsupportedAnswer", verb, err)
			}
		}
		// A refused answer consumes nothing: the door is still open.
		if got := askStatus(t, h, askID); got != "open" {
			t.Fatalf("ask status = %q after refused verbs, want open", got)
		}
		// D10: the card is the run owner's.
		if _, err := h.sk.AnswerLadderAsk(ctx, "u-someone-else", askID,
			json.RawMessage(`{"choice":"cancel"}`)); !errors.Is(err, verify.ErrNotRequester) {
			t.Fatalf("a stranger's answer returned %v, want ErrNotRequester", err)
		}
	})
}

package recovery_test

// tombstonecard_rw14_test.go — P3-RW-14 acceptance test, committed RED
// (CONVENTIONS §3 Amendment-A: the tests-first pipeline commits the brief's
// acceptance tests failing; the packet's implementation commit restores
// green).
//
// Spec S02.5 step 7 names "tombstone-review cards" as a ladder output; the
// landed tombstone branch (recovery.go) records the card only as a STRING
// inside the run.state_changed payload detail — no durable asks row exists,
// and the owning task keeps whatever kanban status it wedged in. Live proof
// (builder world, 2026-08-16): task t-4b982b38a62baee5 sat at kanban
// "verifying" with its verify lineage tombstoned (event_seq 1075) and ZERO
// open asks — no visible, answerable door out (Spec S15.5 "blocked-on-a-
// human"; S15.6 the inbox is where "the platform needs you" lives).

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// TestRW14TombstoneLeavesAnAnswerableDoor: when the ladder tombstones a
// repeat offender (⚙ recovery.max_attempts exhausted, Spec S02.5 step 3),
// the pass MUST leave a durable, OPEN asks row (the S02.5 step 7
// tombstone-review card — a door a human can answer, not a payload string)
// and the owning task MUST leave its stage column (kanban → "attention",
// the S15.5 blocked-on-a-human surface; the verifyTerminal precedent).
func TestRW14TombstoneLeavesAnAnswerableDoor(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)

	// A task row wedged mid-pipeline, exactly the live shape.
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks(task_id, user_id, title, kanban_status, created_ts)
			 VALUES ('t-rw14', 'u1', 'wedge probe', 'verifying', '2026-08-16T00:00:00Z')`)
		return err
	})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}

	// Its run, driven to running through the ratified edges, lineage already
	// at ⚙ recovery.max_attempts (3) forks — the TestRepeatOffenderTombstoned
	// setup plus the task link.
	if _, err := s.runs.Create(ctx, run.NewRun{
		ID: "r-rw14", UserID: "u1", TaskID: "t-rw14", Substrate: "mock", Lane: "mock",
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, to := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := s.runs.Transition(ctx, "r-rw14", to, run.TransitionOptions{}); err != nil {
			t.Fatalf("→%s: %v", to, err)
		}
	}
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE runs SET recovery_attempts = 3 WHERE run_id = 'r-rw14'`)
		return err
	})
	if err != nil {
		t.Fatalf("seed attempts: %v", err)
	}

	// Past the §54 reap bound, inside ⚙ recovery.stale_finalize: the bound
	// decision is the repeat-offender tombstone.
	clock := &fakeClock{t: time.Now().Add(10 * time.Minute)}
	l := newLadder(t, s, clock, nil, nil)
	rpt, err := l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.Tombstoned != 1 {
		t.Fatalf("report = %+v, want 1 tombstoned", rpt)
	}

	// THE DOOR (Spec S02.5 step 7): a durable OPEN ask on the tombstoned
	// lineage, owner-attributed — not a string in an event payload.
	var open int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM asks a JOIN runs r ON r.run_id = a.run_id
		 WHERE r.task_id = 't-rw14' AND a.status = 'open'`).Scan(&open); err != nil {
		t.Fatalf("count open asks: %v", err)
	}
	if open == 0 {
		t.Fatal("tombstoned lineage left NO open ask — the task wedges with no visible, answerable door out (Spec S02.5 step 7 tombstone-review card)")
	}

	// THE TASK LEAVES ITS STAGE COLUMN: kanban → "attention" (the S15.5
	// blocked-on-a-human surface; the stage verifyTerminal card precedent).
	var kanban string
	if err := s.db.QueryRowContext(ctx,
		`SELECT kanban_status FROM tasks WHERE task_id = 't-rw14'`).Scan(&kanban); err != nil {
		t.Fatalf("read kanban: %v", err)
	}
	if kanban != "attention" {
		t.Fatalf("kanban = %q after tombstone, want %q — the board still shows the task working while nothing drives it", kanban, "attention")
	}
}

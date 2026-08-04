package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

func openDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), storage.DBFileName), settings.New())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenAppliesRatifiedPragmas(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

	var synchronous int
	if err := db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatalf("read synchronous: %v", err)
	}
	if synchronous != 2 { // 2 = FULL
		t.Errorf("synchronous = %d, want 2 (FULL — ⚙ state.synchronous default, G2 Def.1)", synchronous)
	}
	var busy int
	if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if busy != 5000 {
		t.Errorf("busy_timeout = %d ms, want 5000 (⚙ state.busy_timeout default)", busy)
	}
	// journal_mode and foreign_keys are asserted inside Open; re-check here
	// against regressions.
	var mode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

func TestMigrateFromEmptyAndRerun(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

	applied, err := db.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if applied < 1 {
		t.Fatalf("Migrate applied %d migrations, want >= 1", applied)
	}
	v1, err := db.UserVersion(ctx)
	if err != nil {
		t.Fatalf("UserVersion: %v", err)
	}
	if v1 != applied {
		t.Errorf("user_version = %d after %d migrations", v1, applied)
	}

	again, err := db.Migrate(ctx)
	if err != nil {
		t.Fatalf("re-run Migrate: %v", err)
	}
	if again != 0 {
		t.Errorf("re-run applied %d migrations, want 0 (idempotent)", again)
	}
	v2, _ := db.UserVersion(ctx)
	if v2 != v1 {
		t.Errorf("user_version moved on re-run: %d -> %d", v1, v2)
	}

	// The S02.2 table families plus the settings pair all exist.
	want := []string{
		"users", "tasks", "runs", "run_events", "checkpoints", "asks",
		"effects", "queue", "lanes", "artifact_claims", "engine_sessions",
		"receipts", "settings", "settings_events",
	}
	for _, table := range want {
		var n int
		err := db.QueryRowContext(ctx,
			`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n)
		if err != nil || n != 1 {
			t.Errorf("table %q missing (err=%v)", table, err)
		}
	}
}

func TestReopenExistingDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), storage.DBFileName)
	db, err := storage.Open(ctx, path, settings.New())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db2, err := storage.Open(ctx, path, settings.New())
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	defer db2.Close()
	applied, err := db2.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate on reopen: %v", err)
	}
	if applied != 0 {
		t.Errorf("reopen applied %d migrations, want 0", applied)
	}
}

func TestAppendOnlyTriggers(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
			 VALUES (NULL, NULL, 'u1', 'test', 1, '{}', '2026-07-19T00:00:00Z')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO settings_events (actor, key, user_id, old, new, ts, reason)
			 VALUES ('operator:u1', 'k', '', '1', '2', '2026-07-19T00:00:00Z', '')`)
		return err
	})
	if err != nil {
		t.Fatalf("seed rows: %v", err)
	}

	for _, stmt := range []string{
		`UPDATE run_events SET type = 'tampered'`,
		`DELETE FROM run_events`,
		`UPDATE settings_events SET new = 'tampered'`,
		`DELETE FROM settings_events`,
	} {
		err := db.WriteTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, stmt)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%q: err = %v, want append-only trigger abort", stmt, err)
		}
	}
}

func TestWriteTxRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	sentinel := errors.New("boom")
	err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO users (user_id, created_ts) VALUES ('u1', '2026-07-19T00:00:00Z')`); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WriteTx err = %v, want sentinel", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if n != 0 {
		t.Errorf("rolled-back insert persisted: %d users", n)
	}
}

func TestCheckpointTruncate(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.CheckpointTruncate(ctx); err != nil {
		t.Fatalf("CheckpointTruncate: %v", err)
	}
}

func TestResolvePath(t *testing.T) {
	if got, err := storage.ResolvePath("/explicit/platform.db"); err != nil || got != "/explicit/platform.db" {
		t.Errorf("explicit: got %q, %v", got, err)
	}

	t.Setenv("STATE_DIRECTORY", "/var/lib/sinet")
	if got, err := storage.ResolvePath(""); err != nil || got != "/var/lib/sinet/platform.db" {
		t.Errorf("STATE_DIRECTORY: got %q, %v", got, err)
	}
	t.Setenv("STATE_DIRECTORY", "/var/lib/sinet:/var/lib/other")
	if got, err := storage.ResolvePath(""); err != nil || got != "/var/lib/sinet/platform.db" {
		t.Errorf("STATE_DIRECTORY list: got %q, %v", got, err)
	}

	t.Setenv("STATE_DIRECTORY", "")
	t.Setenv("XDG_STATE_HOME", "/home/x/.local/state")
	if got, err := storage.ResolvePath(""); err != nil || got != "/home/x/.local/state/sinet/platform.db" {
		t.Errorf("XDG_STATE_HOME: got %q, %v", got, err)
	}
}

// Package storage owns platform.db, the authoritative durable state of the
// platform (Spec S02.1), behind the storage seam of Spec S01.3.
//
// One SQLite database file in WAL mode; the control plane is the sole
// writer, and everything else opens read-only (enforced by posture, not by
// this package). The seam is reopened only by its named triggers: second
// host, sustained >100 writes/s, multi-process writers (Spec S01.3).
//
// Discipline bound here, from Spec S02.1:
//
//   - every writing transaction is BEGIN IMMEDIATE (via the driver's
//     _txlock=immediate), honoring ⚙ state.busy_timeout;
//   - foreign_keys=ON on every connection;
//   - ⚙ state.synchronous (default FULL) on every connection;
//   - PRAGMA integrity_check at every open;
//   - migrations are numbered one-transaction SQL files tracked by
//     PRAGMA user_version — history is never rewritten;
//   - the -wal/-shm sidecars are never deleted or renamed, the live DB file
//     is never file-copied (snapshots go through VACUUM INTO), and WAL is
//     same-host only, never a network filesystem.
//
// The ⚙ values above are read from the settings registry by dotted key
// through the narrow Settings interface; the registry object is supplied by
// the composition root. Because settings overrides themselves live in
// platform.db, bootstrap opens with the registry's declared defaults and the
// shell re-applies via ReapplySettings once overrides are loaded (Spec
// S01.6 step 1, S01.10).
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (Spec S01.5, S16.3; components.lock)
)

// Settings is the storage-facing view of the settings registry (Spec
// S01.10). Implementations resolve dotted ⚙ keys to effective values.
type Settings interface {
	String(key string) (string, error)
	Duration(key string) (time.Duration, error)
}

// Dotted ⚙ keys consumed by this package (owned by Spec S02).
const (
	keySynchronous = "state.synchronous"
	keyBusyTimeout = "state.busy_timeout"
)

// DBFileName is the database file name under the state directory (Spec
// S02.1: "One SQLite database file"; S01.11 places it in /var/lib/sinet via
// StateDirectory=).
const DBFileName = "platform.db"

// DB is the open platform database. Methods are safe for concurrent use;
// the connection pool is deliberately a single connection so every write
// serializes in-process (the sole-writer posture needs no more at Sinet's
// write rate, Spec S02.1).
type DB struct {
	path     string
	settings Settings
	sql      *sql.DB
}

// ResolvePath returns the platform.db path: the explicit override when
// non-empty; else $STATE_DIRECTORY/platform.db (systemd StateDirectory=,
// Spec S01.11 — zero custom path code in production); else the unprivileged
// dev-mode default $XDG_STATE_HOME/sinet/platform.db, with the XDG fallback
// ~/.local/state.
func ResolvePath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if sd := os.Getenv("STATE_DIRECTORY"); sd != "" {
		// StateDirectory= may pass a colon-separated list; the first entry
		// is the primary state directory.
		first := strings.SplitN(sd, ":", 2)[0]
		return filepath.Join(first, DBFileName), nil
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("storage: resolve db path: %w", err)
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, "sinet", DBFileName), nil
}

// Open opens (creating if absent) platform.db at path, applies the ratified
// connection discipline, and verifies journal mode and integrity (Spec
// S02.1). It does not run migrations; call Migrate.
func Open(ctx context.Context, path string, settings Settings) (*DB, error) {
	if settings == nil {
		return nil, errors.New("storage: nil settings registry")
	}
	d := &DB{path: path, settings: settings}
	if err := d.open(ctx); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *DB) open(ctx context.Context) error {
	synchronous, err := d.settings.String(keySynchronous)
	if err != nil {
		return fmt.Errorf("storage: read ⚙ %s: %w", keySynchronous, err)
	}
	busy, err := d.settings.Duration(keyBusyTimeout)
	if err != nil {
		return fmt.Errorf("storage: read ⚙ %s: %w", keyBusyTimeout, err)
	}

	if err := os.MkdirAll(filepath.Dir(d.path), 0o700); err != nil {
		return fmt.Errorf("storage: create state directory: %w", err)
	}

	q := url.Values{}
	// Order matters: the driver applies _pragma in sequence on every new
	// connection. busy_timeout first so the journal-mode switch on a
	// contended file waits rather than failing.
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busy.Milliseconds()))
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(wal)")
	q.Add("_pragma", fmt.Sprintf("synchronous(%s)", synchronous))
	// Every writing transaction is BEGIN IMMEDIATE (Spec S02.1).
	q.Set("_txlock", "immediate")
	dsn := "file:" + d.path + "?" + q.Encode()

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("storage: open %s: %w", d.path, err)
	}
	// One connection: in-process writes serialize, pragmas cannot drift
	// across pool members, and the read path stays short-batch by
	// convention (Spec S02.1 read hygiene).
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	sqlDB.SetConnMaxIdleTime(0)

	if err := verify(ctx, sqlDB); err != nil {
		sqlDB.Close()
		return fmt.Errorf("storage: %s: %w", d.path, err)
	}
	d.sql = sqlDB
	return nil
}

func verify(ctx context.Context, sqlDB *sql.DB) error {
	var mode string
	if err := sqlDB.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("journal_mode: %w", err)
	}
	if !strings.EqualFold(mode, "wal") {
		return fmt.Errorf("journal_mode is %q, want wal (Spec S02.1)", mode)
	}
	var fk int
	if err := sqlDB.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		return fmt.Errorf("foreign_keys: %w", err)
	}
	if fk != 1 {
		return errors.New("foreign_keys is off, want on (Spec S02.1)")
	}
	// PRAGMA integrity_check at every platform start (Spec S02.1).
	var integrity string
	if err := sqlDB.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return fmt.Errorf("integrity_check: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("integrity_check failed: %s", integrity)
	}
	return nil
}

// ReapplySettings reopens the connection with the current effective ⚙
// values (state.synchronous, state.busy_timeout). The shell calls it after
// the settings registry has loaded overrides from the DB (Spec S01.6 step
// 1). Not safe concurrently with other use of the DB.
func (d *DB) ReapplySettings(ctx context.Context) error {
	old := d.sql
	if err := d.open(ctx); err != nil {
		return err
	}
	return old.Close()
}

// WriteTx runs fn inside a single BEGIN IMMEDIATE transaction, committing
// on nil and rolling back on error (Spec S02.1 write discipline). Every
// cross-table invariant is written through one WriteTx.
func (d *DB) WriteTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage: begin immediate: %w", err)
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: commit: %w", err)
	}
	return nil
}

// QueryContext runs a read query outside any explicit transaction. Readers
// keep batches short by ⚙-bounded cursor reads; long-lived read
// transactions are forbidden (Spec S02.1 read hygiene).
func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.sql.QueryContext(ctx, query, args...)
}

// QueryRowContext runs a single-row read query.
func (d *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return d.sql.QueryRowContext(ctx, query, args...)
}

// CheckpointTruncate runs PRAGMA wal_checkpoint(TRUNCATE) — the periodic
// WAL truncation (⚙ state.wal_truncate_interval) and the pre-sleep flush
// step both use it (Spec S02.1, S01.7). Scheduling is the shell's duty.
func (d *DB) CheckpointTruncate(ctx context.Context) error {
	var busy, logFrames, checkpointed int
	err := d.sql.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").
		Scan(&busy, &logFrames, &checkpointed)
	if err != nil {
		return fmt.Errorf("storage: wal_checkpoint(TRUNCATE): %w", err)
	}
	if busy != 0 {
		return errors.New("storage: wal_checkpoint(TRUNCATE) blocked by a reader")
	}
	return nil
}

// Path returns the database file path.
func (d *DB) Path() string { return d.path }

// Close closes the database.
func (d *DB) Close() error { return d.sql.Close() }

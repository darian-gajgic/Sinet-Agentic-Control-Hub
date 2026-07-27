package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// readonly.go — the SECOND handle on platform.db, opened read-only by
// mechanism rather than by posture (Spec S14.10 ¶3, S12.3: "read-only
// connection (query_only)").
//
// WHY A SECOND HANDLE. This package's writing handle sets SetMaxOpenConns(1)
// so the sole-writer posture needs no coordination — which means a per-
// connection `PRAGMA query_only` on it would disable the control plane's own
// writes. The Layer-2 open-SQL surface therefore opens its own *sql.DB. The
// package doc already says "everything else opens read-only (enforced by
// posture, not by this package)"; for the one surface that executes SQL a
// model wrote, posture is not enough, so this is the mechanism.
//
// TWO INDEPENDENT LIMBS, NEITHER OF WHICH IS SUFFICIENT ALONE. Both statements
// below are measured against the pinned modernc.org/sqlite (2026-07-27, and
// re-measured during the B5-8B drain), and the second one CORRECTS an earlier
// version of this comment that overclaimed:
//
//   - `mode=ro` opens the file with SQLITE_OPEN_READONLY. It protects THE MAIN
//     DATABASE ONLY: every INSERT / UPDATE / DELETE / CREATE / DROP / VACUUM
//     against platform.db fails with "attempt to write a readonly database
//     (8)", permanently and undefeatably.
//   - `PRAGMA query_only(1)` is the spec's named limb, and it is what covers
//     the databases `mode=ro` does NOT: TEMP and anything ATTACHed.
//
// The two gaps, precisely:
//
//   - `query_only` is DEFEASIBLE. `PRAGMA query_only = 0` executed on this
//     handle succeeds silently and the flag clears. `mode=ro` still refuses
//     writes to the main database afterwards, so platform.db is never at risk.
//   - But `mode=ro` DOES NOT COVER TEMP OR ATTACHED DATABASES. Measured: with
//     `query_only` cleared, `CREATE TEMP TABLE`, a temp INSERT, `ATTACH`,
//     `CREATE TABLE loot.stolen` and `INSERT INTO loot.stolen SELECT payload
//     FROM run_events` ALL SUCCEED — which is an exfiltration path to a file
//     outside platform.db, even though platform.db itself stays unwritable.
//
// Therefore `query_only` is RE-ASSERTED ON EVERY QUERY (assertQueryOnly
// below), so a cleared flag cannot persist from one query to the next, and the
// window in which the second gap is open is bounded by a single call. The
// parser is the other half of that defense: internal/history refuses PRAGMA
// and ATTACH outright, so a generated statement never gets to clear the flag
// in the first place. VerifyReadOnly proves the whole property at open.
//
// The handle is opened AFTER the writing handle has created the database and
// its WAL sidecars — a read-only connection cannot create them. That ordering
// is guaranteed by construction: the only constructor is a method on the open
// *DB.

// ReadOnly is a read-only handle on the same platform.db file. It exposes
// reads and nothing else: there is no WriteTx, no ExecContext and no Tx verb
// on this type, so the unwritable form is the ONLY form a caller can express.
type ReadOnly struct {
	path string
	sql  *sql.DB
}

// ErrNotReadOnly reports a handle that did not verify as read-only. It is
// returned from OpenReadOnly, so a handle that cannot prove the property never
// reaches a caller.
var ErrNotReadOnly = errors.New("storage: handle did not verify as read-only")

// readOnlyProbe is the canary statement VerifyReadOnly requires to FAIL. It
// names a table that does not exist and never will; a read-only handle refuses
// it for being a write before SQLite ever looks at the name.
const readOnlyProbe = `CREATE TABLE sinet_readonly_probe_must_fail (x INTEGER)`

// OpenReadOnly opens a second, read-only handle on this database.
//
// It reads ⚙ state.busy_timeout through the same narrow Settings interface the
// writing handle uses — the read-only connection contends for the same file
// and must wait the same way rather than failing fast.
func (d *DB) OpenReadOnly(ctx context.Context) (*ReadOnly, error) {
	busy, err := d.settings.Duration(keyBusyTimeout)
	if err != nil {
		return nil, fmt.Errorf("storage: read ⚙ %s: %w", keyBusyTimeout, err)
	}

	q := url.Values{}
	// Order matters: the driver applies _pragma in sequence on every new
	// connection. busy_timeout first, so a contended file waits.
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busy.Milliseconds()))
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "query_only(1)")
	// Limb 2: the open flag. Not defeasible by a generated PRAGMA.
	q.Set("mode", "ro")
	dsn := "file:" + d.path + "?" + q.Encode()

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open %s read-only: %w", d.path, err)
	}
	// One connection, for the same reason the writing handle keeps one:
	// `query_only` is a PER-CONNECTION pragma, so a pool whose members could
	// drift is a pool where the property holds on some connections and not
	// others. One connection also bounds what a generated query can occupy.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	sqlDB.SetConnMaxIdleTime(0)

	r := &ReadOnly{path: d.path, sql: sqlDB}
	if err := r.VerifyReadOnly(ctx); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return r, nil
}

// VerifyReadOnly proves the property instead of asserting it: `query_only`
// reads back as set, and an actual write is REFUSED. It runs at every open,
// the same posture as the writing handle's PRAGMA integrity_check.
func (r *ReadOnly) VerifyReadOnly(ctx context.Context) error {
	var qo int
	if err := r.sql.QueryRowContext(ctx, "PRAGMA query_only").Scan(&qo); err != nil {
		return fmt.Errorf("storage: read query_only: %w", err)
	}
	if qo != 1 {
		return fmt.Errorf("%w: query_only is %d, want 1", ErrNotReadOnly, qo)
	}
	if _, err := r.sql.ExecContext(ctx, readOnlyProbe); err == nil {
		return fmt.Errorf("%w: the write probe SUCCEEDED", ErrNotReadOnly)
	} else if !strings.Contains(err.Error(), "readonly") {
		// A different failure would leave the property unproven — the probe
		// must fail FOR THE RIGHT REASON.
		return fmt.Errorf("%w: the write probe failed for an unexpected reason: %v", ErrNotReadOnly, err)
	}
	return nil
}

// QueryContext runs a read. It is the only way to execute anything on this
// handle.
//
// `query_only` is RE-ASSERTED first, every time. The flag is per-connection
// state that a single statement can clear, and this handle keeps ONE
// connection, so without this a cleared flag would persist for the life of the
// process and every later query would run with the limb that covers TEMP and
// ATTACHed databases silently switched off.
func (r *ReadOnly) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if err := r.assertQueryOnly(ctx); err != nil {
		return nil, err
	}
	return r.sql.QueryContext(ctx, query, args...)
}

// assertQueryOnly re-sets the pragma on the handle's single connection. It is a
// SET rather than a check because setting is both cheaper and stronger: a check
// would have to decide what to do about a cleared flag, and the answer is
// always "set it back".
func (r *ReadOnly) assertQueryOnly(ctx context.Context) error {
	if _, err := r.sql.ExecContext(ctx, "PRAGMA query_only(1)"); err != nil {
		return fmt.Errorf("storage: re-assert query_only: %w", err)
	}
	return nil
}

// Path returns the database file path.
func (r *ReadOnly) Path() string { return r.path }

// Close closes the read-only handle. It does not affect the writing handle.
func (r *ReadOnly) Close() error { return r.sql.Close() }

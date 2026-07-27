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
// TWO INDEPENDENT LIMBS, and the measurement that says why both are needed
// (recorded 2026-07-27 against the pinned modernc.org/sqlite):
//
//   - `mode=ro` in the DSN opens the file with SQLITE_OPEN_READONLY. Every
//     INSERT / UPDATE / DELETE / CREATE / DROP / VACUUM — and every write into
//     an ATTACHed or TEMP database — fails with "attempt to write a readonly
//     database (8)".
//   - `PRAGMA query_only(1)` on every connection is the spec's named limb.
//
// The measurement's finding is that `query_only` ALONE IS NOT SUFFICIENT: a
// statement `PRAGMA query_only=0` executed on the handle SUCCEEDS SILENTLY
// (no error, the flag clears). It is `mode=ro` that then still refuses the
// write. Both limbs ship, and VerifyReadOnly proves the property at every open
// rather than trusting either.
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
func (r *ReadOnly) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return r.sql.QueryContext(ctx, query, args...)
}

// Path returns the database file path.
func (r *ReadOnly) Path() string { return r.path }

// Close closes the read-only handle. It does not affect the writing handle.
func (r *ReadOnly) Close() error { return r.sql.Close() }

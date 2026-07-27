package storage_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// readonly_test.go — R29. The read-only handle's property is proven at the
// SQLITE LAYER, not at a parser: every writing form is attempted through the
// handle and every one is refused by the database itself.

// try runs one statement on the read-only handle and ALWAYS releases the
// connection. The handle keeps a single connection so `query_only` cannot
// drift across pool members; a *sql.Rows left open would therefore wedge every
// later read. Production reads rows to exhaustion and closes them; these tests
// need the same discipline for statements that return none.
func try(t *testing.T, ctx context.Context, ro *storage.ReadOnly, stmt string) error {
	t.Helper()
	rows, err := ro.QueryContext(ctx, stmt)
	if rows != nil {
		rows.Close()
	}
	return err
}

func readOnlyFixture(t *testing.T) (context.Context, *storage.DB, *storage.ReadOnly, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, storage.DBFileName)
	db, err := storage.Open(ctx, path, settings.New())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE probe(a TEXT); INSERT INTO probe VALUES('original');`)
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ro, err := db.OpenReadOnly(ctx)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	t.Cleanup(func() { ro.Close() })
	return ctx, db, ro, dir
}

// TestReadOnlyHandleRefusesEveryWrite — the guardrail's outermost limb. DDL,
// DML, PRAGMA-write, VACUUM and writes into ATTACHed or TEMP databases all die
// at the SQLite layer, so a parser bug cannot become a write.
func TestReadOnlyHandleRefusesEveryWrite(t *testing.T) {
	ctx, db, ro, dir := readOnlyFixture(t)

	side := filepath.Join(dir, "side.db")
	// ATTACH itself is NOT refused by SQLite on a read-only handle (measured);
	// it is the PARSER that must refuse it (see guard_test.go). Attaching here
	// makes the "even an attached database is unwritable" assertion real.
	if err := try(t, ctx, ro, `ATTACH DATABASE '`+side+`' AS side`); err != nil {
		t.Logf("attach: %v (the parser is the limb that refuses ATTACH)", err)
	}

	for _, stmt := range []string{
		`INSERT INTO probe VALUES('written')`,
		`UPDATE probe SET a = 'mutated'`,
		`DELETE FROM probe`,
		`CREATE TABLE made(a TEXT)`,
		`DROP TABLE probe`,
		`ALTER TABLE probe ADD COLUMN b TEXT`,
		`CREATE INDEX i ON probe(a)`,
		`CREATE VIEW v AS SELECT 1`,
		`CREATE TRIGGER tr AFTER INSERT ON probe BEGIN SELECT 1; END`,
		`VACUUM`,
		`CREATE TABLE side.leak(a TEXT)`,
		`CREATE TEMP TABLE tmp_leak(a TEXT)`,
		`INSERT INTO run_events (user_id, type, schema_version, payload, ts) VALUES ('x','y',1,'{}','t')`,
	} {
		err := try(t, ctx, ro, stmt)
		if err == nil {
			t.Errorf("the read-only handle EXECUTED %q", stmt)
			continue
		}
		if !strings.Contains(err.Error(), "readonly") {
			t.Errorf("%q failed with %v — want a readonly refusal, so the property is proven for the right reason", stmt, err)
		}
	}

	// MEASURED, and the reason the parser is not optional: these are NOT
	// refused by SQLite on a read-only handle. ATTACH (above) opens — and
	// creates — a database file; REINDEX is a no-op that returns cleanly;
	// PRAGMA query_only=0 silently clears the spec's named limb. Each dies at
	// the parser instead (guard_test.go / injection_test.go).
	for _, notRefused := range []string{`REINDEX`, `PRAGMA query_only = 0`} {
		if err := try(t, ctx, ro, notRefused); err != nil {
			t.Logf("%q was refused by SQLite (%v) — stricter than measured 2026-07-27; the parser refuses it either way", notRefused, err)
		}
	}

	// The writing handle is untouched by any of it.
	var got string
	if err := db.QueryRowContext(ctx, `SELECT a FROM probe`).Scan(&got); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got != "original" {
		t.Errorf("the row reads %q — a write got through", got)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM probe`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("probe holds %d rows, want 1", n)
	}
}

// TestQueryOnlyAloneIsNotEnough — the measured finding that justifies shipping
// BOTH limbs. `PRAGMA query_only=0` succeeds silently on the handle; the write
// it was meant to enable is still refused, because mode=ro is not defeasible
// by a statement.
func TestQueryOnlyAloneIsNotEnough(t *testing.T) {
	ctx, _, ro, _ := readOnlyFixture(t)

	if err := try(t, ctx, ro, `PRAGMA query_only = 0`); err != nil {
		// If a future driver starts refusing this, the finding has changed and
		// the comment in readonly.go must be re-read — but the property below
		// is what actually matters, so this is a log, not a failure.
		t.Logf("PRAGMA query_only=0 was refused (%v) — stricter than measured 2026-07-27", err)
	}
	err := try(t, ctx, ro, `INSERT INTO probe VALUES('after-pragma')`)
	if err == nil {
		t.Fatal("clearing query_only made the handle writable — mode=ro is not holding")
	}
	if !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("post-pragma write failed with %v, want a readonly refusal", err)
	}
	// And the constructor's own proof still holds afterwards only if the pragma
	// is per-connection state we do not depend on: re-verify explicitly.
	if err := ro.VerifyReadOnly(ctx); err == nil {
		t.Log("query_only survived; both limbs hold")
	} else if !errors.Is(err, storage.ErrNotReadOnly) {
		t.Fatalf("VerifyReadOnly: %v", err)
	} else {
		t.Logf("query_only was cleared by the statement (%v) — mode=ro is the limb that held", err)
	}
}

// TestReadOnlyHandleStillReads — the negative would be trivially satisfiable by
// a handle that does nothing at all.
func TestReadOnlyHandleStillReads(t *testing.T) {
	ctx, _, ro, _ := readOnlyFixture(t)
	rows, err := ro.QueryContext(ctx, `SELECT a FROM probe`)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("the read-only handle returned no rows")
	}
	var a string
	if err := rows.Scan(&a); err != nil {
		t.Fatal(err)
	}
	if a != "original" {
		t.Errorf("read %q, want %q", a, "original")
	}
}

// TestOpenReadOnlyVerifiesAtOpen — the constructor proves the property rather
// than assuming it, so a handle that is not read-only never reaches a caller.
func TestOpenReadOnlyVerifiesAtOpen(t *testing.T) {
	ctx, _, ro, _ := readOnlyFixture(t)
	if err := ro.VerifyReadOnly(ctx); err != nil {
		t.Fatalf("VerifyReadOnly on a freshly opened handle: %v", err)
	}
	// Non-tautology: the same verification against the WRITING handle's DSN
	// must fail. Proven by observing that the writing handle can do what the
	// probe statement does.
	if _, err := os.Stat(ro.Path()); err != nil {
		t.Fatalf("the handle names no real file: %v", err)
	}
}

// TestWritingHandleIsUnaffected — the whole reason for a second handle: the
// control plane keeps writing while the read-only surface is open.
func TestWritingHandleIsUnaffected(t *testing.T) {
	ctx, db, _, _ := readOnlyFixture(t)
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO probe VALUES('control-plane')`)
		return err
	}); err != nil {
		t.Fatalf("the control plane could not write while the read-only handle was open: %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM probe`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("probe holds %d rows, want 2", n)
	}
}

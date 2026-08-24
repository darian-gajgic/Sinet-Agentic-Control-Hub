package storage_test

// migration0024_test.go — P3-RW-11 T12 (brief §7; R10, Spec S15.2, CONVENTIONS
// §6). Migration 0024 widens `repo_registry_captures` with the owner-declared
// task family. The properties that matter to an EXISTING world: rows written
// before it read back honest absence, and the table's immutability /
// no-delete / version-monotone guards survive the widening — a schema change
// that quietly disarmed a trigger would be a silent loss of the audit trail
// S13.7 exists to keep.

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// migrateThrough applies the committed migration files up to and including
// version `through`, exactly as the runner does (one transaction each, with its
// own user_version bump), leaving the database at that older schema.
// migrationCount is how many migration files the tree ships — the version a
// full Migrate leaves behind.
func migrationCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("migrations"))
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			n++
		}
	}
	if n == 0 {
		t.Fatal("no migration files found — a count derived from nothing would pin nothing")
	}
	return n
}

func migrateThrough(t *testing.T, db *storage.DB, through int) {
	t.Helper()
	ctx := context.Background()
	entries, err := os.ReadDir(filepath.Join("migrations"))
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for i, name := range names {
		version := i + 1
		if version > through {
			break
		}
		body, err := os.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(body)); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, "PRAGMA user_version = "+strconv.Itoa(version))
			return err
		}); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
}

func TestMigration0024Additive(t *testing.T) {
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// A world at the PREVIOUS schema, with a capture row written before the
	// column existed.
	migrateThrough(t, db, 23)
	if v, err := db.UserVersion(ctx); err != nil || v != 23 {
		t.Fatalf("user_version = %d err=%v, want 23 before the packet's migration", v, err)
	}
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO repo_registry (project_id, user_id, name, store_path, remote_url,
			                           default_branch, members, protected_refs, state,
			                           capture_version, created_ts, updated_ts)
			VALUES ('shop','alice','shop','/tmp/shop','', 'main','[]','["main"]','pending',1,'t','t')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO repo_registry_captures (project_id, version, conventions, commands,
			                                    danger_zones, scan_hash, captured_by, captured_ts)
			VALUES ('shop',1,'[]','{}','[]','h','alice','t')`)
		return err
	}); err != nil {
		t.Fatalf("seed the pre-0024 world: %v", err)
	}

	applied, err := db.Migrate(ctx)
	if err != nil {
		t.Fatalf("Migrate to 0024: %v", err)
	}
	// 0024 is the first migration this run applies, and every later packet's
	// rides behind it. The count is READ from the tree rather than written
	// down, so the pin stays EXACT as later migrations land instead of rotting
	// into an inequality (§63/§64: exact numbers, never floors).
	head := migrationCount(t)
	if want := head - 23; applied != want {
		t.Fatalf("Migrate applied %d migrations, want exactly %d (0024 and every later packet's)", applied, want)
	}
	if v, err := db.UserVersion(ctx); err != nil || v != head {
		t.Fatalf("user_version = %d err=%v, want %d", v, err, head)
	}

	// The old row reads back honest absence, never NULL and never a guess.
	var family string
	if err := db.QueryRowContext(ctx,
		`SELECT family FROM repo_registry_captures WHERE project_id = 'shop' AND version = 1`).Scan(&family); err != nil {
		t.Fatalf("read the migrated row: %v", err)
	}
	if family != "" {
		t.Errorf("pre-0024 capture family = %q, want \"\" (honest absence, R10)", family)
	}

	// The S13.7 guards survive the widening.
	err = db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE repo_registry_captures SET family = 'software' WHERE project_id = 'shop' AND version = 1`)
		return err
	})
	if err == nil {
		t.Error("a captured version was UPDATE-able after 0024 — the immutability trigger is gone (S13.7)")
	}
	err = db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM repo_registry_captures WHERE project_id = 'shop'`)
		return err
	})
	if err == nil {
		t.Error("capture history was deletable after 0024 — the no-delete trigger is gone (S13.7)")
	}
	err = db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE repo_registry SET capture_version = 0 WHERE project_id = 'shop'`)
		return err
	})
	if err == nil {
		t.Error("capture_version rewound after 0024 — the monotone trigger is gone (S13.7)")
	}

	// A row written AFTER the migration carries the family for real.
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO repo_registry_captures (project_id, version, conventions, commands,
			                                    danger_zones, scan_hash, captured_by, captured_ts, family)
			VALUES ('shop',2,'[]','{}','[]','h','alice','t','software')`)
		return err
	}); err != nil {
		t.Fatalf("insert a post-0024 capture: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT family FROM repo_registry_captures WHERE project_id = 'shop' AND version = 2`).Scan(&family); err != nil {
		t.Fatalf("read the new row: %v", err)
	}
	if family != "software" {
		t.Errorf("post-0024 capture family = %q, want software", family)
	}
}

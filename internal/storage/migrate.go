package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

// Migrations are numbered one-transaction SQL files tracked by PRAGMA
// user_version (Spec S02.1). A file, once committed, is immutable — schema
// changes are new numbered files, never edits; history is never rewritten.
//
//go:embed migrations/*.sql
var migrationFS embed.FS

var migrationNameRe = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("storage: read migrations: %w", err)
	}
	var ms []migration
	for _, e := range entries {
		m := migrationNameRe.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("storage: migration file %q does not match NNNN_name.sql", e.Name())
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n == 0 {
			return nil, fmt.Errorf("storage: migration file %q has invalid number", e.Name())
		}
		body, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("storage: read migration %q: %w", e.Name(), err)
		}
		ms = append(ms, migration{version: n, name: e.Name(), sql: string(body)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	for i, m := range ms {
		if m.version != i+1 {
			return nil, fmt.Errorf("storage: migration numbering not contiguous at %q (want %04d)", m.name, i+1)
		}
	}
	return ms, nil
}

// UserVersion returns the schema version recorded in PRAGMA user_version.
func (d *DB) UserVersion(ctx context.Context) (int, error) {
	var v int
	if err := d.sql.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("storage: user_version: %w", err)
	}
	return v, nil
}

// Migrate applies every migration newer than the current user_version, each
// in its own single transaction that also advances user_version, so a crash
// mid-migration leaves the previous version intact (Spec S02.1). Running it
// again is a no-op; it returns the number of migrations applied.
func (d *DB) Migrate(ctx context.Context) (int, error) {
	ms, err := loadMigrations()
	if err != nil {
		return 0, err
	}
	current, err := d.UserVersion(ctx)
	if err != nil {
		return 0, err
	}
	if current > len(ms) {
		return 0, fmt.Errorf("storage: database at schema version %d but only %d migrations exist (never rewrite history — Spec S02.1)", current, len(ms))
	}
	applied := 0
	for _, m := range ms {
		if m.version <= current {
			continue
		}
		err := d.WriteTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, m.sql); err != nil {
				return fmt.Errorf("storage: apply migration %s: %w", m.name, err)
			}
			// user_version is stored in the database header and is
			// transactional: it advances atomically with the migration.
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
				return fmt.Errorf("storage: set user_version for %s: %w", m.name, err)
			}
			return nil
		})
		if err != nil {
			return applied, err
		}
		applied++
	}
	return applied, nil
}

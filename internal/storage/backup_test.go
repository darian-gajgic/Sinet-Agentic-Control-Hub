package storage_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// The S13.10 snapshot triad (VacuumInto -> DumpFrom -> RebuildFromDump ->
// CheckRestored) proven at the storage seam: a consistent copy, a text dump
// behind the S14.9 11.3 export boundary, a faithful rebuild, and the S02.9
// invariants over the result.
func TestSnapshotTriadRoundTripAndExportBoundary(t *testing.T) {
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)

	// Seed a user + task and three platform events with distinct payload bodies:
	// two trace-class types and one keep-forever class (B5-8A).
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO users (user_id, role, created_ts) VALUES ('op','operator','2026-07-22T00:00:00Z')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, created_ts) VALUES ('t1','op','2026-07-22T00:00:00Z')`)
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// The keep-forever allowlist is DATA seeded by internal/retention at boot;
	// this package cannot import it (the driver seam is a leaf), so the row is
	// planted directly — the view is the boundary either way.
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO retention_keep_forever (type, family, note) VALUES ('routing.decided','routing','routing records')`)
		return err
	}); err != nil {
		t.Fatalf("seed keep-forever: %v", err)
	}
	for _, e := range []struct{ typ, body string }{
		{"tool.completed", `{"trace":"TRACE-BODY-ONE"}`},
		{"tool.completed", `{"trace":"TRACE-BODY-TWO"}`},
		{"routing.decided", `{"plain_reason":"KEEP-FOREVER-BODY"}`},
	} {
		if _, err := log.Append(ctx, eventlog.Append{
			UserID: "op", Type: e.typ, SchemaVersion: 1, Payload: []byte(e.body)}); err != nil {
			t.Fatal(err)
		}
	}

	vac := filepath.Join(t.TempDir(), "vac.db")
	if err := db.VacuumInto(ctx, vac); err != nil {
		t.Fatalf("VacuumInto: %v", err)
	}

	// The boundary is a TYPE boundary at every age (Spec S14.9 ¶3 / S13.10): a
	// raw trace payload body never leaves the host, a keep-forever one does.
	dumpSQL, uv, err := storage.DumpFrom(ctx, vac)
	if err != nil {
		t.Fatalf("DumpFrom: %v", err)
	}
	for _, body := range []string{"TRACE-BODY-ONE", "TRACE-BODY-TWO"} {
		if strings.Contains(dumpSQL, body) {
			t.Errorf("raw trace payload body %q reached the dump — the 11.3 boundary is not structural", body)
		}
	}
	if !strings.Contains(dumpSQL, "KEEP-FOREVER-BODY") {
		t.Error("a keep-forever payload body was stripped from the dump; the allowlist must pass it through")
	}

	rebuilt := filepath.Join(t.TempDir(), "rebuilt.db")
	if err := storage.RebuildFromDump(ctx, rebuilt, dumpSQL, uv); err != nil {
		t.Fatalf("RebuildFromDump: %v", err)
	}
	check, err := storage.CheckRestored(ctx, rebuilt)
	if err != nil {
		t.Fatalf("CheckRestored: %v", err)
	}
	if !check.AllOK() {
		t.Fatalf("S02.9 invariants failed: %+v", check)
	}
	if check.UserVersion != uv {
		t.Errorf("rebuilt user_version %d != %d", check.UserVersion, uv)
	}
	// The rebuilt log keeps every ROW (gap-free event_seq), even the stripped one.
	if check.RunEventCount != 3 {
		t.Errorf("rebuilt run_events count %d, want 3 (rows preserved, only bodies stripped)", check.RunEventCount)
	}
}

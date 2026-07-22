package storage_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// The S13.10 snapshot triad (VacuumInto -> DumpFrom -> RebuildFromDump ->
// CheckRestored) proven at the storage seam: a consistent copy, a text dump
// with the trace-payload horizon strip, a faithful rebuild, and the S02.9
// invariants over the result.
func TestSnapshotTriadRoundTripAndHorizonStrip(t *testing.T) {
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

	// Seed a user + task and three platform events with distinct payload bodies.
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
	var seqs []int64
	for _, body := range []string{`{"trace":"OLDEST-BODY"}`, `{"trace":"MIDDLE-BODY"}`, `{"trace":"NEWEST-BODY"}`} {
		seq, err := log.Append(ctx, eventlog.Append{UserID: "op", Type: "e", SchemaVersion: 1, Payload: []byte(body)})
		if err != nil {
			t.Fatal(err)
		}
		seqs = append(seqs, seq)
	}

	vac := filepath.Join(t.TempDir(), "vac.db")
	if err := db.VacuumInto(ctx, vac); err != nil {
		t.Fatalf("VacuumInto: %v", err)
	}

	// Strip payload bodies at or below the FIRST event's seq (the horizon).
	dumpSQL, uv, err := storage.DumpFrom(ctx, vac, seqs[0])
	if err != nil {
		t.Fatalf("DumpFrom: %v", err)
	}
	if strings.Contains(dumpSQL, "OLDEST-BODY") {
		t.Error("oldest trace body was NOT stripped past the horizon")
	}
	if !strings.Contains(dumpSQL, "NEWEST-BODY") || !strings.Contains(dumpSQL, "MIDDLE-BODY") {
		t.Error("bodies past the horizon were wrongly stripped")
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

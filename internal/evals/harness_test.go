package evals_test

// The Spec S14.8 hermetic battery: a t.TempDir() DB, stdlib testing only, zero
// network, zero paid calls. The pinned runner is exercised through a STUB
// binary under t.TempDir() (the sandbox-runtime stub precedent); the real
// promptfoo leg auto-runs when the binary is installed and prints SANCTIONED
// SKIP otherwise (CONVENTIONS §10).

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/conformance"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/evals"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

const repoRoot = "../../"

type fix struct {
	db    *storage.DB
	log   *eventlog.Log
	reg   *settings.Registry
	conf  *conformance.Store
	store *evals.Store
}

func newFix(t *testing.T) *fix {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	if err := reg.Attach(ctx, db, log); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if err := db.ReapplySettings(ctx); err != nil {
		t.Fatalf("ReapplySettings: %v", err)
	}
	conf := conformance.NewStore(db, log, reg)
	if _, err := conf.EnsureSeeded(ctx); err != nil {
		t.Fatalf("EnsureSeeded: %v", err)
	}
	return &fix{db: db, log: log, reg: reg, conf: conf, store: evals.NewStore(db, conf, reg)}
}

func (f *fix) at(now time.Time) { f.store.Now = func() time.Time { return now } }

// rawSQL runs a statement as a raw-SQL holder would — the attempted-violation
// battery that proves the migration triggers, not the Go guards (the §22/§23
// pattern).
func (f *fix) rawSQL(ctx context.Context, stmt string) error {
	return f.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, stmt)
		return err
	})
}

// registerFloors runs the 8.3 knowledge-gate entry hook.
func (f *fix) registerFloors(t *testing.T) {
	t.Helper()
	if _, err := f.store.EnsureFloorsRegistered(context.Background()); err != nil {
		t.Fatalf("EnsureFloorsRegistered: %v", err)
	}
}

// sweepRow returns the S14.8 recording home's current state.
func (f *fix) sweepRow(t *testing.T) conformance.RowState {
	t.Helper()
	rows, err := f.conf.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, r := range rows {
		if r.ID == conformance.RowRegressionSweep {
			return r
		}
	}
	t.Fatalf("the S14.8 regression-sweep row is not seeded")
	return conformance.RowState{}
}

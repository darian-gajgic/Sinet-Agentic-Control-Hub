package recovery_test

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// stack is the S02 machinery over one platform.db — buildable without a
// *testing.T so the kill-9 crash helper (a re-exec of this test binary) can
// use the same code paths as the assertions.
type stack struct {
	reg  *settings.Registry
	db   *storage.DB
	log  *eventlog.Log
	runs *run.Store
	cps  *gates.Checkpoints
	jrnl *gates.Journal
}

func openStack(dir string) (*stack, error) {
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(dir, storage.DBFileName), reg)
	if err != nil {
		return nil, err
	}
	if _, err := db.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	log := eventlog.New(db, reg)
	jrnl, err := gates.NewJournal(gates.JournalConfig{DB: db, Settings: reg})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &stack{
		reg:  reg,
		db:   db,
		log:  log,
		runs: run.NewStore(db, log),
		cps:  gates.NewCheckpoints(db, log),
		jrnl: jrnl,
	}, nil
}

func newStack(t *testing.T) *stack {
	t.Helper()
	s, err := openStack(t.TempDir())
	if err != nil {
		t.Fatalf("openStack: %v", err)
	}
	t.Cleanup(func() { s.db.Close() })
	return s
}

// fakeClock injects wall-clock time into the ladder (the S02.9 companion
// suspend-cycle test injects clock deltas).
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// fakeUnits is a UnitLiveness test double; runs not in the map are GONE.
type fakeUnits map[string]recovery.UnitStatus

func (f fakeUnits) Probe(ctx context.Context, r run.Run) (recovery.UnitStatus, error) {
	if st, ok := f[r.ID]; ok {
		return st, nil
	}
	return recovery.UnitStatus{State: recovery.UnitGone}, nil
}

// fakeHarvest is a HarvestSource test double.
type fakeHarvest map[string][]recovery.HarvestRecord

func (f fakeHarvest) NewerThan(ctx context.Context, r run.Run, sinceEventSeq int64) ([]recovery.HarvestRecord, error) {
	return f[r.ID], nil
}

// newLadder builds a ladder over s with the given doubles (nil doubles fall
// back to the B0 defaults).
func newLadder(t *testing.T, s *stack, clock *fakeClock, units recovery.UnitLiveness, harvest recovery.HarvestSource) *recovery.Ladder {
	t.Helper()
	cfg := recovery.Config{
		DB:          s.db,
		Log:         s.log,
		Runs:        s.runs,
		Checkpoints: s.cps,
		Effects:     s.jrnl,
		Units:       units,
		Harvest:     harvest,
		Settings:    s.reg,
		Logger:      slog.New(slog.DiscardHandler),
	}
	if clock != nil {
		cfg.Now = clock.now
	}
	l, err := recovery.New(cfg)
	if err != nil {
		t.Fatalf("recovery.New: %v", err)
	}
	return l
}

// mkRunning drives a fresh run to running through the ratified edges.
func mkRunning(s *stack, id string) (run.Run, error) {
	ctx := context.Background()
	if _, err := s.runs.Create(ctx, run.NewRun{ID: id, UserID: "u1", Substrate: "mock", Lane: "mock"}); err != nil {
		return run.Run{}, err
	}
	var r run.Run
	var err error
	for _, to := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if r, err = s.runs.Transition(ctx, id, to, run.TransitionOptions{}); err != nil {
			return run.Run{}, fmt.Errorf("→%s: %w", to, err)
		}
	}
	return r, nil
}

func mustRunning(t *testing.T, s *stack, id string) run.Run {
	t.Helper()
	r, err := mkRunning(s, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func getRun(t *testing.T, s *stack, id string) run.Run {
	t.Helper()
	r, err := s.runs.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get %s: %v", id, err)
	}
	return r
}

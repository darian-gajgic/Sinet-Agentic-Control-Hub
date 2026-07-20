package memory_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// The S09 acceptance battery harness: a real platform.db with the ratified
// schema (FTS5 included), the real event log and ledger, the real settings
// registry serving the S18 defaults.

type fix struct {
	t      *testing.T
	db     *storage.DB
	log    *eventlog.Log
	runs   *run.Store
	ledger *ledger.Store
	reg    *settings.Registry
	store  *memory.Store
	gate   *memory.Gate
	root   string
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
	root := filepath.Join(t.TempDir(), "knowledge")
	store, err := memory.NewStore(db, log, reg, root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return &fix{
		t: t, db: db, log: log, runs: run.NewStore(db, log),
		ledger: ledger.NewStore(db, log), reg: reg,
		store: store, gate: memory.NewGate(store), root: root,
	}
}

func (f *fix) user(id, role string) {
	f.t.Helper()
	err := f.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO users (user_id, role, created_ts) VALUES (?, ?, ?)`,
			id, role, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		f.t.Fatalf("insert user %s: %v", id, err)
	}
}

func (f *fix) task(taskID, userID string) {
	f.t.Helper()
	err := f.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES (?, ?, ?, ?)`,
			taskID, userID, "t", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		f.t.Fatalf("insert task: %v", err)
	}
}

func (f *fix) run(runID, taskID, userID string) {
	f.t.Helper()
	if _, err := f.runs.Create(context.Background(), run.NewRun{
		ID: runID, UserID: userID, TaskID: taskID, Substrate: "claude-cli", Lane: "anthropic",
	}); err != nil {
		f.t.Fatalf("create run: %v", err)
	}
}

// claim writes an artifact claim so projectForTask derives the project
// (the one platform-owned project fact until the registry lands, B4).
func (f *fix) claim(taskID, project, userID string) {
	f.t.Helper()
	err := f.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO artifact_claims (task_id, project, user_id, path_globs, mode, status, created_ts)
			 VALUES (?, ?, ?, '["**"]', 'W', 'active', ?)`,
			taskID, project, userID, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		f.t.Fatalf("insert claim: %v", err)
	}
}

// objective seeds the task ledger (birth at first accepted write,
// CONVENTIONS §13) so Assemble has a document.
func (f *fix) objective(runID string) {
	f.t.Helper()
	_, err := f.ledger.SetObjective(context.Background(), runID, "test", ledger.ObjectiveAC{
		Objective:   "test objective",
		SpecVersion: "spec-v1",
		AcceptanceCriteria: []ledger.AcceptanceCriterion{
			{N: 1, Plain: "assembles"},
		},
	})
	if err != nil {
		f.t.Fatalf("SetObjective: %v", err)
	}
}

// events returns the payloads of every event of one type, in seq order.
func (f *fix) events(typ string) []string {
	f.t.Helper()
	rows, err := f.db.QueryContext(context.Background(),
		`SELECT payload FROM run_events WHERE type = ? ORDER BY event_seq`, typ)
	if err != nil {
		f.t.Fatalf("query events: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			f.t.Fatalf("scan event: %v", err)
		}
		out = append(out, p)
	}
	return out
}

// openAsks returns the open ask ids matching a LIKE pattern.
func (f *fix) openAsks(pattern string) []string {
	f.t.Helper()
	rows, err := f.db.QueryContext(context.Background(),
		`SELECT ask_id FROM asks WHERE status = 'open' AND ask_id LIKE ? ORDER BY ask_id`, pattern)
	if err != nil {
		f.t.Fatalf("query asks: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			f.t.Fatalf("scan ask: %v", err)
		}
		out = append(out, id)
	}
	return out
}

func mustCreate(t *testing.T, g *memory.Gate, actor string, d memory.Draft) memory.Entry {
	t.Helper()
	e, err := g.CreateManual(context.Background(), actor, d)
	if err != nil {
		t.Fatalf("CreateManual(%s, %+v): %v", actor, d, err)
	}
	return e
}

package worker_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// The S08 acceptance battery harness: a real platform.db with the 0005
// schema, the real event log, and the real settings registry serving the
// S18 defaults (the four ⚙ workers.* keys read by dotted key). No engine
// is ever spawned — the dry-run station runs through a fake DryEngine
// (zero paid calls, the established fake-engine precedent).

type fix struct {
	t     *testing.T
	db    *storage.DB
	log   *eventlog.Log
	reg   *settings.Registry
	store *worker.Store
	root  string
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
	root := filepath.Join(t.TempDir(), "workers")
	store, err := worker.NewStore(worker.Config{DB: db, Log: log, Settings: reg, Root: root})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return &fix{t: t, db: db, log: log, reg: reg, store: store, root: root}
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

func (f *fix) journal() *gates.Journal {
	f.t.Helper()
	j, err := gates.NewJournal(gates.JournalConfig{DB: f.db, Settings: f.reg})
	if err != nil {
		f.t.Fatalf("NewJournal: %v", err)
	}
	return j
}

// eventTypes lists appended worker.* event types in seq order.
func (f *fix) eventTypes() []string {
	f.t.Helper()
	rows, err := f.db.QueryContext(context.Background(),
		`SELECT type FROM run_events WHERE type LIKE 'worker.%' ORDER BY event_seq`)
	if err != nil {
		f.t.Fatalf("query events: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			f.t.Fatalf("scan event: %v", err)
		}
		out = append(out, typ)
	}
	return out
}

// overrideInt wraps the registry with one overridden ⚙ key so ⚙
// consumption is proven to flow through the dotted-key read.
type overrideInt struct {
	worker.Settings
	key string
	val int64
}

func (o overrideInt) Int(key string) (int64, error) {
	if key == o.key {
		return o.val, nil
	}
	return o.Settings.Int(key)
}

// storeWith rebuilds a store on the same DB/root with a wrapped Settings.
func (f *fix) storeWith(s worker.Settings) *worker.Store {
	f.t.Helper()
	st, err := worker.NewStore(worker.Config{DB: f.db, Log: f.log, Settings: s, Root: f.root})
	if err != nil {
		f.t.Fatalf("NewStore(wrapped): %v", err)
	}
	return st
}

const agenticSrc = `---
name: code-reviewer
description: Reviews Go code changes for defects and style regressions
kind: agentic
domain: software
selectors:
  family: read-analyze
  task_classes: [review]
  triggers: [review my code, check this diff]
profile:
  duty: execution
  effort_floor: standard
equipment:
  tools: [Read, Grep, Glob]
persona: [Terse and precise.]
---
Review the presented diff. Report defects with file:line anchors and
verify the stated conventions hold.
`

func readGrants() worker.RequestedGrants {
	return worker.RequestedGrants{
		Tools:  []string{"Read", "Grep", "Glob"},
		Class:  "C1",
		Egress: worker.EgressNone,
	}
}

func humanProv() worker.Provenance {
	return worker.Provenance{AuthorKind: "human", Origin: worker.OriginHumanWritten}
}

type fakeDry struct {
	last DryReq
	fail bool
}

type DryReq = worker.DryRunRequest

func (f *fakeDry) DryRun(_ context.Context, req worker.DryRunRequest) (worker.DryRunResult, error) {
	f.last = req
	if f.fail {
		return worker.DryRunResult{Completed: false, Detail: "sample task did not complete"}, nil
	}
	return worker.DryRunResult{Completed: true, TranscriptRef: "fake://transcript", Output: "reviewed: ok", CostUSD: 0.01}, nil
}

// draftValidated drives a fresh agentic template to a green battery.
func draftValidated(t *testing.T, f *fix, actor string) (worker.Template, worker.Version, *fakeDry) {
	t.Helper()
	tpl, v, err := f.store.CreateDraft(context.Background(), actor, agenticSrc, readGrants(), humanProv())
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	dry := &fakeDry{}
	res, err := f.store.RunBattery(context.Background(), v.ID, worker.BatteryInput{
		Actor: actor, SampleTask: "review the sample diff", Engine: dry,
		Model: "claude-haiku-4-5", EnginePin: "claude-cli@2.1.215",
	})
	if err != nil {
		t.Fatalf("RunBattery: %v", err)
	}
	if !res.Green {
		t.Fatalf("battery not green: lint=%+v audit=%+v dry=%+v", res.Lint, res.Audit, res.DryRun)
	}
	return tpl, v, dry
}

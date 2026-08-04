package history_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/retention"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// fixture_test.go — the hermetic harness. Tier F throughout: a migrated temp
// DB and, where a duty call is exercised, a local.FakeServer + a real
// local.Duty (the internal/watchlist canary_logprob_test composition). No
// network, no seat, $0.

const (
	opUser  = "op"
	member1 = "alice"
	member2 = "bob"
)

type fixture struct {
	t   *testing.T
	ctx context.Context
	db  *storage.DB
	log *eventlog.Log
	reg *settings.Registry
	st  *history.Store
	now time.Time
	seq int64
}

func newFixture(t *testing.T, opts ...func(*history.Config)) *fixture {
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
		t.Fatalf("settings Attach: %v", err)
	}
	f := &fixture{t: t, ctx: ctx, db: db, log: log, reg: reg, now: time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)}
	cfg := history.Config{DB: db, Log: log, Now: func() time.Time { return f.now }}
	for _, o := range opts {
		o(&cfg)
	}
	st, err := history.New(cfg)
	if err != nil {
		t.Fatalf("history.New: %v", err)
	}
	f.st = st
	return f
}

// exec runs one write statement.
func (f *fixture) exec(q string, args ...any) {
	f.t.Helper()
	if err := f.db.WriteTx(f.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(f.ctx, q, args...)
		return err
	}); err != nil {
		f.t.Fatalf("exec %.60s…: %v", q, err)
	}
}

func (f *fixture) ts(offset time.Duration) string {
	return f.now.Add(offset).Format(time.RFC3339Nano)
}

func (f *fixture) user(id, role string) {
	f.exec(`INSERT INTO users (user_id, role, created_ts) VALUES (?, ?, ?)`, id, role, f.ts(0))
}

func (f *fixture) task(id, owner, title, kanban string) {
	f.exec(`INSERT INTO tasks (task_id, user_id, title, kanban_status, created_ts) VALUES (?, ?, ?, ?, ?)`,
		id, owner, title, kanban, f.ts(0))
}

func (f *fixture) run(id, owner, taskID, state, lane string) {
	f.runAt(id, owner, taskID, state, lane, 0)
}

func (f *fixture) runAt(id, owner, taskID, state, lane string, at time.Duration) {
	f.exec(`INSERT INTO runs (run_id, user_id, task_id, state, lane, created_ts, updated_ts) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, owner, taskID, state, lane, f.ts(at), f.ts(at))
}

// seedTwoOwners builds a two-owner database rich enough for every catalog query
// to run and for the owner-scope assertions to be non-vacuous. alice's rows are
// the ones the axis values select; bob's are deliberately disjoint on every
// axis (different task, lane, worker, domain, project, state and date), so a
// dropped scope predicate shows up as a leak rather than as a coincidence.
func seedTwoOwners(t *testing.T, f *fixture) {
	t.Helper()

	f.user(member1, "member")
	f.task("t-"+member1, member1, "Alice task", "doing")
	f.run("r-"+member1, member1, "t-"+member1, "completed", "lane-"+member1)
	// A live run too, so the "what is running now" shape has something to find.
	f.run("r-"+member1+"-live", member1, "t-"+member1, "running", "lane-"+member1)
	f.receipt("r-"+member1, member1, 2.5, 4, 0)
	f.exec(`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts) VALUES (?, ?, ?, '{}', 'open', ?)`,
		"ask-"+member1, "r-"+member1, member1, f.ts(0))
	f.exec(`INSERT INTO worker_templates (template_id, user_id, name, scope, kind, domain, status, created_ts, updated_ts)
	        VALUES (?, ?, 'alice-worker', 'personal', 'agentic', 'software', 'active', ?, ?)`,
		"wt-"+member1, member1, f.ts(0), f.ts(0))

	f.event("r-"+member1, member1, "routing.decided", map[string]any{
		"cause": "selector-match", "score": 0.9, "worker": "wt-" + member1,
		"worker_name": "alice-worker", "version": "v1", "model": "claude-sonnet-5",
		"lane": "lane-" + member1, "effort": "standard", "window_tokens": 200000,
		"plain_reason": "matched the alice worker selector",
	})
	f.event("r-"+member1, member1, "verdict.recorded", map[string]any{
		"round": 1, "verdict": "SHIP", "domain": "software", "rubric_id": "rb-1",
		"rubric_version": 1, "judge_model": "claude-opus-4-8", "self_family_judge": true,
		"ac_ids": []string{"AC-1"}, "golden_set": map[string]any{"measured": false},
	})
	f.event("r-"+member1, member1, "run.state_changed", map[string]any{"from": "running", "to": "completed"})
	f.event("r-"+member1, member1, "watchdog.flagged", map[string]any{
		"rule": "loop-repeat", "anomaly_class": "loop", "severity": "high", "parked": false, "actor": "platform",
	})
	f.event("r-"+member1, member1, "run.wedged", map[string]any{"reason": "no progress"})
	f.event("", member1, "local.unmetered_defect", map[string]any{
		"run": "r-" + member1, "duty": "intent-filling", "cause": "no checkpointable run",
	})
	f.event("", member1, "limit.event", map[string]any{
		"class": "rate", "provider_signal": "429", "resets_at": f.ts(time.Hour), "lane": "lane-" + member1,
	})
	f.event("", member1, "drift.finding", map[string]any{
		"source": "src-" + member1, "change_class": "contract", "severity": "high",
		"lanes": []string{"lane-" + member1}, "summary": "the alice source changed shape",
	})
	f.event("", member1, "canary.result", map[string]any{
		"canary_kind": "behavioral", "lane": "lane-" + member1, "result": "fail",
		"delta": 0.4, "summary": "behavioral canary drifted", "detail": "below the floor",
		"purpose_tag": "ceremony", "workload_class": "background",
	})

	// bob — disjoint on every axis, and 48 h earlier.
	f.user(member2, "member")
	f.task("t-"+member2, member2, "Bob task", "backlog")
	f.runAt("r-"+member2, member2, "t-"+member2, "crashed", "lane-"+member2, -48*time.Hour)
	f.receipt("r-"+member2, member2, 0, 3, 3)
	f.exec(`INSERT INTO repo_registry (project_id, user_id, name, store_path, default_branch, state, created_ts, updated_ts)
	        VALUES ('p-bob', ?, 'Bob Project', '/tmp/p-bob', 'main', 'active', ?, ?)`, member2, f.ts(0), f.ts(0))
	f.exec(`INSERT INTO artifact_claims (task_id, project, user_id, path_globs, mode, status, created_ts)
	        VALUES (?, 'p-bob', ?, 'src/**', 'W', 'held', ?)`, "t-"+member2, member2, f.ts(0))
	f.exec(`INSERT INTO worker_templates (template_id, user_id, name, scope, kind, domain, status, created_ts, updated_ts)
	        VALUES (?, ?, 'bob-worker', 'personal', 'agentic', 'web-research', 'active', ?, ?)`,
		"wt-"+member2, member2, f.ts(0), f.ts(0))
	f.event("r-"+member2, member2, "routing.decided", map[string]any{
		"cause": "no-fit-generalist", "worker": "wt-" + member2, "worker_name": "bob-worker",
		"version": "v1", "model": "claude-sonnet-5", "lane": "lane-" + member2,
		"generalist": true, "degraded": true, "window_tokens": 200000,
		"plain_reason": "no worker matched; generalist on the execution seat",
	})
	f.event("r-"+member2, member2, "verdict.recorded", map[string]any{
		"round": 2, "verdict": "REVISE", "domain": "web-research", "rubric_id": "rb-2",
		"ac_ids": []string{"AC-9"}, "golden_set": map[string]any{"measured": false},
	})
}

// sampleSlotValue returns a value of the right shape for a slot, so every
// catalog query can be EXECUTED even where no seeded row matches.
func sampleSlotValue(f *fixture, sl history.Slot) string {
	switch sl.Type {
	case history.SlotDateFrom:
		return f.ts(-72 * time.Hour)
	case history.SlotDateTo:
		return f.ts(72 * time.Hour)
	case history.SlotPerson:
		return member1
	case history.SlotProject:
		return "(no project)"
	case history.SlotRunID:
		return "r-" + member1
	case history.SlotTaskID:
		return "t-" + member1
	case history.SlotLane:
		return "lane-" + member1
	case history.SlotWorker:
		return "wt-" + member1
	case history.SlotDomain:
		return "software"
	case history.SlotStatus:
		return "completed"
	case history.SlotCause:
		return "selector-match"
	case history.SlotChangeClass:
		return "contract"
	case history.SlotSource:
		return "src-" + member1
	case history.SlotSeverity:
		return "high"
	case history.SlotRubric:
		return "rb-1"
	case history.SlotRule:
		return "loop-repeat"
	}
	return ""
}

// receipt writes a receipts row whose usage_json is the marshaled shape
// internal/metering produces. priced<0 means "every call unpriced" — the v0
// empty-price-table posture.
func (f *fixture) receipt(runID, owner string, priced float64, calls, unpriced int64) {
	f.t.Helper()
	body := map[string]any{
		"run_id":               runID,
		"user_id":              owner,
		"currency":             "api-equivalent",
		"total_priced_usd":     priced,
		"total_calls":          calls,
		"total_unpriced_calls": unpriced,
		"worst_tier":           "measured",
		"direct_use": map[string]any{
			"label":               "direct-use estimate (heuristic)",
			"formula_ref":         "Spec/benchmark-preregistration-v1.md §13",
			"currency":            "api-equivalent",
			"unpriced":            unpriced > 0,
			"heuristic_usd":       priced,
			"reason":              "no price table loaded at v0 (UNPRICED, never a silent $0 — S10.1)",
			"measured_stage_seam": "cited, not restated",
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		f.t.Fatal(err)
	}
	f.exec(`INSERT INTO receipts (run_id, user_id, usage_json, materialized_ts) VALUES (?, ?, ?, ?)`,
		runID, owner, string(raw), f.ts(0))
}

// event appends one run_events row through the real log, so the envelope
// discipline (event_seq ordering, generation, schema version) is the production
// one. Returns the assigned event_seq.
func (f *fixture) event(runID, owner, typ string, payload any) int64 {
	f.t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		f.t.Fatal(err)
	}
	f.seq++
	seq, err := f.log.Append(f.ctx, eventlog.Append{
		RunID: runID, UserID: owner, Type: typ, SchemaVersion: 1, Payload: raw,
	})
	if err != nil {
		f.t.Fatalf("append %s: %v", typ, err)
	}
	return seq
}

// indexHistory runs B5-8A's projector so history_fts carries the redacted
// corpus this packet's search layer reads.
func (f *fixture) indexHistory() {
	f.t.Helper()
	rs, err := retention.New(retention.Config{DB: f.db, Log: f.log, Settings: f.reg, Now: func() time.Time { return f.now }})
	if err != nil {
		f.t.Fatalf("retention.New: %v", err)
	}
	if _, err := rs.EnsureKeepForeverSeeded(f.ctx); err != nil {
		f.t.Fatalf("EnsureKeepForeverSeeded: %v", err)
	}
	if _, err := rs.Index(f.ctx); err != nil {
		f.t.Fatalf("Index: %v", err)
	}
}

func opScope() history.Scope              { return history.Scope{Operator: true} }
func memberScope(id string) history.Scope { return history.Scope{UserID: id} }

// cell reads one answer cell by column name.
func cell(t *testing.T, a history.Answer, row int, col string) any {
	t.Helper()
	for i, c := range a.Columns {
		if c == col {
			if row >= len(a.Rows) {
				t.Fatalf("row %d out of range (%d rows) for column %q", row, len(a.Rows), col)
			}
			return a.Rows[row][i]
		}
	}
	t.Fatalf("no column %q in %v", col, a.Columns)
	return nil
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

package watchdog

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// helpers_test.go — the hermetic test harness (brief R32): a t.TempDir() DB, the
// in-process stores, and (for Tier-1) a local.FakeServer + a real local.Duty.
// Zero network, zero paid calls, stdlib testing only.

type env struct {
	t    *testing.T
	db   *storage.DB
	log  *eventlog.Log
	reg  *settings.Registry
	runs *run.Store
	cps  *gates.Checkpoints
	fake *local.FakeServer // nil unless withDuty
	duty *local.Duty       // nil unless withDuty
}

// newEnv bootstraps the base infrastructure (no local tier — Tier-0/Tier-2
// tests need no model).
func newEnv(t *testing.T) *env {
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
	return &env{t: t, db: db, log: log, reg: reg, runs: run.NewStore(db, log), cps: gates.NewCheckpoints(db, log)}
}

// withDuty wires a local.FakeServer + a real local.Duty over the env (the $0
// Tier-1 path — the local tier is $0 by construction, no paid call). Returns the
// env for chaining.
func (e *env) withDuty() *env {
	e.fake = local.NewFakeServer()
	e.t.Cleanup(e.fake.Close)
	e.duty = local.NewDuty(local.DutyDeps{
		Registry:    local.NewRegistry(e.reg),
		Client:      local.NewClient(e.fake.URL),
		Checkpoints: e.cps,
		Events:      e.log,
	})
	return e
}

// seatModel resolves the watchdog-disambiguator alias's seat model (so a test
// sets the FakeServer response for exactly that model).
func (e *env) seatModel() string {
	seat, err := e.duty.ResolveSeat(local.AliasWatchdog)
	if err != nil {
		e.t.Fatalf("ResolveSeat(watchdog): %v", err)
	}
	return seat.Model
}

// wd builds a Watchdog over the env with the given optional seams applied.
func (e *env) wd(opts ...func(*Deps)) *Watchdog {
	d := Deps{DB: e.db, Log: e.log, Runs: e.runs, Settings: e.reg}
	if e.duty != nil {
		d.Duty = e.duty
		d.Meter = e.advisoryMeter()
	}
	for _, o := range opts {
		o(&d)
	}
	return New(d)
}

// advisoryMeter is the test AdvisoryMeter: a short-lived platform run for a
// run-less Tier-1 $0 row (mirrors the shell's advisoryMeter, OQ4).
func (e *env) advisoryMeter() AdvisoryMeter {
	return func(ctx context.Context, purpose string) (string, func(), error) {
		id := "platform.advisory." + purpose + "." + strconv.FormatInt(time.Now().UnixNano(), 10)
		if _, err := e.runs.Create(ctx, run.NewRun{ID: id, UserID: run.ActorPlatform, Lane: local.LaneLocal, Substrate: "local"}); err != nil {
			return "", nil, err
		}
		for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
			if _, err := e.runs.Transition(ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
				return "", nil, err
			}
		}
		return id, func() {
			_, _ = e.runs.Transition(context.WithoutCancel(ctx), id, run.StateCompleted, run.TransitionOptions{Actor: run.ActorPlatform})
		}, nil
	}
}

// runningRun creates run id owned by user and walks it to running.
func (e *env) runningRun(id, user string) {
	e.t.Helper()
	ctx := context.Background()
	if _, err := e.runs.Create(ctx, run.NewRun{ID: id, UserID: user, Lane: "anthropic", Substrate: "claude-cli"}); err != nil {
		e.t.Fatalf("Create %s: %v", id, err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := e.runs.Transition(ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			e.t.Fatalf("Transition %s→%s: %v", id, st, err)
		}
	}
}

// completeRun walks a running run to completed.
func (e *env) completeRun(id string) {
	e.t.Helper()
	if _, err := e.runs.Transition(context.Background(), id, run.StateCompleted, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
		e.t.Fatalf("complete %s: %v", id, err)
	}
}

// toolEvent appends a tool.completed event on a run (the loop/error signal
// source). isError marks it an error.
func (e *env) toolEvent(id, tool, argsDigest string, isError bool) {
	e.t.Helper()
	gen := e.gen(id)
	m := map[string]any{"tool": tool, "args_digest": argsDigest, "excerpt": "x"}
	if isError {
		m["is_error"] = true
	}
	payload, _ := json.Marshal(m)
	if _, err := e.log.Append(context.Background(), eventlog.Append{
		RunID: id, Generation: gen, UserID: e.owner(id), Type: "tool.completed", SchemaVersion: 1, Payload: payload,
	}); err != nil {
		e.t.Fatalf("append tool event: %v", err)
	}
}

// staleActivity appends a tool.completed event whose ts is secondsAgo in the
// past (the LAST event by seq, so newestEventTS returns it) — simulating a run
// whose most recent activity is old (the silence scenario). Ordering is by
// event_seq (not ts); ts here is the wall-clock absence measurement (Spec S02.5).
func (e *env) staleActivity(id string, secondsAgo int) {
	e.t.Helper()
	payload, _ := json.Marshal(map[string]any{"tool": "Bash", "args_digest": "d", "excerpt": "x"})
	if _, err := e.log.Append(context.Background(), eventlog.Append{
		RunID: id, Generation: e.gen(id), UserID: e.owner(id), Type: "tool.completed",
		SchemaVersion: 1, Payload: payload,
		Time: time.Now().Add(-time.Duration(secondsAgo) * time.Second),
	}); err != nil {
		e.t.Fatalf("append stale activity: %v", err)
	}
}

// verdictEvent appends a verdict.recorded event on a run (verification activity).
func (e *env) verdictEvent(id string) {
	e.t.Helper()
	if _, err := e.log.Append(context.Background(), eventlog.Append{
		RunID: id, Generation: e.gen(id), UserID: e.owner(id), Type: "verdict.recorded", SchemaVersion: 1,
		Payload: json.RawMessage(`{"round":1,"verdict":"pass"}`),
	}); err != nil {
		e.t.Fatalf("append verdict event: %v", err)
	}
}

func (e *env) gen(id string) int64 {
	r, err := e.runs.Get(context.Background(), id)
	if err != nil {
		e.t.Fatalf("get %s: %v", id, err)
	}
	return r.Generation
}

func (e *env) owner(id string) string {
	r, err := e.runs.Get(context.Background(), id)
	if err != nil {
		e.t.Fatalf("get %s: %v", id, err)
	}
	return r.UserID
}

func (e *env) state(id string) run.State {
	r, err := e.runs.Get(context.Background(), id)
	if err != nil {
		e.t.Fatalf("get %s: %v", id, err)
	}
	return r.State
}

// flagsFor returns the watchdog.flagged events for a run (or platform when id
// is "").
func (e *env) flagsFor(id string) []FlagPayload {
	e.t.Helper()
	var (
		rows interface {
			Next() bool
			Scan(...any) error
			Close() error
			Err() error
		}
		err error
	)
	if id == "" {
		rows, err = e.db.QueryContext(context.Background(),
			`SELECT payload FROM run_events WHERE type='watchdog.flagged' AND run_id IS NULL ORDER BY event_seq`)
	} else {
		rows, err = e.db.QueryContext(context.Background(),
			`SELECT payload FROM run_events WHERE type='watchdog.flagged' AND run_id=? ORDER BY event_seq`, id)
	}
	if err != nil {
		e.t.Fatalf("query flags: %v", err)
	}
	defer rows.Close()
	var out []FlagPayload
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			e.t.Fatalf("scan flag: %v", err)
		}
		var fp FlagPayload
		if err := json.Unmarshal([]byte(payload), &fp); err != nil {
			e.t.Fatalf("decode flag: %v", err)
		}
		out = append(out, fp)
	}
	return out
}

// eventsOfType returns the payloads of a run_events type (any scope).
func (e *env) eventsOfType(typ string) []json.RawMessage {
	e.t.Helper()
	rows, err := e.db.QueryContext(context.Background(),
		`SELECT payload FROM run_events WHERE type=? ORDER BY event_seq`, typ)
	if err != nil {
		e.t.Fatalf("query %s: %v", typ, err)
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			e.t.Fatalf("scan %s: %v", typ, err)
		}
		out = append(out, json.RawMessage(payload))
	}
	return out
}

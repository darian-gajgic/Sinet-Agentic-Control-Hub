package run_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

func newStore(t *testing.T) (*storage.DB, *eventlog.Log, *run.Store) {
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
	return db, log, run.NewStore(db, log)
}

// lastEvent returns the newest event for runID.
func lastEvent(t *testing.T, log *eventlog.Log, runID string) eventlog.Event {
	t.Helper()
	evs, err := log.After(context.Background(), 0, 1000)
	if err != nil {
		t.Fatalf("After: %v", err)
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].RunID == runID {
			return evs[i]
		}
	}
	t.Fatalf("no events for run %q", runID)
	return eventlog.Event{}
}

func countEvents(t *testing.T, log *eventlog.Log, runID string) int {
	t.Helper()
	evs, err := log.After(context.Background(), 0, 1000)
	if err != nil {
		t.Fatalf("After: %v", err)
	}
	n := 0
	for _, e := range evs {
		if e.RunID == runID {
			n++
		}
	}
	return n
}

func TestTransitionMatrix(t *testing.T) {
	allowed := [][2]run.State{
		{run.StateNew, run.StateQueued},
		{run.StateQueued, run.StateClaimed},
		{run.StateQueued, run.StateFinalized}, // 4.5 human cancel of a queued run (P3-B6-2A OQ1)
		{run.StateClaimed, run.StateRunning},
		{run.StateClaimed, run.StateCrashed},
		{run.StateRunning, run.StateParked},
		{run.StateRunning, run.StateDraining},
		{run.StateRunning, run.StateCompleted},
		{run.StateRunning, run.StateCrashed},
		{run.StateRunning, run.StateTombstoned},
		{run.StateDraining, run.StateParked},
		{run.StateDraining, run.StateCompleted},
		{run.StateDraining, run.StateCrashed},
		{run.StateParked, run.StateRunning},
		{run.StateParked, run.StateDiedAtGate},
		{run.StateParked, run.StateFinalized},
		{run.StateCrashed, run.StateCompleted},
		{run.StateCrashed, run.StateTombstoned},
		{run.StateCrashed, run.StateFinalized},
	}
	isAllowed := map[[2]run.State]bool{}
	for _, e := range allowed {
		isAllowed[e] = true
		if !run.CanTransition(e[0], e[1]) {
			t.Errorf("CanTransition(%s, %s) = false, want true", e[0], e[1])
		}
	}
	states := []run.State{
		run.StateNew, run.StateQueued, run.StateClaimed, run.StateRunning,
		run.StateParked, run.StateDraining, run.StateCompleted, run.StateCrashed,
		run.StateFinalized, run.StateTombstoned, run.StateDiedAtGate,
	}
	for _, from := range states {
		for _, to := range states {
			if !isAllowed[[2]run.State{from, to}] && run.CanTransition(from, to) {
				t.Errorf("CanTransition(%s, %s) = true, not a ratified edge", from, to)
			}
		}
	}
	for _, s := range []run.State{run.StateCompleted, run.StateFinalized, run.StateTombstoned, run.StateDiedAtGate} {
		if !run.IsTerminal(s) {
			t.Errorf("IsTerminal(%s) = false, want true", s)
		}
	}
	if run.IsTerminal(run.StateCrashed) {
		t.Error("IsTerminal(crashed) = true; crashed is supersedable by recovery (Spec S02.3/S02.5)")
	}
}

func TestCreateAppendsBirthEvent(t *testing.T) {
	ctx := context.Background()
	_, log, store := newStore(t)
	r, err := store.Create(ctx, run.NewRun{ID: "r1", UserID: "u1", Substrate: "claude", Lane: "cheap"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if r.State != run.StateNew || r.Generation != 0 {
		t.Fatalf("created run = %s gen %d, want new gen 0", r.State, r.Generation)
	}
	ev := lastEvent(t, log, "r1")
	if ev.Type != run.EventCreated || ev.Generation != 0 || ev.UserID != "u1" {
		t.Fatalf("birth event = %+v, want type %s gen 0 user u1", ev, run.EventCreated)
	}
	if _, err := store.Create(ctx, run.NewRun{ID: "r1", UserID: "u1"}); !errors.Is(err, run.ErrExists) {
		t.Fatalf("duplicate Create err = %v, want ErrExists", err)
	}
}

func TestTransitionAppendsEventAtomically(t *testing.T) {
	ctx := context.Background()
	_, log, store := newStore(t)
	if _, err := store.Create(ctx, run.NewRun{ID: "r1", UserID: "u1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	r, err := store.Transition(ctx, "r1", run.StateQueued, run.TransitionOptions{Reason: "admitted"})
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if r.State != run.StateQueued {
		t.Fatalf("state = %s, want queued", r.State)
	}
	ev := lastEvent(t, log, "r1")
	if ev.Type != run.EventState {
		t.Fatalf("event type = %s, want %s", ev.Type, run.EventState)
	}
	var p struct{ From, To, Reason string }
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.From != "new" || p.To != "queued" || p.Reason != "admitted" {
		t.Fatalf("payload = %+v", p)
	}

	// Not a ratified edge → rejected, nothing written.
	before := countEvents(t, log, "r1")
	if _, err := store.Transition(ctx, "r1", run.StateRunning, run.TransitionOptions{}); !errors.Is(err, run.ErrInvalidTransition) {
		t.Fatalf("queued→running err = %v, want ErrInvalidTransition", err)
	}
	if got := countEvents(t, log, "r1"); got != before {
		t.Fatalf("invalid transition appended an event (%d → %d)", before, got)
	}
}

// TestTransitionRollsBackWithEvent proves the one-transaction discipline
// (Spec S02.3): if the event append fails, the state update must not
// survive.
func TestTransitionRollsBackWithEvent(t *testing.T) {
	ctx := context.Background()
	_, log, store := newStore(t)
	if _, err := store.Create(ctx, run.NewRun{ID: "r1", UserID: "u1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// A detail payload past ⚙ state.event_payload_cap (64 KB default) makes
	// the same-tx append fail after the runs.state UPDATE already ran.
	huge := json.RawMessage(`{"pad":"` + strings.Repeat("x", 70*1024) + `"}`)
	_, err := store.Transition(ctx, "r1", run.StateQueued, run.TransitionOptions{Detail: huge})
	if !errors.Is(err, eventlog.ErrPayloadTooLarge) {
		t.Fatalf("err = %v, want ErrPayloadTooLarge", err)
	}
	r, err := store.Get(ctx, "r1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if r.State != run.StateNew {
		t.Fatalf("state = %s after failed append, want new (rollback)", r.State)
	}
	if got := countEvents(t, log, "r1"); got != 1 {
		t.Fatalf("events = %d, want 1 (birth only)", got)
	}
}

func TestResumeBumpsGeneration(t *testing.T) {
	ctx := context.Background()
	_, log, store := newStore(t)
	if _, err := store.Create(ctx, run.NewRun{ID: "r1", UserID: "u1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, to := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning, run.StateParked} {
		if _, err := store.Transition(ctx, "r1", to, run.TransitionOptions{}); err != nil {
			t.Fatalf("→%s: %v", to, err)
		}
	}
	r, err := store.Transition(ctx, "r1", run.StateRunning, run.TransitionOptions{Reason: "resume"})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if r.Generation != 1 {
		t.Fatalf("generation after resume = %d, want 1 (Spec S02.5 step 4)", r.Generation)
	}
	ev := lastEvent(t, log, "r1")
	if ev.Generation != 1 {
		t.Fatalf("resume event generation = %d, want 1 (the new incarnation's first act)", ev.Generation)
	}
	// The zombie of the pre-nap incarnation is fenced now.
	_, err = log.Append(ctx, eventlog.Append{
		RunID: "r1", Generation: 0, UserID: "u1", Type: "zombie.write",
		SchemaVersion: 1, Payload: json.RawMessage(`{}`),
	})
	if !errors.Is(err, eventlog.ErrStaleGeneration) {
		t.Fatalf("stale append err = %v, want ErrStaleGeneration", err)
	}
}

func TestInStatesAndSuccessorLookups(t *testing.T) {
	ctx := context.Background()
	db, _, store := newStore(t)
	for _, id := range []string{"a", "b"} {
		if _, err := store.Create(ctx, run.NewRun{ID: id, UserID: "u1"}); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}
	if _, err := store.Transition(ctx, "b", run.StateQueued, run.TransitionOptions{}); err != nil {
		t.Fatalf("b→queued: %v", err)
	}
	news, err := store.InStates(ctx, run.StateNew)
	if err != nil {
		t.Fatalf("InStates: %v", err)
	}
	if len(news) != 1 || news[0].ID != "a" {
		t.Fatalf("InStates(new) = %+v, want [a]", news)
	}
	// Successor with lineage columns round-trips.
	err = db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := store.CreateTx(ctx, tx, run.NewRun{
			ID: "a.g1", UserID: "u1", State: run.StateQueued, Generation: 1,
			ParentRunID: "a", DispatchID: "a#g1", RecoveryAttempts: 1,
		}, run.EventForked, nil)
		return err
	})
	if err != nil {
		t.Fatalf("CreateTx successor: %v", err)
	}
	err = db.WriteTx(ctx, func(tx *sql.Tx) error {
		if s, ok, err := store.SuccessorOfTx(ctx, tx, "a"); err != nil || !ok || s.ID != "a.g1" {
			t.Fatalf("SuccessorOfTx = %v %v %v", s.ID, ok, err)
		}
		if s, ok, err := store.SuccessorByDispatchTx(ctx, tx, "a#g1"); err != nil || !ok || s.ID != "a.g1" {
			t.Fatalf("SuccessorByDispatchTx = %v %v %v", s.ID, ok, err)
		}
		if _, ok, err := store.SuccessorOfTx(ctx, tx, "b"); err != nil || ok {
			t.Fatalf("SuccessorOfTx(b) = %v %v, want none", ok, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The unique dispatch index is the double-start CAS (Spec S02.5 step 3).
	err = db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := store.CreateTx(ctx, tx, run.NewRun{
			ID: "a.g1-dup", UserID: "u1", State: run.StateQueued, Generation: 1,
			ParentRunID: "a", DispatchID: "a#g1", RecoveryAttempts: 1,
		}, run.EventForked, nil)
		return err
	})
	if !errors.Is(err, run.ErrExists) {
		t.Fatalf("duplicate dispatch err = %v, want ErrExists", err)
	}
}

package run_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// terminal_hook_test.go — the B5-8A run-end seam (Spec S14.9 ¶1: "at run end,
// never later"). The hook is the ONE edge every terminal transition in the tree
// crosses, so the run summary hangs on it rather than on ten remembered calls.

// TestTerminalHookFiresOnlyAtTerminalStates: not on birth, not on an
// intermediate transition, exactly once per terminal edge.
func TestTerminalHookFiresOnlyAtTerminalStates(t *testing.T) {
	ctx := context.Background()
	_, _, store := newStore(t)

	var fired []string
	mustHook(t, store, func(_ context.Context, _ *sql.Tx, r run.Run) error {
		fired = append(fired, string(r.State))
		return nil
	})

	if _, err := store.Create(ctx, run.NewRun{ID: "r1", UserID: "alice"}); err != nil {
		t.Fatal(err)
	}
	if len(fired) != 0 {
		t.Errorf("the hook fired on a run's BIRTH: %v", fired)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := store.Transition(ctx, "r1", st, run.TransitionOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if len(fired) != 0 {
		t.Errorf("the hook fired on a non-terminal transition: %v", fired)
	}
	if _, err := store.Transition(ctx, "r1", run.StateCompleted, run.TransitionOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(fired) != 1 || fired[0] != string(run.StateCompleted) {
		t.Errorf("hook fired %v, want exactly one completed", fired)
	}
	// The hook receives the POST-transition run, so the terminal state it sees
	// is the one the summary must record.
	if _, err := store.Transition(ctx, "r1", run.StateCompleted, run.TransitionOptions{}); err == nil {
		t.Error("a terminal run accepted another transition")
	}
	if len(fired) != 1 {
		t.Errorf("a refused transition still ran the hook: %v", fired)
	}
}

// TestTerminalHookRunsInsideTheTransitionTransaction: a hook failure ROLLS BACK
// the ending — the state change and whatever the hook writes commit together or
// not at all (S02.3 / CONVENTIONS §6, §8). A half-written ending is not a state
// the log can hold.
func TestTerminalHookRunsInsideTheTransitionTransaction(t *testing.T) {
	ctx := context.Background()
	db, _, store := newStore(t)

	boom := errors.New("the hook refused")
	mustHook(t, store, func(ctx context.Context, tx *sql.Tx, r run.Run) error {
		// A real write inside the caller's transaction, then a failure: both
		// must vanish.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, created_ts) VALUES ('hook-wrote-this','alice','2026-08-01T00:00:00Z')`); err != nil {
			return err
		}
		return boom
	})

	if _, err := store.Create(ctx, run.NewRun{ID: "r1", UserID: "alice"}); err != nil {
		t.Fatal(err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := store.Transition(ctx, "r1", st, run.TransitionOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := store.Transition(ctx, "r1", run.StateCompleted, run.TransitionOptions{})
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("transition err = %v, want the hook's error wrapped", err)
	}
	if !strings.Contains(err.Error(), "terminal hook") {
		t.Errorf("the error must name the hook: %v", err)
	}
	r, err := store.Get(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if r.State != run.StateRunning {
		t.Errorf("run state = %q, want running — a failed hook rolls the ending back", r.State)
	}
	var wrote int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM tasks WHERE task_id = 'hook-wrote-this'`).Scan(&wrote); err != nil {
		t.Fatal(err)
	}
	if wrote != 0 {
		t.Error("the hook's write survived a rolled-back transition — it must ride the caller's transaction")
	}
}

// TestNoTerminalHookLeavesEveryTransitionUnchanged: nil is the default and the
// FSM behaves exactly as it did before B5-8A.
func TestNoTerminalHookLeavesEveryTransitionUnchanged(t *testing.T) {
	ctx := context.Background()
	_, _, store := newStore(t)
	if _, err := store.Create(ctx, run.NewRun{ID: "r1", UserID: "alice"}); err != nil {
		t.Fatal(err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning, run.StateCompleted} {
		if _, err := store.Transition(ctx, "r1", st, run.TransitionOptions{}); err != nil {
			t.Fatalf("transition to %s with no hook: %v", st, err)
		}
	}
}

// TestTerminalHookCoversEveryTerminalState: the hook rides IsTerminal, so it is
// not a completed-only special case. StateCrashed is deliberately NOT terminal
// (recovery supersedes it), so it does not fire.
func TestTerminalHookCoversEveryTerminalState(t *testing.T) {
	ctx := context.Background()
	_, _, store := newStore(t)
	var fired []string
	mustHook(t, store, func(_ context.Context, _ *sql.Tx, r run.Run) error {
		fired = append(fired, r.ID+":"+string(r.State))
		return nil
	})
	drive := func(id string, path ...run.State) {
		t.Helper()
		if _, err := store.Create(ctx, run.NewRun{ID: id, UserID: "alice"}); err != nil {
			t.Fatal(err)
		}
		for _, st := range path {
			if _, err := store.Transition(ctx, id, st, run.TransitionOptions{}); err != nil {
				t.Fatalf("%s -> %s: %v", id, st, err)
			}
		}
	}
	drive("r-done", run.StateQueued, run.StateClaimed, run.StateRunning, run.StateCompleted)
	drive("r-tomb", run.StateQueued, run.StateClaimed, run.StateRunning, run.StateTombstoned)
	drive("r-gate", run.StateQueued, run.StateClaimed, run.StateRunning, run.StateParked, run.StateDiedAtGate)
	drive("r-final", run.StateQueued, run.StateClaimed, run.StateRunning, run.StateParked, run.StateFinalized)
	drive("r-crash", run.StateQueued, run.StateClaimed, run.StateRunning, run.StateCrashed)

	want := map[string]bool{
		"r-done:completed": true, "r-tomb:tombstoned": true,
		"r-gate:died-at-gate": true, "r-final:finalized": true,
	}
	got := map[string]bool{}
	for _, f := range fired {
		got[f] = true
	}
	for w := range want {
		if !got[w] {
			t.Errorf("the hook did not fire for %q", w)
		}
	}
	if got["r-crash:crashed"] {
		t.Error("the hook fired for a crashed run — recovery still supersedes it, so the run has not ended")
	}
	if len(fired) != len(want) {
		t.Errorf("hook fired %d times (%v), want %d", len(fired), fired, len(want))
	}
}

// mustHook installs a terminal hook, failing the test if it is refused.
func mustHook(t *testing.T, s *run.Store, h run.TerminalHook) {
	t.Helper()
	if err := s.SetTerminalHook(h); err != nil {
		t.Fatalf("SetTerminalHook: %v", err)
	}
}

// TestTerminalHookInstallsExactlyOnce is drain D11: the discipline is
// structural. A second install is REFUSED, never a silent swap — a composition
// defect that wired two summary writers would otherwise produce two summaries
// per run, and the doc comment asking callers to behave was its only guard.
func TestTerminalHookInstallsExactlyOnce(t *testing.T) {
	_, _, store := newStore(t)
	first := func(context.Context, *sql.Tx, run.Run) error { return nil }
	if err := store.SetTerminalHook(first); err != nil {
		t.Fatalf("the first install must succeed: %v", err)
	}
	if err := store.SetTerminalHook(first); err == nil {
		t.Error("a second SetTerminalHook succeeded — the hook is install-once")
	}
	if err := store.SetTerminalHook(nil); err == nil {
		t.Error("a nil hook was accepted")
	}
	// A fresh Store installs cleanly — once-ness is per-Store, not global.
	_, _, other := newStore(t)
	if err := other.SetTerminalHook(first); err != nil {
		t.Errorf("a fresh store must accept its own hook: %v", err)
	}
}

// TestTerminalHookIsRaceFree exercises the atomic read under concurrent
// terminal transitions (-race is where this earns its keep).
func TestTerminalHookIsRaceFree(t *testing.T) {
	ctx := context.Background()
	_, _, store := newStore(t)
	var mu sync.Mutex
	fired := 0
	mustHook(t, store, func(context.Context, *sql.Tx, run.Run) error {
		mu.Lock()
		fired++
		mu.Unlock()
		return nil
	})
	ids := []string{"c1", "c2", "c3", "c4"}
	for _, id := range ids {
		if _, err := store.Create(ctx, run.NewRun{ID: id, UserID: "alice"}); err != nil {
			t.Fatal(err)
		}
		for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
			if _, err := store.Transition(ctx, id, st, run.TransitionOptions{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, _ = store.Transition(ctx, id, run.StateCompleted, run.TransitionOptions{})
		}(id)
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if fired != len(ids) {
		t.Errorf("hook fired %d times for %d concurrent terminal transitions", fired, len(ids))
	}
}

package shell

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchdog"
)

// watchdog_seams_test.go — the drain-D7 in-process listener audit and the
// drain-D14 shell-posture wired-path coverage. Both are hermetic: the only
// sockets opened are ephemeral listeners bound and closed in-test — a loopback
// one and, to exercise the audit's trip path, a 0.0.0.0 one — never connected to,
// with no front chain and no paid lane.

// TestAuditOwnListeners (drain D7): the real in-process listener audit passes a
// loopback listener and trips a 0.0.0.0 one — a genuine runtime check over
// /proc, not a re-lint of the immutable config address.
func TestAuditOwnListeners(t *testing.T) {
	// A loopback listener must NOT be reported foreign.
	lo, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen loopback: %v", err)
	}
	defer lo.Close()
	loPort := portOf(t, lo)

	foreign, err := auditOwnListeners()
	if err != nil {
		// Linux-only platform: a missing /proc is a real failure, not a
		// sanctioned skip (CONVENTIONS §10 sanctions only the R/L engine-absence
		// skips).
		t.Fatalf("listener audit failed (Linux /proc required): %v", err)
	}
	if containsPort(foreign, loPort) {
		t.Errorf("a 127.0.0.1 listener was flagged foreign: %v (port %s)", foreign, loPort)
	}

	// A 0.0.0.0 listener (beyond loopback) MUST be reported foreign.
	any, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("listen 0.0.0.0: %v", err)
	}
	defer any.Close()
	anyPort := portOf(t, any)

	foreign, err = auditOwnListeners()
	if err != nil {
		t.Fatalf("audit (0.0.0.0): %v", err)
	}
	if !containsPort(foreign, anyPort) {
		t.Errorf("a 0.0.0.0 listener was NOT flagged foreign (the runtime-drift check missed it): %v (port %s)", foreign, anyPort)
	}
}

func portOf(t *testing.T, ln net.Listener) string {
	t.Helper()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split %v: %v", ln.Addr(), err)
	}
	return port
}

func containsPort(foreign []watchdog.ForeignListener, port string) bool {
	for _, f := range foreign {
		if strings.HasSuffix(f.Addr, ":"+port) {
			return true
		}
	}
	return false
}

// TestWatchdogShellPostureWiredTailToFlag (drain D14): construct the REAL
// watchdog over a test DB in the shell's own composition, start its THREE loops
// exactly as the shell does, drive ONE planted anomaly through the WIRED tail
// path to a flag, then prove the loops stop cleanly on shutdown. The F1/F2 defect
// class lived precisely in this wired system, which had no shell-level test.
func TestWatchdogShellPostureWiredTailToFlag(t *testing.T) {
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
	runs := run.NewStore(db, log)

	// The REAL watchdog in the shell's composition (Duty nil — Tier-0 detection is
	// model-free, so the wired tail still flags; keeps the test $0 and offline).
	wd := watchdog.New(watchdog.Deps{DB: db, Log: log, Runs: runs, Settings: reg})

	// A running run to plant the anomaly on.
	const runID = "t.execute"
	if _, err := runs.Create(ctx, run.NewRun{ID: runID, UserID: "alice", Lane: "anthropic", Substrate: "claude-cli"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, runID, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("transition %s: %v", st, err)
		}
	}

	// Start the three shell-owned loops exactly as the production path does.
	loopCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); watchdogTailLoop(loopCtx, wd, log, discard()) }()
	go func() { defer wg.Done(); watchdogSweepLoop(loopCtx, wd, discard()) }()
	go func() { defer wg.Done(); watchdogDeadManLoop(loopCtx, wd, discard()) }()

	// Let the tail bootstrap its cursor at Head, THEN plant the loop trace so the
	// tail picks it up on its next tick (the 2s cadence is a structural constant).
	time.Sleep(300 * time.Millisecond)
	loopN, _ := reg.Int("watchdog.loop_repeat")
	pay, _ := json.Marshal(map[string]any{"tool": "Bash", "args_digest": "d1", "excerpt": "x"})
	for i := int64(0); i < loopN; i++ {
		r, _ := runs.Get(ctx, runID)
		if _, err := log.Append(ctx, eventlog.Append{
			RunID: runID, Generation: r.Generation, UserID: "alice",
			Type: "tool.completed", SchemaVersion: 1, Payload: pay,
		}); err != nil {
			cancel()
			wg.Wait()
			t.Fatalf("plant loop event: %v", err)
		}
	}

	// Wait for the WIRED tail to detect → flag + park.
	deadline := time.After(10 * time.Second)
	flagged := false
	for !flagged {
		var n int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM run_events WHERE run_id = ? AND type = 'watchdog.flagged'`, runID).Scan(&n); err != nil {
			cancel()
			wg.Wait()
			t.Fatalf("poll flag: %v", err)
		}
		if n > 0 {
			flagged = true
			break
		}
		select {
		case <-deadline:
			cancel()
			wg.Wait()
			t.Fatal("the wired Tier-0 tail did not flag the planted loop within 10s")
		case <-time.After(100 * time.Millisecond):
		}
	}

	if r, _ := runs.Get(ctx, runID); r.State != run.StateParked {
		cancel()
		wg.Wait()
		t.Fatalf("the planted anomaly flagged but the run is %s, want parked (Tier-2 pause-and-flag)", r.State)
	}

	// Shutdown: cancel the process context; the three loops must return cleanly.
	cancel()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the watchdog loops did not stop cleanly on shutdown")
	}
}

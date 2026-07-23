package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// projection_watchdog_test.go — the B5-3 inbox derive-from-log projection (R30):
// open watchdog flags = the latest watchdog.flagged per (run, anomaly class) not
// superseded by a suppress or a resume, owner-scoped per S01.9. Internal test
// (package api) so it drives the projector directly.

func wdTestDB(t *testing.T) (*storage.DB, *eventlog.Log, *run.Store) {
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
	return db, log, run.NewStore(db, log)
}

func runningRun(t *testing.T, runs *run.Store, id, owner string) run.Run {
	t.Helper()
	ctx := context.Background()
	if _, err := runs.Create(ctx, run.NewRun{ID: id, UserID: owner, Lane: "anthropic", Substrate: "claude-cli"}); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, id, st, run.TransitionOptions{Actor: "platform"}); err != nil {
			t.Fatalf("transition %s: %v", id, err)
		}
	}
	r, _ := runs.Get(ctx, id)
	return r
}

func appendFlag(t *testing.T, log *eventlog.Log, runID, owner string, gen int64, class, severity string) int64 {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"rule": class, "anomaly_class": class, "severity": severity, "detail": "d", "actor": "platform",
	})
	seq, err := log.Append(context.Background(), eventlog.Append{
		RunID: runID, Generation: gen, UserID: owner, Type: "watchdog.flagged", SchemaVersion: 1, Payload: payload,
	})
	if err != nil {
		t.Fatalf("append flag: %v", err)
	}
	return seq
}

func TestWatchdogFlagsInboxOwnerScoped(t *testing.T) {
	ctx := context.Background()
	db, log, runs := wdTestDB(t)
	p := &projector{db: db}

	aRun := runningRun(t, runs, "a.execute", "alice")
	bRun := runningRun(t, runs, "b.execute", "bob")
	appendFlag(t, log, aRun.ID, "alice", aRun.Generation, "watchdog.loop", "flag-now")
	appendFlag(t, log, bRun.ID, "bob", bRun.Generation, "watchdog.loop", "flag-now")
	// a per-person spend flag (owner-attributed, run-less)
	if _, err := log.Append(ctx, eventlog.Append{
		UserID: "alice", Type: "watchdog.flagged", SchemaVersion: 1,
		Payload: json.RawMessage(`{"rule":"watchdog.spend","anomaly_class":"watchdog.spend:alice","severity":"flag-now","detail":"spike"}`),
	}); err != nil {
		t.Fatalf("append spend flag: %v", err)
	}
	// a platform-health flag (user_id=platform, run-less)
	if _, err := log.Append(ctx, eventlog.Append{
		UserID: "platform", Type: "watchdog.flagged", SchemaVersion: 1,
		Payload: json.RawMessage(`{"rule":"watchdog.organ_absence","anomaly_class":"watchdog.organ_absence:watchlist","severity":"daily-digest","detail":"down"}`),
	}); err != nil {
		t.Fatalf("append organ flag: %v", err)
	}

	// Operator sees ALL four.
	op, err := p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatalf("operator inbox: %v", err)
	}
	if len(op.WatchdogFlags) != 4 {
		t.Fatalf("operator should see all 4 open flags, got %d: %+v", len(op.WatchdogFlags), op.WatchdogFlags)
	}

	// alice sees ONLY her run flag + her spend flag (2) — not bob's, not platform.
	al, err := p.inbox(ctx, ownerScope{UserID: "alice"})
	if err != nil {
		t.Fatalf("alice inbox: %v", err)
	}
	if len(al.WatchdogFlags) != 2 {
		t.Fatalf("alice should see her 2 flags only, got %d: %+v", len(al.WatchdogFlags), al.WatchdogFlags)
	}
	for _, f := range al.WatchdogFlags {
		if f.Owner != "alice" {
			t.Errorf("alice saw a flag owned by %q — owner-scope leak (S01.9)", f.Owner)
		}
	}
}

// appendRunlessFlag appends a run-less watchdog.flagged (spend/organ/retune),
// owner-attributed, with a suffixed anomaly_class.
func appendRunlessFlag(t *testing.T, log *eventlog.Log, owner, rule, class, severity string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"rule": rule, "anomaly_class": class, "severity": severity, "detail": "d",
	})
	if _, err := log.Append(context.Background(), eventlog.Append{
		UserID: owner, Type: "watchdog.flagged", SchemaVersion: 1, Payload: payload,
	}); err != nil {
		t.Fatalf("append run-less flag: %v", err)
	}
}

// appendRunlessSuppress appends a run-less watchdog.suppressed at platform scope
// carrying the FULL anomaly_class as its rule (what watchdog.Suppress now emits).
func appendRunlessSuppress(t *testing.T, log *eventlog.Log, class string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"rule": class, "count": 1})
	if _, err := log.Append(context.Background(), eventlog.Append{
		UserID: "platform", Type: "watchdog.suppressed", SchemaVersion: 1, Payload: payload,
	}); err != nil {
		t.Fatalf("append run-less suppress: %v", err)
	}
}

// TestWatchdogFlagsExcludeDeadManCanary (drain D4): a canary cycle leaves a
// synthetic watchdog.loop flag on a platform.deadman.* run; the inbox shows the
// real flag and NOT the canary's (which would otherwise sit forever-open).
func TestWatchdogFlagsExcludeDeadManCanary(t *testing.T) {
	ctx := context.Background()
	db, log, runs := wdTestDB(t)
	p := &projector{db: db}

	real := runningRun(t, runs, "a.execute", "alice")
	appendFlag(t, log, real.ID, "alice", real.Generation, "watchdog.loop", "flag-now")
	canary := runningRun(t, runs, "platform.deadman.123", "platform")
	appendFlag(t, log, canary.ID, "platform", canary.Generation, "watchdog.loop", "flag-now")

	op, err := p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatalf("operator inbox: %v", err)
	}
	if len(op.WatchdogFlags) != 1 || op.WatchdogFlags[0].RunID != "a.execute" {
		t.Fatalf("inbox must show the real flag and NOT the dead-man canary's: %+v", op.WatchdogFlags)
	}
}

// TestWatchdogFlagsPlatformSuppressClearsMemberView (drain D5): a platform-scope
// suppress of a run-less flag (spend/organ/retune) clears it in BOTH the operator
// and the owning member's views, WITHOUT exposing any other owner's flags.
func TestWatchdogFlagsPlatformSuppressClearsMemberView(t *testing.T) {
	ctx := context.Background()
	db, log, _ := wdTestDB(t)
	p := &projector{db: db}

	// alice's run-less flags: a spend spike, an organ absence, a retune proposal.
	appendRunlessFlag(t, log, "alice", "watchdog.spend", "watchdog.spend:alice", "flag-now")
	appendRunlessFlag(t, log, "alice", "watchdog.organ_absence", "watchdog.organ_absence:watchlist", "daily-digest")
	appendRunlessFlag(t, log, "alice", "watchdog.retune_proposal", "watchdog.retune_proposal:watchdog.loop_repeat", "daily-digest")
	// bob's own spend flag — alice must never see it.
	appendRunlessFlag(t, log, "bob", "watchdog.spend", "watchdog.spend:bob", "flag-now")

	// Platform-scope suppresses (what Suppress emits for a run-less flag).
	appendRunlessSuppress(t, log, "watchdog.spend:alice")
	appendRunlessSuppress(t, log, "watchdog.organ_absence:watchlist")
	appendRunlessSuppress(t, log, "watchdog.retune_proposal:watchdog.loop_repeat")

	// Operator: alice's three are cleared; only bob's spend remains.
	op, err := p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatalf("operator inbox: %v", err)
	}
	if len(op.WatchdogFlags) != 1 || op.WatchdogFlags[0].AnomalyClass != "watchdog.spend:bob" {
		t.Fatalf("operator: alice's run-less flags should be platform-suppressed, only bob's spend left: %+v", op.WatchdogFlags)
	}

	// alice: her three are cleared by the platform-scope suppress (D5), and she
	// still sees none of bob's.
	al, err := p.inbox(ctx, ownerScope{UserID: "alice"})
	if err != nil {
		t.Fatalf("alice inbox: %v", err)
	}
	if len(al.WatchdogFlags) != 0 {
		t.Fatalf("alice: her run-less flags should clear under a platform-scope suppress (D5): %+v", al.WatchdogFlags)
	}
}

func TestWatchdogFlagsSupersededDropped(t *testing.T) {
	ctx := context.Background()
	db, log, runs := wdTestDB(t)
	p := &projector{db: db}

	aRun := runningRun(t, runs, "a.execute", "alice")
	bRun := runningRun(t, runs, "b.execute", "bob")
	appendFlag(t, log, aRun.ID, "alice", aRun.Generation, "watchdog.loop", "flag-now")
	appendFlag(t, log, bRun.ID, "bob", bRun.Generation, "watchdog.loop", "flag-now")

	// Suppress alice's loop: a watchdog.suppressed(rule=watchdog.loop) at a
	// higher seq clears her flag.
	if _, err := log.Append(ctx, eventlog.Append{
		RunID: aRun.ID, Generation: aRun.Generation, UserID: "alice", Type: "watchdog.suppressed", SchemaVersion: 1,
		Payload: json.RawMessage(`{"rule":"watchdog.loop","count":1}`),
	}); err != nil {
		t.Fatalf("append suppress: %v", err)
	}
	// Resume bob's run: a run.state_changed to=running clears his flag.
	if _, err := log.Append(ctx, eventlog.Append{
		RunID: bRun.ID, Generation: bRun.Generation, UserID: "bob", Type: "run.state_changed", SchemaVersion: 1,
		Payload: json.RawMessage(`{"from":"parked","to":"running","reason":"resume — I was wrong"}`),
	}); err != nil {
		t.Fatalf("append resume: %v", err)
	}

	op, err := p.inbox(ctx, ownerScope{Operator: true})
	if err != nil {
		t.Fatalf("operator inbox: %v", err)
	}
	if len(op.WatchdogFlags) != 0 {
		t.Fatalf("both flags should be superseded (suppress + resume), got %d: %+v", len(op.WatchdogFlags), op.WatchdogFlags)
	}
}

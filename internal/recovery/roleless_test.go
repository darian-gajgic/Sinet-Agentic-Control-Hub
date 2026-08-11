package recovery_test

// roleless_test.go — P3-RW-6 §7 packet B: no production run class is fork-bait
// by its id shape (T-B2, checklist 11), and the dead-man canary in particular
// is safe both alive and orphaned (T-B3, checklist 12).
//
// S02.5 step 3's default disposition for a crashed run is FORK. A fork of a run
// whose id carries NO skeleton role could never dispatch — `Skeleton.Dispatch`
// refuses it, which (before this packet) left a claimed zombie, and the ladder
// then re-forked the corpse: a roleless crash is a fork LOOP, in production,
// with `platform.deadman.*` as the standing bait (the watchdog's canary walks
// claimed/running by direct FSM transitions and an orphaned one ages past
// ⚙ recovery.dead_after like anything else).
//
// The remedy is structural and lives where fork policy already lives: the shell
// composes the NoFork predicate, so a crashed roleless run FINALIZES — the
// honest terminal, since a $0 momentary probe has no paid work to save. The
// §31 watchdog importwall is untouched: nothing here is inside that package.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
)

// shellNoFork is the composition root's predicate, restated here exactly as
// internal/shell composes it (the suffixNoFork precedent: this package learns
// nothing about the classes it names).
func shellNoFork(runID string) bool {
	return stage.IsDirectArmRun(runID) || !stage.Routable(runID)
}

// noForkLadderAt is noForkLadder with the fake clock the §54 fixture note
// requires of any pass that asserts a reap.
func noForkLadderAt(t *testing.T, s *stack, noFork func(string) bool, clock *fakeClock) *recovery.Ladder {
	t.Helper()
	l, err := recovery.New(recovery.Config{
		DB: s.db, Log: s.log, Runs: s.runs, Checkpoints: s.cps, Effects: s.jrnl,
		Settings: s.reg, NoFork: noFork, Now: clock.now,
		Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("recovery.New: %v", err)
	}
	return l
}

func TestRolelessCrashedRunFinalizesNeverForks(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)

	// One pass, one fixture, three classes — so a predicate that matched
	// everything or nothing could not pass.
	roleless := mustRunning(t, s, "platform.deadman.1754870000000000000")
	direct := mustRunning(t, s, "bp-9.direct")
	ordinary := mustRunning(t, s, "t-9.intake")
	for _, r := range []run.Run{roleless, direct, ordinary} {
		if _, err := s.runs.Transition(ctx, r.ID, run.StateCrashed, run.TransitionOptions{
			Reason: "fixture crash", Actor: run.ActorPlatform,
		}); err != nil {
			t.Fatalf("crash %s: %v", r.ID, err)
		}
	}

	rpt, err := noForkLadder(t, s, shellNoFork).ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.Finalized != 2 || rpt.Forked != 1 {
		t.Fatalf("pass finalized=%d forked=%d, want 2 finalized (roleless + direct) and 1 forked (the routable control)",
			rpt.Finalized, rpt.Forked)
	}

	got := getRun(t, s, roleless.ID)
	if got.State != run.StateFinalized {
		t.Errorf("the crashed roleless run is %s, want finalized — its fork could never dispatch", got.State)
	}
	if hasSuccessor(t, s, roleless.ID) {
		t.Error("the crashed roleless run was FORKED — a successor no dispatcher can route")
	}
	// The record must SAY why, or the next reader re-derives the wrong lesson.
	if reason := finalizeReason(t, s, roleless.ID); reason == "" {
		t.Error("the no-fork finalize named no reason in its event detail")
	}

	// The two ratified controls are untouched by the widening.
	if d := getRun(t, s, direct.ID); d.State != run.StateFinalized || hasSuccessor(t, s, direct.ID) {
		t.Errorf("the .direct arm changed behavior: state=%s successor=%v", d.State, hasSuccessor(t, s, direct.ID))
	}
	if !hasSuccessor(t, s, ordinary.ID) {
		t.Error("a routable crashed run was NOT forked — the predicate over-matched")
	}
}

// finalizeReason returns the no-fork reason recorded on the run's finalize
// event, if any.
func finalizeReason(t *testing.T, s *stack, runID string) string {
	t.Helper()
	var payload string
	if err := s.db.QueryRowContext(context.Background(), `
		SELECT payload FROM run_events
		 WHERE run_id = ? AND type = ? AND json_extract(payload, '$.to') = 'finalized'
		 ORDER BY event_seq DESC LIMIT 1`, runID, run.EventState).Scan(&payload); err != nil {
		t.Fatalf("read finalize event for %s: %v", runID, err)
	}
	var ev struct {
		Reason string          `json:"reason"`
		Detail json.RawMessage `json:"detail"`
	}
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		t.Fatalf("decode finalize event: %v", err)
	}
	var d struct {
		NoFork string `json:"no_fork_reason"`
	}
	_ = json.Unmarshal(ev.Detail, &d)
	if strings.TrimSpace(d.NoFork) == "" {
		return ""
	}
	return d.NoFork + " | " + ev.Reason
}

// TestDeadmanMidProbeSweepAndOrphan is checklist 12, in two halves that must
// BOTH hold: a sweep during a HEALTHY probe leaves the canary alone (the
// RW-5 conjunction: fresh appends outrank a dead lease), and an ORPHANED canary
// — the process died before terminateCanary — is crashed and finalized within
// one pass-pair, with no roleless fork chain and no queue row, ever.
func TestDeadmanMidProbeSweepAndOrphan(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	clock := &fakeClock{t: time.Now().UTC()}

	// The canary exactly as internal/watchdog's createCanary builds it: a
	// platform-owned run walked new→queued→claimed→running by direct FSM
	// transitions — no scheduler, no queue row, no lease.
	canaryID := fmt.Sprintf("platform.deadman.%d", clock.now().UnixNano())
	if _, err := s.runs.Create(ctx, run.NewRun{ID: canaryID, UserID: run.ActorPlatform, Lane: "local", Substrate: "local"}); err != nil {
		t.Fatalf("create canary: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := s.runs.Transition(ctx, canaryID, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("walk canary to %s: %v", st, err)
		}
	}
	// The probe's own synthetic trace: fresh appends, exactly like a live probe.
	if _, err := s.log.Append(ctx, eventlog.Append{
		RunID: canaryID, Generation: 0, UserID: run.ActorPlatform,
		Type: "tool.completed", SchemaVersion: 1,
		Payload: json.RawMessage(`{"tool":"deadman.synthetic","args_digest":"sha256:x"}`),
	}); err != nil {
		t.Fatalf("append canary trace: %v", err)
	}

	// Pass 1, DURING the probe: nothing is decided about a run whose cursor is
	// seconds old, even though it holds no lease at all.
	l := noForkLadderAt(t, s, shellNoFork, clock)
	rpt, err := l.ReconcilePass(ctx)
	if err != nil {
		t.Fatalf("mid-probe pass: %v", err)
	}
	if rpt.Alive+rpt.Pending != 1 || rpt.Forked != 0 || rpt.Finalized != 0 {
		t.Fatalf("mid-probe pass %+v — a healthy canary must be left alone (ALIVE/Pending)", rpt)
	}
	if got := getRun(t, s, canaryID); got.State != run.StateRunning {
		t.Fatalf("the canary is %s mid-probe, want running", got.State)
	}

	// Now ORPHAN it: the process died before terminateCanary, and the clock runs
	// past the S02.5 reap bound (§54 fixture note).
	clock.advance(30 * time.Minute)
	if _, err := l.ReconcilePass(ctx); err != nil {
		t.Fatalf("orphan pass: %v", err)
	}
	got := getRun(t, s, canaryID)
	if got.State != run.StateFinalized {
		t.Fatalf("the orphaned canary is %s, want finalized (crashed + bound in one pass-pair)", got.State)
	}
	if hasSuccessor(t, s, canaryID) {
		t.Fatal("the orphaned canary was FORKED — the roleless fork chain is back")
	}
	var forks, queued int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM runs WHERE run_id LIKE 'platform.deadman.%' AND run_id LIKE '%.g%'`).Scan(&forks); err != nil {
		t.Fatalf("count canary forks: %v", err)
	}
	if forks != 0 {
		t.Fatalf("%d canary fork rows exist, want 0", forks)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM queue WHERE run_id LIKE 'platform.deadman.%'`).Scan(&queued); err != nil {
		t.Fatalf("count canary queue rows: %v", err)
	}
	if queued != 0 {
		t.Fatalf("%d queue rows exist for the canary lineage, want 0 — a probe never enters the queue", queued)
	}
}

package watchdog

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

// tier0_test.go — the deterministic counters (brief R1–R8, rubric 1–5). Each
// synthetic trace trips the counter at exactly the ⚙ threshold and the run
// parks (Tier-2 always).

func TestLoopTripsAtThresholdAndParks(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	e.runningRun("task.execute", "alice")

	loopN, _ := e.reg.Int(keyLoopRepeat) // default 5
	// loopN-1 identical calls: below threshold, no trip.
	for i := int64(0); i < loopN-1; i++ {
		e.toolEvent("task.execute", "Bash", "d1", false)
	}
	if err := w.EvaluateRun(ctx, "task.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	if fs := e.flagsFor("task.execute"); len(fs) != 0 {
		t.Fatalf("loop tripped below threshold (%d calls): %+v", loopN-1, fs)
	}
	if e.state("task.execute") != run.StateRunning {
		t.Fatalf("run parked below threshold")
	}

	// One more identical call reaches the threshold → trip + park.
	e.toolEvent("task.execute", "Bash", "d1", false)
	if err := w.EvaluateRun(ctx, "task.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	fs := e.flagsFor("task.execute")
	if len(fs) != 1 || fs[0].Rule != RuleLoop {
		t.Fatalf("loop did not trip at threshold %d: %+v", loopN, fs)
	}
	if !fs[0].Parked || e.state("task.execute") != run.StateParked {
		t.Fatalf("run not parked on the loop flag (Tier-2 pause-and-flag): state=%s parked=%v", e.state("task.execute"), fs[0].Parked)
	}
	if fs[0].Severity != SeverityFlagNow {
		t.Errorf("loop severity = %q, want flag-now", fs[0].Severity)
	}
}

func TestLoopLiveAppliesRaisedThreshold(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	e.runningRun("task.execute", "alice")

	// Raise the loop threshold to 7 (an operator override, read live at
	// evaluation time — §6/§7). 5 identical calls no longer trip.
	if err := e.reg.Set(ctx, settings.SetRequest{Key: keyLoopRepeat, Value: json.RawMessage(`7`), Actor: settings.Actor{Kind: settings.ActorOperator, ID: "op"}, Reason: "test"}); err != nil {
		t.Fatalf("raise loop_repeat: %v", err)
	}
	for i := 0; i < 5; i++ {
		e.toolEvent("task.execute", "Bash", "d1", false)
	}
	if err := w.EvaluateRun(ctx, "task.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	if fs := e.flagsFor("task.execute"); len(fs) != 0 {
		t.Fatalf("loop tripped at 5 after the threshold was raised to 7 (not live-applied): %+v", fs)
	}
	for i := 0; i < 2; i++ {
		e.toolEvent("task.execute", "Bash", "d1", false)
	}
	if err := w.EvaluateRun(ctx, "task.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	if fs := e.flagsFor("task.execute"); len(fs) != 1 {
		t.Fatalf("loop did not trip at the raised threshold 7: %+v", fs)
	}
}

func TestPingPongTrips(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	e.runningRun("task.execute", "bob")

	ppN, _ := e.reg.Int(keyPingPongCycles) // default 6 cycles = 12 alternating calls
	for i := int64(0); i < ppN*2; i++ {
		if i%2 == 0 {
			e.toolEvent("task.execute", "Read", "a", false)
		} else {
			e.toolEvent("task.execute", "Write", "b", false)
		}
	}
	if err := w.EvaluateRun(ctx, "task.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	fs := e.flagsFor("task.execute")
	if len(fs) != 1 || fs[0].Rule != RulePingPong {
		t.Fatalf("ping-pong did not trip over %d cycles: %+v", ppN, fs)
	}
	if e.state("task.execute") != run.StateParked {
		t.Fatalf("ping-pong did not park the run")
	}
}

func TestErrorLoopKeysOnActionNotSignature(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	e.runningRun("task.execute", "carol")

	errN, _ := e.reg.Int(keyErrorLoop) // default 3
	// Same tool, DIFFERENT args each time (distinct signatures → loop cannot
	// trip), all errors — error-loop keys on the ACTION (tool name).
	for i := int64(0); i < errN; i++ {
		e.toolEvent("task.execute", "Bash", "args-"+string(rune('a'+i)), true)
	}
	if err := w.EvaluateRun(ctx, "task.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	fs := e.flagsFor("task.execute")
	if len(fs) != 1 || fs[0].Rule != RuleErrorLoop {
		t.Fatalf("error-loop did not trip on %d same-action errors: %+v", errN, fs)
	}

	// A non-error same-action repeat does NOT trip error-loop.
	e2 := newEnv(t)
	w2 := e2.wd()
	e2.runningRun("t2.execute", "carol")
	for i := int64(0); i < errN; i++ {
		e2.toolEvent("t2.execute", "Grep", "q-"+string(rune('a'+i)), false)
	}
	if err := w2.EvaluateRun(ctx, "t2.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	if fs := e2.flagsFor("t2.execute"); len(fs) != 0 {
		t.Fatalf("error-loop tripped on non-error repeats: %+v", fs)
	}
}

func TestSuspiciousCompletionByClass(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()

	// An execution run that completed with ZERO tools + ZERO verdicts → flag.
	e.runningRun("silent.execute", "dave")
	e.completeRun("silent.execute")
	if err := w.EvaluateRun(ctx, "silent.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	fs := e.flagsFor("silent.execute")
	if len(fs) != 1 || fs[0].Rule != RuleSuspiciousCompletion {
		t.Fatalf("suspicious-completion did not flag a silent execution run: %+v", fs)
	}
	if fs[0].Severity != SeverityDailyDigest {
		t.Errorf("suspicious-completion severity = %q, want daily-digest", fs[0].Severity)
	}
	if fs[0].Parked {
		t.Errorf("a terminal run must not be parked (it already ended)")
	}

	// An execution run that DID produce a tool → not suspicious.
	e.runningRun("busy.execute", "dave")
	e.toolEvent("busy.execute", "Bash", "d", false)
	e.completeRun("busy.execute")
	if err := w.EvaluateRun(ctx, "busy.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	if fs := e.flagsFor("busy.execute"); len(fs) != 0 {
		t.Fatalf("suspicious-completion flagged a run that produced a tool: %+v", fs)
	}

	// A ceremony (intake) run with zero activity is EXEMPT.
	e.runningRun("t.intake", "dave")
	e.completeRun("t.intake")
	if err := w.EvaluateRun(ctx, "t.intake"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	if fs := e.flagsFor("t.intake"); len(fs) != 0 {
		t.Fatalf("suspicious-completion flagged a ceremony run: %+v", fs)
	}

	// A verify run that produced a VERDICT (but no tools) is not suspicious —
	// the floor counts verdicts too (silent-failure = zero tools AND zero verdicts).
	e.runningRun("v.verify", "dave")
	e.verdictEvent("v.verify")
	e.completeRun("v.verify")
	if err := w.EvaluateRun(ctx, "v.verify"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	if fs := e.flagsFor("v.verify"); len(fs) != 0 {
		t.Fatalf("suspicious-completion flagged a verify run that recorded a verdict: %+v", fs)
	}
}

// TestCrashedRunNotFlagged (drain D8, demonstrated): a crashed run is NOT in the
// Tier-0 behavioral watch set, so a crash-shaped loop trace never earns an
// unresolvable flag. run.IsTerminal(StateCrashed)=false by design (crashed is
// terminal-but-supersedable), so the old IsTerminal gate let a crashed run fall
// through to the counters and receive a loop flag it could never resume away.
// FAILS against pre-drain code (which flags it).
func TestCrashedRunNotFlagged(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	e.runningRun("t.execute", "alice")
	for i := 0; i < 5; i++ {
		e.toolEvent("t.execute", "Bash", "d1", false)
	}
	// running → crashed (a ratified edge; recovery owns the crashed row).
	if _, err := e.runs.Transition(ctx, "t.execute", run.StateCrashed, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
		t.Fatalf("crash: %v", err)
	}
	if err := w.EvaluateRun(ctx, "t.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	if fs := e.flagsFor("t.execute"); len(fs) != 0 {
		t.Fatalf("a crashed run with a loop trace was flagged — crashed is recovery's, not the watchdog's (D8): %+v", fs)
	}
}

// TestCompletedSentinelMatchesRunState guards the value-comparison the
// suspicious-completion check uses (isCleanCompletion reads the state by VALUE,
// never the StateCompleted ident — deadman.go's importwall exception, D10b) so it
// cannot silently drift from the run FSM.
func TestCompletedSentinelMatchesRunState(t *testing.T) {
	if completedState != string(run.StateCompleted) {
		t.Fatalf("completedState %q drifted from run.StateCompleted %q", completedState, run.StateCompleted)
	}
}

// TestSuspiciousCompletionCountsPreResumeWork (drain-R1, demonstrated): the
// generation fence is BEHAVIORAL-only. A run that did real tool work at
// generation 0, parked, resumed (gen→1), and completed with NO further tool
// calls is NOT a silent failure — the whole-run (unfenced) read counts the
// gen-0 work, so no suspicious-completion flag. Direction (b), a genuinely
// tool-less completed run STILL flagging, is covered by
// TestSuspiciousCompletionByClass (silent.execute). This test FAILS against the
// pre-R1 fenced read (which counts only gen-1 → 0 tools → false flag).
func TestSuspiciousCompletionCountsPreResumeWork(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	e.runningRun("work.execute", "alice")
	// real tool work at generation 0
	e.toolEvent("work.execute", "Bash", "d1", false)
	e.toolEvent("work.execute", "Grep", "d2", false)
	// park → resume (generation bumps to 1) → complete with NO further tool calls
	if _, err := e.runs.Transition(ctx, "work.execute", run.StateParked, run.TransitionOptions{Reason: "watchdog:watchdog.loop", Actor: run.ActorPlatform}); err != nil {
		t.Fatalf("park: %v", err)
	}
	if _, err := e.runs.Transition(ctx, "work.execute", run.StateRunning, run.TransitionOptions{Reason: "resume — I was wrong", Actor: "alice"}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	e.completeRun("work.execute")
	if err := w.EvaluateRun(ctx, "work.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	if fs := e.flagsFor("work.execute"); len(fs) != 0 {
		t.Fatalf("a run that did real work before a resume was falsely flagged suspicious-completion — the fence blinded the whole-run read (R1): %+v", fs)
	}
}

func TestTailEvaluatesRunsWithNewEvents(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	e.runningRun("task.execute", "erin")

	head, _ := e.log.Head(ctx)
	for i := 0; i < 5; i++ {
		e.toolEvent("task.execute", "Bash", "same", false)
	}
	next, err := w.Tail(ctx, head)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if next <= head {
		t.Fatalf("Tail cursor did not advance: %d ≤ %d", next, head)
	}
	if fs := e.flagsFor("task.execute"); len(fs) != 1 || fs[0].Rule != RuleLoop {
		t.Fatalf("Tail did not drive Tier-0 to a loop flag: %+v", fs)
	}
}

// TestTier0Stateless proves the counters re-derive from the log (no durable
// cursor/state): a fresh Watchdog over the SAME db, given no prior state,
// flags the loop purely from run_events.
func TestTier0Stateless(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.runningRun("task.execute", "frank")
	for i := 0; i < 5; i++ {
		e.toolEvent("task.execute", "Bash", "same", false)
	}
	// A brand-new Watchdog instance (no in-memory state) still detects.
	fresh := New(Deps{DB: e.db, Log: e.log, Runs: e.runs, Settings: e.reg})
	if err := fresh.EvaluateRun(ctx, "task.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	if fs := e.flagsFor("task.execute"); len(fs) != 1 {
		t.Fatalf("stateless re-derivation did not flag: %+v", fs)
	}
}

// TestNoSignatureNoTrip: tool.completed rows lacking a tool name carry no
// signature, so a loop cannot trip on them (the safe direction — honest for a
// non-conformant adapter payload, the B5-2 F13 carry).
func TestNoSignatureNoTrip(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	e.runningRun("task.execute", "grace")
	for i := 0; i < 8; i++ {
		if _, err := e.log.Append(ctx, eventlog.Append{
			RunID: "task.execute", Generation: e.gen("task.execute"), UserID: "grace",
			Type: "tool.completed", SchemaVersion: 1, Payload: json.RawMessage(`{"tool_use_id":"x","excerpt":"exit 0"}`),
		}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := w.EvaluateRun(ctx, "task.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	if fs := e.flagsFor("task.execute"); len(fs) != 0 {
		t.Fatalf("loop tripped on unsignatured tool events: %+v", fs)
	}
}

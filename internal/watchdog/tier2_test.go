package watchdog

import (
	"context"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// tier2_test.go — pause-and-flag (brief R15–R22, rubric 10/12/14). The park and
// the watchdog.flagged commit in ONE WriteTx; alerts dedup per (run, class);
// resume clears the flag.

// TestParkAndFlagInSameTx: the run.state_changed→parked and the watchdog.flagged
// are contiguous in event_seq (one WriteTx, no interleaving under the
// single-writer posture) and at the SAME generation — the §8 transition
// discipline.
func TestParkAndFlagInSameTx(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t) // no duty — keep the event sequence clean for the contiguity proof
	w := e.wd()
	e.runningRun("t.execute", "alice")
	genBefore := e.gen("t.execute")
	for i := 0; i < 5; i++ {
		e.toolEvent("t.execute", "Bash", "d1", false)
	}
	if err := w.EvaluateRun(ctx, "t.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}

	parkedSeq := e.seqOf(t, "t.execute", "run.state_changed")
	flagSeq := e.seqOf(t, "t.execute", "watchdog.flagged")
	if flagSeq != parkedSeq+1 {
		t.Fatalf("watchdog.flagged (seq %d) is not contiguous with the park (seq %d) — not one WriteTx", flagSeq, parkedSeq)
	}
	// Park does not bump generation (only resume does); both events at genBefore.
	if g := e.gen("t.execute"); g != genBefore {
		t.Errorf("park bumped the generation to %d (want %d — only resume bumps)", g, genBefore)
	}
	if e.state("t.execute") != run.StateParked {
		t.Fatalf("run not parked")
	}
}

// TestDedupOneAlertPerRunClass: a second evaluation does not raise a second
// flag-now for an already-open (run, class); later evidence folds in (R22).
func TestDedupOneAlertPerRunClass(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	e.runningRun("t.execute", "bob")
	for i := 0; i < 5; i++ {
		e.toolEvent("t.execute", "Bash", "d1", false)
	}
	if err := w.EvaluateRun(ctx, "t.execute"); err != nil {
		t.Fatalf("EvaluateRun 1: %v", err)
	}
	// The run is now parked; re-evaluate (a stateless re-derivation would see the
	// same trace). No second flag.
	if err := w.EvaluateRun(ctx, "t.execute"); err != nil {
		t.Fatalf("EvaluateRun 2: %v", err)
	}
	if fs := e.flagsFor("t.execute"); len(fs) != 1 {
		t.Fatalf("dedup failed — want 1 open flag, got %d", len(fs))
	}
}

// TestResumeClearsFlag: after "resume — I was wrong" (parked→running), the flag
// is no longer open, and a fresh loop can flag again (R18/R30).
func TestResumeClearsFlag(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	e.runningRun("t.execute", "carol")
	for i := 0; i < 5; i++ {
		e.toolEvent("t.execute", "Bash", "d1", false)
	}
	if err := w.EvaluateRun(ctx, "t.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	// open flag exists
	open, err := w.openFlagExists(ctx, "t.execute", RuleLoop)
	if err != nil || !open {
		t.Fatalf("expected an open loop flag before resume (open=%v err=%v)", open, err)
	}
	// resume — the parked→running transition (the existing resume edge)
	if _, err := e.runs.Transition(ctx, "t.execute", run.StateRunning, run.TransitionOptions{Reason: "resume — I was wrong", Actor: "carol"}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	open, err = w.openFlagExists(ctx, "t.execute", RuleLoop)
	if err != nil {
		t.Fatalf("openFlagExists after resume: %v", err)
	}
	if open {
		t.Fatalf("resume did not clear the open flag (R18/R30)")
	}
	// a fresh loop after resume flags again (not deduped against the cleared one)
	for i := 0; i < 5; i++ {
		e.toolEvent("t.execute", "Bash", "d1", false)
	}
	if err := w.EvaluateRun(ctx, "t.execute"); err != nil {
		t.Fatalf("EvaluateRun after resume: %v", err)
	}
	if fs := e.flagsFor("t.execute"); len(fs) != 2 {
		t.Fatalf("a fresh post-resume loop should flag again: want 2 flags, got %d", len(fs))
	}
}

func (e *env) seqOf(t *testing.T, runID, typ string) int64 {
	t.Helper()
	var seq int64
	if err := e.db.QueryRowContext(context.Background(),
		`SELECT event_seq FROM run_events WHERE run_id=? AND type=? ORDER BY event_seq DESC LIMIT 1`, runID, typ).
		Scan(&seq); err != nil {
		t.Fatalf("seqOf(%s,%s): %v", runID, typ, err)
	}
	return seq
}

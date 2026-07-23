package watchdog

import (
	"context"
	"encoding/json"
	"testing"
)

// suppress_test.go — the per-rule suppression + retune proposal (brief R19,
// rubric 13). A suppress is a tuning signal; the N-th suppression of a rule
// PROPOSES a threshold raise — a proposal, NEVER an auto-move.

func TestSuppressEmitsAndSupersedes(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	e.runningRun("t.execute", "alice")
	for i := 0; i < 5; i++ {
		e.toolEvent("t.execute", "Bash", "d1", false)
	}
	if err := w.EvaluateRun(ctx, "t.execute"); err != nil {
		t.Fatalf("EvaluateRun: %v", err)
	}
	if err := w.Suppress(ctx, "t.execute", RuleLoop); err != nil {
		t.Fatalf("Suppress: %v", err)
	}
	sups := e.eventsOfType(EventSuppressed)
	if len(sups) != 1 {
		t.Fatalf("want 1 watchdog.suppressed, got %d", len(sups))
	}
	var sp SuppressPayload
	if err := json.Unmarshal(sups[0], &sp); err != nil {
		t.Fatalf("decode suppress: %v", err)
	}
	if sp.Rule != RuleLoop || sp.Count != 1 || sp.Propose {
		t.Errorf("suppress = %+v, want rule=loop count=1 propose=false", sp)
	}
	// The flag is now superseded (no longer open).
	open, err := w.openFlagExists(ctx, "t.execute", RuleLoop)
	if err != nil || open {
		t.Errorf("suppress did not supersede the open flag (open=%v err=%v)", open, err)
	}
}

func TestRetuneProposalOnNthSuppressNeverAutoMoves(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()

	retuneN, _ := e.reg.Int(keySuppressRetune) // default 2
	loopBefore, _ := e.reg.Int(keyLoopRepeat)

	// Suppress the loop rule retuneN times (on distinct runs).
	for i := int64(0); i < retuneN; i++ {
		id := "t" + string(rune('a'+i)) + ".execute"
		e.runningRun(id, "bob")
		for j := 0; j < 5; j++ {
			e.toolEvent(id, "Bash", "d1", false)
		}
		if err := w.EvaluateRun(ctx, id); err != nil {
			t.Fatalf("EvaluateRun: %v", err)
		}
		if err := w.Suppress(ctx, id, RuleLoop); err != nil {
			t.Fatalf("Suppress %d: %v", i, err)
		}
	}

	// The retuneN-th suppression proposes a threshold raise (a platform flag).
	platFlags := e.flagsFor("")
	var found bool
	for _, f := range platFlags {
		if f.Rule == RuleRetuneProposal && f.Signature == keyLoopRepeat {
			found = true
			if f.Severity != SeverityDailyDigest {
				t.Errorf("retune proposal severity = %q, want daily-digest", f.Severity)
			}
		}
	}
	if !found {
		t.Fatalf("no retune proposal after %d suppressions of the loop rule: %+v", retuneN, platFlags)
	}
	// NEVER an auto-move: the ⚙ threshold is unchanged.
	if after, _ := e.reg.Int(keyLoopRepeat); after != loopBefore {
		t.Errorf("retune AUTO-MOVED ⚙ %s from %d to %d — must be a proposal only (R19/§6)", keyLoopRepeat, loopBefore, after)
	}
}

// TestSuppressStructuralRuleNoProposal: a rule with a STRUCTURAL threshold
// (suspicious-completion) has no ⚙ to raise, so no retune proposal is made even
// at the retune count.
func TestSuppressStructuralRuleNoProposal(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd()
	retuneN, _ := e.reg.Int(keySuppressRetune)
	for i := int64(0); i < retuneN; i++ {
		if err := w.Suppress(ctx, "", RuleSuspiciousCompletion); err != nil {
			t.Fatalf("Suppress: %v", err)
		}
	}
	for _, f := range e.flagsFor("") {
		if f.Rule == RuleRetuneProposal {
			t.Errorf("a structural-threshold rule must not propose a ⚙ retune: %+v", f)
		}
	}
}

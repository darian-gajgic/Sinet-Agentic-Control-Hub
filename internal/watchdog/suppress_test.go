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
	if err := w.Suppress(ctx, "carol", "t.execute", RuleLoop); err != nil {
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
	// The acting principal rides the payload (drain D13), not the platform.
	if sp.Actor != "carol" {
		t.Errorf("suppress actor = %q, want the acting principal carol (not platform) (D13)", sp.Actor)
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
		if err := w.Suppress(ctx, "op", id, RuleLoop); err != nil {
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

// TestSuppressClearsSuffixedRunlessFlag (drain D5): a run-less flag's
// anomaly_class is SUFFIXED (watchdog.organ_absence:<organ>), so a suppress must
// carry the FULL class to clear it — passing the bare rule (as callers did) left
// those flags permanently un-clearable (they have no resume). The retune count
// still folds into the BASE rule.
func TestSuppressClearsSuffixedRunlessFlag(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	w := e.wd(func(d *Deps) {
		d.Organs = func(context.Context) ([]OrganStatus, error) {
			return []OrganStatus{{Organ: "watchlist", Up: false, Note: "unit inactive"}}, nil
		}
	})
	if err := w.checkOrgans(ctx); err != nil {
		t.Fatalf("checkOrgans: %v", err)
	}
	class := RuleOrganAbsence + ":watchlist"
	if open, err := w.openOwnerlessFlagExists(ctx, class); err != nil || !open {
		t.Fatalf("expected an open suffixed organ-absence flag (open=%v err=%v)", open, err)
	}
	// Suppress with the FULL suffixed class — the run-less flag has no resume, so
	// this is its only path to cleared.
	if err := w.Suppress(ctx, "op", "", class); err != nil {
		t.Fatalf("Suppress: %v", err)
	}
	if open, err := w.openOwnerlessFlagExists(ctx, class); err != nil || open {
		t.Fatalf("a suffixed run-less flag was not cleared by a full-class suppress (open=%v err=%v)", open, err)
	}
	// The suppress folded into the BASE rule's count (not the suffixed instance).
	n, err := w.suppressCount(ctx, RuleOrganAbsence)
	if err != nil || n != 1 {
		t.Errorf("suppressCount(base) = %d (err=%v), want 1 — the suffixed suppress folds into the base rule (D5)", n, err)
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
		if err := w.Suppress(ctx, "op", "", RuleSuspiciousCompletion); err != nil {
			t.Fatalf("Suppress: %v", err)
		}
	}
	for _, f := range e.flagsFor("") {
		if f.Rule == RuleRetuneProposal {
			t.Errorf("a structural-threshold rule must not propose a ⚙ retune: %+v", f)
		}
	}
}

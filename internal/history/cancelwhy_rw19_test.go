package history_test

// cancelwhy_rw19_test.go — P3-RW-19 acceptance tests, committed RED with the
// brief (Amendment-A carve-out, CONVENTIONS §3; the red window closes at the
// packet's implementation commit).
//
// Walk-proven residue (2026-08-18 final gate walk, kept world ~/.sinet-b6-final,
// cancelled task t-fe5ff6c325967c3e): the History view `status.tasks_cancelled`
// renders `cancelled_by` as "—" on pre-parity rows, because its SQL derives the
// actor only from the structured leg ($.detail.actor, NULLIF-platform fallback)
// while the task page's derive (internal/api/reads.go cancelDecisions) has a
// second leg reading the `ledger.decide` record. Same task, two answers: the
// banner says "cancelled by op", the History row says "—". And no surface can
// serve a human MOTIVE at all — every "why" renders rule citations.
//
// These tests pin the brief's serve-side contract for the History row
// (P3/briefs/P3-RW-19.md §Wire): `cancelled_by` served by the SAME two-leg
// reading reads.go uses (structured wins, ledger fills, NULLIF-platform last);
// `reason` = the human's own words at $.detail.reason or the frozen honest
// absence "no reason given" (S14.10 honest-absence posture, the §37
// UNPRICED/'(no project)' precedent — this row has no renderer between it and
// the reader, so the honest line must be IN the row); `cause` = the mechanical
// ending in plain words, no rule citations (§57 drafting rule 1).
//
// Subtests marked CONTROL are expected green today; they make the red
// assertions non-tautological and pin behavior the fix must not break.

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/history"
)

// seedPreParityCancel seeds the walk's own record shape: a cancelled task whose
// run carries the PRE-parity card-cancel pair — a run.state_changed with actor
// "platform" and no detail (matched by the compat leg's "(4.5)"), plus the
// ledger_update ledger.decide event whose LAST entry is authored human. The
// who lives ONLY in the ledger record.
func seedPreParityCancel(f *fixture, owner, taskID, runID string) {
	f.task(taskID, owner, "Task of "+owner, "cancelled")
	f.run(runID, owner, taskID, "finalized", "lane-"+owner)
	f.event(runID, owner, "ledger_update", map[string]any{
		"change": map[string]any{"verb": "ledger.decide", "actor": owner, "stage": "verify"},
		"ledger": map[string]any{
			"task_id": taskID,
			"decisions": []map[string]any{
				{"seq": 1, "ts": "2026-08-17T00:20:00Z", "stage": "triage", "author": "platform",
					"text": "triage: family=software", "reason": "Stage-0 (S06.2)"},
				{"seq": 2, "ts": "2026-08-17T00:31:34Z", "stage": "verify", "author": "human",
					"text":   "requester cancelled at the CHECK-INTEGRITY card",
					"reason": "cancel is always available (4.5); parked→finalized, finalize-with-card (S02.3)"},
			},
		},
	})
	f.event(runID, owner, "run.state_changed", map[string]any{
		"from": "parked", "to": "finalized",
		"reason": "verification cancelled at the card (4.5): finalize-with-card",
		"actor":  "platform",
	})
}

// seedPostParityCancel seeds the post-parity verb-path shape: the transition
// carries the structured detail the ONE constructor writes (run.CancelDetail),
// here with a human reason given at the cancel affordance.
func seedPostParityCancel(f *fixture, owner, taskID, runID, reason string) {
	f.task(taskID, owner, "Task of "+owner, "cancelled")
	f.run(runID, owner, taskID, "completed", "lane-"+owner)
	detail := map[string]any{"cause": "human cancel (4.5)", "actor": owner, "ladder_invoked": true}
	if reason != "" {
		detail["reason"] = reason
	}
	f.event(runID, owner, "run.state_changed", map[string]any{
		"from": "running", "to": "completed",
		"reason": "cancelled by " + owner + " (4.5)",
		"actor":  owner,
		"detail": detail,
	})
}

func runCancelled(t *testing.T, f *fixture, scope history.Scope) history.Answer {
	t.Helper()
	a, err := f.st.RunQuery(f.ctx, "status.tasks_cancelled", nil, scope, 20)
	if err != nil {
		t.Fatalf("RunQuery(status.tasks_cancelled): %v", err)
	}
	return a
}

// TestHistoryServesThePreParityCancelsHuman — residue (a), the walk's own
// defect: a pre-parity row (who recorded ONLY in the ledger.decide record) must
// serve the human in `cancelled_by`, via the same ledger leg the task page
// reads. Today the view serves NULL and the reader is shown "—" for a cancel
// the task page attributes — actively deceptive the day owner ≠ canceller.
func TestHistoryServesThePreParityCancelsHuman(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	seedPreParityCancel(f, member1, "t-pre", "r-pre")

	a := runCancelled(t, f, opScope())
	if a.RowCount != 1 {
		t.Fatalf("served %d rows, want 1: %v", a.RowCount, a.Rows)
	}
	if got := str(cell(t, a, 0, "cancelled_by")); got != member1 {
		t.Errorf("cancelled_by = %q, want %q — the ledger.decide record names the human and the view must serve it (S02.2 reconstruction; S14.10; 15.6)", got, member1)
	}
}

// TestHistoryCancelReasonIsHonestWhenAbsent — residue (b), the absence half:
// no human reason exists on a pre-parity row, and the row must SAY so with the
// frozen honest-absence literal — never a rule citation posing as a motive,
// never a NULL the renderer turns into "—" (S14.10; §37 honest-absence
// posture).
func TestHistoryCancelReasonIsHonestWhenAbsent(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	seedPreParityCancel(f, member1, "t-pre", "r-pre")

	a := runCancelled(t, f, opScope())
	if a.RowCount != 1 {
		t.Fatalf("served %d rows, want 1: %v", a.RowCount, a.Rows)
	}
	if got := str(cell(t, a, 0, "reason")); got != "no reason given" {
		t.Errorf("reason = %q, want the honest absence %q — a rule citation is not a motive (walk residue (b))", got, "no reason given")
	}
}

// TestHistoryServesTheHumanReasonWhenOneWasGiven — residue (b), the presence
// half: a reason captured at the cancel affordance rides $.detail.reason (the
// ONE constructor's additive field) and the History row serves it verbatim.
func TestHistoryServesTheHumanReasonWhenOneWasGiven(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	seedPostParityCancel(f, member1, "t-post", "r-post", "taking a different approach")

	a := runCancelled(t, f, opScope())
	if a.RowCount != 1 {
		t.Fatalf("served %d rows, want 1: %v", a.RowCount, a.Rows)
	}
	if got := str(cell(t, a, 0, "reason")); got != "taking a different approach" {
		t.Errorf("reason = %q, want the human's own words from $.detail.reason", got)
	}
}

// TestHistoryCancelCauseIsPlainWords — residue (b), the mechanical half: the
// row carries a `cause` column rendering HOW the work ended in plain words
// (§57 drafting rule 1: written for someone who is not a programmer). No rule
// citation, no FSM vocabulary — and the running-shape ending and the
// parked/card-shape ending read differently, because they ARE different facts.
func TestHistoryCancelCauseIsPlainWords(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	seedPostParityCancel(f, member1, "t-post", "r-post", "") // running → completed
	seedPreParityCancel(f, member1, "t-pre", "r-pre")        // parked → finalized

	a := runCancelled(t, f, opScope())
	if a.RowCount != 2 {
		t.Fatalf("served %d rows, want 2: %v", a.RowCount, a.Rows)
	}
	causes := map[string]bool{}
	for i := 0; i < a.RowCount; i++ {
		c := str(cell(t, a, i, "cause"))
		if strings.TrimSpace(c) == "" {
			t.Fatalf("row %d serves an empty cause — the mechanical ending must be stated", i)
		}
		for _, banned := range []string{"(4.5)", "finalize-with-card", "§", "S02"} {
			if strings.Contains(c, banned) {
				t.Errorf("cause %q carries the citation token %q — plain words, not rule citations (walk residue (b))", c, banned)
			}
		}
		causes[c] = true
	}
	if len(causes) != 2 {
		t.Errorf("the two mechanical endings (running-stop, parked-withdrawal) serve the same cause %v — the record knows the difference and the row must keep it", causes)
	}
}

// TestHistoryServesThePostParityCancelOnceWithTheStructuredActorWinning —
// CONTROL (green today, must stay green): one human act is ONE row, and when
// both records exist the structured transition wins — the ledger leg fills,
// it never overrides (the reads.go leg-order, mirrored; RW-18 "one act, one
// row"). The ledger record deliberately names a DIFFERENT spelling so an
// override would be visible.
func TestHistoryServesThePostParityCancelOnceWithTheStructuredActorWinning(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	f.task("t-both", member1, "Both records", "cancelled")
	f.run("r-both", member1, "t-both", "finalized", "lane-x")
	f.event("r-both", member1, "ledger_update", map[string]any{
		"change": map[string]any{"verb": "ledger.decide", "actor": "stale-spelling", "stage": "verify"},
		"ledger": map[string]any{
			"task_id": "t-both",
			"decisions": []map[string]any{
				{"seq": 1, "ts": "2026-08-17T00:31:34Z", "stage": "verify", "author": "human",
					"text": "requester cancelled at the CHECK-INTEGRITY card", "reason": "cancel is always available (4.5)"},
			},
		},
	})
	f.event("r-both", member1, "run.state_changed", map[string]any{
		"from": "parked", "to": "finalized",
		"reason": "verification cancelled at the card (4.5): finalize-with-card",
		"actor":  member1,
		"detail": map[string]any{"cause": "human cancel (4.5)", "actor": member1, "ladder_invoked": false, "ask_id": "ask-1"},
	})

	a := runCancelled(t, f, opScope())
	if a.RowCount != 1 {
		t.Fatalf("one cancelled task served %d rows, want exactly 1 (one act, one row)", a.RowCount)
	}
	if got := str(cell(t, a, 0, "cancelled_by")); got != member1 {
		t.Errorf("cancelled_by = %q, want %q — the structured leg wins; the ledger leg only fills", got, member1)
	}
}

// TestHistoryCancelRowsStayOwnerScoped — S01.9 on the widened read: the ledger
// leg must not loosen the scope. The operator sees both households' cancels;
// each member sees exactly their own — and sees it ATTRIBUTED, which is the
// red direction today (the row exists, the who is NULL).
func TestHistoryCancelRowsStayOwnerScoped(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	f.user(member2, "member")
	seedPreParityCancel(f, member1, "t-alice-c", "r-alice-c")
	seedPreParityCancel(f, member2, "t-bob-c", "r-bob-c")

	if a := runCancelled(t, f, opScope()); a.RowCount != 2 {
		t.Fatalf("operator sees %d rows, want 2", a.RowCount)
	}
	a := runCancelled(t, f, memberScope(member1))
	if a.RowCount != 1 {
		t.Fatalf("%s sees %d rows, want exactly 1 (their own)", member1, a.RowCount)
	}
	if got := str(cell(t, a, 0, "task_id")); got != "t-alice-c" {
		t.Errorf("%s was served task %q, want their own t-alice-c", member1, got)
	}
	if got := str(cell(t, a, 0, "cancelled_by")); got != member1 {
		t.Errorf("cancelled_by = %q, want %q — the member's own row must be attributed too", got, member1)
	}
	if b := runCancelled(t, f, memberScope(member2)); b.RowCount != 1 || str(cell(t, b, 0, "task_id")) != "t-bob-c" {
		t.Fatalf("%s sees %d rows (first task %v), want exactly their own t-bob-c", member2, b.RowCount, b.Rows)
	}
}

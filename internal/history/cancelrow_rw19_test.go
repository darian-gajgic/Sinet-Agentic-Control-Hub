package history_test

// cancelrow_rw19_test.go — P3-RW-19 executor half, History row (R11 literal
// pins, T14 History limb). Committed RED before the implementation
// (Amendment-A carve-out, CONVENTIONS §3).
//
// internal/history is a LEAF: a view cannot read a Go registry, so the
// discriminators its new ledger leg reads live in the SQL as literals, and this
// is what makes them a contract rather than a promise (the §37
// TestLimitEventTypesArePinnedToTheRegistry precedent). Half of them have an
// exported producer constant; the other half — the two cancel mint prefixes —
// are pinned at their own mints by cancelmint_rw18_test.go in stage and intake,
// and pinned to the same spellings here. A reworded mint then fails at both
// ends instead of silently emptying this leg.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
)

func TestCancelledQueryLiteralsMatchTheirProducers(t *testing.T) {
	q, ok := history.QueryByName("status.tasks_cancelled")
	if !ok {
		t.Fatal("status.tasks_cancelled is missing from the catalog")
	}
	for _, c := range []struct{ what, literal string }{
		{"the ledger event type", ledger.EventLedgerUpdate},
		{"the ledger decide verb", ledger.VerbDecide},
		{"the ledger human author", ledger.AuthorHuman},
		// The two cancel mint prefixes, pinned at their producers by
		// internal/stage and internal/intake's cancelmint_rw18_test.go, and read
		// here because `ledger.decide` also carries retries, accepts and
		// revisions and the entry has no typed kind to read instead.
		{"the card/verb cancel mint prefix", "requester cancelled"},
		{"the SPEC-DOUBT rethink mint prefix", "requester: rethink"},
		// The frozen honest absence: this row has no renderer between it and the
		// reader, so the line the reader sees IS the served value.
		{"the honest-absence literal", "no reason given"},
	} {
		if !strings.Contains(q.SQL, "'"+c.literal+"'") {
			t.Errorf("%s (%q) does not appear in the query — the ledger leg would go quiet:\n%s",
				c.what, c.literal, q.SQL)
		}
	}
}

// rw19SeedCancelShape seeds one run of a cancelled task in one of the three
// record shapes the four mint paths actually leave behind:
//
//	structured — the post-parity verb path: the transition alone;
//	preparity  — the walk's own shape: a platform-attributed transition whose
//	             only "(4.5)" is in its mechanical sentence, plus the ledger
//	             record that is the sole place the who survives — and no motive
//	             survives anywhere, because none was ever captured;
//	both       — a post-parity card cancel, which writes BOTH.
//
// Wherever the structured leg is also present the ledger record deliberately
// names a DIFFERENT actor spelling, so leg A winning is a measured result and
// not a coincidence.
func rw19SeedCancelShape(f *fixture, owner, taskID, runID, shape string) {
	f.run(runID, owner, taskID, "finalized", "lane-"+owner)
	if shape == "preparity" || shape == "both" {
		ledgerActor := owner
		if shape == "both" {
			ledgerActor = "stale-spelling"
		}
		f.event(runID, owner, "ledger_update", map[string]any{
			"change": map[string]any{"verb": "ledger.decide", "actor": ledgerActor, "stage": "verify"},
			"ledger": map[string]any{
				"task_id": taskID,
				"decisions": []map[string]any{
					{"seq": 1, "ts": "2026-08-17T00:31:34Z", "stage": "verify", "author": "human",
						"text": "requester cancelled at the CHECK-INTEGRITY card", "reason": "cancel is always available (4.5)"},
				},
			},
		})
	}
	transition := map[string]any{
		"from": "parked", "to": "finalized",
		"reason": "verification cancelled at the card (4.5): finalize-with-card",
		"actor":  "platform",
	}
	if shape == "structured" || shape == "both" {
		transition["actor"] = owner
		transition["detail"] = map[string]any{
			"cause": "human cancel (4.5)", "actor": owner, "ladder_invoked": false,
			"reason": "taking a different approach",
		}
	}
	f.event(runID, owner, "run.state_changed", transition)
}

// TestHistoryServesOneRowPerCancelledTaskWhateverTheShape — R9 / T14, the
// History limb of "one act, one row": whatever the record shape and however
// many runs one act ended, the History view answers with exactly ONE row per
// cancelled task — the ledger leg is a correlated scalar, never a join that
// widens the answer — and the structured actor wins wherever it exists.
func TestHistoryServesOneRowPerCancelledTaskWhateverTheShape(t *testing.T) {
	for _, shape := range []string{"structured", "preparity", "both"} {
		for _, k := range []int{1, 2, 3, 5} {
			t.Run(fmt.Sprintf("%s/k=%d", shape, k), func(t *testing.T) {
				f := newFixture(t)
				f.user(member1, "member")
				f.task("t-k", member1, "K runs", "cancelled")
				for i := 0; i < k; i++ {
					rw19SeedCancelShape(f, member1, "t-k", fmt.Sprintf("r-k-%02d", i), shape)
				}
				a := runCancelled(t, f, opScope())
				if a.RowCount != 1 {
					t.Fatalf("%d cancelled runs of one task served %d rows, want exactly 1: %v", k, a.RowCount, a.Rows)
				}
				if got := str(cell(t, a, 0, "cancelled_by")); got != member1 {
					t.Errorf("cancelled_by = %q, want %q — the structured leg wins and the ledger leg only fills", got, member1)
				}
				wantReason := "taking a different approach"
				if shape == "preparity" {
					wantReason = "no reason given"
				}
				if got := str(cell(t, a, 0, "reason")); got != wantReason {
					t.Errorf("reason = %q, want %q", got, wantReason)
				}
				if got := str(cell(t, a, 0, "cause")); strings.TrimSpace(got) == "" {
					t.Error("the row serves no cause — the mechanical ending must be stated in plain words")
				}
			})
		}
	}
}

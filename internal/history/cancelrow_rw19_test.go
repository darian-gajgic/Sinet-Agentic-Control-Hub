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
	"unicode"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
)

// rw19CancelMintPrefixes are the two spellings the catalog's ledger leg matches
// on, mirroring internal/api's cancelLedgerPrefixes and pinned at their own
// mints by cancelmint_rw18_test.go in internal/stage and internal/intake.
var rw19CancelMintPrefixes = []string{"requester cancelled", "requester: rethink"}

func TestCancelledQueryLiteralsMatchTheirProducers(t *testing.T) {
	q, ok := history.QueryByName("status.tasks_cancelled")
	if !ok {
		t.Fatal("status.tasks_cancelled is missing from the catalog")
	}
	cases := []struct{ what, literal string }{
		{"the ledger event type", ledger.EventLedgerUpdate},
		{"the ledger decide verb", ledger.VerbDecide},
		{"the ledger human author", ledger.AuthorHuman},
		// The frozen honest absence: this row has no renderer between it and the
		// reader, so the line the reader sees IS the served value.
		{"the honest-absence literal", "no reason given"},
	}
	// The two cancel mint prefixes are read here because `ledger.decide` also
	// carries retries, accepts and revisions and the entry has no typed kind to
	// read instead.
	for _, p := range rw19CancelMintPrefixes {
		cases = append(cases, struct{ what, literal string }{"a cancel mint prefix", p})
	}
	for _, c := range cases {
		if !strings.Contains(q.SQL, "'"+c.literal+"'") {
			t.Errorf("%s (%q) does not appear in the query — the ledger leg would go quiet:\n%s",
				c.what, c.literal, q.SQL)
		}
	}
}

// TestCancelMintPrefixesStayInTheAsciiSubsetBothReadersAgreeOn — P3-RW-19 drain
// r1 F1.
//
// The catalog matches these prefixes with SQLite's `lower(trim(...))`, and
// internal/api's leg B matches them with Go's `strings.ToLower(strings.
// TrimSpace(...))`. Those pairs are NOT the same function: SQLite's `trim`
// strips spaces only and its `lower` folds ASCII only, while Go's strip and
// fold the whole Unicode space. So a ledger text led by a tab, or carrying a
// non-ASCII case pair inside the matched prefix, would attribute on the task
// page and NOT in History — the two surfaces answering one question
// differently, which is the exact defect this packet was cut to end.
//
// The divergence is UNREACHABLE, and this is the invariant that keeps it so:
// every producer builds its text by concatenating one of these prefixes as a
// literal head ("requester cancelled at the "+card, "requester: rethink — …"),
// so the matched region is always these bytes and nothing else. While both
// spellings are pure printable ASCII with no leading whitespace, the two
// implementations agree on them exactly.
//
// This test is what makes that a checked property rather than an observation:
// a future mint reworded to open with a Unicode space, a non-breaking space, or
// a non-ASCII letter trips HERE — loudly, at the pin — instead of silently
// splitting the two surfaces apart again. The SQL is deliberately not
// "corrected" to chase Unicode: the fix for a divergence nothing can reach is
// to keep it unreachable, not to add a second spelling of the qualification.
func TestCancelMintPrefixesStayInTheAsciiSubsetBothReadersAgreeOn(t *testing.T) {
	if len(rw19CancelMintPrefixes) == 0 {
		t.Fatal("no cancel mint prefixes are pinned — the ledger leg matches on nothing")
	}
	for _, p := range rw19CancelMintPrefixes {
		if p == "" {
			t.Error("an empty cancel mint prefix would match every ledger entry")
			continue
		}
		for i, r := range p {
			switch {
			case r > unicode.MaxASCII:
				t.Errorf("prefix %q holds the non-ASCII rune %q at %d: SQLite's lower() folds ASCII only, "+
					"so History and the task page would disagree about this entry", p, r, i)
			case !unicode.IsPrint(r):
				t.Errorf("prefix %q holds the non-printable rune %q at %d — a control character in a matched "+
					"prefix is a spelling no reader can verify", p, r, i)
			case i == 0 && unicode.IsSpace(r):
				t.Errorf("prefix %q begins with whitespace: SQLite's trim() strips spaces only, so a "+
					"tab- or Unicode-space-led text attributes on the task page and not in History", p)
			}
		}
		// Whole-string belt: the head this leg matches must survive both readers'
		// normalizations identically, which is what the two checks above buy.
		if strings.ToLower(p) != strings.ToLower(strings.TrimSpace(p)) {
			t.Errorf("prefix %q changes under TrimSpace — the two readers normalize it differently", p)
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

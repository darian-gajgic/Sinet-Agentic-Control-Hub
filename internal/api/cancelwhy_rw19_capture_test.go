package api_test

// cancelwhy_rw19_capture_test.go — P3-RW-19 executor half, transport side (T8
// transport limb, T12, T14 task-page limb). Committed RED before the
// implementation (Amendment-A carve-out, CONVENTIONS §3).
//
// The split follows actions_test.go's own seam: the ratified cancel MAPPING is
// asserted where it lives (internal/stage, against real FSM rows), and what is
// asserted here is the transport contract — the optional body field is read,
// bounded and handed to the choreography, and the read side keeps serving one
// row per run whatever record shape the cancel left.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
)

// ── T8, transport limb: the optional body reason rides to the choreography ──

// TestCancelVerbsCarryTheBodyReasonToTheChoreography — R5: both S15.2 cancel
// verbs accept an optional `{"reason": "<one line>"}` and hand the person's own
// words to the surface that mints the record. Absent body, empty body, `{}` and
// an empty reason all mean "no reason" and stay ACCEPTED — the landed clients
// post `{}` or nothing, and this change is strictly additive (§48).
func TestCancelVerbsCarryTheBodyReasonToTheChoreography(t *testing.T) {
	e := newDecisionEnv(t)
	seedTask(t, e.b, "t-alice", "alice", "T", "doing")
	seedRun(t, e.b, "r-alice", "alice", "t-alice", "running", "lane")

	e.mustDo(t, "alice", "POST", "/api/runs/r-alice/cancel", `{"reason":"taking a different approach"}`)
	if e.cancel.runReason != "taking a different approach" {
		t.Errorf("the run cancel handed the choreography reason %q, want the person's own words", e.cancel.runReason)
	}
	e.mustDo(t, "alice", "POST", "/api/tasks/t-alice/cancel", `{"reason":"we found a better tool for this"}`)
	if e.cancel.taskReason != "we found a better tool for this" {
		t.Errorf("the task cancel handed the choreography reason %q, want the person's own words", e.cancel.taskReason)
	}

	// The no-reason shapes, all still accepted, all meaning the same thing: an
	// absent body, an empty one, `{}` (what the landed clients post), and an
	// explicitly empty reason.
	for _, body := range []string{"", "{}", `{"reason":""}`} {
		e.cancel.runReason = "sentinel"
		e.mustDo(t, "alice", "POST", "/api/runs/r-alice/cancel", body)
		if e.cancel.runReason != "" {
			t.Errorf("body %q produced reason %q, want the empty no-reason", body, e.cancel.runReason)
		}
		e.cancel.taskReason = "sentinel"
		e.mustDo(t, "alice", "POST", "/api/tasks/t-alice/cancel", body)
		if e.cancel.taskReason != "" {
			t.Errorf("body %q on the task verb produced reason %q, want the empty no-reason", body, e.cancel.taskReason)
		}
	}
}

// ── T12: over cap the reason is REFUSED, never truncated ────────────────────

// TestOverLongCancelReasonIsRefusedAndNothingIsCancelled — R10 / §60: content
// is never silently altered, so a reason past the bound is refused loudly and
// NOTHING is minted. The bound counts characters, not bytes, which is what the
// multi-byte fixture proves; and the refusal is classified by its machine code
// (§48), with a message written for someone who is not a programmer (§57).
func TestOverLongCancelReasonIsRefusedAndNothingIsCancelled(t *testing.T) {
	e := newDecisionEnv(t)
	seedTask(t, e.b, "t-alice", "alice", "T", "doing")
	seedRun(t, e.b, "r-alice", "alice", "t-alice", "running", "lane")

	// 281 multi-byte characters: over the bound by one character, and far over
	// it in bytes — a byte-counting bound would refuse the 280 control below.
	over, _ := json.Marshal(strings.Repeat("é", 281))
	at, _ := json.Marshal(strings.Repeat("é", 280))

	for _, c := range []struct{ what, path string }{
		{"run", "/api/runs/r-alice/cancel"},
		{"task", "/api/tasks/t-alice/cancel"},
	} {
		e.cancel.runActor, e.cancel.taskActor = "", ""
		code, body := e.do(t, "alice", "POST", c.path, `{"reason":`+string(over)+`}`)
		if code != http.StatusBadRequest {
			t.Errorf("%s cancel with an over-long reason: status %d, want 400", c.what, code)
		}
		if !strings.Contains(body, `"reason_too_long"`) {
			t.Errorf("%s cancel refusal = %s, want the machine code reason_too_long (§48)", c.what, body)
		}
		if !strings.Contains(body, "280") {
			t.Errorf("%s cancel refusal does not name the bound, so the person cannot know what to shorten to: %s", c.what, body)
		}
		for _, banned := range []string{"(4.5)", "rune", "§", "S15", "S02"} {
			if strings.Contains(body, banned) {
				t.Errorf("%s cancel refusal carries the token %q — the message is for someone who is not a programmer (§57): %s",
					c.what, banned, body)
			}
		}
		if e.cancel.runActor != "" || e.cancel.taskActor != "" {
			t.Errorf("%s cancel: a refused reason still reached the choreography — nothing may be minted", c.what)
		}
		// The boundary control: exactly at the bound is ACCEPTED, so the refusal
		// above is the bound doing its job and not a blanket rejection.
		e.mustDo(t, "alice", "POST", c.path, `{"reason":`+string(at)+`}`)
	}
	if e.cancel.runReason == "" || e.cancel.taskReason == "" {
		t.Error("a reason exactly at the bound was dropped rather than carried")
	}
}

// ── T14, task-page limb: one act, one row, over shapes and k runs ───────────

// rw19SeedCancelledRun writes one run of a cancelled task in the wanted record
// shape: the structured transition alone (the post-parity verb path), the
// pre-parity pair whose only record of the who is the ledger entry, or BOTH —
// which is what a post-parity card cancel actually leaves behind.
func rw19SeedCancelledRun(t *testing.T, b *backend, owner, taskID, runID, shape, reason string) {
	t.Helper()
	seedRun(t, b, runID, owner, taskID, "finalized", "lane")
	if shape == "preparity" || shape == "both" {
		// Where the structured leg is also present the ledger record deliberately
		// names a DIFFERENT actor spelling, so leg A winning is a measured result
		// and not a coincidence. On the pre-parity shape the ledger entry is the
		// only record of the who, so it names the person.
		ledgerActor := owner
		if shape == "both" {
			ledgerActor = "stale-spelling"
		}
		appendRun(t, b, owner, runID, "ledger_update",
			`{"change":{"verb":"ledger.decide","actor":"`+ledgerActor+`","stage":"verify"},`+
				`"ledger":{"task_id":"`+taskID+`","decisions":[`+
				`{"seq":1,"ts":"2026-08-17T00:31:34Z","stage":"verify","author":"human",`+
				`"text":"requester cancelled at the CHECK-INTEGRITY card","reason":"cancel is always available (4.5)"}]}}`)
	}
	if shape == "structured" || shape == "both" {
		detail := `{"cause":"human cancel (4.5)","actor":"` + owner + `","ladder_invoked":false`
		if reason != "" {
			detail += `,"reason":` + mustJSONString(reason)
		}
		detail += `}`
		appendRun(t, b, owner, runID, "run.state_changed",
			`{"from":"parked","to":"finalized","reason":"verification cancelled at the card (4.5): finalize-with-card",`+
				`"actor":"`+owner+`","detail":`+detail+`}`)
		return
	}
	// The pre-parity transition: platform-attributed, no detail at all. Leg A
	// finds nothing on it, which is what sends the derive to the ledger record.
	appendRun(t, b, owner, runID, "run.state_changed",
		`{"from":"parked","to":"finalized","reason":"verification cancelled at the card (4.5): finalize-with-card",`+
			`"actor":"platform"}`)
}

func mustJSONString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// TestTaskPageServesExactlyOneCancelRowPerRun — R9 / T14, the served half of
// "one act, one row": a task cancel that ended k runs serves k cancel rows, one
// per run, in every record shape — and where both records exist the STRUCTURED
// actor wins, because it names the actor at the transition itself.
func TestTaskPageServesExactlyOneCancelRowPerRun(t *testing.T) {
	for _, shape := range []string{"structured", "preparity", "both"} {
		for _, k := range []int{1, 2, 3, 5} {
			t.Run(fmt.Sprintf("%s/k=%d", shape, k), func(t *testing.T) {
				b := newBackend(t)
				seedUser(t, b, "op", auth.RoleOperator)
				seedUser(t, b, "alice", auth.RoleMember)
				seedTask(t, b, "t-k", "alice", "K runs", "cancelled")
				for i := 0; i < k; i++ {
					rw19SeedCancelledRun(t, b, "alice", "t-k", fmt.Sprintf("r-k-%02d", i), shape,
						"taking a different approach")
				}
				v, body := rw19Decisions(t, b, "alice", "/api/tasks/t-k")
				runs := map[string]int{}
				for _, d := range v.Decisions {
					if d.Decision != "cancel" {
						continue
					}
					runs[d.RunID]++
					if d.Actor != "alice" {
						t.Errorf("cancel row actor = %q, want alice — the structured leg wins and the ledger leg only fills: %s",
							d.Actor, body)
					}
					if shape != "preparity" && d.HumanReason != "taking a different approach" {
						t.Errorf("cancel row human_reason = %q, want the person's own words", d.HumanReason)
					}
					if shape == "preparity" && d.HumanReason != "" {
						t.Errorf("a pre-parity ledger record served human_reason %q — those records hold no motive, "+
							"and scraping one out of ledger prose would be inventing structure", d.HumanReason)
					}
				}
				if len(runs) != k {
					t.Fatalf("served cancel rows for %d runs, want %d: %s", len(runs), k, body)
				}
				for id, n := range runs {
					if n != 1 {
						t.Errorf("run %s served %d cancel rows, want exactly 1 (one act, one row)", id, n)
					}
				}
			})
		}
	}
}

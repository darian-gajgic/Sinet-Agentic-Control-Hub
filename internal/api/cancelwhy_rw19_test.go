package api_test

// cancelwhy_rw19_test.go — P3-RW-19 acceptance tests (task-page half),
// committed RED with the brief (Amendment-A carve-out, CONVENTIONS §3; the red
// window closes at the packet's implementation commit).
//
// Walk residue (b), serve side: the HUMAN DECISIONS row can name who cancelled
// and which rule fired, but it cannot carry the person's MOTIVE — no field
// exists for it. The brief pins the additive wire contract: `TaskDecision`
// gains `human_reason` (JSON `human_reason`, omitempty), read from the ONE
// constructor's additive `$.detail.reason` on the structured cancel transition
// (leg A of reads.go cancelDecisions). Absence is structural — the key is
// ABSENT when no reason was given (the frontend renders the honest line);
// the mechanical `reason` field keeps its landed semantics untouched, so
// canceldecision_test.go stays green unmodified.
//
// Subtests marked CONTROL are expected green today; they pin the absence
// posture and the store-raw half the fix must not break.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type rw19DecisionsView struct {
	Decisions []struct {
		Type        string `json:"type"`
		RunID       string `json:"run_id"`
		Actor       string `json:"actor"`
		Decision    string `json:"decision"`
		Reason      string `json:"reason"`
		HumanReason string `json:"human_reason"`
	} `json:"decisions"`
}

func rw19Decisions(t *testing.T, b *backend, who, path string) (rw19DecisionsView, string) {
	t.Helper()
	code, body := get(t, b, who, path)
	if code != http.StatusOK {
		t.Fatalf("GET %s as %s: %d (%s)", path, who, code, body)
	}
	var v rw19DecisionsView
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v, body
}

// TestTaskDecisionsServeTheHumanReasonFromTheDetail — a reason given at a
// cancel affordance rides the structured detail (the ONE constructor,
// run.CancelDetail) and the HUMAN DECISIONS row serves it in `human_reason`
// (S15.5 task detail / S2.4; S02.2 derive-from-durable-records — reads.go leg
// A lifts it by key, no second mint).
func TestTaskDecisionsServeTheHumanReasonFromTheDetail(t *testing.T) {
	b := twoOwners(t)
	appendRun(t, b, "alice", "r-alice", "run.state_changed",
		`{"from":"running","to":"completed","reason":"cancelled by alice (4.5)","actor":"alice",`+
			`"detail":{"cause":"human cancel (4.5)","actor":"alice","ladder_invoked":true,`+
			`"reason":"taking a different approach"}}`)

	v, _ := rw19Decisions(t, b, "alice", "/api/tasks/t-alice")
	var hits int
	for _, d := range v.Decisions {
		if d.Decision != "cancel" {
			continue
		}
		hits++
		if d.HumanReason != "taking a different approach" {
			t.Errorf("human_reason = %q, want the person's own words — the motive must reach the row (walk residue (b))", d.HumanReason)
		}
		if d.Reason == "" {
			t.Error("the mechanical reason vanished — human_reason is ADDITIVE, the record's own sentence stays served")
		}
	}
	if hits != 1 {
		t.Fatalf("served %d cancel rows, want exactly 1: %+v", hits, v.Decisions)
	}
}

// TestTaskDecisionsHumanReasonIsAbsentNotEmptyWhenNoneWasGiven — CONTROL
// (green today, pins the omitempty posture): a cancel with no reason serves NO
// human_reason key at all. Absence is structural on this wire — the honest
// "no reason given" line is the renderer's to draw from the absent key, and a
// present-but-blank key would claim a reason that never existed (the ask_id
// omission precedent, cancelliterals_rw18_test.go).
func TestTaskDecisionsHumanReasonIsAbsentNotEmptyWhenNoneWasGiven(t *testing.T) {
	b := twoOwners(t)
	appendRun(t, b, "alice", "r-alice", "run.state_changed",
		`{"from":"running","to":"completed","reason":"cancelled by alice (4.5)","actor":"alice",`+
			`"detail":{"cause":"human cancel (4.5)","actor":"alice","ladder_invoked":false}}`)

	_, body := rw19Decisions(t, b, "alice", "/api/tasks/t-alice")
	if strings.Contains(body, `"human_reason"`) {
		t.Errorf("a reason-less cancel serves a human_reason key — absence must be absent, not blank: %s", body)
	}
}

// TestServedHumanReasonIsRedactedAtTheEdge — §38 D10 / codor C2: human_reason
// is run_events payload content on an unwrapped response, so it passes the
// redaction primitive at the serving edge like every other lifted decision
// string (redactTaskDecisions). Store-raw / serve-redacted: the stored row
// keeps the secret verbatim; the wire never carries it.
func TestServedHumanReasonIsRedactedAtTheEdge(t *testing.T) {
	b := twoOwners(t)
	secret := "sk-ant-api03-" + strings.Repeat("A", 40)
	appendRun(t, b, "alice", "r-alice", "run.state_changed",
		`{"from":"running","to":"completed","reason":"cancelled by alice (4.5)","actor":"alice",`+
			`"detail":{"cause":"human cancel (4.5)","actor":"alice","ladder_invoked":false,`+
			`"reason":"pasted `+secret+` by mistake"}}`)

	v, body := rw19Decisions(t, b, "alice", "/api/tasks/t-alice")
	if strings.Contains(body, secret) {
		t.Fatal("the human reason reached the wire unredacted — redactTaskDecisions must cover the new field (§38 D10)")
	}
	var served string
	for _, d := range v.Decisions {
		if d.Decision == "cancel" {
			served = d.HumanReason
		}
	}
	// The non-tautology direction, RED today: the reason must actually be
	// SERVED (redacted), not merely withheld — an absent field also contains
	// no secret, and that is not redaction.
	if !strings.Contains(served, "[REDACTED") {
		t.Errorf("human_reason = %q, want the redacted marker — the reason is served THROUGH the primitive, not dropped", served)
	}
	// CONTROL: the stored row stays raw (store-raw / serve-redacted, R19).
	var stored string
	if err := b.db.QueryRowContext(t.Context(),
		`SELECT payload FROM run_events WHERE run_id = 'r-alice' AND type = 'run.state_changed'`).Scan(&stored); err != nil {
		t.Fatalf("read stored payload: %v", err)
	}
	if !strings.Contains(stored, secret) {
		t.Error("the STORED payload lost the raw content — redaction is a serving-edge act, never a write")
	}
}

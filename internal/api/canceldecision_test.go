package api_test

// P3-RW-18 D1 acceptance tests — committed RED with the brief (Amendment-A;
// red window closed by the packet's implementation commit).
//
// Walk-proven gap: the task detail's HUMAN DECISIONS list (S15.5 "every human
// decision along the way", S2.4) never receives a cancel. The cancel's who/why
// IS durably recorded — cancel.go transitions carry actor + structured detail
// {"cause":"human cancel (4.5)","actor":...}, and the card-answer paths ledger
// it as a `ledger_update` ledger.decide entry with author "human" (the live
// walk row: event 788 of t-fe5ff6c325967c3e.verify) — but taskDecisions
// (internal/api/reads.go) reads neither source, and reads.go documents the
// miss as deliberate. RW-18 reverses that posture: the cancel is SERVED, by
// reconstruction from the records that already exist (S02.2), never by
// double-minting a decision row (approvals.go OQ8 stands).

import (
	"encoding/json"
	"net/http"
	"testing"
)

type decisionsView struct {
	Decisions []struct {
		Seq      int64  `json:"seq"`
		Type     string `json:"type"`
		RunID    string `json:"run_id"`
		Actor    string `json:"actor"`
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	} `json:"decisions"`
}

func cancelDecisions(t *testing.T, b *backend, who, path string) decisionsView {
	t.Helper()
	code, body := get(t, b, who, path)
	if code != http.StatusOK {
		t.Fatalf("GET %s as %s: %d (%s)", path, who, code, body)
	}
	var v decisionsView
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

// TestTaskDecisionsServeTheVerbPathCancel — leg A: a cancel through the S15.6
// verbs / ladder card (internal/stage/cancel.go) records the human actor and
// the structured cancelDetail on the run.state_changed transition. The
// decisions derive must serve it: who cancelled, that it was a cancel, and the
// recorded why (15.6 attribution; S2.4 "actor … decision").
func TestTaskDecisionsServeTheVerbPathCancel(t *testing.T) {
	b := twoOwners(t)
	appendRun(t, b, "alice", "r-alice", "run.state_changed",
		`{"from":"running","to":"completed","reason":"cancelled by alice (4.5)","actor":"alice",`+
			`"detail":{"cause":"human cancel (4.5)","actor":"alice","ladder_invoked":false}}`)

	v := cancelDecisions(t, b, "alice", "/api/tasks/t-alice")
	var hits int
	for _, d := range v.Decisions {
		if d.Decision != "cancel" {
			continue
		}
		hits++
		if d.Actor != "alice" {
			t.Errorf("cancel decision actor = %q, want the human who pressed cancel (15.6)", d.Actor)
		}
		if d.Reason == "" {
			t.Error("cancel decision carries no why — the recorded reason must ride along (S2.4)")
		}
		if d.RunID != "r-alice" {
			t.Errorf("cancel decision run = %q, want r-alice", d.RunID)
		}
	}
	if hits != 1 {
		t.Fatalf("decisions served %d cancel rows, want exactly 1 — the walk-proven gap (RW-18 D1): %+v", hits, v.Decisions)
	}
}

// TestTaskDecisionsServeTheCardCancelFromTheLedgerRecord — leg B: the shape the
// live walk world holds TODAY (events 788/789 of t-fe5ff6c325967c3e.verify):
// the card-answer transition says actor "platform" and carries no detail; the
// who/why lives only in the ledger_update ledger.decide entry authored human.
// The derive must reconstruct the cancel from that record (S02.2) — and must
// serve it ONCE, not once per source.
func TestTaskDecisionsServeTheCardCancelFromTheLedgerRecord(t *testing.T) {
	b := twoOwners(t)
	appendRun(t, b, "bob", "r-bob", "ledger_update",
		`{"change":{"verb":"ledger.decide","actor":"bob","stage":"verify"},"content_hash":"x",`+
			`"ledger":{"task_id":"t-bob","decisions":[`+
			`{"seq":1,"ts":"2026-08-17T00:23:37Z","stage":"triage","author":"platform","text":"triage: family=software","reason":"Stage-0 (S06.2)"},`+
			`{"seq":2,"ts":"2026-08-17T00:31:34Z","stage":"verify","author":"human","text":"requester cancelled at the CHECK-INTEGRITY card",`+
			`"reason":"cancel is always available (4.5); parked→finalized, finalize-with-card (S02.3; CONVENTIONS §14 reading 9)"}]}}`)
	appendRun(t, b, "bob", "r-bob", "run.state_changed",
		`{"from":"parked","to":"finalized","reason":"verification cancelled at the card (4.5): finalize-with-card","actor":"platform"}`)

	v := cancelDecisions(t, b, "bob", "/api/tasks/t-bob")
	var hits int
	for _, d := range v.Decisions {
		if d.Decision != "cancel" {
			continue
		}
		hits++
		if d.Actor != "bob" {
			t.Errorf("card-cancel actor = %q, want bob — the ledger.decide change.actor is the who (event-788 shape)", d.Actor)
		}
		if d.Reason == "" {
			t.Error("card-cancel decision carries no why")
		}
	}
	if hits != 1 {
		t.Fatalf("decisions served %d cancel rows for the ledger-recorded card cancel, want exactly 1: %+v", hits, v.Decisions)
	}
}

// TestTaskDecisionsServeOneCancelWhenBothRecordsExist — after mint parity a
// card-answer cancel writes BOTH records for the same run (the ledger.decide
// entry AND a structured-detail transition). One human act is ONE decision row:
// the structured transition wins; the ledger leg serves only runs with no
// structured cancel (the D3 lesson applied to D1's own fix).
func TestTaskDecisionsServeOneCancelWhenBothRecordsExist(t *testing.T) {
	b := twoOwners(t)
	appendRun(t, b, "alice", "r-alice", "ledger_update",
		`{"change":{"verb":"ledger.decide","actor":"alice","stage":"verify"},"content_hash":"x",`+
			`"ledger":{"task_id":"t-alice","decisions":[`+
			`{"seq":1,"ts":"2026-08-17T00:31:34Z","stage":"verify","author":"human","text":"requester cancelled at the CHECK-INTEGRITY card",`+
			`"reason":"cancel is always available (4.5)"}]}}`)
	appendRun(t, b, "alice", "r-alice", "run.state_changed",
		`{"from":"parked","to":"finalized","reason":"verification cancelled at the card (4.5): finalize-with-card","actor":"alice",`+
			`"detail":{"cause":"human cancel (4.5)","actor":"alice","ladder_invoked":false}}`)

	v := cancelDecisions(t, b, "alice", "/api/tasks/t-alice")
	var hits int
	for _, d := range v.Decisions {
		if d.Decision == "cancel" {
			hits++
		}
	}
	if hits != 1 {
		t.Fatalf("one human cancel served as %d decision rows, want exactly 1 (never double-served)", hits)
	}
}

// TestPlainLifecycleTransitionsAreNotDecisions — the reads.go posture that was
// RIGHT stays right: an ordinary platform transition (scheduler admit, stage
// park) is run lifecycle, never a human decision. Only the structured cancel
// discriminator ('detail.cause' = "human cancel (4.5)") and the human-authored
// ledger.decide cancel entry qualify.
func TestPlainLifecycleTransitionsAreNotDecisions(t *testing.T) {
	b := twoOwners(t)
	appendRun(t, b, "bob", "r-bob", "run.state_changed",
		`{"from":"new","to":"queued","reason":"admitted to the claim queue (S10.7)","actor":"platform"}`)
	appendRun(t, b, "bob", "r-bob", "run.state_changed",
		`{"from":"queued","to":"claimed","reason":"CAS-claimed by the scheduler (S10.7)","actor":"platform"}`)

	v := cancelDecisions(t, b, "bob", "/api/tasks/t-bob")
	for _, d := range v.Decisions {
		if d.Type == "run.state_changed" {
			t.Fatalf("a plain lifecycle transition surfaced as a decision: %+v", d)
		}
	}
}

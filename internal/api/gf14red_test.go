package api_test

// gf14red_test.go — P3-GF14 acceptance test T7, committed RED with the
// grounding brief (P3/briefs/P3-GF14.md §7; Amendment-A carve-out, CONVENTIONS
// §3). Window closes at the P3-GF14 implementation commit.
//
// The defect (GF9 review M10): the wire serves ONE receipt-absence line for
// every receipt-less run — "no receipt yet — one is written when the work
// finishes" — which is a lie on a run that already ENDED (a crashed leg never
// materializes a receipt: CONVENTIONS §55, `crashed` settles queue-only and
// the successor carries its own receipt, D4). The FE papered over the lie with
// a hardcoded demo-seed guess that misattributes on a fresh world
// (web/src/TaskDetail.tsx:1259 — the FRONTEND.md lane retires it once this
// wire tells the truth). Spec: S10.1 (receipts materialize at the terminal
// transition), S15.12 (honesty), §38 (absences are rendered with their reason).

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
)

// TestGF14ReceiptAbsenceSpeaksToTheRunsOwnEnding is R6: the absence reason is
// derived from the run's own state and lineage — a terminal receipt-less run
// never gets a promised-receipt line, a crashed leg with a successor names the
// run that carries the work onward, and the demo-seed clause never appears on
// the wire.
func TestGF14ReceiptAbsenceSpeaksToTheRunsOwnEnding(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "op", auth.RoleOperator)
	seedTask(t, b, "t-gf14", "op", "GF14 receipt honesty", "doing")

	// The witnessed lineage shape (t-3df81201c0ab0cbe.intake → .g4): a crashed
	// leg superseded by its recovery fork, plus a still-running leg.
	exec(t, b, `INSERT INTO runs (run_id, user_id, task_id, state, lane, generation, created_ts, updated_ts)
	            VALUES (?,?,?,?,?,1,?,?)`,
		"r-gf14-crash", "op", "t-gf14", "crashed", "lane-gf14", nowTS(), nowTS())
	exec(t, b, `INSERT INTO runs (run_id, user_id, task_id, state, lane, generation, parent_run_id, created_ts, updated_ts)
	            VALUES (?,?,?,?,?,2,?,?,?)`,
		"r-gf14-crash.g2", "op", "t-gf14", "completed", "lane-gf14", "r-gf14-crash", nowTS(), nowTS())
	exec(t, b, `INSERT INTO receipts (run_id, user_id, usage_json, materialized_ts) VALUES (?,?,?,?)`,
		"r-gf14-crash.g2", "op", `{"line_items":[]}`, nowTS())
	exec(t, b, `INSERT INTO runs (run_id, user_id, task_id, state, lane, generation, created_ts, updated_ts)
	            VALUES (?,?,?,?,?,1,?,?)`,
		"r-gf14-live", "op", "t-gf14", "running", "lane-gf14", nowTS(), nowTS())

	body := getOK(t, b, "op", "/api/tasks/t-gf14")
	var detail struct {
		Runs []struct {
			RunID         string          `json:"run_id"`
			State         string          `json:"state"`
			Receipt       json.RawMessage `json:"receipt"`
			ReceiptAbsent string          `json:"receipt_absent"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatalf("task detail decode: %v: %.300s", err, body)
	}
	byID := map[string]string{}
	for _, r := range detail.Runs {
		byID[r.RunID] = r.ReceiptAbsent
	}
	if len(detail.Runs) != 3 {
		t.Fatalf("seeded 3 runs, detail served %d: %.400s", len(detail.Runs), body)
	}

	crash, ok := byID["r-gf14-crash"]
	if !ok || crash == "" {
		t.Fatal("the crashed leg must carry a receipt-absence reason")
	}
	if strings.Contains(crash, "is written when the work finishes") {
		t.Errorf("R6: a run that already ENDED is promised a receipt that will never come "+
			"(§55: crashed materializes none): %q", crash)
	}
	if !strings.Contains(crash, "r-gf14-crash.g2") {
		t.Errorf("R6: the crashed leg's reason must name the successor that carries the work "+
			"onward (runs.parent_run_id lineage, §55/D4): %q", crash)
	}
	if strings.Contains(strings.ToLower(crash), "demo") {
		t.Errorf("the demo-seed guess must never ride the wire (GF9 review M10): %q", crash)
	}

	if live := byID["r-gf14-live"]; live == "" || !strings.Contains(live, "finishes") {
		// On a LIVE run the future-tense line is true and stays.
		t.Errorf("the running leg keeps its honest future-tense absence line, got %q", live)
	}
}

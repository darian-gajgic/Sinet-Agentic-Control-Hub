package history_test

import (
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
)

// TestRoutingQualityJoinsCausesToOutcomes — acceptance 26. S14.10 ¶5: "a
// periodic view joins routing.decided CAUSES to OUTCOMES (rework rates,
// verdicts, receipts) so routing itself is auditable over time."
//
// The test drives two runs routed for different causes with different outcomes,
// and asserts the join carries all three outcome families through: verdicts,
// rework and the receipt.
func TestRoutingQualityJoinsCausesToOutcomes(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	f.task("t1", member1, "Task", "doing")

	// A clean run: selector-matched, shipped first time, priced receipt.
	f.run("r-clean", member1, "t1", "completed", "anthropic")
	f.event("r-clean", member1, "routing.decided", map[string]any{
		"cause": "selector-match", "score": 0.92, "worker": "wt-1", "worker_name": "the-worker",
		"version": "v3", "model": "claude-sonnet-5", "lane": "anthropic", "effort": "standard",
		"plain_reason": "the selector matched on domain and artifact shape",
	})
	f.event("r-clean", member1, "verdict.recorded", map[string]any{
		"round": 1, "verdict": "SHIP", "domain": "software", "ac_ids": []string{"AC-1"},
		"golden_set": map[string]any{"measured": false},
	})
	f.receipt("r-clean", member1, 1.5, 3, 0)

	// A reworked run: no fit, generalist, two rounds, second adverse.
	f.run("r-rework", member1, "t1", "completed", "anthropic")
	f.event("r-rework", member1, "routing.decided", map[string]any{
		"cause": "no-fit-generalist", "generalist": true, "degraded": true,
		"model": "claude-sonnet-5", "lane": "anthropic",
		"plain_reason": "no worker matched; generalist on the execution seat",
	})
	f.event("r-rework", member1, "verdict.recorded", map[string]any{
		"round": 1, "verdict": "REVISE", "domain": "software", "ac_ids": []string{"AC-1"},
		"golden_set": map[string]any{"measured": false},
	})
	f.event("r-rework", member1, "verdict.recorded", map[string]any{
		"round": 2, "verdict": "SHIP-with-notes", "domain": "software", "ac_ids": []string{"AC-1"},
		"golden_set": map[string]any{"measured": false},
	})
	f.receipt("r-rework", member1, 0, 5, 5)

	// Worker template so the domain leg of the join has something to reach.
	f.exec(`INSERT INTO worker_templates (template_id, user_id, name, scope, kind, domain, status, created_ts, updated_ts)
	        VALUES ('wt-1', ?, 'the-worker', 'personal', 'agentic', 'software', 'active', ?, ?)`,
		member1, f.ts(0), f.ts(0))

	a, err := f.st.SelectView(f.ctx, history.ViewRoutingQuality, opScope(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Rows) != 2 {
		t.Fatalf("routing_quality returned %d rows, want 2", len(a.Rows))
	}
	byRun := map[string]int{}
	for i := range a.Rows {
		byRun[str(cell(t, a, i, "run_id"))] = i
	}

	clean := byRun["r-clean"]
	if got := str(cell(t, a, clean, "cause")); got != "selector-match" {
		t.Errorf("clean run cause = %q", got)
	}
	if got := str(cell(t, a, clean, "domain")); got != "software" {
		t.Errorf("clean run domain = %q — the worker→domain leg of the join did not resolve", got)
	}
	if got := str(cell(t, a, clean, "worker_version")); got != "v3" {
		t.Errorf("clean run worker_version = %q — the S08.4 version→outcome key is the join", got)
	}
	if got := str(cell(t, a, clean, "verdicts")); got != "1" {
		t.Errorf("clean run verdicts = %q, want 1", got)
	}
	if got := str(cell(t, a, clean, "verdicts_ship")); got != "1" {
		t.Errorf("clean run verdicts_ship = %q, want 1", got)
	}
	if got := str(cell(t, a, clean, "reworked")); got != "0" {
		t.Errorf("clean run reworked = %q, want 0", got)
	}
	if got := str(cell(t, a, clean, "pricing_status")); got != "PRICED" {
		t.Errorf("clean run pricing_status = %q, want PRICED — the receipt leg of the join", got)
	}

	rw := byRun["r-rework"]
	if got := str(cell(t, a, rw, "cause")); got != "no-fit-generalist" {
		t.Errorf("reworked run cause = %q", got)
	}
	if got := str(cell(t, a, rw, "verify_rounds")); got != "2" {
		t.Errorf("reworked run verify_rounds = %q, want 2", got)
	}
	if got := str(cell(t, a, rw, "reworked")); got != "1" {
		t.Errorf("reworked run reworked = %q, want 1 — rounds beyond the first ARE the rework", got)
	}
	if got := str(cell(t, a, rw, "verdicts_adverse")); got != "1" {
		t.Errorf("reworked run verdicts_adverse = %q, want 1", got)
	}
	if got := str(cell(t, a, rw, "degraded")); got != "1" {
		t.Errorf("reworked run degraded = %q, want 1", got)
	}
	// The unpriced receipt still reads UNPRICED here, not $0 — the pricing
	// honesty is stated in one place (cost_per_run) and carried through.
	if got := str(cell(t, a, rw, "pricing_status")); got != "UNPRICED" {
		t.Errorf("reworked run pricing_status = %q, want UNPRICED", got)
	}
	if got := cell(t, a, rw, "priced_usd"); got != nil {
		t.Errorf("reworked run priced_usd = %v, want NULL", got)
	}

	// The aggregate the S2.6 question actually asks: rework rate per cause.
	agg, err := f.st.RunQuery(f.ctx, "routing.rework_by_cause", nil, opScope(), 50)
	if err != nil {
		t.Fatal(err)
	}
	rates := map[string]string{}
	for i := range agg.Rows {
		rates[str(cell(t, agg, i, "cause"))] = str(cell(t, agg, i, "reworked_runs"))
	}
	if rates["selector-match"] != "0" || rates["no-fit-generalist"] != "1" {
		t.Errorf("rework_by_cause = %v, want selector-match 0 and no-fit-generalist 1", rates)
	}
}

// TestRoutingQualityCarriesLegacyVerdictAlias — a history that silently began
// at a rename is not a history (§29 R14). The view must count a pre-rename
// verdict row too.
func TestRoutingQualityCarriesLegacyVerdictAlias(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	f.task("t1", member1, "Task", "doing")
	f.run("r1", member1, "t1", "completed", "anthropic")
	f.event("r1", member1, "routing.decided", map[string]any{
		"cause": "pinned", "model": "m", "lane": "anthropic", "plain_reason": "operator pinned",
	})
	// The registered legacy alias of verdict.recorded.
	f.event("r1", member1, "verify.round", map[string]any{
		"round": 3, "verdict": "REVISE", "domain": "software", "ac_ids": []string{"AC-1"},
		"golden_set": map[string]any{"measured": false},
	})

	a, err := f.st.SelectView(f.ctx, history.ViewRoutingQuality, opScope(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Rows) != 1 {
		t.Fatalf("routing_quality returned %d rows, want 1", len(a.Rows))
	}
	if got := str(cell(t, a, 0, "verdicts")); got != "1" {
		t.Errorf("verdicts = %q — a legacy-alias verdict row was not counted (§29 R14)", got)
	}
	if got := str(cell(t, a, 0, "verify_rounds")); got != "3" {
		t.Errorf("verify_rounds = %q, want 3", got)
	}
}

// TestLimitEventHistoryCarriesTheRegisteredAlias — the same rule on the
// limit-event view, and the pin that keeps the literal type list honest.
func TestLimitEventHistoryCarriesTheRegisteredAlias(t *testing.T) {
	f := newFixture(t)
	f.user(member1, "member")
	f.event("", member1, "limit.event", map[string]any{
		"class": "rate", "provider_signal": "429", "resets_at": f.ts(0), "lane": "anthropic",
	})
	f.event("", member1, "engine.rate_limit", map[string]any{
		"class": "quota", "provider_signal": "overloaded", "resets_at": f.ts(0), "lane": "anthropic",
	})

	a, err := f.st.SelectView(f.ctx, history.ViewLimitEventHist, opScope(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Rows) != 2 {
		t.Fatalf("limit_event_history returned %d rows, want 2 (the canonical type AND its registered legacy alias)", len(a.Rows))
	}
	classes := map[string]bool{}
	for i := range a.Rows {
		classes[str(cell(t, a, i, "limit_class"))] = true
	}
	if !classes["rate"] || !classes["quota"] {
		t.Errorf("limit classes = %v, want both rate and quota", classes)
	}
}

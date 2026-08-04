package worker_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// routing_test.go — the S08.8 selection battery: deterministic selector
// match → tie-break seam → model/effort vs duty maps, subscription-coverage
// bound (metered list EMPTY), no-fit two-stage bookkeeping + gap records,
// the FTS index maintenance, and the class-tightening helper. Fake
// everything — zero engine calls (packet rule).

func newRouter(f *fix) *worker.Router {
	return &worker.Router{
		Store:    f.store,
		DutyMap:  worker.DefaultDutyMap(),
		Coverage: worker.Coverage{FlatRateLanes: []string{"anthropic"}},
	}
}

// approveReviewer drives the harness template to ACTIVE (battery green +
// station-4 approval) and returns it.
func approveReviewer(t *testing.T, f *fix, owner string) (worker.Template, worker.Version) {
	t.Helper()
	tpl, v, _ := draftValidated(t, f, owner)
	if _, err := f.store.Approve(context.Background(), owner, v.ID, worker.ApproveOpts{}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	return tpl, v
}

func TestRouteSelectsActiveWorkerDeterministically(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	tpl, v := approveReviewer(t, f, "alice")
	r := newRouter(f)

	q := worker.RouteQuery{
		Requester: "alice", TaskID: "t-1",
		TaskText: "Please review my code for defects",
		Family:   "read-analyze", Domain: "software",
		Classes: []string{"C1"}, Tools: []string{"Read"},
	}
	d1, err := r.Route(context.Background(), q)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if d1.Cause != "selector-match" || d1.TemplateID != tpl.ID || d1.VersionID != v.ID {
		t.Fatalf("decision = %+v, want selector-match of %s@%s", d1, tpl.ID, v.ID)
	}
	if d1.Generalist {
		t.Fatalf("matched decision marked generalist")
	}
	// Step 3: the duty map resolves the execution seat under flat-rate
	// coverage; the seat row carries the window record (the retired B2-4
	// constant's home).
	if d1.Model != "claude-sonnet-5" || d1.Lane != "anthropic" || d1.WindowTokens != worker.DefaultWindowTokens {
		t.Fatalf("seat = %s/%s/%d", d1.Model, d1.Lane, d1.WindowTokens)
	}
	if d1.Effort != "standard" {
		t.Fatalf("effort floor = %q, want the template's", d1.Effort)
	}
	if strings.TrimSpace(d1.PlainReason) == "" || len(d1.Candidates) == 0 {
		t.Fatalf("plain reason / candidates missing: %+v", d1)
	}

	// Deterministic: the same query yields the identical decision.
	d2, err := r.Route(context.Background(), q)
	if err != nil {
		t.Fatalf("Route(again): %v", err)
	}
	if d1.TemplateID != d2.TemplateID || d1.VersionID != d2.VersionID ||
		d1.Model != d2.Model || d1.Score != d2.Score || d1.PlainReason != d2.PlainReason {
		t.Fatalf("selection not deterministic:\n%+v\n%+v", d1, d2)
	}
}

func TestRouteFiltersGrantsAndConfinement(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	approveReviewer(t, f, "alice")
	r := newRouter(f)

	// Required grants ⊄ granted: the reviewer holds Read/Grep/Glob only.
	d, err := r.Route(context.Background(), worker.RouteQuery{
		Requester: "alice", TaskID: "t-2", TaskText: "review my code",
		Family: "read-analyze", Domain: "software",
		Classes: []string{"C1"}, Tools: []string{"Write"},
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if !d.Generalist || d.Cause != "no-fit-generalist" {
		t.Fatalf("grants filter failed: %+v", d)
	}

	// Confinement: the worker's C1 is LOOSER than a C0-declared plan —
	// equal or tighter only (S08.8 step 1).
	d, err = r.Route(context.Background(), worker.RouteQuery{
		Requester: "alice", TaskID: "t-3", TaskText: "review my code",
		Family: "read-analyze", Domain: "software",
		Classes: []string{"C0"}, Tools: []string{"Read"},
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if !d.Generalist {
		t.Fatalf("confinement filter failed: %+v", d)
	}
}

// pickSecond is a fake S12 tie-break duty.
type pickSecond struct{}

func (pickSecond) Break(_ context.Context, _ worker.RouteQuery, cands []worker.Candidate) (int, string, error) {
	if len(cands) < 2 {
		return 0, "only one", nil
	}
	return 1, "duty alias preferred the second candidate", nil
}

func TestRouteTieBreakSeamAndDegradedOrder(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	approveReviewer(t, f, "alice")

	// A second matching worker under a different name (same selectors).
	src2 := strings.Replace(agenticSrc, "name: code-reviewer", "name: alt-reviewer", 1)
	tpl2, v2, err := f.store.CreateDraft(context.Background(), "alice", src2, readGrants(), humanProv())
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	dry := &fakeDry{}
	if _, err := f.store.RunBattery(context.Background(), v2.ID, worker.BatteryInput{
		Actor: "alice", SampleTask: "review the sample diff", Engine: dry,
		Model: "claude-haiku-4-5", EnginePin: "claude-cli@2.1.216",
	}); err != nil {
		t.Fatalf("RunBattery: %v", err)
	}
	if _, err := f.store.Approve(context.Background(), "alice", v2.ID, worker.ApproveOpts{}); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	q := worker.RouteQuery{
		Requester: "alice", TaskID: "t-4", TaskText: "review my code",
		Family: "read-analyze", Domain: "software",
		Classes: []string{"C1"}, Tools: []string{"Read"},
	}

	// Absent duty (nil TieBreak): deterministic degraded order — the
	// name-ordered first of the equal-scored pair — with the absence
	// recorded in the reason (degrade, never fake).
	r := newRouter(f)
	d, err := r.Route(context.Background(), q)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(d.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(d.Candidates))
	}
	if d.TemplateName != "alt-reviewer" {
		t.Fatalf("degraded order picked %q, want the name-ordered first", d.TemplateName)
	}
	if !strings.Contains(d.PlainReason, "tie-break duty absent") {
		t.Fatalf("degraded tie-break not recorded: %q", d.PlainReason)
	}

	// Wired duty: the S12 seam picks, its one-line reason recorded.
	r.TieBreak = pickSecond{}
	d, err = r.Route(context.Background(), q)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if d.TemplateID != mustPick(t, d.Candidates, 1).TemplateID {
		t.Fatalf("tie-break seam ignored: %+v", d)
	}
	if !strings.Contains(d.PlainReason, "duty alias preferred the second candidate") {
		t.Fatalf("tie-break reason missing: %q", d.PlainReason)
	}
	_ = tpl2
}

func mustPick(t *testing.T, c []worker.Candidate, i int) worker.Candidate {
	t.Helper()
	if i >= len(c) {
		t.Fatalf("no candidate %d", i)
	}
	return c[i]
}

func TestRouteNoFitWritesGapAndEarnsCompose(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	r := newRouter(f)

	q := worker.RouteQuery{
		Requester: "alice", TaskID: "t-5", TaskText: "sort my photo archive",
		Family: "chore", Domain: "chore-domain",
		Classes: []string{"C1"},
	}
	d, err := r.Route(context.Background(), q)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if !d.Generalist || d.GapSignature == "" {
		t.Fatalf("no-fit shape: %+v", d)
	}
	if d.ComposeEarned {
		t.Fatalf("compose earned on first occurrence (⚙ default is 2)")
	}
	if !d.Degraded {
		t.Fatalf("unknown domain must be degraded-marked (2.1 honesty)")
	}
	if !strings.Contains(d.PlainReason, "generalist-with-injected-knowledge") {
		t.Fatalf("reason = %q", d.PlainReason)
	}

	// Second occurrence of the family: compose-when-earned trips (⚙
	// workers.gap_proposal_count = 2) and the gap disposition moves to
	// proposed.
	q.TaskID = "t-6"
	d, err = r.Route(context.Background(), q)
	if err != nil {
		t.Fatalf("Route(2): %v", err)
	}
	if !d.ComposeEarned {
		t.Fatalf("compose not earned at the ⚙ threshold: %+v", d)
	}
	if !strings.Contains(d.PlainReason, "EARNED") {
		t.Fatalf("earned reason missing: %q", d.PlainReason)
	}
}

func TestRouteSubscriptionCoverageBinds(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	r := newRouter(f)

	// No flat-rate lane at all: even the generalist seat is uncovered —
	// the decision carries gap advice and no runnable seat.
	r.Coverage = worker.Coverage{}
	d, err := r.Route(context.Background(), worker.RouteQuery{
		Requester: "alice", TaskID: "t-7", TaskText: "anything",
		Family: "software", Domain: "software", Classes: []string{"C1"},
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if d.GapAdvice == "" || d.Model != "" {
		t.Fatalf("uncovered seat must carry gap advice and no model: %+v", d)
	}
	if !strings.Contains(d.GapAdvice, "config-derived") {
		t.Fatalf("advice must disclose its config-derived basis (P-T17-3 machinery is B5): %q", d.GapAdvice)
	}

	// The metered branch structurally refuses even when a hook claims an
	// exception — the metered-exception list is EMPTY at v0 (G1 P7; D5).
	r.Coverage = worker.Coverage{MeteredAllowed: func(string) bool { return true }}
	_, err = r.Route(context.Background(), worker.RouteQuery{
		Requester: "alice", TaskID: "t-8", TaskText: "anything",
		Family: "software", Domain: "software", Classes: []string{"C1"},
	})
	if !errors.Is(err, worker.ErrInvalid) {
		t.Fatalf("metered selection must structurally refuse at v0, got %v", err)
	}
}

func TestRouteModelPinCoverageGapAdvice(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")

	// A worker pinning a model (with its recorded reason) on a lane the
	// owner does not hold: the gap is a MODEL, not a worker — the 2.7
	// advice leg fires and the task falls to the generalist (S08.8 no-fit
	// stage 2).
	src := strings.Replace(agenticSrc, "  effort_floor: standard",
		"  effort_floor: standard\n  model_pin: some-frontier-x\n  model_pin_reason: needs the X family", 1)
	_, v, err := f.store.CreateDraft(context.Background(), "alice", src, readGrants(), humanProv())
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	dry := &fakeDry{}
	if _, err := f.store.RunBattery(context.Background(), v.ID, worker.BatteryInput{
		Actor: "alice", SampleTask: "review the sample diff", Engine: dry,
		Model: "claude-haiku-4-5", EnginePin: "claude-cli@2.1.216",
	}); err != nil {
		t.Fatalf("RunBattery: %v", err)
	}
	if _, err := f.store.Approve(context.Background(), "alice", v.ID, worker.ApproveOpts{}); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	r := newRouter(f)
	// Coverage covers the anthropic lane, but the pin is only "covered"
	// through its lane at v0 — simulate the uncovered pin by removing
	// coverage so the pin's lane check fails.
	r.Coverage = worker.Coverage{}
	d, err := r.Route(context.Background(), worker.RouteQuery{
		Requester: "alice", TaskID: "t-9", TaskText: "review my code",
		Family: "read-analyze", Domain: "software",
		Classes: []string{"C1"}, Tools: []string{"Read"},
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if !d.Generalist || d.GapAdvice == "" {
		t.Fatalf("model-gap advice missing: %+v", d)
	}
}

func TestRouteMechanicalHelperDegradesLocalAbsence(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	r := newRouter(f)

	d, err := r.Route(context.Background(), worker.RouteQuery{
		Requester: "alice", TaskID: "t-10", TaskText: "grep the logs",
		Family: "software", Domain: "software", Classes: []string{"C1"},
		SpawnTrigger: "T-CTX", Mechanical: true,
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if !strings.Contains(d.PlainReason, "local free tier") || !strings.Contains(d.PlainReason, "not configured") {
		t.Fatalf("local-tier absence not recorded: %q", d.PlainReason)
	}
	if d.Model != "claude-sonnet-5" {
		t.Fatalf("degraded mechanical duty must ride the paid execution seat, got %q", d.Model)
	}
	found := false
	for _, s := range d.Signals {
		if s == "spawn_trigger=T-CTX" {
			found = true
		}
	}
	if !found {
		t.Fatalf("spawn trigger missing from signals: %v", d.Signals)
	}
}

func TestSearchIndexMaintainedAtApproveAndRepoint(t *testing.T) {
	f := newFix(t)
	f.user("alice", "member")
	tpl, _ := approveReviewer(t, f, "alice")

	var n int
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM worker_search WHERE template_id = ?`, tpl.ID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("worker_search rows = %d, want 1 after Approve", n)
	}
	// The FTS leg finds the worker by delegation-description words that
	// appear in NO selector — the S08.8 description channel.
	var hit string
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT template_id FROM worker_search WHERE worker_search MATCH '"style" OR "regressions"'`).Scan(&hit); err != nil {
		t.Fatalf("fts query: %v", err)
	}
	if hit != tpl.ID {
		t.Fatalf("fts hit = %q, want %s", hit, tpl.ID)
	}
}

func TestTighterClass(t *testing.T) {
	cases := []struct{ step, worker, want string }{
		{"C2", "C1", "C1"},
		{"C1", "C2", "C1"},
		{"C2", "C2", "C2"},
		{"C1", "bogus", "C1"},
	}
	for _, c := range cases {
		if got := worker.TighterClass(c.step, c.worker); got != c.want {
			t.Errorf("TighterClass(%s,%s) = %s, want %s", c.step, c.worker, got, c.want)
		}
	}
}

func TestDefaultDutyMapShape(t *testing.T) {
	m := worker.DefaultDutyMap()
	// Seat mix ratified at the B3 gate 2026-07-22 (operator D3): the
	// advisor split — opus plans, sonnet executes, cross-model opus judge
	// (P3/gates/B3-report.md §7).
	want := map[string]string{
		worker.DutyExecution: "claude-sonnet-5",
		worker.DutyPlanning:  "claude-opus-4-8",
		worker.DutyJudge:     "claude-opus-4-8",
	}
	for duty, model := range want {
		seat, ok := m[duty]
		if !ok {
			t.Fatalf("duty %s has no seat", duty)
		}
		if seat.Model != model || seat.Lane != "anthropic" || seat.WindowTokens != 200_000 {
			t.Fatalf("seat %s = %+v, want model %s on anthropic at the 200k budgeting floor", duty, seat, model)
		}
	}
	if _, ok := m[worker.DutyUtility]; ok {
		t.Fatalf("utility seat must be ABSENT (S06.10 pins it local; S12 is B4 — absent duties degrade, never fake)")
	}
}

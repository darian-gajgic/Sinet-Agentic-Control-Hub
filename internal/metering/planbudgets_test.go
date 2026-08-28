package metering

// planbudgets_test.go — P3-LN-6 (S10.4, S18.3): the plan-budget store, the
// (person, lane, window) grain, and the proposal that seeds a row.
//
// $0: temp-dir databases, the shipped plan documents, no provider anywhere.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

// ln6Row is the shape every test here declares from, so a case says what it
// changes rather than restating a row.
func ln6Row(user, lane, window, unit string, units, hours float64, at time.Time) PlanBudgetRow {
	return PlanBudgetRow{
		UserID: user, Lane: lane, Window: window,
		PeriodUnits: units, Unit: unit,
		PeriodStart: at, PeriodHours: hours,
		Source:     PlanBudgetOperatorSet,
		DeclaredTS: at, DeclaredBy: user,
	}
}

// T1 · the row round-trips, the re-declaration reports what it replaced, and an
// undeclared window is an ABSENCE rather than an error or a zero.
func TestLN6PlanBudgetRowRoundTrips(t *testing.T) {
	e := newEnv(t)
	seedUser(t, e, "alice")
	b := NewPlanBudgets(e.db)
	ctx := context.Background()
	at := time.Date(2026, 8, 25, 9, 30, 15, 123456789, time.UTC)

	first := ln6Row("alice", "zai", PlanQuotaWindow, "credits", 14000, 5, at)
	first.Source, first.SeededFrom, first.Fraction = PlanBudgetProposalSeeded, PlanQuotaWindow, 0.5
	prior, existed, err := b.Declare(ctx, first)
	if err != nil || existed {
		t.Fatalf("first Declare = (existed %v, err %v), want (false, nil)", existed, err)
	}
	if (prior != PlanBudgetRow{}) {
		t.Fatalf("first Declare returned a prior row %+v, want the zero row — nothing was replaced", prior)
	}

	got, ok, err := b.Row(ctx, "alice", "zai", PlanQuotaWindow)
	if err != nil || !ok {
		t.Fatalf("Row = (ok %v, err %v), want the row just declared", ok, err)
	}
	assertPlanRow(t, "round trip", got, first)

	// The honest absence: the lane's OTHER window carries no row, and that is
	// not an error.
	if _, ok, err := b.Row(ctx, "alice", "zai", PlanQuotaWeekly); err != nil || ok {
		t.Fatalf("Row(weekly) = (ok %v, err %v), want (false, nil) — an undeclared window is an absence", ok, err)
	}
	if budget, err := b.PlanBudget(ctx, "alice", "zai", PlanQuotaWeekly); err != nil || budget.Declared {
		t.Fatalf("PlanBudget(weekly) = (%+v, %v), want the undeclared posture — no caller may invent a denominator (D4)", budget, err)
	}

	// A re-declaration REPLACES and hands back exactly what it replaced, which
	// is the only place the old value still exists at that moment (S14.2).
	second := ln6Row("alice", "zai", PlanQuotaWindow, "credits", 9000, 5, at.Add(time.Hour))
	second.DeclaredBy = "op"
	prior, existed, err = b.Declare(ctx, second)
	if err != nil || !existed {
		t.Fatalf("re-Declare = (existed %v, err %v), want (true, nil)", existed, err)
	}
	assertPlanRow(t, "prior", prior, first)

	got, _, _ = b.Row(ctx, "alice", "zai", PlanQuotaWindow)
	assertPlanRow(t, "after re-declaration", got, second)

	rows, err := b.Rows(ctx, "alice", "zai")
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if len(rows) != 1 || rows[0].Window != PlanQuotaWindow {
		t.Fatalf("Rows = %+v, want exactly the one declared window", rows)
	}
	// Declaring the second window adds a row rather than moving the first.
	weekly := ln6Row("alice", "zai", PlanQuotaWeekly, "credits", 70000, 168, at)
	if _, _, err := b.Declare(ctx, weekly); err != nil {
		t.Fatalf("Declare(weekly): %v", err)
	}
	rows, _ = b.Rows(ctx, "alice", "zai")
	if len(rows) != 2 || rows[0].Window != PlanQuotaWindow || rows[1].Window != PlanQuotaWeekly {
		t.Fatalf("Rows = %+v, want both declared windows in window order", rows)
	}
}

// assertPlanRow compares a persisted row field for field, with the timestamps
// compared as instants: they round-trip through RFC3339Nano UTC text, and a
// store that dropped a nanosecond would be reporting a period that starts
// somewhere else.
func assertPlanRow(t *testing.T, what string, got, want PlanBudgetRow) {
	t.Helper()
	if got.UserID != want.UserID || got.Lane != want.Lane || got.Window != want.Window {
		t.Errorf("%s: key = %q/%s/%s, want %q/%s/%s", what, got.UserID, got.Lane, got.Window, want.UserID, want.Lane, want.Window)
	}
	if got.PeriodUnits != want.PeriodUnits || got.Unit != want.Unit || got.PeriodHours != want.PeriodHours {
		t.Errorf("%s: %v %s over %vh, want %v %s over %vh",
			what, got.PeriodUnits, got.Unit, got.PeriodHours, want.PeriodUnits, want.Unit, want.PeriodHours)
	}
	if !got.PeriodStart.Equal(want.PeriodStart) || !got.DeclaredTS.Equal(want.DeclaredTS) {
		t.Errorf("%s: period_start %s / declared_ts %s, want %s / %s",
			what, got.PeriodStart, got.DeclaredTS, want.PeriodStart, want.DeclaredTS)
	}
	if got.PeriodStart.Location() != time.UTC || got.DeclaredTS.Location() != time.UTC {
		t.Errorf("%s: timestamps came back in %s/%s, want UTC", what, got.PeriodStart.Location(), got.DeclaredTS.Location())
	}
	if got.Source != want.Source || got.SeededFrom != want.SeededFrom || got.Fraction != want.Fraction {
		t.Errorf("%s: provenance = %s/%q/%v, want %s/%q/%v",
			what, got.Source, got.SeededFrom, got.Fraction, want.Source, want.SeededFrom, want.Fraction)
	}
	if got.DeclaredBy != want.DeclaredBy {
		t.Errorf("%s: declared_by = %q, want %q", what, got.DeclaredBy, want.DeclaredBy)
	}
}

// T2 · the store refuses what the schema refuses, and each refusal says what is
// wrong. A defect never becomes a row.
func TestLN6PlanBudgetRefusesDefects(t *testing.T) {
	e := newEnv(t)
	seedUser(t, e, "alice")
	b := NewPlanBudgets(e.db)
	ctx := context.Background()
	at := time.Now().UTC()

	for _, tc := range []struct {
		name, says string
		mut        func(r *PlanBudgetRow)
	}{
		{"no person", "person", func(r *PlanBudgetRow) { r.UserID = "" }},
		{"no lane", "lane", func(r *PlanBudgetRow) { r.Lane = "" }},
		{"no window", "window", func(r *PlanBudgetRow) { r.Window = "" }},
		{"zero units", "positive", func(r *PlanBudgetRow) { r.PeriodUnits = 0 }},
		{"negative units", "positive", func(r *PlanBudgetRow) { r.PeriodUnits = -1 }},
		{"no period length", "hours", func(r *PlanBudgetRow) { r.PeriodHours = 0 }},
		{"negative period length", "hours", func(r *PlanBudgetRow) { r.PeriodHours = -5 }},
		{"no unit", "unit", func(r *PlanBudgetRow) { r.Unit = "" }},
		{"no declaring actor", "actor", func(r *PlanBudgetRow) { r.DeclaredBy = "" }},
		{"a source outside the vocabulary", "source", func(r *PlanBudgetRow) { r.Source = "guessed" }},
		{"an empty source", "source", func(r *PlanBudgetRow) { r.Source = "" }},
		{"no declared instant", "declared", func(r *PlanBudgetRow) { r.DeclaredTS = time.Time{} }},
		// D4/D5: the unit is the DOCUMENT's for that window and is not the
		// caller's to choose, and a window the plan does not declare
		// denominates nothing.
		{"a made-up unit", "unit", func(r *PlanBudgetRow) { r.Unit = "bananas" }},
		{"a window the plan does not declare", "fortnightly", func(r *PlanBudgetRow) { r.Window = "fortnightly" }},
		{"a lane that meters in no plan units", "anthropic", func(r *PlanBudgetRow) { r.Lane = "anthropic" }},
	} {
		row := ln6Row("alice", "zai", PlanQuotaWindow, "credits", 14000, 5, at)
		tc.mut(&row)
		_, _, err := b.Declare(ctx, row)
		if err == nil {
			t.Errorf("%s: Declare accepted %+v", tc.name, row)
			continue
		}
		if !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%s: error %q does not name what is wrong (want it to mention %q)", tc.name, err, tc.says)
		}
	}

	// A budget declared for nobody is a decision recorded about nobody.
	ghost := ln6Row("nobody", "zai", PlanQuotaWindow, "credits", 14000, 5, at)
	if _, _, err := b.Declare(ctx, ghost); !errors.Is(err, ErrNoSuchPerson) {
		t.Errorf("Declare for an unknown person = %v, want ErrNoSuchPerson", err)
	}
	rows, err := b.Rows(ctx, "alice", "zai")
	if err != nil || len(rows) != 0 {
		t.Errorf("after the refusals the store holds %+v (err %v), want nothing", rows, err)
	}
}

// T3 · a window can only carry a budget when it counts what CONSUMPTION
// counts, and a lane's other windows still report their own allowances.
//
// This is the drain r1 D1 finding, pinned BOTH WAYS. The Kimi plan counts
// REQUESTS (its charging multiplier is per request) and publishes its 7-day
// allowance in CREDITS. A budget on that weekly window would divide requests by
// credits, report the answer labeled "credits", and — under the
// most-constrained-window rule — that incoherent ratio could BIND the lane and
// steer dispatch. So it is refused at the store, refused at the boundary, and
// refused again by the reading if a row ever appears by another route.
func TestLN6WindowCarriesABudgetOnlyWhenItCountsWhatConsumptionCounts(t *testing.T) {
	e := newEnv(t)
	seedUser(t, e, "bob")
	e.runningRun(t, "rk", "bob", "kimi", "opencode")
	e.checkpoint(t, "rk", "kimi-k2", `{"input_tokens":100,"output_tokens":40}`)
	b := NewPlanBudgets(e.db)
	g := NewPressureGauge(e.db, e.reg)
	ctx := context.Background()
	// The period opens BEFORE the seeded consumption, or the reading would
	// honestly count nothing and the "it binds" half would prove nothing.
	at := time.Now().UTC().Add(-time.Hour)

	doc, ok := PlanDocFor("kimi")
	if !ok {
		t.Fatal("no plan document for lane kimi — this test's premise moved")
	}
	// The premise the finding rests on: these two windows really are
	// denominated differently, and consumption is counted in the document's own
	// unit.
	if doc.QuotaUnit(PlanQuotaWindow) == doc.QuotaUnit(PlanQuotaWeekly) {
		t.Fatal("the kimi plan's two windows now share a unit — the premise of this whole test moved")
	}
	if doc.QuotaUnit(PlanQuotaWindow) != doc.Unit {
		t.Fatalf("the kimi rolling window counts %q while consumption counts %q — the coherent case is gone",
			doc.QuotaUnit(PlanQuotaWindow), doc.Unit)
	}

	// ── the COHERENT window still works, unchanged ──────────────────────────
	rolling := ln6Row("bob", "kimi", PlanQuotaWindow, doc.QuotaUnit(PlanQuotaWindow), 150, 5, at)
	if _, _, err := b.Declare(ctx, rolling); err != nil {
		t.Fatalf("Declare(rolling-5h): %v", err)
	}
	budget, err := b.PlanBudget(ctx, "bob", "kimi", PlanQuotaWindow)
	if err != nil {
		t.Fatalf("PlanBudget(rolling-5h): %v", err)
	}
	r, err := g.ReadPlanUnits(ctx, "bob", "kimi", doc, budget, time.Now().UTC())
	if err != nil {
		t.Fatalf("ReadPlanUnits(rolling-5h): %v", err)
	}
	if !r.Applicable || r.Pressure <= 0 {
		t.Fatalf("the coherent rolling-5h budget did not bind: %+v", r)
	}
	if r.Unit != doc.Unit {
		t.Errorf("reading unit = %q, want %q — the reading says what was COUNTED", r.Unit, doc.Unit)
	}
	if r.InapplicableNote != "" {
		t.Errorf("an applicable reading carries a refusal note: %q", r.InapplicableNote)
	}

	// ── the INCOHERENT window is refused at the store ───────────────────────
	weekly := ln6Row("bob", "kimi", PlanQuotaWeekly, doc.QuotaUnit(PlanQuotaWeekly), 5000, 168, at)
	_, _, err = b.Declare(ctx, weekly)
	if err == nil {
		t.Fatal("the store accepted a budget on a window denominated in something other than what consumption counts — " +
			"that row divides requests by credits and can BIND routing under max-binds (drain r1 D1)")
	}
	for _, want := range []string{doc.QuotaUnit(PlanQuotaWeekly), doc.Unit, PlanQuotaWeekly} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
	if _, ok, rerr := b.Row(ctx, "bob", "kimi", PlanQuotaWeekly); ok || rerr != nil {
		t.Errorf("the refused weekly budget was stored anyway (ok=%v err=%v)", ok, rerr)
	}

	// ── and if such a row EXISTS anyway, the reading refuses it ─────────────
	//
	// Defence in depth: the store is not the only way bytes reach a table, and
	// a hand-inserted row must not steer dispatch.
	planted := weekly.Budget()
	pr, err := g.ReadPlanUnits(ctx, "bob", "kimi", doc, planted, time.Now().UTC())
	if err != nil {
		t.Fatalf("ReadPlanUnits(planted weekly): %v", err)
	}
	if pr.Applicable || pr.Pressure != 0 || pr.BackgroundCeiling != 0 {
		t.Fatalf("a planted incoherent row produced a usable ratio: %+v", pr)
	}
	if pr.InapplicableNote == "" {
		t.Error("the reading refused the row and said nothing about why — a silent skip is how it comes back")
	}
	if pr.Unit != doc.Unit {
		t.Errorf("the refused reading is labeled %q, want %q — labeling a requests count \"credits\" is the mislabel this finding is about",
			pr.Unit, doc.Unit)
	}

	// The weekly ALLOWANCE is still reported. Refusing it as a denominator is
	// not hiding it.
	byName := map[string]PlanWindow{}
	for _, w := range pr.Windows {
		byName[w.Name] = w
	}
	if byName[PlanQuotaWeekly].Unit != doc.QuotaUnit(PlanQuotaWeekly) {
		t.Errorf("the weekly window stopped reporting its own unit: %+v", byName[PlanQuotaWeekly])
	}
}

// T3b · the coherence rule is one predicate, and it answers for both lanes.
func TestLN6PlanBudgetWindowRefusalIsTheOneRule(t *testing.T) {
	for _, tc := range []struct {
		lane, window string
		wantRefused  bool
	}{
		{"zai", PlanQuotaWindow, false},
		{"zai", PlanQuotaWeekly, false},
		{"kimi", PlanQuotaWindow, false},
		{"kimi", PlanQuotaWeekly, true},
		{"kimi", "fortnightly", true},
	} {
		doc, ok := PlanDocFor(tc.lane)
		if !ok {
			t.Fatalf("no plan document for lane %s", tc.lane)
		}
		refusal := PlanBudgetWindowRefusal(doc, tc.window)
		if got := refusal != ""; got != tc.wantRefused {
			t.Errorf("%s/%s refused = %v (%q), want %v", tc.lane, tc.window, got, refusal, tc.wantRefused)
		}
	}
}

// T3c · a declared period that has ELAPSED is not a budget for the current one.
//
// period_hours used to be stored, CHECK-constrained and served while no
// arithmetic read it, so a rolling-5h budget declared a month ago still counted
// a month of consumption against a five-hour allowance (drain r1 D6).
func TestLN6ElapsedPeriodStopsApplying(t *testing.T) {
	e := newEnv(t)
	seedUser(t, e, "bob")
	e.runningRun(t, "rz", "bob", "zai", "opencode")
	e.datedCheckpoint(t, "rz", "bob", "glm-5.3", `{"input_tokens":10,"output_tokens":5}`, zaiOffPeak)
	g := NewPressureGauge(e.db, e.reg)
	ctx := context.Background()

	doc, _ := PlanDocFor("zai")
	quota, _ := doc.Quota(PlanQuotaWindow)
	budget := PlanBudget{
		PeriodUnits: 100, PeriodStart: zaiOffPeak.Add(-time.Hour),
		PeriodHours: quota.WindowHours, Declared: true, SeededFrom: PlanQuotaWindow,
	}

	inside, err := g.ReadPlanUnits(ctx, "bob", "zai", doc, budget, zaiOffPeak.Add(time.Hour))
	if err != nil {
		t.Fatalf("ReadPlanUnits(inside the period): %v", err)
	}
	if !inside.Applicable || inside.InapplicableNote != "" {
		t.Fatalf("a live period did not apply: %+v", inside)
	}

	after := budget.PeriodStart.Add(time.Duration(budget.PeriodHours*float64(time.Hour)) + time.Minute)
	elapsed, err := g.ReadPlanUnits(ctx, "bob", "zai", doc, budget, after)
	if err != nil {
		t.Fatalf("ReadPlanUnits(after the period): %v", err)
	}
	if elapsed.Applicable || elapsed.Pressure != 0 || elapsed.BackgroundCeiling != 0 {
		t.Fatalf("an elapsed %v-hour period still denominated the reading: %+v — counting a month of consumption "+
			"against a five-hour allowance reports a pressure that only rises and calls it headroom",
			budget.PeriodHours, elapsed)
	}
	if !strings.Contains(elapsed.InapplicableNote, "declaring again is what starts the next period") {
		t.Errorf("the expired reading does not say what re-opens it: %q", elapsed.InapplicableNote)
	}
	// The consumption figure survives: only the DENOMINATOR expired.
	if elapsed.Calls != inside.Calls {
		t.Errorf("an expired period changed the consumption count (%d vs %d)", elapsed.Calls, inside.Calls)
	}
}

// T4 · the proposal is the published allowance at ⚙
// budget.background_window_fraction, read BY NAME — proven by moving the
// registry value and watching the proposal move with it. And a window whose
// allowance nobody published cannot be proposed from at all.
func TestLN6ProposalSeedsFromTheAllowanceAtTheRegistryFraction(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.reg.Attach(ctx, e.db, eventlog.New(e.db, e.reg)); err != nil {
		t.Fatalf("attach registry: %v", err)
	}
	g := NewPressureGauge(e.db, e.reg)
	start := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)

	zai, ok := PlanDocFor("zai")
	if !ok {
		t.Fatal("no plan document for lane zai")
	}
	quota, ok := zai.Quota(PlanQuotaWindow)
	if !ok {
		t.Fatalf("the zai plan no longer declares a %s window", PlanQuotaWindow)
	}
	fraction, err := e.reg.Float(keyBgWindowFraction)
	if err != nil {
		t.Fatalf("read ⚙ %s: %v", keyBgWindowFraction, err)
	}

	budget, err := g.ProposePlanBudget(zai, PlanQuotaWindow, start)
	if err != nil {
		t.Fatalf("ProposePlanBudget: %v", err)
	}
	if want := quota.Units * fraction; budget.PeriodUnits != want {
		t.Errorf("proposed %v units, want %v (the published allowance at ⚙ %s)", budget.PeriodUnits, want, keyBgWindowFraction)
	}
	if budget.SeededFrom != PlanQuotaWindow || budget.Fraction != fraction {
		t.Errorf("proposal provenance = %q/%v, want %q/%v", budget.SeededFrom, budget.Fraction, PlanQuotaWindow, fraction)
	}
	if budget.PeriodHours != quota.WindowHours {
		t.Errorf("proposed period = %vh, want the window's own %vh", budget.PeriodHours, quota.WindowHours)
	}
	if !budget.Declared {
		t.Error("a proposal that is not Declared is a denominator the reading will never use")
	}

	// Move the registry value: the proposal must move with it. A Go constant
	// standing in for the key would sail through everything above and fail here.
	moved := fraction / 2
	if err := e.reg.Set(ctx, settings.SetRequest{
		Key: keyBgWindowFraction, Value: json.RawMessage(mustJSON(t, moved)),
		Actor: settings.Actor{Kind: settings.ActorOperator, ID: "op"}, Reason: "P3-LN-6 T4",
	}); err != nil {
		t.Fatalf("move ⚙ %s: %v", keyBgWindowFraction, err)
	}
	after, err := g.ProposePlanBudget(zai, PlanQuotaWindow, start)
	if err != nil {
		t.Fatalf("ProposePlanBudget after the move: %v", err)
	}
	if want := quota.Units * moved; after.PeriodUnits != want {
		t.Errorf("after moving ⚙ %s to %v the proposal is %v units, want %v — the fraction is read by NAME, never as a constant",
			keyBgWindowFraction, moved, after.PeriodUnits, want)
	}
	if after.Fraction != moved {
		t.Errorf("the proposal records fraction %v, want the moved %v", after.Fraction, moved)
	}

	// A window whose allowance nobody published has nothing to take a fraction
	// OF, and seeding one would mint the inferred provider window D4 bars.
	kimi, ok := PlanDocFor("kimi")
	if !ok {
		t.Fatal("no plan document for lane kimi")
	}
	_, err = g.ProposePlanBudget(kimi, PlanQuotaWeekly, start)
	if !errors.Is(err, ErrPlanDoc) {
		t.Fatalf("ProposePlanBudget(kimi/%s) = %v, want ErrPlanDoc", PlanQuotaWeekly, err)
	}
	if !strings.Contains(err.Error(), "publishes no allowance") {
		t.Errorf("the refusal does not say what is missing: %q", err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %v: %v", v, err)
	}
	return b
}

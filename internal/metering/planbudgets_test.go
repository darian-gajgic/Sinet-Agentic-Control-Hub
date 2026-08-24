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

// T3 · one lane's two windows keep their OWN units. This is the whole reason
// the grain carries a window: the Kimi plan counts REQUESTS over five hours and
// CREDITS over seven days, and one lane-wide scalar cannot describe that.
func TestLN6TwoWindowsOnOneLaneKeepTheirOwnUnits(t *testing.T) {
	e := newEnv(t)
	seedUser(t, e, "bob")
	e.runningRun(t, "rk", "bob", "kimi", "opencode")
	e.checkpoint(t, "rk", "kimi-k2", `{"input_tokens":100,"output_tokens":40}`)
	b := NewPlanBudgets(e.db)
	g := NewPressureGauge(e.db, e.reg)
	ctx := context.Background()
	at := time.Now().UTC()

	doc, ok := PlanDocFor("kimi")
	if !ok {
		t.Fatal("no plan document for lane kimi — this test's premise moved")
	}
	for _, w := range []struct {
		window string
		units  float64
		hours  float64
	}{
		{PlanQuotaWindow, 150, 5},
		{PlanQuotaWeekly, 5000, 168},
	} {
		row := ln6Row("bob", "kimi", w.window, doc.QuotaUnit(w.window), w.units, w.hours, at)
		if _, _, err := b.Declare(ctx, row); err != nil {
			t.Fatalf("Declare(%s): %v", w.window, err)
		}
	}

	rows, err := b.Rows(ctx, "bob", "kimi")
	if err != nil || len(rows) != 2 {
		t.Fatalf("Rows = %+v (err %v), want both windows persisted independently", rows, err)
	}

	units := map[string]string{}
	for _, row := range rows {
		budget, err := b.PlanBudget(ctx, "bob", "kimi", row.Window)
		if err != nil {
			t.Fatalf("PlanBudget(%s): %v", row.Window, err)
		}
		r, err := g.ReadPlanUnits(ctx, "bob", "kimi", doc, budget, at)
		if err != nil {
			t.Fatalf("ReadPlanUnits(%s): %v", row.Window, err)
		}
		if want := doc.QuotaUnit(row.Window); r.Unit != want {
			t.Errorf("the %s reading is denominated in %q, want the window's own unit %q — rendering both gauges "+
				"in one unit is a quiet lie about what was counted", row.Window, r.Unit, want)
		}
		if !r.Applicable {
			t.Errorf("a declared %s budget did not make the reading applicable", row.Window)
		}
		units[row.Window] = r.Unit
	}
	if units[PlanQuotaWindow] == units[PlanQuotaWeekly] {
		t.Errorf("both windows read in %q — this lane's windows are denominated DIFFERENTLY (requests vs credits), "+
			"and collapsing them is the single-scalar failure the (person, lane, window) grain exists to prevent",
			units[PlanQuotaWindow])
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

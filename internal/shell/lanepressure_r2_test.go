package shell

// lanepressure_r2_test.go — P3-LN-4 drain r2 / R2 evidence, RETIRED AND
// INVERTED at P3-LN-6 on 2026-08-25 (S10.4, S08.8, D5).
//
// WHAT THIS FILE USED TO SAY. Through LN-4 it pinned a limitation: the
// production adapter routed a lane carrying a PLAN DOCUMENT to the tier-3 plan
// reading against `UndeclaredPlanBudget()`, no surface for declaring a plan
// budget existed, and so zai and kimi answered "not applicable" no matter what
// an operator declared. Its failing assertion said, in its own words, that the
// day the plan-budget surface landed this test would fail and name the claim to
// revisit: "the both-directions selection case can and should be driven through
// the production adapter."
//
// WHAT CHANGED. P3-LN-6 landed that surface: a per-(person, lane, window) plan
// budget store, its declaration verb, and `routePressure`/`projMeter` consuming
// declared rows. So the claim about UNREACHABILITY is retired, and the
// both-directions selection case is driven through the production adapter in
// planbudget_ln6_surface_test.go.
//
// WHAT SURVIVES, because it is still true and is the trap most likely to be
// re-introduced: a TOKEN budget declared on a plan-documented lane changes
// nothing, because that lane is not read from the token gauge at all. Only a
// PLAN budget, in the plan's own unit, gives that lane a comparable ratio.
//
// $0: real gauge, real database, seeded rows, no provider anywhere near it.

import (
	"context"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
)

func TestLN6PlanDocumentedLaneIsComparableOnlyOnAPlanBudget(t *testing.T) {
	ctx := context.Background()
	e := newPressureEnv(t)
	budgets := metering.NewBudgets(e.db)
	planBudgets := metering.NewPlanBudgets(e.db)
	// The production reader, with the plan-budget store the surface added.
	rp := routePressure{g: metering.NewPressureGauge(e.db, e.reg), b: budgets, pb: planBudgets}

	declareTokens := func(lane string) {
		t.Helper()
		if _, _, err := budgets.Declare(ctx, metering.BudgetRow{
			UserID: "alice", Lane: lane, PeriodTokens: 6000,
			PeriodStart: time.Now().UTC().Add(-time.Hour), PeriodDays: 7,
			DeclaredTS: time.Now().UTC(), DeclaredBy: "alice",
		}); err != nil {
			t.Fatalf("Declare(%s): %v", lane, err)
		}
	}

	// The control: a lane with NO plan document answers from the tier-1 token
	// gauge, so a declared token budget makes it comparable. The mechanism works.
	declareTokens(adapters.LaneAnthropic)
	anth, err := rp.Pressure(ctx, "alice", adapters.LaneAnthropic)
	if err != nil {
		t.Fatalf("Pressure(anthropic): %v", err)
	}
	if !anth.Applicable || anth.Ratio <= 0 {
		t.Fatalf("a declared budget did not make the anthropic lane comparable (%+v) — the findings below "+
			"would then be about a broken gauge rather than about the plan-budget surface", anth)
	}

	// The premise: this lane is still read as a PLAN lane. If it stopped
	// carrying a document, everything below would be about something else.
	if _, hasPlan := metering.PlanDocFor(adapters.LaneZAI); !hasPlan {
		t.Fatal("the zai lane no longer carries a plan document — this test's whole premise moved")
	}

	// SURVIVING FINDING: a TOKEN budget on a plan-documented lane changes
	// nothing. The lane is metered in the plan's own units and is not read from
	// the token gauge at all, so declaring tokens against it declares a
	// denominator for a reading nobody takes.
	declareTokens(adapters.LaneZAI)
	zai, err := rp.Pressure(ctx, "alice", adapters.LaneZAI)
	if err != nil {
		t.Fatalf("Pressure(zai, token budget only): %v", err)
	}
	if zai.Applicable {
		t.Errorf("a TOKEN budget made the plan-documented zai lane comparable (%+v).\n"+
			"That lane answers from the tier-3 plan reading in the plan's own unit; a weighted-token denominator "+
			"is a number for a different reading, and using it would compare a token against a credit (S10.1; §63 D3).", zai)
	}
	if zai.Unit != "credits" {
		t.Errorf("zai unit = %q, want credits — the plan meters in its own unit", zai.Unit)
	}

	// THE INVERTED CLAIM (P3-LN-6): a PLAN budget, in the plan's own unit, is
	// what makes the lane comparable — and it is now declarable.
	doc, _ := metering.PlanDocFor(adapters.LaneZAI)
	quota, ok := doc.Quota(metering.PlanQuotaWindow)
	if !ok {
		t.Fatalf("the zai plan declares no %q window", metering.PlanQuotaWindow)
	}
	now := time.Now().UTC()
	if _, _, err := planBudgets.Declare(ctx, metering.PlanBudgetRow{
		UserID: "alice", Lane: adapters.LaneZAI, Window: metering.PlanQuotaWindow,
		PeriodUnits: 1400, Unit: doc.QuotaUnit(metering.PlanQuotaWindow),
		PeriodStart: now.Add(-time.Hour), PeriodHours: quota.WindowHours,
		Source: metering.PlanBudgetOperatorSet, DeclaredTS: now, DeclaredBy: "alice",
	}); err != nil {
		t.Fatalf("Declare(plan budget): %v", err)
	}
	zai, err = rp.Pressure(ctx, "alice", adapters.LaneZAI)
	if err != nil {
		t.Fatalf("Pressure(zai, plan budget): %v", err)
	}
	if !zai.Applicable || zai.Ratio <= 0 {
		t.Fatalf("a declared PLAN budget did not make the zai lane comparable (%+v) — with no comparable ratio "+
			"`chooseFlatLane` stops at the deterministic duty-map order and a commissioned lane can never win "+
			"the execution seat through normal intake (P3-LN-6 R6)", zai)
	}
	if zai.Unit != "credits" {
		t.Errorf("zai unit = %q after declaring, want credits", zai.Unit)
	}
}

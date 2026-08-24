package shell

// lanepressure_r2_test.go — P3-LN-4 drain r2 / R2 evidence (S10.4, S08.8, D5).
//
// The drain asked for a case where a commissioned lane WINS selection through
// the real pressure reading, with budget-seeded rows making the ratio favour
// it. That case is not reachable at v0, and this is the proof rather than the
// assertion: the production adapter routes a lane carrying a PLAN DOCUMENT to
// the tier-3 plan reading against `UndeclaredPlanBudget()`, and no surface for
// declaring a plan budget exists yet (13.4). So the reading answers
// "not applicable" for zai and kimi no matter what an operator declares, and
// `chooseFlatLane` stops at the deterministic duty-map order before any ratio
// is compared.
//
// Pinning it here means the day the 13.4 plan-budget surface lands, this test
// fails and says exactly which claim has to be revisited.
//
// $0: real gauge, real database, seeded rows, no provider anywhere near it.

import (
	"context"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
)

func TestLN4PlanDocumentedLaneHasNoComparablePressureAtV0(t *testing.T) {
	ctx := context.Background()
	e := newPressureEnv(t)
	budgets := metering.NewBudgets(e.db)

	declare := func(lane string) {
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
	// gauge, so a declared budget makes it comparable. The mechanism works.
	declare(adapters.LaneAnthropic)
	anth, err := e.rp.Pressure(ctx, "alice", adapters.LaneAnthropic)
	if err != nil {
		t.Fatalf("Pressure(anthropic): %v", err)
	}
	if !anth.Applicable || anth.Ratio <= 0 {
		t.Fatalf("a declared budget did not make the anthropic lane comparable (%+v) — the finding below "+
			"would then be about a broken gauge rather than about the plan-budget surface", anth)
	}

	// The finding: the same declaration on a PLAN-DOCUMENTED lane changes
	// nothing, because that lane is not read from the token gauge at all.
	declare(adapters.LaneZAI)
	zai, err := e.rp.Pressure(ctx, "alice", adapters.LaneZAI)
	if err != nil {
		t.Fatalf("Pressure(zai): %v", err)
	}
	if _, hasPlan := metering.PlanDocFor(adapters.LaneZAI); !hasPlan {
		t.Fatal("the zai lane no longer carries a plan document — this test's whole premise moved")
	}
	if zai.Applicable {
		t.Errorf("the zai lane reports a comparable pressure ratio (%+v).\n"+
			"If a plan budget can now be declared, `chooseFlatLane` can separate two flat lanes on the real "+
			"reading and P3-LN-4 drain r2's recorded limitation is stale: the both-directions selection "+
			"case can and should be driven through the production adapter.", zai)
	}
	if zai.Unit != "credits" {
		t.Errorf("zai unit = %q, want credits — the plan meters in its own unit", zai.Unit)
	}
}

package shell

// planbudget_ln6_test.go — P3-LN-6 grounding, committed RED (S10.4, S08.8, S18.3).
//
// The ratified LN-2B lever has never been reachable: `routePressure.Pressure`
// and `projMeter.LaneMeter` both pass `metering.UndeclaredPlanBudget()`
// unconditionally for a lane carrying a plan document, so `ReadPlanUnits` can
// never reach its `budget.Declared && budget.PeriodUnits > 0` leg, pressure is
// never Applicable, and `chooseFlatLane` stops at the deterministic duty-map
// order before any ratio is compared. `metering.PressureGauge.ProposePlanBudget`
// implements the proposal and nothing in the tree calls it.
//
// These two tests are the structural half of that gap, red at the moment they
// were written and green only when the surface actually lands. They are
// deliberately STRUCTURAL rather than behavioural: the behavioural proofs
// (a declared budget makes pressure applicable; a commissioned lane wins and
// loses selection on consumption through the production adapter) cannot COMPILE
// before the store exists, and a red commit that does not compile is a finding
// (§65). Those are specified for the executor in P3/briefs/P3-LN-6.md §6.
//
// The executor MUST NOT delete or weaken either test.
//
// $0: one temp-dir database and one file read. Nothing here goes near a
// provider.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// TestLN6PlanBudgetRowsArePersisted — the store half.
//
// A plan budget is denominated in the PLAN's own unit, per window, and the
// ratified grain is (person, lane, period) [S18.3, "Per-person automation
// budgets"]. The landed `budgets` table (migration 0017) is keyed
// (user_id, lane) in weighted-consumption units with a whole-DAY period, which
// cannot express a 5-hour window counted in credits — nor a lane whose two
// windows are denominated differently (kimi: requests and credits). So the row
// needs a table of its own, and today there is none.
func TestLN6PlanBudgetRowsArePersisted(t *testing.T) {
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// The control: the TOKEN budget table is present, so a failure below is
	// about the missing plan-budget surface and not about a broken migration
	// runner.
	var control string
	if err := db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'budgets'`).Scan(&control); err != nil {
		t.Fatalf("the S10.4 token budget table is missing (%v) — the finding below would then be "+
			"about migrations rather than about the plan-budget surface", err)
	}

	var name string
	err = db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'plan_budgets'`).Scan(&name)
	if err != nil {
		t.Errorf("no `plan_budgets` table after migration (%v).\n"+
			"A plan-documented lane's budget is in the PLAN's own unit, per window, at the ratified "+
			"(person, lane, period) grain [S18.3] — kimi's rolling-5h window counts requests and its "+
			"weekly window counts credits, so one lane-wide scalar cannot describe it. Until the row "+
			"exists, `ProposePlanBudget` has nothing to write to and `ReadPlanUnits` has no denominator "+
			"but the honest absence (P3-LN-6 R1/R2).", err)
	}
}

// TestLN6RoutePressureConsumesDeclaredPlanBudgets — the consumption half.
//
// A source scan, on the no-lane-constants precedent (§62/§63 D5): the claim is
// about what the production code PATH does, and the two call sites are the
// whole finding. A behavioural assertion cannot be written until the store
// compiles, but the hardcode can be seen from here today.
//
// Both sites must change together. Changing only the router would make
// `GET /api/meters` contradict the lane it routes to — the drain-D2
// self-contradiction the token path's own comment records.
func TestLN6RoutePressureConsumesDeclaredPlanBudgets(t *testing.T) {
	raw, err := os.ReadFile("shell.go")
	if err != nil {
		t.Fatalf("read shell.go: %v", err)
	}
	src := string(raw)

	for _, site := range []struct{ fn, opens, why string }{
		{
			fn:    "routePressure.Pressure",
			opens: "func (p routePressure) Pressure(",
			why: "the S08.8 flat-lane ordering seam reads this. While it is hardcoded, a plan-documented " +
				"lane reports Applicable=false whatever an operator declares, `chooseFlatLane` falls to the " +
				"deterministic duty-map order, and a commissioned GLM/Kimi lane can never win the execution " +
				"seat through normal intake (P3-LN-6 R6)",
		},
		{
			fn:    "projMeter.LaneMeter",
			opens: "func (m projMeter) LaneMeter(",
			why: "`GET /api/meters` reads this. Leaving it hardcoded while the router consumes declared rows " +
				"makes the read surface contradict the routing decision beside it — the drain-D2 " +
				"self-contradiction the token path already documents against itself (P3-LN-6 R6)",
		},
	} {
		body, ok := funcBody(src, site.opens)
		if !ok {
			t.Fatalf("%s no longer opens with %q — this test's premise moved; re-ground it rather than deleting it", site.fn, site.opens)
		}
		if strings.Contains(body, "metering.UndeclaredPlanBudget()") {
			t.Errorf("%s still passes metering.UndeclaredPlanBudget() unconditionally for a plan-documented lane.\n"+
				"%s.\nThe undeclared posture must come from the plan-budget store — the one place that produces "+
				"it from storage, so no caller can invent a denominator (D4; the Budgets.Budget precedent).", site.fn, site.why)
		}
	}
}

// funcBody returns the source of the function opening with the given signature
// prefix, from the signature to the first line that is a bare closing brace at
// column 0. Gofmt makes that the function's own end, and reading the FUNCTION
// rather than the file is what keeps the scan from passing merely because the
// call moved somewhere else in shell.go.
func funcBody(src, opens string) (string, bool) {
	i := strings.Index(src, opens)
	if i < 0 {
		return "", false
	}
	rest := src[i:]
	if j := strings.Index(rest, "\n}\n"); j >= 0 {
		return rest[:j], true
	}
	return rest, true
}

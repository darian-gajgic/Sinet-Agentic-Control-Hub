package shell

// planbudget_ln6_surface_test.go — P3-LN-6 (S10.4, S08.8, S18.3): the declared
// plan budget consumed at the two production call sites.
//
// The structural halves are pinned in planbudget_ln6_test.go, which was
// committed red by the grounding. These are the behavioural halves: a declared
// budget makes a plan-documented lane's pressure comparable, a commissioned lane
// then WINS and LOSES selection on consumption through the production adapter,
// the honest absence is unchanged as a property, and the meters read agrees with
// the router it sits beside.
//
// $0: real database, real gauge, real checkpoints, real declared rows. Nothing
// here goes near a provider.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// ── the environment ─────────────────────────────────────────────────────────

// ln6Env is a real control-plane database with the production readers over it:
// the pressure gauge, both budget stores, and the routePressure adapter the
// S08.8 flat-lane rule consumes. Consumption is seeded per lane so a test can
// say which lane is the more worked one.
type ln6Env struct {
	db    *storage.DB
	reg   *settings.Registry
	log   *eventlog.Log
	runs  *run.Store
	cps   *gates.Checkpoints
	g     *metering.PressureGauge
	b     *metering.Budgets
	pb    *metering.PlanBudgets
	rp    routePressure
	meter projMeter
}

func newLN6Env(t *testing.T, people ...string) *ln6Env {
	t.Helper()
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
	log := eventlog.New(db, reg)
	if len(people) == 0 {
		people = []string{"alice"}
	}
	for _, who := range people {
		if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO users (user_id, display_name, role, created_ts) VALUES (?, ?, 'member', ?)`,
				who, who, time.Now().UTC().Format(time.RFC3339Nano))
			return err
		}); err != nil {
			t.Fatalf("seed user %s: %v", who, err)
		}
	}
	gauge := metering.NewPressureGauge(db, reg)
	planBudgets := metering.NewPlanBudgets(db)
	budgets := metering.NewBudgets(db)
	return &ln6Env{
		db: db, reg: reg, log: log,
		runs: run.NewStore(db, log), cps: gates.NewCheckpoints(db, log),
		g: gauge, b: budgets, pb: planBudgets,
		rp: routePressure{g: gauge, b: budgets, pb: planBudgets},
		meter: projMeter{
			ledger:      metering.NewLedger(db, nil, metering.NoMeteredExceptions(), reg),
			gauge:       gauge,
			budgets:     budgets,
			planBudgets: planBudgets,
		},
	}
}

// work drives a run to running on the lane and writes n checkpoints on it, so a
// lane's consumption is a number the test chose.
func (e *ln6Env) work(t *testing.T, runID, owner, lane, substrate, model string, n int, usage string) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.runs.Create(ctx, run.NewRun{ID: runID, UserID: owner, Lane: lane, Substrate: substrate}); err != nil {
		t.Fatalf("create %s: %v", runID, err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := e.runs.Transition(ctx, runID, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("transition %s→%s: %v", runID, st, err)
		}
	}
	for i := 0; i < n; i++ {
		if _, err := e.cps.Write(ctx, gates.NewCheckpoint{
			RunID: runID, ModelID: model, Usage: json.RawMessage(usage),
			SessionSubstrate: substrate, SessionID: "s-" + runID,
		}); err != nil {
			t.Fatalf("checkpoint %d on %s: %v", i, runID, err)
		}
	}
}

// declarePlan declares a plan budget the way the verb does: in the WINDOW's own
// unit, with the window's own length.
func (e *ln6Env) declarePlan(t *testing.T, owner, lane, window string, units float64) {
	t.Helper()
	doc, ok := metering.PlanDocFor(lane)
	if !ok {
		t.Fatalf("lane %s carries no plan document", lane)
	}
	quota, ok := doc.Quota(window)
	if !ok {
		t.Fatalf("the %s plan declares no %q window", lane, window)
	}
	now := time.Now().UTC()
	if _, _, err := e.pb.Declare(context.Background(), metering.PlanBudgetRow{
		UserID: owner, Lane: lane, Window: window,
		PeriodUnits: units, Unit: doc.QuotaUnit(window),
		PeriodStart: now.Add(-time.Hour), PeriodHours: quota.WindowHours,
		Source: metering.PlanBudgetOperatorSet, DeclaredTS: now, DeclaredBy: owner,
	}); err != nil {
		t.Fatalf("declare plan budget %s/%s: %v", lane, window, err)
	}
}

// expirePlanBudget walks a declared period's start back past its own length,
// which is what an ordinary clock does to a five-hour budget in five hours. It
// writes the row through the STORE so nothing here invents a shape the store
// would not have produced.
func expirePlanBudget(t *testing.T, e *ln6Env, owner, lane, window string) {
	t.Helper()
	row, ok, err := e.pb.Row(context.Background(), owner, lane, window)
	if err != nil || !ok {
		t.Fatalf("read the row to expire: ok=%v err=%v", ok, err)
	}
	row.PeriodStart = time.Now().UTC().Add(-time.Duration(row.PeriodHours*float64(time.Hour)) - time.Hour)
	if _, _, err := e.pb.Declare(context.Background(), row); err != nil {
		t.Fatalf("re-declare the row with an elapsed period: %v", err)
	}
}

func (e *ln6Env) declareTokens(t *testing.T, owner, lane string, tokens int64) {
	t.Helper()
	now := time.Now().UTC()
	if _, _, err := e.b.Declare(context.Background(), metering.BudgetRow{
		UserID: owner, Lane: lane, PeriodTokens: tokens,
		PeriodStart: now.Add(-time.Hour), PeriodDays: 7,
		DeclaredTS: now, DeclaredBy: owner,
	}); err != nil {
		t.Fatalf("declare token budget %s: %v", lane, err)
	}
}

// ── T5 · a declared plan budget makes the reading comparable ────────────────

func TestLN6DeclaredPlanBudgetMakesPressureApplicable(t *testing.T) {
	ctx := context.Background()
	e := newLN6Env(t)
	e.work(t, "r-z", "alice", adapters.LaneZAI, adapters.SubstrateOpencode, "glm-x", 4,
		`{"input_tokens":1000,"output_tokens":500}`)

	// Today's behaviour, at the production reader: no declared plan budget, no
	// comparable ratio.
	before, err := e.rp.Pressure(ctx, "alice", adapters.LaneZAI)
	if err != nil {
		t.Fatalf("Pressure(before): %v", err)
	}
	if before.Applicable || before.Ratio != 0 {
		t.Fatalf("with nothing declared the reading is %+v, want the honest absence — the denominator is never the provider's allowance (D4)", before)
	}

	const declared = 1400
	e.declarePlan(t, "alice", adapters.LaneZAI, metering.PlanQuotaWindow, declared)

	after, err := e.rp.Pressure(ctx, "alice", adapters.LaneZAI)
	if err != nil {
		t.Fatalf("Pressure(after): %v", err)
	}
	if !after.Applicable || after.Ratio <= 0 {
		t.Fatalf("a declared plan budget did not make the lane comparable: %+v", after)
	}
	if after.Unit != "credits" {
		t.Errorf("unit = %q, want credits — the plan meters in its own unit", after.Unit)
	}

	// The ratio is consumption ÷ the DECLARED budget, and emphatically not
	// consumption ÷ the provider's published allowance (the §63 D1 arithmetic).
	doc, _ := metering.PlanDocFor(adapters.LaneZAI)
	budget, err := e.pb.PlanBudget(ctx, "alice", adapters.LaneZAI, metering.PlanQuotaWindow)
	if err != nil {
		t.Fatalf("PlanBudget: %v", err)
	}
	reading, err := e.g.ReadPlanUnits(ctx, "alice", adapters.LaneZAI, doc, budget, time.Now())
	if err != nil {
		t.Fatalf("ReadPlanUnits: %v", err)
	}
	if want := reading.Consumed / declared; after.Ratio != want {
		t.Errorf("ratio = %v, want %v (consumption ÷ the declared budget)", after.Ratio, want)
	}
	quota, _ := doc.Quota(metering.PlanQuotaWindow)
	if after.Ratio == reading.Consumed/quota.Units {
		t.Error("the ratio divides by the PROVIDER's published allowance — that is the inferred provider window S10.4/D4 bars by name")
	}
	if reading.Tier != metering.TierDerived || !reading.Assumed || reading.AssumedNote == "" {
		t.Errorf("the declared reading is tier %d assumed=%v — declaring a budget does not make a tier-3 proxy measured",
			reading.Tier, reading.Assumed)
	}
}

// ── T6 · the selection proof, at the REAL call site, both directions ────────

// ln6Router composes the production routing inputs: coverage in the shape
// stage.New builds it (the configured lane, then what is commissioned), the
// commissioned lane's own seat from its document, and the PRODUCTION pressure
// reader. No fake stands anywhere in this path — a resolver proven in isolation
// cannot see a dropped argument (§63 drain r2 R1).
func ln6Router(t *testing.T, e *ln6Env) *worker.Router {
	t.Helper()
	lanes := seedLanes(t)
	live := commissionedZAI(t, lanes)
	store, err := worker.NewStore(worker.Config{
		DB: e.db, Log: e.log, Settings: e.reg, Root: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("worker store: %v", err)
	}
	return &worker.Router{
		Store:      store,
		DutyMap:    worker.DefaultDutyMap(),
		Alternates: laneAlternateSeats(lanes, live),
		Coverage: worker.Coverage{
			FlatRateLanes: append([]string{adapters.LaneAnthropic}, commissionedLanes(lanes, live)...),
		},
		Pressure: e.rp,
	}
}

func ln6Route(t *testing.T, r *worker.Router, taskID string) worker.Decision {
	t.Helper()
	d, err := r.Route(context.Background(), worker.RouteQuery{
		Requester: "alice", TaskID: taskID,
		TaskText: "Draft the release notes for this change",
		Family:   "write-produce", Domain: "software",
	})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	return d
}

func TestLN6CommissionedLaneWinsAndLosesOnConsumption(t *testing.T) {
	// The two runs differ in NOTHING but consumption: same budgets, same
	// coverage, same seats, same query.
	//
	// The figures are chosen so the outcome holds under EITHER of the zai
	// plan's charging rates. The reading is taken at wall-clock time and the
	// plan charges 1.0× inside its peak window and 0.5× outside it, so a
	// margin narrower than 2× would make this test pass or fail by the hour.
	// 18 calls put the loaded lane between 45% and 90% of its plan budget and
	// the loaded token lane at 90%; 2 calls put either between 5% and 10%.
	const (
		tokenBudget = 30000
		planBudget  = 20
	)
	for _, tc := range []struct {
		name           string
		anthropicCalls int
		zaiCalls       int
		wantLane       string
	}{
		{"the commissioned lane is the less consumed one", 18, 2, adapters.LaneZAI},
		{"the configured lane is the less consumed one", 2, 18, adapters.LaneAnthropic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newLN6Env(t)
			e.work(t, "r-a", "alice", adapters.LaneAnthropic, adapters.SubstrateClaudeCLI, "claude-x",
				tc.anthropicCalls, `{"input_tokens":1000,"output_tokens":500}`)
			e.work(t, "r-z", "alice", adapters.LaneZAI, adapters.SubstrateOpencode, "glm-x",
				tc.zaiCalls, `{"input_tokens":1000,"output_tokens":500}`)
			e.declareTokens(t, "alice", adapters.LaneAnthropic, tokenBudget)
			e.declarePlan(t, "alice", adapters.LaneZAI, metering.PlanQuotaWindow, planBudget)

			d := ln6Route(t, ln6Router(t, e), "t-"+tc.wantLane)
			if d.Lane != tc.wantLane {
				t.Fatalf("selection seated lane %q, want %q — between covered flat lanes the ordering input is "+
					"CONSUMPTION PRESSURE (S08.8; D5: never dollars).\nreason: %s", d.Lane, tc.wantLane, d.PlainReason)
			}
			// "Chosen among" is emitted ONLY by the branch that actually
			// compared two ratios. Asserting "consumption pressure" alone is
			// not enough: the honest-absence branch says "no comparable
			// consumption pressure", which contains it — so a reverted
			// production reader would leave this direction green while proving
			// the opposite of what it claims.
			if !strings.Contains(d.PlainReason, "Chosen among") || !strings.Contains(d.PlainReason, "by how much of each is left") {
				t.Errorf("the reason does not record that two ratios were compared: %q", d.PlainReason)
			}
			if !strings.Contains(d.PlainReason, tc.wantLane) {
				t.Errorf("the reason does not name the lane it chose: %q", d.PlainReason)
			}
			if d.Model == "" || d.WindowTokens <= 0 {
				t.Errorf("the seated decision carries no model or window: %+v", d)
			}
		})
	}
}

// ── T8 · with no plan budget the deterministic order still stands ───────────

func TestLN6DeterministicOrderStandsWithNoPlanBudget(t *testing.T) {
	e := newLN6Env(t)
	e.work(t, "r-a", "alice", adapters.LaneAnthropic, adapters.SubstrateClaudeCLI, "claude-x", 2,
		`{"input_tokens":1000,"output_tokens":500}`)
	e.work(t, "r-z", "alice", adapters.LaneZAI, adapters.SubstrateOpencode, "glm-x", 18,
		`{"input_tokens":1000,"output_tokens":500}`)
	// The anthropic lane IS comparable, so the lane that cannot be compared is
	// the plan-documented one — which is the case this pins.
	e.declareTokens(t, "alice", adapters.LaneAnthropic, 20000)

	d := ln6Route(t, ln6Router(t, e), "t-undeclared")
	if d.Lane != adapters.LaneAnthropic {
		t.Fatalf("selection seated %q with no comparable reading on the second lane, want the deterministic "+
			"duty-map order.\nreason: %s", d.Lane, d.PlainReason)
	}
	for _, want := range []string{"no declared automation budget", "never about money", adapters.LaneZAI} {
		if !strings.Contains(d.PlainReason, want) {
			t.Errorf("the reason does not carry %q: %q", want, d.PlainReason)
		}
	}
}

// ── T7 · the honest absence, as a PROPERTY ─────────────────────────────────

// TestLN6NoRowIsIdenticalToTodayAcrossArbitraryHistories asserts the invariant
// S10.4 states rather than one case of it: until somebody declares a row, this
// packet is INVISIBLE. Over arbitrary consumption histories on both plan lanes,
// the reading the new consumption path produces must equal, field for field,
// the reading the pre-packet hardcode produced — and the routing pressure with
// it.
func TestLN6NoRowIsIdenticalToTodayAcrossArbitraryHistories(t *testing.T) {
	ctx := context.Background()
	e := newLN6Env(t)
	rng := rand.New(rand.NewSource(0x1e6))
	lanes := []struct{ lane, substrate string }{
		{adapters.LaneZAI, adapters.SubstrateOpencode},
		{"kimi", adapters.SubstrateOpencode},
	}
	// Instants inside and outside every declared window and both multiplier
	// windows: the zai plan charges 1.0× Mon–Fri 14:00–18:00 UTC+8 and 0.5×
	// otherwise, so a draw that never left one window would prove nothing about
	// the other.
	base := time.Date(2026, 8, 19, 7, 0, 0, 0, time.UTC)

	const draws = 200
	for i := 0; i < draws; i++ {
		owner := fmt.Sprintf("p%03d", i)
		if err := e.db.WriteTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO users (user_id, display_name, role, created_ts) VALUES (?, ?, 'member', ?)`,
				owner, owner, base.Format(time.RFC3339Nano))
			return err
		}); err != nil {
			t.Fatalf("seed %s: %v", owner, err)
		}
		pick := lanes[rng.Intn(len(lanes))]
		calls := rng.Intn(51)
		at := base.Add(time.Duration(rng.Intn(400)-200) * time.Hour)
		now := at.Add(time.Duration(rng.Intn(240)) * time.Hour)
		if calls > 0 {
			e.work(t, "r-"+owner, owner, pick.lane, pick.substrate, "m-"+owner, calls,
				`{"input_tokens":1000,"output_tokens":500,"cache_read_input_tokens":4000}`)
			if err := e.db.WriteTx(ctx, func(tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, `UPDATE checkpoints SET created_ts = ? WHERE run_id = ?`,
					at.Format(time.RFC3339Nano), "r-"+owner)
				return err
			}); err != nil {
				t.Fatalf("date the checkpoints of %s: %v", owner, err)
			}
		}

		doc, ok := metering.PlanDocFor(pick.lane)
		if !ok {
			t.Fatalf("lane %s carries no plan document", pick.lane)
		}
		// The pre-packet reading: the hardcode this packet replaced.
		want, err := e.g.ReadPlanUnits(ctx, owner, pick.lane, doc, metering.UndeclaredPlanBudget(), now)
		if err != nil {
			t.Fatalf("draw %d: pre-packet ReadPlanUnits: %v", i, err)
		}
		got, _, err := bindingPlanReading(ctx, e.g, e.pb, owner, pick.lane, doc, now)
		if err != nil {
			t.Fatalf("draw %d: bindingPlanReading: %v", i, err)
		}
		if got.Applicable || got.Pressure != 0 || got.BackgroundCeiling != 0 {
			t.Fatalf("draw %d (%s, %d calls): with no declared row the reading claims a denominator: %+v",
				i, pick.lane, calls, got)
		}
		// The comparison covers the SEED provenance and the Budget value too
		// (drain r1 D8): a fabricated SeededFrom on an undeclared lane changes
		// SeedQuota/SeedAllowance/SeedWindowHours without touching Applicable
		// or Pressure, so a narrower comparison let that mutation live.
		if got.SeedQuota != want.SeedQuota || got.SeedAllowance != want.SeedAllowance ||
			got.SeedWindowHours != want.SeedWindowHours || got.Budget != want.Budget ||
			got.InapplicableNote != want.InapplicableNote {
			t.Fatalf("draw %d (%s, %d calls): the seed provenance or the budget moved with no row declared\n got %+v\nwant %+v",
				i, pick.lane, calls, got, want)
		}
		if got.Unit != want.Unit || got.Calls != want.Calls || got.Consumed != want.Consumed ||
			got.Tier != want.Tier || got.Assumed != want.Assumed || got.AssumedNote != want.AssumedNote ||
			got.Multiplier != want.Multiplier || got.MultiplierWindow != want.MultiplierWindow ||
			len(got.Windows) != len(want.Windows) {
			t.Fatalf("draw %d (%s, %d calls at %s, read at %s): the reading moved with no row declared\n got %+v\nwant %+v",
				i, pick.lane, calls, at, now, got, want)
		}
		for w := range got.Windows {
			if got.Windows[w] != want.Windows[w] {
				t.Fatalf("draw %d: window %d moved: got %+v want %+v", i, w, got.Windows[w], want.Windows[w])
			}
		}
		// And the routing figure the S08.8 rule reads.
		p, err := e.rp.Pressure(ctx, owner, pick.lane)
		if err != nil {
			t.Fatalf("draw %d: Pressure: %v", i, err)
		}
		if p.Applicable || p.Ratio != 0 || p.Unit != want.Unit {
			t.Fatalf("draw %d: routing pressure with no declared row = %+v, want the honest absence in %q", i, p, want.Unit)
		}
	}
}

// ── T12 · the meters read agrees with the router beside it ─────────────────

func TestLN6MetersReadAgreesWithTheRouter(t *testing.T) {
	ctx := context.Background()
	e := newLN6Env(t)
	e.work(t, "r-z", "alice", adapters.LaneZAI, adapters.SubstrateOpencode, "glm-x", 6,
		`{"input_tokens":1000,"output_tokens":500}`)

	agree := func(t *testing.T, state string) {
		t.Helper()
		p, err := e.rp.Pressure(ctx, "alice", adapters.LaneZAI)
		if err != nil {
			t.Fatalf("%s: Pressure: %v", state, err)
		}
		lm, err := e.meter.LaneMeter(ctx, "alice", adapters.LaneZAI)
		if err != nil {
			t.Fatalf("%s: LaneMeter: %v", state, err)
		}
		if lm.Plan == nil {
			t.Fatalf("%s: the meters read serves no plan block for a plan-documented lane", state)
		}
		switch {
		case p.Applicable && lm.Plan.Pressure == nil:
			t.Fatalf("%s: the router compares a ratio the meters read does not serve — that is the drain-D2 self-contradiction", state)
		case !p.Applicable && lm.Plan.Pressure != nil:
			t.Fatalf("%s: the meters read serves a pressure the router does not have", state)
		}
		if p.Applicable && *lm.Plan.Pressure != p.Ratio {
			t.Errorf("%s: meters pressure %v ≠ router ratio %v for the same (person, lane)", state, *lm.Plan.Pressure, p.Ratio)
		}
		// THE COHERENT TRIPLE (drain r2 R2/R3). A DECLARED budget serves either
		// a pressure or a reason it produced none — never neither, which is
		// three absences contradicting each other, and never a budget_declared
		// bit with nothing behind it.
		if lm.Plan.BudgetDeclared {
			if lm.Plan.Pressure == nil && lm.Plan.InapplicableNote == "" {
				t.Errorf("%s: the meters read says a budget is declared and serves neither a pressure nor a reason — "+
					"a refusal nobody can see is how it comes back", state)
			}
			if lm.Plan.Pressure != nil && lm.Plan.InapplicableNote != "" {
				t.Errorf("%s: the meters read serves a pressure AND a reason it has none: %q", state, lm.Plan.InapplicableNote)
			}
			if lm.Plan.Budget == nil {
				t.Errorf("%s: a declared budget is reported with no declaration behind it", state)
			}
		}
		if !lm.Plan.BudgetDeclared && lm.Plan.Budget != nil {
			t.Errorf("%s: a budget object is served for a lane reporting nothing declared: %+v", state, lm.Plan.Budget)
		}
		// The router's own reason carries the same sentence, so a person
		// reading a routing decision is not told a budget was never declared
		// when one was and expired.
		if !p.Applicable && p.Reason != lm.Plan.InapplicableNote {
			t.Errorf("%s: the router's reason %q and the meters note %q disagree", state, p.Reason, lm.Plan.InapplicableNote)
		}
	}

	agree(t, "undeclared")

	e.declarePlan(t, "alice", adapters.LaneZAI, metering.PlanQuotaWindow, 1400)
	agree(t, "declared")

	// THE EXPIRED STATE, which the invariant above did not reach before this
	// round: only undeclared and fresh were exercised, so a declared-but-
	// refused row could serve budget_declared:true beside nothing at all and
	// the suite stayed green (drain r2 R2/R3).
	expirePlanBudget(t, e, "alice", adapters.LaneZAI, metering.PlanQuotaWindow)
	agree(t, "expired")

	expired, err := e.meter.LaneMeter(ctx, "alice", adapters.LaneZAI)
	if err != nil {
		t.Fatalf("LaneMeter(expired): %v", err)
	}
	switch {
	case expired.Plan.Pressure != nil:
		t.Errorf("an elapsed period still serves a pressure: %v", *expired.Plan.Pressure)
	case expired.Plan.Budget == nil:
		t.Error("an elapsed period hides the declaration it refused — the row still exists and a reader must see it")
	case !strings.Contains(expired.Plan.InapplicableNote, "declaring again is what starts the next period"):
		t.Errorf("the expired note does not say what re-opens it: %q", expired.Plan.InapplicableNote)
	}
	if p, perr := e.rp.Pressure(ctx, "alice", adapters.LaneZAI); perr != nil {
		t.Fatalf("Pressure(expired): %v", perr)
	} else if p.Applicable || p.Reason == "" {
		t.Errorf("the router's expired reading = %+v, want inapplicable WITH its reason", p)
	}

	// The declaration itself rides the read, so a person can see the
	// denominator their pressure was measured against.
	lm, err := e.meter.LaneMeter(ctx, "alice", adapters.LaneZAI)
	if err != nil {
		t.Fatalf("LaneMeter: %v", err)
	}
	if lm.Plan.Budget == nil {
		t.Fatal("the declared plan budget does not ride the meters read — a pressure whose denominator is invisible cannot be checked")
	}
	if lm.Plan.Budget.PeriodUnits != 1400 || lm.Plan.Budget.Unit != "credits" ||
		lm.Plan.Budget.Window != metering.PlanQuotaWindow {
		t.Errorf("the served budget = %+v, want 1400 credits on the %s window", lm.Plan.Budget, metering.PlanQuotaWindow)
	}
	if lm.Plan.Budget.Source != metering.PlanBudgetOperatorSet || lm.Plan.Budget.DeclaredBy != "alice" {
		t.Errorf("the served budget carries no provenance: %+v", lm.Plan.Budget)
	}
}

// ── T6/T12 · the most-constrained window binds (OQ-2, ratified) ─────────────

// TestLN6MostConstrainedWindowBindsTheLane pins the aggregation rule on its own,
// so a coordinator reversal touches this function and its one production
// counterpart and nothing else. A lane sitting at 90% of its five-hour window
// has no headroom whatever its weekly window says, and admitting background work
// against the fresher number is the overrun S10.4's headroom rule exists to
// prevent.
func TestLN6MostConstrainedWindowBindsTheLane(t *testing.T) {
	ctx := context.Background()
	e := newLN6Env(t)
	e.work(t, "r-z", "alice", adapters.LaneZAI, adapters.SubstrateOpencode, "glm-x", 10,
		`{"input_tokens":1000,"output_tokens":500}`)

	// The weekly window is generous; the rolling window is nearly spent.
	e.declarePlan(t, "alice", adapters.LaneZAI, metering.PlanQuotaWeekly, 100000)
	loose, err := e.rp.Pressure(ctx, "alice", adapters.LaneZAI)
	if err != nil {
		t.Fatalf("Pressure(weekly only): %v", err)
	}
	if !loose.Applicable {
		t.Fatal("a declared weekly budget did not make the lane comparable")
	}

	e.declarePlan(t, "alice", adapters.LaneZAI, metering.PlanQuotaWindow, 6)
	bound, err := e.rp.Pressure(ctx, "alice", adapters.LaneZAI)
	if err != nil {
		t.Fatalf("Pressure(both windows): %v", err)
	}
	if bound.Ratio <= loose.Ratio {
		t.Fatalf("with a nearly-spent five-hour window the lane reports %v, no worse than the weekly-only %v — "+
			"the MOST CONSTRAINED window binds, or an exhausted window hides behind a fresh one", bound.Ratio, loose.Ratio)
	}
	lm, err := e.meter.LaneMeter(ctx, "alice", adapters.LaneZAI)
	if err != nil {
		t.Fatalf("LaneMeter: %v", err)
	}
	if lm.Plan.Budget == nil || lm.Plan.Budget.Window != metering.PlanQuotaWindow {
		t.Errorf("the meters read names %+v as the binding window, want %s", lm.Plan.Budget, metering.PlanQuotaWindow)
	}
}

// ── T11 (shell half) · the proposal has a production caller ────────────────

// TestLN6ProposePlanBudgetHasANonTestCaller closes the "wired and read by
// nothing" class (§63 D2) by OBSERVATION rather than by claim: the ratified
// LN-2B proposal existed and nothing in the tree called it, which is exactly
// what made a commissioned lane unable to win selection.
func TestLN6ProposePlanBudgetHasANonTestCaller(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	callers := 0
	scanned := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(body), ".ProposePlanBudget(") {
			callers++
		}
	}
	if scanned == 0 {
		t.Fatal("the scan read no files — it would pass vacuously")
	}
	if callers == 0 {
		t.Error("no non-test source in internal/shell calls ProposePlanBudget.\n" +
			"The S10.4 proposal is the only sanctioned use of the provider's published allowance, and a proposal " +
			"nothing can reach is a budget an operator can never seed (P3-LN-6 R5; §63 D2).")
	}
}

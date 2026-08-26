package metering

// pool_ln7_test.go — P3-LN-7 §10 specs T21, T22, T23 (S10.1, S10.4, §64).
//
// ONE POOL, DECLARED ONCE. The operator's whole reason for the second kimi lane
// is a comparison — land comparable work on `kimi` and `kimi-cli`, read the
// answer off the Lane column. That reading is only honest if one membership
// never appears as two allowances.
//
// The pool is DATA on the plan document: this package's own no-lane-constants
// scan bans the literal "kimi" from every non-test source here, which is
// exactly the discipline that keeps a pool a row rather than a branch.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ── T21 · two lanes, ONE document ────────────────────────────────────────────

func TestPlanDocResolvesBothKimiLanes(t *testing.T) {
	docs, err := SeedPlanDocs()
	if err != nil {
		t.Fatalf("SeedPlanDocs: %v", err)
	}
	// A second document would BE the second allowance. The count is the
	// assertion, not a detail.
	if len(docs) != 2 {
		t.Fatalf("SeedPlanDocs returned %d documents, want 2 — a pooled lane gets a POOL MEMBERSHIP, never a document of its own", len(docs))
	}

	api, ok := PlanDocFor("kimi")
	if !ok {
		t.Fatal("no plan document for lane kimi")
	}
	cli, ok := PlanDocFor("kimi-cli")
	if !ok {
		t.Fatal("PlanDocFor(\"kimi-cli\") found nothing — the pooled lane must resolve to the pool's document")
	}
	if api.Lane != cli.Lane || api.Plan != cli.Plan {
		t.Errorf("the two kimi lanes resolved to different documents (%q/%q vs %q/%q)", api.Lane, api.Plan, cli.Lane, cli.Plan)
	}
	if api.Pool == "" {
		t.Error("the kimi plan document declares no pool")
	}
	if !api.CountsLane("kimi") || !api.CountsLane("kimi-cli") {
		t.Errorf("the pool does not count both lanes: kimi=%v kimi-cli=%v", api.CountsLane("kimi"), api.CountsLane("kimi-cli"))
	}
	// No document may declare an allowance for the pooled lane on its own.
	for _, d := range docs {
		if d.Lane == "kimi-cli" {
			t.Errorf("a plan document declares lane kimi-cli independently — its allowance IS the pool's")
		}
	}
	// The zai lane is expressible UNCHANGED: absent members mean the
	// pre-packet behaviour exactly (a document with no pool serves its own
	// lane alone) — the PlanQuota.Unit precedent.
	zai, ok := PlanDocFor("zai")
	if !ok {
		t.Fatal("no plan document for lane zai")
	}
	if zai.Pool != "" {
		t.Errorf("the zai document grew a pool %q — it has no sibling lane and must be untouched", zai.Pool)
	}
	if !zai.CountsLane("zai") {
		t.Error("an unpooled document stopped counting its own lane")
	}
	if zai.CountsLane("kimi") || zai.CountsLane("kimi-cli") {
		t.Error("an unpooled document counts a lane that is not its own")
	}
	// And the assumed note now covers three consumers, not two.
	if !strings.Contains(api.AssumedNote, "LOWER BOUND") {
		t.Errorf("the pooled document's assumed_note dropped the lower-bound statement: %q", api.AssumedNote)
	}
}

// ── T22 · pooled consumption is SUMMED, never split ──────────────────────────

// TestPooledConsumptionIsSummedNotSplit is the assertion that stops routing
// steering work onto an allowance that is already spent. Without it each lane
// reports a half-empty pool.
func TestPooledConsumptionIsSummedNotSplit(t *testing.T) {
	env := newPoolEnv(t)
	doc, _ := PlanDocFor("kimi")

	// Six calls on the API lane, four on the CLI lane, one on zai — one
	// person, one membership.
	env.checkpoint(t, "kimi", 6)
	env.checkpoint(t, "kimi-cli", 4)
	env.checkpoint(t, "zai", 1)

	for _, lane := range []string{"kimi", "kimi-cli"} {
		calls, consumed, err := env.g.planUnits(context.Background(), env.user, lane, doc, time.Time{}, false, env.now)
		if err != nil {
			t.Fatalf("planUnits(%s): %v", lane, err)
		}
		if calls != 10 {
			t.Errorf("lane %s counted %d calls, want 10 — the reading for EITHER lane must report the pool's combined "+
				"consumption, or each lane reports a half-empty allowance and routing steers onto a spent one", lane, calls)
		}
		if consumed != 10*doc.StandardMultiplier() {
			t.Errorf("lane %s consumed %v, want %v", lane, consumed, 10*doc.StandardMultiplier())
		}
	}

	// The zai lane is untouched: its reading counts its own single call.
	zdoc, _ := PlanDocFor("zai")
	calls, _, err := env.g.planUnits(context.Background(), env.user, "zai", zdoc, time.Time{}, false, env.now)
	if err != nil {
		t.Fatalf("planUnits(zai): %v", err)
	}
	if calls != 1 {
		t.Errorf("the zai lane counted %d calls, want 1 — an unpooled lane must be byte-identical to before", calls)
	}
}

// TestPooledReadingNamesThePoolAndItsLanes: a combined figure that does not say
// what it combined invites exactly the wrong reading — somebody sees a lane's
// gauge at 80% and looks for 80% of its own traffic.
func TestPooledReadingNamesThePoolAndItsLanes(t *testing.T) {
	env := newPoolEnv(t)
	doc, _ := PlanDocFor("kimi")
	env.checkpoint(t, "kimi-cli", 2)

	r, err := env.g.ReadPlanUnits(context.Background(), env.user, "kimi-cli", doc, UndeclaredPlanBudget(), env.now)
	if err != nil {
		t.Fatalf("ReadPlanUnits: %v", err)
	}
	if r.Lane != "kimi-cli" {
		t.Errorf("the reading's lane = %q, want the REQUESTING lane %q", r.Lane, "kimi-cli")
	}
	if r.Pool == "" {
		t.Error("the pooled reading does not name its pool")
	}
	if len(r.PoolLanes) < 2 {
		t.Errorf("the pooled reading names lanes %v, want both members", r.PoolLanes)
	}
}

// ── T23 · a pool budget cannot be declared twice ─────────────────────────────

// TestPoolBudgetDoubleDeclarationRefused implements OQ-2's ratified answer:
// refuse the second declaration, NAMING the pool's canonical lane. plan_budgets
// is keyed (user_id, lane, window), so without this an operator declares
// against one allowance twice and the two rows disagree about one pool.
//
// Enforced at the THREE layers LN-6 established, because a rule spelled once is
// a rule that can be walked around.
func TestPoolBudgetDoubleDeclarationRefused(t *testing.T) {
	doc, _ := PlanDocFor("kimi")
	if doc.Lane != "kimi" {
		t.Fatalf("the pool's canonical lane is %q; this test assumes the document's own lane is canonical", doc.Lane)
	}

	// Layer 1 — the predicate itself, computed once and carried across seams.
	if refusal := PlanPoolRefusal(doc, "kimi"); refusal != "" {
		t.Errorf("the canonical lane was refused: %s", refusal)
	}
	refusal := PlanPoolRefusal(doc, "kimi-cli")
	if refusal == "" {
		t.Fatal("declaring a budget on the non-canonical pooled lane was allowed — one allowance would carry two rows")
	}
	if !strings.Contains(refusal, "kimi") {
		t.Errorf("the refusal does not name the pool's canonical lane: %q", refusal)
	}
	if !strings.Contains(refusal, doc.Pool) {
		t.Errorf("the refusal does not name the pool: %q", refusal)
	}
	// An unpooled lane is never refused — the rule must be inert where there
	// is no pool.
	zai, _ := PlanDocFor("zai")
	if got := PlanPoolRefusal(zai, "zai"); got != "" {
		t.Errorf("an unpooled lane was refused: %q", got)
	}

	// Layer 2 — the store refuses the write.
	env := newPoolEnv(t)
	row := PlanBudgetRow{
		UserID: env.user, Lane: "kimi", Window: PlanQuotaWindow,
		PeriodUnits: 100, Unit: doc.QuotaUnit(PlanQuotaWindow),
		PeriodStart: env.now, PeriodHours: 5,
		Source: PlanBudgetOperatorSet, DeclaredTS: env.now, DeclaredBy: env.user,
	}
	if _, _, err := env.pb.Declare(context.Background(), row); err != nil {
		t.Fatalf("declaring on the canonical lane failed: %v", err)
	}
	row.Lane = "kimi-cli"
	if _, _, err := env.pb.Declare(context.Background(), row); err == nil {
		t.Error("the store accepted a second declaration on the pooled sibling lane")
	} else if !strings.Contains(err.Error(), "kimi") {
		t.Errorf("the store's refusal does not name the canonical lane: %v", err)
	}

	// Layer 3 — a row planted by any other route does not steer dispatch.
	planted := row.Budget()
	planted.Declared = true
	r, err := env.g.ReadPlanUnits(context.Background(), env.user, "kimi-cli", doc, planted, env.now)
	if err != nil {
		t.Fatalf("ReadPlanUnits: %v", err)
	}
	if r.Applicable {
		t.Error("a planted budget row on the pooled sibling lane was APPLIED — a rule enforced only at the write is " +
			"a rule any other route walks around")
	}
	if r.InapplicableNote == "" {
		t.Error("the refused reading carries no reason — a refusal nobody can see is not a refusal")
	}
	if r.Pressure != 0 {
		t.Errorf("the refused reading still produced pressure %v", r.Pressure)
	}
}

// ── the pool environment ─────────────────────────────────────────────────────

type poolEnv struct {
	*env
	g    *PressureGauge
	pb   *PlanBudgets
	user string
	now  time.Time
	seq  int
}

func newPoolEnv(t *testing.T) *poolEnv {
	t.Helper()
	base := newEnv(t)
	e := &poolEnv{
		env:  base,
		g:    NewPressureGauge(base.db, base.reg),
		pb:   NewPlanBudgets(base.db),
		user: "pooluser",
		// The reading bounds consumption at `now`, and checkpoints are written
		// with the real clock, so a fixed literal in the past would silently
		// count nothing and every assertion below would pass vacuously.
		now: time.Now().UTC().Add(time.Hour),
	}
	if err := base.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO users (user_id, role, created_ts) VALUES (?, 'operator', ?)`,
			e.user, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("seed the pool's person: %v", err)
	}
	return e
}

// checkpoint records n paid calls on a lane for the pool's person. Each call
// gets its own run, because a run carries exactly one lane.
func (e *poolEnv) checkpoint(t *testing.T, lane string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		e.seq++
		id := fmt.Sprintf("pool-%s-%d", lane, e.seq)
		e.runningRun(t, id, e.user, lane, "claude-cli")
		e.checkpointOn(t, id)
		_ = i
	}
}

func (e *poolEnv) checkpointOn(t *testing.T, runID string) {
	t.Helper()
	e.env.checkpoint(t, runID, "k3", `{"input_tokens":10,"output_tokens":5}`)
}

var _ = json.Marshal

// ── drain r1 F3 · the pooled denominator actually pools ─────────────────────

// TestPooledBudgetIsReadThroughThePool is the finding the first cut missed
// entirely, and it made the whole pool half-built: consumption was summed
// across the pool, but the BUDGET — the denominator — was looked up under the
// requesting lane. With the budget declared once on `kimi` (which is what OQ-2
// requires), `kimi-cli` read declared=false and pressure 0 while spending the
// same allowance. Routing therefore saw free headroom on a lane whose pool was
// already committed, which is the exact failure the pool exists to prevent.
func TestPooledBudgetIsReadThroughThePool(t *testing.T) {
	env := newPoolEnv(t)
	doc, _ := PlanDocFor("kimi")

	// The canonical-lane mapping every consumer resolves through.
	if got := PoolBudgetLane("kimi-cli"); got != "kimi" {
		t.Errorf("PoolBudgetLane(kimi-cli) = %q, want the pool's canonical lane %q", got, "kimi")
	}
	if got := PoolBudgetLane("zai"); got != "zai" {
		t.Errorf("PoolBudgetLane(zai) = %q — an unpooled lane is its own", got)
	}

	// Declared ONCE, on the canonical lane, exactly as OQ-2 requires.
	if _, _, err := env.pb.Declare(context.Background(), PlanBudgetRow{
		UserID: env.user, Lane: "kimi", Window: PlanQuotaWindow,
		PeriodUnits: 100, Unit: doc.QuotaUnit(PlanQuotaWindow),
		PeriodStart: env.now.Add(-time.Hour), PeriodHours: 5,
		Source: PlanBudgetOperatorSet, DeclaredTS: env.now, DeclaredBy: env.user,
	}); err != nil {
		t.Fatalf("declare on the canonical lane: %v", err)
	}
	// …and spent through the SIBLING.
	env.checkpoint(t, "kimi-cli", 4)

	for _, lane := range []string{"kimi", "kimi-cli"} {
		budget, err := env.pb.PlanBudget(context.Background(), env.user, lane, PlanQuotaWindow)
		if err != nil {
			t.Fatalf("PlanBudget(%s): %v", lane, err)
		}
		if !budget.Declared {
			t.Errorf("lane %s reads NO declared budget — the allowance is declared once for the pool, so a sibling "+
				"that cannot see it reports free headroom on a pool that is already committed", lane)
			continue
		}
		if budget.Lane != "kimi" {
			t.Errorf("lane %s resolved a budget declared on %q, want the canonical %q", lane, budget.Lane, "kimi")
		}
		r, err := env.g.ReadPlanUnits(context.Background(), env.user, lane, doc, budget, env.now)
		if err != nil {
			t.Fatalf("ReadPlanUnits(%s): %v", lane, err)
		}
		if !r.Applicable {
			t.Errorf("lane %s: the pooled budget did not apply (%s)", lane, r.InapplicableNote)
			continue
		}
		if r.Pressure <= 0 {
			t.Errorf("lane %s: pressure = %v, want the pool's spend against the pool's budget", lane, r.Pressure)
		}
	}

	// Both lanes must read the SAME pressure: it is one allowance and one spend.
	var seen []float64
	for _, lane := range []string{"kimi", "kimi-cli"} {
		b, _ := env.pb.PlanBudget(context.Background(), env.user, lane, PlanQuotaWindow)
		r, _ := env.g.ReadPlanUnits(context.Background(), env.user, lane, doc, b, env.now)
		seen = append(seen, r.Pressure)
	}
	if seen[0] != seen[1] {
		t.Errorf("kimi reads pressure %v and kimi-cli reads %v — one pool, one number", seen[0], seen[1])
	}

	// The OTHER direction is unchanged: declaring on the sibling is still
	// refused, so the pooled read above cannot be mistaken for permission to
	// declare twice.
	if _, _, err := env.pb.Declare(context.Background(), PlanBudgetRow{
		UserID: env.user, Lane: "kimi-cli", Window: PlanQuotaWindow,
		PeriodUnits: 50, Unit: doc.QuotaUnit(PlanQuotaWindow),
		PeriodStart: env.now, PeriodHours: 5,
		Source: PlanBudgetOperatorSet, DeclaredTS: env.now, DeclaredBy: env.user,
	}); err == nil {
		t.Error("declaring on the pooled sibling was accepted — reading through the pool must not open writing to it")
	}

	// And an unpooled lane is untouched: no budget declared, none read.
	zdoc, _ := PlanDocFor("zai")
	zb, err := env.pb.PlanBudget(context.Background(), env.user, "zai", zdoc.Quotas[0].Name)
	if err != nil {
		t.Fatalf("PlanBudget(zai): %v", err)
	}
	if zb.Declared {
		t.Error("the zai lane picked up a budget it never declared — pool resolution leaked to an unpooled lane")
	}
}

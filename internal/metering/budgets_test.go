package metering

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

// budgets_test.go — the S10.4 durable operator switches (P3-B6-2B, R11/R12).

func seedUser(t *testing.T, e *env, id string) {
	t.Helper()
	if err := e.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO users (user_id, display_name, role, created_ts) VALUES (?, ?, 'member', ?)`,
			id, id, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

// TestBudgetRoundTripsAndReturnsWhatItReplaced: the store persists a
// declaration, reads it back as the value object the gauge takes, and hands the
// PRIOR row back so the edit can be audited old→new.
func TestBudgetRoundTripsAndReturnsWhatItReplaced(t *testing.T) {
	e := newEnv(t)
	seedUser(t, e, "alice")
	seedUser(t, e, "bob")
	b := NewBudgets(e.db)
	ctx := context.Background()
	start := time.Now().UTC().Truncate(time.Second)

	// Undeclared is an ABSENCE, not an error and not a zero.
	if _, ok, err := b.Row(ctx, "alice", "anthropic"); err != nil || ok {
		t.Fatalf("Row on an undeclared budget = (ok %v, err %v), want (false, nil)", ok, err)
	}
	got, err := b.Budget(ctx, "alice", "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if got.Declared || got.PeriodTokens != 0 {
		t.Fatalf("undeclared budget = %+v, want the undeclared posture", got)
	}

	first := BudgetRow{UserID: "alice", Lane: "anthropic", PeriodTokens: 1000,
		PeriodStart: start, PeriodDays: 30, DeclaredTS: start, DeclaredBy: "alice"}
	if _, existed, err := b.Declare(ctx, first); err != nil || existed {
		t.Fatalf("first Declare = (existed %v, err %v), want (false, nil)", existed, err)
	}
	got, err = b.Budget(ctx, "alice", "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Declared || got.PeriodTokens != 1000 || !got.PeriodStart.Equal(start) {
		t.Fatalf("declared budget = %+v, want 1000 units from %s", got, start)
	}

	// A re-declaration REPLACES and reports what it replaced.
	second := first
	second.PeriodTokens, second.DeclaredBy = 2500, "op"
	prior, existed, err := b.Declare(ctx, second)
	if err != nil || !existed {
		t.Fatalf("re-Declare = (existed %v, err %v), want (true, nil)", existed, err)
	}
	if prior.PeriodTokens != 1000 || prior.DeclaredBy != "alice" {
		t.Fatalf("prior = %+v, want the row that was replaced (1000, alice) — the old→new audit needs it", prior)
	}
	got, _ = b.Budget(ctx, "alice", "anthropic")
	if got.PeriodTokens != 2500 {
		t.Fatalf("after re-declaration the budget is %d, want 2500", got.PeriodTokens)
	}
	// Per (person, lane): another lane is a different budget, and another person
	// is not touched at all.
	if got, _ := b.Budget(ctx, "alice", "local"); got.Declared {
		t.Error("declaring the anthropic budget also declared the local one — the grain is (person, lane), D4")
	}
	if got, _ := b.Budget(ctx, "bob", "anthropic"); got.Declared {
		t.Error("alice's budget leaked onto bob")
	}
}

// TestBudgetRefusesWhatIsNotABudget: the store refuses the values the schema
// refuses, so a defect never becomes a row.
func TestBudgetRefusesWhatIsNotABudget(t *testing.T) {
	e := newEnv(t)
	seedUser(t, e, "alice")
	b := NewBudgets(e.db)
	now := time.Now().UTC()
	base := BudgetRow{UserID: "alice", Lane: "anthropic", PeriodTokens: 10, PeriodDays: 7,
		PeriodStart: now, DeclaredTS: now, DeclaredBy: "alice"}
	for _, tc := range []struct {
		name string
		mut  func(r *BudgetRow)
	}{
		{"zero budget", func(r *BudgetRow) { r.PeriodTokens = 0 }},
		{"negative budget", func(r *BudgetRow) { r.PeriodTokens = -1 }},
		{"no period length", func(r *BudgetRow) { r.PeriodDays = 0 }},
		{"no lane", func(r *BudgetRow) { r.Lane = "" }},
		{"no person", func(r *BudgetRow) { r.UserID = "" }},
		{"no declaring actor", func(r *BudgetRow) { r.DeclaredBy = "" }},
	} {
		row := base
		tc.mut(&row)
		if _, _, err := b.Declare(context.Background(), row); err == nil {
			t.Errorf("%s: Declare accepted %+v", tc.name, row)
		}
	}
}

// TestDeclaredBudgetMakesTheGaugeApplicable is the pre/post-declaration pressure
// test at the gauge itself: the SAME consumption reads not-applicable with no
// budget and reports a real Pressure once one is declared. The gauge's own code
// is untouched — only where its Budget comes from changed.
func TestDeclaredBudgetMakesTheGaugeApplicable(t *testing.T) {
	e := newEnv(t)
	seedUser(t, e, "alice")
	ctx := context.Background()
	e.runningRun(t, "r1", "alice", "anthropic", "claude-cli")
	e.checkpoint(t, "r1", "claude-sonnet-4-5", `{"input_tokens":900,"output_tokens":100}`)

	b := NewBudgets(e.db)
	g := NewPressureGauge(e.db, e.reg)

	pre, err := g.Read(ctx, "alice", "anthropic", mustBudget(t, b, "alice", "anthropic"))
	if err != nil {
		t.Fatal(err)
	}
	if pre.Applicable {
		t.Fatal("pressure is applicable with no declared budget — a fabricated denominator is worse than no number (D4)")
	}
	if pre.WeightedConsumption != 1000 {
		t.Fatalf("weighted consumption = %v, want 1000 (it is always real, budget or no budget)", pre.WeightedConsumption)
	}

	if _, _, err := b.Declare(ctx, BudgetRow{UserID: "alice", Lane: "anthropic", PeriodTokens: 2000,
		PeriodStart: time.Now().Add(-time.Hour).UTC(), PeriodDays: 30,
		DeclaredTS: time.Now().UTC(), DeclaredBy: "alice"}); err != nil {
		t.Fatal(err)
	}
	post, err := g.Read(ctx, "alice", "anthropic", mustBudget(t, b, "alice", "anthropic"))
	if err != nil {
		t.Fatal(err)
	}
	if !post.Applicable {
		t.Fatal("pressure is not applicable with a declared budget — the declaration binds immediately (S10.4)")
	}
	if post.Pressure != 0.5 {
		t.Fatalf("pressure = %v, want 0.5 (1000 weighted units against a 2000-unit budget)", post.Pressure)
	}
	if post.BackgroundBudget != 1000 {
		t.Fatalf("background budget = %v, want 1000 (⚙ budget.background_window_fraction of the declared budget)", post.BackgroundBudget)
	}
}

func mustBudget(t *testing.T, b *Budgets, user, lane string) Budget {
	t.Helper()
	got, err := b.Budget(context.Background(), user, lane)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// ── the pause switch ────────────────────────────────────────────────────────

func TestPauseFlipsAndReportsItsPriorValue(t *testing.T) {
	e := newEnv(t)
	seedUser(t, e, "alice")
	seedUser(t, e, "bob")
	p := NewPause(e.db)
	ctx := context.Background()

	// Nobody starts paused: the column defaults to 0 and no migration turns it
	// on for anyone.
	for _, u := range []string{"alice", "bob"} {
		if paused, err := p.Paused(ctx, u); err != nil || paused {
			t.Fatalf("%s starts paused=%v (err %v), want false", u, paused, err)
		}
	}
	prior, err := p.SetPause(ctx, "alice", true)
	if err != nil || prior {
		t.Fatalf("SetPause = (prior %v, err %v), want (false, nil)", prior, err)
	}
	if paused, _ := p.Paused(ctx, "alice"); !paused {
		t.Error("alice is not paused after pausing")
	}
	if paused, _ := p.Paused(ctx, "bob"); paused {
		t.Error("pausing alice paused bob — the switch is per person")
	}
	// A repeat is honest about having changed nothing.
	if prior, _ := p.SetPause(ctx, "alice", true); !prior {
		t.Error("a repeat pause reported a prior of false — the flip's audit would claim a change that did not happen")
	}
	if prior, _ := p.SetPause(ctx, "alice", false); !prior {
		t.Error("un-pausing reported a prior of false")
	}
	if paused, _ := p.Paused(ctx, "alice"); paused {
		t.Error("alice is still paused after resuming")
	}
	// Pausing somebody who does not exist records a decision about nobody.
	if _, err := p.SetPause(ctx, "ghost", true); err == nil {
		t.Error("SetPause accepted an unknown person")
	}
}

// TestDeclareRefusesAPersonWhoDoesNotExist is drain D5: `budgets` carries no
// foreign key (the `lanes` precedent), so without this check an operator typo
// mints a ghost row that the recreated cost_budget_remainder view then serves
// as a real declaration.
func TestDeclareRefusesAPersonWhoDoesNotExist(t *testing.T) {
	e := newEnv(t)
	seedUser(t, e, "alice")
	b := NewBudgets(e.db)
	ctx := context.Background()
	now := time.Now().UTC()
	row := BudgetRow{UserID: "ghost", Lane: "anthropic", PeriodTokens: 100, PeriodDays: 30,
		PeriodStart: now, DeclaredTS: now, DeclaredBy: "op"}

	if _, _, err := b.Declare(ctx, row); !errors.Is(err, ErrNoSuchPerson) {
		t.Fatalf("Declare for a nonexistent person: err = %v, want ErrNoSuchPerson", err)
	}
	// NO ROW LANDED — which is the half that matters, because the view unions
	// the budgets table and would have reported the ghost's declaration.
	var n int
	if err := e.db.QueryRowContext(ctx, `SELECT count(*) FROM budgets`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("a refused declaration left %d budget rows", n)
	}
	// The same refusal, from the same sentinel, on the other switch.
	if _, err := NewPause(e.db).SetPause(ctx, "ghost", true); !errors.Is(err, ErrNoSuchPerson) {
		t.Fatalf("SetPause for a nonexistent person: err = %v, want ErrNoSuchPerson", err)
	}
	// …and a real person still works, so the check is not refusing everything.
	row.UserID = "alice"
	if _, _, err := b.Declare(ctx, row); err != nil {
		t.Fatalf("Declare for a real person: %v", err)
	}
}

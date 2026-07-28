package metering

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// budgets.go — the S10.4 operator switches, made durable (migration 0017).
//
// Two things live here, and they are the two halves of "how much automation do
// I want running": the DECLARED BUDGET the pressure gauge measures against, and
// the PAUSE flag that stops automation outright. pressure.go's Budget said it
// itself — "the 13.4 surface + persistence is a later phase" — and this is that
// phase. The gauge is untouched: it still takes a Budget value and still applies
// the one denominator rule (D4). All that changes is where the value comes from.
//
// THE UNIT IS NEVER DOLLARS (D5). PeriodTokens is in weighted-consumption units
// — the same unit Gauge.WeightedConsumption accumulates — because a flat-rate
// lane has no per-token price and budgeting one in currency is exactly the
// inversion S10 exists to prevent.

// BudgetRow is one persisted (person, lane) budget declaration: the value object
// the gauge consumes, plus the period definition and the audit face of the row.
type BudgetRow struct {
	UserID string
	Lane   string
	// PeriodTokens is the budget, in weighted-consumption units (S10.4).
	PeriodTokens int64
	// PeriodStart is the instant the period's consumption is summed from;
	// PeriodDays is the length the operator declared it for. Nothing rolls a
	// period over — re-declaring is the act that starts the next one.
	PeriodStart time.Time
	PeriodDays  int64
	DeclaredTS  time.Time
	DeclaredBy  string
}

// Budget converts the row to the landed value object the gauge takes.
func (r BudgetRow) Budget() Budget {
	return Budget{PeriodTokens: r.PeriodTokens, PeriodStart: r.PeriodStart, Declared: true}
}

// ErrNoSuchPerson refuses an S10.4 switch aimed at somebody who does not exist.
// Both switches make the same refusal for the same reason: a budget or a pause
// declared for nobody is a decision recorded about nobody, and the tables behind
// them carry no foreign key that would have caught it.
var ErrNoSuchPerson = errors.New("metering: no such person")

// Budgets is the durable S10.4 budget store (migration 0017 `budgets`).
type Budgets struct {
	db *storage.DB
}

// NewBudgets returns the budget store over db.
func NewBudgets(db *storage.DB) *Budgets { return &Budgets{db: db} }

// Declare upserts one (person, lane) budget and returns the row it REPLACED,
// if any. The prior row is returned rather than discarded because the edit's
// audit obligation is old→new (S14.2 family 5), and the only place the old
// value still exists at that moment is here.
//
// Validation is the caller's — this is a store, and the boundary that admits a
// person's input is the HTTP verb (CONVENTIONS §30). What this refuses is what
// the schema refuses: a non-positive budget or an empty key, which are defects
// rather than inputs.
func (b *Budgets) Declare(ctx context.Context, row BudgetRow) (prior BudgetRow, existed bool, err error) {
	switch {
	case row.UserID == "" || row.Lane == "":
		return BudgetRow{}, false, fmt.Errorf("metering: budget needs a person and a lane")
	case row.PeriodTokens <= 0:
		return BudgetRow{}, false, fmt.Errorf("metering: budget for %q/%s must be positive (a zero budget is a pause, not a budget)", row.UserID, row.Lane)
	case row.PeriodDays <= 0:
		return BudgetRow{}, false, fmt.Errorf("metering: budget for %q/%s needs a period length in days", row.UserID, row.Lane)
	case row.DeclaredBy == "":
		return BudgetRow{}, false, fmt.Errorf("metering: budget for %q/%s needs the declaring actor", row.UserID, row.Lane)
	}
	if row.PeriodStart.IsZero() {
		row.PeriodStart = row.DeclaredTS
	}
	err = b.db.WriteTx(ctx, func(tx *sql.Tx) error {
		// The person must EXIST. A budget is per-person durable operator data,
		// and `budgets` carries no foreign key (the `lanes` precedent), so a
		// typo would otherwise mint a row for nobody — which the recreated
		// cost_budget_remainder view then faithfully serves, because the view
		// unions the budgets table. Refusing here is the same refusal SetPause
		// already makes, for the same reason: declaring a budget for nobody
		// records a decision about nobody.
		var exists int
		switch err := tx.QueryRowContext(ctx,
			`SELECT 1 FROM users WHERE user_id = ?`, row.UserID).Scan(&exists); {
		case errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("%w: %q", ErrNoSuchPerson, row.UserID)
		case err != nil:
			return fmt.Errorf("metering: read person %q: %w", row.UserID, err)
		}
		p, ok, rerr := readBudget(ctx, tx.QueryRowContext, row.UserID, row.Lane)
		if rerr != nil {
			return rerr
		}
		prior, existed = p, ok
		_, eerr := tx.ExecContext(ctx,
			`INSERT INTO budgets (user_id, lane, period_tokens, period_start, period_days, declared_ts, declared_by)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (user_id, lane) DO UPDATE SET
			   period_tokens = excluded.period_tokens,
			   period_start  = excluded.period_start,
			   period_days   = excluded.period_days,
			   declared_ts   = excluded.declared_ts,
			   declared_by   = excluded.declared_by`,
			row.UserID, row.Lane, row.PeriodTokens,
			row.PeriodStart.UTC().Format(time.RFC3339Nano), row.PeriodDays,
			row.DeclaredTS.UTC().Format(time.RFC3339Nano), row.DeclaredBy)
		if eerr != nil {
			return fmt.Errorf("metering: declare budget %q/%s: %w", row.UserID, row.Lane, eerr)
		}
		return nil
	})
	if err != nil {
		return BudgetRow{}, false, err
	}
	return prior, existed, nil
}

// Row reads one declared (person, lane) budget. The bool is false when none is
// declared — the honest absence, not an error.
func (b *Budgets) Row(ctx context.Context, userID, lane string) (BudgetRow, bool, error) {
	return readBudget(ctx, b.db.QueryRowContext, userID, lane)
}

// Budget reads the (person, lane) budget as the value object the gauge takes,
// falling back to UndeclaredBudget() when none is declared. It is the ONE place
// the undeclared posture is produced from storage, so a caller can never
// accidentally invent a denominator (D4).
func (b *Budgets) Budget(ctx context.Context, userID, lane string) (Budget, error) {
	row, ok, err := b.Row(ctx, userID, lane)
	if err != nil {
		return UndeclaredBudget(), err
	}
	if !ok {
		return UndeclaredBudget(), nil
	}
	return row.Budget(), nil
}

// rowQuery is the shape both *storage.DB and *sql.Tx satisfy, so the read runs
// identically inside and outside the declare transaction.
type rowQuery func(ctx context.Context, query string, args ...any) *sql.Row

func readBudget(ctx context.Context, q rowQuery, userID, lane string) (BudgetRow, bool, error) {
	var (
		row                   BudgetRow
		periodStart, declared string
	)
	err := q(ctx,
		`SELECT user_id, lane, period_tokens, period_start, period_days, declared_ts, declared_by
		   FROM budgets WHERE user_id = ? AND lane = ?`, userID, lane).
		Scan(&row.UserID, &row.Lane, &row.PeriodTokens, &periodStart, &row.PeriodDays, &declared, &row.DeclaredBy)
	if errors.Is(err, sql.ErrNoRows) {
		return BudgetRow{}, false, nil
	}
	if err != nil {
		return BudgetRow{}, false, fmt.Errorf("metering: read budget %q/%s: %w", userID, lane, err)
	}
	row.PeriodStart, row.DeclaredTS = parseBudgetTS(periodStart), parseBudgetTS(declared)
	return row, true, nil
}

func parseBudgetTS(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// ── pause-my-automation (S10.4) ──────────────────────────────────────────────

// Pause is the durable per-person pause-my-automation switch
// (migration 0017 `users.automation_paused`).
//
// What it stops is AUTOMATION. The scheduler consults it in the claim loop and
// admits no background work for a paused person; an in-flight run parks at its
// next stage-session boundary. What it never stops is a person's own
// interactive work — S10.4's headroom rule is that the switch buys the person
// their own capacity back, and a switch that locked them out of their own
// platform would do the opposite. That property lives in the consulting code
// and is asserted there; this store only records the bit.
type Pause struct {
	db *storage.DB
}

// NewPause returns the pause store over db.
func NewPause(db *storage.DB) *Pause { return &Pause{db: db} }

// Paused reports whether the person has paused their automation. An unknown
// person is not paused: the flag is a property of a user row, and refusing to
// admit work for an id that does not exist is the caller's own check.
func (p *Pause) Paused(ctx context.Context, userID string) (bool, error) {
	var paused int64
	err := p.db.QueryRowContext(ctx,
		`SELECT automation_paused FROM users WHERE user_id = ?`, userID).Scan(&paused)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("metering: read pause flag for %q: %w", userID, err)
	}
	return paused == 1, nil
}

// SetPause flips the switch and returns its PRIOR value, so the flip's audit row
// can carry old→new (S14.2 family 5). An unknown person is an error: pausing
// somebody who does not exist would record a decision about nobody.
func (p *Pause) SetPause(ctx context.Context, userID string, paused bool) (bool, error) {
	prior := false
	err := p.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var cur int64
		switch err := tx.QueryRowContext(ctx,
			`SELECT automation_paused FROM users WHERE user_id = ?`, userID).Scan(&cur); {
		case errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("%w: %q", ErrNoSuchPerson, userID)
		case err != nil:
			return fmt.Errorf("metering: read pause flag for %q: %w", userID, err)
		}
		prior = cur == 1
		v := int64(0)
		if paused {
			v = 1
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET automation_paused = ? WHERE user_id = ?`, v, userID); err != nil {
			return fmt.Errorf("metering: set pause flag for %q: %w", userID, err)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return prior, nil
}

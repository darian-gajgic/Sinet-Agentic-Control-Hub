package metering

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// planbudgets.go — the S10.4 automation budget for a PLAN-metered lane, made
// durable (migration 0025 `plan_budgets`).
//
// It is the tier-3 sibling of budgets.go, and it is a sibling rather than a
// second use of the `budgets` table for four reasons, none of them stylistic:
//
//	 1. THE UNIT. A token budget is in weighted-consumption units and is a whole
//	    number; a plan budget is in the PLAN's own unit for one window and is a
//	    float, because a proposal is a fraction of an allowance.
//	 2. THE PERIOD SHAPE. `budgets.period_days` cannot express a rolling FIVE
//	    HOUR window. PlanBudget.PeriodHours already can.
//	 3. THE KEY. A plan lane declares SEVERAL allowance windows and they are
//	    separate allowances — the zai plan publishes a five-hour one beside a
//	    weekly one — so the grain is (person, lane, window), which is the
//	    (person, lane, period) grain S18.3 ratified for this surface. A window
//	    counting something other than what the lane's consumption counts cannot
//	    carry a budget at all (PlanBudgetWindowRefusal); its allowance is still
//	    reported, it just cannot be a denominator.
//	 4. THE PACKAGE'S OWN RULE. planunits.go's head comment: the token reading
//	    and the plan reading "are therefore two readings, each stamped with its
//	    own tier". One table with a nullable unit column is how somebody later
//	    adds a credit to a token.
//
// THE UNIT IS NEVER DOLLARS (D5), and nothing here multiplies, prices or
// rescales anything: a flat-rate lane has no per-unit price, and pricing one is
// the inversion S10 exists to prevent.

// The closed vocabulary of how a row came to exist. It is closed because the
// two provenances mean different things to a person reading their own budget:
// a seeded row is a PROPOSAL the platform derived from the provider's published
// allowance and labeled "assumed", and an operator-set row is a figure somebody
// chose. A value outside the set is refused at write rather than stored and
// rendered.
const (
	// PlanBudgetProposalSeeded is a row taken from the plan document's own
	// allowance at ⚙ budget.background_window_fraction (ProposePlanBudget).
	PlanBudgetProposalSeeded = "proposal-seeded"
	// PlanBudgetOperatorSet is a figure the operator declared themselves.
	PlanBudgetOperatorSet = "operator-set"
)

// PlanBudgetRow is one persisted (person, lane, window) plan-budget
// declaration: the denominator the tier-3 reading measures against, its period
// definition, and the row's own audit and provenance face.
type PlanBudgetRow struct {
	UserID string
	Lane   string
	// Window is the plan document's own quota name ("rolling-5h", "weekly").
	// No vocabulary is minted here: the names come from the documents.
	Window string
	// PeriodUnits is the budget in the WINDOW's own unit — never the
	// provider's published allowance, which is a seed and not a denominator
	// (S10.4: "never an inferred provider window (D4)").
	PeriodUnits float64
	// Unit is the window's own unit, carried on the row so no surface has to
	// re-derive it. It is not the caller's to choose: Declare refuses a row
	// whose unit is not the one the document gives that window, or the stored
	// row would name one thing and the reading count another.
	Unit string
	// PeriodStart is the instant consumption is summed from; PeriodHours is the
	// length the operator declared it for, and the reading stops applying once
	// it has elapsed. Nothing rolls a period over — that would be a timer —
	// so re-declaring is the act that starts the next one.
	PeriodStart time.Time
	PeriodHours float64
	// Source is the closed vocabulary above.
	Source string
	// SeededFrom and Fraction are the PROPOSAL provenance: which allowance row
	// a seeded figure came from and what share of it was taken. Both are empty
	// and zero on an operator-set row, and neither is ever an input to the
	// arithmetic.
	SeededFrom string
	Fraction   float64
	// DeclaredTS / DeclaredBy are the audit face: the row itself says who last
	// set it and when, without a join. The full old→new record is the
	// decision.recorded event (S14.2 family 5).
	DeclaredTS time.Time
	DeclaredBy string
}

// Budget converts the row to the value object ReadPlanUnits takes.
//
// SeededFrom carries the row's WINDOW rather than its proposal provenance,
// because that is the only field on the value object that names WHICH window
// the budget denominates — the reading resolves the seed allowance through it
// and, since drain r1 D1, applies PlanBudgetWindowRefusal through it too. The
// proposal provenance stays on the ROW, where it is read as provenance and
// never as an input to the arithmetic.
//
// (Corrected 2026-08-25: this used to justify itself by the reading taking its
// UNIT from the window. That reason was wrong in the one case it was written
// for — a weekly row on a request-counting plan labeled that count "credits" —
// and the unit now
// comes from the document, which is what consumption is actually counted in.)
func (r PlanBudgetRow) Budget() PlanBudget {
	return PlanBudget{
		PeriodUnits: r.PeriodUnits,
		PeriodStart: r.PeriodStart,
		PeriodHours: r.PeriodHours,
		Declared:    true,
		SeededFrom:  r.Window,
		Fraction:    r.Fraction,
	}
}

// PlanBudgets is the durable S10.4 plan-budget store (migration 0025).
type PlanBudgets struct {
	db *storage.DB
}

// NewPlanBudgets returns the plan-budget store over db.
func NewPlanBudgets(db *storage.DB) *PlanBudgets { return &PlanBudgets{db: db} }

// Declare upserts one (person, lane, window) plan budget and returns the row it
// REPLACED, if any. The prior row is returned rather than discarded because the
// edit's audit obligation is old→new (S14.2 family 5), and the only place the
// old value still exists at that moment is here.
//
// Validation of a person's INPUT is the HTTP verb's (CONVENTIONS §30). What
// this refuses is what the schema refuses: defects, which are never inputs.
func (b *PlanBudgets) Declare(ctx context.Context, row PlanBudgetRow) (prior PlanBudgetRow, existed bool, err error) {
	switch {
	case row.UserID == "" || row.Lane == "" || row.Window == "":
		return PlanBudgetRow{}, false, fmt.Errorf("metering: a plan budget needs a person, a lane and a window — the grain the tier-3 reading denominates at (S18.3)")
	case row.PeriodUnits <= 0:
		return PlanBudgetRow{}, false, fmt.Errorf("metering: plan budget for %q/%s/%s must be positive (a zero budget is a pause, not a budget)", row.UserID, row.Lane, row.Window)
	case row.PeriodHours <= 0:
		return PlanBudgetRow{}, false, fmt.Errorf("metering: plan budget for %q/%s/%s needs a period length in hours", row.UserID, row.Lane, row.Window)
	case row.Unit == "":
		return PlanBudgetRow{}, false, fmt.Errorf("metering: plan budget for %q/%s/%s carries no unit — a budget whose unit is unknown cannot be reported honestly (S10.4)", row.UserID, row.Lane, row.Window)
	case row.DeclaredBy == "":
		return PlanBudgetRow{}, false, fmt.Errorf("metering: plan budget for %q/%s/%s needs the declaring actor", row.UserID, row.Lane, row.Window)
	case row.DeclaredTS.IsZero():
		// The row's own audit face has to say WHEN, and the period start
		// defaults to it — a zero stamp would store the year 1 as the instant
		// consumption is summed from.
		return PlanBudgetRow{}, false, fmt.Errorf("metering: plan budget for %q/%s/%s needs the instant it was declared at", row.UserID, row.Lane, row.Window)
	case row.Source != PlanBudgetProposalSeeded && row.Source != PlanBudgetOperatorSet:
		return PlanBudgetRow{}, false, fmt.Errorf("metering: plan budget for %q/%s/%s declares source %q, want one of {%s, %s} — a provenance outside the set would be stored and rendered as if it meant something",
			row.UserID, row.Lane, row.Window, row.Source, PlanBudgetProposalSeeded, PlanBudgetOperatorSet)
	}
	// The row must be one the LANE's own plan can actually be denominated by.
	// This is the same predicate the reading applies and the same one the HTTP
	// boundary refuses input by — defence in depth against a row that would
	// divide one unit by another and then bind routing (drain r1 D1/D4/D5).
	doc, ok := PlanDocFor(row.Lane)
	if !ok {
		return PlanBudgetRow{}, false, fmt.Errorf("metering: lane %q meters in no plan units, so it carries no plan budget — its automation budget is the weighted-consumption one", row.Lane)
	}
	// OQ-2: a pooled allowance is declared ONCE, against the pool's canonical
	// lane. Two rows against one allowance is a number the operator cannot
	// reconcile — under max-binds either could bind the lane, and the row they
	// did not touch would be the one that moved.
	if refusal := PlanPoolRefusal(doc, row.Lane); refusal != "" {
		return PlanBudgetRow{}, false, fmt.Errorf("metering: plan budget for %q/%s/%s refused: %s",
			row.UserID, row.Lane, row.Window, refusal)
	}
	if refusal := PlanBudgetWindowRefusal(doc, row.Window); refusal != "" {
		return PlanBudgetRow{}, false, fmt.Errorf("metering: plan budget for %q/%s/%s refused: %s", row.UserID, row.Lane, row.Window, refusal)
	}
	if want := doc.QuotaUnit(row.Window); row.Unit != want {
		return PlanBudgetRow{}, false, fmt.Errorf("metering: plan budget for %q/%s/%s declares unit %q, want %q — the unit is the WINDOW's own and is not the caller's to choose, "+
			"or the stored row would name one thing and the reading count another",
			row.UserID, row.Lane, row.Window, row.Unit, want)
	}
	if row.PeriodStart.IsZero() {
		row.PeriodStart = row.DeclaredTS
	}
	err = b.db.WriteTx(ctx, func(tx *sql.Tx) error {
		// The person must EXIST, for the reason the token budget refuses the
		// same thing: a budget declared for nobody is a decision recorded about
		// nobody, and the table carries no foreign key that would have caught
		// it.
		var exists int
		switch err := tx.QueryRowContext(ctx,
			`SELECT 1 FROM users WHERE user_id = ?`, row.UserID).Scan(&exists); {
		case errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("%w: %q", ErrNoSuchPerson, row.UserID)
		case err != nil:
			return fmt.Errorf("metering: read person %q: %w", row.UserID, err)
		}
		p, ok, rerr := readPlanBudget(ctx, tx.QueryRowContext, row.UserID, row.Lane, row.Window)
		if rerr != nil {
			return rerr
		}
		prior, existed = p, ok
		_, eerr := tx.ExecContext(ctx,
			`INSERT INTO plan_budgets (user_id, lane, "window", period_units, unit, period_start, period_hours,
			                           source, seeded_from, fraction, declared_ts, declared_by)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (user_id, lane, "window") DO UPDATE SET
			   period_units = excluded.period_units,
			   unit         = excluded.unit,
			   period_start = excluded.period_start,
			   period_hours = excluded.period_hours,
			   source       = excluded.source,
			   seeded_from  = excluded.seeded_from,
			   fraction     = excluded.fraction,
			   declared_ts  = excluded.declared_ts,
			   declared_by  = excluded.declared_by`,
			row.UserID, row.Lane, row.Window, row.PeriodUnits, row.Unit,
			row.PeriodStart.UTC().Format(time.RFC3339Nano), row.PeriodHours,
			row.Source, row.SeededFrom, row.Fraction,
			row.DeclaredTS.UTC().Format(time.RFC3339Nano), row.DeclaredBy)
		if eerr != nil {
			return fmt.Errorf("metering: declare plan budget %q/%s/%s: %w", row.UserID, row.Lane, row.Window, eerr)
		}
		return nil
	})
	if err != nil {
		return PlanBudgetRow{}, false, err
	}
	return prior, existed, nil
}

// Row reads one declared (person, lane, window) plan budget. The bool is false
// when none is declared — the honest absence, not an error.
func (b *PlanBudgets) Row(ctx context.Context, userID, lane, window string) (PlanBudgetRow, bool, error) {
	return readPlanBudget(ctx, b.db.QueryRowContext, userID, lane, window)
}

// Rows lists every declared window for one (person, lane), ordered by window
// name so a surface reading them renders in a stable order.
func (b *PlanBudgets) Rows(ctx context.Context, userID, lane string) ([]PlanBudgetRow, error) {
	rows, err := b.db.QueryContext(ctx,
		`SELECT user_id, lane, "window", period_units, unit, period_start, period_hours,
		        source, seeded_from, fraction, declared_ts, declared_by
		   FROM plan_budgets WHERE user_id = ? AND lane = ? ORDER BY "window"`, userID, lane)
	if err != nil {
		return nil, fmt.Errorf("metering: read plan budgets %q/%s: %w", userID, lane, err)
	}
	defer rows.Close()
	var out []PlanBudgetRow
	for rows.Next() {
		row, serr := scanPlanBudget(rows.Scan)
		if serr != nil {
			return nil, fmt.Errorf("metering: scan plan budget %q/%s: %w", userID, lane, serr)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// PlanBudget reads one window's budget as the value object ReadPlanUnits takes,
// falling back to UndeclaredPlanBudget() when none is declared. It is the ONE
// place the undeclared plan posture is produced from storage, so a caller can
// never accidentally invent a denominator (D4) — the same rule Budgets.Budget
// holds on the token side.
func (b *PlanBudgets) PlanBudget(ctx context.Context, userID, lane, window string) (PlanBudget, error) {
	row, ok, err := b.Row(ctx, userID, lane, window)
	if err != nil {
		return UndeclaredPlanBudget(), err
	}
	if !ok {
		return UndeclaredPlanBudget(), nil
	}
	return row.Budget(), nil
}

func readPlanBudget(ctx context.Context, q rowQuery, userID, lane, window string) (PlanBudgetRow, bool, error) {
	row, err := scanPlanBudget(q(ctx,
		`SELECT user_id, lane, "window", period_units, unit, period_start, period_hours,
		        source, seeded_from, fraction, declared_ts, declared_by
		   FROM plan_budgets WHERE user_id = ? AND lane = ? AND "window" = ?`, userID, lane, window).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return PlanBudgetRow{}, false, nil
	}
	if err != nil {
		return PlanBudgetRow{}, false, fmt.Errorf("metering: read plan budget %q/%s/%s: %w", userID, lane, window, err)
	}
	return row, true, nil
}

// scanPlanBudget reads one row through whichever Scan the caller holds, so the
// point read and the per-lane listing decode identically.
func scanPlanBudget(scan func(dest ...any) error) (PlanBudgetRow, error) {
	var (
		row                   PlanBudgetRow
		periodStart, declared string
	)
	err := scan(&row.UserID, &row.Lane, &row.Window, &row.PeriodUnits, &row.Unit,
		&periodStart, &row.PeriodHours, &row.Source, &row.SeededFrom, &row.Fraction,
		&declared, &row.DeclaredBy)
	if err != nil {
		return PlanBudgetRow{}, err
	}
	row.PeriodStart, row.DeclaredTS = parseBudgetTS(periodStart), parseBudgetTS(declared)
	return row, nil
}

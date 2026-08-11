package history

import (
	"context"
	"fmt"
	"sort"
)

// layer0.go — S14.10 ¶1: "every S2.5 cost question (per run/task/project/
// person/period, budget remainders, burn rate, limit-event history,
// done-directly figures) is a named SQL view over the ledger/receipts. The
// assistant *selects* views — it never computes money by generation."
//
// THE NEGATIVE IS THE POINT. This file selects from a closed set of view names
// and appends an owner-scope predicate. It contains no arithmetic on tokens, no
// rate, no price table, and no import of internal/metering — pricing happens in
// exactly ONE place (internal/metering, per row, at the row's own timestamp so
// a mid-window price change is honored, S10.3), and the views read the priced
// result. layer0_test.go asserts that as a source scan over both this package
// and migration 0016, in the shape of the §35 "no bare-rate accessor" guard:
// the unpriced-computing form is not merely absent, it is checked absent.

// View names — the closed Layer-0 registry. Every name is a constant, so a view
// name never originates in user text and cannot be interpolated into SQL.
const (
	ViewCostPerRun       = "cost_per_run"
	ViewCostPerTask      = "cost_per_task"
	ViewCostPerProject   = "cost_per_project"
	ViewCostPerPerson    = "cost_per_person"
	ViewCostPerPeriod    = "cost_per_period"
	ViewCostBurnRate     = "cost_burn_rate"
	ViewBudgetRemainder  = "cost_budget_remainder"
	ViewDoneDirectly     = "cost_done_directly"
	ViewLimitEventHist   = "limit_event_history"
	ViewRoutingQuality   = "routing_quality"
	ViewTaskProjectEdge  = "task_project"
	viewOwnerColumnNone  = ""
	viewOwnerColumnOwner = "user_id"
)

// CostQuestion names one S2.5 cost question, so the mapping from the SPEC's
// list to the shipped views is data a test can check for completeness rather
// than a claim in a comment.
type CostQuestion string

// The S14.10 ¶1 list, verbatim in its own vocabulary.
const (
	QuestionPerRun          CostQuestion = "per run"
	QuestionPerTask         CostQuestion = "per task"
	QuestionPerProject      CostQuestion = "per project"
	QuestionPerPerson       CostQuestion = "per person"
	QuestionPerPeriod       CostQuestion = "per period"
	QuestionBudgetRemainder CostQuestion = "budget remainders"
	QuestionBurnRate        CostQuestion = "burn rate"
	QuestionLimitEvents     CostQuestion = "limit-event history"
	QuestionDoneDirectly    CostQuestion = "done-directly figures"
	// QuestionRoutingQuality is S14.10 ¶5's own view, not an S2.5 cost
	// question; it is registered here because it is a Layer-0 named view and
	// Layer 2's allowlist is the Layer-0 registry.
	QuestionRoutingQuality CostQuestion = "routing quality (S14.10 ¶5)"
	// QuestionLinkage marks a view that exists to isolate a linkage weakness in
	// one place rather than to answer a question.
	QuestionLinkage CostQuestion = "linkage (support view)"
)

// View is one named Layer-0 deterministic view.
type View struct {
	// Name is the SQL view name (migration 0016).
	Name string
	// Question is the S2.5 cost question this view answers.
	Question CostQuestion
	// OwnerColumn is the column S01.9 scoping applies to; "" means the view has
	// no owner column and is operator-only.
	OwnerColumn string
	// Description is the plain-language reading, rendered beside an answer.
	Description string
	// Order is the deterministic sort applied when a caller selects the view,
	// so two identical questions get identical answers.
	Order string
}

// layer0Views is the closed registry — DATA, not code (the §32 SeedRows / §35
// registered.go precedent).
var layer0Views = []View{
	{
		Name: ViewCostPerRun, Question: QuestionPerRun, OwnerColumn: viewOwnerColumnOwner,
		Description: "what one run consumed, with its pricing status (a run with no materialized receipt reads NO-RECEIPT, never $0)",
		Order:       "run_created_ts DESC, run_id",
	},
	{
		Name: ViewCostPerTask, Question: QuestionPerTask, OwnerColumn: viewOwnerColumnOwner,
		Description: "what a task's runs consumed together — its intake, compose, execute and verify legs",
		Order:       "priced_usd DESC, task_id",
	},
	{
		Name: ViewCostPerProject, Question: QuestionPerProject, OwnerColumn: viewOwnerColumnOwner,
		Description: "consumption per registered project, via the task→project edge — the claims declared at plan approval, else the registry pin the intake recorded at triage; work with no project lands in an explicit '(no project)' bucket",
		Order:       "priced_usd DESC, project_id",
	},
	{
		Name: ViewCostPerPerson, Question: QuestionPerPerson, OwnerColumn: viewOwnerColumnOwner,
		Description: "consumption per person — work is billed to the requester (3.4), so the owner column IS the person",
		Order:       "priced_usd DESC, user_id",
	},
	{
		Name: ViewCostPerPeriod, Question: QuestionPerPeriod, OwnerColumn: viewOwnerColumnOwner,
		Description: "consumption per UTC day, by the moment the cost became known (the receipt's materialization)",
		Order:       "day DESC, user_id",
	},
	{
		Name: ViewCostBurnRate, Question: QuestionBurnRate, OwnerColumn: viewOwnerColumnOwner,
		Description: "usd per observed day across the span between a person's first and last receipt; quiet days count against the rate, which is what makes it a rate",
		Order:       "usd_per_day DESC, user_id",
	},
	{
		Name: ViewBudgetRemainder, Question: QuestionBudgetRemainder, OwnerColumn: viewOwnerColumnOwner,
		Description: "the budget remainder — the declared automation budget where one exists (in weighted-consumption units, never dollars: a flat-rate lane has no dollar remainder), and a RECORDED ABSENCE with its reason where none is declared. The dollar columns are NULL either way",
		Order:       "user_id",
	},
	{
		Name: ViewDoneDirectly, Question: QuestionDoneDirectly, OwnerColumn: viewOwnerColumnOwner,
		Description: "the done-directly line, CITED from each receipt's own direct_use block (the formula is registered in Spec/benchmark-preregistration-v1.md §13 and is never restated here)",
		Order:       "receipt_ts DESC, run_id",
	},
	{
		Name: ViewLimitEventHist, Question: QuestionLimitEvents, OwnerColumn: viewOwnerColumnOwner,
		Description: "the provider limit signals actually hit, with class, signal and reset time; includes the registered legacy type alias so the history does not begin at a rename",
		Order:       "event_seq DESC",
	},
	{
		Name: ViewRoutingQuality, Question: QuestionRoutingQuality, OwnerColumn: viewOwnerColumnOwner,
		Description: "every routing decision joined to what happened next — verdicts, rework rounds and the receipt — so routing itself is auditable over time (S2.6)",
		Order:       "event_seq DESC",
	},
	{
		Name: ViewTaskProjectEdge, Question: QuestionLinkage, OwnerColumn: viewOwnerColumnNone,
		Description: "the task→project edge, isolated so its weakness is stated in one place: runs and tasks carry no project column at v0, so the edge is read from the approved claims and, before approval, from the registry pin on the task's latest intake state, and for an onboarding task from the registry entry its own id names",
		Order:       "task_id",
	},
}

// Views returns the Layer-0 registry, sorted by name.
func Views() []View {
	out := append([]View(nil), layer0Views...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ViewByName looks a view up in the closed registry.
func ViewByName(name string) (View, bool) {
	for _, v := range layer0Views {
		if v.Name == name {
			return v, true
		}
	}
	return View{}, false
}

// CostQuestions returns the S2.5 questions the registry answers, so a test can
// assert the spec's list is covered without restating it in the test.
func CostQuestions() []CostQuestion {
	seen := map[CostQuestion]bool{}
	var out []CostQuestion
	for _, v := range layer0Views {
		if !seen[v.Question] {
			seen[v.Question] = true
			out = append(out, v.Question)
		}
	}
	return out
}

// SelectView is the Layer-0 verb: SELECT a named view, owner-scoped per S01.9.
// It SELECTS. It does not compute, price, weight, or convert anything.
//
// The view name is looked up in the closed registry above and only then
// concatenated into the statement — a name that is not a registry constant
// never reaches SQL. Every value is bound.
func (s *Store) SelectView(ctx context.Context, name string, scope Scope, limit int) (Answer, error) {
	v, ok := ViewByName(name)
	if !ok {
		return Answer{}, fmt.Errorf("%w: %q", ErrNoSuchView, name)
	}
	a := newAnswer(LayerDeterministic, v.Name)
	a.Notes = append(a.Notes, v.Description)

	limit = clampLimit(limit)
	stmt := "SELECT * FROM " + v.Name
	var args []any
	switch {
	case v.OwnerColumn == viewOwnerColumnNone && !scope.Operator:
		// A view with no owner column cannot be member-scoped, so it is not
		// served to a member at all. Refusing is the honest direction: silently
		// returning every row would leak, and silently returning none would
		// look like an empty history.
		return Answer{}, fmt.Errorf("history: view %q carries no owner column and is operator-only (S01.9)", v.Name)
	case v.OwnerColumn != viewOwnerColumnNone:
		stmt += " WHERE (? = '' OR " + v.OwnerColumn + " = ?)"
		args = append(args, scope.scopeArg(), scope.scopeArg())
	}
	stmt += " ORDER BY " + v.Order + " LIMIT ?"
	args = append(args, limit+1)

	if err := s.query(ctx, &a, stmt, args, limit); err != nil {
		return Answer{}, err
	}
	return a, nil
}

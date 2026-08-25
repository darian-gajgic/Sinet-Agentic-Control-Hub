package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// meters_verbs.go — the S10.4 meters MUTATIONS (P3-B6-2B): the budget edit, the
// pause-my-automation switch, and the S15.5 board-drag priority hint. Joined at
// P3-LN-6 by the PLAN-budget declaration — the same act for a lane whose plan
// meters in its own units, at the (person, lane, window) grain those units need.
//
// The rule meters.go is built around holds here unchanged and is the reason this
// file is separate from it: NO MONEY IS COMPUTED IN internal/api. A budget is
// declared in its GAUGE'S OWN unit — weighted-consumption units for the token
// gauge, the window's own plan unit for the tier-3 one — and never in dollars,
// because a flat-rate lane has no per-token price and budgeting one in currency
// is the D5 inversion the metering layer exists to prevent. Nothing in this file
// multiplies, prices, or converts anything.
//
// All four verbs are control-plane-internal state acts (R22): none performs an
// outward effect, and none is reachable except through an authenticated
// session (§9). The pause verb in particular PARKS and stops admitting — it
// never cancels anything, and it touches no cancel entry point.

// BudgetRecord is one declared (person, lane) automation budget as this
// transport speaks it.
//
// PeriodTokens is in WEIGHTED-CONSUMPTION UNITS — the unit the S10.4 gauge
// accumulates — and there is no dollar field here and none may be added (D5).
type BudgetRecord struct {
	Owner string `json:"owner"`
	Lane  string `json:"lane"`
	// PeriodTokens is the budget itself. The unit is stated on the wire so a
	// surface cannot render it as currency by accident.
	PeriodTokens int64     `json:"period_tokens"`
	Unit         string    `json:"unit,omitempty"`
	PeriodStart  time.Time `json:"period_start"`
	PeriodDays   int64     `json:"period_days"`
	DeclaredTS   time.Time `json:"declared_ts"`
	DeclaredBy   string    `json:"declared_by"`
}

// budgetUnit is the unit label served with every budget. It is a LABEL, not a
// conversion: nothing here turns the number into anything else.
const budgetUnit = "weighted-consumption units (S10.4)"

// BudgetStore is the S10.4 durable budget seam (migration 0017). The shell
// adapts *metering.Budgets to it; nil leaves the budget verb answering 503.
type BudgetStore interface {
	// DeclareBudget upserts the budget, returning the row it replaced so the
	// edit's audit row can carry old→new.
	DeclareBudget(ctx context.Context, rec BudgetRecord) (prior BudgetRecord, existed bool, err error)
	// Budget reads the declared budget; the bool is false when none is declared
	// (the honest absence, never a zero).
	Budget(ctx context.Context, userID, lane string) (BudgetRecord, bool, error)
}

// PlanBudgetRecord is one declared (person, lane, window) PLAN budget as this
// transport speaks it.
//
// PeriodUnits is in the WINDOW's own plan unit — credits, requests — which is
// why Unit rides the record. There is no dollar field here and none may be
// added: a flat-rate lane has no per-unit price, and pricing a plan unit is the
// D5 inversion this whole file is written against.
type PlanBudgetRecord struct {
	Owner string `json:"owner"`
	Lane  string `json:"lane"`
	// Window is the plan document's own quota name ("rolling-5h", "weekly").
	// The grain is (person, lane, window) because a lane's windows can be
	// denominated differently, and one lane-wide figure cannot describe that.
	Window      string    `json:"window"`
	PeriodUnits float64   `json:"period_units"`
	Unit        string    `json:"unit"`
	PeriodStart time.Time `json:"period_start"`
	PeriodHours float64   `json:"period_hours"`
	// Source is how the figure came to exist: seeded from the plan's published
	// allowance, or set by the operator.
	Source string `json:"source"`
	// SeededFrom / Fraction are the proposal's provenance — which allowance row
	// it was taken from and what share of it. Absent on an operator-set row,
	// and never an input to any arithmetic.
	SeededFrom string    `json:"seeded_from,omitempty"`
	Fraction   float64   `json:"seed_share,omitempty"`
	DeclaredTS time.Time `json:"declared_ts"`
	DeclaredBy string    `json:"declared_by"`
}

// PlanWindowRecord is one allowance window a lane's plan declares, as the
// boundary sees it.
//
// It exists so the VERB can validate the window name against the lane's own
// document: input validation belongs at the boundary that admits a person's
// input (CONVENTIONS §30), and internal/api cannot see a plan document (§11).
// Without it the check would either move into the store — a defect check
// standing in for an input check — or surface a typo as a 500.
type PlanWindowRecord struct {
	Name        string  `json:"name"`
	Unit        string  `json:"unit"`
	WindowHours float64 `json:"window_hours"`
	// Allowance is the PUBLISHED figure a budget is proposed FROM. It is
	// provenance and never a denominator (S10.4/D4).
	Allowance float64 `json:"allowance,omitempty"`
	// AllowanceUnverified says nobody published an allowance for this window,
	// so Allowance is 0 and means "unknown" rather than "none". Nothing can be
	// proposed from such a window: a fraction of a number nobody published is
	// the inferred provider window D4 bars.
	AllowanceUnverified bool `json:"allowance_unverified,omitempty"`
	// Budgetable is false when this window cannot carry a budget at all, with
	// NotBudgetable saying why. The verdict is COMPUTED BY THE STORE'S OWN
	// predicate and carried here rather than re-derived: internal/api cannot
	// see a plan document (§11), and a rule spelled twice is a rule that drifts
	// (§65 D4). A window counting something other than what the lane's
	// consumption counts is the live case — a budget on it would divide one
	// unit by another (drain r1 D1).
	Budgetable    bool   `json:"budgetable"`
	NotBudgetable string `json:"not_budgetable,omitempty"`
}

// PlanBudgetStore is the S10.4 durable PLAN-budget seam (migration 0025). The
// shell adapts *metering.PlanBudgets to it; nil leaves the plan-budget verb
// answering 503.
type PlanBudgetStore interface {
	// DeclarePlanBudget upserts the budget, returning the row it replaced so
	// the edit's audit row can carry old→new.
	DeclarePlanBudget(ctx context.Context, rec PlanBudgetRecord) (prior PlanBudgetRecord, existed bool, err error)
	// PlanBudget reads one declared window; the bool is false when none is
	// declared (the honest absence, never a zero).
	PlanBudget(ctx context.Context, userID, lane, window string) (PlanBudgetRecord, bool, error)
	// PlanWindows lists the allowance windows the lane's plan declares. An
	// empty list is a lane whose plan meters in no windows of its own.
	PlanWindows(ctx context.Context, lane string) ([]PlanWindowRecord, error)
	// ProposePlanBudget derives the S10.4 proposal for a window: the published
	// allowance at ⚙ budget.background_window_fraction, in the window's own
	// unit. It is a PROPOSAL — the operator's to accept — and this verb is its
	// first production consumer.
	ProposePlanBudget(ctx context.Context, userID, lane, window string, start time.Time) (PlanBudgetRecord, error)
}

// PauseStore is the S10.4 pause-my-automation seam (migration 0017).
// *metering.Pause satisfies it; nil leaves the pause verb answering 503.
type PauseStore interface {
	Paused(ctx context.Context, userID string) (bool, error)
	// SetPause flips the switch and returns its PRIOR value, so the flip is
	// audited as old→new rather than as a bare assertion of the new state.
	SetPause(ctx context.Context, userID string, paused bool) (bool, error)
}

// HintSurface is the S15.5 board-drag hint seam. *scheduler.Scheduler satisfies
// it; nil leaves the hint verb answering 503. The bool it returns is "a queued
// row was actually reordered" — false is the honest stale board, not a failure.
type HintSurface interface {
	SetPriorityHint(ctx context.Context, runID string, rank int64) (bool, error)
}

// The card types the family-5 audit rows of this file carry (S14.2). They name
// the ACT, so an inventory walk over the decision log can tell a budget edit
// from a pause flip from a drag without reading prose.
const (
	cardTypeBudget     = "budget"
	cardTypePlanBudget = "plan_budget"
	cardTypePause      = "automation_pause"
	cardTypeHint       = "priority_hint"
)

// hintRankBound bounds the S15.5 drag rank. Structural, not ⚙ (S18 ratifies no
// such key — the §7 sseBatchSize precedent, interim under the standing
// settings-tab directive), with its reason: the rank is an ORDERING KEY over one
// person's own queued work, so its useful range is the size of a queue a person
// looks at. The bound exists because a boundary that accepts an arbitrary
// integer stores an arbitrary integer, and a hint is a position, not a number
// anybody should be able to make astronomically large. It changes the range of
// the ordering key, never which runs are eligible.
const hintRankBound = 1000

// ── POST /api/meters/budget — declare the S10.4 automation budget ───────────

// budgetBody is the edit. `person` is optional and defaults to the caller.
type budgetBody struct {
	Person string `json:"person,omitempty"`
	Lane   string `json:"lane"`
	// PeriodTokens is in weighted-consumption units. There is no dollar field
	// and a request may not express one (D5).
	PeriodTokens int64  `json:"period_tokens"`
	PeriodDays   int64  `json:"period_days"`
	PeriodStart  string `json:"period_start,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// BudgetDeclared is the response: the budget now in force, plus what it replaced.
type BudgetDeclared struct {
	Budget BudgetRecord  `json:"budget"`
	Prior  *BudgetRecord `json:"prior,omitempty"`
	Detail string        `json:"detail"`
}

func (s *Server) handleBudgetDeclare(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	if s.budgets == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S10.4 budget store is not wired in this process"})
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body budgetBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	scope := s.readScope(r)
	// OQ4: own + operator-any. A member declares their OWN budget; the operator
	// may administer anybody's — the one role bit, D10 — and every edit records
	// its actor either way, so an administered budget is never anonymous.
	owner := strings.TrimSpace(body.Person)
	if owner == "" {
		owner = scope.UserID
	}
	if owner != scope.UserID && !scope.Operator {
		s.writeSurface(w, nil, &SurfaceError{Status: http.StatusForbidden, Code: "forbidden",
			Msg: "declaring another person's automation budget is the operator's (S15.2 \"budget edits (own)\"; D10)"})
		return
	}
	if !s.requirePerson(w, r, owner) {
		return
	}
	lane := strings.TrimSpace(body.Lane)
	switch {
	case lane == "":
		s.writeSurface(w, nil, badRequest(`missing "lane": a budget is declared per (person, lane) — the grain the S10.4 gauge reads at (D4)`))
		return
	case body.PeriodTokens <= 0:
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			`bad "period_tokens" %d: a budget is a positive figure in %s. To stop automation entirely, pause it (POST /api/meters/pause)`,
			body.PeriodTokens, budgetUnit)))
		return
	case body.PeriodDays <= 0:
		s.writeSurface(w, nil, badRequest(`missing "period_days": a period is a start plus a length, and a budget without one says nothing about how long it lasts`))
		return
	}
	now := time.Now().UTC()
	start := now
	if v := strings.TrimSpace(body.PeriodStart); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			s.writeSurface(w, nil, badRequest(fmt.Sprintf("bad period_start %q: want an RFC3339 timestamp", v)))
			return
		}
		start = t.UTC()
	}
	rec := BudgetRecord{
		Owner: owner, Lane: lane, PeriodTokens: body.PeriodTokens, Unit: budgetUnit,
		PeriodStart: start, PeriodDays: body.PeriodDays,
		DeclaredTS: now, DeclaredBy: scope.UserID,
	}
	prior, existed, err := s.budgets.DeclareBudget(r.Context(), rec)
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("declare budget: %w", err))
		return
	}
	// The audit row lands AFTER the store act (the part-A drain-D6 principle): a
	// record of an edit that never committed would be a record of something that
	// did not happen.
	out := BudgetDeclared{Budget: rec,
		Detail: "declared: the S10.4 gauge measures this person's " + lane + " consumption against it immediately, in " + budgetUnit}
	if existed {
		prior.Unit = budgetUnit
		out.Prior = &prior
	}
	if err := s.recordDecision(r.Context(), decisionPayload{
		Actor: scope.UserID, ActorIsOperator: scope.Operator,
		CardID: cardTypeBudget + ":" + owner + ":" + lane, CardType: cardTypeBudget,
		Decision: "declare", Subject: owner + ":" + lane, Reason: body.Reason,
		Old: budgetSnapshot(prior, existed), New: budgetSnapshot(rec, true),
		Ref: "S10.4 operator-declared automation budget; S15.2 meters row",
	}, owner, now); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	s.writeReadJSON(w, out)
}

// budgetSnapshot renders one side of the old→new audit. An absent prior is the
// explicit "undeclared" object rather than a missing member: "there was no
// budget" is a fact about the edit, not a gap in the record.
func budgetSnapshot(rec BudgetRecord, declared bool) json.RawMessage {
	if !declared {
		return json.RawMessage(`{"declared":false}`)
	}
	b, err := json.Marshal(struct {
		Declared     bool   `json:"declared"`
		Lane         string `json:"lane"`
		PeriodTokens int64  `json:"period_tokens"`
		Unit         string `json:"unit"`
		PeriodStart  string `json:"period_start"`
		PeriodDays   int64  `json:"period_days"`
	}{true, rec.Lane, rec.PeriodTokens, budgetUnit,
		rec.PeriodStart.UTC().Format(time.RFC3339Nano), rec.PeriodDays})
	if err != nil {
		return nil
	}
	return b
}

// ── POST /api/meters/plan-budget — declare a PLAN lane's budget ─────────────

// planBudgetBody is the declaration. `person` is optional and defaults to the
// caller. There is no dollar member and a request may not express one (D5).
type planBudgetBody struct {
	Person string `json:"person,omitempty"`
	Lane   string `json:"lane"`
	// Window is the plan document's own quota name. It is required because a
	// plan's windows are denominated differently and a budget for "the lane" is
	// a budget in no particular unit.
	Window string `json:"window"`
	// PeriodUnits is in the window's own plan unit. Ignored — and not required
	// — when FromProposal asks the platform to derive it.
	PeriodUnits float64 `json:"period_units"`
	PeriodHours float64 `json:"period_hours"`
	PeriodStart string  `json:"period_start,omitempty"`
	// FromProposal takes the S10.4 proposal for this window instead of a
	// figure: the published allowance at ⚙ budget.background_window_fraction.
	// It is still the OPERATOR's act — the platform never declares a budget on
	// somebody's behalf, because a denominator nobody chose is the inferred
	// provider window D4 bars.
	FromProposal bool   `json:"from_proposal,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// PlanBudgetDeclared is the response: the plan budget now in force, plus what
// it replaced.
type PlanBudgetDeclared struct {
	Budget PlanBudgetRecord  `json:"budget"`
	Prior  *PlanBudgetRecord `json:"prior,omitempty"`
	Detail string            `json:"detail"`
}

func (s *Server) handlePlanBudgetDeclare(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	if s.planBudgets == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S10.4 plan-budget store is not wired in this process"})
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body planBudgetBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	scope := s.readScope(r)
	// The same authority as the token budget (OQ4): own + operator-any, with
	// every declaration recording its actor either way.
	owner := strings.TrimSpace(body.Person)
	if owner == "" {
		owner = scope.UserID
	}
	if owner != scope.UserID && !scope.Operator {
		s.writeSurface(w, nil, &SurfaceError{Status: http.StatusForbidden, Code: "forbidden",
			Msg: "declaring another person's automation budget is the operator's (S15.2 \"budget edits (own)\"; D10)"})
		return
	}
	if !s.requirePerson(w, r, owner) {
		return
	}
	lane := strings.TrimSpace(body.Lane)
	window := strings.TrimSpace(body.Window)
	switch {
	case lane == "":
		s.writeSurface(w, nil, badRequest(`missing "lane": a plan budget is declared per (person, lane, window) — the grain the tier-3 reading denominates at (S18.3)`))
		return
	case window == "":
		s.writeSurface(w, nil, badRequest(`missing "window": a plan's windows are separate allowances in possibly different units, so a budget for "the lane" is a budget in no particular unit`))
		return
	}
	// The window must be one the LANE's own plan declares, and that is a
	// question about this request's input — so it is answered here, at the
	// boundary that admits it (CONVENTIONS §30). internal/api cannot see a plan
	// document, so the seam hands over the lane's declared windows (§11).
	windows, err := s.planBudgets.PlanWindows(r.Context(), lane)
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read plan windows: %w", err))
		return
	}
	if len(windows) == 0 {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			"lane %q meters in no plan units, so it has no plan budget to declare. Its automation budget is the "+
				"weighted-consumption one (POST /api/meters/budget)", lane)))
		return
	}
	declared, ok := planWindowNamed(windows, window)
	if !ok {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			"lane %q declares no window %q. Its windows are: %s", lane, window, planWindowNames(windows))))
		return
	}
	if !declared.Budgetable {
		// The window exists and still cannot be a denominator. Refused HERE, at
		// the boundary that admits the input, with the store's own reason
		// (drain r1 D1).
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			"lane %q cannot carry a budget on its %q window: %s", lane, window, declared.NotBudgetable)))
		return
	}
	now := time.Now().UTC()
	start := now
	if v := strings.TrimSpace(body.PeriodStart); v != "" {
		t, perr := time.Parse(time.RFC3339, v)
		if perr != nil {
			s.writeSurface(w, nil, badRequest(fmt.Sprintf("bad period_start %q: want an RFC3339 timestamp", v)))
			return
		}
		start = t.UTC()
	}

	var rec PlanBudgetRecord
	if body.FromProposal {
		if declared.AllowanceUnverified {
			// There is nothing to take a fraction OF. Proposing from a window
			// whose allowance nobody published would mint the denominator out
			// of thin air, which is worse than having none: an operator would
			// accept a number that looks sourced (S10.4/D4). It is the caller's
			// answer, not a platform fault.
			s.writeSurface(w, nil, badRequest(fmt.Sprintf(
				"lane %q publishes no allowance for window %q at primary-source grade, so no budget can be proposed "+
					"from it — declare a figure from your own account console instead (P-T17-3)", lane, window)))
			return
		}
		proposed, perr := s.planBudgets.ProposePlanBudget(r.Context(), owner, lane, window, start)
		if perr != nil {
			s.writeSurface(w, nil, fmt.Errorf("propose plan budget: %w", perr))
			return
		}
		rec = proposed
		rec.DeclaredTS, rec.DeclaredBy = now, scope.UserID
	} else {
		switch {
		case body.PeriodUnits <= 0:
			s.writeSurface(w, nil, badRequest(fmt.Sprintf(
				`bad "period_units" %v: a plan budget is a positive figure in %s, this window's own unit. To take the `+
					`platform's proposal instead, send {"from_proposal": true}. To stop automation entirely, pause it `+
					`(POST /api/meters/pause)`, body.PeriodUnits, declared.Unit)))
			return
		case body.PeriodHours <= 0:
			s.writeSurface(w, nil, badRequest(fmt.Sprintf(
				`bad "period_hours" %v: a period is a start plus a length, and this window is %v hours long`,
				body.PeriodHours, declared.WindowHours)))
			return
		}
		rec = PlanBudgetRecord{
			Owner: owner, Lane: lane, Window: window,
			PeriodUnits: body.PeriodUnits, Unit: declared.Unit,
			PeriodStart: start, PeriodHours: body.PeriodHours,
			Source:     planBudgetOperatorSet,
			DeclaredTS: now, DeclaredBy: scope.UserID,
		}
	}

	// A period that has ALREADY ELAPSED is dead on arrival: the reading refuses
	// it the instant it is stored, so accepting it would answer 200 for a
	// budget that denominates nothing (drain r2 R4).
	ends := rec.PeriodStart.Add(time.Duration(rec.PeriodHours * float64(time.Hour)))
	if !ends.After(now) {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			"that period is already over: a %v-hour period starting %s ended at %s. A budget is declared for a period "+
				"that is still running, and re-declaring is what starts the next one — nothing rolls one over (S10.4)",
			rec.PeriodHours, rec.PeriodStart.UTC().Format(time.RFC3339), ends.UTC().Format(time.RFC3339))))
		return
	}

	prior, existed, err := s.planBudgets.DeclarePlanBudget(r.Context(), rec)
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("declare plan budget: %w", err))
		return
	}
	// The audit row lands AFTER the store act (the part-A drain-D6 principle): a
	// record of an edit that never committed would be a record of something that
	// did not happen.
	// The detail says when this budget STOPS applying, because for the shortest
	// window that is hours away and it is by design: a rolling-5h budget goes
	// inapplicable five hours after it is declared, and a lane silently falling
	// off pressure routing is the surprise this sentence exists to prevent
	// (drain r2 R4).
	out := PlanBudgetDeclared{Budget: rec,
		Detail: fmt.Sprintf("declared: the S10.4 tier-3 reading measures this person's %s consumption in the %s window "+
			"against it immediately, in %s. The period runs %v hours and ends at %s — after that the reading stops "+
			"applying and this lane returns to the deterministic routing order until you declare the next period; "+
			"nothing rolls one over. Nothing is enforced by the budget either: it is what makes the lane's pressure "+
			"comparable when routing picks between flat-rate lanes (S08.8)",
			lane, window, rec.Unit, rec.PeriodHours, ends.UTC().Format(time.RFC3339))}
	if existed {
		out.Prior = &prior
	}
	if err := s.recordDecision(r.Context(), decisionPayload{
		Actor: scope.UserID, ActorIsOperator: scope.Operator,
		CardID:   cardTypePlanBudget + ":" + owner + ":" + lane + ":" + window,
		CardType: cardTypePlanBudget,
		Decision: "declare", Subject: owner + ":" + lane + ":" + window, Reason: body.Reason,
		Old: planBudgetSnapshot(prior, existed), New: planBudgetSnapshot(rec, true),
		Ref: "S10.4 operator-declared automation budget, in the plan's own units; S18.3 per-person automation budgets",
	}, owner, now); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	s.writeReadJSON(w, out)
}

// planBudgetOperatorSet is the provenance of a figure the operator declared
// themselves, as this transport speaks it. The vocabulary itself is the store's
// and is CHECK-constrained there; this is the one value the verb produces.
const planBudgetOperatorSet = "operator-set"

func planWindowNamed(windows []PlanWindowRecord, name string) (PlanWindowRecord, bool) {
	for _, w := range windows {
		if w.Name == name {
			return w, true
		}
	}
	return PlanWindowRecord{}, false
}

func planWindowNames(windows []PlanWindowRecord) string {
	names := make([]string, 0, len(windows))
	for _, w := range windows {
		names = append(names, fmt.Sprintf("%q (in %s, over %v hours)", w.Name, w.Unit, w.WindowHours))
	}
	return strings.Join(names, ", ")
}

// planBudgetSnapshot renders one side of the old→new audit. An absent prior is
// the explicit "undeclared" object rather than a missing member: "there was no
// budget" is a fact about the edit, not a gap in the record.
func planBudgetSnapshot(rec PlanBudgetRecord, declared bool) json.RawMessage {
	if !declared {
		return json.RawMessage(`{"declared":false}`)
	}
	b, err := json.Marshal(struct {
		Declared    bool    `json:"declared"`
		Lane        string  `json:"lane"`
		Window      string  `json:"window"`
		PeriodUnits float64 `json:"period_units"`
		Unit        string  `json:"unit"`
		PeriodStart string  `json:"period_start"`
		PeriodHours float64 `json:"period_hours"`
		Source      string  `json:"source"`
	}{true, rec.Lane, rec.Window, rec.PeriodUnits, rec.Unit,
		rec.PeriodStart.UTC().Format(time.RFC3339Nano), rec.PeriodHours, rec.Source})
	if err != nil {
		return nil
	}
	return b
}

// ── POST /api/meters/pause — pause / resume my automation ───────────────────

type pauseBody struct {
	Person string `json:"person,omitempty"`
	// Paused is the switch's new position. It is required and has no default:
	// a request that did not say which way it wanted the switch is not a
	// request this verb can guess at.
	Paused *bool  `json:"paused"`
	Reason string `json:"reason,omitempty"`
}

// AutomationPause is the switch's state after the flip.
type AutomationPause struct {
	Owner  string `json:"owner"`
	Paused bool   `json:"paused"`
	// Changed is false when the switch was already in the requested position —
	// a repeat is honest about having changed nothing.
	Changed bool   `json:"changed"`
	Detail  string `json:"detail"`
}

func (s *Server) handlePauseSet(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	if s.pause == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S10.4 pause switch is not wired in this process"})
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body pauseBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	if body.Paused == nil {
		s.writeSurface(w, nil, badRequest(`missing "paused": say true to stop admitting your automation, false to let it resume`))
		return
	}
	scope := s.readScope(r)
	owner := strings.TrimSpace(body.Person)
	if owner == "" {
		owner = scope.UserID
	}
	// Same authority as the budget edit (OQ4): own + operator-any, every flip
	// audited with its actor.
	if owner != scope.UserID && !scope.Operator {
		s.writeSurface(w, nil, &SurfaceError{Status: http.StatusForbidden, Code: "forbidden",
			Msg: "pausing another person's automation is the operator's (D10)"})
		return
	}
	if !s.requirePerson(w, r, owner) {
		return
	}
	prior, err := s.pause.SetPause(r.Context(), owner, *body.Paused)
	if err != nil {
		s.writeSurface(w, nil, fmt.Errorf("set automation pause: %w", err))
		return
	}
	out := AutomationPause{Owner: owner, Paused: *body.Paused, Changed: prior != *body.Paused}
	if *body.Paused {
		out.Detail = "paused: no background work is admitted for this person, and an in-flight run parks at its next stage-session boundary. Nothing queued or parked is discarded, and interactive work is not gated at all (S10.4)"
	} else {
		out.Detail = "resumed: background admission is open again and the preserved queue proceeds. A run that parked while paused is released by its own resume (S14.4)"
	}
	if err := s.recordDecision(r.Context(), decisionPayload{
		Actor: scope.UserID, ActorIsOperator: scope.Operator,
		CardID: cardTypePause + ":" + owner, CardType: cardTypePause,
		Decision: pauseDecision(*body.Paused), Subject: owner, Reason: body.Reason,
		Old: pauseSnapshot(prior), New: pauseSnapshot(*body.Paused),
		Ref: "S10.4 pause-my-automation",
	}, owner, time.Now().UTC()); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	s.writeReadJSON(w, out)
}

func pauseDecision(paused bool) string {
	if paused {
		return "pause"
	}
	return "resume"
}

func pauseSnapshot(paused bool) json.RawMessage {
	if paused {
		return json.RawMessage(`{"automation_paused":true}`)
	}
	return json.RawMessage(`{"automation_paused":false}`)
}

// ── POST /api/tasks/{task}/priority-hint — the S15.5 board drag ─────────────

type hintBody struct {
	// Rank orders the caller's own same-class queued work: lower is claimed
	// earlier, 0 is no hint.
	Rank   int64  `json:"rank"`
	Reason string `json:"reason,omitempty"`
}

// PriorityHint is the drag's outcome.
type PriorityHint struct {
	TaskID string `json:"task_id"`
	Rank   int64  `json:"rank"`
	// Runs are the queued runs the hint landed on. Empty means the board the
	// person dragged was stale.
	Runs []string `json:"runs"`
	// Applied is false on a stale board: the task has no queued run any more.
	Applied bool   `json:"applied"`
	Detail  string `json:"detail"`
}

func (s *Server) handlePriorityHint(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	if s.hints == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the scheduler is not wired in this process, so a priority hint has no queue to reorder"})
		return
	}
	taskID := r.PathValue("task")
	var owner string
	err := s.proj.db.QueryRowContext(r.Context(), `SELECT user_id FROM tasks WHERE task_id = ?`, taskID).Scan(&owner)
	found := !errors.Is(err, sql.ErrNoRows)
	if err != nil && found {
		s.writeSurface(w, nil, fmt.Errorf("read task owner: %w", err))
		return
	}
	scope := s.readScope(r)
	// S15.5 says "one's OWN queued tasks", and it means it: the operator is not
	// excepted here. Reordering somebody else's queue is not an administrative
	// act with a broader scope, it is a different act entirely — the operator
	// still sees every board, and a task they do not own is theirs to read, not
	// to reorder. 404-before-403 stays: an unknown id is never an existence
	// oracle.
	if code, cerr := authorizeOwner(ownerScope{UserID: scope.UserID}, owner, found, "task"); cerr != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: code, Code: httpCode(code), Msg: cerr.Error()})
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body hintBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	if body.Rank < -hintRankBound || body.Rank > hintRankBound {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			"bad rank %d: the drag hint is an ordering position within ±%d (S15.5)", body.Rank, hintRankBound)))
		return
	}
	runs, err := s.proj.queuedRunsOfTask(r.Context(), taskID)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	out := PriorityHint{TaskID: taskID, Rank: body.Rank, Runs: []string{}}
	for _, runID := range runs {
		applied, err := s.hints.SetPriorityHint(r.Context(), runID, body.Rank)
		if err != nil {
			s.writeSurface(w, nil, fmt.Errorf("set priority hint: %w", err))
			return
		}
		if applied {
			out.Runs, out.Applied = append(out.Runs, runID), true
		}
	}
	if out.Applied {
		out.Detail = "hint recorded: it breaks ties among your own same-class queued work. It never outranks the workload class ladder and never reaches another person's queue (S15.5; S10.7)"
	} else {
		// The honest stale board: the work moved on between the render and the
		// drag. Not an error — an error here would imply the drag had an
		// authority it never had over a run that is no longer queued.
		out.Detail = "nothing to reorder: this task has no queued run any more (it was claimed, finished or cancelled), so the board you dragged is stale"
	}
	if err := s.recordDecision(r.Context(), decisionPayload{
		Actor: scope.UserID, ActorIsOperator: scope.Operator,
		CardID: cardTypeHint + ":" + taskID, CardType: cardTypeHint,
		Decision: "reorder", Subject: taskID, Reason: body.Reason,
		New: hintSnapshot(body.Rank, out.Applied),
		Ref: "S15.5 board-drag priority hint",
	}, owner, time.Now().UTC()); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	s.writeReadJSON(w, out)
}

func hintSnapshot(rank int64, applied bool) json.RawMessage {
	b, err := json.Marshal(struct {
		HintRank int64 `json:"hint_rank"`
		Applied  bool  `json:"applied"`
	}{rank, applied})
	if err != nil {
		return nil
	}
	return b
}

// personExists is the boundary check both S10.4 switches share (drain D5/D6).
//
// `person` is CLIENT INPUT, and whether it names somebody is a question about
// that input — so it is answered here, at the boundary, and answered 404 (the
// part-A drain-D7 principle: a caller's bad input must not surface as a platform
// fault). Without it an operator typo reached the store, which refuses too, but
// as an error the transport could only render as a 500.
func (s *Server) personExists(ctx context.Context, userID string) (bool, error) {
	var exists int
	switch err := s.proj.db.QueryRowContext(ctx,
		`SELECT 1 FROM users WHERE user_id = ?`, userID).Scan(&exists); {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("read person %q: %w", userID, err)
	}
	return true, nil
}

// requirePerson resolves the switch's subject or writes the refusal.
func (s *Server) requirePerson(w http.ResponseWriter, r *http.Request, owner string) bool {
	ok, err := s.personExists(r.Context(), owner)
	if err != nil {
		s.writeSurface(w, nil, err)
		return false
	}
	if !ok {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusNotFound, Code: "not_found",
			Msg: fmt.Sprintf("no such person %q: a budget or a pause is declared for somebody", owner)})
		return false
	}
	return true
}

// queuedRunsOfTask lists the task's runs that are still QUEUED — the only runs a
// drag can order, because a claimed run has left the queue the hint sorts.
func (p *projector) queuedRunsOfTask(ctx context.Context, taskID string) ([]string, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT run_id FROM runs WHERE task_id = ? AND state = 'queued' ORDER BY created_ts, run_id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("projection: queued runs of task %q: %w", taskID, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("projection: scan queued run: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

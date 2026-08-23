package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/history"
)

// meters.go is the S15.2 `meters` read family: consumption, pressure, budgets,
// burn rates and limit-event status per (person, lane, period).
//
// THE RULE THIS FILE IS BUILT AROUND: no money is computed here. Every dollar
// figure served is READ — from the S10.4 gauge through the MeterReader seam, or
// from the migration-0016 Layer-0 views through internal/history's own verbs.
// internal/api owns no rate, no price table, no token→currency arithmetic and
// no re-grained division (S14.10 ¶1; §11 metering-is-a-read-model). The R8 test
// asserts it with a probe that proves the scan can fail.
//
// The v0 budget posture is an HONEST ABSENCE, not a zero: no operator budget is
// persisted anywhere (metering.Budget is a caller-supplied value object), so
// pressure is reported not-applicable and the absence REASON is read from the
// cost_budget_remainder view rather than restated here. Budget persistence
// arrives with its edit verb (B6-2), not with this read.

// MeterLane is one (person, lane) meter reading. WeightedConsumption is ALWAYS
// real; Pressure exists only when a budget makes it meaningful. There is no
// percent/fraction/ratio/ETA field — pressure is a declared S10.4 quantity
// against a declared denominator, and when there is no denominator there is no
// number (§30 R10).
type MeterLane struct {
	Owner string `json:"owner"`
	Lane  string `json:"lane"`
	// WeightedConsumption is the S10.4 gauge figure: period tokens with cache
	// reads down-weighted. Always available, budget or no budget.
	WeightedConsumption float64 `json:"weighted_consumption"`
	// CacheReadWeight + Assumed carry the S10.4 "assumed" label (G1 Def.10) —
	// the single most impactful gauge assumption, stated on every reading.
	CacheReadWeight float64 `json:"cache_read_weight"`
	Assumed         bool    `json:"assumed"`
	// PressureApplicable is the gauge's own Applicable bit; Pressure is nil
	// unless it is true. A fabricated denominator would be worse than no
	// number (D4).
	PressureApplicable bool     `json:"pressure_applicable"`
	Pressure           *float64 `json:"pressure"`
	BudgetDeclared     bool     `json:"budget_declared"`
	BudgetRemaining    *float64 `json:"budget_remaining"`

	TotalRuns  int64 `json:"total_runs"`
	ActiveRuns int64 `json:"active_runs"`
	ParkedRuns int64 `json:"parked_runs"`
	// ParkedUntil is the lane's current park horizon, derived from the latest
	// limit marker on its parked runs ("parked until…", S15.5 fleet). nil when
	// nothing in the lane is parked or no marker carries a reset time.
	ParkedUntil *string `json:"parked_until"`

	// Tier stamps the approximation tier of the token figures above (S10.1).
	Tier int `json:"tier,omitempty"`
	// Plan is the lane's tier-3 plan-unit reading when its plan has one.
	// Omitted entirely for lanes without a flat plan — an absent reading is
	// absent, never a zero (S10.1).
	Plan *MeterLanePlan `json:"plan,omitempty"`
}

// MeterLanePlan is a flat plan's own-unit reading as the meters read serves it
// (S10.1 tier 3; S10.4). Separate from the token figures, in its own unit, at
// its own tier — the two are never added.
type MeterLanePlan struct {
	Unit             string   `json:"unit"`
	Tier             int      `json:"tier"`
	Assumed          bool     `json:"assumed"`
	AssumedNote      string   `json:"assumed_note,omitempty"`
	Consumed         float64  `json:"consumed"`
	Calls            int64    `json:"calls"`
	Multiplier       float64  `json:"multiplier"`
	MultiplierWindow string   `json:"multiplier_window,omitempty"`
	Pressure         *float64 `json:"pressure"`
	BudgetDeclared   bool     `json:"budget_declared"`
	SeedAllowance    float64  `json:"seed_allowance,omitempty"`
	SeedQuota        string   `json:"seed_quota,omitempty"`
	VerifiedOn       string   `json:"verified_on,omitempty"`
}

// AutomationState is one person's S10.4 pause switch as this read serves it:
// the position the switch is IN, at the grain the switch is set at (per person,
// never per lane — pausing is not a lane act).
type AutomationState struct {
	Owner  string `json:"owner"`
	Paused bool   `json:"paused"`
}

// AutomationView is the pause switch's current position for the people this
// read is already about. It mirrors MeterView's shape deliberately: when the
// store is not wired the position is UNKNOWN, and an unknown switch renders as
// an absence with its reason rather than as an unpaused one — "not paused" is a
// reading, and a reading nobody took is the one thing this surface may not
// serve (§42).
type AutomationView struct {
	States []AutomationState `json:"states,omitempty"`
	Absent string            `json:"absent,omitempty"`
}

// MeterView is one Layer-0 view served through internal/history's own verb, so
// the figures arrive at the view's OWN grain and are never re-grained here. An
// unavailable surface is an absence with its reason, never an empty table
// pretending to be an answer.
type MeterView struct {
	Answer *history.Answer `json:"answer,omitempty"`
	Absent string          `json:"absent,omitempty"`
}

// Meters is the S15.2 meters resource.
type Meters struct {
	Lanes []MeterLane `json:"lanes"`
	// Automation is the S10.4 pause switch's CURRENT position per person, read
	// from the same store the pause verb writes. It is here because the switch
	// is a meters-family fact (S15.2 lists "pause my automation" on this family)
	// and because a control that can only report what it just did cannot tell a
	// person what is true now.
	Automation AutomationView `json:"automation"`
	// BurnRates serves cost_burn_rate rows VERBATIM at the view's own grain,
	// which is per PERSON (usd per observed day). It is deliberately not
	// re-grained onto (owner, lane): dividing a person's rate across their
	// lanes would be arithmetic over money, and this surface reads money.
	BurnRates MeterView `json:"burn_rates"`
	// Budgets is the cost_budget_remainder posture — one row per person,
	// carrying budget_declared, remainder_status and the absence_reason.
	Budgets     MeterView `json:"budgets"`
	LimitEvents MeterView `json:"limit_events"`
	PerPerson   MeterView `json:"per_person"`
	PerPeriod   MeterView `json:"per_period"`
	Cursor      int64     `json:"cursor"`
}

// GET /api/meters — consumption, pressure, budgets, burn rates and limit-event
// status, owner-scoped (S01.9: a member reads their own meters; the operator
// reads everyone's).
func (s *Server) handleMeters(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	scope := s.readScope(r)
	person, err := readPerson(r, scope)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	limit, err := readRawLimit(r)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	out := Meters{Lanes: []MeterLane{}}
	if out.Cursor, err = s.head(r.Context()); err != nil {
		s.writeSurface(w, nil, fmt.Errorf("read head cursor: %w", err))
		return
	}
	if out.Lanes, err = s.proj.meterLanes(r.Context(), scope, person, strings.TrimSpace(r.URL.Query().Get("lane"))); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	out.Automation = s.automationStates(r.Context(), scope, person, out.Lanes)
	hs := history.Scope{Operator: scope.Operator, UserID: scope.UserID}
	out.BurnRates = s.meterView(r.Context(), "cost_burn_rate", hs, limit)
	out.Budgets = s.meterView(r.Context(), "cost_budget_remainder", hs, limit)
	out.LimitEvents = s.meterView(r.Context(), "limit_event_history", hs, limit)
	out.PerPerson = s.meterView(r.Context(), "cost_per_person", hs, limit)
	out.PerPeriod = s.meterView(r.Context(), "cost_per_period", hs, limit)

	// Redacted at the edge: limit_event_history lifts fields straight out of
	// run_events payloads, which is payload content on a REST response (R20).
	s.writeReadRedacted(w, out)
}

// automationStates reads the S10.4 pause switch for the people this response is
// already about, through the SAME PauseStore the pause verb writes.
//
// SCOPE IS THE LANES' OWN and is not widened by a character: the subjects are
// the owners `meterLanes` just served under the caller's scope — a member's own
// id, or the operator's whole (or `?person=`-narrowed) set — plus the CALLER
// themselves whenever the request is not narrowed to somebody else. That one
// addition is not a scope change: a member's entire scope IS their own id, and
// the operator may read anybody's. It is here because a person who has run
// nothing has no lane row to carry their switch, and a switch you cannot see is
// a switch you cannot honestly be offered.
//
// A store that fails is an ABSENCE with the failure's own reason. Reporting
// "not paused" because the read broke would be the one answer this switch must
// never give.
func (s *Server) automationStates(ctx context.Context, scope ownerScope, person string, lanes []MeterLane) AutomationView {
	if s.pause == nil {
		return AutomationView{Absent: "the S10.4 pause switch is not wired in this process, so its current position cannot be read here"}
	}
	owners := []string{}
	seen := map[string]bool{}
	if person == "" || person == scope.UserID {
		owners, seen[scope.UserID] = append(owners, scope.UserID), true
	}
	for _, l := range lanes {
		if !seen[l.Owner] {
			seen[l.Owner] = true
			owners = append(owners, l.Owner)
		}
	}
	states := make([]AutomationState, 0, len(owners))
	for _, o := range owners {
		paused, err := s.pause.Paused(ctx, o)
		if err != nil {
			s.logger.Warn("meters: read pause switch", "owner", o, "err", err)
			return AutomationView{Absent: "the S10.4 pause switch could not be read: " + err.Error()}
		}
		states = append(states, AutomationState{Owner: o, Paused: paused})
	}
	return AutomationView{States: states}
}

// meterView routes ONE Layer-0 view read. It builds no SQL, applies no clamp of
// its own and interprets no row — the view name is a registry constant and the
// store does the rest.
func (s *Server) meterView(ctx context.Context, view string, scope history.Scope, limit int) MeterView {
	if s.history == nil {
		return MeterView{Absent: "the S14.10 query surface is not wired in this process, so the Layer-0 " + view + " read is unavailable"}
	}
	a, err := s.history.SelectView(ctx, view, scope, limit)
	if err != nil {
		s.logger.Warn("meters: select view", "view", view, "err", err)
		return MeterView{Absent: err.Error()}
	}
	return MeterView{Answer: &a}
}

// meterLanes reads the (person, lane) grain from the runs table and folds the
// S10.4 gauge onto each row through the metering seam.
func (p *projector) meterLanes(ctx context.Context, scope ownerScope, person, lane string) ([]MeterLane, error) {
	q := `SELECT user_id, lane, COUNT(*),
	             SUM(CASE WHEN state IN (` + placeholders(len(activeStates)) + `) THEN 1 ELSE 0 END),
	             SUM(CASE WHEN state = 'parked' THEN 1 ELSE 0 END)
	        FROM runs WHERE 1 = 1`
	args := append([]any{}, activeStates...)
	if !scope.Operator {
		q += ` AND user_id = ?`
		args = append(args, scope.UserID)
	}
	if person != "" {
		q += ` AND user_id = ?`
		args = append(args, person)
	}
	if lane != "" {
		q += ` AND lane = ?`
		args = append(args, lane)
	}
	q += ` GROUP BY user_id, lane ORDER BY lane, user_id`

	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("projection: meter lanes: %w", err)
	}
	out := []MeterLane{}
	for rows.Next() {
		var l MeterLane
		if err := rows.Scan(&l.Owner, &l.Lane, &l.TotalRuns, &l.ActiveRuns, &l.ParkedRuns); err != nil {
			rows.Close()
			return nil, fmt.Errorf("projection: meter lane scan: %w", err)
		}
		out = append(out, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// The gauge reads and the park derives run after the grouping cursor is
	// closed — one connection (S02.1), the fleet-projection precedent.
	for i := range out {
		if p.meter != nil {
			if m, merr := p.meter.LaneMeter(ctx, out[i].Owner, out[i].Lane); merr == nil {
				out[i].WeightedConsumption = m.WeightedConsumption
				out[i].CacheReadWeight = m.CacheReadWeight
				out[i].Assumed = m.Assumed
				out[i].BudgetDeclared = m.BudgetDeclared
				out[i].PressureApplicable = m.Utilization != nil
				out[i].Pressure = m.Utilization
				out[i].BudgetRemaining = m.BudgetRemaining
				out[i].Tier = m.Tier
				if m.Plan != nil {
					out[i].Plan = &MeterLanePlan{
						Unit: m.Plan.Unit, Tier: m.Plan.Tier,
						Assumed: m.Plan.Assumed, AssumedNote: m.Plan.AssumedNote,
						Consumed: m.Plan.Consumed, Calls: m.Plan.Calls,
						Multiplier: m.Plan.Multiplier, MultiplierWindow: m.Plan.MultiplierWindow,
						Pressure: m.Plan.Pressure, BudgetDeclared: m.Plan.BudgetDeclared,
						SeedAllowance: m.Plan.SeedAllowance, SeedQuota: m.Plan.SeedQuota,
						VerifiedOn: m.Plan.VerifiedOn,
					}
				}
			}
		}
		if out[i].ParkedRuns > 0 {
			out[i].ParkedUntil = p.laneParkedUntil(ctx, out[i].Owner, out[i].Lane)
		}
	}
	return out, nil
}

// laneParkedUntil derives the lane's park horizon from the latest limit marker
// on its most recently updated parked run — the same derive the run card runs,
// read at the lane grain. It is a timestamp, not a figure: nothing here is
// computed, and an absent marker stays absent.
func (p *projector) laneParkedUntil(ctx context.Context, owner, lane string) *string {
	var runID string
	if err := p.db.QueryRowContext(ctx,
		`SELECT run_id FROM runs WHERE user_id = ? AND lane = ? AND state = 'parked'
		  ORDER BY updated_ts DESC, run_id DESC LIMIT 1`, owner, lane).Scan(&runID); err != nil {
		return nil
	}
	pay, ok := p.latestPayload(ctx, runID, "limit.event", "engine.rate_limit", "run.parked")
	if !ok {
		return nil
	}
	if v := firstString(pay, "parked_until", "parked-until", "resets_at", "resets-at", "reset_at"); v != "" {
		return &v
	}
	return nil
}

// readRawLimit parses `?limit=` WITHOUT clamping: the query package owns its
// own bounds (history.DefaultRowLimit / MaxRowLimit) and the transport passes
// the caller's number through them rather than second-guessing it. 0 means
// unset, which the package reads as its default.
func readRawLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return 0, nil
	}
	n, err := readLimitValue(raw)
	if err != nil {
		return 0, err
	}
	return n, nil
}

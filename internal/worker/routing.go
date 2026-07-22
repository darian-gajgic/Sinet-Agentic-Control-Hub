package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/sandbox"
)

// routing.go — Spec S08.8: worker/model/lane routing as a visible,
// overridable, DETERMINISTIC classifier (2.3, 7.7). The selection pipeline
// is deterministic first and all free-tier [R15 §4.7]:
//
//  1. Selector match over the template registry (task selectors + FTS5 over
//     delegation descriptions; filtered by status=active, domain, kind,
//     required grants ⊆ granted, confinement equal-or-tighter).
//  2. Tie-break among candidates by a local duty alias with a one-line
//     reason (Spec S12 seam — the local tier lands at B4; the absent duty
//     DEGRADES to a deterministic order with the absence in the reason,
//     never faked).
//  3. Model + effort: the template's execution profile (duty class + effort
//     floor) resolves against the requester's duty maps and the task's
//     effort mode; SUBSCRIPTION COVERAGE BINDS EVERY CHOICE — only models
//     the owner holds flat-rate, or the local tier; a metered model only
//     under an explicit 3.10 flag, and the metered-exception list is EMPTY
//     at v0 (G1 P7). Among flat-rate lanes selection uses consumption
//     pressure, NEVER dollars (D5) — no price, cost, or dollar figure
//     enters this package's selection inputs, structurally.
//  4. Research nodes route to a search-capable lane.
//  5. Helpers ride this same pipeline with the spawn trigger as an input
//     (the S04 boundary); mechanical helper duties prefer the local lane —
//     its engine lane has no v0 consumer (S12.1 class (a), B4-5), so the
//     dispatch degrades to the paid seat with a recorded reason.
//
// No trained router exists at household n and no silent switching, ever
// (14.3): every decision carries a plain-language reason, appears on the
// plan/approval card pre-execution, and is overridable there; overrides are
// recorded with their actor (consumers own the surfacing: intake the card,
// stage the routing.decided event per run).

// EventRoutingDecided is the settled per-run routing accountability event
// (Spec S08.8, 7.7, S2.6; R12 §4.1 — "settled", so the name is spec-fixed,
// not provisional): {cause, score, signals, worker + version, model, lane,
// effort, plain_reason}. Worker + version per run is the version→outcome
// join key (Spec S08.4).
const EventRoutingDecided = "routing.decided"

// EventWorkerCompiled records one per-spawn compiled unit on the run (Spec
// S08.3: the compiled artifact is hashed as one unit "and recorded on the
// run"); name provisional pending the S14 event contract (B5), extending
// the standing CONVENTIONS §7/§8 note.
const EventWorkerCompiled = "worker.compiled"

// RoutingSchemaVersion versions the routing.decided payload.
const RoutingSchemaVersion = 1

// Duty classes a template execution profile may name (Spec S08.1: "duty
// class + effort floor, lane-agnostic"). The maps referenced, never
// restated (Spec S08.8 boundary): planning (S06.10), judge = the ratified
// planning-model class (S07.5), utility (S06.10 — local, S12/B4), plus the
// S12.4 local duty aliases. Execution is the default worker seat.
const (
	DutyExecution = "execution"
	DutyPlanning  = "planning"
	DutyJudge     = "judge"
	DutyUtility   = "utility"
)

// Seat is one duty-map row: the concrete model a duty class resolves to on
// which lane, plus the model's context-window record (a MODEL FACT riding
// the seat row — the S05.3 stage-fit budget measures against it; this is
// where the B2-4 stage window constant retired to, per the B3-3 packet).
type Seat struct {
	Model        string `json:"model"`
	Lane         string `json:"lane"`
	WindowTokens int64  `json:"window_tokens"`
}

// DutyMap is the requester's duty map view (S06.10: per-person ceremony
// duty map with a uniform recommended default and per-user override; S08.8
// resolves execution profiles against it). At v0 the per-user override
// surface is not built (1.10 user surface, B6/v1) — the platform-wide
// recommended default below is the shipped map, and consumers treat the
// map as DATA.
type DutyMap map[string]Seat

// DefaultWindowTokens is the BUDGETING window a seat row carries: the
// number the S05.3 stage-fit machinery measures against (fit target and
// overflow threshold are fractions of it). 200k is the verified-safe floor
// across every seated model on this lane (haiku's hard window; the B2-4
// stage constant, retired here into seat-row data). The 1M-class models
// (opus-4-8, sonnet-5 — API-documented windows, live-verified 2026-07-22)
// deliberately keep the floor: UNDERSTATING a window only splits stages
// earlier (safe, and aligned with fresh-context-per-stage), while
// overstating one would disarm overflow protection entirely. Per-seat
// uplift is an S14 recalibration with a CLI-lane-measured window (B5).
const DefaultWindowTokens = 200_000

// DefaultDutyMap is the v0 recommended platform-wide duty map (S06.10
// "uniform recommended default"), seat mix RATIFIED at the B3 gate
// 2026-07-22 (operator D3; record + research grounding in
// P3/gates/B3-report.md §7): the advisor split. Planning rides
// claude-opus-4-8 — S06.10's "paid frontier-class" bar for
// interview/critique and S08.6's frontier-class composer ceremony.
// Execution rides claude-sonnet-5 — the ratified advisor-pattern default
// executor under an opus-class planner (also the subscription's separate
// sonnet weekly pool; a ratification fact, not runtime pricing). The V2
// judge rides claude-opus-4-8 — paid frontier-class per the S07.5 class
// bar, capability ≥ the executor, and deliberately a DIFFERENT model than
// the executor whose output it judges (cross-model judging; same-model
// judges prefer their own output). P-T06-5: the judge retarget IS a
// version bump — the golden-set re-run on the opus-4-8 judge is
// pre-registered at the B4 judge-calibration measurement row (B2-3
// deferral record); verdicts before that run are bring-up-grade. Gate
// rider: the serialize-by-deny E3 leg re-runs on this executor seat in
// the B4 battery (the B3-3 measurement ran on the superseded haiku
// default). The utility seat is deliberately ABSENT: S06.10 pins it to
// the local tier (S12, B4) — an absent duty degrades with a recorded
// reason, never fakes a local model onto a paid lane. No S18 key covers
// the map (the §7/§9/§11 constant precedent; the standing settings-tab
// directive applies). D5 unchanged: all seats subscription-covered,
// metered list EMPTY, selection never prices.
func DefaultDutyMap() DutyMap {
	return DutyMap{
		DutyExecution: Seat{Model: "claude-sonnet-5", Lane: "anthropic", WindowTokens: DefaultWindowTokens},
		DutyPlanning:  Seat{Model: "claude-opus-4-8", Lane: "anthropic", WindowTokens: DefaultWindowTokens},
		DutyJudge:     Seat{Model: "claude-opus-4-8", Lane: "anthropic", WindowTokens: DefaultWindowTokens},
	}
}

// Coverage is the subscription-coverage view binding every choice (Spec
// S08.8 step 3; Operating reality; D5): which lanes the owner holds
// flat-rate, and the metered-exception surface. The v0 shape is static
// configuration data — the observed per-account model-list diffing
// (P-T17-3) is S03.6 watch machinery (B5); gap advice consuming it is
// config-derived until then, and says so.
type Coverage struct {
	// FlatRateLanes are the lanes the owner holds flat-rate coverage on
	// (S03.2 lane names). Selection may choose only these plus the local
	// tier.
	FlatRateLanes []string
	// LocalAvailable: the S12 local tier is serving (B4). False at v0.
	LocalAvailable bool
	// MeteredAllowed reports whether a metered model is selectable under an
	// explicit 3.10 flag. The metered-exception list is EMPTY at v0 (G1
	// P7), so the production wiring always answers false; the hook exists
	// so the refusal is testable, not so it can pass.
	MeteredAllowed func(model string) bool
}

func (c Coverage) laneCovered(lane string) bool {
	for _, l := range c.FlatRateLanes {
		if l == lane {
			return true
		}
	}
	return false
}

// TieBreaker is the S08.8 step-2 seam: a local duty alias picks among
// multiple candidates with a one-line reason (Spec S12.4). The local tier
// lands at B4 — nil is the v0 posture and the router degrades to the
// deterministic order with the absence recorded in the reason (absent
// duties degrade, never faked).
type TieBreaker interface {
	Break(ctx context.Context, q RouteQuery, candidates []Candidate) (pick int, reason string, err error)
}

// PressureReader is the D5 lane-ordering input among multiple covered
// flat-rate lanes: consumption pressure, never dollars (Spec S08.8 step 3;
// S10 owns the gauge). Nil, or a single covered lane, short-circuits to
// the configured lane order.
type PressureReader interface {
	// Pressure returns a comparable consumption-pressure figure for the
	// owner on the lane (higher = more consumed).
	Pressure(ctx context.Context, owner, lane string) (float64, error)
}

// RouteQuery is one selection request. The inputs are PLATFORM facts (plan
// declarations, intake record, task text) — deterministic rule inputs,
// never agent-supplied lookups (Spec S08.1 selector discipline).
type RouteQuery struct {
	Requester string
	TaskID    string
	// RunID is the CONSUMING run of any local duty call the router makes (the
	// S12 tie-break D7 row, drain F2): intake-time routing rides the intake
	// run (<task>.intake); helper-spawn routing rides the coordinator's
	// execute run (per D6/§19). The caller sets it; empty falls back to the
	// intake run for the intake-time default.
	RunID string
	// TaskText is the request title+text — the FTS/trigger match input.
	TaskText string
	// Family is the S06 task family; Domain the verification domain the
	// family maps to (the stage layer owns the mapping).
	Family string
	Domain string
	// Kind selects the worker kind (agentic for engine work).
	Kind Kind
	// Classes are the plan's declared per-step confinement classes; the
	// loosest is the approved envelope a worker may not exceed (Spec S08.8:
	// "equal or tighter only").
	Classes []string
	// Tools are the plan-required tools (required grants ⊆ granted).
	Tools []string
	// Research: the plan carries research nodes (step 4 — search-capable
	// lane).
	Research bool
	// EffortMode is the task's effort mode ("" at v0 — the intake pipeline
	// declares none yet; effort-ladder mechanics are S10's).
	EffortMode string
	// SpawnTrigger marks helper routing (T-CTX | T-PAR | T-SPEC — the S04
	// boundary input); "" for task routing.
	SpawnTrigger string
	// Mechanical marks a mechanical helper duty (defaults to the local
	// lane, the permanent free tier — S08.8 step 5; R06 §4.5).
	Mechanical bool
}

// Candidate is one surviving selector-match candidate, surfaced on the
// plan card as a re-route target (Spec S08.8 "visible and overridable").
type Candidate struct {
	TemplateID string  `json:"template_id"`
	Name       string  `json:"name"`
	VersionID  string  `json:"version_id"`
	Score      float64 `json:"score"`
	Reason     string  `json:"reason"`
}

// Decision is one routing decision (the routing.decided payload shape,
// Spec S08.8 {cause, score, signals, worker + version, model, lane,
// effort, plain_reason}).
type Decision struct {
	Cause   string   `json:"cause"` // selector-match | no-fit-generalist | override | pinned | helper-spawn
	Score   float64  `json:"score,omitempty"`
	Signals []string `json:"signals,omitempty"`

	// Worker identity — empty for the generalist default. Worker + version
	// per run is the version→outcome join key (Spec S08.4).
	TemplateID   string `json:"worker,omitempty"`
	TemplateName string `json:"worker_name,omitempty"`
	VersionID    string `json:"version,omitempty"`
	Generalist   bool   `json:"generalist,omitempty"`

	Model        string `json:"model"`
	Lane         string `json:"lane"`
	Effort       string `json:"effort,omitempty"`
	WindowTokens int64  `json:"window_tokens"`

	// PlainReason is the plain-language reason (Spec S08.8: appears on the
	// plan/approval card; 7.7 accountability).
	PlainReason string `json:"plain_reason"`

	// Degraded: the task's domain lacks full verification maturity — the
	// generalist default is degraded-marked where applicable (Spec S08.8
	// no-fit stage 2; S08.7).
	Degraded bool `json:"degraded,omitempty"`

	// Candidates are the surviving alternatives (re-route targets on the
	// card; empty on helper routing).
	Candidates []Candidate `json:"candidates,omitempty"`

	// No-fit bookkeeping (Spec S08.8: "a gap record is written in every
	// case"; S08.6 compose-when-earned).
	GapSignature  string `json:"gap_signature,omitempty"`
	ComposeEarned bool   `json:"compose_earned,omitempty"`
	// GapAdvice is the 2.7 subscription-gap advice leg — set when the gap
	// is a MODEL, not a worker (a matched worker demanded an uncovered
	// model/lane). Config-derived at v0 (observed-list diffing is S03.6/B5
	// machinery) and says so.
	GapAdvice string `json:"gap_advice,omitempty"`
}

// Router is the S08.8 selection engine over the worker store.
type Router struct {
	Store    *Store
	DutyMap  DutyMap
	Coverage Coverage
	// TieBreak is the S12 local-duty seam (nil = absent at v0, degraded
	// deterministic order).
	TieBreak TieBreaker
	// Pressure is the D5 flat-lane ordering input (nil or single lane =
	// configured order).
	Pressure PressureReader
}

// Route runs the S08.8 pipeline. On no-fit it writes the gap record (Spec
// S08.8: "a gap record is written in every case") through the B3-2
// RecordGap verb and reports compose-when-earned.
func (r *Router) Route(ctx context.Context, q RouteQuery) (Decision, error) {
	if r.Store == nil {
		return Decision{}, fmt.Errorf("%w: router needs the worker store", ErrInvalid)
	}
	if q.Kind == "" {
		q.Kind = KindAgentic
	}

	signals := []string{"family=" + q.Family}
	if q.SpawnTrigger != "" {
		signals = append(signals, "spawn_trigger="+q.SpawnTrigger)
	}
	if q.Research {
		signals = append(signals, "research_nodes=present")
	}

	candidates, err := r.selectorMatch(ctx, q)
	if err != nil {
		return Decision{}, err
	}

	degraded, err := r.domainDegraded(ctx, q.Domain)
	if err != nil {
		return Decision{}, err
	}

	if len(candidates) == 0 {
		return r.noFit(ctx, q, signals, degraded)
	}

	// Step 2 — tie-break. A single candidate needs none; multiple go to the
	// S12 seam, degrading deterministically when it is absent (B4).
	pick := 0
	tieReason := ""
	if len(candidates) > 1 {
		if r.TieBreak != nil {
			i, reason, err := r.TieBreak.Break(ctx, q, candidates)
			if err != nil || i < 0 || i >= len(candidates) {
				return Decision{}, fmt.Errorf("worker: tie-break duty failed: %w", err)
			}
			pick, tieReason = i, reason
		} else {
			// Deterministic degraded order: candidates are already sorted
			// score-desc, name, id (selectorMatch) — the first wins.
			tieReason = "tie-break duty absent (local tier lands at B4, S12); deterministic order: score, then name"
		}
		signals = append(signals, "tie_break="+strings.ReplaceAll(tieReason, "\n", " "))
	}
	chosen := candidates[pick]

	// Step 3 — model + effort against the duty maps under subscription
	// coverage.
	v, def, err := r.Store.activeDefinition(ctx, mustTemplate(ctx, r.Store, chosen.TemplateID))
	if err != nil {
		return Decision{}, err
	}
	seat, effort, seatReason, gapAdvice, err := r.resolveSeat(ctx, q, def.Profile)
	if err != nil {
		return Decision{}, err
	}
	if gapAdvice != "" {
		// The matched worker demands an uncovered model — the gap is a
		// MODEL, not a worker (2.7): fall to no-fit with the advice leg.
		d, nerr := r.noFit(ctx, q, append(signals, "model_gap="+gapAdvice), degraded)
		if nerr != nil {
			return Decision{}, nerr
		}
		d.GapAdvice = gapAdvice
		d.PlainReason += " " + gapAdvice
		return d, nil
	}

	reason := fmt.Sprintf("Specialist %q matched: %s.", def.Name, chosen.Reason)
	if tieReason != "" {
		reason += " Tie-break: " + tieReason + "."
	}
	reason += " " + seatReason

	return Decision{
		Cause:        "selector-match",
		Score:        chosen.Score,
		Signals:      signals,
		TemplateID:   chosen.TemplateID,
		TemplateName: def.Name,
		VersionID:    v.ID,
		Model:        seat.Model,
		Lane:         seat.Lane,
		Effort:       effort,
		WindowTokens: seat.WindowTokens,
		PlainReason:  reason,
		Degraded:     degraded,
		Candidates:   candidates,
	}, nil
}

// mustTemplate re-reads a candidate's template row (candidates were just
// selected from live rows; a disappearing row is a real error surfaced by
// activeDefinition).
func mustTemplate(ctx context.Context, s *Store, id string) Template {
	t, err := s.Template(ctx, id)
	if err != nil {
		return Template{ID: id}
	}
	return t
}

// selectorMatch is step 1: deterministic selector matching plus the FTS5
// description leg, then the structural filters (status=active via the
// authoritative row join, kind, domain, grants ⊆ granted, confinement
// equal-or-tighter).
func (r *Router) selectorMatch(ctx context.Context, q RouteQuery) ([]Candidate, error) {
	rows, err := r.Store.db.QueryContext(ctx,
		`SELECT `+templateColumns+` FROM worker_templates WHERE status = 'active' AND kind = ?`, string(q.Kind))
	if err != nil {
		return nil, fmt.Errorf("worker: scan active templates: %w", err)
	}
	var active []Template
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		active = append(active, t)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	ftsRanks := map[string]float64{}
	if len(active) > 0 {
		hits, err := r.Store.searchDescriptions(ctx, q.TaskText)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			ftsRanks[h.TemplateID] = h.Rank
		}
	}

	planClass := loosestClass(q.Classes)
	taskLower := strings.ToLower(q.TaskText)

	var out []Candidate
	for _, t := range active {
		if q.Domain != "" && t.Domain != q.Domain {
			continue
		}
		v, def, err := r.Store.activeDefinition(ctx, t)
		if err != nil {
			// A template whose file is tampered/unreadable never routes; the
			// condition is loud at its own verbs — selection skips it.
			continue
		}
		g, err := r.Store.Guardrails(ctx, v.ID)
		if err != nil {
			continue
		}
		// Required grants ⊆ granted.
		if !subset(q.Tools, g.GrantedTools) {
			continue
		}
		// Confinement compatibility: equal or tighter only (S11 ladder).
		if planClass != "" && !classTighterOrEqual(g.Class, planClass) {
			continue
		}

		// Deterministic selector score: family match, trigger phrases,
		// task-class keys, FTS description rank.
		score := 0.0
		var why []string
		if def.Selectors.Family != "" && def.Selectors.Family == q.Family {
			score += 2
			why = append(why, "family selector "+q.Family)
		}
		for _, trig := range def.Selectors.Triggers {
			if trig != "" && containsWord(taskLower, strings.ToLower(trig)) {
				score++
				why = append(why, fmt.Sprintf("trigger %q", trig))
			}
		}
		for _, tc := range def.Selectors.TaskClasses {
			if tc != "" && containsWord(taskLower, strings.ToLower(tc)) {
				score++
				why = append(why, fmt.Sprintf("task class %q", tc))
			}
		}
		if rank, ok := ftsRanks[t.ID]; ok {
			// bm25 rank is negative-better in SQLite; fold a bounded
			// contribution so selector hits dominate description echoes.
			score += 0.5
			why = append(why, fmt.Sprintf("description match (rank %.2f)", rank))
		}
		if score == 0 {
			continue
		}
		out = append(out, Candidate{
			TemplateID: t.ID, Name: def.Name, VersionID: v.ID,
			Score: score, Reason: strings.Join(why, ", "),
		})
	}
	// Deterministic order: score desc, then name, then id.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].TemplateID < out[j].TemplateID
	})
	return out, nil
}

// resolveSeat resolves an execution profile against the duty map under
// subscription coverage (step 3). Returns a non-empty gapAdvice when the
// profile demands a model/lane the owner does not hold flat-rate — the 2.7
// model-gap leg (the metered path structurally refuses: the exception list
// is EMPTY at v0).
func (r *Router) resolveSeat(ctx context.Context, q RouteQuery, p ExecutionProfile) (Seat, string, string, string, error) {
	duty := p.Duty
	if duty == "" {
		duty = DutyExecution
	}

	// Mechanical helper duties default to the local lane — the permanent
	// free tier (S08.8 step 5). The consumer-class split binds (S12.1; §8
	// reading 3): the local tier's PLATFORM DUTY calls (class (b): intake
	// seams, tie-break) consume it directly through the S12.4 alias registry
	// (internal/local, B4-5), but the local ENGINE lane (class (a): a run
	// dispatching onto a local model via the opencode adapter) has NO v0
	// consumer in this cut — no second adapter exists. So a template/helper
	// demanding duty utility/mechanical (an engine dispatch) resolves to the
	// paid execution seat either way, never a fake dispatch onto a lane with
	// no adapter, never an error (absent duties degrade, never faked —
	// S08.8/CONVENTIONS §19).
	localNote := ""
	if q.Mechanical || duty == DutyUtility {
		if r.Coverage.LocalAvailable {
			localNote = fmt.Sprintf("Duty %q prefers the local free tier, which is serving; its engine lane carries no v0 consumer (S12.1 class (a) — platform duty calls ride it directly), so this dispatch rides the paid seat.", duty)
		} else {
			localNote = fmt.Sprintf("Duty %q prefers the local free tier, which is not configured (S12); riding the paid seat instead.", duty)
		}
		duty = DutyExecution
	}

	seat, ok := r.DutyMap[duty]
	if !ok {
		localNote = fmt.Sprintf("Duty %q has no seat in the v0 duty map; riding the execution seat.", duty) + " " + localNote
		seat, ok = r.DutyMap[DutyExecution]
		if !ok {
			return Seat{}, "", "", "", fmt.Errorf("%w: duty map has no execution seat", ErrInvalid)
		}
	}

	// A concrete model pin (recorded reason required at lint) overrides the
	// seat model — but coverage still binds.
	pinNote := ""
	if p.ModelPin != "" {
		if !r.modelCovered(p.ModelPin, seat.Lane) {
			advice := fmt.Sprintf("Subscription gap (2.7): the worker pins model %q, which no flat-rate lane covers "+
				"(config-derived; observed-list diffing arrives with the S03.6 watch, B5). "+
				"Options: run the generalist on the covered seat, or extend coverage.", p.ModelPin)
			return Seat{}, "", "", advice, nil
		}
		seat = Seat{Model: p.ModelPin, Lane: seat.Lane, WindowTokens: seat.WindowTokens}
		pinNote = fmt.Sprintf(" Model pinned by the template (%s).", p.ModelPinReason)
	}

	if !r.Coverage.laneCovered(seat.Lane) {
		if r.Coverage.MeteredAllowed != nil && r.Coverage.MeteredAllowed(seat.Model) {
			// Unreachable in production: the metered-exception list is EMPTY
			// at v0 (G1 P7). The branch exists so the refusal is testable.
			return Seat{}, "", "", "", fmt.Errorf("%w: metered selection is structurally disabled at v0 (D5/G1 P7)", ErrInvalid)
		}
		advice := fmt.Sprintf("Subscription gap (2.7): duty %q resolves to lane %q, which the owner does not hold flat-rate "+
			"(config-derived; observed-list diffing arrives with the S03.6 watch, B5).", duty, seat.Lane)
		return Seat{}, "", "", advice, nil
	}

	if seat.WindowTokens == 0 {
		seat.WindowTokens = DefaultWindowTokens
	}

	effort := p.EffortFloor
	reason := fmt.Sprintf("Runs on %s (%s lane) per the %s duty seat; flat-rate coverage bound (D5).", seat.Model, seat.Lane, duty)
	if effort != "" {
		reason += fmt.Sprintf(" Effort floor %s (ladder mechanics are S10's).", effort)
	}
	if q.Research {
		reason += " Research nodes route on the search-capable lane (S08.8 step 4)."
	}
	if localNote != "" {
		reason = localNote + " " + reason
	}
	reason += pinNote
	return seat, effort, reason, "", nil
}

// modelCovered: the model rides a covered flat-rate lane. v0 lane facts
// are configuration; a pinned model is covered when its lane is (per-model
// observed lists are P-T17-3/B5 machinery).
func (r *Router) modelCovered(model, lane string) bool {
	_ = model
	return r.Coverage.laneCovered(lane)
}

// noFit is the two-stage no-fit outcome (Spec S08.8): stage 1 (the task
// interpretation is confirmed) is the S06 restatement riding the same
// approval card; stage 2 is the one card offering run-as-generalist
// (default; degraded-marked where applicable) / compose-a-worker (when
// earned, S08.6) / subscription gap advice. A gap record is written in
// EVERY case.
func (r *Router) noFit(ctx context.Context, q RouteQuery, signals []string, degraded bool) (Decision, error) {
	seat, effort, seatReason, gapAdvice, err := r.resolveSeat(ctx, q, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		return Decision{}, err
	}
	if gapAdvice != "" {
		// Even the generalist seat is uncovered — surface the advice with
		// zero routing (nothing can run; the card carries the gap).
		seat = Seat{Model: "", Lane: "", WindowTokens: DefaultWindowTokens}
		seatReason = "No covered seat exists for the generalist default."
	}

	// A gap record is written in every case — but recurrence means DISTINCT
	// tasks (S08.6 "at ⚙ occurrences of a task family"): re-routing the
	// same task (approval-card rebuilds, re-plans) never increments the
	// counter, it re-reads the standing record.
	sig := GapSignature(q)
	rec, due, err := r.Store.gapForTask(ctx, sig, q.TaskID)
	if err != nil {
		return Decision{}, err
	}
	if rec == nil {
		fresh, freshDue, err := r.Store.RecordGap(ctx, sig, q.Family, q.TaskID)
		if err != nil {
			return Decision{}, err
		}
		rec, due = &fresh, freshDue
	}

	reason := "No specialist fits; running as generalist-with-injected-knowledge (the default for one-offs, S08.8)."
	if degraded {
		reason = "No specialist fits; running as generalist-with-injected-knowledge, DEGRADED-MARKED — the domain lacks a verified quality check (S08.7)."
	}
	if due {
		reason += fmt.Sprintf(" This task family has now recurred %d times — composing a specialist is EARNED (S08.6); a composition proposal is open.", rec.Occurrences)
	}
	reason += " " + seatReason

	return Decision{
		Cause:         "no-fit-generalist",
		Signals:       signals,
		Generalist:    true,
		Model:         seat.Model,
		Lane:          seat.Lane,
		Effort:        effort,
		WindowTokens:  seat.WindowTokens,
		PlainReason:   reason,
		Degraded:      degraded,
		GapSignature:  sig,
		ComposeEarned: due,
		GapAdvice:     gapAdvice,
	}, nil
}

// gapForTask reads the standing gap record when this task already refs the
// signature (nil, nil when the task is new to it — the caller then
// increments through RecordGap). Reported ComposeEarned for a re-read =
// "a proposal is live" (disposition proposed).
func (s *Store) gapForTask(ctx context.Context, signature, taskID string) (*GapRecord, bool, error) {
	var refsRaw, family, disp, lastSeen string
	var count int64
	err := s.db.QueryRowContext(ctx,
		`SELECT task_refs, family, occurrence_count, disposition, last_seen_ts FROM gap_records WHERE signature = ?`,
		signature).Scan(&refsRaw, &family, &count, &disp, &lastSeen)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("worker: read gap record: %w", err)
	}
	var refs []string
	if err := json.Unmarshal([]byte(refsRaw), &refs); err != nil {
		return nil, false, fmt.Errorf("worker: decode gap task refs: %w", err)
	}
	for _, ref := range refs {
		if ref == taskID && taskID != "" {
			rec := GapRecord{Signature: signature, Family: family, TaskRefs: refs,
				Occurrences: count, LastSeenTS: lastSeen, Disposition: GapDisposition(disp)}
			return &rec, rec.Disposition == GapProposed, nil
		}
	}
	return nil, false, nil
}

// domainDegraded reads the domains row (Spec S08.7); an ABSENT row is
// degraded — 2.1 maturity honesty (every domain outside the day-one rows
// enters degraded).
func (r *Router) domainDegraded(ctx context.Context, domain string) (bool, error) {
	if domain == "" {
		return true, nil
	}
	d, err := r.Store.Domain(ctx, domain)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return true, nil
		}
		return false, err
	}
	return d.Maturity == MaturityDegraded, nil
}

// GapSignature is the deterministic selector signature of a no-fit outcome
// (Spec S08.1 gap_records: {selector signature, family, task refs}).
func GapSignature(q RouteQuery) string {
	classes := append([]string(nil), q.Classes...)
	sort.Strings(classes)
	tools := append([]string(nil), q.Tools...)
	sort.Strings(tools)
	parts := []string{"family=" + q.Family, "domain=" + q.Domain,
		"classes=" + strings.Join(classes, "+"), "tools=" + strings.Join(tools, "+")}
	if q.Research {
		parts = append(parts, "research")
	}
	if q.Mechanical {
		parts = append(parts, "mechanical")
	}
	return strings.Join(parts, ";")
}

// loosestClass picks the loosest declared class (the plan's approved
// envelope) by S11 ladder rank.
func loosestClass(classes []string) string {
	best := ""
	bestRank := -1
	for _, c := range classes {
		if r, ok := sandbox.Class(c).LadderRank(); ok && r > bestRank {
			best, bestRank = c, r
		}
	}
	return best
}

// classTighterOrEqual: worker class ≤ plan class on the S11 ladder ("equal
// or tighter only", Spec S08.8 step 1). Unknown classes never pass.
func classTighterOrEqual(workerClass, planClass string) bool {
	w, okW := sandbox.Class(workerClass).LadderRank()
	p, okP := sandbox.Class(planClass).LadderRank()
	return okW && okP && w <= p
}

// TighterClass returns the tighter of a plan-step class and a worker's
// granted class (S11 ladder): a session never runs looser than the
// human-approved step declaration NOR the worker's approved guardrail
// class (Spec S08.8 equal-or-tighter; S08.2). Unknown ranks fall to the
// step class — the plan declaration is the approved envelope.
func TighterClass(stepClass, workerClass string) string {
	sr, okS := sandbox.Class(stepClass).LadderRank()
	wr, okW := sandbox.Class(workerClass).LadderRank()
	if !okS || !okW {
		return stepClass
	}
	if wr < sr {
		return workerClass
	}
	return stepClass
}

func subset(need, have []string) bool {
	set := make(map[string]bool, len(have))
	for _, h := range have {
		set[h] = true
	}
	for _, n := range need {
		if !set[n] {
			return false
		}
	}
	return true
}

// containsWord reports a word-boundary match of phrase inside text (both
// lowercased by the caller) — the §14 cue-phrase discipline.
func containsWord(text, phrase string) bool {
	idx := 0
	for {
		i := strings.Index(text[idx:], phrase)
		if i < 0 {
			return false
		}
		start := idx + i
		end := start + len(phrase)
		beforeOK := start == 0 || !isWordChar(text[start-1])
		afterOK := end == len(text) || !isWordChar(text[end])
		if beforeOK && afterOK {
			return true
		}
		idx = start + 1
		if idx >= len(text) {
			return false
		}
	}
}

func isWordChar(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_'
}

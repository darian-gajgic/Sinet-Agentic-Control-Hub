package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/sandbox"
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
//     A per-task PINNED LANE is the one sanctioned override of that
//     comparison [S00.9 A13]: RouteQuery.PinnedLane names a lane the
//     requester declared at task creation, and when it binds it REPLACES
//     the pressure comparison rather than adding an input to it. Coverage
//     still outranks it — a pin naming a lane the owner does not hold
//     flat-rate is REFUSED here, never degraded onto another lane, because
//     degrading is right for a lane the PLATFORM chose and wrong for one a
//     PERSON chose.
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

// ErrLanePinUnhonorable is SELECTION's own refusal of a per-task lane pin it
// cannot honor [S00.9 A13] — the third of the three layers, so a pin arriving
// by any route other than the boundary that admits one still cannot steer
// dispatch. Selection refuses here rather than routing somewhere else,
// because falling back to its own choice is exactly the silent substitution
// the pin exists to prevent (14.3: no silent switching, ever).
var ErrLanePinUnhonorable = fmt.Errorf("%w: lane pin cannot be honored", ErrInvalid)

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

// plainDuty says what a duty class IS, for the requester reading the
// selection's own sentence. The duty string itself is a machine token and
// stays one — it keys the duty map, rides the routing block's structured
// members and is what an operator configures; only the PROSE translates
// (P3-GF13). An unrecognised duty falls through as itself rather than being
// hidden: a name the requester cannot place still beats a silent omission.
func plainDuty(duty string) string {
	switch duty {
	case DutyExecution:
		return "doing the work"
	case DutyPlanning:
		return "working out the plan"
	case DutyJudge:
		return "checking the finished work"
	case DutyUtility:
		return "small internal chores"
	default:
		return duty
	}
}

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

// AlternateSeats are the additional flat-rate seats a duty may resolve to when
// the owner holds more than one flat-rate lane (S08.8 step 3). Keyed by duty
// class; the DutyMap seat is the configured first choice and these follow it in
// order.
//
// It is a second map rather than a longer Seat list on DutyMap because every
// consumer of DutyMap resolves a duty to exactly ONE seat, and widening that
// type would have made "which seat did this duty get" a question with a
// different answer in each caller.
type AlternateSeats map[string][]Seat

// LaneSeat is one lane's execution-seat facts as its commissioning document
// states them — the input AlternateSeatsFor turns into duty-map seats.
type LaneSeat struct {
	Lane  string
	Model string
	// WindowTokens is the model's context window when the provider publishes
	// one; 0 takes the platform floor.
	WindowTokens int64
}

// AlternateSeatsFor builds the alternate-seat map from commissioned lanes.
//
// EXECUTION ONLY, deliberately. Planning and judge keep their anthropic-only
// seats: the B3 gate ratified that seat mix from measured research, the judge
// seat additionally carries S07.5's capability-≥-executor bar AND the
// different-model-than-the-executor rule, and nobody has measured a second
// lane's models against either. Seating one would be inventing a ratification,
// so an owner holding only that lane gets the unchanged 2.7 subscription-gap
// advice for those duties — the honest answer.
//
// No model id appears in this package. Which model a lane fronts is the lane
// DOCUMENT's fact (S03.6), it carries its own verified-on date, and three of
// the Z.AI seed's rows moved inside five weeks — a constant here would go stale
// invisibly while a dated row goes stale visibly.
func AlternateSeatsFor(seats ...LaneSeat) AlternateSeats {
	out := AlternateSeats{}
	for _, s := range seats {
		if s.Lane == "" || s.Model == "" {
			continue
		}
		window := s.WindowTokens
		if window <= 0 {
			// The platform floor. Understating a window only splits stages
			// earlier, which is safe; minting an unverified one would disarm
			// overflow protection.
			window = DefaultWindowTokens
		}
		out[DutyExecution] = append(out[DutyExecution], Seat{Model: s.Model, Lane: s.Lane, WindowTokens: window})
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

	// LocalLane names the S12.1 class-(a) local ENGINE lane, so a task pin
	// naming it refuses IN ITS OWN WORDS rather than under the flat-rate
	// sentence [S00.9 A13]. The two absences are not the same absence: a
	// commissionable lane the owner has not bought is a subscription gap,
	// while the local engine lane is absent because no local provider entry
	// is commissioned at all — telling an operator to buy a subscription
	// they already hold would be the wrong answer to the wrong question.
	//
	// Empty leaves such a pin refused under the general sentence: the
	// verdict does not move, only the wording. The composition root sets it.
	LocalLane string

	// PinNotes are per-lane SENTENCES the plain reason quotes when a task
	// pins that lane — a fact about the lane the requester should read
	// beside their own choice. The live case is a shared allowance: two
	// lanes drawing ONE membership pool, where pinning between them changes
	// which client runs the work and changes the allowance not at all.
	//
	// It is a sentence rather than a structure for the reason
	// LanePressure.Reason is: this package still does not know what a plan
	// document is and still has nothing money-shaped to reason with (D5).
	// The composition root reads the documents; selection quotes what it is
	// handed.
	PinNotes map[string]string
}

func (c Coverage) laneCovered(lane string) bool {
	for _, l := range c.FlatRateLanes {
		if l == lane {
			return true
		}
	}
	return false
}

// PinnableLane is one lane a per-task lane pin may name, with the verdict
// already computed by the layer that owns it [S00.9 A13].
//
// It exists so the boundary that admits a pin does not have to re-derive
// whether it is honorable: internal/intake cannot import this package (the
// S06.10 seam wall), so the verdict crosses as DATA — the PlanWindowRecord
// shape, for the same reason. A rule spelled twice drifts (§65 D4).
type PinnableLane struct {
	Lane string
	// Pinnable reports that selection would honor a pin naming this lane.
	Pinnable bool
	// NotPinnable says why it would not, when it would not. Empty when
	// Pinnable.
	NotPinnable string
}

// LanePinRefusal is THE lane-pin predicate: it reports why a pin naming this
// lane cannot be honored, or "" when it can [S00.9 A13].
//
// One predicate because it is enforced at three layers — the system boundary
// refuses the submission before a task is born, the refusal verdict is carried
// across the intake seam rather than re-derived there, and selection re-checks
// it so a pin arriving by any other route cannot steer dispatch. A rule spelled
// three times drifts; a rule computed once and carried does not — the ratified
// pooled-plan-budget refusal shape, applied to a different axis. (Named by
// shape rather than by symbol: this package's own D5 scan bans it from naming
// the accounting package at all, and that scan is right.)
//
// The refusal is deliberately NOT the degrade-and-explain posture routing takes
// for an uncovered lane it chose itself, and the inversion is the point:
// routing degrades when the PLATFORM picked a lane it cannot use, and refuses
// when a PERSON did (§19; §12 — the only default is consent).
func LanePinRefusal(cov Coverage, lane string) string {
	switch {
	case lane == "":
		return ""
	case cov.laneCovered(lane):
		return ""
	case cov.LocalLane != "" && lane == cov.LocalLane:
		// S12.1 class (a): the local ENGINE lane has no v0 consumer because no
		// local provider entry is commissioned. Said plainly on the wire.
		return fmt.Sprintf("lane %q is the on-machine model lane, and no task can be sent to it yet: nothing is set "+
			"up to dispatch a task's work there, so pinning to it would quietly run this task on a paid model "+
			"instead. The lanes a task may pin are: %s", lane, pinnableList(cov))
	default:
		// S08.8 step 3: subscription coverage binds every choice.
		return fmt.Sprintf("lane %q is not covered by any subscription this household holds, and every choice the "+
			"platform makes stays inside that coverage. The lanes a task may pin are: %s", lane, pinnableList(cov))
	}
}

// PinnableLanes reports every lane a task-creation pin may name, plus the local
// engine lane with its own refusal, so the boundary can answer a typo by naming
// what exists rather than by guessing.
//
// It is the PROCESS-WIDE set, and the over-approximation is stated rather than
// hidden: Coverage is the union across the people who have placed a credential
// (§65), so at a household with more than one person this admits a pin the
// requester personally cannot draw on. What makes that safe is that selection
// re-checks against the same predicate and the dispatch still refuses; what
// makes it an over-approximation rather than a bug is that per-person coverage
// is not built (B6/v1) and the broker `who` → auth.User.ID relationship is
// unsettled — it rides LN gate-batch item 8. At v0 the platform is operated
// single-user, so the union IS the operator's set. Inventing a namespace
// mapping here would settle a household question the spec has not settled.
func PinnableLanes(cov Coverage) []PinnableLane {
	out := make([]PinnableLane, 0, len(cov.FlatRateLanes)+1)
	seen := map[string]bool{}
	for _, lane := range cov.FlatRateLanes {
		if lane == "" || seen[lane] {
			continue
		}
		seen[lane] = true
		out = append(out, PinnableLane{Lane: lane, Pinnable: true})
	}
	if cov.LocalLane != "" && !seen[cov.LocalLane] {
		out = append(out, PinnableLane{
			Lane:        cov.LocalLane,
			NotPinnable: LanePinRefusal(cov, cov.LocalLane),
		})
	}
	return out
}

// coveredLaneCount is the number of DISTINCT covered flat-rate lanes. The raw
// slice can repeat a lane — the composition root prepends the configured lane
// to the commissioned set, and a commissioned lane that IS the configured one
// appears twice — so counting it raw told an operator the pin replaced a
// comparison across three lanes when there were two (r1 F6). pinnableList
// already dedupes; the count now agrees with the names beside it.
func coveredLaneCount(cov Coverage) int {
	seen := map[string]bool{}
	for _, lane := range cov.FlatRateLanes {
		if lane != "" {
			seen[lane] = true
		}
	}
	return len(seen)
}

// pinnableList names the covered lanes for a refusal message. Lane names are
// not secret — they ship in the lane documents — so unlike the project pin
// there is no existence-oracle concern and the message may enumerate freely.
func pinnableList(cov Coverage) string {
	names := make([]string, 0, len(cov.FlatRateLanes))
	seen := map[string]bool{}
	for _, lane := range cov.FlatRateLanes {
		if lane == "" || seen[lane] {
			continue
		}
		seen[lane] = true
		names = append(names, strconv.Quote(lane))
	}
	if len(names) == 0 {
		return "none — this platform holds no flat-rate lane at all"
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// TieBreaker is the S08.8 step-2 seam: a local duty alias picks among
// multiple candidates with a one-line reason (Spec S12.4). The local tier
// lands at B4 — nil is the v0 posture and the router degrades to the
// deterministic order with the absence recorded in the reason (absent
// duties degrade, never faked).
type TieBreaker interface {
	Break(ctx context.Context, q RouteQuery, candidates []Candidate) (pick int, reason string, err error)
}

// LanePressure is one lane's consumption pressure as the D5 comparison needs
// it: a RATIO of consumption to the operator's declared budget for that lane,
// in that lane's own unit.
//
// A ratio is the only cross-lane comparable quantity here, and this type exists
// because a raw figure is not one. Anthropic consumption is counted in weighted
// tokens and Z.AI's in plan credits; ordering two lanes by their raw counts
// compares a token against a credit, and with no budget declared it compares
// two unbounded LIFETIME totals — under which a newly added lane holds the
// smaller number forever and therefore wins every dispatch, however hard it is
// actually being worked. Applicable says whether a denominator was declared at
// all, so its absence is a fact the caller must handle rather than a zero it
// cannot tell apart from an idle lane.
type LanePressure struct {
	// Ratio is consumption ÷ the declared budget, valid only when Applicable.
	Ratio float64
	// Applicable reports that a budget was declared for this (owner, lane).
	Applicable bool
	// Unit names what was counted, for the plain reason.
	Unit string
	// Reason is why an inapplicable reading is inapplicable, when the gauge has
	// something more specific to say than "none was declared" — a period that
	// has elapsed, or a window that cannot denominate this lane. It is a
	// SENTENCE the plain reason quotes, never a code and never a number: this
	// package still has nothing money-shaped to reason with and still does not
	// know what a plan document is (S08.8; D5).
	Reason string
}

// PressureReader is the D5 lane-ordering input among multiple covered
// flat-rate lanes: consumption pressure, never dollars (Spec S08.8 step 3;
// S10 owns the gauge). Nil, or a single covered lane, short-circuits to
// the configured lane order.
type PressureReader interface {
	// Pressure returns the owner's normalized consumption pressure on the
	// lane (higher = more of the declared budget consumed).
	Pressure(ctx context.Context, owner, lane string) (LanePressure, error)
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
	// Writes reports that the plan durably declares it writes the workspace:
	// any non-empty step write_set, or an unbounded (whole-project) claim —
	// the same fact the S02.8 W claim is minted from. A writing plan is a
	// REQUIREMENT like any other (Spec S08.8 step 1: "the plan declares
	// requirements — confinement class, tools…"), so equipment that cannot
	// write cannot take it, however well its description matches (P3-RW-14 R8).
	Writes bool
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
	// PinnedLane is the lane the requester declared on this task, empty for
	// the ordinary case [S00.9 A13]. When it binds it REPLACES the step-3
	// consumption-pressure comparison; when it cannot be honored selection
	// refuses rather than choosing something else. It is a person's named
	// choice and carries no money — D5 is untouched.
	PinnedLane string
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

	// LanePin records that this task carried a per-task lane pin and which
	// lane it named [S00.9 A13] — the structured member beside the plain
	// reason, so a surface can tell a pinned selection from one that merely
	// landed on the same lane. Empty on an unpinned task, and the empty
	// case serves exactly the bytes it served before.
	//
	// It is NOT RouteBlock.Pinned: that flag freezes the WORKER choice
	// against a re-plan recompute, and a lane pin asks for no such freeze.
	// A lane pin survives re-planning by construction — it is re-read from
	// the request on every recompute.
	LanePin string `json:"lane_pin,omitempty"`

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
	Store   *Store
	DutyMap DutyMap
	// Alternates are the extra flat-rate seats a duty may take when the owner
	// holds more than one flat-rate lane. Nil = the single-lane world, in
	// which selection takes exactly its pre-LN-2 path.
	Alternates AlternateSeats
	Coverage   Coverage
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

	candidates, refusedWrite, err := r.selectorMatch(ctx, q)
	if err != nil {
		return Decision{}, err
	}

	degraded, err := r.domainDegraded(ctx, q.Domain)
	if err != nil {
		return Decision{}, err
	}

	if len(candidates) == 0 {
		return r.noFit(ctx, q, signals, degraded, refusedWrite)
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
		d, nerr := r.noFit(ctx, q, append(signals, "model_gap="+gapAdvice), degraded, refusedWrite)
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
		LanePin:      q.PinnedLane,
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
// equal-or-tighter, and — for a plan that declares writes — equipment that
// can actually write).
//
// It returns the surviving candidates and the names of any refused for
// write-incapability: "nobody could" and "nobody matched" are different
// answers, and only the first can be explained to a person (R8).
func (r *Router) selectorMatch(ctx context.Context, q RouteQuery) ([]Candidate, []string, error) {
	rows, err := r.Store.db.QueryContext(ctx,
		`SELECT `+templateColumns+` FROM worker_templates WHERE status = 'active' AND kind = ?`, string(q.Kind))
	if err != nil {
		return nil, nil, fmt.Errorf("worker: scan active templates: %w", err)
	}
	var active []Template
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			rows.Close()
			return nil, nil, err
		}
		active = append(active, t)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}

	ftsRanks := map[string]float64{}
	if len(active) > 0 {
		hits, err := r.Store.searchDescriptions(ctx, q.TaskText)
		if err != nil {
			return nil, nil, err
		}
		for _, h := range hits {
			ftsRanks[h.TemplateID] = h.Rank
		}
	}

	planClass := loosestClass(q.Classes)
	taskLower := strings.ToLower(q.TaskText)

	var out []Candidate
	var refusedWrite []string
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
		// A writing plan needs equipment that can write (Spec S08.8 step 1:
		// the plan declares REQUIREMENTS and required grants ⊆ granted). The
		// live defect this closes: a template granted {Read, Grep, Glob} at
		// class C1 took a plan whose every step declared a write_set, then
		// spent real money proving it could not do the job. Refused
		// candidates are named, so the no-fit card can say WHY nobody fit
		// rather than implying nobody matched (P3-RW-14 R8).
		if q.Writes && !canWrite(g.Class, g.GrantedTools) {
			refusedWrite = append(refusedWrite, def.Name)
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
			// contribution SCALED BY MAGNITUDE so selector hits dominate
			// description echoes and a vacuous echo earns nothing (R9).
			if c := ftsContribution(rank); c > 0 {
				score += c
				why = append(why, fmt.Sprintf("description match (rank %.2f)", rank))
			}
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
	sort.Strings(refusedWrite)
	return out, refusedWrite, nil
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
	// dispatching onto a local model) still has NO v0 consumer.
	//
	// CORRECTED 2026-08-24 (P3-LN-2B R23). Until P3-LN-1 this clause read "no
	// second adapter exists in this cut", and that was true then and is false
	// now: the opencode substrate landed at LN-1 and is registered alongside
	// claudecli at LN-2A. The CONCLUSION is unchanged and the REASON is not —
	// what is missing is no longer an adapter but a commissioned local
	// PROVIDER ENTRY, which is a separate lane commissioning and not this
	// packet's. Stating it the old way would have a future reader believe the
	// platform still has one adapter.
	//
	// So a template/helper demanding duty utility/mechanical (an engine
	// dispatch) resolves to the paid execution seat either way, never a fake
	// dispatch onto a lane nothing is commissioned on, never an error (absent
	// duties degrade, never faked — S08.8/CONVENTIONS §19).
	// These notes are served on the requester's WHO-DOES-IT surface, so they
	// speak plain words; the citations (S12.1's class (a)/(b) consumer split,
	// S12's local tier) stay in the comment above (P3-GF13).
	localNote := ""
	if q.Mechanical || duty == DutyUtility {
		if r.Coverage.LocalAvailable {
			localNote = fmt.Sprintf("Work of this kind (%s) prefers the free models running on this machine, and they are serving — but nothing is set up yet to send a task's own work to them, so this one runs on a paid model.", plainDuty(duty))
		} else {
			localNote = fmt.Sprintf("Work of this kind (%s) prefers the free models on this machine, but none are set up here, so it runs on a paid model instead.", plainDuty(duty))
		}
		duty = DutyExecution
	}

	// seatDuty is the duty whose seat selection ACTUALLY resolved, which is not
	// always the duty the template asked for: an unknown duty degrades onto the
	// execution seat below. `duty` itself is deliberately NOT reassigned —
	// chooseFlatLane and the reason both read it, and moving it would change
	// the unpinned path — so the effective value is tracked separately and only
	// the lane pin reads it (P3-LN-9 r1 F1/F2).
	seat, ok := r.DutyMap[duty]
	seatDuty := duty
	if !ok {
		localNote = strings.TrimSpace(fmt.Sprintf("No model is assigned to work of this kind (%s), so it runs on the one that does the ordinary work.", plainDuty(duty)) + " " + localNote)
		seat, ok = r.DutyMap[DutyExecution]
		if !ok {
			return Seat{}, "", "", "", fmt.Errorf("%w: duty map has no execution seat", ErrInvalid)
		}
		seatDuty = DutyExecution
	}

	// The per-task LANE PIN [S00.9 A13]. It is resolved BEFORE the template's
	// model pin and before the flat-lane comparison, because it outranks both:
	// S08.8 records overrides with their actor, and a person's declaration on
	// THIS task outranks a standing template default (S08.4 8.9 puts task spec
	// above template baseline). When it binds, chooseFlatLane is not consulted
	// at all — the same reading the model pin already sets, for the same
	// reason: offering a different lane would not be honoring the pin.
	lanePinNote, lanePinned := "", false
	if q.PinnedLane != "" {
		pinnedSeat, note, err := r.resolveLanePin(q.PinnedLane, seatDuty, seat, p)
		if err != nil {
			return Seat{}, "", "", "", err
		}
		seat, lanePinNote, lanePinned = pinnedSeat, note, true
	}

	// A concrete model pin (recorded reason required at lint) overrides the
	// seat model — but coverage still binds. A bound lane pin has already
	// settled the seat, model included, so this arm stands down (the
	// supersession is stated in the pin's own note).
	pinNote := ""
	if p.ModelPin != "" && !lanePinned {
		if !r.modelCovered(p.ModelPin, seat.Lane) {
			// The 2.7 model-gap leg, in plain words. The observed-list diffing
			// this note admits is missing arrives with the S03.6 watch (B5).
			advice := fmt.Sprintf("Not covered by a subscription: this specialist asks for the model %q, and none of "+
				"the household's plans include it. (The covered list is read from this platform's own configuration; "+
				"it is not yet checked against what the provider actually offers.) You can run the all-rounder on a "+
				"covered model instead, or add coverage for that one.", p.ModelPin)
			return Seat{}, "", "", advice, nil
		}
		seat = Seat{Model: p.ModelPin, Lane: seat.Lane, WindowTokens: seat.WindowTokens}
		pinNote = fmt.Sprintf(" Model pinned by the template (%s).", p.ModelPinReason)
	}

	// Among the flat-rate lanes the owner actually holds, the choice is the
	// consumption gauge's. A pinned model skips this entirely: the template
	// asked for one model, and offering it a different lane's model would not
	// be honoring the pin.
	laneNote := ""
	if p.ModelPin == "" && !lanePinned {
		seat, laneNote = r.chooseFlatLane(ctx, q.Requester, duty, seat)
	}

	if !r.Coverage.laneCovered(seat.Lane) {
		if r.Coverage.MeteredAllowed != nil && r.Coverage.MeteredAllowed(seat.Model) {
			// Unreachable in production: the metered-exception list is EMPTY
			// at v0 (G1 P7). The branch exists so the refusal is testable.
			return Seat{}, "", "", "", fmt.Errorf("%w: metered selection is structurally disabled at v0 (D5/G1 P7)", ErrInvalid)
		}
		advice := fmt.Sprintf("Not covered by a subscription: work of this kind (%s) is assigned to %s, and the "+
			"household holds no plan for it. (The covered list is read from this platform's own configuration; it is "+
			"not yet checked against what the provider actually offers.)", plainDuty(duty), seat.Lane)
		return Seat{}, "", "", advice, nil
	}

	if seat.WindowTokens == 0 {
		seat.WindowTokens = DefaultWindowTokens
	}

	effort := p.EffortFloor
	// The seat sentence is the WHO-DOES-IT line the requester reads, so it names
	// the model, the lane and what the seat is FOR in plain words. D5 (never
	// dollars, flat-rate coverage binds), S08.8 step 4 (research nodes take the
	// search-capable lane) and S10's effort ladder are cited here, not on the
	// wire (P3-GF13).
	reason := fmt.Sprintf("Runs on %s (the %s lane), the model assigned here to %s; the choice is bound to what the household's subscriptions cover, so nothing is billed per call.", seat.Model, seat.Lane, plainDuty(duty))
	if effort != "" {
		reason += fmt.Sprintf(" It works at %s effort or higher, never less.", effort)
	}
	if q.Research {
		reason += " Steps that have to look something up run on a lane whose model can search."
	}
	if laneNote != "" {
		reason += " " + laneNote
	}
	if lanePinNote != "" {
		reason += " " + lanePinNote
	}
	if localNote != "" {
		reason = localNote + " " + reason
	}
	reason += pinNote
	return seat, effort, reason, "", nil
}

// resolveLanePin settles the seat a bound per-task lane pin selects, and the
// sentence that says so [S00.9 A13].
//
// The search is three-deep, and the third leg is the one r1 F1/F2 added. A
// commissioned lane seats EXECUTION ONLY by ratification — planning and judge
// keep their anthropic-only seats because nobody has measured a second lane's
// models against the S07.5 bars — so `AlternateSeatsFor` only ever populates
// DutyExecution. Searching the template's own duty alone therefore REFUSED a
// pin to a perfectly covered lane whenever that duty was not execution, which
// is a refusal about this platform's seat bookkeeping wearing the costume of a
// coverage verdict. The pin names a LANE; the lane's seat is the lane
// document's own; so the pin is honored on that lane's execution seat and the
// substitution is STATED rather than smuggled.
//
// No model id is minted on any leg (§63 D5): every seat here came from a lane
// document through the composition root.
func (r *Router) resolveLanePin(pin, seatDuty string, seat Seat, p ExecutionProfile) (Seat, string, error) {
	// Layer 3 of the three. The boundary already refused this, and it is
	// checked again here so a pin planted by any other route cannot steer
	// dispatch — the same defence-in-depth the pooled plan budget takes.
	if refusal := LanePinRefusal(r.Coverage, pin); refusal != "" {
		return Seat{}, "", fmt.Errorf("%w: %s", ErrLanePinUnhonorable, refusal)
	}

	chosen, found, viaExecution := Seat{}, false, false
	switch {
	case seat.Lane == pin:
		chosen, found = seat, true
	default:
		for _, a := range r.Alternates[seatDuty] {
			if a.Lane == pin {
				chosen, found = a, true
				break
			}
		}
		if !found && seatDuty != DutyExecution {
			// The lane seats execution only, which is the ratified shape
			// rather than an accident. Honor the pin there.
			for _, a := range r.Alternates[DutyExecution] {
				if a.Lane == pin {
					chosen, found, viaExecution = a, true, true
					break
				}
			}
		}
	}
	if !found {
		// Held flat-rate and still nothing to seat: NOTHING on this platform
		// has an execution seat on that lane — no lane document contributed
		// one, so there is no model to run. Refusing is the only honest
		// answer; riding another lane's seat would hand the requester a lane
		// they did not ask for, which is the whole failure the pin exists to
		// end. (The cause is this platform's seat set, NOT the duty: saying
		// "this duty resolves to no model there" sent a reader to the template
		// when the missing thing is a commissioned seat — r1 F1.)
		// Served on the wire as an honest refusal (LN-9), so it says the same
		// thing without the citations: S08.8 step 3 (coverage binds) and S03.6
		// (seat rows come from the lane documents, never invented here).
		return Seat{}, "", fmt.Errorf("%w: lane %q is pinned on this task and the household does hold a plan for it, "+
			"but no model on that lane has been set up here to run work, so there is nothing to run this task on. "+
			"Adding one is a change to this platform's own lane setup, not to the task", ErrLanePinUnhonorable, pin)
	}
	if chosen.WindowTokens == 0 {
		chosen.WindowTokens = DefaultWindowTokens
	}

	// The honored-pin note is requester copy (the pin is visible and
	// overridable, S08.8 [S00.9 A13]) and the comparison it replaced is about
	// subscription headroom, never dollars (D5) — both said plainly here.
	note := fmt.Sprintf("Lane %q is pinned on this task, so the platform used it instead of comparing how much "+
		"subscription headroom is left across the %d covered lanes. That comparison is never about money — and this "+
		"pin is shown here so you can change it.", pin, coveredLaneCount(r.Coverage))
	if viaExecution {
		// S07.5 sets the quality bars planning and judging have to clear.
		note += fmt.Sprintf(" Lane %q is only set up for doing the work — planning and checking keep the models "+
			"chosen for them, because no other lane's models have been measured against the quality bars those two "+
			"jobs have to clear — so %s runs on that lane's working model (%s).",
			pin, plainDuty(seatDuty), chosen.Model)
	}
	if laneNote := r.Coverage.PinNotes[pin]; laneNote != "" {
		note += " " + laneNote
	}
	if p.ModelPin != "" {
		if chosen.Lane == seat.Lane {
			// No conflict: the pin names the duty seat's own lane, so the
			// template's model pin still applies on it. Coverage was settled
			// above, and modelCovered is that same lane question, so the 2.7
			// model-gap leg cannot fire on this path.
			chosen = Seat{Model: p.ModelPin, Lane: chosen.Lane, WindowTokens: chosen.WindowTokens}
			note += fmt.Sprintf(" Model pinned by the template (%s), which the lane pin leaves standing "+
				"because it names this seat's own lane.", p.ModelPinReason)
		} else {
			// S08.8 records an override with its actor; S08.4 8.9 puts the
			// task's own spec above a template's standing baseline.
			note += fmt.Sprintf(" It OVERRIDES the model this specialist normally asks for, whose recorded "+
				"reason was %q: the change is recorded against the person who made it, and a choice made on "+
				"this task outranks a standing default.", p.ModelPinReason)
		}
	}
	return chosen, note, nil
}

// chooseFlatLane picks among the flat-rate seats the owner holds for a duty:
// the duty-map seat plus any alternates registered for it.
//
// D5, verbatim (S08.8; S10.2): "Dollar-based routing between flat-rate lanes is
// a D5 violation and NEVER done." The ordering input is CONSUMPTION PRESSURE —
// the lane a person has spent less of goes first — and no price, cost or
// dollar figure is available to this function to reason with even if somebody
// wanted to. That is a structural property, not a discipline: nothing in this
// package's selection inputs carries money (see the head comment).
//
// It returns the seat unchanged when nothing better applies, so the pre-LN-2
// single-lane world takes exactly its old path: one covered candidate, no
// gauge read, no note.
func (r *Router) chooseFlatLane(ctx context.Context, owner, duty string, seat Seat) (Seat, string) {
	alts := r.Alternates[duty]
	if len(alts) == 0 {
		return seat, ""
	}
	// Configured order: the duty-map seat first, then its alternates. This is
	// the tie-break and the fallback, so the choice is deterministic when the
	// gauge cannot separate two lanes.
	covered := make([]Seat, 0, 1+len(alts))
	if r.Coverage.laneCovered(seat.Lane) {
		covered = append(covered, seat)
	}
	for _, a := range alts {
		if a.Lane != seat.Lane && r.Coverage.laneCovered(a.Lane) {
			covered = append(covered, a)
		}
	}
	switch len(covered) {
	case 0:
		// Nothing covered: hand back the duty-map seat so the caller's 2.7
		// subscription-gap leg fires with its unchanged wording.
		return seat, ""
	case 1:
		if covered[0].Lane == seat.Lane {
			return covered[0], ""
		}
		return covered[0], fmt.Sprintf("The %s duty seat's own lane is not held flat-rate; %s is the one covered alternative.",
			duty, covered[0].Lane)
	}

	if r.Pressure == nil {
		return covered[0], fmt.Sprintf("%d covered lanes can do this work and nothing is measuring how much of each is left, so the configured order stands. The choice is never about money.",
			len(covered))
	}
	best, bestP := covered[0], 0.0
	for i, c := range covered {
		p, err := r.Pressure.Pressure(ctx, owner, c.Lane)
		if err != nil {
			// A gauge that cannot answer degrades to the configured order
			// rather than failing the dispatch, and says so.
			return covered[0], fmt.Sprintf("How much of each lane is left could not be read (%v), so the configured lane order stands. The choice is never about money.", err)
		}
		if !p.Applicable {
			// No comparable ratio on a lane means no comparison. The
			// alternative — ordering by raw consumption — compares a token
			// count against a credit count and hands every dispatch to
			// whichever lane was added most recently, forever. A stable,
			// STATED order is the honest answer to "we cannot tell yet".
			//
			// WHY it cannot be compared matters to whoever reads the decision:
			// "nobody declared a budget" and "the declared period ran out" are
			// different situations with different remedies, and a reason that
			// says the first when the second is true sends a person looking for
			// a setting they already set (drain r2 R2/R3).
			why := "has no declared automation budget"
			if p.Reason != "" {
				why = "cannot be compared: " + p.Reason
			}
			return covered[0], fmt.Sprintf("%d covered lanes can do this work, but %s %s, "+
				"so there is nothing comparable to weigh and the configured order stands. The choice is never about money.",
				len(covered), c.Lane, why)
		}
		if i == 0 || p.Ratio < bestP {
			best, bestP = c, p.Ratio
		}
	}
	// S10.4's gauge, D5's never-dollars rule: the comparison is headroom, said
	// plainly for the requester reading the selection.
	return best, fmt.Sprintf("Chosen among %d covered lanes by how much of each is left — %s has used %.0f%% of the automation budget declared for it, the least of any. The choice is never about money.",
		len(covered), best.Lane, bestP*100)
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
func (r *Router) noFit(ctx context.Context, q RouteQuery, signals []string, degraded bool, refusedWrite []string) (Decision, error) {
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

	// The no-fit sentence is the headline of the requester's WHO-DOES-IT card,
	// so it carries none of its rules' ids: S08.8 (the generalist with injected
	// knowledge is the default for a one-off), S08.7 (a domain with no verified
	// quality check marks its results degraded) and S08.6 (a recurring task
	// family EARNS a composed specialist) are cited here (P3-GF13).
	reason := "No trained specialist matches this work, so it runs on the platform's all-rounder, with the knowledge this task needs handed to it up front — the ordinary way a one-off job is done."
	if degraded {
		reason = "No trained specialist matches this work, so it runs on the platform's all-rounder, with the knowledge this task needs handed to it up front. The result is marked lower-confidence: for work of this kind the platform has no proven way to check the quality of what comes back."
	}
	if len(refusedWrite) > 0 {
		// Say WHY first, in the requester's language: these specialists
		// matched the work and were refused the EQUIPMENT for it, which is a
		// different — and actionable — fact from nobody matching at all (R8).
		reason = fmt.Sprintf(
			"This plan changes files, and %s cannot do that — they are set up to read a project, not write to it. ",
			humanList(refusedWrite)) + reason
	}
	if due {
		reason += fmt.Sprintf(" Work like this has now come up %d times, which is enough for a specialist to be worth training for it — a proposal to do that is open for you.", rec.Occurrences)
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
		LanePin:       q.PinnedLane,
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

// writeInstruments is the set of granted tools that can put bytes on disk —
// the engine's file-writing verbs. A STRUCTURAL constant, not a ⚙ setting (the
// §7 sseBatchSize precedent; flagged to the settings tab): it is the platform's
// own tool vocabulary, and an operator turning a write tool into a non-write
// tool by editing a number would be a lie, not a preference.
//
// Bash is deliberately absent. No v0 toolset grants it (stage's execTools is
// Read/Write/Edit precisely so outward effects stay structurally unreachable,
// Spec S02.7/S03.4), so admitting a shell as a "write instrument" would widen
// this filter on a capability the platform does not hand out.
var writeInstruments = map[string]bool{
	"Write":        true,
	"Edit":         true,
	"MultiEdit":    true,
	"NotebookEdit": true,
}

// canWrite reports whether a worker's GRANTED equipment can write the
// workspace: it needs both a class whose workspace is mounted read-write and
// at least one tool that can write a file. Either half missing means the
// worker structurally cannot do a writing plan (Spec S08.8 step 1; S11.1 —
// enforcement is the confinement class, consent is only cooperation).
//
// Note the two halves are independent: the S11 ladder is ordered by RANK, not
// by workspace capability (C0 is the tightest and has no filesystem at all),
// so "can write" is a property of the class, never a rank threshold.
func canWrite(class string, granted []string) bool {
	if !sandbox.Class(class).WorkspaceWritable() {
		return false
	}
	for _, tool := range granted {
		if writeInstruments[tool] {
			return true
		}
	}
	return false
}

// ftsMinMagnitude is the bm25 magnitude below which a description hit is
// NOISE, not evidence. SQLite's bm25 is negative-better and its magnitude
// carries inverse document frequency: a term appearing in EVERY indexed
// description has idf ~0 and ranks at ~-0.000001 regardless of corpus size
// (measured on this driver) — which says "this word distinguishes nobody",
// the exact opposite of a match. A term that genuinely discriminates ranks
// -1.2 (5 templates) to -3.4 (30). This floor sits three orders of magnitude
// above the noise and two below the weakest real signal.
//
// The live defect it closes: "release-notes-writer" won a webshop task on a
// rank of -0.00 — a vacuous echo of the task's own words — because ANY hit
// scored a flat +0.5. Structural constant, no ⚙ key (settings-tab flagged).
const ftsMinMagnitude = 0.01

// ftsWeight bounds the description leg's contribution so it can never
// outweigh a declared selector (family +2, trigger/task-class +1 each):
// what a worker DECLARES it is for outranks what its prose happens to echo.
const ftsWeight = 0.5

// ftsContribution folds a bm25 rank into a bounded, magnitude-scaled score in
// [0, ftsWeight). A ~0-magnitude hit contributes exactly 0, so a candidate
// with no other signal keeps score 0 and takes the existing skip (R9).
func ftsContribution(rank float64) float64 {
	mag := -rank // bm25 is negative-better in SQLite
	if mag < ftsMinMagnitude {
		return 0
	}
	// Saturating: strictly increasing in magnitude, never reaching ftsWeight.
	return ftsWeight * mag / (1 + mag)
}

// humanList renders names as a plain-language list for a requester-facing
// reason ("a", "a and b", "a, b and c").
func humanList(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return strconv.Quote(names[0])
	}
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, strconv.Quote(n))
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
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

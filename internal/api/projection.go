package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// projection.go is the S14.3 snapshot-projection home (brief §3, OQ3): the
// four per-topic read-only projections a topic-subscribed connect emits before
// tailing. Each is a short SELECT set over the existing tables (runs, tasks,
// asks, effects, checkpoints) — no long-lived read transaction (S02.1 read
// hygiene, the After precedent) — owner-scoped per S01.9 (OQ1). Redaction is
// NOT applied here: the serving edge (sse.go) redacts the marshaled snapshot
// state once, so this layer stays a pure read model.
//
// The `run` card carries the S14.3 progress semantics (§4) with the checkable
// NEGATIVE (R10): there is NO percent/fraction/ratio/progress/pct/complete or
// ETA field anywhere in these shapes. Counters are monotonic and unbounded —
// they never imply a denominator.

// ownerScope is the S01.9 AuthZ posture for a projection: the operator (and
// the dev-posture bootstrap identity) sees ALL rows; a member sees only their
// own (user_id = UserID). AuthZ is enforced here in the data layer, per S01.9
// ("enforced in the control plane's data layer; every query passes through
// owner-scoped accessors").
type ownerScope struct {
	UserID   string
	Operator bool
}

// projector reads the live-inspection projections over the platform DB. meter
// is the narrow metering seam for the run-card token/cost counters; nil leaves
// those counters at zero (best-effort — the shell always wires it).
type projector struct {
	db    *storage.DB
	meter MeterReader
	// now is the clock seam for horizon-bounded projections; nil ⇒ time.Now.
	now func() time.Time
}

// clock returns the projector's clock, defaulting to the wall clock.
func (p *projector) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

// activeStates are the non-terminal FSM states — a run still consuming or
// waiting (S02.3). The fleet lane snapshot counts these.
var activeStates = []any{"new", "queued", "claimed", "running", "parked", "draining"}

// ── run detail (§4 progress card) ──

// ToolInfo is the current tool + args digest (never raw args, S14.2
// tools_artifacts).
type ToolInfo struct {
	Name       string `json:"name"`
	ArgsDigest string `json:"args_digest"`
}

// RunCounters are the monotonic, unbounded progress counters (§4). NEVER a
// percent, fraction, ratio or ETA (R10): a denominator is never implied.
type RunCounters struct {
	Tokens          int64   `json:"tokens"`
	APIEquivCostUSD float64 `json:"api_equiv_cost_usd"`
	ElapsedS        int64   `json:"elapsed_s"`
	Steps           int64   `json:"steps"`
}

// Activity is the last-activity line (§4): the latest run_events row summarized.
type Activity struct {
	Type string    `json:"type"`
	TS   time.Time `json:"ts"`
	Line string    `json:"line"`
}

// Ceilings are the per-run S3.7 ceilings (context, not progress).
type Ceilings struct {
	TimeS   int64   `json:"time_s"`
	Steps   int64   `json:"steps"`
	CostUSD float64 `json:"cost_usd"`
}

// RunCard is the `run` topic projection: the S14.3 progress card (§4). It
// exposes FSM state, first-class waiting-on-human and parked-until, the
// current stage, the current tool + args digest, monotonic counters, the
// last-activity line, and the DERIVED wedged marker (never a stored column,
// R11; the recovery-side signal, distinct from B5-3's watchdog.flagged).
type RunCard struct {
	RunID          string      `json:"run_id"`
	Owner          string      `json:"owner"`
	State          string      `json:"state"`
	WaitingOnHuman bool        `json:"waiting_on_human"`
	ParkedUntil    *string     `json:"parked_until"`
	Stage          string      `json:"stage"`
	Tool           *ToolInfo   `json:"tool"`
	Counters       RunCounters `json:"counters"`
	LastActivity   *Activity   `json:"last_activity"`
	Wedged         bool        `json:"wedged"`
	Lane           string      `json:"lane"`
	Generation     int64       `json:"generation"`
	Ceilings       Ceilings    `json:"ceilings"`
}

// runCard projects one run's progress card and returns the run's owner (for
// the OQ1 subscription owner-check) and whether the run exists.
func (p *projector) runCard(ctx context.Context, runID string) (RunCard, string, bool, error) {
	var (
		c          RunCard
		createdRaw string
	)
	c.RunID = runID
	err := p.db.QueryRowContext(ctx,
		`SELECT user_id, state, lane, generation, ceiling_time_s, ceiling_steps, ceiling_cost_usd, created_ts
		 FROM runs WHERE run_id = ?`, runID).
		Scan(&c.Owner, &c.State, &c.Lane, &c.Generation,
			&nullInt{&c.Ceilings.TimeS}, &nullInt{&c.Ceilings.Steps}, &nullFloat{&c.Ceilings.CostUSD}, &createdRaw)
	if err == sql.ErrNoRows {
		return RunCard{}, "", false, nil
	}
	if err != nil {
		return RunCard{}, "", false, fmt.Errorf("projection: run card %q: %w", runID, err)
	}

	// waiting-on-human: parked AND an open ask exists for the run (first-class).
	var openAsks int64
	if err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM asks WHERE run_id = ? AND answered_ts IS NULL`, runID).Scan(&openAsks); err != nil {
		return RunCard{}, "", false, fmt.Errorf("projection: open asks %q: %w", runID, err)
	}
	c.WaitingOnHuman = c.State == "parked" && openAsks > 0

	// parked-until: from the latest usage-limits event payload (not a column).
	if pay, ok := p.latestPayload(ctx, runID, "limit.event", "engine.rate_limit", "run.parked"); ok {
		if v := firstString(pay, "parked_until", "parked-until", "resets_at", "resets-at", "reset_at"); v != "" {
			c.ParkedUntil = &v
		}
	}

	// stage: the latest stage marker (declare-only stage.* producers are B6;
	// today intake.state is the concrete marker). Seam: richer when B6 lands.
	if pay, ok := p.latestPayloadType(ctx, runID, &c.Stage, "stage.started", "stage.finished", "intake.state"); ok {
		if v := firstString(pay, "stage", "kind", "name"); v != "" {
			c.Stage = v
		}
	}

	// tool + args digest: the latest tool.completed (renamed engine.tool_result).
	if pay, ok := p.latestPayload(ctx, runID, "tool.completed", "engine.tool_result"); ok {
		name := firstString(pay, "tool", "name", "tool_name")
		if name != "" {
			c.Tool = &ToolInfo{Name: name, ArgsDigest: argsDigest(pay)}
		}
	}

	// counters — monotonic, no denominator.
	c.Counters.ElapsedS = elapsedSeconds(createdRaw)
	if err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM checkpoints WHERE run_id = ?`, runID).Scan(&c.Counters.Steps); err != nil {
		return RunCard{}, "", false, fmt.Errorf("projection: steps %q: %w", runID, err)
	}
	if p.meter != nil {
		if m, merr := p.meter.RunMeter(ctx, runID); merr == nil {
			c.Counters.Tokens = m.Tokens
			c.Counters.APIEquivCostUSD = m.APIEquivCostUSD
		}
	}

	// last-activity line: the latest run_events row for the run.
	if a, ok := p.lastActivity(ctx, runID); ok {
		c.LastActivity = &a
	}

	// wedged — DERIVED (R11): true iff the most recent lifecycle marker for the
	// run is the recovery-side run.wedged (a later transition supersedes it).
	// NEVER a stored column; NOT B5-3's watchdog.flagged.
	c.Wedged = p.derivedWedged(ctx, runID)

	return c, c.Owner, true, nil
}

// ── board ──

// BoardRun is a task's latest run summarized for a board card.
type BoardRun struct {
	RunID          string     `json:"run_id"`
	State          string     `json:"state"`
	Wedged         bool       `json:"wedged"`
	LastActivityTS *time.Time `json:"last_activity_ts"`
}

// BoardTask is one board card: the task plus its latest run.
type BoardTask struct {
	TaskID       string    `json:"task_id"`
	Title        string    `json:"title"`
	KanbanStatus string    `json:"kanban_status"`
	LatestRun    *BoardRun `json:"latest_run"`
}

// BoardSnapshot is the `board` topic projection (§3): the owner's tasks with
// their latest run, each card the summary of the run card.
type BoardSnapshot struct {
	Tasks []BoardTask `json:"tasks"`
}

func (p *projector) board(ctx context.Context, scope ownerScope) (BoardSnapshot, error) {
	out := BoardSnapshot{Tasks: []BoardTask{}}
	q := `SELECT task_id, title, kanban_status FROM tasks`
	args := []any{}
	if !scope.Operator {
		q += ` WHERE user_id = ?`
		args = append(args, scope.UserID)
	}
	q += ` ORDER BY created_ts, task_id`
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return out, fmt.Errorf("projection: board tasks: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t BoardTask
		if err := rows.Scan(&t.TaskID, &t.Title, &t.KanbanStatus); err != nil {
			return out, fmt.Errorf("projection: board scan: %w", err)
		}
		out.Tasks = append(out.Tasks, t)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	for i := range out.Tasks {
		if lr, ok := p.latestRunForTask(ctx, out.Tasks[i].TaskID); ok {
			out.Tasks[i].LatestRun = &lr
		}
	}
	return out, nil
}

func (p *projector) latestRunForTask(ctx context.Context, taskID string) (BoardRun, bool) {
	var lr BoardRun
	err := p.db.QueryRowContext(ctx,
		`SELECT run_id, state FROM runs WHERE task_id = ? ORDER BY created_ts DESC, run_id DESC LIMIT 1`, taskID).
		Scan(&lr.RunID, &lr.State)
	if err != nil {
		return BoardRun{}, false
	}
	lr.Wedged = p.derivedWedged(ctx, lr.RunID)
	if a, ok := p.lastActivity(ctx, lr.RunID); ok {
		ts := a.TS
		lr.LastActivityTS = &ts
	}
	return lr, true
}

// ── fleet / meters ──

// FleetLane is one (owner, lane)'s meter snapshot (§3): the active-run count
// plus the S10.4 meter reads — weighted_consumption (always available),
// utilization and budget_remaining (nil until an operator budget is declared,
// S10.4 — v0 declares none). burn_rate is a SEAM: the S10.4 gauge exposes
// cumulative weighted consumption, not a trailing-window rate; a real burn_rate
// needs a windowed calc B4-5/B6 own — nil here, never faked.
type FleetLane struct {
	Owner               string   `json:"owner"`
	Lane                string   `json:"lane"`
	ActiveRuns          int64    `json:"active_runs"`
	WeightedConsumption float64  `json:"weighted_consumption"`
	Utilization         *float64 `json:"utilization"`
	BudgetRemaining     *float64 `json:"budget_remaining"`
	BurnRate            *float64 `json:"burn_rate"`
}

// FleetSeat is a local-tier duty seat's admission state. Declared here; the
// internal/local seat read + B4-6 VRAM land with the B6 fleet surface (seam) —
// v0 emits an empty slice, mirroring inbox's declare-only watchdog/drift.
type FleetSeat struct {
	Seat           string `json:"seat"`
	AdmissionState string `json:"admission_state"`
	AliasTarget    string `json:"alias_target"`
}

// FleetGPU is the thin GPU/VRAM read (B4-6); nil at v0 (do not rebuild the GPU
// broker here — S12 owns it).
type FleetGPU struct {
	VRAMUsed  int64 `json:"vram_used"`
	VRAMTotal int64 `json:"vram_total"`
}

// FleetSnapshot is the `fleet` topic projection (§3): a lane-level meter
// snapshot (the v0 minimum) plus the declared local-seat/GPU seams.
type FleetSnapshot struct {
	Lanes      []FleetLane `json:"lanes"`
	LocalSeats []FleetSeat `json:"local_seats"`
	GPU        *FleetGPU   `json:"gpu"`
}

func (p *projector) fleet(ctx context.Context, scope ownerScope) (FleetSnapshot, error) {
	out := FleetSnapshot{Lanes: []FleetLane{}, LocalSeats: []FleetSeat{}}
	// Group by (owner, lane) so each lane's meter is read for its owner — a
	// member sees only their own (owner, lane) rows and figures; the operator
	// sees every owner's (owner-scoped exactly like the rest of the snapshot).
	q := `SELECT user_id, lane, COUNT(*) FROM runs WHERE state IN (?, ?, ?, ?, ?, ?)`
	args := append([]any{}, activeStates...)
	if !scope.Operator {
		q += ` AND user_id = ?`
		args = append(args, scope.UserID)
	}
	q += ` GROUP BY user_id, lane ORDER BY lane, user_id`
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return out, fmt.Errorf("projection: fleet lanes: %w", err)
	}
	type fleetRow struct {
		owner  string
		lane   string
		active int64
	}
	var raw []fleetRow
	for rows.Next() {
		var fr fleetRow
		if err := rows.Scan(&fr.owner, &fr.lane, &fr.active); err != nil {
			rows.Close()
			return out, fmt.Errorf("projection: fleet scan: %w", err)
		}
		raw = append(raw, fr)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	// Meter reads run after draining the lane cursor (the meter seam queries
	// the DB). LaneMeter is owner-keyed on the row's owner, so a member's
	// figures cover only their own consumption.
	for _, fr := range raw {
		l := FleetLane{Owner: fr.owner, Lane: fr.lane, ActiveRuns: fr.active}
		if p.meter != nil {
			if m, merr := p.meter.LaneMeter(ctx, fr.owner, fr.lane); merr == nil {
				l.WeightedConsumption = m.WeightedConsumption
				l.Utilization = m.Utilization
				l.BudgetRemaining = m.BudgetRemaining
			}
		}
		out.Lanes = append(out.Lanes, l)
	}
	return out, nil
}

// argsDigest returns the tool's args digest for the run card (§4: "digest,
// never raw args"). It uses a real digest field when present; when only raw
// args are present it returns a short sha256 fingerprint — NEVER the raw args
// verbatim; "" when neither is present.
func argsDigest(pay json.RawMessage) string {
	if d := firstString(pay, "args_digest", "digest"); d != "" {
		return d
	}
	if raw := firstString(pay, "args"); raw != "" {
		sum := sha256.Sum256([]byte(raw))
		return "sha256:" + hex.EncodeToString(sum[:])[:12]
	}
	return ""
}

// ── inbox ──

// InboxAsk is one open ask (answered_ts IS NULL). kind rides the asks.status
// value at v0 (the asks table has no separate kind column); the invocation
// snapshot is deliberately NOT projected here (it is the resume input, not an
// inbox summary field).
type InboxAsk struct {
	AskID      string    `json:"ask_id"`
	RunID      string    `json:"run_id"`
	Kind       string    `json:"kind"`
	ObservedTS time.Time `json:"observed_ts"`
}

// InboxApproval is one proposed effect awaiting approval (S02.7).
type InboxApproval struct {
	EffectID string `json:"effect_id"`
	RunID    string `json:"run_id"`
	Class    string `json:"class"`
	State    string `json:"state"`
}

// deadManCanaryPrefix marks the watchdog suite's synthetic dead-man canary runs
// (internal/watchdog's canaryPrefix). Their flags never enter the inbox (drain
// D4). internal/api does not import internal/watchdog (the seam is the canonical
// run_events rows), so the prefix is pinned here as a literal.
const deadManCanaryPrefix = "platform.deadman."

// InboxWatchdogFlag is one OPEN watchdog flag surfaced in the inbox (B5-3,
// S14.4): derived from the run_events watchdog.flagged rows — the latest per
// (run, anomaly class) not superseded by a watchdog.suppressed for that rule or
// a resume (run.state_changed to=running, the "resume, I was wrong" action).
// RunID is "" for a run-less flag (platform-health or a per-person spend spike).
// Owner-scoped per S01.9 (the operator sees all incl. platform-scope; a member
// sees only their own owner-attributed flags).
type InboxWatchdogFlag struct {
	Seq          int64     `json:"seq"`
	RunID        string    `json:"run_id"`
	Owner        string    `json:"owner"`
	Rule         string    `json:"rule"`
	AnomalyClass string    `json:"anomaly_class"`
	Severity     string    `json:"severity"`
	Detail       string    `json:"detail"`
	FlaggedTS    time.Time `json:"flagged_ts"`
}

// InboxConformanceCard is one RED conformance-registry result surfaced in the
// inbox (B5-4, S14.5): a suite/drill whose last recorded result is red. It is
// pure derivation over the conformance_registry table — no asks row (a suite
// run has no platform run; asks.run_id is NOT NULL, R15), and internal/api does
// not import internal/conformance (it reads the table directly, the
// watchdog-flags derive-from-log precedent). A GREEN result raises nothing;
// dueness/overdue-ness raise nothing (OQ3). FlagNow is the S14.4 alert-routing
// class — true exactly on lane- or storage-affecting rows (R14); it is distinct
// from the inbox Low/Medium/High risk tiers. Rows are platform-scope, so only
// the operator sees them (R16); a member never does.
type InboxConformanceCard struct {
	RowID         string    `json:"row_id"`
	OwningSection string    `json:"owning_section"`
	Schedule      string    `json:"schedule"`
	AffectClass   string    `json:"affect_class"`
	FlagNow       bool      `json:"flag_now"`
	LastResult    string    `json:"last_result"`
	LastRunTS     time.Time `json:"last_run_ts"`
}

// InboxDriftCard is one outside-world drift card (B5-6A, S14.6 ¶2): a storm of
// drift.finding rows sharing an incident fingerprint, folded into ONE card per
// incident window. It is pure derivation over `run_events` — no card table and
// no bare `asks` row (a watch hit has no run; asks.run_id is NOT NULL) — and
// internal/api does not import internal/watchlist (it reads the canonical type
// string directly, the watchdog-flags derive-from-log precedent). Findings are
// platform-scope, so only the operator sees them; a member never does (S01.9).
//
// Severity is the S14.4 alert-routing class computed by the producer from the
// change class and the WATCH ROW's lane — it is carried, never recomputed here.
type InboxDriftCard struct {
	// Seq is the fingerprint's FIRST finding in this window (the card's
	// identity); LatestSeq is its most recent.
	Seq       int64 `json:"seq"`
	LatestSeq int64 `json:"latest_seq"`
	// Fingerprint is the incident fingerprint over {source, lane, change class}.
	Fingerprint string   `json:"fingerprint"`
	Source      string   `json:"source"`
	Lanes       []string `json:"lanes"`
	ChangeClass string   `json:"change_class"`
	Severity    string   `json:"severity"`
	Summary     string   `json:"summary"`
	RowID       string   `json:"row_id"`
	// Hits is how many findings folded into this card — one card per storm.
	Hits int `json:"hits"`
	// Classified is false when the local second pass could not judge the hit;
	// the card then reads "unclassified — the operator reads it".
	Classified bool `json:"classified"`
	// RevalidationTriggered / RevalidationNote make the S14.8 edge VISIBLE on
	// every card, including when nothing was triggered (OQ4(a)).
	RevalidationTriggered bool      `json:"revalidation_triggered"`
	RevalidationNote      string    `json:"revalidation_note,omitempty"`
	FirstTS               time.Time `json:"first_ts"`
	LatestTS              time.Time `json:"latest_ts"`
}

// driftIncidentWindow is the fingerprint dedup window: findings sharing a
// fingerprint within one window fold into ONE card ("one card per storm",
// S14.6 ¶2; the S10.5 once-per-storm logging discipline), and the next window
// opens a new card.
//
// driftCardHorizon and driftCardCap BOUND the inbox. Unlike watchdog flags
// (which close on a suppress or a resume) and conformance rows (which are RED
// or absent), a drift finding has no close verb at v0 — the dismiss action is
// the B6 inbox surface's — so without a bound every finding ever logged would
// derive into a forever-listed card and a re-firing condition would grow the
// inbox without limit. The horizon is what makes the list an INBOX rather than
// an archive: older findings stay in `run_events` and remain queryable, they
// simply stop being open cards. The cap is the belt for a storm of distinct
// fingerprints inside one horizon.
//
// All three are structural, not ⚙ — S18 ratifies no key (the §7 sseBatchSize
// precedent, interim under the standing settings-tab directive).
const (
	driftIncidentWindow = 24 * time.Hour
	driftCardHorizon    = 30 * 24 * time.Hour
	driftCardCap        = 100
)

// InboxSnapshot is the `inbox` topic projection (§3): open asks + proposed
// approvals + OPEN watchdog flags (B5-3) + RED conformance cards (B5-4) + open
// drift cards (B5-6A).
type InboxSnapshot struct {
	Asks             []InboxAsk             `json:"asks"`
	Approvals        []InboxApproval        `json:"approvals"`
	WatchdogFlags    []InboxWatchdogFlag    `json:"watchdog_flags"`
	ConformanceCards []InboxConformanceCard `json:"conformance_cards"`
	DriftCards       []InboxDriftCard       `json:"drift_cards"`
}

func (p *projector) inbox(ctx context.Context, scope ownerScope) (InboxSnapshot, error) {
	out := InboxSnapshot{Asks: []InboxAsk{}, Approvals: []InboxApproval{}, WatchdogFlags: []InboxWatchdogFlag{}, ConformanceCards: []InboxConformanceCard{}, DriftCards: []InboxDriftCard{}}

	aq := `SELECT ask_id, run_id, status, observed_ts FROM asks WHERE answered_ts IS NULL`
	aargs := []any{}
	if !scope.Operator {
		aq += ` AND user_id = ?`
		aargs = append(aargs, scope.UserID)
	}
	aq += ` ORDER BY observed_ts, ask_id`
	arows, err := p.db.QueryContext(ctx, aq, aargs...)
	if err != nil {
		return out, fmt.Errorf("projection: inbox asks: %w", err)
	}
	defer arows.Close()
	for arows.Next() {
		var a InboxAsk
		var observed string
		if err := arows.Scan(&a.AskID, &a.RunID, &a.Kind, &observed); err != nil {
			return out, fmt.Errorf("projection: inbox ask scan: %w", err)
		}
		a.ObservedTS = parseTS(observed)
		out.Asks = append(out.Asks, a)
	}
	if err := arows.Err(); err != nil {
		return out, err
	}

	eq := `SELECT effect_id, COALESCE(run_id, ''), class, state FROM effects WHERE state = 'proposed'`
	eargs := []any{}
	if !scope.Operator {
		eq += ` AND user_id = ?`
		eargs = append(eargs, scope.UserID)
	}
	eq += ` ORDER BY created_ts, effect_id`
	erows, err := p.db.QueryContext(ctx, eq, eargs...)
	if err != nil {
		return out, fmt.Errorf("projection: inbox approvals: %w", err)
	}
	defer erows.Close()
	for erows.Next() {
		var ap InboxApproval
		if err := erows.Scan(&ap.EffectID, &ap.RunID, &ap.Class, &ap.State); err != nil {
			return out, fmt.Errorf("projection: inbox approval scan: %w", err)
		}
		out.Approvals = append(out.Approvals, ap)
	}
	if err := erows.Err(); err != nil {
		return out, err
	}

	flags, err := p.watchdogFlags(ctx, scope)
	if err != nil {
		return out, err
	}
	out.WatchdogFlags = flags

	cards, err := p.conformanceCards(ctx, scope)
	if err != nil {
		return out, err
	}
	out.ConformanceCards = cards

	drift, err := p.driftCards(ctx, scope)
	if err != nil {
		return out, err
	}
	out.DriftCards = drift
	return out, nil
}

// driftCards derives the open drift cards from the `drift.finding` rows
// (B5-6A, S14.6 ¶2). Findings are read in seq order and folded by incident
// fingerprint: a finding joins the fingerprint's open card while it falls
// inside driftIncidentWindow of that card's FIRST finding, and opens a new card
// otherwise — N hits in one storm are one card, and a fresh storm is a fresh
// card. Owner-scoped per S01.9: findings are platform-scope, so a member's
// filter matches none and only the operator sees them.
func (p *projector) driftCards(ctx context.Context, scope ownerScope) ([]InboxDriftCard, error) {
	// Bounded at the source: only findings inside the horizon are candidates,
	// and an `none`-class row is never a card. The producer already declines to
	// emit `none` (S14.6 ¶2: RELEVANT hits become drift cards); this is the belt
	// that also covers any row written before that rule existed.
	q := `SELECT event_seq, payload, ts FROM run_events
	        WHERE type = 'drift.finding'
	          AND ts >= ?
	          AND COALESCE(json_extract(payload, '$.change_class'), '') <> 'none'`
	args := []any{p.clock().Add(-driftCardHorizon).UTC().Format(time.RFC3339Nano)}
	if !scope.Operator {
		q += ` AND user_id = ?`
		args = append(args, scope.UserID)
	}
	q += ` ORDER BY event_seq` // ascending: the first row of a window opens its card
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("projection: drift cards: %w", err)
	}
	defer rows.Close()

	open := map[string]*InboxDriftCard{}
	out := []InboxDriftCard{}
	// cards holds every card in open order; open[] points into it by
	// fingerprint so a fold updates the card already in the list.
	var cards []*InboxDriftCard
	for rows.Next() {
		var (
			seq         int64
			payload, ts string
		)
		if err := rows.Scan(&seq, &payload, &ts); err != nil {
			return nil, fmt.Errorf("projection: drift card scan: %w", err)
		}
		var f struct {
			Source       string   `json:"source"`
			Lanes        []string `json:"lanes"`
			ChangeClass  string   `json:"change_class"`
			Severity     string   `json:"severity"`
			Summary      string   `json:"summary"`
			Fingerprint  string   `json:"fingerprint"`
			RowID        string   `json:"row_id"`
			Classified   bool     `json:"classified"`
			Revalidation struct {
				Triggered bool   `json:"triggered"`
				Reason    string `json:"reason"`
			} `json:"revalidation"`
		}
		if err := json.Unmarshal([]byte(payload), &f); err != nil {
			// A non-conformant payload never breaks the inbox (S14.2 rule 3
			// forward-tolerance); it is skipped, not fatal.
			continue
		}
		at := parseTS(ts)
		if card, ok := open[f.Fingerprint]; ok && at.Sub(card.FirstTS) < driftIncidentWindow {
			card.Hits++
			card.LatestSeq, card.LatestTS = seq, at
			card.Summary, card.Severity, card.Classified = f.Summary, f.Severity, f.Classified
			card.RevalidationTriggered = card.RevalidationTriggered || f.Revalidation.Triggered
			card.RevalidationNote = f.Revalidation.Reason
			continue
		}
		card := &InboxDriftCard{
			Seq: seq, LatestSeq: seq, Fingerprint: f.Fingerprint, Source: f.Source,
			Lanes: f.Lanes, ChangeClass: f.ChangeClass, Severity: f.Severity,
			Summary: f.Summary, RowID: f.RowID, Hits: 1, Classified: f.Classified,
			RevalidationTriggered: f.Revalidation.Triggered,
			RevalidationNote:      f.Revalidation.Reason,
			FirstTS:               at, LatestTS: at,
		}
		if card.Lanes == nil {
			card.Lanes = []string{}
		}
		open[f.Fingerprint] = card
		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Newest first, then capped: a storm of distinct fingerprints inside one
	// horizon can still only fill the inbox to the cap, and what survives is the
	// most recent — never an arbitrary prefix.
	sort.Slice(cards, func(i, j int) bool { return cards[i].LatestSeq > cards[j].LatestSeq })
	if len(cards) > driftCardCap {
		cards = cards[:driftCardCap]
	}
	for _, c := range cards {
		out = append(out, *c)
	}
	return out, nil
}

// conformanceCards derives the RED conformance-registry cards for the inbox
// (B5-4, S14.5): the registry rows whose last_result is 'red' — a red result
// raises a card (flag-now when lane- or storage-affecting, R14); green raises
// nothing and dueness raises nothing (OQ3). DERIVE rows (restore-drill,
// dead-man) never write last_result to the table, so they never surface here —
// their own subsystems own the alert (OQ2/OQ5(c)), no double-fire. Owner-scoped
// per S01.9: rows are platform-scope, so a member's filter (user_id = <member>)
// matches none and only the operator sees them (R16). internal/api reads the
// table directly — it never imports internal/conformance (the §31 pattern).
func (p *projector) conformanceCards(ctx context.Context, scope ownerScope) ([]InboxConformanceCard, error) {
	q := `SELECT row_id, owning_section, schedule, affect_class, last_run
	        FROM conformance_registry WHERE last_result = 'red'`
	args := []any{}
	if !scope.Operator {
		q += ` AND user_id = ?`
		args = append(args, scope.UserID)
	}
	q += ` ORDER BY row_id`
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("projection: conformance cards: %w", err)
	}
	defer rows.Close()
	out := []InboxConformanceCard{}
	for rows.Next() {
		var c InboxConformanceCard
		var affect string
		var lastRun sql.NullString
		if err := rows.Scan(&c.RowID, &c.OwningSection, &c.Schedule, &affect, &lastRun); err != nil {
			return nil, fmt.Errorf("projection: conformance card scan: %w", err)
		}
		c.AffectClass = affect
		c.FlagNow = affect == "lane" || affect == "storage"
		c.LastResult = "red"
		if lastRun.Valid {
			c.LastRunTS = parseTS(lastRun.String)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// watchdogFlags derives the OPEN watchdog flags for the inbox (B5-3, R30): the
// latest watchdog.flagged per (run, anomaly class), owner-scoped, dropping any
// superseded by a later watchdog.suppressed for the rule or a resume
// (run.state_changed to=running). Pure derive-from-log — no watchdog_flags
// table (S14.1; OQ2). internal/api does not import internal/watchdog: it reads
// the canonical type strings directly.
func (p *projector) watchdogFlags(ctx context.Context, scope ownerScope) ([]InboxWatchdogFlag, error) {
	// latest watchdog.flagged per (run_id, anomaly_class)
	fq := `SELECT event_seq, COALESCE(run_id, ''), user_id, payload, ts
	         FROM run_events WHERE type = 'watchdog.flagged'`
	fargs := []any{}
	if !scope.Operator {
		fq += ` AND user_id = ?`
		fargs = append(fargs, scope.UserID)
	}
	fq += ` ORDER BY event_seq` // ascending: later rows overwrite earlier per key
	frows, err := p.db.QueryContext(ctx, fq, fargs...)
	if err != nil {
		return nil, fmt.Errorf("projection: watchdog flags: %w", err)
	}
	type flagRow struct {
		seq   int64
		runID string
		owner string
		rule  string
		class string
		sev   string
		detl  string
		ts    time.Time
	}
	latest := map[string]flagRow{} // key = runID\x00class
	for frows.Next() {
		var (
			seq          int64
			runID, owner string
			payload, ts  string
		)
		if err := frows.Scan(&seq, &runID, &owner, &payload, &ts); err != nil {
			frows.Close()
			return nil, fmt.Errorf("projection: watchdog flag scan: %w", err)
		}
		if strings.HasPrefix(runID, deadManCanaryPrefix) {
			// The dead-man canary finalizes each day's synthetic run, and only a
			// suppress/resume supersedes a flag — so its synthetic watchdog.loop
			// flag would sit forever-open in the inbox (~30/month). The R23
			// meta-alert already excludes these; the inbox does too (drain D4).
			continue
		}
		pay := json.RawMessage(payload)
		class := firstString(pay, "anomaly_class", "rule")
		fr := flagRow{
			seq: seq, runID: runID, owner: owner,
			rule:  firstString(pay, "rule"),
			class: class,
			sev:   firstString(pay, "severity"),
			detl:  firstString(pay, "detail"),
			ts:    parseTS(ts),
		}
		latest[runID+"\x00"+class] = fr
	}
	frows.Close()
	if err := frows.Err(); err != nil {
		return nil, err
	}
	if len(latest) == 0 {
		return []InboxWatchdogFlag{}, nil
	}

	// supersession sets: a suppress for the rule, or a resume transition for the
	// run, at a higher seq clears the flag. The suppress query includes
	// PLATFORM-scope rows (inclPlatform) so a platform-scope suppress of a
	// run-less flag (spend/organ/retune) clears it in a MEMBER's view too — the
	// member already sees the flag; only the clearing effect crosses, never any
	// other owner's flags (drain D5). The resume query stays strictly
	// owner-scoped (a resume rides its own run, same owner as the flag).
	suppressed, err := p.maxSeqByKey(ctx, scope, true, "watchdog.suppressed", func(runID string, pay json.RawMessage) string {
		return runID + "\x00" + firstString(pay, "rule")
	})
	if err != nil {
		return nil, err
	}
	resumed, err := p.maxSeqByKey(ctx, scope, false, "run.state_changed", func(runID string, pay json.RawMessage) string {
		if firstString(pay, "to") == "running" {
			return runID // any resume clears the run's flags
		}
		return "" // non-resume transitions do not clear
	})
	if err != nil {
		return nil, err
	}

	out := make([]InboxWatchdogFlag, 0, len(latest))
	for _, f := range latest {
		if s, ok := suppressed[f.runID+"\x00"+f.class]; ok && s > f.seq {
			continue
		}
		if f.runID != "" {
			if r, ok := resumed[f.runID]; ok && r > f.seq {
				continue
			}
		}
		out = append(out, InboxWatchdogFlag{
			Seq: f.seq, RunID: f.runID, Owner: f.owner, Rule: f.rule,
			AnomalyClass: f.class, Severity: f.sev, Detail: f.detl, FlaggedTS: f.ts,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

// maxSeqByKey returns the highest event_seq per key for a run_events type,
// owner-scoped. keyFn derives the key from the row's run_id + payload; a "" key
// skips the row. When inclPlatform is set, a member's scope ALSO admits
// platform-scope rows (user_id='platform') — used for the suppress supersession
// so a platform-scope clear of a run-less flag reaches the member's view without
// widening which flags the member sees (drain D5).
func (p *projector) maxSeqByKey(ctx context.Context, scope ownerScope, inclPlatform bool, typ string, keyFn func(runID string, pay json.RawMessage) string) (map[string]int64, error) {
	q := `SELECT event_seq, COALESCE(run_id, ''), payload FROM run_events WHERE type = ?`
	args := []any{typ}
	if !scope.Operator {
		if inclPlatform {
			q += ` AND (user_id = ? OR user_id = 'platform')`
		} else {
			q += ` AND user_id = ?`
		}
		args = append(args, scope.UserID)
	}
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("projection: watchdog supersession %s: %w", typ, err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var seq int64
		var runID, payload string
		if err := rows.Scan(&seq, &runID, &payload); err != nil {
			return nil, err
		}
		k := keyFn(runID, json.RawMessage(payload))
		if k == "" {
			continue
		}
		if seq > out[k] {
			out[k] = seq
		}
	}
	return out, rows.Err()
}

// ── shared read helpers ──

// derivedWedged reports the recovery-side wedged classification (R11): true
// iff the most recent run-lifecycle marker for the run is run.wedged. A later
// transition (resume/tombstone/finalize appends a run.state_changed with a
// higher event_seq) supersedes it. NEVER a stored column.
func (p *projector) derivedWedged(ctx context.Context, runID string) bool {
	var typ string
	err := p.db.QueryRowContext(ctx,
		`SELECT type FROM run_events
		 WHERE run_id = ? AND type IN ('run.wedged', 'run.state_changed', 'run.state')
		 ORDER BY event_seq DESC LIMIT 1`, runID).Scan(&typ)
	return err == nil && typ == "run.wedged"
}

// lastActivity reads the latest run_events row for the run and summarizes it.
func (p *projector) lastActivity(ctx context.Context, runID string) (Activity, bool) {
	var (
		a       Activity
		tsRaw   string
		payload string
	)
	err := p.db.QueryRowContext(ctx,
		`SELECT type, ts, payload FROM run_events WHERE run_id = ? ORDER BY event_seq DESC LIMIT 1`, runID).
		Scan(&a.Type, &tsRaw, &payload)
	if err != nil {
		return Activity{}, false
	}
	a.TS = parseTS(tsRaw)
	a.Line = firstString(json.RawMessage(payload), "line", "summary", "message", "reason", "detail")
	return a, true
}

// latestPayload returns the payload of the newest run_events row for the run
// whose type is in types.
func (p *projector) latestPayload(ctx context.Context, runID string, types ...string) (json.RawMessage, bool) {
	q, args := latestTypeQuery(runID, types)
	var payload string
	if err := p.db.QueryRowContext(ctx, q, args...).Scan(&payload); err != nil {
		return nil, false
	}
	return json.RawMessage(payload), true
}

// latestPayloadType is latestPayload that also records the matched event type
// into *typ (the stage marker's identity is a useful default label).
func (p *projector) latestPayloadType(ctx context.Context, runID string, typ *string, types ...string) (json.RawMessage, bool) {
	q, args := latestTypeQueryWithType(runID, types)
	var payload, matched string
	if err := p.db.QueryRowContext(ctx, q, args...).Scan(&matched, &payload); err != nil {
		return nil, false
	}
	if typ != nil {
		*typ = matched
	}
	return json.RawMessage(payload), true
}

func latestTypeQuery(runID string, types []string) (string, []any) {
	q := `SELECT payload FROM run_events WHERE run_id = ? AND type IN (` + placeholders(len(types)) +
		`) ORDER BY event_seq DESC LIMIT 1`
	return q, append([]any{runID}, toAny(types)...)
}

func latestTypeQueryWithType(runID string, types []string) (string, []any) {
	q := `SELECT type, payload FROM run_events WHERE run_id = ? AND type IN (` + placeholders(len(types)) +
		`) ORDER BY event_seq DESC LIMIT 1`
	return q, append([]any{runID}, toAny(types)...)
}

func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	b := make([]byte, 0, n*2)
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '?')
	}
	return string(b)
}

func toAny(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// firstString unmarshals a JSON object payload and returns the first present
// non-empty string value among keys; "" if none (or non-object payload).
func firstString(payload json.RawMessage, keys ...string) string {
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// parseTS parses a stored timestamp (RFC3339Nano, then RFC3339); zero on
// failure. Stored ts is never an ordering authority (S02.5) — this is display.
func parseTS(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

// elapsedSeconds returns now − created_ts in seconds, clamped ≥ 0 (monotonic
// display counter; never derived from an untrusted single clock for ordering).
func elapsedSeconds(createdRaw string) int64 {
	t := parseTS(createdRaw)
	if t.IsZero() {
		return 0
	}
	d := time.Since(t)
	if d < 0 {
		return 0
	}
	return int64(d.Seconds())
}

// nullInt / nullFloat scan a nullable numeric column into a target, defaulting
// to zero (the runs ceiling columns are NULL when unset at B0).
type nullInt struct{ dst *int64 }

func (n nullInt) Scan(v any) error {
	if v == nil {
		*n.dst = 0
		return nil
	}
	switch t := v.(type) {
	case int64:
		*n.dst = t
	case int:
		*n.dst = int64(t)
	default:
		return fmt.Errorf("projection: nullInt: unexpected %T", v)
	}
	return nil
}

type nullFloat struct{ dst *float64 }

func (n nullFloat) Scan(v any) error {
	if v == nil {
		*n.dst = 0
		return nil
	}
	switch t := v.(type) {
	case float64:
		*n.dst = t
	case int64:
		*n.dst = float64(t)
	default:
		return fmt.Errorf("projection: nullFloat: unexpected %T", v)
	}
	return nil
}

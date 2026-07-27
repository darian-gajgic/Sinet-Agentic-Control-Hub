package benchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// gate.go is BENCH-REG §11 (the 15.3 gate) and §12 (the alarm).
//
// S14.7 fixes this section's posture in one clause: "this section EVALUATES and
// DISPLAYS; it SETS NOTHING." That is implemented literally. Gate evaluation
// writes exactly one thing — the domain's durable first-gate-opened marker,
// which selects the §4.1 sampling phase and is read by nothing outside this
// package — and touches no other table, no run, no queue and no setting. The
// expansion freeze binds by VISIBILITY and by gate limb (c), never by killing
// or parking anything: nothing is auto-killed and running work is untouched
// (§12; G1 D1.3).

// AlarmSeverity is the S14.4 alert-routing class an alarm card carries. §12
// names it: the alarm is a FLAG-NOW card. It is the same wire vocabulary
// internal/watchdog and internal/watchlist emit, so the platform's three
// alert producers never disagree on a value.
const AlarmSeverity = "flag-now"

// LimbResult is one gate limb's evaluation.
type LimbResult struct {
	// Limb is the BENCH-REG §11 letter: a, b, c or d.
	Limb string `json:"limb"`
	Name string `json:"name"`
	Ref  string `json:"ref"`
	// Evaluable is false when the limb cannot honestly be judged — limb (d) on a
	// domain with no registered suites, an unfloored version, or no seam. An
	// unevaluable limb never passes: "the gate cannot be evaluated — and
	// therefore cannot open" (BENCH-REG §10.1(d)).
	Evaluable bool   `json:"evaluable"`
	Pass      bool   `json:"pass"`
	Reason    string `json:"reason"`
}

// GateEvaluation is BENCH-REG §11 for one launch domain over its CURRENT epoch.
type GateEvaluation struct {
	Domain  string `json:"domain"`
	EpochID string `json:"epoch_id"`
	// Open is the conjunction of all four limbs. It is a READING, not a switch:
	// nothing in the platform is gated by this package writing anything.
	Open   bool          `json:"open"`
	Limbs  []LimbResult  `json:"limbs"`
	Rate   PublishedRate `json:"rate"`
	Marker string        `json:"marker"`
}

// Gate evaluates the four registered limbs for a domain over its current epoch
// (§9: gate and alarm evaluate over the current epoch only).
//
// When every limb passes AND the domain has never opened before, the durable
// first-open marker is written in the same transaction as the evaluation that
// opened it (the marker cannot be re-derived later: limb (d) depends on external
// mutable suite state, so a suite that goes red tomorrow must not retroactively
// un-open the past). That write is the ONLY state this evaluation produces.
func (p *Practice) Gate(ctx context.Context, domain string) (GateEvaluation, error) {
	st, err := p.Store.EnsureDomain(ctx, domain)
	if err != nil {
		return GateEvaluation{}, err
	}
	a, err := p.Store.Accrual(ctx, domain, st.CurrentEpochID)
	if err != nil {
		return GateEvaluation{}, err
	}
	rate, err := publishedRate(a, nil)
	if err != nil {
		return GateEvaluation{}, err
	}

	limbA := LimbResult{
		Limb: "a", Name: "minimum non-tied pairs", Ref: "BENCH-REG §11(a)", Evaluable: true,
		Pass:   rate.N >= GateMinNonTiedPairs,
		Reason: fmt.Sprintf("m = %d non-tied pairs in %s (registered minimum %d)", rate.N, st.CurrentEpochID, GateMinNonTiedPairs),
	}
	limbB := LimbResult{
		Limb: "b", Name: "posterior floor", Ref: "BENCH-REG §11(b)", Evaluable: true,
		Pass:   rate.G >= GateGFloor,
		Reason: fmt.Sprintf("G = %.3f (registered floor %.2f)", rate.G, GateGFloor),
	}

	alarm, err := p.AlarmState(ctx, domain)
	if err != nil {
		return GateEvaluation{}, err
	}
	limbC := LimbResult{
		Limb: "c", Name: "no active alarm", Ref: "BENCH-REG §11(c), §12", Evaluable: true,
		Pass: !alarm.Standing, Reason: alarm.Summary,
	}
	if !alarm.Standing {
		limbC.Reason = "no alarm is standing in this domain"
	}

	limbD := p.floorLimb(ctx, domain)

	ev := GateEvaluation{
		Domain: domain, EpochID: st.CurrentEpochID, Rate: rate,
		Limbs: []LimbResult{limbA, limbB, limbC, limbD}, Marker: RegisteredMarker,
	}
	ev.Open = true
	for _, l := range ev.Limbs {
		if !l.Evaluable || !l.Pass {
			ev.Open = false
		}
	}
	if ev.Open && st.FirstGateOpened.IsZero() {
		if err := p.noteFirstOpen(ctx, domain); err != nil {
			return GateEvaluation{}, err
		}
		p.logger().Info("benchmark: gate opened for the first time — sampling moves to the maintenance phase (BENCH-REG §4.1)",
			"domain", domain, "epoch", st.CurrentEpochID, "m", rate.N, "g", rate.G)
	}
	return ev, nil
}

// floorLimb evaluates §11(d) through the shell-composed S14.8 seam. An absent
// seam, a seam error, or an unevaluable report all yield "not evaluable — the
// gate cannot open", with the reason named. It is NEVER a fake green: BENCH-REG
// §10.1(d) says an unfloored or red suite means the gate cannot be evaluated,
// and "cannot be evaluated" is a distinct answer from "failed".
func (p *Practice) floorLimb(ctx context.Context, domain string) LimbResult {
	l := LimbResult{Limb: "d", Name: "regression suites green at registered floors", Ref: "BENCH-REG §11(d), §10.1(d)"}
	if p.Floors == nil {
		l.Reason = "not evaluable — no regression-suite surface is composed, so the gate cannot open (BENCH-REG §10.1(d))"
		return l
	}
	rep, err := p.Floors(ctx, domain)
	if err != nil {
		l.Reason = "not evaluable — reading the domain's registered floors failed: " + err.Error()
		return l
	}
	if !rep.Evaluable {
		l.Reason = "not evaluable — " + rep.Reason
		return l
	}
	l.Evaluable, l.Pass, l.Reason = true, rep.Green, rep.Reason
	return l
}

// noteFirstOpen records the durable first-open marker.
//
// A DECLARED READING of the OQ3 disposition's "same tx as the evaluation": a
// literal same-transaction write is not reachable here and never could be. Limb
// (d) reads EXTERNAL, independently mutable suite state through a func seam, so
// the evaluation is not a single transaction to join — wrapping it in one would
// hold a write transaction open across an out-of-package call, which S02.1's
// read hygiene forbids for good reason. What the disposition ASKS for is
// achieved exactly: the marker is durable, and it opens once. The conditional
// (`WHERE first_gate_opened_ts IS NULL`) makes the write idempotent under
// concurrent evaluations, and migration 0014'"'"'s write-once trigger is the belt
// beneath it — the marker is history and never moves.
func (p *Practice) noteFirstOpen(ctx context.Context, domain string) error {
	return p.Store.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE benchmark_domains SET first_gate_opened_ts = ?
			  WHERE domain = ? AND first_gate_opened_ts IS NULL`,
			p.now().Format(time.RFC3339Nano), domain)
		if err != nil {
			return fmt.Errorf("benchmark: record first gate open for %q: %w", domain, err)
		}
		return nil
	})
}

// PlatformGate is 15.3: "The 15.3 gate opens when every registered launch domain
// passes." At v0 the accruing roster is software-development alone; web-research
// accrues from v0.1 ship (§10).
type PlatformGate struct {
	Open    bool             `json:"open"`
	Domains []GateEvaluation `json:"domains"`
	// Held names the 14.5 breadth restrictions that stand until the gate opens.
	Held   string `json:"held,omitempty"`
	Marker string `json:"marker"`
}

// heldNote is 15.3/14.5's standing restriction, rendered for display. It is a
// STATEMENT: breadth is an operator act, so this package makes the hold visible
// and never enforces it by refusing anything.
const heldNote = "until the 15.3 gate opens, the 14.5 breadth restrictions stand: no new domains beyond the " +
	"launch ladder, no decorative surfaces. Degraded-mode domains (2.1/7.6) remain allowed from launch — they " +
	"are not \"breadth\" in 14.5's sense and are outside this benchmark entirely."

// PlatformGate evaluates the 15.3 gate across the v0 launch roster.
func (p *Practice) PlatformGate(ctx context.Context) (PlatformGate, error) {
	out := PlatformGate{Open: true, Marker: RegisteredMarker}
	for _, d := range V0LaunchDomains() {
		ev, err := p.Gate(ctx, d)
		if err != nil {
			return PlatformGate{}, err
		}
		out.Domains = append(out.Domains, ev)
		if !ev.Open {
			out.Open = false
		}
	}
	if !out.Open {
		out.Held = heldNote
	}
	return out, nil
}

// ── The alarm (BENCH-REG §12) ────────────────────────────────────────────────

// AlarmPayload is a benchmark.alarm event: a raise carries the full record that
// tripped it, a clear carries the operator's disposition.
type AlarmPayload struct {
	Action  string `json:"action"`
	Domain  string `json:"domain"`
	EpochID string `json:"epoch_id"`
	// Severity is the S14.4 alert-routing class: §12 makes the alarm a
	// flag-now card.
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	// LossG and Threshold are the trip condition as measured and as registered.
	LossG     float64 `json:"loss_g"`
	Threshold float64 `json:"threshold"`
	// Rate is "the full record" §12 says the card carries.
	Rate PublishedRate `json:"rate"`
	// ExpansionFreeze marks the 14.5 hold this alarm imposes while it stands.
	ExpansionFreeze bool `json:"expansion_freeze"`
	// PairID is the pair whose record tripped the alarm (raise only).
	PairID string `json:"pair_id,omitempty"`
	// Disposition and Actor are the operator's clearing act (clear only).
	Disposition string `json:"disposition,omitempty"`
	Actor       string `json:"actor,omitempty"`
	Reason      string `json:"reason,omitempty"`
	// Retention: alarm history is keep-forever (§12).
	Retention string `json:"retention"`
}

// The §12 dispositions an operator may take on an alarm card: "investigate,
// fix-and-continue accruing, or re-register with rationale". The set is the
// registration's, so it is closed here rather than free text.
const (
	DispositionInvestigate  = "investigate"
	DispositionFixAndAccrue = "fix-and-continue-accruing"
	DispositionReregister   = "re-register"
)

func validDisposition(d string) bool {
	switch d {
	case DispositionInvestigate, DispositionFixAndAccrue, DispositionReregister:
		return true
	}
	return false
}

// AlarmState is the derived standing state of a domain's alarm.
type AlarmState struct {
	Domain   string `json:"domain"`
	Standing bool   `json:"standing"`
	// Seq is the raising event's sequence — the card's identity.
	Seq       int64         `json:"seq,omitempty"`
	EpochID   string        `json:"epoch_id,omitempty"`
	Severity  string        `json:"severity,omitempty"`
	Summary   string        `json:"summary"`
	LossG     float64       `json:"loss_g,omitempty"`
	Threshold float64       `json:"threshold,omitempty"`
	Rate      PublishedRate `json:"rate,omitempty"`
	RaisedTS  time.Time     `json:"raised_ts,omitempty"`
}

// AlarmState derives a domain's alarm state from the log — the latest
// benchmark.alarm raise not yet superseded by a clear. There is no alarm table:
// alarm history is keep-forever and the event log serves that natively (the
// derive-from-log discipline, CONVENTIONS §31).
func (p *Practice) AlarmState(ctx context.Context, domain string) (AlarmState, error) {
	var st AlarmState
	err := p.Store.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		st, err = p.alarmStateTx(ctx, tx, domain)
		return err
	})
	return st, err
}

func (p *Practice) alarmStateTx(ctx context.Context, tx *sql.Tx, domain string) (AlarmState, error) {
	st := AlarmState{Domain: domain, Summary: "no alarm is standing in this domain"}
	var (
		seq     int64
		payload string
		ts      string
	)
	err := tx.QueryRowContext(ctx,
		`SELECT event_seq, payload, ts FROM run_events
		  WHERE type = ? AND json_extract(payload, '$.domain') = ?
		  ORDER BY event_seq DESC LIMIT 1`, EventAlarm, domain).Scan(&seq, &payload, &ts)
	if err == sql.ErrNoRows {
		return st, nil
	}
	if err != nil {
		return st, fmt.Errorf("benchmark: read alarm state for %q: %w", domain, err)
	}
	var ap AlarmPayload
	if err := json.Unmarshal([]byte(payload), &ap); err != nil {
		// A non-conformant payload never breaks the readout (S14.2 rule 3); it
		// also never silently clears an alarm, so the safe reading is "the
		// latest alarm event is unreadable" — reported, not assumed benign.
		st.Summary = "the latest benchmark.alarm row is unreadable — treat the alarm state as unknown and inspect the log"
		st.Standing = true
		st.Seq = seq
		return st, nil
	}
	if ap.Action == AlarmClear {
		return st, nil
	}
	st.Standing, st.Seq, st.EpochID = true, seq, ap.EpochID
	st.Severity, st.Summary = ap.Severity, ap.Summary
	st.LossG, st.Threshold, st.Rate = ap.LossG, ap.Threshold, ap.Rate
	st.RaisedTS = parseTS(ts)
	return st, nil
}

// evaluateAlarmTx applies §12's trigger inside the record transaction: "at any
// per-pair update, 1 − G > 0.95 (platform losing) in any launch domain's current
// epoch". An alarm already standing is not re-raised — the alarm STANDS while
// the condition holds, and one card per condition is what "a flag-now card"
// means.
//
// The condition ceasing to hold does NOT clear the alarm: §12 says clearing
// requires an operator disposition, logged as a decision event. That reading is
// deliberate and is the conservative one — a losing streak that drifts back over
// the line is exactly the case an operator should look at rather than have
// silently forgiven.
func (p *Practice) evaluateAlarmTx(ctx context.Context, tx *sql.Tx, domain, epochID string) error {
	a, err := p.Store.accrualTx(ctx, tx, domain, epochID)
	if err != nil {
		return err
	}
	rate, err := publishedRate(a, nil)
	if err != nil {
		return err
	}
	post := Posterior{W: rate.W, M: rate.N, G: rate.G, LossG: rate.LossG}
	if !post.AlarmCondition() {
		return nil
	}
	st, err := p.alarmStateTx(ctx, tx, domain)
	if err != nil {
		return err
	}
	if st.Standing {
		return nil
	}
	payload, err := json.Marshal(AlarmPayload{
		Action: AlarmRaise, Domain: domain, EpochID: epochID, Severity: AlarmSeverity,
		Summary: fmt.Sprintf("the platform is losing in %s: 1−G = %.3f exceeds the registered %.2f at w=%d, m=%d (BENCH-REG §12)",
			domain, rate.LossG, AlarmThreshold, rate.W, rate.N),
		LossG: rate.LossG, Threshold: AlarmThreshold, Rate: rate,
		ExpansionFreeze: true, Retention: "keep-forever",
	})
	if err != nil {
		return err
	}
	if _, err := p.Log.AppendTx(ctx, tx, eventlog.Append{
		UserID: PlatformScope, Type: EventAlarm, SchemaVersion: eventSchemaVersion,
		Payload: payload, Time: p.now(),
	}); err != nil {
		return err
	}
	p.logger().Warn("benchmark: ALARM raised — the platform is losing its own benchmark (BENCH-REG §12)",
		"domain", domain, "epoch", epochID, "loss_g", rate.LossG, "w", rate.W, "m", rate.N)
	return nil
}

// DisposeAlarm is the OPERATOR act that clears a standing alarm (§12:
// "the card requires an operator disposition (logged as a decision event) —
// investigate, fix-and-continue accruing, or re-register with rationale").
//
// The actor must hold the one S01.9 role bit, checked against the users row in
// the data layer. §12 says "operator disposition", and the alarm exists because
// the platform may be losing its own benchmark: letting any principal clear it
// would make the one signal designed to be hard to ignore trivially ignorable.
// The refusal is package-level, exactly as the §4.2.1 consent flip's is — B6's
// HTTP surface is an additional gate, never the only one.
//
// The disposition is logged as BOTH a decision.recorded (it IS a human decision,
// S14.2 family 5) and a benchmark.alarm clear (it IS alarm history, keep-forever
// under §12), in one transaction. Nothing about clearing an alarm kills, parks
// or resumes anything — the freeze it lifts was a visibility state, never an
// enforcement.
func (p *Practice) DisposeAlarm(ctx context.Context, actor, domain, disposition, reason string) error {
	switch {
	case actor == "":
		return fmt.Errorf("%w: a disposition needs its acting principal (S02.2 acting-principal-rides-the-payload)", ErrBadInput)
	case !validDisposition(disposition):
		return fmt.Errorf("%w: %q is not a BENCH-REG §12 disposition (investigate | fix-and-continue-accruing | re-register)", ErrBadInput, disposition)
	}
	return p.Store.db.WriteTx(ctx, func(tx *sql.Tx) error {
		operator, err := p.Store.isOperatorTx(ctx, tx, actor)
		if err != nil {
			return err
		}
		if !operator {
			return fmt.Errorf("%w: %q is not the operator — a benchmark alarm is cleared by an OPERATOR disposition (BENCH-REG §12; S01.9)", ErrBadInput, actor)
		}
		st, err := p.alarmStateTx(ctx, tx, domain)
		if err != nil {
			return err
		}
		if !st.Standing {
			return fmt.Errorf("%w: %q", ErrNoAlarm, domain)
		}
		now := p.now()
		decision, err := json.Marshal(DecisionPayload{
			Actor: actor, CardID: fmt.Sprintf("benchmark-alarm:%s:%d", domain, st.Seq),
			CardType: "benchmark.alarm", Decision: disposition, Reason: reason,
			PresentedAt: st.RaisedTS.Format(time.RFC3339Nano),
			DecidedAt:   now.Format(time.RFC3339Nano),
			LatencyS:    int64(now.Sub(st.RaisedTS).Seconds()),
			Subject:     domain, Ref: "BENCH-REG §12",
		})
		if err != nil {
			return err
		}
		if _, err := p.Log.AppendTx(ctx, tx, eventlog.Append{
			UserID: PlatformScope, Type: EventDecision, SchemaVersion: eventSchemaVersion,
			Payload: decision, Time: now,
		}); err != nil {
			return err
		}
		clear, err := json.Marshal(AlarmPayload{
			Action: AlarmClear, Domain: domain, EpochID: st.EpochID, Severity: AlarmSeverity,
			Summary:     fmt.Sprintf("alarm in %s dispositioned %q by %s (BENCH-REG §12)", domain, disposition, actor),
			LossG:       st.LossG,
			Threshold:   AlarmThreshold,
			Disposition: disposition, Actor: actor, Reason: reason,
			ExpansionFreeze: false, Retention: "keep-forever",
		})
		if err != nil {
			return err
		}
		_, err = p.Log.AppendTx(ctx, tx, eventlog.Append{
			UserID: PlatformScope, Type: EventAlarm, SchemaVersion: eventSchemaVersion,
			Payload: clear, Time: now,
		})
		return err
	})
}

// FreezeState is §12's expansion freeze: "no new domain, surface, or breadth
// work while the alarm stands (14.5 hold)".
//
// It is a STANDING, QUERYABLE STATE and nothing else. Breadth is an operator
// act — there is no code path a freeze could block — so the freeze binds by
// being visible and by gate limb (c), exactly as §12 describes. Nothing is
// auto-killed and running work is untouched (D1.3).
type FreezeState struct {
	Standing bool `json:"standing"`
	// Domains names the domains whose alarms hold the freeze.
	Domains []string `json:"domains,omitempty"`
	Summary string   `json:"summary"`
	Ref     string   `json:"ref"`
}

// freezeNote states what the freeze holds and what it deliberately does not.
const freezeNote = "an expansion freeze stands: no new domain, surface, or breadth work while a benchmark alarm " +
	"is undispositioned (BENCH-REG §12; the 14.5 hold). Nothing is auto-killed and running work is untouched — " +
	"breadth is an operator act, so this hold binds by visibility and by gate limb (c)."

// ExpansionFreeze reports the standing §12 freeze across the launch roster.
func (p *Practice) ExpansionFreeze(ctx context.Context) (FreezeState, error) {
	out := FreezeState{Ref: "BENCH-REG §12", Summary: "no expansion freeze is standing"}
	for _, d := range V0LaunchDomains() {
		st, err := p.AlarmState(ctx, d)
		if err != nil {
			return FreezeState{}, err
		}
		if st.Standing {
			out.Standing = true
			out.Domains = append(out.Domains, d)
		}
	}
	if out.Standing {
		out.Summary = freezeNote
	}
	return out, nil
}

// DecisionPayload is the S14.2 Human-decision contract minimum — "actor, card id
// + type, decision, presented-at → decided-at (latency)" — for the two acts this
// package produces: the §12 alarm disposition and the §4.2.1 opt-in consent flip.
type DecisionPayload struct {
	Actor       string `json:"actor"`
	CardID      string `json:"card_id"`
	CardType    string `json:"card_type"`
	Decision    string `json:"decision"`
	PresentedAt string `json:"presented_at"`
	DecidedAt   string `json:"decided_at"`
	LatencyS    int64  `json:"latency_s"`
	Reason      string `json:"reason,omitempty"`
	// Subject is what the decision was about (a domain, a user).
	Subject string `json:"subject,omitempty"`
	// Ref cites the registration clause the decision discharges.
	Ref string `json:"ref,omitempty"`
}

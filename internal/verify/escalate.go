package verify

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// Escalation (Spec S07.7): every verification finding terminates in a
// human-visible sink — a decision card, an operator alert, or a worker-flaw
// ticket. NO OTHER SINK TYPE EXISTS, so "a finding that died in a log" is
// structurally impossible — and the claim is proven by the planted-defect
// test class (escalate_test.go), not assumed (5.6; P46). Escalations enter
// the approval inbox (asks rows, Spec S02.2) as structured, resumable asks;
// answering resumes the pipeline in place (4.3; resume wiring B2-4/S15).

// Category is a route-table category (Spec S07.7; the enum is
// [coordinator-draft], the sink-types-only rule is ratified).
type Category string

const (
	CatACBlocker      Category = "AC-BLOCKER"
	CatSanityBlocker  Category = "SANITY-BLOCKER"
	CatReopenSpec     Category = "REOPEN-SPEC"
	CatCheckIntegrity Category = "CHECK-INTEGRITY"
	CatResearchNotRun Category = "RESEARCH-NOT-RUN"
	CatCapHit         Category = "CAP-HIT"
	CatWorkerFlaw     Category = "WORKER-FLAW"
	CatSafety         Category = "SAFETY"
)

// Sink is a route sink type. The three below are the ONLY sink types (Spec
// S07.7, ratified).
type Sink string

const (
	SinkDecisionCard Sink = "decision_card"
	SinkAlert        Sink = "operator_alert"
	SinkFlawTicket   Sink = "worker_flaw_ticket"
)

// SLAClass is the ratified SLA class set (G1 Def.3 as superseded at close).
type SLAClass string

const (
	// SLAApproval: remind at ⚙ verification.card_remind_hours, push-notify
	// at ⚙ verification.card_push_hours.
	SLAApproval SLAClass = "approval"
	// SLASafety: push immediately, re-ping at
	// ⚙ verification.safety_reping_hours.
	SLASafety SLAClass = "safety"
)

// Route is one route-table row.
type Route struct {
	Category Category
	Sink     Sink
	SLA      SLAClass
	// RaisedBy documents the ratified raisers (Spec S07.7 table).
	RaisedBy string
}

// RouteTable is the S07.7 route table — total over the category enum; a
// conformance test asserts totality and sink-types-only.
var RouteTable = map[Category]Route{
	CatACBlocker:      {Category: CatACBlocker, Sink: SinkDecisionCard, SLA: SLAApproval, RaisedBy: "axis 1; rework round (S07.6), at cap → requester decision card"},
	CatSanityBlocker:  {Category: CatSanityBlocker, Sink: SinkDecisionCard, SLA: SLAApproval, RaisedBy: "axis 2, short of spec doubt; rework round, at cap → requester decision card"},
	CatReopenSpec:     {Category: CatReopenSpec, Sink: SinkDecisionCard, SLA: SLAApproval, RaisedBy: "axis 2; → S06.9 delta on accept"},
	CatCheckIntegrity: {Category: CatCheckIntegrity, Sink: SinkDecisionCard, SLA: SLAApproval, RaisedBy: "executor test-challenge; judge–check disagreement; flake quarantine; failed suite audit"},
	CatResearchNotRun: {Category: CatResearchNotRun, Sink: SinkDecisionCard, SLA: SLAApproval, RaisedBy: "S07.3 counter check, after its retry"},
	CatCapHit:         {Category: CatCapHit, Sink: SinkDecisionCard, SLA: SLAApproval, RaisedBy: "S07.6 stop rules; card carries best-effort state + round history"},
	CatWorkerFlaw:     {Category: CatWorkerFlaw, Sink: SinkFlawTicket, SLA: SLAApproval, RaisedBy: "recurring defect pattern attributable to a worker (detector is S08/S14's)"},
	CatSafety:         {Category: CatSafety, Sink: SinkAlert, SLA: SLASafety, RaisedBy: "watchdog pause-and-flag (G1 D1.3, S14); confinement violation (S11)"},
}

// SLA is the computed reminder schedule of one card, from the ratified ⚙
// set (Spec S07.7; inbox-wide numbers consumed by the S15 inbox surface).
type SLA struct {
	Class SLAClass `json:"class"`
	// Approval class.
	RemindAfterHours int64 `json:"remind_after_hours,omitempty"`
	PushAfterHours   int64 `json:"push_after_hours,omitempty"`
	// Safety class.
	PushImmediately  bool  `json:"push_immediately,omitempty"`
	RepingEveryHours int64 `json:"reping_every_hours,omitempty"`
}

// Card is one escalation sink snapshot — the full, resumable ask content
// (asks.snapshot). Choices per category are [coordinator-draft] except
// REOPEN-SPEC's ratified three (Spec S07.5).
type Card struct {
	Kind     Sink     `json:"kind"`
	Category Category `json:"category"`
	TaskID   string   `json:"task_id"`
	RunID    string   `json:"run_id"`
	IssuedTS string   `json:"issued_ts"`
	SLA      SLA      `json:"sla"`

	Summary string   `json:"summary"`
	Detail  []string `json:"detail,omitempty"`
	Choices []string `json:"choices,omitempty"`

	Findings []Finding `json:"findings,omitempty"`
	// Rounds is the full round history (CAP-HIT: never a silent stop).
	Rounds []RoundRecord `json:"rounds,omitempty"`
	// BestEffort is the best-effort state carried on a CAP-HIT card.
	BestEffort string `json:"best_effort,omitempty"`
	// Quarantined names a suite/check quarantined pending fix
	// (CHECK-INTEGRITY route).
	Quarantined string `json:"quarantined,omitempty"`
	// Canary marks the dead-man canary's card (Spec S07.7 liveness proof
	// 2); the external watch keys on it.
	Canary bool `json:"canary,omitempty"`
	// TicketRef is the worker-flaw ticket reference (WORKER-FLAW route).
	TicketRef string `json:"ticket_ref,omitempty"`

	AskID string `json:"ask_id"`
}

// REOPEN-SPEC card choices (Spec S07.5, ratified): the requester decides;
// an accepted adjustment lands as an S06.9 delta and re-freezes the ACs.
var reopenSpecChoices = []string{"proceed_as_specified", "adjust_specification", "rethink"}

// Per-category decision-card choices [coordinator-draft].
var cardChoices = map[Category][]string{
	CatReopenSpec:     reopenSpecChoices,
	CatCapHit:         {"accept_best_effort", "revise_with_guidance", "cancel"},
	CatCheckIntegrity: {"fix_suite", "waive_check", "cancel"},
	CatResearchNotRun: {"rerun_research", "supply_fact", "proceed_anyway"},
	CatACBlocker:      {"accept_best_effort", "revise_with_guidance", "cancel"},
	CatSanityBlocker:  {"accept_best_effort", "revise_with_guidance", "cancel"},
}

// WorkerFlaw is one recurring-defect report entering the worker-flaw route
// (7.2/7.3; worker identity and ticket lifecycle are Spec S08's, B3).
type WorkerFlaw struct {
	Worker   string     `json:"worker"`
	Pattern  string     `json:"pattern"`
	Findings []Finding  `json:"findings,omitempty"`
	Key      FindingKey `json:"key,omitempty"`
}

// FlawTickets is the S08 ticket seam (B3). The default stub files the
// ticket as an inbox-visible asks row — the route stays human-visible until
// the real ticket machinery lands; absence never becomes a silent drop.
type FlawTickets interface {
	File(ctx context.Context, tx *sql.Tx, askID string, flaw WorkerFlaw) (ticketRef string, err error)
}

// stubFlawTickets is the B2-3 stub: the ticket IS the ask row (the
// Escalator writes it); the stub only mints the ticket ref.
type stubFlawTickets struct{}

func (stubFlawTickets) File(_ context.Context, _ *sql.Tx, askID string, _ WorkerFlaw) (string, error) {
	return "ticket:" + askID + " (S08 ticket machinery lands at B3; filed to the inbox until then)", nil
}

// Escalation is one escalation entering the router.
type Escalation struct {
	Category Category
	TaskID   string
	RunID    string
	// Owner attributes the ask row (15.6).
	Owner   string
	Summary string
	Detail  []string

	Findings   []Finding
	Rounds     []RoundRecord
	BestEffort string

	Quarantined string
	Flaw        *WorkerFlaw
	Canary      bool
}

// Escalator routes escalations to their ratified sinks: one write
// transaction per escalation — the asks row plus its verify.escalation
// event commit atomically (CONVENTIONS §6/§8 discipline).
type Escalator struct {
	DB       *storage.DB
	Log      *eventlog.Log
	Settings Settings
	// Tickets is the S08 seam; nil uses the inbox-filing stub.
	Tickets FlawTickets
	Now     func() time.Time
}

func (e *Escalator) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// ErrUnroutedCategory would mean a category outside the route table — the
// structurally-impossible sink gap; Raise fails closed on it.
var ErrUnroutedCategory = errors.New("verify: category has no route (Spec S07.7)")

// sla computes the card's reminder schedule from the ratified ⚙ set.
func (e *Escalator) sla(class SLAClass) (SLA, error) {
	s := SLA{Class: class}
	switch class {
	case SLAApproval:
		remind, err := e.Settings.Int(keyCardRemindHours)
		if err != nil {
			return SLA{}, fmt.Errorf("verify: read ⚙ %s: %w", keyCardRemindHours, err)
		}
		push, err := e.Settings.Int(keyCardPushHours)
		if err != nil {
			return SLA{}, fmt.Errorf("verify: read ⚙ %s: %w", keyCardPushHours, err)
		}
		s.RemindAfterHours, s.PushAfterHours = remind, push
	case SLASafety:
		reping, err := e.Settings.Int(keySafetyRepingHours)
		if err != nil {
			return SLA{}, fmt.Errorf("verify: read ⚙ %s: %w", keySafetyRepingHours, err)
		}
		s.PushImmediately, s.RepingEveryHours = true, reping
	default:
		return SLA{}, fmt.Errorf("verify: unknown SLA class %q", class)
	}
	return s, nil
}

// Raise routes one escalation to its ratified sink and returns the durable
// card. The ask row is authoritative from the moment observed (Spec S02.2);
// the verify.escalation event commits in the same transaction.
func (e *Escalator) Raise(ctx context.Context, esc Escalation) (Card, error) {
	route, ok := RouteTable[esc.Category]
	if !ok {
		return Card{}, fmt.Errorf("%w: %q", ErrUnroutedCategory, esc.Category)
	}
	if esc.Owner == "" || esc.RunID == "" {
		return Card{}, fmt.Errorf("%w: escalation requires run and owner (15.6)", ErrBadInput)
	}
	sla, err := e.sla(route.SLA)
	if err != nil {
		return Card{}, err
	}
	askID := "ask-verify-" + randomHex(8)
	if esc.Canary {
		askID = "canary-" + randomHex(8)
	}
	card := Card{
		Kind:        route.Sink,
		Category:    esc.Category,
		TaskID:      esc.TaskID,
		RunID:       esc.RunID,
		IssuedTS:    e.now().UTC().Format(time.RFC3339Nano),
		SLA:         sla,
		Summary:     esc.Summary,
		Detail:      esc.Detail,
		Choices:     cardChoices[esc.Category],
		Findings:    esc.Findings,
		Rounds:      esc.Rounds,
		BestEffort:  esc.BestEffort,
		Quarantined: esc.Quarantined,
		Canary:      esc.Canary,
		AskID:       askID,
	}
	err = e.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if route.Sink == SinkFlawTicket {
			tickets := e.Tickets
			if tickets == nil {
				tickets = stubFlawTickets{}
			}
			if esc.Flaw == nil {
				return fmt.Errorf("%w: WORKER-FLAW escalation without a flaw report", ErrBadInput)
			}
			ref, err := tickets.File(ctx, tx, askID, *esc.Flaw)
			if err != nil {
				return fmt.Errorf("verify: file worker-flaw ticket: %w", err)
			}
			card.TicketRef = ref
		}
		snapshot, err := json.Marshal(card)
		if err != nil {
			return fmt.Errorf("verify: marshal card: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts)
			 VALUES (?, ?, ?, ?, 'open', ?)`,
			askID, esc.RunID, esc.Owner, string(snapshot), card.IssuedTS); err != nil {
			return fmt.Errorf("verify: insert ask %q: %w", askID, err)
		}
		var gen int64
		if err := tx.QueryRowContext(ctx,
			`SELECT generation FROM runs WHERE run_id = ?`, esc.RunID).Scan(&gen); err != nil {
			return fmt.Errorf("verify: read run %q: %w", esc.RunID, err)
		}
		payload, err := json.Marshal(escalationPayload{
			Category: esc.Category, Sink: route.Sink, SLA: sla, AskID: askID,
			Summary: esc.Summary, Canary: esc.Canary, TicketRef: card.TicketRef,
		})
		if err != nil {
			return fmt.Errorf("verify: marshal escalation event: %w", err)
		}
		_, err = e.Log.AppendTx(ctx, tx, eventlog.Append{
			RunID:         esc.RunID,
			Generation:    gen,
			UserID:        esc.Owner,
			Type:          EventEscalation,
			SchemaVersion: verifyEventSchemaVersion,
			Payload:       payload,
		})
		return err
	})
	if err != nil {
		return Card{}, err
	}
	return card, nil
}

// escalationPayload is the verify.escalation event payload.
type escalationPayload struct {
	Category  Category `json:"category"`
	Sink      Sink     `json:"sink"`
	SLA       SLA      `json:"sla"`
	AskID     string   `json:"ask_id"`
	Summary   string   `json:"summary"`
	Canary    bool     `json:"canary,omitempty"`
	TicketRef string   `json:"ticket_ref,omitempty"`
}

// ---- Liveness proofs 2 and 3: canary shape + drill cadence (Spec S07.7) ----
//
// Proof 1 (the planted-defect e2e class) lives in escalate_test.go as
// standing conformance-suite entries. The canary SCHEDULING and the
// watchdog alert wiring are Spec S14's (B5) — flagged there; the task
// shape, the plant, and the silence check live here.

// CanaryPlan returns the fixed synthetic always-escalating task shape (Spec
// S07.7 proof 2): a trivial deliverable whose PLAN carries one research node
// that the metering seam will show as never-run — a deterministic,
// zero-model, zero-allowance plant that drives the real pipeline into a
// RESEARCH-NOT-RUN card. When the local tier lands (B4/S12), the canary
// upgrades to exercise the V2 leg on a local judge seat; the escalation
// path under watch is identical.
func CanaryPlan(taskID, runID string) (Deliverable, []intake.Step, []intake.ResearchNode) {
	d := Deliverable{
		TaskID:   taskID,
		RunID:    runID,
		Domain:   DomainSoftware,
		Type:     "markdown",
		Revision: 1,
		Content:  "# canary\n\nDead-man canary deliverable (Spec S07.7): this task exists to prove the escalation path is alive today.\n",
	}
	steps := []intake.Step{{
		ID:       "S-1",
		Title:    "canary research node",
		DoneWhen: "the canary research node has run",
		Class:    "C1",
		Research: true,
	}}
	nodes := []intake.ResearchNode{{RuleID: "canary", StepID: "S-1"}}
	return d, steps, nodes
}

// zeroResearch is the canary's metering view: the plant — research never
// ran.
type zeroResearch struct{}

func (zeroResearch) ResearchInvocations(context.Context, string, string) (int64, bool, error) {
	return 0, true, nil
}

// CanaryResearchUsage returns the canary plant's ResearchUsage.
func CanaryResearchUsage() ResearchUsage { return zeroResearch{} }

// CanaryWatch is the dead-man switch EXTERNAL to the pipeline (Spec S07.7:
// alert-on-silence — the death of the escalation path is itself what
// fires). Silent reports whether no canary card arrived within
// ⚙ verification.canary_interval_hours; the operator alert it feeds is S14
// wiring (B5).
type CanaryWatch struct {
	DB       *storage.DB
	Settings Settings
}

// Silent returns (true, age) when the newest canary card is older than the
// canary interval — or when none has ever arrived (age < 0 then).
func (w *CanaryWatch) Silent(ctx context.Context, now time.Time) (bool, time.Duration, error) {
	interval, err := w.Settings.Int(keyCanaryHours)
	if err != nil {
		return false, 0, fmt.Errorf("verify: read ⚙ %s: %w", keyCanaryHours, err)
	}
	var ts sql.NullString
	err = w.DB.QueryRowContext(ctx,
		`SELECT MAX(observed_ts) FROM asks WHERE ask_id LIKE 'canary-%'`).Scan(&ts)
	if err != nil {
		return false, 0, fmt.Errorf("verify: read canary cards: %w", err)
	}
	if !ts.Valid || ts.String == "" {
		return true, -1, nil
	}
	last, err := time.Parse(time.RFC3339Nano, ts.String)
	if err != nil {
		return false, 0, fmt.Errorf("verify: parse canary ts %q: %w", ts.String, err)
	}
	age := now.Sub(last)
	return age > time.Duration(interval)*time.Hour, age, nil
}

// DrillCard is the quarterly-drill content contract (Spec S07.7 proof 3, the
// Human-Handoff shape): a planted defect must traverse to the right person
// carrying facts, sources, uncertainty, action history, and a recommended
// next step; pass/fail recorded. Drill scheduling and the pass/fail record
// are S14's (B5).
type DrillCard struct {
	Facts           []string `json:"facts"`
	Sources         []string `json:"sources"`
	Uncertainty     []string `json:"uncertainty"`
	ActionHistory   []string `json:"action_history"`
	RecommendedNext string   `json:"recommended_next"`
}

// DrillDue reports whether the quarterly drill is due
// (⚙ verification.drill_interval_days). lastDrill zero = never drilled =
// due.
func DrillDue(lastDrill, now time.Time, settings Settings) (bool, error) {
	days, err := settings.Int(keyDrillDays)
	if err != nil {
		return false, fmt.Errorf("verify: read ⚙ %s: %w", keyDrillDays, err)
	}
	if lastDrill.IsZero() {
		return true, nil
	}
	return now.Sub(lastDrill) > time.Duration(days)*24*time.Hour, nil
}

package intake

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// Pipeline control state is event-sourced: every boundary appends ONE
// `intake.state` run-event carrying the full (small) state — the
// ledger_update precedent of Spec S02.2. Current state = the latest event
// for the task by event_seq; an interrupted intake reloads it and resumes
// from its last artifact, never by replaying conversation (Spec S06.1,
// D7). Artifact bodies never ride the payload (refs + hashes only,
// P-T07-5); ⚙ state.event_payload_cap bounds the whole write at the event
// log.

// EventState is the intake pipeline state event type. Provisional pending
// the S14 event contract (B5), extending the CONVENTIONS §7/§8 naming
// note.
const EventState = "intake.state"

// EventDeltaDecision is the S06.9 delta measurement hook: presented-delta
// size, time-to-decision, decision, outcome linkage — analysis is the eval
// practice's (Spec S14, P-T05-2). Provisional pending S14.
const EventDeltaDecision = "intake.delta_decision"

const stateSchemaVersion = 1

// Phase is the pipeline phase (Spec S06.1 stage map).
type Phase string

const (
	PhaseInterview Phase = "interview" // Stage 1 side: interviewing / drafting
	PhaseSpine     Phase = "spine"     // Stage 2 pending
	PhaseCritique  Phase = "critique"  // Stage 3 pending
	PhaseApproval  Phase = "approval"  // Stage 4: card open or ready to issue
	PhaseApproved  Phase = "approved"  // terminal: the pipeline ends at the approved plan
	PhaseCancelled Phase = "cancelled" // terminal (4.5)
)

// ReviseReq is one pending bounded revision (applied by the next advance).
type ReviseReq struct {
	Reason      string           `json:"reason"`
	Findings    []string         `json:"findings,omitempty"`
	Resolutions []SlotResolution `json:"resolutions,omitempty"`
}

// EmissionFault is one Stage-1 emission the platform REFUSED — the seam's
// bounded re-emission spent, the refusal still standing — parked on the S06.6
// emission card instead of crashing the run (P3-GF12 R3).
//
// It is durable for the same reason PendingEscalation is: the requester's answer
// arrives later, on a fresh process if need be, and the granted round has to
// re-drive the SAME operation with nothing lost. The refusal is the platform's
// own validator sentence; where a validator quotes the offending field back, the
// quoted fragment travels with it — which is exactly what lets the card name
// WHICH step and field was refused instead of saying "something went wrong".
type EmissionFault struct {
	// Op names which operation faulted: "draft" or "revise" — the same
	// vocabulary PendingEscalation uses, so the granted round re-enters the
	// phase that owns it.
	Op string `json:"op"`
	// Refusal is the platform's own refusal sentence, verbatim ("step S-1
	// approach is 1294 characters (cap 1200)"), so the card can say what
	// actually happened rather than "something went wrong".
	Refusal string `json:"refusal"`
	// Rounds counts the planning rounds this task has spent on this one
	// emission and had REFUSED, the rounds the requester granted included. It
	// is the platform's own count of pipeline rounds, not the seam's internal
	// bounce count — intake cannot see that one and does not claim it.
	Rounds int    `json:"rounds"`
	TS     string `json:"ts"`
}

// Emission-fault operations (EmissionFault.Op).
const (
	EmissionOpDraft  = "draft"
	EmissionOpRevise = "revise"
)

// ApprovalRecord is the Stage-4 record: who/when/card version/choices
// (Spec S06.1 Stage 4).
type ApprovalRecord struct {
	Who         string          `json:"who"`
	When        string          `json:"when"`
	CardVersion int             `json:"card_version"`
	Choices     json.RawMessage `json:"choices,omitempty"`
	Ungated     bool            `json:"ungated,omitempty"` // zero-interaction band (S06.4)
}

// ComposeState records the one compose-a-worker request of a task (Spec
// S08.6: the compose verb answered on the no-fit card). One composition
// per task: the composed draft's adoption is the battery + approval's; a
// RECURRING need earns further compositions through new tasks' gap
// occurrences, never by re-running the shot on the same task.
type ComposeState struct {
	RequestedBy  string `json:"requested_by"`
	RequestedTS  string `json:"requested_ts"`
	GapSignature string `json:"gap_signature"`
}

// DeltaRecord tracks one post-approval delta through its card.
type DeltaRecord struct {
	ID             string      `json:"id"`
	Origin         string      `json:"origin"`
	Items          []DeltaItem `json:"items"`
	AskID          string      `json:"ask_id"`
	PresentedBytes int         `json:"presented_bytes"`
	IssuedTS       string      `json:"issued_ts"`
	DecidedTS      string      `json:"decided_ts,omitempty"`
	Decision       string      `json:"decision,omitempty"`
	NewSpecVersion int         `json:"new_spec_version,omitempty"`
	NewPlanVersion int         `json:"new_plan_version,omitempty"`
}

// State is the full durable pipeline state.
type State struct {
	Phase  Phase   `json:"phase"`
	TaskID string  `json:"task_id"`
	RunID  string  `json:"run_id"`
	Owner  string  `json:"owner"`
	Req    Request `json:"request"`

	// Stage 0 — the intake record (Spec S06.2).
	Family Family `json:"family"`
	// FamilySource names what resolved Family (P3-RW-11 R5) — additive, so a
	// state event written before this packet reads back "" and is left exactly
	// as it was. It is an event payload, so it needs no migration.
	FamilySource string         `json:"family_source,omitempty"`
	Tier         Tier           `json:"tier"`
	FloorTier    Tier           `json:"floor_tier,omitempty"` // deterministic floor ("" = none)
	FloorReasons []FloorReason  `json:"floor_reasons,omitempty"`
	Guess        Estimate       `json:"guess"`
	DataBearing  []TriggerHit   `json:"data_bearing,omitempty"`
	Supplied     []SuppliedFact `json:"supplied,omitempty"`
	Registry     *RegistrySlice `json:"registry,omitempty"`
	Band         bool           `json:"band"`                  // zero-interaction membership (S06.4)
	BandExited   bool           `json:"band_exited,omitempty"` // exit is one-way
	BandExitWhy  string         `json:"band_exit_why,omitempty"`
	RecordRef    *ArtifactRef   `json:"record_ref,omitempty"`
	// TriagePending marks a state whose classification has not run yet: Start
	// sets it, and the classify step at the top of the first advance clears it
	// in EVERY branch — a failed classify is a completed triage attempt, not a
	// retry loop.
	//
	// The polarity is the point. A default-false "classified" flag would read
	// true-for-nobody on every state event written before this packet, so the
	// next advance would re-classify tasks already mid-interview and could
	// overwrite a settled family or tier (Spec S06.4 monotonicity). The zero
	// value here is "nothing to do", so an old world proceeds exactly as it did
	// (the family_source/R10 precedent above).
	TriagePending bool `json:"triage_pending,omitempty"`

	// Stage 1 — interview + drafting (Spec S06.5–S06.6).
	TaxonomyID      string           `json:"taxonomy_id"`
	TaxonomyVersion string           `json:"taxonomy_version"`
	Resolutions     []SlotResolution `json:"resolutions,omitempty"`
	ForceProceeded  bool             `json:"force_proceeded,omitempty"`
	// ReinterviewRequested makes the next interview phase issue one full
	// top-weight card even over resolved slots (S06.9 Re-interview /
	// SPEC-DOUBT adjust-spec: back to Stage 1 with artifacts intact).
	ReinterviewRequested bool               `json:"reinterview_requested,omitempty"`
	Escalations          []EscalationAnswer `json:"escalations,omitempty"`
	PendingEscalation    string             `json:"pending_escalation,omitempty"` // "draft" | "revise" | "critique"
	NeedsDraft           bool               `json:"needs_draft"`
	PendingRevise        *ReviseReq         `json:"pending_revise,omitempty"`
	// EmissionFault parks a refused emission on its own card (P3-GF12 R3).
	// Additive and omitempty: a state event written before this packet reads
	// back nil and behaves exactly as it did.
	EmissionFault *EmissionFault `json:"emission_fault,omitempty"`
	// SettledMarkers is the durable answered-marker record (P3-GF12 R5): the
	// proof a point was already asked and answered, which is what lets the
	// platform settle a re-emitted marker from the record instead of asking a
	// person the same question twice.
	SettledMarkers []SettledMarker `json:"settled_markers,omitempty"`
	SpecVersion    int             `json:"spec_version"`
	PlanVersion    int             `json:"plan_version"`
	SpecRef        *ArtifactRef    `json:"spec_ref,omitempty"`
	PlanRef        *ArtifactRef    `json:"plan_ref,omitempty"`

	// Stage 2 — spine bookkeeping (Spec S06.7).
	Spine           *SpineResult `json:"spine,omitempty"`
	CoverageRounds  int          `json:"coverage_rounds"`
	ResearchBounced bool         `json:"research_bounced"`
	// ClarificationRounds counts the NEEDS-CLARIFICATION cards this intake has
	// issued (P3-GF12 R7). Coverage has its ⚙ auto-fix bound and research has
	// its one bounce; the marker loop had NOTHING, which is how a live intake
	// reached a fourth clarification round re-confirming settled facts.
	ClarificationRounds int      `json:"clarification_rounds"`
	AcceptedUncovered   []string `json:"accepted_uncovered,omitempty"` // requester-accepted gaps

	// Stage 3 — critique (Spec S06.8).
	CritiqueRounds int             `json:"critique_rounds"`
	CritiqueDone   bool            `json:"critique_done"`
	OpenFindings   []string        `json:"open_findings,omitempty"`
	Verdicts       []VerdictRecord `json:"verdicts,omitempty"`

	// Gate bookkeeping.
	OpenAskID    string   `json:"open_ask_id,omitempty"`
	OpenAskKind  CardKind `json:"open_ask_kind,omitempty"`
	CardIssuedTS string   `json:"card_issued_ts,omitempty"`
	CardVersion  int      `json:"card_version"`
	StaleFlag    bool     `json:"stale_flag,omitempty"`
	StaleReasons []string `json:"stale_reasons,omitempty"`
	// StoredFingerprint is captured at approval-card issue for the S02.6
	// staleness comparison (G1 Def.5).
	StoredFingerprint *run.Fingerprint `json:"stored_fingerprint,omitempty"`

	// Stage 4 — approval + claims (Spec S06.9, S02.8).
	Approval    *ApprovalRecord `json:"approval,omitempty"`
	ClaimIDs    []int64         `json:"claim_ids,omitempty"`
	ClaimStatus string          `json:"claim_status,omitempty"` // "active" | "waiting" | ""
	Deltas      []DeltaRecord   `json:"deltas,omitempty"`

	// Routing is the S08.8 selection surfaced on the approval card
	// (route.go), including any recorded override — the execute dispatch
	// consumes it (worker + version per run, the version→outcome join).
	Routing *RouteBlock `json:"routing,omitempty"`

	// Compose records the task's compose-a-worker request (Spec S08.6);
	// the stage layer launches the composition ceremony run from it.
	Compose *ComposeState `json:"compose,omitempty"`

	Clearance float64 `json:"clearance"`
}

// resolvedSet returns the resolved slot-id set.
func (s *State) resolvedSet() map[string]bool {
	m := make(map[string]bool, len(s.Resolutions))
	for _, r := range s.Resolutions {
		m[r.SlotID] = true
	}
	return m
}

// resolutionOf returns the slot's current resolution, or nil when nothing has
// settled it yet. It is what makes a slot ADDRESSABLE: the understood block a
// card serves is built from exactly these, so "the card shows it" and "this
// returns non-nil" are the same fact (P3-GF8 R2/R3).
func (s *State) resolutionOf(slotID string) *SlotResolution {
	for i := range s.Resolutions {
		if s.Resolutions[i].SlotID == slotID {
			return &s.Resolutions[i]
		}
	}
	return nil
}

// resolveSlot records a resolution (idempotent per slot: the last write
// wins; slots never un-resolve).
func (s *State) resolveSlot(r SlotResolution) {
	for i := range s.Resolutions {
		if s.Resolutions[i].SlotID == r.SlotID {
			s.Resolutions[i] = r
			return
		}
	}
	s.Resolutions = append(s.Resolutions, r)
}

// normalizeMarker is the marker-identity rule: whitespace collapsed, nothing
// else touched (P3-GF12 OQ-2).
//
// EXACT-after-normalization is the whole point. Deciding that two differently
// worded markers "mean the same thing" is a semantic judgement on model text,
// and a platform that made it silently would eventually swallow a genuinely new
// question — so it is not made here at all. Re-wordings are the prompt rule's
// job (the settled-facts contract) and the round bound's (conversion to a listed
// assumption); this catches only the class the evidence proves is real, where
// ask 6 came back byte-identical to the answered ask 5.
func normalizeMarker(s string) string { return strings.Join(strings.Fields(s), " ") }

// settleMarker records an answered marker (last answer wins per marker text —
// the requester correcting their own answer is not a second question).
func (s *State) settleMarker(marker, answer, ts string) {
	key := normalizeMarker(marker)
	for i := range s.SettledMarkers {
		if normalizeMarker(s.SettledMarkers[i].Marker) == key {
			s.SettledMarkers[i].Answer, s.SettledMarkers[i].TS = answer, ts
			return
		}
	}
	s.SettledMarkers = append(s.SettledMarkers, SettledMarker{Marker: marker, Answer: answer, TS: ts})
}

// settledMarker returns the record for a marker the requester already answered,
// or nil — the deterministic backstop the re-emitted-marker guard reads.
func (s *State) settledMarker(marker string) *SettledMarker {
	key := normalizeMarker(marker)
	for i := range s.SettledMarkers {
		if normalizeMarker(s.SettledMarkers[i].Marker) == key {
			return &s.SettledMarkers[i]
		}
	}
	return nil
}

// dismissed reports whether a P47 rule id has a requester-supplied fact.
func (s *State) dismissed(ruleID string) bool {
	for _, f := range s.Supplied {
		if f.RuleID == ruleID {
			return true
		}
	}
	return false
}

// openDataBearing returns hits not dismissed by requester-supplied facts.
func (s *State) openDataBearing() []TriggerHit {
	var out []TriggerHit
	for _, h := range s.DataBearing {
		if !s.dismissed(h.RuleID) {
			out = append(out, h)
		}
	}
	return out
}

// addHits merges hits add-only, one per rule id (the classifier may only
// ADD the flag, never remove it — Spec S06.2/S06.3).
func (s *State) addHits(hits []TriggerHit) {
	have := make(map[string]bool, len(s.DataBearing))
	for _, h := range s.DataBearing {
		have[h.RuleID] = true
	}
	for _, h := range hits {
		if !have[h.RuleID] {
			have[h.RuleID] = true
			s.DataBearing = append(s.DataBearing, h)
		}
	}
}

// addFloors merges floor reasons add-only and keeps the floor tier high.
func (s *State) addFloors(reasons []FloorReason) {
	if len(reasons) == 0 {
		return
	}
	s.FloorReasons = append(s.FloorReasons, reasons...)
	s.FloorTier = TierHigh
	s.Tier = maxTier(s.Tier, TierHigh)
}

// exitBand ejects the task from the zero-interaction band — one-way for
// the task (Spec S06.4).
func (s *State) exitBand(why string) {
	if s.Band {
		s.Band = false
		s.BandExited = true
		s.BandExitWhy = why
	}
}

// ---- Persistence ----

// appendStateTx appends the state event at the run's current generation
// (a control-plane act; CONVENTIONS §13 fencing split), attributing the
// row to the run's owner (15.6).
func (p *Pipeline) appendStateTx(ctx context.Context, tx *sql.Tx, st *State) error {
	var gen int64
	if err := tx.QueryRowContext(ctx,
		`SELECT generation FROM runs WHERE run_id = ?`, st.RunID).Scan(&gen); err != nil {
		return fmt.Errorf("intake: read run %q: %w", st.RunID, err)
	}
	payload, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("intake: marshal state: %w", err)
	}
	_, err = p.Log.AppendTx(ctx, tx, eventlog.Append{
		RunID:         st.RunID,
		Generation:    gen,
		UserID:        st.Owner,
		Type:          EventState,
		SchemaVersion: stateSchemaVersion,
		Payload:       payload,
	})
	return err
}

// appendState appends in its own write transaction.
func (p *Pipeline) appendState(ctx context.Context, st *State) error {
	return p.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		return p.appendStateTx(ctx, tx, st)
	})
}

// LoadState returns the task's current pipeline state (the latest
// intake.state event across the task's runs, by event_seq).
func (p *Pipeline) LoadState(ctx context.Context, taskID string) (*State, error) {
	var payload string
	err := p.DB.QueryRowContext(ctx, `
		SELECT e.payload FROM run_events e
		  JOIN runs r ON e.run_id = r.run_id
		 WHERE r.task_id = ? AND e.type = ?
		 ORDER BY e.event_seq DESC LIMIT 1`, taskID, EventState).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", ErrNoState, taskID)
	}
	if err != nil {
		return nil, fmt.Errorf("intake: read state: %w", err)
	}
	var st State
	if err := json.Unmarshal([]byte(payload), &st); err != nil {
		return nil, fmt.Errorf("intake: decode state: %w", err)
	}
	return &st, nil
}

// ---- Ask rows ----

// insertAskTx writes the durable ask row (Spec S02.2 asks family:
// authoritative from the moment observed; the snapshot is the full card).
func (p *Pipeline) insertAskTx(ctx context.Context, tx *sql.Tx, askID string, st *State, card *Card) error {
	snapshot, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("intake: marshal card: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts)
		 VALUES (?, ?, ?, ?, 'open', ?)`,
		askID, st.RunID, st.Owner, string(snapshot), p.nowRFC3339())
	if err != nil {
		return fmt.Errorf("intake: insert ask %q: %w", askID, err)
	}
	return nil
}

// updateAskSnapshotTx refreshes an open card in place (stale-flag update,
// card re-versioning). The row keeps its first observed_ts.
func (p *Pipeline) updateAskSnapshotTx(ctx context.Context, tx *sql.Tx, askID string, card *Card) error {
	snapshot, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("intake: marshal card: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE asks SET snapshot = ? WHERE ask_id = ? AND status = 'open'`,
		string(snapshot), askID)
	if err != nil {
		return fmt.Errorf("intake: update ask %q: %w", askID, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("%w: %q", ErrUnknownAsk, askID)
	}
	return nil
}

// closeAskTx records the answer (or closes the card with a status) on the
// ask row.
func (p *Pipeline) closeAskTx(ctx context.Context, tx *sql.Tx, askID, status string, answer json.RawMessage) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE asks SET status = ?, answered_ts = ?, answer = ? WHERE ask_id = ? AND status = 'open'`,
		status, p.nowRFC3339(), nullableJSON(answer), askID)
	if err != nil {
		return fmt.Errorf("intake: close ask %q: %w", askID, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("%w: %q", ErrUnknownAsk, askID)
	}
	return nil
}

// openCard reads one open ask row's card.
func (p *Pipeline) openCard(ctx context.Context, askID string) (*Card, string, error) {
	var snapshot, runID string
	err := p.DB.QueryRowContext(ctx,
		`SELECT snapshot, run_id FROM asks WHERE ask_id = ? AND status = 'open'`, askID).
		Scan(&snapshot, &runID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", fmt.Errorf("%w: %q", ErrUnknownAsk, askID)
	}
	if err != nil {
		return nil, "", fmt.Errorf("intake: read ask %q: %w", askID, err)
	}
	var card Card
	if err := json.Unmarshal([]byte(snapshot), &card); err != nil {
		return nil, "", fmt.Errorf("intake: decode card %q: %w", askID, err)
	}
	return &card, runID, nil
}

func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}

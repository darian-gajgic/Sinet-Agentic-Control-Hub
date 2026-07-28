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

// oversight.go — the verbs of the four oversight card kinds the B6-2A inbox
// listed as `answerable:false` (P3-B6-2B, R14–R17): suppress a watchdog flag
// (S14.4), resume a parked run (S14.4), dismiss a drift card (S14.6), and
// acknowledge a conformance card (S14.5).
//
// Every one of them routes machinery that ALREADY EXISTS and adds nothing to
// it. The transport's whole job is identity, scope and the audit obligation:
//
//   - suppress → internal/watchdog's own Suppress, with the FULL anomaly class
//     the projection served. Its retune-proposal mechanics fire inside the
//     package exactly as they did before there was an endpoint; this file adds
//     no count, no threshold and no proposal of its own.
//   - resume  → the ratified S02.3 parked→running edge through internal/stage,
//     which is the supersession the landed open-flag derivation ALREADY honors.
//     No projection changed to make this work.
//   - dismiss / acknowledge → a decision.recorded row, and the card derivations
//     read those rows back. Derive-from-log, no side store, no card table
//     (S14.1): a dismissal is a fact about a decision, and the decision log is
//     where facts about decisions live.
//
// ── The negative that matters most ──────────────────────────────────────────
//
// NO VERB HERE MAKES A RED THING GREEN. `acknowledge` moves a red conformance
// row out of flag-now attention and leaves it LISTED, red, until a real suite
// run records a green result (§32: a red clears only by a real green result).
// `last_result` is written by internal/conformance's RecordResult and by
// nothing else — this file never writes it, and a source assertion proves it.
// Likewise a drift dismissal closes a CARD and never deletes a finding: the
// `drift.finding` rows stay in run_events, queryable forever (S16.2 — a
// standing mute would be a watch deletion by another name).

// SuppressSurface is the S14.4 flag-suppression seam. *watchdog.Watchdog
// satisfies it directly; nil leaves the suppress verb answering 503.
//
// `rule` is the flag's FULL anomaly class — the suffixed form for a run-less
// flag (watchdog.spend:<owner>, watchdog.organ_absence:<organ>) — because that
// is the string the derive-from-log supersession matches on. Passing the bare
// rule is what left suffixed flags permanently un-clearable before the §34 D5
// fix, and this transport passes through exactly what the card carried.
type SuppressSurface interface {
	Suppress(ctx context.Context, actor, runID, rule string) error
}

// ResumeSurface is S14.4's "resume — I was wrong": the ratified S02.3
// parked→running edge, taken by a person. *stage.Surface satisfies it; nil
// leaves the resume verb answering 503.
type ResumeSurface interface {
	ResumeRun(ctx context.Context, actor, runID string) (json.RawMessage, error)
}

// The oversight card types this file audits under decision.recorded (S14.2
// family 5). They match the inbox kinds the cards are listed under, so the
// decision log and the inbox speak one vocabulary.
const (
	cardTypeDrift       = approvalKindDrift
	cardTypeConformance = approvalKindConformance
)

// The two oversight decisions (OQ6, OQ7).
const (
	decisionDismiss     = "dismiss"
	decisionAcknowledge = "acknowledge"
)

// ── POST /api/watchdog/flags/suppress (R14; S14.4) ──────────────────────────

type suppressBody struct {
	// RunID is empty for a run-less (platform-scope) flag.
	RunID string `json:"run_id,omitempty"`
	// AnomalyClass is the card's own `anomaly_class`, verbatim.
	AnomalyClass string `json:"anomaly_class"`
	Reason       string `json:"reason,omitempty"`
}

// FlagSuppressed is the outcome. It echoes only the identifiers the caller
// supplied — no card content travels back, so this response carries nothing
// lifted from a run_events payload and the redaction edge stays where it is.
type FlagSuppressed struct {
	RunID        string `json:"run_id,omitempty"`
	AnomalyClass string `json:"anomaly_class"`
	Suppressed   bool   `json:"suppressed"`
	Detail       string `json:"detail"`
}

func (s *Server) handleFlagSuppress(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	if s.watchdog == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S14.4 watchdog is not wired in this process"})
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body suppressBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	class := strings.TrimSpace(body.AnomalyClass)
	if class == "" {
		s.writeSurface(w, nil, badRequest(`missing "anomaly_class": a suppression names the flag's own anomaly class, suffix included (S14.4)`))
		return
	}
	scope := s.readScope(r)
	// Scope IS visibility, and that is not a shortcut — it is the landed rule
	// stated exactly ("you can only suppress what you can see"). The open-flag
	// derivation already answers "which flags is this caller entitled to?", so
	// the verb asks it rather than re-deriving an owner rule that could drift
	// from the projection's. A flag outside the caller's scope is 404: an
	// unknown flag and another owner's flag must look the same from outside.
	flags, err := s.proj.watchdogFlags(r.Context(), scope)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	visible := false
	for _, f := range flags {
		if f.RunID == body.RunID && f.AnomalyClass == class {
			visible = true
			break
		}
	}
	if !visible {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusNotFound, Code: "not_found",
			Msg: "no open watchdog flag with that anomaly class on that run is visible to you"})
		return
	}
	id, _ := IdentityFrom(r.Context())
	if err := s.watchdog.Suppress(r.Context(), id.UserID, body.RunID, class); err != nil {
		s.writeSurface(w, nil, fmt.Errorf("suppress watchdog flag: %w", err))
		return
	}
	// NO decision.recorded here (OQ8): Suppress mints the canonical
	// `watchdog.suppressed` row itself, with the actor in the payload. A second
	// row would make one suppression look like two.
	s.writeReadJSON(w, FlagSuppressed{RunID: body.RunID, AnomalyClass: class, Suppressed: true,
		Detail: "suppressed: the flag closes as a tuning signal, and every ⚙ watchdog.suppress_retune_count-th suppression of the rule proposes a threshold raise — a proposal the operator applies, never an auto-move (S14.4)"})
}

// ── POST /api/runs/{run}/resume (R15; S14.4) ────────────────────────────────

func (s *Server) handleRunResume(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	if s.resume == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S02.3 resume edge is not wired in this process"})
		return
	}
	runID := r.PathValue("run")
	var owner string
	err := s.proj.db.QueryRowContext(r.Context(), `SELECT user_id FROM runs WHERE run_id = ?`, runID).Scan(&owner)
	found := !errors.Is(err, sql.ErrNoRows)
	if err != nil && found {
		s.writeSurface(w, nil, fmt.Errorf("read run owner: %w", err))
		return
	}
	if code, cerr := authorizeOwner(s.readScope(r), owner, found, "run"); cerr != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: code, Code: httpCode(code), Msg: cerr.Error()})
		return
	}
	id, _ := IdentityFrom(r.Context())
	payload, err := s.resume.ResumeRun(r.Context(), id.UserID, runID)
	// NO decision.recorded (OQ8): the resume rides its own `run.state_changed`
	// row, which is also exactly the supersession the open-flag derivation
	// reads. One act, one canonical row.
	s.writeSurface(w, payload, err)
}

// ── POST /api/approvals/{id}/dismiss — the drift card (R16; S14.6; OQ7) ─────

// DriftDismissed is the outcome. Identifiers only: no card content, so nothing
// payload-derived travels back through this verb.
type DriftDismissed struct {
	CardID string `json:"card_id"`
	// Fingerprint + WindowStartSeq are the incident's identity. The dismissal is
	// scoped to THIS incident window, so a genuinely new finding in a new window
	// opens a new card rather than landing under a standing mute.
	Fingerprint    string `json:"fingerprint"`
	WindowStartSeq int64  `json:"window_start_seq"`
	Dismissed      bool   `json:"dismissed"`
	Detail         string `json:"detail"`
}

type oversightBody struct {
	Reason string `json:"reason,omitempty"`
}

func (s *Server) handleDriftDismiss(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	kind, fingerprint, ok := strings.Cut(r.PathValue("id"), ":")
	if !ok || kind != approvalKindDrift || fingerprint == "" {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			"dismiss takes a drift card id (%s:<fingerprint>); other kinds have their own verbs", approvalKindDrift)))
		return
	}
	scope := s.readScope(r)
	// Drift cards are platform-scope and operator-only, exactly as the landed
	// projection scopes them (S01.9). A member's own derivation returns none, so
	// asking it is both the authority check and the existence check.
	cards, err := s.proj.driftCards(r.Context(), scope)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	var card InboxDriftCard
	for _, c := range cards {
		if c.Fingerprint == fingerprint {
			card = c
			break
		}
	}
	if card.Fingerprint == "" {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusNotFound, Code: "not_found",
			Msg: "no open drift card with that fingerprint is visible to you"})
		return
	}
	body := s.oversightReason(w, r)
	if body == nil {
		return
	}
	if err := s.recordDecision(r.Context(), decisionPayload{
		Actor: scope.UserID, ActorIsOperator: scope.Operator,
		CardID: approvalKindDrift + ":" + fingerprint, CardType: cardTypeDrift,
		Decision: decisionDismiss,
		// Subject is the INCIDENT, not the fingerprint: fingerprint + the seq of
		// the finding that opened this window. That is what keeps the dismissal
		// scoped to the storm the operator actually read (OQ7) — a fingerprint-
		// forever mute would be a watch deletion by another name (S16.2).
		Subject: driftIncidentSubject(fingerprint, card.Seq),
		Reason:  body.Reason,
		Ref:     "S14.6 drift card; the finding rows stay queryable",
	}, platformOwner, card.FirstTS); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	s.writeReadJSON(w, DriftDismissed{
		CardID: approvalKindDrift + ":" + fingerprint, Fingerprint: fingerprint,
		WindowStartSeq: card.Seq, Dismissed: true,
		Detail: "dismissed: this incident window's card leaves the inbox. The drift.finding rows are untouched and stay queryable, and a finding in a NEW window opens a new card (S14.6)",
	})
}

// driftIncidentSubject is the incident identity written into the audit row and
// read back by the derivation. ONE function so the write and the read cannot
// drift apart.
func driftIncidentSubject(fingerprint string, windowStartSeq int64) string {
	return fmt.Sprintf("%s#%d", fingerprint, windowStartSeq)
}

// ── POST /api/approvals/{id}/acknowledge — conformance (R17; S14.5; OQ6) ────

// ConformanceAcknowledged is the outcome.
type ConformanceAcknowledged struct {
	CardID string `json:"card_id"`
	RowID  string `json:"row_id"`
	// LastRunTS is the result the acknowledgement was given FOR. A later suite
	// run produces a different one, and the acknowledgement does not carry over
	// to it.
	LastRunTS    time.Time `json:"last_run_ts"`
	Acknowledged bool      `json:"acknowledged"`
	// StillRed is always true, and it is in the shape on purpose: an
	// acknowledgement is not a pass.
	StillRed bool   `json:"still_red"`
	Detail   string `json:"detail"`
}

func (s *Server) handleConformanceAcknowledge(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	kind, rowID, ok := strings.Cut(r.PathValue("id"), ":")
	if !ok || kind != approvalKindConformance || rowID == "" {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			"acknowledge takes a conformance card id (%s:<row id>); other kinds have their own verbs", approvalKindConformance)))
		return
	}
	scope := s.readScope(r)
	// Conformance rows are platform-scope and operator-only, as landed.
	cards, err := s.proj.conformanceCards(r.Context(), scope)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	var card InboxConformanceCard
	for _, c := range cards {
		if c.RowID == rowID {
			card = c
			break
		}
	}
	if card.RowID == "" {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusNotFound, Code: "not_found",
			Msg: "no red conformance card with that row id is visible to you"})
		return
	}
	body := s.oversightReason(w, r)
	if body == nil {
		return
	}
	if err := s.recordDecision(r.Context(), decisionPayload{
		Actor: scope.UserID, ActorIsOperator: scope.Operator,
		CardID: approvalKindConformance + ":" + rowID, CardType: cardTypeConformance,
		Decision: decisionAcknowledge,
		// Subject is the row AND the result it was acknowledged for. The
		// cycle-scoping is the same principle the co-approval derivation uses
		// (part A, drain D3): a signature counts for the thing it was given for.
		// A LATER suite run — even another red — is a new failure and a new
		// obligation, and an acknowledgement from before it does not silence it.
		Subject: conformanceResultSubject(rowID, card.LastRunTS),
		Reason:  body.Reason,
		Ref:     "S14.5 conformance card; last_result moves only through a real suite run",
	}, platformOwner, card.LastRunTS); err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	s.writeReadJSON(w, ConformanceAcknowledged{
		CardID: approvalKindConformance + ":" + rowID, RowID: rowID, LastRunTS: card.LastRunTS,
		Acknowledged: true, StillRed: true,
		Detail: "acknowledged: this red stops counting as flag-now attention and STAYS LISTED red. Only a real green suite result clears it — no verb writes last_result (S14.5; §32)",
	})
}

// conformanceResultSubject is the acknowledged-result identity, written into the
// audit row and read back by the derivation. ONE function, both directions.
func conformanceResultSubject(rowID string, lastRun time.Time) string {
	return rowID + "#" + lastRun.UTC().Format(time.RFC3339Nano)
}

// oversightReason reads the optional reason body shared by the two card verbs,
// or writes the error and returns nil.
func (s *Server) oversightReason(w http.ResponseWriter, r *http.Request) *oversightBody {
	raw, ok := s.readBody(w, r)
	if !ok {
		return nil
	}
	var body oversightBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return nil
	}
	return &body
}

// ── reading the dismissals and acknowledgements back (derive-from-log) ──────

// decisionSubjects returns the set of `subject` values recorded for one card
// type and decision. It is the read half of both card verbs, and it reads the
// decision LOG rather than any side store: an oversight decision is a fact about
// a human decision, and S14.1's derive-from-log discipline says that is where it
// belongs. No card table, no dismissal column, nothing to fall out of sync.
//
// The scan is bounded by card_type in SQL rather than filtered in Go, so a
// platform with a long decision history does not materialize all of it to answer
// "which drift cards are dismissed?". Callers add their own time bound where the
// card kind has a horizon.
func (p *projector) decisionSubjects(ctx context.Context, cardType, decision, sinceTS string) (map[string]bool, error) {
	q := `SELECT payload FROM run_events
	       WHERE type = ? AND json_extract(payload, '$.card_type') = ?
	         AND json_extract(payload, '$.decision') = ?`
	args := []any{EventDecisionRecorded, cardType, decision}
	if sinceTS != "" {
		q += ` AND ts >= ?`
		args = append(args, sinceTS)
	}
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("projection: %s %s decisions: %w", cardType, decision, err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("projection: scan %s decision: %w", cardType, err)
		}
		var d decisionPayload
		if err := json.Unmarshal([]byte(payload), &d); err != nil {
			continue // a non-conformant payload never breaks the inbox (S14.2 rule 3)
		}
		if d.Subject != "" {
			out[d.Subject] = true
		}
	}
	return out, rows.Err()
}

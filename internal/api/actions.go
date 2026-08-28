package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// actions.go is the S15.2 task/run/deliverable action verbs: cancel (4.5) and
// follow-up spawn (S13.9).
//
// Both are control-plane-internal state acts. Neither performs an outward
// effect — nothing non-idempotent leaves the platform except through the S02.7
// journal behind an approval (D7), and this file contains no provider call of
// any kind.
//
// ── NO AUTO-KILL ──────────────────────────────────────────────────────────
//
// The cancel routes are the ONLY entry points to the cancel machinery. The
// mapping itself lives behind the CancelSurface seam (internal/stage), which
// takes the acting person's id: there is no automated caller anywhere, and the
// watchdog's import wall and the benchmark package's no-kill wall are untouched
// (S14.4 / G1 D1.3; CONVENTIONS §31, §35).

// CancelSurface is the S02.3 cancel choreography seam (feature 4.5). The
// transport resolves identity and owner scope and bounds the reason; the
// surface owns the ratified mapping — the live-session ladder, the FSM edges,
// the queue-row settle and the kanban column (CONVENTIONS §14 reading 9). The
// stage skeleton implements it; nil leaves the cancel routes answering 503
// (surface not wired).
type CancelSurface interface {
	// CancelRun cancels one run as actor, recording reason as the person's own
	// words for why (empty when they gave none).
	CancelRun(ctx context.Context, actor, runID, reason string) (json.RawMessage, error)
	// CancelTask cancels every non-terminal run of a task as actor, recording
	// the same reason on each ending.
	CancelTask(ctx context.Context, actor, taskID, reason string) (json.RawMessage, error)
}

// cancelReasonMaxRunes bounds the reason a cancel verb accepts.
//
// A structural constant with its reason, not a ⚙ key (the §37 OQ6 pattern; S18
// ratifies nothing here): the affordance is a ONE-LINE motive, the value rides
// every transition a task cancel mints, and anything paragraph-shaped belongs
// in a card's note, which is a channel that already exists. Interim under the
// standing settings-tab directive.
//
// The bound counts RUNES, because it exists to describe what a person typed and
// a byte count would refuse a shorter sentence for being written in a language
// with wider characters. The landed card `note` gains no bound: bounding a
// landed contract is a behavior change this packet was not given.
const cancelReasonMaxRunes = 280

// cancelBody is the optional POST body both cancel verbs accept. Absent, empty,
// `{}` and an empty reason all mean "no reason" — the landed clients post `{}`
// or nothing, so the field is strictly additive.
type cancelBody struct {
	Reason string `json:"reason,omitempty"`
}

// cancelReason reads and bounds the optional reason, answering the caller
// itself when it refuses.
//
// Over the bound the answer is a refusal and NOTHING is cancelled: content is
// never silently altered (CONVENTIONS §60), and a truncated motive is a
// sentence the person did not write attributed to them forever. The words
// themselves are taken verbatim.
func (s *Server) cancelReason(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw, ok := s.readBody(w, r)
	if !ok {
		return "", false
	}
	var body cancelBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return "", false
	}
	if utf8.RuneCountInString(body.Reason) > cancelReasonMaxRunes {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "reason_too_long",
			Msg: fmt.Sprintf(
				"that is a bit long for one line — please shorten it to %d characters or fewer. Nothing has been cancelled yet.",
				cancelReasonMaxRunes)})
		return "", false
	}
	return body.Reason, true
}

func (s *Server) cancelReady(w http.ResponseWriter) bool {
	if s.cancel == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the cancel choreography is not wired in this process"})
		return false
	}
	return true
}

// POST /api/runs/{run}/cancel — the S15.2 runs-row "cancel (4.5)".
func (s *Server) handleRunCancel(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) || !s.cancelReady(w) {
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
	reason, ok := s.cancelReason(w, r)
	if !ok {
		return
	}
	id, _ := IdentityFrom(r.Context())
	payload, err := s.cancel.CancelRun(r.Context(), id.UserID, runID, reason)
	s.writeSurface(w, payload, err)
}

// POST /api/tasks/{task}/cancel — the S15.2 tasks-row "cancel (4.5)": every
// non-terminal run of the task under the same mapping.
func (s *Server) handleTaskCancel(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) || !s.cancelReady(w) {
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
	if code, cerr := authorizeOwner(s.readScope(r), owner, found, "task"); cerr != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: code, Code: httpCode(code), Msg: cerr.Error()})
		return
	}
	reason, ok := s.cancelReason(w, r)
	if !ok {
		return
	}
	id, _ := IdentityFrom(r.Context())
	payload, err := s.cancel.CancelTask(r.Context(), id.UserID, taskID, reason)
	s.writeSurface(w, payload, err)
}

// ── follow-up spawn (S13.9) ─────────────────────────────────────────────────

// landedPresets is the S13.9 framing set, verbatim — presets over the ONE
// successor link, never schema variants. "" is admitted as the plain follow-up
// the package itself defaults to.
var landedPresets = map[string]bool{
	"":                               true,
	string(intake.PresetPlain):       true,
	string(intake.PresetRevision):    true,
	string(intake.PresetExtension):   true,
	string(intake.PresetCounterpart): true,
}

// followUpBody is the POST /api/deliverables/{deliverable}/follow-up payload:
// the source revision plus the preset framing and the follow-up's own ask.
type followUpBody struct {
	Revision  int    `json:"revision"`
	Preset    string `json:"preset"`
	Detail    string `json:"detail,omitempty"`
	Objective string `json:"objective,omitempty"`
	Title     string `json:"title,omitempty"`
}

// FollowUpSpawned is the new task, with the pipeline's own task view nested
// when a pipeline surface is wired in this process.
type FollowUpSpawned struct {
	TaskID        string          `json:"task_id"`
	Owner         string          `json:"owner"`
	Title         string          `json:"title"`
	DeliverableID string          `json:"deliverable_id"`
	RevisionN     int             `json:"revision_n"`
	Preset        string          `json:"preset"`
	Task          json.RawMessage `json:"task,omitempty"`
}

// POST /api/deliverables/{deliverable}/follow-up — routes the landed S13.9
// spawn (successor task + task_successor_of link in ONE action, then normal
// intake through the root-wired Start seam). The presets are the landed
// framings over the SAME link, never schema variants — this route adds none.
//
// It touches neither accept nor preview: those are the B6-3 content family's.
func (s *Server) handleFollowUpSpawn(w http.ResponseWriter, r *http.Request) {
	if !s.projReady(w) {
		return
	}
	if s.followUp == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S13.9 follow-up surface is not wired in this process"})
		return
	}
	deliverableID := r.PathValue("deliverable")
	var owner, projectID string
	var current int
	// project_id through the task_project join, as everywhere a deliverable's
	// project is served (P3-RW-18 D2-R2): a follow-up spawned off a row minted
	// before D2-R1 must land in the same project as its parent, and that row
	// carries ''.
	err := s.proj.db.QueryRowContext(r.Context(),
		`SELECT d.user_id, COALESCE(NULLIF(d.project_id, ''), tp.project_id, ''), d.current_revision
		   FROM deliverables d
		   LEFT JOIN task_project tp ON tp.task_id = d.task_id
		  WHERE d.deliverable_id = ?`, deliverableID).
		Scan(&owner, &projectID, &current)
	found := !errors.Is(err, sql.ErrNoRows)
	if err != nil && found {
		s.writeSurface(w, nil, fmt.Errorf("read deliverable: %w", err))
		return
	}
	if code, cerr := authorizeOwner(s.readScope(r), owner, found, "deliverable"); cerr != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: code, Code: httpCode(code), Msg: cerr.Error()})
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body followUpBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	if body.Revision == 0 {
		// The current revision is the honest default for "follow up on this
		// deliverable"; naming a revision explicitly still wins.
		body.Revision = current
	}
	if body.Revision < 1 {
		// S13.9: a follow-up carries ONE link, to a numbered revision.
		s.writeSurface(w, nil, badRequest("bad version: a follow-up is linked to a numbered version of the work it follows, and versions start at 1"))
		return
	}
	// Client input is validated HERE, at the boundary, so the spawn call below
	// can only fail for internal reasons (drain D7). Mapping every Spawn error
	// to 400 told a caller their input was bad when the database was down; the
	// two failure classes are genuinely different answers and now read as such.
	if !landedPresets[body.Preset] {
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			"unknown preset %q: the S13.9 framings are the landed set (revision / extension / counterpart, or none)", body.Preset)))
		return
	}
	var exists int
	switch err := s.proj.db.QueryRowContext(r.Context(),
		`SELECT 1 FROM deliverable_revisions WHERE deliverable_id = ? AND n = ?`,
		deliverableID, body.Revision).Scan(&exists); {
	case errors.Is(err, sql.ErrNoRows):
		s.writeSurface(w, nil, badRequest(fmt.Sprintf(
			// S13.9: the one successor link must resolve.
			"there is no version %d of %s: a follow-up can only be linked to a version that exists", body.Revision, deliverableID)))
		return
	case err != nil:
		s.writeSurface(w, nil, fmt.Errorf("read deliverable revision: %w", err))
		return
	}
	// The successor is the AUTHENTICATED requester's own task (15.6): whoever
	// asks for the follow-up owns the work it creates.
	id, _ := IdentityFrom(r.Context())
	req, err := s.followUp.Spawn(r.Context(), intake.FollowUpInput{
		Owner:         id.UserID,
		DeliverableID: deliverableID,
		Revision:      body.Revision,
		ProjectID:     projectID,
		Preset:        intake.Preset(strings.TrimSpace(body.Preset)),
		PresetDetail:  body.Detail,
		Objective:     body.Objective,
		Title:         body.Title,
	})
	if err != nil {
		// Every caller-input failure was refused above, so anything reaching
		// here is the platform's own (drain D7): it answers 500, not 400.
		s.writeSurface(w, nil, fmt.Errorf("spawn follow-up: %w", err))
		return
	}
	out := FollowUpSpawned{
		TaskID: req.TaskID, Owner: req.UserID, Title: req.Title,
		DeliverableID: deliverableID, RevisionN: body.Revision, Preset: body.Preset,
	}
	if s.intake != nil {
		if view, verr := s.intake.Task(r.Context(), req.TaskID); verr == nil {
			out.Task = view
		}
	}
	s.writeReadJSON(w, out)
}

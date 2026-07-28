package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
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
// transport resolves identity and owner scope; the surface owns the ratified
// mapping — the live-session ladder, the FSM edges, the queue-row settle and
// the kanban column (CONVENTIONS §14 reading 9). The stage skeleton implements
// it; nil leaves the cancel routes answering 503 (surface not wired).
type CancelSurface interface {
	// CancelRun cancels one run as actor.
	CancelRun(ctx context.Context, actor, runID string) (json.RawMessage, error)
	// CancelTask cancels every non-terminal run of a task as actor.
	CancelTask(ctx context.Context, actor, taskID string) (json.RawMessage, error)
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
	id, _ := IdentityFrom(r.Context())
	payload, err := s.cancel.CancelRun(r.Context(), id.UserID, runID)
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
	id, _ := IdentityFrom(r.Context())
	payload, err := s.cancel.CancelTask(r.Context(), id.UserID, taskID)
	s.writeSurface(w, payload, err)
}

// ── follow-up spawn (S13.9) ─────────────────────────────────────────────────

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
	err := s.proj.db.QueryRowContext(r.Context(),
		`SELECT user_id, project_id, current_revision FROM deliverables WHERE deliverable_id = ?`, deliverableID).
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
		s.writeSurface(w, nil, badRequest("bad revision: a follow-up links to a numbered revision of the source deliverable (S13.9)"))
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
		// A bad preset or an unreal (deliverable, revision) is the caller's
		// input, not an internal failure: the composite FK is what makes the
		// source referentially real (S13.9).
		s.writeSurface(w, nil, badRequest(err.Error()))
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

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// The B2-4 walking-skeleton API surface: submit a request, watch a task,
// answer intake cards, read receipts — the minimal ingress the thin
// end-to-end path needs (Spec S19.5 B2). PROVISIONAL pending the S15.2
// endpoint families (B6): paths may move under the additive-first
// evolution rule; the transport stays a thin JSON pass-through — this
// package owns identity, step-up and status mapping, the pipeline owns
// everything else (Spec S01.3 API seam).

// IntakeSurface is the pipeline surface behind the intake/task routes.
// Implementations return response payloads as raw JSON; errors that should
// map to specific HTTP statuses are *SurfaceError values. The
// stage-package skeleton implements it; nil leaves the routes answering
// 503 (surface not wired).
type IntakeSurface interface {
	// Submit files a new request for the authenticated requester and
	// admits its intake run (Spec S06.1 Stage 0; S16.6 Enqueue ingress).
	Submit(ctx context.Context, userID string, body json.RawMessage) (json.RawMessage, error)
	// Answer applies a card answer as the acting user (D10: the requester
	// answers their own cards). pinVerified reports a same-request PIN
	// re-verification (the S01.9 step-up the High-tier approvals family
	// requires).
	Answer(ctx context.Context, userID, askID string, answer json.RawMessage, pinVerified bool) (json.RawMessage, error)
	// Task returns the task view (state, open card, runs, receipts).
	Task(ctx context.Context, taskID string) (json.RawMessage, error)
	// Artifacts returns the task's CURRENT SPEC/PLAN pair as stored JSON
	// (intake.Pair) for the S15.2 task-detail read. The pipeline owns the
	// artifact store and its sha256 + re-render integrity checks, so the
	// transport reads through it rather than around it. A task with no
	// drafted pair yet returns an error the detail renders as an honest
	// absence — never an error that hides the task.
	Artifacts(ctx context.Context, taskID string) (json.RawMessage, error)
	// Receipt returns a run's materialized receipt (Spec S10.10).
	Receipt(ctx context.Context, runID string) (json.RawMessage, error)
	// Advance re-drives a task's intake pipeline in place (dev nudge; the
	// pipeline itself decides whether anything can move).
	Advance(ctx context.Context, userID, taskID string) (json.RawMessage, error)
}

// SurfaceError maps a pipeline outcome to an HTTP status + machine code.
// The transport returns it as {"error": code, "detail": msg}.
type SurfaceError struct {
	Status int
	Code   string
	Msg    string
}

func (e *SurfaceError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Msg) }

// ErrPINRequired is the step-up demand (Spec S01.9 verify-at-act: High-tier
// approval answers re-prompt the actor's own PIN in the same request).
var ErrPINRequired = &SurfaceError{Status: http.StatusUnauthorized, Code: "pin_required",
	Msg: "this answer needs your PIN typed in as part of the same request"}

// maxIntakeBody bounds intake request/answer bodies (transport hygiene;
// not a ⚙ — S18 ratifies no such key, the sseBatchSize precedent).
const maxIntakeBody = 1 << 20

func (s *Server) readBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, bool) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxIntakeBody+1))
	if err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return nil, false
	}
	if len(raw) > maxIntakeBody {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusRequestEntityTooLarge, Code: "too_large", Msg: "body exceeds 1 MiB"})
		return nil, false
	}
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	return raw, true
}

func (s *Server) writeSurfaceErr(w http.ResponseWriter, e *SurfaceError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(struct {
		Error  string `json:"error"`
		Detail string `json:"detail,omitempty"`
	}{e.Code, e.Msg})
}

func (s *Server) writeSurface(w http.ResponseWriter, payload json.RawMessage, err error) {
	if err != nil {
		var se *SurfaceError
		if errors.As(err, &se) {
			s.writeSurfaceErr(w, se)
			return
		}
		s.logger.Error("intake surface", "err", err)
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusInternalServerError, Code: "internal", Msg: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(payload)
}

func (s *Server) surfaceReady(w http.ResponseWriter) bool {
	if s.intake == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "intake surface not wired in this process"})
		return false
	}
	return true
}

// POST /api/intake/requests
func (s *Server) handleIntakeSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.surfaceReady(w) {
		return
	}
	id, _ := IdentityFrom(r.Context())
	body, ok := s.readBody(w, r)
	if !ok {
		return
	}
	payload, err := s.intake.Submit(r.Context(), id.UserID, body)
	s.writeSurface(w, payload, err)
}

// lanePinOptionRow is one served row of the lane-pin set (S00.9 A13). It is a
// wire struct rather than json tags on intake.LanePinOption because that type
// is the SEAM the verdict crosses on, not a payload — the snake_case spelling
// belongs to the transport, beside `pinned_lane` on the submit verb.
//
// NotPinnable is the selection layer's own sentence, carried verbatim: the same
// words refuseLanePin quotes when it refuses a submission naming this lane.
// `omitempty` keeps it off a pinnable row entirely, so a reader never has to
// decide what an empty reason means.
type lanePinOptionRow struct {
	Lane        string `json:"lane"`
	Pinnable    bool   `json:"pinnable"`
	NotPinnable string `json:"not_pinnable,omitempty"`
}

// pinnableLanesBody is an OBJECT and never a bare array: evolution on this
// surface is additive-first (S15.2), and a top-level array can never grow a
// sibling key.
type pinnableLanesBody struct {
	Lanes []lanePinOptionRow `json:"lanes"`
}

// GET /api/intake/pinnable-lanes — the lanes a task-creation pin may name,
// each carrying the platform's own verdict (S00.9 A13, P3-LN-10a).
//
// The set is the one the intake pipeline was composed with, served in its
// COMPOSITION order: first row is the platform's configured flat-rate lane,
// commissioned lanes follow, the local engine lane is last. Nothing is sorted,
// filtered or re-derived here — that order is meaningful and owned by the
// composition, and the verdicts are the selection layer's (§65/§66 D1: the
// transport re-derives no more than the boundary does).
//
// The set is PROCESS-WIDE, deliberately: coverage is the union across the
// people who placed a credential (worker.PinnableLanes states the same
// over-approximation), selection re-checks, and the boundary still refuses.
func (s *Server) handlePinnableLanes(w http.ResponseWriter, r *http.Request) {
	if !s.surfaceReady(w) {
		return
	}
	// Initialized, never nil: an empty set serves `[]`, which says "no lane is
	// pinnable here" where `null` would read as a broken response.
	rows := make([]lanePinOptionRow, 0, len(s.pinnableLanes))
	for _, opt := range s.pinnableLanes {
		rows = append(rows, lanePinOptionRow{Lane: opt.Lane, Pinnable: opt.Pinnable, NotPinnable: opt.NotPinnable})
	}
	payload, err := json.Marshal(pinnableLanesBody{Lanes: rows})
	s.writeSurface(w, payload, err)
}

// POST /api/tasks/{task}/advance
func (s *Server) handleTaskAdvance(w http.ResponseWriter, r *http.Request) {
	if !s.surfaceReady(w) {
		return
	}
	id, _ := IdentityFrom(r.Context())
	payload, err := s.intake.Advance(r.Context(), id.UserID, r.PathValue("task"))
	s.writeSurface(w, payload, err)
}

// answerBody is the card-answer envelope: the answer payload per the card's
// contract, plus the optional same-request PIN (S01.9 step-up).
type answerBody struct {
	PIN    string          `json:"pin,omitempty"`
	Answer json.RawMessage `json:"answer"`
}

// POST /api/asks/{ask}/answer
func (s *Server) handleAskAnswer(w http.ResponseWriter, r *http.Request) {
	if !s.surfaceReady(w) {
		return
	}
	id, _ := IdentityFrom(r.Context())
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body answerBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	if len(body.Answer) == 0 {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: `missing "answer" object`})
		return
	}
	// Same-request PIN re-verification (Spec S01.9 verify-at-act; the
	// B2+ approvals family routes High-tier answers through VerifyPIN).
	// The dev fallback identity has no PIN and can never step up.
	pinVerified := false
	if body.PIN != "" {
		if id.Dev {
			s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusForbidden, Code: "dev_identity",
				// S01.9: the dev fallback has no PIN and cannot step up.
				Msg: "the developer fallback cannot confirm with a PIN; sign in as yourself"})
			return
		}
		if err := s.sessions.VerifyPIN(r.Context(), id.UserID, body.PIN, "confirming a plan approval"); err != nil {
			s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusUnauthorized, Code: "pin_rejected", Msg: "PIN verification failed"})
			return
		}
		pinVerified = true
	}
	payload, err := s.intake.Answer(r.Context(), id.UserID, r.PathValue("ask"), body.Answer, pinVerified)
	s.writeSurface(w, payload, err)
}

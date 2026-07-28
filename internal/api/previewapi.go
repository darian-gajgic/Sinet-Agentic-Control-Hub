package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/preview"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
)

// previewapi.go — the S13.8 preview verbs (P3-B6-3B): launch, before-vs-after,
// the owner-scoped session list, and stop.
//
// LOW TIER, AND THAT IS THE WHOLE GATE. Launching a preview is a reversible
// action that touches nothing outside the host — a read-only revision checkout,
// a throwaway clone that absorbs writes, a loopback port (Spec S13.8: "previews
// are read-only with respect to the world"). So there is NO effect journal, NO
// proposal, NO approval and NO PIN step-up on any route in this file. Adding
// one would move a 4.2 boundary the spec deliberately drew.
//
// EVERY TYPE GETS A DEFINED STATE, AND NONE OF THEM IS AN ERROR. no-preview,
// requires-container-tier, unavailable and at-capacity are 200-class ANSWERS
// carrying their reason, exactly as the manager produces them — a broken iframe
// or a 500 where the honest answer is "this type has no preview" is the failure
// S13.8 names. The only 4xx here are a caller's own mistakes: an unknown
// deliverable, a deliverable with no minted revision, another owner's session.
//
// THE TRANSPORT OWNER-SCOPES THE LIST. preview.Manager.List() returns every
// live session because the manager is not an authorization layer; the scope
// belongs here, where the session identity lives (Session.User).

// previewLaunchBody names the revision to preview; 0 (or absent) is the
// deliverable's current revision.
type previewLaunchBody struct {
	Revision int `json:"revision"`
}

// PreviewSessions is the owner-scoped list of live preview sessions.
type PreviewSessions struct {
	Sessions []*preview.Session `json:"sessions"`
	Cursor   int64              `json:"cursor"`
}

func (s *Server) previewReady(w http.ResponseWriter) bool {
	if s.preview == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S13.8 preview surface is not wired in this process"})
		return false
	}
	return true
}

// previewRevision resolves the launch body's revision after the deliverable
// read has already established the caller's authority.
func (s *Server) previewRevision(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw, ok := s.readBody(w, r)
	if !ok {
		return 0, false
	}
	var body previewLaunchBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return 0, false
	}
	if body.Revision < 0 {
		s.writeSurface(w, nil, badRequest("bad revision: a preview launches from a numbered revision (0 = the current one)"))
		return 0, false
	}
	return body.Revision, true
}

// ── POST /api/deliverables/{deliverable}/preview ────────────────────────────

func (s *Server) handlePreviewLaunch(w http.ResponseWriter, r *http.Request) {
	if !s.reviewReady(w) || !s.previewReady(w) {
		return
	}
	// Launch visibility IS deliverable read visibility: if you may read the
	// deliverable you may try it out, and if you may not, the launch answers the
	// same 404-before-403 the read does.
	d, ok := s.deliverableScope(w, r)
	if !ok {
		return
	}
	revision, ok := s.previewRevision(w, r)
	if !ok {
		return
	}
	id, _ := IdentityFrom(r.Context())
	sess, err := s.preview.Launch(r.Context(), preview.LaunchRequest{
		DeliverableID: d.ID, Revision: revision, Role: preview.RoleSingle, User: id.UserID,
	})
	if err != nil {
		s.writeSurface(w, nil, s.previewErr(err))
		return
	}
	s.writeReadJSON(w, sess)
}

// ── POST /api/deliverables/{deliverable}/preview/compare ────────────────────

func (s *Server) handlePreviewCompare(w http.ResponseWriter, r *http.Request) {
	if !s.reviewReady(w) || !s.previewReady(w) {
		return
	}
	d, ok := s.deliverableScope(w, r)
	if !ok {
		return
	}
	revision, ok := s.previewRevision(w, r)
	if !ok {
		return
	}
	id, _ := IdentityFrom(r.Context())
	// A deliverable with no accepted revision yet answers the honest
	// single-instance state: revision 1 has no "before", and inventing one
	// would be a preview fake of a story S13.1's diff-against-base already
	// tells truthfully.
	cmp, err := s.preview.LaunchComparison(r.Context(), d.ID, revision, id.UserID)
	if err != nil {
		s.writeSurface(w, nil, s.previewErr(err))
		return
	}
	s.writeReadJSON(w, cmp)
}

// ── GET /api/previews ───────────────────────────────────────────────────────

func (s *Server) handlePreviewList(w http.ResponseWriter, r *http.Request) {
	if !s.previewReady(w) {
		return
	}
	scope := s.readScope(r)
	out := PreviewSessions{Sessions: []*preview.Session{}}
	for _, sess := range s.preview.List() {
		if scope.Operator || sess.User == scope.UserID {
			out.Sessions = append(out.Sessions, sess)
		}
	}
	cursor, err := s.head(r.Context())
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	out.Cursor = cursor
	s.writeReadJSON(w, out)
}

// ── POST /api/previews/{session}/stop ───────────────────────────────────────

// PreviewStopped is the teardown's answer.
type PreviewStopped struct {
	SessionID string `json:"session"`
	Stopped   bool   `json:"stopped"`
	Detail    string `json:"detail"`
}

func (s *Server) handlePreviewStop(w http.ResponseWriter, r *http.Request) {
	if !s.previewReady(w) {
		return
	}
	sessionID := r.PathValue("session")
	// The manager's Stop takes no actor — it is lifecycle machinery, not an
	// authorization layer — so the scope is resolved here, in the landed
	// own-plus-operator shape, and 404 comes before 403 so an unknown session id
	// never reports on another owner's session by its absence.
	var owner string
	found := false
	for _, sess := range s.preview.List() {
		if sess.ID == sessionID {
			owner, found = sess.User, true
			break
		}
	}
	if code, cerr := authorizeOwner(s.readScope(r), owner, found, "preview session"); cerr != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: code, Code: httpCode(code), Msg: cerr.Error()})
		return
	}
	if err := s.preview.Stop(r.Context(), sessionID); err != nil {
		// A partial teardown keeps the session so the stop is retryable; the
		// caller is told it did not complete rather than told it did.
		s.writeSurface(w, nil, s.previewErr(err))
		return
	}
	s.writeReadJSON(w, PreviewStopped{SessionID: sessionID, Stopped: true,
		Detail: "stopped: the backend is down, the port is back in the pool, any route is removed and the disposable substrate is deleted (S13.8)"})
}

// previewErr maps the preview module's refusals ON THE ERROR'S TYPE. The
// per-type dispositions are NOT errors and never reach here — they are states
// on a 200 answer.
func (s *Server) previewErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, preview.ErrNoRevision), errors.Is(err, preview.ErrBadInput):
		return &SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: err.Error()}
	case errors.Is(err, preview.ErrUnknownSession):
		return &SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: err.Error()}
	case errors.Is(err, review.ErrNotFound):
		return &SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: err.Error()}
	}
	s.logger.Error("preview", "err", err)
	return &SurfaceError{Status: http.StatusInternalServerError, Code: "internal", Msg: err.Error()}
}

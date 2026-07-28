package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
)

// historyapi.go routes the S14.10 three-layer queryable-history surface under
// /api/events — the S15.2 events row's "filterable history (S2.10) through the
// S14 query layers". It is a TRANSPORT and nothing else:
//
//   - it builds NO SQL and interpolates nothing into a query;
//   - it re-implements NO guardrail, audit, redaction, confidence or clamp
//     logic — every one of those lives in internal/history and stays there;
//   - it passes the caller's limit through the package's own bounds rather than
//     applying a second, different one;
//   - it never converts a REFUSAL or a DISAMBIGUATION CARD into a bare HTTP
//     error. Those are answers — the honest non-answers the design produces on
//     purpose — and an answer with an audit block on it is served with its
//     audit, at 200, so a reader can see what was refused and why.
//
// Layer 2 has its OWN route. Nothing here falls through to it: an unresolved
// Layer-1 question returns its card and stops, exactly as the store does
// internally. Reaching open SQL is an act (S14.10 ¶3 "escalation, never
// default").

// slotParamPrefix namespaces Layer-1 slot values in the query string so a slot
// can never collide with a transport parameter (`limit`, `q`). A slot named
// `project` is supplied as `?slot_project=…`.
const slotParamPrefix = "slot_"

// HistoryRegistry is the choice surface a client renders from: the Layer-0
// views with the S2.5 cost questions they answer, and the Layer-1 catalog with
// its categories and typed slots. Both are the packages' own registries,
// serialized — not a second copy maintained here.
type HistoryRegistry struct {
	Views         []history.View         `json:"views"`
	CostQuestions []history.CostQuestion `json:"cost_questions"`
	Categories    []history.Category     `json:"categories"`
	Axes          []history.SlotType     `json:"axes"`
	Queries       []history.Query        `json:"queries"`
	QueryNames    []string               `json:"query_names"`
}

// historyReady reports the query surface, or writes the honest absence. Layers
// 0 and 1 are the floor, so a process without the surface says so rather than
// answering from somewhere else.
func (s *Server) historyReady(w http.ResponseWriter) bool {
	if s.history == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S14.10 query surface is not wired in this process"})
		return false
	}
	return true
}

// GET /api/events/views — the Layer-0 registry.
// GET /api/events/catalog — the Layer-1 catalog.
// Both are static registries; they are served together from one builder so the
// two halves can never disagree about what exists.
func (s *Server) handleHistoryViews(w http.ResponseWriter, r *http.Request) {
	if !s.historyReady(w) {
		return
	}
	s.writeReadJSON(w, HistoryRegistry{
		Views:         history.Views(),
		CostQuestions: history.CostQuestions(),
	})
}

func (s *Server) handleHistoryCatalog(w http.ResponseWriter, r *http.Request) {
	if !s.historyReady(w) {
		return
	}
	s.writeReadJSON(w, HistoryRegistry{
		Categories: history.Categories,
		Axes:       history.Axes,
		Queries:    history.Catalog(),
		QueryNames: history.CatalogNames(),
	})
}

// GET /api/events/views/{view} — Layer 0: SELECT a named deterministic view.
func (s *Server) handleHistoryView(w http.ResponseWriter, r *http.Request) {
	scope, limit, ok := s.historyRequest(w, r)
	if !ok {
		return
	}
	a, err := s.history.SelectView(r.Context(), r.PathValue("view"), scope, limit)
	s.writeAnswer(w, a, err)
}

// GET /api/events/query/{query}?slot_<name>=<value> — Layer 1 direct: a named
// catalog query with its typed slots. Slot VALUES are data — they are handed to
// the store, which binds them; nothing is formatted into SQL anywhere.
func (s *Server) handleHistoryQuery(w http.ResponseWriter, r *http.Request) {
	scope, limit, ok := s.historyRequest(w, r)
	if !ok {
		return
	}
	slots := map[string]string{}
	for k, vs := range r.URL.Query() {
		if !strings.HasPrefix(k, slotParamPrefix) || len(vs) == 0 {
			continue
		}
		slots[strings.TrimPrefix(k, slotParamPrefix)] = vs[0]
	}
	a, err := s.history.RunQuery(r.Context(), r.PathValue("query"), slots, scope, limit)
	s.writeAnswer(w, a, err)
}

// GET /api/events/ask?q= — Layer 1 intent-filled: the model chooses among the
// catalog's named queries and fills their typed slots. An unresolved question
// comes back as a disambiguation card, which is a 200-class answer.
func (s *Server) handleHistoryAsk(w http.ResponseWriter, r *http.Request) {
	scope, limit, ok := s.historyRequest(w, r)
	if !ok {
		return
	}
	q, ok := historyQuestion(w, s, r)
	if !ok {
		return
	}
	a, err := s.history.Ask(r.Context(), q, scope, limit)
	s.writeAnswer(w, a, err)
}

// GET /api/events/search?q= — the redact-before-match search over the indexed
// corpus. The redaction, the marker stripping and the excerpt bounding all
// happen in the store; the transport supplies the question and nothing else.
func (s *Server) handleHistorySearch(w http.ResponseWriter, r *http.Request) {
	scope, limit, ok := s.historyRequest(w, r)
	if !ok {
		return
	}
	q, ok := historyQuestion(w, s, r)
	if !ok {
		return
	}
	a, err := s.history.Search(r.Context(), q, scope, limit)
	s.writeAnswer(w, a, err)
}

// GET /api/events/open-sql?q= — Layer 2, the ESCALATION. It is its own route
// precisely so reaching it is a deliberate act: no other handler in this file
// calls it, and none falls back to it when a card comes out of Layer 1.
//
// Every attempt — including every refusal — is audited inside the store, on the
// asker's own scope. The answer carries `confidence: "lower-confidence"`
// because the store's one constructor derives that from the layer; the
// transport does not set, check or adjust it.
func (s *Server) handleHistoryOpenSQL(w http.ResponseWriter, r *http.Request) {
	scope, limit, ok := s.historyRequest(w, r)
	if !ok {
		return
	}
	q, ok := historyQuestion(w, s, r)
	if !ok {
		return
	}
	a, err := s.history.AskOpenSQL(r.Context(), q, scope, limit)
	s.writeAnswer(w, a, err)
}

// historyRequest resolves the S01.9 scope and the caller's unclamped limit for
// one query-layer read.
func (s *Server) historyRequest(w http.ResponseWriter, r *http.Request) (history.Scope, int, bool) {
	if !s.historyReady(w) {
		return history.Scope{}, 0, false
	}
	scope := s.readScope(r)
	limit, err := readRawLimit(r)
	if err != nil {
		s.writeSurface(w, nil, err)
		return history.Scope{}, 0, false
	}
	return history.Scope{Operator: scope.Operator, UserID: scope.UserID}, limit, true
}

// historyQuestion reads the `?q=` question, refusing an empty one at the
// boundary rather than asking the layers to interpret nothing.
func historyQuestion(w http.ResponseWriter, s *Server, r *http.Request) (string, bool) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		s.writeSurface(w, nil, badRequest("a question is required: ?q=<your question>"))
		return "", false
	}
	return q, true
}

// writeAnswer serializes one history.Answer VERBATIM — layer, query name,
// confidence, notes, disambiguation card and Layer-2 audit exactly as the store
// set them.
//
// The error handling is the load-bearing part. internal/history returns a
// populated Answer ALONGSIDE an error on every path where something honest was
// produced: a Layer-2 refusal, an unavailable seat, a timed-out query. Those
// answers carry their audit block and they are the record of what happened, so
// collapsing them into an HTTP status would throw away the one artifact the
// guardrail exists to produce. Only a genuinely EMPTY answer — a caller naming
// a view or query that is not in the closed registry, or a slot error — maps to
// a status, because there is nothing to serve.
func (s *Server) writeAnswer(w http.ResponseWriter, a history.Answer, err error) {
	if err != nil && a.Query == "" {
		s.writeSurfaceErr(w, historyStatus(err))
		return
	}
	if err != nil {
		a.Notes = append(a.Notes, err.Error())
	}
	// Redacted at the edge like every payload-bearing REST response: a catalog
	// query or a generated statement may select run_events.payload, and the
	// primitive is idempotent, so a search answer that was already redacted
	// before matching passes through byte-unchanged (§30).
	s.writeReadRedacted(w, a)
}

// historyStatus maps the closed-registry errors onto statuses. Everything else
// is an internal failure — never a silently empty answer.
func historyStatus(err error) *SurfaceError {
	switch {
	case errors.Is(err, history.ErrNoSuchView), errors.Is(err, history.ErrNoSuchQuery):
		return &SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: err.Error()}
	case errors.Is(err, history.ErrSlot):
		return &SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: err.Error()}
	default:
		return &SurfaceError{Status: http.StatusInternalServerError, Code: "internal", Msg: err.Error()}
	}
}

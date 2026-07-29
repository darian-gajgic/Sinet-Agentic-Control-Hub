package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/chat"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/redact"
)

// chatapi.go — the S15.7 conversational assistant family (P3-B6-7 part A):
// `/api/chat`. Sessions, the immutable transcript, the turn verbs, the file
// exchange, and the S06 intake handoff.
//
// THE THREE VERBS ARE THE WHOLE SURFACE, AND THIS FILE ROUTES THEM WITHOUT
// GUESSING. S15.7 grants the assistant exactly three things — a platform-state
// question, starting a task by chatting, and the sessions/exchange machinery
// around them — and S14.10 sharpens the first: "the conversational assistant
// consumes these layers AND NOTHING ELSE". No free-generation chat duty exists
// anywhere in the spec, so none exists here.
//
// THE CLIENT ROUTES; THE SERVER ACTS (OQ3(a)). A turn arrives carrying the verb
// the person's own gesture chose, from a closed vocabulary
// (ask|view|query|open_sql|task). NO MODEL CLASSIFIES A TURN. That is the point:
// a classifier that misfires could start a task nobody asked for, or escalate a
// question to open SQL nobody escalated, and neither failure is one a person can
// see coming. Typed prose defaults to `ask` — the Layer-1 floor — whose own
// disambiguation card IS the honest refusal when a turn is none of the three
// verbs. The escalation to Layer 2 is a rendered control the person presses, so
// "reaching open SQL is an act" survives intact: nothing here falls through to
// it, and `open_sql` reaches AskOpenSQL only because the caller named it.
//
// THE SERVER ACTS, AND THAT IS WHAT MAKES THE TRANSCRIPT REAL. The turn POST
// performs the act and records the outcome, so the transcript is SERVED state
// rather than something the browser assembled and might lose: a reload restores
// it, a second tab sees it, and a widget swap loses no data (S15.4 — the
// ExternalStoreRuntime boundary is exactly this). A client that posted its own
// answers back would be authoring the record of what the platform said.
//
// NOTHING IS RE-IMPLEMENTED HERE. The query ladder is `s.history`'s — this file
// builds no SQL, derives no confidence, re-checks no guardrail and adds no
// second audit. The intake pipeline is `s.intake`'s — the handoff calls the one
// landed Submit (S16.6: Enqueue is the sole ingress). Answers, cards and
// audited refusals are recorded and served EXACTLY as the store made them,
// which is what keeps a Layer-2 answer's `lower-confidence` flag visible
// wherever it renders (G3 D3.5).
//
// NO OUTWARD EFFECT EXISTS IN THIS FAMILY (S15.2; D7). Nothing sends, publishes
// or pushes. Handing a file to a task is an internal state act; the born task's
// own outward effects stay gated downstream where they belong.

// maxChatUpload bounds an exchange upload body. It is the file bound plus a
// small allowance so an at-the-limit file is refused by chat.MaxFileBytes with
// its own reason, rather than by the transport with a vaguer one — the same
// read-one-more-byte-than-the-bound shape as maxIntakeBody.
const maxChatUpload = chat.MaxFileBytes + (1 << 10)

func (s *Server) chatReady(w http.ResponseWriter) bool {
	if s.chat == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the conversational assistant's store is not wired in this process"})
		return false
	}
	return true
}

// chatCaller resolves the acting identity. Chat reads are OWNER-ONLY — the
// store takes a viewer and has no role parameter at all (P3-B6-7 OQ1) — so
// unlike every other family here there is no operator limb to resolve.
func chatCaller(r *http.Request) string {
	id, _ := IdentityFrom(r.Context()) // present: every chat route is requireIdentity
	return id.UserID
}

// chatStatus maps the store's errors onto statuses by TYPE, never by message
// text (§38). ErrNotFound covers "not yours" as well as "not there", which is
// the 404-before-403 discipline made structural: an id must not become an
// existence oracle for another person's conversation.
func chatStatus(err error) *SurfaceError {
	switch {
	case errors.Is(err, chat.ErrNotFound):
		return &SurfaceError{Status: http.StatusNotFound, Code: "not_found", Msg: err.Error()}
	case errors.Is(err, chat.ErrInvalid):
		return &SurfaceError{Status: http.StatusBadRequest, Code: "bad_request", Msg: err.Error()}
	case errors.Is(err, chat.ErrTooLarge):
		return &SurfaceError{Status: http.StatusRequestEntityTooLarge, Code: "too_large", Msg: err.Error()}
	case errors.Is(err, chat.ErrTooMany):
		return &SurfaceError{Status: http.StatusConflict, Code: "too_many", Msg: err.Error()}
	case errors.Is(err, chat.ErrTurnRunning):
		return &SurfaceError{Status: http.StatusConflict, Code: "turn_running", Msg: err.Error()}
	default:
		return &SurfaceError{Status: http.StatusInternalServerError, Code: "internal", Msg: err.Error()}
	}
}

func (s *Server) writeChatErr(w http.ResponseWriter, err error) {
	var se *SurfaceError
	if errors.As(err, &se) {
		s.writeSurfaceErr(w, se)
		return
	}
	s.writeSurfaceErr(w, chatStatus(err))
}

// serveTurn redacts a turn's stored OUTCOME at the serving edge and nothing
// else in the response (§30 store-raw / serve-redacted).
//
// The line is CONTENT vs PROJECTION, exactly as §38 ruling (b) and §40-C draw
// it. A message body and a session title are chat's own content, read from
// chat's own tables, and go out unwrapped. A turn's outcome is a history.Answer
// whose rows are lifted out of run_events — a projection — so it passes through
// the codor-C2 primitive per serialization. The stored row is never touched.
func serveTurn(t chat.Turn) chat.Turn {
	if len(t.Outcome) > 0 {
		t.Outcome = redact.RedactJSON(t.Outcome)
	}
	return t
}

func serveTurns(ts []chat.Turn) []chat.Turn {
	for i := range ts {
		ts[i] = serveTurn(ts[i])
	}
	return ts
}

// ---- Sessions ----

// chatSessionView is one session with everything the surface needs to render it
// without a second round trip: the transcript, the turns with their outcomes,
// and the running turn if there is one — which is what a view re-entered
// mid-turn re-attaches to (R14).
type chatSessionView struct {
	Session  chat.Session   `json:"session"`
	Messages []chat.Message `json:"messages"`
	Turns    []chat.Turn    `json:"turns"`
	// Running is the in-flight turn, or nil. It is SERVER state, so leaving the
	// view — or reloading the page entirely — cannot lose it.
	Running *chat.Turn `json:"running,omitempty"`
	// TitleSeatWired states whether the auto-titling duty has a seat and a
	// metering run, so a mechanically-named session can say WHY rather than
	// leaving a person to wonder (§37 honest absence).
	TitleSeatWired bool `json:"title_seat_wired"`
}

// GET /api/chat/sessions
func (s *Server) handleChatSessionList(w http.ResponseWriter, r *http.Request) {
	if !s.chatReady(w) {
		return
	}
	list, err := s.chat.ListSessions(r.Context(), chatCaller(r))
	if err != nil {
		s.writeChatErr(w, err)
		return
	}
	s.writeReadJSON(w, struct {
		Sessions []chat.Session `json:"sessions"`
	}{list})
}

// POST /api/chat/sessions
func (s *Server) handleChatSessionCreate(w http.ResponseWriter, r *http.Request) {
	if !s.chatReady(w) {
		return
	}
	v, err := s.chat.CreateSession(r.Context(), chatCaller(r))
	if err != nil {
		s.writeChatErr(w, err)
		return
	}
	s.writeReadJSON(w, struct {
		Session chat.Session `json:"session"`
	}{v})
}

// GET /api/chat/sessions/{session}
func (s *Server) handleChatSessionDetail(w http.ResponseWriter, r *http.Request) {
	if !s.chatReady(w) {
		return
	}
	caller, id := chatCaller(r), r.PathValue("session")
	view, err := s.chatSessionView(r.Context(), caller, id)
	if err != nil {
		s.writeChatErr(w, err)
		return
	}
	s.writeReadJSON(w, view)
}

func (s *Server) chatSessionView(ctx context.Context, caller, id string) (chatSessionView, error) {
	sess, err := s.chat.Session(ctx, caller, id)
	if err != nil {
		return chatSessionView{}, err
	}
	msgs, err := s.chat.Messages(ctx, caller, id)
	if err != nil {
		return chatSessionView{}, err
	}
	turns, err := s.chat.Turns(ctx, caller, id)
	if err != nil {
		return chatSessionView{}, err
	}
	out := chatSessionView{Session: sess, Messages: msgs, Turns: serveTurns(turns),
		TitleSeatWired: s.chat.TitleSeatWired()}
	if running, ok, err := s.chat.RunningTurn(ctx, caller, id); err != nil {
		return chatSessionView{}, err
	} else if ok {
		t := serveTurn(running)
		out.Running = &t
	}
	return out, nil
}

// POST /api/chat/sessions/{session}/rename
func (s *Server) handleChatSessionRename(w http.ResponseWriter, r *http.Request) {
	if !s.chatReady(w) {
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	v, err := s.chat.RenameSession(r.Context(), chatCaller(r), r.PathValue("session"), body.Title)
	if err != nil {
		s.writeChatErr(w, err)
		return
	}
	s.writeReadJSON(w, struct {
		Session chat.Session `json:"session"`
	}{v})
}

// POST /api/chat/sessions/{session}/delete — the owner-only HARD delete. A
// repeat answers the already-resolved state with applied:false rather than a
// refusal (S15.2 retry-safety), so a phone retry is safe.
func (s *Server) handleChatSessionDelete(w http.ResponseWriter, r *http.Request) {
	if !s.chatReady(w) {
		return
	}
	res, err := s.chat.DeleteSession(r.Context(), chatCaller(r), r.PathValue("session"))
	if err != nil {
		s.writeChatErr(w, err)
		return
	}
	s.writeReadJSON(w, res)
}

// ---- Turns ----

// chatTurnBody is the turn submit payload. `kind` is the client's routing
// decision from the closed vocabulary; the rest are that verb's arguments.
type chatTurnBody struct {
	Kind string `json:"kind"`
	// Text is what the person typed. It is ALWAYS recorded as the message,
	// whatever the kind — a turn the person could not read back afterwards
	// would make the transcript a lie by omission.
	Text string `json:"text"`
	// Layer-0 / Layer-1-direct arguments, chosen from the SERVED registries.
	View  string            `json:"view,omitempty"`
	Query string            `json:"query,omitempty"`
	Slots map[string]string `json:"slots,omitempty"`
	Limit int               `json:"limit,omitempty"`
	// The S06 handoff's arguments.
	Title string `json:"title,omitempty"`
	// Inputs are exchange-manifest ids handed to the born task. They are
	// resolved against the CALLER's own folder, so a ref to another person's
	// object is simply not found (OQ8).
	Inputs []string `json:"inputs,omitempty"`
}

type chatTurnResponse struct {
	Turn    chat.Turn    `json:"turn"`
	Message chat.Message `json:"message,omitempty"`
	Session chat.Session `json:"session"`
}

// POST /api/chat/sessions/{session}/turns — the one turn verb.
//
// It is SYNCHRONOUS and it yields the outcome when ready (OQ5(a)). There is
// nothing to stream: a ladder answer arrives whole, an intake handoff returns a
// born task, and no engine session runs for a chat turn (S12.1). Currency for
// other tabs and post-navigation re-reads rides the minted chat.* events on the
// one relay — a second stream shape would buy nothing and cost the one-
// EventSource rule.
func (s *Server) handleChatTurnSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.chatReady(w) {
		return
	}
	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body chatTurnBody
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	if body.Kind == "" {
		// Typed prose with no gesture behind it is a question for the Layer-1
		// floor. It is a default, not a classification: the floor's own card
		// answers when nothing resolves.
		body.Kind = chat.KindAsk
	}
	caller, sessionID := chatCaller(r), r.PathValue("session")

	turn, msg, err := s.chat.BeginTurn(r.Context(), caller, sessionID, body.Kind, body.Text)
	if err != nil {
		if errors.Is(err, chat.ErrTurnRunning) {
			// The already-running turn comes back with the refusal, so the
			// surface can re-attach instead of losing it.
			s.writeSurfaceErr(w, chatStatus(err))
			return
		}
		s.writeChatErr(w, err)
		return
	}

	// The act runs under a cancellable context registered against the turn, so
	// the hard-stop verb — arriving on a different request — cancels it
	// server-side "where cancellable" (OQ6).
	ctx, release := s.chat.Drive(r.Context(), turn.ID)
	defer release()
	outcomeKind, outcome := s.runChatTurn(ctx, r, caller, body)

	settled, err := s.chat.SettleTurn(r.Context(), caller, turn.ID, outcomeKind, outcome)
	if err != nil && !errors.Is(err, chat.ErrTurnResolved) {
		s.writeChatErr(w, err)
		return
	}
	// ErrTurnResolved here means the person hard-stopped while the act ran. The
	// abandoned record STANDS and `settled` is that record: the result arrived
	// late and renders with its honest state (OQ6). Nothing is overwritten.
	sess, err := s.chat.Session(r.Context(), caller, sessionID)
	if err != nil {
		s.writeChatErr(w, err)
		return
	}
	s.writeReadJSON(w, chatTurnResponse{Turn: serveTurn(settled), Message: msg, Session: sess})
}

// runChatTurn performs the routed act and returns the outcome to record. It is
// the whole of the routing: five closed kinds, each reaching one LANDED verb.
func (s *Server) runChatTurn(ctx context.Context, r *http.Request, caller string, body chatTurnBody) (string, json.RawMessage) {
	switch body.Kind {
	case chat.KindTask:
		return s.chatHandoff(ctx, caller, body)
	case chat.KindAsk, chat.KindView, chat.KindQuery, chat.KindOpenSQL:
		return s.chatLadder(ctx, r, body)
	}
	// Unreachable: BeginTurn already refused any kind outside the closed set.
	return chat.OutcomeError, chatErrorOutcome("bad_request", "unknown turn kind")
}

// chatLadder runs one query-layer read through the LANDED store and records
// what it returned.
//
// The error handling mirrors writeAnswer's, and for the same reason:
// internal/history returns a POPULATED answer alongside an error on every path
// where something honest was produced — a Layer-2 refusal, an unavailable seat,
// a timed-out query — and those answers carry the audit block that is the whole
// artifact the guardrail exists to produce. Collapsing one into an error would
// throw that record away. Only a genuinely empty answer becomes an error
// outcome.
func (s *Server) chatLadder(ctx context.Context, r *http.Request, body chatTurnBody) (string, json.RawMessage) {
	if s.history == nil {
		return chat.OutcomeError, chatErrorOutcome("not_wired",
			"the S14.10 query layers are not wired in this process")
	}
	scope := s.readScope(r)
	hs := history.Scope{Operator: scope.Operator, UserID: scope.UserID}
	var (
		a   history.Answer
		err error
	)
	switch body.Kind {
	case chat.KindAsk:
		a, err = s.history.Ask(ctx, body.Text, hs, body.Limit)
	case chat.KindView:
		// The operator-only bit is registry METADATA, so the refusal is decided
		// on the same closed registry the store consults — never by matching an
		// error string (§38 D5).
		if v, ok := history.ViewByName(body.View); ok && v.OwnerColumn == "" && !hs.Operator {
			return chat.OutcomeError, chatErrorOutcome("forbidden",
				"view "+v.Name+" carries no owner column and is operator-only (S01.9)")
		}
		a, err = s.history.SelectView(ctx, body.View, hs, body.Limit)
	case chat.KindQuery:
		a, err = s.history.RunQuery(ctx, body.Query, body.Slots, hs, body.Limit)
	case chat.KindOpenSQL:
		a, err = s.history.AskOpenSQL(ctx, body.Text, hs, body.Limit)
	}
	if err != nil && a.Query == "" {
		se := historyStatus(err)
		return chat.OutcomeError, chatErrorOutcome(se.Code, se.Msg)
	}
	if err != nil {
		a.Notes = append(a.Notes, err.Error())
	}
	encoded, mErr := json.Marshal(a)
	if mErr != nil {
		return chat.OutcomeError, chatErrorOutcome("internal", "answer encoding failed")
	}
	return chat.OutcomeAnswer, encoded
}

// chatHandoff is S15.7's "start a task by chatting, which hands the
// conversation to the S06 intake pipeline IN PLACE".
//
// It calls the ONE landed ingress (S16.6) as the authenticated caller. The
// returned task view carries the born task's id, title and OPEN INTAKE CARD, so
// the conversation continues in the chat feed against the same card the task
// surface would show, answered through the LANDED ask verbs — no second answer
// path, no second pipeline entry, and no lifecycle authority claimed over the
// task that was born.
func (s *Server) chatHandoff(ctx context.Context, caller string, body chatTurnBody) (string, json.RawMessage) {
	if s.intake == nil {
		return chat.OutcomeError, chatErrorOutcome("not_wired", "the intake pipeline is not wired in this process")
	}
	inputs, err := s.chatInputs(ctx, caller, body.Inputs)
	if err != nil {
		se := chatStatus(err)
		return chat.OutcomeError, chatErrorOutcome(se.Code, se.Msg)
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		title = body.Text
	}
	submit, err := json.Marshal(struct {
		Title  string         `json:"title"`
		Text   string         `json:"text"`
		Inputs []intake.Input `json:"inputs,omitempty"`
	}{title, body.Text, inputs})
	if err != nil {
		return chat.OutcomeError, chatErrorOutcome("internal", "request encoding failed")
	}
	view, err := s.intake.Submit(ctx, caller, submit)
	if err != nil {
		var se *SurfaceError
		if errors.As(err, &se) {
			return chat.OutcomeError, chatErrorOutcome(se.Code, se.Msg)
		}
		return chat.OutcomeError, chatErrorOutcome("internal", err.Error())
	}
	return chat.OutcomeTask, view
}

// chatInputs resolves handed-file refs against the CALLER's OWN exchange
// manifest and returns the requester-supplied input descriptors S06.3/S06.6
// records. A ref to an object the caller does not own is simply not found —
// cross-owner handoff is refused by the same predicate that scopes every other
// read in this family, not by a check somebody has to remember to write.
func (s *Server) chatInputs(ctx context.Context, caller string, refs []string) ([]intake.Input, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	if len(refs) > chat.MaxFilesPerOwner {
		return nil, fmt.Errorf("%w: too many inputs", chat.ErrInvalid)
	}
	out := make([]intake.Input, 0, len(refs))
	for _, ref := range refs {
		f, err := s.chat.FileRow(ctx, caller, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, intake.Input{Ref: f.ID, Name: f.Name, SizeBytes: f.SizeBytes, SHA256: f.SHA256})
	}
	return out, nil
}

// chatErrorOutcome is the recorded shape of a turn that failed. It is the same
// {error, detail} pair the transport serves everywhere else, so a surface reads
// one error vocabulary rather than two.
func chatErrorOutcome(code, detail string) json.RawMessage {
	raw, err := json.Marshal(struct {
		Error  string `json:"error"`
		Detail string `json:"detail,omitempty"`
	}{code, detail})
	if err != nil {
		return json.RawMessage(`{"error":"internal"}`)
	}
	return raw
}

// POST /api/chat/turns/{turn}/stop — the S15.7 hard-stop.
//
// It is an EXPLICIT USER ACTION with a record, never a navigation side effect
// (OQ6). It abandons the turn, mints the event and cancels the driving request
// where one is live. It does NOT cancel a task the turn handed off: 4.5's cancel
// is the task's own verb and chat claims no authority over a task's lifecycle.
// A repeat answers the resolved state (S15.2 retry-safety).
func (s *Server) handleChatTurnStop(w http.ResponseWriter, r *http.Request) {
	if !s.chatReady(w) {
		return
	}
	t, err := s.chat.AbandonTurn(r.Context(), chatCaller(r), r.PathValue("turn"))
	if err != nil && !errors.Is(err, chat.ErrTurnResolved) {
		s.writeChatErr(w, err)
		return
	}
	s.writeReadJSON(w, struct {
		Turn chat.Turn `json:"turn"`
		// Applied is false when the turn had already ended — the already-
		// resolved state, not a refusal.
		Applied bool `json:"applied"`
	}{serveTurn(t), err == nil})
}

// ---- The file exchange ----

// GET /api/chat/files
func (s *Server) handleChatFileList(w http.ResponseWriter, r *http.Request) {
	if !s.chatReady(w) {
		return
	}
	list, err := s.chat.ListFiles(r.Context(), chatCaller(r))
	if err != nil {
		s.writeChatErr(w, err)
		return
	}
	s.writeReadJSON(w, struct {
		Files []chat.File `json:"files"`
	}{list})
}

// POST /api/chat/files?name=<filename>[&session=&turn=] — the ONE upload verb.
//
// The body is the raw object; the name rides a query parameter. Both the
// drag-drop and the click-browse gesture drive this same verb — two gestures,
// one act, so there is no second write path to keep in step.
func (s *Server) handleChatFileUpload(w http.ResponseWriter, r *http.Request) {
	if !s.chatReady(w) {
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxChatUpload+1))
	if err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	if len(data) > maxChatUpload {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusRequestEntityTooLarge, Code: "too_large",
			Msg: fmt.Sprintf("upload exceeds the %d-byte bound", chat.MaxFileBytes)})
		return
	}
	q := r.URL.Query()
	f, err := s.chat.Upload(r.Context(), chatCaller(r), q.Get("name"), data,
		q.Get("session"), q.Get("turn"))
	if err != nil {
		s.writeChatErr(w, err)
		return
	}
	s.writeReadJSON(w, struct {
		File chat.File `json:"file"`
	}{f})
}

// POST /api/chat/files/{file}/delete — a repeat answers applied:false.
func (s *Server) handleChatFileDelete(w http.ResponseWriter, r *http.Request) {
	if !s.chatReady(w) {
		return
	}
	res, err := s.chat.Delete(r.Context(), chatCaller(r), r.PathValue("file"))
	if err != nil {
		s.writeChatErr(w, err)
		return
	}
	s.writeReadJSON(w, res)
}

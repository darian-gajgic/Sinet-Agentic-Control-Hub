package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/chat"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/history"
)

// chatapi_test.go — the S15.7 conversational assistant family (P3-B6-7 part A).
//
// The battery composes the REAL subsystems: a migrated platform.db through
// migration 0020, the real event log, the real chat store over an exchange root
// in t.TempDir(), and the REAL S14.10 query surface — so every answer a turn
// records is the one internal/history made, and every event counted is the one
// the store minted inside its own transaction.
//
// $0 AND HERMETIC. The query surface is composed WITHOUT a duty caller, which
// is both the cheapest posture and an honest production one: Layers 0 and 1 are
// the floor and answer without a model, Ask degrades to its disambiguation
// card, and Layer 2 is unavailable and says so with its audit. Nothing dials a
// network, spawns a process or prices a call.

type chatEnv struct {
	t     *testing.T
	b     *backend
	ctx   context.Context
	store *chat.Store
	hist  *history.Store
	fake  *fakeSurface
	// intake is the S06 seam the servers compose. It defaults to fake; the
	// client-disconnect rig swaps it for one that drops the connection mid-act.
	intake api.IntakeSurface
}

func newChatEnv(t *testing.T) *chatEnv {
	t.Helper()
	ctx := context.Background()
	b := newBackend(t)
	// REAL identities with REAL roles (drain r1 D4). With an empty auth store
	// isOperatorRead resolves false for everybody, "op" is merely a second
	// member, and the operator-is-refused-too limb — this packet's headline
	// stated deviation — would be a duplicate of the bob limb wearing a label.
	if err := b.store.CreateUser(ctx, "",
		auth.User{ID: "op", DisplayName: "Op", Role: auth.RoleOperator}, fixturePIN); err != nil {
		t.Fatalf("create the operator: %v", err)
	}
	for _, id := range []string{"alice", "bob"} {
		if err := b.store.CreateUser(ctx, "op",
			auth.User{ID: id, DisplayName: id, Role: auth.RoleMember}, fixturePIN); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	root := filepath.Join(t.TempDir(), "exchange")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("mkdir exchange: %v", err)
	}
	store, err := chat.New(chat.Config{DB: b.db, Log: b.log, Root: root})
	if err != nil {
		t.Fatalf("chat.New: %v", err)
	}
	hist, err := history.New(history.Config{DB: b.db, Log: b.log})
	if err != nil {
		t.Fatalf("history.New: %v", err)
	}
	fake := &fakeSurface{}
	return &chatEnv{t: t, b: b, ctx: ctx, store: store, hist: hist, fake: fake, intake: fake}
}

func (e *chatEnv) server(who string) *api.Server {
	e.t.Helper()
	return api.New(api.Config{
		Log:      e.b.log,
		Sessions: e.b.store,
		Auth:     fixedIdentity{who},
		Settings: fixedSettings{d: 20 * time.Second},
		HealthFn: func() api.Health { return api.Health{Ready: true, Mode: "running", Version: "test"} },
		DB:       e.b.db,
		Meter:    fakeMeter{},
		Chat:     e.store,
		History:  e.hist,
		Intake:   e.intake,
	})
}

func (e *chatEnv) do(t *testing.T, who, method, path, body string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	e.server(who).Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

func (e *chatEnv) mustDo(t *testing.T, who, method, path, body string) string {
	t.Helper()
	code, out := e.do(t, who, method, path, body)
	if code != http.StatusOK {
		t.Fatalf("%s %s as %s: status %d: %s", method, path, who, code, out)
	}
	return out
}

// session creates one through the REAL verb and returns its id.
func (e *chatEnv) session(t *testing.T, who string) string {
	t.Helper()
	var out struct {
		Session chat.Session `json:"session"`
	}
	decodeInto(t, e.mustDo(t, who, "POST", "/api/chat/sessions", ""), &out)
	return out.Session.ID
}

func decodeInto(t *testing.T, raw string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		t.Fatalf("decode %T: %v: %s", v, err, raw)
	}
}

type turnResponse struct {
	Turn    chat.Turn    `json:"turn"`
	Message chat.Message `json:"message"`
	Session chat.Session `json:"session"`
}

func (e *chatEnv) turn(t *testing.T, who, sessionID, body string) turnResponse {
	t.Helper()
	var out turnResponse
	decodeInto(t, e.mustDo(t, who, "POST", "/api/chat/sessions/"+sessionID+"/turns", body), &out)
	return out
}

func (e *chatEnv) eventCount(t *testing.T, typ string) int {
	t.Helper()
	var n int
	if err := e.b.db.QueryRowContext(e.ctx,
		`SELECT count(*) FROM run_events WHERE type = ?`, typ).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", typ, err)
	}
	return n
}

// ── Owner scope, three ways, over HTTP ──────────────────────────────────────

// TestChatIsOwnerOnlyOverHTTP pins the OQ1 deviation at the transport: alice
// reads her own thread (so an all-empty handler cannot pass), bob is refused,
// and THE OPERATOR IS REFUSED TOO — the deliberate narrowing of §30's
// operator-sees-all, because a conversation is content rather than telemetry.
func TestChatIsOwnerOnlyOverHTTP(t *testing.T) {
	e := newChatEnv(t)
	sid := e.session(t, "alice")
	e.turn(t, "alice", sid, `{"kind":"ask","text":"what is running?"}`)

	// The owner's own rows must be PRESENT — the non-tautological direction.
	var mine struct {
		Sessions []chat.Session `json:"sessions"`
	}
	decodeInto(t, e.mustDo(t, "alice", "GET", "/api/chat/sessions", ""), &mine)
	if len(mine.Sessions) != 1 || mine.Sessions[0].ID != sid {
		t.Fatalf("alice's list = %+v", mine.Sessions)
	}
	var detail struct {
		Session  chat.Session   `json:"session"`
		Messages []chat.Message `json:"messages"`
		Turns    []chat.Turn    `json:"turns"`
	}
	decodeInto(t, e.mustDo(t, "alice", "GET", "/api/chat/sessions/"+sid, ""), &detail)
	if len(detail.Messages) != 1 || detail.Messages[0].Body != "what is running?" {
		t.Fatalf("alice's transcript = %+v", detail.Messages)
	}

	for _, who := range []string{"bob", "op"} {
		var theirs struct {
			Sessions []chat.Session `json:"sessions"`
		}
		decodeInto(t, e.mustDo(t, who, "GET", "/api/chat/sessions", ""), &theirs)
		if len(theirs.Sessions) != 0 {
			t.Errorf("%s sees %d of alice's sessions — chat is owner-only, operator included", who, len(theirs.Sessions))
		}
		for _, probe := range []struct{ method, path, body string }{
			{"GET", "/api/chat/sessions/" + sid, ""},
			{"POST", "/api/chat/sessions/" + sid + "/rename", `{"title":"hijacked"}`},
			{"POST", "/api/chat/sessions/" + sid + "/turns", `{"kind":"ask","text":"x"}`},
		} {
			code, out := e.do(t, who, probe.method, probe.path, probe.body)
			if code != http.StatusNotFound {
				t.Errorf("%s %s as %s = %d (%s), want 404 — an id must not be an existence oracle",
					probe.method, probe.path, who, code, out)
			}
		}
	}
	// A stranger's delete answers the already-resolved state and removes nothing.
	var del chat.DeleteResult
	decodeInto(t, e.mustDo(t, "bob", "POST", "/api/chat/sessions/"+sid+"/delete", ""), &del)
	if del.Applied {
		t.Error("bob's delete of alice's session reported applied:true")
	}
	if _, err := e.store.Session(e.ctx, "alice", sid); err != nil {
		t.Fatalf("alice's session must survive: %v", err)
	}

	// ── The limb is not a tautology: "op" really IS the operator here ─────────
	//
	// `task_project` is the one Layer-0 view with no owner column, which S01.9
	// makes operator-only — and the chat transport decides that on the SERVED
	// scope, so the same surface that just refused op alice's thread answers his
	// own operator-scoped question and refuses bob's. If isOperatorRead were
	// resolving false for op (an empty auth store, drain r1 D4), this would
	// fail and the refusals above would be a second member limb.
	opSession := e.session(t, "op")
	opView := e.turn(t, "op", opSession,
		`{"kind":"view","view":"task_project","text":"the task/project edge"}`)
	if opView.Turn.OutcomeKind != chat.OutcomeAnswer {
		t.Fatalf("the operator was refused the operator-only view: %s — his role is not resolving, so every refusal above is vacuous",
			opView.Turn.Outcome)
	}
	bobSession := e.session(t, "bob")
	bobView := e.turn(t, "bob", bobSession,
		`{"kind":"view","view":"task_project","text":"the task/project edge"}`)
	if bobView.Turn.OutcomeKind != chat.OutcomeError || !strings.Contains(string(bobView.Turn.Outcome), "forbidden") {
		t.Fatalf("a member reached the operator-only view: %q %s", bobView.Turn.OutcomeKind, bobView.Turn.Outcome)
	}
	// And the operator's read privilege stops at the chat family's door: he can
	// answer an operator-only question and still not see alice's conversation.
	if code, _ := e.do(t, "op", "GET", "/api/chat/sessions/"+sid, ""); code != http.StatusNotFound {
		t.Errorf("the operator read alice's session detail = %d, want 404", code)
	}
}

// ── Turn routing: the five closed kinds, each reaching one landed verb ───────

// TestTurnRoutingReachesTheLandedLadder drives every query-layer kind through
// the REAL history store and asserts the recorded outcome is the store's own
// answer — layer, query name and confidence as the store set them.
func TestTurnRoutingReachesTheLandedLadder(t *testing.T) {
	e := newChatEnv(t)
	sid := e.session(t, "alice")

	// Layer 0: a named deterministic view.
	view := e.turn(t, "alice", sid, `{"kind":"view","view":"cost_per_run","text":"cost per run"}`)
	a := decodeTurnAnswer(t, view.Turn)
	if a.Layer != 0 || a.Query != "cost_per_run" || a.Confidence != "deterministic" {
		t.Fatalf("Layer-0 turn recorded %+v, want the store's own deterministic answer", a)
	}
	if view.Turn.State != chat.StateSettled || view.Turn.OutcomeKind != chat.OutcomeAnswer {
		t.Fatalf("turn = %+v", view.Turn)
	}
	settle(t, e, "alice", sid)

	// Layer 1 direct: a named catalog query with its typed slots.
	q := e.turn(t, "alice", sid, `{"kind":"query","query":"status.runs_by_state","slots":{"state":"running"},"text":"which runs are running?"}`)
	a = decodeTurnAnswer(t, q.Turn)
	if a.Layer != 1 || a.Query != "status.runs_by_state" || a.Confidence != "canned" {
		t.Fatalf("Layer-1 direct turn recorded %+v, want the store's canned answer", a)
	}
	settle(t, e, "alice", sid)

	// A name outside the CLOSED registry is an error outcome, not a silently
	// empty answer.
	bad := e.turn(t, "alice", sid, `{"kind":"view","view":"not_a_view","text":"?"}`)
	if bad.Turn.OutcomeKind != chat.OutcomeError {
		t.Fatalf("an unknown view recorded %q, want an error outcome", bad.Turn.OutcomeKind)
	}
	settle(t, e, "alice", sid)

	// The routing decision is VISIBLE on every record.
	var detail struct {
		Turns []chat.Turn `json:"turns"`
	}
	decodeInto(t, e.mustDo(t, "alice", "GET", "/api/chat/sessions/"+sid, ""), &detail)
	if len(detail.Turns) != 3 {
		t.Fatalf("turns = %d, want 3", len(detail.Turns))
	}
	for _, turn := range detail.Turns {
		if turn.Kind == "" {
			t.Errorf("turn %s records no kind — which verb answered must be visible", turn.ID)
		}
	}
}

func decodeTurnAnswer(t *testing.T, turn chat.Turn) history.Answer {
	t.Helper()
	var a history.Answer
	if err := json.Unmarshal(turn.Outcome, &a); err != nil {
		t.Fatalf("decode the recorded answer: %v: %s", err, turn.Outcome)
	}
	return a
}

// settle stops any turn left running so the next one may open (one thread per
// conversation). A settled turn needs nothing; a running one is abandoned.
func settle(t *testing.T, e *chatEnv, who, sid string) {
	t.Helper()
	running, ok, err := e.store.RunningTurn(e.ctx, who, sid)
	if err != nil {
		t.Fatalf("running turn: %v", err)
	}
	if ok {
		e.mustDo(t, who, "POST", "/api/chat/turns/"+running.ID+"/stop", "")
	}
}

// TestNeitherVerbTurnGetsTheAskFloorsOwnCard: a turn that is none of the three
// S15.7 verbs is answered by the Layer-1 floor's own disambiguation card. There
// is no free-generation path to fall into, and no Layer-2 call is made.
func TestNeitherVerbTurnGetsTheAskFloorsOwnCard(t *testing.T) {
	e := newChatEnv(t)
	sid := e.session(t, "alice")

	// No `kind`: typed prose defaults to the floor. That is a DEFAULT, not a
	// classification — nothing inspects the words.
	got := e.turn(t, "alice", sid, `{"text":"write me a poem about the sea"}`)
	if got.Turn.Kind != chat.KindAsk {
		t.Fatalf("an unkinded turn routed to %q, want the ask floor", got.Turn.Kind)
	}
	a := decodeTurnAnswer(t, got.Turn)
	if a.Card == nil {
		t.Fatalf("a neither-verb turn must render the floor's own card, got %+v", a)
	}
	if len(a.Card.Choices) == 0 {
		t.Error("the refusal card must carry real catalog choices")
	}
	if a.Rows != nil && len(a.Rows) != 0 {
		t.Errorf("a card answer must carry no fabricated rows, got %v", a.Rows)
	}
	// The card's choices are REAL catalog queries — pickable, not decorative.
	names := map[string]bool{}
	for _, c := range history.Catalog() {
		names[c.Name] = true
	}
	for _, c := range a.Card.Choices {
		if !names[c.Query] {
			t.Errorf("card choice %q is not a catalog query", c.Query)
		}
	}
	// And nothing reached Layer 2.
	if n := e.eventCount(t, "history.query_audited"); n != 0 {
		t.Errorf("a neither-verb turn produced %d Layer-2 audit rows — nothing may fall through to open SQL", n)
	}
}

// TestNothingFallsThroughToLayerTwo drives every non-open_sql kind and asserts
// the Layer-2 audit count stays at ZERO; then names open_sql explicitly and
// asserts the audit row appears with the answer at 200. Reaching open SQL is an
// ACT, and this is the measurement of it.
func TestNothingFallsThroughToLayerTwo(t *testing.T) {
	e := newChatEnv(t)
	sid := e.session(t, "alice")
	for _, body := range []string{
		`{"kind":"ask","text":"which runs cost the most?"}`,
		`{"kind":"view","view":"cost_per_run","text":"cost per run"}`,
		`{"kind":"query","query":"status.runs_by_state","slots":{"state":"running"},"text":"which runs are running?"}`,
		`{"kind":"view","view":"not_a_view","text":"?"}`,
	} {
		e.turn(t, "alice", sid, body)
		settle(t, e, "alice", sid)
	}
	if n := e.eventCount(t, "history.query_audited"); n != 0 {
		t.Fatalf("%d Layer-2 audit rows after four non-escalated turns, want 0", n)
	}

	// The non-tautological control: named explicitly, it goes. The stack is
	// unwired here, so what comes back is the honest UNAVAILABLE answer WITH
	// its audit — which is exactly the artifact the guardrail exists to produce.
	esc := e.turn(t, "alice", sid, `{"kind":"open_sql","text":"which runs cost the most?"}`)
	if esc.Turn.OutcomeKind != chat.OutcomeAnswer {
		t.Fatalf("an explicit escalation recorded %q: %s", esc.Turn.OutcomeKind, esc.Turn.Outcome)
	}
	a := decodeTurnAnswer(t, esc.Turn)
	if a.Layer != 2 {
		t.Fatalf("escalated answer layer = %d, want 2", a.Layer)
	}
	// G3 D3.5: the lower-confidence flag rides the answer wherever it renders.
	if a.Confidence != "lower-confidence" {
		t.Fatalf("Layer-2 answer confidence = %q, want lower-confidence carried VERBATIM", a.Confidence)
	}
	if a.Audit == nil {
		t.Fatal("a Layer-2 answer must carry its audit block")
	}
	if n := e.eventCount(t, "history.query_audited"); n != 1 {
		t.Errorf("the explicit escalation produced %d audit rows, want exactly 1", n)
	}
}

// TestNoDutyCallOutsideTheTitlingSeat is the checkable negative behind "no
// free-generation path exists": the transport names no duty alias and no model
// anywhere.
func TestNoDutyCallOutsideTheTitlingSeat(t *testing.T) {
	src, err := os.ReadFile("chatapi.go")
	if err != nil {
		t.Fatalf("read chatapi.go: %v", err)
	}
	for _, banned := range []string{"local.Alias", "DutyRequest", "internal/local", ".Call("} {
		if strings.Contains(string(src), banned) {
			t.Errorf("chatapi.go names %q — the assistant has no generation duty; the ONLY model call in this family is the titling seat in internal/chat/title.go", banned)
		}
	}
}

// ── The S06 intake handoff ──────────────────────────────────────────────────

// TestIntakeHandoffCallsTheLandedSubmit: the handoff rides the ONE landed
// ingress as the authenticated caller, and the born task is announced in the
// turn's own record.
func TestIntakeHandoffCallsTheLandedSubmit(t *testing.T) {
	e := newChatEnv(t)
	sid := e.session(t, "alice")
	got := e.turn(t, "alice", sid, `{"kind":"task","title":"Ship the report","text":"pull last month's numbers and write it up"}`)
	if got.Turn.OutcomeKind != chat.OutcomeTask {
		t.Fatalf("handoff recorded %q: %s", got.Turn.OutcomeKind, got.Turn.Outcome)
	}
	if e.fake.lastUser != "alice" {
		t.Errorf("Submit was called as %q, want the authenticated caller", e.fake.lastUser)
	}
	var born struct {
		TaskID string `json:"task_id"`
	}
	decodeInto(t, string(got.Turn.Outcome), &born)
	if born.TaskID == "" {
		t.Fatal("the born task's id must be announced in the turn's record (the deep link's target)")
	}
	// The settle event carries the task ref, so another tab learns which task
	// was born without re-reading the transcript.
	payload := lastPayload(t, e, chat.EventTurnSettled)
	if payload["task"] != born.TaskID {
		t.Errorf("chat.turn_settled payload task = %v, want %q", payload["task"], born.TaskID)
	}
	if payload["outcome"] != chat.OutcomeTask {
		t.Errorf("settle payload outcome = %v, want %q", payload["outcome"], chat.OutcomeTask)
	}
}

// TestHandedFilesBecomeRequesterSuppliedInputs drives the OQ8 shape end to end:
// upload → hand → Submit, with the descriptors on the body, and a cross-owner
// ref refused.
func TestHandedFilesBecomeRequesterSuppliedInputs(t *testing.T) {
	e := newChatEnv(t)
	sid := e.session(t, "alice")
	var up struct {
		File chat.File `json:"file"`
	}
	decodeInto(t, e.mustDo(t, "alice", "POST", "/api/chat/files?name=numbers.csv", "id,amount\n1,42\n"), &up)

	// Bob's own object, to prove the cross-owner refusal is about OWNERSHIP.
	var bobs struct {
		File chat.File `json:"file"`
	}
	decodeInto(t, e.mustDo(t, "bob", "POST", "/api/chat/files?name=bobs.csv", "x"), &bobs)

	body := fmt.Sprintf(`{"kind":"task","title":"Analyse","text":"analyse these","inputs":[%q]}`, up.File.ID)
	got := e.turn(t, "alice", sid, body)
	if got.Turn.OutcomeKind != chat.OutcomeTask {
		t.Fatalf("handoff with inputs = %q: %s", got.Turn.OutcomeKind, got.Turn.Outcome)
	}
	var submitted struct {
		Title  string `json:"title"`
		Text   string `json:"text"`
		Inputs []struct {
			Ref       string `json:"ref"`
			Name      string `json:"name"`
			SizeBytes int64  `json:"size_bytes"`
			SHA256    string `json:"sha256"`
		} `json:"inputs"`
	}
	decodeInto(t, string(e.fake.lastBody), &submitted)
	if len(submitted.Inputs) != 1 {
		t.Fatalf("Submit received %d inputs, want 1: %s", len(submitted.Inputs), e.fake.lastBody)
	}
	in := submitted.Inputs[0]
	if in.Ref != up.File.ID || in.Name != "numbers.csv" || in.SHA256 != up.File.SHA256 || in.SizeBytes != up.File.SizeBytes {
		t.Fatalf("the requester-supplied input descriptor = %+v, want the manifest row's identity", in)
	}

	// A ref to somebody ELSE's object is refused, and no task is born from it.
	settle(t, e, "alice", sid)
	before := e.fake.calls
	cross := e.turn(t, "alice", sid, fmt.Sprintf(`{"kind":"task","text":"steal","inputs":[%q]}`, bobs.File.ID))
	if cross.Turn.OutcomeKind != chat.OutcomeError {
		t.Fatalf("a cross-owner input ref recorded %q, want an error outcome", cross.Turn.OutcomeKind)
	}
	if e.fake.calls != before {
		t.Error("a cross-owner input ref reached Submit — it must be refused before the pipeline")
	}
}

// ── The hard stop ───────────────────────────────────────────────────────────

// TestHardStopIsAnExplicitVerbWithARecord, and it claims no authority over a
// task the chat handed off.
func TestHardStopIsAnExplicitVerbWithARecord(t *testing.T) {
	e := newChatEnv(t)
	sid := e.session(t, "alice")
	turn, _, err := e.store.BeginTurn(e.ctx, "alice", sid, chat.KindOpenSQL, "a long question")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	var stopped struct {
		Turn    chat.Turn `json:"turn"`
		Applied bool      `json:"applied"`
	}
	decodeInto(t, e.mustDo(t, "alice", "POST", "/api/chat/turns/"+turn.ID+"/stop", ""), &stopped)
	if !stopped.Applied || stopped.Turn.State != chat.StateAbandoned {
		t.Fatalf("stop = %+v", stopped)
	}
	if n := e.eventCount(t, chat.EventTurnAbandoned); n != 1 {
		t.Errorf("chat.turn_abandoned rows = %d, want 1", n)
	}
	// Retry-safe: the resolved state, not a refusal.
	var again struct {
		Applied bool `json:"applied"`
	}
	decodeInto(t, e.mustDo(t, "alice", "POST", "/api/chat/turns/"+turn.ID+"/stop", ""), &again)
	if again.Applied {
		t.Error("a repeated stop reported applied:true")
	}
	// Cross-owner stop is 404.
	if code, _ := e.do(t, "bob", "POST", "/api/chat/turns/"+turn.ID+"/stop", ""); code != http.StatusNotFound {
		t.Errorf("bob stopping alice's turn = %d, want 404", code)
	}
	// NO run or task was cancelled: chat claims no lifecycle authority.
	if n := e.eventCount(t, "run.state_changed"); n != 0 {
		t.Errorf("the stop verb moved %d run states — a chat stop is not a task cancel (4.5 stays the task's own verb)", n)
	}
}

// ── The client that goes away mid-turn ──────────────────────────────────────

// droppingIntake is the client-disconnect rig. Its Submit cancels the request
// driving the turn — which is what a phone losing its connection does to
// r.Context() — and then answers normally, so the ACT succeeded and only the
// listener is gone.
type droppingIntake struct {
	fakeSurface
	drop context.CancelFunc
}

func (d *droppingIntake) Submit(ctx context.Context, userID string, body json.RawMessage) (json.RawMessage, error) {
	d.drop()
	return d.fakeSurface.Submit(ctx, userID, body)
}

// TestClientDisconnectMidTurnStillSettlesTheTurn: chat is phone-complete
// (S1.10), so a dropped connection is the ORDINARY case, not an exotic one.
// The turn's RECORD must survive it — the §37-C WithoutCancel precedent, the
// same reason internal/history's audit append and internal/stage's session-end
// append outlive their contexts.
//
// The failure this pins is not cosmetic: a settle riding the request's context
// errors on cancellation, the turn stays `running`, no chat.turn_settled is
// minted, and every LATER turn in that session is refused ErrTurnRunning until
// somebody hard-stops it by hand. One dropped connection wedges the whole
// conversation.
func TestClientDisconnectMidTurnStillSettlesTheTurn(t *testing.T) {
	e := newChatEnv(t)
	sid := e.session(t, "alice")

	ctx, cancel := context.WithCancel(context.Background())
	rig := &droppingIntake{drop: cancel}
	e.intake = rig

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/chat/sessions/"+sid+"/turns",
		strings.NewReader(`{"kind":"task","title":"Ship the report","text":"pull the numbers"}`)).WithContext(ctx)
	e.server("alice").Handler().ServeHTTP(rr, req)

	// What the disconnected client received is beside the point; nobody is
	// listening. The RECORD is the whole of it.
	if rig.calls != 1 {
		t.Fatalf("the rig's Submit ran %d times — the disconnect did not happen mid-act", rig.calls)
	}
	if n := e.eventCount(t, chat.EventTurnSettled); n != 1 {
		t.Errorf("chat.turn_settled rows = %d after a disconnect, want 1 — the record of the ending must outlive the request", n)
	}
	turns, err := e.store.Turns(e.ctx, "alice", sid)
	if err != nil {
		t.Fatalf("read turns: %v", err)
	}
	if len(turns) != 1 || turns[0].State != chat.StateSettled {
		t.Fatalf("the turn after a disconnect = %+v, want one SETTLED turn", turns)
	}
	if turns[0].OutcomeKind != chat.OutcomeTask {
		t.Errorf("the settled outcome = %q, want the task the act actually produced", turns[0].OutcomeKind)
	}
	// And the session is not wedged: the next turn opens rather than being
	// refused behind a turn that never ended.
	if _, ok, err := e.store.RunningTurn(e.ctx, "alice", sid); err != nil || ok {
		t.Fatalf("a turn is still running after the disconnect (%v) — the session is wedged", err)
	}
	code, out := e.do(t, "alice", "POST", "/api/chat/sessions/"+sid+"/turns",
		`{"kind":"ask","text":"and what did that cost?"}`)
	if code != http.StatusOK {
		t.Fatalf("the next turn after a disconnect = %d: %s", code, out)
	}
}

// ── Every mutation lands on the log ─────────────────────────────────────────

// TestEveryMutationMintsItsEvent walks the whole family and counts.
func TestEveryMutationMintsItsEvent(t *testing.T) {
	e := newChatEnv(t)
	sid := e.session(t, "alice")
	e.mustDo(t, "alice", "POST", "/api/chat/sessions/"+sid+"/rename", `{"title":"Named by hand"}`)
	var up struct {
		File chat.File `json:"file"`
	}
	decodeInto(t, e.mustDo(t, "alice", "POST", "/api/chat/files?name=a.txt", "bytes"), &up)
	e.turn(t, "alice", sid, `{"kind":"view","view":"cost_per_run","text":"cost per run"}`)
	e.mustDo(t, "alice", "POST", "/api/chat/files/"+up.File.ID+"/delete", "")
	e.mustDo(t, "alice", "POST", "/api/chat/sessions/"+sid+"/delete", "")

	want := map[string]int{
		chat.EventSessionCreated: 1,
		chat.EventSessionRenamed: 1,
		chat.EventSessionDeleted: 1,
		chat.EventTurnStarted:    1,
		chat.EventTurnSettled:    1,
		chat.EventTurnAbandoned:  0,
		chat.EventFileUploaded:   1,
		chat.EventFileDeleted:    1,
	}
	for typ, n := range want {
		if got := e.eventCount(t, typ); got != n {
			t.Errorf("%s rows = %d, want %d", typ, got, n)
		}
	}
	// Every type is REGISTERED in the S14.2 contract (the §29 declare-once rule).
	for typ := range want {
		if _, ok := eventlog.Registry().TypeSpec(typ); !ok {
			t.Errorf("%q is minted but not registered in the event contract", typ)
		}
	}
	// Payloads are refs-not-blobs: the message body never rides the log.
	rows, err := e.b.db.QueryContext(e.ctx,
		`SELECT type, payload FROM run_events WHERE type LIKE 'chat.%'`)
	if err != nil {
		t.Fatalf("read chat events: %v", err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var typ, payload string
		if err := rows.Scan(&typ, &payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seen++
		if strings.Contains(payload, "cost per run") || strings.Contains(payload, "Named by hand") {
			t.Errorf("%s payload carries content, not refs: %s", typ, payload)
		}
	}
	if seen == 0 {
		t.Fatal("the refs-not-blobs scan read no rows — it would pass vacuously")
	}
}

func lastPayload(t *testing.T, e *chatEnv, typ string) map[string]any {
	t.Helper()
	var raw string
	if err := e.b.db.QueryRowContext(e.ctx,
		`SELECT payload FROM run_events WHERE type = ? ORDER BY event_seq DESC LIMIT 1`, typ).Scan(&raw); err != nil {
		t.Fatalf("read last %s payload: %v", typ, err)
	}
	out := map[string]any{}
	decodeInto(t, raw, &out)
	return out
}

// ── The serving edge ────────────────────────────────────────────────────────

// TestTurnOutcomeRedactsAtTheEdgeAndContentDoesNot is the §30 store-raw /
// serve-redacted probe with the CONTENT vs PROJECTION line drawn in one
// response: a secret planted in a run_events payload is redacted on its way out
// through a turn's answer, while the person's own message goes out verbatim.
func TestTurnOutcomeRedactsAtTheEdgeAndContentDoesNot(t *testing.T) {
	e := newChatEnv(t)
	const secret = "sk-ant-api03-PLANTEDSECRETVALUEFORTHISPROBE"
	if _, err := e.b.log.Append(e.ctx, eventlog.Append{
		UserID: "alice", Type: "local.unmetered_defect", SchemaVersion: 1,
		Payload: json.RawMessage(`{"run":"r-1","duty":"utility","cause":"` + secret + `"}`),
	}); err != nil {
		t.Fatalf("plant: %v", err)
	}
	sid := e.session(t, "alice")
	got := e.turn(t, "alice", sid,
		`{"kind":"query","query":"failures.unmetered_defects","text":"`+secret+`"}`)

	raw := e.mustDo(t, "alice", "GET", "/api/chat/sessions/"+sid, "")
	// The person's OWN words are content and go out as stored.
	if !strings.Contains(raw, secret) {
		t.Fatal("the person's own message was rewritten on the way out — a transcript is content, served verbatim")
	}
	// The ANSWER is a projection over run_events and must not carry it.
	var detail struct {
		Turns []chat.Turn `json:"turns"`
	}
	decodeInto(t, raw, &detail)
	if len(detail.Turns) != 1 {
		t.Fatalf("turns = %d", len(detail.Turns))
	}
	// EACH EDGE IS ASSERTED ON ITS OWN. A conjunction here would fire only when
	// BOTH edges leaked, so deleting the redaction from either one of them would
	// leave the probe green while that edge served the secret verbatim — a guard
	// that guards nothing. Both are driven separately, and both are revert-probed.
	if strings.Contains(string(detail.Turns[0].Outcome), secret) {
		t.Errorf("GET /api/chat/sessions/{id} served the planted secret unredacted in a turn outcome: %s",
			detail.Turns[0].Outcome)
	}
	if strings.Contains(string(got.Turn.Outcome), secret) {
		t.Errorf("POST .../turns served the planted secret unredacted in its own answer: %s",
			got.Turn.Outcome)
	}
	// The outcome the probe is reading must be the ANSWER — an error outcome
	// would carry no rows and redact nothing, and the assertions above would
	// pass on emptiness rather than on redaction.
	if got.Turn.OutcomeKind != chat.OutcomeAnswer || len(decodeTurnAnswer(t, got.Turn).Rows) == 0 {
		t.Fatalf("the probe needs a populated answer to redact, got %q: %s",
			got.Turn.OutcomeKind, got.Turn.Outcome)
	}
	// The STORED row is untouched — redaction happens at the edge, not in the DB.
	var stored string
	if err := e.b.db.QueryRowContext(e.ctx,
		`SELECT payload FROM run_events WHERE type = 'local.unmetered_defect' LIMIT 1`).Scan(&stored); err != nil {
		t.Fatalf("read stored: %v", err)
	}
	if !strings.Contains(stored, secret) {
		t.Error("the stored payload was modified — store-raw / serve-redacted (§30)")
	}
}

// ── The fixture handoff body vs the REAL task view ──────────────────────────

// structJSONShape reads a struct's json field list out of Go SOURCE: every
// field's tag name, and which of them are non-omitempty (i.e. always emitted).
func structJSONShape(t *testing.T, file, structName string) (known map[string]bool, always []string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	known = map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != structName {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range st.Fields.List {
			if field.Tag == nil {
				continue
			}
			tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`")).Get("json")
			parts := strings.Split(tag, ",")
			if parts[0] == "" || parts[0] == "-" {
				continue
			}
			known[parts[0]] = true
			if !slices.Contains(parts[1:], "omitempty") {
				always = append(always, parts[0])
			}
		}
		return false
	})
	if len(known) == 0 {
		t.Fatalf("no json fields found on %s in %s — the shape read is empty", structName, file)
	}
	return known, always
}

// TestFixtureHandoffMatchesTheRealTaskView ties the S06 handoff fixture to the
// shape production actually serves.
//
// The chat feed renders the born task's intake card INLINE from this body
// (OQ8(i)), so a fixture that drifts from stage.taskView teaches the widget a
// payload that does not exist — precisely the B6-5 root cause, and nothing was
// asserting against it. internal/api cannot import the pipeline's view type (the
// IntakeSurface seam exists so it never speaks that vocabulary), so the shape is
// read out of internal/stage's SOURCE, where it is written: a rename, a dropped
// field or a new omitempty there fails here rather than silently.
func TestFixtureHandoffMatchesTheRealTaskView(t *testing.T) {
	const realSource = "../stage/surface.go"
	body, err := fixtureIntake{t: t}.Submit(context.Background(), "alice",
		json.RawMessage(`{"title":"Draft the release notes"}`))
	if err != nil {
		t.Fatalf("fixture Submit: %v", err)
	}
	var got map[string]json.RawMessage
	decodeInto(t, string(body), &got)

	known, always := structJSONShape(t, realSource, "taskView")
	for key := range got {
		if !known[key] {
			t.Errorf("the fixture handoff emits %q, which stage.taskView does not have — an imagined key serves a world that does not exist", key)
		}
	}
	for _, key := range always {
		if _, ok := got[key]; !ok {
			t.Errorf("the fixture handoff omits %q, which stage.taskView emits on EVERY task view", key)
		}
	}

	var runs []map[string]json.RawMessage
	decodeInto(t, string(got["runs"]), &runs)
	if len(runs) == 0 {
		t.Fatal("the fixture handoff carries no runs — the nested shape below would be checked against nothing")
	}
	runKnown, runAlways := structJSONShape(t, realSource, "runSummary")
	for _, run := range runs {
		for key := range run {
			if !runKnown[key] {
				t.Errorf("a fixture run summary emits %q, which stage.runSummary does not have", key)
			}
		}
		for _, key := range runAlways {
			if _, ok := run[key]; !ok {
				t.Errorf("a fixture run summary omits %q, which stage.runSummary emits on EVERY run", key)
			}
		}
	}

	// clearance is omitempty on the real view, so the shape check above cannot
	// require it — but the real view DOES carry it from intake state, and the
	// born card this same handoff serves declares its own. One task, one number.
	raw, ok := got["clearance"]
	if !ok {
		t.Fatal("the fixture handoff carries no clearance — stage.taskView reads one off intake state for every task that has one")
	}
	var clearance float64
	decodeInto(t, string(raw), &clearance)
	var card struct {
		Clearance float64 `json:"clearance"`
	}
	decodeInto(t, string(got["open_card"]), &card)
	if clearance == 0 || clearance != card.Clearance {
		t.Errorf("the handoff view's clearance = %v but its own open card declares %v", clearance, card.Clearance)
	}
}

// TestChatFixtureDrivesBothProducedRenders pins what drain r1b added: the
// committed session body carries BOTH produced-files states, so neither render
// is undriven. Exactly one settled turn has a non-empty list — its file arrived
// through the real upload handler while that turn was in flight, so the window
// diff attributes it honestly — and the others are empty, which at v0 is the
// common case (uploads are the only producer, OQ7). Compare-only fixtures catch
// a CHANGE; this states the property the fixture exists to have.
func TestChatFixtureDrivesBothProducedRenders(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(fixtureDir, "chat-session.json"))
	if err != nil {
		t.Fatalf("read the committed session body: %v", err)
	}
	var body struct {
		Turns []chat.Turn `json:"turns"`
	}
	decodeInto(t, string(raw), &body)
	var withFiles, settledEmpty int
	for _, turn := range body.Turns {
		if turn.State != chat.StateSettled {
			continue
		}
		if len(turn.Produced) > 0 {
			withFiles++
		} else {
			settledEmpty++
		}
	}
	if withFiles != 1 {
		t.Errorf("%d settled turns carry produced files, want exactly 1 — the chip render needs a committed body", withFiles)
	}
	if settledEmpty == 0 {
		t.Error("no settled turn has an empty produced list — the honest-sparse render needs a committed body too")
	}
	// The chip must name a file the sidebar actually serves; a chip pointing at
	// nothing is the fabrication this whole mechanism was fixed to prevent.
	files, err := os.ReadFile(filepath.Join(fixtureDir, "chat-files.json"))
	if err != nil {
		t.Fatalf("read the committed file list: %v", err)
	}
	var listed struct {
		Files []chat.File `json:"files"`
	}
	decodeInto(t, string(files), &listed)
	for _, turn := range body.Turns {
		for _, id := range turn.Produced {
			found := false
			for _, f := range listed.Files {
				found = found || f.ID == id
			}
			if !found {
				t.Errorf("turn %s claims produced file %s, which the served file list does not carry", turn.ID, id)
			}
		}
	}
}

// ── Route-table negatives ───────────────────────────────────────────────────

// TestChatRouteTableNegatives: messages are IMMUTABLE, so there is no edit verb
// and no message delete; and no REST verb outside the dispositioned family
// answers.
func TestChatRouteTableNegatives(t *testing.T) {
	e := newChatEnv(t)
	sid := e.session(t, "alice")
	got := e.turn(t, "alice", sid, `{"kind":"ask","text":"hello"}`)
	mid := got.Message.ID

	for _, probe := range []struct{ method, path string }{
		{"POST", "/api/chat/messages/" + mid + "/edit"},
		{"POST", "/api/chat/messages/" + mid + "/delete"},
		{"PUT", "/api/chat/messages/" + mid},
		{"DELETE", "/api/chat/messages/" + mid},
		{"PATCH", "/api/chat/sessions/" + sid},
		{"DELETE", "/api/chat/sessions/" + sid},
		{"PUT", "/api/chat/sessions/" + sid},
		{"POST", "/api/chat/sessions/" + sid + "/archive"},
		{"POST", "/api/chat/generate"},
		{"POST", "/api/chat/complete"},
	} {
		code, _ := e.do(t, "alice", probe.method, probe.path, "{}")
		if code == http.StatusOK {
			t.Errorf("%s %s answered 200 — it is not a verb of this family", probe.method, probe.path)
		}
	}
	// The non-tautological control: the real routes DO answer.
	e.mustDo(t, "alice", "GET", "/api/chat/sessions/"+sid, "")
	e.mustDo(t, "alice", "GET", "/api/chat/files", "")
}

// TestChatFamilyHasNoOutwardEffectVerb reads the route table itself: every chat
// route is a read or a control-plane-internal state act (S15.2; D7).
func TestChatFamilyHasNoOutwardEffectVerb(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "api.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse api.go: %v", err)
	}
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read api.go: %v", err)
	}
	_ = f
	var chatRoutes []string
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, `protected("`) || !strings.Contains(line, "/api/chat") {
			continue
		}
		chatRoutes = append(chatRoutes, line)
	}
	if len(chatRoutes) != 10 {
		t.Fatalf("found %d chat routes, want the 10 dispositioned ones:\n%s",
			len(chatRoutes), strings.Join(chatRoutes, "\n"))
	}
	// The outward vocabulary, by name. Nothing in this family sends, publishes
	// or pushes; the born task's own outward effects stay D7-gated downstream.
	for _, r := range chatRoutes {
		for _, banned := range []string{"/send", "/publish", "/push", "/email", "/post-to", "/execute"} {
			if strings.Contains(r, banned) {
				t.Errorf("chat route %q looks like a direct outward effect (S15.2: none exists anywhere on this API)", r)
			}
		}
	}
}

// TestChatRoutesAnswer503WhenUnwired: an unwired family says so rather than
// pretending a store is open.
func TestChatRoutesAnswer503WhenUnwired(t *testing.T) {
	b := newBackend(t)
	srv := api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{"alice"},
		Settings: fixedSettings{d: 20 * time.Second},
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       b.db,
	})
	for _, probe := range []struct{ method, path string }{
		{"GET", "/api/chat/sessions"},
		{"POST", "/api/chat/sessions"},
		{"GET", "/api/chat/files"},
	} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest(probe.method, probe.path, nil))
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s with no chat store = %d, want 503", probe.method, probe.path, rr.Code)
		}
	}
}

// TestChatSurfaceBuildsNoSQLAndReImplementsNoLayer is the R3 source scan: the
// transport reaches the landed verbs and nothing else.
func TestChatSurfaceBuildsNoSQLAndReImplementsNoLayer(t *testing.T) {
	src, err := os.ReadFile("chatapi.go")
	if err != nil {
		t.Fatalf("read chatapi.go: %v", err)
	}
	body := stripComments(t, string(src))
	for _, banned := range []string{"SELECT ", "INSERT ", "UPDATE ", "DELETE ", "QueryContext", "QueryRowContext", "WriteTx"} {
		if strings.Contains(body, banned) {
			t.Errorf("chatapi.go contains %q — it maps a request onto a landed verb and constructs no SQL of its own", banned)
		}
	}
	for _, banned := range []string{"deterministic", "lower-confidence", "newAnswer"} {
		if strings.Contains(body, banned) {
			t.Errorf("chatapi.go names %q — confidence is derived by internal/history's one constructor and never re-derived here", banned)
		}
	}
	// The non-tautological control: it DOES call the landed layers.
	for _, want := range []string{"s.history.Ask(", "s.history.SelectView(", "s.history.RunQuery(", "s.history.AskOpenSQL(", "s.intake.Submit("} {
		if !strings.Contains(body, want) {
			t.Errorf("chatapi.go does not call %s — the scan above would pass by the file doing nothing", want)
		}
	}
}

// stripComments removes comments so a doc comment naming a construct the file
// must not contain cannot fail its own scan (the historyapi.go precedent).
func stripComments(t *testing.T, src string) string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "chatapi.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var b strings.Builder
	for _, d := range f.Decls {
		start := fset.Position(d.Pos()).Offset
		end := fset.Position(d.End()).Offset
		b.WriteString(src[start:end])
		b.WriteString("\n")
	}
	return b.String()
}

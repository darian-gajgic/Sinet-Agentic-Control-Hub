package chat_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/chat"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// chat_test.go — the store half of the S15.7 assistant (P3-B6-7 part A).
//
// $0 AND HERMETIC. The one duty leg runs against local.FakeServer — an httptest
// double of the llama-swap surface — so the titling battery exercises the REAL
// local.Duty, the REAL alias resolution through ⚙ local.alias, and the REAL D7
// metering write, while dialing nothing and pricing nothing. Every other test
// composes the store with no duty at all, which is also the honest production
// posture when no local stack is configured.

type env struct {
	t     *testing.T
	ctx   context.Context
	db    *storage.DB
	log   *eventlog.Log
	reg   *settings.Registry
	root  string
	store *chat.Store
}

func newEnv(t *testing.T) *env {
	t.Helper()
	return newEnvWithDuty(t, nil, nil)
}

func newEnvWithDuty(t *testing.T, duty *local.Duty, advisory chat.AdvisoryRun) *env {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	root := filepath.Join(t.TempDir(), "exchange")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("mkdir exchange: %v", err)
	}
	st, err := chat.New(chat.Config{DB: db, Log: log, Root: root, Duty: duty, Advisory: advisory})
	if err != nil {
		t.Fatalf("chat.New: %v", err)
	}
	return &env{t: t, ctx: ctx, db: db, log: log, reg: reg, root: root, store: st}
}

func (e *env) eventCount(t *testing.T, typ string) int {
	t.Helper()
	var n int
	if err := e.db.QueryRowContext(e.ctx,
		`SELECT count(*) FROM run_events WHERE type = ?`, typ).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", typ, err)
	}
	return n
}

// ── Sessions: owner-only in BOTH directions ─────────────────────────────────

// TestSessionsAreOwnerOnly pins the OQ1 visibility deviation structurally. The
// owner's own rows must be PRESENT — an all-empty store would otherwise pass a
// scoping test by doing nothing — and the other person's must be ABSENT on
// every read and every mutation.
func TestSessionsAreOwnerOnly(t *testing.T) {
	e := newEnv(t)
	alice, err := e.store.CreateSession(e.ctx, "alice")
	if err != nil {
		t.Fatalf("create alice session: %v", err)
	}
	if _, err := e.store.CreateSession(e.ctx, "bob"); err != nil {
		t.Fatalf("create bob session: %v", err)
	}

	mine, err := e.store.ListSessions(e.ctx, "alice")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(mine) != 1 || mine[0].ID != alice.ID {
		t.Fatalf("alice must see exactly her own session, got %+v", mine)
	}
	// The operator has no limb here AT ALL — the store takes a viewer and has
	// no role parameter — so an operator identity is simply another viewer with
	// no sessions of their own.
	if op, err := e.store.ListSessions(e.ctx, "op"); err != nil || len(op) != 0 {
		t.Fatalf("the operator must not read a member's sessions: %v %+v", err, op)
	}
	for _, who := range []string{"bob", "op", ""} {
		if _, err := e.store.Session(e.ctx, who, alice.ID); err != chat.ErrNotFound {
			t.Errorf("Session(%q) on alice's session = %v, want ErrNotFound", who, err)
		}
		if _, err := e.store.RenameSession(e.ctx, who, alice.ID, "hijack"); err != chat.ErrNotFound {
			t.Errorf("RenameSession(%q) = %v, want ErrNotFound", who, err)
		}
		if _, err := e.store.Messages(e.ctx, who, alice.ID); err != chat.ErrNotFound {
			t.Errorf("Messages(%q) = %v, want ErrNotFound", who, err)
		}
	}
	// A delete by a non-owner must not remove the row: applied:false, and the
	// session is still there afterwards.
	res, err := e.store.DeleteSession(e.ctx, "bob", alice.ID)
	if err != nil || res.Applied {
		t.Fatalf("bob deleting alice's session = %+v %v, want applied:false", res, err)
	}
	if _, err := e.store.Session(e.ctx, "alice", alice.ID); err != nil {
		t.Fatalf("alice's session must survive bob's delete: %v", err)
	}
}

// TestStoreHasNoOperatorLimbAtAll is the STRUCTURAL half of the owner-only
// deviation, and it is what makes `"op"` above mean something: in this package
// the operator is not a refused role, it is a viewer like any other, because
// there is no role to carry. Every exported method on *Store takes the acting
// identity as a plain string and nothing else — no role, no scope, no operator
// bit — so there is no limb anywhere here that a future edit could forget to
// omit. The HTTP half, where a REAL operator-role user is registered and the
// refusal is behavioural, lives in internal/api/chatapi_test.go.
func TestStoreHasNoOperatorLimbAtAll(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	banned := []string{"operator", "role", "scope", "admin"}
	methods := 0
	for _, ent := range entries {
		name := ent.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !fn.Name.IsExported() {
				continue
			}
			star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			if id, ok := star.X.(*ast.Ident); !ok || id.Name != "Store" {
				continue
			}
			methods++
			for _, param := range fn.Type.Params.List {
				var names []string
				for _, n := range param.Names {
					names = append(names, n.Name)
				}
				if id, ok := param.Type.(*ast.Ident); ok && id.Name == "bool" {
					t.Errorf("%s.%s takes a bool %v — a role bit is the only thing this package would want one for, and it must not have one",
						name, fn.Name.Name, names)
				}
				for _, n := range names {
					for _, b := range banned {
						if strings.Contains(strings.ToLower(n), b) {
							t.Errorf("%s.%s takes a parameter named %q — the store takes a VIEWER and knows nothing about roles",
								name, fn.Name.Name, n)
						}
					}
				}
			}
		}
	}
	if methods < 15 {
		t.Fatalf("the scan found %d exported *Store methods — it is not reading the package", methods)
	}
}

// TestSessionUntitledThenNamed drives the honest untitled state and the human
// override, and asserts a rename mints its event.
func TestSessionUntitledThenNamed(t *testing.T) {
	e := newEnv(t)
	s, _ := e.store.CreateSession(e.ctx, "alice")
	if s.Title != "" || s.TitleSource != chat.TitleUnset {
		t.Fatalf("a fresh session must be honestly untitled, got %q/%q", s.Title, s.TitleSource)
	}
	got, err := e.store.RenameSession(e.ctx, "alice", s.ID, "  Budget questions  ")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got.Title != "Budget questions" || got.TitleSource != chat.TitleHuman {
		t.Fatalf("rename = %q/%q", got.Title, got.TitleSource)
	}
	if n := e.eventCount(t, chat.EventSessionRenamed); n != 1 {
		t.Errorf("chat.session_renamed rows = %d, want 1", n)
	}
	if _, err := e.store.RenameSession(e.ctx, "alice", s.ID, strings.Repeat("x", chat.MaxTitleRunes+1)); err == nil {
		t.Error("an over-long title must be refused")
	}
	if _, err := e.store.RenameSession(e.ctx, "alice", s.ID, "   "); err == nil {
		t.Error("a blank title must be refused")
	}
}

// TestDeleteIsHardAndItsRecordSurvives: the rows go, the audit stays.
func TestDeleteIsHardAndItsRecordSurvives(t *testing.T) {
	e := newEnv(t)
	s, _ := e.store.CreateSession(e.ctx, "alice")
	if _, _, err := e.store.BeginTurn(e.ctx, "alice", s.ID, chat.KindAsk, "what is running?"); err != nil {
		t.Fatalf("begin turn: %v", err)
	}
	res, err := e.store.DeleteSession(e.ctx, "alice", s.ID)
	if err != nil || !res.Applied || res.Messages != 1 || res.Turns != 1 {
		t.Fatalf("delete = %+v %v", res, err)
	}
	for _, table := range []string{"chat_sessions", "chat_messages", "chat_turns"} {
		var n int
		if err := e.db.QueryRowContext(e.ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s still holds %d rows after a hard delete", table, n)
		}
	}
	// The lifecycle events are the audit and they OUTLIVE the rows.
	for _, typ := range []string{chat.EventSessionCreated, chat.EventTurnStarted, chat.EventSessionDeleted} {
		if n := e.eventCount(t, typ); n != 1 {
			t.Errorf("%s rows = %d, want 1 (the audit must survive the delete)", typ, n)
		}
	}
	// A repeat is retry-safe: the already-resolved state, not a refusal.
	again, err := e.store.DeleteSession(e.ctx, "alice", s.ID)
	if err != nil || again.Applied {
		t.Fatalf("repeat delete = %+v %v, want applied:false with no error", again, err)
	}
	if n := e.eventCount(t, chat.EventSessionDeleted); n != 1 {
		t.Errorf("a repeat delete must mint nothing more, got %d rows", n)
	}
}

// TestMessagesAreImmutable proves the migration's trigger, not a convention.
func TestMessagesAreImmutable(t *testing.T) {
	e := newEnv(t)
	s, _ := e.store.CreateSession(e.ctx, "alice")
	_, m, err := e.store.BeginTurn(e.ctx, "alice", s.ID, chat.KindAsk, "original words")
	if err != nil {
		t.Fatalf("begin turn: %v", err)
	}
	err = e.db.WriteTx(e.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(e.ctx, `UPDATE chat_messages SET body = 'rewritten' WHERE message_id = ?`, m.ID)
		return err
	})
	if err == nil {
		t.Fatal("a chat message must not be editable — the 0020 trigger did not fire")
	}
	msgs, err := e.store.Messages(e.ctx, "alice", s.ID)
	if err != nil || len(msgs) != 1 || msgs[0].Body != "original words" {
		t.Fatalf("transcript = %+v %v", msgs, err)
	}
}

// ── Turns ───────────────────────────────────────────────────────────────────

// TestOneTurnAtATimeAndItSurvivesReRead is the survives-navigation property at
// the store level: a running turn is a ROW, so re-reading finds it.
func TestOneTurnAtATimeAndItSurvivesReRead(t *testing.T) {
	e := newEnv(t)
	s, _ := e.store.CreateSession(e.ctx, "alice")
	turn, _, err := e.store.BeginTurn(e.ctx, "alice", s.ID, chat.KindAsk, "what is running?")
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, _, err := e.store.BeginTurn(e.ctx, "alice", s.ID, chat.KindAsk, "and again?"); err != chat.ErrTurnRunning {
		t.Fatalf("a second concurrent turn = %v, want ErrTurnRunning", err)
	}
	running, ok, err := e.store.RunningTurn(e.ctx, "alice", s.ID)
	if err != nil || !ok || running.ID != turn.ID {
		t.Fatalf("the in-flight turn must be re-readable: %+v %v %v", running, ok, err)
	}
	settled, err := e.store.SettleTurn(e.ctx, "alice", turn.ID, chat.OutcomeAnswer, json.RawMessage(`{"layer":0,"query":"cost_per_run"}`))
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	if settled.State != chat.StateSettled || settled.SettledTS == "" {
		t.Fatalf("settled turn = %+v", settled)
	}
	if _, ok, _ := e.store.RunningTurn(e.ctx, "alice", s.ID); ok {
		t.Error("no turn should be running after a settle")
	}
	// A second settle is retry-safe.
	if _, err := e.store.SettleTurn(e.ctx, "alice", turn.ID, chat.OutcomeAnswer, json.RawMessage(`{}`)); err != chat.ErrTurnResolved {
		t.Fatalf("repeat settle = %v, want ErrTurnResolved", err)
	}
	if n := e.eventCount(t, chat.EventTurnSettled); n != 1 {
		t.Errorf("chat.turn_settled rows = %d, want 1", n)
	}
}

// TestHardStopIsAnActAndTheRecordStands: the stop wins, and a result arriving
// afterwards does NOT overwrite the abandoned record (OQ6).
func TestHardStopIsAnActAndTheRecordStands(t *testing.T) {
	e := newEnv(t)
	s, _ := e.store.CreateSession(e.ctx, "alice")
	turn, _, _ := e.store.BeginTurn(e.ctx, "alice", s.ID, chat.KindOpenSQL, "select everything")

	stopped, err := e.store.AbandonTurn(e.ctx, "alice", turn.ID)
	if err != nil || stopped.State != chat.StateAbandoned {
		t.Fatalf("abandon = %+v %v", stopped, err)
	}
	if n := e.eventCount(t, chat.EventTurnAbandoned); n != 1 {
		t.Errorf("chat.turn_abandoned rows = %d, want 1 — a stop is an act and has a record", n)
	}
	// The completed-anyway result finds an abandoned row and leaves it alone.
	late, err := e.store.SettleTurn(e.ctx, "alice", turn.ID, chat.OutcomeAnswer, json.RawMessage(`{"layer":2}`))
	if err != chat.ErrTurnResolved {
		t.Fatalf("late settle = %v, want ErrTurnResolved", err)
	}
	if late.State != chat.StateAbandoned || late.OutcomeKind != "" {
		t.Fatalf("the abandoned record must stand, got %+v", late)
	}
	if n := e.eventCount(t, chat.EventTurnSettled); n != 0 {
		t.Errorf("a stopped turn must mint no settle, got %d rows", n)
	}
	// A repeat stop is retry-safe.
	if _, err := e.store.AbandonTurn(e.ctx, "alice", turn.ID); err != chat.ErrTurnResolved {
		t.Errorf("repeat stop = %v, want ErrTurnResolved", err)
	}
	// A stranger cannot stop somebody else's turn.
	if _, err := e.store.AbandonTurn(e.ctx, "bob", turn.ID); err != chat.ErrNotFound {
		t.Errorf("cross-owner stop = %v, want ErrNotFound", err)
	}
}

// TestUnknownTurnKindIsRefused: the vocabulary is closed on the Go side as well
// as in the CHECK.
func TestUnknownTurnKindIsRefused(t *testing.T) {
	e := newEnv(t)
	s, _ := e.store.CreateSession(e.ctx, "alice")
	if _, _, err := e.store.BeginTurn(e.ctx, "alice", s.ID, "freeform", "write me a poem"); err == nil {
		t.Fatal("a kind outside the closed set must be refused")
	}
	if n := e.eventCount(t, chat.EventTurnStarted); n != 0 {
		t.Errorf("a refused turn must mint nothing, got %d rows", n)
	}
	if _, _, err := e.store.BeginTurn(e.ctx, "alice", s.ID, chat.KindAsk,
		strings.Repeat("x", chat.MaxMessageRunes+1)); err == nil {
		t.Error("an over-long message must be refused")
	}
}

// ── Auto-titling (the one duty leg) ─────────────────────────────────────────

// fakeDuty builds a real local.Duty over the llama-swap fake plus an advisory
// run opened through the real run store — the exact rig every duty leg in this
// tree uses. $0: nothing is priced and nothing is dialed beyond the httptest
// double.
func fakeDuty(t *testing.T, e *env) (*local.Duty, chat.AdvisoryRun, *local.FakeServer) {
	t.Helper()
	fake := local.NewFakeServer()
	t.Cleanup(fake.Close)
	if err := e.reg.Attach(e.ctx, e.db, e.log); err != nil {
		t.Fatalf("settings Attach: %v", err)
	}
	duty := local.NewDuty(local.DutyDeps{
		Registry:    local.NewRegistry(e.reg),
		Client:      local.NewClient(fake.URL),
		Checkpoints: gates.NewCheckpoints(e.db, e.log),
		Events:      e.log,
	})
	runs := run.NewStore(e.db, e.log)
	advisory := func(ctx context.Context, label string) (string, func()) {
		id := "platform.advisory." + label + "." + strconv.FormatInt(time.Now().UnixNano(), 10)
		if _, err := runs.Create(ctx, run.NewRun{
			ID: id, UserID: run.ActorPlatform, Lane: local.LaneLocal, Substrate: "local",
		}); err != nil {
			return "", nil
		}
		for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
			if _, err := runs.Transition(ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
				return "", nil
			}
		}
		return id, func() {
			_, _ = runs.Transition(context.WithoutCancel(ctx), id, run.StateCompleted,
				run.TransitionOptions{Actor: run.ActorPlatform})
		}
	}
	return duty, advisory, fake
}

// TestAutoTitlingRidesTheUtilitySeat drives the whole OQ2 disposition: ONE call,
// on the EXISTING `utility` alias, shaped as free-text drafting (no schema, no
// logprobs), metered with one $0 D7 checkpoint on the advisory run.
func TestAutoTitlingRidesTheUtilitySeat(t *testing.T) {
	base := newEnv(t)
	duty, advisory, fake := fakeDuty(t, base)
	seat, err := duty.ResolveSeat(local.AliasUtility)
	if err != nil {
		t.Fatalf("resolve the %s seat: %v", local.AliasUtility, err)
	}
	if !seat.Servable {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10): the %s seat is not servable in this manifest (%s)",
			local.AliasUtility, seat.Note)
	}
	fake.SetResponse(local.FakeResponse{Content: "  \"Quarterly budget review.\"  ", InputTokens: 30, OutputTokens: 6})

	st, err := chat.New(chat.Config{DB: base.db, Log: base.log, Root: base.root, Duty: duty, Advisory: advisory})
	if err != nil {
		t.Fatalf("chat.New: %v", err)
	}
	if !st.TitleSeatWired() {
		t.Fatal("the titling seat must report itself wired when duty + advisory are present")
	}
	s, _ := st.CreateSession(base.ctx, "alice")
	if _, _, err := st.BeginTurn(base.ctx, "alice", s.ID, chat.KindAsk, "How much did we spend last month?"); err != nil {
		t.Fatalf("begin turn: %v", err)
	}
	named, err := st.Session(base.ctx, "alice", s.ID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if named.TitleSource != chat.TitleDuty || named.Title != "Quarterly budget review" {
		t.Fatalf("auto-title = %q/%q, want the cleaned drafted line from the duty", named.Title, named.TitleSource)
	}

	// The request SHAPE is the disposition: the utility seat's model, free-text
	// drafting — no json_schema, no logprobs.
	req := fake.LastRequest()
	if req.Model != seat.Model {
		t.Errorf("titling called model %q, want the %s seat's %q", req.Model, local.AliasUtility, seat.Model)
	}
	if req.HasJSONSchema {
		t.Error("auto-titling is free-text DRAFTING: it must send no json_schema")
	}
	if req.Logprobs {
		t.Error("auto-titling must not request logprobs — utility is honestly uncalibrated, so there is no margin to gate on")
	}
	if req.MaxTokens <= 0 {
		t.Error("the drafting call must carry a length cap")
	}

	// ONE $0 D7 checkpoint, on the advisory run (S12.1 R18).
	var cps int
	var substrate string
	if err := base.db.QueryRowContext(base.ctx,
		`SELECT count(*), coalesce(max(session_substrate), '') FROM checkpoints`).Scan(&cps, &substrate); err != nil {
		t.Fatalf("count checkpoints: %v", err)
	}
	if cps != 1 || substrate != "local" {
		t.Fatalf("checkpoints = %d substrate=%q, want exactly one local-substrate D7 row", cps, substrate)
	}

	// A SECOND turn does not re-title: only the first message names a session.
	if _, err := st.SettleTurn(base.ctx, "alice", mustRunningID(t, st, base.ctx, s.ID), chat.OutcomeAnswer, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("settle: %v", err)
	}
	fake.SetResponse(local.FakeResponse{Content: "A completely different title", InputTokens: 3, OutputTokens: 3})
	if _, _, err := st.BeginTurn(base.ctx, "alice", s.ID, chat.KindAsk, "and this month?"); err != nil {
		t.Fatalf("second turn: %v", err)
	}
	again, _ := st.Session(base.ctx, "alice", s.ID)
	if again.Title != named.Title {
		t.Errorf("a second turn re-titled the session: %q → %q", named.Title, again.Title)
	}
}

func mustRunningID(t *testing.T, st *chat.Store, ctx context.Context, sessionID string) string {
	t.Helper()
	turn, ok, err := st.RunningTurn(ctx, "alice", sessionID)
	if err != nil || !ok {
		t.Fatalf("no running turn: %v %v", ok, err)
	}
	return turn.ID
}

// TestAutoTitlingDegradesMechanically: with no local stack the session is still
// honestly named, from the person's OWN words, and the turn is unblocked.
func TestAutoTitlingDegradesMechanically(t *testing.T) {
	e := newEnv(t) // no duty, no advisory — the ErrStackAbsent posture
	if e.store.TitleSeatWired() {
		t.Fatal("an unwired store must not claim a titling seat")
	}
	s, _ := e.store.CreateSession(e.ctx, "alice")
	turn, _, err := e.store.BeginTurn(e.ctx, "alice", s.ID, chat.KindAsk, "How much did we spend last month?")
	if err != nil {
		t.Fatalf("an absent local stack must never block a turn: %v", err)
	}
	if turn.State != chat.StateRunning {
		t.Fatalf("turn state = %q", turn.State)
	}
	named, _ := e.store.Session(e.ctx, "alice", s.ID)
	if named.TitleSource != chat.TitleMechanical {
		t.Fatalf("title source = %q, want mechanical", named.TitleSource)
	}
	if named.Title != "How much did we spend last month?" {
		t.Fatalf("the mechanical title must derive from the person's own words, got %q", named.Title)
	}
	// It is a DERIVATION, so a long message truncates rather than being invented.
	s2, _ := e.store.CreateSession(e.ctx, "alice")
	long := strings.Repeat("word ", 60)
	if _, _, err := e.store.BeginTurn(e.ctx, "alice", s2.ID, chat.KindAsk, long); err != nil {
		t.Fatalf("begin: %v", err)
	}
	n2, _ := e.store.Session(e.ctx, "alice", s2.ID)
	if !strings.HasPrefix(n2.Title, "word word") || !strings.HasSuffix(n2.Title, "…") {
		t.Fatalf("long mechanical title = %q, want a truncation of the message", n2.Title)
	}
	if len([]rune(n2.Title)) > chat.MaxTitleRunes+1 {
		t.Fatalf("mechanical title is %d runes, over the bound", len([]rune(n2.Title)))
	}
}

// TestHumanRenameIsNeverOverwritten: the override outranks the duty
// permanently, and the guarantee is the UPDATE's own predicate.
func TestHumanRenameIsNeverOverwritten(t *testing.T) {
	base := newEnv(t)
	duty, advisory, fake := fakeDuty(t, base)
	if seat, err := duty.ResolveSeat(local.AliasUtility); err != nil || !seat.Servable {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10): the %s seat is not servable here", local.AliasUtility)
	}
	fake.SetResponse(local.FakeResponse{Content: "Model chosen title", InputTokens: 5, OutputTokens: 3})
	st, err := chat.New(chat.Config{DB: base.db, Log: base.log, Root: base.root, Duty: duty, Advisory: advisory})
	if err != nil {
		t.Fatalf("chat.New: %v", err)
	}
	s, _ := st.CreateSession(base.ctx, "alice")
	if _, err := st.RenameSession(base.ctx, "alice", s.ID, "My own name for this"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, _, err := st.BeginTurn(base.ctx, "alice", s.ID, chat.KindAsk, "first message"); err != nil {
		t.Fatalf("begin: %v", err)
	}
	got, _ := st.Session(base.ctx, "alice", s.ID)
	if got.Title != "My own name for this" || got.TitleSource != chat.TitleHuman {
		t.Fatalf("the duty overwrote a human title: %q/%q", got.Title, got.TitleSource)
	}
}

// TestTheTenAliasRowsAreUnchanged is the checkable negative behind "no new ⚙
// key and no new duty alias": the registry's declared map still carries exactly
// the ten S12.4 rows, and `utility` is the one this packet consumes.
func TestTheTenAliasRowsAreUnchanged(t *testing.T) {
	reg := settings.New()
	m, err := reg.StringMap("local.alias")
	if err != nil {
		t.Fatalf("read ⚙ local.alias: %v", err)
	}
	if len(m) != 10 {
		t.Fatalf("⚙ local.alias holds %d rows, want the ten S12.4 aliases (a chat-titling row would be an S00.9 amendment, not an executor's act)", len(m))
	}
	if _, ok := m[local.AliasUtility]; !ok {
		t.Fatalf("the %s seat this packet rides is missing from the registry", local.AliasUtility)
	}
}

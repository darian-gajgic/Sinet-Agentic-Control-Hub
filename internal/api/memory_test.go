package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// memory_test.go — the S15.2 memory family (P3-B6-3C, R18–R22 + the F slice).
// The battery composes the REAL S09 subsystem: a migrated platform.db, the real
// event log, the real knowledge store over a knowledge root in t.TempDir(), and
// the real write gate — so every wall a test asserts is the gate's own wall
// rather than a fake's, and every event counted is the one the gate minted
// inside its transaction.
//
// $0 AND HERMETIC. Nothing here dials a network, spawns an engine or a model,
// prices a call or touches host state: the contradiction screen is nil (the
// advisory S09.7 pass is a named seam and its absence never suppresses the
// deterministic same-topic_key detection) and the git committer is nil, which
// leaves file_commit pending exactly as the landed migration permits.

// ── fixture ─────────────────────────────────────────────────────────────────

type memEnv struct {
	t     *testing.T
	b     *backend
	ctx   context.Context
	store *memory.Store
	gate  *memory.Gate
	proj  *project.Store
}

// newMemEnv builds the four identities the visibility rules need — the
// operator, the entry owner, another member of her project, and a member of
// nothing — plus the project registry row those memberships live in.
func newMemEnv(t *testing.T) *memEnv {
	t.Helper()
	ctx := context.Background()
	b := newBackend(t)
	if err := b.store.CreateUser(ctx, "", auth.User{ID: "op", DisplayName: "Op", Role: auth.RoleOperator}, dlvPIN); err != nil {
		t.Fatalf("create operator: %v", err)
	}
	for _, id := range []string{"alice", "bob", "carol"} {
		if err := b.store.CreateUser(ctx, "op", auth.User{ID: id, DisplayName: id, Role: auth.RoleMember}, dlvPIN); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	store, err := memory.NewStore(b.db, b.log, b.reg, filepath.Join(t.TempDir(), "knowledge"))
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	proj, err := project.New(project.Config{DB: b.db, Log: b.log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	e := &memEnv{t: t, b: b, ctx: ctx, store: store, gate: memory.NewGate(store), proj: proj}
	// alice owns p-alpha and invited bob; carol and the operator belong to no
	// project, which is what makes the project limb of the visibility rule
	// falsifiable in both directions.
	if _, err := proj.Register(ctx, project.RegisterInput{
		ProjectID: "p-alpha", Owner: "alice", Name: "alpha",
		StorePath: filepath.Join(t.TempDir(), "alpha"), Members: []string{"bob"},
	}); err != nil {
		t.Fatalf("register project: %v", err)
	}
	return e
}

func (e *memEnv) server(who string) *api.Server {
	e.t.Helper()
	return api.New(api.Config{
		Log:        e.b.log,
		Sessions:   e.b.store,
		Auth:       fixedIdentity{who},
		Settings:   approvalSettings(),
		HealthFn:   func() api.Health { return api.Health{Ready: true, Mode: "running", Version: "test"} },
		DB:         e.b.db,
		Meter:      fakeMeter{},
		Memory:     e.store,
		MemoryGate: e.gate,
	})
}

func (e *memEnv) do(t *testing.T, who, method, path, body string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	e.server(who).Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

func (e *memEnv) mustDo(t *testing.T, who, method, path, body string) string {
	t.Helper()
	code, out := e.do(t, who, method, path, body)
	if code != http.StatusOK {
		t.Fatalf("%s %s as %s: status %d: %s", method, path, who, code, out)
	}
	return out
}

// create POSTs one entry through the HTTP surface and returns its detail.
func (e *memEnv) create(t *testing.T, who, body string) api.MemoryEntryDetail {
	t.Helper()
	return decodeDetail(t, e.mustDo(t, who, "POST", "/api/memory", body))
}

func decodeDetail(t *testing.T, raw string) api.MemoryEntryDetail {
	t.Helper()
	var d api.MemoryEntryDetail
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("decode entry detail: %v: %s", err, raw)
	}
	return d
}

func (e *memEnv) list(t *testing.T, who, query string) api.MemoryList {
	t.Helper()
	var l api.MemoryList
	if err := json.Unmarshal([]byte(e.mustDo(t, who, "GET", "/api/memory"+query, "")), &l); err != nil {
		t.Fatalf("decode memory list: %v", err)
	}
	return l
}

// entryRows counts knowledge rows — the probe behind "the refusal wrote
// nothing", which an event count alone cannot prove.
func (e *memEnv) entryRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.b.db.QueryRowContext(e.ctx, `SELECT COUNT(*) FROM knowledge_entries`).Scan(&n); err != nil {
		t.Fatalf("count knowledge rows: %v", err)
	}
	return n
}

func (e *memEnv) count(typ string) int {
	e.t.Helper()
	var n int
	if err := e.b.db.QueryRowContext(e.ctx, `SELECT COUNT(*) FROM run_events WHERE type = ?`, typ).Scan(&n); err != nil {
		e.t.Fatalf("count %s: %v", typ, err)
	}
	return n
}

func listHas(l api.MemoryList, id string) bool {
	for _, e := range l.Entries {
		if e.EntryID == id {
			return true
		}
	}
	return false
}

// entryBody builds a create body for a user-scope preference.
func userEntry(title string) string {
	return fmt.Sprintf(`{"scope":"user","kind":"preference","title":%q,"content":"terse commit messages"}`, title)
}

// ── R18: the visible set, three ways, both OQ5 directions ───────────────────

// TestMemoryListServesTheVisibleSetAndNothingElse is the ratified OQ5 rule in
// both directions at once: a person's own entries plus house plus the projects
// they belong to — and the operator role bit opening NOTHING of another
// member's personal store.
func TestMemoryListServesTheVisibleSetAndNothingElse(t *testing.T) {
	e := newMemEnv(t)
	aliceOwn := e.create(t, "alice", userEntry("alice's own preference")).Entry.EntryID
	bobOwn := e.create(t, "bob", userEntry("bob's own preference")).Entry.EntryID
	house := e.create(t, "op", `{"scope":"house","kind":"convention","title":"house convention","content":"we write tests first"}`).Entry.EntryID
	proj := e.create(t, "alice",
		`{"scope":"project","scope_ref":"p-alpha","kind":"playbook","title":"alpha playbook","content":"deploy on fridays never"}`).Entry.EntryID

	for _, tc := range []struct {
		who     string
		visible []string
		hidden  []string
	}{
		// The OWNER's own rows must be PRESENT — an all-empty handler cannot
		// pass this battery (§38).
		{"alice", []string{aliceOwn, house, proj}, []string{bobOwn}},
		// A project member sees the project's entry and his own, never hers.
		{"bob", []string{bobOwn, house, proj}, []string{aliceOwn}},
		// A member of nothing sees the house scope and nothing else.
		{"carol", []string{house}, []string{aliceOwn, bobOwn, proj}},
		// THE DIVERGENCE: the operator reads house (theirs to curate) and NOT a
		// member's user-scope content, nor a project they are not in.
		{"op", []string{house}, []string{aliceOwn, bobOwn, proj}},
	} {
		l := e.list(t, tc.who, "")
		for _, id := range tc.visible {
			if !listHas(l, id) {
				t.Errorf("%s must see %s — the visible set is own + house + their projects (OQ5)", tc.who, id)
			}
		}
		for _, id := range tc.hidden {
			if listHas(l, id) {
				t.Errorf("%s must NOT see %s: memory is personal content, and the D10 bit is house authority, not member-memory access (OQ5)", tc.who, id)
			}
		}
		if l.Visibility == "" {
			t.Error("the list must state the rule that produced it")
		}
	}
	// The filters narrow within the visible set, never widen it.
	if l := e.list(t, "carol", "?scope=user"); listHas(l, aliceOwn) || len(l.Entries) != 0 {
		t.Errorf("a scope filter must not widen the visible set: %+v", l.Entries)
	}
	if l := e.list(t, "alice", "?kind=playbook"); !listHas(l, proj) || listHas(l, aliceOwn) {
		t.Error("the kind filter must select the playbook and drop the preference")
	}
	if code, out := e.do(t, "alice", "GET", "/api/memory?scope=nonsense", ""); code != http.StatusBadRequest {
		t.Errorf("an unknown scope must be a 400, got %d: %s", code, out)
	}
	// The page bound is OBSERVED rather than assumed: alice has three visible
	// entries, so a page of one is truncated and a full page is not.
	if page := e.list(t, "alice", "?limit=1"); len(page.Entries) != 1 || !page.Truncated {
		t.Errorf("a bounded page must say it is truncated: %d entries, truncated=%v", len(page.Entries), page.Truncated)
	}
	if full := e.list(t, "alice", "?limit=50"); full.Truncated {
		t.Error("a page holding everything visible must not claim truncation")
	}
}

// TestMemoryDetailIs404BeforeItIs403 pins the landed refusal order on the one
// family where an id leak would be a leak of personal content.
func TestMemoryDetailIs404BeforeItIs403(t *testing.T) {
	e := newMemEnv(t)
	aliceOwn := e.create(t, "alice", userEntry("alice's own preference")).Entry.EntryID
	house := e.create(t, "op", `{"scope":"house","kind":"convention","title":"house","content":"c"}`).Entry.EntryID

	if code, _ := e.do(t, "bob", "GET", "/api/memory/k-doesnotexist", ""); code != http.StatusNotFound {
		t.Error("an unknown entry must 404 before it 403s")
	}
	for _, who := range []string{"bob", "carol", "op"} {
		if code, out := e.do(t, who, "GET", "/api/memory/"+aliceOwn, ""); code != http.StatusForbidden {
			t.Errorf("%s reading alice's user-scope entry: want 403, got %d: %s", who, code, out)
		}
	}
	// The non-tautological controls: her own read succeeds, and everyone reads
	// the house entry.
	d := decodeDetail(t, e.mustDo(t, "alice", "GET", "/api/memory/"+aliceOwn, ""))
	if d.Entry.Content == "" || d.Entry.Provenance.Origin != "human_direct" || d.Entry.Status != "active" {
		t.Errorf("the owner's own detail must carry content, provenance and gate status: %+v", d.Entry)
	}
	if d.Entry.Provenance.ApprovedBy != "alice" || d.Entry.Verification.VerifiedTS == "" {
		t.Errorf("the detail must carry the S09.5 approver and the S09.8 verification stamp: %+v", d.Entry)
	}
	for _, who := range []string{"alice", "bob", "carol", "op"} {
		if code, _ := e.do(t, who, "GET", "/api/memory/"+house, ""); code != http.StatusOK {
			t.Errorf("%s must read the house entry: house defaults address everyone (S4.5)", who)
		}
	}
}

// ── R19: the write gate, and the walls it surfaces ──────────────────────────

// TestCreateRoutesTheGateAndMintsItsCanonicalRow drives the v0-live manual path
// and asserts the audit story is the gate's own event, once.
func TestCreateRoutesTheGateAndMintsItsCanonicalRow(t *testing.T) {
	e := newMemEnv(t)
	d := e.create(t, "alice", userEntry("terse commits"))
	if d.Entry.EntryID == "" || d.Entry.Owner != "alice" || d.Entry.Layer != "L2" {
		t.Fatalf("a manual create lands an owner-attributed L2 row: %+v", d.Entry)
	}
	if d.Entry.Provenance.Version != 1 || d.Entry.Provenance.Origin != "human_direct" {
		t.Errorf("origin/version wrong: %+v", d.Entry.Provenance)
	}
	if got := e.count(memory.EventWrite); got != 1 {
		t.Errorf("want one knowledge.write row, got %d", got)
	}
	if got := e.count(api.EventDecisionRecorded); got != 0 {
		t.Errorf("the create co-minted %d decision.recorded rows — its canonical row already exists (§39 OQ8)", got)
	}
	// Medium tier: no step-up was demanded and none was consumed (S15.6).
	if got := e.count("auth.reprompt"); got != 0 {
		t.Errorf("an own-store write is Medium and demands no PIN re-prompt, got %d", got)
	}
}

// TestHouseScopeIsTheOperatorsAndTheGateSaysSo asserts the D10 wall arrives
// from inside Gate.authorize, with its own words, and that the operator's own
// house write is the control that keeps the assertion honest.
func TestHouseScopeIsTheOperatorsAndTheGateSaysSo(t *testing.T) {
	e := newMemEnv(t)
	body := `{"scope":"house","kind":"convention","title":"house rule","content":"c"}`
	code, out := e.do(t, "alice", "POST", "/api/memory", body)
	if code != http.StatusForbidden {
		t.Fatalf("a member's house write: want 403, got %d: %s", code, out)
	}
	if !strings.Contains(out, memory.ErrOperatorRequired.Error()) {
		t.Errorf("the refusal must carry the GATE's own reason, not a transport paraphrase: %s", out)
	}
	if got := e.count(memory.EventWrite); got != 0 {
		t.Error("a refused write must land no knowledge.write row")
	}
	if d := e.create(t, "op", body); d.Entry.Scope != "house" {
		t.Errorf("the operator's own house write must succeed: %+v", d.Entry)
	}
}

// TestTheDormantScopeAndTheAbsentLayerRefuseWithTheGatesReason: worker-overlay
// activates at v1 and no L1 writer exists at v0. Both are the S09.4 activation
// table showing through, and both answer 400 with the gate's sentence.
func TestTheDormantScopeAndTheAbsentLayerRefuseWithTheGatesReason(t *testing.T) {
	e := newMemEnv(t)
	for _, tc := range []struct{ name, body, want string }{
		{"worker-overlay", `{"scope":"worker_overlay","scope_ref":"t-1","kind":"lesson","title":"x","content":"c"}`,
			memory.ErrScopeDormant.Error()},
		{"L1 observation", `{"scope":"user","kind":"observation","title":"x","content":"c"}`,
			memory.ErrLayerForbidden.Error()},
	} {
		code, out := e.do(t, "alice", "POST", "/api/memory", tc.body)
		if code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d: %s", tc.name, code, out)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: want the gate's own reason %q, got %s", tc.name, tc.want, out)
		}
	}
	if got := e.count(memory.EventWrite); got != 0 {
		t.Error("a refused write must land no knowledge.write row")
	}
}

// TestTheDevIdentityCanNeverWriteMemory: the §9 dev-posture identity holds no
// person row, so the gate refuses it — and it refuses it even though the dev
// identity reads as operator-equivalent, because the wall is "is this a known
// person", not "what role does it have".
func TestTheDevIdentityCanNeverWriteMemory(t *testing.T) {
	e := newMemEnv(t)
	dev := api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store, DevPosture: true, Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} }, DB: e.b.db, Meter: fakeMeter{},
		Memory: e.store, MemoryGate: e.gate,
	})
	rr := httptest.NewRecorder()
	dev.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/memory", strings.NewReader(userEntry("dev"))))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("the dev identity must never write memory: want 403, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), memory.ErrNotHuman.Error()) {
		t.Errorf("want the gate's humans-only refusal, got %s", rr.Body.String())
	}
	if got := e.count(memory.EventWrite); got != 0 {
		t.Error("the dev write must have landed nothing")
	}
}

// TestImportIsTheOperatorsHouseCeremony — the routed Import limb (OQ5's
// one-line addition): the N20 house-KB import, where the operator's import IS
// the approval. Nobody else stamps an entry with that origin.
func TestImportIsTheOperatorsHouseCeremony(t *testing.T) {
	e := newMemEnv(t)
	body := `{"scope":"house","kind":"taxonomy","title":"day-one KB","content":"{}","origin":"imported","origin_ref":"N20 import 2026-07-29"}`
	if code, out := e.do(t, "alice", "POST", "/api/memory", body); code != http.StatusForbidden {
		t.Errorf("a member's import: want 403, got %d: %s", code, out)
	}
	// THE LOAD-BEARING DIRECTION (drain D1): the house case above 403s from the
	// gate's own D10 wall whether or not this transport's origin wall exists, so
	// it proves nothing about that wall. A USER-scope import does: the gate
	// permits a member's own user-scope write, so the only thing standing
	// between a member and a forged `imported` provenance — an entry claiming a
	// curator vouched for it — is the wall at memory.go's origin switch.
	before := e.entryRows(t)
	code, out := e.do(t, "alice", "POST", "/api/memory",
		`{"scope":"user","kind":"lesson","title":"forged","content":"c","origin":"imported","origin_ref":"not mine to claim"}`)
	if code != http.StatusForbidden {
		t.Fatalf("a member's USER-scope import: want 403, got %d: %s", code, out)
	}
	if after := e.entryRows(t); after != before {
		t.Errorf("the refused import LANDED a row (%d → %d) — a 403 that still writes is the failure this probe exists for", before, after)
	}
	if got := e.count(memory.EventWrite); got != 0 {
		t.Errorf("the refused import minted %d knowledge.write rows", got)
	}

	d := e.create(t, "op", body)
	if d.Entry.Provenance.Origin != "imported" || d.Entry.Provenance.OriginRef != "N20 import 2026-07-29" {
		t.Errorf("the import must record its origin verbatim: %+v", d.Entry.Provenance)
	}
	// The vocabulary is closed: the two origins with no HTTP door are refused by
	// name rather than silently written as human_direct.
	for _, origin := range []string{"adopted_from", "proposed_from", "invented"} {
		body := fmt.Sprintf(`{"scope":"user","kind":"lesson","title":"x","content":"c","origin":%q}`, origin)
		if code, out := e.do(t, "alice", "POST", "/api/memory", body); code != http.StatusBadRequest {
			t.Errorf("origin %q: want 400, got %d: %s", origin, code, out)
		}
	}
}

// TestProjectScopeWritesStayInsideTheCallersProjects: project knowledge injects
// into every run of that project (S05.4 selects by scope_ref), so a write into
// a project the caller does not belong to is refused before it reaches the gate
// — the same fail-closed question authorizeOwner answers everywhere else.
func TestProjectScopeWritesStayInsideTheCallersProjects(t *testing.T) {
	e := newMemEnv(t)
	body := `{"scope":"project","scope_ref":"p-alpha","kind":"playbook","title":"p","content":"c"}`
	if code, out := e.do(t, "carol", "POST", "/api/memory", body); code != http.StatusForbidden {
		t.Errorf("a non-member's project write: want 403, got %d: %s", code, out)
	}
	if code, out := e.do(t, "alice", "POST", "/api/memory",
		`{"scope":"project","scope_ref":"p-unregistered","kind":"playbook","title":"p","content":"c"}`); code != http.StatusForbidden {
		t.Errorf("a write into an unregistered project: want 403, got %d: %s", code, out)
	}
	// The controls: the owner and the invited member both write into p-alpha.
	for _, who := range []string{"alice", "bob"} {
		if d := e.create(t, who, body); d.Entry.ScopeRef != "p-alpha" {
			t.Errorf("%s must write into their own project: %+v", who, d.Entry)
		}
	}
	if got := e.count(memory.EventWrite); got != 2 {
		t.Errorf("want two knowledge.write rows from the two accepted writes, got %d", got)
	}
}

// TestAnEditIsANewVersionAndTheRowIsNeverMutated — S09.4 station 4: correction
// is supersession. The landed migration trigger is re-probed underneath, so the
// transport's absence of an edit verb rests on a structural fact.
func TestAnEditIsANewVersionAndTheRowIsNeverMutated(t *testing.T) {
	e := newMemEnv(t)
	first := e.create(t, "alice", userEntry("first")).Entry.EntryID
	next := decodeDetail(t, e.mustDo(t, "alice", "POST", "/api/memory/"+first+"/new-version",
		`{"scope":"user","kind":"preference","title":"corrected","content":"even terser"}`)).Entry
	if next.Provenance.Version != 2 || next.Provenance.Supersedes != first || next.EntryID == first {
		t.Fatalf("a new version is a NEW row carrying supersedes_id: %+v", next.Provenance)
	}
	old := decodeDetail(t, e.mustDo(t, "alice", "GET", "/api/memory/"+first, "")).Entry
	if old.Status != "retired" {
		t.Errorf("the superseded version retires in the same transaction (S09.8), got %q", old.Status)
	}
	// Another member cannot version an entry they cannot even see.
	if code, _ := e.do(t, "bob", "POST", "/api/memory/"+first+"/new-version", userEntry("hijack")); code != http.StatusForbidden {
		t.Error("a new version of someone else's entry must be refused")
	}
	// The wall underneath: raw SQL cannot edit content in place either.
	err := e.b.db.WriteTx(e.ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(e.ctx, `UPDATE knowledge_entries SET content = 'rewritten' WHERE entry_id = ?`, first)
		return err
	})
	if err == nil {
		t.Error("the 0004 trigger must abort an in-place content mutation (§17)")
	}
}

// ── R20: the owner's rights ─────────────────────────────────────────────────

// TestRemoveReturnsTheInfluenceList: removal is exact because selection is
// deterministic, and the surface hands back the runs the entry reached.
func TestRemoveReturnsTheInfluenceList(t *testing.T) {
	e := newMemEnv(t)
	id := e.create(t, "alice", userEntry("influential")).Entry.EntryID
	e.injectedInto(t, "t-1", "r-1", "alice", id)

	var out api.MemoryRemoved
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/memory/"+id+"/remove",
		`{"reason":"no longer true"}`)), &out); err != nil {
		t.Fatalf("decode removal: %v", err)
	}
	if out.Status != "removed" || len(out.Influence) != 1 || out.Influence[0].RunID != "r-1" {
		t.Fatalf("the removal must serve the influence list verbatim: %+v", out)
	}
	if got := e.count(memory.EventRemove); got != 1 {
		t.Errorf("want one knowledge.remove row, got %d", got)
	}
	if decodeDetail(t, e.mustDo(t, "alice", "GET", "/api/memory/"+id, "")).Entry.Status != "removed" {
		t.Error("the entry must read as removed")
	}
	// A second removal is not a silent success: the gate refuses, and the
	// transport reports the state conflict rather than softening it.
	if code, _ := e.do(t, "alice", "POST", "/api/memory/"+id+"/remove", `{}`); code != http.StatusConflict {
		t.Error("removing a removed entry must answer a conflict")
	}
}

// TestRemovalIsOwnerOrOperatorWithinWhatTheOperatorMaySee exercises the §17
// reading-4 operator limb on a scope the operator can actually reach.
func TestRemovalIsOwnerOrOperatorWithinWhatTheOperatorMaySee(t *testing.T) {
	e := newMemEnv(t)
	// The operator joins p-alpha, which is the only way a member's project
	// entry is theirs to see at all (OQ5).
	if _, err := e.proj.Register(e.ctx, project.RegisterInput{
		ProjectID: "p-beta", Owner: "alice", Name: "beta",
		StorePath: filepath.Join(t.TempDir(), "beta"), Members: []string{"op"},
	}); err != nil {
		t.Fatalf("register p-beta: %v", err)
	}
	id := e.create(t, "alice",
		`{"scope":"project","scope_ref":"p-beta","kind":"playbook","title":"beta","content":"c"}`).Entry.EntryID
	if code, out := e.do(t, "op", "POST", "/api/memory/"+id+"/remove", `{"reason":"stale"}`); code != http.StatusOK {
		t.Fatalf("the operator may remove an entry they can see (§17 reading 4): %d %s", code, out)
	}
	// And a member with no relationship to it cannot even reach the verb.
	other := e.create(t, "alice", userEntry("hers")).Entry.EntryID
	if code, _ := e.do(t, "carol", "POST", "/api/memory/"+other+"/remove", `{}`); code != http.StatusForbidden {
		t.Error("a stranger's removal must be refused")
	}
}

// TestTrueDeletionIsStrictlyTheOwnerEvenForTheOperator is the G2 Def.9 line the
// transport must not soften: the operator can SEE the entry, can REMOVE it, and
// still cannot purge it.
func TestTrueDeletionIsStrictlyTheOwnerEvenForTheOperator(t *testing.T) {
	e := newMemEnv(t)
	if _, err := e.proj.Register(e.ctx, project.RegisterInput{
		ProjectID: "p-beta", Owner: "alice", Name: "beta",
		StorePath: filepath.Join(t.TempDir(), "beta"), Members: []string{"op"},
	}); err != nil {
		t.Fatalf("register p-beta: %v", err)
	}
	body := `{"scope":"project","scope_ref":"p-beta","kind":"playbook","title":"beta","content":"secret sauce"}`
	id := e.create(t, "alice", body).Entry.EntryID
	// The operator reads it (so this is authorship, not visibility) and is still
	// refused the purge — by the GATE, with its own words.
	if code, _ := e.do(t, "op", "GET", "/api/memory/"+id, ""); code != http.StatusOK {
		t.Fatal("the operator must be able to read the project entry, or the refusal below proves nothing")
	}
	code, out := e.do(t, "op", "POST", "/api/memory/"+id+"/delete", "")
	if code != http.StatusForbidden {
		t.Fatalf("true deletion is the OWNER's right: want 403, got %d: %s", code, out)
	}
	if !strings.Contains(out, memory.ErrNotOwner.Error()) {
		t.Errorf("want the gate's own refusal, got %s", out)
	}
	if got := e.count(memory.EventDelete); got != 0 {
		t.Error("a refused deletion must land no knowledge.delete row")
	}

	// The owner's own deletion succeeds, and the tombstone reads honestly.
	var del api.MemoryDeleted
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/memory/"+id+"/delete", "")), &del); err != nil {
		t.Fatalf("decode deletion: %v", err)
	}
	if !del.Tombstone || del.Note == "" || len(del.Limits) == 0 {
		t.Errorf("the deletion must serve the tombstone and the S09.5 honest limits: %+v", del)
	}
	stub := decodeDetail(t, e.mustDo(t, "alice", "GET", "/api/memory/"+id, "")).Entry
	if !stub.Tombstone || stub.Content != "" || stub.Title != "" || stub.Status != "removed" {
		t.Errorf("the tombstone keeps id, dates and the note and nothing else: %+v", stub)
	}
	if strings.Contains(e.mustDo(t, "alice", "GET", "/api/memory/"+id, ""), "secret sauce") {
		t.Error("purged content must not survive on the wire")
	}
	if got := e.count(memory.EventDelete); got != 1 {
		t.Errorf("want one knowledge.delete row, got %d", got)
	}
}

// ── R21: conflicts, surfaced and resolvable ─────────────────────────────────

// TestConflictEdgesSurfaceOnTheDetailAndResolveToTheirOwner: a same-topic_key
// write raises the S09.7 question, the detail carries it, and closing it is the
// affected owner's act.
func TestConflictEdgesSurfaceOnTheDetailAndResolveToTheirOwner(t *testing.T) {
	e := newMemEnv(t)
	first := e.create(t, "alice",
		`{"scope":"user","kind":"lesson","title":"first","content":"always squash","topic_key":"merge-style"}`).Entry.EntryID
	second := e.create(t, "alice",
		`{"scope":"user","kind":"lesson","title":"second","content":"never squash","topic_key":"merge-style"}`)
	if len(second.Conflicts) != 1 {
		t.Fatalf("the write that raised the question must serve it back: %+v", second.Conflicts)
	}
	c := second.Conflicts[0]
	if c.Affected != "alice" || c.Question == "" || c.Status != "open" || c.OtherEntryID != first {
		t.Fatalf("the edge must name the pair, the addressee and the question: %+v", c)
	}
	if got := e.count(memory.EventConflict); got != 1 {
		t.Errorf("want one knowledge.conflict row, got %d", got)
	}
	// Both sides of the pair surface the same open question.
	if got := decodeDetail(t, e.mustDo(t, "alice", "GET", "/api/memory/"+first, "")).Conflicts; len(got) != 1 {
		t.Errorf("the other side of the pair must surface the edge too: %+v", got)
	}

	path := fmt.Sprintf("/api/memory/conflicts/%d/resolve", c.ID)
	if code, _ := e.do(t, "bob", "POST", path, ""); code != http.StatusForbidden {
		t.Error("the card is addressed to the affected owner; nobody else answers it")
	}
	if code, _ := e.do(t, "op", "POST", path, ""); code != http.StatusForbidden {
		t.Error("house authority is not the right to answer another person's memory question (OQ5)")
	}
	if code, _ := e.do(t, "alice", "POST", "/api/memory/conflicts/999999/resolve", ""); code != http.StatusNotFound {
		t.Error("an unknown conflict must 404 before it 403s")
	}
	var res api.MemoryConflictResolved
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", path, "")), &res); err != nil {
		t.Fatalf("decode resolution: %v", err)
	}
	if res.Conflict.Status != "resolved" || res.Conflict.ResolvedBy != "alice" || res.Conflict.ResolvedTS == "" {
		t.Fatalf("the closed question records who answered and when: %+v", res.Conflict)
	}
	// Resolved-first: the repeat reads back rather than refusing.
	if code, out := e.do(t, "alice", "POST", path, ""); code != http.StatusOK || !strings.Contains(out, "already closed") {
		t.Errorf("a repeated resolve must read the closed question back: %d %s", code, out)
	}
	// A resolved edge leaves the detail: the surface shows OPEN questions.
	if got := decodeDetail(t, e.mustDo(t, "alice", "GET", "/api/memory/"+first, "")).Conflicts; len(got) != 0 {
		t.Errorf("a resolved edge must leave the open list: %+v", got)
	}
	// Nothing is co-minted on a resolution: the edge row IS the record.
	if got := e.count(api.EventDecisionRecorded); got != 0 {
		t.Errorf("the resolution co-minted %d decision.recorded rows", got)
	}
}

// TestConflictEdgesReachOnlyTheirAddressee is the drain-D2 three-way: an edge
// carries the id, kind, version and question text of the OTHER entry, which is
// routinely somebody's personal one — so a shared entry that many people can
// read must not hand that metadata to all of them. Only the affected owner,
// who is also the only person the resolve verb accepts, sees the block.
func TestConflictEdgesReachOnlyTheirAddressee(t *testing.T) {
	e := newMemEnv(t)
	// p-beta is readable by all three, so every viewer below reads the ENTRY —
	// the difference under test is the edge block alone.
	if _, err := e.proj.Register(e.ctx, project.RegisterInput{
		ProjectID: "p-beta", Owner: "alice", Name: "beta",
		StorePath: filepath.Join(t.TempDir(), "beta"), Members: []string{"bob", "op"},
	}); err != nil {
		t.Fatalf("register p-beta: %v", err)
	}
	first := e.create(t, "alice",
		`{"scope":"project","scope_ref":"p-beta","kind":"lesson","title":"a","content":"always squash","topic_key":"merge-style"}`).Entry.EntryID
	e.create(t, "alice",
		`{"scope":"project","scope_ref":"p-beta","kind":"lesson","title":"b","content":"never squash","topic_key":"merge-style"}`)

	edges := func(who, id string) []memory.Conflict {
		t.Helper()
		return decodeDetail(t, e.mustDo(t, who, "GET", "/api/memory/"+id, "")).Conflicts
	}
	if got := edges("alice", first); len(got) != 1 {
		t.Fatalf("the affected owner must see their own question card: %+v", got)
	}
	for _, who := range []string{"bob", "op"} {
		if got := edges(who, first); len(got) != 0 {
			t.Errorf("%s reads the entry but is not the card's addressee — the edge must not carry another person's entry metadata to them: %+v", who, got)
		}
	}

	// The other shape of the same leak: a HOUSE entry everyone reads, with an
	// edge naming a member's personal entry. The addressee is the house entry's
	// owner, and nobody else learns that the personal entry exists.
	house := e.create(t, "op",
		`{"scope":"house","kind":"convention","title":"house merge rule","content":"squash","topic_key":"house-merge"}`).Entry.EntryID
	e.create(t, "alice",
		`{"scope":"user","kind":"lesson","title":"my private counterexample","content":"never squash","topic_key":"house-merge"}`)
	if got := edges("op", house); len(got) != 1 || got[0].Affected != "op" {
		t.Fatalf("the house entry's owner is the addressee and must see it: %+v", got)
	}
	for _, who := range []string{"alice", "bob", "carol"} {
		if got := edges(who, house); len(got) != 0 {
			t.Errorf("%s must not read the card addressed to the operator: %+v", who, got)
		}
	}
}

// ── the NINTH inbox kind (B6-6 OQ1) ─────────────────────────────────────────

// TestOpenConflictsAreTheNinthInboxKind: an open question card is waiting on a
// person, so it belongs in the one place "the platform needs you" lives (S15.6).
//
// The visibility line is the point and it is a DELIBERATE deviation from the
// other eight kinds' operator-sees-all: the card reaches its addressee and
// nobody else, the operator included. It is the landed B6-3C rule carried onto
// the inbox — one predicate decides both who sees the edge and who may resolve
// it — and this test drives the card AND the door for all three viewers, so a
// card can never be listed for somebody the verb would refuse.
func TestOpenConflictsAreTheNinthInboxKind(t *testing.T) {
	e := newMemEnv(t)
	// A project both members read, so every viewer below can see the ENTRIES.
	// What differs is the card, which is the thing under test.
	if _, err := e.proj.Register(e.ctx, project.RegisterInput{
		ProjectID: "p-beta", Owner: "alice", Name: "beta",
		StorePath: filepath.Join(t.TempDir(), "beta"), Members: []string{"bob", "op"},
	}); err != nil {
		t.Fatalf("register p-beta: %v", err)
	}
	e.create(t, "alice",
		`{"scope":"project","scope_ref":"p-beta","kind":"lesson","title":"a","content":"always squash","topic_key":"merge-style"}`)
	e.create(t, "alice",
		`{"scope":"project","scope_ref":"p-beta","kind":"lesson","title":"b","content":"never squash","topic_key":"merge-style"}`)

	conflicts, err := e.store.OpenConflictsFor(e.ctx, "alice")
	if err != nil {
		t.Fatalf("read open conflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("the two same-topic entries produced %d open conflicts, want 1", len(conflicts))
	}
	cardID := "memory_conflict:" + strconv.FormatInt(conflicts[0].ID, 10)

	// The addressee's inbox carries the card, with the verb its own route
	// accepts and nothing it does not.
	mine := decodeList(t, e.mustDo(t, "alice", "GET", "/api/approvals", ""))
	it, ok := itemByID(mine, cardID)
	if !ok {
		t.Fatalf("the addressee's inbox is missing her own question card: %+v", mine.Items)
	}
	switch {
	case !it.Answerable:
		t.Errorf("the addressee cannot answer her own card: %q", it.NotAnswerableReason)
	case it.Owner != "alice":
		t.Errorf("the card is attributed to %q, not to the person it was addressed to", it.Owner)
	case it.Tier != "medium":
		t.Errorf("tier %q: a question answered individually that releases nothing outward is Medium", it.Tier)
	case it.Batchable || it.StepUpRequired:
		t.Error("a Medium card is individual and un-stepped — the tier rules say so in one place")
	case len(it.Actions) != 1 || it.Actions[0] != "resolve":
		t.Errorf("actions %v: the card names the one verb its route accepts", it.Actions)
	case it.PayloadHash != "":
		t.Error("the resolve verb is path-only and idempotent — a pin here would describe a check nothing performs")
	case it.ExpiryAt != nil:
		t.Error("a memory question releases nothing outward and does not lapse — a countdown would be a deadline nothing enforces")
	case it.Stale:
		t.Error("an old question is still exactly the question; the staleness flag's own reason names re-plan, which is meaningless here")
	}
	// The card IS the row: a surface renders the question, both entry refs and
	// when it was detected without re-deriving any of them.
	var card memory.Conflict
	if err := json.Unmarshal(it.Card, &card); err != nil {
		t.Fatalf("decode the conflict card: %v: %s", err, it.Card)
	}
	if card != conflicts[0] {
		t.Errorf("the card is not the stored row:\n got %+v\nwant %+v", card, conflicts[0])
	}

	// Nobody else sees it — a third party OR the operator. Both directions
	// matter: the question text names another person's entry.
	for _, who := range []string{"bob", "op", "carol"} {
		other := decodeList(t, e.mustDo(t, who, "GET", "/api/approvals", ""))
		if _, found := itemByID(other, cardID); found {
			t.Errorf("%s sees a memory-conflict card addressed to somebody else — the operator has no limb here, deliberately", who)
		}
		for _, item := range other.Items {
			if item.Kind == "memory_conflict" {
				t.Errorf("%s's inbox carries memory-conflict card %q that is not theirs", who, item.ID)
			}
		}
	}

	// The card and the door agree: the same predicate that listed it for alice
	// is the one that refuses everybody else at the verb.
	path := "/api/memory/conflicts/" + strconv.FormatInt(conflicts[0].ID, 10) + "/resolve"
	if code, _ := e.do(t, "op", "POST", path, ""); code != http.StatusForbidden {
		t.Errorf("the operator resolved another person's conflict: %d", code)
	}
	e.mustDo(t, "alice", "POST", path, "")
	// Resolved is closed: the card leaves the inbox because the ROW moved, not
	// because anything client-side remembered answering it.
	after := decodeList(t, e.mustDo(t, "alice", "GET", "/api/approvals", ""))
	if _, found := itemByID(after, cardID); found {
		t.Error("a resolved conflict is still listed as waiting on its addressee")
	}
}

// TestApprovalsServeWithoutAMemoryStore: the ninth kind is an OPTIONAL
// subsystem's contribution, and a process without it still has an inbox. Eight
// kinds readable beats a 503 because one seam is unwired.
func TestApprovalsServeWithoutAMemoryStore(t *testing.T) {
	e := newDecisionEnv(t)
	sevenKinds(t, e)
	list := decodeList(t, e.mustDo(t, "op", "GET", "/api/approvals", ""))
	if len(list.Items) == 0 {
		t.Fatal("the inbox is empty in a process with no memory store")
	}
	for _, it := range list.Items {
		if it.Kind == "memory_conflict" {
			t.Errorf("a memory-conflict card appeared with no memory store wired: %q", it.ID)
		}
	}
}

// ── the walls that exist and were unpinned (drain D6) ───────────────────────

// TestNewVersionIsTheOwnersOrTheOperatorsOnAVisibleEntry pins both limbs of the
// gate's supersession check on an entry a co-member can SEE: seeing an entry is
// not authority to rewrite it, and the operator limb is the one exception the
// gate itself makes.
func TestNewVersionIsTheOwnersOrTheOperatorsOnAVisibleEntry(t *testing.T) {
	e := newMemEnv(t)
	if _, err := e.proj.Register(e.ctx, project.RegisterInput{
		ProjectID: "p-beta", Owner: "alice", Name: "beta",
		StorePath: filepath.Join(t.TempDir(), "beta"), Members: []string{"bob", "op"},
	}); err != nil {
		t.Fatalf("register p-beta: %v", err)
	}
	id := e.create(t, "alice",
		`{"scope":"project","scope_ref":"p-beta","kind":"playbook","title":"beta","content":"c"}`).Entry.EntryID
	next := `{"scope":"project","scope_ref":"p-beta","kind":"playbook","title":"beta v2","content":"c2"}`

	if code, _ := e.do(t, "bob", "GET", "/api/memory/"+id, ""); code != http.StatusOK {
		t.Fatal("bob must be able to READ the entry, or the refusal below proves nothing")
	}
	code, out := e.do(t, "bob", "POST", "/api/memory/"+id+"/new-version", next)
	if code != http.StatusForbidden {
		t.Fatalf("a co-member superseding another person's entry: want 403, got %d: %s", code, out)
	}
	if !strings.Contains(out, memory.ErrNotOwner.Error()) {
		t.Errorf("want the gate's own refusal, got %s", out)
	}
	// The operator limb the gate DOES make (§17 reading 3's project row): the
	// new version keeps the original owner, so curation never re-attributes.
	v2 := decodeDetail(t, e.mustDo(t, "op", "POST", "/api/memory/"+id+"/new-version", next)).Entry
	if v2.Provenance.Version != 2 || v2.Owner != "alice" || v2.Provenance.ApprovedBy != "op" {
		t.Errorf("the operator's version keeps the owner and records its own approver: %+v", v2)
	}
}

// TestAMemberCannotRemoveAHouseEntry: the house scope is readable by everyone
// and curated by the operator (D10). Removal is owner-or-operator, and a member
// is neither — the gate says so.
func TestAMemberCannotRemoveAHouseEntry(t *testing.T) {
	e := newMemEnv(t)
	id := e.create(t, "op", `{"scope":"house","kind":"convention","title":"house","content":"c"}`).Entry.EntryID
	code, out := e.do(t, "alice", "POST", "/api/memory/"+id+"/remove", `{"reason":"i disagree"}`)
	if code != http.StatusForbidden {
		t.Fatalf("a member removing a house entry: want 403, got %d: %s", code, out)
	}
	if !strings.Contains(out, memory.ErrNotOwner.Error()) {
		t.Errorf("want the gate's own refusal, got %s", out)
	}
	if decodeDetail(t, e.mustDo(t, "alice", "GET", "/api/memory/"+id, "")).Entry.Status != "active" {
		t.Error("the house entry must still be active")
	}
	if got := e.count(memory.EventRemove); got != 0 {
		t.Errorf("a refused removal minted %d knowledge.remove rows", got)
	}
}

// TestRepeatDeleteReadsTheTombstoneBack pins the idempotence the delete
// handler's read-back relies on: the gate returns cleanly for an entry that is
// already a tombstone, so a retried tap is an answer rather than an error, and
// no second purge is recorded.
func TestRepeatDeleteReadsTheTombstoneBack(t *testing.T) {
	e := newMemEnv(t)
	id := e.create(t, "alice", userEntry("twice")).Entry.EntryID
	for i := 1; i <= 2; i++ {
		var del api.MemoryDeleted
		if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/memory/"+id+"/delete", "")), &del); err != nil {
			t.Fatalf("delete %d: %v", i, err)
		}
		if !del.Tombstone || del.EntryID != id {
			t.Fatalf("delete %d must serve the tombstone back: %+v", i, del)
		}
	}
	if got := e.count(memory.EventDelete); got != 1 {
		t.Errorf("the repeat purged again: want one knowledge.delete row, got %d", got)
	}
}

// ── R22 + F: the negatives ──────────────────────────────────────────────────

// memoryRoutes is this packet's route inventory, used by the walks below. Every
// body is deliberately INCOMPLETE (no content) and every id deliberately
// unknown, so driving the whole inventory reaches each route without landing a
// single row — which is what lets the same walk assert both reachability and an
// empty audit trail.
var memoryRoutes = []struct{ method, path, body string }{
	{"GET", "/api/memory", ""},
	{"POST", "/api/memory", `{"scope":"user","kind":"preference","title":"x"}`},
	{"GET", "/api/memory/k-x", ""},
	{"POST", "/api/memory/k-x/new-version", `{"scope":"user","kind":"preference","title":"x"}`},
	{"POST", "/api/memory/k-x/remove", `{}`},
	{"POST", "/api/memory/k-x/delete", ""},
	{"POST", "/api/memory/conflicts/1/resolve", ""},
}

// TestMemoryHasNoRouteForProposalsAdoptionOrKnowledgeSearch is the R22
// route-table negative: the S09 surfaces that are NOT this family's have no
// door here, at any shape one might be reached through.
func TestMemoryHasNoRouteForProposalsAdoptionOrKnowledgeSearch(t *testing.T) {
	e := newMemEnv(t)
	id := e.create(t, "alice", userEntry("x")).Entry.EntryID
	for _, probe := range []struct{ method, path string }{
		{"GET", "/api/memory/proposals"},          // lesson_proposals: not memory (S09.2)
		{"POST", "/api/memory/proposals"},         //
		{"GET", "/api/memory/lesson-proposals"},   //
		{"POST", "/api/memory/" + id + "/adopt"},  // Adopt: the v1 sharing surface (S09.4)
		{"POST", "/api/memory/adopt"},             //
		{"GET", "/api/memory/search"},             // knowledge_search: the run-scoped engine tool (S09.3)
		{"GET", "/api/memory/search?q=squash"},    //
		{"POST", "/api/memory/" + id + "/import"}, // Import rides the create body's origin, not a door of its own
		{"PUT", "/api/memory/" + id},              // an edit is a new version, never a mutation
		{"PATCH", "/api/memory/" + id},            //
		{"DELETE", "/api/memory/" + id},           // deletion is a verb with a gate, not a method
	} {
		code, out := e.do(t, "alice", probe.method, probe.path, "")
		if code == http.StatusOK {
			t.Errorf("%s %s answered 200 — that surface has no door in this family (R22): %s", probe.method, probe.path, out)
		}
	}
	// The non-tautological control: the doors that DO exist answer.
	if code, _ := e.do(t, "alice", "GET", "/api/memory/"+id, ""); code != http.StatusOK {
		t.Fatal("the probe set proves nothing if the real routes do not answer")
	}
}

// TestMemoryWritesReachTheGateAndNothingElse is the source scan behind the
// hard wall: internal/api holds a *memory.Store and a *memory.Gate, and the one
// thing it may never do is write a knowledge table itself or reach a verb that
// belongs to another surface.
func TestMemoryWritesReachTheGateAndNothingElse(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	banned := map[string]string{
		"INSERT INTO knowledge": "a knowledge row is the gate's to write (S09.4 station 3)",
		"UPDATE knowledge":      "a knowledge row is never mutated in place — an edit is a new version (S09.4 station 4)",
		"DELETE FROM knowledge": "a knowledge row is never deleted — a purge leaves a tombstone (G2 Def.9)",
		"lesson_proposals":      "proposals are not memory and are read into nothing (S09.2)",
		".Adopt(":               "adoption is the v1 browse-and-adopt surface (S09.4)",
		"knowledge_fts":         "knowledge_search is the run-scoped engine tool, scoped from a run id (S09.3)",
	}
	scanned := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src := stripGoComments(readSourceFile(t, name))
		scanned++
		for needle, why := range banned {
			if strings.Contains(src, needle) {
				t.Errorf("%s contains %q — %s", name, needle, why)
			}
		}
	}
	if scanned == 0 {
		t.Fatal("scanned no files — the scan proves nothing")
	}
	// The probe: the scan must be able to SEE what it looks for.
	if !strings.Contains(`x := "INSERT INTO knowledge_entries"`, "INSERT INTO knowledge") {
		t.Fatal("the source scan cannot detect its own probe")
	}
	// And the positive half: every gate verb this family routes is actually
	// called, so "no SQL" cannot pass by the file doing nothing.
	src := readSourceFile(t, "memory.go")
	for _, verb := range []string{
		"memGate.CreateManual(", "memGate.Import(", "memGate.NewVersion(",
		"memGate.Remove(", "memGate.TrueDelete(", "memGate.ResolveConflict(",
	} {
		if !strings.Contains(src, verb) {
			t.Errorf("memory.go does not call %s — the family routes the gate's verbs", verb)
		}
	}
	// The §17 import wall holds in the direction this packet moves: the api may
	// import memory, and it still reaches the outward world nowhere.
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "memory.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range parsed.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if strings.Contains(path, "internal/redact") {
			t.Error("memory.go imports internal/redact — entry content is CONTENT, served unwrapped (§38 ruling (b))")
		}
	}
}

// TestEveryMemoryRouteIsRoutedAndNamesItsAuditRow is the route-table review and
// the R26 mutation walk in one: every route answers (a 405 means it was never
// registered), every mutating one names the landed canonical row it produces,
// and the walk drives them for real and counts what landed.
func TestEveryMemoryRouteIsRoutedAndNamesItsAuditRow(t *testing.T) {
	audits := map[string]string{
		"POST /api/memory":                              "knowledge.write (S09.4, minted inside the gate's one transaction; knowledge.conflict rides the same tx when the write raises a question)",
		"POST /api/memory/{entry}/new-version":          "knowledge.write (the supersession is a new row in the same transaction that retires the old one)",
		"POST /api/memory/{entry}/remove":               "knowledge.remove (S09.5)",
		"POST /api/memory/{entry}/delete":               "knowledge.delete (G2 Def.9)",
		"POST /api/memory/conflicts/{conflict}/resolve": "the knowledge_conflicts row itself — resolved_by + resolved_ts ARE the record of who closed the question (S09.7)",
	}
	e := newMemEnv(t)
	declared := map[string]bool{}
	for _, r := range memoryRoutes {
		code, out := e.do(t, "alice", r.method, r.path, r.body)
		if code == http.StatusMethodNotAllowed {
			t.Errorf("%s %s is not routed (405): %s", r.method, r.path, out)
		}
		if r.method == http.MethodGet {
			continue
		}
		key := r.method + " " + strings.Replace(
			strings.Replace(r.path, "/k-x", "/{entry}", 1), "/conflicts/1/", "/conflicts/{conflict}/", 1)
		declared[key] = true
		if audits[key] == "" {
			t.Errorf("%s has no named audit row — a mutating verb whose audit story is nothing is exactly what §29 forbids", key)
		}
	}
	for key := range audits {
		if !declared[key] {
			t.Errorf("%q is declared with an audit row but is not in the route inventory — the enumeration must stay honest both ways", key)
		}
	}

	if got := e.count(memory.EventWrite); got != 0 {
		t.Fatalf("the inventory walk landed %d rows — its bodies are incomplete on purpose, so the counts below measure the real walk alone", got)
	}

	// The walk: every mutation, driven for real, and its canonical row counted.
	first := e.create(t, "alice",
		`{"scope":"user","kind":"lesson","title":"a","content":"always squash","topic_key":"merge-style"}`).Entry.EntryID
	second := e.create(t, "alice",
		`{"scope":"user","kind":"lesson","title":"b","content":"never squash","topic_key":"merge-style"}`)
	e.mustDo(t, "alice", "POST", "/api/memory/"+first+"/new-version",
		`{"scope":"user","kind":"lesson","title":"a2","content":"squash unless the history is the point"}`)
	e.mustDo(t, "alice", "POST", fmt.Sprintf("/api/memory/conflicts/%d/resolve", second.Conflicts[0].ID), "")
	e.mustDo(t, "alice", "POST", "/api/memory/"+second.Entry.EntryID+"/remove", `{"reason":"superseded in spirit"}`)
	e.mustDo(t, "alice", "POST", "/api/memory/"+second.Entry.EntryID+"/delete", "")

	for _, want := range []struct {
		typ string
		n   int
	}{
		{memory.EventWrite, 3},    // two creates + one new version
		{memory.EventConflict, 1}, // the second create raised the question
		{memory.EventRemove, 1},
		{memory.EventDelete, 1},
	} {
		if got := e.count(want.typ); got != want.n {
			t.Errorf("want %d %s rows, got %d", want.n, want.typ, got)
		}
	}
	if got := e.count(api.EventDecisionRecorded); got != 0 {
		t.Errorf("the memory walk minted %d decision.recorded rows — every act here has a landed canonical row (§39 OQ8)", got)
	}
	if got := e.count("knowledge.curation"); got != 0 {
		t.Errorf("the curation event is the S09.8 assembly-time card's, not this surface's, got %d", got)
	}
}

func TestMemoryRoutesRefuseWhenNotWired(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "op", auth.RoleOperator)
	srv := api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{"op"}, Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} }, DB: b.db, Meter: fakeMeter{},
		// Memory and MemoryGate deliberately nil.
	})
	for _, r := range memoryRoutes {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest(r.method, r.path, strings.NewReader(r.body)))
		if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "not_wired") {
			t.Errorf("%s %s with no surface wired: want 503 not_wired, got %d: %s", r.method, r.path, rr.Code, rr.Body.String())
		}
	}
}

func TestMemoryRoutesAreSessionRequiredAndUnversioned(t *testing.T) {
	e := newMemEnv(t)
	closed := api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store, Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} }, DB: e.b.db, Meter: fakeMeter{},
		Memory: e.store, MemoryGate: e.gate,
	})
	for _, r := range memoryRoutes {
		rr := httptest.NewRecorder()
		closed.Handler().ServeHTTP(rr, httptest.NewRequest(r.method, r.path, strings.NewReader(r.body)))
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with no identity: want 401, got %d", r.method, r.path, rr.Code)
		}
	}
	if code, _ := e.do(t, "op", "GET", "/api/v1/memory", ""); code != http.StatusNotFound {
		t.Errorf("/api/v1/memory: a version prefix must not exist, got %d", code)
	}
	// Outside /api the same absence reads as the app shell answering (B6-4).
	code, body := e.do(t, "op", "GET", "/v1/api/memory", "")
	assertServedByTheAppShell(t, "/v1/api/memory", code, body)
}

// TestMemoryShapesNeverPercent extends the §30 never-percent negative over every
// shape this packet adds, and the §38 R8 money line with it: a memory entry
// carries no figure at all.
func TestMemoryShapesNeverPercent(t *testing.T) {
	e := newMemEnv(t)
	id := e.create(t, "alice",
		`{"scope":"user","kind":"lesson","title":"a","content":"c","topic_key":"k"}`).Entry.EntryID
	// A second entry on the same topic raises the question card, so the
	// resolution shape joins the walk (drain D5).
	conflicted := e.create(t, "alice",
		`{"scope":"user","kind":"lesson","title":"b","content":"c2","topic_key":"k"}`)
	keys := map[string]bool{}
	for _, path := range []string{"/api/memory", "/api/memory/" + id} {
		var v any
		if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "GET", path, "")), &v); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		collectKeys(v, keys)
	}
	// EVERY new shape: the removal, the resolution and the deletion, not just
	// the reads (drain D5).
	for _, w := range []struct{ path, body string }{
		{fmt.Sprintf("/api/memory/conflicts/%d/resolve", conflicted.Conflicts[0].ID), ""},
		{"/api/memory/" + id + "/remove", `{}`},
		{"/api/memory/" + id + "/delete", ""},
	} {
		var v any
		if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", w.path, w.body)), &v); err != nil {
			t.Fatalf("decode %s: %v", w.path, err)
		}
		collectKeys(v, keys)
	}
	for _, want := range []string{"tombstone", "influence", "conflict"} {
		if !keys[want] {
			t.Fatalf("the walk never reached the %q shape — it must cover every shape this packet adds", want)
		}
	}
	if len(keys) == 0 {
		t.Fatal("collected no keys — the scan proves nothing")
	}
	forbidden := map[string]bool{
		"percent": true, "percentage": true, "percent_complete": true,
		"fraction": true, "ratio": true, "progress": true, "pct": true,
		"eta": true, "eta_seconds": true, "cost": true, "usd": true, "cost_usd": true,
	}
	if !forbidden["percent"] {
		t.Fatal("the scan cannot detect its own probe")
	}
	for k := range keys {
		lk := strings.ToLower(k)
		if forbidden[lk] || strings.Contains(lk, "percent") {
			t.Errorf("key %q has no place on a knowledge entry (§30 R12; §38 R8)", k)
		}
		for _, shape := range []string{"_pct", "_ratio", "_fraction", "utiliz", "_usd"} {
			if strings.Contains(lk, shape) {
				t.Errorf("key %q is ratio- or money-shaped and undeclared", k)
			}
		}
	}
}

// ── fixture helpers ─────────────────────────────────────────────────────────

// injectedInto records that an entry was placed into a run's context — the
// S05.4 trace-manifest row the S2.9 influence join reads. It is written as the
// ledger writes it (the item id is `knowledge/<entry>`), because the join is
// over that shape.
func (e *memEnv) injectedInto(t *testing.T, taskID, runID, owner, entryID string) {
	t.Helper()
	exec(t, e.b, `INSERT OR IGNORE INTO tasks (task_id, user_id, title, kanban_status, created_ts) VALUES (?,?,?,'new',?)`,
		taskID, owner, taskID, nowTS())
	if _, err := run.NewStore(e.b.db, e.b.log).Create(e.ctx, run.NewRun{
		ID: runID, UserID: owner, TaskID: taskID, Substrate: "claude-cli", Lane: "anthropic",
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	payload, err := json.Marshal(map[string]any{
		"items": []map[string]string{{"item_id": "knowledge/" + entryID, "source": "knowledge"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.b.log.Append(e.ctx, eventlog.Append{
		RunID: runID, UserID: owner, Type: ledger.EventContextManifest,
		SchemaVersion: 1, Payload: payload,
	}); err != nil {
		t.Fatalf("append manifest: %v", err)
	}
}

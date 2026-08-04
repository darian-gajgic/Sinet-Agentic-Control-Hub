package api_test

// meters_verbs_test.go — the S10.4 meters mutations and the S15.5 board-drag
// hint (P3-B6-2B, R10–R13). Every cross-owner battery here runs in the
// NON-TAUTOLOGICAL direction as well: the person's OWN act must succeed, so a
// handler that refused everything could not pass.

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

// ── the part-B environment ──────────────────────────────────────────────────

// fakeBudgetStore / fakePauseStore / fakeHints stand in for the landed stores at
// the transport boundary. They are deliberately thin: what is under test here is
// SCOPE, VALIDATION and the AUDIT obligation, and a store with behavior of its
// own would blur which side a failure came from. The stores' real behavior is
// asserted in internal/metering, and the hint's real behavior in
// internal/scheduler.
type fakeBudgetStore struct {
	mu   sync.Mutex
	rows map[string]api.BudgetRecord
	err  error
}

func newFakeBudgetStore() *fakeBudgetStore {
	return &fakeBudgetStore{rows: map[string]api.BudgetRecord{}}
}

func (f *fakeBudgetStore) DeclareBudget(_ context.Context, rec api.BudgetRecord) (api.BudgetRecord, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return api.BudgetRecord{}, false, f.err
	}
	key := rec.Owner + "/" + rec.Lane
	prior, existed := f.rows[key]
	f.rows[key] = rec
	return prior, existed, nil
}

func (f *fakeBudgetStore) Budget(_ context.Context, userID, lane string) (api.BudgetRecord, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.rows[userID+"/"+lane]
	return rec, ok, nil
}

type fakePauseStore struct {
	mu     sync.Mutex
	paused map[string]bool
}

func newFakePauseStore() *fakePauseStore { return &fakePauseStore{paused: map[string]bool{}} }

func (f *fakePauseStore) Paused(_ context.Context, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.paused[userID], nil
}

func (f *fakePauseStore) SetPause(_ context.Context, userID string, paused bool) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	prior := f.paused[userID]
	f.paused[userID] = paused
	return prior, nil
}

type fakeHints struct {
	mu    sync.Mutex
	ranks map[string]int64
	// queued is the set of runs that still have a queued row; anything else
	// answers the honest stale-board no-op.
	queued map[string]bool
}

func newFakeHints(queued ...string) *fakeHints {
	h := &fakeHints{ranks: map[string]int64{}, queued: map[string]bool{}}
	for _, r := range queued {
		h.queued[r] = true
	}
	return h
}

func (f *fakeHints) SetPriorityHint(_ context.Context, runID string, rank int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.queued[runID] {
		return false, nil
	}
	f.ranks[runID] = rank
	return true, nil
}

func (f *fakeHints) rank(runID string) (int64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.ranks[runID]
	return r, ok
}

// verbEnv extends the landed decision-plane fixture with the B6-2B seams. It
// composes the SAME server the part-A battery does, so the two halves of the
// decision plane are exercised against one process.
type verbEnv struct {
	*decisionEnv
	budgets  *fakeBudgetStore
	pause    *fakePauseStore
	hints    *fakeHints
	watchdog *fakeSuppressor
	resume   *fakeResumer
}

func newVerbEnv(t *testing.T, queuedRuns ...string) *verbEnv {
	t.Helper()
	return &verbEnv{
		decisionEnv: newDecisionEnv(t),
		budgets:     newFakeBudgetStore(),
		pause:       newFakePauseStore(),
		hints:       newFakeHints(queuedRuns...),
		watchdog:    &fakeSuppressor{},
		resume:      &fakeResumer{},
	}
}

func (e *verbEnv) server(t *testing.T, who string) *api.Server {
	t.Helper()
	return api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store, Auth: fixedIdentity{who},
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       e.b.db, Meter: fakeMeter{},
		Intake: e.surface, Effects: e.journal, Cancel: e.cancel,
		Budgets: e.budgets, Pause: e.pause, Hints: e.hints,
		Watchdog: e.watchdog, Resume: e.resume,
	})
}

func (e *verbEnv) do(t *testing.T, who, method, path, body string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	e.server(t, who).Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

func (e *verbEnv) mustDo(t *testing.T, who, method, path, body string) string {
	t.Helper()
	code, out := e.do(t, who, method, path, body)
	if code != http.StatusOK {
		t.Fatalf("%s %s as %s: status %d: %s", method, path, who, code, out)
	}
	return out
}

// decisionRows returns the decision.recorded payloads of one card type.
func decisionRows(t *testing.T, b *backend, cardType string) []map[string]any {
	t.Helper()
	rows, err := b.db.QueryContext(context.Background(),
		`SELECT payload FROM run_events WHERE type = ? ORDER BY event_seq`, api.EventDecisionRecorded)
	if err != nil {
		t.Fatalf("read decisions: %v", err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			t.Fatalf("decode decision payload: %v", err)
		}
		if cardType == "" || m["card_type"] == cardType {
			out = append(out, m)
		}
	}
	return out
}

// ── R11 / OQ4: budget edits ────────────────────────────────────────────────

// TestBudgetEditAuthorityIsOwnPlusOperatorAny is the three-way cross-owner
// battery for the budget verb, including the direction that makes it mean
// something: a member declaring their OWN budget must succeed.
func TestBudgetEditAuthorityIsOwnPlusOperatorAny(t *testing.T) {
	for _, tc := range []struct {
		name, who, person string
		want              int
	}{
		{"a member declares their own budget", "alice", "", http.StatusOK},
		{"a member declares their own budget by name", "alice", "alice", http.StatusOK},
		{"a member cannot declare another member's", "alice", "bob", http.StatusForbidden},
		{"the operator may declare anybody's (D10)", "op", "bob", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newVerbEnv(t)
			body := `{"lane":"anthropic","period_tokens":1000,"period_days":30`
			if tc.person != "" {
				body += `,"person":"` + tc.person + `"`
			}
			body += `}`
			code, out := e.do(t, tc.who, "POST", "/api/meters/budget", body)
			if code != tc.want {
				t.Fatalf("status %d, want %d: %s", code, tc.want, out)
			}
			if tc.want != http.StatusOK {
				// A refusal fires nothing: no row, no audit.
				if len(e.budgets.rows) != 0 {
					t.Error("a refused budget edit still wrote a budget")
				}
				if len(decisionRows(t, e.b, "budget")) != 0 {
					t.Error("a refused budget edit still recorded a decision")
				}
			}
		})
	}
}

// TestBudgetEditIsAuditedOldToNew: every edit records its ACTOR and the
// before-and-after, which is the whole audit obligation for an edit (OQ4).
func TestBudgetEditIsAuditedOldToNew(t *testing.T) {
	e := newVerbEnv(t)
	e.mustDo(t, "alice", "POST", "/api/meters/budget",
		`{"lane":"anthropic","period_tokens":1000,"period_days":30}`)
	// The operator re-declares it for her: a different actor, a real prior.
	out := e.mustDo(t, "op", "POST", "/api/meters/budget",
		`{"person":"alice","lane":"anthropic","period_tokens":2500,"period_days":30,"reason":"raised for the sprint"}`)

	var declared api.BudgetDeclared
	if err := json.Unmarshal([]byte(out), &declared); err != nil {
		t.Fatalf("decode: %v: %s", err, out)
	}
	if declared.Budget.PeriodTokens != 2500 || declared.Budget.DeclaredBy != "op" {
		t.Fatalf("response budget = %+v, want 2500 declared by op", declared.Budget)
	}
	if declared.Prior == nil || declared.Prior.PeriodTokens != 1000 {
		t.Fatalf("response prior = %+v, want the 1000-unit budget it replaced", declared.Prior)
	}
	// The unit travels, and it is never dollars (D5).
	if !strings.Contains(declared.Budget.Unit, "weighted-consumption") {
		t.Errorf("budget unit = %q, want the S10.4 weighted-consumption unit", declared.Budget.Unit)
	}

	rows := decisionRows(t, e.b, "budget")
	if len(rows) != 2 {
		t.Fatalf("recorded %d budget decisions, want 2", len(rows))
	}
	second := rows[1]
	if second["actor"] != "op" || second["actor_is_operator"] != true {
		t.Errorf("the audit row does not name the operator as actor: %v", second)
	}
	oldSide, _ := json.Marshal(second["old"])
	newSide, _ := json.Marshal(second["new"])
	if !strings.Contains(string(oldSide), "1000") || !strings.Contains(string(newSide), "2500") {
		t.Errorf("the audit row does not carry old→new: old=%s new=%s", oldSide, newSide)
	}
	// The FIRST declaration's old side is the explicit "there was none" — an
	// absence stated, not a member left out.
	firstOld, _ := json.Marshal(rows[0]["old"])
	if !strings.Contains(string(firstOld), `"declared":false`) {
		t.Errorf("the first edit's old side = %s, want an explicit undeclared object", firstOld)
	}
}

// TestBudgetVerbRefusesWhatIsNotABudget: boundary validation, at the boundary.
func TestBudgetVerbRefusesWhatIsNotABudget(t *testing.T) {
	e := newVerbEnv(t)
	for _, tc := range []struct{ name, body string }{
		{"no lane", `{"period_tokens":10,"period_days":30}`},
		{"zero budget", `{"lane":"anthropic","period_tokens":0,"period_days":30}`},
		{"negative budget", `{"lane":"anthropic","period_tokens":-5,"period_days":30}`},
		{"no period length", `{"lane":"anthropic","period_tokens":10}`},
		{"bad period start", `{"lane":"anthropic","period_tokens":10,"period_days":30,"period_start":"yesterday"}`},
	} {
		if code, out := e.do(t, "alice", "POST", "/api/meters/budget", tc.body); code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400: %s", tc.name, code, out)
		}
	}
	if len(e.budgets.rows) != 0 {
		t.Error("a refused request still wrote a budget")
	}
}

// ── R13: the recreated view is TRUE in BOTH states ─────────────────────────

// TestBudgetViewIsTrueInBothStates drives the REAL migration-0017 view over the
// REAL budgets table. It is the honesty check the recreate exists for: the 0016
// text said "no operator budget is persisted at v0", which becomes a lie the
// moment one is.
//
// Both limbs assert the same D5 property: the dollar columns are NULL either
// way, because a flat-rate lane's budget is denominated in weighted-consumption
// units and there is no dollar remainder to report.
func TestBudgetViewIsTrueInBothStates(t *testing.T) {
	b := newBackend(t)
	ctx := context.Background()
	seedUser(t, b, "alice", "member")
	seedUser(t, b, "bob", "member")
	seedUser(t, b, "carol", "member")
	// Consumption for both, so both appear in cost_per_person.
	for _, u := range []string{"alice", "bob"} {
		seedTask(t, b, "t-"+u, u, "T", "doing")
		seedRun(t, b, "r-"+u, u, "t-"+u, "completed", "anthropic")
	}
	exec(t, b, `INSERT INTO budgets (user_id, lane, period_tokens, period_start, period_days, declared_ts, declared_by)
	            VALUES (?,?,?,?,?,?,?)`,
		"alice", "anthropic", 2000, nowTS(), 30, nowTS(), "alice")

	type viewRow struct {
		declared     int64
		budgetUSD    sql.NullFloat64
		remainderUSD sql.NullFloat64
		status       string
		reason       string
		lanes        sql.NullInt64
		tokens       sql.NullInt64
		unit         sql.NullString
	}
	read := func(user string) viewRow {
		t.Helper()
		var v viewRow
		if err := b.db.QueryRowContext(ctx,
			`SELECT budget_declared, budget_usd, remainder_usd, remainder_status, absence_reason,
			        budget_lanes, budget_period_tokens_total, budget_unit
			   FROM cost_budget_remainder WHERE user_id = ?`, user).
			Scan(&v.declared, &v.budgetUSD, &v.remainderUSD, &v.status, &v.reason, &v.lanes, &v.tokens, &v.unit); err != nil {
			t.Fatalf("read view for %s: %v", user, err)
		}
		return v
	}

	// (a) UNDECLARED — the honest absence, unchanged from 0016.
	undeclared := read("bob")
	if undeclared.declared != 0 {
		t.Errorf("bob budget_declared = %d, want 0", undeclared.declared)
	}
	if !strings.Contains(undeclared.reason, "no operator budget") {
		t.Errorf("bob absence_reason does not say why: %q", undeclared.reason)
	}
	if undeclared.tokens.Valid || undeclared.lanes.Valid || undeclared.unit.Valid {
		t.Errorf("bob has budget columns with no budget declared: %+v", undeclared)
	}

	// (b) DECLARED — real, in the budget's own unit.
	declared := read("alice")
	if declared.declared != 1 {
		t.Errorf("alice budget_declared = %d, want 1 — the view must stop saying no budget is persisted", declared.declared)
	}
	if !declared.tokens.Valid || declared.tokens.Int64 != 2000 {
		t.Errorf("alice budget_period_tokens_total = %+v, want 2000", declared.tokens)
	}
	if !declared.lanes.Valid || declared.lanes.Int64 != 1 {
		t.Errorf("alice budget_lanes = %+v, want 1", declared.lanes)
	}
	if !strings.Contains(declared.unit.String, "weighted-consumption") ||
		strings.Contains(strings.ToLower(declared.unit.String), "usd") {
		t.Errorf("alice budget_unit = %q, want the weighted-consumption unit and never a currency", declared.unit.String)
	}
	if strings.Contains(declared.reason, "no operator budget") {
		t.Errorf("alice's row still claims no budget is persisted: %q", declared.reason)
	}
	if !strings.Contains(declared.reason, "D5") {
		t.Errorf("alice's row does not give the D5 reason the dollar columns are absent: %q", declared.reason)
	}

	// (c) The D5 invariant, both states: no dollar budget, no dollar remainder.
	for who, v := range map[string]viewRow{"alice": declared, "bob": undeclared} {
		if v.budgetUSD.Valid || v.remainderUSD.Valid {
			t.Errorf("%s has a dollar budget/remainder (%+v) — a flat-rate lane has none (D5)", who, v)
		}
		if v.status != "UNAVAILABLE" {
			t.Errorf("%s remainder_status = %q, want UNAVAILABLE (the DOLLAR remainder never exists)", who, v.status)
		}
	}

	// (d) A person who declared a budget but has consumed nothing still gets
	// their declaration back: the view answers about the right subject either way.
	exec(t, b, `INSERT INTO budgets (user_id, lane, period_tokens, period_start, period_days, declared_ts, declared_by)
	            VALUES (?,?,?,?,?,?,?)`,
		"carol", "anthropic", 500, nowTS(), 7, nowTS(), "carol")
	if got := read("carol"); got.declared != 1 || got.tokens.Int64 != 500 {
		t.Errorf("carol (budget, no consumption) = %+v, want her declaration", got)
	}
}

// ── R12: the pause switch ──────────────────────────────────────────────────

func TestPauseVerbAuthorityAndAudit(t *testing.T) {
	for _, tc := range []struct {
		name, who, person string
		want              int
	}{
		{"a person pauses their own automation", "alice", "", http.StatusOK},
		{"a member cannot pause another member", "alice", "bob", http.StatusForbidden},
		{"the operator may pause anybody (D10)", "op", "bob", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newVerbEnv(t)
			body := `{"paused":true`
			if tc.person != "" {
				body += `,"person":"` + tc.person + `"`
			}
			body += `}`
			code, out := e.do(t, tc.who, "POST", "/api/meters/pause", body)
			if code != tc.want {
				t.Fatalf("status %d, want %d: %s", code, tc.want, out)
			}
			if tc.want != http.StatusOK {
				if e.pause.paused["bob"] {
					t.Error("a refused pause still flipped the switch")
				}
				if len(decisionRows(t, e.b, "automation_pause")) != 0 {
					t.Error("a refused pause still recorded a decision")
				}
			}
		})
	}
}

func TestPauseVerbRecordsBothFlipsWithOldToNew(t *testing.T) {
	e := newVerbEnv(t)
	pauseOut := e.mustDo(t, "alice", "POST", "/api/meters/pause", `{"paused":true,"reason":"I need my headroom"}`)
	var p api.AutomationPause
	if err := json.Unmarshal([]byte(pauseOut), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !p.Paused || !p.Changed || p.Owner != "alice" {
		t.Fatalf("pause outcome = %+v, want alice paused and changed", p)
	}
	// A repeat says so rather than claiming a change.
	repeat := e.mustDo(t, "alice", "POST", "/api/meters/pause", `{"paused":true}`)
	if err := json.Unmarshal([]byte(repeat), &p); err != nil {
		t.Fatal(err)
	}
	if p.Changed {
		t.Error("a repeat pause claimed to have changed the switch")
	}
	e.mustDo(t, "alice", "POST", "/api/meters/pause", `{"paused":false}`)

	rows := decisionRows(t, e.b, "automation_pause")
	if len(rows) != 3 {
		t.Fatalf("recorded %d pause decisions, want 3 — BOTH flips (and the repeat) are audited", len(rows))
	}
	if rows[0]["decision"] != "pause" || rows[2]["decision"] != "resume" {
		t.Errorf("decisions = %v / %v, want pause then resume", rows[0]["decision"], rows[2]["decision"])
	}
	last, _ := json.Marshal(rows[2])
	if !strings.Contains(string(last), `"automation_paused":true`) || !strings.Contains(string(last), `"automation_paused":false`) {
		t.Errorf("the resume's audit row does not carry old→new: %s", last)
	}
}

func TestPauseVerbNeedsAnExplicitPosition(t *testing.T) {
	e := newVerbEnv(t)
	if code, out := e.do(t, "alice", "POST", "/api/meters/pause", `{}`); code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400 — a request that did not say which way is not a request: %s", code, out)
	}
}

// ── R10: the board-drag hint ───────────────────────────────────────────────

// TestPriorityHintIsOwnOnly is the three-way cross-owner battery, and the
// operator is INSIDE the refusal here rather than outside it: S15.5 sanctions
// reordering one's OWN queued tasks, and reordering somebody else's queue is a
// different act, not an administrative version of the same one.
func TestPriorityHintIsOwnOnly(t *testing.T) {
	for _, tc := range []struct {
		name, who string
		want      int
	}{
		{"the owner reorders her own task", "alice", http.StatusOK},
		{"another member cannot", "bob", http.StatusForbidden},
		{"the operator cannot either — S15.5 says one's own", "op", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newVerbEnv(t, "r-alice")
			seedTask(t, e.b, "t-alice", "alice", "Alice Task", "doing")
			seedRun(t, e.b, "r-alice", "alice", "t-alice", "queued", "lane-a")

			code, out := e.do(t, tc.who, "POST", "/api/tasks/t-alice/priority-hint", `{"rank":-3}`)
			if code != tc.want {
				t.Fatalf("status %d, want %d: %s", code, tc.want, out)
			}
			rank, hinted := e.hints.rank("r-alice")
			if tc.want == http.StatusOK {
				if !hinted || rank != -3 {
					t.Fatalf("hint rank = (%d, %v), want -3 recorded", rank, hinted)
				}
			} else if hinted {
				t.Error("a refused drag still wrote a hint")
			}
		})
	}
	t.Run("an unknown task is 404, never an existence oracle", func(t *testing.T) {
		e := newVerbEnv(t)
		if code, _ := e.do(t, "alice", "POST", "/api/tasks/t-nope/priority-hint", `{"rank":1}`); code != http.StatusNotFound {
			t.Fatalf("status %d, want 404", code)
		}
	})
}

// TestPriorityHintOnAStaleBoardIsAnHonestNoOp: the run moved on between the
// render and the drag. That is not an error — an error would imply the drag had
// an authority over a run it no longer orders.
func TestPriorityHintOnAStaleBoardIsAnHonestNoOp(t *testing.T) {
	e := newVerbEnv(t) // nothing queued
	seedTask(t, e.b, "t-alice", "alice", "Alice Task", "doing")
	seedRun(t, e.b, "r-alice", "alice", "t-alice", "running", "lane-a")

	out := e.mustDo(t, "alice", "POST", "/api/tasks/t-alice/priority-hint", `{"rank":-3}`)
	var h api.PriorityHint
	if err := json.Unmarshal([]byte(out), &h); err != nil {
		t.Fatalf("decode: %v: %s", err, out)
	}
	if h.Applied || len(h.Runs) != 0 {
		t.Fatalf("hint outcome = %+v, want an unapplied no-op", h)
	}
	if !strings.Contains(h.Detail, "stale") {
		t.Errorf("the no-op does not say the board is stale: %q", h.Detail)
	}
	// It is still logged: the act happened, and the record says it changed nothing.
	rows := decisionRows(t, e.b, "priority_hint")
	if len(rows) != 1 {
		t.Fatalf("recorded %d hint decisions, want 1", len(rows))
	}
	newSide, _ := json.Marshal(rows[0]["new"])
	if !strings.Contains(string(newSide), `"applied":false`) {
		t.Errorf("the audit row does not record that nothing was applied: %s", newSide)
	}
}

func TestPriorityHintRankIsBounded(t *testing.T) {
	e := newVerbEnv(t, "r-alice")
	seedTask(t, e.b, "t-alice", "alice", "Alice Task", "doing")
	seedRun(t, e.b, "r-alice", "alice", "t-alice", "queued", "lane-a")
	for _, body := range []string{`{"rank":100000}`, `{"rank":-100000}`} {
		if code, out := e.do(t, "alice", "POST", "/api/tasks/t-alice/priority-hint", body); code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400: %s", body, code, out)
		}
	}
	if _, hinted := e.hints.rank("r-alice"); hinted {
		t.Error("an out-of-range rank was still written")
	}
}

// ── the not-wired posture ──────────────────────────────────────────────────

func TestMetersVerbsNotWiredAre503(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "alice", "member")
	seedTask(t, b, "t-1", "alice", "T", "doing")
	srv := api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{"alice"},
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       b.db,
	})
	for _, route := range []struct{ path, body string }{
		{"/api/meters/budget", `{"lane":"anthropic","period_tokens":1,"period_days":1}`},
		{"/api/meters/pause", `{"paused":true}`},
		{"/api/tasks/t-1/priority-hint", `{"rank":0}`},
	} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest("POST", route.path, strings.NewReader(route.body)))
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("POST %s: status %d, want 503 when the seam is not wired", route.path, rr.Code)
		}
	}
}

// TestBothSwitchesRefuseAPersonWhoDoesNotExist is drain D5/D6: `person` is
// client input, and whether it names somebody is a question about that input —
// so it is answered at the boundary, and answered 404. Before this an operator
// typo reached the store, whose own refusal the transport could only render as
// a 500: a caller's mistake surfacing as a platform fault.
func TestBothSwitchesRefuseAPersonWhoDoesNotExist(t *testing.T) {
	e := newVerbEnv(t)
	for _, tc := range []struct{ name, path, body string }{
		{"budget", "/api/meters/budget", `{"person":"ghost","lane":"anthropic","period_tokens":10,"period_days":7}`},
		{"pause", "/api/meters/pause", `{"person":"ghost","paused":true}`},
	} {
		code, out := e.do(t, "op", "POST", tc.path, tc.body)
		if code != http.StatusNotFound {
			t.Errorf("%s for a nonexistent person: status %d, want 404: %s", tc.name, code, out)
		}
	}
	if len(e.budgets.rows) != 0 {
		t.Error("a refused declaration still wrote a budget row")
	}
	if e.pause.paused["ghost"] {
		t.Error("a refused pause still flipped a switch")
	}
	if n := len(decisionRows(t, e.b, "")); n != 0 {
		t.Errorf("a refused switch recorded %d decisions", n)
	}
	// The non-tautological direction: a real person still works.
	e.mustDo(t, "op", "POST", "/api/meters/pause", `{"person":"bob","paused":true}`)
}

// ── counters (rubric 20) ───────────────────────────────────────────────────

// TestPartBCountersArePinned holds the tally this packet is allowed to move and
// the ones it is not. The ⚙ registry is byte-unchanged (118/33), the adoption
// lock is unchanged (21), and exactly ONE migration is added — 0017 — with
// user_version following it.
func TestPartBCountersArePinned(t *testing.T) {
	reg := settings.New()
	decls := reg.Decls()
	if len(decls) != 118 {
		t.Errorf("⚙ index has %d keys, want 118 — B6-2B adds no key (R24)", len(decls))
	}
	domains := map[string]bool{}
	for _, d := range decls {
		domains[d.Domain()] = true
	}
	if len(domains) != 33 {
		t.Errorf("⚙ index has %d domains, want 33", len(domains))
	}

	b := newBackend(t)
	v, err := b.db.UserVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// 0017 is part B's; part C landed 0018 (the BENCH-REG §2 direct-arm capture
	// column) and B6-3A landed 0019 (the S10.3 price table's durable home), so
	// the pin moves in lockstep with the migration that moved it — 0001–0017
	// stay byte-untouched.
	if v != 21 {
		t.Errorf("user_version = %d, want 21 (0001–0017 untouched; 0017 is part B's, 0018 part C's, 0019 B6-3A's, 0020 B6-7's, 0021 B6-9's)", v)
	}
	// The view the migration recreated must actually exist and be selectable —
	// a DROP without a matching CREATE would otherwise pass every other test in
	// this file that does not read it.
	var n int
	if err := b.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM cost_budget_remainder`).Scan(&n); err != nil {
		t.Fatalf("cost_budget_remainder is not selectable after the recreate: %v", err)
	}
}

// ── §30: no percent, fraction, ratio or ETA in any part-B shape ────────────

func TestPartBShapesNeverPercent(t *testing.T) {
	e := newVerbEnv(t, "r-alice")
	seedTask(t, e.b, "t-alice", "alice", "Alice Task", "doing")
	seedRun(t, e.b, "r-alice", "alice", "t-alice", "queued", "lane-a")

	// The oversight fixtures, so the three card-verb shapes are scanned too
	// (drain D7).
	seedFlag(t, e.b, "alice", "r-alice", "watchdog.loop", "flag-now")
	seedFindingAt(t, e.b, "fp-1", "STORM", time.Now())
	seedRedConformanceRow(t, e.b, "CONF-ROW", "lane", nowTS())

	bodies := []string{
		e.mustDo(t, "alice", "POST", "/api/meters/budget", `{"lane":"anthropic","period_tokens":10,"period_days":7}`),
		e.mustDo(t, "alice", "POST", "/api/meters/pause", `{"paused":true}`),
		e.mustDo(t, "alice", "POST", "/api/tasks/t-alice/priority-hint", `{"rank":-1}`),
		e.mustDo(t, "alice", "POST", "/api/watchdog/flags/suppress",
			`{"run_id":"r-alice","anomaly_class":"watchdog.loop"}`),
		e.mustDo(t, "op", "POST", "/api/approvals/drift_card:fp-1/dismiss", `{}`),
		e.mustDo(t, "op", "POST", "/api/approvals/conformance_card:CONF-ROW/acknowledge", `{}`),
	}
	keys := map[string]bool{}
	for _, body := range bodies {
		var v any
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Fatalf("decode: %v: %s", err, body)
		}
		collectKeys(v, keys)
	}
	if len(keys) == 0 {
		t.Fatal("collected no keys — the scan proves nothing")
	}
	forbidden := map[string]bool{
		"percent": true, "percentage": true, "percent_complete": true,
		"fraction": true, "complete_fraction": true, "ratio": true,
		"progress": true, "pct": true, "complete": true, "completion": true,
		"eta": true, "eta_s": true, "eta_seconds": true,
	}
	for k := range keys {
		lk := strings.ToLower(k)
		if forbidden[lk] || strings.Contains(lk, "percent") {
			t.Fatalf("a B6-2B shape exposes forbidden progress key %q (§30)", k)
		}
	}
}

// nowMinus is used by the oversight battery for aged fixtures.
func nowMinus(d time.Duration) string {
	return time.Now().Add(-d).UTC().Format(time.RFC3339Nano)
}

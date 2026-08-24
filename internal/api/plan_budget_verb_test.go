package api_test

// plan_budget_verb_test.go — P3-LN-6 (S10.4, S15.2, S18.3): POST
// /api/meters/plan-budget, the declaration verb for a lane whose plan meters in
// its OWN units.
//
// It is the sibling of the token budget verb and holds the same three
// properties: own+operator-any authority, validation at the boundary that
// admits the input (§30), and an old→new audit row that lands AFTER the store
// act. What it adds is the WINDOW — a plan's windows can be denominated
// differently, so the grain is (person, lane, window) and an unknown window
// name is a 400 that says which windows exist.
//
// NO DOLLARS anywhere: there is no dollar member on the body, the record or the
// response, and a request that carries one has nothing to bind it to (D5).

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
)

// ── the fake store ──────────────────────────────────────────────────────────

// fakePlanBudgetStore stands in for the landed store at the transport boundary.
// It is thin on purpose: what is under test here is SCOPE, VALIDATION and the
// AUDIT obligation. The store's real behavior is asserted in internal/metering
// and the real consumption in internal/shell.
//
// Its windows are the shipped plan documents' own shapes, because the boundary
// check under test is "is this window one the lane actually declares" and a
// made-up window set would test nothing about that.
type fakePlanBudgetStore struct {
	mu   sync.Mutex
	rows map[string]api.PlanBudgetRecord
	err  error
}

func newFakePlanBudgetStore() *fakePlanBudgetStore {
	return &fakePlanBudgetStore{rows: map[string]api.PlanBudgetRecord{}}
}

func (f *fakePlanBudgetStore) DeclarePlanBudget(_ context.Context, rec api.PlanBudgetRecord) (api.PlanBudgetRecord, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return api.PlanBudgetRecord{}, false, f.err
	}
	key := rec.Owner + "/" + rec.Lane + "/" + rec.Window
	prior, existed := f.rows[key]
	f.rows[key] = rec
	return prior, existed, nil
}

func (f *fakePlanBudgetStore) PlanBudget(_ context.Context, userID, lane, window string) (api.PlanBudgetRecord, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.rows[userID+"/"+lane+"/"+window]
	return rec, ok, nil
}

func (f *fakePlanBudgetStore) PlanWindows(_ context.Context, lane string) ([]api.PlanWindowRecord, error) {
	switch lane {
	case "zai":
		return []api.PlanWindowRecord{
			{Name: "rolling-5h", Unit: "credits", WindowHours: 5, Allowance: 28000},
			{Name: "weekly", Unit: "credits", WindowHours: 168, Allowance: 140000},
		}, nil
	case "kimi":
		return []api.PlanWindowRecord{
			{Name: "rolling-5h", Unit: "requests", WindowHours: 5, Allowance: 300},
			{Name: "weekly", Unit: "credits", WindowHours: 168, AllowanceUnverified: true},
		}, nil
	}
	return nil, nil
}

func (f *fakePlanBudgetStore) ProposePlanBudget(ctx context.Context, userID, lane, window string, start time.Time) (api.PlanBudgetRecord, error) {
	windows, _ := f.PlanWindows(ctx, lane)
	for _, w := range windows {
		if w.Name != window {
			continue
		}
		return api.PlanBudgetRecord{
			Owner: userID, Lane: lane, Window: window,
			PeriodUnits: w.Allowance * 0.5, Unit: w.Unit,
			PeriodStart: start, PeriodHours: w.WindowHours,
			Source: "proposal-seeded", SeededFrom: window, Fraction: 0.5,
		}, nil
	}
	return api.PlanBudgetRecord{}, errors.New("no such window")
}

func (f *fakePlanBudgetStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows)
}

// planVerbEnv composes the same decision-plane server the other verb batteries
// use, with the plan-budget seam wired.
type planVerbEnv struct {
	*decisionEnv
	planBudgets *fakePlanBudgetStore
	// wired false composes the server WITHOUT the seam, which is the
	// pre-migration process: the route must answer 503 rather than pretend.
	wired bool
}

func newPlanVerbEnv(t *testing.T) *planVerbEnv {
	t.Helper()
	return &planVerbEnv{decisionEnv: newDecisionEnv(t), planBudgets: newFakePlanBudgetStore(), wired: true}
}

func (e *planVerbEnv) server(t *testing.T, who string) *api.Server {
	t.Helper()
	cfg := api.Config{
		Log: e.b.log, Sessions: e.b.store, Auth: fixedIdentity{who},
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       e.b.db, Meter: fakeMeter{},
		Intake: e.surface, Effects: e.journal, Cancel: e.cancel,
	}
	if e.wired {
		cfg.PlanBudgets = e.planBudgets
	}
	return api.New(cfg)
}

func (e *planVerbEnv) do(t *testing.T, who, body string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	e.server(t, who).Handler().ServeHTTP(rr,
		httptest.NewRequest("POST", "/api/meters/plan-budget", strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

func (e *planVerbEnv) mustDo(t *testing.T, who, body string) string {
	t.Helper()
	code, out := e.do(t, who, body)
	if code != http.StatusOK {
		t.Fatalf("POST /api/meters/plan-budget as %s: status %d: %s", who, code, out)
	}
	return out
}

// ── T9: authority + validation ──────────────────────────────────────────────

func TestLN6PlanBudgetVerbAuthorityAndValidation(t *testing.T) {
	// An unwired process says so, rather than answering as if nothing was
	// declared.
	t.Run("nil store answers 503", func(t *testing.T) {
		e := newPlanVerbEnv(t)
		e.wired = false
		code, out := e.do(t, "alice", `{"lane":"zai","window":"rolling-5h","period_units":100,"period_hours":5}`)
		if code != http.StatusServiceUnavailable {
			t.Fatalf("status %d, want 503: %s", code, out)
		}
		if !strings.Contains(out, "not_wired") {
			t.Errorf("the 503 does not carry the not_wired code: %s", out)
		}
	})

	for _, tc := range []struct {
		name, who, person string
		want              int
	}{
		{"a member declares their own", "alice", "", http.StatusOK},
		{"a member declares their own by name", "alice", "alice", http.StatusOK},
		{"a member cannot declare another's", "alice", "bob", http.StatusForbidden},
		{"the operator may declare anybody's (D10)", "op", "bob", http.StatusOK},
		{"a person who does not exist", "op", "nobody", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newPlanVerbEnv(t)
			body := `{"lane":"zai","window":"rolling-5h","period_units":14000,"period_hours":5`
			if tc.person != "" {
				body += `,"person":"` + tc.person + `"`
			}
			body += `}`
			code, out := e.do(t, tc.who, body)
			if code != tc.want {
				t.Fatalf("status %d, want %d: %s", code, tc.want, out)
			}
			if tc.want == http.StatusForbidden && (!strings.Contains(out, "S15.2") || !strings.Contains(out, "D10")) {
				t.Errorf("the refusal does not name the authority it rests on: %s", out)
			}
			if tc.want != http.StatusOK {
				if e.planBudgets.count() != 0 {
					t.Error("a refused declaration still wrote a row")
				}
				if len(decisionRows(t, e.b, "plan_budget")) != 0 {
					t.Error("a refused declaration still recorded a decision")
				}
			}
		})
	}

	// The operator's administered declaration records the OPERATOR as the actor,
	// so an administered budget is never anonymous.
	t.Run("the operator's declaration names the operator", func(t *testing.T) {
		e := newPlanVerbEnv(t)
		out := e.mustDo(t, "op", `{"person":"bob","lane":"zai","window":"rolling-5h","period_units":14000,"period_hours":5}`)
		var got api.PlanBudgetDeclared
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("decode: %v: %s", err, out)
		}
		if got.Budget.Owner != "bob" || got.Budget.DeclaredBy != "op" {
			t.Errorf("record = %+v, want bob's budget declared by op", got.Budget)
		}
		if got.Budget.Unit != "credits" {
			t.Errorf("unit = %q, want the WINDOW's own unit — the verb never guesses a unit", got.Budget.Unit)
		}
		if got.Budget.Source != "operator-set" {
			t.Errorf("source = %q, want operator-set for a hand-declared figure", got.Budget.Source)
		}
	})

	t.Run("validation refuses what is not a plan budget", func(t *testing.T) {
		e := newPlanVerbEnv(t)
		for _, tc := range []struct{ name, body, says string }{
			{"no lane", `{"window":"rolling-5h","period_units":10,"period_hours":5}`, "lane"},
			{"no window", `{"lane":"zai","period_units":10,"period_hours":5}`, "window"},
			{"zero units", `{"lane":"zai","window":"rolling-5h","period_units":0,"period_hours":5}`, "period_units"},
			{"negative units", `{"lane":"zai","window":"rolling-5h","period_units":-4,"period_hours":5}`, "period_units"},
			{"no period length", `{"lane":"zai","window":"rolling-5h","period_units":10}`, "period_hours"},
			{"negative period length", `{"lane":"zai","window":"rolling-5h","period_units":10,"period_hours":-5}`, "period_hours"},
			{"unknown window", `{"lane":"zai","window":"fortnightly","period_units":10,"period_hours":5}`, "rolling-5h"},
			{"a lane with no plan", `{"lane":"anthropic","window":"rolling-5h","period_units":10,"period_hours":5}`, "anthropic"},
			{"bad period start", `{"lane":"zai","window":"rolling-5h","period_units":10,"period_hours":5,"period_start":"yesterday"}`, "period_start"},
			// A dollar-shaped member has nothing to bind to: the request is
			// refused for want of a plan-unit figure, and no dollar value is
			// stored, echoed or computed (D5).
			{"a dollar-shaped body", `{"lane":"zai","window":"rolling-5h","period_usd":25,"period_hours":5}`, "period_units"},
		} {
			code, out := e.do(t, "alice", tc.body)
			if code != http.StatusBadRequest {
				t.Errorf("%s: status %d, want 400: %s", tc.name, code, out)
				continue
			}
			if !strings.Contains(out, tc.says) {
				t.Errorf("%s: the refusal does not say what is wrong (want it to mention %q): %s", tc.name, tc.says, out)
			}
			if strings.Contains(out, "25") {
				t.Errorf("%s: the refusal echoed a dollar figure back: %s", tc.name, out)
			}
		}
		if e.planBudgets.count() != 0 {
			t.Error("a refused request still wrote a row")
		}
	})
}

// ── T10: the audit is old→new, and it lands AFTER the store act ─────────────

func TestLN6PlanBudgetVerbRecordsOldToNew(t *testing.T) {
	e := newPlanVerbEnv(t)
	e.mustDo(t, "alice", `{"lane":"zai","window":"rolling-5h","period_units":14000,"period_hours":5}`)
	out := e.mustDo(t, "op",
		`{"person":"alice","lane":"zai","window":"rolling-5h","period_units":9000,"period_hours":5,"reason":"trimmed for the week"}`)

	var declared api.PlanBudgetDeclared
	if err := json.Unmarshal([]byte(out), &declared); err != nil {
		t.Fatalf("decode: %v: %s", err, out)
	}
	if declared.Budget.PeriodUnits != 9000 {
		t.Fatalf("response budget = %+v, want the 9000-unit declaration", declared.Budget)
	}
	if declared.Prior == nil || declared.Prior.PeriodUnits != 14000 {
		t.Fatalf("response prior = %+v, want the 14000-unit budget it replaced", declared.Prior)
	}

	rows := decisionRows(t, e.b, "plan_budget")
	if len(rows) != 2 {
		t.Fatalf("recorded %d plan-budget decisions, want 2", len(rows))
	}
	firstOld, _ := json.Marshal(rows[0]["old"])
	if !strings.Contains(string(firstOld), `"declared":false`) {
		t.Errorf("the first declaration's old side = %s, want the explicit undeclared object — an absence is stated, never a missing member", firstOld)
	}
	second := rows[1]
	if second["actor"] != "op" || second["actor_is_operator"] != true {
		t.Errorf("the audit row does not name the operator as actor: %v", second)
	}
	oldSide, _ := json.Marshal(second["old"])
	newSide, _ := json.Marshal(second["new"])
	if !strings.Contains(string(oldSide), "14000") || !strings.Contains(string(newSide), "9000") {
		t.Errorf("the audit row does not carry old→new: old=%s new=%s", oldSide, newSide)
	}
	if !strings.Contains(string(newSide), "rolling-5h") {
		t.Errorf("the audit row does not name the window the budget denominates: %s", newSide)
	}
	if strings.Contains(string(newSide), "usd") {
		t.Errorf("the audit row carries a dollar member: %s", newSide)
	}

	// A store that refuses produces NO decision row: a record of an edit that
	// never committed would be a record of something that did not happen.
	e.planBudgets.err = errors.New("disk went away")
	if code, _ := e.do(t, "alice", `{"lane":"zai","window":"weekly","period_units":70000,"period_hours":168}`); code == http.StatusOK {
		t.Fatal("a failing store still answered 200")
	}
	if got := len(decisionRows(t, e.b, "plan_budget")); got != 2 {
		t.Errorf("recorded %d decisions after a failed store act, want the 2 that committed", got)
	}
}

// ── T11 (transport half): the proposal path is the production consumer ──────

func TestLN6ProposalPathIsTheProductionConsumer(t *testing.T) {
	e := newPlanVerbEnv(t)
	out := e.mustDo(t, "alice", `{"lane":"zai","window":"rolling-5h","from_proposal":true}`)
	var got api.PlanBudgetDeclared
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode: %v: %s", err, out)
	}
	if got.Budget.Source != "proposal-seeded" {
		t.Errorf("source = %q, want proposal-seeded", got.Budget.Source)
	}
	if got.Budget.SeededFrom != "rolling-5h" || got.Budget.Fraction <= 0 {
		t.Errorf("provenance = %q/%v, want the window it was seeded from and a non-zero share",
			got.Budget.SeededFrom, got.Budget.Fraction)
	}
	if got.Budget.PeriodUnits <= 0 || got.Budget.PeriodHours <= 0 {
		t.Errorf("the proposal stored %v units over %vh — a seeded row carries the window's own figures",
			got.Budget.PeriodUnits, got.Budget.PeriodHours)
	}
	if got.Budget.DeclaredBy != "alice" {
		t.Errorf("declared_by = %q — a proposal is still declared BY somebody", got.Budget.DeclaredBy)
	}

	// A window whose allowance nobody published cannot be proposed from, and
	// that is the caller's answer (400), never a platform fault (500).
	code, body := e.do(t, "alice", `{"lane":"kimi","window":"weekly","from_proposal":true}`)
	if code != http.StatusBadRequest {
		t.Fatalf("proposing from an unverified allowance: status %d, want 400: %s", code, body)
	}
	if !strings.Contains(body, "allowance") {
		t.Errorf("the refusal does not name the unverified allowance: %s", body)
	}
	if e.planBudgets.count() != 1 {
		t.Errorf("the refused proposal wrote a row (store holds %d)", e.planBudgets.count())
	}
}

// ── T13: the wire delta is additive and the fixture exercises it ────────────

func TestLN6WireDeltaIsAdditive(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(fixtureDir, "meters.json"))
	if err != nil {
		t.Fatalf("read the committed meters body: %v", err)
	}
	var body struct {
		Lanes []struct {
			Lane string           `json:"lane"`
			Plan *json.RawMessage `json:"plan"`
		} `json:"lanes"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode the committed meters body: %v", err)
	}

	declared, undeclared := 0, 0
	for _, lane := range body.Lanes {
		if lane.Plan == nil {
			continue
		}
		var plan struct {
			Pressure *float64             `json:"pressure"`
			Budget   *api.MeterPlanBudget `json:"budget"`
			// The pre-existing members: every one must still be present, or the
			// SPA side was built against a contract that moved.
			Unit           *string          `json:"unit"`
			Tier           *int             `json:"tier"`
			Assumed        *bool            `json:"assumed"`
			Consumed       *float64         `json:"consumed"`
			Calls          *int64           `json:"calls"`
			Multiplier     *float64         `json:"multiplier"`
			BudgetDeclared *bool            `json:"budget_declared"`
			Windows        *json.RawMessage `json:"windows"`
		}
		if err := json.Unmarshal(*lane.Plan, &plan); err != nil {
			t.Fatalf("decode the %s plan block: %v", lane.Lane, err)
		}
		// The plan block is denominated in the plan's OWN unit end to end. The
		// Layer-0 view rows elsewhere in this body legitimately carry dollar
		// COLUMN NAMES (all null, by D5), so the scan is scoped to the block.
		if strings.Contains(strings.ToLower(string(*lane.Plan)), "usd") {
			t.Errorf("lane %s: the plan block carries a dollar member — a flat-rate lane has no dollar budget (D5): %s",
				lane.Lane, *lane.Plan)
		}
		for name, present := range map[string]bool{
			"unit": plan.Unit != nil, "tier": plan.Tier != nil, "assumed": plan.Assumed != nil,
			"consumed": plan.Consumed != nil, "calls": plan.Calls != nil,
			"multiplier": plan.Multiplier != nil, "budget_declared": plan.BudgetDeclared != nil,
			"windows": plan.Windows != nil,
		} {
			if !present {
				t.Errorf("lane %s: the plan block lost its %q member — this delta is ADDITIVE", lane.Lane, name)
			}
		}
		if plan.Budget == nil {
			if plan.Pressure != nil {
				t.Errorf("lane %s serves a pressure with no declared budget — the denominator is never the provider's allowance (D4)", lane.Lane)
			}
			undeclared++
			continue
		}
		declared++
		if plan.Pressure == nil {
			t.Errorf("lane %s carries a declared budget and no pressure — a declared denominator is what makes the ratio exist", lane.Lane)
		}
		if plan.Budget.PeriodUnits <= 0 || plan.Budget.Unit == "" || plan.Budget.Window == "" {
			t.Errorf("lane %s: the budget object is not populated: %+v", lane.Lane, plan.Budget)
		}
		if plan.Budget.Source == "" || plan.Budget.DeclaredBy == "" {
			t.Errorf("lane %s: the budget object carries no provenance: %+v", lane.Lane, plan.Budget)
		}
	}
	if declared == 0 {
		t.Error("no fixture lane exercises the plan `budget` member — a wire member no fixture exercises is a contract nobody agreed to (§63 R3)")
	}
	if undeclared == 0 {
		t.Error("every fixture plan block is declared — the UNDECLARED shape is the one this platform serves today and it must stay exercised")
	}
}

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
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
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

// PlanWindows serves the SHIPPED documents' own windows, with the budgetable
// verdict taken from the production predicate rather than hand-spelled here
// (drain r2 R6). A fake that re-states the rule tests its own copy of it, which
// is the twin-maintained hazard the seam exists to avoid; this one is the
// shell adapter's logic over the real data.
//
// The synthetic window below is the one exception and is named as such.
func (f *fakePlanBudgetStore) PlanWindows(_ context.Context, lane string) ([]api.PlanWindowRecord, error) {
	doc, ok := metering.PlanDocFor(lane)
	if !ok {
		return nil, nil
	}
	out := make([]api.PlanWindowRecord, 0, len(doc.Quotas)+1)
	for _, q := range doc.Quotas {
		refusal := metering.PlanBudgetWindowRefusal(doc, q.Name)
		out = append(out, api.PlanWindowRecord{
			Name: q.Name, Unit: doc.QuotaUnit(q.Name), WindowHours: q.WindowHours,
			Allowance: q.Units, AllowanceUnverified: q.AllowanceUnverified,
			Budgetable: refusal == "", NotBudgetable: refusal,
		})
	}
	if lane == synthLane {
		out = append(out, synthUnverifiedWindow)
	}
	return out, nil
}

// synthUnverifiedWindow is a SYNTHETIC window: budgetable (it counts what the
// lane's consumption counts) with an allowance nobody published.
//
// It is synthetic because NO SHIPPED DOCUMENT can reach that combination today,
// and TestLN6NoShippedDocumentReachesTheProposalRefusal asserts exactly that so
// the fiction cannot outlive its reason. The one document window whose
// allowance is unverified is denominated in something else, so the budgetable
// check refuses it first and the proposal refusal is never consulted. The leg
// is kept rather than deleted because a future plan document may publish a
// window in its own unit with no figure, and the boundary must answer that with
// a 400 rather than let it fall through to a 500.
var synthUnverifiedWindow = api.PlanWindowRecord{
	Name: "synthetic-unverified", Unit: "credits", WindowHours: 720,
	AllowanceUnverified: true, Budgetable: true,
}

const synthLane = "zai"

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
		// drain r2 R4: the confirmation SAYS when this stops applying. For the
		// shortest window that is hours away and it is by design — a lane
		// silently falling off pressure routing five hours after a proposal was
		// accepted is the surprise this sentence exists to prevent.
		for _, must := range []string{"ends at", "stops applying", "declare the next period", "5 hours"} {
			if !strings.Contains(got.Detail, must) {
				t.Errorf("the confirmation does not say %q — it reads as though the budget applies indefinitely: %q", must, got.Detail)
			}
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
			// drain r1 D1: the window EXISTS and still cannot be a denominator.
			{"a window counting something else", `{"lane":"kimi","window":"weekly","period_units":10,"period_hours":168}`, "credits"},
			{"a lane with no plan", `{"lane":"anthropic","window":"rolling-5h","period_units":10,"period_hours":5}`, "anthropic"},
			{"bad period start", `{"lane":"zai","window":"rolling-5h","period_units":10,"period_hours":5,"period_start":"yesterday"}`, "period_start"},
			// A dollar-shaped member has nothing to bind to: the request is
			// refused for want of a plan-unit figure, and no dollar value is
			// stored, echoed or computed (D5).
			{"a dollar-shaped body", `{"lane":"zai","window":"rolling-5h","period_usd":25,"period_hours":5}`, "period_units"},
			// drain r2 R4: a period that has already elapsed is dead on
			// arrival — the reading refuses it the instant it is stored, so a
			// 200 would confirm a budget that denominates nothing.
			{"an already-elapsed period", `{"lane":"zai","window":"rolling-5h","period_units":10,"period_hours":5,"period_start":"2026-07-20T09:00:00Z"}`, "already over"},
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
	code, body := e.do(t, "alice", `{"lane":"zai","window":"synthetic-unverified","from_proposal":true}`)
	if code != http.StatusBadRequest {
		t.Fatalf("proposing from an unverified allowance: status %d, want 400: %s", code, body)
	}
	if !strings.Contains(body, "allowance") {
		t.Errorf("the refusal does not name the unverified allowance: %s", body)
	}

	// And a window that cannot carry a budget at all is refused BEFORE the
	// proposal is ever asked for — the categorical answer comes first
	// (drain r1 D1).
	code, body = e.do(t, "alice", `{"lane":"kimi","window":"weekly","from_proposal":true}`)
	if code != http.StatusBadRequest {
		t.Fatalf("proposing on an incoherent window: status %d, want 400: %s", code, body)
	}
	if !strings.Contains(body, "credits") || !strings.Contains(body, "requests") {
		t.Errorf("the refusal does not name the two units it would have divided: %s", body)
	}

	if e.planBudgets.count() != 1 {
		t.Errorf("a refused proposal wrote a row (store holds %d)", e.planBudgets.count())
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

	declared, undeclared, noted := 0, 0, 0
	for _, lane := range body.Lanes {
		if lane.Plan == nil {
			continue
		}
		var plan struct {
			Pressure         *float64             `json:"pressure"`
			Budget           *api.MeterPlanBudget `json:"budget"`
			InapplicableNote string               `json:"inapplicable_note"`
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
		// THE COHERENT TRIPLE on the wire (drain r2 R2/R3): a served
		// declaration carries either the ratio it produced or the reason it
		// produced none. Never neither — that is a refusal a reader cannot
		// see — and never both.
		if plan.Pressure == nil && plan.InapplicableNote == "" {
			t.Errorf("lane %s serves a declared budget with neither a pressure nor a reason it has none", lane.Lane)
		}
		if plan.Pressure != nil && plan.InapplicableNote != "" {
			t.Errorf("lane %s serves a pressure AND a reason it has none: %q", lane.Lane, plan.InapplicableNote)
		}
		if plan.Budget.PeriodUnits <= 0 || plan.Budget.Unit == "" || plan.Budget.Window == "" {
			t.Errorf("lane %s: the budget object is not populated: %+v", lane.Lane, plan.Budget)
		}
		if plan.Budget.Source == "" || plan.Budget.DeclaredBy == "" {
			t.Errorf("lane %s: the budget object carries no provenance: %+v", lane.Lane, plan.Budget)
		}
		if plan.InapplicableNote != "" {
			noted++
		}
	}
	if declared == 0 {
		t.Error("no fixture lane exercises the plan `budget` member — a wire member no fixture exercises is a contract nobody agreed to (§63 R3)")
	}
	if noted == 0 {
		t.Error("no fixture lane exercises `inapplicable_note` — the state it explains is the one a fixed-clock world can " +
			"honestly depict, and a member no fixture exercises is a contract nobody agreed to (§63 R3)")
	}
	if undeclared == 0 {
		t.Error("every fixture plan block is declared — the UNDECLARED shape is the one this platform serves today and it must stay exercised")
	}
}

// ── D3: the §30 never-percent negative, over every shape this packet adds ───

// TestLN6PlanBudgetShapesNeverPercent extends the never-percent scan
// (CONVENTIONS §30, extended the same way at §38/§40/§43/§45) over the shapes
// P3-LN-6 ships: the verb's request and response, the transport record, and the
// meters plan-budget block.
//
// It is not decoration. The plan budget carries a SEEDING SHARE — what fraction
// of a published allowance a proposal took — and the obvious name for it is in
// the forbidden set by name. A member called `fraction` on a read shape is a
// progress key to every other scan in this codebase, so it is `seed_share`, and
// this test is what keeps it that way.
func TestLN6PlanBudgetShapesNeverPercent(t *testing.T) {
	forbidden := map[string]bool{
		"percent": true, "percentage": true, "percent_complete": true,
		"fraction": true, "complete_fraction": true, "ratio": true,
		"progress": true, "pct": true, "complete": true, "completion": true,
		"eta": true, "eta_s": true, "eta_seconds": true,
	}

	e := newPlanVerbEnv(t)
	bodies := []string{
		e.mustDo(t, "alice", `{"lane":"zai","window":"rolling-5h","period_units":14000,"period_hours":5}`),
		e.mustDo(t, "alice", `{"lane":"zai","window":"rolling-5h","from_proposal":true}`),
	}
	// The meters plan-budget block as it is actually served, and the seam
	// record the verb speaks — marshalled populated, so an omitempty member
	// cannot hide from the walk.
	populated, err := json.Marshal(struct {
		Budget api.MeterPlanBudget  `json:"budget"`
		Record api.PlanBudgetRecord `json:"record"`
		Window api.PlanWindowRecord `json:"window"`
	}{
		Budget: api.MeterPlanBudget{PeriodUnits: 1, Unit: "credits", Window: "rolling-5h",
			PeriodStart: "t", PeriodHours: 5, Source: "operator-set", SeededFrom: "rolling-5h",
			Fraction: 0.5, DeclaredBy: "alice", DeclaredTS: "t"},
		Record: api.PlanBudgetRecord{Owner: "alice", Lane: "zai", Window: "rolling-5h",
			PeriodUnits: 1, Unit: "credits", PeriodHours: 5, Source: "operator-set",
			SeededFrom: "rolling-5h", Fraction: 0.5, DeclaredBy: "alice"},
		Window: api.PlanWindowRecord{Name: "rolling-5h", Unit: "credits", WindowHours: 5,
			Allowance: 1, AllowanceUnverified: true, Budgetable: true, NotBudgetable: "why"},
	})
	if err != nil {
		t.Fatalf("marshal the packet's shapes: %v", err)
	}
	// The REQUEST shape too: a body member named `fraction` would be a progress
	// key an operator could send.
	requestShape, err := json.Marshal(map[string]any{
		"person": "", "lane": "", "window": "", "period_units": 0.0,
		"period_hours": 0.0, "period_start": "", "from_proposal": false, "reason": "",
	})
	if err != nil {
		t.Fatalf("marshal the request shape: %v", err)
	}
	bodies = append(bodies, string(populated), string(requestShape))

	checked := 0
	for _, body := range bodies {
		var v any
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Fatalf("decode: %v: %s", err, body)
		}
		var walk func(any)
		walk = func(n any) {
			switch x := n.(type) {
			case map[string]any:
				for k, sub := range x {
					checked++
					lk := strings.ToLower(k)
					if forbidden[lk] || strings.Contains(lk, "percent") {
						t.Errorf("a P3-LN-6 shape exposes forbidden progress key %q (§30)", k)
					}
					walk(sub)
				}
			case []any:
				for _, sub := range x {
					walk(sub)
				}
			}
		}
		walk(v)
	}
	if checked == 0 {
		t.Fatal("the never-percent walk checked nothing — it would pass vacuously")
	}
	if !forbidden["fraction"] {
		t.Fatal("the never-percent walk cannot detect its own probe")
	}
}

// TestLN6NoShippedDocumentReachesTheProposalRefusal is the honesty check on the
// synthetic window above (drain r2 R6).
//
// The verb's unverified-allowance refusal is a REAL branch — a plan may publish
// a window's shape without its figure — but on the documents that ship today it
// is unreachable: the only window with an unverified allowance is denominated
// in something other than what its lane counts, so the budgetable check refuses
// it first and the proposal refusal is never consulted. The test that exercises
// that branch therefore drives a synthetic window, and this pins WHY. The day a
// shipped document publishes a window in its own unit with no figure, this goes
// red and the synthetic one can be deleted.
func TestLN6NoShippedDocumentReachesTheProposalRefusal(t *testing.T) {
	docs, err := metering.SeedPlanDocs()
	if err != nil {
		t.Fatalf("SeedPlanDocs: %v", err)
	}
	checked := 0
	for _, doc := range docs {
		for _, q := range doc.Quotas {
			checked++
			if !q.AllowanceUnverified {
				continue
			}
			if metering.PlanBudgetWindowRefusal(doc, q.Name) == "" {
				t.Errorf("%s/%s is budgetable AND publishes no allowance — the verb's proposal refusal is now reachable "+
					"on a SHIPPED document, so exercise it with that window and delete the synthetic one", doc.Lane, q.Name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("the walk read no quota rows — it would pass vacuously")
	}
}

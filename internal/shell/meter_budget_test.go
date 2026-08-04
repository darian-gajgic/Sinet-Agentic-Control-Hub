package shell

// meter_budget_test.go — drain D2: the S10.4 budget verb and the meters READ
// surface must agree, over the REAL shell adapters (budgetAdapter, projMeter).
//
// This is the level the package-level tests could not see. internal/metering
// proved the store round-trips and internal/api proved the verb is scoped and
// audited, but neither could see that the shell handed the gauge
// UndeclaredBudget() unconditionally — so `GET /api/meters` contradicted itself
// after a successful declare: the Layer-0 view block reported a declared budget
// while the gauge block beside it reported none and served no pressure.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// fixedShellIdentity authenticates every request as one person.
type fixedShellIdentity struct{ id string }

func (f fixedShellIdentity) Authenticate(*http.Request) (api.Identity, error) {
	return api.Identity{UserID: f.id}, nil
}

// meterEnv is the real composition of the two seams under test, in front of a
// real HTTP handler over a real migrated DB. $0: no engine, no network.
type meterEnv struct {
	db  *storage.DB
	log *eventlog.Log
	srv *api.Server
}

func newMeterEnv(t *testing.T) *meterEnv {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	if err := reg.Attach(ctx, db, log); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	sessions := auth.New(db, log)
	if err := sessions.CreateUser(ctx, "", auth.User{ID: "alice", DisplayName: "Alice", Role: auth.RoleOperator}, "hunter2hunter"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	priceTable := metering.NewEffectiveDatedTable("empty-v0")
	exceptions := metering.NoMeteredExceptions()
	budgets := metering.NewBudgets(db)
	histStore, err := history.New(history.Config{DB: db, Log: log})
	if err != nil {
		t.Fatalf("history.New: %v", err)
	}
	srv := api.New(api.Config{
		Log: log, Sessions: sessions,
		// A FIXED identity, not the dev fallback: the dev fallback authenticates
		// as `dev`, which would have declared a budget for one person and metered
		// the consumption of another — the two halves would then "agree" by both
		// being empty, which is exactly the agreement this test must not accept.
		Auth:     fixedShellIdentity{"alice"},
		Settings: reg,
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       db, History: histStore,
		// THE TWO SEAMS UNDER TEST, exactly as shell.Run composes them.
		Meter: projMeter{
			ledger:  metering.NewLedger(db, priceTable, exceptions, reg),
			gauge:   metering.NewPressureGauge(db, reg),
			budgets: budgets,
		},
		Budgets: budgetAdapter{b: budgets},
	})
	return &meterEnv{db: db, log: log, srv: srv}
}

func (e *meterEnv) do(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

// consumed walks a run to running through the ratified edges and writes one
// checkpoint, so the gauge has real weighted consumption to measure.
func (e *meterEnv) consumed(t *testing.T, runID, owner, lane string, input, output int64) {
	t.Helper()
	ctx := context.Background()
	runs := run.NewStore(e.db, e.log)
	if _, err := runs.Create(ctx, run.NewRun{ID: runID, UserID: owner, Lane: lane, Substrate: "claude-cli"}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, runID, st, run.TransitionOptions{Reason: "fixture", Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("transition %s: %v", st, err)
		}
	}
	usage := json.RawMessage(`{"input_tokens":` + strconv.FormatInt(input, 10) + `,"output_tokens":` + strconv.FormatInt(output, 10) + `}`)
	if _, err := gates.NewCheckpoints(e.db, e.log).Write(ctx, gates.NewCheckpoint{
		RunID: runID, ModelID: "claude-haiku-4-5", SessionSubstrate: "claude-cli", Usage: usage,
	}); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if _, err := runs.Transition(ctx, runID, run.StateCompleted, run.TransitionOptions{Reason: "fixture", Actor: run.ActorPlatform}); err != nil {
		t.Fatalf("complete: %v", err)
	}
}

// meterLaneOf pulls one lane block out of the meters response.
func meterLaneOf(t *testing.T, body, lane string) api.MeterLane {
	t.Helper()
	var m api.Meters
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("decode meters: %v: %s", err, body)
	}
	for _, l := range m.Lanes {
		if l.Lane == lane {
			return l
		}
	}
	t.Fatalf("no lane %q in the meters response: %s", lane, body)
	return api.MeterLane{}
}

// budgetViewDeclared reads the Layer-0 view block's budget_declared for a person.
func budgetViewDeclared(t *testing.T, body, user string) (declared bool, found bool) {
	t.Helper()
	var m api.Meters
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("decode meters: %v", err)
	}
	if m.Budgets.Answer == nil {
		t.Fatalf("the meters response carries no budget view: %+v", m.Budgets)
	}
	cols := m.Budgets.Answer.Columns
	idxUser, idxDeclared := -1, -1
	for i, c := range cols {
		switch c {
		case "user_id":
			idxUser = i
		case "budget_declared":
			idxDeclared = i
		}
	}
	if idxUser < 0 || idxDeclared < 0 {
		t.Fatalf("the budget view is missing its columns: %v", cols)
	}
	for _, row := range m.Budgets.Answer.Rows {
		if fmtCell(row[idxUser]) == user {
			return fmtCell(row[idxDeclared]) == "1", true
		}
	}
	return false, false
}

func fmtCell(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case nil:
		return ""
	}
	return ""
}

// TestMetersReadAgreesWithTheBudgetVerb is drain D2's locking test: declare
// through the VERB, then read the meters surface and require the two halves of
// the SAME response to agree. Before the fix the gauge half said "no budget"
// while the view half said "budget declared" — one response, two answers.
func TestMetersReadAgreesWithTheBudgetVerb(t *testing.T) {
	e := newMeterEnv(t)
	e.consumed(t, "r-1", "alice", "anthropic", 900, 100) // 1000 weighted units

	// ── PRE-declaration: the honest absence, on BOTH halves.
	_, body := e.do(t, "GET", "/api/meters", "")
	pre := meterLaneOf(t, body, "anthropic")
	if pre.BudgetDeclared || pre.PressureApplicable || pre.Pressure != nil || pre.BudgetRemaining != nil {
		t.Fatalf("pre-declare gauge block = %+v, want the honest absence (D4)", pre)
	}
	if pre.WeightedConsumption != 1000 {
		t.Fatalf("pre-declare weighted consumption = %v, want 1000 — it is real either way", pre.WeightedConsumption)
	}
	if declared, found := budgetViewDeclared(t, body, "alice"); !found || declared {
		t.Fatalf("pre-declare view block: declared=%v found=%v, want an undeclared row", declared, found)
	}

	// ── The verb. The period is declared as ALREADY RUNNING (an hour back), which
	// is the ordinary operator act — "this is my budget for the period I am in"
	// — and it is what puts the consumption above inside the declared period. A
	// period starting now would be equally correct and would measure nothing yet,
	// which is a different (and also honest) reading.
	start := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	if code, out := e.do(t, "POST", "/api/meters/budget",
		`{"lane":"anthropic","period_tokens":2000,"period_days":30,"period_start":"`+start+`"}`); code != http.StatusOK {
		t.Fatalf("declare: status %d: %s", code, out)
	}

	// ── POST-declaration: BOTH halves say the same thing.
	_, body = e.do(t, "GET", "/api/meters", "")
	post := meterLaneOf(t, body, "anthropic")
	if !post.BudgetDeclared {
		t.Error("the gauge block still reports no declared budget after a successful declare (drain D2)")
	}
	if !post.PressureApplicable || post.Pressure == nil {
		t.Fatalf("the gauge block serves no pressure after a declare: %+v", post)
	}
	if *post.Pressure != 0.5 {
		t.Errorf("pressure = %v, want 0.5 (1000 weighted units against a 2000-unit budget)", *post.Pressure)
	}
	if post.BudgetRemaining == nil || *post.BudgetRemaining != 1000 {
		t.Errorf("budget remaining = %v, want 1000 in the budget's own unit", post.BudgetRemaining)
	}
	declared, found := budgetViewDeclared(t, body, "alice")
	if !found || !declared {
		t.Fatalf("the view block reports declared=%v found=%v after a declare", declared, found)
	}
	// THE AGREEMENT ITSELF: one response must not answer the same question two
	// ways.
	if post.BudgetDeclared != declared {
		t.Fatalf("the meters response contradicts itself: gauge declared=%v, view declared=%v",
			post.BudgetDeclared, declared)
	}
	// A lane the person never declared stays honestly absent — the read is
	// per (person, lane), which is the grain the gauge's denominator rule needs.
	e.consumed(t, "r-2", "alice", "local", 10, 10)
	_, body = e.do(t, "GET", "/api/meters", "")
	other := meterLaneOf(t, body, "local")
	if other.BudgetDeclared || other.PressureApplicable {
		t.Errorf("an undeclared lane picked up another lane's budget: %+v", other)
	}
}

// TestRunComposesTheBudgetReadIntoTheMeterReader pins the PRODUCTION
// composition line (re-check residual R-A): the agreement test above builds
// projMeter{…, budgets} itself, so severing the `budgets:` field from the
// projMeter literal in Run() would leave every test green while the live
// `/api/meters` regressed to the exact self-contradiction drain D2 closed.
// A source pin with a vacuity guard (the repo's scan precedent): Run()'s
// projMeter literal must wire a budgets field.
func TestRunComposesTheBudgetReadIntoTheMeterReader(t *testing.T) {
	src, err := os.ReadFile("shell.go")
	if err != nil {
		t.Fatalf("reading shell.go: %v", err)
	}
	text := string(src)

	// Vacuity guard: the composition literal this test pins must exist at all.
	idx := strings.Index(text, "projMeter{")
	if idx < 0 {
		t.Fatal("shell.go no longer builds a projMeter literal — this pin is scanning nothing; move it to wherever the MeterReader is now composed")
	}
	// Every projMeter composite literal in shell.go must wire budgets (today
	// there is exactly one, in Run(); a second unwired one would be the same
	// defect reappearing).
	for n, rest := 0, text; ; n++ {
		i := strings.Index(rest, "projMeter{")
		if i < 0 {
			if n == 0 {
				t.Fatal("unreachable: guarded above")
			}
			break
		}
		lit := rest[i:]
		end := strings.Index(lit, "}")
		if end < 0 {
			t.Fatal("unterminated projMeter literal — scan confused; fix the pin")
		}
		if !strings.Contains(lit[:end], "budgets:") {
			t.Errorf("projMeter literal #%d in shell.go wires no budgets field — the meters read surface would silently report every budget undeclared (drain D2's F2)", n+1)
		}
		rest = lit[end:]
	}
}

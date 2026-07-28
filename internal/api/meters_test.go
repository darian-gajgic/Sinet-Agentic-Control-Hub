package api_test

// meters_test.go — the S15.2 meters family (B6-1 R7–R9): the S10.4 gauge's
// honesty carried onto the wire, the v0 budget ABSENCE rendered with its reason
// instead of a zero, burn rates read at the view's own grain, limit-event
// status, per-axis answerability, cross-owner scope — and the checkable
// negative that no money is computed in internal/api at all.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
)

type wireMeters struct {
	Lanes []struct {
		Owner               string   `json:"owner"`
		Lane                string   `json:"lane"`
		WeightedConsumption float64  `json:"weighted_consumption"`
		CacheReadWeight     float64  `json:"cache_read_weight"`
		Assumed             bool     `json:"assumed"`
		PressureApplicable  bool     `json:"pressure_applicable"`
		Pressure            *float64 `json:"pressure"`
		BudgetDeclared      bool     `json:"budget_declared"`
		BudgetRemaining     *float64 `json:"budget_remaining"`
		TotalRuns           int64    `json:"total_runs"`
		ActiveRuns          int64    `json:"active_runs"`
		ParkedRuns          int64    `json:"parked_runs"`
		ParkedUntil         *string  `json:"parked_until"`
	} `json:"lanes"`
	BurnRates   meterViewWire `json:"burn_rates"`
	Budgets     meterViewWire `json:"budgets"`
	LimitEvents meterViewWire `json:"limit_events"`
	PerPerson   meterViewWire `json:"per_person"`
	PerPeriod   meterViewWire `json:"per_period"`
	Cursor      int64         `json:"cursor"`
}

type meterViewWire struct {
	Answer *struct {
		Layer      int      `json:"layer"`
		Query      string   `json:"query"`
		Confidence string   `json:"confidence"`
		Columns    []string `json:"columns"`
		Rows       [][]any  `json:"rows"`
	} `json:"answer"`
	Absent string `json:"absent"`
}

func meters(t *testing.T, b *backend, who, query string) wireMeters {
	t.Helper()
	var m wireMeters
	if err := json.Unmarshal([]byte(historyGetOK(t, b, who, "/api/meters"+query)), &m); err != nil {
		t.Fatalf("decode meters: %v", err)
	}
	return m
}

// column returns a view answer's column values by name.
func column(t *testing.T, v meterViewWire, name string) []any {
	t.Helper()
	if v.Answer == nil {
		t.Fatalf("view is absent (%s) — expected rows", v.Absent)
	}
	idx := -1
	for i, c := range v.Answer.Columns {
		if c == name {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("column %q not in %v", name, v.Answer.Columns)
	}
	out := []any{}
	for _, r := range v.Answer.Rows {
		out = append(out, r[idx])
	}
	return out
}

// TestMetersServeTheGaugeHonestly proves R7's consumption/pressure half: the
// weighted consumption is real, the "assumed" label rides it, and with no
// declared budget the pressure is NOT applicable and carries no number.
func TestMetersServeTheGaugeHonestly(t *testing.T) {
	b := twoOwners(t)
	m := meters(t, b, "op", "")
	if len(m.Lanes) != 2 {
		t.Fatalf("lanes = %+v, want one per (owner, lane)", m.Lanes)
	}
	if m.Cursor == 0 {
		t.Error("the meters read carries its tail cursor (R18)")
	}
	for _, l := range m.Lanes {
		if l.WeightedConsumption == 0 {
			t.Errorf("lane %s/%s: weighted consumption must be real regardless of budget (S10.4)", l.Owner, l.Lane)
		}
		if l.PressureApplicable || l.Pressure != nil {
			t.Errorf("lane %s/%s: no budget is declared at v0, so pressure has no denominator and must carry no number (D4)", l.Owner, l.Lane)
		}
		if l.BudgetDeclared || l.BudgetRemaining != nil {
			t.Errorf("lane %s/%s: budget_declared must be false and the remainder absent — never a fabricated zero (S10.1)", l.Owner, l.Lane)
		}
		if l.TotalRuns != 1 {
			t.Errorf("lane %s/%s: total_runs = %d", l.Owner, l.Lane, l.TotalRuns)
		}
	}
	// Bob's lane holds the parked run; its park horizon is derived from the
	// limit marker, not stored.
	appendRun(t, b, "bob", "r-bob", "limit.event", `{"class":"class-2","resets_at":"2026-08-01T00:00:00Z"}`)
	m = meters(t, b, "op", "?lane=lane-bob")
	if len(m.Lanes) != 1 || m.Lanes[0].ParkedRuns != 1 {
		t.Fatalf("lane filter / parked count wrong: %+v", m.Lanes)
	}
	if m.Lanes[0].ParkedUntil == nil || *m.Lanes[0].ParkedUntil != "2026-08-01T00:00:00Z" {
		t.Errorf("parked_until = %v, want the lane's limit-marker reset time (S15.5)", m.Lanes[0].ParkedUntil)
	}
}

// TestMetersBudgetIsAnAbsenceWithItsReason is R7's budget half: the v0 posture
// is the recorded absence from cost_budget_remainder — a status and a REASON,
// one row per person, and no number anywhere.
func TestMetersBudgetIsAnAbsenceWithItsReason(t *testing.T) {
	b := twoOwners(t)
	receiptsFixture(t, b)
	m := meters(t, b, "op", "")

	if m.Budgets.Answer == nil {
		t.Fatalf("the budget posture is absent from the meters read: %s", m.Budgets.Absent)
	}
	for _, s := range column(t, m.Budgets, "remainder_status") {
		if s != "UNAVAILABLE" {
			t.Errorf("remainder_status = %v, want UNAVAILABLE — no budget is persisted at v0", s)
		}
	}
	for _, r := range column(t, m.Budgets, "absence_reason") {
		if s, _ := r.(string); !strings.Contains(s, "no operator budget") {
			t.Errorf("the absence must state its reason, got %v", r)
		}
	}
	for _, v := range column(t, m.Budgets, "remainder_usd") {
		if v != nil {
			t.Errorf("remainder_usd = %v, want NULL — an absence is never a zero (S10.1)", v)
		}
	}
	for _, v := range column(t, m.Budgets, "budget_declared") {
		if n, ok := v.(float64); !ok || n != 0 {
			t.Errorf("budget_declared = %v, want 0", v)
		}
	}
}

// TestMetersBurnRatesAndLimitEventsAreReadFromTheViews is R7/R9: burn rates
// arrive VERBATIM at cost_burn_rate's own grain (per person), and the
// limit-event history includes the registered legacy type alias so the history
// does not begin at a rename.
func TestMetersBurnRatesAndLimitEventsAreReadFromTheViews(t *testing.T) {
	b := twoOwners(t)
	receiptsFixture(t, b)
	appendRun(t, b, "alice", "r-alice", "limit.event", `{"class":"class-2","provider_signal":"429","resets_at":"2026-08-01T00:00:00Z","lane":"lane-alice"}`)
	appendRun(t, b, "bob", "r-bob", "engine.rate_limit", `{"class":"class-2","provider_signal":"429","resets_at":"2026-08-02T00:00:00Z","lane":"lane-bob"}`)

	m := meters(t, b, "op", "")
	if m.BurnRates.Answer == nil {
		t.Fatalf("burn rates absent: %s", m.BurnRates.Absent)
	}
	// The grain is the view's own: a user_id column and a rate basis, NOT a
	// lane. internal/api never re-grains a money figure.
	cols := strings.Join(m.BurnRates.Answer.Columns, ",")
	if !strings.Contains(cols, "user_id") || !strings.Contains(cols, "usd_per_day") || !strings.Contains(cols, "rate_basis") {
		t.Errorf("burn-rate columns = %s, want the view's own grain", cols)
	}
	if strings.Contains(cols, "lane") {
		t.Error("the burn-rate view has no lane grain; serving one would mean dividing money here")
	}
	if m.BurnRates.Answer.Confidence != "deterministic" || m.BurnRates.Answer.Layer != 0 {
		t.Errorf("the money figures come from Layer 0 with no model in the path: %+v", m.BurnRates.Answer)
	}

	types := column(t, m.LimitEvents, "type")
	seen := map[any]bool{}
	for _, ty := range types {
		seen[ty] = true
	}
	if !seen["limit.event"] || !seen["engine.rate_limit"] {
		t.Errorf("limit-event history = %v, want BOTH the canonical type and its registered legacy alias (§29 R14)", types)
	}
}

// TestMetersAreOwnerScoped is the cross-owner battery for the meters family,
// across BOTH halves: the lane grain and the view rows.
func TestMetersAreOwnerScoped(t *testing.T) {
	b := twoOwners(t)
	receiptsFixture(t, b)

	op := meters(t, b, "op", "")
	if len(op.Lanes) != 2 {
		t.Fatalf("the operator sees every owner's lanes: %+v", op.Lanes)
	}
	al := meters(t, b, "alice", "")
	if len(al.Lanes) != 1 || al.Lanes[0].Owner != "alice" {
		t.Fatalf("a member sees only their own lanes: %+v", al.Lanes)
	}
	aliceViews := mustString(t, []any{al.BurnRates, al.Budgets, al.LimitEvents, al.PerPerson, al.PerPeriod})
	if strings.Contains(aliceViews, "bob") || strings.Contains(aliceViews, "r-bob") {
		t.Fatalf("a member's meter views leak another owner: %s", aliceViews)
	}
	if !strings.Contains(aliceViews, "alice") {
		t.Fatalf("the member's own view rows vanished — empty is not scoped: %s", aliceViews)
	}
	bo := meters(t, b, "bob", "")
	boViews := mustString(t, []any{bo.BurnRates, bo.Budgets, bo.LimitEvents, bo.PerPerson, bo.PerPeriod})
	if strings.Contains(boViews, "alice") {
		t.Fatalf("the other member's meter views leak: %s", boViews)
	}
	// A member may not read another person's meters by naming them.
	if code, _ := historyGet(t, b, "alice", "/api/meters?person=bob"); code != http.StatusForbidden {
		t.Errorf("member naming another person: %d, want 403", code)
	}
	// The operator may, and the per-person axis answers.
	one := meters(t, b, "op", "?person=bob")
	if len(one.Lanes) != 1 || one.Lanes[0].Owner != "bob" {
		t.Errorf("person axis = %+v", one.Lanes)
	}
}

// TestMetersWithoutTheQuerySurfaceRenderAbsences: with no Layer-0 surface the
// money blocks say so. The gauge half still reads — an absent view surface must
// not take the whole meter down.
func TestMetersWithoutTheQuerySurfaceRenderAbsences(t *testing.T) {
	b := twoOwners(t)
	srv := api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{"op"},
		Settings: fixedSettings{d: 20 * 1e9},
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       b.db, Meter: fakeMeter{},
	})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/meters", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	var m wireMeters
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(m.Lanes) != 2 {
		t.Errorf("the gauge half must still read: %+v", m.Lanes)
	}
	for name, v := range map[string]meterViewWire{
		"burn_rates": m.BurnRates, "budgets": m.Budgets, "limit_events": m.LimitEvents,
		"per_person": m.PerPerson, "per_period": m.PerPeriod,
	} {
		if v.Answer != nil || v.Absent == "" {
			t.Errorf("%s must render as a stated absence when the query surface is unwired: %+v", name, v)
		}
	}
}

// TestNoMoneyIsComputedInTheAPI is R8, the checkable NEGATIVE: internal/api
// contains no token→currency arithmetic, names no price or rate identifier in a
// money expression, and does not import the price table. Every dollar figure it
// serves is READ — from the metering seam or from a Layer-0 view.
func TestNoMoneyIsComputedInTheAPI(t *testing.T) {
	tokenFields := []string{
		"prompt_tokens", "billed_output_tokens", "cache_read_tokens",
		"cache_creation_tokens", "output_tokens", "input_tokens",
	}
	badIdents := []string{"per_token", "per_million", "price_per", "rate_usd", "priceTable", "PriceTable"}
	// A `*` in arithmetic position on a line that mentions money is the defect.
	mult := regexp.MustCompile(`[a-zA-Z_0-9)\]]\s*\*\s*[a-zA-Z_0-9(]`)

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src := readSourceFile(t, name)
		scanned++
		for _, tf := range tokenFields {
			if strings.Contains(src, tf) {
				t.Errorf("%s references the token field %q — internal/api never turns tokens into money (S14.10 ¶1)", name, tf)
			}
		}
		for _, id := range badIdents {
			if strings.Contains(src, id) {
				t.Errorf("%s names %q — the price table lives in internal/metering and internal/api never reaches it", name, id)
			}
		}
		if strings.Contains(src, `"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"`) {
			t.Errorf("%s imports internal/metering — the API seam reads a priced figure through the MeterReader seam, never the ledger", name)
		}
		for _, line := range strings.Split(src, "\n") {
			l := strings.TrimSpace(line)
			if strings.HasPrefix(l, "//") {
				continue
			}
			low := strings.ToLower(l)
			if !strings.Contains(low, "usd") && !strings.Contains(low, "cost") && !strings.Contains(low, "consumption") {
				continue
			}
			if mult.MatchString(l) {
				t.Errorf("%s multiplies in a money expression — money is READ, never computed:\n  %s", name, line)
			}
		}
	}
	if scanned == 0 {
		t.Fatal("the money scan read no files — it would pass vacuously")
	}

	// Non-tautology probe: the scan detects its own planted defect.
	probe := "\tcostUSD := prompt_tokens * ratePerToken\n"
	if !strings.Contains(probe, tokenFields[0]) || !mult.MatchString(probe) {
		t.Fatal("the money-by-generation scan cannot detect its own probe — it would pass vacuously")
	}
}

// TestMissionControlAxesAnswerEndToEnd is R12: project, person, status and date
// are each demonstrably answerable through the shipped read surface on the
// two-owner fixture — through the list filters and through the routed catalog.
func TestMissionControlAxesAnswerEndToEnd(t *testing.T) {
	b := twoOwners(t)
	receiptsFixture(t, b)

	// Through the R1/R4 list filters.
	for _, c := range []struct{ axis, path, want, notWant string }{
		{"project", "/api/tasks?project=proj-alice", "t-alice", "t-bob"},
		{"person", "/api/runs?person=bob", "r-bob", "r-alice"},
		{"status", "/api/runs?status=running", "r-alice", "r-bob"},
		{"date", "/api/runs?since=2020-01-01T00:00:00Z", "r-alice", ""},
	} {
		body := historyGetOK(t, b, "op", c.path)
		if !strings.Contains(body, c.want) {
			t.Errorf("axis %s via %s: missing %q", c.axis, c.path, c.want)
		}
		if c.notWant != "" && strings.Contains(body, c.notWant) {
			t.Errorf("axis %s via %s: leaked %q", c.axis, c.path, c.notWant)
		}
	}

	// Through the routed catalog — the same four axes as typed slots.
	for _, c := range []struct{ axis, path, want string }{
		{"person", "/api/events/query/cost.for_person?slot_person=alice", "alice"},
		{"status", "/api/events/query/status.runs_by_state?slot_state=parked", "r-bob"},
		{"date", "/api/events/query/status.runs_in_date_range?slot_from=2020-01-01&slot_to=2099-01-01", "r-alice"},
		{"project", "/api/events/query/cost.for_project?slot_project=" + url.QueryEscape("(no project)"), "(no project)"},
	} {
		code, body := historyGet(t, b, "op", c.path)
		if code != http.StatusOK {
			t.Fatalf("axis %s via %s: %d: %s", c.axis, c.path, code, body)
		}
		if !strings.Contains(body, c.want) {
			t.Errorf("axis %s via %s: missing %q in %s", c.axis, c.path, c.want, body)
		}
	}

	// And the four mission-control axes are reachable as declared slot TYPES
	// through the routed catalog. The axis registry is the package's own (its
	// completeness is asserted there, §37); what this checks is that the
	// transport exposes it so a client can bind them.
	declared := map[history.SlotType]bool{}
	for _, a := range history.Axes {
		declared[a] = true
	}
	for _, want := range []history.SlotType{"project", "person", "status", "date_from", "date_to"} {
		if !declared[want] {
			t.Errorf("axis %q is not a declared slot type", want)
		}
	}
	catalogBody := historyGetOK(t, b, "op", "/api/events/catalog")
	for _, want := range []string{`"project"`, `"person"`, `"status"`, `"date_from"`} {
		if !strings.Contains(catalogBody, want) {
			t.Errorf("the routed catalog does not expose the %s axis for binding", want)
		}
	}
}

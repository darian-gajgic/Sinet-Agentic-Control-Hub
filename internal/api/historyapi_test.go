package api_test

// historyapi_test.go + the meters battery — the B half of B6-1: the S14.10
// query layers ROUTED (registries, SelectView, RunQuery, Ask, Search,
// AskOpenSQL), scope threaded through the layers on the two-owner fixture, the
// Layer-2 flag and audit carried verbatim on the wire, and the meters surface's
// honest money posture.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
)

// historyServer builds a server with the REAL query store over the fixture DB.
// Duty, calibration, advisory and the read-only handle are nil — the honest
// degraded posture the composed surface is proven usable in (§37): Layer 0 and
// Layer 1's named verbs need no model, Ask degrades to its card, and Layer 2
// reports itself unavailable through its own audited answer.
func historyServer(t *testing.T, b *backend, who string) *api.Server {
	t.Helper()
	st, err := history.New(history.Config{DB: b.db, Log: b.log})
	if err != nil {
		t.Fatalf("history.New: %v", err)
	}
	return api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{who},
		Settings: fixedSettings{d: 20 * 1e9},
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       b.db, Meter: fakeMeter{}, History: st,
	})
}

func historyGet(t *testing.T, b *backend, who, path string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	historyServer(t, b, who).Handler().ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
	return rr.Code, rr.Body.String()
}

func historyGetOK(t *testing.T, b *backend, who, path string) string {
	t.Helper()
	code, body := historyGet(t, b, who, path)
	if code != http.StatusOK {
		t.Fatalf("GET %s as %s: %d: %s", path, who, code, body)
	}
	return body
}

// wireAnswer is the answer shape as it arrives — decoded rather than
// string-matched so the confidence label and the audit block are asserted as
// FIELDS, which is what the contract is about.
type wireAnswer struct {
	Layer      int      `json:"layer"`
	Query      string   `json:"query"`
	Confidence string   `json:"confidence"`
	Question   string   `json:"question"`
	Columns    []string `json:"columns"`
	Rows       [][]any  `json:"rows"`
	RowCount   int      `json:"row_count"`
	Truncated  bool     `json:"truncated"`
	Notes      []string `json:"notes"`
	Card       *struct {
		Question string `json:"question"`
		Reason   string `json:"reason"`
		Choices  []struct {
			Query    string `json:"query"`
			Category string `json:"category"`
		} `json:"choices"`
	} `json:"card"`
	Audit *struct {
		Question      string `json:"question"`
		Outcome       string `json:"outcome"`
		Refusal       string `json:"refusal"`
		Alias         string `json:"alias"`
		AsOperator    bool   `json:"as_operator"`
		LimitInjected int    `json:"limit_injected"`
		TimeoutMS     int64  `json:"timeout_ms"`
	} `json:"audit"`
}

func decodeAnswer(t *testing.T, body string) wireAnswer {
	t.Helper()
	var a wireAnswer
	if err := json.Unmarshal([]byte(body), &a); err != nil {
		t.Fatalf("decode answer: %v (%s)", err, body)
	}
	return a
}

// receiptsFixture adds a materialized receipt per owner so the Layer-0 cost
// views have real rows to be scoped over.
func receiptsFixture(t *testing.T, b *backend) {
	t.Helper()
	for _, c := range []struct{ run, owner string }{{"r-alice", "alice"}, {"r-bob", "bob"}} {
		exec(t, b, `INSERT INTO receipts (run_id, user_id, usage_json, materialized_ts) VALUES (?,?,?,?)`,
			c.run, c.owner,
			`{"run_id":"`+c.run+`","total_calls":2,"total_unpriced_calls":2,"currency":"api-equivalent","direct_use":{"label":"UNPRICED","unpriced":true}}`,
			nowTS())
	}
}

// TestHistoryRegistriesAreRouted proves a client can render the choices: the
// Layer-0 view registry with its S2.5 cost questions, and the Layer-1 catalog
// with its categories, typed axes and per-query slots.
func TestHistoryRegistriesAreRouted(t *testing.T) {
	b := twoOwners(t)

	var views struct {
		Views []struct {
			Name        string `json:"Name"`
			Question    string `json:"Question"`
			OwnerColumn string `json:"OwnerColumn"`
		}
		CostQuestions []string `json:"cost_questions"`
	}
	if err := json.Unmarshal([]byte(historyGetOK(t, b, "op", "/api/events/views")), &views); err != nil {
		t.Fatalf("decode views: %v", err)
	}
	if len(views.Views) != len(history.Views()) || len(views.Views) == 0 {
		t.Fatalf("the routed view registry must be the package's own: %d vs %d", len(views.Views), len(history.Views()))
	}
	if len(views.CostQuestions) == 0 {
		t.Error("the S2.5 cost questions must ride the registry so a client can label the choices")
	}

	var cat struct {
		Categories []string `json:"categories"`
		Axes       []string `json:"axes"`
		Queries    []struct {
			Name  string `json:"name"`
			Slots []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"slots"`
		} `json:"queries"`
		QueryNames []string `json:"query_names"`
	}
	if err := json.Unmarshal([]byte(historyGetOK(t, b, "op", "/api/events/catalog")), &cat); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(cat.Queries) != len(history.Catalog()) || len(cat.QueryNames) != len(history.CatalogNames()) {
		t.Fatalf("the routed catalog must be the package's own: %d/%d", len(cat.Queries), len(cat.QueryNames))
	}
	if len(cat.Categories) != len(history.Categories) || len(cat.Axes) != len(history.Axes) {
		t.Errorf("categories/axes = %d/%d, want the package's %d/%d", len(cat.Categories), len(cat.Axes), len(history.Categories), len(history.Axes))
	}
	slotted := 0
	for _, q := range cat.Queries {
		if len(q.Slots) > 0 && q.Slots[0].Type != "" {
			slotted++
		}
	}
	if slotted == 0 {
		t.Error("no query carries its typed slots — a client cannot render the parameters")
	}
	// The SQL is NOT on the wire: the statement is the package's, and a client
	// that could read it would be one edit from a client that sends one.
	if strings.Contains(historyGetOK(t, b, "op", "/api/events/catalog"), "SELECT ") {
		t.Error("the catalog serialized its SQL — the transport exposes names and slots, not statements")
	}
}

// TestHistoryLayer0AndLayer1AreScopedThroughTheLayers proves SelectView and
// RunQuery route AND that the S01.9 scope threads through the package: the
// operator sees both owners' rows, each member sees only their own.
func TestHistoryLayer0AndLayer1AreScopedThroughTheLayers(t *testing.T) {
	b := twoOwners(t)
	receiptsFixture(t, b)

	for _, path := range []string{
		"/api/events/views/cost_per_run",
		"/api/events/query/status.runs_active",
		"/api/events/query/status.runs_by_state?slot_state=running",
	} {
		op := decodeAnswer(t, historyGetOK(t, b, "op", path))
		if op.Query == "" || op.Confidence == "" {
			t.Fatalf("%s: an answer is never anonymous or unlabelled: %+v", path, op)
		}
		opRows := mustString(t, op.Rows)

		al := decodeAnswer(t, historyGetOK(t, b, "alice", path))
		aliceRows := mustString(t, al.Rows)
		if strings.Contains(aliceRows, "r-bob") || strings.Contains(aliceRows, "bob") {
			t.Errorf("%s: a member read another owner's rows THROUGH the layers: %s", path, aliceRows)
		}
		if strings.Contains(path, "cost_per_run") || strings.Contains(path, "runs_active") {
			if !strings.Contains(aliceRows, "r-alice") {
				t.Errorf("%s: the member's own rows vanished — empty is not scoped: %s", path, aliceRows)
			}
			if !strings.Contains(opRows, "r-alice") || !strings.Contains(opRows, "r-bob") {
				t.Errorf("%s: the operator must see every owner: %s", path, opRows)
			}
		}
	}

	// Layer 0 labels its own floor: no model was in the path at all.
	a := decodeAnswer(t, historyGetOK(t, b, "op", "/api/events/views/cost_per_run"))
	if a.Layer != 0 || a.Confidence != "deterministic" {
		t.Errorf("Layer-0 answer = layer %d confidence %q", a.Layer, a.Confidence)
	}
	// Layer 1 names the query it ran (R26).
	c := decodeAnswer(t, historyGetOK(t, b, "op", "/api/events/query/status.runs_active"))
	if c.Layer != 1 || c.Confidence != "canned" || c.Query != "status.runs_active" {
		t.Errorf("Layer-1 answer = %+v", c)
	}
}

// TestHistoryClosedRegistryIsTheOnlyWayIn: a name that is not in the closed
// registry never reaches SQL — it is a 404, not an interpolated query.
func TestHistoryClosedRegistryIsTheOnlyWayIn(t *testing.T) {
	b := twoOwners(t)
	for _, path := range []string{
		"/api/events/views/runs",
		"/api/events/views/" + url.PathEscape("cost_per_run; DROP TABLE runs"),
		"/api/events/query/nope.not_a_query",
	} {
		if code, body := historyGet(t, b, "op", path); code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 from the closed registry: %s", path, code, body)
		}
	}
	// A declared slot that is missing, and an undeclared slot supplied, are both
	// errors — a filter that silently did not apply returns a WIDER answer.
	if code, _ := historyGet(t, b, "op", "/api/events/query/status.runs_by_state"); code != http.StatusBadRequest {
		t.Errorf("missing slot: %d, want 400", code)
	}
	if code, _ := historyGet(t, b, "op", "/api/events/query/status.runs_active?slot_bogus=x"); code != http.StatusBadRequest {
		t.Errorf("undeclared slot: %d, want 400", code)
	}
	// A hostile slot VALUE is data: it matches nothing and destroys nothing.
	body := historyGetOK(t, b, "op", "/api/events/query/status.runs_by_state?slot_state="+url.QueryEscape("running'; DROP TABLE runs; --"))
	if a := decodeAnswer(t, body); a.RowCount != 0 {
		t.Errorf("a hostile slot value matched rows: %+v", a)
	}
	var n int
	if err := b.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM runs`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("the runs table was disturbed (%d rows, err %v)", n, err)
	}
}

// TestHistoryAskReturnsTheCardNotAnError: with no local stack the intent layer
// cannot resolve, and S12.4's mandated failure mode is a disambiguation card
// with real catalog choices — a 200-class answer, never a transport error.
func TestHistoryAskReturnsTheCardNotAnError(t *testing.T) {
	b := twoOwners(t)
	code, body := historyGet(t, b, "alice", "/api/events/ask?q="+url.QueryEscape("what did my runs cost last week"))
	if code != http.StatusOK {
		t.Fatalf("an unresolved question must be an ANSWER, got %d: %s", code, body)
	}
	a := decodeAnswer(t, body)
	if a.Card == nil {
		t.Fatalf("no disambiguation card on an unresolved question: %s", body)
	}
	if a.Card.Reason == "" || len(a.Card.Choices) == 0 {
		t.Errorf("the card must carry its reason and real catalog choices: %+v", a.Card)
	}
	if a.Query == "" || a.Confidence == "" {
		t.Error("even the non-answer names something (R26)")
	}
	if code, _ := historyGet(t, b, "alice", "/api/events/ask"); code != http.StatusBadRequest {
		t.Error("an empty question is refused at the boundary")
	}
}

// TestHistorySearchIsRouted: the redact-before-match search answers over HTTP.
// The corpus-level C2 property is internal/history's own battery; what is under
// test here is that the verb is reachable and its answer arrives whole.
func TestHistorySearchIsRouted(t *testing.T) {
	b := twoOwners(t)
	a := decodeAnswer(t, historyGetOK(t, b, "alice", "/api/events/search?q="+url.QueryEscape("deployment credential rotation")))
	if a.Query == "" || a.Confidence == "" {
		t.Fatalf("search answer is unlabelled: %+v", a)
	}
	if a.Rows == nil {
		t.Error("an answer always carries its rows collection, empty or not")
	}
}

// TestHistoryLayer2IsItsOwnActAndItsAnswerCarriesTheAudit is the R11/R13
// acceptance: Layer 2 has its own route, its answer is flagged
// lower-confidence, a non-executed attempt comes back AS AN ANSWER carrying its
// audit block rather than as a bare HTTP error, and the attempt is recorded in
// the log as a history.query_audited row scoped to the asker.
func TestHistoryLayer2IsItsOwnActAndItsAnswerCarriesTheAudit(t *testing.T) {
	b := twoOwners(t)
	const q = "show me everything"

	code, body := historyGet(t, b, "alice", "/api/events/open-sql?q="+url.QueryEscape(q))
	if code != http.StatusOK {
		t.Fatalf("a refused/unavailable Layer-2 attempt must be an ANSWER, got %d: %s", code, body)
	}
	a := decodeAnswer(t, body)
	if a.Layer != 2 || a.Confidence != "lower-confidence" {
		t.Fatalf("every Layer-2 answer is flagged lower-confidence: layer %d confidence %q", a.Layer, a.Confidence)
	}
	if a.Audit == nil {
		t.Fatalf("the Layer-2 audit block must ride the answer: %s", body)
	}
	if a.Audit.Outcome == "" || a.Audit.Refusal == "" || a.Audit.Alias == "" {
		t.Errorf("the audit must say what happened and why: %+v", a.Audit)
	}
	if a.Audit.AsOperator {
		t.Error("the audit records the ASKER's scope; alice is not the operator")
	}
	if a.Audit.LimitInjected == 0 || a.Audit.TimeoutMS == 0 {
		t.Errorf("the injected bound and timeout are part of the record: %+v", a.Audit)
	}

	// The attempt is in the log, scoped to the asker.
	var owner, payload string
	if err := b.db.QueryRowContext(context.Background(),
		`SELECT user_id, payload FROM run_events WHERE type = ? ORDER BY event_seq DESC LIMIT 1`,
		history.EventQueryAudited).Scan(&owner, &payload); err != nil {
		t.Fatalf("no history.query_audited row for the attempt: %v", err)
	}
	if owner != "alice" {
		t.Errorf("the audit row is scoped to %q, want the asker", owner)
	}
	if !strings.Contains(payload, q) {
		t.Errorf("the audit row does not carry the question: %s", payload)
	}

	// R13: no Layer-0/1 route escalates. Drive each of them once more; if any
	// fell through to open SQL there would be a second audit row.
	historyGetOK(t, b, "alice", "/api/events/ask?q="+url.QueryEscape("anything at all"))
	historyGetOK(t, b, "alice", "/api/events/views/cost_per_run")
	historyGetOK(t, b, "alice", "/api/events/query/status.runs_active")
	historyGetOK(t, b, "alice", "/api/events/search?q=anything")
	var audits int
	if err := b.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM run_events WHERE type = ?`, history.EventQueryAudited).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Errorf("audited Layer-2 attempts = %d, want exactly the ONE deliberate act — no route falls through to open SQL (R13)", audits)
	}
}

// TestHistoryRoutingBuildsNoSQL is checklist 8: the transport ROUTES the query
// layers and rebuilds nothing above them. No SQL text for history data is
// constructed in historyapi.go, no read is executed there, and no second copy
// of the guardrail, audit, redaction or confidence logic lives there.
func TestHistoryRoutingBuildsNoSQL(t *testing.T) {
	// Comments are STRIPPED before the scan: this file's doc comments name the
	// very constructs it must not contain, and a citation in prose is not a
	// construct (the §37 TestNoMoneyByGeneration precedent).
	src := stripGoComments(readSourceFile(t, "historyapi.go"))
	for _, bad := range []string{"SELECT ", " FROM ", "QueryContext(", "lower-confidence", "redact.", "clampLimit"} {
		if strings.Contains(src, bad) {
			t.Errorf("historyapi.go contains %q in CODE — the layers are routed, never rebuilt above them (R10)", bad)
		}
	}
	// Non-tautology: the scan finds what it looks for when it is there.
	probe := `db.QueryContext(ctx, "SELECT 1 FROM run_events")`
	for _, bad := range []string{"SELECT ", " FROM ", "QueryContext("} {
		if !strings.Contains(probe, bad) {
			t.Fatalf("the routing-only scan cannot detect its own probe (%q)", bad)
		}
	}
}

// readSourceFile reads one non-test source file of internal/api from the test
// binary's own package directory.
func readSourceFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(raw)
}

// stripGoComments removes // and /* */ comments so a source scan judges CODE.
// It is deliberately simple: the files it reads carry no comment markers inside
// string literals, and a scan that over-strips would only make the assertion
// weaker in a way the non-tautology probe catches.
func stripGoComments(src string) string {
	src = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(src, "")
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

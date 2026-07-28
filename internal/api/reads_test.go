package api_test

// reads_test.go — the S15.2 REST read families (B6-1): owner scope proven
// three ways per endpoint (operator all / owner theirs / other member zero),
// the mission-control filter axes exercised end to end, the S15.3 cursor, the
// store-raw/serve-redacted edge on payload-bearing responses, and the honest
// absences the task detail renders instead of hiding a task.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
)

// noProjectBucketToken is the 0016 honest bucket label a project filter may
// name (§37): work with no registry-resolved project is bucketed, never dropped.
const noProjectBucketToken = "(no project)"

// twoOwners is the standing fixture: an operator, two members, and rows that
// are DISJOINT on every axis, so a dropped predicate shows up as a leak rather
// than as a coincidence.
func twoOwners(t *testing.T) *backend {
	t.Helper()
	b := newBackend(t)
	seedUser(t, b, "op", auth.RoleOperator)
	seedUser(t, b, "alice", auth.RoleMember)
	seedUser(t, b, "bob", auth.RoleMember)

	seedTask(t, b, "t-alice", "alice", "Alice Task", "doing")
	seedTask(t, b, "t-bob", "bob", "Bob Task", "review")
	seedRun(t, b, "r-alice", "alice", "t-alice", "running", "lane-alice")
	seedRun(t, b, "r-bob", "bob", "t-bob", "parked", "lane-bob")
	seedAsk(t, b, "ask-bob", "r-bob", "bob", "gate")

	// artifact_claims is the only populated project edge (§37): alice's task
	// resolves a project, bob's resolves none and lands in the honest bucket.
	exec(t, b, `INSERT INTO artifact_claims (task_id, project, user_id, path_globs, mode, status, created_ts)
	            VALUES (?,?,?,?,?,?,?)`, "t-alice", "proj-alice", "alice", "**", "W", "active", nowTS())

	appendRun(t, b, "alice", "r-alice", "routing.decided",
		`{"cause":"selector-match","score":0.9,"model":"m","lane":"anthropic","plain_reason":"best fit"}`)
	appendRun(t, b, "alice", "r-alice", "helper.spawned",
		`{"spawn_id":"sp-1","depth":1,"trigger":"tg","reason":"r","brief_hash":"bh"}`)
	appendRun(t, b, "alice", "r-alice", "stage.started",
		`{"stage":"S-1","kind":"execute","brief_hash":"abc"}`)
	appendRun(t, b, "alice", "r-alice", "stage.finished",
		`{"stage":"S-1","kind":"execute","brief_hash":"abc","outcome":"completed"}`)
	appendRun(t, b, "bob", "r-bob", "routing.decided",
		`{"cause":"selector-match","score":0.4,"model":"m","lane":"zai","plain_reason":"bob only"}`)
	return b
}

// get issues an authenticated GET as one identity and returns status + body.
func get(t *testing.T, b *backend, who, path string) (int, string) {
	t.Helper()
	srv := api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{who},
		Settings: fixedSettings{d: 20 * 1e9},
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       b.db, Meter: fakeMeter{},
	})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
	return rr.Code, rr.Body.String()
}

func getOK(t *testing.T, b *backend, who, path string) string {
	t.Helper()
	code, body := get(t, b, who, path)
	if code != http.StatusOK {
		t.Fatalf("GET %s as %s: status %d: %s", path, who, code, body)
	}
	return body
}

// threeWay is the standing cross-owner assertion: the operator sees both
// owners' tokens, the owner sees only their own, and the other member sees
// NEITHER of the first owner's — zero, not a filtered-down subset.
func threeWay(t *testing.T, b *backend, path, aliceToken, bobToken string) {
	t.Helper()
	op := getOK(t, b, "op", path)
	if !strings.Contains(op, aliceToken) || !strings.Contains(op, bobToken) {
		t.Fatalf("operator GET %s must see both owners; got %s", path, op)
	}
	a := getOK(t, b, "alice", path)
	if !strings.Contains(a, aliceToken) {
		t.Fatalf("owner GET %s missing own row %q (empty is not scoped): %s", path, aliceToken, a)
	}
	if strings.Contains(a, bobToken) {
		t.Fatalf("owner GET %s LEAKS another owner's row %q: %s", path, bobToken, a)
	}
	bo := getOK(t, b, "bob", path)
	if strings.Contains(bo, aliceToken) {
		t.Fatalf("other member GET %s LEAKS %q: %s", path, aliceToken, bo)
	}
}

func TestRunListOwnerScopedThreeWay(t *testing.T) {
	b := twoOwners(t)
	threeWay(t, b, "/api/runs", "r-alice", "r-bob")
}

func TestTaskListOwnerScopedThreeWay(t *testing.T) {
	b := twoOwners(t)
	threeWay(t, b, "/api/tasks", "t-alice", "t-bob")
}

// TestRunListCarriesFSMStateAndDerivedMarkers pins the row contents: stored FSM
// state plus the derived members, and the cursor a client tails from.
func TestRunListCarriesFSMStateAndDerivedMarkers(t *testing.T) {
	b := twoOwners(t)
	appendRun(t, b, "bob", "r-bob", "limit.event", `{"class":"class-2","resets_at":"2026-08-01T00:00:00Z"}`)

	var list struct {
		Runs []struct {
			RunID          string  `json:"run_id"`
			State          string  `json:"state"`
			WaitingOnHuman bool    `json:"waiting_on_human"`
			ParkedUntil    *string `json:"parked_until"`
			Wedged         bool    `json:"wedged"`
			Stage          string  `json:"stage"`
			TaskID         string  `json:"task_id"`
		} `json:"runs"`
		Cursor    int64 `json:"cursor"`
		Truncated bool  `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(getOK(t, b, "op", "/api/runs")), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Cursor == 0 {
		t.Error("the list must carry its snapshot-then-tail cursor (R18)")
	}
	byID := map[string]int{}
	for i, r := range list.Runs {
		byID[r.RunID] = i
	}
	alice := list.Runs[byID["r-alice"]]
	if alice.State != "running" || alice.TaskID != "t-alice" {
		t.Errorf("alice row = %+v", alice)
	}
	if alice.Stage != "S-1" {
		t.Errorf("stage = %q, want the stage.* marker the producer minted (R16)", alice.Stage)
	}
	bob := list.Runs[byID["r-bob"]]
	if !bob.WaitingOnHuman {
		t.Error("a parked run with an open ask is waiting_on_human (first-class, §30)")
	}
	if bob.ParkedUntil == nil || *bob.ParkedUntil != "2026-08-01T00:00:00Z" {
		t.Errorf("parked_until = %v, want the limit event's resets_at", bob.ParkedUntil)
	}
	if alice.Wedged || bob.Wedged {
		t.Error("wedged is DERIVED from run.wedged and neither run has one")
	}
}

// TestRunListFiltersAndBounds exercises the mission-control axes and the
// boundary validation: status, person, task, date, and the page bound.
func TestRunListFiltersAndBounds(t *testing.T) {
	b := twoOwners(t)

	if body := getOK(t, b, "op", "/api/runs?status=parked"); strings.Contains(body, "r-alice") || !strings.Contains(body, "r-bob") {
		t.Errorf("status filter wrong: %s", body)
	}
	if body := getOK(t, b, "op", "/api/runs?person=alice"); strings.Contains(body, "r-bob") || !strings.Contains(body, "r-alice") {
		t.Errorf("person filter wrong: %s", body)
	}
	if body := getOK(t, b, "op", "/api/runs?task=t-bob"); strings.Contains(body, "r-alice") || !strings.Contains(body, "r-bob") {
		t.Errorf("task filter wrong: %s", body)
	}
	if body := getOK(t, b, "op", "/api/runs?since=2099-01-01T00:00:00Z"); strings.Contains(body, "r-alice") {
		t.Errorf("date filter wrong: %s", body)
	}
	if code, _ := get(t, b, "op", "/api/runs?status=wedged"); code != http.StatusBadRequest {
		t.Errorf("status=wedged: %d, want 400 — wedged is derived, never a stored state", code)
	}
	if code, _ := get(t, b, "op", "/api/runs?since=yesterday"); code != http.StatusBadRequest {
		t.Errorf("bad since: %d, want 400", code)
	}
	if code, _ := get(t, b, "op", "/api/runs?limit=0"); code != http.StatusBadRequest {
		t.Errorf("limit=0: %d, want 400", code)
	}
	// A member may not name another person — refused, never silently re-scoped.
	if code, _ := get(t, b, "alice", "/api/runs?person=bob"); code != http.StatusForbidden {
		t.Errorf("member naming another person: %d, want 403", code)
	}

	// The bound is observed, not inferred: with two runs and limit=1 the page
	// is short AND says so.
	var page struct {
		Runs      []json.RawMessage `json:"runs"`
		Truncated bool              `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(getOK(t, b, "op", "/api/runs?limit=1")), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Runs) != 1 || !page.Truncated {
		t.Errorf("limit=1 → %d rows truncated=%v, want 1 and true", len(page.Runs), page.Truncated)
	}
}

// TestRunDetailScopeAndRecords proves the authorizeRun semantics on the detail
// route and that the S04.4 spawn records and S2.6 routing records are served.
func TestRunDetailScopeAndRecords(t *testing.T) {
	b := twoOwners(t)

	if code, _ := get(t, b, "alice", "/api/runs/r-nope"); code != http.StatusNotFound {
		t.Errorf("unknown run: %d, want 404", code)
	}
	if code, _ := get(t, b, "alice", "/api/runs/r-bob"); code != http.StatusForbidden {
		t.Errorf("другой owner's run: %d, want 403", code)
	}
	if code, _ := get(t, b, "op", "/api/runs/r-bob"); code != http.StatusOK {
		t.Errorf("operator reading any run: %d, want 200", code)
	}

	var d struct {
		Card struct {
			RunID string `json:"run_id"`
			State string `json:"state"`
		} `json:"card"`
		LiveActivity struct {
			Cursor int64  `json:"cursor"`
			Topic  string `json:"topic"`
			RunID  string `json:"run_id"`
		} `json:"live_activity"`
		SpawnRecords   []struct{ Type string } `json:"spawn_records"`
		RoutingRecords []struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		} `json:"routing_records"`
		Cursor int64 `json:"cursor"`
	}
	if err := json.Unmarshal([]byte(getOK(t, b, "alice", "/api/runs/r-alice")), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.Card.RunID != "r-alice" || d.Card.State != "running" {
		t.Errorf("card = %+v", d.Card)
	}
	if d.Cursor == 0 || d.LiveActivity.Cursor != d.Cursor || d.LiveActivity.Topic != "run" || d.LiveActivity.RunID != "r-alice" {
		t.Errorf("live-activity refs = %+v (cursor %d) — S15.3 snapshot-then-tail", d.LiveActivity, d.Cursor)
	}
	if len(d.SpawnRecords) != 1 || d.SpawnRecords[0].Type != "helper.spawned" {
		t.Errorf("spawn records = %+v", d.SpawnRecords)
	}
	if len(d.RoutingRecords) != 1 || !strings.Contains(string(d.RoutingRecords[0].Payload), "plain_reason") {
		t.Errorf("routing records = %+v — S14.2 family 7 payloads served", d.RoutingRecords)
	}
	// The other owner's routing row is not reachable through the operator's
	// detail of a different run either.
	if strings.Contains(mustString(t, d.RoutingRecords), "bob only") {
		t.Error("a run detail served another run's records")
	}
}

func mustString(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestRunDetailStoreRawServeRedacted proves R20 at the REST edge: a secret
// planted in a run_events payload is [REDACTED:…] on the wire while the stored
// row keeps it verbatim.
func TestRunDetailStoreRawServeRedacted(t *testing.T) {
	b := twoOwners(t)
	const secret = "sk-ant-api03-PLANTEDSECRETPLANTEDSECRETPLANTEDSECRET"
	appendRun(t, b, "alice", "r-alice", "helper.spawned",
		fmt.Sprintf(`{"spawn_id":"sp-2","depth":1,"trigger":"t","reason":%q,"brief_hash":"bh"}`, secret))

	body := getOK(t, b, "alice", "/api/runs/r-alice")
	if strings.Contains(body, secret) {
		t.Fatalf("the REST run detail served a planted secret verbatim: %s", body)
	}
	if !strings.Contains(body, "[REDACTED:") {
		t.Fatalf("no redaction marker on the wire: %s", body)
	}
	var stored string
	if err := b.db.QueryRowContext(context.Background(),
		`SELECT payload FROM run_events WHERE run_id = 'r-alice' AND type = 'helper.spawned'
		  ORDER BY event_seq DESC LIMIT 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored, secret) {
		t.Fatal("the STORED row was mutated — store-raw / serve-redacted (R19)")
	}
}

// TestTaskListProjectAxis proves the project filter and the honest bucket: a
// task with no registry-resolved project is bucketed, never dropped (§37).
func TestTaskListProjectAxis(t *testing.T) {
	b := twoOwners(t)

	all := getOK(t, b, "op", "/api/tasks")
	if !strings.Contains(all, "proj-alice") || !strings.Contains(all, noProjectBucketToken) {
		t.Fatalf("both the resolved project and the honest bucket must render: %s", all)
	}
	if body := getOK(t, b, "op", "/api/tasks?project=proj-alice"); strings.Contains(body, "t-bob") || !strings.Contains(body, "t-alice") {
		t.Errorf("project filter wrong: %s", body)
	}
	if body := getOK(t, b, "op", "/api/tasks?project="+url.QueryEscape(noProjectBucketToken)); strings.Contains(body, "t-alice") || !strings.Contains(body, "t-bob") {
		t.Errorf("the honest bucket must be selectable: %s", body)
	}
	if body := getOK(t, b, "op", "/api/tasks?status=review"); strings.Contains(body, "t-alice") || !strings.Contains(body, "t-bob") {
		t.Errorf("kanban status filter wrong: %s", body)
	}
	if body := getOK(t, b, "op", "/api/tasks?person=bob"); strings.Contains(body, "t-alice") || !strings.Contains(body, "t-bob") {
		t.Errorf("person filter wrong: %s", body)
	}
	if body := getOK(t, b, "op", "/api/tasks?since=2099-01-01T00:00:00Z"); strings.Contains(body, "t-alice") {
		t.Errorf("date filter wrong: %s", body)
	}
}

// TestTaskDetailIsScopedAndEnriched proves R5 + R6 together: the walking
// skeleton's UNSCOPED task read is closed (a member reading another owner's
// task is 403, an unknown task 404), and the detail carries the spec's numbered
// ACs verbatim, the plan's coverage map, the stage progress derived from the
// log, lineage in both directions and the receipt served verbatim.
func TestTaskDetailIsScopedAndEnriched(t *testing.T) {
	b := twoOwners(t)
	exec(t, b, `INSERT INTO receipts (run_id, user_id, usage_json, materialized_ts) VALUES (?,?,?,?)`,
		"r-alice", "alice", `{"run_id":"r-alice","direct_use":{"label":"UNPRICED"}}`, nowTS())

	// R6: the read that previously answered any authenticated identity.
	if code, _ := get(t, b, "bob", "/api/tasks/t-alice"); code != http.StatusForbidden {
		t.Errorf("member reading another owner's task: %d, want 403 (the B2-4 read leaked)", code)
	}
	if code, _ := get(t, b, "alice", "/api/tasks/t-nope"); code != http.StatusNotFound {
		t.Errorf("unknown task: %d, want 404", code)
	}
	if code, _ := get(t, b, "op", "/api/tasks/t-bob"); code != http.StatusOK {
		t.Errorf("operator reading any task: %d, want 200", code)
	}

	pair := mustJSON(t, map[string]any{
		"spec": map[string]any{
			"task_id": "t-alice", "owner": "alice", "version": 1, "status": "approved",
			"restatement": "Write the note",
			"acs": []map[string]any{
				{"n": 1, "plain": "The note exists", "structured": "WHEN asked THEN the note exists", "structured_kind": "ears"},
				{"n": 2, "plain": "It is short"},
			},
			"assumptions":  []map[string]any{{"text": "plain prose is fine", "origin": "planner"}},
			"out_of_scope": []string{"a whole book"},
		},
		"plan": map[string]any{
			"task_id": "t-alice", "owner": "alice", "version": 1, "spec_version": 1,
			"steps":          []map[string]any{{"id": "S-1", "title": "Write it", "done_when": "the note reads well", "class": "C1"}},
			"coverage":       map[string][]string{"AC-1": {"S-1"}, "AC-2": {"S-1"}},
			"research_nodes": []map[string]any{{"rule_id": "R1", "step_id": "S-1"}},
		},
	})

	srv := api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{"alice"},
		Settings: fixedSettings{d: 20 * 1e9},
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       b.db, Meter: fakeMeter{},
		Intake: &fakeSurface{artifacts: json.RawMessage(pair), taskPayload: json.RawMessage(`{"phase":"approved"}`)},
	})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/tasks/t-alice", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	var d struct {
		Spec *struct {
			Restatement string `json:"restatement"`
			ACs         []struct {
				N              int    `json:"n"`
				Plain          string `json:"plain"`
				Structured     string `json:"structured"`
				StructuredKind string `json:"structured_kind"`
			} `json:"acs"`
			Assumptions []struct{ Text string } `json:"assumptions"`
			OutOfScope  []string                `json:"out_of_scope"`
		} `json:"spec"`
		Plan *struct {
			Steps []struct {
				ID       string `json:"id"`
				DoneWhen string `json:"done_when"`
			} `json:"steps"`
			Coverage      map[string][]string `json:"coverage"`
			ResearchNodes []struct {
				RuleID string `json:"rule_id"`
			} `json:"research_nodes"`
		} `json:"plan"`
		StageProgress []struct {
			Stage   string `json:"stage"`
			Kind    string `json:"kind"`
			Outcome string `json:"outcome"`
			Type    string `json:"type"`
		} `json:"stage_progress"`
		Lineage struct {
			Project        string `json:"project"`
			ProjectChoices int64  `json:"project_choices"`
		} `json:"lineage"`
		Runs []struct {
			RunID         string          `json:"run_id"`
			Receipt       json.RawMessage `json:"receipt"`
			ReceiptAbsent string          `json:"receipt_absent"`
		} `json:"runs"`
		Cursor   int64           `json:"cursor"`
		Pipeline json.RawMessage `json:"pipeline"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode: %v (%s)", err, rr.Body)
	}
	if d.Spec == nil || len(d.Spec.ACs) != 2 {
		t.Fatalf("spec ACs missing: %s", rr.Body)
	}
	if d.Spec.ACs[0].N != 1 || d.Spec.ACs[0].Plain != "The note exists" ||
		d.Spec.ACs[0].Structured != "WHEN asked THEN the note exists" || d.Spec.ACs[0].StructuredKind != "ears" {
		t.Errorf("AC-1 must render numbered and verbatim in BOTH phrasings (G1 P10): %+v", d.Spec.ACs[0])
	}
	if d.Spec.ACs[1].Structured != "" {
		t.Error("an AC with no structured phrasing must not gain one")
	}
	if len(d.Spec.Assumptions) != 1 || len(d.Spec.OutOfScope) != 1 {
		t.Errorf("assumptions/out-of-scope missing: %+v", d.Spec)
	}
	if d.Plan == nil || len(d.Plan.Steps) != 1 || d.Plan.Steps[0].DoneWhen == "" ||
		len(d.Plan.Coverage) != 2 || len(d.Plan.ResearchNodes) != 1 {
		t.Errorf("plan steps/coverage/research nodes missing: %+v", d.Plan)
	}
	if len(d.StageProgress) != 2 {
		t.Fatalf("stage progress = %+v, want the started/finished pair derived from the log", d.StageProgress)
	}
	if d.StageProgress[0].Stage != "S-1" || d.StageProgress[0].Kind != "execute" || d.StageProgress[0].Outcome != "" {
		t.Errorf("stage progress opening = %+v", d.StageProgress[0])
	}
	if d.StageProgress[1].Outcome != "completed" || d.StageProgress[1].Type != "stage.finished" {
		t.Errorf("stage progress close = %+v", d.StageProgress[1])
	}
	if d.Lineage.Project != "proj-alice" || d.Lineage.ProjectChoices != 1 {
		t.Errorf("lineage = %+v", d.Lineage)
	}
	if len(d.Runs) != 1 || !strings.Contains(string(d.Runs[0].Receipt), "direct_use") {
		t.Errorf("the per-run receipt must be served verbatim from usage_json: %+v", d.Runs)
	}
	if d.Cursor == 0 {
		t.Error("the detail must carry its tail cursor (R18)")
	}
	if !strings.Contains(string(d.Pipeline), "approved") {
		t.Error("the pipeline's own view rides the detail")
	}
}

// TestTaskDetailRendersHonestAbsences: a task before its drafting stage and a
// run before its receipt render as ABSENCES with reasons — never an error that
// hides the task, and never a fabricated artifact.
func TestTaskDetailRendersHonestAbsences(t *testing.T) {
	b := twoOwners(t)
	var d struct {
		TaskID          string `json:"task_id"`
		Spec            any    `json:"spec"`
		ArtifactsAbsent string `json:"artifacts_absent"`
		Lineage         struct {
			Project string `json:"project"`
		} `json:"lineage"`
		Runs []struct {
			ReceiptAbsent string `json:"receipt_absent"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(getOK(t, b, "bob", "/api/tasks/t-bob")), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.TaskID != "t-bob" {
		t.Fatalf("the task must still read: %+v", d)
	}
	if d.Spec != nil || d.ArtifactsAbsent == "" {
		t.Errorf("a task with no drafted pair needs a stated absence, got spec=%v reason=%q", d.Spec, d.ArtifactsAbsent)
	}
	if d.Lineage.Project != "(no project)" {
		t.Errorf("project = %q, want the honest bucket", d.Lineage.Project)
	}
	if len(d.Runs) != 1 || d.Runs[0].ReceiptAbsent == "" {
		t.Errorf("a run with no receipt needs a stated absence, not a zero: %+v", d.Runs)
	}
}

// TestReceiptReadIsOwnerScoped closes the second walking-skeleton read (R6).
func TestReceiptReadIsOwnerScoped(t *testing.T) {
	b := twoOwners(t)
	newSrv := func(who string) *api.Server {
		return api.New(api.Config{
			Log: b.log, Sessions: b.store, Auth: fixedIdentity{who},
			Settings: fixedSettings{d: 20 * 1e9},
			HealthFn: func() api.Health { return api.Health{Ready: true} },
			DB:       b.db, Intake: &fakeSurface{},
		})
	}
	for _, c := range []struct {
		who, run string
		want     int
	}{
		{"alice", "r-alice", http.StatusOK},
		{"alice", "r-bob", http.StatusForbidden},
		{"alice", "r-nope", http.StatusNotFound},
		{"op", "r-bob", http.StatusOK},
	} {
		rr := httptest.NewRecorder()
		newSrv(c.who).Handler().ServeHTTP(rr, httptest.NewRequest("GET", "/api/runs/"+c.run+"/receipt", nil))
		if rr.Code != c.want {
			t.Errorf("%s GET /api/runs/%s/receipt = %d, want %d: %s", c.who, c.run, rr.Code, c.want, rr.Body)
		}
	}
}

// TestReadShapesNeverPercent extends the §30 progress-semantics NEGATIVE to
// every new REST read shape: no percent/fraction/ratio/progress/ETA key exists
// anywhere in them.
func TestReadShapesNeverPercent(t *testing.T) {
	b := twoOwners(t)
	forbiddenExact := map[string]bool{
		"percent": true, "percentage": true, "percent_complete": true,
		"fraction": true, "complete_fraction": true, "ratio": true,
		"progress": true, "pct": true, "complete": true, "completion": true,
		"eta": true, "eta_s": true, "eta_seconds": true,
	}
	keys := map[string]bool{}
	for _, path := range []string{"/api/runs", "/api/runs/r-alice", "/api/tasks", "/api/tasks/t-alice"} {
		var body any
		if err := json.Unmarshal([]byte(getOK(t, b, "op", path)), &body); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		collectKeys(body, keys)
	}
	if len(keys) == 0 {
		t.Fatal("collected no keys — the scan proves nothing")
	}
	for k := range keys {
		lk := strings.ToLower(k)
		if forbiddenExact[lk] || strings.Contains(lk, "percent") {
			t.Fatalf("a REST read shape exposes forbidden progress key %q — the never-percent negative (§30)", k)
		}
	}
}

// TestReadRoutesAddNoMutatingVerb pins R19: the read families answer GET only.
// A mutating method on a read path must not be routed here — the decision plane
// is B6-2's.
func TestReadRoutesAddNoMutatingVerb(t *testing.T) {
	b := twoOwners(t)
	srv := api.New(api.Config{
		Log: b.log, Sessions: b.store, Auth: fixedIdentity{"op"},
		Settings: fixedSettings{d: 20 * 1e9},
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       b.db, Intake: &fakeSurface{},
	})
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/runs"}, {"DELETE", "/api/runs/r-alice"}, {"PATCH", "/api/runs/r-alice"},
		{"POST", "/api/tasks"}, {"PUT", "/api/tasks/t-alice"},
	} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest(c.method, c.path, nil))
		if rr.Code != http.StatusMethodNotAllowed && rr.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d — the read families add no mutating verb (R19)", c.method, c.path, rr.Code)
		}
	}
	if strings.Contains(mustString(t, "/v1"), "no") {
		t.Fatal("unreachable")
	}
}

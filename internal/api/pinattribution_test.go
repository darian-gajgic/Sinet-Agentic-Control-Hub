package api_test

// pinattribution_test.go — P3-RW-3 acceptance: the durable intake-time project
// pin reaches the task list.
//
// The S06.2 registry match happens at TRIAGE and lands on the task's first
// `intake.state` event as `$.registry.project`; the S02.8 claims that
// `task_project` collapsed are written only at PLAN APPROVAL. So a pinned task
// read "(no project)" on the operator's board for its whole pre-approval life.
// These tests are the read-side contract: attribution from birth, the honest
// bucket for genuinely unmatched work, one expression behind both the served
// column and the filter, and both serving edges redacting.

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/history"
)

// pinnedProject is the registry id the pinned world resolves.
const pinnedProject = "proj-a"

// seedIntakeState plants one `intake.state` row by direct INSERT — the
// apifixtures precedent (§42): api tests seed run_events rows directly, while
// production appends ride the envelope. The payload is the marshaled
// intake.State shape at the one key this packet reads.
func seedIntakeState(t *testing.T, b *backend, runID, owner, payload string) {
	t.Helper()
	exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
	            VALUES (?,0,?,'intake.state',1,?,?)`, runID, owner, payload, nowTS())
}

// pinPayload is an intake.state body carrying a resolved registry slice.
func pinPayload(project string) string {
	raw, err := json.Marshal(map[string]any{
		"phase": "triage",
		"registry": map[string]any{
			"project": project,
			"ref":     "registry-slice/" + project,
		},
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// pinnedWorld is an operator, one member, and three tasks that differ ONLY in
// their attribution edge — so an assertion about the project cannot pass for a
// reason about anything else. No artifact_claims row exists anywhere in it:
// every task here is pre-approval.
func pinnedWorld(t *testing.T) *backend {
	t.Helper()
	b := newBackend(t)
	seedUser(t, b, "op", auth.RoleOperator)
	seedUser(t, b, "alice", auth.RoleMember)

	// The pinned task: triage resolved a registry slice, nothing is approved.
	seedTask(t, b, "t-pin", "alice", "Pinned at intake", "doing")
	seedRun(t, b, "r-pin", "alice", "t-pin", "running", "anthropic")
	seedIntakeState(t, b, "r-pin", "alice", pinPayload(pinnedProject))

	// The control: an intake.state with no registry key at all.
	seedTask(t, b, "t-plain", "alice", "Unmatched at intake", "doing")
	seedRun(t, b, "r-plain", "alice", "t-plain", "running", "anthropic")
	seedIntakeState(t, b, "r-plain", "alice", `{"phase":"triage"}`)

	// The second control: a task with no intake run at all.
	seedTask(t, b, "t-bare", "alice", "No pipeline state", "intake")

	assertNoClaims(t, b)
	return b
}

// assertNoClaims keeps the pre-approval premise honest: if a claim row existed,
// every assertion below would be measuring the 0016 claims collapse instead of
// the pin edge.
func assertNoClaims(t *testing.T, b *backend) {
	t.Helper()
	var n int
	if err := b.db.QueryRowContext(t.Context(), `SELECT count(*) FROM artifact_claims`).Scan(&n); err != nil {
		t.Fatalf("count artifact_claims: %v", err)
	}
	if n != 0 {
		t.Fatalf("the pre-approval world holds %d artifact_claims rows — the pin edge would not be what is measured", n)
	}
}

// listProjects decodes GET /api/tasks into task id → served project.
func listProjects(t *testing.T, b *backend, who, query string) map[string]string {
	t.Helper()
	var out struct {
		Tasks []struct {
			TaskID  string `json:"task_id"`
			Project string `json:"project"`
		} `json:"tasks"`
	}
	body := getOK(t, b, who, "/api/tasks"+query)
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode task list: %v: %s", err, body)
	}
	got := map[string]string{}
	for _, tk := range out.Tasks {
		got[tk.TaskID] = tk.Project
	}
	return got
}

// detailLineage decodes GET /api/tasks/{task} down to its lineage block.
func detailLineage(t *testing.T, b *backend, who, taskID string) (string, int64) {
	t.Helper()
	var out struct {
		Lineage struct {
			Project        string `json:"project"`
			ProjectChoices int64  `json:"project_choices"`
		} `json:"lineage"`
	}
	body := getOK(t, b, who, "/api/tasks/"+taskID)
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode task detail: %v: %s", err, body)
	}
	return out.Lineage.Project, out.Lineage.ProjectChoices
}

// TestTaskListServesThePinBeforeApproval — R1. A task whose latest intake.state
// carries a non-empty $.registry.project serves that id from the moment triage
// wrote the event, with zero artifact_claims rows in the database.
//
// Non-tautological in the direction that matters: the pinned task's row must be
// PRESENT and asserted by id, so an empty list cannot pass for an attributed
// one, and the unpinned control must still read the honest bucket.
func TestTaskListServesThePinBeforeApproval(t *testing.T) {
	b := pinnedWorld(t)

	got := listProjects(t, b, "alice", "")
	if _, ok := got["t-pin"]; !ok {
		t.Fatalf("the pinned task is missing from the list entirely: %v", got)
	}
	if got["t-pin"] != pinnedProject {
		t.Errorf("pinned task project = %q, want %q before any plan approval (R1)", got["t-pin"], pinnedProject)
	}
	if got["t-plain"] != noProjectBucketToken {
		t.Errorf("unpinned control project = %q, want %q (R2)", got["t-plain"], noProjectBucketToken)
	}
	if got["t-bare"] != noProjectBucketToken {
		t.Errorf("task with no intake state = %q, want %q (R2)", got["t-bare"], noProjectBucketToken)
	}
	// The operator's own read of the same world agrees.
	if op := listProjects(t, b, "op", ""); op["t-pin"] != pinnedProject {
		t.Errorf("operator reads pinned task project = %q, want %q", op["t-pin"], pinnedProject)
	}
}

// TestTaskListProjectFilterMatchesPinnedUnapproved — R4. The filter clause and
// the served column are ONE resolved expression, so `?project=` can never
// disagree with what the same request serves.
func TestTaskListProjectFilterMatchesPinnedUnapproved(t *testing.T) {
	b := pinnedWorld(t)

	byPin := listProjects(t, b, "alice", "?project="+url.QueryEscape(pinnedProject))
	if _, ok := byPin["t-pin"]; !ok {
		t.Errorf("?project=%s does not match the pinned task (R4): %v", pinnedProject, byPin)
	}
	for _, id := range []string{"t-plain", "t-bare"} {
		if _, ok := byPin[id]; ok {
			t.Errorf("?project=%s returned the unpinned task %q", pinnedProject, id)
		}
	}

	byBucket := listProjects(t, b, "alice", "?project="+url.QueryEscape(noProjectBucketToken))
	for _, id := range []string{"t-plain", "t-bare"} {
		if _, ok := byBucket[id]; !ok {
			t.Errorf("?project=%q does not match the unattributed task %q — the bucket must stay selectable (R2/R4)", noProjectBucketToken, id)
		}
	}
	if _, ok := byBucket["t-pin"]; ok {
		t.Errorf("?project=%q returned the PINNED task — the bucket is for genuinely unmatched work", noProjectBucketToken)
	}
}

// TestTaskDetailLineageAgreesWithTheList — R5. Two surfaces, one attribution:
// the list and the detail read the same source expression and may never
// disagree about a task's project.
func TestTaskDetailLineageAgreesWithTheList(t *testing.T) {
	b := pinnedWorld(t)

	project, choices := detailLineage(t, b, "alice", "t-pin")
	if project != pinnedProject {
		t.Errorf("lineage.project = %q, want %q (R1/R5)", project, pinnedProject)
	}
	if choices != 1 {
		t.Errorf("lineage.project_choices = %d, want 1 — a pin resolves exactly one project", choices)
	}
	if list := listProjects(t, b, "alice", "")["t-pin"]; list != project {
		t.Errorf("the two surfaces disagree: list %q vs detail %q (R5)", list, project)
	}

	// The honest bucket reads the same on both surfaces too.
	plain, plainChoices := detailLineage(t, b, "alice", "t-plain")
	if plain != noProjectBucketToken || plainChoices != 1 {
		t.Errorf("unattributed detail lineage = %q/%d, want %q/1 (R2)", plain, plainChoices, noProjectBucketToken)
	}
}

// TestApprovalDoesNotFlickerAttribution — R6. The claim's project IS
// `st.Registry.Project` by construction (internal/intake/answer.go), so pin and
// claim can never disagree: approval must not move the served value.
func TestApprovalDoesNotFlickerAttribution(t *testing.T) {
	b := pinnedWorld(t)

	before := listProjects(t, b, "alice", "")["t-pin"]
	if before != pinnedProject {
		t.Fatalf("pre-approval project = %q, want %q — the flicker test needs the pin to land first", before, pinnedProject)
	}

	// The approval-time write, in the shape writeClaimsTx produces.
	exec(t, b, `INSERT INTO artifact_claims (task_id, project, user_id, path_globs, mode, status, created_ts)
	            VALUES (?,?,?,?,?,?,?)`, "t-pin", pinnedProject, "alice", `["**"]`, "W", "active", nowTS())

	after := listProjects(t, b, "alice", "")["t-pin"]
	if after != before {
		t.Errorf("attribution flickered at approval: %q → %q (R6)", before, after)
	}
	project, choices := detailLineage(t, b, "alice", "t-pin")
	if project != pinnedProject || choices != 1 {
		t.Errorf("post-approval lineage = %q/%d, want %q/1 (R6)", project, choices, pinnedProject)
	}
}

// TestNoProjectBucketStaysHonest — R2. Absence is RENDERED, never fabricated
// and never dropped: an empty pin and an empty claim both land in the bucket,
// and neither inherits the pinned task's id.
func TestNoProjectBucketStaysHonest(t *testing.T) {
	b := pinnedWorld(t)

	// An intake.state whose registry slice resolved to no project id.
	seedTask(t, b, "t-empty-pin", "alice", "Registry present, project empty", "doing")
	seedRun(t, b, "r-empty-pin", "alice", "t-empty-pin", "running", "anthropic")
	seedIntakeState(t, b, "r-empty-pin", "alice", pinPayload(""))

	// A claim row written with the '' project the 0016 collapse excludes.
	seedTask(t, b, "t-empty-claim", "alice", "Claimed with no project", "doing")
	exec(t, b, `INSERT INTO artifact_claims (task_id, project, user_id, path_globs, mode, status, created_ts)
	            VALUES (?,?,?,?,?,?,?)`, "t-empty-claim", "", "alice", `["**"]`, "W", "active", nowTS())

	got := listProjects(t, b, "alice", "")
	for _, id := range []string{"t-empty-pin", "t-empty-claim", "t-plain", "t-bare"} {
		v, ok := got[id]
		if !ok {
			t.Errorf("task %q dropped out of the list — an absence is rendered, never dropped (R2)", id)
			continue
		}
		if v != noProjectBucketToken {
			t.Errorf("task %q project = %q, want %q — no task inherits another's pin (R2)", id, v, noProjectBucketToken)
		}
	}
	if got["t-pin"] != pinnedProject {
		t.Errorf("the genuinely pinned task lost its project while the bucket was asserted: %q", got["t-pin"])
	}
}

// TestPinDerivedProjectRedactsAtTheEdge — R10. The pin is lifted out of a
// run_events PAYLOAD, so it is payload-derived and must pass the codor-C2
// primitive at BOTH serving edges: the list rides writeReadRedacted, and the
// task detail — which goes out UNWRAPPED (§38 ruling (b)) — must redact per
// value, the D10 redactStageProgress shape. Stored rows stay verbatim.
func TestPinDerivedProjectRedactsAtTheEdge(t *testing.T) {
	const planted = "sk-ant-api03-PLANTEDSECRETVALUE0123456789"
	const marker = "[REDACTED:anthropic_key]"

	b := newBackend(t)
	seedUser(t, b, "alice", auth.RoleMember)
	seedTask(t, b, "t-secret", "alice", "A task whose pin carries a secret", "doing")
	seedRun(t, b, "r-secret", "alice", "t-secret", "running", "anthropic")
	seedIntakeState(t, b, "r-secret", "alice", pinPayload(planted))

	list := getOK(t, b, "alice", "/api/tasks")
	if strings.Contains(list, planted) {
		t.Errorf("the task list served the planted secret verbatim (R10):\n%s", list)
	}
	if !strings.Contains(list, marker) {
		t.Errorf("the task list did not carry %s — the pin value must redact at the edge:\n%s", marker, list)
	}

	detail := getOK(t, b, "alice", "/api/tasks/t-secret")
	if strings.Contains(detail, planted) {
		t.Errorf("the UNWRAPPED task detail served the planted secret verbatim (R10):\n%s", detail)
	}
	if !strings.Contains(detail, marker) {
		t.Errorf("the task detail lineage did not carry %s:\n%s", marker, detail)
	}

	// Store-raw / serve-redacted: the event row keeps the value it recorded.
	var stored string
	if err := b.db.QueryRowContext(t.Context(),
		`SELECT payload FROM run_events WHERE run_id = 'r-secret' AND type = 'intake.state'`).Scan(&stored); err != nil {
		t.Fatalf("read stored payload: %v", err)
	}
	if !strings.Contains(stored, planted) {
		t.Errorf("the stored run_events payload was rewritten — redaction happens at the edge, never in the log: %s", stored)
	}
}

// TestTaskProjectStaysOperatorOnly — R11. The re-created view still carries NO
// owner column, so the history registry keeps it operator-only at Layers 0 and
// 2 and the §38 D5 transport pre-check keeps answering 403 on registry
// METADATA rather than on an error string.
func TestTaskProjectStaysOperatorOnly(t *testing.T) {
	b := pinnedWorld(t)

	v, ok := history.ViewByName("task_project")
	if !ok {
		t.Fatal("task_project is not in the Layer-0 registry")
	}
	if v.OwnerColumn != "" {
		t.Errorf("task_project.OwnerColumn = %q, want empty — the view has no owner column and is operator-only (R11)", v.OwnerColumn)
	}

	code, body := historyGet(t, b, "alice", "/api/events/views/task_project")
	if code != http.StatusForbidden {
		t.Errorf("member reading task_project = %d, want 403 (R11, §38 D5): %s", code, body)
	}
	code, body = historyGet(t, b, "op", "/api/events/views/task_project")
	if code != http.StatusOK {
		t.Fatalf("operator reading task_project = %d, want 200: %s", code, body)
	}
	// Non-tautological: the operator's answer must actually carry the pinned
	// edge, so a view that returned nothing could not pass for an honest one.
	if !strings.Contains(body, pinnedProject) {
		t.Errorf("the operator's task_project rows do not carry the pinned project %q: %s", pinnedProject, body)
	}
}

// TestPinnedProjectIsIndistinguishableOnTheWire — OQ-(c). ONE `project` field
// carries the attribution whatever resolved it; provenance is stated in the
// view text, never as a wire discriminator. A `project_source`-shaped field
// would encode only "approved yet?", which the kanban status already says.
func TestPinnedProjectIsIndistinguishableOnTheWire(t *testing.T) {
	b := pinnedWorld(t)

	var out struct {
		Tasks []map[string]json.RawMessage `json:"tasks"`
	}
	body := getOK(t, b, "alice", "/api/tasks")
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode task list: %v", err)
	}
	if len(out.Tasks) == 0 {
		t.Fatal("the task list is empty — the field inventory would be vacuous")
	}
	for _, tk := range out.Tasks {
		for _, banned := range []string{"project_source", "project_provenance", "project_origin", "project_pinned"} {
			if _, ok := tk[banned]; ok {
				t.Errorf("the served task carries %q — attribution is ONE indistinguishable field (OQ-(c))", banned)
			}
		}
		if _, ok := tk["project"]; !ok {
			t.Errorf("the served task carries no `project` field at all: %v", tk)
		}
	}
}

// TestBoardSnapshotCarriesNoProject — OQ-(d)/R13. The `board` SSE snapshot stays
// byte-for-byte as it was: the SPA renders from REST and frames are triggers
// (web/src/live.ts), and `intake.state` is already a board trigger, so the pin
// is live at birth with no snapshot change. `reads.go`'s byte-for-byte promise
// stays literally true.
func TestBoardSnapshotCarriesNoProject(t *testing.T) {
	raw, err := json.Marshal(api.BoardTask{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["project"]; ok {
		t.Errorf("BoardTask gained a `project` field — the board snapshot stays as-is (OQ-(d), R13): %s", raw)
	}
}

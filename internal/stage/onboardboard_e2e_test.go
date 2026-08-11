package stage_test

// onboardboard_e2e_test.go — P3-RW-7 acceptance on the producer side: an
// onboarding task renders on the board with an HONEST declared column and its
// project's name from the moment it exists, through approval.
//
// THE TWO DISHONEST VALUES this packet removes (brief §1):
//   - `kanban_status` was `''` — 0001's schema default, because StartOnboarding
//     was the one task producer that omitted the column. The board's forward
//     tolerance rendered that as a raw column titled "(no status recorded)"
//     (web/src/kanban.ts:60) — the checkpoint-2 C2-6 wound, re-created live by
//     every project the operator onboards.
//   - `project` was "(no project)" — for the very task whose id NAMES its
//     project (`onboard-<p>`), because 0022's task_project had no arm that
//     could see it.
//
// The rig drives the REAL chain: the real projects create door
// (POST /api/projects → api.OnboardSurface → project.OnboardStart +
// Skeleton.StartOnboarding, mirroring internal/shell's onboardDoor), the real
// scheduler dispatch, the real durable ask, the real owner answer through the
// stage Surface, and the real API reads (task list, task detail, SSE board
// snapshot) over the same migrated database.
//
// Committed RED per P3/briefs/P3-RW-7.md §8: T1, T2, T3 and T6 assert behavior
// that does not exist yet. T8's DO-NOTHING limb is born GREEN — `ON CONFLICT
// (task_id) DO NOTHING` already cannot overwrite an advanced column, and the
// assertion exists so it stays that way when the INSERT grows a status.

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
)

// The two declared columns this packet's lifecycle uses, named here from the
// board's own vocabulary (web/src/kanban.ts `declared`): `intake` renders as
// **Backlog** and `done` as **Done**, which stays visible (map §3 v3, D-B).
const (
	boardBirthColumn    = "intake"
	boardApprovedColumn = "done"
)

// ── the api rig over the real stage/project composition ─────────────────────

// boardIdentity authenticates every request as one non-dev user, so owner
// scope reads that user's role from the auth store (api_test's fixedIdentity).
type boardIdentity struct{ id string }

func (b boardIdentity) Authenticate(*http.Request) (api.Identity, error) {
	return api.Identity{UserID: b.id}, nil
}

// boardSettings serves one duration and one integer for every ⚙ key (the SSE
// keepalive is the only consumer these reads reach).
type boardSettings struct{}

func (boardSettings) Duration(string) (time.Duration, error) { return 20 * time.Second, nil }
func (boardSettings) Int(string) (int64, error)              { return 0, nil }

// boardMeter is the metering seam the task list reads a run's cost through.
type boardMeter struct{}

func (boardMeter) RunMeter(context.Context, string) (api.RunMeter, error) {
	return api.RunMeter{Unpriced: true}, nil
}

func (boardMeter) LaneMeter(context.Context, string, string) (api.LaneMeter, error) {
	return api.LaneMeter{}, nil
}

// onboardDoorOver is internal/shell's production `onboardDoor`
// (shell/project_seams.go): register + draft the entry, then launch the
// onboarding task. It is restated here for the same reason `registryOver` is —
// this is the composition test of what the root composes.
type onboardDoorOver struct {
	proj *project.Store
	sk   *stage.Skeleton
}

var _ api.OnboardSurface = onboardDoorOver{}

func (d onboardDoorOver) OnboardRefs(projectID string) api.OnboardRefs {
	return api.OnboardRefs{TaskID: stage.OnboardTaskID(projectID), AskRef: stage.OnboardAskID(projectID)}
}

func (d onboardDoorOver) StartOnboarding(ctx context.Context, owner, projectID, name, remoteURL string) (api.OnboardRefs, error) {
	if _, err := d.proj.OnboardStart(ctx, project.OnboardInput{
		ProjectID: projectID, Owner: owner, Name: name, RemoteURL: remoteURL,
	}); err != nil {
		return api.OnboardRefs{}, err
	}
	// Source stays EMPTY: over HTTP a clone source is a host-filesystem read
	// primitive, so the door cannot express one (P3-RW-2 OQ5).
	taskID, err := d.sk.StartOnboarding(ctx, owner, projectID, name, "")
	if err != nil {
		return api.OnboardRefs{}, err
	}
	return api.OnboardRefs{TaskID: taskID, AskRef: stage.OnboardAskID(projectID)}, nil
}

// boardServer builds the read+create surface over the harness's own database.
func (h *projHarness) boardServer(who string) *api.Server {
	return api.New(api.Config{
		Log:      h.log,
		Sessions: auth.New(h.db, h.log),
		Auth:     boardIdentity{who},
		Settings: boardSettings{},
		HealthFn: func() api.Health { return api.Health{Ready: true, Mode: "running", Version: "test"} },
		DB:       h.db,
		Meter:    boardMeter{},
		Onboard:  onboardDoorOver{proj: h.proj, sk: h.sk},
		Intake:   h.sur,
	})
}

func (h *projHarness) call(t *testing.T, who, method, path, body string) (int, string) {
	t.Helper()
	var rdr *strings.Reader
	if body == "" {
		rdr = strings.NewReader("")
	} else {
		rdr = strings.NewReader(body)
	}
	rr := httptest.NewRecorder()
	h.boardServer(who).Handler().ServeHTTP(rr, httptest.NewRequest(method, path, rdr))
	return rr.Code, rr.Body.String()
}

func (h *projHarness) callOK(t *testing.T, who, method, path, body string) string {
	t.Helper()
	code, out := h.call(t, who, method, path, body)
	if code != http.StatusOK {
		t.Fatalf("%s %s as %s: status %d, want 200: %s", method, path, who, code, out)
	}
	return out
}

// boardCard is what the operator's board reads about one task: the two values
// this packet is about, plus the id.
type boardCard struct {
	TaskID       string `json:"task_id"`
	KanbanStatus string `json:"kanban_status"`
	Project      string `json:"project"`
}

// listBoard reads GET /api/tasks and returns the cards by task id.
func (h *projHarness) listBoard(t *testing.T, who, query string) map[string]boardCard {
	t.Helper()
	var out struct {
		Tasks []boardCard `json:"tasks"`
	}
	body := h.callOK(t, who, "GET", "/api/tasks"+query, "")
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode task list: %v: %s", err, body)
	}
	got := map[string]boardCard{}
	for _, c := range out.Tasks {
		got[c.TaskID] = c
	}
	return got
}

// snapshotBoardStatus reads the SSE `board` topic snapshot — the projection the
// live board bootstraps from — and returns the task's stored status.
func (h *projHarness) snapshotBoardStatus(t *testing.T, who, taskID string) (string, bool) {
	t.Helper()
	ts := httptest.NewServer(h.boardServer(who).Handler())
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events?topics=board", nil)
	if err != nil {
		t.Fatalf("board snapshot request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("board snapshot: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("board snapshot: status %d", resp.StatusCode)
	}
	br := bufio.NewReader(resp.Body)
	var event, data string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("board snapshot stream ended before a snapshot frame: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			event = line[len("event: "):]
		case strings.HasPrefix(line, "data: "):
			data = line[len("data: "):]
		case line == "" && event == "snapshot" && data != "":
			var snap struct {
				Topic string `json:"topic"`
				State struct {
					Tasks []struct {
						TaskID       string `json:"task_id"`
						KanbanStatus string `json:"kanban_status"`
					} `json:"tasks"`
				} `json:"state"`
			}
			if err := json.Unmarshal([]byte(data), &snap); err != nil {
				t.Fatalf("decode board snapshot: %v: %s", err, data)
			}
			for _, task := range snap.State.Tasks {
				if task.TaskID == taskID {
					return task.KanbanStatus, true
				}
			}
			return "", false
		case line == "":
			event, data = "", ""
		}
	}
}

// storedKanban reads the column straight out of the row, so a test can tell a
// serving-layer coalesce from a stored value.
func (h *projHarness) storedKanban(t *testing.T, taskID string) string {
	t.Helper()
	var status string
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT kanban_status FROM tasks WHERE task_id = ?`, taskID).Scan(&status); err != nil {
		t.Fatalf("read stored kanban_status of %s: %v", taskID, err)
	}
	return status
}

// createProject drives the real create door and returns the onboarding task id.
func (h *projHarness) createProject(t *testing.T, owner, projectID, name string) string {
	t.Helper()
	out := h.callOK(t, owner, "POST", "/api/projects",
		fmt.Sprintf(`{"project_id":%q,"name":%q}`, projectID, name))
	var started struct {
		TaskID string `json:"task_id"`
		AskRef string `json:"ask_ref"`
	}
	if err := json.Unmarshal([]byte(out), &started); err != nil {
		t.Fatalf("decode create response: %v: %s", err, out)
	}
	if started.TaskID != stage.OnboardTaskID(projectID) {
		t.Fatalf("create door named the task %q, want %q", started.TaskID, stage.OnboardTaskID(projectID))
	}
	return started.TaskID
}

// ── T1: the door → the board ────────────────────────────────────────────────

// TestOnboardingTaskIsBornOnTheBoard — brief T1, checklist 1/2/3. The FIRST
// read after the create door returns carries a declared column and the
// project's own name. Nothing has ticked yet: this is the birth state.
func TestOnboardingTaskIsBornOnTheBoard(t *testing.T) {
	h := newProjectHarness(t)
	const owner = "alice"
	taskID := h.createProject(t, owner, "shop", "Shop backend")

	cards := h.listBoard(t, owner, "")
	card, ok := cards[taskID]
	if !ok {
		t.Fatalf("the onboarding task is not on the board at all: %v", cards)
	}
	if card.KanbanStatus != boardBirthColumn {
		t.Errorf("onboarding task is born in column %q, want %q — an empty status renders as the raw column "+
			"'(no status recorded)' in front of the operator (checklist 1/2)", card.KanbanStatus, boardBirthColumn)
	}
	if card.Project != "shop" {
		t.Errorf("onboarding task serves project %q, want %q — the task whose id names its project must not read "+
			"'(no project)' (checklist 3)", card.Project, "shop")
	}
	// The column is STORED, not coalesced at the serving edge: every reader of
	// the tasks row (the SSE projection, the canned history query, cost_per_task)
	// must see the same value.
	if stored := h.storedKanban(t, taskID); stored != boardBirthColumn {
		t.Errorf("stored kanban_status = %q, want %q — the value belongs in the row the producers write", stored, boardBirthColumn)
	}
	// The live board bootstraps from the SSE snapshot, so it must agree.
	status, found := h.snapshotBoardStatus(t, owner, taskID)
	if !found {
		t.Fatalf("the onboarding task is missing from the SSE board snapshot")
	}
	if status != boardBirthColumn {
		t.Errorf("SSE board snapshot carries %q for the onboarding task, want %q", status, boardBirthColumn)
	}
}

// ── T2: through approval ────────────────────────────────────────────────────

// TestOnboardingTaskAdvancesOnApproval — brief T2, checklist 4. The owner's
// approval ends the onboarding task's work (S13.7: the entry goes active), so
// the column advances to `done` — and the card STAYS on the board, because
// Done stays visible (operator-ratified D-B).
func TestOnboardingTaskAdvancesOnApproval(t *testing.T) {
	h := newProjectHarness(t)
	ctx := context.Background()
	const owner = "alice"
	taskID := h.createProject(t, owner, "shop", "Shop backend")

	// Dispatch parks the run on the durable owner-approval ask.
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the onboarding run", n)
	}
	if got := h.listBoard(t, owner, "")[taskID].KanbanStatus; got != boardBirthColumn {
		t.Errorf("while parked on the approval ask the column is %q, want %q — waiting on the owner is carried by the "+
			"ask (waiting_on_human), not by a fabricated stage", got, boardBirthColumn)
	}

	askID := stage.OnboardAskID("shop")
	if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"approve":true}`), false); err != nil {
		t.Fatalf("owner Answer: %v", err)
	}
	e, err := h.proj.Get(ctx, "shop")
	if err != nil || !e.Active() {
		t.Fatalf("entry after approval = %+v err=%v, want active", e, err)
	}

	cards := h.listBoard(t, owner, "")
	card, ok := cards[taskID]
	if !ok {
		t.Fatalf("the completed onboarding task left the board — Done stays visible (D-B): %v", cards)
	}
	if card.KanbanStatus != boardApprovedColumn {
		t.Errorf("after approval the column is %q, want %q (checklist 4)", card.KanbanStatus, boardApprovedColumn)
	}
	if card.Project != "shop" {
		t.Errorf("after approval the project is %q, want shop — attribution does not flicker", card.Project)
	}
	if stored := h.storedKanban(t, taskID); stored != boardApprovedColumn {
		t.Errorf("stored kanban_status after approval = %q, want %q", stored, boardApprovedColumn)
	}
	status, found := h.snapshotBoardStatus(t, owner, taskID)
	if !found || status != boardApprovedColumn {
		t.Errorf("SSE board snapshot after approval = %q (found=%v), want %q", status, found, boardApprovedColumn)
	}
}

// ── T3: the task-detail overlay's backend ───────────────────────────────────

// TestOnboardingTaskDetailLineage — brief T3. The overlay reads the SAME single
// place as the list, so lineage must name the project; and the spec/plan pair
// is honestly ABSENT rather than fabricated (an onboarding task drafts neither).
func TestOnboardingTaskDetailLineage(t *testing.T) {
	h := newProjectHarness(t)
	const owner = "alice"
	taskID := h.createProject(t, owner, "shop", "Shop backend")

	body := h.callOK(t, owner, "GET", "/api/tasks/"+taskID, "")
	var detail struct {
		KanbanStatus string `json:"kanban_status"`
		Lineage      struct {
			Project        string `json:"project"`
			ProjectChoices int64  `json:"project_choices"`
		} `json:"lineage"`
		ArtifactsAbsent string          `json:"artifacts_absent"`
		Spec            json.RawMessage `json:"spec"`
		Plan            json.RawMessage `json:"plan"`
	}
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatalf("decode task detail: %v: %s", err, body)
	}
	if detail.Lineage.Project != "shop" {
		t.Errorf("task-detail lineage project = %q, want shop — the list and the overlay read ONE place (R3)", detail.Lineage.Project)
	}
	if detail.Lineage.ProjectChoices != 1 {
		t.Errorf("task-detail project_choices = %d, want 1 — a registry id resolves exactly one project", detail.Lineage.ProjectChoices)
	}
	if detail.KanbanStatus != boardBirthColumn {
		t.Errorf("task-detail kanban_status = %q, want %q", detail.KanbanStatus, boardBirthColumn)
	}
	// Honest absence, not fabrication (checklist 6).
	if detail.ArtifactsAbsent == "" {
		t.Errorf("task detail reports no artifacts_absent reason — an onboarding task drafts no spec/plan and the "+
			"absence must be stated: %s", body)
	}
	if len(detail.Spec) != 0 && string(detail.Spec) != "null" {
		t.Errorf("task detail served a spec for an onboarding task: %s", detail.Spec)
	}
	if len(detail.Plan) != 0 && string(detail.Plan) != "null" {
		t.Errorf("task detail served a plan for an onboarding task: %s", detail.Plan)
	}
}

// ── T5/T6: the filters and the honest bucket ────────────────────────────────

// TestOnboardingTaskFiltersAndHonestBucket — brief T6 plus T5's API half
// (checklist 7). The board's project and status axes both find the onboarding
// task, the status axis stops finding it once it advances, and a task with no
// edge of any kind still reads '(no project)' rather than inheriting anything.
func TestOnboardingTaskFiltersAndHonestBucket(t *testing.T) {
	h := newProjectHarness(t)
	ctx := context.Background()
	const owner = "alice"
	taskID := h.createProject(t, owner, "shop", "Shop backend")

	// A plain task of the same owner, with no claims, no pin and no onboard
	// shape: the honest bucket must still hold it.
	h.execTask(t, "t-plain", owner, "A plain task", "executing")

	byProject := h.listBoard(t, owner, "?project=shop")
	if _, ok := byProject[taskID]; !ok {
		t.Errorf("?project=shop does not return the onboarding task: %v", byProject)
	}
	if _, ok := byProject["t-plain"]; ok {
		t.Errorf("?project=shop returned the unattributed task — the filter and the served column are one expression")
	}
	if bucket := h.listBoard(t, owner, "")["t-plain"]; bucket.Project != "(no project)" {
		t.Errorf("an unattributed task serves project %q, want '(no project)' (checklist 7)", bucket.Project)
	}

	byStatus := h.listBoard(t, owner, "?status="+boardBirthColumn)
	if _, ok := byStatus[taskID]; !ok {
		t.Errorf("?status=%s does not return the pre-approval onboarding task: %v", boardBirthColumn, byStatus)
	}

	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the onboarding run", n)
	}
	if _, err := h.sur.Answer(ctx, owner, stage.OnboardAskID("shop"), json.RawMessage(`{"approve":true}`), false); err != nil {
		t.Fatalf("owner Answer: %v", err)
	}
	if _, ok := h.listBoard(t, owner, "?status="+boardBirthColumn)[taskID]; ok {
		t.Errorf("?status=%s still returns the approved onboarding task — the column advanced, so the axis must move with it", boardBirthColumn)
	}
	if _, ok := h.listBoard(t, owner, "?status="+boardApprovedColumn)[taskID]; !ok {
		t.Errorf("?status=%s does not return the approved onboarding task", boardApprovedColumn)
	}
}

// setKanbanDirect plants a column on an existing row, standing in for whatever
// later stage would have advanced it.
func (h *projHarness) setKanbanDirect(t *testing.T, taskID, status string) {
	t.Helper()
	ctx := context.Background()
	if err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE tasks SET kanban_status = ? WHERE task_id = ?`, status, taskID)
		return err
	}); err != nil {
		t.Fatalf("plant kanban_status: %v", err)
	}
}

// execTask inserts a plain task row directly — the control case, born the way
// every non-onboarding producer writes it.
func (h *projHarness) execTask(t *testing.T, taskID, owner, title, status string) {
	t.Helper()
	ctx := context.Background()
	if err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, title, kanban_status, created_ts) VALUES (?, ?, ?, ?, ?)`,
			taskID, owner, title, status, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("seed plain task: %v", err)
	}
}

// ── T8: crash-relaunch idempotence ──────────────────────────────────────────

// TestOnboardingRelaunchNeitherDuplicatesNorRegresses — brief T8. StartOnboarding
// re-run after a crash takes the `ON CONFLICT (task_id) DO NOTHING` path, which
// already cannot overwrite a column a later stage advanced. This limb is born
// GREEN; it exists so that growing the INSERT with a status does not quietly
// turn DO NOTHING into an UPDATE that walks an advanced task backwards.
//
// The relaunch window is PRE-approval, because that is the only window in which
// a relaunch is a real event: once the owner approves, the onboarding run is
// `completed` and the scheduler refuses to re-admit it (the run FSM's own guard,
// not this packet's). The advanced column is planted directly, so the assertion
// is about the INSERT and nothing else.
func TestOnboardingRelaunchNeitherDuplicatesNorRegresses(t *testing.T) {
	h := newProjectHarness(t)
	ctx := context.Background()
	const owner = "alice"
	taskID := h.createProject(t, owner, "shop", "Shop backend")

	h.setKanbanDirect(t, taskID, "executing")
	advanced := h.storedKanban(t, taskID)
	if advanced != "executing" {
		t.Fatalf("planted column = %q, want executing — the assertion below would be vacuous", advanced)
	}

	// The crash-relaunch path: one onboarding task per project.
	if _, err := h.sk.StartOnboarding(ctx, owner, "shop", "Shop backend", ""); err != nil {
		t.Fatalf("relaunch StartOnboarding: %v", err)
	}
	var n int
	if err := h.db.QueryRowContext(ctx, `SELECT count(*) FROM tasks WHERE task_id = ?`, taskID).Scan(&n); err != nil {
		t.Fatalf("count onboarding tasks: %v", err)
	}
	if n != 1 {
		t.Errorf("relaunch produced %d rows for %s, want 1", n, taskID)
	}
	if got := h.storedKanban(t, taskID); got != advanced {
		t.Errorf("relaunch regressed the column from %q to %q — ON CONFLICT DO NOTHING must not walk an advanced task backwards", advanced, got)
	}
}

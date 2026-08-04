package api_test

// actions_test.go — the transport half of the 4.5 cancel verbs and the S13.9
// follow-up spawn (B6-2A). The ratified cancel MAPPING is asserted where it
// lives (internal/stage/cancel_test.go, against real FSM rows and a real fake
// engine session); what is asserted here is the transport contract: identity
// rides, owner scope is server-side and fail-closed, and the verbs are reachable
// only as authenticated human acts.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// fakeCancel records the actor and subject each cancel verb hands to the
// choreography, so the transport's identity plumbing is asserted independently
// of the mapping it drives.
type fakeCancel struct {
	runActor, runID   string
	taskActor, taskID string
	err               error
}

func (f *fakeCancel) CancelRun(_ context.Context, actor, runID string) (json.RawMessage, error) {
	f.runActor, f.runID = actor, runID
	if f.err != nil {
		return nil, f.err
	}
	return json.RawMessage(`{"run_id":"` + runID + `","to":"completed","applied":true}`), nil
}

func (f *fakeCancel) CancelTask(_ context.Context, actor, taskID string) (json.RawMessage, error) {
	f.taskActor, f.taskID = actor, taskID
	if f.err != nil {
		return nil, f.err
	}
	return json.RawMessage(`{"task_id":"` + taskID + `","kanban_status":"cancelled","applied":true}`), nil
}

func TestCancelVerbsRideIdentityAndOwnerScope(t *testing.T) {
	e := newDecisionEnv(t)
	seedTask(t, e.b, "t-alice", "alice", "T", "doing")
	seedRun(t, e.b, "r-alice", "alice", "t-alice", "running", "lane")

	// The OWNER's own cancel must SUCCEED — the non-tautological direction, so
	// an all-refusing handler cannot pass the cross-owner assertion.
	out := e.mustDo(t, "alice", "POST", "/api/runs/r-alice/cancel", "{}")
	if !strings.Contains(out, `"applied":true`) {
		t.Fatalf("the owner's own run cancel must apply; got %s", out)
	}
	if e.cancel.runActor != "alice" || e.cancel.runID != "r-alice" {
		t.Fatalf("the choreography saw actor=%q run=%q, want the authenticated identity",
			e.cancel.runActor, e.cancel.runID)
	}
	// The operator cancels any run (S01.9 role bit).
	e.mustDo(t, "op", "POST", "/api/runs/r-alice/cancel", "{}")
	if e.cancel.runActor != "op" {
		t.Errorf("the operator's cancel must ride the operator's identity; got %q", e.cancel.runActor)
	}
	// Another member: 403, and the choreography is never reached.
	e.cancel.runActor = ""
	if code, body := e.do(t, "bob", "POST", "/api/runs/r-alice/cancel", "{}"); code != http.StatusForbidden {
		t.Errorf("another member cancelling: status %d body %s, want 403", code, body)
	}
	if e.cancel.runActor != "" {
		t.Error("a refused cancel reached the choreography")
	}
	// 404 before 403: an unknown id must not become an existence oracle.
	if code, _ := e.do(t, "bob", "POST", "/api/runs/nope/cancel", "{}"); code != http.StatusNotFound {
		t.Error("an unknown run must answer 404")
	}

	// Same three ways on the task verb.
	out = e.mustDo(t, "alice", "POST", "/api/tasks/t-alice/cancel", "{}")
	if !strings.Contains(out, `"kanban_status":"cancelled"`) {
		t.Fatalf("the task cancel must report the kanban column; got %s", out)
	}
	if code, _ := e.do(t, "bob", "POST", "/api/tasks/t-alice/cancel", "{}"); code != http.StatusForbidden {
		t.Error("another member cancelling a task must be refused")
	}
	if code, _ := e.do(t, "bob", "POST", "/api/tasks/nope/cancel", "{}"); code != http.StatusNotFound {
		t.Error("an unknown task must answer 404")
	}
}

// ── S13.9 follow-up spawn ───────────────────────────────────────────────────

// followUpEnv wires the LANDED intake.FollowUp over the real DB, so the route
// drives the real spawn (successor task + task_successor_of link + the Start
// seam) rather than a stand-in for it.
type followUpEnv struct {
	*decisionEnv
	fu      *intake.FollowUp
	started []intake.Request
}

func newFollowUpEnv(t *testing.T) *followUpEnv {
	t.Helper()
	e := newDecisionEnv(t)
	fe := &followUpEnv{decisionEnv: e}
	fe.fu = &intake.FollowUp{DB: e.b.db, Log: e.b.log}
	// The Start seam is what enters the successor into normal intake; the
	// successor's task row and link are already created by Spawn itself, so
	// this records the entry the composition root drives.
	fe.fu.Start = func(_ context.Context, req intake.Request) error {
		fe.started = append(fe.started, req)
		return nil
	}
	return fe
}

func (e *followUpEnv) srv(t *testing.T, who string) *api.Server {
	t.Helper()
	return api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store, Auth: fixedIdentity{who},
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       e.b.db, Meter: fakeMeter{},
		Intake: e.surface, Effects: e.journal, Cancel: e.cancel, FollowUp: e.fu,
	})
}

// doAs issues one authenticated request against a server.
func doAs(t *testing.T, srv *api.Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr
}

// seedDeliverable writes a deliverable and one numbered revision — the
// composite FK the S13.9 link needs to be referentially real.
func seedDeliverable(t *testing.T, b *backend, id, owner, taskID, project string, revision int) {
	t.Helper()
	exec(t, b, `INSERT INTO deliverables
	     (deliverable_id, user_id, task_id, project_id, dtype, current_revision, state, created_ts, updated_ts)
	     VALUES (?,?,?,?,?,?,?,?,?)`,
		id, owner, taskID, project, "markdown", revision, "in-review", nowTS(), nowTS())
	exec(t, b, `INSERT INTO deliverable_revisions
	     (deliverable_id, n, user_id, run_id, pin_kind, content_sha256, platform_ref, created_ts)
	     VALUES (?,?,?,?,?,?,?,?)`,
		id, revision, owner, taskID+".execute", "content", "sha256:abc", "refs/sinet/rev", nowTS())
}

func TestFollowUpSpawnCreatesTaskLinkAndEntersIntake(t *testing.T) {
	fe := newFollowUpEnv(t)
	seedTask(t, fe.b, "t-src", "alice", "Source", "done")
	seedRun(t, fe.b, "t-src.execute", "alice", "t-src", "completed", "lane")
	seedDeliverable(t, fe.b, "d-1", "alice", "t-src", "proj-a", 1)

	rr := doAs(t, fe.srv(t, "alice"), "POST",
		"/api/deliverables/d-1/follow-up",
		`{"revision":1,"preset":"counterpart","detail":"now the English version","objective":"translate it","title":"EN version"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("spawn: status %d: %s", rr.Code, rr.Body)
	}
	var out api.FollowUpSpawned
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v: %s", err, rr.Body)
	}
	if out.TaskID == "" || out.Owner != "alice" || out.Preset != "counterpart" {
		t.Fatalf("spawn response = %+v", out)
	}
	// The link is real (the composite FK made it so).
	var linkDeliverable string
	var linkRevision int
	if err := fe.b.db.QueryRowContext(context.Background(),
		`SELECT deliverable_id, revision_n FROM task_successor_of WHERE task_id = ?`, out.TaskID).
		Scan(&linkDeliverable, &linkRevision); err != nil {
		t.Fatalf("read task_successor_of: %v", err)
	}
	if linkDeliverable != "d-1" || linkRevision != 1 {
		t.Errorf("link = (%s, %d), want (d-1, 1)", linkDeliverable, linkRevision)
	}
	// It entered normal intake through the root-wired Start seam, carrying the
	// preset framing VERBATIM (S13.9 presets, not schema variants).
	if len(fe.started) != 1 {
		t.Fatalf("the Start seam ran %d times, want 1", len(fe.started))
	}
	req := fe.started[0]
	switch {
	case !strings.Contains(req.Text, "Counterpart of the accepted deliverable"):
		t.Errorf("the preset framing must ride verbatim; got %q", req.Text)
	case !strings.Contains(req.Text, "now the English version"):
		t.Error("the preset detail must ride")
	case !strings.Contains(req.Text, "refs/sinet/deliverable/d-1/rev-1"):
		t.Error("the predecessor ref must ride as brief material")
	case !strings.Contains(req.Text, "proj-a"):
		t.Error("the inherited project context must ride")
	}
	// The canonical audit row for a spawn is the landed intake.followup_spawned
	// — no decision.recorded is double-minted for it (OQ8).
	if n := countEvents(t, fe.b, intake.EventFollowUp); n != 1 {
		t.Errorf("the spawn must land its own canonical row; got %d", n)
	}
	if n := countEvents(t, fe.b, api.EventDecisionRecorded); n != 0 {
		t.Errorf("an act with a landed canonical row must not be double-minted; got %d decision rows", n)
	}

	// Lineage is visible through the B6-1 read, BOTH directions.
	srcDetail := readTaskDetail(t, fe, "alice", "t-src")
	if len(srcDetail.Lineage.SucceededBy) != 1 || srcDetail.Lineage.SucceededBy[0].TaskID != out.TaskID {
		t.Errorf("the source task must be succeeded by the follow-up; got %+v", srcDetail.Lineage)
	}
	sucDetail := readTaskDetail(t, fe, "alice", out.TaskID)
	if len(sucDetail.Lineage.Succeeds) != 1 || sucDetail.Lineage.Succeeds[0].DeliverableID != "d-1" {
		t.Errorf("the successor must succeed the source revision; got %+v", sucDetail.Lineage)
	}
}

func TestFollowUpSpawnCrossOwnerAndBadInput(t *testing.T) {
	fe := newFollowUpEnv(t)
	seedTask(t, fe.b, "t-src", "alice", "Source", "done")
	seedRun(t, fe.b, "t-src.execute", "alice", "t-src", "completed", "lane")
	seedDeliverable(t, fe.b, "d-1", "alice", "t-src", "", 1)

	if rr := doAs(t, fe.srv(t, "bob"), "POST",
		"/api/deliverables/d-1/follow-up", `{"revision":1}`); rr.Code != http.StatusForbidden {
		t.Errorf("another member spawning from alice's deliverable: status %d, want 403", rr.Code)
	}
	if rr := doAs(t, fe.srv(t, "bob"), "POST",
		"/api/deliverables/nope/follow-up", `{"revision":1}`); rr.Code != http.StatusNotFound {
		t.Errorf("an unknown deliverable: status %d, want 404 (404-before-403)", rr.Code)
	}
	// The operator may spawn from any deliverable, and the successor is the
	// OPERATOR's own task (15.6: whoever asks owns the work it creates).
	rr := doAs(t, fe.srv(t, "op"), "POST",
		"/api/deliverables/d-1/follow-up", `{"revision":1,"preset":"revision"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("operator spawn: status %d: %s", rr.Code, rr.Body)
	}
	var out api.FollowUpSpawned
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	if out.Owner != "op" {
		t.Errorf("the successor owner = %q, want the authenticated requester", out.Owner)
	}
	// Caller-input failures are 400 — validated at the boundary, before the
	// spawn runs (drain D7).
	if rr := doAs(t, fe.srv(t, "alice"), "POST",
		"/api/deliverables/d-1/follow-up", `{"revision":99}`); rr.Code != http.StatusBadRequest {
		t.Errorf("a link to a non-existent revision: status %d, want 400", rr.Code)
	}
	if rr := doAs(t, fe.srv(t, "alice"), "POST",
		"/api/deliverables/d-1/follow-up", `{"revision":1,"preset":"invented"}`); rr.Code != http.StatusBadRequest {
		t.Errorf("an unknown preset: status %d, want 400 (presets are the landed set)", rr.Code)
	}
	// ...and a caller-input refusal never reaches the spawn at all.
	if len(fe.started) != 1 {
		t.Errorf("a refused input still drove the Start seam (%d entries)", len(fe.started))
	}
	// This route touches neither accept nor preview (B6-3's).
	if n := countEvents(t, fe.b, "deliverable.accepted"); n != 0 {
		t.Errorf("the follow-up route wrote %d accept rows; accept is B6-3's", n)
	}
}

// readTaskDetail reads the B6-1 task detail as who.
func readTaskDetail(t *testing.T, fe *followUpEnv, who, taskID string) struct {
	Lineage struct {
		Succeeds []struct {
			TaskID        string `json:"task_id"`
			DeliverableID string `json:"deliverable_id"`
		} `json:"succeeds"`
		SucceededBy []struct {
			TaskID string `json:"task_id"`
		} `json:"succeeded_by"`
	} `json:"lineage"`
} {
	t.Helper()
	var out struct {
		Lineage struct {
			Succeeds []struct {
				TaskID        string `json:"task_id"`
				DeliverableID string `json:"deliverable_id"`
			} `json:"succeeds"`
			SucceededBy []struct {
				TaskID string `json:"task_id"`
			} `json:"succeeded_by"`
		} `json:"lineage"`
	}
	rr := doAs(t, fe.srv(t, who), "GET", "/api/tasks/"+taskID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("task detail %s: status %d: %s", taskID, rr.Code, rr.Body)
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode task detail: %v", err)
	}
	return out
}

// TestFollowUpInternalFailureIs500NotABadRequest (drain D7): the two failure
// classes are different answers. Telling a caller their input was bad when the
// platform's own machinery failed sends them to fix something that is not wrong.
func TestFollowUpInternalFailureIs500NotABadRequest(t *testing.T) {
	fe := newFollowUpEnv(t)
	seedTask(t, fe.b, "t-src", "alice", "Source", "done")
	seedRun(t, fe.b, "t-src.execute", "alice", "t-src", "completed", "lane")
	seedDeliverable(t, fe.b, "d-1", "alice", "t-src", "", 1)
	// The Start seam fails — a platform-side failure with perfectly valid input.
	fe.fu.Start = func(context.Context, intake.Request) error {
		return errors.New("intake pipeline unavailable")
	}
	rr := doAs(t, fe.srv(t, "alice"), "POST", "/api/deliverables/d-1/follow-up", `{"revision":1}`)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("an internal spawn failure: status %d, want 500 (drain D7): %s", rr.Code, rr.Body)
	}
	if strings.Contains(rr.Body.String(), "bad_request") {
		t.Errorf("an internal failure was reported as bad input: %s", rr.Body)
	}
}

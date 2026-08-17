package api_test

import (
	"encoding/json"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// tasktriage_test.go — P3-RW-17 R8: `floor_reasons` reaches a landed owner read.
//
// The S06 Stage-0 record ("stakes tier + floor reasons") rides every
// `intake.state` payload and no API read served it, so the numbers that DECIDED
// a task's tier were recorded and never shown — against 13.5 ("approvals explain
// themselves") and the operator's see-everything directive. The block is the
// record's own facts, lifted by key and redacted at this serving edge like every
// other payload-derived string on this response.

// seedTriageState plants one `intake.state` row on a run, the producer's shape.
func seedTriageState(t *testing.T, e *dlvEnv, runID, payload string) {
	t.Helper()
	if _, err := e.b.log.Append(e.ctx, eventlog.Append{
		RunID: runID, UserID: "alice", Type: "intake.state", SchemaVersion: 1,
		Payload: json.RawMessage(payload),
	}); err != nil {
		t.Fatalf("append intake.state: %v", err)
	}
}

func readTaskDetailBody(t *testing.T, e *dlvEnv, who, taskID string) api.TaskDetail {
	t.Helper()
	var detail api.TaskDetail
	if err := json.Unmarshal([]byte(e.mustDo(t, who, "GET", "/api/tasks/"+taskID, "")), &detail); err != nil {
		t.Fatalf("decode task detail: %v", err)
	}
	return detail
}

// TestTaskDetailServesTheStageZeroTriageRecord — R8. The tier the task runs at,
// what resolved its family, and the deterministic floors that were tripped, on
// the owner's own read of the task.
func TestTaskDetailServesTheStageZeroTriageRecord(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	seedTriageState(t, e, "r-a", `{"phase":"approval","task_id":"t-a","run_id":"r-a",`+
		`"family":"content","family_source":"classifier","tier":"high","floor_tier":"high",`+
		`"floor_reasons":[{"class":"outward_effect","source":"plan","detail":"step S-5 publishes to a live site"},`+
		`{"class":"spend","source":"request","detail":"the guess crosses the ask floor"}]}`)

	detail := readTaskDetailBody(t, e, "alice", "t-a")
	if detail.Triage == nil {
		t.Fatal("the Stage-0 triage record is recorded on every intake.state and served nowhere — R8 is exactly this absence")
	}
	tri := detail.Triage
	if tri.Family != "content" || tri.FamilySource != "classifier" {
		t.Errorf("the family and WHAT RESOLVED IT must both be served: %+v", tri)
	}
	if tri.Tier != "high" || tri.FloorTier != "high" {
		t.Errorf("the tier and its deterministic floor must be served: %+v", tri)
	}
	if len(tri.FloorReasons) != 2 {
		t.Fatalf("every recorded floor reason must be served, got %+v", tri.FloorReasons)
	}
	if tri.FloorReasons[0].Class != "outward_effect" || tri.FloorReasons[0].Source != "plan" ||
		tri.FloorReasons[0].Detail != "step S-5 publishes to a live site" {
		t.Errorf("a floor reason is class + source + detail, verbatim: %+v", tri.FloorReasons[0])
	}
	if tri.FloorReasons[1].Class != "spend" {
		t.Errorf("the second floor reason must survive too: %+v", tri.FloorReasons[1])
	}

	// The LATEST state wins, the same ordering authority LoadState and the 0022
	// pin edge use: triage facts move (a floor recheck at the spine adds one).
	seedTriageState(t, e, "r-a", `{"task_id":"t-a","family":"software","family_source":"registry","tier":"medium"}`)
	later := readTaskDetailBody(t, e, "alice", "t-a")
	if later.Triage == nil || later.Triage.Family != "software" || later.Triage.Tier != "medium" {
		t.Errorf("the latest intake.state is the record: %+v", later.Triage)
	}
	if len(later.Triage.FloorReasons) != 0 {
		t.Errorf("a state that tripped no floor serves none, got %+v", later.Triage.FloorReasons)
	}
}

// TestTaskTriageIsAbsentWhenNothingWasRecorded is the honest-absence control in
// both of its shapes: a task with no intake.state at all, and one whose state
// event carries no triage facts. Serving an empty block would claim a triage
// that never happened.
func TestTaskTriageIsAbsentWhenNothingWasRecorded(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-none", "r-none", "alice")
	if got := readTaskDetailBody(t, e, "alice", "t-none").Triage; got != nil {
		t.Errorf("a task with no intake.state has no triage record to serve, got %+v", got)
	}
	e.mkRun("t-bare", "r-bare", "alice")
	seedTriageState(t, e, "r-bare", `{"stage":"drafting","kind":"intake"}`)
	if got := readTaskDetailBody(t, e, "alice", "t-bare").Triage; got != nil {
		t.Errorf("a state event recording no triage facts serves no block, got %+v", got)
	}
}

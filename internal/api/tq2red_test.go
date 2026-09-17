package api_test

// tq2red_test.go — P3-TQ-2 grounding (committed RED, SKILL.md amendment A):
// the served join for the crash→fork narrative (TQ-F8's "one connective
// backend line", P3/gates/rework-sitting-gate.md Part C).
//
// Spec S15.5/S15.2 (task detail serves the run lineage), S02.5 step 2/3 (the
// successor's `parent_run_id` IS the supersession edge, migration 0002), S14.2
// family 1 (every FSM transition carries its cause — the `reason`). The stage
// list (`stage_progress`) is per-run rows; the SPA showed "two unconnected
// sequences" because nothing served connected them. `taskRuns` already scans
// `runs.parent_run_id` (it builds the successor map for `receipt_absent`) and
// never serves it; the run's own ending sentence lives in its latest
// `run.state_changed.reason` and is served nowhere.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
)

func TestTQ2TaskRunsServeLineageAndEnding(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "op", auth.RoleOperator)
	seedTask(t, b, "t-tq2", "op", "TQ2 crash→fork narration", "doing")

	// The witnessed lineage (t-3120e8e3d14591d3.execute → .execute.g1): a
	// crashed execute leg superseded by its recovery fork.
	exec(t, b, `INSERT INTO runs (run_id, user_id, task_id, state, lane, generation, created_ts, updated_ts)
	            VALUES (?,?,?,?,?,1,?,?)`,
		"t-tq2.execute", "op", "t-tq2", "crashed", "anthropic", nowTS(), nowTS())
	exec(t, b, `INSERT INTO runs (run_id, user_id, task_id, state, lane, generation, parent_run_id, created_ts, updated_ts)
	            VALUES (?,?,?,?,?,1,?,?,?)`,
		"t-tq2.execute.g1", "op", "t-tq2", "running", "anthropic", "t-tq2.execute", nowTS(), nowTS())

	const parentEnding = "Step S-4: the work session ran, but the platform's own record-keeping failed during it. Finished steps stay finished; this attempt stops here."
	const forkEnding = "Picking up at step S-4 after run t-tq2.execute stopped: steps S-1 to S-3 were finished by it and are kept; the working copy is reset to the state after S-3 and S-4 starts over."
	// Two earlier transitions on the parent, then its ending: `ending` must be
	// the LATEST transition's reason, not the first.
	exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
	            VALUES (?,?,?,?,1,?,?)`,
		"t-tq2.execute", 0, "op", "run.state_changed",
		`{"from":"claimed","to":"running","reason":"execute stage sessions (S05.3)","actor":"platform"}`, nowTS())
	exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
	            VALUES (?,?,?,?,1,?,?)`,
		"t-tq2.execute", 0, "op", "run.state_changed",
		`{"from":"running","to":"crashed","reason":"`+parentEnding+`","actor":"platform","detail":{"step":"S-4","failed":"platform","cause":"adapters: artifact snapshot (S02.4d): git add -A (exit 128)"}}`, nowTS())
	exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
	            VALUES (?,?,?,?,1,?,?)`,
		"t-tq2.execute.g1", 1, "op", "run.state_changed",
		`{"from":"claimed","to":"running","reason":"`+forkEnding+`","actor":"platform","detail":{"resume_step":"S-4","completed_steps":["S-1","S-2","S-3"],"parent_run_id":"t-tq2.execute"}}`, nowTS())

	body := getOK(t, b, "op", "/api/tasks/t-tq2")
	var detail struct {
		Runs []struct {
			RunID       string `json:"run_id"`
			State       string `json:"state"`
			ParentRunID string `json:"parent_run_id"`
			Ending      string `json:"ending"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatalf("task detail decode: %v: %.300s", err, body)
	}
	if len(detail.Runs) != 2 {
		t.Fatalf("seeded 2 runs, detail served %d: %.400s", len(detail.Runs), body)
	}
	byID := map[string]struct{ parent, ending string }{}
	for _, r := range detail.Runs {
		byID[r.RunID] = struct{ parent, ending string }{r.ParentRunID, r.Ending}
	}

	// The supersession edge is served on the successor, absent on the root.
	if got := byID["t-tq2.execute.g1"].parent; got != "t-tq2.execute" {
		t.Errorf("successor parent_run_id = %q, want t-tq2.execute (runs.parent_run_id, Spec S02.5 step 3)", got)
	}
	if got := byID["t-tq2.execute"].parent; got != "" {
		t.Errorf("root run parent_run_id = %q, want empty", got)
	}
	// Each run's ending is its latest transition's own words (S14.2: cause).
	if got := byID["t-tq2.execute"].ending; got != parentEnding {
		t.Errorf("crashed run ending = %q, want the crash transition's reason", got)
	}
	if got := byID["t-tq2.execute.g1"].ending; got != forkEnding {
		t.Errorf("successor ending = %q, want the resume transition's reason", got)
	}
	// The field is omitted, not served empty, where no transition exists —
	// and the raw crash cause (an internal error line) never rides `ending`.
	if strings.Contains(body, `"ending":""`) {
		t.Errorf("ending served as an empty string rather than omitted: %.400s", body)
	}
	if strings.Contains(byID["t-tq2.execute"].ending, "S02.4d") {
		t.Errorf("the internal cause line leaked into the served ending: %q", byID["t-tq2.execute"].ending)
	}
}

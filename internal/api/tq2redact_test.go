package api_test

// tq2redact_test.go — P3-TQ-2 §7 (executor-added): `ending` is lifted out of a
// run_events payload, so it redacts at the serving edge.
//
// The drain-D10 reading that put `stage_progress`, the lineage, the decisions
// and the triage through the codor-C2 primitive applies verbatim here:
// "enumerated by key" is not the same property as "cannot carry a secret".
// `ending` is a `run.state_changed.reason` — a string the platform writes today
// and an engine-influenced one tomorrow — so it takes the same edge as its
// neighbours. Honest text is byte-for-byte unchanged by the primitive, which is
// why the lineage test beside this one can still assert exact sentences.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
)

func TestTQ2EndingRedactsAtTheServingEdge(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "op", auth.RoleOperator)
	seedTask(t, b, "t-tq2r", "op", "TQ2 ending redaction", "doing")

	exec(t, b, `INSERT INTO runs (run_id, user_id, task_id, state, lane, generation, created_ts, updated_ts)
	            VALUES (?,?,?,?,?,1,?,?)`,
		"t-tq2r.execute", "op", "t-tq2r", "crashed", "anthropic", nowTS(), nowTS())
	// A key the engine echoed into the line that says how the run ended: the
	// incidental capture the second line of defense exists for (§7-C2).
	const leaked = "sk-ant-api03-AAAABBBBCCCCDDDDEEEE"
	exec(t, b, `INSERT INTO run_events (run_id, generation, user_id, type, schema_version, payload, ts)
	            VALUES (?,?,?,?,1,?,?)`,
		"t-tq2r.execute", 0, "op", "run.state_changed",
		`{"from":"running","to":"crashed","reason":"Step S-1: the work session stopped before the step was finished, reporting `+leaked+`.","actor":"platform"}`, nowTS())

	body := getOK(t, b, "op", "/api/tasks/t-tq2r")
	if strings.Contains(body, leaked) {
		t.Errorf("the run's ending served an unredacted key: %.400s", body)
	}
	var detail struct {
		Runs []struct {
			RunID  string `json:"run_id"`
			Ending string `json:"ending"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatalf("task detail decode: %v: %.300s", err, body)
	}
	if len(detail.Runs) != 1 {
		t.Fatalf("seeded 1 run, detail served %d", len(detail.Runs))
	}
	got := detail.Runs[0].Ending
	if !strings.Contains(got, "[REDACTED:anthropic_key]") {
		t.Errorf("ending = %q, want the secret replaced by its inert marker", got)
	}
	// The rest of the sentence survives: redaction replaces the secret, never
	// the ending the requester needs to read.
	if !strings.Contains(got, "Step S-1") {
		t.Errorf("ending = %q, want the plain words around the marker kept", got)
	}
}

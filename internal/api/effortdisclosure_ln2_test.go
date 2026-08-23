package api_test

// effortdisclosure_ln2_test.go — LN-2A/D10: a mode change is a DISCLOSED STATE
// (S10.6), so the card must show the mode the engine actually ran under. The
// routing decision is the usual witness; an adapter that stamps the effort on
// its own terminal event is the only one a run dispatched without a routing
// record has.

import (
	"encoding/json"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
)

func TestEffortModeFallsBackToTheEngineTerminalEvent(t *testing.T) {
	b := newBackend(t)
	seedUser(t, b, "alice", auth.RoleMember)

	// A run with a routing decision AND an engine.done: routing wins, because
	// it is the record of what was chosen.
	seedTask(t, b, "t-routed", "alice", "Routed", "doing")
	seedRun(t, b, "r-routed", "alice", "t-routed", "running", "anthropic")
	appendRun(t, b, "alice", "r-routed", "routing.decided",
		`{"cause":"selector-match","score":0.9,"model":"m","lane":"anthropic","plain_reason":"best fit","effort":"smart"}`)
	appendRun(t, b, "alice", "r-routed", "engine.done",
		`{"outcome":"completed","effort":"eco","paid_calls":1}`)

	// A run dispatched with NO routing record — the adapter's terminal event
	// is the only disclosure there is.
	seedTask(t, b, "t-engine", "alice", "Engine only", "doing")
	seedRun(t, b, "r-engine", "alice", "t-engine", "running", "zai")
	appendRun(t, b, "alice", "r-engine", "engine.done",
		`{"outcome":"completed","effort":"eco","paid_calls":1}`)

	// A run that disclosed nothing stays empty: absence is expressible, and
	// a plausible-looking default is exactly what must not appear.
	seedTask(t, b, "t-silent", "alice", "Silent", "doing")
	seedRun(t, b, "r-silent", "alice", "t-silent", "running", "zai")
	appendRun(t, b, "alice", "r-silent", "engine.done",
		`{"outcome":"completed","paid_calls":1}`)

	var list wireTaskList
	if err := json.Unmarshal([]byte(fixtureGet(t, b, "alice", "/api/tasks")), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := map[string]string{}
	for _, tk := range list.Tasks {
		if tk.LatestRun != nil {
			got[tk.TaskID] = tk.LatestRun.EffortMode
		}
	}
	for task, want := range map[string]string{
		"t-routed": "smart",
		"t-engine": "eco",
		"t-silent": "",
	} {
		if got[task] != want {
			t.Errorf("%s effort_mode = %q, want %q", task, got[task], want)
		}
	}
}

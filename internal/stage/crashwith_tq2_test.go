package stage

// crashwith_tq2_test.go — P3-TQ-2 §7 (executor-added): `crashWith` is a crash,
// so it owes the whole §56 posture.
//
// `crashWith` exists because the step-error site knows things the generic
// corpse cannot say: which step died, whether the WORK or the PLATFORM failed,
// and a sentence the requester can read (R8). None of that may cost the corpse
// its two §56 properties — THE RECORD OF THE ENDING OUTLIVES THE REQUEST (a
// dispatch leg most often dies because its caller did, and an uncorpsed run is
// unforkable), and a leg unwound by a HUMAN CANCEL still files nothing (drain
// D1). Both are asserted here against the same helper `crash` delegates to, so
// neither can be lost on one path while the other keeps it.
//
// $0: nothing here spawns a process or dials anything.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// crashNarration reads the run's crash transition as it was recorded: the
// requester-facing reason and the structured detail beside it.
func crashNarration(t *testing.T, e *cancelEnv, runID string) (reason string, detail map[string]any) {
	t.Helper()
	var payload string
	if err := e.db.QueryRowContext(context.Background(), `
		SELECT payload FROM run_events
		 WHERE run_id = ? AND type = ? AND json_extract(payload, '$.to') = 'crashed'
		 ORDER BY event_seq LIMIT 1`, runID, run.EventState).Scan(&payload); err != nil {
		t.Fatalf("crash event for %s: %v", runID, err)
	}
	var pay struct {
		Reason string         `json:"reason"`
		Detail map[string]any `json:"detail"`
	}
	if err := json.Unmarshal([]byte(payload), &pay); err != nil {
		t.Fatalf("decode crash payload: %v", err)
	}
	return pay.Reason, pay.Detail
}

func TestTQ2CrashWithKeepsTheDetachedCrashPosture(t *testing.T) {
	const owner = "u-tq2-crashwith"
	const plain = "Step S-2: the work session stopped before the step was finished. Finished steps stay finished; this attempt stops here."

	t.Run("the narrated corpse outlives its request", func(t *testing.T) {
		e := newCancelEnv(t)
		const runID, taskID = "t-tq2-crashwith.execute", "t-tq2-crashwith"
		e.seedRun(t, runID, taskID, owner, run.StateRunning)

		e.sk.crashWith(deadContext(), runID, plain, stepCrashDetail("engine died mid-step", "S-2", failedWork, plain))

		if got := e.state(t, runID); got != run.StateCrashed {
			t.Fatalf("run is %s after crashWith on a dead context, want crashed (CONVENTIONS §56)", got)
		}
		reason, detail := crashNarration(t, e, runID)
		if reason != plain {
			t.Errorf("crash reason = %q, want the plain sentence the requester reads", reason)
		}
		for key, want := range map[string]string{"step": "S-2", "failed": failedWork, "plain": plain, "cause": "engine died mid-step"} {
			if got, _ := detail[key].(string); got != want {
				t.Errorf("crash detail %s = %q, want %q", key, got, want)
			}
		}
	})

	t.Run("a human cancel still suppresses it", func(t *testing.T) {
		e := newCancelEnv(t)
		const runID, taskID = "t-tq2-crashwith-cancel.execute", "t-tq2-crashwith-cancel"
		r := e.seedRun(t, runID, taskID, owner, run.StateRunning)

		e.sk.cancels.markRequested(runID, r.Generation)
		e.sk.crashWith(deadContext(), runID, plain, stepCrashDetail("unwound by the cancel", "S-2", failedWork, plain))

		if got := e.state(t, runID); got != run.StateRunning {
			t.Fatalf("run is %s, want still running — a narrated crash must consult the cancel suppression too (drain D1)", got)
		}
		if e.sk.cancels.consumeCancel(runID, r.Generation) {
			t.Fatal("the suppression mark was not consumed — it would swallow a later, genuine crash")
		}
	})
}

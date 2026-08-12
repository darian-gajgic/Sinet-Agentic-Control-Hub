package stage_test

// strand_e2e_test.go — P3-RW-9 T5–T7: an intake beat that dies mid-drive
// leaves a classifiable corpse, and one that never got that far leaves the
// gate exactly as it found it.
//
// The defect these tests pin (live, task `t-59f2a72bab7f8109`, 2026-08-12):
// the human answered an interview card, `closeAndResume` committed
// parked→running, the plan-draft sessions came back with unusable text, the
// error travelled up through `Surface.Answer` to a bare 500 — and NOBODY
// transitioned the run. It sat `running`, driver-less, corpse-less: the
// recovery ladder never scans a run whose lease is still being renewed by
// nothing, the watchdog silence-parked it 301s later, and a parked ask-less
// run's only release is another human resume. The strand was unbounded.
//
// The doctrine already existed on both neighbours — the dispatch leg
// (`skeleton.go` Dispatch/dispatchIntake) and the S07.7 verify answer beat
// (`answer.go`, CONVENTIONS §16: "a resumed drain that errors mid-flight
// crashes the run for the recovery ladder"). These tests extend it to the
// intake answer/advance beats, and pin the cut line: only a run the beat
// already took to `running` gets the corpse (R4); everything refused before
// the resume commit stays parked on its open ask (R5).
//
// Zero paid calls: the planning seam is the fork harness's recorder.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// strandCodeRecovering is the distinct surface code a crashed advance
// answers with: still 500 (the request truly failed) but classifiable, so the
// SPA can say "recovering" instead of a bare internal error (R6).
const strandCodeRecovering = "advance_crashed_recovering"

// walkToOpenCard submits a request and dispatches it to its first interview
// card, returning the task, its intake run, the open ask and a well-formed
// answer body for that card.
func walkToOpenCard(t *testing.T, h *forkHarness, owner string) (taskID, runID, askID string, body json.RawMessage) {
	t.Helper()
	ctx := context.Background()
	raw, err := h.sur.Submit(ctx, owner, json.RawMessage(
		`{"title":"SQLite note","text":"Write a short appreciation note about the SQLite database engine."}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	taskID = decodeView(t, raw).TaskID
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the intake run", n)
	}
	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	v := decodeView(t, raw)
	if v.OpenCard.Kind != "interview" || len(v.OpenCard.Questions) == 0 {
		t.Fatalf("expected an open interview card, got %s", raw)
	}
	body, err = json.Marshal(map[string]any{
		"answers":       []map[string]string{{"id": v.OpenCard.Questions[0].ID, "value": "the-answer-from-card-one"}},
		"force_proceed": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return taskID, taskID + ".intake", v.OpenAskID, body
}

// crashCause reads the cause recorded on the run's crash transition.
func crashCause(t *testing.T, h *forkHarness, runID string) string {
	t.Helper()
	var cause string
	err := h.db.QueryRowContext(context.Background(), `
		SELECT COALESCE(json_extract(payload, '$.detail.cause'), '')
		  FROM run_events
		 WHERE run_id = ? AND type = ? AND json_extract(payload, '$.to') = 'crashed'
		 ORDER BY event_seq LIMIT 1`, runID, run.EventState).Scan(&cause)
	if err != nil {
		t.Fatalf("crash cause for %s: %v", runID, err)
	}
	return cause
}

func surfaceError(t *testing.T, err error) *api.SurfaceError {
	t.Helper()
	var se *api.SurfaceError
	if !errors.As(err, &se) {
		t.Fatalf("error %v is not a *api.SurfaceError", err)
	}
	return se
}

// ---- T5: the strand, healed end to end ----

func TestIntakeAnswerStrandCrashesAndHealsWithoutAHuman(t *testing.T) {
	h := newForkHarness(t)
	ctx := context.Background()
	const owner = "u-strand"

	taskID, intakeRun, askID, body := walkToOpenCard(t, h, owner)

	// The answer commits (ask closed, parked→running) and THEN the planning
	// seam dies twice over `jsonWithRetry`'s bounce-once — the live shape.
	h.planner.failFirst = 2
	_, err := h.sur.Answer(ctx, owner, askID, body, false)
	if err == nil {
		t.Fatal("the answer's advance must still fail loudly — the request really did fail (R6)")
	}
	se := surfaceError(t, err)
	if se.Status != http.StatusInternalServerError || se.Code != strandCodeRecovering {
		t.Fatalf("answer error = %d/%s, want 500/%s (R6: honest AND classifiable)", se.Status, se.Code, strandCodeRecovering)
	}

	// R4: a corpse, not a driver-less `running` run.
	if got := h.runState(intakeRun); got != run.StateCrashed {
		t.Fatalf("run %s is %s after the dead advance, want crashed (R4: no stranded run)", intakeRun, got)
	}
	if n := h.crashEvents(intakeRun); n != 1 {
		t.Fatalf("%d crash events on %s, want exactly 1", n, intakeRun)
	}
	if cause := crashCause(t, h, intakeRun); cause == "" {
		t.Fatal("the crash carries no cause — an unclassifiable corpse (§8: transitions carry reason+actor+detail)")
	}
	if got := h.openAskID(taskID); got != "" {
		t.Fatalf("ask %q is still open — the answered card must stay closed", got)
	}

	// R7: the heal is machine-only and bounded — ONE ladder pass forks the
	// corpse, the fork dispatches, rebinds and re-parks on its own card. No
	// watchdog-silence window, no human resume anywhere in this test.
	forkID := h.forkOf(intakeRun)
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the fork", n)
	}
	if got := h.crashEvents(forkID); got != 0 {
		t.Fatalf("the fork crashed %d times — it must take over, not re-die", got)
	}
	if got := h.runState(forkID); got != run.StateParked {
		t.Fatalf("fork %s is %s, want parked on a fresh gate", forkID, got)
	}
	freshAsk := h.openAskID(taskID)
	if freshAsk == "" {
		t.Fatal("the healed fork left no open card — the task would sit attention-less")
	}
	if got := h.askRunID(freshAsk); got != forkID {
		t.Fatalf("fresh ask run_id = %q, want the fork %q", got, forkID)
	}
}

// ---- T6: pre-resume negatives (R5) ----

func TestIntakeAnswerRefusalsLeaveTheGateStanding(t *testing.T) {
	h := newForkHarness(t)
	ctx := context.Background()
	const owner = "u-negatives"

	taskID, intakeRun, askID, body := walkToOpenCard(t, h, owner)

	badSlot, err := json.Marshal(map[string]any{
		"answers": []map[string]string{{"id": "slot-that-was-never-asked", "value": "x"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		actor  string
		ask    string
		body   json.RawMessage
		status int
		code   string
	}{
		{"unknown ask", owner, "intake:" + taskID + ":99", body, http.StatusNotFound, "not_found"},
		{"not the requester", "u-somebody-else", askID, body, http.StatusForbidden, "not_requester"},
		{"bad answer", owner, askID, badSlot, http.StatusBadRequest, "bad_answer"},
		{"undecodable answer", owner, askID, json.RawMessage(`{"answers":"not-a-list"}`), http.StatusBadRequest, "bad_answer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.sur.Answer(ctx, tc.actor, tc.ask, tc.body, false)
			if err == nil {
				t.Fatal("refusal expected")
			}
			se := surfaceError(t, err)
			if se.Status != tc.status || se.Code != tc.code {
				t.Fatalf("error = %d/%s, want %d/%s", se.Status, se.Code, tc.status, tc.code)
			}
			if got := h.runState(intakeRun); got != run.StateParked {
				t.Fatalf("run %s is %s, want still parked (R5: gates wait)", intakeRun, got)
			}
			if got := h.openAskID(taskID); got != askID {
				t.Fatalf("open ask = %q, want the untouched card %q", got, askID)
			}
			if n := h.crashEvents(intakeRun); n != 0 {
				t.Fatalf("%d crash events — a refused answer must mint no corpse (R5)", n)
			}
		})
	}
}

// ---- T7: Advance nudge parity ----

func TestAdvanceCrashesMidDriveAndConflictsWhenNotRunning(t *testing.T) {
	h := newForkHarness(t)
	ctx := context.Background()
	const owner = "u-advance"

	taskID, intakeRun, askID, body := walkToOpenCard(t, h, owner)

	// The window: answer through the PIPELINE (which crashes nothing — the
	// corpse is the stage layer's act, CONVENTIONS §14/§16 layering), so the
	// run is left running with no open gate and nobody driving it.
	h.planner.failFirst = 2
	if _, err := h.sk.Pipeline().Answer(ctx, owner, askID, body); err == nil {
		t.Fatal("expected the pipeline answer to fail with the dying planning seam")
	}
	if got := h.runState(intakeRun); got != run.StateRunning {
		t.Fatalf("window: run %s is %s, want running (the pipeline never transitions runs)", intakeRun, got)
	}

	// (a) the nudge re-drives, dies mid-flight, and leaves the SAME corpse the
	// answer beat leaves.
	h.planner.failFirst = 2
	_, err := h.sur.Advance(ctx, owner, taskID)
	if err == nil {
		t.Fatal("expected the advance to fail with the dying planning seam")
	}
	se := surfaceError(t, err)
	if se.Status != http.StatusInternalServerError || se.Code != strandCodeRecovering {
		t.Fatalf("advance error = %d/%s, want 500/%s", se.Status, se.Code, strandCodeRecovering)
	}
	if got := h.runState(intakeRun); got != run.StateCrashed {
		t.Fatalf("run %s is %s after the dead advance, want crashed", intakeRun, got)
	}
	if n := h.crashEvents(intakeRun); n != 1 {
		t.Fatalf("%d crash events, want exactly 1", n)
	}

	// (b) a run that is NOT running keeps today's conflict mapping and mints no
	// second corpse (R5's cut line is the state, not the caller).
	_, err = h.sur.Advance(ctx, owner, taskID)
	if err == nil {
		t.Fatal("advancing a crashed run must refuse")
	}
	se = surfaceError(t, err)
	if se.Status != http.StatusConflict || se.Code != "conflict" {
		t.Fatalf("advance-on-crashed error = %d/%s, want 409/conflict (untouched mapping)", se.Status, se.Code)
	}
	if n := h.crashEvents(intakeRun); n != 1 {
		t.Fatalf("%d crash events after the refused nudge, want the original 1", n)
	}
}

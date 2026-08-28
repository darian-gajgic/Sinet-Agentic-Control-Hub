package stage_test

// lanepin_ln9r1_test.go — P3-LN-9 drain r1 F3 + F1/F2's surface half
// (S08.8, S00.9 A13, §30).
//
// Two guards nothing was holding.
//
// F3: the spawn-site disagreement refusal (helpers.go) had NO test — mutating
// its condition to `if false` left the whole stage package green. A guard whose
// deletion its own package cannot see is a guard nobody is holding (§64), and
// this one is load-bearing: it is what stops a pinned coordinator from spawning
// helpers on another lane, which would silently un-split the comparison the pin
// was declared for.
//
// F1/F2: SELECTION's own refusal (worker.ErrLanePinUnhonorable) reached the
// transport UNMAPPED, so a bad request answered as a platform defect (§30).
//
// The world both use is a deliberate MISCONFIGURATION — a lane that is covered
// and seated by nothing. Not reachable with the three SHIPPED lane documents
// (each declares a default_model), but production CAN produce it: coverage and
// seats derive from the same placed-credential set through DIFFERENT
// predicates — CommissionedLanes has no model condition while CommissionedSeats
// skips any lane whose DefaultModel is empty, and a lane document omitting
// default_model loads clean — so a config-only lane addition (the S03.6
// rider-3 mechanism) that omits default_model lands exactly here. That is why
// this guard exists and why it is tested from the misconfigured shape.
//
// $0: recording adapters that start no engine; no provider on any path.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// ln9r1Coordinator seeds a task, its recorded intake state carrying the pin,
// and a RUNNING coordinator run — without walking the ceremony, because in this
// misconfigured world the walk itself cannot reach approval (selection refuses
// the pin at the advance, which is its own correct behaviour).
func ln9r1Coordinator(t *testing.T, h *ln9Harness, taskID, owner, pin string) run.Run {
	t.Helper()
	ctx := context.Background()
	if err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES (?, ?, 'ln9 r1 harness', ?)`,
			taskID, owner, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	runID := taskID + ".execute"
	if _, err := h.runs.Create(ctx, run.NewRun{
		ID: runID, UserID: owner, TaskID: taskID,
		Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic,
	}); err != nil {
		t.Fatalf("create coordinator run: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := h.runs.Transition(ctx, runID, st, run.TransitionOptions{
			Reason: "test admission", Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("admit: %v", err)
		}
	}
	req := map[string]any{"user_id": owner, "title": "compare the lanes", "text": "run the comparison task"}
	if pin != "" {
		req["pinned_lane"] = pin
	}
	// The state's own tag is "request" (intake/state.go), not "req": a
	// mis-tagged seed unmarshals to a ZERO Request, so the pin would be
	// absent and every guard below would pass vacuously.
	payload, err := json.Marshal(map[string]any{"request": req})
	if err != nil {
		t.Fatal(err)
	}
	r, err := h.runs.Get(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.log.Append(ctx, eventlog.Append{
		RunID: r.ID, Generation: r.Generation, UserID: owner,
		Type: intake.EventState, SchemaVersion: 1, Payload: payload,
	}); err != nil {
		t.Fatalf("seed pipeline state: %v", err)
	}
	return r
}

func ln9r1Spawn(t *testing.T, h *ln9Harness, runID, reason string) error {
	t.Helper()
	_, err := h.sk.Spawn(context.Background(), stage.SpawnRequest{
		RunID:   runID,
		Trigger: stage.TriggerParallel,
		Reason:  reason,
		Brief: stage.HelperBrief{
			Objective:      "Trawl the notes archive for entries about SQLite.",
			OutputContract: "FINDINGS / EVIDENCE / GAPS",
			ToolsSources:   "Read only; start in notes/",
			Boundaries:     "read-only; do NOT edit anything",
			Context:        "The task is writing an appreciation note.",
			Class:          "C1",
			Tools:          []string{"Read"},
		},
	})
	return err
}

// ── F3 · the spawn-site disagreement refusal, actually tripped ────────────

func TestLN9R1HelperSpawnRefusesWhenTheLaneDisagreesWithThePin(t *testing.T) {
	// Covered, and seated by nothing: selection cannot honor the pin and the
	// degraded fallback cannot either, so the spawn site is handed a decision
	// on a lane the task did not ask for. That is the one thing it exists for.
	h := newLN9HarnessWith(t, nil)
	const owner = "u-ln9"
	insertUser(t, h.db, owner, "member")

	r := ln9r1Coordinator(t, h, "t-ln9r1-pinned", owner, adapters.LaneZAI)
	err := ln9r1Spawn(t, h, r.ID, "a lane-pin disagreement guard, not a real search")
	if err == nil {
		t.Fatalf("the spawn was ADMITTED while the task pinned %q and nothing seats it — a helper on another "+
			"lane un-splits the comparison the pin was declared for (S08.8 step 5 [S00.9 A13])", adapters.LaneZAI)
	}
	if !strings.Contains(err.Error(), "lane-pin") {
		t.Fatalf("the spawn failed for some OTHER reason than the lane-pin guard: %v", err)
	}
	// RECORDED, not merely returned: a refusal nobody can see afterwards is
	// not a refusal.
	refusals := eventsOfType(t, h.db, r.ID, stage.EventSpawnRefused)
	if len(refusals) != 1 {
		t.Fatalf("spawn.refused events = %d, want 1", len(refusals))
	}
	if refusals[0]["check"] != "lane-pin" {
		t.Errorf("the recorded refusal check = %v, want lane-pin", refusals[0]["check"])
	}
	detail, _ := refusals[0]["detail"].(string)
	for _, want := range []string{adapters.LaneZAI, "pins lane", "refused"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the recorded refusal detail does not say %q: %q", want, detail)
		}
	}

	// No engine was reached: the refusal lands BEFORE dispatch, which is the
	// whole point of refusing rather than degrading.
	h.mu.Lock()
	seen := append([]string(nil), h.seen...)
	h.mu.Unlock()
	if len(seen) != 0 {
		t.Errorf("a refused spawn still reached the %v adapter(s)", seen)
	}

	// The CONTROL, in the SAME misconfigured world: an unpinned task spawns
	// normally. Without it this guard is satisfied by a site that refuses
	// everything, which would be a worse platform than the one before it.
	c := ln9r1Coordinator(t, h, "t-ln9r1-unpinned", owner, "")
	if err := ln9r1Spawn(t, h, c.ID, "the unpinned control"); err != nil &&
		strings.Contains(err.Error(), "lane-pin") {
		t.Errorf("an UNPINNED task was refused by the lane-pin guard: %v", err)
	}
	for _, ev := range eventsOfType(t, h.db, c.ID, stage.EventSpawnRefused) {
		if ev["check"] == "lane-pin" {
			t.Errorf("the unpinned control recorded a lane-pin refusal: %v", ev)
		}
	}
}

// ── F1/F2 · selection's refusal is loud, attributable and mapped ──────────

// TestLN9R1SelectionRefusalIsLoudAndAttributable drives the production intake
// path in the misconfigured world and pins what an operator actually gets: the
// refusal is RAISED, it carries selection's own sentinel so a caller can tell
// it from a broken router, and it NAMES the pin. No task proceeds to execution
// on a lane nothing can run.
func TestLN9R1SelectionRefusalIsLoudAndAttributable(t *testing.T) {
	h := newLN9HarnessWith(t, nil)
	ctx := context.Background()
	const owner = "u-ln9"
	insertUser(t, h.db, owner, "member")

	// The boundary ADMITS this pin — the lane is commissioned and covered —
	// and selection refuses it when the ceremony reaches routing.
	raw, err := h.sur.Submit(ctx, owner,
		json.RawMessage(`{"title":"SQLite note","text":"Write a short appreciation note about the SQLite database engine.","pinned_lane":"`+adapters.LaneZAI+`"}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	v := decodeView(t, raw)
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d", n)
	}
	raw, err = h.sur.Task(ctx, v.TaskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	raw = clearFamilyGate(t, ctx, h.sur, owner, raw)
	view := decodeView(t, raw)
	_, err = h.sur.Answer(ctx, owner, view.OpenAskID, json.RawMessage(`{"force_proceed":true}`), false)
	if err == nil {
		t.Fatal("the ceremony reached approval on a lane nothing seats — selection cannot honor the pin, so a " +
			"card offering it would promise a run that cannot happen")
	}
	// The refusal REACHES the requester with its whole reason intact: the lane
	// it refused and the true cause, so the operator can act on it.
	if !strings.Contains(err.Error(), adapters.LaneZAI) {
		t.Errorf("the refusal does not name the pin: %v", err)
	}
	if !strings.Contains(err.Error(), "no model on that lane has been set up here") {
		t.Errorf("the refusal does not name the true cause: %v", err)
	}
	if !strings.Contains(err.Error(), "lane pin cannot be honored") {
		t.Errorf("the refusal does not identify itself as a lane-pin refusal: %v", err)
	}
	// RECORDED, and stated rather than wished away: on the ADVANCE path the
	// error is re-minted by the S02.3 corpse posture (P3-RW-9 R6), which
	// replaces the cause for every drive that died past the resume commit —
	// so `errors.Is` does NOT see selection's sentinel here, and the status is
	// the corpse's rather than a 4xx. That posture is ratified and predates
	// this packet; the lane pin does not get an exception to it. What the
	// mapper fix guarantees is the OTHER paths, pinned in the in-package
	// mapper test beside this file.
	if errors.Is(err, worker.ErrLanePinUnhonorable) {
		t.Log("note: the advance path now preserves the sentinel — the corpse posture may have changed; " +
			"if so, the mapped-4xx claim can be widened to this path too")
	}
}

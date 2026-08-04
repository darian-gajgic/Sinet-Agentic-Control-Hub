package stage_test

// stageevents_e2e_test.go — the S14.2 family-1 stage boundary observed through
// the REAL spine with the fake engine (zero paid calls, B6-1 R14/R16): every
// fresh engine stage session across every role announces itself and records how
// it ended, the pairs are complete, and a stage split is recorded as a SPLIT
// rather than as a completion or an error.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

type stageRecord struct {
	runID   string
	typ     string
	gen     int64
	seq     int64
	stage   string
	kind    string
	hash    string
	outcome string
}

// stageRecords reads every stage boundary row in the database, in event order.
func stageRecords(t *testing.T, db *storage.DB) []stageRecord {
	t.Helper()
	rows, err := db.QueryContext(context.Background(),
		`SELECT event_seq, COALESCE(run_id, ''), generation, type, payload FROM run_events
		  WHERE type IN (?, ?) ORDER BY event_seq`,
		stage.EventStageStarted, stage.EventStageFinished)
	if err != nil {
		t.Fatalf("query stage rows: %v", err)
	}
	defer rows.Close()
	var out []stageRecord
	for rows.Next() {
		var r stageRecord
		var payload string
		if err := rows.Scan(&r.seq, &r.runID, &r.gen, &r.typ, &payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		r.stage, _ = m["stage"].(string)
		r.kind, _ = m["kind"].(string)
		r.hash, _ = m["brief_hash"].(string)
		r.outcome, _ = m["outcome"].(string)
		out = append(out, r)
	}
	return out
}

// TestStageEventsMintAtEveryStageSession drives the walking skeleton end to end
// and asserts the boundary record: every session opened is a session closed,
// each pair shares one non-empty brief hash, the contract-minimum fields are
// present, outcomes come from the closed vocabulary, and the rows are
// run-scoped at the run's own generation. Roles are covered by construction —
// the walk runs ceremony, execute and verify sessions.
func TestStageEventsMintAtEveryStageSession(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	const owner = "u-stage-events"

	raw, err := h.sur.Submit(ctx, owner, json.RawMessage(
		`{"title":"SQLite note","text":"Write a short appreciation note about the SQLite database engine."}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	taskID := decodeView(t, raw).TaskID
	h.tick(ctx)
	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	v := decodeView(t, raw)
	if v.OpenAskID == "" {
		t.Fatalf("no open card after intake: %s", raw)
	}
	raw, err = h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"force_proceed":true}`), false)
	if err != nil {
		t.Fatalf("Answer(force_proceed): %v", err)
	}
	v = decodeView(t, raw)
	if v.OpenCard.Kind != "approval" {
		t.Fatalf("expected the approval card, got %s", raw)
	}
	if _, err := h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"action":"approve"}`), true); err != nil {
		t.Fatalf("Answer(approve): %v", err)
	}
	h.tick(ctx) // execute
	h.tick(ctx) // verify

	recs := stageRecords(t, h.db)
	if len(recs) < 4 {
		t.Fatalf("stage boundary rows = %d, want at least two sessions' worth: %+v", len(recs), recs)
	}

	type key struct{ runID, stage, hash string }
	open := map[key]int{}
	var finished int
	kinds := map[string]bool{}
	for _, r := range recs {
		if r.runID == "" {
			t.Errorf("seq %d: a stage row must be run-scoped", r.seq)
		}
		if r.stage == "" || r.kind == "" || r.hash == "" {
			t.Errorf("seq %d: contract minimum missing (stage=%q kind=%q brief_hash=%q)", r.seq, r.stage, r.kind, r.hash)
		}
		kinds[r.kind] = true
		k := key{r.runID, r.stage, r.hash}
		switch r.typ {
		case stage.EventStageStarted:
			if r.outcome != "" {
				t.Errorf("seq %d: stage.started must carry no outcome, got %q", r.seq, r.outcome)
			}
			open[k]++
		case stage.EventStageFinished:
			switch r.outcome {
			case stage.StageOutcomeCompleted, stage.StageOutcomeSplit, stage.StageOutcomeError:
			default:
				t.Errorf("seq %d: outcome %q is outside the closed vocabulary", r.seq, r.outcome)
			}
			if open[k] == 0 {
				t.Errorf("seq %d: stage.finished with no matching started (run=%s stage=%s)", r.seq, r.runID, r.stage)
			}
			open[k]--
			finished++
		}
	}
	for k, n := range open {
		if n != 0 {
			t.Errorf("%d unfinished stage session(s) for %+v — a stage that began and never ended in the log", n, k)
		}
	}
	if finished < 2 {
		t.Fatalf("finished rows = %d, want the ceremony and execute sessions at least", finished)
	}
	if len(kinds) < 2 {
		t.Fatalf("stage kinds seen = %v, want several roles across the walk", kinds)
	}
	if kinds["unmarked"] {
		t.Errorf("a stage session reached the engine with no stage marker: %v", kinds)
	}

	// Every row was appended at the run's generation AT THE TIME — the B5-1
	// envelope fences stale AND ahead, so the append succeeding is the proof.
	// What is checkable afterwards is that no row is ahead of where the run
	// has since reached (a later park/resume legitimately moves it forward).
	for _, r := range recs {
		var gen int64
		if err := h.db.QueryRowContext(ctx, `SELECT generation FROM runs WHERE run_id = ?`, r.runID).Scan(&gen); err != nil {
			t.Fatalf("read run generation: %v", err)
		}
		if r.gen > gen {
			t.Errorf("seq %d: row generation %d is ahead of the run's %d", r.seq, r.gen, gen)
		}
	}
}

// TestStageSplitIsRecordedAsASplit pins the middle member of the outcome
// vocabulary through the real overflow loop: the interrupted leg is neither a
// completion nor an error, and its successor sub-stage opens its own session.
func TestStageSplitIsRecordedAsASplit(t *testing.T) {
	h := newSplitHarness(t)
	ctx := context.Background()
	const owner = "u-split-events"

	raw, err := h.sur.Submit(ctx, owner, json.RawMessage(
		`{"title":"SQLite note","text":"Write a short appreciation note about the SQLite database engine."}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	taskID := decodeView(t, raw).TaskID
	h.tick(ctx)
	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	v := decodeView(t, raw)
	raw, err = h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"force_proceed":true}`), false)
	if err != nil {
		t.Fatalf("Answer(force_proceed): %v", err)
	}
	v = decodeView(t, raw)
	if _, err := h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"action":"approve"}`), true); err != nil {
		t.Fatalf("Answer(approve): %v", err)
	}
	h.tick(ctx) // execute — the first S-1 session overflows and splits
	h.tick(ctx) // verify

	execRun := taskID + ".execute"
	var splits, successors int
	for _, r := range stageRecords(t, h.db) {
		if r.runID != execRun || r.typ != stage.EventStageFinished {
			continue
		}
		if r.outcome == stage.StageOutcomeSplit {
			splits++
			if r.stage != "S-1" {
				t.Errorf("the split leg names stage %q, want the PLANNED stage S-1", r.stage)
			}
		}
		if r.stage == "S-1#2" {
			successors++
		}
	}
	if splits != 1 {
		t.Fatalf("split outcomes = %d, want exactly 1 (the interrupted S-1 leg)", splits)
	}
	if successors != 1 {
		t.Fatalf("successor sub-stage sessions recorded = %d, want 1 (S-1#2 opens its own boundary)", successors)
	}
}

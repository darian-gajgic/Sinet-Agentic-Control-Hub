package stage_test

// resulttext_e2e_test.go — P3-RW-9 T8: the adapter-seam fallback reaches the
// consumers with ZERO consumer changes.
//
// The stage runner hands `Outcome.ResultText` on as `SessionResult.Text`
// (runner.go), which is the parse input of every JSON-contract duty
// (Spec S06.10 planner/critic, S07.5 judge) and the deliverable of record of
// the execute leg. This test drives a REAL claudecli adapter over a REAL
// subprocess whose stream carries the recorded flake shape — a clean success
// envelope with an EMPTY `result` field and a full final assistant message —
// so the assertion covers spawn → parse → assemble → runner, not a stub that
// hands the answer to itself.
//
// Provenance of the shape: task `t-59f2a72bab7f8109`, intake generation 4,
// read 2026-08-12 from a COPY of the B6-clickthrough world (see the tier-F
// fixtures in internal/adapters/claudecli/testdata). The text here is short
// but keeps the observed property: it is one closing brace short, and the
// adapter must hand it through unrepaired.
//
// Zero paid calls: the engine is a shell script replaying JSONL.

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// emptyResultEngine writes a fake engine binary that replays one stream: init,
// one assistant message carrying finalText, then a CLEAN success envelope
// whose `result` field is empty.
func emptyResultEngine(t *testing.T, finalText string) string {
	t.Helper()
	dir := t.TempDir()
	const sid = "9edcfad3-0000-4000-8000-000000000000"
	usage := map[string]any{
		"input_tokens": 14210, "output_tokens": 1394,
		"cache_read_input_tokens": 12800, "cache_creation_input_tokens": 1410,
	}
	var stream []byte
	for _, line := range []any{
		map[string]any{"type": "system", "subtype": "init", "cwd": "/tmp/fake-cwd",
			"session_id": sid, "model": "claude-sonnet-4-5", "permissionMode": "default", "tools": []string{}},
		map[string]any{"type": "assistant", "message": map[string]any{
			"id": "msg_01EmptyResultRepro", "model": "claude-sonnet-4-5",
			"content": []map[string]any{{"type": "text", "text": finalText}},
			"usage":   usage}},
		map[string]any{"type": "stream_event", "session_id": sid, "event": map[string]any{"type": "message_stop"}},
		map[string]any{"type": "result", "subtype": "success", "is_error": false,
			"result": "", "session_id": sid, "stop_reason": "end_turn",
			"terminal_reason": "completed", "num_turns": 1, "total_cost_usd": 0.0421, "usage": usage},
	} {
		raw, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		stream = append(append(stream, raw...), '\n')
	}
	streamPath := filepath.Join(dir, "stream.jsonl")
	if err := os.WriteFile(streamPath, stream, 0o600); err != nil {
		t.Fatal(err)
	}
	enginePath := filepath.Join(dir, "fake-engine")
	script := "#!/bin/sh\ncat >/dev/null 2>/dev/null\nexec cat " + streamPath + "\n"
	if err := os.WriteFile(enginePath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return enginePath
}

// emptyResultSkeleton builds the minimum real spine plus a run walked to `running`
// through the ratified edges.
func emptyResultSkeleton(t *testing.T, engine string) (*stage.Skeleton, string) {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	runs := run.NewStore(db, log)
	root := t.TempDir()
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: gates.NewCheckpoints(db, log),
		Ledger: ledger.NewStore(db, log), Settings: reg,
		Adapters: map[string]adapters.Adapter{
			adapters.SubstrateClaudeCLI: &claudecli.Adapter{
				Binary:      engine,
				HookCmd:     "/opt/sinet/bin/sinet engine-hook",
				Settings:    reg,
				CancelGrace: 500 * time.Millisecond,
			},
		},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
		CopyAsideDir: filepath.Join(root, "copy-aside"),
		Model:        "fake-model-1",
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	const owner, taskID, runID = "u-rw9", "t-rw9", "t-rw9.execute"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO users (user_id, display_name, role, created_ts) VALUES (?,?,?,?)`,
			owner, owner, "member", now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO tasks (task_id, user_id, title, created_ts) VALUES (?,?,?,?)`,
			taskID, owner, "T", now)
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := runs.Create(ctx, run.NewRun{
		ID: runID, UserID: owner, TaskID: taskID,
		Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, to := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, runID, to, run.TransitionOptions{
			Reason: "fixture", Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
	}
	return sk, runID
}

func TestSessionTextSurvivesAnEmptyResultField(t *testing.T) {
	// One closing brace short, exactly as the reproduction's transcripts end.
	const finalText = `{"spec":{"provenance":"planning-model draft v1","restatement":"a short note about SQLite"},` +
		`"plan":{"provenance":"planning-model draft v1","steps":[{"id":"S-1","title":"Write the note"}]}`

	sk, runID := emptyResultSkeleton(t, emptyResultEngine(t, finalText))
	res, err := sk.Session(context.Background(), stage.SessionInput{
		RunID:        runID,
		Stage:        "plan",
		Instructions: "SINET-STAGE: plan-draft\nReply with the pair object.",
	})
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if res.Outcome.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome = %q, want completed", res.Outcome.Kind)
	}
	if res.Text != finalText {
		t.Fatalf("SessionResult.Text = %q, want the stream's final assistant text %q — the consumers inherit the seam's fallback with zero changes (R8)",
			res.Text, finalText)
	}
	// Unrepaired: the brace-short text is still brace-short when the planner's
	// JSON contract sees it. Repairing here would hide an engine-side loss.
	var any any
	if err := json.Unmarshal([]byte(res.Text), &any); err == nil {
		t.Fatal("the text parsed as JSON — something repaired the engine's output (R1)")
	}
}

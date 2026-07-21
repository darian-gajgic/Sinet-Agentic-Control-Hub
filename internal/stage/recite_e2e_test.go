package stage_test

// Recitation e2e (P3-B3-4; Spec S05.3, S05.4; Research/18 §7-C1): the
// full loop with the ENGINE faked and everything platform-side real —
// real lowering compiles the PostToolUse valve into the settings the fake
// engine reads; the real reciter computes dueness from persisted run
// events at the ⚙ REGISTRY DEFAULT interval and authors the pending file;
// the fake engine execs the REAL delivery valve (this test binary in
// engine-hook mode → claudecli.RunPostToolUseHook) exactly the way the
// engine contract does, and the delivered additionalContext round-trips
// into the session's result; the fires log becomes hash-VERIFIED
// recitation manifest run events. Zero paid calls — the engine-side
// PostToolUse behavior itself is the probe's paid evidence
// (P3/measurements/2026-07-21-posttooluse-additionalcontext-probe.md).

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// reciteFakeTurns: with the registry-default ⚙ interval (10) the reciter
// authors after the 10th persisted turn; the fake delivers at its last
// (12th) tool boundary.
const reciteFakeTurns = 12

// fakeHookMain is the engine-hook re-exec target (TestMain routes argv[1]
// == "engine-hook" here): the REAL delivery valve behind the compiled
// hook command shape `<bin> engine-hook --ctl '<dir>' --post-tool-use`.
func fakeHookMain(args []string) int {
	ctl := ""
	post := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--ctl":
			if i+1 < len(args) {
				ctl = args[i+1]
				i++
			}
		case "--post-tool-use":
			post = true
		}
	}
	if !post {
		fmt.Fprintln(os.Stderr, "fake hook: only --post-tool-use is wired in the stage fake")
		return 1
	}
	if err := claudecli.RunPostToolUseHook(os.Stdin, os.Stdout, ctl); err != nil {
		fmt.Fprintln(os.Stderr, "fake hook:", err)
		return 1
	}
	return 0
}

// reciteFakeEngine streams a multi-turn session: init, turns 1..N (one
// assistant message + message_stop each — one paid call per turn), and at
// the LAST tool boundary runs the compiled PostToolUse command from the
// lowered settings (the probe-recorded engine contract: hook per matched
// tool call, stdout additionalContext reaches the model). The delivered
// content rides the final result so the platform-visible ResultText
// proves the round-trip.
func reciteFakeEngine(sid string) int {
	emit := func(v any) {
		raw, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		fmt.Println(string(raw))
	}
	usage := map[string]any{
		"input_tokens": 50, "output_tokens": 10,
		"cache_read_input_tokens": 0, "cache_creation_input_tokens": 0,
	}
	emit(map[string]any{"type": "system", "subtype": "init", "cwd": "/tmp/fake-cwd",
		"session_id": sid, "model": "claude-haiku-4-5", "permissionMode": "default", "tools": []string{"Bash"}})

	injected := ""
	for i := 1; i <= reciteFakeTurns; i++ {
		if i == reciteFakeTurns {
			injected = runCompiledValve(sid, fmt.Sprintf("toolu_fake_%02d", i))
		}
		emit(map[string]any{"type": "assistant", "message": map[string]any{
			"id": fmt.Sprintf("msg_rc_%02d", i), "model": "claude-haiku-4-5",
			"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("turn %d.", i)}},
			"usage":   usage,
		}})
		emit(map[string]any{"type": "stream_event", "session_id": sid, "event": map[string]any{"type": "message_stop"}})
	}
	final := "done without recitation"
	if injected != "" {
		final = "RECITATION-ECHO:\n" + injected
	}
	emit(map[string]any{"type": "result", "subtype": "success", "is_error": false,
		"result": final, "session_id": sid, "stop_reason": "end_turn",
		"terminal_reason": "completed", "num_turns": reciteFakeTurns, "total_cost_usd": 0.01,
		"usage": usage})
	return 0
}

// runCompiledValve reads the lowered settings named on the engine argv,
// extracts the compiled PostToolUse command, waits (bounded) for the
// platform reciter to author the pending file, and executes the command
// with a PostToolUse-shaped stdin — exactly the engine's hook contract.
// Empty return = quiet path or timeout (the test's assertions then fail
// loudly).
func runCompiledValve(sid, toolUseID string) string {
	settingsPath := ""
	for i, a := range os.Args {
		if a == "--settings" && i+1 < len(os.Args) {
			settingsPath = os.Args[i+1]
		}
	}
	if settingsPath == "" {
		return ""
	}
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		return ""
	}
	var es struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &es); err != nil {
		return ""
	}
	ptu := es.Hooks["PostToolUse"]
	if len(ptu) == 0 || len(ptu[0].Hooks) == 0 {
		return ""
	}
	// Dueness lands once the driver has persisted the ⚙ interval-th turn;
	// wait for the authored pending before firing the boundary.
	pending := filepath.Join(filepath.Dir(settingsPath), "gate-ctl", "recite", "pending.json")
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if _, err := os.Stat(pending); err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	cmd := exec.Command("sh", "-c", ptu[0].Hooks[0].Command)
	cmd.Stdin = strings.NewReader(fmt.Sprintf(
		`{"session_id":%q,"tool_use_id":%q,"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"echo x"},"tool_response":"x","duration_ms":3}`,
		sid, toolUseID))
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return ""
	}
	var dec struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out, &dec); err != nil {
		return ""
	}
	return dec.HookSpecificOutput.AdditionalContext
}

// reciteHarness wires the stage runtime with the hook command pointed at
// THIS binary's engine-hook mode, so the compiled settings drive the real
// valve.
type reciteHarness struct {
	t       *testing.T
	db      *storage.DB
	log     *eventlog.Log
	runs    *run.Store
	led     *ledger.Store
	sk      *stage.Skeleton
	runRoot string
}

func newReciteHarness(t *testing.T) *reciteHarness {
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
	led := ledger.NewStore(db, log)
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	root := t.TempDir()
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: gates.NewCheckpoints(db, log),
		Ledger: led, Settings: reg,
		Adapters: map[string]adapters.Adapter{
			adapters.SubstrateClaudeCLI: &claudecli.Adapter{
				Binary:      self,
				HookCmd:     self + " engine-hook",
				Settings:    reg,
				Env:         append(os.Environ(), "SINET_STAGE_FAKE=1"),
				CancelGrace: 500 * time.Millisecond,
			},
		},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
		CopyAsideDir: filepath.Join(root, "copy-aside"),
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	return &reciteHarness{t: t, db: db, log: log, runs: runs, led: led,
		sk: sk, runRoot: filepath.Join(root, "runs")}
}

func (h *reciteHarness) runningRun(taskID, runID string) run.Run {
	h.t.Helper()
	ctx := context.Background()
	if err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES (?, 'u-recite', 'recite e2e', ?)`,
			taskID, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		h.t.Fatalf("create task: %v", err)
	}
	if _, err := h.runs.Create(ctx, run.NewRun{
		ID: runID, UserID: "u-recite", TaskID: taskID,
		Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic,
	}); err != nil {
		h.t.Fatalf("create run: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := h.runs.Transition(ctx, runID, st, run.TransitionOptions{
			Reason: "test admission", Actor: run.ActorPlatform,
		}); err != nil {
			h.t.Fatalf("admit: %v", err)
		}
	}
	r, err := h.runs.Get(ctx, runID)
	if err != nil {
		h.t.Fatal(err)
	}
	return r
}

func TestRecitationE2EDeliversManifestsAndJournals(t *testing.T) {
	h := newReciteHarness(t)
	ctx := context.Background()
	const taskID, runID = "t-recite", "t-recite.execute"
	r := h.runningRun(taskID, runID)

	// Seed the ledger: objective (v1) then working state with
	// next_actions (v2) — the recitation body's source of truth.
	if _, err := h.led.SetObjective(ctx, runID, "operator", ledger.ObjectiveAC{
		Objective: "Prove the recitation loop end to end.",
		AcceptanceCriteria: []ledger.AcceptanceCriterion{
			{N: 1, Plain: "A due recitation reaches the session."},
		},
		SpecVersion: "spec-v1",
	}); err != nil {
		t.Fatalf("SetObjective: %v", err)
	}
	verbs := h.led.SessionVerbs(runID, "seed", r.Generation)
	current := "w1"
	next := []string{"finish the recitation e2e", "close the ledger cleanly"}
	if _, err := verbs.State(ctx, ledger.StateUpdate{
		Upserts: []ledger.WorkItem{{ID: "w1", Summary: "drive the loop", Status: ledger.StatusInProgress}},
		Current: &current, NextActions: &next,
	}); err != nil {
		t.Fatalf("State: %v", err)
	}

	res, err := h.sk.Session(ctx, stage.SessionInput{
		RunID: runID, Stage: "exec-1",
		Assemble:     true,
		Tools:        []string{"Bash"},
		Instructions: "SINET-STAGE: recite\nWork the plan; recitations arrive at tool boundaries.",
		Model:        "claude-haiku-4-5",
	})
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if res.Outcome.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome = %q (%s)", res.Outcome.Kind, res.Outcome.Detail)
	}

	// The delivered recitation reached the session mid-run and rode into
	// its final output — platform-authored state/next_actions, verbatim.
	if !strings.Contains(res.Text, "RECITATION-ECHO") {
		t.Fatalf("no recitation delivered; result = %q", res.Text)
	}
	for _, want := range []string{"SINET RECITATION", "finish the recitation e2e", "ledger_version: 2"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("delivered recitation missing %q; result = %q", want, res.Text)
		}
	}

	// Consumed on the airlock: pending gone, the claimed copy + exactly
	// one fire retained; the stage-authored bytes parse as the adapter's
	// contract type (the mirror-shape pin).
	ctl := filepath.Join(h.runRoot, runID, "work", "exec-1", "gate-ctl")
	if _, err := os.Stat(filepath.Join(ctl, "recite", "pending.json")); !os.IsNotExist(err) {
		t.Errorf("pending survived delivery (err=%v)", err)
	}
	entries, err := os.ReadDir(filepath.Join(ctl, "recite"))
	if err != nil || len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "delivered-") {
		t.Fatalf("delivered evidence = %v (err=%v)", entries, err)
	}
	deliveredRaw, err := os.ReadFile(filepath.Join(ctl, "recite", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var pin claudecli.PendingRecitation
	if err := json.Unmarshal(deliveredRaw, &pin); err != nil {
		t.Fatalf("stage-authored pending does not parse as the adapter contract: %v (%s)", err, deliveredRaw)
	}
	if pin.LedgerVersion != 2 || pin.Content == "" || pin.ContentSHA256 == "" || pin.WrittenAt == "" {
		t.Errorf("pending contract fields = %+v", pin)
	}
	fires, err := claudecli.ReadReciteFires(ctl)
	if err != nil || len(fires) != 1 {
		t.Fatalf("fires = %v err=%v, want exactly one delivery", fires, err)
	}
	if fires[0].LedgerVersion != 2 || fires[0].ToolUseID != "toolu_fake_12" {
		t.Errorf("fire = %+v", fires[0])
	}

	// The delivery is manifested + journaled: one context.manifest run
	// event, kind recitation, at the pinned revision, hash-VERIFIED
	// against the platform re-rendering (empty disposition). The test
	// recomputes the expected hash independently from the point read.
	doc, err := h.led.AtVersion(ctx, taskID, 2)
	if err != nil {
		t.Fatalf("AtVersion: %v", err)
	}
	wantSum := sha256.Sum256([]byte(ledger.RecitationText(doc)))
	if got := hex.EncodeToString(wantSum[:]); fires[0].ContentSHA256 != got {
		t.Errorf("fire hash %q != independent re-render at v2 %q", fires[0].ContentSHA256, got)
	}
	var payloads []string
	rows, err := h.db.QueryContext(ctx,
		`SELECT payload FROM run_events WHERE run_id = ? AND type = 'context.manifest' ORDER BY event_seq`, runID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	type manifest struct {
		Kind          string `json:"kind"`
		Stage         string `json:"stage"`
		LedgerVersion int64  `json:"ledger_version"`
		SessionID     string `json:"session_id"`
		ToolUseID     string `json:"tool_use_id"`
		Items         []struct {
			ItemID      string `json:"item_id"`
			ContentHash string `json:"content_hash"`
			Version     string `json:"version"`
			Disposition string `json:"disposition"`
		} `json:"items"`
	}
	var recitations []manifest
	for _, p := range payloads {
		var m manifest
		if err := json.Unmarshal([]byte(p), &m); err != nil {
			t.Fatalf("manifest payload: %v (%s)", err, p)
		}
		if m.Kind == "recitation" {
			recitations = append(recitations, m)
		}
	}
	if len(recitations) != 1 {
		t.Fatalf("recitation manifests = %d (%v), want exactly one", len(recitations), payloads)
	}
	m := recitations[0]
	if m.Stage != "exec-1" || m.LedgerVersion != 2 || m.ToolUseID != "toolu_fake_12" {
		t.Errorf("manifest identity = %+v", m)
	}
	if len(m.Items) != 1 || m.Items[0].ItemID != "ledger.recitation" {
		t.Fatalf("manifest items = %+v", m.Items)
	}
	if m.Items[0].ContentHash != fires[0].ContentSHA256 {
		t.Errorf("manifest hash %q != fire hash %q", m.Items[0].ContentHash, fires[0].ContentSHA256)
	}
	if m.Items[0].Disposition != "" {
		t.Errorf("disposition = %q, want empty (delivered bytes verified against the platform rendering at v2)", m.Items[0].Disposition)
	}

	// Journal sanity: the session's paid calls all landed as run events
	// (the turns the dueness computation counted).
	var usageEvents int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_events WHERE run_id = ? AND type = 'engine.usage'`, runID).Scan(&usageEvents); err != nil {
		t.Fatal(err)
	}
	if usageEvents != reciteFakeTurns {
		t.Errorf("engine.usage events = %d, want %d (turns-dueness rides persisted run events)", usageEvents, reciteFakeTurns)
	}
}

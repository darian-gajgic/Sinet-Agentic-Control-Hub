package stage_test

// The walking-skeleton dev-mode E2E (the B2-4 acceptance test): request →
// interview → approved plan → executed step → verified deliverable →
// receipts, end to end through the REAL spine — intake pipeline, scheduler
// claim loop, stage dispatcher, adapter Driver, ledger assembly,
// verification drain — with the ENGINE faked: the test binary re-execs
// itself as a `claude -p`-shaped process (the claudecli e2e / kill-9
// harness pattern, CONVENTIONS §8/§10) that keys a canned stream-json
// reply off the SINET-STAGE prompt marker. Zero paid calls; the identical
// wiring against the real engine is the tier-L live smoke
// (live_smoke_test.go) and the B2 gate demo.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

func TestMain(m *testing.M) {
	if os.Getenv("SINET_STAGE_FAKE") == "1" {
		os.Exit(stageFakeMain())
	}
	os.Exit(m.Run())
}

// fakeDeliverable is the deliverable the fake execute session produces;
// the fake judge quotes exact substrings of it as extractive evidence.
const fakeDeliverable = "A short note on SQLite: it keeps every promise a database makes, in a single file.\n" +
	"Its tests outnumber its lines, and it runs everywhere.\n"

// stageFakeMain is the fake engine: read the prompt from stdin, pick the
// canned reply by SINET-STAGE marker, stream it as `claude -p
// --output-format stream-json` frames.
func stageFakeMain() int {
	stdin := new(strings.Builder)
	buf := make([]byte, 64<<10)
	for {
		n, err := os.Stdin.Read(buf)
		stdin.Write(buf[:n])
		if err != nil {
			break
		}
	}
	prompt := stdin.String()
	marker := func(kind string) bool { return strings.Contains(prompt, "SINET-STAGE: "+kind) }

	var payload string
	var sid string
	switch {
	case marker("plan-draft"), marker("plan-revise"):
		sid = "0000f00d-0000-4000-8000-000000000001"
		payload = fakePairJSON()
	case marker("critique"):
		sid = "0000f00d-0000-4000-8000-000000000002"
		payload = `{"kind":"pass"}`
	case marker("judge-compliance"):
		sid = "0000f00d-0000-4000-8000-000000000004"
		payload = fakeAxis1For(os.Getenv("SINET_STAGE_FAKE_JUDGE"), prompt)
	case marker("judge-sanity"):
		sid = "0000f00d-0000-4000-8000-000000000005"
		payload = fakeAxis2JSON()
	case marker("execute"):
		sid = "0000f00d-0000-4000-8000-000000000003"
		payload = fakeDeliverable
	case marker("revise"):
		sid = "0000f00d-0000-4000-8000-000000000006"
		payload = fakeReviseOutput(prompt)
	default:
		fmt.Fprintf(os.Stderr, "fake engine: no SINET-STAGE marker in prompt\n")
		return 7
	}
	emit := func(v any) {
		raw, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		fmt.Println(string(raw))
	}
	usage := map[string]any{
		"input_tokens": 100, "output_tokens": 20,
		"cache_read_input_tokens": 0, "cache_creation_input_tokens": 40,
		"cache_creation": map[string]any{"ephemeral_5m_input_tokens": 40, "ephemeral_1h_input_tokens": 0},
	}
	emit(map[string]any{"type": "system", "subtype": "init", "cwd": "/tmp/fake-cwd",
		"session_id": sid, "model": "claude-haiku-4-5", "permissionMode": "default", "tools": []string{}})
	emit(map[string]any{"type": "assistant", "message": map[string]any{
		"id": "msg_" + sid[len(sid)-4:], "model": "claude-haiku-4-5",
		"content": []map[string]any{{"type": "text", "text": payload}},
		"usage":   usage,
	}})
	emit(map[string]any{"type": "stream_event", "session_id": sid, "event": map[string]any{"type": "message_stop"}})
	emit(map[string]any{"type": "result", "subtype": "success", "is_error": false,
		"result": payload, "session_id": sid, "stop_reason": "end_turn",
		"terminal_reason": "completed", "num_turns": 1, "total_cost_usd": 0.003,
		"usage": usage})
	return 0
}

func fakePairJSON() string {
	pair := map[string]any{
		"spec": map[string]any{
			"restatement": "Write a short appreciation note about the SQLite database engine.",
			"acs": []map[string]any{
				{"n": 1, "plain": "The note mentions SQLite by name."},
				{"n": 2, "plain": "The note is short (a few sentences)."},
			},
			"assumptions": []map[string]any{
				{"text": "Plain-text note; no publication anywhere.", "origin": "planner"},
			},
			"out_of_scope": []string{"publishing the note outside the workspace"},
		},
		"plan": map[string]any{
			"steps": []map[string]any{{
				"id": "S-1", "title": "Write the note", "class": "C2",
				"done_when": "the final message contains the complete note text mentioning SQLite",
				"write_set": []string{"note.md"},
			}},
			"coverage": map[string]any{"AC-1": []string{"S-1"}, "AC-2": []string{"S-1"}},
			"est":      map[string]any{"known": false, "basis": "tiny fixed plan"},
		},
	}
	raw, err := json.Marshal(pair)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func fakeAxis1JSON() string {
	res := map[string]any{
		"verdicts": []map[string]any{
			{"key": "AC-1", "pass": true, "evidence": "SQLite"},
			{"key": "AC-2", "pass": true, "evidence": "A short note"},
		},
	}
	raw, _ := json.Marshal(res)
	return string(raw)
}

// fakeAxis1For applies the judge knob (SINET_STAGE_FAKE_JUDGE): "" passes;
// "always-fail" fails AC-1 forever; "fail-unless-guidance" fails AC-1 until
// the artifact (riding the judge prompt) carries the guidance marker the
// fake revise session applies.
func fakeAxis1For(mode, prompt string) string {
	pass := true
	switch mode {
	case "always-fail":
		pass = false
	case "fail-unless-guidance":
		pass = strings.Contains(prompt, "GUIDANCE-APPLIED")
	}
	if pass {
		return fakeAxis1JSON()
	}
	res := map[string]any{
		"verdicts": []map[string]any{
			{"key": "AC-1", "pass": false},
			{"key": "AC-2", "pass": true, "evidence": "A short note"},
		},
		"findings": []map[string]any{{
			"severity": "blocker", "category": "AC-BLOCKER", "criterion": "AC-1",
			"anchor": "note:1", "text": "AC-1 is not met by this revision",
		}},
	}
	raw, _ := json.Marshal(res)
	return string(raw)
}

// fakeReviseOutput rebuilds the deliverable from the base (fresh
// non-cumulative revision), applies the guidance marker exactly when the
// retry package carries requester findings, and appends a unique touch line
// so revisions are byte-distinct (the V0 diff-empty gate sees real edits).
func fakeReviseOutput(prompt string) string {
	out := fakeDeliverable
	// The fake reads the raw stream-json user frame, so the retry-package
	// JSON arrives once-escaped: match both forms of the requester flag.
	if strings.Contains(prompt, `"requester": true`) || strings.Contains(prompt, `\"requester\": true`) {
		out += "GUIDANCE-APPLIED\n"
	}
	return out + fmt.Sprintf("touch-%d\n", time.Now().UnixNano())
}

func fakeAxis2JSON() string {
	res := map[string]any{
		"probe_notes": map[string]any{
			"reasonable-user":       "a reasonable user gets exactly the asked-for note",
			"implicit-expectations": "nothing material absent for a note this size",
			"side-effects":          "no unrequested changes",
			"expert-standard":       "fine for an appreciation note",
		},
	}
	raw, _ := json.Marshal(res)
	return string(raw)
}

// ---- harness ----

type harness struct {
	t            *testing.T
	db           *storage.DB
	log          *eventlog.Log
	runs         *run.Store
	cps          *gates.Checkpoints
	led          *ledger.Store
	sk           *stage.Skeleton
	sched        *scheduler.Scheduler
	sur          *stage.Surface
	artifactRoot string
}

func newHarness(t *testing.T, extraEnv ...string) *harness {
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
	cps := gates.NewCheckpoints(db, log)
	led := ledger.NewStore(db, log)

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	root := t.TempDir()
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Ledger: led, Settings: reg,
		Adapters: map[string]adapters.Adapter{
			adapters.SubstrateClaudeCLI: &claudecli.Adapter{
				Binary:      self,
				HookCmd:     "/opt/sinet/bin/sinet engine-hook",
				Settings:    reg,
				Env:         append(append(os.Environ(), "SINET_STAGE_FAKE=1"), extraEnv...),
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
	priceTable := metering.NewEffectiveDatedTable("empty-v0")
	exceptions := metering.NoMeteredExceptions()
	sched, err := scheduler.New(scheduler.Config{
		DB: db, Runs: runs, Settings: reg, Dispatcher: sk,
		Receipts:     metering.NewReceipts(db, metering.NewLedger(db, priceTable, exceptions, reg), exceptions),
		LeaseTTL:     time.Minute,
		PollInterval: time.Hour, // manual Ticks only
	})
	if err != nil {
		t.Fatalf("scheduler.New: %v", err)
	}
	sk.Bind(sched)
	return &harness{t: t, db: db, log: log, runs: runs, cps: cps, led: led,
		sk: sk, sched: sched, sur: sk.Surface(), artifactRoot: filepath.Join(root, "artifacts")}
}

// tick runs one synchronous admission cycle and waits for its dispatches.
func (h *harness) tick(ctx context.Context) int {
	h.t.Helper()
	n, err := h.sched.Tick(ctx)
	if err != nil {
		h.t.Fatalf("Tick: %v", err)
	}
	h.sched.WaitInFlight()
	return n
}

type view struct {
	TaskID    string `json:"task_id"`
	Kanban    string `json:"kanban_status"`
	Phase     string `json:"phase"`
	Tier      string `json:"tier"`
	OpenAskID string `json:"open_ask_id"`
	OpenCard  struct {
		Kind      string `json:"kind"`
		Questions []struct {
			ID string `json:"id"`
		} `json:"questions"`
	} `json:"open_card"`
	Runs []struct {
		RunID      string `json:"run_id"`
		Role       string `json:"role"`
		State      string `json:"state"`
		HasReceipt bool   `json:"has_receipt"`
	} `json:"runs"`
}

func decodeView(t *testing.T, raw json.RawMessage) view {
	t.Helper()
	var v view
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode view: %v (%s)", err, raw)
	}
	return v
}

func TestWalkingSkeletonE2E(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	const owner = "u-operator"

	// 1. Submit a request (Stage 0 + admission; wording avoids the P47 cue
	// classes — no data-bearing research obligations in this walk).
	raw, err := h.sur.Submit(ctx, owner, json.RawMessage(
		`{"title":"SQLite note","text":"Write a short appreciation note about the SQLite database engine."}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	v := decodeView(t, raw)
	taskID := v.TaskID
	if taskID == "" {
		t.Fatalf("no task id in %s", raw)
	}
	if v.Tier != "high" {
		t.Fatalf("tier %q, want fail-closed high (no classifier at B2, S06.2)", v.Tier)
	}

	// 2. First dispatch: the interview card opens and parks the intake run.
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the intake run", n)
	}
	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	v = decodeView(t, raw)
	if v.OpenAskID == "" || v.OpenCard.Kind != "interview" {
		t.Fatalf("expected an open interview card, got %s", raw)
	}

	// 3. Force-proceed: the interview stops, drafting + critique run as
	// fake-engine ceremony sessions, the approval card opens.
	raw, err = h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"force_proceed":true}`), false)
	if err != nil {
		t.Fatalf("Answer(force_proceed): %v", err)
	}
	v = decodeView(t, raw)
	if v.OpenCard.Kind != "approval" {
		t.Fatalf("expected the approval card, got %s", raw)
	}

	// 4. High-tier approval demands the same-request PIN step-up (S01.9).
	if _, err := h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"action":"approve"}`), false); err == nil {
		t.Fatal("high-tier approval without PIN verification must be refused (S01.9 step-up)")
	}

	// 5. Approve (step-up verified): the intake run completes with its
	// ceremony receipt; the execute run is launched.
	raw, err = h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"action":"approve"}`), true)
	if err != nil {
		t.Fatalf("Answer(approve): %v", err)
	}
	v = decodeView(t, raw)
	if v.Phase != "approved" {
		t.Fatalf("phase %q after approve", v.Phase)
	}

	// 6. Execute dispatch: one fresh session per plan step; deliverable
	// captured; verify run launched.
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the execute run", n)
	}
	// 7. Verify dispatch: V0 → (no pack: generic domain, degraded V1 per
	// S07.8) → V2 both axes → SHIP → ledger items verified.
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the verify run", n)
	}

	// ---- assertions over the durable record ----

	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	v = decodeView(t, raw)
	if v.Kanban != "done" {
		t.Fatalf("kanban %q, want done (%s)", v.Kanban, raw)
	}
	wantRuns := map[string]string{"intake": "completed", "execute": "completed", "verify": "completed"}
	seen := map[string]bool{}
	for _, r := range v.Runs {
		if want, ok := wantRuns[r.Role]; ok {
			seen[r.Role] = true
			if r.State != want {
				t.Errorf("%s run state %q, want %q", r.Role, r.State, want)
			}
			if !r.HasReceipt {
				t.Errorf("%s run has no receipt (S10.1: receipts per run-end)", r.Role)
			}
		}
	}
	for role := range wantRuns {
		if !seen[role] {
			t.Errorf("no %s run in task view %s", role, raw)
		}
	}

	// Checkpoints per role run, all carrying the LIVE ledger-revision block
	// (S02.4c via B2-1's CheckpointRef): intake 2 paid calls (draft +
	// critique), execute 1, verify 2 (compliance + sanity).
	wantCPs := map[string]int{
		taskID + ".intake":  2,
		taskID + ".execute": 1,
		taskID + ".verify":  2,
	}
	for runID, want := range wantCPs {
		var n int
		if err := h.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM checkpoints WHERE run_id = ?`, runID).Scan(&n); err != nil {
			t.Fatalf("count checkpoints: %v", err)
		}
		if n != want {
			t.Errorf("%s: %d checkpoints, want %d", runID, n, want)
		}
		var withLedger int
		if err := h.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM checkpoints WHERE run_id = ? AND ledger_revision <> ''`, runID).Scan(&withLedger); err != nil {
			t.Fatalf("count ledger revisions: %v", err)
		}
		if withLedger != n {
			t.Errorf("%s: %d/%d checkpoints carry the ledger-revision block (S02.4c must be live)", runID, withLedger, n)
		}
	}

	// Receipt purposes itemize ceremony / execution / verification
	// (S06.10/S07.11 via the role-run derivation).
	wantPurpose := map[string]string{
		taskID + ".intake":  "ceremony",
		taskID + ".execute": "execution",
		taskID + ".verify":  "verification",
	}
	for runID, want := range wantPurpose {
		var usage string
		if err := h.db.QueryRowContext(ctx,
			`SELECT usage_json FROM receipts WHERE run_id = ?`, runID).Scan(&usage); err != nil {
			t.Fatalf("receipt %s: %v", runID, err)
		}
		var rec struct {
			Items []struct {
				Purpose string `json:"purpose"`
				Calls   int64  `json:"calls"`
			} `json:"items"`
		}
		if err := json.Unmarshal([]byte(usage), &rec); err != nil {
			t.Fatalf("decode receipt: %v", err)
		}
		if len(rec.Items) == 0 {
			t.Errorf("%s: receipt has no line items", runID)
			continue
		}
		for _, li := range rec.Items {
			if li.Purpose != want {
				t.Errorf("%s: line item purpose %q, want %q", runID, li.Purpose, want)
			}
		}
	}

	// The ledger: pinned §1 (frozen ACs), the step item VERIFIED via the
	// S07 SHIP path, the deliverable artifact registered, stage closed.
	doc, found, err := h.led.Current(ctx, taskID)
	if err != nil || !found {
		t.Fatalf("ledger Current: %v found=%v", err, found)
	}
	if len(doc.ObjectiveAC.AcceptanceCriteria) != 2 || doc.ObjectiveAC.SpecVersion != "spec-v1" {
		t.Errorf("pinned objective_ac = %+v", doc.ObjectiveAC)
	}
	var stepItem *ledger.WorkItem
	for i := range doc.State.Items {
		if doc.State.Items[i].ID == "S-1" {
			stepItem = &doc.State.Items[i]
		}
	}
	if stepItem == nil || stepItem.Status != ledger.StatusVerified {
		t.Errorf("work item S-1 = %+v, want verified (S07 SetVerified)", stepItem)
	}
	foundDeliverable := false
	for _, a := range doc.Artifacts {
		if a.Kind == "deliverable" {
			foundDeliverable = true
			if got, err := os.ReadFile(a.Ref); err != nil || string(got) != fakeDeliverable {
				t.Errorf("deliverable file %q: err=%v", a.Ref, err)
			}
		}
	}
	if !foundDeliverable {
		t.Error("no deliverable artifact in the ledger")
	}

	// Trace manifests exist for the assembled stages (S05.4: one manifest
	// per assembly) and the verify round record shipped.
	for _, q := range []struct {
		runID, typ string
		min        int
	}{
		{taskID + ".intake", ledger.EventContextManifest, 1}, // plan-draft assembly
		{taskID + ".execute", ledger.EventContextManifest, 1},
		{taskID + ".verify", ledger.EventContextManifest, 1}, // clean judge assembly
		{taskID + ".verify", "verify.round", 1},
	} {
		var n int
		if err := h.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM run_events WHERE run_id = ? AND type = ?`, q.runID, q.typ).Scan(&n); err != nil {
			t.Fatalf("count %s events: %v", q.typ, err)
		}
		if n < q.min {
			t.Errorf("%s: %d %s events, want ≥ %d", q.runID, n, q.typ, q.min)
		}
	}

	// No overflow events fired on this tiny walk (⚙ thresholds far away).
	var overflow int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_events WHERE type = ?`, stage.EventContextOverflow).Scan(&overflow); err != nil {
		t.Fatalf("count overflow events: %v", err)
	}
	if overflow != 0 {
		t.Errorf("%d context.overflow events on a tiny walk", overflow)
	}

	// Queue rows all settled.
	var open int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM queue WHERE status <> 'done'`).Scan(&open); err != nil {
		t.Fatalf("count queue rows: %v", err)
	}
	if open != 0 {
		t.Errorf("%d unsettled queue rows", open)
	}

	// The SHIP verdict is durable in the verify round record.
	var payload string
	if err := h.db.QueryRowContext(ctx,
		`SELECT payload FROM run_events WHERE run_id = ? AND type = 'verify.round' ORDER BY event_seq DESC LIMIT 1`,
		taskID+".verify").Scan(&payload); err != nil {
		t.Fatalf("read verify.round: %v", err)
	}
	if !strings.Contains(payload, `"SHIP`) {
		t.Errorf("verify.round verdict not SHIP: %s", payload)
	}

	_ = verify.VerdictShip // documentation anchor: the drain's SHIP path is what this walk proves
}

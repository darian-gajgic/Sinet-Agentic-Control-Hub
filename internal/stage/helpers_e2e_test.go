package stage_test

// helpers_e2e_test.go — the S04 spawn/helper conformance battery (S04.5:
// containment and enforcement are proven, not assumed): spawn-record-
// before-session, the D6 report→coordinator-L0 shape, the refusal set
// (trigger, brief, depth-3 recursion, native-spawn/lateral tool, spawn
// budget), reject→fresh-retry→FAILED/ESCALATED, mid-run kill →
// clean SALVAGE with the explicit unfinished marker, and sibling-failure
// containment. Fake engine only — zero paid calls.

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
)

// helperRun creates and admits a coordinator run the helpers serve.
func (h *routedHarness) helperRun(taskID string) run.Run {
	h.t.Helper()
	ctx := context.Background()
	if err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES (?, 'u-helper', 'helper harness', ?)`,
			taskID, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		h.t.Fatalf("create task: %v", err)
	}
	runID := taskID + ".execute"
	if _, err := h.runs.Create(ctx, run.NewRun{
		ID: runID, UserID: "u-helper", TaskID: taskID,
		Substrate: "claude-cli", Lane: "anthropic",
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

func goodBrief() stage.HelperBrief {
	return stage.HelperBrief{
		Objective:      "Trawl the notes archive for entries about SQLite.",
		OutputContract: "FINDINGS / EVIDENCE / GAPS",
		ToolsSources:   "Read only; start in notes/",
		Boundaries:     "read-only; do NOT edit anything",
		Context:        "The task is writing an appreciation note; the archive may hold prior notes.",
		Tools:          []string{"Read"},
		Class:          "C1",
	}
}

func TestHelperSpawnRecordAndReportLandInCoordinatorL0(t *testing.T) {
	h := newRoutedHarness(t)
	ctx := context.Background()
	r := h.helperRun("t-helper1")

	res, err := h.sk.Spawn(ctx, stage.SpawnRequest{
		RunID:   r.ID,
		Trigger: stage.TriggerContext,
		Reason:  "archive trawl would flood the coordinator (T-CTX)",
		Brief:   goodBrief(),
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if res.Outcome != stage.HelperAccepted {
		t.Fatalf("outcome = %s", res.Outcome)
	}
	if !strings.Contains(res.Report, "FINDINGS") {
		t.Fatalf("report = %q", res.Report)
	}

	// Spawn record written with the full S04.4 field set, BEFORE any
	// lifecycle event.
	spawns := eventsOfType(t, h.db, r.ID, "helper.spawned")
	if len(spawns) != 1 {
		t.Fatalf("spawn records = %d", len(spawns))
	}
	rec := spawns[0]
	for _, field := range []string{"spawn_id", "task_id", "owner", "parent_session", "child_session", "depth", "trigger", "reason", "brief_hash", "model", "lane", "budget"} {
		if _, ok := rec[field]; !ok {
			t.Fatalf("spawn record missing %s: %v", field, rec)
		}
	}
	if rec["trigger"] != "T-CTX" || rec["parent_session"] != "coordinator" || rec["depth"].(float64) != 1 {
		t.Fatalf("spawn record = %v", rec)
	}
	if b, _ := rec["budget"].(map[string]any); b["turns"].(float64) != 20 || b["tokens"].(float64) != 80_000 {
		t.Fatalf("⚙ budget defaults not on the record: %v", rec["budget"])
	}

	// Lifecycle: SPAWNED → RUNNING → RETURNED → ACCEPTED, every transition
	// an event (S04.5).
	var states []string
	for _, ev := range eventsOfType(t, h.db, r.ID, "orchestration.helper") {
		states = append(states, ev["state"].(string))
	}
	want := []string{"SPAWNED", "RUNNING", "RETURNED", "ACCEPTED"}
	if strings.Join(states, ",") != strings.Join(want, ",") {
		t.Fatalf("lifecycle = %v, want %v", states, want)
	}

	// Helper routing rode the SAME pipeline: routing.decided with the
	// spawn cause on the coordinator's run.
	decided := eventsOfType(t, h.db, r.ID, "routing.decided")
	if len(decided) != 1 || decided[0]["cause"] != "helper-spawn" {
		t.Fatalf("routing.decided = %v", decided)
	}

	// D6: the report landed in the COORDINATOR's L0 — the run's ledger
	// carries the artifact ref (platform-written; the helper held no
	// verbs), and the file is the helper's words verbatim.
	if res.ReportPath == "" {
		t.Fatal("no report path")
	}
	raw, err := os.ReadFile(res.ReportPath)
	if err != nil {
		t.Fatalf("report file: %v", err)
	}
	if string(raw) != res.Report {
		t.Fatal("filed report is not verbatim")
	}
	doc, found, err := h.led.Current(ctx, r.TaskID)
	if err != nil || !found {
		t.Fatalf("ledger doc: %v found=%v", err, found)
	}
	foundRef := false
	for _, a := range doc.Artifacts {
		if a.Kind == "helper-report" && a.Ref == res.ReportPath {
			foundRef = true
		}
	}
	if !foundRef {
		t.Fatalf("helper report not in the coordinator's ledger: %+v", doc.Artifacts)
	}
}

func TestHelperRefusalsAreLoggedEvidence(t *testing.T) {
	h := newRoutedHarness(t)
	ctx := context.Background()
	r := h.helperRun("t-helper2")

	cases := []struct {
		name  string
		check string
		req   stage.SpawnRequest
	}{
		{"no-trigger", "trigger", stage.SpawnRequest{RunID: r.ID, Trigger: "because", Reason: "x", Brief: goodBrief()}},
		{"no-reason", "reason", stage.SpawnRequest{RunID: r.ID, Trigger: stage.TriggerContext, Brief: goodBrief()}},
		{"bad-brief", "brief", stage.SpawnRequest{RunID: r.ID, Trigger: stage.TriggerContext, Reason: "x",
			Brief: stage.HelperBrief{Objective: "only objective"}}},
		{"native-or-lateral-tool", "lateral-or-native-spawn", func() stage.SpawnRequest {
			b := goodBrief()
			b.Tools = []string{"Read", "Task"}
			return stage.SpawnRequest{RunID: r.ID, Trigger: stage.TriggerSpecialization, Reason: "x", Brief: b}
		}()},
		{"unknown-parent", "parent", stage.SpawnRequest{RunID: r.ID, ParentSpawn: "sp-nope",
			Trigger: stage.TriggerContext, Reason: "x", Brief: goodBrief()}},
		{"looser-confinement", "confinement", func() stage.SpawnRequest {
			b := goodBrief()
			b.Class = "C3" // looser than the coordinator envelope (P-T05-1)
			return stage.SpawnRequest{RunID: r.ID, Trigger: stage.TriggerContext, Reason: "x", Brief: b}
		}()},
	}
	for _, c := range cases {
		if _, err := h.sk.Spawn(ctx, c.req); err == nil {
			t.Fatalf("%s: spawn must refuse", c.name)
		}
	}
	refusals := eventsOfType(t, h.db, r.ID, "spawn.refused")
	if len(refusals) != len(cases) {
		t.Fatalf("refusal events = %d, want %d", len(refusals), len(cases))
	}
	for i, c := range cases {
		if refusals[i]["check"] != c.check {
			t.Fatalf("refusal %s named %q, want %q", c.name, refusals[i]["check"], c.check)
		}
	}
	// No spawn record was written for any refused proposal.
	if spawns := eventsOfType(t, h.db, r.ID, "helper.spawned"); len(spawns) != 0 {
		t.Fatalf("refused proposals wrote %d spawn records", len(spawns))
	}
}

func TestHelperDepth3RecursionRefused(t *testing.T) {
	h := newRoutedHarness(t)
	ctx := context.Background()
	r := h.helperRun("t-helper3")

	// Depth 1.
	res1, err := h.sk.Spawn(ctx, stage.SpawnRequest{
		RunID: r.ID, Trigger: stage.TriggerContext, Reason: "d1", Brief: goodBrief(),
	})
	if err != nil {
		t.Fatalf("d1: %v", err)
	}
	// Depth 2 (sub-helper through the SAME API, same triggers — S04.1).
	res2, err := h.sk.Spawn(ctx, stage.SpawnRequest{
		RunID: r.ID, ParentSpawn: res1.SpawnID,
		Trigger: stage.TriggerParallel, Reason: "d2", Brief: goodBrief(),
	})
	if err != nil {
		t.Fatalf("d2: %v", err)
	}
	if res2.Depth != 2 {
		t.Fatalf("depth = %d", res2.Depth)
	}
	// Depth 3: refused at the ⚙ cap (S04.1: nothing in the evidence base
	// demonstrates value at depth ≥ 3).
	if _, err := h.sk.Spawn(ctx, stage.SpawnRequest{
		RunID: r.ID, ParentSpawn: res2.SpawnID,
		Trigger: stage.TriggerContext, Reason: "d3", Brief: goodBrief(),
	}); err == nil {
		t.Fatal("depth-3 recursion must refuse")
	}
	refusals := eventsOfType(t, h.db, r.ID, "spawn.refused")
	if len(refusals) != 1 || refusals[0]["check"] != "depth" {
		t.Fatalf("refusals = %v", refusals)
	}
}

func TestHelperSpawnBudgetExhaustionRefuses(t *testing.T) {
	h := newRoutedHarness(t)
	ctx := context.Background()
	r := h.helperRun("t-helper4")

	// ⚙ orchestration.spawn_budget default = 8 per task incl. retries.
	for i := 0; i < 8; i++ {
		if _, err := h.sk.Spawn(ctx, stage.SpawnRequest{
			RunID: r.ID, Trigger: stage.TriggerContext, Reason: "n", Brief: goodBrief(),
		}); err != nil {
			t.Fatalf("spawn %d: %v", i, err)
		}
	}
	if _, err := h.sk.Spawn(ctx, stage.SpawnRequest{
		RunID: r.ID, Trigger: stage.TriggerContext, Reason: "over", Brief: goodBrief(),
	}); err == nil {
		t.Fatal("9th spawn must refuse (⚙ spawn_budget; overrun only via an operator-visible gate)")
	}
	refusals := eventsOfType(t, h.db, r.ID, "spawn.refused")
	if len(refusals) != 1 || refusals[0]["check"] != "spawn-budget" {
		t.Fatalf("refusals = %v", refusals)
	}
}

func TestHelperRejectRetriesFreshThenFails(t *testing.T) {
	h := newRoutedHarness(t)
	ctx := context.Background()
	r := h.helperRun("t-helper5")

	b := goodBrief()
	b.Objective = "BAD-REPORT: produce junk" // the fake engine returns a non-conformant report
	res, err := h.sk.Spawn(ctx, stage.SpawnRequest{
		RunID: r.ID, Trigger: stage.TriggerContext, Reason: "screen test", Brief: b,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if res.Outcome != stage.HelperFailed {
		t.Fatalf("outcome = %s, want FAILED after the retry", res.Outcome)
	}
	// A retry is a FRESH spawn with a revised brief (fork-don't-poison):
	// two spawn records, distinct ids, the second carrying the revision.
	spawns := eventsOfType(t, h.db, r.ID, "helper.spawned")
	if len(spawns) != 2 {
		t.Fatalf("spawn records = %d, want 2 (original + fresh retry)", len(spawns))
	}
	if spawns[0]["spawn_id"] == spawns[1]["spawn_id"] {
		t.Fatal("retry reused the spawn id — not a fresh spawn")
	}
	if spawns[1]["retry"].(float64) != 1 {
		t.Fatalf("retry counter = %v", spawns[1]["retry"])
	}
	// The dropped facet is recorded in the coordinator's record (1.4).
	doc, _, err := h.led.Current(ctx, r.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range doc.Decisions {
		if strings.Contains(d.Text, "ABANDONED") {
			found = true
		}
	}
	if !found {
		t.Fatalf("failed facet not recorded: %+v", doc.Decisions)
	}
}

func TestHelperEscalationOpensDurableAsk(t *testing.T) {
	h := newRoutedHarness(t)
	ctx := context.Background()
	r := h.helperRun("t-helper6")

	b := goodBrief()
	b.Objective = "BAD-REPORT: produce junk"
	res, err := h.sk.Spawn(ctx, stage.SpawnRequest{
		RunID: r.ID, Trigger: stage.TriggerContext, Reason: "escalate test", Brief: b,
		EscalateOnFailure: true,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if res.Outcome != stage.HelperEscalated {
		t.Fatalf("outcome = %s", res.Outcome)
	}
	// The escalation is a durable ask — a finding that dies in a log is a
	// platform defect (5.6; S04.5).
	var n int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM asks WHERE ask_id LIKE 'ask-helper-%' AND status = 'open' AND run_id = ?`, r.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("open helper asks = %d", n)
	}
}

func TestHelperKillSalvagesAndSiblingsContained(t *testing.T) {
	h := newRoutedHarness(t)
	ctx := context.Background()
	r := h.helperRun("t-helper7")

	// Two concurrent helpers: one dies mid-run, the sibling finishes. One
	// helper's death NEVER cancels siblings and NEVER fails the task
	// (S04.5 containment).
	var wg sync.WaitGroup
	var dead, alive stage.SpawnResult
	var deadErr, aliveErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		b := goodBrief()
		b.Objective = "DIE-NOW: the engine dies mid-run"
		dead, deadErr = h.sk.Spawn(ctx, stage.SpawnRequest{
			RunID: r.ID, Trigger: stage.TriggerParallel, Reason: "doomed facet", Brief: b,
		})
	}()
	go func() {
		defer wg.Done()
		alive, aliveErr = h.sk.Spawn(ctx, stage.SpawnRequest{
			RunID: r.ID, Trigger: stage.TriggerParallel, Reason: "healthy facet", Brief: goodBrief(),
		})
	}()
	wg.Wait()

	if deadErr != nil || aliveErr != nil {
		t.Fatalf("containment: helper outcomes must not error the caller (dead=%v alive=%v)", deadErr, aliveErr)
	}
	if dead.Outcome != stage.HelperSalvaged {
		t.Fatalf("dead outcome = %s, want SALVAGED", dead.Outcome)
	}
	if !strings.Contains(dead.Report, stage.UnfinishedMarker) {
		t.Fatalf("salvaged partial missing the explicit unfinished marker: %q", dead.Report)
	}
	if alive.Outcome != stage.HelperAccepted {
		t.Fatalf("sibling outcome = %s — a sibling death must not touch it", alive.Outcome)
	}
	// The task's run is untouched.
	got, err := h.runs.Get(ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != run.StateRunning {
		t.Fatalf("coordinator run state = %s — helper death is a scheduling event, not a task failure", got.State)
	}
}

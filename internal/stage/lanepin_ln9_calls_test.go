package stage_test

// lanepin_ln9_calls_test.go — P3-LN-9 §9.1/§9.2/§9.7 (S08.8, S00.9 A13, S10.1).
//
// A correct resolver reached through a dropped argument is a broken feature
// (§63 drain r2 R1), so nothing here tests worker.resolveSeat. Every guard
// drives a composed stage through its PRODUCTION surfaces — Submit, the intake
// router seam, Spawn, the execute dispatch — in a world with a second lane
// commissioned, which is the only world in which a pin can mean anything.
//
// $0: the claude-cli fake engine for the ceremony and a recording adapter that
// starts nothing for the commissioned lane. No provider call on any path.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// ln9Adapter reports which engine dispatch actually reached and starts nothing.
// Asserting which LANE was computed proves less: the lane has to survive all
// the way to the adapter lookup to matter (§63-R2/§65 recording-adapter shape).
type ln9Adapter struct {
	name string
	seen *[]string
	mu   *sync.Mutex
}

func (a ln9Adapter) Substrate() string { return a.name }
func (a ln9Adapter) Start(context.Context, adapters.StartRequest) (adapters.Session, error) {
	a.mu.Lock()
	*a.seen = append(*a.seen, a.name)
	a.mu.Unlock()
	return nil, errors.New("ln9 recording adapter starts no engine ($0)")
}
func (a ln9Adapter) Resume(context.Context, adapters.ParkRecord, *adapters.Answer) (adapters.Session, error) {
	return nil, errors.New("ln9 recording adapter starts no engine ($0)")
}

// ln9Harness is the routed e2e harness with ONE commissioned lane, composed the
// way the composition root composes one: the lane, its engine and its seat.
// Everything the pin touches — coverage, the pinnable set, the alternates — is
// derived by stage.New from these three, never hand-built here.
type ln9Harness struct {
	*routedHarness
	seen []string
	mu   sync.Mutex
}

func newLN9Harness(t *testing.T) *ln9Harness {
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
	workers, err := worker.NewStore(worker.Config{
		DB: db, Log: log, Settings: reg, Root: filepath.Join(t.TempDir(), "workers"),
	})
	if err != nil {
		t.Fatalf("worker.NewStore: %v", err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	h := &ln9Harness{}
	root := t.TempDir()
	sk, err := stage.New(stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Ledger: led, Settings: reg,
		Workers:   workers,
		Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic,
		Adapters: map[string]adapters.Adapter{
			adapters.SubstrateClaudeCLI: &claudecli.Adapter{
				Binary: self, HookCmd: "/opt/sinet/bin/sinet engine-hook", Settings: reg,
				Env:         append(os.Environ(), "SINET_STAGE_FAKE=1"),
				CancelGrace: 500 * time.Millisecond,
			},
			adapters.SubstrateOpencode: ln9Adapter{adapters.SubstrateOpencode, &h.seen, &h.mu},
		},
		CommissionedLanes: []string{adapters.LaneZAI},
		LaneSubstrates:    map[string]string{adapters.LaneZAI: adapters.SubstrateOpencode},
		AlternateSeats:    worker.AlternateSeatsFor(worker.LaneSeat{Lane: adapters.LaneZAI, Model: "glm-5.3"}),
		ArtifactRoot:      filepath.Join(root, "artifacts"),
		RunRoot:           filepath.Join(root, "runs"),
		CopyAsideDir:      filepath.Join(root, "copy-aside"),
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	sk.Pipeline().Classifier = softwareClassifier{}
	priceTable := metering.NewEffectiveDatedTable("empty-v0")
	exceptions := metering.NoMeteredExceptions()
	sched, err := scheduler.New(scheduler.Config{
		DB: db, Runs: runs, Settings: reg, Dispatcher: sk,
		Receipts:     metering.NewReceipts(db, metering.NewLedger(db, priceTable, exceptions, reg), exceptions),
		LeaseTTL:     time.Minute,
		PollInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("scheduler.New: %v", err)
	}
	sk.Bind(sched)
	h.routedHarness = &routedHarness{
		harness: &harness{t: t, db: db, log: log, runs: runs, cps: cps, led: led,
			sk: sk, sched: sched, sur: sk.Surface(), artifactRoot: filepath.Join(root, "artifacts")},
		workers: workers,
	}
	return h
}

// ln9SeedHelperWorker approves a template whose trigger matches the helper
// brief below, so helper selection takes the ROUTER path rather than the
// degraded generalist one. Both paths have to honor the pin, and a test that
// only ever drove one of them would leave the other free to seat any lane it
// liked.
func (h *ln9Harness) ln9SeedHelperWorker(owner string) {
	h.t.Helper()
	ctx := context.Background()
	src := `---
name: archive-trawler
description: Trawls note archives for prior entries
kind: agentic
domain: software
selectors:
  triggers: [notes archive]
profile:
  duty: execution
equipment:
  tools: [Read]
---
Trawl the archive and report findings.
`
	_, v, err := h.workers.CreateDraft(ctx, owner, src, worker.RequestedGrants{
		Tools: []string{"Read"}, Class: "C1", Egress: worker.EgressNone,
	}, worker.Provenance{AuthorKind: "human", Origin: worker.OriginHumanWritten})
	if err != nil {
		h.t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := h.workers.RunBattery(ctx, v.ID, worker.BatteryInput{
		Actor: owner, SampleTask: "trawl a sample archive", Engine: passDry{},
		Model: "claude-haiku-4-5", EnginePin: "claude-cli@2.1.216",
	}); err != nil {
		h.t.Fatalf("RunBattery: %v", err)
	}
	if _, err := h.workers.Approve(ctx, owner, v.ID, worker.ApproveOpts{}); err != nil {
		h.t.Fatalf("Approve: %v", err)
	}
}

// ln9Walk submits a task — with or without a lane pin — and answers through to
// the open approval card, returning the task id and the open ask id.
//
// It is walkToApproval's shape with one difference: the submitted body carries
// `pinned_lane`. That difference is the packet, so it is written out rather
// than parameterised into somebody else's helper.
func (h *ln9Harness) ln9Walk(ctx context.Context, owner, pin string) (taskID, askID string) {
	h.t.Helper()
	body := `{"title":"SQLite note","text":"Write a short appreciation note about the SQLite database engine."}`
	if pin != "" {
		body = fmt.Sprintf(
			`{"title":"SQLite note","text":"Write a short appreciation note about the SQLite database engine.","pinned_lane":%q}`,
			pin)
	}
	raw, err := h.sur.Submit(ctx, owner, json.RawMessage(body))
	if err != nil {
		h.t.Fatalf("Submit(pin=%q): %v", pin, err)
	}
	v := decodeView(h.t, raw)
	taskID = v.TaskID
	if n := h.tick(ctx); n != 1 {
		h.t.Fatalf("tick dispatched %d", n)
	}
	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		h.t.Fatalf("Task: %v", err)
	}
	raw = clearFamilyGate(h.t, ctx, h.sur, owner, raw)
	v = decodeView(h.t, raw)
	raw, err = h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"force_proceed":true}`), false)
	if err != nil {
		h.t.Fatalf("Answer(force_proceed): %v", err)
	}
	v = decodeView(h.t, raw)
	if v.OpenCard.Kind != "approval" {
		h.t.Fatalf("expected approval card, got %s", raw)
	}
	return taskID, v.OpenAskID
}

// ln9Routing reads the RECORDED selection off the durable intake state — the
// same block the approval card renders and the execute dispatch consumes. It
// is read from the state rather than from an ask snapshot because the state is
// the authority: a card is a rendering of it.
func ln9Routing(t *testing.T, h *ln9Harness, taskID string) *intake.RouteBlock {
	t.Helper()
	st, err := h.sk.Pipeline().LoadState(context.Background(), taskID)
	if err != nil {
		t.Fatalf("LoadState(%s): %v", taskID, err)
	}
	if st.Routing == nil {
		t.Fatalf("task %s carries no recorded selection", taskID)
	}
	return st.Routing
}

// ── §9.1 · the pin is honored at the REAL call site, end to end ────────────

func TestLN9PinnedLaneWinsAtTheRealCallSite(t *testing.T) {
	h := newLN9Harness(t)
	ctx := context.Background()
	const owner = "u-ln9"
	insertUser(t, h.db, owner, "member")

	unpinnedTask, _ := h.ln9Walk(ctx, owner, "")
	pinnedTask, _ := h.ln9Walk(ctx, owner, adapters.LaneZAI)

	unpinned := ln9Routing(t, h, unpinnedTask)
	pinned := ln9Routing(t, h, pinnedTask)

	if pinned.Lane != adapters.LaneZAI {
		t.Fatalf("the pinned task's recorded lane is %q, want %q — the pin travelled Submit → intake state → "+
			"routeQueryFor → the router seam → selection, and something on that path dropped it.\nreason: %s",
			pinned.Lane, adapters.LaneZAI, pinned.PlainReason)
	}
	if unpinned.Lane != adapters.LaneAnthropic {
		t.Fatalf("the unpinned control routed to %q — with no gauge wired the deterministic duty-map order "+
			"stands, so the control must be the configured lane", unpinned.Lane)
	}
	if pinned.LanePin != adapters.LaneZAI {
		t.Errorf("RouteBlock.LanePin = %q, want %q — the structured member LN-10's picker binds to",
			pinned.LanePin, adapters.LaneZAI)
	}
	if unpinned.LanePin != "" {
		t.Errorf("an unpinned task's block carries LanePin=%q — the member is omitempty and must be ABSENT, so "+
			"the unpinned body is byte-identical to the one served before this packet", unpinned.LanePin)
	}

	for _, want := range []string{`"zai"`, "REPLACED", "pinned on this task"} {
		if !strings.Contains(pinned.PlainReason, want) {
			t.Errorf("the recorded reason does not say %q: %q", want, pinned.PlainReason)
		}
	}
	// R1's own claim: the reason is what makes the pin visible on every surface
	// that renders one, with no web/src change at all.
	if strings.Contains(unpinned.PlainReason, "pinned on this task") {
		t.Errorf("an unpinned decision's reason claims a pin: %q", unpinned.PlainReason)
	}
	// The WORKER axis is untouched: a lane pin freezes no worker choice.
	if pinned.Pinned {
		t.Error("a lane pin set RouteBlock.Pinned — that flag freezes the WORKER choice against a re-plan " +
			"recompute, which no lane pin asked for (§1.2, the trap)")
	}
	if pinned.OverriddenBy != "" {
		t.Errorf("a lane pin recorded an override actor %q — no card override happened", pinned.OverriddenBy)
	}
}

// ── §9.2 · the pin binds the HELPER spawn, proven by the engine reached ────

func (h *ln9Harness) ln9CoordinatorRun(ctx context.Context, taskID, owner, pin string) run.Run {
	h.t.Helper()
	// The task, its intake state (pin included — the durable form the pin
	// actually travels in) and a running coordinator run.
	_, _ = h.ln9Walk(ctx, owner, pin)
	runID := taskID + ".execute"
	if _, err := h.runs.Create(ctx, run.NewRun{
		ID: runID, UserID: owner, TaskID: taskID,
		Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic,
	}); err != nil {
		h.t.Fatalf("create coordinator run: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := h.runs.Transition(ctx, runID, st, run.TransitionOptions{
			Reason: "test admission", Actor: run.ActorPlatform}); err != nil {
			h.t.Fatalf("admit: %v", err)
		}
	}
	r, err := h.runs.Get(ctx, runID)
	if err != nil {
		h.t.Fatal(err)
	}
	return r
}

func TestLN9PinBindsTheHelperSpawn(t *testing.T) {
	for _, tc := range []struct {
		name, pin, wantEngine string
		routed                bool
	}{
		// The ROUTER path: a template matches, so selection resolves the pin.
		{"pinned, worker-backed", adapters.LaneZAI, adapters.SubstrateOpencode, true},
		{"unpinned control, worker-backed", "", adapters.SubstrateClaudeCLI, true},
		// The DEGRADED path: nothing matches, the router errors, and the
		// generalist fallback must honor the pin rather than quietly seat the
		// duty default.
		{"pinned, degraded fallback", adapters.LaneZAI, adapters.SubstrateOpencode, false},
		{"unpinned control, degraded fallback", "", adapters.SubstrateClaudeCLI, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newLN9Harness(t)
			ctx := context.Background()
			const owner = "u-ln9"
			insertUser(t, h.db, owner, "member")
			if tc.routed {
				h.ln9SeedHelperWorker(owner)
			}

			// ln9Walk creates the task; the coordinator run is seeded on the
			// SAME task id the walk produced.
			taskID, _ := h.ln9Walk(ctx, owner, tc.pin)
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

			// The helper session errors (the recording adapter starts nothing);
			// the dispatch it made on the way is the assertion.
			_, _ = h.sk.Spawn(ctx, stage.SpawnRequest{
				RunID:   runID,
				Trigger: stage.TriggerParallel,
				Reason:  "a lane-pin dispatch guard, not a real search",
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

			decided := eventsOfType(t, h.db, runID, "routing.decided")
			if len(decided) != 1 {
				t.Fatalf("routing.decided on the helper spawn = %d, want 1", len(decided))
			}
			wantLane := adapters.LaneAnthropic
			if tc.pin != "" {
				wantLane = tc.pin
			}
			if got := decided[0]["lane"]; got != wantLane {
				t.Fatalf("the helper decision seated lane %v, want %q — a comparison split between a coordinator "+
					"on one lane and its helpers on another is not a comparison (S08.8 step 5).\nreason: %v",
					got, wantLane, decided[0]["plain_reason"])
			}
			if tc.pin != "" && decided[0]["lane_pin"] != tc.pin {
				t.Errorf("the helper's routing.decided carries lane_pin=%v, want %q — the pin's bookkeeping rides "+
					"the helper's own event for free once the decision carries it (R7)", decided[0]["lane_pin"], tc.pin)
			}
			if tc.routed == (decided[0]["cause"] == "helper-spawn" && decided[0]["generalist"] == true) {
				t.Errorf("this subtest meant to drive the %v path and the decision says otherwise: %v",
					map[bool]string{true: "router", false: "degraded"}[tc.routed], decided[0])
			}

			h.mu.Lock()
			seen := append([]string(nil), h.seen...)
			h.mu.Unlock()
			reachedOpencode := len(seen) > 0
			if want := tc.wantEngine == adapters.SubstrateOpencode; reachedOpencode != want {
				t.Errorf("the commissioned lane's engine reached = %v, want %v (engines seen: %v) — the lane has "+
					"to survive to the ADAPTER LOOKUP to matter, not merely to the decision", reachedOpencode, want, seen)
			}
		})
	}
}

// ── §9.7 (wiring half) · the run row is stamped by the DISPATCH ───────────

// A guard proven by calling the verb directly is a guard whose wiring nobody is
// holding (§67 drain r2 N2), so this drives the real execute dispatch. The
// verb's own behaviour is pinned in internal/run and the receipt consequence in
// internal/metering; what this holds is that the dispatch actually calls it.
func TestLN9ExecuteDispatchStampsTheDecidedLaneOnTheRunRow(t *testing.T) {
	for _, tc := range []struct {
		name, pin, wantLane, wantSubstrate string
	}{
		{"a pinned run's row names the lane that ran", adapters.LaneZAI,
			adapters.LaneZAI, adapters.SubstrateOpencode},
		// The control that keeps the correction a correction: with nothing to
		// correct, nothing moves — the row is byte-identical to its birth stamp.
		{"an unpinned run's row is untouched", "",
			adapters.LaneAnthropic, adapters.SubstrateClaudeCLI},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newLN9Harness(t)
			ctx := context.Background()
			const owner = "u-ln9"
			insertUser(t, h.db, owner, "member")

			taskID, askID := h.ln9Walk(ctx, owner, tc.pin)
			if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"action":"approve"}`), false); err != nil {
				t.Fatalf("Answer(approve): %v", err)
			}
			execRun := taskID + ".execute"
			born, err := h.runs.Get(ctx, execRun)
			if err != nil {
				t.Fatal(err)
			}
			// launchRole stamps the PROCESS DEFAULT at creation, before routing
			// has chosen anything. That is the defect R9 corrects.
			if born.Lane != adapters.LaneAnthropic || born.Substrate != adapters.SubstrateClaudeCLI {
				t.Fatalf("the run was born with %+v, want the process default", born)
			}
			h.tick(ctx)

			after, err := h.runs.Get(ctx, execRun)
			if err != nil {
				t.Fatal(err)
			}
			if after.Lane != tc.wantLane {
				t.Errorf("runs.lane = %q after dispatch, want %q — the receipt's Lane column, /api/meters and "+
					"the run list all read this row, so a run that routed elsewhere metered as the configured "+
					"lane with nothing anywhere contradicting it (R9)", after.Lane, tc.wantLane)
			}
			if after.Substrate != tc.wantSubstrate {
				t.Errorf("runs.substrate = %q after dispatch, want %q — a row whose lane moved and whose "+
					"substrate did not is a row that contradicts itself (OQ-3)", after.Substrate, tc.wantSubstrate)
			}
			// The decision the row now agrees with is the one that was recorded.
			decided := eventsOfType(t, h.db, execRun, "routing.decided")
			if len(decided) != 1 {
				t.Fatalf("routing.decided = %d, want 1", len(decided))
			}
			if got := decided[0]["lane"]; got != tc.wantLane {
				t.Errorf("routing.decided lane = %v but the row says %q — routing_quality carries both, and "+
					"disagreement there is the defect made visible", got, after.Lane)
			}
		})
	}
}

// The pin reaches the intake router SEAM with its own field rather than by
// accident: the seam is the site §63 R1 names, and a query that arrives without
// the pin is the dropped-argument failure in its exact form.
func TestLN9IntakeRouterSeamCarriesThePin(t *testing.T) {
	h := newLN9Harness(t)
	ctx := context.Background()

	block, err := h.sk.Pipeline().Router.RouteTask(ctx, intake.RouteQuery{
		Requester: "u-ln9", TaskID: "t-ln9-seam", RunID: "t-ln9-seam.intake",
		TaskText: "run the comparison task on the named lane", Family: "write-produce",
		PinnedLane: adapters.LaneZAI,
	})
	if err != nil {
		t.Fatalf("RouteTask: %v", err)
	}
	if block.Lane != adapters.LaneZAI || block.LanePin != adapters.LaneZAI {
		t.Fatalf("the seam answered lane=%q lane_pin=%q, want %q for both.\nreason: %s",
			block.Lane, block.LanePin, adapters.LaneZAI, block.PlainReason)
	}
	unpinned, err := h.sk.Pipeline().Router.RouteTask(ctx, intake.RouteQuery{
		Requester: "u-ln9", TaskID: "t-ln9-seam-control", RunID: "t-ln9-seam-control.intake",
		TaskText: "run the comparison task on the named lane", Family: "write-produce",
	})
	if err != nil {
		t.Fatalf("RouteTask(control): %v", err)
	}
	if unpinned.LanePin != "" || unpinned.Lane != adapters.LaneAnthropic {
		t.Errorf("the unpinned control answered lane=%q lane_pin=%q", unpinned.Lane, unpinned.LanePin)
	}
}

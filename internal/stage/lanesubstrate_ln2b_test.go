package stage

// lanesubstrate_ln2b_test.go — P3-LN-2B drain r1 D4 (S03.2, S03.6, S08.8).
//
// Before this, a routing decision that seated a model on a second lane still
// dispatched to whichever substrate was the process default: the run row is
// stamped at creation, which happens before routing runs, and the decision's
// lane was dropped on the way to the adapter lookup. The run would have
// executed on the wrong engine and metered as the wrong lane.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// laneAdapter is a $0 stand-in that reports the substrate it represents.
type laneAdapter struct{ name string }

func (a laneAdapter) Substrate() string { return a.name }
func (a laneAdapter) Start(context.Context, adapters.StartRequest) (adapters.Session, error) {
	return nil, errors.New("no engine is started in the lane-substrate tests ($0)")
}
func (a laneAdapter) Resume(context.Context, adapters.ParkRecord, *adapters.Answer) (adapters.Session, error) {
	return nil, errors.New("no engine is started in the lane-substrate tests ($0)")
}

func laneSkeleton(lanes map[string]string) *Skeleton {
	return &Skeleton{cfg: Config{
		Substrate: adapters.SubstrateClaudeCLI,
		Lane:      adapters.LaneAnthropic,
		Adapters: map[string]adapters.Adapter{
			adapters.SubstrateClaudeCLI: laneAdapter{adapters.SubstrateClaudeCLI},
			adapters.SubstrateOpencode:  laneAdapter{adapters.SubstrateOpencode},
		},
		LaneSubstrates: lanes,
	}}
}

// D4 · A commissioned zai decision reaches the opencode adapter; an anthropic
// decision reaches claudecli.
func TestLaneDecidesWhichAdapterServesTheRun(t *testing.T) {
	commissioned := map[string]string{adapters.LaneZAI: adapters.SubstrateOpencode}

	for _, tc := range []struct {
		name         string
		lanes        map[string]string
		decisionLane string
		row          run.Run
		want         string
	}{
		{
			name: "a commissioned zai decision goes to opencode",
			// The row still says claude-cli, because launchRole stamped it
			// before routing ran. The decision must win.
			lanes: commissioned, decisionLane: adapters.LaneZAI,
			row:  run.Run{ID: "r1", Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic},
			want: adapters.SubstrateOpencode,
		},
		{
			name:  "an anthropic decision stays on claudecli",
			lanes: commissioned, decisionLane: adapters.LaneAnthropic,
			row:  run.Run{ID: "r2", Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic},
			want: adapters.SubstrateClaudeCLI,
		},
		{
			name: "a zai RUN row with no decision lane still goes to opencode",
			// Recovery forks and ladder successors inherit the row's lane and
			// carry no fresh decision.
			lanes: commissioned, decisionLane: "",
			row:  run.Run{ID: "r3", Substrate: adapters.SubstrateOpencode, Lane: adapters.LaneZAI},
			want: adapters.SubstrateOpencode,
		},
		{
			name: "nothing commissioned changes nothing",
			// The v0 posture: the map is empty and every dispatch takes its
			// pre-LN-2 path, even for a lane a document describes.
			lanes: nil, decisionLane: adapters.LaneZAI,
			row:  run.Run{ID: "r4", Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic},
			want: adapters.SubstrateClaudeCLI,
		},
		{
			name:  "an intake run with no substrate falls to the ceremony default",
			lanes: commissioned, decisionLane: "",
			row:  run.Run{ID: "r5"},
			want: adapters.SubstrateClaudeCLI,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := laneSkeleton(tc.lanes)
			got := s.substrateFor(tc.decisionLane, tc.row)
			if got != tc.want {
				t.Fatalf("substrate = %q, want %q", got, tc.want)
			}
			// And the decision actually reaches THAT adapter — the lookup is
			// what dispatch does, so asserting the string alone would not
			// prove the engine changed.
			adapter, ok := s.cfg.Adapters[got]
			if !ok {
				t.Fatalf("no adapter registered for %q", got)
			}
			if adapter.Substrate() != tc.want {
				t.Errorf("dispatch reached the %q adapter, want %q", adapter.Substrate(), tc.want)
			}
		})
	}
}

// D4 · A lane naming an unregistered substrate is refused at construction,
// never silently served by the default engine.
func TestLaneSubstrateMustHaveARegisteredAdapter(t *testing.T) {
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
	base := func() Config {
		return Config{
			DB: db, Log: log, Runs: run.NewStore(db, log),
			Checkpoints: gates.NewCheckpoints(db, log),
			Ledger:      ledger.NewStore(db, log), Settings: reg,
			Adapters: map[string]adapters.Adapter{
				adapters.SubstrateClaudeCLI: laneAdapter{adapters.SubstrateClaudeCLI},
			},
			ArtifactRoot: t.TempDir(), RunRoot: t.TempDir(),
		}
	}

	// The control: this config is otherwise valid.
	if _, err := New(base()); err != nil {
		t.Fatalf("the control config was rejected, so the refusal below would prove nothing: %v", err)
	}

	cfg := base()
	cfg.LaneSubstrates = map[string]string{"zai": adapters.SubstrateOpencode}
	_, err = New(cfg)
	if err == nil {
		t.Fatal("a lane pointing at an unregistered substrate was accepted — the run would have gone to the default engine")
	}
	if !strings.Contains(err.Error(), "zai") || !strings.Contains(err.Error(), adapters.SubstrateOpencode) {
		t.Errorf("the refusal does not name the lane and the substrate: %v", err)
	}
}

// ── drain r2 R1: the guard belongs at the CALL SITES ────────────────────────
//
// substrateFor was already correct and already tested. What was wrong is that
// two of the eight SessionInput construction sites never passed it the lane —
// and they were the two fed by an EXECUTION-seat decision, the only duty that
// ever seats a second lane. A test of the resolver in isolation cannot see a
// dropped argument, so these drive the real call sites.

// recordingAdapter reports which substrate dispatch actually reached. It never
// starts an engine: the lookup and the Start call are the observation, and
// failing immediately afterwards keeps the test at $0.
type recordingAdapter struct {
	name string
	seen *[]string
	mu   *sync.Mutex
}

func (a recordingAdapter) Substrate() string { return a.name }
func (a recordingAdapter) Start(context.Context, adapters.StartRequest) (adapters.Session, error) {
	a.mu.Lock()
	*a.seen = append(*a.seen, a.name)
	a.mu.Unlock()
	return nil, errors.New("recording adapter starts no engine ($0)")
}
func (a recordingAdapter) Resume(context.Context, adapters.ParkRecord, *adapters.Answer) (adapters.Session, error) {
	return nil, errors.New("recording adapter starts no engine ($0)")
}

type callSiteEnv struct {
	sk   *Skeleton
	db   *storage.DB
	log  *eventlog.Log
	runs *run.Store
	seen []string
	mu   sync.Mutex
}

func newCallSiteEnv(t *testing.T) *callSiteEnv {
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
	e := &callSiteEnv{db: db, log: log, runs: runs}
	root := t.TempDir()
	sk, err := New(Config{
		DB: db, Log: log, Runs: runs, Checkpoints: gates.NewCheckpoints(db, log),
		Ledger: ledger.NewStore(db, log), Settings: reg,
		Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic,
		Adapters: map[string]adapters.Adapter{
			adapters.SubstrateClaudeCLI: recordingAdapter{adapters.SubstrateClaudeCLI, &e.seen, &e.mu},
			adapters.SubstrateOpencode:  recordingAdapter{adapters.SubstrateOpencode, &e.seen, &e.mu},
		},
		// The commissioned world: zai is served by opencode.
		LaneSubstrates: map[string]string{adapters.LaneZAI: adapters.SubstrateOpencode},
		ArtifactRoot:   filepath.Join(root, "artifacts"),
		RunRoot:        filepath.Join(root, "runs"),
	})
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	e.sk = sk
	return e
}

// seedRun creates a coordinator run whose ROW says claude-cli/anthropic — the
// stamp launchRole applies at creation, before routing has run.
func (e *callSiteEnv) seedRun(t *testing.T, taskID string) run.Run {
	t.Helper()
	ctx := context.Background()
	if err := e.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES (?, 'u1', 'call-site harness', ?)`,
			taskID, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	runID := taskID + ".execute"
	if _, err := e.runs.Create(ctx, run.NewRun{
		ID: runID, UserID: "u1", TaskID: taskID,
		Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := e.runs.Transition(ctx, runID, st, run.TransitionOptions{
			Reason: "test admission", Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("admit: %v", err)
		}
	}
	r, err := e.runs.Get(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (e *callSiteEnv) reached(t *testing.T) string {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.seen) == 0 {
		t.Fatal("no adapter was reached at all — the call site never dispatched")
	}
	return e.seen[len(e.seen)-1]
}

// R1 · The HELPER spawn call site forwards the decision's lane.
func TestHelperSpawnDispatchesOnTheDecisionsLane(t *testing.T) {
	for _, tc := range []struct{ lane, want string }{
		{adapters.LaneZAI, adapters.SubstrateOpencode},
		{adapters.LaneAnthropic, adapters.SubstrateClaudeCLI},
	} {
		t.Run(tc.lane, func(t *testing.T) {
			e := newCallSiteEnv(t)
			r := e.seedRun(t, "t-helper-"+tc.lane)
			req := SpawnRequest{
				RunID:   r.ID,
				Trigger: TriggerParallel,
				Reason:  "a lane-dispatch guard, not a real search",
				Brief: HelperBrief{
					Objective:      "Trawl the notes archive for entries about SQLite.",
					OutputContract: "FINDINGS / EVIDENCE / GAPS",
					Class:          "C1",
					Tools:          []string{"read"},
				},
			}
			decision := worker.Decision{Model: "some-model", Lane: tc.lane, WindowTokens: 200000}
			// The helper session errors (the adapter starts nothing); the
			// dispatch it made on the way is the assertion.
			_, _ = e.sk.runHelper(context.Background(), r, req, decision, 1, 0)
			if got := e.reached(t); got != tc.want {
				t.Errorf("helper on lane %s reached the %q adapter, want %q — the spawn site dropped the decision's lane",
					tc.lane, got, tc.want)
			}
		})
	}
}

// R1 · The REVISE call site forwards the lane of the RECORDED S08.8 selection.
func TestReviseDispatchesOnTheRecordedSelectionsLane(t *testing.T) {
	for _, tc := range []struct{ lane, want string }{
		{adapters.LaneZAI, adapters.SubstrateOpencode},
		{adapters.LaneAnthropic, adapters.SubstrateClaudeCLI},
	} {
		t.Run(tc.lane, func(t *testing.T) {
			e := newCallSiteEnv(t)
			taskID := "t-revise-" + tc.lane
			r := e.seedRun(t, taskID)
			// The recorded execution selection this task's rework rides.
			state := fmt.Sprintf(`{"routing":{"cause":"test","model":"some-model","lane":%q,"window_tokens":200000,"plain_reason":"seeded"}}`, tc.lane)
			if _, err := e.log.Append(context.Background(), eventlog.Append{
				RunID: r.ID, Generation: r.Generation, UserID: "u1",
				Type: intake.EventState, SchemaVersion: 1,
				Payload: json.RawMessage(state),
			}); err != nil {
				t.Fatalf("seed pipeline state: %v", err)
			}

			_, _ = e.sk.engineRevise(context.Background(), verify.RetryPackage{
				Round: 1,
				Deliverable: verify.Deliverable{
					TaskID: taskID, RunID: r.ID,
				},
			})
			if got := e.reached(t); got != tc.want {
				t.Errorf("revise on lane %s reached the %q adapter, want %q — the revise site dropped the recorded selection's lane",
					tc.lane, got, tc.want)
			}
		})
	}
}

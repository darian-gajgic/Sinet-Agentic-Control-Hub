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
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
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

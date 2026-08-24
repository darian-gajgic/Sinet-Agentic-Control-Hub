package stage

// kimi_ln3_test.go — P3-LN-3 §6 spec 20 (S03.2, S03.6).
//
// A commissioned kimi decision must reach the OPENCODE adapter. The lane rides
// opencode on the Anthropic protocol, which is exactly the kind of difference
// that composes fine in one place and not another — so the assertion is driven
// through the REAL dispatch and the REAL call sites (§63 drain-r2 R1), never
// through the resolver alone.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

func TestKimiSubstrateDispatchesToOpencode(t *testing.T) {
	// Both flat lanes commissioned: kimi must not shadow zai, and neither may
	// borrow the other's engine because they happen to share a substrate name.
	commissioned := map[string]string{
		adapters.LaneKimi: adapters.SubstrateOpencode,
		adapters.LaneZAI:  adapters.SubstrateOpencode,
	}

	for _, tc := range []struct {
		name         string
		lanes        map[string]string
		decisionLane string
		row          run.Run
		want         string
	}{
		{
			name:  "a commissioned kimi decision goes to opencode",
			lanes: commissioned, decisionLane: adapters.LaneKimi,
			row:  run.Run{ID: "k1", Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic},
			want: adapters.SubstrateOpencode,
		},
		{
			name: "a kimi RUN row with no decision lane still goes to opencode",
			// Recovery forks and ladder successors inherit the row's lane.
			lanes: commissioned, decisionLane: "",
			row:  run.Run{ID: "k2", Substrate: adapters.SubstrateOpencode, Lane: adapters.LaneKimi},
			want: adapters.SubstrateOpencode,
		},
		{
			name: "kimi uncommissioned changes nothing",
			// A lane a document describes but nobody holds takes the pre-lane
			// path, exactly as zai did before its ceremony.
			lanes: nil, decisionLane: adapters.LaneKimi,
			row:  run.Run{ID: "k3", Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic},
			want: adapters.SubstrateClaudeCLI,
		},
		{
			name:  "an anthropic decision is untouched by the second flat lane",
			lanes: commissioned, decisionLane: adapters.LaneAnthropic,
			row:  run.Run{ID: "k4", Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic},
			want: adapters.SubstrateClaudeCLI,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := laneSkeleton(tc.lanes)
			got := s.substrateFor(tc.decisionLane, tc.row)
			if got != tc.want {
				t.Fatalf("substrate = %q, want %q", got, tc.want)
			}
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

// The two call sites that dropped the lane before drain r2: a helper spawn and
// a revise. Both are fed by an EXECUTION-seat decision, which is the only duty
// that ever seats a second lane.
func TestKimiHelperSpawnAndReviseDispatchOnTheLane(t *testing.T) {
	t.Run("helper spawn", func(t *testing.T) {
		e := newCallSiteEnvWith(t, map[string]string{
			adapters.LaneKimi: adapters.SubstrateOpencode,
			adapters.LaneZAI:  adapters.SubstrateOpencode,
		})
		r := e.seedRun(t, "t-helper-kimi")
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
		decision := worker.Decision{Model: "some-model", Lane: adapters.LaneKimi, WindowTokens: 200000}
		_, _ = e.sk.runHelper(context.Background(), r, req, decision, 1, 0)
		if got := e.reached(t); got != adapters.SubstrateOpencode {
			t.Errorf("a kimi helper reached the %q adapter, want %q — the spawn site dropped the decision's lane",
				got, adapters.SubstrateOpencode)
		}
	})

	t.Run("revise", func(t *testing.T) {
		e := newCallSiteEnvWith(t, map[string]string{
			adapters.LaneKimi: adapters.SubstrateOpencode,
			adapters.LaneZAI:  adapters.SubstrateOpencode,
		})
		taskID := "t-revise-kimi"
		r := e.seedRun(t, taskID)
		state := fmt.Sprintf(`{"routing":{"cause":"test","model":"k3","lane":%q,"window_tokens":200000,"plain_reason":"seeded"}}`, adapters.LaneKimi)
		if _, err := e.log.Append(context.Background(), eventlog.Append{
			RunID: r.ID, Generation: r.Generation, UserID: "u1",
			Type: intake.EventState, SchemaVersion: 1,
			Payload: json.RawMessage(state),
		}); err != nil {
			t.Fatalf("seed pipeline state: %v", err)
		}
		_, _ = e.sk.engineRevise(context.Background(), verify.RetryPackage{
			Round:       1,
			Deliverable: verify.Deliverable{TaskID: taskID, RunID: r.ID},
		})
		if got := e.reached(t); got != adapters.SubstrateOpencode {
			t.Errorf("a kimi revise reached the %q adapter, want %q — the revise site dropped the recorded selection's lane",
				got, adapters.SubstrateOpencode)
		}
	})
}

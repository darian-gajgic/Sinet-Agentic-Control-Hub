package stage

// kimicli_ln7_test.go — P3-LN-7 §10 spec T10 (S03.2, §63 D5, §65).
//
// Dispatch is proven at the REAL call sites, never at the resolver: §63 drain
// r2's lesson is that a correct function reached through a dropped argument is
// a broken feature, and helper-spawn and revise are the two sites that dropped
// the lane once already.
//
// The LN-4 guard already drives every SEEDED lane through both sites and
// asserts which engine was reached, so the fourth lane rides it the moment its
// document ships. What that guard cannot say is whether the kimi-cli case was
// exercised at all — a lane the fill silently dropped would leave it green with
// one fewer subtest. This pins that it is not vacuous, and that the two kimi
// lanes reach DIFFERENT engines despite sharing one credential and one pool.

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// runKimiCLIHelper drives the helper-spawn call site on the kimi-cli lane.
func runKimiCLIHelper(t *testing.T, e *callSiteEnv, r run.Run) {
	t.Helper()
	_, _ = e.sk.runHelper(context.Background(), r, SpawnRequest{
		RunID:   r.ID,
		Trigger: TriggerParallel,
		Reason:  "a lane-dispatch guard, not a real search",
		Brief: HelperBrief{
			Objective:      "Trawl the notes archive for entries about SQLite.",
			OutputContract: "FINDINGS / EVIDENCE / GAPS",
			Class:          "C1",
			Tools:          []string{"read"},
		},
	}, worker.Decision{Model: "k3", Lane: adapters.LaneKimiCLI, WindowTokens: 200000}, 1, 0)
}

// runKimiCLIRevise drives the revise call site on the kimi-cli lane.
func runKimiCLIRevise(t *testing.T, e *callSiteEnv, taskID string, r run.Run) {
	t.Helper()
	state := fmt.Sprintf(
		`{"routing":{"cause":"test","model":"k3","lane":%q,"window_tokens":200000,"plain_reason":"seeded"}}`,
		adapters.LaneKimiCLI)
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
}

func TestKimiCLIDispatchAtRealCallSites(t *testing.T) {
	stateDir := t.TempDir()
	lanes, err := opencode.SeedLaneConfigs()
	if err != nil {
		t.Fatalf("SeedLaneConfigs: %v", err)
	}
	for _, l := range lanes {
		ln4Place(t, stateDir, "me", l.Credential.Profile, broker.KindEngineCred)
	}
	lanes, commissioned := ln4Commissioned(t, stateDir)
	subs := opencode.CommissionedSubstrates(lanes, commissioned)

	// The fill must actually carry the lane, or every assertion below is about
	// an empty map.
	if subs[adapters.LaneKimiCLI] != adapters.SubstrateKimiCLI {
		t.Fatalf("laneSubstrates[%s] = %q, want %q — the production fill dropped the lane, and the LN-4 dispatch "+
			"guard would then be green with one fewer case rather than red",
			adapters.LaneKimiCLI, subs[adapters.LaneKimiCLI], adapters.SubstrateKimiCLI)
	}
	// ONE credential, ONE pool, TWO engines. If these ever collapse to one
	// substrate the operator's comparison is measuring the same path twice.
	if subs[adapters.LaneKimi] == subs[adapters.LaneKimiCLI] {
		t.Errorf("both kimi lanes map to substrate %q — they share a membership and a quota pool, but the whole "+
			"point of the second lane is that it runs on a different engine", subs[adapters.LaneKimi])
	}

	for _, site := range []string{"helper", "revise"} {
		t.Run(site, func(t *testing.T) {
			e := newCallSiteEnvWith(t, subs)
			taskID := "t-ln7-" + site
			r := e.seedRun(t, taskID)
			switch site {
			case "helper":
				runKimiCLIHelper(t, e, r)
			case "revise":
				runKimiCLIRevise(t, e, taskID, r)
			}
			if got := e.reached(t); got != adapters.SubstrateKimiCLI {
				t.Errorf("a %s on lane %s reached the %q adapter, want %q — a decision seated on this lane and "+
					"dispatched elsewhere would execute on the wrong engine and meter against the wrong lane, "+
					"with nothing anywhere contradicting it (S03.2)",
					site, adapters.LaneKimiCLI, got, adapters.SubstrateKimiCLI)
			}
		})
	}

	// Mutation check: neuter the fill and the case collapses into the control's
	// claude-cli path, which is what makes the assertion above mean something.
	e := newCallSiteEnvWith(t, nil)
	r := e.seedRun(t, "t-ln7-mutation")
	runKimiCLIHelper(t, e, r)
	if got := e.reached(t); got != adapters.SubstrateClaudeCLI {
		t.Errorf("with the fill neutered the helper reached %q, want the unchanged %q — if the commissioned and "+
			"uncommissioned worlds do not differ here, the guard above proves nothing",
			got, adapters.SubstrateClaudeCLI)
	}

	// And the seeded corpus really does carry four lanes on three substrates.
	seen := map[string]bool{}
	for _, l := range lanes {
		seen[l.Substrate] = true
	}
	if !reflect.DeepEqual(seen, map[string]bool{adapters.SubstrateOpencode: true, adapters.SubstrateKimiCLI: true}) {
		t.Errorf("the shipped lane corpus spans substrates %v, want opencode + kimi-cli (the anthropic lane's "+
			"substrate is the ceremony default and carries no document)", seen)
	}
}

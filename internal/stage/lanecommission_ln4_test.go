package stage

// lanecommission_ln4_test.go — P3-LN-4 / T9 (S03.2, S08.8, CONVENTIONS §63
// drain r2 R1).
//
// LN-2B's guards proved that a lane→substrate map, once it exists, reaches the
// real dispatch sites. They could not prove the map is ever non-empty, because
// at v0 nothing filled it: every one of them handed in a map written by the
// test. LN-4 fills it from what an operator placed, so the question this file
// asks is the one that was unaskable — does a PLACED credential reach the right
// engine?
//
// "A correct function reached through a dropped argument is a broken feature"
// (§63 r2 R1), so the assertion is driven through the helper-spawn and revise
// call sites against recording adapters, never through the resolver alone.
//
// $0: no engine starts, no provider is dialled, and every credential record is
// a fake with an empty ciphertext in a t.TempDir() store.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// ln4Place writes one credential record by hand, at the path the broker daemon
// owns. No store is opened, so no master key is created and nothing here holds
// credential material.
func ln4Place(t *testing.T, stateDir, who, profile, kind string) {
	t.Helper()
	dir := filepath.Join(broker.StoreRoot(stateDir), who)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("store dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, profile+".cred"),
		[]byte(`{"kind":"`+kind+`","nonce":"","ct":""}`), 0o600); err != nil {
		t.Fatalf("write record: %v", err)
	}
}

// ln4Commissioned runs the PRODUCTION fill over a state dir: the secret-free
// broker placement read, then the lane documents' own provider entries. It is
// the chain the control plane composes at startup, and it is why reverting the
// fill fails every assertion below rather than none of them.
func ln4Commissioned(t *testing.T, stateDir string) ([]opencode.LaneConfig, map[string]opencode.ProviderConfig) {
	t.Helper()
	lanes, err := opencode.SeedLaneConfigs()
	if err != nil {
		t.Fatalf("SeedLaneConfigs: %v", err)
	}
	root := broker.StoreRoot(stateDir)
	people, err := broker.StorePeople(root)
	if err != nil {
		t.Fatalf("StorePeople: %v", err)
	}
	placed := map[string]map[string]bool{}
	for _, who := range people {
		creds, err := broker.PlacedEngineCreds(root, who)
		if err != nil {
			t.Fatalf("PlacedEngineCreds(%q): %v", who, err)
		}
		placed[who] = creds
	}
	return lanes, opencode.Commission(lanes, placed)
}

// ln4Substrates is the lane→substrate input stage.Config takes, derived from
// the commissioned map exactly as internal/shell's laneSubstrates derives it.
// It is restated here rather than imported because internal/stage cannot import
// internal/shell — shell composes the stage, so the dependency only runs one
// way. What matters for this test is that the FILL above is production.
func ln4Substrates(lanes []opencode.LaneConfig, commissioned map[string]opencode.ProviderConfig) map[string]string {
	out := map[string]string{}
	for _, l := range lanes {
		for _, entries := range commissioned {
			if _, held := entries[l.ProviderID]; held && l.Substrate != "" {
				out[l.Lane] = l.Substrate
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// T9 · a placed credential produces a lane the REAL call sites dispatch to.
func TestLN4CommissionedLaneIsDispatchedThroughTheRealCallSites(t *testing.T) {
	stateDir := t.TempDir()
	lanes, err := opencode.SeedLaneConfigs()
	if err != nil {
		t.Fatalf("SeedLaneConfigs: %v", err)
	}
	for _, l := range lanes {
		ln4Place(t, stateDir, "me", l.Credential.Profile, broker.KindEngineCred)
	}
	lanes, commissioned := ln4Commissioned(t, stateDir)
	subs := ln4Substrates(lanes, commissioned)
	if len(subs) != len(lanes) {
		t.Fatalf("the fill mapped %v from %d placed credentials — the dispatch assertions below would be "+
			"testing an empty map", subs, len(lanes))
	}

	cases := []struct{ lane, want string }{{adapters.LaneAnthropic, adapters.SubstrateClaudeCLI}}
	for _, l := range lanes {
		cases = append(cases, struct{ lane, want string }{l.Lane, l.Substrate})
	}

	for _, tc := range cases {
		t.Run("helper/"+tc.lane, func(t *testing.T) {
			e := newCallSiteEnvWith(t, subs)
			r := e.seedRun(t, "t-ln4-helper-"+tc.lane)
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
			}, worker.Decision{Model: "some-model", Lane: tc.lane, WindowTokens: 200000}, 1, 0)
			if got := e.reached(t); got != tc.want {
				t.Errorf("a helper on the COMMISSIONED lane %s reached the %q adapter, want %q — a placed "+
					"credential that routing seats and dispatch sends elsewhere executes on the wrong "+
					"engine and meters against the wrong lane (S03.2)", tc.lane, got, tc.want)
			}
		})

		t.Run("revise/"+tc.lane, func(t *testing.T) {
			e := newCallSiteEnvWith(t, subs)
			taskID := "t-ln4-revise-" + tc.lane
			r := e.seedRun(t, taskID)
			state := fmt.Sprintf(
				`{"routing":{"cause":"test","model":"some-model","lane":%q,"window_tokens":200000,"plain_reason":"seeded"}}`,
				tc.lane)
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
			if got := e.reached(t); got != tc.want {
				t.Errorf("a revise on the COMMISSIONED lane %s reached the %q adapter, want %q", tc.lane, got, tc.want)
			}
		})
	}
}

// T9 (the control) · with nothing placed the same call sites take their exact
// pre-LN-4 path. This is the mutation verification's other end: if the fill is
// reverted, the test above collapses into this one.
func TestLN4NothingPlacedDispatchesExactlyAsBefore(t *testing.T) {
	lanes, commissioned := ln4Commissioned(t, t.TempDir())
	if len(commissioned) != 0 {
		t.Fatalf("an empty state dir commissioned %v", commissioned)
	}
	subs := ln4Substrates(lanes, commissioned)
	if subs != nil {
		t.Fatalf("laneSubstrates = %v with nothing placed, want nil", subs)
	}
	for _, l := range lanes {
		e := newCallSiteEnvWith(t, subs)
		r := e.seedRun(t, "t-ln4-inert-"+l.Lane)
		_, _ = e.sk.runHelper(context.Background(), r, SpawnRequest{
			RunID:   r.ID,
			Trigger: TriggerParallel,
			Reason:  "the uncommissioned control",
			Brief: HelperBrief{
				Objective:      "Trawl the notes archive for entries about SQLite.",
				OutputContract: "FINDINGS / EVIDENCE / GAPS",
				Class:          "C1",
				Tools:          []string{"read"},
			},
		}, worker.Decision{Model: "some-model", Lane: l.Lane, WindowTokens: 200000}, 1, 0)
		if got := e.reached(t); got != adapters.SubstrateClaudeCLI {
			t.Errorf("with nothing placed, lane %s reached %q — an uncommissioned lane must take the "+
				"ceremony default, unchanged", l.Lane, got)
		}
	}
}

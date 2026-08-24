package worker

// lanecommission_ln4_test.go — P3-LN-4 drain r1 / D7 (S08.8 steps 1-3, S03.6).
//
// In-package because resolveSeat is unexported, following routing_local_test.go
// and zai_ln2b_test.go.
//
// Those earlier guards proved a zai seat resolves under coverage — from
// literals, deliberately, because seats are DATA the composition root reads out
// of a lane document. What nothing proved is the join: that the coverage and the
// seat a control plane actually derives FROM A PLACED CREDENTIAL are the ones
// selection accepts. Each half was tested against a hand-written stand-in for
// the other, which is the §63 r2 blind spot in its selection form.
//
// So this drives the production chain end to end: a record on disk → the
// secret-free placement read → the fill → the coverage and seat derivations →
// resolveSeat.
//
// $0: the credential record is a fake with an EMPTY ciphertext in t.TempDir(),
// nothing decrypts it, and no engine or provider is touched.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
)

// ln4Fill runs the production commissioning chain over a state dir and returns
// the routing inputs a control plane composes from it.
func ln4Fill(t *testing.T, stateDir string) (Coverage, AlternateSeats) {
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
		profiles, err := broker.PlacedEngineCreds(root, who)
		if err != nil {
			t.Fatalf("PlacedEngineCreds(%q): %v", who, err)
		}
		placed[who] = profiles
	}
	commissioned := opencode.Commission(lanes, placed)

	var seats []LaneSeat
	for _, s := range opencode.CommissionedSeats(lanes, commissioned) {
		seats = append(seats, LaneSeat{Lane: s.Lane, Model: s.Model})
	}
	return Coverage{FlatRateLanes: opencode.CommissionedLanes(lanes, commissioned)}, AlternateSeatsFor(seats...)
}

func ln4PlaceCred(t *testing.T, stateDir, who, profile string) {
	t.Helper()
	dir := filepath.Join(broker.StoreRoot(stateDir), who)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("store dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, profile+".cred"),
		[]byte(`{"kind":"engine-cred","nonce":"","ct":""}`), 0o600); err != nil {
		t.Fatalf("write record: %v", err)
	}
}

func ln4LaneDoc(t *testing.T, name string) opencode.LaneConfig {
	t.Helper()
	lanes, err := opencode.SeedLaneConfigs()
	if err != nil {
		t.Fatalf("SeedLaneConfigs: %v", err)
	}
	for _, l := range lanes {
		if l.Lane == name {
			return l
		}
	}
	t.Fatalf("no seed document for lane %q", name)
	return opencode.LaneConfig{}
}

// D7 · a PLACED credential is what routing selects on.
func TestLN4PlacedCredentialIsSelectedByRouting(t *testing.T) {
	ctx := context.Background()
	zai := ln4LaneDoc(t, "zai")

	stateDir := t.TempDir()
	ln4PlaceCred(t, stateDir, "me", zai.Credential.Profile)
	coverage, alternates := ln4Fill(t, stateDir)

	if len(coverage.FlatRateLanes) != 1 || coverage.FlatRateLanes[0] != "zai" {
		t.Fatalf("coverage from the fill = %v, want [zai] — the rest of this test would prove nothing",
			coverage.FlatRateLanes)
	}

	r := &Router{DutyMap: DefaultDutyMap(), Alternates: alternates, Coverage: coverage}
	seat, _, reason, gap, err := r.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		t.Fatalf("resolveSeat: %v", err)
	}
	if gap != "" {
		t.Fatalf("a lane whose credential is placed produced the 2.7 subscription-gap advice: %q", gap)
	}
	if seat.Lane != "zai" {
		t.Errorf("selection resolved to lane %q, want zai. Placement is what makes a lane SELECTABLE "+
			"(S08.8 step 3): the credential on disk, the fill, the coverage and the seat are one chain, and "+
			"a break anywhere in it leaves a commissioned lane that routing never offers.", seat.Lane)
	}
	if seat.Model != zai.DefaultModel {
		t.Errorf("seat model = %q, want the document's own default %q — no model id is a constant in this package",
			seat.Model, zai.DefaultModel)
	}
	if seat.WindowTokens <= 0 {
		t.Error("the selected seat carries no context window")
	}
	if !strings.Contains(strings.ToLower(reason), "zai") {
		t.Errorf("the plain reason does not name the lane it chose: %q", reason)
	}

	// Duties nobody has measured a second-lane model for gain nothing.
	for _, duty := range []string{DutyPlanning, DutyJudge} {
		s, _, _, _, err := r.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: duty})
		if err != nil {
			t.Fatalf("resolveSeat(%s): %v", duty, err)
		}
		if s.Lane == "zai" {
			t.Errorf("duty %q was seated on zai by a placed credential — no zai model has been measured "+
				"against the B3/S07.5 bars (CONVENTIONS §63)", duty)
		}
	}
}

// D7 (the control) · with nothing placed, selection takes its unchanged path:
// no coverage, no alternates, and the 2.7 advice in its existing words.
func TestLN4NothingPlacedIsNotSelectable(t *testing.T) {
	ctx := context.Background()
	coverage, alternates := ln4Fill(t, t.TempDir())
	if len(coverage.FlatRateLanes) != 0 || len(alternates) != 0 {
		t.Fatalf("an empty state dir produced coverage %v and seats %v", coverage.FlatRateLanes, alternates)
	}
	r := &Router{DutyMap: DefaultDutyMap(), Alternates: alternates, Coverage: coverage}
	_, _, _, gap, err := r.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		t.Fatalf("resolveSeat: %v", err)
	}
	if !strings.Contains(gap, "Subscription gap (2.7)") {
		t.Errorf("gap advice = %q, want the unchanged 2.7 leg", gap)
	}
}

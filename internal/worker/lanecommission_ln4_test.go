package worker

// lanecommission_ln4_test.go — P3-LN-4 / D7, drain r2 / R2+R3 (S08.8 steps 1-3,
// S03.6, S10.4, D5).
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
// resolveSeat, with the Coverage SHAPE production composes (skeleton.go: the
// configured lane, then the commissioned ones). That shape is why the first
// version of this test proved less than it claimed: with zai as the only flat
// lane, "selection resolved to zai" was forced, and the two-lane path S08.8
// opens when a second lane is commissioned went untested.
//
// $0: the credential record is a fake with an EMPTY ciphertext in t.TempDir(),
// nothing decrypts it, and no engine or provider is touched.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
)

// ln4Fill runs the production commissioning chain over a state dir and returns
// the routing inputs a control plane composes from it.
//
// Coverage mirrors what stage.New composes: the CONFIGURED lane first, then the
// commissioned ones. A commissioned lane is an ADDITION to the lane the
// platform already runs on, never a replacement, and a test that forgets the
// configured lane tests a single-lane world that production never has.
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
			// Production WARNs and carries on: one unreadable store directory
			// must not take the whole household's commissioning down with it.
			// The helper mirrors that rather than failing, so this harness
			// behaves like the thing it is standing in for (drain r2 R3).
			t.Logf("PlacedEngineCreds(%q): %v — that person stays uncommissioned, as production would have it", who, err)
			continue
		}
		placed[who] = profiles
	}
	commissioned := opencode.Commission(lanes, placed)

	var seats []LaneSeat
	for _, s := range opencode.CommissionedSeats(lanes, commissioned) {
		seats = append(seats, LaneSeat{Lane: s.Lane, Model: s.Model})
	}
	coverage := Coverage{FlatRateLanes: append(
		[]string{adapters.LaneAnthropic}, opencode.CommissionedLanes(lanes, commissioned)...)}
	return coverage, AlternateSeatsFor(seats...)
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

// D7 · a PLACED credential is what routing selects on — and with the Coverage
// shape production composes, that means it joins a two-lane choice rather than
// winning by being the only candidate.
func TestLN4PlacedCredentialIsSelectedByRouting(t *testing.T) {
	ctx := context.Background()
	zai := ln4LaneDoc(t, "zai")

	stateDir := t.TempDir()
	ln4PlaceCred(t, stateDir, "me", zai.Credential.Profile)
	coverage, alternates := ln4Fill(t, stateDir)

	// The fill put the placed lane BESIDE the configured one.
	if want := []string{adapters.LaneAnthropic, "zai"}; !ln4SameLanes(coverage.FlatRateLanes, want) {
		t.Fatalf("coverage from the fill = %v, want %v (the configured lane, then what is commissioned) — "+
			"the rest of this test would be about a world production never has", coverage.FlatRateLanes, want)
	}
	seats := alternates[DutyExecution]
	if len(seats) != 1 || seats[0].Lane != "zai" || seats[0].Model != zai.DefaultModel {
		t.Fatalf("execution alternates from the fill = %+v, want the zai document's own default model %q",
			seats, zai.DefaultModel)
	}

	// ── the v0 posture: two covered lanes, no comparable reading ────────────
	//
	// This is what a control plane does TODAY. The deterministic duty-map order
	// stands and says so, which is the LN-2B D3 rule: ordering two lanes by raw
	// consumption compares a token against a credit and hands every dispatch to
	// whichever lane was commissioned most recently, forever.
	r := &Router{DutyMap: DefaultDutyMap(), Alternates: alternates, Coverage: coverage}
	seat, _, reason, gap, err := r.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		t.Fatalf("resolveSeat: %v", err)
	}
	if gap != "" {
		t.Fatalf("a covered duty produced the 2.7 subscription-gap advice: %q", gap)
	}
	if seat.Lane != adapters.LaneAnthropic {
		t.Errorf("with no comparable reading selection chose %q, want the deterministic duty-map order "+
			"(anthropic). Forcing the commissioned lane to win here would be inventing a rule S10.4 "+
			"does not have.", seat.Lane)
	}
	if !strings.Contains(reason, "2 covered lanes") {
		t.Errorf("the reason does not record that TWO lanes covered this duty: %q\n"+
			"That number is the observable proof the placed credential entered selection at all.", reason)
	}
	if !strings.Contains(reason, "configured order") {
		t.Errorf("the reason does not state why the order stood: %q", reason)
	}

	// ── mixed applicability: reachable today, and it must not guess ─────────
	//
	// An operator can declare an automation budget on the anthropic lane; the
	// zai lane carries a PLAN document and no plan-budget surface exists yet
	// (13.4), so its reading stays inapplicable whatever is declared — proven
	// against the production adapter in internal/shell's
	// TestLN4PlanDocumentedLaneHasNoComparablePressureAtV0. One comparable
	// ratio and one absent one is not a comparison.
	mixed := &Router{DutyMap: DefaultDutyMap(), Alternates: alternates, Coverage: coverage,
		Pressure: fixedPressure{adapters.LaneAnthropic: 0.91, "zai": -1}}
	seat, _, reason, _, err = mixed.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		t.Fatalf("resolveSeat(mixed): %v", err)
	}
	if seat.Lane != adapters.LaneAnthropic {
		t.Errorf("with one lane comparable and one not, selection chose %q — a lane with no declared budget "+
			"has no ratio, and preferring it anyway would be routing on a number that does not exist", seat.Lane)
	}
	if !strings.Contains(reason, "no declared automation budget") || !strings.Contains(reason, "zai") {
		t.Errorf("the reason does not name the lane that could not be compared: %q", reason)
	}

	// ── both comparable: the choice moves, in BOTH directions ───────────────
	//
	// This is the S08.8 flat-lane rule the second lane makes reachable. The
	// readings are supplied rather than gauged because production cannot yet
	// produce an applicable one for a plan-documented lane (above); what is
	// under test here is that selection, over the REAL fill's coverage and the
	// REAL document's seat, follows the gauge in whichever direction it points.
	for _, tc := range []struct {
		name     string
		pressure fixedPressure
		wantLane string
	}{
		{"the commissioned lane is the less consumed one", fixedPressure{adapters.LaneAnthropic: 0.91, "zai": 0.10}, "zai"},
		{"the configured lane is the less consumed one", fixedPressure{adapters.LaneAnthropic: 0.10, "zai": 0.91}, adapters.LaneAnthropic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Router{DutyMap: DefaultDutyMap(), Alternates: alternates, Coverage: coverage, Pressure: tc.pressure}
			seat, _, reason, gap, err := r.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
			if err != nil || gap != "" {
				t.Fatalf("resolveSeat: err=%v gap=%q", err, gap)
			}
			if seat.Lane != tc.wantLane {
				t.Errorf("selection chose lane %q, want %q — between covered flat lanes the ordering input "+
					"is consumption pressure (S08.8; D5: never dollars)", seat.Lane, tc.wantLane)
			}
			if tc.wantLane == "zai" && seat.Model != zai.DefaultModel {
				t.Errorf("the commissioned lane was chosen but seated on %q, want the document's own default "+
					"model %q — no model id is a constant in this package", seat.Model, zai.DefaultModel)
			}
			if !strings.Contains(reason, "by how much of each is left") {
				t.Errorf("the reason does not record what the choice was made on: %q", reason)
			}
			if seat.WindowTokens <= 0 {
				t.Error("the selected seat carries no context window")
			}
		})
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

func ln4SameLanes(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// D7 (the control) · with nothing placed there is ONE flat lane, selection takes
// its unchanged single-lane path, and no flat-lane note is invented.
func TestLN4NothingPlacedIsNotSelectable(t *testing.T) {
	ctx := context.Background()
	coverage, alternates := ln4Fill(t, t.TempDir())
	if !ln4SameLanes(coverage.FlatRateLanes, []string{adapters.LaneAnthropic}) {
		t.Fatalf("an empty state dir produced coverage %v, want just the configured lane", coverage.FlatRateLanes)
	}
	if len(alternates) != 0 {
		t.Fatalf("an empty state dir produced alternate seats %v", alternates)
	}
	r := &Router{DutyMap: DefaultDutyMap(), Alternates: alternates, Coverage: coverage,
		Pressure: fixedPressure{adapters.LaneAnthropic: 0.42}}
	seat, _, reason, gap, err := r.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil || gap != "" {
		t.Fatalf("resolveSeat: err=%v gap=%q", err, gap)
	}
	if seat.Lane != adapters.LaneAnthropic {
		t.Errorf("selection resolved to %q with nothing commissioned, want the configured lane unchanged", seat.Lane)
	}
	if strings.Contains(reason, "flat-rate lanes cover this duty") || strings.Contains(reason, "consumption pressure") {
		t.Errorf("the single-lane world grew a flat-lane comparison it has nothing to compare: %q", reason)
	}

	// And a person holding no coverage at all still gets the unchanged 2.7 leg.
	bare := &Router{DutyMap: DefaultDutyMap(), Alternates: alternates, Coverage: Coverage{}}
	_, _, _, gap, err = bare.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		t.Fatalf("resolveSeat(no coverage): %v", err)
	}
	if !strings.Contains(gap, "Not covered by a subscription") {
		t.Errorf("gap advice = %q, want the unchanged 2.7 leg", gap)
	}
}

package opencode

// commission_ln4_test.go — P3-LN-4 drain r1 / D6+F11 (S03.6, S11.5, S08.8).
//
// The fill and the three derivations it feeds live here, beside the documents
// they read, because internal/stage's dispatch guards must reach the SAME
// functions the composition root uses and cannot import internal/shell. A
// function that only its consumers' packages test is one whose own package
// stays green while it is broken, so it is tested here too.
//
// $0: placement arrives as data — a person-to-profile map — and no store, no
// broker and no credential material is anywhere near this file.

import (
	"reflect"
	"testing"
)

func ln4SeedLanes(t *testing.T) []LaneConfig {
	t.Helper()
	lanes, err := SeedLaneConfigs()
	if err != nil {
		t.Fatalf("SeedLaneConfigs: %v", err)
	}
	if len(lanes) < 2 {
		t.Fatalf("the platform ships %d lane documents — these assertions need at least two", len(lanes))
	}
	return lanes
}

func ln4LaneNamed(t *testing.T, lanes []LaneConfig, name string) LaneConfig {
	t.Helper()
	for _, l := range lanes {
		if l.Lane == name {
			return l
		}
	}
	t.Fatalf("no seed document for lane %q", name)
	return LaneConfig{}
}

// D6/F11 · Commission itself, in its own package.
func TestLN4CommissionBuildsEntriesFromPlacement(t *testing.T) {
	lanes := ln4SeedLanes(t)
	zai := ln4LaneNamed(t, lanes, "zai")
	kimi := ln4LaneNamed(t, lanes, "kimi")

	got := Commission(lanes, map[string]map[string]bool{
		"me":  {zai.Credential.Profile: true},
		"you": {zai.Credential.Profile: true, kimi.Credential.Profile: true},
		// A person whose store holds nothing this platform ships.
		"nobody": {"some-other-key": true},
	})

	if len(got) != 2 {
		t.Fatalf("commissioned %d people (%v), want the two who placed a lane's credential", len(got), got)
	}
	if _, ok := got["nobody"]; ok {
		t.Error("a person holding only unrelated credentials was commissioned")
	}
	if len(got["me"]) != 1 || !reflect.DeepEqual(got["me"][zai.ProviderID], zai.ProviderEntry()) {
		t.Errorf("me = %v, want exactly the zai document's own entry", got["me"])
	}
	// THREE entries from TWO placed profiles: since P3-LN-7 the kimi-code
	// profile serves BOTH kimi lanes, because one membership is one key. That
	// is the shape the operator's comparison rests on — placing one credential
	// must light up both paths, or the comparison cannot be run at all.
	if len(got["you"]) != 3 {
		t.Errorf("you = %v, want three entries: the zai lane plus BOTH lanes the kimi-code profile serves", got["you"])
	}

	if empty := Commission(lanes, nil); len(empty) != 0 {
		t.Errorf("nothing placed commissioned %v", empty)
	}
}

// D6/F11 · a document that cannot carry a credential never commissions,
// whatever is placed — the R6 conjunction, at its single definition.
func TestLN4CommissionRefusesUncommissionableDocuments(t *testing.T) {
	for _, tc := range []struct {
		name string
		lane LaneConfig
	}{
		{"no auth profile", LaneConfig{Lane: "a", ProviderID: "a-p",
			Credential: LaneCredential{EnvVar: "A_KEY"}}},
		{"no environment variable", LaneConfig{Lane: "b", ProviderID: "b-p",
			Credential: LaneCredential{Profile: "b-profile"}}},
		{"neither", LaneConfig{Lane: "c", ProviderID: "c-p"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.lane.Commissionable() {
				t.Fatalf("%+v reports itself commissionable", tc.lane.Credential)
			}
			// Placed under every name it could plausibly be placed under.
			placed := map[string]map[string]bool{"me": {
				tc.lane.Credential.Profile: true, tc.lane.Lane: true, tc.lane.ProviderID: true, "": true,
			}}
			if got := Commission([]LaneConfig{tc.lane}, placed); len(got) != 0 {
				t.Errorf("commissioned %v — a lane with no credential path would be seated by routing and "+
					"then authenticate as nobody (S11.5)", got)
			}
		})
	}

	// The control: every SHIPPED document is commissionable, so the refusals
	// above are about the documents and not about the predicate refusing all.
	for _, l := range ln4SeedLanes(t) {
		if !l.Commissionable() {
			t.Errorf("shipped lane %q is not commissionable (profile=%q env_var=%q)",
				l.Lane, l.Credential.Profile, l.Credential.EnvVar)
		}
	}
}

// D6 · the three derivations the control plane and the dispatch guards share.
func TestLN4CommissionedDerivations(t *testing.T) {
	lanes := ln4SeedLanes(t)
	zai := ln4LaneNamed(t, lanes, "zai")
	kimi := ln4LaneNamed(t, lanes, "kimi")
	one := Commission(lanes, map[string]map[string]bool{"me": {zai.Credential.Profile: true}})
	two := Commission(lanes, map[string]map[string]bool{
		"me":  {zai.Credential.Profile: true},
		"you": {kimi.Credential.Profile: true},
	})

	if got := CommissionedLanes(lanes, one); !reflect.DeepEqual(got, []string{"zai"}) {
		t.Errorf("CommissionedLanes = %v, want [zai]", got)
	}
	// The UNION across people, sorted — one Router serves the whole household.
	if got := CommissionedLanes(lanes, two); !reflect.DeepEqual(got, []string{"kimi", "kimi-cli", "zai"}) {
		t.Errorf("CommissionedLanes = %v, want [kimi kimi-cli zai] — coverage is the union across people, sorted, "+
			"and one placed kimi-code credential commissions both kimi lanes", got)
	}
	if got := CommissionedLanes(lanes, nil); len(got) != 0 {
		t.Errorf("CommissionedLanes(nothing) = %v, want none", got)
	}

	if got := CommissionedSubstrates(lanes, one); !reflect.DeepEqual(got, map[string]string{"zai": zai.Substrate}) {
		t.Errorf("CommissionedSubstrates = %v, want zai on its document's own substrate — without it a "+
			"zai-seated decision executes on the Anthropic CLI and meters as anthropic", got)
	}
	if got := CommissionedSubstrates(lanes, nil); got != nil {
		t.Errorf("CommissionedSubstrates(nothing) = %v, want nil", got)
	}

	if got := CommissionedSeats(lanes, one); !reflect.DeepEqual(got, []CommissionedSeat{{Lane: "zai", Model: zai.DefaultModel}}) {
		t.Errorf("CommissionedSeats = %v, want the zai document's own default model %q", got, zai.DefaultModel)
	}
	if got := CommissionedSeats(lanes, two); len(got) != 3 {
		t.Errorf("CommissionedSeats = %v, want one seat per commissioned lane (three, since the kimi-code profile "+
			"commissions two of them)", got)
	}
	if got := CommissionedSeats(lanes, nil); len(got) != 0 {
		t.Errorf("CommissionedSeats(nothing) = %v, want none", got)
	}

	// A commissioned lane whose document ships no default model has no seat —
	// a lane may ship without an execution seat, and that is not a failure.
	seatless := LaneConfig{Lane: "seatless", ProviderID: "seatless-p", Substrate: "opencode",
		Credential: LaneCredential{Profile: "seatless-profile", EnvVar: "SEATLESS_KEY"}}
	set := append(append([]LaneConfig(nil), lanes...), seatless)
	held := Commission(set, map[string]map[string]bool{"me": {seatless.Credential.Profile: true}})
	if got := CommissionedLanes(set, held); !reflect.DeepEqual(got, []string{"seatless"}) {
		t.Fatalf("CommissionedLanes = %v, want [seatless]", got)
	}
	if got := CommissionedSeats(set, held); len(got) != 0 {
		t.Errorf("CommissionedSeats = %v, want none for a document shipping no default_model", got)
	}
	if got := CommissionedSubstrates(set, held); got["seatless"] != "opencode" {
		t.Errorf("CommissionedSubstrates = %v — a seatless lane still names the engine that serves it", got)
	}
}

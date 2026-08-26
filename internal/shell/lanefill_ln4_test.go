package shell

// lanefill_ln4_test.go — P3-LN-4 / T5, T6, T8, T10, T12 (S03.6, S11.5, S08.8).
//
// The packet's headline is "a placed key = a routable lane". These tests hold
// the FILL to that: what an operator placed decides the map, the map decides
// coverage, the lane→substrate mapping, the execution seats and who gets a
// credential injector — and a host with nothing placed is byte-identical to
// the world before this packet.
//
// $0, and no secret anywhere: every record is written by hand into a
// t.TempDir() store with an EMPTY ciphertext. The presence read never
// decrypts, so a fake record with no secret in it is the whole input.

import (
	"encoding/json"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

func ln4Logger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(testLogWriter{t}, nil))
}

// ln4Place writes one credential record into a person's store by hand, at the
// exact path the broker daemon would use. No store is opened, so no master key
// is created and nothing here holds credential material.
func ln4Place(t *testing.T, stateDir, who, profile, kind string) {
	t.Helper()
	ln4PlaceRaw(t, stateDir, who, profile+".cred", `{"kind":"`+kind+`","nonce":"","ct":""}`)
}

func ln4PlaceRaw(t *testing.T, stateDir, who, name, body string) {
	t.Helper()
	dir := filepath.Join(stateDir, "broker-store", who)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("store dir for %q: %v", who, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// ── T5 · a placed credential commissions THAT lane, and only that lane ──────

func TestLN4PlacedCredentialCommissionsTheLane(t *testing.T) {
	stateDir := t.TempDir()
	lanes := seedLanes(t)
	zai := laneByName(t, lanes, adapters.LaneZAI)
	kimi := laneByName(t, lanes, adapters.LaneKimi)
	ln4Place(t, stateDir, "me", zai.Credential.Profile, broker.KindEngineCred)

	m := commissionEngineLanes(stateDir, lanes, ln4Logger(t))

	if len(m) != 1 {
		t.Fatalf("the commissioned map holds %d people (%v), want exactly the one who placed a credential", len(m), m)
	}
	entries, ok := m["me"]
	if !ok {
		t.Fatalf("the map is keyed by %v, want the broker `who` the store directory names — the credential "+
			"injector dials <stateDir>/broker/<who>.sock, so any other key dials a socket for somebody the "+
			"store knows nothing about (D2, S11.5)", keysOf(m))
	}
	if len(entries) != 1 {
		t.Fatalf("person `me` holds %d provider entries (%v), want exactly the placed lane's", len(entries), entries)
	}
	entry, ok := entries[zai.ProviderID]
	if !ok {
		t.Fatalf("the entry is keyed by %v, want the document's own provider id %q", keysOfEntries(entries), zai.ProviderID)
	}
	if got := entry.Options["baseURL"]; got != zai.BaseURL {
		t.Errorf("baseURL = %v, want the DOCUMENT's %q — no endpoint is composed anywhere but the lane "+
			"document (S03.6, CONVENTIONS §62)", got, zai.BaseURL)
	}
	if want := "{env:" + zai.Credential.EnvVar + "}"; entry.Options["apiKey"] != want {
		t.Errorf("apiKey = %v, want %q — the compiled config names the VARIABLE and never a value; the "+
			"material is resolved from the broker at spawn (S11.5)", entry.Options["apiKey"], want)
	}
	if _, ok := entries[kimi.ProviderID]; ok {
		t.Errorf("the kimi lane was commissioned with nothing placed under %q — commissioning is derived "+
			"from what an operator actually placed, never from what ships", kimi.Credential.Profile)
	}
}

func keysOf(m map[string]opencode.ProviderConfig) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func keysOfEntries(m opencode.ProviderConfig) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ── T6 · nothing but an engine-cred commissions, and nothing corrupt refuses
//        to start ────────────────────────────────────────────────────────────

func TestLN4NonEngineCredNeverCommissions(t *testing.T) {
	lanes := seedLanes(t)
	zai := laneByName(t, lanes, adapters.LaneZAI)

	for _, tc := range []struct{ name, kind string }{
		{"a signing key under the lane's profile", broker.KindSigningKey},
		{"a git ssh key under the lane's profile", broker.KindGitSSHKey},
		{"a kind nobody defined", "totally-made-up"},
		{"no kind at all", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stateDir := t.TempDir()
			ln4Place(t, stateDir, "me", zai.Credential.Profile, tc.kind)
			m := commissionEngineLanes(stateDir, lanes, ln4Logger(t))
			if len(m) != 0 {
				t.Errorf("kind %q commissioned %v. Only an engine-cred is DELIVERED to an engine (S11.5 "+
					"destination constraint): anything else under a lane's profile name commissions "+
					"nothing, or the spawn produces an engine authenticating as nobody.", tc.kind, m)
			}
		})
	}

	for _, tc := range []struct{ name, file, body string }{
		{"a truncated record", zai.Credential.Profile + ".cred", `{"kind":`},
		{"a record that is not JSON at all", zai.Credential.Profile + ".cred", "not json, not anything"},
		{"an empty record", zai.Credential.Profile + ".cred", ""},
		{"a file that is not a .cred", zai.Credential.Profile, `{"kind":"engine-cred","nonce":"","ct":""}`},
		{"a .cred-less name holding a record", "zai.json", `{"kind":"engine-cred","nonce":"","ct":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stateDir := t.TempDir()
			ln4PlaceRaw(t, stateDir, "me", tc.file, tc.body)
			m := commissionEngineLanes(stateDir, lanes, ln4Logger(t))
			if len(m) != 0 {
				t.Errorf("%s commissioned %v", tc.name, m)
			}
		})
	}
}

// ── T8 · the invariant, as a PROPERTY ───────────────────────────────────────
//
// "A placed engine-cred is a commissioned lane, and nothing else is"
// (S03.6 + S11.5). An example-based test samples that; this asserts it over
// every combination a fixed-seed draw reaches — including the R6 half, which
// the shipped documents alone cannot exercise because both of them declare a
// profile AND a variable. The two synthetic documents supply the other half.
func TestLN4CommissioningIsAProperty(t *testing.T) {
	shipped := seedLanes(t)
	lanes := append(append([]opencode.LaneConfig(nil), shipped...),
		// A document that names no auth profile: not commissionable, because
		// laneCredInject refuses to build an injector for it and a lane
		// registered as held with no credential path is the exact state
		// ErrLaneNotCommissioned exists to make visible.
		opencode.LaneConfig{
			Lane: "ln4-no-profile", ProviderID: "ln4-no-profile-provider", Substrate: adapters.SubstrateOpencode,
			Credential: opencode.LaneCredential{EnvVar: "LN4_NO_PROFILE_KEY"},
		},
		// A document that names no environment variable: same conjunction, other
		// half — the broker would resolve the material into nothing.
		opencode.LaneConfig{
			Lane: "ln4-no-var", ProviderID: "ln4-no-var-provider", Substrate: adapters.SubstrateOpencode,
			Credential: opencode.LaneCredential{Profile: "ln4-no-var"},
		},
	)
	people := []string{"p1", "p2", "p3"}
	kinds := []string{broker.KindEngineCred, broker.KindSigningKey, broker.KindGitSSHKey, ""}

	rng := rand.New(rand.NewSource(0x5A17))
	for draw := 0; draw < 200; draw++ {
		stateDir := t.TempDir()
		// The property is about PLACEMENT, not about lanes: what is placed is
		// an engine-cred under a PROFILE, and commissioning gives that person
		// every commissionable lane naming it. Those are not the same set —
		// two lanes can share one profile (the kimi lanes share `kimi-code`,
		// since one membership is one Console key), so a placement made while
		// iterating one lane legitimately commissions its sibling too.
		// Building `want` per drawn lane would encode the opposite claim.
		placed := map[string]map[string]string{}
		for _, who := range people {
			for _, lane := range lanes {
				if rng.Intn(2) == 0 {
					continue // this (person, lane) pair places nothing
				}
				kind := kinds[rng.Intn(len(kinds))]
				profile := lane.Credential.Profile
				if profile == "" {
					// A lane naming no profile cannot have one placed FOR it;
					// the draw still places a record, under the lane name, so
					// the property sees a store that is not empty.
					profile = lane.Lane
				}
				ln4Place(t, stateDir, who, profile, kind)
				// The LAST placement under a profile wins, because it
				// overwrites the record on disk. That matters now that two
				// lanes share one profile: a draw for the sibling lane can
				// replace an engine-cred with a signing-key under the same
				// name, and the store then holds what was written last.
				if placed[who] == nil {
					placed[who] = map[string]string{}
				}
				placed[who][profile] = kind
			}
		}
		want := map[string]bool{}
		for who, profiles := range placed {
			for _, lane := range lanes {
				if lane.Credential.Profile == "" || lane.Credential.EnvVar == "" {
					continue
				}
				if profiles[lane.Credential.Profile] == broker.KindEngineCred {
					want[who+"\x00"+lane.ProviderID] = true
				}
			}
		}

		got := map[string]bool{}
		for who, entries := range commissionEngineLanes(stateDir, lanes, ln4Logger(t)) {
			for providerID := range entries {
				got[who+"\x00"+providerID] = true
			}
		}
		for pair := range want {
			if !got[pair] {
				t.Fatalf("draw %d: a placed engine-cred did not commission its lane (%q missing)\nwant %v\ngot  %v",
					draw, pair, sortedPairs(want), sortedPairs(got))
			}
		}
		for pair := range got {
			if !want[pair] {
				t.Fatalf("draw %d: %q was commissioned without a placed engine-cred under a document "+
					"declaring both a profile and a variable\nwant %v\ngot  %v",
					draw, pair, sortedPairs(want), sortedPairs(got))
			}
		}
	}
}

func sortedPairs(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ── T10 · coverage, substrates, seats and injectors grow ONLY from placement ─

func TestLN4CoverageAndSeatsGrowOnlyFromPlacement(t *testing.T) {
	lanes := seedLanes(t)
	zai := laneByName(t, lanes, adapters.LaneZAI)
	kimi := laneByName(t, lanes, adapters.LaneKimi)

	stateDir := t.TempDir()
	ln4Place(t, stateDir, "me", zai.Credential.Profile, broker.KindEngineCred)
	m := commissionEngineLanes(stateDir, lanes, ln4Logger(t))

	if got := commissionedLanes(lanes, m); len(got) != 1 || got[0] != adapters.LaneZAI {
		t.Errorf("commissionedLanes = %v, want [zai] — coverage grows from what is placed (S08.8 step 3)", got)
	}
	if got := laneSubstrates(lanes, m)[adapters.LaneZAI]; got != adapters.SubstrateOpencode {
		t.Errorf("laneSubstrates[zai] = %q, want %q — without the mapping a zai-seated decision executes "+
			"on the Anthropic CLI and meters as anthropic (S03.2)", got, adapters.SubstrateOpencode)
	}
	if _, ok := laneSubstrates(lanes, m)[adapters.LaneKimi]; ok {
		t.Error("the kimi lane was mapped with nothing placed for it")
	}
	seats := laneAlternateSeats(lanes, m)
	exec := seats[worker.DutyExecution]
	if len(exec) != 1 || exec[0].Lane != adapters.LaneZAI || exec[0].Model != zai.DefaultModel {
		t.Errorf("execution alternates = %+v, want exactly the zai document's default model %q", exec, zai.DefaultModel)
	}
	for _, duty := range []string{worker.DutyPlanning, worker.DutyJudge, worker.DutyUtility} {
		if len(seats[duty]) != 0 {
			t.Errorf("duty %q gained a seat from a placed credential — a seat activates under coverage, and "+
				"no zai or kimi model has been measured against the B3/S07.5 bars (CONVENTIONS §63)", duty)
		}
	}

	build := laneCredInjector(stateDir, lanes, m)
	if build("me") == nil {
		t.Error("the person who placed the credential got no injector, so their commissioned lane could " +
			"never authenticate (S11.5)")
	}
	if build("someone-else") != nil {
		t.Error("a person with nothing placed got a credential injector")
	}

	// Both profiles placed: THREE lanes, sorted, three seats — and still
	// execution only. Three from two placements, because the kimi-code profile
	// serves both kimi lanes: one membership, one Console key, two engines.
	ln4Place(t, stateDir, "me", kimi.Credential.Profile, broker.KindEngineCred)
	both := commissionEngineLanes(stateDir, lanes, ln4Logger(t))
	if got, want := commissionedLanes(lanes, both), []string{adapters.LaneKimi, adapters.LaneKimiCLI, adapters.LaneZAI}; !equalStrings(got, want) {
		t.Errorf("commissionedLanes = %v, want %v (sorted)", got, want)
	}
	if got := laneAlternateSeats(lanes, both)[worker.DutyExecution]; len(got) != 3 {
		t.Errorf("execution alternates = %+v, want one seat per commissioned lane", got)
	}
	for _, duty := range []string{worker.DutyPlanning, worker.DutyJudge, worker.DutyUtility} {
		if len(laneAlternateSeats(lanes, both)[duty]) != 0 {
			t.Errorf("duty %q gained a seat from a second commissioned lane", duty)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── T12 · nothing placed ⇒ nothing changes, asserted at the ADAPTER surface ──
//
// TestLN4NothingPlacedIsInert pins the derivations. This pins the other end:
// the adapter the control plane actually registers still answers "no provider
// entries" for everybody, which is what makes a dispatch onto an uncommissioned
// lane a named refusal rather than an unauthenticated call.
func TestLN4StartupIsUnchangedWithNothingPlaced(t *testing.T) {
	stateDir := t.TempDir()
	lanes := seedLanes(t)
	logger := ln4Logger(t)

	m := commissionEngineLanes(stateDir, lanes, logger)
	if len(m) != 0 {
		t.Fatalf("an empty state dir commissioned %v — an absent store root is a host with nothing placed, "+
			"never an error and never a default", m)
	}

	reg := engineAdapters(engineAdapterDeps{
		Settings: settings.New(), Logger: logger, StateDir: stateDir, Lanes: lanes, Commissioned: m,
	})
	oc, ok := reg[adapters.SubstrateOpencode].(*opencode.Adapter)
	if !ok {
		t.Fatalf("substrate %q is registered with %T", adapters.SubstrateOpencode, reg[adapters.SubstrateOpencode])
	}
	for _, who := range []string{"me", "alice", "sinep", ""} {
		got, err := oc.ProvidersFor(who)
		if err != nil {
			t.Fatalf("ProvidersFor(%q): %v", who, err)
		}
		if len(got) != 0 {
			t.Errorf("ProvidersFor(%q) = %v with nothing placed, want none", who, got)
		}
	}
}

// ── the fill is composed from the DOCUMENTS, byte for byte ──────────────────
//
// R5: nothing in this packet may build a provider entry by hand. The entry the
// fill produces must be exactly what the document renders.
func TestLN4CommissionedEntryIsTheDocumentsOwn(t *testing.T) {
	stateDir := t.TempDir()
	lanes := seedLanes(t)
	zai := laneByName(t, lanes, adapters.LaneZAI)
	ln4Place(t, stateDir, "me", zai.Credential.Profile, broker.KindEngineCred)

	got, err := json.Marshal(commissionEngineLanes(stateDir, lanes, ln4Logger(t))["me"])
	if err != nil {
		t.Fatalf("marshal the composed entries: %v", err)
	}
	want, err := json.Marshal(zai.Providers())
	if err != nil {
		t.Fatalf("marshal the document's own entries: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("the commissioned entry is not the document's own.\ngot  %s\nwant %s", got, want)
	}
}

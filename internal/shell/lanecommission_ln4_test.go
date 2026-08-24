package shell

// lanecommission_ln4_test.go — P3-LN-4 BRIEF-SPECIFIED ACCEPTANCE TESTS,
// written by the grounding agent BEFORE any implementation exists
// (S03.6 lane onboarding, S11.5 credential posture, S08.8 selection).
//
// Two of these are RED on purpose at the moment they are committed. They fail
// because the feature is ABSENT, not because anything is misspelled: the
// composition root still constructs `engineCommissioned` empty, and the key
// ceremony's step-8 summary still tells the operator that placing a key cannot
// make a lane routable. Both statements stop being true in the same packet.
//
// Two are GREEN and must STAY green. They are the invariants the fill must not
// break: the presence read never opens the store (no decrypt, no master key,
// no write), and the nothing-placed path stays byte-identical.
//
// $0 absolutely: nothing here dials a provider, spawns an engine, or holds
// credential material of any kind.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// ln4Sources returns internal/shell's non-test Go sources, keyed by file name.
func ln4Sources(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	out := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		out[name] = string(raw)
	}
	if len(out) == 0 {
		t.Fatal("no non-test sources found — the source scan would pass vacuously")
	}
	return out
}

// ── R1 · the composition root fills the map from what is PLACED ─────────────
//
// RED until LN-4 lands. The seam's own comment says the key ceremony fills it;
// nothing does, so a placed key can never become a routable lane. The empty
// literal is the whole gap, stated in one line of Go.
//
// This is a STRUCTURAL pin, deliberately: the behavioural acceptance tests the
// brief specifies (§7 T5–T12) cannot be written against a producer that does
// not exist yet without failing to compile, and a compile error proves nothing
// about behaviour. What this test can say today, it says.
func TestLN4CommissionedMapIsFilledFromPlacedCredentials(t *testing.T) {
	src := ln4Sources(t)["shell.go"]
	if src == "" {
		t.Fatal("shell.go not found — the composition root moved and this scan is looking at nothing")
	}
	const emptyLiteral = "engineCommissioned := map[string]opencode.ProviderConfig{}"
	if strings.Contains(src, emptyLiteral) {
		t.Errorf("shell.go still constructs the commissioned map EMPTY:\n\t%s\n"+
			"A lane is COMMISSIONED by a credential and made SELECTABLE by a provider entry (S03.6/S11.5). "+
			"The ceremony places the first; this map is the second, and it feeds BOTH the adapter registration "+
			"and the spawn-time credential injector. While it is constructed empty, a placed key can never "+
			"become a routable lane. LN-4 fills it at startup from the credentials actually placed in each "+
			"person's broker store under each shipped lane document's auth profile.", emptyLiteral)
	}

	// …and the fill must be derived from the broker store, not from a default.
	// Any of these markers satisfies it; the brief recommends a read-only
	// placement reader owned by internal/broker (which owns the record shape).
	markers := []string{"broker-store", "PlacedProfiles", "PlacedEngineCreds", "brokerStoreRoot", "BrokerStoreRoot"}
	found := ""
	for name, body := range ln4Sources(t) {
		for _, m := range markers {
			if strings.Contains(body, m) {
				found = name + ":" + m
			}
		}
	}
	if found == "" {
		t.Errorf("no non-test source in internal/shell names the broker credential store (looked for %v). "+
			"Commissioning must be DERIVED from what an operator actually placed — never from a default and "+
			"never from what ships (S03.6: a lane is a provider entry per user plus a credential).", markers)
	}
}

// ── R9 · the ceremony's step-8 summary must tell the new truth ──────────────
//
// RED until LN-4 lands. Step 8 currently discloses the gap in operator words —
// honestly, and that honesty is why the text is load-bearing. Once the map is
// filled the same words become a lie in the opposite direction: they would
// tell an operator their freshly placed key does nothing.
//
// The script is ours to edit. The lane documents and the classifier fixtures
// are NOT, and nothing here touches them.
func TestLN4CeremonyStep8TellsTheNewTruth(t *testing.T) {
	path := filepath.Join("..", "..", "P3", "gates", "lane-key-ceremony.sh")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the key ceremony: %v", err)
	}
	body := string(raw)

	stale := []string{
		// The step-8 "what placing a key does not do" disclosure.
		"constructed EMPTY at v0",
		"the control plane will still not route",
		// The LN gate-batch item this packet IS.
		"Fill the control plane's commissioned map from what is placed",
	}
	for _, phrase := range stale {
		if strings.Contains(body, phrase) {
			t.Errorf("the key ceremony still says %q.\n"+
				"That sentence was true while the commissioned map was constructed empty. LN-4 fills it, so "+
				"step 8 must state the new truth: a placed credential IS picked up, at control-plane STARTUP, "+
				"and therefore a control plane already running when the key was placed must be restarted "+
				"before the lane is routable.", phrase)
		}
	}
	// The replacement must actually say the restart part — an operator who is
	// told only "it works now" will place a key and watch nothing happen.
	if !strings.Contains(body, "restart") && !strings.Contains(body, "Restart") {
		t.Error("the key ceremony never mentions a restart. Commissioning is startup-bound (CONVENTIONS §61(a): " +
			"opencode resolves its provider block when the server boots, and the router's coverage, lane→substrate " +
			"map and alternate seats are all composed once at the composition root). A key placed while the " +
			"control plane is running is picked up on its next start, and the operator has to be told so.")
	}
}

// ── R3 · the presence read is SECRET-FREE, and stays that way ───────────────
//
// GREEN today, load-bearing after LN-4. broker.OpenStore is not a read: it
// MkdirAll+Chmods the store dir and CREATES the per-broker master key if one
// is absent (store.go loadOrCreateMaster). A commissioning probe that called
// it would change the host every time a control plane started with nothing
// commissioned — and would hold a decryption key it has no business holding.
//
// The sanctioned posture is the one the broker's own kindOf takes and the
// tier-L predicate copies: read the record's PLAINTEXT `kind` field, never
// decrypt, never create the master key, never write (S11.5: the decision path
// holds zero secrets).
func TestLN4PresenceReadNeverOpensTheBrokerStore(t *testing.T) {
	banned := map[string]string{
		"broker.OpenStore": "OpenStore creates the store dir AND the master key — it is a write, and it hands back a decrypting Store",
		"broker.NewServer": "the control plane is not the broker daemon; serving the store from here would put member secrets in the large-attack-surface process (S01.2)",
		".Resolve(":        "Resolve DECRYPTS. Presence is a question about a kind, not about a secret (S11.5)",
	}
	for name, body := range ln4Sources(t) {
		for needle, why := range banned {
			if strings.Contains(body, needle) {
				t.Errorf("%s contains %q — %s.\n"+
					"Commissioning asks whether a credential is PLACED; it must never learn what it is.", name, needle, why)
			}
		}
	}
}

// ── R6 · nothing placed ⇒ byte-identical behaviour ──────────────────────────
//
// GREEN today and the pin that must survive the fill. With no credential
// placed the map stays empty and every downstream derivation is exactly what
// it was before LN-4: no coverage growth, no lane→substrate mapping, no
// alternate seat, no injector. The empty path is the one an operator who has
// placed nothing actually runs, and it must not change at all.
func TestLN4NothingPlacedIsInert(t *testing.T) {
	lanes := seedLanes(t)
	empty := map[string]opencode.ProviderConfig{}

	if got := commissionedLanes(lanes, empty); len(got) != 0 {
		t.Errorf("commissionedLanes = %v with nothing placed, want none — coverage may grow only from a "+
			"credential an operator actually placed (S08.8 step 3; D5)", got)
	}
	if got := laneSubstrates(lanes, empty); got != nil {
		t.Errorf("laneSubstrates = %v with nothing placed, want nil — mapping a lane nobody holds would "+
			"describe a dispatch that cannot happen (S03.2)", got)
	}
	if seats := laneAlternateSeats(lanes, empty); len(seats[worker.DutyExecution]) != 0 {
		t.Errorf("laneAlternateSeats seated %d execution alternates with nothing placed, want 0 — a seat "+
			"activates under COVERAGE, never under registration (CONVENTIONS §63)", len(seats[worker.DutyExecution]))
	}
	build := laneCredInjector(t.TempDir(), lanes, empty)
	for _, who := range []string{"alice", "bob", "sinep", ""} {
		if build(who) != nil {
			t.Errorf("laneCredInjector built an injector for %q with nothing placed, want nil — a lane with "+
				"no credential must report itself uncommissioned rather than spawn an engine that "+
				"authenticates as nobody (S11.5)", who)
		}
	}
}

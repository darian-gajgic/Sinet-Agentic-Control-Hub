package shell

// lanedrain_r1_test.go — P3-LN-4 drain r1 / D2, D3, D4, D5 (S11.5, S14.1, S03.6).
//
// Four guards over the ways commissioning can be WRONG QUIETLY: a typo'd
// profile that reads like an empty host, a person's whole store dropped without
// a word, two spellings of the one rule that decides commissionability, and a
// hoisted lane set that stops being passed.
//
// $0: every record is a fake with an empty ciphertext in a t.TempDir() store.

import (
	"bytes"
	"log/slog"
	"regexp"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
)

// ln4Startup composes the map over a state dir and returns everything the
// control plane said while doing it.
func ln4Startup(t *testing.T, stateDir string, lanes []opencode.LaneConfig) (map[string]opencode.ProviderConfig, string) {
	t.Helper()
	var out bytes.Buffer
	m := commissionEngineLanes(stateDir, lanes, slog.New(slog.NewTextHandler(&out, nil)))
	t.Logf("startup said: %s", strings.TrimRight(out.String(), "\n"))
	return m, out.String()
}

// ── D2 · a typo'd profile must not read like an empty host ──────────────────
//
// The startup line exists so an operator can tell a placed key from a typo'd
// profile name. It could not: a store holding `zai-coding-plaan` produced a
// line character-identical to a store holding nothing, so the one failure the
// line was written to surface was the one it hid.
func TestLN4StartupNamesAPlacedProfileNoLaneDocumentClaims(t *testing.T) {
	lanes := seedLanes(t)
	zai := laneByName(t, lanes, adapters.LaneZAI)
	typo := zai.Credential.Profile + "n" // zai-coding-plann: one key away from working

	nothing := t.TempDir()
	_, emptyLine := ln4Startup(t, nothing, lanes)

	mistyped := t.TempDir()
	ln4Place(t, mistyped, "me", typo, broker.KindEngineCred)
	m, typoLine := ln4Startup(t, mistyped, lanes)

	if len(m) != 0 {
		t.Fatalf("a profile no lane document names commissioned %v", m)
	}
	if !strings.Contains(typoLine, typo) {
		t.Errorf("the startup line never names the placed profile %q that matched no lane document:\n\t%s\n"+
			"Auth-profile names ship in the lane documents and are not secrets. Without this, an operator who "+
			"mistyped a profile in the key ceremony sees exactly what an operator who placed nothing sees.",
			typo, strings.TrimRight(typoLine, "\n"))
	}
	if !strings.Contains(typoLine, "me") {
		t.Errorf("the startup line does not say WHOSE store the stray profile is in:\n\t%s", typoLine)
	}
	if ln4Strip(emptyLine) == ln4Strip(typoLine) {
		t.Errorf("a host with nothing placed and a host with a typo'd profile produce the same startup line:\n\t%s",
			ln4Strip(emptyLine))
	}
	// …and the empty host does not invent a stray of its own.
	if strings.Contains(emptyLine, typo) {
		t.Errorf("the nothing-placed line names a profile nobody placed:\n\t%s", emptyLine)
	}

	// The control: a CORRECTLY placed profile is not reported as a stray, so
	// the field means "nothing reads this" rather than "something was placed".
	good := t.TempDir()
	ln4Place(t, good, "me", zai.Credential.Profile, broker.KindEngineCred)
	held, goodLine := ln4Startup(t, good, lanes)
	if len(held) != 1 {
		t.Fatalf("the correctly placed credential commissioned %v", held)
	}
	if strings.Contains(goodLine, "placed_matching_no_lane_document=me:") {
		t.Errorf("a profile a lane document names was reported as matching no lane document:\n\t%s", goodLine)
	}
}

// ln4Strip removes the timestamp so two startup lines can be compared as text.
var ln4TimeField = regexp.MustCompile(`time=[^ ]+ `)

func ln4Strip(line string) string { return ln4TimeField.ReplaceAllString(line, "") }

// ── D3 · a person's store is never dropped in silence ───────────────────────
//
// A directory the broker will not open a store for (a dotted name, say) used to
// vanish from commissioning without a word, while its unreadable sibling warned
// by name. Nothing this platform writes can create such a directory — OpenStore
// applies the same check — so it was made by hand, which is precisely the case
// that deserves to be said out loud rather than shrugged off.
func TestLN4SkippedStoreDirectoryIsAnnounced(t *testing.T) {
	lanes := seedLanes(t)
	zai := laneByName(t, lanes, adapters.LaneZAI)
	stateDir := t.TempDir()
	ln4Place(t, stateDir, "first.last", zai.Credential.Profile, broker.KindEngineCred)
	ln4Place(t, stateDir, "ok", zai.Credential.Profile, broker.KindEngineCred)

	m, said := ln4Startup(t, stateDir, lanes)

	if _, held := m["first.last"]; held {
		t.Error("a store directory the broker refuses to open was commissioned anyway")
	}
	if len(m["ok"]) != 1 {
		t.Errorf("the valid sibling was not commissioned (%v) — a bad directory must not take its neighbours down", m)
	}
	if !strings.Contains(said, "first.last") {
		t.Errorf("the skipped store directory is never named:\n\t%s\n"+
			"Dropping a person's whole store in silence is how an operator spends an afternoon on a lane "+
			"that was never going to commission.", strings.TrimRight(said, "\n"))
	}
	if !strings.Contains(said, "level=WARN") {
		t.Errorf("the skipped store directory is mentioned below WARN:\n\t%s", said)
	}
	// The reason travels with it: a name says what, the error says why.
	if !strings.Contains(said, "auth-profile name") {
		t.Errorf("the warning does not say WHY the directory was skipped:\n\t%s", said)
	}
}

// ── D4 · ONE commissionability rule, consumed by both halves ────────────────
//
// R6 said "the same conjunction" precisely so this cannot drift: a document
// that commissions but has no injector is a lane routing seats and the spawn
// cannot authenticate; a document with an injector that never commissions is an
// injector nothing can reach.
func TestLN4CommissioningAndInjectionRefuseTheSameDocuments(t *testing.T) {
	shipped := seedLanes(t)
	cases := append(append([]opencode.LaneConfig(nil), shipped...),
		opencode.LaneConfig{Lane: "no-profile", ProviderID: "no-profile-p",
			Credential: opencode.LaneCredential{EnvVar: "NO_PROFILE_KEY"}},
		opencode.LaneConfig{Lane: "no-var", ProviderID: "no-var-p",
			Credential: opencode.LaneCredential{Profile: "no-var-profile"}},
		opencode.LaneConfig{Lane: "neither", ProviderID: "neither-p"},
	)

	for _, lane := range cases {
		// Placed under every name it could plausibly be placed under, so the
		// only thing that can differ is the commissionability rule itself.
		placed := map[string]map[string]bool{"me": {
			lane.Credential.Profile: true, lane.Lane: true, lane.ProviderID: true,
		}}
		_, commissions := opencode.Commission([]opencode.LaneConfig{lane}, placed)["me"][lane.ProviderID]
		injects := laneCredInject("/run/sinet/broker/me.sock", lane) != nil
		if commissions != injects {
			t.Errorf("lane %q: commissioned=%v injectable=%v (profile=%q env_var=%q).\n"+
				"These are one rule. A lane that commissions with no injector is seated by routing and then "+
				"authenticates as nobody; a lane with an injector that never commissions is a credential path "+
				"nothing can reach (S11.5, R6).",
				lane.Lane, commissions, injects, lane.Credential.Profile, lane.Credential.EnvVar)
		}
		if got := lane.Commissionable(); got != commissions {
			t.Errorf("lane %q: Commissionable()=%v but Commission %v it — the predicate and its consumer disagree",
				lane.Lane, got, map[bool]string{true: "commissioned", false: "did not commission"}[commissions])
		}
	}
}

// ── D5 · the hoisted lane set must keep reaching the registration ───────────
//
// engineAdapterDeps.Lanes falls back to loading the documents itself, so
// dropping `Lanes:` from the composition root compiles, runs and leaves the
// suite green — while re-creating exactly the desync this packet hoisted the
// load to prevent: the commissioned map derived from one read of the documents
// and the adapter registered against another.
func TestLN4CompositionRootPassesTheHoistedLaneSet(t *testing.T) {
	src := ln4Sources(t)["shell.go"]
	if src == "" {
		t.Fatal("shell.go not found — the composition root moved and this scan is looking at nothing")
	}
	hoist := regexp.MustCompile(`(?m)^\s*engineLaneDocs\s*:?=\s*engineLanes\(`)
	if !hoist.MatchString(src) {
		t.Error("shell.go no longer loads the lane documents ONCE into a shared value")
	}
	// Matched anywhere in the file, not inside a brace-bounded window: a
	// composite literal in an earlier FIELD (`&slog.HandlerOptions{}` in the
	// logger, say) ends a `[^}]*` window early, and the guard then reports
	// that Lanes is not passed while it sits there untouched. A guard whose
	// failure message can be the opposite of the truth is worse than none.
	passed := regexp.MustCompile(`Lanes:\s*engineLaneDocs\b`)
	if !passed.MatchString(src) {
		t.Error("the composition root does not pass the hoisted lane set into engineAdapterDeps.Lanes.\n" +
			"engineAdapters falls back to loading the documents itself, so this omission is SILENT: the " +
			"commissioned map would be derived from one read of the lane documents and the adapter " +
			"registered against another, and a document that stopped loading between them would leave a " +
			"control plane whose adapter and whose router disagree about which lanes exist.")
	}
	// The commissioning read and every snapshot consumer take the same value.
	for _, consumer := range []string{
		"commissionEngineLanes(stateDir, engineLaneDocs",
		"commissionedLanes(engineLaneDocs",
		"laneSubstrates(engineLaneDocs",
		"laneAlternateSeats(engineLaneDocs",
		"laneCredInjector(stateDir, engineLaneDocs",
	} {
		if !strings.Contains(src, consumer) {
			t.Errorf("the composition root does not feed the hoisted lane set to %q", consumer)
		}
	}
	// Counted by CALL, not by the identifier that happens to be passed:
	// `lg := logger; engineLanes(lg)` is still a call, and a count keyed to
	// the literal `engineLanes(logger)` reads a renamed variable as zero
	// calls — green for a file that loads the documents five times.
	if n := strings.Count(src, "engineLanes("); n != 1 {
		t.Errorf("shell.go calls engineLanes( %d times, want exactly one — five reads of the same "+
			"documents can disagree with each other", n)
	}
}

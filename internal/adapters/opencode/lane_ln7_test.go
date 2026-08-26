package opencode

// lane_ln7_test.go — P3-LN-7 §10 specs T3, T4, T24 (S03.6, S10.5, §64).
//
// The `kimi-cli` lane arrives as a DOCUMENT in this corpus, on a substrate this
// package does not drive. That is the point: LaneConfig has carried a
// `substrate` field since LN-2B precisely so which engine serves a lane is
// DATA, and every platform-side consumer walks all documents substrate-blind.
//
// Hermetic and $0: every assertion is a read of a shipped JSON file.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

const (
	kimiCLIProviderID = "kimi-code-cli"
	kimiCLIEnvVar     = "KIMI_MODEL_API_KEY"
	kimiCredProfile   = "kimi-code"
	kimiPoolID        = "kimi-code-membership"
)

func kimiCLISeedBytes(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("lanedata/kimi-cli.json")
	if err != nil {
		t.Fatalf("read the shipped kimi-cli lane document: %v", err)
	}
	return raw
}

func laneNamed(t *testing.T, name string) LaneConfig {
	t.Helper()
	lanes, err := SeedLaneConfigs()
	if err != nil {
		t.Fatalf("SeedLaneConfigs: %v", err)
	}
	for _, l := range lanes {
		if l.Lane == name {
			return l
		}
	}
	t.Fatalf("no lane %q among the %d shipped documents", name, len(lanes))
	return LaneConfig{}
}

// ── T3 · the document loads, validates, and says the right things ────────────

func TestKimiCLILaneDocumentValidates(t *testing.T) {
	c, err := LoadLaneConfig(kimiCLISeedBytes(t))
	if err != nil {
		t.Fatalf("the shipped kimi-cli lane document does not validate: %v", err)
	}
	if c.Lane != adapters.LaneKimiCLI {
		t.Errorf("lane = %q, want %q", c.Lane, adapters.LaneKimiCLI)
	}
	if c.Substrate != adapters.SubstrateKimiCLI {
		t.Errorf("substrate = %q, want %q — this lane does NOT ride opencode", c.Substrate, adapters.SubstrateKimiCLI)
	}
	// A duplicate provider id is refused at seed time BY NAME, and rightly:
	// laneFor takes the first match. So the CLI lane needs its own.
	if c.ProviderID == kimiProviderID {
		t.Errorf("provider_id = %q, which collides with the opencode kimi lane — loadLaneDocs would refuse the corpus", c.ProviderID)
	}
	if c.ProviderID != kimiCLIProviderID {
		t.Errorf("provider_id = %q, want %q", c.ProviderID, kimiCLIProviderID)
	}
	// The npm field names the ai-sdk provider package opencode loads. This
	// substrate has none — the engine IS the pinned CLI, recorded in
	// components.lock. Declaring one would be a false dated claim in a
	// document whose entire value is that its claims are dated (§64).
	if c.NPM != "" {
		t.Errorf("npm = %q, want empty — a non-opencode substrate loads no ai-sdk provider package", c.NPM)
	}
	if c.Credential.Profile != kimiCredProfile {
		t.Errorf("credential profile = %q, want %q — the SAME broker profile as lane kimi (one membership, one key)", c.Credential.Profile, kimiCredProfile)
	}
	if c.Credential.EnvVar != kimiCLIEnvVar {
		t.Errorf("credential env_var = %q, want %q — a shell KIMI_API_KEY does NOT authenticate this CLI, and getting it wrong "+
			"produces a startup failure that looks like a broken lane", c.Credential.EnvVar, kimiCLIEnvVar)
	}
	if c.DefaultModel != "k3" {
		t.Errorf("default_model = %q, want %q", c.DefaultModel, "k3")
	}
	if c.EndpointMarker != "/coding/v1" {
		t.Errorf("endpoint_marker = %q, want %q", c.EndpointMarker, "/coding/v1")
	}
	// The npm emptiness needs a stated reason, or a later reader fills it in.
	if !strings.Contains(strings.ToLower(rawField(t, kimiCLISeedBytes(t), "npm_note")), "components.lock") {
		t.Error("npm is empty with no note pointing at components.lock — an empty field with no reason reads as an omission")
	}
	// C5 rides both lanes: the constraint is a property of the PROVIDER, not
	// of the client path.
	if c.DataPolicy.Constraint == "" {
		t.Error("the C5 data_policy rider is absent — it is a property of the provider and binds every path to it")
	}
	if c.DataPolicy.Enforced {
		t.Error("data_policy.enforced = true, but no per-lane data-policy enforcement point exists in the tree")
	}
	kimi := laneNamed(t, adapters.LaneKimi)
	if c.DataPolicy.Constraint != kimi.DataPolicy.Constraint {
		t.Errorf("the two kimi lanes carry different data-policy constraints (%q vs %q) — same provider, same constraint",
			c.DataPolicy.Constraint, kimi.DataPolicy.Constraint)
	}
	// reset_marker stays deliberately empty on both (A11 audit U4).
	if c.ResetMarker != "" {
		t.Errorf("reset_marker = %q, want empty — no reset-time signal is documented, and a fabricated one turns a probe-park into a wait on a time nobody published", c.ResetMarker)
	}
}

// ── T4 · the two documents' signal tables are pinned EQUAL ───────────────────

// TestBothKimiLaneSignalTablesAreEqual is the assertion that keeps one
// membership from being classified two ways. Same subscription, same wire: a
// divergence would make one lane freeze where the other parks, and the operator
// comparing the two lanes would read the difference as a property of the
// client path.
func TestBothKimiLaneSignalTablesAreEqual(t *testing.T) {
	type triple struct {
		Status  int
		Message string
		Class   string
	}
	set := func(c LaneConfig) map[triple]bool {
		out := map[triple]bool{}
		for _, r := range c.Signals {
			out[triple{r.HTTPStatus, r.MessageContains, string(r.DocumentedClass)}] = true
		}
		return out
	}
	api, cli := set(laneNamed(t, adapters.LaneKimi)), set(laneNamed(t, adapters.LaneKimiCLI))
	if len(api) == 0 {
		t.Fatal("the kimi lane has no signal rows — the comparison would pass vacuously")
	}
	for row := range api {
		if !cli[row] {
			t.Errorf("lane kimi carries signal row %+v and lane kimi-cli does not", row)
		}
	}
	for row := range cli {
		if !api[row] {
			t.Errorf("lane kimi-cli carries signal row %+v and lane kimi does not", row)
		}
	}

	// The five strings the 2026-08-26 re-read of the vendor's own error
	// reference added, each on BOTH documents.
	for _, want := range []triple{
		{403, "reached your 5-hour usage limit", "depletion"},
		{403, "reached your weekly (7-day) usage limit", "depletion"},
		{403, "credit balance is insufficient", ""},
		{403, "concurrent request limit", ""},
		{402, "unable to verify your membership benefits", ""},
	} {
		if !api[want] || !cli[want] {
			t.Errorf("the 2026-08-26 row %+v is missing (kimi=%v kimi-cli=%v)", want, api[want], cli[want])
		}
	}
}

// TestConcurrentLimitRowCarriesNoDocumentedClass is the narrow half of T5 that
// lives with the documents: the trap itself.
//
// `403 "You've reached your concurrent request limit"` is BOTH an ordinary
// concurrency shed AND the vendor's stated enforcement signal for a terms
// concern — verbatim: "we'll … take appropriate action—such as limiting
// concurrent access—based on the severity. You'll then see a You've reached
// your concurrent request limit error." Classing it `transient` would make the
// platform retry silently THROUGH an enforcement action against the operator's
// own account.
func TestConcurrentLimitRowCarriesNoDocumentedClass(t *testing.T) {
	for _, lane := range []string{adapters.LaneKimi, adapters.LaneKimiCLI} {
		c := laneNamed(t, lane)
		found := false
		for _, r := range c.Signals {
			if !strings.Contains(r.MessageContains, "concurrent request limit") {
				continue
			}
			found = true
			if r.DocumentedClass != "" {
				t.Errorf("lane %s: the concurrent-limit row declares documented_class %q — it must carry NONE and fall "+
					"through to the Class-4 status rule, because it is also the vendor's enforcement signal", lane, r.DocumentedClass)
			}
			if !strings.Contains(strings.ToLower(r.Note), "enforcement") {
				t.Errorf("lane %s: the concurrent-limit row's note does not say why it carries no class: %q", lane, r.Note)
			}
		}
		if !found {
			t.Errorf("lane %s carries no concurrent-request-limit row", lane)
		}
	}
}

// ── T24 · both documents declare the SAME pool ───────────────────────────────

func TestBothLaneDocumentsDeclareTheSamePool(t *testing.T) {
	api, cli := laneNamed(t, adapters.LaneKimi), laneNamed(t, adapters.LaneKimiCLI)
	if api.Pool == "" {
		t.Fatal("lane kimi declares no pool — without it the two lanes read as two allowances")
	}
	if api.Pool != cli.Pool {
		t.Errorf("lane kimi declares pool %q and lane kimi-cli declares %q — one membership is one pool", api.Pool, cli.Pool)
	}
	if api.Pool != kimiPoolID {
		t.Errorf("pool = %q, want %q", api.Pool, kimiPoolID)
	}
	for _, c := range []LaneConfig{api, cli} {
		if c.PoolNote == "" {
			t.Errorf("lane %s carries a pool with no note — the shared-quota claim is the load-bearing one and it needs its source", c.Lane)
		}
		// The vendor's own words, dated. A pool claim with no quote is an
		// assertion; with the quote it is evidence.
		if !strings.Contains(c.PoolNote, "sharing the same quota") {
			t.Errorf("lane %s's pool note does not carry the vendor's verbatim shared-quota line: %q", c.Lane, c.PoolNote)
		}
		if !strings.Contains(c.PoolNote, "2026-08-26") {
			t.Errorf("lane %s's pool note carries no date", c.Lane)
		}
	}
}

// TestKimiCLIRecordsTheUsageOverlayUnwired pins R23(f): the `kimi web`
// GET /api/v1/oauth/usage finding is RECORDED and built on by nothing. An
// overlay with no consumer is the class §63 D2 exists to close, and the
// denominator stays the operator's budget (D4).
func TestKimiCLIRecordsTheUsageOverlayUnwired(t *testing.T) {
	c := laneNamed(t, adapters.LaneKimiCLI)
	var found bool
	for _, e := range c.RecordedEndpoints {
		if !strings.Contains(e.Note, "oauth/usage") && !strings.Contains(e.BaseURL, "oauth/usage") {
			continue
		}
		found = true
		if e.Wired {
			t.Error("the oauth/usage overlay is marked wired — this packet builds nothing on it")
		}
		if e.VerifiedOn == "" {
			t.Error("the oauth/usage record carries no verified-on date")
		}
	}
	if !found {
		t.Error("the kimi-cli document does not record the kimi web oauth/usage finding — it closes A11 audit U3 and is a candidate for U4, and an unrecorded finding is a re-derived one")
	}
}

// rawField reads one top-level string field straight out of the document,
// for members LaneConfig deliberately does not model.
func rawField(t *testing.T, raw []byte, key string) string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal lane document: %v", err)
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return s
}

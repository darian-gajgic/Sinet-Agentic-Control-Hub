package opencode

// lane_ln3_test.go — P3-LN-3 §6 specs 1-5 + 22-24 (S03.6, S03.1, S10.5, D2).
//
// The Kimi (Moonshot) lane, onboarded via the report-02 §5 checklist. Every
// value asserted here traces to a quoted line in
// P3/measurements/2026-08-24-kimi-lane-gate-audit.md — the packet's only source
// of provider facts. Hermetic and $0: no live provider call, no credential
// material, every turn terminating on the loopback fake serve.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// kimiCaptureDates are the dated captures this document is sourced from. It
// began as one (the 2026-08-24 Gate A-C audit) and gained a second at P3-LN-7
// (the 2026-08-26 re-read that added seven signal rows and regraded two model
// ids). A row must carry a date from a REAL capture — the point of the pin is
// that no row is dated by hand.
var kimiCaptureDates = map[string]bool{"2026-08-24": true, "2026-08-26": true}

var kimiCaptureDateList = []string{"2026-08-24", "2026-08-26"}

const (
	kimiProviderID   = "kimi-for-coding"
	kimiCodingBase   = "https://api.kimi.com/coding/v1"
	kimiMeteredIntl  = "https://api.moonshot.ai/v1"
	kimiMeteredCN    = "https://api.moonshot.cn/v1"
	kimiAnthropicNPM = "@ai-sdk/anthropic"
)

// kimiSeedBytes reads the SHIPPED kimi lane document off disk rather than out
// of the embed, so the negative table below can edit it by one field. The
// package's tests run with the package directory as their cwd.
func kimiSeedBytes(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("lanedata/kimi.json")
	if err != nil {
		t.Fatalf("read the shipped kimi lane document: %v", err)
	}
	return raw
}

// kimiDocWith renders the shipped kimi seed with one field edited, so each
// negative differs from a KNOWN-GOOD document by exactly one thing (the
// laneDocWith idiom, pointed at this lane's seed).
func kimiDocWith(t *testing.T, edit func(map[string]any)) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(kimiSeedBytes(t), &m); err != nil {
		t.Fatalf("decode the kimi seed document: %v", err)
	}
	edit(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	return out
}

func seedKimi(t *testing.T) LaneConfig {
	t.Helper()
	seeds, err := SeedLaneConfigs()
	if err != nil {
		t.Fatalf("SeedLaneConfigs: %v", err)
	}
	for _, c := range seeds {
		if c.Lane == adapters.LaneKimi {
			return c
		}
	}
	t.Fatalf("no seed lane document for %q (lanes: %d)", adapters.LaneKimi, len(seeds))
	return LaneConfig{}
}

// ── spec 1 · the shipped document loads, with the audit's own values ─────────

func TestKimiLaneDocumentLoads(t *testing.T) {
	seed := seedKimi(t)

	if seed.ProviderID != kimiProviderID {
		t.Errorf("provider id = %q, want %q (the models.dev provider URL and the installed opencode provider record both resolve under it)", seed.ProviderID, kimiProviderID)
	}
	if seed.Substrate != adapters.SubstrateOpencode {
		t.Errorf("substrate = %q, want %q — a lane is never a new substrate (S03.6)", seed.Substrate, adapters.SubstrateOpencode)
	}
	if seed.NPM != kimiAnthropicNPM {
		t.Errorf("npm = %q, want %q — this lane rides opencode on the ANTHROPIC protocol where zai rides it OpenAI-compatible, and the field exists so that difference is DATA", seed.NPM, kimiAnthropicNPM)
	}
	if seed.BaseURL != kimiCodingBase {
		t.Errorf("endpoint = %q, want the Kimi Code membership endpoint %q", seed.BaseURL, kimiCodingBase)
	}
	if seed.EndpointMarker != "/coding/v1" {
		t.Errorf("endpoint marker = %q, want /coding/v1 — the substring both the vendor docs and models.dev agree on", seed.EndpointMarker)
	}
	if seed.VerifiedOn != "2026-08-24" {
		t.Errorf("verified_on = %q, want the audit's access date 2026-08-24", seed.VerifiedOn)
	}
	if seed.DefaultModel != "k3" {
		t.Errorf("default_model = %q, want k3 — the operator's stated reason for the lane", seed.DefaultModel)
	}
	// The vendor's own endpoint warning is quoted, AND the honest limit is
	// stated: Moonshot does not publish what a membership key does against the
	// metered endpoint (audit U5), so nothing here may imply a guarantee.
	if !strings.Contains(seed.EndpointNote, "different Base URLs") {
		t.Errorf("the endpoint note does not carry the vendor's own warning: %q", seed.EndpointNote)
	}
	if !strings.Contains(strings.ToLower(seed.EndpointNote), "does not publish") {
		t.Errorf("the endpoint note claims more than the vendor published — the unpublished wrong-endpoint behavior (U5) must be stated, not implied away: %q", seed.EndpointNote)
	}

	wantModels := map[string]bool{"k3": false, "k3-256k": false, "kimi-for-coding": false, "kimi-for-coding-highspeed": false}
	for _, m := range seed.Models {
		if _, ok := wantModels[m.ID]; ok {
			wantModels[m.ID] = true
		}
		// The document is now sourced from TWO dated captures — the original
		// 2026-08-24 audit and the 2026-08-26 re-read that regraded two model
		// ids against the vendor's own models page. Every row still carries a
		// date from a real capture; what moved is that there is more than one.
		if !kimiCaptureDates[m.VerifiedOn] {
			t.Errorf("model %q verified_on = %q, want one of the capture dates %v", m.ID, m.VerifiedOn, kimiCaptureDateList)
		}
		if m.Billing != BillingFlat {
			t.Errorf("model %q billing = %q, want flat — the membership is a flat subscription", m.ID, m.Billing)
		}
		if m.OverflowMode != OverflowOptInCredits {
			t.Errorf("model %q overflow_mode = %q, want %q — Extra Usage is off by default with a PROVEN disable (C1/P-T17-2)", m.ID, m.OverflowMode, OverflowOptInCredits)
		}
		// The gate is by MEMBERSHIP TIER, not by region: the model-list-diff
		// canary handles it, and no tier_model_gate attribute is invented.
		if m.RegionModelGate != "none" {
			t.Errorf("model %q region_model_gate = %q, want none — Kimi's gate is by tier, and the tier gate is the model-list canary's job (P-T17-3)", m.ID, m.RegionModelGate)
		}
	}
	for id, seen := range wantModels {
		if !seen {
			t.Errorf("seed model %q missing", id)
		}
	}
	if !strings.Contains(strings.ToLower(seed.ModelRoutingNote), "tier") {
		t.Errorf("the model-routing note never says the gate is by tier rather than by region: %q", seed.ModelRoutingNote)
	}

	// Both metered platform bases are RECORDED and not wired, so the endpoint
	// self-check owns the wrong-endpoint case by name (B5, the BytePlus lesson).
	recorded := map[string]bool{}
	for _, e := range seed.RecordedEndpoints {
		recorded[e.BaseURL] = true
		if e.Wired {
			t.Errorf("recorded endpoint %q is marked wired — it must be recorded, never wired", e.BaseURL)
		}
		if e.VerifiedOn != "2026-08-24" {
			t.Errorf("recorded endpoint %q carries verified_on %q, want 2026-08-24", e.BaseURL, e.VerifiedOn)
		}
	}
	for _, want := range []string{kimiMeteredIntl, kimiMeteredCN, "https://api.kimi.com/coding/"} {
		if !recorded[want] {
			t.Errorf("the endpoint %q is not recorded as a dated fact", want)
		}
	}

	// The reset marker is DELIBERATELY empty: the audit found no documented
	// reset-time signal (U4), and an invented one would fabricate a Class 2.
	if seed.ResetMarker != "" {
		t.Errorf("reset_marker = %q, want empty — no reset-time signal is documented (U4), and depletion must route to the Class-3 probe park rather than to a fabricated Class 2", seed.ResetMarker)
	}
	// No Eco lever: K3 documents thinking efforts but NO disable, so the Z.AI
	// thinking:{type:disabled} rung has no equivalent and none is invented.
	if len(seed.EcoOptions) != 0 {
		t.Errorf("eco_options = %v, want none — Kimi publishes no thinking DISABLE, so wiring an S10.6 rung here would invent one", seed.EcoOptions)
	}
	if len(seed.EcoOptionEfforts) != 0 {
		t.Errorf("thinking_disabled_efforts = %v, want none for this lane", seed.EcoOptionEfforts)
	}
	if seed.ReasoningEffort.Wired {
		t.Error("reasoning_effort is marked wired — the efforts are a recorded dated fact, not a wired lever")
	}

	// The credential is a NAME plus a variable name — never material.
	if seed.Credential.Profile == "" {
		t.Error("the document names no broker auth profile")
	}
	// OQ4: the variable name is READ OFF the installed opencode provider record
	// ($0, local) or landed empty with the named-missing state — never guessed.
	// Whichever happened, the document says which.
	if seed.Credential.EnvVar == "" {
		t.Error("the credential variable is empty; if that is the OQ4 outcome the note must say so and R11's named-missing state is what carries it")
	}
	if !strings.Contains(strings.ToLower(seed.Credential.Note), "npm-shipped opencode") {
		t.Errorf("the credential note does not record where the variable name came from — a name with no provenance is a guess wearing a date: %q", seed.Credential.Note)
	}
	if !strings.Contains(seed.Credential.Note, "2026-08-24") {
		t.Errorf("the credential note carries no date for that reading: %q", seed.Credential.Note)
	}

	// The C5 routing rider rides the document as DATA, and is honest that it is
	// recorded rather than machine-enforced (OQ3).
	if seed.DataPolicy.Constraint != "no-household-personal-data" {
		t.Errorf("data_policy.constraint = %q, want no-household-personal-data — the C5 rider is mandatory for this lane", seed.DataPolicy.Constraint)
	}
	if seed.DataPolicy.Enforced {
		t.Error("the data-policy row claims enforcement; no per-lane data-policy enforcement point exists, and claiming one would let a reader assume the code stops it")
	}
	if seed.DataPolicy.VerifiedOn != "2026-08-24" {
		t.Errorf("data_policy verified_on = %q, want 2026-08-24", seed.DataPolicy.VerifiedOn)
	}
	if !strings.Contains(seed.DataPolicy.Audit, "2026-08-24-kimi-lane-gate-audit.md") {
		t.Errorf("the data-policy row does not cite the audit document: %q", seed.DataPolicy.Audit)
	}
	if strings.Contains(strings.ToLower(seed.DataPolicy.EnforcementNote), "applied") {
		t.Errorf("the enforcement note says the rider is 'applied' — it is recorded and surfaced, NOT machine-enforced: %q", seed.DataPolicy.EnforcementNote)
	}
	if !strings.Contains(seed.DataPolicy.EnforcementNote, "NOT MACHINE-ENFORCED") {
		t.Errorf("the enforcement note does not state plainly that the rider is not machine-enforced: %q", seed.DataPolicy.EnforcementNote)
	}

	// drain r1 F3 · three seed values reached this document from the BRIEF and
	// not from the audit, which never captured them. The packet cannot re-fetch
	// (scope wall) and the audit is not retro-edited with quotes it never took,
	// so each says whose word it is and what grade that is.
	for _, m := range seed.Models {
		if m.ID != "kimi-for-coding" && m.ID != "kimi-for-coding-highspeed" {
			continue
		}
		if m.NoteGrade != "unverified-primary" {
			t.Errorf("model %q carries a tier-availability note at grade %q — the audit captured no tier line for "+
				"it, so the note is the brief's word and must say so", m.ID, m.NoteGrade)
		}
		if !strings.Contains(m.Note, "PROVENANCE") {
			t.Errorf("model %q's note names no provenance: %q", m.ID, m.Note)
		}
	}
	if seed.ReasoningEffort.ValuesGrade != "unverified-primary" {
		t.Errorf("the thinking-effort values carry grade %q — the audit records no effort vocabulary for this "+
			"model at all, so the list is the brief's word", seed.ReasoningEffort.ValuesGrade)
	}

	// drain r1 F4 · the installed engine's own catalogue, read dated. All four
	// declared ids ARE in it — the finding's premise that two were absent does
	// not hold — but its display name for one model diverges, and a divergence
	// is recorded rather than reconciled away.
	// drain r2 C1 · the record is a read of what the PINNED ENGINE embeds, and
	// the verbatim id list is what makes every other claim in it checkable.
	cat := seed.InstalledCatalogue
	if cat.VerifiedOn != "2026-08-24" {
		t.Errorf("the installed-catalogue record carries verified_on %q, want 2026-08-24", cat.VerifiedOn)
	}
	wantIDs := []string{"k2p5", "k2p7", "kimi-k2-thinking", "kimi-for-coding-highspeed", "k3", "k2p6"}
	if len(cat.ModelsVerbatim) != len(wantIDs) {
		t.Fatalf("models_verbatim = %v, want the engine record's own six ids %v", cat.ModelsVerbatim, wantIDs)
	}
	for i, id := range wantIDs {
		if cat.ModelsVerbatim[i] != id {
			t.Errorf("models_verbatim[%d] = %q, want %q", i, cat.ModelsVerbatim[i], id)
		}
	}
	// The record is EMBEDDED in the shipped binary. It is not a cache, and the
	// difference is the whole provenance — reading a runtime-fetched snapshot
	// as "the installed engine's record" is exactly the error this replaces.
	if !strings.Contains(cat.Source, "EMBEDDED") || !strings.Contains(cat.Source, "binar") {
		t.Errorf("the catalogue record does not say the record is embedded in the shipped binary: %q", cat.Source)
	}
	if strings.Contains(strings.ToLower(cat.Source), "cached file") && !strings.Contains(cat.Source, "NOT a cached file") {
		t.Errorf("the catalogue record still calls the source a cached file: %q", cat.Source)
	}
	if !strings.Contains(cat.SecondSourceDisagrees, "models.dev") {
		t.Errorf("the record does not carry the second, disagreeing source it was first written from: %q", cat.SecondSourceDisagrees)
	}
	// The two ids the engine record does NOT carry are flagged, not dropped.
	graded := map[string]string{}
	for _, m := range seed.Models {
		graded[m.ID] = m.ObservationGrade
	}
	for _, id := range []string{"k3-256k", "kimi-for-coding"} {
		if graded[id] != "documented-primary-absent-from-pinned-engine-record" {
			t.Errorf("model %q carries observation_grade %q — it is ABSENT from the pinned engine's own record "+
				"and must say so; the account's observed list settles it, and silently dropping it would lose a "+
				"fact the vendor publishes", id, graded[id])
		}
	}
	for _, id := range []string{"k3", "kimi-for-coding-highspeed"} {
		if graded[id] != "" {
			t.Errorf("model %q is flagged %q, but the engine record carries it", id, graded[id])
		}
	}
	// The engine record contradicts BOTH halves of the brief-sourced thinking
	// claim, so both are graded and neither is wired.
	for _, needle := range []string{`values:["max"]`, "toggle"} {
		if !strings.Contains(cat.ReasoningOptions, needle) {
			t.Errorf("the catalogue record does not carry the k3 reasoning fact %q: %q", needle, cat.ReasoningOptions)
		}
	}
	if !strings.Contains(seed.ReasoningEffort.Note, "toggle") {
		t.Errorf("the thinking-effort note does not record the engine's `toggle` type, which contradicts its own "+
			"no-disable claim: %q", seed.ReasoningEffort.Note)
	}
	if seed.ReasoningEffort.Wired {
		t.Error("the thinking lever is wired despite both halves of its evidence being graded unverified")
	}
	// The limits belong to the ids that carry them.
	if !strings.Contains(cat.Limits, "1,048,576") || !strings.Contains(cat.Limits, "262,144") {
		t.Errorf("the catalogue record does not attribute the context limits: %q", cat.Limits)
	}

	// Every signal row is dated AND marked documented-not-observed (§4's
	// non-negotiable): these are published message strings, not wire bodies.
	if len(seed.Signals) == 0 {
		t.Fatal("the kimi document declares no signal rows")
	}
	for _, row := range seed.Signals {
		if !kimiCaptureDates[row.VerifiedOn] {
			t.Errorf("signal row %q verified_on = %q, want one of the capture dates %v", row.MessageContains, row.VerifiedOn, kimiCaptureDateList)
		}
		if row.MessageContains == "" {
			t.Errorf("signal row on HTTP %d carries no message_contains — this lane's taxonomy is (status x message), and a status-only row cannot key it", row.HTTPStatus)
		}
	}
	if !strings.Contains(seed.SignalsNote, "DOCUMENTED-NOT-OBSERVED") {
		t.Errorf("the signal table is not marked DOCUMENTED-NOT-OBSERVED: %q", seed.SignalsNote)
	}
	if !strings.Contains(strings.ToLower(seed.SignalsNote), "probe") {
		t.Errorf("the signal note does not name the ceremony's live probe as the thing that closes it: %q", seed.SignalsNote)
	}
}

// ── spec 2 · duplicate lane / duplicate provider id are refused BY NAME ──────

func TestLaneSeedRefusesDuplicateLaneOrProvider(t *testing.T) {
	doc := func(lane, provider string) string {
		return `{
		  "lane": "` + lane + `",
		  "substrate": "opencode",
		  "provider_id": "` + provider + `",
		  "npm": "@ai-sdk/openai-compatible",
		  "display_name": "D",
		  "verified_on": "2026-08-24",
		  "base_url": "https://api.example.invalid/v1",
		  "endpoint_marker": "/v1",
		  "credential": {"profile": "p", "env_var": "V"},
		  "models": [{"id": "m", "name": "M", "verified_on": "2026-08-24", "billing": "flat", "overflow_mode": "hard-stop", "region_model_gate": "none"}]
		}`
	}

	// The control: two documents with distinct lane names AND distinct provider
	// ids load, and come back sorted by lane name.
	ok := fstest.MapFS{
		"lanedata/beta.json":  {Data: []byte(doc("beta", "beta-plan"))},
		"lanedata/alpha.json": {Data: []byte(doc("alpha", "alpha-plan"))},
	}
	got, err := loadLaneDocs(ok, "lanedata")
	if err != nil {
		t.Fatalf("two distinct documents did not load: %v", err)
	}
	if len(got) != 2 || got[0].Lane != "alpha" || got[1].Lane != "beta" {
		t.Fatalf("loaded %+v, want both documents sorted by lane name", got)
	}

	for _, tc := range []struct {
		name  string
		fsys  fstest.MapFS
		wants []string
	}{
		{
			name: "two documents claiming the same lane",
			fsys: fstest.MapFS{
				"lanedata/a.json": {Data: []byte(doc("alpha", "alpha-plan"))},
				"lanedata/b.json": {Data: []byte(doc("alpha", "other-plan"))},
			},
			wants: []string{"alpha"},
		},
		{
			name: "two documents claiming the same provider id",
			fsys: fstest.MapFS{
				"lanedata/a.json": {Data: []byte(doc("alpha", "shared-plan"))},
				"lanedata/b.json": {Data: []byte(doc("beta", "shared-plan"))},
			},
			wants: []string{"shared-plan"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadLaneDocs(tc.fsys, "lanedata")
			if err == nil {
				t.Fatal("the duplicate loaded — laneFor takes the FIRST provider-id match and laneConfiguredModels overwrites by lane, so a config-only lane system whose failure mode is silent shadowing is not one")
			}
			if !errors.Is(err, ErrLaneConfig) {
				t.Errorf("error = %v, want it to wrap ErrLaneConfig", err)
			}
			for _, want := range tc.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal does not name %q — an unnamed refusal is one nobody can act on: %v", want, err)
				}
			}
		})
	}

	// And the SHIPPED set loads: three lanes, sorted, no duplicate. The third
	// arrived at P3-LN-7 and it is the first document in this corpus whose
	// substrate is NOT opencode — which is the point of the substrate field.
	shipped, err := SeedLaneConfigs()
	if err != nil {
		t.Fatalf("SeedLaneConfigs: %v", err)
	}
	if len(shipped) != 3 {
		t.Fatalf("the platform ships %d lane documents, want 3 (kimi + kimi-cli + zai)", len(shipped))
	}
	wantOrder := []string{adapters.LaneKimi, adapters.LaneKimiCLI, adapters.LaneZAI}
	for i, want := range wantOrder {
		if shipped[i].Lane != want {
			t.Errorf("shipped lane %d = %q, want %q — the corpus is sorted by lane name", i, shipped[i].Lane, want)
		}
	}
}

// ── spec 4 · message-keyed rows are ADDITIVE — zai is byte-identical ─────────

func TestMessageKeyedSignalRowsAreAdditive(t *testing.T) {
	zai := seedZAI(t)

	// The control that proves the extension changed nothing: the UNMODIFIED
	// zai seed still extracts exactly what it extracted before, and a document
	// with no message_contains row behaves exactly as it did.
	for _, tc := range []struct {
		code, status string
		body         string
		wantCode     string
		wantKnown    bool
	}{
		{code: "1308", body: zaiBody("1308", "Usage limit reached for `20` `credits`"), wantCode: "1308", wantKnown: true},
		{code: "1113", body: zaiBody("1113", "Insufficient balance or no resource package."), wantCode: "1113", wantKnown: true},
		{code: "9999", body: zaiBody("9999", "who knows"), wantCode: "9999", wantKnown: false},
	} {
		sig, ok := zai.ExtractSignal(tc.body, 0)
		if !ok {
			t.Fatalf("the zai seed stopped extracting %q — the message_contains extension must be additive", tc.code)
		}
		if sig.ErrorCode != tc.wantCode || sig.Known != tc.wantKnown {
			t.Errorf("zai %s: code=%q known=%v, want %q/%v", tc.code, sig.ErrorCode, sig.Known, tc.wantCode, tc.wantKnown)
		}
		if sig.DocumentedClass != "" {
			t.Errorf("zai %s carries documented_class %q — the zai document declares none, so the member must stay empty and the zai path byte-identical", tc.code, sig.DocumentedClass)
		}
	}
	// A body with no decodable provider envelope still yields NOTHING on a lane
	// whose taxonomy is code-keyed.
	if _, ok := zai.ExtractSignal("connection reset by peer", 429); ok {
		t.Error("the zai lane produced a signal from an undecodable body — a code-keyed lane yields nothing rather than a guess")
	}

	// Kimi: no code decodes, and the signal comes from (status, message).
	kimi := seedKimi(t)
	sig, ok := kimi.ExtractSignal(`{"error":{"message":"You've reached your usage limit for this billing cycle"}}`, 403)
	if !ok {
		t.Fatal("the kimi lane produced no signal for a documented 403 message — with no code field, ExtractSignal must still yield one or only a bare status reaches the classifier")
	}
	if !sig.Known {
		t.Error("a documented (403, weekly-depletion) pair is not Known")
	}
	if sig.HTTPStatus != 403 {
		t.Errorf("http status = %d, want 403", sig.HTTPStatus)
	}
	if sig.BodyText == "" {
		t.Error("the codeless signal carries no body text — the message string IS the taxonomy on this lane")
	}
	if sig.DocumentedClass != DocumentedDepletion {
		t.Errorf("documented_class = %q, want %q — this 403 is the weekly window emptying, not an auth event", sig.DocumentedClass, DocumentedDepletion)
	}

	// A NON-matching message on the same status is honestly unknown.
	unmatched, ok := kimi.ExtractSignal(`{"error":{"message":"something nobody documented"}}`, 403)
	if !ok {
		t.Fatal("an undocumented message on a message-keyed lane produced no signal at all")
	}
	if unmatched.Known {
		t.Error("an undocumented message is reported as Known")
	}
	if unmatched.DocumentedClass != "" {
		t.Errorf("an undocumented message carries documented_class %q — the guard is an EXEMPTION LIST and must never fire on a row nobody published", unmatched.DocumentedClass)
	}

	// drain r1 F1, the document-layer half: when a body matches BOTH a classed
	// and an unclassed same-status row, the UNCLASSED one wins — the row that
	// classifies on its status, which on 401/403 means it freezes.
	//
	// Pinned here rather than only through the classifier because the
	// scheduler's belt would catch these bodies anyway; without this assertion
	// the tie-break is a branch nothing reads, which is exactly the class of
	// dead wiring this codebase keeps finding.
	for _, tc := range []struct {
		name   string
		body   string
		status int
	}{
		{
			name:   "revocation phrase before the depletion phrase",
			body:   "Access terminated. Your account was suspended; you have also reached your usage limit for this billing cycle.",
			status: 403,
		},
		{
			name:   "depletion phrase before the revocation phrase",
			body:   "You've reached your usage limit for this billing cycle. Access terminated: this account is suspended.",
			status: 403,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := kimi.ExtractSignal(`{"error":{"message":"`+tc.body+`"}}`, tc.status)
			if !ok {
				t.Fatal("no signal was produced")
			}
			if !got.Known {
				t.Error("the body matches documented rows yet is reported unknown")
			}
			if got.DocumentedClass != "" {
				t.Errorf("documented_class = %q, want empty — an ambiguous body must resolve to the row that "+
					"FREEZES, never to whichever row the JSON happened to list first", got.DocumentedClass)
			}
		})
	}

	// Raw engine prose (no JSON envelope at all) still keys on the message.
	raw, ok := kimi.ExtractSignal("upstream said: The engine is currently overloaded, please try again later", 429)
	if !ok || !raw.Known || raw.DocumentedClass != DocumentedTransient {
		t.Errorf("engine-wrapped prose: ok=%v known=%v class=%q, want a known transient row", ok, raw.Known, raw.DocumentedClass)
	}
}

// ── spec 5 · the document's negative table ───────────────────────────────────

func TestKimiLaneDocumentRefusals(t *testing.T) {
	if _, err := LoadLaneConfig(kimiDocWith(t, func(map[string]any) {})); err != nil {
		t.Fatalf("the unedited kimi seed must load, or the refusals below prove nothing: %v", err)
	}

	models := func(m map[string]any) []any {
		list, _ := m["models"].([]any)
		if len(list) == 0 {
			t.Fatal("the seed document has no models to edit")
		}
		return list
	}
	first := func(m map[string]any) map[string]any {
		row, _ := models(m)[0].(map[string]any)
		if row == nil {
			t.Fatal("the seed's first model row is not an object")
		}
		return row
	}

	for _, tc := range []struct {
		name  string
		edit  func(map[string]any)
		wants []string
	}{
		{
			name:  "an invented overflow mode",
			edit:  func(m map[string]any) { first(m)["overflow_mode"] = "spill-quietly" },
			wants: []string{"overflow_mode", "spill-quietly", "k3"},
		},
		{
			name:  "an empty region_model_gate",
			edit:  func(m map[string]any) { first(m)["region_model_gate"] = "" },
			wants: []string{"region_model_gate", "k3"},
		},
		{
			name:  "no credential block",
			edit:  func(m map[string]any) { delete(m, "credential") },
			wants: []string{"credential", "kimi"},
		},
		{
			name: "an undated signal row",
			edit: func(m map[string]any) {
				rows, _ := m["signals"].([]any)
				row, _ := rows[0].(map[string]any)
				delete(row, "verified_on")
			},
			wants: []string{"signal row", "verified-on"},
		},
		{
			name:  "a malformed date",
			edit:  func(m map[string]any) { m["verified_on"] = "24 August 2026" },
			wants: []string{"verified-on", "24 August 2026"},
		},
		{
			name:  "a documented_class outside the narrow exemption vocabulary",
			edit:  func(m map[string]any) { setSignalClass(t, m, "auth") },
			wants: []string{"documented_class", "auth"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadLaneConfig(kimiDocWith(t, tc.edit))
			if err == nil {
				t.Fatal("the document loaded — the gate does not exist")
			}
			if !errors.Is(err, ErrLaneConfig) {
				t.Errorf("error = %v, want it to wrap ErrLaneConfig", err)
			}
			for _, want := range tc.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal does not name %q: %v", want, err)
				}
			}
		})
	}
}

func setSignalClass(t *testing.T, m map[string]any, class string) {
	t.Helper()
	rows, _ := m["signals"].([]any)
	if len(rows) == 0 {
		t.Fatal("the seed declares no signal rows")
	}
	row, _ := rows[0].(map[string]any)
	if row == nil {
		t.Fatal("the seed's first signal row is not an object")
	}
	row["documented_class"] = class
}

// ── spec 22 · no kimi value is a Go constant in this package ─────────────────

func TestNoKimiLaneValueIsAConstantInTheAdapter(t *testing.T) {
	for name, src := range packageSources(t) {
		for _, lit := range []string{"api.kimi.com", "kimi-for-coding", "api.moonshot", "\"k3\""} {
			if strings.Contains(src, lit) {
				t.Errorf("%s contains the literal %q — lane values are DATA with dates, never constants (S03.6)", name, lit)
			}
		}
	}
}

// ── spec 23 · the credential never leaves the broker path ────────────────────

func TestKimiCredentialNeverLeaves(t *testing.T) {
	const sentinel = "SINET-TEST-SECRET-a41f9c07-2d5e-4b18-93aa-71e0c4b8d2f6"
	e := newE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e.claimedRun(t, "r1")

	f := newFakeServe(t)
	f.onPrompt = func(f *fakeServe, n int) { f.publishFixture("happy.sse") }

	seed := seedKimi(t)
	logs := &captureWriter{}
	spy := &spyInstances{inst: f.instance()}
	a := laneAdapter(t, seed)
	a.Instances = spy
	a.Log = slog.New(slog.NewTextHandler(logs, nil))

	req := laneRequest(t, seed)
	req.CredInject = func(base []string) ([]string, error) {
		return append(append([]string(nil), base...), seed.Credential.EnvVar+"="+sentinel), nil
	}
	var payloads []string
	req.OnEvent = func(ev adapters.Event) { payloads = append(payloads, string(ev.Payload)) }

	out, err := e.drv.Drive(ctx, a, req)
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome = %q (%s)", out.Kind, out.Detail)
	}
	if len(spy.specs) != 1 {
		t.Fatalf("instance specs = %d, want 1", len(spy.specs))
	}
	if _, ok := envValue(spy.specs[0].Env, seed.Credential.EnvVar); !ok {
		t.Fatalf("the injected credential never reached the serve environment")
	}
	for i, p := range payloads {
		if strings.Contains(p, sentinel) {
			t.Errorf("event payload %d carries the credential: %s", i, p)
		}
	}
	if id := identityOf(spy.specs[0]); strings.Contains(id, sentinel) {
		t.Errorf("the instance identity key carries the credential: %q", id)
	}
	if s := logs.String(); strings.Contains(s, sentinel) {
		t.Error("the ops log carries the credential")
	}
	if p := out.Park; p != nil {
		blob, _ := json.Marshal(p)
		if strings.Contains(string(blob), sentinel) {
			t.Error("the park record carries the credential")
		}
	}
	rows, err := e.db.QueryContext(ctx, `SELECT payload FROM run_events WHERE run_id = 'r1'`)
	if err != nil {
		t.Fatalf("run_events: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if strings.Contains(payload, sentinel) {
			t.Errorf("a run_events payload carries the credential: %s", payload)
		}
	}
	if err := e.db.CheckpointTruncate(ctx); err != nil {
		t.Fatalf("checkpoint truncate: %v", err)
	}
	for _, suffix := range []string{"", "-wal"} {
		raw, err := os.ReadFile(e.db.Path() + suffix)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read db%s: %v", suffix, err)
		}
		if strings.Contains(string(raw), sentinel) {
			t.Errorf("the database file%s contains the credential", suffix)
		}
	}
}

// ── spec 24 · a missing credential is a NAMED state ──────────────────────────

func TestKimiMissingCredentialIsNamedState(t *testing.T) {
	seed := seedKimi(t)
	f := newFakeServe(t)
	spy := &spyInstances{inst: f.instance()}
	a := laneAdapter(t, seed)
	a.Instances = spy

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := a.Start(ctx, laneRequest(t, seed))
	if err == nil {
		t.Fatal("Start succeeded without a credential — an unauthenticated call is not a success")
	}
	if !strings.Contains(err.Error(), "not commissioned") {
		t.Errorf("error = %v, want a named not-commissioned state", err)
	}
	if !strings.Contains(err.Error(), seed.Credential.Profile) {
		t.Errorf("error = %v, want the auth-profile name so an operator knows what to place", err)
	}
	if len(spy.specs) != 0 {
		t.Errorf("a serve instance was acquired for an uncommissioned lane (%d specs)", len(spy.specs))
	}
	if n := len(f.requests()); n != 0 {
		t.Errorf("the uncommissioned lane made %d engine requests, want 0", n)
	}

	// The named-missing-credential state is ALSO what carries the OQ4 outcome
	// if the variable name could never be determined: a document with no
	// variable is refused at load, so an empty one can never sail past the
	// spawn-time check and authenticate as nobody.
	if _, err := LoadLaneConfig(kimiDocWith(t, func(m map[string]any) {
		cred, _ := m["credential"].(map[string]any)
		cred["env_var"] = ""
	})); err == nil {
		t.Error("a document with an empty credential variable loaded — the spawn-time check would be skipped and the engine would authenticate as nobody (S11.5)")
	}
}

// ── R15 · an opt-in-credits lane is distinguishable from a hard-stop lane ────

func TestOverflowRegimeIsDistinguishableAcrossLanes(t *testing.T) {
	kimi, zai := seedKimi(t), seedZAI(t)

	modes := func(c LaneConfig) map[string]bool {
		out := map[string]bool{}
		for _, m := range c.Models {
			out[m.OverflowMode] = true
		}
		return out
	}
	k, z := modes(kimi), modes(zai)
	if !k[OverflowOptInCredits] {
		t.Errorf("the kimi lane declares %v — Sinet's FIRST non-hard-stop lane must carry opt-in-credits end to end (3.10, C1)", k)
	}
	if k[OverflowHardStop] {
		t.Errorf("a kimi model declares hard-stop; the whole lane spills on opt-in, and a mixed claim would make the regime unreadable: %v", k)
	}
	if !z[OverflowHardStop] || z[OverflowOptInCredits] {
		t.Errorf("the zai lane's regime moved (%v) — it is hard-stop, and this test is the CONTROL that the two are distinguishable", z)
	}
	// TBD-S10.2(currency-flip receipts): no surface reads billing regime yet,
	// so "carried end to end" ends at the validated document. The consuming
	// section is S10.2's flip mechanics + 3.10's "receipts must visibly change
	// currency when overflow triggers" (report-01 P-T01-3); building it is out
	// of scope until that surface lands (the ProposePlanBudget/13.4 precedent).
	if !strings.Contains(strings.ToUpper(kimi.OverflowNote), "EXTRA USAGE OFF") {
		t.Errorf("the overflow note does not record the recommended operator posture: %q", kimi.OverflowNote)
	}
	if !strings.Contains(kimi.OverflowNote, "turn it off at any time") {
		t.Errorf("the overflow note does not quote the PROVEN disable that clears 3.10: %q", kimi.OverflowNote)
	}
}

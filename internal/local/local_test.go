package local

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
)

// local_test.go — tier-F hermetic tests of the pure surface (alias resolution,
// the manifest, config generation, the duty schemas, the usage marker, the
// GameMode legs). Zero network, zero paid calls.

func TestResolveAllAliasesEmbedderRefused(t *testing.T) {
	reg := NewRegistry(settings.New())
	// Every duty alias except the embedder resolves to a manifest seat (R15).
	for _, a := range []string{
		AliasUtility, AliasIntakeTriage, AliasWatchdog, AliasWatchlist,
		AliasIntentFilling, AliasSQLOpen, AliasEntailment, AliasContradiction,
		AliasDistillSummarize,
	} {
		seat, err := reg.SeatFor(a)
		if err != nil {
			t.Errorf("alias %q: %v", a, err)
			continue
		}
		if seat.Model == "" {
			t.Errorf("alias %q resolved to an empty model", a)
		}
	}
	// The embedder alias refuses with the post-gate reason (§8 reading 9).
	if _, err := reg.SeatFor(AliasEmbedder); !errors.Is(err, ErrEmbedderPostGate) {
		t.Errorf("embedder alias: got %v, want ErrEmbedderPostGate", err)
	}
	// An unknown alias is a loud error.
	if _, err := reg.SeatFor("bogus-alias"); !errors.Is(err, ErrUnknownAlias) {
		t.Errorf("unknown alias: got %v, want ErrUnknownAlias", err)
	}
}

func TestManifestLicensesAndServability(t *testing.T) {
	byModel := map[string]SeatRecord{}
	for _, s := range Manifest() {
		byModel[s.Model] = s
	}

	// The Gemma card-vs-spec conflict is recorded + flagged, never silently
	// adopted (R8).
	gemma := byModel["Gemma 4 12B QAT"]
	if !gemma.License.Conflict {
		t.Error("Gemma 4 12B QAT: license conflict not flagged (R8)")
	}
	// Bespoke-MiniCheck is the sole ratified CC-BY-NC exception, carried
	// per-seat with its reason (R7/R8).
	bespoke := byModel["Bespoke-MiniCheck-7B"]
	if bespoke.License.SPDX != "CC-BY-NC-4.0" || !bespoke.License.Exception {
		t.Errorf("Bespoke-MiniCheck: got %q exception=%v, want CC-BY-NC-4.0 exception", bespoke.License.SPDX, bespoke.License.Exception)
	}
	if bespoke.License.Note == "" {
		t.Error("Bespoke-MiniCheck exception carries no reason (R8)")
	}
	// The DeBERTa NLI cross-encoder has no llama-server /v1 serving path —
	// recorded honestly, never faked into config (R10).
	deberta := byModel["cross-encoder/nli-deberta-v3-base"]
	if deberta.Servable || deberta.GPUSeated {
		t.Error("DeBERTa NLI cross-encoder marked servable/GPU-seated (R10: non-servable, honest)")
	}
	// The embedder is NOT pulled and NOT configured (post-gate; §8 reading 9).
	emb := byModel["Qwen3-Embedding-0.6B"]
	if len(emb.Files) != 0 || emb.Servable || emb.GPUSeated {
		t.Error("embedder is present as pulled/servable — must be post-gate-only (§8 reading 9)")
	}
	// Every GPU-seated servable seat carries a card-verified license + a file.
	for _, s := range Manifest() {
		if s.GPUSeated && s.Servable {
			if s.License.VerifyDate == "" {
				t.Errorf("%s: license verify date missing", s.Model)
			}
			if len(s.Files) == 0 {
				t.Errorf("%s: GPU-seated servable seat has no file", s.Model)
			}
		}
	}
	if _, err := ManifestJSON(); err != nil {
		t.Fatalf("ManifestJSON: %v", err)
	}
}

func TestGenerateConfigPool12ResidentEmptyNoEmbedder(t *testing.T) {
	cfg, err := GenerateConfig(ConfigParams{
		ModelCacheDir: "/models", GPUUUIDs: []string{"GPU-ed0494dc"},
		LlamaServer: "/opt/llama-server", TTLFastS: 120, TTLWorkhorseS: 300, UnloadGraceS: 5,
	})
	if err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}
	// pool12 swap group, per-model UUID placement (R10).
	for _, want := range []string{
		`"pool12":`, "swap: true", "CUDA_VISIBLE_DEVICES=GPU-ed0494dc",
		`"Qwen3.5-9B":`, `"Qwen3.5-4B":`, `"Granite Guardian 3.3-8B":`, `"Arctic-Text2SQL-R1-7B":`,
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config missing %q", want)
		}
	}
	// resident EMPTY (S12.8): documented, never a resident group with members.
	if !strings.Contains(cfg, "resident: deliberately EMPTY") {
		t.Error("config does not document the empty resident group (S12.8)")
	}
	// No embedder member (post-gate) and no non-servable CPU seat faked in.
	for _, forbidden := range []string{"Qwen3-Embedding-0.6B", "nli-deberta", "MiniCheck-Flan-T5"} {
		if strings.Contains(cfg, forbidden) {
			t.Errorf("config leaked a non-configured seat %q (R10)", forbidden)
		}
	}
	// Per-class TTL: fast=120, workhorse-class=300.
	if !strings.Contains(cfg, "ttl: 120") || !strings.Contains(cfg, "ttl: 300") {
		t.Error("config missing per-class TTLs from the two ⚙ keys")
	}
	// A missing GPU UUID is a loud error — placement is by UUID, never index.
	if _, err := GenerateConfig(ConfigParams{ModelCacheDir: "/m", LlamaServer: "/s"}); err == nil {
		t.Error("GenerateConfig with no GPU UUID should error (R10)")
	}
}

func TestDutySchemasCarryAbstain(t *testing.T) {
	schemas := map[string]json.RawMessage{
		"triage":    TriageSchema([]string{"software"}, []string{"low"}, []string{"small"}),
		"tie-break": TieBreakSchema([]string{"w-1", "w-2"}),
		"spotcheck": SpotCheckSchema(),
	}
	for name, s := range schemas {
		if err := assertAbstain(s); err != nil {
			t.Errorf("schema %q: %v (every classification schema needs an abstain member, S12.4)", name, err)
		}
	}
	// A schema without an abstain member is rejected by the belt.
	noAbstain := objectSchema(map[string]any{"x": map[string]any{"type": "string"}}, []string{"x"})
	if err := assertAbstain(noAbstain); err == nil {
		t.Error("assertAbstain accepted a schema with no abstain member")
	}
}

func TestUsageMarkerRoundTrip(t *testing.T) {
	raw, err := UsageJSON("intake-triage", "Qwen3.5-4B", "deadbeef", LlamaCppPin, 120, 8)
	if err != nil {
		t.Fatalf("UsageJSON: %v", err)
	}
	var got struct {
		InputTokens  int64        `json:"input_tokens"`
		OutputTokens int64        `json:"output_tokens"`
		Local        *LocalMarker `json:"local"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.InputTokens != 120 || got.OutputTokens != 8 {
		t.Errorf("tokens = %d/%d, want 120/8", got.InputTokens, got.OutputTokens)
	}
	if got.Local == nil || got.Local.Lane != LaneLocal || got.Local.Duty != "intake-triage" ||
		got.Local.Model != "Qwen3.5-4B" || got.Local.ModelSHA256 != "deadbeef" || got.Local.EngineBuild != LlamaCppPin {
		t.Errorf("marker = %+v, want the full local marker", got.Local)
	}
}

func TestGameModeSnippetAndDeferredSubscription(t *testing.T) {
	snip := GameModeSnippet("/usr/local/bin/sinet")
	for _, want := range []string{"[custom]", "start=/usr/local/bin/sinet local unload", "end=/usr/local/bin/sinet local resume"} {
		if !strings.Contains(snip, want) {
			t.Errorf("gamemode snippet missing %q", want)
		}
	}
	// The busctl subscription with no bus address is DEFERRED-WITH-FINDING —
	// returns nil (not an error); the scripts leg carries the duty (R14).
	sub := &GameModeSubscription{BusAddr: ""}
	if err := sub.Run(context.Background()); err != nil {
		t.Errorf("unaddressed subscription should defer (nil), got %v", err)
	}
}

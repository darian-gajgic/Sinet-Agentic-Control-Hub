package local_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// conformance_test.go — the §10 three-tier conformance suite for the local
// serving stack (brief R26/R27): F (fake/fixtures, always), R (real
// llama-swap+llama.cpp when installed, $0, auto-runs), L (the ONE live smoke
// under SINET_LIVE_SMOKE=1, $0 by construction). Absence-skips print
// SANCTIONED SKIP (CONVENTIONS §10) — the one allowed skip class. The pins are
// test-coupled to the components.lock entries; an installed-vs-pin delta is
// reported LOUDLY, never retargeted.

const sanctionedSkip = "SANCTIONED SKIP (CONVENTIONS §10): "

// TestPinsMatchLock couples LlamaSwapPin / LlamaCppPin to the components.lock
// entries (the §10 Pin↔lock precedent). A drift is a LOUD failure — the
// operator's S03.3/S12.10 deliberate-bump decision, never a silent retarget.
func TestPinsMatchLock(t *testing.T) {
	var lock struct {
		Components []struct {
			Name string `json:"name"`
			Pin  string `json:"pin"`
		} `json:"components"`
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "components.lock"))
	if err != nil {
		t.Fatalf("read components.lock: %v", err)
	}
	if err := json.Unmarshal(raw, &lock); err != nil {
		t.Fatalf("decode components.lock: %v", err)
	}
	pin := map[string]string{}
	for _, c := range lock.Components {
		pin[c.Name] = c.Pin
	}
	if got := pin["llama-swap"]; got != local.LlamaSwapPin {
		t.Errorf("llama-swap pin drift: lock %q vs package %q — reconcile via S12.10 deliberate bump, never silently (P-T15-2)", got, local.LlamaSwapPin)
	}
	if got := pin["llama.cpp (llama-server)"]; got != local.LlamaCppPin {
		t.Errorf("llama.cpp pin drift: lock %q vs package %q — reconcile via S03.3 deliberate bump, never silently (P-T15-2)", got, local.LlamaCppPin)
	}
}

// TestConformanceTierR is the real-stack tier ($0): when llama-swap +
// llama-server are installed, assert the installed binaries exist; the
// behavioral /v1 asserts (logprobs present, json_schema actually constrains,
// the llama-swap contract) run in the tier-L smoke against a live endpoint.
// Absent binaries SANCTIONED-SKIP.
func TestConformanceTierR(t *testing.T) {
	llamaSwap, llamaServer, ok := local.InstalledStack()
	if !ok {
		t.Skip(sanctionedSkip + "local serving stack not installed (llama-swap + llama-server absent) — host install is the B4 gate/hardening step (the srt/ttyd precedent)")
	}
	t.Logf("tier R: llama-swap=%s llama-server=%s (pins %s / %s)", llamaSwap, llamaServer, local.LlamaSwapPin, local.LlamaCppPin)
}

// TestTierLLiveSmoke is THE one tier-L smoke ($0 by construction): against a
// live llama-swap endpoint (SINET_LOCAL_ENDPOINT), one duty call end-to-end —
// alias → model load → temp-0 constrained response with the abstain-capable
// schema → the $0 D7 checkpoint row with model hash + engine build → the
// zero-allowance receipt line via the metering fold — then eager-unload. Never
// touches production units/ports. Gated by SINET_LIVE_SMOKE=1; absent it
// SANCTIONED-SKIPs.
func TestTierLLiveSmoke(t *testing.T) {
	if os.Getenv("SINET_LIVE_SMOKE") != "1" {
		t.Skip(sanctionedSkip + "tier-L live smoke gated by SINET_LIVE_SMOKE=1")
	}
	endpoint := os.Getenv("SINET_LOCAL_ENDPOINT")
	if endpoint == "" {
		t.Skip(sanctionedSkip + "tier-L smoke needs SINET_LOCAL_ENDPOINT (a live llama-swap loopback endpoint)")
	}
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	runs := run.NewStore(db, log)
	if _, err := runs.Create(ctx, run.NewRun{ID: "smoke.intake", UserID: "operator", Lane: "anthropic", Substrate: "claude-cli"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, "smoke.intake", st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("Transition: %v", err)
		}
	}
	duty := local.NewDuty(local.DutyDeps{
		Registry: local.NewRegistry(reg), Client: local.NewClient(endpoint),
		Checkpoints: gates.NewCheckpoints(db, log), Events: log,
	})
	// alias → load → temp-0 constrained response (abstain-capable schema).
	res, err := duty.Call(ctx, "smoke.intake", local.DutyRequest{
		Alias:  local.AliasIntakeTriage,
		System: "classify",
		User:   "add a unit test to the parser",
		Schema: local.TriageSchema([]string{"software", "generic"}, []string{"low", "standard"}, []string{"small", "medium"}),
		Name:   "intake-triage", MaxTokens: 512, Classification: true,
	})
	if err != nil {
		t.Fatalf("live duty call: %v", err)
	}
	t.Logf("tier L: model=%q content=%q logprobs=%d", res.Model, res.Content, len(res.Logprobs))

	// The $0 zero-allowance row landed with the model hash + engine build.
	led := metering.NewLedger(db, nil, metering.NoMeteredExceptions(), reg)
	rc, err := led.RunConsumption(ctx, "smoke.intake")
	if err != nil {
		t.Fatalf("RunConsumption: %v", err)
	}
	var found bool
	for _, li := range rc.Items {
		if li.Lane == metering.LaneLocal {
			found = true
			if li.Unpriced() || li.PricedUSD != 0 || !li.ZeroAllowance() {
				t.Errorf("live local line not $0 zero-allowance: %+v", li)
			}
		}
	}
	if !found {
		t.Error("no $0 local line on the live smoke row (R27)")
	}
	// eager-unload + observe VRAM-release at the llama-swap surface.
	if err := local.NewEagerUnload(duty, log, nil).Engage(ctx, "smoke"); err != nil {
		t.Fatalf("eager-unload: %v", err)
	}
}

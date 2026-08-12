package stage_test

// phrase_test.go — the P3-RW-12 §7 T9b/T12 legs: the S06.5
// phrase-and-summarize duty on the utility alias (Spec S06.10 duty row,
// S12.4), its ONE $0 D7 row on the consuming run, and the tier-L live smoke.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

func phraseInput(runID string) intake.PhraseInput {
	return intake.PhraseInput{
		RunID:   runID,
		Request: intake.Request{TaskID: "t1", Title: "webshop", Text: "Create a simple webshop for car parts"},
		Family:  intake.FamilySoftware,
		Tier:    intake.TierStandard,
		Questions: []intake.PhraseQuestion{
			{ID: "behavior", Text: "What exactly should this do — what behavior or outcome makes it correct?"},
			{ID: "technology_stack", Text: "What should this be built with?"},
		},
		Understood: []intake.UnderstoodItem{
			{SlotID: "units", Name: "Units", How: intake.ResolvedRegistry, Value: "millimetres"},
		},
	}
}

// usageJSON reads the one checkpoint row's usage block on a run.
func usageJSON(t *testing.T, db *storage.DB, runID string) string {
	t.Helper()
	var raw string
	if err := db.QueryRowContext(context.Background(),
		`SELECT usage_json FROM checkpoints WHERE run_id = ? ORDER BY checkpoint_id DESC LIMIT 1`, runID).
		Scan(&raw); err != nil {
		t.Fatalf("read usage for %s: %v", runID, err)
	}
	return raw
}

// TestPhraseAdapterMetersOnConsumingRun (§7 T9b; R7): the utility seat holds
// the phrase duty as well as Help, the phrasings come back keyed by the slot
// ids the caller asked for, and the call writes exactly ONE $0 D7 row — on
// the run the caller named, marked with the utility duty.
func TestPhraseAdapterMetersOnConsumingRun(t *testing.T) {
	ctx := context.Background()
	duty, fake, db, runs := localSeamEnv(t)
	runningRun(t, runs, "t1.intake.g2") // the fork the pipeline is driving now

	seat, ok := stage.NewLocalUtility(duty).(intake.Phraser)
	if !ok {
		t.Fatal("the utility seat does not hold the phrase-and-summarize duty (S06.10 assigns them to one seat)")
	}
	// utility → workhorse → Qwen3.5-9B.
	fake.SetModelResponse("Qwen3.5-9B", local.FakeResponse{
		Content: `{"reason":"r","summary":"You want a small shop that sells car parts.",` +
			`"behavior":"What should the shop actually do?","technology_stack":"What should we build it with?"}`,
		InputTokens: 120, OutputTokens: 40,
	})

	res, err := seat.PhraseAndSummarize(ctx, phraseInput("t1.intake.g2"))
	if err != nil {
		t.Fatalf("PhraseAndSummarize: %v", err)
	}
	if res.Summary != "You want a small shop that sells car parts." {
		t.Errorf("summary = %q", res.Summary)
	}
	if res.Phrasings["behavior"] != "What should the shop actually do?" {
		t.Errorf("behavior phrasing = %q", res.Phrasings["behavior"])
	}
	if res.Phrasings["technology_stack"] != "What should we build it with?" {
		t.Errorf("technology_stack phrasing = %q", res.Phrasings["technology_stack"])
	}
	if _, leaked := res.Phrasings["reason"]; leaked {
		t.Error("the schema's reason field leaked into the phrasings")
	}
	if _, leaked := res.Phrasings["summary"]; leaked {
		t.Error("the summary leaked into the phrasings")
	}

	if n := cpCount(t, db, "t1.intake.g2"); n != 1 {
		t.Errorf("D7 rows on the consuming run = %d, want exactly 1 (OQ2: one call per card)", n)
	}
	if n := cpCount(t, db, "t1.intake"); n != 0 {
		t.Errorf("%d rows landed on the superseded parent — the call must ride the passed run", n)
	}
	if u := usageJSON(t, db, "t1.intake.g2"); !strings.Contains(u, `"duty":"utility"`) || !strings.Contains(u, `"lane":"local"`) {
		t.Errorf("usage marker = %s, want the local/utility marker (§26 wire contract)", u)
	}

	// A run in no checkpointable state is a SURFACED defect, never a quietly
	// unmetered call (§26): the caller errors and the seam degrades.
	fake.SetModelResponse("Qwen3.5-9B", local.FakeResponse{
		Content: `{"reason":"r","summary":"s","behavior":"b","technology_stack":"t"}`, InputTokens: 10, OutputTokens: 2,
	})
	if _, err := seat.PhraseAndSummarize(ctx, phraseInput("no-such-run")); err == nil {
		t.Error("a call naming a run with no checkpointable state must surface loudly")
	}
}

// workhorseSeatPresent reports whether the seat the `utility` alias resolves
// to has its weights on disk. A missing GGUF is an absent stack, not a failure.
func workhorseSeatPresent(modelCache string) bool {
	for _, s := range local.Manifest() {
		if s.Seat != "workhorse" || !s.Pulled || len(s.Files) == 0 {
			continue
		}
		if _, err := os.Stat(filepath.Join(modelCache, s.Files[0].Name)); err == nil {
			return true
		}
	}
	return false
}

// TestLivePhraseAndSummarize (§7 T12, rebuilt at drain r1): the REAL utility
// seat phrases a REAL interview card against a REAL llama-swap on an ephemeral
// loopback port, with a config generated from today's manifest. $0 by
// construction (lane `local` prices a true zero-allowance, §26/R18).
//
// It AUTO-RUNS where the stack is installed and SANCTIONED-SKIPS where it is
// not — the RW-11 T11 pattern, deliberately NOT env-gated. The first cut of
// this leg was gated behind SINET_LIVE_SMOKE and therefore never ran, and what
// it would have caught is exactly what the drain caught instead: the phrase
// call is a DRAFTING duty, so the reasoning workhorse keeps its think phase,
// and at the original 1000-token cap the think phase consumed the whole budget
// before the constrained region ever started — content length 0, every time, on
// every card. A seam that is only exercised by a fake is a seam nobody has
// run.
//
// So the assertion that matters here is the tripwire: a NON-EMPTY Phrased for
// every id the card asked about. Everything else about the wording is model
// output and is checked for shape only.
func TestLivePhraseAndSummarize(t *testing.T) {
	llamaSwap, llamaServer, modelCache, ok := installedStack()
	if !ok {
		t.Skip(liveSanctionedSkip + "local serving stack not installed (llama-swap + llama-server + model cache absent) — host install is the B4 gate/hardening step")
	}
	if !workhorseSeatPresent(modelCache) {
		t.Skip(liveSanctionedSkip + "the utility seat's `workhorse` weights are not in the model cache")
	}

	ctx := context.Background()
	cfg, err := local.GenerateConfig(local.ConfigParams{
		ModelCacheDir: modelCache, GPUUUIDs: liveGPUUUIDs(t), LlamaServer: llamaServer,
		TTLFastS: 120, TTLWorkhorseS: 300, UnloadGraceS: 5,
	})
	if err != nil {
		t.Fatalf("GenerateConfig from today's manifest: %v", err)
	}
	cfgPath := filepath.Join(t.TempDir(), "llamaswap.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	port := liveFreePort(t)
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	cmd := exec.Command(llamaSwap, "--config", cfgPath, "--listen", fmt.Sprintf("127.0.0.1:%d", port))
	if err := cmd.Start(); err != nil {
		t.Fatalf("start llama-swap: %v", err)
	}
	// Unload BEFORE killing the manager: killing it orphans the llama-server
	// child still holding its VRAM, and the next live leg then fails to load
	// (measured at RW-11).
	defer func() {
		if resp, err := http.Post(base+"/api/models/unload", "application/json", nil); err == nil {
			resp.Body.Close()
		}
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	if !liveWaitReady(base + "/v1/models") {
		t.Fatal("llama-swap did not accept the generated config / did not become ready")
	}

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
	const runID = "t-webshop.intake"
	runningRun(t, runs, runID)

	duty := local.NewDuty(local.DutyDeps{
		Registry: local.NewRegistry(reg), Client: local.NewClient(base),
		Checkpoints: gates.NewCheckpoints(db, log), Events: log,
	})
	seat, ok := stage.NewLocalUtility(duty).(intake.Phraser)
	if !ok {
		t.Fatal("the utility seat does not hold the phrase-and-summarize duty (S06.10 assigns them to one seat)")
	}

	// The operator's own checkpoint-3 request, drawn from the shipping software
	// question set exactly as buildInterviewCard would offer it.
	soft := intake.SeedTaxonomies()[intake.FamilySoftware]
	in := intake.PhraseInput{
		RunID: runID,
		Request: intake.Request{
			TaskID: "t-webshop", UserID: "operator",
			Title: "Create a simple webshop for car parts",
			Text:  "Create a simple webshop for car parts, with a product list, a cart and a checkout page.",
		},
		Family: intake.FamilySoftware, Tier: intake.TierStandard,
	}
	for _, s := range soft.Unresolved(nil) {
		if len(in.Questions) == 4 { // maxQuestionsPerCard
			break
		}
		in.Questions = append(in.Questions, intake.PhraseQuestion{ID: s.ID, Text: s.Question})
	}

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// COLD: the model is not resident, so this includes llama-swap's load.
	coldStart := time.Now()
	res, err := seat.PhraseAndSummarize(callCtx, in)
	cold := time.Since(coldStart)
	if err != nil {
		t.Fatalf("live PhraseAndSummarize (cold): %v", err)
	}
	for _, q := range in.Questions {
		got := strings.TrimSpace(res.Phrasings[q.ID])
		if got == "" {
			t.Errorf("no phrasing came back for %q — the card would ship the taxonomy's own words, "+
				"which is the honest degrade but means the seat is not working (drain r1 F1)", q.ID)
			continue
		}
		t.Logf("phrased %-20s %s", q.ID, got)
	}
	if strings.TrimSpace(res.Summary) == "" {
		t.Error("live run returned an empty summary")
	}
	if len(res.Phrasings) != len(in.Questions) {
		t.Errorf("got %d phrasings for %d asked questions — the fold drops the rest, but the seat should answer them all",
			len(res.Phrasings), len(in.Questions))
	}

	// WARM: the model is resident. This is the cost a requester actually pays
	// per card once the stack is up, and it is SYNCHRONOUS in the card build.
	warmStart := time.Now()
	if _, err := seat.PhraseAndSummarize(callCtx, in); err != nil {
		t.Fatalf("live PhraseAndSummarize (warm): %v", err)
	}
	warm := time.Since(warmStart)
	t.Logf("LATENCY per card: cold %s (includes model load), warm %s — synchronous in the interview card build", cold, warm)

	// ONE $0 D7 row per call, on the consuming run, with the local marker.
	if n := cpCount(t, db, runID); n != 2 {
		t.Errorf("D7 rows = %d after two calls, want exactly one per call (§26 R18)", n)
	}
	var usage string
	if err := db.QueryRowContext(ctx,
		`SELECT usage_json FROM checkpoints WHERE run_id = ? ORDER BY event_seq DESC LIMIT 1`, runID).Scan(&usage); err != nil {
		t.Fatalf("read the $0 D7 checkpoint row: %v", err)
	}
	var w struct {
		Local *struct {
			Lane        string `json:"lane"`
			Duty        string `json:"duty"`
			Model       string `json:"model"`
			ModelSHA256 string `json:"model_sha256"`
			EngineBuild string `json:"engine_build"`
		} `json:"local"`
	}
	if err := json.Unmarshal([]byte(usage), &w); err != nil {
		t.Fatalf("decode usage block: %v: %s", err, usage)
	}
	if w.Local == nil {
		t.Fatalf("no local marker on the D7 row: %s", usage)
	}
	if w.Local.Lane != "local" || w.Local.Duty != "utility" || w.Local.EngineBuild == "" {
		t.Errorf("local marker = %+v, want the utility duty on the local lane with its engine build", *w.Local)
	}
	var defects int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM run_events WHERE type = ?`, local.EventLocalUnmeteredDefect).Scan(&defects); err != nil {
		t.Fatalf("count unmetered-defect events: %v", err)
	}
	if defects != 0 {
		t.Errorf("%d unmetered-defect events on the happy path, want 0 (R12)", defects)
	}
}

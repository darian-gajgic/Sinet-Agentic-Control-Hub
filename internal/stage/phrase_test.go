package stage_test

// phrase_test.go — the P3-RW-12 §7 T9b/T12 legs: the S06.5
// phrase-and-summarize duty on the utility alias (Spec S06.10 duty row,
// S12.4), its ONE $0 D7 row on the consuming run, and the tier-L live smoke.

import (
	"context"
	"encoding/json"
	"os"
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

// liveCapHeadroomPct is how much of a duty's length cap must still be UNSPENT
// when a realistic call finishes: the reply may use at most half the budget.
//
// Why half, and not "under the cap": PH-1 shipped because a call that fits
// exactly is indistinguishable from one that fits by luck, and the thing that
// grows — think phase, request size, question count — grows continuously while
// the cap does not. A working phrase call emits roughly 350–500 tokens of JSON
// for a full four-question card, so half of a 4000-token cap leaves room for a
// card several times larger than any the pipeline can build
// (maxQuestionsPerCard is 4) before this fires. Anything above that means the
// budget is being consumed by something other than the schema region, which is
// the defect, and it should fail in a test rather than on a requester's card.
const liveCapHeadroomPct = 50

// liveUsageTokens reads the input/output token counts off the newest D7 row on
// a run — the same durable field that diagnosed PH-1 (output_tokens equal to
// the cap, to the token).
func liveUsageTokens(t *testing.T, db *storage.DB, runID string) (in, out int64) {
	t.Helper()
	var raw string
	if err := db.QueryRowContext(context.Background(),
		`SELECT usage_json FROM checkpoints WHERE run_id = ? ORDER BY event_seq DESC LIMIT 1`, runID).Scan(&raw); err != nil {
		t.Fatalf("read usage tokens for %s: %v", runID, err)
	}
	var u struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	}
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatalf("decode usage tokens: %v: %s", err, raw)
	}
	return u.InputTokens, u.OutputTokens
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

// TestLivePhraseAndSummarize (§7 T12, rebuilt at drain r1, hardened at PH-1):
// the REAL utility seat phrases a REAL full-size interview card against a REAL
// llama-swap — the operator's running stack when one is declared, otherwise its
// own manager on an ephemeral port from today's manifest. $0 by construction
// (lane `local` prices a true zero-allowance, §26/R18).
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
	base := liveStackBase(t, llamaSwap, llamaServer, modelCache)

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

	// A card of the size a REAL walk produces, not a token one. Cold walk 1's
	// phrase call carried 598 input tokens — a full four-question card with an
	// understanding block behind it — and the leg that was supposed to protect
	// that path passed on a much smaller request. A tripwire sized below the
	// traffic it guards tests a point, not the margin (PH-1 F4).
	soft := intake.SeedTaxonomies()[intake.FamilySoftware]
	in := intake.PhraseInput{
		RunID: runID,
		Request: intake.Request{
			TaskID: "t-webshop", UserID: "operator",
			Title: "Create a simple webshop for car parts",
			Text: "Create a simple webshop for car parts, with a product list, a cart and a checkout page. " +
				"Parts should be searchable by the car they fit — make, model and year — and each part needs a photo, " +
				"a part number, a price and whether it is in stock. The cart should survive a page reload, and checkout " +
				"should collect a delivery address and show shipping cost before the customer confirms. I want to be able " +
				"to add and edit parts myself without touching code, including bulk-uploading a price list from a " +
				"spreadsheet. It should look clean and load fast on a phone, since most of my customers browse from the " +
				"workshop floor. Payment can wait for a later version — for now an order confirmation email to me and to " +
				"the customer is enough. This replaces the paper catalogue I hand out at the counter, so the part numbers " +
				"have to match the ones printed there exactly.",
		},
		Family: intake.FamilySoftware, Tier: intake.TierStandard,
		Understood: []intake.UnderstoodItem{
			{SlotID: "units", Name: "Units", How: intake.ResolvedRegistry, Value: "millimetres"},
			{SlotID: "deploy_target", Name: "Where it runs", How: intake.ResolvedRegistry, Value: "the shop's own small VPS"},
			{SlotID: "audience", Name: "Who uses it", How: intake.ResolvedAssumption, Assumption: "walk-in customers and the counter staff"},
		},
	}
	for _, s := range soft.Unresolved(nil) {
		if len(in.Questions) == 4 { // maxQuestionsPerCard — a full card, as a walk builds it
			break
		}
		in.Questions = append(in.Questions, intake.PhraseQuestion{ID: s.ID, Text: s.Question})
	}
	if len(in.Questions) != 4 {
		t.Fatalf("the live card carries %d questions, want the full 4 — a smaller card is a smaller test", len(in.Questions))
	}

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// COLD: the model may not be resident, so this can include llama-swap's load.
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
	t.Logf("summary: %s", strings.TrimSpace(res.Summary))
	if len(res.Phrasings) != len(in.Questions) {
		t.Errorf("got %d phrasings for %d asked questions — the fold drops the rest, but the seat should answer them all",
			len(res.Phrasings), len(in.Questions))
	}

	// THE MARGIN, not the point. The D7 row is the durable record of what the
	// call actually spent, and it is the same field that diagnosed PH-1 —
	// output_tokens sitting exactly on the cap was the whole tell.
	//
	// Two assertions, in opposite directions:
	//   • the request must be at least as big as the traffic (≥ the walk's 598
	//     input tokens), so this leg can never quietly shrink back into a point;
	//   • the reply must land with at least liveCapHeadroomPct headroom under
	//     the cap, so "it fit, barely" fails HERE instead of on a requester's
	//     card three cards later.
	inTok, outTok := liveUsageTokens(t, db, runID)
	if inTok < 598 {
		t.Errorf("live phrase request was %d input tokens, want ≥598 — the size cold walk 1 actually sent (PH-1 F4)", inTok)
	}
	if ceiling := int64(stage.PhraseMaxTokens) * (100 - liveCapHeadroomPct) / 100; outTok > ceiling {
		t.Errorf("live phrase reply spent %d of the %d-token cap (ceiling %d, %d%% headroom): the budget is no longer "+
			"comfortable and the next slightly larger card truncates — that is exactly how PH-1 shipped",
			outTok, stage.PhraseMaxTokens, ceiling, liveCapHeadroomPct)
	}
	t.Logf("CAP MARGIN: %d in / %d out against a %d cap — %.1f%% of the budget used, %.1f%% headroom",
		inTok, outTok, stage.PhraseMaxTokens,
		100*float64(outTok)/float64(stage.PhraseMaxTokens), 100-100*float64(outTok)/float64(stage.PhraseMaxTokens))

	// WARM: the model is resident. This is the cost a requester actually pays
	// per card once the stack is up, and it is SYNCHRONOUS in the card build.
	warmStart := time.Now()
	if _, err := seat.PhraseAndSummarize(callCtx, in); err != nil {
		t.Fatalf("live PhraseAndSummarize (warm): %v", err)
	}
	warm := time.Since(warmStart)
	t.Logf("LATENCY per card: cold %s (includes model load), warm %s — synchronous in the interview card build", cold, warm)

	// The HELP duty, on the same seat and the same alias, failed identically on
	// cold walk 1 (cp 13: 700 out of a 700 cap, byte-identical fallback to
	// defaultHelp) and had no live leg at all. It has one now — the same
	// think-phase fix has to hold for both drafting duties or only half the seat
	// works (PH-1 F1/F4).
	helpPair := intake.Pair{Spec: intake.Spec{
		TaskID:      "t-webshop",
		Restatement: "Build a small webshop for car parts with a searchable product list, a cart that survives reload, and a checkout that collects a delivery address.",
		Outcome:     []string{"customers can find a part by car make/model/year", "an order confirmation reaches both the shop and the customer"},
	}}
	helpPair.Plan.Steps = []intake.Step{{Title: "model the parts catalogue"}, {Title: "build the product list and search"}, {Title: "build the cart and checkout"}}
	utility, ok := seat.(intake.Utility)
	if !ok {
		t.Fatal("the utility seat does not hold the 13.5 help duty (S06.10 assigns them to one seat)")
	}
	help, err := utility.Help(callCtx, helpPair)
	if err != nil {
		t.Fatalf("live Help: %v", err)
	}
	if strings.TrimSpace(help.What) == "" || strings.TrimSpace(help.Recommend) == "" {
		t.Errorf("live help block came back empty (what=%q recommend=%q) — the approval card would ship deterministic text",
			help.What, help.Recommend)
	}
	t.Logf("help.what: %s", strings.TrimSpace(help.What))
	_, helpOut := liveUsageTokens(t, db, runID)
	if ceiling := int64(stage.HelpMaxTokens) * (100 - liveCapHeadroomPct) / 100; helpOut > ceiling {
		t.Errorf("live help reply spent %d of the %d-token cap (ceiling %d) — no headroom left (PH-1 F4)",
			helpOut, stage.HelpMaxTokens, ceiling)
	}
	t.Logf("CAP MARGIN (help): %d out against a %d cap — %.1f%% headroom",
		helpOut, stage.HelpMaxTokens, 100-100*float64(helpOut)/float64(stage.HelpMaxTokens))

	// ONE $0 D7 row per call, on the consuming run, with the local marker.
	if n := cpCount(t, db, runID); n != 3 {
		t.Errorf("D7 rows = %d after three calls, want exactly one per call (§26 R18)", n)
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

package stage_test

// gf3live_test.go — the P3-GF3-BE1 extended-output live measurement leg
// (brief §8 T6b, second half; design note §2.B "PH-1 discipline").
//
// What it measures, and why it is separate from the landed tripwire: the
// landed TestLivePhraseAndSummarize auto-runs and guards the DELIVERY card —
// four questions, the size a normal walk sends. This packet adds two things
// that leg cannot see: a suggestion per question, and the S06.9 review card
// carrying every slot of a family at once. That is the biggest phrase call the
// pipeline can now build, and its cost is a number the operator owns rather
// than an assertion, so it runs only when asked for.
//
// OPT-IN, by its own flag and nothing inherited (§59's rule for legs that cost
// something — here time and a resident GPU model, never money: the local lane
// prices a true zero). Run it serially, on a host with the stack installed:
//
//	SINET_LIVE_REVIEW_CARD=1 go test -p 1 -run TestLiveReviewCardPhraseHeadroom ./internal/stage/
//
// It writes what it measured to P3/measurements/, because a number recorded in
// a test log is a number nobody can find again.

import (
	"context"
	"fmt"
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

// liveReviewCardEnv is this leg's OWN flag. Nothing else turns it on: a leg
// that starts because some other switch happened to be set is a leg that runs
// when nobody meant it to (§59).
const liveReviewCardEnv = "SINET_LIVE_REVIEW_CARD"

// TestLiveReviewCardPhraseHeadroom drives the real utility seat with the
// largest card the pipeline can build — every software slot, options included,
// suggestions requested — and records what it cost.
func TestLiveReviewCardPhraseHeadroom(t *testing.T) {
	if os.Getenv(liveReviewCardEnv) != "1" {
		t.Skip(liveSanctionedSkip + "the extended-output live measurement runs only under " + liveReviewCardEnv + "=1 (GPU stack, serial)")
	}
	llamaSwap, llamaServer, modelCache, ok := installedStack()
	if !ok {
		t.Skip(liveSanctionedSkip + "local serving stack not installed (llama-swap + llama-server + model cache absent)")
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
	const runID = "t-webshop-review.intake"
	runningRun(t, runs, runID)

	duty := local.NewDuty(local.DutyDeps{
		Registry: local.NewRegistry(reg), Client: local.NewClient(base),
		Checkpoints: gates.NewCheckpoints(db, log), Events: log,
	})
	seat, ok := stage.NewLocalUtility(duty).(intake.Phraser)
	if !ok {
		t.Fatal("the utility seat does not hold the phrase-and-summarize duty (S06.10)")
	}

	// The review card: every slot of the software family, with its options —
	// the S06.9 re-interview surface, which is the largest phrase call there is.
	soft := intake.SeedTaxonomies()[intake.FamilySoftware]
	in := intake.PhraseInput{
		RunID: runID,
		Request: intake.Request{
			TaskID: "t-webshop-review", UserID: "operator",
			Title: "Create a simple webshop for car parts",
			Text: "Create a simple webshop for car parts, with a product list, a cart and a checkout page. " +
				"Parts should be searchable by the car they fit (make, model and year) and each part needs a photo, " +
				"a part number, a price and whether it is in stock. I want to add and edit parts myself without touching " +
				"code, including bulk-uploading a price list from a spreadsheet. It should look clean and load fast on a " +
				"phone, since most of my customers browse from the workshop floor. This replaces the paper catalogue I hand " +
				"out at the counter, so the part numbers have to match the ones printed there exactly.",
		},
		Family: intake.FamilySoftware, Tier: intake.TierStandard,
		Understood: []intake.UnderstoodItem{
			{SlotID: "units", Name: "Units and measurements", How: intake.ResolvedRegistry, Value: "millimetres"},
			{SlotID: "look_feel", Name: "Look and feel", How: intake.ResolvedAnswered, Value: "plain and clean"},
		},
	}
	for _, s := range soft.Slots {
		in.Questions = append(in.Questions, intake.PhraseQuestion{ID: s.ID, Text: s.Question, Options: s.Options})
	}
	if len(in.Questions) != len(soft.Slots) {
		t.Fatalf("the review card carries %d of %d slots", len(in.Questions), len(soft.Slots))
	}

	callCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()

	coldStart := time.Now()
	res, err := seat.PhraseAndSummarize(callCtx, in)
	cold := time.Since(coldStart)
	if err != nil {
		t.Fatalf("live review-card PhraseAndSummarize (cold): %v", err)
	}

	var missingPhrasing, missingSuggestion, badOption []string
	for _, q := range in.Questions {
		if strings.TrimSpace(res.Phrasings[q.ID]) == "" {
			missingPhrasing = append(missingPhrasing, q.ID)
		}
		if strings.TrimSpace(res.Suggestions[q.ID]) == "" {
			missingSuggestion = append(missingSuggestion, q.ID)
		}
		if v, ok := res.SuggestedOptions[q.ID]; ok && v != "" {
			known := false
			for _, o := range q.Options {
				if o.Value == v {
					known = true
				}
			}
			if !known {
				badOption = append(badOption, q.ID+"="+v)
			}
		}
		t.Logf("%-20s phrased=%q suggested=%q option=%q", q.ID,
			res.Phrasings[q.ID], res.Suggestions[q.ID], res.SuggestedOptions[q.ID])
	}
	if len(missingPhrasing) > 0 {
		t.Errorf("no phrasing for %v — the whole review card would ship taxonomy wording (the PH-1 symptom)", missingPhrasing)
	}
	if len(missingSuggestion) > 0 {
		t.Errorf("no suggestion for %v — the one-click recommendation is the point of the extended output", missingSuggestion)
	}
	if len(badOption) > 0 {
		// The pipeline's fold drops these, so this is a schema observation, not
		// a containment failure: the engine enum should have made it impossible.
		t.Errorf("suggested option values that name no option of their question: %v", badOption)
	}

	inTok, outTok := liveUsageTokens(t, db, runID)
	budget := int64(stage.PhraseBudgetForTest(len(in.Questions)))
	if ceiling := budget * (100 - liveCapHeadroomPct) / 100; outTok > ceiling {
		t.Errorf("the review card spent %d of its %d-token budget (ceiling %d): the derivation is no longer comfortable",
			outTok, budget, ceiling)
	}

	warmStart := time.Now()
	if _, err := seat.PhraseAndSummarize(callCtx, in); err != nil {
		t.Fatalf("live review-card PhraseAndSummarize (warm): %v", err)
	}
	warm := time.Since(warmStart)

	headroom := 100 - 100*float64(outTok)/float64(budget)
	t.Logf("REVIEW CARD: %d questions, %d in / %d out against a %d budget (%.1f%% headroom); cold %s, warm %s",
		len(in.Questions), inTok, outTok, budget, headroom, cold, warm)

	record := fmt.Sprintf(`# Review-card phrase call — extended output headroom (P3-GF3-BE1)

Measured %s on the ratified local stack, %s=1, serial. $0: the local lane
prices a true zero-allowance (§26).

| What | Value |
|---|---|
| Card | S06.9 review card, software family, every slot with its options |
| Questions | %d |
| Input tokens | %d |
| Output tokens | %d |
| Budget (derived, phraseBudget) | %d |
| Headroom | %.1f%% |
| Cold wall time | %s |
| Warm wall time | %s |
| Phrasings returned | %d of %d |
| Suggestions returned | %d of %d |

The delivery card's own budget is unchanged at 4000 (PH-1's measured number);
this leg exists because the review card is several times larger and its cost is
the operator's to know, not a test's to assert alone.
`, time.Now().UTC().Format("2006-01-02"), liveReviewCardEnv,
		len(in.Questions), inTok, outTok, budget, headroom, cold, warm,
		len(res.Phrasings), len(in.Questions), len(res.Suggestions), len(in.Questions))

	path := filepath.Join("..", "..", "P3", "measurements",
		time.Now().UTC().Format("2006-01-02")+"-gf3-review-card-phrase-headroom.md")
	if err := os.WriteFile(path, []byte(record), 0o644); err != nil {
		t.Errorf("write measurement record: %v", err)
		return
	}
	t.Logf("measurement written to %s", path)
}

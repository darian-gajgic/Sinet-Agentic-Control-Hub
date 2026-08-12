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

// TestPhraseAdapterLiveSmoke (§7 T12; the §26 tier-L pattern, $0): the real
// stack phrases a real card. Shape-only assertions — the content is model
// output — plus the $0 D7 row with its model hash and engine build.
func TestPhraseAdapterLiveSmoke(t *testing.T) {
	if os.Getenv("SINET_LIVE_SMOKE") != "1" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §26 tier L): live smoke gated by SINET_LIVE_SMOKE=1")
	}
	endpoint := os.Getenv("SINET_LOCAL_ENDPOINT")
	if endpoint == "" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §26 tier L): needs SINET_LOCAL_ENDPOINT (a live llama-swap loopback endpoint)")
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
	runningRun(t, runs, "t1.intake")
	duty := local.NewDuty(local.DutyDeps{
		Registry: local.NewRegistry(reg), Client: local.NewClient(endpoint),
		Checkpoints: gates.NewCheckpoints(db, log), Events: log,
	})
	seat, ok := stage.NewLocalUtility(duty).(intake.Phraser)
	if !ok {
		t.Fatal("the utility seat does not hold the phrase duty")
	}
	in := phraseInput("t1.intake")
	res, err := seat.PhraseAndSummarize(ctx, in)
	if err != nil {
		t.Fatalf("live PhraseAndSummarize: %v", err)
	}
	for _, q := range in.Questions {
		if strings.TrimSpace(res.Phrasings[q.ID]) == "" {
			t.Errorf("live run returned no phrasing for %q", q.ID)
		}
	}
	if strings.TrimSpace(res.Summary) == "" {
		t.Error("live run returned an empty summary")
	}
	if n := cpCount(t, db, "t1.intake"); n != 1 {
		t.Errorf("live D7 rows = %d, want 1", n)
	}
	var u struct {
		Local struct {
			Model, ModelSHA256, EngineBuild string
		} `json:"local"`
	}
	if err := json.Unmarshal([]byte(usageJSON(t, db, "t1.intake")), &u); err != nil {
		t.Fatalf("decode live usage: %v", err)
	}
	if u.Local.Model == "" || u.Local.EngineBuild == "" {
		t.Errorf("live $0 row missing model/engine provenance: %+v", u.Local)
	}
}

package watchlist_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/lockfile"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/watchlist"
)

func seedByID(t *testing.T) map[string]watchlist.Row {
	t.Helper()
	out := map[string]watchlist.Row{}
	for _, r := range watchlist.SeedRows() {
		if _, dup := out[r.ID]; dup {
			t.Fatalf("duplicate seed row id %q", r.ID)
		}
		out[r.ID] = r
	}
	return out
}

// TestEverySeedRowValidatesAndCitesItsSource (R3): every row passes structural
// validation and carries its ratifying source in notes.
func TestEverySeedRowValidatesAndCitesItsSource(t *testing.T) {
	for _, r := range watchlist.SeedRows() {
		if err := r.Validate(); err != nil {
			t.Errorf("seed row %q invalid: %v", r.ID, err)
		}
		if len(strings.TrimSpace(r.Notes)) < 20 {
			t.Errorf("seed row %q carries no meaningful ratifying citation in notes", r.ID)
		}
		if r.Group == "" {
			t.Errorf("seed row %q has no source group", r.ID)
		}
	}
}

// TestSchemaStudyRowIsSeeded is acceptance item 12: the G4 [schema] STUDY row is
// present, sourced at schema-harness.github.io, and carries the report-12
// discipline caveat verbatim.
func TestSchemaStudyRowIsSeeded(t *testing.T) {
	r, ok := seedByID(t)["study-schema-arc-agi-3-harness"]
	if !ok {
		t.Fatal("the G4 [schema] STUDY row is not seeded")
	}
	if r.Kind != watchlist.KindStudy {
		t.Errorf("the [schema] row kind = %q, want study", r.Kind)
	}
	if !strings.Contains(r.URL, "schema-harness.github.io") {
		t.Errorf("the [schema] row source = %q, want schema-harness.github.io", r.URL)
	}
	for _, want := range []string{
		"ARC-AGI-3 harness",
		"world-model-as-code",
		"mechanism-as-executable-program",
		"candidate future skill/technique pattern for domain workers",
		"self-reported, public-set, scaffold-specific",
	} {
		if !strings.Contains(r.Notes, want) {
			t.Errorf("the [schema] row notes lack the GATE-4 phrase %q", want)
		}
	}
}

// TestAwesomeHarnessRowWatchesBothReferents is acceptance item 12 (G2 Def.15):
// ONE row carrying BOTH referents and the drop-if-both-stall rule — a single
// ratified decision, not two independent watches.
func TestAwesomeHarnessRowWatchesBothReferents(t *testing.T) {
	r, ok := seedByID(t)["s168-awesome-harness-engineering"]
	if !ok {
		t.Fatal("the awesome-harness-engineering row is not seeded")
	}
	for _, want := range []string{"walkinglabs/", "ai-boost/", "DROP THIS ROW IF BOTH STALL", "G2 Def.15"} {
		if !strings.Contains(r.Notes, want) {
			t.Errorf("the awesome-harness row notes lack %q", want)
		}
	}
}

// TestEveryS168RowIsSeeded: all sixteen S16.8 standing rows are registered into
// this executor, each naming what it watches and what a hit fires.
func TestEveryS168RowIsSeeded(t *testing.T) {
	byID := seedByID(t)
	want := []string{
		"s168-pat-collaborator-gap", "s168-free-plan-rulesets", "s168-diffity-maturity",
		"s168-jj-compat-matrix", "s168-xwing-age-plugin", "s168-dbos-parity",
		"s168-mcp-final", "s168-acp-v1", "s168-litestream-1083", "s168-redlines-pandiff",
		"s168-dndkit-react-1", "s168-assistant-ui-1", "s168-awesome-harness-engineering",
		"s168-crush-fsl-mit", "s168-genai-prices-staleness", "s168-opencode-v2",
	}
	if got := countGroup(watchlist.GroupS168); got != len(want) {
		t.Errorf("%d S16.8 rows seeded, want %d", got, len(want))
	}
	for _, id := range want {
		r, ok := byID[id]
		if !ok {
			t.Errorf("S16.8 row %q is not seeded", id)
			continue
		}
		if !strings.Contains(r.Notes, "fires →") {
			t.Errorf("S16.8 row %q does not record what a hit fires", id)
		}
	}
}

// TestReport02TiersAreSeeded: every report-02 §6 tier is materialized, with the
// tiers the report assigns and a plausible population per tier.
func TestReport02TiersAreSeeded(t *testing.T) {
	byTier := map[int]int{}
	for _, r := range watchlist.SeedRows() {
		if r.Group == watchlist.GroupReport02 {
			byTier[r.Tier]++
		}
	}
	for tier, min := range map[int]int{1: 15, 2: 8, 3: 4, 4: 4} {
		if byTier[tier] < min {
			t.Errorf("report-02 tier %d has %d rows, want at least %d", tier, byTier[tier], min)
		}
	}

	byID := seedByID(t)
	// Named sources report-02 calls out explicitly.
	for id, wantKind := range map[string]string{
		"t1-anthropic-pricing": watchlist.KindPage,
		"t1-zai-devpack":       watchlist.KindPage,
		"t1-cerebras-code":     watchlist.KindPage,
		"t2-opencode-releases": watchlist.KindFeed,
		"t2-modelsdev-commits": watchlist.KindFeed,
		"t3-modelsdev-api":     watchlist.KindAPI,
		"t3-openrouter-blog":   watchlist.KindFeed,
		"t4-localllama":        watchlist.KindFeed,
		watchlist.PriceRowID:   watchlist.KindAPI,
	} {
		r, ok := byID[id]
		if !ok {
			t.Errorf("report-02 row %q is not seeded", id)
			continue
		}
		if r.Kind != wantKind {
			t.Errorf("row %q kind = %q, want %q", id, r.Kind, wantKind)
		}
	}

	// hnrss is CONSUMED AS HOSTED and never self-hosted (no license file).
	for _, r := range watchlist.SeedRows() {
		if strings.Contains(r.URL, "hnrss.org") && !strings.Contains(r.Notes, "never self-hosted") {
			t.Errorf("row %q consumes hnrss without recording the never-self-host rule", r.ID)
		}
	}
}

// TestScrapeTargetsCarryTheMigrateBias (R4/P-T11-3): the standing bias to
// migrate a scrape target to a feed/API is recorded as ROW DATA on every page
// row, not as a comment.
func TestScrapeTargetsCarryTheMigrateBias(t *testing.T) {
	for _, r := range watchlist.SeedRows() {
		if r.Kind == watchlist.KindPage && !r.MigrateBias {
			t.Errorf("page row %q does not carry the P-T11-3 migrate bias", r.ID)
		}
	}
}

// TestEveryLockEntryResolvesToASeededWatchRow is acceptance item 11 / S16.4 #9:
// every components.lock entry's watch reference resolves to a real row. A new
// adoption without its watch row fails here.
func TestEveryLockEntryResolvesToASeededWatchRow(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, "components.lock"))
	if err != nil {
		t.Fatal(err)
	}
	lf, err := lockfile.Parse(raw)
	if err != nil {
		t.Fatalf("parse components.lock: %v", err)
	}
	byID := seedByID(t)
	names := map[string]bool{}
	for _, c := range lf.Components {
		names[c.Name] = true
		if len(c.Watch) == 0 {
			t.Errorf("lock entry %q carries no watch reference (S16.2 mandatory field)", c.Name)
		}
		id := watchlist.LockRowID(c.Name)
		if _, ok := byID[id]; !ok {
			t.Errorf("lock entry %q has no seeded watch row %q — S16.4 #9 requires every entry's watch reference to resolve", c.Name, id)
		}
	}
	// And the reverse: the seed does not claim a review row for an entry that
	// no longer exists (a row claiming a dead component would fake coverage).
	for _, name := range watchlist.LockEntryNames {
		if !names[name] {
			t.Errorf("seeded lock-review row names %q, which is not in components.lock", name)
		}
	}
}

// TestLoadRowsIsStrict (R3): an unknown field fails LOUDLY rather than being
// silently ignored, and a duplicate id is refused. The round trip through
// WriteSeed must parse.
func TestLoadRowsIsStrict(t *testing.T) {
	good := `[{"id":"x","kind":"feed","url":"https://e.invalid/f","tier":2,"cadence":"weekly","enabled":true,"notes":"a hand-written override row"}]`
	rows, err := watchlist.LoadRows([]byte(good))
	if err != nil {
		t.Fatalf("LoadRows(valid): %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "x" {
		t.Fatalf("LoadRows returned %+v", rows)
	}

	for name, body := range map[string]string{
		"unknown field": `[{"id":"x","kind":"feed","url":"https://e.invalid/f","tier":2,"cadence":"weekly","notes":"n","cadenceTypo":"weekly"}]`,
		"bad cadence":   `[{"id":"x","kind":"feed","url":"https://e.invalid/f","tier":2,"cadence":"hourly","notes":"n"}]`,
		"bad kind":      `[{"id":"x","kind":"webhook","url":"https://e.invalid/f","tier":2,"cadence":"weekly","notes":"n"}]`,
		"missing url":   `[{"id":"x","kind":"feed","tier":2,"cadence":"weekly","notes":"n"}]`,
		"no notes":      `[{"id":"x","kind":"feed","url":"https://e.invalid/f","tier":2,"cadence":"weekly"}]`,
		"tier range":    `[{"id":"x","kind":"feed","url":"https://e.invalid/f","tier":9,"cadence":"weekly","notes":"n"}]`,
		"duplicate id":  `[{"id":"x","kind":"feed","url":"https://e.invalid/f","tier":2,"cadence":"weekly","notes":"n"},{"id":"x","kind":"feed","url":"https://e.invalid/g","tier":2,"cadence":"weekly","notes":"n"}]`,
	} {
		if _, err := watchlist.LoadRows([]byte(body)); err == nil {
			t.Errorf("LoadRows accepted a %s override — it must fail loudly", name)
		}
	}

	dump, err := watchlist.WriteSeed()
	if err != nil {
		t.Fatalf("WriteSeed: %v", err)
	}
	if !json.Valid(dump) {
		t.Fatal("WriteSeed output is not valid JSON")
	}
	back, err := watchlist.LoadRows(dump)
	if err != nil {
		t.Fatalf("WriteSeed output does not round-trip through LoadRows: %v", err)
	}
	if len(back) != len(watchlist.SeedRows()) {
		t.Errorf("round trip lost rows: %d vs %d", len(back), len(watchlist.SeedRows()))
	}
}

func countGroup(group string) int {
	n := 0
	for _, r := range watchlist.SeedRows() {
		if r.Group == group {
			n++
		}
	}
	return n
}

// TestS147StandingQuestionsAreRegistered is the B5-7 R25 obligation: S14.7's
// four standing benchmark/eval questions are REGISTERED so they cannot
// evaporate. They are data-only study rows — never polled, each naming its
// spec reference — surfaced through PendingReview for the S16.7 quarterly pass.
func TestS147StandingQuestionsAreRegistered(t *testing.T) {
	want := map[string]string{
		"s147-anchored-findings-vs-file-notes":       "R13-OQ6",
		"s147-delta-card-rubber-stamp-analysis":      "P-T05-2",
		"s147-stakes-classifier-pre-registered-eval": "P-T05-4",
		"s147-intake-approval-ux-measured-only":      "MEASURED outcomes",
	}
	byID := seedByID(t)
	group := 0
	for _, r := range watchlist.SeedRows() {
		if r.Group != watchlist.GroupBenchmarkQuestions {
			continue
		}
		group++
		// A registration is never a fetch: a study row has no URL to poll and
		// no polling cadence could apply to it.
		if r.Kind != watchlist.KindStudy {
			t.Errorf("standing question %q is kind %q, want study — these are registrations, never probes", r.ID, r.Kind)
		}
		if r.URL != "" {
			t.Errorf("standing question %q carries a URL %q — there is nothing to fetch", r.ID, r.URL)
		}
		if !strings.Contains(r.Notes, "S14.7") {
			t.Errorf("standing question %q does not cite S14.7", r.ID)
		}
	}
	if group != len(want) {
		t.Errorf("%d rows in group %q, want the four S14.7 questions", group, watchlist.GroupBenchmarkQuestions)
	}
	for id, ref := range want {
		r, ok := byID[id]
		if !ok {
			t.Errorf("standing question %q is not seeded — S14.7 registers it so it cannot evaporate", id)
			continue
		}
		if !strings.Contains(r.Notes, ref) {
			t.Errorf("standing question %q does not name its reference %q", id, ref)
		}
	}
}

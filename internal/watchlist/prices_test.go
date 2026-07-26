package watchlist_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/lockfile"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchlist"
)

// TestVendoredPriceDataMatchesTheLockPin: the vendored bytes cannot drift from
// the manifest silently — the content hash is the pin.
func TestVendoredPriceDataMatchesTheLockPin(t *testing.T) {
	sum := sha256.Sum256(watchlist.VendoredPriceData())
	got := hex.EncodeToString(sum[:])
	if got != watchlist.VendoredPriceSHA256 {
		t.Fatalf("the vendored price data hashes to %s, the package pins %s", got, watchlist.VendoredPriceSHA256)
	}

	raw, err := os.ReadFile(filepath.Join(repoRoot, "components.lock"))
	if err != nil {
		t.Fatal(err)
	}
	lf, err := lockfile.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range lf.Components {
		if c.Name != "genai-prices data.json" {
			continue
		}
		if c.Kind != "data" {
			t.Errorf("genai-prices entry kind = %q, want data", c.Kind)
		}
		if !strings.Contains(c.Pin, got) {
			t.Errorf("components.lock pin %q does not carry the vendored content hash %s", c.Pin, got)
		}
		if !strings.Contains(c.Pin, watchlist.VendoredPricePin) {
			t.Errorf("components.lock pin %q does not carry the upstream tag %s", c.Pin, watchlist.VendoredPricePin)
		}
		if len(c.Modules) != 0 {
			t.Error("the vendored data entry must claim no Go modules")
		}
		// S16.2: the upstream LICENSE is kept in-tree beside vendored content.
		if _, err := os.Stat(filepath.Join(repoRoot, "internal/watchlist/pricedata/LICENSE")); err != nil {
			t.Errorf("the upstream LICENSE is not kept in-tree beside the vendored file: %v", err)
		}
		return
	}
	t.Fatal("components.lock carries no genai-prices data.json entry")
}

// TestPriceRefreshLandsAsAProposal is acceptance item 18: the refresh produces a
// PROPOSAL record with effective-dated rows and writes no table.
func TestPriceRefreshLandsAsAProposal(t *testing.T) {
	// Move one Anthropic price and add a model, against the vendored baseline.
	var doc []map[string]any
	if err := json.Unmarshal(watchlist.VendoredPriceData(), &doc); err != nil {
		t.Fatal(err)
	}
	moved := false
	for _, p := range doc {
		if p["id"] != "anthropic" {
			continue
		}
		models := p["models"].([]any)
		for _, m := range models {
			mm := m.(map[string]any)
			prices, ok := mm["prices"].(map[string]any)
			if !ok {
				continue
			}
			if _, has := prices["input_mtok"].(float64); !has {
				continue
			}
			prices["input_mtok"] = 99.0
			moved = true
			break
		}
		break
	}
	if !moved {
		t.Fatal("the fixture could not move an anthropic price — the vendored document shape changed")
	}
	upstream, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	at := mustTime(t, "2026-08-01T00:00:00Z")
	prop, err := watchlist.BuildPriceProposal(upstream, at)
	if err != nil {
		t.Fatalf("BuildPriceProposal: %v", err)
	}
	if prop.AutoApplied {
		t.Fatal("a price proposal reported itself as applied — it is a proposal, never a table write")
	}
	if prop.TotalRows == 0 {
		t.Fatal("the moved price produced no proposed row")
	}
	if prop.BaselineSHA256 != watchlist.VendoredPriceSHA256 {
		t.Error("the proposal does not identify the vendored baseline it diffed against")
	}
	for _, r := range prop.Rows {
		if r.EffectiveFrom != "2026-08-01" {
			t.Errorf("proposed row %s/%s effective %q, want the observation date", r.Lane, r.Model, r.EffectiveFrom)
		}
		if r.Lane != "anthropic" && r.Lane != "zai" {
			t.Errorf("proposed row lane %q — only lanes Sinet prices may be proposed", r.Lane)
		}
		// Prices convert to per-SINGLE-token (S10.3 exact arithmetic).
		pr, err := r.PriceRow()
		if err != nil {
			t.Errorf("proposed row does not convert to a metering.PriceRow: %v", err)
			continue
		}
		if pr.Prices.InputUSD > 0.01 {
			t.Errorf("proposed input price %g looks per-million, not per-token", pr.Prices.InputUSD)
		}
	}
	if prop.OutOfScope == 0 {
		t.Error("the proposal reports no out-of-scope models — upstream covers many providers Sinet runs no lane for, and that is reported, not dropped")
	}
}

// TestPriceRefreshEmitsAProposalCardAndWritesNoTable: the end-to-end job path.
func TestPriceRefreshEmitsAProposalCardAndWritesNoTable(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	var doc []map[string]any
	if err := json.Unmarshal(watchlist.VendoredPriceData(), &doc); err != nil {
		t.Fatal(err)
	}
	for _, p := range doc {
		if p["id"] == "anthropic" {
			for _, m := range p["models"].([]any) {
				if prices, ok := m.(map[string]any)["prices"].(map[string]any); ok {
					if _, has := prices["output_mtok"].(float64); has {
						prices["output_mtok"] = 123.0
						break
					}
				}
			}
		}
	}
	upstream, _ := json.Marshal(doc)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(upstream)
	}))
	defer srv.Close()

	h.seedOne(t, watchlist.Row{
		ID: watchlist.PriceRowID, Kind: watchlist.KindAPI, URL: srv.URL,
		ParserHint: "json", Tier: 3, Cadence: watchlist.CadenceWeekly, Enabled: true,
		Group: watchlist.GroupReport02, Notes: "the genai-prices refresh row",
	})
	clk := &clock{t: mustTime(t, "2026-08-01T00:00:00Z")}
	x := h.exec(clk, watchlist.Deps{HTTPClient: srv.Client()})
	if _, err := x.RunDue(ctx); err != nil {
		t.Fatal(err)
	}

	f := h.findings(t)
	if len(f) != 1 {
		t.Fatalf("%d findings, want 1 price-refresh proposal", len(f))
	}
	if f[0].Proposal == nil {
		t.Fatal("the finding carries no proposal record")
	}
	if f[0].ChangeClass != watchlist.ClassPrice {
		t.Errorf("change class = %q, want price", f[0].ChangeClass)
	}
	if !strings.Contains(f[0].Detail, "Nothing was applied") {
		t.Error("the card does not state plainly that nothing was applied")
	}
	if f[0].Proposal.AutoApplied {
		t.Error("the proposal claims it was applied")
	}
}

// TestNoCodePathAppliesAPriceOrFlipsABillingFlag is acceptance item 18's
// negative, asserted by source scan: this package writes no price table and
// touches no billing-regime flag. The metering seam it may name is the PriceRow
// TYPE (a proposal shape), never a writer.
func TestNoCodePathAppliesAPriceOrFlipsABillingFlag(t *testing.T) {
	src := readPackageSource(t)
	for _, banned := range []string{
		"SetPrice", "ApplyPrice", "UpsertPrice", "WritePrice",
		"EffectiveDatedTable{", "SetMetered", "MeteredExceptions{",
		"PriceTableVersion =", "SetBillingRegime",
	} {
		if strings.Contains(src, banned) {
			t.Errorf("the watchlist references %q — a refresh PROPOSES, it never applies a price or flips a billing-regime flag (S10.2/S14.6)", banned)
		}
	}
	if !strings.Contains(src, "metering.PriceRow") {
		t.Error("the proposal does not produce values against metering.PriceRow — the proposed shape must be the real type")
	}
}

// TestPriceStalenessProposesTheFallback (S16.8): when upstream stops moving past
// ⚙ adoption.price_data_stale_days, the row proposes the LiteLLM-primary
// fallback — a proposal for the operator, never a switch.
func TestPriceStalenessProposesTheFallback(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	body := watchlist.VendoredPriceData()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	h.seedOne(t, watchlist.Row{
		ID: watchlist.PriceRowID, Kind: watchlist.KindAPI, URL: srv.URL,
		ParserHint: "json", Tier: 3, Cadence: watchlist.CadenceWeekly, Enabled: true,
		Group: watchlist.GroupReport02, Notes: "the genai-prices refresh row",
	})
	// First pass: upstream matches the vendored baseline exactly, so the
	// proposal is empty and nothing is raised yet.
	clk := &clock{t: mustTime(t, "2026-07-27T00:00:00Z")}
	x := h.exec(clk, watchlist.Deps{HTTPClient: srv.Client()})
	if _, err := x.RunDue(ctx); err != nil {
		t.Fatal(err)
	}

	// Well past the ⚙ 60-day horizon with no upstream movement.
	clk.advance(120 * 24 * time.Hour)
	if _, err := x.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	f := h.findings(t)
	if len(f) == 0 {
		t.Fatal("a stale price source raised nothing")
	}
	last := f[len(f)-1]
	if last.Proposal == nil || last.Proposal.Stale == "" {
		t.Fatalf("the staleness proposal was not recorded: %+v", last.Proposal)
	}
	if !strings.Contains(last.Proposal.Stale, "LiteLLM-primary") {
		t.Error("the staleness proposal does not name the S16.8 LiteLLM-primary fallback")
	}
	if !strings.Contains(last.Proposal.Stale, "not a switch") {
		t.Error("the staleness proposal does not state it is a proposal, not a switch")
	}
}

// TestStructuralDiffIsKeyLevelNeverFreeText (R13): the aggregator diff reports
// added, removed and CHANGED leaf keys.
func TestStructuralDiffIsKeyLevelNeverFreeText(t *testing.T) {
	before := []byte(`{"models":{"a":{"ctx":1000},"b":{"ctx":2000}},"meta":{"v":1}}`)
	after := []byte(`{"models":{"a":{"ctx":1000},"c":{"ctx":3000}},"meta":{"v":2}}`)

	baseline, ok := watchlist.StructuralBaseline(before)
	if !ok {
		t.Fatal("StructuralBaseline refused a small document")
	}
	d := watchlist.DiffStructural(baseline, after)
	if d.Coarse {
		t.Fatal("a small document produced a coarse diff")
	}
	if d.TotalAdded != 1 || d.TotalRemoved != 1 || d.TotalChanged != 1 {
		t.Errorf("diff = +%d -%d ~%d, want +1 -1 ~1 (%v / %v / %v)",
			d.TotalAdded, d.TotalRemoved, d.TotalChanged, d.Added, d.Removed, d.Changed)
	}
	if d.Empty() {
		t.Error("a real diff reported Empty")
	}
	if watchlist.DiffStructural(baseline, before).Empty() != true {
		t.Error("an identical document did not diff empty")
	}
	// An absent baseline degrades HONESTLY to coarse rather than claiming a
	// key-level answer it does not have.
	if !watchlist.DiffStructural("", after).Coarse {
		t.Error("an absent baseline did not degrade to a coarse diff")
	}
}

// TestAggregatorFirstFetchEstablishesTheBaselineQuietly: there was no prior
// state to have drifted from, so the first observation raises no card.
func TestAggregatorFirstFetchEstablishesTheBaselineQuietly(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	body := `{"models":{"a":{"ctx":1000}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	h.seedOne(t, watchlist.Row{
		ID: "t3-modelsdev-api", Kind: watchlist.KindAPI, URL: srv.URL, ParserHint: "json",
		Tier: 3, Cadence: watchlist.CadenceWeekly, Enabled: true,
		Group: watchlist.GroupReport02, Notes: "models.dev api.json",
	})
	clk := &clock{t: mustTime(t, "2026-07-01T00:00:00Z")}
	x := h.exec(clk, watchlist.Deps{HTTPClient: srv.Client()})
	if _, err := x.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if got := len(h.findings(t)); got != 0 {
		t.Fatalf("the first observation raised %d findings, want 0", got)
	}

	body = `{"models":{"a":{"ctx":1000},"b":{"ctx":2000}}}`
	clk.advance(8 * 24 * time.Hour)
	if _, err := x.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	f := h.findings(t)
	if len(f) != 1 {
		t.Fatalf("a real structural change raised %d findings, want 1", len(f))
	}
	if !strings.Contains(f[0].Detail, "$.models.b.ctx") {
		t.Errorf("the finding does not name the changed key path: %q", f[0].Detail)
	}
}

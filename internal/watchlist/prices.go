package watchlist

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
)

// prices.go — the genai-prices refresh, which ALWAYS lands as a PROPOSAL
// (Spec S14.6 ¶T3; G2 Def.16; S10.3; P-T08-3).
//
// The pinned `prices/data.json` is VENDORED in-tree (coordinator disposition
// OQ3(a)) with its upstream LICENSE kept beside it (S16.2). Its ONE consumer is
// the diff baseline below: the refresh job fetches upstream, diffs it against
// this file, and lands proposed metering.PriceRow values WITH EFFECTIVE DATES
// on the drift finding. The loader / user overlay / persistence remain S10.3's
// metering follow-up and are deliberately NOT pulled in here.
//
// Nothing in this file writes a price table. There is no code path that
// auto-applies a price change and none that auto-flips a billing-regime flag:
// the operator confirms, and only then does the rehearsed S10.2 flip run
// (S10.2/S14.6). An APPROVED price change is what moves
// run.Fingerprint.PriceTableVersion and fires the G1 Def.5 freshness trigger —
// this package proposes, it never approves.

//go:embed pricedata/data.json
var vendoredPriceData []byte

// The vendored pin. VendoredPriceSHA256 is the content hash recorded in the
// components.lock `data` entry; a test asserts the file still hashes to it, so
// the vendored bytes can never drift from the manifest silently.
const (
	VendoredPricePin     = "v0.0.72"
	VendoredPriceSHA256  = "42e71b090d70007fa8f31851f5b09fcfe83b6764ec89cd42ee1a27a0502a4273"
	VendoredPriceSource  = "https://raw.githubusercontent.com/pydantic/genai-prices/v0.0.72/prices/data.json"
	vendoredPriceChecked = "2026-07-26"
)

// proposedRowCap bounds how many proposed price rows ride one finding payload
// (⚙ state.event_payload_cap is the hard gate above it). The total is always
// reported, so a truncated list never understates a change.
const proposedRowCap = 30

// VendoredPriceData returns the vendored baseline bytes.
func VendoredPriceData() []byte { return vendoredPriceData }

// PriceProposal is the durable refresh proposal carried on a drift finding
// (S14.6 ¶T3 "always lands as a proposal"). It is a PROPOSAL RECORD, not a
// table write: `Rows` are the proposed metering.PriceRow values with their
// effective dates, awaiting operator confirmation.
type PriceProposal struct {
	// BaselinePin / BaselineSHA256 identify the vendored file the diff ran
	// against.
	BaselinePin    string `json:"baseline_pin"`
	BaselineSHA256 string `json:"baseline_sha256"`
	// UpstreamSHA256 is the content hash of the fetched document.
	UpstreamSHA256 string `json:"upstream_sha256"`
	// Diff is the structural summary of what moved.
	Diff string `json:"diff"`
	// Rows are the proposed price rows, bounded; TotalRows is the untruncated
	// count.
	Rows      []ProposedPriceRow `json:"rows"`
	TotalRows int                `json:"total_rows"`
	// OutOfScope counts upstream models whose provider maps to no Sinet lane —
	// real upstream movement that this table would never price. Reported, not
	// dropped.
	OutOfScope int `json:"out_of_scope"`
	// Structured counts models whose upstream pricing is conditional or tiered
	// (a nested object, or a list of effective-dated variants) and therefore is
	// not expressible as one flat effective-dated row. They are surfaced for
	// the operator to price by hand rather than silently flattened to a wrong
	// number.
	Structured []string `json:"structured,omitempty"`
	// Removed lists in-scope models the baseline priced and upstream no longer
	// carries — a retired model is real in-scope movement, and a diff assembled
	// only from what upstream still carries can never see one.
	Removed []string `json:"removed,omitempty"`
	// ShapeChanged lists in-scope models whose upstream pricing is not a flat
	// scalar shape AND does not match the baseline's block for that model:
	// either it stopped being a flat rate, or its conditional/tiered numbers
	// moved. Neither is expressible as one proposed row, so it is reported as
	// change rather than dropped.
	//
	// Removed and ShapeChanged are bounded by the number of models the two
	// priced lanes carry (a few dozen), not by upstream's ~1,400, so neither
	// needs a cap.
	ShapeChanged []string `json:"shape_changed,omitempty"`
	// Stale, when set, is the S16.8 genai-prices staleness finding: upstream
	// has not moved for longer than ⚙ adoption.price_data_stale_days while
	// providers reprice, which PROPOSES the LiteLLM-primary fallback.
	Stale string `json:"stale,omitempty"`
	// AutoApplied is always false and is asserted by test: a price proposal is
	// never an applied table write, and a billing-regime change never
	// auto-flips a flag (S10.2/S14.6).
	AutoApplied bool `json:"auto_applied"`
}

// ProposedPriceRow is one proposed effective-dated price-table row. It mirrors
// metering.PriceRow's shape as JSON — metering owns the type, this package
// proposes values for it and writes none.
type ProposedPriceRow struct {
	Model            string  `json:"model"`
	Lane             string  `json:"lane"`
	InputUSD         float64 `json:"input_usd"`
	OutputUSD        float64 `json:"output_usd"`
	CacheReadUSD     float64 `json:"cache_read_usd"`
	CacheCreationUSD float64 `json:"cache_creation_usd"`
	// EffectiveFrom is the row's effective date — future-dated rows are
	// first-class, because a table without effective dates is guaranteed wrong
	// on a known future date (P-T08-3).
	EffectiveFrom string `json:"effective_from"`
	Source        string `json:"source"`
}

// PriceRow converts a proposal row into the metering type the operator would
// approve. It is the one place the two shapes meet; nothing here writes it into
// a table (the loader is S10.3's follow-up).
func (p ProposedPriceRow) PriceRow() (metering.PriceRow, error) {
	from, err := time.Parse(time.DateOnly, p.EffectiveFrom)
	if err != nil {
		return metering.PriceRow{}, fmt.Errorf("watchlist: proposed row %s/%s: effective date %q: %w",
			p.Lane, p.Model, p.EffectiveFrom, err)
	}
	return metering.PriceRow{
		Model: p.Model, Lane: p.Lane, EffectiveFrom: from, Source: p.Source,
		Prices: metering.UnitPrices{
			InputUSD: p.InputUSD, OutputUSD: p.OutputUSD,
			CacheReadUSD: p.CacheReadUSD, CacheCreationUSD: p.CacheCreationUSD,
		},
	}, nil
}

// The genai-prices document at the vendored pin is a TOP-LEVEL LIST of
// providers. `prices` is decoded as raw JSON because upstream carries two
// shapes this proposal cannot flatten: a LIST of effective-dated variants, and
// per-field conditional objects (tiered by context length). Unknown fields are
// ignored — forward-tolerant, like every other outside-world decoder here.
type priceDoc []struct {
	ID     string `json:"id"`
	Models []struct {
		ID     string          `json:"id"`
		Prices json.RawMessage `json:"prices"`
	} `json:"models"`
}

// laneForProvider maps a genai-prices provider id to the Sinet lane it prices.
// The price table only ever prices lanes Sinet runs (S03.2: anthropic, zai,
// local — and local is a true $0 lane, never priced from this table), so a
// provider outside this map is counted out-of-scope rather than proposed.
var laneForProvider = map[string]string{
	"anthropic": "anthropic",
	"zhipuai":   "zai",
}

// flatPrices are the four per-million-token fields, present only when upstream
// states them as plain scalars.
type flatPrices struct {
	InputMTok      json.Number `json:"input_mtok"`
	OutputMTok     json.Number `json:"output_mtok"`
	CacheReadMTok  json.Number `json:"cache_read_mtok"`
	CacheWriteMTok json.Number `json:"cache_write_mtok"`
}

// parseFlatPrices decodes a model's `prices` block, reporting ok=false when the
// shape is conditional or tiered and cannot become one flat row.
func parseFlatPrices(raw json.RawMessage) (flatPrices, bool) {
	var fp flatPrices
	if len(raw) == 0 {
		return fp, false
	}
	if err := json.Unmarshal(raw, &fp); err != nil {
		return flatPrices{}, false
	}
	return fp, true
}

// perMillion converts a per-million-token price to the per-single-token price
// metering keeps (S10.3: prices are per single token so the arithmetic is
// exact).
func perMillion(n json.Number) float64 {
	f, err := n.Float64()
	if err != nil {
		return 0
	}
	return f / 1_000_000
}

// BuildPriceProposal diffs a freshly fetched genai-prices document against the
// vendored baseline and renders the proposal. effectiveFrom dates every
// proposed row; it is the observation date, never "now minus something".
func BuildPriceProposal(upstream []byte, effectiveFrom time.Time) (*PriceProposal, error) {
	prop := &PriceProposal{
		BaselinePin:    VendoredPricePin,
		BaselineSHA256: VendoredPriceSHA256,
		UpstreamSHA256: contentHash(upstream),
	}
	baseline, ok := StructuralBaseline(vendoredPriceData)
	if !ok {
		prop.Diff = "coarse: the vendored baseline exceeded the structural digest cap"
	} else {
		prop.Diff = DiffStructural(baseline, upstream).Summary()
	}

	var fresh, prev priceDoc
	if err := json.Unmarshal(upstream, &fresh); err != nil {
		return nil, fmt.Errorf("watchlist: parse upstream price data: %w", err)
	}
	if err := json.Unmarshal(vendoredPriceData, &prev); err != nil {
		return nil, fmt.Errorf("watchlist: parse vendored price data: %w", err)
	}

	// The baseline's in-scope models: their flat rows where they have one, and
	// their raw price block either way. BOTH are needed — a model that leaves
	// upstream, and one whose block stops being flat, are in-scope changes that
	// a diff built only from what upstream still carries would never see.
	old := map[string]ProposedPriceRow{}
	prevBlock := map[string]string{}
	for _, p := range prev {
		lane, inScope := laneForProvider[p.ID]
		if !inScope {
			continue
		}
		for _, m := range p.Models {
			key := p.ID + "/" + m.ID
			prevBlock[key] = canonicalJSON(m.Prices)
			if fp, ok := parseFlatPrices(m.Prices); ok {
				old[key] = priceRowOf(lane, m.ID, fp, effectiveFrom)
			}
		}
	}
	var moved []ProposedPriceRow
	stillUpstream := map[string]bool{}
	for _, p := range fresh {
		lane, inScope := laneForProvider[p.ID]
		for _, m := range p.Models {
			if !inScope {
				prop.OutOfScope++
				continue
			}
			key := p.ID + "/" + m.ID
			stillUpstream[key] = true
			fp, ok := parseFlatPrices(m.Prices)
			if !ok {
				prop.Structured = append(prop.Structured, key)
				// A non-flat block is a CHANGE when the baseline priced this
				// model flat, or when the block itself moved. Unchanged tiered
				// pricing is reported for hand-pricing but is not movement.
				if base, known := prevBlock[key]; !known || base != canonicalJSON(m.Prices) {
					prop.ShapeChanged = append(prop.ShapeChanged, key)
				}
				continue
			}
			row := priceRowOf(lane, m.ID, fp, effectiveFrom)
			if b, seen := old[key]; seen && samePrices(b, row) {
				continue
			}
			moved = append(moved, row)
		}
	}
	for key := range prevBlock {
		if !stillUpstream[key] {
			prop.Removed = append(prop.Removed, key)
		}
	}
	sort.Slice(moved, func(i, j int) bool {
		if moved[i].Lane != moved[j].Lane {
			return moved[i].Lane < moved[j].Lane
		}
		return moved[i].Model < moved[j].Model
	})
	sort.Strings(prop.Structured)
	sort.Strings(prop.ShapeChanged)
	sort.Strings(prop.Removed)
	prop.TotalRows = len(moved)
	if len(moved) > proposedRowCap {
		moved = moved[:proposedRowCap]
	}
	prop.Rows = moved
	return prop, nil
}

// HasInScopeChange reports whether anything Sinet prices actually moved: a
// proposed row, a model that left upstream, or one whose price shape or tiered
// numbers moved. It is what gates the finding, so the question asked is "did
// anything IN SCOPE change?" and never the narrower "did we manage to propose a
// row?" — the narrow question silently swallows every removal and every
// change to conditional pricing.
func (p *PriceProposal) HasInScopeChange() bool {
	return p.TotalRows > 0 || len(p.Removed) > 0 || len(p.ShapeChanged) > 0
}

// canonicalJSON renders a raw price block in a canonical form — object keys
// sorted (json.Marshal sorts map keys), numbers in one format, no incidental
// whitespace — so only a real change to a conditional/tiered block reads as
// one. Upstream regenerating the document with a different key order or number
// formatting must not manufacture drift findings.
func canonicalJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(out)
}

func priceRowOf(lane, model string, fp flatPrices, from time.Time) ProposedPriceRow {
	return ProposedPriceRow{
		Model: model, Lane: lane,
		InputUSD: perMillion(fp.InputMTok), OutputUSD: perMillion(fp.OutputMTok),
		CacheReadUSD: perMillion(fp.CacheReadMTok), CacheCreationUSD: perMillion(fp.CacheWriteMTok),
		EffectiveFrom: from.UTC().Format(time.DateOnly),
		Source:        "genai-prices " + VendoredPricePin + " refresh proposal",
	}
}

func samePrices(a, b ProposedPriceRow) bool {
	return a.InputUSD == b.InputUSD && a.OutputUSD == b.OutputUSD &&
		a.CacheReadUSD == b.CacheReadUSD && a.CacheCreationUSD == b.CacheCreationUSD
}

// stalenessNote renders the S16.8 genai-prices staleness proposal when upstream
// has not moved for longer than ⚙ adoption.price_data_stale_days. It PROPOSES
// the LiteLLM-primary fallback; it never switches anything.
func stalenessNote(days int64, since time.Duration) string {
	return fmt.Sprintf(
		"genai-prices has not changed for %d days, past the ⚙ adoption.price_data_stale_days horizon of %d, while providers reprice — "+
			"S16.8 proposes flipping to a LiteLLM-primary source with a manual table. This is a proposal for the operator, not a switch.",
		int(since.Hours()/24), days)
}

// vendoredBaselineDate is the date the vendored file was pinned — the staleness
// clock's starting point until the row produces its first observed change.
func vendoredBaselineDate() time.Time {
	t, err := time.Parse(time.DateOnly, vendoredPriceChecked)
	if err != nil {
		return time.Time{}
	}
	return t
}

// PriceRowID is the seeded watch row that drives the refresh job.
const PriceRowID = "t3-genai-prices"

// isPriceRow reports whether a row is the genai-prices refresh row.
func isPriceRow(r Row) bool { return r.ID == PriceRowID }

// priceProposalSummary renders the one-line summary of a refresh proposal.
func priceProposalSummary(p *PriceProposal) string {
	if p.Stale != "" {
		return "genai-prices data is stale — S16.8 proposes the LiteLLM-primary fallback"
	}
	var parts []string
	if p.TotalRows > 0 {
		parts = append(parts, fmt.Sprintf("%d price-table row(s) with effective dates", p.TotalRows))
	}
	if n := len(p.Removed); n > 0 {
		parts = append(parts, fmt.Sprintf("%d priced model(s) gone from upstream", n))
	}
	if n := len(p.ShapeChanged); n > 0 {
		parts = append(parts, fmt.Sprintf("%d model(s) whose pricing is no longer one flat rate", n))
	}
	return "genai-prices refresh: " + strings.Join(parts, ", ") +
		" — operator confirmation required, nothing applied"
}

// priceProposalDetail renders the bounded human-readable proposal body.
func priceProposalDetail(p *PriceProposal) string {
	var b strings.Builder
	fmt.Fprintf(&b, "baseline %s (sha256 %s…)\nupstream sha256 %s…\nstructural diff: %s\n",
		p.BaselinePin, p.BaselineSHA256[:12], p.UpstreamSHA256[:12], p.Diff)
	if p.Stale != "" {
		b.WriteString(p.Stale)
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "proposed rows: %d (showing %d)\n", p.TotalRows, len(p.Rows))
	for _, r := range p.Rows {
		fmt.Fprintf(&b, "  %s/%s in=%g out=%g effective %s\n", r.Lane, r.Model, r.InputUSD, r.OutputUSD, r.EffectiveFrom)
	}
	if len(p.Removed) > 0 {
		fmt.Fprintf(&b, "priced models gone from upstream (retired, renamed, or dropped — price them from the last known table until the operator decides): %s\n",
			strings.Join(p.Removed, ", "))
	}
	if len(p.ShapeChanged) > 0 {
		fmt.Fprintf(&b, "pricing no longer expressible as one flat rate (conditional or tiered upstream, or its tiers moved): %s\n",
			strings.Join(p.ShapeChanged, ", "))
	}
	fmt.Fprintf(&b, "out of scope (providers Sinet runs no lane for): %d models\n", p.OutOfScope)
	if len(p.Structured) > 0 {
		fmt.Fprintf(&b, "conditional/tiered upstream pricing, priced by hand: %s\n", strings.Join(p.Structured, ", "))
	}
	b.WriteString("Nothing was applied: a price change lands only through operator confirmation, and a billing-regime change never auto-flips a flag (S10.2/S14.6).")
	return b.String()
}

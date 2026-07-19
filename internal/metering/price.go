package metering

import (
	"sort"
	"time"
)

// Currency is the reporting currency of a priced row (Spec S10.2, S10.10).
// Flat-rate lanes report API-EQUIVALENT dollars from the price table; metered
// lanes report REAL dollars. The currency visibly flips for any metered-
// flagged run (P-T01-3) — at v0 the metered-exception list is EMPTY, so every
// row is api-equivalent.
type Currency string

const (
	CurrencyAPIEquiv Currency = "api-equivalent"
	CurrencyReal     Currency = "real"
)

// MeteredExceptions is the set of lanes for which per-token dollar billing is
// explicitly enabled — the S10.2 "metered spending is opt-in only" rule.
//
// The list is EMPTY at v0 [G1 P7]: every lane is flat-rate, so scheduling and
// routing use consumption pressure and receipts report api-equivalent dollars
// (S10.2). DeepSeek is the pre-registered designated exception should one ever
// be enabled (S03.6), and a model whose overflow is auto-metered without a
// proven disable is rejected at onboarding (S03.6) — that onboarding gate is
// Spec S03's, referenced here for the coverage rule. Metered spending is never
// a default and never a silent fallback (14.3).
type MeteredExceptions struct {
	lanes map[string]bool
}

// NoMeteredExceptions is the v0 posture: nothing metered (S10.2, G1 P7).
func NoMeteredExceptions() MeteredExceptions { return MeteredExceptions{} }

// Metered reports whether a lane is on the explicit metered-exception list.
// At v0 this is always false.
func (m MeteredExceptions) Metered(lane string) bool { return m.lanes[lane] }

// Currency returns the reporting currency for a lane: real dollars only for an
// explicitly metered lane, api-equivalent otherwise (S10.2/S10.10).
func (m MeteredExceptions) Currency(lane string) Currency {
	if m.Metered(lane) {
		return CurrencyReal
	}
	return CurrencyAPIEquiv
}

// UnitPrices are per-token dollar prices for one {model, lane} at an effective
// date (Spec S10.3). Prices are per single token (not per million) to keep the
// arithmetic exact; the vendored genai-prices seed is converted at load.
type UnitPrices struct {
	InputUSD         float64
	OutputUSD        float64
	CacheReadUSD     float64
	CacheCreationUSD float64
	// ServerToolUSD prices one priced-per-use server-tool call (outside token
	// math, S10.1); zero when the lane has none.
	ServerToolUSD float64
}

func (p UnitPrices) allZero() bool {
	return p.InputUSD == 0 && p.OutputUSD == 0 && p.CacheReadUSD == 0 &&
		p.CacheCreationUSD == 0 && p.ServerToolUSD == 0
}

// PriceRow is one effective-dated price-table row (Spec S10.3): future-dated
// rows are first-class — a table without effective dates is guaranteed wrong
// on a known future date (P-T08-3).
type PriceRow struct {
	Model         string
	Lane          string
	Prices        UnitPrices
	EffectiveFrom time.Time
	VerifiedOn    time.Time
	Source        string
}

// PricedCost is the outcome of pricing one usage row (Spec S10.1 pricing
// group). Unpriced is the load-bearing honesty guard: an unpriceable row
// renders UNPRICED on the receipt and raises a drift alert — never a silent
// zero (P-T08-1). CostUSD is meaningful only when !Unpriced.
type PricedCost struct {
	Unpriced     bool
	Reason       string // why unpriced (shown with the UNPRICED label)
	CostUSD      float64
	Currency     Currency
	TableVersion string    // the priced-against table version (S10.1)
	EffectiveOn  time.Time // the effective-from date of the row used
	Source       string
}

// PriceTable is the pricing seam (Spec S10.3, D5). Dollars come ONLY from
// here; the engine's own figures are cross-checks, never billing truth
// (S10.1). The scheduler and routing NEVER price between flat-rate lanes with
// dollars (D5) — this table feeds receipts and the divergence cross-check.
type PriceTable interface {
	// Price prices a normalized usage row for {model, lane} as of ts. It
	// returns Unpriced (never a silent $0) when no row applies or when a
	// flat-rate lane would be priced from an all-zero row (the S10.3 coverage
	// guard). Version is the table version stamped on the priced cost.
	Price(model, lane string, ts time.Time, a Accounting, cur Currency) PricedCost
	// Version identifies the loaded table (S10.1 priced_cost.table_version).
	Version() string
}

// EffectiveDatedTable is the in-memory realization of the S10.3 price-table
// SEMANTICS: effective-dated rows, latest-effective-from-not-after-ts lookup,
// and the two coverage guards. It ships EMPTY at B1-2.
//
// The shipped defaults are the pinned genai-prices data.json, vendored [G2
// Def.16; D2.2 Layer B; S10.3] — a vendored-CONTENT S16.3 adoption (the
// manifest row carries TBD-P3(exact pin); it materializes with a funeral plan
// and operator ratification per CONVENTIONS §4). That seed, plus the user
// overlay / refresh-as-proposal / persistence machinery (S10.3), is a flagged
// follow-up: this stdlib-only packet pulls no new dependency, and an empty
// table prices every row UNPRICED — exactly S10.1's ratified honest behavior
// until the seed lands. Rows can be Add-ed programmatically (tests, and the
// loader when it lands).
type EffectiveDatedTable struct {
	version string
	rows    []PriceRow
}

// NewEffectiveDatedTable returns an empty table with a version label.
func NewEffectiveDatedTable(version string) *EffectiveDatedTable {
	return &EffectiveDatedTable{version: version}
}

// Add appends a price row (the loader/overlay path when it lands; tests use it
// directly). Rows are kept sorted so lookup picks the latest effective row.
func (t *EffectiveDatedTable) Add(rows ...PriceRow) {
	t.rows = append(t.rows, rows...)
	sort.SliceStable(t.rows, func(i, j int) bool {
		return t.rows[i].EffectiveFrom.Before(t.rows[j].EffectiveFrom)
	})
}

// Version implements PriceTable.
func (t *EffectiveDatedTable) Version() string { return t.version }

// Price implements PriceTable with the S10.3 semantics and coverage guards.
func (t *EffectiveDatedTable) Price(model, lane string, ts time.Time, a Accounting, cur Currency) PricedCost {
	row, ok := t.lookup(model, lane, ts)
	if !ok {
		// No applicable row → UNPRICED + drift alert (never $0, P-T08-1). The
		// empty-table v0 posture takes this path for every row.
		return PricedCost{Unpriced: true, Reason: "no price-table row for {model, lane} at date", Currency: cur, TableVersion: t.version}
	}
	// Never price a flat-rate lane's usage from an all-zero row (S10.3): the
	// vendored coding-plan rows price at $0 and would render every receipt $0.
	// An all-zero row for a flat-rate lane is UNPRICED, not $0.
	if cur == CurrencyAPIEquiv && row.Prices.allZero() {
		return PricedCost{Unpriced: true, Reason: "flat-rate lane would price from a $0 row (S10.3 coverage guard)", Currency: cur, TableVersion: t.version}
	}
	cost := float64(a.PromptTokensPricedInput())*row.Prices.InputUSD +
		float64(a.CacheReadTokens)*row.Prices.CacheReadUSD +
		float64(a.CacheCreationTokens)*row.Prices.CacheCreationUSD +
		float64(a.BilledOutputTokens)*row.Prices.OutputUSD +
		float64(a.ServerToolCalls)*row.Prices.ServerToolUSD
	return PricedCost{
		CostUSD:      cost,
		Currency:     cur,
		TableVersion: t.version,
		EffectiveOn:  row.EffectiveFrom,
		Source:       row.Source,
	}
}

// PromptTokensPricedInput is the input-token count priced at the INPUT rate:
// the true prompt minus the cached portions, which are priced at their own
// cache rates (Spec S10.1 accounting — cache_read/cache_creation are billed
// separately from fresh input).
func (a Accounting) PromptTokensPricedInput() int64 {
	n := a.PromptTokens - a.CacheReadTokens - a.CacheCreationTokens
	if n < 0 {
		return 0
	}
	return n
}

// lookup returns the row for {model, lane} with the latest EffectiveFrom not
// after ts (future-dated rows are ignored until their date, S10.3).
func (t *EffectiveDatedTable) lookup(model, lane string, ts time.Time) (PriceRow, bool) {
	var best PriceRow
	found := false
	for _, r := range t.rows {
		if r.Model != model || r.Lane != lane {
			continue
		}
		if r.EffectiveFrom.After(ts) {
			continue
		}
		if !found || r.EffectiveFrom.After(best.EffectiveFrom) {
			best, found = r, true
		}
	}
	return best, found
}

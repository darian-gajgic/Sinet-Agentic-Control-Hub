// Package metering is the S10 consumption layer beneath every run: the exact
// usage ledger (one row per paid call at the D7 checkpoint), the two-currency
// model, the consumption-pressure gauge, per-run receipts, and the observed
// Anthropic-header overlay. It meters the platform's OWN consumption exactly
// and never trusts a provider to report remaining quota (D4).
//
// This is NOT internal/ledger — that seam is the S05 Task Context Ledger
// (compiled working context), a different concept. The name "ledger" here
// means the consumption ledger of S10.1, which is a READ MODEL over the
// checkpoints table (Spec S02.4a): "the checkpoint's usage block IS the
// ledger row"; aggregations are queries over it, not new state (S10.1).
// Receipts (Spec S02.2 receipts family) are the one materialized artifact,
// written per run-end.
//
// Pricing comes only from Sinet's own effective-dated table (D5); engine
// dollar figures are cross-checks, never billing truth (S10.1). At B1-2 the
// price table ships EMPTY (its shipped genai-prices defaults are a vendored-
// content S16.3 adoption — see price.go), so every row prices UNPRICED
// (tier-5) — which is exactly S10.1's ratified honest behavior: "No row
// silently prices to $0."
package metering

import (
	"encoding/json"
	"math"
)

// PurposeTag classifies a usage row so ceremony is itemized separately from
// execution on receipts (Spec S10.1, feature 1.10/3.4). The four tags are
// coined in S10.1.
//
// At B1-2 the only classifier is PurposeExecution: a run is a single engine
// invocation, and the ceremony/verification distinction is a pipeline concept
// (intake Spec S06, verification Spec S07) that lands at B2. The itemization
// machinery is present now; the additional buckets populate when the pipeline
// tags stages. A caller that already knows a row's purpose supplies it.
type PurposeTag string

const (
	PurposeCeremony     PurposeTag = "ceremony"
	PurposeExecution    PurposeTag = "execution"
	PurposeVerification PurposeTag = "verification"
	PurposeProbe        PurposeTag = "probe"
)

// validPurpose reports whether tag is one of the four ratified tags.
func validPurpose(tag PurposeTag) bool {
	switch tag {
	case PurposeCeremony, PurposeExecution, PurposeVerification, PurposeProbe:
		return true
	}
	return false
}

// ApproximationTier is the 1..5 honesty rank of how a row's consumption was
// obtained, stamped on every row and shown on receipts (Spec S10.1 "the
// honest approximation hierarchy", best-first):
//
//	1 measured per-call usage (the normal case, both v0 subscription lanes + local)
//	2 engine-computed aggregates (cross-check tier; divergence alarm)
//	3 derived plan units (a flat plan's own units — requests-as-proxy)
//	4 tokenizer estimates (pre-run cost gates only, never a ledger fact)
//	5 unknown → flagged UNPRICED, never a silent zero (P-T08-1)
//
// The ledger stamps tier 1 when the checkpoint carries measured token counts,
// tier 5 when it does not. Tiers 2-4 attach at their own sources: tier 2 at
// the divergence cross-check (below), tier 3 at the plan-unit gauge
// (planunits.go — landed with the Z.AI lane at P3-LN-2B), tier 4 at the intake
// cost gate (Spec S06).
//
// Tier 3's unit is the PLAN's, and it is data: S10.1 wrote "Z.AI prompts"
// while that plan was denominated in prompts, and the plan is denominated in
// CREDITS as of 2026-08-23. The tier is unchanged — a derived plan unit is a
// derived plan unit — but nothing here names the unit, because the document
// that carries the numbers carries the unit too.
type ApproximationTier int

const (
	TierMeasured  ApproximationTier = 1
	TierAggregate ApproximationTier = 2
	TierDerived   ApproximationTier = 3
	TierTokenizer ApproximationTier = 4
	TierUnknown   ApproximationTier = 5
	tierFirst                       = TierMeasured
	tierLast                        = TierUnknown
)

func (t ApproximationTier) valid() bool { return t >= tierFirst && t <= tierLast }

// EngineUsage is the S02.4(a) usage block as stored by the adapter Driver
// (Spec S03/S02.4): the verbatim engine token counts for one paid call. The
// metering layer reads it back from the checkpoint's usage_json and NEVER
// re-derives it — the platform measures its own consumption exactly (D4).
//
// Field names mirror the Anthropic-surface object recorded at the pin
// (parse.go engineUsage): input_tokens counts only tokens after the last
// cache breakpoint, so the true prompt size is cache_read + cache_creation +
// input (Accounting.normalize below encodes this, S10.1).
type EngineUsage struct {
	InputTokens              int64            `json:"input_tokens"`
	OutputTokens             int64            `json:"output_tokens"`
	CacheReadInputTokens     int64            `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64            `json:"cache_creation_input_tokens"`
	CacheCreation            map[string]int64 `json:"cache_creation"` // by TTL bucket
	ThinkingTokens           int64            `json:"thinking_tokens"`
	ServerToolUse            *serverToolUse   `json:"server_tool_use"`

	// RequestID is the provider's own id for the call, carried through as the
	// reconciliation key to an operator's provider dashboard (Spec S03.7 names
	// it for the TBD-OPERATOR plan-unit calibration recipe). It is a
	// RECONCILIATION key and never a billing input: nothing here prices from
	// it, and a block that carries none carries none — an absent key is absent
	// (S10.1's honest-absence rule), never invented.
	RequestID string `json:"request_id,omitempty"`
}

// serverToolUse counts priced-per-use server tools (web search/fetch), which
// sit OUTSIDE token math (Spec S10.1; R09 §2.6).
type serverToolUse struct {
	WebSearchRequests int64 `json:"web_search_requests"`
	WebFetchRequests  int64 `json:"web_fetch_requests"`
}

// Accounting is the normalized, sanity-checked view of one paid call's
// consumption — the S10.1 "Accounting math is encoded, not assumed" rule made
// concrete so a row can neither double- nor under-count (P-T08-1).
type Accounting struct {
	// PromptTokens is the TRUE prompt size: cache_read + cache_creation +
	// input (input_tokens alone omits cached content on the Anthropic surface,
	// S10.1).
	PromptTokens int64
	// BilledOutputTokens folds thinking tokens into output — thinking is
	// billed as output (S10.1; R09 §2.6).
	BilledOutputTokens int64
	// CacheReadTokens / CacheCreationTokens are carried through for the
	// pressure gauge's cache weighting (Spec S10.4) and receipts.
	CacheReadTokens     int64
	CacheCreationTokens int64
	CacheCreationByTTL  map[string]int64
	// ServerToolCalls is the count of priced-per-use server tools, outside
	// token math (S10.1).
	ServerToolCalls int64
	// Measured reports whether the block carried recognizable token counts —
	// the tier-1 vs tier-5 discriminator (S10.1).
	Measured bool
	// RequestID is the provider's reconciliation key for this call (S03.7),
	// carried through the normalization unchanged. Empty when the block
	// carried none.
	RequestID string
}

// normalize decodes and normalizes an engine usage block. An empty or
// tokenless block is honestly non-measured (tier 5), never silently zeroed
// into a real reading.
func normalize(usageJSON json.RawMessage) Accounting {
	var eu EngineUsage
	if len(usageJSON) > 0 {
		// Tolerant decode: an unparsable block is treated as unknown, never
		// fatal (the ledger is a read model over already-persisted rows).
		_ = json.Unmarshal(usageJSON, &eu)
	}
	a := Accounting{
		PromptTokens:        eu.CacheReadInputTokens + eu.CacheCreationInputTokens + eu.InputTokens,
		BilledOutputTokens:  eu.OutputTokens + eu.ThinkingTokens,
		CacheReadTokens:     eu.CacheReadInputTokens,
		CacheCreationTokens: eu.CacheCreationInputTokens,
		CacheCreationByTTL:  eu.CacheCreation,
		RequestID:           eu.RequestID,
	}
	if eu.ServerToolUse != nil {
		a.ServerToolCalls = eu.ServerToolUse.WebSearchRequests + eu.ServerToolUse.WebFetchRequests
	}
	// A row is "measured" (tier 1) when it carries any recognizable token
	// signal; a bare {} or a block with no token fields is unknown (tier 5).
	a.Measured = a.PromptTokens > 0 || a.BilledOutputTokens > 0 || a.ServerToolCalls > 0
	return a
}

// tier returns the approximation tier implied by a normalized block at B1-2:
// measured → tier 1, otherwise unknown → tier 5 (Spec S10.1). Higher-fidelity
// intermediate tiers attach at their own sources (see ApproximationTier).
func (a Accounting) tier() ApproximationTier {
	if a.Measured {
		return TierMeasured
	}
	return TierUnknown
}

// absFrac returns |a-b|/max(|b|,epsilon) — the fractional divergence used by
// the tier-1-vs-tier-2 value alarm (Spec S10.1, ⚙ meter.value_divergence_alarm).
func absFrac(a, b float64) float64 {
	d := math.Abs(a - b)
	base := math.Abs(b)
	if base < 1e-12 {
		if d < 1e-12 {
			return 0
		}
		return math.Inf(1)
	}
	return d / base
}

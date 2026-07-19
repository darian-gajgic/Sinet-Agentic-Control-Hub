package metering

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestNormalizeAccounting(t *testing.T) {
	// S10.1: total prompt = cache_read + cache_creation + input; thinking is
	// billed as output; server tools counted outside token math.
	usage := json.RawMessage(`{
		"input_tokens": 10,
		"output_tokens": 4,
		"cache_read_input_tokens": 8000,
		"cache_creation_input_tokens": 120,
		"thinking_tokens": 30,
		"server_tool_use": {"web_search_requests": 2, "web_fetch_requests": 1}
	}`)
	a := normalize(usage)
	if a.PromptTokens != 8000+120+10 {
		t.Errorf("PromptTokens = %d, want %d (cache_read+cache_creation+input)", a.PromptTokens, 8130)
	}
	if a.BilledOutputTokens != 4+30 {
		t.Errorf("BilledOutputTokens = %d, want 34 (output + thinking)", a.BilledOutputTokens)
	}
	if a.ServerToolCalls != 3 {
		t.Errorf("ServerToolCalls = %d, want 3", a.ServerToolCalls)
	}
	if !a.Measured || a.tier() != TierMeasured {
		t.Errorf("expected measured/tier-1, got measured=%v tier=%d", a.Measured, a.tier())
	}
	if got := a.PromptTokensPricedInput(); got != 10 {
		t.Errorf("PromptTokensPricedInput = %d, want 10 (prompt - cached)", got)
	}
}

func TestNormalizeUnknownIsTier5(t *testing.T) {
	for _, in := range []string{"", "{}", `{"note":"no tokens"}`} {
		a := normalize(json.RawMessage(in))
		if a.Measured {
			t.Errorf("normalize(%q).Measured = true, want unknown/tier-5", in)
		}
		if a.tier() != TierUnknown {
			t.Errorf("normalize(%q).tier() = %d, want 5 (unknown, never a silent zero)", in, a.tier())
		}
	}
}

func TestPriceTableEmptyIsUnpriced(t *testing.T) {
	// The B1-2 v0 posture: an empty table prices every row UNPRICED — never a
	// silent $0 (S10.1 P-T08-1).
	pt := NewEffectiveDatedTable("empty-v0")
	a := normalize(json.RawMessage(`{"input_tokens":100,"output_tokens":50}`))
	pc := pt.Price("claude-sonnet-4-5", "anthropic", time.Now(), a, CurrencyAPIEquiv)
	if !pc.Unpriced {
		t.Fatalf("empty table priced a row: %+v (want UNPRICED)", pc)
	}
	if pc.CostUSD != 0 || pc.TableVersion != "empty-v0" {
		t.Errorf("unpriced cost = %+v", pc)
	}
}

func TestPriceTableFlatZeroRowGuard(t *testing.T) {
	// S10.3 coverage guard: never price a flat-rate lane from an all-zero row
	// (the coding-plan-$0 trap that would render every receipt $0).
	pt := NewEffectiveDatedTable("v1")
	pt.Add(PriceRow{Model: "glm-4.6", Lane: "zai", EffectiveFrom: day(2026, 1, 1), Source: "coding-plan"})
	a := normalize(json.RawMessage(`{"input_tokens":100}`))
	pc := pt.Price("glm-4.6", "zai", day(2026, 6, 1), a, CurrencyAPIEquiv)
	if !pc.Unpriced {
		t.Fatalf("flat-rate lane priced from a $0 row: %+v (S10.3 guard)", pc)
	}
}

func TestPriceTableEffectiveDating(t *testing.T) {
	pt := NewEffectiveDatedTable("v1")
	pt.Add(
		PriceRow{Model: "m", Lane: "anthropic", EffectiveFrom: day(2026, 1, 1),
			Prices: UnitPrices{InputUSD: 1e-6, OutputUSD: 5e-6}, Source: "old"},
		PriceRow{Model: "m", Lane: "anthropic", EffectiveFrom: day(2026, 8, 1),
			Prices: UnitPrices{InputUSD: 2e-6, OutputUSD: 10e-6}, Source: "new"},
	)
	a := normalize(json.RawMessage(`{"input_tokens":1000000,"output_tokens":1000000}`))

	// A date before the future row uses the old row; future-dated rows are
	// ignored until their date (P-T08-3).
	early := pt.Price("m", "anthropic", day(2026, 3, 1), a, CurrencyAPIEquiv)
	if early.Unpriced || early.Source != "old" {
		t.Fatalf("early price = %+v, want old row", early)
	}
	if early.CostUSD != 1.0+5.0 { // 1e6*1e-6 + 1e6*5e-6
		t.Errorf("early cost = %v, want 6.0", early.CostUSD)
	}
	// A later date crosses the future row.
	late := pt.Price("m", "anthropic", day(2026, 9, 1), a, CurrencyAPIEquiv)
	if late.Source != "new" || late.CostUSD != 2.0+10.0 {
		t.Errorf("late price = %+v, want new row cost 12.0", late)
	}
}

func TestMeteredExceptionsEmptyAtV0(t *testing.T) {
	ex := NoMeteredExceptions()
	for _, lane := range []string{"anthropic", "zai", "local"} {
		if ex.Metered(lane) {
			t.Errorf("lane %q metered at v0 (list must be EMPTY, G1 P7)", lane)
		}
		if ex.Currency(lane) != CurrencyAPIEquiv {
			t.Errorf("lane %q currency = %s, want api-equivalent", lane, ex.Currency(lane))
		}
	}
}

func TestParseUnifiedHeadersAndTrustGate(t *testing.T) {
	h := http.Header{}
	h.Set("Anthropic-Ratelimit-Unified-5h-Reset", "1893456000")
	h.Set("Anthropic-Ratelimit-Unified-5h-Utilization", "42")
	h.Set("Anthropic-Ratelimit-Unified-5h-Status", "allowed")
	h.Set("Anthropic-Ratelimit-Unified-7d-Reset", "1893456000")
	h.Set("Anthropic-Ratelimit-Unified-7d-Utilization", "10")
	h.Set("Anthropic-Ratelimit-Unified-7d-Oi-Utilization", "5")
	h.Set("Anthropic-Ratelimit-Unified-Overage-Status", "off")
	h.Set("X-Unrelated", "ignored")

	u, ok := ParseUnifiedHeaders(h)
	if !ok {
		t.Fatal("expected unified headers present")
	}
	if u.OverageStatus != "off" {
		t.Errorf("overage status = %q", u.OverageStatus)
	}
	if w, ok := u.Windows["5h"]; !ok || w.Status != "allowed" || w.Utilization != "42" {
		t.Errorf("5h window = %+v", w)
	}
	if w, ok := u.Windows["7d_oi"]; !ok || w.Utilization != "5" {
		t.Errorf("7d_oi window = %+v (weekly-Opus-input sub-limit, S10.4)", w)
	}
	if _, present := u.Raw["X-Unrelated"]; present {
		t.Error("captured an unrelated header")
	}
	// TRUST GATE: never trusted for park timing at B1-2 (TBD-BRINGUP).
	if u.TrustParkTiming() {
		t.Fatal("TrustParkTiming must be false until the S19.6 bring-up observation (S10.4)")
	}
	// The reset parses (untrusted) so the bring-up can compare it later.
	if _, ok := u.ResetFor("5h"); !ok {
		t.Error("expected a parsed (untrusted) 5h reset for the bring-up comparison")
	}
}

func TestParseUnifiedHeadersAbsent(t *testing.T) {
	if _, ok := ParseUnifiedHeaders(http.Header{}); ok {
		t.Error("no unified headers present, want ok=false (absent on the non-subscription surface / pre-proxy)")
	}
}

func day(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC) }

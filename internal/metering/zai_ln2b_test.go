package metering

// zai_ln2b_test.go — P3-LN-2B §9 specs 15-19 (S10.1, S10.3, S10.4, S03.7).
//
// The packet's easiest mistake is conflating two readings that share a run:
// the token row is MEASURED per-call usage (tier 1) and the plan-unit gauge is
// a DERIVED proxy (tier 3). Both are stamped, both are labeled, and neither is
// folded into the other.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// zaiPeak is a Wednesday inside the documented peak window (14:00-18:00 in
// UTC+8, i.e. 06:00-10:00 UTC). 2026-08-19 is a Wednesday.
var zaiPeak = time.Date(2026, 8, 19, 7, 0, 0, 0, time.UTC)

// zaiOffPeak is the same Wednesday two hours after the window closes.
var zaiOffPeak = time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)

// spec 15 — the tier split, on the same run.
func TestZAITokenRowIsTier1AndGaugeIsTier3(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.runningRun(t, "rz", "bob", "zai", "opencode")
	e.datedCheckpoint(t, "rz", "bob", "glm-5.3",
		`{"input_tokens":1200,"output_tokens":300,"cache_read_input_tokens":4000}`, zaiPeak)

	led := NewLedger(e.db, nil, NoMeteredExceptions(), e.reg)
	rc, err := led.RunConsumption(ctx, "rz")
	if err != nil {
		t.Fatalf("RunConsumption: %v", err)
	}
	if len(rc.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(rc.Items))
	}
	if rc.Items[0].Tier != TierMeasured {
		t.Errorf("zai ledger line tier = %d, want %d (measured per-call usage is tier 1, S10.1)",
			rc.Items[0].Tier, TierMeasured)
	}

	doc, ok := PlanDocFor("zai")
	if !ok {
		t.Fatal("no plan document for lane zai — the S10.4 denominator has nowhere to come from")
	}
	g := NewPressureGauge(e.db, e.reg)

	gauge, err := g.Read(ctx, "bob", "zai", UndeclaredBudget())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if gauge.Tier != TierMeasured {
		t.Errorf("token-side gauge tier = %d, want %d — WeightedConsumption is measured tokens",
			gauge.Tier, TierMeasured)
	}

	plan, err := g.ReadPlanUnits(ctx, "bob", "zai", doc, PlanQuotaWindow, zaiPeak.Add(time.Hour))
	if err != nil {
		t.Fatalf("ReadPlanUnits: %v", err)
	}
	if plan.Tier != TierDerived {
		t.Errorf("plan-unit reading tier = %d, want %d (derived plan units, S10.1 tier 3)",
			plan.Tier, TierDerived)
	}
	if !plan.Assumed {
		t.Error("the plan-unit reading carries no \"assumed\" label (S10.4)")
	}
	if plan.AssumedNote == "" {
		t.Error("the \"assumed\" label carries no reason")
	}
	// C-2: the unit changed with the plan. Receipts and gauges say CREDITS.
	if plan.Unit != "credits" {
		t.Errorf("plan unit = %q, want %q — the plan re-denominated from prompts to credits (verified 2026-08-23)", plan.Unit, "credits")
	}
	if plan.Calls != 1 {
		t.Errorf("plan reading counted %d calls, want 1", plan.Calls)
	}
	// Never conflated: the plan reading must not carry the token sum.
	if plan.Consumed == gauge.WeightedConsumption {
		t.Errorf("the tier-3 plan reading (%v) equals the tier-1 token sum (%v) — the two readings were folded together",
			plan.Consumed, gauge.WeightedConsumption)
	}
	if plan.VerifiedOn == "" {
		t.Error("the plan reading carries no verified-on date")
	}
}

// spec 16 — with no metered zai row, zai usage renders UNPRICED (tier 5), and
// a $0 coding-plan row does not rescue it.
func TestZAIUnpricedWithoutMeteredRow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.runningRun(t, "rz", "bob", "zai", "opencode")
	e.datedCheckpoint(t, "rz", "bob", "glm-5.3", `{"input_tokens":900,"output_tokens":100}`, zaiPeak)

	// The coding plan's own row prices at $0. It must never make a receipt look
	// complete (S10.3, verbatim).
	flat := NewEffectiveDatedTable("coding-plan-only")
	flat.Add(PriceRow{Model: "glm-5.3", Lane: "zai", EffectiveFrom: zaiPeak.Add(-24 * time.Hour),
		VerifiedOn: zaiPeak.Add(-24 * time.Hour), Source: "coding plan (flat)"})

	for name, table := range map[string]PriceTable{
		"empty table":        NewEffectiveDatedTable("empty"),
		"coding-plan $0 row": flat,
	} {
		rc, err := NewLedger(e.db, table, NoMeteredExceptions(), e.reg).RunConsumption(ctx, "rz")
		if err != nil {
			t.Fatalf("%s: RunConsumption: %v", name, err)
		}
		if len(rc.Items) != 1 {
			t.Fatalf("%s: items = %d, want 1", name, len(rc.Items))
		}
		if !rc.Items[0].Unpriced() {
			t.Errorf("%s: the zai line is not UNPRICED — a flat lane priced from a $0 row renders every receipt $0 (S10.3)", name)
		}
		if rc.Items[0].Tier != TierMeasured {
			t.Errorf("%s: tier = %d — UNPRICED is a PRICING verdict, never a downgrade of a measured row", name, rc.Items[0].Tier)
		}
		if rc.TotalPricedUSD != 0 || rc.TotalUnpricedCalls != 1 {
			t.Errorf("%s: priced=%v unpriced-calls=%d, want 0 / 1", name, rc.TotalPricedUSD, rc.TotalUnpricedCalls)
		}
	}

	// And the receipt says so, in api-equivalent (R27).
	led := NewLedger(e.db, NewEffectiveDatedTable("empty"), NoMeteredExceptions(), e.reg)
	rcpt, err := NewReceipts(e.db, led, NoMeteredExceptions()).Materialize(ctx, "rz")
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if rcpt.TotalUnpricedCalls != 1 || rcpt.TotalPricedUSD != 0 {
		t.Errorf("receipt: priced=%v unpriced=%d, want 0 / 1", rcpt.TotalPricedUSD, rcpt.TotalUnpricedCalls)
	}
	if rcpt.Currency != CurrencyAPIEquiv {
		t.Errorf("receipt currency = %q, want %q — a flat lane reports api-equivalent (S10.2/S10.10)", rcpt.Currency, CurrencyAPIEquiv)
	}
	if !rcpt.DirectUse.Unpriced {
		t.Error("the direct-use estimate does not carry the UNPRICED state")
	}
}

// spec 17 — the pricestore's zero-row refusal is intact and this packet adds no
// $0 zai row anywhere.
func TestZAIZeroPriceRowRefused(t *testing.T) {
	err := validatePriceRow(AddRowRequest{
		Actor: "op",
		Row: PriceRow{
			Model: "glm-5.3", Lane: "zai", Source: "coding plan",
			EffectiveFrom: zaiPeak, VerifiedOn: zaiPeak,
		},
	})
	if !errors.Is(err, ErrBadPriceRow) {
		t.Fatalf("an all-zero zai row was admitted (err = %v) — the UNPRICED posture is defeated from the inside (S10.1)", err)
	}
	// And no non-test source in this package ships one.
	for _, src := range meteringSources(t) {
		body := readFile(t, src)
		if strings.Contains(body, `Lane: "zai"`) || strings.Contains(body, `Lane:"zai"`) {
			t.Errorf("%s constructs a zai price row in code — price rows are the operator's (S10.3)", src)
		}
	}
}

// spec 18 — the multipliers and the quota denominators are DATA. Changing a row
// changes the gauge with no code edit.
func TestZAIMultipliersAreConfigNotConstants(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.runningRun(t, "rz", "bob", "zai", "opencode")
	e.datedCheckpoint(t, "rz", "bob", "glm-5.3", `{"input_tokens":10,"output_tokens":10}`, zaiOffPeak)

	g := NewPressureGauge(e.db, e.reg)
	seed, ok := PlanDocFor("zai")
	if !ok {
		t.Fatal("no seed plan document for lane zai")
	}

	base, err := g.ReadPlanUnits(ctx, "bob", "zai", seed, PlanQuotaWindow, zaiOffPeak.Add(time.Hour))
	if err != nil {
		t.Fatalf("ReadPlanUnits(seed): %v", err)
	}
	if base.Multiplier != 0.5 {
		t.Errorf("off-peak multiplier = %v, want 0.5 (verified 2026-08-23: \"charged at 50%% of the standard credit rate\")", base.Multiplier)
	}
	if base.QuotaUnits != 28000 {
		t.Errorf("rolling-window quota = %v, want 28000 credits (GLM Coding Max, verified 2026-08-23)", base.QuotaUnits)
	}

	// Now edit the DATA and nothing else.
	edited := editPlanDoc(t, seed, func(m map[string]any) {
		for _, raw := range m["multipliers"].([]any) {
			row := raw.(map[string]any)
			if row["name"] == "off-peak" {
				row["factor"] = 0.25
			}
		}
		for _, raw := range m["quotas"].([]any) {
			row := raw.(map[string]any)
			if row["name"] == PlanQuotaWindow {
				row["units"] = 14000.0
			}
		}
	})
	moved, err := g.ReadPlanUnits(ctx, "bob", "zai", edited, PlanQuotaWindow, zaiOffPeak.Add(time.Hour))
	if err != nil {
		t.Fatalf("ReadPlanUnits(edited): %v", err)
	}
	if moved.Consumed == base.Consumed {
		t.Errorf("halving the off-peak multiplier did not move the reading (%v both times)", base.Consumed)
	}
	if moved.Pressure == base.Pressure {
		t.Errorf("halving the quota did not move the pressure (%v both times)", base.Pressure)
	}
	for _, r := range []PlanReading{base, moved} {
		if !r.Assumed {
			t.Error("a plan reading is not labeled \"assumed\"")
		}
	}

	// No quota number and no multiplier is a Go constant.
	for _, src := range meteringSources(t) {
		body := readFile(t, src)
		for _, banned := range []string{"28000", "140000", "14:00", "Singapore"} {
			if strings.Contains(body, banned) {
				t.Errorf("%s hardcodes %q — the plan's numbers are dated DATA, never constants (S10.4/S18.3)", src, banned)
			}
		}
	}
}

// spec 19 — the dashboard reconciliation key survives onto the row (S03.7).
func TestZAIReceiptCarriesRequestIDForReconciliation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.runningRun(t, "rz", "bob", "zai", "opencode")
	e.datedCheckpoint(t, "rz", "bob", "glm-5.3",
		`{"input_tokens":10,"output_tokens":5,"request_id":"req-zai-0001"}`, zaiPeak)
	e.datedCheckpoint(t, "rz", "bob", "glm-5.3",
		`{"input_tokens":10,"output_tokens":5,"request_id":"req-zai-0002"}`, zaiPeak.Add(time.Minute))

	led := NewLedger(e.db, nil, NoMeteredExceptions(), e.reg)
	ids, err := led.RequestIDs(ctx, "rz")
	if err != nil {
		t.Fatalf("RequestIDs: %v", err)
	}
	want := []string{"req-zai-0001", "req-zai-0002"}
	if len(ids) != len(want) {
		t.Fatalf("request ids = %v, want %v", ids, want)
	}
	for i, w := range want {
		if ids[i] != w {
			t.Errorf("request id[%d] = %q, want %q", i, ids[i], w)
		}
	}
	// A block with no request_id is not an error and mints nothing.
	e.runningRun(t, "rn", "bob", "zai", "opencode")
	e.datedCheckpoint(t, "rn", "bob", "glm-5.3", `{"input_tokens":10,"output_tokens":5}`, zaiPeak)
	none, err := led.RequestIDs(ctx, "rn")
	if err != nil {
		t.Fatalf("RequestIDs(no key): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("request ids = %v, want none — an absent key is absent, never invented", none)
	}
}

// editPlanDoc round-trips a seed document through JSON with an edit applied, so
// the test changes DATA and never code.
func editPlanDoc(t *testing.T, seed PlanDoc, edit func(map[string]any)) PlanDoc {
	t.Helper()
	raw, err := json.Marshal(seed)
	if err != nil {
		t.Fatalf("marshal plan doc: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode plan doc: %v", err)
	}
	edit(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("re-encode plan doc: %v", err)
	}
	doc, err := LoadPlanDoc(out)
	if err != nil {
		t.Fatalf("LoadPlanDoc(edited): %v", err)
	}
	return doc
}

func meteringSources(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		out = append(out, filepath.Join(".", n))
	}
	if len(out) == 0 {
		t.Fatal("the source scan found no files — it would pass vacuously")
	}
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

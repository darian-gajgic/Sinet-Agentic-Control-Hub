package metering

// plan_ln3_test.go — P3-LN-3 §6 spec 3 + OQ2 (S10.1, S10.4, S16.6).
//
// The kimi plan document, and the shape the zai document never needed: this
// lane's rolling 5-hour window counts REQUESTS and its 7-day window counts
// CREDITS. A single lane-wide unit cannot describe it, and rendering both
// gauges in one unit would be a quiet lie about what was counted.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func kimiPlanSeedBytes(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("plandata/kimi.json")
	if err != nil {
		t.Fatalf("read the shipped kimi plan document: %v", err)
	}
	return raw
}

// ── spec 3 · every plan document loads; a duplicate lane is refused ──────────

func TestPlanSeedLoadsEveryDocument(t *testing.T) {
	docs, err := SeedPlanDocs()
	if err != nil {
		t.Fatalf("SeedPlanDocs: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("the platform ships %d plan documents, want 2 (kimi + zai)", len(docs))
	}
	if docs[0].Lane != "kimi" || docs[1].Lane != "zai" {
		t.Errorf("plan documents = %q/%q, want them sorted by lane name", docs[0].Lane, docs[1].Lane)
	}
	for _, lane := range []string{"kimi", "zai"} {
		if _, ok := PlanDocFor(lane); !ok {
			t.Errorf("no plan document for lane %q", lane)
		}
	}

	one := `{
	  "lane": "%s", "plan": "P", "unit": "credits", "verified_on": "2026-08-24",
	  "assumed_note": "assumed",
	  "multipliers": [{"name": "standard", "factor": 1, "default": true, "verified_on": "2026-08-24"}],
	  "quotas": [{"name": "rolling-5h", "units": 10, "window_hours": 5, "verified_on": "2026-08-24"}]
	}`
	ok := fstest.MapFS{
		"plandata/b.json": {Data: []byte(strings.Replace(one, "%s", "beta", 1))},
		"plandata/a.json": {Data: []byte(strings.Replace(one, "%s", "alpha", 1))},
	}
	got, err := loadPlanDocs(ok, "plandata")
	if err != nil {
		t.Fatalf("two distinct plan documents did not load: %v", err)
	}
	if len(got) != 2 || got[0].Lane != "alpha" || got[1].Lane != "beta" {
		t.Fatalf("loaded %+v, want both sorted by lane name", got)
	}

	dup := fstest.MapFS{
		"plandata/a.json": {Data: []byte(strings.Replace(one, "%s", "alpha", 1))},
		"plandata/b.json": {Data: []byte(strings.Replace(one, "%s", "alpha", 1))},
	}
	_, err = loadPlanDocs(dup, "plandata")
	if err == nil {
		t.Fatal("two plan documents claiming the same lane loaded — PlanDocFor takes the first match, so the second would be silently unreachable")
	}
	if !errors.Is(err, ErrPlanDoc) {
		t.Errorf("error = %v, want it to wrap ErrPlanDoc", err)
	}
	if !strings.Contains(err.Error(), "alpha") {
		t.Errorf("the refusal does not name the duplicated lane: %v", err)
	}
}

// ── OQ2 · each quota window carries its OWN unit ─────────────────────────────

func TestKimiPlanWindowsCarryTheirOwnUnits(t *testing.T) {
	doc, ok := PlanDocFor("kimi")
	if !ok {
		t.Fatal("no plan document for lane kimi")
	}
	if doc.VerifiedOn != "2026-08-24" {
		t.Errorf("verified_on = %q, want the audit's access date", doc.VerifiedOn)
	}
	// The shared-pool reason is sharper than the zai lane's and must ride every
	// reading: Sinet's own count is a LOWER BOUND, never the pool state.
	if !strings.Contains(doc.AssumedNote, "share the same quota") {
		t.Errorf("the assumed-note does not quote the shared-quota fact: %q", doc.AssumedNote)
	}
	if !strings.Contains(strings.ToLower(doc.AssumedNote), "lower bound") {
		t.Errorf("the assumed-note does not say Sinet's count is a lower bound on consumption: %q", doc.AssumedNote)
	}

	five, ok := doc.Quota(PlanQuotaWindow)
	if !ok {
		t.Fatal("the kimi document declares no rolling-5h quota")
	}
	week, ok := doc.Quota(PlanQuotaWeekly)
	if !ok {
		t.Fatal("the kimi document declares no weekly quota")
	}
	if doc.QuotaUnit(PlanQuotaWindow) != "requests" {
		t.Errorf("rolling-5h unit = %q, want requests — the 5-hour window counts requests", doc.QuotaUnit(PlanQuotaWindow))
	}
	if doc.QuotaUnit(PlanQuotaWeekly) != "credits" {
		t.Errorf("weekly unit = %q, want credits — the 7-day window counts credits/tokens", doc.QuotaUnit(PlanQuotaWeekly))
	}
	if doc.QuotaUnit(PlanQuotaWindow) == doc.QuotaUnit(PlanQuotaWeekly) {
		t.Error("both kimi windows report one unit — that is the failure this requirement exists to prevent")
	}
	if five.Units <= 0 {
		t.Errorf("rolling-5h declares %v units — the published range is 300-1,200 and the low end is the conservative seed", five.Units)
	}
	// The weekly allowance is UNVERIFIED at primary grade (audit U1/U2), so the
	// row declares its SHAPE and its UNIT and honestly declares no number.
	// Seeding the aggregators' 1x/5x/15x/30x figures would promote secondary
	// repetition to fact.
	if !week.AllowanceUnverified {
		t.Error("the weekly row claims a verified allowance — no primary page publishes one for any tier")
	}
	if week.Units != 0 {
		t.Errorf("the weekly row carries %v units while declaring its allowance unverified — one of the two is a lie", week.Units)
	}
	if week.WindowHours != 168 {
		t.Errorf("weekly window = %v hours, want 168 (the published 7-day cycle)", week.WindowHours)
	}
	if !strings.Contains(strings.ToLower(doc.WeeklyReset), "subscription") {
		t.Errorf("weekly_reset = %q, want the subscription-anchored cycle — it is not a calendar week", doc.WeeklyReset)
	}
	if !strings.Contains(doc.TierMultiplierNote, "UNVERIFIED") {
		t.Errorf("the document does not record that the per-tier multipliers are unverified: %q", doc.TierMultiplierNote)
	}

	// The zai document must stay expressible UNCHANGED: both its windows are
	// credits, it declares no per-quota unit, and each inherits the lane-wide
	// one. This is the control that the extension is additive.
	zai, ok := PlanDocFor("zai")
	if !ok {
		t.Fatal("no plan document for lane zai")
	}
	for _, name := range []string{PlanQuotaWindow, PlanQuotaWeekly} {
		q, ok := zai.Quota(name)
		if !ok {
			t.Fatalf("the zai document lost its %q quota", name)
		}
		if q.Unit != "" {
			t.Errorf("zai quota %q now declares its own unit %q — the seed was not to change", name, q.Unit)
		}
		if zai.QuotaUnit(name) != "credits" {
			t.Errorf("zai quota %q resolves to unit %q, want the document's own credits", name, zai.QuotaUnit(name))
		}
		if q.AllowanceUnverified {
			t.Errorf("zai quota %q became allowance-unverified — its numbers are published", name)
		}
	}
}

// ── the reading reports the unit of the window it was DERIVED from ───────────

func TestPlanReadingUnitFollowsItsWindow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	e.runningRun(t, "rk", "bob", "kimi", "opencode")
	e.datedCheckpoint(t, "rk", "bob", "k3", `{"input_tokens":900,"output_tokens":100}`, at)

	doc, ok := PlanDocFor("kimi")
	if !ok {
		t.Fatal("no plan document for lane kimi")
	}
	g := NewPressureGauge(e.db, e.reg)

	// Seeded from the 5-hour REQUESTS window: the reading says requests.
	budget, err := g.ProposePlanBudget(doc, PlanQuotaWindow, at.Add(-time.Hour))
	if err != nil {
		t.Fatalf("ProposePlanBudget(rolling-5h): %v", err)
	}
	r, err := g.ReadPlanUnits(ctx, "bob", "kimi", doc, budget, at.Add(time.Minute))
	if err != nil {
		t.Fatalf("ReadPlanUnits: %v", err)
	}
	if r.Unit != "requests" {
		t.Errorf("reading unit = %q, want requests — the reading is derived from the 5-hour window and must say what IT counts", r.Unit)
	}
	if r.Tier != TierDerived {
		t.Errorf("tier = %d, want %d (derived)", r.Tier, TierDerived)
	}
	if !r.Assumed || r.AssumedNote == "" {
		t.Error("the reading is not labeled assumed with its reason")
	}
	if r.Calls != 1 {
		t.Errorf("calls = %d, want 1", r.Calls)
	}

	// Every declared window rides the reading with its OWN unit, so a surface
	// can never render a credit window in requests.
	if len(r.Windows) != 2 {
		t.Fatalf("the reading reports %d windows, want both declared ones", len(r.Windows))
	}
	byName := map[string]PlanWindow{}
	for _, w := range r.Windows {
		byName[w.Name] = w
	}
	if byName[PlanQuotaWindow].Unit != "requests" || byName[PlanQuotaWeekly].Unit != "credits" {
		t.Errorf("windows = %+v, want rolling-5h in requests and weekly in credits", r.Windows)
	}
	if !byName[PlanQuotaWeekly].AllowanceUnverified {
		t.Error("the weekly window's unverified allowance is not reported — a surface would render a 0 allowance as a fact")
	}

	// A budget can NEVER be proposed from a window whose allowance nobody
	// published: there is nothing to take a fraction of.
	if _, err := g.ProposePlanBudget(doc, PlanQuotaWeekly, at); err == nil {
		t.Error("a budget was proposed from the unverified weekly allowance — a denominator invented from an unpublished number is exactly what S10.4/D4 forbids")
	} else if !strings.Contains(err.Error(), "weekly") {
		t.Errorf("the refusal does not name the quota: %v", err)
	}

	// And the zai lane's reading is unchanged: credits, both windows.
	zaiDoc, _ := PlanDocFor("zai")
	e.runningRun(t, "rz", "bob", "zai", "opencode")
	e.datedCheckpoint(t, "rz", "bob", "glm-5.3", `{"input_tokens":10,"output_tokens":10}`, at)
	zr, err := g.ReadPlanUnits(ctx, "bob", "zai", zaiDoc, UndeclaredPlanBudget(), at.Add(time.Minute))
	if err != nil {
		t.Fatalf("ReadPlanUnits(zai): %v", err)
	}
	if zr.Unit != "credits" {
		t.Errorf("zai reading unit = %q, want credits — the zai document did not change", zr.Unit)
	}
	for _, w := range zr.Windows {
		if w.Unit != "credits" {
			t.Errorf("zai window %q reports unit %q, want credits", w.Name, w.Unit)
		}
	}
}

// ── the kimi plan document's negatives ───────────────────────────────────────

func TestKimiPlanDocumentRefusals(t *testing.T) {
	if _, err := LoadPlanDoc(kimiPlanSeedBytes(t)); err != nil {
		t.Fatalf("the unedited kimi plan document must load: %v", err)
	}
	edit := func(f func(map[string]any)) []byte {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal(kimiPlanSeedBytes(t), &m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		f(m)
		out, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		return out
	}
	quota := func(m map[string]any, i int) map[string]any {
		rows, _ := m["quotas"].([]any)
		row, _ := rows[i].(map[string]any)
		if row == nil {
			t.Fatalf("quota row %d is not an object", i)
		}
		return row
	}

	for _, tc := range []struct {
		name  string
		f     func(map[string]any)
		wants []string
	}{
		{
			name: "a quota with no allowance and no unverified flag",
			f: func(m map[string]any) {
				q := quota(m, 1)
				delete(q, "allowance_unverified")
			},
			wants: []string{"weekly", "units"},
		},
		{
			name: "a quota that declares BOTH an allowance and that it is unverified",
			f: func(m map[string]any) {
				q := quota(m, 1)
				q["units"] = 140000.0
			},
			wants: []string{"weekly", "allowance_unverified"},
		},
		{
			name:  "an undated quota row",
			f:     func(m map[string]any) { delete(quota(m, 0), "verified_on") },
			wants: []string{"rolling-5h", "verified-on"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadPlanDoc(edit(tc.f))
			if err == nil {
				t.Fatal("the document loaded — the gate does not exist")
			}
			if !errors.Is(err, ErrPlanDoc) {
				t.Errorf("error = %v, want it to wrap ErrPlanDoc", err)
			}
			for _, want := range tc.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal does not name %q: %v", want, err)
				}
			}
		})
	}
}

package worker

// zai_ln2b_test.go — P3-LN-2B §9 specs 24-26 (S08.8, S10.2, D5; CONVENTIONS §26).
//
// In-package because resolveSeat is unexported, following routing_local_test.go.

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// fixedPressure is the D5 ordering input: normalized consumption, never
// dollars. A negative value stands for "no budget declared on this lane".
type fixedPressure map[string]float64

func (p fixedPressure) Pressure(_ context.Context, _, lane string) (LanePressure, error) {
	v, ok := p[lane]
	if !ok {
		return LanePressure{}, fmt.Errorf("no pressure reading for lane %q", lane)
	}
	if v < 0 {
		return LanePressure{Applicable: false}, nil
	}
	return LanePressure{Ratio: v, Applicable: true, Unit: "test units"}, nil
}

// zaiSeats is the alternate-seat data a commissioned zai lane contributes. It
// is built HERE from literals because that is exactly the point: seats are
// data the composition root reads out of a lane document, not a constant the
// routing package ships.
func zaiSeats() AlternateSeats {
	return AlternateSeatsFor(LaneSeat{Lane: "zai", Model: "glm-5.3"})
}

// spec 24 — a zai seat resolves under coverage; without it the 2.7 gap advice
// is unchanged.
func TestZAISeatResolvesUnderCoverage(t *testing.T) {
	ctx := context.Background()

	covered := &Router{
		DutyMap:    DefaultDutyMap(),
		Alternates: zaiSeats(),
		Coverage:   Coverage{FlatRateLanes: []string{"zai"}},
	}
	seat, _, reason, gap, err := covered.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		t.Fatalf("resolveSeat(zai covered): %v", err)
	}
	if gap != "" {
		t.Fatalf("zai is covered yet the gap advice fired: %q", gap)
	}
	if seat.Lane != "zai" {
		t.Errorf("seat lane = %q, want zai — the zai seat is duty-map DATA gated by coverage (S08.8 step 3)", seat.Lane)
	}
	if seat.Model == "" {
		t.Error("the zai seat resolves to no model")
	}
	if seat.WindowTokens <= 0 {
		t.Error("the zai seat carries no context window")
	}
	if !strings.Contains(strings.ToLower(reason), "zai") {
		t.Errorf("the plain reason does not name the lane it chose: %q", reason)
	}

	// An owner with the anthropic subscription only keeps the old seat exactly.
	anthropicOnly := &Router{
		DutyMap:    DefaultDutyMap(),
		Alternates: zaiSeats(),
		Coverage:   Coverage{FlatRateLanes: []string{"anthropic"}},
	}
	seat, _, _, gap, err = anthropicOnly.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil || gap != "" {
		t.Fatalf("anthropic-only: err=%v gap=%q", err, gap)
	}
	if seat.Lane != "anthropic" || seat.Model != DefaultDutyMap()[DutyExecution].Model {
		t.Errorf("anthropic-only resolved to %+v — the pre-LN-2 seat must be untouched", seat)
	}

	// An owner with neither lane still gets the 2.7 advice, unchanged.
	none := &Router{
		DutyMap:    DefaultDutyMap(),
		Alternates: zaiSeats(),
		Coverage:   Coverage{},
	}
	_, _, _, gap, err = none.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		t.Fatalf("no coverage: %v", err)
	}
	if !strings.Contains(gap, "Subscription gap (2.7)") {
		t.Errorf("gap advice = %q, want the unchanged 2.7 leg", gap)
	}
}

// spec 25 — among flat lanes selection follows PRESSURE, and a price
// difference changes nothing. Pinned as a property, not an example (D5).
func TestFlatLaneSelectionIgnoresDollars(t *testing.T) {
	ctx := context.Background()
	both := Coverage{FlatRateLanes: []string{"anthropic", "zai"}}

	// Same world, opposite pressures: the choice must follow the gauge.
	for _, tc := range []struct {
		name     string
		pressure fixedPressure
		wantLane string
	}{
		{"anthropic is the more consumed lane", fixedPressure{"anthropic": 0.91, "zai": 0.10}, "zai"},
		{"zai is the more consumed lane", fixedPressure{"anthropic": 0.10, "zai": 0.91}, "anthropic"},
	} {
		r := &Router{DutyMap: DefaultDutyMap(), Alternates: zaiSeats(),
			Coverage: both, Pressure: tc.pressure}
		seat, _, reason, gap, err := r.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
		if err != nil || gap != "" {
			t.Fatalf("%s: err=%v gap=%q", tc.name, err, gap)
		}
		if seat.Lane != tc.wantLane {
			t.Errorf("%s: chose lane %q, want %q — selection between flat lanes is the pressure gauge's (S08.8; D5)",
				tc.name, seat.Lane, tc.wantLane)
		}
		if !strings.Contains(strings.ToLower(reason), "pressure") {
			t.Errorf("%s: the plain reason does not say the choice was made on consumption pressure: %q", tc.name, reason)
		}
	}

	// No declared budget on a lane ⇒ deterministic duty-map order, never a
	// raw count (drain r1 D3: a raw lifetime total hands every dispatch to
	// whichever lane was added most recently, forever).
	for _, tc := range []struct {
		name     string
		pressure fixedPressure
	}{
		{"neither lane has a budget", fixedPressure{"anthropic": -1, "zai": -1}},
		{"only the alternate has one", fixedPressure{"anthropic": -1, "zai": 0.01}},
	} {
		r := &Router{DutyMap: DefaultDutyMap(), Alternates: zaiSeats(),
			Coverage: both, Pressure: tc.pressure}
		seat, _, reason, gap, err := r.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
		if err != nil || gap != "" {
			t.Fatalf("%s: err=%v gap=%q", tc.name, err, gap)
		}
		if seat.Lane != DefaultDutyMap()[DutyExecution].Lane {
			t.Errorf("%s: chose %q, want the deterministic duty-map seat when no comparable ratio exists", tc.name, seat.Lane)
		}
		if !strings.Contains(reason, "no declared automation budget") {
			t.Errorf("%s: the reason does not say WHY the order was deterministic: %q", tc.name, reason)
		}
	}

	// The property: no dollar figure can even enter selection. Every input type
	// the router takes is scanned for a money-shaped member.
	for _, typ := range []reflect.Type{
		reflect.TypeOf(Router{}), reflect.TypeOf(Coverage{}), reflect.TypeOf(Seat{}),
		reflect.TypeOf(DutyMap{}), reflect.TypeOf(AlternateSeats{}),
		reflect.TypeOf(ExecutionProfile{}), reflect.TypeOf(RouteQuery{}),
	} {
		for _, name := range moneyMembers(typ) {
			t.Errorf("%s carries a money-shaped member %q — dollar-based routing between flat-rate lanes is a D5 violation and is NEVER done (S10.2)",
				typ.Name(), name)
		}
	}

	// And no selection source names one either.
	body := readWorkerSource(t, "routing.go")
	for _, banned := range []string{"PricedUSD", "CostUSD", "PriceTable", "metering."} {
		if strings.Contains(body, banned) {
			t.Errorf("routing.go names %q — no price, cost or dollar figure enters this package's selection inputs (D5)", banned)
		}
	}
}

// D5 · No lane value is a Go constant OUTSIDE the lane document — the scan the
// opencode package runs on itself, extended to the packages that consume lane
// data. A model id in routing goes stale invisibly; a dated row does not.
func TestNoLaneValueIsAConstantInRoutingOrShell(t *testing.T) {
	scanned := 0
	for _, dir := range []string{".", "../shell", "../stage"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
				continue
			}
			scanned++
			body, err := os.ReadFile(dir + "/" + n)
			if err != nil {
				t.Fatalf("read %s/%s: %v", dir, n, err)
			}
			// Extended at P3-LN-3 with the kimi lane's own values. `"k3"` is
			// matched WITH its quotes: the ban is on a model id compiled in as
			// a Go constant, not on the two characters appearing in prose.
			for _, banned := range []string{
				"glm-", "api.z.ai", "zai-coding-plan",
				"api.kimi.com", "api.moonshot", "kimi-for-coding", `"k3"`,
			} {
				if strings.Contains(string(body), banned) {
					t.Errorf("%s/%s hardcodes %q — lane values live in the lane DOCUMENT with their verified-on dates (S03.6)", dir, n, banned)
				}
			}
		}
	}
	if scanned == 0 {
		t.Fatal("the lane-constant scan read no files — it would pass vacuously")
	}
}

// spec 26 — the utility duty still degrades to the paid seat, with the
// CORRECTED reason: a second adapter now exists; what is absent is a
// commissioned local provider entry.
func TestUtilityDutyStillDegradesWithCorrectedReason(t *testing.T) {
	ctx := context.Background()
	for _, localUp := range []bool{true, false} {
		r := &Router{
			DutyMap:    DefaultDutyMap(),
			Alternates: zaiSeats(),
			Coverage:   Coverage{FlatRateLanes: []string{"anthropic"}, LocalAvailable: localUp},
		}
		seat, _, reason, gap, err := r.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyUtility})
		if err != nil {
			t.Fatalf("LocalAvailable=%v: %v", localUp, err)
		}
		if gap != "" {
			t.Fatalf("LocalAvailable=%v: unexpected gap %q", localUp, gap)
		}
		if seat.Lane != "anthropic" {
			t.Errorf("LocalAvailable=%v: utility resolved to lane %q, want the paid execution seat", localUp, seat.Lane)
		}
		if strings.Contains(reason, "no second adapter") {
			t.Errorf("LocalAvailable=%v: the reason still claims no second adapter exists — LN-1 landed one: %q", localUp, reason)
		}
		if localUp && !strings.Contains(strings.ToLower(reason), "provider entry") {
			t.Errorf("LocalAvailable=%v: the corrected reason does not name the absent local PROVIDER ENTRY: %q", localUp, reason)
		}
	}

	// The three sources that repeated the stale clause carry the dated
	// correction now.
	for _, f := range []string{"routing.go", "ceiling.go"} {
		if strings.Contains(readWorkerSource(t, f), "no second adapter exists") {
			t.Errorf("%s still asserts \"no second adapter exists\" — the clause is FALSE since P3-LN-1", f)
		}
	}
}

// moneyMembers reports the money-shaped exported members of a type.
//
// It follows maps, slices and pointers to the struct underneath (drain r1 D10:
// scanning DutyMap/AlternateSeats as bare map kinds found nothing at all, which
// is a scan that passes because it looked nowhere).
func moneyMembers(t reflect.Type) []string {
	for t.Kind() == reflect.Map || t.Kind() == reflect.Slice ||
		t.Kind() == reflect.Array || t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var out []string
	for i := 0; i < t.NumField(); i++ {
		n := strings.ToLower(t.Field(i).Name)
		for _, needle := range []string{"price", "cost", "usd", "dollar", "spend"} {
			if strings.Contains(n, needle) {
				out = append(out, t.Field(i).Name)
			}
		}
	}
	return out
}

func readWorkerSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

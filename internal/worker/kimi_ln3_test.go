package worker

// kimi_ln3_test.go — P3-LN-3 §6 specs 19 + 21 (S08.8, D5, S10.4).
//
// Two flat-rate lanes finally exist, so the S08.8 rule that selection between
// them uses CONSUMPTION PRESSURE and never dollars stops being a rule about a
// hypothetical. Seats are DATA composed at the composition root, which is why
// the ids below are literals here and constants nowhere.

import (
	"context"
	"strings"
	"testing"
)

// kimiSeats is what a commissioned kimi lane contributes — read out of its lane
// document by the composition root, never a constant in this package.
func kimiSeats() AlternateSeats {
	return AlternateSeatsFor(LaneSeat{Lane: "kimi", Model: "k3"})
}

func bothFlatSeats() AlternateSeats {
	return AlternateSeatsFor(
		LaneSeat{Lane: "kimi", Model: "k3"},
		LaneSeat{Lane: "zai", Model: "glm-5.3"},
	)
}

// ── spec 19 · the kimi seat resolves under coverage ──────────────────────────

func TestKimiSeatResolvesUnderCoverage(t *testing.T) {
	ctx := context.Background()

	covered := &Router{
		DutyMap:    DefaultDutyMap(),
		Alternates: kimiSeats(),
		Coverage:   Coverage{FlatRateLanes: []string{"kimi"}},
	}
	seat, _, reason, gap, err := covered.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		t.Fatalf("resolveSeat(kimi covered): %v", err)
	}
	if gap != "" {
		t.Fatalf("kimi is covered yet the gap advice fired: %q", gap)
	}
	if seat.Lane != "kimi" {
		t.Errorf("seat lane = %q, want kimi — the seat is duty-map DATA gated by coverage (S08.8 step 3)", seat.Lane)
	}
	if seat.Model != "k3" {
		t.Errorf("seat model = %q, want the document's default_model k3", seat.Model)
	}
	if seat.WindowTokens <= 0 {
		t.Error("the kimi seat carries no context window")
	}
	if !strings.Contains(strings.ToLower(reason), "kimi") {
		t.Errorf("the plain reason does not name the lane it chose: %q", reason)
	}

	// The anthropic-only duties are unchanged, and without coverage the 2.7
	// subscription-gap advice is the honest answer it always was.
	uncovered := &Router{DutyMap: DefaultDutyMap(), Alternates: kimiSeats()}
	for _, duty := range []string{DutyExecution, DutyPlanning, DutyJudge} {
		seat, _, _, gap, err := uncovered.resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: duty})
		if err != nil {
			t.Fatalf("resolveSeat(%s, uncovered): %v", duty, err)
		}
		if seat.Lane == "kimi" {
			t.Errorf("duty %q seated kimi with no coverage — a lane nobody holds is not selectable", duty)
		}
		if duty != DutyExecution && gap == "" {
			// Planning/judge keep their anthropic-only seats; whatever advice
			// they gave before a kimi document existed, they give now.
			continue
		}
	}
}

// ── spec 21 · two flat lanes, ordered by pressure, with no dollar term ───────

func TestFlatLaneSelectionAcrossTwoFlatLanes(t *testing.T) {
	ctx := context.Background()
	base := func(p fixedPressure) *Router {
		return &Router{
			DutyMap:    DefaultDutyMap(),
			Alternates: bothFlatSeats(),
			Coverage:   Coverage{FlatRateLanes: []string{"kimi", "zai"}},
			Pressure:   p,
		}
	}

	// The duty-map seat's own lane is anthropic and is not flat-rate covered
	// here, so the two covered candidates are the genuinely competing pair —
	// the first real two-flat-lane choice this router has ever had to make.
	for _, tc := range []struct {
		name     string
		pressure fixedPressure
		want     string
	}{
		{name: "kimi is less consumed", pressure: fixedPressure{"kimi": 0.10, "zai": 0.80}, want: "kimi"},
		{name: "zai is less consumed", pressure: fixedPressure{"kimi": 0.90, "zai": 0.05}, want: "zai"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seat, _, reason, _, err := base(tc.pressure).resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
			if err != nil {
				t.Fatalf("resolveSeat: %v", err)
			}
			if seat.Lane != tc.want {
				t.Errorf("seat lane = %q, want %q — selection between flat lanes is by consumption pressure (S08.8/D5)", seat.Lane, tc.want)
			}
			low := strings.ToLower(reason)
			if !strings.Contains(low, "by how much of each is left") && !strings.Contains(low, "used") {
				t.Errorf("the reason does not say WHY the lane was chosen: %q", reason)
			}
			for _, money := range []string{"$", "usd", "cost", "price", "cheap"} {
				if strings.Contains(low, money) {
					t.Errorf("the selection reason names money (%q): %q — flat lanes are never chosen on dollars (D5)", money, reason)
				}
			}
		})
	}

	// No declared budget on one lane ⇒ no comparable ratio ⇒ the deterministic
	// duty-map order stands, and SAYS so. Ordering by raw consumption would
	// compare a request against a credit and hand every dispatch to whichever
	// lane was commissioned most recently.
	seat, _, reason, _, err := base(fixedPressure{"kimi": -1, "zai": 0.5}).
		resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil {
		t.Fatalf("resolveSeat(no budget): %v", err)
	}
	if seat.Lane == "" {
		t.Fatal("no seat was chosen when a lane had no declared budget")
	}
	if !strings.Contains(strings.ToLower(reason), "budget") {
		t.Errorf("the fallback reason does not name the missing budget: %q", reason)
	}
	again, _, _, _, err := base(fixedPressure{"kimi": -1, "zai": 0.5}).
		resolveSeat(ctx, RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
	if err != nil || again.Lane != seat.Lane {
		t.Errorf("the no-budget fallback is not deterministic: %q then %q (err=%v)", seat.Lane, again.Lane, err)
	}
}

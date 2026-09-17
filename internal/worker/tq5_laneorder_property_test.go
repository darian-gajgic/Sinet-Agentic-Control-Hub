package worker

import (
	"math/rand"
	"strings"
	"testing"
)

// tq5_laneorder_property_test.go — P3-TQ-5 acceptance checklist 15, the
// spec-stated invariant as a PROPERTY rather than three examples (Spec S08.8
// step 3 "subscription coverage binds every choice" + the accountability
// clause; the §65 precedent for drawing over coverage subsets).
//
// The invariant: with no comparable gauge reading, the execution duty resolves
// to the FIRST COVERED lane of the configured order, whatever order the
// commissioning read handed the alternates in, and every listed lane ahead of
// the winner that the household holds no plan for is NAMED on the requester's
// surface. Never a gap, never an error — a preference that cannot be honored
// is a sentence, not a park.
//
// The draw is over coverage subsets containing the always-configured lane,
// with every optional lane seated (which is the production coupling: coverage
// and seats both grow at commissioning, §65) and the seats shuffled, so the
// only thing the outcome can depend on is the configured order.
//
// $0: pure selection over in-memory data. No store, no engine, no network.
func TestTQ5LaneOrderProperty(t *testing.T) {
	order := DefaultLaneOrder()[DutyExecution]
	if len(order) == 0 {
		t.Fatal("the execution duty carries no lane order — the property has nothing to draw over")
	}
	base := DefaultDutyMap()[DutyExecution].Lane

	// The lanes a household may hold beyond the always-configured one. `zai`
	// is deliberately in the draw and deliberately NOT in the order: an
	// unlisted covered lane must keep its place after the listed ones rather
	// than jumping the preference.
	optional := []string{"kimi-cli", "kimi", "zai"}

	rng := rand.New(rand.NewSource(20260917))
	for draw := 0; draw < 200; draw++ {
		lanes := []string{base}
		covered := map[string]bool{base: true}
		for _, l := range optional {
			if rng.Intn(2) == 1 {
				lanes = append(lanes, l)
				covered[l] = true
			}
		}
		seats := make([]LaneSeat, 0, len(optional))
		for _, l := range optional {
			seats = append(seats, LaneSeat{Lane: l, Model: "seat-" + l})
		}
		rng.Shuffle(len(seats), func(i, j int) { seats[i], seats[j] = seats[j], seats[i] })

		r := &Router{
			DutyMap:    DefaultDutyMap(),
			Alternates: AlternateSeatsFor(seats...),
			LaneOrder:  DefaultLaneOrder(),
			Coverage:   Coverage{FlatRateLanes: lanes},
			// Pressure nil: the gauge cannot separate the lanes, so the
			// configured order is the whole answer.
		}
		seat, _, reason, gap, err := r.resolveSeat(t.Context(), RouteQuery{}, ExecutionProfile{Duty: DutyExecution})
		if err != nil {
			t.Fatalf("draw %d (covered %v): resolveSeat errored: %v", draw, lanes, err)
		}
		if gap != "" {
			t.Fatalf("draw %d (covered %v): a covered lane produced subscription-gap advice: %q", draw, lanes, gap)
		}

		want := ""
		for _, l := range order {
			if covered[l] {
				want = l
				break
			}
		}
		if want == "" {
			t.Fatalf("draw %d: no listed lane is covered although %s always is", draw, base)
		}
		if seat.Lane != want {
			t.Fatalf("draw %d (covered %v, seats handed in %v): chose %q, want the first covered lane of %v (%q)",
				draw, lanes, seatLanes(seats), seat.Lane, order, want)
		}
		if seat.Model == "" || seat.WindowTokens <= 0 {
			t.Fatalf("draw %d: chosen seat is incomplete: %+v", draw, seat)
		}

		for _, l := range order {
			if l == want {
				break
			}
			if covered[l] {
				t.Fatalf("draw %d: lane %q is covered and ahead of %q, yet lost", draw, l, want)
			}
			// "the <lane> lane" rather than the bare name: `kimi` is a
			// substring of `kimi-cli`, and a bare-name assertion would pass on
			// a sentence that named only the other one.
			if !strings.Contains(reason, "the "+l+" lane") {
				t.Fatalf("draw %d (covered %v): the reason does not name the skipped lane %q: %q", draw, lanes, l, reason)
			}
		}

		// Requester copy: plain words, no spec citations (CONVENTIONS §38).
		for _, cite := range []string{"S08", "S07", "S06", "A13", "D5"} {
			if strings.Contains(reason, cite) {
				t.Fatalf("draw %d: the reason cites %q on a requester surface: %q", draw, cite, reason)
			}
		}
		if strings.Contains(reason, "Not covered by a subscription") {
			t.Fatalf("draw %d: a resolved seat carries the subscription-gap sentence: %q", draw, reason)
		}
	}
}

func seatLanes(seats []LaneSeat) []string {
	out := make([]string, len(seats))
	for i, s := range seats {
		out[i] = s.Lane
	}
	return out
}

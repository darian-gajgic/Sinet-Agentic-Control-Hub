package benchmark_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/benchmark"
)

// feed_test.go pins BENCH-REG §13's stage 2 — the measured done-directly
// figure the S10 dashboards consume. Stage 1 (the per-run heuristic line) is
// internal/metering's and is NOT touched by this packet.

// TestMeasuredMedianActivatesAtTheRegisteredMinimum is acceptance item 19: the
// aggregate switches to the measured median at the registered ≥ 10, with the
// §13.2 label verbatim, and reports honestly before then.
func TestMeasuredMedianActivatesAtTheRegisteredMinimum(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	p := h.practice(t)

	// The priced fixture makes one "unit" of consumption cost exactly $2.
	const usdPerUnit = 2.0

	// Nine measured pairs: one short of the registered minimum.
	for i := 1; i <= 9; i++ {
		h.recordPair(t, pairSpec{
			task: fmt.Sprintf("t%d", i), choice: benchmark.ChoiceA, guess: benchmark.SideA,
			directModel: "frontier-a", units: int64(i),
		})
	}
	agg, err := p.DoneDirectly(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if agg.Measured {
		t.Errorf("the aggregate switched at n=%d; the registered minimum is %d (BENCH-REG §13.2)",
			agg.N, benchmark.DoneDirectlyMinPairs)
	}
	if agg.N != 9 {
		t.Errorf("n = %d, want 9 measured pairs", agg.N)
	}
	if agg.Label != "" {
		t.Errorf("the measured label must not appear before the threshold: %q", agg.Label)
	}
	// Even short of the threshold the heuristic label is present: both labels
	// remain available side by side, and per-run receipts keep the heuristic
	// line forever.
	if agg.HeuristicLabel != benchmark.LabelHeuristic {
		t.Errorf("heuristic label = %q, want the §13.1 label verbatim", agg.HeuristicLabel)
	}
	if !strings.Contains(agg.Summary, fmt.Sprint(benchmark.DoneDirectlyMinPairs)) {
		t.Errorf("the pre-threshold summary must say what it is waiting for: %q", agg.Summary)
	}

	// The tenth measured pair activates it.
	h.recordPair(t, pairSpec{task: "t10", choice: benchmark.ChoiceA, guess: benchmark.SideA,
		directModel: "frontier-a", units: 10})
	agg, err = p.DoneDirectly(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if !agg.Measured || agg.N != 10 {
		t.Fatalf("the aggregate did not activate at the registered minimum: %+v", agg)
	}
	if agg.Label != benchmark.MeasuredLabel(10) {
		t.Errorf("label = %q, want %q verbatim", agg.Label, benchmark.MeasuredLabel(10))
	}
	// Units 1..10 cost $2..$20; the median of ten values is the mean of the
	// fifth and sixth, $11.
	if want := 5.5 * usdPerUnit; agg.MedianUSD != want {
		t.Errorf("measured median = %v, want %v", agg.MedianUSD, want)
	}
	if agg.MinimumPairs != benchmark.DoneDirectlyMinPairs || agg.Marker != benchmark.RegisteredMarker {
		t.Errorf("the aggregate must carry the registered minimum and its read-only marker: %+v", agg)
	}
	// Both labels ride together, per §13.2.
	if agg.HeuristicLabel != benchmark.LabelHeuristic || !strings.Contains(agg.Summary, benchmark.LabelHeuristic) {
		t.Errorf("both labels must remain available side by side: %+v", agg)
	}
}

// TestDeclinedAndUnmeasuredPairsNeverEnterTheMedian: only MEASURED pairs feed
// the figure — §13.2 says "measured benchmark pairs", and a pair with no
// consumption reading would drag the median toward a number nobody observed.
func TestDeclinedAndUnmeasuredPairsNeverEnterTheMedian(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	p := h.practice(t)

	h.recordPair(t, pairSpec{task: "m1", choice: benchmark.ChoiceA, guess: benchmark.SideA,
		directModel: "frontier-a", units: 3})
	// A pair with no metered consumption at all.
	h.recordPair(t, pairSpec{task: "u1", choice: benchmark.ChoiceA, guess: benchmark.SideA,
		directModel: "frontier-a"})
	// And a decline.
	h.recordPair(t, pairSpec{task: "d1", decline: true})

	agg, err := p.DoneDirectly(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if agg.N != 1 {
		t.Errorf("n = %d measured pairs, want 1 — unmeasured and declined pairs never enter the median", agg.N)
	}
}

// TestMedianPoolsAllEpochsAndAnnotatesThem is the OQ6 disposition: §13.2 names
// no epoch qualifier, and §9's no-pooling rule binds DECISION statistics, which
// the done-directly figure is not. The epochs pooled are annotated so the scope
// is visible rather than inferred.
func TestMedianPoolsAllEpochsAndAnnotatesThem(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.user(t, "alice", true)
	p := h.practice(t)

	for i := 1; i <= 5; i++ {
		h.recordPair(t, pairSpec{task: fmt.Sprintf("a%d", i), choice: benchmark.ChoiceA,
			guess: benchmark.SideA, directModel: "frontier-a", units: int64(i)})
	}
	for i := 6; i <= 10; i++ {
		h.recordPair(t, pairSpec{task: fmt.Sprintf("b%d", i), choice: benchmark.ChoiceA,
			guess: benchmark.SideA, directModel: "frontier-b", units: int64(i)})
	}
	agg, err := p.DoneDirectly(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if agg.N != 10 || !agg.Measured {
		t.Fatalf("the median must pool ALL measured pairs of the domain: %+v", agg)
	}
	if len(agg.Epochs) != 2 {
		t.Errorf("%d epochs annotated, want the 2 the pool spans", len(agg.Epochs))
	}

	// Meanwhile the DECISION statistic stays inside the current epoch — the two
	// scopes are deliberately different and both are correct.
	rate, err := p.PublishedRate(ctx, benchmark.DomainSoftwareDevelopment, "")
	if err != nil {
		t.Fatal(err)
	}
	if rate.N != 10 {
		t.Fatalf("an all-epoch readout should see all 10; got %d", rate.N)
	}
	ev, err := p.Gate(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Rate.N != 5 {
		t.Errorf("the gate pooled %d pairs; it must see the CURRENT epoch's 5 only (BENCH-REG §9)", ev.Rate.N)
	}
}

// TestTheFeedIsAReadApiNotAReceiptEdit: the §13.1 per-run line is
// internal/metering's forever, and this package never writes a receipt.
func TestTheFeedIsAReadApiNotAReceiptEdit(t *testing.T) {
	code := readPackageCode(t)
	for _, forbidden := range []string{"Receipts", "Materialize", "DirectUseEstimate", "receipts"} {
		if strings.Contains(code, forbidden) {
			t.Errorf("the package's code names %q — per-run receipts keep the heuristic line and this packet does not touch receipt.go (BENCH-REG §13.2)", forbidden)
		}
	}
	if !strings.Contains(readPackageSource(t), "receipt.go is NOT edited") &&
		!strings.Contains(readPackageSource(t), "receipt.go") {
		t.Error("the receipt seam must be named in a doc comment so the two halves stay legible")
	}
}

package benchmark_test

import (
	"math"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/benchmark"
)

// stats_test.go pins BENCH-REG §7 against its own registered reference points.
//
// The registration publishes verified reference values for G. If this code's G
// ever disagreed with them, the platform would be running a different statistic
// from the one it registered — the exact "silent drift between this text and the
// running platform" §17 calls a platform defect. So every published reference
// value is asserted here, exactly, and the whole table is re-derived from the
// registration file in registered_test.go so a row cannot be quietly edited out.

// round3 rounds to the three decimals the registration publishes.
func round3(f float64) float64 { return math.Round(f*1000) / 1000 }

// TestGReferenceTable asserts every §7/§15 reference record, row for row. The
// two load-bearing rows are the gate boundary: 13/20 clears at G = 0.905, and
// 12/20 does NOT at 0.808 — one win apart, and the registration says which side
// of the line each falls on.
func TestGReferenceTable(t *testing.T) {
	for _, c := range []struct {
		w, m  int
		wantG float64
		note  string
	}{
		{13, 20, 0.905, "minimal gate-clearing record at m=20 (BENCH-REG §7)"},
		{12, 20, 0.808, "does NOT clear the gate (BENCH-REG §7)"},
		{7, 10, 0.887, "report-12 §2.6 cross-check (registration rounds to 0.89)"},
		{14, 20, 0.961, "report-12 §2.6 cross-check (registration rounds to 0.96)"},
		{20, 30, 0.965, "§15 honest-claims row (registration rounds to 0.97)"},
		{35, 50, 0.998, "report-12 §2.6 cross-check"},
	} {
		p, err := benchmark.ComputeG(c.w, c.m)
		if err != nil {
			t.Fatalf("ComputeG(%d,%d): %v", c.w, c.m, err)
		}
		if got := round3(p.G); got != c.wantG {
			t.Errorf("G(%d/%d) = %.4f (rounded %.3f), want %.3f — %s", c.w, c.m, p.G, got, c.wantG, c.note)
		}
	}
}

// TestGateBoundaryIsTheRegisteredOne is the same two rows read as the gate
// reads them: the registered floor is G ≥ 0.90 at m ≥ 20, so 13/20 satisfies
// the statistical limbs and 12/20 does not.
func TestGateBoundaryIsTheRegisteredOne(t *testing.T) {
	clears, err := benchmark.ComputeG(13, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !clears.ClearsGateStatistic() {
		t.Errorf("13/20 (G=%.4f) must satisfy the registered gate statistic m≥%d ∧ G≥%.2f",
			clears.G, benchmark.GateMinNonTiedPairs, benchmark.GateGFloor)
	}
	misses, err := benchmark.ComputeG(12, 20)
	if err != nil {
		t.Fatal(err)
	}
	if misses.ClearsGateStatistic() {
		t.Errorf("12/20 (G=%.4f) must NOT satisfy the registered gate statistic — the registration says so explicitly", misses.G)
	}
	// And m alone is never enough: a perfect but short record fails limb (a).
	short, err := benchmark.ComputeG(19, 19)
	if err != nil {
		t.Fatal(err)
	}
	if short.ClearsGateStatistic() {
		t.Errorf("19/19 at m=19 must fail limb (a): the registered minimum is m ≥ %d", benchmark.GateMinNonTiedPairs)
	}
}

// TestAlarmZoneReferenceRecords asserts §7's "earliest alarm-zone records" row:
// 1−G at 0/4, 2/10, 4/15 and 6/20, each above the registered 0.95 threshold.
func TestAlarmZoneReferenceRecords(t *testing.T) {
	for _, c := range []struct {
		w, m      int
		wantLossG float64
	}{
		{0, 4, 0.969},
		{2, 10, 0.967},
		{4, 15, 0.962},
		{6, 20, 0.961},
	} {
		p, err := benchmark.ComputeG(c.w, c.m)
		if err != nil {
			t.Fatalf("ComputeG(%d,%d): %v", c.w, c.m, err)
		}
		if got := round3(p.LossG); got != c.wantLossG {
			t.Errorf("1−G(%d/%d) = %.4f (rounded %.3f), want %.3f (BENCH-REG §7 alarm-zone table)",
				c.w, c.m, p.LossG, got, c.wantLossG)
		}
		if !p.AlarmCondition() {
			t.Errorf("%d/%d has 1−G = %.4f and must trip the registered alarm (1−G > %.2f)",
				c.w, c.m, p.LossG, benchmark.AlarmThreshold)
		}
	}
	// The boundary itself, asserted against EXACT fixtures rather than against
	// the comparison's own definition (which could not fail):
	//
	//   3/12 → G = 378/8192, so 1−G = 7814/8192 ≈ 0.95386 — just OVER 0.95, trips.
	//   2/9  → G = 56/1024,  so 1−G = 968/1024  ≈ 0.94531 — just UNDER, does not.
	//
	// One pair either side of the registered line, so a threshold edited from
	// 0.95 in either direction breaks exactly one of them.
	for _, c := range []struct {
		w, m      int
		exactG    string
		wantLossG float64
		wantTrip  bool
	}{
		{3, 12, "189/4096", 0.954, true},
		{2, 9, "7/128", 0.945, false},
	} {
		p, err := benchmark.ComputeG(c.w, c.m)
		if err != nil {
			t.Fatal(err)
		}
		r, err := benchmark.GExact(c.w, c.m)
		if err != nil {
			t.Fatal(err)
		}
		if got := r.String(); got != c.exactG {
			t.Errorf("GExact(%d,%d) = %s, want %s", c.w, c.m, got, c.exactG)
		}
		if got := round3(p.LossG); got != c.wantLossG {
			t.Errorf("1−G(%d/%d) = %.5f (rounded %.3f), want %.3f", c.w, c.m, p.LossG, got, c.wantLossG)
		}
		if p.AlarmCondition() != c.wantTrip {
			t.Errorf("1−G(%d/%d) = %.5f: alarm trips = %v, want %v against the registered threshold %.2f (STRICTLY greater)",
				c.w, c.m, p.LossG, p.AlarmCondition(), c.wantTrip, benchmark.AlarmThreshold)
		}
	}
}

// TestFixedTestRejectionAtN20 asserts the §15 honest-claims row "n=20:
// fixed-test rejection needs ≥15/20" from first principles: P(X≥15) < 0.05 and
// P(X≥14) > 0.05 at one-sided α=0.05 against 50/50. The table ships verbatim as
// data; this proves the row it ships is arithmetically what it says.
func TestFixedTestRejectionAtN20(t *testing.T) {
	at15, err := benchmark.FixedTestTailAtLeast(20, 15)
	if err != nil {
		t.Fatal(err)
	}
	at14, err := benchmark.FixedTestTailAtLeast(20, 14)
	if err != nil {
		t.Fatal(err)
	}
	if !(at15 < 0.05) {
		t.Errorf("P(X≥15 | n=20) = %.4f, want < 0.05 (BENCH-REG §15)", at15)
	}
	if !(at14 > 0.05) {
		t.Errorf("P(X≥14 | n=20) = %.4f, want > 0.05 — 15/20 is the registered rejection point, not 14/20", at14)
	}
	if got := math.Round(at15*10000) / 10000; got != 0.0207 {
		t.Errorf("P(X≥15 | n=20) = %.4f, want 0.0207", got)
	}
	if got := math.Round(at14*10000) / 10000; got != 0.0577 {
		t.Errorf("P(X≥14 | n=20) = %.4f, want 0.0577", got)
	}
}

// TestGIsExactRational proves "computed exactly" is literal: 13/20 has an exact
// rational value with denominator 2^21, and the float64 G is that rational.
func TestGIsExactRational(t *testing.T) {
	r, err := benchmark.GExact(13, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.String(); got != "237339/262144" {
		// 1898712/2097152 in lowest terms.
		t.Errorf("GExact(13,20) = %s, want 237339/262144 (= 1898712/2^21)", got)
	}
	p, err := benchmark.ComputeG(13, 20)
	if err != nil {
		t.Fatal(err)
	}
	exact, _ := r.Float64()
	if p.G != exact {
		t.Errorf("ComputeG's G (%v) is not the exact rational (%v)", p.G, exact)
	}
}

// TestEmptyAndDegenerateRecords: an empty record is the uniform prior, not an
// accidental zero, and the extremes behave.
func TestEmptyAndDegenerateRecords(t *testing.T) {
	empty, err := benchmark.ComputeG(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if empty.G != 0.5 || empty.LossG != 0.5 {
		t.Errorf("an empty record must read as the uniform prior (G=0.5), got G=%v", empty.G)
	}
	if empty.AlarmCondition() {
		t.Error("an empty record must never trip the alarm")
	}
	if _, err := benchmark.ComputeG(5, 4); err == nil {
		t.Error("w > m is not a record and must be refused")
	}
	if _, err := benchmark.ComputeG(-1, 4); err == nil {
		t.Error("a negative w is not a record and must be refused")
	}
}

// TestGIsMonotoneInWins: one more win never lowers G. A statistic that could
// would make the gate incoherent, and the property holds for the exact
// binomial-CDF identity by construction — asserted because it is cheap and it
// would catch an off-by-one in the coefficient recurrence.
func TestGIsMonotoneInWins(t *testing.T) {
	const m = 30
	prev := -1.0
	for w := 0; w <= m; w++ {
		p, err := benchmark.ComputeG(w, m)
		if err != nil {
			t.Fatal(err)
		}
		if p.G < prev {
			t.Fatalf("G fell from %.6f to %.6f between %d and %d wins at m=%d", prev, p.G, w-1, w, m)
		}
		prev = p.G
	}
	if prev >= 1.0 {
		t.Errorf("G at %d/%d = %.6f; a posterior probability is never 1", m, m, prev)
	}
}

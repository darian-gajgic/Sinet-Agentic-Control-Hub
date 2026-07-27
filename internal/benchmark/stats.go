package benchmark

import (
	"fmt"
	"math/big"
)

// stats.go is BENCH-REG §7, implemented exactly and in stdlib integer
// arithmetic — no dependency, no floating-point special function, no
// approximation.
//
// §7 verbatim, per (domain, epoch) over non-tied pairs only:
//
//	Posterior: p ~ Beta(1 + w, 1 + (m − w)) — Beta-Binomial with uniform prior,
//	updated per pair.
//	Define G := P(p > 0.5 | w, m) — computed exactly (regularized incomplete
//	beta; for integer parameters G = P(Binomial(m+1, ½) ≤ w)).
//
// The integer identity is what makes "computed exactly" reachable without a
// numerics library: with a uniform prior the posterior parameters are integers,
// so G is a finite sum of binomial coefficients over a power of two — an exact
// rational, computed here with math/big and converted to float64 only at the
// display boundary.
//
// Monitoring is continuous: the posterior may be inspected at any time, updated
// per pair, with optional stopping. G is the ONLY decision input; the fixed-n
// views below are readouts (§7, §16) and are never consulted by the gate or the
// alarm.

// Posterior is the §7 reading for one (domain, epoch) accrual record.
type Posterior struct {
	// W is platform wins, M is non-tied pairs. tie and both-bad enter neither
	// (§8, frozen).
	W int `json:"w"`
	M int `json:"m"`
	// G is P(platform better) under the uniform-prior Beta-Binomial posterior.
	G float64 `json:"g"`
	// LossG is 1 − G, the quantity the §12 alarm tests. It is carried rather
	// than left to the caller so no consumer re-derives the alarm's input.
	LossG float64 `json:"loss_g"`
	// WinRate is w/m. It NEVER travels alone: every published surface carries
	// it inside PublishedRate with the full epistemics block (§15, R19).
	WinRate float64 `json:"win_rate"`
}

// ComputeG returns the §7 posterior for w platform wins out of m non-tied
// pairs. It is a pure function: the same record always yields the same G, and
// nothing about the phase, the domain or the operator can move it.
func ComputeG(w, m int) (Posterior, error) {
	if m < 0 || w < 0 || w > m {
		return Posterior{}, fmt.Errorf("%w: w=%d m=%d is not a record (need 0 ≤ w ≤ m)", ErrBadInput, w, m)
	}
	p := Posterior{W: w, M: m}
	if m == 0 {
		// No non-tied pairs: the posterior is the uniform prior, so
		// P(p > 0.5) = 0.5 exactly. The identity gives the same answer
		// (P(Binomial(1,½) ≤ 0) = ½); it is stated here so the empty record
		// reads as "no evidence", never as an accidental zero.
		p.G, p.LossG = 0.5, 0.5
		return p, nil
	}
	num, den := binomialCDF(m+1, w)
	g := new(big.Rat).SetFrac(num, den)
	p.G, _ = g.Float64()
	p.LossG = 1 - p.G
	p.WinRate = float64(w) / float64(m)
	return p, nil
}

// GExact returns G as an exact rational, for a caller that must compare or
// display without float rounding. It is the same computation ComputeG performs;
// ComputeG's float64 is that rational at the display boundary.
func GExact(w, m int) (*big.Rat, error) {
	if m < 0 || w < 0 || w > m {
		return nil, fmt.Errorf("%w: w=%d m=%d is not a record (need 0 ≤ w ≤ m)", ErrBadInput, w, m)
	}
	if m == 0 {
		return big.NewRat(1, 2), nil
	}
	num, den := binomialCDF(m+1, w)
	return new(big.Rat).SetFrac(num, den), nil
}

// binomialCDF returns the exact numerator and denominator of
// P(Binomial(n, ½) ≤ k) = (Σ_{i=0..k} C(n,i)) / 2^n, in integer arithmetic.
// k is clamped into [0, n] by the callers' own range checks.
func binomialCDF(n, k int) (num, den *big.Int) {
	num = new(big.Int)
	c := big.NewInt(1) // C(n,0)
	for i := 0; i <= k; i++ {
		if i > 0 {
			// C(n,i) = C(n,i-1) * (n-i+1) / i — exact at every step, because
			// the running value is always an integer binomial coefficient.
			c.Mul(c, big.NewInt(int64(n-i+1)))
			c.Quo(c, big.NewInt(int64(i)))
		}
		num.Add(num, c)
	}
	den = new(big.Int).Lsh(big.NewInt(1), uint(n))
	return num, den
}

// AlarmCondition reports whether a posterior trips the §12 alarm: 1 − G > 0.95,
// strictly greater, against the registered threshold. It exists so no consumer
// spells the comparison itself and drifts from the registration.
func (p Posterior) AlarmCondition() bool { return p.LossG > AlarmThreshold }

// ClearsGateStatistic reports whether a posterior satisfies the two STATISTICAL
// gate limbs (§11(a) m ≥ 20 and §11(b) G ≥ 0.90). Limbs (c) and (d) are not
// statistics and are evaluated in gate.go.
func (p Posterior) ClearsGateStatistic() bool {
	return p.M >= GateMinNonTiedPairs && p.G >= GateGFloor
}

// ── Fixed-n readouts (§7: "Fixed-n views are readouts, never decision rules") ─

// FixedTestTailAtLeast returns the exact one-sided binomial tail P(X ≥ x) for n
// trials at p = ½ — the classical fixed-n test the §15 honest-claims table
// reports ("n=20: fixed-test rejection needs ≥15/20").
//
// It exists to PROVE that registered claim rather than take it on trust: the
// §15 table is reproduced verbatim as data, and this function is what lets the
// test assert the reproduced row is arithmetically what it says. It is a
// READOUT and is never consulted by the gate or the alarm — §7 makes G the only
// decision input, and a fixed-n rule wired into a decision would contradict the
// registration.
func FixedTestTailAtLeast(n, x int) (float64, error) {
	if n < 0 || x < 0 || x > n {
		return 0, fmt.Errorf("%w: n=%d x=%d is not a fixed-n tail", ErrBadInput, n, x)
	}
	if x == 0 {
		return 1, nil
	}
	// P(X ≥ x) = 1 − P(X ≤ x−1).
	num, den := binomialCDF(n, x-1)
	lower, _ := new(big.Rat).SetFrac(num, den).Float64()
	return 1 - lower, nil
}

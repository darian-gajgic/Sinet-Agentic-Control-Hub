package local

import (
	"fmt"
	"math"
	"sort"
)

// calibrate.go — the Spec S12.5 four-step calibration recipe (brief R2/R3),
// verbatim in shape, every number re-derived locally (no imported constants),
// stdlib-only (isotonic regression = pool-adjacent-violators, no Go module):
//
//  1. a labeled set (~30–50 items per duty, bootstrap; grown from production
//     under golden-set governance — the S14/B5 seam);
//  2. margins via the /v1 logprobs (margin.go, LabelMargin);
//  3. an isotonic map margin→P(error) fit on a calibration split (IsotonicFit,
//     pool-adjacent-violators, monotone non-increasing: a higher margin means a
//     lower error);
//  4. the escalation threshold picked on a validation split — the LOWEST margin
//     threshold whose accepted-set error stays within the duty's acceptance bar
//     (accept the most locally ⇒ escalate the least; escalation costs more).
//
// Calibrated values are platform DATA keyed (duty, model hash, engine build)
// (S12.5; R3) — model hash = SeatRecord.ModelHash(), engine build =
// LlamaCppPin. This is EXACTLY the key the S12.10 swap gate keeps valid
// (P-T15-1): any swap invalidates it, and a retarget is refused until a fresh
// fit for the NEW key exists (swap.go). The machinery ships honest-uncalibrated:
// with no calibration for a key the duty's ungated v0 behaviour continues (the
// calibrated=false VRAM-ledger precedent) — no threshold belief, no re-check.

// LabeledItem is one calibration observation: the item's margin (from
// LabelMargin over its logprobs) and whether the local answer was WRONG against
// the human/gold label.
type LabeledItem struct {
	Margin float64 `json:"margin"`
	Wrong  bool    `json:"wrong"`
}

// CalibrationKey is the S12.5 platform-data key: the duty alias, the seat's
// model hash (manifest sha256), and the engine build (LlamaCppPin). The swap
// gate keeps exactly this key valid (R3/R11).
type CalibrationKey struct {
	Duty        string `json:"duty"`
	ModelHash   string `json:"model_hash"`
	EngineBuild string `json:"engine_build"`
}

// String renders the key for logs/reasons.
func (k CalibrationKey) String() string {
	return fmt.Sprintf("%s@%s/%s", k.Duty, short(k.ModelHash), k.EngineBuild)
}

func short(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// IsotonicMap is a monotone-non-increasing step function margin→P(error) fit by
// pool-adjacent-violators (S12.5 step 3). X is the sorted margin breakpoints, Y
// the fitted P(error) at each (Y non-increasing). PError reads the fitted value
// at the largest breakpoint ≤ the query margin (conservative below the range).
type IsotonicMap struct {
	X []float64 `json:"x"`
	Y []float64 `json:"y"`
}

// IsotonicFit fits a monotone-non-increasing map margin→P(error) by
// pool-adjacent-violators (S12.5 step 3). Items may be unsorted; equal margins
// pool. An empty set yields an empty map (PError then returns 0.5 — maximal
// uncertainty, honest).
func IsotonicFit(items []LabeledItem) IsotonicMap {
	if len(items) == 0 {
		return IsotonicMap{}
	}
	pts := append([]LabeledItem(nil), items...)
	sort.SliceStable(pts, func(i, j int) bool { return pts[i].Margin < pts[j].Margin })

	// PAV blocks, enforcing non-increasing means (a higher margin ⇒ lower
	// P(error)): pool the last two blocks while the earlier mean is BELOW the
	// later mean (a non-increasing violation).
	type block struct {
		sum float64 // sum of error indicators
		n   float64 // weight
		xhi float64 // largest margin in the block
	}
	var blocks []block
	for _, p := range pts {
		e := 0.0
		if p.Wrong {
			e = 1.0
		}
		blocks = append(blocks, block{sum: e, n: 1, xhi: p.Margin})
		for len(blocks) >= 2 {
			a, b := blocks[len(blocks)-2], blocks[len(blocks)-1]
			if a.sum/a.n >= b.sum/b.n {
				break
			}
			blocks = blocks[:len(blocks)-2]
			blocks = append(blocks, block{sum: a.sum + b.sum, n: a.n + b.n, xhi: b.xhi})
		}
	}
	m := IsotonicMap{}
	for _, bl := range blocks {
		m.X = append(m.X, bl.xhi)
		m.Y = append(m.Y, bl.sum/bl.n)
	}
	return m
}

// PError returns the fitted P(error) at a margin. Below the fitted range it
// returns the first (highest) fitted value — the conservative reading; above,
// the last (lowest). An empty map returns 0.5 (maximal uncertainty).
func (m IsotonicMap) PError(margin float64) float64 {
	if len(m.X) == 0 {
		return 0.5
	}
	if margin < m.X[0] {
		return m.Y[0]
	}
	// largest breakpoint ≤ margin
	i := sort.Search(len(m.X), func(i int) bool { return m.X[i] > margin }) - 1
	if i < 0 {
		i = 0
	}
	return m.Y[i]
}

// MarginSummary is the recorded distribution of a duty's calibration margins
// (S12.9 #6 record: "margin distribution"). Min/Max/Mean over the labeled set.
type MarginSummary struct {
	N    int     `json:"n"`
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Mean float64 `json:"mean"`
}

// Calibration is the fitted per-duty confidence calibration (S12.5), platform
// data keyed by CalibrationKey (R3). AcceptLocal(margin) is the live re-check
// gate: margin ≥ Threshold ⇒ accept the fast-tier answer; below ⇒ re-check on
// the workhorse (R4).
type Calibration struct {
	Key           CalibrationKey `json:"key"`
	Threshold     float64        `json:"threshold"`
	AcceptanceBar float64        `json:"acceptance_bar"`
	AchievedError float64        `json:"achieved_error"` // P(error) among accepted, on the validation split
	AcceptRate    float64        `json:"accept_rate"`    // fraction accepted locally, on the validation split
	LabeledN      int            `json:"labeled_n"`
	MeetsBar      bool           `json:"meets_bar"` // false ⇒ no threshold met the bar; the duty escalates ~everything
	Isotonic      IsotonicMap    `json:"isotonic"`
	Margins       MarginSummary  `json:"margins"`
}

// AcceptLocal reports whether a fast-tier answer with this margin is accepted
// without a workhorse re-check (R4): margin ≥ Threshold.
func (c *Calibration) AcceptLocal(margin float64) bool {
	if c == nil {
		return true // uncalibrated ⇒ ungated v0 behaviour continues (R3)
	}
	return margin >= c.Threshold
}

// Fit runs the S12.5 recipe over a labeled set for a key, at an acceptance bar
// (max tolerable P(error) among accepted). The split is DETERMINISTIC (index
// parity — no RNG in a fit, so a re-fit of the same set is byte-identical):
// even indices calibrate (the isotonic map), odd indices validate (the
// threshold + achieved error). Requires ≥4 items so both splits are non-empty.
func Fit(key CalibrationKey, items []LabeledItem, bar float64) (Calibration, error) {
	if len(items) < 4 {
		return Calibration{}, fmt.Errorf("local: calibration needs ≥4 labeled items, got %d (S12.5 ~30–50 bootstrap)", len(items))
	}
	if bar <= 0 || bar >= 1 {
		return Calibration{}, fmt.Errorf("local: acceptance bar %v out of (0,1)", bar)
	}
	var cal, val []LabeledItem
	for i, it := range items {
		if i%2 == 0 {
			cal = append(cal, it)
		} else {
			val = append(val, it)
		}
	}
	iso := IsotonicFit(cal)
	thr, achieved, acceptRate, meets := pickThreshold(val, bar)

	return Calibration{
		Key:           key,
		Threshold:     thr,
		AcceptanceBar: bar,
		AchievedError: achieved,
		AcceptRate:    acceptRate,
		LabeledN:      len(items),
		MeetsBar:      meets,
		Isotonic:      iso,
		Margins:       summarize(items),
	}, nil
}

// pickThreshold picks the LOWEST margin threshold whose accepted-set error
// (items with margin ≥ threshold) stays ≤ bar on the validation split (S12.5
// step 4 — accept the most locally subject to the bar). Returns the threshold,
// the achieved error among accepted, the accept rate, and whether any threshold
// met the bar. When none does, the threshold is above every margin (escalate
// ~everything) and meets=false.
func pickThreshold(val []LabeledItem, bar float64) (threshold, achievedErr, acceptRate float64, meets bool) {
	if len(val) == 0 {
		return math.Inf(1), 0, 0, false
	}
	// candidate thresholds = the observed margins, ascending (a lower threshold
	// accepts more). Try each; keep the lowest that satisfies the bar.
	cands := make([]float64, len(val))
	for i, it := range val {
		cands[i] = it.Margin
	}
	sort.Float64s(cands)

	best := math.Inf(1)
	var bestErr, bestAccept float64
	found := false
	for _, tau := range cands {
		accepted, wrong := 0, 0
		for _, it := range val {
			if it.Margin >= tau {
				accepted++
				if it.Wrong {
					wrong++
				}
			}
		}
		if accepted == 0 {
			continue
		}
		e := float64(wrong) / float64(accepted)
		if e <= bar && tau < best {
			best = tau
			bestErr = e
			bestAccept = float64(accepted) / float64(len(val))
			found = true
		}
	}
	if !found {
		// No threshold meets the bar; escalate everything (accept nothing
		// locally). Record the best achievable error at the strictest threshold.
		strict := cands[len(cands)-1]
		accepted, wrong := 0, 0
		for _, it := range val {
			if it.Margin >= strict {
				accepted++
				if it.Wrong {
					wrong++
				}
			}
		}
		e := 0.0
		if accepted > 0 {
			e = float64(wrong) / float64(accepted)
		}
		return math.Nextafter(strict, math.Inf(1)), e, 0, false
	}
	return best, bestErr, bestAccept, true
}

func summarize(items []LabeledItem) MarginSummary {
	if len(items) == 0 {
		return MarginSummary{}
	}
	s := MarginSummary{N: len(items), Min: items[0].Margin, Max: items[0].Margin}
	var sum float64
	for _, it := range items {
		if it.Margin < s.Min {
			s.Min = it.Margin
		}
		if it.Margin > s.Max {
			s.Max = it.Margin
		}
		sum += it.Margin
	}
	s.Mean = sum / float64(len(items))
	return s
}

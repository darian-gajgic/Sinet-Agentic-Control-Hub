package local

import (
	"testing"
)

func TestIsotonicFitMonotoneNonIncreasing(t *testing.T) {
	// Higher margin ⇒ lower error. Include a local violation (a wrong item at a
	// high margin) that PAV must pool away.
	items := []LabeledItem{
		{Margin: 0.1, Wrong: true}, {Margin: 0.2, Wrong: true}, {Margin: 0.3, Wrong: true},
		{Margin: 0.9, Wrong: true}, {Margin: 1.0, Wrong: false}, {Margin: 1.1, Wrong: false},
		{Margin: 2.0, Wrong: false}, {Margin: 2.5, Wrong: false}, {Margin: 3.0, Wrong: false},
	}
	m := IsotonicFit(items)
	for i := 1; i < len(m.Y); i++ {
		if m.Y[i] > m.Y[i-1]+1e-9 {
			t.Fatalf("isotonic map not non-increasing at %d: %v", i, m.Y)
		}
	}
	if m.PError(0.1) <= m.PError(3.0) {
		t.Errorf("PError(low)=%v should exceed PError(high)=%v", m.PError(0.1), m.PError(3.0))
	}
	// Below the fitted range returns the highest (conservative) error.
	if m.PError(-5) != m.Y[0] {
		t.Errorf("PError below range = %v, want the first fitted %v", m.PError(-5), m.Y[0])
	}
}

func TestIsotonicFitEmpty(t *testing.T) {
	m := IsotonicFit(nil)
	if m.PError(0.5) != 0.5 {
		t.Errorf("empty map PError = %v, want 0.5 (maximal uncertainty)", m.PError(0.5))
	}
}

func TestFitThresholdMeetsBar(t *testing.T) {
	// A clean separable set: low margins wrong, high margins right. The
	// threshold should accept the high-margin items within the bar.
	var items []LabeledItem
	for i := 0; i < 20; i++ {
		items = append(items, LabeledItem{Margin: 0.05 * float64(i), Wrong: true})      // 0.0..0.95, wrong
		items = append(items, LabeledItem{Margin: 1.0 + 0.05*float64(i), Wrong: false}) // 1.0..1.95, right
	}
	key := CalibrationKey{Duty: "intake-triage", ModelHash: "abc", EngineBuild: "b10085"}
	cal, err := Fit(key, items, 0.2)
	if err != nil {
		t.Fatalf("Fit: %v", err)
	}
	if !cal.MeetsBar {
		t.Fatalf("expected the bar to be met on a separable set; got %+v", cal)
	}
	if cal.AchievedError > 0.2+1e-9 {
		t.Errorf("achieved error %v exceeds the bar 0.2", cal.AchievedError)
	}
	if cal.Threshold <= 0.95 {
		t.Errorf("threshold %v should exclude the wrong low-margin region (≤0.95)", cal.Threshold)
	}
	if cal.LabeledN != 40 {
		t.Errorf("labeled N = %d, want 40", cal.LabeledN)
	}
	// AcceptLocal at/above the threshold, escalate below.
	if !cal.AcceptLocal(cal.Threshold) {
		t.Error("AcceptLocal at the threshold should be true")
	}
	if cal.AcceptLocal(cal.Threshold - 0.001) {
		t.Error("AcceptLocal below the threshold should be false (re-check)")
	}
}

func TestFitUnmetBarEscalatesEverything(t *testing.T) {
	// The margin carries NO usable signal: the validation split (odd indices)
	// alternates right/wrong across margins and ENDS in wrong at the top, so
	// every candidate threshold's accepted set has error ≥ 0.5 — no threshold
	// meets the bar, and the fit escalates ~everything (meets=false).
	var items []LabeledItem
	for k := 0; k < 10; k++ {
		// even index k*2 = calibration split (content irrelevant to meets)
		items = append(items, LabeledItem{Margin: 0.2*float64(k+1) - 0.05, Wrong: k%2 == 0})
		// odd index k*2+1 = validation split: wrong on odd k (top k=9 ⇒ wrong)
		items = append(items, LabeledItem{Margin: 0.2 * float64(k+1), Wrong: k%2 == 1})
	}
	cal, err := Fit(CalibrationKey{Duty: "d", ModelHash: "h", EngineBuild: "b"}, items, 0.2)
	if err != nil {
		t.Fatalf("Fit: %v", err)
	}
	if cal.MeetsBar {
		t.Errorf("expected meets=false on a no-signal set; got %+v", cal)
	}
	if cal.AcceptLocal(2.0) {
		t.Error("an unmet-bar calibration accepts nothing locally (threshold above every margin)")
	}
}

func TestFitNeedsMinimumItems(t *testing.T) {
	if _, err := Fit(CalibrationKey{Duty: "d", ModelHash: "h", EngineBuild: "b"}, []LabeledItem{{Margin: 1}}, 0.2); err == nil {
		t.Error("Fit should require ≥4 items")
	}
}

func TestFitDeterministic(t *testing.T) {
	items := []LabeledItem{
		{Margin: 0.1, Wrong: true}, {Margin: 0.5, Wrong: true}, {Margin: 1.5, Wrong: false},
		{Margin: 2.0, Wrong: false}, {Margin: 2.5, Wrong: false}, {Margin: 3.0, Wrong: false},
	}
	key := CalibrationKey{Duty: "d", ModelHash: "h", EngineBuild: "b"}
	a, _ := Fit(key, items, 0.2)
	b, _ := Fit(key, items, 0.2)
	if a.Threshold != b.Threshold || a.AchievedError != b.AchievedError {
		t.Error("Fit must be deterministic (no RNG in the split)")
	}
}

package evals

import (
	"fmt"
	"strings"
)

// Per-slice comparison against the pinned baseline (Spec S14.8 ¶3). The
// comparison is never an average: aggregate-green is insufficient (S08.10(a);
// R15 §2.4), so a red slice reds the asset even when the other slices improve.
// The planted-defect suite is the SLICE GUARD — the slice whose red is
// structurally fatal.

// Slice is one named metric with its pinned baseline and its re-measured value.
type Slice struct {
	Name string
	// Baseline is the asset's last measured value for this slice — the PINNED
	// baseline the spec compares against.
	Baseline float64
	Measured float64
	// Floor is the registered floor for the slice; HasFloor=false when none is
	// registered.
	Floor    float64
	HasFloor bool
	// Guard marks the planted-defect slice (Spec S08.10(a) slice guard).
	Guard bool
	// ReportOnly slices are recorded and NEVER red the asset — the ratified
	// over-strict-is-safe direction for the judge's true-negative rate.
	ReportOnly bool
}

// SliceResult is one slice's verdict.
type SliceResult struct {
	Slice
	Green bool
	Note  string
}

// Comparison is the per-slice verdict set plus the asset-level result.
type Comparison struct {
	Green  bool
	Slices []SliceResult
	Reason string
}

// Compare evaluates slices against their baselines and floors. A non-report-
// only slice is green iff it did not regress below its baseline AND (where a
// floor is registered) it meets that floor.
func Compare(slices []Slice) Comparison {
	out := Comparison{Green: true, Slices: make([]SliceResult, 0, len(slices))}
	var red []string
	for _, sl := range slices {
		res := SliceResult{Slice: sl, Green: true}
		switch {
		case sl.ReportOnly:
			res.Note = fmt.Sprintf("report-only: %.3f (baseline %.3f) — never reds the asset", sl.Measured, sl.Baseline)
		case sl.HasFloor && sl.Measured < sl.Floor:
			res.Green = false
			res.Note = fmt.Sprintf("%.3f below the registered floor %.3f", sl.Measured, sl.Floor)
		case sl.Measured < sl.Baseline:
			res.Green = false
			res.Note = fmt.Sprintf("%.3f regressed from the pinned baseline %.3f", sl.Measured, sl.Baseline)
		default:
			res.Note = fmt.Sprintf("%.3f vs baseline %.3f — no regression", sl.Measured, sl.Baseline)
		}
		if !res.Green {
			out.Green = false
			label := sl.Name
			if sl.Guard {
				label += " (planted-defect slice guard)"
			}
			red = append(red, label+": "+res.Note)
		}
		out.Slices = append(out.Slices, res)
	}
	if out.Green {
		out.Reason = fmt.Sprintf("all %d slices green against the pinned baseline", len(slices))
		return out
	}
	out.Reason = "red slice(s), asset stays flagged (aggregate-green is insufficient — S08.10(a)): " + strings.Join(red, "; ")
	return out
}

// Metrics renders a comparison as the metrics block of an eval.score_recorded
// payload (Spec S14.2 family 14): one entry per slice value, plus its baseline.
func (c Comparison) Metrics() map[string]any {
	m := make(map[string]any, 2*len(c.Slices)+1)
	for _, s := range c.Slices {
		m[s.Name] = s.Measured
		m[s.Name+"_baseline"] = s.Baseline
	}
	m["slices_green"] = c.Green
	return m
}

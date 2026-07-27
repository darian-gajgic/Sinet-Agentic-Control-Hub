package benchmark

import (
	"context"
	"fmt"
	"sort"
)

// feed.go is BENCH-REG §13's stage 2: the measured done-directly figure.
//
// §13, frozen text:
//
//  1. Per-run receipts, from day one: every receipt carries "direct-use estimate
//     (heuristic)". Label always present verbatim.
//  2. Aggregate honesty figure: once a domain has accrued ≥ 10 measured
//     benchmark pairs, the aggregate switches to the MEASURED MEDIAN direct-arm
//     consumption for that domain, labelled "measured (benchmark n=…)". Per-run
//     receipts keep the heuristic line; both labels remain available side by side.
//  3. The formula text above is part of this registration: the honesty keystone
//     does not float.
//
// Stage 1 is already live and stays untouched: internal/metering/receipt.go
// carries the heuristic line on every receipt forever and names this seam
// (DirectUseEstimate.MeasuredStageSeam). This file is the other side of that
// seam — the READ API S10's dashboards consume. No receipt is edited, no
// per-run line changes, and both labels are returned together, because §13.2
// says both remain available side by side.
//
// Pooling scope: ALL measured pairs of the domain, across epochs. That is §13.2's
// letter ("once a domain has accrued ≥ 10 measured benchmark pairs … the
// measured median … for that domain" — no epoch qualifier), and §9's
// no-cross-epoch-pooling rule binds DECISION statistics, which this is not: the
// done-directly figure is an honesty figure that never gates anything. The
// epochs the median spans are annotated on the result so a reader can see the
// scope rather than infer it.

// DirectUseAggregate is the §13.2 aggregate honesty figure for one domain.
type DirectUseAggregate struct {
	Domain string `json:"domain"`
	// Measured is true once the registered minimum of measured pairs has
	// accrued; until then the aggregate is the heuristic's and this figure
	// simply reports how far accrual has to go.
	Measured bool `json:"measured"`
	// N is the number of MEASURED pairs pooled (the n in the label).
	N int `json:"n"`
	// MedianUSD is the measured median direct-arm consumption. Zero and
	// meaningless while Measured is false.
	MedianUSD float64 `json:"median_usd"`
	// Label is the §13.2 label verbatim when measured; HeuristicLabel is
	// §13.1's, always present, because both labels remain available side by
	// side and per-run receipts keep the heuristic line forever.
	Label          string `json:"label,omitempty"`
	HeuristicLabel string `json:"heuristic_label"`
	// Epochs annotates the epochs the pooled pairs span (the §9 reading above).
	Epochs []string `json:"epochs,omitempty"`
	// MinimumPairs is the registered threshold, carried so a display can say
	// what it is waiting for without re-deriving it.
	MinimumPairs int    `json:"minimum_pairs"`
	Ref          string `json:"ref"`
	Marker       string `json:"marker"`
	Summary      string `json:"summary"`
}

// DoneDirectly computes the §13.2 figure for a domain.
func (p *Practice) DoneDirectly(ctx context.Context, domain string) (DirectUseAggregate, error) {
	out := DirectUseAggregate{
		Domain: domain, HeuristicLabel: LabelHeuristic,
		MinimumPairs: DoneDirectlyMinPairs, Ref: "BENCH-REG §13", Marker: RegisteredMarker,
	}
	rows, err := p.Store.db.QueryContext(ctx,
		`SELECT direct_usd, epoch_id FROM benchmark_pairs
		  WHERE domain = ? AND state = ? AND direct_measured = 1
		  ORDER BY recorded_ts, pair_id`, domain, StateRecorded)
	if err != nil {
		return out, fmt.Errorf("benchmark: done-directly pairs for %q: %w", domain, err)
	}
	defer rows.Close()
	var usd []float64
	seen := map[string]bool{}
	for rows.Next() {
		var (
			v     float64
			epoch string
		)
		if err := rows.Scan(&v, &epoch); err != nil {
			return out, fmt.Errorf("benchmark: scan done-directly pair: %w", err)
		}
		usd = append(usd, v)
		if epoch != "" && !seen[epoch] {
			seen[epoch] = true
			out.Epochs = append(out.Epochs, epoch)
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	out.N = len(usd)
	if out.N < DoneDirectlyMinPairs {
		out.Summary = fmt.Sprintf("%d measured pair(s) accrued; the aggregate switches to the measured median at the registered %d (BENCH-REG §13.2). Until then the aggregate is the heuristic: %q",
			out.N, DoneDirectlyMinPairs, LabelHeuristic)
		return out, nil
	}
	out.Measured, out.MedianUSD = true, median(usd)
	out.Label = MeasuredLabel(out.N)
	out.Summary = fmt.Sprintf("%s: median direct-arm consumption $%.4f across %d measured pair(s) in %s. Per-run receipts keep %q (BENCH-REG §13.2: both labels remain available side by side)",
		out.Label, out.MedianUSD, out.N, domain, LabelHeuristic)
	return out, nil
}

// median returns the median of xs. An even-length sample averages the two
// middle values — the ordinary definition; §13 registers the STATISTIC (the
// measured median), not a tie-breaking convention, so the standard one applies.
func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := make([]float64, len(xs))
	copy(s, xs)
	sort.Float64s(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

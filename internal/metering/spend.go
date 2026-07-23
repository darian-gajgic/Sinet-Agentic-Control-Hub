package metering

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// spend.go — the S14.4 spend-watchdog read model (brief R6/§5): the daily
// per-person PRICED total plus the trailing-window median of those daily
// totals. Money-math stays here (S14.10 Layer-0: the watchdog never computes
// money by generation — it reads the priced ledger). At v0 the price table
// ships EMPTY (§11), so every non-local row prices UNPRICED and the daily
// totals are $0 (AnyUnpriced=true): the read is built + tested DORMANT and
// activates the moment a price table is seeded.

// SpendWindow is the daily per-person spend read behind the S14.4 spend rule.
// TodayUSD is the as-of day's priced total for the person; MedianUSD is the
// median of the priced daily totals over the trailing window (the days BEFORE
// the as-of day); DaysOfHistory counts the distinct prior days with any
// observed activity (the arm-day gate reads it); AnyUnpriced is true when any
// row in the window went UNPRICED — the empty-price-table dormancy signal (§11).
type SpendWindow struct {
	UserID        string
	Day           string // the as-of UTC date (YYYY-MM-DD)
	TodayUSD      float64
	MedianUSD     float64
	DaysOfHistory int
	AnyUnpriced   bool
}

// SpendWindow computes the daily per-person priced totals over a trailing
// window ending at asOf and folds them into today's total, the trailing median,
// and the history depth. It prices the ledger exactly as RunConsumption does
// (per-row at the row's own timestamp; local rows a TRUE $0), so a mid-window
// price change is honored (Spec S10.3). Read-only, outside any caller
// transaction (Spec S10.1: aggregations are queries). windowDays ≤ 0 is treated
// as 1.
func (l *Ledger) SpendWindow(ctx context.Context, userID string, asOf time.Time, windowDays int) (SpendWindow, error) {
	if windowDays <= 0 {
		windowDays = 1
	}
	asOf = asOf.UTC()
	today := asOf.Format("2006-01-02")
	midnight := time.Date(asOf.Year(), asOf.Month(), asOf.Day(), 0, 0, 0, 0, time.UTC)
	start := midnight.AddDate(0, 0, -windowDays) // window opens windowDays before today's midnight
	end := midnight.AddDate(0, 0, 1)             // through the end of the as-of day

	rows, err := l.db.QueryContext(ctx,
		`SELECT c.model_id, r.lane, c.usage_json, c.created_ts
		   FROM checkpoints c JOIN runs r ON r.run_id = c.run_id
		  WHERE c.user_id = ? AND c.created_ts >= ? AND c.created_ts < ?`,
		userID, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))
	if err != nil {
		return SpendWindow{}, fmt.Errorf("metering: spend window for %q: %w", userID, err)
	}
	defer rows.Close()

	out := SpendWindow{UserID: userID, Day: today}
	perDay := map[string]float64{} // day → priced total; presence marks an observed day
	for rows.Next() {
		var model, lane, usage, createdTS string
		if err := rows.Scan(&model, &lane, &usage, &createdTS); err != nil {
			return SpendWindow{}, fmt.Errorf("metering: spend scan: %w", err)
		}
		ts, perr := time.Parse(time.RFC3339Nano, createdTS)
		if perr != nil {
			continue // an unparseable ts never orders or prices (S02.5); skip the row
		}
		day := ts.UTC().Format("2006-01-02")
		if _, seen := perDay[day]; !seen {
			perDay[day] = 0 // materialize the observed day even if all its rows go UNPRICED
		}

		usageJSON := []byte(usage)
		localRow := parseLocalMarker(usageJSON) != nil
		effLane := effectiveLane(lane, usageJSON)
		cur := l.exceptions.Currency(effLane)
		acc := normalize(usageJSON)
		var priced PricedCost
		if localRow {
			priced = priceLocal(cur, l.prices.Version())
		} else {
			priced = l.prices.Price(model, effLane, ts, acc, cur)
		}
		if priced.Unpriced {
			out.AnyUnpriced = true
			continue
		}
		perDay[day] += priced.CostUSD
	}
	if err := rows.Err(); err != nil {
		return SpendWindow{}, err
	}

	out.TodayUSD = perDay[today]
	var priorTotals []float64
	for day, total := range perDay {
		if day == today {
			continue
		}
		out.DaysOfHistory++
		priorTotals = append(priorTotals, total)
	}
	out.MedianUSD = median(priorTotals)
	return out, nil
}

// median returns the median of xs (0 for an empty slice). Even counts average
// the two central values.
func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

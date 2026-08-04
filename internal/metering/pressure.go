package metering

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// ⚙ keys of the S10.4 consumption-pressure gauge, owned by Spec S10.
const (
	keyCacheReadWeight  = "pressure.cache_read_weight"
	keyBgWindowFraction = "budget.background_window_fraction"
)

// Budget is the operator-declared automation budget for one (person, lane,
// period) — the D5 flat-rate currency denominator of the pressure gauge (Spec
// S10.4). It is DATA the operator edits (a 13.4 setting seeded from plan
// marketing shape), NOT a single ⚙ default, and NEVER an inferred provider
// window (D4). PeriodTokens is expressed in weighted-consumption units (the
// same unit the gauge accumulates).
//
// At B1-2 no operator budgets are declared (the 13.4 surface + persistence is
// a later phase), so callers pass an undeclared Budget and the gauge reports
// pressure as not-applicable — honest: with no declared budget there is
// nothing to gate against, and interactive work is never gated regardless
// (3.3). When the operator declares a budget it binds immediately.
type Budget struct {
	PeriodTokens int64
	PeriodStart  time.Time
	Declared     bool
}

// UndeclaredBudget is the B1-2 dev-mode posture: no operator budget (S10.4).
func UndeclaredBudget() Budget { return Budget{} }

// Gauge is one reading of the consumption-pressure gauge for a (person, lane):
// weighted consumption against the operator-declared budget (Spec S10.4). The
// cache-read weight is labeled "assumed" until subscription quota semantics
// are published (G1 Def.10) — Assumed carries that label to the receipt.
type Gauge struct {
	UserID string
	Lane   string

	// WeightedConsumption folds the period's tokens with cache reads down-
	// weighted by ⚙ pressure.cache_read_weight (S10.4); the single most
	// impactful gauge assumption on cache-heavy work (P-T08-5).
	WeightedConsumption float64
	CacheReadWeight     float64
	Assumed             bool // the cache-weight "assumed" label (G1 Def.10)

	Budget Budget

	// Pressure is WeightedConsumption / Budget.PeriodTokens; valid only when
	// Budget.Declared. BackgroundBudget is ⚙ budget.background_window_fraction
	// of the operator budget — background work's own ceiling (S10.4).
	Pressure         float64
	BackgroundBudget float64
	Applicable       bool // Budget.Declared: is Pressure meaningful?
}

// PressureGauge computes the S10.4 gauge from the consumption ledger.
type PressureGauge struct {
	db       *storage.DB
	settings Settings
}

// NewPressureGauge returns a gauge over db reading its ⚙ weights from settings.
func NewPressureGauge(db *storage.DB, settings Settings) *PressureGauge {
	return &PressureGauge{db: db, settings: settings}
}

// Read computes the gauge for (userID, lane) over the budget's period. The
// denominator is ALWAYS the operator budget, never an inferred provider window
// (D4); provider-signaled observed events (utilization.go) are an overlay and
// cross-check, never the denominator.
func (g *PressureGauge) Read(ctx context.Context, userID, lane string, budget Budget) (Gauge, error) {
	weight, err := g.settings.Float(keyCacheReadWeight)
	if err != nil {
		return Gauge{}, fmt.Errorf("metering: read ⚙ %s: %w", keyCacheReadWeight, err)
	}
	bgFraction, err := g.settings.Float(keyBgWindowFraction)
	if err != nil {
		return Gauge{}, fmt.Errorf("metering: read ⚙ %s: %w", keyBgWindowFraction, err)
	}
	consumption, err := g.weightedConsumption(ctx, userID, lane, budget.PeriodStart, budget.Declared, weight)
	if err != nil {
		return Gauge{}, err
	}
	gauge := Gauge{
		UserID:              userID,
		Lane:                lane,
		WeightedConsumption: consumption,
		CacheReadWeight:     weight,
		Assumed:             true, // S10.4: labeled "assumed" until quota semantics publish
		Budget:              budget,
	}
	if budget.Declared && budget.PeriodTokens > 0 {
		gauge.Applicable = true
		gauge.Pressure = consumption / float64(budget.PeriodTokens)
		gauge.BackgroundBudget = bgFraction * float64(budget.PeriodTokens)
	}
	return gauge, nil
}

// weightedConsumption sums the (person, lane) period consumption with cache
// reads down-weighted (Spec S10.4). When the budget is undeclared, the period
// start is the zero time and the sum covers all recorded consumption — used
// only for the observability reading, never as a denominator.
func (g *PressureGauge) weightedConsumption(ctx context.Context, userID, lane string, since time.Time, sincePinned bool, weight float64) (float64, error) {
	// Read by USER (all lanes) and match the EFFECTIVE lane in Go so the local
	// per-row lane override is honored (§8 reading 4): a local duty call meters
	// on lane "local" (the D5 floor, S12.1), not on the consuming run's paid
	// lane. With no local rows the behavior is identical to the old
	// lane-filtered query (effectiveLane == the run lane). No new pressure
	// machinery — the existing gauge honors the same override the fold does
	// (R19).
	var (
		rows *sql.Rows
		err  error
	)
	if sincePinned && !since.IsZero() {
		rows, err = g.db.QueryContext(ctx,
			`SELECT c.usage_json, r.lane FROM checkpoints c JOIN runs r ON r.run_id = c.run_id
			  WHERE r.user_id = ? AND c.created_ts >= ?`,
			userID, since.UTC().Format(time.RFC3339Nano))
	} else {
		rows, err = g.db.QueryContext(ctx,
			`SELECT c.usage_json, r.lane FROM checkpoints c JOIN runs r ON r.run_id = c.run_id
			  WHERE r.user_id = ?`, userID)
	}
	if err != nil {
		return 0, fmt.Errorf("metering: read consumption for %q/%s: %w", userID, lane, err)
	}
	defer rows.Close()
	var total float64
	for rows.Next() {
		var usage, runLane string
		if err := rows.Scan(&usage, &runLane); err != nil {
			return 0, fmt.Errorf("metering: scan consumption: %w", err)
		}
		if effectiveLane(runLane, json.RawMessage(usage)) != lane {
			continue
		}
		acc := normalize(json.RawMessage(usage))
		// Everything at full weight except cache reads at ⚙ weight (S10.4).
		total += float64(acc.PromptTokensPricedInput()) +
			float64(acc.CacheCreationTokens) +
			float64(acc.BilledOutputTokens) +
			float64(acc.CacheReadTokens)*weight
	}
	return total, rows.Err()
}

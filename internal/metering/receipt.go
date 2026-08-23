package metering

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// DirectUseLabel is the S10.10 done-directly line label, present verbatim on
// every receipt from day one (Spec/benchmark-preregistration-v1.md §13, GATE-2
// D2.8). The formula text is REGISTERED there and its numbers are cited, never
// restated (CONTRACT §binding-layer-3): this package computes against the
// registered form, it does not re-encode the threshold.
const DirectUseLabel = "direct-use estimate (heuristic)"

// DirectUseFormulaRef points at the frozen registration (cited, not restated).
const DirectUseFormulaRef = "Spec/benchmark-preregistration-v1.md §13"

// Receipt is the honest end-of-job account of what a run consumed (Spec
// S10.10, feature 3.6). It is materialized into the receipts table (Spec
// S02.2) per run-end and rendered by Spec S15's surfaces.
type Receipt struct {
	RunID  string `json:"run_id"`
	UserID string `json:"user_id"` // billed to the requester (3.4)

	// Items: consumed units per model × purpose, ceremony itemized separately
	// from execution and verification (Spec S10.10, 1.10/3.4).
	Items []LineItem `json:"items"`

	Currency           Currency          `json:"currency"`
	TotalPricedUSD     float64           `json:"total_priced_usd"`
	TotalCalls         int64             `json:"total_calls"`
	TotalUnpricedCalls int64             `json:"total_unpriced_calls"`
	WorstTier          ApproximationTier `json:"worst_tier"`

	// ParkHistory is the S10.10 park line ("parked 2 h 14 m, resumed on
	// provider signal"), computed from the run's park/resume transitions.
	ParkHistory []ParkEpisode `json:"park_history,omitempty"`

	// RequestIDs are the provider request ids the run's calls carried, in call
	// order — the S03.7 reconciliation key to an operator's provider dashboard
	// (the TBD-OPERATOR plan-unit calibration recipe).
	//
	// They sit on the RECEIPT rather than on a line item because a line
	// aggregates every call for a {model, lane, purpose}: one id there could
	// only ever name one of them, which invites exactly the wrong
	// reconciliation. Omitted entirely for a run whose calls carried none —
	// most lanes publish no per-call id, and an absent key is absent.
	RequestIDs []string `json:"request_ids,omitempty"`

	// Mode is the S10.6 mode/degradation seam. Empty at B1-2: the downgrade
	// ladder (flat-lane switch → in-lane demotion → effort reduction → local
	// fallback → park) needs routing (Spec S08, B3) and the local tier (Spec
	// S12, B4), so no mode changes occur yet. The line is present so the
	// surface shape is stable.
	Mode ModeSummary `json:"mode"`

	// DirectUse is the honesty keystone (Spec S10.10, 3.6).
	DirectUse DirectUseEstimate `json:"direct_use"`

	MaterializedTS time.Time `json:"materialized_ts"`
}

// ParkEpisode is one park→resume span (Spec S10.10).
type ParkEpisode struct {
	ParkedAt        time.Time `json:"parked_at"`
	ResumedAt       time.Time `json:"resumed_at,omitempty"`
	DurationSeconds int64     `json:"duration_seconds,omitempty"`
	ParkReason      string    `json:"park_reason,omitempty"`
	ResumeCause     string    `json:"resume_cause,omitempty"`
	Ongoing         bool      `json:"ongoing,omitempty"`
}

// ModeSummary is the S10.6 mode/degradation seam.
type ModeSummary struct {
	Note string `json:"note"`
}

// DirectUseEstimate is the two-stage done-directly figure (Spec S10.10;
// registered text Spec/benchmark-preregistration-v1.md §13). Stage 1 (per-run,
// from day one) is the heuristic below; stage 2 (the measured-median stage,
// once a domain has accrued enough benchmark pairs) is the benchmark
// machinery's (Spec S14, B5) — a seam here. Threshold + label semantics are
// registered in §13 and NOT restated.
type DirectUseEstimate struct {
	Label        string   `json:"label"` // DirectUseLabel, verbatim
	FormulaRef   string   `json:"formula_ref"`
	Unpriced     bool     `json:"unpriced"`
	Reason       string   `json:"reason,omitempty"`
	HeuristicUSD float64  `json:"heuristic_usd,omitempty"`
	Currency     Currency `json:"currency"`
	// MeasuredStageSeam notes where the aggregate switches to measured median
	// (cited, not restated).
	MeasuredStageSeam string `json:"measured_stage_seam"`
}

// Receipts materializes receipts from the ledger at run-end.
type Receipts struct {
	db         *storage.DB
	ledger     *Ledger
	exceptions MeteredExceptions
}

// NewReceipts returns a Receipts materializer.
func NewReceipts(db *storage.DB, ledger *Ledger, exceptions MeteredExceptions) *Receipts {
	return &Receipts{db: db, ledger: ledger, exceptions: exceptions}
}

// Materialize writes (or returns the existing) receipt for a finished run, in
// one write transaction (Spec S10.1: receipts materialize per run-end). It is
// idempotent: a run has exactly one receipt, so a re-dispatch or recovery pass
// does not double-write.
func (rc *Receipts) Materialize(ctx context.Context, runID string) (Receipt, error) {
	var r Receipt
	err := rc.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		r, err = rc.MaterializeTx(ctx, tx, runID)
		return err
	})
	return r, err
}

// MaterializeTx is Materialize inside the caller's transaction, so a receipt
// can commit atomically with the run's terminal transition.
func (rc *Receipts) MaterializeTx(ctx context.Context, tx *sql.Tx, runID string) (Receipt, error) {
	if existing, ok, err := rc.existingTx(ctx, tx, runID); err != nil {
		return Receipt{}, err
	} else if ok {
		return existing, nil
	}

	cons, err := rc.ledger.RunConsumptionTx(ctx, tx, runID)
	if err != nil {
		return Receipt{}, err
	}
	parks, err := parkHistoryTx(ctx, tx, runID)
	if err != nil {
		return Receipt{}, err
	}
	requestIDs, err := rc.ledger.RequestIDsTx(ctx, tx, runID)
	if err != nil {
		return Receipt{}, err
	}

	r := Receipt{
		RunID:              cons.RunID,
		UserID:             cons.UserID,
		Items:              cons.Items,
		Currency:           rc.dominantCurrency(cons.Items),
		TotalPricedUSD:     cons.TotalPricedUSD,
		TotalCalls:         cons.TotalCalls,
		TotalUnpricedCalls: cons.TotalUnpricedCalls,
		WorstTier:          cons.WorstTier,
		ParkHistory:        parks,
		RequestIDs:         requestIDs,
		Mode: ModeSummary{
			Note: "no mode change (S10.6 downgrade ladder lands with routing S08/local tier S12)",
		},
		DirectUse:      directUse(cons),
		MaterializedTS: time.Now().UTC(),
	}

	usageJSON, err := json.Marshal(r)
	if err != nil {
		return Receipt{}, fmt.Errorf("metering: marshal receipt: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO receipts (run_id, user_id, usage_json, materialized_ts)
		 VALUES (?, ?, ?, ?)`,
		r.RunID, r.UserID, string(usageJSON), r.MaterializedTS.Format(time.RFC3339Nano)); err != nil {
		return Receipt{}, fmt.Errorf("metering: insert receipt for %q: %w", runID, err)
	}
	return r, nil
}

// existingTx returns a prior receipt for the run, if any (idempotency).
func (rc *Receipts) existingTx(ctx context.Context, tx *sql.Tx, runID string) (Receipt, bool, error) {
	var usage string
	err := tx.QueryRowContext(ctx,
		`SELECT usage_json FROM receipts WHERE run_id = ? ORDER BY receipt_id LIMIT 1`, runID).Scan(&usage)
	if err == sql.ErrNoRows {
		return Receipt{}, false, nil
	}
	if err != nil {
		return Receipt{}, false, fmt.Errorf("metering: read receipt for %q: %w", runID, err)
	}
	var r Receipt
	if err := json.Unmarshal([]byte(usage), &r); err != nil {
		return Receipt{}, false, fmt.Errorf("metering: decode receipt for %q: %w", runID, err)
	}
	return r, true, nil
}

// dominantCurrency reports the receipt currency and lets a metered line flip it
// visibly (Spec S10.10, P-T01-3). At v0 every line is api-equivalent.
func (rc *Receipts) dominantCurrency(items []LineItem) Currency {
	for _, li := range items {
		if li.Currency == CurrencyReal {
			return CurrencyReal
		}
	}
	return CurrencyAPIEquiv
}

// directUse builds the stage-1 heuristic done-directly line: the final accepted
// attempt's EXECUTION usage priced at list price (Spec §13, D5 api-equivalent).
// At B1-2 a run has a single execution attempt, so the run's execution-purpose
// priced portion is the heuristic; the price table is empty at v0, so it
// renders UNPRICED — the label is present verbatim regardless (§13).
func directUse(cons RunConsumption) DirectUseEstimate {
	est := DirectUseEstimate{
		Label:             DirectUseLabel,
		FormulaRef:        DirectUseFormulaRef,
		Currency:          CurrencyAPIEquiv,
		MeasuredStageSeam: "aggregate switches to the measured-median stage per " + DirectUseFormulaRef + " once the benchmark machinery (S14, B5) has enough pairs; threshold registered there, not restated",
	}
	var priced float64
	var pricedCalls, unpricedCalls int64
	for _, li := range cons.Items {
		if li.Purpose != PurposeExecution {
			continue
		}
		priced += li.PricedUSD
		pricedCalls += li.PricedCalls
		unpricedCalls += li.UnpricedCalls
		if li.Currency == CurrencyReal {
			est.Currency = CurrencyReal
		}
	}
	if unpricedCalls > 0 || pricedCalls == 0 {
		est.Unpriced = true
		est.Reason = "no price table loaded at v0 (UNPRICED, never a silent $0 — S10.1)"
		return est
	}
	est.HeuristicUSD = priced
	return est
}

// parkHistoryTx reconstructs the run's park episodes from its run.state
// transitions (Spec S10.10). A running→parked edge opens an episode; the next
// parked→running edge closes it; an unclosed episode is Ongoing.
func parkHistoryTx(ctx context.Context, tx *sql.Tx, runID string) ([]ParkEpisode, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT payload, ts FROM run_events WHERE run_id = ? AND type = ? ORDER BY event_seq`,
		runID, run.EventState)
	if err != nil {
		return nil, fmt.Errorf("metering: read park history for %q: %w", runID, err)
	}
	defer rows.Close()

	var episodes []ParkEpisode
	var open *ParkEpisode
	for rows.Next() {
		var payload, ts string
		if err := rows.Scan(&payload, &ts); err != nil {
			return nil, fmt.Errorf("metering: scan run.state_changed: %w", err)
		}
		var p struct {
			From   string `json:"from"`
			To     string `json:"to"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			continue // tolerant: a malformed historical payload is skipped
		}
		at, _ := time.Parse(time.RFC3339Nano, ts)
		switch {
		case p.To == string(run.StateParked):
			open = &ParkEpisode{ParkedAt: at, ParkReason: p.Reason, Ongoing: true}
		case p.From == string(run.StateParked) && open != nil:
			open.ResumedAt = at
			open.Ongoing = false
			open.ResumeCause = p.Reason
			if !open.ParkedAt.IsZero() {
				open.DurationSeconds = int64(at.Sub(open.ParkedAt).Seconds())
			}
			episodes = append(episodes, *open)
			open = nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if open != nil {
		episodes = append(episodes, *open)
	}
	return episodes, nil
}

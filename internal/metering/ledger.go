package metering

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// keyDivergenceAlarm is the ⚙ tier-1-vs-tier-2 value divergence threshold
// (percent), owned by Spec S10 (S10.1; P-T08-1).
const keyDivergenceAlarm = "meter.value_divergence_alarm"

// Settings is the metering-facing view of the settings registry (Spec
// S01.10): effective ⚙ values by dotted key. *settings.Registry satisfies it.
type Settings interface {
	Float(key string) (float64, error)
}

// Ledger is the S10.1 consumption ledger: a READ MODEL over the checkpoints
// table (Spec S02.4a — "the checkpoint's usage block IS the ledger row").
// Aggregations are queries, never new state (S10.1). Pricing comes only from
// the price table (D5); the metered-exception list selects the currency (at
// v0, empty → api-equivalent everywhere).
type Ledger struct {
	db         *storage.DB
	prices     PriceTable
	exceptions MeteredExceptions
	settings   Settings
}

// NewLedger returns a Ledger over db. A nil price table is treated as an empty
// table (everything UNPRICED — the S10.1 honest v0 posture).
func NewLedger(db *storage.DB, prices PriceTable, exceptions MeteredExceptions, settings Settings) *Ledger {
	if prices == nil {
		prices = NewEffectiveDatedTable("empty")
	}
	return &Ledger{db: db, prices: prices, exceptions: exceptions, settings: settings}
}

// LineItem is one {model, lane, purpose} row of a receipt/rollup: consumed
// units, the worst approximation tier folded in, and the priced portion. It
// is the presentation unit behind the /usage per-capability breakdown (Spec
// S10.1/S10.10; the surface is Spec S15).
type LineItem struct {
	Model   string
	Lane    string
	Purpose PurposeTag
	// Tier is the WORST (numerically highest) approximation tier across the
	// folded rows — honesty takes the least-certain reading (Spec S10.1).
	Tier ApproximationTier

	Calls               int64
	PromptTokens        int64
	BilledOutputTokens  int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	ServerToolCalls     int64

	// PricedUSD is the summed cost of the priceable rows; PricedCalls and
	// UnpricedCalls split the rows. UnpricedCalls>0 means the line renders
	// with an UNPRICED remainder — never a silent zero (P-T08-1).
	PricedUSD     float64
	PricedCalls   int64
	UnpricedCalls int64
	Currency      Currency
	TableVersion  string
}

// Unpriced reports whether any call in the line went UNPRICED (drift-alert
// worthy; shown verbatim on the receipt).
func (l LineItem) Unpriced() bool { return l.UnpricedCalls > 0 }

// RunConsumption is the whole-run rollup: the per-{model,purpose} line items
// plus the run totals. It is derived entirely from the run's checkpoint rows.
type RunConsumption struct {
	RunID  string
	UserID string
	Items  []LineItem

	// TotalPricedUSD sums the priced portions; TotalUnpricedCalls counts the
	// rows that could not be priced.
	TotalPricedUSD     float64
	TotalCalls         int64
	TotalUnpricedCalls int64
	// WorstTier is the least-certain approximation tier in the run.
	WorstTier ApproximationTier
}

// checkpointRow is one ledger source row read back from the checkpoints table
// joined to its run's lane (the lane lives on runs, Spec S02.2).
type checkpointRow struct {
	model     string
	lane      string
	usageJSON json.RawMessage
	sessionID string
	msgIndex  int64
	createdTS time.Time
}

// RunConsumption computes the whole-run rollup outside any caller transaction
// — the read-model query dashboards use (Spec S10.1: aggregations are queries,
// not new state). The receipt materializer uses the Tx variant to stay atomic
// with the run-end transition.
func (l *Ledger) RunConsumption(ctx context.Context, runID string) (RunConsumption, error) {
	var rc RunConsumption
	err := l.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var err error
		rc, err = l.RunConsumptionTx(ctx, tx, runID)
		return err
	})
	return rc, err
}

// RunConsumptionTx computes the run rollup inside the caller's transaction, so
// it can be read atomically with a run-end transition (receipt materialization,
// receipt.go). It folds the run's checkpoint rows into per-{model,purpose}
// lines, pricing each row at its own timestamp against the price table.
func (l *Ledger) RunConsumptionTx(ctx context.Context, tx *sql.Tx, runID string) (RunConsumption, error) {
	rows, err := l.readCheckpointsTx(ctx, tx, runID)
	if err != nil {
		return RunConsumption{}, err
	}
	var userID string
	if err := tx.QueryRowContext(ctx, `SELECT user_id FROM runs WHERE run_id = ?`, runID).Scan(&userID); err != nil {
		if err == sql.ErrNoRows {
			return RunConsumption{}, fmt.Errorf("metering: run %q not found", runID)
		}
		return RunConsumption{}, fmt.Errorf("metering: read run owner: %w", err)
	}
	return l.fold(runID, userID, rows), nil
}

// fold aggregates checkpoint rows into a RunConsumption. Purpose is derived
// (execution-only at B1-2, see derivePurpose); pricing is per-row at the row's
// timestamp then summed, so a mid-run price change is honored (Spec S10.3).
func (l *Ledger) fold(runID, userID string, rows []checkpointRow) RunConsumption {
	type key struct {
		model, lane string
		purpose     PurposeTag
	}
	agg := map[key]*LineItem{}
	order := []key{}
	rc := RunConsumption{RunID: runID, UserID: userID, WorstTier: TierMeasured}

	for _, r := range rows {
		acc := normalize(r.usageJSON)
		purpose := derivePurpose(runID, r)
		cur := l.exceptions.Currency(r.lane)
		k := key{r.model, r.lane, purpose}
		li := agg[k]
		if li == nil {
			li = &LineItem{Model: r.model, Lane: r.lane, Purpose: purpose, Tier: TierMeasured, Currency: cur, TableVersion: l.prices.Version()}
			agg[k] = li
			order = append(order, k)
		}
		li.Calls++
		li.PromptTokens += acc.PromptTokens
		li.BilledOutputTokens += acc.BilledOutputTokens
		li.CacheReadTokens += acc.CacheReadTokens
		li.CacheCreationTokens += acc.CacheCreationTokens
		li.ServerToolCalls += acc.ServerToolCalls
		if t := acc.tier(); t > li.Tier {
			li.Tier = t
		}
		if t := acc.tier(); t > rc.WorstTier {
			rc.WorstTier = t
		}

		priced := l.prices.Price(r.model, r.lane, r.createdTS, acc, cur)
		if priced.Unpriced {
			li.UnpricedCalls++
			rc.TotalUnpricedCalls++
		} else {
			li.PricedUSD += priced.CostUSD
			li.PricedCalls++
			rc.TotalPricedUSD += priced.CostUSD
		}
		rc.TotalCalls++
	}

	for _, k := range order {
		rc.Items = append(rc.Items, *agg[k])
	}
	return rc
}

// derivePurpose classifies a usage row's purpose — still in ONE place (the
// B1-2 shape), with the input the B2 pipeline changed: a task's engine work
// now runs under ROLE-named runs (the B2-4 walking-skeleton convention,
// extending intake's fixed `<task>.intake`, CONVENTIONS §14): intake
// ceremony sessions ride `<task>.intake`, verification sessions ride
// `<task>.verify`, everything else is execution. So ceremony and
// verification itemize separately on receipts (1.10/3.4, Spec S06.10/
// S07.11) without a schema change. Per-SESSION purpose splits inside one
// run (e.g. a rework executor round inside the verify run, Spec S07.6)
// arrive when sessions get first-class identity (S08/S13); until then the
// verify run's whole consumption is the verification tax and the per-round
// itemization lives in the verify.round records.
func derivePurpose(runID string, _ checkpointRow) PurposeTag {
	base := stripForkSuffix(runID)
	switch {
	case strings.HasSuffix(base, RunSuffixIntake):
		return PurposeCeremony
	case strings.HasSuffix(base, RunSuffixVerify):
		return PurposeVerification
	default:
		return PurposeExecution
	}
}

// stripForkSuffix removes trailing `.g<n>` recovery-fork segments (Spec
// S02.5: successor run ids are `<parent>.g<newGen>`) so a forked role run
// keeps its role's purpose.
func stripForkSuffix(runID string) string {
	for {
		i := strings.LastIndex(runID, ".g")
		if i < 0 || i+2 >= len(runID) {
			return runID
		}
		digits := runID[i+2:]
		for _, r := range digits {
			if r < '0' || r > '9' {
				return runID
			}
		}
		runID = runID[:i]
	}
}

// Role-run suffixes (the B2 walking-skeleton run-naming convention; intake
// fixed `.intake` at B2-2, the skeleton fixed `.execute`/`.verify` at
// B2-4). Owned here because purpose derivation is this package's duty
// (S10.1); the skeleton names its runs with these same constants.
const (
	RunSuffixIntake  = ".intake"
	RunSuffixExecute = ".execute"
	RunSuffixVerify  = ".verify"
)

// readCheckpointsTx reads a run's checkpoint rows (usage + model) joined to the
// run lane, in checkpoint order (event_seq is the sole ordering authority,
// Spec S02.5).
func (l *Ledger) readCheckpointsTx(ctx context.Context, tx *sql.Tx, runID string) ([]checkpointRow, error) {
	sqlRows, err := tx.QueryContext(ctx,
		`SELECT c.model_id, r.lane, c.usage_json, c.session_id, c.message_index, c.created_ts
		   FROM checkpoints c JOIN runs r ON r.run_id = c.run_id
		  WHERE c.run_id = ? ORDER BY c.event_seq`, runID)
	if err != nil {
		return nil, fmt.Errorf("metering: read checkpoints for %q: %w", runID, err)
	}
	defer sqlRows.Close()
	var out []checkpointRow
	for sqlRows.Next() {
		var (
			cr        checkpointRow
			usage     string
			msgIndex  sql.NullInt64
			createdTS string
		)
		if err := sqlRows.Scan(&cr.model, &cr.lane, &usage, &cr.sessionID, &msgIndex, &createdTS); err != nil {
			return nil, fmt.Errorf("metering: scan checkpoint: %w", err)
		}
		cr.usageJSON = json.RawMessage(usage)
		cr.msgIndex = msgIndex.Int64
		if t, err := time.Parse(time.RFC3339Nano, createdTS); err == nil {
			cr.createdTS = t
		}
		out = append(out, cr)
	}
	return out, sqlRows.Err()
}

// DivergenceAlarm is the tier-1-vs-tier-2 cross-check result (Spec S10.1,
// ⚙ meter.value_divergence_alarm): the platform's priced cost (tier 1 × the
// price table) versus the engine's own estimate (tier 2). It catches both
// price staleness and engine bugs — the 10× cost-inflation class of P-T08-1.
type DivergenceAlarm struct {
	Fired           bool
	PricedUSD       float64
	EngineUSD       float64
	Fraction        float64 // observed |priced-engine|/engine
	ThresholdFrac   float64 // ⚙ meter.value_divergence_alarm as a fraction
	Comparable      bool    // false when the priced side is UNPRICED (v0 posture)
	UnpricedRemarks string
}

// CheckDivergence compares a priced total against the engine's cross-check
// estimate. When the priced side is UNPRICED (the B1-2 empty-table posture) or
// no engine estimate exists, the alarm is not comparable and never fires — the
// ⚙ threshold is still read so the wiring is exercised. Machinery consumers
// (the watchdog, Spec S14) subscribe to a fired alarm.
func (l *Ledger) CheckDivergence(ctx context.Context, priced RunConsumption, engineEstimateUSD float64, engineAvailable bool) (DivergenceAlarm, error) {
	thresholdPct, err := l.settings.Float(keyDivergenceAlarm)
	if err != nil {
		return DivergenceAlarm{}, fmt.Errorf("metering: read ⚙ %s: %w", keyDivergenceAlarm, err)
	}
	al := DivergenceAlarm{
		PricedUSD:     priced.TotalPricedUSD,
		EngineUSD:     engineEstimateUSD,
		ThresholdFrac: thresholdPct / 100,
	}
	if priced.TotalUnpricedCalls > 0 || priced.TotalPricedUSD == 0 {
		al.UnpricedRemarks = "priced side UNPRICED (no price table loaded at v0); divergence not comparable"
		return al, nil
	}
	if !engineAvailable {
		al.UnpricedRemarks = "no engine cross-check estimate available"
		return al, nil
	}
	al.Comparable = true
	al.Fraction = absFrac(priced.TotalPricedUSD, engineEstimateUSD)
	al.Fired = al.Fraction > al.ThresholdFrac
	return al, nil
}

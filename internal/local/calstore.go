package local

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// calstore.go — the durable, queryable home for S12.5 calibration records +
// S12.9 T15 battery results (OQ1(a); migration 0010). Keyed (duty, model hash,
// engine build) — the S12.10 swap gate's P-T15-1 checks read it, and
// EntailmentGate.Calibrated (S07.4) reads the entailment duty's record. The
// Def.8 result files in P3/measurements/ remain the human-readable record;
// this store is what the machinery queries. Platform-owned data. It imports
// storage only (CONVENTIONS §26 wall) and upserts (recalibration/re-run
// replaces a key's row — latest-wins, not an event log).

// CalStore persists + reads calibration records and battery suite results.
type CalStore struct {
	db  *storage.DB
	now func() time.Time
}

// NewCalStore wraps the platform DB.
func NewCalStore(db *storage.DB) *CalStore {
	return &CalStore{db: db, now: time.Now}
}

// SaveCalibration upserts a fitted calibration (latest-wins per key).
func (s *CalStore) SaveCalibration(ctx context.Context, c Calibration) error {
	if s == nil || s.db == nil {
		return ErrStackAbsent
	}
	if c.Key.Duty == "" || c.Key.ModelHash == "" || c.Key.EngineBuild == "" {
		return fmt.Errorf("local: calibration record needs a full (duty, model hash, engine build) key")
	}
	iso, err := json.Marshal(c.Isotonic)
	if err != nil {
		return fmt.Errorf("local: marshal isotonic map: %w", err)
	}
	ms, err := json.Marshal(c.Margins)
	if err != nil {
		return fmt.Errorf("local: marshal margin summary: %w", err)
	}
	meets := 0
	if c.MeetsBar {
		meets = 1
	}
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO calibration_records
			  (duty, model_hash, engine_build, threshold, acceptance_bar, achieved_error,
			   accept_rate, labeled_n, meets_bar, isotonic_json, margin_summary_json, calibrated_ts, user_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'platform')
			ON CONFLICT(duty, model_hash, engine_build) DO UPDATE SET
			  threshold=excluded.threshold, acceptance_bar=excluded.acceptance_bar,
			  achieved_error=excluded.achieved_error, accept_rate=excluded.accept_rate,
			  labeled_n=excluded.labeled_n, meets_bar=excluded.meets_bar,
			  isotonic_json=excluded.isotonic_json, margin_summary_json=excluded.margin_summary_json,
			  calibrated_ts=excluded.calibrated_ts`,
			c.Key.Duty, c.Key.ModelHash, c.Key.EngineBuild, c.Threshold, c.AcceptanceBar, c.AchievedError,
			c.AcceptRate, c.LabeledN, meets, string(iso), string(ms), s.now().UTC().Format(time.RFC3339))
		return err
	})
}

// Calibration reads a calibration record for a key. ok=false when none exists
// (the caller then keeps the ungated v0 behaviour — R3).
func (s *CalStore) Calibration(ctx context.Context, key CalibrationKey) (Calibration, bool, error) {
	if s == nil || s.db == nil {
		return Calibration{}, false, ErrStackAbsent
	}
	var (
		c       Calibration
		iso, ms string
		meets   int
	)
	c.Key = key
	err := s.db.QueryRowContext(ctx, `
		SELECT threshold, acceptance_bar, achieved_error, accept_rate, labeled_n, meets_bar, isotonic_json, margin_summary_json
		  FROM calibration_records WHERE duty = ? AND model_hash = ? AND engine_build = ?`,
		key.Duty, key.ModelHash, key.EngineBuild).Scan(
		&c.Threshold, &c.AcceptanceBar, &c.AchievedError, &c.AcceptRate, &c.LabeledN, &meets, &iso, &ms)
	if errors.Is(err, sql.ErrNoRows) {
		return Calibration{}, false, nil
	}
	if err != nil {
		return Calibration{}, false, fmt.Errorf("local: read calibration record: %w", err)
	}
	c.MeetsBar = meets == 1
	if err := json.Unmarshal([]byte(iso), &c.Isotonic); err != nil {
		return Calibration{}, false, fmt.Errorf("local: decode isotonic map: %w", err)
	}
	if err := json.Unmarshal([]byte(ms), &c.Margins); err != nil {
		return Calibration{}, false, fmt.Errorf("local: decode margin summary: %w", err)
	}
	return c, true, nil
}

// HasCalibration reports whether a fresh calibration exists for a key — the
// P-T15-1 check (R11).
func (s *CalStore) HasCalibration(ctx context.Context, key CalibrationKey) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrStackAbsent
	}
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM calibration_records WHERE duty = ? AND model_hash = ? AND engine_build = ?`,
		key.Duty, key.ModelHash, key.EngineBuild).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("local: calibration existence check: %w", err)
	}
	return n > 0, nil
}

// SaveBatterySuite upserts one duty suite's battery result (latest-wins per
// battery version × target).
func (s *CalStore) SaveBatterySuite(ctx context.Context, r SuiteResult) error {
	if s == nil || s.db == nil {
		return ErrStackAbsent
	}
	if r.Duty == "" || r.ModelHash == "" || r.EngineBuild == "" || r.BatteryVersion == "" {
		return fmt.Errorf("local: battery result needs (battery version, duty, model hash, engine build)")
	}
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO battery_results
			  (battery_version, duty, model_hash, engine_build, cases_total, cases_passed, tokens_per_s, wilson_lo, wilson_hi, ran_ts, user_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'platform')
			ON CONFLICT(battery_version, duty, model_hash, engine_build) DO UPDATE SET
			  cases_total=excluded.cases_total, cases_passed=excluded.cases_passed,
			  tokens_per_s=excluded.tokens_per_s, wilson_lo=excluded.wilson_lo, wilson_hi=excluded.wilson_hi,
			  ran_ts=excluded.ran_ts`,
			r.BatteryVersion, r.Duty, r.ModelHash, r.EngineBuild, r.Total, r.Passed, r.TokensPerSec, r.WilsonLo, r.WilsonHi,
			s.now().UTC().Format(time.RFC3339))
		return err
	})
}

// HasBatterySuite reports whether any battery suite ran for a (duty, model
// hash, engine build) target — the P-T15-1 "affected suite re-ran" check (R11).
func (s *CalStore) HasBatterySuite(ctx context.Context, duty, modelHash, engineBuild string) (bool, error) {
	if s == nil || s.db == nil {
		return false, ErrStackAbsent
	}
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM battery_results WHERE duty = ? AND model_hash = ? AND engine_build = ?`,
		duty, modelHash, engineBuild).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("local: battery suite existence check: %w", err)
	}
	return n > 0, nil
}

// BatterySuites reads all suite results for a battery version + target model,
// for the promotion-rule comparison (R10). engineBuild is the current pin.
func (s *CalStore) BatterySuites(ctx context.Context, batteryVersion, modelHash, engineBuild string) ([]SuiteResult, error) {
	if s == nil || s.db == nil {
		return nil, ErrStackAbsent
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT duty, cases_total, cases_passed, tokens_per_s, wilson_lo, wilson_hi
		  FROM battery_results WHERE battery_version = ? AND model_hash = ? AND engine_build = ? ORDER BY duty`,
		batteryVersion, modelHash, engineBuild)
	if err != nil {
		return nil, fmt.Errorf("local: read battery suites: %w", err)
	}
	defer rows.Close()
	var out []SuiteResult
	for rows.Next() {
		r := SuiteResult{ModelHash: modelHash, EngineBuild: engineBuild, BatteryVersion: batteryVersion}
		if err := rows.Scan(&r.Duty, &r.Total, &r.Passed, &r.TokensPerSec, &r.WilsonLo, &r.WilsonHi); err != nil {
			return nil, fmt.Errorf("local: scan battery suite: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

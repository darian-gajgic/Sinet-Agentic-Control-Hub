// Package conformance is the S14.5 conformance registry: the control-plane
// scheduling HOME for every "proven, not assumed" obligation the spec declares
// (Spec S14 term coinage line 7; body S14.5). It owns the conformance_registry
// table (migration 0011) — the standing suites and drills as rows (id, owning
// section, fixtures, trigger set, schedule, last run, last result) — plus the
// recording surface (results land as eval.score_recorded events), the dueness
// read surface, and the red→card derivation input.
//
// It is a scheduling home, NOT an executor: it never runs a Go-test battery,
// spawns an engine, or dials anything. Suite CONTENT stays with the declaring
// section (Spec S14.5); the weekly RUN of the adapter suites is the S14.6
// canary layer's (B5-6), the regression sweep is S14.8's (B5-5). Dueness is a
// passive read surface derived from stored state (never an in-memory ticker),
// so it survives restart and suspend.
//
// Imports are storage + eventlog + settings + stdlib ONLY (AST-pinned): it
// never imports the producer packages whose suites it registers, and it never
// imports api. Derive rows (restore-drill, dead-man) read the existing
// platform.restore_drill / platform.deadman.* events by their canonical type
// STRINGS — never a Go import edge to internal/backup or internal/watchdog.
package conformance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// EventScoreRecorded is the S14.2 Benchmark & eval family type this package
// mints: a conformance-suite result recorded in Sinet's DB (Spec S14.5
// "results land as eval.score_recorded events"; P-T11-4 runners-never-stores).
// B5-5 (regression evals) and B5-7 (BENCH-REG pairs) are future co-producers
// of the same family; benchmark.pair_recorded stays declare-only (its numbers
// change only via BENCH-REG §17).
const EventScoreRecorded = "eval.score_recorded"

// Settings is this package's narrow view of the settings registry (CONVENTIONS
// §2: consumers read ⚙ values by dotted key through per-package interfaces).
// *settings.Registry satisfies it. Used only for dueness: the settings-backed
// cadences (backup.drill_interval, verification.drill_interval_days) and the
// visible-only escalation-canary schedule (verification.canary_interval_hours)
// are read live at evaluation time — never a hardcoded ⚙ value.
type Settings interface {
	Int(key string) (int64, error)
}

// Recording-surface errors. Callers branch on these.
var (
	// ErrUnknownRow: no registry row has the given id.
	ErrUnknownRow = errors.New("conformance: unknown registry row")
	// ErrDeriveRow: the row is a DERIVE row (restore-drill / dead-man) whose
	// last run/result come from its own subsystem's events, not the recording
	// surface (OQ2/OQ5(c)). Recording a result on it would double-source.
	ErrDeriveRow = errors.New("conformance: derive-row result is owned by its subsystem, not the recording surface")
	// ErrIncompleteResult: a result is missing a contract-minimum field.
	ErrIncompleteResult = errors.New("conformance: result missing a contract-minimum field (Spec S14.2)")
)

// Store is the S14.5 registry over platform.db plus the eventlog recording
// surface. Imports stay storage + eventlog + settings (the review.Store shape).
type Store struct {
	db       *storage.DB
	log      *eventlog.Log
	settings Settings
	// Now is the test clock seam; nil ⇒ time.Now.
	Now func() time.Time
}

// NewStore returns a registry Store over db, recording through log and reading
// ⚙ cadences from settings.
func NewStore(db *storage.DB, log *eventlog.Log, settings Settings) *Store {
	return &Store{db: db, log: log, settings: settings}
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Result is one recorded conformance-suite outcome. The recording surface is a
// control-plane verb usable by CI/operator ingestion now and by the B5-5/B5-6
// executors later (the R12 seam); it accepts an arbitrary runner identity.
type Result struct {
	// RowID is the conformance_registry row id — the suite id (R11 mapping).
	RowID string
	// SuiteVersion is the fixture/suite version (R11).
	SuiteVersion string
	// AssetID / AssetVersion identify the component under test — the engine
	// pin, the storage schema, etc. (R11).
	AssetID      string
	AssetVersion string
	// Runner / RunnerVersion are the run handle and its toolchain/engine
	// identity (R11).
	Runner        string
	RunnerVersion string
	// Metrics are the suite's measured metrics (pass/fail counts, rates). nil
	// records an empty metrics object — the contract-minimum key is still
	// present.
	Metrics map[string]any
	// Passed maps to last_result green|red.
	Passed bool
	// RanAt defaults to the store clock when zero.
	RanAt time.Time
}

// scorePayload is the eval.score_recorded payload: the S14.2 Benchmark & eval
// contract minimum verbatim (suite id + version, asset id + version, metrics,
// runner + runner version) plus the pass/fail result the row mirrors.
type scorePayload struct {
	SuiteID       string         `json:"suite_id"`
	SuiteVersion  string         `json:"suite_version"`
	AssetID       string         `json:"asset_id"`
	AssetVersion  string         `json:"asset_version"`
	Metrics       map[string]any `json:"metrics"`
	Runner        string         `json:"runner"`
	RunnerVersion string         `json:"runner_version"`
	Result        string         `json:"result"`
}

// RecordResult records a suite outcome: it appends one eval.score_recorded
// event AND updates the row's last_run/last_result in ONE WriteTx (Spec S14.5;
// the §6/§8 same-tx state+event discipline — both land atomically or neither).
// The event is platform-scope (a suite run has no platform run; asks.run_id is
// NOT NULL, so a red result never becomes a bare asks row — it surfaces by
// derivation, R15). A DERIVE row is refused (ErrDeriveRow): its result is owned
// by its own subsystem's events.
func (s *Store) RecordResult(ctx context.Context, r Result) error {
	switch {
	case r.RowID == "", r.SuiteVersion == "", r.AssetID == "", r.AssetVersion == "", r.Runner == "", r.RunnerVersion == "":
		return ErrIncompleteResult
	}
	ranAt := r.RanAt
	if ranAt.IsZero() {
		ranAt = s.now()
	}
	result := ResultGreen
	if !r.Passed {
		result = ResultRed
	}
	metrics := r.Metrics
	if metrics == nil {
		metrics = map[string]any{}
	}
	payload, err := json.Marshal(scorePayload{
		SuiteID:       r.RowID,
		SuiteVersion:  r.SuiteVersion,
		AssetID:       r.AssetID,
		AssetVersion:  r.AssetVersion,
		Metrics:       metrics,
		Runner:        r.Runner,
		RunnerVersion: r.RunnerVersion,
		Result:        result,
	})
	if err != nil {
		return fmt.Errorf("conformance: marshal score payload: %w", err)
	}
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var deriveKind string
		err := tx.QueryRowContext(ctx,
			`SELECT derive_kind FROM conformance_registry WHERE row_id = ?`, r.RowID).Scan(&deriveKind)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %q", ErrUnknownRow, r.RowID)
		}
		if err != nil {
			return fmt.Errorf("conformance: look up row %q: %w", r.RowID, err)
		}
		if deriveKind != DeriveNone {
			return fmt.Errorf("%w: %q (derive_kind %q)", ErrDeriveRow, r.RowID, deriveKind)
		}
		if _, err := s.log.AppendTx(ctx, tx, eventlog.Append{
			UserID:        platformUser,
			Type:          EventScoreRecorded,
			SchemaVersion: 1,
			Payload:       payload,
			Time:          ranAt,
		}); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE conformance_registry SET last_run = ?, last_result = ? WHERE row_id = ?`,
			ranAt.UTC().Format(time.RFC3339Nano), result, r.RowID); err != nil {
			return fmt.Errorf("conformance: update row %q result: %w", r.RowID, err)
		}
		return nil
	})
}

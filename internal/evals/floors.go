package evals

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// The floor registry (Spec S14.8 ¶2; BENCH-REG §10.1(d)): floors register per
// (rubric/eval-set id, VERSION) at 8.3 knowledge-gate entry — the same
// governance entry point the B2 seed objects already ride (S09.10), not a
// second gate. Gate-limb (d) evaluation and display are B5-7's; this package
// owns the store and the per-suite green/red check.

// Seed floor values (OQ5(a), coordinator-dispositioned): the floor IS the
// MEASURED Wilson 95% lower bound of the B4-7 rider-1 golden-set run — a floor
// is measurement-derived, never invented, and the point estimate would be
// brittle at n=20 (a single miss would red the rubric).
//
// These are named code constants, not ⚙ values: S18 ratifies no key for them
// (the §7 sseBatchSize / §9 auth-constant precedent, under the standing
// settings-tab directive). Operator RATIFICATION of the seeded floors is
// flagged to the B5 gate; until then the registered rows carry ratified=0.
const (
	// rubricSoftwareCatchFloor: TPR 20/20 = 1.000 measured on claude-opus-4-8,
	// Wilson 95% interval [0.84, 1.00] → floor 0.84.
	rubricSoftwareCatchFloor = 0.84
	// rubricSoftwareTNR: TNR 3/6 = 0.500, Wilson 95% [0.19, 0.81] → carried
	// REPORT-ONLY at 0.19. A TNR floor would flag the judge for being
	// CONSERVATIVE — the over-strict direction the B4 gate accepted as safe for
	// a quality gate (D4, 2026-07-23).
	rubricSoftwareTNR = 0.19
)

// floorBasis is the provenance every seeded floor carries.
const floorBasis = "measured 2026-07-22 on claude-opus-4-8 (B4-7 rider 1, P-T06-5; " +
	"P3/measurements/2026-07-22-rider1-golden-set-opus.md): planted-defect catch 20/20 = 1.000, " +
	"Wilson 95% [0.84, 1.00] → floor = the lower bound 0.84. Clean controls 3/6 = 0.500, " +
	"Wilson 95% [0.19, 0.81] → carried report-only. Operator ratification pending at the B5 gate."

// Floor is one registered per-version floor.
type Floor struct {
	AssetKind    string
	AssetID      string
	AssetVersion int
	// CatchFloor is the registered planted-defect catch floor (Spec S14.8 ¶5).
	CatchFloor float64
	// TNRReportOnly is the companion true-negative rate, recorded and never
	// used to red an asset (OQ5(a) over-strict-is-safe direction). nil = none.
	TNRReportOnly *float64
	Basis         string
	// Ratified reports operator ratification at the asset's phase gate.
	Ratified     bool
	RegisteredTS time.Time
}

// SeedFloors returns the v0 registered floors, derived from the in-tree assets
// so a floor always tracks the version it was measured against.
func SeedFloors() []Floor {
	r := verify.SeedSoftwareRubric()
	tnr := rubricSoftwareTNR
	return []Floor{{
		AssetKind: AssetRubric, AssetID: r.ID, AssetVersion: r.Version,
		CatchFloor: rubricSoftwareCatchFloor, TNRReportOnly: &tnr, Basis: floorBasis,
	}}
}

// EnsureFloorsRegistered registers the seed floors idempotently — the 8.3
// knowledge-gate entry hook (Spec S14.8 ¶2). It is insert-once: an already
// registered (asset, version) is left exactly as it is, so a ratification flip
// or a later measurement is never overwritten by a boot. It returns the number
// of newly registered floors.
func (s *Store) EnsureFloorsRegistered(ctx context.Context) (int, error) {
	registered := 0
	for _, f := range SeedFloors() {
		err := s.RegisterFloor(ctx, f)
		switch {
		case errors.Is(err, ErrFloorRegistered):
			continue
		case err != nil:
			return registered, err
		}
		registered++
	}
	return registered, nil
}

// RegisterFloor registers a floor for one asset version. A floor registers ONCE
// per version (ErrFloorRegistered on a repeat): re-flooring is a new asset
// version, which is what per-version keying exists to enforce.
func (s *Store) RegisterFloor(ctx context.Context, f Floor) error {
	if f.AssetID == "" || f.AssetVersion < 1 || f.Basis == "" {
		return fmt.Errorf("evals: floor requires asset id, version ≥ 1 and a measurement basis")
	}
	ts := f.RegisteredTS
	if ts.IsZero() {
		ts = s.now()
	}
	ratified := 0
	if f.Ratified {
		ratified = 1
	}
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM eval_floors WHERE asset_id = ? AND asset_version = ?`,
			f.AssetID, f.AssetVersion).Scan(&n); err != nil {
			return fmt.Errorf("evals: floor existence check: %w", err)
		}
		if n > 0 {
			return fmt.Errorf("%w: %s v%d", ErrFloorRegistered, f.AssetID, f.AssetVersion)
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO eval_floors
			   (asset_kind, asset_id, asset_version, catch_floor, tnr_report_only,
			    basis, ratified, registered_ts, user_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			f.AssetKind, f.AssetID, f.AssetVersion, f.CatchFloor, f.TNRReportOnly,
			f.Basis, ratified, ts.UTC().Format(time.RFC3339Nano), platformUser)
		if err != nil {
			return fmt.Errorf("evals: register floor %s v%d: %w", f.AssetID, f.AssetVersion, err)
		}
		return nil
	})
}

// Ratify records operator ratification of a registered floor — the one mutable
// field on the row (the seeded values enter gate-pending).
func (s *Store) Ratify(ctx context.Context, assetID string, version int) error {
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE eval_floors SET ratified = 1 WHERE asset_id = ? AND asset_version = ?`,
			assetID, version)
		if err != nil {
			return fmt.Errorf("evals: ratify floor %s v%d: %w", assetID, version, err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return fmt.Errorf("%w: %s v%d", ErrNoFloor, assetID, version)
		}
		return nil
	})
}

// Floor returns the registered floor for an asset version, or ErrNoFloor.
func (s *Store) Floor(ctx context.Context, assetID string, version int) (Floor, error) {
	var f Floor
	var tnr sql.NullFloat64
	var ratified int
	var ts string
	err := s.db.QueryRowContext(ctx,
		`SELECT asset_kind, asset_id, asset_version, catch_floor, tnr_report_only,
		        basis, ratified, registered_ts
		   FROM eval_floors WHERE asset_id = ? AND asset_version = ?`,
		assetID, version).Scan(&f.AssetKind, &f.AssetID, &f.AssetVersion, &f.CatchFloor,
		&tnr, &f.Basis, &ratified, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return Floor{}, fmt.Errorf("%w: %s v%d", ErrNoFloor, assetID, version)
	}
	if err != nil {
		return Floor{}, fmt.Errorf("evals: read floor %s v%d: %w", assetID, version, err)
	}
	if tnr.Valid {
		v := tnr.Float64
		f.TNRReportOnly = &v
	}
	f.Ratified = ratified == 1
	f.RegisteredTS = parseTS(ts)
	return f, nil
}

// FloorCheck is one rubric-falsifiability verdict (Spec S14.8 ¶5): a rubric
// version "works" iff its planted-defect suite catches at least its registered
// floor, measured per rubric version. Below-floor is RED — the version is
// flagged to its owner, never silently passed.
type FloorCheck struct {
	AssetID      string
	AssetVersion int
	// Registered reports whether a floor exists at all. Without one the
	// version is not evaluable, so it is not green (BENCH-REG §10.1(d): the
	// gate cannot open while a suite lacks a registered floor).
	Registered bool
	Floor      float64
	// Catch is the measured planted-defect catch rate.
	Catch float64
	Green bool
	// Reason states the verdict in one line, for the card and the record.
	Reason string
}

// CheckFloor evaluates one asset version's planted-defect catch rate against
// its registered floor.
func (s *Store) CheckFloor(ctx context.Context, assetID string, version int, catch float64) (FloorCheck, error) {
	chk := FloorCheck{AssetID: assetID, AssetVersion: version, Catch: catch}
	f, err := s.Floor(ctx, assetID, version)
	if errors.Is(err, ErrNoFloor) {
		chk.Reason = fmt.Sprintf("%s v%d has no registered floor — falsifiability is not evaluable (BENCH-REG §10.1(d))", assetID, version)
		return chk, nil
	}
	if err != nil {
		return FloorCheck{}, err
	}
	chk.Registered, chk.Floor = true, f.CatchFloor
	chk.Green = catch >= f.CatchFloor
	if chk.Green {
		chk.Reason = fmt.Sprintf("%s v%d planted-defect catch %.3f ≥ registered floor %.3f", assetID, version, catch, f.CatchFloor)
	} else {
		chk.Reason = fmt.Sprintf("%s v%d planted-defect catch %.3f BELOW registered floor %.3f — the rubric version does not falsify (Spec S14.8 ¶5)", assetID, version, catch, f.CatchFloor)
	}
	return chk, nil
}

// parseTS parses a stored RFC3339Nano timestamp, returning the zero time on a
// malformed value (ordering is by event_seq, never timestamps — P-T07-4).
func parseTS(ts string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}
	}
	return t
}

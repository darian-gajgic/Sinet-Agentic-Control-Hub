package conformance

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// daysPerMonth is the coarse structural month for the passive dueness surface.
// S14.5 is a scheduling HOME, not a precise scheduler (OQ3: dueness is a passive
// read surface that makes overdue-ness trivially visible), so a 30-day month is
// adequate and keeps quarterly = 3 months = 90 days consistent.
const daysPerMonth = 30

// Structural cadence intervals (OQ6(a): quarterly = 3 months, weekly = 7 days;
// OQ5(c)/§31: dead-man daily = 24h). Row seed data, not new settings.
const (
	quarterlyInterval = 3 * daysPerMonth * 24 * time.Hour
	weeklyInterval    = 7 * 24 * time.Hour
	dailyInterval     = 24 * time.Hour
)

// EnsureSeeded idempotently seeds the S14.5 registry rows (INSERT OR IGNORE, so
// re-running is a no-op and a committed row is never rewritten — the §17 seed
// precedent; the structure-immutable trigger also blocks any structural update).
// It returns the number of seed rows. Seeding emits no events (the rows are seed
// data, not results).
func (s *Store) EnsureSeeded(ctx context.Context) (int, error) {
	rows := SeedRows()
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		for _, r := range rows {
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO conformance_registry
				   (row_id, owning_section, fixtures, trigger_set, schedule, cadence,
				    canary_cadence, affect_class, derive_kind, notes, user_id)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				r.ID, r.OwningSection, fixturesText(r.Fixtures), strings.Join(r.TriggerSet, ","),
				r.Schedule, r.Cadence, r.CanaryCadence, r.AffectClass, r.DeriveKind, r.Notes, platformUser,
			); err != nil {
				return fmt.Errorf("conformance: seed row %q: %w", r.ID, err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

// RowState is one registry row's static structure plus its resolved runtime
// state (last run/result, dueness). It is the read surface B6 renders (Spec
// S14.5; the §31 "HTTP endpoint + React are B6/S15" posture — this package
// exposes the projection, not the surface).
type RowState struct {
	ID            string
	OwningSection string
	Fixtures      string
	TriggerSet    string
	Schedule      string
	Cadence       string
	AffectClass   string
	DeriveKind    string
	Notes         string
	// LastRun / HasRun: the last recorded/derived run time; HasRun=false means
	// never run.
	LastRun time.Time
	HasRun  bool
	// LastResult is green|red|unknown, or "" when never run.
	LastResult string
	// Due is true when the row has never run or has passed its cadence interval
	// (OQ3: a passive read surface — nothing fires on dueness, only on a red
	// RECORDED result).
	Due bool
	// FlagNow reports whether a red result routes flag-now (R14): lane- or
	// storage-affecting rows.
	FlagNow bool
	// CanaryEvery is the visible-only secondary escalation-canary cadence
	// (OQ4(a), ⚙ verification.canary_interval_hours); 0 when the row has none.
	CanaryEvery time.Duration
}

// rawRow is a scanned registry row before state resolution (phase 1). Derive
// queries run in phase 2, after the row cursor is closed — the storage pool is
// a single connection, so a nested query while iterating would deadlock.
type rawRow struct {
	id, section, fixtures, triggers, schedule, cadence, canaryCadence, affect, deriveKind, notes string
	lastRun, lastResult                                                                          sql.NullString
}

// List returns every registry row with its resolved state. For a RECORDED row
// (derive_kind ”) the last run/result are the table columns; for a DERIVE row
// they are computed from the existing subsystem events (OQ2/OQ5(c)). Dueness is
// computed from stored state + the ⚙ cadence, so it is restart/suspend-safe.
func (s *Store) List(ctx context.Context) ([]RowState, error) {
	// Phase 1: scan all rows, then close the cursor.
	rows, err := s.db.QueryContext(ctx,
		`SELECT row_id, owning_section, fixtures, trigger_set, schedule, cadence,
		        canary_cadence, affect_class, derive_kind, notes, last_run, last_result
		   FROM conformance_registry ORDER BY row_id`)
	if err != nil {
		return nil, fmt.Errorf("conformance: list rows: %w", err)
	}
	var raw []rawRow
	for rows.Next() {
		var r rawRow
		if err := rows.Scan(&r.id, &r.section, &r.fixtures, &r.triggers, &r.schedule, &r.cadence,
			&r.canaryCadence, &r.affect, &r.deriveKind, &r.notes, &r.lastRun, &r.lastResult); err != nil {
			rows.Close()
			return nil, fmt.Errorf("conformance: scan row: %w", err)
		}
		raw = append(raw, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Phase 2: resolve state (derive queries are safe now the cursor is closed).
	now := s.now()
	out := make([]RowState, 0, len(raw))
	for _, r := range raw {
		st := RowState{
			ID: r.id, OwningSection: r.section, Fixtures: r.fixtures, TriggerSet: r.triggers,
			Schedule: r.schedule, Cadence: r.cadence, AffectClass: r.affect,
			DeriveKind: r.deriveKind, Notes: r.notes,
			FlagNow: r.affect == AffectLane || r.affect == AffectStorage,
		}
		switch r.deriveKind {
		case DeriveNone:
			if r.lastRun.Valid && r.lastRun.String != "" {
				st.LastRun, st.HasRun = parseTS(r.lastRun.String), true
			}
			st.LastResult = r.lastResult.String
		case DeriveRestoreDrill:
			st.LastRun, st.HasRun, st.LastResult, err = s.deriveRestoreDrill(ctx)
		case DeriveDeadMan:
			st.LastRun, st.HasRun, st.LastResult, err = s.deriveDeadMan(ctx)
		}
		if err != nil {
			return nil, err
		}
		due, err := s.due(r.cadence, st.LastRun, st.HasRun, now)
		if err != nil {
			return nil, err
		}
		st.Due = due
		if r.canaryCadence != "" {
			hours, err := s.settings.Int(r.canaryCadence)
			if err != nil {
				return nil, fmt.Errorf("conformance: read ⚙ %s: %w", r.canaryCadence, err)
			}
			st.CanaryEvery = time.Duration(hours) * time.Hour
		}
		out = append(out, st)
	}
	return out, nil
}

// Due returns the subset of List that is due or overdue (a passive read
// surface; nothing fires on dueness — OQ3).
func (s *Store) Due(ctx context.Context) ([]RowState, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RowState, 0)
	for _, st := range all {
		if st.Due {
			out = append(out, st)
		}
	}
	return out, nil
}

// due computes dueness from the cadence token + last-run time, reading a
// settings-backed cadence live (never a hardcoded ⚙ value). A never-run row is
// due (its overdue-ness is trivially visible, OQ3).
func (s *Store) due(cadence string, lastRun time.Time, hasRun bool, now time.Time) (bool, error) {
	interval, err := s.interval(cadence)
	if err != nil {
		return false, err
	}
	if !hasRun {
		return true, nil
	}
	return now.Sub(lastRun) >= interval, nil
}

// interval resolves a cadence token to a duration. Structural tokens are OQ6(a)
// constants; a setting:<key>:<unit> token reads the ⚙ value by dotted key at
// evaluation time.
func (s *Store) interval(cadence string) (time.Duration, error) {
	switch cadence {
	case CadenceQuarterly:
		return quarterlyInterval, nil
	case CadenceWeekly:
		return weeklyInterval, nil
	case CadenceDaily:
		return dailyInterval, nil
	}
	rest, ok := strings.CutPrefix(cadence, cadenceSettingPrefix)
	if !ok {
		return 0, fmt.Errorf("conformance: unknown cadence token %q", cadence)
	}
	key, unit, ok := strings.Cut(rest, ":")
	if !ok {
		return 0, fmt.Errorf("conformance: malformed setting cadence %q", cadence)
	}
	n, err := s.settings.Int(key)
	if err != nil {
		return 0, fmt.Errorf("conformance: read ⚙ %s: %w", key, err)
	}
	switch unit {
	case "hours":
		return time.Duration(n) * time.Hour, nil
	case "days":
		return time.Duration(n) * 24 * time.Hour, nil
	case "months":
		return time.Duration(n) * daysPerMonth * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("conformance: unknown cadence unit %q in %q", unit, cadence)
}

// deriveRestoreDrill resolves the restore-drill row's last run/result from the
// existing platform.restore_drill events (OQ2(a); internal/backup untouched).
// The type string is a literal — no import edge to internal/backup.
func (s *Store) deriveRestoreDrill(ctx context.Context) (time.Time, bool, string, error) {
	var payload, ts string
	err := s.db.QueryRowContext(ctx,
		`SELECT payload, ts FROM run_events WHERE type = 'platform.restore_drill'
		 ORDER BY event_seq DESC LIMIT 1`).Scan(&payload, &ts)
	if err == sql.ErrNoRows {
		return time.Time{}, false, "", nil
	}
	if err != nil {
		return time.Time{}, false, "", fmt.Errorf("conformance: derive restore-drill: %w", err)
	}
	var body struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(payload), &body); err != nil {
		return time.Time{}, false, "", fmt.Errorf("conformance: parse restore-drill payload: %w", err)
	}
	result := ResultRed
	if body.OK {
		result = ResultGreen
	}
	return parseTS(ts), true, result, nil
}

// deriveDeadMan resolves the dead-man row's last run/result from the deadman
// events (OQ5(c); NO import edge to internal/watchdog — the type strings and
// run-id prefix are literals). last run = the latest platform.deadman.* run
// birth; result = red when the latest probe alarmed (a run-less
// watchdog.flagged rule=watchdog.dead_man after that birth), else green (a pass
// emits no event), else unknown when no probe has ever run.
func (s *Store) deriveDeadMan(ctx context.Context) (time.Time, bool, string, error) {
	var birthSeq int64
	var birthTS string
	err := s.db.QueryRowContext(ctx,
		`SELECT event_seq, ts FROM run_events
		 WHERE run_id LIKE 'platform.deadman.%' AND type = 'run.created'
		 ORDER BY event_seq DESC LIMIT 1`).Scan(&birthSeq, &birthTS)
	if err == sql.ErrNoRows {
		return time.Time{}, false, "", nil
	}
	if err != nil {
		return time.Time{}, false, "", fmt.Errorf("conformance: derive dead-man birth: %w", err)
	}
	alarmed, err := s.deadManAlarmedAfter(ctx, birthSeq)
	if err != nil {
		return time.Time{}, false, "", err
	}
	result := ResultGreen
	if alarmed {
		result = ResultRed
	}
	return parseTS(birthTS), true, result, nil
}

// deadManAlarmedAfter reports whether the latest dead-man probe raised its
// alarm — a run-less watchdog.flagged with rule watchdog.dead_man appended after
// the latest canary birth (the flag rides the same probe pass, so its seq is
// above the birth seq). Bounded to post-birth run-less flags.
func (s *Store) deadManAlarmedAfter(ctx context.Context, birthSeq int64) (bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT payload FROM run_events
		 WHERE run_id IS NULL AND type = 'watchdog.flagged' AND event_seq > ?`, birthSeq)
	if err != nil {
		return false, fmt.Errorf("conformance: derive dead-man flag: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return false, fmt.Errorf("conformance: scan dead-man flag: %w", err)
		}
		var body struct {
			Rule string `json:"rule"`
		}
		if err := json.Unmarshal([]byte(payload), &body); err != nil {
			continue // a non-conformant flag payload never trips the derivation
		}
		if body.Rule == ruleDeadMan {
			return true, nil
		}
	}
	return false, rows.Err()
}

// ruleDeadMan is the watchdog dead-man flag rule string (internal/watchdog
// RuleDeadMan) — a literal here, never an import (OQ5(c) NO import edge).
const ruleDeadMan = "watchdog.dead_man"

// parseTS parses a stored RFC3339Nano timestamp, returning the zero time on a
// malformed value (ordering is by event_seq, never timestamps — S02.5/P-T07-4).
func parseTS(ts string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}
	}
	return t
}

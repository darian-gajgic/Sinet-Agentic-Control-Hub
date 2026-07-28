package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// read.go — the bulk exported reads the S15.9 settings surface serves from
// (P3-B6-3A). index.go is byte-unchanged; these are new READS in a sibling
// file.
//
// Everything the surface renders is computed HERE, in the registry that owns
// the declarations, the overrides, the effective-bounds rule and the audit
// table: `internal/api` narrows what it serves by owner scope and computes
// nothing of its own. A second implementation of "what is the effective value"
// or "what are the effective bounds" is exactly the drift S01.10 exists to
// prevent.

// Value is one declared setting as the settings surface reads it: the
// declaration's own metadata, the effective value at platform scope, the
// per-user overrides in force, and the EFFECTIVE clamp bounds (the operator's
// override where set, else the ratified clamp — G1 rider 1), so the UI can
// "show each setting's clamp bounds" (S15.9) without re-deriving them.
type Value struct {
	Key      string `json:"key"`
	Section  string `json:"section"`
	Domain   string `json:"domain"`
	Title    string `json:"title"`
	Help     string `json:"help"`
	Type     string `json:"type"`
	Unit     string `json:"unit,omitempty"`
	Ratified string `json:"ratified_by"`
	// Dormant is the S18.5 dormant-at-v0 note; empty means live.
	Dormant string `json:"dormant,omitempty"`

	Default any `json:"default"`
	// Effective is the value in force at PLATFORM scope: the platform override
	// where one is stored, else the declared default.
	Effective any `json:"effective"`
	// Overridden reports whether a platform override row is in force, so
	// "equals the default" and "is explicitly set to the default value" stay
	// distinguishable — reset-to-default deletes the row, and a surface that
	// could not tell them apart could not show what reset would do.
	Overridden bool `json:"overridden"`
	// UserValues are the per-user overrides in force, by user id. Nil for a key
	// that is not per-user scoped. The api layer narrows this by owner scope.
	UserValues map[string]any `json:"user_values,omitempty"`

	// Floor/Ceiling are the EFFECTIVE bounds; nil is unbounded. Numeric says
	// whether the type carries bounds at all, so an absent bound reads as
	// "unbounded" rather than "not applicable".
	Numeric bool     `json:"numeric"`
	Floor   *float64 `json:"floor,omitempty"`
	Ceiling *float64 `json:"ceiling,omitempty"`
	// MinExclusive mirrors the declared exclusive-floor flag (S18.2).
	MinExclusive bool `json:"min_exclusive,omitempty"`

	Enum []string `json:"enum,omitempty"`
	// Auto: automation may move this value within bounds (G1 rider 1).
	Auto bool `json:"auto"`
	// PerUser: the key admits per-user overrides (S18 row qualifier).
	PerUser bool `json:"per_user"`
	// Restart: the change takes effect at the next restart (S01.10 flag).
	Restart bool `json:"restart_required"`
}

// Values returns every declaration in index order with its effective value,
// per-user overrides and effective bounds.
func (r *Registry) Values() []Value {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Value, 0, len(r.order))
	for _, key := range r.order {
		out = append(out, r.valueLocked(r.decls[key]))
	}
	return out
}

// valueLocked builds one Value. r.mu must be held for reading.
func (r *Registry) valueLocked(d *Decl) Value {
	v := Value{
		Key:          d.Key,
		Section:      d.Section,
		Domain:       d.Domain(),
		Title:        d.Title,
		Help:         d.Help,
		Type:         d.Type.String(),
		Unit:         d.Unit,
		Ratified:     d.Ratified,
		Dormant:      d.Dormant,
		Default:      d.Default,
		Effective:    r.effective(d, ""),
		Numeric:      d.Type.numeric(),
		MinExclusive: d.MinExclusive,
		Enum:         d.Enum,
		Auto:         d.Auto,
		PerUser:      d.PerUser,
		Restart:      d.Restart,
	}
	_, v.Overridden = r.vals[d.Key]
	if d.Type.numeric() {
		f, c := r.effectiveBoundsLocked(d)
		v.Floor, v.Ceiling = cloneF(f), cloneF(c)
	}
	if uv := r.userVals[d.Key]; len(uv) > 0 {
		v.UserValues = make(map[string]any, len(uv))
		for user, val := range uv {
			v.UserValues[user] = val
		}
	}
	return v
}

// AuditEntry is one settings_events row (Spec S01.10 {actor, key, old, new,
// timestamp, reason}) — the per-setting audit history S15.9 renders.
//
// Old and New are the stored canonical JSON verbatim: settings values are
// CONTENT, not an observability projection, and nothing rewrites them on the
// way out.
type AuditEntry struct {
	ID     int64           `json:"id"`
	Actor  string          `json:"actor"`
	Key    string          `json:"key"`
	UserID string          `json:"user_id,omitempty"`
	Old    json.RawMessage `json:"old"`
	New    json.RawMessage `json:"new"`
	TS     string          `json:"ts"`
	Reason string          `json:"reason,omitempty"`
}

// History returns a key's audit rows, newest first, bounded by limit.
//
// Ordering is by the append-only table's own primary key, not by `ts`:
// timestamps are recorded, never an ordering authority (§6; P-T07-4). The key
// index (0001 `settings_events_key_idx`) serves the filter.
func (r *Registry) History(ctx context.Context, key string, limit int) ([]AuditEntry, error) {
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return nil, ErrNotAttached
	}
	if _, err := r.decl(key); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("settings: history limit must be positive, got %d", limit)
	}
	rows, err := db.QueryContext(ctx,
		`SELECT settings_event_id, actor, key, user_id, old, new, ts, reason
		   FROM settings_events WHERE key = ?
		  ORDER BY settings_event_id DESC LIMIT ?`, key, limit)
	if err != nil {
		return nil, fmt.Errorf("settings: read audit history for %s: %w", key, err)
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var (
			e        AuditEntry
			old, new sql.NullString
		)
		if err := rows.Scan(&e.ID, &e.Actor, &e.Key, &e.UserID, &old, &new, &e.TS, &e.Reason); err != nil {
			return nil, fmt.Errorf("settings: scan audit row: %w", err)
		}
		if old.Valid {
			e.Old = json.RawMessage(old.String)
		}
		if new.Valid {
			e.New = json.RawMessage(new.String)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("settings: read audit history for %s: %w", key, err)
	}
	return out, nil
}

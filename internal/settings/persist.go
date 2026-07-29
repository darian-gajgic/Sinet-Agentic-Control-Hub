package settings

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

// Attach connects the registry to platform.db and the event log and loads
// the override rows (Spec S01.10 storage hybrid). It is strict: rows for
// undeclared keys or with values that fail validation are configuration
// drift and fail the attach with a named error.
func (r *Registry) Attach(ctx context.Context, db *storage.DB, log *eventlog.Log) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	r.mu.RLock()
	attached := r.db != nil
	r.mu.RUnlock()
	if attached {
		return errors.New("settings: already attached")
	}
	rows, err := db.QueryContext(ctx, `SELECT key, user_id, value, floor, ceiling FROM settings`)
	if err != nil {
		return fmt.Errorf("settings: load overrides: %w", err)
	}
	defer rows.Close()

	vals := make(map[string]any)
	userVals := make(map[string]map[string]any)
	bounds := make(map[string]boundsOverride)
	for rows.Next() {
		var (
			key, userID    string
			value          sql.NullString
			floor, ceiling sql.NullFloat64
		)
		if err := rows.Scan(&key, &userID, &value, &floor, &ceiling); err != nil {
			return fmt.Errorf("settings: scan override: %w", err)
		}
		d, ok := r.decls[key]
		if !ok {
			return fmt.Errorf("settings: override row for undeclared key %q (declare-once drift)", key)
		}
		var ov *boundsOverride
		if floor.Valid || ceiling.Valid {
			if userID != "" {
				return fmt.Errorf("settings: per-user bounds on %q are not admissible", key)
			}
			if !d.Type.numeric() {
				return fmt.Errorf("settings: bounds stored for non-numeric key %q", key)
			}
			b := boundsOverride{}
			if floor.Valid {
				f := floor.Float64
				b.floor = &f
			}
			if ceiling.Valid {
				c := ceiling.Float64
				b.ceiling = &c
			}
			bounds[key] = b
			ov = &b
		}
		if value.Valid {
			if userID != "" && !d.PerUser {
				return fmt.Errorf("settings: per-user override for non-per-user key %q", key)
			}
			v, err := decodeValue(d, json.RawMessage(value.String))
			if err != nil {
				return fmt.Errorf("settings: stored override for %s: %w", key, err)
			}
			if n, isNum := numericOf(d, v); isNum {
				if err := checkClamp(d, ov, n); err != nil {
					return fmt.Errorf("settings: stored override for %s: %w", key, err)
				}
			}
			if userID == "" {
				vals[key] = v
			} else {
				if userVals[key] == nil {
					userVals[key] = make(map[string]any)
				}
				userVals[key][userID] = v
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("settings: load overrides: %w", err)
	}
	r.mu.Lock()
	r.vals, r.userVals, r.bounds = vals, userVals, bounds
	r.db, r.log = db, log
	r.mu.Unlock()
	return nil
}

// SetRequest is one value write. A nil Value is reset-to-default (the
// override row is deleted, Spec S01.10). ForUser targets a per-user
// override of a per-user-scoped key.
type SetRequest struct {
	Key     string
	ForUser string
	Value   json.RawMessage
	Actor   Actor
	Reason  string
}

// Set validates and applies one value write with its audit trail, in one
// transaction (Spec S01.10; G1 rider 1).
func (r *Registry) Set(ctx context.Context, req SetRequest) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	r.mu.RLock()
	attached := r.db != nil
	r.mu.RUnlock()
	if !attached {
		return ErrNotAttached
	}
	d, err := r.decl(req.Key)
	if err != nil {
		return err
	}
	if err := req.Actor.validate(); err != nil {
		return err
	}
	if req.ForUser != "" && !d.PerUser {
		return invalid("settings: %s is not per-user scoped", d.Key)
	}
	reset := req.Value == nil
	if req.Actor.Kind == ActorAutomation {
		switch {
		case !d.Auto:
			return fmt.Errorf("%w: %s is not auto-adjustable", ErrAutomation, d.Key)
		case reset:
			return fmt.Errorf("%w: automation moves values, it does not reset", ErrAutomation)
		case req.ForUser != "":
			return fmt.Errorf("%w: automation writes are platform-scope", ErrAutomation)
		}
	}

	var newVal any
	if reset {
		newVal = d.Default
	} else {
		newVal, err = decodeValue(d, req.Value)
		if err != nil {
			return err
		}
	}
	r.mu.RLock()
	var clampErr error
	if n, isNum := numericOf(d, newVal); isNum {
		clampErr = checkClamp(d, r.boundsPtr(d.Key), n)
	}
	var rulesErr error
	if clampErr == nil {
		overlay := map[string]any{}
		if req.ForUser == "" {
			overlay[d.Key] = newVal
		}
		rulesErr = r.checkRules(overlay)
	}
	oldVal := r.effective(d, req.ForUser)
	r.mu.RUnlock()
	if clampErr != nil {
		return fmt.Errorf("settings: %s: %w", d.Key, clampErr)
	}
	if rulesErr != nil {
		return rulesErr
	}
	change := auditChange{
		key:     d.Key,
		forUser: req.ForUser,
		kind:    "value",
		old:     canonicalJSON(oldVal),
		new:     canonicalJSON(newVal),
		actor:   req.Actor,
		reason:  req.Reason,
	}
	err = r.persist(ctx, change, func(ctx context.Context, tx *sql.Tx) error {
		if reset {
			return r.deleteValueCell(ctx, tx, d.Key, req.ForUser)
		}
		return r.upsertValueCell(ctx, tx, d.Key, req.ForUser, string(canonicalJSON(newVal)), req.Actor)
	})
	if err != nil {
		return err
	}

	// Commit to memory.
	r.mu.Lock()
	defer r.mu.Unlock()
	if req.ForUser != "" {
		if reset {
			if uv := r.userVals[d.Key]; uv != nil {
				delete(uv, req.ForUser)
				if len(uv) == 0 {
					delete(r.userVals, d.Key)
				}
			}
		} else {
			if r.userVals[d.Key] == nil {
				r.userVals[d.Key] = make(map[string]any)
			}
			r.userVals[d.Key][req.ForUser] = newVal
		}
	} else if reset {
		delete(r.vals, d.Key)
	} else {
		r.vals[d.Key] = newVal
	}
	return nil
}

// SetBoundsRequest is one operator edit of a numeric key's (floor,
// ceiling). Nil Floor and Ceiling together reset the bounds to the declared
// clamp.
type SetBoundsRequest struct {
	Key            string
	Floor, Ceiling *float64
	Actor          Actor
	Reason         string
}

// SetBounds applies an operator bounds edit with its audit trail (G1 rider
// 1: only the operator edits bounds; automation then moves values within
// them).
func (r *Registry) SetBounds(ctx context.Context, req SetBoundsRequest) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	r.mu.RLock()
	attached := r.db != nil
	r.mu.RUnlock()
	if !attached {
		return ErrNotAttached
	}
	d, err := r.decl(req.Key)
	if err != nil {
		return err
	}
	if err := req.Actor.validate(); err != nil {
		return err
	}
	if req.Actor.Kind != ActorOperator {
		return fmt.Errorf("%w: only the operator edits bounds", ErrAutomation)
	}
	if !d.Type.numeric() {
		return invalid("settings: %s is %v and carries no bounds", d.Key, d.Type)
	}
	reset := req.Floor == nil && req.Ceiling == nil
	if req.Floor != nil && req.Ceiling != nil && *req.Floor > *req.Ceiling {
		return invalid("settings: %s: floor %v > ceiling %v", d.Key, *req.Floor, *req.Ceiling)
	}

	// Every current effective value must remain inside the new bounds.
	var next *boundsOverride
	if !reset {
		next = &boundsOverride{floor: cloneF(req.Floor), ceiling: cloneF(req.Ceiling)}
	}
	checkVal := func(scope string, v any) error {
		n, _ := numericOf(d, v)
		if err := checkClamp(d, next, n); err != nil {
			return fmt.Errorf("settings: %s: current %s value %v would fall outside the new bounds: %w", d.Key, scope, n, err)
		}
		return nil
	}
	r.mu.RLock()
	scopeErr := checkVal("platform", r.effective(d, ""))
	if scopeErr == nil {
		for user, v := range r.userVals[d.Key] {
			if scopeErr = checkVal("user "+user, v); scopeErr != nil {
				break
			}
		}
	}
	oldF, oldC := r.effectiveBoundsLocked(d)
	r.mu.RUnlock()
	if scopeErr != nil {
		return scopeErr
	}
	change := auditChange{
		key:    d.Key,
		kind:   "bounds",
		old:    boundsJSON(oldF, oldC),
		actor:  req.Actor,
		reason: req.Reason,
	}
	if reset {
		change.new = boundsJSON(d.Min, d.Max)
	} else {
		nf, nc := d.Min, d.Max
		if req.Floor != nil {
			nf = req.Floor
		}
		if req.Ceiling != nil {
			nc = req.Ceiling
		}
		change.new = boundsJSON(nf, nc)
	}
	err = r.persist(ctx, change, func(ctx context.Context, tx *sql.Tx) error {
		if reset {
			return r.deleteBoundsCells(ctx, tx, d.Key)
		}
		return r.upsertBoundsCells(ctx, tx, d.Key, req.Floor, req.Ceiling, req.Actor)
	})
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if reset {
		delete(r.bounds, d.Key)
	} else {
		r.bounds[d.Key] = *next
	}
	return nil
}

func (r *Registry) boundsPtr(key string) *boundsOverride {
	if b, ok := r.bounds[key]; ok {
		return &b
	}
	return nil
}

func (r *Registry) effectiveBoundsLocked(d *Decl) (*float64, *float64) {
	f, c := d.Min, d.Max
	if ov, ok := r.bounds[d.Key]; ok {
		if ov.floor != nil {
			f = ov.floor
		}
		if ov.ceiling != nil {
			c = ov.ceiling
		}
	}
	return f, c
}

func boundsJSON(floor, ceiling *float64) json.RawMessage {
	return canonicalJSON(map[string]any{"floor": floor, "ceiling": ceiling})
}

// auditChange is one write's audit record: a settings_events row AND a
// settings.changed event on the main log, atomic with the row change (Spec
// S01.10; G3 Def.2).
type auditChange struct {
	key     string
	forUser string
	kind    string // "value" | "bounds"
	old     json.RawMessage
	new     json.RawMessage
	actor   Actor
	reason  string
}

func (r *Registry) persist(ctx context.Context, ch auditChange, apply func(ctx context.Context, tx *sql.Tx) error) error {
	return r.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := apply(ctx, tx); err != nil {
			return err
		}
		now := r.now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO settings_events (actor, key, user_id, old, new, ts, reason)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ch.actor.String(), ch.key, ch.forUser, string(ch.old), string(ch.new), now, ch.reason); err != nil {
			return fmt.Errorf("settings: audit row: %w", err)
		}
		payload := canonicalJSON(map[string]any{
			"key":    ch.key,
			"scope":  scopeName(ch.forUser),
			"user":   ch.forUser,
			"change": ch.kind,
			"old":    ch.old,
			"new":    ch.new,
			"actor":  map[string]string{"kind": string(ch.actor.Kind), "id": ch.actor.ID},
			"reason": ch.reason,
		})
		_, err := r.log.AppendTx(ctx, tx, eventlog.Append{
			UserID:        ch.actor.ID,
			Type:          EventSettingsChanged,
			SchemaVersion: settingsEventSchemaVersion,
			Payload:       payload,
		})
		if err != nil {
			return fmt.Errorf("settings: emit %s: %w", EventSettingsChanged, err)
		}
		return nil
	})
}

func scopeName(forUser string) string {
	if forUser == "" {
		return "platform"
	}
	return "user"
}

func (r *Registry) upsertValueCell(ctx context.Context, tx *sql.Tx, key, userID, value string, actor Actor) error {
	now := r.now().UTC().Format(time.RFC3339Nano)
	_, err := tx.ExecContext(ctx,
		`INSERT INTO settings (key, user_id, value, floor, ceiling, updated_ts, updated_by)
		 VALUES (?, ?, ?, NULL, NULL, ?, ?)
		 ON CONFLICT (key, user_id) DO UPDATE SET
		   value = excluded.value, updated_ts = excluded.updated_ts, updated_by = excluded.updated_by`,
		key, userID, value, now, actor.String())
	if err != nil {
		return fmt.Errorf("settings: upsert override: %w", err)
	}
	return nil
}

func (r *Registry) deleteValueCell(ctx context.Context, tx *sql.Tx, key, userID string) error {
	// Reset-to-default deletes the row unless it still carries bounds
	// (platform scope only; per-user rows never carry bounds).
	if _, err := tx.ExecContext(ctx,
		`UPDATE settings SET value = NULL WHERE key = ? AND user_id = ?
		   AND (floor IS NOT NULL OR ceiling IS NOT NULL)`, key, userID); err != nil {
		return fmt.Errorf("settings: clear override: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM settings WHERE key = ? AND user_id = ?
		   AND floor IS NULL AND ceiling IS NULL`, key, userID); err != nil {
		return fmt.Errorf("settings: delete override: %w", err)
	}
	return nil
}

func (r *Registry) upsertBoundsCells(ctx context.Context, tx *sql.Tx, key string, floor, ceiling *float64, actor Actor) error {
	now := r.now().UTC().Format(time.RFC3339Nano)
	var f, c any
	if floor != nil {
		f = *floor
	}
	if ceiling != nil {
		c = *ceiling
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO settings (key, user_id, value, floor, ceiling, updated_ts, updated_by)
		 VALUES (?, '', NULL, ?, ?, ?, ?)
		 ON CONFLICT (key, user_id) DO UPDATE SET
		   floor = excluded.floor, ceiling = excluded.ceiling,
		   updated_ts = excluded.updated_ts, updated_by = excluded.updated_by`,
		key, f, c, now, actor.String())
	if err != nil {
		return fmt.Errorf("settings: upsert bounds: %w", err)
	}
	return nil
}

func (r *Registry) deleteBoundsCells(ctx context.Context, tx *sql.Tx, key string) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE settings SET floor = NULL, ceiling = NULL
		 WHERE key = ? AND user_id = '' AND value IS NOT NULL`, key); err != nil {
		return fmt.Errorf("settings: clear bounds: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM settings WHERE key = ? AND user_id = '' AND value IS NULL`, key); err != nil {
		return fmt.Errorf("settings: delete bounds row: %w", err)
	}
	return nil
}

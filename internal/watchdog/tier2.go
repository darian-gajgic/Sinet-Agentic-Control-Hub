package watchdog

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// tier2.go — the S14.4 Tier-2 pause-and-flag routing (brief R15–R22). ALWAYS on
// a Tier-0 trigger the run parks at its checkpoint boundary via the S02.3 FSM
// and watchdog.flagged is appended in the SAME WriteTx (§8 transition
// discipline; the recovery-ladder WEDGED precedent). Containment is STRUCTURAL
// — the scheduler admits no next paid call for a parked run (Spec S10.8).
//
// NO auto-kill anywhere: park is the only run transition this file makes, and
// only along the ratified running/draining → parked edge. A check that killed,
// terminated, tombstoned, or gated would be a defect (R16; importwall_test.go
// proves this package invokes no such primitive).

// flag is the run-scoped Tier-2 entry (R15): dedup by (run, anomaly class),
// Tier-1 annotate (behavioral rules only — the verdict never gates), then
// park-and-flag in one WriteTx.
func (w *Watchdog) flag(ctx context.Context, r run.Run, trig trigger) error {
	open, err := w.openFlagExists(ctx, r.ID, trig.Rule)
	if err != nil {
		return err
	}
	if open {
		// One alert per (run, class); later evidence folds into the open alert
		// (R22; the S10.5 once-per-storm discipline).
		w.logger.Debug("watchdog: flag folded into the open alert (dedup)", "run", r.ID, "class", trig.Rule)
		return nil
	}

	var annRef *int64
	if annotatable(trig.Rule) {
		events, rerr := w.readRecentEvents(ctx, r.ID, r.Generation, recentWindow)
		if rerr != nil {
			w.logger.Warn("watchdog: read events for annotation", "run", r.ID, "err", rerr)
		} else {
			annRef = w.annotate(ctx, r, trig, events)
		}
	}
	return w.parkAndFlag(ctx, r, trig, annRef)
}

// parkAndFlag parks the run (if it is on the ratified running/draining → parked
// edge) and appends watchdog.flagged in the SAME WriteTx (R15/§8). A terminal
// run (suspicious-completion) or an already-parked run is not re-parked —
// containment is already structural (a terminal run makes no further calls; a
// parked run admits none) — and the flag stands alone.
func (w *Watchdog) parkAndFlag(ctx context.Context, r run.Run, trig trigger, annRef *int64) error {
	return w.db.WriteTx(ctx, func(tx *sql.Tx) error {
		cur, err := w.runs.GetTx(ctx, tx, r.ID)
		if err != nil {
			return err
		}
		gen := cur.Generation
		parked := false
		if run.CanTransition(cur.State, run.StateParked) {
			updated, err := w.runs.TransitionTx(ctx, tx, r.ID, run.StateParked, run.TransitionOptions{
				Reason: "watchdog:" + trig.Rule,
				Actor:  run.ActorPlatform,
			})
			if err != nil {
				return err
			}
			gen = updated.Generation // park does not bump generation (only resume does)
			parked = true
		}
		payload, err := json.Marshal(FlagPayload{
			Rule:            trig.Rule,
			AnomalyClass:    trig.Rule,
			Severity:        severityFor(trig.Rule),
			Signature:       trig.Signature,
			Counts:          trig.Counts,
			SpendVsBaseline: trig.Spend,
			AnnotationRef:   annRef,
			Detail:          trig.Detail,
			Parked:          parked,
			Actor:           run.ActorPlatform,
		})
		if err != nil {
			return err
		}
		_, err = w.log.AppendTx(ctx, tx, eventlog.Append{
			RunID:         r.ID,
			Generation:    gen,
			UserID:        cur.UserID,
			Type:          EventFlagged,
			SchemaVersion: eventSchemaVersion,
			Payload:       payload,
		})
		return err
	})
}

// flagPlatform emits a run-less, platform-scope watchdog.flagged (the registered
// checks R25–R27, the dead-man alarm R28-DMC, the meta-alert R23). Platform
// scope: user_id=platform, run_id NULL, no generation (R34). No park (no run) —
// the alert IS the containment surface for a platform condition. Deduped by
// (platform, class = the rule).
func (w *Watchdog) flagPlatform(ctx context.Context, trig trigger) error {
	return w.ownerlessFlag(ctx, run.ActorPlatform, trig, trig.Rule)
}

// ownerlessFlag emits a run-less watchdog.flagged (run_id NULL, no generation),
// attributed to userID with an explicit dedup class. Platform-health flags
// carry user_id=platform (R34); the SPEND rule (R6) is per-PERSON, not per-run
// and not a platform condition, so it carries user_id=owner (the eventlog
// permits an owner-attributed run-less row — validate keys owner-attribution on
// user_id, and the S01.9 owner-scoped inbox shows it to that member). No park —
// a per-person or platform condition contains no single run.
func (w *Watchdog) ownerlessFlag(ctx context.Context, userID string, trig trigger, class string) error {
	open, err := w.openOwnerlessFlagExists(ctx, class)
	if err != nil {
		return err
	}
	if open {
		w.logger.Debug("watchdog: run-less flag folded into the open alert (dedup)", "class", class)
		return nil
	}
	payload, err := json.Marshal(FlagPayload{
		Rule:            trig.Rule,
		AnomalyClass:    class,
		Severity:        severityFor(trig.Rule),
		Signature:       trig.Signature,
		Counts:          trig.Counts,
		SpendVsBaseline: trig.Spend,
		Detail:          trig.Detail,
		Parked:          false,
		Actor:           run.ActorPlatform,
	})
	if err != nil {
		return err
	}
	_, err = w.log.Append(ctx, eventlog.Append{
		UserID:        userID,
		Type:          EventFlagged,
		SchemaVersion: eventSchemaVersion,
		Payload:       payload,
	})
	return err
}

// ── derive-from-log dedup (OQ2 — no state table; open state is derived) ──

// openFlagExists reports whether a run has an OPEN watchdog flag for the class:
// the latest watchdog.flagged for (run, class) is not superseded by a later
// watchdog.suppressed for the rule OR a resume (run.state_changed to=running —
// the "resume, I was wrong" action, R18/R30). Derived from run_events (S14.1).
func (w *Watchdog) openFlagExists(ctx context.Context, runID, class string) (bool, error) {
	flagSeq, found, err := w.latestFlagSeq(ctx, runID, class)
	if err != nil || !found {
		return false, err
	}
	superseded, err := w.supersededAfter(ctx, runID, class, flagSeq)
	if err != nil {
		return false, err
	}
	return !superseded, nil
}

// latestFlagSeq returns the seq of the newest watchdog.flagged for (run, class).
func (w *Watchdog) latestFlagSeq(ctx context.Context, runID, class string) (int64, bool, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT event_seq, payload FROM run_events
		 WHERE run_id = ? AND type = ? ORDER BY event_seq DESC`, runID, EventFlagged)
	if err != nil {
		return 0, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var seq int64
		var payload string
		if err := rows.Scan(&seq, &payload); err != nil {
			return 0, false, err
		}
		if firstString(json.RawMessage(payload), "anomaly_class", "rule") == class {
			return seq, true, rows.Err()
		}
	}
	return 0, false, rows.Err()
}

// supersededAfter reports whether a later watchdog.suppressed for the rule, or a
// resume transition (to=running), exists after afterSeq for the run.
func (w *Watchdog) supersededAfter(ctx context.Context, runID, class string, afterSeq int64) (bool, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT type, payload FROM run_events
		 WHERE run_id = ? AND event_seq > ?
		   AND type IN ('watchdog.suppressed', 'run.state_changed', 'run.state')`,
		runID, afterSeq)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var typ, payload string
		if err := rows.Scan(&typ, &payload); err != nil {
			return false, err
		}
		p := json.RawMessage(payload)
		switch typ {
		case EventSuppressed:
			if firstString(p, "rule") == class {
				return true, rows.Err()
			}
		default: // a run.state_changed
			if firstString(p, "to") == string(run.StateRunning) {
				return true, rows.Err() // resume — the flag is cleared (R18/R30)
			}
		}
	}
	return false, rows.Err()
}

// openOwnerlessFlagExists is openFlagExists for a run-less flag (platform or
// per-person): the latest run-less watchdog.flagged for the class is not
// superseded by a later run-less watchdog.suppressed for the rule. The class is
// unique per condition (e.g. "spend:<owner>"), so no user_id filter is needed.
func (w *Watchdog) openOwnerlessFlagExists(ctx context.Context, class string) (bool, error) {
	var flagSeq int64
	found := false
	rows, err := w.db.QueryContext(ctx,
		`SELECT event_seq, payload FROM run_events
		 WHERE run_id IS NULL AND type = ? ORDER BY event_seq DESC`, EventFlagged)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var seq int64
		var payload string
		if err := rows.Scan(&seq, &payload); err != nil {
			rows.Close()
			return false, err
		}
		if firstString(json.RawMessage(payload), "anomaly_class", "rule") == class {
			flagSeq, found = seq, true
			break
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	// A later platform-scope suppress for this rule clears the platform flag.
	srows, err := w.db.QueryContext(ctx,
		`SELECT payload FROM run_events
		 WHERE run_id IS NULL AND type = ? AND event_seq > ?`,
		EventSuppressed, flagSeq)
	if err != nil {
		return false, err
	}
	defer srows.Close()
	for srows.Next() {
		var payload string
		if err := srows.Scan(&payload); err != nil {
			return false, err
		}
		if firstString(json.RawMessage(payload), "rule") == class {
			return false, srows.Err() // superseded by a suppress
		}
	}
	return true, srows.Err()
}

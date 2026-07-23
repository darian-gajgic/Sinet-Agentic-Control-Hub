package watchdog

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// sweep.go — the time-based half of the suite (brief R5/R6, R23–R27). Sweep is
// driven by the shell-owned goroutine on the structural SweepInterval (R28); it
// runs the absence/spend counters and the four registered platform-health
// checks + the flag-now meta-alert. Each check logs and continues — a failing
// check never aborts the sweep (level-triggered, the recovery-sweep precedent).

// activeStates are the non-terminal FSM states silence/spend evaluate over (R5).
var activeStates = []run.State{
	run.StateNew, run.StateQueued, run.StateClaimed,
	run.StateRunning, run.StateParked, run.StateDraining,
}

// Sweep runs one pass of the time-based signals + registered checks (R28).
func (w *Watchdog) Sweep(ctx context.Context) error {
	var errs []error
	for _, step := range []struct {
		name string
		fn   func(context.Context) error
	}{
		{"silence", w.sweepSilence},
		{"spend", w.sweepSpend},
		{"resume-reconcile", w.checkResumeReconcile},
		{"listener-audit", w.checkListeners},
		{"organ-absence", w.checkOrgans},
		{"event-log-size", w.checkEventLogSize},
		{"flag-now-rate", w.checkFlagNowRate},
	} {
		if err := step.fn(ctx); err != nil {
			w.logger.Warn("watchdog: sweep step", "step", step.name, "err", err)
			errs = append(errs, fmt.Errorf("%s: %w", step.name, err))
		}
	}
	return errors.Join(errs...)
}

// sweepSilence flags an active run that has emitted no event past its run-type's
// budget (R5). The budget is ⚙ watchdog.silence_budget.<run_type>; an empty
// entry (the v0 default — the whole map ships empty) falls back to the
// recovery.dead_after seed (5 min) until the TBD-BRINGUP per-run-type
// calibration. A run that is LEGITIMATELY WAITING is excluded: a limit-park with
// an explicit resume-time (the named guard) OR a park on an open ask
// (waiting-on-human — the same "legitimately waiting" principle, the projection
// models it as a first-class healthy state). Absence is time-based, so it reads
// the newest event's wall-clock ts (never an ordering authority — display only).
func (w *Watchdog) sweepSilence(ctx context.Context) error {
	active, err := w.runs.InStates(ctx, activeStates...)
	if err != nil {
		return err
	}
	budgets, err := w.settings.StringMap(keySilenceBudget)
	if err != nil {
		return err
	}
	seed, err := w.settings.Duration(keyDeadAfter)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, r := range active {
		if r.State == run.StateParked && w.legitimatelyWaiting(ctx, r) {
			continue
		}
		newest, ok, err := w.newestEventTS(ctx, r.ID)
		if err != nil {
			return err
		}
		if !ok || newest.IsZero() {
			continue // no timestamped activity to measure absence against
		}
		budget := seed
		if v, ok := budgets[runType(r)]; ok {
			if secs, perr := strconv.ParseInt(strings.TrimSpace(v), 10, 64); perr == nil && secs > 0 {
				budget = time.Duration(secs) * time.Second
			}
		}
		silent := now.Sub(newest)
		if silent > budget {
			if err := w.flag(ctx, r, trigger{
				Rule:   RuleSilence,
				Counts: map[string]int{"silent_seconds": int(silent.Seconds()), "budget_seconds": int(budget.Seconds())},
				Detail: fmt.Sprintf("no event for %s (run-type %q budget %s)", silent.Round(time.Second), runType(r), budget),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// sweepSpend flags a person whose daily priced total exceeds ⚙
// watchdog.spend_median_mult × the trailing-median, armed only after ⚙
// watchdog.spend_arm_days of history (R6). DORMANT at v0: the price table ships
// EMPTY (§11) so every row is UNPRICED (AnyUnpriced=true) and the rule stays at
// its cold-start floor (ceilings only). The machinery is built + tested; it
// activates when a price table is seeded. The flag is per-PERSON and run-less
// (a spend spike contains no single run — never a park), owner-attributed for
// the S01.9 inbox.
func (w *Watchdog) sweepSpend(ctx context.Context) error {
	if w.spend == nil {
		return nil // dormant: no metering reader wired
	}
	mult, err := w.settings.Float(keySpendMedianMult)
	if err != nil {
		return err
	}
	armDays, err := w.settings.Int(keySpendArmDays)
	if err != nil {
		return err
	}
	owners, err := w.distinctActiveOwners(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, owner := range owners {
		if owner == run.ActorPlatform {
			continue
		}
		win, err := w.spend.SpendWindow(ctx, owner, now, int(armDays))
		if err != nil {
			return err
		}
		armed := !win.AnyUnpriced && win.DaysOfHistory >= int(armDays)
		if !armed || win.MedianUSD <= 0 || win.TodayUSD <= mult*win.MedianUSD {
			continue // cold-start floor / dormant / within baseline
		}
		ev := &SpendEvidence{
			TodayUSD: win.TodayUSD, MedianUSD: win.MedianUSD, Multiplier: mult,
			DaysOfHistory: win.DaysOfHistory, Armed: armed, Dormant: win.AnyUnpriced,
		}
		if err := w.ownerlessFlag(ctx, owner, trigger{
			Rule:   RuleSpend,
			Spend:  ev,
			Detail: fmt.Sprintf("daily spend $%.2f > %.1f× the %d-day median $%.2f", win.TodayUSD, mult, win.DaysOfHistory, win.MedianUSD),
		}, RuleSpend+":"+owner); err != nil {
			return err
		}
	}
	return nil
}

// ── the four registered platform-health checks (R24–R27) ──
// Each references the S14.5 conformance registry via the seam below; B5-3 does
// NOT build that table — TODO(S14.5/B5-4): register these check ids into the
// conformance_registry when it lands (the B5-2 NoEngineSSEReplay precedent).

// checkResumeReconcile confirms the S01.7 wake steps completed (R24). Honestly
// absent when no wake-completion seam is wired (the logind sleep/wake wiring is
// a later packet — there is nothing to confirm yet).
func (w *Watchdog) checkResumeReconcile(ctx context.Context) error {
	if w.wake == nil {
		return nil
	}
	st, err := w.wake(ctx)
	if err != nil {
		return err
	}
	if !st.Pending || st.Completed {
		return nil
	}
	return w.flagPlatform(ctx, trigger{
		Rule:   RuleResumeReconcile,
		Counts: map[string]int{"missing_steps": len(st.Missing)},
		Detail: "a wake reconcile did not complete all S01.7 steps: " + strings.Join(st.Missing, ", "),
	})
}

// checkListeners re-runs the S01.6/S01.8 listener-binding audit (R25). A foreign
// (non-loopback, non-allowlisted) listener is a flag-now (High). Honestly absent
// when no audit seam is wired.
func (w *Watchdog) checkListeners(ctx context.Context) error {
	if w.listener == nil {
		return nil
	}
	foreign, err := w.listener(ctx)
	if err != nil {
		return err
	}
	if len(foreign) == 0 {
		return nil
	}
	var addrs []string
	for _, f := range foreign {
		addrs = append(addrs, f.Addr)
	}
	return w.flagPlatform(ctx, trigger{
		Rule:      RuleListener,
		Signature: strings.Join(addrs, ","),
		Counts:    map[string]int{"foreign_listeners": len(foreign)},
		Detail:    "a sinet process listens beyond loopback / the operator allowlist: " + strings.Join(addrs, ", "),
	})
}

// checkOrgans surfaces adopted-organ absence as degraded-mode digest flags
// (R26): a down organ is degraded, not an emergency. Honestly absent when no
// organ-liveness seam is wired (sinet-control tolerates their absence — S01.6).
// Each down organ is a distinct card (dedup per organ) so they never collapse.
func (w *Watchdog) checkOrgans(ctx context.Context) error {
	if w.organs == nil {
		return nil
	}
	statuses, err := w.organs(ctx)
	if err != nil {
		return err
	}
	for _, s := range statuses {
		if s.Up {
			continue
		}
		if err := w.flagPlatformClass(ctx, trigger{
			Rule:   RuleOrganAbsence,
			Detail: fmt.Sprintf("adopted organ %q is down (degraded mode): %s", s.Organ, s.Note),
		}, RuleOrganAbsence+":"+s.Organ); err != nil {
			return err
		}
	}
	return nil
}

// flagPlatformClass emits a platform-scope run-less flag with an explicit dedup
// class (per-organ / per-key admin cards). Thin wrapper over ownerlessFlag.
func (w *Watchdog) flagPlatformClass(ctx context.Context, trig trigger, class string) error {
	return w.ownerlessFlag(ctx, run.ActorPlatform, trig, class)
}

// checkEventLogSize watches WAL + run_events growth (R27): past a structural
// threshold (no ⚙) → a daily-digest flag. Uses the storage WAL seam when wired,
// else reads the run_events payload-byte total directly (a hermetic fallback).
func (w *Watchdog) checkEventLogSize(ctx context.Context) error {
	var (
		bytes int64
		note  string
	)
	if w.wal != nil {
		stat, err := w.wal(ctx)
		if err != nil {
			return err
		}
		bytes = stat.WALBytes + stat.EventsBytes
		note = fmt.Sprintf("wal=%d bytes, run_events=%d bytes (%d rows)", stat.WALBytes, stat.EventsBytes, stat.EventRows)
	} else {
		var rows, payloadBytes int64
		if err := w.db.QueryRowContext(ctx,
			`SELECT COUNT(*), COALESCE(SUM(LENGTH(payload)), 0) FROM run_events`).Scan(&rows, &payloadBytes); err != nil {
			return err
		}
		bytes = payloadBytes
		note = fmt.Sprintf("run_events=%d bytes (%d rows)", payloadBytes, rows)
	}
	if bytes <= eventLogSizeThresholdBytes {
		return nil
	}
	return w.flagPlatform(ctx, trigger{
		Rule:   RuleEventLogSize,
		Counts: map[string]int{"bytes": int(bytes), "threshold_bytes": eventLogSizeThresholdBytes},
		Detail: "the event log has grown past the structural threshold: " + note,
	})
}

// checkFlagNowRate is the self-monitoring meta-alert (R23): a sustained breach
// of ⚙ watchdog.flag_now_target/day raises a DIGEST-tier meta-alert ("your
// watchdog is too chatty"). It is NEVER a flag-now itself (severityFor maps
// watchdog.flag_now_rate to daily-digest).
func (w *Watchdog) checkFlagNowRate(ctx context.Context) error {
	target, err := w.settings.Int(keyFlagNowTarget)
	if err != nil {
		return err
	}
	since := time.Now().Add(-24 * time.Hour)
	n, err := w.flagNowCountSince(ctx, since)
	if err != nil {
		return err
	}
	if int64(n) <= target {
		return nil
	}
	return w.flagPlatform(ctx, trigger{
		Rule:   RuleFlagNowRate,
		Counts: map[string]int{"flag_now_24h": n, "target_per_day": int(target)},
		Detail: fmt.Sprintf("%d flag-now alerts in the last 24h exceeds the target of %d/day — the watchdog may be too chatty (a self-monitoring meta-alert)", n, target),
	})
}

// ── sweep read helpers (derive-from-log) ──

// legitimatelyWaiting reports whether a parked run is waiting by design (R5): a
// park with an explicit resume-time, or a park on an open ask (waiting-on-human)
// — either is a healthy wait, not silence.
func (w *Watchdog) legitimatelyWaiting(ctx context.Context, r run.Run) bool {
	// open ask ⇒ waiting-on-human (the projection's first-class healthy state)
	var openAsks int64
	if err := w.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM asks WHERE run_id = ? AND answered_ts IS NULL`, r.ID).Scan(&openAsks); err == nil && openAsks > 0 {
		return true
	}
	// explicit resume-time on the latest limit/park event
	rows, err := w.db.QueryContext(ctx,
		`SELECT payload FROM run_events
		 WHERE run_id = ? AND type IN ('limit.event', 'engine.rate_limit', 'run.parked', 'run.state_changed', 'run.state')
		 ORDER BY event_seq DESC LIMIT 1`, r.ID)
	if err != nil {
		return false
	}
	defer rows.Close()
	if rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err == nil {
			if firstString([]byte(payload), "parked_until", "parked-until", "resets_at", "resets-at", "reset_at") != "" {
				return true
			}
		}
	}
	return false
}

// newestEventTS returns the wall-clock ts of the run's newest event (time-based
// absence measurement, R5).
func (w *Watchdog) newestEventTS(ctx context.Context, runID string) (time.Time, bool, error) {
	var tsRaw string
	err := w.db.QueryRowContext(ctx,
		`SELECT ts FROM run_events WHERE run_id = ? ORDER BY event_seq DESC LIMIT 1`, runID).Scan(&tsRaw)
	if err != nil {
		return time.Time{}, false, nil //nolint:nilerr — no rows ⇒ nothing to measure
	}
	return parseTS(tsRaw), true, nil
}

// distinctActiveOwners lists the distinct owners of active runs (spend is
// per-person; at v0 spend is dormant so the set is only used to iterate).
func (w *Watchdog) distinctActiveOwners(ctx context.Context) ([]string, error) {
	q := `SELECT DISTINCT user_id FROM runs WHERE state IN (?, ?, ?, ?, ?, ?)`
	args := make([]any, len(activeStates))
	for i, s := range activeStates {
		args[i] = string(s)
	}
	rows, err := w.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// flagNowCountSince counts flag-now watchdog.flagged events since ts (R23),
// EXCLUDING the daily dead-man canary's synthetic flags (platform.deadman.* —
// the self-test must not inflate its own chattiness meta-alert).
func (w *Watchdog) flagNowCountSince(ctx context.Context, since time.Time) (int, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT COALESCE(run_id, ''), payload FROM run_events WHERE type = ? AND ts > ?`,
		EventFlagged, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var runID, payload string
		if err := rows.Scan(&runID, &payload); err != nil {
			return 0, err
		}
		if strings.HasPrefix(runID, canaryPrefix) {
			continue
		}
		if firstString([]byte(payload), "severity") == SeverityFlagNow {
			n++
		}
	}
	return n, rows.Err()
}

// runType derives a run's silence-budget key (R5): the role suffix
// (intake/execute/verify/compose — the B2 walking-skeleton naming) else the
// lane else "default". The ⚙ watchdog.silence_budget map is keyed by run-type;
// empty entries fall back to the recovery.dead_after seed.
func runType(r run.Run) string {
	base := stripForkSuffix(r.ID)
	for _, suffix := range []string{".intake", ".execute", ".verify", ".compose"} {
		if strings.HasSuffix(base, suffix) {
			return strings.TrimPrefix(suffix, ".")
		}
	}
	if r.Lane != "" {
		return r.Lane
	}
	return "default"
}

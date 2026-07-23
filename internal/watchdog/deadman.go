package watchdog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// deadman.go — the S14.4 dead-man canary V2 (brief R28-DMC; B4-7 OQ5 carry).
// The watchdog's OWN liveness: does the detection machinery still detect? V2 is
// an ACTIVE end-to-end probe (V1 was a research-note plant with no local tier):
//
//	(a) inject a synthetic known-anomaly (loop) trace on a canary run and assert
//	    the watchdog DETECTS + Tier-1-ANNOTATES + FLAGS it (Tier-0 → Tier-1
//	    local → flag end-to-end), then
//	(b) make a $0 local liveness call through the disambiguator seat to confirm
//	    the local floor answers.
//
// SHELL-DRIVEN (the independent observer — NOT the watchdog's own Tier-0 loop,
// so a dead detection loop is observable → non-circular). Distinct from the
// systemd WatchdogSec control-plane heartbeat (S01.2, which watches the whole
// process). On FAILURE (detection dark / the local call fails) → an ALARM
// (watchdog.flagged, rule=watchdog.dead_man, flag-now — OQ5, staying within the
// suite's own watchdog.* types, not poaching B5-6's canary.result). On PASS → a
// quiet liveness marker (journald, no new event type). The daily cadence is a
// structural constant (DeadManInterval — no new ⚙).

// canaryPrefix marks the synthetic dead-man canary runs so their (real, honest)
// synthetic flags are excluded from the flag-now chattiness meta-alert (R23) AND
// from the inbox projection (drain D4). internal/api cannot import this package,
// so internal/api/projection.go pins an identical literal (deadManCanaryPrefix) —
// a rename here MUST update that copy too.
const canaryPrefix = "platform.deadman."

// DeadManProbeIfDue runs a dead-man probe only when one is DUE (drain D6). The
// shell calls it on the sweep cadence instead of a 24h wall-clock ticker: a
// ticker resets on every process restart and freezes across suspend, so on a
// laptop the daily probe may rarely or never fire — silently — which is the
// exact failure class the dead-man exists to catch. Dueness is derived from the
// event log (OQ2), so it survives restart and suspend.
func (w *Watchdog) DeadManProbeIfDue(ctx context.Context) error {
	due, err := w.deadManDue(ctx, time.Now())
	if err != nil {
		return err
	}
	if !due {
		return nil
	}
	return w.DeadManProbe(ctx)
}

// deadManDue reports whether a dead-man probe is due as of now: no probe has
// ever run (first-probe-due-immediately — it is $0 and proves the whole
// detection pipeline at bring-up), or ≥ DeadManInterval has elapsed since the
// last canary's birth. The DeadManInterval (24h) stays a structural constant;
// only WHEN it is measured changed (from a wall-clock ticker to a log anchor).
func (w *Watchdog) deadManDue(ctx context.Context, now time.Time) (bool, error) {
	last, ok, err := w.lastDeadManTime(ctx)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil // never probed → due immediately
	}
	return now.Sub(last) >= DeadManInterval, nil
}

// lastDeadManTime is the wall-clock birth of the most recent dead-man canary,
// derived from the log (the latest platform.deadman.* run.created event) — no
// side store, no cursor (S14.1 / OQ2). Returns ok=false when no canary has ever
// been born.
func (w *Watchdog) lastDeadManTime(ctx context.Context) (time.Time, bool, error) {
	var tsRaw string
	err := w.db.QueryRowContext(ctx,
		`SELECT ts FROM run_events
		 WHERE run_id LIKE ? AND type = ?
		 ORDER BY event_seq DESC LIMIT 1`, canaryPrefix+"%", run.EventCreated).Scan(&tsRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return parseTS(tsRaw), true, nil
}

// DeadManProbe runs one dead-man canary pass (R28-DMC). It never returns an
// error for a detection/local failure — a failure is the alarm it RAISES (the
// probe itself succeeding at raising the alarm is the success). It returns an
// error only when it cannot run the probe at all (e.g. the DB is unreachable),
// which the shell logs.
func (w *Watchdog) DeadManProbe(ctx context.Context) error {
	canaryID := fmt.Sprintf("%s%d", canaryPrefix, time.Now().UnixNano())

	canary, err := w.createCanary(ctx, canaryID)
	if err != nil {
		return fmt.Errorf("watchdog: dead-man create canary: %w", err)
	}
	defer w.terminateCanary(ctx, canaryID)

	// (b) $0 local liveness call FIRST, while the canary is running
	// (checkpointable). A duty error ⇒ the local floor is dark.
	localAlive := w.deadManLiveness(ctx, canaryID)

	// (a) inject the synthetic loop trace, then drive the FULL detection path.
	if err := w.injectLoopTrace(ctx, canary); err != nil {
		return fmt.Errorf("watchdog: dead-man inject trace: %w", err)
	}
	if err := w.EvaluateRun(ctx, canaryID); err != nil {
		return fmt.Errorf("watchdog: dead-man evaluate: %w", err)
	}
	detected, err := w.canaryFlagged(ctx, canaryID)
	if err != nil {
		return fmt.Errorf("watchdog: dead-man read result: %w", err)
	}

	if detected && localAlive {
		// Quiet liveness marker — journald only, NO new event type (OQ5).
		w.logger.Info("watchdog: dead-man canary PASS — detection + local floor alive",
			"canary", canaryID, "local_probed", w.duty != nil)
		return nil
	}
	// ALARM: the watchdog is dark. flag-now, staying within watchdog.* (OQ5).
	detail := "dead-man canary FAILED:"
	if !detected {
		detail += " the detection machinery did not flag a known synthetic loop"
	}
	if !localAlive {
		detail += " the $0 local liveness call did not answer"
	}
	w.logger.Error("watchdog: DEAD-MAN ALARM — the watchdog is dark", "canary", canaryID, "detected", detected, "local_alive", localAlive)
	return w.flagPlatform(ctx, trigger{
		Rule:   RuleDeadMan,
		Counts: map[string]int{"detected": boolToInt(detected), "local_alive": boolToInt(localAlive)},
		Detail: detail,
	})
}

// createCanary creates and drives a synthetic platform canary run to running
// (checkpointable), so the probe can land a $0 D7 row and detect a loop on it.
func (w *Watchdog) createCanary(ctx context.Context, id string) (run.Run, error) {
	if _, err := w.runs.Create(ctx, run.NewRun{
		ID: id, UserID: run.ActorPlatform, Lane: local.LaneLocal, Substrate: "local",
	}); err != nil {
		return run.Run{}, err
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := w.runs.Transition(ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			return run.Run{}, err
		}
	}
	return w.runs.Get(ctx, id)
}

// deadManLiveness makes a minimal $0 disambiguator call to confirm the local
// floor answers (R28-DMC part b). No duty (unconfigured local tier) ⇒ nothing
// to probe ⇒ alive (an honest skip, not a failure). A duty error ⇒ dark.
func (w *Watchdog) deadManLiveness(ctx context.Context, canaryID string) bool {
	if w.duty == nil {
		return true
	}
	_, err := w.duty.Call(ctx, canaryID, local.DutyRequest{
		Alias:          local.AliasWatchdog,
		System:         "Liveness probe. Reason briefly, then answer verdict=productive.",
		User:           "This is a liveness check. Respond with verdict=productive.",
		Schema:         WatchdogSchema(),
		Name:           "watchdog-deadman-liveness",
		MaxTokens:      disambigMaxTokens,
		Classification: true,
	})
	if err != nil {
		w.logger.Warn("watchdog: dead-man local liveness call failed", "err", err)
		return false
	}
	return true
}

// injectLoopTrace appends ⚙ watchdog.loop_repeat identical synthetic
// tool.completed events on the canary run so loopTrip trips at the live
// threshold (proving detection at the configured value, not a hard-coded one).
func (w *Watchdog) injectLoopTrace(ctx context.Context, canary run.Run) error {
	n, err := w.settings.Int(keyLoopRepeat)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"tool":        "deadman.synthetic",
		"args_digest": "sha256:deadman-canary",
		"excerpt":     "synthetic known-anomaly trace (dead-man canary)",
	})
	if err != nil {
		return err
	}
	for i := int64(0); i < n; i++ {
		if _, err := w.log.Append(ctx, eventlog.Append{
			RunID:         canary.ID,
			Generation:    canary.Generation,
			UserID:        run.ActorPlatform,
			Type:          "tool.completed",
			SchemaVersion: eventSchemaVersion,
			Payload:       payload,
		}); err != nil {
			return err
		}
	}
	return nil
}

// canaryFlagged reports whether the detection path produced a loop flag on the
// canary — the liveness signal (detection is alive).
func (w *Watchdog) canaryFlagged(ctx context.Context, canaryID string) (bool, error) {
	_, found, err := w.latestFlagSeq(ctx, canaryID, RuleLoop)
	return found, err
}

// terminateCanary drives the canary run to a terminal state so it never lingers
// as an active run the silence sweep would later flag. Best-effort, on a
// cancel-immune context (cleanup must complete even as ctx unwinds).
func (w *Watchdog) terminateCanary(ctx context.Context, id string) {
	cctx := context.WithoutCancel(ctx)
	r, err := w.runs.Get(cctx, id)
	if err != nil {
		return
	}
	var to run.State
	switch r.State {
	case run.StateParked:
		to = run.StateFinalized // detection parked it → finalize-with-card terminal
	case run.StateRunning, run.StateDraining:
		to = run.StateCompleted // detection did not park (broken) → complete it
	default:
		return // already terminal
	}
	if _, err := w.runs.Transition(cctx, id, to, run.TransitionOptions{
		Reason: "dead-man canary complete", Actor: run.ActorPlatform,
	}); err != nil {
		w.logger.Warn("watchdog: dead-man canary cleanup", "canary", id, "err", err)
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

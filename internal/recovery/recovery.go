// Package recovery is the startup/wake recovery ladder of Spec S02.5: one
// level-triggered reconcile pass — the same code at platform start, at
// wake, and on the periodic sweep (⚙ recovery.sweep_interval; the shell
// owns the scheduling) — following the kubelet rule: re-list actual state,
// never trust supervisor memory.
//
// The pass classifies every non-terminal run as ALIVE / WEDGED /
// FINISHED-DURING-OUTAGE / DEAD from observed actuals (unit liveness,
// event-cursor advance, engine-store harvest records, suspend-aware lease
// expiry), then acts: reattach, pause-and-flag, harvest-and-complete, or
// crash + fork-from-last-checkpoint with generation+1 — bounded by one
// durable dispatch id per interruption, ⚙ recovery.max_attempts, and
// ⚙ recovery.stale_finalize. It then re-observes asks, resolves in-doubt
// effects per class, and runs GC. Suspend/resume is a crash-equivalent
// (P-T02-3): generation fencing plus ⚙ recovery.wake_grace on wall-clock
// deadlines render the double-resume factory inert (P-T07-4).
package recovery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// Dotted ⚙ keys consumed by the ladder (owned by Spec S02; G2 Def.2 set).
const (
	keyDeadAfter     = "recovery.dead_after"
	keyWakeGrace     = "recovery.wake_grace"
	keyMaxAttempts   = "recovery.max_attempts"
	keyStaleFinalize = "recovery.stale_finalize"
	keySweepInterval = "recovery.sweep_interval"
)

// Settings is the ladder's view of the settings registry (Spec S01.10).
type Settings interface {
	Duration(key string) (time.Duration, error)
	Int(key string) (int64, error)
}

// Config assembles a Ladder. DB, Log, Runs, Checkpoints, Effects and
// Settings are required; Units and Harvest default to the B0 no-op probes
// (NoUnits, NoHarvest), Now to time.Now, Logger to a discarding logger.
type Config struct {
	DB          *storage.DB
	Log         *eventlog.Log
	Runs        *run.Store
	Checkpoints *gates.Checkpoints
	Effects     *gates.Journal
	Units       UnitLiveness
	Harvest     HarvestSource
	Settings    Settings
	// NoFork names the run classes whose crashed disposition is FINALIZE rather
	// than the step-3 default fork. It is a PREDICATE SEAM composed at the shell
	// root (this package imports nothing of the classes it names), nil meaning
	// "every crashed run takes the ordinary ladder" — the pre-B6-2C behavior
	// exactly.
	//
	// The classes at v0 are (a) the BENCH-REG §2 direct arm — §2 is frozen: the
	// arm runs ONCE, single-shot, "no retries, no follow-up turns, take what
	// comes", so a fork is a SECOND billed engine run on the same frozen
	// statement whose output could never be used anyway (the capture is keyed on
	// the parent run id) — and (b) any run id carrying no dispatchable role,
	// whose successor no dispatcher could route (P3-RW-6 R15).
	NoFork func(runID string) bool
	Logger *slog.Logger
	Now    func() time.Time
}

// Ladder is the reconcile pass. It satisfies shell.RecoveryLadder.
type Ladder struct {
	cfg Config

	mu        sync.Mutex // serializes passes (start, wake, sweep may overlap)
	passCount int
	lastPass  time.Time
}

// New validates cfg and returns the ladder.
func New(cfg Config) (*Ladder, error) {
	switch {
	case cfg.DB == nil:
		return nil, errors.New("recovery: nil DB")
	case cfg.Log == nil:
		return nil, errors.New("recovery: nil event log")
	case cfg.Runs == nil:
		return nil, errors.New("recovery: nil run store")
	case cfg.Checkpoints == nil:
		return nil, errors.New("recovery: nil checkpoint store")
	case cfg.Effects == nil:
		return nil, errors.New("recovery: nil effect journal")
	case cfg.Settings == nil:
		return nil, errors.New("recovery: nil settings registry")
	}
	if cfg.Units == nil {
		cfg.Units = NoUnits{}
	}
	if cfg.Harvest == nil {
		cfg.Harvest = NoHarvest{}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	return &Ladder{cfg: cfg}, nil
}

// Report is one pass's tally, logged by the ladder and returned by
// ReconcilePass for tests and tooling.
type Report struct {
	Scanned    int // non-terminal runs observed
	Alive      int
	Wedged     int
	Harvested  int
	Forked     int
	Tombstoned int
	Finalized  int
	Pending    int // left untouched (live lease not yet expired, suspend-aware)

	AsksObserved      int
	AsksEngineExpired int

	EffectsReplayable int // in-doubt A/B/C returned to approved
	EffectsUnknown    int // in-doubt D flipped to unknown

	GraceApplied bool // this pass evaluated deadlines with ⚙ recovery.wake_grace
}

// Reconcile runs one full pass (shell.RecoveryLadder).
func (l *Ladder) Reconcile(ctx context.Context) error {
	_, err := l.ReconcilePass(ctx)
	return err
}

// ReconcilePass runs one full pass and returns its report. Per-run
// failures do not stop the pass (level-triggered: the next sweep retries)
// but are joined into the returned error so startup surfaces defects
// loudly.
func (l *Ladder) ReconcilePass(ctx context.Context) (Report, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.cfg.Now()
	deadAfter, err := l.cfg.Settings.Duration(keyDeadAfter)
	if err != nil {
		return Report{}, fmt.Errorf("recovery: read ⚙ %s: %w", keyDeadAfter, err)
	}
	wakeGrace, err := l.cfg.Settings.Duration(keyWakeGrace)
	if err != nil {
		return Report{}, fmt.Errorf("recovery: read ⚙ %s: %w", keyWakeGrace, err)
	}
	staleFinalize, err := l.cfg.Settings.Duration(keyStaleFinalize)
	if err != nil {
		return Report{}, fmt.Errorf("recovery: read ⚙ %s: %w", keyStaleFinalize, err)
	}
	maxAttempts, err := l.cfg.Settings.Int(keyMaxAttempts)
	if err != nil {
		return Report{}, fmt.Errorf("recovery: read ⚙ %s: %w", keyMaxAttempts, err)
	}
	sweepInterval, err := l.cfg.Settings.Duration(keySweepInterval)
	if err != nil {
		return Report{}, fmt.Errorf("recovery: read ⚙ %s: %w", keySweepInterval, err)
	}

	// Suspend-aware grace (Spec S02.5 step 4, P-T07-4): after a wake no
	// holder had CPU to heartbeat, so wall-clock deadlines are evaluated
	// with ⚙ recovery.wake_grace before anything is declared dead. B0 has
	// no logind wiring (S01.7, later packet), so "just woke" is detected
	// level-triggered: the first pass of a process is crash-equivalent and
	// gets the grace unconditionally; a wall-clock jump of more than two
	// sweep intervals between passes means the process was not getting CPU
	// — the S01.7 clock-jump wake detection.
	grace := time.Duration(0)
	if l.passCount == 0 || (!l.lastPass.IsZero() && now.Sub(l.lastPass) > 2*sweepInterval) {
		grace = wakeGrace
	}
	l.passCount++
	l.lastPass = now

	rpt := Report{GraceApplied: grace > 0}
	var errs []error

	// ── Steps 1–4 (observe, classify, bound, leases+fencing), per
	// non-terminal run. Fencing itself is standing eventlog behavior: every
	// append carries the run's generation and stale appends are rejected.
	observed, err := l.cfg.Runs.InStates(ctx, run.StateClaimed, run.StateRunning, run.StateDraining)
	if err != nil {
		return rpt, err
	}
	for _, r := range observed {
		rpt.Scanned++
		if err := l.reconcileRun(ctx, now, grace, deadAfter, staleFinalize, maxAttempts, r, &rpt); err != nil {
			errs = append(errs, fmt.Errorf("run %q: %w", r.ID, err))
		}
	}
	// Crashed runs are terminal-but-supersedable: normally crash + bound
	// decision commit in ONE transaction, so a crashed run without a
	// successor only exists if rows predate this machinery or were written
	// by hand — sweep them through the same step-3 bound decision (and the
	// ratified crashed→completed harvest path).
	crashed, err := l.cfg.Runs.InStates(ctx, run.StateCrashed)
	if err != nil {
		return rpt, err
	}
	for _, r := range crashed {
		if err := l.reconcileCrashed(ctx, now, staleFinalize, maxAttempts, r, &rpt); err != nil {
			errs = append(errs, fmt.Errorf("crashed run %q: %w", r.ID, err))
		}
	}

	// ── Step 5: asks reconcile. Platform ask rows are authoritative
	// (engine-side asks are volatile); the row's invocation snapshot is the
	// resume input. Re-hydrating an engine that still holds the ask and
	// re-issuing full reconstruction where it lost it need a live adapter —
	// that arrives at B1/B2 (Spec S03/S06); here the pass re-observes and
	// counts, including asks whose engine-side expiry has passed.
	if err := l.reconcileAsks(ctx, now, &rpt); err != nil {
		errs = append(errs, err)
	}

	// ── Step 6: effects reconcile — any `executing` row is in-doubt
	// (Spec S02.7): replay path for classes A–C, unknown + card for D.
	resolutions, err := l.cfg.Effects.ReconcileInDoubt(ctx)
	if err != nil {
		errs = append(errs, err)
	}
	for _, res := range resolutions {
		if res.To == gates.EffectUnknown {
			rpt.EffectsUnknown++
		} else {
			rpt.EffectsReplayable++
		}
	}

	// ── Step 7: GC. At B0 every GC target still lacks its machinery, so
	// this pass is observation-only:
	//   - unit corpses reset-failed after harvest → needs systemd control
	//     (B1, run units; Spec S02.5 step 7);
	//   - orphan worktrees flagged, NEVER auto-deleted while they hold
	//     uncommitted work → workspace machinery (B1, Spec S02.10);
	//   - engine session files past retention → adapters (B1, Spec S03);
	//   - tombstone-review cards → operator surfaces (B5/B6, Spec S15).

	l.cfg.Logger.Info("recovery ladder: pass complete (Spec S02.5)",
		"scanned", rpt.Scanned, "alive", rpt.Alive, "wedged", rpt.Wedged,
		"harvested", rpt.Harvested, "forked", rpt.Forked,
		"tombstoned", rpt.Tombstoned, "finalized", rpt.Finalized,
		"pending", rpt.Pending,
		"asks_observed", rpt.AsksObserved, "asks_engine_expired", rpt.AsksEngineExpired,
		"effects_replayable", rpt.EffectsReplayable, "effects_unknown", rpt.EffectsUnknown,
		"wake_grace_applied", rpt.GraceApplied)
	return rpt, errors.Join(errs...)
}

// reconcileRun classifies one observed run (Spec S02.5 step 2) and acts.
func (l *Ladder) reconcileRun(ctx context.Context, now time.Time, grace, deadAfter, staleFinalize time.Duration, maxAttempts int64, r run.Run, rpt *Report) error {
	status, err := l.cfg.Units.Probe(ctx, r)
	if err != nil {
		return fmt.Errorf("probe unit: %w", err)
	}
	lastCkptSeq := int64(0)
	if cp, ok, err := l.cfg.Checkpoints.Last(ctx, r.ID); err != nil {
		return err
	} else if ok {
		lastCkptSeq = cp.EventSeq
	}
	recs, err := l.cfg.Harvest.NewerThan(ctx, r, lastCkptSeq)
	if err != nil {
		return fmt.Errorf("harvest probe: %w", err)
	}
	terminalHarvest := false
	for _, rec := range recs {
		if rec.Terminal {
			terminalHarvest = true
			break
		}
	}
	lastActivity, lastType, err := l.lastRunEvent(ctx, r)
	if err != nil {
		return err
	}

	// The two independent liveness observations the ladder weighs. The cursor
	// is PROGRESS ("cursor advancing" / "nothing newer to harvest", Spec S02.5
	// step 2; the ⚙ recovery.dead_after row's own help text is cursor silence),
	// the lease is HOLDER liveness (Spec S02.2, renewed at ⚙ recovery.heartbeat
	// cadence by whoever drives the advance — internal/run's LeaseKeeper).
	cursorFresh := now.Sub(lastActivity) <= deadAfter+grace
	// leaseBeating is strict: a deadline still in the future, i.e. a holder that
	// demonstrably beat recently. leaseDead is the suspend-aware form — between
	// them sits the wake grace, where a lease has expired but the platform
	// cannot yet honestly say whether its holder is gone (P-T07-4), and nothing
	// is decided at all.
	leaseBeating := !r.LeaseDeadline.IsZero() && !now.After(r.LeaseDeadline)

	switch {
	case terminalHarvest || status.State == UnitCorpseExit0:
		// FINISHED-DURING-OUTAGE → harvest: deliver the produced result
		// instead of redoing the work (Spec S02.5 step 2).
		if err := l.harvest(ctx, r, recs, status); err != nil {
			return err
		}
		rpt.Harvested++

	case status.State == UnitActive && cursorFresh:
		// ALIVE: unit active, cursor advancing → reattach streams and
		// re-arm watchdogs. Stream reattach is the adapter's (B1, Spec
		// S03); watchdog arming is Spec S14 (B5).
		rpt.Alive++

	case status.State == UnitActive:
		// WEDGED (derived, never stored — Spec S02.3): unit active, cursor
		// stalled past ⚙ recovery.dead_after → pause-and-flag, NEVER
		// auto-kill (D1.3). Tombstoning a repeat offender is a policy/
		// operator decision, not this pass's.
		if lastType != run.EventWedged { // flag once per stall episode
			if err := l.flagWedged(ctx, r, now.Sub(lastActivity)); err != nil {
				return err
			}
		}
		rpt.Wedged++

	case status.State == UnitCorpseFailed:
		// DEAD on the body alone: a failed corpse is conclusive and OUTRANKS
		// cursor recency. P-T07-2 makes the run wrapper write its own exit
		// record as its last act, so a corpse case can show a very recent
		// append that is death evidence rather than life — which is exactly
		// why cursor freshness gates DEAD only under an unknowable body.
		if err := l.crashAndBound(ctx, r, now.Sub(lastActivity), staleFinalize, maxAttempts, status, rpt); err != nil {
			return err
		}

	// ── From here the body is unknowable: UnitGone or UnitUnknown. There is
	// no unit to be "active", so the lease and the cursor are the only
	// actuals, and each answers a different question.

	case l.leaseDead(now, grace, r) && cursorFresh:
		// ALIVE. The holder's lease expired — but the run APPENDED something
		// within ⚙ recovery.dead_after, so S02.5's DEAD ("unit gone/failed
		// with nothing newer to harvest") is simply false: there is something
		// newer. This is the P3-RW-5 defect: a human-paced interview advancing
		// in-process past its one-shot claim lease was crashed and forked
		// while its checkpoint was seconds old.
		rpt.Alive++

	case l.leaseDead(now, grace, r):
		// DEAD, as a conjunction: no holder (lease dead or absent,
		// suspend-aware) AND no progress within ⚙ recovery.dead_after AND
		// nothing terminal to harvest → mark crashed, then supersede,
		// bounded by step 3.
		if err := l.crashAndBound(ctx, r, now.Sub(lastActivity), staleFinalize, maxAttempts, status, rpt); err != nil {
			return err
		}

	case leaseBeating && !cursorFresh:
		// WEDGED, in its in-process form: the holder is beating (so it is
		// alive) and progress has stopped. Same disposition as the unit-active
		// arm — flag once per stall episode, NEVER auto-kill (D1.3, S10.8).
		// Containment stays the watchdog's silence rule (park + card, S14.4);
		// this event is the ladder's own observation.
		if lastType != run.EventWedged {
			if err := l.flagWedged(ctx, r, now.Sub(lastActivity)); err != nil {
				return err
			}
		}
		rpt.Wedged++

	default:
		// Lease still live (suspend-aware) and no proof of death — leave
		// it; the next sweep re-evaluates.
		rpt.Pending++
	}
	return nil
}

// leaseDead evaluates the wall-clock lease deadline suspend-aware (Spec
// S02.5 step 4): expired only when now is past deadline + any wake grace.
// A run with no lease at all cannot re-assert itself, so it counts as dead
// for classification.
func (l *Ladder) leaseDead(now time.Time, grace time.Duration, r run.Run) bool {
	if r.LeaseDeadline.IsZero() {
		return true
	}
	return now.After(r.LeaseDeadline.Add(grace))
}

// lastRunEvent returns the wall-clock time and type of the run's newest
// event (ordering by event_seq, the sole authority; the recorded ts is used
// only as a staleness duration input — P-T07-4). Falls back to the row's
// updated_ts for rows without events.
func (l *Ladder) lastRunEvent(ctx context.Context, r run.Run) (time.Time, string, error) {
	var ts, typ string
	err := l.cfg.DB.QueryRowContext(ctx,
		`SELECT ts, type FROM run_events WHERE run_id = ? ORDER BY event_seq DESC LIMIT 1`,
		r.ID).Scan(&ts, &typ)
	if errors.Is(err, sql.ErrNoRows) {
		return r.UpdatedTS, "", nil
	}
	if err != nil {
		return time.Time{}, "", fmt.Errorf("recovery: last event: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("recovery: parse event ts %q: %w", ts, err)
	}
	return t, typ, nil
}

// flagWedged appends the run.wedged observation event (no state change —
// wedged is derived, Spec S02.3).
func (l *Ladder) flagWedged(ctx context.Context, r run.Run, stalledFor time.Duration) error {
	payload, err := json.Marshal(struct {
		StalledForS int64  `json:"stalled_for_s"`
		Actor       string `json:"actor"`
	}{int64(stalledFor.Seconds()), run.ActorPlatform})
	if err != nil {
		return fmt.Errorf("recovery: marshal wedged payload: %w", err)
	}
	_, err = l.cfg.Log.Append(ctx, eventlog.Append{
		RunID:         r.ID,
		Generation:    r.Generation,
		UserID:        r.UserID,
		Type:          run.EventWedged,
		SchemaVersion: 1,
		Payload:       payload,
	})
	return err
}

// harvest appends the mined records (deduplicated by (session_id,
// message_id)) and marks the run completed, in one transaction (Spec S02.5
// step 2).
func (l *Ladder) harvest(ctx context.Context, r run.Run, recs []HarvestRecord, status UnitStatus) error {
	return l.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		appended := 0
		for _, rec := range recs {
			dup, err := harvestDup(ctx, tx, r.ID, rec)
			if err != nil {
				return err
			}
			if dup {
				continue
			}
			payload, err := json.Marshal(struct {
				SessionID string          `json:"session_id"`
				MessageID string          `json:"message_id"`
				Terminal  bool            `json:"terminal,omitempty"`
				Payload   json.RawMessage `json:"payload,omitempty"`
				Usage     json.RawMessage `json:"usage,omitempty"`
			}{rec.SessionID, rec.MessageID, rec.Terminal, rec.Payload, rec.Usage})
			if err != nil {
				return fmt.Errorf("recovery: marshal harvest payload: %w", err)
			}
			if _, err := l.cfg.Log.AppendTx(ctx, tx, eventlog.Append{
				RunID:         r.ID,
				Generation:    r.Generation,
				UserID:        r.UserID,
				Type:          run.EventHarvest,
				SchemaVersion: 1,
				Payload:       payload,
			}); err != nil {
				return err
			}
			appended++
		}
		detail, err := json.Marshal(struct {
			HarvestedRecords int    `json:"harvested_records"`
			UnitResult       string `json:"unit_result,omitempty"`
		}{appended, status.Result})
		if err != nil {
			return fmt.Errorf("recovery: marshal harvest detail: %w", err)
		}
		_, err = l.cfg.Runs.TransitionTx(ctx, tx, r.ID, run.StateCompleted, run.TransitionOptions{
			Reason: "harvest: finished-during-outage (Spec S02.5)",
			Actor:  run.ActorPlatform,
			Detail: detail,
		})
		return err
	})
}

// harvestDup reports whether a harvest event for (session_id, message_id)
// already exists — the ratified dedup key (Spec S02.5 step 2).
func harvestDup(ctx context.Context, tx *sql.Tx, runID string, rec HarvestRecord) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM run_events
		     WHERE run_id = ? AND type = ?
		       AND json_extract(payload, '$.session_id') = ?
		       AND json_extract(payload, '$.message_id') = ?)`,
		runID, run.EventHarvest, rec.SessionID, rec.MessageID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("recovery: harvest dedup: %w", err)
	}
	return exists == 1, nil
}

// crashAndBound marks a DEAD run crashed and applies the step-3 bound —
// finalize (interrupted past ⚙ recovery.stale_finalize), tombstone
// (⚙ recovery.max_attempts exhausted), or fork-from-last-checkpoint with
// generation+1 — all in ONE transaction, so recovery never leaves a crashed
// run without its decision (Spec S02.5 steps 2–3).
func (l *Ladder) crashAndBound(ctx context.Context, r run.Run, interrupted time.Duration, staleFinalize time.Duration, maxAttempts int64, status UnitStatus, rpt *Report) error {
	return l.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		detail, err := json.Marshal(struct {
			UnitState  string `json:"unit_state"`
			UnitResult string `json:"unit_result,omitempty"`
			Actor      string `json:"actor"`
		}{unitStateString(status.State), status.Result, run.ActorPlatform})
		if err != nil {
			return fmt.Errorf("recovery: marshal crash detail: %w", err)
		}
		crashed, err := l.cfg.Runs.TransitionTx(ctx, tx, r.ID, run.StateCrashed, run.TransitionOptions{
			Reason: "recovery: DEAD (Spec S02.5)",
			Actor:  run.ActorPlatform,
			Detail: detail,
		})
		if err != nil {
			return err
		}
		return l.boundTx(ctx, tx, crashed, interrupted, staleFinalize, maxAttempts, rpt)
	})
}

// reconcileCrashed applies the bound decision to a crashed run that has no
// successor yet, harvesting first when the engine store shows the finished
// result ("{crashed | finished-during-outage}→completed via harvest",
// Spec S02.3).
func (l *Ladder) reconcileCrashed(ctx context.Context, now time.Time, staleFinalize time.Duration, maxAttempts int64, r run.Run, rpt *Report) error {
	lastCkptSeq := int64(0)
	if cp, ok, err := l.cfg.Checkpoints.Last(ctx, r.ID); err != nil {
		return err
	} else if ok {
		lastCkptSeq = cp.EventSeq
	}
	recs, err := l.cfg.Harvest.NewerThan(ctx, r, lastCkptSeq)
	if err != nil {
		return fmt.Errorf("harvest probe: %w", err)
	}
	for _, rec := range recs {
		if rec.Terminal {
			if err := l.harvest(ctx, r, recs, UnitStatus{}); err != nil {
				return err
			}
			rpt.Harvested++
			return nil
		}
	}
	lastActivity, _, err := l.lastRunEvent(ctx, r)
	if err != nil {
		return err
	}
	return l.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, ok, err := l.cfg.Runs.SuccessorOfTx(ctx, tx, r.ID); err != nil {
			return err
		} else if ok {
			return nil // already superseded
		}
		return l.boundTx(ctx, tx, r, now.Sub(lastActivity), staleFinalize, maxAttempts, rpt)
	})
}

// boundTx is the step-3 bound decision on a crashed run, inside the
// caller's transaction.
func (l *Ladder) boundTx(ctx context.Context, tx *sql.Tx, crashed run.Run, interrupted time.Duration, staleFinalize time.Duration, maxAttempts int64, rpt *Report) error {
	switch {
	case l.cfg.NoFork != nil && l.cfg.NoFork(crashed.ID):
		// PER-EDGE CITATION (crashed→finalized, a ratified S02.3 edge): for a
		// no-fork run class the class's own contract OUTRANKS the step-3 default
		// disposition. The predicate covers two grounds, both composed by the
		// shell — this package still names no run class of its own:
		//
		//   - a SINGLE-SHOT class, the BENCH-REG §2 direct arm, whose "run once,
		//     no retries" is FROZEN registered text: a fork would not be a
		//     recovery but a protocol breach that also bills the requester a
		//     second time for an output nothing can consume;
		//   - a run id carrying NO dispatchable role: its successor could never
		//     be routed, so forking it produces a run that crashes on dispatch
		//     and is forked again — a loop that ends at a tombstone having done
		//     nothing (P3-RW-6 R15).
		//
		// Finalize-with-card is the honest terminal either way: the run ended, it
		// ended badly, and the record says so. Any partial spend is already on
		// the ledger and the checkpoints, so nothing is lost by not re-running it.
		detail, err := json.Marshal(struct {
			InterruptedS int64  `json:"interrupted_s"`
			Card         string `json:"card"`
			NoFork       string `json:"no_fork_reason"`
		}{int64(interrupted.Seconds()),
			"finalized-with-card: this run class is never forked",
			"a run class the platform never forks: a single-shot class whose contract outranks the S02.5 step-3 default fork (BENCH-REG §2, the direct arm), or a run id with no dispatchable role, whose fork could never be routed"})
		if err != nil {
			return fmt.Errorf("recovery: marshal no-fork finalize detail: %w", err)
		}
		if _, err := l.cfg.Runs.TransitionTx(ctx, tx, crashed.ID, run.StateFinalized, run.TransitionOptions{
			Reason: "recovery: this run class is finalized, never forked (Spec S02.5 step 3; BENCH-REG §2 for the single-shot arm)",
			Actor:  run.ActorPlatform,
			Detail: detail,
		}); err != nil {
			return err
		}
		rpt.Finalized++
		return nil

	case interrupted > staleFinalize:
		// Finalize-with-card rather than blind resume (Spec S02.5 step 3;
		// the S02.6 freshness pass precedes any RESUME regardless — here
		// there is no resume at all). Card surfaces arrive at B5/B6; the
		// durable substance is the terminal state + its event.
		detail, err := json.Marshal(struct {
			InterruptedS int64  `json:"interrupted_s"`
			Card         string `json:"card"`
		}{int64(interrupted.Seconds()), "finalized-with-card: interrupted past ⚙ recovery.stale_finalize"})
		if err != nil {
			return fmt.Errorf("recovery: marshal finalize detail: %w", err)
		}
		if _, err := l.cfg.Runs.TransitionTx(ctx, tx, crashed.ID, run.StateFinalized, run.TransitionOptions{
			Reason: "recovery: stale interruption (Spec S02.5 step 3)",
			Actor:  run.ActorPlatform,
			Detail: detail,
		}); err != nil {
			return err
		}
		rpt.Finalized++
		return nil

	case crashed.RecoveryAttempts+1 > maxAttempts:
		// Repeat offender (Spec S02.5 step 3): tombstone; the review card
		// is GC/operator-surface material (B5/B6).
		detail, err := json.Marshal(struct {
			RecoveryAttempts int64  `json:"recovery_attempts"`
			Card             string `json:"card"`
		}{crashed.RecoveryAttempts, "tombstone-review: ⚙ recovery.max_attempts exhausted"})
		if err != nil {
			return fmt.Errorf("recovery: marshal tombstone detail: %w", err)
		}
		if _, err := l.cfg.Runs.TransitionTx(ctx, tx, crashed.ID, run.StateTombstoned, run.TransitionOptions{
			Reason: "recovery: repeat offender (Spec S02.5 step 3)",
			Actor:  run.ActorPlatform,
			Detail: detail,
		}); err != nil {
			return err
		}
		rpt.Tombstoned++
		return nil

	default:
		if err := l.forkTx(ctx, tx, crashed); err != nil {
			return err
		}
		rpt.Forked++
		return nil
	}
}

// forkTx supersedes a crashed run by fork-from-last-checkpoint as a new run
// with generation+1 (Spec S02.3, S02.5 step 2): the crashed row's
// generation is bumped — the takeover fence that renders any zombie holder
// of the old generation inert — and the successor is created queued at the
// new generation under one durable dispatch id per interruption (step 3),
// enforced by the unique dispatch index (an ambiguous failure cannot
// double-start). Backoff between attempts rides the pass cadence
// (⚙ recovery.sweep_interval) until the scheduler owns re-dispatch timing
// (B1, Spec S10).
func (l *Ladder) forkTx(ctx context.Context, tx *sql.Tx, crashed run.Run) error {
	newGen := crashed.Generation + 1
	dispatchID := fmt.Sprintf("%s#g%d", crashed.ID, newGen)
	if _, ok, err := l.cfg.Runs.SuccessorByDispatchTx(ctx, tx, dispatchID); err != nil {
		return err
	} else if ok {
		return nil // this interruption already dispatched its recovery
	}
	// Fence the old generation before the successor exists (Spec S02.5
	// step 4): the crash event above appended at the old generation — the
	// last act of the dead incarnation.
	if _, err := l.cfg.Runs.BumpGenerationTx(ctx, tx, crashed.ID); err != nil {
		return err
	}
	var (
		ckptID  int64
		ckptSeq int64
	)
	if cp, ok, err := l.cfg.Checkpoints.LastTx(ctx, tx, crashed.ID); err != nil {
		return err
	} else if ok {
		ckptID, ckptSeq = cp.ID, cp.EventSeq
	}
	detail, err := json.Marshal(struct {
		ParentRunID       string `json:"parent_run_id"`
		ParentGeneration  int64  `json:"parent_generation"`
		FromCheckpointID  int64  `json:"from_checkpoint_id,omitempty"`
		FromCheckpointSeq int64  `json:"from_checkpoint_seq,omitempty"`
		DispatchID        string `json:"dispatch_id"`
		RecoveryAttempt   int64  `json:"recovery_attempt"`
		Actor             string `json:"actor"`
	}{crashed.ID, crashed.Generation, ckptID, ckptSeq, dispatchID,
		crashed.RecoveryAttempts + 1, run.ActorPlatform})
	if err != nil {
		return fmt.Errorf("recovery: marshal fork detail: %w", err)
	}
	// The successor re-enters as queued: the work was already admitted
	// once; queue-row materialization and claiming are the scheduler's
	// (B1, Spec S10).
	_, err = l.cfg.Runs.CreateTx(ctx, tx, run.NewRun{
		ID:               fmt.Sprintf("%s.g%d", crashed.ID, newGen),
		UserID:           crashed.UserID,
		TaskID:           crashed.TaskID,
		Substrate:        crashed.Substrate,
		Lane:             crashed.Lane,
		WorkspaceRef:     crashed.WorkspaceRef,
		CeilingTimeS:     crashed.CeilingTimeS,
		CeilingSteps:     crashed.CeilingSteps,
		CeilingCostUSD:   crashed.CeilingCostUSD,
		State:            run.StateQueued,
		Generation:       newGen,
		ParentRunID:      crashed.ID,
		DispatchID:       dispatchID,
		RecoveryAttempts: crashed.RecoveryAttempts + 1,
	}, run.EventForked, detail)
	return err
}

// reconcileAsks re-observes open asks (answered_ts unset) on non-terminal
// runs (Spec S02.5 step 5).
func (l *Ladder) reconcileAsks(ctx context.Context, now time.Time, rpt *Report) error {
	rows, err := l.cfg.DB.QueryContext(ctx,
		`SELECT a.ask_id, a.engine_expiry_ts
		   FROM asks a JOIN runs r ON r.run_id = a.run_id
		  WHERE a.answered_ts IS NULL
		    AND r.state NOT IN ('completed', 'finalized', 'tombstoned', 'died-at-gate')`)
	if err != nil {
		return fmt.Errorf("recovery: list open asks: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var askID string
		var engineExpiry sql.NullString
		if err := rows.Scan(&askID, &engineExpiry); err != nil {
			return fmt.Errorf("recovery: scan ask: %w", err)
		}
		rpt.AsksObserved++
		if engineExpiry.Valid && engineExpiry.String != "" {
			t, err := time.Parse(time.RFC3339Nano, engineExpiry.String)
			if err != nil {
				return fmt.Errorf("recovery: parse ask %q engine_expiry_ts: %w", askID, err)
			}
			if now.After(t) {
				// The engine has expired its side (P-T02-1): resume needs
				// re-issue from the row's invocation snapshot — adapter
				// machinery, B1/B2 (Spec S03/S06).
				rpt.AsksEngineExpired++
			}
		}
	}
	return rows.Err()
}

func unitStateString(s UnitState) string {
	switch s {
	case UnitActive:
		return "active"
	case UnitCorpseExit0:
		return "corpse-exit0"
	case UnitCorpseFailed:
		return "corpse-failed"
	case UnitGone:
		return "gone"
	default:
		return "unknown"
	}
}

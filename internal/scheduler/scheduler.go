// Package scheduler is the one SQLite-backed run scheduler of Spec S10.7 (the
// scheduler module seam of S01.1): it owns run admission, priority, park/
// resume timing, and budget gating. It reads and CAS-claims the S02 queue /
// lanes tables (per-(user, lane) concurrency slots from OBSERVED concurrency),
// enforces per-run ceilings (columns on runs, 3.7), attributes every admission
// to its owner (15.6), and orders work by the S10.7 priority ladder. It is
// needed even single-user for limit events (parks/resumes/budgets) — v0
// machinery regardless of the v1 schedule surface (S10.7, S00.2).
//
// Fire-and-forget is banned (S16.6): the ONLY way a run reaches an engine is
// Enqueue → claim → dispatch under this scheduler, owner-attributed. The
// Dispatcher seam keeps the scheduler decoupled from the adapter/engine
// specifics (Spec S01.3). B1-1's adapter already emits engine.rate_limit
// observations; the five-class limit taxonomy that consumes them lives here
// (limits.go), and the workload-class priority ladder in workload.go.
//
// B1-2 replaces the B0 admission stub with this scheduler. Dollar-based routing
// between flat-rate lanes is a D5 violation and NEVER done — the claim order
// uses workload class, aging, and consumption pressure, never dollars (S10.2).
package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// Queue-row statuses (Spec S02.2 queue family). 'queued' is claimable;
// 'claimed' holds a lease; 'done' is a settled admission (the run has left the
// live states). Ordering authority is never the queue row — it is the run FSM
// (Spec S02.3) and event_seq (S02.5).
const (
	queueQueued  = "queued"
	queueClaimed = "claimed"
	queueDone    = "done"
)

// defaultLaneConcurrency is the per-(user, lane) slot count when no lanes row
// declares one. Observed concurrency starts at 1 — serialize by default until
// the operator declares a higher cap (lanes are DATA, seeded from observed
// concurrency, Spec S10.7; S18 ratifies no lane-cap ⚙, so this is a structural
// default, the sseBatchSize precedent, CONVENTIONS §7). SetLaneCap edits it.
const defaultLaneConcurrency = 1

// keyBgAdmitStop is the ⚙ background-admission-stop pressure level, owned by
// Spec S10 (S10.4).
const keyBgAdmitStop = "pressure.bg_admit_stop"

// claimHolder is the lease holder stamped on a claimed run/queue row — the
// claimer identity (owner attribution of the WORK is the run's user_id, 15.6;
// this is the admitting party).
const claimHolder = "scheduler"

// defaultPoll is the idle re-poll cadence of the claim loop. Not a ⚙ setting —
// S18 ratifies no scheduler-poll key (the sseBatchSize precedent); an Enqueue
// nudges the loop, so the poll is only a backstop.
const defaultPoll = 500 * time.Millisecond

// Settings is the scheduler-facing view of the registry (Spec S01.10).
type Settings interface {
	Int(key string) (int64, error)
	Float(key string) (float64, error)
	Duration(key string) (time.Duration, error)
}

// Dispatcher drives a CAS-claimed run through its engine invocation to a
// terminal or parked disposition (Spec S03 adapter Driver). It owns the run
// from claimed onward: claimed→running→(completed|parked|died-at-gate|crashed).
// Keeping it a seam decouples the scheduler from the adapter/engine specifics
// (Spec S01.3) and from confinement (the sandbox/broker land at B1-3, Spec
// S11) — the control plane never spawns an engine except through a registered
// dispatcher.
type Dispatcher interface {
	Dispatch(ctx context.Context, r run.Run) error
}

// GPUAdmitter is the S10.9 GPU-admission POLICY hook (the S10-policy/S12-mechanic
// split): before the scheduler dispatches a run onto the local-inference lane,
// it asks whether the VRAM ledger admits loading the run's model (Spec S12.7:
// live memory.free ≥ ledger(model) + guard band). Whether the WORK is admitted
// at all — pressure, budgets, background caps, priorities — stays S10's; this
// hook is only the VRAM mechanic (S10.9 "the ledger/broker mechanics are
// [XREF:S12]"). It is satisfied by an internal/local VRAM admitter wired at the
// shell — the scheduler NEVER imports internal/local (CONVENTIONS §26), so this
// is a scheduler-DECLARED interface (the Pressure/Dispatcher nil-able precedent).
// Nil ⇒ no GPU gate (every non-local run, and the dev default, unchanged).
// WIRED-BUT-DORMANT at v0: class-(a) local execution has no consumer, so no run
// dispatches onto the local lane (P3/STATE binding) — the hook is hermetically
// tested, never live-exercised.
type GPUAdmitter interface {
	AdmitLoad(ctx context.Context, model string) (ok bool, reason string, err error)
}

// Config assembles a Scheduler. DB, Runs, Settings and Dispatcher are
// mandatory; the metering hooks default to inert if omitted.
type Config struct {
	DB         *storage.DB
	Runs       *run.Store
	Settings   Settings
	Dispatcher Dispatcher
	// Pressure gates background admission against consumption pressure (Spec
	// S10.4). Nil disables the pressure gate (background always admits).
	Pressure *metering.PressureGauge
	// Receipts materializes a receipt when a dispatched run reaches a terminal
	// state (Spec S10.1/S10.10). Nil skips receipt materialization.
	Receipts *metering.Receipts
	// GPUAdmitter is the S10.9 GPU-admission policy hook (Spec S12.7 mechanics).
	// Nil ⇒ no GPU gate. Consulted only for candidates on LocalLane (below).
	GPUAdmitter GPUAdmitter
	// LocalLane is the lane name whose dispatch is GPU-admission-gated (the S03.2
	// local lane, "local"). "" ⇒ no lane is gated. Set by the shell to the local
	// lane name as a bare string (never an internal/local import — §26 wall).
	LocalLane string
	Logger    *slog.Logger
	// Now is the clock seam (tests). Nil = time.Now.
	Now func() time.Time
	// LeaseTTL overrides the claim lease deadline (tests). 0 = ⚙
	// recovery.dead_after.
	LeaseTTL time.Duration
	// PollInterval overrides the claim-loop backstop cadence. 0 = defaultPoll.
	PollInterval time.Duration
}

// keyLeaseTTL is the cross-cutting ⚙ (owned by Spec S02) the claim lease
// deadline rides: a claimed run must heartbeat before recovery.dead_after or
// the recovery ladder reclaims it (Spec S02.5).
const keyLeaseTTL = "recovery.dead_after"

// Scheduler is the one run scheduler (Spec S10.7).
type Scheduler struct {
	db          *storage.DB
	runs        *run.Store
	settings    Settings
	dispatcher  Dispatcher
	pressure    *metering.PressureGauge
	receipts    *metering.Receipts
	gpuAdmitter GPUAdmitter
	localLane   string
	logger      *slog.Logger
	now         func() time.Time
	poll        time.Duration
	leaseTTL    time.Duration

	admitMu   sync.Mutex
	admitting bool

	nudge chan struct{}

	inflightMu sync.Mutex
	inflight   map[string]struct{}
	wg         sync.WaitGroup
}

// New assembles a Scheduler.
func New(cfg Config) (*Scheduler, error) {
	if cfg.DB == nil || cfg.Runs == nil || cfg.Settings == nil {
		return nil, errors.New("scheduler: DB, Runs and Settings are required")
	}
	if cfg.Dispatcher == nil {
		return nil, errors.New("scheduler: a Dispatcher is required (fire-and-forget is banned, S16.6)")
	}
	s := &Scheduler{
		db:          cfg.DB,
		runs:        cfg.Runs,
		settings:    cfg.Settings,
		dispatcher:  cfg.Dispatcher,
		pressure:    cfg.Pressure,
		receipts:    cfg.Receipts,
		gpuAdmitter: cfg.GPUAdmitter,
		localLane:   cfg.LocalLane,
		logger:      cfg.Logger,
		now:         cfg.Now,
		poll:        cfg.PollInterval,
		leaseTTL:    cfg.LeaseTTL,
		nudge:       make(chan struct{}, 1),
		inflight:    map[string]struct{}{},
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.poll <= 0 {
		s.poll = defaultPoll
	}
	return s, nil
}

// ── shell.Admission ────────────────────────────────────────────────────────

// ResumeAdmission opens admission (Spec S01.6 startup step 5, maintenance
// exit): the claim loop resumes claiming. Idempotent.
func (s *Scheduler) ResumeAdmission(ctx context.Context) error {
	s.admitMu.Lock()
	s.admitting = true
	s.admitMu.Unlock()
	s.doNudge()
	s.logger.InfoContext(ctx, "scheduler: admission resumed")
	return nil
}

// StopAdmission closes admission (Spec S01.6 shutdown, maintenance enter): the
// scheduler claims nothing new. In-flight dispatches continue (they drain or
// park per maintenance). Idempotent.
func (s *Scheduler) StopAdmission(ctx context.Context) error {
	s.admitMu.Lock()
	s.admitting = false
	s.admitMu.Unlock()
	s.logger.InfoContext(ctx, "scheduler: admission stopped")
	return nil
}

// ParkInFlightRuns parks still-running dispatched runs at the maintenance
// drain-grace terminal (Spec S01.6: parked, flagged, resumable — never a kill
// of record). It CAS-parks each tracked run that is still running/draining;
// the FSM CAS makes it race-safe against a dispatch goroutine finishing
// concurrently (a lost race means the run already resolved). The live
// engine-side pause (Session.Pause via the Driver) integrates with the
// dispatch path as confined execution lands (B1-3).
func (s *Scheduler) ParkInFlightRuns(ctx context.Context) error {
	for _, runID := range s.trackedRuns() {
		r, err := s.runs.Get(ctx, runID)
		if err != nil {
			continue
		}
		if r.State != run.StateRunning && r.State != run.StateDraining {
			continue
		}
		if _, err := s.runs.Transition(ctx, runID, run.StateParked, run.TransitionOptions{
			Reason: "maintenance drain grace expired (S01.6); parked, resumable", Actor: run.ActorPlatform,
		}); err != nil && !errors.Is(err, run.ErrInvalidTransition) {
			s.logger.WarnContext(ctx, "scheduler: park in-flight run", "run", runID, "err", err)
		}
	}
	return nil
}

func (s *Scheduler) admissionOpen() bool {
	s.admitMu.Lock()
	defer s.admitMu.Unlock()
	return s.admitting
}

// Tick runs one claim pass synchronously and returns how many runs it
// dispatched — a manual admission cycle for tests and ops, independent of the
// background loop and the admission gate. Dispatch itself remains asynchronous
// (each dispatched run runs in a tracked goroutine); WaitInFlight blocks until
// they settle.
func (s *Scheduler) Tick(ctx context.Context) (int, error) { return s.claimPass(ctx) }

// WaitInFlight blocks until every in-flight dispatch (and its receipt
// materialization) has settled. It is the drain primitive tests use after a
// Tick and the shell can use during shutdown.
func (s *Scheduler) WaitInFlight() { s.wg.Wait() }

// ── the claim loop ─────────────────────────────────────────────────────────

// Run is the level-triggered claim loop (Spec S10.7). The shell starts it as a
// process-lifetime goroutine (like the WAL and recovery loops); it claims only
// while admission is open. It returns when ctx is canceled, after waiting for
// in-flight dispatches to settle.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.poll)
	defer t.Stop()
	for {
		if s.admissionOpen() {
			if _, err := s.claimPass(ctx); err != nil && ctx.Err() == nil {
				s.logger.WarnContext(ctx, "scheduler: claim pass", "err", err)
			}
		}
		select {
		case <-ctx.Done():
			s.wg.Wait() // let in-flight dispatches settle
			return
		case <-s.nudged():
		case <-t.C:
		}
	}
}

// Enqueue is the sole ingress (Spec S16.6: fire-and-forget banned). It admits
// an existing run to the claim queue, owner-attributed: it transitions a new
// run to queued (the new→queued edge) and materializes its queue row, in ONE
// write transaction. A run already queued (e.g. a recovery fork successor)
// just gets its queue row materialized. It then nudges the claim loop.
func (s *Scheduler) Enqueue(ctx context.Context, runID string, class WorkloadClass) error {
	if class == "" {
		class = DefaultWorkloadClass
	}
	if !validClass(class) {
		return fmt.Errorf("scheduler: invalid workload class %q", class)
	}
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		r, err := s.runs.GetTx(ctx, tx, runID)
		if err != nil {
			return err
		}
		switch r.State {
		case run.StateNew:
			if _, err := s.runs.TransitionTx(ctx, tx, runID, run.StateQueued, run.TransitionOptions{
				Reason: "admitted to the claim queue (S10.7)", Actor: run.ActorPlatform,
			}); err != nil {
				return err
			}
		case run.StateQueued:
			// Already queued (recovery successor): just materialize the row.
		default:
			return fmt.Errorf("scheduler: cannot enqueue run %q in state %s (want new or queued)", runID, r.State)
		}
		return s.materializeQueueRowTx(ctx, tx, r.ID, r.UserID, class)
	})
	if err != nil {
		return err
	}
	s.doNudge()
	return nil
}

// materializeQueueRowTx inserts a queue row for a run if it has no active one
// (idempotent). Owner-attributed (queue.user_id = the run owner, 15.6).
func (s *Scheduler) materializeQueueRowTx(ctx context.Context, tx *sql.Tx, runID, userID string, class WorkloadClass) error {
	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM queue WHERE run_id = ? AND status IN (?, ?)`,
		runID, queueQueued, queueClaimed).Scan(&n); err != nil {
		return fmt.Errorf("scheduler: check queue row for %q: %w", runID, err)
	}
	if n > 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO queue (run_id, user_id, status, priority_lane, enqueued_ts)
		 VALUES (?, ?, ?, ?, ?)`,
		runID, userID, queueQueued, string(class), s.now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("scheduler: materialize queue row for %q: %w", runID, err)
	}
	return nil
}

// SettleQueuedTx settles a QUEUED run's queue row inside the CALLER's
// transaction — the queue half of the S15.6 human cancel of a run that never
// ran (4.5; P3-B6-2A OQ1). The run's queued→finalized transition and this
// settle commit together, so the claim loop can never pick up a run whose
// cancellation has already committed; and it is scoped to `queued` rows, so a
// claim that won the CAS race first is left strictly alone (that run is
// `claimed`, and the cancel verb answers 409-retry for it rather than racing
// the dispatcher).
//
// It settles the QUEUE only: no receipt is materialized, because a run that
// never ran consumed nothing and a receipt would assert a measurement that was
// never taken (Spec S10.1). The terminal transition's own run-end machinery
// owns everything else.
func (s *Scheduler) SettleQueuedTx(ctx context.Context, tx *sql.Tx, runID string) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE queue SET status = ? WHERE run_id = ? AND status = ?`,
		queueDone, runID, queueQueued); err != nil {
		return fmt.Errorf("scheduler: settle queued row for %q: %w", runID, err)
	}
	return nil
}

// claimPass runs one admission pass (Spec S10.7): materialize queue rows for
// any queued run that lacks one (recovery successors), then claim eligible
// candidates in priority order under per-(user, lane) concurrency slots. It
// returns the number of runs dispatched.
func (s *Scheduler) claimPass(ctx context.Context) (int, error) {
	if err := s.reconcileQueueRows(ctx); err != nil {
		return 0, err
	}
	cands, err := s.candidates(ctx)
	if err != nil {
		return 0, err
	}
	if len(cands) == 0 {
		return 0, nil
	}
	active, err := s.activeSlots(ctx)
	if err != nil {
		return 0, err
	}
	bgStop, err := s.settings.Float(keyBgAdmitStop)
	if err != nil {
		return 0, fmt.Errorf("scheduler: read ⚙ %s: %w", keyBgAdmitStop, err)
	}

	now := s.now()
	sortCandidates(cands, now)

	dispatched := 0
	for _, c := range cands {
		if ctx.Err() != nil {
			break
		}
		cap := s.laneCap(ctx, c.userID, c.lane)
		if active[laneKey{c.userID, c.lane}] >= cap {
			continue // lane full
		}
		if c.class == ClassBackground {
			ok, err := s.backgroundAdmits(ctx, c, bgStop)
			if err != nil {
				return dispatched, err
			}
			if !ok {
				continue // pressure headroom (S10.4)
			}
		}
		// GPU-admission POLICY hook (S10.9; the S10-policy/S12-mechanic split):
		// before dispatching onto the local-inference lane, the VRAM ledger must
		// admit the load (Spec S12.7). Wired-but-dormant — class-(a) local
		// execution has no v0 consumer, so no candidate is ever on the local lane
		// (P3/STATE binding); when one lands, a full ledger holds this back until
		// there is VRAM headroom.
		if s.gpuAdmitter != nil && s.localLane != "" && c.lane == s.localLane {
			ok, reason, err := s.gpuAdmitter.AdmitLoad(ctx, c.substrate)
			if err != nil {
				return dispatched, fmt.Errorf("scheduler: GPU admission check for run %s: %w", c.runID, err)
			}
			if !ok {
				s.logger.InfoContext(ctx, "scheduler: GPU admission refused local-lane dispatch (S10.9/S12.7)", "run", c.runID, "reason", reason)
				continue // no VRAM headroom (S12.7)
			}
		}
		r, claimed, err := s.claimOne(ctx, c)
		if err != nil {
			return dispatched, err
		}
		if !claimed {
			continue // lost the CAS race
		}
		active[laneKey{c.userID, c.lane}]++
		s.dispatch(ctx, r)
		dispatched++
	}
	return dispatched, nil
}

// claimOne CAS-claims a candidate's queue row and, if won, transitions the run
// queued→claimed and establishes its lease — all in ONE write transaction so a
// claim is atomic (Spec S10.7 CAS claiming; S02.3 discipline). The bool is
// false when another claimer won the row first (RowsAffected != 1).
func (s *Scheduler) claimOne(ctx context.Context, c candidate) (run.Run, bool, error) {
	leaseTTL := s.leaseTTL
	if leaseTTL <= 0 {
		d, err := s.settings.Duration(keyLeaseTTL)
		if err != nil {
			return run.Run{}, false, fmt.Errorf("scheduler: read ⚙ %s: %w", keyLeaseTTL, err)
		}
		leaseTTL = d
	}
	deadline := s.now().Add(leaseTTL)

	var r run.Run
	won := false
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE queue SET status = ?, claimed_by = ?, lease_deadline_ts = ?
			 WHERE queue_id = ? AND status = ?`,
			queueClaimed, claimHolder, deadline.UTC().Format(time.RFC3339Nano), c.queueID, queueQueued)
		if err != nil {
			return fmt.Errorf("scheduler: CAS-claim queue %d: %w", c.queueID, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return nil // lost the race; won stays false
		}
		if r, err = s.runs.TransitionTx(ctx, tx, c.runID, run.StateClaimed, run.TransitionOptions{
			Reason: "CAS-claimed by the scheduler (S10.7)", Actor: run.ActorPlatform,
		}); err != nil {
			return err
		}
		if err := s.runs.SetLeaseTx(ctx, tx, c.runID, claimHolder, deadline, 0); err != nil {
			return err
		}
		won = true
		return nil
	})
	if err != nil {
		return run.Run{}, false, err
	}
	return r, won, nil
}

// dispatch drives a claimed run under the scheduler (Spec S16.6: every run
// spawns under the scheduler with owner attribution). It runs in a tracked
// goroutine so ParkInFlightRuns and shutdown can account for it; on a terminal
// disposition it settles the queue row and materializes the receipt.
func (s *Scheduler) dispatch(ctx context.Context, r run.Run) {
	s.track(r.ID)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.untrack(r.ID)

		if err := s.dispatcher.Dispatch(ctx, r); err != nil {
			s.logger.WarnContext(ctx, "scheduler: dispatch", "run", r.ID, "err", err)
		}
		// Settle on the authoritative FSM state, not the dispatcher's return.
		fresh, err := s.runs.Get(context.WithoutCancel(ctx), r.ID)
		if err != nil {
			s.logger.WarnContext(ctx, "scheduler: post-dispatch read", "run", r.ID, "err", err)
			return
		}
		if run.IsTerminal(fresh.State) {
			s.settleTerminal(context.WithoutCancel(ctx), fresh)
		}
	}()
}

// SettleRun settles a run that reached a terminal state OUTSIDE a dispatch:
// queue row done + receipt materialized, one transaction, idempotent. The
// dispatch path settles automatically when Dispatch returns; runs whose
// terminal transition happens later — the intake pattern, where a dispatch
// returns at a parked gate and the card answer (API path) later completes
// the run (Spec S06.1 "gates wait"; S10.1 receipts per run-end) — are
// settled by their pipeline through this. A non-terminal run is an error
// (nothing to settle).
func (s *Scheduler) SettleRun(ctx context.Context, runID string) error {
	r, err := s.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	if !run.IsTerminal(r.State) {
		return fmt.Errorf("scheduler: settle non-terminal run %q (state %s)", runID, r.State)
	}
	s.settleTerminal(ctx, r)
	return nil
}

// settleTerminal marks the queue row done and materializes the receipt for a
// finished run, atomically in one write transaction (Spec S10.1/S10.10:
// receipts materialize per run-end). Receipt materialization is idempotent, so
// a re-dispatch or recovery pass is safe.
func (s *Scheduler) settleTerminal(ctx context.Context, r run.Run) {
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE queue SET status = ? WHERE run_id = ? AND status = ?`, queueDone, r.ID, queueClaimed); err != nil {
			return fmt.Errorf("settle queue row: %w", err)
		}
		if s.receipts != nil {
			if _, err := s.receipts.MaterializeTx(ctx, tx, r.ID); err != nil {
				return fmt.Errorf("materialize receipt: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		s.logger.WarnContext(ctx, "scheduler: settle terminal run", "run", r.ID, "err", err)
	}
}

// backgroundAdmits applies the S10.4 background admission stop: background work
// stops being admitted once consumption pressure reaches ⚙ pressure.bg_admit_stop,
// leaving the remainder as interactive headroom (3.3). With no pressure gauge
// or no declared budget the gate is inert (nothing to gate against) — honest at
// v0, where operator budgets are not yet declared.
func (s *Scheduler) backgroundAdmits(ctx context.Context, c candidate, bgStop float64) (bool, error) {
	if s.pressure == nil {
		return true, nil
	}
	g, err := s.pressure.Read(ctx, c.userID, c.lane, metering.UndeclaredBudget())
	if err != nil {
		return false, err
	}
	if !g.Applicable {
		return true, nil // no declared budget → nothing to gate (S10.4)
	}
	return g.Pressure < bgStop, nil
}

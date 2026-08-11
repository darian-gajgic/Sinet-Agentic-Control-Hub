package run

// lease.go — the lease HOLDER of Spec S02.2's lease block (P3-RW-5).
//
// S02.2 gives every run a lease {holder, wall-clock deadline, heartbeat
// cursor}; G2 Def.2 ratifies ⚙ recovery.heartbeat = 60 s beside ⚙
// recovery.dead_after = 5 min with the registry cross-check dead_after >= 2x
// heartbeat, and the S18 row for the heartbeat says in as many words "how often
// a live run refreshes its lease heartbeat". That shape only makes sense if
// something renews at heartbeat cadence and expiry tolerates a missed beat —
// but no S-section ever named the renewing component and no code performed the
// renewal, so a claimed run's lease died 5 minutes after its claim while the
// run was still advancing, and the S02.5 ladder read the dead lease as death.
//
// The duty lands here, on the platform component actively driving the run's
// in-process advance (the scheduler dispatch leg, the intake post-answer
// advance, the stage answer/resume drains, the adapter driver's pump). The
// lease is HOLDER LIVENESS; the event cursor stays PROGRESS evidence, and
// internal/recovery honours it separately. Neither is a substitute for the
// other: a quiet long engine call has a beating holder and no progress; a
// renewal gap on some path still leaves the cursor.
//
// What this deliberately is NOT: an in-memory map of live advance goroutines
// offered to the ladder as unit liveness. That is supervisor memory, and
// S02.5's kubelet rule ("re-list actual state, never trust supervisor memory")
// forbids exactly it. The DB lease row and the event cursor ARE the re-listable
// actuals — which is why the beat is a durable write and not a variable.

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// Dotted ⚙ keys the keeper consumes (owned by Spec S02; the G2 Def.2 set).
// This packet is the FIRST consumer of recovery.heartbeat.
const (
	keyHeartbeat = "recovery.heartbeat"
	keyDeadAfter = "recovery.dead_after"
)

// LeaseSettings is the keeper's view of the settings registry (Spec S01.10):
// both values are read by dotted key at every use, never cached across a
// live-apply.
type LeaseSettings interface {
	Duration(key string) (time.Duration, error)
}

// LeaseKeeperConfig assembles a LeaseKeeper. DB, Runs and Settings are
// required; Logger defaults to a discarding logger, Now to time.Now, and Tick
// to time.NewTicker.
type LeaseKeeperConfig struct {
	DB       *storage.DB
	Runs     *Store
	Settings LeaseSettings
	Logger   *slog.Logger
	// Now is the clock seam (tests). Lease deadlines are wall-clock columns
	// evaluated suspend-aware at reconcile (Spec S02.5 step 4).
	Now func() time.Time
	// Tick is the cadence seam (tests): it returns a channel firing every d and
	// the func that stops it. Nil = time.NewTicker.
	Tick func(d time.Duration) (<-chan time.Time, func())
}

// LeaseKeeper renews the lease of a run the platform is actively driving, at
// ⚙ recovery.heartbeat cadence, through the fenced RenewLeaseTx. One keeper
// serves every driving layer; each Hold is independent.
//
// A nil *LeaseKeeper is valid and inert — an unwired composition (a test, a
// layer built directly) behaves exactly as it did before this packet.
type LeaseKeeper struct {
	db       *storage.DB
	runs     *Store
	settings LeaseSettings
	logger   *slog.Logger
	now      func() time.Time
	tick     func(d time.Duration) (<-chan time.Time, func())
}

// NewLeaseKeeper validates cfg and returns the keeper.
func NewLeaseKeeper(cfg LeaseKeeperConfig) (*LeaseKeeper, error) {
	switch {
	case cfg.DB == nil:
		return nil, errors.New("run: lease keeper needs a DB")
	case cfg.Runs == nil:
		return nil, errors.New("run: lease keeper needs a run store")
	case cfg.Settings == nil:
		return nil, errors.New("run: lease keeper needs the settings registry")
	}
	k := &LeaseKeeper{
		db:       cfg.DB,
		runs:     cfg.Runs,
		settings: cfg.Settings,
		logger:   cfg.Logger,
		now:      cfg.Now,
		tick:     cfg.Tick,
	}
	if k.logger == nil {
		k.logger = slog.New(slog.DiscardHandler)
	}
	if k.now == nil {
		k.now = time.Now
	}
	if k.tick == nil {
		k.tick = func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTicker(d)
			return t.C, t.Stop
		}
	}
	return k, nil
}

// Beat performs ONE fenced renewal at the run's current generation — the
// single-shot form, for a resume that hands the run onward instead of driving
// it inline (an approved plan, a person's "resume, I was wrong"). It leaves the
// run's lease live for ⚙ recovery.dead_after, so no sweep can observe the
// un-park instant with an expired claim lease (R5).
func (k *LeaseKeeper) Beat(ctx context.Context, runID, holder string) {
	if k == nil {
		return
	}
	gen, ok := k.pin(ctx, runID)
	if !ok {
		return
	}
	k.beat(context.WithoutCancel(ctx), runID, holder, gen)
}

// Hold takes runID as the holder of its lease for the duration of an advance:
// it renews IMMEDIATELY (so the instant of taking the run is covered by
// construction, never by luck) and then every ⚙ recovery.heartbeat until the
// returned release func is called — which every caller defers, so the hold ends
// on park, terminal state, or error alike.
//
// The generation is pinned at Hold time, which is the fence: call it AFTER the
// transition that takes the run (the parked→running resume bumps the
// generation), and a takeover underneath a running hold silently stops having
// any effect. Release is idempotent and waits for the ticker goroutine to
// finish, so nothing writes after a hold ends.
//
// Errors never propagate: a heartbeat that cannot be written is logged, and the
// advance it was protecting continues. The failure mode of a missed beat is a
// run the ladder may later re-examine, not a run that dies here.
func (k *LeaseKeeper) Hold(ctx context.Context, runID, holder string) func() {
	noop := func() {}
	if k == nil {
		return noop
	}
	// The beat outlives the request context on purpose: a cancelled HTTP
	// request must not silently stop the heartbeat of work still running.
	ctx = context.WithoutCancel(ctx)
	gen, ok := k.pin(ctx, runID)
	if !ok {
		return noop
	}
	k.beat(ctx, runID, holder, gen)

	period, err := k.settings.Duration(keyHeartbeat)
	if err != nil {
		k.logger.WarnContext(ctx, "run: lease keeper cannot read ⚙ "+keyHeartbeat+" (holding without a ticker)",
			"run", runID, "err", err)
		return noop
	}
	ch, stop := k.tick(period)
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { stop() }()
		for {
			select {
			case <-done:
				return
			case <-ch:
				// ⚙ re-read every beat: a live-applied cadence change takes
				// effect on the next tick (S01.10), never at process restart.
				if next, err := k.settings.Duration(keyHeartbeat); err == nil && next != period {
					stop()
					ch, stop = k.tick(next)
					period = next
				}
				k.beat(ctx, runID, holder, gen)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			wg.Wait()
		})
	}
}

// pin reads the generation the holder will beat at.
func (k *LeaseKeeper) pin(ctx context.Context, runID string) (int64, bool) {
	r, err := k.runs.Get(ctx, runID)
	if err != nil {
		k.logger.WarnContext(ctx, "run: lease keeper cannot read the run it was asked to hold",
			"run", runID, "err", err)
		return 0, false
	}
	return r.Generation, true
}

// beat is one tiny single-row write transaction — never held across a tick.
func (k *LeaseKeeper) beat(ctx context.Context, runID, holder string, gen int64) {
	deadAfter, err := k.settings.Duration(keyDeadAfter)
	if err != nil {
		k.logger.WarnContext(ctx, "run: lease keeper cannot read ⚙ "+keyDeadAfter, "run", runID, "err", err)
		return
	}
	deadline := k.now().Add(deadAfter)
	if err := k.db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := k.runs.RenewLeaseTx(ctx, tx, runID, holder, deadline, gen)
		return err
	}); err != nil {
		k.logger.WarnContext(ctx, "run: lease heartbeat failed (the advance continues; Spec S02.2)",
			"run", runID, "generation", gen, "err", err)
	}
}

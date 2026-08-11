package run_test

// lease_test.go — P3-RW-5 acceptance battery (brief §8 T4–T5).
//
// Spec S02.2 gives every run a lease block {holder, wall-clock deadline,
// heartbeat cursor}; G2 Def.2 ratifies the cadence (⚙ recovery.heartbeat 60 s)
// alongside ⚙ recovery.dead_after (5 min) with the registry cross-check
// dead_after >= 2x heartbeat — a shape that only makes sense if something
// RENEWS at heartbeat cadence and expiry tolerates a missed beat. Nothing did.
// These pin the two halves that make it real: a fenced renewal (S02.5 step 4)
// and the keeper that drives it.

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// Ratified G2 Def.2 defaults, used only to compute expected instants.
const (
	leaseDeadAfter = 300 * time.Second
	leaseHeartbeat = 60 * time.Second
)

// driveTo walks a fresh run to state through the ratified S02.3 edges.
func driveTo(t *testing.T, s *run.Store, id string, state run.State) run.Run {
	t.Helper()
	ctx := context.Background()
	path := map[run.State][]run.State{
		run.StateNew:        {},
		run.StateQueued:     {run.StateQueued},
		run.StateClaimed:    {run.StateQueued, run.StateClaimed},
		run.StateRunning:    {run.StateQueued, run.StateClaimed, run.StateRunning},
		run.StateDraining:   {run.StateQueued, run.StateClaimed, run.StateRunning, run.StateDraining},
		run.StateParked:     {run.StateQueued, run.StateClaimed, run.StateRunning, run.StateParked},
		run.StateCompleted:  {run.StateQueued, run.StateClaimed, run.StateRunning, run.StateCompleted},
		run.StateCrashed:    {run.StateQueued, run.StateClaimed, run.StateRunning, run.StateCrashed},
		run.StateFinalized:  {run.StateQueued, run.StateFinalized},
		run.StateTombstoned: {run.StateQueued, run.StateClaimed, run.StateRunning, run.StateTombstoned},
		run.StateDiedAtGate: {run.StateQueued, run.StateClaimed, run.StateRunning, run.StateParked, run.StateDiedAtGate},
	}
	steps, ok := path[state]
	if !ok {
		t.Fatalf("no ratified path to %s", state)
	}
	r, err := s.Create(ctx, run.NewRun{ID: id, UserID: "u1"})
	if err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
	for _, to := range steps {
		if r, err = s.Transition(ctx, id, to, run.TransitionOptions{}); err != nil {
			t.Fatalf("%s →%s: %v", id, to, err)
		}
	}
	return r
}

// renew calls the fenced renewal in its own write transaction.
func renew(t *testing.T, db *storage.DB, s *run.Store, runID, holder string, deadline time.Time, gen int64) bool {
	t.Helper()
	applied := false
	err := db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		applied, err = s.RenewLeaseTx(context.Background(), tx, runID, holder, deadline, gen)
		return err
	})
	if err != nil {
		t.Fatalf("RenewLeaseTx %s: %v", runID, err)
	}
	return applied
}

func getRun(t *testing.T, s *run.Store, id string) run.Run {
	t.Helper()
	r, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get %s: %v", id, err)
	}
	return r
}

// newestSeq returns the run's newest event_seq — what a renewal stamps into the
// heartbeat cursor (Spec S02.2: "the last event_seq the holder asserted").
func newestSeq(t *testing.T, db *storage.DB, runID string) int64 {
	t.Helper()
	var seq int64
	if err := db.QueryRowContext(context.Background(),
		`SELECT COALESCE(MAX(event_seq), 0) FROM run_events WHERE run_id = ?`, runID).Scan(&seq); err != nil {
		t.Fatalf("newest seq of %s: %v", runID, err)
	}
	return seq
}

// ── T4 ──────────────────────────────────────────────────────────────────────

// TestRenewLeaseTx_Fenced sweeps the fence rather than sampling it: renewal
// applies ONLY at the holder's own generation and only while the run is in a
// state a holder can be driving ({claimed, running, draining}); everything else
// is a SILENT no-op — never an error that would kill a healthy path — so a
// zombie holder of a superseded generation cannot keep a run alive
// (Spec S02.5 step 4, P-T02-3).
func TestRenewLeaseTx_Fenced(t *testing.T) {
	db, _, store := newStore(t)
	t0 := time.Now().UTC().Truncate(time.Millisecond)

	drivable := map[run.State]bool{
		run.StateClaimed:  true,
		run.StateRunning:  true,
		run.StateDraining: true,
	}
	states := []run.State{
		run.StateNew, run.StateQueued, run.StateClaimed, run.StateRunning,
		run.StateDraining, run.StateParked, run.StateCompleted, run.StateCrashed,
		run.StateFinalized, run.StateTombstoned, run.StateDiedAtGate,
	}
	for _, st := range states {
		t.Run(string(st), func(t *testing.T) {
			id := "r-" + string(st)
			r := driveTo(t, store, id, st)

			// The claim-time write is SetLeaseTx and its semantics are
			// unchanged by this packet.
			err := db.WriteTx(context.Background(), func(tx *sql.Tx) error {
				return store.SetLeaseTx(context.Background(), tx, id, "claimer", t0.Add(leaseDeadAfter), 0)
			})
			if err != nil {
				t.Fatalf("SetLeaseTx: %v", err)
			}
			before := getRun(t, store, id)
			if before.LeaseHolder != "claimer" || !before.LeaseDeadline.Equal(t0.Add(leaseDeadAfter)) || before.HeartbeatSeq != 0 {
				t.Fatalf("claim lease = %q %v hb %d, want claimer / +dead_after / 0",
					before.LeaseHolder, before.LeaseDeadline, before.HeartbeatSeq)
			}

			// At the holder's own generation.
			want := t0.Add(2 * leaseDeadAfter)
			applied := renew(t, db, store, id, "holder", want, r.Generation)
			after := getRun(t, store, id)
			if applied != drivable[st] {
				t.Fatalf("renew on %s applied = %v, want %v", st, applied, drivable[st])
			}
			if drivable[st] {
				if !after.LeaseDeadline.Equal(want) {
					t.Errorf("deadline = %v, want %v (the renewal extends the lease)", after.LeaseDeadline, want)
				}
				if after.LeaseHolder != "holder" {
					t.Errorf("holder = %q, want holder (the driving component holds the run)", after.LeaseHolder)
				}
				if seq := newestSeq(t, db, id); after.HeartbeatSeq != seq {
					t.Errorf("heartbeat cursor = %d, want %d (the newest event_seq the holder asserted)", after.HeartbeatSeq, seq)
				}
			} else {
				if after.LeaseHolder != before.LeaseHolder || !after.LeaseDeadline.Equal(before.LeaseDeadline) ||
					after.HeartbeatSeq != before.HeartbeatSeq {
					t.Errorf("renew on %s changed the lease row %+v → %+v — a non-drivable state must be a silent no-op",
						st, before, after)
				}
			}

			// Stale and ahead generations are both fenced out, in every state.
			base := getRun(t, store, id)
			for _, gen := range []int64{r.Generation - 1, r.Generation + 1} {
				if applied := renew(t, db, store, id, "zombie", t0.Add(time.Hour), gen); applied {
					t.Fatalf("renew at generation %d applied on a run at generation %d — the fence is open", gen, r.Generation)
				}
			}
			if now := getRun(t, store, id); now.LeaseHolder != base.LeaseHolder ||
				!now.LeaseDeadline.Equal(base.LeaseDeadline) || now.HeartbeatSeq != base.HeartbeatSeq {
				t.Errorf("a fenced renewal still wrote: %+v → %+v", base, now)
			}
		})
	}
}

// TestRenewLeaseTx_AppendsNoEvent keeps the S02.2 property that makes the whole
// design honest: a heartbeat is holder liveness, not progress. If renewal
// appended an event, every idle holder would forge its own aliveness.
func TestRenewLeaseTx_AppendsNoEvent(t *testing.T) {
	db, elog, store := newStore(t)
	r := driveTo(t, store, "r1", run.StateRunning)
	before := countEvents(t, elog, "r1")
	for i := range 3 {
		if !renew(t, db, store, "r1", "holder", time.Now().Add(time.Duration(i+1)*leaseDeadAfter), r.Generation) {
			t.Fatal("renewal did not apply on a running run")
		}
	}
	if after := countEvents(t, elog, "r1"); after != before {
		t.Fatalf("run events %d → %d: renewal appended %d event(s) — a heartbeat is not progress",
			before, after, after-before)
	}
}

// ── T5 ──────────────────────────────────────────────────────────────────────

// fakeTicker is the LeaseKeeper's cadence seam under test.
type fakeTicker struct {
	c        chan time.Time
	period   time.Duration
	stops    int
	requests int
}

func (f *fakeTicker) tick(d time.Duration) (<-chan time.Time, func()) {
	f.period = d
	f.requests++
	return f.c, func() { f.stops++ }
}

// probeSettings signals every ⚙ read so the test can tell when a beat is in
// flight without sleeping.
type probeSettings struct {
	reg  *settings.Registry
	seen chan string
}

func (p *probeSettings) Duration(key string) (time.Duration, error) {
	d, err := p.reg.Duration(key)
	select {
	case p.seen <- key:
	default:
	}
	return d, err
}

// awaitRead blocks until the keeper reads key (proving a beat is in flight).
func (p *probeSettings) awaitRead(t *testing.T, key string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case got := <-p.seen:
			if got == key {
				return
			}
		case <-deadline:
			t.Fatalf("the keeper never read ⚙ %s", key)
		}
	}
}

// waitDeadline polls until the run's lease deadline reaches want.
func waitDeadline(t *testing.T, s *run.Store, runID string, want time.Time) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last time.Time
	for time.Now().Before(deadline) {
		r := getRun(t, s, runID)
		last = r.LeaseDeadline
		if last.Equal(want) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("lease deadline = %v, want %v (the holder's heartbeat never landed)", last, want)
}

// TestLeaseKeeper_HeartbeatCadence pins the holder half of R3: the component
// driving a run renews its lease immediately on taking it and every ⚙
// recovery.heartbeat after, each renewal setting deadline = now + ⚙
// recovery.dead_after; renewals stop when the hold is released; and a mid-hold
// generation bump (a fork or a park+resume underneath the holder) makes every
// further renewal inert — T4's fence, observed end to end.
func TestLeaseKeeper_HeartbeatCadence(t *testing.T) {
	ctx := context.Background()
	db, _, store := newStore(t)
	reg := settings.New()
	probe := &probeSettings{reg: reg, seen: make(chan string, 256)}
	ticker := &fakeTicker{c: make(chan time.Time, 1)}
	clock := time.Now().UTC().Truncate(time.Millisecond)

	keeper, err := run.NewLeaseKeeper(run.LeaseKeeperConfig{
		DB:       db,
		Runs:     store,
		Settings: probe,
		Logger:   slog.New(slog.DiscardHandler),
		Now:      func() time.Time { return clock },
		Tick:     ticker.tick,
	})
	if err != nil {
		t.Fatalf("NewLeaseKeeper: %v", err)
	}

	r := driveTo(t, store, "r1", run.StateClaimed)

	// 1. Immediately on taking the run — the un-park instant is covered by
	//    construction, never by luck (R5).
	release := keeper.Hold(ctx, "r1", "test-holder")
	got := getRun(t, store, "r1")
	if !got.LeaseDeadline.Equal(clock.Add(leaseDeadAfter)) {
		t.Fatalf("deadline after Hold = %v, want %v (renewal starts immediately)", got.LeaseDeadline, clock.Add(leaseDeadAfter))
	}
	if got.LeaseHolder != "test-holder" {
		t.Fatalf("holder = %q, want test-holder", got.LeaseHolder)
	}
	if seq := newestSeq(t, db, "r1"); got.HeartbeatSeq != seq {
		t.Fatalf("heartbeat cursor = %d, want %d", got.HeartbeatSeq, seq)
	}
	if ticker.period != leaseHeartbeat {
		t.Fatalf("tick period = %v, want ⚙ recovery.heartbeat (%v)", ticker.period, leaseHeartbeat)
	}

	// 2. One renewal per tick, each extending to now + ⚙ recovery.dead_after.
	for range 2 {
		clock = clock.Add(leaseHeartbeat)
		ticker.c <- clock
		waitDeadline(t, store, "r1", clock.Add(leaseDeadAfter))
	}

	// 3. The fence, end to end: a takeover bumps the generation underneath the
	//    holder, and every further beat is inert.
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := store.BumpGenerationTx(ctx, tx, "r1")
		return err
	}); err != nil {
		t.Fatalf("bump generation: %v", err)
	}
	fenced := getRun(t, store, "r1")
	if fenced.Generation != r.Generation+1 {
		t.Fatalf("generation = %d, want %d", fenced.Generation, r.Generation+1)
	}
	clock = clock.Add(leaseHeartbeat)
	ticker.c <- clock
	probe.awaitRead(t, "recovery.dead_after") // the beat is in flight
	release()                                 // …and release waits for it to finish
	if now := getRun(t, store, "r1"); !now.LeaseDeadline.Equal(fenced.LeaseDeadline) {
		t.Fatalf("deadline moved %v → %v after the generation bump — a zombie holder kept a superseded run alive",
			fenced.LeaseDeadline, now.LeaseDeadline)
	}

	// 4. Released: the ticker is stopped and nothing can renew any more.
	if ticker.stops != ticker.requests {
		t.Fatalf("ticker stops = %d, requests = %d — the hold leaked its ticker", ticker.stops, ticker.requests)
	}
	stopped := getRun(t, store, "r1")
	select {
	case ticker.c <- clock.Add(leaseHeartbeat):
	default:
	}
	release() // idempotent
	if now := getRun(t, store, "r1"); !now.LeaseDeadline.Equal(stopped.LeaseDeadline) {
		t.Fatalf("deadline moved %v → %v after release — the holder kept beating on a run it no longer drives",
			stopped.LeaseDeadline, now.LeaseDeadline)
	}
}

// TestLeaseKeeper_NilIsInert keeps every unwired composition (tests, layers a
// packet has not reached) working exactly as before: a nil keeper holds
// nothing and beats nothing.
func TestLeaseKeeper_NilIsInert(t *testing.T) {
	var k *run.LeaseKeeper
	release := k.Hold(context.Background(), "r1", "nobody")
	if release == nil {
		t.Fatal("a nil keeper must still return a release func")
	}
	release()
	k.Beat(context.Background(), "r1", "nobody")
}

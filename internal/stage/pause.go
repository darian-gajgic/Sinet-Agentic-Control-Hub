package stage

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// pause.go — the in-flight half of S10.4's pause-my-automation (P3-B6-2B, R12;
// OQ5(ii)).
//
// S10.4 says a pause "parks in-flight runs at their next checkpoint". The
// checkpoint boundary the spec means is a D7 paid-call boundary, and no
// per-paid-call gate exists yet — so the v0 approximation, recorded as such, is
// the STAGE-SESSION boundary: the execute leg checks the flag between sessions
// and parks before starting the next one. A stage boundary IS a checkpoint
// boundary (CONVENTIONS §14 reading 8), so the approximation is coarser than
// S10.8's eventual gate but never wrong about where it can safely stop: it never
// interrupts a paid call, and the work that has already happened is on the
// ledger and the checkpoints exactly as if the leg had continued.
//
// ── This is a PARK, and the distinction is the whole point ──────────────────
//
// A pause parks; it never kills. Nothing here reaches the S02.3 cancel
// machinery, nothing here transitions a run to any terminal state, and nothing
// queued is touched at all. Paused work is preserved work (P-T08-4): the queue
// keeps its rows, the parked run keeps its state, and resuming leaves both
// exactly where they were. The NO-AUTO-KILL invariant (S14.4 / G1 D1.3;
// CONVENTIONS §31) is untouched, and the cancel-reachability wall still names
// exactly three files, none of them this one.
//
// ── The remaining S10.8 seam, named ─────────────────────────────────────────
//
// pausedPerPaidCallGate is where the per-paid-call budget/pause gate of S10.8
// would consult the same flag — inside the adapter Driver's checkpoint loop,
// between paid calls rather than between sessions. It is post-B6/hardening
// scope and is deliberately absent rather than half-built: a gate that stopped
// mid-call would either abandon a paid call's result or fake a checkpoint, and
// both are worse than parking one session later.
const pausedPerPaidCallGate = "S10.8 per-paid-call budget/pause gate — post-B6 hardening; the v0 approximation is the stage-session boundary below (OQ5(iii))"

// errPausedPark unwinds a leg that parked at a session boundary. It is NOT a
// failure and must never be treated as one: the leg's caller returns cleanly, so
// no crash corpse is filed and the recovery ladder is never told to fork work
// the person just asked to stop.
var errPausedPark = errors.New("stage: leg parked at a stage-session boundary (automation paused, S10.4)")

// paused reports whether the run's OWNER has paused their automation. With no
// pause seam wired (tests, and any process composed without the S10.4 store)
// nobody is paused, which is the pre-0017 behavior exactly.
func (s *Skeleton) paused(ctx context.Context, userID string) bool {
	if s.cfg.Paused == nil {
		return false
	}
	p, err := s.cfg.Paused(ctx, userID)
	if err != nil {
		// A failed read must not park work: the honest failure direction for a
		// switch that stops work is to keep working and say so, because the
		// alternative parks a healthy run on a database hiccup.
		s.logger().Warn("stage: pause-flag read failed; continuing the leg", "run.owner", userID, "err", err)
		return false
	}
	return p
}

// pauseParkPoint is the stage-session boundary check. It reports whether the run
// was parked; a parked leg returns cleanly, because a pause is not a failure and
// a crash corpse would send the recovery ladder to fork work the person just
// asked to stop.
func (s *Skeleton) pauseParkPoint(ctx context.Context, r run.Run, nextStage string) bool {
	if !s.paused(ctx, r.UserID) {
		return false
	}
	if _, err := s.cfg.Runs.Transition(ctx, r.ID, run.StateParked, run.TransitionOptions{
		Reason: "automation paused by " + r.UserID + ": parked at the stage-session boundary (S10.4; the v0 checkpoint approximation)",
		// The PLATFORM takes the edge; the AUTHORITY is the person's standing
		// switch, which the detail names. Attributing the transition to the
		// person would claim they acted at this instant, and they did not.
		Actor:  run.ActorPlatform,
		Detail: pauseParkDetail(r.UserID, nextStage),
	}); err != nil {
		// A refused park is a lost race (the leg is ending anyway, or the run
		// already moved): the leg continues and the next boundary tries again.
		s.logger().Warn("stage: pause park refused", "run", r.ID, "err", err)
		return false
	}
	s.logger().Info("stage: automation paused — run parked at the stage-session boundary (S10.4)",
		"run", r.ID, "owner", r.UserID, "next_stage", nextStage)
	return true
}

func pauseParkDetail(owner, nextStage string) json.RawMessage {
	b, err := json.Marshal(struct {
		Cause     string `json:"cause"`
		Owner     string `json:"owner"`
		NextStage string `json:"next_stage"`
		Seam      string `json:"seam"`
	}{"automation paused (S10.4)", owner, nextStage, pausedPerPaidCallGate})
	if err != nil {
		return nil
	}
	return b
}

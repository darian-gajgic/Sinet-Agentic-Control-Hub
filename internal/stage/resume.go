package stage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// resume.go — S14.4's first-class "resume — I was wrong" (P3-B6-2B, R15).
//
// The watchdog's posture is pause-and-flag, never auto-kill (S14.4 / G1 D1.3):
// when a tier flags a run it PARKS it and raises a card. The person reading that
// card has exactly two honest answers — suppress the rule (it was noise) or
// resume the run (the flag was right about the signal and wrong about the work).
// This is the second one, and it is why the landed open-flag derivation already
// treats a `run.state_changed` to=running as a supersession: the resume was
// always the designed release, the transport for it just had not landed.
//
// So this verb takes the ratified S02.3 resume edge and nothing else. The
// generation bump is the edge's own (CONVENTIONS §8), which is what renders any
// stale fencing inert; the projection needs no change at all.
//
// ── What it deliberately does NOT do ────────────────────────────────────────
//
// It does not resume a run parked on an OPEN ASK. For that class of park the
// ANSWER is the resume (CONVENTIONS §16, S07.7: escalations are structured,
// resumable asks and answering them resumes the pipeline in place). Flipping the
// state underneath an open card would leave a card nobody can act on attached to
// a run nobody is driving, so the verb refuses and points at the ask instead.
// This verb is the ASK-LESS park's release: watchdog, operator, maintenance.

// ErrResumeNotParked refuses a resume of a run that is not parked. The person
// re-reads and decides again; nothing is forced onto the FSM.
var ErrResumeNotParked = fmt.Errorf("stage: only a parked run can be resumed")

// ResumeOpenAskError refuses the resume of a run parked on an open ask and
// carries the ask id, because the useful answer is not "no" — it is "answer this
// card, and answering it is the resume".
type ResumeOpenAskError struct {
	RunID string
	AskID string
}

func (e *ResumeOpenAskError) Error() string {
	return fmt.Sprintf("stage: run %s is waiting on card %s — answering that card is what starts it again", e.RunID, e.AskID)
}

// ResumeOutcome is one run's resume disposition, made legible: which edge was
// taken and the generation the resume fenced the run at.
type ResumeOutcome struct {
	RunID string `json:"run_id"`
	From  string `json:"from"`
	To    string `json:"to"`
	// Applied is always true on success; it is carried so the shape reads the
	// same way the cancel outcome does.
	Applied bool `json:"applied"`
	// Generation is the run's generation AFTER the resume — the bump the edge
	// takes (§8), which is what makes a double-resume inert (P-T02-3).
	Generation int64  `json:"generation"`
	Detail     string `json:"detail"`
}

// ResumeRun takes the ratified parked→running edge for a HUMAN actor. The
// transport has already resolved owner scope (S01.9); actor is the
// authenticated person, and it rides the transition so the record says a person
// released this run rather than the platform.
func (s *Skeleton) ResumeRun(ctx context.Context, actor, runID string) (ResumeOutcome, error) {
	r, err := s.cfg.Runs.Get(ctx, runID)
	if err != nil {
		return ResumeOutcome{}, err
	}
	if r.State != run.StateParked {
		return ResumeOutcome{}, fmt.Errorf("%w: run %s is %s", ErrResumeNotParked, runID, r.State)
	}
	askID, err := s.openAskOf(ctx, runID)
	if err != nil {
		return ResumeOutcome{}, err
	}
	if askID != "" {
		return ResumeOutcome{}, &ResumeOpenAskError{RunID: runID, AskID: askID}
	}
	resumed, err := s.cfg.Runs.Transition(ctx, runID, run.StateRunning, run.TransitionOptions{
		// S14.4's "resume, I was wrong": a person releases the park.
		Reason: "resumed by " + actor + " — a person decided the work should carry on",
		Actor:  actor,
		Detail: resumeDetail(actor),
	})
	if err != nil {
		return ResumeOutcome{}, raced(err)
	}
	// The person released the park; whatever picks the run up next is not this
	// call. The resume must still leave a LIVE lease behind it or the next
	// recovery sweep meets an un-parked run holding an expired claim lease and
	// reads it as death (P3-RW-5 R5). One fenced beat at the resumed
	// generation buys ⚙ recovery.dead_after of cover. If nothing ever drives
	// the run, that one lease lapses and the ladder reaps and forks it within
	// the S02.5 bound — a bounded self-heal through a queued successor, not a
	// kill of live work (live work renews via its driver's LeaseKeeper).
	s.leases.Beat(ctx, runID, leaseHolderStage)
	s.logger().Info("stage: run resumed by a person (S14.4)", "run", runID, "actor", actor,
		"generation", resumed.Generation)
	return ResumeOutcome{
		RunID: runID, From: string(run.StateParked), To: string(run.StateRunning),
		Applied: true, Generation: resumed.Generation,
		// S14.4: resuming supersedes the run's open watchdog flags.
		Detail: "resumed: the work is moving again, and the warnings that stopped it no longer stand",
	}, nil
}

// openAskOf returns the run's open ask id, or "" when it is parked on nothing
// answerable.
func (s *Skeleton) openAskOf(ctx context.Context, runID string) (string, error) {
	var askID string
	err := s.cfg.DB.QueryRowContext(ctx,
		`SELECT ask_id FROM asks WHERE run_id = ? AND answered_ts IS NULL
		  ORDER BY observed_ts, ask_id LIMIT 1`, runID).Scan(&askID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("stage: read open asks of %q: %w", runID, err)
	}
	return askID, nil
}

func resumeDetail(actor string) json.RawMessage {
	b, err := json.Marshal(struct {
		Cause string `json:"cause"`
		Actor string `json:"actor"`
	}{"human resume (S14.4)", actor})
	if err != nil {
		return nil
	}
	return b
}

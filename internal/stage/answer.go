package stage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// Answering non-intake asks (Spec S07.7: escalations are structured,
// RESUMABLE asks — answering resumes the pipeline in place, 4.3). The
// verb/quality semantics live in internal/verify (answer.go); this file
// owns the run-FSM choreography around them, mirroring the intake answer
// precedent (intake/answer.go closeAndResume):
//
//   - the ask closes and the parked run acts in ONE transaction;
//   - the resume edge parked→running bumps the generation (Spec S02.5 step
//     4; CONVENTIONS §8), so superseded stage sessions are fenced;
//   - cancel takes the ratified S02.3 mapping (CONVENTIONS §14 reading 9):
//     parked→finalized, finalize-with-card;
//   - the resumed drain runs inline in the answer request (the walking-
//     skeleton continuation pattern — Surface.Answer→afterIntake drives
//     intake stages the same way) and lands on verifyTerminal: completed on
//     SHIP, re-parked on a recurrent card.

// AnswerVerifyAsk applies one S07.7 card answer on a parked verify run and
// returns the card's task id for the caller's task view.
func (s *Skeleton) AnswerVerifyAsk(ctx context.Context, actor, askID string, raw json.RawMessage) (string, error) {
	card, owner, err := verify.LoadOpenCard(ctx, s.cfg.DB, askID)
	if err != nil {
		return "", err
	}
	if actor != owner {
		return card.TaskID, fmt.Errorf("%w: %q", verify.ErrNotRequester, actor)
	}
	ans, err := verify.DecodeAnswer(raw)
	if err != nil {
		return card.TaskID, err
	}
	if err := verify.ValidateAnswer(card, ans); err != nil {
		return card.TaskID, err
	}
	r, err := s.cfg.Runs.Get(ctx, card.RunID)
	if err != nil {
		return card.TaskID, err
	}
	if rl, ok := runRole(r.ID); !ok || rl != roleVerify {
		return card.TaskID, fmt.Errorf("%w: run %q is not a verify role run", verify.ErrNotResumable, r.ID)
	}
	if r.State != run.StateParked {
		return card.TaskID, fmt.Errorf("%w: run %q is %s (a card issued before the park-on-card build has no parked run to resume)",
			verify.ErrNotResumable, r.ID, r.State)
	}

	switch ans.Choice {
	case verify.VerbCancel:
		return card.TaskID, s.answerCancel(ctx, actor, askID, card, ans, raw)
	case verify.VerbRetry: // infrastructure card only — ValidateAnswer enforces it.
		return card.TaskID, s.answerInfraRetry(ctx, actor, askID, card, ans, raw)
	case verify.VerbAcceptBestEffort:
		return card.TaskID, s.answerAccept(ctx, actor, askID, card, ans, raw)
	default: // VerbReviseWithGuidance — ValidateAnswer admitted nothing else.
		return card.TaskID, s.answerRevise(ctx, actor, askID, card, ans, raw)
	}
}

// answerInfraRetry applies `retry` on a verification-infrastructure card
// (P3-RW-14 R4/OQ2): the missing screen now exists, so the drain runs again on
// the SAME run — resumed in place (parked→running, generation bumped, the one
// ratified resume edge) and re-entered through the ordinary drain path. If the
// thing is STILL missing, the drain parks on a fresh card and says so again:
// the door never closes onto silence.
func (s *Skeleton) answerInfraRetry(ctx context.Context, actor, askID string, card verify.Card, ans verify.Answer, raw json.RawMessage) error {
	if _, err := s.cfg.Ledger.RecordDecision(ctx, card.RunID, ledger.AuthorHuman, actor, "verify",
		"requester retried verification at the infrastructure card"+noteSuffix(ans),
		"retry (S07.7): an answer is an explicit human grant of a fresh bounded budget; the drain re-runs with the missing screen supplied (S07.2/S02.5)", 0); err != nil {
		return err
	}
	err := s.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := verify.CloseAnsweredTx(ctx, tx, askID, raw, s.now()); err != nil {
			return err
		}
		_, err := s.cfg.Runs.TransitionTx(ctx, tx, card.RunID, run.StateRunning, run.TransitionOptions{
			// 4.3 / S07.7: answering resumes the run in place.
			Reason: "you answered the card, so checking runs again", Actor: run.ActorPlatform,
		})
		return err
	})
	if err != nil {
		return err
	}
	r, err := s.cfg.Runs.Get(ctx, card.RunID)
	if err != nil {
		return err
	}
	// The re-entered drain runs inline in this request, so the stage layer holds
	// the run's lease across it (Spec S02.2; P3-RW-5 R3) — taken after the resume
	// commit so it beats at the bumped generation.
	defer s.leases.Hold(ctx, card.RunID, leaseHolderStage)()
	// Revision 1: the infrastructure card means the drain never judged anything,
	// so there is no later revision to resume from.
	return s.runVerifyDrain(ctx, r, 1)
}

// answerCancel applies the ratified cancel mapping (S02.3 via CONVENTIONS
// §14 reading 9): the parked run finalizes with its card.
func (s *Skeleton) answerCancel(ctx context.Context, actor, askID string, card verify.Card, ans verify.Answer, raw json.RawMessage) error {
	// The human decision records first (the ledger opens its own
	// transaction; the single-connection pool forbids nesting — the intake
	// precedent). A crash before the closing tx leaves the ask open and a
	// re-answer re-records: decisions are append-only facts.
	if _, err := s.cfg.Ledger.RecordDecision(ctx, card.RunID, ledger.AuthorHuman, actor, "verify",
		"requester cancelled at the "+string(card.Category)+" card"+noteSuffix(ans),
		"cancel is always available (4.5); parked→finalized, finalize-with-card (S02.3; CONVENTIONS §14 reading 9)", 0); err != nil {
		return err
	}
	err := s.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := verify.CloseAnsweredTx(ctx, tx, askID, raw, s.now()); err != nil {
			return err
		}
		// 15.6 attribution: the human who answered the card is the actor of the
		// transition their answer caused, and the structured detail carries the
		// same name plus the card it was answered at (P3-RW-18 D1-R1). Before
		// this the transition said `platform` and carried no detail, so the
		// only record of who cancelled lived in the ledger entry above and the
		// task detail could not serve it.
		//
		// The card's own note IS the reason channel (P3-RW-19 R6): it keeps
		// riding the ledger text above, and it now also lands where a reader
		// can find the motive without scraping a sentence.
		_, err := s.cfg.Runs.TransitionTx(ctx, tx, card.RunID, run.StateFinalized, run.TransitionOptions{
			Reason: "verification cancelled at the card (4.5): finalize-with-card", Actor: actor,
			Detail: run.CancelDetail(actor, false, askID, ans.Note),
		})
		return err
	})
	if err != nil {
		return err
	}
	s.setKanban(ctx, card.TaskID, "cancelled")
	s.settle(ctx, card.RunID)
	return nil
}

// answerAccept applies accept_best_effort: the run resumes in place
// (parked→running, generation bumped — the one ratified resume edge, kept
// uniform across the verbs so the fence holds for all of them) and
// completes with the acceptance recorded. The quality record stays honest:
// no SHIP verdict is minted and work items stay done_unverified (Spec
// S05.1/S07.11 — SetVerified is the only path to verified); the effects
// gate is untouched (Spec S07.1).
func (s *Skeleton) answerAccept(ctx context.Context, actor, askID string, card verify.Card, ans verify.Answer, raw json.RawMessage) error {
	best := card.BestEffort
	if best == "" {
		best = "the last recorded round's revision"
	}
	if _, err := s.cfg.Ledger.RecordDecision(ctx, card.RunID, ledger.AuthorHuman, actor, "verify",
		"requester accepted the best-effort state under the "+string(card.Category)+" card: "+best+noteSuffix(ans),
		"accept_best_effort (S07.7): quality record stays honest — items remain done_unverified, no SHIP verdict, effects gate untouched (S07.1/S07.11)", 0); err != nil {
		return err
	}
	err := s.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := verify.CloseAnsweredTx(ctx, tx, askID, raw, s.now()); err != nil {
			return err
		}
		if _, err := s.cfg.Runs.TransitionTx(ctx, tx, card.RunID, run.StateRunning, run.TransitionOptions{
			// 4.3 / S07.7: answering resumes the run in place.
			Reason: "you answered the card, so the task picks up exactly where it left off", Actor: run.ActorPlatform,
		}); err != nil {
			return err
		}
		_, err := s.cfg.Runs.TransitionTx(ctx, tx, card.RunID, run.StateCompleted, run.TransitionOptions{
			// S07.7: the requester's accept-best-effort answer resolves it.
			Reason: "you accepted the work as it stands, so checking is finished here", Actor: run.ActorPlatform,
		})
		return err
	})
	if err != nil {
		return err
	}
	s.setKanban(ctx, card.TaskID, "done")
	s.settle(ctx, card.RunID)
	return nil
}

// answerRevise applies revise_with_guidance: close the ask + resume the run
// (generation bump) in one transaction, then re-enter the S07.6 drain with
// the guidance — inline in the answer request — and land the outcome on
// verifyTerminal (completed on SHIP; re-parked on a recurrent card with the
// full round history).
func (s *Skeleton) answerRevise(ctx context.Context, actor, askID string, card verify.Card, ans verify.Answer, raw json.RawMessage) error {
	pinRev, pinSHA, err := card.BestEffortPin()
	if err != nil {
		return err
	}
	in, err := s.verifyInput(ctx, card.RunID, card.TaskID, pinRev)
	if err != nil {
		return err
	}
	if got := in.Deliverable.SHA256(); got != pinSHA {
		return fmt.Errorf("stage: artifact store rev%d sha %s does not match the card's best-effort pin %s — refusing to rework unpinned content",
			pinRev, got, pinSHA)
	}
	points := make([]string, 0, len(ans.Guidance))
	for i, g := range ans.Guidance {
		points = append(points, fmt.Sprintf("[G%d] %s", i+1, g.Text))
	}
	if _, err := s.cfg.Ledger.RecordDecision(ctx, card.RunID, ledger.AuthorHuman, actor, "verify",
		"requester chose revise_with_guidance at the "+string(card.Category)+" card: "+strings.Join(points, "; ")+noteSuffix(ans),
		"guidance becomes numbered findings feeding a FRESH bounded rework round (S07.6/S07.7)", 0); err != nil {
		return err
	}
	err = s.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := verify.CloseAnsweredTx(ctx, tx, askID, raw, s.now()); err != nil {
			return err
		}
		_, err := s.cfg.Runs.TransitionTx(ctx, tx, card.RunID, run.StateRunning, run.TransitionOptions{
			// 4.3 / S07.7: answering resumes the run in place.
			Reason: "you answered the card with guidance, so the task picks up exactly where it left off", Actor: run.ActorPlatform,
		})
		return err
	})
	if err != nil {
		return err
	}
	// The resumed drain runs inline in this request, so the stage layer HOLDS
	// the run's lease across it (Spec S02.2 at ⚙ recovery.heartbeat; P3-RW-5
	// R3). Taken after the resume transaction, so it beats at the bumped
	// generation, and immediately, so the un-park instant is covered (R5).
	defer s.leases.Hold(ctx, card.RunID, leaseHolderStage)()
	v, err := s.newVerifier(ctx, in.Deliverable.Domain, card.TaskID)
	if err != nil {
		// Including a verification-infrastructure refusal: this leg's card
		// already exists and was answered, so the honest posture is the landed
		// one — a corpse for the ladder. A deterministic refusal that survives
		// the ladder's attempts ends at the tombstone-review card (R1), so the
		// door is still there either way.
		s.crash(ctx, card.RunID, err.Error())
		return err
	}
	out, err := v.ResumeWithGuidance(ctx, in, card, ans.Comments())
	if err != nil {
		// The resumed drain died mid-flight: leave a classifiable corpse
		// for the recovery ladder (the dispatch-leg posture). The answered
		// ask stays closed; a recovered lineage re-verifies and terminates
		// in its own card — no finding dies silently (Spec S07.7).
		s.crash(ctx, card.RunID, "resumed verification drain: "+err.Error())
		return fmt.Errorf("stage: resumed verify: %w", err)
	}
	if err := s.verifyTerminal(ctx, card.RunID, card.TaskID, out); err != nil {
		// The rework rounds are paid for and their outcome could not be recorded:
		// the same posture the dead drain takes, so the run stays classifiable
		// instead of stranding on a verdict nobody landed (P3-RW-10 R5b).
		s.crash(ctx, card.RunID, "resumed verification terminal record: "+err.Error())
		return fmt.Errorf("stage: resumed verify terminal: %w", err)
	}
	return nil
}

func noteSuffix(ans verify.Answer) string {
	if strings.TrimSpace(ans.Note) == "" {
		return ""
	}
	return " — note: " + ans.Note
}

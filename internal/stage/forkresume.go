package stage

// Fork-from-last-checkpoint, at step granularity (Spec S02.5 step 2): a
// recovery successor picks the execution up at the first plan step its lineage
// did not finish, on the working copy as that step's predecessor left it.
//
// The resume point is DERIVED FROM THE RECORD, never carried: S02.5's own rule
// is to re-list actual state rather than trust a supervisor's memory, and the
// record is the task ledger, which is task-scoped and cumulative — a fork of a
// fork reads the same document. The plan is the order authority (Spec S06.6:
// each step carries its own Done-when, which is what makes a step the unit of
// completion); a ledger item the plan does not name is not a step and is
// ignored.
//
// A step counts as finished only when it is both recorded done AND carries the
// close snapshot the successor would start on (Spec S05.1 §4 evidence_ref, Spec
// S13.5 snapshot commits at stage boundaries). Done without evidence is an
// honest half-record — a workspace-less run, or a snapshot that failed — and
// the only sound reading of it is to run the step again: there is no tree to
// put the successor on.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// resumePoint is where an execute leg starts driving, and everything the two
// runs need to say about it in words a requester reads.
type resumePoint struct {
	// Index is the count of leading plan steps already finished — the index the
	// step loop starts at. It equals the step count when nothing is left.
	Index int
	// Step is the first step to drive, "" when every step is finished.
	Step string
	// Completed names the finished steps, in plan order.
	Completed []string
	// Snapshot is the tree the successor starts on: the last finished step's
	// close snapshot, or the attempt's base when no step finished. "" is an
	// honest absence — the run has no working copy to restore.
	Snapshot string
	// Parent is the run this one supersedes, and ParentCause how that run said
	// it ended (Spec S02.5 step 3; the supersession edge is runs.parent_run_id).
	Parent      string
	ParentCause string
}

// resumeFrom derives a recovery successor's resume point from the task ledger
// and the approved plan's step order.
func (s *Skeleton) resumeFrom(ctx context.Context, r run.Run, steps []intake.Step) (resumePoint, error) {
	rp := resumePoint{Parent: r.ParentRunID}
	doc, found, err := s.cfg.Ledger.Current(ctx, r.TaskID)
	if err != nil {
		return rp, fmt.Errorf("stage: resume point for %s: %w", r.ID, err)
	}
	if found {
		byID := make(map[string]ledger.WorkItem, len(doc.State.Items))
		for _, it := range doc.State.Items {
			byID[it.ID] = it
		}
		for _, step := range steps {
			it, ok := byID[step.ID]
			if !ok || it.EvidenceRef == "" {
				break
			}
			if it.Status != ledger.StatusDoneUnverified && it.Status != ledger.StatusVerified {
				break
			}
			rp.Index++
			rp.Completed = append(rp.Completed, step.ID)
			rp.Snapshot = it.EvidenceRef
		}
	}
	if rp.Index < len(steps) {
		rp.Step = steps[rp.Index].ID
	}
	if rp.Index == 0 && s.cfg.RepoFacts != nil {
		// Nothing finished: the successor starts on the state the task started
		// from, which is the attempt's durably recorded base. Read through
		// RepoFacts because it is read-only by contract (CONVENTIONS §59) —
		// asking where to restore to must not itself take a snapshot.
		_, base, err := s.cfg.RepoFacts(ctx, r.TaskID)
		if err != nil {
			return rp, fmt.Errorf("stage: resume base for %s: %w", r.ID, err)
		}
		rp.Snapshot = base
	}
	rp.ParentCause = s.runEnding(ctx, r.ParentRunID)
	return rp, nil
}

// runEnding is how a run last said it ended: the reason on its latest FSM
// transition (Spec S14.2 family 1 — every transition carries its cause). "" when
// the run recorded none, which is an absence, never a guess.
func (s *Skeleton) runEnding(ctx context.Context, runID string) string {
	if runID == "" || s.cfg.DB == nil {
		return ""
	}
	var reason string
	err := s.cfg.DB.QueryRowContext(ctx, `
		SELECT COALESCE(json_extract(payload, '$.reason'), '')
		  FROM run_events
		 WHERE run_id = ? AND type = ?
		 ORDER BY event_seq DESC LIMIT 1`, runID, run.EventState).Scan(&reason)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.logger().Warn("stage: read parent ending", "run", runID, "err", err)
		return ""
	}
	return reason
}

// reason is the successor's claimed→running sentence: plain words for the
// person whose task this is, naming the step it picks up at and the run it
// follows, and saying plainly what happens to the interrupted attempt's work.
// No spec citation and no internal token beyond run and step ids.
func (p resumePoint) reason() string {
	if p.Step == "" {
		return fmt.Sprintf("Every plan step was finished before run %s stopped, so this run has nothing left to do and closes the work out.", p.Parent)
	}
	switch {
	case p.Index == 1:
		return fmt.Sprintf("Picking up at step %s after run %s stopped: step %s was finished by it and is kept; the working copy is reset to the state after %s and %s starts over.",
			p.Step, p.Parent, p.Completed[0], p.Completed[0], p.Step)
	case p.Index > 1:
		return fmt.Sprintf("Picking up at step %s after run %s stopped: steps %s to %s were finished by it and are kept; the working copy is reset to the state after %s and %s starts over.",
			p.Step, p.Parent, p.Completed[0], p.Completed[p.Index-1], p.Completed[p.Index-1], p.Step)
	case p.Snapshot == "":
		return fmt.Sprintf("Picking up at step %s after run %s stopped: it left no saved working copy to pick up from, so this run starts again at the first step.",
			p.Step, p.Parent)
	default:
		return fmt.Sprintf("Picking up at step %s after run %s stopped: it left no finished step to pick up from, so this run starts again at the first step, on the working copy as it was before the task began.",
			p.Step, p.Parent)
	}
}

// forkResumeDetail is the structured half of the successor's running transition:
// the same facts the sentence states, in the form a screen can join on.
type forkResumeDetail struct {
	ResumeStep     string   `json:"resume_step,omitempty"`
	CompletedSteps []string `json:"completed_steps,omitempty"`
	ResumeSnapshot string   `json:"resume_snapshot,omitempty"`
	ParentRunID    string   `json:"parent_run_id"`
	ParentCause    string   `json:"parent_cause,omitempty"`
}

func (p resumePoint) detail() json.RawMessage {
	b, err := json.Marshal(forkResumeDetail{
		ResumeStep:     p.Step,
		CompletedSteps: p.Completed,
		ResumeSnapshot: p.Snapshot,
		ParentRunID:    p.Parent,
		ParentCause:    p.ParentCause,
	})
	if err != nil {
		return nil
	}
	return b
}

// The two classes of step failure. They are different facts about the world and
// they need different answers: the WORK failing is the engine session ending
// short of the step's Done-when, and the PLATFORM failing is everything around
// it — a session that could not be started, a record that could not be written.
const (
	failedWork     = "work"
	failedPlatform = "platform"
)

// failedClass classifies a step error. A *stageError is the engine's own
// session ending short (runner.go); anything else happened on the platform's
// side of the seam.
func failedClass(err error) string {
	var se *stageError
	if errors.As(err, &se) {
		return failedWork
	}
	return failedPlatform
}

// stepCrashPlain is the crashing run's own ending, in plain words: which step
// it died on, whose step failed, and what that means for the work already done.
// It is true because of the step-close record: finished steps keep their
// evidence, so the successor really does keep them.
func stepCrashPlain(stepID, failed string) string {
	if failed == failedWork {
		return fmt.Sprintf("Step %s: the work session stopped before the step was finished. Steps finished earlier stay finished; this attempt stops here.", stepID)
	}
	return fmt.Sprintf("Step %s: the platform could not run the work session for this step. Steps finished earlier stay finished; this attempt stops here.", stepID)
}

// stepCrashDetail is the structured half of that ending. `cause` is the raw
// error line — bounded, kept for whoever is debugging the platform, and never
// the sentence a requester is shown.
type stepCrashPayload struct {
	Cause  string `json:"cause"`
	Step   string `json:"step"`
	Failed string `json:"failed"`
	Plain  string `json:"plain"`
}

func stepCrashDetail(cause, stepID, failed, plain string) json.RawMessage {
	b, err := json.Marshal(stepCrashPayload{Cause: cause, Step: stepID, Failed: failed, Plain: plain})
	if err != nil {
		return nil
	}
	return b
}

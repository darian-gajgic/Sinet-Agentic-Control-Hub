package stage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
)

// Onboarding a repository is itself a task the platform performs (Spec S13.7,
// R5): a dedicated run role (.onboard) whose deterministic platform steps
// (register → clone → scan → draft) run over internal/project through the
// OnboardStart seam, whose drafted conventions/commands/danger-zones surface on
// a durable `asks` row ANSWERED BY THE OWNER (D10 — a non-owner is refused),
// and whose approval activates the entry via OnboardApprove. Park/answer ride
// the existing ask machinery (the intake issueCard/closeAndResume precedent,
// CONVENTIONS §16); the compose-run ceremony (§21) is the run-role precedent.
//
// Reading (F1, documented): the register/clone/scan are platform prep done at
// StartOnboarding so the pending entry + draft are durable before the run
// dispatches; the run then DRIVES the interactive half S13.7 emphasises — the
// owner's review-and-approve of the draft. dispatchOnboard re-reads the drafted
// capture (OnboardStart is idempotent) and surfaces it; nothing is faked.

// RunSuffixOnboard names the onboarding ceremony run.
const RunSuffixOnboard = ".onboard"

// onboardAskPrefix keys the durable onboarding-approval ask (routed by the
// Surface to AnswerOnboarding; distinct from intake:/ask-verify- prefixes).
const onboardAskPrefix = "onboard:"

// onboardTaskPrefix keys a project's onboarding task.
const onboardTaskPrefix = "onboard-"

// The board columns the onboarding task occupies, both drawn from the DECLARED
// vocabulary web/src/kanban.ts owns (map §3 v3, checkpoint-2 decision D-B):
// `intake` renders as **Backlog** — a record of intent whose execution has not
// begun — and `done` as **Done**, which stays visible. No new status string
// enters the vocabulary; the seed-hygiene guard (internal/api's
// assertNoUndeclaredKanbanStatus) parses that file for exactly this set.
const (
	kanbanOnboardBirth    = "intake"
	kanbanOnboardApproved = "done"
)

// roleOnboard is the onboarding run role.
const roleOnboard role = "onboard"

// IsOnboardAskID reports whether an ask routes to the onboarding answer path.
func IsOnboardAskID(askID string) bool { return strings.HasPrefix(askID, onboardAskPrefix) }

// OnboardTaskID and OnboardAskID name a project's onboarding task and its
// durable owner-approval ask (Spec S13.7). They are exported and USED BELOW
// because a transport has to name those references in its answer — the S15.2
// create door reports where the approval card will arrive — and a second
// composition of the same id scheme in another package is how two layers come
// to disagree about what one thing is called.
func OnboardTaskID(projectID string) string { return onboardTaskPrefix + projectID }
func OnboardAskID(projectID string) string  { return onboardAskPrefix + projectID }

// errOnboardNotOwner is D10: only the entry's owner approves onboarding (a
// non-owner answer is refused). The Surface maps it to 403.
var errOnboardNotOwner = errors.New("stage: only the project owner may approve onboarding (D10)")

// mapOnboardErr maps the onboarding answer-path errors onto transport
// statuses (the surface error contract).
func mapOnboardErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errOnboardNotOwner):
		return surfaceErr(http.StatusForbidden, "not_owner", err)
	case strings.Contains(err.Error(), "unknown onboarding ask"):
		return surfaceErr(http.StatusNotFound, "not_found", err)
	default:
		return err
	}
}

// onboardCard is the durable ask snapshot surfaced for owner approval.
type onboardCard struct {
	Kind      string          `json:"kind"` // "onboard"
	ProjectID string          `json:"project_id"`
	Owner     string          `json:"owner"`
	Draft     json.RawMessage `json:"draft"`
	IssuedTS  string          `json:"issued_ts"`
}

// onboardAnswer is the owner's decision.
type onboardAnswer struct {
	Approve bool            `json:"approve"`
	Draft   json.RawMessage `json:"draft,omitempty"` // optional edited draft
}

// StartOnboarding launches a project's onboarding task: the deterministic
// register → clone → scan → draft (OnboardStart, over internal/project), then a
// run whose durable ask carries the draft for owner approval. Returns the
// onboarding task id.
func (s *Skeleton) StartOnboarding(ctx context.Context, owner, projectID, name, source string) (string, error) {
	if s.cfg.OnboardStart == nil {
		return "", errors.New("stage: onboarding seam not wired (Spec S13.7)")
	}
	if s.sched == nil {
		return "", errors.New("stage: no scheduler bound (Bind was not called)")
	}
	if _, err := s.cfg.OnboardStart(ctx, projectID, owner, name, source); err != nil {
		return "", fmt.Errorf("stage: onboarding scan/draft: %w", err)
	}
	taskID := OnboardTaskID(projectID)
	runID := taskID + RunSuffixOnboard
	now := s.now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	err := s.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		// The birth column is written HERE, in the same transaction as the task
		// and the run (the intake INSERT precedent, internal/intake/pipeline.go;
		// CONVENTIONS §8: the task exists in the D7 record from its first
		// instant). Without it the column takes 0001's schema default '' and the
		// board's forward tolerance renders it as a column of its own titled
		// "(no status recorded)" — a raw string in front of the operator for a
		// task that has an honest declared column available to it.
		// `intake` is the board's Backlog (web/src/kanban.ts): a recorded
		// intent whose execution has not begun, which is exactly what a drafted
		// onboarding awaiting its owner is. Waiting-on-the-owner is carried by
		// the durable ask (waiting_on_human on the list read), never by a
		// column.
		//
		// ON CONFLICT DO NOTHING keeps the crash-relaunch path harmless: a
		// re-run must not walk an advanced task backwards into its birth column.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, title, kanban_status, created_ts) VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT (task_id) DO NOTHING`,
			taskID, owner, "Onboard "+name, kanbanOnboardBirth, now); err != nil {
			return err
		}
		_, err := s.cfg.Runs.CreateTx(ctx, tx, run.NewRun{
			ID: runID, UserID: owner, TaskID: taskID, Substrate: s.cfg.Substrate, Lane: s.cfg.Lane,
		}, run.EventCreated, nil)
		return err
	})
	switch {
	case errors.Is(err, run.ErrExists):
		// Re-launch after a crash: one onboarding task per project.
	case err != nil:
		return "", fmt.Errorf("stage: create onboarding task/run: %w", err)
	}
	if err := s.sched.Enqueue(ctx, runID, scheduler.ClassInteractive); err != nil {
		return "", fmt.Errorf("stage: enqueue onboarding run: %w", err)
	}
	s.logger().Info("stage: launched onboarding run (Spec S13.7)", "run", runID, "project", projectID, "owner", owner)
	return taskID, nil
}

// dispatchOnboard drives the onboarding run: re-read the drafted capture
// (idempotent), surface it on a durable owner-approval ask, and park.
func (s *Skeleton) dispatchOnboard(ctx context.Context, r run.Run) error {
	if s.cfg.OnboardStart == nil {
		s.crash(ctx, r.ID, "onboarding seam not wired")
		return errors.New("stage: onboarding seam not wired")
	}
	if _, err := s.cfg.Runs.Transition(ctx, r.ID, run.StateRunning, run.TransitionOptions{
		Reason: "onboarding scan/draft (Spec S13.7)", Actor: run.ActorPlatform,
	}); err != nil {
		return err
	}
	projectID := strings.TrimPrefix(r.TaskID, onboardTaskPrefix)
	draft, err := s.cfg.OnboardStart(ctx, projectID, r.UserID, "", "")
	if err != nil {
		s.crash(ctx, r.ID, "onboarding scan/draft: "+err.Error())
		return err
	}
	now := s.now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	card := onboardCard{Kind: "onboard", ProjectID: projectID, Owner: r.UserID, Draft: draft, IssuedTS: now}
	snapshot, err := json.Marshal(card)
	if err != nil {
		return err
	}
	askID := OnboardAskID(projectID)
	return s.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			// run_id REBINDS on conflict (P3-RW-6 R11): the re-issued ask belongs
			// to the run that is issuing it. A recovery fork re-dispatches this
			// leg, and leaving the row pinned to the crashed parent tore the
			// answer in half — OnboardApprove activated the entry and then the
			// run transition failed (crashed→running is no edge), leaving the
			// fork parked forever on a card it could never close.
			`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts)
			 VALUES (?, ?, ?, ?, 'open', ?)
			 ON CONFLICT (ask_id) DO UPDATE SET
			     run_id = excluded.run_id, snapshot = excluded.snapshot, status = 'open'`,
			askID, r.ID, r.UserID, string(snapshot), now); err != nil {
			return fmt.Errorf("stage: insert onboarding ask: %w", err)
		}
		_, err := s.cfg.Runs.TransitionTx(ctx, tx, r.ID, run.StateParked, run.TransitionOptions{
			Reason: "onboarding draft awaiting owner approval — gates wait (D10)", Actor: run.ActorPlatform,
		})
		return err
	})
}

// AnswerOnboarding answers a durable onboarding ask: D10 — only the ask's owner
// may approve (a non-owner is refused with errOnboardNotOwner). On approval the
// entry activates (OnboardApprove, optional edited draft) and the run completes.
func (s *Skeleton) AnswerOnboarding(ctx context.Context, userID, askID string, answer json.RawMessage) (string, error) {
	if s.cfg.OnboardApprove == nil {
		return "", errors.New("stage: onboarding seam not wired")
	}
	var (
		runID, owner, snapshot, status string
	)
	err := s.cfg.DB.QueryRowContext(ctx,
		`SELECT run_id, user_id, snapshot, status FROM asks WHERE ask_id = ?`, askID).
		Scan(&runID, &owner, &snapshot, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("stage: unknown onboarding ask %q", askID)
	}
	if err != nil {
		return "", err
	}
	if status != "open" {
		return "", fmt.Errorf("stage: onboarding ask %q is already %s", askID, status)
	}
	// D10: the ask's owner answers; a non-owner is refused.
	if userID != owner {
		return "", errOnboardNotOwner
	}
	var card onboardCard
	if err := json.Unmarshal([]byte(snapshot), &card); err != nil {
		return "", err
	}
	var ans onboardAnswer
	if err := json.Unmarshal(answer, &ans); err != nil {
		return "", fmt.Errorf("stage: bad onboarding answer: %w", err)
	}
	if !ans.Approve {
		return "", fmt.Errorf("stage: onboarding answer must approve (edit-and-approve is the only verb at v0)")
	}
	// Activate the entry (D10) — outside the run transaction (the project store
	// verbs compose their own tx; the ledger-never-nests discipline, §13).
	if err := s.cfg.OnboardApprove(ctx, card.ProjectID, owner, ans.Draft); err != nil {
		return "", fmt.Errorf("stage: onboarding approve: %w", err)
	}
	now := s.now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	err = s.cfg.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE asks SET status = 'answered', answered_ts = ?, answer = ? WHERE ask_id = ? AND status = 'open'`,
			now, string(answer), askID); err != nil {
			return err
		}
		// Resume then complete: the onboarding task's work ends at activation.
		if _, err := s.cfg.Runs.TransitionTx(ctx, tx, runID, run.StateRunning, run.TransitionOptions{
			Reason: "onboarding approved (D10) — activating", Actor: run.ActorPlatform,
		}); err != nil {
			return err
		}
		if _, err := s.cfg.Runs.TransitionTx(ctx, tx, runID, run.StateCompleted, run.TransitionOptions{
			Reason: "onboarding complete: entry active", Actor: run.ActorPlatform,
		}); err != nil {
			return err
		}
		// The board follows in the SAME transaction as the run's completion,
		// because they are one fact: S13.7 ends the onboarding task's work at
		// activation. `done` stays VISIBLE (operator-ratified D-B: "I want to
		// see what is already done"), and the platform has no archived state at
		// v0 to move it to. The in-tx UPDATE is chosen over the skeleton's
		// setKanban helper, which opens its own WriteTx and would nest here.
		_, err := tx.ExecContext(ctx,
			`UPDATE tasks SET kanban_status = ? WHERE task_id = ?`,
			kanbanOnboardApproved, OnboardTaskID(card.ProjectID))
		return err
	})
	if err != nil {
		return "", err
	}
	s.logger().Info("stage: onboarding approved — entry active (Spec S13.7)", "project", card.ProjectID, "owner", owner)
	return card.ProjectID, nil
}

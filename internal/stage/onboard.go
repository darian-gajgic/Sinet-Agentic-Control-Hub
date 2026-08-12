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

// choiceApproveOnboard is the onboarding card's ONE answer verb (Spec S13.7:
// "the owner approves the draft (D10)"). It is the single place the value is
// written: the card DECLARES it (onboardVocabulary) and AnswerOnboarding
// VALIDATES against it — one list, two readers, the intake DeltaActions
// precedent (CONVENTIONS §43).
//
// THERE IS NO DECLINE VERB, and its absence is deliberate rather than pending.
// No server behavior exists behind one — AnswerOnboarding refuses a
// non-approval outright — and S13.7 defines none, so declaring reject/deny
// would render a button that always fails, which is the same class of defect
// as the verb-less card. The exit that DOES exist is cancelling the
// onboarding task (CancelTask finalizes the parked run and closes the open ask
// as `cancelled` in one transaction); the help block names it in operator
// language and the choices do not fake it as a verb.
const choiceApproveOnboard = "approve"

// onboardOption is one labeled option of the S06.7 decision wire shape. It is
// declared HERE, as a wire struct, because the wire shape is the contract: the
// inbox derives a card's verbs from `decision.choices[].value` and the browser
// composes its answer from the same key (the B6-6 finding — the Go field name
// was fine, the wire key was not). internal/intake's identical `Option` would
// serve equally; a local struct keeps the vocabulary's home in the package that
// owns this card (brief OQ4).
type onboardOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// onboardHelp is the 13.5 help block every S15.6 card carries: what this
// decision does, what could go wrong, what the platform recommends.
type onboardHelp struct {
	What      string `json:"what"`
	Wrong     string `json:"wrong"`
	Recommend string `json:"recommend"`
}

// onboardDecision is the card's declared answer contract plus the content the
// person deciding reads.
type onboardDecision struct {
	Summary string          `json:"summary"`
	Detail  []string        `json:"detail,omitempty"`
	Choices []onboardOption `json:"choices"`
	Help    onboardHelp     `json:"help"`
}

// onboardVocabulary is the card's answer vocabulary, from the constant the
// answer path itself reads. One list, two readers.
func onboardVocabulary() []onboardOption {
	return []onboardOption{{Label: "Approve — activate this project", Value: choiceApproveOnboard}}
}

// declaresOnboardChoice reports whether c is a verb this card offers.
//
// It validates against the PACKAGE's vocabulary, not against the answering
// card's stored declaration (brief OQ5): a snapshot written before the
// vocabulary existed then answers exactly as a fresh one does, the stored
// declaration stays presentation, and there is no third behavior to reason
// about. The stored snapshot is still the resume input and the pin source
// (Spec S02.2/S02.5) — this reads the platform's verb set, which is what the
// answer path applies.
func declaresOnboardChoice(c string) bool {
	for _, o := range onboardVocabulary() {
		if o.Value == c {
			return true
		}
	}
	return false
}

// onboardVocabularySentence names the declared verbs for a refusal, so a
// refused answer says what this card actually accepts.
func onboardVocabularySentence() string {
	quoted := make([]string, 0, len(onboardVocabulary()))
	for _, o := range onboardVocabulary() {
		quoted = append(quoted, fmt.Sprintf("%q", o.Value))
	}
	return strings.Join(quoted, ", ")
}

// onboardCard is the durable ask snapshot surfaced for owner approval.
//
// The top-level fields are the record: AnswerOnboarding reads project_id off
// the snapshot to activate, and the full `draft` rides it for the answer path
// and the audit. The `decision` body is the card's DECLARATION — the verbs it
// offers and the content the person deciding reads — in the same S06.7 shape
// every other decision card uses, so the inbox derives its controls from the
// card instead of the transport naming this pipeline's vocabulary for it
// (CONVENTIONS §40-B, §43).
type onboardCard struct {
	Kind      string           `json:"kind"` // "onboard"
	ProjectID string           `json:"project_id"`
	Owner     string           `json:"owner"`
	Draft     json.RawMessage  `json:"draft"`
	IssuedTS  string           `json:"issued_ts"`
	Decision  *onboardDecision `json:"decision,omitempty"`
}

// onboardAnswer is the owner's decision. TWO envelopes reach it and both are
// the operator's own: the `{choice}` a surface composes for the family this
// card declares, and the landed `{approve, draft}` raw door — which stays the
// ONLY way to approve an edited draft at v0 (a stated absence, S13.7: an inbox
// draft editor is frontend work this packet does not own).
//
// `criteria` — which a decision surface may append from the card's own detail
// lines — is deliberately NOT a field here. encoding/json ignores keys a
// struct does not name, which IS the tolerance: declaring a field the answer
// path never reads would only add a way for a value nothing consumes to fail
// the parse.
type onboardAnswer struct {
	// Approve is a POINTER because absent and false are different answers. The
	// choice envelope carries no `approve` at all, while an explicit
	// `"approve": false` beside a choice is a contradiction — and a
	// contradiction is refused, never resolved: picking a winner would apply a
	// decision the person did not make.
	Approve *bool           `json:"approve"`
	Draft   json.RawMessage `json:"draft,omitempty"` // optional edited draft
	Choice  string          `json:"choice,omitempty"`
}

// approves reads the answer as this card's declared approval, or says exactly
// why it is not one.
func (a onboardAnswer) approves() error {
	if a.Choice != "" {
		if !declaresOnboardChoice(a.Choice) {
			return fmt.Errorf("stage: onboarding answer choice %q is not a verb this card declares — it declares %s "+
				"(edit-and-approve is the only verb at v0; to not onboard this project, cancel the onboarding task)",
				a.Choice, onboardVocabularySentence())
		}
		if a.Approve != nil && !*a.Approve {
			return fmt.Errorf("stage: contradictory onboarding answer: choice %q with \"approve\": false — one answer "+
				"carries one decision, and nothing was applied. Send the choice %s, or {\"approve\": true}",
				a.Choice, onboardVocabularySentence())
		}
		return nil
	}
	if a.Approve != nil && *a.Approve {
		return nil
	}
	return fmt.Errorf("stage: onboarding answer must approve — send the choice %s, or {\"approve\": true} "+
		"(edit-and-approve is the only verb at v0; to not onboard this project, cancel the onboarding task)",
		onboardVocabularySentence())
}

// onboardDraftShape is the slice of the drafted capture the card's digest
// reads. The OnboardStart seam hands the draft over as JSON precisely so this
// layer never imports internal/project (the wall stated on
// project.OnboardStart itself), so the digest reads the draft the way every
// snapshot consumer reads one: off the wire keys.
type onboardDraftShape struct {
	Conventions []string `json:"conventions"`
	Commands    struct {
		Build   string `json:"build"`
		Test    string `json:"test"`
		Lint    string `json:"lint"`
		Run     string `json:"run"`
		Preview string `json:"preview"`
	} `json:"commands"`
	DangerZones []struct {
		Path   string `json:"path"`
		Action string `json:"action"`
		Rule   string `json:"rule"`
	} `json:"danger_zones"`
}

// onboardDigest states the drafted capture in plain lines, so the rendered card
// SHOWS what is being approved (D10) instead of carrying the draft as an
// unrendered blob. Platform-drafted and deterministic — no model duty exists
// here (S13.7 names none; the S06.10 deterministic-fallback posture) — and the
// full draft still rides the snapshot for the answer path and the audit.
func onboardDigest(draft json.RawMessage) []string {
	var d onboardDraftShape
	if len(draft) > 0 {
		if err := json.Unmarshal(draft, &d); err != nil {
			// Honest absence over a fabricated summary: the draft itself still
			// rides the card and the approval applies it unchanged.
			return []string{"The drafted capture could not be read back for display. The full draft is on this card and approving applies it unchanged."}
		}
	}
	var lines []string
	for _, c := range d.Conventions {
		lines = append(lines, "Convention: "+c)
	}
	for _, c := range []struct{ name, command string }{
		{"Build", d.Commands.Build}, {"Test", d.Commands.Test}, {"Lint", d.Commands.Lint},
		{"Run", d.Commands.Run}, {"Preview", d.Commands.Preview},
	} {
		if c.command != "" {
			lines = append(lines, c.name+" command: "+c.command)
		}
	}
	for _, z := range d.DangerZones {
		line := "Danger zone: " + z.Path
		if z.Action != "" {
			line += " (" + z.Action + ")"
		}
		if z.Rule != "" {
			line += " — " + z.Rule
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, "The scan captured no conventions, commands or danger zones: approving activates the entry with the empty capture it has.")
	}
	return lines
}

// onboardCardBody drafts the card's declaration for one project's draft:
// what this is, what is in it, the verb, and the 13.5 help block.
func onboardCardBody(projectID string, draft json.RawMessage) *onboardDecision {
	return &onboardDecision{
		Summary: "Onboarding " + projectID + ": approve the drafted conventions, commands and danger zones to activate this project's entry.",
		Detail:  onboardDigest(draft),
		Choices: onboardVocabulary(),
		Help: onboardHelp{
			What: "Approving records this draft as the project's captured content and activates the entry. From then on the platform " +
				"reads these conventions and commands when it works on " + projectID + ", keeps away from the danger zones, and the " +
				"project can be named in a task.",
			Wrong: "If the draft is wrong, the platform works from wrong instructions until the capture is replaced — a missing danger zone " +
				"matters most, because danger zones are what keep work away from secrets and protected branches.",
			Recommend: "Approve if this matches the project. Approving is the only verb this card has: there is no decline, because " +
				"nothing yet undoes a registration. To not onboard this project, cancel the onboarding task instead — that ends the run " +
				"and closes this card.",
		},
	}
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
	// The card declares the verb its own answer path applies (§43: fixed at the
	// producer). Every dispatch re-marshals, so a re-issued ask carries the
	// declaration with no migration and no re-issue door; a snapshot written
	// before it keeps its shape and answers exactly as it did (declaresOnboardChoice).
	card := onboardCard{
		Kind: "onboard", ProjectID: projectID, Owner: r.UserID, Draft: draft, IssuedTS: now,
		Decision: onboardCardBody(projectID, draft),
	}
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
	if err := ans.approves(); err != nil {
		return "", err
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

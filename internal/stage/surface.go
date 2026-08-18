package stage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// Surface adapts the skeleton to the api.IntakeSurface transport contract
// (Spec S01.3: the API seam stays thin — identity/step-up/status live in
// internal/api, pipeline behavior here). Payloads are assembled JSON; the
// pipeline's domain errors map to *api.SurfaceError statuses.
type Surface struct{ sk *Skeleton }

var (
	_ api.IntakeSurface = (*Surface)(nil)
	_ api.CancelSurface = (*Surface)(nil)
	_ api.ResumeSurface = (*Surface)(nil)
)

// Surface returns the api-facing adapter.
func (s *Skeleton) Surface() *Surface { return &Surface{sk: s} }

func surfaceErr(status int, code string, err error) *api.SurfaceError {
	return &api.SurfaceError{Status: status, Code: code, Msg: err.Error()}
}

// mapIntakeErr maps pipeline errors onto transport statuses.
func mapIntakeErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, intake.ErrNotRequester):
		return surfaceErr(http.StatusForbidden, "not_requester", err)
	case errors.Is(err, intake.ErrPinUnknown):
		// A pinned project that does not exist and one the requester may not
		// see are ONE response: telling them apart would make Submit an
		// existence oracle for other people's projects (S15.2).
		return surfaceErr(http.StatusNotFound, "not_found", err)
	case errors.Is(err, intake.ErrPinNotActive):
		// Visible but not yet owner-approved: a state the requester may know
		// honestly, and one that resolves by finishing onboarding (S13.7).
		return surfaceErr(http.StatusConflict, "project_not_active", err)
	case errors.Is(err, intake.ErrUnknownAsk), errors.Is(err, sql.ErrNoRows):
		return surfaceErr(http.StatusNotFound, "not_found", err)
	case errors.Is(err, intake.ErrBadAnswer), errors.Is(err, intake.ErrMarkersOpen),
		errors.Is(err, intake.ErrBelowFloor):
		return surfaceErr(http.StatusBadRequest, "bad_answer", err)
	case errors.Is(err, intake.ErrGateOpen), errors.Is(err, intake.ErrNotRunning),
		errors.Is(err, intake.ErrPhase):
		return surfaceErr(http.StatusConflict, "conflict", err)
	case errors.Is(err, intake.ErrNoState):
		return surfaceErr(http.StatusNotFound, "not_found", err)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// The request died under its own beat (a page navigating away, a client
		// timeout): nothing about the platform is broken, so a raw 500 would be a
		// lie (S01.3 honest statuses). It is the same "not now, re-read and
		// retry" the state-based refusals answer with — and it stays a REFUSAL
		// only because mapDriveErr consults the run's state FIRST, so a beat that
		// already took its run to `running` gets the corpse instead (R3).
		return surfaceErr(http.StatusConflict, "conflict", err)
	default:
		return err
	}
}

// CodeAdvanceCrashed is the surface code of a drive that died AFTER the run
// was already running: the request failed (500 stands — nothing is swallowed)
// but the run is a classifiable corpse the recovery ladder forks within one
// sweep, so the caller can say "recovering" instead of showing a bare
// internal error (P3-RW-9 R6).
const CodeAdvanceCrashed = "advance_crashed_recovering"

// mapDriveErr maps an intake DRIVE error — from `pipe.Answer`, `pipe.Advance`
// or the continuation that follows either — onto its transport status, minting
// the S02.3 corpse when the beat would otherwise leave the run running with
// nobody driving it.
//
// The cut is the RESUME COMMIT, and the run's own state is what records it —
// so the state is consulted FIRST, before any error classification:
//
//   - a run still PARKED on its open ask never reached the drive: the refusal
//     (unknown ask, not-requester, bad answer, gate open, not running) keeps
//     its 4xx and nothing transitions (R5, S06.1 gates wait). Same for a run
//     already terminal — there is nothing to strand;
//   - a run this beat already took to `running` gets the corpse WHATEVER the
//     error looks like. Classification cannot gate this: the drive calls duty
//     seams (planner, critic, judge, registry) that are free to return an
//     error wrapping any sentinel, and answering such a drive failure with a
//     4xx would leave exactly the strand this exists to end — the pipeline
//     errored up as designed (it owns no run FSM, CONVENTIONS §14), and
//     returning the error alone leaves the run running, driver-less and
//     corpse-less: invisible to the ladder (it scans claimed/running/draining
//     but reaps only what has no live lease), then silence-parked by the
//     watchdog, then releasable only by another human resume. The corpse is
//     what makes the heal machine-only and bounded: crashed runs are swept
//     every pass and forked from their last checkpoint (S02.5 steps 2–3), and
//     the fork's dispatch rebinds and re-drives (P3-RW-6).
//
// This is the posture the dispatch leg (`Skeleton.crash` at dispatchIntake)
// and the S07.7 verify answer beat (answer.go) already take — CONVENTIONS §16
// doctrine, extended to the intake beats it had skipped.
//
// The whole posture is DETACHED from the request context (P3-RW-10 R1): the
// commonest way a drive dies is that its caller died, and a posture riding the
// same dead context read nothing, decided nothing and wrote nothing — the exact
// strand this exists to end. Values are kept; only cancellation is dropped.
func (u *Surface) mapDriveErr(ctx context.Context, runID, beat string, err error) error {
	if runID == "" {
		return mapIntakeErr(err)
	}
	ctx = context.WithoutCancel(ctx)
	r, gerr := u.sk.cfg.Runs.Get(ctx, runID)
	if gerr != nil {
		u.sk.logger().Error("stage: intake "+beat+" failed and its run could not be read",
			"run", runID, "err", err, "read_err", gerr)
		return mapIntakeErr(err)
	}
	if r.State != run.StateRunning {
		return mapIntakeErr(err)
	}
	u.sk.logger().Error("stage: intake "+beat+" died mid-drive; crashing the run for the recovery ladder",
		"run", runID, "task", r.TaskID, "err", err)
	u.sk.crash(ctx, runID, "intake "+beat+": "+err.Error())
	return surfaceErr(http.StatusInternalServerError, CodeAdvanceCrashed, err)
}

// mapVerifyErr maps the S07.7 answer-path errors onto transport statuses.
func mapVerifyErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, verify.ErrNotRequester):
		return surfaceErr(http.StatusForbidden, "not_requester", err)
	case errors.Is(err, verify.ErrUnknownAsk), errors.Is(err, sql.ErrNoRows):
		return surfaceErr(http.StatusNotFound, "not_found", err)
	case errors.Is(err, verify.ErrBadAnswer), errors.Is(err, verify.ErrUnsupportedAnswer):
		return surfaceErr(http.StatusBadRequest, "bad_answer", err)
	case errors.Is(err, verify.ErrNotResumable):
		return surfaceErr(http.StatusConflict, "not_resumable", err)
	default:
		return err
	}
}

// submitBody is the POST /api/intake/requests payload.
type submitBody struct {
	Title string `json:"title"`
	Text  string `json:"text"`
	// Inputs are requester-supplied input descriptors, ADDITIVE (S15.2
	// additive-first; P3-B6-7 OQ8). The path and every existing caller are
	// unchanged: a body without them submits exactly as before. They are
	// already owner-resolved by the surface that collected them — the assistant
	// resolves each ref against the requester's OWN exchange manifest — so
	// nothing here can be handed another person's object.
	Inputs []intake.Input `json:"inputs,omitempty"`
	// Project OPTIONALLY pins the request to a registered project by registry
	// id, ADDITIVE (S15.2; the Inputs precedent above). It is the Projects-tab
	// door: the picker sends the id, so scoping a request no longer depends on
	// the requester typing the project's name in the text. The id is validated
	// server-side at the registry seam — owner-or-member and ACTIVE — and an
	// invalid pin refuses the submission rather than quietly dropping it.
	Project string `json:"project,omitempty"`
}

// Submit implements api.IntakeSurface: Stage-0 triage + task/run birth,
// then admission through the scheduler (Spec S16.6: Enqueue is the sole
// ingress — the claim loop dispatches the intake run).
func (u *Surface) Submit(ctx context.Context, userID string, body json.RawMessage) (json.RawMessage, error) {
	var b submitBody
	if err := json.Unmarshal(body, &b); err != nil {
		return nil, surfaceErr(http.StatusBadRequest, "bad_body", err)
	}
	if strings.TrimSpace(b.Text) == "" {
		return nil, surfaceErr(http.StatusBadRequest, "bad_body", errors.New(`missing "text"`))
	}
	if u.sk.sched == nil {
		return nil, errors.New("stage: no scheduler bound")
	}
	st, err := u.sk.pipe.Start(ctx, intake.Request{
		UserID: userID, Title: b.Title, Text: b.Text, Inputs: b.Inputs, Project: b.Project})
	if err != nil {
		return nil, mapIntakeErr(err)
	}
	if err := u.sk.sched.Enqueue(ctx, st.RunID, scheduler.ClassInteractive); err != nil {
		return nil, fmt.Errorf("stage: enqueue intake run: %w", err)
	}
	u.sk.logger().Info("stage: request submitted", "task", st.TaskID, "user", userID)
	return u.taskView(ctx, st.TaskID)
}

// Answer implements api.IntakeSurface. High-tier approval answers demand a
// same-request PIN re-verification (Spec S01.9 step-up; the api layer
// verified it — pinVerified reports the fact).
//
// Verify-minted asks (the `ask-verify-`/`canary-` prefixes, CONVENTIONS
// §15) route to the S07.7 answer path: the three-verb cards resume the
// parked verify run in place; every other verify category rejects loudly
// until its verbs land. S01.9's approvals-family step-up stays scoped to
// the intake approval/delta cards — the verify decision cards release
// nothing (the effects gate is untouched, Spec S07.1), and V3 accept
// mechanics with their own gating are Spec S13's (B4).
func (u *Surface) Answer(ctx context.Context, userID, askID string, answer json.RawMessage, pinVerified bool) (json.RawMessage, error) {
	if IsOnboardAskID(askID) {
		// The S13.7 onboarding-approval ask (D10: the owner answers; a
		// non-owner is refused). Registry activation releases nothing outward,
		// so no step-up is demanded.
		projectID, err := u.sk.AnswerOnboarding(ctx, userID, askID, answer)
		if err != nil {
			return nil, mapOnboardErr(err)
		}
		return u.taskView(ctx, onboardTaskPrefix+projectID)
	}
	if verify.IsVerifyAskID(askID) {
		taskID, err := u.sk.AnswerVerifyAsk(ctx, userID, askID, answer)
		if err != nil {
			return nil, mapVerifyErr(err)
		}
		return u.taskView(ctx, taskID)
	}
	if IsLadderAskID(askID) {
		// The recovery ladder's terminal card (P3-RW-14 R3): retry grants the
		// ended lineage a fresh bounded budget, cancel ends the task. It
		// releases nothing outward, so no step-up is demanded — the same
		// reading the verification decision cards take.
		taskID, err := u.sk.AnswerLadderAsk(ctx, userID, askID, answer)
		if err != nil {
			return nil, mapVerifyErr(err)
		}
		return u.taskView(ctx, taskID)
	}
	kind, tier, runID, err := u.askCardMeta(ctx, askID)
	if err != nil {
		return nil, mapIntakeErr(err)
	}
	if (kind == intake.CardApproval || kind == intake.CardDelta) && tier == intake.TierHigh && !pinVerified {
		return nil, api.ErrPINRequired
	}
	st, err := u.sk.pipe.Answer(ctx, userID, askID, answer)
	if err != nil {
		// The answer resumed the run and drove it in this request: a drive that
		// died past the resume commit leaves a corpse, never a stranded run (R4).
		return nil, u.mapDriveErr(ctx, runID, "answer", err)
	}
	// The walking-skeleton continuation: an approved plan completes the
	// intake run and launches execution (the "what runs next is B2-4's"
	// edge of Spec S02.3's approval resume). It runs on the SAME resumed run,
	// so it owes the same posture — a continuation that dies before the run
	// leaves `running` would strand it exactly as a dead drive does (R4).
	if err := u.sk.afterIntake(ctx, st); err != nil {
		return nil, u.mapDriveErr(ctx, st.RunID, "answer continuation", err)
	}
	return u.taskView(ctx, st.TaskID)
}

// askCardMeta reads the durable ask's card kind + tier (asks.snapshot is
// the full card, Spec S02.2), plus the run the ask is bound to — read here,
// while the ask is still open, because it is the run the answer is about to
// resume and drive, and a failed drive has to be able to name it (R4).
func (u *Surface) askCardMeta(ctx context.Context, askID string) (intake.CardKind, intake.Tier, string, error) {
	var snapshot, runID string
	err := u.sk.cfg.DB.QueryRowContext(ctx,
		`SELECT snapshot, run_id FROM asks WHERE ask_id = ? AND status = 'open'`, askID).Scan(&snapshot, &runID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", intake.ErrUnknownAsk
	}
	if err != nil {
		return "", "", "", err
	}
	var card struct {
		Kind intake.CardKind `json:"kind"`
		Tier intake.Tier     `json:"tier"`
	}
	if err := json.Unmarshal([]byte(snapshot), &card); err != nil {
		return "", "", "", fmt.Errorf("stage: decode ask snapshot: %w", err)
	}
	return card.Kind, card.Tier, runID, nil
}

// Task implements api.IntakeSurface.
func (u *Surface) Task(ctx context.Context, taskID string) (json.RawMessage, error) {
	return u.taskView(ctx, taskID)
}

// Artifacts implements api.IntakeSurface: the task's CURRENT drafted
// SPEC/PLAN pair, loaded through the pipeline's own artifact store (sha256
// + re-render integrity checks included, Spec S06.6). Nothing EXECUTES from
// this read — execution still demands ApprovedPair (D10) — it is the read
// seam the S15.2 task detail renders.
func (u *Surface) Artifacts(ctx context.Context, taskID string) (json.RawMessage, error) {
	pair, err := u.sk.pipe.CurrentPair(ctx, taskID)
	if err != nil {
		return nil, mapIntakeErr(err)
	}
	out, err := json.Marshal(pair)
	if err != nil {
		return nil, fmt.Errorf("stage: marshal artifact pair: %w", err)
	}
	return out, nil
}

// Advance implements api.IntakeSurface: the dev nudge — re-drive a task's
// intake in place (owner-only; useful when a continuation error left the
// run running without an open card).
func (u *Surface) Advance(ctx context.Context, userID, taskID string) (json.RawMessage, error) {
	st, err := u.sk.pipe.LoadState(ctx, taskID)
	if err != nil {
		return nil, mapIntakeErr(err)
	}
	if st.Owner != userID {
		return nil, surfaceErr(http.StatusForbidden, "not_requester", intake.ErrNotRequester)
	}
	// The run this nudge is about to drive, captured BEFORE the drive: a failed
	// Advance returns a nil state, and a corpse has to be able to name its run.
	runID := st.RunID
	st, err = u.sk.pipe.Advance(ctx, taskID)
	if err != nil {
		// Same posture as the answer beat: the nudge drives a RUNNING run, and a
		// drive that dies mid-flight leaves the ladder something to fork (R4).
		return nil, u.mapDriveErr(ctx, runID, "advance", err)
	}
	if err := u.sk.afterIntake(ctx, st); err != nil {
		return nil, u.mapDriveErr(ctx, st.RunID, "advance continuation", err)
	}
	return u.taskView(ctx, taskID)
}

// CancelRun implements api.CancelSurface: the ratified S02.3 cancel mapping on
// ONE run (feature 4.5; cancel.go). The transport has already resolved owner
// scope and bounded the reason; actor is the authenticated person, reason is
// their own words for why (empty when they gave none), and a cancel is only
// ever a human act — these two methods are the mapping's only callers
// (NO AUTO-KILL, S14.4 / G1 D1.3).
func (u *Surface) CancelRun(ctx context.Context, actor, runID, reason string) (json.RawMessage, error) {
	out, err := u.sk.CancelRun(ctx, actor, runID, reason)
	if err != nil {
		return nil, mapCancelErr(err)
	}
	return json.Marshal(out)
}

// CancelTask implements api.CancelSurface: every non-terminal run of the task
// under the same mapping, each carrying the same reason.
func (u *Surface) CancelTask(ctx context.Context, actor, taskID, reason string) (json.RawMessage, error) {
	out, err := u.sk.CancelTask(ctx, actor, taskID, reason)
	if err != nil {
		return nil, mapCancelErr(err)
	}
	return json.Marshal(out)
}

// mapCancelErr maps the cancel path's errors onto transport statuses. The
// transient CAS window answers 409-retry: a claimed run is between the
// scheduler's claim and the dispatcher's first transition, and racing that is
// how a cancel would corrupt a dispatch rather than stop it.
func mapCancelErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrCancelInFlight):
		return surfaceErr(http.StatusConflict, "claim_in_flight", err)
	case errors.Is(err, ErrCancelRaced):
		// The state moved under the cancel: a conflict the caller resolves by
		// re-reading, never an internal error (drain D8).
		return surfaceErr(http.StatusConflict, "cancel_raced", err)
	case errors.Is(err, ErrCancelQueueUnsettleable):
		return surfaceErr(http.StatusServiceUnavailable, "not_wired", err)
	case errors.Is(err, run.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		return surfaceErr(http.StatusNotFound, "not_found", err)
	default:
		return err
	}
}

// ResumeRun implements api.ResumeSurface: S14.4's "resume — I was wrong" on an
// ASK-LESS park (resume.go). The transport has already resolved owner scope;
// actor is the authenticated person, and the resume is only ever a human act —
// nothing automated takes this edge.
func (u *Surface) ResumeRun(ctx context.Context, actor, runID string) (json.RawMessage, error) {
	out, err := u.sk.ResumeRun(ctx, actor, runID)
	if err != nil {
		return nil, mapResumeErr(err)
	}
	return json.Marshal(out)
}

// mapResumeErr maps the resume path's errors onto transport statuses. The
// open-ask refusal carries the ask id in its message, because the answer to
// "why can't I resume this?" is "answer that card — that IS the resume".
func mapResumeErr(err error) error {
	var openAsk *ResumeOpenAskError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &openAsk):
		return surfaceErr(http.StatusConflict, "ask_open", err)
	case errors.Is(err, ErrResumeNotParked):
		return surfaceErr(http.StatusConflict, "not_parked", err)
	case errors.Is(err, ErrCancelRaced):
		// The same lost-CAS classifier the cancel path uses: the run moved
		// between the read and the transition, which the caller resolves by
		// re-reading rather than by a retry loop here.
		return surfaceErr(http.StatusConflict, "resume_raced", err)
	case errors.Is(err, run.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		return surfaceErr(http.StatusNotFound, "not_found", err)
	default:
		return err
	}
}

// Receipt implements api.IntakeSurface: the materialized receipt row
// (Spec S10.10), verbatim.
func (u *Surface) Receipt(ctx context.Context, runID string) (json.RawMessage, error) {
	var usage string
	err := u.sk.cfg.DB.QueryRowContext(ctx,
		`SELECT usage_json FROM receipts WHERE run_id = ?`, runID).Scan(&usage)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, surfaceErr(http.StatusNotFound, "not_found",
			fmt.Errorf("no receipt for run %s (receipts materialize per run-end, Spec S10.1)", runID))
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(usage), nil
}

// ---- the task view ----

type runSummary struct {
	RunID      string `json:"run_id"`
	Role       string `json:"role"`
	State      string `json:"state"`
	HasReceipt bool   `json:"has_receipt"`
}

type taskView struct {
	TaskID string `json:"task_id"`
	Title  string `json:"title"`
	Kanban string `json:"kanban_status"`
	Owner  string `json:"owner"`

	Phase     string  `json:"phase,omitempty"`
	Tier      string  `json:"tier,omitempty"`
	Family    string  `json:"family,omitempty"`
	Clearance float64 `json:"clearance,omitempty"`

	OpenAskID string          `json:"open_ask_id,omitempty"`
	OpenCard  json.RawMessage `json:"open_card,omitempty"`

	Runs []runSummary `json:"runs"`
}

// taskView assembles the demo/watch surface: task row + intake state +
// open card snapshot + role runs + receipt presence.
func (u *Surface) taskView(ctx context.Context, taskID string) (json.RawMessage, error) {
	v := taskView{TaskID: taskID}
	err := u.sk.cfg.DB.QueryRowContext(ctx,
		`SELECT title, kanban_status, user_id FROM tasks WHERE task_id = ?`, taskID).
		Scan(&v.Title, &v.Kanban, &v.Owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, surfaceErr(http.StatusNotFound, "not_found", fmt.Errorf("no task %s", taskID))
	}
	if err != nil {
		return nil, err
	}
	if st, err := u.sk.pipe.LoadState(ctx, taskID); err == nil {
		v.Phase, v.Tier, v.Family = string(st.Phase), string(st.Tier), string(st.Family)
		v.Clearance = st.Clearance
		v.OpenAskID = st.OpenAskID
		if st.OpenAskID != "" {
			var snapshot string
			if err := u.sk.cfg.DB.QueryRowContext(ctx,
				`SELECT snapshot FROM asks WHERE ask_id = ?`, st.OpenAskID).Scan(&snapshot); err == nil {
				v.OpenCard = json.RawMessage(snapshot)
			}
		}
	}
	if v.OpenAskID == "" {
		// Pipeline stages beyond intake park their durable cards in asks
		// keyed by run (verify escalations, Spec S07.7 sinks) — the view
		// shows the task's oldest open ask wherever it came from, so an
		// attention column is never reasonless. The risk-ranked inbox is
		// B6's surface (Spec S15; FC-v1); verify-class asks answer through
		// this same surface (Answer above, the S07.7 resume path). (Found
		// live at the B2 gate demo, 2026-07-20: a CAP-HIT card sat
		// invisible under "attention".)
		var askID, snapshot string
		err := u.sk.cfg.DB.QueryRowContext(ctx, `
			SELECT a.ask_id, a.snapshot FROM asks a
			  JOIN runs r ON r.run_id = a.run_id
			 WHERE r.task_id = ? AND a.status = 'open'
			 ORDER BY a.observed_ts LIMIT 1`, taskID).Scan(&askID, &snapshot)
		if err == nil {
			v.OpenAskID = askID
			v.OpenCard = json.RawMessage(snapshot)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	rows, err := u.sk.cfg.DB.QueryContext(ctx, `
		SELECT r.run_id, r.state, EXISTS (SELECT 1 FROM receipts rc WHERE rc.run_id = r.run_id)
		  FROM runs r WHERE r.task_id = ? ORDER BY r.created_ts`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rs runSummary
		var hasReceipt int
		if err := rows.Scan(&rs.RunID, &rs.State, &hasReceipt); err != nil {
			return nil, err
		}
		rs.HasReceipt = hasReceipt == 1
		if rl, ok := runRole(rs.RunID); ok {
			rs.Role = string(rl)
		}
		v.Runs = append(v.Runs, rs)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	v.Kanban = deriveKanban(v.Kanban, v.Runs)
	out, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// deriveKanban overlays the stored kanban with what the run lineage
// proves — derived, not stored (the S02.3 stalled pattern). A tombstoned
// run means the recovery ladder exhausted ⚙ recovery.max_attempts (Spec
// S02.5 step 3): the task needs eyes NOW, whatever phase the pipeline
// last recorded — a dead lineage under a green column is a finding dying
// in a log (Spec S07.7). (Found live at the B2 gate demo, 2026-07-20: a
// tombstoned verify lineage sat under kanban "verifying" indefinitely.)
//
// The overlay speaks only while the door is GENUINELY OPEN (P3-RW-14A drain
// D1). Since R1 a tombstone writes its own card and stores "attention" in the
// same transaction, so the overlay's remaining job is the row that predates
// that build — and left unconditional it now CONTRADICTS the answers its own
// card accepts: a task cancelled at the card read "attention" forever against
// a stored `cancelled`, and a retried lineage stayed "attention" while its
// successor worked and after it finished. Two conditions, both about whether
// anyone still owes a decision:
//
//   - the TASK has not ended. `done` / `cancelled` are decisions already made;
//     a dead run in the task's history may not re-open them.
//   - the tombstoned lineage was not SUPERSEDED. A successor exists only
//     because a human answered the card with `retry` (Spec S02.5 step 2 fork
//     lineage), so while it runs the board follows the work — and when it
//     COMPLETES the lineage recovered, which is the one thing a tombstone can
//     stop being. If a successor ends badly in turn, IT is a tombstone with no
//     successor of its own — and it mints its own card, so the overlay is
//     right about it.
//
// A `crashed` successor also suppresses: it is terminal-but-SUPERSEDABLE (Spec
// S02.3), the ladder owns its disposition (S02.5 step 3), and if that
// disposition ends the lineage the tombstone-review card says so.
func deriveKanban(stored string, runs []runSummary) string {
	if stored == kanbanCancelled || stored == kanbanDone {
		return stored
	}
	for _, r := range runs {
		if r.State != "tombstoned" || supersededByALaterAttempt(r.RunID, runs) {
			continue
		}
		return "attention"
	}
	return stored
}

// kanbanDone is the board column a finished task lands in (the verifyTerminal
// SHIP vocabulary, named here beside kanbanCancelled for the overlay's
// task-has-ended test).
const kanbanDone = "done"

// supersededByALaterAttempt reports whether a recovery-fork successor of runID
// took the lineage over — either by finishing its work or by still being
// driven. Successors carry the parent's id plus `.g<generation>` segments (Spec
// S02.5 step 2; the runRole strip is built on the same shape).
//
// A COMPLETED successor is the load-bearing case (P3-RW-14A drain r2 R-2): the
// answered retry RECOVERED the lineage, the pipeline moved on to its next leg,
// and an overlay that still shouted "attention" over the dead parent would
// contradict a card that has been answered and a phase that is running. Only
// the states that ended the lineage WITHOUT recovering it — finalized,
// tombstoned, died-at-gate — leave the parent's tombstone speaking, and a
// tombstoned successor speaks for itself through its own card.
func supersededByALaterAttempt(runID string, runs []runSummary) bool {
	if runID == "" {
		return false
	}
	for _, r := range runs {
		if r.RunID == runID || !strings.HasPrefix(r.RunID, runID+".g") {
			continue
		}
		switch r.State {
		case "finalized", "tombstoned", "died-at-gate":
			// Ended without recovering; it decides nothing about this tombstone.
		default:
			return true // completed, or still being driven (incl. crashed)
		}
	}
	return false
}

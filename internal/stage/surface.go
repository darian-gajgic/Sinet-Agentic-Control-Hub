package stage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// Surface adapts the skeleton to the api.IntakeSurface transport contract
// (Spec S01.3: the API seam stays thin — identity/step-up/status live in
// internal/api, pipeline behavior here). Payloads are assembled JSON; the
// pipeline's domain errors map to *api.SurfaceError statuses.
type Surface struct{ sk *Skeleton }

var _ api.IntakeSurface = (*Surface)(nil)

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
	default:
		return err
	}
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
	st, err := u.sk.pipe.Start(ctx, intake.Request{UserID: userID, Title: b.Title, Text: b.Text})
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
	kind, tier, err := u.askCardMeta(ctx, askID)
	if err != nil {
		return nil, mapIntakeErr(err)
	}
	if (kind == intake.CardApproval || kind == intake.CardDelta) && tier == intake.TierHigh && !pinVerified {
		return nil, api.ErrPINRequired
	}
	st, err := u.sk.pipe.Answer(ctx, userID, askID, answer)
	if err != nil {
		return nil, mapIntakeErr(err)
	}
	// The walking-skeleton continuation: an approved plan completes the
	// intake run and launches execution (the "what runs next is B2-4's"
	// edge of Spec S02.3's approval resume).
	if err := u.sk.afterIntake(ctx, st); err != nil {
		return nil, err
	}
	return u.taskView(ctx, st.TaskID)
}

// askCardMeta reads the durable ask's card kind + tier (asks.snapshot is
// the full card, Spec S02.2).
func (u *Surface) askCardMeta(ctx context.Context, askID string) (intake.CardKind, intake.Tier, error) {
	var snapshot string
	err := u.sk.cfg.DB.QueryRowContext(ctx,
		`SELECT snapshot FROM asks WHERE ask_id = ? AND status = 'open'`, askID).Scan(&snapshot)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", intake.ErrUnknownAsk
	}
	if err != nil {
		return "", "", err
	}
	var card struct {
		Kind intake.CardKind `json:"kind"`
		Tier intake.Tier     `json:"tier"`
	}
	if err := json.Unmarshal([]byte(snapshot), &card); err != nil {
		return "", "", fmt.Errorf("stage: decode ask snapshot: %w", err)
	}
	return card.Kind, card.Tier, nil
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
	st, err = u.sk.pipe.Advance(ctx, taskID)
	if err != nil {
		return nil, mapIntakeErr(err)
	}
	if err := u.sk.afterIntake(ctx, st); err != nil {
		return nil, err
	}
	return u.taskView(ctx, taskID)
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
// in a log (Spec S07.7). The rich tombstone-review card remains the
// recorded B0-4 deferral (B5/B6, Spec S15); this keeps the column honest
// until then. (Found live at the B2 gate demo, 2026-07-20: a tombstoned
// verify lineage sat under kanban "verifying" indefinitely.)
func deriveKanban(stored string, runs []runSummary) string {
	for _, r := range runs {
		if r.State == "tombstoned" {
			return "attention"
		}
	}
	return stored
}

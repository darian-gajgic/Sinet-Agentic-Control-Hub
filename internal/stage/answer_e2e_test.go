package stage_test

// The S07.7 resume-in-place acceptance battery (B2-5): planted-route
// end-to-end tests per answer verb over the fake-engine walking skeleton
// (the TestWalkingSkeletonE2E precedent — zero paid calls). Each test
// drives request → approval → execute → a verify drain that terminates in
// a durable CAP-HIT card with the verify run PARKED on it, then answers
// the card through the same surface the API route calls and asserts the
// ratified aftermath: resume-in-place with the generation fence, honest
// quality records, the S02.3 cancel mapping, and the repeat CAP-HIT
// re-park with full round history.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// driveToOpenCapHit walks the pipeline to a parked verify run holding an
// open CAP-HIT card (judge knob supplied via newHarness extraEnv).
func driveToOpenCapHit(t *testing.T, h *harness, owner string) (taskID, verifyRunID, askID string, card verify.Card) {
	t.Helper()
	ctx := context.Background()
	raw, err := h.sur.Submit(ctx, owner, json.RawMessage(
		`{"title":"SQLite note","text":"Write a short appreciation note about the SQLite database engine."}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	v := decodeView(t, raw)
	taskID = v.TaskID
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the intake run", n)
	}
	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	v = decodeView(t, raw)
	raw, err = h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"force_proceed":true}`), false)
	if err != nil {
		t.Fatalf("Answer(force_proceed): %v", err)
	}
	v = decodeView(t, raw)
	if _, err = h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"action":"approve"}`), true); err != nil {
		t.Fatalf("Answer(approve): %v", err)
	}
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the execute run", n)
	}
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the verify run", n)
	}

	verifyRunID = taskID + ".verify"
	if got := h.runState(t, verifyRunID); got != "parked" {
		t.Fatalf("verify run state %q, want parked on the card (S07.7/S02.3)", got)
	}
	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	v = decodeView(t, raw)
	if v.Kanban != "attention" {
		t.Fatalf("kanban %q, want attention", v.Kanban)
	}
	askID = v.OpenAskID
	if !verify.IsVerifyAskID(askID) {
		t.Fatalf("open ask %q is not verify-minted", askID)
	}
	card = h.openCard(t, askID)
	if card.Category != verify.CatCapHit {
		t.Fatalf("card category %s, want CAP-HIT", card.Category)
	}
	if len(card.Rounds) != 3 {
		t.Fatalf("card rounds = %d, want 3 (cap/convergence under the default ⚙)", len(card.Rounds))
	}
	if len(card.Choices) != 3 {
		t.Fatalf("card choices = %v, want the three S07.7 verbs", card.Choices)
	}
	return taskID, verifyRunID, askID, card
}

func (h *harness) runState(t *testing.T, runID string) string {
	t.Helper()
	var state string
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT state FROM runs WHERE run_id = ?`, runID).Scan(&state); err != nil {
		t.Fatalf("read run %s: %v", runID, err)
	}
	return state
}

func (h *harness) runGeneration(t *testing.T, runID string) int64 {
	t.Helper()
	var gen int64
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT generation FROM runs WHERE run_id = ?`, runID).Scan(&gen); err != nil {
		t.Fatalf("read run %s: %v", runID, err)
	}
	return gen
}

func (h *harness) openCard(t *testing.T, askID string) verify.Card {
	t.Helper()
	var snapshot string
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT snapshot FROM asks WHERE ask_id = ?`, askID).Scan(&snapshot); err != nil {
		t.Fatalf("read ask %s: %v", askID, err)
	}
	var c verify.Card
	if err := json.Unmarshal([]byte(snapshot), &c); err != nil {
		t.Fatalf("decode card %s: %v", askID, err)
	}
	return c
}

func (h *harness) askStatus(t *testing.T, askID string) (status, answer string) {
	t.Helper()
	var ans sql.NullString
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT status, answer FROM asks WHERE ask_id = ?`, askID).Scan(&status, &ans); err != nil {
		t.Fatalf("read ask %s: %v", askID, err)
	}
	return status, ans.String
}

func (h *harness) stateEvents(t *testing.T, runID string) []string {
	t.Helper()
	rows, err := h.db.QueryContext(context.Background(),
		`SELECT payload FROM run_events WHERE run_id = ? AND type = 'run.state_changed' ORDER BY event_seq`, runID)
	if err != nil {
		t.Fatalf("read run.state_changed events: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, p)
	}
	return out
}

func hasTransition(events []string, from, to string) bool {
	needle := fmt.Sprintf(`"from":%q,"to":%q`, from, to)
	for _, e := range events {
		if strings.Contains(e, needle) {
			return true
		}
	}
	return false
}

func TestAnswerAcceptBestEffortE2E(t *testing.T) {
	h := newHarness(t, "SINET_STAGE_FAKE_JUDGE=always-fail")
	ctx := context.Background()
	const owner = "u-operator"
	taskID, verifyRunID, askID, _ := driveToOpenCapHit(t, h, owner)

	// A live effect-journal row: accept must not touch it (the quality gate
	// is never the effects gate, Spec S07.1 — the SHIP-over-live-journal
	// precedent applied to the answer path).
	if err := h.db.WriteTx(ctx, func(tx *sql.Tx) error {
		ts := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := tx.ExecContext(ctx, `INSERT INTO effects
			(effect_id, user_id, class, payload, payload_hash, state, created_ts, updated_ts)
			VALUES ('fx-answer', ?, 'C', '{}', 'h', 'proposed', ?, ?)`,
			owner, ts, ts)
		return err
	}); err != nil {
		t.Fatalf("plant effect row: %v", err)
	}

	answer := json.RawMessage(`{"choice":"accept_best_effort","note":"good enough for tonight"}`)
	raw, err := h.sur.Answer(ctx, owner, askID, answer, false)
	if err != nil {
		t.Fatalf("Answer(accept_best_effort): %v", err)
	}
	v := decodeView(t, raw)
	if v.Kanban != "done" {
		t.Fatalf("kanban %q, want done", v.Kanban)
	}
	if got := h.runState(t, verifyRunID); got != "completed" {
		t.Fatalf("verify run state %q, want completed", got)
	}
	for _, r := range v.Runs {
		if r.RunID == verifyRunID && !r.HasReceipt {
			t.Fatalf("verify run has no receipt after accept (S10.1 receipts per run-end)")
		}
	}
	// Resume-in-place: the answer resumed through the one ratified edge —
	// parked→running bumped the generation — then completed.
	if gen := h.runGeneration(t, verifyRunID); gen != 1 {
		t.Fatalf("generation = %d, want 1 (parked→running bump, Spec S02.5)", gen)
	}
	events := h.stateEvents(t, verifyRunID)
	if !hasTransition(events, "parked", "running") || !hasTransition(events, "running", "completed") {
		t.Fatalf("run.state_changed trail missing the resume-then-complete edges: %v", events)
	}
	if status, ans := h.askStatus(t, askID); status != "answered" || !strings.Contains(ans, "accept_best_effort") {
		t.Fatalf("ask row = (%s, %s), want answered with the payload", status, ans)
	}

	// The quality record stays honest: no SHIP verdict exists, the work
	// item stays done_unverified, and the acceptance is a HUMAN decision.
	var ships int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_events WHERE run_id = ? AND type = 'verdict.recorded' AND payload LIKE '%"verdict":"SHIP"%'`,
		verifyRunID).Scan(&ships); err != nil {
		t.Fatalf("count SHIP rounds: %v", err)
	}
	if ships != 0 {
		t.Fatalf("%d SHIP verdict rows after accept_best_effort — the record must stay honest (S07.11)", ships)
	}
	doc, found, err := h.led.Current(ctx, taskID)
	if err != nil || !found {
		t.Fatalf("ledger Current: %v found=%v", err, found)
	}
	for _, item := range doc.State.Items {
		if item.ID == "S-1" && item.Status != ledger.StatusDoneUnverified {
			t.Fatalf("work item S-1 status %s, want done_unverified (SetVerified is the only path to verified)", item.Status)
		}
	}
	foundDecision := false
	for _, dec := range doc.Decisions {
		if dec.Author == ledger.AuthorHuman && strings.Contains(dec.Text, "accepted the best-effort") {
			foundDecision = true
		}
	}
	if !foundDecision {
		t.Fatalf("no human acceptance decision in the ledger: %+v", doc.Decisions)
	}

	// The effects gate is untouched.
	var state string
	if err := h.db.QueryRowContext(ctx, `SELECT state FROM effects WHERE effect_id = 'fx-answer'`).Scan(&state); err != nil {
		t.Fatalf("read effect row: %v", err)
	}
	if state != "proposed" {
		t.Fatalf("effect row state %q after accept, want proposed (S07.1: verdict/answer releases nothing)", state)
	}
}

func TestAnswerReviseWithGuidanceToShipE2E(t *testing.T) {
	h := newHarness(t, "SINET_STAGE_FAKE_JUDGE=fail-unless-guidance")
	ctx := context.Background()
	const owner = "u-operator"
	taskID, verifyRunID, askID, _ := driveToOpenCapHit(t, h, owner)

	answer := json.RawMessage(`{"choice":"revise_with_guidance","guidance":[{"text":"apply the agreed marker line","criterion":"AC-1"}]}`)
	raw, err := h.sur.Answer(ctx, owner, askID, answer, false)
	if err != nil {
		t.Fatalf("Answer(revise_with_guidance): %v", err)
	}
	v := decodeView(t, raw)
	if v.Kanban != "done" {
		t.Fatalf("kanban %q, want done (guidance rework satisfied the judge)", v.Kanban)
	}
	if got := h.runState(t, verifyRunID); got != "completed" {
		t.Fatalf("verify run state %q, want completed", got)
	}
	if gen := h.runGeneration(t, verifyRunID); gen != 1 {
		t.Fatalf("generation = %d, want 1", gen)
	}
	if status, _ := h.askStatus(t, askID); status != "answered" {
		t.Fatalf("original ask status %q, want answered", status)
	}
	if v.OpenAskID != "" {
		t.Fatalf("an ask is still open after the SHIP resume: %s", v.OpenAskID)
	}

	// The resumed drain judged the guidance rework as round 4 and SHIPPED;
	// the ledger item verified through the ratified SHIP path.
	var payload string
	if err := h.db.QueryRowContext(ctx,
		`SELECT payload FROM run_events WHERE run_id = ? AND type = 'verdict.recorded' ORDER BY event_seq DESC LIMIT 1`,
		verifyRunID).Scan(&payload); err != nil {
		t.Fatalf("read verdict.recorded: %v", err)
	}
	if !strings.Contains(payload, `"verdict":"SHIP"`) || !strings.Contains(payload, `"round":4`) {
		t.Fatalf("last verdict.recorded = %s, want SHIP at round 4 (continued numbering)", payload)
	}
	doc, _, err := h.led.Current(ctx, taskID)
	if err != nil {
		t.Fatalf("ledger Current: %v", err)
	}
	verified := false
	for _, item := range doc.State.Items {
		if item.ID == "S-1" && item.Status == ledger.StatusVerified {
			verified = true
		}
	}
	if !verified {
		t.Fatalf("work item S-1 not verified after the guidance SHIP")
	}
	// The guidance revision persisted to the artifact store with the marker.
	rev4, err := os.ReadFile(filepath.Join(h.artifactRoot, taskID, "deliverable-rev4.md"))
	if err != nil || !strings.Contains(string(rev4), "GUIDANCE-APPLIED") {
		t.Fatalf("persisted rev4 = (%v, %q), want the guidance marker", err, rev4)
	}
}

func TestAnswerReviseRecurrentCapHitReparksE2E(t *testing.T) {
	h := newHarness(t, "SINET_STAGE_FAKE_JUDGE=always-fail")
	ctx := context.Background()
	const owner = "u-operator"
	taskID, verifyRunID, askID, _ := driveToOpenCapHit(t, h, owner)

	// A superseded stage session: it asserted generation 0 at spawn; the
	// answer's resume bumps the run to generation 1 and the fence must
	// reject its writes (Spec S02.5 step 4; CONVENTIONS §13 fencing split).
	zombie := h.led.SessionVerbs(verifyRunID, "verify", h.runGeneration(t, verifyRunID))

	answer := json.RawMessage(`{"choice":"revise_with_guidance","guidance":[{"text":"apply the agreed marker line","criterion":"AC-1"}]}`)
	raw, err := h.sur.Answer(ctx, owner, askID, answer, false)
	if err != nil {
		t.Fatalf("Answer(revise_with_guidance): %v", err)
	}
	v := decodeView(t, raw)

	// The guidance could not satisfy the judge: the FRESH bounded budget
	// ran (rounds 4–5 under ⚙ defaults) and re-parked on a recurrent
	// CAP-HIT carrying the FULL history.
	if got := h.runState(t, verifyRunID); got != "parked" {
		t.Fatalf("verify run state %q, want parked again", got)
	}
	if v.Kanban != "attention" {
		t.Fatalf("kanban %q, want attention", v.Kanban)
	}
	if status, _ := h.askStatus(t, askID); status != "answered" {
		t.Fatalf("original ask status %q, want answered", status)
	}
	if v.OpenAskID == "" || v.OpenAskID == askID || !verify.IsVerifyAskID(v.OpenAskID) {
		t.Fatalf("open ask after re-park = %q, want a NEW verify card", v.OpenAskID)
	}
	recard := h.openCard(t, v.OpenAskID)
	if recard.Category != verify.CatCapHit || len(recard.Rounds) != 5 {
		t.Fatalf("re-park card = %s with %d rounds, want CAP-HIT with 5 (3 carried + 2 resumed)", recard.Category, len(recard.Rounds))
	}
	for i, r := range recard.Rounds {
		if r.Round != i+1 {
			t.Fatalf("round numbering broken: %+v", recard.Rounds)
		}
	}
	if gen := h.runGeneration(t, verifyRunID); gen != 1 {
		t.Fatalf("generation = %d, want 1 after one resume", gen)
	}

	// The fence: the superseded session's ledger write is rejected.
	if _, err := zombie.Note(ctx, "zombie write from the pre-park session"); !errors.Is(err, eventlog.ErrStaleGeneration) {
		t.Fatalf("superseded session write err = %v, want ErrStaleGeneration (Spec S02.5)", err)
	}

	// The re-parked card is itself answerable: accept resolves the lineage.
	raw, err = h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"choice":"accept_best_effort"}`), false)
	if err != nil {
		t.Fatalf("Answer(accept on re-park): %v", err)
	}
	v = decodeView(t, raw)
	if v.Kanban != "done" || h.runState(t, verifyRunID) != "completed" {
		t.Fatalf("after accepting the re-parked card: kanban %q state %q", v.Kanban, h.runState(t, verifyRunID))
	}
	if gen := h.runGeneration(t, verifyRunID); gen != 2 {
		t.Fatalf("generation = %d, want 2 after the second resume", gen)
	}
	_ = taskID
}

func TestAnswerCancelE2E(t *testing.T) {
	h := newHarness(t, "SINET_STAGE_FAKE_JUDGE=always-fail")
	ctx := context.Background()
	const owner = "u-operator"
	taskID, verifyRunID, askID, _ := driveToOpenCapHit(t, h, owner)

	answer := json.RawMessage(`{"choice":"cancel","note":"not worth more rounds"}`)
	raw, err := h.sur.Answer(ctx, owner, askID, answer, false)
	if err != nil {
		t.Fatalf("Answer(cancel): %v", err)
	}
	v := decodeView(t, raw)
	// The ratified S02.3 mapping (CONVENTIONS §14 reading 9): the parked
	// run finalizes with its card — no resume edge, no generation bump.
	if got := h.runState(t, verifyRunID); got != "finalized" {
		t.Fatalf("verify run state %q, want finalized (finalize-with-card)", got)
	}
	if gen := h.runGeneration(t, verifyRunID); gen != 0 {
		t.Fatalf("generation = %d, want 0 (cancel takes no resume edge)", gen)
	}
	if v.Kanban != "cancelled" {
		t.Fatalf("kanban %q, want cancelled", v.Kanban)
	}
	for _, r := range v.Runs {
		if r.RunID == verifyRunID && !r.HasReceipt {
			t.Fatalf("finalized verify run has no receipt (S10.1 receipts per run-end)")
		}
	}
	if status, ans := h.askStatus(t, askID); status != "answered" || !strings.Contains(ans, "cancel") {
		t.Fatalf("ask row = (%s, %s), want answered with the cancel payload", status, ans)
	}
	doc, _, err := h.led.Current(ctx, taskID)
	if err != nil {
		t.Fatalf("ledger Current: %v", err)
	}
	foundDecision := false
	for _, dec := range doc.Decisions {
		if dec.Author == ledger.AuthorHuman && strings.Contains(dec.Text, "cancelled at the CAP-HIT card") {
			foundDecision = true
		}
	}
	if !foundDecision {
		t.Fatalf("no human cancel decision in the ledger: %+v", doc.Decisions)
	}

	// The consumed ask is gone from the answer surface.
	if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"choice":"cancel"}`), false); err == nil {
		t.Fatal("answering an already-answered ask must fail")
	} else if se := new(api.SurfaceError); !errors.As(err, &se) || se.Status != 404 {
		t.Fatalf("re-answer err = %v, want a 404 SurfaceError", err)
	}
}

func TestAnswerVerifyNegativePaths(t *testing.T) {
	h := newHarness(t, "SINET_STAGE_FAKE_JUDGE=always-fail")
	ctx := context.Background()
	const owner = "u-operator"
	_, _, askID, _ := driveToOpenCapHit(t, h, owner)

	cases := []struct {
		name       string
		user, ask  string
		answer     string
		wantStatus int
	}{
		{"not the requester", "u-other", askID, `{"choice":"cancel"}`, 403},
		{"unknown ask", owner, "ask-verify-deadbeef00000000", `{"choice":"cancel"}`, 404},
		{"unsupported verb", owner, askID, `{"choice":"fix_suite"}`, 400},
		{"revise without guidance", owner, askID, `{"choice":"revise_with_guidance"}`, 400},
		{"missing choice", owner, askID, `{}`, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.sur.Answer(ctx, tc.user, tc.ask, json.RawMessage(tc.answer), false)
			se := new(api.SurfaceError)
			if err == nil || !errors.As(err, &se) || se.Status != tc.wantStatus {
				t.Fatalf("err = %v, want SurfaceError status %d", err, tc.wantStatus)
			}
		})
	}
	// Nothing moved: the card is still open, the run still parked.
	if status, _ := h.askStatus(t, askID); status != "open" {
		t.Fatalf("ask status %q after rejected answers, want open", status)
	}
}

package stage_test

// verifywedge_rw14_e2e_test.go — P3-RW-14 acceptance tests, committed RED
// (CONVENTIONS §3 Amendment-A: the grounding agent commits the brief's
// acceptance tests failing; the packet's implementation commit restores
// green).
//
// Live proof (builder world, 2026-08-16, task t-4b982b38a62baee5 "The GPU
// parts shop"): a SOFTWARE-family task reached its verify leg and the drain
// crashed with "verify: launch-domain deliverable without a domain check
// pack (Spec S07.8)" (event_seq 1051); the ladder forked the crash three
// times into the same deterministic error and tombstoned the lineage
// (event_seq 1075); the task sat at kanban "verifying" with ZERO open asks.
// Every landed e2e walk answers the family gate "generic" — the launch
// domain, the one v0 actually ships, was walked by no test.
//
// Spec anchors: S07.2 ("a screen never blocks on its own failure, a screen
// outage escalates rather than approves"); S07.7 (every drain terminal is a
// human-visible sink; P-T06-4 escalation-path death); S02.3 (parked =
// suspended on an open gate/ask); S06.5 (assumptions are disclosed to the
// requester — in requester words, not internal tokens; CONVENTIONS §57
// drafting rule 1).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// answerFamily answers the S06.5 family gate with the given family (the
// familywalk helper answers "generic"; this walk wants the launch domain —
// "a walk that wants a different family answers for itself").
func answerFamily(t *testing.T, ctx context.Context, h *harness, owner string, raw json.RawMessage, family intake.Family) json.RawMessage {
	t.Helper()
	v := decodeView(t, raw)
	if v.OpenCard.Kind != string(intake.CardFamily) {
		t.Fatalf("expected the family card, got %s", raw)
	}
	next, err := h.sur.Answer(ctx, owner, v.OpenAskID,
		json.RawMessage(`{"choice":"`+string(family)+`"}`), false)
	if err != nil {
		t.Fatalf("answer the family question: %v", err)
	}
	return next
}

// TestRW14LaunchDomainVerifyNeverWedgesSilently: a software-family (launch
// domain) task whose world has NO check pack wired must NOT crash-fork-
// tombstone at verify. The drain's inability to run is a verification-
// infrastructure outage: it MUST terminate in a durable card the owner can
// answer (S07.2 outage-escalates; S07.7 sink-types-only), the verify run
// MUST park on that ask (S02.3), and the task MUST surface as needing a
// human (kanban "attention" — the verifyTerminal card branch).
func TestRW14LaunchDomainVerifyNeverWedgesSilently(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	const owner = "u-operator"

	raw, err := h.sur.Submit(ctx, owner, json.RawMessage(
		`{"title":"SQLite note","text":"Write a short appreciation note about the SQLite database engine."}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	taskID := decodeView(t, raw).TaskID

	// Intake dispatch → the family gate → SOFTWARE (the launch domain).
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the intake run", n)
	}
	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	raw = answerFamily(t, ctx, h, owner, raw, intake.FamilySoftware)
	v := decodeView(t, raw)
	if v.OpenCard.Kind != "interview" {
		t.Fatalf("expected an open interview card, got %s", raw)
	}

	// Force-proceed → approval → approve (PIN step-up).
	raw, err = h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"force_proceed":true}`), false)
	if err != nil {
		t.Fatalf("Answer(force_proceed): %v", err)
	}
	v = decodeView(t, raw)
	if v.OpenCard.Kind != "approval" {
		t.Fatalf("expected the approval card, got %s", raw)
	}
	if _, err := h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"action":"approve"}`), true); err != nil {
		t.Fatalf("Answer(approve): %v", err)
	}

	// Execute, then verify — the leg that wedged the live world.
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the execute run", n)
	}
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the verify run", n)
	}

	// The verify lineage is NEVER left crashed (the ladder would fork the
	// same deterministic refusal until tombstone): it parks on the card.
	rows, err := h.db.QueryContext(ctx,
		`SELECT run_id, state FROM runs WHERE task_id = ? AND run_id LIKE '%.verify%'`, taskID)
	if err != nil {
		t.Fatalf("query verify runs: %v", err)
	}
	defer rows.Close()
	var sawParked bool
	for rows.Next() {
		var id, state string
		if err := rows.Scan(&id, &state); err != nil {
			t.Fatal(err)
		}
		if state == "crashed" || state == "tombstoned" {
			t.Fatalf("verify run %s is %q — the launch-domain no-pack refusal took the crash path (Spec S07.2: a screen outage escalates rather than approves)", id, state)
		}
		if state == "parked" {
			sawParked = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !sawParked {
		t.Fatal("no verify run parked on the infrastructure card (Spec S02.3 parked-on-ask; S07.7 resumable escalation)")
	}

	// The door is a durable OPEN ask on the task (S07.7: no other sink type
	// exists; a crash-tombstone loop with no ask is a finding dying in a log).
	var open int
	if err := h.db.QueryRowContext(ctx,
		`SELECT count(*) FROM asks a JOIN runs r ON r.run_id = a.run_id
		 WHERE r.task_id = ? AND a.status = 'open'`, taskID).Scan(&open); err != nil {
		t.Fatalf("count open asks: %v", err)
	}
	if open == 0 {
		t.Fatal("launch-domain verify left NO open ask — the task wedges silently at verifying")
	}

	// And the board says so: kanban "attention", not a forever-"verifying".
	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	if got := decodeView(t, raw).Kanban; got != "attention" {
		t.Fatalf("kanban = %q, want %q (S15.5 blocked-on-a-human)", got, "attention")
	}
}

// TestRW14ForcedAssumptionsSpeakPlainly: the assumptions minted when a
// requester force-proceeds are requester-facing prose (they render on the
// approval card's assumptions list and in the Understood recap) and MUST NOT
// leak the raw internal origin token — the live world served
// "(force_proceed) Comparison Rules — assumed: proceeding without an answer"
// into operator prose (asks row intake:t-4b982b38a62baee5:4, offset 4152).
// The structural origin already rides the artifact's Origin field; the prose
// is for the person (S06.5 disclosed assumptions; CONVENTIONS §57 drafting
// rule 1: requester-facing strings are written for someone who is not a
// programmer).
func TestRW14ForcedAssumptionsSpeakPlainly(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	const owner = "u-operator"

	raw, err := h.sur.Submit(ctx, owner, json.RawMessage(
		`{"title":"SQLite note","text":"Write a short appreciation note about the SQLite database engine."}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	taskID := decodeView(t, raw).TaskID
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("tick dispatched %d, want the intake run", n)
	}
	raw, err = h.sur.Task(ctx, taskID)
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	raw = clearFamilyGate(t, ctx, h.sur, owner, raw)
	v := decodeView(t, raw)
	if v.OpenCard.Kind != "interview" {
		t.Fatalf("expected an open interview card, got %s", raw)
	}
	if _, err := h.sur.Answer(ctx, owner, v.OpenAskID, json.RawMessage(`{"force_proceed":true}`), false); err != nil {
		t.Fatalf("Answer(force_proceed): %v", err)
	}

	st, err := h.sk.Pipeline().LoadState(ctx, taskID)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	var checked int
	for _, r := range st.Resolutions {
		if r.How != intake.ResolvedAssumption {
			continue
		}
		checked++
		for _, tok := range []string{"force_proceed", "(band)"} {
			if strings.Contains(r.Assumption, tok) {
				t.Errorf("assumption for slot %q leaks the raw internal token %q into requester prose: %q", r.SlotID, tok, r.Assumption)
			}
		}
	}
	if checked == 0 {
		t.Fatal("force-proceed minted no assumption resolutions — the walk did not exercise the leak site")
	}
}

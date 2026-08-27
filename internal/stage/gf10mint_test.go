package stage_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
)

// gf10mint_test.go — P3-GF10 acceptance spec, committed RED at grounding
// (the Amendment-A carve-out; closed by the GF10 implementation commit).
// Brief: P3/briefs/P3-GF10.md §9. Walk evidence:
// P3/design/walkf1-findings-2026-08-27.md — the walk world's revision row is
// run_id = <task>.verify while the settled routing decision lives only on
// <task>.execute, so the accept's attribution had no facts to render from.

// TestGF10DrainMintCarriesTheProducingRun drives the REAL composition (the
// round1_e2e harness: real stores, real project topology, fake engine, $0) from
// onboarding through execute and verify, and asserts the drain-minted revision
// carries BOTH run refs: the minting run stays the verify leg (S13.1 — the
// drain's own events and tax ride it, S07.11), and the producing run — the run
// whose settled S08.8 selection made the content — is carried beside it, which
// is what the S13.6 accept resolves truthful trailers through.
//
// RED until GF10 lands: ProducedBy is inert type surface today, so the minted
// revision records no producing run. The minting-run and routing-row
// assertions are green today — the non-tautological controls proving the
// facts the fix must join EXIST on the execute run.
func TestGF10DrainMintCarriesTheProducingRun(t *testing.T) {
	h := newProjectHarness(t)
	ctx := context.Background()
	const owner = "u-operator"

	// A registered project, so the execute leg gets a worktree and the mint a
	// snapshot pin — the PUSH-arm shape the walk world proves (repo-backed).
	src := gitFixture(t, map[string]string{"go.mod": "module shop\n"})
	if _, err := h.proj.OnboardStart(ctx, project.OnboardInput{ProjectID: "shop", Owner: owner, Name: "shop", Source: src}); err != nil {
		t.Fatalf("OnboardStart: %v", err)
	}
	if err := h.proj.OnboardApprove(ctx, "shop", owner, nil); err != nil {
		t.Fatalf("OnboardApprove: %v", err)
	}
	raw, err := h.sur.Submit(ctx, owner, json.RawMessage(
		`{"title":"shop note","text":"Write a short appreciation note in the shop project."}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	taskID := decodeView(t, raw).TaskID
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("intake tick dispatched %d", n)
	}
	raw = clearFamilyGate(t, ctx, h.sur, owner, mustTask(t, h, taskID))
	raw, _ = h.sur.Answer(ctx, owner, decodeView(t, raw).OpenAskID, json.RawMessage(`{"force_proceed":true}`), false)
	if _, err := h.sur.Answer(ctx, owner, decodeView(t, raw).OpenAskID, json.RawMessage(`{"action":"approve"}`), true); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("execute tick dispatched %d", n)
	}
	if n := h.tick(ctx); n != 1 {
		t.Fatalf("verify tick dispatched %d", n)
	}

	rev, err := h.review.RevisionAt(ctx, "dlv-"+taskID, 1)
	if err != nil {
		t.Fatalf("RevisionAt: %v", err)
	}
	executeRun, verifyRun := taskID+stage.RunSuffixExecute, taskID+stage.RunSuffixVerify

	// Control (green today): the MINTING run stays the verify leg — the
	// verification handoff mints, and its ref is never rewritten (S13.1).
	if rev.RunID != verifyRun {
		t.Fatalf("the minting run must stay the verify leg %q, got %q", verifyRun, rev.RunID)
	}
	// Control (green today): the attribution facts EXIST, one run over — the
	// execute dispatch emitted the settled routing.decided (S08.8; LN-9 R9).
	var routingRows int
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM run_events WHERE run_id = ? AND type = 'routing.decided'`,
		executeRun).Scan(&routingRows); err != nil {
		t.Fatalf("count routing.decided: %v", err)
	}
	if routingRows < 1 {
		t.Fatalf("no routing.decided on %q — the producing run's facts are missing, the fix has nothing to join", executeRun)
	}

	// RED (P3-GF10 R1/R2): the drain-minted revision carries the producing run.
	if rev.ProducedBy != executeRun {
		t.Errorf("the minted revision must carry the producing run %q (got %q): without it the S13.6 accept "+
			"resolves the verify run, finds no routing decision, and a correct deliverable can never be accepted "+
			"(WALK-F1 errand 5)", executeRun, rev.ProducedBy)
	}
}

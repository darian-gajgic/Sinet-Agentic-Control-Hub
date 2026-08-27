package api_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// gf10accept_test.go — P3-GF10 acceptance specs, committed RED at grounding
// (the Amendment-A carve-out; the window is closed by the GF10 implementation
// commit). Brief: P3/briefs/P3-GF10.md. Walk evidence:
// P3/design/walkf1-findings-2026-08-27.md §"The wall, precisely" plus the walk
// world's own rows (~/.sinet-walkf1/platform.db, quoted in the brief §1).
//
// THE SHAPE UNDER TEST IS THE PRODUCTION SHAPE. mkRun (deliverables_test.go)
// seeds routing.decided AND the engine session onto the ONE run the revision is
// minted with — a shape the real pipeline never produces. In production the
// drain mints every revision with run_id = the VERIFY run (<task>.verify),
// while the settled routing decision lives on the EXECUTE run and only there
// (routing.decided is emitted on the execute dispatch alone; P3-LN-9 R9 stamps
// ride the same leg). mkProductionRuns seeds exactly that two-run shape.

// mkProductionRuns seeds the run shape the real pipeline leaves behind for a
// task: an execute run carrying the settled routing.decided (model
// claude-sonnet-5, lane anthropic — the walk's own receipts) plus its engine
// session, and a verify run carrying an engine session ONLY (the drain's judge
// sessions record substrate, never a routing decision).
func mkProductionRuns(t *testing.T, e *dlvEnv, taskID, owner string) (executeRun, verifyRun string) {
	t.Helper()
	executeRun, verifyRun = taskID+".execute", taskID+".verify"
	e.exec(`INSERT OR IGNORE INTO tasks (task_id, user_id, title, kanban_status, created_ts) VALUES (?, ?, ?, 'new', ?)`,
		taskID, owner, taskID, dlvNow())
	runs := run.NewStore(e.b.db, e.b.log)
	for _, id := range []string{executeRun, verifyRun} {
		if _, err := runs.Create(e.ctx, run.NewRun{
			ID: id, UserID: owner, TaskID: taskID, Substrate: "claude-cli", Lane: "anthropic",
		}); err != nil {
			t.Fatalf("create run %s: %v", id, err)
		}
		e.exec(`INSERT INTO engine_sessions (run_id, user_id, substrate, engine_session_id, created_ts, updated_ts)
		        VALUES (?, ?, 'claude-cli', ?, ?, ?)`, id, owner, id+"-sess", dlvNow(), dlvNow())
	}
	if _, err := e.b.log.Append(e.ctx, eventlog.Append{
		RunID: executeRun, UserID: owner, Type: "routing.decided", SchemaVersion: 1,
		Payload: json.RawMessage(`{"model":"claude-sonnet-5","lane":"anthropic","worker":"generalist","generalist":true}`),
	}); err != nil {
		t.Fatalf("append routing.decided: %v", err)
	}
	return executeRun, verifyRun
}

// TestGF10AcceptCompletesOnAProductionShapedRevision is WALK-F1 errand 5 at the
// wire (GF10 checklist item 3): a repo-backed revision minted the way the drain
// mints it — run_id = the verify run, the producing run carried beside it —
// must serve an OPEN accept card whose trailers are rendered from the EXECUTE
// run's recorded facts, and the accept must land them into the squash commit.
//
// RED until GF10 lands: today the revision's ProducedBy member is inert, the
// provenance resolves the verify run, finds no routing decision, and the card
// closes with empty trailers — the exact wall the walker hit.
func TestGF10AcceptCompletesOnAProductionShapedRevision(t *testing.T) {
	e := newDlvEnv(t)
	e.prepareProject("proj", "alice", map[string]string{"a.txt": "base\n"})
	snap := e.snapshotCandidate("proj", "pipe", "a.txt", "candidate\n")
	executeRun, verifyRun := mkProductionRuns(t, e, "t-g", "alice")
	if _, err := e.rev.EnsureDeliverable(e.ctx, review.EnsureInput{
		ID: "d-g", Owner: "alice", TaskID: "t-g", ProjectID: "proj", Type: "markdown",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.rev.MintRevision(e.ctx, review.MintInput{
		DeliverableID: "d-g", N: 1, RunID: verifyRun, ProducedBy: executeRun,
		AttemptRef: verifyRun + "#round-1",
		Files:      map[string]string{"a.txt": "candidate\n"}, SnapshotSHA: snap,
	}); err != nil {
		t.Fatal(err)
	}

	card := readAcceptCard(t, e, "alice", "d-g")
	if !card.Acceptable {
		t.Fatalf("a drain-shaped revision whose producing run records the routing facts must be acceptable, got %+v", card)
	}
	// The trailer inputs are the EXECUTE run's platform facts (S13.6 step 3 by
	// TRUTH): engine from the substrate, model+lane from routing.decided.
	p := card.Provenance
	if p.Engine != "claude-cli" || p.Model != "claude-sonnet-5" || p.Lane != "anthropic" {
		t.Errorf("attribution must resolve the producing run's facts, got %+v", p)
	}
	if p.VendorNoreply != "anthropic@vendor.noreply.sinet.invalid" {
		t.Errorf("the vendor address derives structurally from the lane, got %q", p.VendorNoreply)
	}
	// The MINTING ref is not rewritten: the verification handoff stays the
	// minting run (S13.1); the producing run is carried BESIDE it.
	if p.MintingRunID != verifyRun {
		t.Errorf("the minting run must stay the verify run %q, got %q", verifyRun, p.MintingRunID)
	}
	wantCo := "Co-Authored-By: claude-cli claude-sonnet-5 <anthropic@vendor.noreply.sinet.invalid>"
	if !strings.Contains(card.Trailers, wantCo) ||
		!strings.Contains(card.Trailers, "Assisted-by: claude-cli (claude-sonnet-5) via Sinet") {
		t.Errorf("the card must render the truthful trailers verbatim, got %q", card.Trailers)
	}

	// The act completes: one attributed commit whose message carries exactly
	// the trailers the reviewer was shown, and the deliverable leaves in-review.
	var out api.AcceptOutcome
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/deliverables/d-g/accept",
		acceptBody(card.PayloadHash, dlvPIN))), &out); err != nil {
		t.Fatalf("decode accept outcome: %v", err)
	}
	if !out.Applied || out.Commit == "" || out.State != review.StateAccepted {
		t.Fatalf("the accept did not complete: %+v", out)
	}
	entry, err := e.proj.Get(e.ctx, "proj")
	if err != nil {
		t.Fatal(err)
	}
	if msg := e.git("-C", entry.StorePath, "show", "-s", "--format=%B", out.Commit); !strings.Contains(msg, wantCo) {
		t.Errorf("the commit must carry the truthful trailers, got %q", msg)
	}
}

// TestGF10WithheldWhyNeverOverclaims pins R4 (GF10 checklist item 4): when
// accept IS withheld, the served reason names exactly the absent facts. The
// fixture is the walk shape minus the fix: the minting verify run HAS a
// recorded engine substrate (the walk world's verify run does — engine_sessions
// row, substrate claude-cli) and lacks only the settled routing decision; no
// producing run is recorded. The act stays closed (control, green today) — but
// the reason must not claim "no engine substrate" when a substrate row exists,
// which the walker's door-state sentence did (walk record §"The wall").
//
// RED until GF10 lands: today's one-size sentence asserts both absences
// unconditionally.
func TestGF10WithheldWhyNeverOverclaims(t *testing.T) {
	e := newDlvEnv(t)
	e.prepareProject("projw", "alice", map[string]string{"a.txt": "base\n"})
	snap := e.snapshotCandidate("projw", "pipe", "a.txt", "candidate\n")
	const taskID, verifyRun = "t-w", "t-w.verify"
	e.exec(`INSERT OR IGNORE INTO tasks (task_id, user_id, title, kanban_status, created_ts) VALUES (?, ?, ?, 'new', ?)`,
		taskID, "alice", taskID, dlvNow())
	runs := run.NewStore(e.b.db, e.b.log)
	if _, err := runs.Create(e.ctx, run.NewRun{
		ID: verifyRun, UserID: "alice", TaskID: taskID, Substrate: "claude-cli", Lane: "anthropic",
	}); err != nil {
		t.Fatal(err)
	}
	e.exec(`INSERT INTO engine_sessions (run_id, user_id, substrate, engine_session_id, created_ts, updated_ts)
	        VALUES (?, ?, 'claude-cli', ?, ?, ?)`, verifyRun, "alice", verifyRun+"-sess", dlvNow(), dlvNow())
	if _, err := e.rev.EnsureDeliverable(e.ctx, review.EnsureInput{
		ID: "d-w", Owner: "alice", TaskID: taskID, ProjectID: "projw", Type: "markdown",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.rev.MintRevision(e.ctx, review.MintInput{
		DeliverableID: "d-w", N: 1, RunID: verifyRun, AttemptRef: verifyRun + "#round-1",
		Files: map[string]string{"a.txt": "candidate\n"}, SnapshotSHA: snap,
	}); err != nil {
		t.Fatal(err)
	}

	card := readAcceptCard(t, e, "alice", "d-w")
	// Controls, green today: with no resolvable model the push arm stays
	// honestly closed, with a served reason and no rendered trailers.
	if card.Acceptable || card.Trailers != "" {
		t.Fatalf("unresolvable attribution must keep the push arm closed with no trailers: %+v", card)
	}
	if card.Reason == "" {
		t.Fatal("a closed accept must carry its why as data (B6-2A D9)")
	}
	// RED: the reason must not assert an absence the database contradicts —
	// this run RECORDS an engine substrate; only the routing decision is absent.
	if strings.Contains(card.Reason, "no engine substrate") {
		t.Errorf("the withheld-why overclaims: a substrate row is recorded for %s, yet the reason says %q",
			verifyRun, card.Reason)
	}
}

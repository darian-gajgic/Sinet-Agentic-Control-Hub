package accept_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// gf11remoteless_test.go — P3-GF11 acceptance spec, committed RED at grounding
// (the Amendment-A carve-out; the window is closed by the GF11 implementation
// commit). Brief: P3/briefs/P3-GF11.md. Walk evidence:
// P3/design/e5-rewalk-findings-2026-08-27.md — three identical 500s
// `accept: broker push: broker: push with no remote` (internal/broker/git.go:66)
// on a New-project-door project ("no remote — local store only").
//
// THE SHAPE UNDER TEST IS THE DOOR'S SHAPE. Every landed accept fixture
// onboards with a seeded file:// remote; the New-project door can express no
// clone source over HTTP and stores remote_url as OPTIONAL data
// (internal/shell/project_seams.go:163-166), so a door-minted project is a
// fresh init store with remote_url "" — a shape no accept test ever met until
// the walker did. This fixture runs the REAL in-process broker (newFix's own
// client), so today the accept fails at the exact production seam.

// prepareLocalOnly onboards a project the way the New-project door does —
// Source "" (fresh init store, one init commit) and RemoteURL "" (the walker's
// "decide later") — then mints an in-review repo-backed deliverable whose
// candidate is a run-branch snapshot commit, and returns the accept Input.
func (f *fix) prepareLocalOnly(candidateBody string) accept.Input {
	f.t.Helper()
	ctx := f.ctx
	if _, _, err := f.proj.Onboard(ctx, project.OnboardInput{
		ProjectID: "proj-local", Owner: "u1", Name: "Plocal", Source: "", RemoteURL: "",
	}); err != nil {
		f.t.Fatalf("Onboard (local-only): %v", err)
	}
	if _, err := f.proj.Approve(ctx, "proj-local", "u1", nil); err != nil {
		f.t.Fatalf("Approve: %v", err)
	}
	ws, err := f.proj.EnsureWorkspace(ctx, "proj-local", "pipe1")
	if err != nil {
		f.t.Fatalf("EnsureWorkspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ws.Path, "note.md"), []byte(candidateBody), 0o600); err != nil {
		f.t.Fatal(err)
	}
	candidate, err := f.proj.Snapshot(ctx, ws.Path)
	if err != nil {
		f.t.Fatalf("Snapshot: %v", err)
	}
	if err := f.proj.CreateRevisionRef(ctx, "proj-local", review.RevisionRef("dlv-local", 1), candidate); err != nil {
		f.t.Fatalf("CreateRevisionRef: %v", err)
	}
	f.mkTaskRun("t-local", "r-local", "u1")
	if _, err := f.rev.EnsureDeliverable(ctx, review.EnsureInput{
		ID: "dlv-local", Owner: "u1", TaskID: "t-local", ProjectID: "proj-local", Type: "markdown",
	}); err != nil {
		f.t.Fatalf("EnsureDeliverable: %v", err)
	}
	if _, err := f.rev.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv-local", N: 1, RunID: "r-local", AttemptRef: "r-local#round-1",
		Files: map[string]string{"note.md": candidateBody}, SnapshotSHA: candidate,
	}); err != nil {
		f.t.Fatalf("MintRevision: %v", err)
	}
	return accept.Input{
		DeliverableID: "dlv-local", AcceptingUser: "u1", AcceptingUserName: "User One",
		ProjectID: "proj-local", Subject: "docs: thank-you note",
		Engine: "claude-cli", Model: "claude-sonnet-5",
		VendorNoreply: "anthropic@vendor.noreply.sinet.invalid",
	}
}

// TestGF11AcceptLandsOnARemotelessStore is E5-REWALK errand 5 at the
// orchestration seam (GF11 checklist item 2): on a deliberately local-only
// store the local commit IS the official landing (S13.6 steps 1-3 + S13.7's
// no-repo project; brief §2-§3) — the accept must complete, advance the store's
// default branch to the attributed commit, settle the deliverable, and journal
// the landing as the class-A accept effect (brief R2). No push exists to
// request, and the real broker in this fixture would refuse one.
//
// RED until GF11 lands: today the execute phase composes a broker push with
// Remote "" and the broker refuses it — Accept returns
// `accept: broker push: broker: push with no remote`, the walker's exact wall.
func TestGF11AcceptLandsOnARemotelessStore(t *testing.T) {
	f := newFix(t)
	in := f.prepareLocalOnly("Thank you for your orders this year.\n")

	entry, err := f.proj.Get(f.ctx, "proj-local")
	if err != nil {
		t.Fatal(err)
	}
	// Controls, green today: the registry honestly records NO remote (the
	// New-project-door shape), and the store's default branch sits on the init
	// commit — so a landing is observable as a head move.
	if entry.RemoteURL != "" {
		t.Fatalf("fixture: expected a local-only entry, got remote %q", entry.RemoteURL)
	}
	headBefore := f.git("-C", entry.StorePath, "rev-parse", "refs/heads/main")

	out, err := f.acc.Accept(f.ctx, in)
	if err != nil {
		t.Fatalf("an accept on a deliberately local-only store must LAND — the local commit into the "+
			"project's store is the official landing (S13.6/S13.7; brief R1) — got: %v", err)
	}
	if !out.Accepted || out.Commit == "" || out.Card != nil {
		t.Fatalf("the local landing must accept with a commit and no merge card: %+v", out)
	}

	// The official copy is on the store's default branch: one attributed commit,
	// carrying the shown trailers verbatim.
	head := f.git("-C", entry.StorePath, "rev-parse", "refs/heads/main")
	if head != out.Commit {
		t.Errorf("the store's default branch must advance to the accept commit: head %s, commit %s", head, out.Commit)
	}
	if head == headBefore {
		t.Errorf("the default branch did not move — nothing landed")
	}
	msg := f.git("-C", entry.StorePath, "show", "-s", "--format=%B", out.Commit)
	wantCo := "Co-Authored-By: claude-cli claude-sonnet-5 <anthropic@vendor.noreply.sinet.invalid>"
	if !strings.Contains(msg, wantCo) ||
		!strings.Contains(msg, "Assisted-by: claude-cli (claude-sonnet-5) via Sinet") {
		t.Errorf("the accept commit must carry the deterministic trailers verbatim, got %q", msg)
	}

	// The deliverable is settled.
	d, err := f.rev.Deliverable(f.ctx, "dlv-local")
	if err != nil {
		t.Fatal(err)
	}
	if d.State != review.StateAccepted {
		t.Errorf("the deliverable must leave in-review, got %s", d.State)
	}

	// The landing is journaled as the class-A accept effect and SUCCEEDED
	// (brief R2: the git-store mutation is a side effect outside the DB
	// transaction — the shape S02.7's two-phase discipline exists for — and the
	// accepted read-back derives the commit from this row).
	if out.EffectID == "" {
		t.Errorf("the local landing must journal the accept effect (brief R2)")
	} else if eff, err := f.journal.Get(f.ctx, out.EffectID); err != nil {
		t.Errorf("read effect %s: %v", out.EffectID, err)
	} else if eff.State != gates.EffectSucceeded {
		t.Errorf("the landed effect must be succeeded, got %s", eff.State)
	}

	// The sibling-accept freshness trigger fires on this arm exactly as on the
	// push arm (the fixture's one stale active run routes).
	if len(out.RoutedRuns) == 0 {
		t.Errorf("the sibling-accept freshness trigger must fire for a project deliverable (S02.8)")
	}
}

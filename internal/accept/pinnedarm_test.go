package accept_test

import (
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// pinnedarm_test.go — P3-RW-17: the accept of a revision that pins CONTENT.
//
// The S13.6 ceremony below it is the outward arm for repo-targeted work; a
// revision with no snapshot commit has no candidate to push and nothing to push
// it onto, so its accept is what S13.1 defines — the owner's decision moving the
// deliverable to accepted against an immutable pin. These tests are adversarial
// in one direction: each one re-reads the world to prove that NOTHING outward
// happened, with the landed push-arm suite as the control that the other arm
// still does everything it did.

// mkPinned mints an in-review deliverable whose revision pins content only
// (snapshot_sha NULL — the deliberate, landed state internal/stage's content
// lane produces for a task with no repo).
func (f *fix) mkPinned(id, taskID, runID, projectID string) accept.Input {
	f.t.Helper()
	f.mkTaskRun(taskID, runID, "u1")
	if _, err := f.rev.EnsureDeliverable(f.ctx, review.EnsureInput{
		ID: id, Owner: "u1", TaskID: taskID, ProjectID: projectID, Type: "markdown",
	}); err != nil {
		f.t.Fatalf("EnsureDeliverable %s: %v", id, err)
	}
	if _, err := f.rev.MintRevision(f.ctx, review.MintInput{
		DeliverableID: id, N: 1, RunID: runID, AttemptRef: runID + "#round-1",
		Files: map[string]string{"deliverable.md": "# " + id + "\n"},
	}); err != nil {
		f.t.Fatalf("MintRevision %s: %v", id, err)
	}
	return accept.Input{
		DeliverableID: id, AcceptingUser: "u1", AcceptingUserName: "User One", ProjectID: projectID,
		Subject: "feat: ship the reviewed work", Engine: "claude-cli", Model: "opus-4-8",
		VendorNoreply: "noreply@anthropic.invalid",
	}
}

func (f *fix) countRows(query string) int {
	f.t.Helper()
	var n int
	if err := f.db.QueryRowContext(f.ctx, query).Scan(&n); err != nil {
		f.t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func TestPinnedAcceptRecordsTheDecisionAndJournalsNoEffect(t *testing.T) {
	f := newFix(t)
	in := f.mkPinned("dlv-pin", "t-pin", "r-pin", "")

	out, err := f.acc.Accept(f.ctx, in)
	if err != nil {
		t.Fatalf("the accept of a payload-pinned deliverable must not error: %v", err)
	}
	if !out.Accepted || out.Commit != "" || out.EffectID != "" {
		t.Fatalf("a pinned accept records a decision and makes no commit: %+v", out)
	}
	if len(out.RoutedRuns) != 0 {
		t.Errorf("a projectless accept has no sibling runs to route, got %v", out.RoutedRuns)
	}
	// Nothing outward: the S02.7 journal exists for outward effects and none
	// happened, so proposing one would journal an act that never left.
	if n := f.countRows(`SELECT COUNT(*) FROM effects`); n != 0 {
		t.Errorf("a pinned accept proposed %d effects — it must propose none", n)
	}
	d, err := f.rev.Deliverable(f.ctx, "dlv-pin")
	if err != nil {
		t.Fatal(err)
	}
	if d.State != review.StateAccepted {
		t.Errorf("durable state %q after the accept, want accepted", d.State)
	}
	if n := f.countRows(`SELECT COUNT(*) FROM run_events WHERE type = 'deliverable.accepted'`); n != 1 {
		t.Errorf("want exactly one owner-attributed deliverable.accepted row, got %d", n)
	}

	// The structural backstop is the state guard, unchanged: a second accept
	// cannot re-apply. (The transport answers a repeat with the resolved state
	// before it ever reaches here — this is what holds if it did.)
	if _, err := f.acc.Accept(f.ctx, in); err == nil {
		t.Error("a second accept of an accepted deliverable must be refused by the state guard")
	}
	if n := f.countRows(`SELECT COUNT(*) FROM run_events WHERE type = 'deliverable.accepted'`); n != 1 {
		t.Errorf("the refused repeat minted a second accepted row: %d", n)
	}
}

// TestPinnedAcceptInAProjectLeavesTheRepoAlone is the OQ-B shape: content pinned
// INSIDE a project that does have a repo. The decision is recorded and the
// project is not touched — no commit, no ref move — because materializing a
// content payload into the document store at accept is unbuilt (a named
// follow-up, not something this arm invents).
func TestPinnedAcceptInAProjectLeavesTheRepoAlone(t *testing.T) {
	f := newFix(t)
	f.prepare("candidate\n") // onboards `proj` from a seeded bare remote
	head, err := f.proj.RepoHead(f.ctx, "proj")
	if err != nil {
		t.Fatal(err)
	}
	remoteBefore := f.remoteMain()

	out, err := f.acc.Accept(f.ctx, f.mkPinned("dlv-doc", "t-doc", "r-doc", "proj"))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if !out.Accepted || out.Commit != "" || out.EffectID != "" {
		t.Fatalf("a pinned accept in a project still pushes nothing: %+v", out)
	}
	// The S02.8 sibling-accept freshness DOES fire here: the project's active
	// runs may have read this work, and that is a project-scoped fact.
	if len(out.RoutedRuns) != 1 || out.RoutedRuns[0] != f.activeID {
		t.Errorf("the project's active runs must be routed for re-validation, got %v", out.RoutedRuns)
	}
	if now, err := f.proj.RepoHead(f.ctx, "proj"); err != nil || now != head {
		t.Errorf("the project's default branch moved (%q → %q, err %v)", head, now, err)
	}
	if f.remoteMain() != remoteBefore {
		t.Error("the remote's protected ref moved for an accept that pushes nothing")
	}
	if n := f.countRows(`SELECT COUNT(*) FROM effects`); n != 0 {
		t.Errorf("a pinned accept in a project proposed %d effects", n)
	}
}

// TestPinnedAcceptSupersedesThePriorAcceptedDefinition — R6. Supersession by
// subject_ref is the review store's, and the pinned arm reaches it through the
// same verb the push arm ends on (S13.1: a new accepted definition supersedes
// the old; automation definitions ride the same machinery).
func TestPinnedAcceptSupersedesThePriorAcceptedDefinition(t *testing.T) {
	f := newFix(t)
	f.mkTaskRun("t-def", "r-def", "u1")
	for _, id := range []string{"dlv-def-v1", "dlv-def-v2"} {
		if _, err := f.rev.EnsureDeliverable(f.ctx, review.EnsureInput{
			ID: id, Owner: "u1", TaskID: "t-def", SubjectRef: "worker:tidy", Type: "worker-definition",
		}); err != nil {
			t.Fatalf("EnsureDeliverable %s: %v", id, err)
		}
		if _, err := f.rev.MintRevision(f.ctx, review.MintInput{
			DeliverableID: id, N: 1, RunID: "r-def", AttemptRef: "r-def#round-1",
			Files: map[string]string{"definition.md": "# " + id + "\n"},
		}); err != nil {
			t.Fatalf("MintRevision %s: %v", id, err)
		}
	}
	base := accept.Input{AcceptingUser: "u1", AcceptingUserName: "User One"}
	first := base
	first.DeliverableID = "dlv-def-v1"
	if _, err := f.acc.Accept(f.ctx, first); err != nil {
		t.Fatalf("accept v1: %v", err)
	}
	second := base
	second.DeliverableID = "dlv-def-v2"
	out, err := f.acc.Accept(f.ctx, second)
	if err != nil {
		t.Fatalf("accept v2: %v", err)
	}
	if len(out.Superseded) != 1 || out.Superseded[0] != "dlv-def-v1" {
		t.Fatalf("the newly accepted definition must supersede its prior version, got %v", out.Superseded)
	}
	d, err := f.rev.Deliverable(f.ctx, "dlv-def-v1")
	if err != nil {
		t.Fatal(err)
	}
	if d.State != review.StateSuperseded {
		t.Errorf("the prior version is %q, want superseded", d.State)
	}
}

// TestRepoBackedAcceptStillDemandsItsProject pins the other half of the input
// rule: a project is a PUSH-arm fact, so it stops being an unconditional input
// requirement and starts being one the push arm states for itself. A repo-backed
// revision with no project has a candidate and nowhere to put it.
func TestRepoBackedAcceptStillDemandsItsProject(t *testing.T) {
	f := newFix(t)
	in, _ := f.prepare("candidate\n")
	in.ProjectID = ""
	if _, err := f.acc.Accept(f.ctx, in); err == nil {
		t.Fatal("a repo-backed accept with no project must be refused")
	}
	if n := f.countRows(`SELECT COUNT(*) FROM effects`); n != 0 {
		t.Errorf("the refusal proposed %d effects", n)
	}
	d, err := f.rev.Deliverable(f.ctx, "dlv")
	if err != nil {
		t.Fatal(err)
	}
	if d.State != review.StateInReview {
		t.Errorf("state %q after a refused accept, want in-review", d.State)
	}
	// The non-tautological control: with its project, the same accept completes.
	in.ProjectID = "proj"
	out, err := f.acc.Accept(f.ctx, in)
	if err != nil {
		t.Fatalf("the repo-backed accept must still work: %v", err)
	}
	if !out.Accepted || out.Commit == "" || out.EffectID == "" {
		t.Fatalf("the push arm is byte-for-byte what it was: %+v", out)
	}
}

// TestPinnedAcceptNeedsADeliverableAndAUser keeps the input validation honest at
// the boundary now that the project limb moved.
func TestPinnedAcceptNeedsADeliverableAndAUser(t *testing.T) {
	f := newFix(t)
	for _, in := range []accept.Input{
		{AcceptingUser: "u1"},
		{DeliverableID: "dlv-pin"},
	} {
		if _, err := f.acc.Accept(f.ctx, in); err == nil {
			t.Errorf("accept(%+v) must be refused at the boundary", in)
		}
	}
}

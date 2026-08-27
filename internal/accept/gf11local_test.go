package accept_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// gf11local_test.go — the executor-owned properties of the local landing
// (P3-GF11 checklist item 6, brief R2/R7 + OQ1). The grounding suite proves the
// landing HAPPENS; these prove it behaves like the act it claims to be: the
// depth-1 merge queue still holds across two local landings, a branch that moved
// under a pinned approval is a reviewable merge card rather than a fault, a
// crash-window replay lands exactly once, and the effect result names the
// landing on the local arm ONLY — the push arm's recorded bytes are untouched.

// localOnlyEntry is the project the New-project door mints: a fresh init store
// with no clone source and no remote (internal/shell/project_seams.go).
func (f *fix) localOnlyEntry(projectID string) project.Entry {
	f.t.Helper()
	e, err := f.proj.Get(f.ctx, projectID)
	if err != nil {
		f.t.Fatalf("Get %s: %v", projectID, err)
	}
	if e.RemoteURL != "" {
		f.t.Fatalf("fixture: %s is not local-only (remote %q)", projectID, e.RemoteURL)
	}
	return e
}

// localNext mints a SECOND in-review deliverable in the already-onboarded
// local-only project. Its workspace is cut AFTER any prior landing, so its base
// is the store's current default HEAD — the same shape prepareNext gives the
// remoted fixture.
func (f *fix) localNext(n int, body string) accept.Input {
	f.t.Helper()
	ctx := f.ctx
	id := fmt.Sprintf("dlv-local-%d", n)
	ws, err := f.proj.EnsureWorkspace(ctx, "proj-local", fmt.Sprintf("pipe-local-%d", n))
	if err != nil {
		f.t.Fatalf("EnsureWorkspace: %v", err)
	}
	rel := fmt.Sprintf("note-%d.md", n)
	if err := os.WriteFile(filepath.Join(ws.Path, rel), []byte(body), 0o600); err != nil {
		f.t.Fatal(err)
	}
	candidate, err := f.proj.Snapshot(ctx, ws.Path)
	if err != nil {
		f.t.Fatalf("Snapshot: %v", err)
	}
	if err := f.proj.CreateRevisionRef(ctx, "proj-local", review.RevisionRef(id, 1), candidate); err != nil {
		f.t.Fatalf("CreateRevisionRef: %v", err)
	}
	task, runID := "t-local-"+strconv.Itoa(n), "r-local-"+strconv.Itoa(n)
	f.mkTaskRun(task, runID, "u1")
	if _, err := f.rev.EnsureDeliverable(ctx, review.EnsureInput{
		ID: id, Owner: "u1", TaskID: task, ProjectID: "proj-local", Type: "markdown",
	}); err != nil {
		f.t.Fatalf("EnsureDeliverable: %v", err)
	}
	if _, err := f.rev.MintRevision(ctx, review.MintInput{
		DeliverableID: id, N: 1, RunID: runID, AttemptRef: runID + "#round-1",
		Files: map[string]string{rel: body}, SnapshotSHA: candidate,
	}); err != nil {
		f.t.Fatalf("MintRevision: %v", err)
	}
	return accept.Input{
		DeliverableID: id, AcceptingUser: "u1", AcceptingUserName: "User One",
		ProjectID: "proj-local", Subject: "docs: a second note",
		Engine: "claude-cli", Model: "claude-sonnet-5",
		VendorNoreply: "anthropic@vendor.noreply.sinet.invalid",
	}
}

// crashedLocalEffect drives a class-A accept effect for the local-only project
// to `approved` with the given pinned expect-sha — the crash window: the act was
// authorized and the process died before anything landed, and the recovery pass
// returned the in-doubt row for replay.
func (f *fix) crashedLocalEffect(deliverableID, candidate, expectSHA string) string {
	f.t.Helper()
	payload, err := json.Marshal(map[string]any{
		"expect_sha": expectSHA, "candidate": candidate,
		"trailers": accept.RenderTrailers("claude-cli", "claude-sonnet-5", "anthropic@vendor.noreply.sinet.invalid"),
		"ref":      "refs/heads/main", "deliverable_id": deliverableID, "project_id": "proj-local",
		"author_name": "User One", "subject": "docs: recovered accept", "provenance": "Task: t-local", "git_profile": "",
	})
	if err != nil {
		f.t.Fatal(err)
	}
	eff, err := f.journal.Propose(f.ctx, gates.Proposal{UserID: "u1", Class: gates.ClassA, Payload: payload})
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.journal.Approve(f.ctx, eff.ID, "u1"); err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.journal.BeginExecute(f.ctx, eff.ID); err != nil {
		f.t.Fatal(err)
	}
	f.reconcileToApproved(eff.ID)
	return eff.ID
}

// reconcileToApproved runs the REAL recovery verb over the in-doubt row.
func (f *fix) reconcileToApproved(effectID string) {
	f.t.Helper()
	if _, err := f.journal.ReconcileInDoubt(f.ctx); err != nil {
		f.t.Fatal(err)
	}
	if got, _ := f.journal.Get(f.ctx, effectID); got.State != gates.EffectApproved {
		f.t.Fatalf("reconcile left the effect in %s, want approved", got.State)
	}
}

// commitsOnDefault counts the commits reachable from the store's default branch
// — the "exactly once" measure a double landing would break.
func (f *fix) commitsOnDefault(storePath string) int {
	f.t.Helper()
	n, err := strconv.Atoi(f.git("-C", storePath, "rev-list", "--count", "refs/heads/main"))
	if err != nil {
		f.t.Fatal(err)
	}
	return n
}

// TestGF11LocalLandingsStackSequentially is brief R7: the depth-1 merge queue is
// arm-agnostic. A second accept in the same local-only project validates onto
// the branch the FIRST landing advanced, and its commit is parented on it — the
// same stacking the remoted fixture proves through HEAD-plus-first, reached here
// through real HEAD because on this arm the advance IS the landing and there is
// no in-flight window to look ahead into.
func TestGF11LocalLandingsStackSequentially(t *testing.T) {
	f := newFix(t)
	first := f.prepareLocalOnly("the first note\n")
	entry := f.localOnlyEntry("proj-local")

	outA, err := f.acc.Accept(f.ctx, first)
	if err != nil {
		t.Fatalf("first local accept: %v", err)
	}
	if !outA.Accepted || outA.Card != nil {
		t.Fatalf("first local accept did not land: %+v", outA)
	}

	second := f.localNext(2, "the second note\n")
	outB, err := f.acc.Accept(f.ctx, second)
	if err != nil {
		t.Fatalf("second local accept: %v", err)
	}
	if !outB.Accepted || outB.Card != nil {
		t.Fatalf("the second local accept did not stack cleanly: %+v", outB)
	}
	if parent := f.git("-C", entry.StorePath, "rev-parse", outB.Commit+"^"); parent != outA.Commit {
		t.Errorf("the second landing is parented on %s, want the first landing %s", parent, outA.Commit)
	}
	if head := f.git("-C", entry.StorePath, "rev-parse", "refs/heads/main"); head != outB.Commit {
		t.Errorf("the default branch holds %s, want the second landing %s", head, outB.Commit)
	}
	if d, _ := f.rev.Deliverable(f.ctx, second.DeliverableID); d.State != review.StateAccepted {
		t.Errorf("the second deliverable is %s, want accepted", d.State)
	}
}

// TestGF11LocalReDriveLandsExactlyOnce is brief R2's crash-window closure on the
// local arm: the shared execute phase replays from the pinned payload alone, and
// because the squash is content-addressed and pinned to the effect's approval
// time, the replay reproduces the SAME commit and AdvanceDefaultBranch no-ops on
// it. Two replays of one approved effect leave one commit, not two.
func TestGF11LocalReDriveLandsExactlyOnce(t *testing.T) {
	f := newFix(t)
	in := f.prepareLocalOnly("a recovered note\n")
	entry := f.localOnlyEntry("proj-local")
	rev, err := f.rev.RevisionAt(f.ctx, in.DeliverableID, 1)
	if err != nil {
		t.Fatal(err)
	}
	base := f.git("-C", entry.StorePath, "rev-parse", "refs/heads/main")
	before := f.commitsOnDefault(entry.StorePath)

	effID := f.crashedLocalEffect(in.DeliverableID, rev.SnapshotSHA, base)
	outA, err := f.acc.ReDrive(f.ctx, effID)
	if err != nil {
		t.Fatalf("first re-drive: %v", err)
	}
	if !outA.Accepted || outA.Card != nil {
		t.Fatalf("the re-driven local landing did not complete: %+v", outA)
	}
	if head := f.git("-C", entry.StorePath, "rev-parse", "refs/heads/main"); head != outA.Commit {
		t.Fatalf("the re-drive did not advance the default branch: head %s, commit %s", head, outA.Commit)
	}
	if got := f.commitsOnDefault(entry.StorePath); got != before+1 {
		t.Fatalf("one landing must add exactly one commit: %d -> %d", before, got)
	}

	// The window re-opens: the process died again after the branch moved and
	// before the journal recorded success, and recovery hands the row back.
	f.exec(`UPDATE effects SET state = 'executing' WHERE effect_id = ?`, effID)
	f.reconcileToApproved(effID)

	outB, err := f.acc.ReDrive(f.ctx, effID)
	if err != nil {
		t.Fatalf("second re-drive: %v", err)
	}
	if outB.Commit != outA.Commit {
		t.Errorf("the replay produced a different commit: %s != %s — the squash is not deterministic", outB.Commit, outA.Commit)
	}
	if got := f.commitsOnDefault(entry.StorePath); got != before+1 {
		t.Errorf("the replay landed a SECOND commit: %d, want %d", got, before+1)
	}
	if eff, _ := f.journal.Get(f.ctx, effID); eff.State != gates.EffectSucceeded {
		t.Errorf("effect state %s after the replay, want succeeded", eff.State)
	}
	if d, _ := f.rev.Deliverable(f.ctx, in.DeliverableID); d.State != review.StateAccepted {
		t.Errorf("deliverable %s after the replay, want accepted", d.State)
	}
}

// TestGF11LocalAdvanceCollisionIsAMergeCard is brief R7's other half: a local
// landing whose pinned approval can no longer fast-forward the default branch —
// the branch moved under it, exactly what a stale CAS lease means on the push
// arm — surfaces as the S13.6 reviewable merge card with a FAILED effect and an
// unmoved branch. Never a silent overwrite, and never a raw fault on the wire.
func TestGF11LocalAdvanceCollisionIsAMergeCard(t *testing.T) {
	f := newFix(t)
	in := f.prepareLocalOnly("a contested note\n")
	entry := f.localOnlyEntry("proj-local")
	rev, err := f.rev.RevisionAt(f.ctx, in.DeliverableID, 1)
	if err != nil {
		t.Fatal(err)
	}
	base := f.git("-C", entry.StorePath, "rev-parse", "refs/heads/main")

	// A concurrent accept lands first, moving the branch off the pinned base.
	outA, err := f.acc.Accept(f.ctx, in)
	if err != nil {
		t.Fatalf("the first local accept: %v", err)
	}
	landed := f.commitsOnDefault(entry.StorePath)

	// The stale approval replays against the base it pinned. Its squash reproduces
	// a commit parented on that base, which the moved branch cannot fast-forward to.
	effID := f.crashedLocalEffect(in.DeliverableID, rev.SnapshotSHA, base)
	out, err := f.acc.ReDrive(f.ctx, effID)
	if err != nil {
		t.Fatalf("a moved default branch is a merge card, not an error: %v", err)
	}
	if out.Accepted || out.Card == nil {
		t.Fatalf("the collision did not route to a merge card: %+v", out)
	}
	if out.Card.Reason != "lease-rejected" {
		t.Errorf("merge-card reason %q, want %q", out.Card.Reason, "lease-rejected")
	}
	if eff, _ := f.journal.Get(f.ctx, effID); eff.State != gates.EffectFailed {
		t.Errorf("the refused landing's effect is %s, want failed", eff.State)
	}
	if head := f.git("-C", entry.StorePath, "rev-parse", "refs/heads/main"); head != outA.Commit {
		t.Errorf("the refused landing moved the branch: head %s, want %s", head, outA.Commit)
	}
	if got := f.commitsOnDefault(entry.StorePath); got != landed {
		t.Errorf("the refused landing added a commit: %d, want %d", got, landed)
	}
}

// TestGF11EffectResultNamesTheLandingOnlyLocally pins OQ1 and the regression
// wall it rides on: the local landing's succeeded effect carries the additive
// `landing` member so the record says which arm ran, and the PUSH arm's recorded
// result stays byte-identical to the landed one — the commit and nothing else.
// The payload SHAPE is identical on both arms by construction (the arm is chosen
// at execute from the registry, never from a payload member), which this asserts
// by decoding both payloads through the same key set.
func TestGF11EffectResultNamesTheLandingOnlyLocally(t *testing.T) {
	local := func() gates.Effect {
		f := newFix(t)
		out, err := f.acc.Accept(f.ctx, f.prepareLocalOnly("a local note\n"))
		if err != nil {
			t.Fatalf("local accept: %v", err)
		}
		eff, err := f.journal.Get(f.ctx, out.EffectID)
		if err != nil {
			t.Fatal(err)
		}
		if string(eff.Result) != fmt.Sprintf(`{"commit":%q,"landing":"local"}`, out.Commit) {
			t.Errorf("the local landing's effect result must name its landing: %s", eff.Result)
		}
		return eff
	}()

	remoted := func() gates.Effect {
		f := newFix(t)
		in, _ := f.prepare("a pushed note\n")
		out, err := f.acc.Accept(f.ctx, in)
		if err != nil {
			t.Fatalf("push-arm accept: %v", err)
		}
		eff, err := f.journal.Get(f.ctx, out.EffectID)
		if err != nil {
			t.Fatal(err)
		}
		// THE REGRESSION WALL: the push arm's recorded bytes are what they were
		// before GF11. A landing member here would be a silent contract change on
		// the arm this packet promised not to touch.
		if string(eff.Result) != fmt.Sprintf(`{"commit":%q}`, out.Commit) {
			t.Errorf("the push arm's effect result changed: %s", eff.Result)
		}
		return eff
	}()

	// One payload shape, both arms — the same keys, none of them naming an arm.
	keys := func(raw json.RawMessage) []string {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		return out
	}
	localKeys, remoteKeys := keys(local.Payload), keys(remoted.Payload)
	if len(localKeys) != len(remoteKeys) {
		t.Fatalf("the class-A accept payload shape differs by arm: %v vs %v", localKeys, remoteKeys)
	}
	var localMap, remoteMap map[string]any
	_ = json.Unmarshal(local.Payload, &localMap)
	_ = json.Unmarshal(remoted.Payload, &remoteMap)
	for _, k := range localKeys {
		if _, ok := remoteMap[k]; !ok {
			t.Errorf("payload key %q exists only on the local arm — the shape must be identical (GF10 §6)", k)
		}
	}
}

// TestGF11LocalArmNeverAsksTheBroker is brief R1's forbidden landing at the
// orchestration seam: a Pusher that fails the test on ANY call proves the local
// arm composes no request at all, rather than composing one something absorbs.
func TestGF11LocalArmNeverAsksTheBroker(t *testing.T) {
	f := newFix(t)
	in := f.prepareLocalOnly("an unpushed note\n")
	// Replace the wired pusher with one that cannot be called. accept.New copies
	// the config, so the Accepter is rebuilt around the refusing seam.
	refusing := &refusingPusher{t: t}
	acc, err := accept.New(accept.Config{
		Project: f.proj, Journal: f.journal, Push: refusing, Review: f.rev,
		Freshness: f.reg,
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := acc.Accept(f.ctx, in)
	if err != nil {
		t.Fatalf("the local landing must not need a pusher at all: %v", err)
	}
	if !out.Accepted || out.Commit == "" {
		t.Fatalf("the local landing did not complete: %+v", out)
	}
	if refusing.calls != 0 {
		t.Errorf("the local arm called the broker seam %d time(s); no push exists to request (brief R1)", refusing.calls)
	}
}

// refusingPusher fails the test if the accept ever asks it to push.
type refusingPusher struct {
	t     *testing.T
	calls int
}

func (p *refusingPusher) Push(req broker.Request) (broker.PushResult, error) {
	p.calls++
	p.t.Errorf("the accept composed a broker push request on a store with no remote: %+v", req)
	return broker.PushResult{}, errors.New("broker: push with no remote")
}

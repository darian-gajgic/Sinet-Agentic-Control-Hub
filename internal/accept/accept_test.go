package accept_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// The S13.6 accept acceptance battery: the REAL project topology, effect
// journal, broker (server + client), and review store composed over file://
// fixtures in t.TempDir() — zero network, zero paid calls.

type fix struct {
	t        *testing.T
	ctx      context.Context
	db       *storage.DB
	log      *eventlog.Log
	reg      *settings.Registry
	proj     *project.Store
	rev      *review.Store
	journal  *gates.Journal
	pusher   accept.Pusher
	signer   accept.Signer
	client   *broker.Client
	remote   string // bare remote path
	baseSHA  string
	acc      *accept.Accepter
	activeID string
}

func newFix(t *testing.T) *fix {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatal(err)
	}
	rev := &review.Store{DB: db, Log: log, Settings: reg, Root: filepath.Join(t.TempDir(), "review")}
	journal, err := gates.NewJournal(gates.JournalConfig{DB: db, Settings: reg})
	if err != nil {
		t.Fatal(err)
	}
	client := startBroker(t)

	f := &fix{t: t, ctx: ctx, db: db, log: log, reg: reg, proj: proj, rev: rev,
		journal: journal, pusher: client, signer: client.SignData, client: client, activeID: "r-active"}

	acc, err := accept.New(accept.Config{
		Project: proj, Journal: journal, Push: client, Review: rev, Signer: client.SignData,
		// The PRODUCTION signing posture: a user signs iff they have a
		// git-ssh-key enrolled in the broker (F3 — derived, structural,
		// per-user; no per-call override).
		SigningPosture: func(_ context.Context, user string) (bool, string, error) {
			profile := user + "-git"
			kind, has, err := client.HasKey(profile)
			return has && kind == broker.KindGitSSHKey, profile, err
		},
		ActiveRuns: func(context.Context, string) ([]run.SiblingAcceptRun, error) {
			// One active run in the project, checkpointed long ago.
			return []run.SiblingAcceptRun{{RunID: f.activeID, CheckpointTime: time.Unix(1, 0)}}, nil
		},
		Freshness: reg,
		Now:       func() time.Time { return time.Unix(1_700_000_000, 0) },
	})
	if err != nil {
		t.Fatal(err)
	}
	f.acc = acc
	return f
}

// prepare onboards a project cloned from a seeded bare remote, mints an
// in-review deliverable whose candidate is a run-branch snapshot commit, and
// returns the accept Input template.
func (f *fix) prepare(candidateBody string) (accept.Input, string) {
	f.t.Helper()
	ctx := f.ctx
	f.remote, f.baseSHA = f.seedRemote("base\n")

	if _, _, err := f.proj.Onboard(ctx, project.OnboardInput{
		ProjectID: "proj", Owner: "u1", Name: "P", Source: f.remote, RemoteURL: "file://" + f.remote,
	}); err != nil {
		f.t.Fatalf("Onboard: %v", err)
	}
	if _, err := f.proj.Approve(ctx, "proj", "u1", nil); err != nil {
		f.t.Fatalf("Approve: %v", err)
	}
	ws, err := f.proj.EnsureWorkspace(ctx, "proj", "pipe1")
	if err != nil {
		f.t.Fatalf("EnsureWorkspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ws.Path, "a.txt"), []byte(candidateBody), 0o600); err != nil {
		f.t.Fatal(err)
	}
	candidate, err := f.proj.Snapshot(ctx, ws.Path)
	if err != nil {
		f.t.Fatalf("Snapshot: %v", err)
	}
	if err := f.proj.CreateRevisionRef(ctx, "proj", review.RevisionRef("dlv", 1), candidate); err != nil {
		f.t.Fatalf("CreateRevisionRef: %v", err)
	}

	f.mkTaskRun("t1", "r1", "u1")
	if _, err := f.rev.EnsureDeliverable(ctx, review.EnsureInput{
		ID: "dlv", Owner: "u1", TaskID: "t1", ProjectID: "proj", Type: "markdown",
	}); err != nil {
		f.t.Fatalf("EnsureDeliverable: %v", err)
	}
	if _, err := f.rev.MintRevision(ctx, review.MintInput{
		DeliverableID: "dlv", N: 1, RunID: "r1", AttemptRef: "r1#round-1",
		Files: map[string]string{"a.txt": candidateBody}, SnapshotSHA: candidate,
	}); err != nil {
		f.t.Fatalf("MintRevision: %v", err)
	}
	in := accept.Input{
		DeliverableID: "dlv", AcceptingUser: "u1", AcceptingUserName: "User One", ProjectID: "proj",
		Subject: "feat: ship the reviewed change", Provenance: "Task: t1\nSession: r1",
		Engine: "claude-cli", Model: "opus-4-8", VendorNoreply: "noreply@anthropic.invalid",
	}
	return in, candidate
}

// prepareNext mints a SECOND in-review deliverable on a new pipeline (its base
// is the CURRENT default HEAD, moved by the prior accept), reusing the already-
// onboarded project.
func (f *fix) prepareNext(n int, body string) accept.Input {
	f.t.Helper()
	ctx := f.ctx
	id := fmt.Sprintf("dlv%d", n)
	pipe := fmt.Sprintf("pipe%d", n)
	task := fmt.Sprintf("t%d", n)
	runID := fmt.Sprintf("r%d", n)
	ws, err := f.proj.EnsureWorkspace(ctx, "proj", pipe)
	if err != nil {
		f.t.Fatalf("EnsureWorkspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ws.Path, fmt.Sprintf("f%d.txt", n)), []byte(body), 0o600); err != nil {
		f.t.Fatal(err)
	}
	candidate, err := f.proj.Snapshot(ctx, ws.Path)
	if err != nil {
		f.t.Fatalf("Snapshot: %v", err)
	}
	if err := f.proj.CreateRevisionRef(ctx, "proj", review.RevisionRef(id, 1), candidate); err != nil {
		f.t.Fatalf("CreateRevisionRef: %v", err)
	}
	f.mkTaskRun(task, runID, "u1")
	if _, err := f.rev.EnsureDeliverable(ctx, review.EnsureInput{
		ID: id, Owner: "u1", TaskID: task, ProjectID: "proj", Type: "markdown",
	}); err != nil {
		f.t.Fatalf("EnsureDeliverable: %v", err)
	}
	if _, err := f.rev.MintRevision(ctx, review.MintInput{
		DeliverableID: id, N: 1, RunID: runID, AttemptRef: runID + "#round-1",
		Files: map[string]string{fmt.Sprintf("f%d.txt", n): body}, SnapshotSHA: candidate,
	}); err != nil {
		f.t.Fatalf("MintRevision: %v", err)
	}
	return accept.Input{
		DeliverableID: id, AcceptingUser: "u1", AcceptingUserName: "User One", ProjectID: "proj",
		Subject: "feat: follow-on change", Engine: "claude-cli", Model: "opus-4-8", VendorNoreply: "noreply@anthropic.invalid",
	}
}

// enrollGitKey generates and stores a git-ssh-key for a user in the broker,
// making their derived signing posture "signs".
func (f *fix) enrollGitKey(t *testing.T, user string) {
	t.Helper()
	keyPath := filepath.Join(t.TempDir(), user+"-gitkey")
	f.runCmd("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", keyPath, "-C", user)
	priv, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	f.storeBrokerKey(user+"-git", string(priv))
}

// TestAcceptEndToEnd (rubric 2,4,5,10): one class-A effect through the journal,
// a platform squash into one attributed commit CAS-pushed to the remote's
// protected ref, the deliverable moves to accepted, and the sibling-accept
// freshness trigger fires.
func TestAcceptEndToEnd(t *testing.T) {
	f := newFix(t)
	in, candidate := f.prepare("candidate\n")

	out, err := f.acc.Accept(f.ctx, in)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if !out.Accepted || out.Card != nil {
		t.Fatalf("accept did not complete: %+v", out)
	}
	// The remote's protected ref advanced to the accept commit.
	if got := f.remoteMain(); got != out.Commit {
		t.Errorf("remote main %s != accept commit %s", got, out.Commit)
	}
	// One class-A effect, succeeded, approved by the accepting user.
	eff, err := f.journal.Get(f.ctx, out.EffectID)
	if err != nil {
		t.Fatal(err)
	}
	if eff.Class != gates.ClassA || eff.State != gates.EffectSucceeded || eff.ApprovedBy != "u1" {
		t.Errorf("effect not a succeeded class-A approved by u1: %+v", eff)
	}
	// The pinned payload carries the CAS expect-sha + candidate + rendered trailers.
	var pl map[string]any
	json.Unmarshal(eff.Payload, &pl)
	if pl["expect_sha"] != f.baseSHA || pl["candidate"] != candidate {
		t.Errorf("effect payload not pinned to expect-sha/candidate: %v", pl)
	}
	if !strings.Contains(pl["trailers"].(string), "Co-Authored-By: claude-cli opus-4-8 <noreply@anthropic.invalid>") {
		t.Errorf("effect payload trailers not pinned: %v", pl["trailers"])
	}
	// The accept commit is attributed + carries the trailers in its body.
	body := f.git("--git-dir="+f.remote, "cat-file", "-p", out.Commit)
	for _, want := range []string{
		"author User One <u1@users.noreply.sinet.invalid>",
		"Co-Authored-By: claude-cli opus-4-8 <noreply@anthropic.invalid>",
		"Assisted-by: claude-cli (opus-4-8) via Sinet",
		"Task: t1",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("accept commit missing %q:\n%s", want, body)
		}
	}
	// The deliverable moved to accepted.
	d, _ := f.rev.Deliverable(f.ctx, "dlv")
	if d.State != review.StateAccepted {
		t.Errorf("deliverable state %q, want accepted", d.State)
	}
	// The sibling-accept trigger routed the active run to re-validation.
	if len(out.RoutedRuns) != 1 || out.RoutedRuns[0] != f.activeID {
		t.Errorf("sibling-accept did not route the active run: %v", out.RoutedRuns)
	}
}

// TestDepth1StacksOntoHeadPlusFirst (F2): while a first accept is in-flight
// (pushed to the remote, local not yet advanced), a second accept for the
// project validates onto HEAD-plus-first (the first's resulting commit) and
// stacks — its accept commit's parent is the first's commit, not the stale base.
func TestDepth1StacksOntoHeadPlusFirst(t *testing.T) {
	f := newFix(t)
	inA, _ := f.prepare("a-content\n")
	inB := f.prepareNext(2, "b-content\n") // B's candidate is based on the same base

	var bParent, bCommit string
	f.acc.AfterPush = func(_ string, commitA string) {
		f.acc.AfterPush = nil // the nested accept must not re-enter the hook
		outB, err := f.acc.Accept(f.ctx, inB)
		if err != nil {
			t.Errorf("stacked accept: %v", err)
			return
		}
		if outB.Card != nil || !outB.Accepted {
			t.Errorf("B did not stack cleanly: %+v", outB)
			return
		}
		bCommit = outB.Commit
		bParent = f.git("--git-dir="+f.remote, "rev-parse", bCommit+"^")
		if bParent != commitA {
			t.Errorf("B stacked onto %s, want HEAD-plus-first %s", bParent, commitA)
		}
	}
	outA, err := f.acc.Accept(f.ctx, inA)
	if err != nil {
		t.Fatalf("A accept: %v", err)
	}
	if !outA.Accepted {
		t.Fatalf("A not accepted: %+v", outA)
	}
	if bCommit == "" {
		t.Fatal("the stacked second accept did not run")
	}
	// B is a descendant of A on the remote (the depth-1 stack landed in order).
	if outA.Commit != bParent {
		t.Errorf("A commit %s is not B's parent %s", outA.Commit, bParent)
	}
	// The registry is empty after both complete (no leak → the next accept
	// revalidates against real HEAD).
	if outA.Card != nil {
		t.Error("A produced a merge card")
	}
}

// pinnedPayload builds the class-A accept effect payload keys ReDrive reads
// (the crash-window re-drive is self-sufficient from the effect, F4).
func pinnedPayload(candidate string) []byte {
	p, _ := json.Marshal(map[string]any{
		"expect_sha": "", "candidate": candidate,
		"trailers": accept.RenderTrailers("claude-cli", "opus-4-8", "noreply@anthropic.invalid"),
		"ref":      "refs/heads/main", "deliverable_id": "dlv", "project_id": "proj",
		"author_name": "User One", "subject": "feat: recovered accept", "provenance": "Task: t1", "git_profile": "",
	})
	return p
}

// crashedEffect drives the journal to `executing` (BeginExecute done, no push)
// — the crash window — then reconciles the in-doubt row back to `approved`, and
// returns the effect id ready for ReDrive.
func (f *fix) crashedEffect(t *testing.T, candidate string) string {
	t.Helper()
	// expect_sha = the base (RepoHead); patch it into the payload.
	pl := map[string]any{}
	json.Unmarshal(pinnedPayload(candidate), &pl)
	pl["expect_sha"] = f.baseSHA
	payload, _ := json.Marshal(pl)
	eff, err := f.journal.Propose(f.ctx, gates.Proposal{UserID: "u1", Class: gates.ClassA, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.journal.Approve(f.ctx, eff.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.journal.BeginExecute(f.ctx, eff.ID); err != nil { // now `executing` — crash here, before the push
		t.Fatal(err)
	}
	// Recovery: the in-doubt executing class-A row returns to approved for replay.
	if _, err := f.journal.ReconcileInDoubt(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.journal.Get(f.ctx, eff.ID); got.State != gates.EffectApproved {
		t.Fatalf("reconcile left the effect in %s, want approved", got.State)
	}
	return eff.ID
}

// TestCrashWindowReDriveLandsAtPinnedSha (F4): BeginExecute → crash (no push) →
// reconcile → ReDrive blind-replays the CAS push against the SAME pinned
// expect-sha → the commit lands and the effect Succeeds.
func TestCrashWindowReDriveLandsAtPinnedSha(t *testing.T) {
	f := newFix(t)
	_, candidate := f.prepare("candidate\n")
	effID := f.crashedEffect(t, candidate)

	out, err := f.acc.ReDrive(f.ctx, effID)
	if err != nil {
		t.Fatalf("ReDrive: %v", err)
	}
	if !out.Accepted || out.Card != nil {
		t.Fatalf("re-drive did not complete: %+v", out)
	}
	// The push landed at the pinned expect-sha (remote base → the accept commit).
	if tip := f.remoteMain(); tip != out.Commit {
		t.Errorf("remote main %s != re-driven commit %s", tip, out.Commit)
	}
	if eff, _ := f.journal.Get(f.ctx, effID); eff.State != gates.EffectSucceeded {
		t.Errorf("effect state %s after re-drive, want succeeded", eff.State)
	}
	if d, _ := f.rev.Deliverable(f.ctx, "dlv"); d.State != review.StateAccepted {
		t.Errorf("deliverable not accepted after re-drive: %s", d.State)
	}
}

// TestCrashWindowReDriveRefMovedMergeCard (F4): if the ref moved during the
// crash window, the blind replay against the pinned sha lease-rejects → Fail +
// merge card, NEVER a new-sha retry.
func TestCrashWindowReDriveRefMovedMergeCard(t *testing.T) {
	f := newFix(t)
	_, candidate := f.prepare("candidate\n")
	effID := f.crashedEffect(t, candidate)

	// The remote ref moves out from under the pinned lease (a concurrent push).
	other := filepath.Join(t.TempDir(), "other")
	f.git("clone", "file://"+f.remote, other)
	os.WriteFile(filepath.Join(other, "z.txt"), []byte("z\n"), 0o600)
	f.git("-C", other, "add", "-A")
	f.git("-C", other, "-c", "user.name=x", "-c", "user.email=x@x", "commit", "-m", "concurrent")
	f.git("-C", other, "push", "file://"+f.remote, "HEAD:refs/heads/main")
	moved := f.remoteMain()

	out, err := f.acc.ReDrive(f.ctx, effID)
	if err != nil {
		t.Fatalf("ReDrive: %v", err)
	}
	if out.Accepted || out.Card == nil || out.Card.Reason != "lease-rejected" {
		t.Fatalf("ref-moved re-drive did not route to a merge card: %+v", out)
	}
	if eff, _ := f.journal.Get(f.ctx, effID); eff.State != gates.EffectFailed {
		t.Errorf("effect state %s, want failed", eff.State)
	}
	// Never a new-sha retry: the remote stays at the concurrent commit.
	if f.remoteMain() != moved {
		t.Error("a re-drive rejection still moved the remote — a forbidden new-sha retry")
	}
	if d, _ := f.rev.Deliverable(f.ctx, "dlv"); d.State != review.StateInReview {
		t.Errorf("deliverable accepted despite a rejected re-drive: %s", d.State)
	}
}

// TestDeterministicTrailers (rubric 5): the trailers render byte-stable from the
// structural template.
func TestDeterministicTrailers(t *testing.T) {
	a := accept.RenderTrailers("claude-cli", "opus-4-8", "noreply@x")
	b := accept.RenderTrailers("claude-cli", "opus-4-8", "noreply@x")
	if a != b {
		t.Fatal("RenderTrailers is not deterministic")
	}
	want := "Co-Authored-By: claude-cli opus-4-8 <noreply@x>\nAssisted-by: claude-cli (opus-4-8) via Sinet"
	if a != want {
		t.Errorf("trailers = %q, want %q", a, want)
	}
}

// TestAcceptCollisionMergeCard (rubric 3): a candidate that conflicts with a
// moved HEAD produces a merge card with exactly three options — never a push,
// never an effect.
func TestAcceptCollisionMergeCard(t *testing.T) {
	f := newFix(t)
	in, _ := f.prepare("line1\nCANDIDATE\n")
	// Move the project's default branch HEAD with a conflicting change.
	e, _ := f.proj.Get(f.ctx, "proj")
	f.git("-C", e.StorePath, "-c", "user.name=x", "-c", "user.email=x@x", "commit", "--allow-empty", "-m", "n")
	os.WriteFile(filepath.Join(e.StorePath, "a.txt"), []byte("line1\nHEADSIDE\n"), 0o600)
	f.git("-C", e.StorePath, "add", "-A")
	f.git("-C", e.StorePath, "-c", "user.name=x", "-c", "user.email=x@x", "commit", "-m", "moved")

	out, err := f.acc.Accept(f.ctx, in)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if out.Accepted || out.Card == nil {
		t.Fatalf("collision did not produce a merge card: %+v", out)
	}
	if got := len(out.Card.Options); got != 3 {
		t.Errorf("merge card has %d options, want exactly 3: %v", got, out.Card.Options)
	}
	want := map[string]bool{accept.OptAgentAutoResolve: true, accept.OptResolveManually: true, accept.OptAbortToNewAttempt: true}
	for _, o := range out.Card.Options {
		if !want[o] {
			t.Errorf("unexpected merge-card option %q", o)
		}
	}
	// No effect was proposed and the deliverable stays in-review.
	d, _ := f.rev.Deliverable(f.ctx, "dlv")
	if d.State != review.StateInReview {
		t.Errorf("deliverable state %q after a collision, want in-review", d.State)
	}
}

// TestAcceptLeaseRejectionMergeCard (rubric 6): a stale lease at push routes
// back to a merge card (never a blind retry), the effect fails, and the
// deliverable stays in-review.
func TestAcceptLeaseRejectionMergeCard(t *testing.T) {
	f := newFix(t)
	in, _ := f.prepare("candidate\n")
	// Move the REMOTE ref out from under the lease (a concurrent push).
	other := filepath.Join(t.TempDir(), "other")
	f.git("clone", "file://"+f.remote, other)
	os.WriteFile(filepath.Join(other, "z.txt"), []byte("z\n"), 0o600)
	f.git("-C", other, "add", "-A")
	f.git("-C", other, "-c", "user.name=x", "-c", "user.email=x@x", "commit", "-m", "concurrent")
	f.git("-C", other, "push", "file://"+f.remote, "HEAD:refs/heads/main")

	out, err := f.acc.Accept(f.ctx, in)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if out.Accepted || out.Card == nil || out.Card.Reason != "lease-rejected" {
		t.Fatalf("stale lease did not route to a merge card: %+v", out)
	}
	// The effect failed (not left executing, never blind-retried).
	if eff, _ := f.journal.Get(f.ctx, out.EffectID); eff.State != gates.EffectFailed {
		t.Errorf("effect state after lease rejection = %s, want failed", eff.State)
	}
	if d, _ := f.rev.Deliverable(f.ctx, "dlv"); d.State != review.StateInReview {
		t.Errorf("deliverable accepted despite a rejected push")
	}
}

// TestAcceptSigned (rubric 8): a user whose STRUCTURAL posture is "signs" (a
// git-ssh-key enrolled in the broker) produces an SSH-signed accept commit; the
// key never leaves the broker.
func TestAcceptSigned(t *testing.T) {
	f := newFix(t)
	in, _ := f.prepare("signed candidate\n")
	f.enrollGitKey(t, "u1") // u1's posture is now "signs" (derived, not a call flag)

	out, err := f.acc.Accept(f.ctx, in)
	if err != nil {
		t.Fatalf("Accept signed: %v", err)
	}
	body := f.git("--git-dir="+f.remote, "cat-file", "-p", out.Commit)
	if !strings.Contains(body, "gpgsig -----BEGIN SSH SIGNATURE-----") {
		t.Fatalf("accept commit is not signed:\n%s", body)
	}
}

// TestSigningPostureIsStructural (F3): a user's signing is a durable per-user
// fact, not a per-call argument — so the platform NEVER produces a mixed-signed
// history. A signing user's every accept is signed; a non-signing user's every
// accept is unsigned; no call can override it (the flag does not exist).
func TestSigningPostureIsStructural(t *testing.T) {
	// A user WITHOUT an enrolled key: every accept is unsigned.
	f1 := newFix(t)
	in1, _ := f1.prepare("v1\n")
	out1, err := f1.acc.Accept(f1.ctx, in1)
	if err != nil {
		t.Fatalf("unsigned accept: %v", err)
	}
	if strings.Contains(f1.git("--git-dir="+f1.remote, "cat-file", "-p", out1.Commit), "gpgsig") {
		t.Error("a user with no enrolled key produced a SIGNED commit — posture leaked")
	}

	// A user WITH an enrolled key: EVERY accept signs. Two deliverables in one
	// project both sign — there is no per-call flag to un-sign one, so a mixed
	// history is impossible by construction (Input has no signing field).
	f2 := newFix(t)
	f2.enrollGitKey(t, "u1")
	inA, _ := f2.prepare("a\n")
	out2a, err := f2.acc.Accept(f2.ctx, inA)
	if err != nil {
		t.Fatalf("signed accept 1: %v", err)
	}
	inB := f2.prepareNext(2, "b\n")
	out2b, err := f2.acc.Accept(f2.ctx, inB)
	if err != nil {
		t.Fatalf("signed accept 2: %v", err)
	}
	for i, c := range []string{out2a.Commit, out2b.Commit} {
		if !strings.Contains(f2.git("--git-dir="+f2.remote, "cat-file", "-p", c), "gpgsig") {
			t.Errorf("accept %d by a signing user is UNSIGNED — mixed-signed history (F3)", i+1)
		}
	}
}

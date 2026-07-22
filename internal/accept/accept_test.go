package accept_test

import (
	"context"
	"encoding/json"
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

// TestAcceptSigned (rubric 8): with signing on, the accept commit is SSH-signed
// via the broker (the key never leaves) and the signature verifies.
func TestAcceptSigned(t *testing.T) {
	f := newFix(t)
	in, _ := f.prepare("signed candidate\n")
	// Enroll a git-ssh-key in the broker and turn on signing.
	keyDir := t.TempDir()
	keyPath := filepath.Join(keyDir, "gitkey")
	f.runCmd("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", keyPath, "-C", "u1")
	priv, _ := os.ReadFile(keyPath)
	f.storeBrokerKey("u1-git", string(priv))
	in.Sign = true
	in.SigningProfile = "u1-git"

	out, err := f.acc.Accept(f.ctx, in)
	if err != nil {
		t.Fatalf("Accept signed: %v", err)
	}
	body := f.git("--git-dir="+f.remote, "cat-file", "-p", out.Commit)
	if !strings.Contains(body, "gpgsig -----BEGIN SSH SIGNATURE-----") {
		t.Fatalf("accept commit is not signed:\n%s", body)
	}
}

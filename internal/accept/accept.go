// Package accept is the Spec S13.6 broker-mediated accept orchestration — one
// High-tier action that turns a reviewed deliverable revision into an
// attributed commit on the project's protected branch, exactly once. It
// composes ABOVE the storage-only packages (CONVENTIONS §27), importing
// internal/project (applies-cleanly / squash / advance / protected refs),
// internal/gates (the two-phase effect journal), internal/broker (the CAS push
// + SSH signing client), internal/review (the accepted state verb), and
// internal/run (the sibling-accept freshness producer).
//
// The flow (Spec S13.6): applies-cleanly depth-1 merge gate → a reviewable
// merge card on any collision (never a silent overwrite) → deterministic
// attribution trailers rendered from a structural template and pinned in the
// effect payload at approval → the platform squash into ONE attributed commit
// → the class-A effect through the journal (Propose/Approve/BeginExecute BEFORE
// the broker CAS push/Succeed|Fail) → on success the review state moves to
// accepted and the S02.8 sibling-accept freshness trigger fires to active
// project runs. A stale-lease rejection is a NORMAL collision routed back to a
// merge card, NEVER a blind retry against a new sha.
package accept

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
)

// The attribution-trailer TEMPLATE is a structural string constant (§8 reading
// 2): S18's registry ratifies no trailer key and its tally is frozen at 118, so
// the trailer text ships as code constants (the sseBatchSize/auth-constant
// precedent, flagged to the B4 gate under the settings-tab directive), NOT an
// invented ⚙ key. The RENDERED trailers are pinned in the effect payload at
// approval (deterministic, audit-trailed, displayed pre-accept). The
// Co-Authored-By line is what GitHub renders; the Assisted-by line is the
// machine-parseable provenance (the VS Code mis-attribution lesson, S13.6
// step 3).
const (
	coAuthoredByTemplate = "Co-Authored-By: %s %s <%s>"
	assistedByTemplate   = "Assisted-by: %s (%s) via Sinet"
)

// RenderTrailers renders the deterministic attribution trailers for an accept
// (Spec S13.6 step 3). Pure and byte-stable — never per-run improvisation.
func RenderTrailers(engine, model, vendorNoreply string) string {
	return fmt.Sprintf(coAuthoredByTemplate, engine, model, vendorNoreply) + "\n" +
		fmt.Sprintf(assistedByTemplate, engine, model)
}

// The three merge-card options (Spec S13.6 step 1 / S1.11) — exactly these,
// never a silent overwrite.
const (
	OptAgentAutoResolve  = "agent-auto-resolve"   // a bounded rework (the S07 seam)
	OptResolveManually   = "resolve-manually"     // the human resolves
	OptAbortToNewAttempt = "abort-to-new-attempt" // a fresh attempt off the moved HEAD
)

// MergeCard is the reviewable collision surface (Spec S13.6 step 1): a
// candidate that does not apply cleanly onto current HEAD, or whose CAS lease
// went stale at push, surfaces here with exactly three options — never a silent
// overwrite.
type MergeCard struct {
	DeliverableID string   `json:"deliverable_id"`
	ProjectID     string   `json:"project_id"`
	Onto          string   `json:"onto"`
	Candidate     string   `json:"candidate"`
	Reason        string   `json:"reason"` // "applies-clean-collision" | "lease-rejected"
	Conflicts     string   `json:"conflicts,omitempty"`
	Options       []string `json:"options"`
}

func mergeCard(deliverableID, projectID, onto, candidate, reason, conflicts string) *MergeCard {
	return &MergeCard{
		DeliverableID: deliverableID, ProjectID: projectID, Onto: onto, Candidate: candidate,
		Reason: reason, Conflicts: conflicts,
		Options: []string{OptAgentAutoResolve, OptResolveManually, OptAbortToNewAttempt},
	}
}

// Pusher is the broker CAS-push seam (*broker.Client satisfies it).
type Pusher interface {
	Push(req broker.Request) (broker.PushResult, error)
}

// Signer produces an SSHSIG over data with a broker-held git-ssh-key
// (broker.Client.SignData satisfies it via a thin adapter). nil when the
// accepting user does not sign.
type Signer func(profile, namespace string, data []byte) ([]byte, error)

// ActiveRuns resolves a project's active (non-terminal) runs for the
// sibling-accept producer (Spec S02.8). Wired by the composition root over the
// intake resolution + run store (R16); nil short-circuits (no active runs).
type ActiveRuns func(ctx context.Context, projectID string) ([]run.SiblingAcceptRun, error)

// Config wires an Accepter.
type Config struct {
	Project    *project.Store
	Journal    *gates.Journal
	Push       Pusher
	Review     *review.Store
	Signer     Signer
	ActiveRuns ActiveRuns
	// Freshness reads ⚙ freshness.max_age for the sibling-accept producer.
	Freshness run.FreshnessSettings
	Now       func() time.Time
}

// Accepter runs the S13.6 accept.
type Accepter struct {
	cfg Config
}

// New builds an Accepter.
func New(cfg Config) (*Accepter, error) {
	if cfg.Project == nil || cfg.Journal == nil || cfg.Push == nil || cfg.Review == nil {
		return nil, errors.New("accept: config needs Project, Journal, Push and Review")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Accepter{cfg: cfg}, nil
}

// Input is one accept action (Spec S13.6). High tier — the S01.9 PIN step-up is
// enforced at the API/answer layer (the §9 approvals-family call); this
// composes the journal transitions and NAMES the step-up call site (seam §3):
// the Approve below is the gated act.
type Input struct {
	DeliverableID     string
	AcceptingUser     string
	AcceptingUserName string
	ProjectID         string
	// Subject is the Conventional-Commit subject; Provenance the task/session
	// link carried in the body (Spec S13.6 step 2).
	Subject    string
	Provenance string
	// Engine/Model/VendorNoreply render the attribution trailers (step 3).
	Engine, Model, VendorNoreply string
	// Sign + SigningProfile: when the accepting user signs (all-or-nothing per
	// user, S13.6 step 5), the squash commit is SSH-signed via the broker.
	Sign           bool
	SigningProfile string
	// GitProfile is the broker git-ssh-key profile for the push transport
	// (empty in dev = file://; the live ssh leg is pure-config, R24).
	GitProfile string
}

// Outcome is the accept result: either an accepted commit, or a merge card the
// human resolves (never a silent overwrite).
type Outcome struct {
	Accepted   bool
	Commit     string
	EffectID   string
	Superseded []string
	RoutedRuns []string // active project runs routed to S02.6 re-validation
	Card       *MergeCard
}

// Accept runs the S13.6 broker-mediated accept.
func (a *Accepter) Accept(ctx context.Context, in Input) (Outcome, error) {
	if in.DeliverableID == "" || in.AcceptingUser == "" || in.ProjectID == "" {
		return Outcome{}, fmt.Errorf("accept: input needs deliverable, accepting user and project")
	}
	// Resolve the candidate: the current revision's snapshot commit (repo-backed).
	d, err := a.cfg.Review.Deliverable(ctx, in.DeliverableID)
	if err != nil {
		return Outcome{}, err
	}
	if d.State != review.StateInReview {
		return Outcome{}, fmt.Errorf("accept: deliverable %q is %s, not in-review", in.DeliverableID, d.State)
	}
	rev, err := a.cfg.Review.RevisionAt(ctx, in.DeliverableID, d.CurrentRevision)
	if err != nil {
		return Outcome{}, err
	}
	if rev.SnapshotSHA == "" {
		return Outcome{}, fmt.Errorf("accept: deliverable %q revision %d is not repo-backed (no snapshot commit to push)", in.DeliverableID, rev.N)
	}
	candidate := rev.SnapshotSHA

	e, err := a.cfg.Project.Get(ctx, in.ProjectID)
	if err != nil {
		return Outcome{}, err
	}
	onto, err := a.cfg.Project.RepoHead(ctx, in.ProjectID)
	if err != nil {
		return Outcome{}, err
	}
	if onto == "" {
		return Outcome{}, fmt.Errorf("accept: project %q has no default-branch HEAD", in.ProjectID)
	}

	// 1. Applies-cleanly gate (depth-1 merge queue). A collision → merge card.
	mr, err := a.cfg.Project.AppliesClean(ctx, in.ProjectID, onto, candidate)
	if err != nil {
		return Outcome{}, err
	}
	if !mr.Clean {
		return Outcome{Card: mergeCard(in.DeliverableID, in.ProjectID, onto, candidate, "applies-clean-collision", mr.Conflicts)}, nil
	}

	// 2. Deterministic trailers (structural template; rendered + pinned).
	trailers := RenderTrailers(in.Engine, in.Model, in.VendorNoreply)
	protectedRef := "refs/heads/" + e.DefaultBranch

	// 3. Class-A effect: Propose (payload = CAS expect-sha + candidate pin +
	// rendered trailers) → Approve (the gated High-tier act) → BeginExecute
	// (BEFORE the push).
	payload, err := json.Marshal(map[string]any{
		"expect_sha": onto, "candidate": candidate, "trailers": trailers,
		"ref": protectedRef, "deliverable_id": in.DeliverableID,
	})
	if err != nil {
		return Outcome{}, err
	}
	eff, err := a.cfg.Journal.Propose(ctx, gates.Proposal{UserID: in.AcceptingUser, Class: gates.ClassA, Payload: payload})
	if err != nil {
		return Outcome{}, err
	}
	if _, err := a.cfg.Journal.Approve(ctx, eff.ID, in.AcceptingUser); err != nil {
		return Outcome{}, err
	}
	if _, err := a.cfg.Journal.BeginExecute(ctx, eff.ID); err != nil {
		return Outcome{}, err
	}

	// 4. Platform squash → one attributed commit (the pinned trailers in the body).
	var sign func([]byte) ([]byte, error)
	if in.Sign {
		if a.cfg.Signer == nil {
			return Outcome{}, fmt.Errorf("accept: signing requested but no broker signer wired")
		}
		sign = func(p []byte) ([]byte, error) { return a.cfg.Signer(in.SigningProfile, "git", p) }
	}
	commit, err := a.cfg.Project.SquashAccept(ctx, in.ProjectID, project.SquashInput{
		Tree: mr.Tree, Parent: onto, UserID: in.AcceptingUser, AuthorName: in.AcceptingUserName,
		Message: buildMessage(in.Subject, in.Provenance, trailers), When: a.cfg.Now(), Sign: sign,
	})
	if err != nil {
		a.failEffect(ctx, eff.ID, "squash failed")
		return Outcome{}, err
	}

	// 5. Broker CAS push (--force-with-lease=<ref>:<expect> + protected-ref
	// authorization). A stale-lease REJECTION routes back to a merge card
	// (never a blind retry, S13.6 step 4); the crash-window in-doubt row would
	// replay blind against the SAME pinned expect-sha (class-A, ReconcileInDoubt).
	res, err := a.cfg.Push.Push(broker.Request{
		RepoDir: e.StorePath, Remote: e.RemoteURL,
		Refs:       []broker.RefUpdate{{Ref: protectedRef, ExpectSHA: onto, SrcSHA: commit}},
		Protected:  e.ProtectedRefs,
		Authorized: true,
		Profile:    in.GitProfile,
	})
	if err != nil {
		a.failEffect(ctx, eff.ID, "push failed")
		return Outcome{}, fmt.Errorf("accept: broker push: %w", err)
	}
	if res.Rejected {
		a.failEffect(ctx, eff.ID, "lease rejected")
		return Outcome{
			EffectID: eff.ID,
			Card:     mergeCard(in.DeliverableID, in.ProjectID, onto, candidate, "lease-rejected", "the ref moved since approval"),
		}, nil
	}

	// 6. Succeed, then commit the durable outcome: accepted state + advance the
	// local default branch + fire the sibling-accept freshness trigger.
	if _, err := a.cfg.Journal.Succeed(ctx, eff.ID, mustJSON(map[string]string{"commit": commit})); err != nil {
		return Outcome{}, err
	}
	acc, err := a.cfg.Review.Accept(ctx, in.DeliverableID, in.AcceptingUser)
	if err != nil {
		return Outcome{}, err
	}
	if err := a.cfg.Project.AdvanceDefaultBranch(ctx, in.ProjectID, commit, onto); err != nil {
		return Outcome{}, err
	}
	routed, err := a.fireSibling(ctx, in.ProjectID)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Accepted: true, Commit: commit, EffectID: eff.ID, Superseded: acc.Superseded, RoutedRuns: routed}, nil
}

// fireSibling fires the S02.8 sibling-accept freshness trigger to the project's
// active runs (R16): the producer feeds run.EvaluateFreshness's SiblingAccept
// consumer for each, and the routed (non-fresh) runs go to S02.6 re-validation.
func (a *Accepter) fireSibling(ctx context.Context, projectID string) ([]string, error) {
	if a.cfg.ActiveRuns == nil || a.cfg.Freshness == nil {
		return nil, nil
	}
	runs, err := a.cfg.ActiveRuns(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("accept: resolve active runs: %w", err)
	}
	return run.FireSiblingAccept(a.cfg.Freshness, runs, a.cfg.Now())
}

// failEffect fails the effect journal row on an execution error (best-effort —
// the caller returns the underlying error).
func (a *Accepter) failEffect(ctx context.Context, effectID, reason string) {
	_, _ = a.cfg.Journal.Fail(ctx, effectID, mustJSON(map[string]string{"error": reason}))
}

// buildMessage assembles the Conventional-Commit message: subject, the
// task/session provenance body, and the deterministic attribution trailers.
func buildMessage(subject, provenance, trailers string) string {
	if subject == "" {
		subject = "chore: accept reviewed deliverable"
	}
	msg := subject + "\n"
	if provenance != "" {
		msg += "\n" + provenance + "\n"
	}
	return msg + "\n" + trailers + "\n"
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

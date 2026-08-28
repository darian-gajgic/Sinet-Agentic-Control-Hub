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
	"sync"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
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

// ErrPushFailed names a push the broker could not perform — its own refusal or
// a transport failure — so the answer layer maps it ON THE ERROR'S TYPE and
// never by matching an error string (CONVENTIONS §38). It wraps the broker's
// own cause, which belongs in the ops log and never on the wire: an internal
// error chain served as a response detail is the crash-shaped answer this
// sentinel exists to replace (P3-GF11 R5).
//
// A stale-lease rejection is NOT this error. The broker reports it as a normal
// rejection and S13.6 answers it with a merge card, which is a decision the
// human makes rather than a failure anybody has to be told about.
var ErrPushFailed = errors.New("accept: the broker could not perform the push")

// landingLocal is the additive member the succeeded accept effect's result
// carries when the landing was local (P3-GF11 R2/OQ1). The PUSH arm's result
// stays byte-identical to the landed one — the commit and nothing else — so
// only the local landing names itself.
const landingLocal = "local"

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

// SigningPosture reports whether a user's accept commits are SSH-signed and,
// if so, the broker git-ssh-key profile to sign with — a STRUCTURAL per-user
// fact (Spec S13.6 step 5: all-or-nothing per user; the platform NEVER produces
// a mixed-signed history, F3). The production impl derives it from the broker
// git-ssh-key presence (the operator signs from day one; members opt in at
// enrollment). There is NO per-call override — signing is never an accept-call
// argument — so one user cannot produce a mixed signed/unsigned history. nil
// posture = unsigned (dev).
type SigningPosture func(ctx context.Context, userID string) (sign bool, profile string, err error)

// Config wires an Accepter.
type Config struct {
	Project        *project.Store
	Journal        *gates.Journal
	Push           Pusher
	Review         *review.Store
	Signer         Signer
	SigningPosture SigningPosture
	ActiveRuns     ActiveRuns
	// Freshness reads ⚙ freshness.max_age for the sibling-accept producer.
	Freshness run.FreshnessSettings
	Now       func() time.Time
}

// inflight is a project's in-flight accept — pushed to the remote's protected
// ref, local default branch not yet advanced. The next candidate for the
// project validates onto its commit (HEAD-plus-first, the S13.6 depth-1 merge
// queue).
type inflight struct {
	effectID string
	commit   string
}

// Accepter runs the S13.6 accept.
type Accepter struct {
	cfg Config
	mu  sync.Mutex
	// inflightByProject is the depth-1 merge-queue look-ahead: at most one
	// in-flight accept per project (S13.6 step 1). It is control-plane
	// coordination state (transient by nature — a crash aborts in-flight
	// accepts and the effect journal's ReconcileInDoubt resolves the rows).
	inflightByProject map[string]inflight
	// AfterPush, when set, is invoked after a successful push while this accept
	// is in-flight (its commit on the remote, local not yet advanced) — an
	// observability/coordination seam a test uses to drive a SECOND accept that
	// stacks onto HEAD-plus-first (F2). nil in production.
	AfterPush func(projectID, commit string)
}

// New builds an Accepter.
func New(cfg Config) (*Accepter, error) {
	if cfg.Project == nil || cfg.Journal == nil || cfg.Push == nil || cfg.Review == nil {
		return nil, errors.New("accept: config needs Project, Journal, Push and Review")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Accepter{cfg: cfg, inflightByProject: map[string]inflight{}}, nil
}

// projectOnto is the applies-cleanly base for a project (Spec S13.6 step 1): an
// in-flight accept's resulting commit (HEAD-plus-first, the depth-1 queue) when
// one is pending, else current default-branch HEAD. A second in-flight accept
// therefore validates against HEAD-plus-first; once the first terminates it
// revalidates against real HEAD.
func (a *Accepter) projectOnto(ctx context.Context, projectID string) (string, error) {
	a.mu.Lock()
	inf, ok := a.inflightByProject[projectID]
	a.mu.Unlock()
	if ok {
		return inf.commit, nil
	}
	return a.cfg.Project.RepoHead(ctx, projectID)
}

func (a *Accepter) markInflight(projectID, effectID, commit string) {
	a.mu.Lock()
	a.inflightByProject[projectID] = inflight{effectID: effectID, commit: commit}
	a.mu.Unlock()
}

func (a *Accepter) clearInflight(projectID, effectID string) {
	a.mu.Lock()
	if inf, ok := a.inflightByProject[projectID]; ok && inf.effectID == effectID {
		delete(a.inflightByProject, projectID)
	}
	a.mu.Unlock()
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
	// GitProfile is the broker git-ssh-key profile for the push transport
	// (empty in dev = file://; the live ssh leg is pure-config, R24). Signing
	// is NOT an accept-call argument — it is the accepting user's structural
	// posture (SigningPosture, F3), so no call can produce a mixed history.
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

// Accept runs the S13.6 accept, on the arm the revision's own pin selects.
//
// TWO ARMS, ONE ACT (P3-RW-17). The accept decision is defined for EVERY
// deliverable — 5.8 "the requester accepts or rejects; nothing self-approves
// into the world", D10, S07.8, and the S13.1 state machine — while the S13.6
// commit-push ceremony below is by its own text the outward arm for
// repo-targeted work: every step of it is defined over current project HEAD, a
// run branch and a protected ref, and its High tier is "(risk tier High —
// outward push)". A revision that pins CONTENT rather than a snapshot commit
// (S13.1: "binary types pin content-addressed object-dir hashes") has no
// candidate to push and nothing to push it onto, so its accept is what S13.1
// defines: the owner's decision moving the deliverable to accepted, recorded
// durably against the revision's immutable content pin and audit-trailed in the
// event log. The corpus already contains that class — an automation definition
// approved as a deliverable pushes nowhere, and `review.Store.Accept`'s
// supersession logic exists for exactly it.
func (a *Accepter) Accept(ctx context.Context, in Input) (Outcome, error) {
	if in.DeliverableID == "" || in.AcceptingUser == "" {
		return Outcome{}, fmt.Errorf("accept: input needs a deliverable and an accepting user")
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
		return a.acceptPinned(ctx, in)
	}
	// From here the PUSH arm, unchanged: a project is a push-arm fact, and only
	// this arm needs one (there is no protected ref without a project).
	if in.ProjectID == "" {
		return Outcome{}, fmt.Errorf("accept: repo-backed deliverable %q needs a project to push to", in.DeliverableID)
	}
	// S13.6 step 3 is an invariant of the ACT, so the act's owner enforces it.
	// The trailers rendered below go into a permanent commit; rendering the
	// templates around an empty engine, model or vendor address would push a
	// co-author line naming nobody, which is the mis-attribution failure step 3
	// exists to prevent. Refused HERE — before Propose, so no effect row records
	// an authorization for an act that must not happen, and before the squash,
	// so no commit object is ever made. The API's card-level guard stays where
	// it is; a wall only the caller upholds is not a wall.
	if in.Engine == "" || in.Model == "" || in.VendorNoreply == "" {
		return Outcome{}, fmt.Errorf("accept: deliverable %q has no renderable attribution (engine %q, model %q, vendor address %q): "+
			"an accept never signs off work in the name of nobody",
			in.DeliverableID, in.Engine, in.Model, in.VendorNoreply)
	}
	candidate := rev.SnapshotSHA

	e, err := a.cfg.Project.Get(ctx, in.ProjectID)
	if err != nil {
		return Outcome{}, err
	}
	// onto is the depth-1 merge-queue base (S13.6 step 1): HEAD-plus-first when
	// an accept for this project is already in-flight, else current HEAD.
	onto, err := a.projectOnto(ctx, in.ProjectID)
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
	// rendered trailers + the self-sufficient re-drive fields) → Approve (the
	// gated High-tier act). The payload pins EVERYTHING the crash-window
	// re-drive needs to reproduce the push deterministically (F4).
	payload, err := json.Marshal(acceptPayload{
		ExpectSHA: onto, Candidate: candidate, Trailers: trailers, Ref: protectedRef,
		DeliverableID: in.DeliverableID, ProjectID: in.ProjectID, AuthorName: in.AcceptingUserName,
		Subject: in.Subject, Provenance: in.Provenance, GitProfile: in.GitProfile,
	})
	if err != nil {
		return Outcome{}, err
	}
	eff, err := a.cfg.Journal.Propose(ctx, gates.Proposal{UserID: in.AcceptingUser, Class: gates.ClassA, Payload: payload})
	if err != nil {
		return Outcome{}, err
	}
	approved, err := a.cfg.Journal.Approve(ctx, eff.ID, in.AcceptingUser)
	if err != nil {
		return Outcome{}, err
	}
	// The squash timestamp is pinned to the effect's approval time so a
	// crash-window re-drive reproduces the SAME commit deterministically (F4).
	return a.executeAccept(ctx, executeSpec{
		effectID: eff.ID, projectID: in.ProjectID, deliverableID: in.DeliverableID,
		acceptingUser: in.AcceptingUser, authorName: in.AcceptingUserName,
		onto: onto, candidate: candidate, trailers: trailers, protectedRef: protectedRef,
		subject: in.Subject, provenance: in.Provenance, gitProfile: in.GitProfile,
		when: approved.ApprovedTS,
	})
}

// acceptPinned is the payload-pinned arm (P3-RW-17): the owner's decision on a
// revision that pins content rather than a snapshot commit.
//
// It is the SAME state verb the push arm ends on — `review.Store.Accept`, which
// carries the S13.1 supersession, the owner-attributed `deliverable.accepted`
// row and the idempotent state guard — and nothing else. No effect is proposed,
// because the S02.7 journal exists for OUTWARD effects and nothing here leaves
// the platform: a DB state flip and its event row are the record of a decision,
// not a class-A act. No project verb is called, no broker request is built, and
// there is no commit to return.
//
// The sibling-accept freshness trigger (S02.8) fires only for a deliverable that
// HAS a project: it routes that project's active runs to re-validation, and a
// projectless accept has no siblings to route.
func (a *Accepter) acceptPinned(ctx context.Context, in Input) (Outcome, error) {
	acc, err := a.cfg.Review.Accept(ctx, in.DeliverableID, in.AcceptingUser)
	if err != nil {
		return Outcome{}, err
	}
	var routed []string
	if in.ProjectID != "" {
		if routed, err = a.fireSibling(ctx, in.ProjectID); err != nil {
			return Outcome{}, err
		}
	}
	return Outcome{Accepted: true, Superseded: acc.Superseded, RoutedRuns: routed}, nil
}

// acceptPayload is the class-A accept effect's pinned payload — self-sufficient
// so the crash-window re-drive reproduces the push from the effect alone (F4).
type acceptPayload struct {
	ExpectSHA     string `json:"expect_sha"`
	Candidate     string `json:"candidate"`
	Trailers      string `json:"trailers"`
	Ref           string `json:"ref"`
	DeliverableID string `json:"deliverable_id"`
	ProjectID     string `json:"project_id"`
	AuthorName    string `json:"author_name"`
	Subject       string `json:"subject"`
	Provenance    string `json:"provenance"`
	GitProfile    string `json:"git_profile"`
}

// executeSpec is the execute phase's inputs — from Input on a fresh accept, or
// reconstructed from the pinned payload on a crash-window re-drive.
type executeSpec struct {
	effectID, projectID, deliverableID, acceptingUser, authorName string
	onto, candidate, trailers, protectedRef, subject, provenance  string
	gitProfile                                                    string
	when                                                          time.Time
}

// ReDrive re-drives an in-doubt accept effect that ReconcileInDoubt returned to
// `approved` (the S02.7 class-A crash-window replay, R8): it re-squashes the
// candidate onto the SAME pinned expect-sha and blind-replays the CAS push
// against that sha — NEVER a new sha. A push success → Succeed; a lease
// rejection (the ref moved) → Fail + merge card. The effect's idempotency key
// and attempt count survive the replay (gates.BeginExecute), so the at-least-
// once retry is invisible at the ref-level CAS.
func (a *Accepter) ReDrive(ctx context.Context, effectID string) (Outcome, error) {
	eff, err := a.cfg.Journal.Get(ctx, effectID)
	if err != nil {
		return Outcome{}, err
	}
	if eff.Class != gates.ClassA {
		return Outcome{}, fmt.Errorf("accept: effect %q is not a class-A accept", effectID)
	}
	if eff.State != gates.EffectApproved {
		return Outcome{}, fmt.Errorf("accept: re-drive wants an approved (reconciled) effect, %q is %s", effectID, eff.State)
	}
	var p acceptPayload
	if err := json.Unmarshal(eff.Payload, &p); err != nil {
		return Outcome{}, fmt.Errorf("accept: parse pinned payload: %w", err)
	}
	return a.executeAccept(ctx, executeSpec{
		effectID: effectID, projectID: p.ProjectID, deliverableID: p.DeliverableID,
		acceptingUser: eff.UserID, authorName: p.AuthorName,
		onto: p.ExpectSHA, candidate: p.Candidate, trailers: p.Trailers, protectedRef: p.Ref,
		subject: p.Subject, provenance: p.Provenance, gitProfile: p.GitProfile,
		when: eff.ApprovedTS,
	})
}

// ReDriveApproved re-drives every approved class-A ACCEPT effect (F4): a crash
// leaves an accept effect approved — either between Approve and BeginExecute, or
// returned there from `executing` by ReconcileInDoubt. Run at startup right
// after the recovery ladder's reconcile, this leads reconcile to the push being
// re-driven with the pinned expect-sha. Non-accept class-A effects are skipped
// (their own executors re-drive them). Returns the number re-driven; a
// per-effect error aborts so the caller can log and retry next boot.
func (a *Accepter) ReDriveApproved(ctx context.Context) (int, error) {
	approved, err := a.cfg.Journal.InState(ctx, gates.EffectApproved)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, eff := range approved {
		if eff.Class != gates.ClassA {
			continue
		}
		var p acceptPayload
		if json.Unmarshal(eff.Payload, &p) != nil || p.DeliverableID == "" || p.ProjectID == "" {
			continue // not an accept effect
		}
		if _, err := a.ReDrive(ctx, eff.ID); err != nil {
			return n, fmt.Errorf("accept: re-drive %s: %w", eff.ID, err)
		}
		n++
	}
	return n, nil
}

// executeAccept is the shared execute phase (BeginExecute BEFORE the landing →
// squash → CAS push or local advance → Succeed|Fail), driven by a fresh Accept
// or a re-drive. The push is always blind against the pinned expect-sha; a lease
// rejection is a merge card, never a new-sha retry (S13.6 step 4 / R8).
//
// It carries the ONE arm decision the S13.7 registry makes: a project entry that
// records a remote lands through the broker's CAS push, one that records none
// lands in its own store (landLocal). Both arms run every other step of the same
// act, and both go through the same class-A effect with the same payload shape.
func (a *Accepter) executeAccept(ctx context.Context, s executeSpec) (Outcome, error) {
	if _, err := a.cfg.Journal.BeginExecute(ctx, s.effectID); err != nil {
		return Outcome{}, err
	}
	e, err := a.cfg.Project.Get(ctx, s.projectID)
	if err != nil {
		return Outcome{}, err
	}
	// Re-validate applies-cleanly onto the PINNED expect-sha (deterministic
	// tree); a collision now is a merge card + Fail.
	mr, err := a.cfg.Project.AppliesClean(ctx, s.projectID, s.onto, s.candidate)
	if err != nil {
		return Outcome{}, err
	}
	if !mr.Clean {
		a.failEffect(ctx, s.effectID, "collision")
		return Outcome{EffectID: s.effectID, Card: mergeCard(s.deliverableID, s.projectID, s.onto, s.candidate, "applies-clean-collision", mr.Conflicts)}, nil
	}
	sign, err := a.signerFor(ctx, s.acceptingUser)
	if err != nil {
		return Outcome{}, err
	}
	commit, err := a.cfg.Project.SquashAccept(ctx, s.projectID, project.SquashInput{
		Tree: mr.Tree, Parent: s.onto, UserID: s.acceptingUser, AuthorName: s.authorName,
		Message: buildMessage(s.subject, s.provenance, s.trailers), When: s.when, Sign: sign,
	})
	if err != nil {
		a.failEffect(ctx, s.effectID, "squash failed")
		return Outcome{}, err
	}
	// THE ARM IS THE REGISTRY'S FACT, AND IT IS READ HERE (P3-GF11 §3). The
	// S13.7 entry's remote URL is the ONE authority for whether this ceremony
	// has a transport step: S13.6 step 4 ("push as the user over broker-held SSH
	// keys with explicit CAS") is defined in every word over a remote and a
	// credential, and S13.7 blesses by name the project that has neither. The
	// read happens in the SHARED execute phase so a fresh accept and a
	// crash-window re-drive take the same arm from the same fact — never from a
	// payload member and never from a caller's flag.
	//
	// The BROKER stays the guardrail on the other side of this branch: its
	// refusal of a push request with no remote (internal/broker/git.go) is
	// correct and untouched. Teaching it to no-op such a request "successfully"
	// would silently mask a genuinely missing remote on a store that should have
	// one, and would put registry knowledge into a component that deliberately
	// imports no DB.
	if e.RemoteURL == "" {
		return a.landLocal(ctx, s, commit)
	}
	res, err := a.cfg.Push.Push(broker.Request{
		RepoDir: e.StorePath, Remote: e.RemoteURL,
		Refs:       []broker.RefUpdate{{Ref: s.protectedRef, ExpectSHA: s.onto, SrcSHA: commit}},
		Protected:  e.ProtectedRefs,
		Authorized: true,
		Profile:    s.gitProfile,
	})
	if err != nil {
		a.failEffect(ctx, s.effectID, "push failed")
		return Outcome{}, fmt.Errorf("%w: %w", ErrPushFailed, err)
	}
	if res.Rejected {
		a.failEffect(ctx, s.effectID, "lease rejected")
		return Outcome{
			EffectID: s.effectID,
			Card:     mergeCard(s.deliverableID, s.projectID, s.onto, s.candidate, "lease-rejected", "the ref moved since approval"),
		}, nil
	}

	// The commit is now on the remote's protected ref but the LOCAL default
	// branch is not yet advanced — the in-flight window during which a
	// concurrent accept for this project stacks onto HEAD-plus-first (F2).
	a.markInflight(s.projectID, s.effectID, commit)
	defer a.clearInflight(s.projectID, s.effectID)
	if a.AfterPush != nil {
		a.AfterPush(s.projectID, commit)
	}

	if _, err := a.cfg.Journal.Succeed(ctx, s.effectID, mustJSON(map[string]string{"commit": commit})); err != nil {
		return Outcome{}, err
	}
	acc, err := a.cfg.Review.Accept(ctx, s.deliverableID, s.acceptingUser)
	if err != nil {
		return Outcome{}, err
	}
	if err := a.cfg.Project.AdvanceDefaultBranch(ctx, s.projectID, commit); err != nil {
		return Outcome{}, err
	}
	routed, err := a.fireSibling(ctx, s.projectID)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Accepted: true, Commit: commit, EffectID: s.effectID, Superseded: acc.Superseded, RoutedRuns: routed}, nil
}

// landLocal is the S13.6 ceremony MINUS TRANSPORT (P3-GF11 R1): on a project
// whose S13.7 registry entry records no remote there is no push to compose, and
// the attributed commit advanced onto the store's own default branch IS the
// official landing. That store is not a scratch copy — S13.1 keeps minted
// revision refs "in the platform-owned project store under a platform ref
// namespace" and S13.5 has snapshot commits never reach a user-facing remote at
// all, so the platform-owned store is already the permanent record for a
// project that has no outward one.
//
// EVERY OTHER STEP IS THE SAME ACT. The applies-cleanly gate, the trailer
// invariant, the payload-hash pin, the PIN step-up and the platform squash all
// ran before this function; only the transport differs, and the difference is
// read from the registry rather than chosen by anybody.
//
// WHY THIS ARM STILL JOURNALS (R2), where the RW-17 pinned arm does not: the
// pinned arm moves DB rows only, whose exactly-once is the transaction's own,
// while this arm mutates the git store — a side effect OUTSIDE the transaction's
// guarantee, which is precisely the non-atomic shape S02.7's two-phase
// discipline exists for. Succeed comes AFTER the advance for the same reason it
// comes after the push on the other arm: an effect is recorded as done only once
// it has happened.
//
// The crash-window replay is idempotent by construction: the squash is
// content-addressed and pinned to the effect's approval time (same bytes ⇒ same
// sha) and AdvanceDefaultBranch no-ops on a sha the branch already carries or
// has incorporated. A branch that moved somewhere the pinned sha cannot
// fast-forward onto is the S13.6 collision answer — the same reviewable merge
// card a stale CAS lease earns on the push arm, never a silent overwrite and
// never a raw fault.
//
// There is deliberately no markInflight here. That window is "the commit is on
// the remote and the local branch is not yet advanced", and on this arm the
// advance IS the landing — the next candidate for the project reads the moved
// HEAD directly, so a look-ahead would be a claim about a state that never
// exists.
func (a *Accepter) landLocal(ctx context.Context, s executeSpec, commit string) (Outcome, error) {
	if err := a.cfg.Project.AdvanceDefaultBranch(ctx, s.projectID, commit); err != nil {
		if errors.Is(err, project.ErrNotFastForward) {
			a.failEffect(ctx, s.effectID, "default branch moved")
			return Outcome{
				EffectID: s.effectID,
				Card: mergeCard(s.deliverableID, s.projectID, s.onto, s.candidate,
					"lease-rejected", "the project's default branch moved since approval"),
			}, nil
		}
		a.failEffect(ctx, s.effectID, "local landing failed")
		return Outcome{}, err
	}
	if _, err := a.cfg.Journal.Succeed(ctx, s.effectID,
		mustJSON(map[string]string{"commit": commit, "landing": landingLocal})); err != nil {
		return Outcome{}, err
	}
	acc, err := a.cfg.Review.Accept(ctx, s.deliverableID, s.acceptingUser)
	if err != nil {
		return Outcome{}, err
	}
	routed, err := a.fireSibling(ctx, s.projectID)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Accepted: true, Commit: commit, EffectID: s.effectID, Superseded: acc.Superseded, RoutedRuns: routed}, nil
}

// signerFor resolves the accepting user's STRUCTURAL signing posture (F3) into a
// commit signer, or nil when the user does not sign. Never a per-call flag.
func (a *Accepter) signerFor(ctx context.Context, user string) (func([]byte) ([]byte, error), error) {
	if a.cfg.SigningPosture == nil {
		return nil, nil
	}
	signs, profile, err := a.cfg.SigningPosture(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("accept: resolve signing posture: %w", err)
	}
	if !signs {
		return nil, nil
	}
	if a.cfg.Signer == nil {
		return nil, fmt.Errorf("accept: user %q signs but no broker signer is wired", user)
	}
	return func(p []byte) ([]byte, error) { return a.cfg.Signer(profile, "git", p) }, nil
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

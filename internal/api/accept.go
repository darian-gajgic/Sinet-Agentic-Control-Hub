package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// accept.go — the S13.6 accept, as one High-tier action (P3-B6-3B).
//
// THE ACCEPT IS THE ONLY OUTWARD ACT ON THIS API, AND IT EXITS THROUGH ONE
// DOOR. This file calls `accept.Accepter.Accept` and nothing else: the
// applies-cleanly depth-1 merge gate, the platform squash, the class-A effect
// through the S02.7 journal (Propose → Approve → BeginExecute BEFORE the push →
// Succeed|Fail) and the broker CAS push are all inside that one call, which is
// what makes the crash-window re-drive story hold. internal/api imports neither
// internal/broker nor internal/project, asserted by an import scan — a transport
// that could reach the pusher would be a second way out (D7; §24).
//
// WHAT THE CARD PINS, AND WHAT IT DELIBERATELY DOES NOT. The payload hash
// covers {deliverable, revision, content pin, rendered trailers} — what the
// reviewer SAW about the thing being pushed. The project's expect-sha is NOT in
// it: HEAD moving between the read and the accept is not a stale card, it is a
// collision, and S13.6 already answers a collision with a merge card rather
// than a refusal. Pinning HEAD would turn every concurrent accept into a 409
// the reviewer cannot act on.
//
// RETRY-SAFETY IS RESOLVED-FIRST, THEN PINNED (the §39 order). A POST against
// an already-accepted deliverable reads the resolved state back at 200 with
// applied:false, so a phone retry can never double-fire; the state guard inside
// review.Accept is the structural backstop underneath that courtesy.

// The three merge-card options have NO executor at v0, and the card says so per
// option (OQ4). agent-auto-resolve is a bounded rework through an S07 seam that
// is not built; resolve-manually is a human act in the working copy that the
// platform records no verb for; abort-to-new-attempt has no executor either,
// but the landed follow-up door reaches the same outcome and is named. An
// option rendered as a button that 403s or 501s is the dead-control shape the
// answerable-with-a-reason discipline exists to prevent.

// MergeCardOption is one collision option with its honest answerability.
type MergeCardOption struct {
	Option     string `json:"option"`
	Answerable bool   `json:"answerable"`
	Reason     string `json:"reason"`
	// Route + Preset name a LANDED door that reaches the same outcome, when one
	// exists. It is not this option's executor — it is where the person goes.
	Route  string `json:"route,omitempty"`
	Preset string `json:"preset,omitempty"`
}

// MergeCardView is the reviewable collision surface (S13.6 step 1): the card
// verbatim from internal/accept, its three options with their answerability,
// and a plain statement of where this card lives.
type MergeCardView struct {
	Card    *accept.MergeCard `json:"card"`
	Options []MergeCardOption `json:"options"`
	// Durability states the OQ4 disposition as data: v0 stores no merge card.
	Durability string `json:"durability"`
}

// mergeCardDurability is the OQ4 disposition, served rather than only
// commented, so a surface knows this card is the response's own data and not a
// row it can come back to.
const mergeCardDurability = "this card is the accept response's own data: v0 keeps no merge-card store, because a durable copy would be a side " +
	"store of something the log already explains (the derive-from-log rule). A collision that reached the execute phase left a FAILED accept effect " +
	"in the S02.7 journal, queryable by its effect_id; a collision caught at the applies-cleanly gate proposed no effect, because nothing was ever approved."

func mergeCardOptions(deliverableID string) []MergeCardOption {
	return []MergeCardOption{
		{Option: accept.OptAgentAutoResolve, Answerable: false,
			Reason: "no executor at v0: agent-auto-resolve is a bounded rework through an S07 seam that is not built, and nothing on this API runs it"},
		{Option: accept.OptResolveManually, Answerable: false,
			Reason: "no executor by design: resolving the conflict is a human act in the project working copy, and the platform records no verb for work it does not perform"},
		{Option: accept.OptAbortToNewAttempt, Answerable: false,
			Reason: "no executor at v0, but the landed door reaches the same outcome: spawn a successor task off the moved base through the S13.9 follow-up",
			Route:  "POST /api/deliverables/" + deliverableID + "/follow-up", Preset: "revision"},
	}
}

// ── GET /api/deliverables/{deliverable}/accept-card ─────────────────────────

// AcceptSigning is the S13.6 step-5 posture stated as data, secret-free.
//
// Signing is NOT a choice on this form. It is the accepting user's structural
// per-user posture — all-or-nothing, so the platform can never produce a mixed
// signed/unsigned history — and the broker performs it with a key that never
// leaves the broker. There is deliberately no signing flag in the accept body:
// a per-call flag is exactly how a mixed history gets made.
type AcceptSigning struct {
	Structural      bool   `json:"structural"`
	PerCallFlag     bool   `json:"per_call_flag"`
	KeyLeavesBroker bool   `json:"key_leaves_broker"`
	Statement       string `json:"statement"`
}

// acceptSigningPosture is the one instance of that statement.
func acceptSigningPosture() AcceptSigning {
	return AcceptSigning{
		Structural: true, PerCallFlag: false, KeyLeavesBroker: false,
		// S13.6 step 5: signing is all-or-nothing per user, set once and not
		// per accept; the broker holds the key and no key material crosses here.
		Statement: "Whether your commits are signed is settled once for your account, not chosen here: it is all your commits or none of them. " +
			"The signing key is held by the credential broker, which does the signing — the key never reaches this page, and there is no per-accept " +
			"switch. This page does not ask the broker whether your signing is on, so it does not claim either way.",
	}
}

// The landing vocabulary (P3-GF11 R4/OQ2): where the official copy of this
// work ends up if the accept is made. It is a FACT ABOUT THE ACT, never a
// refusal taxonomy — `acceptable` + `reason` stay the one gate for whether the
// act is open at all.
//
// The value is derived from the S13.7 registry entry and the revision's own
// pin, which are the same two facts the accept orchestration reads to select
// its arm, so the card can never name a landing the verb will not make.
const (
	// LandingRemotePush: the S13.6 step-4 CAS push to the project's remote.
	LandingRemotePush = "remote-push"
	// LandingLocalStore: the project registers no remote, so the attributed
	// commit on the store's own default branch is the official copy (S13.7).
	LandingLocalStore = "local-store"
	// LandingDecisionRecord: the RW-17 pinned arm — the revision pins content
	// rather than a snapshot commit, so the durable record is the decision
	// against that immutable pin (S13.1).
	LandingDecisionRecord = "decision-record"
)

// AcceptCard is the accept decision data, displayed BEFORE the accept (S13.6
// step 3 — the mis-attribution lesson: trailers are shown, not discovered in
// the commit afterwards).
type AcceptCard struct {
	DeliverableID string `json:"deliverable_id"`
	RevisionN     int    `json:"revision_n"`
	// PinKind + ContentPin are the revision's own immutable pin (S13.1): a
	// repo-backed revision pins its snapshot commit, which is the candidate the
	// accept pushes.
	PinKind      string `json:"pin_kind"`
	ContentPin   string `json:"content_pin"`
	ProjectID    string `json:"project_id"`
	ProtectedRef string `json:"protected_ref,omitempty"`
	// Landing says where the official copy of this work lands — one of the three
	// values above. It is absent, rather than guessed, for a card whose arm is
	// not yet determinable: a deliverable with no minted revision, and a
	// repo-backed one belonging to no project (which the accept refuses anyway,
	// so there is no landing to name).
	Landing string `json:"landing,omitempty"`
	// Trailers are the rendered attribution lines, byte-for-byte what the commit
	// will carry. Their inputs are platform facts (below), never model output.
	// On the payload-pinned arm no commit is made, so they are the revision's
	// machine attribution and nothing more — the card's reason and tier statement
	// both say that nothing is pushed.
	Trailers      string           `json:"trailers"`
	Provenance    AcceptProvenance `json:"provenance"`
	Signing       AcceptSigning    `json:"signing"`
	Tier          string           `json:"tier"`
	TierStatement string           `json:"tier_statement"`
	// PayloadHash is the pin the accept body must quote back.
	PayloadHash string `json:"payload_hash"`
	Acceptable  bool   `json:"acceptable"`
	Reason      string `json:"reason"`
	Route       string `json:"route"`
}

// AcceptProvenance names WHERE each trailer input came from. Every one is a
// platform fact read off the run that PRODUCED the revision — the routing
// record the stage skeleton emitted and the engine session's substrate — so no
// string a model produced can name a co-author (the model-output-is-untrusted-
// input rule).
//
// The two run refs are both served because they answer different questions and
// are usually different runs: MintingRunID is the verification handoff that
// froze the content (Spec S13.1), ProducingRunID the execute leg whose settled
// S08.8 selection made it. A revision minted before migration 0026 records no
// producing run and resolves through its minting run.
type AcceptProvenance struct {
	MintingRunID   string `json:"minting_run_id,omitempty"`
	ProducingRunID string `json:"producing_run_id,omitempty"`

	Engine        string `json:"engine,omitempty"`
	Model         string `json:"model,omitempty"`
	Lane          string `json:"lane,omitempty"`
	VendorNoreply string `json:"vendor_noreply,omitempty"`
	// Absent states why the trailers could not be rendered, when they could not.
	Absent string `json:"absent,omitempty"`
}

// acceptTierStatement is the S15.6 High-tier fact, on the card where the person
// decides rather than only in a doc. Each arm states what ITS act does: the push
// arm's High tier comes from the outward push, and the pinned arm keeps the same
// ceremony without one.
//
// The pinned arm's tier is a deliberate, conservative sub-choice (P3-RW-17):
// S13.6 derives High from the push, so a payload-pinned accept could argue for
// less. It keeps the full ceremony — payload-hash echo plus PIN step-up — for
// accept-family uniformity: one act, one posture, and an owner who never has to
// wonder which kind of accept they are making. Relaxing it later is an
// operator-visible change, which is the right direction for that decision.
// The local-store arm's tier is the SAME conservative sub-choice for the same
// reason (P3-GF11 R4): the ceremony is identical minus its transport, and one
// accept family keeps one posture. What its statement must not do is claim the
// push it will not make — a tier sentence about a shared branch, shown to
// someone accepting into a store that has no remote, is untrue at the moment it
// matters most.
const (
	// S15.6 tier + S01.9 step-up: a high-tier act re-asks for the PIN in the
	// same request and never rides an idle session's elevation.
	acceptTierStatement = "This is a high-stakes act: it puts a commit on a branch other people work from, so it is never bundled with other " +
		"answers and you type your PIN again as part of this request. Having signed in earlier is not enough."
	acceptPinnedTierStatement = "This is a high-stakes act: you type your PIN again as part of this request and it is never bundled with other " +
		"answers. Nothing is pushed — this deliverable is pinned to its exact content, so your decision is recorded against that pin, which " +
		"cannot change afterwards. Accepting works the same way wherever the work lands. Having signed in earlier is not enough."
	acceptLocalTierStatement = "This is a high-stakes act: you type your PIN again as part of this request and it is never bundled with other " +
		"answers. Nothing is sent anywhere — this project has no remote address, so the commit is written into its own store on this machine. " +
		"Accepting works the same way wherever the work lands. Having signed in earlier is not enough."
)

// acceptPinCore is the canonical accept-card core the payload hash covers.
type acceptPinCore struct {
	DeliverableID string `json:"deliverable_id"`
	RevisionN     int    `json:"revision_n"`
	ContentPin    string `json:"content_pin"`
	Trailers      string `json:"trailers"`
}

func (s *Server) handleAcceptCard(w http.ResponseWriter, r *http.Request) {
	if !s.reviewReady(w) || !s.projReady(w) {
		return
	}
	d, ok := s.deliverableScope(w, r)
	if !ok {
		return
	}
	card, err := s.acceptCard(r.Context(), d)
	if err != nil {
		s.writeSurface(w, nil, s.reviewErr(err))
		return
	}
	s.writeReadJSON(w, card)
}

// acceptCard builds the decision data for one deliverable. It answers for ANY
// state — an accepted or non-repo-backed deliverable gets a card that says why
// the act is closed rather than a refusal that hides the facts.
func (s *Server) acceptCard(ctx context.Context, d review.Deliverable) (AcceptCard, error) {
	card := AcceptCard{
		DeliverableID: d.ID, ProjectID: d.ProjectID,
		Signing: acceptSigningPosture(), Tier: tierHigh, TierStatement: acceptTierStatement,
		Route: "POST /api/deliverables/" + d.ID + "/accept",
	}
	if d.CurrentRevision < 1 {
		card.Reason = acceptNoRevisionReason
		return card, nil
	}
	rev, err := s.review.RevisionAt(ctx, d.ID, d.CurrentRevision)
	if err != nil {
		return AcceptCard{}, err
	}
	card.RevisionN, card.PinKind = rev.N, rev.PinKind
	if rev.SnapshotSHA == "" {
		// The pinned arm: the act is a recorded decision, not an outward push, and
		// the tier statement says which one the person is about to make.
		card.TierStatement, card.Landing = acceptPinnedTierStatement, LandingDecisionRecord
	}
	card.ContentPin = revisionContentPin(rev)
	card.Provenance = s.acceptProvenance(ctx, rev)
	if card.Provenance.Absent == "" {
		card.Trailers = accept.RenderTrailers(card.Provenance.Engine, card.Provenance.Model, card.Provenance.VendorNoreply)
	}
	// The protected ref is where the push GOES, so it is a push-arm fact even
	// when a payload-pinned deliverable belongs to a project: naming a ref the
	// accept will not touch would be the card claiming a target it has none of.
	if rev.SnapshotSHA != "" && d.ProjectID != "" {
		card.ProtectedRef = s.protectedRef(ctx, d.ProjectID)
		// The repo arm's landing is the registry's own fact — the same column the
		// accept orchestration reads to choose its transport (P3-GF11 §3). The
		// protected ref is served on BOTH repo landings because both advance that
		// branch: on the local one it is the branch the official copy lands on, so
		// naming it is not a claim about a target the act does not have.
		if remote, ok := s.projectRemote(ctx, d.ProjectID); ok {
			card.Landing = LandingRemotePush
			if remote == "" {
				card.Landing, card.TierStatement = LandingLocalStore, acceptLocalTierStatement
			}
		}
	}
	hash, err := gates.CanonicalHash(mustMarshal(acceptPinCore{
		DeliverableID: d.ID, RevisionN: rev.N, ContentPin: card.ContentPin, Trailers: card.Trailers,
	}))
	if err != nil {
		return AcceptCard{}, fmt.Errorf("pin accept card: %w", err)
	}
	card.PayloadHash = hash
	card.Acceptable, card.Reason = acceptable(s.accept != nil, d, rev, card.Provenance, card.Landing)
	return card, nil
}

// acceptable is the ONE place the accept's preconditions are expressed, so the
// card and the verb can never disagree about whether the act is open.
//
// THE PIN KIND SELECTS THE ARM; IT IS NEVER A REFUSAL (P3-RW-17). Feature list
// 5.8, D10, S07.8 and the S13.1 state machine define the accept for every
// deliverable, and S13.1's pin vocabulary covers non-repo types by name — the
// payload hash IS the durable pin, so nothing needs inventing to record what was
// accepted. What the arms differ in is what HAPPENS: the repo arm pushes an
// attributed commit to a protected ref and therefore needs a project and
// renderable trailers; the pinned arm records a decision against an immutable
// content hash and needs neither, because a commit that is never made cannot
// mis-attribute anybody. Refusing the pinned arm for want of a push target was
// the defect this shape removes.
//
// LANDING IS COPY, NEVER A PRECONDITION (P3-GF11 R4). The landing argument
// changes only WHICH true sentence an open repo-backed act gets — a project
// with no remote is as acceptable as one with a remote, and S13.7 blesses it by
// name. Callers that do not resolve the arm pass "" and get the landing-neutral
// sentence; the accept door does exactly that, because it replaces this reason
// with its own per-arm open sentence anyway and resolving the registry per
// deliverable would buy a query per row for a string nobody reads.
func acceptable(wired bool, d review.Deliverable, rev review.Revision, prov AcceptProvenance, landing string) (bool, string) {
	switch {
	case !wired:
		return false, "no accept orchestration is composed in this process"
	case d.State != review.StateInReview:
		// S13.1/S13.6: only an in-review deliverable is acceptable.
		return false, fmt.Sprintf("this work is %s: only work that is still in review can be accepted", d.State)
	case rev.N < 1:
		return false, acceptNoRevisionReason
	}
	if rev.SnapshotSHA == "" {
		// S13.1 content pin; 5.8 durable decision.
		return true, fmt.Sprintf("open: accept with this payload_hash and your PIN in the same request. Nothing is pushed — version %d is "+
			"pinned to its exact content, so your decision is recorded against that pin and the work is filed as accepted", rev.N)
	}
	switch {
	case d.ProjectID == "":
		// S13.7: without a registry entry there is no protected ref.
		return false, "this work belongs to no project, so there is no branch to put it on"
	case prov.Absent != "":
		return false, prov.Absent
	}
	if landing == LandingLocalStore {
		// S13.7 registry + S13.1 landing: local store, no remote.
		return true, "open: accept with this payload_hash and your PIN in the same request. This project has no remote address, so the official " +
			"copy is the commit written into the project's own local store on this machine, on its main branch — nothing is sent anywhere"
	}
	return true, "open: accept with this payload_hash and your PIN in the same request"
}

// acceptNoRevisionReason is the pre-mint answer, shared by the card (which
// cannot read a revision that does not exist) and by acceptable itself, so the
// two cannot drift apart.
// (S13.1: a revision is minted at the verification handoff.)
const acceptNoRevisionReason = "no version of this work has been produced yet, so there is nothing to accept"

// revisionContentPin is the revision's immutable content pin as ONE string
// (Spec S13.1: repo-backed types pin a snapshot-commit sha, content types pin
// their content hash). It is what the card shows and what the payload hash
// covers, so a re-mint or a different revision moves the pin.
func revisionContentPin(rev review.Revision) string {
	if rev.SnapshotSHA != "" {
		return rev.SnapshotSHA
	}
	return rev.ContentSHA256
}

// acceptProvenance derives the trailer inputs from the run that PRODUCED the
// revision.
//
// Engine comes from the run's engine session substrate (Spec S03) and model
// from its settled `routing.decided` record (Spec S08.8) — both platform facts
// recorded by the machinery that made the decision. The vendor address is
// derived STRUCTURALLY from the lane rather than looked up: the platform does
// not invent a mailbox at a company it does not speak for, and the RFC 2606
// `.invalid` TLD is reserved so the address can never resolve or deliver. The
// form mirrors the landed committer noreply convention in internal/project.
//
// WHICH RUN IS ASKED. The producing run when the revision records one, else the
// minting run. The drain mints on the VERIFY leg (the verification tax rides
// it, S07.11) while the routing decision is emitted by the EXECUTE dispatch and
// only there, so resolving the minting run asked the wrong leg for facts it
// never records and closed the accept on correct work. The fallback is not a
// second guess at the same question — it is the unchanged read for a revision
// minted before the producing run was recorded at all (migration 0026), and
// there is deliberately no fallback FROM a recorded producing run: crediting
// the verify leg's engine for content the execute leg made would be a truthful-
// looking trailer about the wrong run.
//
// A revision with no resolvable attribution gets NO trailers and says exactly
// which facts are missing (never one that IS recorded). The alternative —
// rendering `Co-Authored-By:` around empty strings — would put a co-author line
// naming nobody into a permanent commit, which is the mis-attribution failure
// S13.6 step 3 exists to prevent.
func (s *Server) acceptProvenance(ctx context.Context, rev review.Revision) AcceptProvenance {
	p := AcceptProvenance{MintingRunID: rev.RunID, ProducingRunID: rev.ProducedBy}
	runID, role := rev.ProducedBy, "producing"
	if runID == "" {
		runID, role = rev.RunID, "minting"
	}
	if runID == "" {
		// S13.6 step 3.
		p.Absent = "nothing recorded which run produced this version, so the credit lines on the commit have no facts to be built from"
		return p
	}
	if pay, ok := s.proj.latestPayload(ctx, runID, "routing.decided"); ok {
		p.Model = firstString(pay, "model")
		p.Lane = firstString(pay, "lane")
	}
	var substrate sql.NullString
	if err := s.proj.db.QueryRowContext(ctx,
		`SELECT substrate FROM engine_sessions WHERE run_id = ? ORDER BY session_key DESC LIMIT 1`,
		runID).Scan(&substrate); err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.logger.Warn("accept: read engine session substrate", "run", runID, "err", err)
	}
	if substrate.Valid {
		p.Engine = substrate.String
	}
	if p.Engine == "" {
		p.Engine = p.Lane
	}
	if p.Engine == "" || p.Model == "" {
		p.Absent = attributionAbsence(role, runID, p.Engine, p.Model)
		return p
	}
	p.VendorNoreply = vendorNoreply(p.Lane, p.Engine)
	return p
}

// attributionAbsence states which trailer facts the resolution run is missing —
// exactly those, and never one the database records.
//
// The single sentence this replaced asserted BOTH absences whatever was
// actually missing, so a run that recorded its engine substrate and lacked only
// the routing decision was told "records no engine substrate": a withheld
// reason the platform's own rows contradict. A person reading a closed door has
// no way to check the claim, so the claim has to be true.
func attributionAbsence(role, runID, engine, model string) string {
	// S13.6 step 3: an accept never pushes a Co-Authored-By line naming nobody.
	const cannot = "the credit lines on the commit cannot be filled in from what the platform actually recorded — and an accept never " +
		"signs off work in the name of nobody"
	who := "the " + role + " run " + runID + " "
	switch {
	case engine == "" && model == "":
		return who + "records neither a routing decision nor an engine substrate, so " + cannot
	case model == "":
		return who + "records an engine substrate but no settled routing decision, so the model that made this work is unnamed and " + cannot
	default:
		return who + "records a routing decision but no engine substrate, so the engine that made this work is unnamed and " + cannot
	}
}

// vendorNoreply is the structural per-lane co-author address. See
// acceptProvenance for why it is derived rather than tabulated.
func vendorNoreply(lane, engine string) string {
	who := lane
	if who == "" {
		who = engine
	}
	return who + "@vendor.noreply.sinet.invalid"
}

// protectedRef reads the project's default branch from the S13.7 registry row.
// It is a plain SELECT on a table this process already owns the handle to —
// internal/api imports neither internal/project nor internal/broker, and an
// import to read one column would be the widening the accept walls forbid.
func (s *Server) protectedRef(ctx context.Context, projectID string) string {
	var branch string
	err := s.proj.db.QueryRowContext(ctx,
		`SELECT default_branch FROM repo_registry WHERE project_id = ?`, projectID).Scan(&branch)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		s.logger.Warn("accept: read project default branch", "project", projectID, "err", err)
		return ""
	}
	return "refs/heads/" + branch
}

// projectRemote reads the project's stored remote URL from the same S13.7
// registry row, as a second one-column SELECT beside protectedRef and for the
// same reason: internal/api imports neither internal/project nor internal/broker
// (§40-B), and an import to read one column would be the widening the accept
// walls forbid.
//
// It reports the column's value AND whether a registry row exists at all, so an
// unregistered project produces no landing claim rather than a false
// "local-store" — the empty string means "this project registers no remote",
// which is a different fact from "there is no project row to ask".
func (s *Server) projectRemote(ctx context.Context, projectID string) (string, bool) {
	var remote string
	err := s.proj.db.QueryRowContext(ctx,
		`SELECT remote_url FROM repo_registry WHERE project_id = ?`, projectID).Scan(&remote)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false
	}
	if err != nil {
		s.logger.Warn("accept: read project remote", "project", projectID, "err", err)
		return "", false
	}
	return remote, true
}

// ── POST /api/deliverables/{deliverable}/accept ─────────────────────────────

// acceptRequest is the accept body: the pin the card was shown with, the PIN
// verified at the act, and the two commit-message strings.
type acceptRequest struct {
	PayloadHash string `json:"payload_hash"`
	PIN         string `json:"pin"`
	Subject     string `json:"subject,omitempty"`
	Provenance  string `json:"provenance,omitempty"`
}

// AcceptOutcome is the accept's answer.
type AcceptOutcome struct {
	DeliverableID string `json:"deliverable_id"`
	// Applied is false for a read-back of an already-accepted deliverable and
	// for a collision — in both cases nothing new fired.
	Applied   bool   `json:"applied"`
	State     string `json:"state"`
	RevisionN int    `json:"revision_n"`
	Commit    string `json:"commit,omitempty"`
	EffectID  string `json:"effect_id,omitempty"`
	// Superseded lists deliverables this accept moved to superseded (S13.1);
	// RoutedRuns lists the project's active runs the S02.8 sibling-accept
	// freshness trigger routed to re-validation.
	Superseded []string       `json:"superseded"`
	RoutedRuns []string       `json:"routed_runs"`
	MergeCard  *MergeCardView `json:"merge_card,omitempty"`
	Detail     string         `json:"detail"`
}

func (s *Server) handleAccept(w http.ResponseWriter, r *http.Request) {
	if !s.reviewReady(w) || !s.projReady(w) {
		return
	}
	if s.accept == nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusServiceUnavailable, Code: "not_wired",
			Msg: "the S13.6 accept orchestration is not wired in this process"})
		return
	}
	d, err := s.review.Deliverable(r.Context(), r.PathValue("deliverable"))
	found := !errors.Is(err, review.ErrNotFound)
	if err != nil && found {
		s.writeSurface(w, nil, fmt.Errorf("read deliverable: %w", err))
		return
	}
	// AUTHORSHIP, NOT VISIBILITY. The accept is the owner's own outward act on
	// their own work, pushed with their own credentials (D9; the one-account-per-
	// human ToS posture). The operator's role bit decides what they can SEE and
	// never who they can act as — so the scope here drops the operator limb, the
	// same construction the ask answer uses (D10).
	id, _ := IdentityFrom(r.Context())
	if code, cerr := authorizeOwner(ownerScope{UserID: id.UserID}, d.Owner, found, "deliverable"); cerr != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: code, Code: httpCode(code), Msg: cerr.Error()})
		return
	}

	// Resolved first, then pinned (the §39 order). A repeat against an accepted
	// deliverable reads the recorded outcome back and fires nothing.
	switch d.State {
	case review.StateAccepted:
		out, err := s.acceptedReadBack(r.Context(), d)
		if err != nil {
			s.writeSurface(w, nil, s.reviewErr(err))
			return
		}
		s.writeReadJSON(w, out)
		return
	case review.StateSuperseded:
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusConflict, Code: "conflict",
			// S13.1: a superseded deliverable is never accepted afterwards.
			Msg: "this work has been superseded: a newer version was accepted in its place, and an older one is never accepted after that"})
		return
	}

	raw, ok := s.readBody(w, r)
	if !ok {
		return
	}
	var body acceptRequest
	if err := json.Unmarshal(raw, &body); err != nil {
		s.writeSurfaceErr(w, &SurfaceError{Status: http.StatusBadRequest, Code: "bad_body", Msg: err.Error()})
		return
	}
	card, err := s.acceptCard(r.Context(), d)
	if err != nil {
		s.writeSurface(w, nil, s.reviewErr(err))
		return
	}

	// The pin is checked BEFORE the step-up, which is the landed order in the
	// approvals family and the right one here for its own reason: a successful
	// step-up appends an `auth.reprompt` audit row, and a row recording an
	// authorization for an act that then 409s would claim an approval that never
	// took (the §39 drain-D6 principle). A stale card costs the reviewer a
	// re-read, never a PIN prompt they cannot use.
	if err := s.checkAcceptPin(card, body.PayloadHash); err != nil {
		s.writeAcceptErr(w, err, card)
		return
	}
	if !card.Acceptable {
		s.writeSurface(w, nil, badRequest(card.Reason))
		return
	}
	pinned, err := s.stepUp(r.Context(), id, ApprovalItem{StepUpRequired: true, Tier: tierHigh}, body.PIN)
	if err != nil {
		s.writeSurface(w, nil, err)
		return
	}
	if !pinned {
		// StepUpRequired is set unconditionally above, so stepUp either verified
		// the PIN or returned an error. Reaching here would mean the High-tier
		// gate silently opened, and the accept refuses rather than proceed.
		s.writeSurfaceErr(w, ErrPINRequired)
		return
	}

	out, err := s.accept.Accept(r.Context(),
		acceptInput(d, card, id.UserID, s.acceptingName(r.Context(), id.UserID), body))
	if err != nil {
		s.writeSurface(w, nil, s.acceptErr(err))
		return
	}
	s.writeReadJSON(w, acceptOutcome(d, card, out))
}

// acceptPushFailedMsg is the served answer for a push the broker could not
// perform. It says what did NOT happen, what state the work is in, and what the
// person can do — and it carries no internal error chain, because the cause is
// a platform fact the ops log records and a caller can neither read nor act on.
const acceptPushFailedMsg = "the commit could not be pushed to this project's remote, so nothing was accepted: the deliverable is still in review and " +
	"the accept can be made again once the remote is reachable. Nothing was left half-applied — the accept's effect is recorded as failed and no " +
	"branch moved. The underlying cause is in the platform's ops log."

// acceptBrokerDownMsg is the served answer when the platform's credential
// helper is not running on this machine. It is true at all three of the
// accept's broker touches (reading the signing posture, signing the commit,
// pushing), because all three mean the same thing to the person: the helper is
// down, so the act did not happen. It names the helper by what it does for
// them, says what state their work is in, and says what makes the accept
// possible again — and it carries no socket path, no dial verb and no internal
// chain, because those are ops facts a requester can neither read nor act on.
const acceptBrokerDownMsg = "the platform's credential helper — the part that signs commits and pushes in your name — is not running on this machine, so the work is " +
	"not accepted: the deliverable is still in review, no commit was made and no branch moved, and the accept's effect is recorded as failed. The accept can be made " +
	"again once the helper is running; starting it is the machine owner's act, not yours. The underlying cause is in the platform's ops log."

// acceptErr maps the accept orchestration's own refusals ON THE ERROR'S TYPE
// (CONVENTIONS §38), then falls through to the review store's mapping for
// everything the accept passes up unchanged.
//
// THE 502 REPLACES A CRASH-SHAPED ANSWER. A push the broker could not perform
// used to arrive here unmarked and reviewErr answered it 500 `internal` with
// `err.Error()` as the detail, which put the raw internal chain on the wire for
// a legitimate act on a legitimately-configured project. A failure to reach a
// remote is an upstream failure, which is what 502 says, and the deliverable is
// honestly still in review afterwards.
func (s *Server) acceptErr(err error) error {
	// THE BROKER-DOWN ARM IS CHECKED FIRST, and the order is load-bearing: a
	// push that failed because the broker was never reachable wraps BOTH
	// sentinels, and it belongs to this class rather than the remote's — 502
	// push_failed blames a project's remote for a helper that is down on this
	// machine. 503 is the honest status: a named platform component is not
	// running, which is neither an unexpected fault (500) nor an upstream
	// failure (502) (S01.3, CONVENTIONS §56).
	if errors.Is(err, accept.ErrBrokerUnavailable) {
		s.logger.Error("accept: the credential broker is unreachable", "err", err)
		return &SurfaceError{Status: http.StatusServiceUnavailable, Code: "broker_unreachable", Msg: acceptBrokerDownMsg}
	}
	if errors.Is(err, accept.ErrPushFailed) {
		s.logger.Error("accept: the broker could not perform the push", "err", err)
		return &SurfaceError{Status: http.StatusBadGateway, Code: "push_failed", Msg: acceptPushFailedMsg}
	}
	return s.reviewErr(err)
}

// acceptingName resolves the human-readable name the squash commit is authored
// under (S13.6 step 2: author = committer = the accepting user, set
// per-invocation). The name is the person's own display name from the S01.9
// user row — the id is the platform's handle for them, not what a permanent
// commit should call them. A user with no display name falls back to their id,
// which is honest: an empty author name would produce a commit nobody is named
// on. The committer EMAIL is not this surface's — internal/project derives the
// ID-based noreply form so attribution survives renames.
func (s *Server) acceptingName(ctx context.Context, userID string) string {
	if s.sessions == nil {
		return userID
	}
	u, err := s.sessions.User(ctx, userID)
	if err != nil || strings.TrimSpace(u.DisplayName) == "" {
		return userID
	}
	return u.DisplayName
}

// acceptInput assembles the ONE call. Engine, model and vendor address come
// from the CARD — the same platform facts the reviewer was shown and the same
// ones the payload hash covers — so the commit's trailers cannot differ from
// the trailers that were approved.
func acceptInput(d review.Deliverable, card AcceptCard, user, name string, body acceptRequest) accept.Input {
	return accept.Input{
		DeliverableID:     d.ID,
		AcceptingUser:     user,
		AcceptingUserName: name,
		ProjectID:         d.ProjectID,
		Subject:           body.Subject,
		Provenance:        body.Provenance,
		Engine:            card.Provenance.Engine,
		Model:             card.Provenance.Model,
		VendorNoreply:     card.Provenance.VendorNoreply,
	}
}

func acceptOutcome(d review.Deliverable, card AcceptCard, out accept.Outcome) AcceptOutcome {
	res := AcceptOutcome{
		DeliverableID: d.ID, RevisionN: card.RevisionN, EffectID: out.EffectID,
		Superseded: out.Superseded, RoutedRuns: out.RoutedRuns,
	}
	if res.Superseded == nil {
		res.Superseded = []string{}
	}
	if res.RoutedRuns == nil {
		res.RoutedRuns = []string{}
	}
	if out.Card != nil {
		res.State = review.StateInReview
		res.MergeCard = &MergeCardView{Card: out.Card, Options: mergeCardOptions(d.ID), Durability: mergeCardDurability}
		// S13.6 step 1 + S1.11: a conflict surfaces, never overwrites.
		res.Detail = "not accepted: this work does not fit cleanly onto the branch as it stands now, so it comes back to you as a merge to look " +
			"at rather than quietly overwriting what is there. The work is still in review and nothing was pushed."
		return res
	}
	res.Applied, res.State, res.Commit = true, review.StateAccepted, out.Commit
	// S13.6 push through the journal + the broker's CAS; S02.8 fires the
	// project's active runs for S02.6 re-validation.
	res.Detail = "accepted: one commit, credited to you, is now on the project's branch, this work is filed as accepted, and the project's " +
		"other running tasks were told to re-check themselves against it."
	if card.Landing == LandingLocalStore {
		// The local landing: the same ceremony minus its transport, and the answer
		// says so rather than reporting a push that was never composed.
		// S13.6/S13.7 local landing + S02.8 sibling re-validation.
		res.Detail = "accepted: one commit, credited to you, is now on this project's main branch in its own store on this machine, this work is " +
			"filed as accepted, and the project's other running tasks were told to re-check themselves against it. This project has no remote " +
			"address, so that commit is the official copy and nothing was sent anywhere."
	}
	if out.EffectID == "" {
		// The payload-pinned arm: no effect was proposed because nothing outward
		// happened. What is durable is the decision itself, against a pin that
		// cannot change (S13.1).
		// S13.1 content pin; 5.8/D10 durable, owner-attributed record.
		res.Detail = fmt.Sprintf("accepted: your decision is recorded against version %d's exact content, which cannot change afterwards, and "+
			"the work is filed as accepted in your name. Nothing was pushed — this work is pinned to its content rather than to a commit, so "+
			"there is no commit and nothing went outward.", card.RevisionN)
	}
	return res
}

// acceptedReadBack is the retry-safe answer for an already-accepted
// deliverable: the recorded outcome, applied:false, nothing fired.
func (s *Server) acceptedReadBack(ctx context.Context, d review.Deliverable) (AcceptOutcome, error) {
	rev, err := s.review.AcceptedRevision(ctx, d.ID)
	if err != nil {
		return AcceptOutcome{}, err
	}
	out := AcceptOutcome{
		DeliverableID: d.ID, Applied: false, State: d.State, RevisionN: rev.N,
		Superseded: []string{}, RoutedRuns: []string{},
		// S15.2 retry-safety: a repeat returns the recorded outcome.
		Detail: "already accepted: this is the answer that was recorded the first time, and nothing ran again. " +
			"Accepting twice can never produce a second commit.",
	}
	out.EffectID, out.Commit = s.acceptedEffect(ctx, d.ID)
	return out, nil
}

// acceptedEffect finds the class-A accept effect for a deliverable and reads
// the commit out of its recorded result. The journal is the record of what
// happened; there is no side store of accepted commits to consult.
func (s *Server) acceptedEffect(ctx context.Context, deliverableID string) (effectID, commit string) {
	var result sql.NullString
	err := s.proj.db.QueryRowContext(ctx,
		`SELECT effect_id, result FROM effects
		  WHERE class = ? AND state = ? AND json_extract(payload, '$.deliverable_id') = ?
		  ORDER BY created_ts DESC, effect_id DESC LIMIT 1`,
		string(gates.ClassA), string(gates.EffectSucceeded), deliverableID).Scan(&effectID, &result)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ""
	}
	if err != nil {
		s.logger.Warn("accept: read accepted effect", "deliverable", deliverableID, "err", err)
		return "", ""
	}
	if result.Valid {
		commit = firstString(json.RawMessage(result.String), "commit")
	}
	return effectID, commit
}

// ── the pin check and its refusal ───────────────────────────────────────────

// staleAcceptError carries the FRESH card back with the refusal, so the
// answerer's next act is a re-read rather than a guess.
type staleAcceptError struct{ card AcceptCard }

func (e *staleAcceptError) Error() string {
	// S15.2: an answer is pinned to the payload it was shown for.
	return "this card changed since you opened it: read it again and accept the version it now shows"
}

// checkAcceptPin is the S15.2 hash-pin limb for the ONE answerable act in this
// family. It hashes through gates.CanonicalHash and nothing else — a second
// canonicalization would be a second answer to "is this the same payload?", and
// the whole retry-safety rule rests on there being one.
func (s *Server) checkAcceptPin(card AcceptCard, given string) error {
	// TrimSpace parity with the landed checkPin (approvals.go): a whitespace-only
	// quote is a MISSING pin, not a stale one, and the two answers send the
	// caller to different places — 400 "you did not quote a hash" versus 409
	// "re-read the card".
	if strings.TrimSpace(given) == "" {
		// S15.2 hash pin.
		return badRequest(`missing "payload_hash": an accept has to quote the card it was shown for`)
	}
	if given != card.PayloadHash {
		return &staleAcceptError{card: card}
	}
	return nil
}

func (s *Server) writeAcceptErr(w http.ResponseWriter, err error, card AcceptCard) {
	var stale *staleAcceptError
	if errors.As(err, &stale) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(struct {
			Error   string     `json:"error"`
			Detail  string     `json:"detail"`
			Current AcceptCard `json:"current"`
		}{"stale_payload", stale.Error(), card})
		return
	}
	s.writeSurface(w, nil, err)
}

func mustMarshal(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		// The core is four plain fields; a marshal failure is impossible and an
		// empty document would silently pin everything to one hash.
		panic(fmt.Sprintf("api: marshal accept pin core: %v", err))
	}
	return raw
}

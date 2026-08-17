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
		Statement: "signing is your structural posture, not an option on this card: it is all-or-nothing per user (S13.6 step 5), the broker holds the " +
			"git-ssh-key and does the signing, and no key material or per-call flag crosses this surface. Whether YOUR commits are signed is a broker " +
			"fact this transport does not probe — asking would mean reaching past the accept orchestration that owns it.",
	}
}

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
// platform fact read off the revision's minting run — the routing record the
// stage skeleton emitted and the engine session's substrate — so no string a
// model produced can name a co-author (the model-output-is-untrusted-input
// rule).
type AcceptProvenance struct {
	MintingRunID  string `json:"minting_run_id,omitempty"`
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
const (
	acceptTierStatement = "High tier: this pushes a commit to a shared branch, so it is never batched and your PIN is re-prompted in the same " +
		"request. No elevation is inherited from an idle session (S15.6; S01.9)."
	acceptPinnedTierStatement = "High tier: your PIN is re-prompted in the same request and this is never batched. Nothing is pushed — this " +
		"deliverable pins its content, so the accept records your decision against that immutable pin. The accept family keeps one posture " +
		"whichever way the work lands. No elevation is inherited from an idle session (S15.6; S01.9)."
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
		card.TierStatement = acceptPinnedTierStatement
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
	}
	hash, err := gates.CanonicalHash(mustMarshal(acceptPinCore{
		DeliverableID: d.ID, RevisionN: rev.N, ContentPin: card.ContentPin, Trailers: card.Trailers,
	}))
	if err != nil {
		return AcceptCard{}, fmt.Errorf("pin accept card: %w", err)
	}
	card.PayloadHash = hash
	card.Acceptable, card.Reason = acceptable(s.accept != nil, d, rev, card.Provenance)
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
func acceptable(wired bool, d review.Deliverable, rev review.Revision, prov AcceptProvenance) (bool, string) {
	switch {
	case !wired:
		return false, "no accept orchestration is composed in this process"
	case d.State != review.StateInReview:
		return false, fmt.Sprintf("this deliverable is %s: only an in-review deliverable is accepted (S13.1/S13.6)", d.State)
	case rev.N < 1:
		return false, acceptNoRevisionReason
	}
	if rev.SnapshotSHA == "" {
		return true, fmt.Sprintf("open: accept with this payload_hash and your PIN in the same request. Nothing is pushed — revision %d pins "+
			"its content, so the accept records your decision against that immutable pin and files the work as accepted (S13.1; 5.8)", rev.N)
	}
	switch {
	case d.ProjectID == "":
		return false, "this deliverable belongs to no project, so there is no protected ref to push to (S13.7)"
	case prov.Absent != "":
		return false, prov.Absent
	}
	return true, "open: accept with this payload_hash and your PIN in the same request"
}

// acceptNoRevisionReason is the pre-mint answer, shared by the card (which
// cannot read a revision that does not exist) and by acceptable itself, so the
// two cannot drift apart.
const acceptNoRevisionReason = "no revision is minted yet, so there is nothing to accept (S13.1)"

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

// acceptProvenance derives the trailer inputs from the revision's minting run.
//
// Engine comes from the run's engine session substrate and model from the
// settled `routing.decided` record — both platform facts recorded by the
// machinery that made the decision. The vendor address is derived
// STRUCTURALLY from the lane rather than looked up: the platform does not
// invent a mailbox at a company it does not speak for, and the RFC 2606
// `.invalid` TLD is reserved so the address can never resolve or deliver. The
// form mirrors the landed committer noreply convention in internal/project.
//
// A revision with no resolvable engine attribution gets NO trailers and says
// so. The alternative — rendering `Co-Authored-By:` around empty strings —
// would put a co-author line naming nobody into a permanent commit, which is
// the mis-attribution failure S13.6 step 3 exists to prevent.
func (s *Server) acceptProvenance(ctx context.Context, rev review.Revision) AcceptProvenance {
	p := AcceptProvenance{MintingRunID: rev.RunID}
	if rev.RunID == "" {
		p.Absent = "this revision records no minting run, so the attribution trailers have no platform facts to render from (S13.6 step 3)"
		return p
	}
	if pay, ok := s.proj.latestPayload(ctx, rev.RunID, "routing.decided"); ok {
		p.Model = firstString(pay, "model")
		p.Lane = firstString(pay, "lane")
	}
	var substrate sql.NullString
	if err := s.proj.db.QueryRowContext(ctx,
		`SELECT substrate FROM engine_sessions WHERE run_id = ? ORDER BY session_key DESC LIMIT 1`,
		rev.RunID).Scan(&substrate); err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.logger.Warn("accept: read engine session substrate", "run", rev.RunID, "err", err)
	}
	if substrate.Valid {
		p.Engine = substrate.String
	}
	if p.Engine == "" {
		p.Engine = p.Lane
	}
	if p.Engine == "" || p.Model == "" {
		p.Absent = "the minting run " + rev.RunID + " records no engine substrate or routing decision, so the attribution trailers cannot be " +
			"rendered from platform facts — and an accept must never push a Co-Authored-By line naming nobody (S13.6 step 3)"
		return p
	}
	p.VendorNoreply = vendorNoreply(p.Lane, p.Engine)
	return p
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
			Msg: "this deliverable is superseded: a newer accepted version replaced it and the old one is never accepted afterwards (S13.1)"})
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
		s.writeSurface(w, nil, s.reviewErr(err))
		return
	}
	s.writeReadJSON(w, acceptOutcome(d, card, out))
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
		res.Detail = "not accepted: the candidate does not apply cleanly, so it surfaces as a reviewable merge card rather than a silent overwrite " +
			"(S13.6 step 1; S1.11). The deliverable is still in review and nothing was pushed."
		return res
	}
	res.Applied, res.State, res.Commit = true, review.StateAccepted, out.Commit
	res.Detail = "accepted: one attributed commit is on the protected ref through the effect journal and the broker's CAS push, the deliverable is " +
		"accepted, and the project's active runs were fired for S02.6 re-validation (S13.6; S02.8)."
	if out.EffectID == "" {
		// The payload-pinned arm: no effect was proposed because nothing outward
		// happened. What is durable is the decision itself, against a pin that
		// cannot change (S13.1).
		res.Detail = fmt.Sprintf("accepted: your decision is recorded against revision %d's immutable content pin and the deliverable is filed as "+
			"accepted, owner-attributed in the event log. Nothing was pushed — this deliverable pins content rather than a snapshot commit, so "+
			"there is no commit and no outward effect to journal (S13.1; 5.8/D10).", card.RevisionN)
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
		Detail: "already accepted: the recorded outcome is returned and nothing fired again (S15.2 retry-safety). " +
			"A repeated accept can never produce a second commit — the deliverable-state guard inside the accept is the structural backstop.",
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
	return "the accept card moved since you read it: re-read the card and accept the pin it now shows (S15.2)"
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
		return badRequest(`missing "payload_hash": an accept is pinned to the card it was shown for (S15.2)`)
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

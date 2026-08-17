package api_test

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// acceptapi_test.go — the S13.6 accept over HTTP (P3-B6-3B, R13–R15).
//
// Every assertion here is adversarial in the same direction: the negative
// probes re-read the world afterwards to prove NOTHING fired, and each has a
// non-tautological control proving the positive path still works — so a handler
// that refused everything could not pass this file.

// acceptFixture prepares a real project, a repo-backed in-review deliverable
// owned by alice, and returns the deliverable id.
func acceptFixture(t *testing.T, e *dlvEnv) string {
	t.Helper()
	e.prepareProject("proj", "alice", map[string]string{"a.txt": "base\n"})
	snap := e.snapshotCandidate("proj", "pipe", "a.txt", "candidate\n")
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "proj", "markdown", map[string]string{"a.txt": "candidate\n"}, snap)
	return "d-a"
}

func readAcceptCard(t *testing.T, e *dlvEnv, who, id string) api.AcceptCard {
	t.Helper()
	var card api.AcceptCard
	if err := json.Unmarshal([]byte(e.mustDo(t, who, "GET", "/api/deliverables/"+id+"/accept-card", "")), &card); err != nil {
		t.Fatalf("decode accept card: %v", err)
	}
	return card
}

func acceptBody(hash, pin string) string {
	return fmt.Sprintf(`{"payload_hash":%q,"pin":%q,"subject":"feat: ship the reviewed change","provenance":"Task: t-a"}`, hash, pin)
}

// ── R13: the card ───────────────────────────────────────────────────────────

func TestAcceptCardShowsTheTrailersAndThePinBeforeTheAccept(t *testing.T) {
	e := newDlvEnv(t)
	id := acceptFixture(t, e)
	card := readAcceptCard(t, e, "alice", id)

	if !card.Acceptable {
		t.Fatalf("an in-review repo-backed deliverable must be acceptable: %+v", card)
	}
	if card.RevisionN != 1 || card.ContentPin == "" {
		t.Errorf("the card must name the revision and its content pin: %+v", card)
	}
	if card.ProtectedRef != "refs/heads/main" {
		t.Errorf("the card must name the protected ref the push targets, got %q", card.ProtectedRef)
	}
	// The trailers are rendered from PLATFORM facts: the minting run's engine
	// substrate and its settled routing record. Nothing a model wrote can name a
	// co-author (S13.6 step 3).
	if card.Provenance.Engine != "claude-cli" || card.Provenance.Model != "opus-4-8" || card.Provenance.MintingRunID != "r-a" {
		t.Errorf("trailer inputs must derive from the minting run's platform facts: %+v", card.Provenance)
	}
	if !strings.Contains(card.Trailers, "Co-Authored-By: claude-cli opus-4-8") ||
		!strings.Contains(card.Trailers, "Assisted-by: claude-cli (opus-4-8) via Sinet") {
		t.Errorf("the card must show the rendered trailers verbatim, got %q", card.Trailers)
	}
	if !strings.HasSuffix(card.Provenance.VendorNoreply, ".invalid") {
		t.Errorf("the vendor address must be non-routable by construction, got %q", card.Provenance.VendorNoreply)
	}
	if card.Tier != "high" || card.TierStatement == "" {
		t.Errorf("the card must state the High-tier posture: %+v", card)
	}
	if !card.Signing.Structural || card.Signing.PerCallFlag || card.Signing.KeyLeavesBroker {
		t.Errorf("signing is structural, never a per-call flag, and the key never leaves the broker: %+v", card.Signing)
	}
	// The pin is the ONE canonicalization over the card's own core.
	want, err := gates.CanonicalHash(json.RawMessage(fmt.Sprintf(
		`{"deliverable_id":%q,"revision_n":%d,"content_pin":%q,"trailers":%q}`,
		id, card.RevisionN, card.ContentPin, card.Trailers)))
	if err != nil {
		t.Fatal(err)
	}
	if card.PayloadHash != want {
		t.Errorf("payload_hash %q is not gates.CanonicalHash over the card core %q", card.PayloadHash, want)
	}
}

// TestAcceptCardSaysWhyTheActIsClosed is the P3-RW-17 INVERSION of the landed
// test of this name (brief §4/§6, sanctioned by name). Its old body pinned the
// refusal of a content-pinned revision, which 5.8/D10/S13.1 say is the owner's
// act to make; what stays true — and what this now pins — is that a card whose
// act is closed by the deliverable's STATE says so as data. The state limb is
// the one refusal no pin kind can open: an accepted deliverable is finished.
func TestAcceptCardSaysWhyTheActIsClosed(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "proj", "markdown", map[string]string{"a.md": "x\n"}, "")
	if _, err := e.rev.Accept(e.ctx, "d-a", "alice"); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	card := readAcceptCard(t, e, "alice", "d-a")
	if card.Acceptable {
		t.Fatalf("an accepted deliverable must not be acceptable again: %+v", card)
	}
	if !strings.Contains(card.Reason, "accepted") {
		t.Errorf("the card must NAME why the act is closed, got %q", card.Reason)
	}
	if card.PayloadHash == "" {
		t.Error("the card still pins itself — the reason is data, not an error that hides the facts")
	}
}

// ── P3-RW-17: the pinned arm — the journey's final press for content tasks ──

func pinnedFixture(t *testing.T, e *dlvEnv) string {
	t.Helper()
	e.mkRun("t-a", "r-a", "alice")
	// The rewalk-a shape: content family → no project, no snapshot commit.
	e.mkDeliverable("d-pin", "alice", "t-a", "r-a", "", "markdown",
		map[string]string{"deliverable.md": "# Amber & Oak\n"}, "")
	return "d-pin"
}

// RW17-T1 (red): the card opens for a payload-pinned deliverable (5.8/D10/S13.1).
// Inverts the landed TestAcceptCardSaysWhyTheActIsClosed, which pinned the refusal.
func TestAcceptCardOpensForAPayloadPinnedDeliverable(t *testing.T) {
	e := newDlvEnv(t)
	id := pinnedFixture(t, e)
	card := readAcceptCard(t, e, "alice", id)
	if !card.Acceptable {
		t.Fatalf("V3 is universal: an in-review payload-pinned deliverable must be acceptable (5.8/D10/S13.1), got %+v", card)
	}
	if card.PinKind != "content" || card.ContentPin == "" || card.PayloadHash == "" {
		t.Errorf("the card must pin the act to the revision's immutable content pin (S13.1): %+v", card)
	}
	if card.ProtectedRef != "" {
		t.Errorf("no project, no protected ref — the card must not name a push target, got %q", card.ProtectedRef)
	}
	if !strings.Contains(card.Reason, "pushed") && !strings.Contains(card.Reason, "push") {
		t.Errorf("the open reason must say honestly that nothing is pushed, got %q", card.Reason)
	}
}

// RW17-T2 (red): the accept records the decision and pushes NOTHING (S13.1; R3).
func TestPayloadPinnedAcceptRecordsTheDecisionAndPushesNothing(t *testing.T) {
	e := newDlvEnv(t)
	id := pinnedFixture(t, e)
	card := readAcceptCard(t, e, "alice", id)
	before := e.count(review.EventAccepted)

	var out api.AcceptOutcome
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST",
		"/api/deliverables/"+id+"/accept", acceptBody(card.PayloadHash, dlvPIN))), &out); err != nil {
		t.Fatalf("decode accept outcome: %v", err)
	}
	if !out.Applied || out.State != review.StateAccepted {
		t.Fatalf("the accept must apply and land accepted, got %+v", out)
	}
	if out.Commit != "" || out.EffectID != "" {
		t.Errorf("a pinned accept makes no commit and journals no effect, got commit=%q effect=%q", out.Commit, out.EffectID)
	}
	if len(e.push.reqs) != 0 {
		t.Error("nothing may reach the broker for a payload-pinned accept")
	}
	var effects int
	if err := e.b.db.QueryRowContext(e.ctx, `SELECT COUNT(*) FROM effects`).Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if effects != 0 {
		t.Errorf("no class-A effect may exist for a pinned accept, found %d", effects)
	}
	if e.count(review.EventAccepted) != before+1 {
		t.Error("the accept must land exactly one deliverable.accepted event (S13.1 audit)")
	}
	d, err := e.rev.Deliverable(e.ctx, id)
	if err != nil || d.State != review.StateAccepted {
		t.Fatalf("the durable state must be accepted, got %+v err=%v", d, err)
	}

	// Retry-safety (R5): a repeat reads the resolved state back, fires nothing.
	var again api.AcceptOutcome
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST",
		"/api/deliverables/"+id+"/accept", acceptBody(card.PayloadHash, dlvPIN))), &again); err != nil {
		t.Fatal(err)
	}
	if again.Applied || e.count(review.EventAccepted) != before+1 {
		t.Errorf("a repeat must be applied:false with no second event, got %+v", again)
	}
}

// RW17-T3 (red): the pinned arm keeps the High-tier ceremony (named sub-choice).
func TestPayloadPinnedAcceptStillDemandsThePIN(t *testing.T) {
	e := newDlvEnv(t)
	id := pinnedFixture(t, e)
	card := readAcceptCard(t, e, "alice", id)
	code, out := e.do(t, "alice", "POST", "/api/deliverables/"+id+"/accept", acceptBody(card.PayloadHash, ""))
	if code != http.StatusUnauthorized || !strings.Contains(out, "pin_required") {
		t.Fatalf("the pinned arm keeps the step-up, got %d: %s", code, out)
	}
	if d, _ := e.rev.Deliverable(e.ctx, id); d.State != review.StateInReview {
		t.Error("nothing may change state before the step-up")
	}
}

// RW17-T4 (red): provenance absence closes only the PUSH arm (R4) — a pinned
// accept renders no permanent trailers, so absent trailer facts must not
// refuse the owner's decision.
func TestPayloadPinnedAcceptDoesNotRequireTrailerProvenance(t *testing.T) {
	e := newDlvEnv(t)
	// A deliverable minted with NO run: no substrate, no routing decision.
	if _, err := e.rev.EnsureDeliverable(e.ctx, review.EnsureInput{
		ID: "d-noprov", Owner: "alice", TaskID: "t-noprov", Type: "markdown",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.rev.MintRevision(e.ctx, review.MintInput{
		DeliverableID: "d-noprov", N: 1, Files: map[string]string{"deliverable.md": "x\n"},
	}); err != nil {
		t.Fatal(err)
	}
	card := readAcceptCard(t, e, "alice", "d-noprov")
	if !card.Acceptable {
		t.Fatalf("absent trailer provenance must not close the pinned arm: %+v", card)
	}
	if card.Trailers != "" {
		t.Errorf("no provenance still means no rendered trailers, got %q", card.Trailers)
	}
}

// TestAcceptCardWithoutProvenanceRendersNoTrailers pins the honest-absence
// branch: a revision whose minting run recorded no engine substrate and no
// routing decision gets NO trailers and is not acceptable, because a
// Co-Authored-By line naming nobody is exactly the mis-attribution failure the
// trailers exist to prevent.
func TestAcceptCardWithoutProvenanceRendersNoTrailers(t *testing.T) {
	e := newDlvEnv(t)
	e.prepareProject("proj", "alice", map[string]string{"a.txt": "base\n"})
	snap := e.snapshotCandidate("proj", "pipe", "a.txt", "candidate\n")
	// A deliverable minted with NO run at all: no substrate row, no routing row.
	if _, err := e.rev.EnsureDeliverable(e.ctx, review.EnsureInput{
		ID: "d-bare", Owner: "alice", TaskID: "t-bare", ProjectID: "proj", Type: "markdown",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.rev.MintRevision(e.ctx, review.MintInput{
		DeliverableID: "d-bare", N: 1, Files: map[string]string{"a.txt": "candidate\n"}, SnapshotSHA: snap,
	}); err != nil {
		t.Fatal(err)
	}
	card := readAcceptCard(t, e, "alice", "d-bare")
	if card.Trailers != "" {
		t.Errorf("with no platform provenance the card must render NO trailers, got %q", card.Trailers)
	}
	if card.Acceptable || card.Provenance.Absent == "" {
		t.Errorf("the absence must be stated and the act closed, got %+v", card)
	}
	code, out := e.do(t, "alice", "POST", "/api/deliverables/d-bare/accept", acceptBody(card.PayloadHash, dlvPIN))
	if code != http.StatusBadRequest {
		t.Errorf("accepting without renderable trailers must be refused, got %d: %s", code, out)
	}
}

// ── R14: the PIN, the pin, and what fires ───────────────────────────────────

func TestAcceptWithoutAPINDemandsOne(t *testing.T) {
	e := newDlvEnv(t)
	id := acceptFixture(t, e)
	card := readAcceptCard(t, e, "alice", id)
	code, out := e.do(t, "alice", "POST", "/api/deliverables/"+id+"/accept", acceptBody(card.PayloadHash, ""))
	if code != http.StatusUnauthorized || !strings.Contains(out, "pin_required") {
		t.Fatalf("a High-tier act must re-prompt the PIN, got %d: %s", code, out)
	}
	if len(e.push.reqs) != 0 {
		t.Error("nothing may be pushed before the step-up")
	}
}

func TestAcceptWithAWrongPINIsRefusedAndAudited(t *testing.T) {
	e := newDlvEnv(t)
	id := acceptFixture(t, e)
	card := readAcceptCard(t, e, "alice", id)
	before := e.count(auth.EventRepromptFailed)
	code, out := e.do(t, "alice", "POST", "/api/deliverables/"+id+"/accept", acceptBody(card.PayloadHash, "wrong-pin-entirely"))
	if code != http.StatusUnauthorized || !strings.Contains(out, "pin_rejected") {
		t.Fatalf("a wrong PIN must be refused, got %d: %s", code, out)
	}
	if e.count(auth.EventRepromptFailed) != before+1 {
		t.Error("a failed step-up must land on the audit log (auth.reprompt_failed)")
	}
	if len(e.push.reqs) != 0 {
		t.Error("a refused step-up must fire nothing")
	}
}

func TestAcceptWithAStalePinFiresNothingAndReturnsTheFreshCard(t *testing.T) {
	e := newDlvEnv(t)
	id := acceptFixture(t, e)
	failedBefore := e.count(auth.EventRepromptFailed)
	repromptBefore := e.count(auth.EventReprompt)

	code, body := e.do(t, "alice", "POST", "/api/deliverables/"+id+"/accept",
		acceptBody("sha256:not-the-card-you-were-shown", dlvPIN))
	if code != http.StatusConflict {
		t.Fatalf("a stale pin must be a 409, got %d: %s", code, body)
	}
	var refusal struct {
		Error   string         `json:"error"`
		Current api.AcceptCard `json:"current"`
	}
	if err := json.Unmarshal([]byte(body), &refusal); err != nil {
		t.Fatalf("decode refusal: %v", err)
	}
	if refusal.Error != "stale_payload" || refusal.Current.PayloadHash == "" {
		t.Fatalf("the refusal must carry the FRESH card back: %+v", refusal)
	}
	// The side-effect probe: nothing fired, and no step-up was even attempted —
	// an audit row claiming an authorization for an act that 409'd would record
	// an approval that never took.
	if len(e.push.reqs) != 0 {
		t.Error("a stale pin must never reach the pusher")
	}
	effects, err := e.journal.InState(e.ctx, gates.EffectProposed)
	if err != nil {
		t.Fatal(err)
	}
	if len(effects) != 0 {
		t.Errorf("a stale pin proposed %d effects — it must propose none", len(effects))
	}
	if e.count("deliverable.accepted") != 0 {
		t.Error("a stale pin must not move the deliverable")
	}
	if e.count(auth.EventReprompt) != repromptBefore || e.count(auth.EventRepromptFailed) != failedBefore {
		t.Error("a stale pin must not consume a step-up at all")
	}
	// The non-tautological control: the FRESH pin the refusal handed back works.
	if code, out := e.do(t, "alice", "POST", "/api/deliverables/"+id+"/accept",
		acceptBody(refusal.Current.PayloadHash, dlvPIN)); code != http.StatusOK {
		t.Fatalf("the fresh pin must be accepted: %d %s", code, out)
	}
}

// TestAcceptDistinguishesAMissingPinFromAStaleOne pins the TrimSpace parity
// with the landed checkPin: a whitespace-only quote is a MISSING pin, and the
// two refusals send the caller to different places — "you did not quote a hash"
// versus "re-read the card".
func TestAcceptDistinguishesAMissingPinFromAStaleOne(t *testing.T) {
	e := newDlvEnv(t)
	id := acceptFixture(t, e)
	for _, given := range []string{"", "   ", "\t\n "} {
		code, out := e.do(t, "alice", "POST", "/api/deliverables/"+id+"/accept", acceptBody(given, dlvPIN))
		if code != http.StatusBadRequest || !strings.Contains(out, "payload_hash") {
			t.Errorf("a blank pin %q must be a 400 naming the missing field, got %d: %s", given, code, out)
		}
	}
	// The non-tautological control: a NON-blank wrong hash is the other answer.
	if code, _ := e.do(t, "alice", "POST", "/api/deliverables/"+id+"/accept",
		acceptBody("sha256:wrong", dlvPIN)); code != http.StatusConflict {
		t.Error("a non-blank wrong hash must still be the 409 stale answer")
	}
	if len(e.push.reqs) != 0 {
		t.Error("no refused pin may fire anything")
	}
}

func TestCleanAcceptExitsThroughTheJournalAndTheBrokerCASPush(t *testing.T) {
	e := newDlvEnv(t)
	id := acceptFixture(t, e)
	card := readAcceptCard(t, e, "alice", id)
	head, err := e.proj.RepoHead(e.ctx, "proj")
	if err != nil {
		t.Fatal(err)
	}

	var out api.AcceptOutcome
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/deliverables/"+id+"/accept",
		acceptBody(card.PayloadHash, dlvPIN))), &out); err != nil {
		t.Fatalf("decode accept outcome: %v", err)
	}
	if !out.Applied || out.Commit == "" || out.EffectID == "" || out.MergeCard != nil {
		t.Fatalf("the accept did not complete: %+v", out)
	}
	if out.State != review.StateAccepted {
		t.Errorf("the accept must report the accepted state, got %q", out.State)
	}
	// ONE class-A effect, succeeded, approved by the accepting user.
	eff, err := e.journal.Get(e.ctx, out.EffectID)
	if err != nil {
		t.Fatal(err)
	}
	if eff.Class != gates.ClassA || eff.State != gates.EffectSucceeded || eff.ApprovedBy != "alice" {
		t.Errorf("want a succeeded class-A effect approved by alice, got %+v", eff)
	}
	// The push is CAS against the sha at approval, on the protected ref, and
	// carries the Authorized bit only the accept path sets (P-T12-1).
	if len(e.push.reqs) != 1 {
		t.Fatalf("want exactly one push, got %d", len(e.push.reqs))
	}
	req := e.push.reqs[0]
	if !req.Authorized {
		t.Error("the broker refuses a protected-ref push that is not an approved accept — Authorized must be set")
	}
	if len(req.Refs) != 1 {
		t.Fatalf("want one ref update, got %d", len(req.Refs))
	}
	if req.Refs[0].Ref != "refs/heads/main" || req.Refs[0].ExpectSHA != head || req.Refs[0].SrcSHA != out.Commit {
		t.Errorf("the CAS arguments are wrong: %+v (head %s, commit %s)", req.Refs[0], head, out.Commit)
	}
	// The durable state moved and the canonical row landed.
	d, err := e.rev.Deliverable(e.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if d.State != review.StateAccepted {
		t.Errorf("review state %q after a clean accept, want accepted", d.State)
	}
	if e.count("deliverable.accepted") != 1 {
		t.Errorf("want exactly one deliverable.accepted row, got %d", e.count("deliverable.accepted"))
	}
	// The S02.8 sibling-accept freshness trigger fired to the project's active
	// runs (the fixture declares one).
	if len(out.RoutedRuns) != 1 || out.RoutedRuns[0] != "r-sibling" {
		t.Errorf("the sibling-accept freshness fire must be reported, got %v", out.RoutedRuns)
	}
	// The commit carries the trailers the card showed — what was approved is
	// what was written.
	entry, err := e.proj.Get(e.ctx, "proj")
	if err != nil {
		t.Fatal(err)
	}
	msg := e.git("-C", entry.StorePath, "show", "-s", "--format=%B", out.Commit)
	if !strings.Contains(msg, "Co-Authored-By: claude-cli opus-4-8") {
		t.Errorf("the commit message must carry the approved trailers, got %q", msg)
	}
	// The commit is authored under the accepting PERSON's name, resolved from
	// their S01.9 user row — not under the platform's handle for them. The
	// fixture's display name differs from the id on purpose, so this fails if
	// the raw id is passed through (S13.6 step 2).
	if got := e.git("-C", entry.StorePath, "show", "-s", "--format=%an", out.Commit); got != "Alice Member" {
		t.Errorf("commit author name %q — want the accepting user's display name", got)
	}
}

// TestAcceptFallsBackToTheUserIDWhenNoDisplayNameExists pins the honest
// fallback: a person with no display name is named by their id rather than by
// nothing, because an empty author name would produce a commit nobody is on.
func TestAcceptFallsBackToTheUserIDWhenNoDisplayNameExists(t *testing.T) {
	e := newDlvEnv(t)
	e.exec(`UPDATE users SET display_name = '' WHERE user_id = ?`, "alice")
	id := acceptFixture(t, e)
	card := readAcceptCard(t, e, "alice", id)
	var out api.AcceptOutcome
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/deliverables/"+id+"/accept",
		acceptBody(card.PayloadHash, dlvPIN))), &out); err != nil {
		t.Fatal(err)
	}
	entry, err := e.proj.Get(e.ctx, "proj")
	if err != nil {
		t.Fatal(err)
	}
	if got := e.git("-C", entry.StorePath, "show", "-s", "--format=%an", out.Commit); got != "alice" {
		t.Errorf("commit author name %q — want the honest id fallback", got)
	}
}

func TestRepeatedAcceptReadsBackAndNeverDoubleFires(t *testing.T) {
	e := newDlvEnv(t)
	id := acceptFixture(t, e)
	card := readAcceptCard(t, e, "alice", id)
	first := e.mustDo(t, "alice", "POST", "/api/deliverables/"+id+"/accept", acceptBody(card.PayloadHash, dlvPIN))
	var applied api.AcceptOutcome
	if err := json.Unmarshal([]byte(first), &applied); err != nil {
		t.Fatal(err)
	}

	// The identical request again — the phone-retry shape.
	var repeat api.AcceptOutcome
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/deliverables/"+id+"/accept",
		acceptBody(card.PayloadHash, dlvPIN))), &repeat); err != nil {
		t.Fatal(err)
	}
	if repeat.Applied {
		t.Fatal("a repeat must never re-apply")
	}
	if repeat.State != review.StateAccepted || repeat.RevisionN != applied.RevisionN {
		t.Errorf("the repeat must read the resolved state back: %+v", repeat)
	}
	if repeat.Commit != applied.Commit || repeat.EffectID != applied.EffectID {
		t.Errorf("the repeat must return the recorded commit and effect: %+v vs %+v", repeat, applied)
	}
	// The double-fire probe.
	if len(e.push.reqs) != 1 {
		t.Errorf("a repeat pushed again: %d pushes", len(e.push.reqs))
	}
	if e.count("deliverable.accepted") != 1 {
		t.Errorf("a repeat minted a second accepted row: %d", e.count("deliverable.accepted"))
	}
}

func TestAcceptCollisionServesTheMergeCardAndLeavesTheDeliverableInReview(t *testing.T) {
	e := newDlvEnv(t)
	e.prepareProject("proj", "alice", map[string]string{"a.txt": "line1\nbase\n"})
	snap := e.snapshotCandidate("proj", "pipe", "a.txt", "line1\nCANDIDATE\n")
	e.mkRun("t-a", "r-a", "alice")
	e.mkDeliverable("d-a", "alice", "t-a", "r-a", "proj", "markdown", map[string]string{"a.txt": "line1\nCANDIDATE\n"}, snap)
	card := readAcceptCard(t, e, "alice", "d-a")

	// HEAD moves with a CONFLICTING change.
	entry, err := e.proj.Get(e.ctx, "proj")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry.StorePath, "a.txt"), []byte("line1\nHEADSIDE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.git("-C", entry.StorePath, "add", "-A")
	e.git("-C", entry.StorePath, "-c", "user.name=x", "-c", "user.email=x@x", "commit", "-q", "-m", "moved")

	var out api.AcceptOutcome
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/deliverables/d-a/accept",
		acceptBody(card.PayloadHash, dlvPIN))), &out); err != nil {
		t.Fatal(err)
	}
	// A collision is 200-class DATA, never a refusal and never a silent
	// overwrite (S13.6 step 1; S1.11).
	if out.Applied || out.MergeCard == nil || out.MergeCard.Card == nil {
		t.Fatalf("a collision must serve the merge card as data: %+v", out)
	}
	if out.MergeCard.Card.Reason != "applies-clean-collision" {
		t.Errorf("merge-card reason %q", out.MergeCard.Card.Reason)
	}
	if len(out.MergeCard.Card.Options) != 3 {
		t.Errorf("the card carries exactly three options, got %v", out.MergeCard.Card.Options)
	}
	// OQ4: the options serve with honest answerability — no dead buttons.
	if len(out.MergeCard.Options) != 3 {
		t.Fatalf("want three option entries, got %d", len(out.MergeCard.Options))
	}
	seen := map[string]bool{}
	for _, o := range out.MergeCard.Options {
		seen[o.Option] = true
		if o.Answerable {
			t.Errorf("option %q claims an executor that does not exist at v0", o.Option)
		}
		if o.Reason == "" {
			t.Errorf("option %q gives no reason", o.Option)
		}
	}
	for _, want := range []string{accept.OptAgentAutoResolve, accept.OptResolveManually, accept.OptAbortToNewAttempt} {
		if !seen[want] {
			t.Errorf("missing merge-card option %q", want)
		}
	}
	if out.MergeCard.Durability == "" {
		t.Error("the card must say where it lives (OQ4: return-only at v0)")
	}
	if len(e.push.reqs) != 0 {
		t.Error("a collision caught at the applies-cleanly gate must never push")
	}
	d, err := e.rev.Deliverable(e.ctx, "d-a")
	if err != nil {
		t.Fatal(err)
	}
	if d.State != review.StateInReview {
		t.Errorf("state %q after a collision, want in-review", d.State)
	}
}

func TestLeaseRejectionIsAMergeCardNeverANewShaRetry(t *testing.T) {
	e := newDlvEnv(t)
	id := acceptFixture(t, e)
	card := readAcceptCard(t, e, "alice", id)
	e.push.rejected = true

	var out api.AcceptOutcome
	if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "POST", "/api/deliverables/"+id+"/accept",
		acceptBody(card.PayloadHash, dlvPIN))), &out); err != nil {
		t.Fatal(err)
	}
	if out.Applied || out.MergeCard == nil || out.MergeCard.Card.Reason != "lease-rejected" {
		t.Fatalf("a stale lease must route to a merge card: %+v", out)
	}
	// EXACTLY ONE push: a lease failure is a collision, never a blind retry
	// against a new sha (S13.6 step 4).
	if len(e.push.reqs) != 1 {
		t.Errorf("want exactly one push attempt, got %d", len(e.push.reqs))
	}
	eff, err := e.journal.Get(e.ctx, out.EffectID)
	if err != nil {
		t.Fatal(err)
	}
	if eff.State != gates.EffectFailed {
		t.Errorf("effect state after a lease rejection = %s, want failed", eff.State)
	}
	d, err := e.rev.Deliverable(e.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if d.State != review.StateInReview {
		t.Errorf("state %q after a lease rejection, want in-review", d.State)
	}
}

// TestAcceptIsAuthorshipNotVisibility pins the D5/D9 line: the role bit decides
// what the operator can SEE, never who they can act AS. The accept is pushed
// with the accepting user's own credentials, so it is the owner's act alone.
func TestAcceptIsAuthorshipNotVisibility(t *testing.T) {
	e := newDlvEnv(t)
	id := acceptFixture(t, e)
	card := readAcceptCard(t, e, "alice", id)

	if code, out := e.do(t, "bob", "POST", "/api/deliverables/"+id+"/accept",
		acceptBody(card.PayloadHash, dlvPIN)); code != http.StatusForbidden {
		t.Errorf("another member accepting alice's deliverable: want 403, got %d: %s", code, out)
	}
	// The operator can READ the card (visibility) and still cannot accept it.
	if code, _ := e.do(t, "op", "GET", "/api/deliverables/"+id+"/accept-card", ""); code != http.StatusOK {
		t.Error("the operator reads across owners")
	}
	if code, out := e.do(t, "op", "POST", "/api/deliverables/"+id+"/accept",
		acceptBody(card.PayloadHash, dlvPIN)); code != http.StatusForbidden {
		t.Errorf("the operator must not accept another owner's deliverable: want 403, got %d: %s", code, out)
	}
	if code, _ := e.do(t, "bob", "POST", "/api/deliverables/d-nope/accept", acceptBody("x", dlvPIN)); code != http.StatusNotFound {
		t.Error("an unknown deliverable must 404 before it 403s")
	}
	if len(e.push.reqs) != 0 {
		t.Error("no refused accept may fire anything")
	}
	// The non-tautological control: the OWNER's own accept succeeds.
	if code, out := e.do(t, "alice", "POST", "/api/deliverables/"+id+"/accept",
		acceptBody(card.PayloadHash, dlvPIN)); code != http.StatusOK {
		t.Fatalf("the owner's own accept must succeed: %d %s", code, out)
	}
}

// ── R15/R24/R26: the negatives ──────────────────────────────────────────────

// TestAPIReachesTheOutwardWorldOnlyThroughTheAccepter is the import scan behind
// R15: internal/api cannot touch the pusher or the git topology, so there is
// exactly one way out of the platform and it is the journal-mediated accept.
func TestAPIReachesTheOutwardWorldOnlyThroughTheAccepter(t *testing.T) {
	forbidden := []string{"internal/broker", "internal/project"}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	scanned := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		scanned++
		for _, imp := range parsed.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, f := range forbidden {
				if strings.Contains(path, f) {
					t.Errorf("%s imports %q — the accept exits through accept.Accepter and nothing else (D7; §24)", name, path)
				}
			}
		}
	}
	if scanned == 0 {
		t.Fatal("scanned no files — the import scan proves nothing")
	}
	// The probe: the scan must be able to SEE an import when one is present.
	if !strings.Contains(`"github.com/x/internal/broker"`, forbidden[0]) {
		t.Fatal("the import scan cannot detect its own probe")
	}
}

// TestAcceptMintsOnlyItsLandedCanonicalRows is the R26 mutation walk for the
// accept: the canonical rows land and ZERO decision.recorded rows are co-minted
// — an act with a landed canonical row is never double-minted (§39 OQ8).
func TestAcceptMintsOnlyItsLandedCanonicalRows(t *testing.T) {
	e := newDlvEnv(t)
	id := acceptFixture(t, e)
	card := readAcceptCard(t, e, "alice", id)
	e.mustDo(t, "alice", "POST", "/api/deliverables/"+id+"/accept", acceptBody(card.PayloadHash, dlvPIN))

	if got := e.count("deliverable.accepted"); got != 1 {
		t.Errorf("want one deliverable.accepted row, got %d", got)
	}
	if got := e.count(api.EventDecisionRecorded); got != 0 {
		t.Errorf("the accept co-minted %d decision.recorded rows — its canonical row already exists (§39 OQ8)", got)
	}
	// The journal row is the effect record; nothing else records the push.
	var effects int
	if err := e.b.db.QueryRowContext(e.ctx, `SELECT COUNT(*) FROM effects WHERE class = 'A'`).Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if effects != 1 {
		t.Errorf("want exactly one class-A accept effect, got %d", effects)
	}
}

// TestOnlyOneCanonicalizationHashesAnswerablePins is the R25 source scan: the
// pin the accept quotes back is hashed by gates.CanonicalHash and by nothing
// else. A second canonicalization would be a second answer to "is this the same
// payload?", and the whole retry-safety rule rests on there being one.
// hashingFilesThatArePins enumerates the non-test files of internal/api that
// hash bytes themselves, each with the reason it is NOT a payload pin. The
// enumeration is honest in both directions (the redactionEdgeFiles precedent):
// a new hasher must be declared here, and a declared one that stopped hashing
// must be removed.
var hashingFilesThatArePins = map[string]string{
	"projection.go": "the S14.3 arg FINGERPRINT (§30): a short digest served INSTEAD of raw args, so a reader can tell two invocations apart " +
		"without the args themselves reaching the wire. It pins no answer and nothing is ever compared against it.",
}

func TestOnlyOneCanonicalizationHashesAnswerablePins(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// A SECOND canonicalizer is banned outright: canonical JSON is where "is
	// this the same payload?" is decided, and there is one of those.
	banned := []string{"canonicalJSON(", "hashPayload("}
	scanned, sawCanonical := 0, false
	hashers := map[string]bool{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src := stripGoComments(readSourceFile(t, name))
		scanned++
		if strings.Contains(src, "gates.CanonicalHash(") {
			sawCanonical = true
		}
		for _, b := range banned {
			if strings.Contains(src, b) {
				t.Errorf("%s calls %q — gates.CanonicalHash is the ONE canonicalization (§39)", name, b)
			}
		}
		if strings.Contains(src, "sha256.") {
			hashers[name] = true
			if _, declared := hashingFilesThatArePins[name]; !declared {
				t.Errorf("%s hashes bytes itself and is not a declared non-pin hasher — a second pin hasher is exactly what §39 forbids", name)
			}
		}
		// Every file that PRODUCES a payload pin must reach it through the one
		// canonicalization.
		if strings.Contains(src, "PayloadHash:") && !strings.Contains(src, "gates.CanonicalHash(") &&
			!strings.Contains(src, "PayloadHash: hash") {
			t.Errorf("%s builds a PayloadHash without gates.CanonicalHash", name)
		}
	}
	for name := range hashingFilesThatArePins {
		if !hashers[name] {
			t.Errorf("%s is declared a non-pin hasher but no longer hashes — the enumeration must stay honest in both directions", name)
		}
	}
	if scanned == 0 {
		t.Fatal("scanned no files — the scan proves nothing")
	}
	if !sawCanonical {
		t.Fatal("no file calls gates.CanonicalHash — the scan would pass for the wrong reason")
	}
}

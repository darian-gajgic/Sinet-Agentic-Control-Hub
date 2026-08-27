package api_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
)

// gf11accept_test.go — P3-GF11 acceptance specs, committed RED at grounding
// (the Amendment-A carve-out; the window is closed by the GF11 implementation
// commit). Brief: P3/briefs/P3-GF11.md. Walk evidence:
// P3/design/e5-rewalk-findings-2026-08-27.md — POST /accept answered
// 500 `accept: broker push: broker: push with no remote` three-for-three on a
// New-project-door project ("no remote — local store only"), and the raw
// internal chain reached the wire as the 500's detail.
//
// THE FIXTURE GAP THIS FILE CLOSES. Every landed accept fixture onboards with
// a file:// remote (prepareProject), and fakePusher OKs ANY request — including
// Remote "" — so the api suite as it stands would happily green an accept the
// REAL broker refuses (internal/broker/git.go:65-67). The GF10 §2 lesson
// (mkRun's one-run shape) repeated one seam over. These tests pin the door's
// own shape: a fresh init store with remote_url "".

// prepareLocalOnlyProject onboards + approves a project the way the New-project
// door does: no clone source (fresh init store) and remote_url "" (the walker's
// "decide later" — internal/shell/project_seams.go:163-166).
func prepareLocalOnlyProject(t *testing.T, e *dlvEnv, projectID, owner string) {
	t.Helper()
	if _, _, err := e.proj.Onboard(e.ctx, project.OnboardInput{
		ProjectID: projectID, Owner: owner, Name: projectID, Source: "", RemoteURL: "",
	}); err != nil {
		t.Fatalf("Onboard (local-only): %v", err)
	}
	if _, err := e.proj.Approve(e.ctx, projectID, owner, nil); err != nil {
		t.Fatalf("Approve: %v", err)
	}
}

// TestGF11LocalOnlyAcceptComposesNoPushRequest is E5-REWALK errand 5 at the
// wire (GF11 checklist item 3): on a local-only store the accept LANDS — 200,
// applied:true, the attributed commit on the store's default branch — and NO
// broker push request is ever composed, because no push exists for a store the
// registry records no remote for (brief R1). The request that today reaches the
// broker with Remote "" is exactly what earns `broker: push with no remote`
// (git.go:66) and the walk's three 500s; this fixture's fake absorbs the
// malformed request instead of refusing it, so the load-bearing red assertion
// is that the request is never BUILT.
//
// RED until GF11 lands: today one push request with Remote "" is composed.
func TestGF11LocalOnlyAcceptComposesNoPushRequest(t *testing.T) {
	e := newDlvEnv(t)
	prepareLocalOnlyProject(t, e, "proj-local", "alice")
	snap := e.snapshotCandidate("proj-local", "pipe", "note.md", "Thank you for your orders.\n")
	e.mkRun("t-l", "r-l", "alice")
	e.mkDeliverable("d-l", "alice", "t-l", "r-l", "proj-local", "markdown",
		map[string]string{"note.md": "Thank you for your orders.\n"}, snap)

	// Control, green today (the walk's own door-state): the card is honestly
	// OPEN — the act is legitimate, which is why a 500 was a crash-shaped
	// answer and never a refusal.
	card := readAcceptCard(t, e, "alice", "d-l")
	if !card.Acceptable {
		t.Fatalf("a local-only in-review deliverable with resolvable attribution must be acceptable: %+v", card)
	}

	code, out := e.do(t, "alice", "POST", "/api/deliverables/d-l/accept", acceptBody(card.PayloadHash, dlvPIN))
	if code != http.StatusOK {
		t.Fatalf("the accept on a local-only store must answer 200 (the E5-REWALK settle), got %d: %s", code, out)
	}
	// THE RED PIN: zero push requests for a store with no remote (brief R1).
	if n := len(e.push.reqs); n != 0 {
		t.Errorf("a local-only accept composed %d broker push request(s); the local commit is the landing — no push exists to request (brief R1; git.go:66 is downstream of exactly this request)", n)
	}

	var res api.AcceptOutcome
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode accept outcome: %v", err)
	}
	if !res.Applied || res.State != review.StateAccepted || res.Commit == "" {
		t.Fatalf("the local landing must apply with a commit: %+v", res)
	}
	// The official copy is on the store's default branch.
	entry, err := e.proj.Get(e.ctx, "proj-local")
	if err != nil {
		t.Fatal(err)
	}
	if head := e.git("-C", entry.StorePath, "rev-parse", "refs/heads/main"); head != res.Commit {
		t.Errorf("the store's default branch must hold the served commit: head %s, commit %s", head, res.Commit)
	}
	// The landing is journaled (brief R2) — the retry read-back derives the
	// commit from the succeeded effect row.
	if res.EffectID == "" {
		t.Errorf("the local landing must journal the accept effect (brief R2)")
	} else if eff, err := e.journal.Get(e.ctx, res.EffectID); err != nil || eff.State != gates.EffectSucceeded {
		t.Errorf("the landed effect must read back succeeded: state %v err %v", eff.State, err)
	}
	// Arm-accurate success copy (brief R4): the answer must not claim a CAS
	// push that never happened.
	if strings.Contains(out, "broker's CAS push") {
		t.Errorf("the local landing's detail claims a push that did not happen: %s", out)
	}
}

// TestGF11CardSaysWhereTheOfficialCopyLands pins brief R4 (checklist item 4):
// the accept card SAYS where the official copy lands, as data — `landing`:
// "local-store" for a remote-less repo arm, "remote-push" where a remote
// exists — with arm-accurate copy: the local card's open reason names the
// project's local store, and its tier statement keeps tier High (the RW-17
// uniformity reading) without claiming a push that will not happen. Decoded
// raw: the typed AcceptCard gains the member only at implementation.
//
// RED until GF11 lands: no `landing` member exists, and the local card's copy
// claims "this pushes a commit to a shared branch".
func TestGF11CardSaysWhereTheOfficialCopyLands(t *testing.T) {
	e := newDlvEnv(t)
	prepareLocalOnlyProject(t, e, "proj-local", "alice")
	snapL := e.snapshotCandidate("proj-local", "pipe", "note.md", "local body\n")
	e.mkRun("t-l", "r-l", "alice")
	e.mkDeliverable("d-l", "alice", "t-l", "r-l", "proj-local", "markdown",
		map[string]string{"note.md": "local body\n"}, snapL)
	e.prepareProject("proj-rem", "alice", map[string]string{"a.txt": "base\n"})
	snapR := e.snapshotCandidate("proj-rem", "pipe-r", "a.txt", "candidate\n")
	e.mkRun("t-r", "r-r", "alice")
	e.mkDeliverable("d-r", "alice", "t-r", "r-r", "proj-rem", "markdown",
		map[string]string{"a.txt": "candidate\n"}, snapR)

	readRaw := func(id string) map[string]any {
		var raw map[string]any
		if err := json.Unmarshal([]byte(e.mustDo(t, "alice", "GET", "/api/deliverables/"+id+"/accept-card", "")), &raw); err != nil {
			t.Fatalf("decode card %s: %v", id, err)
		}
		return raw
	}

	local := readRaw("d-l")
	if got, _ := local["landing"].(string); got != "local-store" {
		t.Errorf("the local-only card must say where the official copy lands: landing %q, want %q (brief R4)", got, "local-store")
	}
	if reason, _ := local["reason"].(string); !strings.Contains(reason, "local store") {
		t.Errorf("the local-only open reason must name the project's local store in plain words, got %q", reason)
	}
	if ts, _ := local["tier_statement"].(string); strings.Contains(ts, "pushes a commit") {
		t.Errorf("the local-only tier statement claims a push that will not happen: %q", ts)
	}
	// Control, green today: one accept family, one ceremony — the tier stays
	// "high" (the landed wire vocabulary; RW-17's uniformity reading — relaxing
	// it later is operator-visible, never silent).
	if tier, _ := local["tier"].(string); tier != "high" {
		t.Errorf("the local-only accept keeps the High ceremony, got tier %q", tier)
	}

	remoted := readRaw("d-r")
	if got, _ := remoted["landing"].(string); got != "remote-push" {
		t.Errorf("the remoted card names its push landing: landing %q, want %q", got, "remote-push")
	}
}

// TestGF11PushFailureIsServedNotACrash pins brief R5 (checklist item 5): on a
// REMOTED store, a push the broker could not perform is a served, plain-words
// answer — 502 `push_failed`, the deliverable honestly still in review and
// retryable — never a 500 whose detail is the raw internal error chain. The
// walk's E2: the 500 banner quoted `accept: broker push: broker: …` verbatim
// on the wire, which is the §38 posture bypassed at this seam.
//
// RED until GF11 lands: today this answers 500 `internal` with the raw chain
// as its detail.
func TestGF11PushFailureIsServedNotACrash(t *testing.T) {
	e := newDlvEnv(t)
	id := acceptFixture(t, e)
	card := readAcceptCard(t, e, "alice", id)
	// The shape broker.Client.Push returns for a refusal or transport failure
	// (client.go's !OK collapse) — landed tests never set this.
	e.push.pushErr = errors.New("broker: push failed")

	code, out := e.do(t, "alice", "POST", "/api/deliverables/"+id+"/accept", acceptBody(card.PayloadHash, dlvPIN))
	if code != http.StatusBadGateway {
		t.Errorf("a push the broker could not perform is a served answer: want 502, got %d: %s", code, out)
	}
	var body struct {
		Error  string `json:"error"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode refusal: %v", err)
	}
	if body.Error != "push_failed" {
		t.Errorf("the refusal carries a machine code: want %q, got %q", "push_failed", body.Error)
	}
	if strings.Contains(out, "accept: broker push:") {
		t.Errorf("the raw internal error chain reached the wire (§38): %s", out)
	}
	// Honest state, green on both sides of the fix: nothing was accepted and
	// the act stays retryable.
	d, err := e.rev.Deliverable(e.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if d.State != review.StateInReview {
		t.Errorf("a failed push must leave the deliverable in review, got %s", d.State)
	}
}

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// gf15accept_test.go — P3-GF15 acceptance specs, committed RED at grounding
// (the Amendment-A carve-out; the window closes at the GF15 implementation
// commit). Brief: P3/briefs/P3-GF15.md. Walk evidence:
// P3/design/exitwalk-findings-2026-08-31.md E7/F3 — with the world's broker
// not running, POST /accept answered 500 `internal` with the raw chain
// `accept: resolve signing posture: broker: dial unix
// /home/sinep/.sinet-exitwalk/broker/sinep.sock: connect: no such file or
// directory` as the detail, on the requester surface. Loud and fail-closed —
// the correct posture — but a Unix socket path is not an operator sentence
// (CONVENTIONS §38; the GF11 `acceptErr`/`acceptPushFailedMsg` precedent
// humanized exactly this class for the remote push at this same door).
//
// RED today, per limb: T1 status/code (500 `internal`, not 503
// `broker_unreachable`), T1 wire hygiene (the raw dial chain is the detail),
// T1 journal truth (the effect row is left `executing` — in-doubt residue the
// boot reconcile would ghost-re-drive); T2 status/code (the push-arm dial
// failure is answered with the REMOTE's 502 `push_failed`) and T2's wanted
// words; T3 status/code + wire hygiene (the mid-squash signing failure is a
// raw 500). Every world-truth limb that is already honest today (still
// in-review, branch unmoved, retry lands) is asserted as the non-tautological
// control so the fix cannot trade fail-closed for fail-locked.

// gf15BannedOnTheWire are the raw-internals tokens CONVENTIONS §38 keeps off
// the requester surface: the socket dialect, the dial verb, the errno prose
// and the internal call-chain prefix the walk read verbatim.
var gf15BannedOnTheWire = []string{
	"unix", ".sock", "dial", "no such file", "resolve signing posture", "broker:",
}

// gf15WantedWords are the walker's own asked-for sentence, reduced to its
// load-bearing tokens (E7/F3: "a helper the platform needs for signing is not
// running on this machine" — and NOTHING happened, the work is not accepted).
// Case-insensitive; the exact prose is the implementation's.
var gf15WantedWords = []string{"sign", "not running", "not accepted"}

// gf15Accepter swaps the env's accepter for one whose broker seams are the
// caller's — the shell's buildAcceptSurface shape
// (internal/shell/accept_seams.go:70-87) with the dial destination under test
// control, over the same production stores newDlvEnv composes.
func gf15Accepter(t *testing.T, e *dlvEnv, posture accept.SigningPosture, signer accept.Signer) {
	t.Helper()
	acc, err := accept.New(accept.Config{
		Project: e.proj, Journal: e.journal, Push: e.push, Review: e.rev,
		Signer: signer, SigningPosture: posture,
		ActiveRuns: func(context.Context, string) ([]run.SiblingAcceptRun, error) {
			return []run.SiblingAcceptRun{{RunID: "r-sibling", CheckpointTime: time.Unix(1, 0)}}, nil
		},
		Freshness: e.b.reg,
	})
	if err != nil {
		t.Fatalf("accept.New: %v", err)
	}
	e.acc = acc
}

// gf15Refusal decodes the transport's {error, detail} answer.
type gf15Refusal struct {
	Error  string `json:"error"`
	Detail string `json:"detail"`
}

// gf15AssertPlainRefusal is the shared wire contract (brief R4/R5): 503,
// code broker_unreachable, no raw internals, the walker's wanted words.
func gf15AssertPlainRefusal(t *testing.T, code int, out string) {
	t.Helper()
	if code != http.StatusServiceUnavailable {
		t.Errorf("a platform helper not running answers 503 — a named component is down, the platform did not break unexpectedly (S01.3 honest status, CONVENTIONS §56; the not_wired precedent) — got %d: %s", code, out)
	}
	var refusal gf15Refusal
	if err := json.Unmarshal([]byte(out), &refusal); err != nil {
		t.Fatalf("decode refusal body: %v: %s", err, out)
	}
	if refusal.Error != "broker_unreachable" {
		t.Errorf("the refusal's code must name THIS class — broker_unreachable, distinct from push_failed (the remote) and internal (a genuine fault) — got %q", refusal.Error)
	}
	for _, raw := range gf15BannedOnTheWire {
		if strings.Contains(out, raw) {
			t.Errorf("raw internals reached the requester surface (§38): the body contains %q: %s", raw, out)
		}
	}
	low := strings.ToLower(out)
	for _, want := range gf15WantedWords {
		if !strings.Contains(low, want) {
			t.Errorf("the refusal must say what it means in the requester's words — missing %q: %s", want, out)
		}
	}
}

// gf15AssertNothingHappened is the shared journal/world contract (brief R3):
// the deliverable is still in review AND the effect journal holds neither an
// `executing` (in-doubt) nor an `approved` residue row — both are fed to the
// boot reconcile (ReconcileInDoubt / ReDriveApproved), which would re-drive
// the accept behind the requester's back after they were told nothing
// happened. "NOTHING happened" has to be the journal's truth, not just the
// wire's.
func gf15AssertNothingHappened(t *testing.T, e *dlvEnv, deliverableID string) {
	t.Helper()
	d, err := e.rev.Deliverable(e.ctx, deliverableID)
	if err != nil {
		t.Fatalf("re-read deliverable: %v", err)
	}
	if d.State != review.StateInReview {
		t.Errorf("the work must still be in review — the refusal said nothing was accepted — got %s", d.State)
	}
	for _, st := range []gates.EffectState{gates.EffectExecuting, gates.EffectApproved} {
		rows, err := e.journal.InState(e.ctx, st)
		if err != nil {
			t.Fatalf("journal.InState(%s): %v", st, err)
		}
		if len(rows) != 0 {
			t.Errorf("a broker-down accept left %d %s effect row(s) — ghost re-drive residue the served 'nothing happened' contradicts (S02.7; the push-arm failEffect precedent)", len(rows), st)
		}
	}
}

// TestGF15BrokerDownAcceptServesAPlainRefusal is E7/F3 at the wire, on the
// walk's own shape: a local-only project (nothing is ever sent anywhere), an
// open legitimate accept, and no broker daemon running. The accept must
// refuse in plain words with an appropriate status class, leave the world
// exactly as it was, and land cleanly on the SAME click once the broker is
// back — the walk's own arc.
func TestGF15BrokerDownAcceptServesAPlainRefusal(t *testing.T) {
	e := newDlvEnv(t)
	prepareLocalOnlyProject(t, e, "proj-gone", "alice")
	snap := e.snapshotCandidate("proj-gone", "pipe", "note.md", "Thank you for your orders.\n")
	e.mkRun("t-g", "r-g", "alice")
	e.mkDeliverable("d-g", "alice", "t-g", "r-g", "proj-gone", "markdown",
		map[string]string{"note.md": "Thank you for your orders.\n"}, snap)

	// No daemon ever listens here — the real client against the walk's world.
	sock := filepath.Join(t.TempDir(), "broker", "alice.sock")
	brokerUp := false
	gf15Accepter(t, e, func(_ context.Context, user string) (bool, string, error) {
		profile := user + "-git"
		if brokerUp {
			// The broker reachable again with no git-ssh-key placed — the
			// exit walk's bare control world (unsigned commits).
			return false, profile, nil
		}
		c, err := broker.Dial(sock)
		if err != nil {
			return false, "", err
		}
		defer c.Close()
		kind, has, err := c.HasKey(profile)
		return has && kind == broker.KindGitSSHKey, profile, err
	}, nil)

	// Control, green today: the act itself is legitimate and OPEN — which is
	// what makes a crash-shaped answer wrong (the card never dials the broker
	// and honestly says it does not claim the signing posture either way).
	card := readAcceptCard(t, e, "alice", "d-g")
	if !card.Acceptable {
		t.Fatalf("an in-review local-only deliverable with resolvable attribution is acceptable: %+v", card)
	}
	entry, err := e.proj.Get(e.ctx, "proj-gone")
	if err != nil {
		t.Fatal(err)
	}
	headBefore := e.git("-C", entry.StorePath, "rev-parse", "refs/heads/main")

	code, out := e.do(t, "alice", "POST", "/api/deliverables/d-g/accept", acceptBody(card.PayloadHash, dlvPIN))
	gf15AssertPlainRefusal(t, code, out)
	gf15AssertNothingHappened(t, e, "d-g")
	if head := e.git("-C", entry.StorePath, "rev-parse", "refs/heads/main"); head != headBefore {
		t.Errorf("the store's default branch moved under a refused accept: %s -> %s", headBefore, head)
	}

	// Fail-closed, never fail-locked: the broker comes back, the SAME click
	// (same card, same payload hash) lands — the walk's own retry.
	brokerUp = true
	code2, out2 := e.do(t, "alice", "POST", "/api/deliverables/d-g/accept", acceptBody(card.PayloadHash, dlvPIN))
	if code2 != http.StatusOK {
		t.Fatalf("with the broker back the same accept must land, got %d: %s", code2, out2)
	}
	var res api.AcceptOutcome
	if err := json.Unmarshal([]byte(out2), &res); err != nil {
		t.Fatalf("decode accept outcome: %v", err)
	}
	if !res.Applied || res.State != review.StateAccepted || res.Commit == "" {
		t.Fatalf("the retried accept must apply with a commit: %+v", res)
	}
	if head := e.git("-C", entry.StorePath, "rev-parse", "refs/heads/main"); head != res.Commit {
		t.Errorf("the store's default branch must hold the retried commit: head %s, commit %s", head, res.Commit)
	}
}

// TestGF15PushArmBrokerDownNamesTheHelperNotTheRemote pins the arm ordering
// (brief R4): a push the broker never PERFORMED because the broker itself is
// unreachable is this packet's class — 503 broker_unreachable — and must not
// fall through to the GF11-era 502 push_failed, whose served sentence blames
// the project's remote ("once the remote is reachable") for a helper that is
// down on this machine. A push the REACHABLE broker could not perform keeps
// the landed 502 untouched.
func TestGF15PushArmBrokerDownNamesTheHelperNotTheRemote(t *testing.T) {
	e := newDlvEnv(t)
	id := acceptFixture(t, e)
	// The production seam (internal/shell/accept_seams.go:52-62 dialPusher)
	// returns broker.Dial's error unchanged; mint the real one.
	_, dialErr := broker.Dial(filepath.Join(t.TempDir(), "gone", "op.sock"))
	if dialErr == nil {
		t.Fatal("dialing a nonexistent socket must fail")
	}
	e.push.pushErr = dialErr

	card := readAcceptCard(t, e, "alice", id)
	if !card.Acceptable {
		t.Fatalf("the act is open: %+v", card)
	}
	code, out := e.do(t, "alice", "POST", "/api/deliverables/"+id+"/accept", acceptBody(card.PayloadHash, dlvPIN))
	gf15AssertPlainRefusal(t, code, out)
	gf15AssertNothingHappened(t, e, id)

	// Control: the broker back, the same click lands (the push arm's own).
	e.push.pushErr = nil
	code2, out2 := e.do(t, "alice", "POST", "/api/deliverables/"+id+"/accept", acceptBody(card.PayloadHash, dlvPIN))
	if code2 != http.StatusOK {
		t.Fatalf("with the broker back the same accept must land, got %d: %s", code2, out2)
	}
	var res api.AcceptOutcome
	if err := json.Unmarshal([]byte(out2), &res); err != nil {
		t.Fatalf("decode accept outcome: %v", err)
	}
	if !res.Applied || res.Commit == "" {
		t.Fatalf("the retried accept must apply with a commit: %+v", res)
	}
}

// TestGF15BrokerDyingAtTheSigningStepStaysAPlainRefusal covers the third
// broker touch on this one door: a SIGNING user whose broker dies between the
// posture read and the SignData call. The failure surfaces mid-squash
// (project: sign accept commit), the effect is failed (green today), and the
// wire must serve the same plain refusal instead of the raw chain.
func TestGF15BrokerDyingAtTheSigningStepStaysAPlainRefusal(t *testing.T) {
	e := newDlvEnv(t)
	prepareLocalOnlyProject(t, e, "proj-sig", "alice")
	snap := e.snapshotCandidate("proj-sig", "pipe", "note.md", "Signed work.\n")
	e.mkRun("t-s", "r-s", "alice")
	e.mkDeliverable("d-s", "alice", "t-s", "r-s", "proj-sig", "markdown",
		map[string]string{"note.md": "Signed work.\n"}, snap)

	_, dialErr := broker.Dial(filepath.Join(t.TempDir(), "gone", "alice.sock"))
	if dialErr == nil {
		t.Fatal("dialing a nonexistent socket must fail")
	}
	gf15Accepter(t, e,
		func(_ context.Context, user string) (bool, string, error) { return true, user + "-git", nil },
		func(profile, namespace string, data []byte) ([]byte, error) { return nil, dialErr })

	card := readAcceptCard(t, e, "alice", "d-s")
	if !card.Acceptable {
		t.Fatalf("the act is open: %+v", card)
	}
	code, out := e.do(t, "alice", "POST", "/api/deliverables/d-s/accept", acceptBody(card.PayloadHash, dlvPIN))
	gf15AssertPlainRefusal(t, code, out)
	gf15AssertNothingHappened(t, e, "d-s")
}

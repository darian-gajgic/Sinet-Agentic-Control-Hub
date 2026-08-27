package api_test

import (
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
)

// gf11landing_test.go — the executor-owned api properties of P3-GF11: the third
// value of the `landing` vocabulary, and the fixture-faithfulness rule itself.

// TestGF11PinnedArmLandsADecisionRecord completes the R4/OQ2 vocabulary. The
// RW-17 pinned arm makes no commit anywhere, so its landing is neither a push
// nor a store: the durable record is the decision against the revision's
// immutable content pin. The card says which of the three acts the person is
// about to make, and its tier statement and reason stay exactly as RW-17 left
// them — GF11 adds a name for this arm, it does not re-open it.
func TestGF11PinnedArmLandsADecisionRecord(t *testing.T) {
	e := newDlvEnv(t)
	card := readAcceptCard(t, e, "alice", pinnedFixture(t, e))

	if card.Landing != api.LandingDecisionRecord {
		t.Errorf("the pinned arm's landing is %q, want %q", card.Landing, api.LandingDecisionRecord)
	}
	// No project, no store to land in: the card must not borrow either repo
	// arm's target.
	if card.ProtectedRef != "" {
		t.Errorf("the pinned arm named a protected ref: %q", card.ProtectedRef)
	}
	if !card.Acceptable {
		t.Errorf("naming the landing must not close the act: %+v", card)
	}
}

// TestGF11FakePusherRefusesAnEmptyRemote pins brief R6 as a property of THIS
// suite rather than a comment in it. The api tests drive a fake broker, and the
// walk's three 500s happened while every one of them passed: the fake OK'd a
// push request with no remote that the real broker refuses before touching git
// (internal/broker/git.go), so production's answer and the suite's answer had
// drifted apart on the exact shape the New-project door mints.
//
// The refusal is the fake's DEFAULT, not a per-test opt-in, so the gap cannot
// re-open one test at a time. If this assertion ever fails, the fixture has gone
// back to lying and every accept assertion in the package is worth less.
func TestGF11FakePusherRefusesAnEmptyRemote(t *testing.T) {
	p := &fakePusher{}
	if _, err := p.Push(broker.Request{RepoDir: "/tmp/x", Remote: "",
		Refs: []broker.RefUpdate{{Ref: "refs/heads/main", ExpectSHA: "a", SrcSHA: "b"}}}); err == nil {
		t.Fatal("the api suite's pusher accepted a push request with no remote — the real broker refuses it (brief R6)")
	}
	// A remoted request still goes through, so the fixture refuses the malformed
	// shape and nothing else.
	if _, err := p.Push(broker.Request{RepoDir: "/tmp/x", Remote: "file:///tmp/r.git",
		Refs: []broker.RefUpdate{{Ref: "refs/heads/main", ExpectSHA: "a", SrcSHA: "b"}}}); err != nil {
		t.Fatalf("the fixture refused a well-formed push: %v", err)
	}
}

package opencode

// answer_ln2_test.go — LN-2A: the shared adapters.Answer decision member
// replaces LN-1's smuggled `sinet.decision` envelope. Every guarantee the
// envelope carried has to survive the migration verbatim.

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// ── spec 28 · reject is first-class, and the envelope is gone ────────────────

func TestRejectIsFirstClassOnOpenCode(t *testing.T) {
	ans := RejectAnswer(fixtureAskID, "not allowed here")
	if ans.Decision != adapters.DecisionReject {
		t.Errorf("RejectAnswer decision = %q, want %q", ans.Decision, adapters.DecisionReject)
	}
	if ans.Reason != "not allowed here" {
		t.Errorf("RejectAnswer reason = %q", ans.Reason)
	}
	if len(ans.UpdatedInput) != 0 {
		t.Errorf("RejectAnswer still rides UpdatedInput: %s", ans.UpdatedInput)
	}
	if ApproveAnswer(fixtureAskID).Decision != adapters.DecisionApprove {
		t.Error("ApproveAnswer does not carry the approve (zero) decision")
	}

	// The wire body the engine actually receives.
	f := newFakeServe(t)
	f.setPermissions(fixtureAsk())
	a := fakeAdapter(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sess, err := a.Resume(ctx, parkRecordFixture(t, a), RejectAnswer(fixtureAskID, "not allowed here"))
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	f.publishFixture("happy.sse")
	for range sess.Events() {
	}
	replies := f.repliesSeen()
	if len(replies) != 1 {
		t.Fatalf("replies = %d, want 1", len(replies))
	}
	if replies[0].Reply != replyReject || replies[0].Message != "not allowed here" {
		t.Errorf("reply = %+v, want reject-with-feedback (S03.4 CorrectedError)", replies[0])
	}
	for _, r := range f.requests() {
		if strings.Contains(r.Body, "sinet.decision") {
			t.Errorf("the retired decision envelope reached the wire: %s", r.Body)
		}
	}
	for name, src := range packageSources(t) {
		if strings.Contains(src, "sinet.decision") {
			t.Errorf("%s still carries the retired sinet.decision envelope", name)
		}
	}
}

// ── spec 29 · `always` stays unsendable, as a property ───────────────────────

func TestAlwaysStillUnsendableAfterMigration(t *testing.T) {
	rng := rand.New(rand.NewSource(20260823))
	for i := 0; i < 500; i++ {
		ans := &adapters.Answer{AskID: fixtureAskID}
		switch rng.Intn(4) {
		case 0:
			ans.Decision = adapters.DecisionApprove
		case 1:
			ans.Decision = adapters.DecisionReject
			ans.Reason = pickOne(rng, "always", `{"reply":"always"}`, "always always", "")
		case 2:
			ans.Decision = adapters.AnswerDecision(pickOne(rng, "always", "allow", "ALWAYS", "approve"))
		case 3:
			ans.Decision = adapters.DecisionReject
			ans.Reason = "no"
			ans.UpdatedInput = json.RawMessage(`{"command":"echo x"}`)
		}
		reply, _, err := decodeDecision(ans)
		if err != nil {
			continue
		}
		if reply != replyOnce && reply != replyReject {
			t.Fatalf("decodeDecision emitted %q — only %q/%q may ever go out (S03.4)", reply, replyOnce, replyReject)
		}
	}
}

// ── spec 30 · a raw replacement input is still refused ───────────────────────

func TestRawUpdatedInputStillRefusedOnOpenCode(t *testing.T) {
	for name, ans := range map[string]*adapters.Answer{
		"approve with edited input": {AskID: fixtureAskID, UpdatedInput: json.RawMessage(`{"command":"echo EDITED"}`)},
		"reject with edited input": {AskID: fixtureAskID, Decision: adapters.DecisionReject, Reason: "no",
			UpdatedInput: json.RawMessage(`{"command":"echo EDITED"}`)},
	} {
		if _, _, err := decodeDecision(ans); !errors.Is(err, ErrUpdatedInputUnsupported) {
			t.Errorf("%s: err = %v, want ErrUpdatedInputUnsupported — this substrate cannot substitute a "+
				"gated call's input, and dropping the edit would execute a call the human did not approve", name, err)
		}
	}
}

// ── spec 31 · an inexpressible decision fails CLOSED ─────────────────────────

func TestInexpressibleDecisionFailsClosed(t *testing.T) {
	for _, decision := range []adapters.AnswerDecision{"deny", "escalate", "always", "APPROVE", "reject "} {
		reply, _, err := decodeDecision(&adapters.Answer{AskID: fixtureAskID, Decision: decision})
		if err == nil {
			t.Errorf("decision %q decoded to reply %q — a verdict this substrate cannot express became consent", decision, reply)
		}
		if reply == replyOnce {
			t.Errorf("decision %q degraded to an approve", decision)
		}
	}
	// The two expressible verdicts still decode.
	if reply, msg, err := decodeDecision(ApproveAnswer(fixtureAskID)); err != nil || reply != replyOnce || msg != "" {
		t.Errorf("approve = (%q, %q, %v)", reply, msg, err)
	}
	if reply, msg, err := decodeDecision(RejectAnswer(fixtureAskID, "nope")); err != nil || reply != replyReject || msg != "nope" {
		t.Errorf("reject = (%q, %q, %v)", reply, msg, err)
	}
}

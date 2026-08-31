package api_test

// gf14doors_test.go — P3-GF14 drain r1 F8 (GF9 review L10): the compare door's
// reason must say what the compare will actually SHOW. It said "With nothing
// accepted yet to compare against…" on every deliverable, accepted ones
// included — and the committed fixture proved it, because that fixture's
// deliverable IS accepted. A door may summarize its authority; it may never
// contradict it.

import (
	"strings"
	"testing"
)

func TestGF14CompareDoorReasonFollowsTheAcceptState(t *testing.T) {
	e := newDlvEnv(t)
	e.mkRun("t-cmp", "r-cmp", "alice")
	e.mkDeliverable("d-cmp", "alice", "t-cmp", "r-cmp", "", "markdown", map[string]string{"a.md": "one\n"}, "")

	// Before any accept the single-instance sentence is TRUE: LaunchComparison
	// has no accepted revision to pair against.
	body := e.mustDo(t, "alice", "GET", "/api/deliverables/d-cmp", "")
	before := doorByVerb(t, body, "preview-compare")
	if !strings.Contains(before.Reason, "nothing accepted yet") {
		t.Fatalf("with nothing accepted, the door must say so: %q", before.Reason)
	}

	if _, err := e.rev.Accept(e.ctx, "d-cmp", "alice"); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	e.mintNext("d-cmp", "r-cmp", 2, map[string]string{"a.md": "two\n"}, "")

	body = e.mustDo(t, "alice", "GET", "/api/deliverables/d-cmp", "")
	after := doorByVerb(t, body, "preview-compare")
	if strings.Contains(after.Reason, "nothing accepted yet") {
		t.Errorf("the door still claims nothing is accepted after an accept (GF9 review L10): %q", after.Reason)
	}
	if !strings.Contains(after.Reason, "you accepted") {
		t.Errorf("the door must say what the pair will show: %q", after.Reason)
	}
	for _, token := range []string{"S13", "§", "Spec/", "revision "} {
		if strings.Contains(after.Reason, token) {
			t.Errorf("the served reason carries the platform's own vocabulary (%q): %q", token, after.Reason)
		}
	}
}

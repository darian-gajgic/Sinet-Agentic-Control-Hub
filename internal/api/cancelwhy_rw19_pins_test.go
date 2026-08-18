package api

// cancelwhy_rw19_pins_test.go — P3-RW-19 R11, the api end of the constructor
// contract, extending cancelliterals_rw18_test.go's posture with the packet's
// new key. Committed RED before the implementation (CONVENTIONS §3).
//
// internal/api imports no producer package in production, so the decisions
// derive reads the human reason by JSON PATH — a literal with a comment. This
// is what makes it a contract: the one constructor must actually write a
// `reason` member, and must omit it when there is none, or the served
// `human_reason` would either go quiet or claim a motive nobody gave.

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

func TestTheCancelDetailConstructorWritesTheHumanReason(t *testing.T) {
	got := string(run.CancelDetail("alice", false, "ask-1", "taking a different approach"))
	for _, want := range []string{
		`"cause":"` + humanCancelCause + `"`,
		`"reason":"taking a different approach"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("run.CancelDetail produced %s, which does not carry %s — leg A lifts that key", got, want)
		}
	}
	// Absence is STRUCTURAL on this wire: no reason given means no key at all,
	// so the view can draw its honest line from the absence (the ask_id
	// precedent). A present-and-blank key would claim a motive that never
	// existed.
	if none := string(run.CancelDetail("alice", true, "ask-1", "")); strings.Contains(none, "reason") {
		t.Errorf("a reason-less cancel detail carries a reason key: %s", none)
	}
}

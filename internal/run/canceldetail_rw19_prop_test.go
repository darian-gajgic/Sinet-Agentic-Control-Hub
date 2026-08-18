package run_test

// canceldetail_rw19_prop_test.go — P3-RW-19 R4 / T13, the ADDITIVE-ONLY
// property of the one cancel-detail constructor, committed RED with the
// executor's test half (Amendment-A carve-out, CONVENTIONS §3).
//
// The human reason is a new member of a payload shape that three surfaces
// already read by key (`internal/api`'s decisions derive, the S14.10 catalog's
// `status.tasks_cancelled`, and the mint-side pins in stage and intake). A
// worked example proves the new key rides; it cannot prove the old ones did
// not move. So the contract is stated as a property over ALL inputs
// (stdlib testing/quick — no dependency):
//
//	the marshalled key set is a SUBSET of the five frozen names;
//	`cause` is byte-exact CancelCauseHuman for every input;
//	`ask_id` and `reason` are present IFF non-empty — absence is structural,
//	because a present-and-blank key claims a card, or a motive, that never
//	existed.

import (
	"encoding/json"
	"testing"
	"testing/quick"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// rw19FrozenDetailKeys is the whole vocabulary of a cancel detail. A sixth
// name appearing here is a wire change, and this is where it must be argued.
var rw19FrozenDetailKeys = map[string]bool{
	"cause": true, "actor": true, "ladder_invoked": true, "ask_id": true, "reason": true,
}

// rw19DetailHolds checks one input against the three limbs and reports what
// broke, so a quick.Check counterexample names its own failure.
func rw19DetailHolds(t *testing.T, actor string, ladder bool, askID, reason string) bool {
	t.Helper()
	raw := run.CancelDetail(actor, ladder, askID, reason)
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Errorf("CancelDetail(%q,%v,%q,%q) produced undecodable JSON %s: %v", actor, ladder, askID, reason, raw, err)
		return false
	}
	ok := true
	for k := range got {
		if !rw19FrozenDetailKeys[k] {
			t.Errorf("CancelDetail(%q,%v,%q,%q) marshalled the unfrozen key %q: %s", actor, ladder, askID, reason, k, raw)
			ok = false
		}
	}
	for _, always := range []string{"cause", "actor", "ladder_invoked"} {
		if _, present := got[always]; !present {
			t.Errorf("CancelDetail(%q,%v,%q,%q) dropped the always-present key %q: %s", actor, ladder, askID, reason, always, raw)
			ok = false
		}
	}
	var cause string
	if err := json.Unmarshal(got["cause"], &cause); err != nil || cause != run.CancelCauseHuman {
		t.Errorf("CancelDetail(%q,%v,%q,%q) wrote cause %q (err %v), want the frozen %q — three surfaces discriminate on it",
			actor, ladder, askID, reason, cause, err, run.CancelCauseHuman)
		ok = false
	}
	for _, c := range []struct {
		key, value string
	}{{"ask_id", askID}, {"reason", reason}} {
		_, present := got[c.key]
		if present != (c.value != "") {
			t.Errorf("CancelDetail(%q,%v,%q,%q): key %q present=%v, want present iff non-empty: %s",
				actor, ladder, askID, reason, c.key, present, raw)
			ok = false
		}
	}
	return ok
}

// TestCancelDetailPayloadIsAdditiveOnly — T13.
func TestCancelDetailPayloadIsAdditiveOnly(t *testing.T) {
	// The four corners first, by hand: every combination of the two
	// omit-when-empty members, which is where a struct-tag slip would land.
	for _, c := range []struct{ askID, reason string }{
		{"", ""}, {"ask-1", ""}, {"", "taking a different approach"}, {"ask-1", "taking a different approach"},
	} {
		rw19DetailHolds(t, "alice", false, c.askID, c.reason)
		rw19DetailHolds(t, "alice", true, c.askID, c.reason)
	}
	// Then the sweep: arbitrary actors, ask ids and reasons — quotes, braces,
	// newlines and non-ASCII all come out of the generator, and none of them
	// may add, drop or rename a key.
	prop := func(actor string, ladder bool, askID, reason string) bool {
		return rw19DetailHolds(t, actor, ladder, askID, reason)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
		t.Errorf("the additive-only property does not hold: %v", err)
	}
}

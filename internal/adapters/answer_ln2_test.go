package adapters

// answer_ln2_test.go — LN-2A: the shared approve/reject member. This package
// owns the type both substrates read, so the zero-value guarantee is pinned
// here, where neither lane can quietly redefine it.

import (
	"encoding/json"
	"testing"
)

func TestAnswerZeroValueMeansApprove(t *testing.T) {
	var zero Answer
	if zero.Decision != DecisionApprove {
		t.Fatalf("the zero Answer decision is %q, want the approve zero value", zero.Decision)
	}
	if DecisionApprove != "" {
		t.Fatal("DecisionApprove is not the zero value — every construction site predating the member " +
			"would change meaning without an edit")
	}
	if DecisionReject == DecisionApprove {
		t.Fatal("reject and approve are the same value")
	}
	// An approve is invisible on the wire, so a park record round-trips
	// byte-identically to one written before the member existed.
	blob, err := json.Marshal(Answer{AskID: "toolu_1", UpdatedInput: json.RawMessage(`{"a":1}`)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Answer
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Decision != DecisionApprove || back.Reason != "" {
		t.Errorf("round-tripped approve = %+v", back)
	}
}

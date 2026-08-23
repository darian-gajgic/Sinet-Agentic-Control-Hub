package claudecli

// answer_ln2_test.go — LN-2A: the shared adapters.Answer gains a first-class
// approve/reject member. On this lane the approve path must not move by a
// single byte, and the reject path becomes a real engine `deny`.

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

func runGateHook(t *testing.T, ctlDir, toolUseID string) hookDecision {
	t.Helper()
	in, err := json.Marshal(hookInput{
		SessionID: "sess-ln2", ToolName: "Bash", ToolUseID: toolUseID,
		ToolInput: json.RawMessage(`{"command":"echo original"}`), PermissionMode: "default",
	})
	if err != nil {
		t.Fatalf("marshal hook input: %v", err)
	}
	var out bytes.Buffer
	if err := RunHook(bytes.NewReader(in), &out, ctlDir); err != nil {
		t.Fatalf("RunHook: %v", err)
	}
	var decoded hookOutput
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("hook output %q: %v", out.String(), err)
	}
	return decoded.HookSpecificOutput
}

// ── spec 27 · the zero value approves, byte-identically ──────────────────────

func TestAnswerZeroValueIsApprove(t *testing.T) {
	// The type-level guarantee: a zero Answer is an approve on every lane.
	var zero adapters.Answer
	if zero.Decision != adapters.DecisionApprove {
		t.Fatalf("the zero Answer decision is %q, want approve — every construction site predating the "+
			"member would silently change meaning", zero.Decision)
	}

	ctl := t.TempDir()
	if err := writeCtlConfig(ctl, ""); err != nil {
		t.Fatalf("writeCtlConfig: %v", err)
	}
	updated := json.RawMessage(`{"command":"echo ANSWER-42"}`)
	if err := writeAnswer(ctl, "toolu_zero", updated); err != nil {
		t.Fatalf("writeAnswer: %v", err)
	}
	// Byte-identical to the shape that predates the decision member.
	raw, err := os.ReadFile(answerPath(ctl, "toolu_zero"))
	if err != nil {
		t.Fatalf("read staged answer: %v", err)
	}
	if string(raw) != `{"updatedInput":{"command":"echo ANSWER-42"}}` {
		t.Errorf("staged approve = %s, want the unchanged pre-member shape", raw)
	}

	d := runGateHook(t, ctl, "toolu_zero")
	if d.PermissionDecision != "allow" {
		t.Errorf("permissionDecision = %q, want allow", d.PermissionDecision)
	}
	var got struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(d.UpdatedInput, &got); err != nil || got.Command != "echo ANSWER-42" {
		t.Errorf("updatedInput = %s (err %v) — the genuine input substitution moved", d.UpdatedInput, err)
	}
}

// ── spec 32 · a reject makes the hook emit deny ──────────────────────────────

func TestClaudeHookEmitsDenyOnReject(t *testing.T) {
	ctl := t.TempDir()
	if err := writeCtlConfig(ctl, ""); err != nil {
		t.Fatalf("writeCtlConfig: %v", err)
	}
	const reason = "sinet gate: the operator refused this command"
	if err := writeDenyAnswer(ctl, "toolu_deny", reason); err != nil {
		t.Fatalf("writeDenyAnswer: %v", err)
	}
	d := runGateHook(t, ctl, "toolu_deny")
	if d.PermissionDecision != "deny" {
		t.Fatalf("permissionDecision = %q, want deny — an answered branch that only ever allows turns a "+
			"human's refusal into consent (S03.4)", d.PermissionDecision)
	}
	if !strings.Contains(d.PermissionDecisionReason, reason) {
		t.Errorf("deny reason = %q, want the human's reason carried through", d.PermissionDecisionReason)
	}
	if len(d.UpdatedInput) != 0 {
		t.Errorf("a denial carries replacement input: %s", d.UpdatedInput)
	}

	// An approve with no replacement input still allows, unchanged.
	if err := writeAnswer(ctl, "toolu_plain", nil); err != nil {
		t.Fatalf("writeAnswer: %v", err)
	}
	if d := runGateHook(t, ctl, "toolu_plain"); d.PermissionDecision != "allow" || len(d.UpdatedInput) != 0 {
		t.Errorf("bare approve = %+v, want allow with no updatedInput", d)
	}
}

// ── the Resume seam carries the decision to the control dir ──────────────────

func TestResumeStagesTheDecision(t *testing.T) {
	ctl := t.TempDir()
	if err := writeCtlConfig(ctl, ""); err != nil {
		t.Fatalf("writeCtlConfig: %v", err)
	}
	if err := stageResumeAnswer(ctl, &adapters.Answer{
		AskID: "toolu_r1", Decision: adapters.DecisionReject, Reason: "no",
	}); err != nil {
		t.Fatalf("stageResumeAnswer: %v", err)
	}
	if d := runGateHook(t, ctl, "toolu_r1"); d.PermissionDecision != "deny" {
		t.Errorf("staged reject produced %q", d.PermissionDecision)
	}
	// A decision this lane cannot express is refused loudly, never approved.
	err := stageResumeAnswer(ctl, &adapters.Answer{AskID: "toolu_r2", Decision: adapters.AnswerDecision("escalate")})
	if err == nil {
		t.Fatal("an inexpressible decision was staged silently")
	}
}

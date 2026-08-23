package claudecli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// The PreToolUse gate hook (S03.4): `defer` is the Anthropic lane's
// exit-park primitive — the headless process exits stop_reason
// "tool_deferred" and the platform builds its durable ask-record from the
// exit JSON alone. The hook binary is the sinet binary itself (`sinet
// engine-hook --ctl <dir>`), compiled into the lowered settings by this
// adapter; it exchanges state with the adapter through the gate control
// dir only:
//
//	<ctl>/config.json          written at lowering: {"fallback": ⚙ adapter.parallel_gate_fallback}
//	<ctl>/fires.log            appended per hook fire (JSONL); truncated by
//	                           the adapter at spawn and at each new
//	                           assistant-turn boundary — the fire count is
//	                           the S03.4 fallback-detection input
//	<ctl>/answers/<id>.json    written by the platform on resume:
//	                           {"updatedInput": …} (or {} = approve as-is)
//
// Decision logic per fire, in order (recorded contract, SPIKE G1-S2):
//
//  1. an answer file for this tool_use_id → allow (+updatedInput) — the
//     resume path: --resume re-fires PreToolUse on the same tool_use_id
//     and the answer replaces the input wholesale, no fork;
//  2. under serialize-by-deny, a fire that is not the turn's first →
//     deny with reason "re-issue the gated call alone" (SPIKE P2-S4:
//     converts the silent parallel fallback into ~one extra cheap turn);
//  3. otherwise → defer (exit-park).
//
// The turn-boundary reset behind rule 2 is adapter-maintained and
// heuristic at B1; the B1-4 spike battery (serialize-by-deny reconfirm,
// TBD-BRINGUP) re-measures the mechanism live before the gate carries
// real work.

// hookInput is the PreToolUse stdin payload (recorded at the pin: SPIKE
// G1-S1 — session_id, transcript_path, cwd, tool_name, tool_input; G1-S2
// logged permission_mode and tool_use_id per fire). Tolerant decode.
type hookInput struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	CWD            string          `json:"cwd"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolUseID      string          `json:"tool_use_id"`
	PermissionMode string          `json:"permission_mode"`
}

// hookOutput is the PreToolUse stdout decision, verbatim the recorded
// shape (SPIKE G1-S2 §Method).
type hookOutput struct {
	HookSpecificOutput hookDecision `json:"hookSpecificOutput"`
}

type hookDecision struct {
	HookEventName            string          `json:"hookEventName"`
	PermissionDecision       string          `json:"permissionDecision"`
	PermissionDecisionReason string          `json:"permissionDecisionReason"`
	UpdatedInput             json.RawMessage `json:"updatedInput,omitempty"`
}

// gateCtlConfig is <ctl>/config.json.
type gateCtlConfig struct {
	Fallback string `json:"fallback"` // ⚙ adapter.parallel_gate_fallback value at lowering
}

// fireRecord is one fires.log line.
type fireRecord struct {
	TS             string `json:"ts"`
	SessionID      string `json:"session_id"`
	ToolName       string `json:"tool_name"`
	ToolUseID      string `json:"tool_use_id"`
	PermissionMode string `json:"permission_mode"`
}

// serializeByDenyReason is the deny reason of the ratified fallback
// (S03.4: "deny all calls of a parallel gated turn with reason 're-issue
// the gated call alone'").
const serializeByDenyReason = "sinet gate: parallel gated turn — re-issue the gated call alone"

// deferReason marks platform-parked calls.
const deferReason = "sinet gate: parked awaiting answer (S03.4 exit-park)"

// RunHook executes one PreToolUse gate-hook invocation: stdin JSON in,
// decision JSON out. Wired as the `sinet engine-hook` tool subcommand
// (internal/cli); unit tests call it directly.
func RunHook(stdin io.Reader, stdout io.Writer, ctlDir string) error {
	if ctlDir == "" {
		return fmt.Errorf("engine-hook: missing --ctl dir")
	}
	raw, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
	if err != nil {
		return fmt.Errorf("engine-hook: read stdin: %w", err)
	}
	var in hookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return fmt.Errorf("engine-hook: parse stdin: %w", err)
	}

	// Record the fire first: the fire log must exist even when the
	// decision path fails — it is the fallback-detection evidence
	// (S03.4: >1 fire for one turn + no tool_deferred exit ⇒ fallback).
	firstOfTurn, err := appendFire(ctlDir, in)
	if err != nil {
		return err
	}

	if ans, ok, err := readAnswer(ctlDir, in.ToolUseID); err != nil {
		return err
	} else if ok {
		// An answered call is not automatically an allowed one. Until LN-2A
		// this branch could only ever say "allow", which made a human's
		// refusal unrepresentable on this lane; a first-class reject reaches
		// the engine through the deny verdict serialize-by-deny already uses.
		if ans.Decision == hookDecisionDeny {
			return writeDecision(stdout, hookDecision{
				HookEventName:            "PreToolUse",
				PermissionDecision:       hookDecisionDeny,
				PermissionDecisionReason: answerDenyReason(ans.Reason),
			})
		}
		return writeDecision(stdout, hookDecision{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "allow",
			PermissionDecisionReason: "sinet gate: answered (S03.4 resume re-supply)",
			UpdatedInput:             ans.UpdatedInput,
		})
	}

	cfg, err := readCtlConfig(ctlDir)
	if err != nil {
		return err
	}
	if cfg.Fallback == "serialize-by-deny" && !firstOfTurn {
		return writeDecision(stdout, hookDecision{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "deny",
			PermissionDecisionReason: serializeByDenyReason,
		})
	}
	return writeDecision(stdout, hookDecision{
		HookEventName:            "PreToolUse",
		PermissionDecision:       "defer",
		PermissionDecisionReason: deferReason,
	})
}

func writeDecision(w io.Writer, d hookDecision) error {
	return json.NewEncoder(w).Encode(hookOutput{HookSpecificOutput: d})
}

// appendFire appends one fire record and reports whether it is the first
// fire of the current turn window (fires.log is truncated by the adapter
// at each turn boundary).
func appendFire(ctlDir string, in hookInput) (firstOfTurn bool, err error) {
	rec, err := json.Marshal(fireRecord{
		TS:             time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:      in.SessionID,
		ToolName:       in.ToolName,
		ToolUseID:      in.ToolUseID,
		PermissionMode: in.PermissionMode,
	})
	if err != nil {
		return false, fmt.Errorf("engine-hook: marshal fire record: %w", err)
	}
	path := firesPath(ctlDir)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return false, fmt.Errorf("engine-hook: open fires log: %w", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("engine-hook: stat fires log: %w", err)
	}
	if _, err := f.Write(append(rec, '\n')); err != nil {
		return false, fmt.Errorf("engine-hook: append fire: %w", err)
	}
	return st.Size() == 0, nil
}

func readAnswer(ctlDir, toolUseID string) (answerFile, bool, error) {
	if toolUseID == "" {
		return answerFile{}, false, nil
	}
	raw, err := os.ReadFile(answerPath(ctlDir, toolUseID))
	if os.IsNotExist(err) {
		return answerFile{}, false, nil
	}
	if err != nil {
		return answerFile{}, false, fmt.Errorf("engine-hook: read answer: %w", err)
	}
	var a answerFile
	if err := json.Unmarshal(raw, &a); err != nil {
		return answerFile{}, false, fmt.Errorf("engine-hook: parse answer file: %w", err)
	}
	return a, true, nil
}

// answerDenyReason is what the engine hands the model when a gated call is
// refused. The human's own words lead; the platform prefix is what tells the
// model this was a decision rather than a tool failure.
func answerDenyReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "sinet gate: the call was refused"
	}
	return reason
}

func readCtlConfig(ctlDir string) (gateCtlConfig, error) {
	raw, err := os.ReadFile(filepath.Join(ctlDir, "config.json"))
	if os.IsNotExist(err) {
		return gateCtlConfig{}, nil
	}
	if err != nil {
		return gateCtlConfig{}, fmt.Errorf("engine-hook: read ctl config: %w", err)
	}
	var cfg gateCtlConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return gateCtlConfig{}, fmt.Errorf("engine-hook: parse ctl config: %w", err)
	}
	return cfg, nil
}

func firesPath(ctlDir string) string { return filepath.Join(ctlDir, "fires.log") }

func answerPath(ctlDir, toolUseID string) string {
	// tool_use_ids are engine-minted identifiers; sanitize defensively so
	// a hostile id cannot escape the answers dir.
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		}
		return '_'
	}, toolUseID)
	return filepath.Join(ctlDir, "answers", safe+".json")
}

// countFires reads the current fire count and records (adapter side:
// post-exit fallback detection + evidence).
func countFires(ctlDir string) (int, []fireRecord, error) {
	raw, err := os.ReadFile(firesPath(ctlDir))
	if os.IsNotExist(err) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, err
	}
	var recs []fireRecord
	n := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n++
		var r fireRecord
		if err := json.Unmarshal([]byte(line), &r); err == nil {
			recs = append(recs, r)
		}
	}
	return n, recs, nil
}

// truncateFires resets the turn window (adapter side, at spawn and at
// each new assistant-turn boundary).
func truncateFires(ctlDir string) error {
	err := os.Truncate(firesPath(ctlDir), 0)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// writeCtlConfig materializes <ctl>/config.json (+ answers dir) at spawn.
func writeCtlConfig(ctlDir, fallback string) error {
	if err := os.MkdirAll(filepath.Join(ctlDir, "answers"), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(gateCtlConfig{Fallback: fallback})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ctlDir, "config.json"), raw, 0o600)
}

// answerFile is <ctl>/answers/<id>.json. UpdatedInput leads so an APPROVE
// marshals byte-identically to the shape that predates the decision member.
type answerFile struct {
	UpdatedInput json.RawMessage `json:"updatedInput,omitempty"`
	// Decision is empty for an approve (the zero value approves, shared
	// adapters.Answer) and "deny" for a first-class rejection.
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// hookDecisionDeny is the engine's own refusal verdict. The mechanism is the
// one serialize-by-deny already relies on, so a first-class reject is wiring an
// existing engine capability rather than inventing one (S03.4; S02.8).
const hookDecisionDeny = "deny"

// writeAnswer persists the platform's APPROVAL for a tool_use_id before a
// resume (adapter side): allow, optionally with replacement input.
func writeAnswer(ctlDir, toolUseID string, updatedInput json.RawMessage) error {
	return stageAnswer(ctlDir, toolUseID, answerFile{UpdatedInput: updatedInput})
}

// writeDenyAnswer persists a first-class REJECTION with its reason. The hook
// turns it into permissionDecision "deny": a refusal that arrived as an
// approve-shaped file would run the very call a human refused.
func writeDenyAnswer(ctlDir, toolUseID, reason string) error {
	return stageAnswer(ctlDir, toolUseID, answerFile{Decision: hookDecisionDeny, Reason: reason})
}

// stageResumeAnswer maps a platform Answer onto the control dir before a
// resume. A decision this lane cannot express is refused LOUDLY: the tempting
// fallback is the approve-shaped file, and that turns a refusal into consent.
func stageResumeAnswer(ctlDir string, ans *adapters.Answer) error {
	switch ans.Decision {
	case adapters.DecisionApprove:
		return writeAnswer(ctlDir, ans.AskID, ans.UpdatedInput)
	case adapters.DecisionReject:
		if len(ans.UpdatedInput) > 0 {
			// A refusal that also edits the call is two answers at once, and
			// obeying either half would execute something nobody chose.
			return fmt.Errorf("engine-hook: answer %q rejects the call AND supplies replacement input; "+
				"a refusal has nothing to substitute into (S03.4)", ans.AskID)
		}
		return writeDenyAnswer(ctlDir, ans.AskID, ans.Reason)
	}
	return fmt.Errorf("engine-hook: answer %q carries decision %q, which this lane cannot express — refusing "+
		"the whole answer rather than defaulting it, because the only default available is consent (S03.4)",
		ans.AskID, ans.Decision)
}

func stageAnswer(ctlDir, toolUseID string, a answerFile) error {
	if err := os.MkdirAll(filepath.Join(ctlDir, "answers"), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return os.WriteFile(answerPath(ctlDir, toolUseID), raw, 0o600)
}

package claudecli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tier F (replay/hermetic): the SessionStart re-injection channel against
// the B1-4 spike-recorded engine contract (P3/measurements/
// 2026-07-20-precompact-injection-mechanics.md M3/M4) — the
// hookSpecificOutput emission shape the spike proved reaches the model,
// the source enum matcher, and the settings compilation that wires it.

func TestLowerCompilesSessionStartHook(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	pinned := filepath.Join(req.WorkDir, "pinned-context.md")
	req.Worker.SessionStartContextPath = pinned

	l, err := a.lower(req, nil, "11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	var es struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(l.settingsJSON, &es); err != nil {
		t.Fatalf("settings JSON: %v", err)
	}
	ss, ok := es.Hooks["SessionStart"]
	if !ok || len(ss) != 1 {
		t.Fatalf("no SessionStart hook compiled: %s", l.settingsJSON)
	}
	// Matcher = the spike-observed source enum (M3: startup, resume,
	// compact) — re-injection at every session-start boundary.
	if ss[0].Matcher != "startup|resume|compact" {
		t.Fatalf("matcher = %q", ss[0].Matcher)
	}
	cmd := ss[0].Hooks[0].Command
	if ss[0].Hooks[0].Type != "command" ||
		!strings.Contains(cmd, "--session-start '"+pinned+"'") ||
		!strings.Contains(cmd, "--ctl '") ||
		!strings.HasPrefix(cmd, "/opt/sinet/bin/sinet engine-hook") {
		t.Fatalf("hook command = %q", cmd)
	}
	// The gate hook coexists (both channels ride the one settings object).
	if _, ok := es.Hooks["PreToolUse"]; !ok {
		t.Fatalf("PreToolUse hook lost: %s", l.settingsJSON)
	}

	// The channel is part of the S02.4(e) invocation fingerprint (it rides
	// settingsJSON): with vs without must differ.
	req2 := testRequest(t)
	l2, err := a.lower(req2, nil, "11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatalf("lower without channel: %v", err)
	}
	if bytes.Equal(l.settingsJSON, l2.settingsJSON) {
		t.Fatalf("SessionStart wiring absent from compiled settings")
	}
}

func TestRunSessionStartHookEmitsPinnedContextVerbatim(t *testing.T) {
	ctl := t.TempDir()
	dir := t.TempDir()
	content := "=== [task] ledger/objective_ac v7 ===\n{\n  \"objective\": \"Ship\"\n}\n"
	path := filepath.Join(dir, "pinned-context.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("place pinned: %v", err)
	}

	stdin := strings.NewReader(`{"session_id":"s1","source":"compact","hook_event_name":"SessionStart"}`)
	var stdout bytes.Buffer
	if err := RunSessionStartHook(stdin, &stdout, ctl, path); err != nil {
		t.Fatalf("RunSessionStartHook: %v", err)
	}

	// The spike-recorded emission contract, exactly (M3): additionalContext
	// under hookSpecificOutput reaches the model.
	var out struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout: %v", err)
	}
	if out.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Fatalf("hookEventName = %q", out.HookSpecificOutput.HookEventName)
	}
	if out.HookSpecificOutput.AdditionalContext != content {
		t.Fatalf("re-injection is not verbatim:\n%q\nvs\n%q", out.HookSpecificOutput.AdditionalContext, content)
	}

	// Every fire is recorded for the S05.4 mid-stage manifest.
	fires, err := ReadSessionStartFires(ctl)
	if err != nil {
		t.Fatalf("ReadSessionStartFires: %v", err)
	}
	sum := sha256.Sum256([]byte(content))
	if len(fires) != 1 || fires[0].SessionID != "s1" || fires[0].Source != "compact" ||
		fires[0].ContentSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("fires = %+v", fires)
	}

	// A second boundary (resume) appends, preserving order.
	if err := RunSessionStartHook(strings.NewReader(`{"session_id":"s1","source":"resume"}`), &bytes.Buffer{}, ctl, path); err != nil {
		t.Fatalf("second fire: %v", err)
	}
	fires, err = ReadSessionStartFires(ctl)
	if err != nil || len(fires) != 2 || fires[1].Source != "resume" {
		t.Fatalf("fires after resume = %+v, %v", fires, err)
	}
}

func TestRunSessionStartHookFailsLoudWithoutPlacement(t *testing.T) {
	ctl := t.TempDir()
	stdin := strings.NewReader(`{"session_id":"s1","source":"startup"}`)
	err := RunSessionStartHook(stdin, &bytes.Buffer{}, ctl, filepath.Join(t.TempDir(), "missing.md"))
	if err == nil {
		t.Fatalf("missing pinned-context file must fail loudly, not inject nothing")
	}
	if err := RunSessionStartHook(strings.NewReader("{}"), &bytes.Buffer{}, "", "x"); err == nil {
		t.Fatalf("missing ctl dir must fail")
	}
	if err := RunSessionStartHook(strings.NewReader("{}"), &bytes.Buffer{}, ctl, ""); err == nil {
		t.Fatalf("missing context path must fail")
	}
}

package claudecli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The SessionStart re-injection hook — the Claude-lane deterministic
// injection channel resolved by the B1-4 spike (P3/measurements/
// 2026-07-20-precompact-injection-mechanics.md; Spec S05.4, S05.7 step 3):
// after any session-start boundary — startup, resume, and post-compaction
// (`source:"compact"`) — the hook re-emits the platform-placed
// pinned-context file verbatim as `additionalContext`, so the ledger's
// pinned sections survive outside any summarizer's reach. Wired through
// the lowered settings exactly like the S03.4 gate hook; the engine
// contract is the spike-recorded hookSpecificOutput shape.
//
// Every fire is appended to <ctl>/session_start.fires.log; the platform
// side turns fires into S05.4 mid-stage manifest entries
// (ledger.AppendReinjectionManifest) when it pumps the stage.

// sessionStartMatcher is the source enum the hook matches, exactly the
// spike-observed set (M3: sources observed = startup, resume, compact).
const sessionStartMatcher = "startup|resume|compact"

// sessionStartFiresFile is the fire log inside the gate control dir — the
// one read-write exchange dir under confinement (CONVENTIONS §12; the
// pinned-context file itself is read-only platform config).
const sessionStartFiresFile = "session_start.fires.log"

// sessionStartInput is the SessionStart stdin payload (spike-recorded:
// session_id, source, hook_event_name among others). Tolerant decode.
type sessionStartInput struct {
	SessionID     string `json:"session_id"`
	Source        string `json:"source"`
	HookEventName string `json:"hook_event_name"`
}

// sessionStartOutput is the spike-recorded emission contract (M3): a
// hook emitting {"hookSpecificOutput":{"hookEventName":"SessionStart",
// "additionalContext":…}} reaches the model.
type sessionStartOutput struct {
	HookSpecificOutput sessionStartSpecific `json:"hookSpecificOutput"`
}

type sessionStartSpecific struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// SessionStartFire is one recorded hook fire — the platform-side input for
// the S05.4 mid-stage re-injection manifest.
type SessionStartFire struct {
	TS            string `json:"ts"`
	SessionID     string `json:"session_id"`
	Source        string `json:"source"`
	ContentSHA256 string `json:"content_sha256"`
}

// RunSessionStartHook executes one SessionStart re-injection: stdin hook
// payload in, hookSpecificOutput JSON out, emitting the pinned-context
// file verbatim. Wired as `sinet engine-hook --ctl <dir> --session-start
// <path>` (internal/cli); unit tests call it directly. A missing pinned
// file is a loud failure (nonzero exit): the placement is a platform duty
// and its absence is a defect, never silently uninjected context.
func RunSessionStartHook(stdin io.Reader, stdout io.Writer, ctlDir, contextPath string) error {
	if ctlDir == "" {
		return fmt.Errorf("engine-hook: missing --ctl dir")
	}
	if contextPath == "" {
		return fmt.Errorf("engine-hook: missing --session-start context path")
	}
	raw, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
	if err != nil {
		return fmt.Errorf("engine-hook: read stdin: %w", err)
	}
	var in sessionStartInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return fmt.Errorf("engine-hook: parse stdin: %w", err)
	}
	content, err := os.ReadFile(contextPath)
	if err != nil {
		return fmt.Errorf("engine-hook: read pinned context: %w", err)
	}
	sum := sha256.Sum256(content)
	if err := appendSessionStartFire(ctlDir, SessionStartFire{
		TS:            time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:     in.SessionID,
		Source:        in.Source,
		ContentSHA256: hex.EncodeToString(sum[:]),
	}); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(sessionStartOutput{
		HookSpecificOutput: sessionStartSpecific{
			HookEventName:     "SessionStart",
			AdditionalContext: string(content),
		},
	})
}

func appendSessionStartFire(ctlDir string, fire SessionStartFire) error {
	if err := os.MkdirAll(ctlDir, 0o700); err != nil {
		return fmt.Errorf("engine-hook: ctl dir: %w", err)
	}
	rec, err := json.Marshal(fire)
	if err != nil {
		return fmt.Errorf("engine-hook: marshal fire record: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(ctlDir, sessionStartFiresFile),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("engine-hook: open session-start fires log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(rec, '\n')); err != nil {
		return fmt.Errorf("engine-hook: append fire: %w", err)
	}
	return nil
}

// ReadSessionStartFires returns the recorded SessionStart fires of a run's
// control dir, oldest first — the platform-side source for mid-stage
// re-injection manifest entries (Spec S05.4).
func ReadSessionStartFires(ctlDir string) ([]SessionStartFire, error) {
	raw, err := os.ReadFile(filepath.Join(ctlDir, sessionStartFiresFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var fires []SessionStartFire
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var f SessionStartFire
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			return nil, fmt.Errorf("claudecli: parse session-start fire: %w", err)
		}
		fires = append(fires, f)
	}
	return fires, nil
}

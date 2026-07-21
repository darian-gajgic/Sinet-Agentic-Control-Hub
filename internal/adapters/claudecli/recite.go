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

// The PostToolUse recitation delivery valve — the C1 per-turn channel of
// Research/18 §7 (operator standing directive 2026-07-20), scope v0 =
// recitation ONLY: platform-authored ledger state/next_actions at a pinned
// ledger_version (Spec S05.3), delivered over the S03.4 ctl-dir airlock —
// the one read-write exchange bind (CONVENTIONS §12) — with NO new socket,
// NO new credential, NO new principal.
//
// Protocol (the "quiet inbox" contract on Sinet's own airlock):
//
//	<ctl>/recite/pending.json      written by the PLATFORM (stage reciter)
//	                               via temp-file + atomic rename when
//	                               recitation is due (⚙ context.
//	                               recitation_interval_turns; dueness is
//	                               computed platform-side from run events —
//	                               the hook never decides anything)
//	<ctl>/recite/delivered-<ns>.json  the consumed file after delivery
//	                               (rename target; retained as evidence)
//	<ctl>/recite.fires.log         JSONL, one line per DELIVERY; the
//	                               platform turns fires into S05.4
//	                               mid-stage manifest events + journal
//	                               (the CONVENTIONS §13 fires-log pattern)
//
// Consumption is os.Rename — atomic on one filesystem, so under parallel
// tool calls (two PostToolUse fires racing one pending file) exactly ONE
// hook invocation wins the rename and speaks; every loser sees ENOENT and
// prints nothing. There is no lock, no daemon, no state beyond the files.
//
// Dated engine facts this channel rests on (probed live 2026-07-21 on
// installed claude 2.1.216 vs lock pin 2.1.215 — P3/measurements/
// 2026-07-21-posttooluse-additionalcontext-probe.md; re-checked per pin
// bump as canary rows):
//
//	P1  PostToolUse command hooks fire per matched tool call in -p
//	    stream-json; matcher = tool-name alternation (same as PreToolUse).
//	P2  stdout {"hookSpecificOutput":{"hookEventName":"PostToolUse",
//	    "additionalContext":…}} reaches the model mid-turn; EMPTY stdout
//	    injects nothing (the quiet path is genuinely free).
//	P3  the stdin payload is the PreToolUse field set plus tool_response
//	    and duration_ms.
//
// Explicitly NOT here (Research/18 §7-C1 condition 1): cancel-notice and
// operator-note delivery — amendment territory (S00.9), not adopted. The
// pending-file content is recipient-fixed platform-authored recitation;
// free-form message passing would re-create the lateral channel D6 bans.

// Recite ctl-dir layout constants (the airlock file contract; the
// platform-side writer in internal/stage mirrors these shapes against
// CONVENTIONS §20 — the §13 session_start.fires.log precedent — and the
// stage↔claudecli contract test pins the two halves byte-level).
const (
	reciteSubdir    = "recite"
	recitePending   = "pending.json"
	reciteFiresFile = "recite.fires.log"
)

// PendingRecitation is the pending-file payload: platform-authored ledger
// state/next_actions at a PINNED ledger_version (Spec S05.3; Research/18
// §7-C1 condition 2 — content by pure lookup, no free-form messaging).
type PendingRecitation struct {
	LedgerVersion int64  `json:"ledger_version"`
	Content       string `json:"content"`
	ContentSHA256 string `json:"content_sha256"`
	WrittenAt     string `json:"written_at"`
}

// ReciteFire is one recorded delivery — the platform-side input for the
// S05.4 mid-stage manifest + run-event journal (condition 3: every
// delivery manifested and journaled; nothing silent). ContentSHA256 is
// recomputed by the hook from the delivered bytes, never trusted from the
// pending payload.
type ReciteFire struct {
	TS            string `json:"ts"`
	SessionID     string `json:"session_id"`
	ToolUseID     string `json:"tool_use_id"`
	LedgerVersion int64  `json:"ledger_version"`
	ContentSHA256 string `json:"content_sha256"`
}

// postToolUseInput is the PostToolUse stdin payload (probe-recorded field
// set, 2026-07-21: session_id, transcript_path, cwd, prompt_id,
// permission_mode, hook_event_name, tool_name, tool_input, tool_response,
// tool_use_id, duration_ms). Tolerant decode — only identity fields are
// consumed.
type postToolUseInput struct {
	SessionID     string `json:"session_id"`
	ToolUseID     string `json:"tool_use_id"`
	HookEventName string `json:"hook_event_name"`
}

// postToolUseOutput is the probe-recorded emission contract (P2).
type postToolUseOutput struct {
	HookSpecificOutput postToolUseSpecific `json:"hookSpecificOutput"`
}

type postToolUseSpecific struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// RunPostToolUseHook executes one recitation delivery-valve invocation:
// stdin hook payload in; on a pending recitation, consume-and-emit as
// additionalContext; on a quiet inbox, print NOTHING (probe P2: empty
// stdout injects zero context). Wired as `sinet engine-hook --ctl <dir>
// --post-tool-use` (internal/cli); unit tests call it directly.
//
// The hook is a delivery valve ONLY (Research/18 §7-C1 condition 6): it
// holds no cadence state, reads no settings, and makes no decision beyond
// "is something pending". PreToolUse gating (hook.go) is untouched —
// single-purpose per condition 4.
func RunPostToolUseHook(stdin io.Reader, stdout io.Writer, ctlDir string) error {
	if ctlDir == "" {
		return fmt.Errorf("engine-hook: missing --ctl dir")
	}
	raw, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
	if err != nil {
		return fmt.Errorf("engine-hook: read stdin: %w", err)
	}
	var in postToolUseInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return fmt.Errorf("engine-hook: parse stdin: %w", err)
	}

	pending, deliveredPath, ok, err := consumePendingRecitation(ctlDir)
	if err != nil {
		return err
	}
	if !ok {
		// Quiet inbox: zero stdout, zero injected context (probe P2).
		return nil
	}
	// Crash-window note (C3 rows): a death between the rename above and the
	// fire append below leaves a delivered-* file with NO fire record — the
	// platform manifests deliveries from FIRES only, so nothing ever claims
	// an injection that was not evidenced; the periodic reciter re-writes a
	// fresh pending at the next dueness (self-healing, never silent-lying).
	sum := sha256.Sum256([]byte(pending.Content))
	if err := appendReciteFire(ctlDir, ReciteFire{
		TS:            time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:     in.SessionID,
		ToolUseID:     in.ToolUseID,
		LedgerVersion: pending.LedgerVersion,
		ContentSHA256: hex.EncodeToString(sum[:]),
	}); err != nil {
		// The delivery is aborted, not half-made: without a fire record the
		// content must not speak (an unevidenced injection would be a
		// silent delivery — condition 3). The consumed file remains as the
		// on-disk trace of the aborted attempt.
		return fmt.Errorf("%w (claimed recitation retained at %s)", err, deliveredPath)
	}
	return json.NewEncoder(stdout).Encode(postToolUseOutput{
		HookSpecificOutput: postToolUseSpecific{
			HookEventName:     "PostToolUse",
			AdditionalContext: pending.Content,
		},
	})
}

// consumePendingRecitation atomically claims the pending file: rename to a
// delivered-<nanos> name (single winner under concurrent fires), then read
// the claimed copy. ok=false (no error) when nothing is pending — the
// quiet path.
func consumePendingRecitation(ctlDir string) (p PendingRecitation, deliveredPath string, ok bool, err error) {
	dir := filepath.Join(ctlDir, reciteSubdir)
	pendingPath := filepath.Join(dir, recitePending)
	deliveredPath = filepath.Join(dir, fmt.Sprintf("delivered-%d.json", time.Now().UnixNano()))
	if err := os.Rename(pendingPath, deliveredPath); err != nil {
		if os.IsNotExist(err) {
			return PendingRecitation{}, "", false, nil
		}
		return PendingRecitation{}, "", false, fmt.Errorf("engine-hook: claim pending recitation: %w", err)
	}
	raw, err := os.ReadFile(deliveredPath)
	if err != nil {
		return PendingRecitation{}, "", false, fmt.Errorf("engine-hook: read claimed recitation: %w", err)
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return PendingRecitation{}, "", false, fmt.Errorf("engine-hook: parse pending recitation: %w", err)
	}
	if p.Content == "" {
		return PendingRecitation{}, "", false, fmt.Errorf("engine-hook: pending recitation without content")
	}
	return p, deliveredPath, true, nil
}

func appendReciteFire(ctlDir string, fire ReciteFire) error {
	rec, err := json.Marshal(fire)
	if err != nil {
		return fmt.Errorf("engine-hook: marshal recite fire: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(ctlDir, reciteFiresFile),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("engine-hook: open recite fires log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(rec, '\n')); err != nil {
		return fmt.Errorf("engine-hook: append recite fire: %w", err)
	}
	return nil
}

// ensureReciteDir materializes <ctl>/recite/ at spawn (adapter side) so an
// in-sandbox hook never needs MkdirAll rights beyond the rw exchange bind.
func ensureReciteDir(ctlDir string) error {
	return os.MkdirAll(filepath.Join(ctlDir, reciteSubdir), 0o700)
}

// resetRecite clears recitation state at a NON-resume spawn: a fresh
// session starts un-recited (its brief is fresh, Spec S05.3) and a
// leftover pending/fires trail from a dead prior invocation of the same
// stage workdir must neither deliver stale content nor double-manifest. A
// RESUME keeps the state — the parked invocation continues and an
// undelivered pending stays deliverable (its content is still
// platform-authored truth at its recorded pinned version).
func resetRecite(ctlDir string) error {
	dir := filepath.Join(ctlDir, reciteSubdir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("claudecli: reset recite dir: %w", err)
	}
	for _, e := range entries {
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("claudecli: reset recite dir: %w", err)
		}
	}
	if err := os.Remove(filepath.Join(ctlDir, reciteFiresFile)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("claudecli: reset recite fires: %w", err)
	}
	return nil
}

// ReadReciteFires returns the recorded recitation deliveries of a run's
// control dir, oldest first — the platform-side source for S05.4 manifest
// events (internal/stage re-reads the same log without importing this
// package, the CONVENTIONS §13 platform-side-artifact precedent; this
// reader serves the adapter's own tests and tooling).
func ReadReciteFires(ctlDir string) ([]ReciteFire, error) {
	raw, err := os.ReadFile(filepath.Join(ctlDir, reciteFiresFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var fires []ReciteFire
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var f ReciteFire
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			return nil, fmt.Errorf("claudecli: parse recite fire: %w", err)
		}
		fires = append(fires, f)
	}
	return fires, nil
}

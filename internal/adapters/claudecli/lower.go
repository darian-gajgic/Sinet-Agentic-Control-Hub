// Package claudecli is the Anthropic-lane adapter of Spec S03.2: the
// pinned `claude` CLI wrapped behind the D3 contract (internal/adapters).
// The engine runs UNMODIFIED (S16.1; S16.6): every platform opinion —
// lowering, parsing, gating, checkpoints — lives on this side of the seam.
//
// Engine pin: components.lock entry "claude CLI (engine)"; the pin is the
// behavioral contract this adapter targets and the conformance suite
// asserts (S03.3: assert behavior, never docs). Bumps land only through
// the S03.3 operator-gated procedure.
package claudecli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
)

// ⚙ keys this adapter consumes (Spec S03 settings table; declared in the
// S18 registry, read by dotted key per CONVENTIONS §2).
const (
	keyCleanupDays  = "adapter.claude.cleanup_period_days"
	keyBackstopMult = "adapter.engine_ceiling_backstop_mult"
	keyGateFallback = "adapter.parallel_gate_fallback"
)

// nativeSpawnTools is the engine's native subagent family. The tool
// allowlist MUST exclude all of it (S03.5 sole-controller posture, G1
// rider 2: engine-native spawning is structurally disabled; the operator
// re-confirmed KEEP DISABLED at S03 review 2026-07-18).
var nativeSpawnTools = map[string]bool{
	"Task": true, "TaskCreate": true, "TaskGet": true, "TaskList": true,
	"TaskOutput": true, "TaskStop": true, "TaskUpdate": true,
}

// forbiddenFlags are never emitted by the lowering, under any input
// (S03.2: "never --dangerously-skip-permissions").
var forbiddenFlags = []string{"--dangerously-skip-permissions"}

// envScrubExact and envScrubPrefixes are the S03.5 nested-session env
// scrub set, plus the credential/env channel: ANTHROPIC_* is scrubbed so
// ambient API keys can never flip the invocation off subscription auth —
// lane auth is the per-user config root only (S03.2 setup-token; compiled
// config is the ONLY config, S03.5). CLAUDE_CONFIG_DIR is scrubbed and
// re-set from the owner credential ref exclusively.
var (
	envScrubExact    = map[string]bool{"CLAUDECODE": true, "AI_AGENT": true, "CLAUDE_EFFORT": true, "CLAUDE_PID": true, "CLAUDE_CONFIG_DIR": true}
	envScrubPrefixes = []string{"CLAUDE_CODE_", "ANTHROPIC_"}
)

// engineSettings is the compiled settings file content — the ONE settings
// object the engine sees (S03.5: --settings <sinet.json> +
// --setting-sources "" closes the settings/hooks/CLAUDE.md channel).
type engineSettings struct {
	// CleanupPeriodDays is ⚙ adapter.claude.cleanup_period_days: raised
	// well above the park horizon so a parked session cannot silently
	// evaporate in the engine's startup sweep (P-T02-1; S03.4).
	CleanupPeriodDays int64 `json:"cleanupPeriodDays"`
	// Hooks carries the PreToolUse gate wiring for the gated tools
	// (S03.4: gate via PreToolUse hooks, never canUseTool).
	Hooks map[string][]hookMatcher `json:"hooks,omitempty"`
}

type hookMatcher struct {
	Matcher string      `json:"matcher"`
	Hooks   []hookEntry `json:"hooks"`
}

type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// lowered is one compiled invocation: engine lowering per S03.5 — the
// compiled config is the ONLY config the engine sees, one knob per config
// channel.
type lowered struct {
	argv         []string // argv[0] = engine binary
	env          []string
	settingsJSON []byte
	settingsPath string
	ctlDir       string // gate-hook control dir (fires log, answers, config)
	fingerprint  string // S02.4(e): settings hash + permission mode + model (+ toolset/agent body)
	sessionID    string // requested --session-id (start) — the REPORTED id still wins (S02.4b)
	resume       bool
	cleanupDays  int64  // ⚙ adapter.claude.cleanup_period_days at lowering (P-T02-1 horizon)
	gateFallback string // ⚙ adapter.parallel_gate_fallback at lowering
}

// resumeSpec augments lowering for the resume path (S03.4: full
// invocation re-supply; --resume with the engine-reported session id, no
// --session-id, no fork on the defer path).
type resumeSpec struct {
	sessionID string
}

// lower compiles a StartRequest to the exact engine invocation.
func (a *Adapter) lower(req adapters.StartRequest, res *resumeSpec, newSessionID string) (*lowered, error) {
	if req.RunID == "" {
		return nil, fmt.Errorf("claudecli: start without run_id")
	}
	if req.Model == "" {
		return nil, fmt.Errorf("claudecli: start without model (S03.2: model per invocation)")
	}
	if req.WorkDir == "" {
		return nil, fmt.Errorf("claudecli: start without WorkDir (lowered config needs a platform-owned dir, S03.5)")
	}
	if req.Cwd == "" {
		return nil, fmt.Errorf("claudecli: start without Cwd (isolated run cwd, S03.5)")
	}
	for _, tool := range req.Worker.ToolAllowlist {
		if nativeSpawnTools[tool] {
			return nil, fmt.Errorf("claudecli: tool allowlist contains native-spawn tool %q — engine-native subagents are structurally disabled (S03.5)", tool)
		}
	}
	if a.Settings == nil {
		return nil, fmt.Errorf("claudecli: adapter without a settings registry (⚙ discipline, S01.10)")
	}
	cleanupDays, err := a.Settings.Int(keyCleanupDays)
	if err != nil {
		return nil, fmt.Errorf("claudecli: read ⚙ %s: %w", keyCleanupDays, err)
	}
	backstop, err := a.Settings.Float(keyBackstopMult)
	if err != nil {
		return nil, fmt.Errorf("claudecli: read ⚙ %s: %w", keyBackstopMult, err)
	}
	fallback, err := a.Settings.String(keyGateFallback)
	if err != nil {
		return nil, fmt.Errorf("claudecli: read ⚙ %s: %w", keyGateFallback, err)
	}

	ctlDir := filepath.Join(req.WorkDir, "gate-ctl")
	settingsPath := filepath.Join(req.WorkDir, "settings.json")

	es := engineSettings{CleanupPeriodDays: cleanupDays}
	if len(req.Worker.GatedTools) > 0 {
		hookCmd := a.hookCommand()
		if hookCmd == "" {
			return nil, fmt.Errorf("claudecli: gated tools declared but no gate-hook command available")
		}
		es.Hooks = map[string][]hookMatcher{
			// Matcher = alternation of exactly the gated tools (S03.4).
			"PreToolUse": {{
				Matcher: strings.Join(req.Worker.GatedTools, "|"),
				Hooks:   []hookEntry{{Type: "command", Command: fmt.Sprintf("%s --ctl '%s'", hookCmd, ctlDir)}},
			}},
		}
	}
	settingsJSON, err := json.Marshal(es)
	if err != nil {
		return nil, fmt.Errorf("claudecli: marshal engine settings: %w", err)
	}

	// Invocation core, verbatim S03.2.
	argv := []string{a.binary(), "-p",
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
	}
	// Config channels, one knob per channel (S03.5 table).
	argv = append(argv,
		"--settings", settingsPath,
		"--setting-sources", "", // settings/CLAUDE.md/HOME cross-read channel
		"--strict-mcp-config",                                  // MCP/connector channel (no --mcp-config: no Sinet MCP servers exist yet)
		"--disable-slash-commands",                             // skills/slash-command channel
		"--tools", strings.Join(req.Worker.ToolAllowlist, ","), // default-deny toolset; excludes Task* (validated above)
	)
	l := &lowered{
		settingsJSON: settingsJSON, settingsPath: settingsPath, ctlDir: ctlDir,
		cleanupDays: cleanupDays, gateFallback: fallback,
	}
	if res != nil {
		if res.sessionID == "" {
			return nil, fmt.Errorf("claudecli: resume without an engine-reported session id")
		}
		argv = append(argv, "--resume", res.sessionID)
		l.resume = true
		l.sessionID = res.sessionID
	} else {
		if newSessionID == "" {
			return nil, fmt.Errorf("claudecli: start without a control-plane session id")
		}
		// Control-plane-chosen session id, honored exactly by the engine
		// (S03.1 start row) — the checkpoint cursor still records the
		// REPORTED id, never this requested one (S02.4b).
		argv = append(argv, "--session-id", newSessionID)
		l.sessionID = newSessionID
	}
	argv = append(argv, "--model", req.Model)
	if req.Worker.PermissionMode != "" {
		argv = append(argv, "--permission-mode", req.Worker.PermissionMode)
	}
	if len(req.Worker.AgentsJSON) > 0 {
		if req.Worker.AgentName == "" {
			return nil, fmt.Errorf("claudecli: agents JSON without --agent name")
		}
		// Inline agent injection — no disk write into any shared dir
		// (S03.5 agent/prompt-body row).
		argv = append(argv, "--agents", string(req.Worker.AgentsJSON), "--agent", req.Worker.AgentName)
	}
	if req.Worker.SystemPromptAppend != "" {
		argv = append(argv, "--append-system-prompt", req.Worker.SystemPromptAppend)
	}
	// Engine ceilings as belts only: ≥ ⚙ adapter.engine_ceiling_backstop_mult
	// × the platform ceiling, so the platform gate always fires first and a
	// same-turn budget trip cannot pre-empt a park (S03.4 ceiling ordering;
	// the platform ledger is the real ceiling, Spec S10).
	if req.CeilingSteps > 0 {
		argv = append(argv, "--max-turns", strconv.FormatInt(int64(float64(req.CeilingSteps)*backstop+0.5), 10))
	}
	if req.CeilingCostUSD > 0 {
		argv = append(argv, "--max-budget-usd", strconv.FormatFloat(req.CeilingCostUSD*backstop, 'f', 4, 64))
	}
	for _, f := range argv {
		for _, forbidden := range forbiddenFlags {
			if f == forbidden {
				return nil, fmt.Errorf("claudecli: forbidden flag %s in lowering", forbidden)
			}
		}
	}
	l.argv = argv
	l.env = loweredEnv(a.environ(), req.OwnerCredRef)
	l.fingerprint = fingerprint(settingsJSON, req, fallback)
	return l, nil
}

// loweredEnv builds the engine environment: the ambient environment minus
// the S03.5 scrub set (nested-session markers, ANTHROPIC_*,
// CLAUDE_CONFIG_DIR), with the per-user config root re-added exclusively
// from the owner credential ref (D2: per-run owner credential in the
// engine process; the ref is a path, never material). Full environment
// construction is the sandbox's at B1-3 (Spec S11) — this scrub is the
// named dev-mode seam.
func loweredEnv(base []string, ownerCredRef string) []string {
	env := make([]string, 0, len(base)+1)
Outer:
	for _, kv := range base {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if envScrubExact[name] {
			continue
		}
		for _, p := range envScrubPrefixes {
			if strings.HasPrefix(name, p) {
				continue Outer
			}
		}
		env = append(env, kv)
	}
	if ownerCredRef != "" {
		env = append(env, "CLAUDE_CONFIG_DIR="+ownerCredRef)
	}
	return env
}

// fingerprint is the S02.4(e) invocation-config fingerprint: settings
// hash + permission mode + model, extended over the full S03.4 park-record
// snapshot set (tool allowlist, agent body, system-prompt append) so any
// drifted re-supply is visible to the freshness pass (S02.6).
func fingerprint(settingsJSON []byte, req adapters.StartRequest, gateFallback string) string {
	h := sha256.New()
	for _, part := range [][]byte{
		settingsJSON,
		[]byte(req.Worker.PermissionMode),
		[]byte(req.Model),
		[]byte(strings.Join(req.Worker.ToolAllowlist, ",")),
		[]byte(strings.Join(req.Worker.GatedTools, ",")),
		req.Worker.AgentsJSON,
		[]byte(req.Worker.AgentName),
		[]byte(req.Worker.SystemPromptAppend),
		[]byte(gateFallback),
	} {
		h.Write(part)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

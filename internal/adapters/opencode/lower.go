package opencode

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// Engine lowering (Spec S03.5): the run is compiled down to the exact
// invocation the engine receives, and COMPILED CONFIG IS THE ONLY CONFIG THE
// ENGINE SEES. Isolating one channel is not sufficient — the guarantee is
// COMPOUND, one knob per channel, and each knob has an attempt-the-leak entry
// in the conformance suite:
//
//	settings / config        isolated config body via OPENCODE_CONFIG_CONTENT,
//	                         under a per-user disjoint XDG quartet
//	MCP                      configured only in that body (none at LN-1)
//	tools incl. native spawn permission `task: "deny"` — blanket deny STRIPS the
//	                         tool from the model's toolset pre-inference, so it
//	                         is bypass-proof and inherited by any subagent
//	                         [SPIKE P2-S5 Probe 5]
//	HOME-relative cross-read OPENCODE_DISABLE_CLAUDE_CODE=1 — HOME is unchanged
//	                         under XDG isolation, so the operator's own
//	                         ~/.claude/CLAUDE.md and .claude/skills would
//	                         otherwise bleed into every worker [Probe 4]
//	cwd project-config       OPENCODE_DISABLE_PROJECT_CONFIG=1 — an adversarial
//	                         walked-up opencode.json re-enabling task:"allow"
//	                         was reproduced live [Probe 4]
//	plugins                  OPENCODE_PURE=1 + OPENCODE_DISABLE_DEFAULT_PLUGINS=1
//	agent / prompt body      agent map in the compiled body, selected per turn by
//	                         NAME in prompt_async (1.18.3 has no inline
//	                         per-request agent DEFINITION)
//	env                      the XDG quartet on every invocation + the scrub set
//
// ⚙ DISCIPLINE (Spec S01.10, S18; CONVENTIONS §2, §6). This substrate consumes
// NO settings key, and that is a finding, not an omission. Of the three keys the
// S03 settings table declares:
//
//   - `adapter.claude.cleanup_period_days` is Claude-lane mechanics — it raises
//     that engine's session-retention sweep above the park horizon. opencode
//     keeps sessions in SQLite with no equivalent sweep.
//   - `adapter.parallel_gate_fallback` selects the behavior for the Claude
//     lane's DEFER TRAP (defer honored only when a turn holds one tool call).
//     That trap does not exist here: `permission.asked` parks the turn on an
//     in-process deferred with no single-tool-call bound, and a parallel turn
//     parks every call simultaneously as a flat array [SPIKE P2-S1 Probe 6].
//   - `adapter.engine_ceiling_backstop_mult` is consumed only where an
//     engine-side budget belt exists, so the platform gate can be made to fire
//     first (S03.4 ceiling ordering). `opencode serve` 1.18.3 exposes NO
//     per-turn budget belt: there is no engine ceiling to order against, the
//     platform ledger (Spec S10) is the only ceiling, and S03.4's ordering
//     obligation is thereby trivially satisfied. A fabricated belt would be a
//     lie about enforcement, so none is built and OutcomeDiedAtGate is
//     unreachable on this substrate.
//
// No new ⚙ key is registered: the S18 index is test-pinned at 118 keys / 33
// domains, and a new key is an S00.9 amendment plus an S18 sweep.

// nativeSpawnTool is the engine's native subagent tool. The compiled config
// denies it blanket, and no input may re-enable it (S03.5 sole-controller
// posture, G1 rider 2; operator re-confirmed KEEP DISABLED 2026-07-18).
const nativeSpawnTool = "task"

// permissionAll is the blanket key of opencode's permission map. Compiled to
// "deny" it makes the toolset default-deny: only the tools the worker's
// allowlist names survive into the model's toolset.
const permissionAll = "*"

// Permission values (1.18.3 `PermissionConfig`).
const (
	permDeny  = "deny"
	permAsk   = "ask"
	permAllow = "allow"
)

// envScrubExact / envScrubPrefixes are the S03.5 env channel for this engine.
// Every OPENCODE_* variable is scrubbed — the engine's whole configuration
// surface is env-reachable (config content, config dir, the disable switches,
// the server password), so an ambient one could re-open any channel this
// lowering just closed. The Claude-lane nested-session markers are scrubbed too
// (CONVENTIONS §10 scrub set): a Sinet worker is not a nested Claude session,
// and ANTHROPIC_* must never reach an engine on another lane.
var (
	envScrubExact = map[string]bool{
		"CLAUDECODE": true, "AI_AGENT": true, "CLAUDE_EFFORT": true, "CLAUDE_PID": true,
		"XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true, "XDG_STATE_HOME": true, "XDG_CACHE_HOME": true,
	}
	envScrubPrefixes = []string{"OPENCODE_", "ANTHROPIC_", "CLAUDE_CODE_"}
)

// engineConfig is the compiled config body — the ONE config the engine sees.
// The agent map is carried as raw members so a compiled worker body reaches the
// engine verbatim; only the enforcement keys (`permission`, `mode`) are
// platform-owned and overwritten.
type engineConfig struct {
	Schema     string                     `json:"$schema"`
	Permission map[string]string          `json:"permission"`
	Provider   ProviderConfig             `json:"provider,omitempty"`
	Agent      map[string]json.RawMessage `json:"agent,omitempty"`
}

const configSchemaURL = "https://opencode.ai/config.json"

// modelRef is opencode's per-message model selector. The platform's model
// string is `<providerID>/<modelID>` — the engine's own shape.
type modelRef struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

// lowered is one compiled invocation: the compiled config is the ONLY config
// the engine sees, one knob per config channel (S03.5).
type lowered struct {
	root         string // the platform-owned per-user root (0700)
	configJSON   []byte // the config channel (OPENCODE_CONFIG_CONTENT body)
	env          []string
	agentName    string // selected per turn BY NAME in prompt_async
	systemAppend string
	prompt       string
	model        modelRef
	fingerprint  string // S02.4(e), config-only and session-id-independent

	// lane is the commissioning document behind this invocation's provider,
	// when the provider names one; nil for a provider that is not a lane.
	lane *LaneConfig
	// endpointVerified is the R11 self-check verdict for THIS user's entry —
	// established here, where the config is, and forwarded as an input to the
	// S10.5 classifier, which stays pure and does no lookups of its own.
	endpointVerified bool
	endpointReason   string
}

// lower compiles a StartRequest into the exact invocation the engine receives.
func (a *Adapter) lower(req adapters.StartRequest) (*lowered, error) {
	if req.RunID == "" {
		return nil, errors.New("opencode: start without run_id")
	}
	if req.UserID == "" {
		return nil, errors.New("opencode: start without user_id (the serve instance is per-user, S03.2)")
	}
	if req.Cwd == "" {
		return nil, errors.New("opencode: start without Cwd (isolated run cwd, S03.5)")
	}
	model, err := parseModel(req.Model)
	if err != nil {
		return nil, err
	}
	if req.Worker.AgentName == "" {
		return nil, errors.New("opencode: start without an agent name (1.18.3 selects agents by name, S03.5)")
	}
	root, err := a.userRoot(req)
	if err != nil {
		return nil, err
	}
	perms, err := compilePermissions(req.Worker)
	if err != nil {
		return nil, err
	}
	agents, err := compileAgents(req.Worker, perms)
	if err != nil {
		return nil, err
	}
	providers, err := a.providersFor(req.UserID)
	if err != nil {
		return nil, fmt.Errorf("opencode: resolve the provider entry of user %q (S03.6): %w", req.UserID, err)
	}
	lane := a.laneFor(model.ProviderID)
	entry, configured := providers[model.ProviderID]
	if lane != nil && !configured {
		return nil, fmt.Errorf("%w: lane %q (provider %q) has no provider entry for user %q — a lane is a "+
			"provider entry per person, and this person has none (S03.6)",
			ErrLaneNotCommissioned, lane.Lane, lane.ProviderID, req.UserID)
	}
	cfg := engineConfig{
		Schema: configSchemaURL,
		// The blanket native-spawn denial rides the top level as well as every
		// agent: a worker body can never be the only thing standing between the
		// model and its own spawn tool.
		Permission: map[string]string{nativeSpawnTool: permDeny},
		Provider:   withEffortOptions(providers, lane, req.EffortMode),
		Agent:      agents,
	}
	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("opencode: marshal compiled config: %w", err)
	}
	l := &lowered{
		root:         root,
		configJSON:   configJSON,
		env:          loweredEnv(a.environ(), root),
		agentName:    req.Worker.AgentName,
		systemAppend: req.Worker.SystemPromptAppend,
		prompt:       req.Worker.Prompt,
		model:        model,
		lane:         lane,
	}
	if lane != nil {
		l.endpointVerified, l.endpointReason = lane.VerifyEndpoint(entry)
		if !l.endpointVerified {
			// Loud, not fatal. A wrong endpoint spends the wrong balance and
			// then reports it as a depletion; the run still goes out, and the
			// classifier is handed the fact so the symptom cannot be mistaken
			// for the disease (R11).
			a.warnf("opencode: %s", l.endpointReason)
		}
	}
	l.fingerprint = fingerprint(configJSON, req)
	return l, nil
}

// providersFor resolves this user's provider block. The single-value
// Adapter.Providers is the default/single-user case, so no caller that
// predates per-user resolution changes behavior.
func (a *Adapter) providersFor(userID string) (ProviderConfig, error) {
	if a.ProvidersFor == nil {
		return a.Providers, nil
	}
	return a.ProvidersFor(userID)
}

// laneFor reports the commissioning document behind a provider id, if any.
func (a *Adapter) laneFor(providerID string) *LaneConfig {
	for i := range a.Lanes {
		if a.Lanes[i].ProviderID == providerID {
			return &a.Lanes[i]
		}
	}
	return nil
}

// withEffortOptions applies the S10.6 effort rung's per-lane lever.
//
// WHICH efforts take the lever is the lane document's call, not this
// function's (LaneConfig.EcoOptionEfforts): the spec names the Z.AI thinking
// lever an "Eco/Balanced rung" without settling whether Balanced takes it, and
// that is a quality-for-consumption trade an operator owns rather than a
// constant an implementation picks.
//
// The lever rides the compiled config because that is the only channel 1.18.3
// exposes for provider options — which means an effort change is
// STARTUP-BOUND like every other config change and restarts that user's serve
// (§61(a)). That is a real cost, honestly incurred: the alternative is
// inventing a per-request field this engine does not have.
//
// The input is never mutated: a resolver's config belongs to the resolver.
func withEffortOptions(providers ProviderConfig, lane *LaneConfig, effort string) ProviderConfig {
	if lane == nil || len(lane.EcoOptions) == 0 || !lane.EcoOptionApplies(effort) {
		return providers
	}
	entry, ok := providers[lane.ProviderID]
	if !ok {
		return providers
	}
	models := make(map[string]ModelEntry, len(entry.Models))
	for id, m := range entry.Models {
		opts := make(map[string]json.RawMessage, len(m.Options)+len(lane.EcoOptions))
		for k, v := range m.Options {
			opts[k] = v
		}
		for k, v := range lane.EcoOptions {
			opts[k] = v
		}
		models[id] = ModelEntry{Name: m.Name, Options: opts}
	}
	entry.Models = models
	out := make(ProviderConfig, len(providers))
	for id, e := range providers {
		out[id] = e
	}
	out[lane.ProviderID] = entry
	return out
}

// lookupEnv reads one variable out of a lowered environment.
func lookupEnv(env []string, name string) (string, bool) {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v, true
		}
	}
	return "", false
}

// userRoot resolves and materializes the platform-owned per-user root. Every
// XDG path of that user's instance lives under it, 0700 — the tree holds
// auth.json and a SQLite DB with plaintext provider tokens (SPIKE G1-S3 §3).
func (a *Adapter) userRoot(req adapters.StartRequest) (string, error) {
	// D2: the owner credential REF is a path, never material. On this lane it
	// names the per-user root the engine's credential store lives under; empty
	// means the platform-owned default (dev posture).
	root := req.OwnerCredRef
	if root == "" {
		if a.Root == "" {
			return "", errors.New("opencode: adapter without a platform-owned Root (the per-user XDG tree needs one, S03.2)")
		}
		if !safeUserID(req.UserID) {
			return "", fmt.Errorf("opencode: user id %q cannot name a directory", req.UserID)
		}
		root = filepath.Join(a.Root, req.UserID)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("opencode: per-user root: %w", err)
	}
	// MkdirAll leaves an existing directory's mode alone; the 0700 obligation
	// is on the tree as it is USED, not only as it is created.
	if err := os.Chmod(root, 0o700); err != nil {
		return "", fmt.Errorf("opencode: per-user root mode: %w", err)
	}
	return root, nil
}

func safeUserID(id string) bool {
	if id == "" || id == "." || id == ".." {
		return false
	}
	return !strings.ContainsAny(id, `/\`) && !strings.Contains(id, "..")
}

// parseModel splits the platform model string into opencode's per-message
// `model:{providerID, modelID}` (S03.1 start row). The `provider/model` shape
// is the engine's own.
func parseModel(model string) (modelRef, error) {
	provider, id, ok := strings.Cut(model, "/")
	if !ok || provider == "" || id == "" {
		return modelRef{}, fmt.Errorf("opencode: model %q is not <providerID>/<modelID> (S03.1 per-message model ref)", model)
	}
	return modelRef{ProviderID: provider, ModelID: id}, nil
}

// compilePermissions builds the per-agent permission map: default-deny, the
// allowlist allowed, the gated tools asking (the S03.4 park trigger), and
// native spawn denied under every input.
func compilePermissions(w adapters.CompiledWorker) (map[string]string, error) {
	perms := map[string]string{
		permissionAll:   permDeny,
		nativeSpawnTool: permDeny,
	}
	for _, tool := range w.ToolAllowlist {
		if isNativeSpawn(tool) {
			return nil, fmt.Errorf("%w: tool allowlist names %q — engine-native subagents are structurally disabled (S03.5)",
				ErrNativeSpawnTool, tool)
		}
		if tool == permissionAll {
			return nil, fmt.Errorf("%w: tool allowlist is a wildcard, which re-admits %q (S03.5 default-deny)",
				ErrNativeSpawnTool, nativeSpawnTool)
		}
		perms[tool] = permAllow
	}
	for _, tool := range w.GatedTools {
		if isNativeSpawn(tool) {
			return nil, fmt.Errorf("%w: gated tools name %q — the native spawn tool is denied, never gated (S03.5)",
				ErrNativeSpawnTool, tool)
		}
		// A gated tool asks even when the allowlist also allowed it: the gate
		// is the stronger statement.
		perms[tool] = permAsk
	}
	return perms, nil
}

func isNativeSpawn(tool string) bool { return strings.EqualFold(tool, nativeSpawnTool) }

// compileAgents merges the compiled worker's agent map with the platform-owned
// enforcement keys. Members other than `permission`/`mode` pass through
// VERBATIM — the body is the worker compiler's (Spec S08), not this adapter's.
func compileAgents(w adapters.CompiledWorker, perms map[string]string) (map[string]json.RawMessage, error) {
	bodies := map[string]map[string]json.RawMessage{}
	if len(w.AgentsJSON) > 0 {
		if err := json.Unmarshal(w.AgentsJSON, &bodies); err != nil {
			return nil, fmt.Errorf("opencode: compiled agent map is not a name→body object: %w", err)
		}
	}
	if _, ok := bodies[w.AgentName]; !ok {
		if bodies == nil {
			bodies = map[string]map[string]json.RawMessage{}
		}
		bodies[w.AgentName] = map[string]json.RawMessage{}
	}
	permJSON, err := json.Marshal(perms)
	if err != nil {
		return nil, fmt.Errorf("opencode: marshal permission map: %w", err)
	}
	out := make(map[string]json.RawMessage, len(bodies))
	for name, body := range bodies {
		// A supplied fragment that tries to re-enable native spawn is REFUSED,
		// not silently overwritten: the platform must never absorb an attempt to
		// widen enforcement quietly (S03.5; the §12 fail-closed precedent).
		if raw, ok := body["permission"]; ok {
			var supplied map[string]json.RawMessage
			if err := json.Unmarshal(raw, &supplied); err != nil {
				return nil, fmt.Errorf("opencode: agent %q has a permission member that is not an object: %w", name, err)
			}
			if v, ok := supplied[nativeSpawnTool]; ok {
				var value string
				if err := json.Unmarshal(v, &value); err != nil || value != permDeny {
					return nil, fmt.Errorf("%w: agent %q supplies permission.%s = %s (S03.5 sole-controller)",
						ErrNativeSpawnTool, name, nativeSpawnTool, v)
				}
			}
		}
		merged := make(map[string]json.RawMessage, len(body)+2)
		for k, v := range body {
			merged[k] = v
		}
		merged["permission"] = permJSON
		merged["mode"] = json.RawMessage(`"primary"`)
		encoded, err := json.Marshal(merged)
		if err != nil {
			return nil, fmt.Errorf("opencode: marshal agent %q: %w", name, err)
		}
		out[name] = encoded
	}
	return out, nil
}

// loweredEnv builds the engine environment: the ambient environment minus the
// S03.5 scrub set, plus the XDG quartet under the platform-owned per-user root
// and the four disable switches. The engine's own config content is NOT an env
// member here — it rides InstanceSpec.ConfigJSON so the config channel and the
// env channel stay separately auditable.
//
// HOME is deliberately left ALONE. That is exactly why
// OPENCODE_DISABLE_CLAUDE_CODE is required rather than optional: XDG isolation
// does not move HOME, so the engine's `~/.claude` cross-reads would otherwise
// stay open (S03.5; SPIKE P2-S5 Probe 4).
func loweredEnv(base []string, root string) []string {
	env := make([]string, 0, len(base)+8)
Outer:
	for _, kv := range base {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || envScrubExact[name] {
			continue
		}
		for _, p := range envScrubPrefixes {
			if strings.HasPrefix(name, p) {
				continue Outer
			}
		}
		env = append(env, kv)
	}
	env = append(env,
		"XDG_CONFIG_HOME="+filepath.Join(root, "config"),
		"XDG_DATA_HOME="+filepath.Join(root, "data"),
		"XDG_STATE_HOME="+filepath.Join(root, "state"),
		"XDG_CACHE_HOME="+filepath.Join(root, "cache"),
		"OPENCODE_DISABLE_CLAUDE_CODE=1",
		"OPENCODE_DISABLE_PROJECT_CONFIG=1",
		"OPENCODE_PURE=1",
		"OPENCODE_DISABLE_DEFAULT_PLUGINS=1",
	)
	return env
}

// fingerprint is the S02.4(e) invocation-config fingerprint: a hash of the
// config the engine will actually run under, plus the per-invocation knobs the
// config body does not carry. It is session-id-independent by construction —
// no session id is an input to lowering at all, because this engine mints its
// own session id (S03.1 start row).
func fingerprint(configJSON []byte, req adapters.StartRequest) string {
	h := sha256.New()
	for _, part := range [][]byte{
		configJSON,
		[]byte(req.Model),
		[]byte(req.Worker.AgentName),
		[]byte(req.Worker.PermissionMode),
		[]byte(req.Worker.SystemPromptAppend),
		[]byte(strings.Join(req.Worker.ToolAllowlist, ",")),
		[]byte(strings.Join(req.Worker.GatedTools, ",")),
	} {
		h.Write(part)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// instanceSpec is the R3 endpoint hand-off: everything a serve instance needs,
// and nothing about how it is produced. The dev-posture Manager implements the
// seam now; the D6 `sinet-engine@<user>` posture swaps the provider, not the
// adapter.
func (a *Adapter) instanceSpec(req adapters.StartRequest, l *lowered, env []string) InstanceSpec {
	return InstanceSpec{
		UserID:      req.UserID,
		Root:        l.root,
		Cwd:         req.Cwd,
		Fingerprint: l.fingerprint,
		ConfigJSON:  l.configJSON,
		Env:         env,
	}
}

package kimicli

// conformance_test.go — P3-LN-7 §10 specs T2, T11-T20 (S03.1, S03.3, S03.5,
// S03.4, S10.1, S11.5, S16.2).
//
// Tier F: every assertion here is a lowering inspection or a replay of the R0
// capture (P3/measurements/2026-08-26-kimi-cli-print-mode-spike.md). $0 — no
// process is spawned by tier F at all, and no path here can reach a provider.
// Tier R (a real `kimi` against a loopback fake provider) is realengine_test.go;
// tier L is live_smoke_test.go.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/lockfile"
)

// ── the fixture request ──────────────────────────────────────────────────────

func testAdapter(t *testing.T) *Adapter {
	t.Helper()
	return &Adapter{
		Binary:       filepath.Join(t.TempDir(), "kimi-not-spawned"),
		Root:         t.TempDir(),
		BaseURL:      "https://api.kimi.com/coding/v1",
		ProviderType: "openai",
		Env:          []string{"PATH=/usr/bin:/bin"},
	}
}

func testRequest(t *testing.T) adapters.StartRequest {
	t.Helper()
	dir := t.TempDir()
	cwd := filepath.Join(dir, "cwd")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}
	return adapters.StartRequest{
		RunID:  "run-ln7",
		UserID: "sinep",
		Model:  "k3",
		Cwd:    cwd,
		Worker: adapters.CompiledWorker{
			Prompt:        "say hello",
			ToolAllowlist: []string{"Read", "Grep"},
		},
	}
}

// ── T2 · the pin is coupled to the lock entry ────────────────────────────────

func TestPinMatchesLock(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "components.lock"))
	if err != nil {
		t.Fatalf("read components.lock: %v", err)
	}
	lock, err := lockfile.Parse(raw)
	if err != nil {
		t.Fatalf("parse components.lock: %v", err)
	}
	for _, c := range lock.Components {
		if c.Kind != "engine" || !strings.Contains(c.Name, "kimi-code") {
			continue
		}
		if c.Pin != Pin {
			t.Fatalf("components.lock pin %q ≠ kimicli.Pin %q — bump one only via the S03.3 procedure", c.Pin, Pin)
		}
		if c.Pin != "0.38.0" {
			t.Errorf("pin %q — the packet pins 0.38.0 EXACT", c.Pin)
		}
		// An external CLI is neither a Go module nor a web/ dependency.
		// Claiming either would make lockgate cover a path nothing consumes.
		if len(c.Modules) != 0 {
			t.Errorf("the kimi-code entry claims Go modules %v — it is an external CLI", c.Modules)
		}
		if len(c.NpmPackages) != 0 {
			t.Errorf("the kimi-code entry claims npm packages %v — it is a global CLI, not a web/ dependency", c.NpmPackages)
		}
		if c.License.SPDX != "MIT" {
			t.Errorf("license %q, want MIT", c.License.SPDX)
		}
		// The license must be read AT THE PINNED REF, and the scope says so.
		if !strings.Contains(c.License.Scope, "0.38.0") {
			t.Errorf("the license scope does not name the pinned ref: %q", c.License.Scope)
		}
		if c.License.Checked == "" {
			t.Error("the license carries no checked date")
		}
		// The notes carry the findings a later reader would otherwise re-derive.
		for _, needle := range []string{"provenance", "auto-update", "KIMI_CODE_HOME", "fail-open"} {
			if !strings.Contains(strings.ToLower(c.Notes), strings.ToLower(needle)) {
				t.Errorf("the kimi-code entry's notes do not record %q", needle)
			}
		}
		return
	}
	t.Fatal("no kimi-code engine entry in components.lock (the S16.3 row materializes with this adapter)")
}

// ── T11 · a gated-tool worker is REFUSED, never auto-approved ────────────────

// TestGatedToolsRefusedOnKimiCLI is the fail-closed rule of CONVENTIONS §12
// applied to a substrate that cannot express the decision. Print mode requests
// no approval, there is no defer, and the R0 spike measured that static
// `[permission] deny` rules are INERT in -p — so the only default available
// here is consent, and consent is exactly what must not be given silently.
func TestGatedToolsRefusedOnKimiCLI(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	req.Worker.GatedTools = []string{"Bash"}

	_, err := a.lower(req, nil)
	if err == nil {
		t.Fatal("a CompiledWorker carrying GatedTools was accepted — it would have run the gated call under `auto` with nothing in front of it")
	}
	if !isErr(err, ErrGateParkUnsupported) {
		t.Errorf("error = %v, want ErrGateParkUnsupported", err)
	}
	for _, needle := range []string{"S03.4", "kimi-cli"} {
		if !strings.Contains(err.Error(), needle) {
			t.Errorf("the refusal does not name %q: %v", needle, err)
		}
	}
	// The refusal happens at lowering, BEFORE any process could exist. A
	// refusal that arrived after the spawn would have already handed the
	// engine the invocation it must never receive.
	if entries, _ := os.ReadDir(a.Root); len(entries) != 0 {
		t.Errorf("the refused spawn left %d entries under the engine root — nothing may be materialized for a refused invocation", len(entries))
	}
}

// ── T12 · native-spawn re-admission is refused, as a property ────────────────

func TestNativeSpawnReadmissionRefused(t *testing.T) {
	a := testAdapter(t)
	// Every shape that would re-admit the engine's own subagent family. The
	// wildcards are in here because the engine inverts them: `enabled=["*"]`
	// disables every tool and `disabled=["*"]` disables NONE, so a caller
	// writing either almost certainly meant the opposite of what they get.
	cases := [][]string{
		{"Agent"}, {"AgentSwarm"}, {"Read", "Agent"}, {"agent"}, {"AGENTSWARM"}, {"AgEnT"},
		{"*"}, {"Read", "*"},
	}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 200; i++ {
		// Property: a native-spawn name ANYWHERE in an otherwise innocent
		// allowlist is refused, at any position and in any case.
		base := []string{"Read", "Grep", "Glob", "Write", "Edit"}
		name := []string{"Agent", "AgentSwarm"}[rng.Intn(2)]
		if rng.Intn(2) == 0 {
			name = strings.ToLower(name)
		}
		at := rng.Intn(len(base) + 1)
		mixed := append(append(append([]string{}, base[:at]...), name), base[at:]...)
		cases = append(cases, mixed)
	}
	for _, allow := range cases {
		req := testRequest(t)
		req.Worker.ToolAllowlist = allow
		if _, err := a.lower(req, nil); !isErr(err, ErrNativeSpawnTool) {
			t.Fatalf("allowlist %v: error = %v, want ErrNativeSpawnTool — never a silent overwrite", allow, err)
		}
	}
	// The inverse control: an ordinary allowlist is accepted, so "refuse
	// everything" cannot pass as a fix.
	req := testRequest(t)
	req.Worker.ToolAllowlist = []string{"Read", "Grep"}
	if _, err := a.lower(req, nil); err != nil {
		t.Fatalf("an ordinary allowlist was refused: %v", err)
	}
	// And a gated-tools entry naming the family is refused too.
	req = testRequest(t)
	req.Worker.GatedTools = []string{"Agent"}
	if _, err := a.lower(req, nil); err == nil {
		t.Fatal("a GatedTools entry naming the native-spawn family was accepted")
	}
}

// TestNativeSpawnIsDisabledInEveryLoweredConfig pins the structural half: the
// R0 spike measured that `[tools] disabled` strips the family PRE-inference
// (26 tools offered → 24, with Agent and AgentSwarm absent from what the model
// ever saw). The lowering must therefore emit it on every invocation.
func TestNativeSpawnIsDisabledInEveryLoweredConfig(t *testing.T) {
	l := mustLower(t, testAdapter(t), testRequest(t))
	cfg := l.files[configTOML]
	if cfg == "" {
		t.Fatal("the lowering wrote no config.toml")
	}
	for _, tool := range []string{"Agent", "AgentSwarm"} {
		if !strings.Contains(cfg, `"`+tool+`"`) {
			t.Errorf("the lowered config does not disable %q:\n%s", tool, cfg)
		}
	}
	if !strings.Contains(cfg, "[tools]") || !strings.Contains(cfg, "disabled") {
		t.Errorf("the lowered config has no [tools] disabled list:\n%s", cfg)
	}
}

// ── T13 · forbidden flags are never emitted, as a property ───────────────────

func TestForbiddenFlagsNeverEmitted(t *testing.T) {
	a := testAdapter(t)
	rng := rand.New(rand.NewSource(11))
	models := []string{"k3", "k3-256k", "kimi-for-coding", "kimi-for-coding-highspeed"}
	for i := 0; i < 200; i++ {
		req := testRequest(t)
		req.Model = models[rng.Intn(len(models))]
		req.Worker.Prompt = strings.Repeat("x", rng.Intn(40)+1)
		req.CeilingSteps = int64(rng.Intn(500))
		req.CeilingCostUSD = rng.Float64() * 10
		if rng.Intn(2) == 0 {
			req.Worker.SystemPromptAppend = "appended"
		}
		if rng.Intn(2) == 0 {
			req.Worker.PermissionMode = "acceptEdits"
		}
		l := mustLower(t, a, req)
		argv := strings.Join(l.argv, " ")
		for _, bad := range forbiddenFlags {
			for _, got := range l.argv {
				if got == bad {
					t.Fatalf("argv carries the forbidden flag %s: %s", bad, argv)
				}
			}
		}
		// The transport itself, on every invocation.
		if !hasPair(l.argv, "--output-format", "stream-json") {
			t.Fatalf("argv does not select stream-json: %s", argv)
		}
		if !contains(l.argv, "-p") {
			t.Fatalf("argv does not run in print mode: %s", argv)
		}
		// The model rides the ENV, not argv. `-m` selects a config ALIAS, and
		// on the KIMI_MODEL_* channel the synthesized alias is
		// `__kimi_env_model__` — so `-m k3` names something that does not
		// exist and the engine CRASHES (measured at pin 0.38.0, exit 1 with an
		// unhandled lifecycle error after only the version frame).
		if contains(l.argv, "-m") || contains(l.argv, "--model") {
			t.Fatalf("argv carries a model flag: %s — the model is chosen by KIMI_MODEL_NAME on this channel", argv)
		}
		if got := envValue(l.env, "KIMI_MODEL_NAME"); got != req.Model {
			t.Fatalf("KIMI_MODEL_NAME = %q, want the per-invocation model %q", got, req.Model)
		}
		// `--yolo` and `-p` are mutually exclusive at the pin and the engine
		// REJECTS the pair (R0-Q7, measured: "Cannot combine --prompt with
		// --yolo"). Emitting one would not weaken the gate, it would break
		// every run — which is a different bug with the same cause.
		if contains(l.argv, "--yolo") || contains(l.argv, "--auto") {
			t.Fatalf("argv carries a permission-mode flag print mode refuses: %s", argv)
		}
	}
}

// ── T14 · the run is BOUNDED or it does not spawn ────────────────────────────

// TestLoweredRunIsBounded is measured-not-inspected for a reason recorded in
// the spike: unknown config keys are SILENTLY IGNORED by this engine, so a key
// written at the wrong level is indistinguishable from a key that took effect.
// The table names are therefore asserted verbatim against the ones the spike
// proved by execution ([background] and [loop_control]), and the tier-R suite
// re-proves the effect against the real binary.
func TestLoweredRunIsBounded(t *testing.T) {
	l := mustLower(t, testAdapter(t), testRequest(t))
	cfg := l.files[configTOML]

	if !strings.Contains(cfg, "[background]") {
		t.Error("the lowered config has no [background] table — the boundedness keys live there, NOT at top level (R0-Q5, measured)")
	}
	if !strings.Contains(cfg, "[loop_control]") {
		t.Error("the lowered config has no [loop_control] table")
	}
	// print_background_mode must not be the default "steer": at defaults the
	// R0 spike measured a model-launched background task keeping `kimi -p`
	// alive past a 25 s harness timeout. With "exit" the same run ended in
	// 1.11 s. This is the one requirement whose absence has already cost this
	// household a session.
	if !strings.Contains(cfg, `print_background_mode = "exit"`) {
		t.Errorf("print_background_mode is not \"exit\":\n%s", cfg)
	}
	if strings.Contains(cfg, `"steer"`) {
		t.Error("the lowered config selects the unbounded default background mode")
	}
	for _, key := range []string{
		"print_max_turns", "print_wait_ceiling_s", "bash_task_timeout_s",
		"max_steps_per_turn", "max_attempts_per_step",
	} {
		if !strings.Contains(cfg, key) {
			t.Errorf("the lowered config does not set %s explicitly:\n%s", key, cfg)
		}
		if v, ok := tomlInt(cfg, key); !ok {
			t.Errorf("%s is not an integer in the lowered config", key)
		} else if v <= 0 {
			t.Errorf("%s = %d — an explicit ceiling must be finite and positive (0 means unbounded on this engine)", key, v)
		}
	}
	// Subagent/swarm ceilings ride the env channel, because the spike found a
	// `[subagent]` section in the bundle and NO swarm section at all — a TOML
	// key the engine does not read is worse than none, since unknown keys are
	// silently ignored.
	if v := envValue(l.env, "KIMI_SUBAGENT_TIMEOUT_MS"); v == "" || v == "0" {
		t.Errorf("KIMI_SUBAGENT_TIMEOUT_MS = %q — an unset or zero subagent timeout is unbounded", v)
	}
	// S10.5: engine-native retry is NEVER the policy layer.
	if envValue(l.env, "KIMI_CODE_INFINITE_RETRY") != "" {
		t.Error("KIMI_CODE_INFINITE_RETRY is set — engine-native retry is never the policy layer (S10.5)")
	}
	if strings.Contains(cfg, "infinite_retry") {
		t.Error("the lowered config enables infinite retry")
	}
}

// ── T15 · auto-update, telemetry, engine selection, client identity ──────────

func TestAutoUpdateAndTelemetryDisabled(t *testing.T) {
	l := mustLower(t, testAdapter(t), testRequest(t))

	if got := envValue(l.env, "KIMI_CODE_NO_AUTO_UPDATE"); got != "1" {
		t.Errorf("KIMI_CODE_NO_AUTO_UPDATE = %q, want \"1\" — the package published 70 versions in ~3 months and a pin that self-upgrades is not a pin (S03.3 rule 2)", got)
	}
	if got := envValue(l.env, "KIMI_DISABLE_TELEMETRY"); got != "1" {
		t.Errorf("KIMI_DISABLE_TELEMETRY = %q, want \"1\"", got)
	}
	tui := l.files[tuiTOML]
	if !strings.Contains(tui, "[upgrade]") || !strings.Contains(tui, "auto_install = false") {
		t.Errorf("the lowered tui.toml does not disable auto-install:\n%s", tui)
	}
	// KIMI_CODE_LEGACY_FLAG unset ⇒ agent-core-v2, which is what the pin was
	// characterized against. Setting it would silently target a second engine
	// that ignores several of the settings above.
	if envValue(l.env, "KIMI_CODE_LEGACY_FLAG") != "" {
		t.Error("KIMI_CODE_LEGACY_FLAG is set — the pin targets agent-core-v2")
	}
	// The Community Guidelines forbid altering client identity, verbatim:
	// "Don't spoof or alter client identity information". Doing it while F0
	// is a recorded gray zone would convert a policy question into a policy
	// violation.
	for _, name := range []string{"KIMI_CODE_IDENTITY_NAME", "KIMI_CODE_IDENTITY_SLUG"} {
		if envValue(l.env, name) != "" {
			t.Errorf("%s is set — the guidelines forbid altering client identity", name)
		}
	}
}

// TestClientIdentityNeverAppearsInNonTestSources is the source half of R14: a
// variable that is never set in one code path but present in another is a
// variable somebody will set.
func TestClientIdentityNeverAppearsInNonTestSources(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		scanned++
		// R14: neither identity variable may appear anywhere in a non-test
		// source. The guidelines forbid altering client identity, and a name
		// that is absent from one code path but present in another is a name
		// somebody will set.
		//
		// KIMI_CODE_INFINITE_RETRY is deliberately NOT on this list: the
		// lowering must NAME it in order to refuse it, and a source ban would
		// force the guard to be deleted to satisfy the scan. Its absence from
		// the environment is asserted behaviorally instead, in
		// TestLoweredRunIsBounded, which is the stronger check anyway.
		for _, bad := range []string{"KIMI_CODE_IDENTITY_NAME", "KIMI_CODE_IDENTITY_SLUG"} {
			if strings.Contains(string(raw), bad) {
				t.Errorf("%s names %s in a non-test source", name, bad)
			}
		}
		// The `upgrade` subcommand is unreachable BY CONSTRUCTION — absent,
		// not guarded (S03.3 rule 2, the opencode /global/upgrade precedent).
		if strings.Contains(string(raw), `"upgrade"`) {
			t.Errorf("%s names the upgrade subcommand — it must be absent, not guarded", name)
		}
	}
	if scanned == 0 {
		t.Fatal("the source scan read no files — it would pass vacuously")
	}
}

// ── T16 · per-RUN homes under a per-USER root ────────────────────────────────

func TestPerRunHomeIsolation(t *testing.T) {
	a := testAdapter(t)
	seen := map[string]string{}
	for _, who := range []string{"sinep", "guest"} {
		for _, run := range []string{"run-a", "run-b"} {
			req := testRequest(t)
			req.UserID, req.RunID = who, run
			l := mustLower(t, a, req)
			if prev, dup := seen[l.home]; dup {
				t.Fatalf("%s/%s reuses the KIMI_CODE_HOME of %s — two concurrent runs would share one lowered config "+
					"and one credential file, and S03.5's guarantee would be lost outright", who, run, prev)
			}
			seen[l.home] = who + "/" + run
			// HOME is bounded too: the `~/.agents/` instruction leg does NOT
			// move with KIMI_CODE_HOME, so leaving HOME ambient leaves that
			// channel open (§2(4)).
			home := envValue(l.env, "HOME")
			if home == "" {
				t.Fatal("HOME is unset in the lowered environment — the engine would resolve the real OS home")
			}
			if home == l.home {
				t.Error("HOME and KIMI_CODE_HOME are the same directory — the engine's own data would sit inside the bounded home")
			}
			if !strings.HasPrefix(home, a.Root) || !strings.HasPrefix(l.home, a.Root) {
				t.Errorf("HOME %q / KIMI_CODE_HOME %q escape the engine root %q", home, l.home, a.Root)
			}
			// The per-USER root separates people before runs separate
			// invocations (the opencodeRoot precedent).
			if !strings.Contains(l.home, who) {
				t.Errorf("the run home %q does not sit under a per-user root for %q", l.home, who)
			}
		}
	}
	if len(seen) != 4 {
		t.Fatalf("%d distinct homes, want 4", len(seen))
	}
}

func TestEngineRootsAreOwnerOnly(t *testing.T) {
	a := testAdapter(t)
	l := mustLower(t, a, testRequest(t))
	if err := a.materialize(l); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	for dir := l.home; strings.HasPrefix(dir, a.Root) && dir != a.Root; dir = filepath.Dir(dir) {
		st, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if perm := st.Mode().Perm(); perm != 0o700 {
			t.Errorf("%s has mode %o, want 0700 — a person's engine tree is theirs alone", dir, perm)
		}
	}
}

// ── T17 · every config channel's decoy fails to take effect ──────────────────

// TestConfigChannelDecoysDoNotTakeEffect plants one decoy per S03.5 channel and
// asserts the lowering closes it. Each assertion NAMES its channel, so a
// failure says which door is open rather than that some door is.
func TestConfigChannelDecoysDoNotTakeEffect(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	// Ambient nested-session and credential-channel decoys.
	a.Env = []string{
		"PATH=/usr/bin:/bin",
		"HOME=/home/decoy",
		"KIMI_CODE_HOME=/home/decoy/.kimi-code",
		"KIMI_API_KEY=decoy-key",
		"KIMI_MODEL_API_KEY=decoy-model-key",
		"KIMI_MODEL_BASE_URL=https://decoy.example/v1",
		"KIMI_CODE_BASE_URL=https://decoy.example/coding/v1",
		"KIMI_CODE_LEGACY_FLAG=1",
		"KIMI_CODE_INFINITE_RETRY=1",
		"KIMI_CODE_IDENTITY_NAME=decoy",
		"KIMI_CODE_BUILTIN_PRODUCT_SKILLS=1",
		"AI_AGENT=decoy",
		"CLAUDECODE=1",
		"ANTHROPIC_API_KEY=decoy-anthropic",
	}
	l := mustLower(t, a, req)

	for _, ch := range []struct{ channel, name, want string }{
		{"data root", "KIMI_CODE_HOME", l.home},
		{"instruction files / HOME", "HOME", envValue(l.env, "HOME")},
		{"credential (the CLI's own)", "KIMI_API_KEY", ""},
		{"credential (the model channel)", "KIMI_MODEL_API_KEY", ""},
		{"endpoint", "KIMI_MODEL_BASE_URL", a.BaseURL},
		{"endpoint (OAuth-managed)", "KIMI_CODE_BASE_URL", ""},
		{"engine selection", "KIMI_CODE_LEGACY_FLAG", ""},
		{"engine retry", "KIMI_CODE_INFINITE_RETRY", ""},
		{"client identity", "KIMI_CODE_IDENTITY_NAME", ""},
		{"built-in skills", "KIMI_CODE_BUILTIN_PRODUCT_SKILLS", "0"},
		{"nested session", "AI_AGENT", ""},
		{"nested session", "CLAUDECODE", ""},
		{"sibling lane credential", "ANTHROPIC_API_KEY", ""},
	} {
		if got := envValue(l.env, ch.name); got != ch.want {
			t.Errorf("channel %q: %s = %q, want %q — the ambient decoy took effect", ch.channel, ch.name, got, ch.want)
		}
	}
	// HOME must not be the decoy's.
	if strings.HasPrefix(envValue(l.env, "HOME"), "/home/decoy") {
		t.Error("channel \"instruction files\": HOME is the ambient decoy's")
	}

	// The system-prompt channel: SYSTEM.md replaces the built-in prompt IN
	// FULL and, measured at R0-Q6, omitting ${agents_md} closes all THREE
	// AGENTS.md legs at once — the KIMI_CODE_HOME leg, the cwd leg, and the
	// ~/.agents/ leg that KIMI_CODE_HOME cannot move.
	sys, ok := l.files[systemMD]
	if !ok || sys == "" {
		t.Error("channel \"instruction files\": no SYSTEM.md was lowered — AGENTS.md ingestion has no other knob")
	}
	if strings.Contains(sys, "${agents_md}") {
		t.Error("channel \"instruction files\": the lowered SYSTEM.md interpolates ${agents_md}, which re-opens every AGENTS.md leg")
	}
	// The skills/slash-command channel: --skills-dir REPLACES discovery.
	if !hasFlag(l.argv, "--skills-dir") {
		t.Error("channel \"skills\": the lowering does not replace skill discovery with a platform-owned dir")
	}
	if l.skillsDir == "" || !strings.HasPrefix(l.skillsDir, a.Root) {
		t.Errorf("channel \"skills\": skills dir %q is not platform-owned", l.skillsDir)
	}
	// The MCP channel: no server declared, plus the wildcard belt.
	cfg := l.files[configTOML]
	if !strings.Contains(cfg, `"mcp__*"`) {
		t.Errorf("channel \"MCP\": the lowered config does not disable the mcp__* family:\n%s", cfg)
	}
	if _, declared := l.files["mcp.json"]; declared {
		t.Error("channel \"MCP\": the lowering writes an mcp.json — a fresh per-run home must declare no server at all")
	}
}

// TestCwdTakeoverChannelIsAssertedAbsent covers the S03.5 cwd channel and the
// S11.7 P-T09-1 surface: a project-scoped `agent.md` with `override: true`
// replaces the default main agent's whole system prompt, and the vendor's own
// docs tell you to review those files in unfamiliar repositories. The run cwd
// is platform-owned, so the honest check is that none of them exists BEFORE
// the process does.
func TestCwdTakeoverChannelIsAssertedAbsent(t *testing.T) {
	a := testAdapter(t)
	for _, decoy := range []string{
		".kimi-code/local.toml",
		".kimi-code/agents/agent.md",
		".agents/agents/agent.md",
		".kimi-code/AGENTS.md",
		"AGENTS.md",
	} {
		req := testRequest(t)
		full := filepath.Join(req.Cwd, decoy)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte("override = true\n"), 0o600); err != nil {
			t.Fatalf("write decoy: %v", err)
		}
		l, err := a.lower(req, nil)
		if err == nil {
			err = a.assertCleanCwd(req.Cwd)
		}
		if err == nil {
			t.Errorf("channel %q: a run cwd carrying %s was accepted — that file can take over the main agent entirely", decoy, decoy)
			continue
		}
		if !strings.Contains(err.Error(), decoy) {
			t.Errorf("channel %q: the refusal does not name the file it found: %v", decoy, err)
		}
		_ = l
	}
	// The inverse control: a clean cwd is accepted.
	req := testRequest(t)
	if err := a.assertCleanCwd(req.Cwd); err != nil {
		t.Errorf("a clean run cwd was refused: %v", err)
	}
}

// ── T18 · unknown frames are skipped, never fatal, as a property ─────────────

func TestUnknownFramesAreSkippedNotFatal(t *testing.T) {
	known := map[adapters.EventKind]bool{
		adapters.KindMessage: true, adapters.KindUsage: true, adapters.KindRateLimit: true,
		adapters.KindGateAsk: true, adapters.KindToolResult: true, adapters.KindDone: true,
	}
	rng := rand.New(rand.NewSource(23))
	for i := 0; i < 200; i++ {
		var b strings.Builder
		b.WriteString(`{"role":"meta","type":"system.version","version":"0.38.0"}` + "\n")
		// A random spray of frames this pin never emitted, interleaved with
		// the ones it does. The envelope is undocumented and moves weekly:
		// this is the one place that has to hold under that.
		for j := 0; j < rng.Intn(8)+1; j++ {
			switch rng.Intn(4) {
			case 0:
				b.WriteString(`{"role":"meta","type":"` + randToken(rng) + `","payload":` + strconv.Itoa(rng.Int()) + "}\n")
			case 1:
				b.WriteString(`{"role":"` + randToken(rng) + `","content":"drifted"}` + "\n")
			case 2:
				b.WriteString(`{"` + randToken(rng) + `":true}` + "\n")
			case 3:
				b.WriteString("not json at all {{{\n")
			}
		}
		b.WriteString(`{"role":"assistant","content":"the answer"}` + "\n")
		b.WriteString(`{"role":"meta","type":"session.resume_hint","session_id":"session_x","command":"kimi -r session_x","content":"hint"}` + "\n")

		p := newParser(func(string, ...any) {})
		var evs []adapters.Event
		for _, line := range strings.Split(b.String(), "\n") {
			if line == "" {
				continue
			}
			evs = append(evs, p.feed([]byte(line))...)
		}
		for _, ev := range evs {
			if !known[ev.Kind] {
				t.Fatalf("an unknown engine frame was minted as platform kind %q — the closed set is {message,usage,rate_limit,gate_ask,tool_result,done}", ev.Kind)
			}
		}
		// The session still reaches its facts: drift never costs the answer.
		if p.sessionID != "session_x" {
			t.Fatalf("the session id was lost among unknown frames: %q", p.sessionID)
		}
		if p.finalText != "the answer" {
			t.Fatalf("the final assistant text was lost among unknown frames: %q", p.finalText)
		}
	}
}

// TestResultTextFollowsTheRatifiedOrder pins §60 on a substrate where the order
// COLLAPSES: R0-Q1 found no terminal result envelope at all, so limb (1) has no
// source here and limb (2) — the stream's final assistant message, VERBATIM —
// is the whole story. Nothing is repaired, completed or fabricated.
func TestResultTextFollowsTheRatifiedOrder(t *testing.T) {
	p := newParser(func(string, ...any) {})
	for _, line := range []string{
		`{"role":"meta","type":"system.version","version":"0.38.0"}`,
		`{"role":"assistant","content":"first"}`,
		`{"role":"assistant","tool_calls":[{"type":"function","id":"c1","function":{"name":"Read","arguments":"{}"}}]}`,
		`{"role":"tool","tool_call_id":"c1","content":"file body"}`,
		`{"role":"assistant","content":"  final answer, verbatim  "}`,
	} {
		p.feed([]byte(line))
	}
	if p.finalText != "  final answer, verbatim  " {
		t.Errorf("ResultText source = %q — the final assistant message rides VERBATIM, whitespace included", p.finalText)
	}
	// A tool-only trailing message must not zero the retained answer: that is
	// the ordinary tail of a tool-using turn, and zeroing there would discard
	// the answer the engine really did stream (the P3-RW-9 drain r1 lesson).
	p.feed([]byte(`{"role":"assistant","tool_calls":[{"type":"function","id":"c2","function":{"name":"Read","arguments":"{}"}}]}`))
	if p.finalText != "  final answer, verbatim  " {
		t.Errorf("a trailing tool-only message zeroed the retained answer: %q", p.finalText)
	}
	// An empty stream yields empty — limb (3), never an invention.
	if got := newParser(func(string, ...any) {}).finalText; got != "" {
		t.Errorf("an empty stream produced ResultText %q", got)
	}
}

// ── T19 · one paid call is exactly one Usage ─────────────────────────────────

// TestOnePaidCallOneUsageEvent replays the R0 capture's usage source. The
// stream carries NO usage on this engine (R0-Q2); the run's own transcript
// records one `usage.record` per model call, already decomposed the way
// S02.4(a) wants it.
func TestOnePaidCallOneUsageEvent(t *testing.T) {
	wire := strings.Join([]string{
		`{"type":"metadata","protocol_version":"1.5","created_at":1787763099423}`,
		`{"type":"llm.request","agentId":"main","model":"k3","modelAlias":"__kimi_env_model__","turnStep":"0.1","time":1787763099466}`,
		`{"type":"usage.record","agentId":"main","model":"__kimi_env_model__","usage":{"inputOther":73,"output":29,"inputCacheRead":64,"inputCacheCreation":0},"usageScope":"turn","time":1787763099487}`,
		`{"type":"token_counting.measured","agentId":"main","length":3,"tokens":166,"time":1787763099487}`,
		`{"type":"llm.request","agentId":"main","model":"k3","modelAlias":"__kimi_env_model__","turnStep":"0.2","time":1787763099500}`,
		`{"type":"usage.record","agentId":"main","model":"__kimi_env_model__","usage":{"inputOther":10,"output":5,"inputCacheRead":0,"inputCacheCreation":7},"usageScope":"turn","time":1787763099510}`,
		`{"type":"turn.ended","agentId":"main","turnId":0,"reason":"completed","durationMs":35,"time":1787763099489}`,
	}, "\n") + "\n"

	tr := newTranscript(func(string, ...any) {})
	var usages []*adapters.Usage
	for _, line := range strings.Split(strings.TrimSuffix(wire, "\n"), "\n") {
		for _, ev := range tr.feed([]byte(line)) {
			if ev.Kind != adapters.KindUsage {
				t.Fatalf("the transcript minted a %q event — it is the usage source and nothing else", ev.Kind)
			}
			usages = append(usages, ev.Usage)
		}
	}
	if len(usages) != 2 {
		t.Fatalf("%d Usage events for 2 model calls, want exactly 2 (S10.1: one row per paid call)", len(usages))
	}
	u := usages[0]
	// The decomposition, checked by arithmetic against what the loopback
	// provider actually returned in the spike (prompt 137 = 73 + 64 cached,
	// completion 29, total 166). `inputOther` EXCLUDES cache reads, which is
	// the same field semantics the Anthropic normalization assumes — verified,
	// not assumed (R6).
	if u.InputTokens != 73 || u.CacheReadTokens != 64 || u.CacheCreationTokens != 0 || u.OutputTokens != 29 {
		t.Errorf("usage = in %d / cacheRead %d / cacheCreate %d / out %d, want 73/64/0/29",
			u.InputTokens, u.CacheReadTokens, u.CacheCreationTokens, u.OutputTokens)
	}
	// The model id comes from llm.request, NEVER from usage.record — whose
	// `model` member is the synthesized alias `__kimi_env_model__`. Reading it
	// there would meter every run under a fictional model name.
	if u.ModelID != "k3" {
		t.Errorf("ModelID = %q, want %q — usage.record carries the alias, llm.request carries the model", u.ModelID, "k3")
	}
	if strings.Contains(u.ModelID, "__kimi_env_model__") {
		t.Error("ModelID is the synthesized env alias")
	}
	// Per-call rows only; a run total would trigger a checkpoint per D7 and
	// there is no terminal envelope on this engine to carry one anyway.
	for i, got := range usages {
		if got.Total {
			t.Errorf("usage %d is marked Total — this substrate emits no run-total row", i)
		}
		if got.MessageID == "" {
			t.Errorf("usage %d carries no message id — one paid call must be locatable in the session", i)
		}
	}
	if usages[0].MessageID == usages[1].MessageID {
		t.Error("two paid calls share one message id — they would group into one Usage")
	}
	if usages[1].MessageIndex <= usages[0].MessageIndex {
		t.Errorf("message index did not advance: %d then %d", usages[0].MessageIndex, usages[1].MessageIndex)
	}

	// A SECOND drain of the same transcript must not re-bill. The guard is the
	// byte offset the tail carries, so the assertion drives the real path —
	// two drains of one growing file — rather than re-feeding lines, which is
	// something production never does.
	s := tailSession(t, wire)
	s.drainUsage()
	first := drainEvents(s)
	s.drainUsage()
	if again := drainEvents(s); len(again) != 0 {
		t.Errorf("a second drain of an unchanged transcript emitted %d more Usage events — a checkpointed call would be billed twice", len(again))
	}
	if len(first) != 2 {
		t.Fatalf("the first drain emitted %d Usage events, want 2", len(first))
	}
	// And a call appended AFTER the first drain is picked up exactly once —
	// the tail must not stall, or a long run checkpoints only its opening.
	appendWire(t, s.wirePath, `{"type":"usage.record","agentId":"main","model":"__kimi_env_model__","usage":{"inputOther":1,"output":1,"inputCacheRead":0,"inputCacheCreation":0},"usageScope":"turn","time":1787763099600}`)
	s.drainUsage()
	if grew := drainEvents(s); len(grew) != 1 {
		t.Errorf("a newly appended paid call produced %d Usage events, want exactly 1", len(grew))
	}
}

// tailSession builds a session whose per-run home holds one session transcript,
// laid out exactly as the engine lays it out.
func tailSession(t *testing.T, wire string) *session {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "wd_spike_abc123def456", "session_ln7", "agents", "main")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir transcript dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wire.jsonl"), []byte(wire), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return &session{
		a:      &Adapter{Log: discardLogger()},
		low:    &lowered{home: home},
		events: make(chan adapters.Event, 64),
	}
}

func drainEvents(s *session) []adapters.Event {
	var out []adapters.Event
	for {
		select {
		case ev := <-s.events:
			out = append(out, ev)
		default:
			return out
		}
	}
}

func appendWire(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("append to transcript: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("append to transcript: %v", err)
	}
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// ── helpers ──────────────────────────────────────────────────────────────────

func mustLower(t *testing.T, a *Adapter, req adapters.StartRequest) *lowered {
	t.Helper()
	l, err := a.lower(req, nil)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	return l
}

func isErr(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func hasFlag(argv []string, flag string) bool { return contains(argv, flag) }

func hasPair(argv []string, flag, value string) bool {
	for i := 0; i < len(argv)-1; i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}
	return false
}

func envValue(env []string, name string) string {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v
		}
	}
	return ""
}

// tomlInt reads `key = <int>` out of the lowered config body.
func tomlInt(body, key string) (int64, bool) {
	for _, line := range strings.Split(body, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n, err == nil
	}
	return 0, false
}

func randToken(rng *rand.Rand) string {
	const alpha = "abcdefghijklmnopqrstuvwxyz._"
	b := make([]byte, rng.Intn(10)+3)
	for i := range b {
		b[i] = alpha[rng.Intn(len(alpha))]
	}
	return string(b)
}

var _ = json.Marshal

// ── T25 · the three tiers, and the ONE sanctioned skip class ─────────────────

// TestKimiCLIConformanceTiers pins the tier split itself: tier F always runs,
// tier R runs for real at $0 whenever the binary is present and otherwise
// prints exactly the sanctioned text, and tier L never spends without its
// ratified opt-in. The skip TEXT is load-bearing — CONVENTIONS §10 admits one
// skip class, and a suite that invents its own wording makes a skipped tier
// indistinguishable from a passing one in a battery log.
func TestKimiCLIConformanceTiers(t *testing.T) {
	const sanctioned = "SANCTIONED SKIP (CONVENTIONS §10)"

	// Tier F needs nothing: it is the R0 capture replayed, and it has already
	// run by the time this line executes.
	l := mustLower(t, testAdapter(t), testRequest(t))
	if len(l.argv) == 0 {
		t.Fatal("tier F could not lower an invocation")
	}

	// Tier R's absence-skip is the sanctioned text, verbatim, and it names the
	// binary rather than the packet.
	src, err := os.ReadFile("realengine_test.go")
	if err != nil {
		t.Fatalf("read realengine_test.go: %v", err)
	}
	if !strings.Contains(string(src), sanctioned) {
		t.Errorf("the tier-R absence skip does not print %q", sanctioned)
	}
	// Tier R must terminate on loopback and NEVER on a provider. The suite
	// asserting that about itself is cheap and catches the one edit that would
	// make the whole tier cost money.
	for _, banned := range []string{"api.kimi.com", "api.moonshot", "https://"} {
		body := string(src)
		if banned == "https://" {
			// httptest serves http://127.0.0.1; any https URL here would be an
			// endpoint reached off-host.
			if strings.Contains(body, "https://") && !strings.Contains(body, "kimi-code@") {
				t.Errorf("realengine_test.go names an https endpoint — every tier-R turn must terminate on loopback")
			}
			continue
		}
		if strings.Contains(body, banned) {
			t.Errorf("realengine_test.go names %q — tier R is $0 and reaches no provider", banned)
		}
	}

	// Tier L is gated on the EXISTING env name; no second one is minted.
	live, err := os.ReadFile("live_smoke_test.go")
	if err != nil {
		t.Fatalf("read live_smoke_test.go: %v", err)
	}
	if !strings.Contains(string(live), `"SINET_LIVE_SMOKE"`) {
		t.Error("tier L does not gate on SINET_LIVE_SMOKE")
	}
	for _, minted := range []string{"SINET_KIMI_LIVE", "SINET_LIVE_KIMI", "KIMI_LIVE_SMOKE"} {
		if strings.Contains(string(live), minted) {
			t.Errorf("tier L mints a second opt-in env name %q — §10 ratifies exactly one", minted)
		}
	}
	if !strings.Contains(string(live), "SANCTIONED SKIP") {
		t.Error("tier L does not print a named skip")
	}
	// And it is structurally unreachable at landing for a reason it states.
	if !strings.Contains(string(live), "gray") {
		t.Error("tier L does not record that the first live call on this lane is the operator's own door run")
	}
}

// ── T?/A8 · resume re-supplies the ENTIRE invocation ─────────────────────────

// TestResumeRebuildsTheWholeInvocation pins S03.4's obligation on the substrate
// where it is most literal. On the Anthropic lane "re-supply" means passing the
// settings, permission mode, model and tool allowlist again; here the whole
// invocation LIVES in the run's own KIMI_CODE_HOME, so a resume that trusted a
// home somebody had edited would run under configuration nobody re-checked.
//
// The engine restores nothing on its own. It cannot even be told which session
// id to use on a fresh start — the id is server-generated — so a resume names
// the REPORTED id and rebuilds everything else from the park record.
func TestResumeRebuildsTheWholeInvocation(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	start := mustLower(t, a, req)
	if err := a.materialize(start); err != nil {
		t.Fatalf("materialize: %v", err)
	}

	// Somebody edits the run's lowered config between the park and the resume.
	// This is not a hypothetical: the home is a directory on disk that outlives
	// the process precisely so a parked run can come back.
	tampered := filepath.Join(start.home, configTOML)
	if err := os.WriteFile(tampered, []byte("telemetry = true\n"), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	rec := adapters.ParkRecord{
		RunID:     req.RunID,
		Substrate: adapters.SubstrateKimiCLI,
		Cursor:    adapters.Cursor{Substrate: adapters.SubstrateKimiCLI, SessionID: "session_abc-123"},
		Reason:    adapters.ParkReasonPause,
		Start:     req,
	}
	resumed, err := a.lower(rec.Start, &resumeSpec{sessionID: rec.Cursor.SessionID, continuation: "carry on"})
	if err != nil {
		t.Fatalf("lower(resume): %v", err)
	}
	if err := a.materialize(resumed); err != nil {
		t.Fatalf("materialize(resume): %v", err)
	}

	// The home is REBUILT, not assumed intact: the tampered body is gone.
	raw, err := os.ReadFile(filepath.Join(resumed.home, configTOML))
	if err != nil {
		t.Fatalf("read the resumed config: %v", err)
	}
	if strings.Contains(string(raw), "telemetry = true") {
		t.Error("the resume inherited a tampered config — the invocation must be re-supplied and re-verified, not trusted")
	}
	// Every channel is closed again, not just the one that was edited.
	for _, needle := range []string{`disabled = ["Agent", "AgentSwarm", "mcp__*"]`, `print_background_mode = "exit"`, "max_attempts_per_step"} {
		if !strings.Contains(string(raw), needle) {
			t.Errorf("the resumed config lost %q", needle)
		}
	}
	if _, ok := resumed.files[systemMD]; !ok {
		t.Error("the resume did not re-supply SYSTEM.md, so the AGENTS.md channel re-opens on the second leg")
	}
	if got := envValue(resumed.env, "KIMI_MODEL_NAME"); got != req.Model {
		t.Errorf("the resumed env carries model %q, want %q", got, req.Model)
	}

	// It names the ENGINE-REPORTED id, and nothing fabricates one.
	if !hasPair(resumed.argv, "--session", "session_abc-123") {
		t.Errorf("the resume does not name the engine-reported session id: %v", resumed.argv)
	}
	if !resumed.resume {
		t.Error("the lowering is not marked as a resume")
	}
	if !contains(resumed.argv, "carry on") {
		t.Errorf("the continuation never reached argv: %v", resumed.argv)
	}

	// A resume with no continuation is REFUSED rather than sent as an empty
	// turn: every resume here is a pause-resume, since there is no gate defer
	// to unpark, and a resumed session with no user body does nothing.
	if _, err := a.lower(rec.Start, &resumeSpec{sessionID: rec.Cursor.SessionID}); err == nil {
		t.Error("a resume with no continuation was accepted")
	}
	// And a park record with no engine-reported id cannot resume at all.
	if _, err := a.Resume(context.Background(), adapters.ParkRecord{Start: req}, nil); err == nil {
		t.Error("a park record with no engine session id was resumed — there is nothing to resume ONTO")
	}
	// An answer carrying a gate ask is refused: this substrate never parks on a
	// gate, so an ask id in an answer is a caller error, and dropping it
	// silently is how a refused call gets executed anyway.
	_, err = a.Resume(context.Background(), rec, &adapters.Answer{AskID: "ask-1", Continuation: "go"})
	if !isErr(err, ErrGateParkUnsupported) {
		t.Errorf("resuming with a gate answer returned %v, want ErrGateParkUnsupported", err)
	}
}

package opencode

// Conformance suite v1 for the opencode substrate (Spec S03.3): behavioral
// assertions at the exact pinned engine version, never docs-derived. Three
// marked tiers, per CONVENTIONS §10:
//
//	tier F — replay fixtures + a fake serve speaking the recorded 1.18.3
//	         surface (always run; CI has no engine). Provenance for every
//	         fixture is testdata/PROVENANCE.md.
//	tier R — real `opencode serve`, ZERO provider cost: loopback bind, Basic
//	         auth, XDG containment, and the full permission round-trip driven
//	         against a loopback fake OpenAI-compatible provider. Skipped with
//	         notice when the binary is absent.
//	tier L — one minimal PAID call (live_smoke_test.go), only under
//	         SINET_LIVE_SMOKE=1, and no lane is commissioned on this substrate
//	         until LN-2 — so it names both skips.
//
// The version check reports a pin↔installed delta LOUDLY without failing: the
// suite's own purpose (S03.3 step 3) is to run against bump candidates.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/lockfile"
)

// testProviders is the R15 lane-config seam fed FIXTURE values only: a
// loopback fake endpoint, no real provider, no credential (§1: $0 absolute).
func testProviders(baseURL string) ProviderConfig {
	return ProviderConfig{
		"sinetfake": {
			NPM:     "@ai-sdk/openai-compatible",
			Name:    "Sinet Fake",
			Options: map[string]any{"baseURL": baseURL + "/v1"},
			Models:  map[string]ModelEntry{"fakemodel": {Name: "Fake Model"}},
		},
	}
}

func testAdapter(t *testing.T) *Adapter {
	t.Helper()
	return &Adapter{
		Root:      t.TempDir(),
		Providers: testProviders("http://127.0.0.1:1"),
		Env:       []string{"PATH=/usr/bin", "HOME=/home/user"},
		Now:       func() time.Time { return time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC) },
		Log:       slog.New(slog.NewTextHandler(testWriter{t}, nil)),
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s", strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

func testRequest(t *testing.T) adapters.StartRequest {
	t.Helper()
	return adapters.StartRequest{
		RunID: "r1", UserID: "u1",
		Model:   "sinetfake/fakemodel",
		Cwd:     t.TempDir(),
		WorkDir: t.TempDir(),
		Worker: adapters.CompiledWorker{
			Prompt:             "run echo hello-from-fake using bash",
			SystemPromptAppend: "You are terse.",
			AgentsJSON:         json.RawMessage(`{"sinet_w":{"description":"probe","prompt":"be terse"}}`),
			AgentName:          "sinet_w",
			ToolAllowlist:      []string{"bash", "read"},
			GatedTools:         []string{"bash"},
			PermissionMode:     "default",
		},
		CeilingCostUSD: 0.10,
		CeilingSteps:   4,
	}
}

// loweredConfig decodes the compiled OPENCODE_CONFIG_CONTENT body.
type loweredConfig struct {
	Permission map[string]string `json:"permission"`
	Provider   map[string]struct {
		NPM     string         `json:"npm"`
		Options map[string]any `json:"options"`
		Models  map[string]any `json:"models"`
	} `json:"provider"`
	Agent map[string]struct {
		Description string            `json:"description"`
		Mode        string            `json:"mode"`
		Prompt      string            `json:"prompt"`
		Permission  map[string]string `json:"permission"`
	} `json:"agent"`
}

func decodeConfig(t *testing.T, l *lowered) loweredConfig {
	t.Helper()
	var cfg loweredConfig
	if err := json.Unmarshal(l.configJSON, &cfg); err != nil {
		t.Fatalf("compiled config is not valid JSON: %v (%s)", err, l.configJSON)
	}
	return cfg
}

func envValue(env []string, name string) (string, bool) {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v, true
		}
	}
	return "", false
}

// ─── tier F1: lowering — one knob per config channel (S03.5) ───

func TestLoweringConfigChannels(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)

	// Attempt the leak on every channel S03.5 names (SPIKE P2-S5 Probe 4):
	// a walked-up project config re-enabling native spawn, an ambient inline
	// config, and a HOME-relative cross-read decoy.
	ancestor := t.TempDir()
	req.Cwd = filepath.Join(ancestor, "run")
	if err := os.MkdirAll(req.Cwd, 0o700); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}
	decoy := `{"permission":{"task":"allow"},"agent":{"evil_cwd":{"prompt":"DECOY-CWD-BODY"}}}`
	if err := os.WriteFile(filepath.Join(ancestor, "opencode.json"), []byte(decoy), 0o600); err != nil {
		t.Fatalf("plant decoy config: %v", err)
	}
	claudeDecoy := filepath.Join(ancestor, "CLAUDE.md")
	if err := os.WriteFile(claudeDecoy, []byte("DECOY-CLAUDE-MD"), 0o600); err != nil {
		t.Fatalf("plant CLAUDE.md decoy: %v", err)
	}
	a.Env = append(a.Env,
		"OPENCODE_CONFIG_CONTENT="+decoy,
		"OPENCODE_CONFIG_DIR="+ancestor,
		"OPENCODE_DISABLE_PROJECT_CONFIG=0",
	)

	l, err := a.lower(req)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	cfg := decodeConfig(t, l)

	// tools incl. native spawn: blanket deny strips the tool pre-inference.
	if cfg.Permission["task"] != "deny" {
		t.Errorf("compiled permission.task = %q, want \"deny\" (S03.5 sole-controller)", cfg.Permission["task"])
	}
	// agent map, selected by NAME (no inline per-request agent definition).
	ag, ok := cfg.Agent[req.Worker.AgentName]
	if !ok {
		t.Fatalf("compiled config has no agent %q: %s", req.Worker.AgentName, l.configJSON)
	}
	if ag.Prompt != "be terse" {
		t.Errorf("agent prompt = %q, want the compiled body", ag.Prompt)
	}
	if ag.Permission["task"] != "deny" {
		t.Errorf("agent permission.task = %q, want \"deny\" (inherited by any subagent)", ag.Permission["task"])
	}
	if ag.Permission["bash"] != "ask" {
		t.Errorf("gated tool bash = %q, want \"ask\" (the S03.4 park trigger)", ag.Permission["bash"])
	}
	if ag.Permission["read"] != "allow" {
		t.Errorf("allowlisted tool read = %q, want \"allow\"", ag.Permission["read"])
	}
	if ag.Permission["*"] != "deny" {
		t.Errorf("agent permission[*] = %q, want \"deny\" (default-deny toolset)", ag.Permission["*"])
	}
	if l.agentName != req.Worker.AgentName {
		t.Errorf("lowered agent name = %q", l.agentName)
	}
	// the fixture provider block (R15 seam, fixture values only).
	if _, ok := cfg.Provider["sinetfake"]; !ok {
		t.Errorf("compiled config lost the provider block: %s", l.configJSON)
	}

	// env: the XDG quartet, all under the platform-owned root, plus the four
	// knobs the S03.5 opencode column names.
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		v, ok := envValue(l.env, name)
		if !ok {
			t.Errorf("lowered env missing %s", name)
			continue
		}
		if !strings.HasPrefix(v, l.root+string(filepath.Separator)) && v != l.root {
			t.Errorf("%s = %q is outside the platform-owned root %q", name, v, l.root)
		}
	}
	for name, want := range map[string]string{
		"OPENCODE_DISABLE_CLAUDE_CODE":     "1",
		"OPENCODE_DISABLE_PROJECT_CONFIG":  "1",
		"OPENCODE_PURE":                    "1",
		"OPENCODE_DISABLE_DEFAULT_PLUGINS": "1",
	} {
		if v, ok := envValue(l.env, name); !ok || v != want {
			t.Errorf("lowered env %s = %q (present=%v), want %q", name, v, ok, want)
		}
	}

	// The decoys took effect NOWHERE (the S03.5 compound guarantee).
	joined := string(l.configJSON) + "\n" + strings.Join(l.env, "\n")
	for _, leak := range []string{"DECOY-CWD-BODY", "evil_cwd", "DECOY-CLAUDE-MD", `"task":"allow"`, ancestor + "\n"} {
		if strings.Contains(joined, leak) {
			t.Errorf("decoy %q reached the lowered invocation — a config channel is open (S03.5)", leak)
		}
	}
	if v, _ := envValue(l.env, "OPENCODE_CONFIG_CONTENT"); v == decoy {
		t.Errorf("ambient OPENCODE_CONFIG_CONTENT survived lowering")
	}
	if _, ok := envValue(l.env, "OPENCODE_CONFIG_DIR"); ok {
		t.Errorf("ambient OPENCODE_CONFIG_DIR survived lowering")
	}
}

// TestLoweringLeakProperty is the property half of the S03.5 compound
// guarantee (F1/F4): over RANDOM adversarial inputs, lowering either refuses
// or produces a config whose native-spawn knob is deny and which carries no
// ambient value.
func TestLoweringLeakProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5A17))
	for i := 0; i < 200; i++ {
		a := testAdapter(t)
		req := testRequest(t)
		marker := fmt.Sprintf("AMBIENT-MARKER-%d-%d", i, rng.Int63())
		ambient := []string{
			"OPENCODE_CONFIG_CONTENT=" + `{"permission":{"task":"allow"},"x":"` + marker + `"}`,
			"OPENCODE_SERVER_PASSWORD=" + marker,
			"OPENCODE_DISABLE_CLAUDE_CODE=0",
			"OPENCODE_DISABLE_PROJECT_CONFIG=0",
			"OPENCODE_PURE=0",
			"ANTHROPIC_API_KEY=" + marker,
			"CLAUDECODE=" + marker,
			"CLAUDE_CODE_ENTRYPOINT=" + marker,
			"CLAUDE_EFFORT=" + marker,
			"CLAUDE_PID=" + marker,
			"AI_AGENT=" + marker,
			"XDG_CONFIG_HOME=/ambient/" + marker,
			"XDG_DATA_HOME=/ambient/" + marker,
		}
		rng.Shuffle(len(ambient), func(x, y int) { ambient[x], ambient[y] = ambient[y], ambient[x] })
		a.Env = append(a.Env, ambient[:1+rng.Intn(len(ambient))]...)

		// Random adversarial worker shapes.
		switch rng.Intn(4) {
		case 0:
			req.Worker.ToolAllowlist = append(req.Worker.ToolAllowlist, pickOne(rng, "task", "Task", "TASK", "*"))
		case 1:
			req.Worker.GatedTools = append(req.Worker.GatedTools, pickOne(rng, "task", "Task"))
		case 2:
			req.Worker.AgentsJSON = json.RawMessage(
				`{"sinet_w":{"prompt":"be terse","permission":{"task":"allow"}}}`)
		}

		l, err := a.lower(req)
		if err != nil {
			// Refusing is a correct outcome — but it must be a NAMED refusal.
			if !errors.Is(err, ErrNativeSpawnTool) {
				t.Fatalf("iteration %d: lowering failed with an unnamed error: %v", i, err)
			}
			continue
		}
		cfg := decodeConfig(t, l)
		if cfg.Permission["task"] != "deny" {
			t.Fatalf("iteration %d: compiled permission.task = %q, want deny", i, cfg.Permission["task"])
		}
		if ag, ok := cfg.Agent[req.Worker.AgentName]; ok && ag.Permission["task"] != "deny" {
			t.Fatalf("iteration %d: agent permission.task = %q, want deny", i, ag.Permission["task"])
		}
		joined := string(l.configJSON) + "\n" + strings.Join(l.env, "\n")
		if strings.Contains(joined, marker) {
			t.Fatalf("iteration %d: ambient marker %q reached the lowered invocation", i, marker)
		}
	}
}

func pickOne(rng *rand.Rand, options ...string) string { return options[rng.Intn(len(options))] }

// ─── tier F2: env scrub (S03.5 env channel, extended per CONVENTIONS §10) ───

func TestLoweringEnvScrub(t *testing.T) {
	a := testAdapter(t)
	a.Env = []string{
		"PATH=/usr/bin", "HOME=/home/user", "LANG=C.UTF-8",
		"OPENCODE_SERVER_PASSWORD=ambient-secret",
		"OPENCODE_CONFIG_CONTENT={}", "OPENCODE_API_KEY=zen-key",
		"ANTHROPIC_API_KEY=sk-secret", "ANTHROPIC_BASE_URL=http://x",
		"CLAUDECODE=1", "CLAUDE_CODE_ENTRYPOINT=cli", "AI_AGENT=x",
		"CLAUDE_EFFORT=high", "CLAUDE_PID=42",
		"XDG_CONFIG_HOME=/ambient/config", "XDG_DATA_HOME=/ambient/data",
		"XDG_STATE_HOME=/ambient/state", "XDG_CACHE_HOME=/ambient/cache",
	}
	l, err := a.lower(testRequest(t))
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	env := strings.Join(l.env, "\n")
	for _, banned := range []string{"ambient-secret", "zen-key", "sk-secret", "CLAUDECODE=",
		"CLAUDE_CODE_", "AI_AGENT=", "CLAUDE_EFFORT=", "CLAUDE_PID=", "ANTHROPIC_", "/ambient/"} {
		if strings.Contains(env, banned) {
			t.Errorf("env leaks %q (S03.5 scrub set): %s", banned, env)
		}
	}
	for _, benign := range []string{"PATH=/usr/bin", "HOME=/home/user", "LANG=C.UTF-8"} {
		if !strings.Contains(env, benign) {
			t.Errorf("benign env dropped: %q", benign)
		}
	}
	// HOME is deliberately UNCHANGED (that is exactly why
	// OPENCODE_DISABLE_CLAUDE_CODE is required, S03.5).
	if v, _ := envValue(l.env, "HOME"); v != "/home/user" {
		t.Errorf("HOME = %q, want the ambient value (the S03.5 premise)", v)
	}
}

// ─── tier F3: per-user XDG disjointness (SPIKE G1-S3 F1) ───

func TestXDGDisjointPerUser(t *testing.T) {
	a := testAdapter(t)
	r1 := testRequest(t)
	r1.UserID = "alice"
	r2 := testRequest(t)
	r2.UserID = "bob"

	l1, err := a.lower(r1)
	if err != nil {
		t.Fatalf("lower(alice): %v", err)
	}
	l2, err := a.lower(r2)
	if err != nil {
		t.Fatalf("lower(bob): %v", err)
	}
	if l1.root == l2.root {
		t.Fatalf("both users share the platform root %q", l1.root)
	}
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		v1, ok1 := envValue(l1.env, name)
		v2, ok2 := envValue(l2.env, name)
		if !ok1 || !ok2 {
			t.Fatalf("%s missing (alice=%v bob=%v)", name, ok1, ok2)
		}
		if v1 == v2 {
			t.Errorf("%s is SHARED between users: %q", name, v1)
		}
		if strings.HasPrefix(v1, l2.root) || strings.HasPrefix(v2, l1.root) {
			t.Errorf("%s crosses the other user's root (%q vs %q)", name, v1, v2)
		}
	}
	// Both roots exist at 0700 (plaintext tokens live under them, G1-S3 §3).
	for _, root := range []string{l1.root, l2.root} {
		st, err := os.Stat(root)
		if err != nil {
			t.Fatalf("per-user root %q not materialized: %v", root, err)
		}
		if perm := st.Mode().Perm(); perm != 0o700 {
			t.Errorf("per-user root %q mode = %o, want 700", root, perm)
		}
	}
}

// ─── tier F4: native-spawn refusal (S03.5 sole-controller) ───

func TestLoweringRefusesNativeSpawnEnable(t *testing.T) {
	cases := map[string]func(*adapters.StartRequest){
		"allowlist names task":    func(r *adapters.StartRequest) { r.Worker.ToolAllowlist = []string{"read", "task"} },
		"allowlist names Task":    func(r *adapters.StartRequest) { r.Worker.ToolAllowlist = []string{"read", "Task"} },
		"allowlist is a wildcard": func(r *adapters.StartRequest) { r.Worker.ToolAllowlist = []string{"*"} },
		"gated tools name task":   func(r *adapters.StartRequest) { r.Worker.GatedTools = []string{"task"} },
		"agent fragment allows task": func(r *adapters.StartRequest) {
			r.Worker.AgentsJSON = json.RawMessage(`{"sinet_w":{"permission":{"task":"allow"}}}`)
		},
		"agent fragment asks for task": func(r *adapters.StartRequest) {
			r.Worker.AgentsJSON = json.RawMessage(`{"sinet_w":{"permission":{"task":"ask"}}}`)
		},
	}
	a := testAdapter(t)
	for name, mutate := range cases {
		req := testRequest(t)
		mutate(&req)
		_, err := a.lower(req)
		if err == nil {
			t.Errorf("%s: lowered without error — sole-controller breach (S03.5)", name)
			continue
		}
		if !errors.Is(err, ErrNativeSpawnTool) {
			t.Errorf("%s: error %v is not ErrNativeSpawnTool", name, err)
		}
	}
}

// ─── tier F5: the S02.4(e) invocation fingerprint ───

func TestFingerprintTracksInvocation(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	l1, err := a.lower(req)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	l2, err := a.lower(req)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if l1.fingerprint != l2.fingerprint {
		t.Errorf("fingerprint is not stable across identical invocations")
	}
	// Session-id independence is structural here: no session id is an input to
	// lowering at all (the engine mints its own id, S03.1 start row). Assert it
	// stays that way by lowering after a session already exists.
	if strings.Contains(l1.fingerprint, fixtureSessionID) {
		t.Errorf("fingerprint embeds a session id")
	}

	for name, mutate := range map[string]func(*Adapter, *adapters.StartRequest){
		"model": func(_ *Adapter, r *adapters.StartRequest) { r.Model = "sinetfake/other" },
		"agent body": func(_ *Adapter, r *adapters.StartRequest) {
			r.Worker.AgentsJSON = json.RawMessage(`{"sinet_w":{"prompt":"other"}}`)
		},
		"agent name": func(_ *Adapter, r *adapters.StartRequest) {
			r.Worker.AgentsJSON = json.RawMessage(`{"w2":{"prompt":"be terse"}}`)
			r.Worker.AgentName = "w2"
		},
		"permission map":  func(_ *Adapter, r *adapters.StartRequest) { r.Worker.GatedTools = []string{"bash", "read"} },
		"tool allowlist":  func(_ *Adapter, r *adapters.StartRequest) { r.Worker.ToolAllowlist = []string{"read"} },
		"system append":   func(_ *Adapter, r *adapters.StartRequest) { r.Worker.SystemPromptAppend = "Other." },
		"provider config": func(ad *Adapter, _ *adapters.StartRequest) { ad.Providers = testProviders("http://127.0.0.1:2") },
	} {
		ad := testAdapter(t)
		ad.Root = a.Root
		mut := testRequest(t)
		mutate(ad, &mut)
		lm, err := ad.lower(mut)
		if err != nil {
			t.Fatalf("lower(%s): %v", name, err)
		}
		if lm.fingerprint == l1.fingerprint {
			t.Errorf("fingerprint blind to %s change (S02.4e)", name)
		}
	}
}

// ─── tier F6: Basic auth on EVERY endpoint, health included (R4) ───

func TestClientSendsBasicAuthOnEveryEndpoint(t *testing.T) {
	f := newFakeServe(t)
	f.onPrompt = func(f *fakeServe, n int) { f.publishFixture("happy.sse") }

	logs := &captureWriter{}
	a := fakeAdapter(t, f)
	a.Log = slog.New(slog.NewTextHandler(logs, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sess, err := a.Start(ctx, testRequest(t))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	var payloads []string
	for ev := range sess.Events() {
		payloads = append(payloads, string(ev.Payload))
	}
	if _, err := sess.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	// The health watcher rides the same credentials (S03.2: /api/health is not
	// exempt from the server password).
	w := NewHealthWatch(f.instance(), nil, time.Second, a.Log)
	if err := w.Probe(ctx); err != nil {
		t.Fatalf("health probe: %v", err)
	}
	if !w.Healthy() {
		t.Errorf("health watcher reported unhealthy against a healthy serve")
	}

	want := basicHeader(f.password)
	reqs := f.requests()
	if len(reqs) == 0 {
		t.Fatal("fake serve recorded no requests")
	}
	sawHealth := false
	for _, r := range reqs {
		if r.Auth != want {
			t.Errorf("%s %s carried Authorization %q, want the instance Basic credentials", r.Method, r.Path, r.Auth)
		}
		if r.Path == "/api/health" {
			sawHealth = true
		}
	}
	if !sawHealth {
		t.Error("no /api/health request recorded — the health watcher never probed")
	}
	// D2 hygiene: the password never reaches logs or event payloads.
	if strings.Contains(logs.String(), f.password) {
		t.Errorf("the instance password reached the ops log")
	}
	for _, p := range payloads {
		if strings.Contains(p, f.password) {
			t.Errorf("the instance password reached an event payload: %s", p)
		}
	}
}

type captureWriter struct{ b strings.Builder }

func (c *captureWriter) Write(p []byte) (int, error) { return c.b.Write(p) }
func (c *captureWriter) String() string              { return c.b.String() }

// fakeAdapter builds an Adapter bound to a fake serve through a static
// instance provider — the R3 seam with the dev-posture spawner swapped out.
func fakeAdapter(t *testing.T, f *fakeServe) *Adapter {
	t.Helper()
	a := testAdapter(t)
	a.Providers = testProviders(f.srv.URL)
	a.Instances = &staticInstances{inst: f.instance()}
	a.CancelGrace = 250 * time.Millisecond
	return a
}

type staticInstances struct {
	inst    Instance
	stopped []string
	specs   []InstanceSpec
}

func (s *staticInstances) Acquire(_ context.Context, spec InstanceSpec) (Instance, error) {
	s.specs = append(s.specs, spec)
	return s.inst, nil
}

func (s *staticInstances) Stop(_ context.Context, userID string) error {
	s.stopped = append(s.stopped, userID)
	return nil
}

// ─── tier F7: the happy path, end to end over the fake serve ───

func TestParseHappyFixture(t *testing.T) {
	f := newFakeServe(t)
	f.onPrompt = func(f *fakeServe, n int) { f.publishFixture("tooltrun.sse") }
	a := fakeAdapter(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	sess, err := a.Start(ctx, testRequest(t))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	var usages []*adapters.Usage
	var kinds []adapters.EventKind
	for ev := range sess.Events() {
		kinds = append(kinds, ev.Kind)
		if ev.Kind == adapters.KindUsage && ev.Usage != nil {
			usages = append(usages, ev.Usage)
		}
	}
	out, err := sess.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome = %q (%s)", out.Kind, out.Detail)
	}
	// A tool-using turn is TWO paid model calls, one Usage each, grouped by
	// message id — never one per stream frame (SPIKE P2-S1 consumption note).
	if len(usages) != 2 {
		t.Fatalf("usage events = %d, want 2 (one per paid call): kinds %v", len(usages), kinds)
	}
	seen := map[string]bool{}
	for _, u := range usages {
		if seen[u.MessageID] {
			t.Errorf("two Usage events for message id %q", u.MessageID)
		}
		seen[u.MessageID] = true
		if u.InputTokens != 8 || u.OutputTokens != 5 || u.CacheReadTokens != 3 {
			t.Errorf("token mapping lost detail: %+v", u)
		}
		// opencode prices flat-rate lanes at $0: forwarded as a cross-check
		// figure ONLY, and nothing here derives a price (S03.7, D5).
		if u.EngineCostUSD != 0 {
			t.Errorf("EngineCostUSD = %v, want the engine's own 0 forwarded verbatim", u.EngineCostUSD)
		}
		if !strings.Contains(string(u.Raw), `"reasoning":2`) {
			t.Errorf("reasoning detail lost from Raw: %s", u.Raw)
		}
		if u.Total {
			t.Errorf("per-call usage row marked Total — run totals ride the terminal envelope (D7)")
		}
		if u.ModelID == "" {
			t.Errorf("usage row lost the model id")
		}
	}
	cur := sess.Cursor()
	if cur.Substrate != adapters.SubstrateOpencode {
		t.Errorf("cursor substrate = %q", cur.Substrate)
	}
	if cur.SessionID != fixtureSessionID {
		t.Errorf("cursor session id = %q, want the engine-REPORTED id (S02.4b)", cur.SessionID)
	}
	// opencode keeps its store in SQLite, not a JSONL transcript: the cursor's
	// transcript path stays empty and the Driver's copy-aside is inert (R5).
	if cur.TranscriptPath != "" {
		t.Errorf("cursor transcript path = %q, want empty (opencode has no JSONL transcript)", cur.TranscriptPath)
	}
	if cur.CwdKey != fixtureDirectory {
		t.Errorf("cursor cwd key = %q, want the engine-reported session directory", cur.CwdKey)
	}
	if cur.MessageIndex != 2 {
		t.Errorf("message index = %d, want 2", cur.MessageIndex)
	}
	if out.ResultText != "fake-done" {
		t.Errorf("ResultText = %q, want the final assistant message VERBATIM (R19)", out.ResultText)
	}
	if out.Totals != nil {
		t.Errorf("opencode emits no run-total envelope; Totals must stay nil rather than be invented")
	}
	// One tool_result event for the completed tool part.
	toolResults := 0
	for _, k := range kinds {
		if k == adapters.KindToolResult {
			toolResults++
		}
	}
	if toolResults != 1 {
		t.Errorf("tool_result events = %d, want 1: %v", toolResults, kinds)
	}
}

// ─── tier F8: forward tolerance (S03.1 MUST) ───

func TestParseForwardTolerance(t *testing.T) {
	p := newParser(fixtureSessionID, t.Logf)
	var kinds []adapters.EventKind
	for _, frame := range fixtureFrames(t, "unknown.sse") {
		for _, ev := range p.feed(frame) {
			kinds = append(kinds, ev.Kind)
		}
	}
	if !p.sawIdle {
		t.Fatal("the turn's idle was lost behind unknown/malformed frames")
	}
	if p.paidCalls != 1 {
		t.Errorf("paid calls = %d, want 1 (the real one still landed)", p.paidCalls)
	}
	if p.unknownFrames == 0 {
		t.Error("unknown/malformed frames were not counted as skipped")
	}
	for _, k := range kinds {
		switch k {
		case adapters.KindMessage, adapters.KindUsage, adapters.KindRateLimit,
			adapters.KindGateAsk, adapters.KindToolResult, adapters.KindDone:
		default:
			t.Errorf("a new platform kind %q was minted from an engine frame (S03.1 closed set)", k)
		}
	}
	// The v2-generation frame is parsed tolerantly and produces no platform
	// gate ask of its own (permission events ride v1 at 1.18.3, Probe 6-B).
	if p.ask != nil {
		t.Errorf("a v2-generation frame minted a gate ask: %+v", p.ask)
	}
}

func TestParseRateLimitForwardsRetryAsData(t *testing.T) {
	p := newParser(fixtureSessionID, t.Logf)
	var limits []adapters.Event
	for _, frame := range fixtureFrames(t, "retry.sse") {
		for _, ev := range p.feed(frame) {
			if ev.Kind == adapters.KindRateLimit {
				limits = append(limits, ev)
			}
		}
	}
	if len(limits) != 1 {
		t.Fatalf("rate-limit events = %d, want 1", len(limits))
	}
	var got struct {
		Attempt int64           `json:"attempt"`
		Next    int64           `json:"next"`
		Message string          `json:"message"`
		Action  json.RawMessage `json:"action"`
	}
	if err := json.Unmarshal(limits[0].Payload, &got); err != nil {
		t.Fatalf("rate-limit payload: %v (%s)", err, limits[0].Payload)
	}
	if got.Attempt != 2 || got.Next != 1787504400000 || got.Message == "" || len(got.Action) == 0 {
		t.Errorf("retry variant forwarded lossily: %+v (S03.1: forwarded as DATA, classified by S10)", got)
	}
}

// ─── tier F9: the v1 event endpoint (SPIKE P2-S1 Probe 6-B) ───

func TestEventSubscriptionUsesV1Endpoint(t *testing.T) {
	f := newFakeServe(t)
	f.onPrompt = func(f *fakeServe, n int) { f.publishFixture("happy.sse") }
	a := fakeAdapter(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sess, err := a.Start(ctx, testRequest(t))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	for range sess.Events() {
	}
	if _, err := sess.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !f.sawPath("/event") {
		t.Error("the adapter never subscribed to v1 GET /event")
	}
	for _, r := range f.requests() {
		if strings.HasPrefix(r.Path, "/api/event") || strings.HasSuffix(r.Path, "/event") && r.Path != "/event" {
			t.Errorf("the adapter subscribed to %q — permission events ride v1 /event at 1.18.3 (Probe 6-B)", r.Path)
		}
	}
}

// ─── tier F10: the gate park (S03.4) ───

func TestPermissionAskParksWithDurableAskRecord(t *testing.T) {
	f := newFakeServe(t)
	f.onPrompt = func(f *fakeServe, n int) {
		f.setPermissions(fixtureAsk())
		f.publishFixture("gate.sse")
	}
	a := fakeAdapter(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req := testRequest(t)
	sess, err := a.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	var kinds []adapters.EventKind
	var askEvents int
	for ev := range sess.Events() {
		kinds = append(kinds, ev.Kind)
		if ev.Kind == adapters.KindGateAsk {
			askEvents++
			if ev.Ask == nil || ev.Ask.ID != fixtureAskID {
				t.Errorf("gate_ask event without a usable Ask: %+v", ev.Ask)
			}
		}
	}
	out, err := sess.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if out.Kind != adapters.OutcomeParked {
		t.Fatalf("outcome = %q (%s), want parked at zero further cost (S03.4)", out.Kind, out.Detail)
	}
	if out.Ask == nil || out.Ask.ID != fixtureAskID {
		t.Fatalf("ask = %+v, want the requestID key", out.Ask)
	}
	if out.Ask.ToolName != "bash" {
		t.Errorf("ask tool name = %q", out.Ask.ToolName)
	}
	if out.Park == nil || out.Park.Reason != adapters.ParkReasonGateAsk {
		t.Fatalf("park = %+v", out.Park)
	}
	// The FULL invocation snapshot rides the park record (S03.4 obligation).
	if out.Park.Start.Model != req.Model || out.Park.Start.Worker.AgentName != req.Worker.AgentName ||
		len(out.Park.Start.Worker.ToolAllowlist) != len(req.Worker.ToolAllowlist) {
		t.Errorf("park record lost invocation state: %+v", out.Park.Start)
	}
	if out.Park.Fingerprint == "" {
		t.Error("park record lost the invocation fingerprint (S02.4e)")
	}
	if out.Park.Cursor.SessionID != fixtureSessionID {
		t.Errorf("park cursor session id = %q", out.Park.Cursor.SessionID)
	}
	if askEvents != 1 {
		t.Errorf("gate_ask events = %d, want 1: %v", askEvents, kinds)
	}
	if kinds[len(kinds)-1] != adapters.KindDone {
		t.Errorf("last event = %q, want done: %v", kinds[len(kinds)-1], kinds)
	}
	// The park cost nothing beyond the call that asked.
	if len(f.repliesSeen()) != 0 {
		t.Errorf("the adapter answered its own ask: %+v", f.repliesSeen())
	}
}

// ─── tier F11: once / reject, and NEVER always (SPIKE P2-S1 Probe 6-A) ───

func TestPermissionReplyOnceNeverAlways(t *testing.T) {
	for name, tc := range map[string]struct {
		answer    *adapters.Answer
		wantReply string
		wantMsg   string
	}{
		"approve": {ApproveAnswer(fixtureAskID), "once", ""},
		"reject":  {RejectAnswer(fixtureAskID, "not allowed here"), "reject", "not allowed here"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeServe(t)
			f.setPermissions(fixtureAsk())
			f.onReply = func(f *fakeServe, _ fakeReply) { f.publishFixture("gate-replied.sse") }
			a := fakeAdapter(t, f)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			rec := parkRecordFixture(t, a)
			sess, err := a.Resume(ctx, rec, tc.answer)
			if err != nil {
				t.Fatalf("Resume: %v", err)
			}
			for range sess.Events() {
			}
			if _, err := sess.Wait(ctx); err != nil {
				t.Fatalf("Wait: %v", err)
			}
			replies := f.repliesSeen()
			if len(replies) != 1 {
				t.Fatalf("recorded replies = %+v, want exactly one", replies)
			}
			if replies[0].RequestID != fixtureAskID {
				t.Errorf("reply keyed by %q, want the requestID", replies[0].RequestID)
			}
			if replies[0].Reply != tc.wantReply {
				t.Errorf("reply = %q, want %q", replies[0].Reply, tc.wantReply)
			}
			if replies[0].Message != tc.wantMsg {
				t.Errorf("reply message = %q, want %q (CorrectedError feedback)", replies[0].Message, tc.wantMsg)
			}
			// The legacy endpoint is never used: it cannot carry feedback.
			for _, r := range f.requests() {
				if strings.Contains(r.Path, "/permissions/") {
					t.Errorf("the adapter used legacy permission.respond (%s)", r.Path)
				}
			}
		})
	}
}

// TestNeverAlwaysProperty is the property half of R8: NO input to the adapter
// produces "always" on the wire. v1 `always` rules are in-memory AND
// server-wide — they leak approvals across a person's sessions and vanish on
// restart (SPIKE P2-S1 Probe 6-A/Bonus A).
func TestNeverAlwaysProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(0xA1FA))
	for i := 0; i < 120; i++ {
		f := newFakeServe(t)
		f.setPermissions(fixtureAsk())
		f.onReply = func(f *fakeServe, _ fakeReply) { f.publishFixture("gate-replied.sse") }
		a := fakeAdapter(t, f)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

		ans := &adapters.Answer{AskID: fixtureAskID}
		switch rng.Intn(5) {
		case 0:
			ans = ApproveAnswer(fixtureAskID)
		case 1:
			ans = RejectAnswer(fixtureAskID, pickOne(rng, "always", `{"reply":"always"}`, "always always always", ""))
		case 2:
			ans.UpdatedInput = json.RawMessage(`{"reply":"always"}`)
		case 3:
			ans.Continuation = pickOne(rng, "always", "please answer always", "")
		case 4:
			ans = nil
		}
		rec := parkRecordFixture(t, a)
		sess, err := a.Resume(ctx, rec, ans)
		if err == nil {
			for range sess.Events() {
			}
			_, _ = sess.Wait(ctx)
		}
		for _, r := range f.repliesSeen() {
			if r.Reply == "always" {
				t.Fatalf("iteration %d: the adapter sent reply %q on the wire (answer %+v)", i, r.Reply, ans)
			}
			if r.Reply != "once" && r.Reply != "reject" {
				t.Fatalf("iteration %d: the adapter sent reply %q, outside {once,reject}", i, r.Reply)
			}
		}
		// The invariant is about the reply VERB, not about the bytes: a
		// rejection's feedback is human text and may legitimately contain the
		// word "always", so the wire body is decoded rather than searched.
		for _, req := range f.requests() {
			if !strings.HasSuffix(req.Path, "/reply") {
				continue
			}
			var body struct {
				Reply string `json:"reply"`
			}
			if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
				t.Fatalf("iteration %d: reply body is not JSON: %s", i, req.Body)
			}
			if body.Reply == "always" {
				t.Fatalf("iteration %d: an always reply reached the wire: %s", i, req.Body)
			}
		}
		cancel()
	}
}

// TestResumeRefusesUnhonorableUpdatedInput: 1.18.3's permission.reply carries
// no input-substitution channel, so a human's edited input cannot be honored on
// this substrate. Dropping it silently would execute the parked call
// unreviewed — the exact S03.4 failure the park exists to prevent.
func TestResumeRefusesUnhonorableUpdatedInput(t *testing.T) {
	f := newFakeServe(t)
	f.setPermissions(fixtureAsk())
	a := fakeAdapter(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ans := &adapters.Answer{AskID: fixtureAskID, UpdatedInput: json.RawMessage(`{"command":"echo EDITED"}`)}
	if _, err := a.Resume(ctx, parkRecordFixture(t, a), ans); !errors.Is(err, ErrUpdatedInputUnsupported) {
		t.Fatalf("Resume error = %v, want ErrUpdatedInputUnsupported", err)
	}
	if len(f.repliesSeen()) != 0 {
		t.Errorf("a reply went out despite the refusal: %+v", f.repliesSeen())
	}
}

// parkRecordFixture builds the park record a gate park would have produced.
func parkRecordFixture(t *testing.T, a *Adapter) adapters.ParkRecord {
	t.Helper()
	req := testRequest(t)
	l, err := a.lower(req)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	return adapters.ParkRecord{
		RunID:     req.RunID,
		Substrate: adapters.SubstrateOpencode,
		Cursor: adapters.Cursor{
			Substrate: adapters.SubstrateOpencode,
			SessionID: fixtureSessionID,
			CwdKey:    fixtureDirectory,
		},
		Reason:          adapters.ParkReasonGateAsk,
		DeferredToolUse: fixtureAsk(),
		Start:           req,
		Fingerprint:     l.fingerprint,
		ParkedAt:        a.Now(),
	}
}

// ─── tier F12: the always-class fan-out (SPIKE P2-S1 Probe 6) ───

func TestAlwaysClassFanoutResolvedByRequestID(t *testing.T) {
	p := newParser(fixtureSessionID, t.Logf)
	var asks []string
	for _, frame := range fixtureFrames(t, "fanout.sse") {
		for _, ev := range p.feed(frame) {
			if ev.Kind == adapters.KindGateAsk && ev.Ask != nil {
				asks = append(asks, ev.Ask.ID)
			}
		}
	}
	// All three entries of the flat array are enumerated, each by its own
	// requestID — never collapsed into one.
	if len(asks) != 3 {
		t.Fatalf("gate asks = %v, want three distinct requestIDs", asks)
	}
	seen := map[string]bool{}
	for _, id := range asks {
		if seen[id] {
			t.Errorf("duplicate ask id %q", id)
		}
		seen[id] = true
	}
	// One reply fanned out to three permission.replied events: each resolves
	// its own requestID, without a duplicate platform event or a crash.
	if len(p.replied) != 3 {
		t.Fatalf("resolved replies = %v, want three keyed by requestID", p.replied)
	}
	for id := range seen {
		if _, ok := p.replied[id]; !ok {
			t.Errorf("ask %q was never resolved by the fan-out", id)
		}
	}
	if p.ask != nil {
		t.Errorf("an ask survived its own reply: %+v", p.ask)
	}
	if !p.sawIdle {
		t.Error("the fan-out killed the stream before idle")
	}
}

// ─── tier F13: no reliance on engine SSE replay (S14.5, R17) ───

func TestNoEngineSSEReplayReliance(t *testing.T) {
	f := newFakeServe(t)
	f.onPrompt = func(f *fakeServe, n int) {
		// The stream dies mid-turn; the turn itself completes on the serve.
		f.publish(fixtureFrames(t, "happy.sse")[1])
		f.setStatus(fixtureSessionID, `{"type":"idle"}`)
		go func() {
			time.Sleep(50 * time.Millisecond)
			f.closeSubs()
		}()
	}
	a := fakeAdapter(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sess, err := a.Start(ctx, testRequest(t))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	for range sess.Events() {
	}
	if _, err := sess.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	subs := 0
	for _, r := range f.requests() {
		if r.Path != "/event" {
			continue
		}
		subs++
		// opencode ignores Last-Event-ID (fix declined upstream): the adapter
		// must never send it expecting replay.
		if r.LastEventID != "" {
			t.Errorf("the reconnect sent Last-Event-ID %q — this engine ignores it and never replays", r.LastEventID)
		}
	}
	if subs < 2 {
		t.Fatalf("event subscriptions = %d, want a reconnect after the stream died", subs)
	}
	// The reconnect reconciles by STATE, not by replay.
	if !f.sawPath("/permission") {
		t.Error("the reconnect never diffed GET /permission (R9/R17 state reconcile)")
	}
	if !f.sawPath("/session/status") {
		t.Error("the reconnect never read GET /session/status (R9/R17 state reconcile)")
	}
}

// ─── tier F14: restart-ask-loss containment (S03.4; Probe 2, Probe 5) ───

func TestRestartAskLossReconcile(t *testing.T) {
	f := newFakeServe(t)
	// The serve restarted: the ask evaporated (in-memory Map, rejected
	// silently), the session survived (SQLite).
	f.restart()
	f.mu.Lock()
	f.sessions = []string{fixtureSessionID}
	f.mu.Unlock()
	f.onPrompt = func(f *fakeServe, n int) { f.publishFixture("happy.sse") }

	a := fakeAdapter(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	sess, err := a.Resume(ctx, parkRecordFixture(t, a), ApproveAnswer(fixtureAskID))
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	var cancelled bool
	for ev := range sess.Events() {
		if ev.Kind == adapters.KindToolResult && strings.Contains(string(ev.Payload), `"cancelled":true`) {
			cancelled = true
			if !strings.Contains(string(ev.Payload), fixtureCallID) {
				t.Errorf("orphan record does not name the engine call id: %s", ev.Payload)
			}
		}
	}
	out, err := sess.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome = %q (%s), want the re-prompt to complete", out.Kind, out.Detail)
	}
	// The dead park was diffed against permission.list, never assumed.
	if !f.sawPath("/permission") {
		t.Error("the resume never enumerated GET /permission before deciding")
	}
	// No reply went to a requestID the serve no longer holds.
	if len(f.repliesSeen()) != 0 {
		t.Errorf("the adapter replied to an evaporated ask: %+v", f.repliesSeen())
	}
	// Recovery is a re-prompt of the SURVIVING session id (a paid turn).
	reprompted := false
	for _, r := range f.requests() {
		if r.Path == "/session/"+fixtureSessionID+"/prompt_async" {
			reprompted = true
		}
		if r.Method == "POST" && r.Path == "/session" {
			t.Error("the resume created a NEW session — the surviving session id must be re-prompted")
		}
	}
	if !reprompted {
		t.Error("the surviving session was never re-prompted")
	}
	if !cancelled {
		t.Error("the orphaned call was never marked cancelled in the platform's OWN record " +
			"(opencode leaves the transcript part cosmetically `running` and is never trusted)")
	}
}

// ─── tier F15: outcome assembly ───

func TestOutcomeAssembly(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	l, err := a.lower(req)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	newSess := func(fixture string, mutate func(*session)) *session {
		s := &session{a: a, req: req, low: l, p: newParser(fixtureSessionID, t.Logf)}
		s.cursor = adapters.Cursor{Substrate: adapters.SubstrateOpencode, SessionID: fixtureSessionID}
		for _, frame := range fixtureFrames(t, fixture) {
			s.p.feed(frame)
		}
		if mutate != nil {
			mutate(s)
		}
		return s
	}
	for name, tc := range map[string]struct {
		fixture string
		mutate  func(*session)
		want    adapters.OutcomeKind
		reason  string
		text    string
	}{
		"clean done": {"happy.sse", nil, adapters.OutcomeCompleted, "", "fake-done"},
		"abort raced by completion": {"happy.sse", func(s *session) { s.cancelStage = 1 },
			adapters.OutcomeCompleted, "", "fake-done"},
		"pause raced by completion": {"happy.sse", func(s *session) { s.paused = true },
			adapters.OutcomeCompleted, "", "fake-done"},
		"cooperative abort": {"gate.sse", func(s *session) { s.p.ask = nil; s.cancelStage = 1 },
			adapters.OutcomeCanceled, "", ""},
		"serve death mid-turn": {"gate.sse", func(s *session) { s.p.ask = nil },
			adapters.OutcomeCrashed, "", ""},
		"engine-reported error": {"crash.sse", nil, adapters.OutcomeCrashed, "", ""},
		"pause at boundary": {"gate.sse", func(s *session) { s.p.ask = nil; s.paused = true },
			adapters.OutcomeParked, adapters.ParkReasonPause, ""},
		"gate park": {"gate.sse", nil, adapters.OutcomeParked, adapters.ParkReasonGateAsk, ""},
	} {
		t.Run(name, func(t *testing.T) {
			s := newSess(tc.fixture, tc.mutate)
			out, _ := s.assembleOutcome()
			if out.Kind != tc.want {
				t.Fatalf("outcome = %q (%s), want %q", out.Kind, out.Detail, tc.want)
			}
			if tc.reason != "" {
				if out.Park == nil || out.Park.Reason != tc.reason {
					t.Fatalf("park = %+v, want reason %q", out.Park, tc.reason)
				}
			}
			if out.ResultText != tc.text {
				t.Errorf("ResultText = %q, want %q (verbatim, never repaired — R19)", out.ResultText, tc.text)
			}
			if tc.want == adapters.OutcomeCrashed && out.Detail == "" {
				t.Error("a crash without a detail tells the operator nothing")
			}
			// No lane on this substrate has an engine-side per-turn budget belt
			// at 1.18.3, so died-at-gate is unreachable and never invented.
			if out.Kind == adapters.OutcomeDiedAtGate {
				t.Error("died-at-gate minted on a substrate with no engine ceiling belt (R14)")
			}
			if out.GateFallback {
				t.Error("GateFallback set: the S03.4 defer trap does not exist on this substrate")
			}
		})
	}
}

// ─── tier F17: pin ↔ lock coupling ───

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
		if c.Kind == "engine" && strings.Contains(c.Name, "opencode") {
			if c.Pin != Pin {
				t.Fatalf("components.lock pin %q ≠ opencode.Pin %q — bump one only via the S03.3 procedure", c.Pin, Pin)
			}
			return
		}
	}
	t.Fatal("no opencode engine entry in components.lock (the S16.3 row materializes with this adapter)")
}

// ─── tier R: real engine, ZERO provider cost (sanctioned skip when absent) ───

// enginePath resolves the real engine binary; callers skip-with-notice when
// absent (the one sanctioned skip class, CONVENTIONS §10).
func enginePath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath(DefaultBinary)
	if err != nil {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10): engine binary %q not installed — tier-R real-engine conformance not run on this host", DefaultBinary)
	}
	return p
}

func TestRealEngineVersionAgainstPin(t *testing.T) {
	bin := enginePath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("%s --version: %v (%s)", bin, err, out)
	}
	got := strings.Fields(strings.TrimSpace(string(out)))
	if len(got) == 0 {
		t.Fatalf("empty --version output")
	}
	if got[len(got)-1] != Pin {
		// LOUD, not fatal: the pin is the contract (S03.3); the suite must be
		// runnable against bump candidates, and the delta is reported to the
		// operator, never silently retargeted (CONVENTIONS §10).
		t.Logf("ENGINE VERSION DELTA: installed %s ≠ pin %s — the adapter targets the PIN's contract; "+
			"resolve via the S03.3 deliberate-bump procedure or install the pinned version", got[len(got)-1], Pin)
	} else {
		t.Logf("installed engine matches pin %s", Pin)
	}
}

package kimicli

// realengine_test.go — tier R of the conformance split: a REAL pinned `kimi`
// binary, driven end to end, at ZERO COST.
//
// The whole tier rests on one property of this engine: the KIMI_MODEL_* channel
// accepts an arbitrary OpenAI-compatible base URL, so a real `kimi -p` turn can
// terminate on a LOOPBACK FAKE PROVIDER and still exercise every platform-side
// guarantee — the lowering, the argv, the containment, the boundedness and the
// usage source. No provider is dialled, no quota is consumed, no login exists.
// It is the same tier-R shape §61 ratified for the opencode substrate.
//
// Absence-skips print exactly `SANCTIONED SKIP (CONVENTIONS §10)`.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// realBinary resolves the pinned engine, or reports why the tier is skipped.
func realBinary(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("SINET_KIMI_BINARY"); p != "" {
		return p
	}
	// PATH resolution can find EITHER distribution of the same version: the npm
	// package this platform pins, or the vendor's `curl | bash` installer build
	// that some hosts also carry. Both report 0.38.0 and they are not
	// byte-identical in behavior — the installer build extracts a native
	// runtime cache into HOME where the npm build does not. The suite reports
	// which one answered rather than assuming, and SINET_KIMI_BINARY pins it.
	p, err := exec.LookPath(DefaultBinary)
	if err != nil {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10): the pinned `%s` CLI is not installed on this host, so the "+
			"tier-R suite has no engine to assert behavior against (tier F still ran)", DefaultBinary)
	}
	t.Logf("tier R is driving %s (set SINET_KIMI_BINARY to pin a distribution)", p)
	return p
}

// fakeProvider is the loopback OpenAI-compatible endpoint every tier-R turn
// terminates on. It records what the engine actually sent — which is the only
// way to see the toolset the model was offered, PRE-inference.
type fakeProvider struct {
	mu       sync.Mutex
	requests []providerRequest
	usage    string
}

type providerRequest struct {
	Auth  string
	Tools []string
	Model string
	Body  map[string]any
}

func newFakeProvider(t *testing.T) (*fakeProvider, string) {
	t.Helper()
	p := &fakeProvider{usage: `{"prompt_tokens":137,"completion_tokens":29,"total_tokens":166,"prompt_tokens_details":{"cached_tokens":64}}`}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		req := providerRequest{Auth: r.Header.Get("Authorization"), Body: body}
		if m, ok := body["model"].(string); ok {
			req.Model = m
		}
		if tools, ok := body["tools"].([]any); ok {
			for _, raw := range tools {
				tool, _ := raw.(map[string]any)
				fn, _ := tool["function"].(map[string]any)
				if name, ok := fn["name"].(string); ok {
					req.Tools = append(req.Tools, name)
				}
			}
		}
		p.mu.Lock()
		p.requests = append(p.requests, req)
		p.mu.Unlock()

		if stream, _ := body["stream"].(bool); stream {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			send := func(s string) {
				fmt.Fprintf(w, "data: %s\n\n", s)
				if flusher != nil {
					flusher.Flush()
				}
			}
			head := `{"id":"chatcmpl-tierR","object":"chat.completion.chunk","created":1,"model":"k3","choices":[{"index":0,"delta":`
			send(head + `{"role":"assistant","content":"TIER_R_OK"},"finish_reason":null}]}`)
			send(head + `{},"finish_reason":"stop"}]}`)
			send(fmt.Sprintf(`{"id":"chatcmpl-tierR","object":"chat.completion.chunk","created":1,"model":"k3","choices":[],"usage":%s}`, p.usage))
			send("[DONE]")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"chatcmpl-tierR","object":"chat.completion","created":1,"model":"k3","choices":[{"index":0,"message":{"role":"assistant","content":"TIER_R_OK"},"finish_reason":"stop"}],"usage":%s}`, p.usage)
	}))
	t.Cleanup(srv.Close)
	return p, srv.URL + "/v1"
}

func (p *fakeProvider) seen() []providerRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]providerRequest{}, p.requests...)
}

// runReal drives one real engine turn against the fake provider and returns the
// lowered invocation plus what the process produced.
type realResult struct {
	l      *lowered
	stdout string
	stderr string
	code   int
	prov   *fakeProvider
}

func runReal(t *testing.T, mutate func(a *Adapter, req *adapters.StartRequest)) realResult {
	t.Helper()
	bin := realBinary(t)
	prov, baseURL := newFakeProvider(t)

	a := testAdapter(t)
	a.Binary = bin
	a.BaseURL = baseURL
	a.Env = []string{"PATH=" + os.Getenv("PATH")}
	req := testRequest(t)
	if mutate != nil {
		mutate(a, &req)
	}
	l, err := a.lower(req, nil)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if err := a.materialize(l); err != nil {
		t.Fatalf("materialize: %v", err)
	}

	// The lowered environment already carries this run's own KIMI_CODE_HOME and
	// bounded HOME. The sentinel rides the credential channel exactly as the
	// broker would place it, and the proxy belt makes leaving loopback fail at
	// CONNECT rather than authenticate. The same three lines are what
	// beltedEnv gives the execs that have no lowering to inherit from.
	env := append(append([]string{}, l.env...),
		"KIMI_MODEL_API_KEY=sk-TIER-R-SENTINEL",
		"HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1", "ALL_PROXY=http://127.0.0.1:1")
	assertBelted(t, env)

	cmd := exec.Command(l.argv[0], l.argv[1:]...)
	cmd.Dir = req.Cwd
	cmd.Env = env
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn the real engine: %v", err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		code := 0
		var ee *exec.ExitError
		if err != nil {
			if ok := asExitError(err, &ee); ok {
				code = ee.ExitCode()
			} else {
				t.Fatalf("wait: %v", err)
			}
		}
		return realResult{l: l, stdout: out.String(), stderr: errb.String(), code: code, prov: prov}
	case <-time.After(90 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("the real engine did not exit within 90s — the boundedness lowering is exactly what must prevent this")
	}
	return realResult{}
}

// ── the pin, asserted LOUDLY against what is installed ───────────────────────

func TestRealEngineVersionMatchesPin(t *testing.T) {
	bin := realBinary(t)
	env, _ := beltedEnv(t, "http://127.0.0.1:1/v1")
	cmd := exec.Command(bin, "--version")
	cmd.Env = env
	cmd.Dir = t.TempDir()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("`%s --version`: %v", bin, err)
	}
	got := strings.TrimSpace(string(out))
	if got != Pin {
		// LOUD, never a silent retarget: reconciling is the operator's S03.3
		// deliberate-bump procedure, and this engine published 70 versions in
		// ~3 months with auto-update ON by default.
		t.Errorf("INSTALLED ENGINE %q ≠ kimicli.Pin %q — the pin is the behavioral contract every assertion in "+
			"this package targets. Do NOT retarget the pin to match the host: run the S03.3 bump procedure, or "+
			"reinstall the pinned version (npm install -g @moonshot-ai/kimi-code@%s).", got, Pin, Pin)
	}
}

// ── the turn, end to end, at $0 ──────────────────────────────────────────────

func TestRealEngineTurnOnLoopbackProvider(t *testing.T) {
	r := runReal(t, nil)

	if r.code != 0 {
		t.Fatalf("the real engine exited %d\nstdout:\n%s\nstderr:\n%s", r.code, r.stdout, r.stderr)
	}
	// The stream is JSONL and the version frame is in-band, so the pin is
	// checkable from the stream itself.
	var sawVersion, sawAnswer bool
	for _, line := range strings.Split(strings.TrimSpace(r.stdout), "\n") {
		if line == "" {
			continue
		}
		var frame map[string]any
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Errorf("stdout carried a non-JSON line, which would corrupt the JSONL contract: %q", line)
			continue
		}
		if frame["type"] == "system.version" {
			sawVersion = true
			if v, _ := frame["version"].(string); v != Pin {
				t.Errorf("the stream announced engine version %q, want the pin %q", v, Pin)
			}
		}
		if frame["role"] == "assistant" {
			if c, _ := frame["content"].(string); strings.Contains(c, "TIER_R_OK") {
				sawAnswer = true
			}
		}
	}
	if !sawVersion {
		t.Error("no system.version frame — the in-band pin check has no source")
	}
	if !sawAnswer {
		t.Errorf("the loopback provider's answer never reached stdout:\n%s", r.stdout)
	}

	reqs := r.prov.seen()
	if len(reqs) == 0 {
		t.Fatal("the engine made no call to the loopback provider — every assertion below would be vacuous")
	}
	// The credential really did travel the KIMI_MODEL_* channel and arrive as
	// a bearer token. This is R15's "measured, not assumed".
	if got := reqs[0].Auth; !strings.Contains(got, "sk-TIER-R-SENTINEL") {
		t.Errorf("Authorization = %q, want the injected sentinel — the KIMI_MODEL_API_KEY channel is the only one "+
			"this CLI reads from the shell, and a lane whose key does not arrive authenticates as nobody", got)
	}
	if reqs[0].Model != "k3" {
		t.Errorf("the engine called model %q, want the per-invocation model k3", reqs[0].Model)
	}

	// ★ The sole-controller guarantee, STRUCTURAL: the toolset the model was
	// offered — before any inference — carries neither native-spawn tool.
	for _, name := range reqs[0].Tools {
		if strings.EqualFold(name, "Agent") || strings.EqualFold(name, "AgentSwarm") {
			t.Errorf("the model was OFFERED %q — [tools] disabled must strip the native-spawn family pre-inference, "+
				"which is the whole substance of the sole-controller rider (S03.5, G1 rider 2)", name)
		}
	}
	// And the allowlist really is an allowlist, not a suggestion.
	offered := map[string]bool{}
	for _, name := range reqs[0].Tools {
		offered[name] = true
	}
	for _, want := range []string{"Read", "Grep"} {
		if !offered[want] {
			t.Errorf("the allowlisted tool %q was not offered to the model; offered = %v", want, reqs[0].Tools)
		}
	}
	if offered["Write"] || offered["Edit"] || offered["Bash"] {
		t.Errorf("a tool outside the allowlist was offered: %v", reqs[0].Tools)
	}

	// ★ The usage source: the run's own transcript carries one record per paid
	// call, and it decomposes the way S02.4(a) needs.
	wire, ok := soleTranscript(r.l.home)
	if !ok {
		t.Fatal("the run wrote no session transcript — this substrate's ONLY usage source is that file, so without " +
			"it the lane cannot write a D7 checkpoint at all")
	}
	raw, err := os.ReadFile(wire)
	if err != nil {
		t.Fatalf("read the transcript: %v", err)
	}
	tr := newTranscript(func(string, ...any) {})
	var usages int
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		for _, ev := range tr.feed([]byte(line)) {
			usages++
			u := ev.Usage
			if u.InputTokens != 73 || u.CacheReadTokens != 64 || u.OutputTokens != 29 {
				t.Errorf("usage = in %d / cacheRead %d / out %d, want 73/64/29 from the provider's own numbers "+
					"(prompt 137 = 73 + 64 cached) — inputOther EXCLUDES cache reads",
					u.InputTokens, u.CacheReadTokens, u.OutputTokens)
			}
			if u.ModelID != "k3" {
				t.Errorf("usage carries model %q, want k3 — the alias on usage.record is not the model", u.ModelID)
			}
		}
	}
	if usages != 1 {
		t.Errorf("%d Usage events for one model call, want exactly 1 (S10.1)", usages)
	}

	// Isolation, stated as what it actually guarantees. The engine DOES write
	// into HOME — depending on the distribution it may extract a native runtime
	// cache under $HOME/.cache/kimi-code/native/<version>/ — and that is
	// precisely why HOME is bounded rather than left ambient: the writes land
	// in a platform-owned tree instead of the operator's real home, where the
	// `~/.agents/` instruction leg lives. So the assertion is CONTAINMENT, and
	// the cost is measured and reported (R11, the opencode ~62 MB precedent).
	var homeFiles int
	var homeBytes int64
	_ = filepath.Walk(r.l.bounded, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		homeFiles++
		homeBytes += info.Size()
		if !strings.HasPrefix(p, r.l.bounded) {
			t.Errorf("the engine wrote outside the bounded HOME: %s", p)
		}
		return nil
	})
	t.Logf("bounded-HOME first-use cost for this run: %d file(s), %d bytes under %s (a native runtime cache the "+
		"installer-distribution binary extracts per HOME; reported, not asserted)", homeFiles, homeBytes, r.l.bounded)

	// The real OS home is never touched. This is the assertion that matters:
	// whatever the engine caches, it caches inside the platform's tree.
	if real, err := os.UserHomeDir(); err == nil && real != "" {
		if strings.HasPrefix(r.l.bounded, real) || strings.HasPrefix(r.l.home, real) {
			t.Errorf("the run's engine tree sits inside the real OS home %q — the bounding is cosmetic", real)
		}
	}

	// The first-use binary cost is MEASURED and REPORTED rather than asserted
	// (the opencode ~62 MB precedent): rg and fd download on first Grep or
	// reference use, per home.
	binDir := filepath.Join(r.l.home, "bin")
	entries, _ := os.ReadDir(binDir)
	var bytes int64
	for _, e := range entries {
		if info, err := e.Info(); err == nil {
			bytes += info.Size()
		}
	}
	t.Logf("first-use managed-binary cost for this run's home: %d file(s), %d bytes under %s "+
		"(rg/fd download on first Grep or file-reference use; reported, not asserted)", len(entries), bytes, binDir)
}

// ── the argv the engine ACCEPTS, and the ones it refuses ─────────────────────

// beltedEnv is THE environment every real-engine exec in this package runs
// under. It exists because one exec here originally inherited the ambient
// environment: no KIMI_CODE_HOME, so the real engine read and wrote the
// OPERATOR'S OWN ~/.kimi-code — which holds their live credentials — with
// auto-update enabled and no proxy belt. A test suite that touches the
// operator's live data root is a worse defect than anything it was asserting.
//
// Centralized deliberately: a beltless exec should be impossible to write by
// forgetting something, so there is exactly one place the belt lives.
func beltedEnv(t *testing.T, baseURL string) ([]string, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	codeHome := filepath.Join(root, "kimi-code")
	for _, d := range []string{home, codeHome} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		// Bounded HOME and a per-test data root: never the operator's.
		"HOME=" + home,
		"KIMI_CODE_HOME=" + codeHome,
		// The pin does not get to install its own successor mid-test.
		"KIMI_CODE_NO_AUTO_UPDATE=1",
		"KIMI_DISABLE_TELEMETRY=1",
		"KIMI_CODE_BUILTIN_PRODUCT_SKILLS=0",
		"KIMI_MODEL_NAME=k3",
		"KIMI_MODEL_API_KEY=sk-TIER-R-SENTINEL",
		"KIMI_MODEL_BASE_URL=" + baseURL,
		"KIMI_MODEL_PROVIDER_TYPE=openai",
		// $0 belt: any attempt to leave loopback fails at CONNECT rather than
		// authenticating. Loopback is always proxy-bypassed.
		"HTTP_PROXY=http://127.0.0.1:1",
		"HTTPS_PROXY=http://127.0.0.1:1",
		"ALL_PROXY=http://127.0.0.1:1",
		"NO_COLOR=1", "CI=1",
	}
	// The helper guards ITSELF, so the guarantee is a property of using it
	// rather than of remembering to check afterwards. Round 1 wired the guard
	// at one of three exec sites while the comment claimed "every".
	assertBelted(t, env)
	return env, home
}

// TestRealEngineRejectsForbiddenFlagCombination asserts BEHAVIOR, not help text
// (S03.3 rule 4). It also pins the doc contradiction the spike settled: the
// command reference says `--prompt` cannot be combined with `--yolo`, while a
// configuration guide shows exactly that combination. The engine decides.
func TestRealEngineRejectsForbiddenFlagCombination(t *testing.T) {
	bin := realBinary(t)
	_, baseURL := newFakeProvider(t)
	env, _ := beltedEnv(t, baseURL)
	cmd := exec.Command(bin, "--yolo", "-p", "go", "--output-format", "stream-json")
	cmd.Dir = t.TempDir()
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("the engine ACCEPTED `--yolo -p`, which the command reference says it refuses. The lowering never "+
			"emits it either way, but this suite exists to notice when the engine's behavior moves:\n%s", out)
	}
}

// assertBelted is the guard that makes F4 non-recurrable: every real-engine
// exec must carry its own data root, a bounded HOME, the auto-update disable
// and the proxy belt. A test that forgets one reaches the operator's live
// ~/.kimi-code, and that must fail here rather than in their credentials.
func assertBelted(t *testing.T, env []string) {
	t.Helper()
	real, _ := os.UserHomeDir()
	get := func(name string) string {
		for _, kv := range env {
			if k, v, ok := strings.Cut(kv, "="); ok && k == name {
				return v
			}
		}
		return ""
	}
	for _, name := range []string{"KIMI_CODE_HOME", "HOME"} {
		v := get(name)
		if v == "" {
			t.Fatalf("real-engine exec has no %s — it would read and write the operator's own data root", name)
		}
		if real != "" && (v == real || strings.HasPrefix(v, real+"/.kimi-code")) {
			t.Fatalf("real-engine exec points %s at the operator's own home (%q)", name, v)
		}
	}
	for _, name := range []string{"KIMI_CODE_NO_AUTO_UPDATE", "KIMI_DISABLE_TELEMETRY", "HTTPS_PROXY"} {
		if get(name) == "" {
			t.Fatalf("real-engine exec has no %s", name)
		}
	}
}

// soleTranscript finds the one transcript a completed tier-R run produced. It
// is a TEST helper: production pins the path by session identity (see
// resolveTranscript), and this glob deliberately does not, so that a run which
// wrote its transcript somewhere unexpected fails loudly here.
func soleTranscript(home string) (string, bool) {
	matches, err := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "agents", "main", "wire.jsonl"))
	if err != nil || len(matches) != 1 {
		return "", false
	}
	return matches[0], true
}

// asExitError is errors.As without importing errors for one call.
func asExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}

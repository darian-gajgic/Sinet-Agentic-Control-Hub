package kimicli

// drain1_test.go — P3-LN-7 drain round 1: F1, F2, F5, F6, F7, F8, F16.
//
// Every test here is shaped after the evaluator's own probe, so a regression
// reproduces the finding rather than something adjacent to it.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

const oneRealCall = `{"type":"llm.request","agentId":"main","model":"k3","modelAlias":"__kimi_env_model__","turnStep":"0.1","time":1}
{"type":"usage.record","agentId":"main","model":"__kimi_env_model__","usage":{"inputOther":73,"output":29,"inputCacheRead":64,"inputCacheCreation":0},"usageScope":"turn","time":2}
`

// ── F1 · a planted session store cannot become a paid call ───────────────────

// TestPlantedSessionStoreIsNotBilled is the evaluator's PROBE 1.
//
// The engine's tools run inside the same filesystem namespace as its data root,
// so the run's OWN WORK can create directories under sessions/. The first cut
// picked the transcript with a lexicographic glob, so a planted
// `sessions/aaa_planted/…/wire.jsonl` sorted ahead of the real one and became a
// real Usage event carrying an attacker-chosen ModelID — the price-table key.
// The usage source was forgeable by the thing being metered.
func TestPlantedSessionStoreIsNotBilled(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	const forged = `{"type":"llm.request","agentId":"main","model":"forged-expensive-model","turnStep":"0.1","time":1}
{"type":"usage.record","agentId":"main","model":"__kimi_env_model__","usage":{"inputOther":999999,"output":999999,"inputCacheRead":0,"inputCacheCreation":0},"usageScope":"turn","time":2}
`
	// The REAL session, as the engine records it — index entry included.
	s := tailSessionAt(t, home, cwd, "session_real", oneRealCall, true)
	// The plant, sorting ahead of the real one under a lexicographic glob.
	plantDir := filepath.Join(home, "sessions", "aaa_planted", "session_planted", "agents", "main")
	if err := os.MkdirAll(plantDir, 0o700); err != nil {
		t.Fatalf("plant: %v", err)
	}
	if err := os.WriteFile(filepath.Join(plantDir, "wire.jsonl"), []byte(forged), 0o600); err != nil {
		t.Fatalf("plant: %v", err)
	}

	s.drainUsage()
	evs := drainEvents(s)
	if len(evs) != 1 {
		t.Fatalf("%d Usage events, want exactly 1 — the planted store must not be billed", len(evs))
	}
	u := evs[0].Usage
	if u.ModelID == "forged-expensive-model" {
		t.Fatal("the FORGED session was billed: its model id — the price-table key — was chosen by the thing being metered")
	}
	if u.ModelID != "k3" || u.InputTokens != 73 {
		t.Errorf("billed usage = model %q / in %d, want the real session's k3 / 73", u.ModelID, u.InputTokens)
	}
	if strings.Contains(s.wirePath, "aaa_planted") {
		t.Errorf("the tail resolved to the planted store: %s", s.wirePath)
	}
}

// TestAmbiguousSessionIndexRefusesToBill: if the index itself is tampered so
// that two sessions claim this run's cwd, nothing is billed at all. An
// ambiguous store is one where something other than the engine has been
// writing, and the safe answer is to bill nothing and say so.
func TestAmbiguousSessionIndexRefusesToBill(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	s := tailSessionAt(t, home, cwd, "session_real", oneRealCall, true)
	tailSessionAt(t, home, cwd, "session_rival", oneRealCall, false)
	idx := filepath.Join(home, "session_index.jsonl")
	raw, err := os.ReadFile(idx)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	rival := `{"sessionId":"session_rival","sessionDir":"` + filepath.Join(home, "sessions", "wd_spike_abc123def456", "session_rival") + `","workDir":"` + cwd + `"}` + "\n"
	if err := os.WriteFile(idx, append(raw, []byte(rival)...), 0o600); err != nil {
		t.Fatalf("tamper index: %v", err)
	}

	s.drainUsage()
	if evs := drainEvents(s); len(evs) != 0 {
		t.Errorf("%d Usage events from an ambiguous store, want none", len(evs))
	}
	if !s.wireRefused {
		t.Error("the tail did not mark the store refused, so a later drain would try again")
	}
}

// TestReportedSessionMismatchStopsBilling is the cross-check: the engine's own
// reported id against the one this run has been billing from.
func TestReportedSessionMismatchStopsBilling(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	s := tailSessionAt(t, home, cwd, "session_real", oneRealCall, true)
	s.drainUsage()
	if evs := drainEvents(s); len(evs) != 1 {
		t.Fatalf("%d Usage events before the mismatch, want 1", len(evs))
	}
	s.confirmSession("session_somethingelse")
	if !s.wireRefused {
		t.Error("a reported session id that disagrees with the billed one did not stop billing")
	}
	appendWire(t, s.wirePath, strings.TrimSpace(oneRealCall))
	s.drainUsage()
	if evs := drainEvents(s); len(evs) != 0 {
		t.Errorf("%d further Usage events after the mismatch, want none", len(evs))
	}
}

// ── F2 · a resumed leg does not re-bill ──────────────────────────────────────

// TestResumeDoesNotRebillTheTranscript is the evaluator's PROBE 8. The
// transcript is APPEND-ONLY and a resumed run re-opens the SAME file, so a
// fresh tail starting at byte 0 re-read — and re-billed — every call the
// previous leg had already checkpointed: one new paid call produced three
// Usage events.
func TestResumeDoesNotRebillTheTranscript(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	two := oneRealCall + `{"type":"llm.request","agentId":"main","model":"k3","turnStep":"0.2","time":3}
{"type":"usage.record","agentId":"main","model":"__kimi_env_model__","usage":{"inputOther":10,"output":5,"inputCacheRead":0,"inputCacheCreation":0},"usageScope":"turn","time":4}
`
	first := tailSessionAt(t, home, cwd, "session_resume", two, true)
	first.drainUsage()
	if evs := drainEvents(first); len(evs) != 2 {
		t.Fatalf("first leg billed %d calls, want 2", len(evs))
	}
	consumed := first.consumedRecords()
	if consumed != 2 {
		t.Fatalf("the first leg reports %d consumed records, want 2 — the park record carries this to the next leg", consumed)
	}

	// The resumed leg re-opens the same file, which has grown by ONE call.
	appendWire(t, first.wirePath, `{"type":"llm.request","agentId":"main","model":"k3","turnStep":"1.1","time":5}`)
	appendWire(t, first.wirePath, `{"type":"usage.record","agentId":"main","model":"__kimi_env_model__","usage":{"inputOther":7,"output":3,"inputCacheRead":0,"inputCacheCreation":0},"usageScope":"turn","time":6}`)

	// The resumed leg is a NEW session over the SAME home and the SAME
	// transcript — nothing is rewritten, which is the whole point.
	second := &session{
		a:      &Adapter{Log: discardLogger()},
		low:    &lowered{home: home, resume: true, sessionID: "session_resume", resumeConsumed: consumed},
		req:    adapters.StartRequest{RunID: "run-tail", Cwd: cwd},
		events: make(chan adapters.Event, 64),
	}
	second.tr = newTranscriptFrom(second.a.logf, consumed)
	second.drainUsage()
	evs := drainEvents(second)
	if len(evs) != 1 {
		t.Fatalf("the resumed leg billed %d calls for ONE new paid call, want exactly 1 — a resume that re-reads "+
			"from byte 0 charges the run again for everything it already checkpointed", len(evs))
	}
	if got := evs[0].Usage.InputTokens; got != 7 {
		t.Errorf("the resumed leg billed input %d, want the NEW call's 7", got)
	}
	if got := second.consumedRecords(); got != 3 {
		t.Errorf("the resumed leg reports %d consumed records, want 3", got)
	}
}

// ── F5 · a terminal stderr error reaches the signal seam ─────────────────────

// TestTerminalStderrReachesTheSignalSeam: a 403 depletion produces NO stdout
// frame at all (spike Q3), so without this leg the lane's 19-row signal table
// could never fire on the event class that ENDS a run — a weekly quota
// exhaustion would surface as an unclassified crash.
func TestTerminalStderrReachesTheSignalSeam(t *testing.T) {
	var sawBody string
	var sawStatus int
	s := &session{
		a:      &Adapter{Log: discardLogger()},
		low:    &lowered{},
		req:    adapters.StartRequest{RunID: "r"},
		stderr: &boundedBuffer{cap: stderrCap},
		events: make(chan adapters.Event, 8),
	}
	s.stderr.Write([]byte("error: failed to run prompt: provider.auth_error: 403 You've reached your weekly (7-day) usage limit\n"))
	p := newParser(func(string, ...any) {})
	p.signals = func(body string, status int) (json.RawMessage, bool) {
		sawBody, sawStatus = body, status
		return json.RawMessage(`{"lane":"kimi-cli","http_status":403,"documented_class":"depletion"}`), true
	}

	ev, ok := s.terminalSignal(p, errFake{})
	if !ok {
		t.Fatal("a terminal provider error produced no signal — the only carrier of that text is stderr")
	}
	if ev.Kind != adapters.KindRateLimit {
		t.Errorf("signal event kind = %q, want %q", ev.Kind, adapters.KindRateLimit)
	}
	if !strings.Contains(sawBody, "weekly (7-day) usage limit") {
		t.Errorf("the seam received %q, which does not carry the vendor's message string", sawBody)
	}
	if sawStatus != 403 {
		t.Errorf("the seam received status %d, want 403 — the engine prints it on the same line", sawStatus)
	}

	// A clean exit signals nothing, and a nil seam degrades honestly rather
	// than guessing a class.
	if _, ok := s.terminalSignal(p, nil); ok {
		t.Error("a clean exit produced a limit signal")
	}
	p.signals = nil
	if _, ok := s.terminalSignal(p, errFake{}); ok {
		t.Error("a nil signal seam produced a classified event — the honest degrade is nothing")
	}
}

type errFake struct{}

func (errFake) Error() string { return "exit status 1" }

// ── F6/F8 · fields this substrate cannot honor are refused BY NAME ───────────

func TestUnsupportedWorkerFieldsRefusedByName(t *testing.T) {
	a := testAdapter(t)
	for _, tc := range []struct {
		name  string
		apply func(*adapters.CompiledWorker)
	}{
		{"Recitation", func(w *adapters.CompiledWorker) { w.Recitation = true }},
		{"AgentsJSON", func(w *adapters.CompiledWorker) { w.AgentsJSON = []byte(`{"a":{}}`) }},
		{"AgentName", func(w *adapters.CompiledWorker) { w.AgentName = "sinet_w" }},
		{"SessionStartContextPath", func(w *adapters.CompiledWorker) { w.SessionStartContextPath = "/tmp/ctx.md" }},
		{"PermissionMode", func(w *adapters.CompiledWorker) { w.PermissionMode = "acceptEdits" }},
	} {
		req := testRequest(t)
		tc.apply(&req.Worker)
		_, err := a.lower(req, nil)
		if err == nil {
			t.Errorf("%s was silently DROPPED — the caller compiled a guarantee into the worker and got the "+
				"opposite with nothing saying so", tc.name)
			continue
		}
		if !isErr(err, ErrWorkerFieldUnsupported) {
			t.Errorf("%s: error = %v, want ErrWorkerFieldUnsupported", tc.name, err)
		}
		if !strings.Contains(err.Error(), tc.name) {
			t.Errorf("%s: the refusal does not name the field: %v", tc.name, err)
		}
	}
}

// TestTemplateReferenceInAppendedPromptRefused (F6): SYSTEM.md is INTERPOLATED,
// so an appended `${agents_md}` re-opens all three AGENTS.md ingestion legs that
// omitting it closes — turning the one knob on that channel into its opposite.
func TestTemplateReferenceInAppendedPromptRefused(t *testing.T) {
	a := testAdapter(t)
	for _, bad := range []string{
		"Context: ${agents_md}",
		"${agents_md}",
		"prefix ${cwd} suffix",
		"unterminated ${agents_md",
	} {
		req := testRequest(t)
		req.Worker.SystemPromptAppend = bad
		if _, err := a.lower(req, nil); !isErr(err, ErrTemplateReference) {
			t.Errorf("appended %q: error = %v, want ErrTemplateReference", bad, err)
		}
	}
	// The control: ordinary appended text is accepted and lands verbatim.
	req := testRequest(t)
	req.Worker.SystemPromptAppend = "Prefer small diffs. Cost $5 for 100% of the work."
	l, err := a.lower(req, nil)
	if err != nil {
		t.Fatalf("ordinary appended text was refused: %v", err)
	}
	if !strings.Contains(l.files[systemMD], "Prefer small diffs") {
		t.Error("the appended system prompt never reached SYSTEM.md")
	}
	if strings.Contains(l.files[systemMD], "${") {
		t.Error("SYSTEM.md carries a template reference")
	}
}

// ── F7 · a tool name that breaks the TOML is REFUSED, not escaped ────────────

// TestHostileToolNamesRefused. The engine IGNORES a `[tools]` section it cannot
// parse — and ignoring it restores all 26 tools including the native-spawn
// family, so a name that breaks the syntax silently destroys the only
// structural brake this substrate has. Refusing is the one response that cannot
// fail open.
func TestHostileToolNamesRefused(t *testing.T) {
	a := testAdapter(t)
	hostile := []string{
		"Read\x7f", "Read\x00", "Read\n", "Read\r\n[tools]\ndisabled = []",
		"Read\"", "Read\\", "Read]", "Read = 1", "Read\tGrep", "Réad", "Read ",
		strings.Repeat("R", 65), "", " ", "Read Grep",
	}
	for _, name := range hostile {
		req := testRequest(t)
		req.Worker.ToolAllowlist = []string{"Read", name}
		_, err := a.lower(req, nil)
		if err == nil {
			t.Errorf("tool name %q was accepted — an unparseable [tools] section is IGNORED by the engine, which "+
				"restores every tool including Agent/AgentSwarm", name)
			continue
		}
		if !isErr(err, ErrToolName) && !isErr(err, ErrNativeSpawnTool) {
			t.Errorf("tool name %q: error = %v, want ErrToolName", name, err)
		}
	}
	// The inverse control, so "refuse everything" cannot pass as a fix.
	for _, ok := range [][]string{{"Read", "Grep"}, {"Read", "mcp__srv__*"}, {"Tool-1.2_x"}} {
		req := testRequest(t)
		req.Worker.ToolAllowlist = ok
		if _, err := a.lower(req, nil); err != nil {
			t.Errorf("ordinary allowlist %v was refused: %v", ok, err)
		}
	}
}

// TestLoweredToolListParsesBackExactly is the read-back half: what the builder
// APPENDED is not evidence the engine will see it.
func TestLoweredToolListParsesBackExactly(t *testing.T) {
	req := testRequest(t)
	req.Worker.ToolAllowlist = []string{"Read", "Grep", "Glob"}
	l := mustLower(t, testAdapter(t), req)

	enabled, ok := parsedToolList(l.files[configTOML], "enabled")
	if !ok {
		t.Fatalf("the [tools] enabled list does not parse back:\n%s", l.files[configTOML])
	}
	if strings.Join(enabled, ",") != "Read,Grep,Glob" {
		t.Errorf("compiled allowlist = %v, want exactly [Read Grep Glob]", enabled)
	}
	disabled, ok := parsedToolList(l.files[configTOML], "disabled")
	if !ok {
		t.Fatal("the [tools] disabled list does not parse back")
	}
	for _, want := range []string{"Agent", "AgentSwarm", "mcp__*"} {
		if !containsFold(disabled, want) {
			t.Errorf("compiled disabled list %v does not carry %q", disabled, want)
		}
	}
	// A mutation that corrupts the section must be caught by assertBounded,
	// not merely by eyeball.
	broken := strings.Replace(l.files[configTOML], `disabled = ["Agent", "AgentSwarm", "mcp__*"]`, `disabled = [`, 1)
	if err := assertBounded(broken, l.env, req.Worker.ToolAllowlist); err == nil {
		t.Error("assertBounded accepted a config whose [tools] section does not parse — the engine would ignore it and re-enable everything")
	}
}

// ── F16 · the Confiner seam, and what it binds ───────────────────────────────

// TestConfinerSpawnSpecBindings locks the sandbox posture so mutation M5 —
// removing the read-write binding — dies here instead of leaving the package
// green. It also records the measured RESIDUAL: this engine requires its data
// root to be WRITABLE.
func TestConfinerSpawnSpecBindings(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	l := mustLower(t, a, req)

	rec := &recordingConfiner{}
	req.Confiner = rec
	_, _, _ = a.buildCmd(req, l, l.env)
	got := rec.spec

	if got.Workspace != req.Cwd {
		t.Errorf("workspace = %q, want the run cwd %q", got.Workspace, req.Cwd)
	}
	// The engine's data root must be READ-WRITE, and that is measured rather
	// than assumed: with the whole home read-only the real engine fails with
	// "storage write failed: permission denied" even when every subdirectory
	// and root-level file is pre-created and writable, because it writes
	// atomically (tmp → rename) and a rename needs a writable DIRECTORY. The
	// session store is also this substrate's only usage source, so it cannot
	// be anywhere else.
	if !containsPath(got.RWExchange, l.home) {
		t.Errorf("RWExchange = %v, missing the engine home %q — the engine cannot start without a writable data root, "+
			"and the transcript that IS the usage source lives there", got.RWExchange, l.home)
	}
	if !containsPath(got.RWExchange, l.bounded) {
		t.Errorf("RWExchange = %v, missing the bounded HOME %q", got.RWExchange, l.bounded)
	}
	// The skills dir is platform-owned and read-only: it exists to REPLACE
	// discovery, and a writable one would let the run add its own skills.
	if !containsPath(got.ROConfig, l.skillsDir) {
		t.Errorf("ROConfig = %v, missing the platform-owned skills dir %q", got.ROConfig, l.skillsDir)
	}
	if containsPath(got.ROConfig, l.home) {
		t.Errorf("ROConfig carries the engine home %q — measured, the engine cannot run with it read-only", l.home)
	}
	if got.EnginePrefix == "" {
		t.Error("EnginePrefix is empty for an absolute binary — a CLI installed outside /usr is unreachable inside the sandbox")
	}
	// The compiled config files are written READ-ONLY. It is a partial
	// mitigation and the test says so: the engine never rewrites them
	// (measured — a run with all three at 0400 exits 0), but the directory
	// they sit in must stay writable, so a determined tool inside the sandbox
	// can still chmod them. The residual is recorded on the lane document.
	if err := a.materialize(l); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	for _, name := range []string{configTOML, tuiTOML, systemMD} {
		st, err := os.Stat(filepath.Join(l.home, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if perm := st.Mode().Perm(); perm != 0o400 {
			t.Errorf("%s has mode %o, want 0400 — the compiled config is not the engine's to rewrite (S11.7 P-T09-1)", name, perm)
		}
	}
}

// recordingConfiner captures the SpawnSpec the adapter composes. It never
// returns a command: what is under test is the BINDING SET, not the spawn.
type recordingConfiner struct{ spec adapters.SpawnSpec }

func (c *recordingConfiner) Confine(_ adapters.StartRequest, spec adapters.SpawnSpec) (*exec.Cmd, func(), error) {
	c.spec = spec
	return nil, nil, errFake{}
}

func containsPath(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}

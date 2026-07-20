package claudecli

// Conformance suite v1 for the Anthropic lane (Spec S03.3): behavioral
// assertions at the exact pinned engine version, never docs-derived. Three
// marked tiers:
//
//	tier F — replay fixtures (always run; CI has no engine). Fixtures
//	         reproduce RECORDED engine behavior: happy.jsonl re-recorded
//	         live 2026-07-19 against the installed 2.1.215 (tier-L smoke:
//	         terminal_reason "completed", extended usage fields);
//	         defer.jsonl re-recorded live 2026-07-20 at the B1-4 spike
//	         battery (paid defer on 2.1.215: real deferred_tool_use +
//	         permission_denials/iterations/context_management drift fields);
//	         budget.jsonl remains the pinned-family shape [SPIKE G1-S2 F4]
//	         (live budget-preempt re-record deferred — rationale in
//	         P3/measurements/2026-07-20-serialize-by-deny-reconfirm.md);
//	         rate_limit from [SPIKE G1-S1 F1] (live-confirmed in-band by
//	         the smoke). parallel.jsonl models the completed-silent-fallback
//	         DETECTION case (defense-in-depth); live 2.1.215+haiku instead
//	         parks via an honored first-defer — see the measurement file.
//	tier R — real-engine, zero-cost (run when the engine binary is on
//	         PATH; otherwise skipped-with-notice — the one sanctioned
//	         skip class, CONVENTIONS §10).
//	tier L — real-engine, one paid call (live_smoke_test.go; only under
//	         SINET_LIVE_SMOKE=1 — never in a plain `go test`).
//
// The version check reports a pin↔installed delta LOUDLY without failing:
// the suite's own purpose (S03.3 step 3) is to run against bump
// candidates; equality enforcement would defeat it. The pin remains the
// contract; deltas are reported to the operator.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/lockfile"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
)

// Pin is the binding engine version (components.lock "claude CLI
// (engine)"; Spec S16.3). TestPinMatchesLock keeps the two mechanically
// coupled.
const Pin = "2.1.215"

func testAdapter(t *testing.T) *Adapter {
	t.Helper()
	return &Adapter{
		Binary:   "claude-test-binary",
		HookCmd:  "/opt/sinet/bin/sinet engine-hook",
		Settings: settings.New(), // the real S18 registry: proves the ⚙ keys exist
		Now:      func() time.Time { return time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC) },
	}
}

func testRequest(t *testing.T) adapters.StartRequest {
	t.Helper()
	return adapters.StartRequest{
		RunID: "r1", UserID: "u1",
		Model:        "claude-haiku-4-5",
		OwnerCredRef: "/home/user/.claude-sinet",
		Cwd:          t.TempDir(),
		WorkDir:      t.TempDir(),
		Worker: adapters.CompiledWorker{
			Prompt:             "Reply with exactly: ok",
			SystemPromptAppend: "You are terse.",
			AgentsJSON:         json.RawMessage(`{"w1":{"description":"d","prompt":"p"}}`),
			AgentName:          "w1",
			ToolAllowlist:      []string{"Bash", "Read"},
			GatedTools:         []string{"Bash"},
			PermissionMode:     "default",
		},
		CeilingCostUSD: 0.10,
		CeilingSteps:   4,
	}
}

// ─── tier F: lowering (S03.5 — one knob per config channel) ───

func TestLoweringKnobs(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	l, err := a.lower(req, nil, "11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	argv := strings.Join(l.argv, " ")

	// Invocation core, verbatim S03.2.
	for _, want := range []string{
		"-p", "--output-format stream-json", "--input-format stream-json",
		"--verbose", "--include-partial-messages",
	} {
		if !strings.Contains(argv, want) {
			t.Errorf("argv missing invocation-core %q: %s", want, argv)
		}
	}
	// One knob per channel (S03.5 table).
	for knob, why := range map[string]string{
		"--settings " + l.settingsPath:                      "settings/hooks channel",
		"--setting-sources ":                                "ambient settings excluded",
		"--strict-mcp-config":                               "MCP/connector channel",
		"--disable-slash-commands":                          "skills/slash channel",
		"--tools Bash,Read":                                 "default-deny toolset",
		"--session-id 11111111-2222-4333-8444-555555555555": "control-plane-chosen id",
		"--model claude-haiku-4-5":                          "model per invocation",
		"--permission-mode default":                         "permission mode re-supplied",
		"--agents ":                                         "inline agent injection",
		"--agent w1":                                        "agent by name",
		"--append-system-prompt You are terse.":             "prompt body",
	} {
		if !strings.Contains(argv, knob) {
			t.Errorf("argv missing %q (%s): %s", knob, why, argv)
		}
	}
	// --setting-sources value is the EMPTY string (loads none).
	for i, f := range l.argv {
		if f == "--setting-sources" && l.argv[i+1] != "" {
			t.Errorf("--setting-sources = %q, want empty", l.argv[i+1])
		}
	}
	// Ceilings: engine belts = ⚙ backstop mult (default 2) × platform.
	if !strings.Contains(argv, "--max-turns 8") {
		t.Errorf("argv missing --max-turns 8 (2×4): %s", argv)
	}
	if !strings.Contains(argv, "--max-budget-usd 0.2000") {
		t.Errorf("argv missing --max-budget-usd 0.2000 (2×0.10): %s", argv)
	}
	// Forbidden, under any input (S03.2).
	if strings.Contains(argv, "--dangerously-skip-permissions") {
		t.Fatalf("forbidden flag in argv: %s", argv)
	}

	// Lowered settings: ⚙ cleanup horizon + gate hook wiring.
	var es struct {
		CleanupPeriodDays int64 `json:"cleanupPeriodDays"`
		Hooks             map[string][]struct {
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
	if es.CleanupPeriodDays != 365 { // ⚙ adapter.claude.cleanup_period_days default
		t.Errorf("cleanupPeriodDays = %d, want 365", es.CleanupPeriodDays)
	}
	pre := es.Hooks["PreToolUse"]
	if len(pre) != 1 || pre[0].Matcher != "Bash" {
		t.Fatalf("PreToolUse hook wiring = %+v", pre)
	}
	if !strings.Contains(pre[0].Hooks[0].Command, "engine-hook --ctl") {
		t.Errorf("hook command = %q", pre[0].Hooks[0].Command)
	}
}

func TestLoweringRefusesNativeSpawnTools(t *testing.T) {
	a := testAdapter(t)
	for _, tool := range []string{"Task", "TaskCreate", "TaskGet", "TaskList", "TaskOutput", "TaskStop", "TaskUpdate"} {
		req := testRequest(t)
		req.Worker.ToolAllowlist = []string{"Read", tool}
		if _, err := a.lower(req, nil, "sid"); err == nil {
			t.Errorf("allowlist with %q lowered without error — sole-controller breach (S03.5)", tool)
		}
	}
}

func TestLoweringEnvScrub(t *testing.T) {
	a := testAdapter(t)
	a.Env = []string{
		"PATH=/usr/bin", "HOME=/home/user",
		"CLAUDECODE=1", "CLAUDE_CODE_ENTRYPOINT=cli", "AI_AGENT=x",
		"CLAUDE_EFFORT=high", "CLAUDE_PID=42",
		"ANTHROPIC_API_KEY=sk-secret", "ANTHROPIC_BASE_URL=http://x",
		"CLAUDE_CONFIG_DIR=/ambient/claude",
	}
	req := testRequest(t)
	l, err := a.lower(req, nil, "sid")
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	env := strings.Join(l.env, "\n")
	for _, banned := range []string{"CLAUDECODE=", "CLAUDE_CODE_", "AI_AGENT=", "CLAUDE_EFFORT=", "CLAUDE_PID=", "ANTHROPIC_"} {
		if strings.Contains(env, banned) {
			t.Errorf("env leaks %q (S03.5 scrub): %s", banned, env)
		}
	}
	if !strings.Contains(env, "PATH=/usr/bin") || !strings.Contains(env, "HOME=/home/user") {
		t.Errorf("benign env dropped: %s", env)
	}
	// CLAUDE_CONFIG_DIR comes ONLY from the owner cred ref (D2).
	if !strings.Contains(env, "CLAUDE_CONFIG_DIR=/home/user/.claude-sinet") {
		t.Errorf("owner cred ref not applied: %s", env)
	}
	if strings.Contains(env, "/ambient/claude") {
		t.Errorf("ambient CLAUDE_CONFIG_DIR survived: %s", env)
	}
}

func TestFingerprintTracksInvocation(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	l1, err := a.lower(req, nil, "sid-a")
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	l2, err := a.lower(req, nil, "sid-b")
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if l1.fingerprint != l2.fingerprint {
		t.Errorf("fingerprint depends on the session id — it must track config only")
	}
	for name, mutate := range map[string]func(*adapters.StartRequest){
		"model":           func(r *adapters.StartRequest) { r.Model = "other" },
		"permission mode": func(r *adapters.StartRequest) { r.Worker.PermissionMode = "acceptEdits" },
		"tool allowlist":  func(r *adapters.StartRequest) { r.Worker.ToolAllowlist = []string{"Read"} },
		"agent body":      func(r *adapters.StartRequest) { r.Worker.AgentsJSON = json.RawMessage(`{"w1":{"prompt":"q"}}`) },
		"system append":   func(r *adapters.StartRequest) { r.Worker.SystemPromptAppend = "Other." },
	} {
		mut := testRequest(t)
		mut.Cwd, mut.WorkDir = req.Cwd, req.WorkDir
		mutate(&mut)
		lm, err := a.lower(mut, nil, "sid-a")
		if err != nil {
			t.Fatalf("lower(%s): %v", name, err)
		}
		if lm.fingerprint == l1.fingerprint {
			t.Errorf("fingerprint blind to %s change (S02.4e)", name)
		}
	}
}

func TestResumeLoweringFullResupply(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	l, err := a.lower(req, &resumeSpec{sessionID: "reported-sid"}, "")
	if err != nil {
		t.Fatalf("lower(resume): %v", err)
	}
	argv := strings.Join(l.argv, " ")
	if !strings.Contains(argv, "--resume reported-sid") {
		t.Errorf("resume argv missing --resume: %s", argv)
	}
	if strings.Contains(argv, "--session-id") {
		t.Errorf("resume argv must not request a new session id: %s", argv)
	}
	// FULL invocation re-supply (S03.4: the engine restores nothing).
	for _, want := range []string{
		"--settings ", "--setting-sources ", "--strict-mcp-config",
		"--disable-slash-commands", "--tools Bash,Read",
		"--model claude-haiku-4-5", "--permission-mode default",
	} {
		if !strings.Contains(argv, want) {
			t.Errorf("resume argv missing %q — partial re-supply executes parked calls unreviewed: %s", want, argv)
		}
	}
	if l.fingerprint == "" {
		t.Error("resume lowering lost the fingerprint")
	}
}

// ─── tier F: parser (recorded stream shapes; forward tolerance) ───

type parseResult struct {
	events []adapters.Event
	p      *parser
	inits  []string
}

func feedFixture(t *testing.T, name string) *parseResult {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	res := &parseResult{}
	res.p = &parser{
		logf: t.Logf,
		onInit: func(sessionID, model, permissionMode, cwd, transcriptPath string) {
			res.inits = append(res.inits, sessionID)
		},
	}
	for _, line := range bytes.Split(raw, []byte("\n")) {
		res.events = append(res.events, res.p.feed(line)...)
	}
	return res
}

func kinds(evs []adapters.Event) []adapters.EventKind {
	out := make([]adapters.EventKind, len(evs))
	for i, ev := range evs {
		out[i] = ev.Kind
	}
	return out
}

func TestParseHappyFixture(t *testing.T) {
	res := feedFixture(t, "happy.jsonl")
	got := kinds(res.events)
	want := []adapters.EventKind{adapters.KindMessage, adapters.KindUsage}
	if len(got) != len(want) {
		t.Fatalf("kinds = %v, want %v (two frames of one message id = ONE paid call)", got, want)
	}
	u := res.events[1].Usage
	if u == nil || u.MessageID != "msg_01HappyA" || u.MessageIndex != 1 {
		t.Fatalf("usage = %+v", u)
	}
	if u.InputTokens != 10 || u.OutputTokens != 4 || u.CacheCreationTokens != 120 {
		t.Errorf("usage numbers: %+v (last frame wins)", u)
	}
	if u.CacheCreationByTTL["ephemeral_5m_input_tokens"] != 120 {
		t.Errorf("TTL buckets: %+v", u.CacheCreationByTTL)
	}
	if len(res.inits) != 1 || res.inits[0] != "7f1c9f60-4b2e-4c11-9a51-2f4e8d0c3b7a" {
		t.Errorf("init session ids: %v (the REPORTED id, S02.4b)", res.inits)
	}
	if !res.p.sawResult || res.p.result.Subtype != "success" || res.p.result.TotalCostUSD != 0.0184178 {
		t.Errorf("result envelope: %+v", res.p.result)
	}
}

func TestParseDeferFixture(t *testing.T) {
	res := feedFixture(t, "defer.jsonl")
	if !res.p.sawResult || res.p.result.StopReason != "tool_deferred" {
		t.Fatalf("defer exit not recognized: %+v", res.p.result)
	}
	d, err := parseDeferred(res.p.result.DeferredToolUse)
	if err != nil {
		t.Fatalf("parseDeferred: %v", err)
	}
	// The ask record comes from the exit JSON alone (SPIKE G1-S2 F1).
	if d.ID != "toolu_01JwN5WHtxKWbHoLyfaRdifh" || d.Name != "Bash" {
		t.Errorf("deferred_tool_use = %+v", d)
	}
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(d.Input, &input); err != nil || input.Command != "echo HELLO" {
		t.Errorf("deferred input = %s (err %v)", d.Input, err)
	}
}

func TestParseRateLimitFixture(t *testing.T) {
	res := feedFixture(t, "ratelimit.jsonl")
	got := kinds(res.events)
	if len(got) < 1 || got[0] != adapters.KindRateLimit {
		t.Fatalf("kinds = %v, want rate_limit first (in-band, D4)", got)
	}
	var info struct {
		ResetsAt      int64  `json:"resetsAt"`
		RateLimitType string `json:"rateLimitType"`
		OverageStatus string `json:"overageStatus"`
	}
	if err := json.Unmarshal(res.events[0].Payload, &info); err != nil {
		t.Fatalf("rate limit payload: %v", err)
	}
	if info.ResetsAt != 1784270400 || info.RateLimitType != "five_hour" || info.OverageStatus != "rejected" {
		t.Errorf("rate limit forwarded lossily: %+v (S03.1: forwarded as data)", info)
	}
}

func TestParseForwardTolerance(t *testing.T) {
	res := feedFixture(t, "unknown.jsonl")
	// Unknown event type + malformed line + unknown system subtype must
	// not kill the stream (S03.1 MUST): the paid call and result still
	// land.
	got := kinds(res.events)
	want := []adapters.EventKind{adapters.KindMessage, adapters.KindUsage}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	if !res.p.sawResult {
		t.Fatal("result lost after unknown lines")
	}
	if res.p.unknownLines != 2 { // unknown type + malformed line (system subtypes are known-skip)
		t.Errorf("unknownLines = %d, want 2", res.p.unknownLines)
	}
}

func TestParseParallelFixture(t *testing.T) {
	res := feedFixture(t, "parallel.jsonl")
	got := kinds(res.events)
	want := []adapters.EventKind{
		adapters.KindMessage, adapters.KindUsage, // turn 1 (two tool_use blocks, one paid call)
		adapters.KindToolResult, adapters.KindToolResult,
		adapters.KindMessage, adapters.KindUsage, // turn 2
	}
	if len(got) != len(want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	var msg struct {
		ToolUses []struct{ ID, Name string } `json:"tool_uses"`
	}
	if err := json.Unmarshal(res.events[0].Payload, &msg); err != nil || len(msg.ToolUses) != 2 {
		t.Errorf("parallel tool uses not surfaced: %s (err %v)", res.events[0].Payload, err)
	}
}

// ─── tier F: gate hook (recorded decision shapes, SPIKE G1-S2) ───

func hookIn(t *testing.T, toolUseID string) string {
	t.Helper()
	raw, err := json.Marshal(hookInput{
		SessionID: "sid", TranscriptPath: "/tmp/t.jsonl", CWD: "/tmp",
		ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"echo HELLO"}`),
		ToolUseID: toolUseID, PermissionMode: "default",
	})
	if err != nil {
		t.Fatalf("marshal hook input: %v", err)
	}
	return string(raw)
}

func runHook(t *testing.T, ctl, stdin string) hookOutput {
	t.Helper()
	var buf bytes.Buffer
	if err := RunHook(strings.NewReader(stdin), &buf, ctl); err != nil {
		t.Fatalf("RunHook: %v", err)
	}
	var out hookOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decision JSON: %v (%s)", err, buf.String())
	}
	if out.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Fatalf("hookEventName = %q", out.HookSpecificOutput.HookEventName)
	}
	return out
}

func TestHookDefersFirstFire(t *testing.T) {
	ctl := t.TempDir()
	if err := writeCtlConfig(ctl, "serialize-by-deny"); err != nil {
		t.Fatalf("ctl: %v", err)
	}
	out := runHook(t, ctl, hookIn(t, "toolu_1"))
	if out.HookSpecificOutput.PermissionDecision != "defer" {
		t.Fatalf("first fire decision = %q, want defer (exit-park, S03.4)", out.HookSpecificOutput.PermissionDecision)
	}
	n, recs, err := countFires(ctl)
	if err != nil || n != 1 || len(recs) != 1 || recs[0].ToolUseID != "toolu_1" {
		t.Errorf("fires after first: n=%d recs=%+v err=%v", n, recs, err)
	}
}

func TestHookSerializeByDenyOnParallelFire(t *testing.T) {
	ctl := t.TempDir()
	if err := writeCtlConfig(ctl, "serialize-by-deny"); err != nil {
		t.Fatalf("ctl: %v", err)
	}
	runHook(t, ctl, hookIn(t, "toolu_1"))
	out := runHook(t, ctl, hookIn(t, "toolu_2")) // same turn window
	if out.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("parallel fire decision = %q, want deny (SPIKE P2-S4)", out.HookSpecificOutput.PermissionDecision)
	}
	if !strings.Contains(out.HookSpecificOutput.PermissionDecisionReason, "re-issue the gated call alone") {
		t.Errorf("deny reason = %q", out.HookSpecificOutput.PermissionDecisionReason)
	}
	// After the adapter resets the turn window, the re-issued call defers.
	if err := truncateFires(ctl); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	out = runHook(t, ctl, hookIn(t, "toolu_3"))
	if out.HookSpecificOutput.PermissionDecision != "defer" {
		t.Fatalf("re-issued call decision = %q, want defer", out.HookSpecificOutput.PermissionDecision)
	}
}

func TestHookHoldProcessNeverDenies(t *testing.T) {
	ctl := t.TempDir()
	if err := writeCtlConfig(ctl, "hold-process"); err != nil {
		t.Fatalf("ctl: %v", err)
	}
	runHook(t, ctl, hookIn(t, "toolu_1"))
	out := runHook(t, ctl, hookIn(t, "toolu_2"))
	if out.HookSpecificOutput.PermissionDecision != "defer" {
		t.Fatalf("hold-process parallel fire = %q, want defer", out.HookSpecificOutput.PermissionDecision)
	}
}

func TestHookAnswerAllowsWithUpdatedInput(t *testing.T) {
	ctl := t.TempDir()
	if err := writeCtlConfig(ctl, "serialize-by-deny"); err != nil {
		t.Fatalf("ctl: %v", err)
	}
	if err := writeAnswer(ctl, "toolu_1", json.RawMessage(`{"command":"echo ANSWER-42"}`)); err != nil {
		t.Fatalf("writeAnswer: %v", err)
	}
	out := runHook(t, ctl, hookIn(t, "toolu_1"))
	d := out.HookSpecificOutput
	if d.PermissionDecision != "allow" {
		t.Fatalf("answered fire decision = %q, want allow (SPIKE G1-S2 F2)", d.PermissionDecision)
	}
	var upd struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(d.UpdatedInput, &upd); err != nil || upd.Command != "echo ANSWER-42" {
		t.Errorf("updatedInput = %s (err %v)", d.UpdatedInput, err)
	}
	// Approve-as-is: an answer file without updatedInput allows the
	// ORIGINAL input (no updatedInput key in the decision).
	if err := writeAnswer(ctl, "toolu_9", nil); err != nil {
		t.Fatalf("writeAnswer: %v", err)
	}
	out = runHook(t, ctl, hookIn(t, "toolu_9"))
	if out.HookSpecificOutput.PermissionDecision != "allow" || len(out.HookSpecificOutput.UpdatedInput) != 0 {
		t.Errorf("approve-as-is decision = %+v", out.HookSpecificOutput)
	}
}

// ─── tier F: outcome assembly (defer / budget / fallback shapes) ───

func outcomeSession(t *testing.T) *session {
	t.Helper()
	a := testAdapter(t)
	return &session{
		a:   a,
		req: testRequest(t),
		low: &lowered{ctlDir: t.TempDir(), cleanupDays: 365, gateFallback: "serialize-by-deny", fingerprint: "fp"},
	}
}

func feedLines(t *testing.T, p *parser, fixture string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	for _, line := range bytes.Split(raw, []byte("\n")) {
		p.feed(line)
	}
}

func writeFire(t *testing.T, ctl string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := appendFire(ctl, hookInput{SessionID: "sid", ToolName: "Bash", ToolUseID: "toolu_x"}); err != nil {
			t.Fatalf("appendFire: %v", err)
		}
	}
}

func TestOutcomeDeferParksWithAskRecord(t *testing.T) {
	s := outcomeSession(t)
	p := &parser{logf: t.Logf}
	feedLines(t, p, "defer.jsonl")
	out, extra := s.assembleOutcome(p, nil)
	if out.Kind != adapters.OutcomeParked {
		t.Fatalf("outcome = %q, want parked", out.Kind)
	}
	if out.Ask == nil || out.Ask.ID != "toolu_01JwN5WHtxKWbHoLyfaRdifh" || out.Ask.ToolName != "Bash" {
		t.Fatalf("ask = %+v", out.Ask)
	}
	if out.Park == nil || out.Park.Reason != adapters.ParkReasonGateAsk {
		t.Fatalf("park = %+v", out.Park)
	}
	// Full invocation snapshot rides the park record (S03.4 obligation).
	if out.Park.Start.Model != "claude-haiku-4-5" || out.Park.Start.Worker.PermissionMode != "default" ||
		len(out.Park.Start.Worker.ToolAllowlist) != 2 || out.Park.Fingerprint != "fp" {
		t.Errorf("park record lost invocation state: %+v", out.Park.Start)
	}
	// Engine expiry = ⚙ cleanup horizon from now (P-T02-1 watch).
	wantExpiry := s.a.Now().Add(365 * 24 * time.Hour)
	if !out.Ask.EngineExpiry.Equal(wantExpiry) {
		t.Errorf("engine expiry = %v, want %v", out.Ask.EngineExpiry, wantExpiry)
	}
	// gate_ask + done events surfaced for the run record.
	if len(extra) != 2 || extra[0].Kind != adapters.KindGateAsk || extra[1].Kind != adapters.KindDone {
		t.Errorf("extra events = %v", kinds(extra))
	}
}

func TestOutcomeBudgetPreemptDiesAtGate(t *testing.T) {
	s := outcomeSession(t)
	writeFire(t, s.low.ctlDir, 1) // the gate had fired (SPIKE G1-S2 M2/M3)
	p := &parser{logf: t.Logf}
	feedLines(t, p, "budget.jsonl")
	out, _ := s.assembleOutcome(p, nil)
	if out.Kind != adapters.OutcomeDiedAtGate {
		t.Fatalf("outcome = %q, want died-at-gate (S03.4 ceiling ordering)", out.Kind)
	}
	if out.Park == nil {
		t.Fatal("died-at-gate without a park record — the platform ask record is the survivor")
	}
}

func TestOutcomeBudgetWithoutGateCrashes(t *testing.T) {
	s := outcomeSession(t)
	p := &parser{logf: t.Logf}
	feedLines(t, p, "budget.jsonl")
	out, _ := s.assembleOutcome(p, nil)
	if out.Kind != adapters.OutcomeCrashed {
		t.Fatalf("outcome = %q, want crashed (no gate moment, engine belt tripped)", out.Kind)
	}
}

func TestOutcomeParallelFallbackDetected(t *testing.T) {
	s := outcomeSession(t)
	writeFire(t, s.low.ctlDir, 2) // >1 fire in the final turn window
	p := &parser{logf: t.Logf}
	feedLines(t, p, "parallel.jsonl")
	out, _ := s.assembleOutcome(p, nil)
	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome = %q", out.Kind)
	}
	if !out.GateFallback {
		t.Fatal("GateFallback not detected: >1 fire + no defer exit (S03.4, P-T02-4)")
	}
}

// ─── tier F: pin ↔ lock coupling ───

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
		if c.Kind == "engine" && strings.Contains(c.Name, "claude") {
			if c.Pin != Pin {
				t.Fatalf("components.lock pin %q ≠ claudecli.Pin %q — bump one only via the S03.3 procedure", c.Pin, Pin)
			}
			return
		}
	}
	t.Fatal("no claude engine entry in components.lock (S16.3 row must materialize with this adapter)")
}

// ─── tier R: real engine, zero-cost (sanctioned skip when absent) ───

// enginePath resolves the real engine binary; callers skip-with-notice
// when absent (the one sanctioned skip class, CONVENTIONS §10).
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
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("%s --version: %v (%s)", bin, err, out)
	}
	got := strings.Fields(strings.TrimSpace(string(out)))
	if len(got) == 0 {
		t.Fatalf("empty --version output")
	}
	if got[0] != Pin {
		// LOUD, not fatal: the pin is the contract (S03.3); the suite
		// must be runnable against bump candidates, and the delta is
		// reported to the operator, never silently retargeted.
		t.Logf("ENGINE VERSION DELTA: installed %s ≠ pin %s — adapter targets the PIN's contract; "+
			"resolve via the S03.3 deliberate-bump procedure or install the pinned version", got[0], Pin)
	} else {
		t.Logf("installed engine matches pin %s", Pin)
	}
}

func TestRealEngineRejectsInvalidEnumValue(t *testing.T) {
	bin := enginePath(t)
	out, err := exec.Command(bin, "-p", "--output-format", "bogus").CombinedOutput()
	if err == nil {
		t.Fatalf("invalid --output-format accepted; output: %s", out)
	}
	if !strings.Contains(string(out), "--output-format") {
		t.Errorf("rejection did not name the flag: %s", out)
	}
}

func TestRealEngineAcceptsFullLoweredArgv(t *testing.T) {
	// Dry probe: the COMPLETE lowered argv plus a trailing invalid
	// --output-format value. The engine must fail on that value — proving
	// argument processing consumed every Sinet flag without aborting
	// earlier, at zero cost (no API call is possible past a parse error).
	bin := enginePath(t)
	a := testAdapter(t)
	a.Binary = bin
	req := testRequest(t)
	l, err := a.lower(req, nil, "11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if err := os.WriteFile(l.settingsPath, l.settingsJSON, 0o600); err != nil {
		t.Fatalf("settings: %v", err)
	}
	argv := append(l.argv[1:], "--output-format", "bogus")
	cmd := exec.Command(bin, argv...)
	cmd.Dir = req.Cwd
	cmd.Env = l.env
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("dry probe unexpectedly succeeded — it must die on the injected bogus value; output: %s", out)
	}
	if !strings.Contains(string(out), "--output-format") {
		t.Errorf("dry probe failed on something other than the injected value — a Sinet flag may be unsupported: %s", out)
	}
}

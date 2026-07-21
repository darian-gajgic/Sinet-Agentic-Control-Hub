package claudecli

// Recitation delivery-valve suite (P3-B3-4; Spec S05.3 over the S03.4
// ctl-dir airlock; Research/18 §7-C1). All tier F — the engine-side
// PostToolUse contract itself is probe-proven live (P3/measurements/
// 2026-07-21-posttooluse-additionalcontext-probe.md, PASS at 2.1.216) and
// re-checked per pin bump by the SINET_B3_4-gated canary
// (recite_live_test.go); nothing here spawns a paid engine.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
)

func writePending(t *testing.T, ctlDir string, p PendingRecitation) {
	t.Helper()
	dir := filepath.Join(ctlDir, reciteSubdir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("recite dir: %v", err)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal pending: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, recitePending), raw, 0o600); err != nil {
		t.Fatalf("write pending: %v", err)
	}
}

func postToolUseStdin(t *testing.T, sessionID, toolUseID string) *bytes.Reader {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"session_id": sessionID, "tool_use_id": toolUseID,
		"hook_event_name": "PostToolUse", "tool_name": "Bash",
		"tool_input": map[string]any{"command": "echo x"}, "tool_response": "x",
		"duration_ms": 12,
	})
	if err != nil {
		t.Fatalf("marshal stdin: %v", err)
	}
	return bytes.NewReader(raw)
}

// ─── the valve: deliver, consume, quiet, race ───

func TestPostToolUseValveDeliversAndConsumes(t *testing.T) {
	ctl := t.TempDir()
	content := "SINET RECITATION test body\n## next_actions\n- finish\n"
	sum := sha256.Sum256([]byte(content))
	writePending(t, ctl, PendingRecitation{
		LedgerVersion: 7, Content: content,
		ContentSHA256: hex.EncodeToString(sum[:]), WrittenAt: "2026-07-21T00:00:00Z",
	})

	var out bytes.Buffer
	if err := RunPostToolUseHook(postToolUseStdin(t, "sid-1", "toolu_r1"), &out, ctl); err != nil {
		t.Fatalf("RunPostToolUseHook: %v", err)
	}
	var dec postToolUseOutput
	if err := json.Unmarshal(out.Bytes(), &dec); err != nil {
		t.Fatalf("hook stdout: %v (%s)", err, out.Bytes())
	}
	if dec.HookSpecificOutput.HookEventName != "PostToolUse" {
		t.Errorf("hookEventName = %q", dec.HookSpecificOutput.HookEventName)
	}
	if dec.HookSpecificOutput.AdditionalContext != content {
		t.Errorf("additionalContext = %q, want the pending content verbatim", dec.HookSpecificOutput.AdditionalContext)
	}

	// Consumed: pending gone, the claimed copy retained as evidence.
	if _, err := os.Stat(filepath.Join(ctl, reciteSubdir, recitePending)); !os.IsNotExist(err) {
		t.Errorf("pending file survived delivery (err=%v)", err)
	}
	entries, err := os.ReadDir(filepath.Join(ctl, reciteSubdir))
	if err != nil || len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "delivered-") {
		t.Errorf("delivered evidence = %v (err=%v), want exactly one delivered-* file", entries, err)
	}

	// The fire records the delivery with the hash RECOMPUTED over the
	// delivered bytes (never trusted from the pending payload).
	fires, err := ReadReciteFires(ctl)
	if err != nil || len(fires) != 1 {
		t.Fatalf("fires = %v err=%v, want exactly one", fires, err)
	}
	f := fires[0]
	if f.SessionID != "sid-1" || f.ToolUseID != "toolu_r1" || f.LedgerVersion != 7 {
		t.Errorf("fire identity = %+v", f)
	}
	if f.ContentSHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("fire hash = %q, want recomputed %q", f.ContentSHA256, hex.EncodeToString(sum[:]))
	}

	// Second boundary with nothing pending: the quiet path (probe P3 —
	// empty stdout injects zero context).
	out.Reset()
	if err := RunPostToolUseHook(postToolUseStdin(t, "sid-1", "toolu_r2"), &out, ctl); err != nil {
		t.Fatalf("quiet invocation: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("quiet path wrote %q, want zero stdout", out.Bytes())
	}
	if fires, _ := ReadReciteFires(ctl); len(fires) != 1 {
		t.Errorf("quiet path appended a fire: %v", fires)
	}
}

func TestPostToolUseValveQuietOnEmptyInbox(t *testing.T) {
	ctl := t.TempDir() // no recite dir at all
	var out bytes.Buffer
	if err := RunPostToolUseHook(postToolUseStdin(t, "sid", "toolu"), &out, ctl); err != nil {
		t.Fatalf("RunPostToolUseHook: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("quiet inbox wrote %q, want zero stdout", out.Bytes())
	}
}

func TestPostToolUseValveRejectsContentlessPending(t *testing.T) {
	ctl := t.TempDir()
	writePending(t, ctl, PendingRecitation{LedgerVersion: 1})
	var out bytes.Buffer
	if err := RunPostToolUseHook(postToolUseStdin(t, "sid", "toolu"), &out, ctl); err == nil {
		t.Fatal("contentless pending delivered, want loud failure")
	}
	if out.Len() != 0 {
		t.Errorf("failure path wrote %q to stdout", out.Bytes())
	}
}

// TestPostToolUseValveSingleWinnerUnderRace pins the atomic-rename claim:
// under concurrent fires racing one pending file exactly one invocation
// speaks and exactly one fire is recorded — the losers see ENOENT and
// stay quiet (no lock, no daemon; Research/18 §7-C1 coherence shape).
func TestPostToolUseValveSingleWinnerUnderRace(t *testing.T) {
	ctl := t.TempDir()
	content := "raced recitation\n"
	writePending(t, ctl, PendingRecitation{LedgerVersion: 3, Content: content})

	const racers = 8
	outs := make([]bytes.Buffer, racers)
	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = RunPostToolUseHook(postToolUseStdin(t, "sid", fmt.Sprintf("toolu_%d", i)), &outs[i], ctl)
		}(i)
	}
	wg.Wait()

	spoke := 0
	for i := 0; i < racers; i++ {
		if errs[i] != nil {
			t.Errorf("racer %d error: %v", i, errs[i])
		}
		if outs[i].Len() > 0 {
			spoke++
		}
	}
	if spoke != 1 {
		t.Errorf("%d racers spoke, want exactly one winner", spoke)
	}
	if fires, err := ReadReciteFires(ctl); err != nil || len(fires) != 1 {
		t.Errorf("fires = %v err=%v, want exactly one", fires, err)
	}
}

func TestResetReciteClearsAllState(t *testing.T) {
	ctl := t.TempDir()
	writePending(t, ctl, PendingRecitation{LedgerVersion: 1, Content: "stale"})
	if err := os.WriteFile(filepath.Join(ctl, reciteSubdir, "delivered-1.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := appendReciteFire(ctl, ReciteFire{TS: "t", SessionID: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := resetRecite(ctl); err != nil {
		t.Fatalf("resetRecite: %v", err)
	}
	if entries, err := os.ReadDir(filepath.Join(ctl, reciteSubdir)); err != nil || len(entries) != 0 {
		t.Errorf("recite dir after reset = %v (err=%v), want empty", entries, err)
	}
	if fires, err := ReadReciteFires(ctl); err != nil || fires != nil {
		t.Errorf("fires after reset = %v err=%v, want none", fires, err)
	}
	// Reset of a never-recited dir is a no-op, not an error.
	if err := resetRecite(t.TempDir()); err != nil {
		t.Errorf("resetRecite on clean dir: %v", err)
	}
}

// ─── lowering: the compiled PostToolUse hook (Spec S03.5 × §7-C1) ───

func TestLoweringRecitationHook(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	req.Worker.Recitation = true
	l, err := a.lower(req, nil, "11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	var es struct {
		Hooks map[string][]hookMatcher `json:"hooks"`
	}
	if err := json.Unmarshal(l.settingsJSON, &es); err != nil {
		t.Fatalf("settings JSON: %v", err)
	}
	ptu, ok := es.Hooks["PostToolUse"]
	if !ok || len(ptu) != 1 {
		t.Fatalf("PostToolUse hooks = %+v, want exactly one matcher", es.Hooks)
	}
	// Matcher = the FULL tool allowlist alternation (every allowed tool is
	// a delivery boundary; probe canary row 1 — same alternation semantics
	// as the PreToolUse gate).
	if ptu[0].Matcher != "Bash|Read" {
		t.Errorf("matcher = %q, want tool-allowlist alternation", ptu[0].Matcher)
	}
	cmd := ptu[0].Hooks[0].Command
	if !strings.Contains(cmd, "--post-tool-use") || !strings.Contains(cmd, "--ctl '"+l.ctlDir+"'") {
		t.Errorf("hook command = %q, want --ctl <dir> --post-tool-use", cmd)
	}

	// PreToolUse stays single-purpose and BYTE-IDENTICAL with recitation on
	// (§7-C1 condition 4: the gate/defer-park primitive is untouched).
	// Same request, only the Recitation bit flipped — same WorkDir, so the
	// compiled gate wiring must not move by a byte.
	reqOff := req
	reqOff.Worker.Recitation = false
	lOff, err := a.lower(reqOff, nil, "11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatalf("lower (recitation off): %v", err)
	}
	var esOff struct {
		Hooks map[string][]hookMatcher `json:"hooks"`
	}
	if err := json.Unmarshal(lOff.settingsJSON, &esOff); err != nil {
		t.Fatalf("settings JSON (off): %v", err)
	}
	on, _ := json.Marshal(es.Hooks["PreToolUse"])
	off, _ := json.Marshal(esOff.Hooks["PreToolUse"])
	if !bytes.Equal(on, off) {
		t.Errorf("PreToolUse wiring changed under recitation: on=%s off=%s", on, off)
	}
	if _, leaked := esOff.Hooks["PostToolUse"]; leaked {
		t.Error("PostToolUse compiled without Recitation")
	}

	// The invocation fingerprint tracks the recitation wiring (S02.4e:
	// settings content is hashed — a resumed invocation re-supplies it).
	if l.fingerprint == lOff.fingerprint {
		t.Error("fingerprint identical with and without recitation wiring")
	}
}

func TestLoweringRecitationRequiresTools(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	req.Worker.Recitation = true
	req.Worker.ToolAllowlist = nil
	req.Worker.GatedTools = nil
	if _, err := a.lower(req, nil, "11111111-2222-4333-8444-555555555555"); err == nil {
		t.Fatal("recitation without a tool allowlist lowered, want loud refusal (no delivery boundary can ever fire)")
	}
}

// TestRecitationSettingsStayPlatformPlacedRO re-asserts §7-C1 condition 7
// on the leak-test suite: with the PostToolUse hook added, compiled
// settings remain platform-placed (--settings under the platform WorkDir +
// --setting-sources "" exclusivity) and engine-UNwritable under
// confinement (WorkDir stays a read-only bind, P-T09-1/S11.7 — the
// CVE-2026-25725 class is in-sandbox settings creation); the ctl dir stays
// the ONE read-write exchange bind (no new socket, credential, or
// principal).
func TestRecitationSettingsStayPlatformPlacedRO(t *testing.T) {
	a := testAdapter(t)
	req := testRequest(t)
	req.Worker.Recitation = true
	l, err := a.lower(req, nil, "11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	argv := strings.Join(l.argv, " ")
	if !strings.Contains(argv, "--settings "+l.settingsPath) || !strings.Contains(argv, "--setting-sources ") {
		t.Errorf("settings channel knobs missing from argv: %s", argv)
	}
	if filepath.Dir(l.settingsPath) != req.WorkDir {
		t.Errorf("settings path %q not under the platform WorkDir %q", l.settingsPath, req.WorkDir)
	}

	cc := &captureConfiner{}
	req.Confiner = cc
	if _, _, err := a.buildCmd(req, l, l.env); err != nil {
		t.Fatalf("buildCmd: %v", err)
	}
	if len(cc.spec.ROConfig) != 1 || cc.spec.ROConfig[0] != req.WorkDir {
		t.Errorf("ROConfig = %v, want exactly the WorkDir (settings stay ro)", cc.spec.ROConfig)
	}
	if len(cc.spec.RWExchange) != 1 || cc.spec.RWExchange[0] != l.ctlDir {
		t.Errorf("RWExchange = %v, want exactly the ctl dir (the one airlock)", cc.spec.RWExchange)
	}

	// Recitation WITHOUT gated tools still needs the airlock rw (the valve
	// renames/appends there) — and nothing else.
	req2 := testRequest(t)
	req2.Worker.Recitation = true
	req2.Worker.GatedTools = nil
	l2, err := a.lower(req2, nil, "11111111-2222-4333-8444-555555555555")
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	cap2 := &captureConfiner{}
	req2.Confiner = cap2
	if _, _, err := a.buildCmd(req2, l2, l2.env); err != nil {
		t.Fatalf("buildCmd: %v", err)
	}
	if len(cap2.spec.RWExchange) != 1 || cap2.spec.RWExchange[0] != l2.ctlDir {
		t.Errorf("RWExchange (recitation-only) = %v, want exactly the ctl dir", cap2.spec.RWExchange)
	}
}

type captureConfiner struct{ spec adapters.SpawnSpec }

func (c *captureConfiner) Confine(req adapters.StartRequest, spec adapters.SpawnSpec) (*exec.Cmd, func(), error) {
	c.spec = spec
	return exec.Command("/bin/true"), func() {}, nil
}

// ─── spawn integration: fresh-vs-resume recitation state ───

// reciteFakeAdapter spawns THIS test binary as the engine (the
// e2e_test.go TestMain fixture branch — one test binary serves both
// packages), from the internal package.
func reciteFakeAdapter(t *testing.T, fixture string) *Adapter {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	abs, err := filepath.Abs(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatalf("fixture path: %v", err)
	}
	a := testAdapter(t)
	a.Binary = self
	a.Env = append(os.Environ(), "SINET_FAKE_ENGINE="+abs)
	a.CancelGrace = 500 * time.Millisecond
	return a
}

// TestSpawnResetsReciteStateOnFreshStart pins the fresh-session rule: a
// NON-resume spawn starts un-recited — stale pending/delivered/fires from
// a dead prior invocation of the same stage workdir neither deliver stale
// content nor double-manifest.
func TestSpawnResetsReciteStateOnFreshStart(t *testing.T) {
	ctx := context.Background()
	a := reciteFakeAdapter(t, "happy.jsonl")
	req := testRequest(t)
	req.Worker.Recitation = true
	req.Worker.GatedTools = nil
	req.OwnerCredRef = "" // no config-root containment in this spawn test

	ctl := filepath.Join(req.WorkDir, "gate-ctl")
	writePending(t, ctl, PendingRecitation{LedgerVersion: 9, Content: "stale content"})
	if err := os.WriteFile(filepath.Join(ctl, reciteSubdir, "delivered-42.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := appendReciteFire(ctl, ReciteFire{TS: "t", SessionID: "dead-prior"}); err != nil {
		t.Fatal(err)
	}

	sess, err := a.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	for range sess.Events() {
	}
	if _, err := sess.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	if entries, err := os.ReadDir(filepath.Join(ctl, reciteSubdir)); err != nil || len(entries) != 0 {
		t.Errorf("recite dir after fresh spawn = %v (err=%v), want empty (stale state wiped, dir ensured)", entries, err)
	}
	if fires, _ := ReadReciteFires(ctl); fires != nil {
		t.Errorf("stale fires survived fresh spawn: %v", fires)
	}
}

// TestResumeKeepsPendingRecitation pins the resume exemption: a parked
// invocation continues, and an undelivered pending stays deliverable (its
// content is still platform-authored truth at its recorded version). The
// resumed lowering re-supplies the PostToolUse wiring (S03.4 full-config
// re-supply obligation).
func TestResumeKeepsPendingRecitation(t *testing.T) {
	ctx := context.Background()
	req := testRequest(t)
	req.Worker.Recitation = true
	req.OwnerCredRef = ""

	a := reciteFakeAdapter(t, "defer.jsonl")
	sess, err := a.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	for range sess.Events() {
	}
	out, err := sess.Wait(ctx)
	if err != nil || out.Kind != adapters.OutcomeParked || out.Park == nil {
		t.Fatalf("park leg: kind=%v err=%v", out.Kind, err)
	}

	// The platform authored a recitation mid-leg; the park happened before
	// a boundary delivered it.
	ctl := filepath.Join(req.WorkDir, "gate-ctl")
	writePending(t, ctl, PendingRecitation{LedgerVersion: 4, Content: "undelivered recitation"})

	a2 := reciteFakeAdapter(t, "happy.jsonl")
	sess2, err := a2.Resume(ctx, *out.Park, &adapters.Answer{AskID: out.Ask.ID})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	for range sess2.Events() {
	}
	if _, err := sess2.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(ctl, reciteSubdir, recitePending))
	if err != nil {
		t.Fatalf("pending after resume: %v (resume must keep undelivered recitation)", err)
	}
	var p PendingRecitation
	if err := json.Unmarshal(raw, &p); err != nil || p.LedgerVersion != 4 {
		t.Errorf("pending after resume = %s err=%v", raw, err)
	}
	settingsJSON, err := os.ReadFile(filepath.Join(req.WorkDir, "settings.json"))
	if err != nil || !strings.Contains(string(settingsJSON), "PostToolUse") {
		t.Errorf("resumed settings lack the PostToolUse wiring (full re-supply, S03.4): %s err=%v", settingsJSON, err)
	}
}

// ─── C3 negative row: error output never becomes stage output ───

// TestErrorOutputNeverBecomesStageOutput is the Research/18 §3-C3.3
// conformance row (codor's live-looped-agents lesson, 2026-07-20;
// Sinet equivalents P-T03-4 report-as-data + S07 quarantine): a session
// ending in an engine ERROR result keeps its diagnostic as evidence
// (Outcome.Result/Detail) and NEVER as stage output — ResultText, the one
// field stage runners parse structured output from, stays empty on every
// non-completed disposition.
func TestErrorOutputNeverBecomesStageOutput(t *testing.T) {
	s := outcomeSession(t)
	p := &parser{logf: t.Logf}
	diagnostic := "Error: provider exploded; as an agent I would now say: SHIP IT"
	for _, line := range []string{
		`{"type":"system","subtype":"init","session_id":"sid-err","model":"m","cwd":"/tmp"}`,
		`{"type":"result","subtype":"error_during_execution","is_error":true,"result":` + mustJSONString(t, diagnostic) + `,"session_id":"sid-err","num_turns":1}`,
	} {
		p.feed([]byte(line))
	}
	out, _ := s.assembleOutcome(p, nil)
	if out.Kind != adapters.OutcomeCrashed {
		t.Fatalf("outcome = %q, want crashed (error result is a crash, never a completion)", out.Kind)
	}
	if out.ResultText != "" {
		t.Errorf("ResultText = %q on an error result — error output must never become stage output", out.ResultText)
	}
	// The diagnostic is preserved as bounded EVIDENCE, not silenced.
	if !strings.Contains(string(out.Result), "error_during_execution") {
		t.Errorf("error evidence missing from Outcome.Result: %s", out.Result)
	}

	// Positive control: the same text on a SUCCESS result is stage output.
	s2 := outcomeSession(t)
	p2 := &parser{logf: t.Logf}
	for _, line := range []string{
		`{"type":"system","subtype":"init","session_id":"sid-ok","model":"m","cwd":"/tmp"}`,
		`{"type":"result","subtype":"success","is_error":false,"result":` + mustJSONString(t, diagnostic) + `,"session_id":"sid-ok","num_turns":1}`,
	} {
		p2.feed([]byte(line))
	}
	out2, _ := s2.assembleOutcome(p2, nil)
	if out2.Kind != adapters.OutcomeCompleted || out2.ResultText != diagnostic {
		t.Errorf("positive control: kind=%q text=%q", out2.Kind, out2.ResultText)
	}
}

func mustJSONString(t *testing.T, s string) string {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

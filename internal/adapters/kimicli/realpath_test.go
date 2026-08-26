package kimicli

// realpath_test.go — P3-LN-7 drain round 2, N1 + N2.
//
// Round 1 proved three guards by calling them directly: confirmSession was
// invoked by hand, and the resumed session was hand-built instead of going
// through Resume. So the GUARDS were tested and their WIRINGS were not —
// removing `s.confirmSession(id)` from the pump, or reverting spawn's
// `newTranscriptFrom` to `newTranscript`, left the package green while
// reintroducing the finding.
//
// Everything here drives the REAL Adapter.Start / Adapter.Resume against a stub
// engine binary, so the pump, the tail and the lowering are the ones under
// test. $0 and hermetic: the stub is a bash script and no provider exists on
// any path.

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// stubEngine writes a `kimi` stand-in that behaves like the real one: it
// creates its session store and index, appends usage records to its
// transcript, prints the stream frames, and exits 0.
//
// It is driven by SINET_FAKE_* variables, which survive the S03.5 env scrub
// precisely because they are not part of any KIMI_*/CLAUDE_*/ANTHROPIC_* family
// the lowering strips.
func stubEngine(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "stub-kimi")
	const script = `#!/usr/bin/env bash
set -u
home="${KIMI_CODE_HOME:?}"
sid="${SINET_FAKE_SESSION:-session_stub}"
reported="${SINET_FAKE_REPORT:-$sid}"
calls="${SINET_FAKE_CALLS:-1}"
bucket="${SINET_FAKE_BUCKET:-wd_stub_000000000000}"
sdir="$home/sessions/$bucket/$sid"
mkdir -p "$sdir/agents/main"

# The engine writes its index when it creates the session — before the first
# model call, which is what makes the index trustworthy as an identity pin.
if [ ! -s "$home/session_index.jsonl" ]; then
  printf '{"sessionId":"%s","sessionDir":"%s","workDir":"%s"}\n' "$sid" "$sdir" "$PWD" > "$home/session_index.jsonl"
fi

# An optional decoy: a SECOND directory carrying the same session id under a
# different workDirKey bucket. Reachable by the run's own work, which knows
# KIMI_CODE_HOME and can read the index.
if [ -n "${SINET_FAKE_DECOY:-}" ]; then
  ddir="$home/sessions/${SINET_FAKE_DECOY}/$sid/agents/main"
  mkdir -p "$ddir"
  printf '%s\n' '{"type":"llm.request","agentId":"main","model":"decoy-model","turnStep":"0.1","time":1}' > "$ddir/wire.jsonl"
  printf '%s\n' '{"type":"usage.record","agentId":"main","model":"__kimi_env_model__","usage":{"inputOther":111,"output":222,"inputCacheRead":0,"inputCacheCreation":0},"usageScope":"turn","time":2}' >> "$ddir/wire.jsonl"
fi

printf '%s\n' '{"role":"meta","type":"system.version","version":"0.38.0"}'

# When the reported id disagrees with the index, say so EARLY so the ordering
# is deterministic: the cross-check must land before any record is billed.
if [ "$reported" != "$sid" ]; then
  printf '{"role":"meta","type":"session.resume_hint","session_id":"%s","command":"kimi -r %s","content":"hint"}\n' "$reported" "$reported"
fi

i=0
while [ "$i" -lt "$calls" ]; do
  i=$((i+1))
  printf '%s\n' "{\"type\":\"llm.request\",\"agentId\":\"main\",\"model\":\"k3\",\"turnStep\":\"0.$i\",\"time\":$i}" >> "$sdir/agents/main/wire.jsonl"
  printf '%s\n' "{\"type\":\"usage.record\",\"agentId\":\"main\",\"model\":\"__kimi_env_model__\",\"usage\":{\"inputOther\":$((10+i)),\"output\":$i,\"inputCacheRead\":0,\"inputCacheCreation\":0},\"usageScope\":\"turn\",\"time\":$i}" >> "$sdir/agents/main/wire.jsonl"
  printf '%s\n' '{"role":"assistant","content":"stub answer"}'
done

sleep 0.5
if [ "$reported" = "$sid" ]; then
  printf '{"role":"meta","type":"session.resume_hint","session_id":"%s","command":"kimi -r %s","content":"hint"}\n' "$sid" "$sid"
fi
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write stub engine: %v", err)
	}
	return path
}

// realRun drives one REAL Start (or Resume) and collects what the session
// billed. Nothing here reaches into the tail by hand.
type realRun struct {
	sess    adapters.Session
	usages  []*adapters.Usage
	outcome adapters.Outcome
	logs    *bytes.Buffer
}

func newRealAdapter(t *testing.T, fake map[string]string) (*Adapter, *bytes.Buffer) {
	t.Helper()
	logs := &bytes.Buffer{}
	env := []string{"PATH=/usr/bin:/bin"}
	for k, v := range fake {
		env = append(env, k+"="+v)
	}
	return &Adapter{
		Binary:       stubEngine(t, t.TempDir()),
		Root:         t.TempDir(),
		BaseURL:      "https://api.kimi.com/coding/v1",
		ProviderType: "openai",
		Env:          env,
		Log:          slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelInfo})),
	}, logs
}

func drive(t *testing.T, a *Adapter, logs *bytes.Buffer, start func(context.Context) (adapters.Session, error)) realRun {
	t.Helper()
	ctx := context.Background()
	sess, err := start(ctx)
	if err != nil {
		t.Fatalf("start/resume: %v", err)
	}
	r := realRun{sess: sess, logs: logs}
	done := make(chan struct{})
	go func() {
		for ev := range sess.Events() {
			if ev.Kind == adapters.KindUsage {
				r.usages = append(r.usages, ev.Usage)
			}
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("the stub engine did not finish within 60s")
	}
	out, err := sess.Wait(ctx)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	r.outcome = out
	return r
}

func startReq(t *testing.T) adapters.StartRequest {
	t.Helper()
	dir := t.TempDir()
	cwd := filepath.Join(dir, "cwd")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}
	return adapters.StartRequest{
		RunID: "run-realpath", UserID: "sinep", Model: "k3", Cwd: cwd,
		Worker: adapters.CompiledWorker{Prompt: "go", ToolAllowlist: []string{"Read"}},
	}
}

// ── N2 / M10 · the resume guard, through the REAL Resume path ───────────────

// TestResumeThroughRealPathDoesNotRebill is PROBE 8 driven end to end.
//
// Round 1's version hand-built the resumed session and called
// newTranscriptFrom itself, so reverting spawn's wiring to newTranscript —
// which reintroduces F2 in full — left the suite green. This one goes through
// Adapter.Resume, so the wiring IS the thing under test.
func TestResumeThroughRealPathDoesNotRebill(t *testing.T) {
	a, logs := newRealAdapter(t, map[string]string{
		"SINET_FAKE_SESSION": "session_resume",
		"SINET_FAKE_CALLS":   "2",
	})
	req := startReq(t)

	first := drive(t, a, logs, func(ctx context.Context) (adapters.Session, error) { return a.Start(ctx, req) })
	if len(first.usages) != 2 {
		t.Fatalf("the first leg billed %d calls, want 2", len(first.usages))
	}
	cur := first.sess.Cursor()
	if cur.MessageIndex != 2 {
		t.Fatalf("the cursor carries %d consumed records, want 2 — the park record is how the next leg learns "+
			"what was already billed", cur.MessageIndex)
	}

	// The resumed leg re-opens the SAME transcript, which has grown by one
	// call. Everything about the resume comes from the park record.
	a.Env = append(a.Env, "SINET_FAKE_CALLS_OVERRIDE=1")
	for i, kv := range a.Env {
		if strings.HasPrefix(kv, "SINET_FAKE_CALLS=") {
			a.Env[i] = "SINET_FAKE_CALLS=1"
		}
	}
	rec := adapters.ParkRecord{
		RunID: req.RunID, Substrate: adapters.SubstrateKimiCLI,
		Cursor: cur, Reason: adapters.ParkReasonPause, Start: req,
	}
	second := drive(t, a, logs, func(ctx context.Context) (adapters.Session, error) {
		return a.Resume(ctx, rec, &adapters.Answer{Continuation: "carry on"})
	})
	if len(second.usages) != 1 {
		t.Fatalf("the resumed leg billed %d calls for ONE new paid call, want exactly 1 — a resume that re-reads "+
			"the append-only transcript from the start charges the run again for everything it already "+
			"checkpointed (F2)", len(second.usages))
	}
	if got := second.sess.Cursor().MessageIndex; got != 3 {
		t.Errorf("the resumed cursor carries %d consumed records, want 3", got)
	}
}

// ── N2 / M9 · the session cross-check, through the REAL pump ────────────────

// TestReportedSessionMismatchThroughRealPumpStopsBilling.
//
// Round 1 called confirmSession directly, so deleting its call from the pump
// left the suite green — the guard existed and nothing invoked it. Here the
// stub reports a session id that disagrees with its own index, and the only
// thing that can notice is the pump's own wiring.
func TestReportedSessionMismatchThroughRealPumpStopsBilling(t *testing.T) {
	a, logs := newRealAdapter(t, map[string]string{
		"SINET_FAKE_SESSION": "session_indexed",
		"SINET_FAKE_REPORT":  "session_somethingelse",
		"SINET_FAKE_CALLS":   "2",
	})
	r := drive(t, a, logs, func(ctx context.Context) (adapters.Session, error) { return a.Start(ctx, startReq(t)) })

	if len(r.usages) != 0 {
		t.Errorf("billed %d calls from a store whose session id the engine disagrees with, want 0 — the cross-check "+
			"is the only thing standing between a mis-pinned store and a real ledger row", len(r.usages))
	}
	// EITHER ordering must refuse loudly, and which one fires depends on
	// whether the stream reported its id before or after the store became
	// resolvable — a race this test deliberately does not pin. What it pins is
	// that the disagreement is never absorbed silently.
	out := logs.String()
	if !strings.Contains(out, "refusing") {
		t.Errorf("no loud refusal was logged for the session-id mismatch:\n%s", out)
	}
	if !strings.Contains(out, "session_somethingelse") || !strings.Contains(out, "session_indexed") {
		t.Errorf("the refusal does not name BOTH ids that disagree:\n%s", out)
	}
}

// ── N1 / M11 · an ambiguous transcript refuses LOUDLY, never silently ───────

// TestDecoySessionDirectoryRefusesLoudly is the evaluator's PROBE 10.
//
// Two directories sharing the pinned session id under different workDirKey
// buckets used to make transcriptFor return "not found", which drainUsage
// treated as "not created yet": zero usage, no refusal flag, no warning, for
// the life of the run. A SILENT billing stall, reachable by the run's own work
// — the mirror image of the over-billing round 1 closed, and asymmetric with
// the index leg, which had always refused loudly.
func TestDecoySessionDirectoryRefusesLoudly(t *testing.T) {
	a, logs := newRealAdapter(t, map[string]string{
		"SINET_FAKE_SESSION": "session_ambiguous",
		"SINET_FAKE_CALLS":   "2",
		"SINET_FAKE_DECOY":   "wd_decoy_111111111111",
	})
	r := drive(t, a, logs, func(ctx context.Context) (adapters.Session, error) { return a.Start(ctx, startReq(t)) })

	if len(r.usages) != 0 {
		t.Errorf("billed %d calls from an ambiguous store, want 0", len(r.usages))
	}
	// LOUD is half the requirement: silence here is the defect itself.
	out := logs.String()
	if !strings.Contains(out, "refusing to read usage") {
		t.Errorf("the ambiguous store produced no refusal warning — a stall nobody is told about is indistinguishable "+
			"from a run that simply made no paid calls:\n%s", out)
	}
	if !strings.Contains(out, "transcript directories") {
		t.Errorf("the warning does not say WHAT was ambiguous:\n%s", out)
	}
	// And the decoy's numbers never reach the ledger.
	for _, u := range r.usages {
		if u.InputTokens == 111 {
			t.Error("the DECOY's usage was billed")
		}
	}

	// The inverse control, so "refuse everything" cannot pass as a fix: the
	// same stub with no decoy bills normally.
	b, blogs := newRealAdapter(t, map[string]string{
		"SINET_FAKE_SESSION": "session_ambiguous",
		"SINET_FAKE_CALLS":   "2",
	})
	ok := drive(t, b, blogs, func(ctx context.Context) (adapters.Session, error) { return b.Start(ctx, startReq(t)) })
	if len(ok.usages) != 2 {
		t.Errorf("the unambiguous control billed %d calls, want 2 — the refusal must be narrow", len(ok.usages))
	}
}

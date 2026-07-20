package claudecli

// P3-B1-4 spike battery — the serialize-by-deny reconfirm (S02.8 carry-forward
// caveat ≡ S03.4 / S19.6 TBD-BRINGUP parallel-gate fallback measurement).
//
// This is a LIVE, PAID, gated harness in the same standing as
// live_smoke_test.go: it never runs in a plain `go test`. It runs only under
// SINET_B1_4=1 and needs the real gate-hook binary (a built `sinet`) named by
// SINET_HOOK_BIN — because the reconfirm's whole point is to exercise the
// SHIPPED path end-to-end: the adapter spawns the real engine, the engine
// invokes the real `sinet engine-hook` PreToolUse gate, the fires.log
// turn-window heuristic drives serialize-by-deny, and assembleOutcome maps the
// result. Cheapest model (haiku): the reconfirm is PROVISIONAL for the S08
// default worker (see P3/measurements/2026-07-20-serialize-by-deny-reconfirm.md).
//
// It also captures raw stream-json (via the adapter's own lower()) for the
// live re-record of the defer/parallel tier-F fixtures (B1-1 flag).

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
)

func spikeGuard(t *testing.T) string {
	t.Helper()
	if os.Getenv("SINET_B1_4") != "1" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): B1-4 reconfirm runs only under SINET_B1_4=1 (live paid calls)")
	}
	if _, err := exec.LookPath(DefaultBinary); err != nil {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10): engine binary %q not installed", DefaultBinary)
	}
	hook := os.Getenv("SINET_HOOK_BIN")
	if hook == "" {
		t.Skip("SANCTIONED SKIP: SINET_HOOK_BIN (path to a built sinet) required for the live gate")
	}
	return hook
}

// spikeModel is the reconfirm target model. B1-4 ran the B1-1 cheapest
// ("haiku") while the spec left "the default worker model" unpinned; S08
// fixed it at B3-3 (worker.DefaultDutyMap execution seat), so the B3-3
// re-run passes it explicitly via SINET_B1_4_MODEL (see
// P3/measurements/2026-07-21-serialize-by-deny-reconfirm-s08.md).
func spikeModel() string {
	if m := os.Getenv("SINET_B1_4_MODEL"); m != "" {
		return m
	}
	return "haiku"
}

// spikeAdapter builds the shipped adapter wired to the real gate hook.
func spikeAdapter(t *testing.T, hookBin string) *Adapter {
	t.Helper()
	return &Adapter{
		Binary:   DefaultBinary,
		HookCmd:  hookBin + " engine-hook",
		Settings: settings.New(), // registry default: adapter.parallel_gate_fallback = serialize-by-deny
	}
}

// costOf parses total_cost_usd out of an Outcome (Result envelope for parked,
// Totals for completed).
func costOf(out adapters.Outcome) float64 {
	if out.Totals != nil && out.Totals.EngineCostUSD > 0 {
		return out.Totals.EngineCostUSD
	}
	var r struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
		NumTurns     int64   `json:"num_turns"`
	}
	_ = json.Unmarshal(out.Result, &r)
	return r.TotalCostUSD
}

func numTurns(out adapters.Outcome) int64 {
	var r struct {
		NumTurns int64 `json:"num_turns"`
	}
	_ = json.Unmarshal(out.Result, &r)
	return r.NumTurns
}

// driveOnce runs one gated invocation through the shipped adapter to a
// terminal Outcome, draining events.
func driveOnce(t *testing.T, a *Adapter, req adapters.StartRequest) adapters.Outcome {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	sess, err := a.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	for range sess.Events() {
	}
	out, err := sess.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	return out
}

// TestSpikeSingleGatedDefer — E1: a single gated tool-call turn parks cleanly
// on the defer exit-park, ask record built from the exit JSON alone.
func TestSpikeSingleGatedDefer(t *testing.T) {
	hook := spikeGuard(t)
	a := spikeAdapter(t, hook)
	req := adapters.StartRequest{
		RunID: "b14-single", UserID: "op",
		Model:   spikeModel(),
		Cwd:     t.TempDir(),
		WorkDir: t.TempDir(),
		Worker: adapters.CompiledWorker{
			Prompt: "Use the Bash tool exactly once to run this command: echo HELLO. " +
				"Call the tool now; do not explain first.",
			ToolAllowlist: []string{"Bash"},
			GatedTools:    []string{"Bash"},
		},
		CeilingCostUSD: 0.10, CeilingSteps: 3,
	}
	out := driveOnce(t, a, req)
	t.Logf("E1 single-gated outcome=%s gate_fallback=%v cost=$%.6f num_turns=%d detail=%q",
		out.Kind, out.GateFallback, costOf(out), numTurns(out), out.Detail)
	if out.Ask != nil {
		t.Logf("E1 ask: id=%s tool=%s input=%s", out.Ask.ID, out.Ask.ToolName, string(out.Ask.ToolInput))
	}
	if out.Kind != adapters.OutcomeParked || out.Ask == nil {
		t.Fatalf("E1 FAIL: outcome=%s ask=%v (want parked defer)", out.Kind, out.Ask)
	}
	if out.GateFallback {
		t.Errorf("E1 unexpected fallback on a single-call turn")
	}
}

// TestSpikeParallelSerializeByDeny — E2/E3: run gated multi-file reads several
// times; each trial that produces a PARALLEL gated turn must (a) be detected as
// a fallback and (b) be converted by serialize-by-deny into a clean single-call
// park at ~+1 turn. Records the observed parallel-batch hit rate (E4).
func TestSpikeParallelSerializeByDeny(t *testing.T) {
	hook := spikeGuard(t)
	a := spikeAdapter(t, hook)

	const trials = 6
	parallel, cleanPark, completed := 0, 0, 0
	var totalCost float64
	for i := 0; i < trials; i++ {
		cwd := t.TempDir()
		for _, f := range []string{"a.txt", "b.txt"} {
			if err := os.WriteFile(filepath.Join(cwd, f), []byte("contents of "+f+"\n"), 0o600); err != nil {
				t.Fatalf("seed: %v", err)
			}
		}
		req := adapters.StartRequest{
			RunID: "b14-par", UserID: "op",
			Model:   spikeModel(),
			Cwd:     cwd,
			WorkDir: t.TempDir(),
			Worker: adapters.CompiledWorker{
				Prompt: "Read BOTH files a.txt and b.txt. Issue the two Read tool calls " +
					"together in a SINGLE response (parallel tool use), then stop.",
				ToolAllowlist: []string{"Read"},
				GatedTools:    []string{"Read"},
			},
			CeilingCostUSD: 0.10, CeilingSteps: 4,
		}
		out := driveOnce(t, a, req)
		c := costOf(out)
		totalCost += c
		fires, _, _ := countFires(filepath.Join(req.WorkDir, "gate-ctl"))
		t.Logf("trial %d: outcome=%s gate_fallback=%v fires(final-window)=%d num_turns=%d cost=$%.6f ask=%v",
			i, out.Kind, out.GateFallback, fires, numTurns(out), c, out.Ask != nil)
		switch out.Kind {
		case adapters.OutcomeParked:
			cleanPark++
			if out.Ask != nil {
				// faithful single-call ask record (S02.8 fidelity value)
				var in map[string]any
				_ = json.Unmarshal(out.Ask.ToolInput, &in)
				t.Logf("  park ask: id=%s tool=%s input=%v", out.Ask.ID, out.Ask.ToolName, in)
			}
		case adapters.OutcomeCompleted:
			completed++
		}
		if out.GateFallback {
			parallel++
		}
	}
	t.Logf("SUMMARY: trials=%d parallel_fallback_detected=%d clean_park=%d completed=%d total_cost=$%.6f",
		trials, parallel, cleanPark, completed, totalCost)
}

// TestSpikeDeferResumeRoundTrip — the full defer→resume→complete cycle live,
// proving the ask record round-trips through the shipped adapter (the answer is
// staged into the ctl dir and the re-fired PreToolUse returns allow).
func TestSpikeDeferResumeRoundTrip(t *testing.T) {
	hook := spikeGuard(t)
	a := spikeAdapter(t, hook)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	req := adapters.StartRequest{
		RunID: "b14-resume", UserID: "op",
		Model:   spikeModel(),
		Cwd:     t.TempDir(),
		WorkDir: t.TempDir(),
		Worker: adapters.CompiledWorker{
			Prompt:        "Use the Bash tool exactly once to run: echo NEEDS-APPROVAL. Call the tool now.",
			ToolAllowlist: []string{"Bash"},
			GatedTools:    []string{"Bash"},
		},
		CeilingCostUSD: 0.10, CeilingSteps: 3,
	}
	sess, err := a.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	for range sess.Events() {
	}
	out, err := sess.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	parkCost := costOf(out)
	if out.Kind != adapters.OutcomeParked || out.Park == nil || out.Ask == nil {
		t.Fatalf("no park to resume: %s", out.Kind)
	}
	t.Logf("park: ask=%s cost=$%.6f", out.Ask.ID, parkCost)

	// Answer: allow with an updatedInput the human approves.
	ans := &adapters.Answer{AskID: out.Ask.ID, UpdatedInput: json.RawMessage(`{"command":"echo APPROVED-42","description":"approved"}`)}
	rsess, err := a.Resume(ctx, *out.Park, ans)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	for range rsess.Events() {
	}
	rout, err := rsess.Wait(ctx)
	if err != nil {
		t.Fatalf("resume Wait: %v", err)
	}
	t.Logf("resume outcome=%s cost=$%.6f num_turns=%d", rout.Kind, costOf(rout), numTurns(rout))
	if rout.Kind != adapters.OutcomeCompleted {
		t.Fatalf("resume outcome=%s (want completed)", rout.Kind)
	}
}

// TestSpikeCaptureRawFixtures — direct exec of the adapter's OWN lowered argv
// (a.lower) with --include-hook-events, to capture live 2.1.215 stream-json for
// the defer/parallel tier-F fixture re-record. Writes raw to $SINET_RAW_OUT.
func TestSpikeCaptureRawFixtures(t *testing.T) {
	hook := spikeGuard(t)
	outDir := os.Getenv("SINET_RAW_OUT")
	if outDir == "" {
		t.Skip("SINET_RAW_OUT unset: raw capture skipped")
	}
	a := spikeAdapter(t, hook)

	capture := func(name, prompt string, gated []string, cwdFiles map[string]string) {
		cwd := t.TempDir()
		for f, c := range cwdFiles {
			if err := os.WriteFile(filepath.Join(cwd, f), []byte(c), 0o600); err != nil {
				t.Fatalf("seed: %v", err)
			}
		}
		req := adapters.StartRequest{
			RunID: "raw-" + name, UserID: "op", Model: spikeModel(),
			Cwd: cwd, WorkDir: t.TempDir(),
			Worker: adapters.CompiledWorker{
				Prompt: prompt, ToolAllowlist: gated, GatedTools: gated,
			},
			CeilingCostUSD: 0.10, CeilingSteps: 4,
		}
		l, err := a.lower(req, nil, newSessionID())
		if err != nil {
			t.Fatalf("lower: %v", err)
		}
		if err := os.WriteFile(l.settingsPath, l.settingsJSON, 0o600); err != nil {
			t.Fatalf("settings: %v", err)
		}
		if err := writeCtlConfig(l.ctlDir, l.gateFallback); err != nil {
			t.Fatalf("ctl: %v", err)
		}
		if err := truncateFires(l.ctlDir); err != nil {
			t.Fatalf("truncate: %v", err)
		}
		argv := append([]string{}, l.argv[1:]...)
		argv = append(argv, "--include-hook-events")
		cmd := exec.Command(l.argv[0], argv...)
		cmd.Dir = req.Cwd
		cmd.Env = l.env
		msg, _ := json.Marshal(map[string]any{"type": "user", "message": map[string]any{
			"role": "user", "content": []map[string]string{{"type": "text", "text": prompt}}}})
		stdin, _ := cmd.StdinPipe()
		go func() { stdin.Write(append(msg, '\n')); stdin.Close() }()
		raw, _ := cmd.CombinedOutput()
		path := filepath.Join(outDir, name+".raw.jsonl")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatalf("write raw: %v", err)
		}
		t.Logf("captured %s -> %s (%d bytes)", name, path, len(raw))
	}

	capture("defer", "Use the Bash tool exactly once to run: echo HELLO. Call the tool now.",
		[]string{"Bash"}, nil)
	capture("parallel", "Read BOTH files a.txt and b.txt with two Read calls in a SINGLE response, then stop.",
		[]string{"Read"}, map[string]string{"a.txt": "contents of a\n", "b.txt": "contents of b\n"})
}

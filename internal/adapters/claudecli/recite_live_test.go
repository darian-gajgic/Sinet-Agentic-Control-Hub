package claudecli

// The per-pin PostToolUse recitation canary — LIVE, PAID, gated (the
// spike_test.go standing): it never runs in a plain `go test`; it runs
// under SINET_B3_4=1 with the engine installed and a built `sinet` binary
// named by SINET_HOOK_BIN, and re-checks the probe-recorded engine facts
// of P3/measurements/2026-07-21-posttooluse-additionalcontext-probe.md at
// every S03.3 pin bump (S14/S2.8 canary rows, beside the B1-4 SessionStart
// rows; registry machinery lands B5):
//
//	row 1  PostToolUse command hooks fire per matched tool call in -p
//	       stream-json (asserted: a delivery happens at a tool boundary
//	       through the SHIPPED lowering + valve path).
//	row 2  hookSpecificOutput.additionalContext reaches the model
//	       mid-turn; empty stdout injects nothing (asserted: token echo on
//	       the delivery leg, exact NONE on the quiet leg).
//	row 3  stdin payload = PreToolUse fields + tool_response +
//	       duration_ms (asserted here for the identity subset the valve
//	       consumes — session_id + tool_use_id recorded per fire; the FULL
//	       field-set capture is the probe file's method, re-run per bump).
//	row 4  hook lifecycle frames under --include-hook-events (probe
//	       instrumentation only — not exercised by the shipped lowering,
//	       re-checked via the probe method).
//
// Cost ≈ two haiku calls (the probe's scale, ~$0.03); auth rides the
// operator config root (dev posture, B1-4 method).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

func reciteCanaryGuard(t *testing.T) string {
	t.Helper()
	if os.Getenv("SINET_B3_4") != "1" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): PostToolUse canary runs only under SINET_B3_4=1 (live paid calls; re-run per S03.3 pin bump)")
	}
	if _, err := exec.LookPath(DefaultBinary); err != nil {
		t.Skipf("SANCTIONED SKIP (CONVENTIONS §10): engine binary %q not installed", DefaultBinary)
	}
	hook := os.Getenv("SINET_HOOK_BIN")
	if hook == "" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): SINET_HOOK_BIN not set (needs a built sinet binary for the real engine-hook path)")
	}
	return hook
}

func TestLivePostToolUseCanary(t *testing.T) {
	hookBin := reciteCanaryGuard(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	a := &Adapter{HookCmd: hookBin + " engine-hook", Settings: settings.New()}
	token := fmt.Sprintf("SINET-RECITE-%d", time.Now().Unix()%100000)

	runLeg := func(deliver bool) (result string, ctlDir string) {
		req := adapters.StartRequest{
			RunID: "canary", UserID: "u1", Model: "haiku",
			Cwd: t.TempDir(), WorkDir: t.TempDir(),
			Worker: adapters.CompiledWorker{
				Prompt: "Run `echo PROBE-STEP` via Bash once, then run `echo PROBE-STEP-2` via Bash once. " +
					"Then, if a message containing a token of the form SINET-RECITE-<digits> arrived, reply with exactly that token; otherwise reply with exactly NONE.",
				ToolAllowlist: []string{"Bash"},
				Recitation:    true,
			},
			CeilingCostUSD: 0.05, CeilingSteps: 8,
		}
		sess, err := a.Start(ctx, req)
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		ctlDir = filepath.Join(req.WorkDir, "gate-ctl")
		delivered := false
		for ev := range sess.Events() {
			// Author the recitation AFTER the first observed paid call
			// (the platform-reciter shape: dueness from observed turns),
			// so the SECOND tool boundary delivers it.
			if deliver && !delivered && ev.Kind == adapters.KindUsage && ev.Usage != nil && !ev.Usage.Total {
				delivered = true
				content := "Platform note: the recitation token is " + token + "\n"
				sum := sha256.Sum256([]byte(content))
				dir := filepath.Join(ctlDir, reciteSubdir)
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatalf("recite dir: %v", err)
				}
				tmp := filepath.Join(dir, ".pending-tmp")
				raw := fmt.Sprintf(`{"ledger_version":1,"content":%q,"content_sha256":%q,"written_at":%q}`,
					content, hex.EncodeToString(sum[:]), time.Now().UTC().Format(time.RFC3339Nano))
				if err := os.WriteFile(tmp, []byte(raw), 0o600); err != nil {
					t.Fatalf("write pending: %v", err)
				}
				if err := os.Rename(tmp, filepath.Join(dir, recitePending)); err != nil {
					t.Fatalf("rename pending: %v", err)
				}
			}
		}
		out, err := sess.Wait(ctx)
		if err != nil {
			t.Fatalf("Wait: %v", err)
		}
		if out.Kind != adapters.OutcomeCompleted {
			t.Fatalf("leg outcome = %q (%s)", out.Kind, out.Detail)
		}
		return strings.TrimSpace(out.ResultText), ctlDir
	}

	// Delivery leg — canary rows 1 + 2 (inject half) + 3 (identity subset).
	result, ctl := runLeg(true)
	if !strings.Contains(result, token) {
		t.Errorf("delivery leg result = %q, want the token %s (additionalContext must reach the model mid-turn — canary row 2)", result, token)
	}
	fires, err := ReadReciteFires(ctl)
	if err != nil || len(fires) != 1 {
		t.Fatalf("fires = %v err=%v, want exactly one delivery (canary row 1)", fires, err)
	}
	if fires[0].SessionID == "" || fires[0].ToolUseID == "" {
		t.Errorf("fire identity fields empty: %+v (canary row 3 identity subset — payload carried session_id + tool_use_id)", fires[0])
	}

	// Quiet leg — canary row 2 (quiet half): no pending, zero injection.
	result, ctl = runLeg(false)
	if result != "NONE" {
		t.Errorf("quiet leg result = %q, want exactly NONE (empty stdout must inject nothing)", result)
	}
	if fires, _ := ReadReciteFires(ctl); len(fires) != 0 {
		t.Errorf("quiet leg recorded fires: %v", fires)
	}
}

package stage_test

// livetriage_test.go — P3-RW-11 T11, the tier-R live leg (brief §7; R8/R9,
// CONVENTIONS §26 tiers F/R/L, §10 sanctioned skip).
//
// The classifier seam has been landed and unexercised: nothing in any world set
// SINET_LOCAL_ENDPOINT, so `taxonomyFor(FamilyGeneric)` stood for every task.
// This runs the REAL S12.4 `intake-triage` duty against a REAL llama-swap on an
// ephemeral loopback port, with a config generated from TODAY'S manifest, and
// asserts the thing the packet exists to make true: "Create a simple webshop"
// classifies `software` without a registry and without asking. $0 by
// construction (lane `local` prices a true zero-allowance, §26/R18).
//
// It AUTO-RUNS where the stack is installed and SANCTIONED-SKIPS where it is
// not. Resolution order is the §26 one — local.InstalledStack() first (PATH +
// the SINET_LOCAL_LLAMA_* overrides) — then the operator's user-level stack
// root, which is where the B4-5 drain actually put the binaries and weights.
// Nothing here installs, changes or reads the host beyond those paths.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

const liveSanctionedSkip = "SANCTIONED SKIP (CONVENTIONS §10): "

// installedStack resolves the three paths a live classify needs. ok=false ⇒ the
// real stack is absent on this machine and the leg sanctioned-skips.
func installedStack() (llamaSwap, llamaServer, modelCache string, ok bool) {
	llamaSwap, llamaServer, ok = local.InstalledStack()
	modelCache = os.Getenv("SINET_LOCAL_MODEL_CACHE")
	root := os.Getenv("SINET_LOCAL_STACK_ROOT")
	if root == "" {
		if home, err := os.UserHomeDir(); err == nil {
			root = filepath.Join(home, ".sinet-b45")
		}
	}
	if root != "" {
		if !ok {
			llamaSwap = existingFile(llamaSwap, filepath.Join(root, "bin", "llama-swap"))
			llamaServer = existingFile(llamaServer, filepath.Join(root, "build", "llama.cpp", "build-cuda", "bin", "llama-server"))
		}
		if modelCache == "" {
			modelCache = filepath.Join(root, "models")
		}
	}
	if llamaSwap == "" || llamaServer == "" || modelCache == "" {
		return "", "", "", false
	}
	if _, err := os.Stat(modelCache); err != nil {
		return "", "", "", false
	}
	return llamaSwap, llamaServer, modelCache, true
}

func existingFile(have, candidate string) string {
	if have != "" {
		return have
	}
	if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
		return candidate
	}
	return ""
}

// fastSeatPresent reports whether the seat `intake-triage` resolves to has its
// weights on disk. A missing GGUF is an absent stack, not a failure.
func fastSeatPresent(modelCache string) bool {
	for _, s := range local.Manifest() {
		if s.Seat != "fast" || !s.Pulled || len(s.Files) == 0 {
			continue
		}
		if _, err := os.Stat(filepath.Join(modelCache, s.Files[0].Name)); err == nil {
			return true
		}
	}
	return false
}

func liveFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func liveWaitReady(url string) bool {
	for i := 0; i < 120; i++ {
		if resp, err := http.Get(url); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

// TestLiveIntakeTriageClassifiesWebshop (T11): the real 4B fast seat classifies
// the operator's own checkpoint-3 request, and the $0 D7 row lands on the
// consuming run with the model hash and engine build that produced it.
func TestLiveIntakeTriageClassifiesWebshop(t *testing.T) {
	llamaSwap, llamaServer, modelCache, ok := installedStack()
	if !ok {
		t.Skip(liveSanctionedSkip + "local serving stack not installed (llama-swap + llama-server + model cache absent) — host install is the B4 gate/hardening step")
	}
	if !fastSeatPresent(modelCache) {
		t.Skip(liveSanctionedSkip + "the intake-triage `fast` seat's weights are not in the model cache")
	}

	ctx := context.Background()
	cfg, err := local.GenerateConfig(local.ConfigParams{
		ModelCacheDir: modelCache, GPUUUIDs: liveGPUUUIDs(t), LlamaServer: llamaServer,
		TTLFastS: 120, TTLWorkhorseS: 300, UnloadGraceS: 5,
	})
	if err != nil {
		t.Fatalf("GenerateConfig from today's manifest: %v", err)
	}
	cfgPath := filepath.Join(t.TempDir(), "llamaswap.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	port := liveFreePort(t)
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	cmd := exec.Command(llamaSwap, "--config", cfgPath, "--listen", fmt.Sprintf("127.0.0.1:%d", port))
	if err := cmd.Start(); err != nil {
		t.Fatalf("start llama-swap: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()
	if !liveWaitReady(base + "/v1/models") {
		t.Fatal("llama-swap did not accept the generated config / did not become ready")
	}

	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	runs := run.NewStore(db, log)
	const taskID = "t-webshop"
	runID := taskID + stage.RunSuffixIntake
	if _, err := runs.Create(ctx, run.NewRun{ID: runID, UserID: "operator", Lane: "anthropic", Substrate: "claude-cli"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, runID, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("Transition: %v", err)
		}
	}

	duty := local.NewDuty(local.DutyDeps{
		Registry: local.NewRegistry(reg), Client: local.NewClient(base),
		Checkpoints: gates.NewCheckpoints(db, log), Events: log,
	})
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	prop, err := stage.NewLocalClassifier(duty).Classify(callCtx, intake.Request{
		TaskID: taskID, UserID: "operator",
		Title: "Create a simple webshop",
		Text:  "Create a simple webshop with a product list, a cart and a checkout page.",
	}, nil)
	if err != nil {
		t.Fatalf("live Classify: %v", err)
	}
	t.Logf("tier R live triage: family=%q tier=%q size=%q", prop.Family, prop.Tier, prop.Est.SizeClass)
	if prop.Family != intake.FamilySoftware {
		t.Errorf("family = %q, want software — the live classifier is what makes the software taxonomy reachable (R8)", prop.Family)
	}

	// The ONE $0 D7 row on the consuming run, carrying the local marker with the
	// model hash + engine build (§26 wire contract; R9).
	var usage string
	if err := db.QueryRowContext(ctx,
		`SELECT usage_json FROM checkpoints WHERE run_id = ? ORDER BY event_seq DESC LIMIT 1`, runID).Scan(&usage); err != nil {
		t.Fatalf("read the $0 D7 checkpoint row: %v", err)
	}
	var w struct {
		Local *struct {
			Lane        string `json:"lane"`
			Duty        string `json:"duty"`
			Model       string `json:"model"`
			ModelSHA256 string `json:"model_sha256"`
			EngineBuild string `json:"engine_build"`
		} `json:"local"`
	}
	if err := json.Unmarshal([]byte(usage), &w); err != nil {
		t.Fatalf("decode usage block: %v: %s", err, usage)
	}
	if w.Local == nil {
		t.Fatalf("no local marker on the D7 row: %s", usage)
	}
	if w.Local.Lane != "local" || w.Local.ModelSHA256 == "" || w.Local.EngineBuild == "" {
		t.Errorf("local marker incomplete: %+v (the $0 row must name the model hash and engine build)", *w.Local)
	}
}

// liveGPUUUIDs resolves the placement UUID the generated config needs (S12.2:
// placement is by UUID, never index). Production reads it through nvidia-smi
// (local.GPUUUIDs) and that stays the only production path.
//
// The test adds ONE fallback and nothing else: the NVIDIA kernel driver's own
// procfs record. NVML — which nvidia-smi is a client of — refuses with
// "Driver/library version mismatch" on a host whose driver package was upgraded
// under a running kernel module, a reboot-pending condition that leaves CUDA
// itself perfectly working. Skipping there would report "no stack" on a machine
// whose GPU is serving fine, so the leg reads the same real identity from the
// driver instead of inventing one, and says out loud that it did. A fabricated
// UUID is never used — no UUID anywhere means a genuine sanctioned skip.
func liveGPUUUIDs(t *testing.T) []string {
	t.Helper()
	if u, err := local.GPUUUIDs(context.Background()); err == nil && len(u) > 0 {
		return u
	}
	if u := procfsGPUUUIDs(); len(u) > 0 {
		t.Logf("NOTICE: nvidia-smi/NVML did not answer; placement UUID read from the driver's own procfs record instead: %v", u)
		return u
	}
	t.Skip(liveSanctionedSkip + "no GPU UUID readable (nvidia-smi and the driver procfs record both silent) — placement is by UUID, never index (S12.2)")
	return nil
}

// procfsGPUUUIDs reads /proc/driver/nvidia/gpus/*/information for its "GPU
// UUID:" line — a pure file read that never wakes or polls the device.
func procfsGPUUUIDs() []string {
	dirs, err := filepath.Glob("/proc/driver/nvidia/gpus/*/information")
	if err != nil {
		return nil
	}
	var out []string
	for _, path := range dirs {
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(body), "\n") {
			if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "GPU UUID:"); ok {
				if u := strings.TrimSpace(rest); u != "" {
					out = append(out, u)
				}
			}
		}
	}
	return out
}

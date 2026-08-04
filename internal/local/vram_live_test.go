package local_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

// vram_live_test.go — the tier-L live core (brief R19), $0 by construction, only
// under SINET_LIVE_SMOKE=1 on the real GPU: the sleep-gated VRAM reads
// (runtime_status → device-truth memory.free → per-process incl. graphics), the
// admission math on live readings, and the SIGTERM→grace→SIGKILL of a REAL
// llama-server backend with memory.free recovery verified (kill-not-freeze).
// The sandboxed plane stays hermetic (no live confined consumer — BINDING).

func requireLiveSmoke(t *testing.T) {
	t.Helper()
	if os.Getenv("SINET_LIVE_SMOKE") != "1" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): live GPU smoke only under SINET_LIVE_SMOKE=1 ($0)")
	}
}

// TestLiveVRAMReadsAndAdmission exercises the live sleep-gated read + admission
// math against the real GPU (R8/R9/R19).
func TestLiveVRAMReadsAndAdmission(t *testing.T) {
	requireLiveSmoke(t)
	ctx := context.Background()
	adm := local.NewVRAMAdmitter(local.VRAMDeps{Settings: settings.New()})

	snap, err := adm.Snapshot(ctx)
	if err != nil {
		t.Fatalf("live Snapshot: %v", err)
	}
	if len(snap.GPUs) == 0 {
		t.Fatal("no NVIDIA GPU enumerated from sysfs")
	}
	g := snap.GPUs[0]
	t.Logf("live GPU: pci=%s suspended=%v free=%d/%d MiB procs=%d on_battery=%v guard=%d mode=%s",
		g.PCIAddr, g.Suspended, g.MemFreeMiB, g.MemTotalMiB, len(g.Processes), snap.OnBattery, snap.GuardMiB, snap.BatteryMode)
	if !g.Suspended {
		if g.MemFreeMiB <= 0 || g.MemTotalMiB <= 0 {
			t.Errorf("active GPU device-truth memory looks wrong: free=%d total=%d", g.MemFreeMiB, g.MemTotalMiB)
		}
	}
	ok, reason, err := adm.AdmitLoad(ctx, "Qwen3.5-9B")
	if err != nil {
		t.Fatalf("live AdmitLoad: %v", err)
	}
	t.Logf("live admission: ok=%v reason=%q", ok, reason)
}

// TestLiveKillNotFreeze spawns a REAL llama-server holding VRAM, then applies
// the SIGTERM→grace→SIGKILL discipline and verifies memory.free recovery
// (R11/R12/R19). Set SINET_LIVE_LLAMA_SERVER + SINET_LIVE_MODEL. $0.
func TestLiveKillNotFreeze(t *testing.T) {
	requireLiveSmoke(t)
	server := os.Getenv("SINET_LIVE_LLAMA_SERVER")
	model := os.Getenv("SINET_LIVE_MODEL")
	if server == "" || model == "" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): set SINET_LIVE_LLAMA_SERVER + SINET_LIVE_MODEL for the kill leg")
	}
	ctx := context.Background()
	adm := local.NewVRAMAdmitter(local.VRAMDeps{Settings: settings.New()})

	// Free before the backend loads.
	pre, err := adm.Snapshot(ctx)
	if err != nil {
		t.Fatalf("pre Snapshot: %v", err)
	}
	preFree := pre.GPUs[0].MemFreeMiB

	cmd := exec.Command(server, "--model", model, "--port", "0", "--n-gpu-layers", "999", "--ctx-size", "2048")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn llama-server: %v", err)
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()
	defer func() { _ = cmd.Process.Kill() }()

	// Wait until the backend is HOLDING VRAM on the GPU (model loaded).
	held := false
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := adm.Snapshot(ctx)
		if err == nil && !snap.GPUs[0].Suspended {
			for _, p := range snap.GPUs[0].Processes {
				if int(p.PID) == pid && p.UsedMiB > 0 {
					held = true
				}
			}
		}
		if held {
			t.Logf("backend pid=%d holding VRAM; free now %d MiB (was %d)", pid, mustFree(t, adm), preFree)
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !held {
		t.Fatal("llama-server never showed as holding VRAM (model did not load onto the GPU)")
	}

	// Kill-not-freeze: SIGTERM → grace → SIGKILL, then verify recovery (R11/R9).
	p := local.NewPreemptor(local.PreemptorDeps{Settings: settings.New(), Admitter: adm})
	if err := p.KillBackend(ctx, pid); err != nil {
		t.Fatalf("KillBackend (recovery must verify): %v", err)
	}
	post := mustFree(t, adm)
	t.Logf("post-kill free=%d MiB (pre-load was %d) — VRAM returned, recovery verified", post, preFree)
	if post+256 < preFree {
		t.Errorf("post-kill free %d MiB did not recover toward pre-load %d MiB (zombie VRAM?)", post, preFree)
	}
	// Admission is un-gated again (recovery cleared).
	if ok, reason, err := adm.AdmitLoad(ctx, "Qwen3.5-9B"); err != nil || !ok {
		t.Fatalf("post-recovery admit: ok=%v reason=%q err=%v", ok, reason, err)
	}
}

func mustFree(t *testing.T, adm *local.VRAMAdmitter) int64 {
	t.Helper()
	snap, err := adm.Snapshot(context.Background())
	if err != nil || len(snap.GPUs) == 0 {
		t.Fatalf("Snapshot: %v", err)
	}
	return snap.GPUs[0].MemFreeMiB
}

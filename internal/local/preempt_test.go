package local_test

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
)

// preempt_test.go — hermetic tests of the S12.7 kill-not-freeze discipline
// (brief R11/R12/R18): a REAL child process that IGNORES SIGTERM proves the
// MANDATORY SIGKILL fires; a pid-aware fake reader models VRAM freeing on
// teardown so recovery verification is exercised. No GPU, no network.

// pidAwareReader models a GPU whose VRAM is held by a backend WHILE its pid is
// alive and freed once it dies — so post-kill recovery is a real transition.
type pidAwareReader struct{ pid int }

func (r *pidAwareReader) Read(context.Context) ([]local.GPUReading, error) {
	if syscall.Kill(r.pid, 0) == nil { // alive
		return []local.GPUReading{local.FakeActiveGPU(8000, 12227,
			local.GPUProcess{PID: int64(r.pid), Type: "C", Name: "llama-server", UsedMiB: 2000})}, nil
	}
	return []local.GPUReading{local.FakeActiveGPU(12000, 12227)}, nil // freed
}

// startChild starts a real child; ignoreTerm makes it trap-ignore SIGTERM. The
// child is reaped in the background so a killed pid returns ESRCH (the test is
// the parent — in production Sinet is NOT the backend's parent, so no zombie).
func startChild(t *testing.T, ignoreTerm bool) int {
	t.Helper()
	script := "while true; do sleep 0.2; done"
	if ignoreTerm {
		script = "trap '' TERM; " + script
	}
	cmd := exec.Command("sh", "-c", script)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd.Process.Pid
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

// TestKillBackendForcesSIGKILL proves the MANDATORY SIGKILL when the backend
// ignores SIGTERM (R11), and that post-kill recovery is verified so the shared
// admitter's zombie-VRAM gate clears.
func TestKillBackendForcesSIGKILL(t *testing.T) {
	ctx := context.Background()
	pid := startChild(t, true) // ignores SIGTERM
	reader := &pidAwareReader{pid: pid}
	adm := local.NewVRAMAdmitter(local.VRAMDeps{
		Settings: stubSettings{guard: 512, battery: local.BatteryAlways}, Reader: reader, Power: &local.FakePowerReader{},
	})
	p := local.NewPreemptor(local.PreemptorDeps{Settings: stubSettings{guard: 512}, Admitter: adm, Reader: reader})

	if err := p.KillBackend(ctx, pid); err != nil {
		t.Fatalf("KillBackend: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for alive(pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if alive(pid) {
		t.Fatal("backend survived — the MANDATORY SIGKILL did not fire (R11)")
	}
	// Recovery verified ⇒ the admitter's zombie-VRAM gate is cleared and admits.
	if ok, reason, err := adm.AdmitLoad(ctx, "m"); err != nil || !ok {
		t.Fatalf("post-kill admit: ok=%v reason=%q err=%v, want admitted (recovery verified)", ok, reason, err)
	}
}

// TestKillBackendCleanSIGTERM proves the clean path: a backend that exits on
// SIGTERM is not SIGKILL-ed, and recovery is still verified.
func TestKillBackendCleanSIGTERM(t *testing.T) {
	ctx := context.Background()
	pid := startChild(t, false) // default sh dies on SIGTERM
	reader := &pidAwareReader{pid: pid}
	p := local.NewPreemptor(local.PreemptorDeps{Settings: stubSettings{}, Reader: reader})
	if err := p.KillBackend(ctx, pid); err != nil {
		t.Fatalf("KillBackend: %v", err)
	}
	if alive(pid) {
		t.Fatal("backend still alive after a clean SIGTERM stop")
	}
}

// TestForceKillWedged proves the escalation BEHIND eager-unload (R12): a
// llama-server backend still holding VRAM is found by name and force-killed.
func TestForceKillWedged(t *testing.T) {
	ctx := context.Background()
	pid := startChild(t, true) // wedged: ignores SIGTERM
	reader := &pidAwareReader{pid: pid}
	p := local.NewPreemptor(local.PreemptorDeps{Settings: stubSettings{}, Reader: reader})
	if err := p.ForceKillWedged(ctx); err != nil {
		t.Fatalf("ForceKillWedged: %v", err)
	}
	if alive(pid) {
		t.Fatal("wedged llama-server backend survived ForceKillWedged (R12)")
	}
}

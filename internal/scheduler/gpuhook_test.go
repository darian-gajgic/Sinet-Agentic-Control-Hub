package scheduler

import (
	"context"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
)

// gpuhook_test.go — the S10.9 GPU-admission policy hook (brief R13): a
// scheduler-declared, nil-able interface satisfied by an internal/local VRAM
// admitter (wired at the shell). Wired-but-dormant — class-(a) local execution
// has no v0 consumer — so this exercises the seam hermetically with a fixture
// run flagged local-seated.

type fakeGPUAdmitter struct {
	ok     bool
	reason string
	calls  int
}

func (f *fakeGPUAdmitter) AdmitLoad(context.Context, string) (bool, string, error) {
	f.calls++
	return f.ok, f.reason, nil
}

func (e *schedEnv) gpuScheduler(t *testing.T, disp Dispatcher, adm GPUAdmitter, lane string) *Scheduler {
	t.Helper()
	led := metering.NewLedger(e.db, nil, metering.NoMeteredExceptions(), e.reg)
	s, err := New(Config{
		DB: e.db, Runs: e.runs, Settings: e.reg, Dispatcher: disp,
		Pressure:    metering.NewPressureGauge(e.db, e.reg),
		Receipts:    metering.NewReceipts(e.db, led, metering.NoMeteredExceptions()),
		GPUAdmitter: adm, LocalLane: lane, Logger: discardLogger(),
	})
	if err != nil {
		t.Fatalf("New scheduler: %v", err)
	}
	return s
}

// TestGPUAdmissionHookGatesLocalLane proves a local-seated run is REFUSED
// dispatch when the ledger reports no headroom, and admitted once there is
// headroom (R13). This is the dormant seam — no v0 run is actually on the local
// lane (BINDING), so the fixture stands in for a future class-(a) consumer.
func TestGPUAdmissionHookGatesLocalLane(t *testing.T) {
	ctx := context.Background()
	e := newSchedEnv(t)
	e.newRun(t, "loc", "alice", "local")
	disp := &fakeDispatcher{runs: e.runs, cps: e.cps}
	adm := &fakeGPUAdmitter{ok: false, reason: "no VRAM headroom"}
	s := e.gpuScheduler(t, disp, adm, "local")

	if err := s.Enqueue(ctx, "loc", ClassInteractive); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// No headroom ⇒ refused (not claimed).
	if n, err := s.Tick(ctx); err != nil || n != 0 {
		t.Fatalf("no headroom: dispatched %d err=%v, want 0 (GPU admission refused)", n, err)
	}
	if adm.calls == 0 {
		t.Fatal("the GPU hook was never consulted for a local-lane candidate")
	}
	// Headroom ⇒ admitted.
	adm.ok = true
	if n, err := s.Tick(ctx); err != nil || n != 1 {
		t.Fatalf("headroom: dispatched %d err=%v, want 1", n, err)
	}
}

// TestGPUAdmissionHookIgnoresNonLocalLane proves the hook gates ONLY the local
// lane — every other lane dispatches unchanged even when the admitter would
// refuse (R13: nil-safe, lane-scoped; existing behavior preserved).
func TestGPUAdmissionHookIgnoresNonLocalLane(t *testing.T) {
	ctx := context.Background()
	e := newSchedEnv(t)
	e.newRun(t, "ant", "alice", "anthropic")
	disp := &fakeDispatcher{runs: e.runs, cps: e.cps}
	adm := &fakeGPUAdmitter{ok: false, reason: "would refuse if consulted"}
	s := e.gpuScheduler(t, disp, adm, "local")

	if err := s.Enqueue(ctx, "ant", ClassInteractive); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if n, err := s.Tick(ctx); err != nil || n != 1 {
		t.Fatalf("non-local lane: dispatched %d err=%v, want 1 (never GPU-gated)", n, err)
	}
	if adm.calls != 0 {
		t.Errorf("the GPU hook was consulted %d times for a non-local lane, want 0", adm.calls)
	}
}

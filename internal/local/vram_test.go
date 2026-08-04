package local_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
)

// vram_test.go — black-box tests of the S12.7 admission math + S12.8 battery
// policy (brief R9/R14/R18), via the injectable fake reader/power.

// stubSettings serves the four ⚙ keys the admitter/broker read.
type stubSettings struct {
	guard    int64
	battery  string
	logprobs bool
}

func (s stubSettings) Int(k string) (int64, error) {
	switch k {
	case "local.vram.guard_band_mb":
		return s.guard, nil
	case "local.unload.term_grace_s":
		return 1, nil
	}
	return 0, fmt.Errorf("stub: unexpected int key %q", k)
}
func (s stubSettings) Bool(k string) (bool, error) {
	if k == "local.broker.sandbox_logprobs" {
		return s.logprobs, nil
	}
	return false, fmt.Errorf("stub: unexpected bool key %q", k)
}
func (s stubSettings) String(k string) (string, error) {
	if k == "local.battery.gpu_admission" {
		return s.battery, nil
	}
	return "", fmt.Errorf("stub: unexpected string key %q", k)
}
func (s stubSettings) StringMap(string) (map[string]string, error) {
	return nil, fmt.Errorf("stub: no map")
}

func admitter(guard int64, battery string, onBat bool, readings ...local.GPUReading) *local.VRAMAdmitter {
	return local.NewVRAMAdmitter(local.VRAMDeps{
		Settings: stubSettings{guard: guard, battery: battery},
		Reader:   local.NewFakeGPUReader(readings...),
		Power:    &local.FakePowerReader{OnBat: onBat},
	})
}

// TestAdmissionMathLiveReading proves admission rests on the LIVE memory.free vs
// guard band, never a belief (R9): honest-uncalibrated ⇒ footprint 0 ⇒ admitted
// iff free ≥ guard.
func TestAdmissionMathLiveReading(t *testing.T) {
	ctx := context.Background()
	// free 8000 ≥ guard 512 ⇒ admit.
	ok, _, err := admitter(512, local.BatteryUrgentOnly, false, local.FakeActiveGPU(8000, 12227)).AdmitLoad(ctx, "Qwen3.5-9B")
	if err != nil || !ok {
		t.Fatalf("headroom present: ok=%v err=%v, want admitted", ok, err)
	}
	// free 256 < guard 512 ⇒ refuse (a game/compositor holds VRAM).
	ok, reason, err := admitter(512, local.BatteryUrgentOnly, false, local.FakeActiveGPU(256, 12227)).AdmitLoad(ctx, "Qwen3.5-9B")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if ok {
		t.Fatal("free below guard band must refuse (operator-wins holds live, S12.7)")
	}
	if reason == "" {
		t.Error("refusal must carry the live numbers")
	}
}

// TestAdmitSuspendedGPU proves a suspended GPU (asleep, 0 client bytes) is
// admitted — nothing holds VRAM; loading wakes it (S12.8 load-on-demand).
func TestAdmitSuspendedGPU(t *testing.T) {
	ok, _, err := admitter(512, local.BatteryAlways, false, local.FakeSuspendedGPU()).AdmitLoad(context.Background(), "Qwen3.5-9B")
	if err != nil || !ok {
		t.Fatalf("suspended GPU: ok=%v err=%v, want admitted (accept the wake)", ok, err)
	}
}

// TestBatteryModes proves ⚙ local.battery.gpu_admission gates by mode × urgency
// (R14): never refuses all; urgent-only admits only the watchdog-verdict class;
// always admits (subject to VRAM). On AC the battery gate never triggers.
func TestBatteryModes(t *testing.T) {
	ctx := context.Background()
	free := local.FakeActiveGPU(8000, 12227) // ample headroom, so only battery gates
	cases := []struct {
		mode     string
		urgentOK bool // AdmitDuty(watchdog, urgent=true)
		batchOK  bool // AdmitDuty(non-urgent)
	}{
		{local.BatteryNever, false, false},
		{local.BatteryUrgentOnly, true, false},
		{local.BatteryAlways, true, true},
	}
	for _, c := range cases {
		a := admitter(512, c.mode, true, free)
		gotU, _, err := a.AdmitDuty(ctx, "Qwen3.5-9B", true)
		if err != nil {
			t.Fatalf("%s urgent: %v", c.mode, err)
		}
		if gotU != c.urgentOK {
			t.Errorf("%s urgent admit = %v, want %v", c.mode, gotU, c.urgentOK)
		}
		gotB, _, err := admitter(512, c.mode, true, free).AdmitDuty(ctx, "Qwen3.5-9B", false)
		if err != nil {
			t.Fatalf("%s batch: %v", c.mode, err)
		}
		if gotB != c.batchOK {
			t.Errorf("%s batch admit = %v, want %v", c.mode, gotB, c.batchOK)
		}
	}
	// On AC, even mode=never admits (the battery gate does not trigger).
	ok, _, err := admitter(512, local.BatteryNever, false, free).AdmitDuty(ctx, "Qwen3.5-9B", false)
	if err != nil || !ok {
		t.Fatalf("on AC with mode=never: ok=%v err=%v, want admitted", ok, err)
	}
}

// TestNilAdmitterAdmitsEverything proves the Pressure-precedent nil-safety
// (R13): a nil admitter admits (the unconfigured dev default is unchanged).
func TestNilAdmitterAdmitsEverything(t *testing.T) {
	var a *local.VRAMAdmitter
	ok, _, err := a.AdmitLoad(context.Background(), "anything")
	if err != nil || !ok {
		t.Fatalf("nil admitter: ok=%v err=%v, want admit-everything", ok, err)
	}
}

// TestDutyPreflightRefusesLive proves OQ2: the live duty pre-flight refuses a
// duty call when VRAM is exhausted (a game holds it), surfaced as
// ErrGPUAdmissionRefused so the caller degrades — never a faked load. And it
// admits when there is headroom. Uses the real registry (default alias map).
func TestDutyPreflightRefusesLive(t *testing.T) {
	ctx := context.Background()
	e := newDutyEnv(t)
	e.runningRun(t, "task.preflight")
	e.fake.SetModelResponse("Qwen3.5-4B", local.FakeResponse{Content: `{"abstain":true}`, InputTokens: 5, OutputTokens: 1})

	schema := local.TriageSchema([]string{"software"}, []string{"low"}, []string{"small"})
	req := local.DutyRequest{Alias: local.AliasIntakeTriage, User: "x", Schema: schema, Name: "t", MaxTokens: 64, Classification: true}

	// No headroom ⇒ the pre-flight refuses BEFORE the load; the fake server is
	// never reached.
	exhausted := local.NewDuty(local.DutyDeps{
		Registry: local.NewRegistry(e.reg), Client: local.NewClient(e.fake.URL),
		Checkpoints: e.cps, Events: e.log,
		Admitter: local.NewVRAMAdmitter(local.VRAMDeps{
			Settings: e.reg, Reader: local.NewFakeGPUReader(local.FakeActiveGPU(128, 12227)), Power: &local.FakePowerReader{},
		}),
	})
	if _, err := exhausted.Call(ctx, "task.preflight", req); !errors.Is(err, local.ErrGPUAdmissionRefused) {
		t.Fatalf("exhausted VRAM: err=%v, want ErrGPUAdmissionRefused (degrade, never a faked load)", err)
	}

	// Headroom ⇒ the call proceeds.
	okAdm := local.NewDuty(local.DutyDeps{
		Registry: local.NewRegistry(e.reg), Client: local.NewClient(e.fake.URL),
		Checkpoints: e.cps, Events: e.log,
		Admitter: local.NewVRAMAdmitter(local.VRAMDeps{
			Settings: e.reg, Reader: local.NewFakeGPUReader(local.FakeActiveGPU(9000, 12227)), Power: &local.FakePowerReader{},
		}),
	})
	if _, err := okAdm.Call(ctx, "task.preflight", req); err != nil {
		t.Fatalf("headroom present: Call err=%v, want success", err)
	}
}

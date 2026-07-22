package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// vram_internal_test.go — white-box tests of the LIVE sleep-gated reader (brief
// R8/R15/R18): enumeration + the sleep gate + XML parsing over a temp sysfs
// tree + an injected nvidia-smi, so the "a suspended GPU is NEVER polled" rule
// is proven as BEHAVIOR (the injected smi fails the test if called).

// writeSysfsDevice builds a fake NVIDIA PCI device under root.
func writeSysfsDevice(t *testing.T, root, addr, vendor, class, runtimeStatus string) {
	t.Helper()
	dir := filepath.Join(root, "bus", "pci", "devices", addr)
	if err := os.MkdirAll(filepath.Join(dir, "power"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, val := range map[string]string{"vendor": vendor, "class": class} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(val+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "power", "runtime_status"), []byte(runtimeStatus+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSleepGateNeverPollsSuspended proves the sleep gate (R8 step 2 / R15): a
// suspended GPU is recorded "asleep, 0 client bytes" from sysfs ALONE — the
// nvidia-smi poll is NEVER issued (polling wakes it, defeating RTD3).
func TestSleepGateNeverPollsSuspended(t *testing.T) {
	root := t.TempDir()
	writeSysfsDevice(t, root, "0000:01:00.0", "0x10de", "0x030000", "suspended")

	polled := false
	r := &sysfsGPUReader{root: root, smi: func(context.Context, ...string) ([]byte, error) {
		polled = true
		return nil, nil
	}}
	readings, err := r.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if polled {
		t.Fatal("nvidia-smi was polled on a SUSPENDED GPU — the sleep gate failed (R8/R15: polling wakes it)")
	}
	if len(readings) != 1 || !readings[0].Suspended {
		t.Fatalf("suspended reading = %+v, want one Suspended=true", readings)
	}
	if readings[0].MemFreeMiB != 0 || len(readings[0].Processes) != 0 {
		t.Error("a suspended GPU must record 0 client bytes and no processes (never polled)")
	}
}

// TestActiveReadDeviceTruthAndGraphics proves an ACTIVE GPU is read for
// device-truth memory.free AND per-process attribution INCLUDING graphics
// clients (R8 steps 3–4): the fixture XML mirrors the live 2026-07-22 shape
// (a type=G gnome-shell process that --query-compute-apps would miss).
func TestActiveReadDeviceTruthAndGraphics(t *testing.T) {
	root := t.TempDir()
	writeSysfsDevice(t, root, "0000:01:00.0", "0x10de", "0x030000", "active")
	// The HDMI-audio function on the same card (class 0x0403) must be EXCLUDED.
	writeSysfsDevice(t, root, "0000:01:00.1", "0x10de", "0x040300", "suspended")

	const xml = `<nvidia_smi_log><gpu>
	 <uuid>GPU-test</uuid>
	 <fb_memory_usage><total>12227 MiB</total><used>494 MiB</used><free>11733 MiB</free></fb_memory_usage>
	 <bar1_memory_usage><total>16384 MiB</total><used>43 MiB</used><free>16341 MiB</free></bar1_memory_usage>
	 <processes>
	  <process_info><pid>6745</pid><type>G</type><process_name>/usr/bin/gnome-shell</process_name><used_memory>3 MiB</used_memory></process_info>
	  <process_info><pid>9001</pid><type>C</type><process_name>llama-server</process_name><used_memory>491 MiB</used_memory></process_info>
	 </processes></gpu></nvidia_smi_log>`

	var gotArgs []string
	r := &sysfsGPUReader{root: root, smi: func(_ context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(xml), nil
	}}
	readings, err := r.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// Only the display-class GPU is enumerated; the audio function is excluded.
	if len(readings) != 1 {
		t.Fatalf("readings = %d, want 1 (the audio function must be excluded)", len(readings))
	}
	g := readings[0]
	if g.MemFreeMiB != 11733 || g.MemTotalMiB != 12227 {
		t.Errorf("device-truth memory = free %d total %d, want 11733/12227 (first fb_memory_usage)", g.MemFreeMiB, g.MemTotalMiB)
	}
	if len(g.Processes) != 2 {
		t.Fatalf("processes = %d, want 2 (compute AND graphics)", len(g.Processes))
	}
	// The graphics client (gnome-shell, type G) must be attributed.
	var sawGraphics bool
	for _, p := range g.Processes {
		if p.Type == "G" && p.PID == 6745 {
			sawGraphics = true
		}
	}
	if !sawGraphics {
		t.Error("the graphics client (type G) was not attributed — compute-only queries miss it (S12.7 step 4)")
	}
	// The read targeted the specific device by -i (never a whole-tier query).
	if !hasArg(gotArgs, "-i") || !hasArg(gotArgs, "0000:01:00.0") {
		t.Errorf("nvidia-smi args = %v, want a device-targeted -i read", gotArgs)
	}
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// internalStub serves the ⚙ keys the admitter reads (white-box).
type internalStub struct{ guard int64 }

func (s internalStub) Int(string) (int64, error)                   { return s.guard, nil }
func (s internalStub) Bool(string) (bool, error)                   { return false, nil }
func (s internalStub) String(string) (string, error)               { return BatteryAlways, nil }
func (s internalStub) StringMap(string) (map[string]string, error) { return nil, nil }

// TestPostKillRecoveryGate proves the zombie-VRAM guard (R9): after a forced
// kill marks recovery pending, admission is REFUSED until memory.free returns to
// the recovery target — always the live reading.
func TestPostKillRecoveryGate(t *testing.T) {
	ctx := context.Background()
	reader := NewFakeGPUReader(FakeActiveGPU(2000, 12227)) // free below the target
	a := NewVRAMAdmitter(VRAMDeps{Settings: internalStub{guard: 512}, Reader: reader, Power: &FakePowerReader{}})

	a.markKilled(2000, 6000) // a kill should free ~6000; target = 2000 + 4500 = 6500
	if ok, _, _ := a.admit(ctx, "m", false); ok {
		t.Fatal("admission must be refused while post-kill recovery is unverified (zombie VRAM, R9)")
	}
	reader.Set(FakeActiveGPU(9000, 12227)) // VRAM recovered
	if ok, _, _ := a.admit(ctx, "m", false); !ok {
		t.Fatal("admission must proceed once memory.free has recovered (R9)")
	}
}

// A compile-time assertion that *VRAMAdmitter satisfies the scheduler's
// GPU-admission hook signature (R13) — the scheduler declares that interface;
// local satisfies it structurally without importing scheduler (§26 wall).
var _ interface {
	AdmitLoad(ctx context.Context, model string) (bool, string, error)
} = (*VRAMAdmitter)(nil)

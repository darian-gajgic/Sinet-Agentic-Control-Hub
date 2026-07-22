package local

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// vram.go — the Spec S12.7 VRAM ledger + admission math + battery policy (brief
// R8/R9/R14). The ledger is the mechanics BEHIND the S10.9 scheduler
// GPU-admission policy hook (R13) AND the live platform-plane pre-flight run
// before a duty call triggers a llama-swap load (OQ2). It is built as the
// R09-OQ8 protocol machinery; the per-model FOOTPRINT VALUES are B4-7's
// calibration run (S12.9 measurement #2, OQ1 machinery-only) — this ledger
// ships HONEST-UNCALIBRATED: an unmeasured seat carries no footprint belief and
// the live memory.free reading is ALWAYS authoritative regardless (R10).
//
// Read discipline (R8/R15; S12.7/S12.8): every GPU read SLEEP-GATES — it reads
// power/runtime_status from sysfs FIRST (a pure file read, no GPU wake) and a
// SUSPENDED GPU is recorded "asleep, 0 client bytes" and NEVER polled (polling
// wakes it, defeating RTD3). Device truth comes from device-level memory.free,
// never per-process sums. Per-process attribution includes GRAPHICS clients
// (nvidia-smi --query-compute-apps misses every graphics process — live-verified
// 2026-07-22: gnome-shell showed only under `nvidia-smi -q -x` process_info).
// There is NO periodic poll anywhere: the reads run only when a load/admission
// is contemplated or a kill's recovery is verified (event-driven, R15).

// GPUProcess is one process holding VRAM (compute OR graphics — S12.7 step 4).
type GPUProcess struct {
	PID     int64  `json:"pid"`
	Type    string `json:"type"` // "C" | "G" | "C+G" (nvidia-smi process type)
	Name    string `json:"name"`
	UsedMiB int64  `json:"used_mib"`
}

// GPUReading is one GPU's sleep-gated read (S12.7 steps 2–4). A suspended GPU
// carries Suspended=true, zero memory figures, and nil Processes — it was
// recorded from sysfs alone and NEVER polled (R8/R15).
type GPUReading struct {
	PCIAddr     string       `json:"pci_addr"` // sysfs address, e.g. "0000:01:00.0"
	UUID        string       `json:"uuid"`     // "" for a suspended GPU (never polled)
	Suspended   bool         `json:"suspended"`
	MemFreeMiB  int64        `json:"mem_free_mib"`
	MemTotalMiB int64        `json:"mem_total_mib"`
	Processes   []GPUProcess `json:"processes,omitempty"`
}

// LedgerSnapshot is a full sleep-gated read of the tier plus the host power
// state — the CLI status surface (R? cli) and the observability data. It never
// wakes a suspended GPU.
type LedgerSnapshot struct {
	GPUs      []GPUReading `json:"gpus"`
	OnBattery bool         `json:"on_battery"`
	GuardMiB  int64        `json:"guard_band_mib"`
	// BatteryMode is ⚙ local.battery.gpu_admission at read time.
	BatteryMode string `json:"battery_mode"`
}

// gpuReader reads the tier's GPUs, sleep-gating every device (R8). The live
// implementation is sysfsGPUReader; tests inject a fixture reader (R18).
type gpuReader interface {
	Read(ctx context.Context) ([]GPUReading, error)
}

// powerReader reports whether the host is on battery — a sleep-safe host
// observation (sysfs power-supply state, NEVER a periodic GPU wake — S12.8/R14).
type powerReader interface {
	OnBattery() (bool, error)
}

// PowerReader is the exported host AC/battery observation (the B4-7 battery
// harness consumes it for the ⚙ local.batch.ac_only gate, R7). FakePowerReader
// satisfies it in tests.
type PowerReader interface {
	OnBattery() (bool, error)
}

// NewPowerReader returns the live sysfs host power-supply reader (a sleep-safe
// host observation, never a periodic GPU wake — S12.8).
func NewPowerReader() PowerReader { return newSysfsPowerReader() }

// smiFunc runs nvidia-smi and returns its stdout — injected so a test can prove
// a suspended GPU is NEVER polled (the fixture makes this panic if called; R18).
type smiFunc func(ctx context.Context, args ...string) ([]byte, error)

// sysfsGPUReader is the live reader (R8). It enumerates NVIDIA GPUs from sysfs
// (sleep-safe), sleep-gates each on power/runtime_status (sleep-safe), and
// polls memory.free + per-process ONLY for active devices via nvidia-smi.
type sysfsGPUReader struct {
	root string  // sysfs root ("/sys"); overridable for tests
	smi  smiFunc // nvidia-smi runner; overridable for tests
}

func newSysfsGPUReader() *sysfsGPUReader {
	return &sysfsGPUReader{root: "/sys", smi: runNvidiaSMI}
}

func runNvidiaSMI(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "nvidia-smi", args...).Output()
}

// Read enumerates + sleep-gates + reads (R8 steps 1–4). The runtime_status
// read is a pure sysfs file read; a suspended GPU is recorded asleep and the
// nvidia-smi poll is SKIPPED (never wake it — R15).
func (r *sysfsGPUReader) Read(ctx context.Context) ([]GPUReading, error) {
	addrs, err := enumerateNVIDIAGPUs(r.root)
	if err != nil {
		return nil, err
	}
	var out []GPUReading
	for _, addr := range addrs {
		status := readRuntimeStatus(r.root, addr)
		if status == "suspended" {
			// Asleep: 0 client bytes, NEVER polled (S12.7 step 2, R15).
			out = append(out, GPUReading{PCIAddr: addr, Suspended: true})
			continue
		}
		// Active: device-truth memory.free + per-process (incl. graphics) in one
		// nvidia-smi call (-q -x carries fb_memory_usage AND process_info).
		reading, err := r.readActive(ctx, addr)
		if err != nil {
			return nil, err
		}
		out = append(out, reading)
	}
	return out, nil
}

func (r *sysfsGPUReader) readActive(ctx context.Context, addr string) (GPUReading, error) {
	raw, err := r.smi(ctx, "-q", "-x", "-i", addr)
	if err != nil {
		return GPUReading{}, fmt.Errorf("local: nvidia-smi read of active GPU %s: %w", addr, err)
	}
	return parseSMIXML(addr, raw)
}

// enumerateNVIDIAGPUs lists NVIDIA display-class PCI devices from sysfs (R8
// step 1). Vendor 0x10de + class prefix 0x0300 (VGA/3D controller) — the HDMI-
// audio function (class 0x0403) on the same device is EXCLUDED (live-verified
// 2026-07-22: 01:00.1 is the audio function, not a GPU). Pure file reads — no
// GPU wake.
func enumerateNVIDIAGPUs(root string) ([]string, error) {
	base := filepath.Join(root, "bus", "pci", "devices")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, fmt.Errorf("local: enumerate PCI devices: %w", err)
	}
	var addrs []string
	for _, e := range entries {
		dir := filepath.Join(base, e.Name())
		vendor := strings.TrimSpace(readSysfs(filepath.Join(dir, "vendor")))
		class := strings.TrimSpace(readSysfs(filepath.Join(dir, "class")))
		if vendor == "0x10de" && strings.HasPrefix(class, "0x0300") {
			addrs = append(addrs, e.Name())
		}
	}
	return addrs, nil
}

// readRuntimeStatus reads the device's RTD3 runtime_status from sysfs (the
// sleep gate; S12.7 step 2). A read of this file never wakes the GPU.
func readRuntimeStatus(root, addr string) string {
	return strings.TrimSpace(readSysfs(filepath.Join(root, "bus", "pci", "devices", addr, "power", "runtime_status")))
}

func readSysfs(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// smiXML is the minimal shape of `nvidia-smi -q -x`. The FIRST fb_memory_usage
// block is the frame buffer (the second is BAR1 — live-verified); encoding/xml
// binds the first occurrence to a single field.
type smiXML struct {
	GPUs []struct {
		UUID      string `xml:"uuid"`
		FBMemory  smiMem `xml:"fb_memory_usage"`
		Processes struct {
			Info []struct {
				PID  int64  `xml:"pid"`
				Type string `xml:"type"`
				Name string `xml:"process_name"`
				Used string `xml:"used_memory"`
			} `xml:"process_info"`
		} `xml:"processes"`
	} `xml:"gpu"`
}

type smiMem struct {
	Total string `xml:"total"`
	Free  string `xml:"free"`
}

func parseSMIXML(addr string, raw []byte) (GPUReading, error) {
	var doc smiXML
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return GPUReading{}, fmt.Errorf("local: parse nvidia-smi XML for %s: %w", addr, err)
	}
	if len(doc.GPUs) == 0 {
		return GPUReading{}, fmt.Errorf("local: nvidia-smi XML for %s carried no gpu", addr)
	}
	g := doc.GPUs[0]
	reading := GPUReading{
		PCIAddr:     addr,
		UUID:        strings.TrimSpace(g.UUID),
		MemFreeMiB:  parseMiB(g.FBMemory.Free),
		MemTotalMiB: parseMiB(g.FBMemory.Total),
	}
	for _, p := range g.Processes.Info {
		reading.Processes = append(reading.Processes, GPUProcess{
			PID: p.PID, Type: strings.TrimSpace(p.Type), Name: strings.TrimSpace(p.Name), UsedMiB: parseMiB(p.Used),
		})
	}
	return reading, nil
}

// parseMiB parses "11733 MiB" → 11733 (nvidia-smi's XML unit form).
func parseMiB(s string) int64 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "MiB")
	s = strings.TrimSpace(s)
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// sysfsPowerReader reads the host AC/battery state from sysfs power-supply
// (S12.8/R14) — a sleep-safe host observation, never a periodic GPU wake. The
// host is ON BATTERY when a type=Mains supply exists and reads online=0; the
// absence of any Mains supply (a desktop) reads as AC.
type sysfsPowerReader struct{ root string }

func newSysfsPowerReader() *sysfsPowerReader { return &sysfsPowerReader{root: "/sys"} }

func (p *sysfsPowerReader) OnBattery() (bool, error) {
	base := filepath.Join(p.root, "class", "power_supply")
	entries, err := os.ReadDir(base)
	if err != nil {
		return false, nil // no power-supply class (a server/VM) ⇒ treat as AC
	}
	sawMains := false
	for _, e := range entries {
		dir := filepath.Join(base, e.Name())
		if strings.TrimSpace(readSysfs(filepath.Join(dir, "type"))) != "Mains" {
			continue
		}
		sawMains = true
		if strings.TrimSpace(readSysfs(filepath.Join(dir, "online"))) == "1" {
			return false, nil // an online mains supply ⇒ on AC
		}
	}
	return sawMains, nil // a mains supply exists but none online ⇒ on battery
}

// FootprintKey is the FULL S12.7 step-5 footprint tuple (brief R18 / B4-6 F5
// carry): a VRAM footprint is a fact of (model, quant, context, slots, engine
// build), not of the model string alone — the same weights at a different
// context or slot count occupy different VRAM, and an engine bump re-measures
// (LedgerReRunTriggers). The calibration run (S12.9 #2, R21) fills the ledger
// per tuple; an unmeasured tuple stays honest-uncalibrated.
type FootprintKey struct {
	Model       string `json:"model"`
	Quant       string `json:"quant"`
	Context     int64  `json:"context"`
	Slots       int64  `json:"slots"`
	EngineBuild string `json:"engine_build"`
}

// Ledger holds the per-tuple VRAM footprint beliefs (S12.7 step 5, R18). At v0
// it is EMPTY — the calibration values are B4-7's measurement (S12.9 #2), so
// every tuple is UNCALIBRATED and admission rests entirely on the live
// memory.free reading (R10). The compositor-headroom figures (hybrid + MUX,
// S12.7 step 6) live here too, uncalibrated at v0 — machinery for B4-7 to fill,
// never guessed.
type Ledger struct {
	footprints map[FootprintKey]int64 // tuple → measured MiB (EMPTY at v0)
	// HybridHeadroomMiB / MUXHeadroomMiB are the S12.7 step-6 compositor-headroom
	// figures per display mode (hybrid is the recommended VRAM-maximizing mode).
	// Zero = uncalibrated (B4-7 measures them; never a documented constant, R8).
	HybridHeadroomMiB int64
	MUXHeadroomMiB    int64
}

// NewLedger returns the honest-uncalibrated v0 ledger (R10/R18).
func NewLedger() *Ledger { return &Ledger{footprints: map[FootprintKey]int64{}} }

// SetFootprint records a MEASURED per-tuple footprint — the calibration run
// (R21) fills the ledger through this; never a guessed constant (R8/R18).
func (l *Ledger) SetFootprint(key FootprintKey, mib int64) {
	if l == nil {
		return
	}
	l.footprints[key] = mib
}

// FootprintByKey returns a tuple's measured VRAM footprint and whether it is
// CALIBRATED (S12.7/R18). Uncalibrated ⇒ (0, false) and admission rests on the
// live reading (R10).
func (l *Ledger) FootprintByKey(key FootprintKey) (mib int64, calibrated bool) {
	if l == nil {
		return 0, false
	}
	v, ok := l.footprints[key]
	return v, ok
}

// Footprint returns a MODEL's measured footprint for the admission-belief note
// (S12.7): any calibrated tuple for the model, else (0, false). Admission rests
// on the LIVE reading regardless of this belief (R9/R10/R18) — the tuple key is
// the precise calibration surface (FootprintByKey), this is the coarse note.
func (l *Ledger) Footprint(model string) (mib int64, calibrated bool) {
	if l == nil {
		return 0, false
	}
	for k, v := range l.footprints {
		if k.Model == model {
			return v, true
		}
	}
	return 0, false
}

// LedgerReRunTriggers documents the S12.7 invalidation rule (R8): the ledger is
// re-measured on any of these changes, NOT on a timer (R15). Recorded as data
// so the CLI/observability + B4-7 read one authority.
func LedgerReRunTriggers() []string {
	return []string{
		"driver upgrade",
		"engine bump (llama.cpp b-tag / llama-swap release)",
		"model or quant change (a swap through the S12.10 gate)",
		"display-mode change (hybrid ↔ MUX)",
	}
}

// aliasUrgent reports whether a duty alias is URGENT-class for battery
// admission (S12.8 posture; OQ2 disposition kept minimal): the
// watchdog-verdict class is urgent (it must still run on battery, falling to
// the CPU tier when the dGPU is asleep — S12.8); every other duty is
// batch-class (not urgent) and parks until AC under ⚙ urgent-only.
func aliasUrgent(alias string) bool {
	return normalizeAlias(alias) == AliasWatchdog
}

// VRAMAdmitter is the S12.7 admission authority (R9/R13/R14). It satisfies the
// scheduler's nil-able GPU-admission POLICY hook (AdmitLoad; R13) AND backs the
// live platform-plane duty pre-flight (AdmitDuty; OQ2). Nil-safe: a nil
// admitter admits everything (the unconfigured dev default; the Pressure
// precedent). It never imports the scheduler (structural interface
// satisfaction — CONVENTIONS §26 wall).
type VRAMAdmitter struct {
	reader   gpuReader
	power    powerReader
	settings Settings
	ledger   *Ledger
	logger   *slog.Logger

	mu              sync.Mutex
	recoveryPending bool  // a forced kill happened; recovery not yet verified (R9)
	recoveryTarget  int64 // the freed GPU's post-kill memory.free must reach this
}

// VRAMDeps wires a live admitter (the shell builds it when the stack is
// configured). Reader/Power default to the live sysfs+nvidia-smi implementations.
type VRAMDeps struct {
	Settings Settings
	Ledger   *Ledger
	Logger   *slog.Logger
	// Reader / Power are injected in tests (R18); nil ⇒ the live implementations.
	Reader gpuReader
	Power  powerReader
}

// NewVRAMAdmitter wires the admitter (R9).
func NewVRAMAdmitter(d VRAMDeps) *VRAMAdmitter {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	ledger := d.Ledger
	if ledger == nil {
		ledger = NewLedger()
	}
	reader := d.Reader
	if reader == nil {
		reader = newSysfsGPUReader()
	}
	power := d.Power
	if power == nil {
		power = newSysfsPowerReader()
	}
	return &VRAMAdmitter{reader: reader, power: power, settings: d.Settings, ledger: ledger, logger: logger}
}

// AdmitLoad is the scheduler's GPU-admission policy hook (R13): may a run be
// dispatched onto the local-inference lane? It applies the batch-class urgency
// (a run dispatch is not a watchdog verdict). Wired-but-dormant at v0 — class-(a)
// local execution has no consumer, so no run dispatches onto the local lane
// (P3/STATE binding). Nil-safe.
func (a *VRAMAdmitter) AdmitLoad(ctx context.Context, model string) (ok bool, reason string, err error) {
	return a.admit(ctx, model, false)
}

// AdmitDuty is the live platform-plane pre-flight (OQ2): run BEFORE a duty call
// triggers a llama-swap load, so operator-wins holds live (a game/compositor
// holding VRAM refuses the load). urgent = aliasUrgent(the duty). Nil-safe: a
// nil admitter (unconfigured stack) admits everything so the dev default is
// unchanged.
func (a *VRAMAdmitter) AdmitDuty(ctx context.Context, model string, urgent bool) (ok bool, reason string, err error) {
	return a.admit(ctx, model, urgent)
}

func (a *VRAMAdmitter) admit(ctx context.Context, model string, urgent bool) (bool, string, error) {
	if a == nil {
		return true, "", nil // no admitter ⇒ no gate (the Pressure precedent, R13)
	}

	// Post-kill recovery MUST be verified before the next admission (zombie VRAM
	// — R9). If a forced kill left recovery unverified, re-read live and clear if
	// memory.free has recovered; otherwise refuse.
	if a.recoveryUnverified() {
		if !a.reverify(ctx) {
			return false, "post-kill VRAM recovery not yet verified — zombie VRAM guard (S12.7/R9)", nil
		}
	}

	// Battery policy (R14): a sleep-safe host observation, never a GPU wake.
	onBat, err := a.power.OnBattery()
	if err != nil {
		return false, "", fmt.Errorf("local: read host power state: %w", err)
	}
	if onBat {
		mode, err := a.settings.String(keyBatteryGPUAdmission)
		if err != nil {
			return false, "", fmt.Errorf("local: read ⚙ %s: %w", keyBatteryGPUAdmission, err)
		}
		switch mode {
		case BatteryNever:
			return false, "on battery; ⚙ " + keyBatteryGPUAdmission + "=never refuses GPU loads on battery (S12.8)", nil
		case BatteryUrgentOnly:
			if !urgent {
				return false, "on battery; ⚙ " + keyBatteryGPUAdmission + "=urgent-only and this duty is batch-class (S12.8)", nil
			}
		case BatteryAlways:
			// admit — fall through to the VRAM check
		default:
			return false, "", fmt.Errorf("local: unexpected ⚙ %s value %q", keyBatteryGPUAdmission, mode)
		}
	}

	guard, err := a.settings.Int(keyGuardBandMB)
	if err != nil {
		return false, "", fmt.Errorf("local: read ⚙ %s: %w", keyGuardBandMB, err)
	}

	readings, err := a.reader.Read(ctx)
	if err != nil {
		return false, "", fmt.Errorf("local: sleep-gated VRAM read: %w", err)
	}
	target := poolGPU(readings)
	if target == nil {
		return false, "no NVIDIA GPU present for the local pool (S12.2 pool12)", nil
	}
	if target.Suspended {
		// Asleep ⇒ no resident clients (0 client bytes); loading wakes it, which
		// the battery decision above already sanctioned (S12.8: larger duties
		// load-on-demand accepting a wake). Nothing holds VRAM, so admit.
		return true, "", nil
	}

	foot, calibrated := a.ledger.Footprint(model)
	need := foot + guard
	if target.MemFreeMiB >= need {
		return true, "", nil
	}
	beliefNote := "uncalibrated footprint belief (OQ1: values are B4-7's, S12.9 #2)"
	if calibrated {
		beliefNote = fmt.Sprintf("calibrated footprint %d MiB", foot)
	}
	// Always the LIVE reading, never the ledger's belief (R9).
	return false, fmt.Sprintf("live memory.free %d MiB < %d MiB needed (guard %d + %s); operator/compositor holds VRAM (S12.7)",
		target.MemFreeMiB, need, guard, beliefNote), nil
}

// Snapshot is a full sleep-gated read + the host power state — the CLI status
// surface and observability data (R? cli). It never wakes a suspended GPU.
func (a *VRAMAdmitter) Snapshot(ctx context.Context) (LedgerSnapshot, error) {
	if a == nil {
		return LedgerSnapshot{}, ErrStackAbsent
	}
	readings, err := a.reader.Read(ctx)
	if err != nil {
		return LedgerSnapshot{}, err
	}
	onBat, _ := a.power.OnBattery()
	guard, _ := a.settings.Int(keyGuardBandMB)
	mode, _ := a.settings.String(keyBatteryGPUAdmission)
	return LedgerSnapshot{GPUs: readings, OnBattery: onBat, GuardMiB: guard, BatteryMode: mode}, nil
}

// markKilled records that a forced kill freed a backend holding roughly
// freedMiB — the next admission must verify memory.free recovered to at least
// the pre-kill free + a fraction of that (zombie VRAM guard, R9/R11). Called by
// the Preemptor before it verifies recovery.
func (a *VRAMAdmitter) markKilled(preKillFreeMiB, freedMiB int64) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.recoveryPending = true
	// Recovered = free returns to at least the pre-kill free plus most of what
	// the killed process should release (allow slack for fragmentation).
	a.recoveryTarget = preKillFreeMiB + freedMiB*3/4
}

// clearRecovery marks post-kill recovery verified (R9) — the Preemptor calls it
// once memory.free has returned.
func (a *VRAMAdmitter) clearRecovery() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.recoveryPending = false
	a.recoveryTarget = 0
}

func (a *VRAMAdmitter) recoveryUnverified() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.recoveryPending
}

// reverify re-reads live memory.free and clears the recovery flag if the target
// GPU has recovered to the post-kill target (R9). Returns true when cleared.
func (a *VRAMAdmitter) reverify(ctx context.Context) bool {
	readings, err := a.reader.Read(ctx)
	if err != nil {
		return false
	}
	target := poolGPU(readings)
	if target == nil {
		return false
	}
	a.mu.Lock()
	want := a.recoveryTarget
	a.mu.Unlock()
	if target.Suspended || target.MemFreeMiB >= want {
		a.clearRecovery()
		return true
	}
	return false
}

// poolGPU returns the pool GPU from a set of readings — the local tier serves a
// single internal dGPU at v0 (pool12; pool24/eGPU is dormant, S12.11), so the
// first NVIDIA GPU is the pool. Returns nil when none is present.
func poolGPU(readings []GPUReading) *GPUReading {
	for i := range readings {
		return &readings[i]
	}
	return nil
}

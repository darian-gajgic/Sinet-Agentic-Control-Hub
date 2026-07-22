package local

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"syscall"
	"time"
)

// preempt.go — the Spec S12.7 kill-not-freeze preemption discipline (brief
// R11/R12). Freezing is categorically WRONG for VRAM: a stopped process keeps
// its CUDA context and allocations; the driver frees memory only at teardown.
// Freeze/thaw is the CPU/RAM tool (S10) — unload/KILL is the GPU tool. This is
// the Sinet-side forced-kill discipline that sits BEHIND the B4-5 eager-unload
// verb for the wedge case where llama-swap's own POST /api/models/unload wedges
// (llama-server can ignore SIGTERM): SIGTERM → ⚙ local.unload.term_grace_s →
// SIGKILL — the kill path is MANDATORY, not a fallback — then memory.free
// recovery MUST be verified before the next admission (zombie VRAM exists, R9).
//
// Granularity (S12.7/R12): generation cancels between decode steps (~ms); a
// running prefill batch cannot be interrupted — worst-case voluntary preemption
// ≈ one prefill batch. When the operator needs VRAM now, the process is KILLED.

// recoveryPollInterval is the memory.free re-read cadence while verifying
// post-kill recovery. A structural constant (the sseBatchSize precedent — S18
// ratifies no such key); event-driven, never a periodic GPU wake (R15).
const recoveryPollInterval = 200 * time.Millisecond

// recoveryTimeout bounds post-kill recovery verification. Structural constant.
const recoveryTimeout = 20 * time.Second

// sigkillGrace bounds the wait for a process to disappear after the mandatory
// SIGKILL. Structural constant.
const sigkillGrace = 5 * time.Second

// Preemptor performs the S12.7 forced-kill-then-verify discipline (R11/R12). It
// reads per-process attribution + memory.free through the same sleep-gated
// reader as the ledger, and (when wired) marks/clears the admitter's post-kill
// recovery gate so the next admission is refused until recovery is verified.
type Preemptor struct {
	reader   gpuReader
	admitter *VRAMAdmitter // optional: the post-kill recovery gate (R9)
	settings Settings
	logger   *slog.Logger
	// signal sends a signal to a pid — injected in tests; default syscall.Kill.
	signal func(pid int, sig syscall.Signal) error
	now    func() time.Time
	poll   time.Duration
}

// PreemptorDeps wires a live Preemptor.
type PreemptorDeps struct {
	Settings Settings
	Admitter *VRAMAdmitter
	Logger   *slog.Logger
	// Reader is injected in tests; nil ⇒ the live sysfs+nvidia-smi reader.
	Reader gpuReader
	// Signal is injected in tests; nil ⇒ syscall.Kill.
	Signal func(pid int, sig syscall.Signal) error
}

// NewPreemptor wires the forced-kill discipline (R11).
func NewPreemptor(d PreemptorDeps) *Preemptor {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	reader := d.Reader
	if reader == nil {
		reader = newSysfsGPUReader()
	}
	sig := d.Signal
	if sig == nil {
		sig = func(pid int, s syscall.Signal) error { return syscall.Kill(pid, s) }
	}
	return &Preemptor{
		reader: reader, admitter: d.Admitter, settings: d.Settings,
		logger: logger, signal: sig, now: time.Now, poll: recoveryPollInterval,
	}
}

// KillBackend stops a backend process with the MANDATORY SIGTERM→grace→SIGKILL
// discipline, then verifies memory.free recovery (R11/R9). The grace comes from
// ⚙ local.unload.term_grace_s. Returns an error if the process survives SIGKILL
// or recovery is not verified within the timeout (the admitter then stays
// gated — zombie VRAM guard).
func (p *Preemptor) KillBackend(ctx context.Context, pid int) error {
	graceS, err := p.settings.Int(keyUnloadGrace)
	if err != nil {
		return fmt.Errorf("local: read ⚙ %s: %w", keyUnloadGrace, err)
	}
	grace := time.Duration(graceS) * time.Second

	preFree, held := p.readPidHold(ctx, pid)

	// SIGTERM, then wait out the grace (S12.7).
	if err := p.signal(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("local: SIGTERM backend pid %d: %w", pid, err)
	}
	if !p.waitExit(ctx, pid, grace) {
		// MANDATORY SIGKILL — llama-server can wedge and ignore SIGTERM (R11).
		p.logger.WarnContext(ctx, "local: backend ignored SIGTERM within grace — forcing SIGKILL (kill-not-freeze, S12.7)",
			"pid", pid, "grace_s", graceS)
		if err := p.signal(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("local: SIGKILL backend pid %d: %w", pid, err)
		}
		if !p.waitExit(ctx, pid, sigkillGrace) {
			return fmt.Errorf("local: backend pid %d survived SIGKILL (zombie process)", pid)
		}
	}

	// Recovery MUST be verified before the next admission (R9). Mark the admitter
	// gated up front so a verification timeout leaves it refusing (zombie VRAM).
	if p.admitter != nil {
		p.admitter.markKilled(preFree, held)
	}
	if err := p.verifyRecovery(ctx, preFree+held*3/4); err != nil {
		return err
	}
	if p.admitter != nil {
		p.admitter.clearRecovery()
	}
	return nil
}

// ForceKillWedged is the escalation BEHIND the eager-unload verb (R12): after
// llama-swap's own unload, if VRAM did not recover, any llama-server backend
// still holding VRAM is force-killed with the full discipline. Identifies
// backends by process name (llama-swap names its child backends "llama-server").
// Idempotent: with no wedged backend it is a no-op.
func (p *Preemptor) ForceKillWedged(ctx context.Context) error {
	readings, err := p.reader.Read(ctx)
	if err != nil {
		return fmt.Errorf("local: read for wedged-backend scan: %w", err)
	}
	target := poolGPU(readings)
	if target == nil || target.Suspended {
		return nil // asleep/absent ⇒ nothing resident to kill
	}
	var killed []int
	for _, proc := range target.Processes {
		if !strings.Contains(proc.Name, "llama-server") {
			continue
		}
		p.logger.WarnContext(ctx, "local: llama-server backend still holds VRAM after unload — forcing kill (S12.7 wedge case)",
			"pid", proc.PID, "held_mib", proc.UsedMiB)
		if err := p.KillBackend(ctx, int(proc.PID)); err != nil {
			return err
		}
		killed = append(killed, int(proc.PID))
	}
	if len(killed) > 0 {
		p.logger.InfoContext(ctx, "local: forced-kill escalation complete, VRAM recovery verified", "pids", killed)
	}
	return nil
}

// readPidHold reads the pool GPU's memory.free before a kill and how much the
// target pid holds — the recovery target is the pre-kill free plus most of the
// held bytes (R9). Zero/zero when the GPU is asleep or the pid is unattributed.
func (p *Preemptor) readPidHold(ctx context.Context, pid int) (preFree, held int64) {
	readings, err := p.reader.Read(ctx)
	if err != nil {
		return 0, 0
	}
	g := poolGPU(readings)
	if g == nil || g.Suspended {
		return 0, 0
	}
	for _, proc := range g.Processes {
		if proc.PID == int64(pid) {
			held = proc.UsedMiB
		}
	}
	return g.MemFreeMiB, held
}

// waitExit polls for a process to disappear (signal 0 → ESRCH) within d. True
// when the process is gone. This is a liveness probe on the PID, not a GPU read.
func (p *Preemptor) waitExit(ctx context.Context, pid int, d time.Duration) bool {
	deadline := p.now().Add(d)
	for {
		if err := p.signal(pid, 0); errors.Is(err, syscall.ESRCH) {
			return true // gone
		}
		if !p.now().Before(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// verifyRecovery re-reads memory.free until it returns to want (or the target
// GPU is asleep) within recoveryTimeout (R9). Event-driven re-reads, not a
// periodic GPU wake (R15).
func (p *Preemptor) verifyRecovery(ctx context.Context, want int64) error {
	deadline := p.now().Add(recoveryTimeout)
	for {
		readings, err := p.reader.Read(ctx)
		if err != nil {
			return fmt.Errorf("local: post-kill recovery read: %w", err)
		}
		g := poolGPU(readings)
		if g != nil && (g.Suspended || g.MemFreeMiB >= want) {
			return nil // recovered
		}
		if !p.now().Before(deadline) {
			return fmt.Errorf("local: post-kill VRAM recovery not verified within %s (zombie VRAM, S12.7/R9)", recoveryTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(p.poll):
		}
	}
}

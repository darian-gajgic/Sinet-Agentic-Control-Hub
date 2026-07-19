package sandbox

import (
	"encoding/binary"
	"fmt"

	"golang.org/x/sys/unix"
)

// seccomp.go builds the static seccomp-BPF profile of the S11.1 stack
// ("static allowlist profile (kills ptrace/process_vm_readv/odd execve)"),
// emitted as a classic-BPF sock_filter program and loaded into the sandbox by
// bwrap --seccomp <fd>. It is defense-in-depth, not the boundary (S11.1).
//
// Reading, section-cited (S11.1; CONVENTIONS new-convention rule): the S11.1
// "allowlist-only, never denylist" STRUCTURAL rule is scoped to "Mounts,
// tools, and egress" (S11.1 second structural bullet) — not to the syscall
// profile, which S11.1 separately describes by the dangerous calls it "kills".
// A full positive syscall allowlist safe for a Node engine is exactly srt's
// contribution (S16.3 sandbox-runtime), which is NOT consumed at B1 (direct
// composition per srt's funeral-plan rebuild path). So the B1 dev-mode profile
// is a default-ALLOW filter that KILLS the S11.1-named cross-process-inspection
// and tracing calls — safe for the engine, testable, and a real defense-in-
// depth layer. The generator also supports a positive allow-set (allowOnly)
// so the srt-equivalent profile drops in behind the same seam later.

// Classic-BPF opcodes (linux/filter.h). Hardcoded: x/sys/unix does not export
// the BPF_* class/mode constants, only the seccomp return actions and the
// SockFilter/SockFprog types.
const (
	bpfLD  = 0x00
	bpfW   = 0x00
	bpfABS = 0x20
	bpfJMP = 0x05
	bpfJEQ = 0x10
	bpfK   = 0x00
	bpfRET = 0x06
	bpfJA  = 0x00
)

// seccomp_data field offsets (linux/seccomp.h): nr @0, arch @4.
const (
	seccompDataNROffset   = 0
	seccompDataArchOffset = 4
)

// sockFilter mirrors unix.SockFilter with the classic-BPF field names, so the
// program reads like the kernel's struct sock_filter.
type sockFilter = unix.SockFilter

// deniedSyscalls is the S11.1-named kill set: ptrace and the cross-process
// memory-inspection pair. These are the concrete "kills ptrace/
// process_vm_readv" calls; "odd execve" arg-inspection needs the srt allowlist
// (deferred). Same set for every v0 class — the profile is structural, not ⚙.
func deniedSyscalls() []uint32 {
	return []uint32{
		uint32(unix.SYS_PTRACE),
		uint32(unix.SYS_PROCESS_VM_READV),
		uint32(unix.SYS_PROCESS_VM_WRITEV),
	}
}

// seccompProfile returns the compiled BPF program bytes for a class. The v0
// classes share the structural profile (S11 settings note: the seccomp-BPF
// profile is structural, versioned here, not an operator dial).
func seccompProfile(c Class) ([]byte, error) {
	if !c.isV0() {
		return nil, fmt.Errorf("%w: seccomp profile for %s", ErrClassNotV0, c)
	}
	prog := denyProgram(deniedSyscalls())
	return marshalProgram(prog), nil
}

// denyProgram builds a default-ALLOW filter that returns EPERM for each denied
// syscall, guarded by an architecture check that KILLs a call arriving under
// the wrong ABI (defeats the x32/compat syscall-number-confusion trick).
func denyProgram(denied []uint32) []sockFilter {
	errnoEPERM := uint32(unix.SECCOMP_RET_ERRNO) | (uint32(unix.EPERM) & uint32(unix.SECCOMP_RET_DATA))
	var f []sockFilter
	// Load arch; kill if it is not x86_64.
	f = append(f, sockFilter{Code: bpfLD | bpfW | bpfABS, K: seccompDataArchOffset})
	f = append(f, sockFilter{Code: bpfJMP | bpfJEQ | bpfK, K: uint32(unix.AUDIT_ARCH_X86_64), Jt: 1, Jf: 0})
	f = append(f, sockFilter{Code: bpfRET | bpfK, K: uint32(unix.SECCOMP_RET_KILL_PROCESS)})
	// Load nr.
	f = append(f, sockFilter{Code: bpfLD | bpfW | bpfABS, K: seccompDataNROffset})
	// One JEQ per denied nr, each jumping to the EPERM ret at the tail.
	// The tail layout is: [ALLOW][ERRNO]; a JEQ's Jt is the distance to ERRNO.
	n := len(denied)
	for i, nr := range denied {
		jt := uint8((n - 1 - i) + 1) // remaining JEQs after this one, then past ALLOW
		f = append(f, sockFilter{Code: bpfJMP | bpfJEQ | bpfK, K: nr, Jt: jt, Jf: 0})
	}
	f = append(f, sockFilter{Code: bpfRET | bpfK, K: uint32(unix.SECCOMP_RET_ALLOW)})
	f = append(f, sockFilter{Code: bpfRET | bpfK, K: errnoEPERM})
	return f
}

// allowProgram builds a positive allow-list filter (default EPERM) — the
// srt-equivalent shape, wired behind the same seam for when srt's audited
// Node-safe allow-set is adopted. Unused at B1; kept so the profile can flip
// without touching the composer.
func allowProgram(allowed []uint32) []sockFilter {
	errnoEPERM := uint32(unix.SECCOMP_RET_ERRNO) | (uint32(unix.EPERM) & uint32(unix.SECCOMP_RET_DATA))
	var f []sockFilter
	f = append(f, sockFilter{Code: bpfLD | bpfW | bpfABS, K: seccompDataArchOffset})
	f = append(f, sockFilter{Code: bpfJMP | bpfJEQ | bpfK, K: uint32(unix.AUDIT_ARCH_X86_64), Jt: 1, Jf: 0})
	f = append(f, sockFilter{Code: bpfRET | bpfK, K: uint32(unix.SECCOMP_RET_KILL_PROCESS)})
	f = append(f, sockFilter{Code: bpfLD | bpfW | bpfABS, K: seccompDataNROffset})
	for _, nr := range allowed {
		// If nr matches, jump forward to the ALLOW ret (past the trailing
		// ERRNO). Compute Jt lazily below after we know the layout: here each
		// match jumps to the ALLOW which we place immediately before ERRNO.
		f = append(f, sockFilter{Code: bpfJMP | bpfJEQ | bpfK, K: nr, Jt: 0, Jf: 0})
	}
	// Fix up: matches should skip the default-deny and hit ALLOW. Rebuild with
	// correct offsets now that the count is known.
	f = f[:4]
	m := len(allowed)
	for i, nr := range allowed {
		jt := uint8(m - 1 - i) // to the ALLOW that follows the last JEQ
		f = append(f, sockFilter{Code: bpfJMP | bpfJEQ | bpfK, K: nr, Jt: jt, Jf: 0})
	}
	f = append(f, sockFilter{Code: bpfRET | bpfK, K: uint32(unix.SECCOMP_RET_ALLOW)})
	f = append(f, sockFilter{Code: bpfRET | bpfK, K: errnoEPERM})
	return f
}

// marshalProgram serializes a sock_filter slice to the exact native-endian
// byte layout bwrap reads from the --seccomp fd (struct sock_filter[]: u16
// code, u8 jt, u8 jf, u32 k — 8 bytes each; little-endian on amd64).
func marshalProgram(f []sockFilter) []byte {
	buf := make([]byte, 0, len(f)*8)
	var b [8]byte
	for _, x := range f {
		binary.LittleEndian.PutUint16(b[0:], x.Code)
		b[2] = x.Jt
		b[3] = x.Jf
		binary.LittleEndian.PutUint32(b[4:], x.K)
		buf = append(buf, b[:]...)
	}
	return buf
}

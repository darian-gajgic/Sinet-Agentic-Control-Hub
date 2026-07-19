package sandbox

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestSeccompProfileStructure(t *testing.T) {
	bpf, err := seccompProfile(C1)
	if err != nil {
		t.Fatal(err)
	}
	if len(bpf) == 0 || len(bpf)%8 != 0 {
		t.Fatalf("profile is %d bytes, want a nonzero multiple of 8 (sock_filter is 8 bytes)", len(bpf))
	}
	// C0/C1/C2 share the structural profile.
	for _, c := range []Class{C0, C2} {
		b, err := seccompProfile(c)
		if err != nil {
			t.Fatalf("profile %s: %v", c, err)
		}
		if len(b) != len(bpf) {
			t.Errorf("profile %s differs in size from C1 (should be structural/shared)", c)
		}
	}
	if _, err := seccompProfile(C3); err == nil {
		t.Error("seccomp profile composed for non-v0 class C3")
	}
}

func TestDenyProgramGuardsArchAndKillsNamedSyscalls(t *testing.T) {
	denied := deniedSyscalls()
	prog := denyProgram(denied)

	// First instruction loads the arch; second compares it to x86_64; third
	// KILLs on mismatch (defeats the compat/x32 syscall-number-confusion trick).
	if prog[0].K != seccompDataArchOffset {
		t.Error("program does not load seccomp_data.arch first")
	}
	if prog[1].K != uint32(unix.AUDIT_ARCH_X86_64) {
		t.Error("program does not compare against AUDIT_ARCH_X86_64")
	}
	if prog[2].K != uint32(unix.SECCOMP_RET_KILL_PROCESS) {
		t.Error("program does not KILL on an architecture mismatch")
	}

	// Every named-dangerous syscall must appear as a JEQ compare (S11.1 kills).
	present := map[uint32]bool{}
	for _, f := range prog {
		present[f.K] = true
	}
	for _, nr := range denied {
		if !present[nr] {
			t.Errorf("denied syscall %d is not compared in the program", nr)
		}
	}
	if !present[uint32(unix.SYS_PTRACE)] {
		t.Error("ptrace is not in the kill set (S11.1)")
	}

	// The tail must default-ALLOW then EPERM (default-allow filter).
	last := prog[len(prog)-1]
	allow := prog[len(prog)-2]
	if allow.K != uint32(unix.SECCOMP_RET_ALLOW) {
		t.Error("second-to-last instruction is not SECCOMP_RET_ALLOW (default allow)")
	}
	wantErrno := uint32(unix.SECCOMP_RET_ERRNO) | (uint32(unix.EPERM) & uint32(unix.SECCOMP_RET_DATA))
	if last.K != wantErrno {
		t.Errorf("last instruction K = %#x, want ERRNO|EPERM %#x", last.K, wantErrno)
	}
}

func TestAllowProgramIsDefaultDeny(t *testing.T) {
	// The srt-equivalent positive allow-list shape (behind the same seam):
	// default DENY (EPERM), explicit allows. Kept correct so the profile can
	// flip to a Node-safe allow-set without touching the composer.
	allowed := []uint32{uint32(unix.SYS_READ), uint32(unix.SYS_WRITE), uint32(unix.SYS_EXIT_GROUP)}
	prog := allowProgram(allowed)
	if prog[2].K != uint32(unix.SECCOMP_RET_KILL_PROCESS) {
		t.Error("allow program lacks the arch-mismatch KILL guard")
	}
	// The tail is ALLOW (for matches) then EPERM (default).
	if prog[len(prog)-2].K != uint32(unix.SECCOMP_RET_ALLOW) {
		t.Error("allow program tail is not ALLOW for matched syscalls")
	}
	wantErrno := uint32(unix.SECCOMP_RET_ERRNO) | (uint32(unix.EPERM) & uint32(unix.SECCOMP_RET_DATA))
	if prog[len(prog)-1].K != wantErrno {
		t.Error("allow program default is not EPERM (default-deny)")
	}
	present := map[uint32]bool{}
	for _, f := range prog {
		present[f.K] = true
	}
	for _, nr := range allowed {
		if !present[nr] {
			t.Errorf("allowed syscall %d not compared in the program", nr)
		}
	}
}

func TestMarshalProgramLittleEndian(t *testing.T) {
	prog := []sockFilter{{Code: 0x1234, Jt: 0x56, Jf: 0x78, K: 0x9abcdef0}}
	b := marshalProgram(prog)
	if len(b) != 8 {
		t.Fatalf("one filter marshaled to %d bytes, want 8", len(b))
	}
	// u16 code LE, u8 jt, u8 jf, u32 k LE.
	want := []byte{0x34, 0x12, 0x56, 0x78, 0xf0, 0xde, 0xbc, 0x9a}
	for i := range want {
		if b[i] != want[i] {
			t.Errorf("byte %d = %#x, want %#x", i, b[i], want[i])
		}
	}
}

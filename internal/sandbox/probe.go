package sandbox

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Composition/probe errors. Named so callers (and the S16.3 OS-mechanism
// discipline) can branch and fail closed with a legible reason.
var (
	// ErrUnknownClass rejects a class outside C0–C4.
	ErrUnknownClass = errors.New("sandbox: unknown confinement class")
	// ErrClassNotV0 rejects composing C3/C4 (post-v0, S11.6; 15.1).
	ErrClassNotV0 = errors.New("sandbox: class not shipped at v0")
	// ErrNoBwrap: the system bubblewrap binary is absent — the boundary
	// cannot be composed (fail closed, S16.3).
	ErrNoBwrap = errors.New("sandbox: system bubblewrap not present")
	// ErrUserns: unprivileged user namespaces do not work on this host — the
	// per-run sandbox is fundamentally unavailable (fail closed).
	ErrUserns = errors.New("sandbox: unprivileged user namespaces unavailable")
	// ErrNotComposable is returned by Capabilities.require when a mandatory
	// mechanism is missing and no skip was sanctioned.
	ErrNotComposable = errors.New("sandbox: host cannot compose the boundary")
)

// Capabilities is the probe-at-compose result (Spec S16.3 OS-mechanism
// discipline: "detect tool presence/kernel support, fail closed with named
// errors"). Bwrap+Userns are the boundary and are MANDATORY; Seccomp and
// Landlock are defense-in-depth and may be absent (sanctioned skip).
type Capabilities struct {
	// Bwrap is set when the system bubblewrap binary exists and reports a
	// version; BwrapVersion carries it for the lock entry / evidence.
	Bwrap        bool
	BwrapVersion string
	// Userns is set when an unprivileged user+net namespace actually forms
	// (the functional probe, not just a sysctl read).
	Userns bool
	// Seccomp is set when the kernel supports SECCOMP_MODE_FILTER (near
	// universal; bwrap --seccomp needs it).
	Seccomp bool
	// Landlock carries the detected ABI level (0 = unsupported). Enforcement
	// is a seam at B1 (landlock.go); the ABI is recorded for evidence and for
	// the enforcement path when it lands.
	LandlockABI int

	// Srt is set when the adopted `sandbox-runtime` CLI (S11.1 Adoption; S16.3
	// library row) is present — SrtPath is its resolved binary, SrtVersion the
	// best-effort `srt --version`. When set, the confiner composes VIA srt (the
	// ratified primary); when clear, it falls back to the native direct
	// composition (the S16.3 funeral plan). srt is a host-deferred install
	// (batches at the B2 gate), so at B1 this is normally clear and the native
	// fallback runs — no regression.
	Srt        bool
	SrtPath    string
	SrtVersion string

	// Notes accumulates human-readable probe outcomes for the packet
	// evidence and the lock entry's probe record.
	Notes []string
}

// Probe measures the host's sandbox mechanisms without root, reboot, or a
// real suspend — the S11.2 discharge repeated at runtime (an OS upgrade is a
// deliberate event that re-runs this before normal admission, S16.3). It
// never mutates the host.
func Probe() Capabilities {
	var c Capabilities
	if v, ok := probeBwrap(); ok {
		c.Bwrap = true
		c.BwrapVersion = v
		c.Notes = append(c.Notes, "bwrap "+v+" at "+SystemBwrap)
	} else {
		c.Notes = append(c.Notes, "bwrap absent at "+SystemBwrap)
	}
	if c.Bwrap {
		if probeUserns() {
			c.Userns = true
			c.Notes = append(c.Notes, "unprivileged user+net namespace forms")
		} else {
			c.Notes = append(c.Notes, "unprivileged userns blocked (AppArmor profile missing or kernel disabled)")
		}
	}
	if probeSeccomp() {
		c.Seccomp = true
		c.Notes = append(c.Notes, "seccomp SECCOMP_MODE_FILTER available")
	} else {
		c.Notes = append(c.Notes, "seccomp filter unsupported — SANCTIONED SKIP of the defense-in-depth layer")
	}
	c.LandlockABI = probeLandlockABI()
	if c.LandlockABI > 0 {
		c.Notes = append(c.Notes, "Landlock ABI "+strconv.Itoa(c.LandlockABI)+" (enforcement is a B1 seam; boundary is bwrap+netns)")
	} else {
		c.Notes = append(c.Notes, "Landlock unsupported — SANCTIONED SKIP")
	}
	if p, v, ok := probeSrt(); ok {
		c.Srt = true
		c.SrtPath = p
		c.SrtVersion = v
		vs := v
		if vs == "" {
			vs = "version unknown"
		}
		c.Notes = append(c.Notes, "sandbox-runtime (srt) present at "+p+" ("+vs+") — ADOPTED primary composition path (S11.1)")
	} else {
		c.Notes = append(c.Notes, "sandbox-runtime (srt) absent — native direct composition is the active funeral-plan fallback (S16.3); srt install batches at the B2 host gate")
	}
	return c
}

// require fails closed when a MANDATORY mechanism (the boundary) is absent.
// Defense-in-depth layers (seccomp/Landlock) never trip this — their absence
// downgrades to a sanctioned skip inside Compose.
func (c Capabilities) require() error {
	if !c.Bwrap {
		return fmt.Errorf("%w: %w", ErrNotComposable, ErrNoBwrap)
	}
	if !c.Userns {
		return fmt.Errorf("%w: %w", ErrNotComposable, ErrUserns)
	}
	return nil
}

// Available reports whether the boundary can be composed on this host — the
// gate a test uses to decide between running a live composition and printing
// a sanctioned skip (S16.3 discipline; the one skip class allowed in a green
// battery, CONVENTIONS §10 precedent).
func (c Capabilities) Available() bool { return c.require() == nil }

func probeBwrap() (string, bool) {
	if _, err := os.Stat(SystemBwrap); err != nil {
		return "", false
	}
	out, err := exec.Command(SystemBwrap, "--version").Output()
	if err != nil {
		return "", false
	}
	// "bubblewrap 0.11.1" → "0.11.1".
	f := strings.Fields(strings.TrimSpace(string(out)))
	if len(f) >= 2 {
		return f[len(f)-1], true
	}
	return strings.TrimSpace(string(out)), true
}

// probeUserns runs the smallest functional userns+netns test: a bwrap that
// unshares user and net and runs true. This exercises the AppArmor-profile
// path (S11.2) rather than trusting a sysctl.
func probeUserns() bool {
	cmd := exec.Command(SystemBwrap, "--unshare-user", "--unshare-net",
		"--ro-bind", "/usr", "/usr", "--ro-bind-try", "/bin", "/bin",
		"--ro-bind-try", "/lib", "/lib", "--ro-bind-try", "/lib64", "/lib64",
		"--proc", "/proc", "--dev", "/dev", "--", "/bin/true")
	return cmd.Run() == nil
}

// probeSeccomp reports whether SECCOMP_MODE_FILTER is available. It asks the
// kernel with a deliberately invalid filter pointer: an unsupported mode
// returns EINVAL, a supported one reaches argument validation and returns
// EFAULT (bad pointer). Either way the host process is never filtered.
func probeSeccomp() bool {
	_, _, errno := unix.Syscall(unix.SYS_SECCOMP, uintptr(unix.SECCOMP_SET_MODE_FILTER), 0, 0)
	// EFAULT: mode accepted, bad args pointer → filter mode supported.
	// EINVAL: mode/flags rejected → unsupported.
	return errno == unix.EFAULT
}

// probeLandlockABI returns the supported Landlock ABI version (0 = none).
// landlock_create_ruleset(NULL, 0, LANDLOCK_CREATE_RULESET_VERSION).
func probeLandlockABI() int {
	const landlockCreateRulesetVersion = 1 << 0
	r, _, errno := unix.Syscall(sysLandlockCreateRuleset, 0, 0, landlockCreateRulesetVersion)
	if errno != 0 {
		return 0
	}
	return int(r)
}

// sysLandlockCreateRuleset is the landlock_create_ruleset syscall number on
// linux/amd64 (444; 445 is landlock_add_rule, 446 landlock_restrict_self).
// x/sys v0.47.0 exposes no Landlock wrappers, so the ABI probe uses the raw
// number; live enforcement (which needs the full ruleset ABI) is the B1 seam
// in landlock.go.
const sysLandlockCreateRuleset = 444

// lookPath resolves a bare binary name against PATH (composition needs an
// absolute host path). Wrapped so sandbox.go stays import-light.
func lookPath(name string) (string, error) { return exec.LookPath(name) }

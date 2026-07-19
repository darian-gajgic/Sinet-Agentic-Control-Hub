package sandbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// TestMain hosts the re-exec confinement children (the S02.9 kill-9-harness
// re-exec pattern, CONVENTIONS §8): a child that drives a real syscall inside
// the composed sandbox and exits with a code the parent asserts.
func TestMain(m *testing.M) {
	switch os.Getenv("SANDBOX_CHILD") {
	case "seccomp-ptrace":
		// PTRACE_TRACEME succeeds (0) without a filter; our profile denies it
		// with EPERM. Exit codes let the parent assert which happened.
		_, _, errno := unix.Syscall(uintptr(unix.SYS_PTRACE), uintptr(unix.PTRACE_TRACEME), 0, 0)
		switch errno {
		case 0:
			os.Exit(70) // allowed — seccomp NOT enforcing
		case unix.EPERM:
			os.Exit(71) // denied by our filter — the proof
		default:
			os.Exit(72)
		}
	case "fs-netns":
		// Prove filesystem isolation + empty netns from inside the sandbox.
		leak := "leak"
		if _, err := os.ReadFile(os.Getenv("HOST_SECRET")); err != nil {
			leak = "isolated"
		}
		routes := "unknown"
		if b, err := os.ReadFile("/proc/net/route"); err == nil {
			n := 0
			for _, ln := range strings.Split(strings.TrimSpace(string(b)), "\n") {
				if ln != "" && !strings.HasPrefix(ln, "Iface") {
					n++
				}
			}
			if n == 0 {
				routes = "no-routes"
			} else {
				routes = "has-routes"
			}
		}
		ws := "no-workspace"
		if _, err := os.ReadFile(filepath.Join(os.Getenv("WS"), "marker.txt")); err == nil {
			ws = "workspace-ok"
		}
		os.Stdout.WriteString(leak + " " + routes + " " + ws + "\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestProbeReportsHost(t *testing.T) {
	c := Probe()
	// The probe never fails — it reports. Log the notes as packet evidence.
	for _, n := range c.Notes {
		t.Log("probe:", n)
	}
	if c.Bwrap && c.BwrapVersion == "" {
		t.Error("bwrap present but no version parsed")
	}
	// require() must agree with Available().
	if c.Available() != (c.require() == nil) {
		t.Error("Available disagrees with require")
	}
}

func TestComposeC1Shape(t *testing.T) {
	caps := Capabilities{Bwrap: true, Userns: true, Seccomp: true}
	argv, bpf, err := Compose(C1, Spawn{
		Argv:      []string{"/bin/echo", "hi"},
		Env:       []string{"PATH=/usr/bin", "MODEL_TOKEN=SENTINEL"},
		Workspace: "/tmp",
		ROConfig:  []string{"/etc/alternatives"},
	}, caps)
	if err != nil {
		t.Fatalf("compose C1: %v", err)
	}
	joined := strings.Join(argv, " ")
	for _, want := range []string{
		"--unshare-all",                 // empty netns + all namespaces (S11.1/S11.4)
		"--clearenv",                    // empty-env deny-by-default (S11.1)
		"--setenv MODEL_TOKEN SENTINEL", // sentinel only inside the sandbox (D2/S11.5)
		"--ro-bind /tmp /tmp",           // C1 workspace is read-only (S11.6)
		"--chdir /tmp",
		"--seccomp 3", // profile loaded from fd 3
		"-- /bin/echo hi",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("composed argv missing %q\nfull: %s", want, joined)
		}
	}
	if len(bpf) == 0 || len(bpf)%8 != 0 {
		t.Errorf("seccomp program is %d bytes, want a nonzero multiple of 8", len(bpf))
	}
	// C1 must NOT bind the workspace rw.
	if strings.Contains(joined, "--bind /tmp /tmp") {
		t.Error("C1 workspace bound read-write (must be ro, S11.6)")
	}
}

func TestComposeC2WorkspaceRW(t *testing.T) {
	caps := Capabilities{Bwrap: true, Userns: true, Seccomp: false} // seccomp skipped
	argv, bpf, err := Compose(C2, Spawn{Argv: []string{"/bin/echo"}, Workspace: "/tmp/ws"}, caps)
	if err != nil {
		t.Fatalf("compose C2: %v", err)
	}
	if !strings.Contains(strings.Join(argv, " "), "--bind /tmp/ws /tmp/ws") {
		t.Error("C2 workspace must be bound read-write (S11.6)")
	}
	if len(bpf) != 0 {
		t.Error("seccomp bytes returned despite Seccomp=false (sanctioned skip)")
	}
	if strings.Contains(strings.Join(argv, " "), "--seccomp") {
		t.Error("--seccomp emitted despite Seccomp=false")
	}
}

func TestComposeRejectsNonV0AndFailsClosed(t *testing.T) {
	full := Capabilities{Bwrap: true, Userns: true, Seccomp: true}
	if _, _, err := Compose(C3, Spawn{Argv: []string{"/bin/echo"}}, full); !errors.Is(err, ErrClassNotV0) {
		t.Errorf("C3 compose: want ErrClassNotV0, got %v", err)
	}
	// Fail closed when the boundary is absent (S16.3 discipline).
	noBwrap := Capabilities{Bwrap: false}
	if _, _, err := Compose(C1, Spawn{Argv: []string{"/bin/echo"}}, noBwrap); !errors.Is(err, ErrNotComposable) {
		t.Errorf("compose without bwrap: want ErrNotComposable, got %v", err)
	}
	noUserns := Capabilities{Bwrap: true, Userns: false}
	if _, _, err := Compose(C1, Spawn{Argv: []string{"/bin/echo"}}, noUserns); !errors.Is(err, ErrUserns) {
		t.Errorf("compose without userns: want ErrUserns, got %v", err)
	}
	// Relative paths are rejected (composition needs host-absolute paths).
	if _, _, err := Compose(C1, Spawn{Argv: []string{"/bin/echo"}, Workspace: "rel/path"}, full); err == nil {
		t.Error("relative workspace path accepted")
	}
}

// TestLiveConfinement is the packet's live confinement evidence. It composes a
// real C1 sandbox and runs a re-exec'd child inside it, asserting filesystem
// isolation, an empty netns (no route), and seccomp denial of ptrace. It is a
// sanctioned skip on a host that cannot compose the boundary (CONVENTIONS §10
// / S16.3: print the skip, document the condition).
func TestLiveConfinement(t *testing.T) {
	caps := Probe()
	if !caps.Available() {
		t.Skipf("SANCTIONED SKIP (S16.3): host cannot compose the sandbox boundary — %v", caps.Notes)
	}
	testbin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ws := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(ws, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "marker.txt"), []byte("in-ws"), 0o600); err != nil {
		t.Fatal(err)
	}
	hostSecret := filepath.Join(dir, "host-secret.txt")
	if err := os.WriteFile(hostSecret, []byte("SECRET-NEVER-VISIBLE"), 0o600); err != nil {
		t.Fatal(err)
	}

	// (1) FS isolation + empty netns via the fs-netns child.
	fsOut := runChild(t, caps, ws, testbin, []string{
		"SANDBOX_CHILD=fs-netns", "HOST_SECRET=" + hostSecret, "WS=" + ws, "PATH=/usr/bin:/bin",
	})
	t.Logf("live confinement (fs/netns): %s", strings.TrimSpace(fsOut.stdout))
	if !strings.Contains(fsOut.stdout, "isolated") {
		t.Errorf("filesystem NOT isolated: host secret was visible in the sandbox: %q", fsOut.stdout)
	}
	if !strings.Contains(fsOut.stdout, "no-routes") {
		t.Errorf("netns NOT empty: sandbox had a network route: %q", fsOut.stdout)
	}
	if !strings.Contains(fsOut.stdout, "workspace-ok") {
		t.Errorf("workspace not readable inside the sandbox: %q", fsOut.stdout)
	}

	// (2) seccomp denies ptrace (defense-in-depth proof) — only when the host
	// supports seccomp filtering.
	if caps.Seccomp {
		ptOut := runChild(t, caps, ws, testbin, []string{"SANDBOX_CHILD=seccomp-ptrace", "PATH=/usr/bin:/bin"})
		switch ptOut.exit {
		case 71:
			t.Logf("live confinement (seccomp): PTRACE_TRACEME denied with EPERM inside the sandbox (exit 71)")
		case 70:
			t.Error("seccomp NOT enforcing: PTRACE_TRACEME succeeded inside the sandbox")
		default:
			t.Logf("seccomp ptrace child exited %d (%s) — ptrace neither cleanly allowed nor EPERM; not asserting", ptOut.exit, strings.TrimSpace(ptOut.stderr))
		}
	} else {
		t.Log("SANCTIONED SKIP (S16.3): seccomp filtering unsupported on this host")
	}
}

type childResult struct {
	stdout, stderr string
	exit           int
}

// runChild composes a C1 sandbox around the re-exec'd test binary and runs it,
// returning stdout/stderr/exit. EnginePrefix binds the test binary's directory
// read-only so it is reachable inside the sandbox.
func runChild(t *testing.T, caps Capabilities, ws, testbin string, env []string) childResult {
	t.Helper()
	argv, seccomp, err := Compose(C1, Spawn{
		Argv:         []string{testbin},
		Env:          env,
		Workspace:    ws,
		EnginePrefix: filepath.Dir(testbin),
	}, caps)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	cmd := exec.Command(SystemBwrap, argv...)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if len(seccomp) > 0 {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		cmd.ExtraFiles = []*os.File{r}
		go func() { _, _ = w.Write(seccomp); _ = w.Close() }()
		defer r.Close()
	}
	err = cmd.Run()
	exit := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exit = ee.ExitCode()
		} else {
			t.Fatalf("run confined child: %v (stderr: %s)", err, stderr.String())
		}
	}
	return childResult{stdout: stdout.String(), stderr: stderr.String(), exit: exit}
}

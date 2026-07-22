package sandbox

import (
	"strings"
	"testing"
)

// bridge_test.go — the S12.6 GPU-broker bridge injection seam (brief R6/R7/OQ3):
// Compose binds the per-sandbox UDS + sets the worker's /v1 env for a granted
// class; a C0 connector carrying a bridge FAILS CLOSED; and BridgeBudgetFor is
// the class→budget policy (C0 never / C1-C2 standard / C3 tighter). Dormant seam
// (no v0 consumer), hermetically tested.

func fullCaps() Capabilities { return Capabilities{Bwrap: true, Userns: true, Seccomp: true} }

func bridge() *Bridge {
	return &Bridge{
		SocketPath: "/run/sinet/bridge-run1.sock", EnvName: "OPENAI_BASE_URL",
		BaseURL: "http://localhost/v1", TokenEnvName: "OPENAI_API_KEY", Token: "run1-token",
	}
}

// TestComposeGrantsBridgeC1 proves a granted class (C1) binds the UDS read-write
// and sets the worker's /v1 env at it (R7).
func TestComposeGrantsBridgeC1(t *testing.T) {
	argv, _, err := Compose(C1, Spawn{Argv: []string{"/bin/echo"}, Bridge: bridge()}, fullCaps())
	if err != nil {
		t.Fatalf("Compose C1 with bridge: %v", err)
	}
	joined := strings.Join(argv, " ")
	if !adjacent(argv, "--bind", "/run/sinet/bridge-run1.sock") {
		t.Errorf("bridge socket not bound read-write: %s", joined)
	}
	if !triple(argv, "--setenv", "OPENAI_BASE_URL", "http://localhost/v1") {
		t.Errorf("worker /v1 base URL not set to the bridge: %s", joined)
	}
	if !triple(argv, "--setenv", "OPENAI_API_KEY", "run1-token") {
		t.Errorf("per-run token not injected: %s", joined)
	}
	// Never a raw device or a routable host address (S12.6).
	if strings.Contains(joined, "/dev/nvidia") {
		t.Error("a sandbox must NEVER see /dev/nvidia* (S12.6)")
	}
}

// TestComposeC0BridgeFailsClosed proves a C0 connector carrying a bridge is
// refused — C0 never gets it (R6), and the sandbox is the boundary (fail closed).
func TestComposeC0BridgeFailsClosed(t *testing.T) {
	_, _, err := Compose(C0, Spawn{Argv: []string{"/bin/echo"}, Bridge: bridge()}, fullCaps())
	if err == nil {
		t.Fatal("a C0 connector with a bridge must fail closed (S12.6/S11.6)")
	}
	if !strings.Contains(err.Error(), "C0") {
		t.Errorf("error = %v, want a C0-bridge refusal", err)
	}
}

// TestBridgeBudgetFor proves the class→bridge policy (R6): C0 never; C1/C2
// standard; C3 tighter (v0.1 hostile-input step-up).
func TestBridgeBudgetFor(t *testing.T) {
	if g, _, _ := BridgeBudgetFor(C0); g {
		t.Error("C0 must never be granted the bridge")
	}
	g1, r1, b1 := BridgeBudgetFor(C1)
	g3, r3, b3 := BridgeBudgetFor(C3)
	if !g1 || !g3 {
		t.Fatal("C1 and C3 must be granted the bridge")
	}
	if !(r3 < r1 && b3 < b1) {
		t.Errorf("C3 budgets (%d/%d) must be TIGHTER than C1 (%d/%d)", r3, b3, r1, b1)
	}
}

func adjacent(argv []string, a, b string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == a && argv[i+1] == b {
			return true
		}
	}
	return false
}

func triple(argv []string, a, b, c string) bool {
	for i := 0; i+2 < len(argv); i++ {
		if argv[i] == a && argv[i+1] == b && argv[i+2] == c {
			return true
		}
	}
	return false
}

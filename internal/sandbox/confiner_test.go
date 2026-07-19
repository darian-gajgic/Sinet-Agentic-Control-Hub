package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
)

// TestEgressPolicyReadsSettings proves the four S18 S11-domain ⚙ keys are
// consumed by dotted key through the registry — never hardcoded (S01.10). The
// registry defaults are the S11 ratified values.
func TestEgressPolicyReadsSettings(t *testing.T) {
	co := NewComposer(settings.New(), nil)

	c2, err := co.EgressPolicyFor(C2, nil)
	if err != nil {
		t.Fatalf("C2 egress policy: %v", err)
	}
	if c2.Mode != NetRegistries {
		t.Errorf("C2 net mode = %q, want registries", c2.Mode)
	}
	if !containsStr(c2.DenyCIDRs, "169.254.169.254/32") {
		t.Errorf("egress_deny_cidrs missing the metadata-IP floor: %v", c2.DenyCIDRs)
	}
	if !c2.BlockDoH {
		t.Error("block_outbound_doh default should be true (S11 ⚙)")
	}
	if !c2.TLSTerminate {
		t.Error("model_egress_tls_terminate default should be true (S11 ⚙)")
	}
	if !c2.RequiresSubstrate {
		t.Error("C2 requires the deferred host egress substrate")
	}
	// The C2 registry allowlist ships empty (preset-not-installed, S18 help).
	if len(c2.AllowHosts) != 0 {
		t.Errorf("c2_registry_allowlist default should be empty, got %v", c2.AllowHosts)
	}

	c1, err := co.EgressPolicyFor(C1, nil)
	if err != nil {
		t.Fatalf("C1 egress policy: %v", err)
	}
	if c1.Mode != NetNone || c1.RequiresSubstrate {
		t.Errorf("C1 should be empty-netns / no substrate, got mode=%q substrate=%v", c1.Mode, c1.RequiresSubstrate)
	}
}

// TestConfineRunsLive drives the full adapters.Confiner path: NewComposer →
// Confine → a runnable *exec.Cmd whose engine executes INSIDE the sandbox. It
// asserts isolation on the real composition. Sanctioned skip when the host
// cannot compose the boundary (S16.3 / CONVENTIONS §10).
func TestConfineRunsLive(t *testing.T) {
	co := NewComposer(settings.New(), nil)
	if !co.Caps().Available() {
		t.Skipf("SANCTIONED SKIP (S16.3): host cannot compose the boundary — %v", co.Caps().Notes)
	}
	testbin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ws := filepath.Join(dir, "ws")
	if err := os.MkdirAll(ws, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "marker.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	hostSecret := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(hostSecret, []byte("SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd, cleanup, err := co.Confine(
		adapters.StartRequest{RunID: "run-x", UserID: "u", Class: string(C1)},
		adapters.SpawnSpec{
			Argv:         []string{testbin},
			Env:          []string{"SANDBOX_CHILD=fs-netns", "HOST_SECRET=" + hostSecret, "WS=" + ws, "PATH=/usr/bin:/bin"},
			Workspace:    ws,
			EnginePrefix: filepath.Dir(testbin),
		})
	if err != nil {
		t.Fatalf("Confine: %v", err)
	}
	defer cleanup()
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !asExitErr(err, &ee) {
			t.Fatalf("confined run: %v (%s)", err, out.String())
		}
	}
	got := strings.TrimSpace(out.String())
	t.Logf("Confine round-trip: %s", got)
	if !strings.Contains(got, "isolated") || !strings.Contains(got, "no-routes") {
		t.Errorf("Confine did not isolate the engine: %q", got)
	}
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func asExitErr(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}

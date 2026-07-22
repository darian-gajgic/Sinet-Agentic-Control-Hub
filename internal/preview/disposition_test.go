package preview

import (
	"os"
	"path/filepath"
	"testing"
)

// tree writes marker files into a fresh temp dir and returns it.
func tree(t *testing.T, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestDetectRunnerHeuristics: the four spec-named lanes are detected
// deterministically on fixture trees (Spec S13.8; R2).
func TestDetectRunnerHeuristics(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		lane  Lane
		tool  string
	}{
		{"static index.html", []string{"index.html"}, LaneStatic, "static"},
		{"npm (no lockfile)", []string{"package.json"}, LaneDevServer, "npm"},
		{"pnpm (lockfile)", []string{"package.json", "pnpm-lock.yaml"}, LaneDevServer, "pnpm"},
		{"uv pyproject", []string{"pyproject.toml"}, LaneDevServer, "uv"},
		{"mise", []string{"mise.toml"}, LaneDevServer, "mise"},
		{"notebook file", []string{"analysis.ipynb"}, LaneNotebook, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lane, runner := Detect(tree(t, c.files...), "code", "")
			if lane != c.lane {
				t.Fatalf("lane = %s, want %s", lane, c.lane)
			}
			if c.tool != "" {
				if runner == nil || runner.Tool != c.tool {
					t.Fatalf("tool = %v, want %s", runner, c.tool)
				}
				if runner.Source != "heuristic" {
					t.Fatalf("source = %s, want heuristic", runner.Source)
				}
			}
		})
	}
}

// TestRegistryCommandWinsOverHeuristics: an owner-approved registry preview
// command takes precedence over the heuristics (Spec S13.7/S13.8; R3).
func TestRegistryCommandWinsOverHeuristics(t *testing.T) {
	// index.html would heuristically be static; the registry command overrides.
	lane, runner := Detect(tree(t, "index.html"), "code", "vite dev --port 3000")
	if lane != LaneDevServer {
		t.Fatalf("lane = %s, want dev-server (registry wins)", lane)
	}
	if runner == nil || runner.Source != "registry" || runner.Tool != "registry" {
		t.Fatalf("runner = %v, want registry-sourced", runner)
	}
	want := []string{"vite", "dev", "--port", "3000"}
	if len(runner.Argv) != len(want) {
		t.Fatalf("argv = %v, want %v", runner.Argv, want)
	}
	for i := range want {
		if runner.Argv[i] != want[i] {
			t.Fatalf("argv = %v, want %v", runner.Argv, want)
		}
	}
}

// TestPerTypeDispositions: every type gets a defined lane — no refusal branch,
// no broken-iframe path (Spec S13.8; R4). CLI is data-driven, never guessed.
func TestPerTypeDispositions(t *testing.T) {
	// Sidecar: a compose file → the container-tier state (conservative).
	if lane, _ := Detect(tree(t, "docker-compose.yml"), "code", ""); lane != LaneSidecar {
		t.Errorf("compose file → %s, want sidecar", lane)
	}
	// Registry command wins over sidecar detection (the owner said how).
	if lane, _ := Detect(tree(t, "docker-compose.yml", "package.json"), "code", "npm start"); lane != LaneDevServer {
		t.Errorf("compose + registry cmd → %s, want dev-server", lane)
	}
	// CLI: explicit type only, never heuristic-guessed (R20).
	lane, runner := Detect(tree(t, "package.json"), "cli", "mytool serve")
	if lane != LaneCLI {
		t.Errorf("cli type → %s, want cli", lane)
	}
	if runner == nil || runner.Tool != "ttyd" {
		t.Errorf("cli runner = %v, want ttyd", runner)
	}
	// A code deliverable in a bare tree → the honest no-preview state.
	if lane, _ := Detect(tree(t), "text", ""); lane != LaneNone {
		t.Errorf("bare tree → %s, want none", lane)
	}
	// Heuristics NEVER produce a CLI (a bare package.json is a web dev server).
	if lane, _ := Detect(tree(t, "package.json"), "code", ""); lane == LaneCLI {
		t.Errorf("heuristics guessed a CLI — forbidden (R20)")
	}
}

// TestHostInjectionsPerProvider: allowedHosts injection is present in the
// runner invocation data per provider (Spec S13.8: "allowedHosts injected, or
// Vite HMR silently dies"; R13). The Vite lane names server.allowedHosts.
func TestHostInjectionsPerProvider(t *testing.T) {
	host := "preview-p1.sinet.example.ts.net"
	for _, tool := range []string{"pnpm", "npm"} {
		inj := hostInjections(tool, host)
		if !hasConfig(inj, "server.allowedHosts", host) {
			t.Errorf("%s: missing Vite server.allowedHosts injection: %+v", tool, inj)
		}
		if !hasEnv(inj, "SINET_PREVIEW_HOST", host) {
			t.Errorf("%s: missing platform host env: %+v", tool, inj)
		}
	}
	for _, tool := range []string{"uv", "mise", "registry"} {
		if !hasEnv(hostInjections(tool, host), "SINET_PREVIEW_HOST", host) {
			t.Errorf("%s: missing host injection", tool)
		}
	}
	// Static + ttyd need no dev-server host check.
	if hostInjections("static", host) != nil || hostInjections("ttyd", host) != nil {
		t.Errorf("static/ttyd should carry no host injection")
	}
}

func hasConfig(inj []HostInjection, key, val string) bool {
	for _, hi := range inj {
		if hi.Mechanism == "config" && hi.Key == key && hi.Value == val {
			return true
		}
	}
	return false
}

func hasEnv(inj []HostInjection, key, val string) bool {
	for _, hi := range inj {
		if hi.Mechanism == "env" && hi.Key == key && hi.Value == val {
			return true
		}
	}
	return false
}

// TestTtydLookupOverride: ttyd is probe-gated (PATH + SINET_TTYD_PATH), and an
// absent binary yields honest absence — never an install (Spec S13.8; R20).
func TestTtydLookupOverride(t *testing.T) {
	// Absent by override (a non-existent path) → not found.
	t.Setenv(EnvTtydPath, filepath.Join(t.TempDir(), "no-such-ttyd"))
	if _, ok := DefaultToolLookup("ttyd"); ok {
		t.Fatalf("ttyd override to a missing path should report absent")
	}
	// Present by override (a stub executable) → found at the override path.
	stub := filepath.Join(t.TempDir(), "ttyd")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvTtydPath, stub)
	p, ok := DefaultToolLookup("ttyd")
	if !ok || p != stub {
		t.Fatalf("ttyd override lookup = %q,%v, want %q,true", p, ok, stub)
	}
}

// TestTtydInvocation: the ttyd invocation serves the command over the probe→
// pool→route port (Spec S13.8; R20). Asserted with a stub path — no real ttyd.
func TestTtydInvocation(t *testing.T) {
	inv := ttydInvocation("/opt/ttyd", ttydDefaultPort, []string{"mytool", "serve"})
	want := []string{"/opt/ttyd", "-p", "7681", "-W", "mytool", "serve"}
	if len(inv) != len(want) {
		t.Fatalf("invocation = %v, want %v", inv, want)
	}
	for i := range want {
		if inv[i] != want[i] {
			t.Fatalf("invocation = %v, want %v", inv, want)
		}
	}
}

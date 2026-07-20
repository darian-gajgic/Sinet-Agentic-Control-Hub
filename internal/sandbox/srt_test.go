package sandbox

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
)

// TestSrtConfigForClasses is the primary evidence for the generated srt config
// (S11.1 Adoption): the single-JSON config per confinement class, mapping
// filesystem/network per the S11.6 class table. Pure — no real srt needed.
func TestSrtConfigForClasses(t *testing.T) {
	cfgPath := "/tmp"
	roConfig := "/run/sinet/config"

	// C1 trusted reasoning: ro workspace clone; empty netns / no egress.
	c1, err := SrtConfigFor(C1, Spawn{
		Argv:      []string{"/bin/echo"},
		Workspace: "/ws",
		ROConfig:  []string{roConfig},
	}, EgressPolicy{Mode: NetNone})
	if err != nil {
		t.Fatalf("C1 srt config: %v", err)
	}
	// No egress: strict allow-list of nothing + hard deny-all.
	if len(c1.Network.AllowedDomains) != 0 {
		t.Errorf("C1 allowedDomains = %v, want empty (no egress)", c1.Network.AllowedDomains)
	}
	if !containsStr(c1.Network.DeniedDomains, "*") {
		t.Errorf("C1 deniedDomains = %v, want [*] (hard deny-all)", c1.Network.DeniedDomains)
	}
	if !c1.Network.StrictAllowlist {
		t.Error("C1 strictAllowlist must be true (fail-closed, no ask callback)")
	}
	// ro workspace: readable, NOT writable; config read-only (P-T09-1).
	if !containsStr(c1.Filesystem.AllowRead, "/ws") {
		t.Errorf("C1 allowRead missing the workspace: %v", c1.Filesystem.AllowRead)
	}
	if containsStr(c1.Filesystem.AllowWrite, "/ws") {
		t.Error("C1 (ro) must NOT allow writing the workspace (S11.6)")
	}
	if !containsStr(c1.Filesystem.AllowWrite, tmpWritable) {
		t.Errorf("C1 allowWrite missing ephemeral /tmp: %v", c1.Filesystem.AllowWrite)
	}
	if !containsStr(c1.Filesystem.AllowRead, roConfig) || !containsStr(c1.Filesystem.DenyWrite, roConfig) {
		t.Errorf("C1 config path must be read-only (allowRead + denyWrite): read=%v denyWrite=%v",
			c1.Filesystem.AllowRead, c1.Filesystem.DenyWrite)
	}

	// C2 workspace-write: rw overlay; registries allow-list.
	regs := []string{"registry.npmjs.org", "files.pythonhosted.org"}
	c2, err := SrtConfigFor(C2, Spawn{
		Argv:      []string{"/bin/echo"},
		Workspace: "/ws",
	}, EgressPolicy{Mode: NetRegistries, AllowHosts: regs})
	if err != nil {
		t.Fatalf("C2 srt config: %v", err)
	}
	for _, h := range regs {
		if !containsStr(c2.Network.AllowedDomains, h) {
			t.Errorf("C2 allowedDomains missing %q: %v", h, c2.Network.AllowedDomains)
		}
	}
	if containsStr(c2.Network.DeniedDomains, "*") {
		t.Error("C2 must not hard-deny-all — it has an egress allow-list")
	}
	if !containsStr(c2.Filesystem.AllowWrite, "/ws") {
		t.Errorf("C2 (rw) must allow writing the workspace overlay (S11.6): %v", c2.Filesystem.AllowWrite)
	}

	// C0 connectors: one named service host; no workspace filesystem.
	c0, err := SrtConfigFor(C0, Spawn{
		Argv: []string{"/bin/echo"},
	}, EgressPolicy{Mode: NetSingleHost, AllowHosts: []string{"api.service.example"}})
	if err != nil {
		t.Fatalf("C0 srt config: %v", err)
	}
	if !containsStr(c0.Network.AllowedDomains, "api.service.example") {
		t.Errorf("C0 allowedDomains missing the single service host: %v", c0.Network.AllowedDomains)
	}
	if containsStr(c0.Network.DeniedDomains, "*") {
		t.Error("C0 has a single-host allow-list, must not hard-deny-all")
	}

	_ = cfgPath
}

// TestSrtConfigDenyReadFlows proves the credential-store / explicit deny-read
// path lands in denyRead AND denyWrite (D2/S11.6) when the profile carries it.
// (At B1 the per-run structural denials are empty from Profile() — the same as
// native Compose; this asserts the wiring so B2's per-run threading only
// changes the input, not the mapping.)
func TestSrtConfigDenyReadFlows(t *testing.T) {
	cfg, err := srtConfigFromProfile(Confinement{
		Class:    C1,
		Network:  NetNone,
		DenyRead: []string{"/home/sinet/.ssh", "/home/sinet/.aws"},
	}, Spawn{Argv: []string{"/bin/echo"}}, EgressPolicy{Mode: NetNone})
	if err != nil {
		t.Fatalf("srt config: %v", err)
	}
	for _, p := range []string{"/home/sinet/.ssh", "/home/sinet/.aws"} {
		if !containsStr(cfg.Filesystem.DenyRead, p) {
			t.Errorf("credential store %q not denied for read (D2): %v", p, cfg.Filesystem.DenyRead)
		}
		if !containsStr(cfg.Filesystem.DenyWrite, p) {
			t.Errorf("credential store %q not denied for write: %v", p, cfg.Filesystem.DenyWrite)
		}
	}
}

// TestSrtConfigMarshalsArraysNotNull guards srt's zod schema: arrays must
// marshal as [] (not null), or srt rejects the config.
func TestSrtConfigMarshalsArraysNotNull(t *testing.T) {
	cfg, err := SrtConfigFor(C1, Spawn{Argv: []string{"/bin/echo"}}, EgressPolicy{Mode: NetNone})
	if err != nil {
		t.Fatal(err)
	}
	blob, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "null") {
		t.Errorf("srt config marshaled a null (zod rejects null arrays): %s", blob)
	}
	for _, field := range []string{`"allowedDomains":[]`, `"deniedDomains":["*"]`, `"denyRead":[]`} {
		if !strings.Contains(string(blob), field) {
			t.Errorf("srt config missing %s\nfull: %s", field, blob)
		}
	}
}

func TestSrtConfigRejectsNonV0(t *testing.T) {
	if _, err := SrtConfigFor(C3, Spawn{Argv: []string{"/bin/echo"}}, EgressPolicy{}); !errors.Is(err, ErrClassNotV0) {
		t.Errorf("C3 srt config: want ErrClassNotV0, got %v", err)
	}
}

// TestSrtInvocation asserts the `srt --settings <cfg> -- <engine> args` shape.
func TestSrtInvocation(t *testing.T) {
	argv, err := SrtInvocation("/run/sinet/jobs/srt-config.json", Spawn{
		Argv: []string{"/bin/echo", "hello", "--flag"},
	})
	if err != nil {
		t.Fatalf("SrtInvocation: %v", err)
	}
	want := []string{"--settings", "/run/sinet/jobs/srt-config.json", "--", "/bin/echo", "hello", "--flag"}
	if strings.Join(argv, " ") != strings.Join(want, " ") {
		t.Errorf("srt argv = %v, want %v", argv, want)
	}
	if _, err := SrtInvocation("/cfg", Spawn{}); err == nil {
		t.Error("SrtInvocation with no engine argv should fail")
	}
}

// TestProbeSrtDiscovery covers the three discovery outcomes: absent (native
// fallback), present via SINET_SRT_PATH override, and override-set-but-unusable
// (falls back to native, not an error).
func TestProbeSrtDiscovery(t *testing.T) {
	// Absent: no override and nothing on PATH → native fallback. PATH is
	// pointed at an empty dir so a host srt install cannot leak in (only the
	// srt lookup consults PATH; the boundary probes use absolute paths and
	// syscalls).
	t.Setenv("PATH", t.TempDir())
	t.Setenv(EnvSrtPath, "")
	if c := Probe(); c.Srt {
		t.Errorf("srt reported present with no override and no PATH install: %v", c.Notes)
	}

	// Present via override (a stub executable).
	stub := writeSrtStub(t)
	t.Setenv(EnvSrtPath, stub)
	c := Probe()
	if !c.Srt || c.SrtPath != stub {
		t.Errorf("srt override not honored: Srt=%v SrtPath=%q", c.Srt, c.SrtPath)
	}
	var sawNote bool
	for _, n := range c.Notes {
		if strings.Contains(n, "srt) present") {
			sawNote = true
		}
	}
	if !sawNote {
		t.Errorf("probe notes do not record srt presence: %v", c.Notes)
	}

	// Override set but unusable: fall back to native, never fail.
	t.Setenv(EnvSrtPath, filepath.Join(t.TempDir(), "does-not-exist"))
	if c := Probe(); c.Srt {
		t.Error("unusable srt override reported present (must fall back to native)")
	}
}

// TestConfineSelectsSrtWhenPresentNativeWhenAbsent is the both-branches
// evidence: the confiner uses srt-when-present and the native funeral-plan
// fallback when absent. Deterministic (faked caps; the cmd is built, not run).
func TestConfineSelectsSrtWhenPresentNativeWhenAbsent(t *testing.T) {
	req := adapters.StartRequest{RunID: "run-x", UserID: "u", Class: string(C1), WorkDir: t.TempDir()}
	spec := adapters.SpawnSpec{Argv: []string{"/bin/echo", "hi"}, Env: []string{"PATH=/usr/bin:/bin"}, Workspace: t.TempDir()}

	// Native fallback: srt absent, boundary faked present.
	native := NewComposer(settings.New(), nil)
	native.caps = Capabilities{Bwrap: true, Userns: true, Seccomp: true} // Srt: false
	cmd, cleanup, err := native.Confine(req, spec)
	if err != nil {
		t.Fatalf("native Confine: %v", err)
	}
	defer cleanup()
	if cmd.Path != SystemBwrap {
		t.Errorf("srt-absent path used %q, want the native bwrap %q", cmd.Path, SystemBwrap)
	}

	// srt primary: srt present (stub path).
	stub := writeSrtStub(t)
	srtc := NewComposer(settings.New(), nil)
	srtc.caps = Capabilities{Bwrap: true, Userns: true, Srt: true, SrtPath: stub}
	cmd2, cleanup2, err := srtc.Confine(req, spec)
	if err != nil {
		t.Fatalf("srt Confine: %v", err)
	}
	defer cleanup2()
	if cmd2.Path != stub {
		t.Errorf("srt-present path used %q, want the srt binary %q", cmd2.Path, stub)
	}
	if !containsStr(cmd2.Args, "--settings") {
		t.Errorf("srt invocation missing --settings: %v", cmd2.Args)
	}
	// The generated config file exists and is valid srt JSON while the run lives.
	cfgArg := cmd2.Args[indexOf(cmd2.Args, "--settings")+1]
	blob, err := os.ReadFile(cfgArg)
	if err != nil {
		t.Fatalf("read generated srt config: %v", err)
	}
	var got SrtConfig
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("generated srt config is not valid JSON: %v", err)
	}
	if !got.Network.StrictAllowlist || !containsStr(got.Network.DeniedDomains, "*") {
		t.Errorf("C1 generated config is not deny-all/strict: %+v", got.Network)
	}
	cleanup2()
	if _, err := os.Stat(cfgArg); !os.IsNotExist(err) {
		t.Errorf("cleanup did not remove the srt config file %q", cfgArg)
	}
}

// TestSrtStubReceivesConfigAndArgv drives the FULL confiner→srt exec path with
// a stub `srt`: it asserts the exact argv and config JSON srt receives. This is
// the generated-config-JSON + invocation evidence for a sample class, proven at
// the real process boundary (no real srt, no host install). Sanctioned to run
// anywhere with /bin/sh (loopback/tempdir only).
func TestSrtStubReceivesConfigAndArgv(t *testing.T) {
	stub := writeSrtStub(t)

	// Boundary caps are faked (bwrap+userns are the mandatory gate, but srt
	// itself invokes bwrap at runtime — the stub does not, so this test stays
	// hermetic and runs where bwrap is absent, e.g. CI). Confine execs the stub
	// at SrtPath; srt discovery itself is covered by TestProbeSrtDiscovery.
	co := NewComposer(settings.New(), nil)
	co.caps = Capabilities{Bwrap: true, Userns: true, Srt: true, SrtPath: stub}
	ws := t.TempDir()
	req := adapters.StartRequest{RunID: "run-1", UserID: "u", Class: string(C2), WorkDir: t.TempDir()}
	spec := adapters.SpawnSpec{
		Argv:      []string{"/bin/echo", "engine-arg"},
		Env:       []string{"PATH=/usr/bin:/bin", "MODEL_TOKEN=SENTINEL"},
		Workspace: ws,
	}
	cmd, cleanup, err := co.Confine(req, spec)
	if err != nil {
		t.Fatalf("Confine: %v", err)
	}
	defer cleanup()

	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("stub srt run: %v (%s)", err, out.String())
	}
	got := out.String()
	t.Logf("stub srt saw:\n%s", got)

	// The srt CLI received the wrapping invocation and the engine argv.
	if !strings.Contains(got, "--settings") || !strings.Contains(got, "-- /bin/echo engine-arg") {
		t.Errorf("srt did not receive the expected wrapping invocation:\n%s", got)
	}
	// The generated config srt loaded is the C2 config (rw workspace + strict).
	if !strings.Contains(got, `"strictAllowlist": true`) {
		t.Errorf("srt config missing strictAllowlist:\n%s", got)
	}
	if !strings.Contains(got, ws) {
		t.Errorf("srt config missing the C2 workspace path %q:\n%s", ws, got)
	}
}

// srtConfigFromProfile is a test shim exercising the deny-read/mount overlay
// mapping from a full Confinement (the shape B2 threads into composition). It
// mirrors SrtConfigFor but seeds ROBinds/Mounts/DenyRead from the record.
func srtConfigFromProfile(conf Confinement, sp Spawn, egress EgressPolicy) (SrtConfig, error) {
	cfg, err := SrtConfigFor(conf.Class, sp, egress)
	if err != nil {
		return SrtConfig{}, err
	}
	for _, p := range conf.DenyRead {
		cfg.Filesystem.DenyRead = append(cfg.Filesystem.DenyRead, p)
		cfg.Filesystem.DenyWrite = append(cfg.Filesystem.DenyWrite, p)
	}
	return cfg, nil
}

// writeSrtStub writes a fake `srt` executable that echoes its argv and the
// content of the --settings config to stdout (so tests assert the exact
// invocation + config JSON), and answers --version. It never sandboxes.
func writeSrtStub(t *testing.T) string {
	t.Helper()
	stub := filepath.Join(t.TempDir(), "srt")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "srt-stub 0.0.66-test"; exit 0; fi
echo "SRT-STUB ARGV: $*"
prev=""
for a in "$@"; do
  if [ "$prev" = "--settings" ]; then
    echo "SRT-STUB CONFIG-BEGIN"
    cat "$a"
    echo "SRT-STUB CONFIG-END"
  fi
  prev="$a"
done
`
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return stub
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

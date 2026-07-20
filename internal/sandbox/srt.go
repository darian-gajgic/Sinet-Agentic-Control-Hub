package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// srt.go is the ADOPTED, ratified composition path of the S11.1 stack:
// Anthropic's Apache-2.0 `sandbox-runtime` ("srt"), bound as adopted for the
// bwrap + seccomp + proxy core [Spec S11.1 "Adoption"; G2 D2.2; S16.3
// sandbox-runtime row]. srt wraps ANY process with a single-JSON config (no
// container image) and supplies exactly the S11.4 egress shape — on Linux it
// removes the sandboxed process's network namespace entirely and routes all
// traffic through host-side proxies reached over bind-mounted Unix-domain
// sockets. Adopt-don't-fork holds (S16.1): srt wraps the engine EXTERNALLY;
// the engine is configured, never modified.
//
// This file generates srt's config from a confinement class and builds the
// `srt` invocation. The native direct composition (sandbox.go Compose →
// bwrap argv) is RETAINED as the S16.3 funeral-plan / fallback (the ratified
// "direct composition of the same kernel primitives" replacement path), NOT
// as the primary design. BuildPlan selects srt when the Probe finds it
// present and falls back to the native composition when it is absent, logging
// which path is active — so dev-mode confinement keeps working now (srt is a
// host-deferred install batching at the B2 gate, S16.3) and the native code
// is the spec-sanctioned fallback rather than a rogue primary.
//
// srt is a Node CLI, not a Go module — it is invoked as an external process
// (S01.5 sanctions consuming JS organs as external processes) and carries a
// `library` components.lock entry with NO `modules` claim (like the claude
// engine + os-mechanism entries). Its pin is LIVE-verified at build time.

// EnvSrtPath overrides srt discovery with an explicit binary path. It is the
// operator's activation knob (srt installed outside PATH, or the B2
// host-activation config) AND the test-injection seam (a stub `srt` under
// t.TempDir()). Set + usable ⇒ the srt path is active; set + unusable ⇒ Probe
// falls back to native and logs it.
const EnvSrtPath = "SINET_SRT_PATH"

// srtLookPath is the discovery hook (indirected so tests are hermetic).
var srtLookPath = func() (path string, ok bool) {
	if p := os.Getenv(EnvSrtPath); p != "" {
		if isExecutableFile(p) {
			return p, true
		}
		return "", false // override set but unusable: fall back to native (logged)
	}
	if p, err := exec.LookPath("srt"); err == nil {
		return p, true
	}
	return "", false
}

func isExecutableFile(p string) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false
	}
	return fi.Mode().Perm()&0o111 != 0
}

// probeSrt discovers the srt CLI and its version (best-effort). Absence is not
// a failure — the native funeral-plan fallback covers it (S16.3). srt still
// rides bwrap+userns under the hood, so the MANDATORY boundary probe
// (Capabilities.require) governs BOTH paths.
func probeSrt() (path, version string, ok bool) {
	path, ok = srtLookPath()
	if !ok {
		return "", "", false
	}
	return path, srtVersion(path), true
}

// srtVersion runs `srt --version` best-effort; the value is evidence only,
// never asserted, so any error yields an empty string.
func srtVersion(path string) string {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// SrtConfig is srt's single-JSON config (the user-facing shape validated by
// srt's SandboxRuntimeConfigSchema at the pinned version): the `network` and
// `filesystem` sections. Field names and semantics are srt's own — this is a
// generated config for an ADOPTED, UNMODIFIED tool, not a Sinet schema.
//
// srt's `credentials` injection block (TLS-terminating model-egress
// substitution) is DEFERRED with the credential-injection proxy to B2
// (⚙ sandbox.model_egress_tls_terminate, S11.5); it is intentionally omitted
// here (the section is optional in srt's schema). The broker never puts a
// secret in the sandbox regardless (D2) — auth-profiles resolve host-side.
type SrtConfig struct {
	Network    SrtNetwork    `json:"network"`
	Filesystem SrtFilesystem `json:"filesystem"`
}

// SrtNetwork mirrors srt's NetworkConfigSchema (the subset Sinet drives).
// srt does hostname allowlisting — the S11.4 "convenience layer" that decides
// WHICH allowed host. The IP:port default-drop that actually CONTAINS (⚙
// sandbox.egress_deny_cidrs + block_outbound_doh) is the host nftables layer
// (S11.4 #2), carried on EgressPolicy and installed with the deferred host
// egress substrate — NOT expressible as srt domain patterns.
type SrtNetwork struct {
	// AllowedDomains is the per-class hostname allow-list (C0 single host; C2
	// registries + CDN hosts; empty for no-egress classes).
	AllowedDomains []string `json:"allowedDomains"`
	// DeniedDomains carries a bare "*" (deny-all) for no-egress classes; empty
	// otherwise (a wildcard here would deny even allowed hosts — checked first).
	DeniedDomains []string `json:"deniedDomains"`
	// StrictAllowlist makes the allow-list policy enforcement, not a
	// prompt-suppression hint: an unmatched host is denied without consulting
	// any ask callback (headless runs have none) — the fail-closed posture.
	StrictAllowlist bool `json:"strictAllowlist"`
}

// SrtFilesystem mirrors srt's FilesystemConfigSchema. srt's read model is
// deny-then-allow-back (reads allowed by default; deny broad regions, re-allow
// specifics — allowRead beats denyRead); its write model is allow-only (writes
// denied by default; allowWrite grants, denyWrite beats allowWrite). Sinet
// drives it toward S11.6: credential stores are always deny-read (D2), config
// paths are deny-write (P-T09-1, symlink-resolved by srt/bwrap), and only the
// per-run workspace (C2) plus ephemeral /tmp are writable.
type SrtFilesystem struct {
	DenyRead   []string `json:"denyRead"`
	AllowRead  []string `json:"allowRead"`
	AllowWrite []string `json:"allowWrite"`
	DenyWrite  []string `json:"denyWrite"`
}

// tmpWritable is the ephemeral scratch every class gets (the native path binds
// a fresh --tmpfs /tmp; srt's allow-only write model needs it named).
const tmpWritable = "/tmp"

// SrtConfigFor generates srt's config for a class + spawn + compiled egress
// policy (Spec S11.6 class table + S11.3 mount table). It is a PURE function
// (no I/O) so the exact config JSON is unit-tested without a real srt. The
// caller writes the marshaled bytes to a file passed as `srt --settings`.
//
// Filesystem hardening here is the spec-MANDATED minimum (credential-store
// deny-read; config deny-write; workspace-scoped writes). Broader deny-then-
// allow tightening of srt's permissive default reads is tuned against the
// DEFERRED B2 live srt-wrapping smoke, not guessed blind at B1 (the engine's
// real read set is only observable live) — noted so B2 hardens, never loosens.
func SrtConfigFor(c Class, sp Spawn, egress EgressPolicy) (SrtConfig, error) {
	if !c.isV0() {
		return SrtConfig{}, fmt.Errorf("%w: srt config for %s", ErrClassNotV0, c)
	}
	prof, err := Profile(c)
	if err != nil {
		return SrtConfig{}, err
	}

	// Network: strict allow-list; deny-all when the class has no egress.
	net := SrtNetwork{
		AllowedDomains:  nonNil(egress.AllowHosts),
		DeniedDomains:   []string{},
		StrictAllowlist: true,
	}
	if prof.Network == NetNone || !prof.Network.needsEgressSubstrate() {
		net.DeniedDomains = []string{"*"} // hard block-all regardless of callback
	}

	// Filesystem read allow-set: the runtime trees, the engine prefix, the
	// workspace, platform config, sanctioned ro caches, and project mounts —
	// mirrors the native bwrap binds (sandbox.go Compose).
	allowRead := append([]string{}, runtimeBinds()...)
	if sp.EnginePrefix != "" {
		p, err := absClean(sp.EnginePrefix)
		if err != nil {
			return SrtConfig{}, err
		}
		allowRead = append(allowRead, p)
	}
	allowWrite := []string{tmpWritable}
	if sp.Workspace != "" {
		ws, err := absClean(sp.Workspace)
		if err != nil {
			return SrtConfig{}, err
		}
		allowRead = append(allowRead, ws)
		if prof.WorkspaceMode == "rw" { // C2 overlay upper = the reviewable diff
			allowWrite = append(allowWrite, ws)
		}
	}
	denyWrite := []string{} // config paths are read-only (P-T09-1)
	for _, p := range sp.ROConfig {
		cp, err := absClean(p)
		if err != nil {
			return SrtConfig{}, err
		}
		allowRead = append(allowRead, cp)
		denyWrite = append(denyWrite, cp)
	}
	for _, p := range sp.RWExchange { // the S03.4 gate ctl dir is writable
		xp, err := absClean(p)
		if err != nil {
			return SrtConfig{}, err
		}
		allowRead = append(allowRead, xp)
		allowWrite = append(allowWrite, xp)
	}
	for _, p := range prof.ROBinds {
		cp, err := absClean(p)
		if err != nil {
			return SrtConfig{}, err
		}
		allowRead = append(allowRead, cp)
	}
	for _, m := range prof.Mounts {
		host, err := absClean(m.Host)
		if err != nil {
			return SrtConfig{}, err
		}
		allowRead = append(allowRead, host)
		if m.Mode == "rw-no-delete" {
			allowWrite = append(allowWrite, host)
		}
	}

	// Credential stores are ALWAYS deny-read (D2/S11.6) — the one non-negotiable
	// denial; plus any explicitly-denied path.
	denyRead := []string{}
	for _, p := range prof.DenyRead {
		dp, err := absClean(p)
		if err != nil {
			return SrtConfig{}, err
		}
		denyRead = append(denyRead, dp)
		denyWrite = append(denyWrite, dp)
	}

	return SrtConfig{
		Network: net,
		Filesystem: SrtFilesystem{
			DenyRead:   denyRead,
			AllowRead:  allowRead,
			AllowWrite: allowWrite,
			DenyWrite:  denyWrite,
		},
	}, nil
}

// SrtInvocation builds the `srt` argv that wraps the engine. Shape:
//
//	srt --settings <configPath> -- <engine> <engine-args...>
//
// `--settings` is fail-closed in srt (an unloadable settings file makes srt
// refuse to run rather than silently drop restrictions). The `--` terminates
// srt's own option parsing so the engine's flags are never eaten by srt's
// commander parser (the robust shape; the exact need for `--` is a detail
// confirmed at the DEFERRED B2 live smoke). Argv[0] is resolved to an absolute
// host path (as the native path does) so it resolves inside srt's bwrap mount.
func SrtInvocation(configPath string, sp Spawn) ([]string, error) {
	if len(sp.Argv) == 0 {
		return nil, fmt.Errorf("sandbox: srt invocation without an engine argv")
	}
	cfg, err := absClean(configPath)
	if err != nil {
		return nil, err
	}
	bin, err := resolveBinary(sp.Argv[0])
	if err != nil {
		return nil, err
	}
	argv := []string{"--settings", cfg, "--", bin}
	argv = append(argv, sp.Argv[1:]...)
	return argv, nil
}

// Path names the active composition path (evidence + log key).
const (
	PathSrt    = "srt"    // adopted, ratified primary (S11.1 Adoption)
	PathNative = "native" // funeral-plan fallback (S16.3 direct composition)
)

// Plan is a resolved sandbox invocation: which path won, the executable and
// argv, any extra files the child inherits (the native seccomp pipe read-end
// on fd 3), and a cleanup to run after the process starts. Both callers — the
// dev in-process confiner (confiner.go) and the host `sinet run-launch`
// launcher (run_launch.go) — build one Plan so the srt-vs-native selection is
// identical on both paths (S11.1 "one composition core, two callers").
type Plan struct {
	Path       string     // PathSrt | PathNative
	Bin        string     // the executable to run (srt binary, or SystemBwrap)
	Argv       []string   // its arguments
	Env        []string   // environment for the WRAPPER process (see below)
	ExtraFiles []*os.File // inherited by the child (native: seccomp on fd 3)
	Cleanup    func()     // close the pipe / remove the srt config file
}

// The wrapper Env differs by path, deliberately:
//   - native: bwrap needs almost nothing (it is given an absolute path and
//     every sandbox var via --setenv), so a bare PATH keeps it robust; the
//     lowered engine env is injected INSIDE the sandbox by Compose.
//   - srt: srt runs host-side and FORWARDS its own environment to the wrapped
//     child (applying its credentials unset/set), so the lowered engine env
//     must BE the wrapper env. Exact srt env-pass-through semantics + srt's own
//     host needs (node on PATH) are confirmed at the DEFERRED B2 live smoke.
const nativeWrapperPATH = "PATH=/usr/bin:/bin"

// BuildPlan selects srt-when-present, native-fallback-when-absent, and
// resolves the full invocation. `dir` is a run-owned scratch directory for the
// srt config file (a fresh temp dir is used when empty); `egress` is the
// compiled egress policy for the class (the native path ignores it — it
// composes an empty netns and carries the allow-list as data until the host
// substrate lands; the srt path folds AllowHosts into srt's domain allow-list).
//
// The MANDATORY boundary (bwrap + functional userns) gates BOTH paths (srt
// itself rides bwrap): an absent boundary fails closed here, never a silent
// unconfined run (S16.3 OS-mechanism discipline).
func BuildPlan(caps Capabilities, c Class, sp Spawn, egress EgressPolicy, dir string) (*Plan, error) {
	if err := caps.require(); err != nil {
		return nil, err
	}
	if caps.Srt {
		return buildSrtPlan(caps, c, sp, egress, dir)
	}
	return buildNativePlan(caps, c, sp)
}

func buildSrtPlan(caps Capabilities, c Class, sp Spawn, egress EgressPolicy, dir string) (*Plan, error) {
	cfg, err := SrtConfigFor(c, sp, egress)
	if err != nil {
		return nil, err
	}
	blob, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("sandbox: marshal srt config: %w", err)
	}
	ownedDir := "" // a temp dir WE created gets removed wholesale on cleanup
	if dir == "" {
		dir, err = os.MkdirTemp("", "sinet-srt-")
		if err != nil {
			return nil, fmt.Errorf("sandbox: srt config dir: %w", err)
		}
		ownedDir = dir
	}
	cfgPath := filepath.Join(dir, "srt-config.json")
	if err := os.WriteFile(cfgPath, blob, 0o600); err != nil {
		if ownedDir != "" {
			_ = os.RemoveAll(ownedDir)
		}
		return nil, fmt.Errorf("sandbox: write srt config: %w", err)
	}
	argv, err := SrtInvocation(cfgPath, sp)
	if err != nil {
		if ownedDir != "" {
			_ = os.RemoveAll(ownedDir)
		} else {
			_ = os.Remove(cfgPath)
		}
		return nil, err
	}
	cleanup := func() { _ = os.Remove(cfgPath) }
	if ownedDir != "" {
		cleanup = func() { _ = os.RemoveAll(ownedDir) }
	}
	return &Plan{
		Path:    PathSrt,
		Bin:     caps.SrtPath,
		Argv:    argv,
		Env:     sp.Env, // srt forwards its env to the engine child
		Cleanup: cleanup,
	}, nil
}

func buildNativePlan(caps Capabilities, c Class, sp Spawn) (*Plan, error) {
	argv, seccomp, err := Compose(c, sp, caps)
	if err != nil {
		return nil, err
	}
	p := &Plan{Path: PathNative, Bin: SystemBwrap, Argv: argv, Env: []string{nativeWrapperPATH}, Cleanup: func() {}}
	if len(seccomp) > 0 {
		// Pass the compiled BPF on fd 3 (seccompFD) via a pipe — no disk
		// artifact. The child inherits its own copy at fork; a tiny program
		// never blocks the pipe. Cleanup closes the parent's read end.
		r, w, err := os.Pipe()
		if err != nil {
			return nil, fmt.Errorf("sandbox: seccomp pipe: %w", err)
		}
		p.ExtraFiles = []*os.File{r} // → child fd 3
		go func() {
			_, _ = w.Write(seccomp)
			_ = w.Close()
		}()
		p.Cleanup = func() { _ = r.Close() }
	}
	return p, nil
}

// nonNil returns a non-nil slice so JSON marshals `[]`, not `null` (srt's zod
// arrays reject null).
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

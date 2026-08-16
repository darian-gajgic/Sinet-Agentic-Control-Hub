// Package sandbox composes the per-run sandbox of Spec S11 — D2's
// load-bearing isolation boundary. It is the sandbox-side of the S01.3
// process seam: the control plane compiles a declarative Confinement record
// per run (S11.6: "all enforcement state lives exclusively in control-plane
// tables and is recompiled every run"; workers can never alter it, 14.2) and
// this package turns it into the concrete kernel-primitive stack of S11.1.
//
// Composed outward-to-inward (S11.1):
//
//	sinet-run@<id>.service   PID-1 time ceiling + cgroup accounting [S01; host, deferred]
//	  └─ bubblewrap          user/mount/PID/UTS/IPC + empty per-run netns; workspace + ro caches
//	       └─ seccomp-BPF     static profile (kills ptrace/process_vm_readv), loaded via bwrap --seccomp
//	            └─ Landlock   filesystem allowlist (ABI 8) — composed as data; enforcement seam (landlock.go)
//	                 └─ engine  claude -p / opencode — unprivileged, NoNewPrivileges
//
// The boundary is the namespace isolation plus the empty-netns network
// policy (S11.4); seccomp and Landlock are defense-in-depth, not the
// boundary (S11.1). The whole stack runs UNPRIVILEGED: bwrap under the
// shipped system AppArmor profile, an empty netns, seccomp and Landlock all
// compose without privilege (S11.8) — verified on the v0 host (S11.2).
//
// Adopt-don't-fork (S16.1): the engines are configured, never modified. srt
// (Anthropic's Apache-2.0 sandbox-runtime) is ADOPTED as the ratified PRIMARY
// composition path for this core [S11.1 "Adoption"; G2 D2.2; S16.3
// sandbox-runtime row] — it wraps the engine EXTERNALLY (see srt.go), supplying
// bwrap+seccomp + the S11.4 netns-removed UDS-to-host-proxy egress shape + a
// Node-safe positive syscall allow-list. This native direct composition
// (Compose → bwrap argv) is RETAINED but relabeled to its correct spec role:
// the S16.3 funeral-plan / FALLBACK — the ratified "direct composition of the
// same kernel primitives" replacement path — selected only when srt is absent.
// srt is a host-deferred install (its activation batches at the B2 gate with
// the egress substrate + run@ install, S16.3), so at B1 the native fallback
// normally runs: dev-mode confinement keeps working, no regression. The
// confiner (confiner.go) / launcher (run_launch.go) select srt-when-present via
// BuildPlan and log which path is active. srt carries a `library`
// components.lock entry (a Node CLI — no `modules` claim); the OS-mechanism
// entry (bubblewrap/seccomp/Landlock/netns) also does, since both paths ride
// those kernel primitives.
package sandbox

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SystemBwrap is the bubblewrap binary. It MUST be the system path: the
// shipped AppArmor profile /etc/apparmor.d/bwrap-userns-restrict attaches to
// exactly this path and grants the unprivileged userns; a vendored or
// renamed copy would be blocked (S11.2 binding rider).
const SystemBwrap = "/usr/bin/bwrap"

// Class is a confinement rung of the S5 isolation ladder (Spec S11.6). v0
// ships C0–C2; C3 is v0.1 and C4 future — both are recognized here so the
// ladder is fixed and the tightening comparison (P-T05-1) is total, but
// composing them is refused (ErrClassNotV0).
type Class string

const (
	// C0 connectors: no model in loop; host-side deterministic code; one
	// named service host + IP deny-CIDR; one narrowest-scope broker-held
	// key. Filesystem: none.
	C0 Class = "C0"
	// C1 trusted reasoning: model in loop; ro workspace clone; empty netns,
	// no route, no proxy; no credentials. The strongest and simplest class.
	C1 Class = "C1"
	// C2 workspace-write: model in loop; rw overlay workspace + ro caches;
	// registries allowlist via the host proxy; no credentials in sandbox.
	C2 Class = "C2"
	// C3 web-reading (v0.1) — fetch-broker host only; gVisor step-up. Seam.
	C3 Class = "C3"
	// C4 web-acting (future, 12.2). Seam.
	C4 Class = "C4"
)

// ladderRank is the S5 isolation-ladder position (S11.6 table order). Higher
// rank = more capability = looser. The P-T05-1 tightening check compares
// ranks; the cross-axis guard in admit.go covers the C0 corner (C0 carries a
// scoped credential + single-host egress that the pure rank does not model).
func (c Class) ladderRank() (int, bool) {
	switch c {
	case C0:
		return 0, true
	case C1:
		return 1, true
	case C2:
		return 2, true
	case C3:
		return 3, true
	case C4:
		return 4, true
	default:
		return 0, false
	}
}

// isV0 reports whether the class ships at v0 (C0–C2 — S11.6; 15.1).
func (c Class) isV0() bool { return c == C0 || c == C1 || c == C2 }

// LadderRank exposes the S5 ladder position for the S08.8 selection
// comparison ("confinement compatibility with the plan's declared class —
// equal or tighter only", whose mechanics S11 owns). False = unknown class
// (never passes a comparison).
func (c Class) LadderRank() (int, bool) { return c.ladderRank() }

// WorkspaceWritable reports whether this class mounts the workspace
// read-write — the one class fact the S08.8 selection needs that the ladder
// RANK cannot answer, because the ladder orders by tightness and the classes
// are not totally ordered by capability (C0 is rank 0 and has no filesystem at
// all, while C1 at rank 1 has a read-only workspace). Derived from the class
// profile, never a second hardcoding of the same table: whatever Profile says
// the workspace mode is, this answers.
//
// The S08.8 consumer: a plan that declares writes may not be given to a worker
// whose granted class cannot write (P3-RW-14 R8).
func (c Class) WorkspaceWritable() bool {
	p, err := Profile(c)
	if err != nil {
		return false // an unknown class never passes a capability question
	}
	return p.WorkspaceMode == "rw"
}

// NetMode is the per-class egress posture (Spec S11.6 network column).
type NetMode string

const (
	// NetNone: empty netns, no route, no proxy (C1). Composable unprivileged
	// and demonstrated live — the S11.4 primary default-drop.
	NetNone NetMode = "none"
	// NetSingleHost: one named service host + IP deny-CIDR guard (C0).
	NetSingleHost NetMode = "single-host"
	// NetRegistries: registries allowlist (+ CDN hosts) via the host proxy
	// behind the nftables default-drop (C2).
	NetRegistries NetMode = "registries"
	// NetFetchBroker: fetch-broker host only (C3, v0.1). Seam.
	NetFetchBroker NetMode = "fetch-broker"
)

// needsEgressSubstrate reports whether a net mode reaches out at all — i.e.
// whether it depends on the host egress substrate (nftables default-DROP +
// host proxy, installed once at boot by the privileged sinet-egress-setup
// unit, S11.4/S11.8). That substrate is a host change and is DEFERRED at B1;
// until it lands, egress-needing classes compose with an empty netns (no
// route) and carry their allow-list as data (see Confinement.RequiresEgress).
func (n NetMode) needsEgressSubstrate() bool {
	return n == NetSingleHost || n == NetRegistries || n == NetFetchBroker
}

// Mount is a project-shared resource mounted into the sandbox per the
// worker's declarative mounts list (Spec S11.3; 4.1 sanctioned sharing).
type Mount struct {
	Host  string // host path
	Guest string // path inside the sandbox (empty = same as Host)
	Mode  string // "ro" | "rw-no-delete"
}

// Confinement is the compiled per-run confinement record (Spec S11.6): the
// declarative enforcement state — class, filesystem, network, credentials,
// Rule-of-Two properties — recompiled every run and carried in control-plane
// data, never derived from anything the engine remembers (S11.7 mitigation).
// It holds ONLY auth-profile references, never secrets (S11.5, S11.6).
type Confinement struct {
	Class Class `json:"class"`

	// WorkspaceMode is "ro" (C1) or "rw" (C2); "" for C0 (filesystem none).
	// Derived from the class profile; carried explicitly so the record is
	// self-describing (guardrail split, 14.2).
	WorkspaceMode string `json:"workspace_mode,omitempty"`

	// ROBinds are the sanctioned read-only shares (package/model caches,
	// S11.3): bound read-only at the same path. A writable cross-run cache is
	// the poisoning anti-pattern (S11.3) and is never emitted here.
	ROBinds []string `json:"ro_binds,omitempty"`
	// Mounts are project-shared resources (ro | rw-no-delete), S11.3.
	Mounts []Mount `json:"mounts,omitempty"`
	// DenyRead names paths that MUST be structurally unreachable. Credential
	// stores are always denied (S11.6). With empty-env deny-by-default
	// (S11.1) nothing is bound unless listed, so denial is by construction;
	// these are masked belt-and-suspenders (tmpfs over the path).
	DenyRead []string `json:"deny_read,omitempty"`

	// Network is the egress posture (S11.6). AllowHosts is the hostname
	// allow-list carried as data for the egress substrate (S11.4); it is
	// enforcement input for the host proxy, never a boundary by itself
	// (S11.4 "the proxy is convenience").
	Network    NetMode  `json:"network"`
	AllowHosts []string `json:"allow_hosts,omitempty"`

	// AuthProfiles are the auth-profile references this run may resolve via
	// the broker (S11.5/S11.6) — NAMES, never secrets. The credential itself
	// never enters the sandbox (D2): the engine's own model credential rides
	// the S11.5 injection proxy (a sentinel is all the sandbox holds).
	AuthProfiles []string `json:"auth_profiles,omitempty"`

	// Props are the Rule-of-Two properties this run's record asserts, and
	// Supervised marks a supervision gate (human-in-loop / reliable
	// validation) that permits holding all three (S11.6 admission check).
	Props      Props `json:"props"`
	Supervised bool  `json:"supervised,omitempty"`
}

// RequiresEgress reports whether the class reaches out and therefore depends
// on the host egress substrate (deferred at B1). Composing such a class in
// dev mode yields an empty netns (no route) regardless — safe, but the
// allow-listed egress is inert until the substrate lands (S11.4/S11.8).
func (c Confinement) RequiresEgress() bool { return c.Network.needsEgressSubstrate() }

// Profile returns the structural per-class default Confinement (Spec S11.6
// "Per-class concrete profiles" table). These defaults are STRUCTURAL, not ⚙
// — versioned in code, not operator dials (S11 settings note). The caller
// overlays concrete per-run data (ROBinds, Mounts, AuthProfiles, Props).
func Profile(c Class) (Confinement, error) {
	switch c {
	case C0:
		return Confinement{Class: C0, WorkspaceMode: "", Network: NetSingleHost}, nil
	case C1:
		return Confinement{Class: C1, WorkspaceMode: "ro", Network: NetNone}, nil
	case C2:
		return Confinement{Class: C2, WorkspaceMode: "rw", Network: NetRegistries}, nil
	case C3, C4:
		return Confinement{}, fmt.Errorf("%w: %s ships post-v0 (S11.6; 15.1)", ErrClassNotV0, c)
	default:
		return Confinement{}, fmt.Errorf("%w: %q", ErrUnknownClass, c)
	}
}

// Spawn is one concrete confined engine invocation: the engine argv/env the
// adapter lowered (empty-env base; the sandbox holds a sentinel, never the
// real model credential — D2/S11.5), plus the platform-owned paths that must
// be visible inside the sandbox.
type Spawn struct {
	// Argv is the engine command; Argv[0] is resolved to an absolute host
	// path by the composer (bwrap needs the binary present under a bound
	// tree). Env is the lowered engine environment.
	Argv []string
	Env  []string

	// Workspace is the per-run workspace host path. It is bound at the SAME
	// path inside the sandbox (ro for C1, rw for C2) and is the chdir target,
	// so the engine's cwd is identical confined or not (S11.3 overlay is
	// mounted here; the mount itself is S02's — this composes onto it).
	Workspace string

	// ROConfig are platform-owned config paths bound READ-ONLY at the same
	// path (the lowered settings.json dir): the engine reads its compiled
	// config but can never rewrite it — the S11.7 P-T09-1 deny-writes-to-
	// config mitigation, symlink-resolved by bwrap's bind resolution.
	ROConfig []string
	// RWExchange are platform-owned exchange dirs bound read-write at the
	// same path (the S03.4 gate control dir: the in-sandbox hook appends
	// fires.log, the control plane reads it). This is the platform's exchange
	// channel, NOT engine config — distinct from ROConfig.
	RWExchange []string

	// EnginePrefix, when set, is an extra host directory tree bound read-only
	// so a real engine installed outside /usr (e.g. an npm-global claude) is
	// reachable inside the sandbox. Empty for system-path probes.
	EnginePrefix string

	// Bridge, when set, is the S12.6 GPU-broker sandboxed-plane injection
	// (B4-6/OQ3): a per-sandbox UDS the granted worker reaches `/v1` through.
	// The socket is bound read-write and the worker's OpenAI env is set to point
	// at it — never /dev/nvidia*, never a routable host address (S12.6). C0
	// connectors never get it (Compose fails closed on a C0 bridge). Nil = no
	// bridge (the v0 default — no class-(a) consumer, dormant seam).
	Bridge *Bridge
}

// Bridge is the S12.6 GPU-broker sandboxed-plane injection payload (B4-6/OQ3).
// The GPU broker (internal/local) mints the token + allowlist; the confinement
// class decides whether it is granted and its budgets (BridgeBudgetFor); the
// sandbox runtime binds SocketPath and sets EnvName=BaseURL (+ the token env) so
// the worker's `/v1` points at the per-sandbox bridge.
type Bridge struct {
	SocketPath   string
	EnvName      string
	BaseURL      string
	TokenEnvName string
	Token        string
}

// BridgeBudgetFor is the S11.6/S12.6 class→bridge policy (B4-6/R6): whether the
// GPU bridge is granted to a class and its class-derived rate/size budgets. C0
// connectors NEVER get it; C1/C2 get standard budgets; C3 web-reading (v0.1)
// gets TIGHTER budgets (hostile-input step-up). A class comparison over the
// exported ladder constants — this is the ladder's home package, so the policy
// is authoritative here, never re-derived across the import wall. The mint site
// (the future class-(a) consumer) calls this before minting a grant. Rate/size
// values are structural constants (the sseBatchSize precedent; S18 ratifies no
// key — a per-run budget is not operator-tuned ⚙).
func BridgeBudgetFor(c Class) (granted bool, ratePerMin int, maxBytes int64) {
	switch c {
	case C3:
		return true, 30, 256 << 10 // C3: tighter (v0.1 web-reading step-up)
	case C1, C2:
		return true, 120, 1 << 20 // C1/C2: standard
	default:
		return false, 0, 0 // C0 (and any non-v0/unknown class): never granted
	}
}

// Compose turns a class + spawn into the exact bwrap argv and the seccomp
// BPF program bytes (loaded via bwrap --seccomp <fd>). It composes but never
// runs — the runner (confiner.go dev path, or `sinet run-launch` host path)
// executes the returned argv. Composition is the S11 admission rule made
// concrete: no engine reaches real work except through this.
//
// caps gates enforcement: an absent mechanism fails closed (probe-at-compose,
// S16.3 OS-mechanism discipline) unless the caller sanctioned a skip.
func Compose(c Class, sp Spawn, caps Capabilities) (argv []string, seccomp []byte, err error) {
	if !c.isV0() {
		return nil, nil, fmt.Errorf("%w: %s", ErrClassNotV0, c)
	}
	if len(sp.Argv) == 0 {
		return nil, nil, fmt.Errorf("sandbox: compose C%s without an engine argv", c)
	}
	if err := caps.require(); err != nil {
		return nil, nil, err
	}
	prof, err := Profile(c)
	if err != nil {
		return nil, nil, err
	}

	bin, err := resolveBinary(sp.Argv[0])
	if err != nil {
		return nil, nil, err
	}

	a := []string{
		// Namespaces: user/mount/PID/UTS/IPC/cgroup + empty netns (S11.1).
		// --unshare-all includes --unshare-net: the empty netns with no route
		// is the S11.4 primary default-drop, composable unprivileged.
		"--unshare-all",
		// Empty-env, deny-by-default — not scrub (S11.1): start from nothing,
		// set back only the lowered vars the engine needs.
		"--clearenv",
		"--die-with-parent", // no orphaned engine outlives its run unit
		"--new-session",     // detach the controlling tty (no TIOCSTI injection)
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
	}
	// Minimal read-only runtime (the engine + its interpreter live here).
	for _, base := range runtimeBinds() {
		a = append(a, "--ro-bind-try", base, base)
	}
	if sp.EnginePrefix != "" {
		a = append(a, "--ro-bind", sp.EnginePrefix, sp.EnginePrefix)
	}
	// Empty-env restore: only the allowed variables (S11.1). Values are the
	// lowered env — a sentinel on the credential channel, never the real
	// model token (D2/S11.5).
	for _, kv := range sp.Env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			continue
		}
		a = append(a, "--setenv", k, v)
	}
	// Workspace (S11.3): ro clone for C1, rw overlay for C2, at the same path.
	if sp.Workspace != "" {
		ws, err := absClean(sp.Workspace)
		if err != nil {
			return nil, nil, err
		}
		switch prof.WorkspaceMode {
		case "ro":
			a = append(a, "--ro-bind", ws, ws)
		case "rw":
			a = append(a, "--bind", ws, ws)
		default:
			return nil, nil, fmt.Errorf("sandbox: C%s workspace with mode %q", c, prof.WorkspaceMode)
		}
		a = append(a, "--chdir", ws)
	}
	// Platform-owned config: read-only (P-T09-1 deny-writes-to-config, S11.7).
	for _, p := range sp.ROConfig {
		cp, err := absClean(p)
		if err != nil {
			return nil, nil, err
		}
		a = append(a, "--ro-bind", cp, cp)
	}
	// Platform-owned exchange channel: read-write (S03.4 gate ctl dir).
	for _, p := range sp.RWExchange {
		xp, err := absClean(p)
		if err != nil {
			return nil, nil, err
		}
		a = append(a, "--bind", xp, xp)
	}
	// The S12.6 GPU-broker bridge (B4-6/OQ3): bind the per-sandbox UDS read-write
	// and set the worker's `/v1` env to point at it. C0 connectors NEVER get the
	// bridge — a C0 spawn carrying one FAILS CLOSED (the sandbox is the boundary;
	// the mint also refuses C0, so this is defense-in-depth, S12.6/S11.6).
	if sp.Bridge != nil && sp.Bridge.SocketPath != "" {
		if c == C0 {
			return nil, nil, fmt.Errorf("sandbox: a C0 connector must never be granted the GPU bridge (S12.6/S11.6)")
		}
		bp, err := absClean(sp.Bridge.SocketPath)
		if err != nil {
			return nil, nil, err
		}
		a = append(a, "--bind", bp, bp)
		if sp.Bridge.EnvName != "" {
			a = append(a, "--setenv", sp.Bridge.EnvName, sp.Bridge.BaseURL)
		}
		if sp.Bridge.TokenEnvName != "" {
			a = append(a, "--setenv", sp.Bridge.TokenEnvName, sp.Bridge.Token)
		}
	}
	// Sanctioned read-only caches (S11.3). A writable cross-run cache is the
	// poisoning anti-pattern and is never emitted (rw-no-delete mounts below
	// are the only writable shares, and only when a project declares them).
	for _, p := range prof.ROBinds {
		cp, err := absClean(p)
		if err != nil {
			return nil, nil, err
		}
		a = append(a, "--ro-bind", cp, cp)
	}
	for _, m := range prof.Mounts {
		host, err := absClean(m.Host)
		if err != nil {
			return nil, nil, err
		}
		guest := host
		if m.Guest != "" {
			if guest, err = absClean(m.Guest); err != nil {
				return nil, nil, err
			}
		}
		switch m.Mode {
		case "ro":
			a = append(a, "--ro-bind", host, guest)
		case "rw-no-delete":
			// rw-no-delete has no bwrap primitive; v0 binds rw and the
			// no-delete guarantee is a workspace-GC concern (S02). Recorded
			// as bound rw here; the delete-guard is [XREF:S02].
			a = append(a, "--bind", host, guest)
		default:
			return nil, nil, fmt.Errorf("sandbox: mount %q with unknown mode %q", host, m.Mode)
		}
	}
	// Structural deny (S11.6): mask any explicitly-denied path with an empty
	// tmpfs. Credential stores are already unreachable by construction
	// (never bound), so this is defense-in-depth for paths that a bind above
	// might otherwise expose a sensitive sibling of.
	for _, p := range prof.DenyRead {
		dp, err := absClean(p)
		if err != nil {
			return nil, nil, err
		}
		a = append(a, "--tmpfs", dp)
	}

	// seccomp (S11.1): the static profile, loaded from fd 3 (the composer/
	// runner passes it as the first ExtraFile). Absent kernel support is a
	// sanctioned skip (caps.Seccomp false ⇒ no --seccomp; the namespace
	// boundary stands regardless — seccomp is defense-in-depth).
	if caps.Seccomp {
		seccomp, err = seccompProfile(c)
		if err != nil {
			return nil, nil, err
		}
		a = append(a, "--seccomp", fmt.Sprintf("%d", seccompFD))
	}

	a = append(a, "--", bin)
	a = append(a, sp.Argv[1:]...)
	return a, seccomp, nil
}

// seccompFD is the child fd number the seccomp BPF program is passed on
// (bwrap --seccomp <fd>); the runner supplies it as ExtraFiles[0], which the
// Go runtime maps to fd 3.
const seccompFD = 3

// runtimeBinds is the minimal read-only host tree the engine needs. Bound
// with --ro-bind-try so a layout without a separate /lib64 (merged-usr) does
// not fail composition.
func runtimeBinds() []string {
	return []string{"/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc/alternatives", "/etc/ssl/certs"}
}

// resolveBinary returns an absolute host path for the engine binary. bwrap
// execs inside the mount namespace, so a bare name is resolved against the
// host PATH here (the resolved path must fall under a bound tree).
func resolveBinary(name string) (string, error) {
	if strings.Contains(name, "/") {
		return absClean(name)
	}
	p, err := lookPath(name)
	if err != nil {
		return "", fmt.Errorf("sandbox: resolve engine binary %q: %w", name, err)
	}
	return p, nil
}

func absClean(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("sandbox: empty path in composition")
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("sandbox: path %q is not absolute (composition needs host-absolute paths)", p)
	}
	return filepath.Clean(p), nil
}

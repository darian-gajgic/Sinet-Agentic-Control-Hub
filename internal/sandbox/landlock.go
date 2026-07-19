package sandbox

import "errors"

// landlock.go composes the Landlock filesystem-allowlist ruleset of the
// S11.1 stack (ABI 8 on the v0 host, S11.2). At B1 it is composed as DATA and
// enforcement is a SEAM: golang.org/x/sys/unix v0.47.0 (already built) exposes
// the seccomp return actions but NO Landlock wrappers, so live enforcement
// needs either raw syscalls in an in-sandbox helper or a Landlock library
// adoption. Landlock is defense-in-depth, not the boundary (S11.1) — the
// boundary (bwrap namespaces + empty netns) is delivered and demonstrated
// live; the Landlock ruleset is built and unit-tested here so the enforcement
// path drops in behind this seam without re-deriving the policy.
//
// Enforcement placement (when it lands): Landlock is applied by a process
// INSIDE the bwrap sandbox, after the mounts, which then execs the engine
// (Landlock is inherited across exec). It cannot be applied by bwrap itself.

// ErrLandlockSeam marks the deferred live-enforcement path (see file comment).
var ErrLandlockSeam = errors.New("sandbox: Landlock enforcement is a B1 seam (x/sys exposes no Landlock wrappers)")

// LandlockAccess is a coarse access class for a ruleset path.
type LandlockAccess int

const (
	AccessRO LandlockAccess = iota // read + execute
	AccessRW                       // read + write + execute
)

// LandlockRule is one path rule.
type LandlockRule struct {
	Path   string
	Access LandlockAccess
}

// LandlockRuleset is the composed filesystem allowlist for one run.
type LandlockRuleset struct {
	ABI   int
	Rules []LandlockRule
}

// LandlockFor composes the ruleset for a class + spawn. Binding S11.3 gotcha
// (b): Landlock rules do NOT propagate through overlay layers, so every rule
// targets the MOUNTED path (sp.Workspace, bound at the same path inside), not
// any lower/upper dir. The allowlist mirrors the bwrap binds: the runtime
// trees read-only, the workspace ro (C1) or rw (C2), platform config
// read-only, the exchange dir read-write.
func LandlockFor(c Class, sp Spawn, abi int) LandlockRuleset {
	rs := LandlockRuleset{ABI: abi}
	for _, base := range runtimeBinds() {
		rs.Rules = append(rs.Rules, LandlockRule{Path: base, Access: AccessRO})
	}
	if sp.EnginePrefix != "" {
		rs.Rules = append(rs.Rules, LandlockRule{Path: sp.EnginePrefix, Access: AccessRO})
	}
	if sp.Workspace != "" {
		acc := AccessRO
		if prof, err := Profile(c); err == nil && prof.WorkspaceMode == "rw" {
			acc = AccessRW
		}
		// Target the MOUNTED overlay path (S11.3 gotcha b).
		rs.Rules = append(rs.Rules, LandlockRule{Path: sp.Workspace, Access: acc})
	}
	for _, p := range sp.ROConfig {
		rs.Rules = append(rs.Rules, LandlockRule{Path: p, Access: AccessRO})
	}
	for _, p := range sp.RWExchange {
		rs.Rules = append(rs.Rules, LandlockRule{Path: p, Access: AccessRW})
	}
	rs.Rules = append(rs.Rules, LandlockRule{Path: "/tmp", Access: AccessRW})
	return rs
}

// Enforce is the enforcement seam. At B1 it reports ErrLandlockSeam so a
// caller that requires enforced Landlock fails closed rather than silently
// running without it; the composer treats an unsupported/seam Landlock as a
// sanctioned skip of the defense-in-depth layer (the boundary stands).
func (rs LandlockRuleset) Enforce() error { return ErrLandlockSeam }

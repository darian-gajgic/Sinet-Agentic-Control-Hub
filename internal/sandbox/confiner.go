package sandbox

import (
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
)

// confiner.go is the dev-mode realization of adapters.Confiner (the B1-1
// spawn seam, CONVENTIONS §10 "the sandbox/broker wrap arrives at B1-3"). The
// control plane wires a *Composer onto a StartRequest; the adapter builds its
// engine *exec.Cmd through Confine, so the engine runs inside the composed
// per-run sandbox instead of unconfined. The same composition powers the host
// path (`sinet run-launch`, run_launch.go) — one composition core, two
// callers. No adapter imports this package: sandbox → adapters is the only
// direction (adapters owns the seam type), so there is no import cycle.

// Settings is the narrow ⚙ reader for the four S18 S11-domain keys
// (CONVENTIONS §2: consumers read the registry by dotted key). *settings.
// Registry satisfies it. Every number/list flows from the registry — nothing
// here is a hardcoded constant (S01.10 discipline; the per-class profiles and
// the seccomp/Landlock rulesets are STRUCTURAL, not ⚙, per the S11 settings
// note, and live in sandbox.go/seccomp.go/landlock.go).
type Settings interface {
	Bool(key string) (bool, error)
	Strings(key string) ([]string, error)
}

// The four ratified S18 keys of the S11 domain.
const (
	keyEgressDenyCIDRs = "sandbox.egress_deny_cidrs"
	keyBlockDoH        = "sandbox.block_outbound_doh"
	keyC2Allowlist     = "sandbox.c2_registry_allowlist"
	keyModelTLSTerm    = "sandbox.model_egress_tls_terminate"
)

// EgressPolicy is the compiled egress data an egress-needing class carries to
// the host substrate (S11.4: nftables default-DROP + host proxy) — read from
// the ⚙ registry, never hardcoded. At B1 the substrate is a deferred host
// change; the policy is compiled (consumed) here and enforced when the
// substrate lands (RequiresSubstrate marks that dependency).
type EgressPolicy struct {
	Mode              NetMode
	AllowHosts        []string // single-host (C0) / registries + CDN allow-list (C2)
	DenyCIDRs         []string // ⚙ sandbox.egress_deny_cidrs (metadata IP is a floor)
	BlockDoH          bool     // ⚙ sandbox.block_outbound_doh
	TLSTerminate      bool     // ⚙ sandbox.model_egress_tls_terminate (injection proxy, S11.5)
	RequiresSubstrate bool     // true ⇒ needs the deferred host egress substrate
}

// Composer composes and runs per-run sandboxes. It probes the host ONCE at
// construction (re-probe is a deliberate OS-upgrade event, S16.3), so a spawn
// never re-runs the functional userns test.
type Composer struct {
	caps     Capabilities
	settings Settings
	log      *slog.Logger
}

var _ adapters.Confiner = (*Composer)(nil)

// NewComposer probes the host and returns a Composer. A nil settings reader
// is rejected lazily (only egress-class composition reads ⚙).
func NewComposer(settings Settings, log *slog.Logger) *Composer {
	if log == nil {
		log = slog.Default()
	}
	return &Composer{caps: Probe(), settings: settings, log: log}
}

// Caps returns the probe result (evidence, and the Available() gate tests use
// to choose between a live composition and a sanctioned skip).
func (co *Composer) Caps() Capabilities { return co.caps }

// EgressPolicyFor reads the four ⚙ keys and compiles the egress data for a
// class. singleHost is the C0 named service host(s) from the worker record.
func (co *Composer) EgressPolicyFor(c Class, singleHost []string) (EgressPolicy, error) {
	if co.settings == nil {
		return EgressPolicy{}, fmt.Errorf("sandbox: egress policy needs a settings registry (⚙ discipline, S01.10)")
	}
	prof, err := Profile(c)
	if err != nil {
		return EgressPolicy{}, err
	}
	denyCIDRs, err := co.settings.Strings(keyEgressDenyCIDRs)
	if err != nil {
		return EgressPolicy{}, fmt.Errorf("sandbox: read ⚙ %s: %w", keyEgressDenyCIDRs, err)
	}
	blockDoH, err := co.settings.Bool(keyBlockDoH)
	if err != nil {
		return EgressPolicy{}, fmt.Errorf("sandbox: read ⚙ %s: %w", keyBlockDoH, err)
	}
	tlsTerm, err := co.settings.Bool(keyModelTLSTerm)
	if err != nil {
		return EgressPolicy{}, fmt.Errorf("sandbox: read ⚙ %s: %w", keyModelTLSTerm, err)
	}
	p := EgressPolicy{
		Mode: prof.Network, DenyCIDRs: denyCIDRs, BlockDoH: blockDoH,
		TLSTerminate: tlsTerm, RequiresSubstrate: prof.Network.needsEgressSubstrate(),
	}
	switch prof.Network {
	case NetSingleHost:
		p.AllowHosts = singleHost
	case NetRegistries:
		allow, err := co.settings.Strings(keyC2Allowlist)
		if err != nil {
			return EgressPolicy{}, fmt.Errorf("sandbox: read ⚙ %s: %w", keyC2Allowlist, err)
		}
		p.AllowHosts = allow // empty ⇒ preset-not-installed (S18 help text)
	}
	return p, nil
}

// Confine implements adapters.Confiner: it composes the sandbox for the run's
// class around the engine's lowered spawn and returns the *exec.Cmd to start
// plus a cleanup. It selects the ADOPTED srt path when the host has srt (the
// ratified S11.1 primary — srt wraps the engine externally, adopt-don't-fork)
// and falls back to the native direct composition (the S16.3 funeral plan)
// when srt is absent, LOGGING which path is active. srt is a host-deferred
// install (batches at the B2 gate, S16.3), so at B1 the native fallback
// normally runs — dev-mode confinement keeps working, no regression.
//
// Either way the engine sees only a sentinel on the credential channel, never
// the real model token (D2/S11.5 — the real token rides the injection proxy):
// on the native path the lowered env is injected inside the sandbox via
// --clearenv/--setenv (empty-env deny-by-default, S11.1); on the srt path srt
// forwards the lowered env to the engine child. Class comes from the run's
// compiled confinement (req.Class); dev-mode default is C1, the strongest and
// simplest class.
func (co *Composer) Confine(req adapters.StartRequest, spec adapters.SpawnSpec) (*exec.Cmd, func(), error) {
	class := Class(req.Class)
	if class == "" {
		class = C1
	}
	sp := Spawn{
		Argv:         spec.Argv,
		Env:          spec.Env,
		Workspace:    spec.Workspace,
		ROConfig:     spec.ROConfig,
		RWExchange:   spec.RWExchange,
		EnginePrefix: spec.EnginePrefix,
	}
	egress, err := co.egressForClass(class)
	if err != nil {
		return nil, nil, err
	}
	// req.WorkDir (platform-owned scratch, S03.5) holds the srt config file when
	// the srt path is active; empty falls back to a fresh temp dir in BuildPlan.
	plan, err := BuildPlan(co.caps, class, sp, egress, req.WorkDir)
	if err != nil {
		return nil, nil, err // fail closed: the boundary cannot be composed (S16.3)
	}
	co.log.Info("per-run sandbox composed",
		"run_id", req.RunID, "class", string(class),
		"path", plan.Path, "srt_path", co.caps.SrtPath)

	cmd := exec.Command(plan.Bin, plan.Argv...)
	cmd.Env = plan.Env
	cmd.ExtraFiles = plan.ExtraFiles
	return cmd, plan.Cleanup, nil
}

// egressForClass compiles the egress policy that feeds srt's domain allow-list
// (the S11.4 convenience layer). C1 has no egress and needs no ⚙ read; egress
// classes read the four ⚙ keys via EgressPolicyFor. The C0 single-host list is
// not carried on the B1 confiner seam (it arrives with the worker record when
// that seam widens), so C0's srt allow-list is empty here — inert anyway until
// the deferred host egress substrate lands (S11.4/S11.8), and fully covered by
// the pure SrtConfigFor unit test.
func (co *Composer) egressForClass(c Class) (EgressPolicy, error) {
	prof, err := Profile(c)
	if err != nil {
		return EgressPolicy{}, err
	}
	if !prof.Network.needsEgressSubstrate() {
		return EgressPolicy{Mode: prof.Network}, nil // C1: deny-all, no ⚙ read
	}
	return co.EgressPolicyFor(c, nil)
}

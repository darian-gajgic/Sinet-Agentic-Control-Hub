package local

import (
	"os"
	"os/exec"
)

// conformance.go — structural-config passthrough + installed-stack detection
// for the §10 conformance tiers (brief R26/R28). The pins (LlamaSwapPin,
// LlamaCppPin in local.go) are exported and test-coupled to the components.lock
// entries in conformance_test.go (the §10 Pin↔lock precedent); an installed-
// vs-pin delta is reported LOUDLY, never retargeted.

// StackConfig is the structural deployment config for the local serving stack
// (R28: NO ⚙ key — the SINET_SRT_PATH/B4-4 precedent; composition-root
// passthrough, gate-flagged under the settings-tab directive). The dev default
// is UNCONFIGURED (Endpoint == "" ⇒ the stack is absent and the duty seams
// degrade per each S12.4/S06 row, R17).
type StackConfig struct {
	// Endpoint is the llama-swap /v1 base (loopback). "" ⇒ stack absent.
	Endpoint string
	// LlamaServer is the llama-server backend binary path (config-gen R10).
	LlamaServer string
	// ModelCache is the shared read-only model-cache root (S12.1 feature 4.1;
	// operator-relocatable, gate-ratified — operator item e).
	ModelCache string
	// GameModeBus is the operator session-bus address the GameMode D-Bus
	// subscription monitors (R14; "" ⇒ deferred-with-finding).
	GameModeBus string
}

// StackFromEnv reads the SINET_LOCAL_* structural config (bring-up
// passthrough; the SINET_SRT_PATH/SINET_PREVIEW_* precedent).
func StackFromEnv() StackConfig {
	return StackConfig{
		Endpoint:    os.Getenv("SINET_LOCAL_ENDPOINT"),
		LlamaServer: os.Getenv("SINET_LOCAL_LLAMA_SERVER"),
		ModelCache:  os.Getenv("SINET_LOCAL_MODEL_CACHE"),
		GameModeBus: os.Getenv("SINET_LOCAL_GAMEMODE_BUS"),
	}
}

// Configured reports whether the local stack is configured (Endpoint set) —
// the shell wires the duty seams only then (R20/R22).
func (c StackConfig) Configured() bool { return c.Endpoint != "" }

// InstalledStack reports the installed llama-swap + llama-server binaries for
// tier-R auto-run (PATH + the SINET_LOCAL_LLAMA_* overrides). ok=false ⇒ the
// real stack is absent and tier R/L SANCTIONED-SKIP (the one allowed skip
// class, CONVENTIONS §10). This detects binaries only — a running endpoint is
// the tier-L concern.
func InstalledStack() (llamaSwap, llamaServer string, ok bool) {
	llamaSwap = lookBinary("SINET_LOCAL_LLAMA_SWAP", "llama-swap")
	llamaServer = lookBinary("SINET_LOCAL_LLAMA_SERVER", "llama-server")
	return llamaSwap, llamaServer, llamaSwap != "" && llamaServer != ""
}

func lookBinary(envOverride, name string) string {
	if p := os.Getenv(envOverride); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

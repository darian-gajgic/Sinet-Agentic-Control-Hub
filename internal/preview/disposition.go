package preview

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// disposition.go resolves a deliverable revision to its preview LANE and (for
// runner lanes) the runner invocation (Spec S13.8: per-type dispositions + the
// heuristic host-toolchain detection). Detection is pure over the checkout tree
// + the deliverable type + the owner-approved registry preview command, so it
// is deterministic and fixture-testable (R2/R4). The runner TOOLS (pnpm/npm/uv/
// mise/ttyd) are detected per-project and executed inside the sandbox — they
// are NOT adoptions: an absent tool is the honest no-preview state at launch,
// never an install (R2; ttyd is the one adopted os-mechanism, R21).

// ToolLookup resolves a runner/ttyd tool to its host path — the discovery seam
// (default: PATH, plus SINET_TTYD_PATH for ttyd, the srt injection precedent).
// Tests inject a stub lookup so no real toolchain is required (R2/R20).
type ToolLookup func(name string) (path string, ok bool)

// EnvTtydPath overrides ttyd discovery with an explicit binary path — the
// operator activation knob AND the hermetic test-injection seam (the
// SINET_SRT_PATH precedent, CONVENTIONS §12). ttyd is the packet's one adopted
// os-mechanism (S16.4 walk in components.lock; R21).
const EnvTtydPath = "SINET_TTYD_PATH"

// DefaultToolLookup resolves a tool over PATH, honoring SINET_TTYD_PATH for
// ttyd (probe-gated consumption, R20). It never installs anything.
func DefaultToolLookup(name string) (string, bool) {
	if name == "ttyd" {
		if p := os.Getenv(EnvTtydPath); p != "" {
			if isExecutable(p) {
				return p, true
			}
			return "", false // override set but unusable — honest absence
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}
	return "", false
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false
	}
	return fi.Mode().Perm()&0o111 != 0
}

// composeFiles are the conservative sidecar signal (Spec S13.8: Postgres/Redis-
// class projects surface as "requires container tier"; e.g. compose-file
// presence, R4).
var composeFiles = []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}

// Detect resolves the preview lane + runner for a deliverable revision from its
// read-only checkout, its open deliverable type, and the owner-approved
// registry preview command (empty when none). It is pure and deterministic.
//
// Precedence (executor discretion within the four spec-named detections, R2):
//
//  1. notebook — the type/file self-previews (R4).
//  2. CLI — data-driven only (an explicit CLI type); heuristics NEVER guess a
//     CLI (R20). ttyd serves the captured command.
//  3. registry preview command — the owner-approved command WINS over
//     heuristics (R3), as a dev-server runner.
//  4. sidecar — a compose file → the "requires container tier" state (R4).
//  5. host-toolchain heuristics — package.json→pnpm/npm, pyproject.toml→uv,
//     mise.toml→mise, index.html→static (R2).
//  6. none — the honest no-preview state for a type with no launchable surface
//     (R4).
func Detect(checkoutDir, dtype, registryCmd string) (Lane, *Runner) {
	dt := strings.ToLower(strings.TrimSpace(dtype))
	registryCmd = strings.TrimSpace(registryCmd) // F12: whitespace-only ⇒ no command

	// 1. Notebook self-preview.
	if dt == "notebook" || hasFileWithExt(checkoutDir, ".ipynb") {
		return LaneNotebook, nil
	}

	// 2. CLI — explicit type only, never heuristic-guessed (R20). ttyd serves
	// the captured command (the registry preview command).
	if dt == "cli" {
		r := &Runner{Tool: "ttyd", Source: "registry"}
		if registryCmd != "" {
			r.Argv = splitCommand(registryCmd)
		}
		return LaneCLI, r
	}

	// 3. Registry preview command wins over heuristics (R3).
	if registryCmd != "" {
		return LaneDevServer, &Runner{Tool: "registry", Argv: splitCommand(registryCmd), Source: "registry"}
	}

	// 4. Sidecar — conservative compose-file detection (R4).
	for _, f := range composeFiles {
		if exists(filepath.Join(checkoutDir, f)) {
			return LaneSidecar, nil
		}
	}

	// 5. Host-toolchain heuristics (R2), in a fixed precedence.
	switch {
	case exists(filepath.Join(checkoutDir, "package.json")):
		tool := "npm"
		if exists(filepath.Join(checkoutDir, "pnpm-lock.yaml")) {
			tool = "pnpm"
		}
		return LaneDevServer, &Runner{Tool: tool, Argv: []string{tool, "run", "dev"}, Source: "heuristic"}
	case exists(filepath.Join(checkoutDir, "pyproject.toml")):
		return LaneDevServer, &Runner{Tool: "uv", Argv: []string{"uv", "run", "dev"}, Source: "heuristic"}
	case exists(filepath.Join(checkoutDir, "mise.toml")):
		return LaneDevServer, &Runner{Tool: "mise", Argv: []string{"mise", "run", "dev"}, Source: "heuristic"}
	case exists(filepath.Join(checkoutDir, "index.html")):
		return LaneStatic, &Runner{Tool: "static", Source: "heuristic"}
	}

	// 6. No preview available for this type (R4).
	return LaneNone, nil
}

// hostInjections is the per-provider allowedHosts injection carried on the
// runner invocation (Spec S13.8: "allowedHosts injected, or Vite HMR silently
// dies"; R13). Every dev-server invocation carries the preview host as the
// platform env SINET_PREVIEW_HOST; the Vite lane (pnpm/npm) ADDITIONALLY names
// Vite's real `server.allowedHosts` config. The exact wire mechanism is
// confirmed at the deferred live smoke (the srt-config precedent) — the DATA is
// what the S15 surface and the tests assert now.
func hostInjections(tool, host string) []HostInjection {
	switch tool {
	case "static", "ttyd", "":
		return nil // platform-served / terminal — no dev-server host check
	}
	inj := []HostInjection{{Mechanism: "env", Key: "SINET_PREVIEW_HOST", Value: host}}
	if tool == "pnpm" || tool == "npm" {
		inj = append(inj, HostInjection{Mechanism: "config", Key: "server.allowedHosts", Value: host})
	}
	return inj
}

// splitCommand splits an owner-captured command into argv on whitespace. A
// registry preview command is a plain command line (S13.7 capture); shell
// metacharacters are the project's own concern and run inside the sandbox.
func splitCommand(cmd string) []string {
	return strings.Fields(cmd)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// hasFileWithExt reports whether the checkout's top level holds a file with the
// given extension (the notebook self-preview signal, R4). Top-level only — a
// deep scan is not the disposition's job.
func hasFileWithExt(dir, ext string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ext) {
			return true
		}
	}
	return false
}

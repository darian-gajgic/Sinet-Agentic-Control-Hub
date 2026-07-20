package claudecli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Engine-native auto-memory containment (Spec S09.9; P-T10-1).
//
// At the pinned engine version auto-memory is a background feature whose
// write root is $CLAUDE_CONFIG_DIR/memory/ (measured live —
// P3/measurements/2026-07-20-auto-memory-containment.md, C1). The lowering
// sets CLAUDE_CONFIG_DIR from the per-run owner credential ref, which
// closes the CROSS-USER channel by construction; the residual channel is
// per-user cross-RUN persistence — structurally an ungated L2, the 8.1
// bypass S09.9 targets. The containment mechanic is the measurement's
// recommendation: a version-independent per-run wipe of the config-root
// memory/ subdir at every session START, keeping the per-user auth files
// intact (a fresh config dir cannot authenticate — measurement C2). Every
// engine session therefore begins with an empty auto-memory dir, so
// nothing an engine writes there can steer any later run: the dir is
// workspace-scoped L0 in effect, discarded per the v0 posture (harvesting
// into the station-1 evidence pool is v1, Spec S09.4/S09.9). Resume does
// NOT wipe — a resumed session is the same run continuing, and its
// mid-flight scratch is that run's own L0.
//
// The GA memory tool needs no mechanic here: the default-deny --tools
// allowlist excludes it (measurement C4). Engine memory behavior is a
// standing per-pin canary entry (P-T10-1); a pin bump re-checks the write
// root and this posture.

// wipeEngineMemoryDir empties <configRoot>/memory/, keeping the directory
// itself and every sibling (auth files) untouched. A missing dir is a
// no-op. An empty configRoot means the platform supplied no credential
// ref — the compiled-config channel is not engaged (bare dev spawns), so
// there is nothing platform-owned to wipe.
func wipeEngineMemoryDir(configRoot string) error {
	if configRoot == "" {
		return nil
	}
	dir := filepath.Join(configRoot, "memory")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("claudecli: read engine memory dir: %w", err)
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("claudecli: wipe engine memory dir: %w", err)
		}
	}
	return nil
}

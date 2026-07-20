package claudecli

import (
	"os"
	"path/filepath"
	"testing"
)

// S09.9 containment mechanics (measurement:
// P3/measurements/2026-07-20-auto-memory-containment.md).

func TestWipeEngineMemoryDir(t *testing.T) {
	root := t.TempDir()
	// The per-user config root: auth files beside the auto-memory dir —
	// the wipe must present a fresh memory/ while keeping auth intact
	// (measurement C2: a fresh config dir cannot authenticate).
	mustWrite := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(".credentials.json", `{"token":"x"}`)
	mustWrite("settings.json", `{}`)
	mustWrite("memory/MEMORY.md", "agent-written steering state")
	mustWrite("memory/topics/deploys.md", "more state")

	if err := wipeEngineMemoryDir(root); err != nil {
		t.Fatalf("wipe: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "memory"))
	if err != nil {
		t.Fatalf("memory dir must survive (only its contents go): %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("memory dir not empty after wipe: %v", entries)
	}
	for _, rel := range []string{".credentials.json", "settings.json"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("auth sibling %s touched by the wipe: %v", rel, err)
		}
	}

	// Missing dir and disengaged channel are no-ops.
	if err := wipeEngineMemoryDir(t.TempDir()); err != nil {
		t.Fatalf("missing memory dir: %v", err)
	}
	if err := wipeEngineMemoryDir(""); err != nil {
		t.Fatalf("empty config root: %v", err)
	}
}

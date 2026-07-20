package claudecli_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
)

// TestStartWipesEngineAutoMemory proves the S09.9 containment on the real
// spawn path (fake engine, tier-F): a session START begins with an empty
// $CLAUDE_CONFIG_DIR/memory/ — stale agent-written state from any earlier
// run is gone before the engine process exists — while the auth files
// beside it survive. This closes the B1-4 measurement's reported gap (the
// config-root memory/ channel the B1 lowering did not cover) and stands as
// the per-pin canary's behavioral anchor (P-T10-1).
func TestStartWipesEngineAutoMemory(t *testing.T) {
	e := newE2E(t)
	ctx := context.Background()
	e.claimedRun(t, "r-mem")
	req := e2eRequest(t, "r-mem")

	credDir := t.TempDir()
	stale := filepath.Join(credDir, "memory", "MEMORY.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("stale steering state from a prior run"), 0o600); err != nil {
		t.Fatal(err)
	}
	auth := filepath.Join(credDir, ".credentials.json")
	if err := os.WriteFile(auth, []byte(`{"token":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	req.OwnerCredRef = credDir

	a := fakeEngineAdapter(t, "happy.jsonl", "SINET_FAKE_EXPECT_PROMPT=1")
	out, err := e.drv.Drive(ctx, a, req)
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("outcome = %q (%s)", out.Kind, out.Detail)
	}

	entries, err := os.ReadDir(filepath.Join(credDir, "memory"))
	if err != nil {
		t.Fatalf("memory dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("auto-memory dir not wiped at session start: %v", entries)
	}
	if _, err := os.Stat(auth); err != nil {
		t.Fatalf("auth file lost to the wipe: %v", err)
	}
}

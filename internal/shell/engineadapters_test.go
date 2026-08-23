package shell

// engineadapters_test.go — LN-2A/R30: the step that makes the lane exist.
// Registering the second adapter is what ends the one-agentic-lane posture, so
// it is asserted here rather than left to be noticed.

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

func ln2Deps(t *testing.T) engineAdapterDeps {
	t.Helper()
	return engineAdapterDeps{
		Settings: settings.New(),
		Logger:   slog.New(slog.NewTextHandler(testLogWriter{t}, nil)),
		StateDir: t.TempDir(),
	}
}

type testLogWriter struct{ t *testing.T }

func (w testLogWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s", strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// ── spec 33 · both substrates are registered ─────────────────────────────────

func TestOpenCodeAdapterRegistered(t *testing.T) {
	reg := engineAdapters(ln2Deps(t))
	for _, substrate := range []string{adapters.SubstrateClaudeCLI, adapters.SubstrateOpencode} {
		a, ok := reg[substrate]
		if !ok {
			t.Fatalf("the control plane registers no adapter for substrate %q — nothing on that substrate "+
				"can dispatch (S03.2)", substrate)
		}
		if a.Substrate() != substrate {
			t.Errorf("adapter registered under %q reports substrate %q", substrate, a.Substrate())
		}
	}
	if _, ok := reg[adapters.SubstrateClaudeCLI].(*claudecli.Adapter); !ok {
		t.Error("the Anthropic lane's adapter changed type")
	}
	oc, ok := reg[adapters.SubstrateOpencode].(*opencode.Adapter)
	if !ok {
		t.Fatalf("substrate %q is registered with %T", adapters.SubstrateOpencode, reg[adapters.SubstrateOpencode])
	}
	if oc.Instances == nil {
		t.Error("the opencode adapter has no per-user instance provider — every Start would refuse")
	}
	if oc.Root == "" {
		t.Error("the opencode adapter has no platform-owned per-user root")
	}
	if oc.ProvidersFor == nil {
		t.Error("the opencode adapter has no per-user provider resolver (S03.6: a provider entry per user)")
	}
	if len(oc.Lanes) == 0 {
		t.Error("the opencode adapter carries no lane documents, so no lane can report itself uncommissioned")
	}
}

// ── spec 34 · the second adapter is inert until a lane is commissioned ───────

func TestSecondAdapterInertWithoutLaneConfig(t *testing.T) {
	deps := ln2Deps(t)
	reg := engineAdapters(deps)
	oc := reg[adapters.SubstrateOpencode].(*opencode.Adapter)

	// No provider entry is commissioned for anybody at v0.
	for _, user := range []string{"u1", "operator", ""} {
		got, err := oc.ProvidersFor(user)
		if err != nil {
			t.Fatalf("ProvidersFor(%q): %v", user, err)
		}
		if len(got) != 0 {
			t.Errorf("user %q already has %d provider entries — the lane is commissioned by the key "+
				"ceremony, not by registering the adapter", user, len(got))
		}
	}

	// A dispatch onto the uncommissioned lane is a NAMED refusal, never a
	// spawn, never a silent unauthenticated call.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := oc.Start(ctx, adapters.StartRequest{
		RunID: "r-ln2", UserID: "u1",
		Model: "zai-coding-plan/glm-5.3",
		Cwd:   t.TempDir(), WorkDir: t.TempDir(),
		Worker: adapters.CompiledWorker{Prompt: "hi", AgentName: "sinet_w", ToolAllowlist: []string{"read"}},
	})
	if err == nil {
		t.Fatal("an uncommissioned lane started a session")
	}
	if !strings.Contains(err.Error(), "not commissioned") {
		t.Errorf("err = %v, want the named not-commissioned state", err)
	}

	// The Anthropic lane's registration is untouched.
	claude, ok := reg[adapters.SubstrateClaudeCLI].(*claudecli.Adapter)
	if !ok {
		t.Fatal("the claudecli registration changed shape")
	}
	if claude.Settings == nil || claude.Log == nil {
		t.Error("the claudecli adapter lost its settings or log wiring")
	}
	if claude.Binary != "" || claude.HookCmd != "" || claude.Env != nil {
		t.Errorf("the claudecli adapter gained construction arguments it did not have: %+v",
			struct{ Binary, HookCmd string }{claude.Binary, claude.HookCmd})
	}
}

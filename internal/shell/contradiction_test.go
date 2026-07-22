package shell

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

func screenEnv(t *testing.T) (*local.Duty, *local.FakeServer, stage.AdvisoryMeter) {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := eventlog.New(db, reg)
	runs := run.NewStore(db, log)
	if _, err := runs.Create(ctx, run.NewRun{ID: "adv.run", UserID: "platform", Lane: "local", Substrate: "local"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := runs.Transition(ctx, "adv.run", st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("Transition: %v", err)
		}
	}
	fake := local.NewFakeServer()
	t.Cleanup(fake.Close)
	duty := local.NewDuty(local.DutyDeps{Registry: local.NewRegistry(reg), Client: local.NewClient(fake.URL), Checkpoints: gates.NewCheckpoints(db, log), Events: log})
	meter := func(context.Context, string) (string, func(), error) { return "adv.run", func() {}, nil }
	return duty, fake, meter
}

func TestContradictionScreenHighPrecision(t *testing.T) {
	ctx := context.Background()
	duty, fake, meter := screenEnv(t)
	screen := newContradictionScreen(duty, meter)

	a := memory.Entry{ID: "e1", Kind: "lesson", Title: "Always deploy on Fridays", Content: "Always deploy on Fridays."}
	b := memory.Entry{ID: "e2", Kind: "lesson", Title: "Never deploy on Fridays", Content: "Never deploy on Fridays."}

	// The one-stage screen confirms on the WORKHORSE seat (Qwen3.5-9B), OQ4(a).
	fake.SetModelResponse("Qwen3.5-9B", local.FakeResponse{Content: `{"reason":"direct opposite","contradicts":"yes","abstain":false}`, InputTokens: 40, OutputTokens: 6})
	contradicts, rationale, err := screen.Screen(ctx, a, b)
	if err != nil {
		t.Fatalf("Screen: %v", err)
	}
	if !contradicts || rationale == "" {
		t.Errorf("a confident yes should contradict with a rationale; got %v %q", contradicts, rationale)
	}

	for _, tc := range []struct {
		name, content string
	}{
		{"no", `{"reason":"unrelated","contradicts":"no","abstain":false}`},
		{"unclear", `{"reason":"maybe","contradicts":"unclear","abstain":false}`},
		{"abstain", `{"reason":"cant tell","contradicts":"no","abstain":true}`},
	} {
		fake.SetModelResponse("Qwen3.5-9B", local.FakeResponse{Content: tc.content})
		if c, _, _ := screen.Screen(ctx, a, b); c {
			t.Errorf("%s must not be reported as a contradiction (high precision)", tc.name)
		}
	}
}

func TestContradictionScreenNilSafe(t *testing.T) {
	if s := newContradictionScreen(nil, nil); s != nil {
		t.Error("a nil duty must yield a nil screen (the deterministic detection runs regardless)")
	}
}

// TestWriteGateWiresScreen proves the screen attaches to a memory write gate
// through the shell factory (R16 "wired into Gate.Screen").
func TestWriteGateWiresScreen(t *testing.T) {
	duty, _, meter := screenEnv(t)
	screen := newContradictionScreen(duty, meter)
	g := newWriteGate(nil, nil, screen)
	if g.Screen == nil {
		t.Error("newWriteGate must wire the contradiction screen onto Gate.Screen (R16)")
	}
}

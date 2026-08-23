package shell

// lanecred_ln2_test.go — LN-2A/R7–R10: the lane's credential reaches the
// engine ONLY through the broker's existing engine-cred delivery, at the one
// place both halves are in scope. $0: the secret is a sentinel, no serve
// process is spawned, and nothing dials a provider.

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
)

const laneSentinel = "SINET-TEST-SECRET-0a7d41b8-6c22-4f9e-8f31-2b6d90c4e551"

// recordingInstances is the per-user serve seam, stopped one step before a
// process exists: it records the lowered spec and refuses. That is enough to
// see whether the credential arrived, and it spawns nothing.
type recordingInstances struct{ specs []opencode.InstanceSpec }

func (r *recordingInstances) Acquire(_ context.Context, spec opencode.InstanceSpec) (opencode.Instance, error) {
	r.specs = append(r.specs, spec)
	return opencode.Instance{}, errors.New("no serve is started in this test")
}
func (r *recordingInstances) Stop(context.Context, string) error { return nil }

func laneBrokerSocket(t *testing.T, profile string) string {
	t.Helper()
	store, err := broker.OpenStore(t.TempDir(), "me")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	socket := filepath.Join(t.TempDir(), "broker.sock")
	ln, err := broker.Listen(socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	srv := broker.NewServer(store, uint32(os.Getuid()), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	srv.AllowStore = true
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = srv.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	c, err := broker.Dial(socket)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	if err := c.Store(profile, broker.KindEngineCred, laneSentinel); err != nil {
		t.Fatalf("store %q: %v", profile, err)
	}
	return socket
}

func seedLane(t *testing.T) opencode.LaneConfig {
	t.Helper()
	lanes, err := opencode.SeedLaneConfigs()
	if err != nil || len(lanes) == 0 {
		t.Fatalf("SeedLaneConfigs: %v (%d lanes)", err, len(lanes))
	}
	for _, l := range lanes {
		if l.Lane == adapters.LaneZAI {
			return l
		}
	}
	t.Fatalf("no seed document for lane %q", adapters.LaneZAI)
	return opencode.LaneConfig{}
}

func laneStart(t *testing.T, lane opencode.LaneConfig, inject func([]string) ([]string, error)) (*recordingInstances, error) {
	t.Helper()
	inst := &recordingInstances{}
	a := &opencode.Adapter{
		Instances: inst,
		Root:      t.TempDir(),
		Lanes:     []opencode.LaneConfig{lane},
		Env:       []string{"PATH=/usr/bin", "HOME=/home/user"},
		Log:       slog.New(slog.NewTextHandler(testLogWriter{t}, nil)),
		ProvidersFor: func(string) (opencode.ProviderConfig, error) {
			return lane.Providers(), nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := a.Start(ctx, adapters.StartRequest{
		RunID: "r-cred", UserID: "u1",
		Model: lane.ProviderID + "/" + lane.Models[0].ID,
		Cwd:   t.TempDir(), WorkDir: t.TempDir(),
		Worker: adapters.CompiledWorker{
			Prompt: "hi", AgentName: "sinet_w", ToolAllowlist: []string{"read"},
		},
		CredInject: inject,
	})
	return inst, err
}

func TestLaneCredentialReachesTheEngineThroughTheBroker(t *testing.T) {
	lane := seedLane(t)
	socket := laneBrokerSocket(t, lane.Credential.Profile)

	inject := laneCredInject(socket, lane)
	if inject == nil {
		t.Fatal("the seed lane produced no credential injector — its document names no profile or variable")
	}
	inst, err := laneStart(t, lane, inject)
	if errors.Is(err, opencode.ErrLaneNotCommissioned) {
		t.Fatalf("a commissioned lane reported itself uncommissioned: %v", err)
	}
	if len(inst.specs) != 1 {
		t.Fatalf("instance specs = %d, want 1 — the run never reached the serve hand-off (%v)", len(inst.specs), err)
	}
	var got string
	for _, kv := range inst.specs[0].Env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == lane.Credential.EnvVar {
			got = v
		}
	}
	if got != laneSentinel {
		t.Fatalf("the engine environment did not receive the lane credential as %s", lane.Credential.EnvVar)
	}
	// The compiled config names the VARIABLE, never the value: the engine
	// reads the material from its own environment, and the config body is a
	// thing that gets logged, hashed and inspected.
	if strings.Contains(string(inst.specs[0].ConfigJSON), laneSentinel) {
		t.Error("the compiled config carries the credential material")
	}
	if !strings.Contains(string(inst.specs[0].ConfigJSON), lane.Credential.EnvVar) {
		t.Errorf("the compiled config does not reference %s, so the engine has no way to read the credential: %s",
			lane.Credential.EnvVar, inst.specs[0].ConfigJSON)
	}
}

func TestLaneWithoutABrokerCredentialIsNotCommissioned(t *testing.T) {
	lane := seedLane(t)
	if inject := laneCredInject("", lane); inject != nil {
		t.Error("a lane with no broker socket produced an injector — it must report itself uncommissioned instead")
	}
	inst, err := laneStart(t, lane, nil)
	if !errors.Is(err, opencode.ErrLaneNotCommissioned) {
		t.Fatalf("err = %v, want ErrLaneNotCommissioned", err)
	}
	if len(inst.specs) != 0 {
		t.Errorf("an uncommissioned lane reached the serve hand-off (%d specs)", len(inst.specs))
	}
}

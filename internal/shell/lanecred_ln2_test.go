package shell

// lanecred_ln2_test.go — LN-2A/R7–R10: the lane's credential reaches the
// engine ONLY through the broker's existing engine-cred delivery, at the one
// place both halves are in scope. $0: the secret is a sentinel, no serve
// process is spawned, and nothing dials a provider.

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
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
	return laneBrokerAt(t, filepath.Join(t.TempDir(), "broker.sock"), profile)
}

// laneBrokerAt stands up a broker on an exact socket path and stores the
// sentinel behind one auth profile.
func laneBrokerAt(t *testing.T, socket, profile string) string {
	t.Helper()
	store, err := broker.OpenStore(t.TempDir(), "me")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
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

// TestLaneSignalRoundTripsIntoTheScheduler is D3: the adapter's forwarded
// payload and the classifier's input are a real contract, pinned where both
// packages are in scope. A member lost in translation is not cosmetic — an
// EndpointVerified that decodes false turns a genuine depletion into an
// "endpoint misconfigured" verdict, and a lost ResetAt turns a Class-2 park
// with a known resume time into an indefinite probe schedule.
func TestLaneSignalRoundTripsIntoTheScheduler(t *testing.T) {
	reset := time.Date(2026, 8, 23, 18, 0, 0, 0, time.UTC)
	sent := opencode.LaneSignal{
		Lane: "zai", ErrorCode: "1308", HTTPStatus: 429, ResetAt: reset,
		BodyText: "Usage limit reached", EndpointVerified: true, Known: true,
	}
	raw, err := json.Marshal(sent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := scheduler.SignalFromPayload(raw)
	if err != nil {
		t.Fatalf("SignalFromPayload: %v (%s)", err, raw)
	}
	if got.Lane != sent.Lane || got.ErrorCode != sent.ErrorCode || got.HTTPStatus != sent.HTTPStatus {
		t.Errorf("identity members lost: %+v from %s", got, raw)
	}
	if !got.ResetAt.Equal(reset) {
		t.Errorf("reset = %v, want the signalled %v — a lost resume time becomes an indefinite park", got.ResetAt, reset)
	}
	if got.BodyText != sent.BodyText {
		t.Errorf("body text lost: %q", got.BodyText)
	}
	if !got.EndpointVerified {
		t.Fatal("EndpointVerified decoded false — a genuine depletion would be reported as a misconfigured endpoint")
	}
	// The decoded signal classifies as the depletion it is.
	cfg := scheduler.LimitConfig{RetryCap: 3, RetryBudgetRatio: 0.1, ProbeIntervalMax: 30 * time.Minute}
	if act := scheduler.Classify(got, cfg); act.Class != scheduler.ClassDepletionSignal {
		t.Errorf("round-tripped signal classified %d (%s), want class 2", act.Class, act.Reason)
	}

	// A signal with NO resume time must not arrive carrying a year-1 park
	// horizon (D2's other half, from the consumer's side).
	bare, err := json.Marshal(opencode.LaneSignal{Lane: "zai", ErrorCode: "1113", HTTPStatus: 429, EndpointVerified: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	sig, err := scheduler.SignalFromPayload(bare)
	if err != nil {
		t.Fatalf("SignalFromPayload(bare): %v", err)
	}
	if !sig.ResetAt.IsZero() {
		t.Errorf("an absent resume time decoded as %v", sig.ResetAt)
	}
	if act := scheduler.Classify(sig, cfg); act.Class != scheduler.ClassDepletionNoSignal {
		t.Errorf("1113 on a verified endpoint classified %d (%s), want class 3", act.Class, act.Reason)
	}
}

// TestProductionCredInjectDeliversTheLaneCredential is D4: the seam shell
// actually assigns to stage.Config.CredInject, exercised end to end.
func TestProductionCredInjectDeliversTheLaneCredential(t *testing.T) {
	lane := seedLane(t)
	stateDir := t.TempDir()
	// The broker listens where the production path looks for it.
	sockDir := filepath.Join(stateDir, "broker")
	if err := os.MkdirAll(sockDir, 0o700); err != nil {
		t.Fatalf("broker dir: %v", err)
	}
	laneBrokerAt(t, filepath.Join(sockDir, "u1.sock"), lane.Credential.Profile)

	lanes := engineLanes(slog.New(slog.NewTextHandler(testLogWriter{t}, nil)))
	commissioned := map[string]opencode.ProviderConfig{"u1": lane.Providers()}
	build := laneCredInjector(stateDir, lanes, commissioned)

	inject := build("u1")
	if inject == nil {
		t.Fatal("a commissioned user got no credential injector — the lane could never authenticate")
	}
	env, err := inject([]string{"PATH=/usr/bin"})
	if err != nil {
		t.Fatalf("inject: %v", err)
	}
	var got string
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == lane.Credential.EnvVar {
			got = v
		}
	}
	if got != laneSentinel {
		t.Fatalf("the engine environment did not receive the lane credential as %s (env %v)", lane.Credential.EnvVar, env)
	}
	if len(env) != 2 {
		t.Errorf("the base environment was not preserved: %v", env)
	}
	// Nobody commissioned gets nil — the unchanged dev posture, and the reason
	// an uncommissioned lane refuses instead of authenticating as nobody.
	if build("u2") != nil {
		t.Error("an uncommissioned user got a credential injector")
	}
	if laneCredInjector(stateDir, lanes, map[string]opencode.ProviderConfig{})("u1") != nil {
		t.Error("an empty commissioned map produced an injector")
	}
}

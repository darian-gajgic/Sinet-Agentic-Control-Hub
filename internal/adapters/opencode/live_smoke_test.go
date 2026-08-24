package opencode

// live_smoke_test.go — tier L of the conformance split (conformance_test.go):
// ONE minimal PAID call against a REAL commissioned lane on this substrate.
//
// Two gates stand in front of it and both print a named skip:
//
//  1. `SINET_LIVE_SMOKE=1` — THE tier-L opt-in, ratified by CONVENTIONS §10.
//     This suite reuses it and mints no second env name.
//  2. A lane COMMISSIONED on this host — its credential actually placed in this
//     person's broker store under the profile the lane document names.
//
// Gate 2 was the constant `false` at LN-1, when the packet placed no credential
// anywhere and there was nothing a predicate could read. LN-CEREMONY places
// them, so the gate is now the real question, asked the way production asks it:
// `internal/shell`'s `laneCredInject` commissions a lane when the broker
// resolves the lane document's engine-cred profile into the variable that same
// document names, and that is exactly the conjunction below.
//
// Nothing here can spend money by accident. Both gates must open, the engine
// must be installed, and the credential must already be on this host — three
// conditions no CI run and no ordinary `go test ./...` satisfies.

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// brokerStateDir / brokerStoreUser mirror the dev-posture path convention the
// `sinet broker` mode resolves its own defaults from. They are mirrored rather
// than imported because both are unexported there, and a tier-L gate that
// looked somewhere else would answer about a store nobody uses.
func brokerStateDir() string {
	if d := os.Getenv("STATE_DIRECTORY"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "sinet")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "state", "sinet")
	}
	return filepath.Join(os.TempDir(), "sinet-state")
}

func brokerStoreUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "uid-" + strconv.Itoa(os.Getuid())
}

func brokerStoreRoot() string { return filepath.Join(brokerStateDir(), "broker-store") }

// placedEngineCreds reports which auth profiles this person's broker store
// holds an ENGINE-CRED under.
//
// It is a presence check and stays one: it reads the record's plaintext `kind`
// — the same secret-free field the broker's own `kindOf` reads to derive a
// posture — and never opens the store, never decrypts, and never creates the
// master key. A gate that had to write to answer would change the host every
// time somebody ran the suite with nothing commissioned.
func placedEngineCreds() map[string]bool {
	dir := filepath.Join(brokerStoreRoot(), brokerStoreUser())
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	placed := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".cred" {
			continue
		}
		blob, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var rec struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(blob, &rec) != nil || rec.Kind != broker.KindEngineCred {
			continue
		}
		placed[name[:len(name)-len(".cred")]] = true
	}
	return placed
}

// commissionedLanes returns the seed lanes whose credential is placed. A lane
// declaring no profile or no variable is not commissionable at all — the same
// pair `laneCredInject` refuses to build an injector from.
func commissionedLanes() ([]LaneConfig, error) {
	lanes, err := SeedLaneConfigs()
	if err != nil {
		return nil, err
	}
	placed := placedEngineCreds()
	var out []LaneConfig
	for _, l := range lanes {
		if l.Credential.Profile == "" || l.Credential.EnvVar == "" {
			continue
		}
		if placed[l.Credential.Profile] {
			out = append(out, l)
		}
	}
	return out, nil
}

// laneCommissioned reports whether any lane on this substrate is commissioned
// on this host.
func laneCommissioned() bool {
	lanes, err := commissionedLanes()
	return err == nil && len(lanes) > 0
}

// laneBrokerSocket serves the placed credential over a PRIVATE in-process
// broker bound to a 0700 temp dir over the REAL store, so the injection under
// test travels the production path — `broker.EnvInjector` → `Client.Resolve` →
// the server's audience binding and destination constraint — rather than a
// shortcut that would prove the key resolves under rules the platform does not
// use. It runs no daemon and leaves no socket behind.
func laneBrokerSocket(t *testing.T) string {
	t.Helper()
	store, err := broker.OpenStore(brokerStoreRoot(), brokerStoreUser())
	if err != nil {
		t.Fatalf("open the broker store the ceremony placed the credential in: %v", err)
	}
	dir, err := os.MkdirTemp("", "sinet-tierl-")
	if err != nil {
		t.Fatalf("socket dir: %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("socket dir perms: %v", err)
	}
	sock := filepath.Join(dir, "b.sock")
	ln, err := broker.Listen(sock)
	if err != nil {
		t.Fatalf("listen on the private broker socket: %v", err)
	}
	srv := broker.NewServer(store, uint32(os.Getuid()), slog.New(slog.NewTextHandler(testWriter{t}, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = srv.Serve(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		<-done
		os.RemoveAll(dir)
	})
	return sock
}

func TestLiveSmoke(t *testing.T) {
	if os.Getenv("SINET_LIVE_SMOKE") != "1" {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §10): tier-L live smoke runs only under SINET_LIVE_SMOKE=1 (one paid call)")
	}
	lanes, err := commissionedLanes()
	if err != nil {
		t.Fatalf("the shipped lane documents did not load, so no lane can be commissioned: %v", err)
	}
	if len(lanes) == 0 {
		t.Skipf("SANCTIONED SKIP: no lane commissioned on this substrate — no engine-cred is placed in %s "+
			"under any shipped lane's auth profile (place one with P3/gates/lane-key-ceremony.sh)",
			filepath.Join(brokerStoreRoot(), brokerStoreUser()))
	}
	// One SUBTEST per commissioned lane, named by lane, so the ceremony can run
	// exactly one paid call behind exactly one typed confirmation:
	// `go test -run 'TestLiveSmoke/zai' ./internal/adapters/opencode`.
	for _, lane := range lanes {
		t.Run(lane.Lane, func(t *testing.T) { liveSmokeLane(t, lane) })
	}
}

// liveSmokeLane makes ONE minimal paid call on one commissioned lane and holds
// it to the same contract every other tier asserts: the Driver persists a
// checkpoint per paid call carrying the engine-REPORTED session id and the
// S02.4e invocation fingerprint.
func liveSmokeLane(t *testing.T, lane LaneConfig) {
	if lane.DefaultModel == "" {
		t.Skipf("SANCTIONED SKIP: lane %q ships no default_model, so this suite has no seat to spend on", lane.Lane)
	}
	bin := enginePath(t)
	sock := laneBrokerSocket(t)

	e := newE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	runID := "tierl-" + lane.Lane
	if _, err := e.runs.Create(ctx, run.NewRun{
		ID: runID, UserID: "u1", Substrate: adapters.SubstrateOpencode, Lane: lane.Lane,
	}); err != nil {
		t.Fatalf("create the run row: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed} {
		if _, err := e.runs.Transition(ctx, runID, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("transition to %s: %v", st, err)
		}
	}

	m := realManager(t, bin, 90*time.Second)
	a := &Adapter{
		Root:      t.TempDir(),
		Instances: m,
		Lanes:     []LaneConfig{lane},
		Providers: lane.Providers(),
		Env:       []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()},
		Log:       slog.New(slog.NewTextHandler(testWriter{t}, nil)),
		Now:       time.Now,
	}
	req := adapters.StartRequest{
		RunID: runID, UserID: "u1",
		Model:   lane.ProviderID + "/" + lane.DefaultModel,
		Cwd:     t.TempDir(),
		WorkDir: t.TempDir(),
		Worker: adapters.CompiledWorker{
			Prompt:        "Reply with exactly: ok",
			AgentsJSON:    json.RawMessage(`{"sinet_w":{"description":"tier-L smoke","prompt":"Answer in one word."}}`),
			AgentName:     "sinet_w",
			ToolAllowlist: []string{"read"}, // never `task` (S03.5)
			// No gated tools: a park would hold a paid turn open waiting for an
			// answer nobody is at a terminal to give.
			PermissionMode: "default",
		},
		// The lane's credential travels the S11.5 path and nowhere else: the
		// compiled config names the VARIABLE, the broker resolves the material
		// at spawn, and nothing here ever holds it.
		CredInject:     broker.EnvInjector(sock, lane.Credential.Profile, lane.Credential.EnvVar),
		CeilingCostUSD: 0.10,
		CeilingSteps:   2,
	}

	out, err := e.drv.Drive(ctx, a, req)
	if err != nil {
		t.Fatalf("lane %q: the paid call did not complete: %v", lane.Lane, err)
	}
	if out.Kind != adapters.OutcomeCompleted {
		t.Fatalf("lane %q: outcome = %q (%s), want completed", lane.Lane, out.Kind, out.Detail)
	}
	if r, _ := e.runs.Get(ctx, runID); r.State != run.StateCompleted {
		t.Fatalf("lane %q: run state = %s, want completed", lane.Lane, r.State)
	}

	var checkpoints int
	if err := e.db.QueryRowContext(ctx,
		`SELECT count(*) FROM checkpoints WHERE run_id = ?`, runID).Scan(&checkpoints); err != nil {
		t.Fatalf("count checkpoints: %v", err)
	}
	if checkpoints < 1 {
		t.Fatalf("lane %q: %d checkpoint rows — a paid call left no ledger row (D7)", lane.Lane, checkpoints)
	}
	cp, ok, err := gates.NewCheckpoints(e.db, e.log).Last(ctx, runID)
	if err != nil || !ok {
		t.Fatalf("lane %q: last checkpoint: ok=%v err=%v", lane.Lane, ok, err)
	}
	if cp.SessionID == "" {
		t.Errorf("lane %q: checkpoint carries no engine-reported session id (S02.4b)", lane.Lane)
	}
	if cp.InvocationFingerprint == "" {
		t.Errorf("lane %q: checkpoint lost the invocation fingerprint (S02.4e)", lane.Lane)
	}
	t.Logf("TIER L PASS: lane %q model %q completed on %s — %d paid call(s), session %q",
		lane.Lane, lane.DefaultModel, lane.ProviderID, checkpoints, cp.SessionID)
}

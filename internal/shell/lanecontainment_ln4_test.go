package shell

// lanecontainment_ln4_test.go — P3-LN-4 / T11 (S11.5, CONVENTIONS §62).
//
// Commissioning changes WHO gets an injector, never HOW the secret travels. So
// the containment property is re-run for a lane commissioned by the NEW path —
// composed from what is placed in the broker store — rather than inherited from
// LN-2's hand-built map. A property that holds for one map and was never
// checked against the other is a property about the test, not about the code.
//
// $0: no engine process exists (recordingInstances stops one step before one),
// nothing dials a provider, and the "credential" is a sentinel this repo
// generated for the purpose.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/opencode"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/broker"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// ln4Broker places the sentinel in the REAL per-person store the control plane
// composes its commissioned map from, and serves it on the socket the
// production injector dials. Both paths therefore point at one person's one
// store, which is the whole point of keying the map by the broker `who`.
func ln4Broker(t *testing.T, stateDir, who, profile string) {
	t.Helper()
	store, err := broker.OpenStore(broker.StoreRoot(stateDir), who)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	sockDir := filepath.Join(stateDir, "broker")
	if err := os.MkdirAll(sockDir, 0o700); err != nil {
		t.Fatalf("broker dir: %v", err)
	}
	ln, err := broker.Listen(filepath.Join(sockDir, who+".sock"))
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
	c, err := broker.Dial(filepath.Join(sockDir, who+".sock"))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	if err := c.Store(profile, broker.KindEngineCred, laneSentinel); err != nil {
		t.Fatalf("store %q: %v", profile, err)
	}
}

// T11 · the sentinel reaches the lowered serve environment exactly once, and
// appears nowhere else at all.
func TestLN4CredentialContainmentOnACommissionedLane(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	logger := ln4Logger(t)
	lanes := engineLanes(logger)
	lane := laneByName(t, lanes, adapters.LaneZAI)
	ln4Broker(t, stateDir, "me", lane.Credential.Profile)

	// THE fill: composed from what is placed, not handed in.
	commissioned := commissionEngineLanes(stateDir, lanes, logger)
	if len(commissioned["me"]) == 0 {
		t.Fatalf("the placed credential commissioned nothing (%v) — the rest of this test would prove nothing", commissioned)
	}
	inject := laneCredInjector(stateDir, lanes, commissioned)("me")
	if inject == nil {
		t.Fatal("the commissioned person got no injector")
	}

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
	drv := &adapters.Driver{
		Runs: runs, Checkpoints: gates.NewCheckpoints(db, log), Log: log, DB: db,
		CopyAsideDir: filepath.Join(t.TempDir(), "copy-aside"),
	}
	if _, err := runs.Create(ctx, run.NewRun{
		ID: "r-ln4", UserID: "me", Substrate: adapters.SubstrateOpencode, Lane: lane.Lane,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed} {
		if _, err := runs.Transition(ctx, "r-ln4", st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("transition %s: %v", st, err)
		}
	}

	ops := &strings.Builder{}
	inst := &recordingInstances{}
	a := &opencode.Adapter{
		Instances: inst,
		Root:      t.TempDir(),
		Lanes:     lanes,
		Env:       []string{"PATH=/usr/bin", "HOME=/home/user"},
		Log:       slog.New(slog.NewTextHandler(ops, nil)),
		ProvidersFor: func(userID string) (opencode.ProviderConfig, error) {
			return commissioned[userID], nil
		},
	}
	var payloads []string
	out, _ := drv.Drive(ctx, a, adapters.StartRequest{
		RunID: "r-ln4", UserID: "me",
		Model: lane.ProviderID + "/" + lane.DefaultModel,
		Cwd:   t.TempDir(), WorkDir: t.TempDir(),
		Worker: adapters.CompiledWorker{
			Prompt: "hi", AgentName: "sinet_w", ToolAllowlist: []string{"read"},
		},
		CredInject: inject,
		OnEvent:    func(ev adapters.Event) { payloads = append(payloads, string(ev.Payload)) },
	})

	// The positive control: the credential DID travel, exactly once, through
	// the env channel and nowhere else in the lowered spec.
	if len(inst.specs) != 1 {
		t.Fatalf("instance specs = %d, want 1 — the commissioned lane never reached the serve hand-off", len(inst.specs))
	}
	spec := inst.specs[0]
	var carried int
	for _, kv := range spec.Env {
		if strings.Contains(kv, laneSentinel) {
			carried++
			if k, _, _ := strings.Cut(kv, "="); k != lane.Credential.EnvVar {
				t.Errorf("the credential arrived as %q, want the document's own %q", k, lane.Credential.EnvVar)
			}
		}
	}
	if carried != 1 {
		t.Fatalf("the sentinel appears %d times in the lowered serve env, want exactly once", carried)
	}

	// …and nowhere else. The instance IDENTITY key is a sha256 over exactly
	// ConfigJSON, Root, Cwd and the env, so proving the sentinel is in none of
	// the first three — and that the key is a digest — is what makes the key
	// safe to compare, log around and hold in memory.
	for name, body := range map[string]string{
		"the compiled config":              string(spec.ConfigJSON),
		"the invocation fingerprint":       spec.Fingerprint,
		"the per-user root":                spec.Root,
		"the serve cwd":                    spec.Cwd,
		"the user id the spec is keyed by": spec.UserID,
		"the ops log":                      ops.String(),
	} {
		if strings.Contains(body, laneSentinel) {
			t.Errorf("%s carries the credential material", name)
		}
	}
	if !strings.Contains(string(spec.ConfigJSON), lane.Credential.EnvVar) {
		t.Errorf("the compiled config never references %s, so the engine has no way to read the credential",
			lane.Credential.EnvVar)
	}
	for _, p := range payloads {
		if strings.Contains(p, laneSentinel) {
			t.Errorf("an emitted event payload carries the credential material: %s", p)
		}
	}
	if out.Park != nil {
		blob, err := json.Marshal(out.Park)
		if err != nil {
			t.Fatalf("marshal the park record: %v", err)
		}
		if bytes.Contains(blob, []byte(laneSentinel)) {
			t.Error("the park record carries the credential material")
		}
	}

	// Every durable row, and then the whole store — not only the columns this
	// test thought to name.
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM run_events WHERE run_id = 'r-ln4'`).Scan(&rows); err != nil {
		t.Fatalf("count run_events: %v", err)
	}
	if rows == 0 {
		t.Fatal("the run produced no run_events rows at all — the durable scan below would be vacuous")
	}
	if err := forEachEventPayload(ctx, db, func(payload string) {
		if strings.Contains(payload, laneSentinel) {
			t.Errorf("a run_events row carries the credential material: %s", payload)
		}
	}); err != nil {
		t.Fatalf("scan run_events: %v", err)
	}
	if err := db.CheckpointTruncate(ctx); err != nil {
		t.Fatalf("checkpoint the WAL into the main file: %v", err)
	}
	for _, suffix := range []string{"", "-wal"} {
		raw, err := os.ReadFile(db.Path() + suffix)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", db.Path()+suffix, err)
		}
		if bytes.Contains(raw, []byte(laneSentinel)) {
			t.Errorf("the credential material is somewhere in %s", db.Path()+suffix)
		}
	}
}

func forEachEventPayload(ctx context.Context, db *storage.DB, fn func(string)) error {
	rows, err := db.QueryContext(ctx, `SELECT payload FROM run_events WHERE run_id = 'r-ln4'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var payload sql.NullString
		if err := rows.Scan(&payload); err != nil {
			return err
		}
		fn(payload.String)
	}
	return rows.Err()
}

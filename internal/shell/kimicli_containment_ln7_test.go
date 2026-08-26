package shell

// kimicli_containment_ln7_test.go — P3-LN-7 §10 spec T20 (S11.5, D2, §65).
//
// CONTAINMENT IS A PROPERTY, NOT A REVIEW. The credential reaches the lowered
// engine environment exactly ONCE and appears nowhere else — not in an event
// payload, not in a run_events row, not in the park record, not in the
// invocation fingerprint, not in the ops log, and nowhere in the SQLite file OR
// its write-ahead log.
//
// It is re-proven for THIS substrate rather than inherited from the opencode
// one, because the two carry credentials differently: opencode names a variable
// inside a compiled config body the engine reads, while this engine reads the
// KIMI_MODEL_* channel from the process environment directly. A guarantee that
// holds through one shape is not evidence about the other.
//
// $0: no provider is dialled, the engine binary is a path that does not exist,
// and the "credential" is a sentinel this repo invented.

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

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/kimicli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

const kimiCLISentinel = "sk-LN7-CONTAINMENT-SENTINEL-9f31c7"

func TestKimiCLICredentialContainmentProperty(t *testing.T) {
	ctx := context.Background()
	lanes := seedLanes(t)
	lane := laneByName(t, lanes, adapters.LaneKimiCLI)

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
	const runID = "r-ln7-containment"
	if _, err := runs.Create(ctx, run.NewRun{
		ID: runID, UserID: "me", Substrate: adapters.SubstrateKimiCLI, Lane: lane.Lane,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed} {
		if _, err := runs.Transition(ctx, runID, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("transition %s: %v", st, err)
		}
	}

	root := t.TempDir()
	ops := &strings.Builder{}
	a := &kimicli.Adapter{
		// A stub engine that BEHAVES like the real one: it announces its
		// version, answers, writes the session store and the transcript the
		// platform bills from, and exits 0.
		//
		// The first cut pointed this at a binary that does not exist, so the
		// spawn failed and every scan below — the emitted events, the park
		// record, the ops log, the durable rows — scanned NOTHING and passed
		// vacuously. A containment test that never ran a session proves
		// containment of nothing. It is a stub rather than the real binary so
		// the test stays hermetic and $0.
		Binary:       fakeEngine(t, root),
		Root:         filepath.Join(root, "engines"),
		BaseURL:      lane.BaseURL,
		ProviderType: "openai",
		Env:          []string{"PATH=/usr/bin", "HOME=/home/decoy"},
		Log:          slog.New(slog.NewTextHandler(ops, nil)),
	}

	// The injector is the production one, resolving the profile once per spawn
	// and fanning the material out to every variable that profile serves.
	var injected []string
	inject := laneCredInjectorWith(lanes, map[string]bool{lane.Credential.Profile: true},
		func(profile string, vars []string) func([]string) ([]string, error) {
			return func(base []string) ([]string, error) {
				out := append([]string{}, base...)
				for _, name := range vars {
					out = append(out, name+"="+kimiCLISentinel)
				}
				injected = out
				return out, nil
			}
		})
	if inject == nil {
		t.Fatal("no injector was composed for the kimi-cli lane's profile")
	}

	var payloads []string
	var sess adapters.Session
	out, _ := drv.Drive(ctx, a, adapters.StartRequest{
		RunID: runID, UserID: "me",
		Model:   lane.DefaultModel,
		Cwd:     t.TempDir(),
		WorkDir: t.TempDir(),
		Worker: adapters.CompiledWorker{
			Prompt: "hi", ToolAllowlist: []string{"Read"},
		},
		CredInject: inject,
		OnEvent:    func(ev adapters.Event) { payloads = append(payloads, string(ev.Payload)) },
		OnSession:  func(s adapters.Session) { sess = s },
	})

	// ── the positive control ────────────────────────────────────────────────
	// The credential DID travel, exactly once, under the variable the DOCUMENT
	// names. A containment test that never carried the secret proves nothing.
	if len(injected) == 0 {
		t.Fatal("the injector never ran — every assertion below would be vacuous")
	}
	// ONCE PER VARIABLE, and only under variables a commissioned lane document
	// actually names. The two kimi lanes share profile `kimi-code` — one
	// membership, one Console key — and name DIFFERENT variables, so one
	// resolution fans out to both: this CLI reads only the KIMI_MODEL_*
	// channel and ignores a shell KIMI_API_KEY entirely, while the API lane
	// reads exactly the name this one ignores. "Exactly once" is therefore a
	// statement about each variable, not about the environment as a whole, and
	// a duplicate under ONE name would be the real defect.
	wantVars := map[string]int{}
	for _, l := range lanes {
		if l.Credential.Profile == lane.Credential.Profile && l.Credential.EnvVar != "" {
			wantVars[l.Credential.EnvVar] = 0
		}
	}
	if len(wantVars) < 2 {
		t.Fatalf("only %d variables share profile %q — the fan-out this test exists to check is not exercised",
			len(wantVars), lane.Credential.Profile)
	}
	for _, kv := range injected {
		if !strings.Contains(kv, kimiCLISentinel) {
			continue
		}
		k, _, _ := strings.Cut(kv, "=")
		if _, named := wantVars[k]; !named {
			t.Errorf("the credential arrived under %q, which no lane document sharing profile %q names — the "+
				"material must only ever appear under a variable some document asked for", k, lane.Credential.Profile)
			continue
		}
		wantVars[k]++
	}
	for name, n := range wantVars {
		if n != 1 {
			t.Errorf("the sentinel appears %d times under %q, want exactly once", n, name)
		}
	}
	if wantVars[lane.Credential.EnvVar] != 1 {
		t.Errorf("this lane's own variable %q did not receive the credential — the spawn would authenticate as nobody",
			lane.Credential.EnvVar)
	}

	// ── and nowhere else ────────────────────────────────────────────────────
	for name, body := range map[string]string{
		"the ops log": ops.String(),
	} {
		if strings.Contains(body, kimiCLISentinel) {
			t.Errorf("%s carries the credential material", name)
		}
	}
	// Every scan leg gets a non-emptiness guard, because a leg with nothing in
	// it is a leg that cannot fail.
	if len(payloads) == 0 {
		t.Error("the run emitted no events at all — the payload scan below proves nothing")
	}
	for _, p := range payloads {
		if strings.Contains(p, kimiCLISentinel) {
			t.Errorf("an emitted event payload carries the credential material: %s", p)
		}
	}
	if ops.String() == "" {
		t.Log("the ops log is empty for this run; the scan of it is therefore weak evidence")
	}
	// The DIGEST leg A15 names: the invocation fingerprint is hashed, logged
	// and compared, so a secret reaching it would travel everywhere a digest
	// travels.
	if fp := out.Park; fp != nil && strings.Contains(fp.Fingerprint, kimiCLISentinel) {
		t.Error("the invocation fingerprint carries the credential material")
	}
	if sess != nil && strings.Contains(sess.Fingerprint(), kimiCLISentinel) {
		t.Error("the session fingerprint carries the credential material")
	}
	// The park record is the S03.4 full-invocation snapshot, and StartRequest
	// carries CredInject as a FUNC with json:"-" — so the material cannot ride
	// it by construction. Asserting it anyway is cheap, and the construction is
	// exactly the kind that a later struct edit can quietly undo.
	if out.Park != nil {
		blob, err := json.Marshal(out.Park)
		if err != nil {
			t.Fatalf("marshal the park record: %v", err)
		}
		if bytes.Contains(blob, []byte(kimiCLISentinel)) {
			t.Error("the park record carries the credential material")
		}
	}

	// Whatever the lowering wrote to disk inside the run's own home — the
	// config, the tui preferences, the system prompt. This engine is configured
	// through TOML files the platform writes, so a credential leaking into one
	// would be a secret at rest under the state dir.
	_ = filepath.Walk(a.Root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		raw, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		if bytes.Contains(raw, []byte(kimiCLISentinel)) {
			t.Errorf("a lowered config file carries the credential material: %s", p)
		}
		return nil
	})

	// Every durable row, then the whole store — not only the columns this test
	// thought to name.
	rows, err := db.QueryContext(ctx, `SELECT payload FROM run_events WHERE run_id = ?`, runID)
	if err != nil {
		t.Fatalf("scan run_events: %v", err)
	}
	seen := 0
	for rows.Next() {
		var payload sql.NullString
		if err := rows.Scan(&payload); err != nil {
			rows.Close()
			t.Fatalf("scan: %v", err)
		}
		seen++
		if strings.Contains(payload.String, kimiCLISentinel) {
			t.Errorf("a run_events row carries the credential material: %s", payload.String)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("run_events: %v", err)
	}
	if seen == 0 {
		t.Error("the run produced no run_events rows, so the durable scan proved less than it looks")
	}
	// The WAL is read BEFORE truncation as well as after: truncating first can
	// empty the very file the scan is meant to inspect, which would make the
	// -wal leg vacuous exactly when it matters.
	for _, suffix := range []string{"", "-wal"} {
		raw, err := os.ReadFile(db.Path() + suffix)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", db.Path()+suffix, err)
		}
		if bytes.Contains(raw, []byte(kimiCLISentinel)) {
			t.Errorf("the credential material is in %s (scanned before WAL truncation)", db.Path()+suffix)
		}
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
		if bytes.Contains(raw, []byte(kimiCLISentinel)) {
			t.Errorf("the credential material is somewhere in %s", db.Path()+suffix)
		}
	}
}

// fakeEngine writes a stub `kimi` that behaves like the real one for this
// test's purposes: it announces its version, answers, writes the session index
// and the transcript the platform bills from, and exits 0.
func fakeEngine(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "fake-kimi")
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\n")
	b.WriteString("set -u\n")
	b.WriteString(`home="${KIMI_CODE_HOME:?}"` + "\n")
	b.WriteString(`sid="session_containment"` + "\n")
	b.WriteString(`sdir="$home/sessions/wd_test_000000000000/$sid"` + "\n")
	b.WriteString(`mkdir -p "$sdir/agents/main"` + "\n")
	b.WriteString(`printf '{"sessionId":"%s","sessionDir":"%s","workDir":"%s"}\n' "$sid" "$sdir" "$PWD" > "$home/session_index.jsonl"` + "\n")
	b.WriteString(`echo '{"type":"llm.request","agentId":"main","model":"k3","turnStep":"0.1","time":1}' > "$sdir/agents/main/wire.jsonl"` + "\n")
	b.WriteString(`echo '{"type":"usage.record","agentId":"main","model":"__kimi_env_model__","usage":{"inputOther":73,"output":29,"inputCacheRead":64,"inputCacheCreation":0},"usageScope":"turn","time":2}' >> "$sdir/agents/main/wire.jsonl"` + "\n")
	b.WriteString(`echo '{"role":"meta","type":"system.version","version":"0.38.0"}'` + "\n")
	b.WriteString(`echo '{"role":"assistant","content":"containment ok"}'` + "\n")
	b.WriteString(`echo "{\"role\":\"meta\",\"type\":\"session.resume_hint\",\"session_id\":\"$sid\",\"command\":\"kimi -r $sid\",\"content\":\"hint\"}"` + "\n")
	b.WriteString("sleep 0.5\n")
	b.WriteString("exit 0\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o700); err != nil {
		t.Fatalf("write fake engine: %v", err)
	}
	return path
}

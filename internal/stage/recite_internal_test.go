package stage

// In-package reciter units (P3-B3-4): gating (which sessions recite),
// turns-dueness against the ⚙ interval, atomic re-author supersession,
// and the fires→manifest recording with its three dispositions. The full
// loop through a live engine session is recite_e2e_test.go.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// reciteSettings wraps the real registry with an interval override so the
// unit tests exercise clamp-legal values (0 off, 5..50) without a DB
// attach.
type reciteSettings struct {
	Settings
	interval int64
	fail     bool
}

func (s reciteSettings) Int(key string) (int64, error) {
	if key == keyRecitationInterval {
		if s.fail {
			return 0, fmt.Errorf("synthetic registry failure")
		}
		return s.interval, nil
	}
	return s.Settings.Int(key)
}

type reciteFixture struct {
	sk    *Skeleton
	led   *ledger.Store
	db    *storage.DB
	runID string
	gen   int64
}

func newReciteFixture(t *testing.T, interval int64) *reciteFixture {
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
	led := ledger.NewStore(db, log)
	if err := db.WriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES ('t-ru', 'u1', 'reciter units', ?)`,
			time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}); err != nil {
		t.Fatalf("task: %v", err)
	}
	if _, err := runs.Create(ctx, run.NewRun{ID: "t-ru.execute", UserID: "u1", TaskID: "t-ru",
		Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := led.SetObjective(ctx, "t-ru.execute", "operator", ledger.ObjectiveAC{
		Objective:          "unit fixture",
		AcceptanceCriteria: []ledger.AcceptanceCriterion{{N: 1, Plain: "exists"}},
		SpecVersion:        "spec-v1",
	}); err != nil {
		t.Fatalf("SetObjective: %v", err)
	}
	sk := &Skeleton{cfg: Config{
		DB: db, Log: log, Runs: runs, Ledger: led,
		Settings: reciteSettings{Settings: reg, interval: interval},
		RunRoot:  t.TempDir(),
	}}
	return &reciteFixture{sk: sk, led: led, db: db, runID: "t-ru.execute", gen: 0}
}

func (f *reciteFixture) input() SessionInput {
	return SessionInput{RunID: f.runID, Stage: "exec-1", Assemble: true}
}

func (f *reciteFixture) brief() ledger.Brief { return ledger.Brief{TaskID: "t-ru", LedgerVersion: 1} }

func usageTurn() adapters.Event {
	return adapters.Event{Kind: adapters.KindUsage, Usage: &adapters.Usage{InputTokens: 10, OutputTokens: 2}}
}

func TestReciterGating(t *testing.T) {
	f := newReciteFixture(t, 5)
	ctx := context.Background()
	wd := t.TempDir()
	tools := []string{"Bash"}

	if r := f.sk.newReciter(ctx, f.input(), f.brief(), wd, tools); r == nil {
		t.Error("ledger-backed working session with tools and interval 5 must recite")
	}
	in := f.input()
	in.Assemble = false
	if r := f.sk.newReciter(ctx, in, ledger.Brief{}, wd, tools); r != nil {
		t.Error("artifact-only session (no assembled brief) must not recite")
	}
	in = f.input()
	in.Clean = true
	if r := f.sk.newReciter(ctx, in, f.brief(), wd, tools); r != nil {
		t.Error("clean-context session must not recite (Spec S05.4 exception)")
	}
	if r := f.sk.newReciter(ctx, f.input(), f.brief(), wd, nil); r != nil {
		t.Error("tool-less session must not recite (no delivery boundary exists)")
	}

	off := newReciteFixture(t, 0)
	if r := off.sk.newReciter(ctx, off.input(), off.brief(), wd, tools); r != nil {
		t.Error("⚙ 0 must turn recitation off")
	}
	failing := newReciteFixture(t, 5)
	failing.sk.cfg.Settings = reciteSettings{Settings: settings.New(), fail: true}
	if r := failing.sk.newReciter(ctx, failing.input(), failing.brief(), wd, tools); r != nil {
		t.Error("a ⚙ read failure must disable loudly, never invent a number")
	}
}

func TestReciterDuenessAndReauthor(t *testing.T) {
	f := newReciteFixture(t, 5)
	ctx := context.Background()
	wd := t.TempDir()
	r := f.sk.newReciter(ctx, f.input(), f.brief(), wd, []string{"Bash"})
	if r == nil {
		t.Fatal("reciter gated off")
	}
	pendingPath := filepath.Join(wd, "gate-ctl", "recite", "pending.json")

	// Non-turn events never count toward dueness.
	r.observe(adapters.Event{Kind: adapters.KindMessage})
	r.observe(adapters.Event{Kind: adapters.KindUsage, Usage: &adapters.Usage{Total: true}})
	for i := 0; i < 4; i++ {
		r.observe(usageTurn())
	}
	if _, err := os.Stat(pendingPath); !os.IsNotExist(err) {
		t.Fatalf("pending authored before the interval (err=%v)", err)
	}
	// The 5th turn makes the recitation due.
	r.observe(usageTurn())
	raw, err := os.ReadFile(pendingPath)
	if err != nil {
		t.Fatalf("pending after 5th turn: %v", err)
	}
	var p pendingRecitation
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("pending shape: %v", err)
	}
	if p.LedgerVersion != 1 {
		t.Errorf("pending pinned to v%d, want the current revision v1", p.LedgerVersion)
	}
	doc, err := f.led.AtVersion(ctx, "t-ru", 1)
	if err != nil {
		t.Fatal(err)
	}
	if p.Content != ledger.RecitationText(doc) {
		t.Errorf("pending content diverges from the platform rendering at v1")
	}
	sum := sha256.Sum256([]byte(p.Content))
	if p.ContentSHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("pending hash inconsistent with its content")
	}

	// The ledger moves on; the next dueness re-authors and atomically
	// SUPERSEDES the undelivered pending with the fresh revision.
	next := []string{"the newer action"}
	if _, err := f.led.SessionVerbs(f.runID, "exec-1", f.gen).State(ctx,
		ledger.StateUpdate{NextActions: &next}); err != nil {
		t.Fatalf("State: %v", err)
	}
	for i := 0; i < 5; i++ {
		r.observe(usageTurn())
	}
	raw, err = os.ReadFile(pendingPath)
	if err != nil {
		t.Fatalf("pending after re-author: %v", err)
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.LedgerVersion != 2 {
		t.Errorf("re-authored pending pinned to v%d, want the superseding v2", p.LedgerVersion)
	}
	entries, err := os.ReadDir(filepath.Join(wd, "gate-ctl", "recite"))
	if err != nil || len(entries) != 1 {
		t.Errorf("recite dir = %v (err=%v), want exactly the one pending (atomic replace, no litter)", entries, err)
	}
}

func TestRecordRecitationsDispositions(t *testing.T) {
	f := newReciteFixture(t, 5)
	ctx := context.Background()
	in := f.input()

	// The session workdir the runner derives; seed a fires log with a
	// verified fire, a tampered-content fire, and an unknown-revision fire.
	workDir, _, err := f.sk.runDirs(in.RunID, in.Stage)
	if err != nil {
		t.Fatal(err)
	}
	ctl := filepath.Join(workDir, "gate-ctl")
	if err := os.MkdirAll(ctl, 0o700); err != nil {
		t.Fatal(err)
	}
	doc, err := f.led.AtVersion(ctx, "t-ru", 1)
	if err != nil {
		t.Fatal(err)
	}
	good := sha256.Sum256([]byte(ledger.RecitationText(doc)))
	bad := sha256.Sum256([]byte("bytes the platform never authored"))
	var lines []byte
	for _, fire := range []reciteFire{
		{TS: "t1", SessionID: "sid", ToolUseID: "toolu_ok", LedgerVersion: 1, ContentSHA256: hex.EncodeToString(good[:])},
		{TS: "t2", SessionID: "sid", ToolUseID: "toolu_tampered", LedgerVersion: 1, ContentSHA256: hex.EncodeToString(bad[:])},
		{TS: "t3", SessionID: "sid", ToolUseID: "toolu_ghost", LedgerVersion: 99, ContentSHA256: hex.EncodeToString(good[:])},
	} {
		raw, err := json.Marshal(fire)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(append(lines, raw...), '\n')
	}
	if err := os.WriteFile(filepath.Join(ctl, reciteFiresFile), lines, 0o600); err != nil {
		t.Fatal(err)
	}

	f.sk.recordRecitations(ctx, in)

	rows, err := f.db.QueryContext(ctx,
		`SELECT payload FROM run_events WHERE run_id = ? AND type = ? ORDER BY event_seq`,
		in.RunID, ledger.EventContextManifest)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type item struct {
		Disposition string `json:"disposition"`
	}
	type manifest struct {
		Kind      string `json:"kind"`
		ToolUseID string `json:"tool_use_id"`
		Items     []item `json:"items"`
	}
	got := map[string]string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		var m manifest
		if err := json.Unmarshal([]byte(p), &m); err != nil {
			t.Fatal(err)
		}
		if m.Kind != "recitation" || len(m.Items) != 1 {
			t.Fatalf("manifest = %s", p)
		}
		got[m.ToolUseID] = m.Items[0].Disposition
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"toolu_ok":       "",
		"toolu_tampered": ledger.RecitationHashMismatch,
		"toolu_ghost":    ledger.RecitationUnknownVersion,
	}
	if len(got) != len(want) {
		t.Fatalf("manifested deliveries = %v, want %v (every delivery manifested, none silent)", got, want)
	}
	for id, disposition := range want {
		if got[id] != disposition {
			t.Errorf("delivery %s disposition = %q, want %q", id, got[id], disposition)
		}
	}
}

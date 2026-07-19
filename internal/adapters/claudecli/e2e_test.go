package claudecli_test

// End-to-end tests of the Anthropic-lane adapter over a REAL subprocess:
// the test binary re-execs itself as a fake engine (the kill-9-harness
// pattern, CONVENTIONS §8) that replays a recorded fixture stream, so the
// full spawn → parse → run_events → checkpoints → engine_sessions path
// runs in CI with no engine installed. Tier F of the conformance split
// (conformance_test.go); the identical path against the real engine is
// tier L (live_smoke_test.go).

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

func TestMain(m *testing.M) {
	if fixture := os.Getenv("SINET_FAKE_ENGINE"); fixture != "" {
		os.Exit(fakeEngineMain(fixture))
	}
	os.Exit(m.Run())
}

// fakeEngineMain replays a fixture as the engine process.
func fakeEngineMain(fixture string) int {
	if os.Getenv("SINET_FAKE_IGNORE_TERM") == "1" {
		signal.Ignore(syscall.SIGTERM)
	}
	if out := os.Getenv("SINET_FAKE_ARGV_OUT"); out != "" {
		_ = os.WriteFile(out, []byte(strings.Join(os.Args, "\n")), 0o600)
	}
	stdin, _ := io.ReadAll(os.Stdin)
	if os.Getenv("SINET_FAKE_EXPECT_PROMPT") == "1" {
		var msg struct {
			Type    string `json:"type"`
			Message struct {
				Role string `json:"role"`
			} `json:"message"`
		}
		line := strings.TrimSpace(string(stdin))
		if err := json.Unmarshal([]byte(line), &msg); err != nil || msg.Type != "user" || msg.Message.Role != "user" {
			fmt.Fprintf(os.Stderr, "fake engine: bad prompt line %q\n", line)
			return 7
		}
	}
	raw, err := os.ReadFile(fixture)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake engine: %v\n", err)
		return 8
	}
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	sc.Buffer(make([]byte, 64<<10), 10<<20)
	for sc.Scan() {
		fmt.Println(sc.Text())
	}
	if os.Getenv("SINET_FAKE_STDERR") != "" {
		fmt.Fprintln(os.Stderr, os.Getenv("SINET_FAKE_STDERR"))
	}
	if d := os.Getenv("SINET_FAKE_SLEEP_AFTER"); d != "" {
		dur, err := time.ParseDuration(d)
		if err != nil {
			return 9
		}
		time.Sleep(dur)
	}
	if os.Getenv("SINET_FAKE_EXIT") != "" {
		var code int
		fmt.Sscanf(os.Getenv("SINET_FAKE_EXIT"), "%d", &code)
		return code
	}
	return 0
}

type e2eEnv struct {
	db   *storage.DB
	log  *eventlog.Log
	runs *run.Store
	drv  *adapters.Driver
}

func newE2E(t *testing.T) *e2eEnv {
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
	return &e2eEnv{
		db: db, log: log, runs: runs,
		drv: &adapters.Driver{
			Runs: runs, Checkpoints: gates.NewCheckpoints(db, log), Log: log, DB: db,
			CopyAsideDir: filepath.Join(t.TempDir(), "copy-aside"),
		},
	}
}

func (e *e2eEnv) claimedRun(t *testing.T, id string) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.runs.Create(ctx, run.NewRun{ID: id, UserID: "u1", Substrate: adapters.SubstrateClaudeCLI, Lane: adapters.LaneAnthropic}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, st := range []run.State{run.StateQueued, run.StateClaimed} {
		if _, err := e.runs.Transition(ctx, id, st, run.TransitionOptions{Actor: run.ActorPlatform}); err != nil {
			t.Fatalf("Transition: %v", err)
		}
	}
}

// fakeEngineAdapter builds a claudecli.Adapter that spawns THIS test
// binary as the engine, replaying the given fixture.
func fakeEngineAdapter(t *testing.T, fixture string, extraEnv ...string) *claudecli.Adapter {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	abs, err := filepath.Abs(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatalf("fixture path: %v", err)
	}
	env := append(os.Environ(), "SINET_FAKE_ENGINE="+abs)
	env = append(env, extraEnv...)
	return &claudecli.Adapter{
		Binary:      self,
		HookCmd:     "/opt/sinet/bin/sinet engine-hook",
		Settings:    settings.New(),
		Env:         env,
		CancelGrace: 500 * time.Millisecond,
	}
}

func e2eRequest(t *testing.T, runID string) adapters.StartRequest {
	t.Helper()
	return adapters.StartRequest{
		RunID: runID, UserID: "u1",
		Model:   "claude-haiku-4-5",
		Cwd:     t.TempDir(),
		WorkDir: t.TempDir(),
		Worker: adapters.CompiledWorker{
			Prompt:        "Reply with exactly: ok",
			ToolAllowlist: []string{"Bash", "Read"},
			GatedTools:    []string{"Bash"},
		},
	}
}

func TestE2EHappyPath(t *testing.T) {
	e := newE2E(t)
	ctx := context.Background()
	e.claimedRun(t, "r1")
	req := e2eRequest(t, "r1")

	// Seed the engine-transcript location the fixture implies, under the
	// per-user config root (owner cred ref), so copy-aside runs: the
	// fixture init reports cwd /tmp/run-cwd and session 7f1c9f60-….
	credDir := t.TempDir()
	transcript := filepath.Join(credDir, "projects", "-tmp-run-cwd", "7f1c9f60-4b2e-4c11-9a51-2f4e8d0c3b7a.jsonl")
	if err := os.MkdirAll(filepath.Dir(transcript), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(transcript, []byte(`{"transcript":true}`+"\n"), 0o600); err != nil {
		t.Fatalf("seed transcript: %v", err)
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
	if out.Totals == nil || out.Totals.EngineCostUSD != 0.0184178 {
		t.Errorf("totals = %+v", out.Totals)
	}
	r, _ := e.runs.Get(ctx, "r1")
	if r.State != run.StateCompleted {
		t.Fatalf("run state %s", r.State)
	}
	// The checkpoint cursor carries the engine-REPORTED session id, not
	// the control-plane-requested one (S02.4b).
	cps := gates.NewCheckpoints(e.db, e.log)
	cp, ok, err := cps.Last(ctx, "r1")
	if err != nil || !ok {
		t.Fatalf("checkpoint: ok=%v err=%v", ok, err)
	}
	if cp.SessionID != "7f1c9f60-4b2e-4c11-9a51-2f4e8d0c3b7a" {
		t.Errorf("checkpoint session id = %q, want the reported id", cp.SessionID)
	}
	if cp.CwdKey != "/tmp/run-cwd" {
		t.Errorf("cwd key = %q, want the reported cwd", cp.CwdKey)
	}
	if cp.TranscriptPath != transcript {
		t.Errorf("transcript path = %q, want derived %q", cp.TranscriptPath, transcript)
	}
	var copyPath string
	if err := e.db.QueryRowContext(ctx,
		`SELECT transcript_copy_path FROM engine_sessions WHERE run_id = 'r1'`).Scan(&copyPath); err != nil {
		t.Fatalf("engine_sessions: %v", err)
	}
	if got, err := os.ReadFile(copyPath); err != nil || string(got) != `{"transcript":true}`+"\n" {
		t.Errorf("copy-aside = %q err=%v", got, err)
	}
}

func TestE2EDeferParkThenResume(t *testing.T) {
	e := newE2E(t)
	ctx := context.Background()
	e.claimedRun(t, "r2")
	req := e2eRequest(t, "r2")

	a := fakeEngineAdapter(t, "defer.jsonl")
	out, err := e.drv.Drive(ctx, a, req)
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if out.Kind != adapters.OutcomeParked || out.Ask == nil {
		t.Fatalf("outcome = %q ask=%v", out.Kind, out.Ask)
	}
	if out.Ask.ID != "toolu_01JwN5WHtxKWbHoLyfaRdifh" {
		t.Fatalf("ask id = %q", out.Ask.ID)
	}
	r, _ := e.runs.Get(ctx, "r2")
	if r.State != run.StateParked {
		t.Fatalf("run state %s", r.State)
	}
	genAtPark := r.Generation
	var status string
	if err := e.db.QueryRowContext(ctx,
		`SELECT status FROM asks WHERE ask_id = ?`, out.Ask.ID).Scan(&status); err != nil {
		t.Fatalf("ask row: %v", err)
	}
	if status != "open" {
		t.Fatalf("ask status %q", status)
	}

	// Resume: full re-supply + staged answer; the fake engine dumps its
	// argv so the re-supply is assertable end-to-end.
	argvOut := filepath.Join(t.TempDir(), "argv.txt")
	a2 := fakeEngineAdapter(t, "happy.jsonl", "SINET_FAKE_ARGV_OUT="+argvOut)
	ans := &adapters.Answer{AskID: out.Ask.ID, UpdatedInput: json.RawMessage(`{"command":"echo ANSWER-42"}`)}
	out2, err := e.drv.DriveResume(ctx, a2, *out.Park, ans)
	if err != nil {
		t.Fatalf("DriveResume: %v", err)
	}
	if out2.Kind != adapters.OutcomeCompleted {
		t.Fatalf("resume outcome = %q (%s)", out2.Kind, out2.Detail)
	}
	argvRaw, err := os.ReadFile(argvOut)
	if err != nil {
		t.Fatalf("argv dump: %v", err)
	}
	argv := strings.ReplaceAll(string(argvRaw), "\n", " ")
	if !strings.Contains(argv, "--resume 44baeb68-1f97-4310-bb19-3bba41f482eb") {
		t.Errorf("resume argv missing --resume <reported id>: %s", argv)
	}
	if strings.Contains(argv, "--session-id") {
		t.Errorf("resume argv requested a fresh session: %s", argv)
	}
	for _, want := range []string{"--settings", "--tools Bash,Read", "--model claude-haiku-4-5"} {
		if !strings.Contains(argv, want) {
			t.Errorf("resume argv missing %q (full re-supply, S03.4): %s", want, argv)
		}
	}
	// The staged answer is on disk for the re-fired hook.
	answerFile := filepath.Join(out.Park.Start.WorkDir, "gate-ctl", "answers", out.Ask.ID+".json")
	if raw, err := os.ReadFile(answerFile); err != nil || !strings.Contains(string(raw), "ANSWER-42") {
		t.Errorf("staged answer %q err=%v", raw, err)
	}
	r, _ = e.runs.Get(ctx, "r2")
	if r.State != run.StateCompleted || r.Generation != genAtPark+1 {
		t.Fatalf("after resume: state=%s gen=%d (want completed, gen %d)", r.State, r.Generation, genAtPark+1)
	}
	if err := e.db.QueryRowContext(ctx,
		`SELECT status FROM asks WHERE ask_id = ?`, out.Ask.ID).Scan(&status); err != nil {
		t.Fatalf("ask row: %v", err)
	}
	if status != "answered" {
		t.Fatalf("ask status %q after resume", status)
	}
}

func TestE2EPauseParksAtBoundary(t *testing.T) {
	// Adapter-level (Driver park mapping is covered in driver_test):
	// pause lands at the message boundary and yields a full park record.
	a := fakeEngineAdapter(t, "pause.jsonl", "SINET_FAKE_SLEEP_AFTER=60s")
	req := e2eRequest(t, "r3")
	ctx := context.Background()
	sess, err := a.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	sawUsage := false
	for ev := range sess.Events() {
		if ev.Kind == adapters.KindUsage && !sawUsage {
			sawUsage = true
			if err := sess.Pause(ctx); err != nil {
				t.Fatalf("Pause: %v", err)
			}
		}
	}
	out, err := sess.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !sawUsage {
		t.Fatal("no usage event before pause")
	}
	if out.Kind != adapters.OutcomeParked || out.Park == nil || out.Park.Reason != adapters.ParkReasonPause {
		t.Fatalf("outcome = %q park=%+v", out.Kind, out.Park)
	}
	if out.Park.Cursor.SessionID != "a1b2c3d4-2222-4333-8444-000000000007" || out.Park.Cursor.MessageIndex != 1 {
		t.Errorf("park cursor = %+v", out.Park.Cursor)
	}
}

func TestE2ECancelLadderKillsTermIgnorer(t *testing.T) {
	a := fakeEngineAdapter(t, "pause.jsonl",
		"SINET_FAKE_SLEEP_AFTER=60s", "SINET_FAKE_IGNORE_TERM=1")
	req := e2eRequest(t, "r4")
	ctx := context.Background()
	sess, err := a.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	go func() {
		for range sess.Events() {
		}
	}()
	time.Sleep(200 * time.Millisecond) // let it stream the fixture
	if err := sess.Cancel(ctx); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := sess.Wait(waitCtx)
	if err != nil {
		t.Fatalf("Wait: %v (TERM-ignoring engine must die by KILL)", err)
	}
	if out.Kind != adapters.OutcomeCanceled {
		t.Fatalf("outcome = %q, want canceled", out.Kind)
	}
}

func TestE2EEngineCrashSurfacesStderr(t *testing.T) {
	e := newE2E(t)
	ctx := context.Background()
	e.claimedRun(t, "r5")
	a := fakeEngineAdapter(t, "crash.jsonl",
		"SINET_FAKE_EXIT=3", "SINET_FAKE_STDERR=engine exploding")
	out, err := e.drv.Drive(ctx, a, e2eRequest(t, "r5"))
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if out.Kind != adapters.OutcomeCrashed {
		t.Fatalf("outcome = %q", out.Kind)
	}
	if !strings.Contains(out.Detail, "engine exploding") {
		t.Errorf("crash detail lost stderr: %q", out.Detail)
	}
	if r, _ := e.runs.Get(ctx, "r5"); r.State != run.StateCrashed {
		t.Fatalf("run state %s", r.State)
	}
}

package stage_test

// tq2_resume_e2e_test.go — P3-TQ-2 grounding (committed RED, SKILL.md
// amendment A): fork-from-last-checkpoint resumes at the first plan step not
// completed, on a worktree restored to the last completed step's snapshot,
// and both runs narrate the crash→fork in plain words.
//
// Spec S02.5 step 2 (DEAD → "supersede by fork-from-last-checkpoint");
// S02.3 (crashed → superseded, generation+1); S02.4 (d) (the Claude-lane
// artifact ref is a platform snapshot commit in the run worktree); S06.6
// (plan steps with a per-step Done-when — the unit of resume); S05.1 §4 (work
// items carry status + evidence_ref); S05.3 (one fresh session per step, never a
// resumed transcript across a stage boundary); S13.5 (snapshot commits at
// stage boundaries; the branch only fast-forwards); S14.2 family 1 (every FSM
// transition carries its cause).
//
// The witnessed defect (P3/design/taskquality-webshop-findings-2026-09-16.md
// TQ-F2/TQ-F8, sitting world t-3120e8e3d14591d3): `execute.g1` re-drove S-1..S-8
// over the dead run's S-1..S-4 output, its first ledger write reset every
// finished item to `pending`, and no record on either run said why.
//
// The harness is the real spine (intake pipeline with a Go planner fake,
// scheduler Tick, stage dispatcher, adapter Driver with real D7 checkpoints,
// ledger, real internal/project git worktree, real recovery ladder), with the
// ENGINE replaced by an in-process scripted adapter that writes one file per
// plan step into the run's worktree — so every step's tree is distinguishable.
// Zero paid calls.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// ---- the n-step planner ----

// tq2Planner drafts a plan of n steps S-1..S-n; step i's Done-when is that
// `step-i.txt` exists in the working directory.
type tq2Planner struct{ n int }

func (p tq2Planner) pair(in intake.DraftInput) intake.Pair {
	steps := make([]intake.Step, 0, p.n)
	cover := []string{}
	for i := 1; i <= p.n; i++ {
		id := fmt.Sprintf("S-%d", i)
		steps = append(steps, intake.Step{
			ID: id, Title: fmt.Sprintf("Write step file %d", i),
			DoneWhen: fmt.Sprintf("step-%d.txt exists in the working directory", i),
			Class:    "C1", WriteSet: []string{fmt.Sprintf("step-%d.txt", i)},
			Approach: "I write one file and leave everything else alone.",
		})
		cover = append(cover, id)
	}
	return intake.Pair{
		Spec: intake.Spec{
			Provenance: "tq2-planner v1", Restatement: "Requester wants: " + in.Request.Title,
			Outcome:     []string{"the requester recognizes their goal"},
			ACs:         []intake.AC{{N: 1, Plain: "every step file exists", Structured: "WHEN listed THEN every step file is present", StructuredKind: "ears"}},
			Constraints: []string{"stay within the repo"}, OutOfScope: []string{"no deploys"},
		},
		Plan: intake.Plan{
			Provenance: "tq2-planner v1", Steps: steps,
			Coverage: map[string][]string{"AC-1": cover},
			Risks:    []string{"none"},
			Est:      intake.Estimate{SizeClass: "S", USD: 1.0, Known: true, Basis: "fake"},
		},
	}
}

func (p tq2Planner) Draft(_ context.Context, in intake.DraftInput) (intake.Pair, error) {
	return p.pair(in), nil
}

func (p tq2Planner) Revise(_ context.Context, in intake.ReviseInput) (intake.Pair, error) {
	return in.Pair, nil
}

// ---- the scripted engine ----

const (
	tq2CrashMid   = "mid"   // the step's session writes a partial file, checkpoints, then dies
	tq2CrashSpawn = "spawn" // the step's session never spawns (a crash after the previous step's close)
)

// tq2Start is what the engine saw at one execute-session spawn: which run and
// step, and the worktree exactly as it stood — file set, HEAD's tree id and
// `git status --porcelain` — before the session touched anything.
type tq2Start struct {
	RunID, Step, HeadTree, Status string
	Files                         []string
	Prompt                        string
	Seq                           int
}

type tq2Session struct {
	text   string
	kind   adapters.OutcomeKind
	detail string
	events []adapters.Event
}

func (s *tq2Session) Events() <-chan adapters.Event {
	ch := make(chan adapters.Event, len(s.events))
	for _, ev := range s.events {
		ch <- ev
	}
	close(ch)
	return ch
}
func (s *tq2Session) Cursor() adapters.Cursor {
	return adapters.Cursor{Substrate: adapters.SubstrateClaudeCLI, SessionID: "sess-tq2"}
}
func (s *tq2Session) Fingerprint() string          { return "fp-tq2" }
func (s *tq2Session) Pause(context.Context) error  { return nil }
func (s *tq2Session) Cancel(context.Context) error { return nil }
func (s *tq2Session) Wait(context.Context) (adapters.Outcome, error) {
	kind := s.kind
	if kind == "" {
		kind = adapters.OutcomeCompleted
	}
	return adapters.Outcome{Kind: kind, ResultText: s.text, Detail: s.detail}, nil
}

var tq2StepRe = regexp.MustCompile(`plan step (S-\d+) of task`)

// tq2Adapter is the scripted engine. Execute sessions write `step-<i>.txt`
// into the run's cwd at spawn (before the session's one paid call is consumed,
// so the D7 checkpoint snapshot captures it). The crash script fires ONCE at
// step crashAt: `mid` writes `step-<i>.partial`, checkpoints, and ends
// crashed; `spawn` refuses to start.
type tq2Adapter struct {
	t         *testing.T
	mu        sync.Mutex
	seq       int
	crashAt   int
	crashMode string
	fired     bool
	starts    []tq2Start
	// restoreSeq records the counter value at each RestoreWorkspace call, so
	// "restored BEFORE the first session" is an ordering fact, not a guess.
	restores []tq2Restore
}

type tq2Restore struct {
	RunID, SHA string
	Seq        int
}

func (a *tq2Adapter) Substrate() string { return adapters.SubstrateClaudeCLI }

func (a *tq2Adapter) usage() adapters.Event {
	return adapters.Event{
		Kind: adapters.KindUsage,
		Usage: &adapters.Usage{ModelID: "fake-model-1", InputTokens: 100, OutputTokens: 20,
			MessageID: "m1", MessageIndex: 1},
		Payload: []byte(`{"model":"fake-model-1"}`),
	}
}

func (a *tq2Adapter) Start(_ context.Context, req adapters.StartRequest) (adapters.Session, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	prompt := req.Worker.Prompt
	kind := ""
	if i := strings.Index(prompt, "SINET-STAGE: "); i >= 0 {
		rest := prompt[i+len("SINET-STAGE: "):]
		if j := strings.IndexByte(rest, '\n'); j >= 0 {
			rest = rest[:j]
		}
		kind = strings.TrimSpace(rest)
	}
	switch kind {
	case "critique":
		return &tq2Session{text: `{"kind":"pass"}`, events: []adapters.Event{a.usage()}}, nil
	case "execute":
		m := tq2StepRe.FindStringSubmatch(prompt)
		if m == nil {
			return nil, fmt.Errorf("tq2 adapter: execute prompt names no plan step:\n%.400s", prompt)
		}
		step := m[1]
		var n int
		fmt.Sscanf(step, "S-%d", &n)
		a.seq++
		a.starts = append(a.starts, tq2Start{
			RunID: req.RunID, Step: step, Prompt: prompt, Seq: a.seq,
			Files: tq2Files(a.t, req.Cwd), HeadTree: tq2GitOut(req.Cwd, "rev-parse", "HEAD^{tree}"),
			Status: tq2GitOut(req.Cwd, "status", "--porcelain"),
		})
		if n == a.crashAt && !a.fired {
			a.fired = true
			switch a.crashMode {
			case tq2CrashSpawn:
				return nil, errors.New("tq2: engine spawn refused (simulated process death after the previous step's close)")
			case tq2CrashMid:
				tq2Write(a.t, req.Cwd, fmt.Sprintf("step-%d.partial", n), "half of step "+step+"\n")
				return &tq2Session{kind: adapters.OutcomeCrashed, detail: "tq2: engine died mid-step (simulated)",
					events: []adapters.Event{a.usage()}}, nil
			}
		}
		tq2Write(a.t, req.Cwd, fmt.Sprintf("step-%d.txt", n), "step "+step+" done\n")
		return &tq2Session{text: "deliverable after " + step, events: []adapters.Event{a.usage()}}, nil
	}
	return nil, fmt.Errorf("tq2 adapter: unexpected session kind %q", kind)
}

func (a *tq2Adapter) Resume(context.Context, adapters.ParkRecord, *adapters.Answer) (adapters.Session, error) {
	return nil, errors.New("tq2 adapter: resume is never the mechanism across a stage boundary (Spec S05.3)")
}

func (a *tq2Adapter) execStarts(runID string) []tq2Start {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []tq2Start
	for _, s := range a.starts {
		if s.RunID == runID {
			out = append(out, s)
		}
	}
	return out
}

func (a *tq2Adapter) restoresFor(runID string) []tq2Restore {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []tq2Restore
	for _, r := range a.restores {
		if r.RunID == runID {
			out = append(out, r)
		}
	}
	return out
}

func tq2Write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// tq2Files lists the worktree's files (relative, sorted, .git excluded).
func tq2Files(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	sort.Strings(out)
	return out
}

// tq2GitOut runs a hermetic git in dir and returns trimmed stdout ("" on a
// non-repo dir, so workspace-less runs record an honest absence).
func tq2GitOut(dir string, args ...string) string {
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "HOME=/nonexistent")
	out, err := c.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ---- the project seams, fork-aware ----

// tq2Seams mirrors shell.projectSeams (CONVENTIONS §23/§55): run→project via
// the durable intake match, the worktree scoped to execute legs THROUGH THE
// SHARED FORK-SUFFIX MATCHER (P3-RW-6 R12 — a recovery fork `<task>.execute.g1`
// is the same execution and resolves to the same worktree), snapshot on an
// EXISTING worktree only.
type tq2Seams struct {
	proj *project.Store
	runs *run.Store
	pipe *intake.Pipeline
}

func (p *tq2Seams) projectForTask(ctx context.Context, taskID string) string {
	if p.pipe == nil || taskID == "" {
		return ""
	}
	st, err := p.pipe.LoadState(ctx, taskID)
	if err != nil || st.Registry == nil {
		return ""
	}
	return st.Registry.Project
}

func (p *tq2Seams) WorkspaceCwd(ctx context.Context, runID string) (string, bool, error) {
	if !strings.HasSuffix(metering.StripForkSuffix(runID), stage.RunSuffixExecute) {
		return "", false, nil
	}
	r, err := p.runs.Get(ctx, runID)
	if err != nil {
		return "", false, err
	}
	pid := p.projectForTask(ctx, r.TaskID)
	if pid == "" {
		return "", false, nil
	}
	ws, err := p.proj.EnsureWorkspace(ctx, pid, r.TaskID)
	if err != nil {
		return "", false, err
	}
	if r.WorkspaceRef != ws.Path {
		if err := p.runs.SetWorkspaceRef(ctx, runID, ws.Path); err != nil {
			return "", false, err
		}
	}
	return ws.Path, true, nil
}

func (p *tq2Seams) existingPath(ctx context.Context, runID string) (string, error) {
	r, err := p.runs.Get(ctx, runID)
	if err != nil {
		return "", err
	}
	pid := p.projectForTask(ctx, r.TaskID)
	if pid == "" {
		return "", nil
	}
	path, ok, err := p.proj.ExistingWorkspace(ctx, pid, r.TaskID)
	if err != nil || !ok {
		return "", err
	}
	return path, nil
}

func (p *tq2Seams) Snapshot(ctx context.Context, runID string) (string, error) {
	path, err := p.existingPath(ctx, runID)
	if err != nil || path == "" {
		return "", err
	}
	return p.proj.Snapshot(ctx, path)
}

func (p *tq2Seams) CreateRevisionRef(ctx context.Context, runID, ref, sha string) error {
	r, err := p.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	pid := p.projectForTask(ctx, r.TaskID)
	if pid == "" {
		return nil
	}
	return p.proj.CreateRevisionRef(ctx, pid, ref, sha)
}

// ---- the world ----

type tq2World struct {
	*harness
	proj    *project.Store
	adapter *tq2Adapter
	ladder  *recovery.Ladder
	clock   time.Time
	// failCompleted makes the NEXT `completed` transition of the named run
	// fail inside its transaction (the run.TerminalHook seam): the crash
	// window AFTER the last step's close.
	failCompleted string
}

// newTQ2World builds the spine for a plan of n steps whose engine crashes at
// step crashAt (0 = never) in crashMode. project=true onboards "shop" so the
// task is repo-backed (worktree + snapshots); false leaves it workspace-less.
func newTQ2World(t *testing.T, n, crashAt int, crashMode string, projectBacked bool) *tq2World {
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
	cps := gates.NewCheckpoints(db, log)
	led := ledger.NewStore(db, log)
	journal, err := gates.NewJournal(gates.JournalConfig{DB: db, Settings: reg})
	if err != nil {
		t.Fatalf("gates.NewJournal: %v", err)
	}
	root := t.TempDir()
	rev := &review.Store{DB: db, Log: log, Settings: reg, Root: filepath.Join(root, "review")}
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(root, "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	ps := &tq2Seams{proj: proj, runs: runs}
	w := &tq2World{proj: proj, clock: time.Now(),
		adapter: &tq2Adapter{t: t, crashAt: crashAt, crashMode: crashMode}}

	cfg := stage.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Ledger: led, Settings: reg,
		Adapters:     map[string]adapters.Adapter{adapters.SubstrateClaudeCLI: w.adapter},
		ArtifactRoot: filepath.Join(root, "artifacts"),
		RunRoot:      filepath.Join(root, "runs"),
		CopyAsideDir: filepath.Join(root, "copy-aside"),
		Model:        "fake-model-1",
		Planner:      tq2Planner{n: n},
		Review:       rev,
	}
	if projectBacked {
		cfg.Registry = registryOver{proj: proj}
		cfg.Snapshot, cfg.CreateRevisionRef, cfg.WorkspaceCwd = ps.Snapshot, ps.CreateRevisionRef, ps.WorkspaceCwd
		cfg.RepoFacts = func(ctx context.Context, taskID string) (string, string, error) {
			return proj.SnapshotAndBase(ctx, "shop", taskID)
		}
		// The fork's worktree seam: recorded (so ordering against the first
		// session is a fact) and performed through the real project verb.
		cfg.RestoreWorkspace = func(ctx context.Context, runID, sha string) (string, error) {
			w.adapter.mu.Lock()
			w.adapter.seq++
			w.adapter.restores = append(w.adapter.restores, tq2Restore{RunID: runID, SHA: sha, Seq: w.adapter.seq})
			w.adapter.mu.Unlock()
			r, err := runs.Get(ctx, runID)
			if err != nil {
				return "", err
			}
			path, ok, err := proj.ExistingWorkspace(ctx, "shop", r.TaskID)
			if err != nil || !ok {
				return "", err
			}
			return proj.RestoreSnapshot(ctx, path, sha)
		}
	}
	sk, err := stage.New(cfg)
	if err != nil {
		t.Fatalf("stage.New: %v", err)
	}
	ps.pipe = sk.Pipeline()
	priceTable := metering.NewEffectiveDatedTable("empty-v0")
	exceptions := metering.NoMeteredExceptions()
	sched, err := scheduler.New(scheduler.Config{
		DB: db, Runs: runs, Settings: reg, Dispatcher: sk,
		Receipts:     metering.NewReceipts(db, metering.NewLedger(db, priceTable, exceptions, reg), exceptions),
		LeaseTTL:     time.Minute,
		PollInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("scheduler.New: %v", err)
	}
	sk.Bind(sched)
	ladder, err := recovery.New(recovery.Config{
		DB: db, Log: log, Runs: runs, Checkpoints: cps, Effects: journal, Settings: reg,
		Now: func() time.Time { return w.clock },
	})
	if err != nil {
		t.Fatalf("recovery.New: %v", err)
	}
	if err := runs.SetTerminalHook(func(_ context.Context, _ *sql.Tx, r run.Run) error {
		if w.failCompleted != "" && r.ID == w.failCompleted && r.State == run.StateCompleted {
			w.failCompleted = ""
			return errors.New("tq2: process died inside the completed transition (simulated)")
		}
		return nil
	}); err != nil {
		t.Fatalf("SetTerminalHook: %v", err)
	}
	w.ladder = ladder
	w.harness = &harness{t: t, db: db, log: log, runs: runs, cps: cps, led: led,
		sk: sk, sched: sched, sur: sk.Surface(), review: rev, artifactRoot: filepath.Join(root, "artifacts")}
	if projectBacked {
		src := gitFixture(t, map[string]string{"go.mod": "module shop\n"})
		if _, err := proj.OnboardStart(ctx, project.OnboardInput{ProjectID: "shop", Owner: "u-tq2", Name: "shop", Source: src}); err != nil {
			t.Fatalf("OnboardStart: %v", err)
		}
		if err := proj.OnboardApprove(ctx, "shop", "u-tq2", nil); err != nil {
			t.Fatalf("OnboardApprove: %v", err)
		}
	}
	return w
}

// walkToExecute submits a task, clears the family and interview gates,
// approves the plan, and dispatches the execute leg once. Returns the task id.
func (w *tq2World) walkToExecute(ctx context.Context, projectBacked bool) string {
	w.t.Helper()
	const owner = "u-tq2"
	text := `{"title":"step files","text":"Write the numbered step files."}`
	if projectBacked {
		text = `{"title":"shop step files","text":"Write the numbered step files in the shop project."}`
	}
	raw, err := w.sur.Submit(ctx, owner, json.RawMessage(text))
	if err != nil {
		w.t.Fatalf("Submit: %v", err)
	}
	taskID := decodeView(w.t, raw).TaskID
	if n := w.tick(ctx); n != 1 {
		w.t.Fatalf("intake tick dispatched %d", n)
	}
	raw, err = w.sur.Task(ctx, taskID)
	if err != nil {
		w.t.Fatalf("Task: %v", err)
	}
	raw = clearFamilyGate(w.t, ctx, w.sur, owner, raw)
	raw, err = w.sur.Answer(ctx, owner, decodeView(w.t, raw).OpenAskID, json.RawMessage(`{"force_proceed":true}`), false)
	if err != nil {
		w.t.Fatalf("force_proceed: %v", err)
	}
	if _, err := w.sur.Answer(ctx, owner, decodeView(w.t, raw).OpenAskID, json.RawMessage(`{"action":"approve"}`), true); err != nil {
		w.t.Fatalf("approve: %v", err)
	}
	if n := w.tick(ctx); n != 1 {
		w.t.Fatalf("execute tick dispatched %d", n)
	}
	return taskID
}

// forkOf drives ONE ladder pass past the S02.5 reap bound (CONVENTIONS §54)
// and returns the successor the ladder created for runID.
func (w *tq2World) forkOf(ctx context.Context, runID string) string {
	w.t.Helper()
	w.clock = w.clock.Add(30 * time.Minute)
	rpt, err := w.ladder.ReconcilePass(ctx)
	if err != nil {
		w.t.Fatalf("ReconcilePass: %v", err)
	}
	if rpt.Forked != 1 {
		w.t.Fatalf("pass forked %d runs, want exactly 1 (report %+v)", rpt.Forked, rpt)
	}
	parent, err := w.runs.Get(ctx, runID)
	if err != nil {
		w.t.Fatalf("parent: %v", err)
	}
	if parent.State != run.StateCrashed {
		w.t.Fatalf("parent %s is %s, want crashed", runID, parent.State)
	}
	return fmt.Sprintf("%s.g%d", runID, parent.Generation)
}

func (w *tq2World) state(runID string) run.State {
	w.t.Helper()
	r, err := w.runs.Get(context.Background(), runID)
	if err != nil {
		w.t.Fatalf("Get %s: %v", runID, err)
	}
	return r.State
}

// stateEvent returns the payload of the run's run.state_changed row INTO `to`
// (the first such row).
func (w *tq2World) stateEvent(runID string, to run.State) map[string]any {
	w.t.Helper()
	rows, err := w.db.QueryContext(context.Background(),
		`SELECT payload FROM run_events WHERE run_id = ? AND type = ? ORDER BY event_seq`, runID, run.EventState)
	if err != nil {
		w.t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			w.t.Fatal(err)
		}
		var pay map[string]any
		if err := json.Unmarshal([]byte(raw), &pay); err != nil {
			w.t.Fatal(err)
		}
		if pay["to"] == string(to) {
			return pay
		}
	}
	return nil
}

// firstLedgerState returns the work-item statuses and `current` of the run's
// FIRST ledger_update row — the fork's seed.
func (w *tq2World) firstLedgerState(runID string) (status map[string]string, current string) {
	w.t.Helper()
	var raw string
	err := w.db.QueryRowContext(context.Background(),
		`SELECT payload FROM run_events WHERE run_id = ? AND type = ? ORDER BY event_seq LIMIT 1`,
		runID, ledger.EventLedgerUpdate).Scan(&raw)
	if err != nil {
		w.t.Fatalf("first ledger_update of %s: %v", runID, err)
	}
	var pay struct {
		Ledger ledger.Document `json:"ledger"`
	}
	if err := json.Unmarshal([]byte(raw), &pay); err != nil {
		w.t.Fatal(err)
	}
	status = map[string]string{}
	for _, it := range pay.Ledger.State.Items {
		status[it.ID] = string(it.Status)
	}
	return status, pay.Ledger.State.Current
}

func detailOf(pay map[string]any) map[string]any {
	d, _ := pay["detail"].(map[string]any)
	return d
}

func stepFiles(k int) []string {
	out := []string{"go.mod"}
	for i := 1; i <= k; i++ {
		out = append(out, fmt.Sprintf("step-%d.txt", i))
	}
	sort.Strings(out)
	return out
}

func stepIDs(from, to int) []string {
	var out []string
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf("S-%d", i))
	}
	return out
}

func joinAny(v any) string {
	arr, _ := v.([]any)
	parts := make([]string, 0, len(arr))
	for _, a := range arr {
		parts = append(parts, fmt.Sprint(a))
	}
	return strings.Join(parts, ",")
}

// ---- the invariant, over (n, k, mode) ----

// TestTQ2ForkResumesAtFirstIncompleteStep: for a plan of n steps and a crash
// after step k's close (`spawn` of k+1 refused) or mid-step k+1 (`mid`), the
// successor's first driven step is k+1, the worktree at its start is step k's
// close snapshot tree (clean, no partial), no step ≤ k is driven, the ledger
// seed keeps S-1..S-k done and points at S-(k+1), and both runs narrate.
func TestTQ2ForkResumesAtFirstIncompleteStep(t *testing.T) {
	for n := 1; n <= 3; n++ {
		for k := 0; k < n; k++ {
			for _, mode := range []string{tq2CrashMid, tq2CrashSpawn} {
				t.Run(fmt.Sprintf("n%d_k%d_%s", n, k, mode), func(t *testing.T) {
					ctx := context.Background()
					w := newTQ2World(t, n, k+1, mode, true)
					taskID := w.walkToExecute(ctx, true)
					parent := taskID + ".execute"
					if got := w.state(parent); got != run.StateCrashed {
						t.Fatalf("crash window: %s is %s, want crashed", parent, got)
					}

					// The resume target, from the record: step k's evidence_ref
					// (k>0) or the attempt base (k=0).
					doc, _, err := w.led.Current(ctx, taskID)
					if err != nil {
						t.Fatalf("ledger Current: %v", err)
					}
					want := ""
					if k > 0 {
						for _, it := range doc.State.Items {
							if it.ID == fmt.Sprintf("S-%d", k) {
								if it.Status != ledger.StatusDoneUnverified {
									t.Fatalf("S-%d is %s in the ledger before the fork, want done_unverified", k, it.Status)
								}
								want = it.EvidenceRef
							}
						}
						if want == "" {
							t.Fatalf("S-%d is done_unverified but carries no evidence_ref: no step-close snapshot was recorded (Spec S13.5 stage-boundary snapshot; S05.1 §4 evidence_ref)", k)
						}
					} else {
						_, base, err := w.proj.SnapshotAndBase(ctx, "shop", taskID)
						if err != nil || base == "" {
							t.Fatalf("attempt base: %q %v", base, err)
						}
						want = base
					}
					e, _ := w.proj.Get(ctx, "shop")
					wantTree := gitRevParse(t, e.StorePath, want+"^{tree}")

					fork := w.forkOf(ctx, parent)
					if n := w.tick(ctx); n != 1 {
						t.Fatalf("fork tick dispatched %d", n)
					}

					starts := w.adapter.execStarts(fork)
					if len(starts) == 0 {
						t.Fatalf("the fork %s drove no execute session", fork)
					}
					first := starts[0]
					if first.Step != fmt.Sprintf("S-%d", k+1) {
						t.Errorf("first driven step = %s, want S-%d (the first step NOT completed, Spec S02.5 step 2 / S06.6)", first.Step, k+1)
					}
					if len(starts) != n-k {
						t.Errorf("the fork drove %d sessions, want %d (steps S-%d..S-%d only)", len(starts), n-k, k+1, n)
					}
					for _, s := range starts {
						var i int
						fmt.Sscanf(s.Step, "S-%d", &i)
						if i <= k {
							t.Errorf("step %s (≤ k=%d) was driven again — finished work re-spent", s.Step, k)
						}
					}
					// The worktree at the first session's start IS step k's close
					// tree: clean, every finished step's file present, no partial.
					if first.HeadTree != wantTree {
						t.Errorf("worktree HEAD tree at the fork's first session = %s, want step-%d snapshot tree %s (Spec S02.4 (d), S13.5)", first.HeadTree, k, wantTree)
					}
					if first.Status != "" {
						t.Errorf("worktree dirty at the fork's first session:\n%s", first.Status)
					}
					if got, want := strings.Join(first.Files, ","), strings.Join(stepFiles(k), ","); got != want {
						t.Errorf("files at the fork's first session = [%s], want [%s]", got, want)
					}
					// Restored through the seam, exactly once, to that snapshot,
					// BEFORE the first session.
					rs := w.adapter.restoresFor(fork)
					if len(rs) != 1 {
						t.Errorf("RestoreWorkspace called %d times for the fork, want exactly 1", len(rs))
					} else {
						if rs[0].SHA != want {
							t.Errorf("RestoreWorkspace asked for %s, want %s", rs[0].SHA, want)
						}
						if rs[0].Seq > first.Seq {
							t.Errorf("RestoreWorkspace ran AFTER the first session (seq %d > %d)", rs[0].Seq, first.Seq)
						}
					}
					// The ledger seed keeps the record: S-1..S-k done, current S-(k+1).
					status, current := w.firstLedgerState(fork)
					for i := 1; i <= k; i++ {
						if got := status[fmt.Sprintf("S-%d", i)]; got != string(ledger.StatusDoneUnverified) {
							t.Errorf("fork's first ledger write: S-%d = %q, want done_unverified (the parent's record must not be reset)", i, got)
						}
					}
					for i := k + 1; i <= n; i++ {
						if got := status[fmt.Sprintf("S-%d", i)]; got != string(ledger.StatusPending) {
							t.Errorf("fork's first ledger write: S-%d = %q, want pending", i, got)
						}
					}
					if current != fmt.Sprintf("S-%d", k+1) {
						t.Errorf("fork's first ledger write: current = %q, want S-%d", current, k+1)
					}
					// The successor's brief tells it where it stands (Spec S05.4).
					if !strings.Contains(first.Prompt, fmt.Sprintf("S-%d", k+1)) {
						t.Errorf("the fork's first prompt does not name its step S-%d", k+1)
					}
					if k > 0 && !strings.Contains(first.Prompt, "done_unverified") {
						t.Errorf("the fork's first brief does not show the finished steps as done (ledger state block)")
					}
					// Narration (TQ-F8): the successor's running transition names
					// the resume point and the cause; the parent's crash says what
					// failed, in plain words.
					running := w.stateEvent(fork, run.StateRunning)
					if running == nil {
						t.Fatalf("no claimed→running transition recorded for %s", fork)
					}
					d := detailOf(running)
					if got := fmt.Sprint(d["resume_step"]); got != fmt.Sprintf("S-%d", k+1) {
						t.Errorf("fork running.detail.resume_step = %q, want S-%d", got, k+1)
					}
					if got, want := joinAny(d["completed_steps"]), strings.Join(stepIDs(1, k), ","); got != want {
						t.Errorf("fork running.detail.completed_steps = [%s], want [%s]", got, want)
					}
					if got := fmt.Sprint(d["parent_run_id"]); got != parent {
						t.Errorf("fork running.detail.parent_run_id = %q, want %s", got, parent)
					}
					if k > 0 {
						if got := fmt.Sprint(d["resume_snapshot"]); got != want {
							t.Errorf("fork running.detail.resume_snapshot = %q, want %s", got, want)
						}
					}
					reason := fmt.Sprint(running["reason"])
					if !strings.Contains(reason, fmt.Sprintf("S-%d", k+1)) || !strings.Contains(reason, parent) {
						t.Errorf("fork running.reason must name the resume step and the run it picks up from, got %q", reason)
					}
					crash := w.stateEvent(parent, run.StateCrashed)
					if crash == nil {
						t.Fatalf("no crash transition recorded for %s", parent)
					}
					cd := detailOf(crash)
					if got := fmt.Sprint(cd["step"]); got != fmt.Sprintf("S-%d", k+1) {
						t.Errorf("parent crash.detail.step = %q, want S-%d", got, k+1)
					}
					wantFailed := "work"
					if mode == tq2CrashSpawn {
						wantFailed = "platform"
					}
					if got := fmt.Sprint(cd["failed"]); got != wantFailed {
						t.Errorf("parent crash.detail.failed = %q, want %q (mode %s)", got, wantFailed, mode)
					}
					if plain := fmt.Sprint(cd["plain"]); plain == "" || plain == "<nil>" || !strings.Contains(plain, fmt.Sprintf("S-%d", k+1)) {
						t.Errorf("parent crash.detail.plain must be a sentence naming the step, got %q", plain)
					}
					// The lineage finishes: the fork completes and hands to verify.
					if got := w.state(fork); got != run.StateCompleted {
						t.Errorf("fork %s is %s, want completed", fork, got)
					}
					if got := w.state(taskID + ".verify"); got != run.StateQueued {
						t.Errorf("verify run is %s, want queued", got)
					}
				})
			}
		}
	}
}

// TestTQ2ForkAfterLastStepCloseDrivesNothing: k = n — the parent dies after
// the last step's close (inside its own completed transition). The successor
// drives NO step: the deliverable was filed as part of the last step's close,
// so it closes the leg and hands to verify.
func TestTQ2ForkAfterLastStepCloseDrivesNothing(t *testing.T) {
	const n = 2
	ctx := context.Background()
	w := newTQ2World(t, n, 0, "", true)
	// Arm the terminal-hook failure for the parent's `completed`.
	w.failCompleted = "" // set below once the task id is known
	const owner = "u-tq2"
	raw, err := w.sur.Submit(ctx, owner, json.RawMessage(`{"title":"shop step files","text":"Write the numbered step files in the shop project."}`))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	taskID := decodeView(t, raw).TaskID
	parent := taskID + ".execute"
	w.failCompleted = parent
	if n := w.tick(ctx); n != 1 {
		t.Fatalf("intake tick dispatched %d", n)
	}
	raw, _ = w.sur.Task(ctx, taskID)
	raw = clearFamilyGate(t, ctx, w.sur, owner, raw)
	raw, _ = w.sur.Answer(ctx, owner, decodeView(t, raw).OpenAskID, json.RawMessage(`{"force_proceed":true}`), false)
	if _, err := w.sur.Answer(ctx, owner, decodeView(t, raw).OpenAskID, json.RawMessage(`{"action":"approve"}`), true); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if n := w.tick(ctx); n != 1 {
		t.Fatalf("execute tick dispatched %d", n)
	}
	if got := w.state(parent); got != run.StateRunning {
		t.Fatalf("after the failed completed transition %s is %s, want running (driver-less, for the ladder)", parent, got)
	}
	if got := len(w.adapter.execStarts(parent)); got != n {
		t.Fatalf("parent drove %d sessions, want %d", got, n)
	}
	fork := w.forkOf(ctx, parent)
	if n := w.tick(ctx); n != 1 {
		t.Fatalf("fork tick dispatched %d", n)
	}
	if got := w.adapter.execStarts(fork); len(got) != 0 {
		var steps []string
		for _, s := range got {
			steps = append(steps, s.Step)
		}
		t.Errorf("the fork drove %d sessions after a crash past the last step's close, want 0: %v", len(got), steps)
	}
	if got := w.state(fork); got != run.StateCompleted {
		t.Errorf("fork %s is %s, want completed", fork, got)
	}
	if got := w.state(taskID + ".verify"); got != run.StateQueued {
		t.Errorf("verify run is %s, want queued", got)
	}
	status, current := w.firstLedgerState(fork)
	for i := 1; i <= n; i++ {
		if got := status[fmt.Sprintf("S-%d", i)]; got != string(ledger.StatusDoneUnverified) {
			t.Errorf("fork's first ledger write: S-%d = %q, want done_unverified", i, got)
		}
	}
	if current != "" {
		t.Errorf("fork's first ledger write: current = %q, want \"\" (nothing left to drive)", current)
	}
	running := w.stateEvent(fork, run.StateRunning)
	if d := detailOf(running); fmt.Sprint(d["resume_step"]) != "" && fmt.Sprint(d["resume_step"]) != "<nil>" {
		t.Errorf("fork running.detail.resume_step = %q, want absent (no step to drive)", d["resume_step"])
	}
}

// TestTQ2StepCloseRecordsEvidenceSnapshot: with no crash at all, every plan
// step's close takes a platform snapshot commit and records it as the done
// item's evidence_ref (Spec S13.5 "at stage/round boundaries"; S05.1 §4).
// Each snapshot's tree holds exactly the files of steps 1..i.
func TestTQ2StepCloseRecordsEvidenceSnapshot(t *testing.T) {
	const n = 3
	ctx := context.Background()
	w := newTQ2World(t, n, 0, "", true)
	taskID := w.walkToExecute(ctx, true)
	if got := w.state(taskID + ".execute"); got != run.StateCompleted {
		t.Fatalf("execute is %s, want completed", got)
	}
	doc, _, err := w.led.Current(ctx, taskID)
	if err != nil {
		t.Fatalf("ledger Current: %v", err)
	}
	e, _ := w.proj.Get(ctx, "shop")
	seen := map[string]bool{}
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("S-%d", i)
		var item *ledger.WorkItem
		for j := range doc.State.Items {
			if doc.State.Items[j].ID == id {
				item = &doc.State.Items[j]
			}
		}
		if item == nil {
			t.Fatalf("no work item %s", id)
		}
		if item.Status != ledger.StatusDoneUnverified {
			t.Errorf("%s status = %s, want done_unverified", id, item.Status)
		}
		if item.EvidenceRef == "" {
			t.Errorf("%s carries no evidence_ref: the step's close snapshot is not recorded", id)
			continue
		}
		if seen[item.EvidenceRef] {
			t.Errorf("%s evidence_ref %s repeats an earlier step's (each close is its own snapshot)", id, item.EvidenceRef)
		}
		seen[item.EvidenceRef] = true
		listing := tq2GitOut(e.StorePath, "ls-tree", "-r", "--name-only", item.EvidenceRef)
		var got []string
		for _, l := range strings.Split(listing, "\n") {
			if l != "" {
				got = append(got, l)
			}
		}
		sort.Strings(got)
		if a, b := strings.Join(got, ","), strings.Join(stepFiles(i), ","); a != b {
			t.Errorf("%s evidence snapshot tree = [%s], want [%s]", id, a, b)
		}
	}
}

// TestTQ2WorkspaceLessForkStartsOverHonestly: a task with no registered
// project has no snapshots, so nothing can be resumed: the fork starts at S-1
// in its own fresh run dir (today's behaviour, kept), calls no restore, and
// SAYS so in its running transition rather than pretending to resume.
func TestTQ2WorkspaceLessForkStartsOverHonestly(t *testing.T) {
	const n = 2
	ctx := context.Background()
	w := newTQ2World(t, n, 2, tq2CrashMid, false)
	taskID := w.walkToExecute(ctx, false)
	parent := taskID + ".execute"
	if got := w.state(parent); got != run.StateCrashed {
		t.Fatalf("%s is %s, want crashed", parent, got)
	}
	fork := w.forkOf(ctx, parent)
	if n := w.tick(ctx); n != 1 {
		t.Fatalf("fork tick dispatched %d", n)
	}
	starts := w.adapter.execStarts(fork)
	if len(starts) != n || starts[0].Step != "S-1" {
		t.Fatalf("workspace-less fork drove %d sessions starting at %q, want %d from S-1", len(starts), starts[0].Step, n)
	}
	if rs := w.adapter.restoresFor(fork); len(rs) != 0 {
		t.Errorf("RestoreWorkspace was called on a workspace-less run: %+v", rs)
	}
	running := w.stateEvent(fork, run.StateRunning)
	if running == nil {
		t.Fatalf("no claimed→running transition recorded for %s", fork)
	}
	d := detailOf(running)
	if got := fmt.Sprint(d["resume_step"]); got != "S-1" {
		t.Errorf("fork running.detail.resume_step = %q, want S-1", got)
	}
	if got := joinAny(d["completed_steps"]); got != "" {
		t.Errorf("fork running.detail.completed_steps = [%s], want none", got)
	}
	reason := fmt.Sprint(running["reason"])
	if !strings.Contains(reason, "S-1") || !strings.Contains(reason, parent) {
		t.Errorf("fork running.reason must say it starts at S-1 and name the run it follows, got %q", reason)
	}
	if got := w.state(fork); got != run.StateCompleted {
		t.Errorf("fork %s is %s, want completed", fork, got)
	}
}

package verify_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// The S07 acceptance battery harness: a real platform.db with the ratified
// schema, the real event log, the real ledger — model duties and the
// sandbox behind scripted fakes.

type fix struct {
	t      *testing.T
	db     *storage.DB
	log    *eventlog.Log
	runs   *run.Store
	ledger *ledger.Store
	reg    *settings.Registry
}

func newFix(t *testing.T) *fix {
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
	return &fix{t: t, db: db, log: log, runs: run.NewStore(db, log), ledger: ledger.NewStore(db, log), reg: reg}
}

func (f *fix) task(taskID, userID string) {
	f.t.Helper()
	err := f.db.WriteTx(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`INSERT INTO tasks (task_id, user_id, title, created_ts) VALUES (?, ?, ?, ?)`,
			taskID, userID, "t", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		f.t.Fatalf("insert task: %v", err)
	}
}

func (f *fix) run(runID, taskID string) run.Run {
	f.t.Helper()
	r, err := f.runs.Create(context.Background(), run.NewRun{
		ID: runID, UserID: "u1", TaskID: taskID, Substrate: "claude-cli", Lane: "anthropic",
	})
	if err != nil {
		f.t.Fatalf("create run: %v", err)
	}
	return r
}

// seedTask creates task + run + a frozen two-criterion objective and one
// done_unverified work item covering AC-1 and AC-2.
func (f *fix) seedTask(taskID, runID string) {
	f.t.Helper()
	ctx := context.Background()
	f.task(taskID, "u1")
	f.run(runID, taskID)
	_, err := f.ledger.SetObjective(ctx, runID, ledger.AuthorHuman, ledger.ObjectiveAC{
		Objective: "Ship the greeting demo",
		AcceptanceCriteria: []ledger.AcceptanceCriterion{
			{N: 1, Plain: "the demo runs cleanly"},
			{N: 2, Plain: "the output includes the greeting", Structured: "stdout contains 'hello'"},
		},
		SpecVersion: "spec-v1",
	})
	if err != nil {
		f.t.Fatalf("SetObjective: %v", err)
	}
	_, err = f.ledger.SessionVerbs(runID, "execute", 0).State(ctx, ledger.StateUpdate{
		Upserts: []ledger.WorkItem{{
			ID: "w1", Summary: "implement the greeting", ACRefs: []int{1, 2},
			Status: ledger.StatusDoneUnverified,
		}},
	})
	if err != nil {
		f.t.Fatalf("seed work item: %v", err)
	}
}

// testSettings overlays test values on the registry defaults through the
// narrow verify.Settings interface.
type testSettings struct {
	base   *settings.Registry
	ints   map[string]int64
	floats map[string]float64
	strs   map[string]string
}

func (s testSettings) Int(key string) (int64, error) {
	if v, ok := s.ints[key]; ok {
		return v, nil
	}
	return s.base.Int(key)
}

func (s testSettings) Float(key string) (float64, error) {
	if v, ok := s.floats[key]; ok {
		return v, nil
	}
	return s.base.Float(key)
}

func (s testSettings) String(key string) (string, error) {
	if v, ok := s.strs[key]; ok {
		return v, nil
	}
	return s.base.String(key)
}

// ---- Scripted judge ----

type fakeJudge struct {
	compliance func(in verify.JudgeInput) (verify.Axis1Result, error)
	sanity     func(in verify.JudgeInput) (verify.Axis2Result, error)

	complianceCalls int
	sanityCalls     int
	inputs          []verify.JudgeInput
}

func (j *fakeJudge) Compliance(_ context.Context, in verify.JudgeInput) (verify.Axis1Result, error) {
	j.complianceCalls++
	j.inputs = append(j.inputs, in)
	if j.compliance == nil {
		return passAll(in), nil
	}
	return j.compliance(in)
}

func (j *fakeJudge) Sanity(_ context.Context, in verify.JudgeInput) (verify.Axis2Result, error) {
	j.sanityCalls++
	if j.sanity == nil {
		return cleanSanity(), nil
	}
	return j.sanity(in)
}

func (j *fakeJudge) Meta() verify.JudgeMeta {
	return verify.JudgeMeta{Model: "fake-judge-1", SelfFamily: true}
}

// passAll returns a passing axis-1 result with the whole artifact as the
// extractive evidence quote (a valid substring by construction).
func passAll(in verify.JudgeInput) verify.Axis1Result {
	var out verify.Axis1Result
	for _, ac := range in.ACs {
		out.Verdicts = append(out.Verdicts, verify.ACVerdict{
			Key: acKey(ac.N), Pass: true, Evidence: in.Artifact,
		})
	}
	return out
}

func cleanSanity() verify.Axis2Result {
	return verify.Axis2Result{ProbeNotes: map[verify.Probe]string{
		verify.ProbeReasonableUser:       "done and good",
		verify.ProbeImplicitExpectations: "nothing absent",
		verify.ProbeSideEffects:          "no unrequested changes",
		verify.ProbeExpertStandard:       "competent",
	}}
}

func acKey(n int) string {
	return fmt.Sprintf("AC-%d", n)
}

// ---- Scripted check runner ----

type scriptRunner struct {
	exits map[string]int // check id → exit code (missing = 0)
	errs  map[string]error
	calls []string
}

func (r *scriptRunner) RunCheck(_ context.Context, req verify.CheckRequest) (verify.CheckResult, error) {
	r.calls = append(r.calls, req.Check.ID)
	if err := r.errs[req.Check.ID]; err != nil {
		return verify.CheckResult{}, err
	}
	return verify.CheckResult{ExitCode: r.exits[req.Check.ID], EvidenceRef: "evidence/" + req.Check.ID + ".log"}, nil
}

// passPack is a minimal valid software pack: one static build check and one
// acceptance check executing AC-2's structured sub-line.
func passPack() *verify.CheckPack {
	return &verify.CheckPack{
		Domain: verify.DomainSoftware, Version: 1, VerifiedOn: time.Now().Add(-24 * time.Hour),
		Checks: []verify.Check{
			{ID: "build", Stage: verify.StageStatic, Argv: []string{"true"}, StepID: "S-1", FindingCategory: verify.CatACBlocker},
			{ID: "ac2", Stage: verify.StageUnit, Argv: []string{"true"}, StepID: "S-1", ACKey: "AC-2",
				Provenance: "authored in a separate session from the frozen AC-2 sub-line (S07.3 rule 4)", FindingCategory: verify.CatACBlocker},
		},
	}
}

func steps() []intake.Step {
	return []intake.Step{{ID: "S-1", Title: "implement", DoneWhen: "checks pass", Class: "C1"}}
}

func spec() intake.Spec {
	return intake.Spec{
		TaskID: "t1", Owner: "u1", Version: 1, Status: intake.StatusApproved,
		Tier: intake.TierStandard, Provenance: "test",
		Restatement: "ship the greeting demo",
		ACs: []intake.AC{
			{N: 1, Plain: "the demo runs cleanly"},
			{N: 2, Plain: "the output includes the greeting", Structured: "stdout contains 'hello'"},
		},
	}
}

// verifier assembles a Verifier over the fixture with a scripted judge and
// runner.
func (f *fix) verifier(j *fakeJudge, r verify.CheckRunner, pack *verify.CheckPack) *verify.Verifier {
	return &verify.Verifier{
		DB: f.db, Log: f.log, Ledger: f.ledger,
		Settings: testSettings{base: f.reg},
		Judge:    j, Runner: r, Pack: pack,
	}
}

func input(d verify.Deliverable) verify.VerifyInput {
	return verify.VerifyInput{
		Deliverable: d, Spec: spec(), Steps: steps(), Tier: intake.TierStandard,
		Workspace: "unused-dev-workspace", EvidenceDir: "unused-evidence",
	}
}

func deliverable(taskID, runID string) verify.Deliverable {
	return verify.Deliverable{
		TaskID: taskID, RunID: runID, Domain: verify.DomainSoftware, Type: "code",
		Revision: 1, Content: "package main\n\nfunc main() { println(\"hello\") }\n",
	}
}

// openAsks returns the open ask rows (ask_id → decoded card).
func (f *fix) openAsks() map[string]verify.Card {
	f.t.Helper()
	rows, err := f.db.QueryContext(context.Background(),
		`SELECT ask_id, snapshot FROM asks WHERE status = 'open'`)
	if err != nil {
		f.t.Fatalf("read asks: %v", err)
	}
	defer rows.Close()
	out := map[string]verify.Card{}
	for rows.Next() {
		var id, snap string
		if err := rows.Scan(&id, &snap); err != nil {
			f.t.Fatalf("scan ask: %v", err)
		}
		var c verify.Card
		if err := json.Unmarshal([]byte(snap), &c); err != nil {
			f.t.Fatalf("decode card %s: %v", id, err)
		}
		out[id] = c
	}
	return out
}

// oneOpenCard asserts exactly one open ask and returns it.
func (f *fix) oneOpenCard() verify.Card {
	f.t.Helper()
	asks := f.openAsks()
	if len(asks) != 1 {
		f.t.Fatalf("want exactly 1 open ask, have %d: %v", len(asks), asks)
	}
	for _, c := range asks {
		return c
	}
	panic("unreachable")
}

// events returns all events of a type, in seq order.
func (f *fix) events(eventType string) []eventlog.Event {
	f.t.Helper()
	all, err := f.log.After(context.Background(), 0, 10000)
	if err != nil {
		f.t.Fatalf("read events: %v", err)
	}
	var out []eventlog.Event
	for _, e := range all {
		if e.Type == eventType {
			out = append(out, e)
		}
	}
	return out
}

// fakeConfiner records the confinement request and swaps the argv for a
// hermetic /bin/sh execution so the runner's platform-side verdict
// derivation is observable.
type fakeConfiner struct {
	req  adapters.StartRequest
	spec adapters.SpawnSpec
}

func (c *fakeConfiner) Confine(req adapters.StartRequest, spec adapters.SpawnSpec) (*exec.Cmd, func(), error) {
	c.req, c.spec = req, spec
	return exec.Command("sh", "-c", "echo check-output; exit "+spec.Argv[len(spec.Argv)-1]), func() {}, nil
}

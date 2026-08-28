package intake_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// The S06 acceptance battery: dev-mode end-to-end to an approved plan with
// durable artifacts, ledger writes through the control-plane ops, and the
// S02.8 claim row — plus the forced escalation routes (P46: coverage and
// SPEC-DOUBT are covered by end-to-end tests that force them), the
// zero-interaction band, deterministic floors, resume-from-artifact, and
// the delta re-approval path.

// ---- Fakes (the S06.10 seams) ----

type fakePlanner struct {
	draft       func(in intake.DraftInput) (intake.Pair, error)
	revise      func(in intake.ReviseInput) (intake.Pair, error)
	draftCalls  int
	reviseCalls int
}

func (f *fakePlanner) Draft(_ context.Context, in intake.DraftInput) (intake.Pair, error) {
	f.draftCalls++
	if f.draft != nil {
		return f.draft(in)
	}
	return basePair(in), nil
}

func (f *fakePlanner) Revise(_ context.Context, in intake.ReviseInput) (intake.Pair, error) {
	f.reviseCalls++
	if f.revise != nil {
		return f.revise(in)
	}
	return baseRevise(in), nil
}

// basePair emits an honest S06.6 pair from the draft input.
func basePair(in intake.DraftInput) intake.Pair {
	writes := []string{"src/**"}
	est := intake.Estimate{SizeClass: "S", USD: 1.0, Known: true, Basis: "fake"}
	if in.Tier == intake.TierTrivial {
		writes = nil // the band's degenerate read-only plan (S06.4)
		est = intake.Estimate{SizeClass: "XS", USD: 0.10, Known: true, Basis: "fake"}
	}
	// The [A15] per-step approach is REQUIRED at the artifact boundary
	// (P3-GF8 R11), so the fixture planner emits one — an honest one, in the
	// plain words a real emission owes. No assertion changes.
	steps := []intake.Step{
		{ID: "S-1", Title: "Implement the change", DoneWhen: "tests pass", Class: "C1", WriteSet: writes,
			Approach: "I change the widget code in place and leave everything around it alone.",
			Decisions: []intake.StepDecision{{
				Decision:     "edit the existing module rather than add a new one",
				Alternatives: []string{"add a second module beside it"},
				Why:          "one place to look is easier to keep right than two",
			}}},
		{ID: "S-2", Title: "Verify against the criteria", DoneWhen: "all ACs demonstrably hold", Class: "C1",
			Approach:          "I run the project's own checks and read each acceptance criterion against what they report.",
			OrderingRationale: "nothing can be verified before the change exists"},
	}
	var nodes []intake.ResearchNode
	if len(in.DataHits) > 0 {
		steps[0].Research = true
		for _, h := range in.DataHits {
			nodes = append(nodes, intake.ResearchNode{RuleID: h.RuleID, StepID: "S-1", Query: "verify " + h.Class})
		}
	}
	var assumptions []intake.Assumption
	for _, r := range in.Resolutions {
		if r.How == intake.ResolvedAssumption {
			assumptions = append(assumptions, intake.Assumption{Text: r.Assumption, Origin: "slot:" + r.SlotID})
		}
	}
	return intake.Pair{
		Spec: intake.Spec{
			Provenance:  "fake-planner v1",
			Restatement: "Requester wants: " + in.Request.Title,
			Outcome:     []string{"the requester recognizes their goal"},
			ACs: []intake.AC{
				{N: 1, Plain: "the change works", Structured: "WHEN run THEN exit 0", StructuredKind: "ears"},
				{N: 2, Plain: "the result is verified"},
			},
			Constraints: []string{"stay within the repo"},
			Assumptions: assumptions,
			OutOfScope:  []string{"no deploys"},
		},
		Plan: intake.Plan{
			Provenance:    "fake-planner v1",
			Steps:         steps,
			Coverage:      map[string][]string{"AC-1": {"S-1"}, "AC-2": {"S-2"}},
			ResearchNodes: nodes,
			Risks:         []string{"estimate may be off"},
			Est:           est,
		},
	}
}

// baseRevise addresses exactly the named findings (criteria never drift).
func baseRevise(in intake.ReviseInput) intake.Pair {
	p := in.Pair
	switch in.Reason {
	case intake.ReviseCoverage:
		if p.Plan.Coverage == nil {
			p.Plan.Coverage = map[string][]string{}
		}
		for _, key := range in.Findings {
			p.Plan.Coverage[key] = []string{"S-2"}
		}
	case intake.ReviseResearch:
		for _, rule := range in.Findings {
			p.Plan.ResearchNodes = append(p.Plan.ResearchNodes, intake.ResearchNode{RuleID: rule, StepID: "S-1"})
		}
		p.Plan.Steps[0].Research = true
	case intake.ReviseDrop:
		var kept []intake.AC
		drop := map[string]bool{}
		for _, k := range in.Findings {
			drop[k] = true
		}
		cov := map[string][]string{}
		for _, ac := range p.Spec.ACs {
			if drop[ac.Key()] {
				continue
			}
			steps := p.Plan.Coverage[ac.Key()]
			ac.N = len(kept) + 1
			kept = append(kept, ac)
			cov[ac.Key()] = steps
		}
		p.Spec.ACs = kept
		p.Plan.Coverage = cov
	case intake.ReviseMarkers:
		p.Spec.Clarifications = nil
	}
	return p
}

type fakeClassifier struct {
	prop intake.TriageProposal
	err  error
}

func (f *fakeClassifier) Classify(context.Context, intake.TriageInput) (intake.TriageProposal, error) {
	return f.prop, f.err
}

type fakeCritic struct {
	verdicts []intake.Verdict // consumed per Critique call
	recheck  func(pair intake.Pair, findings []string) (intake.Verdict, error)
	calls    int
}

func (f *fakeCritic) Critique(_ context.Context, _ intake.Pair) (intake.Verdict, error) {
	f.calls++
	if len(f.verdicts) == 0 {
		return intake.Verdict{Kind: intake.VerdictPass}, nil
	}
	v := f.verdicts[0]
	f.verdicts = f.verdicts[1:]
	return v, nil
}

func (f *fakeCritic) Recheck(_ context.Context, pair intake.Pair, findings []string) (intake.Verdict, error) {
	if f.recheck != nil {
		return f.recheck(pair, findings)
	}
	return intake.Verdict{Kind: intake.VerdictPass}, nil
}

type fakeRegistry struct {
	slice intake.RegistrySlice
	ok    bool
}

func (f *fakeRegistry) Match(context.Context, intake.Request) (intake.RegistrySlice, bool, error) {
	return f.slice, f.ok, nil
}

// overrideSettings overrides string keys over the real registry.
type overrideSettings struct {
	intake.Settings
	strings map[string]string
}

func (o overrideSettings) String(key string) (string, error) {
	if v, ok := o.strings[key]; ok {
		return v, nil
	}
	return o.Settings.String(key)
}

// ---- Fixture ----

type fix struct {
	t       *testing.T
	db      *storage.DB
	log     *eventlog.Log
	runs    *run.Store
	led     *ledger.Store
	p       *intake.Pipeline
	planner *fakePlanner
	critic  *fakeCritic
	class   *fakeClassifier
	now     time.Time
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
	f := &fix{
		t: t, db: db, log: log,
		runs:    run.NewStore(db, log),
		led:     ledger.NewStore(db, log),
		planner: &fakePlanner{},
		critic:  &fakeCritic{},
		class: &fakeClassifier{prop: intake.TriageProposal{
			Family: intake.FamilySoftware, Tier: intake.TierStandard,
			Est: intake.Estimate{SizeClass: "S", USD: 1.0, Known: true, Basis: "fake"},
		}},
		now: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
	}
	f.p = &intake.Pipeline{
		DB: db, Log: log, Runs: f.runs, Ledger: f.led, Settings: reg,
		ArtifactRoot: filepath.Join(t.TempDir(), "artifacts"),
		Planner:      f.planner, Critic: f.critic, Classifier: f.class,
		Now: func() time.Time { return f.now },
	}
	return f
}

func (f *fix) start(req intake.Request) *intake.State {
	f.t.Helper()
	st, err := f.p.Start(context.Background(), req)
	if err != nil {
		f.t.Fatalf("Start: %v", err)
	}
	return st
}

// admit walks the intake run to running — the scheduler's job at B2-4;
// the dev-mode harness drives the ratified FSM edges directly.
func (f *fix) admit(runID string) {
	f.t.Helper()
	ctx := context.Background()
	for _, s := range []run.State{run.StateQueued, run.StateClaimed, run.StateRunning} {
		if _, err := f.runs.Transition(ctx, runID, s, run.TransitionOptions{
			Reason: "test admission", Actor: run.ActorPlatform,
		}); err != nil {
			f.t.Fatalf("admit %s→%s: %v", runID, s, err)
		}
	}
}

func (f *fix) advance(taskID string) *intake.State {
	f.t.Helper()
	st, err := f.p.Advance(context.Background(), taskID)
	if err != nil {
		f.t.Fatalf("Advance: %v", err)
	}
	return st
}

func (f *fix) answer(owner, askID string, ans any) *intake.State {
	f.t.Helper()
	raw, err := json.Marshal(ans)
	if err != nil {
		f.t.Fatal(err)
	}
	st, err := f.p.Answer(context.Background(), owner, askID, raw)
	if err != nil {
		f.t.Fatalf("Answer(%s): %v", askID, err)
	}
	return st
}

func (f *fix) openAsk(runID string) (string, intake.Card) {
	f.t.Helper()
	var askID, snapshot string
	err := f.db.QueryRowContext(context.Background(),
		`SELECT ask_id, snapshot FROM asks WHERE run_id = ? AND status = 'open'`, runID).
		Scan(&askID, &snapshot)
	if err != nil {
		f.t.Fatalf("open ask for %s: %v", runID, err)
	}
	var card intake.Card
	if err := json.Unmarshal([]byte(snapshot), &card); err != nil {
		f.t.Fatal(err)
	}
	return askID, card
}

func (f *fix) runState(runID string) run.State {
	f.t.Helper()
	r, err := f.runs.Get(context.Background(), runID)
	if err != nil {
		f.t.Fatal(err)
	}
	return r.State
}

func (f *fix) ledgerDoc(taskID string) ledger.Document {
	f.t.Helper()
	doc, found, err := f.led.Current(context.Background(), taskID)
	if err != nil || !found {
		f.t.Fatalf("ledger doc: found=%v err=%v", found, err)
	}
	return doc
}

// answerInterviewToFloor answers every asked question until the interview
// stops issuing cards.
func (f *fix) answerInterviewToFloor(owner, runID string) *intake.State {
	f.t.Helper()
	for i := 0; i < 10; i++ {
		askID, card := f.openAsk(runID)
		if card.Kind != intake.CardInterview {
			st, err := f.p.LoadState(context.Background(), card.TaskID)
			if err != nil {
				f.t.Fatal(err)
			}
			return st
		}
		var answers []intake.SlotAnswer
		for _, q := range card.Questions {
			answers = append(answers, intake.SlotAnswer{ID: q.ID, Value: "answered: " + q.ID})
		}
		st := f.answer(owner, askID, intake.Answer{Answers: answers})
		if st.OpenAskID == "" || st.OpenAskKind != intake.CardInterview {
			return st
		}
	}
	f.t.Fatal("interview did not settle")
	return nil
}

func stdRequest() intake.Request {
	return intake.Request{UserID: "u1", Title: "Fix the widget", Text: "Please fix the widget in the repo."}
}

// ---- The battery ----

// TestStandardFlowEndToEnd is the packet's headline acceptance: a task
// enters intake and reaches an approved plan with durable artifacts,
// ledger writes through the control-plane ops, and the claim row.
func TestStandardFlowEndToEnd(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()

	// The request carries a P47-7 temporal cue → deterministic rule hit.
	req := stdRequest()
	req.Text = "Please fix the widget using the latest dependency versions."
	st := f.start(req)

	// The deterministic half of Stage 0 runs at Start; classification does not
	// (its $0 D7 row needs a running consuming run — S06.2 on the RUNNING intake
	// run), so the state carries the fail-closed baseline here.
	if st.Tier != intake.TierHigh || st.Family != intake.FamilyGeneric {
		t.Fatalf("pre-classification baseline: tier=%s family=%s", st.Tier, st.Family)
	}
	if len(st.DataBearing) == 0 {
		t.Fatal("P47-7 cue did not set the data-bearing flag (S06.3)")
	}
	if f.runState(st.RunID) != run.StateNew {
		t.Fatal("intake run must start new (admission is the scheduler's)")
	}
	if st.RecordRef == nil {
		t.Fatal("no intake record artifact")
	}
	if _, err := os.Stat(st.RecordRef.Path); err != nil {
		t.Fatalf("intake record file: %v", err)
	}
	doc := f.ledgerDoc(st.TaskID)
	if len(doc.Artifacts) != 1 || doc.Artifacts[0].Kind != "intake-record" || doc.Artifacts[0].ProducingStage != "triage" {
		t.Fatalf("Stage-0 ledger artifacts: %+v", doc.Artifacts)
	}
	if len(doc.Decisions) != 0 {
		t.Fatalf("Stage-0 ledger decisions before classification: %+v", doc.Decisions)
	}

	// Stage work refuses to run before admission.
	if _, err := f.p.Advance(ctx, st.TaskID); !errors.Is(err, intake.ErrNotRunning) {
		t.Fatalf("Advance before admission: %v", err)
	}
	f.admit(st.RunID)

	// The first advance classifies on the running run, then interviews:
	// standard floor 75, all slots unresolved → card, run parks.
	st = f.advance(st.TaskID)
	if st.Tier != intake.TierStandard || st.Family != intake.FamilySoftware {
		t.Fatalf("triage: tier=%s family=%s", st.Tier, st.Family)
	}
	// Record v2 joins v1 (both immutable), and the triage decision lands where
	// the classified values became facts.
	doc = f.ledgerDoc(st.TaskID)
	if len(doc.Artifacts) != 2 || doc.Artifacts[1].Kind != "intake-record" || doc.Artifacts[1].ProducingStage != "triage" {
		t.Fatalf("Stage-0 ledger artifacts after classification: %+v", doc.Artifacts)
	}
	if len(doc.Decisions) != 1 || doc.Decisions[0].Author != ledger.AuthorPlatform ||
		!strings.Contains(doc.Decisions[0].Text, "family=software") {
		t.Fatalf("Stage-0 ledger decisions: %+v", doc.Decisions)
	}
	if st.OpenAskKind != intake.CardInterview {
		t.Fatalf("expected interview card, got %q", st.OpenAskKind)
	}
	if f.runState(st.RunID) != run.StateParked {
		t.Fatal("gate open must park the run (gates wait, S06.1)")
	}
	askID, card := f.openAsk(st.RunID)
	if len(card.Questions) == 0 || len(card.Questions) > 4 {
		t.Fatalf("interview card carries %d questions (S06.5: up to 4)", len(card.Questions))
	}
	// Highest-weight unresolved slots come first (ClarifyCodeBench
	// weighting: the empirically failed types lead).
	if card.Questions[0].Weight < card.Questions[len(card.Questions)-1].Weight {
		t.Fatal("questions not ordered by weight")
	}
	_ = askID

	// Answer through the floor; the pipeline continues in place: draft →
	// spine → critique (PASS) → approval card.
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("expected approval card, got %q (phase %s)", st.OpenAskKind, st.Phase)
	}
	if f.critic.calls != 1 {
		t.Fatalf("critique calls = %d, want 1 (standard tier, one pass)", f.critic.calls)
	}
	if st.Clearance < 75 {
		t.Fatalf("clearance %v below the standard floor", st.Clearance)
	}

	// The artifact pair is durable, draft status, with research node.
	askID, card = f.openAsk(st.RunID)
	if card.Approval == nil || len(card.Approval.Layer1.Assumptions) == 0 && card.Approval.Layer1.Restatement == "" {
		t.Fatal("approval card missing the content contract")
	}
	if len(card.Approval.Layer2.ResearchNodes) == 0 {
		t.Fatal("research node missing from the plan (1.9/P47 policy)")
	}
	if got := card.Approval.Actions; len(got) != 4 {
		t.Fatalf("approval actions %v", got)
	}

	// Approve (D10: the requester).
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionApprove})
	if st.Phase != intake.PhaseApproved {
		t.Fatalf("phase %s", st.Phase)
	}

	// On Approve (S06.9): objective + frozen ACs + constraints pinned;
	// spec+plan version recorded; approval decision present.
	doc = f.ledgerDoc(st.TaskID)
	if doc.SpecVersion != "spec-v1" || doc.ObjectiveAC.Objective == "" {
		t.Fatalf("objective_ac not pinned: %+v", doc.ObjectiveAC)
	}
	if len(doc.ObjectiveAC.AcceptanceCriteria) != 2 {
		t.Fatalf("frozen ACs: %+v", doc.ObjectiveAC.AcceptanceCriteria)
	}
	if doc.Constraints.SpecVersion != "spec-v1" {
		t.Fatalf("constraints not pinned: %+v", doc.Constraints)
	}
	if doc.PlanVersion != "spec-v1/plan-v1" {
		t.Fatalf("plan version %q", doc.PlanVersion)
	}
	foundApproval := false
	for _, d := range doc.Decisions {
		if d.Author == ledger.AuthorHuman && strings.Contains(d.Text, "plan approved") {
			foundApproval = true
		}
	}
	if !foundApproval {
		t.Fatalf("no approval decision: %+v", doc.Decisions)
	}

	// The S02.8 claim row.
	var globs, mode, status, declaredAt string
	err := f.db.QueryRowContext(ctx,
		`SELECT path_globs, mode, status, declared_at_plan FROM artifact_claims WHERE task_id = ?`, st.TaskID).
		Scan(&globs, &mode, &status, &declaredAt)
	if err != nil {
		t.Fatalf("claim row: %v", err)
	}
	if mode != "W" || status != "active" || declaredAt != "plan-v1" || !strings.Contains(globs, "src/**") {
		t.Fatalf("claim row: globs=%s mode=%s status=%s declared=%s", globs, mode, status, declaredAt)
	}

	// Approved artifacts are frozen as new immutable files.
	if !strings.HasSuffix(st.SpecRef.Path, ".approved.md") {
		t.Fatalf("spec ref %s", st.SpecRef.Path)
	}
	if _, err := os.Stat(st.SpecRef.Path); err != nil {
		t.Fatal(err)
	}
	approvedMD, _ := os.ReadFile(st.SpecRef.Path)
	if !strings.Contains(string(approvedMD), "status: approved") {
		t.Fatal("approved spec not marked approved")
	}

	// Approval resumes the run (S02.3 approval resume trigger).
	if f.runState(st.RunID) != run.StateRunning {
		t.Fatalf("run %s after approve", f.runState(st.RunID))
	}
	r, _ := f.runs.Get(ctx, st.RunID)
	if r.Generation == 0 {
		t.Fatal("resume must bump the generation")
	}
}

// TestFailClosedTriage: no classifier, or a failing one, is high stakes
// (S06.2) — at the classify step, which is where the seam is now called.
func TestFailClosedTriage(t *testing.T) {
	f := newFix(t)
	f.p.Classifier = nil
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if st.Tier != intake.TierHigh || st.Band {
		t.Fatalf("nil classifier: tier=%s band=%v", st.Tier, st.Band)
	}
	if st.TriagePending {
		t.Fatal("the classify marker survived a fail-closed classify — a failed attempt is not a retry loop")
	}

	f2 := newFix(t)
	f2.class.err = errors.New("alias down")
	st = f2.start(stdRequest())
	f2.admit(st.RunID)
	st = f2.advance(st.TaskID)
	if st.Tier != intake.TierHigh {
		t.Fatalf("failing classifier: tier=%s", st.Tier)
	}
	if st.TriagePending {
		t.Fatal("the classify marker survived a failing classifier — the seam would be re-called every advance")
	}
}

// TestZeroInteractionBand: the trivial band runs the whole pipeline with
// no cards and no blocking gate; the ungated SPEC and degenerate PLAN
// exist in the ledger (S06.4).
func TestZeroInteractionBand(t *testing.T) {
	f := newFix(t)
	f.class.prop = intake.TriageProposal{
		Family: intake.FamilySoftware, Tier: intake.TierTrivial, ReadOnly: true,
		Est: intake.Estimate{SizeClass: "XS", USD: 0.10, Known: true, Basis: "fake"},
	}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if !st.Band || st.Tier != intake.TierTrivial {
		t.Fatalf("band=%v tier=%s", st.Band, st.Tier)
	}
	if st.Phase != intake.PhaseApproved {
		t.Fatalf("phase %s", st.Phase)
	}
	if st.Approval == nil || !st.Approval.Ungated {
		t.Fatalf("approval record %+v", st.Approval)
	}
	if f.critic.calls != 0 {
		t.Fatal("trivial tier must not critique (S06.4)")
	}
	if f.runState(st.RunID) != run.StateRunning {
		t.Fatal("band task must never park (no blocking gate)")
	}
	// Read-only plan → no write claim.
	var n int
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM artifact_claims WHERE task_id = ?`, st.TaskID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("read-only band plan wrote %d claims", n)
	}
	doc := f.ledgerDoc(st.TaskID)
	if doc.SpecVersion != "spec-v1" || doc.PlanVersion != "spec-v1/plan-v1" {
		t.Fatalf("ungated pins missing: %q %q", doc.SpecVersion, doc.PlanVersion)
	}
	// Unresolved must-know slots became auto-listed assumptions.
	spec := readSpecSidecar(t, st.SpecRef.Path)
	if len(spec.Assumptions) == 0 {
		t.Fatal("band SPEC lists no auto-assumptions")
	}
}

func readSpecSidecar(t *testing.T, mdPath string) intake.Spec {
	t.Helper()
	raw, err := os.ReadFile(strings.TrimSuffix(mdPath, ".md") + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var s intake.Spec
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestDeterministicFloors: floors force high regardless of the classifier
// proposal; a data-bearing cue alone breaks band membership (S06.2/S06.4).
func TestDeterministicFloors(t *testing.T) {
	f := newFix(t)
	f.class.prop = intake.TriageProposal{
		Family: intake.FamilySoftware, Tier: intake.TierTrivial, ReadOnly: true,
		Est:    intake.Estimate{USD: 0.10, Known: true},
		Floors: []intake.FloorReason{{Class: intake.FloorOutwardEffect, Source: "classifier", Detail: "sends email"}},
	}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if st.Tier != intake.TierHigh || st.Band {
		t.Fatalf("floor: tier=%s band=%v", st.Tier, st.Band)
	}
	if st.FloorTier != intake.TierHigh {
		t.Fatal("floor tier not recorded")
	}

	// Same proposal, no floor, but a data-bearing cue → band condition 2
	// fails; trivial-but-not-band lands at low.
	f2 := newFix(t)
	f2.class.prop = intake.TriageProposal{
		Family: intake.FamilySoftware, Tier: intake.TierTrivial, ReadOnly: true,
		Est: intake.Estimate{USD: 0.10, Known: true},
	}
	req := stdRequest()
	req.Text = "check the latest widget price"
	st = f2.start(req)
	f2.admit(st.RunID)
	st = f2.advance(st.TaskID)
	if st.Band || st.Tier != intake.TierLow {
		t.Fatalf("data-bearing: band=%v tier=%s", st.Band, st.Tier)
	}
}

// TestLowTierSkipsCritique: low tier is spine-only (S06.4).
func TestLowTierSkipsCritique(t *testing.T) {
	f := newFix(t)
	f.class.prop.Tier = intake.TierLow
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("expected approval card, got %q", st.OpenAskKind)
	}
	if f.critic.calls != 0 {
		t.Fatalf("low tier critiqued %d times", f.critic.calls)
	}
}

// TestCoverageEscalationForced is the P46-mandated end-to-end test of the
// coverage escalation route: uncovered AC → bounded auto-fix → decision
// card (S06.7(a), 5.6).
func TestCoverageEscalationForced(t *testing.T) {
	f := newFix(t)
	broken := func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		delete(p.Plan.Coverage, "AC-2") // never covered
		return p, nil
	}
	f.planner.draft = broken
	f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
		return in.Pair, nil // the auto-fix round fails to fix
	}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardCoverage {
		t.Fatalf("expected coverage decision card, got %q", st.OpenAskKind)
	}
	if f.planner.reviseCalls != 1 {
		t.Fatalf("auto-fix rounds = %d, want ⚙ intake.coverage_autofix_rounds = 1", f.planner.reviseCalls)
	}
	askID, card := f.openAsk(st.RunID)
	if len(card.Decision.Choices) != 3 {
		t.Fatalf("coverage choices %+v", card.Decision.Choices)
	}

	// The requester accepts the gap explicitly; it rides the approval card.
	st = f.answer("u1", askID, intake.Answer{Choice: intake.ChoiceProceedUncovered})
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("after proceed_uncovered: %q", st.OpenAskKind)
	}
	_, card = f.openAsk(st.RunID)
	if len(card.Approval.Layer1.Uncovered) == 0 {
		t.Fatal("accepted gap not listed on the approval card")
	}
}

// TestCoverageDropCriterion: the explicit requester drop re-emits the spec
// without the criterion — recorded, never silent (1.4).
func TestCoverageDropCriterion(t *testing.T) {
	f := newFix(t)
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		delete(p.Plan.Coverage, "AC-2")
		return p, nil
	}
	first := true
	f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
		if in.Reason == intake.ReviseCoverage && first {
			first = false
			return in.Pair, nil // auto-fix fails once → card
		}
		return baseRevise(in), nil
	}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	f.answerInterviewToFloor("u1", st.RunID)
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Choice: intake.ChoiceDropCriterion, Criteria: []string{"AC-2"}})
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("after drop: %q", st.OpenAskKind)
	}
	spec := readSpecSidecar(t, st.SpecRef.Path)
	if len(spec.ACs) != 1 {
		t.Fatalf("spec still has %d ACs", len(spec.ACs))
	}
	doc := f.ledgerDoc(st.TaskID)
	foundDrop := false
	for _, d := range doc.Decisions {
		if d.Author == ledger.AuthorHuman && strings.Contains(d.Text, "dropped criteria") {
			foundDrop = true
		}
	}
	if !foundDrop {
		t.Fatal("requester drop not recorded in the ledger")
	}
}

// TestSpecDoubtForced is the P46-mandated end-to-end test of the
// SPEC-DOUBT route: mandatory decision card, never absorbed (S06.8).
func TestSpecDoubtForced(t *testing.T) {
	f := newFix(t)
	f.critic.verdicts = []intake.Verdict{{Kind: intake.VerdictSpecDoubt, Doubt: "the goal duplicates an existing tool"}}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardSpecDoubt {
		t.Fatalf("expected SPEC-DOUBT card, got %q", st.OpenAskKind)
	}
	askID, card := f.openAsk(st.RunID)
	if !strings.Contains(card.Decision.Summary, "duplicates an existing tool") {
		t.Fatal("the plain-language why must reach the requester")
	}
	if len(card.Decision.Choices) != 3 {
		t.Fatalf("choices %+v", card.Decision.Choices)
	}
	st = f.answer("u1", askID, intake.Answer{Choice: intake.ChoiceProceedAnyway})
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("proceed anyway → %q", st.OpenAskKind)
	}
}

// TestCritiqueReviseBounded: REVISE fixes once within ⚙, recheck sees only
// the named findings; exhausted rounds proceed with the critique attached.
func TestCritiqueReviseBounded(t *testing.T) {
	f := newFix(t)
	f.critic.verdicts = []intake.Verdict{{Kind: intake.VerdictRevise, Findings: []string{"1. step S-1 lacks rollback"}}}
	var recheckGot []string
	f.critic.recheck = func(_ intake.Pair, findings []string) (intake.Verdict, error) {
		recheckGot = findings
		return intake.Verdict{Kind: intake.VerdictPass}, nil
	}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("got %q", st.OpenAskKind)
	}
	if len(recheckGot) != 1 || !strings.Contains(recheckGot[0], "rollback") {
		t.Fatalf("recheck saw %v", recheckGot)
	}
	if st.CritiqueRounds != 1 {
		t.Fatalf("critique rounds %d", st.CritiqueRounds)
	}

	// Exhausted: recheck keeps failing → approval carries the findings.
	f2 := newFix(t)
	f2.critic.verdicts = []intake.Verdict{{Kind: intake.VerdictRevise, Findings: []string{"1. no rollback"}}}
	f2.critic.recheck = func(_ intake.Pair, findings []string) (intake.Verdict, error) {
		return intake.Verdict{Kind: intake.VerdictRevise, Findings: findings}, nil
	}
	st2 := f2.start(stdRequest())
	f2.admit(st2.RunID)
	f2.advance(st2.TaskID)
	st2 = f2.answerInterviewToFloor("u1", st2.RunID)
	if st2.OpenAskKind != intake.CardApproval {
		t.Fatalf("got %q", st2.OpenAskKind)
	}
	_, card := f2.openAsk(st2.RunID)
	if len(card.Approval.Layer1.OpenFinds) == 0 {
		t.Fatal("open critique findings must ride the approval card")
	}
}

// TestTierUp: the pipeline re-enters at the new tier's requirements
// (S06.8); a no-op TIER-UP cannot loop.
func TestTierUp(t *testing.T) {
	f := newFix(t)
	f.critic.verdicts = []intake.Verdict{{Kind: intake.VerdictTierUp, ProposedTier: intake.TierHigh}}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	// The answer loop rides through the TIER-UP: the standard floor (75)
	// is met, the raised high floor (90) re-opens the interview, the loop
	// answers it, and a FRESH critique runs at the new tier.
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.Tier != intake.TierHigh {
		t.Fatalf("tier %s", st.Tier)
	}
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("got %q", st.OpenAskKind)
	}
	if st.Clearance < 90 {
		t.Fatalf("clearance %v below the high floor — the tier-up interview did not run", st.Clearance)
	}
	if f.critic.calls != 2 {
		t.Fatalf("critique calls %d, want a fresh pass at the new tier", f.critic.calls)
	}

	// No-op TIER-UP (already high) proceeds with the verdict attached.
	f2 := newFix(t)
	f2.class.prop.Tier = intake.TierHigh
	f2.critic.verdicts = []intake.Verdict{{Kind: intake.VerdictTierUp, ProposedTier: intake.TierHigh}}
	st2 := f2.start(stdRequest())
	f2.admit(st2.RunID)
	f2.advance(st2.TaskID)
	st2 = f2.answerInterviewToFloor("u1", st2.RunID)
	if st2.OpenAskKind != intake.CardApproval {
		t.Fatalf("no-op TIER-UP: %q", st2.OpenAskKind)
	}
}

// TestSizeDelta: an over-guess plan reclassifies before expensive work and
// the finding rides the card (2.5; S06.7(c)).
func TestSizeDelta(t *testing.T) {
	f := newFix(t)
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		p.Plan.Est = intake.Estimate{SizeClass: "L", USD: 5.0, Known: true, Basis: "fake"} // > 1.0 × ⚙ 2.0
		return p, nil
	}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.Spine == nil || st.Spine.SizeFinding != "over" {
		t.Fatalf("spine size finding %+v", st.Spine)
	}
	if st.Guess.USD != 5.0 {
		t.Fatal("size not reclassified from the plan")
	}
	_, card := f.openAsk(st.RunID)
	if card.Approval.Layer1.SizeNote == "" {
		t.Fatal("size finding not surfaced on the card")
	}
}

// TestMarkersBlockApproval: open NEEDS-CLARIFICATION markers route
// through a clarification card and cannot reach approval (S06.6).
func TestMarkersBlockApproval(t *testing.T) {
	f := newFix(t)
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		p.Spec.Clarifications = []string{"Which environment is the target?"}
		return p, nil
	}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardClarification {
		t.Fatalf("expected clarification card, got %q", st.OpenAskKind)
	}
	askID, card := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Answers: []intake.SlotAnswer{{ID: card.Questions[0].ID, Value: "staging"}}})
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("after clarification: %q", st.OpenAskKind)
	}
}

// TestResearchDismissalIsRequesters: a missing research node bounces once,
// then cards; only the requester's supplied fact dismisses the flag
// (S06.3, S06.7(d)).
func TestResearchDismissalIsRequesters(t *testing.T) {
	f := newFix(t)
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		p.Plan.ResearchNodes = nil // policy violation: flag without node
		return p, nil
	}
	f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
		if in.Reason == intake.ReviseResearch {
			return in.Pair, nil // the bounce fails to add the node
		}
		return baseRevise(in), nil
	}
	req := stdRequest()
	req.Text = "use the latest dependency versions"
	st := f.start(req)
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardResearch {
		t.Fatalf("expected research decision card, got %q", st.OpenAskKind)
	}
	askID, _ := f.openAsk(st.RunID)
	// "latest dependency versions" trips both P47-4 (versions) and P47-7
	// (latest); dismissal covers every open flag.
	var facts []intake.SuppliedFact
	for _, rule := range st.Spine.MissingResearch {
		facts = append(facts, intake.SuppliedFact{RuleID: rule, Fact: "dependency X is at 2.4.1 as of today"})
	}
	st = f.answer("u1", askID, intake.Answer{Choice: intake.ChoiceSupplyFact, Facts: facts})
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("after supply_fact: %q", st.OpenAskKind)
	}
	// The supplied input is recorded on the SPEC (S06.3).
	spec := readSpecSidecar(t, st.SpecRef.Path)
	if len(spec.Supplied) == 0 || !strings.Contains(spec.Supplied[0].Fact, "2.4.1") {
		t.Fatalf("supplied inputs on spec: %+v", spec.Supplied)
	}
}

// TestForceProceed: unresolved must-know slots convert into listed
// assumptions on the card's centerpiece (S06.5).
func TestForceProceed(t *testing.T) {
	f := newFix(t)
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{ForceProceed: true})
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("after force-proceed: %q", st.OpenAskKind)
	}
	_, card := f.openAsk(st.RunID)
	slotAssumptions := 0
	for _, a := range card.Approval.Layer1.Assumptions {
		if strings.HasPrefix(a.Origin, "slot:") {
			slotAssumptions++
		}
	}
	if slotAssumptions == 0 {
		t.Fatal("force-proceed produced no slot assumptions on the card")
	}
}

// TestEscalation: a consequential ambiguity raises exactly one
// single-question card; the call re-issues after the answer (1.7).
func TestEscalation(t *testing.T) {
	f := newFix(t)
	asked := false
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		if !asked {
			asked = true
			return intake.Pair{}, &intake.Escalation{Question: "Deleting the cache changes the write-set — proceed?"}
		}
		if len(in.Escalations) == 0 {
			t.Fatal("escalation answer not fed back to the planner")
		}
		return basePair(in), nil
	}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardEscalation {
		t.Fatalf("expected escalation card, got %q", st.OpenAskKind)
	}
	askID, card := f.openAsk(st.RunID)
	if len(card.Questions) != 1 {
		t.Fatalf("escalation must be a single question, got %d", len(card.Questions))
	}
	st = f.answer("u1", askID, intake.Answer{Text: "yes, the cache is disposable"})
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("after escalation answer: %q", st.OpenAskKind)
	}
}

// TestResumeFromArtifact: an interrupted intake resumes from its durable
// state and artifacts — never by replaying conversation (S06.1, D7).
func TestResumeFromArtifact(t *testing.T) {
	f := newFix(t)
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	f.answerInterviewToFloor("u1", st.RunID)

	// A fresh pipeline instance over the same durable substrate (the
	// in-memory one is gone).
	p2 := &intake.Pipeline{
		DB: f.db, Log: f.log, Runs: f.runs, Ledger: f.led, Settings: settings.New(),
		ArtifactRoot: f.p.ArtifactRoot,
		Planner:      &fakePlanner{}, Critic: &fakeCritic{},
		Now: func() time.Time { return f.now },
	}
	st2, err := p2.LoadState(context.Background(), st.TaskID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if st2.OpenAskKind != intake.CardApproval {
		t.Fatalf("reloaded state: %q", st2.OpenAskKind)
	}
	askID, _ := f.openAsk(st.RunID)
	raw, _ := json.Marshal(intake.Answer{Action: intake.ActionApprove})
	st3, err := p2.Answer(context.Background(), "u1", askID, raw)
	if err != nil {
		t.Fatalf("approve after resume: %v", err)
	}
	if st3.Phase != intake.PhaseApproved {
		t.Fatalf("phase %s", st3.Phase)
	}
}

// TestClaims: unbounded write-sets take the ⚙-selected claim; collisions
// surface at plan time (S02.8).
func TestClaims(t *testing.T) {
	f := newFix(t)
	f.p.Registry = &fakeRegistry{ok: true, slice: intake.RegistrySlice{Project: "proj-1", Ref: "reg:proj-1"}}
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		p.Plan.Steps[0].Unbounded = true
		return p, nil
	}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	f.answerInterviewToFloor("u1", st.RunID)
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionApprove})

	var globs string
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT path_globs FROM artifact_claims WHERE task_id = ? AND mode = 'W'`, st.TaskID).Scan(&globs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(globs, `"**"`) {
		t.Fatalf("unbounded plan claim %s, want whole-project (⚙ claims.default_write)", globs)
	}

	// Second task in the same project overlaps → sequenced, surfaced.
	f.planner.draft = nil
	f.planner.revise = nil
	st2 := f.start(intake.Request{UserID: "u1", Title: "Second task", Text: "another change"})
	f.admit(st2.RunID)
	f.advance(st2.TaskID)
	f.answerInterviewToFloor("u1", st2.RunID)
	askID2, _ := f.openAsk(st2.RunID)
	st2 = f.answer("u1", askID2, intake.Answer{Action: intake.ActionApprove})
	if st2.ClaimStatus != "waiting" {
		t.Fatalf("overlapping claim status %q, want waiting", st2.ClaimStatus)
	}
	doc := f.ledgerDoc(st2.TaskID)
	foundCollision := false
	for _, d := range doc.Decisions {
		if strings.Contains(d.Text, "writes files that task") {
			foundCollision = true
		}
	}
	if !foundCollision {
		t.Fatal("claim collision not surfaced at plan time")
	}
}

// TestDeclaredSetSetting: ⚙ claims.default_write = declared-set keeps the
// declared globs on an unbounded plan.
func TestDeclaredSetSetting(t *testing.T) {
	f := newFix(t)
	f.p.Settings = overrideSettings{Settings: f.p.Settings, strings: map[string]string{"claims.default_write": "declared-set"}}
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		p := basePair(in)
		p.Plan.Steps[0].Unbounded = true // unbounded, but src/** declared
		return p, nil
	}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	f.answerInterviewToFloor("u1", st.RunID)
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionApprove})
	var globs string
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT path_globs FROM artifact_claims WHERE task_id = ? AND mode = 'W'`, st.TaskID).Scan(&globs); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(globs, `"**"`) || !strings.Contains(globs, "src/**") {
		t.Fatalf("declared-set claim %s", globs)
	}
}

// TestApprovalStaleFlag: a pending card past ⚙ freshness.max_age is
// flagged, never blocked (S06.9).
func TestApprovalStaleFlag(t *testing.T) {
	f := newFix(t)
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	st = f.answerInterviewToFloor("u1", st.RunID)
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("got %q", st.OpenAskKind)
	}

	f.now = f.now.Add(25 * time.Hour) // past the 24 h default
	st, err := f.p.CheckApprovalStale(context.Background(), st.TaskID, false)
	if err != nil {
		t.Fatal(err)
	}
	if !st.StaleFlag || len(st.StaleReasons) == 0 {
		t.Fatalf("stale flag %v %v", st.StaleFlag, st.StaleReasons)
	}
	_, card := f.openAsk(st.RunID)
	if !card.Approval.StaleFlag {
		t.Fatal("card snapshot not flagged")
	}
	// The flag never blocks approving.
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionApprove})
	if st.Phase != intake.PhaseApproved {
		t.Fatalf("stale flag blocked approval: %s", st.Phase)
	}
}

// TestRePlanContest: the structured Re-plan action produces a bounded
// delta re-plan and a fresh card (S06.9).
func TestRePlanContest(t *testing.T) {
	f := newFix(t)
	var contested []string
	f.planner.revise = func(in intake.ReviseInput) (intake.Pair, error) {
		if in.Reason == intake.ReviseContest {
			contested = in.Findings
		}
		return baseRevise(in), nil
	}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	f.answerInterviewToFloor("u1", st.RunID)
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{
		Action:  intake.ActionRePlan,
		Contest: &intake.ContestRef{Target: "AC-2", Note: "verification is too vague"},
	})
	if st.OpenAskKind != intake.CardApproval {
		t.Fatalf("after re-plan: %q", st.OpenAskKind)
	}
	if len(contested) != 1 || !strings.Contains(contested[0], "AC-2") {
		t.Fatalf("planner saw contest %v", contested)
	}
	if st.CardVersion < 2 {
		t.Fatal("re-plan must issue a fresh card version")
	}
}

// TestDeltaReApproval: a post-approval change is an ADDED/MODIFIED/REMOVED
// delta on a delta-only card with the measurement hook; approving moves
// the ledger pins under the new spec version (S06.9).
func TestDeltaReApproval(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	f.answerInterviewToFloor("u1", st.RunID)
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionApprove})

	// Propose: drop AC-2, widen S-2's confinement.
	next := basePair(intake.DraftInput{Request: st.Req, Tier: st.Tier})
	next.Spec.ACs = next.Spec.ACs[:1]
	next.Plan.Coverage = map[string][]string{"AC-1": {"S-1"}}
	next.Plan.Steps[1].Class = "C2"
	st, deltaAsk, err := f.p.ProposeDelta(ctx, st.TaskID, "contested_card", next)
	if err != nil {
		t.Fatalf("ProposeDelta: %v", err)
	}
	_, card := f.openAsk(st.RunID)
	if card.Kind != intake.CardDelta {
		t.Fatalf("card kind %q", card.Kind)
	}
	// The card declares its OWN answer vocabulary (B6-6 drain D4). Until it did,
	// a delta card carried {origin, items, help} and nothing else — so a surface
	// that renders controls from the card had nothing to render, while the card
	// held the task's OpenAskID open. The offered set is built from the same
	// constants applyDeltaAnswer validates against, so it cannot offer a verb
	// the answer path would refuse.
	if got := card.Delta.Actions; len(got) != 2 ||
		got[0] != intake.ChoiceApproveDelta || got[1] != intake.ChoiceRejectDelta {
		t.Fatalf("delta card actions = %v, want the approve/reject vocabulary its answer path accepts", got)
	}
	// …and the verb it offers is the one the answer path below actually applies,
	// end to end, so "offered" and "accepted" are the same list in practice.

	kinds := map[intake.DeltaKind]int{}
	confinement := false
	for _, it := range card.Delta.Items {
		kinds[it.Kind]++
		if strings.HasPrefix(it.Target, "confinement:") {
			confinement = true
		}
	}
	if kinds[intake.DeltaRemoved] == 0 || !confinement {
		t.Fatalf("delta items %+v", card.Delta.Items)
	}

	st = f.answer("u1", deltaAsk, intake.Answer{Action: intake.ChoiceApproveDelta})
	doc := f.ledgerDoc(st.TaskID)
	if doc.SpecVersion != "spec-v2" {
		t.Fatalf("pins not moved: %q", doc.SpecVersion)
	}
	if len(doc.ObjectiveAC.AcceptanceCriteria) != 1 {
		t.Fatalf("frozen ACs after delta: %+v", doc.ObjectiveAC.AcceptanceCriteria)
	}
	if doc.PlanVersion != "spec-v2/plan-v2" {
		t.Fatalf("plan version %q", doc.PlanVersion)
	}

	// The measurement hook landed (P-T05-2).
	var payload string
	err = f.db.QueryRowContext(ctx,
		`SELECT payload FROM run_events WHERE type = 'intake.delta_decision' ORDER BY event_seq DESC LIMIT 1`).
		Scan(&payload)
	if err != nil {
		t.Fatalf("delta hook event: %v", err)
	}
	for _, want := range []string{"presented_items", "time_to_decision_s", `"decision":"approved"`} {
		if !strings.Contains(payload, want) {
			t.Fatalf("hook payload missing %s: %s", want, payload)
		}
	}

	// Old claims superseded, fresh ones active.
	var n int
	if err := f.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM artifact_claims WHERE task_id = ? AND status = 'superseded'`, st.TaskID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("old claims not superseded")
	}
}

// TestD10OnlyRequesterAnswers: another identity cannot answer intake
// cards (D10; 5.8).
func TestD10OnlyRequesterAnswers(t *testing.T) {
	f := newFix(t)
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	askID, _ := f.openAsk(st.RunID)
	raw, _ := json.Marshal(intake.Answer{ForceProceed: true})
	if _, err := f.p.Answer(context.Background(), "intruder", askID, raw); !errors.Is(err, intake.ErrNotRequester) {
		t.Fatalf("got %v", err)
	}
}

// TestCancel: cancel is always available (4.5); a parked intake finalizes.
func TestCancel(t *testing.T) {
	f := newFix(t)
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	askID, _ := f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{ForceProceed: true}) // reach approval
	askID, _ = f.openAsk(st.RunID)
	st = f.answer("u1", askID, intake.Answer{Action: intake.ActionCancel})
	if st.Phase != intake.PhaseCancelled {
		t.Fatalf("phase %s", st.Phase)
	}
	if f.runState(st.RunID) != run.StateFinalized {
		t.Fatalf("run %s", f.runState(st.RunID))
	}
}

// TestLowerTier: lowering is explicit, floor-guarded, and never re-enters
// the band (S06.4).
func TestLowerTier(t *testing.T) {
	f := newFix(t)
	f.class.prop.Floors = []intake.FloorReason{{Class: intake.FloorNewSpend, Source: "classifier", Detail: "buys credits"}}
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID) // the classify step records the classifier's floor
	if _, err := f.p.LowerTier(context.Background(), "u1", st.TaskID, intake.TierLow); !errors.Is(err, intake.ErrBelowFloor) {
		t.Fatalf("floor-guarded lowering: %v", err)
	}

	f2 := newFix(t)
	st2 := f2.start(stdRequest())
	f2.admit(st2.RunID)
	f2.advance(st2.TaskID)
	got, err := f2.p.LowerTier(context.Background(), "u1", st2.TaskID, intake.TierLow)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tier != intake.TierLow {
		t.Fatalf("tier %s", got.Tier)
	}
	if _, err := f2.p.LowerTier(context.Background(), "u1", st2.TaskID, intake.TierTrivial); err == nil {
		t.Fatal("band re-entry by hand must be refused")
	}
}

// TestPlanSource: the approved plan serves stage instructions through the
// S05.4 Plan seam.
func TestPlanSource(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	st := f.start(stdRequest())
	src := &intake.PlanSource{P: f.p}

	items, err := src.Items(ctx, ledger.SliceQuery{TaskID: st.TaskID, Owner: "u1", Stage: "S-1"})
	if err != nil || items != nil {
		t.Fatalf("unapproved plan contributed: %v %v", items, err)
	}

	f.admit(st.RunID)
	f.advance(st.TaskID)
	f.answerInterviewToFloor("u1", st.RunID)
	askID, _ := f.openAsk(st.RunID)
	f.answer("u1", askID, intake.Answer{Action: intake.ActionApprove})

	items, err = src.Items(ctx, ledger.SliceQuery{TaskID: st.TaskID, Owner: "u1", Stage: "S-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items %d", len(items))
	}
	if items[0].Precedence != ledger.PrecedenceTask || items[1].Precedence != ledger.PrecedenceStage {
		t.Fatalf("precedence %v %v", items[0].Precedence, items[1].Precedence)
	}
	if !strings.Contains(items[1].Content, "Done when: tests pass") || !strings.Contains(items[1].Content, "AC-1") {
		t.Fatalf("step contract: %s", items[1].Content)
	}
	unknown, err := src.Items(ctx, ledger.SliceQuery{TaskID: "t-none", Owner: "u1", Stage: "S-1"})
	if err != nil || unknown != nil {
		t.Fatalf("unknown task: %v %v", unknown, err)
	}
}

// TestStateEventSourced: every boundary appended an intake.state event and
// the latest reconstructs the pipeline (S02.2 pattern).
func TestStateEventSourced(t *testing.T) {
	f := newFix(t)
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	var n int
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM run_events WHERE run_id = ? AND type = 'intake.state'`, st.RunID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 2 {
		t.Fatalf("state events %d", n)
	}
	if _, err := f.p.LoadState(context.Background(), "t-missing"); !errors.Is(err, intake.ErrNoState) {
		t.Fatalf("got %v", err)
	}
}

// TestSeamMissingCritic: standard tier without a Critic is a structural
// error, never a silent skip (S06.4).
func TestSeamMissingCritic(t *testing.T) {
	f := newFix(t)
	f.p.Critic = nil
	st := f.start(stdRequest())
	f.admit(st.RunID)
	f.advance(st.TaskID)
	askID, _ := f.openAsk(st.RunID)
	raw, _ := json.Marshal(intake.Answer{ForceProceed: true})
	_, err := f.p.Answer(context.Background(), "u1", askID, raw)
	if !errors.Is(err, intake.ErrSeamMissing) {
		t.Fatalf("got %v", err)
	}
}

// TestRegistrySuppliedSlots: the interview never asks what the platform
// already knows (S1.6).
func TestRegistrySuppliedSlots(t *testing.T) {
	f := newFix(t)
	f.p.Registry = &fakeRegistry{ok: true, slice: intake.RegistrySlice{
		Project: "proj-1", Ref: "reg:proj-1", Family: intake.FamilySoftware,
		ResolvedSlots: map[string]string{"output_format": "match repo conventions", "units": "SI"},
		DangerZones:   []intake.DangerZone{{Path: "infra/**", Rule: "never touch"}},
	}}
	st := f.start(stdRequest())
	// The two registry-supplied slots, plus the ones the software set settles
	// without asking — both are resolved when the question set is keyed
	// (P3-GF7 R3).
	wantResolutions := 2 + gf7NeverAskedCount(intake.FamilySoftware)
	if len(st.Resolutions) != wantResolutions {
		t.Fatalf("registry resolutions %+v, want %d", st.Resolutions, wantResolutions)
	}
	f.admit(st.RunID)
	f.advance(st.TaskID)
	_, card := f.openAsk(st.RunID)
	for _, q := range card.Questions {
		if q.ID == "output_format" || q.ID == "units" {
			t.Fatalf("asked a registry-supplied slot %q", q.ID)
		}
	}
	// Danger zones pin into §2 at approval.
	st = f.answerInterviewToFloor("u1", st.RunID)
	askID, _ := f.openAsk(st.RunID)
	f.answer("u1", askID, intake.Answer{Action: intake.ActionApprove})
	doc := f.ledgerDoc(st.TaskID)
	if len(doc.Constraints.DangerZones) != 1 || doc.Constraints.DangerZones[0].Path != "infra/**" {
		t.Fatalf("danger zones not pinned: %+v", doc.Constraints)
	}
}

// TestGateOpenRefusesAdvance ensures no timer or code path proceeds past
// an open gate (S06.1: gates wait).
func TestGateOpenRefusesAdvance(t *testing.T) {
	f := newFix(t)
	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	before := st.OpenAskID
	f.now = f.now.Add(1000 * time.Hour) // no timer ever auto-proceeds
	st = f.advance(st.TaskID)
	if st.OpenAskID != before {
		t.Fatal("advance moved past an open gate")
	}
	if f.runState(st.RunID) != run.StateParked {
		t.Fatal("run must stay parked")
	}
}

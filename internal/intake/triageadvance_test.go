package intake_test

// triageadvance_test.go — the P3-RW-13 grounding acceptance battery
// (committed FAILING by the grounding session; the executor makes it pass).
//
// Stage-0 restructure (Spec S06.2, S02.3; CONVENTIONS §26 R18): the
// classifier leg moves out of Pipeline.Start onto a run-once classify step at
// the top of the FIRST advance on the RUNNING intake run — the one place its
// mandatory $0 D7 row has a checkpointable consuming run to ride
// (gates.Checkpoints accepts running/draining only). Start keeps the registry
// match, the deterministic P47 rule layer, and the fail-closed baseline
// exactly as they are; classification still precedes Stage-1 planning, so
// S06.2's "before any paid model runs" holds by code order in the same drive.
//
// NOTE FOR THE EXECUTOR: probeClassifier implements the CURRENT Classifier
// signature. If the seam gains an explicit consuming-run input (the brief's
// R8 recommendation, the DraftInput/PhraseInput precedent), adapt the probe's
// signature — the ASSERTIONS are the packet's acceptance contract and stay as
// written.

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// probeClassifier counts its invocations and records the intake run's FSM
// state as observed at each call — the §26 R18 precondition ("a RUNNING
// consuming run") made directly assertable.
type probeClassifier struct {
	prop       intake.TriageProposal
	err        error
	calls      int
	callStates []string
	observe    func() string
}

func (p *probeClassifier) Classify(context.Context, intake.Request, *intake.RegistrySlice) (intake.TriageProposal, error) {
	p.calls++
	if p.observe != nil {
		p.callStates = append(p.callStates, p.observe())
	}
	return p.prop, p.err
}

// observeRunState returns a closure reporting the run's current FSM state
// ("absent" while the run row does not exist yet — which is exactly what the
// Start-time call site sees today).
func observeRunState(f *fix, runID *string) func() string {
	return func() string {
		if *runID == "" {
			return "absent"
		}
		r, err := f.runs.Get(context.Background(), *runID)
		if err != nil {
			return "absent"
		}
		return string(r.State)
	}
}

func decodeRecord(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read intake record %s: %v", path, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode intake record %s: %v", path, err)
	}
	return m
}

// TestTriageClassifiesOnTheRunningIntakeRun is the packet headline: Start
// performs NO classifier call (fail-closed baseline only), the first advance
// classifies exactly once while the intake run is RUNNING (§26 R18: the $0 D7
// row's consuming run is checkpointable), and the classify step is run-once —
// later advances and resumes never re-classify.
func TestTriageClassifiesOnTheRunningIntakeRun(t *testing.T) {
	f := newFix(t)
	var runID string
	probe := &probeClassifier{
		prop: intake.TriageProposal{
			Family: intake.FamilySoftware, Tier: intake.TierStandard,
			Est: intake.Estimate{SizeClass: "S", USD: 1.0, Known: true, Basis: "probe"},
		},
		observe: observeRunState(f, &runID),
	}
	f.p.Classifier = probe

	st := f.start(stdRequest())
	runID = st.RunID

	// R1/R2: Start runs no classifier and returns the fail-closed baseline.
	if probe.calls != 0 {
		t.Fatalf("Start called the classifier %d time(s); the classify leg belongs to the first advance on the RUNNING intake run (S06.2 restructure, §26 R18)", probe.calls)
	}
	if st.Tier != intake.TierHigh {
		t.Errorf("post-Start tier = %q, want %q (fail-closed baseline until classified, S06.2)", st.Tier, intake.TierHigh)
	}
	if st.FamilySource != intake.FamilySourceDefault {
		t.Errorf("post-Start family source = %q, want %q (nothing resolved it yet)", st.FamilySource, intake.FamilySourceDefault)
	}
	if st.Guess.Known {
		t.Errorf("post-Start estimate is Known; want the unclassified baseline")
	}
	if st.Band {
		t.Errorf("post-Start band membership; band entry moves to first-advance (S06.4)")
	}

	f.admit(st.RunID)
	st = f.advance(st.TaskID)

	// The first advance classifies exactly once, on a RUNNING run.
	if probe.calls != 1 {
		t.Fatalf("first advance made %d classifier call(s), want exactly 1", probe.calls)
	}
	if len(probe.callStates) != 1 || probe.callStates[0] != string(run.StateRunning) {
		t.Fatalf("classifier was called with the intake run in state %v, want [%q] — the $0 D7 row must ride a checkpointable consuming run (§26 R18)",
			probe.callStates, run.StateRunning)
	}
	if st.Tier != intake.TierStandard || st.Family != intake.FamilySoftware || st.FamilySource != intake.FamilySourceClassifier {
		t.Errorf("post-classify state = family %q (%s) tier %q, want software (classifier) standard",
			st.Family, st.FamilySource, st.Tier)
	}

	// Run-once: driving the pipeline onward (answers resume in place and
	// reload state from the durable events) never re-classifies.
	st = f.answerInterviewToFloor("u1", st.RunID)
	if probe.calls != 1 {
		t.Errorf("pipeline re-classified on a later advance (%d calls); the classify step is guarded run-once", probe.calls)
	}
	if st.FamilySource != intake.FamilySourceClassifier {
		t.Errorf("family source drifted to %q across resumes", st.FamilySource)
	}
}

// TestStartWritesTheBaselineIntakeRecord: the Stage-0 record v1 written at
// birth carries the honest pre-classification baseline — never classifier
// values it does not have yet.
func TestStartWritesTheBaselineIntakeRecord(t *testing.T) {
	f := newFix(t)
	f.p.Classifier = &probeClassifier{prop: intake.TriageProposal{
		Family: intake.FamilySoftware, Tier: intake.TierStandard,
		Est: intake.Estimate{USD: 1.0, Known: true},
	}}

	st := f.start(stdRequest())
	if st.RecordRef == nil {
		t.Fatal("Start left no intake-record ref")
	}
	if st.RecordRef.Version != 1 {
		t.Errorf("record version = %d, want 1 at birth", st.RecordRef.Version)
	}
	rec := decodeRecord(t, st.RecordRef.Path)
	if rec["stakes_tier"] != string(intake.TierHigh) {
		t.Errorf("record v1 stakes_tier = %v, want %q (fail-closed baseline; classification has not run)", rec["stakes_tier"], intake.TierHigh)
	}
	if rec["family"] != string(intake.FamilyGeneric) {
		t.Errorf("record v1 family = %v, want %q (unresolved baseline)", rec["family"], intake.FamilyGeneric)
	}
	if rec["band"] != false {
		t.Errorf("record v1 band = %v, want false", rec["band"])
	}
}

// TestIntakeRecordVersionsAcrossClassification: the classify step writes a NEW
// record version (v2) carrying the classified values; the v1 file stays on
// disk byte-untouched (S06.6 immutability — draft artifacts never mutate).
func TestIntakeRecordVersionsAcrossClassification(t *testing.T) {
	f := newFix(t)
	f.p.Classifier = &probeClassifier{prop: intake.TriageProposal{
		Family: intake.FamilySoftware, Tier: intake.TierStandard,
		Est: intake.Estimate{SizeClass: "S", USD: 1.0, Known: true, Basis: "probe"},
	}}

	st := f.start(stdRequest())
	v1 := *st.RecordRef

	f.admit(st.RunID)
	st = f.advance(st.TaskID)

	if st.RecordRef.Version != 2 {
		t.Fatalf("post-classify record version = %d, want 2 (the intake-record artifact is versioned)", st.RecordRef.Version)
	}
	if st.RecordRef.Path == v1.Path {
		t.Fatalf("record v2 overwrote v1 in place (%s); versions are distinct immutable files", v1.Path)
	}
	if _, err := os.Stat(v1.Path); err != nil {
		t.Errorf("record v1 vanished after classification: %v (draft artifacts are immutable)", err)
	}
	rec := decodeRecord(t, st.RecordRef.Path)
	if rec["family"] != string(intake.FamilySoftware) || rec["stakes_tier"] != string(intake.TierStandard) {
		t.Errorf("record v2 = family %v tier %v, want the classified software/standard", rec["family"], rec["stakes_tier"])
	}
}

// TestBandEntersAtFirstAdvanceZeroTouch: a trivial read-only under-cap task is
// NOT banded at Start (nothing classified it yet) and enters the band on the
// first advance — reaching the ungated auto-approval with ZERO cards issued.
// No extra click moved onto the requester (S06.4).
func TestBandEntersAtFirstAdvanceZeroTouch(t *testing.T) {
	f := newFix(t)
	f.p.Classifier = &probeClassifier{prop: intake.TriageProposal{
		Family: intake.FamilyChore, Tier: intake.TierTrivial, ReadOnly: true,
		Est: intake.Estimate{SizeClass: "XS", USD: 0.10, Known: true, Basis: "probe"},
	}}

	st := f.start(stdRequest())
	if st.Band {
		t.Fatalf("banded at Start; band entry moves to the first advance (classification has not run yet)")
	}

	f.admit(st.RunID)
	st = f.advance(st.TaskID)

	if !st.Band {
		t.Fatalf("first advance did not enter the band (tier %q, band-exit %q)", st.Tier, st.BandExitWhy)
	}
	if st.Phase != intake.PhaseApproved || st.Approval == nil || !st.Approval.Ungated {
		t.Fatalf("band task did not reach the ungated auto-approval: phase %q approval %+v", st.Phase, st.Approval)
	}
	var asks int
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM asks WHERE run_id = ?`, st.RunID).Scan(&asks); err != nil {
		t.Fatal(err)
	}
	if asks != 0 {
		t.Errorf("band task issued %d card(s); the zero-interaction band never requires a click (S06.4)", asks)
	}
}

// TestPreRW13StateIsNeverReclassified is the in-flight compatibility guard
// (the familyGate/R10 precedent): a state event written before this packet
// carries no classify marker, and the next advance must leave it exactly as
// it was — no classifier call, no family/tier rewrite. It must PASS before
// and after the restructure.
func TestPreRW13StateIsNeverReclassified(t *testing.T) {
	f := newFix(t)
	probe := &probeClassifier{prop: intake.TriageProposal{
		Family: intake.FamilySoftware, Tier: intake.TierStandard,
		Est: intake.Estimate{USD: 1.0, Known: true},
	}}
	f.p.Classifier = probe

	req := stdRequest()
	req.TaskID = "t-oldworld"
	st := f.start(req)
	f.admit(st.RunID)
	baseline := probe.calls

	// A pre-RW-13 world: the requester already answered the family question
	// and the tier settled low. The zero value of any later-added marker field
	// is by construction the not-pending posture.
	old := intake.State{
		Phase: intake.PhaseInterview, TaskID: st.TaskID, RunID: st.RunID,
		Owner: "u1", Req: req,
		Family: intake.FamilyResearch, FamilySource: intake.FamilySourceRequester,
		Tier: intake.TierLow, Guess: intake.Estimate{Known: false},
		NeedsDraft: true,
	}
	payload, err := json.Marshal(&old)
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.runs.Get(context.Background(), st.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.log.Append(context.Background(), eventlog.Append{
		RunID: st.RunID, Generation: r.Generation, UserID: "u1",
		Type: intake.EventState, SchemaVersion: 1, Payload: payload,
	}); err != nil {
		t.Fatal(err)
	}

	st = f.advance(st.TaskID)
	if probe.calls != baseline {
		t.Errorf("advance re-classified a pre-RW-13 state (%d → %d calls); an interview already in flight in an old world is never touched", baseline, probe.calls)
	}
	if st.Family != intake.FamilyResearch || st.FamilySource != intake.FamilySourceRequester {
		t.Errorf("family rewritten to %q (%s); a requester-answered family outranks the classifier", st.Family, st.FamilySource)
	}
	if st.Tier != intake.TierLow {
		t.Errorf("tier rewritten to %q; automatic adjustments are monotone and classification never re-runs (S06.4)", st.Tier)
	}
}

// TestRegistryFamilySurvivesClassification pins the ratified precedence
// registry > classifier across the move: a registered project's family stands;
// the classifier still supplies stakes and size. Must PASS before and after.
func TestRegistryFamilySurvivesClassification(t *testing.T) {
	f := newFix(t)
	f.p.Registry = &fakeRegistry{
		slice: intake.RegistrySlice{Project: "p1", Ref: "reg-p1", Family: intake.FamilyResearch},
		ok:    true,
	}
	f.p.Classifier = &probeClassifier{prop: intake.TriageProposal{
		Family: intake.FamilySoftware, Tier: intake.TierStandard,
		Est: intake.Estimate{SizeClass: "S", USD: 1.0, Known: true, Basis: "probe"},
	}}

	st := f.start(stdRequest())
	if st.Family != intake.FamilyResearch || st.FamilySource != intake.FamilySourceRegistry {
		t.Fatalf("post-Start family = %q (%s), want the registry's research", st.Family, st.FamilySource)
	}

	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	if st.Family != intake.FamilyResearch || st.FamilySource != intake.FamilySourceRegistry {
		t.Errorf("classification overrode the registry family: %q (%s); precedence registry > classifier (P3-RW-11 OQ3)", st.Family, st.FamilySource)
	}
	if st.Tier != intake.TierStandard {
		t.Errorf("tier = %q, want the classifier's standard (the registry pins family, not stakes)", st.Tier)
	}
}

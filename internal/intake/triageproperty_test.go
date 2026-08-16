package intake_test

// triageproperty_test.go — P3-RW-13, the two Stage-0 invariants that are
// STATED as rules rather than as examples, pinned over their whole input
// domain instead of over one case each:
//
//   - the family precedence registry > classifier > requester > default
//     (Spec S06.2 rule 1; P3-RW-11 OQ3), swept over every family the
//     vocabulary contains and every source that can already hold the slot;
//   - S06.2's ordering guarantee — classification precedes Stage-1 planning,
//     so no paid seam can fire before it in a drive.
//
// Both are properties of the classify step, and both would survive a
// single-example test that happened to pick the one input the code handles.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// recordingClassifier appends to a shared call log so ORDER across seams is
// assertable, not just call counts.
type recordingClassifier struct {
	prop intake.TriageProposal
	seq  *[]string
}

func (c recordingClassifier) Classify(context.Context, intake.TriageInput) (intake.TriageProposal, error) {
	*c.seq = append(*c.seq, "classify")
	return c.prop, nil
}

// recordingPhraser is a paid seat that must never speak before triage.
type recordingPhraser struct{ seq *[]string }

func (p recordingPhraser) PhraseAndSummarize(context.Context, intake.PhraseInput) (intake.PhraseResult, error) {
	*p.seq = append(*p.seq, "phrase")
	return intake.PhraseResult{}, nil
}

// TestClassifyStepFamilyPrecedenceHolds sweeps the precedence rule over the
// whole family vocabulary plus the out-of-vocabulary case, against each source
// that can already hold the slot when the classify step runs.
//
// The rule has one shape: the classifier's family is adopted EXACTLY when the
// label is a real family AND nothing better already resolved the slot. A
// registry family is its owner's standing declaration and a requester's answer
// is the requester's own word — a per-request guess overrules neither, and an
// out-of-vocabulary label resolves nothing at all (it is asked, never assumed).
func TestClassifyStepFamilyPrecedenceHolds(t *testing.T) {
	proposals := append([]intake.Family{}, intake.Families()...)
	proposals = append(proposals, "bogus") // an out-of-vocabulary label resolves nothing

	for _, proposed := range proposals {
		for _, held := range []struct {
			name   string
			source string
			family intake.Family
		}{
			{"registry", intake.FamilySourceRegistry, intake.FamilyResearch},
			{"requester", intake.FamilySourceRequester, intake.FamilyContent},
			{"default", intake.FamilySourceDefault, intake.FamilyGeneric},
		} {
			t.Run(string(proposed)+"/over-"+held.name, func(t *testing.T) {
				f := newFix(t)
				f.p.Registry = nil
				f.class.prop = intake.TriageProposal{Family: proposed, Tier: intake.TierStandard}

				st := f.start(stdRequest())
				// Put the pre-held resolution in place the way each source really
				// arrives: the registry resolves at Start, a requester answer lands
				// on a state the classify step then re-enters.
				if held.source != intake.FamilySourceDefault {
					seedFamily(t, f, st, held.family, held.source)
				}
				f.admit(st.RunID)
				st = f.advance(st.TaskID)

				wantFamily, wantSource := held.family, held.source
				if held.source == intake.FamilySourceDefault && intake.ValidFamily(proposed) {
					wantFamily, wantSource = proposed, intake.FamilySourceClassifier
				}
				if st.Family != wantFamily || st.FamilySource != wantSource {
					t.Errorf("family/source = %q/%q, want %q/%q (precedence registry > classifier > requester > default)",
						st.Family, st.FamilySource, wantFamily, wantSource)
				}
				// The tier is the classifier's regardless: the registry pins the
				// family, not the stakes.
				if st.Tier != intake.TierStandard {
					t.Errorf("tier = %q, want standard — a family precedence rule must not disturb the stakes reading", st.Tier)
				}
			})
		}
	}
}

// seedFamily rewrites the pending-triage state with a family already resolved
// by `source`, through the real event log at the run's current generation — the
// state the classify step will load, not a shape that merely resembles one.
func seedFamily(t *testing.T, f *fix, st *intake.State, family intake.Family, source string) {
	t.Helper()
	ctx := context.Background()
	seeded := *st
	seeded.Family, seeded.FamilySource = family, source
	payload, err := json.Marshal(&seeded)
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.runs.Get(ctx, st.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.log.Append(ctx, eventlog.Append{
		RunID: st.RunID, Generation: r.Generation, UserID: st.Owner,
		Type: intake.EventState, SchemaVersion: 1, Payload: payload,
	}); err != nil {
		t.Fatalf("seed the %s-resolved family: %v", source, err)
	}
}

// TestNoPaidSeamPrecedesClassification pins S06.2's ordering guarantee by code
// order: whatever a drive does, the classify call is the FIRST seam call it
// makes. The phrase seat is the earliest paid seam an intake drive can reach
// (it words the first interview card), so if anything could outrun triage, it
// would be that.
func TestNoPaidSeamPrecedesClassification(t *testing.T) {
	var seq []string
	f := newFix(t)
	f.p.Registry = nil
	f.p.Classifier = recordingClassifier{seq: &seq, prop: intake.TriageProposal{
		Family: intake.FamilySoftware, Tier: intake.TierStandard,
		Est: intake.Estimate{SizeClass: "S", USD: 1.0, Known: true, Basis: "probe"},
	}}
	f.p.Phraser = recordingPhraser{seq: &seq}
	f.planner.draft = func(in intake.DraftInput) (intake.Pair, error) {
		seq = append(seq, "draft")
		return basePair(in), nil
	}

	st := f.start(stdRequest())
	if len(seq) != 0 {
		t.Fatalf("Start called seams %v; Start runs no model at all (S06.2)", seq)
	}
	f.admit(st.RunID)
	st = f.advance(st.TaskID)
	f.answerInterviewToFloor("u1", st.RunID)

	if len(seq) < 2 {
		t.Fatalf("drive called seams %v; the run must reach at least triage and one paid seat", seq)
	}
	if seq[0] != "classify" {
		t.Errorf("seam order %v: %q ran before classification — S06.2 requires triage before any paid model", seq, seq[0])
	}
	for _, name := range seq[1:] {
		if name == "classify" {
			t.Errorf("seam order %v: classification ran twice; the classify step is guarded run-once", seq)
		}
	}
}

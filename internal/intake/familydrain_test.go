package intake_test

// familydrain_test.go — P3-RW-11 drain round 1 (D1, D3, D4). Three legs the
// committed battery left unpinned, each found by driving the shipped code
// rather than by reading it.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// bandProposal is the S06.4 zero-interaction band's four conditions met, with
// the family label the caller wants tested.
func bandProposal(family intake.Family) intake.TriageProposal {
	return intake.TriageProposal{
		Family: family, Tier: intake.TierTrivial, ReadOnly: true, NewNeeds: false,
		Est: intake.Estimate{SizeClass: "XS", USD: 0.10, Known: true, Basis: "fake"},
	}
}

// TestBandNeverAsksFamilyWhenUnresolved (drain D1) is the band exemption's OWN
// guard, which the committed T8 did not reach.
//
// T8 drove a band task whose family the classifier RESOLVED, so the family gate
// declined for the source reason and the `st.Band ||` limb was never the thing
// keeping the card away. Deleting that limb left the whole suite green — the
// mutation the evaluator ran. Here the band task's family is UNRESOLVED (an
// out-of-vocabulary label, which the D3 boundary check turns into no family at
// all), so `Band && FamilySource == "default"` is exactly the state that reaches
// the gate, and only the band limb stands between it and a card.
//
// S06.4 is ratified: a trivial task never requires a click. A band task that
// asked one would be the zero-interaction band asking for interaction.
func TestBandNeverAsksFamilyWhenUnresolved(t *testing.T) {
	f := newFix(t)
	f.p.Registry = nil
	f.class.prop = bandProposal("bogus") // classified, but the label is not a family

	st := f.start(stdRequest())
	f.admit(st.RunID)
	st = f.advance(st.TaskID)

	if !st.Band {
		t.Fatalf("task did not enter the band: tier=%q band=%v", st.Tier, st.Band)
	}
	if st.FamilySource != intake.FamilySourceDefault {
		t.Fatalf("family source = %q, want %q — this test only bites when the family is UNRESOLVED",
			st.FamilySource, intake.FamilySourceDefault)
	}

	if kinds := cardKindsIssued(t, f, st.RunID); len(kinds) != 0 {
		t.Errorf("a band task with an unresolved family was asked %v — S06.4: a trivial task never requires a click", kinds)
	}
	if st.Phase != intake.PhaseApproved {
		t.Errorf("band phase = %q, want approved (the band completes without a gate)", st.Phase)
	}
	if st.Family != intake.FamilyGeneric {
		t.Errorf("band family = %q, want generic (the fail-closed baseline, attributed \"default\")", st.Family)
	}
}

// TestOutOfVocabularyFamilyIsUnresolvedAtBothSeams (drain D3): a family value
// neither seam should ever have produced is treated as UNRESOLVED — never
// applied to the state, never attributed to the seam that sent it, and never
// silently keyed to the generic set. The requester is asked instead.
func TestOutOfVocabularyFamilyIsUnresolvedAtBothSeams(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(f *fix)
	}{
		{"registry", func(f *fix) {
			f.p.Classifier = nil
			f.p.Registry = &fakeRegistry{ok: true, slice: intake.RegistrySlice{
				Project: "shop", Ref: "shop@capture-v1", Family: "bogus",
			}}
		}},
		{"classifier", func(f *fix) {
			f.p.Registry = nil
			f.class.prop = intake.TriageProposal{Family: "bogus", Tier: intake.TierStandard}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFix(t)
			tc.setup(f)

			st := f.start(stdRequest())
			f.admit(st.RunID)
			st = f.advance(st.TaskID)
			if st.FamilySource != intake.FamilySourceDefault {
				t.Errorf("family source = %q, want %q — an out-of-vocabulary value must not be attributed to the %s",
					st.FamilySource, intake.FamilySourceDefault, tc.name)
			}
			if st.Family != intake.FamilyGeneric {
				t.Errorf("family = %q, want the untouched generic baseline", st.Family)
			}
			if st.OpenAskKind != intake.CardFamily {
				t.Fatalf("card kind = %q, want %q — an unresolved family is ASKED", st.OpenAskKind, intake.CardFamily)
			}
		})
	}
}

// TestPreRW11StateIsNeverInterrupted (drain D4) pins checklist 9's in-flight
// half durably.
//
// A state event written before this packet carries NO `family_source` key at
// all, so it decodes to "". The gate fires on `== "default"` and nothing else
// precisely so an interview ALREADY IN FLIGHT in an old world is never stopped
// mid-way to be asked a question its requester was never going to be asked. The
// event here is the real thing: appended through the event log, at the run's
// current generation, with the field deleted from the payload rather than a
// hand-written shape that only resembles one.
func TestPreRW11StateIsNeverInterrupted(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.p.Classifier = nil
	f.p.Registry = nil

	st := f.start(stdRequest())
	f.admit(st.RunID)

	// Mid-interview, the way an old world would be: one slot already resolved,
	// no gate open, the pair not yet drafted.
	var payload map[string]any
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if _, present := payload["family_source"]; !present {
		t.Fatal("the state did not carry family_source — this test's premise is that it does and an OLD one does not")
	}
	delete(payload, "family_source")
	payload["resolutions"] = []map[string]string{{"slot_id": "goal", "how": "answered", "value": "ship the note"}}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	gen, err := f.runs.Get(ctx, st.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.log.Append(ctx, eventlog.Append{
		RunID: st.RunID, Generation: gen.Generation, UserID: st.Owner,
		Type: intake.EventState, SchemaVersion: 1, Payload: body,
	}); err != nil {
		t.Fatalf("append the pre-RW-11 state event: %v", err)
	}

	loaded, err := f.p.LoadState(ctx, st.TaskID)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if loaded.FamilySource != "" {
		t.Fatalf("the appended state carries family_source %q — it must be absent for this to be the old shape", loaded.FamilySource)
	}

	next := f.advance(st.TaskID)
	if next.OpenAskKind == intake.CardFamily {
		t.Fatal("an in-flight pre-RW-11 interview was interrupted by the family question (checklist 9)")
	}
	if next.OpenAskKind != intake.CardInterview {
		t.Fatalf("card kind = %q, want interview — the old world proceeds exactly as today", next.OpenAskKind)
	}
	// And the answer it already held is still held: the interview continues, it
	// does not restart.
	if len(next.Resolutions) != 1 || next.Resolutions[0].SlotID != "goal" {
		t.Errorf("resolutions = %+v, want the one the old state carried", next.Resolutions)
	}
	_, card := f.openAsk(next.RunID)
	for _, q := range card.Questions {
		if q.ID == "goal" {
			t.Error("the continuing card re-asks an already-resolved slot")
		}
	}
}

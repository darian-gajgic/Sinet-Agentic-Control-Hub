package verify_test

// preamble_rw14_test.go — P3-RW-14 R4, the property (not an example): EXACTLY
// the deterministic verification-infrastructure absences convert to a card;
// everything else still crashes for the recovery ladder.
//
// The distinction is the packet's whole point. A missing screen is a screen
// OUTAGE — forking it re-runs the identical refusal until the ladder tombstones
// the lineage (the live 2026-08-16 wedge), so it escalates instead (Spec S07.2
// "a screen outage escalates rather than approves"). A MECHANICAL failure — a
// judge transport error, a DB error — is exactly what the ladder is for, and a
// fork of it may well succeed (CONVENTIONS §21).

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

func TestRW14PreambleRefusalIsExactlyTheInfrastructureAbsences(t *testing.T) {
	ctx := context.Background()

	// ── The refusal class: each is an absent screen, and each MUST be marked.
	outages := []struct {
		name     string
		build    func(f *fix) *verify.Verifier
		sentinel error
	}{
		{
			name: "launch domain with no check pack",
			build: func(f *fix) *verify.Verifier {
				return f.verifier(&fakeJudge{}, &scriptRunner{}, nil)
			},
			sentinel: verify.ErrNoCheckPack,
		},
		{
			name: "no judge seam",
			build: func(f *fix) *verify.Verifier {
				v := f.verifier(nil, &scriptRunner{}, passPack())
				v.Judge = nil
				return v
			},
			sentinel: verify.ErrSeamMissing,
		},
		{
			name: "a pack with no runner",
			build: func(f *fix) *verify.Verifier {
				return f.verifier(&fakeJudge{}, nil, passPack())
			},
			sentinel: verify.ErrSeamMissing,
		},
	}
	for _, c := range outages {
		t.Run("outage: "+c.name, func(t *testing.T) {
			f := newFix(t)
			f.seedTask("t1", "r1")
			_, err := c.build(f).Verify(ctx, input(deliverable("t1", "r1")))
			if _, ok := verify.AsPreambleRefusal(err); !ok {
				t.Fatalf("err = %v — an absent screen MUST be the preamble refusal class, or the ladder forks it forever", err)
			}
			if !errors.Is(err, c.sentinel) {
				t.Fatalf("err = %v, want it to still carry %v (marking never hides the cause)", err, c.sentinel)
			}
		})
	}

	// ── NOT the refusal class: a mechanical mid-drain failure. The judge is
	// wired and the pack is present — the drain STARTED and died in flight.
	t.Run("mechanical: the judge transport fails mid-drain", func(t *testing.T) {
		f := newFix(t)
		f.seedTask("t1", "r1")
		boom := errors.New("judge transport: connection reset")
		j := &fakeJudge{compliance: func(verify.JudgeInput) (verify.Axis1Result, error) {
			return verify.Axis1Result{}, boom
		}}
		_, err := f.verifier(j, &scriptRunner{}, passPack()).Verify(ctx, input(deliverable("t1", "r1")))
		if err == nil {
			t.Fatal("a dead judge must fail the drain")
		}
		if _, ok := verify.AsPreambleRefusal(err); ok {
			t.Fatalf("err = %v — a MECHANICAL failure must stay a crash for the ladder (CONVENTIONS §21)", err)
		}
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want the transport cause", err)
		}
	})

	// ── Nor a caller defect: a malformed deliverable is nobody's outage.
	t.Run("mechanical: a malformed deliverable", func(t *testing.T) {
		f := newFix(t)
		f.seedTask("t1", "r1")
		d := deliverable("t1", "r1")
		d.Revision = 0
		_, err := f.verifier(&fakeJudge{}, &scriptRunner{}, passPack()).Verify(ctx, input(d))
		if _, ok := verify.AsPreambleRefusal(err); ok {
			t.Fatalf("err = %v — a malformed input is a caller defect, not a missing screen", err)
		}
		if !errors.Is(err, verify.ErrBadInput) {
			t.Fatalf("err = %v, want ErrBadInput", err)
		}
	})
}

// TestRW14InfrastructureCardShapeAndVerbs: the card the refusal raises carries
// the ratified route (CHECK-INTEGRITY + decision card + quarantine, OQ1) and
// exactly the two verbs (OQ2) — with accept_best_effort refused loudly, because
// V1 never ran and SetVerified stays the only path to verified (S05.1/S07.11).
func TestRW14InfrastructureCardShapeAndVerbs(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")

	esc := &verify.Escalator{DB: f.db, Log: f.log, Settings: testSettings{base: f.reg}}
	cause := errors.New("no build, test or lint commands are captured for project \"shop\"")
	card, err := esc.Raise(ctx, verify.InfrastructureEscalation(deliverable("t1", "r1"), "u1", cause))
	if err != nil {
		t.Fatalf("Raise: %v", err)
	}
	switch {
	case card.Category != verify.CatCheckIntegrity:
		t.Fatalf("category = %s, want CHECK-INTEGRITY (OQ1)", card.Category)
	case card.Kind != verify.SinkDecisionCard:
		t.Fatalf("sink = %s, want the decision card (S07.7 sink-types-only)", card.Kind)
	case !card.Infrastructure:
		t.Fatal("the card is not marked as the infrastructure card, so the answer surface cannot route its verbs")
	case card.Quarantined == "":
		t.Fatal("the unusable suite is not quarantined pending fix (the ratified CHECK-INTEGRITY route)")
	case card.SLA.Class != verify.SLAApproval || card.SLA.RemindAfterHours == 0:
		t.Fatalf("SLA = %+v, want the approval-class ⚙ numbers", card.SLA)
	}
	// It names the exact missing thing — that is the whole point of the door.
	if want := "shop"; !strings.Contains(card.Summary, want) {
		t.Fatalf("summary %q does not name what is missing", card.Summary)
	}

	for _, verb := range []string{"retry", "cancel"} {
		if err := verify.ValidateAnswer(card, verify.Answer{Choice: verify.AnswerVerb(verb)}); err != nil {
			t.Fatalf("verb %q refused: %v", verb, err)
		}
	}
	for _, verb := range []string{"accept_best_effort", "revise_with_guidance", "fix_suite", "waive_check", ""} {
		err := verify.ValidateAnswer(card, verify.Answer{Choice: verify.AnswerVerb(verb)})
		if verb == "" {
			if !errors.Is(err, verify.ErrBadAnswer) {
				t.Fatalf("empty verb answered %v, want ErrBadAnswer", err)
			}
			continue
		}
		if !errors.Is(err, verify.ErrUnsupportedAnswer) {
			t.Fatalf("verb %q answered %v, want ErrUnsupportedAnswer", verb, err)
		}
	}
	// …and `retry` is NOT admitted on an ordinary rework card.
	rework := verify.Card{Category: verify.CatCapHit, Choices: []string{"accept_best_effort", "revise_with_guidance", "cancel"}}
	if err := verify.ValidateAnswer(rework, verify.Answer{Choice: verify.VerbRetry}); !errors.Is(err, verify.ErrUnsupportedAnswer) {
		t.Fatalf("retry on a CAP-HIT card answered %v, want ErrUnsupportedAnswer", err)
	}
}

// TestRW14ABadPackSurfacingMidDrainCardsRatherThanCrashes — P3-RW-14A drain D3.
// The composition root validates a pack before handing it over, so today the
// invalid-pack refusal is caught in the preamble. A pack source that skips that
// (a future resolver, a hand-written Verifier) would surface the identical
// deterministic refusal from inside RunV1 — and an unmarked one takes the
// crash→fork→tombstone backstop instead of the R4 card door. The mark makes the
// class independent of WHERE the pack was checked.
func TestRW14ABadPackSurfacingMidDrainCardsRatherThanCrashes(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")

	// Passes the preamble (a pack IS present, and a runner with it) and fails
	// its own contract only when V1 executes it.
	bad := &verify.CheckPack{Domain: verify.DomainSoftware, Version: 1, VerifiedOn: time.Now()}
	_, err := f.verifier(&fakeJudge{}, &scriptRunner{}, bad).Verify(ctx, input(deliverable("t1", "r1")))
	if err == nil {
		t.Fatal("an invalid pack must fail the drain")
	}
	if _, ok := verify.AsPreambleRefusal(err); !ok {
		t.Fatalf("err = %v — an unusable suite is a screen outage wherever it is caught (S07.2), never a crash the ladder re-forks", err)
	}
	if !errors.Is(err, verify.ErrBadPack) {
		t.Fatalf("err = %v, want it to still carry ErrBadPack", err)
	}
}

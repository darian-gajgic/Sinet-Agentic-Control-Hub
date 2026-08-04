package verify_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// Drain-level acceptance: the SHIP path records the S07.11 verdict row
// (reasons, criteria, judge identity, golden-set honesty, keep-forever) and
// flips the ledger's verified status through SetVerified — the only path to
// verified.

func TestShipPathRecordsAndVerifies(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	v := f.verifier(&fakeJudge{}, &scriptRunner{}, passPack())
	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Verdict != verify.VerdictShip {
		t.Fatalf("verdict %s", out.Verdict)
	}

	// The keep-forever verdict row with its reasons (Spec S07.11).
	rounds := f.events(verify.EventRound)
	if len(rounds) != 1 {
		t.Fatalf("verify.round rows: %d", len(rounds))
	}
	var p map[string]any
	if err := json.Unmarshal(rounds[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	for field, want := range map[string]any{
		"verdict":           "SHIP",
		"rubric_id":         "rubric-software",
		"judge_model":       "fake-judge-1",
		"self_family_judge": true, // always flagged (G1 Def.1)
		"retention":         "keep-forever",
		"domain":            "software",
	} {
		if p[field] != want {
			t.Fatalf("verdict row %s = %v, want %v", field, p[field], want)
		}
	}
	if got := p["ac_ids"].([]any); len(got) != 2 {
		t.Fatalf("criteria ids: %v", got)
	}
	if got := p["passed"].([]any); len(got) != 2 {
		t.Fatalf("passed: %v", got)
	}
	golden := p["golden_set"].(map[string]any)
	if golden["measured"] != true {
		t.Fatal("golden-set rates must be measured (rubric v2 — the B4-7 rider-1 P-T06-5 run on opus-4-8)")
	}
	if p["content_sha256"] == "" {
		t.Fatal("verdict row without the revision content hash")
	}

	// The event row is attributed to the run's owner (15.6).
	if rounds[0].UserID != "u1" {
		t.Fatalf("verdict row owner %q", rounds[0].UserID)
	}

	// SetVerified flipped the AC-satisfying item, evidenced by the verdict
	// row (G1 Def.12; the ONLY path to verified).
	if len(out.VerifiedItems) != 1 || out.VerifiedItems[0] != "w1" {
		t.Fatalf("verified items: %v", out.VerifiedItems)
	}
	doc, _, err := f.ledger.Current(context.Background(), "t1")
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	item := doc.State.Items[0]
	if item.Status != ledger.StatusVerified {
		t.Fatalf("item status %s", item.Status)
	}
	if !strings.HasPrefix(item.EvidenceRef, "run_events/") {
		t.Fatalf("evidence ref %q must name the verdict row", item.EvidenceRef)
	}
}

func TestUnknownACNeverShips(t *testing.T) {
	// w1 cites AC-1 and AC-2; a judge Unknown on AC-1 is the S07.5 Unknown
	// escape: it synthesizes a blocker citing AC-1, so the round is REVISE,
	// and with no executor seam the drain terminates in a CAP-HIT card —
	// never SHIP, never silent, nothing verified. (Corrected at the B2 gate
	// demo, 2026-07-20: an all-Unknown round had shipped clean.)
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{
		compliance: func(in verify.JudgeInput) (verify.Axis1Result, error) {
			res := passAll(in)
			res.Verdicts[0].Pass = false
			res.Verdicts[0].Unknown = true
			return res, nil
		},
	}
	v := f.verifier(j, &scriptRunner{}, passPack())
	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Verdict == verify.VerdictShip || out.Verdict == verify.VerdictShipWithNotes {
		t.Fatalf("undecided criterion shipped: verdict %s", out.Verdict)
	}
	if len(out.VerifiedItems) != 0 {
		t.Fatalf("item verified on an Unknown criterion: %v", out.VerifiedItems)
	}
	if out.Card == nil || out.Card.Category != verify.CatCapHit {
		t.Fatalf("want the drain to terminate in a CAP-HIT card, got %+v", out.Card)
	}
	found := false
	for _, fd := range out.Rounds[0].Findings {
		if fd.Criterion == "AC-1" && fd.Anchor == "unknown:AC-1" && fd.Severity == verify.SeverityBlocker {
			found = true
		}
	}
	if !found {
		t.Fatalf("no synthesized unknown-escape blocker citing AC-1: %+v", out.Rounds[0].Findings)
	}
}

func TestUnknownResolvedByReworkShips(t *testing.T) {
	// The escape gives rework its chance: round 1's Unknown on AC-1 drives
	// a REVISE round; the round-2 judge decides every criterion → SHIP with
	// the item verified. The unknown-escape key resolves instead of
	// recurring.
	f := newFix(t)
	f.seedTask("t1", "r1")
	round := 0
	j := &fakeJudge{
		compliance: func(in verify.JudgeInput) (verify.Axis1Result, error) {
			round++
			res := passAll(in)
			if round == 1 {
				res.Verdicts[0].Pass = false
				res.Verdicts[0].Unknown = true
			}
			return res, nil
		},
	}
	v := f.verifier(j, &scriptRunner{}, passPack())
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		d := pkg.Deliverable
		d.PrevContent = d.Content
		d.Content = d.Content + "// decidable now\n"
		d.Revision++
		return d, nil
	}
	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Verdict != verify.VerdictShip {
		t.Fatalf("verdict %s, want SHIP once the criterion is decided", out.Verdict)
	}
	if len(out.VerifiedItems) != 1 || out.VerifiedItems[0] != "w1" {
		t.Fatalf("verified items %v, want [w1]", out.VerifiedItems)
	}
}

func TestLaunchDomainRequiresPack(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	v := f.verifier(&fakeJudge{}, &scriptRunner{}, nil)
	_, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if !errors.Is(err, verify.ErrNoCheckPack) {
		t.Fatalf("want ErrNoCheckPack, got %v", err)
	}
}

func TestJudgeSeamRequired(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	v := f.verifier(nil, &scriptRunner{}, passPack())
	v.Judge = nil
	_, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if !errors.Is(err, verify.ErrSeamMissing) {
		t.Fatalf("want ErrSeamMissing, got %v", err)
	}
}

func TestResearchCountersUnknownIsLoudNotSilent(t *testing.T) {
	// No metering seam wired (the per-step counter substrate is B2-4's):
	// the research check surfaces UNVERIFIABLE-HERE as a round-1 note —
	// never a fake pass, never a false card.
	f := newFix(t)
	f.seedTask("t1", "r1")
	v := f.verifier(&fakeJudge{}, &scriptRunner{}, passPack()) // v.Research stays nil
	in := input(deliverable("t1", "r1"))
	in.ResearchNodes = []intake.ResearchNode{{RuleID: "P47-9", StepID: "S-1"}}
	out, err := v.Verify(context.Background(), in)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Card != nil {
		t.Fatalf("unknown counters raised a false card: %+v", out.Card)
	}
	if out.Verdict != verify.VerdictShipWithNotes {
		t.Fatalf("verdict %s — the undecidable check must surface as a note", out.Verdict)
	}
	found := false
	for _, fd := range out.Rounds[0].Findings {
		if fd.Category == verify.CatResearchNotRun && strings.Contains(fd.Text, "undecidable") {
			found = true
		}
	}
	if !found {
		t.Fatalf("undecidable research check not surfaced: %+v", out.Rounds[0].Findings)
	}
}

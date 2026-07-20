package verify_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// S07.7 acceptance — liveness proof 1, the planted-defect e2e class:
// standing conformance-suite entries. For EVERY route-table category a
// synthetic task plants one defect of that category and the test asserts a
// human-visible card/alert/ticket reaches the inbox (the asks table) with
// its ratified SLA. "A finding that died in a log" is structurally
// impossible — proven here, not assumed (P46).

func TestRouteTableTotalAndSinkTypesOnly(t *testing.T) {
	want := []verify.Category{
		verify.CatACBlocker, verify.CatSanityBlocker, verify.CatReopenSpec,
		verify.CatCheckIntegrity, verify.CatResearchNotRun, verify.CatCapHit,
		verify.CatWorkerFlaw, verify.CatSafety,
	}
	if len(verify.RouteTable) != len(want) {
		t.Fatalf("route table has %d rows, want %d", len(verify.RouteTable), len(want))
	}
	validSinks := map[verify.Sink]bool{verify.SinkDecisionCard: true, verify.SinkAlert: true, verify.SinkFlawTicket: true}
	for _, cat := range want {
		route, ok := verify.RouteTable[cat]
		if !ok {
			t.Fatalf("category %s has no route", cat)
		}
		if !validSinks[route.Sink] {
			t.Fatalf("category %s routes to unknown sink %q — only three sink types exist", cat, route.Sink)
		}
		wantSLA := verify.SLAApproval
		if cat == verify.CatSafety {
			wantSLA = verify.SLASafety
		}
		if route.SLA != wantSLA {
			t.Fatalf("category %s SLA %s, want %s", cat, route.SLA, wantSLA)
		}
	}
}

// assertInboxCard asserts the one open ask matches the category's ratified
// route and SLA (⚙ defaults: remind 4 h, push 24 h, safety re-ping 1 h).
func assertInboxCard(t *testing.T, f *fix, cat verify.Category) verify.Card {
	t.Helper()
	card := f.oneOpenCard()
	if card.Category != cat {
		t.Fatalf("card category %s, want %s", card.Category, cat)
	}
	route := verify.RouteTable[cat]
	if card.Kind != route.Sink {
		t.Fatalf("card sink %s, want %s", card.Kind, route.Sink)
	}
	switch route.SLA {
	case verify.SLAApproval:
		if card.SLA.RemindAfterHours != 4 || card.SLA.PushAfterHours != 24 {
			t.Fatalf("approval SLA not from ⚙: %+v", card.SLA)
		}
	case verify.SLASafety:
		if !card.SLA.PushImmediately || card.SLA.RepingEveryHours != 1 {
			t.Fatalf("safety SLA not from ⚙: %+v", card.SLA)
		}
	}
	if len(f.events(verify.EventEscalation)) == 0 {
		t.Fatal("escalation not recorded in the event log")
	}
	return card
}

func TestPlantedACBlockerEscalates(t *testing.T) {
	// Axis 1 finds a systemic AC problem and escalates instead of another
	// round → AC-BLOCKER decision card in the inbox.
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{
		compliance: func(in verify.JudgeInput) (verify.Axis1Result, error) {
			res, _ := blockerOn("AC-1", "main.go:3", "the whole approach cannot satisfy AC-1")(in)
			res.Escalate = "structural: no rework round can fix this"
			return res, nil
		},
	}
	v := f.verifier(j, &scriptRunner{}, passPack())
	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Verdict != verify.VerdictEscalate {
		t.Fatalf("verdict %s", out.Verdict)
	}
	assertInboxCard(t, f, verify.CatACBlocker)
}

func TestPlantedSanityBlockerEscalates(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{
		sanity: func(verify.JudgeInput) (verify.Axis2Result, error) {
			r := cleanSanity()
			r.Escalate = "outcome-sanity failure beyond rework"
			return r, nil
		},
	}
	v := f.verifier(j, &scriptRunner{}, passPack())
	if _, err := v.Verify(context.Background(), input(deliverable("t1", "r1"))); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	assertInboxCard(t, f, verify.CatSanityBlocker)
}

func TestPlantedReopenSpecReachesInbox(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{
		sanity: func(verify.JudgeInput) (verify.Axis2Result, error) {
			r := cleanSanity()
			r.ReopenSpec = "compliant would be harmful"
			return r, nil
		},
	}
	v := f.verifier(j, &scriptRunner{}, passPack())
	if _, err := v.Verify(context.Background(), input(deliverable("t1", "r1"))); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	assertInboxCard(t, f, verify.CatReopenSpec)
}

func TestPlantedJudgeCheckDisagreementCardsAndQuarantines(t *testing.T) {
	// The V1 structured check for AC-2 PASSES; the judge says it fails →
	// CHECK-INTEGRITY card + the suite check quarantined pending fix; the
	// mechanical fact stands and the drain continues to SHIP.
	f := newFix(t)
	f.seedTask("t1", "r1")
	pack := passPack()
	j := &fakeJudge{
		compliance: func(in verify.JudgeInput) (verify.Axis1Result, error) {
			res := passAll(in)
			for i := range res.Verdicts {
				if res.Verdicts[i].Key == "AC-2" {
					res.Verdicts[i].Pass = false // disagree with the machine
				}
			}
			return res, nil
		},
	}
	v := f.verifier(j, &scriptRunner{}, pack)
	out, err := v.Verify(context.Background(), input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(out.IntegrityCards) != 1 {
		t.Fatalf("integrity cards %d", len(out.IntegrityCards))
	}
	c := out.IntegrityCards[0]
	if c.Category != verify.CatCheckIntegrity || c.Quarantined != "ac2" {
		t.Fatalf("integrity card: %+v", c)
	}
	if pack.Quarantines["ac2"].CheckID != "ac2" {
		t.Fatal("suite check not quarantined pending fix (S07.7 route)")
	}
	// Never an override: the verdict kept the mechanical fact — the drain
	// shipped despite the judge's disagreement, with the disagreement
	// recorded as a blocker finding on the card, not on the verdict.
	if out.Verdict != verify.VerdictShip {
		t.Fatalf("verdict %s — the V1 fact should stand", out.Verdict)
	}
}

func TestPlantedExecutorChallengeCards(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	v := f.verifier(&fakeJudge{}, &scriptRunner{}, passPack())
	card, err := v.ChallengeCheck(context.Background(), deliverable("t1", "r1"), "unit", "the fixture asserts the OLD contract")
	if err != nil {
		t.Fatalf("ChallengeCheck: %v", err)
	}
	if card.Category != verify.CatCheckIntegrity {
		t.Fatalf("challenge category %s", card.Category)
	}
	assertInboxCard(t, f, verify.CatCheckIntegrity)
}

func TestPlantedAuditFailureQuarantines(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	pack := passPack()
	v := f.verifier(&fakeJudge{}, &scriptRunner{}, pack)
	if _, err := v.RecordAuditFailure(context.Background(), deliverable("t1", "r1"), "ac2", "planted defect sailed through"); err != nil {
		t.Fatalf("RecordAuditFailure: %v", err)
	}
	if pack.Quarantines["ac2"].CheckID != "ac2" {
		t.Fatal("audit failure did not quarantine the check (P-T06-1)")
	}
	assertInboxCard(t, f, verify.CatCheckIntegrity)
}

func TestPlantedResearchNotRunReachesInbox(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{}
	v := f.verifier(j, &scriptRunner{}, passPack())
	v.Research = verify.CanaryResearchUsage() // every counter reads zero
	reruns := 0
	v.ResearchRerun = func(context.Context, intake.ResearchNode) error {
		reruns++
		return nil
	}
	in := input(deliverable("t1", "r1"))
	in.ResearchNodes = []intake.ResearchNode{{RuleID: "P47-2", StepID: "S-1", Query: "current pricing"}}
	out, err := v.Verify(context.Background(), in)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	// ⚙ verification.research_rerun_limit = 1: exactly one fresh-session
	// re-run, then the card; the judge never ran.
	if reruns != 1 {
		t.Fatalf("re-runs %d, want 1", reruns)
	}
	if j.complianceCalls != 0 {
		t.Fatal("RESEARCH-NOT-RUN should terminate before any paid judging")
	}
	if out.Card == nil {
		t.Fatal("no card")
	}
	assertInboxCard(t, f, verify.CatResearchNotRun)
}

func TestPlantedCapHitReachesInbox(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{compliance: blockerOn("AC-1", "main.go:3", "persistent defect")}
	v := f.verifier(j, &scriptRunner{}, passPack())
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		d := pkg.Deliverable
		d.Content += "// cosmetic\n"
		return d, nil
	}
	if _, err := v.Verify(context.Background(), input(deliverable("t1", "r1"))); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	card := assertInboxCard(t, f, verify.CatCapHit)
	if len(card.Rounds) == 0 || card.BestEffort == "" {
		t.Fatalf("CAP-HIT card without round history/best-effort: %+v", card)
	}
}

func TestPlantedWorkerFlawFilesTicket(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	v := f.verifier(&fakeJudge{}, &scriptRunner{}, passPack())
	card, err := v.RaiseWorkerFlaw(context.Background(), deliverable("t1", "r1"), verify.WorkerFlaw{
		Worker: "web-dev-default", Pattern: "drops error handling under load",
	})
	if err != nil {
		t.Fatalf("RaiseWorkerFlaw: %v", err)
	}
	if card.TicketRef == "" {
		t.Fatal("worker-flaw route without a ticket ref (S08 stub)")
	}
	got := assertInboxCard(t, f, verify.CatWorkerFlaw)
	if got.Kind != verify.SinkFlawTicket {
		t.Fatalf("sink %s", got.Kind)
	}
}

func TestPlantedSafetyAlertsOperator(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	v := f.verifier(&fakeJudge{}, &scriptRunner{}, passPack())
	if _, err := v.RaiseSafety(context.Background(), deliverable("t1", "r1"),
		"confinement violation: egress attempt from a C1 run", []string{"S11 probe detail"}); err != nil {
		t.Fatalf("RaiseSafety: %v", err)
	}
	card := assertInboxCard(t, f, verify.CatSafety)
	if !card.SLA.PushImmediately {
		t.Fatal("safety class must push immediately")
	}
}

func TestSLAConsumesSettings(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	v := f.verifier(&fakeJudge{}, &scriptRunner{}, passPack())
	v.Settings = testSettings{base: f.reg, ints: map[string]int64{
		"verification.card_remind_hours": 7,
		"verification.card_push_hours":   48,
	}}
	card, err := v.ChallengeCheck(context.Background(), deliverable("t1", "r1"), "unit", "challenge")
	if err != nil {
		t.Fatalf("ChallengeCheck: %v", err)
	}
	if card.SLA.RemindAfterHours != 7 || card.SLA.PushAfterHours != 48 {
		t.Fatalf("SLA not read from ⚙: %+v", card.SLA)
	}
}

func TestUnroutedCategoryFailsClosed(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	esc := &verify.Escalator{DB: f.db, Log: f.log, Settings: testSettings{base: f.reg}}
	_, err := esc.Raise(context.Background(), verify.Escalation{
		Category: "NOT-A-CATEGORY", TaskID: "t1", RunID: "r1", Owner: "u1", Summary: "x",
	})
	if err == nil {
		t.Fatal("unrouted category accepted — the sink gap must be impossible")
	}
}

// ---- Liveness proofs 2 & 3: canary + drill (shape and seams; scheduling
// is S14's, B5). ----

func TestCanaryCardAndSilenceWatch(t *testing.T) {
	f := newFix(t)
	f.task("canary-task", "u1")
	f.run("canary-run", "canary-task")
	v := f.verifier(&fakeJudge{}, &scriptRunner{}, passPack())

	watch := &verify.CanaryWatch{DB: f.db, Settings: testSettings{base: f.reg}}
	// Never ran: silent.
	silent, age, err := watch.Silent(context.Background(), time.Now())
	if err != nil || !silent || age >= 0 {
		t.Fatalf("empty canary history: silent=%v age=%v err=%v", silent, age, err)
	}

	card, err := v.RunCanary(context.Background(), "canary-task", "canary-run")
	if err != nil {
		t.Fatalf("RunCanary: %v", err)
	}
	if !card.Canary || !strings.HasPrefix(card.AskID, "canary-") {
		t.Fatalf("canary card not marked: %+v", card)
	}
	if card.Category != verify.CatResearchNotRun {
		t.Fatalf("canary plant category %s", card.Category)
	}

	// Fresh card → not silent; ⚙ verification.canary_interval_hours = 24
	// later → silent (the external watchdog alerts; wiring B5).
	silent, _, err = watch.Silent(context.Background(), time.Now())
	if err != nil || silent {
		t.Fatalf("fresh canary reported silent: %v %v", silent, err)
	}
	silent, age, err = watch.Silent(context.Background(), time.Now().Add(25*time.Hour))
	if err != nil || !silent || age < 24*time.Hour {
		t.Fatalf("stale canary not detected: silent=%v age=%v err=%v", silent, age, err)
	}
}

func TestDrillDue(t *testing.T) {
	s := regSettings(t) // ⚙ verification.drill_interval_days = 90
	now := time.Now()
	if due, err := verify.DrillDue(time.Time{}, now, s); err != nil || !due {
		t.Fatalf("never-drilled must be due: %v %v", due, err)
	}
	if due, err := verify.DrillDue(now.Add(-30*24*time.Hour), now, s); err != nil || due {
		t.Fatalf("recent drill reported due: %v %v", due, err)
	}
	if due, err := verify.DrillDue(now.Add(-91*24*time.Hour), now, s); err != nil || !due {
		t.Fatalf("overdue drill not reported: %v %v", due, err)
	}
}

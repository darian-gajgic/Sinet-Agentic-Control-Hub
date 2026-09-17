package verify_test

// tq7_kill_test.go — P3-TQ-7 acceptance battery: a failed captured check of
// the OWNER's pack is a V1 kill (Spec S07.1 "V0/V1 kill broken output before
// any paid call"). It mints exactly one blocker that cites the check —
// criterion and anchor `check:<id>`, the check's declared finding category
// (Spec S07.3: a stage contract declares its finding categories and their
// escalation routes; production packChecks declares AC-BLOCKER) — the round is
// REVISE and never SHIP, the finding rides the retry package with the check's
// own output (Spec S13.4), its key is round-stable so a persisting failure
// trips the S07.6 convergence stop into the CAP-HIT card, and no path reaches
// SetVerified (Spec S07.11). The S07.5 citation rule governs the JUDGE's
// findings: a judge finding citing a check id is demoted (goalposts stay
// fixed, S07.3 rule 7). A DETECTED pack (Spec S07.8 [A16]; P3-TQ-4a) is
// evidence and mints nothing.
//
// Committed RED at grounding (CONVENTIONS §3 amendment-A carve-out): today a
// CheckFailed outcome mints no finding (v1.go, the default arm of RunV1's
// ladder switch), validateFindings admits no `check:` criterion for anyone,
// and CheckResult.OutputTail is an inert surface nothing fills.
// TestTQ7DetectedPackFailuresAreEvidenceNotKills is GREEN by construction
// today (nothing mints) and is the guard that must stay green.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// tq7StageWords is the plain rendering of each ladder rung inside a finding
// (brief R3): the requester never reads the enum token.
var tq7StageWords = map[verify.LadderStage]string{
	verify.StageStatic: "lint, typecheck and build",
	verify.StageUnit:   "unit and integration tests",
	verify.StageSmoke:  "runtime smoke",
	verify.StageE2E:    "end-to-end",
}

// tq7OwnerPack is the production shape: packChecks
// (internal/shell/project_seams.go) binds NO step and NO criterion to the
// captured lint/build/test commands and declares AC-BLOCKER for each.
func tq7OwnerPack() *verify.CheckPack {
	return &verify.CheckPack{
		Domain: verify.DomainSoftware, Version: 3, VerifiedOn: time.Now().Add(-time.Hour),
		Checks: []verify.Check{
			{ID: "lint", Stage: verify.StageStatic, Argv: []string{"/bin/sh", "-lc", "npm run lint"}, FindingCategory: verify.CatACBlocker},
			{ID: "build", Stage: verify.StageStatic, Argv: []string{"/bin/sh", "-lc", "npm run build"}, FindingCategory: verify.CatACBlocker},
			{ID: "test", Stage: verify.StageUnit, Argv: []string{"/bin/sh", "-lc", "npm test"}, FindingCategory: verify.CatACBlocker},
		},
	}
}

// tq7DetectedPack is the same commands as the platform would detect them at a
// bootstrap round (P3-TQ-4a): posture stays bootstrap, and every rung carries
// the detected origin that the kill guard reads (P3-TQ-4a made origin a
// per-CHECK fact, because a pack can mix owner rungs with detected ones).
func tq7DetectedPack() *verify.CheckPack {
	p := tq7OwnerPack()
	p.Posture = verify.PostureBootstrap
	p.Provenance = verify.ProvenanceDetected
	for i := range p.Checks {
		p.Checks[i].Origin = verify.ProvenanceDetected
	}
	return p
}

// tq7Runner scripts exit codes per check with a bounded output tail, and can
// flip a check to passing after its first run (the "fixed by rework" shape).
type tq7Runner struct {
	exits     map[string]int
	tails     map[string]string
	errs      map[string]error
	passAfter map[string]int // check id → runs after which it exits 0
	runs      map[string]int
}

func (r *tq7Runner) RunCheck(_ context.Context, req verify.CheckRequest) (verify.CheckResult, error) {
	id := req.Check.ID
	if r.runs == nil {
		r.runs = map[string]int{}
	}
	r.runs[id]++
	if err := r.errs[id]; err != nil {
		return verify.CheckResult{}, err
	}
	code := r.exits[id]
	if n, ok := r.passAfter[id]; ok && r.runs[id] > n {
		code = 0
	}
	return verify.CheckResult{ExitCode: code, EvidenceRef: "evidence/" + id + ".log", OutputTail: r.tails[id]}, nil
}

// checkBlockers returns the AC-BLOCKER findings anchored check:*, by anchor.
func checkBlockers(fs []verify.Finding) map[string]verify.Finding {
	out := map[string]verify.Finding{}
	for _, f := range fs {
		if strings.HasPrefix(f.Anchor, "check:") && f.Category == verify.CatACBlocker {
			out[f.Anchor] = f
		}
	}
	return out
}

func tq7Sorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// tq7ItemStatus reads the ledger status of the seeded work item.
func tq7ItemStatus(t *testing.T, f *fix, taskID, itemID string) ledger.Status {
	t.Helper()
	doc, found, err := f.ledger.Current(context.Background(), taskID)
	if err != nil || !found {
		t.Fatalf("ledger Current(%s): found=%v err=%v", taskID, found, err)
	}
	for _, it := range doc.State.Items {
		if it.ID == itemID {
			return it.Status
		}
	}
	t.Fatalf("no work item %s on the ledger: %+v", itemID, doc.State.Items)
	panic("unreachable")
}

// TestTQ7FailedOwnerCheckIsAKill [R1, R2, R3, R7, R10, R11]: the live P46
// instance. The owner's captured build fails while a content judge marks every
// criterion met. The round mints exactly one blocker anchored check:build,
// citing the check, in plain words naming the check, its rung and the exit
// status; the passed lint and the blocked-upstream test mint nothing; the
// verdict is REVISE, the terminal is the CAP-HIT card carrying the finding,
// nothing is verified, no integrity card is raised, and the verify.v1 row
// carries the finding.
func TestTQ7FailedOwnerCheckIsAKill(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{}
	v := f.verifier(j, &tq7Runner{exits: map[string]int{"build": 1}}, tq7OwnerPack())

	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(out.Rounds) == 0 {
		t.Fatal("no rounds recorded")
	}
	rd := out.Rounds[0]
	fd, ok := findingAt(rd.Findings, "check:build")
	if !ok {
		t.Fatalf("no finding anchored check:build — the failed build died in a log (Spec S07.1/S07.7): %+v", rd.Findings)
	}
	if fd.Severity != verify.SeverityBlocker || fd.Demoted {
		t.Fatalf("finding is %q (demoted=%v), want an undemoted blocker", fd.Severity, fd.Demoted)
	}
	if fd.Category != verify.CatACBlocker || fd.Criterion != "check:build" {
		t.Fatalf("finding category/criterion = %q/%q, want the check's declared AC-BLOCKER citing check:build", fd.Category, fd.Criterion)
	}
	if got, want := fd.Key(), verify.FindingKey("check:build|check:build|AC-BLOCKER"); got != want {
		t.Fatalf("finding key %q, want %q (round-stable, Spec S07.6)", got, want)
	}
	for _, want := range []string{`"build"`, tq7StageWords[verify.StageStatic], "status 1"} {
		if !strings.Contains(fd.Text, want) {
			t.Fatalf("finding text %q does not say %q", fd.Text, want)
		}
	}
	if strings.Contains(fd.Text, "evidence/") {
		t.Fatalf("finding text %q carries a platform path; the log stays a ref on the outcome row", fd.Text)
	}
	assertPlainWords(t, fd.Text)
	for _, anchor := range []string{"check:lint", "check:test"} {
		if stray, ok := findingAt(rd.Findings, anchor); ok {
			t.Fatalf("a check that did not FAIL minted a finding: %+v", stray)
		}
	}
	if rd.V1 == nil {
		t.Fatal("no V1 record on the round")
	}
	for _, c := range rd.V1.Checks {
		switch c.CheckID {
		case "build":
			if c.State != verify.CheckFailed || c.ExitCode != 1 {
				t.Fatalf("build outcome %+v, want FAIL exit 1", c)
			}
		case "test":
			if c.State != verify.CheckUnverifiable || c.AttributedTo != "build" {
				t.Fatalf("test outcome %+v, want UNVERIFIABLE-HERE attributed to build (first upstream failure)", c)
			}
		}
	}
	if rd.Verdict != verify.VerdictRevise {
		t.Fatalf("round verdict %s, want REVISE — a failed owner check is a kill (Spec S07.1)", rd.Verdict)
	}
	if out.Verdict != verify.VerdictEscalate || out.Card == nil || out.Card.Category != verify.CatCapHit {
		t.Fatalf("terminal %s / card %+v, want ESCALATE on the CAP-HIT card (REVISE with no retry seam)", out.Verdict, out.Card)
	}
	if _, ok := findingAt(out.Card.Findings, "check:build"); !ok {
		t.Fatalf("the card does not carry the check finding: %+v", out.Card.Findings)
	}
	if len(out.VerifiedItems) != 0 {
		t.Fatalf("a killed round verified %v", out.VerifiedItems)
	}
	if got := tq7ItemStatus(t, f, "t1", "w1"); got != ledger.StatusDoneUnverified {
		t.Fatalf("work item w1 is %q, want done_unverified — SetVerified must be unreachable on a killed round (Spec S07.11)", got)
	}
	if len(out.IntegrityCards) != 0 {
		t.Fatalf("a failed check is the work's fault, not the suite's, yet an integrity card was raised: %+v", out.IntegrityCards)
	}
	if j.complianceCalls != 1 {
		t.Fatalf("compliance calls %d, want 1 (one judged round, then the card)", j.complianceCalls)
	}

	rows := f.events(verify.EventV1)
	if len(rows) == 0 {
		t.Fatal("no verify.v1 row")
	}
	var rec verify.V1Result
	if err := json.Unmarshal(rows[len(rows)-1].Payload, &rec); err != nil {
		t.Fatalf("decode verify.v1: %v", err)
	}
	if _, ok := findingAt(rec.Findings, "check:build"); !ok {
		t.Fatalf("verify.v1 row does not carry the check finding: %s", string(rows[len(rows)-1].Payload))
	}
}

// TestTQ7CheckOutputRidesTheRetryPackageAndTheKeyResolves [R4, R7, R8]: the
// finding enters the retry package as a numbered point whose text carries the
// check's own output, so the fresh executor can act on it (Spec S13.4). When
// the reworked revision passes the check, the key resolves: the next round
// mints nothing, SHIPs, and verifies the ledger item.
func TestTQ7CheckOutputRidesTheRetryPackageAndTheKeyResolves(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	tail := "src/app.ts(3,1): error TS2304: Cannot find name 'greet'.\nnpm ERR! code 2\n"
	r := &tq7Runner{exits: map[string]int{"build": 2}, tails: map[string]string{"build": tail}, passAfter: map[string]int{"build": 1}}
	j := &fakeJudge{}
	v := f.verifier(j, r, tq7OwnerPack())
	var pkgs []verify.RetryPackage
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		pkgs = append(pkgs, pkg)
		d := pkg.Deliverable
		d.Content += "// greet defined per F1\n"
		return d, nil
	}

	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("retry packages %d, want exactly 1 (round 1 REVISE, round 2 SHIP)", len(pkgs))
	}
	fd, ok := findingAt(pkgs[0].Findings, "check:build")
	if !ok {
		t.Fatalf("the retry package carries no check:build point: %+v", pkgs[0].Findings)
	}
	if fd.Severity != verify.SeverityBlocker || fd.N == 0 {
		t.Fatalf("retry point %+v, want a numbered blocker", fd)
	}
	if !strings.Contains(fd.Text, "Cannot find name 'greet'") {
		t.Fatalf("retry point text %q does not carry the check's output — the executor would have to rebuild blind (Spec S13.4)", fd.Text)
	}
	if !strings.Contains(fd.Text, "status 2") {
		t.Fatalf("retry point text %q does not name the exit status", fd.Text)
	}
	if len(out.Rounds) != 2 {
		t.Fatalf("rounds %d, want 2", len(out.Rounds))
	}
	if _, ok := findingAt(out.Rounds[1].Findings, "check:build"); ok {
		t.Fatalf("the check passed on the reworked revision and still minted: %+v", out.Rounds[1].Findings)
	}
	if out.Verdict != verify.VerdictShip {
		t.Fatalf("verdict %s, want SHIP once the check passes", out.Verdict)
	}
	if len(out.VerifiedItems) != 1 || out.VerifiedItems[0] != "w1" {
		t.Fatalf("verified items %v, want [w1]", out.VerifiedItems)
	}
	if len(j.inputs) != 2 {
		t.Fatalf("judge inputs %d, want 2", len(j.inputs))
	}
	carried := false
	for _, p := range j.inputs[1].PriorFindings {
		if p.Anchor == "check:build" && p.Text == fd.Text {
			carried = true
		}
	}
	if !carried {
		t.Fatalf("the round-2 judge did not see the check finding verbatim in its prior-findings scope (Spec S07.6): %+v", j.inputs[1].PriorFindings)
	}
}

// TestTQ7PersistingCheckFailTripsTheConvergenceStop [R8]: the key is
// round-stable, so a build that keeps failing across rework rounds recurs
// unresolved and trips the S07.6 convergence stop into the CAP-HIT card
// (patience 2, cap raised to 5 so convergence fires first).
func TestTQ7PersistingCheckFailTripsTheConvergenceStop(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	v := f.verifier(&fakeJudge{}, &tq7Runner{exits: map[string]int{"build": 1}}, tq7OwnerPack())
	v.Settings = testSettings{base: f.reg, ints: map[string]int64{"verification.rework_rounds": 5}}
	rounds := 0
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		rounds++
		d := pkg.Deliverable
		d.Content = strings.Repeat(fmt.Sprintf("attempt %d rewritten from scratch\nblock-%d\n", rounds, rounds), 5+rounds)
		return d, nil
	}

	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(out.Rounds) != 3 {
		t.Fatalf("rounds %d, want 3 (the same check key recurring unresolved converges before the cap)", len(out.Rounds))
	}
	var keys []verify.FindingKey
	for _, rd := range out.Rounds {
		fd, ok := findingAt(rd.Findings, "check:build")
		if !ok {
			t.Fatalf("round %d carries no check:build finding: %+v", rd.Round, rd.Findings)
		}
		if rd.Verdict != verify.VerdictRevise {
			t.Fatalf("round %d verdict %s, want REVISE", rd.Round, rd.Verdict)
		}
		keys = append(keys, fd.Key())
	}
	for _, k := range keys[1:] {
		if k != keys[0] {
			t.Fatalf("check finding key drifted across rounds: %v", keys)
		}
	}
	if out.Card == nil || out.Card.Category != verify.CatCapHit || !strings.Contains(out.Card.Summary, "finding keys") {
		t.Fatalf("convergence card %+v, want CAP-HIT on recurring finding keys", out.Card)
	}
	if len(out.VerifiedItems) != 0 {
		t.Fatalf("a converged drain verified %v", out.VerifiedItems)
	}
}

// TestTQ7TheJudgeMayNeverCiteACheck [R5, R6]: `check:<id>` is admissible as a
// criterion ONLY on the platform's own V1 finding. A judge finding citing a
// check id is demoted to a note whether the check passed or failed, and when
// the check failed the only undemoted blocker citing it is the platform's.
func TestTQ7TheJudgeMayNeverCiteACheck(t *testing.T) {
	ctx := context.Background()
	judgeCites := func(in verify.JudgeInput) (verify.Axis1Result, error) {
		res := passAll(in)
		res.Findings = []verify.Finding{{
			Severity: verify.SeverityBlocker, Category: verify.CatACBlocker,
			Criterion: "check:build", Anchor: "src/app.ts:3", Text: "the build is broken (judge)",
		}}
		for i := 0; i < 6; i++ {
			res.Findings = append(res.Findings, verify.Finding{
				Severity: verify.SeverityBlocker, Category: verify.CatACBlocker,
				Criterion: fmt.Sprintf("check:c%d", i), Anchor: fmt.Sprintf("src/f%d.ts:1", i),
				Text: fmt.Sprintf("judge cites a check id %d", i),
			})
		}
		return res, nil
	}

	t.Run("checks pass", func(t *testing.T) {
		f := newFix(t)
		f.seedTask("t1", "r1")
		v := f.verifier(&fakeJudge{compliance: judgeCites}, &tq7Runner{}, tq7OwnerPack())
		out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		rd := out.Rounds[0]
		for _, fd := range rd.Findings {
			if strings.HasPrefix(fd.Criterion, "check:") && (fd.Severity != verify.SeverityNote || !fd.Demoted) {
				t.Fatalf("judge finding citing %q kept blocker severity: %+v", fd.Criterion, fd)
			}
		}
		if len(checkBlockers(rd.Findings)) != 0 {
			t.Fatalf("nothing failed, yet check-anchored blockers exist: %+v", rd.Findings)
		}
		if out.Verdict != verify.VerdictShipWithNotes {
			t.Fatalf("verdict %s, want SHIP-with-notes (demoted notes never spin a round)", out.Verdict)
		}
		if len(out.VerifiedItems) != 1 || out.VerifiedItems[0] != "w1" {
			t.Fatalf("verified items %v, want [w1]", out.VerifiedItems)
		}
	})

	t.Run("build fails", func(t *testing.T) {
		f := newFix(t)
		f.seedTask("t1", "r1")
		v := f.verifier(&fakeJudge{compliance: judgeCites}, &tq7Runner{exits: map[string]int{"build": 1}}, tq7OwnerPack())
		out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		rd := out.Rounds[0]
		judge, ok := findingAt(rd.Findings, "src/app.ts:3")
		if !ok {
			t.Fatalf("the judge's finding is gone: %+v", rd.Findings)
		}
		if judge.Severity != verify.SeverityNote || !judge.Demoted {
			t.Fatalf("judge finding citing check:build is %q (demoted=%v), want a demoted note — the judge may never cite a check id", judge.Severity, judge.Demoted)
		}
		undemoted := 0
		for _, fd := range rd.Findings {
			if fd.Criterion == "check:build" && fd.Severity == verify.SeverityBlocker && !fd.Demoted {
				undemoted++
				if fd.Anchor != "check:build" {
					t.Fatalf("an undemoted blocker citing check:build is anchored %q, not the check: %+v", fd.Anchor, fd)
				}
			}
		}
		if undemoted != 1 {
			t.Fatalf("undemoted blockers citing check:build = %d, want exactly the platform's one: %+v", undemoted, rd.Findings)
		}
		if rd.Verdict != verify.VerdictRevise || out.Card == nil || out.Card.Category != verify.CatCapHit {
			t.Fatalf("round %s / card %+v, want REVISE and the CAP-HIT card", rd.Verdict, out.Card)
		}
		if len(out.VerifiedItems) != 0 {
			t.Fatalf("a killed round verified %v", out.VerifiedItems)
		}
	})
}

// TestTQ7DetectedPackFailuresAreEvidenceNotKills [R9] — the guard. A pack the
// platform DETECTED at a bootstrap round (Spec S07.8 [A16]; P3-TQ-4a) records
// its failures as evidence and never mints the kill: the ladder still says
// FAIL on the outcome row, no check-anchored AC-BLOCKER exists, no REVISE is
// driven by it, and nothing is verified (the posture stays advisory). GREEN by
// construction today; it must stay green once the rule lands.
func TestTQ7DetectedPackFailuresAreEvidenceNotKills(t *testing.T) {
	ctx := context.Background()

	t.Run("ladder", func(t *testing.T) {
		res, err := verify.RunV1(ctx, tq7DetectedPack(), &tq7Runner{exits: map[string]int{"build": 1}}, v1req(), nil, nil, time.Now(), regSettings(t))
		if err != nil {
			t.Fatalf("RunV1: %v", err)
		}
		recorded := false
		for _, c := range res.Checks {
			if c.CheckID == "build" && c.State == verify.CheckFailed && c.ExitCode == 1 {
				recorded = true
			}
		}
		if !recorded {
			t.Fatalf("the detected build's failure is not recorded as evidence: %+v", res.Checks)
		}
		if got := checkBlockers(res.Findings); len(got) != 0 {
			t.Fatalf("a detected pack minted the kill: %+v", got)
		}
	})

	t.Run("drain", func(t *testing.T) {
		f := newFix(t)
		f.seedTask("t1", "r1")
		v := f.verifier(&fakeJudge{}, &tq7Runner{exits: map[string]int{"build": 1}}, tq7DetectedPack())
		out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		for _, rd := range out.Rounds {
			if got := checkBlockers(rd.Findings); len(got) != 0 {
				t.Fatalf("round %d: a detected pack minted the kill: %+v", rd.Round, got)
			}
			if rd.Verdict == verify.VerdictRevise {
				t.Fatalf("round %d is REVISE under a detected pack with no other blocker: %+v", rd.Round, rd.Findings)
			}
		}
		if out.Card != nil && out.Card.Category == verify.CatCapHit {
			t.Fatalf("the kill route fired for a detected pack: %+v", out.Card)
		}
		if len(out.VerifiedItems) != 0 {
			t.Fatalf("an advisory round verified %v", out.VerifiedItems)
		}
	})
}

// TestTQ7PropCheckBlockersAreExactlyTheFailedOwnerOutcomes [R1, R2, R9, R10
// property]: over generated owner packs (random rungs, step and criterion
// bindings, exit codes, runner failures, quarantines) the AC-BLOCKER findings
// anchored check:* are in bijection with the CheckFailed outcomes, each citing
// its own check with the declared category and naming the check, its rung and
// the exit status in plain words; a runner failure keeps exactly its
// CHECK-INTEGRITY blocker and a quarantine its CHECK-INTEGRITY note, and
// neither carries an AC-BLOCKER; the same pack with detected provenance mints
// no check-anchored AC-BLOCKER at all.
func TestTQ7PropCheckBlockersAreExactlyTheFailedOwnerOutcomes(t *testing.T) {
	ctx := context.Background()
	s := regSettings(t)
	stages := []verify.LadderStage{verify.StageStatic, verify.StageUnit, verify.StageSmoke, verify.StageE2E}
	fails, runnerFails, quarantines, detectedFails := 0, 0, 0, 0
	for seed := int64(1); seed <= 8; seed++ {
		r := rand.New(rand.NewSource(seed))
		for n := 1; n <= 6; n++ {
			base := &verify.CheckPack{Domain: verify.DomainSoftware, Version: 1, VerifiedOn: time.Now().Add(-time.Hour)}
			runner := &tq7Runner{exits: map[string]int{}, errs: map[string]error{}, tails: map[string]string{}}
			stageOf := map[string]verify.LadderStage{}
			var quarantined []string
			for k := 1; k <= n; k++ {
				id := fmt.Sprintf("c%d", k)
				c := verify.Check{ID: id, Stage: stages[r.Intn(len(stages))], Argv: []string{id}, FindingCategory: verify.CatACBlocker}
				if r.Intn(3) == 0 {
					c.StepID = fmt.Sprintf("S-%d", 1+r.Intn(2))
				}
				if r.Intn(4) == 0 {
					c.ACKey = fmt.Sprintf("AC-%d", 1+r.Intn(2))
					c.Provenance = "authored separately (S07.3 rule 4)"
				}
				base.Checks = append(base.Checks, c)
				stageOf[id] = c.Stage
				switch r.Intn(6) {
				case 0, 1:
					runner.exits[id] = 1 + r.Intn(3)
				case 2:
					runner.errs[id] = errors.New("sandbox compose failed")
				}
				runner.tails[id] = fmt.Sprintf("output of %s\n", id)
				if r.Intn(6) == 0 {
					quarantined = append(quarantined, id)
				}
			}
			for _, id := range quarantined {
				if err := base.QuarantineCheck(verify.Quarantine{CheckID: id, Owner: "op", Reason: "flaky", FixBy: time.Now().Add(48 * time.Hour)}); err != nil {
					t.Fatalf("seed %d n=%d: quarantine %s: %v", seed, n, id, err)
				}
			}
			for _, detected := range []bool{false, true} {
				pack := *base
				pack.Checks = append([]verify.Check(nil), base.Checks...)
				if detected {
					pack.Posture = verify.PostureBootstrap
					pack.Provenance = verify.ProvenanceDetected
					for i := range pack.Checks {
						pack.Checks[i].Origin = verify.ProvenanceDetected
					}
				}
				res, err := verify.RunV1(ctx, &pack, runner, v1req(), ladderSteps()[:2], map[string][]string{"AC-1": {"S-1"}}, time.Now(), s)
				if err != nil {
					t.Fatalf("seed %d n=%d detected=%v: RunV1: %v", seed, n, detected, err)
				}
				want := map[string]bool{}
				for _, c := range res.Checks {
					anchor := "check:" + c.CheckID
					var integrity []verify.Finding
					for _, fd := range res.Findings {
						if fd.Anchor == anchor && fd.Category == verify.CatCheckIntegrity {
							integrity = append(integrity, fd)
						}
					}
					switch c.State {
					case verify.CheckFailed:
						if detected {
							detectedFails++
						} else {
							want[anchor] = true
							fails++
						}
						if len(integrity) != 0 {
							t.Fatalf("seed %d n=%d: a failed check carries an integrity finding: %+v", seed, n, integrity)
						}
					case verify.CheckRunnerFailed:
						runnerFails++
						// A DETECTED rung mints nothing at all — not the kill
						// this packet adds and not the runner-failure blocker
						// either (P3-TQ-4a, which landed after this battery
						// was grounded: a command nobody captured raises no
						// finding anywhere). Asserted as the stronger claim.
						if detected {
							if len(integrity) != 0 {
								t.Fatalf("seed %d n=%d: a detected rung's runner failure minted %+v", seed, n, integrity)
							}
							break
						}
						if len(integrity) != 1 || integrity[0].Severity != verify.SeverityBlocker {
							t.Fatalf("seed %d n=%d: runner failure on %s wants exactly one CHECK-INTEGRITY blocker, got %+v", seed, n, c.CheckID, integrity)
						}
					case verify.CheckQuarantined:
						quarantines++
						if len(integrity) != 1 || integrity[0].Severity != verify.SeverityNote {
							t.Fatalf("seed %d n=%d: quarantine of %s wants exactly one CHECK-INTEGRITY note, got %+v", seed, n, c.CheckID, integrity)
						}
					default:
						if len(integrity) != 0 {
							t.Fatalf("seed %d n=%d: %s outcome %s carries integrity findings: %+v", seed, n, c.CheckID, c.State, integrity)
						}
					}
				}
				got := map[string]bool{}
				for anchor, fd := range checkBlockers(res.Findings) {
					if fd.Severity != verify.SeverityBlocker || fd.Criterion != anchor {
						t.Fatalf("seed %d n=%d: check-anchored finding of the wrong shape: %+v", seed, n, fd)
					}
					got[anchor] = true
					id := strings.TrimPrefix(anchor, "check:")
					for _, w := range []string{fmt.Sprintf("%q", id), tq7StageWords[stageOf[id]], fmt.Sprintf("status %d", runner.exits[id])} {
						if !strings.Contains(fd.Text, w) {
							t.Fatalf("seed %d n=%d: finding text %q does not say %q", seed, n, fd.Text, w)
						}
					}
					assertPlainWords(t, fd.Text)
				}
				if g, w := tq7Sorted(got), tq7Sorted(want); strings.Join(g, ",") != strings.Join(w, ",") {
					t.Fatalf("seed %d n=%d detected=%v: check blockers %v, want exactly the FAIL outcomes %v (checks %+v)", seed, n, detected, g, w, res.Checks)
				}
			}
		}
	}
	if fails == 0 || runnerFails == 0 || quarantines == 0 || detectedFails == 0 {
		t.Fatalf("the generator left a class unexercised: fails=%d runnerFails=%d quarantines=%d detectedFails=%d", fails, runnerFails, quarantines, detectedFails)
	}
}

// TestTQ7PropARoundWithACheckBlockerNeverShips [R7 property]: whatever else
// the round carries — notes, demoted notes, integrity blockers that
// ComputeVerdict excludes by design — one undemoted check blocker makes the
// verdict REVISE, never SHIP or SHIP-with-notes.
func TestTQ7PropARoundWithACheckBlockerNeverShips(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for i := 0; i < 200; i++ {
		var fs []verify.Finding
		for k := r.Intn(5); k > 0; k-- {
			switch r.Intn(3) {
			case 0:
				fs = append(fs, verify.Finding{Severity: verify.SeverityNote, Category: verify.CatSanityBlocker, Anchor: "a"})
			case 1:
				fs = append(fs, verify.Finding{Severity: verify.SeverityNote, Category: verify.CatACBlocker, Criterion: "check:x", Anchor: "b", Demoted: true})
			case 2:
				fs = append(fs, verify.Finding{Severity: verify.SeverityBlocker, Category: verify.CatCheckIntegrity, Criterion: "CHECK-INTEGRITY", Anchor: "check:c"})
			}
		}
		at := r.Intn(len(fs) + 1)
		kill := verify.Finding{Severity: verify.SeverityBlocker, Category: verify.CatACBlocker, Criterion: "check:build", Anchor: "check:build"}
		fs = append(fs[:at], append([]verify.Finding{kill}, fs[at:]...)...)
		ax2 := cleanSanity()
		if got := verify.ComputeVerdict(&ax2, fs, ""); got != verify.VerdictRevise {
			t.Fatalf("case %d: verdict %s with a check blocker present, want REVISE: %+v", i, got, fs)
		}
	}
}

// TestTQ7CheckOutputInTheFindingIsBounded [R3, R4]: the finding carries the
// END of the check's output, bounded — a numbered point is a description, the
// full log stays a ref on the outcome row (P-T07-5). With no output the text
// still names the check and the status, and never a platform path.
func TestTQ7CheckOutputInTheFindingIsBounded(t *testing.T) {
	ctx := context.Background()
	s := regSettings(t)
	huge := strings.Repeat("noise line that says nothing useful about the failure\n", 400) + "LAST LINE: the actual error\n"
	res, err := verify.RunV1(ctx, tq7OwnerPack(), &tq7Runner{exits: map[string]int{"build": 1}, tails: map[string]string{"build": huge}}, v1req(), nil, nil, time.Now(), s)
	if err != nil {
		t.Fatalf("RunV1: %v", err)
	}
	fd, ok := findingAt(res.Findings, "check:build")
	if !ok {
		t.Fatalf("no finding anchored check:build: %+v", res.Findings)
	}
	if !strings.Contains(fd.Text, "LAST LINE: the actual error") {
		t.Fatalf("the finding does not carry the END of the output: %q", fd.Text)
	}
	if len(fd.Text) > 3072 {
		t.Fatalf("finding text is %d bytes for a %d-byte output; the tail is bounded at 2 KB plus the sentence", len(fd.Text), len(huge))
	}

	res, err = verify.RunV1(ctx, tq7OwnerPack(), &tq7Runner{exits: map[string]int{"build": 1}}, v1req(), nil, nil, time.Now(), s)
	if err != nil {
		t.Fatalf("RunV1: %v", err)
	}
	fd, ok = findingAt(res.Findings, "check:build")
	if !ok {
		t.Fatalf("no finding anchored check:build with empty output: %+v", res.Findings)
	}
	for _, w := range []string{`"build"`, "status 1"} {
		if !strings.Contains(fd.Text, w) {
			t.Fatalf("finding text %q does not say %q", fd.Text, w)
		}
	}
	if strings.Contains(fd.Text, "evidence/") {
		t.Fatalf("finding text %q carries a platform path", fd.Text)
	}
	assertPlainWords(t, fd.Text)
}

// TestTQ7SandboxRunnerHandsBackTheOutputTail [R4]: the production runner,
// which already reads the retained evidence for its hash, hands the bounded
// tail back on the result so RunV1 never re-reads a file.
func TestTQ7SandboxRunnerHandsBackTheOutputTail(t *testing.T) {
	conf := &fakeConfiner{}
	r := &verify.SandboxCheckRunner{Confiner: conf}
	res, err := r.RunCheck(context.Background(), verify.CheckRequest{
		RunID:       "r1",
		Check:       verify.Check{ID: "build", Stage: verify.StageStatic, Argv: []string{"run-checks", "1"}, FindingCategory: verify.CatACBlocker},
		Workspace:   t.TempDir(),
		EvidenceDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("RunCheck: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit %d, want 1", res.ExitCode)
	}
	if !strings.Contains(res.OutputTail, "check-output") {
		t.Fatalf("OutputTail %q does not carry the check's output", res.OutputTail)
	}
}

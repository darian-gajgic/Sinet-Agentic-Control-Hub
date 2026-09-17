package verify_test

// tq6_routing_test.go — P3-TQ-6 acceptance battery: a ladder (check-pack
// path) step-contract FAIL must route, and a judge axis-1 PASS that
// contradicts a V1 contract FAIL is a CHECK-INTEGRITY finding (Spec S07.3
// stage contracts with declared routes; S07.5 disagreement sentence + blocker
// citation rule; S07.6 round-stable finding keys; S07.7 every finding
// terminates in a human-visible sink; CONVENTIONS §15 CHECK-INTEGRITY never
// enters the REVISE drain; §74 the bootstrap precedent this mirrors).
//
// The two findings these bind (P3-TQ-3 evaluation, STATE 2026-09-17): on the
// pack path nothing reads V1Result.Steps except the verify.v1 row and the
// judge slice, so a FAIL contract has no verdict effect and reaches no human
// sink; and ValidateAxis1 realizes S07.5's disagreement only for ACKey-bound
// check outcomes, so a judge PASS over a step-contract FAIL raises nothing.
//
// Committed RED at grounding (CONVENTIONS §3 amendment-A carve-out): today
// RunV1 mints no contract finding (the coverage parameter is accepted and
// ignored), StepContract.Detail is empty on the ladder path, and
// ContractDisagreements is an inert stub returning no findings.

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// tq6Coverage is the ladder plan's AC coverage map: AC-1 is owned by S-1 (the
// step the failing static check binds), AC-2 by S-2 (the step blocked
// upstream). S-3 is covered by nothing.
func tq6Coverage() map[string][]string {
	return map[string][]string{"AC-1": {"S-1"}, "AC-2": {"S-2"}}
}

// tq6Input is the pack-path harness input with the ladder plan and coverage.
func tq6Input(coverage map[string][]string) verify.VerifyInput {
	in := input(deliverable("t1", "r1"))
	in.Steps = ladderSteps()
	in.Coverage = coverage
	return in
}

// failAll is a judge that agrees with a failing ladder: every criterion is
// marked not met, with no judge-authored findings, so any finding in the
// round is the platform's.
func failAll(in verify.JudgeInput) (verify.Axis1Result, error) {
	res := passAll(in)
	for i := range res.Verdicts {
		res.Verdicts[i].Pass = false
	}
	return res, nil
}

// internalTokens are never allowed in requester-facing finding text (the
// P3-TQ-3 T6 vocabulary plus this packet's anchor prefixes).
var internalTokens = []string{"S07", "Spec", "§", "AC-BLOCKER", "CHECK-INTEGRITY", "check-pack:", "tree:", "check:", "step:"}

func assertPlainWords(t *testing.T, text string) {
	t.Helper()
	for _, tok := range internalTokens {
		if strings.Contains(text, tok) {
			t.Fatalf("requester-facing finding text carries the internal token %q: %q", tok, text)
		}
	}
}

// lowestCoveringAC computes, in the test's own words, the criterion a FAIL
// on stepID must cite: the lowest-numbered AC-<n> whose coverage entry names
// the step, "" when none does (Spec S06.6 coverage map; S07.5 citation rule).
func lowestCoveringAC(stepID string, coverage map[string][]string) string {
	best, bestN := "", 0
	for ac, steps := range coverage {
		for _, s := range steps {
			if s != stepID {
				continue
			}
			n, err := strconv.Atoi(strings.TrimPrefix(ac, "AC-"))
			if err != nil {
				continue
			}
			if best == "" || n < bestN {
				best, bestN = ac, n
			}
		}
	}
	return best
}

func tq6Sorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestTQ6LadderContractFailReachesTheRequesterAsACBlocker [R1, R2, R3, R6]:
// on the check-pack path a step whose bound check fails is a FAIL contract,
// and that FAIL is ONE AC-BLOCKER blocker anchored step:<id>, citing the
// lowest criterion the plan's coverage gives the step, keyed exactly as the
// bootstrap path keys it, naming the failed check in plain words; it forces
// REVISE and lands the CAP-HIT card carrying it. Blocked-upstream and N-A
// contracts mint nothing.
func TestTQ6LadderContractFailReachesTheRequesterAsACBlocker(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{compliance: failAll}
	v := f.verifier(j, &scriptRunner{exits: map[string]int{"lint": 1}}, ladderPack())

	out, err := v.Verify(ctx, tq6Input(tq6Coverage()))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(out.Rounds) == 0 {
		t.Fatal("no rounds recorded")
	}
	r := out.Rounds[0]
	sc := stepByID(t, r, "S-1")
	if sc.State != verify.ContractFail || sc.AttributedTo != "lint" {
		t.Fatalf("S-1 contract %+v, want FAIL attributed to lint (pre-existing stepContracts semantics)", sc)
	}
	if !strings.Contains(sc.Detail, "lint") {
		t.Fatalf("S-1 FAIL detail %q does not name the check that failed, in plain words", sc.Detail)
	}
	assertPlainWords(t, sc.Detail)

	fd, ok := findingAt(r.Findings, "step:S-1")
	if !ok {
		t.Fatalf("no finding anchored to step:S-1 — a ladder contract FAIL died in a log (Spec S07.7): %+v", r.Findings)
	}
	if fd.Severity != verify.SeverityBlocker || fd.Demoted {
		t.Fatalf("finding is %q (demoted=%v), want an undemoted blocker", fd.Severity, fd.Demoted)
	}
	if fd.Category != verify.CatACBlocker || fd.Criterion != "AC-1" {
		t.Fatalf("finding category/criterion = %q/%q, want AC-BLOCKER citing AC-1 (the lowest criterion S-1 owns)", fd.Category, fd.Criterion)
	}
	if got, want := fd.Key(), verify.FindingKey("AC-1|step:S-1|AC-BLOCKER"); got != want {
		t.Fatalf("finding key %q, want %q — the same shape the bootstrap path mints (CONVENTIONS §74)", got, want)
	}
	if !strings.Contains(fd.Text, "S-1") || !strings.Contains(fd.Text, "lint") {
		t.Fatalf("finding text %q does not name the step and the check that failed", fd.Text)
	}
	assertPlainWords(t, fd.Text)
	for _, anchor := range []string{"step:S-2", "step:S-3"} {
		if stray, ok := findingAt(r.Findings, anchor); ok {
			t.Fatalf("a non-FAIL contract minted a finding: %+v", stray)
		}
	}
	if r.Verdict != verify.VerdictRevise {
		t.Fatalf("round verdict %s, want REVISE — a blocker forces the rework route", r.Verdict)
	}
	if out.Card == nil || out.Card.Category != verify.CatCapHit {
		t.Fatalf("want the CAP-HIT card (REVISE with no retry seam), got %+v", out.Card)
	}
	if _, ok := findingAt(out.Card.Findings, "step:S-1"); !ok {
		t.Fatalf("the card does not carry the contract finding: %+v", out.Card.Findings)
	}
	if len(out.VerifiedItems) != 0 {
		t.Fatalf("a FAIL round verified %v", out.VerifiedItems)
	}
	if len(out.IntegrityCards) != 0 {
		t.Fatalf("the judge agreed with the ladder, yet an integrity card was raised: %+v", out.IntegrityCards)
	}

	// The finding rides the verify.v1 row as the bootstrap path's does (Spec
	// S07.11).
	rows := f.events(verify.EventV1)
	if len(rows) == 0 {
		t.Fatal("no verify.v1 row")
	}
	var rec verify.V1Result
	if err := json.Unmarshal(rows[len(rows)-1].Payload, &rec); err != nil {
		t.Fatalf("decode verify.v1: %v", err)
	}
	if _, ok := findingAt(rec.Findings, "step:S-1"); !ok {
		t.Fatalf("verify.v1 row does not carry the contract finding: %s", string(rows[len(rows)-1].Payload))
	}
}

// TestTQ6JudgePassOverLadderContractFailIsCheckIntegrity [R4, R5, R7, R8]:
// the judge marks AC-1 met while the contract of the step that covers AC-1
// is FAIL. The disagreement is a CHECK-INTEGRITY blocker (criterion
// CHECK-INTEGRITY, anchored to the step and the criterion, plain words), the
// verdict record marks the disagreement without overriding the judge's
// verdict, and it routes to its own durable decision card — never the REVISE
// drain, which the AC-BLOCKER drives. A criterion covered by a blocked
// (UNVERIFIABLE-HERE) step raises no disagreement.
func TestTQ6JudgePassOverLadderContractFailIsCheckIntegrity(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{} // passes every criterion with the artifact as evidence
	v := f.verifier(j, &scriptRunner{exits: map[string]int{"lint": 1}}, ladderPack())

	out, err := v.Verify(ctx, tq6Input(tq6Coverage()))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	r := out.Rounds[0]
	fd, ok := findingAt(r.Findings, "step:S-1/AC-1")
	if !ok {
		t.Fatalf("no CHECK-INTEGRITY finding anchored step:S-1/AC-1 — the judge's PASS over a FAIL contract raised nothing (Spec S07.5): %+v", r.Findings)
	}
	if fd.Severity != verify.SeverityBlocker || fd.Demoted {
		t.Fatalf("disagreement is %q (demoted=%v), want an undemoted blocker", fd.Severity, fd.Demoted)
	}
	if fd.Category != verify.CatCheckIntegrity || fd.Criterion != string(verify.CatCheckIntegrity) {
		t.Fatalf("disagreement category/criterion = %q/%q, want CHECK-INTEGRITY/CHECK-INTEGRITY", fd.Category, fd.Criterion)
	}
	for _, want := range []string{"S-1", "criterion 1", "disagree", "lint"} {
		if !strings.Contains(fd.Text, want) {
			t.Fatalf("disagreement text %q does not say %q", fd.Text, want)
		}
	}
	assertPlainWords(t, fd.Text)
	if stray, ok := findingAt(r.Findings, "step:S-2/AC-2"); ok {
		t.Fatalf("a criterion covered by a blocked-upstream contract raised a disagreement: %+v", stray)
	}

	// The verdict record: the judge's PASS is kept (there is no AC-level
	// mechanical fact to substitute), the disagreement is marked.
	marked := false
	for _, av := range r.Axis1 {
		if av.Key != "AC-1" {
			continue
		}
		marked = true
		if !av.Pass || av.Unknown || av.FromV1 {
			t.Fatalf("AC-1 verdict %+v, want the judge's PASS kept (not FromV1, not Unknown)", av)
		}
		if !av.Disagreement {
			t.Fatalf("AC-1 verdict %+v, want Disagreement recorded", av)
		}
	}
	if !marked {
		t.Fatalf("no AC-1 verdict on the record: %+v", r.Axis1)
	}

	// Its sink is the CHECK-INTEGRITY decision card, raised mid-drain, with no
	// suite check quarantined (the FAIL is the fact that must keep standing).
	if len(out.IntegrityCards) != 1 {
		t.Fatalf("integrity cards %d, want exactly 1: %+v", len(out.IntegrityCards), out.IntegrityCards)
	}
	c := out.IntegrityCards[0]
	if c.Category != verify.CatCheckIntegrity || c.Kind != verify.RouteTable[verify.CatCheckIntegrity].Sink {
		t.Fatalf("integrity card category/kind = %q/%q", c.Category, c.Kind)
	}
	if c.Quarantined != "" {
		t.Fatalf("the disagreement quarantined %q — the contract's check is the mechanical fact, not a suspect", c.Quarantined)
	}
	if _, ok := findingAt(c.Findings, "step:S-1/AC-1"); !ok {
		t.Fatalf("integrity card carries no step:S-1/AC-1 finding: %+v", c.Findings)
	}
	durable := false
	for _, ask := range f.openAsks() {
		if ask.Category == verify.CatCheckIntegrity {
			if _, ok := findingAt(ask.Findings, "step:S-1/AC-1"); ok {
				durable = true
			}
		}
	}
	if !durable {
		t.Fatalf("no open CHECK-INTEGRITY ask row carries the disagreement: %+v", f.openAsks())
	}

	// The round is REVISE because of the AC-BLOCKER, never because of the
	// disagreement; nothing is verified.
	if r.Verdict != verify.VerdictRevise {
		t.Fatalf("round verdict %s, want REVISE driven by the contract's AC-BLOCKER", r.Verdict)
	}
	if out.Card == nil || out.Card.Category != verify.CatCapHit {
		t.Fatalf("terminal card %+v, want CAP-HIT", out.Card)
	}
	if len(out.VerifiedItems) != 0 {
		t.Fatalf("a disagreement round verified %v", out.VerifiedItems)
	}
}

// TestTQ6DisagreementIsRaisedOnTheBootstrapPathToo [R4, R9]: the same
// detector runs over a bootstrap round's tree-decided contracts — the P3-TQ-3
// T8 shape (pass-all judge, S-2 refuted by the tree, AC-1 owned by S-2) now
// raises the CHECK-INTEGRITY finding and its card, and the contract finding
// keys identically on both paths.
func TestTQ6DisagreementIsRaisedOnTheBootstrapPathToo(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	tree := t.TempDir()
	writeTree(t, tree, map[string]string{"src/app.ts": "export {}\n", "src/data/products.json": "[]"})
	v := f.verifier(&fakeJudge{}, nil, bootstrapPack())

	out, err := v.Verify(ctx, tq3Input(webshopSteps(), map[string][]string{"AC-1": {"S-2"}}, tree))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	r := out.Rounds[0]
	if sc := stepByID(t, r, "S-2"); sc.State != verify.ContractFail {
		t.Fatalf("S-2 contract %q, want FAIL (the landed P3-TQ-3 decision)", sc.State)
	}
	fd, ok := findingAt(r.Findings, "step:S-2/AC-1")
	if !ok {
		t.Fatalf("no CHECK-INTEGRITY finding anchored step:S-2/AC-1 on the bootstrap path: %+v", r.Findings)
	}
	if fd.Category != verify.CatCheckIntegrity || fd.Severity != verify.SeverityBlocker || fd.Demoted {
		t.Fatalf("disagreement finding %+v, want an undemoted CHECK-INTEGRITY blocker", fd)
	}
	assertPlainWords(t, fd.Text)
	if len(out.IntegrityCards) != 1 || out.IntegrityCards[0].Category != verify.CatCheckIntegrity {
		t.Fatalf("integrity cards %+v, want exactly one CHECK-INTEGRITY card", out.IntegrityCards)
	}
	contract, ok := findingAt(r.Findings, "step:S-2")
	if !ok {
		t.Fatalf("the landed bootstrap contract finding is gone: %+v", r.Findings)
	}
	if got, want := contract.Key(), verify.FindingKey("AC-1|step:S-2|AC-BLOCKER"); got != want {
		t.Fatalf("bootstrap contract finding key %q, want %q", got, want)
	}
}

// TestTQ6NoCoverageMapMeansNoDisagreementAndANote [R10]: a plan with no
// coverage map (the pre-A16 seed worlds) cannot say which criterion a step
// owns, so no disagreement can be computed and none is guessed; the contract
// FAIL still reaches the requester as a demoted note, exactly the bootstrap
// precedent (P3-TQ-3 T7).
func TestTQ6NoCoverageMapMeansNoDisagreementAndANote(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	v := f.verifier(&fakeJudge{}, &scriptRunner{exits: map[string]int{"lint": 1}}, ladderPack())

	out, err := v.Verify(ctx, tq6Input(nil))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	r := out.Rounds[0]
	fd, ok := findingAt(r.Findings, "step:S-1")
	if !ok {
		t.Fatalf("no finding anchored step:S-1 without a coverage map — the FAIL must still reach the requester: %+v", r.Findings)
	}
	if fd.Severity != verify.SeverityNote || !fd.Demoted || fd.Criterion != "" {
		t.Fatalf("finding %+v, want a demoted note citing nothing (Spec S07.5 citation rule)", fd)
	}
	for _, x := range r.Findings {
		if x.Category == verify.CatCheckIntegrity {
			t.Fatalf("a disagreement was computed without a coverage map: %+v", x)
		}
	}
	for _, av := range r.Axis1 {
		if av.Disagreement {
			t.Fatalf("verdict %+v marked a disagreement nobody could compute", av)
		}
	}
	if len(out.IntegrityCards) != 0 {
		t.Fatalf("integrity cards raised without a coverage map: %+v", out.IntegrityCards)
	}
	if out.Verdict != verify.VerdictShipWithNotes {
		t.Fatalf("verdict %s, want SHIP-with-notes — a note never spins a round", out.Verdict)
	}
	if out.Card != nil {
		t.Fatalf("a note raised a card: %+v", out.Card)
	}
}

// TestTQ6PropStepAnchoredBlockersAreExactlyTheFailContracts [R1, R2, R3
// property (a)+(c)]: for generated packs (one check per step at a random
// rung, random exit codes) and random coverage, the AC-BLOCKER findings
// anchored step:* are in bijection with the FAIL contracts by step id, each
// citing the lowest covering criterion; PASS, UNVERIFIABLE-HERE and N-A
// contracts mint nothing of any category.
func TestTQ6PropStepAnchoredBlockersAreExactlyTheFailContracts(t *testing.T) {
	ctx := context.Background()
	s := regSettings(t)
	stages := []verify.LadderStage{verify.StageStatic, verify.StageUnit, verify.StageSmoke, verify.StageE2E}
	failsSeen := 0
	for seed := int64(1); seed <= 3; seed++ {
		r := rand.New(rand.NewSource(seed))
		for n := 1; n <= 5; n++ {
			var steps []intake.Step
			pack := &verify.CheckPack{Domain: verify.DomainSoftware, Version: 1, VerifiedOn: time.Now().Add(-time.Hour)}
			exits := map[string]int{}
			for k := 1; k <= n; k++ {
				id := fmt.Sprintf("S-%d", k)
				steps = append(steps, intake.Step{ID: id, Title: "step", DoneWhen: fmt.Sprintf("check %d passes", k), Class: "C1"})
				cid := fmt.Sprintf("c%d", k)
				pack.Checks = append(pack.Checks, verify.Check{
					ID: cid, Stage: stages[r.Intn(len(stages))], Argv: []string{cid}, StepID: id,
					FindingCategory: verify.CatACBlocker,
				})
				exits[cid] = r.Intn(2)
			}
			coverage := map[string][]string{}
			for ac := 1; ac <= 3; ac++ {
				var owners []string
				for _, st := range steps {
					if r.Intn(2) == 1 {
						owners = append(owners, st.ID)
					}
				}
				if len(owners) > 0 {
					coverage[fmt.Sprintf("AC-%d", ac)] = owners
				}
			}
			if seed == 3 && n == 5 {
				coverage = nil // the pre-A16 shape
			}

			res, err := verify.RunV1(ctx, pack, &scriptRunner{exits: exits}, v1req(), steps, coverage, time.Now(), s)
			if err != nil {
				t.Fatalf("seed %d n=%d: RunV1: %v", seed, n, err)
			}
			want := map[string]bool{}
			for _, sc := range res.Steps {
				if sc.State == verify.ContractFail {
					want["step:"+sc.StepID] = true
					failsSeen++
				}
			}
			got := map[string]bool{}
			for _, fd := range res.Findings {
				if !strings.HasPrefix(fd.Anchor, "step:") {
					continue
				}
				if fd.Category != verify.CatACBlocker || fd.Severity != verify.SeverityBlocker {
					t.Fatalf("seed %d n=%d: step-anchored finding of the wrong shape: %+v", seed, n, fd)
				}
				if got[fd.Anchor] {
					t.Fatalf("seed %d n=%d: two findings anchored %s", seed, n, fd.Anchor)
				}
				got[fd.Anchor] = true
				stepID := strings.TrimPrefix(fd.Anchor, "step:")
				if !want[fd.Anchor] {
					t.Fatalf("seed %d n=%d: finding on a non-FAIL contract %s: %+v (steps %+v)", seed, n, stepID, fd, res.Steps)
				}
				if exp := lowestCoveringAC(stepID, coverage); fd.Criterion != exp {
					t.Fatalf("seed %d n=%d: %s cites %q, want %q (coverage %v)", seed, n, fd.Anchor, fd.Criterion, exp, coverage)
				}
			}
			if g, w := tq6Sorted(got), tq6Sorted(want); strings.Join(g, ",") != strings.Join(w, ",") {
				t.Fatalf("seed %d n=%d: step-anchored blockers %v, want exactly the FAIL contracts %v (exits %v, steps %+v)", seed, n, g, w, exits, res.Steps)
			}
		}
	}
	if failsSeen == 0 {
		t.Fatal("the generator produced no FAIL contract; the property was never exercised")
	}
}

// TestTQ6PropDisagreementIffJudgePassOverAFailContract [R4, R5 property (b)]:
// over generated verdict vectors, contract vectors and coverage maps, a
// CHECK-INTEGRITY finding exists for exactly the (criterion, step) pairs where
// the judge's own verdict is PASS (not Unknown, not taken from V1) and the
// step covering that criterion is FAIL; verdicts keep their Pass/Unknown/
// FromV1 and gain Disagreement on exactly the affected criteria.
func TestTQ6PropDisagreementIffJudgePassOverAFailContract(t *testing.T) {
	states := []verify.ContractState{verify.ContractPass, verify.ContractFail, verify.ContractNA, verify.ContractUnverifiable}
	pairsSeen := 0
	for seed := int64(1); seed <= 6; seed++ {
		r := rand.New(rand.NewSource(seed))
		for m := 1; m <= 4; m++ {
			for n := 1; n <= 4; n++ {
				var verdicts []verify.ACVerdict
				for a := 1; a <= m; a++ {
					v := verify.ACVerdict{Key: fmt.Sprintf("AC-%d", a), BoundTo: "plain"}
					switch r.Intn(4) {
					case 0:
						v.Pass = true
					case 1:
						v.Pass = false
					case 2:
						v.Unknown = true
					case 3:
						v.Pass, v.FromV1 = true, true
					}
					verdicts = append(verdicts, v)
				}
				var contracts []verify.StepContract
				for k := 1; k <= n; k++ {
					contracts = append(contracts, verify.StepContract{
						StepID: fmt.Sprintf("S-%d", k), DoneWhen: fmt.Sprintf("condition %d", k),
						State: states[r.Intn(len(states))], Detail: fmt.Sprintf("detail %d", k),
					})
				}
				coverage := map[string][]string{}
				for _, v := range verdicts {
					var owners []string
					for _, sc := range contracts {
						if r.Intn(2) == 1 {
							owners = append(owners, sc.StepID)
						}
					}
					if len(owners) > 0 {
						coverage[v.Key] = owners
					}
				}

				want := map[string]bool{}
				affected := map[string]bool{}
				for _, v := range verdicts {
					if v.Unknown || !v.Pass || v.FromV1 {
						continue
					}
					for _, sid := range coverage[v.Key] {
						for _, sc := range contracts {
							if sc.StepID == sid && sc.State == verify.ContractFail {
								want["step:"+sid+"/"+v.Key] = true
								affected[v.Key] = true
								pairsSeen++
							}
						}
					}
				}

				gotVerdicts, findings := verify.ContractDisagreements(verdicts, contracts, coverage)
				got := map[string]bool{}
				for _, fd := range findings {
					if fd.Category != verify.CatCheckIntegrity || fd.Severity != verify.SeverityBlocker || fd.Criterion != string(verify.CatCheckIntegrity) {
						t.Fatalf("seed %d m=%d n=%d: disagreement of the wrong shape: %+v", seed, m, n, fd)
					}
					if got[fd.Anchor] {
						t.Fatalf("seed %d m=%d n=%d: two findings anchored %s", seed, m, n, fd.Anchor)
					}
					got[fd.Anchor] = true
					rest := strings.TrimPrefix(fd.Anchor, "step:")
					sid, ac, ok := strings.Cut(rest, "/")
					if !ok || !strings.Contains(fd.Text, sid) || !strings.Contains(fd.Text, "criterion "+strings.TrimPrefix(ac, "AC-")) {
						t.Fatalf("seed %d m=%d n=%d: finding %+v does not name its step and criterion in plain words", seed, m, n, fd)
					}
				}
				if g, w := tq6Sorted(got), tq6Sorted(want); strings.Join(g, ",") != strings.Join(w, ",") {
					t.Fatalf("seed %d m=%d n=%d: disagreements %v, want exactly %v (verdicts %+v, contracts %+v, coverage %v)", seed, m, n, g, w, verdicts, contracts, coverage)
				}
				if len(gotVerdicts) != len(verdicts) {
					t.Fatalf("seed %d m=%d n=%d: %d verdicts returned for %d", seed, m, n, len(gotVerdicts), len(verdicts))
				}
				for i, v := range gotVerdicts {
					in := verdicts[i]
					if v.Key != in.Key || v.Pass != in.Pass || v.Unknown != in.Unknown || v.FromV1 != in.FromV1 {
						t.Fatalf("seed %d m=%d n=%d: verdict %+v was rewritten from %+v — the detector marks, never overrides", seed, m, n, v, in)
					}
					if v.Disagreement != affected[v.Key] {
						t.Fatalf("seed %d m=%d n=%d: %s Disagreement=%v, want %v", seed, m, n, v.Key, v.Disagreement, affected[v.Key])
					}
				}
			}
		}
	}
	if pairsSeen == 0 {
		t.Fatal("the generator produced no disagreement pair; the property was never exercised")
	}
}

// TestTQ6PersistingLadderFailTripsTheConvergenceStop [R6]: the contract
// finding's key is round-stable, so a check that keeps failing across rework
// rounds recurs unresolved and trips the S07.6 convergence stop into the
// CAP-HIT card (patience 2, cap raised to 5 so convergence fires first).
func TestTQ6PersistingLadderFailTripsTheConvergenceStop(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	j := &fakeJudge{compliance: failAll}
	v := f.verifier(j, &scriptRunner{exits: map[string]int{"lint": 1}}, ladderPack())
	v.Settings = testSettings{base: f.reg, ints: map[string]int64{"verification.rework_rounds": 5}}
	rounds := 0
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		rounds++
		d := pkg.Deliverable
		d.Content = strings.Repeat(fmt.Sprintf("attempt %d rewritten from scratch\nblock-%d\n", rounds, rounds), 5+rounds)
		return d, nil
	}

	out, err := v.Verify(ctx, tq6Input(map[string][]string{"AC-1": {"S-1"}}))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(out.Rounds) != 3 {
		t.Fatalf("rounds %d, want 3 (the same contract key recurring unresolved converges before the cap)", len(out.Rounds))
	}
	var keys []verify.FindingKey
	for _, r := range out.Rounds {
		fd, ok := findingAt(r.Findings, "step:S-1")
		if !ok {
			t.Fatalf("round %d carries no step:S-1 finding: %+v", r.Round, r.Findings)
		}
		keys = append(keys, fd.Key())
	}
	for _, k := range keys[1:] {
		if k != keys[0] {
			t.Fatalf("contract finding key drifted across rounds: %v", keys)
		}
	}
	if out.Card == nil || out.Card.Category != verify.CatCapHit || !strings.Contains(out.Card.Summary, "finding keys") {
		t.Fatalf("convergence card %+v, want CAP-HIT on recurring finding keys", out.Card)
	}
}

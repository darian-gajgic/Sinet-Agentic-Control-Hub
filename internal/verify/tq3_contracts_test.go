package verify_test

// tq3_contracts_test.go — P3-TQ-3 acceptance battery: at a bootstrap-posture
// round, every PLAN step's "Done when" contract that the produced TREE can
// decide is DECIDED at V1 — write-set globs, named files, structural facts —
// never from the executor's report (Spec S07.8 bootstrap bullet, A16 (3),
// 2026-09-17; Spec S07.3 stage contracts; S07.5 blocker rule; S07.7 route).
//
// The live defect these bind (TQ-F4): PLAN S-2 contracted photos "downloaded
// into `public/images/**`"; the executor shipped hotlinked placeholders and
// no public/images/ at all; bootstrapV1 marked the contract UNVERIFIABLE-HERE
// like every other and nobody looked at the tree.
//
// Committed RED at grounding (CONVENTIONS §3 amendment-A carve-out): today
// bootstrapV1 takes no tree, records check-pack:absent with an empty Detail
// for every step, and mints no finding.

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// writeTree materializes files (path → content) under root.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

// tq3Input is the harness input with the plan, coverage and tree overridden.
// The deliverable CONTENT is the executor's own claim of success — the one
// input the decision must never read.
func tq3Input(steps []intake.Step, coverage map[string][]string, tree string) verify.VerifyInput {
	d := deliverable("t1", "r1")
	d.Content = "Verified by construction: representative photos were downloaded into public/images/ and the catalogue is complete.\n"
	in := input(d)
	in.Steps = steps
	in.Coverage = coverage
	in.Workspace = tree
	return in
}

// webshopSteps is the TQ-F4 plan shape: S-1 scaffolds, S-2 promises images
// under public/images/** and data under src/data/**, S-3 is prose only.
func webshopSteps() []intake.Step {
	return []intake.Step{
		{ID: "S-1", Title: "scaffold", DoneWhen: "the app scaffold exists", Class: "C2", WriteSet: []string{"src/**"}},
		{ID: "S-2", Title: "catalogue", Class: "C2",
			DoneWhen: "representative free stock photos downloaded into `public/images/**` and product data written to `src/data/products.json`",
			WriteSet: []string{"src/data/**"}},
		{ID: "S-3", Title: "pricing", DoneWhen: "the price ranges are realistic and the data is believable", Class: "C1"},
	}
}

func stepByID(t *testing.T, r verify.RoundRecord, id string) verify.StepContract {
	t.Helper()
	if r.V1 == nil {
		t.Fatal("no V1 record — the bootstrap posture RUNS V1 (Spec S07.8)")
	}
	for _, sc := range r.V1.Steps {
		if sc.StepID == id {
			return sc
		}
	}
	t.Fatalf("no contract recorded for step %s: %+v", id, r.V1.Steps)
	panic("unreachable")
}

func findingAt(fs []verify.Finding, anchor string) (verify.Finding, bool) {
	for _, f := range fs {
		if f.Anchor == anchor {
			return f, true
		}
	}
	return verify.Finding{}, false
}

// TestTQ3WriteSetRefutedByTreeIsFAIL [R1, R2, R7, R14]: a step that declared a
// write set whose globs match nothing in the produced tree is FAIL — decided
// from the tree, attributed to the tree, with a reason — however loudly the
// deliverable text claims otherwise.
func TestTQ3WriteSetRefutedByTreeIsFAIL(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	tree := t.TempDir()
	writeTree(t, tree, map[string]string{"src/data/products.json": "[{\"id\":1}]"})
	steps := []intake.Step{
		{ID: "S-1", Title: "data", DoneWhen: "product data exists", Class: "C2", WriteSet: []string{"src/data/**"}},
		{ID: "S-2", Title: "images", DoneWhen: "photos are in place for every product", Class: "C2", WriteSet: []string{"public/images/**"}},
	}
	v := f.verifier(&fakeJudge{}, nil, bootstrapPack())

	out, err := v.Verify(ctx, tq3Input(steps, nil, tree))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(out.Rounds) == 0 {
		t.Fatal("no rounds recorded")
	}
	sc := stepByID(t, out.Rounds[0], "S-2")
	if sc.State != verify.ContractFail {
		t.Fatalf("S-2 contract %q, want FAIL — the step promised public/images/** and the tree holds nothing there (Spec S07.8 A16: decided from the tree, never from the executor's report)", sc.State)
	}
	if !strings.HasPrefix(sc.AttributedTo, "tree:") {
		t.Fatalf("S-2 attributed to %q, want a tree:<pattern> attribution naming what refuted it", sc.AttributedTo)
	}
	if strings.TrimSpace(sc.Detail) == "" {
		t.Fatal("S-2 FAIL carries no plain-words detail")
	}
	if sc.Category != verify.CatACBlocker || sc.Route != verify.RouteTable[verify.CatACBlocker].Sink {
		t.Fatalf("S-2 category/route = %q/%q, want AC-BLOCKER/%q (Spec S07.3: a contract declares its category and route)", sc.Category, sc.Route, verify.RouteTable[verify.CatACBlocker].Sink)
	}
	if s1 := stepByID(t, out.Rounds[0], "S-1"); s1.State == verify.ContractFail {
		t.Fatalf("S-1 (write set satisfied) is FAIL: %+v", s1)
	}

	// The decided contract is recorded on the verify.v1 row (Spec S07.11).
	rows := f.events(verify.EventV1)
	if len(rows) == 0 {
		t.Fatal("no verify.v1 row")
	}
	var rec verify.V1Result
	if err := json.Unmarshal(rows[len(rows)-1].Payload, &rec); err != nil {
		t.Fatalf("decode verify.v1: %v", err)
	}
	found := false
	for _, s := range rec.Steps {
		if s.StepID == "S-2" && s.State == verify.ContractFail && s.Detail != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("verify.v1 row does not carry S-2 FAIL with its detail: %s", string(rows[len(rows)-1].Payload))
	}
}

// TestTQ3NamedPathInDoneWhenRefutedIsFAIL [R3]: the TQ-F4 shape exactly — the
// write set is satisfied elsewhere, but the Done-when line names
// `public/images/**` and the tree has nothing under it.
func TestTQ3NamedPathInDoneWhenRefutedIsFAIL(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	tree := t.TempDir()
	writeTree(t, tree, map[string]string{
		"src/app.ts":             "export {}\n",
		"src/data/products.json": "[{\"id\":1,\"image\":\"https://picsum.photos/seed/1/400\"}]",
	})
	v := f.verifier(&fakeJudge{}, nil, bootstrapPack())

	out, err := v.Verify(ctx, tq3Input(webshopSteps(), nil, tree))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	sc := stepByID(t, out.Rounds[0], "S-2")
	if sc.State != verify.ContractFail {
		t.Fatalf("S-2 contract %q, want FAIL — the line names `public/images/**` and the tree holds no file there", sc.State)
	}
	if sc.AttributedTo != "tree:public/images/**" {
		t.Fatalf("S-2 attributed to %q, want %q", sc.AttributedTo, "tree:public/images/**")
	}
}

// TestTQ3TreeSatisfyingContractIsPASSWithFacts [R5]: when every deterministic
// class holds, the contract is PASS with the facts named; the round stays the
// advisory SHIP-with-notes, no card.
func TestTQ3TreeSatisfyingContractIsPASSWithFacts(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	tree := t.TempDir()
	writeTree(t, tree, map[string]string{
		"src/app.ts":             "export {}\n",
		"src/data/products.json": "[{\"id\":1,\"image\":\"/images/a.jpg\"}]",
		"public/images/a.jpg":    "\xff\xd8\xff\xe0 not really a jpeg but bytes",
	})
	v := f.verifier(&fakeJudge{}, nil, bootstrapPack())

	out, err := v.Verify(ctx, tq3Input(webshopSteps(), nil, tree))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	sc := stepByID(t, out.Rounds[0], "S-2")
	if sc.State != verify.ContractPass {
		t.Fatalf("S-2 contract %q, want PASS — the tree satisfies the write set and every named path (detail %q)", sc.State, sc.Detail)
	}
	if sc.AttributedTo != "" {
		t.Fatalf("PASS attributed to %q, want no attribution", sc.AttributedTo)
	}
	if !strings.Contains(sc.Detail, "public/images/**") {
		t.Fatalf("PASS detail %q does not name the fact decided", sc.Detail)
	}
	if out.Verdict != verify.VerdictShipWithNotes {
		t.Fatalf("verdict %s, want SHIP-with-notes (advisory bootstrap round)", out.Verdict)
	}
	if out.Card != nil {
		t.Fatalf("a satisfied contract raised a card: %+v", out.Card)
	}
}

// TestTQ3ProseOnlyContractStaysUnverifiableWithReason [R4]: a contract no
// tree fact can decide stays UNVERIFIABLE-HERE attributed to the absent pack,
// and now says why in plain words.
func TestTQ3ProseOnlyContractStaysUnverifiableWithReason(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	tree := t.TempDir()
	writeTree(t, tree, map[string]string{"src/app.ts": "export {}\n"})
	v := f.verifier(&fakeJudge{}, nil, bootstrapPack())

	out, err := v.Verify(ctx, tq3Input(webshopSteps(), nil, tree))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	sc := stepByID(t, out.Rounds[0], "S-3")
	if sc.State != verify.ContractUnverifiable {
		t.Fatalf("S-3 contract %q, want UNVERIFIABLE-HERE — \"believable\" is not a tree fact", sc.State)
	}
	if sc.AttributedTo != verify.BootstrapAttribution {
		t.Fatalf("S-3 attributed to %q, want %q", sc.AttributedTo, verify.BootstrapAttribution)
	}
	if strings.TrimSpace(sc.Detail) == "" {
		t.Fatal("S-3 records no reason for being undecidable here — recorded, not silently passed (Spec S07.3 definition-of-cannot-be-done-here)")
	}
}

// TestTQ3RemovalWordingIsNeverDecidedByPresence [R3 removal guard]: a line
// that speaks of removing a path cannot be decided by presence; it is never
// FAIL, and the reason names the removal wording.
func TestTQ3RemovalWordingIsNeverDecidedByPresence(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	tree := t.TempDir()
	writeTree(t, tree, map[string]string{"src/app.ts": "export {}\n"})
	steps := []intake.Step{
		{ID: "S-4", Title: "cleanup", DoneWhen: "the old `legacy/` folder is removed and nothing imports it", Class: "C2"},
	}
	v := f.verifier(&fakeJudge{}, nil, bootstrapPack())

	out, err := v.Verify(ctx, tq3Input(steps, nil, tree))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	sc := stepByID(t, out.Rounds[0], "S-4")
	if sc.State == verify.ContractFail {
		t.Fatalf("a removal contract was FAILed because the path is absent — absence is the SUCCESS condition here: %+v", sc)
	}
	if !strings.Contains(strings.ToLower(sc.Detail), "remov") {
		t.Fatalf("S-4 detail %q does not say the line speaks of removal", sc.Detail)
	}
}

// TestTQ3FailContractReachesTheRequesterAsACBlocker [R8, R10]: a FAIL on a
// step that owns AC-1 is a blocker citing AC-1, anchored to the step, in
// requester words; it forces REVISE and, with no executor seam to retry,
// lands the CAP-HIT card carrying the finding and the bootstrap posture.
func TestTQ3FailContractReachesTheRequesterAsACBlocker(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	tree := t.TempDir()
	writeTree(t, tree, map[string]string{"src/app.ts": "export {}\n", "src/data/products.json": "[]"})
	v := f.verifier(&fakeJudge{}, nil, bootstrapPack())

	out, err := v.Verify(ctx, tq3Input(webshopSteps(), map[string][]string{"AC-1": {"S-2"}, "AC-2": {"S-1"}}, tree))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Card == nil {
		t.Fatal("a FAIL contract raised no card — the AC-BLOCKER route ends in a decision card (Spec S07.7)")
	}
	if out.Card.Category != verify.CatCapHit {
		t.Fatalf("card category %q, want CAP-HIT (REVISE with no retry seam)", out.Card.Category)
	}
	if out.Card.Infrastructure {
		t.Fatalf("the drain parked on the infrastructure card: %+v", out.Card)
	}
	fd, ok := findingAt(out.Card.Findings, "step:S-2")
	if !ok {
		t.Fatalf("card carries no finding anchored to step:S-2: %+v", out.Card.Findings)
	}
	if fd.Severity != verify.SeverityBlocker || fd.Demoted {
		t.Fatalf("finding is %q (demoted=%v), want an undemoted blocker", fd.Severity, fd.Demoted)
	}
	if fd.Category != verify.CatACBlocker || fd.Criterion != "AC-1" {
		t.Fatalf("finding category/criterion = %q/%q, want AC-BLOCKER citing AC-1 (the criterion this step owns)", fd.Category, fd.Criterion)
	}
	for _, tok := range []string{"S07", "Spec", "§", "AC-BLOCKER", "check-pack:", "tree:"} {
		if strings.Contains(fd.Text, tok) {
			t.Fatalf("requester-facing finding text carries the internal token %q: %q", tok, fd.Text)
		}
	}
	if !strings.Contains(fd.Text, "S-2") || !strings.Contains(fd.Text, "public/images") {
		t.Fatalf("finding text %q does not name the step and what the files failed to show", fd.Text)
	}
	if detail := strings.Join(out.Card.Detail, "\n"); !strings.Contains(detail, "restores the full ladder") {
		t.Fatalf("card detail %q does not name the bootstrap posture", detail)
	}
	if len(out.VerifiedItems) != 0 {
		t.Fatalf("a FAIL round verified %v", out.VerifiedItems)
	}
	if out.Rounds[0].Verdict != verify.VerdictRevise {
		t.Fatalf("round verdict %s, want REVISE — a blocker forces the rework route", out.Rounds[0].Verdict)
	}
}

// TestTQ3FailOnUncoveredStepIsANoteNotAGoalpost [R8]: a FAIL on a step that
// no frozen criterion covers cites nothing, so it can only be a note (Spec
// S07.5) — recorded, demoted, delivered as a review comment, never a rework
// round.
func TestTQ3FailOnUncoveredStepIsANoteNotAGoalpost(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	tree := t.TempDir()
	writeTree(t, tree, map[string]string{"src/app.ts": "export {}\n", "src/data/products.json": "[]"})
	v := f.verifier(&fakeJudge{}, nil, bootstrapPack())

	out, err := v.Verify(ctx, tq3Input(webshopSteps(), map[string][]string{"AC-1": {"S-1"}, "AC-2": {"S-1"}}, tree))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	fd, ok := findingAt(out.Rounds[0].Findings, "step:S-2")
	if !ok {
		t.Fatalf("no finding anchored to step:S-2 in the round: %+v", out.Rounds[0].Findings)
	}
	if fd.Severity != verify.SeverityNote || !fd.Demoted {
		t.Fatalf("finding is %q (demoted=%v), want a demoted note — it cites no frozen criterion", fd.Severity, fd.Demoted)
	}
	if out.Verdict != verify.VerdictShipWithNotes {
		t.Fatalf("verdict %s, want SHIP-with-notes — a note never spins a round", out.Verdict)
	}
	if out.Card != nil {
		t.Fatalf("a note raised a card: %+v", out.Card)
	}
}

// TestTQ3JudgeConsumesContractsAsEvidenceAndCannotFlipThem [R11]: the judge
// sees the decided contract in its V1 slice and a pass-all judge cannot undo
// it — the mechanical fact stands (Spec S07.5).
func TestTQ3JudgeConsumesContractsAsEvidenceAndCannotFlipThem(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	tree := t.TempDir()
	writeTree(t, tree, map[string]string{"src/app.ts": "export {}\n", "src/data/products.json": "[]"})
	j := &fakeJudge{} // passes every AC with the artifact as evidence
	v := f.verifier(j, nil, bootstrapPack())

	out, err := v.Verify(ctx, tq3Input(webshopSteps(), map[string][]string{"AC-1": {"S-2"}}, tree))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(j.inputs) == 0 || j.inputs[0].V1 == nil {
		t.Fatal("the judge received no V1 outcomes — the contracts must ride the S07.5 input slice as evidence")
	}
	seen := false
	for _, sc := range j.inputs[0].V1.Steps {
		if sc.StepID == "S-2" && sc.State == verify.ContractFail {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("judge input V1 steps do not carry S-2 FAIL: %+v", j.inputs[0].V1.Steps)
	}
	if sc := stepByID(t, out.Rounds[0], "S-2"); sc.State != verify.ContractFail {
		t.Fatalf("after a pass-all judge the recorded contract is %q — the judge re-decided a mechanical fact", sc.State)
	}
	if out.Verdict == verify.VerdictShip || out.Verdict == verify.VerdictShipWithNotes {
		t.Fatalf("verdict %s over a FAIL contract — the judge overrode V1", out.Verdict)
	}
}

// TestTQ3PropContractDecisionIsThePredicateOverTheTree [R2 property]: for
// generated plans whose steps declare d<k>/** and generated trees, the
// recorded state equals the predicate "no file under d<k>/ ⇒ FAIL, else
// PASS", whatever the deliverable text claims; a tree satisfying every glob
// never yields FAIL.
func TestTQ3PropContractDecisionIsThePredicateOverTheTree(t *testing.T) {
	ctx := context.Background()
	for seed := int64(1); seed <= 3; seed++ {
		r := rand.New(rand.NewSource(seed))
		for n := 1; n <= 4; n++ {
			f := newFix(t)
			f.seedTask("t1", "r1")
			tree := t.TempDir()
			var steps []intake.Step
			expect := map[string]verify.ContractState{}
			files := map[string]string{}
			for k := 1; k <= n; k++ {
				id := fmt.Sprintf("S-%d", k)
				steps = append(steps, intake.Step{
					ID: id, Title: "step", Class: "C2",
					DoneWhen: fmt.Sprintf("condition %d holds", r.Intn(1000)),
					WriteSet: []string{fmt.Sprintf("d%d/**", k)},
				})
				count := r.Intn(3)
				for j := 0; j < count; j++ {
					files[fmt.Sprintf("d%d/f%d.txt", k, j)] = "x"
				}
				if count == 0 {
					expect[id] = verify.ContractFail
				} else {
					expect[id] = verify.ContractPass
				}
			}
			writeTree(t, tree, files)
			in := tq3Input(steps, nil, tree)
			if seed%2 == 0 {
				in.Deliverable.Content = "Nothing was written; the run failed early.\n"
			}
			v := f.verifier(&fakeJudge{}, nil, bootstrapPack())
			out, err := v.Verify(ctx, in)
			if err != nil {
				t.Fatalf("seed %d n=%d: Verify: %v", seed, n, err)
			}
			allSatisfied := true
			for id, want := range expect {
				got := stepByID(t, out.Rounds[0], id)
				if got.State != want {
					t.Fatalf("seed %d n=%d: %s state %q, want %q (files=%v) — the decision is the predicate over the tree, never the report", seed, n, id, got.State, want, files)
				}
				if want == verify.ContractFail {
					allSatisfied = false
				}
			}
			if allSatisfied {
				for _, sc := range out.Rounds[0].V1.Steps {
					if sc.State == verify.ContractFail {
						t.Fatalf("seed %d n=%d: a tree satisfying every glob yielded FAIL on %s", seed, n, sc.StepID)
					}
				}
			}
		}
	}
}

// TestTQ3UnreadableWorkspaceIsUnverifiableNeverPass [R6]: a tree the platform
// cannot read decides nothing — UNVERIFIABLE-HERE attributed to the
// workspace, no fabricated PASS or FAIL, no blocker, no card.
func TestTQ3UnreadableWorkspaceIsUnverifiableNeverPass(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	steps := []intake.Step{
		{ID: "S-2", Title: "images", DoneWhen: "photos are in place", Class: "C2", WriteSet: []string{"public/images/**"}},
	}
	v := f.verifier(&fakeJudge{}, nil, bootstrapPack())

	out, err := v.Verify(ctx, tq3Input(steps, map[string][]string{"AC-1": {"S-2"}}, filepath.Join(t.TempDir(), "missing")))
	if err != nil {
		t.Fatalf("Verify: %v — an unreadable tree is recorded, never a crash", err)
	}
	sc := stepByID(t, out.Rounds[0], "S-2")
	if sc.State != verify.ContractUnverifiable {
		t.Fatalf("S-2 contract %q, want UNVERIFIABLE-HERE", sc.State)
	}
	if sc.AttributedTo != "workspace:unreadable" {
		t.Fatalf("S-2 attributed to %q, want %q", sc.AttributedTo, "workspace:unreadable")
	}
	for _, fd := range out.Rounds[0].Findings {
		if fd.Severity == verify.SeverityBlocker {
			t.Fatalf("an unreadable tree minted a blocker: %+v", fd)
		}
	}
	if out.Card != nil {
		t.Fatalf("an unreadable tree raised a card: %+v", out.Card)
	}
}

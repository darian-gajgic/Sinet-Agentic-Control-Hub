package verify_test

// tq4_evidence_test.go — P3-TQ-4a acceptance battery, verify half: at a
// bootstrap-posture round the platform RUNS the commands it detected in the
// produced tree, as EVIDENCE rungs, inside the network-off verification
// sandbox (Spec S07.8 bootstrap bullet [A16, 2026-09-17]; Spec S07.3 ladder
// rules 1/3/4; Spec S07.5 evidence-not-verdict).
//
// The live defect these bind (TQ-F3): a task that created a project's whole
// build system was verified by two judge reads and nothing else — the
// executor's scaffold declared `npm test` and `vite build` and nobody ran
// either, because the registry capture could only be filled by hand.
//
// Committed RED at grounding (CONVENTIONS §3 amendment-A carve-out): today
// bootstrapV1 records ladder:<stage> UNVERIFIABLE-HERE for all four rungs and
// never consults a pack's Checks, so a bootstrap pack carrying detected
// commands executes nothing.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// detectedPack is the A16 resolution for a project with no hand-captured
// command whose produced tree declared build/test/lint: the bootstrap posture,
// carrying the detected commands as rungs.
func detectedPack(checks ...verify.Check) *verify.CheckPack {
	p := verify.BootstrapPack(verify.DomainSoftware, 3)
	p.Provenance = verify.ProvenanceDetected
	p.Checks = checks
	return p
}

func detectedCheck(id string, stage verify.LadderStage, argv ...string) verify.Check {
	// CatACBlocker mirrors the landed packChecks: a failing project check says
	// the work does not meet the bar the project itself set. The category is
	// the ROUTE declaration (Spec S07.3 "definition-of-done includes
	// definition-of-cannot-be-done-here"); it is not what makes a rung an
	// acceptance check — the empty ACKey is.
	return verify.Check{ID: id, Stage: stage, Argv: argv, FindingCategory: verify.CatACBlocker}
}

// outcomeFor returns the recorded outcome for one check id.
func outcomeFor(t *testing.T, r verify.RoundRecord, id string) verify.CheckOutcome {
	t.Helper()
	if r.V1 == nil {
		t.Fatal("no V1 record — the bootstrap posture RUNS V1 (Spec S07.8)")
	}
	for _, c := range r.V1.Checks {
		if c.CheckID == id {
			return c
		}
	}
	var ids []string
	for _, c := range r.V1.Checks {
		ids = append(ids, c.CheckID)
	}
	t.Fatalf("no outcome recorded for check %q; recorded: %v", id, ids)
	panic("unreachable")
}

// TestTQ4DetectedRungsRunAsEvidence [R5, R6, R8]: the detected commands are
// executed through the existing CheckRunner and their outcomes are derived
// platform-side from the exit status (Spec S07.3 rule 3), cheap-first, with
// first-upstream-failure attribution.
func TestTQ4DetectedRungsRunAsEvidence(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	runner := &scriptRunner{exits: map[string]int{"detected:test": 1}}
	pack := detectedPack(
		detectedCheck("detected:build", verify.StageStatic, "/bin/sh", "-lc", "npm run build"),
		detectedCheck("detected:test", verify.StageUnit, "/bin/sh", "-lc", "npm test"),
	)
	v := f.verifier(&fakeJudge{}, runner, pack)

	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(out.Rounds) == 0 {
		t.Fatal("no rounds recorded")
	}
	if len(runner.calls) == 0 {
		t.Fatal("the detected commands were never executed — a bootstrap round with detected rungs must RUN them (Spec S07.8 [A16]: the posture is not execution-less)")
	}
	build := outcomeFor(t, out.Rounds[0], "detected:build")
	if build.State != verify.CheckPassed {
		t.Fatalf("detected:build state %q, want PASS — exit 0 is a pass derived platform-side (Spec S07.3 rule 3)", build.State)
	}
	test := outcomeFor(t, out.Rounds[0], "detected:test")
	if test.State != verify.CheckFailed || test.ExitCode != 1 {
		t.Fatalf("detected:test state %q exit %d, want FAIL/1", test.State, test.ExitCode)
	}
	// Cheap-first: static ran before unit.
	if len(runner.calls) < 2 || runner.calls[0] != "detected:build" {
		t.Fatalf("rung order %v, want the ladder's cheap-first order (Spec S07.3)", runner.calls)
	}
	// The outcomes are recorded durably (Spec S07.11).
	rows := f.events(verify.EventV1)
	if len(rows) == 0 {
		t.Fatal("no verify.v1 row")
	}
	var rec verify.V1Result
	if err := json.Unmarshal(rows[len(rows)-1].Payload, &rec); err != nil {
		t.Fatalf("decode verify.v1: %v", err)
	}
	found := false
	for _, c := range rec.Checks {
		if c.CheckID == "detected:test" && c.State == verify.CheckFailed {
			found = true
		}
	}
	if !found {
		t.Fatalf("verify.v1 row does not carry the detected rung's outcome: %s", string(rows[len(rows)-1].Payload))
	}
}

// TestTQ4DetectedRungIsEvidenceNeverTheACVerdict [R9]: a detected rung carries
// no frozen-criterion key and no step id, so it can neither bind an AC verdict
// at axis 1 nor decide a PLAN step's contract. Spec S07.3 rule 4: executor-
// authored tests are evidence, never the acceptance verdict.
func TestTQ4DetectedRungIsEvidenceNeverTheACVerdict(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	runner := &scriptRunner{}
	pack := detectedPack(detectedCheck("detected:test", verify.StageUnit, "/bin/sh", "-lc", "npm test"))
	v := f.verifier(&fakeJudge{}, runner, pack)

	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	r := out.Rounds[0]
	if r.V1 == nil {
		t.Fatal("no V1 record")
	}
	// The rung must EXIST before "it carries no AC key" says anything: a
	// posture that records nothing satisfies every claim about what it records.
	if got := outcomeFor(t, r, "detected:test"); got.State == "" {
		t.Fatal("the detected rung has no state")
	}
	for _, c := range r.V1.Checks {
		if strings.HasPrefix(c.CheckID, "detected:") && c.ACKey != "" {
			t.Fatalf("detected rung %q carries AC key %q — a detected command is evidence, never an acceptance verdict (Spec S07.3 rule 4)", c.CheckID, c.ACKey)
		}
		if strings.HasPrefix(c.CheckID, "detected:") && c.StepID != "" {
			t.Fatalf("detected rung %q carries step id %q — it decides no PLAN contract; the tree does (P3-TQ-3)", c.CheckID, c.StepID)
		}
	}
	if got := r.V1.ACOutcomes(); len(got) != 0 {
		t.Fatalf("ACOutcomes = %v, want empty — no detected rung may reach ValidateAxis1 as a mechanical AC fact (Spec S07.5)", got)
	}
}

// TestTQ4UnrunnableDetectedRungIsUnverifiableNeverPass [R7]: a detected command
// whose tool the verification workspace cannot supply — the project's
// dependencies were never installed into the tree under review and the sandbox
// has no network (Spec S07.3 rule 1) — is recorded UNVERIFIABLE-HERE with its
// reason. It is never executed, never PASS, and never a FAIL that would blame
// the work for a platform condition.
func TestTQ4UnrunnableDetectedRungIsUnverifiableNeverPass(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	// The TQ-F3 reference tree exactly: a package.json declaring dependencies,
	// a build script whose tool comes from node_modules, a test script that
	// runs on the host node, no lint script, and no node_modules directory.
	tree := t.TempDir()
	writeTree(t, tree, map[string]string{
		"package.json": `{"name":"shop","scripts":{"build":"vite build","test":"node --test tests"},` +
			`"devDependencies":{"vite":"^5.4.8"}}`,
		"tests/catalog.test.js": "import {test} from 'node:test'\n",
	})
	runner := &scriptRunner{}
	pack := detectedPack(
		detectedCheck("detected:build", verify.StageStatic, "/bin/sh", "-lc", "npm run build"),
		detectedCheck("detected:lint", verify.StageStatic, "/bin/sh", "-lc", "npm run lint"),
		detectedCheck("detected:test", verify.StageUnit, "/bin/sh", "-lc", "npm test"),
	)
	v := f.verifier(&fakeJudge{}, runner, pack)
	in := input(deliverable("t1", "r1"))
	in.Workspace = tree

	out, err := v.Verify(ctx, in)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	build := outcomeFor(t, out.Rounds[0], "detected:build")
	if build.State != verify.CheckUnverifiable {
		t.Fatalf("detected:build state %q, want UNVERIFIABLE-HERE — `vite` comes from dependencies that are not in the tree and the sandbox has no network (Spec S07.3 rule 1; never an egress exception)", build.State)
	}
	if build.AttributedTo != verify.DetectedUnrunnable || strings.TrimSpace(build.Detail) == "" {
		t.Fatalf("detected:build attributed %q detail %q, want %q with a plain-words reason", build.AttributedTo, build.Detail, verify.DetectedUnrunnable)
	}
	lint := outcomeFor(t, out.Rounds[0], "detected:lint")
	if lint.State != verify.CheckUnverifiable {
		t.Fatalf("detected:lint state %q, want UNVERIFIABLE-HERE — the project declares no lint script, so running it would produce a meaningless failure", lint.State)
	}
	if got := outcomeFor(t, out.Rounds[0], "detected:test"); got.State != verify.CheckPassed {
		t.Fatalf("detected:test state %q, want PASS — `node` is on the host and the test script needs no dependency", got.State)
	}
	for _, id := range runner.calls {
		if id == "detected:build" || id == "detected:lint" {
			t.Fatalf("unrunnable rung %q was handed to the runner anyway (calls %v) — the precondition decides before execution, not from an exit code", id, runner.calls)
		}
	}
}

// TestTQ4DetectedCommandsNeverGraduateThePosture [R10]: every detected rung
// passing changes nothing about the posture — V2 stays advisory, V3 stays
// mandatory, and nothing is marked verified (Spec S07.8 [A16]: "detected
// commands do not graduate the project").
func TestTQ4DetectedCommandsNeverGraduateThePosture(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	runner := &scriptRunner{}
	pack := detectedPack(
		detectedCheck("detected:build", verify.StageStatic, "/bin/sh", "-lc", "npm run build"),
		detectedCheck("detected:test", verify.StageUnit, "/bin/sh", "-lc", "npm test"),
	)
	v := f.verifier(&fakeJudge{}, runner, pack)

	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	r := out.Rounds[0]
	// Both rungs must have RUN and PASSED before "and still nothing
	// graduated" is a claim about anything (Spec S07.8 [A16]).
	for _, id := range []string{"detected:build", "detected:test"} {
		if got := outcomeFor(t, r, id); got.State != verify.CheckPassed {
			t.Fatalf("%s state %q, want PASS — the graduation rule is only tested by a round whose detected rungs all passed", id, got.State)
		}
	}
	if r.Posture != verify.PostureBootstrap {
		t.Fatalf("posture %q, want bootstrap — detected commands are evidence, not graduation (Spec S07.8 [A16])", r.Posture)
	}
	if !r.ReviewMandatory {
		t.Fatal("review_mandatory is false — V3 blocks at every stakes tier until a hand-captured pack exists (Spec S07.8)")
	}
	if len(out.VerifiedItems) != 0 {
		t.Fatalf("VerifiedItems = %v on an advisory verdict — an advisory SHIP releases nothing (§70)", out.VerifiedItems)
	}
	if r.PostureNote == "" {
		t.Fatal("the round record lost its plain-words posture disclosure")
	}
}

// TestTQ4RungWithoutADetectedCommandKeepsItsHonestPlaceholder [R8]: a ladder
// stage no detected command covers still records UNVERIFIABLE-HERE under the
// absent-pack attribution — nothing is silently skipped, and a stage is never
// reported twice.
func TestTQ4RungWithoutADetectedCommandKeepsItsHonestPlaceholder(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	runner := &scriptRunner{}
	pack := detectedPack(detectedCheck("detected:test", verify.StageUnit, "/bin/sh", "-lc", "go test ./..."))
	v := f.verifier(&fakeJudge{}, runner, pack)
	// A NON-web tree: a Go module, no index.html.
	tree := t.TempDir()
	writeTree(t, tree, map[string]string{"go.mod": "module shop\n", "main.go": "package main\n"})
	in := input(deliverable("t1", "r1"))
	in.Workspace = tree

	out, err := v.Verify(ctx, in)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	r := out.Rounds[0]
	smoke := outcomeFor(t, r, "ladder:"+string(verify.StageSmoke))
	if smoke.State != verify.CheckUnverifiable || smoke.AttributedTo != verify.BootstrapAttribution {
		t.Fatalf("smoke rung %q/%q, want UNVERIFIABLE-HERE/%q — an uncovered rung keeps the absent-command placeholder", smoke.State, smoke.AttributedTo, verify.BootstrapAttribution)
	}
	// The walk reason's NEGATIVE half: this tree is not web-shaped, so the
	// missing thing on the e2e rung really is a command. Blaming an unadopted
	// browser for a Go CLI would be a false explanation in the other direction.
	e2e := outcomeFor(t, r, "ladder:"+string(verify.StageE2E))
	if e2e.AttributedTo != verify.BootstrapAttribution {
		t.Fatalf("e2e rung on a non-web tree attributed %q, want %q", e2e.AttributedTo, verify.BootstrapAttribution)
	}
	// The covered stage is reported ONCE, by its detected rung.
	for _, c := range r.V1.Checks {
		if c.CheckID == "ladder:"+string(verify.StageUnit) {
			t.Fatal("the unit stage records both a detected rung and the absent-command placeholder — a stage a rung covers is reported once")
		}
	}
}

// TestTQ4WalkRungIsUnverifiableWithItsRealReason [R8, R11]: the e2e rung for a
// web launch-domain deliverable is the platform-authored AC walk. No browser is
// adopted for verification walks, so the rung records UNVERIFIABLE-HERE naming
// exactly that — never PASS, and never the generic absent-command sentence,
// which would be a false explanation (Spec S07.3 [A16]; S16.4).
func TestTQ4WalkRungIsUnverifiableWithItsRealReason(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	runner := &scriptRunner{}
	pack := detectedPack(detectedCheck("detected:test", verify.StageUnit, "/bin/sh", "-lc", "npm test"))
	v := f.verifier(&fakeJudge{}, runner, pack)
	// A WEB launch-domain deliverable: the e2e rung is the platform-authored
	// walk, so its missing substrate is a browser, not a command. Web-shaped is
	// decided from the tree, never from the executor's report.
	tree := t.TempDir()
	writeTree(t, tree, map[string]string{
		"index.html":   "<!doctype html><div id=root></div>",
		"package.json": `{"name":"shop","scripts":{"dev":"vite","test":"node --test tests"}}`,
	})
	in := input(deliverable("t1", "r1"))
	in.Workspace = tree

	out, err := v.Verify(ctx, in)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	e2e := outcomeFor(t, out.Rounds[0], "ladder:"+string(verify.StageE2E))
	if e2e.State != verify.CheckUnverifiable {
		t.Fatalf("e2e rung state %q, want UNVERIFIABLE-HERE", e2e.State)
	}
	if e2e.AttributedTo != verify.WalkUnavailable {
		t.Fatalf("e2e rung attributed %q, want %q — the reason is the missing browser, not a missing command", e2e.AttributedTo, verify.WalkUnavailable)
	}
	if e2e.Detail != verify.WalkUnavailableReason {
		t.Fatalf("e2e rung detail %q, want the walk's own plain-words reason", e2e.Detail)
	}
}

// TestTQ4MissingRunnerDoesNotParkABootstrapRound [R6]: a bootstrap resolution
// carrying detected rungs with no CheckRunner wired records them
// UNVERIFIABLE-HERE. It is never the preamble refusal a hand-captured pack
// without a runner is — bootstrap never parks the run (Spec S07.8).
func TestTQ4MissingRunnerDoesNotParkABootstrapRound(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	pack := detectedPack(detectedCheck("detected:test", verify.StageUnit, "/bin/sh", "-lc", "npm test"))
	v := f.verifier(&fakeJudge{}, nil, pack)

	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify returned an error for a bootstrap round without a runner: %v — pack absence never parks (Spec S07.8)", err)
	}
	if got := outcomeFor(t, out.Rounds[0], "detected:test"); got.State != verify.CheckUnverifiable {
		t.Fatalf("detected:test state %q with no runner wired, want UNVERIFIABLE-HERE", got.State)
	}
}

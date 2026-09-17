package verify_test

// tq4c_mixed_test.go — P3-TQ-4c acceptance battery. In a GRADUATED pack that
// mixes the owner's captured rungs with the platform's detected ones (Spec
// S07.8 [A16]; CONVENTIONS §79), two rules hold:
//
//   - first-upstream-failure attribution (Spec S07.3) is computed PER ORIGIN:
//     a detected rung's failure never silences an owner rung — the owner's
//     rungs run and decide on their own exit status — while an owner failure
//     still blocks every later rung of either origin (a detected test on a
//     tree the owner's own build refused is undecidable, and stays evidence);
//   - a detected rung that FAILS is a human-visible NOTE (Spec S07.7: every
//     verification finding terminates in a human-visible sink; Spec S07.5: a
//     finding citing no criterion can only be a note; Spec S07.6 / S13.4:
//     notes travel to the review surface and never spin a round). It is never
//     a kill (§80) and never silence: the round is SHIP-with-notes, never a
//     clean SHIP, never REVISE.
//
// Committed RED at grounding (CONVENTIONS §3 amendment-A carve-out): today
// RunV1 keeps ONE failedStage/firstFailure pair for both origins
// (v1.go:477-489, :530-536), so a failed detected:build records the owner's
// test rung UNVERIFIABLE-HERE attributed to the detected rung; and the
// CheckFailed arm's origin guard (v1.go:545) mints nothing for a detected
// rung, so a round whose only failure is detected SHIPs clean with items
// verified. The tests marked GUARD are green today and must stay green.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// tq4cRunner scripts, per check id, the exit code of each successive run (the
// last entry repeats; a missing id exits 0), with an optional output tail and
// an optional runner error. It records the order checks were handed to it.
type tq4cRunner struct {
	script map[string][]int
	tails  map[string]string
	errs   map[string]error
	runs   map[string]int
	calls  []string
}

func (r *tq4cRunner) RunCheck(_ context.Context, req verify.CheckRequest) (verify.CheckResult, error) {
	id := req.Check.ID
	r.calls = append(r.calls, id)
	if r.runs == nil {
		r.runs = map[string]int{}
	}
	n := r.runs[id]
	r.runs[id]++
	if err := r.errs[id]; err != nil {
		return verify.CheckResult{}, err
	}
	code := 0
	if s := r.script[id]; len(s) > 0 {
		if n >= len(s) {
			n = len(s) - 1
		}
		code = s[n]
	}
	return verify.CheckResult{ExitCode: code, EvidenceRef: "evidence/" + id + ".log", OutputTail: r.tails[id]}, nil
}

func (r *tq4cRunner) ran(id string) bool {
	for _, c := range r.calls {
		if c == id {
			return true
		}
	}
	return false
}

// tq4cMixedPack is the pack packChecksFor (internal/shell/project_seams.go)
// composes for an owner who captured lint by hand while the platform detected
// build and test in the produced tree: graduated (no posture), no pack-level
// provenance, ids and Check.Origin exactly as production mints them.
func tq4cMixedPack() *verify.CheckPack {
	return &verify.CheckPack{
		Domain: verify.DomainSoftware, Version: 5, VerifiedOn: time.Now().Add(-time.Hour),
		Checks: []verify.Check{
			{ID: "lint", Stage: verify.StageStatic, Argv: []string{"/bin/sh", "-lc", "eslint ."}, FindingCategory: verify.CatACBlocker},
			{ID: "detected:build", Stage: verify.StageStatic, Argv: []string{"/bin/sh", "-lc", "npm run build"}, Origin: verify.ProvenanceDetected, FindingCategory: verify.CatACBlocker},
			{ID: "detected:test", Stage: verify.StageUnit, Argv: []string{"/bin/sh", "-lc", "npm test"}, Origin: verify.ProvenanceDetected, FindingCategory: verify.CatACBlocker},
		},
	}
}

// tq4cFindingsAt returns every finding anchored at anchor.
func tq4cFindingsAt(fs []verify.Finding, anchor string) []verify.Finding {
	var out []verify.Finding
	for _, f := range fs {
		if f.Anchor == anchor {
			out = append(out, f)
		}
	}
	return out
}

// tq4cCheckBlockers returns the undemoted BLOCKER-severity findings anchored
// check:* — the kills, whatever their category.
func tq4cCheckBlockers(fs []verify.Finding) []verify.Finding {
	var out []verify.Finding
	for _, f := range fs {
		if strings.HasPrefix(f.Anchor, "check:") && f.Severity == verify.SeverityBlocker && !f.Demoted {
			out = append(out, f)
		}
	}
	return out
}

func tq4cOutcome(t *testing.T, res verify.V1Result, id string) verify.CheckOutcome {
	t.Helper()
	for _, c := range res.Checks {
		if c.CheckID == id {
			return c
		}
	}
	t.Fatalf("no outcome recorded for %q: %+v", id, res.Checks)
	panic("unreachable")
}

func tq4cContract(t *testing.T, res verify.V1Result, stepID string) verify.StepContract {
	t.Helper()
	for _, sc := range res.Steps {
		if sc.StepID == stepID {
			return sc
		}
	}
	t.Fatalf("no contract recorded for step %s: %+v", stepID, res.Steps)
	panic("unreachable")
}

// assertDetectedNote asserts the ONE note a failed detected rung mints (brief
// R3/R4): note severity, born a note (never demoted), the check's DECLARED
// category, no criterion (Spec S07.5), anchored check:<id>, round-stable key,
// plain words naming the quoted id, the rung, the exit status and the capture
// door, carrying the end of the output, and never a platform path.
func assertDetectedNote(t *testing.T, fs []verify.Finding, id string, stage verify.LadderStage, exit int, cat verify.Category, tailLine string) verify.Finding {
	t.Helper()
	anchor := "check:" + id
	at := tq4cFindingsAt(fs, anchor)
	if len(at) != 1 {
		t.Fatalf("findings anchored %s = %d, want exactly one note — a failed detected rung is visible, once (Spec S07.7): %+v", anchor, len(at), fs)
	}
	fd := at[0]
	if fd.Severity != verify.SeverityNote || fd.Demoted {
		t.Fatalf("finding at %s is %q (demoted=%v), want a note born a note — evidence is never a kill (§79/§80): %+v", anchor, fd.Severity, fd.Demoted, fd)
	}
	if fd.Category != cat {
		t.Fatalf("note category %q, want the check's DECLARED %q (Spec S07.3)", fd.Category, cat)
	}
	if fd.Criterion != "" {
		t.Fatalf("note cites %q, want no criterion — a finding citing none can only be a note (Spec S07.5), and the check citation is the owner kill's alone", fd.Criterion)
	}
	if got, want := fd.Key(), verify.FindingKey("|"+anchor+"|"+string(cat)); got != want {
		t.Fatalf("note key %q, want %q (round-stable: no exit status, no output)", got, want)
	}
	for _, w := range []string{fmt.Sprintf("%q", id), tq7StageWords[stage], fmt.Sprintf("status %d", exit)} {
		if !strings.Contains(fd.Text, w) {
			t.Fatalf("note text %q does not say %q", fd.Text, w)
		}
	}
	if !strings.Contains(strings.ToLower(fd.Text), "captur") {
		t.Fatalf("note text %q does not name the capture door — the requester must learn that capturing the command makes it an authoritative check", fd.Text)
	}
	if tailLine != "" && !strings.Contains(fd.Text, tailLine) {
		t.Fatalf("note text %q does not carry the end of what the command printed (%q)", fd.Text, tailLine)
	}
	if strings.Contains(fd.Text, "evidence/") {
		t.Fatalf("note text %q carries a platform path; the log stays a ref on the outcome row", fd.Text)
	}
	assertPlainWords(t, fd.Text)
	return fd
}

// TestTQ4cDetectedFailureInAGraduatedPackIsAVisibleNote [R3, R4, R7, R9]: the
// live N1 instance. The owner's lint passes, the platform's detected build
// fails, a content judge marks every criterion met. The round mints exactly one
// NOTE anchored check:detected:build, no blocker, the verdict is SHIP-with-notes
// (never clean SHIP, never REVISE), the work items are verified (the owner's
// own bar passed and the graduated verdict is authoritative), the note reaches
// the review surface, and both the verify.v1 row and the verdict row carry it.
func TestTQ4cDetectedFailureInAGraduatedPackIsAVisibleNote(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	tail := "src/app.ts(3,1): error TS2304: Cannot find name 'greet'.\nnpm ERR! code 2\n"
	runner := &tq4cRunner{script: map[string][]int{"detected:build": {2}}, tails: map[string]string{"detected:build": tail}}
	sink := &fakeSink{}
	j := &fakeJudge{}
	v := f.verifier(j, runner, tq4cMixedPack())
	v.Review = sink

	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(out.Rounds) != 1 {
		t.Fatalf("rounds %d, want 1", len(out.Rounds))
	}
	rd := out.Rounds[0]
	fd := assertDetectedNote(t, rd.Findings, "detected:build", verify.StageStatic, 2, verify.CatACBlocker, "npm ERR! code 2")
	if fd.N == 0 {
		t.Fatalf("the note is not numbered: %+v", fd)
	}
	if b := tq4cCheckBlockers(rd.Findings); len(b) != 0 {
		t.Fatalf("a detected failure minted a kill: %+v", b)
	}
	if _, ok := findingAt(rd.Findings, "check:lint"); ok {
		t.Fatalf("the passing owner lint minted a finding: %+v", rd.Findings)
	}
	if rd.Verdict != verify.VerdictShipWithNotes {
		t.Fatalf("round verdict %s, want SHIP-with-notes — the failure is a note the requester sees, not silence and not a kill", rd.Verdict)
	}
	if out.Verdict != verify.VerdictShipWithNotes || out.Card != nil {
		t.Fatalf("terminal %s / card %+v, want SHIP-with-notes and no card", out.Verdict, out.Card)
	}
	if len(out.IntegrityCards) != 0 {
		t.Fatalf("a detected failure is not a suite defect, yet an integrity card was raised: %+v", out.IntegrityCards)
	}
	if len(out.VerifiedItems) != 1 || out.VerifiedItems[0] != "w1" {
		t.Fatalf("verified items %v, want [w1] — the owner's own bar passed and the graduated verdict is authoritative", out.VerifiedItems)
	}
	if got := tq7ItemStatus(t, f, "t1", "w1"); got != ledger.StatusVerified {
		t.Fatalf("work item w1 is %q, want verified", got)
	}
	if j.complianceCalls != 1 {
		t.Fatalf("compliance calls %d, want 1", j.complianceCalls)
	}
	// The detected test rung sits downstream of the detected failure: undecided,
	// attributed to the detected build — the detected lane's own attribution.
	test := outcomeFor(t, rd, "detected:test")
	if test.State != verify.CheckUnverifiable || test.AttributedTo != "detected:build" {
		t.Fatalf("detected:test outcome %+v, want UNVERIFIABLE-HERE attributed to detected:build", test)
	}
	if _, ok := findingAt(rd.Findings, "check:detected:test"); ok {
		t.Fatalf("a blocked detected rung minted a finding: %+v", rd.Findings)
	}
	// The review surface (Spec S07.6 notes ride as review comments; S13.4).
	reached := false
	for _, of := range sink.openFindings {
		if of.Anchor == "check:detected:build" && of.Severity == verify.SeverityNote {
			reached = true
		}
	}
	if !reached {
		t.Fatalf("the note never reached the review surface (Review.RecordFindings): %+v", sink.openFindings)
	}
	// Recording (Spec S07.11): the verify.v1 row and the verdict row.
	v1rows := f.events(verify.EventV1)
	if len(v1rows) == 0 {
		t.Fatal("no verify.v1 row")
	}
	var rec verify.V1Result
	if err := json.Unmarshal(v1rows[len(v1rows)-1].Payload, &rec); err != nil {
		t.Fatalf("decode verify.v1: %v", err)
	}
	if _, ok := findingAt(rec.Findings, "check:detected:build"); !ok {
		t.Fatalf("verify.v1 row does not carry the note: %s", string(v1rows[len(v1rows)-1].Payload))
	}
	roundRows := f.events(verify.EventRound)
	if len(roundRows) == 0 {
		t.Fatal("no verdict row")
	}
	var round struct {
		Findings []verify.Finding `json:"findings"`
	}
	if err := json.Unmarshal(roundRows[len(roundRows)-1].Payload, &round); err != nil {
		t.Fatalf("decode verdict row: %v", err)
	}
	if _, ok := findingAt(round.Findings, "check:detected:build"); !ok {
		t.Fatalf("verdict row does not carry the note: %s", string(roundRows[len(roundRows)-1].Payload))
	}
}

// TestTQ4cNoteCarriesTheChecksDeclaredCategory [R3]: the note's category is the
// one the CHECK declared (Spec S07.3), never a literal at the mint site.
// Production declares AC-BLOCKER; a detected rung declaring SANITY-BLOCKER
// notes under SANITY-BLOCKER.
func TestTQ4cNoteCarriesTheChecksDeclaredCategory(t *testing.T) {
	ctx := context.Background()
	pack := &verify.CheckPack{
		Domain: verify.DomainSoftware, Version: 2, VerifiedOn: time.Now().Add(-time.Hour),
		Checks: []verify.Check{
			{ID: "lint", Stage: verify.StageStatic, Argv: []string{"lint"}, FindingCategory: verify.CatACBlocker},
			{ID: "detected:smoke", Stage: verify.StageSmoke, Argv: []string{"smoke"}, Origin: verify.ProvenanceDetected, FindingCategory: verify.CatSanityBlocker},
		},
	}
	res, err := verify.RunV1(ctx, pack, &tq4cRunner{script: map[string][]int{"detected:smoke": {1}}}, v1req(), nil, nil, time.Now(), regSettings(t))
	if err != nil {
		t.Fatalf("RunV1: %v", err)
	}
	assertDetectedNote(t, res.Findings, "detected:smoke", verify.StageSmoke, 1, verify.CatSanityBlocker, "")
	if !strings.Contains(assertDetectedNote(t, res.Findings, "detected:smoke", verify.StageSmoke, 1, verify.CatSanityBlocker, "").Text, "printed nothing") {
		t.Fatalf("a note with no output must say so: %+v", res.Findings)
	}
}

// TestTQ4cDetectedFailureNeverBlocksAnOwnerRung [R1, R2]: the crossing. The
// owner captured lint AND test; the platform detected build. When the detected
// build fails, the owner's test rung RUNS and decides on its own exit status,
// and the PLAN step it binds is decided from that — never attributed to the
// platform's guess.
func TestTQ4cDetectedFailureNeverBlocksAnOwnerRung(t *testing.T) {
	ctx := context.Background()
	s := regSettings(t)
	pack := func() *verify.CheckPack {
		p := tq4cMixedPack()
		p.Checks[2] = verify.Check{ID: "test", Stage: verify.StageUnit, Argv: []string{"/bin/sh", "-lc", "npm test"}, StepID: "S-1", FindingCategory: verify.CatACBlocker}
		return p
	}
	coverage := map[string][]string{"AC-1": {"S-1"}}
	steps := ladderSteps()[:1]

	t.Run("the owner rung runs and passes on its own", func(t *testing.T) {
		r := &tq4cRunner{script: map[string][]int{"detected:build": {1}}}
		res, err := verify.RunV1(ctx, pack(), r, v1req(), steps, coverage, time.Now(), s)
		if err != nil {
			t.Fatalf("RunV1: %v", err)
		}
		if !r.ran("test") {
			t.Fatalf("the owner's test rung was never run — a detected failure silenced it (calls %v)", r.calls)
		}
		test := tq4cOutcome(t, res, "test")
		if test.State != verify.CheckPassed || test.AttributedTo != "" {
			t.Fatalf("owner test outcome %+v, want PASS with no attribution — attribution never crosses origins (Spec S07.3; §79)", test)
		}
		sc := tq4cContract(t, res, "S-1")
		if sc.State != verify.ContractPass || sc.AttributedTo != "" {
			t.Fatalf("contract S-1 %+v, want PASS decided by the owner's own test rung", sc)
		}
		assertDetectedNote(t, res.Findings, "detected:build", verify.StageStatic, 1, verify.CatACBlocker, "")
		if b := tq4cCheckBlockers(res.Findings); len(b) != 0 {
			t.Fatalf("blockers %+v, want none", b)
		}
	})

	t.Run("the owner rung runs and fails on its own", func(t *testing.T) {
		r := &tq4cRunner{script: map[string][]int{"detected:build": {1}, "test": {3}}}
		res, err := verify.RunV1(ctx, pack(), r, v1req(), steps, coverage, time.Now(), s)
		if err != nil {
			t.Fatalf("RunV1: %v", err)
		}
		test := tq4cOutcome(t, res, "test")
		if test.State != verify.CheckFailed || test.ExitCode != 3 || test.AttributedTo != "" {
			t.Fatalf("owner test outcome %+v, want FAIL exit 3 on its own", test)
		}
		kill, ok := findingAt(res.Findings, "check:test")
		if !ok || kill.Severity != verify.SeverityBlocker || kill.Criterion != "check:test" || kill.Demoted {
			t.Fatalf("the owner's failed test minted %+v, want the undemoted blocker citing check:test (§80)", kill)
		}
		sc := tq4cContract(t, res, "S-1")
		if sc.State != verify.ContractFail || sc.AttributedTo != "test" {
			t.Fatalf("contract S-1 %+v, want FAIL attributed to the owner's test", sc)
		}
		if _, ok := findingAt(res.Findings, "step:S-1"); !ok {
			t.Fatalf("the refuted contract minted no finding: %+v", res.Findings)
		}
		assertDetectedNote(t, res.Findings, "detected:build", verify.StageStatic, 1, verify.CatACBlocker, "")
		if b := tq4cCheckBlockers(res.Findings); len(b) != 1 || b[0].Anchor != "check:test" {
			t.Fatalf("check blockers %+v, want exactly the owner's check:test", b)
		}
	})

	t.Run("a quarantined owner rung's contract is never attributed to a detected failure", func(t *testing.T) {
		p := pack()
		if err := p.QuarantineCheck(verify.Quarantine{CheckID: "test", Owner: "op", Reason: "flaky", FixBy: time.Now().Add(48 * time.Hour)}); err != nil {
			t.Fatalf("quarantine: %v", err)
		}
		r := &tq4cRunner{script: map[string][]int{"detected:build": {1}}}
		res, err := verify.RunV1(ctx, p, r, v1req(), steps, coverage, time.Now(), s)
		if err != nil {
			t.Fatalf("RunV1: %v", err)
		}
		if got := tq4cOutcome(t, res, "test"); got.State != verify.CheckQuarantined {
			t.Fatalf("test outcome %+v, want QUARANTINED", got)
		}
		sc := tq4cContract(t, res, "S-1")
		if sc.State != verify.ContractUnverifiable {
			t.Fatalf("contract S-1 %+v, want UNVERIFIABLE-HERE (its check is quarantined)", sc)
		}
		if strings.HasPrefix(sc.AttributedTo, "detected:") {
			t.Fatalf("contract S-1 attributed to %q — a platform guess may never be what silences an owner's step", sc.AttributedTo)
		}
	})
}

// TestTQ4cOwnerFailureBlocksLaterRungsOfBothOrigins [R1 — GUARD, green today]:
// the spec-honest other direction. An owner failure at the static rung makes
// every later rung undecidable — the platform's detected test and the owner's
// smoke alike — and each is attributed to the owner's failure. A blocked
// detected rung mints nothing: only a FAIL is a note.
func TestTQ4cOwnerFailureBlocksLaterRungsOfBothOrigins(t *testing.T) {
	ctx := context.Background()
	pack := &verify.CheckPack{
		Domain: verify.DomainSoftware, Version: 2, VerifiedOn: time.Now().Add(-time.Hour),
		Checks: []verify.Check{
			{ID: "build", Stage: verify.StageStatic, Argv: []string{"build"}, FindingCategory: verify.CatACBlocker},
			{ID: "detected:test", Stage: verify.StageUnit, Argv: []string{"test"}, Origin: verify.ProvenanceDetected, FindingCategory: verify.CatACBlocker},
			{ID: "smoke", Stage: verify.StageSmoke, Argv: []string{"smoke"}, FindingCategory: verify.CatACBlocker},
		},
	}
	r := &tq4cRunner{script: map[string][]int{"build": {1}}}
	res, err := verify.RunV1(ctx, pack, r, v1req(), nil, nil, time.Now(), regSettings(t))
	if err != nil {
		t.Fatalf("RunV1: %v", err)
	}
	if len(r.calls) != 1 || r.calls[0] != "build" {
		t.Fatalf("runner calls %v, want only the owner's build — everything after an owner failure is undecidable", r.calls)
	}
	for _, id := range []string{"detected:test", "smoke"} {
		o := tq4cOutcome(t, res, id)
		if o.State != verify.CheckUnverifiable || o.AttributedTo != "build" {
			t.Fatalf("%s outcome %+v, want UNVERIFIABLE-HERE attributed to the owner's build", id, o)
		}
	}
	if b := tq4cCheckBlockers(res.Findings); len(b) != 1 || b[0].Anchor != "check:build" {
		t.Fatalf("check blockers %+v, want exactly check:build", b)
	}
	if _, ok := findingAt(res.Findings, "check:detected:test"); ok {
		t.Fatalf("a blocked detected rung minted a finding: %+v", res.Findings)
	}
}

// TestTQ4cNoteFirstRaisedAtRoundTwoIsNotSuppressed [R6]: the S07.6 new-note
// suppression exists to stop the JUDGE's goalposts drifting round by round. A
// detected rung that first fails at round 2 (the rework broke the detected
// build) is a platform fact about THIS round's verification, not a drifting
// goalpost — suppressing it to a count would re-create the N1 silence one round
// later. The note is itemized, reaches the review surface, and the round is
// SHIP-with-notes with the items verified.
func TestTQ4cNoteFirstRaisedAtRoundTwoIsNotSuppressed(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	r := &tq4cRunner{
		script: map[string][]int{"lint": {1, 0}, "detected:build": {0, 1}},
		tails:  map[string]string{"detected:build": "error: the rework broke the build\n"},
	}
	sink := &fakeSink{}
	v := f.verifier(&fakeJudge{}, r, tq4cMixedPack())
	v.Review = sink
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		d := pkg.Deliverable
		d.Content += "// lint fixed per F1\n"
		return d, nil
	}

	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(out.Rounds) != 2 {
		t.Fatalf("rounds %d, want 2 (round 1 REVISE on the owner's lint, round 2 SHIP-with-notes)", len(out.Rounds))
	}
	r1 := out.Rounds[0]
	if r1.Verdict != verify.VerdictRevise {
		t.Fatalf("round 1 verdict %s, want REVISE", r1.Verdict)
	}
	if _, ok := findingAt(r1.Findings, "check:detected:build"); ok {
		t.Fatalf("round 1: the passing detected build minted a finding: %+v", r1.Findings)
	}
	r2 := out.Rounds[1]
	assertDetectedNote(t, r2.Findings, "detected:build", verify.StageStatic, 1, verify.CatACBlocker, "the rework broke the build")
	if r2.SuppressedNotes != 0 {
		t.Fatalf("round 2 suppressed %d notes — the platform's own evidence note was swallowed by the goalpost-drift rule", r2.SuppressedNotes)
	}
	if r2.Verdict != verify.VerdictShipWithNotes || out.Verdict != verify.VerdictShipWithNotes {
		t.Fatalf("round 2 %s / terminal %s, want SHIP-with-notes", r2.Verdict, out.Verdict)
	}
	if len(out.VerifiedItems) != 1 || out.VerifiedItems[0] != "w1" {
		t.Fatalf("verified items %v, want [w1]", out.VerifiedItems)
	}
	reached := false
	for _, of := range sink.openFindings {
		if of.Anchor == "check:detected:build" && of.Severity == verify.SeverityNote {
			reached = true
		}
	}
	if !reached {
		t.Fatalf("the round-2 note never reached the review surface: %+v", sink.openFindings)
	}
}

// TestTQ4cAJudgeNoteShapedLikeTheDetectedNoteIsStillSuppressed [R6 — GUARD,
// green today]: the exemption is by the platform's unforgeable mark, never by
// anchor shape. A judge note at round 2 anchored exactly like the platform's
// note is a new note like any other and is suppressed to a count.
func TestTQ4cAJudgeNoteShapedLikeTheDetectedNoteIsStillSuppressed(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	calls := 0
	j := &fakeJudge{compliance: func(in verify.JudgeInput) (verify.Axis1Result, error) {
		calls++
		res := passAll(in)
		if calls == 2 {
			res.Findings = []verify.Finding{{
				Severity: verify.SeverityNote, Category: verify.CatACBlocker,
				Anchor: "check:detected:build", Text: "the judge dresses up as the platform",
			}}
		}
		return res, nil
	}}
	r := &tq4cRunner{script: map[string][]int{"lint": {1, 0}}}
	v := f.verifier(j, r, tq4cMixedPack())
	v.Revise = func(_ context.Context, pkg verify.RetryPackage) (verify.Deliverable, error) {
		d := pkg.Deliverable
		d.Content += "// lint fixed per F1\n"
		return d, nil
	}

	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(out.Rounds) != 2 {
		t.Fatalf("rounds %d, want 2", len(out.Rounds))
	}
	r2 := out.Rounds[1]
	if r2.SuppressedNotes != 1 {
		t.Fatalf("round 2 suppressed %d, want 1 — a judge's new note is suppressed whatever its anchor looks like", r2.SuppressedNotes)
	}
	if _, ok := findingAt(r2.Findings, "check:detected:build"); ok {
		t.Fatalf("the judge's forged-shape note was itemized: %+v", r2.Findings)
	}
}

// TestTQ4cBootstrapDetectedFailureMintsTheSameNote [R8]: at a bootstrap round
// the posture disclosure explains why the OWNER's ladder did not run; it says
// nothing about what the platform's own evidence rung found. The same note,
// from the same helper, rides beside the disclosure — the verdict there is
// already advisory SHIP-with-notes and releases nothing, so the only change is
// that the person making the mandatory review decision is told.
func TestTQ4cBootstrapDetectedFailureMintsTheSameNote(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	f.seedTask("t1", "r1")
	runner := &tq4cRunner{script: map[string][]int{"detected:test": {1}}, tails: map[string]string{"detected:test": "# fail 1\n# pass 3\n"}}
	pack := detectedPack(
		detectedCheck("detected:build", verify.StageStatic, "/bin/sh", "-lc", "npm run build"),
		detectedCheck("detected:test", verify.StageUnit, "/bin/sh", "-lc", "npm test"),
	)
	v := f.verifier(&fakeJudge{}, runner, pack)

	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	rd := out.Rounds[0]
	if _, ok := findingAt(rd.Findings, verify.BootstrapAttribution); !ok {
		t.Fatalf("the posture disclosure is gone: %+v", rd.Findings)
	}
	assertDetectedNote(t, rd.Findings, "detected:test", verify.StageUnit, 1, verify.CatACBlocker, "# fail 1")
	if _, ok := findingAt(rd.Findings, "check:detected:build"); ok {
		t.Fatalf("the passing detected build minted a finding: %+v", rd.Findings)
	}
	if b := tq4cCheckBlockers(rd.Findings); len(b) != 0 {
		t.Fatalf("a bootstrap round minted a kill: %+v", b)
	}
	if rd.Verdict != verify.VerdictShipWithNotes || out.Verdict != verify.VerdictShipWithNotes {
		t.Fatalf("round %s / terminal %s, want SHIP-with-notes", rd.Verdict, out.Verdict)
	}
	if !rd.ReviewMandatory || rd.Posture != verify.PostureBootstrap {
		t.Fatalf("round posture %q review_mandatory=%v, want the bootstrap posture unchanged", rd.Posture, rd.ReviewMandatory)
	}
	if len(out.VerifiedItems) != 0 {
		t.Fatalf("an advisory round verified %v", out.VerifiedItems)
	}
}

// TestTQ4cPropAttributionNeverCrossesOriginsAndDetectedFailuresAreNotes [R10
// (i), (ii), (iv) property]: over generated mixed packs (random rungs of random
// origin and stage, random step bindings on owner rungs, exit codes, runner
// errors and quarantines) every outcome's state and attribution match the
// two-lane oracle — an owner rung is blocked only by an earlier OWNER failure
// and attributed to the first one; a detected rung is blocked by the earliest
// failure of ANY origin — no owner outcome or step contract is ever attributed
// to a detected id; every detected FAIL has exactly one note and no blocker;
// every owner FAIL has exactly one blocker citing the check (§80 unchanged).
func TestTQ4cPropAttributionNeverCrossesOriginsAndDetectedFailuresAreNotes(t *testing.T) {
	ctx := context.Background()
	s := regSettings(t)
	stages := []verify.LadderStage{verify.StageStatic, verify.StageUnit, verify.StageSmoke, verify.StageE2E}
	type expect struct {
		state verify.CheckOutcomeState
		by    string
	}
	detectedFails, ownerFails, crossings, ownerBlocksDetected, quarantines, runnerErrs := 0, 0, 0, 0, 0, 0
	for seed := int64(1); seed <= 12; seed++ {
		r := rand.New(rand.NewSource(seed))
		for n := 1; n <= 7; n++ {
			pack := &verify.CheckPack{Domain: verify.DomainSoftware, Version: 1, VerifiedOn: time.Now().Add(-time.Hour)}
			runner := &tq4cRunner{script: map[string][]int{}, errs: map[string]error{}, tails: map[string]string{}}
			detectedIDs := map[string]bool{}
			var quarantined []string
			for k := 1; k <= n; k++ {
				id := fmt.Sprintf("c%d", k)
				c := verify.Check{Stage: stages[r.Intn(len(stages))], FindingCategory: verify.CatACBlocker}
				if r.Intn(2) == 0 {
					id = "detected:" + id
					c.Origin = verify.ProvenanceDetected
					detectedIDs[id] = true
				} else if r.Intn(3) == 0 {
					c.StepID = fmt.Sprintf("S-%d", 1+r.Intn(2))
				}
				c.ID, c.Argv = id, []string{id}
				pack.Checks = append(pack.Checks, c)
				switch r.Intn(6) {
				case 0, 1:
					runner.script[id] = []int{1 + r.Intn(3)}
				case 2:
					runner.errs[id] = errors.New("sandbox compose failed")
				}
				runner.tails[id] = "output of " + id + "\n"
				if r.Intn(7) == 0 {
					quarantined = append(quarantined, id)
				}
			}
			for _, id := range quarantined {
				if err := pack.QuarantineCheck(verify.Quarantine{CheckID: id, Owner: "op", Reason: "flaky", FixBy: time.Now().Add(48 * time.Hour)}); err != nil {
					t.Fatalf("seed %d n=%d: quarantine %s: %v", seed, n, id, err)
				}
			}

			// The oracle: two lanes, walked in ladder order then pack order.
			ownerFirst, ownerStage, anyFirst, anyStage := "", -1, "", -1
			want := map[string]expect{}
			for rank, stage := range stages {
				for _, c := range pack.Checks {
					if c.Stage != stage {
						continue
					}
					det := c.Origin == verify.ProvenanceDetected
					blockedBy, blocked := "", false
					switch {
					case det && anyStage >= 0 && rank > anyStage:
						blocked, blockedBy = true, anyFirst
					case !det && ownerStage >= 0 && rank > ownerStage:
						blocked, blockedBy = true, ownerFirst
					}
					switch {
					case blocked:
						want[c.ID] = expect{verify.CheckUnverifiable, blockedBy}
						if det && !detectedIDs[blockedBy] {
							ownerBlocksDetected++
						}
					case pack.Quarantines[c.ID].CheckID != "":
						want[c.ID] = expect{verify.CheckQuarantined, ""}
						quarantines++
					case runner.errs[c.ID] != nil:
						want[c.ID] = expect{verify.CheckRunnerFailed, ""}
						runnerErrs++
					case len(runner.script[c.ID]) > 0:
						want[c.ID] = expect{verify.CheckFailed, ""}
						if anyStage < 0 || rank < anyStage {
							anyStage = rank
						}
						if anyFirst == "" {
							anyFirst = c.ID
						}
						if det {
							detectedFails++
						} else {
							ownerFails++
							if ownerStage < 0 || rank < ownerStage {
								ownerStage = rank
							}
							if ownerFirst == "" {
								ownerFirst = c.ID
							}
						}
					default:
						want[c.ID] = expect{verify.CheckPassed, ""}
						// An owner rung that RAN at a stage after a detected
						// failure is the crossing this packet closes.
						if !det && anyStage >= 0 && rank > anyStage && detectedIDs[anyFirst] {
							crossings++
						}
					}
				}
			}

			res, err := verify.RunV1(ctx, pack, runner, v1req(), ladderSteps()[:2], map[string][]string{"AC-1": {"S-1"}}, time.Now(), s)
			if err != nil {
				t.Fatalf("seed %d n=%d: RunV1: %v", seed, n, err)
			}
			if len(res.Checks) != len(pack.Checks) {
				t.Fatalf("seed %d n=%d: %d outcomes for %d checks", seed, n, len(res.Checks), len(pack.Checks))
			}
			for _, o := range res.Checks {
				w, ok := want[o.CheckID]
				if !ok {
					t.Fatalf("seed %d n=%d: unexpected outcome %+v", seed, n, o)
				}
				if o.State != w.state || o.AttributedTo != w.by {
					t.Fatalf("seed %d n=%d: %s recorded %s/%q, want %s/%q (owner lane first failure %q, any lane %q; checks %+v)",
						seed, n, o.CheckID, o.State, o.AttributedTo, w.state, w.by, ownerFirst, anyFirst, pack.Checks)
				}
				if !detectedIDs[o.CheckID] && detectedIDs[o.AttributedTo] {
					t.Fatalf("seed %d n=%d: owner rung %s attributed to detected %s — attribution crossed origins", seed, n, o.CheckID, o.AttributedTo)
				}
				anchor := "check:" + o.CheckID
				at := tq4cFindingsAt(res.Findings, anchor)
				var kills, notes, integrity int
				for _, fd := range at {
					switch {
					case fd.Category == verify.CatCheckIntegrity:
						integrity++
					case fd.Severity == verify.SeverityBlocker && !fd.Demoted:
						kills++
					case fd.Severity == verify.SeverityNote:
						notes++
					}
				}
				switch {
				case o.State == verify.CheckFailed && detectedIDs[o.CheckID]:
					if notes != 1 || kills != 0 || integrity != 0 {
						t.Fatalf("seed %d n=%d: detected FAIL %s has notes=%d kills=%d integrity=%d, want exactly one note: %+v", seed, n, o.CheckID, notes, kills, integrity, at)
					}
					assertDetectedNote(t, res.Findings, o.CheckID, o.Stage, o.ExitCode, verify.CatACBlocker, "output of "+o.CheckID)
				case o.State == verify.CheckFailed:
					if kills != 1 || notes != 0 || integrity != 0 || at[0].Criterion != anchor {
						t.Fatalf("seed %d n=%d: owner FAIL %s has kills=%d notes=%d integrity=%d, want exactly the one blocker citing the check: %+v", seed, n, o.CheckID, kills, notes, integrity, at)
					}
				default:
					if kills != 0 || notes != 0 {
						t.Fatalf("seed %d n=%d: %s outcome %s carries kills=%d notes=%d: %+v", seed, n, o.CheckID, o.State, kills, notes, at)
					}
				}
			}
			for _, sc := range res.Steps {
				if detectedIDs[sc.AttributedTo] {
					t.Fatalf("seed %d n=%d: step %s attributed to detected %s — a platform guess silenced an owner's step: %+v", seed, n, sc.StepID, sc.AttributedTo, sc)
				}
			}
		}
	}
	if detectedFails == 0 || ownerFails == 0 || crossings == 0 || ownerBlocksDetected == 0 || quarantines == 0 || runnerErrs == 0 {
		t.Fatalf("the generator left a class unexercised: detectedFails=%d ownerFails=%d crossings=%d ownerBlocksDetected=%d quarantines=%d runnerErrs=%d",
			detectedFails, ownerFails, crossings, ownerBlocksDetected, quarantines, runnerErrs)
	}
}

// TestTQ4cPropARoundWhoseOnlyFailuresAreDetectedShipsWithNotes [R10 (iii)
// property, through the real drain]: with the owner's rungs passing and any
// subset of the detected rungs failing, the round is SHIP-with-notes exactly
// when a detected rung that RAN failed (clean SHIP when none did), never
// REVISE, with the items verified either way, no card, and one note per
// detected FAIL on the review surface.
func TestTQ4cPropARoundWhoseOnlyFailuresAreDetectedShipsWithNotes(t *testing.T) {
	ctx := context.Background()
	clean, noted := 0, 0
	for mask := 0; mask < 4; mask++ {
		for _, tailed := range []bool{false, true} {
			f := newFix(t)
			f.seedTask("t1", "r1")
			runner := &tq4cRunner{script: map[string][]int{}, tails: map[string]string{}}
			if mask&1 != 0 {
				runner.script["detected:build"] = []int{1}
			}
			if mask&2 != 0 {
				runner.script["detected:test"] = []int{2}
			}
			if tailed {
				runner.tails["detected:build"] = "build says no\n"
				runner.tails["detected:test"] = "test says no\n"
			}
			sink := &fakeSink{}
			v := f.verifier(&fakeJudge{}, runner, tq4cMixedPack())
			v.Review = sink

			out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
			if err != nil {
				t.Fatalf("mask %d: Verify: %v", mask, err)
			}
			rd := out.Rounds[0]
			// Which detected rungs FAILED (ran and exited non-zero), by the
			// recorded outcomes — a blocked rung is undecided, not failed.
			var failed []verify.CheckOutcome
			for _, o := range rd.V1.Checks {
				if strings.HasPrefix(o.CheckID, "detected:") && o.State == verify.CheckFailed {
					failed = append(failed, o)
				}
			}
			want := verify.VerdictShip
			if len(failed) > 0 {
				want = verify.VerdictShipWithNotes
				noted++
			} else {
				clean++
			}
			if rd.Verdict != want || out.Verdict != want {
				t.Fatalf("mask %d: round %s / terminal %s, want %s (failed detected rungs %v)", mask, rd.Verdict, out.Verdict, want, failed)
			}
			if out.Card != nil || len(out.IntegrityCards) != 0 {
				t.Fatalf("mask %d: cards %+v / %+v, want none", mask, out.Card, out.IntegrityCards)
			}
			if len(out.VerifiedItems) != 1 || out.VerifiedItems[0] != "w1" {
				t.Fatalf("mask %d: verified items %v, want [w1]", mask, out.VerifiedItems)
			}
			if b := tq4cCheckBlockers(rd.Findings); len(b) != 0 {
				t.Fatalf("mask %d: blockers %+v, want none", mask, b)
			}
			for _, o := range failed {
				tail := ""
				if tailed {
					tail = "says no"
				}
				assertDetectedNote(t, rd.Findings, o.CheckID, o.Stage, o.ExitCode, verify.CatACBlocker, tail)
				assertDetectedNote(t, sink.openFindings, o.CheckID, o.Stage, o.ExitCode, verify.CatACBlocker, tail)
			}
			for _, o := range rd.V1.Checks {
				if o.State != verify.CheckFailed {
					if at := tq4cFindingsAt(rd.Findings, "check:"+o.CheckID); len(at) != 0 {
						t.Fatalf("mask %d: %s (%s) minted %+v", mask, o.CheckID, o.State, at)
					}
				}
			}
		}
	}
	if clean == 0 || noted == 0 {
		t.Fatalf("both classes must be exercised: clean=%d noted=%d", clean, noted)
	}
}

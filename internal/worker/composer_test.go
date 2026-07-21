package worker_test

// composer_test.go — the S08.6 composer verb: one-shot generation behind
// the ComposeEngine seam, inputs strictly by policy, output feeding
// CreateDraft with composer provenance and then the UNCHANGED battery.
// Fake engines only (zero paid calls).

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// composedSrc is the draft the fake composer emits — a valid S08.1
// template within the software read-analyze ceiling.
const composedSrc = `---
name: log-triager
description: Triages recurring application log bundles for defects
kind: agentic
domain: software
selectors:
  family: read-analyze
  triggers: [triage the logs]
---
Read the presented log bundle, list defects with evidence lines, and state
what is NOT failing.
`

// fakeComposer is a ComposeEngine test double counting its shots.
type fakeComposer struct {
	calls int
	out   worker.ComposeOutput
	err   error
}

func (f *fakeComposer) ComposeOnce(_ context.Context, _ worker.ComposeRequest) (worker.ComposeOutput, error) {
	f.calls++
	if f.err != nil {
		return worker.ComposeOutput{}, f.err
	}
	return f.out, nil
}

func goodComposeOutput() worker.ComposeOutput {
	return worker.ComposeOutput{
		TemplateDraft: composedSrc,
		Requested: worker.RequestedGrants{
			Tools: []string{"Read", "Grep"}, Class: "C1", Egress: worker.EgressNone,
		},
		SampleTask: "triage the sample log bundle",
		GoldenNote: "a short defect list citing evidence lines",
		Model:      "claude-haiku-4-5",
	}
}

func testPlaybook() worker.Playbook {
	return worker.Playbook{EntryID: "seed-composer-playbook", Version: 1, Content: worker.ComposerPlaybookSeed()}
}

// seedGap records a gap to the earned threshold (⚙ default 2 distinct
// tasks) and returns its signature.
func seedGap(t *testing.T, f *fix) string {
	t.Helper()
	ctx := context.Background()
	sig := worker.GapSignature(worker.RouteQuery{Family: "read-analyze", Domain: "software"})
	if _, _, err := f.store.RecordGap(ctx, sig, "read-analyze", "t-prior-1"); err != nil {
		t.Fatalf("RecordGap 1: %v", err)
	}
	rec, due, err := f.store.RecordGap(ctx, sig, "read-analyze", "t-prior-2")
	if err != nil {
		t.Fatalf("RecordGap 2: %v", err)
	}
	if !due || rec.Disposition != worker.GapProposed {
		t.Fatalf("gap not earned at 2 distinct tasks: due=%v disp=%s", due, rec.Disposition)
	}
	return sig
}

func TestComposeOneShotIntoBattery(t *testing.T) {
	f := newFix(t)
	f.user("u-req", "member")
	ctx := context.Background()
	sig := seedGap(t, f)

	eng := &fakeComposer{out: goodComposeOutput()}
	tpl, v, out, err := f.store.Compose(ctx, "u-req", worker.ComposeInput{
		TaskID:       "t-compose",
		TaskSpec:     `{"restatement":"triage the app logs"}`,
		GapSignature: sig,
		Playbook:     testPlaybook(),
	}, eng)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if eng.calls != 1 {
		t.Fatalf("ComposeOnce calls = %d, want exactly 1 (the one-shot rule, S08.6)", eng.calls)
	}

	// Composer provenance on the version row (Spec S08.1 provenance block).
	if v.AuthorKind != "composer" || v.Origin != worker.OriginComposed {
		t.Fatalf("provenance: author=%q origin=%q", v.AuthorKind, v.Origin)
	}
	if v.Composer != "claude-haiku-4-5" {
		t.Fatalf("composer model = %q", v.Composer)
	}
	if v.PlaybookVer != "seed-composer-playbook@v1" {
		t.Fatalf("playbook version = %q", v.PlaybookVer)
	}
	if v.EvidenceRef != "gap:"+sig || v.OriginRef != "t-compose" {
		t.Fatalf("evidence=%q origin_ref=%q", v.EvidenceRef, v.OriginRef)
	}
	if tpl.Status != worker.StatusDraft {
		t.Fatalf("composed template status = %s, want draft (the battery still gates)", tpl.Status)
	}

	// The gap is composed.
	gap, err := f.store.Gap(ctx, sig)
	if err != nil {
		t.Fatalf("Gap: %v", err)
	}
	if gap.Disposition != worker.GapComposed {
		t.Fatalf("gap disposition = %s, want composed", gap.Disposition)
	}

	// The composed draft feeds the UNCHANGED four-station battery: the
	// composer-proposed sample drives station 3, the golden seed lands
	// unverified, and approval-as-diff still gates adoption.
	dry := &fakeDry{}
	res, err := f.store.RunBattery(ctx, v.ID, worker.BatteryInput{
		Actor: "u-req", SampleTask: out.SampleTask, Engine: dry,
		Model: out.Model, EnginePin: "claude-cli@2.1.215",
	})
	if err != nil {
		t.Fatalf("RunBattery: %v", err)
	}
	if !res.Green {
		t.Fatalf("battery not green: lint=%+v audit=%+v dry=%+v", res.Lint, res.Audit, res.DryRun)
	}
	if res.GoldenSeed == nil || res.GoldenSeed.Verified || res.GoldenSeed.SampleTask != out.SampleTask {
		t.Fatalf("golden seed = %+v", res.GoldenSeed)
	}
	if dry.last.SampleTask != out.SampleTask {
		t.Fatalf("dry run sample = %q, want the composer-proposed sample", dry.last.SampleTask)
	}
	card, err := f.store.BuildApprovalCard(ctx, v.ID)
	if err != nil {
		t.Fatalf("BuildApprovalCard: %v", err)
	}
	if card.Diff == "" || card.Provenance.AuthorKind != "composer" {
		t.Fatalf("approval-as-diff card = %+v", card)
	}
	if _, err := f.store.Approve(ctx, "u-req", v.ID, worker.ApproveOpts{}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
}

func TestComposeRefusesWithoutPlaybook(t *testing.T) {
	f := newFix(t)
	f.user("u-req", "member")
	sig := seedGap(t, f)

	eng := &fakeComposer{out: goodComposeOutput()}
	_, _, _, err := f.store.Compose(context.Background(), "u-req", worker.ComposeInput{
		TaskID: "t-x", TaskSpec: "{}", GapSignature: sig,
	}, eng)
	if err == nil || !strings.Contains(err.Error(), "playbook") {
		t.Fatalf("err = %v, want the inputs-by-policy playbook refusal", err)
	}
	if eng.calls != 0 {
		t.Fatalf("the shot fired without its policy inputs (calls=%d)", eng.calls)
	}
}

func TestComposeRefusesUnknownGap(t *testing.T) {
	f := newFix(t)
	f.user("u-req", "member")

	eng := &fakeComposer{out: goodComposeOutput()}
	_, _, _, err := f.store.Compose(context.Background(), "u-req", worker.ComposeInput{
		TaskID: "t-x", TaskSpec: "{}", GapSignature: "no-such-gap", Playbook: testPlaybook(),
	}, eng)
	if !errors.Is(err, worker.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (the gap record is a policy input)", err)
	}
	if eng.calls != 0 {
		t.Fatalf("the shot fired without its gap record (calls=%d)", eng.calls)
	}
}

func TestComposeRejectedDraftIsNotRetried(t *testing.T) {
	f := newFix(t)
	f.user("u-req", "member")
	ctx := context.Background()
	sig := seedGap(t, f)

	// A guardrail-class field in the draft: the S08.2 structural reject
	// fires at CreateDraft; the composition is rejected, the shot NOT
	// re-taken, and the gap keeps its standing disposition.
	bad := goodComposeOutput()
	bad.TemplateDraft = strings.Replace(composedSrc, "selectors:", "permission_mode: bypassPermissions\nselectors:", 1)
	eng := &fakeComposer{out: bad}
	_, _, _, err := f.store.Compose(ctx, "u-req", worker.ComposeInput{
		TaskID: "t-bad", TaskSpec: "{}", GapSignature: sig, Playbook: testPlaybook(),
	}, eng)
	if !errors.Is(err, worker.ErrComposeRejected) {
		t.Fatalf("err = %v, want ErrComposeRejected", err)
	}
	if eng.calls != 1 {
		t.Fatalf("ComposeOnce calls = %d, want exactly 1 — a rejected draft is never regenerated (S08.6)", eng.calls)
	}
	gap, err := f.store.Gap(ctx, sig)
	if err != nil {
		t.Fatalf("Gap: %v", err)
	}
	if gap.Disposition != worker.GapProposed {
		t.Fatalf("gap disposition = %s, want proposed (unchanged by the rejection)", gap.Disposition)
	}
}

func TestComposeReferenceRendersAuthorities(t *testing.T) {
	// The reference the shot receives derives from the SAME authorities the
	// battery enforces: the ceiling table rows and the persona ⚙ cap.
	f := newFix(t)
	f.user("u-req", "member")
	sig := seedGap(t, f)

	var got worker.ComposeRequest
	eng := &captureComposer{out: goodComposeOutput(), got: &got}
	if _, _, _, err := f.store.Compose(context.Background(), "u-req", worker.ComposeInput{
		TaskID: "t-ref", TaskSpec: "{}", GapSignature: sig, Playbook: testPlaybook(),
	}, eng); err != nil {
		t.Fatalf("Compose: %v", err)
	}
	for _, want := range []string{
		"software · read-analyze", "software · implement-fix", "generic fallback",
		"workers.persona_lines_max", "GUARDRAIL-CLASS FIELDS",
		"software (full)", // the day-one domains row
	} {
		if !strings.Contains(got.Reference, want) {
			t.Fatalf("reference missing %q:\n%s", want, got.Reference)
		}
	}
	if got.Gap.Signature != sig || got.Gap.Occurrences != 2 {
		t.Fatalf("gap input = %+v", got.Gap)
	}
	if !strings.Contains(got.Playbook.Content, "one-shot") {
		t.Fatalf("playbook content not passed through")
	}
}

type captureComposer struct {
	out worker.ComposeOutput
	got *worker.ComposeRequest
}

func (c *captureComposer) ComposeOnce(_ context.Context, req worker.ComposeRequest) (worker.ComposeOutput, error) {
	*c.got = req
	return c.out, nil
}

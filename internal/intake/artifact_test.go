package intake

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The artifact pair (Spec S06.6): validation contract, durable write with
// markdown-of-record + sidecar, and resume-integrity verification.

func testPair(task, owner string) Pair {
	return Pair{
		Spec: Spec{
			TaskID: task, Owner: owner, Version: 1, Status: StatusDraft,
			Tier: TierStandard, Provenance: "test-planner v1",
			Restatement: "Do the thing the requester asked.",
			Outcome:     []string{"the requester recognizes their goal"},
			ACs: []AC{
				{N: 1, Plain: "the change works", Structured: "WHEN run THEN exit 0", StructuredKind: "ears"},
				{N: 2, Plain: "the result is verified"},
			},
			Constraints: []string{"stay within the repo"},
			Assumptions: []Assumption{{Text: "scope is the main branch", Origin: "planner"}},
			OutOfScope:  []string{"no deploys"},
		},
		Plan: Plan{
			TaskID: task, Owner: owner, Version: 1, SpecVersion: 1, Status: StatusDraft,
			Tier: TierStandard, Provenance: "test-planner v1",
			Steps: []Step{
				{ID: "S-1", Title: "Implement", DoneWhen: "tests pass", Class: "C1", WriteSet: []string{"src/**"}},
				{ID: "S-2", Title: "Verify", DoneWhen: "ACs demonstrably hold", Class: "C1"},
			},
			Coverage: map[string][]string{"AC-1": {"S-1"}, "AC-2": {"S-2"}},
			Risks:    []string{"the estimate may be off"},
			Est:      Estimate{SizeClass: "S", USD: 1.0, Known: true, Basis: "test"},
		},
	}
}

func TestPairValidate(t *testing.T) {
	p := testPair("t1", "u1")
	if err := p.Validate(); err != nil {
		t.Fatalf("valid pair rejected: %v", err)
	}

	bad := testPair("t1", "u1")
	bad.Spec.ACs[1].N = 3 // gap
	if err := bad.Validate(); err == nil {
		t.Fatal("non-contiguous AC numbering accepted")
	}

	bad = testPair("t1", "u1")
	bad.Plan.Steps[0].Class = "C3"
	if err := bad.Validate(); err == nil {
		t.Fatal("confinement class outside C0–C2 accepted (v0)")
	}

	bad = testPair("t1", "u1")
	bad.Plan.Coverage["AC-9"] = []string{"S-1"}
	if err := bad.Validate(); err == nil {
		t.Fatal("coverage over unknown criterion accepted")
	}

	bad = testPair("t1", "u1")
	bad.Plan.SpecVersion = 2
	if err := bad.Validate(); err == nil {
		t.Fatal("spec/plan version mismatch accepted")
	}
}

func TestArtifactRoundTrip(t *testing.T) {
	store := artifactStore{root: t.TempDir()}
	pair := testPair("t1", "u1")

	specRef, err := store.write(store.specPath("t1", 1), renderSpecMD(&pair.Spec), &pair.Spec, 1)
	if err != nil {
		t.Fatalf("write spec: %v", err)
	}
	planRef, err := store.write(store.planPath("t1", 1), renderPlanMD(&pair.Plan), &pair.Plan, 1)
	if err != nil {
		t.Fatalf("write plan: %v", err)
	}

	spec, err := store.loadSpec(&specRef)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	if spec.ACs[0].Key() != "AC-1" || spec.ACs[0].Structured == "" {
		t.Fatalf("spec did not survive: %+v", spec)
	}
	plan, err := store.loadPlan(&planRef)
	if err != nil {
		t.Fatalf("load plan: %v", err)
	}
	if plan.Steps[0].DoneWhen != "tests pass" {
		t.Fatalf("plan did not survive: %+v", plan)
	}

	// The markdown of record carries the S06.6 contract sections.
	md, _ := os.ReadFile(specRef.Path)
	for _, want := range []string{"## Restatement", "## Acceptance criteria", "**AC-1**", "(ears)", "## Assumptions", "## Out of scope — will NOT do"} {
		if !strings.Contains(string(md), want) {
			t.Fatalf("spec markdown missing %q", want)
		}
	}
	planMD, _ := os.ReadFile(planRef.Path)
	for _, want := range []string{"Done when: tests pass", "Confinement: C1", "## Coverage map", "- AC-1 → S-1"} {
		if !strings.Contains(string(planMD), want) {
			t.Fatalf("plan markdown missing %q", want)
		}
	}
}

func TestArtifactHashDriftDetected(t *testing.T) {
	store := artifactStore{root: t.TempDir()}
	pair := testPair("t1", "u1")
	ref, err := store.write(store.specPath("t1", 1), renderSpecMD(&pair.Spec), &pair.Spec, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ref.Path, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.loadSpec(&ref); err == nil {
		t.Fatal("tampered markdown of record loaded silently")
	}
}

func TestWriteGlobs(t *testing.T) {
	p := testPair("t1", "u1").Plan
	globs, unbounded := p.WriteGlobs()
	if unbounded || len(globs) != 1 || globs[0] != "src/**" {
		t.Fatalf("globs=%v unbounded=%v", globs, unbounded)
	}
	p.Steps[1].Unbounded = true
	if _, unbounded := p.WriteGlobs(); !unbounded {
		t.Fatal("unbounded step not reported")
	}
}

func TestApprovedPathNaming(t *testing.T) {
	got := approvedPath(filepath.Join("x", "SPEC-v2.md"))
	if got != filepath.Join("x", "SPEC-v2.approved.md") {
		t.Fatalf("approvedPath = %q", got)
	}
}

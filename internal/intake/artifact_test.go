package intake

import (
	"bytes"
	"fmt"
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

// ---- P3-GF8: pre-A15 re-render integrity (brief §9 property mark (b)) ----

// gf8PreA15Golden holds the markdown of record for a seeded family of plans
// whose [A15] members are all empty, rendered by the PRE-A15 renderer and
// committed with the packet's red tests. It is the ORACLE the property below
// needs: "byte-identical" is only checkable against bytes minted before the
// change, and every other formulation would compare the new renderer with
// itself.
//
// Regenerate deliberately (it is a contract change, not a refresh):
//
//	SINET_WRITE_PREA15_GOLDEN=1 go test ./internal/intake -run PreA15
const gf8PreA15Golden = "testdata/gf8-prea15-plans.md"

const gf8PreA15WriteEnv = "SINET_WRITE_PREA15_GOLDEN"

// gf8Rand is a tiny deterministic xorshift, deliberately NOT math/rand: the
// golden bytes must stay reproducible across Go releases, and the standard
// library's generator does not promise that its stream never changes.
type gf8Rand uint64

func (r *gf8Rand) next() uint64 {
	x := uint64(*r)
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	*r = gf8Rand(x)
	return x
}

func (r *gf8Rand) intn(n int) int { return int(r.next() % uint64(n)) }

func (r *gf8Rand) pick(from []string) string { return from[r.intn(len(from))] }

// gf8PreA15Plan generates one plan with NO [A15] content — the shape every
// artifact stored before this packet has on disk.
func gf8PreA15Plan(r *gf8Rand, i int) Plan {
	titles := []string{"Implement the change", "Verify against the criteria", "Collect the merged changes", "Write the note"}
	dones := []string{"tests pass", "all ACs demonstrably hold", "the note exists", "the checks are green"}
	globs := []string{"src/**", "app/**", "docs/*.md"}
	classes := []string{"C0", "C1", "C2"}

	n := 1 + r.intn(4)
	steps := make([]Step, 0, n)
	coverage := map[string][]string{}
	for k := 1; k <= n; k++ {
		s := Step{
			ID:       fmt.Sprintf("S-%d", k),
			Title:    r.pick(titles),
			DoneWhen: r.pick(dones),
			Class:    r.pick(classes),
		}
		if r.intn(2) == 0 {
			s.WriteSet = []string{r.pick(globs)}
		}
		if r.intn(3) == 0 {
			s.ReadSet = []string{r.pick(globs)}
		}
		if r.intn(4) == 0 {
			s.Unbounded = true
		}
		if r.intn(4) == 0 {
			s.OutwardEffects = []string{"publishes the note"}
		}
		s.NewSpend = r.intn(4) == 0
		s.CredentialTouch = r.intn(5) == 0
		s.SharedAssetWrite = r.intn(5) == 0
		if r.intn(4) == 0 {
			s.NewTools = []string{"ripgrep"}
		}
		s.Research = r.intn(3) == 0
		steps = append(steps, s)
		coverage[fmt.Sprintf("AC-%d", k)] = []string{s.ID}
	}
	p := Plan{
		TaskID: fmt.Sprintf("t-%d", i), Owner: "u1", Version: 1 + r.intn(3), SpecVersion: 1,
		Status: StatusDraft, Tier: TierStandard, Provenance: "pre-A15 planner",
		Steps: steps, Coverage: coverage,
	}
	p.SpecVersion = p.Version
	if r.intn(2) == 0 {
		p.Risks = []string{"the estimate may be off"}
	}
	if r.intn(3) == 0 {
		p.CitedEntries = []string{"project:conventions"}
	}
	if steps[0].Research {
		p.ResearchNodes = []ResearchNode{{RuleID: "P47-7", StepID: "S-1", Query: "verify the current version"}}
	}
	if r.intn(2) == 0 {
		p.Est = Estimate{SizeClass: "S", USD: 1.25, Known: true, Basis: "median of the last five"}
	} else {
		p.Est = Estimate{SizeClass: "M", Known: false}
	}
	return p
}

// TestPreA15RerenderIntegrityProperty — PROPERTY (brief R12; §9 mark (b)):
// over a seeded family of plans carrying NO [A15] content, the markdown of
// record is byte-identical to what the pre-A15 renderer produced, and each one
// still survives the S06.6 write/load round-trip (sha256 + byte-identical
// re-render). This is the CitedEntries precedent held as a property rather than
// as one example: R12's step lines are GUARDED, so every artifact stored before
// this packet keeps loading.
func TestPreA15RerenderIntegrityProperty(t *testing.T) {
	r := gf8Rand(0xA15C0FFEE)
	store := artifactStore{root: t.TempDir()}
	var all bytes.Buffer
	for i := 0; i < 64; i++ {
		plan := gf8PreA15Plan(&r, i)
		md := renderPlanMD(&plan)
		all.Write(md)

		for _, marker := range []string{"Approach", "Decision", "Alternatives", "Ordering"} {
			if bytes.Contains(md, []byte(marker)) {
				t.Fatalf("plan %d has no [A15] content but its markdown of record carries %q:\n%s", i, marker, md)
			}
		}
		ref, err := store.write(store.planPath(plan.TaskID, plan.Version), md, &plan, plan.Version)
		if err != nil {
			t.Fatalf("write plan %d: %v", i, err)
		}
		if _, err := store.loadPlan(&ref); err != nil {
			t.Fatalf("a pre-A15 plan no longer loads: %v", err)
		}
	}

	if os.Getenv(gf8PreA15WriteEnv) == "1" {
		if err := os.MkdirAll(filepath.Dir(gf8PreA15Golden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(gf8PreA15Golden, all.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("wrote %s — unset %s to run the comparison", gf8PreA15Golden, gf8PreA15WriteEnv)
	}
	want, err := os.ReadFile(gf8PreA15Golden)
	if err != nil {
		t.Fatalf("read the pre-A15 oracle: %v", err)
	}
	if !bytes.Equal(all.Bytes(), want) {
		t.Errorf("the markdown of record for pre-A15 plans changed: %d bytes now, %d in %s — every stored artifact would fail its re-render check (S06.6)",
			all.Len(), len(want), gf8PreA15Golden)
	}
}

func TestApprovedPathNaming(t *testing.T) {
	got := approvedPath(filepath.Join("x", "SPEC-v2.md"))
	if got != filepath.Join("x", "SPEC-v2.approved.md") {
		t.Fatalf("approvedPath = %q", got)
	}
}

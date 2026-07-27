package shell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/benchmark"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/conformance"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/evals"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
)

// benchmark_seams_test.go — the B5-7 composition-root coverage. Hermetic and
// $0: nothing is dispatched, no engine is reached, and the direct-arm text seam
// is asserted to be honestly absent rather than faked.

// TestBenchmarkDomainMappingIsPinnedAtComposition: internal/benchmark holds the
// code→registered domain mapping without importing internal/verify, so the
// composition root asserts the other side of it. A rename on either side must
// fail the boot rather than mislabel every pair record.
func TestBenchmarkDomainMappingIsPinnedAtComposition(t *testing.T) {
	if err := checkBenchmarkDomainMapping(); err != nil {
		t.Fatalf("the BENCH-REG §10.1 domain mapping broke: %v", err)
	}
}

// TestFloorSeamIsHonestlyNotEvaluable is BENCH-REG §10.1(d) at the seam: an
// absent surface, an unregistered domain, an unratified floor and a sweep that
// has never run are all "not evaluable", never a fake green — because a gate
// that cannot be evaluated cannot open.
func TestFloorSeamIsHonestlyNotEvaluable(t *testing.T) {
	ctx := context.Background()
	db, log, reg := watchlistTestDeps(t)

	// No surface composed at all: the seam is nil, and internal/benchmark reads
	// a nil seam as "not evaluable".
	if benchmarkFloorSeam(nil) != nil {
		t.Fatal("an absent evals surface must yield a nil seam, so limb (d) reads not-evaluable")
	}

	conf := conformance.NewStore(db, log, reg)
	if _, err := conf.EnsureSeeded(ctx); err != nil {
		t.Fatal(err)
	}
	store := evals.NewStore(db, conf, reg)
	seam := benchmarkFloorSeam(store)
	if seam == nil {
		t.Fatal("a composed evals surface must yield a seam")
	}

	// An unregistered domain: web-research's suites register at their 8.3 entry
	// (BENCH-REG §10.2), so at v0 it is not evaluable and says why.
	rep, err := seam(ctx, benchmark.DomainWebResearch)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Evaluable || rep.Green {
		t.Errorf("web-research must not be evaluable at v0: %+v", rep)
	}
	if !strings.Contains(rep.Reason, "no registered regression suites") {
		t.Errorf("the reason must name the gap: %q", rep.Reason)
	}

	// Floors not registered yet.
	rep, err = seam(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Evaluable {
		t.Errorf("an unfloored domain must not be evaluable: %+v", rep)
	}
	if !strings.Contains(rep.Reason, "no registered floor") {
		t.Errorf("the reason must name the unfloored version: %q", rep.Reason)
	}

	// Registered but NOT ratified: still not a standard the gate may lean on.
	if _, err := store.EnsureFloorsRegistered(ctx); err != nil {
		t.Fatal(err)
	}
	rep, err = seam(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Evaluable {
		t.Errorf("an unratified floor must not be evaluable: %+v", rep)
	}
	if !strings.Contains(rep.Reason, "NOT ratified") {
		t.Errorf("the reason must name the missing ratification: %q", rep.Reason)
	}

	// Ratified, but no sweep has ever run: there is no result to be green.
	for _, f := range evals.SeedFloors() {
		if err := store.Ratify(ctx, f.AssetID, f.AssetVersion); err != nil {
			t.Fatal(err)
		}
	}
	rep, err = seam(ctx, benchmark.DomainSoftwareDevelopment)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Evaluable {
		t.Errorf("with no sweep result the limb is not evaluable: %+v", rep)
	}
	if !strings.Contains(rep.Reason, "never run") {
		t.Errorf("the reason must say the sweep has never run: %q", rep.Reason)
	}
}

// TestDirectArmTextSeamIsHonestlyAbsent: no pair can complete until the
// dispatcher captures a single-shot direct-arm output and B6 wires the verdict
// form. The seam says so instead of returning a placeholder — a fabricated
// "direct arm" would make every statistic downstream fiction.
func TestDirectArmTextSeamIsHonestlyAbsent(t *testing.T) {
	seam := benchmarkDirectTextSeam()
	if seam == nil {
		t.Fatal("the direct-arm text seam must exist so its absence is legible")
	}
	body, err := seam(context.Background(), "bp-1.direct")
	if err == nil {
		t.Fatal("the seam must report the gap, not return a body")
	}
	if body != "" {
		t.Errorf("the seam returned %q — a placeholder arm would corrupt the record", body)
	}
	if !strings.Contains(err.Error(), "BENCH-REG §2") {
		t.Errorf("the error must cite the clause it cannot yet satisfy: %v", err)
	}
}

// TestBenchmarkSurfaceSeedsOnlyDomainBookkeeping: the boot registers the v0
// launch roster's bookkeeping rows idempotently and manufactures NOTHING else.
// BENCH-REG §4.5's bring-up batch is an operator intent, not a boot step.
func TestBenchmarkSurfaceSeedsOnlyDomainBookkeeping(t *testing.T) {
	ctx := context.Background()
	db, log, reg := watchlistTestDeps(t)
	evalStore := evals.NewStore(db, conformance.NewStore(db, log, reg), reg)

	bs, err := buildBenchmarkSurface(ctx, db, log, reg, nil, nil, nil, evalStore, nil, nil, testLogger())
	if err != nil {
		t.Fatalf("buildBenchmarkSurface: %v", err)
	}
	if bs.Domains != len(benchmark.V0LaunchDomains()) {
		t.Errorf("seeded %d domains, want the v0 roster's %d", bs.Domains, len(benchmark.V0LaunchDomains()))
	}
	if !bs.FloorsWired {
		t.Error("a composed evals surface must wire limb (d)")
	}
	// Idempotent: a second boot changes nothing.
	again, err := buildBenchmarkSurface(ctx, db, log, reg, nil, nil, nil, evalStore, nil, nil, testLogger())
	if err != nil {
		t.Fatalf("second boot: %v", err)
	}
	if again.Domains != bs.Domains {
		t.Errorf("re-seeding changed the domain count: %d → %d", bs.Domains, again.Domains)
	}
	// No pair was manufactured.
	var pairs int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM benchmark_pairs`).Scan(&pairs); err != nil {
		t.Fatal(err)
	}
	if pairs != 0 {
		t.Errorf("boot created %d benchmark pair(s) — accrual is a bring-up act, never a boot step", pairs)
	}
	// And no user was opted in: it is an opt-IN and the default is OFF.
	var optedIn int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE benchmark_opt_in = 1`).Scan(&optedIn); err != nil {
		t.Fatal(err)
	}
	if optedIn != 0 {
		t.Errorf("boot opted %d user(s) in — the standing opt-in defaults OFF (BENCH-REG §4.2.1)", optedIn)
	}
}

// TestBenchmarkLoopIntervalIsATickNotACadence: the sampler's cursor is durable,
// so this interval decides only how promptly a card appears — never how often a
// task is considered. It is a structural constant; S18 ratifies no key.
func TestBenchmarkLoopIntervalIsATickNotACadence(t *testing.T) {
	if benchmarkLoopInterval <= 0 || benchmarkLoopInterval > 5*time.Minute {
		t.Errorf("benchmarkLoopInterval = %v; a tick should be brisk and cheap", benchmarkLoopInterval)
	}
}

// TestBriefSeamSourcesTheArtifactOfRecord (D1): BENCH-REG §3.1 requires both
// arms answer the IDENTICAL frozen statement, and the platform arm executed
// from the CONFIRMED specification — so the seam reads the S06 artifact of
// record, hash-verified, and never the arriving request.
func TestBriefSeamSourcesTheArtifactOfRecord(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	specPath := filepath.Join(dir, "SPEC-v3.md")
	const confirmed = "# SPEC v3\n\n## Restatement\n\nthe CONFIRMED goal, after two rounds of interview\n"
	if err := os.WriteFile(specPath, []byte(confirmed), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(confirmed))
	pin := hex.EncodeToString(sum[:])

	// The seam reads its input from the intake state, so drive the underlying
	// resolution directly against a state the pipeline would have written.
	brief, err := briefFromState(ctx, "t1", &intake.State{
		TaskID:  "t1",
		Req:     intake.Request{TaskID: "t1", Text: "the RAW ask, before any revision"},
		SpecRef: &intake.ArtifactRef{Path: specPath, SHA256: pin, Version: 3},
	})
	if err != nil {
		t.Fatalf("briefFromState: %v", err)
	}
	if brief.Statement != confirmed {
		t.Errorf("statement = %q, want the confirmed artifact of record — never the raw ask (BENCH-REG §3.1)", brief.Statement)
	}
	if strings.Contains(brief.Statement, "RAW ask") {
		t.Error("the seam handed the direct arm the arriving request; the platform arm executed from the CONFIRMED spec")
	}
	if !strings.Contains(brief.StatementSource, "SPEC-v3.md") || !strings.Contains(brief.StatementSource, pin) {
		t.Errorf("statement source = %q, want the artifact name pinned by content hash", brief.StatementSource)
	}
	if len(brief.Attachments) != 1 || brief.Attachments[0] != brief.StatementSource {
		t.Errorf("attachments = %v; at v0 intake has no separate attachment channel, so the artifact ref is what travels", brief.Attachments)
	}

	// A DRIFTED artifact of record fails loudly rather than feeding the direct
	// arm a statement the platform arm never answered.
	if err := os.WriteFile(specPath, []byte(confirmed+"\nsomething else\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := briefFromState(ctx, "t1", &intake.State{
		TaskID:  "t1",
		SpecRef: &intake.ArtifactRef{Path: specPath, SHA256: pin, Version: 3},
	}); err == nil {
		t.Error("a drifted artifact of record must be refused, not used")
	}

	// No confirmed specification at all: there is no frozen statement to put.
	if _, err := briefFromState(ctx, "t1", &intake.State{TaskID: "t1"}); err == nil {
		t.Error("a task with no confirmed specification has no §2 statement and must be refused")
	}
}

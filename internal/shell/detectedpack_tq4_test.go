package shell

// detectedpack_tq4_test.go — P3-TQ-4a drain r1 F1/F4 at the COMPOSITION ROOT:
// the check pack the platform ACTUALLY BUILDS for a task, over the real project
// store, a real capture, a real produced worktree and the real re-scan.
//
// What the packet's own battery could not prove: internal/verify's tests hand
// bootstrapV1 a pack they constructed themselves, so every claim about
// `detected:` ids, provenance, posture and per-slot precedence was a claim
// about a fixture. Setting `pack.Posture = ""` in the resolver — detected
// commands GRADUATING the project, the one thing A16 forbids — passed the whole
// battery. So did short-circuiting the re-scan so no project ever got a
// detected pack. These tests are the ones those mutations have to trip.
//
// The composition is workspacefork_test.go's: Onboard + Approve, an intake
// pipeline over the real registry seam, a task PINNED to the project, and
// WorkspaceCwd to create the task's own worktree — which is the produced tree
// the re-scan reads.

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// tq4Env is the pack resolver in its production composition.
type tq4Env struct {
	ctx  context.Context
	proj *project.Store
	ps   *projectSeams
	tree string
}

// newTQ4Env registers and activates a COMMANDLESS project, starts a task
// pinned to it, creates that task's worktree, and writes the TQ-F3 reference
// scaffold into it: a Vite/React app declaring build/test/dev and NO lint,
// with its dependencies never installed.
func newTQ4Env(t *testing.T) *tq4Env {
	t.Helper()
	ctx := context.Background()
	db, log, reg := seamDB(t)
	proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(t.TempDir(), "projects")})
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	if _, _, err := proj.Onboard(ctx, project.OnboardInput{ProjectID: "shop", Owner: "alice", Name: "shop"}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if _, err := proj.Approve(ctx, "shop", "alice", nil); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	runs := run.NewStore(db, log)
	pipe := &intake.Pipeline{
		DB: db, Log: log, Runs: runs, Ledger: ledger.NewStore(db, log), Settings: reg,
		ArtifactRoot: filepath.Join(t.TempDir(), "artifacts"),
		Registry:     registrySeam{proj: proj},
	}
	st, err := pipe.Start(ctx, intake.Request{
		TaskID: "t-shop", UserID: "alice", Title: "build the shop",
		Text: "scaffold the storefront", Project: "shop",
	})
	if err != nil {
		t.Fatalf("intake Start: %v", err)
	}
	if st.Registry == nil || st.Registry.Project != "shop" {
		t.Fatalf("intake state carries no project match: %+v", st.Registry)
	}
	ps := &projectSeams{proj: proj, runs: runs, db: db, pipe: pipe}
	if _, err := runs.Create(ctx, run.NewRun{ID: "t-shop.execute", UserID: "alice", TaskID: "t-shop"}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	tree, ok, err := ps.WorkspaceCwd(ctx, "t-shop.execute")
	if err != nil || !ok || tree == "" {
		t.Fatalf("WorkspaceCwd = %q, %v, %v — want the task worktree", tree, ok, err)
	}
	return &tq4Env{ctx: ctx, proj: proj, ps: ps, tree: tree}
}

// scaffold writes the produced tree the executor would have left behind.
func (e *tq4Env) scaffold(t *testing.T, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(e.tree, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

// webshopScaffold is the TQ-F3 reference tree, verbatim in shape.
func webshopScaffold() map[string]string {
	return map[string]string{
		"package.json": `{"name":"shop","private":true,"type":"module","scripts":{` +
			`"dev":"vite","build":"vite build","preview":"vite preview","test":"node --test tests"},` +
			`"devDependencies":{"vite":"^5.4.8"}}`,
		"index.html":            "<!doctype html><div id=root></div>",
		"tests/catalog.test.js": "import {test} from 'node:test'\n",
	}
}

// pack resolves the pack the platform builds for the task, the way the drain
// does.
func (e *tq4Env) pack(t *testing.T) *verify.CheckPack {
	t.Helper()
	p, err := e.ps.CheckPackFor(e.ctx, verify.DomainSoftware, "t-shop")
	if err != nil {
		t.Fatalf("CheckPackFor: %v", err)
	}
	if p == nil {
		t.Fatal("CheckPackFor resolved no pack for a registered software project")
	}
	return p
}

// checkIDs returns the pack's check ids, sorted, for a stable comparison.
func checkIDs(p *verify.CheckPack) []string {
	var ids []string
	for _, c := range p.Checks {
		ids = append(ids, c.ID)
	}
	sort.Strings(ids)
	return ids
}

func checkByID(t *testing.T, p *verify.CheckPack, id string) verify.Check {
	t.Helper()
	for _, c := range p.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no check %q in the resolved pack; have %v", id, checkIDs(p))
	panic("unreachable")
}

// TestTQ4DetectedPackIsBuiltAtTheCompositionRoot [F1]: a commandless project
// whose task produced a package.json resolves to a BOOTSTRAP pack carrying the
// detected rungs — built by the platform end to end, not by a fixture.
//
// Mutations this must trip: clearing Posture in the resolver's detected branch
// (detected commands graduating the project); clearing Provenance; and
// short-circuiting rescanDetected so no project ever gets a detected pack.
func TestTQ4DetectedPackIsBuiltAtTheCompositionRoot(t *testing.T) {
	e := newTQ4Env(t)
	e.scaffold(t, webshopScaffold())

	p := e.pack(t)

	if p.Posture != verify.PostureBootstrap {
		t.Fatalf("posture %q, want bootstrap — detected commands are evidence, never graduation (Spec S07.8 [A16])", p.Posture)
	}
	if p.Provenance != verify.ProvenanceDetected {
		t.Fatalf("pack provenance %q, want %q — every rung here came from the tree", p.Provenance, verify.ProvenanceDetected)
	}
	if got, want := checkIDs(p), []string{"detected:build", "detected:test"}; !sameStrings(got, want) {
		t.Fatalf("pack checks %v, want %v — the tree declares no lint script, and a rung the platform invented would fail for a reason that has nothing to do with the work", got, want)
	}
	for _, c := range p.Checks {
		if c.Origin != verify.ProvenanceDetected {
			t.Errorf("check %q origin %q, want %q — the per-rung fact every consumer branches on", c.ID, c.Origin, verify.ProvenanceDetected)
		}
		if c.ACKey != "" || c.StepID != "" {
			t.Errorf("check %q carries ACKey %q / StepID %q — a detected rung is evidence, never an acceptance verdict and never a PLAN contract", c.ID, c.ACKey, c.StepID)
		}
	}
	if got, want := checkByID(t, p, "detected:build").Argv, []string{"/bin/sh", "-lc", "npm run build"}; !sameStrings(got, want) {
		t.Fatalf("detected:build argv %v, want %v", got, want)
	}
	// Rule 7: the suite is exactly as fresh as the scan it came from.
	entry, err := e.proj.Get(e.ctx, "shop")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	capturedAt, err := time.Parse(time.RFC3339Nano, entry.Capture.CapturedTS)
	if err != nil {
		t.Fatalf("capture date %q: %v", entry.Capture.CapturedTS, err)
	}
	if !p.VerifiedOn.Equal(capturedAt) {
		t.Fatalf("pack verified-on %s, want the capture date %s", p.VerifiedOn, capturedAt)
	}
	if entry.Capture.Detected == nil || entry.Capture.Detected.Dev != "npm run dev" {
		t.Fatalf("the re-scan did not record the dev command on the capture: %+v", entry.Capture.Detected)
	}
}

// TestTQ4MixedPackKeepsOwnerRungsAndDetectedEvidence [F4]: A16's precedence is
// PER SLOT. An owner who captured only a lint command gets their lint rung AND
// the build and test rungs the platform detected — one pack, graduated on the
// owner's own bar, with the detected rungs still evidence.
//
// Mutation this must trip: composing the pack from Capture.Commands instead of
// project.EffectiveCommands (the detected rungs vanish and a one-rung graduated
// pack ships).
func TestTQ4MixedPackKeepsOwnerRungsAndDetectedEvidence(t *testing.T) {
	e := newTQ4Env(t)
	e.scaffold(t, webshopScaffold())
	// Resolve once so the re-scan records the detected set, then capture ONE
	// hand-typed command through the owner's own door.
	e.pack(t)
	if _, _, err := e.proj.EditCommands(e.ctx, "shop", "alice", project.Commands{Lint: "eslint ."}); err != nil {
		t.Fatalf("EditCommands: %v", err)
	}

	p := e.pack(t)

	if p.Posture != "" {
		t.Fatalf("posture %q, want the graduated posture — one hand-captured rung is a bar of the project's own (Spec S07.8)", p.Posture)
	}
	if p.Provenance != "" {
		t.Fatalf("pack provenance %q, want empty — a MIXED pack has no single answer, and Check.Origin is the per-rung fact", p.Provenance)
	}
	if got, want := checkIDs(p), []string{"detected:build", "detected:test", "lint"}; !sameStrings(got, want) {
		t.Fatalf("pack checks %v, want %v — a hand-captured command outranks a detected one PER SLOT, it does not withdraw the others", got, want)
	}
	lint := checkByID(t, p, "lint")
	if lint.Origin != "" {
		t.Errorf("lint origin %q, want empty — it is the owner's own command", lint.Origin)
	}
	if got, want := lint.Argv, []string{"/bin/sh", "-lc", "eslint ."}; !sameStrings(got, want) {
		t.Errorf("lint argv %v, want the owner's typed command %v", got, want)
	}
	for _, id := range []string{"detected:build", "detected:test"} {
		c := checkByID(t, p, id)
		if c.Origin != verify.ProvenanceDetected {
			t.Errorf("check %q origin %q, want %q — graduating on the owner's lint rung does not promote the platform's", id, c.Origin, verify.ProvenanceDetected)
		}
		if c.ACKey != "" || c.StepID != "" {
			t.Errorf("check %q carries ACKey %q / StepID %q — evidence in a mixed pack too", id, c.ACKey, c.StepID)
		}
	}
	// A graduated pack is a real suite and must satisfy the pack contract.
	if err := p.Validate(); err != nil {
		t.Fatalf("the mixed pack does not validate: %v", err)
	}
}

// TestTQ4OwnerCoveringEveryRungNeedsNoRescan [F1/F4]: when the owner's own
// commands already fill every ladder slot, a detected command could add no
// rung — so the re-scan is skipped and no capture version is minted for it.
func TestTQ4OwnerCoveringEveryRungNeedsNoRescan(t *testing.T) {
	e := newTQ4Env(t)
	e.scaffold(t, webshopScaffold())
	if _, _, err := e.proj.EditCommands(e.ctx, "shop", "alice", project.Commands{
		Lint: "eslint .", Build: "make build", Test: "make test",
	}); err != nil {
		t.Fatalf("EditCommands: %v", err)
	}
	before, err := e.proj.Get(e.ctx, "shop")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	p := e.pack(t)

	if got, want := checkIDs(p), []string{"build", "lint", "test"}; !sameStrings(got, want) {
		t.Fatalf("pack checks %v, want %v — every slot is the owner's", got, want)
	}
	for _, c := range p.Checks {
		if c.Origin != "" {
			t.Errorf("check %q origin %q, want empty", c.ID, c.Origin)
		}
	}
	after, err := e.proj.Get(e.ctx, "shop")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if after.CaptureVersion != before.CaptureVersion {
		t.Fatalf("capture version moved %d → %d — a re-scan that can add no rung must not run at all",
			before.CaptureVersion, after.CaptureVersion)
	}
}

// TestTQ4ProducedTreeWithNoManifestStaysTheHonestBootstrapPack [F1]: the
// platform invents nothing. A produced tree the scan finds no toolchain in
// resolves to the A14 landing — a bootstrap pack with no rungs at all.
func TestTQ4ProducedTreeWithNoManifestStaysTheHonestBootstrapPack(t *testing.T) {
	e := newTQ4Env(t)
	e.scaffold(t, map[string]string{"notes.txt": "nothing a toolchain would claim\n"})

	p := e.pack(t)

	if p.Posture != verify.PostureBootstrap {
		t.Fatalf("posture %q, want bootstrap", p.Posture)
	}
	if len(p.Checks) != 0 {
		t.Fatalf("pack carries %v — nothing in this tree declares a command, and the platform invents none", checkIDs(p))
	}
	if p.Provenance != "" {
		t.Fatalf("pack provenance %q, want empty — there is no detected rung to attribute", p.Provenance)
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

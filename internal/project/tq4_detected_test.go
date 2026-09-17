package project

// tq4_detected_test.go — P3-TQ-4a acceptance battery, capture half: the A16
// re-scan of a task's own produced tree (Spec S13.7 [A16, 2026-09-17]).
//
// The live defect these bind (TQ-F3): a task that created a project's whole
// build system was verified by two judge reads and nothing else, because the
// S13.7 registry capture could only be filled by hand.
//
// Committed RED at grounding (CONVENTIONS §3 amendment-A carve-out):
// RescanDetected is not built, and EffectiveCommands returns the hand-captured
// set alone.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// tq4Tree writes the TQ-F3 reference tree: the live webshop scaffold's shape,
// verbatim — a Vite/React app declaring build/test/dev and NO lint, with its
// dependencies never installed.
func tq4Tree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"package.json": `{"name":"shop","private":true,"type":"module","scripts":{` +
			`"dev":"vite","build":"vite build","preview":"vite preview","test":"node --test tests"},` +
			`"devDependencies":{"vite":"^5.4.8"}}`,
		"index.html":            "<!doctype html><div id=root></div>",
		"src/App.jsx":           "export default function App(){return null}",
		"tests/catalog.test.js": "import {test} from 'node:test'\n",
	}
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dir
}

// tq4Seed registers, captures and activates a project holding no command at
// all — the bootstrap shape the A16 re-scan exists for.
func tq4Seed(t *testing.T, f *fix, id, owner string) {
	t.Helper()
	gf5Seed(t, f, id, owner, CaptureInput{
		Conventions: []string{"Node project"},
		ScanHash:    "seed-scan-hash",
	})
}

// treeSnapshot records every path under root with its size and mtime, so
// "read-only on the tree" is a claim about the filesystem rather than about
// intent.
func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		out[rel] = fi.Mode().String() + "|" +
			fi.ModTime().Format("2006-01-02T15:04:05.000000000") + "|" +
			strconv.FormatInt(fi.Size(), 10)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

// TestTQ4RescanDetectsOnlyScriptsTheTreeDeclares [R2, R3]: the re-scan proposes
// a command only where the produced tree can actually run one. The onboarding
// scan proposes `npm run lint` from the package.json MARKER alone
// (scan.go toolchainRules) — for a human to edit at an approval card, which is
// fine. A DETECTED command is executed with nobody approving it, so the
// detected set is refined against the manifest: no `lint` script means no lint
// rung, and the `dev` script A16 names becomes its own slot.
func TestTQ4RescanDetectsOnlyScriptsTheTreeDeclares(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	tq4Seed(t, f, "p1", "u1")
	tree := tq4Tree(t)

	c, minted, err := f.store.RescanDetected(ctx, "p1", "u1", tree)
	if err != nil {
		t.Fatalf("RescanDetected: %v", err)
	}
	if !minted {
		t.Fatal("the first re-scan minted no capture version — it found commands where the capture had none")
	}
	if c.Detected == nil {
		t.Fatal("the capture carries no detected set — nil means no scan ever proposed anything, which is now false")
	}
	if got, want := c.Detected.Build, "npm run build"; got != want {
		t.Fatalf("detected build %q, want %q", got, want)
	}
	if got, want := c.Detected.Test, "npm test"; got != want {
		t.Fatalf("detected test %q, want %q", got, want)
	}
	if c.Detected.Lint != "" {
		t.Fatalf("detected lint %q — this tree declares no lint script, and running one the platform invented would fail for a reason that has nothing to do with the work (Spec S07.3 rule 1)", c.Detected.Lint)
	}
	if got, want := c.Detected.Dev, "npm run dev"; got != want {
		t.Fatalf("detected dev %q, want %q — A16 names build/test/lint/DEV, and the walk is served by the dev command", got, want)
	}
	// The owner's own set is untouched: this is a second member, never a
	// rewrite of what a person captured.
	if c.Commands != (Commands{}) {
		t.Fatalf("the re-scan wrote into the owner's command set: %+v", c.Commands)
	}
	// Everything else carries forward byte-equal (the Rescan discipline).
	if len(c.Conventions) != 1 || c.Conventions[0] != "Node project" {
		t.Fatalf("conventions did not carry forward byte-equal: %v", c.Conventions)
	}
	if c.ScanHash != "seed-scan-hash" {
		t.Fatalf("scan hash %q — dropping it would leave DriftCheck comparing against nothing", c.ScanHash)
	}
}

// TestTQ4HandCapturedOutranksDetectedPerSlot [R4]: "a hand-captured command
// always outranks a detected one" (Spec S13.7 [A16]) is a PER-SLOT rule, not a
// whole-set one — an owner who typed a test command has not thereby withdrawn
// the build command the platform found.
func TestTQ4HandCapturedOutranksDetectedPerSlot(t *testing.T) {
	detected := Commands{Build: "npm run build", Test: "npm test", Dev: "npm run dev"}
	c := Capture{
		Commands: Commands{Test: "make check"},
		Detected: &detected,
	}
	eff := EffectiveCommands(c)
	if eff.Test != "make check" {
		t.Fatalf("effective test %q, want the hand-captured %q", eff.Test, "make check")
	}
	if eff.Build != "npm run build" {
		t.Fatalf("effective build %q, want the detected %q — an unfilled slot falls through to what the platform found", eff.Build, "npm run build")
	}
	if eff.Dev != "npm run dev" {
		t.Fatalf("effective dev %q, want the detected %q", eff.Dev, "npm run dev")
	}
	// No detected set at all is an honest absence, never a panic and never an
	// invented command.
	if got := EffectiveCommands(Capture{Commands: Commands{Test: "make check"}}); got != (Commands{Test: "make check"}) {
		t.Fatalf("with no detected set the effective commands are %+v, want the hand-captured set alone", got)
	}
}

// TestTQ4RescanIsIdempotentAndReadOnly [R3]: the re-scan runs at the
// execute→verify boundary of EVERY bootstrap round, so a drain that re-enters
// the verify leg must not grow the capture history one version per round (the
// EditCommands retry-safety precedent). And the tree it reads is the work under
// review: reading a fact must not create one (§59).
func TestTQ4RescanIsIdempotentAndReadOnly(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	tq4Seed(t, f, "p1", "u1")
	tree := tq4Tree(t)
	before := treeSnapshot(t, tree)
	eventsBefore := len(gf5CapturedPayloads(t, f))

	first, minted, err := f.store.RescanDetected(ctx, "p1", "u1", tree)
	if err != nil {
		t.Fatalf("RescanDetected 1: %v", err)
	}
	if !minted {
		t.Fatal("the first re-scan minted nothing")
	}
	second, minted2, err := f.store.RescanDetected(ctx, "p1", "u1", tree)
	if err != nil {
		t.Fatalf("RescanDetected 2: %v", err)
	}
	if minted2 {
		t.Fatalf("the second re-scan of an unchanged tree minted version %d — an unchanged detected set is a no-op, or a drain grows the capture history one version per round", second.Version)
	}
	if second.Version != first.Version {
		t.Fatalf("version moved from %d to %d on an unchanged tree", first.Version, second.Version)
	}
	if got, want := len(gf5CapturedPayloads(t, f)), eventsBefore+1; got != want {
		t.Fatalf("%d capture events after two re-scans, want %d — the no-op appends none", got, want)
	}
	if after := treeSnapshot(t, tree); len(after) != len(before) {
		t.Fatalf("the re-scan changed the tree's file set: %d paths before, %d after", len(before), len(after))
	} else {
		for rel, sig := range before {
			if after[rel] != sig {
				t.Fatalf("the re-scan wrote to %s (%q → %q) — the scan is read-only on the work under review", rel, sig, after[rel])
			}
		}
	}
}

// TestTQ4EditCommandsCarriesDetectedForward [R5]: EditCommands carries every
// non-`commands` member forward byte-equal. The detected set is one of them —
// an owner typing a test command has not asked the platform to forget what it
// noticed, and losing it would silently re-empty the ladder on the next round.
func TestTQ4EditCommandsCarriesDetectedForward(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	tq4Seed(t, f, "p1", "u1")
	tree := tq4Tree(t)
	if _, _, err := f.store.RescanDetected(ctx, "p1", "u1", tree); err != nil {
		t.Fatalf("RescanDetected: %v", err)
	}

	after, minted, err := f.store.EditCommands(ctx, "p1", "u1", Commands{Test: "make check"})
	if err != nil {
		t.Fatalf("EditCommands: %v", err)
	}
	if !minted {
		t.Fatal("EditCommands minted no version for a new command set")
	}
	if after.Detected == nil {
		t.Fatal("the owner's edit dropped the detected set — it is carried forward byte-equal like conventions, zones, the scan hash and the family")
	}
	if after.Detected.Build != "npm run build" {
		t.Fatalf("carried-forward detected build %q, want %q", after.Detected.Build, "npm run build")
	}
	if eff := EffectiveCommands(after); eff.Test != "make check" || eff.Build != "npm run build" {
		t.Fatalf("effective commands after the edit = %+v; want the typed test and the detected build", eff)
	}
}

// TestTQ4RescanIsDistinguishableFromAnOwnerEditInTheAuditTrail [R3]: a person
// reading the trail must be able to tell "the platform noticed this" from
// "someone decided this". `origin` is the one member that says which, and it is
// additive and omitempty — so BOTH directions are the assertion.
func TestTQ4RescanIsDistinguishableFromAnOwnerEditInTheAuditTrail(t *testing.T) {
	ctx := context.Background()
	f := newFix(t)
	tq4Seed(t, f, "p1", "u1")
	tree := tq4Tree(t)
	if _, _, err := f.store.RescanDetected(ctx, "p1", "u1", tree); err != nil {
		t.Fatalf("RescanDetected: %v", err)
	}
	if _, _, err := f.store.EditCommands(ctx, "p1", "u1", Commands{Test: "make check"}); err != nil {
		t.Fatalf("EditCommands: %v", err)
	}

	var origins []string
	for _, p := range gf5CapturedPayloads(t, f) {
		var payload struct {
			Origin string `json:"origin"`
		}
		if err := json.Unmarshal([]byte(p), &payload); err != nil {
			t.Fatalf("decode capture payload %s: %v", p, err)
		}
		origins = append(origins, payload.Origin)
	}
	if len(origins) != 3 {
		t.Fatalf("%d capture events, want 3 (seed, re-scan, edit): %v", len(origins), origins)
	}
	if origins[0] != "" {
		t.Fatalf("the seed capture carries origin %q — the key stays ABSENT on every pre-existing producer", origins[0])
	}
	if origins[1] != OriginDetected {
		t.Fatalf("the re-scan's capture carries origin %q, want %q", origins[1], OriginDetected)
	}
	if origins[2] != OriginEdit {
		t.Fatalf("the owner's edit carries origin %q, want %q", origins[2], OriginEdit)
	}
}

package project

// detected.go — the re-scan of a task's own produced tree [A16, 2026-09-17].
//
// Spec S13.7: "At the execute→verify boundary of a bootstrap-posture round
// (S07.8) the platform rescans the task's own produced tree with the same
// heuristics and enters detected build/test/lint/dev commands into the capture
// with provenance `detected` — requester-visible and editable through the
// Commands door; a hand-captured command always outranks a detected one."
//
// Three properties bind it: the scan is READ-ONLY on the tree (Store.Scan
// already is — it stats and reads, never writes); the write is IDEMPOTENT (a
// detected set byte-equal to the current one mints no version and emits no
// event — the EditCommands retry-safety precedent); and a hand-captured
// command in the same slot is never displaced, because the detected set lives
// in its own member and precedence is resolved at read time.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
)

// OriginDetected labels a capture version minted by the A16 re-scan in the
// audit trail (the CaptureInput.Origin axis, alongside OriginEdit). Capture
// versions are otherwise indistinguishable from an owner edit in the
// registry.captured event, and a person reading the trail should be able to
// tell "the platform noticed this" from "someone decided this".
const OriginDetected = "detected"

// storedCommands is the shape of a capture's `commands` JSON column: the
// owner's set, plus the platform's detected set beside it [A16].
//
// One column and no migration, because the column is already JSON and every
// member is omitempty: a capture with no detected set marshals to exactly the
// bytes it marshalled to before this packet, and every row written before it
// decodes with Detected nil. The two sets are never merged — merging would
// make the precedence rule unrecoverable after one write, and would let a
// detected command graduate the project through packChecks.
type storedCommands struct {
	Commands
	Detected *Commands `json:"detected,omitempty"`
}

// EffectiveCommands resolves the commands a consumer should use for one
// capture: per slot, the hand-captured command when it is non-blank, otherwise
// the detected one. This is the whole of the A16 precedence rule, in one place,
// so no consumer re-derives it.
//
// PER SLOT, not per set: an owner who typed a test command has not thereby
// withdrawn the build command the platform found.
func EffectiveCommands(c Capture) Commands {
	if c.Detected == nil {
		return c.Commands
	}
	out := c.Commands
	for _, slot := range commandSlots {
		if slot.get(out) == "" {
			slot.set(&out, slot.get(*c.Detected))
		}
	}
	return out
}

// RescanDetected re-scans tree with the S13.7 onboarding heuristics and records
// the result as this project's detected command set, minting a new immutable
// capture version whose every other member (conventions, commands, danger
// zones, scan hash, family) is carried forward byte-equal.
//
// minted reports whether a new version was written: a detected set byte-equal
// to the current one is a no-op, so a drain that re-enters the verify leg does
// not grow the capture history one version per round.
//
// tree is the task's own produced tree at the execute→verify boundary. Nothing
// here writes to it and `.git` is never inspected (Store.Scan's own contract),
// so a live worktree is as safe a subject as the §59 stripped copy.
func (s *Store) RescanDetected(ctx context.Context, projectID, by, tree string) (Capture, bool, error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return Capture{}, false, err
	}
	draft, err := s.Scan(tree, e.DefaultBranch)
	if err != nil {
		return Capture{}, false, err
	}
	detected, err := validCommands(refineDetected(tree, draft.Commands))
	if err != nil {
		return Capture{}, false, err
	}
	if e.Capture.Detected != nil && *e.Capture.Detected == detected {
		return e.Capture, false, nil
	}
	c, err := s.Capture(ctx, CaptureInput{
		ProjectID:   projectID,
		By:          by,
		Conventions: e.Capture.Conventions,
		Commands:    e.Capture.Commands,
		DangerZones: e.Capture.DangerZones,
		ScanHash:    e.Capture.ScanHash,
		Family:      e.Capture.Family,
		Detected:    &detected,
		Origin:      OriginDetected,
	})
	if err != nil {
		return Capture{}, false, err
	}
	return c, true, nil
}

// refineDetected narrows Store.Scan's toolchain proposal to what the tree can
// actually run, and adds the dev slot A16 names.
//
// Why the onboarding scan is not enough on its own, and why it is nonetheless
// left byte-unchanged: Scan proposes from the MARKER FILE alone — a
// package.json earns `npm run lint` whether or not a lint script exists —
// which is right for a draft a person approves at the onboarding card, and
// wrong for a command the platform executes with nobody approving it. Changing
// Scan instead would move every registered project's scanHash drift baseline
// (scan.go) and rewrite drafts a human already approved, so the refinement
// lives here.
//
// Only the package.json toolchain is refined, because it is the only one whose
// commands run a manifest-declared script: `go build ./...` needs the host
// toolchain and nothing else. An honest cut, not a gap.
func refineDetected(tree string, proposed Commands) Commands {
	if firstToolchainMarker(tree) != "package.json" {
		return proposed
	}
	scripts := packageScripts(tree)
	var out Commands
	// The package.json script each slot's proposed command runs. `run` is the
	// odd one: Scan proposes `npm start`/`pnpm start`, whose script is
	// "start".
	for _, slot := range []struct {
		script string
		get    func(Commands) string
		set    func(*Commands, string)
	}{
		{"build", func(c Commands) string { return c.Build }, func(c *Commands, v string) { c.Build = v }},
		{"test", func(c Commands) string { return c.Test }, func(c *Commands, v string) { c.Test = v }},
		{"lint", func(c Commands) string { return c.Lint }, func(c *Commands, v string) { c.Lint = v }},
		{"start", func(c Commands) string { return c.Run }, func(c *Commands, v string) { c.Run = v }},
		{"preview", func(c Commands) string { return c.Preview }, func(c *Commands, v string) { c.Preview = v }},
	} {
		if scripts[slot.script] {
			slot.set(&out, slot.get(proposed))
		}
	}
	// Dev has no proposal to narrow — Scan has no dev slot — so it is read
	// straight off the manifest in the runner style Scan itself chose (pnpm
	// runs a script by name; npm needs the `run` verb).
	if scripts["dev"] {
		out.Dev = "npm run dev"
		if _, ok := marker(tree, "pnpm-lock.yaml"); ok {
			out.Dev = "pnpm dev"
		}
	}
	return out
}

// packageScripts reads the set of script names a tree's package.json declares.
// An absent or unreadable manifest yields an empty set, which drops every npm
// slot: a command whose backing script the platform cannot confirm would fail
// for a reason that has nothing to do with the work (Spec S07.3 rule 1).
func packageScripts(tree string) map[string]bool {
	raw, err := os.ReadFile(filepath.Join(tree, "package.json"))
	if err != nil {
		return nil
	}
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil
	}
	out := make(map[string]bool, len(manifest.Scripts))
	for name := range manifest.Scripts {
		out[name] = true
	}
	return out
}

// firstToolchainMarker names the marker file Store.Scan's own first-match rule
// selected for tree, so the refinement above narrows the same toolchain the
// proposal came from. A tree holding both go.mod and package.json is a Go
// project with a helper manifest, and its go commands are not npm scripts.
func firstToolchainMarker(tree string) string {
	for _, r := range toolchainRules {
		if _, ok := marker(tree, r.marker); ok {
			return r.marker
		}
	}
	return ""
}

// marker reports whether one marker file exists in tree.
func marker(tree, rel string) (string, bool) {
	p := filepath.Join(tree, rel)
	if _, err := os.Stat(p); err == nil {
		return p, true
	}
	return "", false
}

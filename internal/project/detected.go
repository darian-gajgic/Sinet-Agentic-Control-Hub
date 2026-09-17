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

import "context"

// OriginDetected labels a capture version minted by the A16 re-scan in the
// audit trail (the CaptureInput.Origin axis, alongside OriginEdit). Capture
// versions are otherwise indistinguishable from an owner edit in the
// registry.captured event, and a person reading the trail should be able to
// tell "the platform noticed this" from "someone decided this".
const OriginDetected = "detected"

// EffectiveCommands resolves the commands a consumer should use for one
// capture: per slot, the hand-captured command when it is non-blank, otherwise
// the detected one. This is the whole of the A16 precedence rule, in one place,
// so no consumer re-derives it.
func EffectiveCommands(c Capture) Commands {
	// P3-TQ-4a: the packet's work. Returning the hand-captured set alone is
	// today's behavior and is what makes the acceptance tests red.
	return c.Commands
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
// tree is the verification workspace — the produced revision with its VCS
// history stripped (§59). Nothing here writes to it.
func (s *Store) RescanDetected(ctx context.Context, projectID, by, tree string) (Capture, bool, error) {
	// P3-TQ-4a: the packet's work.
	_, _, _, _ = ctx, projectID, by, tree
	return Capture{}, false, errRescanDetectedNotBuilt
}

// errRescanDetectedNotBuilt is the red-state sentinel; it disappears with the
// implementation.
var errRescanDetectedNotBuilt = errNotBuilt("project: RescanDetected is not built yet (P3-TQ-4a)")

type errNotBuilt string

func (e errNotBuilt) Error() string { return string(e) }

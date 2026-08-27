package shell

// checkpack_gf4_test.go — P3-GF4 (Spec S07.8 bootstrap posture, A14
// 2026-08-27): a launch-domain deliverable whose REGISTERED project has no
// captured build/test/lint commands "is NEVER a verification refusal and never
// parks the run". These are the packet's committed acceptance tests
// (CONVENTIONS §3 Amendment-A red window): they FAIL against the pre-GF4
// resolver, whose only honest answer was ErrNoCheckPack — the wall the
// operator hit twice (P3/design/b6-gate-operator-findings-r4-2026-08-23.md
// §F1a).
//
// Deliberately encoding-agnostic: the brief leaves the seam's bootstrap
// answer shape to the executor, so these tests assert only what A14 fixes —
// no refusal error, and no invented executable inventory.

import (
	"errors"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// TestGF4CommandlessProjectIsNeverARefusal: the fresh-scaffold case verbatim
// from A14 — a registered project whose capture holds NO build, test or lint
// command resolves to the bootstrap posture, not to an ErrNoCheckPack refusal
// that becomes an unanswerable parked card (S07.8: "NEVER a verification
// refusal"; refusal terminals remain only for S07.7 integrity cases).
func TestGF4CommandlessProjectIsNeverARefusal(t *testing.T) {
	e := registeredProject(t, project.Commands{})
	pack, err := packFromCapture(verify.DomainSoftware, e)
	if errors.Is(err, verify.ErrNoCheckPack) {
		t.Fatalf("command-less registered project still refuses with ErrNoCheckPack: %v — A14 (Spec S07.8 bootstrap bullet) makes this the bootstrap posture, never a refusal", err)
	}
	if err != nil {
		t.Fatalf("command-less registered project errors: %v — the bootstrap posture is an answer, not an error", err)
	}
	if pack != nil && len(pack.Checks) > 0 {
		t.Fatalf("command-less project produced executable checks %+v — an invented inventory; bootstrap runs NO executable rung (they record UNVERIFIABLE-HERE)", pack.Checks)
	}
}

// TestGF4RunPreviewOnlyCaptureIsStillBootstrap: run/preview commands are
// previews, not verdicts (they start something and wait — S13.8), so a capture
// holding ONLY them is still the command-less case: S07.8's bootstrap
// condition is on build/test/lint specifically ("no captured build/test/lint
// commands", A14).
func TestGF4RunPreviewOnlyCaptureIsStillBootstrap(t *testing.T) {
	e := registeredProject(t, project.Commands{Run: "flask run", Preview: "flask run --port 5000"})
	pack, err := packFromCapture(verify.DomainSoftware, e)
	if errors.Is(err, verify.ErrNoCheckPack) {
		t.Fatalf("run/preview-only capture still refuses with ErrNoCheckPack: %v — no executable rung exists, so this is the bootstrap posture (Spec S07.8, A14)", err)
	}
	if err != nil {
		t.Fatalf("run/preview-only capture errors: %v", err)
	}
	if pack != nil && len(pack.Checks) > 0 {
		t.Fatalf("run/preview-only capture produced executable checks %+v — run/preview are previews (S13.8), never rungs", pack.Checks)
	}
}

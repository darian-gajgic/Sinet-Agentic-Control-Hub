package verify_test

// tq4d_posture_note_test.go — P3-TQ-4d: the bootstrap posture disclosure says
// what a bootstrap round actually does (Spec S07.8 [A16]). The platform runs
// the commands it detects in the produced tree as evidence rungs, and those can
// pass or fail, so the disclosure may not claim that every check rung is
// unverifiable. It names each lane in plain words instead.

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// TestTQ4dBootstrapPostureNoteNamesEachLane pins the disclosure's claims, one
// per lane, and the absence of the claim A16 made false.
func TestTQ4dBootstrapPostureNoteNamesEachLane(t *testing.T) {
	note := verify.BootstrapPostureNote
	for _, claim := range []struct{ lane, fragment string }{
		{"the project's own checks could not run", "the project's own checks are recorded as unverifiable here"},
		{"what the platform ran is evidence and decides nothing", "anything the platform ran from the project's files is evidence only"},
		{"the requester's review decides (V3 is mandatory)", "your review is what decides this work"},
	} {
		if !strings.Contains(note, claim.fragment) {
			t.Errorf("the posture disclosure no longer says %s: want %q in %q", claim.lane, claim.fragment, note)
		}
	}
	if strings.Contains(note, "every check rung is recorded as unverifiable") {
		t.Errorf("the posture disclosure claims every check rung is unverifiable, but a bootstrap round runs the detected commands and they can pass or fail (Spec S07.8 [A16]): %q", note)
	}
}

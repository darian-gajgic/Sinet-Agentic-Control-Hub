package kimicli

// stall_test.go — P3-LN-7 cap residual R1, coordinator-inline (post-cap rule).
//
// The evaluator's PROBES 11/12: after the ambiguity legs went loud, the
// ZERO-match and open-failure legs were still quiet forever — a named session
// whose store never appears (or vanishes) stalled billing with no warning and
// no refusal flag. Both tests are shaped after the probes, with the grace
// window asserted in both directions so "refuse everything" cannot pass.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIndexNamedSessionThatNeverAppearsRefusesLoudly is PROBE 11: the index
// names session_real, but its directory is gone before the tail ever resolves
// it. Within the grace the quiet poll is the benign startup reading; past it,
// the tail must refuse loudly rather than under-bill in silence.
func TestIndexNamedSessionThatNeverAppearsRefusesLoudly(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	s := tailSessionAt(t, home, cwd, "session_real", oneRealCall, true)
	if err := os.RemoveAll(filepath.Join(home, "sessions")); err != nil {
		t.Fatalf("unlink sessions: %v", err)
	}

	s.drainUsage()
	if s.wireRefused {
		t.Fatal("refused on the FIRST poll — the benign startup reading is gone and every real spawn would refuse")
	}
	for i := 0; i < transcriptStallGracePolls+2; i++ {
		s.drainUsage()
	}
	if !s.wireRefused {
		t.Error("a named session whose store never appeared was polled past the grace without refusing — " +
			"a silent billing stall the run's own work can trigger")
	}
	if evs := drainEvents(s); len(evs) != 0 {
		t.Errorf("%d Usage events from a store that never existed, want none", len(evs))
	}
}

// TestPinnedTranscriptThatVanishesRefusesLoudly is PROBE 12: the store was
// pinned and billed from, then deleted mid-run. The open failure must escalate
// exactly like the never-appeared case, not return quietly forever.
func TestPinnedTranscriptThatVanishesRefusesLoudly(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	s := tailSessionAt(t, home, cwd, "session_real", oneRealCall, true)

	s.drainUsage()
	if evs := drainEvents(s); len(evs) != 1 {
		t.Fatalf("%d Usage events before the deletion, want 1 — the control leg must bill normally", len(evs))
	}
	if s.wirePath == "" {
		t.Fatal("the transcript was never pinned; the deletion leg below would prove nothing")
	}
	if err := os.RemoveAll(filepath.Join(home, "sessions")); err != nil {
		t.Fatalf("unlink sessions: %v", err)
	}

	s.drainUsage()
	if s.wireRefused {
		t.Fatal("refused on the first failed open — a transient blip inside the grace must stay quiet")
	}
	for i := 0; i < transcriptStallGracePolls+2; i++ {
		s.drainUsage()
	}
	if !s.wireRefused {
		t.Error("a pinned transcript that became unopenable was polled past the grace without refusing — " +
			"silent under-billing in the suppression direction")
	}
	if evs := drainEvents(s); len(evs) != 0 {
		t.Errorf("%d Usage events after the store vanished, want none", len(evs))
	}
}

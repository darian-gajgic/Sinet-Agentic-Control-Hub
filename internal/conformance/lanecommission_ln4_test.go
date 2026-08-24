package conformance_test

// lanecommission_ln4_test.go — P3-LN-4 / R13 (S18, CONVENTIONS §64 precedent).
//
// Commissioning has no number in it: its inputs are the state dir, the shipped
// lane documents and the contents of a broker store. So the ⚙ tally must be
// exactly what it was, and this asserts that against the spec TEXT rather than
// assuming it — the §64 precedent, because a tally nobody re-reads is a tally
// that moved.

import (
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
)

func TestLN4SettingsTallyIsUnmoved(t *testing.T) {
	reg := settings.New()
	decls := reg.Decls()
	if len(decls) != 118 {
		t.Errorf("⚙ index has %d keys, want 118 — P3-LN-4 consumes no setting and adds none (R13)", len(decls))
	}
	domains := map[string]bool{}
	for _, d := range decls {
		domains[d.Domain()] = true
	}
	if len(domains) != 33 {
		t.Errorf("⚙ index spans %d domains, want 33", len(domains))
	}

	s18 := readSpec(t, "Spec/drafts/S18-settings-registry.md")
	if !strings.Contains(s18, "**118 dotted registry keys** across **33 domains**") {
		t.Error("the S18 tally prose moved — this packet declares no ⚙ key and must not touch it. A lane " +
			"allow-list, a commission-nothing switch or a store-root override would each be an S00.9 " +
			"amendment adding an S18 row, never a constant minted in the composition root.")
	}
	if !strings.Contains(s18, "**3 data-valued settings surfaces**") {
		t.Error("the S18.3 data-surface count moved — filling the commissioned map reads the EXISTING lane " +
			"documents and the broker store; it adds no surface")
	}
}

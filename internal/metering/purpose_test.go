package metering

import "testing"

// derivePurpose (B2-4): purpose derives from the walking-skeleton run role
// so ceremony/verification itemize separately from execution on receipts
// (Spec S06.10/S07.11) — still one derivation site, per the B1-2 shape.
func TestDerivePurposeByRunRole(t *testing.T) {
	cases := []struct {
		runID string
		want  PurposeTag
	}{
		{"t-abc.intake", PurposeCeremony},
		{"t-abc.verify", PurposeVerification},
		{"t-abc.execute", PurposeExecution},
		{"t-abc.compose", PurposeCeremony},    // S08.6 composer = ceremony (B3-5)
		{"t-abc.compose.g2", PurposeCeremony}, // fork successor keeps it
		{"r-plain", PurposeExecution},         // B1-2-era shape: a run is execution
		{"t-abc.intake.g2", PurposeCeremony},  // recovery-fork successor keeps its role (S02.5 `<parent>.g<n>`)
		{"t-abc.verify.g2.g3", PurposeVerification},
		{"t-abc.gadget", PurposeExecution}, // `.g` followed by non-digits is not a fork suffix
		{"t-abc" + RunSuffixVerify, PurposeVerification},
	}
	for _, c := range cases {
		if got := derivePurpose(c.runID, checkpointRow{}); got != c.want {
			t.Errorf("derivePurpose(%q) = %s, want %s", c.runID, got, c.want)
		}
	}
}

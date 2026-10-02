package stage

import (
	"strings"
	"testing"
)

// tq8_prompt_internal_test.go — P3-TQ-8: the axis-1 instruction must name
// the QUOTABLE items (verify/artifact, verify/diff) as where an extractive
// evidence quote comes from and the executor's report item
// (verify/executor-report) as claims that never count — so the judge is
// told the same rule ValidateAxis1 enforces (Spec S07.5/S07.10). The block
// ids are the brief's own headers (ledger.BriefText), the one stable spelling.
func TestTQ8ComplianceInstructionsNameTheQuotableItemsAndTheReport(t *testing.T) {
	for _, want := range []string{"verify/artifact", "verify/diff", "verify/executor-report"} {
		if !strings.Contains(axis1Schema, want) {
			t.Errorf("axis-1 instructions do not name %q:\n%s", want, axis1Schema)
		}
	}
}

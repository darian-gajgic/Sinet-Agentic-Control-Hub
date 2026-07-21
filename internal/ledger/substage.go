package ledger

import (
	"strconv"
	"strings"
)

// Sub-stage identity (Spec S05.3). An accepted overflow stage-split turns
// one PLANNED stage into successor sub-stages: consolidate-to-ledger
// (stage-close gate) → end session → successor stage brief from the updated
// ledger. The successor is a fresh brief of the SAME planned stage — its
// name derives from the planned stage so deterministic selection (the plan
// step contract, Spec S05.4) keys on the planned identity while work dirs,
// manifests, and decisions record the concrete sub-stage. The ledger owns
// stage-brief identity, so the naming convention lives here.

// subStageSep separates the planned-stage name from the sub-stage ordinal.
const subStageSep = "#"

// SubStageName names sub-stage k (k ≥ 2) of a planned stage; k ≤ 1 is the
// planned stage itself (the first session of a stage is not a sub-stage).
func SubStageName(planned string, k int) string {
	if k <= 1 {
		return planned
	}
	return planned + subStageSep + strconv.Itoa(k)
}

// PlannedStage returns the planned-stage name a (possibly sub-) stage name
// derives from: a trailing "#<digits>" ordinal is stripped, anything else
// is returned unchanged (conservative — a plan step id is never rewritten).
func PlannedStage(stage string) string {
	i := strings.LastIndex(stage, subStageSep)
	if i <= 0 || i == len(stage)-1 {
		return stage
	}
	for _, r := range stage[i+1:] {
		if r < '0' || r > '9' {
			return stage
		}
	}
	return stage[:i]
}

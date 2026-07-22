package local

import "context"

// recheck.go — the S12.5 low-margin re-check consumer (brief R4). Where a duty
// has a LIVE caller (intake-triage: "fast; workhorse re-check on low margin",
// S12.4), a below-threshold fast-tier answer re-checks on the workhorse ($0,
// local→local) when a calibrated threshold exists for the (duty, fast-seat
// model hash, engine build) key. Uncalibrated ⇒ NO re-check gate (the duty's
// ungated v0 behaviour continues — the honest-uncalibrated posture, R3). The
// B5 consumers (watchdog-disambiguator → verdict IS `unclear` on low margin;
// watchlist re-check; intent-filling "which did you mean?" card) are seams —
// their margin plumbing exists (LabelMargin) but no consumer is faked here (R4).

// WorkhorseSeatKey is the manifest seat key the low-margin re-check escalates
// to (the bigger local model, $0). Structural — the workhorse is the alias
// map's default target.
const WorkhorseSeatKey = "workhorse"

// ReChecker performs the R4 margin-gated workhorse re-check. Nil-safe: a nil
// ReChecker (no calibration store wired — the uncalibrated dev posture) never
// re-checks and the fast-tier answer stands.
type ReChecker struct {
	duty        *Duty
	store       *CalStore
	engineBuild string
}

// NewReChecker wires the re-check consumer over the duty caller + calibration
// store. A nil store disables re-checking (uncalibrated posture).
func NewReChecker(duty *Duty, store *CalStore) *ReChecker {
	return &ReChecker{duty: duty, store: store, engineBuild: LlamaCppPin}
}

// RecheckResult reports a re-check outcome.
type RecheckResult struct {
	// Rechecked is true when the workhorse re-check ran (margin below the
	// calibrated threshold). False ⇒ the fast-tier answer stands (accepted or
	// uncalibrated).
	Rechecked bool
	// Margin is the fast-tier label margin (for observability).
	Margin float64
	// HadMargin reports whether a margin could be computed at all.
	HadMargin bool
	// Result is the final DutyResult — the workhorse's when Rechecked, else the
	// fast-tier's (passed through).
	Result DutyResult
}

// MarginRecheck applies the R4 gate: compute the fast-tier label margin, look
// up the calibration for (duty, fast-seat model hash, engine build); if
// calibrated AND the margin is below the threshold, re-run the SAME request on
// the workhorse seat and return that. Otherwise the fast-tier result stands.
// duty is the duty-alias label; in is the original request; first is the
// completed fast-tier result; labelFields are the routing-decisive labels the
// margin is taken over.
func (r *ReChecker) MarginRecheck(ctx context.Context, runID, duty string, in DutyRequest, first DutyResult, labelFields []string) (RecheckResult, error) {
	out := RecheckResult{Result: first}
	if r == nil || r.store == nil || r.duty == nil {
		return out, nil // uncalibrated posture: no re-check gate (R3/R4)
	}
	margin, ok := LabelMargin(first.Logprobs, labelFields)
	out.Margin, out.HadMargin = margin, ok
	if !ok {
		return out, nil // no margin ⇒ nothing to gate on (honest)
	}
	key := CalibrationKey{Duty: normalizeAlias(duty), ModelHash: first.Seat.ModelHash(), EngineBuild: r.engineBuild}
	cal, found, err := r.store.Calibration(ctx, key)
	if err != nil {
		return out, err
	}
	if !found || cal.AcceptLocal(margin) {
		return out, nil // uncalibrated, or margin ≥ threshold ⇒ accept the fast answer
	}
	// Below threshold: re-check on the workhorse ($0, local→local, R4).
	seat, exists := SeatByKey(WorkhorseSeatKey)
	if !exists || !seat.Servable {
		return out, nil // no workhorse seat available ⇒ the fast answer stands (honest)
	}
	res, err := r.duty.CallSeat(ctx, runID, duty, seat, in)
	if err != nil {
		// The re-check failed; the fast-tier answer stands (advisory escalation,
		// never a hard failure of the classification).
		return out, nil
	}
	out.Rechecked = true
	out.Result = res
	return out, nil
}

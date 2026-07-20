package intake

import (
	"context"
	"fmt"
	"strings"
)

// Stage 2 — the deterministic spine (Spec S06.7): platform code, no model.
// Runs after every Stage-1 emission and after every revision.

// SpineResult is one spine pass over the current pair.
type SpineResult struct {
	// (a) Coverage check (1.4): uncovered AC keys, deterministic on stable
	// keys. AdvisoryNotes carry the optional local-model semantic
	// spot-check — advisory-only, never a gate.
	Uncovered     []string `json:"uncovered,omitempty"`
	AdvisoryNotes []string `json:"advisory_notes,omitempty"`

	// (b) Floor re-check against the plan's contents.
	FloorAdds []FloorReason `json:"floor_adds,omitempty"`
	TierAfter Tier          `json:"tier_after"`
	BandAfter bool          `json:"band_after"`

	// (c) Size-delta rule (2.5): "over" (plan > guess × ⚙ factor), "under"
	// (plan ≪ guess), "incomparable" (either side unpriced), or "".
	SizeFinding string `json:"size_finding,omitempty"`
	SizeDetail  string `json:"size_detail,omitempty"`

	// (d) Research-node presence: open data-bearing rule ids without a
	// research node in the plan.
	MissingResearch []string `json:"missing_research,omitempty"`

	// Open NEEDS-CLARIFICATION markers (S06.6: cannot reach approval).
	OpenMarkers []string `json:"open_markers,omitempty"`
}

// coverageCheck is spine (a): every AC key maps to at least one owning
// step. Drop-detection is deterministic on stable keys (Spec S06.7(a)).
func coverageCheck(pair *Pair, accepted []string) []string {
	acceptedSet := make(map[string]bool, len(accepted))
	for _, k := range accepted {
		acceptedSet[k] = true
	}
	var uncovered []string
	for _, ac := range pair.Spec.ACs {
		if acceptedSet[ac.Key()] {
			continue // requester explicitly accepted the gap on a decision card
		}
		if len(pair.Plan.Coverage[ac.Key()]) == 0 {
			uncovered = append(uncovered, ac.Key())
		}
	}
	return uncovered
}

// floorRecheck is spine (b): if the PLAN introduces outward effects,
// spend, credential touches, shared-asset writes, or new tools the request
// didn't mention, the tier rises now (Spec S06.7(b)).
func floorRecheck(plan *Plan) []FloorReason {
	var adds []FloorReason
	add := func(class, detail string) {
		adds = append(adds, FloorReason{Class: class, Source: "plan", Detail: detail})
	}
	for _, st := range plan.Steps {
		for _, e := range st.OutwardEffects {
			add(FloorOutwardEffect, st.ID+": "+e)
		}
		if st.NewSpend {
			add(FloorNewSpend, st.ID)
		}
		if st.CredentialTouch {
			add(FloorCredentialTouch, st.ID)
		}
		if st.SharedAssetWrite {
			add(FloorSharedAssetWrite, st.ID)
		}
	}
	return adds
}

// bandRecheck re-verifies the four zero-interaction conditions
// deterministically from the PLAN's actual contents (Spec S06.4). The
// cost condition was decided at Stage 0 per-user; here the plan's own
// estimate re-tests it.
func (p *Pipeline) bandRecheck(st *State, plan *Plan) (ok bool, why string) {
	globs, unbounded := plan.WriteGlobs()
	if len(globs) > 0 || unbounded {
		return false, "plan declares writes outside the run workspace"
	}
	for _, s := range plan.Steps {
		if len(s.OutwardEffects) > 0 {
			return false, "plan expects outward-effect proposals"
		}
		if len(s.NewTools) > 0 {
			return false, "plan needs new tools/workers/grants (S06.4 condition 4)"
		}
		if s.NewSpend || s.CredentialTouch || s.SharedAssetWrite {
			return false, "plan trips a deterministic floor class"
		}
	}
	if len(st.openDataBearing()) > 0 {
		return false, "data-bearing flag set (S06.3)"
	}
	capUSD, err := p.Settings.FloatFor(keyZeroInteractionCost, st.Owner)
	if err != nil {
		return false, "⚙ " + keyZeroInteractionCost + " unreadable (conservative)"
	}
	if !plan.Est.Known {
		return false, "estimate unpriced (conservative)"
	}
	if plan.Est.USD >= capUSD {
		return false, fmt.Sprintf("estimated %.2f USD ≥ ⚙ %s", plan.Est.USD, keyZeroInteractionCost)
	}
	return true, ""
}

// sizeDelta is spine (c): plan-derived estimate vs the Stage-0 guess.
func sizeDelta(guess, plan Estimate, factor float64) (finding, detail string) {
	if !guess.Known || !plan.Known {
		return "incomparable", "size classification cannot be compared numerically (unpriced side) — surfaced, never silent (2.5)"
	}
	switch {
	case plan.USD > guess.USD*factor:
		return "over", fmt.Sprintf("plan ~%.2f USD > guess ~%.2f USD × ⚙ %s %.1f — size reclassified before expensive work", plan.USD, guess.USD, keySizeRecheckFactor, factor)
	case guess.USD > 0 && plan.USD*factor < guess.USD:
		return "under", fmt.Sprintf("plan ~%.2f USD ≪ guess ~%.2f USD — noted on the approval card", plan.USD, guess.USD)
	}
	return "", ""
}

// researchCheck is spine (d): every open data-bearing flag needs a
// research node in the PLAN — present by policy, never model initiative
// (Spec S06.6, 1.9/P47).
func researchCheck(plan *Plan, open []TriggerHit) []string {
	nodes := make(map[string]bool, len(plan.ResearchNodes))
	for _, rn := range plan.ResearchNodes {
		nodes[rn.RuleID] = true
	}
	var missing []string
	for _, h := range open {
		if !nodes[h.RuleID] {
			missing = append(missing, h.RuleID)
		}
	}
	return missing
}

// runSpine executes the full Stage-2 pass over the current pair and
// applies its monotone consequences to the state (tier up, band exit,
// size reclassification).
func (p *Pipeline) runSpine(ctx context.Context, st *State, pair *Pair, advisory []string) (*SpineResult, error) {
	res := &SpineResult{AdvisoryNotes: advisory}

	res.Uncovered = coverageCheck(pair, st.AcceptedUncovered)

	// (b) Floor re-check + band re-decision (band exit is one-way).
	res.FloorAdds = floorRecheck(&pair.Plan)
	st.addFloors(res.FloorAdds)
	if st.Band {
		if ok, why := p.bandRecheck(st, &pair.Plan); !ok {
			st.exitBand(why)
		}
	}

	// (c) Size-delta.
	factor, err := p.Settings.Float(keySizeRecheckFactor)
	if err != nil {
		return nil, fmt.Errorf("intake: read ⚙ %s: %w", keySizeRecheckFactor, err)
	}
	res.SizeFinding, res.SizeDetail = sizeDelta(st.Guess, pair.Plan.Est, factor)
	if res.SizeFinding == "over" {
		// Reclassify size and re-run tier assignment (monotone upward;
		// ceremony upgrades before expensive work — Spec S06.7(c)).
		st.Guess = pair.Plan.Est
		st.exitBand("size re-check: plan exceeds the Stage-0 guess")
		if p.Classifier != nil {
			if prop, cerr := p.Classifier.Classify(ctx, st.Req, st.Registry); cerr == nil && ValidTier(prop.Tier) {
				st.Tier = maxTier(st.Tier, prop.Tier)
			}
		}
	}

	// (d) Research-node presence.
	res.MissingResearch = researchCheck(&pair.Plan, st.openDataBearing())

	res.OpenMarkers = append(res.OpenMarkers, pair.Spec.Clarifications...)
	res.TierAfter = st.Tier
	res.BandAfter = st.Band
	return res, nil
}

// summary renders the spine findings for the ledger decision entry.
func (r *SpineResult) summary() string {
	parts := []string{}
	if len(r.Uncovered) > 0 {
		parts = append(parts, "uncovered: "+strings.Join(r.Uncovered, ","))
	}
	if len(r.FloorAdds) > 0 {
		parts = append(parts, fmt.Sprintf("floors+%d", len(r.FloorAdds)))
	}
	if r.SizeFinding != "" {
		parts = append(parts, "size:"+r.SizeFinding)
	}
	if len(r.MissingResearch) > 0 {
		parts = append(parts, "missing research: "+strings.Join(r.MissingResearch, ","))
	}
	if len(r.OpenMarkers) > 0 {
		parts = append(parts, fmt.Sprintf("open markers: %d", len(r.OpenMarkers)))
	}
	if len(parts) == 0 {
		return "all checks pass"
	}
	return strings.Join(parts, "; ")
}

// ---- S02.8 claim intersection ----

// globsIntersect is the conservative glob-intersection used at claim time
// (Spec S02.8): "**" intersects everything; otherwise two globs are
// treated as intersecting when either's literal prefix (up to the first
// metacharacter) prefixes the other's. Over-detection is the safe
// direction — it sequences more, never overwrites.
func globsIntersect(a, b string) bool {
	if a == "**" || b == "**" || a == "" || b == "" {
		return true
	}
	pa, pb := literalPrefix(a), literalPrefix(b)
	return strings.HasPrefix(pa, pb) || strings.HasPrefix(pb, pa)
}

func literalPrefix(glob string) string {
	if i := strings.IndexAny(glob, "*?["); i >= 0 {
		return glob[:i]
	}
	return glob
}

// anyGlobsIntersect reports whether any pair across the two sets
// intersects.
func anyGlobsIntersect(a, b []string) bool {
	for _, ga := range a {
		for _, gb := range b {
			if globsIntersect(ga, gb) {
				return true
			}
		}
	}
	return false
}

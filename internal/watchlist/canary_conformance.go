package watchlist

import (
	"context"
	"fmt"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/conformance"
)

// canary_conformance.go — the conformance canaries (Spec S14.6 ¶3 bullet 2:
// "the S14.5 adapter suites on their weekly schedule").
//
// COORDINATOR DISPOSITION OQ5(a): the conformance canary is a DUENESS
// SURFACER. The adapter conformance suites are Go tests in three marked tiers
// and the release artifact is a static binary with no toolchain, so the daemon
// cannot run them — and re-implementing the tier-R checks as in-process probes
// would give Sinet two implementations of the same conformance claim that can
// disagree, which is exactly what "suites assert behavior" guards against. So
// the canary raises a card on a due weekly LANE-AFFECTING adapter row, and the
// actual run stays an operator/CI act that records through
// conformance.RecordResult into the row it is already registered under.
//
// That narrows B5-4's "dueness raises nothing at v0" to "dueness raises nothing
// EXCEPT the two weekly lane-affecting adapter rows" — a deliberate narrowing,
// recorded in CONVENTIONS §34.
//
// internal/conformance is NOT modified by this packet: the registry stays a
// scheduling HOME, this layer reads its dueness and writes results through its
// existing recording verb, and no new registry row and no new event type are
// created (results land as the already-minted eval.score_recorded).

// ConformanceCanary surfaces due adapter-suite rows as cards.
type ConformanceCanary struct {
	// Registry is the S14.5 conformance registry; nil ⇒ the canary is absent.
	Registry *conformance.Store
}

// NewConformanceCanary builds the dueness surfacer.
func NewConformanceCanary(reg *conformance.Store) *ConformanceCanary {
	return &ConformanceCanary{Registry: reg}
}

// conformanceDueWindow is how often ONE due row may re-raise its card.
// STRUCTURAL, not ⚙. A due row stays due until somebody runs it, and the shell
// asks the canary layer far more often than weekly, so without a window a
// standing obligation would re-card on every sweep. The window matches the rows'
// own weekly cadence: one reminder per cadence period, which is the most a
// reminder can honestly be.
const conformanceDueWindow = 7 * 24 * time.Hour

// SurfaceDue raises one card per due weekly lane-affecting adapter row, at most
// once per row per window. It returns how many cards it raised.
//
// The card is a drift.finding like every other card in this package, so it
// derives through the existing inbox query with no internal/api edit. Its change
// class is `unclear` — the S14.2 abstain member — because that is the honest
// statement: nothing has been observed about the adapter's behaviour, the suite
// that WOULD observe it is due and unrun, and the platform cannot tell whether
// it drifted. That routes to the daily digest, which is right: a due suite is
// not a stall, a loop or a spend anomaly, and flag-now is budgeted at
// ⚙ watchdog.flag_now_target = 2/day for the things that are.
func (cc *ConformanceCanary) SurfaceDue(ctx context.Context, c *Canaries) (int, error) {
	if cc.Registry == nil || c.Emitter == nil {
		return 0, nil
	}
	rows, err := cc.Registry.Due(ctx)
	if err != nil {
		return 0, fmt.Errorf("watchlist: read conformance dueness: %w", err)
	}
	now := c.now()
	raised := 0
	for _, r := range rows {
		if r.Cadence != conformance.CadenceWeekly || r.AffectClass != conformance.AffectLane {
			continue
		}
		last, had, err := c.Emitter.LastObservedChangeAt(ctx, r.ID)
		if err != nil {
			return raised, err
		}
		if had && now.Sub(last) < conformanceDueWindow {
			continue
		}
		when := "has never run"
		if r.HasRun {
			when = "last ran " + r.LastRun.UTC().Format(time.RFC3339)
		}
		if _, _, err := c.Emitter.Emit(ctx, Hit{
			RowID:  r.ID,
			Kind:   "conformance",
			Source: "conformance:" + r.ID,
			Lane:   conformanceRowLane(r.ID),
			Class:  ClassUnclear,
			Summary: fmt.Sprintf("conformance suite %q is due (weekly, lane-affecting) and %s — running it is an operator/CI act",
				r.ID, when),
			Detail: fmt.Sprintf("owning section: %s\nfixtures: %s\nschedule: %s\nlast result: %s\n\n"+
				"The release binary carries no Go toolchain, so the daemon cannot run this suite (OQ5(a)). "+
				"Run it and record the outcome through the registry's RecordResult verb; the row is its scheduling home.",
				r.OwningSection, r.Fixtures, r.Schedule, firstNonEmpty(r.LastResult, "never recorded")),
		}); err != nil {
			return raised, err
		}
		raised++
	}
	return raised, nil
}

// conformanceRowLane maps an adapter registry row to the lane it affects, so
// the card carries the same server-side lane truth every other card does.
func conformanceRowLane(rowID string) string {
	switch rowID {
	case "adapter-anthropic":
		return LaneAnthropic
	case "adapter-local":
		return LaneLocal
	case "adapter-zai":
		// The zai LANE row, landed at P3-LN-2B. Its sibling adapter-opencode
		// row is deliberately absent from this map: that one proves the
		// SUBSTRATE, and a substrate is not a lane — mapping it would file a
		// substrate result under whichever lane happened to be listed first.
		return LaneZAI
	case "adapter-kimi":
		// The kimi LANE row (P3-LN-3). Two lanes now ride the one opencode
		// substrate row, which is exactly why that row stays unmapped: a
		// substrate result belongs to no single lane.
		return LaneKimi
	}
	return ""
}

// RecordAdapterSuite records an adapter-suite run into its EXISTING registry
// row. This is the recording half of OQ5(a): the run is an operator/CI act, and
// this is the one verb it lands through — no new row, no new event type (the
// registry appends the already-minted eval.score_recorded), and
// internal/conformance is unmodified.
//
// It refuses any row that is not one of the two weekly lane-affecting adapter
// rows, so the conformance-canary path cannot be used to write a result into an
// obligation it does not own.
func (cc *ConformanceCanary) RecordAdapterSuite(ctx context.Context, r conformance.Result) error {
	if cc.Registry == nil {
		return fmt.Errorf("watchlist: no conformance registry is wired")
	}
	if conformanceRowLane(r.RowID) == "" {
		return fmt.Errorf("watchlist: %q is not a weekly lane-affecting adapter row — the conformance canary records only those (OQ5(a))", r.RowID)
	}
	return cc.Registry.RecordResult(ctx, r)
}

package watchlist

import (
	"context"
	"fmt"
)

// decay.go — the P-T11-3 decay posture.
//
// A DEAD WATCH MUST ANNOUNCE ITSELF. Consecutive fetch failures accumulate on
// the row; when the streak reaches ⚙ watchlist.fetch_fail_streak (read by dotted
// key at evaluation time, so an operator change applies live) the meta-watch
// raises a drift card and advances the fetcher-escalation ladder:
//
//	plain → browser fetcher → flagged-decayed card proposing an alternative
//
// The second rung is real only where it can be: at changedetection.io 0.55.8 the
// browser leg is an EXTERNAL CDP service the operator selects on the organ with
// PLAYWRIGHT_DRIVER_URL — the project ships no browser and does not depend on
// playwright — so Sinet expresses the escalation as CONFIGURATION
// (`fetch_backend: html_webdriver` on the reconciled watch) and never claims a
// browser exists. A feed or API row has no browser rung at all and goes straight
// from plain to decayed. Every escalation and every decay is on the card, with
// the row's P-T11-3 migrate-to-a-feed bias attached.

// noteFailure records a failed fetch cycle and applies the ladder.
func (x *Executor) noteFailure(ctx context.Context, r Row, cause error) error {
	streak, err := x.Store.RecordFailure(ctx, r.ID, x.now())
	if err != nil {
		return err
	}
	limit, err := x.Settings.Int(keyFetchFailStreak)
	if err != nil {
		return fmt.Errorf("watchlist: read ⚙ %s: %w", keyFetchFailStreak, err)
	}
	x.Logger.Warn("watchlist: fetch failed", "row", r.ID, "streak", streak, "limit", limit, "err", cause)
	if int64(streak) < limit {
		return nil
	}

	next, rung := nextDecayStage(r)
	if err := x.Store.SetDecayStage(ctx, r.ID, next); err != nil {
		return err
	}
	_, err = x.Emitter.Emit(ctx, Hit{
		RowID: r.ID, Kind: r.Kind, Source: sourceOf(r), Lane: r.Lane,
		// Platform-authored: the platform, not a model, knows the watch is
		// failing. A dead source is an ENDPOINTS-class change.
		Class:       ClassEndpoints,
		Summary:     fmt.Sprintf("watch %s has failed %d consecutive fetch cycles — %s", r.ID, streak, rung),
		Detail:      decayDetail(r, streak, limit, next, cause),
		DecayStage:  next,
		MigrateBias: r.MigrateBias,
	})
	return err
}

// nextDecayStage advances the ladder and names the rung taken.
func nextDecayStage(r Row) (int, string) {
	if r.Kind == KindPage && r.DecayStage < DecayEscalated {
		return DecayEscalated, "escalating it to the organ's browser fetcher on the next reconciliation"
	}
	if r.DecayStage < DecayDecayed {
		return DecayDecayed, "the source is flagged DECAYED and an alternative should replace it"
	}
	return DecayDecayed, "the source remains DECAYED"
}

func decayDetail(r Row, streak int, limit int64, next int, cause error) string {
	detail := fmt.Sprintf(
		"row: %s (kind %s, tier %d, cadence %s)\nsource: %s\nconsecutive failures: %d (⚙ watchlist.fetch_fail_streak = %d)\nlast error: %v\ndecay stage: %d → %d\n",
		r.ID, r.Kind, r.Tier, r.Cadence, sourceOf(r), streak, limit, cause, r.DecayStage, next)
	switch next {
	case DecayEscalated:
		detail += "Next rung: the reconciled changedetection.io watch is asked for fetch_backend=html_webdriver. " +
			"Whether a browser leg exists is the ORGAN's fact — at pin 0.55.8 it is an external CDP service the operator selects with PLAYWRIGHT_DRIVER_URL, " +
			"so if none is provisioned this rung is a no-op and the next streak decays the source.\n"
	case DecayDecayed:
		detail += "The escalation ladder is exhausted: propose an alternative source for this watch.\n"
	}
	if r.MigrateBias {
		detail += "P-T11-3 standing bias: this is a scrape target — migrate it to a feed or API the moment one exists.\n"
	}
	return detail
}

func sourceOf(r Row) string {
	if r.URL != "" {
		return r.URL
	}
	return r.ID
}

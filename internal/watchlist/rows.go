package watchlist

import (
	"fmt"
	"strings"
	"time"
)

// Row is one watch-row config row (Spec S14.6 ¶1). The first block is the
// IMMUTABLE structure (trigger-enforced, migration 0013); the FetchState block
// is the only part that moves.
type Row struct {
	ID string `json:"id"`
	// Kind is one of page|feed|api|study.
	Kind string `json:"kind"`
	// URL is the S14.6 ¶1 "url|feed" field; empty only for a study row.
	URL string `json:"url,omitempty"`
	// ParserHint is the S14.6 ¶1 "parser-hint" field (atom|rss|json|"").
	ParserHint string `json:"parser_hint,omitempty"`
	// Lane is the S14.6 ¶1 "lane|candidate" field.
	Lane string `json:"lane,omitempty"`
	// Tier is the report-02 §6 source tier (1–4); for a study row it is the
	// review-urgency class on the same scale.
	Tier int `json:"tier"`
	// Cadence is the dueness token (continuous|weekly|monthly|quarterly).
	Cadence string `json:"cadence"`
	Enabled bool   `json:"enabled"`
	// Group is the seeding provenance (report-02 §6, S16.8, components.lock,
	// G4-study).
	Group string `json:"group,omitempty"`
	// Notes cites the row's ratifying source (R3) — every row carries one.
	Notes string `json:"notes"`
	// MigrateBias records the P-T11-3 standing bias to migrate this scrape
	// target to a feed/API the moment one exists. It is a spec-named posture
	// held as data and surfaced on the decay card, not a comment.
	MigrateBias bool `json:"migrate_bias,omitempty"`

	FetchState `json:"-"`
}

// FetchState is a row's mutable per-fetch state.
type FetchState struct {
	LastFetchAt  time.Time
	HasFetched   bool
	ETag         string
	LastModified string
	ContentHash  string
	SeenEntries  []string
	Baseline     string
	FailStreak   int
	DecayStage   int
	ExternalRef  string
}

// Decay stages (P-T11-3 fetcher escalation ladder).
const (
	// DecayPlain is the ordinary fetcher.
	DecayPlain = 0
	// DecayEscalated marks a page row escalated to the organ's browser
	// fetcher (`fetch_backend: html_webdriver`). Whether a browser leg
	// actually exists is the ORGAN's fact — changedetection.io 0.55.8 binds
	// its browser fetcher to an external CDP service selected by
	// PLAYWRIGHT_DRIVER_URL and ships no browser of its own — so Sinet
	// requests the backend as configuration and never claims a browser is
	// present.
	DecayEscalated = 1
	// DecayDecayed marks a source whose escalation is exhausted: a card
	// proposing an alternative source has been raised.
	DecayDecayed = 2
)

// seenEntryCap bounds the per-row set of already-reported feed-entry hashes.
// A feed window is far shorter than this, so the bound never loses a live
// entry; it keeps the column small forever. Structural, not ⚙.
const seenEntryCap = 200

// Validate checks a row's structure. It is applied to the in-code seed set AND
// to an operator LoadRows override, so a hand-edited seed file fails loudly at
// load rather than half-applying (the §14/§15/§17 seed precedent).
func (r Row) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("%w: empty id", ErrInvalidRow)
	}
	switch r.Kind {
	case KindPage, KindFeed, KindAPI, KindStudy:
	default:
		return fmt.Errorf("%w: %s: unknown kind %q", ErrInvalidRow, r.ID, r.Kind)
	}
	if r.Kind != KindStudy && strings.TrimSpace(r.URL) == "" {
		return fmt.Errorf("%w: %s: kind %s needs a url|feed", ErrInvalidRow, r.ID, r.Kind)
	}
	if r.Tier < 1 || r.Tier > 4 {
		return fmt.Errorf("%w: %s: tier %d outside 1–4", ErrInvalidRow, r.ID, r.Tier)
	}
	if _, ok := cadenceInterval(r.Cadence); !ok {
		return fmt.Errorf("%w: %s: unknown cadence %q", ErrInvalidRow, r.ID, r.Cadence)
	}
	if strings.TrimSpace(r.Notes) == "" {
		return fmt.Errorf("%w: %s: every row cites its ratifying source in notes (R3)", ErrInvalidRow, r.ID)
	}
	return nil
}

// Pollable reports whether a row has a fetch mechanism at all. A study row is a
// standing review obligation, never polled — it surfaces through PendingReview.
func (r Row) Pollable() bool { return r.Kind != KindStudy }

// due reports whether a pollable row's cadence has elapsed. A never-fetched row
// is due. Dueness derives from STORED state, so it survives restart and suspend
// (never an in-memory ticker).
func (r Row) due(now time.Time) bool {
	if !r.Enabled || !r.Pollable() {
		return false
	}
	if !r.HasFetched {
		return true
	}
	iv, ok := cadenceInterval(r.Cadence)
	if !ok {
		return false
	}
	return now.Sub(r.LastFetchAt) >= iv
}

// reviewDue reports whether a study row's review cadence has elapsed (R21). It
// raises nothing on its own: it is the read surface the S16.7 quarterly
// dependency pass consumes, and its outputs are PROPOSALS, never silent bumps.
func (r Row) reviewDue(now time.Time) bool {
	if !r.Enabled || r.Pollable() {
		return false
	}
	if !r.HasFetched {
		return true
	}
	iv, ok := cadenceInterval(r.Cadence)
	if !ok {
		return false
	}
	return now.Sub(r.LastFetchAt) >= iv
}

// seen reports whether a per-entry hash was already reported for this row.
func (s FetchState) seen(hash string) bool {
	for _, h := range s.SeenEntries {
		if h == hash {
			return true
		}
	}
	return false
}

// withSeen returns the bounded seen-entry set extended with the new hashes,
// newest last.
func (s FetchState) withSeen(hashes ...string) []string {
	out := append(append([]string(nil), s.SeenEntries...), hashes...)
	if len(out) > seenEntryCap {
		out = out[len(out)-seenEntryCap:]
	}
	return out
}

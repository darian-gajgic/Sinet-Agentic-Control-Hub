package benchmark

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// epoch.go is BENCH-REG §9 (P-T11-2), frozen:
//
//   - "An epoch ENDS when the DIRECT arm's observed model identity changes
//     (consumer surfaces auto-upgrade; baselines cannot be frozen). Platform-arm
//     model changes are recorded per pair but do not split epochs (the platform
//     evolving is the treatment)."
//   - "No decision statistic ever pools across epochs. Gate and alarm evaluate
//     over the CURRENT epoch only. Reporting views may show per-epoch history
//     side by side."
//   - "Predecessor data: Nexus bench-02 results are a CLOSED HISTORICAL EPOCH —
//     recorded for context, never pooled into any Sinet statistic (different
//     protocol, non-blind, different system)."
//
// The third clause is honoured by CONSTRUCTION and deliberately by absence:
// there is no bench-02 import, no seed, no migration column and no code path
// anywhere in this package that could introduce a pre-Sinet pair. An epoch is
// created only by assignEpochTx below, from a direct-arm identity observed on a
// Sinet run. There is nothing to exclude because nothing can enter.
//
// The tracker is the one place a new epoch is opened, so "the direct arm's
// identity, and nothing else, splits an epoch" is a property of a single
// function rather than a convention spread across call sites.

// assignEpochTx returns the epoch id a pair being recorded belongs to, opening
// the next epoch first when the direct arm's observed identity has changed.
//
// An empty observed identity (a declined pair, or a direct arm metering never
// measured) never splits an epoch: an unobserved identity is not a CHANGED
// identity, and splitting on absence would shred the record into unusable
// one-pair epochs every time a reading was missing.
func (p *Practice) assignEpochTx(ctx context.Context, tx *sql.Tx, domain, directModel string) (string, error) {
	st, err := p.Store.ensureDomainTx(ctx, tx, domain)
	if err != nil {
		return "", err
	}
	if directModel == "" {
		return st.CurrentEpochID, nil
	}
	if st.EpochIdentity == "" {
		// The domain's first observed direct-arm identity DEFINES the current
		// epoch rather than ending it — there was no prior identity to differ
		// from, so no interval closed.
		if _, err := tx.ExecContext(ctx,
			`UPDATE benchmark_domains SET epoch_identity = ? WHERE domain = ?`,
			directModel, domain); err != nil {
			return "", fmt.Errorf("benchmark: set epoch identity for %q: %w", domain, err)
		}
		return st.CurrentEpochID, nil
	}
	if st.EpochIdentity == directModel {
		return st.CurrentEpochID, nil
	}

	// The identity changed: this pair opens the next epoch and belongs to it.
	next := st.EpochSeq + 1
	id := epochID(domain, next)
	if _, err := tx.ExecContext(ctx,
		`UPDATE benchmark_domains
		    SET current_epoch_id = ?, epoch_started_ts = ?, epoch_identity = ?, epoch_seq = ?
		  WHERE domain = ?`,
		id, p.now().Format(time.RFC3339Nano), directModel, next, domain); err != nil {
		return "", fmt.Errorf("benchmark: open next epoch for %q: %w", domain, err)
	}
	p.logger().Info("benchmark: epoch ended — the direct arm's observed model identity changed (BENCH-REG §9)",
		"domain", domain, "closed_epoch", st.CurrentEpochID, "closed_identity", st.EpochIdentity,
		"new_epoch", id, "new_identity", directModel)
	return id, nil
}

// Epoch is one epoch of a domain's history, for the §16 side-by-side readout.
type Epoch struct {
	ID       string    `json:"id"`
	Identity string    `json:"identity"`
	Started  time.Time `json:"started"`
	Current  bool      `json:"current"`
}

// EpochHistory returns a domain's epochs oldest-first, derived from the recorded
// pairs themselves (each carries its epoch id and the direct-arm identity that
// defined it). §16 permits showing per-epoch history side by side; §9 forbids
// POOLING across epochs, and this returns them separately for exactly that
// reason — it is a readout, never a decision input.
func (p *Practice) EpochHistory(ctx context.Context, domain string) ([]Epoch, error) {
	st, err := p.Store.Domain(ctx, domain)
	if err != nil {
		return nil, err
	}
	// The epoch's IDENTITY is the direct-arm model that defines it, so the read
	// takes the first NON-EMPTY one: a declined pair records no direct arm, and
	// if a decline happened to be an epoch's earliest recorded pair, grouping on
	// the raw column would display the epoch as having no identity at all —
	// which would misreport the very thing §9 says an epoch IS. Ordering is by
	// the epoch's earliest recorded pair either way, so the timeline is
	// unaffected.
	rows, err := p.Store.db.QueryContext(ctx,
		`SELECT epoch_id,
		        COALESCE(MIN(NULLIF(direct_model, '')), '') AS identity,
		        MIN(recorded_ts) AS first_ts
		   FROM benchmark_pairs
		  WHERE domain = ? AND state IN (?, ?) AND epoch_id <> ''
		  GROUP BY epoch_id
		  ORDER BY first_ts, epoch_id`, domain, StateRecorded, StateDeclined)
	if err != nil {
		return nil, fmt.Errorf("benchmark: epoch history for %q: %w", domain, err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	out := []Epoch{}
	for rows.Next() {
		var e Epoch
		var started string
		if err := rows.Scan(&e.ID, &e.Identity, &started); err != nil {
			return nil, fmt.Errorf("benchmark: scan epoch: %w", err)
		}
		seen[e.ID] = true
		e.Started, e.Current = parseTS(started), e.ID == st.CurrentEpochID
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !seen[st.CurrentEpochID] {
		// The current epoch has no recorded pair yet; it still exists.
		out = append(out, Epoch{
			ID: st.CurrentEpochID, Identity: st.EpochIdentity,
			Started: st.EpochStarted, Current: true,
		})
	}
	return out, nil
}

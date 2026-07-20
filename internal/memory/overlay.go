package memory

import (
	"context"
	"fmt"
)

// overlay.go — the worker-overlay read surface (Spec S08.4 × S09.2): the
// S08 instance compile reads the requester's overlay L2 slice (per user ×
// template) through this store method — the settled memory machinery, no
// second system. At v0 the result is structurally EMPTY: the gate refuses
// worker-overlay writes (ErrScopeDormant) and no other writer exists, so
// dormancy is the absence of content, not a disabled code path (Spec S09.4
// activation table). v1 activation adds writers; this read is day-one.

// OverlaySlice returns the ACTIVE L2 entries of the worker-overlay scope
// for owner × template, deterministically ordered (oldest first, so the
// most recent lesson lands last in the most-specific-last concatenation —
// Spec S08.4). L1 observations are NOT returned here: their injection is
// "labeled history, never rules" and activates at v1 with the L1 writers
// (Spec S09.1).
func (s *Store) OverlaySlice(ctx context.Context, owner, templateID string) ([]Entry, error) {
	if owner == "" || templateID == "" {
		return nil, fmt.Errorf("%w: overlay slice needs owner and template", ErrInvalidEntry)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+entryColumns+` FROM knowledge_entries
		 WHERE scope = ? AND user_id = ? AND scope_ref = ? AND layer = 'L2' AND status = 'active'
		 ORDER BY created_ts, entry_id`,
		string(ScopeWorkerOverlay), owner, templateID)
	if err != nil {
		return nil, fmt.Errorf("memory: overlay slice: %w", err)
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

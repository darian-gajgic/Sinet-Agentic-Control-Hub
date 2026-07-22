package preview

import "context"

// comparison.go builds the before-vs-after dual instances (Spec S13.8 §S1.9;
// R17/R18): the accepted revision (the "before", resolved via
// review.Store.AcceptedRevision) and the candidate revision (the "after"), each
// launched as a full preview instance against its own port/route/substrate and
// each taking one ⚙ preview.max_concurrent slot (R15). The result is the typed,
// JSON-serializable synced-navigation DATA contract the S15 dual-iframe widget
// consumes (B6) — DATA only, no HTML/iframe here (R18).

// LaunchComparison launches the candidate revision (default: the deliverable's
// current revision) as the "after" instance, and — when the deliverable has an
// accepted revision — the accepted pin as the "before" instance. A deliverable
// with no accepted revision yet gets the honest single-instance state (candidate
// only; revision 1 has no "before", its diff-against-base story is S13.1's, not
// a preview fake, R17).
func (m *Manager) LaunchComparison(ctx context.Context, deliverableID string, candidateRevision int, user string) (*Comparison, error) {
	after, err := m.Launch(ctx, LaunchRequest{
		DeliverableID: deliverableID, Revision: candidateRevision, Role: RoleAfter, User: user,
	})
	if err != nil {
		return nil, err
	}
	cmp := &Comparison{DeliverableID: deliverableID, After: after.View()}

	acc, err := m.cfg.Reviews.AcceptedRevision(ctx, deliverableID)
	if err != nil {
		// No accepted revision (the deliverable is not accepted): honest
		// single-instance state — the candidate stands alone, nothing to sync to.
		cmp.SingleInstance = true
		cmp.Sync = SyncSemantics{Mode: "path", Enabled: false}
		return cmp, nil
	}
	before, err := m.Launch(ctx, LaunchRequest{
		DeliverableID: deliverableID, Revision: acc.N, Role: RoleBefore, User: user,
	})
	if err != nil {
		return nil, err
	}
	bv := before.View()
	cmp.Before = &bv
	cmp.Sync = SyncSemantics{Mode: "path", Enabled: true}
	return cmp, nil
}

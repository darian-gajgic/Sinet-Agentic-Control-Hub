package benchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// optin.go is BENCH-REG §4.2.1's standing opt-in: eligibility limb 1.
//
// It is an opt-IN. The column defaults to 0 (migration 0014), no migration and
// no boot step turns it on for anyone, and a user row that does not exist is not
// opted in. The consequence is deliberate and is recorded rather than
// engineered around: until a requester says yes, their accepted deliverables are
// never sampled, and the practice accrues nothing for them.
//
// Consent is NOT delegable. The verb refuses to flip anyone's flag but the
// actor's own — including the operator's. A benchmark whose participation an
// administrator can switch on for other people is not measuring what §4.2.5
// ("selective participation must be visible, not silent") is trying to protect,
// and the requester's automation budget pays for the duplicate run (§4.2.1).
//
// The flip logs as a human decision (decision.recorded, S14.2 family 5), so the
// consent has an audit trail with its actor and its timestamp. The UI surface is
// B6's (Spec S15); this is the verb behind it.

// SetOptIn records a user's standing benchmark opt-in decision. actor must be
// the user themselves.
func (p *Practice) SetOptIn(ctx context.Context, actor, userID string, enabled bool) error {
	switch {
	case actor == "" || userID == "":
		return fmt.Errorf("%w: an opt-in decision needs its actor and its subject", ErrBadInput)
	case actor != userID:
		return fmt.Errorf("%w: the benchmark opt-in is the requester's own consent and is not delegable — %q may not set it for %q (BENCH-REG §4.2.1)", ErrBadInput, actor, userID)
	}
	return p.Store.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE users SET benchmark_opt_in = ? WHERE user_id = ?`, boolInt(enabled), userID)
		if err != nil {
			return fmt.Errorf("benchmark: set opt-in for %q: %w", userID, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%w: no user %q", ErrBadInput, userID)
		}
		now := p.now()
		decision := "opt-out"
		if enabled {
			decision = "opt-in"
		}
		payload, err := json.Marshal(DecisionPayload{
			Actor: actor, CardID: "benchmark-opt-in:" + userID, CardType: "benchmark.opt_in",
			Decision: decision,
			// The consent card is presented and decided in the same act — a
			// person toggling their own participation, not an inbox item that
			// waited. The latency is honestly zero rather than fabricated.
			PresentedAt: now.Format(time.RFC3339Nano),
			DecidedAt:   now.Format(time.RFC3339Nano),
			Subject:     userID,
			Ref:         "BENCH-REG §4.2.1",
			Reason:      "standing benchmark opt-in; duplicate-run consumption comes from this requester's automation budget",
		})
		if err != nil {
			return err
		}
		_, err = p.Log.AppendTx(ctx, tx, eventlog.Append{
			UserID: userID, Type: EventDecision, SchemaVersion: eventSchemaVersion,
			Payload: payload, Time: now,
		})
		return err
	})
}

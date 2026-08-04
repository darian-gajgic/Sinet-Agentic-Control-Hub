package watchdog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
)

// suppress.go — the S14.4 per-rule suppression EFFECT (brief R19; OQ6). A
// suppress is a TUNING SIGNAL, never a resolution: it emits watchdog.suppressed
// and, on every ⚙ watchdog.suppress_retune_count-th suppression of a rule,
// PROPOSES a threshold-raise admin card — a proposal, NEVER an auto-move (the ⚙
// write stays operator-gated through the settings registry; §6 automation moves
// value only within bounds, and a threshold raise is an operator act). The
// operator-facing HTTP endpoint + React button are B6/S15 (the B5-2
// data-not-rendering precedent); this is the internal effect they call.

// RuleRetuneProposal is the admin-card rule the retune proposal rides (R19). It
// is an anomaly class under FamilyWatchdog (rule id + trigger evidence), a
// digest-tier proposal card — never an auto-move.
const RuleRetuneProposal = "watchdog.retune_proposal"

// Suppress records a human's suppression of a rule's flag on a run (R19). The
// subject `rule` is the flag's FULL anomaly_class — a bare rule for a run-scoped
// flag (watchdog.loop), or a SUFFIXED class for a run-less one
// (watchdog.spend:<owner>, watchdog.organ_absence:<organ>,
// watchdog.retune_proposal:<key>). It is carried verbatim in the payload so the
// derive-from-log supersession (which matches the full anomaly_class) can clear
// even a suffixed run-less flag (drain D5 — those were permanently un-clearable
// when the caller passed the bare rule). The retune count + ⚙-key are derived
// from the BASE rule (split on the first ':'). It emits watchdog.suppressed
// (owner-attributed when run-scoped, R34) with the acting principal in the
// payload (drain D13 — a suppress is an operator act, not the platform's), and
// on every ⚙ watchdog.suppress_retune_count-th suppression proposes a
// threshold-raise admin card. runID "" suppresses a run-less/platform flag.
func (w *Watchdog) Suppress(ctx context.Context, actor, runID, rule string) error {
	base := baseRule(rule)
	prior, err := w.suppressCount(ctx, base)
	if err != nil {
		return err
	}
	count := prior + 1

	retuneN, err := w.settings.Int(keySuppressRetune)
	if err != nil {
		return err
	}
	propose := retuneN > 0 && count%int(retuneN) == 0 && retuneKeyFor(base) != ""

	payload, err := json.Marshal(SuppressPayload{
		Rule:    rule,
		Actor:   actor,
		Count:   count,
		Reason:  "operator suppression (tuning signal, S14.4)",
		Propose: propose,
	})
	if err != nil {
		return err
	}

	if runID == "" {
		if _, err := w.log.Append(ctx, eventlog.Append{
			UserID:        run.ActorPlatform,
			Type:          EventSuppressed,
			SchemaVersion: eventSchemaVersion,
			Payload:       payload,
		}); err != nil {
			return err
		}
	} else {
		r, gerr := w.runs.Get(ctx, runID)
		if gerr != nil {
			return gerr
		}
		if _, err := w.log.Append(ctx, eventlog.Append{
			RunID:         runID,
			Generation:    r.Generation,
			UserID:        r.UserID,
			Type:          EventSuppressed,
			SchemaVersion: eventSchemaVersion,
			Payload:       payload,
		}); err != nil {
			return err
		}
	}

	if propose {
		return w.proposeRetune(ctx, base, count)
	}
	return nil
}

// baseRule strips a run-less flag's suffix (watchdog.spend:<owner> →
// watchdog.spend) so the retune count/⚙-key are keyed on the rule, not the
// per-owner/per-organ/per-key instance (drain D5). A bare rule is unchanged.
func baseRule(rule string) string {
	if i := strings.IndexByte(rule, ':'); i >= 0 {
		return rule[:i]
	}
	return rule
}

// proposeRetune emits the threshold-raise admin card for a BASE rule (R19) — a
// digest-tier platform flag naming the ⚙ key the operator may raise, deduped
// per-key. It NEVER writes the ⚙ (operator-gated through the registry, §6).
func (w *Watchdog) proposeRetune(ctx context.Context, base string, count int) error {
	key := retuneKeyFor(base)
	if key == "" {
		return nil // a structural-threshold rule has no ⚙ to raise
	}
	class := RuleRetuneProposal + ":" + key
	return w.ownerlessFlag(ctx, run.ActorPlatform, trigger{
		Rule:      RuleRetuneProposal,
		Signature: key,
		Counts:    map[string]int{"suppressions": count},
		Detail: fmt.Sprintf("rule %s suppressed %d× — consider raising ⚙ %s (a proposal; the operator sets it in the settings surface, never an auto-move)",
			base, count, key),
	}, class)
}

// suppressCount counts prior watchdog.suppressed events for a BASE rule across
// all runs and platform scope (the count is per-rule, R19; a suffixed run-less
// suppress folds into its base — drain D5). Derived from the log.
func (w *Watchdog) suppressCount(ctx context.Context, base string) (int, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT payload FROM run_events WHERE type = ?`, EventSuppressed)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return 0, err
		}
		if baseRule(firstString(json.RawMessage(payload), "rule")) == base {
			n++
		}
	}
	return n, rows.Err()
}

// retuneKeyFor maps a ⚙-thresholded rule to the settings key a retune proposal
// raises (R19). Rules with a STRUCTURAL threshold (suspicious-completion, the
// registered checks, the dead-man canary) have no ⚙ and return "" — no proposal.
func retuneKeyFor(rule string) string {
	switch rule {
	case RuleLoop:
		return keyLoopRepeat
	case RulePingPong:
		return keyPingPongCycles
	case RuleErrorLoop:
		return keyErrorLoop
	case RuleSilence:
		return keySilenceBudget
	case RuleSpend:
		return keySpendMedianMult
	default:
		return ""
	}
}

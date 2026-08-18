package run

import "encoding/json"

// canceldetail.go — the ONE structured shape a human cancel writes onto its
// `run.state_changed` transition, and the ONE definition of the discriminator
// that shape is recognised by (P3-RW-18 D1-R1).
//
// WHY IT LIVES HERE. Four mint paths cancel a run — the verb/ladder path
// (internal/stage/cancel.go), the verify-card answer (internal/stage/answer.go),
// and the two intake cancels (internal/intake/answer.go) — and `internal/intake`
// cannot import `internal/stage`. `internal/run` is the package all of them
// already depend on to take the transition at all, so it is the only place the
// detail can have a single definition. A second spelling of the cause string is
// exactly the drift that made the walk's cancel unservable: the projection
// discriminates on this literal, so it must have one producer.
//
// WHAT THE RECORD IS FOR. 15.6 attribution: a cancel is a HUMAN act, so the
// transition names the human who took it (Actor) and this detail carries the
// same name in a place the S02.2 reconstruction can read without guessing at
// reason prose. `internal/api`'s decisions derive (reads.go taskDecisions)
// reads `$.detail.cause` = CancelCauseHuman and serves the row — reconstruction
// from a durable record, never a second mint (the approvals-plane rule that
// cancels are never double-minted still holds).

// CancelCauseHuman is the discriminator: this transition ended a run because a
// person cancelled it (4.5 — cancel is always available). It is a payload
// literal, so it is frozen: the projection and the S14.10 catalog's
// `status.tasks_cancelled` entry both match on this exact string, and rows
// already written carry it.
const CancelCauseHuman = "human cancel (4.5)"

// CancelDetail renders the structured detail for a human cancel's transition.
//
// actor is the person who cancelled (15.6). ladderInvoked reports whether the
// S03.1 safe-boundary → abort → TERM → KILL ladder ran on a live session.
// askID names the card the cancel was answered at when there was one (S2.4
// wants "card id + type"); it is omitted for the verb path, which has no card
// — an empty key would claim a card that never existed. reason is the person's
// own one-line motive, captured at whichever cancel affordance they used, and
// is omitted on the same principle: a blank reason would claim a motive nobody
// gave, and the surfaces render their honest line from the ABSENT key.
//
// The reason is taken VERBATIM. Its bound belongs to the transport, which is
// where a caller can be told to shorten it (internal/api); this constructor
// never edits and never fails, so every cancel that reaches it is recorded.
func CancelDetail(actor string, ladderInvoked bool, askID, reason string) json.RawMessage {
	b, err := json.Marshal(struct {
		Cause         string `json:"cause"`
		Actor         string `json:"actor"`
		LadderInvoked bool   `json:"ladder_invoked"`
		AskID         string `json:"ask_id,omitempty"`
		Reason        string `json:"reason,omitempty"`
	}{CancelCauseHuman, actor, ladderInvoked, askID, reason})
	if err != nil {
		// Marshalling five plain fields cannot fail; a nil detail still leaves
		// the transition (and its actor) recorded, so the cancel is never lost.
		return nil
	}
	return b
}

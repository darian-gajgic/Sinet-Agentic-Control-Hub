package stage_test

// familywalk_test.go — the ONE shared traversal helper the landed e2e walks
// gained when P3-RW-11 put the S06.5 family question in front of the interview.
//
// WHY EVERY WALK NEEDED A LINE. These harnesses wire no Classifier and no
// Registry, which is exactly the configuration the packet's R4 targets: with
// nothing to resolve the task family from, the pipeline now ASKS instead of
// assuming generic. So the first card a walk meets is `decision.family`, and a
// walk that answered it with an interview envelope was answering the wrong
// card. That is a REAL behavior change, not a test defect — the walks are
// updated to answer it, and nothing they assert is relaxed.
//
// The helper answers "generic", deliberately: it is the family these fixtures
// were always interviewed under, so every downstream assertion (slot ids,
// clearance figures, question counts, card versions past this one) sees the
// same question set it saw before. A walk that wants a different family answers
// for itself.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
)

// clearFamilyGate answers the family question when `raw` shows it open, and
// returns the view of whatever card follows. A view showing any other card
// passes through untouched, so a walk can call it unconditionally.
func clearFamilyGate(t *testing.T, ctx context.Context, sur *stage.Surface, owner string, raw json.RawMessage) json.RawMessage {
	t.Helper()
	var v struct {
		OpenAskID string `json:"open_ask_id"`
		OpenCard  struct {
			Kind string `json:"kind"`
		} `json:"open_card"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode view: %v (%s)", err, raw)
	}
	if v.OpenCard.Kind != string(intake.CardFamily) {
		return raw
	}
	next, err := sur.Answer(ctx, owner, v.OpenAskID,
		json.RawMessage(`{"choice":"`+string(intake.FamilyGeneric)+`"}`), false)
	if err != nil {
		t.Fatalf("answer the family question: %v", err)
	}
	return next
}

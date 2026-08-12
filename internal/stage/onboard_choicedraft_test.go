package stage_test

// P3-RW-8 landing pin (coordinator, eval finding F1): the CHOICE envelope
// carrying an edited draft APPLIES it — probe-verified coherent behavior
// (the choice answer holds the same D10 authority as the landed
// {approve,draft} door), pinned here so it cannot drift into either silent
// dropping or an undocumented refusal. The SPA composes no draft today; this
// is the API-door contract, same as the raw envelope's.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
)

func TestOnboardChoiceEnvelopeAppliesAnEditedDraft(t *testing.T) {
	const owner = "alice"
	askID := stage.OnboardAskID("shop")

	h := newProjectHarness(t)
	h.dispatchOnboarding(t, owner, "shop", "Shop backend", onboardingFixture(t))

	const edited = `{"conventions":["Choice-door convention"],"commands":{"build":"make choice"}}`
	if _, err := h.sur.Answer(context.Background(), owner, askID,
		json.RawMessage(`{"choice":"approve","draft":`+edited+`}`), false); err != nil {
		t.Fatalf("{choice:approve, draft:…}: %v", err)
	}
	e, err := h.proj.Get(context.Background(), "shop")
	if err != nil || !e.Active() {
		t.Fatalf("entry after choice+draft approval = %+v err=%v, want active", e, err)
	}
	if strings.Join(e.Capture.Conventions, "|") != "Choice-door convention" || e.Capture.Commands.Build != "make choice" {
		t.Errorf("the edited draft on the choice envelope did not reach the store: %+v", e.Capture)
	}
}

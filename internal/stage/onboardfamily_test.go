package stage_test

// onboardfamily_test.go — P3-RW-11 T2 (card + approve half) and T9 (brief §7;
// R2/R10, Spec S13.7 → D10). The owner SEES the declared task family on the
// card they approve, both landed approval envelopes apply it, and a
// pre-RW-11 draft — one written before the field existed — answers exactly as
// it did before.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
)

// preOnboardWithFamily registers the pending entry through the project store
// with an owner-declared family, exactly as the create door does at the
// composition root. StartOnboarding's own OnboardStart call is then the
// idempotent RE-READ it is in production (the two calls are one onboarding),
// so the dispatched card carries the family without widening the landed stage
// seam.
func preOnboardWithFamily(t *testing.T, h *projHarness, projectID, owner, name, source, family string) {
	t.Helper()
	if _, _, err := h.proj.Onboard(context.Background(), project.OnboardInput{
		ProjectID: projectID, Owner: owner, Name: name, Source: source, Family: family,
	}); err != nil {
		t.Fatalf("Onboard(%s, family=%q): %v", projectID, family, err)
	}
}

// TestOnboardFamilyEndToEnd (T2): the digest NAMES the family, the choice
// envelope activates an entry whose capture carries it, and an edited draft on
// the choice envelope re-captures an edited one.
func TestOnboardFamilyEndToEnd(t *testing.T) {
	t.Run("digest names it and approval carries it", func(t *testing.T) {
		h := newProjectHarness(t)
		ctx := context.Background()
		const owner = "alice"
		src := onboardingFixture(t)
		preOnboardWithFamily(t, h, "shop", owner, "Shop backend", src, project.FamilySoftware)
		h.dispatchOnboarding(t, owner, "shop", "Shop backend", src)

		askID := stage.OnboardAskID("shop")
		card := h.onboardCardOf(t, askID)
		if card.Decision == nil {
			t.Fatal("onboarding card carries no decision body")
		}
		var named bool
		for _, line := range card.Decision.Detail {
			if strings.Contains(strings.ToLower(line), "family") && strings.Contains(line, project.FamilySoftware) {
				named = true
			}
		}
		if !named {
			t.Errorf("no digest line names the task family — the owner approves what they cannot see (D10): %v",
				card.Decision.Detail)
		}

		if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"choice":"approve"}`), false); err != nil {
			t.Fatalf("Answer{choice:approve}: %v", err)
		}
		e, err := h.proj.Get(ctx, "shop")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !e.Active() {
			t.Fatalf("entry state = %q, want active", e.State)
		}
		if e.Capture.Family != project.FamilySoftware {
			t.Errorf("active capture family = %q, want software", e.Capture.Family)
		}
	})

	t.Run("an edited draft applies an edited family", func(t *testing.T) {
		h := newProjectHarness(t)
		ctx := context.Background()
		const owner = "alice"
		src := onboardingFixture(t)
		preOnboardWithFamily(t, h, "shop", owner, "Shop backend", src, project.FamilySoftware)
		h.dispatchOnboarding(t, owner, "shop", "Shop backend", src)

		askID := stage.OnboardAskID("shop")
		const edited = `{"conventions":["Edited"],"commands":{"build":"make"},"family":"content"}`
		if _, err := h.sur.Answer(ctx, owner, askID,
			json.RawMessage(`{"choice":"approve","draft":`+edited+`}`), false); err != nil {
			t.Fatalf("Answer{choice:approve, draft}: %v", err)
		}
		e, err := h.proj.Get(ctx, "shop")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if e.CaptureVersion != 2 || e.Capture.Family != project.FamilyContent {
			t.Errorf("after the edited approval: v%d family %q, want v2 content (the RW-8 F1 pin applies it)",
				e.CaptureVersion, e.Capture.Family)
		}
	})
}

// TestOnboardCompatNoFamily (T9; R10): a draft with no family — every draft
// written before this packet — answers exactly as today. The entry activates,
// the capture reads back honest absence, and the digest carries no family line.
func TestOnboardCompatNoFamily(t *testing.T) {
	h := newProjectHarness(t)
	ctx := context.Background()
	const owner = "alice"
	h.dispatchOnboarding(t, owner, "shop", "Shop backend", onboardingFixture(t))

	askID := stage.OnboardAskID("shop")
	card := h.onboardCardOf(t, askID)
	if card.Decision == nil {
		t.Fatal("onboarding card carries no decision body")
	}
	for _, line := range card.Decision.Detail {
		if strings.Contains(strings.ToLower(line), "task family") {
			t.Errorf("a family-less draft grew a family digest line: %q", line)
		}
	}
	// The stored snapshot is the resume input: a draft with no `family` key is
	// what every pre-RW-11 world holds.
	var shape struct {
		Draft map[string]json.RawMessage `json:"draft"`
	}
	if err := json.Unmarshal([]byte(h.onboardAskSnapshot(t, askID)), &shape); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if _, present := shape.Draft["family"]; present {
		t.Errorf("a family-less draft serialized a `family` key — the field must be omitempty (R10): %v", shape.Draft)
	}

	if _, err := h.sur.Answer(ctx, owner, askID, json.RawMessage(`{"choice":"approve"}`), false); err != nil {
		t.Fatalf("Answer{choice:approve} on a pre-RW-11 draft: %v", err)
	}
	e, err := h.proj.Get(ctx, "shop")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !e.Active() {
		t.Fatalf("entry state = %q, want active", e.State)
	}
	if e.Capture.Family != "" {
		t.Errorf("capture family = %q, want \"\" (honest absence)", e.Capture.Family)
	}
}

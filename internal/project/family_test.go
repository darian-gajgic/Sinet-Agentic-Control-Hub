package project

// family_test.go — P3-RW-11 T2 (registry half) and T3 (brief §7; R2/R3, Spec
// S13.7 "captured content is versioned", D10). The owner-declared task family
// is capture-shaped data: it rides the draft, lands on an immutable capture
// version, survives a re-scan it cannot be detected by, and refuses anything
// outside the six-value vocabulary loudly.

import (
	"context"
	"errors"
	"testing"
)

// captureFamilyColumn reads the raw migration-0024 column for one version, so
// the assertion rests on the stored row rather than on the read path's memory.
func captureFamilyColumn(t *testing.T, f *fix, projectID string, version int) string {
	t.Helper()
	var family string
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT family FROM repo_registry_captures WHERE project_id = ? AND version = ?`,
		projectID, version).Scan(&family); err != nil {
		t.Fatalf("read capture family %s v%d: %v", projectID, version, err)
	}
	return family
}

// TestOnboardCapturesTheDeclaredFamily (T2, registry half): the family declared
// at onboarding lands on capture v1, rides the draft the owner approves, and an
// edited draft at approval re-captures it as a NEW version.
func TestOnboardCapturesTheDeclaredFamily(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	src := f.fixtureRepo(map[string]string{"go.mod": "module shop\n"})

	e, draft, err := f.store.Onboard(ctx, OnboardInput{
		ProjectID: "shop", Owner: "alice", Name: "shop", Source: src, Family: FamilySoftware,
	})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}
	if draft.Family != FamilySoftware {
		t.Errorf("draft family = %q, want software (the owner's declaration rides the draft)", draft.Family)
	}
	if e.Capture.Family != FamilySoftware {
		t.Errorf("capture v1 family = %q, want software", e.Capture.Family)
	}
	if got := captureFamilyColumn(t, f, "shop", 1); got != FamilySoftware {
		t.Errorf("stored capture family = %q, want software (migration 0024's column)", got)
	}

	// The edited-draft approval path applies an edited family exactly as it
	// applies edited conventions (the RW-8 F1 pin, one layer down).
	edited := draft
	edited.Family = FamilyContent
	if _, err := f.store.Approve(ctx, "shop", "alice", &edited); err != nil {
		t.Fatalf("Approve(edited): %v", err)
	}
	after, err := f.store.Get(ctx, "shop")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !after.Active() {
		t.Fatalf("entry state = %q, want active", after.State)
	}
	if after.CaptureVersion != 2 || after.Capture.Family != FamilyContent {
		t.Errorf("after edited approval: v%d family %q, want v2 content", after.CaptureVersion, after.Capture.Family)
	}
	if got := captureFamilyColumn(t, f, "shop", 1); got != FamilySoftware {
		t.Errorf("capture v1 family changed to %q — a captured version is immutable (S13.7)", got)
	}
}

// TestRescanCarriesFamilyForward (T3; R3): a re-scan cannot DETECT an
// owner-declared family, so it carries the current one forward. Only an
// explicit owner edit changes it.
func TestRescanCarriesFamilyForward(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	src := f.fixtureRepo(map[string]string{"go.mod": "module shop\n"})
	if _, _, err := f.store.Onboard(ctx, OnboardInput{
		ProjectID: "shop", Owner: "alice", Name: "shop", Source: src, Family: FamilySoftware,
	}); err != nil {
		t.Fatalf("Onboard: %v", err)
	}

	c, err := f.store.Rescan(ctx, "shop", "alice")
	if err != nil {
		t.Fatalf("Rescan: %v", err)
	}
	if c.Version != 2 {
		t.Fatalf("rescan version = %d, want 2", c.Version)
	}
	if c.Family != FamilySoftware {
		t.Errorf("rescanned capture family = %q, want software — a re-scan must never silently drop it (R3)", c.Family)
	}
	if got := captureFamilyColumn(t, f, "shop", 2); got != FamilySoftware {
		t.Errorf("stored v2 family = %q, want software", got)
	}
}

// TestFamilyVocabularyIsEnforcedLoudly (R2): a value outside the six-value
// vocabulary is refused with ErrBadInput at both doors, and BEFORE any store
// directory or registry row exists — a malformed declaration never leaves a
// half-onboarded project behind.
func TestFamilyVocabularyIsEnforcedLoudly(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	src := f.fixtureRepo(map[string]string{"go.mod": "module shop\n"})

	_, _, err := f.store.Onboard(ctx, OnboardInput{
		ProjectID: "shop", Owner: "alice", Name: "shop", Source: src, Family: "webshop",
	})
	if !errors.Is(err, ErrBadInput) {
		t.Fatalf("Onboard(bad family) error = %v, want ErrBadInput", err)
	}
	if _, err := f.store.Get(ctx, "shop"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a refused family left a registry entry behind: %v", err)
	}

	// The empty string is honest absence, not a refusal.
	if _, _, err := f.store.Onboard(ctx, OnboardInput{
		ProjectID: "plain", Owner: "alice", Name: "plain", Source: src,
	}); err != nil {
		t.Fatalf("Onboard(no family): %v", err)
	}
	e, err := f.store.Get(ctx, "plain")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e.Capture.Family != "" {
		t.Errorf("family-less capture = %q, want \"\" (honest absence)", e.Capture.Family)
	}

	// Capture is the other door; it refuses on its own account.
	if _, err := f.store.Capture(ctx, CaptureInput{ProjectID: "plain", By: "alice", Family: "nonsense"}); !errors.Is(err, ErrBadInput) {
		t.Errorf("Capture(bad family) error = %v, want ErrBadInput", err)
	}
}

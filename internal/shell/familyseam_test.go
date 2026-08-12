package shell

// familyseam_test.go — P3-RW-11 T2 (the seam half) and T10 (brief §7; R1/R2,
// CONVENTIONS §23 + §43). The composition root is the ONE place both
// vocabularies are legitimately visible, so it is where the cross-wall
// duplication is pinned equal — and where the door→store→registry-slice path
// for a project's task family is driven end to end.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
)

// TestFamilyVocabularyPinned (T10): internal/project duplicates the six family
// values BY VALUE because the §23 import wall bars it from importing
// internal/intake. The duplication cannot drift: this holds the two lists
// byte-equal, in order, at the root that sees both.
func TestFamilyVocabularyPinned(t *testing.T) {
	want := []string{
		string(intake.FamilySoftware), string(intake.FamilyResearch), string(intake.FamilyContent),
		string(intake.FamilyData), string(intake.FamilyChore), string(intake.FamilyGeneric),
	}
	got := project.Families()
	if len(got) != len(want) {
		t.Fatalf("project.Families() has %d values, intake has %d: %v vs %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("family %d: project %q vs intake %q — the cross-wall duplication has drifted (§23/§43)", i, got[i], want[i])
		}
	}
	// The intake card offers exactly that set, in that order (one list, two
	// readers: what the card offers is what the answer path accepts).
	choices := intake.FamilyChoices()
	if len(choices) != len(want) {
		t.Fatalf("the family card offers %d choices, want %d", len(choices), len(want))
	}
	for i, c := range choices {
		if c.Value != want[i] {
			t.Errorf("card choice %d = %q, want %q", i, c.Value, want[i])
		}
	}
}

// TestOnboardDoorThreadsFamilyToTheRegistrySlice (T2, seam half): the create
// door's family reaches project.OnboardInput at the root, lands on the capture,
// and — once the owner has approved — is what registrySeam.Match projects into
// the intake consuming shape, which is the whole of R1's plumbing.
func TestOnboardDoorThreadsFamilyToTheRegistrySlice(t *testing.T) {
	r := newOnboardRig(t)
	r.bind()

	if _, err := r.door.StartOnboardingWithFamily(r.ctx, "alice", "shop", "Shop backend", "", project.FamilySoftware); err != nil {
		t.Fatalf("StartOnboardingWithFamily: %v", err)
	}
	e, err := r.proj.Get(r.ctx, "shop")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e.Capture.Family != project.FamilySoftware {
		t.Fatalf("capture family = %q, want software (the door's declaration reached the store)", e.Capture.Family)
	}

	// Only an ACTIVE entry feeds intake resolution (S13.7), so approve as the
	// owner and then read the seam the pipeline reads.
	if _, err := r.proj.Approve(r.ctx, "shop", "alice", nil); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	slice, ok, err := (registrySeam{proj: r.proj}).Match(context.Background(), intake.Request{
		UserID: "alice", Project: "shop", Title: "Create a simple webshop", Text: "build the storefront",
	})
	if err != nil || !ok {
		t.Fatalf("registrySeam.Match: ok=%v err=%v", ok, err)
	}
	if slice.Family != intake.FamilySoftware {
		t.Errorf("registry slice family = %q, want software — the landed pipeline plumbing stays dead without it (R1)", slice.Family)
	}
}

// TestOnboardDoorRefusesAFamilyOutsideTheVocabulary (R2): the door carries no
// vocabulary of its own; it forwards, and the store's loud refusal becomes the
// transport's 400 through the landed sentinel translation.
func TestOnboardDoorRefusesAFamilyOutsideTheVocabulary(t *testing.T) {
	r := newOnboardRig(t)
	r.bind()

	_, err := r.door.StartOnboardingWithFamily(r.ctx, "alice", "shop", "Shop backend", "", "webshop")
	if err == nil {
		t.Fatal("a family outside the vocabulary was accepted")
	}
	var se *api.SurfaceError
	if !errors.As(err, &se) {
		t.Fatalf("refusal = %v (%T), want an api.SurfaceError", err, err)
	}
	if se.Status != http.StatusBadRequest {
		t.Errorf("refusal status = %d, want 400 (the landed ErrBadInput translation)", se.Status)
	}
	if _, err := r.proj.Get(r.ctx, "shop"); err == nil {
		t.Error("the refused onboarding left a registry entry behind")
	}
}

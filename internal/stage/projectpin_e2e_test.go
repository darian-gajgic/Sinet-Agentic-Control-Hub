package stage_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
)

// P3-RW-1 acceptance tests — the intake project pin (product map §5; Spec
// S06.2 step 2, S13.7, S15.2). Committed RED at grounding per
// P3/briefs/P3-RW-1.md: subtests marked [RED] assert behavior that does not
// exist yet (the optional `project` field on the intake Submit body) and are
// sanctioned to fail until the executor lands. Subtests marked [GUARD] pass
// today (vacuously, or as regression pins on the existing text-match path)
// and exist to hold the security edge once the pin exists.
//
// The pin contract under test:
//   - a Submit body may carry an optional `project` field naming an ACTIVE
//     registry entry the requester owns or is a member of; when valid it pins
//     the RegistrySlice directly — no name-token appearance in the request
//     text required (S06.2 injection; S13.7 "the registry feeds intake
//     resolution"; product map §5: the Projects-tab door must not depend on
//     the user typing the project's name);
//   - owner/member + ACTIVE validation is the security edge, enforced
//     server-side (S15.2: the browser is a display, never an authority);
//   - a body without the field submits exactly as before (S15.2
//     additive-first; the B6-7 `Inputs` precedent, commit 56b93c4).
//
// NOTE for the executor: this harness wires `registryOver`
// (round1_e2e_test.go), the inlined copy of the shell's production
// `registrySeam`. Whatever seam-side pin resolution lands must be mirrored
// there, or [RED] stays red for the wrong reason.

// pinFixture registers three registry entries directly at the store layer
// (the onboarding-as-task ceremony is exercised by TestOnboardingAsTaskE2E;
// the pin consumes registry DATA, so the data layer is the honest fixture):
//
//	shop  — ACTIVE, owner alice, member bob, name "shop backend"
//	attic — ACTIVE, owner dana, no members
//	draft — PENDING (captured, never activated), owner alice
func pinFixture(t *testing.T, h *projHarness) {
	t.Helper()
	ctx := context.Background()
	entries := []struct {
		id, owner, name string
		members         []string
		activate        bool
	}{
		{"shop", "alice", "shop backend", []string{"bob"}, true},
		{"attic", "dana", "attic", nil, true},
		{"draft", "alice", "draftproj", nil, false},
	}
	for _, e := range entries {
		if _, err := h.proj.Register(ctx, project.RegisterInput{
			ProjectID: e.id, Owner: e.owner, Name: e.name,
			StorePath: h.root + "/stores/" + e.id, Members: e.members,
		}); err != nil {
			t.Fatalf("Register %s: %v", e.id, err)
		}
		if _, err := h.proj.Capture(ctx, project.CaptureInput{
			ProjectID: e.id, By: e.owner,
			Conventions: []string{"tabs not spaces"},
			Commands:    project.Commands{Test: "go test ./..."},
		}); err != nil {
			t.Fatalf("Capture %s: %v", e.id, err)
		}
		if e.activate {
			if _, err := h.proj.Activate(ctx, e.id, e.owner); err != nil {
				t.Fatalf("Activate %s: %v", e.id, err)
			}
		}
	}
}

// submitPin submits a request whose body carries an explicit `project` field
// and whose TEXT never names any project (the Projects-tab door), and returns
// the born task's durable intake state, or the Submit error.
func submitPin(t *testing.T, h *projHarness, userID, projectField string) (*intake.State, error) {
	t.Helper()
	ctx := context.Background()
	body := fmt.Sprintf(
		`{"title":"wishlist","text":"add a wishlist option for signed-in shoppers","project":%q}`,
		projectField)
	raw, err := h.sur.Submit(ctx, userID, json.RawMessage(body))
	if err != nil {
		return nil, err
	}
	v := decodeView(t, raw)
	if v.TaskID == "" {
		t.Fatalf("no task id in %s", raw)
	}
	st, err := h.sk.Pipeline().LoadState(ctx, v.TaskID)
	if err != nil {
		t.Fatalf("LoadState(%s): %v", v.TaskID, err)
	}
	return st, nil
}

// TestProjectPinAtSubmit is the P3-RW-1 acceptance battery.
func TestProjectPinAtSubmit(t *testing.T) {
	h := newProjectHarness(t)
	pinFixture(t, h)
	ctx := context.Background()

	// [RED] AC-1/AC-2: the owner's pin injects the RegistrySlice with the
	// request text never naming the project — deterministic, no token match.
	t.Run("owner_pin_without_name_in_text", func(t *testing.T) {
		st, err := submitPin(t, h, "alice", "shop")
		if err != nil {
			t.Fatalf("Submit(pinned, owner): %v", err)
		}
		if st.Registry == nil || st.Registry.Project != "shop" {
			t.Fatalf("intake state registry = %+v, want pinned project %q "+
				"(P3-RW-1: the Projects-tab door must not depend on the user typing the project's name — product map §5)",
				st.Registry, "shop")
		}
	})

	// [RED] AC-3: a member's pin works identically ("owns/belongs" — the
	// MatchForIntake visibility semantics: owner OR invited member, S13.7).
	t.Run("member_pin", func(t *testing.T) {
		st, err := submitPin(t, h, "bob", "shop")
		if err != nil {
			t.Fatalf("Submit(pinned, member): %v", err)
		}
		if st.Registry == nil || st.Registry.Project != "shop" {
			t.Fatalf("intake state registry = %+v, want pinned project %q for member bob", st.Registry, "shop")
		}
	})

	// [GUARD] AC-4: a requester who neither owns nor belongs NEVER receives
	// the pin — the security edge (S15.2 server-side authority). Whether an
	// invalid pin rejects loudly or falls back to the text match is OQ1;
	// this assertion holds under every candidate disposition: the foreign
	// entry must not be pinned.
	t.Run("cross_user_pin_never_pins", func(t *testing.T) {
		st, err := submitPin(t, h, "alice", "attic")
		if err != nil {
			return // a loud rejection satisfies the invariant (OQ1 candidate a)
		}
		if st.Registry != nil && st.Registry.Project == "attic" {
			t.Fatalf("alice pinned dana's project: %+v — owner/member validation is the security edge", st.Registry)
		}
	})

	// [GUARD] AC-5: a PENDING entry never pins (S13.7: only active entries
	// feed intake resolution) — same OQ1-proof shape as AC-4.
	t.Run("pending_entry_never_pins", func(t *testing.T) {
		st, err := submitPin(t, h, "alice", "draft")
		if err != nil {
			return
		}
		if st.Registry != nil && st.Registry.Project == "draft" {
			t.Fatalf("a pending entry pinned: %+v — only ACTIVE entries feed intake (S13.7)", st.Registry)
		}
	})

	// [GUARD] AC-6: an unknown project id never yields a slice — same
	// OQ1-proof shape.
	t.Run("unknown_project_never_pins", func(t *testing.T) {
		st, err := submitPin(t, h, "alice", "no-such-project")
		if err != nil {
			return
		}
		if st.Registry != nil {
			t.Fatalf("an unknown pin produced a registry slice: %+v", st.Registry)
		}
	})

	// [GUARD] AC-7: the unpinned path is unchanged — the deterministic
	// name-token text match stays for bodies without the field (S15.2
	// additive-first; product map §5 "the text match stays for unpinned
	// submissions"). Red here after the executor lands = a regression.
	t.Run("unpinned_text_match_unchanged", func(t *testing.T) {
		raw, err := h.sur.Submit(ctx, "alice", json.RawMessage(
			`{"title":"checkout","text":"fix the shop backend checkout flow"}`))
		if err != nil {
			t.Fatalf("Submit(unpinned): %v", err)
		}
		v := decodeView(t, raw)
		st, err := h.sk.Pipeline().LoadState(ctx, v.TaskID)
		if err != nil {
			t.Fatalf("LoadState: %v", err)
		}
		if st.Registry == nil || st.Registry.Project != "shop" {
			t.Fatalf("unpinned text match broke: registry = %+v, want shop via the name token", st.Registry)
		}
	})
}

package project

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// P3-RW-1 (brief §8): the store-level validation sweep for the intake project
// pin — the security edge, tested in the package that OWNS the predicate
// (OQ3(i): PinForIntake is MatchForIntake's sibling, and `visibleTo` stays the
// single copy). The sweep is exhaustive over requester × entry-state × known,
// not a handful of examples, because the invariant it pins is universal:
//
//	for ALL (requester, entry): a pin NEVER resolves an entry where
//	!Active() || !(owner || member)          — Spec S13.7, S15.2.
//
// Colocated in a new file rather than appended to registry_test.go: the packet
// may not modify pre-existing test files.

// pinEntries registers the sweep fixture at the DATA layer (Register → Capture
// → Activate). The pin consumes registry data and needs a MEMBER, which the
// git-backed Onboard path does not take.
func pinEntries(t *testing.T, f *fix) {
	t.Helper()
	ctx := context.Background()
	for _, e := range []struct {
		id       string
		activate bool
	}{{"shop", true}, {"draft", false}} {
		if _, err := f.store.Register(ctx, RegisterInput{
			ProjectID: e.id, Owner: "alice", Name: e.id,
			StorePath: f.root + "/stores/" + e.id, Members: []string{"bob"},
		}); err != nil {
			t.Fatalf("Register %s: %v", e.id, err)
		}
		if _, err := f.store.Capture(ctx, CaptureInput{ProjectID: e.id, By: "alice"}); err != nil {
			t.Fatalf("Capture %s: %v", e.id, err)
		}
		if e.activate {
			if _, err := f.store.Activate(ctx, e.id, "alice"); err != nil {
				t.Fatalf("Activate %s: %v", e.id, err)
			}
		}
	}
}

// TestPinForIntakeVisibilitySweep walks every requester × pinned-id pair and
// asserts resolve-iff(visible AND active), plus the universal invariant on
// whatever comes back.
func TestPinForIntakeVisibilitySweep(t *testing.T) {
	f := newFix(t)
	pinEntries(t, f)
	ctx := context.Background()

	// The full cross product: 4 requester classes × 3 pinned ids.
	requesters := []struct{ name, user string }{
		{"owner", "alice"}, {"member", "bob"}, {"stranger", "carol"}, {"anonymous", ""},
	}
	pins := []struct{ name, id string }{
		{"active", "shop"}, {"pending", "draft"}, {"unknown", "ghost"},
	}
	for _, r := range requesters {
		for _, p := range pins {
			t.Run(r.name+"_"+p.name, func(t *testing.T) {
				visible := r.user == "alice" || r.user == "bob"
				e, err := f.store.PinForIntake(ctx, p.id, r.user)

				// The universal invariant, asserted on every outcome.
				if err == nil && (!e.Active() || !visibleTo(e, r.user)) {
					t.Fatalf("pin resolved a forbidden entry: id=%q requester=%q state=%q owner=%q members=%v",
						p.id, r.user, e.State, e.Owner, e.Members)
				}

				wantResolve := visible && p.id == "shop"
				if wantResolve {
					if err != nil {
						t.Fatalf("PinForIntake(%q, %q) = %v, want the active entry", p.id, r.user, err)
					}
					if e.ProjectID != "shop" {
						t.Fatalf("resolved entry = %q, want shop", e.ProjectID)
					}
					return
				}
				if err == nil {
					t.Fatalf("PinForIntake(%q, %q) resolved %q — want a refusal", p.id, r.user, e.ProjectID)
				}
				// Refusal CLASS: a visible-but-pending entry may say so; an
				// unknown id and an invisible entry are one indistinguishable
				// refusal (no existence leak).
				if visible && p.id == "draft" {
					if !errors.Is(err, ErrNotActive) {
						t.Fatalf("visible pending pin: err = %v, want ErrNotActive", err)
					}
					return
				}
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("unknown/invisible pin: err = %v, want ErrNotFound", err)
				}
			})
		}
	}
}

// TestPinForIntakeLeaksNoExistence: for the SAME requester, a project that
// exists but is not theirs must refuse exactly as an id that exists nowhere —
// otherwise the refusal itself is a project-existence oracle (S15.2).
func TestPinForIntakeLeaksNoExistence(t *testing.T) {
	f := newFix(t)
	pinEntries(t, f)
	ctx := context.Background()

	_, invisible := f.store.PinForIntake(ctx, "shop", "carol")
	_, unknown := f.store.PinForIntake(ctx, "ghost", "carol")
	if invisible == nil || unknown == nil {
		t.Fatalf("both pins must refuse: invisible=%v unknown=%v", invisible, unknown)
	}
	// Identical modulo the requester's own echoed id.
	a := strings.ReplaceAll(invisible.Error(), "shop", "PIN")
	b := strings.ReplaceAll(unknown.Error(), "ghost", "PIN")
	if a != b {
		t.Fatalf("refusals are distinguishable — an existence oracle:\n  invisible: %s\n  unknown:   %s", a, b)
	}
	// A pending entry the requester CANNOT see must also refuse as unknown.
	if _, err := f.store.PinForIntake(ctx, "draft", "carol"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("invisible pending pin: err = %v, want the unknown-shaped ErrNotFound", err)
	}
}

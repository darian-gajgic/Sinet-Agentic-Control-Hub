package review_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
)

// TestAcceptMovesToAcceptedAndRecords: the S13.6 state verb moves a deliverable
// to accepted, records one owner-attributed event, and exposes the accepted
// revision. It is idempotent (the class-A accept effect may replay).
func TestAcceptMovesToAcceptedAndRecords(t *testing.T) {
	f := newFix(t)
	f.seed()
	f.mint(1, "body v1")
	ctx := context.Background()

	res, err := f.store.Accept(ctx, "dlv-t1", "u1")
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if res.Deliverable.State != review.StateAccepted {
		t.Errorf("state = %q, want accepted", res.Deliverable.State)
	}
	if evs := f.events(review.EventAccepted); len(evs) != 1 || evs[0].UserID != "u1" {
		t.Errorf("want 1 owner-attributed accept event, got %+v", evs)
	}
	rev, err := f.store.AcceptedRevision(ctx, "dlv-t1")
	if err != nil || rev.N != 1 {
		t.Errorf("AcceptedRevision = rev %d, %v", rev.N, err)
	}

	// Idempotent replay: no second event, still accepted.
	if _, err := f.store.Accept(ctx, "dlv-t1", "u1"); err != nil {
		t.Fatalf("idempotent Accept: %v", err)
	}
	if evs := f.events(review.EventAccepted); len(evs) != 1 {
		t.Errorf("idempotent accept appended a second event: %d", len(evs))
	}
}

// TestAcceptRefusesNoRevision: a deliverable with no minted revision has
// nothing to accept.
func TestAcceptRefusesNoRevision(t *testing.T) {
	f := newFix(t)
	f.seed()
	if _, err := f.store.Accept(context.Background(), "dlv-t1", "u1"); !errors.Is(err, review.ErrBadInput) {
		t.Fatalf("accept with no revision: got %v, want ErrBadInput", err)
	}
}

// TestAcceptSupersedesPriorDefinition: a newly accepted definition (shared
// subject_ref) supersedes its prior accepted version (Spec S13.1).
func TestAcceptSupersedesPriorDefinition(t *testing.T) {
	f := newFix(t)
	f.task("t1", "u1")
	ctx := context.Background()
	// Two definition deliverables sharing one subject_ref.
	for _, id := range []string{"def-a", "def-b"} {
		if _, err := f.store.EnsureDeliverable(ctx, review.EnsureInput{
			ID: id, Owner: "u1", TaskID: "t1", Type: "worker-definition", SubjectRef: "worker/v1",
		}); err != nil {
			t.Fatalf("ensure %s: %v", id, err)
		}
		if _, err := f.store.MintRevision(ctx, review.MintInput{
			DeliverableID: id, N: 1, Files: map[string]string{"def.json": id},
		}); err != nil {
			t.Fatalf("mint %s: %v", id, err)
		}
	}
	if _, err := f.store.Accept(ctx, "def-a", "u1"); err != nil {
		t.Fatalf("accept def-a: %v", err)
	}
	res, err := f.store.Accept(ctx, "def-b", "u1")
	if err != nil {
		t.Fatalf("accept def-b: %v", err)
	}
	if len(res.Superseded) != 1 || res.Superseded[0] != "def-a" {
		t.Errorf("def-b accept superseded %v, want [def-a]", res.Superseded)
	}
	prior, _ := f.store.Deliverable(ctx, "def-a")
	if prior.State != review.StateSuperseded {
		t.Errorf("def-a state = %q, want superseded", prior.State)
	}
	// A superseded deliverable cannot be accepted.
	if _, err := f.store.Accept(ctx, "def-a", "u1"); !errors.Is(err, review.ErrBadInput) {
		t.Errorf("accepting a superseded deliverable: got %v, want ErrBadInput", err)
	}
}

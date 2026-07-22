package review_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
)

func TestMintLineageAndPins(t *testing.T) {
	f := newFix(t)
	d := f.seed()
	if d.CurrentRevision != 0 || d.State != review.StateInReview {
		t.Fatalf("fresh deliverable: %+v", d)
	}

	r1 := f.mint(1, "line one\nline two\n")
	r2 := f.mint(2, "line one\nline two\nline three\n")
	r3 := f.mint(3, "line one\nline 2\nline three\n")

	if r1.PlatformRef != "refs/sinet/deliverable/dlv-t1/rev-1" {
		t.Fatalf("platform ref namespace: %q", r1.PlatformRef)
	}
	if r1.PinKind != "content" || r1.ContentSHA256 == "" {
		t.Fatalf("content pin: %+v", r1)
	}

	d, err := f.store.Deliverable(context.Background(), "dlv-t1")
	if err != nil || d.CurrentRevision != 3 {
		t.Fatalf("current revision: %+v err %v", d, err)
	}

	revs, err := f.store.Revisions(context.Background(), "dlv-t1")
	if err != nil || len(revs) != 3 {
		t.Fatalf("lineage: %d revs, err %v", len(revs), err)
	}
	for i, r := range revs {
		if r.N != i+1 {
			t.Fatalf("lineage order: %+v", revs)
		}
	}

	// Round-trip: object-dir bytes hash-verify against the pin.
	files, err := f.store.RevisionFiles(context.Background(), "dlv-t1", 2)
	if err != nil || files["deliverable.md"] != "line one\nline two\nline three\n" {
		t.Fatalf("revision files: %v err %v", files, err)
	}

	// Any revision pair is diffable on demand (Spec S13.1) — including a
	// non-adjacent pair.
	cmp, err := f.store.Compare(context.Background(), "dlv-t1", 1, 3)
	if err != nil {
		t.Fatalf("Compare(1,3): %v", err)
	}
	if cmp.Surface != review.SurfaceLineDiff || cmp.Fallback {
		t.Fatalf("markdown compare surface: %+v", cmp)
	}
	if !strings.Contains(cmp.Unified, "+line 2") || !strings.Contains(cmp.Unified, "-line two") {
		t.Fatalf("unified diff content:\n%s", cmp.Unified)
	}
	_ = r2
	_ = r3

	// The minted events ride the verify run, owner-attributed.
	evs := f.events(review.EventMinted)
	if len(evs) != 3 {
		t.Fatalf("minted events: %d", len(evs))
	}
	for _, e := range evs {
		if e.RunID != "r1" || e.UserID != "u1" {
			t.Fatalf("minted event scope: %+v", e)
		}
	}
}

func TestMintImmutabilityAndContiguity(t *testing.T) {
	f := newFix(t)
	f.seed()
	f.mint(1, "content A\n")

	// Idempotent re-mint of the same revision with identical content (the
	// S07.7 resume re-entry).
	if _, err := f.store.MintRevision(context.Background(), review.MintInput{
		DeliverableID: "dlv-t1", N: 1, RunID: "r1", AttemptRef: "r1#round-1",
		Files: map[string]string{"deliverable.md": "content A\n"},
	}); err != nil {
		t.Fatalf("idempotent re-mint: %v", err)
	}

	// Different content under a minted number violates immutability.
	_, err := f.store.MintRevision(context.Background(), review.MintInput{
		DeliverableID: "dlv-t1", N: 1, RunID: "r1", AttemptRef: "r1#round-1",
		Files: map[string]string{"deliverable.md": "content B\n"},
	})
	if !errors.Is(err, review.ErrRevisionImmutable) {
		t.Fatalf("want ErrRevisionImmutable, got %v", err)
	}

	// Revisions are 1..N with every hop persisted: a gap refuses.
	_, err = f.store.MintRevision(context.Background(), review.MintInput{
		DeliverableID: "dlv-t1", N: 3, RunID: "r1", AttemptRef: "r1#round-3",
		Files: map[string]string{"deliverable.md": "content C\n"},
	})
	if !errors.Is(err, review.ErrBadInput) {
		t.Fatalf("want ErrBadInput on gap mint, got %v", err)
	}
}

func TestVerdictRefFillOnce(t *testing.T) {
	f := newFix(t)
	f.seed()
	f.mint(1, "content\n")
	if err := f.store.SetVerdictRef(context.Background(), "dlv-t1", 1, 41); err != nil {
		t.Fatalf("first fill: %v", err)
	}
	if err := f.store.SetVerdictRef(context.Background(), "dlv-t1", 1, 42); err == nil {
		t.Fatalf("second fill must refuse (fill-once)")
	}
	r, err := f.store.RevisionAt(context.Background(), "dlv-t1", 1)
	if err != nil || r.VerdictRef != 41 {
		t.Fatalf("verdict ref: %+v err %v", r, err)
	}
}

func TestContentDriftDetected(t *testing.T) {
	f := newFix(t)
	f.seed()
	rev := f.mint(1, "pinned content\n")

	// Corrupt the object-dir copy: reads must refuse, never serve drifted
	// bytes (the B2-5 hash-verify posture).
	path := f.store.ObjectPath(rev.Objects[0].SHA256)
	if err := os.WriteFile(path, []byte("tampered\n"), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	_, err := f.store.RevisionFiles(context.Background(), "dlv-t1", 1)
	if !errors.Is(err, review.ErrContentDrift) {
		t.Fatalf("want ErrContentDrift, got %v", err)
	}
}

func TestBinaryRevisionAndCards(t *testing.T) {
	f := newFix(t)
	f.task("t2", "u1")
	f.run("r2", "t2")
	_, err := f.store.EnsureDeliverable(context.Background(), review.EnsureInput{
		ID: "dlv-t2", Owner: "u1", TaskID: "t2", Type: "binary",
	})
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	mint := func(n int, payload string) {
		t.Helper()
		_, err := f.store.MintRevision(context.Background(), review.MintInput{
			DeliverableID: "dlv-t2", N: n, RunID: "r2", AttemptRef: "r2#round-1",
			Objects: map[string][]byte{"model.bin": []byte(payload)},
			Types:   map[string]string{"model.bin": "application/octet-stream"},
		})
		if err != nil {
			t.Fatalf("mint binary %d: %v", n, err)
		}
	}
	mint(1, "AAAA")
	mint(2, "BBBB")

	cmp, err := f.store.Compare(context.Background(), "dlv-t2", 1, 2)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if cmp.Surface != review.SurfaceBinaryCards || !cmp.Changed {
		t.Fatalf("binary compare: %+v", cmp)
	}
	card := cmp.NewObjects[0]
	if card.Name != "model.bin" || card.Size != 4 || card.SHA256 == "" || card.Type != "application/octet-stream" {
		t.Fatalf("metadata card fields: %+v", card)
	}
}

package intake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// P3-RW-1 drain r1 F2: a PINNED request never degrades. The requester named a
// specific project, so every way of failing to resolve it fails Start — the
// refusal classes (mapped 4xx) and an infrastructure failure of the store
// (an ordinary internal error) alike — and no task is born believing it got a
// project it did not. The UNPINNED scan path keeps today's degrade posture.

// erroringRegistry is a registry seam whose store is broken (the evaluator's
// probe: a closed database behind pin resolution).
type erroringRegistry struct{ err error }

func (r erroringRegistry) Match(context.Context, intake.Request) (intake.RegistrySlice, bool, error) {
	return intake.RegistrySlice{}, false, r.err
}

func countTaskRows(t *testing.T, f *fix) int {
	t.Helper()
	var n int
	if err := f.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM tasks`).Scan(&n); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	return n
}

func TestPinnedRequestNeverDegradesOnSeamError(t *testing.T) {
	ctx := context.Background()

	// An INFRASTRUCTURE error: not a refusal, so it must not masquerade as one
	// (it maps through mapIntakeErr's default, not a 4xx) — but it is still
	// loud, and it births nothing.
	t.Run("infrastructure_error", func(t *testing.T) {
		f := newFix(t)
		boom := errors.New("project: sql: database is closed")
		f.p.Registry = erroringRegistry{err: boom}

		_, err := f.p.Start(ctx, intake.Request{
			UserID: "alice", Text: "add a wishlist option", Project: "shop"})
		if !errors.Is(err, boom) {
			t.Fatalf("Start(pinned, broken store) = %v, want the store error propagated", err)
		}
		if errors.Is(err, intake.ErrPinRefused) {
			t.Fatalf("an infrastructure failure surfaced as a pin refusal: %v", err)
		}
		if n := countTaskRows(t, f); n != 0 {
			t.Fatalf("%d task row(s) born from a pinned request that could not be resolved", n)
		}
	})

	// A REFUSAL takes the same rule, keeping its sentinel for the 4xx mapping.
	t.Run("refusal", func(t *testing.T) {
		f := newFix(t)
		f.p.Registry = erroringRegistry{err: intake.ErrPinUnknown}

		_, err := f.p.Start(ctx, intake.Request{
			UserID: "alice", Text: "add a wishlist option", Project: "ghost"})
		if !errors.Is(err, intake.ErrPinRefused) {
			t.Fatalf("Start(refused pin) = %v, want a pin refusal", err)
		}
		if n := countTaskRows(t, f); n != 0 {
			t.Fatalf("%d task row(s) born from a refused pin", n)
		}
	})

	// UNPINNED: the S06.2 degrade posture is untouched — the identical seam
	// error is swallowed and the task is born with no slice.
	t.Run("unpinned_still_degrades", func(t *testing.T) {
		f := newFix(t)
		f.p.Registry = erroringRegistry{err: errors.New("project: sql: database is closed")}

		st, err := f.p.Start(ctx, intake.Request{UserID: "alice", Text: "add a wishlist option"})
		if err != nil {
			t.Fatalf("Start(unpinned, broken store) = %v, want the silent degrade", err)
		}
		if st.Registry != nil {
			t.Fatalf("a failed scan produced a slice: %+v", st.Registry)
		}
		if n := countTaskRows(t, f); n != 1 {
			t.Fatalf("task rows = %d, want the unpinned task born as before", n)
		}
	})
}

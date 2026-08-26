package run_test

// lanepin_ln9_test.go — P3-LN-9 R9 (S10.1): the run-row lane stamp.
//
// The columns are stamped at run BIRTH from process config, before routing has
// chosen anything, and nothing in the tree ever updated them — so a run that
// routed to a second lane still said it ran on the first. This is the verb that
// closes that, and what it must NOT do matters as much as what it does: in a
// world where the decision agrees with the row it writes nothing at all, which
// is what keeps the correction invisible where there is nothing to correct.
//
// $0: a temp database, no engine, no provider.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

func ln9Store(t *testing.T) *run.Store {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return run.NewStore(db, eventlog.New(db, reg))
}

func TestLN9SetDecidedLaneStampsTheRowAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := ln9Store(t)
	if _, err := s.Create(ctx, run.NewRun{
		ID: "r-ln9", UserID: "alice",
		Substrate: "claude-cli", Lane: "anthropic",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.SetDecidedLane(ctx, "r-ln9", "zai", "opencode"); err != nil {
		t.Fatalf("SetDecidedLane: %v", err)
	}
	got, err := s.Get(ctx, "r-ln9")
	if err != nil {
		t.Fatal(err)
	}
	if got.Lane != "zai" || got.Substrate != "opencode" {
		t.Fatalf("row = lane %q substrate %q, want zai/opencode — the row must name what RAN, not what the "+
			"process default was when it was born (R9)", got.Lane, got.Substrate)
	}
	// Idempotent: stamping the same pair again changes nothing and errors on
	// nothing. A resumed run re-drives its dispatch.
	if err := s.SetDecidedLane(ctx, "r-ln9", "zai", "opencode"); err != nil {
		t.Fatalf("SetDecidedLane (second time): %v", err)
	}
	again, err := s.Get(ctx, "r-ln9")
	if err != nil {
		t.Fatal(err)
	}
	if again.Lane != got.Lane || again.Substrate != got.Substrate {
		t.Errorf("a repeat stamp moved the row: %+v vs %+v", got, again)
	}

	// It is NOT a state change: the FSM, the generation and the lease are
	// untouched, which is the whole reason it carries no fence.
	if again.State != got.State || again.Generation != got.Generation {
		t.Errorf("the stamp moved the run's state or generation: %s/%d → %s/%d",
			got.State, got.Generation, again.State, again.Generation)
	}
}

// The no-op case is the one that keeps R9 a correction rather than a redesign:
// in a world where nothing is commissioned the decided lane EQUALS the
// configured one, and this verb must then not write a row at all.
func TestLN9SetDecidedLaneWritesNothingWhenNothingMoved(t *testing.T) {
	ctx := context.Background()
	s := ln9Store(t)
	if _, err := s.Create(ctx, run.NewRun{
		ID: "r-noop", UserID: "alice",
		Substrate: "claude-cli", Lane: "anthropic",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	before, err := s.Get(ctx, "r-noop")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetDecidedLane(ctx, "r-noop", "anthropic", "claude-cli"); err != nil {
		t.Fatalf("SetDecidedLane: %v", err)
	}
	after, err := s.Get(ctx, "r-noop")
	if err != nil {
		t.Fatal(err)
	}
	if after.Lane != before.Lane || after.Substrate != before.Substrate ||
		!after.UpdatedTS.Equal(before.UpdatedTS) {
		t.Errorf("a no-op stamp touched the row: %+v → %+v — the guard is in the statement so a world with "+
			"nothing to correct stays byte-identical", before, after)
	}

	// An empty decision is likewise inert: a run whose selection named no lane
	// must not have its row blanked.
	if err := s.SetDecidedLane(ctx, "r-noop", "", ""); err != nil {
		t.Fatalf("SetDecidedLane(empty): %v", err)
	}
	blank, err := s.Get(ctx, "r-noop")
	if err != nil {
		t.Fatal(err)
	}
	if blank.Lane != before.Lane || blank.Substrate != before.Substrate {
		t.Errorf("an empty decision blanked the row: %+v", blank)
	}
}

// TestLN9R1PartialStampNeverBlanksAStampedTruth — drain r1 F5.
//
// The first cut guarded the two values JOINTLY, skipping only when BOTH were
// empty. So the HALF-EMPTY shapes wrote through: a substrate with no lane
// blanked `runs.lane` — over a row that already named the lane that ran. The
// S08.8 gap leg zeroes the seat, so a decision carrying no lane is a real
// shape, and it is precisely the case where the row's own value is the better
// answer. An absent decision is an ABSENCE, never an instruction to forget.
func TestLN9R1PartialStampNeverBlanksAStampedTruth(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ name, lane, substrate string }{
		{"a substrate with no lane", "", "opencode"},
		{"a lane with no substrate", "zai", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := ln9Store(t)
			if _, err := s.Create(ctx, run.NewRun{
				ID: "r-partial", UserID: "alice",
				Substrate: "claude-cli", Lane: "anthropic",
			}); err != nil {
				t.Fatalf("Create: %v", err)
			}
			// The row already carries a stamped truth — the state R9 exists to
			// produce — so a partial call must not erase half of it.
			if err := s.SetDecidedLane(ctx, "r-partial", "zai", "opencode"); err != nil {
				t.Fatalf("SetDecidedLane (the real stamp): %v", err)
			}
			if err := s.SetDecidedLane(ctx, "r-partial", tc.lane, tc.substrate); err != nil {
				t.Fatalf("SetDecidedLane(%q, %q): %v", tc.lane, tc.substrate, err)
			}
			got, err := s.Get(ctx, "r-partial")
			if err != nil {
				t.Fatal(err)
			}
			if got.Lane != "zai" || got.Substrate != "opencode" {
				t.Fatalf("a partial stamp (%q, %q) left the row at lane %q / substrate %q — it overwrote a "+
					"value that already named what ran, which turns a correction into a data loss (r1 F5)",
					tc.lane, tc.substrate, got.Lane, got.Substrate)
			}
		})
	}
}

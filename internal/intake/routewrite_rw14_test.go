package intake

import "testing"

// P3-RW-14 R8, the load-bearing joint — drain D1.
//
// The write posture only reaches the S08.8 step-1 filter if routeQueryFor
// actually derives it from the plan. Every other R8 test hand-sets
// worker.RouteQuery.Writes, so they all stay green while this one line is
// reverted to `q.Writes = false` — and a reverted line silently restores the
// live defect in full: a plan whose every step declares a write_set handed to
// a worker that structurally cannot write, 7 sessions, $1.11, an empty tree.
//
// This is the $0 test that fails on that revert. It calls the derivation
// directly rather than through a walk, because the walk can route generalist
// for a dozen unrelated reasons and would keep passing with the signal dead.

func TestRW14RouteQueryCarriesThePlanWritePosture(t *testing.T) {
	st := &State{
		Owner: "u1", TaskID: "t-1", RunID: "t-1.intake",
		Req:    Request{TaskID: "t-1", UserID: "u1", Title: "Shop", Text: "Add a landing page."},
		Family: FamilySoftware,
	}
	plan := func(steps ...Step) *Pair {
		return &Pair{Plan: Plan{TaskID: "t-1", Owner: "u1", Steps: steps}}
	}

	cases := []struct {
		name string
		pair *Pair
		want bool
		why  string
	}{
		{
			name: "a declared write_set is a write requirement",
			pair: plan(Step{ID: "S-1", Class: "C2", WriteSet: []string{"app/**"}}),
			want: true,
		},
		{
			name: "an unbounded step claims the whole project (S02.8)",
			pair: plan(Step{ID: "S-1", Class: "C2", Unbounded: true}),
			want: true,
			why:  "a step that cannot bound its write-set writes the most, not the least",
		},
		{
			name: "one writing step among many is enough",
			pair: plan(
				Step{ID: "S-1", Class: "C1"},
				Step{ID: "S-2", Class: "C1", ReadSet: []string{"docs/**"}},
				Step{ID: "S-3", Class: "C2", WriteSet: []string{"templates/**"}},
			),
			want: true,
			why:  "the posture is the plan's union — routing equips for the whole plan, not step 1",
		},
		{
			name: "the LIVE shape: write_sets under a read-only class",
			pair: plan(Step{ID: "S-1", Class: "C1", WriteSet: []string{"app/**", "templates/**"}}),
			want: true,
			why: "the plan is self-contradictory (OQ4) and intake has no ratified check for it — " +
				"routing must still SEE the write requirement, which is what contains the blast",
		},
		{
			name: "a genuinely read-only plan requires nothing",
			pair: plan(
				Step{ID: "S-1", Class: "C1", ReadSet: []string{"**"}},
				Step{ID: "S-2", Class: "C1"},
			),
			want: false,
			why:  "refusing read-only workers a read-only plan would be a bug, not a fix",
		},
		{
			name: "an empty write_set is not a write claim",
			pair: plan(Step{ID: "S-1", Class: "C1", WriteSet: []string{}}),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := routeQueryFor(st, tc.pair)
			if got.Writes != tc.want {
				t.Fatalf("RouteQuery.Writes = %v, want %v — the S08.8 filter is blind to this plan's write posture%s",
					got.Writes, tc.want, suffix(tc.why))
			}
		})
	}
}

// The derivation must agree with the plan's own claim surface: whatever
// WriteGlobs reports is what routing is told, so the S02.8 W claim and the
// router can never disagree about whether this task writes.
func TestRW14RouteQueryWritesAgreesWithTheClaimSurface(t *testing.T) {
	st := &State{Owner: "u1", TaskID: "t-1", Req: Request{TaskID: "t-1", UserID: "u1"}}
	for _, steps := range [][]Step{
		{{ID: "S-1", WriteSet: []string{"a/**"}}},
		{{ID: "S-1", Unbounded: true}},
		{{ID: "S-1", ReadSet: []string{"a/**"}}},
		{{ID: "S-1"}, {ID: "S-2", WriteSet: []string{"b/**"}}},
		{},
	} {
		pair := &Pair{Plan: Plan{TaskID: "t-1", Steps: steps}}
		globs, unbounded := pair.Plan.WriteGlobs()
		claims := len(globs) > 0 || unbounded
		if got := routeQueryFor(st, pair).Writes; got != claims {
			t.Errorf("steps %+v: routing sees Writes=%v while the claim surface says %v", steps, got, claims)
		}
	}
}

func suffix(why string) string {
	if why == "" {
		return ""
	}
	return " (" + why + ")"
}

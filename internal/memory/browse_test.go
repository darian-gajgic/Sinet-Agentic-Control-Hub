package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
)

// browse_test.go — the three reads a human-facing browse surface needs
// (P3-B6-3C). The scope predicate is the load-bearing part: it decides what one
// person may see of a store several people write into.

func TestListVisibleIsOwnPlusHousePlusTheirProjects(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("op", "operator")
	f.user("alice", "member")
	f.user("bob", "member")

	alice := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindPreference, Title: "hers", Content: "c"})
	bob := mustCreate(t, f.gate, "bob", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindPreference, Title: "his", Content: "c"})
	house := mustCreate(t, f.gate, "op", memory.Draft{
		Scope: memory.ScopeHouse, Kind: memory.KindConvention, Title: "ours", Content: "c"})
	proj := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeProject, ScopeRef: "p-alpha", Kind: memory.KindPlaybook,
		Title: "alpha", Content: "c"})

	ids := func(viewer string, projects []string) map[string]bool {
		t.Helper()
		got, err := f.store.ListVisible(ctx, memory.BrowseQuery{
			Viewer: viewer, Projects: projects, Limit: 50})
		if err != nil {
			t.Fatalf("ListVisible(%s): %v", viewer, err)
		}
		out := map[string]bool{}
		for _, e := range got {
			out[e.ID] = true
		}
		return out
	}
	for _, tc := range []struct {
		viewer   string
		projects []string
		want     []string
		notWant  []string
	}{
		{"alice", []string{"p-alpha"}, []string{alice.ID, house.ID, proj.ID}, []string{bob.ID}},
		{"bob", []string{"p-alpha"}, []string{bob.ID, house.ID, proj.ID}, []string{alice.ID}},
		// No project membership: the project entry is out, house stays in.
		{"bob", nil, []string{bob.ID, house.ID}, []string{alice.ID, proj.ID}},
		// The operator has no special limb here: the role bit is not a
		// parameter of this read at all.
		{"op", nil, []string{house.ID}, []string{alice.ID, bob.ID, proj.ID}},
	} {
		got := ids(tc.viewer, tc.projects)
		for _, id := range tc.want {
			if !got[id] {
				t.Errorf("viewer %q (projects %v) must see %s", tc.viewer, tc.projects, id)
			}
		}
		for _, id := range tc.notWant {
			if got[id] {
				t.Errorf("viewer %q (projects %v) must NOT see %s", tc.viewer, tc.projects, id)
			}
		}
	}

	// The single-entry read and the set read share one SQL predicate (drain D3),
	// and this is where that agreement is checkable: for every viewer and every
	// entry, Visible answers exactly what ListVisible included.
	for _, viewer := range []struct {
		id       string
		projects []string
	}{{"alice", []string{"p-alpha"}}, {"bob", []string{"p-alpha"}}, {"bob", nil}, {"op", nil}} {
		inSet := ids(viewer.id, viewer.projects)
		for _, e := range []memory.Entry{alice, bob, house, proj} {
			got, err := f.store.Visible(ctx, e.ID, viewer.id, viewer.projects)
			if err != nil {
				t.Fatalf("Visible(%s, %s): %v", e.ID, viewer.id, err)
			}
			if got != inSet[e.ID] {
				t.Errorf("viewer %q (projects %v): Visible(%s) = %v but the list says %v — the two reads must be one rule",
					viewer.id, viewer.projects, e.ID, got, inSet[e.ID])
			}
		}
	}
	if _, err := f.store.Visible(ctx, alice.ID, "", nil); !errors.Is(err, memory.ErrInvalidEntry) {
		t.Errorf("Visible with no viewer: err = %v, want ErrInvalidEntry", err)
	}
	if seen, err := f.store.Visible(ctx, "k-doesnotexist", "alice", nil); err != nil || seen {
		t.Errorf("an unknown id is not visible: %v (%v)", seen, err)
	}

	// The filters narrow by exact stored value, and the bound is honored.
	byKind, err := f.store.ListVisible(ctx, memory.BrowseQuery{
		Viewer: "alice", Projects: []string{"p-alpha"}, Kind: memory.KindPlaybook, Limit: 50})
	if err != nil || len(byKind) != 1 || byKind[0].ID != proj.ID {
		t.Errorf("kind filter: %+v (%v)", byKind, err)
	}
	bounded, err := f.store.ListVisible(ctx, memory.BrowseQuery{
		Viewer: "alice", Projects: []string{"p-alpha"}, Limit: 1})
	if err != nil || len(bounded) != 1 {
		t.Errorf("limit 1 must return one row, got %d (%v)", len(bounded), err)
	}
	// Boundary refusals: an unscoped or unbounded read is not offered.
	for _, q := range []memory.BrowseQuery{{Limit: 10}, {Viewer: "alice"}} {
		if _, err := f.store.ListVisible(ctx, q); !errors.Is(err, memory.ErrInvalidEntry) {
			t.Errorf("ListVisible(%+v): err = %v, want ErrInvalidEntry", q, err)
		}
	}
}

func TestConflictReadsServeTheOpenEdgeToItsAddressee(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")

	first := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "a",
		Content: "always squash", TopicKey: "merge-style"})
	second := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "b",
		Content: "never squash", TopicKey: "merge-style"})

	// Both sides of the pair surface the one edge.
	for _, id := range []string{first.ID, second.ID} {
		got, err := f.store.EntryConflicts(ctx, id)
		if err != nil {
			t.Fatalf("EntryConflicts(%s): %v", id, err)
		}
		if len(got) != 1 || got[0].Affected != "alice" || got[0].Status != "open" || got[0].Question == "" {
			t.Fatalf("EntryConflicts(%s) = %+v", id, got)
		}
	}
	edges, err := f.store.EntryConflicts(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	one, err := f.store.Conflict(ctx, edges[0].ID)
	if err != nil || one.ID != edges[0].ID || one.EntryID != second.ID || one.OtherEntryID != first.ID {
		t.Fatalf("Conflict(%d) = %+v (%v)", edges[0].ID, one, err)
	}
	if _, err := f.store.Conflict(ctx, 999999); !errors.Is(err, memory.ErrNotFound) {
		t.Errorf("Conflict(unknown): err = %v, want ErrNotFound", err)
	}

	// A resolved edge leaves the open list — the read is what "open question"
	// means on a surface.
	if err := f.gate.ResolveConflict(ctx, "alice", one.ID); err != nil {
		t.Fatalf("ResolveConflict: %v", err)
	}
	if got, err := f.store.EntryConflicts(ctx, first.ID); err != nil || len(got) != 0 {
		t.Errorf("after resolution: %+v (%v)", got, err)
	}
	closed, err := f.store.Conflict(ctx, one.ID)
	if err != nil || closed.Status != "resolved" || closed.ResolvedBy != "alice" || closed.ResolvedTS == "" {
		t.Errorf("the closed edge records who answered and when: %+v (%v)", closed, err)
	}
}

// TestOpenConflictsForServesOnlyTheAddressee is the ADDRESSEE WALL on the
// B6-6 browse read that feeds the inbox's ninth card kind.
//
// The read exists because a decision surface has to answer "what is waiting on
// ME?", and the whole reason it is scoped by the addressee — rather than by
// entry, or by visibility — is that `mayAnswerConflict` is the ONE expression
// of both who may SEE an open edge and who may RESOLVE it. A read that handed
// over somebody else's question would put a card on a screen whose verb would
// refuse it, and would leak the other entry's title and topic while doing so.
//
// The direction that matters is the negative one, so it is driven with two
// people who each have a real conflict of their own: "returns nothing" and "was
// never scoped" look identical from outside when only one person has any rows.
func TestOpenConflictsForServesOnlyTheAddressee(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	f.user("alice", "member")
	f.user("bob", "member")
	f.user("op", "operator")

	alice := mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "a",
		Content: "always squash", TopicKey: "merge-style"})
	mustCreate(t, f.gate, "alice", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "b",
		Content: "never squash", TopicKey: "merge-style"})
	mustCreate(t, f.gate, "bob", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "c",
		Content: "rebase first", TopicKey: "rebase-style"})
	mustCreate(t, f.gate, "bob", memory.Draft{
		Scope: memory.ScopeUser, Kind: memory.KindLesson, Title: "d",
		Content: "never rebase", TopicKey: "rebase-style"})

	mine, err := f.store.OpenConflictsFor(ctx, "alice")
	if err != nil {
		t.Fatalf("OpenConflictsFor(alice): %v", err)
	}
	if len(mine) != 1 {
		t.Fatalf("alice has %d open conflicts, want her own 1: %+v", len(mine), mine)
	}
	if mine[0].Affected != "alice" || mine[0].Status != "open" || mine[0].Question == "" {
		t.Fatalf("the served edge is not alice's own open question: %+v", mine[0])
	}
	if mine[0].EntryID != alice.ID && mine[0].OtherEntryID != alice.ID {
		t.Errorf("the edge names neither of alice's entries: %+v", mine[0])
	}

	// Bob has a conflict of his OWN, so his read being disjoint from alice's is
	// a fact about the scoping rather than about an empty table.
	his, err := f.store.OpenConflictsFor(ctx, "bob")
	if err != nil {
		t.Fatalf("OpenConflictsFor(bob): %v", err)
	}
	if len(his) != 1 || his[0].Affected != "bob" {
		t.Fatalf("bob has %d open conflicts, want his own 1: %+v", len(his), his)
	}
	if his[0].ID == mine[0].ID {
		t.Error("alice and bob were served the SAME edge — the read is not addressee-scoped")
	}

	// The OPERATOR has no limb here, deliberately: this is the one read on the
	// platform where the role bit buys nothing, because the question is about
	// somebody's own stored knowledge and the resolve verb accepts only them.
	ops, err := f.store.OpenConflictsFor(ctx, "op")
	if err != nil {
		t.Fatalf("OpenConflictsFor(op): %v", err)
	}
	if len(ops) != 0 {
		t.Errorf("the operator was served %d conflicts addressed to other people: %+v", len(ops), ops)
	}

	// RESOLVED leaves the read: "open question" is what the surface means by a
	// waiting card, and the row moving is what takes the card away.
	if err := f.gate.ResolveConflict(ctx, "alice", mine[0].ID); err != nil {
		t.Fatalf("ResolveConflict: %v", err)
	}
	if got, err := f.store.OpenConflictsFor(ctx, "alice"); err != nil || len(got) != 0 {
		t.Errorf("a resolved conflict is still listed as open: %+v (%v)", got, err)
	}
	// …and it takes only her own with it.
	if got, err := f.store.OpenConflictsFor(ctx, "bob"); err != nil || len(got) != 1 {
		t.Errorf("alice's resolution changed bob's open list: %+v (%v)", got, err)
	}
}

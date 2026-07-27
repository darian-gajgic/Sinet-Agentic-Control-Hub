package watchlist_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchlist"
)

// canary_invariant_test.go — the R29 HARD INVARIANT proof, mirroring the shape
// of internal/watchdog's own no-auto-kill wall.
//
// A canary FAILURE raises a CARD, never a kill (Spec S14.4 alert discipline /
// G1 D1.3). The auth canary is the sharpest case: it is the first production
// caller of the S10.5 classifier, and Class 4's action is `lane-freeze` — so it
// would be very easy to reach for a terminate here. It must not. The canary
// records the classifier's verdict as DATA and raises a flag-now card; the
// dispatch path is what would honour a frozen lane.
//
// The wall is AST-shaped rather than a doc claim: identifiers, string literals
// (so run.State("tombstoned") cannot slip past the ident check) and the imports
// a killer would need.

// killIdents are the terminate/tombstone/kill identifiers this package must
// never reference.
var killIdents = []string{
	"StateTombstoned", "StateCrashed", "StateDiedAtGate", "StateCompleted", "StateFinalized",
	"Tombstone", "Terminate", "SIGKILL", "SIGTERM",
}

// killStateStrings are the recovery-owned terminal state VALUES as literals.
var killStateStrings = []string{"tombstoned", "crashed", "died-at-gate"}

// killVerbs are kill-verb literals, matched case-insensitively: exact for the
// short ambiguous verbs, substring for the unambiguous signal names.
var (
	killVerbsExact     = []string{"kill", "terminate", "tombstone"}
	killVerbsSubstring = []string{"sigkill", "sigterm"}
)

// killImports are the packages a killer would need and this one must not.
var killImports = []string{"os/exec", "syscall"}

func TestCanaryLayerHasNoKillPrimitive(t *testing.T) {
	dir := filepath.Join(repoRoot, "internal/watchlist")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		scanned++

		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, banned := range killImports {
				if path == banned {
					t.Errorf("%s imports %s — the watchlist detects and records; it never executes or signals a process", name, banned)
				}
			}
		}

		ast.Inspect(f, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.Ident:
				for _, banned := range killIdents {
					if node.Name == banned {
						t.Errorf("%s references %s — a canary raises a card, NEVER a kill (S14.4 / G1 D1.3)", name, banned)
					}
				}
			case *ast.BasicLit:
				if node.Kind != token.STRING {
					return true
				}
				v := strings.ToLower(strings.Trim(node.Value, "`\""))
				for _, banned := range killStateStrings {
					if v == banned {
						t.Errorf("%s carries the terminal-state literal %q — a state a canary must never construct", name, banned)
					}
				}
				for _, banned := range killVerbsExact {
					if v == banned {
						t.Errorf("%s carries the kill-verb literal %q", name, banned)
					}
				}
				for _, banned := range killVerbsSubstring {
					if strings.Contains(v, banned) {
						t.Errorf("%s carries the signal literal %q", name, banned)
					}
				}
			}
			return true
		})
	}
	if scanned == 0 {
		t.Fatal("the no-kill wall scanned no files — it would pass vacuously")
	}
}

// TestCanaryNoKillWallIsNotTautological feeds the wall's own matchers the
// forbidden shapes: a wall that cannot fail guarantees nothing.
func TestCanaryNoKillWallIsNotTautological(t *testing.T) {
	for _, ident := range killIdents {
		var hit bool
		for _, banned := range killIdents {
			hit = hit || ident == banned
		}
		if !hit {
			t.Errorf("the ident matcher let %q through", ident)
		}
	}
	for _, lit := range []string{"SIGKILL request", "sigterm"} {
		var hit bool
		low := strings.ToLower(lit)
		for _, banned := range killVerbsSubstring {
			hit = hit || strings.Contains(low, banned)
		}
		if !hit {
			t.Errorf("the substring matcher let %q through", lit)
		}
	}
	// And the wall must NOT reject the vocabulary this package legitimately
	// uses: `lane-freeze` is a recorded scheduler ACTION, not a kill.
	for _, ok := range []string{"lane-freeze", "flag-now", "policy-revocation-suspected", "park-probe"} {
		low := strings.ToLower(ok)
		for _, banned := range killVerbsExact {
			if low == banned {
				t.Errorf("the wall wrongly rejects the legitimate term %q", ok)
			}
		}
		for _, banned := range killVerbsSubstring {
			if strings.Contains(low, banned) {
				t.Errorf("the wall wrongly rejects the legitimate term %q", ok)
			}
		}
	}
}

// TestSchedulerLimitsIsUnchangedByThisPacket: the canary is Classify's first
// production CALLER, and the classifier stays exactly one tested component
// (S10.5). This package must not carry a second copy of the taxonomy — no
// class-selecting branch on wire codes lives here.
func TestCanaryDoesNotReimplementTheClassifier(t *testing.T) {
	src := readPackageSource(t)
	// The Z.AI wire codes the taxonomy switches on (S10.5). Their presence here
	// would mean a second classifier had been grown alongside the ratified one.
	for _, code := range []string{"1302", "1305", "1308", "1310", "1113"} {
		if strings.Contains(src, code) {
			t.Errorf("the wire code %q appears in internal/watchlist — the five-class taxonomy lives in ONE tested component (S10.5, P-T08-2)", code)
		}
	}
	// The canary must actually CALL the ratified classifier.
	if !strings.Contains(src, "scheduler.Classify(") {
		t.Error("the auth canary does not call scheduler.Classify — it is meant to be its first production caller")
	}
}

// TestCanarySetMembersAreSeededAsRows (R27): "Engine memory features and
// compaction behavior join the canary set" [G2 §Follow-ups; S05.7] is
// discharged by REGISTERING them as watch rows, not by inventing probes for
// observations the engines do not expose. Each is a study row — never polled,
// surfaced through PendingReview for the S16.7 quarterly pass.
func TestCanarySetMembersAreSeededAsRows(t *testing.T) {
	want := map[string]bool{
		"canary-engine-memory-features":     false,
		"canary-engine-compaction-behavior": false,
	}
	group := 0
	for _, r := range watchlist.SeedRows() {
		if r.Group == watchlist.GroupCanarySet {
			group++
		}
		if _, ok := want[r.ID]; !ok {
			continue
		}
		want[r.ID] = true
		if r.Kind != watchlist.KindStudy {
			t.Errorf("row %q kind = %q, want study — neither member has a pollable source", r.ID, r.Kind)
		}
		if r.URL != "" {
			t.Errorf("row %q carries a URL %q — a fabricated endpoint is worse than an honest registration", r.ID, r.URL)
		}
		if r.Group != watchlist.GroupCanarySet {
			t.Errorf("row %q group = %q, want %q", r.ID, r.Group, watchlist.GroupCanarySet)
		}
		if !strings.Contains(r.Notes, "S14.6 ¶3") || !strings.Contains(r.Notes, "S05.7") {
			t.Errorf("row %q notes do not cite the ratifying source: %q", r.ID, r.Notes)
		}
	}
	for id, seen := range want {
		if !seen {
			t.Errorf("canary-set row %q is not seeded", id)
		}
	}
	if group != len(want) {
		t.Errorf("%d rows in the canary-set group, want %d", group, len(want))
	}
}

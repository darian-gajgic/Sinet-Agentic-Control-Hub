package shell

// tq4c_slots_test.go — P3-TQ-4c brief R5, the dedupe proof at the composition:
// the owner kill (§80) is keyed on `check:<slot>` and the detected note on
// `check:detected:<slot>`; the two can never both exist for one slot because
// packFromCapture emits ONE rung per slot — the owner's when they captured one,
// the platform's detected command otherwise (S13.7 per-slot precedence,
// CONVENTIONS §79). GUARD: green today; it pins the fact the note's key relies
// on.

import (
	"testing"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

func TestTQ4cASlotIsTheOwnersOrDetectedNeverBoth(t *testing.T) {
	slots := []string{"lint", "build", "test"}
	commands := func(mask int) project.Commands {
		var c project.Commands
		if mask&1 != 0 {
			c.Lint = "eslint ."
		}
		if mask&2 != 0 {
			c.Build = "npm run build"
		}
		if mask&4 != 0 {
			c.Test = "npm test"
		}
		return c
	}
	for om := 0; om < 8; om++ {
		for dm := 0; dm < 8; dm++ {
			owner, det := commands(om), commands(dm)
			e := project.Entry{Name: "shop", Capture: project.Capture{
				Version: 3, Commands: owner, Detected: &det,
				CapturedTS: time.Now().UTC().Format(time.RFC3339Nano),
			}}
			pack, err := packFromCapture(verify.DomainSoftware, e)
			if err != nil {
				t.Fatalf("owner %03b detected %03b: packFromCapture: %v", om, dm, err)
			}
			ids := map[string]verify.Check{}
			for _, c := range pack.Checks {
				if _, dup := ids[c.ID]; dup {
					t.Fatalf("owner %03b detected %03b: duplicate rung id %q", om, dm, c.ID)
				}
				ids[c.ID] = c
			}
			for i, s := range slots {
				ownerHas, detHas := om&(1<<i) != 0, dm&(1<<i) != 0
				_, asOwner := ids[s]
				_, asDetected := ids["detected:"+s]
				if asOwner && asDetected {
					t.Fatalf("owner %03b detected %03b: slot %s is BOTH an owner rung and a detected rung — the kill key and the note key would coexist", om, dm, s)
				}
				if asOwner != ownerHas {
					t.Fatalf("owner %03b detected %03b: slot %s as owner rung = %v, want %v", om, dm, s, asOwner, ownerHas)
				}
				if asDetected != (detHas && !ownerHas) {
					t.Fatalf("owner %03b detected %03b: slot %s as detected rung = %v, want %v (detected only where the owner captured nothing)", om, dm, s, asDetected, detHas && !ownerHas)
				}
				if asOwner && ids[s].Origin != "" {
					t.Fatalf("owner rung %s carries origin %q", s, ids[s].Origin)
				}
				if asDetected && ids["detected:"+s].Origin != verify.ProvenanceDetected {
					t.Fatalf("detected rung %s carries origin %q", s, ids["detected:"+s].Origin)
				}
			}
		}
	}
}

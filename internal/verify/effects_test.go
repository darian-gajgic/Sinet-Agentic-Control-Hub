package verify_test

import (
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// S07.1/S02.7 acceptance: THE QUALITY GATE IS NEVER THE EFFECTS GATE. The
// sequence is quality verdict (V0–V2) → human accept (V3) → gated effect
// execution under D7. A SHIP verdict releases nothing outward, and the
// judge is never an implicit release authority. Proven two ways: an e2e run
// against a live effect-journal row, and a structural import check on the
// production sources (gates is imported by THIS TEST only).

func TestShipVerdictReleasesNoEffects(t *testing.T) {
	f := newFix(t)
	f.seedTask("t1", "r1")
	ctx := context.Background()

	// A proposed outward effect sits in the two-phase journal, waiting for
	// its HUMAN approval (D7).
	journal, err := gates.NewJournal(gates.JournalConfig{DB: f.db, Settings: f.reg})
	if err != nil {
		t.Fatalf("NewJournal: %v", err)
	}
	effect, err := journal.Propose(ctx, gates.Proposal{
		RunID: "r1", UserID: "u1", Class: gates.ClassC,
		Payload: json.RawMessage(`{"action":"create_github_comment","body":"done!"}`),
	})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}

	// The full drain to a SHIP verdict, verdict recorded, ledger item
	// verified.
	v := f.verifier(&fakeJudge{}, &scriptRunner{}, passPack())
	out, err := v.Verify(ctx, input(deliverable("t1", "r1")))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if out.Verdict != verify.VerdictShip || len(out.VerifiedItems) == 0 {
		t.Fatalf("drain outcome: %s %v", out.Verdict, out.VerifiedItems)
	}

	// The verdict released NOTHING: the effect row is byte-for-byte still
	// a proposal — unapproved, unexecuted, zero attempts.
	after, err := journal.Get(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if after.State != gates.EffectProposed {
		t.Fatalf("SHIP verdict moved an effect to %s — the quality gate acted as the effects gate", after.State)
	}
	if after.ApprovedBy != "" || after.Attempts != 0 || after.IdempotencyKey != "" {
		t.Fatalf("SHIP verdict touched the journal row: %+v", after)
	}
	// And no new effects appeared anywhere in the journal.
	for _, state := range []gates.EffectState{gates.EffectApproved, gates.EffectExecuting, gates.EffectSucceeded, gates.EffectFailed, gates.EffectUnknown} {
		rows, err := journal.InState(ctx, state)
		if err != nil {
			t.Fatalf("InState(%s): %v", state, err)
		}
		if len(rows) != 0 {
			t.Fatalf("verification produced %s effects: %+v", state, rows)
		}
	}
}

func TestVerdictRecordingTouchesNoEffectsTable(t *testing.T) {
	// Escalation cards — the loudest verification writes — also touch no
	// effect-journal row.
	f := newFix(t)
	f.seedTask("t1", "r1")
	ctx := context.Background()
	journal, err := gates.NewJournal(gates.JournalConfig{DB: f.db, Settings: f.reg})
	if err != nil {
		t.Fatalf("NewJournal: %v", err)
	}
	v := f.verifier(&fakeJudge{}, &scriptRunner{}, passPack())
	if _, err := v.RaiseSafety(ctx, deliverable("t1", "r1"), "planted", nil); err != nil {
		t.Fatalf("RaiseSafety: %v", err)
	}
	for _, state := range []gates.EffectState{gates.EffectProposed, gates.EffectApproved, gates.EffectExecuting} {
		rows, err := journal.InState(ctx, state)
		if err != nil {
			t.Fatalf("InState: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("escalation wrote a %s effect row", state)
		}
	}
}

func TestProductionCodeImportsNoGates(t *testing.T) {
	// Structural separation: the verify package's PRODUCTION sources never
	// import the effect journal — verdict recording cannot touch an
	// effects row even by accident. (This test file is the only importer.)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			if strings.Contains(imp.Path.Value, "internal/gates") {
				t.Fatalf("%s imports internal/gates — the quality gate must be structurally separate from the effects gate (Spec S07.1/S02.7)", name)
			}
		}
	}
}

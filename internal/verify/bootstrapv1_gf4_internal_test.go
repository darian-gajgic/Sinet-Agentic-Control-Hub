package verify

// bootstrapv1_gf4_internal_test.go — P3-GF4 properties P2 and P3 (Spec S07.8
// bootstrap posture, A14 2026-08-27), pinned over generated inputs rather than
// by example, because both are invariants the spec states universally: EVERY
// rung that would need the missing commands records UNVERIFIABLE-HERE, and
// NOTHING the posture mints may block.
//
// Deterministic generator (fixed seed, stdlib only): step counts 0..12 with
// arbitrary ids and "Done when" texts, over several seeds.

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// genSteps builds an arbitrary PLAN step list for the generator.
func genSteps(r *rand.Rand, n int) []intake.Step {
	steps := make([]intake.Step, 0, n)
	for i := 0; i < n; i++ {
		steps = append(steps, intake.Step{
			ID:       fmt.Sprintf("S-%d", r.Intn(1000)),
			Title:    fmt.Sprintf("step %d", i),
			DoneWhen: fmt.Sprintf("done when condition %d holds", r.Intn(1000)),
			Class:    "C1",
		})
	}
	return steps
}

// TestGF4PropBootstrapV1IsUniversallyUnverifiable is P2 [R3]: over arbitrary
// step lists, the bootstrap V1 result records every ladder rung and every
// step contract as UNVERIFIABLE-HERE with the stable attribution — never PASS
// and never absent, which are the two silent failures S07.8 forbids.
func TestGF4PropBootstrapV1IsUniversallyUnverifiable(t *testing.T) {
	for seed := int64(1); seed <= 8; seed++ {
		r := rand.New(rand.NewSource(seed))
		for n := 0; n <= 12; n++ {
			steps := genSteps(r, n)
			res := bootstrapV1(BootstrapPack(DomainSoftware, 1), steps)

			if len(res.Checks) == 0 {
				t.Fatalf("seed %d n=%d: no rung recorded at all — a silent skip", seed, n)
			}
			for _, c := range res.Checks {
				if c.State != CheckUnverifiable {
					t.Fatalf("seed %d n=%d: rung %q state %q, want UNVERIFIABLE-HERE", seed, n, c.CheckID, c.State)
				}
				if c.AttributedTo != BootstrapAttribution {
					t.Fatalf("seed %d n=%d: rung %q attributed to %q, want %q", seed, n, c.CheckID, c.AttributedTo, BootstrapAttribution)
				}
				if c.EvidenceRef != "" || c.ExitCode != 0 {
					t.Fatalf("seed %d n=%d: rung %q fabricated an execution record: %+v", seed, n, c.CheckID, c)
				}
			}
			if len(res.Steps) != len(steps) {
				t.Fatalf("seed %d n=%d: %d step contracts for %d steps — every step's contract is decided, including as undecidable", seed, n, len(res.Steps), len(steps))
			}
			for i, sc := range res.Steps {
				if sc.State != ContractUnverifiable {
					t.Fatalf("seed %d n=%d: step %q contract %q, want UNVERIFIABLE-HERE", seed, n, sc.StepID, sc.State)
				}
				if sc.AttributedTo != BootstrapAttribution {
					t.Fatalf("seed %d n=%d: step %q attributed to %q, want %q", seed, n, sc.StepID, sc.AttributedTo, BootstrapAttribution)
				}
				if sc.StepID != steps[i].ID || sc.DoneWhen != steps[i].DoneWhen {
					t.Fatalf("seed %d n=%d: contract %d is %+v, want the PLAN step %+v", seed, n, i, sc, steps[i])
				}
				if sc.Route == "" {
					t.Fatalf("seed %d n=%d: step %q declares no escalation route (Spec S07.3 stage-contract rule)", seed, n, sc.StepID)
				}
			}
			if res.StaleAudit {
				t.Fatalf("seed %d n=%d: bootstrap flagged a stale audit — no suite ran to be stale", seed, n)
			}
		}
	}
}

// TestGF4PropBootstrapMintsOnlyNotes is P3 [R4]: whatever the round shape, the
// posture machinery mints its disclosure and mints it note-class. A blocker
// here would make REVISE → cap → park unconditional, which is precisely the
// wall A14 abolishes.
func TestGF4PropBootstrapMintsOnlyNotes(t *testing.T) {
	for seed := int64(1); seed <= 8; seed++ {
		r := rand.New(rand.NewSource(seed))
		for n := 0; n <= 12; n++ {
			res := bootstrapV1(BootstrapPack(DomainSoftware, 1), genSteps(r, n))
			notes := 0
			for _, f := range res.Findings {
				if f.Severity == SeverityBlocker {
					t.Fatalf("seed %d n=%d: the posture minted a blocker: %+v", seed, n, f)
				}
				if f.Severity == SeverityNote {
					notes++
				}
			}
			if notes == 0 {
				t.Fatalf("seed %d n=%d: the posture minted no disclosure at all — the requester is never told why nothing was checked", seed, n)
			}
			key := bootstrapPostureFinding().Key()
			if got := res.Findings[0].Key(); got != key {
				t.Fatalf("seed %d n=%d: posture finding key %q is not the stable %q — note suppression would drop it after round 1", seed, n, got, key)
			}
		}
	}
}

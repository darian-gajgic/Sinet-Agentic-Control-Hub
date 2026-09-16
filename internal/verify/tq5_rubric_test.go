package verify_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
)

// tq5_rubric_test.go — P3-TQ-5 acceptance (Spec S07.9 P-T06-5, S07.10 "each
// rubric version pins its judge model", S07.11). The judge seat moves to
// claude-opus-5 (gate record P3/gates/rework-sitting-gate.md B7), which IS a
// rubric version bump: rubric-software v3 pins claude-opus-5 and carries rates
// re-measured on it (P3/measurements/2026-09-17-rider1-golden-set-opus5.md),
// never rates carried over from the opus-4-8 run.

func TestTQ5RubricV3PinsOpus5AndIsMeasuredOnIt(t *testing.T) {
	r := verify.SeedSoftwareRubric()
	if err := r.Validate(); err != nil {
		t.Fatalf("seed rubric invalid: %v", err)
	}
	if r.ID != "rubric-software" || r.Domain != verify.DomainSoftware || r.Version != 3 {
		t.Fatalf("rubric identity = %s/%s v%d, want rubric-software/software v3 (the judge change is a version bump, P-T06-5)", r.ID, r.Domain, r.Version)
	}
	// The gate is the one check that cannot disagree with the record: the pin
	// names claude-opus-5 at a token boundary AND the rates are measured and
	// dated.
	if err := verify.UnsupervisedJudgingGate(r, "claude-opus-5"); err != nil {
		t.Fatalf("v3 must drive unsupervised judging under claude-opus-5: %v", err)
	}
	if strings.Contains(r.JudgePin, "opus-4-8") {
		t.Errorf("the retired judge seat is still named in the v3 pin: %q", r.JudgePin)
	}
	if r.GoldenSet.MeasuredOn < "2026-09-17" {
		t.Errorf("golden-set rates dated %q — v3 must carry the re-run on claude-opus-5, not the 2026-07-22 opus-4-8 numbers", r.GoldenSet.MeasuredOn)
	}
	if r.GoldenSet.TNR == nil {
		t.Error("v3 carries no TNR — the clean controls were re-measured too")
	}
	if !strings.Contains(r.LengthBiasNote, "MEASURED") || !strings.Contains(r.LengthBiasNote, "claude-opus-5") {
		t.Errorf("length bias must be re-measured on the new judge (P-T06-3): %q", r.LengthBiasNote)
	}
	// Content untouched: the four axis-2 items in v2's order — only the pin and
	// its measurement moved.
	want := []string{"axis2/reasonable-user", "axis2/implicit-expectations", "axis2/side-effects", "axis2/expert-standard"}
	if len(r.Items) != len(want) {
		t.Fatalf("items = %d, want %d (v3 changes the pin, never the rubric content)", len(r.Items), len(want))
	}
	for i, id := range want {
		if r.Items[i].ID != id {
			t.Errorf("item %d = %q, want %q", i, r.Items[i].ID, id)
		}
	}
}

func TestTQ5RetiredJudgeSeatsAreRefusedByTheGate(t *testing.T) {
	r := verify.SeedSoftwareRubric()
	for _, seat := range []string{"claude-opus-4-8", "claude-sonnet-5"} {
		if err := verify.UnsupervisedJudgingGate(r, seat); !errors.Is(err, verify.ErrJudgeUnvalidated) {
			t.Errorf("retired seat %q must block unsupervised judging under the current rubric, got %v", seat, err)
		}
	}
}

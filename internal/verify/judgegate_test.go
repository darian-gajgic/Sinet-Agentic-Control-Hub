package verify_test

import (
	"errors"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
)

// P-T06-5 (Spec S14.8 ¶3): a judge-model change blocks unsupervised judging
// until the golden-set re-measurement passes. The gate is pure over the rubric
// bundle's own fields, so the proof is that the SAME bundle passes under the
// seat it was measured on and refuses under any other.

func TestUnsupervisedJudgingGateBlocksAJudgeChange(t *testing.T) {
	r := verify.SeedSoftwareRubric()

	// The shipped bundle was measured on the ratified seat: judging proceeds.
	if err := verify.UnsupervisedJudgingGate(r, "claude-opus-4-8"); err != nil {
		t.Fatalf("the measured rubric must judge under its own pinned seat: %v", err)
	}

	// A judge change blocks — this is the whole point of the pin.
	for _, seat := range []string{"claude-sonnet-5", "claude-haiku-4-5", "gpt-6"} {
		if err := verify.UnsupervisedJudgingGate(r, seat); !errors.Is(err, verify.ErrJudgeUnvalidated) {
			t.Errorf("seat %q must block unsupervised judging, got %v", seat, err)
		}
	}
	// A model id that merely PREFIXES the pinned one is not the pinned seat.
	if err := verify.UnsupervisedJudgingGate(r, "claude-opus-4"); !errors.Is(err, verify.ErrJudgeUnvalidated) {
		t.Errorf("a prefix of the pinned model must not satisfy the pin: %v", err)
	}
	if err := verify.UnsupervisedJudgingGate(r, ""); !errors.Is(err, verify.ErrJudgeUnvalidated) {
		t.Error("an unnamed seat must block")
	}

	// Unmeasured rates block even when the pin matches: an unmeasured judge
	// says so honestly rather than being trusted.
	unmeasured := *r
	unmeasured.GoldenSet = verify.GoldenSetRates{Measured: false}
	if err := verify.UnsupervisedJudgingGate(&unmeasured, "claude-opus-4-8"); !errors.Is(err, verify.ErrJudgeUnvalidated) {
		t.Errorf("unmeasured golden-set rates must block: %v", err)
	}
	// A "measured" claim without a date is not a measurement either.
	undated := *r
	undated.GoldenSet.MeasuredOn = ""
	if err := verify.UnsupervisedJudgingGate(&undated, "claude-opus-4-8"); !errors.Is(err, verify.ErrJudgeUnvalidated) {
		t.Errorf("an undated measurement must block: %v", err)
	}

	// The unblock path: a re-measurement against the new seat, carried on a new
	// rubric version (the S14.8 runbook owns both).
	remeasured := *r
	remeasured.Version = r.Version + 1
	remeasured.JudgePin = "claude-sonnet-5 (re-measured after the judge change)"
	remeasured.GoldenSet.MeasuredOn = "2026-09-01"
	if err := verify.UnsupervisedJudgingGate(&remeasured, "claude-sonnet-5"); err != nil {
		t.Fatalf("a passing re-measurement must clear the block: %v", err)
	}
}

package verify

import (
	"errors"
	"fmt"
	"strings"
)

// ErrJudgeUnvalidated is the P-T06-5 block: a judge-model change blocks
// unsupervised judging until a golden-set re-measurement passes (Spec S14.8 ¶3;
// judge logic S07.9). The block clears when the rubric version's own recorded
// rates are re-measured against the seat actually judging — a version bump the
// S14.8 revalidation runbook owns.
var ErrJudgeUnvalidated = errors.New("verify: unsupervised judging blocked until the golden-set re-measurement passes (P-T06-5)")

// UnsupervisedJudgingGate reports whether the rubric version may drive
// unsupervised judging under judgeSeat. It is a PURE function over the bundle's
// own fields — the pin it declares and the golden-set rates it carries — so the
// check can never disagree with the numbers the verdict record publishes
// (Spec S07.11). The enforcement point is the judge-construction site.
func UnsupervisedJudgingGate(r *RubricBundle, judgeSeat string) error {
	if r == nil {
		return fmt.Errorf("%w: no rubric bundle", ErrJudgeUnvalidated)
	}
	if strings.TrimSpace(judgeSeat) == "" {
		return fmt.Errorf("%w: rubric %s v%d would judge under an unnamed seat", ErrJudgeUnvalidated, r.ID, r.Version)
	}
	if !r.GoldenSet.Measured || r.GoldenSet.TPR == nil || r.GoldenSet.MeasuredOn == "" {
		return fmt.Errorf("%w: rubric %s v%d carries no dated golden-set measurement", ErrJudgeUnvalidated, r.ID, r.Version)
	}
	if !pinNamesSeat(r.JudgePin, judgeSeat) {
		return fmt.Errorf("%w: rubric %s v%d was measured against %q, but the judge seat is %q",
			ErrJudgeUnvalidated, r.ID, r.Version, r.JudgePin, judgeSeat)
	}
	return nil
}

// pinNamesSeat reports whether a rubric's judge pin names the seat. The pin is
// prose that states the model plus its ratification, so the match is on the
// model id at a token boundary — a prefix of a longer model id never counts as
// the seat.
func pinNamesSeat(pin, seat string) bool {
	for i := 0; ; {
		j := strings.Index(pin[i:], seat)
		if j < 0 {
			return false
		}
		start := i + j
		end := start + len(seat)
		if !modelIDByte(pin, start-1) && !modelIDByte(pin, end) {
			return true
		}
		i = start + 1
	}
}

// modelIDByte reports whether the byte at index i could continue a model id
// (letters, digits, '-', '.', '_'), i.e. whether a match at that edge is
// actually part of a longer identifier.
func modelIDByte(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i]
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '-', c == '.', c == '_':
		return true
	}
	return false
}

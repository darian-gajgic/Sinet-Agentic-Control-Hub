package local

import (
	"math"
	"testing"
)

// tok builds a TokenLogprob for one emitted piece with a top1/top2 gap. The
// chosen token IS top1; top2 is a runner-up `gap` below it.
func tok(text string, gap float64) TokenLogprob {
	return TokenLogprob{
		Token:   text,
		Logprob: -0.1,
		Top: []topLogprob{
			{Token: text, Logprob: -0.1},
			{Token: "<alt>", Logprob: -0.1 - gap},
		},
	}
}

func TestTokenMargin(t *testing.T) {
	m, ok := TokenMargin(tok("high", 2.0))
	if !ok || math.Abs(m-2.0) > 1e-9 {
		t.Fatalf("TokenMargin = %v, %v; want 2.0, true", m, ok)
	}
	if _, ok := TokenMargin(TokenLogprob{Token: "x"}); ok {
		t.Error("TokenMargin with no alternatives should report ok=false")
	}
}

func TestLabelMarginPicksLeastConfidentLabel(t *testing.T) {
	// {"reason":"ok","family":"software","stakes":"high"} — the value tokens are
	// their own pieces. family's gap is smaller (0.5) than stakes' (2.0), so the
	// min-over-labels margin is family's 0.5.
	lps := []TokenLogprob{
		tok(`{"reason":"`, 3.0),
		tok(`ok`, 3.0),
		tok(`","family":"`, 3.0),
		tok(`software`, 0.5),
		tok(`","stakes":"`, 3.0),
		tok(`high`, 2.0),
		tok(`"}`, 3.0),
	}
	m, ok := LabelMargin(lps, []string{"family", "stakes"})
	if !ok {
		t.Fatal("LabelMargin: expected a margin")
	}
	if math.Abs(m-0.5) > 1e-9 {
		t.Errorf("LabelMargin = %v, want 0.5 (the least-confident label)", m)
	}
}

func TestLabelMarginSingleField(t *testing.T) {
	lps := []TokenLogprob{
		tok(`{"reason":"`, 3.0),
		tok(`ok`, 3.0),
		tok(`","verdict":"`, 3.0),
		tok(`supported`, 1.25),
		tok(`"}`, 3.0),
	}
	m, ok := LabelMargin(lps, []string{"verdict"})
	if !ok || math.Abs(m-1.25) > 1e-9 {
		t.Errorf("LabelMargin = %v, %v; want 1.25, true", m, ok)
	}
}

func TestLabelMarginMissingFieldHonest(t *testing.T) {
	lps := []TokenLogprob{tok(`{"reason":"ok"}`, 3.0)}
	if _, ok := LabelMargin(lps, []string{"stakes"}); ok {
		t.Error("LabelMargin should report ok=false when no named field's value is located (never a fabricated confidence)")
	}
}

func TestLabelMarginEmptyInputs(t *testing.T) {
	if _, ok := LabelMargin(nil, []string{"x"}); ok {
		t.Error("empty logprobs should be ok=false")
	}
	if _, ok := LabelMargin([]TokenLogprob{tok("x", 1)}, nil); ok {
		t.Error("empty fields should be ok=false")
	}
}

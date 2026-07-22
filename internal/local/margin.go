package local

import "strings"

// margin.go — the Spec S12.5 confidence signal: the label-token logprob margin
// (top1−top2) under greedy decoding (brief R1). This is the ONLY supported
// cheap confidence signal — verbalized confidence from a 4–8B model is NEVER a
// routing input (there is no code path anywhere that reads a self-reported
// confidence field; the signal is structural). The margin computes from the
// top-2 logprobs the B4-5 client already requests (client.go TopLogprobs=2),
// post-hoc calibrated per duty (calibrate.go).
//
// A classification duty emits reason-first-then-constrained JSON
// (schema.go): the free-text `reason` region is high-entropy natural language
// and its margins are noise; the DECISIVE tokens are the enum label values.
// LabelMargin isolates them: it reconstructs the emitted text token-by-token,
// locates each named label field's FIRST value token (for an enum the first
// token commits the choice — "soft…" vs "gen…"), and takes the MINIMUM gap
// across the duty's label fields (the least-confident label decision — if any
// routing label was a coin-flip the whole classification is low-confidence).
// This same scalar is what calibration fits and what the live re-check gate
// reads, so the two never disagree (R4/R25).

// TokenMargin is one decoded token's top1−top2 logprob gap. Returns ok=false
// when the token carries fewer than two alternatives (no real choice was
// recorded, so no margin is defined).
func TokenMargin(t TokenLogprob) (margin float64, ok bool) {
	if len(t.Top) < 2 {
		return 0, false
	}
	// llama.cpp / the OpenAI surface return top_logprobs sorted descending, so
	// Top[0] is the chosen token and Top[1] the runner-up.
	return t.Top[0].Logprob - t.Top[1].Logprob, true
}

// LabelMargin returns the classification-confidence margin for a completion:
// the minimum, across the named label fields, of the top1−top2 gap at each
// field's first value token. ok=false when no named field's value token could
// be located (the margin is unavailable — recorded honestly, never a
// fabricated confidence, R1). fields is the duty's routing-decisive label set
// (the caller owns it; the local package owns the extraction).
func LabelMargin(logprobs []TokenLogprob, fields []string) (margin float64, ok bool) {
	if len(logprobs) == 0 || len(fields) == 0 {
		return 0, false
	}
	// Reconstruct the emitted text and remember each token's start offset so a
	// located value position maps back to the token that produced it.
	var text strings.Builder
	starts := make([]int, len(logprobs))
	for i, t := range logprobs {
		starts[i] = text.Len()
		text.WriteString(t.Token)
	}
	full := text.String()

	found := false
	worst := 0.0
	for _, field := range fields {
		off, located := valueStart(full, field)
		if !located {
			continue
		}
		tokIdx := tokenAt(starts, off)
		if tokIdx < 0 {
			continue
		}
		m, has := TokenMargin(logprobs[tokIdx])
		if !has {
			continue
		}
		if !found || m < worst {
			worst = m
			found = true
		}
	}
	return worst, found
}

// valueStart finds the character offset of the first value character of the
// JSON member `"field": <value>` in text, and whether it was found. It handles
// the string-value case (`"field":"value"` → the offset of the first char
// INSIDE the opening quote) and the general case (the first non-space char
// after the colon). Conservative: a field name that does not appear as a
// quoted key returns not-found.
func valueStart(text, field string) (int, bool) {
	key := `"` + field + `"`
	ki := strings.Index(text, key)
	if ki < 0 {
		return 0, false
	}
	i := ki + len(key)
	// skip whitespace, then the required colon
	for i < len(text) && isJSONSpace(text[i]) {
		i++
	}
	if i >= len(text) || text[i] != ':' {
		return 0, false
	}
	i++ // past the colon
	for i < len(text) && isJSONSpace(text[i]) {
		i++
	}
	if i >= len(text) {
		return 0, false
	}
	if text[i] == '"' {
		i++ // first char inside the string value — the decisive enum token
	}
	if i >= len(text) {
		return 0, false
	}
	return i, true
}

func isJSONSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

// tokenAt returns the index of the token whose span covers character offset
// off, or -1. starts[i] is token i's start offset; token i spans
// [starts[i], starts[i+1]).
func tokenAt(starts []int, off int) int {
	for i := 0; i < len(starts); i++ {
		next := 1 << 62
		if i+1 < len(starts) {
			next = starts[i+1]
		}
		if off >= starts[i] && off < next {
			return i
		}
	}
	return -1
}

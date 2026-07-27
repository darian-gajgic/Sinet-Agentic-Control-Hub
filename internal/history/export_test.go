package history

// export_test.go — test hooks for the external history_test package.
//
// These exist so a guardrail limb can be asserted ON ITS OWN rather than only
// through a path where an earlier limb would mask it (drain r2 R4). They are
// compiled only into the test binary, so they add nothing to the package's API.

// QuoteIdentForTest exposes quoteIdent, the defensive identifier-quoting limb
// behind checkAlias.
func QuoteIdentForTest(s string) string { return quoteIdent(s) }

// DropMarkerOnlyHitsForTest exposes the marker-only row verification — the
// load-bearing half of the codor-C2 search property (drain r2 R3).
func DropMarkerOnlyHitsForTest(a Answer, terms []string) [][]any {
	return dropMarkerOnlyHits(a, terms)
}

// CheckOffsetsForTest exposes the fail-closed offset belt with explicit spans.
// The belt is unreachable while the rewrite's arithmetic is right, so only a
// direct call can assert that a stale span REFUSES rather than reaching the
// slice that would panic on it.
func CheckOffsetsForTest(src string, spans [][2]int) error {
	targets := make([]target, 0, len(spans))
	for _, s := range spans {
		targets = append(targets, target{start: s[0], end: s[1]})
	}
	return checkOffsets(src, targets)
}

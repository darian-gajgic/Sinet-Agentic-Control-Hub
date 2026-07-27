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

package history

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/redact"
)

// search.go — the codor-C2 SEARCH obligation (CONVENTIONS §30 final bullet:
// "redact-before-match is B5-8's"), and the S14.10 preamble's "everything above
// is filterable and SEARCHABLE".
//
// THE PROPERTY, and where it actually lives. A full-text index over RAW text is
// an oracle however the reader is written: a MATCH on a plaintext secret
// confirms its presence without ever returning it. So the property cannot be
// bought at query time, and B5-8A bought it at write time instead — every
// string entering `history_fts` passes through internal/redact FIRST, per
// individual string value (§36 drain D4). This layer inherits that corpus and
// adds the two things a SEARCH SURFACE owes on top of it:
//
//  1. THE QUERY IS REDACTED BEFORE IT IS MATCHED, AND WHAT REDACTION REMOVED
//     IS NOT SEARCHED FOR. A question carrying a secret never reaches the
//     database as plaintext — not into the statement, not into a driver log,
//     not into an error string. Redaction alone is not enough here, which is
//     the finding this layer was built against: redacting `sk-ant-…` yields
//     `[REDACTED:anthropic_key]`, and the CORPUS is full of that same marker,
//     so a redacted query would match every record carrying any key of that
//     class — a weak oracle, but an oracle, and one that reads to the asker as
//     confirmation of the exact secret they typed. So the markers are STRIPPED
//     from the query before it is tokenized: a secret contributes no search
//     term at all. A question that was nothing but a secret matches nothing and
//     says so.
//  2. EXCERPTS ARE BOUNDED AND REDACTED. Snippets come from the already-
//     redacted corpus, are capped, and pass through the primitive again on the
//     way out — belt and braces, at zero cost on honest text.
//
// STORE-RAW / SERVE-REDACTED holds (§30 R19): nothing here writes, and
// run_events.payload keeps the verbatim body. NEVER DELIVERABLE CONTENT
// (§7-C2·2): this package serves observability history only; the deliverable/
// review/preview/accept paths do not import the primitive and this package
// serves none of their content.
//
// RECORDED READING on the seam with B5-8A. internal/retention.Search stays
// retention's OWN read over its own index — the projector's refs-only
// primitive. The S14.10 search LAYER reads the same corpus here rather than
// through it, because internal/history's import wall admits storage, eventlog,
// settings, local and redact, and internal/retention is not on that list; the
// C2 property belongs to the CORPUS both read, not to either reader, which is
// exactly what makes reading it twice safe.

// Search structural constants (S18 ratifies no key; §35 precedent).
const (
	// SearchExcerptRunes bounds one excerpt. Its reason: an excerpt exists to
	// let a person recognize a hit, not to serve the record — the record is
	// read from its own surface by the ref this returns. A bound also caps how
	// much of any single row one query can drain, which matters for a corpus
	// that is redacted rather than secret-free.
	SearchExcerptRunes = 240
	// searchSnippetTokens is the FTS5 snippet window, in tokens, sized to land
	// under the rune cap on ordinary prose so the cap rarely truncates.
	searchSnippetTokens = 24
	// searchMaxTerms bounds how many terms one query may match on. Its reason:
	// an FTS5 MATCH cost grows with the term count, and a question long enough
	// to exceed this is not a search, it is a paste.
	searchMaxTerms = 16
)

// SearchQueryName is the name a search answer carries (R26: every answer
// renders with its query named, including this one).
const SearchQueryName = "search.history"

// Search runs a redact-before-match full-text search over the indexed history
// — run summaries, verdict texts and drift summaries (S14.10 ¶4) — owner-scoped
// per S01.9.
//
// Columns are {event_seq, kind, ref, user_id, excerpt}. The excerpt is bounded
// and redacted; the REF is what a caller renders the record from.
func (s *Store) Search(ctx context.Context, question string, scope Scope, limit int) (Answer, error) {
	a := newAnswer(LayerDeterministic, SearchQueryName)
	a.Question = question
	a.Notes = append(a.Notes,
		"matched against the redacted corpus (codor C2): a query for a secret's plaintext cannot confirm the secret",
		"excerpts are bounded and redacted; render the record from its ref")

	// (1) REDACT BEFORE MATCH, then drop what redaction removed. Note the
	// ordering: redaction runs on the raw question, the markers it leaves are
	// stripped, and only what remains is ever tokenized into the MATCH
	// expression. redact.Redact is documented for individual string values,
	// which is exactly what a question is.
	redacted := redact.Redact(question)
	if redacted != question {
		a.Notes = append(a.Notes,
			"the question carried a secret-shaped value; it was redacted and contributes no search term")
	}
	match, terms := matchExpression(stripMarkers(redacted))
	if len(terms) == 0 {
		a.Notes = append(a.Notes, "the question carried no searchable term")
		return a, nil
	}

	limit = clampLimit(limit)
	stmt := `SELECT rowid, kind, ref, user_id, snippet(history_fts, 0, '', '', '…', ?) AS excerpt, body AS match_check` +
		` FROM history_fts WHERE history_fts MATCH ?` +
		OwnerScopeOpen + `user_id` + OwnerScopeClose +
		` ORDER BY rank, rowid DESC LIMIT ?`
	args := []any{searchSnippetTokens, match, scope.scopeArg(), scope.scopeArg(), limit + 1}

	if err := s.query(ctx, &a, stmt, args, limit); err != nil {
		return Answer{}, err
	}

	// (1b) MARKER-ONLY HITS ARE DROPPED (drain D3). Stripping markers from the
	// QUERY closes the accidental path but not the deliberate one: the corpus
	// holds `[REDACTED:anthropic_key]` as ORDINARY TOKENS, so `REDACTED
	// anthropic_key`, `[REDACTED:anthropic_key` and `REDACTED:anthropic_key]`
	// all tokenized into terms that matched the marker itself — a class-level
	// oracle, measured returning 1 row each.
	//
	// Censoring those words out of every query would be the obvious fix and the
	// wrong one: "key", "session", "github" and "token" are ordinary words, and
	// a search surface that cannot find them is broken to close a leak. So the
	// MATCH is allowed to over-return and the ROW IS VERIFIED instead — a hit
	// survives only if some query term still appears once the markers are gone.
	// A row whose only "key" is inside a redaction marker is not a hit for
	// "key", and a row that genuinely says "key" still is.
	a.Rows = dropMarkerOnlyHits(a, terms)
	dropColumn(&a, "match_check")
	a.RowCount = len(a.Rows)

	// (2) BOUND AND RE-REDACT THE EXCERPTS. The corpus is already redacted, so
	// this is a second pass over inert text on the honest path; it is here so
	// the surface's guarantee does not rest on a promise made by another
	// package's write path.
	col := columnIndex(a.Columns, "excerpt")
	if col >= 0 {
		for _, row := range a.Rows {
			if txt, ok := row[col].(string); ok {
				row[col] = boundExcerpt(redact.Redact(txt))
			}
		}
	}
	return a, nil
}

// markerPattern matches internal/redact's inert marker AND its fragments. It is
// derived from the marker SHAPE (`[REDACTED:<class>]`), so a new secret class
// added to the primitive is handled here without this file changing.
//
// The brackets and the colon are all OPTIONAL (drain D3): a question does not
// have to spell the marker correctly to be asking about it, and
// `REDACTED anthropic_key`, `[REDACTED:anthropic_key` and
// `REDACTED:anthropic_key]` were each measured returning a hit against a
// well-formed marker in the corpus.
var markerPattern = regexp.MustCompile(`(?i)\[?\s*REDACTED\s*:?\s*[a-z_]*\s*\]?`)

// stripMarkers removes the redaction markers and their fragments so they never
// become search terms. This is the difference between "the secret is not in the
// corpus" (B5-8A's property) and "asking for the secret confirms nothing"
// (this layer's).
//
// It is the FIRST of two limbs; the second is dropMarkerOnlyHits, because a
// query need not name the marker at all to match one.
func stripMarkers(s string) string { return markerPattern.ReplaceAllString(s, " ") }

// dropMarkerOnlyHits keeps a row only if a query term survives in its body once
// the redaction markers are stripped out of it.
//
// The body column is read for this check and then discarded — it is already
// redacted (B5-8A's corpus property), so this reads inert text, and it never
// reaches the caller: only the bounded, re-redacted excerpt does.
func dropMarkerOnlyHits(a Answer, terms []string) [][]any {
	check := columnIndex(a.Columns, "match_check")
	if check < 0 {
		return a.Rows
	}
	kept := make([][]any, 0, len(a.Rows))
	for _, row := range a.Rows {
		body, _ := row[check].(string)
		stripped := strings.ToLower(stripMarkers(body))
		for _, term := range terms {
			if strings.Contains(stripped, term) {
				kept = append(kept, row)
				break
			}
		}
	}
	return kept
}

// matchExpression turns free text into a safe FTS5 MATCH expression and reports
// how many terms it carries.
//
// It is a whitelist, not an escape: only letters and digits survive, each term
// is phrase-quoted, and terms are joined by an explicit OR. That disposes of
// the whole FTS5 query-syntax surface at once — column filters, NEAR, prefix
// stars, boolean operators, and the redaction markers' own punctuation, which
// would otherwise be parsed as syntax rather than matched as text.
// It returns the bare terms alongside the expression, because the marker-only
// verification (dropMarkerOnlyHits) has to know what was actually asked for.
func matchExpression(text string) (string, []string) {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !('a' <= r && r <= 'z') && !('0' <= r && r <= '9')
	})
	var terms, quoted []string
	seen := map[string]bool{}
	for _, f := range fields {
		if len(f) < 2 || seen[f] {
			continue
		}
		seen[f] = true
		terms = append(terms, f)
		quoted = append(quoted, `"`+f+`"`)
		if len(terms) >= searchMaxTerms {
			break
		}
	}
	if len(terms) == 0 {
		return "", nil
	}
	return strings.Join(quoted, " OR "), terms
}

// dropColumn removes a working column from an answer, so a column read only to
// VERIFY a hit never becomes part of what the hit returns.
func dropColumn(a *Answer, name string) {
	i := columnIndex(a.Columns, name)
	if i < 0 {
		return
	}
	a.Columns = append(a.Columns[:i:i], a.Columns[i+1:]...)
	for r, row := range a.Rows {
		a.Rows[r] = append(row[:i:i], row[i+1:]...)
	}
}

// boundExcerpt caps an excerpt at SearchExcerptRunes, marking the truncation so
// a reader never mistakes a cut for the end of the text.
func boundExcerpt(s string) string {
	if utf8.RuneCountInString(s) <= SearchExcerptRunes {
		return s
	}
	n := 0
	for i := range s {
		if n == SearchExcerptRunes {
			return s[:i] + "…(excerpt bounded)"
		}
		n++
	}
	return s
}

func columnIndex(cols []string, name string) int {
	for i, c := range cols {
		if c == name {
			return i
		}
	}
	return -1
}

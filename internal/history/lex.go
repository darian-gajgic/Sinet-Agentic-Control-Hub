package history

import (
	"errors"
	"fmt"
	"strings"
)

// lex.go — a small SQLite-dialect lexer, stdlib only.
//
// It exists because every check the guardrail makes is a check about WORDS,
// and a substring scan cannot tell a word from a fragment: `updated_ts` is not
// an update, `'drop the table'` inside a string literal is not a schema
// change, and `-- delete everything` inside a comment is not a write. A
// guardrail built on substring matching produces both false refusals and — far
// worse — false acceptances, because a hostile statement can hide a word from a
// substring scan by quoting or commenting AROUND it.
//
// It is a lexer, not a parser: it answers "what are the words, and where", and
// guard.go answers "is this statement allowed". A third-party SQL parser would
// be an S16 adoption requiring the §4 checklist and operator approval, and is
// not proposed — the checks needed here are lexical.
//
// COMMENTS ARE DISCARDED, which is the point: a comment-hidden second statement
// stops being hidden once the comment is gone, and the statement separator it
// was hiding behind becomes visible to the single-statement check.

type tokenKind int

const (
	tokIdent tokenKind = iota // identifier or keyword (they are the same lexically)
	tokNumber
	tokString // a literal; its CONTENT is never interpreted
	tokPunct
	tokParam // ?, ?NNN, :name, @name, $name
)

type token struct {
	kind  tokenKind
	text  string // as written, with any quoting removed for identifiers
	lower string // lowercased text, for word comparison
	// quoted marks an identifier that was written in quotes/brackets/backticks.
	// A quoted word is an IDENTIFIER by the author's own declaration, so it is
	// never treated as a statement keyword — and never as a table name the
	// allowlist would accept, either.
	quoted     bool
	start, end int // byte offsets into the source
}

// ErrLex reports text that cannot be lexed at all.
var ErrLex = errors.New("history: the statement could not be read as SQL")

// lex tokenizes one statement. Whitespace and comments are dropped.
func lex(src string) ([]token, error) {
	var out []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
			i++
		case c == '-' && i+1 < len(src) && src[i+1] == '-':
			// Line comment, to end of line.
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				i = len(src)
			} else {
				i += j + 1
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				// An unterminated block comment would swallow the rest of the
				// statement; refusing beats guessing where it ends.
				return nil, fmt.Errorf("%w: an unterminated block comment", ErrLex)
			}
			i += 2 + j + 2
		case c == '\'':
			end, err := closeQuote(src, i, '\'')
			if err != nil {
				return nil, err
			}
			out = append(out, token{kind: tokString, text: src[i:end], lower: strings.ToLower(src[i:end]), start: i, end: end})
			i = end
		case c == '"' || c == '`':
			end, err := closeQuote(src, i, c)
			if err != nil {
				return nil, err
			}
			inner := src[i+1 : end-1]
			out = append(out, token{kind: tokIdent, text: inner, lower: strings.ToLower(inner), quoted: true, start: i, end: end})
			i = end
		case c == '[':
			j := strings.IndexByte(src[i:], ']')
			if j < 0 {
				return nil, fmt.Errorf("%w: an unterminated bracket identifier", ErrLex)
			}
			inner := src[i+1 : i+j]
			out = append(out, token{kind: tokIdent, text: inner, lower: strings.ToLower(inner), quoted: true, start: i, end: i + j + 1})
			i += j + 1
		case c == '?' || c == ':' || c == '@' || c == '$':
			j := i + 1
			for j < len(src) && isIdentByte(src[j]) {
				j++
			}
			// A bare ':' is punctuation (it appears in no SQLite expression we
			// accept, but it is not a parameter either).
			if j == i+1 && c != '?' {
				out = append(out, token{kind: tokPunct, text: string(c), lower: string(c), start: i, end: j})
			} else {
				out = append(out, token{kind: tokParam, text: src[i:j], lower: strings.ToLower(src[i:j]), start: i, end: j})
			}
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < len(src) && (isIdentByte(src[j]) || src[j] == '.') {
				j++
			}
			out = append(out, token{kind: tokNumber, text: src[i:j], lower: strings.ToLower(src[i:j]), start: i, end: j})
			i = j
		case isIdentByte(c):
			j := i
			for j < len(src) && isIdentByte(src[j]) {
				j++
			}
			out = append(out, token{kind: tokIdent, text: src[i:j], lower: strings.ToLower(src[i:j]), start: i, end: j})
			i = j
		default:
			out = append(out, token{kind: tokPunct, text: string(c), lower: string(c), start: i, end: i + 1})
			i++
		}
	}
	return out, nil
}

// closeQuote returns the offset one past the closing quote, honoring the
// doubled-quote escape SQLite uses.
func closeQuote(src string, start int, q byte) (int, error) {
	for i := start + 1; i < len(src); i++ {
		if src[i] != q {
			continue
		}
		if i+1 < len(src) && src[i+1] == q {
			i++ // a doubled quote is an escaped quote, not a close
			continue
		}
		return i + 1, nil
	}
	return 0, fmt.Errorf("%w: an unterminated %c-quoted literal", ErrLex, q)
}

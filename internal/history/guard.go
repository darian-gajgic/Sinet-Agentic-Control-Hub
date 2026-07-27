package history

import (
	"errors"
	"fmt"
	"strings"
)

// guard.go — the S12.3 guardrail stack's parsing limb (S14.10 ¶3: "single-
// statement parse rejecting DDL/DML").
//
// THE MODEL IS UNTRUSTED INPUT AND THIS FILE IS THE BOUNDARY. Nothing about
// what Arctic emits is assumed: not that it is SQL, not that it is one
// statement, not that it reads what it claims to read. Everything it emits
// passes through here, and what does not survive does not execute.
//
// WHY EXTRACTION IS THE DESIGN, NOT A CONVENIENCE. The deployed seat scored
// 0/30 on the raw alias suite (P3/measurements/2026-07-22-sql-open-arctic.md,
// Wilson [0.00, 0.11]) — WORSE than the general-purpose proxies. The diagnosis
// is that Arctic-Text2SQL-R1 is a REASONING model: it emits a long chain of
// thought and the SQL comes after it. Suppressing the reasoning is not the fix
// — the reasoning IS the model's method, and `NoThink` would degrade the very
// thing the seat was chosen for. The fix is that the single-statement parse
// LOCATES the statement inside the reasoning. That is what this file does, and
// it is the measurement's own stated conclusion.
//
// WHAT SQLITE DOES NOT STOP (measured 2026-07-27 on the pinned driver, and the
// reason every limb below exists rather than deferring to the handle):
//
//   - Multi-statement text EXECUTES. `SELECT 1 AS one; SELECT 2 AS two` runs
//     both and returns the SECOND statement's rows, with no error. A benign
//     prefix is therefore a real smuggling vector, and only this parse stops it.
//   - `ATTACH` SUCCEEDS on the read-only handle and creates a file on disk. It
//     is refused here or not at all.
//   - `PRAGMA query_only = 0` succeeds silently, clearing the spec's named
//     limb. (`mode=ro` still refuses the write — see internal/storage.)
//   - A runaway recursive query is NOT bounded by a limit clause, because the
//     aggregate never yields a row. Only the wall clock stops it.
//
// The owner predicate is enforced HERE, not by the handle: the read-only
// handle can see every row in the database, so S01.9 scoping is a property of
// the statement this file produces (rewriteScope below).

// Guardrail errors. Every one is a REFUSAL, carried into the audit record.
var (
	// ErrNoSQL — the output contained no statement at all. The honest-failure
	// path: the model answered in prose, or refused, or wandered.
	ErrNoSQL = errors.New("history: the model emitted no SQL")
	// ErrRefused — the statement was parsed and rejected by the guardrail.
	ErrRefused = errors.New("history: refused by the Layer-2 guardrail")
)

// refusal wraps a reason as an ErrRefused.
func refusal(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrRefused, fmt.Sprintf(format, args...))
}

// bannedWords are statement words that may not appear anywhere in a generated
// statement. They are matched as WORDS by the lexer — never as substrings — so
// a column called `updated_ts` is not confused with a write.
//
// The list is deliberately wider than "DDL/DML": it also closes the attach,
// pragma, transaction-control and extension-loading surfaces, each of which
// the measurement above shows the read-only handle does not close by itself.
var bannedWords = map[string]string{
	"insert":          "a write",
	"update":          "a write",
	"delete":          "a write",
	"upsert":          "a write",
	"truncate":        "a write",
	"drop":            "schema change",
	"alter":           "schema change",
	"reindex":         "schema maintenance (SQLite executes it silently on a read-only handle)",
	"vacuum":          "schema maintenance",
	"analyze":         "schema maintenance",
	"trigger":         "schema change",
	"attach":          "reaches another database file (SQLite does NOT refuse this on a read-only handle)",
	"detach":          "database-file control",
	"pragma":          "engine control (PRAGMA query_only=0 succeeds silently)",
	"begin":           "transaction control",
	"commit":          "transaction control",
	"rollback":        "transaction control",
	"savepoint":       "transaction control",
	"release":         "transaction control",
	"into":            "a write target",
	"returning":       "a write clause",
	"load_extension":  "loads native code",
	"writable_schema": "engine control",
	// D8: blob inflation. These build arbitrarily large values in memory, and
	// memory is the one resource the guardrail does NOT bound — the row limit
	// counts rows and the timeout counts seconds, and a single row can be a
	// gigabyte. Banning the constructors is the cheap, precise limb; the
	// residual (an expensive expression built from ordinary functions) is
	// recorded in CONVENTIONS §37-C with the timeout as its stated backstop.
	"randomblob": "builds an arbitrarily large value in memory",
	"zeroblob":   "builds an arbitrarily large value in memory",
}

// createWord and replaceWord are banned with one exemption each, so the guard
// stays a guard without refusing ordinary reading SQL:
//   - `replace(x,y,z)` is a string function; the statement word is not.
//   - `create` has no function form and is banned outright.
const (
	createWord  = "create"
	replaceWord = "replace"
)

// clauseWords end a table-reference list, so an alias is never mistaken for a
// second table.
var clauseWords = map[string]bool{
	"where": true, "group": true, "having": true, "order": true, "limit": true,
	"offset": true, "window": true, "union": true, "intersect": true, "except": true,
	"on": true, "using": true, "join": true, "inner": true, "left": true, "right": true,
	"full": true, "cross": true, "natural": true, "select": true, "from": true,
	"values": true, "when": true, "then": true, "else": true, "end": true, "and": true,
	"or": true, "not": true, "as": true, "returning": true,
}

// GuardResult is one statement that survived the whole stack, with everything
// the audit record needs about how it got there.
type GuardResult struct {
	// Extracted is the statement as located in the model's output, before any
	// guardrail rewriting. The model's own text.
	Extracted string
	// Statement is what actually executes: Extracted with the owner predicate
	// applied to every view it reads and the row bound wrapped around it.
	Statement string
	// Args are the bound values, in the order Statement's placeholders appear:
	// two per scoped view reference, then the row bound. The generated SQL is
	// forbidden to contain placeholders of its own, so every one of these is
	// the guardrail's.
	Args []any
	// Views names every allowlisted view the statement reads, in text order.
	Views []string
	// Limit is the injected row bound.
	Limit int
	// Scoped counts the view references the owner predicate was applied to.
	Scoped int
}

// ExtractSQL locates the statement inside a reasoning model's output.
//
// The order is the order an R1-style answer is structured in: a fenced code
// block if there is one (the model's own statement of "this is the answer"),
// otherwise the LAST statement-opening word in the text — because the
// reasoning comes first and the answer comes last.
//
// It returns ErrNoSQL when there is no statement to find. That is a real and
// expected outcome, not a defect: a model that answered in prose has not
// produced SQL, and saying so is more honest than salvaging a fragment.
func ExtractSQL(raw string) (string, error) {
	body := stripThinkBlocks(raw)
	if candidate, ok := lastFencedBlock(body); ok {
		if stmt, idx, ok := lastStatement(candidate); ok {
			return stmt, checkStatementPrefix(candidate, idx)
		}
	}
	if stmt, idx, ok := lastStatement(body); ok {
		return stmt, checkStatementPrefix(body, idx)
	}
	// The reasoning may have been fenced and the answer inside it; fall back to
	// the whole raw output before giving up.
	if stmt, idx, ok := lastStatement(raw); ok {
		return stmt, checkStatementPrefix(raw, idx)
	}
	return "", ErrNoSQL
}

// checkStatementPrefix refuses a statement whose OWN LINE begins with something
// that was trying to be a different statement.
//
// This closes the gap extraction would otherwise open. Locating the statement
// inside surrounding text means a prefix gets dropped — and `DROP TABLE runs;
// SELECT 1` or `INSERT INTO t SELECT …` would then be silently DEFUSED into a
// harmless read rather than refused. Defusing is safe but quiet, and a
// guardrail that handles an attack without recording one has not detected it.
//
// The check is scoped to the statement's own line, which is where a
// SQL-contiguous prefix lives; reasoning prose sits on earlier lines and is not
// judged. A false refusal is possible (a reasoning line ending in a banned word
// immediately before the statement) and is the SAFE direction: it is loud, it
// is audited, and the floor below it still answers.
// The prefix examined is the statement's own PARAGRAPH — back to the last blank
// line, not merely to the last newline (D6). Scoping it to one line let
// "DROP TABLE runs;\nSELECT …" be defused into a harmless read and audited as
// `executed`, which is the quiet handling this function exists to prevent. A
// blank line (or a fence) is what actually separates reasoning from an answer
// in a model's output, so the paragraph is the honest unit.
//
// Two signals, both deliberately narrow so ordinary reasoning prose survives:
// a line that BEGINS with a banned statement word (statement position), and a
// statement separator anywhere in the paragraph.
func checkStatementPrefix(src string, idx int) error {
	start := 0
	if i := strings.LastIndex(src[:idx], "\n\n"); i >= 0 {
		start = i + 2
	}
	prefix := src[start:idx]
	if strings.TrimSpace(prefix) == "" {
		return nil
	}
	for _, line := range strings.Split(prefix, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		w := strings.ToLower(strings.Trim(fields[0], "`\"[]();,"))
		if why, banned := bannedWords[w]; banned {
			return refusal("the statement is preceded by a line beginning %q (%s)", fields[0], why)
		}
		if w == createWord {
			return refusal("the statement is preceded by a line beginning %q (schema change)", fields[0])
		}
	}
	// A separator before the statement means there WAS another statement, even
	// when both halves read. Guard's own single-statement check only sees what
	// extraction returned, so without this a `SELECT a; SELECT b` would run its
	// second half and say nothing about the first.
	if toks, err := lex(prefix); err == nil {
		for _, tk := range toks {
			if tk.kind == tokPunct && tk.text == ";" {
				return refusal("more than one statement — a statement separator precedes it")
			}
		}
	} else if strings.Contains(prefix, ";") {
		return refusal("more than one statement — a statement separator precedes it")
	}
	return nil
}

// stripThinkBlocks removes <think>…</think> regions. An unterminated opener is
// treated as "everything after it is reasoning we could not close", and the
// text before it is kept — the safe direction, since an unclosed think block
// means the generation was cut off mid-thought and has no answer in it.
func stripThinkBlocks(s string) string {
	var b strings.Builder
	rest := s
	for {
		open := indexFold(rest, "<think>")
		if open < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:open])
		after := rest[open+len("<think>"):]
		close := indexFold(after, "</think>")
		if close < 0 {
			return b.String()
		}
		rest = after[close+len("</think>"):]
	}
}

// lastFencedBlock returns the content of the last ``` fenced block that holds a
// statement-opening word.
func lastFencedBlock(s string) (string, bool) {
	parts := strings.Split(s, "```")
	// Fence CONTENTS are the odd-indexed parts; the even ones are the prose
	// between fences. Start at the last odd index and walk back by fences.
	start := len(parts) - 1
	if start%2 == 0 {
		start--
	}
	for i := start; i >= 1; i -= 2 {
		body := parts[i]
		// A fence may be tagged (```sql); drop the tag line.
		if nl := strings.IndexByte(body, '\n'); nl >= 0 {
			tag := strings.TrimSpace(body[:nl])
			if tag != "" && !strings.ContainsAny(tag, " \t") {
				body = body[nl+1:]
			}
		}
		if _, _, ok := lastStatement(body); ok {
			return body, true
		}
	}
	return "", false
}

// lastStatement returns the text from the last statement START to the end.
//
// "Start" is doing real work here. Taking the last `select` outright would
// SILENTLY TRUNCATE a compound statement — `… UNION SELECT …` would lose its
// first half, and `WITH x AS (…) SELECT …` would lose its whole with-clause,
// each producing a different question than the model asked. So a candidate
// preceded by a continuation word or an opening bracket is not a start, and
// the search keeps looking backwards.
//
// It deliberately does NOT stop at the first `;` — truncating there would
// silently DISCARD a smuggled second statement instead of refusing it, and a
// guardrail that quietly drops an attack has not detected one. The
// single-statement check in Guard is what judges what follows.
// It is TOKEN-AWARE wherever the text lexes (D5). Anchoring on raw bytes
// re-anchored inside string literals — `… LIKE '%with%'` anchored on the `with`
// INSIDE the literal and produced the fragment `with%'`, which was then refused
// as an unterminated literal. Safe, but a pure accuracy tax on a perfectly good
// statement. Text that does NOT lex is reasoning prose (an apostrophe is enough
// to stop the lexer), and there the byte scan is still the right instrument.
func lastStatement(s string) (string, int, bool) {
	if toks, err := lex(s); err == nil {
		idx, ok := anchorFromTokens(toks)
		if !ok {
			return "", 0, false
		}
		return strings.TrimSpace(s[idx:]), idx, true
	}
	best := -1
	for _, word := range []string{"select", "with"} {
		if i := lastStartIndex(s, word); i > best {
			best = i
		}
	}
	if best < 0 {
		return "", 0, false
	}
	return strings.TrimSpace(s[best:]), best, true
}

// anchorFromTokens finds the last statement START among real tokens, so a
// keyword inside a string literal or a comment is never mistaken for one.
func anchorFromTokens(toks []token) (int, bool) {
	best := -1
	for i, tk := range toks {
		if tk.kind != tokIdent || tk.quoted {
			continue
		}
		if tk.lower != "select" && tk.lower != "with" {
			continue
		}
		if i > 0 {
			p := toks[i-1]
			if p.kind == tokPunct && (p.text == "(" || p.text == "," || p.text == ")") {
				continue
			}
			if p.kind == tokIdent && !p.quoted && continuationWords[p.lower] {
				continue
			}
		}
		best = tk.start
	}
	return best, best >= 0
}

// continuationWords precede a `select` that CONTINUES a statement rather than
// starting one.
var continuationWords = map[string]bool{
	"union": true, "all": true, "intersect": true, "except": true,
	"as": true, "exists": true, "in": true, "not": true,
}

// lastStartIndex finds the last case-insensitive occurrence of word at
// identifier boundaries that is not a continuation of an earlier statement.
func lastStartIndex(s, word string) int {
	lower := strings.ToLower(s)
	for i := len(lower) - len(word); i >= 0; i-- {
		if lower[i:i+len(word)] != word {
			continue
		}
		if i > 0 && isIdentByte(lower[i-1]) {
			continue
		}
		if j := i + len(word); j < len(lower) && isIdentByte(lower[j]) {
			continue
		}
		if isContinuation(lower, i) {
			continue
		}
		return i
	}
	return -1
}

// isContinuation reports whether the token before position i makes this a
// continuation rather than a statement start.
func isContinuation(lower string, i int) bool {
	j := i - 1
	for j >= 0 && (lower[j] == ' ' || lower[j] == '\t' || lower[j] == '\n' || lower[j] == '\r') {
		j--
	}
	if j < 0 {
		return false
	}
	// "(" and "," open a subquery or a list; ")" closes a with-clause's body,
	// so the select after it belongs to that with-statement.
	if lower[j] == '(' || lower[j] == ',' || lower[j] == ')' {
		return true
	}
	k := j
	for k >= 0 && isIdentByte(lower[k]) {
		k--
	}
	return continuationWords[lower[k+1:j+1]]
}

func indexFold(s, sub string) int { return strings.Index(strings.ToLower(s), sub) }

func isIdentByte(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// Guard runs the whole parsing stack over one extracted statement and returns
// the statement that may execute — or a refusal.
//
// Every limb is a refusal, never a repair: the guardrail does not edit a
// statement into safety, because an editor has to be right about intent and a
// refuser only has to be right about danger. The single rewrite it does make —
// the owner predicate — narrows what the statement can see and never widens it.
func Guard(sqlText string, scope Scope, limit int) (GuardResult, error) {
	toks, err := lex(sqlText)
	if err != nil {
		return GuardResult{}, refusal("%v", err)
	}
	if len(toks) == 0 {
		return GuardResult{}, ErrNoSQL
	}

	// 1. ONE statement. A `;` is tolerated only as the final character; text
	// after it is a smuggled statement, or prose we cannot distinguish from
	// one, and both are refused. (Measured: the driver runs both halves and
	// returns the SECOND statement's rows.)
	for i, tk := range toks {
		if tk.kind == tokPunct && tk.text == ";" {
			if i != len(toks)-1 {
				return GuardResult{}, refusal("more than one statement — text follows the statement separator")
			}
			toks = toks[:i]
		}
	}
	if len(toks) == 0 {
		return GuardResult{}, ErrNoSQL
	}
	// D4: an ACCEPTED statement must actually be EXECUTABLE. The filter above
	// drops a trailing separator from the TOKENS, but it is the raw TEXT that
	// gets wrapped — so a trailing `;` or a trailing comment survived into the
	// wrapper and turned an accepted statement into `… ;) LIMIT ?`, a syntax
	// error at execution. Truncating to the end of the last real token drops
	// both, and cannot disturb the target offsets, which all precede it. A
	// trailing semicolon is the likeliest thing a text-to-SQL model emits.
	sqlText = strings.TrimSpace(sqlText[:toks[len(toks)-1].end])

	// 2. It must OPEN as a read.
	if first := toks[0]; first.kind != tokIdent || (first.lower != "select" && first.lower != "with") {
		return GuardResult{}, refusal("a statement must open with select or with, not %q", first.text)
	}

	// 3a. A QUOTED identifier's content is model text, and lex.go strips its
	// quoting — so it is checked here rather than skipped (D1). Only a plain
	// bare name is allowed inside quotes: that is all a legitimate column
	// reference over the allowlisted views ever needs, and it makes "a quoted
	// identifier cannot carry SQL" a checked property instead of an assumption.
	for _, tk := range toks {
		if tk.kind == tokIdent && tk.quoted && !plainIdent(tk.text) {
			return GuardResult{}, refusal(
				"the quoted identifier %q is not a plain name — quoted identifiers may not carry SQL", tk.text)
		}
	}

	// 3b. No banned statement word, anywhere, at any depth. Quoted identifiers
	// are included: after 3a they can only be plain names, and a plain name
	// that spells a write verb has no business here either.
	for i, tk := range toks {
		if tk.kind != tokIdent {
			continue
		}
		if why, banned := bannedWords[tk.lower]; banned {
			return GuardResult{}, refusal("%q is not allowed here (%s)", tk.text, why)
		}
		if tk.lower == createWord {
			return GuardResult{}, refusal("%q is not allowed here (schema change)", tk.text)
		}
		if tk.lower == replaceWord && !followedByOpenParen(toks, i) {
			return GuardResult{}, refusal("the statement word %q is not allowed here (a write); only the replace() function form is", tk.text)
		}
	}

	// 4. No bind parameters. Every placeholder in the executed statement is the
	// guardrail's own, so a generated one would shift the argument order and
	// bind a scope value into the model's expression.
	for _, tk := range toks {
		if tk.kind == tokParam {
			return GuardResult{}, refusal("generated SQL may not contain the bind parameter %q", tk.text)
		}
	}

	// 5. Common table expressions are allowed, and their names are allowed as
	// read targets — but a name that SHADOWS an allowlisted view is refused,
	// because a shadowed name would make the owner predicate apply to a
	// definition the model wrote.
	ctes := cteNames(toks)
	for name := range ctes {
		if _, isView := ViewByName(name); isView {
			return GuardResult{}, refusal("the common table expression %q shadows the allowlisted view of the same name", name)
		}
	}

	// 6. Every read target is in the allowlist. This is the limb that makes
	// R9's "raw trace payloads are structurally unreachable" compose: the base
	// tables are not on the list, so no generated statement names one.
	targets, err := readTargets(toks)
	if err != nil {
		return GuardResult{}, err
	}
	var views []string
	for _, tg := range targets {
		if ctes[tg.name] {
			continue
		}
		v, ok := ViewByName(tg.name)
		if !ok {
			return GuardResult{}, refusal("%q is not an allowlisted view", tg.text)
		}
		if v.OwnerColumn == viewOwnerColumnNone && !scope.Operator {
			return GuardResult{}, refusal("the view %q carries no owner column and is operator-only (S01.9)", v.Name)
		}
		views = append(views, v.Name)
	}
	if len(views) == 0 {
		return GuardResult{}, refusal("the statement reads no allowlisted view")
	}

	// 7. The owner predicate, applied to every view reference (S01.9).
	statement, args, scoped := rewriteScope(sqlText, targets, ctes, scope)

	// 8. The row bound. It WRAPS rather than edits, so the model's own text is
	// what executes and the bound applies whatever the model wrote — including
	// a larger limit of its own, which the wrapper tightens.
	limit = openSQLLimit(limit)
	statement = "SELECT * FROM (" + statement + ") LIMIT ?"
	args = append(args, limit+1)

	return GuardResult{
		Extracted: sqlText,
		Statement: statement,
		Args:      args,
		Views:     views,
		Limit:     limit,
		Scoped:    scoped,
	}, nil
}

func followedByOpenParen(toks []token, i int) bool {
	return i+1 < len(toks) && toks[i+1].kind == tokPunct && toks[i+1].text == "("
}

// checkAlias refuses a table alias that is not a plain bare identifier.
//
// THIS IS THE ROOT OF THE B5-8B DRAIN'S CRITICAL FINDING (D1). The alias is the
// ONE piece of model text this file re-emits into the statement it builds
// (rewriteScope's `AS <alias>`), and lex.go deliberately STRIPS the quoting
// from a quoted, bracketed or backticked identifier — so `AS "a, run_events x"`
// arrived here as the bare content `a, run_events x` and was re-emitted as raw
// SQL. That text was seen by NOTHING: the banned-word scan skipped quoted
// tokens, the bind-parameter check never looked inside one, and readTargets
// never saw the tables it named. The measured result was a comma-join to
// run_events that returned both owners' raw payloads to a member, and from
// there a chain to ATTACH and a file written outside platform.db.
//
// The fix is at the root and is belt-and-braces: a quoted alias is REFUSED
// here, and rewriteScope quotes defensively on the way out (quoteIdent), so no
// future path can re-open this by accident.
func checkAlias(tk token) error {
	if tk.quoted {
		return refusal("a quoted table alias (%q) is not allowed — an alias is re-emitted into the executed statement, so it must be a plain name", tk.text)
	}
	if !plainIdent(tk.text) {
		return refusal("the table alias %q is not a plain identifier", tk.text)
	}
	return nil
}

// plainIdent reports whether s is a bare SQL identifier: a letter or underscore
// followed by letters, digits, underscores or dollars, and nothing else. It is
// the whitelist that makes "cannot carry SQL" a property rather than a hope.
func plainIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if i == 0 && (c >= '0' && c <= '9') {
			return false
		}
		if !isIdentByte(c) {
			return false
		}
	}
	return true
}

// quoteIdent renders an identifier as a double-quoted SQL identifier with the
// doubled-quote escape. Defence in depth behind checkAlias: even if a
// non-plain name ever reached emission, it could not close its own quoting and
// become syntax.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// target is one read target with the byte span the scope rewrite replaces.
type target struct {
	name  string // lowercased, unquoted
	text  string // as written
	start int    // byte offset of the reference in the source
	end   int    // byte offset one past the reference (including any alias)
	alias string // the alias to preserve, or the view name itself
}

// readTargets collects every table reference — the identifier after a from or
// a join, and each further identifier in a comma-separated list.
//
// A QUALIFIED name is refused outright: `main.cost_per_run` and
// `side.anything` are the shape that reaches out of the database the allowlist
// describes, and no legitimate generated query needs one.
func readTargets(toks []token) ([]target, error) {
	var out []target
	depth, fromDepth := 0, 0
	inFrom, expect := false, false

	for i := 0; i < len(toks); i++ {
		tk := toks[i]
		// Bracket depth is accounted FIRST, for every token, so a target
		// position that happens to be a subquery cannot desynchronize it.
		if tk.kind == tokPunct {
			switch tk.text {
			case "(":
				depth++
			case ")":
				depth--
				if inFrom && depth < fromDepth {
					inFrom = false
				}
			}
		}
		if expect {
			switch {
			case tk.kind == tokPunct && tk.text == "(":
				// A subquery or a parenthesized join; its own targets are seen
				// by this same walk.
				expect = false
			case tk.kind == tokIdent && !clauseWords[tk.lower]:
				if i+1 < len(toks) && toks[i+1].kind == tokPunct && toks[i+1].text == "." {
					return nil, refusal("qualified table name %q is not allowed", tk.text)
				}
				// D7: a QUOTED table name is refused. lex.go strips the quoting,
				// so a quoted name reaches the allowlist as its bare content —
				// and a name the author had to quote is not one of the plain
				// registry names anyway.
				if tk.quoted {
					return nil, refusal("a quoted table name (%q) is not allowed; the allowlisted views are named plainly", tk.text)
				}
				tg := target{name: tk.lower, text: tk.text, start: tk.start, end: tk.end, alias: tk.text}
				// Preserve an alias so column references keep resolving.
				if j := i + 1; j < len(toks) {
					if toks[j].kind == tokIdent && toks[j].lower == "as" && j+1 < len(toks) &&
						toks[j+1].kind == tokIdent && !clauseWords[toks[j+1].lower] {
						if err := checkAlias(toks[j+1]); err != nil {
							return nil, err
						}
						tg.alias, tg.end = toks[j+1].text, toks[j+1].end
						i = j + 1
					} else if toks[j].kind == tokIdent && !clauseWords[toks[j].lower] {
						if err := checkAlias(toks[j]); err != nil {
							return nil, err
						}
						tg.alias, tg.end = toks[j].text, toks[j].end
						i = j
					}
				}
				out = append(out, tg)
				expect = false
			default:
				expect = false
			}
			continue
		}
		switch {
		case tk.kind == tokPunct && tk.text == "," && inFrom && depth == fromDepth:
			expect = true
		case tk.kind == tokIdent && !tk.quoted:
			switch tk.lower {
			case "from", "join":
				inFrom, fromDepth, expect = true, depth, true
			case "where", "group", "having", "order", "limit", "window", "union",
				"intersect", "except", "on", "using", "select":
				inFrom = false
			}
		}
	}
	return out, nil
}

// cteNames collects the names a leading with-clause defines.
func cteNames(toks []token) map[string]bool {
	out := map[string]bool{}
	if len(toks) == 0 || toks[0].kind != tokIdent || toks[0].lower != "with" {
		return out
	}
	i := 1
	if i < len(toks) && toks[i].kind == tokIdent && toks[i].lower == "recursive" {
		i++
	}
	for i < len(toks) {
		if toks[i].kind != tokIdent || clauseWords[toks[i].lower] {
			return out
		}
		out[toks[i].lower] = true
		i++
		if i < len(toks) && toks[i].kind == tokPunct && toks[i].text == "(" {
			i = skipBalanced(toks, i)
		}
		if i >= len(toks) || toks[i].kind != tokIdent || toks[i].lower != "as" {
			return out
		}
		i++
		for i < len(toks) && toks[i].kind == tokIdent &&
			(toks[i].lower == "not" || toks[i].lower == "materialized") {
			i++
		}
		if i >= len(toks) || toks[i].kind != tokPunct || toks[i].text != "(" {
			return out
		}
		i = skipBalanced(toks, i)
		if i < len(toks) && toks[i].kind == tokPunct && toks[i].text == "," {
			i++
			continue
		}
		return out
	}
	return out
}

// skipBalanced returns the index one past the ")" matching the "(" at i.
func skipBalanced(toks []token, i int) int {
	depth := 0
	for ; i < len(toks); i++ {
		if toks[i].kind != tokPunct {
			continue
		}
		switch toks[i].text {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return i
}

// rewriteScope replaces every allowlisted view reference with the same view
// under the S01.9 owner predicate, preserving the reference's name so column
// qualifications keep resolving.
//
// This is where owner scoping lives, and it has to be here: the read-only
// handle can see every row in the file, so scoping is a property of the
// statement. The predicate is the SAME parameterized form the Layer-1 catalog
// uses (OwnerScopeOpen/OwnerScopeClose) — the operator binds the empty string
// and a member binds their identity, and user_id is CHECK'd non-empty in every
// table, so the empty string can only ever mean "operator".
func rewriteScope(src string, targets []target, ctes map[string]bool, scope Scope) (string, []any, int) {
	var b strings.Builder
	var args []any
	scoped, cursor := 0, 0
	for _, tg := range targets {
		if ctes[tg.name] {
			continue
		}
		v, ok := ViewByName(tg.name)
		if !ok || v.OwnerColumn == viewOwnerColumnNone {
			continue
		}
		b.WriteString(src[cursor:tg.start])
		b.WriteString("(SELECT * FROM " + v.Name + " WHERE (? = '' OR " + v.OwnerColumn + " = ?)) AS " + quoteIdent(tg.alias))
		args = append(args, scope.scopeArg(), scope.scopeArg())
		cursor = tg.end
		scoped++
	}
	b.WriteString(src[cursor:])
	return b.String(), args, scoped
}

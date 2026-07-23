// Package redact is the codor-C2 serve-side redaction primitive
// (Research/18 §3-C2 / §7-C2): a pure, deterministic, idempotent text→text
// scrubber that replaces incidentally-captured secrets with an inert
// [REDACTED:<class>] marker at the OBSERVABILITY serving edge.
//
// DEFENSE IN DEPTH — NOT A SUBSTITUTE FOR D2. Sinet keeps secrets out of run
// environments at the source: the credential broker, the scrubbed child env,
// and the CI key-hygiene greps are the D2 prevention story (S11, S03.5). This
// primitive is the SECOND line of defense: tool output can still capture a
// secret the broker never touched — an agent `cat`s a key file, a provider
// echoes an Authorization header — and from there it flows into run_events →
// the S14.3 /events projection → household-visible screens. Redacting at the
// serving edge catches that incidental capture. It NEVER licenses weakening
// the first line: no D2 posture is relaxed on the strength of this backstop
// (§7-C2·4).
//
// The package is a stdlib-only LEAF (no in-repo imports), so it composes
// without cycles: internal/api imports it now for the live stream (frame
// payloads + snapshot strings), and the B5-8 query/search layers import the
// same primitive later to redact BEFORE matching — a search endpoint that
// matches over already-redacted text can never be an oracle for a redacted
// secret (§3-C2).
//
// SCOPE (§7-C2·1/·2): this is applied only to observability projections. It is
// never wired as a blanket response middleware and is never applied to
// deliverable file content (S13's product) or broker internals — those serve
// paths do not import this package, which is a grep/AST-checkable invariant.
//
// The primitive stores nothing and is constant-on: there is no ⚙ toggle
// (§7-C2·3). The raw bytes remain the audit truth in run_events; redaction
// happens only at serialization, so the stored row is never mutated (§3-C2,
// S02.1 append-only).
package redact

import (
	"bytes"
	"encoding/json"
	"regexp"
)

// rule is one ordered redaction pattern. repl may reference the ${1} capture
// group so a KEY=value pair keeps its (non-secret) key name while the value is
// replaced. Every marker is inert — no rule matches any marker it produces —
// so the ordered pipeline is idempotent (see the package test).
type rule struct {
	re   *regexp.Regexp
	repl string
}

// rules are applied in order. Order matters where classes overlap: the PEM
// block is matched whole before any inner pattern can chew it; the specific
// Anthropic key class claims sk-ant-… before the generic sk-… class. The
// token-prefix formats below are the stable, published shapes as of mid-2026
// (Anthropic sk-ant-… is the load-bearing class — the live claude-cli lane);
// patterns are deliberately conservative (a prefix plus a length floor) so
// honest content is left byte-for-byte unchanged while any leak is caught.
var rules = []rule{
	// PEM private-key blocks — matched whole (?s = dot matches newlines) so a
	// multi-line key never survives in fragments.
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`), "[REDACTED:pem_private_key]"},
	// Anthropic API keys (sk-ant-api03-…, sk-ant-admin01-…) — claimed before
	// the generic sk-… rule so they carry the specific class label.
	{regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{16,}`), "[REDACTED:anthropic_key]"},
	// GitHub tokens: the gh?_ class (ghp_/gho_/ghu_/ghs_/ghr_) + fine-grained.
	{regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`), "[REDACTED:github_token]"},
	{regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`), "[REDACTED:github_token]"},
	// OpenAI-style keys: generic sk-… (after sk-ant-… was already claimed).
	{regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`), "[REDACTED:api_key]"},
	// AWS access key ids: long-term AKIA + short-term STS ASIA.
	{regexp.MustCompile(`(?:AKIA|ASIA)[0-9A-Z]{16}`), "[REDACTED:aws_key_id]"},
	// Bearer tokens (Authorization: Bearer …). The \s+ guard means no marker
	// ("[REDACTED:bearer_token]" has no space after "bearer") re-matches.
	{regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]{12,}`), "[REDACTED:bearer_token]"},
	// The named Sinet session cookie value — the analog of codor's "own
	// pairing codes". These normally never reach run_events (D2); this is the
	// incidental-echo backstop. The marker's [ ] break the value class, so the
	// rule never re-fires on its own output.
	{regexp.MustCompile(`sinet_session=[A-Za-z0-9._~+/=-]+`), "sinet_session=[REDACTED:session]"},
	// Uppercase env-style secret pairs: FOO_TOKEN=…, API_KEY=…, DB_PASSWORD=…
	// The value class is \S+ (whitespace-bounded); the key name is preserved so
	// an operator sees WHICH variable was scrubbed. Redact operates on one
	// string value at a time (RedactJSON walks the parsed structure and calls
	// Redact per value), so \S+ never crosses a JSON delimiter, and the marker
	// is a fixpoint of the value class — the rule re-fires to the same bytes.
	{regexp.MustCompile(`([A-Z][A-Z0-9_]*(?:TOKEN|SECRET|KEY|PASSWORD|PASSWD|CREDENTIAL)[A-Z0-9_]*=)\S+`), "${1}[REDACTED:secret_pair]"},
}

// Redact returns s with every recognized secret replaced by an inert
// [REDACTED:<class>] marker. It is a pure function: deterministic, idempotent
// (Redact(Redact(s)) == Redact(s)), and byte-for-byte identity on honest text
// (no match → the input string is returned unchanged). It is the reusable
// text→text form the B5-8 search layer redacts with BEFORE matching.
//
// Redact is intended for individual string values (a last-activity line, a
// tool name, one JSON string field). RedactJSON deep-walks a JSON payload and
// applies Redact to each string value; do not feed serialized JSON to Redact
// directly, or a whitespace-bounded value class could span JSON syntax.
func Redact(s string) string {
	for _, r := range rules {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}

// RedactJSON deep-walks a JSON payload and redacts every string VALUE (never a
// key) with Redact, returning re-serialized JSON. When nothing was redacted it
// returns the INPUT bytes unchanged, so an honest frame is byte-for-byte
// stable and the function is idempotent at the JSON layer
// (RedactJSON(RedactJSON(p)) == RedactJSON(p)): the second pass finds no
// secret in the markers and returns its input untouched. Invalid JSON is
// treated as one opaque string and redacted whole (defensive — run_events
// payloads are validated JSON at append, S02.1).
func RedactJSON(payload json.RawMessage) json.RawMessage {
	var v any
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber() // preserve numeric formatting across the round-trip
	if err := dec.Decode(&v); err != nil {
		if quoted, qerr := json.Marshal(Redact(string(payload))); qerr == nil {
			return quoted
		}
		return payload
	}
	changed := false
	walked := walk(v, &changed)
	if !changed {
		return payload // honest payload: byte-for-byte unchanged
	}
	out, err := json.Marshal(walked)
	if err != nil {
		return payload
	}
	return out
}

// walk recurses the decoded JSON, redacting string values in place and
// recording whether any value changed. Keys, numbers, booleans and null pass
// through untouched.
func walk(v any, changed *bool) any {
	switch t := v.(type) {
	case string:
		r := Redact(t)
		if r != t {
			*changed = true
		}
		return r
	case map[string]any:
		for k, val := range t {
			t[k] = walk(val, changed)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = walk(val, changed)
		}
		return t
	default:
		return v
	}
}

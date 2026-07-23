package redact_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/redact"
)

// TestRedactClasses proves each secret pattern class of Research/18 §3-C2 is
// caught and replaced by its marker, with the load-bearing Anthropic key class
// (the live claude-cli lane) carrying its own label.
func TestRedactClasses(t *testing.T) {
	cases := []struct {
		name, in, wantMarker, mustNotContain string
	}{
		{"anthropic", "key is sk-ant-api03-Abc123_def456-GHI789jklmno end", "[REDACTED:anthropic_key]", "sk-ant-api03"},
		{"openai", "OPENAI sk-proj-ABCDEF0123456789abcdef done", "[REDACTED:api_key]", "sk-proj-ABCDEF"},
		{"github_classic", "token ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ012345 x", "[REDACTED:github_token]", "ghp_ABCDEF"},
		{"github_finegrained", "github_pat_11ABCDE0000abcdefghij_KLMNOP now", "[REDACTED:github_token]", "github_pat_11ABCDE"},
		{"aws", "id AKIAIOSFODNN7EXAMPLE here", "[REDACTED:aws_key_id]", "AKIAIOSFODNN7EXAMPLE"},
		{"bearer", "Authorization: Bearer eyJhbGciOi.JIUzI1NiIsIn.abcdefghij done", "[REDACTED:bearer_token]", "eyJhbGciOi"},
		{"pem", "-----BEGIN RSA PRIVATE KEY-----\nMIIabc\nDEF==\n-----END RSA PRIVATE KEY-----", "[REDACTED:pem_private_key]", "MIIabc"},
		{"secret_pair", "env DB_PASSWORD=hunter2superlong value", "[REDACTED:secret_pair]", "hunter2superlong"},
		{"session_cookie", "Cookie: sinet_session=abcDEF123.tokenvalue x", "[REDACTED:session]", "abcDEF123.tokenvalue"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := redact.Redact(c.in)
			if !strings.Contains(got, c.wantMarker) {
				t.Fatalf("Redact(%q) = %q, want marker %q", c.in, got, c.wantMarker)
			}
			if strings.Contains(got, c.mustNotContain) {
				t.Fatalf("Redact(%q) = %q still leaks %q", c.in, got, c.mustNotContain)
			}
		})
	}
}

// TestRedactHonestContentByteUnchanged proves invariant #4: an honest string
// with no secret is returned byte-for-byte unchanged (zero behavior change for
// honest content, §3-C2).
func TestRedactHonestContentByteUnchanged(t *testing.T) {
	honest := []string{
		"the run finished stage 3 of the pipeline and produced a report",
		"tool: read_file args_digest=sha256:deadbeef path=/home/user/report.md",
		"commit 3d74a0f applied on branch main with 42 files changed",
		"lane=anthropic model=claude-opus tokens=1234 cost=0.00",
		"",
	}
	for _, h := range honest {
		if got := redact.Redact(h); got != h {
			t.Fatalf("Redact(%q) = %q, want byte-unchanged", h, got)
		}
	}
}

// TestRedactIdempotent proves invariant #4: redacting redacted text is a
// no-op across every class (no marker re-matches any rule).
func TestRedactIdempotent(t *testing.T) {
	inputs := []string{
		"sk-ant-api03-AbcDEF123456_ghiJKL and ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ012345",
		"Authorization: Bearer eyJhbGciOi.JIUzI1NiIsIn.abcdefghij",
		"AWS AKIAIOSFODNN7EXAMPLE plus API_KEY=verylongsecretvalue123",
		"-----BEGIN EC PRIVATE KEY-----\nMIIabcDEF\n-----END EC PRIVATE KEY-----",
		"Cookie: sinet_session=abcDEF123.tokenvalue; other=1",
		"honest text, unchanged",
	}
	for _, in := range inputs {
		once := redact.Redact(in)
		twice := redact.Redact(once)
		if once != twice {
			t.Fatalf("not idempotent for %q:\n once  = %q\n twice = %q", in, once, twice)
		}
	}
}

// TestRedactJSONDeepWalk proves RedactJSON scrubs string values at any depth
// (object, array, nested), leaves keys and non-string scalars intact, and
// produces valid JSON.
func TestRedactJSONDeepWalk(t *testing.T) {
	in := json.RawMessage(`{
		"note": "leaked sk-ant-api03-AbcDEF123456_ghiJKLmnop here",
		"count": 7,
		"ok": true,
		"nested": {"auth": "Bearer eyJhbGciOi.JIUzI1NiIsIn.abcdefghij"},
		"list": ["AKIAIOSFODNN7EXAMPLE", "plain string", 3.14]
	}`)
	out := redact.RedactJSON(in)
	s := string(out)
	if !json.Valid(out) {
		t.Fatalf("RedactJSON produced invalid JSON: %s", s)
	}
	for _, leak := range []string{"sk-ant-api03", "eyJhbGciOi", "AKIAIOSFODNN7EXAMPLE"} {
		if strings.Contains(s, leak) {
			t.Fatalf("RedactJSON still leaks %q: %s", leak, s)
		}
	}
	// Keys and non-string scalars survive.
	for _, keep := range []string{`"count"`, `7`, `"ok"`, `true`, `"plain string"`, `3.14`} {
		if !strings.Contains(s, keep) {
			t.Fatalf("RedactJSON dropped %q: %s", keep, s)
		}
	}
}

// TestRedactJSONHonestByteUnchanged proves an honest JSON payload is returned
// byte-for-byte unchanged — the property the live-stream honest frame relies
// on (existing SSE tests assert exact payload bytes).
func TestRedactJSONHonestByteUnchanged(t *testing.T) {
	honest := []json.RawMessage{
		json.RawMessage(`{"n":1}`),
		json.RawMessage(`{"stage":"verify","state":"running","steps":3}`),
		json.RawMessage(`{"a":{"b":[1,2,{"c":"honest"}]},"z":null}`),
	}
	for _, h := range honest {
		got := redact.RedactJSON(h)
		if string(got) != string(h) {
			t.Fatalf("RedactJSON(%s) = %s, want byte-unchanged", h, got)
		}
	}
}

// TestRedactJSONIdempotent proves RedactJSON(RedactJSON(p)) == RedactJSON(p)
// on a secret-bearing payload: the second pass finds nothing to redact and
// returns its input untouched.
func TestRedactJSONIdempotent(t *testing.T) {
	in := json.RawMessage(`{"k":"API_KEY=verylongsecretvalue123","d":{"t":"sk-ant-api03-AbcDEF123456_ghiJKL"}}`)
	once := redact.RedactJSON(in)
	twice := redact.RedactJSON(once)
	if string(once) != string(twice) {
		t.Fatalf("RedactJSON not idempotent:\n once  = %s\n twice = %s", once, twice)
	}
	if strings.Contains(string(once), "verylongsecretvalue123") || strings.Contains(string(once), "sk-ant-api03") {
		t.Fatalf("first pass failed to redact: %s", once)
	}
}

package api

import (
	"net/url"
	"testing"
	"time"
)

// goPathEscapeForTest is the Go standard library's own path escaper, named here
// only so the control below can show it disagrees.
func goPathEscapeForTest(s string) string { return url.PathEscape(s) }

// NavigatePathForTest exposes the server-side deep-link composer to the
// external api_test package, which writes it into the cross-language golden the
// web suite round-trips through its own router.
func NavigatePathForTest(cardID string) string { return inboxItemPath(cardID) }

// PushDueForTest exposes the S15.6 re-nag rule to the external api_test
// package, which is where the SLA table is driven exhaustively. The rule is a
// pure function of stored state precisely so it can be checked this way, with
// no database and no clock of its own.
func PushDueForTest(class string, born, last, now time.Time, pushAfter, reping time.Duration) bool {
	return pushDue(class, born, last, now, pushAfter, reping)
}

// TestNavigatePathEncodesLikeEncodeURIComponent pins the Go half of the
// cross-language agreement (B6-9 R3/rubric 8).
//
// A tapped notification lands on `navigate`, and `navigate` is composed
// SERVER-SIDE while the URL it has to match is produced CLIENT-SIDE by
// hrefFor('inbox-item', {id}) — which encodes with JavaScript's
// encodeURIComponent. Go's own url.PathEscape is a DIFFERENT rule (it leaves
// ':' and '@' alone, escapes a different set of marks), so using it would send
// people to a URL the router resolves to a different card id, or to no card at
// all. The vitest half of this tie round-trips the SAME composed paths from a
// golden fixture through matchRoute.
func TestNavigatePathEncodesLikeEncodeURIComponent(t *testing.T) {
	cases := []struct{ id, want string }{
		// The plain shape.
		{"ask:ask-verify-0001", "/inbox/ask%3Aask-verify-0001"},
		// The three characters App.tsx names: ':' in every composite id, '#'
		// from a fork's dispatch id, and the unit separator.
		{"effect:e-rotate#g2", "/inbox/effect%3Ae-rotate%23g2"},
		{"ask:r-1.g2\x1fslot", "/inbox/ask%3Ar-1.g2%1Fslot"},
		// The seven marks encodeURIComponent does NOT escape, which is exactly
		// where a Go-flavoured escaper diverges.
		{"ask:a-_.!~*'()", "/inbox/ask%3Aa-_.!~*'()"},
		// '/' and '?' and '&' and '=' and '+' and ' ' all escape.
		{"kind:a/b?c&d=e+f g", "/inbox/kind%3Aa%2Fb%3Fc%26d%3De%2Bf%20g"},
		// Non-ASCII is escaped per UTF-8 BYTE, uppercase hex.
		{"ask:café", "/inbox/ask%3Acaf%C3%A9"},
		{"ask:→", "/inbox/ask%3A%E2%86%92"},
		// The percent sign itself.
		{"ask:100%", "/inbox/ask%3A100%25"},
	}
	for _, c := range cases {
		if got := inboxItemPath(c.id); got != c.want {
			t.Errorf("inboxItemPath(%q) = %q, want %q", c.id, got, c.want)
		}
	}
	// The non-tautological control: url.PathEscape — the obvious Go answer —
	// produces a DIFFERENT string for the commonest id shape there is, so this
	// test is pinning a real divergence rather than restating a library.
	if inboxItemPath("ask:x") == "/inbox/"+goPathEscapeForTest("ask:x") {
		t.Fatal("url.PathEscape agrees with encodeURIComponent on ':', so this test cannot catch the substitution it exists for")
	}
}

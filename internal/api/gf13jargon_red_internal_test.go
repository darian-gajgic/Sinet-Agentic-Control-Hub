// gf13jargon_red_internal_test.go — P3-GF13 RED tests: the api family's
// requester-facing served copy drops the wire-side spec citations (r4
// RA-5..RA-13 + WALK-F1 W4; the register bar is internal/verify/bootstrap.go
// BootstrapPostureNote). Written RED by grounding (P3/briefs/P3-GF13.md);
// no replacement wording is dictated — only the classes' absence.
package api

import (
	"regexp"
	"testing"
)

var gf13JargonClass = regexp.MustCompile(`\(S[0-9]+\.|\(S[0-9]+;|\(Spec S|\(D[0-9]+|P47-|\([0-9]+\.[0-9]+\)`)

// gf13BareD10 catches the bare decision token the Projects footer wears
// ("D10 is authority over what the platform DOES") — a spec id with no
// parentheses is the same jargon to the person reading it.
var gf13BareD10 = regexp.MustCompile(`\bD10\b`)

func gf13AssertPlain(t *testing.T, surface, s string) {
	t.Helper()
	if m := gf13JargonClass.FindString(s); m != "" {
		t.Errorf("%s carries the spec-ref %q on the wire; requester copy speaks plain words (citations live in code comments): %q", surface, m, s)
	}
}

// TestGF13CommandsDetailSpeaksPlainWords — the commands door's three answers
// are the sentence the person who pressed the button reads (WALK-F1 errand 4
// quoted them verbatim, "(S07.3)"/"(S07.8)" included).
func TestGF13CommandsDetailSpeaksPlainWords(t *testing.T) {
	gf13AssertPlain(t, "the repeated-write answer", commandsDetail(false, ProjectCapture{Version: 2}))
	gf13AssertPlain(t, "the cleared-commands answer", commandsDetail(true, ProjectCapture{Version: 3}))
	gf13AssertPlain(t, "the captured answer", commandsDetail(true, ProjectCapture{Version: 2,
		Commands: ProjectCommands{Test: "test -f index.html"}}))
}

// TestGF13ProjectsVisibilityRuleSpeaksPlainWords — the Projects list footer
// (ProjectList.Visibility): "(S13.7)" and "D10 is authority…" are the walk's
// W4 quotes.
func TestGF13ProjectsVisibilityRuleSpeaksPlainWords(t *testing.T) {
	gf13AssertPlain(t, "ProjectList.Visibility", projectsVisibilityRule)
	if gf13BareD10.MatchString(projectsVisibilityRule) {
		t.Errorf("ProjectList.Visibility names the bare decision id D10; the sentence can state the same rule in plain words: %q", projectsVisibilityRule)
	}
}

// TestGF13AcceptCardStatementsSpeakPlainWords — the accept consent card's
// tier statements, signing statement and withheld-why (GF10 put the why on the
// card's face, which makes its register requester copy, not a panel note).
func TestGF13AcceptCardStatementsSpeakPlainWords(t *testing.T) {
	gf13AssertPlain(t, "the push-arm tier statement", acceptTierStatement)
	gf13AssertPlain(t, "the pinned-arm tier statement", acceptPinnedTierStatement)
	gf13AssertPlain(t, "the local-arm tier statement", acceptLocalTierStatement)
	gf13AssertPlain(t, "the signing statement", acceptSigningPosture().Statement)
	for name, reason := range map[string]string{
		"both absent":   attributionAbsence("verify", "r-1", "", ""),
		"model absent":  attributionAbsence("verify", "r-1", "claude-cli", ""),
		"engine absent": attributionAbsence("verify", "r-1", "", "claude-sonnet-5"),
	} {
		gf13AssertPlain(t, "the withheld-accept reason ("+name+")", reason)
	}
}

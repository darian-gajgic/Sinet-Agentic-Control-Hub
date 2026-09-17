package verify

import "testing"

// Coordinator-inline pins after the drain cap (P3-TQ-3, 2026-09-17): the
// removal guard's asymmetric boundary and the first-segment shape rule.

func TestRemovalGuardCompoundPrefixCountsSuffixDoesNot(t *testing.T) {
	guarded := []string{
		"the `legacy/` folder is soft-deleted",
		"`old/` is auto-removed by the build",
		"the table is hard_deleted",
		"`tmp/` is un-removed only on failure",
		"the `legacy/` folder is removed",
		"the old import is gone",
		"nothing no longer imports it",
	}
	for _, line := range guarded {
		if _, removal := namedPaths(line); !removal {
			t.Errorf("namedPaths(%q): a real removal wording is not guarded — a finished step could be failed by presence", line)
		}
	}
	notGuarded := []string{
		"the `ui/deleted-items.ts` view lists them",
		"`ui/drop-shadow.css` styles the card",
		"lint passes with `no-unused-vars` on",
		"`ui/dropdown.tsx` opens on click",
	}
	for _, line := range notGuarded {
		if _, removal := namedPaths(line); removal {
			t.Errorf("namedPaths(%q): a compound NAME tripped the removal guard", line)
		}
	}
}

func TestPathShapedFirstSegmentShapeAndEmptySegments(t *testing.T) {
	impostors := []string{
		"React/Next.js",          // a product pair, capitalised
		"I/O.md",                 // prose, capitalised
		"TCP/IP.v4",              // an acronym pair
		"gopkg.in/yaml.v3",       // a Go module path: dotted first segment
		"example.com/index.html", // a host
		"process.env/FOO",        // an expression
		"a//b.ts",                // an empty segment can never match
		"Docs/guide.md",          // capitalised folder: undecided, the safe direction
	}
	for _, span := range impostors {
		if pathShaped(span) {
			t.Errorf("pathShaped(%q) = true — this span is not the shape of a repository path", span)
		}
	}
	keepers := []string{
		"public/images/**",
		"src/data/products.json",
		"public/images/",
		"./src/app.ts",
		"docs/a/b/c.md",
		".github/workflows/ci.yml",
		"_site/index.html",
		"2024/report.md",
	}
	for _, span := range keepers {
		if !pathShaped(span) {
			t.Errorf("pathShaped(%q) = false — a real repository path must still be decided", span)
		}
	}
}

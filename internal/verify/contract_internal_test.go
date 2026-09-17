package verify

// contract_internal_test.go — the decision's own boundary cases, at the unit
// the acceptance battery reaches only through the drain: a malformed pattern
// in the plan (plan text is a boundary input and must never crash the drain),
// the segment-wise "**" matcher, and the normalizations both classes share.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// tree writes files (slash path → content) under a fresh temp dir and
// indexes it.
func tree(t *testing.T, files map[string]string) *treeIndex {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	idx, err := indexTree(root)
	if err != nil {
		t.Fatalf("indexTree: %v", err)
	}
	return idx
}

// TestContractMalformedPatternIsRecordedNotFatal: an unterminated character
// class is plan text the platform cannot read as a pattern. It is recorded
// undecidable, attributed to the plan member it came from — never a FAIL
// against the work, and never a panic in the drain.
func TestContractMalformedPatternIsRecordedNotFatal(t *testing.T) {
	idx := tree(t, map[string]string{"src/app.ts": "export {}\n"})
	for _, tc := range []struct {
		name string
		step intake.Step
		want string
	}{
		{
			name: "in the write set",
			step: intake.Step{ID: "S-1", DoneWhen: "it builds", WriteSet: []string{"src/[unterminated"}},
			want: attrPlanWriteSet,
		},
		{
			name: "in the done-when line",
			step: intake.Step{ID: "S-2", DoneWhen: "the file `src/[unterminated` is written"},
			want: attrPlanDoneWhen,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sc := decideFromTree(tc.step, idx, nil)
			if sc.State != ContractUnverifiable {
				t.Fatalf("state %q, want UNVERIFIABLE-HERE — a pattern the platform cannot read decides nothing about the work", sc.State)
			}
			if sc.AttributedTo != tc.want {
				t.Fatalf("attributed to %q, want %q", sc.AttributedTo, tc.want)
			}
			if sc.Detail == "" {
				t.Fatal("no reason recorded")
			}
		})
	}
}

// TestContractMalformedPatternNeverMasksARefutation: precedence — a pattern
// that IS refuted still FAILs even when a sibling pattern is malformed.
func TestContractMalformedPatternNeverMasksARefutation(t *testing.T) {
	idx := tree(t, map[string]string{"src/app.ts": "export {}\n"})
	step := intake.Step{ID: "S-1", DoneWhen: "photos land in `public/images/**` and `src/[unterminated` is written"}
	sc := decideFromTree(step, idx, nil)
	if sc.State != ContractFail {
		t.Fatalf("state %q, want FAIL", sc.State)
	}
	if sc.AttributedTo != contractTreeAttribution+"public/images/**" {
		t.Fatalf("attributed to %q, want the refuted pattern", sc.AttributedTo)
	}
}

// TestTreeMatchSpansSegments pins the matcher the two classes share: "**"
// spans zero or more segments, an ordinary segment spans exactly one, and a
// metacharacter-free directory name matches everything under it.
func TestTreeMatchSpansSegments(t *testing.T) {
	idx := tree(t, map[string]string{
		"public/images/a.jpg":       "x",
		"public/images/deep/b.jpg":  "x",
		"public/robots.txt":         "x",
		"src/data/products.json":    "[]",
		"src/data/nested/more.json": "{}",
	})
	for _, tc := range []struct {
		pattern string
		want    int
	}{
		{"public/images/**", 2},
		{"public/**", 3},
		{"public/*", 1},       // one segment only
		{"public/*/*.jpg", 1}, // a.jpg, not deep/b.jpg
		{"**/*.json", 2},      // ** spans zero or more leading segments
		{"public/images", 2},  // a directory names everything under it
		{"src/data/products.json", 1},
		{"public/images/none/**", 0},
	} {
		got, err := idx.match(tc.pattern)
		if err != nil {
			t.Fatalf("%s: %v", tc.pattern, err)
		}
		if len(got) != tc.want {
			t.Errorf("%s matched %d (%v), want %d", tc.pattern, len(got), got, tc.want)
		}
	}
}

// TestContractNamedFilePresentButEmptyIsRefuted: "there is a file there" is
// not the structural fact the line asked for when the file holds nothing.
func TestContractNamedFilePresentButEmptyIsRefuted(t *testing.T) {
	idx := tree(t, map[string]string{"src/data/products.json": ""})
	sc := decideFromTree(intake.Step{ID: "S-1", DoneWhen: "product data written to `src/data/products.json`"}, idx, nil)
	if sc.State != ContractFail {
		t.Fatalf("state %q, want FAIL for an empty named file", sc.State)
	}
	// A glob is not judged on emptiness — only a named file is.
	sc = decideFromTree(intake.Step{ID: "S-2", DoneWhen: "data lands under `src/data/**`"}, idx, nil)
	if sc.State != ContractPass {
		t.Fatalf("state %q, want PASS — a glob asks for a match, not for bytes", sc.State)
	}
}

// TestNamedPathsReadsOnlyPathShapedSpans: the parser is conservative by
// design — prose, commands, versions and URLs in backticks stay the judge's.
func TestNamedPathsReadsOnlyPathShapedSpans(t *testing.T) {
	paths, removal := namedPaths("`npm run build` passes, `react@19.2` is pinned, `https://x.test/a/b` responds, `README` exists, and `docs/guide.md` is written")
	if removal {
		t.Fatal("no removal wording in the line")
	}
	if len(paths) != 1 || paths[0] != "docs/guide.md" {
		t.Fatalf("named paths = %v, want only docs/guide.md", paths)
	}
}

// TestPathShapedRejectsImpostors [drain r1 F1]: a slash does not make a path.
// Every span here carries one and none names a file, so none may be decided —
// a route read as a file FAILs a webshop that works.
func TestPathShapedRejectsImpostors(t *testing.T) {
	impostors := []string{
		"/cart",              // a route, not a file
		"/api/products",      // a route with two segments
		"/public/images/**",  // absolute: not this workspace's
		"next/image",         // an import specifier
		"node:fs/promises",   // a builtin module specifier
		"v1.2/3",             // a version, dot in the FIRST segment
		"and/or",             // prose
		"24/7",               // prose
		"on/off",             // prose
		"*.ts",               // root-only glob: no slash at all
		"README",             // a bare name
		"npm run build",      // a command (whitespace)
		"https://x.test/a/b", // a URL
		"process.env/FOO",    // an expression-ish span with a dot in the first segment
		// [drain r2 R2] the extension arm's own impostors.
		"e.g/i.e",               // an abbreviation: ".e" is not an extension
		"a/b.",                  // nothing after the dot
		"yes/no.",               // a sentence's full stop, not an extension
		"docs/guide.md#install", // a link fragment: a place in a document
		"api/v1.0",              // a version: ".0" has no letter in it
		"0.5/1.0",               // two numbers, not a path
		"../other/file.ts",      // climbs out of the workspace
		`src\data/x.ts`,         // backslashes this index cannot address
	}
	for _, span := range impostors {
		if pathShaped(span) {
			t.Errorf("pathShaped(%q) = true — this span names no file and deciding it would fail finished work", span)
		}
	}
	keepers := map[string]string{
		"public/images/**":       "a glob metacharacter",
		"src/data/products.json": "a dot in the last segment",
		"public/images/":         "a trailing slash",
		"./src/app.ts":           "a relative file",
		"docs/a/b/c.md":          "a deep file",
	}
	for span, why := range keepers {
		if !pathShaped(span) {
			t.Errorf("pathShaped(%q) = false — it qualifies by %s", span, why)
		}
	}
}

// TestNamedPathsSkipsImpostorsInALiveLine: the webshop shape that motivated
// the rule — a plan line naming routes AND a real glob decides only the glob.
func TestNamedPathsSkipsImpostorsInALiveLine(t *testing.T) {
	paths, removal := namedPaths(
		"`/cart` and `/api/products` respond, `next/image` is used, and photos are downloaded into `public/images/**`")
	if removal {
		t.Fatal("no removal wording in the line")
	}
	if len(paths) != 1 || paths[0] != "public/images/**" {
		t.Fatalf("named paths = %v, want only public/images/** — the routes and the import specifier are prose", paths)
	}
}

// TestContractWriteSetUnionPassesWhenAnyGlobMatches [drain r1 F2]: the write
// set is judged as ONE union. Over-declaring is the safe direction (Spec
// S02.8); failing a step because one of its globs is unmatched would teach
// planners to under-declare what they touch.
func TestContractWriteSetUnionPassesWhenAnyGlobMatches(t *testing.T) {
	idx := tree(t, map[string]string{"src/app.ts": "export {}\n"})
	step := intake.Step{ID: "S-1", DoneWhen: "the app is scaffolded",
		WriteSet: []string{"src/**", "docs/**"}} // docs/** matches nothing
	sc := decideFromTree(step, idx, nil)
	if sc.State != ContractPass {
		t.Fatalf("state %q, want PASS — one matching glob satisfies the union (detail %q)", sc.State, sc.Detail)
	}
	// And the union genuinely empty is still FAIL, so the rule is not vacuous.
	empty := intake.Step{ID: "S-2", DoneWhen: "docs are written",
		WriteSet: []string{"docs/**", "notes/**"}}
	if sc := decideFromTree(empty, idx, nil); sc.State != ContractFail {
		t.Fatalf("state %q, want FAIL — no glob in the union matches", sc.State)
	}
}

// TestTreeMatchDoubleStarSpansZeroSegments [drain r1 F3]: "**" is zero or
// more, so a glob ending in /** covers the directory's immediate children.
func TestTreeMatchDoubleStarSpansZeroSegments(t *testing.T) {
	idx := tree(t, map[string]string{"public/a.jpg": "x", "public/deep/b.jpg": "x"})
	got, err := idx.match("public/**")
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("public/** matched %v, want both the immediate child and the nested one", got)
	}
	// The zero-segment cases on their own [drain r2 R1]. The assertion above
	// does NOT pin them: public/a.jpg has a segment left for "**" to eat, so a
	// matcher that made "**" mean ONE or more still matches it. These two
	// leave "**" nothing at all — src/**/a.txt spans nothing between two
	// literal segments, **/*.json spans nothing before one — so a one-or-more
	// matcher fails exactly here and nowhere else in this file.
	flat := tree(t, map[string]string{"src/a.txt": "x", "root.json": "{}"})
	for _, tc := range []struct{ pattern, want string }{
		{"src/**/a.txt", "src/a.txt"},
		{"**/*.json", "root.json"},
	} {
		got, err := flat.match(tc.pattern)
		if err != nil || len(got) != 1 || got[0] != tc.want {
			t.Fatalf("%s matched %v (err %v), want [%s] — ** spans ZERO or more segments", tc.pattern, got, err, tc.want)
		}
	}
}

// TestContractUnboundedStepIsNeverClassW [drain r1 F4]: a step that could not
// bound what it writes declares nothing the files can refute (Spec S02.8).
func TestContractUnboundedStepIsNeverClassW(t *testing.T) {
	idx := tree(t, map[string]string{"src/app.ts": "export {}\n"})
	step := intake.Step{ID: "S-1", DoneWhen: "the whole project is migrated",
		WriteSet: []string{"docs/**"}, Unbounded: true}
	sc := decideFromTree(step, idx, nil)
	if sc.State != ContractUnverifiable {
		t.Fatalf("state %q, want UNVERIFIABLE-HERE — an unbounded step makes no bounded promise", sc.State)
	}
	if sc.AttributedTo != BootstrapAttribution {
		t.Fatalf("attributed to %q, want %q", sc.AttributedTo, BootstrapAttribution)
	}
}

// TestRemovalGuardMatchesWholeWordsOnly [drain r1 F5]: "dropdown" is not a
// removal. A guard that trips on a substring hands the step back with a
// reason about removing things the line never mentioned.
func TestRemovalGuardMatchesWholeWordsOnly(t *testing.T) {
	notRemovals := []string{
		"the `ui/dropdown.tsx` component opens on click",
		"the `ui/backdrop.tsx` overlay dims the page",
		"the `docs/gonegative.md` note is written",
		"the `src/unusedish.ts` helper is wired up",
		// [drain r2 R3] a hyphen is part of a word here, not a boundary:
		// Go's \b would read all three of these as removals.
		"the `src/deleted-items.ts` list renders",
		"the card carries a drop-shadow",
		"the `eslint.config.js` no-unused-vars rule is on",
	}
	for _, line := range notRemovals {
		if _, removal := namedPaths(line); removal {
			t.Errorf("namedPaths(%q) reported a removal — no removal word appears as a word", line)
		}
	}
	removals := []string{
		"the `legacy/` folder is removed and nothing imports it",
		"the old helper is deleted",
		"the stale column is dropped",
		"the `tmp/` cache is gone",
		"the unused imports are cleaned up",
		"`src/old.ts` is no longer referenced",
		"the deletion of the shim is complete",
		"removal of the shim is complete",
		// [drain r2 R3] and the real wordings still trip it, at a line's
		// start and at its end as well as mid-sentence.
		"removed the old folder",
		"the placeholder is gone",
		"the page no longer imports it",
	}
	for _, line := range removals {
		if _, removal := namedPaths(line); !removal {
			t.Errorf("namedPaths(%q) missed the removal wording — presence cannot decide it", line)
		}
	}
}

// TestIndexTreeResolvesASymlinkedRoot [drain r1 F6]: WalkDir does not follow a
// symlinked root, so walking one directly lists NOTHING — and an empty listing
// refutes every contract in the plan from files nobody read.
func TestIndexTreeResolvesASymlinkedRoot(t *testing.T) {
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "note.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	link := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	idx, err := indexTree(link)
	if err != nil {
		t.Fatalf("indexTree over a symlinked root: %v", err)
	}
	if len(idx.order) != 1 || idx.order[0] != "note.md" {
		t.Fatalf("indexed %v, want the file behind the link — an empty listing would FAIL finished work", idx.order)
	}
	step := intake.Step{ID: "S-1", DoneWhen: "the note is written", WriteSet: []string{"note.md"}}
	if sc := decideFromTree(step, idx, nil); sc.State != ContractPass {
		t.Fatalf("state %q over a symlinked root, want PASS", sc.State)
	}
}

// TestIndexTreeRefusesANonDirectoryRoot [drain r1 F6]: a root that is a file,
// or is not there at all, decides nothing — never a FAIL.
func TestIndexTreeRefusesANonDirectoryRoot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	step := intake.Step{ID: "S-1", DoneWhen: "the note is written", WriteSet: []string{"note.md"}}
	for _, root := range []string{file, filepath.Join(t.TempDir(), "absent")} {
		idx, err := indexTree(root)
		if err == nil {
			t.Fatalf("indexTree(%q) returned no error — an unreadable workspace must not read as empty", root)
		}
		sc := decideFromTree(step, idx, err)
		if sc.State != ContractUnverifiable || sc.AttributedTo != attrWorkspaceUnreadable {
			t.Fatalf("state %q attributed %q, want UNVERIFIABLE-HERE / %q", sc.State, sc.AttributedTo, attrWorkspaceUnreadable)
		}
	}
}

// TestUnreadableDetailNamesNoHostPath [drain r1 F7]: the Detail is read by the
// requester, for whom an absolute path on this machine means nothing.
func TestUnreadableDetailNamesNoHostPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	idx, err := indexTree(root)
	sc := decideFromTree(intake.Step{ID: "S-1", DoneWhen: "done", WriteSet: []string{"note.md"}}, idx, err)
	if strings.Contains(sc.Detail, root) || strings.Contains(sc.Detail, os.TempDir()) || strings.Contains(sc.Detail, "/") {
		t.Fatalf("detail %q carries a host path", sc.Detail)
	}
	if sc.Detail == "" {
		t.Fatal("no reason recorded")
	}
}

// TestFailDetailNamesAPatternOnce [drain r1 F7]: a pattern the write set and
// the line both name is one refutation, said once.
func TestFailDetailNamesAPatternOnce(t *testing.T) {
	idx := tree(t, map[string]string{"src/app.ts": "export {}\n"})
	step := intake.Step{ID: "S-1",
		DoneWhen: "photos are downloaded into `public/images/**`",
		WriteSet: []string{"public/images/**"},
	}
	sc := decideFromTree(step, idx, nil)
	if sc.State != ContractFail {
		t.Fatalf("state %q, want FAIL", sc.State)
	}
	if n := strings.Count(sc.Detail, "public/images/**"); n != 1 {
		t.Fatalf("detail names the pattern %d times, want once: %q", n, sc.Detail)
	}
}

// TestAbsoluteWriteGlobIsUndecidedNotRefuted: an absolute glob names somewhere
// other than the workspace the platform was handed, so these files can neither
// confirm nor refute it. Never a fabricated FAIL.
func TestAbsoluteWriteGlobIsUndecidedNotRefuted(t *testing.T) {
	idx := tree(t, map[string]string{"src/app.ts": "export {}\n"})
	step := intake.Step{ID: "S-1", DoneWhen: "the report is written", WriteSet: []string{"/var/reports/**"}}
	sc := decideFromTree(step, idx, nil)
	if sc.State != ContractUnverifiable {
		t.Fatalf("state %q, want UNVERIFIABLE-HERE", sc.State)
	}
	if sc.AttributedTo != attrPlanWriteSet {
		t.Fatalf("attributed to %q, want %q", sc.AttributedTo, attrPlanWriteSet)
	}
}

// TestNormalizePatternSharesOneShape: both classes read a trailing slash as
// "everything under here" and ignore a leading ./ or /.
func TestNormalizePatternSharesOneShape(t *testing.T) {
	for in, want := range map[string]string{
		"./src/app.ts": "src/app.ts",
		// A leading "/" is KEPT: an absolute pattern names somewhere other
		// than this workspace, and rebasing it onto the workspace would
		// invent a claim the plan never made (drain r1 F1).
		"/public/": "/public/**",
		"legacy/":  "legacy/**",
		"src/**":   "src/**",
		"/":        "",
	} {
		if got := normalizePattern(in); got != want {
			t.Errorf("normalizePattern(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCoveringCriterionTakesTheLowestNumber: the blocker cites one criterion
// deterministically, whatever order the coverage map iterates in.
func TestCoveringCriterionTakesTheLowestNumber(t *testing.T) {
	cov := map[string][]string{"AC-10": {"S-2"}, "AC-2": {"S-2"}, "AC-1": {"S-1"}}
	for i := 0; i < 8; i++ {
		if got := coveringCriterion("S-2", cov); got != "AC-2" {
			t.Fatalf("criterion %q, want AC-2 (the lowest-numbered covering criterion)", got)
		}
	}
	if got := coveringCriterion("S-9", cov); got != "" {
		t.Fatalf("criterion %q for an uncovered step, want none", got)
	}
}

// TestUnreadableRootDetailNamesTheFilesNotADot [drain r2 R4]: when the ROOT
// itself cannot be listed, its relative path is ".", and rendering that into
// the requester's sentence produced "…: . inside them could not be read." A
// person is owed a sentence about their files, not a dot to decode.
func TestUnreadableRootDetailNamesTheFilesNotADot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := os.Chmod(root, 0o000); err != nil {
		t.Skipf("cannot make a folder unreadable here: %v", err)
	}
	// Restored before the temp dir is cleaned up, whatever this test does.
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	idx, err := indexTree(root)
	if err == nil {
		t.Skip("this user can list a 0000 folder, so there is no unreadable root to pin here")
	}
	sc := decideFromTree(intake.Step{ID: "S-1", DoneWhen: "the note is written", WriteSet: []string{"note.md"}}, idx, err)
	if sc.State != ContractUnverifiable || sc.AttributedTo != attrWorkspaceUnreadable {
		t.Fatalf("state %q attributed %q, want UNVERIFIABLE-HERE / %q", sc.State, sc.AttributedTo, attrWorkspaceUnreadable)
	}
	if strings.Contains(sc.Detail, "them: .") || strings.Contains(sc.Detail, "inside them") {
		t.Fatalf("detail %q renders the root as \".\" — it must say the files themselves could not be read", sc.Detail)
	}
	if !strings.Contains(sc.Detail, "could not be listed") {
		t.Fatalf("detail %q does not say what happened", sc.Detail)
	}
	// A deeper unreadable path still names itself, workspace-relative.
	deep := &treeReadError{Rel: "public/images"}
	if got := deep.Error(); got != "public/images inside them could not be read" {
		t.Fatalf("a deeper failure reads %q — the workspace-relative rule is for paths below the root", got)
	}
}

// TestRootWriteSetIsUndecidedNotNoWrites [drain r2 R5]: a write set of ["/"]
// normalizes to nothing, and reading that as "this step declares no files to
// write" tells the requester the plan said something it did not. It names a
// root — which these files can neither confirm nor refute — so it takes the
// absolute-pattern route, and the reason names the pattern.
func TestRootWriteSetIsUndecidedNotNoWrites(t *testing.T) {
	idx := tree(t, map[string]string{"src/app.ts": "export {}\n"})
	for _, pattern := range []string{"/", "./"} {
		step := intake.Step{ID: "S-1", DoneWhen: "the work is done", WriteSet: []string{pattern}}
		sc := decideFromTree(step, idx, nil)
		if sc.State != ContractUnverifiable {
			t.Fatalf("write set [%q]: state %q, want UNVERIFIABLE-HERE", pattern, sc.State)
		}
		if sc.AttributedTo != attrPlanWriteSet {
			t.Fatalf("write set [%q]: attributed to %q, want %q", pattern, sc.AttributedTo, attrPlanWriteSet)
		}
		if !strings.Contains(sc.Detail, pattern) {
			t.Fatalf("write set [%q]: detail %q does not name the pattern", pattern, sc.Detail)
		}
		if strings.Contains(sc.Detail, "declares no files to write") {
			t.Fatalf("write set [%q]: detail %q reports a claim the plan never made", pattern, sc.Detail)
		}
	}
}

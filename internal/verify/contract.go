package verify

// contract.go decides a PLAN step's "Done when" contract from the tree the
// work actually produced (Spec S07.3 stage contracts; Spec S07.8 bootstrap
// bullet [A16, 2026-09-17]: every Done-when contract decidable from the tree
// — write-set globs, named files, structural facts — is DECIDED at V1,
// never from the executor's report).
//
// Two deterministic classes decide, and both read only files:
//
//   - W, the step's declared write set. A step that bounded what it would
//     write and whose globs, as a union, match nothing is refuted. The union
//     reading is the S07.2 wrote-nothing gate's own; per-glob strictness is
//     deliberately NOT applied, because over-declaring a write set is the
//     safe direction (Spec S02.8) and punishing it would teach planners to
//     under-declare.
//   - N, the paths the "Done when" line itself names in backticks. A glob
//     must match a file, a directory must hold one, and a named file must
//     exist and be non-empty.
//
// A line no class applies to is recorded UNVERIFIABLE-HERE with the reason,
// never PASS and never N-A: N-A is the ladder's word for "a suite ran and no
// check covered this", and at the bootstrap posture no suite ran.
//
// Nothing the executor said about its own work — report, transcript or
// deliverable text — is an input to any of it.

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/intake"
)

// contractDetailMaxExamples caps how many example paths one fact names in a
// contract's Detail. Structural, not a ⚙ setting: Detail is one sentence a
// person reads, so this bounds a rendering, never what the platform decides
// — the decision is identical at any value.
const contractDetailMaxExamples = 3

// Contract attributions beyond the ladder's own check ids (Spec S07.3):
// what refuted a contract, or why the files could not decide it.
const (
	// contractTreeAttribution prefixes the pattern the produced files
	// refuted, so the record names what was measured and found missing.
	contractTreeAttribution = "tree:"
	// attrWorkspaceUnreadable: the produced tree could not be read at all,
	// so nothing was decided from it — an outage escalates rather than
	// approves (Spec S07.2), and it fabricates neither PASS nor FAIL.
	attrWorkspaceUnreadable = "workspace:unreadable"
	// attrPlanWriteSet / attrPlanDoneWhen: the plan's own pattern is
	// malformed. Plan text is a boundary input, so a bad glob is recorded
	// here and never crashes the drain.
	attrPlanWriteSet = "plan:write-set"
	attrPlanDoneWhen = "plan:done-when"
)

// removalPattern matches the wordings that make a "Done when" line
// undecidable by presence: absence is that line's success condition, so a
// missing path proves the step done rather than undone. A false FAIL costs a
// rework round; a false UNVERIFIABLE-HERE costs nothing the round did not
// already have.
//
// Whole words only, and here a hyphen or an underscore is part of a word
// rather than a boundary. On a substring test a "dropdown" or a "backdrop"
// reads as a removal; on Go's own \b, so do `deleted-items.ts`, `drop-shadow`
// and `unused-vars`, because \b treats "-" as a boundary and the file, class
// and rule names people actually write are full of them. The step would then
// be handed back with a reason about removing things its line never mentioned
// — a wrong explanation misleads a person as surely as a wrong verdict.
var removalPattern = regexp.MustCompile(
	`(?i)(?:^|[^\pL\pN_-])(?:remov(?:e|es|ed|ing|al)|delet(?:e|es|ed|ing|ion)|drop(?:s|ped|ping)?|gone|unused)(?:$|[^\pL\pN_-])|no longer`)

// treeIndex is a read-only listing of the verification workspace: every
// regular file with its size, plus the set of directories. Reading a fact
// must not create one, so nothing here writes to the tree (§59).
type treeIndex struct {
	files map[string]int64 // slash path relative to the root → size in bytes
	dirs  map[string]bool
	order []string // file paths, sorted, so examples are deterministic
}

// treeReadError reports a verification workspace the platform could not read.
// Its message names no host path: it is rendered into a contract's Detail,
// which a requester reads (§38), and an absolute path on this machine tells
// them nothing while saying more about the host than it should.
type treeReadError struct {
	// Rel is the path inside the workspace that failed, relative to its
	// root. Empty when the root itself could not be opened.
	Rel string
	Err error
}

func (e *treeReadError) Error() string {
	switch e.Rel {
	case "":
		return "the folder holding them could not be opened"
	case ".":
		// The failure is on the ROOT itself, whose relative path is ".".
		// Dropped into the sentence a requester reads, that dot says nothing
		// and only asks to be puzzled over, so the files themselves are named
		// instead. Deeper paths keep their workspace-relative name.
		return "the files in that folder could not be listed"
	}
	return fmt.Sprintf("%s inside them could not be read", e.Rel)
}

func (e *treeReadError) Unwrap() error { return e.Err }

// indexTree lists root. Symlinks inside the tree are neither followed nor
// listed (the S13 stripped revision drops them anyway) and any .git directory
// is skipped: answer-bearing history is not part of what the work produced. A
// read failure is returned rather than swallowed — an unreadable tree decides
// nothing.
func indexTree(root string) (*treeIndex, error) {
	// The ROOT's own symlinks are resolved first, because WalkDir does not
	// follow a symlinked root: it would visit the link itself, list nothing,
	// and hand back an EMPTY tree that refutes every contract in the plan
	// from files nobody ever read. A root that is not a readable directory
	// decides nothing at all rather than deciding everything wrongly.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, &treeReadError{Err: err}
	}
	st, err := os.Stat(resolved)
	if err != nil {
		return nil, &treeReadError{Err: err}
	}
	if !st.IsDir() {
		return nil, &treeReadError{Err: fmt.Errorf("%w: the verification workspace is not a directory", ErrBadInput)}
	}
	idx := &treeIndex{files: map[string]int64{}, dirs: map[string]bool{}}
	walkErr := filepath.WalkDir(resolved, func(p string, d fs.DirEntry, err error) error {
		rel, relErr := filepath.Rel(resolved, p)
		if relErr != nil {
			rel = filepath.Base(p)
		}
		rel = filepath.ToSlash(rel)
		if err != nil {
			return &treeReadError{Rel: rel, Err: err}
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			idx.dirs[rel] = true
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return &treeReadError{Rel: rel, Err: err}
		}
		idx.files[rel] = info.Size()
		idx.order = append(idx.order, rel)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(idx.order)
	return idx, nil
}

// match returns the indexed files pattern matches, in sorted order. Matching
// is segment-wise on "/": a "**" segment spans zero or more path segments,
// every other segment matches one segment through path.Match. A
// metacharacter-free pattern that names a directory matches everything under
// it.
//
// A malformed pattern is an error, never a panic and never a silent miss.
func (idx *treeIndex) match(pattern string) ([]string, error) {
	segs := strings.Split(pattern, "/")
	for _, s := range segs {
		if s == "**" {
			continue
		}
		if _, err := path.Match(s, ""); err != nil {
			return nil, fmt.Errorf("verify: step contract pattern %q: %w", pattern, err)
		}
	}
	if !strings.ContainsAny(pattern, "*?[") && idx.dirs[pattern] {
		segs = append(segs, "**")
	}
	var out []string
	for _, f := range idx.order {
		if matchSegments(segs, strings.Split(f, "/")) {
			out = append(out, f)
		}
	}
	return out, nil
}

// matchSegments matches the remaining pattern segments against the remaining
// path segments. Every segment was validated by match, so path.Match's error
// return cannot fire here.
func matchSegments(pat, seg []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(seg); i++ {
				if matchSegments(pat[1:], seg[i:]) {
					return true
				}
			}
			return false
		}
		if len(seg) == 0 {
			return false
		}
		ok, _ := path.Match(pat[0], seg[0])
		if !ok {
			return false
		}
		pat, seg = pat[1:], seg[1:]
	}
	return len(seg) == 0
}

// emptyNamedFile reports whether pattern names one existing file that holds
// no bytes — the structural fact behind "a named file must be there and have
// something in it". A glob is never judged this way.
func (idx *treeIndex) emptyNamedFile(pattern string) bool {
	if strings.ContainsAny(pattern, "*?[") {
		return false
	}
	size, ok := idx.files[pattern]
	return ok && size == 0
}

// normalizePattern puts a declared glob or a named path into the index's own
// shape: slash-separated, with a leading "./" dropped and a trailing slash
// read as "everything under here". A leading "/" is NOT stripped — an
// absolute pattern names somewhere other than this workspace, and quietly
// rebasing it onto the workspace would invent a claim the plan never made.
func normalizePattern(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "./")
	if strings.HasSuffix(p, "/") {
		if p = strings.TrimRight(p, "/"); p == "" {
			return ""
		}
		return p + "/**"
	}
	return p
}

// writeSetPatterns returns the step's BOUNDED write-set globs. A step that
// could not bound what it writes declares nothing the files can refute
// (Spec S02.8), so class W does not apply to it.
func writeSetPatterns(step intake.Step) []string {
	if step.Unbounded {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, g := range step.WriteSet {
		p := normalizePattern(g)
		if p == "" {
			// A pattern that normalizes away is NOT "this step declares no
			// files to write": "/" and "./" name a root. Kept exactly as the
			// plan wrote it, so the reason can name it and the undecidable
			// arm — never a refutation — decides it.
			p = strings.TrimSpace(g)
		}
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// unmeasurablePattern reports a declared glob these files can neither confirm
// nor refute. An absolute one names somewhere other than the workspace the
// platform was handed; one made of nothing but "." and "/" ("/", "./") names a
// whole root rather than any file the step promised. Both are recorded
// undecidable and named in the reason, never refuted — a claim about another
// place, or about no particular file, is not one this tree can disprove.
func unmeasurablePattern(p string) bool {
	return strings.HasPrefix(p, "/") || strings.Trim(p, "./") == ""
}

// namedPaths reads the paths a "Done when" line names in backticks, and
// reports whether the line speaks of removal. Only backtick-quoted spans are
// read, and only conservatively path-shaped ones are returned: an unquoted
// path, a bare filename, a stack name or a version is prose that the judge
// weighs, not a fact these files decide.
func namedPaths(doneWhen string) (paths []string, removal bool) {
	if removalPattern.MatchString(doneWhen) {
		return nil, true
	}
	seen := map[string]bool{}
	rest := doneWhen
	for {
		open := strings.IndexByte(rest, '`')
		if open < 0 {
			break
		}
		width := strings.IndexByte(rest[open+1:], '`')
		if width < 0 {
			break
		}
		span := strings.TrimSpace(rest[open+1 : open+1+width])
		rest = rest[open+width+2:]
		if !pathShaped(span) {
			continue
		}
		p := normalizePattern(span)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	return paths, false
}

// pathShaped reports whether a backtick span is conservatively a path or a
// glob rather than prose (Spec S07.3 — what cannot be decided from the files
// is recorded as such, never guessed).
//
// The bar is deliberately high, because a span wrongly read as a path FAILs
// work that is finished. A slash alone proves nothing: a route (`/cart`,
// `/api/products`), an import specifier (`next/image`, `node:fs/promises`), a
// version (`v1.2/3`) and ordinary slashed prose (`and/or`, `24/7`, `on/off`)
// all carry one and none of them names a file. So a span qualifies only when
// it holds no whitespace and none of the characters that mark a URL, a
// command or an expression; carries no "#" (a link fragment: a place in a
// document, not a file on disk) and no backslash (a Windows-shaped or escaped
// span this slash-separated index cannot address); has
// no ".." segment and no trailing "." (a path that climbs out of the workspace
// decides nothing here, and a sentence-ending dot is punctuation); does not
// begin with "/", since a route is not a file and an absolute path is not this
// workspace's; holds at least one "/"; and finally looks like a file or a
// folder — a glob metacharacter anywhere, a real extension on its LAST segment,
// or a trailing slash.
//
// What it still admits, deliberately: `example.com/index.html` reads as a
// path, because a first segment with a dot in it is exactly the shape of a
// real folder (a site directory in a multi-site repo, a dotted package
// directory), and rejecting the shape would lose true paths to catch a
// hostname. A domain is the one impostor this rule does not claim to catch.
func pathShaped(span string) bool {
	if span == "" || strings.ContainsAny(span, " \t\r\n\v\f") {
		return false
	}
	if strings.ContainsAny(span, ":(){}$=#\\") {
		return false
	}
	if strings.HasPrefix(span, "/") || !strings.Contains(span, "/") {
		return false
	}
	if strings.HasSuffix(span, ".") {
		return false
	}
	for _, seg := range strings.Split(span, "/") {
		if seg == ".." {
			return false
		}
	}
	if strings.ContainsAny(span, "*?[") || strings.HasSuffix(span, "/") {
		return true
	}
	return hasExtension(span[strings.LastIndexByte(span, '/')+1:])
}

// hasExtension reports whether a last path segment ends in something that
// reads as a file extension: a non-empty name, then a dot, then two or more
// letters-and-digits holding at least one LETTER.
//
// Each clause buys a rejection the plain "there is a dot in it" test made:
// the letter kills a version (`api/v1.0`, `0.5/1.0` — ".0" ends nothing), the
// two-character minimum kills an abbreviation (`e.g/i.e`), the non-empty name
// and non-empty extension kill `a/b.` and a bare dotfile. The price is a
// one-letter extension (`src/main.c`) read as prose and left undecided, which
// is the safe direction: an undecided contract costs a reading, a wrong one
// FAILs finished work.
func hasExtension(last string) bool {
	dot := strings.LastIndexByte(last, '.')
	if dot <= 0 || dot == len(last)-1 {
		return false
	}
	ext := last[dot+1:]
	if len(ext) < 2 {
		return false
	}
	letter := false
	for _, r := range ext {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			letter = true
		case r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return letter
}

// refutation is one pattern the produced files did not satisfy, carrying the
// plain-words phrase that says so.
type refutation struct {
	pattern string
	phrase  string
}

// decideFromTree decides one step's contract from the produced files
// (Spec S07.3; Spec S07.8 [A16]). It is branch-agnostic — nothing in it
// knows which posture called it — and idx is nil exactly when walkErr is
// set.
//
// Precedence: a refutation FAILs; otherwise an unreadable tree, then a
// malformed pattern, leave the contract undecided with the reason; a class
// that applied and was not refuted PASSes; a line no class applies to stays
// undecided attributed to the absent pack, exactly as before.
func decideFromTree(step intake.Step, idx *treeIndex, walkErr error) StepContract {
	sc := StepContract{
		StepID:   step.ID,
		DoneWhen: step.DoneWhen,
		Category: CatACBlocker,
		Route:    RouteTable[CatACBlocker].Sink,
	}
	writes := writeSetPatterns(step)
	named, removal := namedPaths(step.DoneWhen)
	if len(writes) == 0 && len(named) == 0 {
		sc.State = ContractUnverifiable
		sc.AttributedTo = BootstrapAttribution
		sc.Detail = undecidedDetail(removal)
		return sc
	}
	if walkErr != nil {
		sc.State = ContractUnverifiable
		sc.AttributedTo = attrWorkspaceUnreadable
		sc.Detail = fmt.Sprintf("The files this work produced could not be read, so nothing about this step was decided from them: %v.", walkErr)
		return sc
	}

	var refuted []refutation
	var malformed []string
	var facts []string
	badPatternAttribution := ""
	// A pattern the write set and the line BOTH name is one outcome, said
	// once: a person reading the reason should not meet the same path twice.
	// The dedupe is per outcome, not per pattern, because the write set is
	// judged as a union and may leave a pattern unspoken for that the line
	// then decides on its own.
	saidRefuted, saidFact, saidMalformed := map[string]bool{}, map[string]bool{}, map[string]bool{}
	addRefuted := func(pattern, phrase string) {
		if saidRefuted[pattern] {
			return
		}
		saidRefuted[pattern] = true
		refuted = append(refuted, refutation{pattern: pattern, phrase: phrase})
	}
	addMalformed := func(pattern, attribution string) {
		if badPatternAttribution == "" {
			badPatternAttribution = attribution
		}
		if saidMalformed[pattern] {
			return
		}
		saidMalformed[pattern] = true
		malformed = append(malformed, pattern)
	}

	// Class W: the declared write set, judged as one union.
	if len(writes) > 0 {
		matched, bad := 0, false
		type wFact struct {
			pattern string
			matches []string
		}
		var wFacts []wFact
		for _, p := range writes {
			m, err := idx.match(p)
			if unmeasurablePattern(p) || err != nil {
				bad = true
				addMalformed(p, attrPlanWriteSet)
				continue
			}
			if len(m) > 0 {
				matched += len(m)
				wFacts = append(wFacts, wFact{pattern: p, matches: m})
			}
		}
		switch {
		case matched > 0:
			for _, f := range wFacts {
				if !saidFact[f.pattern] {
					saidFact[f.pattern] = true
					facts = append(facts, factPhrase(f.pattern, f.matches))
				}
			}
		case !bad:
			for _, p := range writes {
				addRefuted(p, fmt.Sprintf("the step said it would write %s and no file it produced matches that", p))
			}
		}
	}

	// Class N: the paths the line names, each judged on its own.
	for _, p := range named {
		m, err := idx.match(p)
		switch {
		case err != nil:
			addMalformed(p, attrPlanDoneWhen)
		case len(m) == 0:
			addRefuted(p, fmt.Sprintf("the line names %s and no file it produced matches that", p))
		case idx.emptyNamedFile(p):
			addRefuted(p, fmt.Sprintf("the line names %s and that file is there but empty", p))
		default:
			if !saidFact[p] {
				saidFact[p] = true
				facts = append(facts, factPhrase(p, m))
			}
		}
	}

	switch {
	case len(refuted) > 0:
		sc.State = ContractFail
		sc.AttributedTo = contractTreeAttribution + refuted[0].pattern
		sc.Detail = failDetail(refuted)
	case len(malformed) > 0:
		sc.State = ContractUnverifiable
		sc.AttributedTo = badPatternAttribution
		sc.Detail = malformedDetail(malformed)
	default:
		sc.State = ContractPass
		sc.Detail = passDetail(facts)
	}
	return sc
}

// factPhrase states what one pattern matched, naming at most
// contractDetailMaxExamples example paths.
func factPhrase(pattern string, matches []string) string {
	noun := "files"
	if len(matches) == 1 {
		noun = "file"
	}
	shown, more := matches, 0
	if len(shown) > contractDetailMaxExamples {
		more = len(shown) - contractDetailMaxExamples
		shown = shown[:contractDetailMaxExamples]
	}
	list := strings.Join(shown, ", ")
	if more > 0 {
		list = fmt.Sprintf("%s and %d more", list, more)
	}
	return fmt.Sprintf("%s is matched by %d %s (%s)", pattern, len(matches), noun, list)
}

// failDetail says, in the requester's words, what the produced files failed
// to show.
func failDetail(refuted []refutation) string {
	phrases := make([]string, 0, len(refuted))
	for _, r := range refuted {
		phrases = append(phrases, r.phrase)
	}
	return "The files this work produced do not show this step finished: " + strings.Join(phrases, "; ") + "."
}

// passDetail names the facts that were decided and leaves everything the
// line asks beyond them where it belongs — the reviewing judge and the
// person (the ladder's own PASS reading, Spec S07.3).
func passDetail(facts []string) string {
	return "Decided from the files this work produced: " + strings.Join(facts, "; ") +
		". Anything this step's line asks for beyond those files is left to the reviewing judge and to you."
}

// malformedDetail records a plan pattern these files cannot be measured
// against — said plainly, and never as a verdict on the work.
func malformedDetail(patterns []string) string {
	noun := "pattern"
	if len(patterns) > 1 {
		noun = "patterns"
	}
	return fmt.Sprintf("Nothing here decides this step: the plan's own file %s %s could not be matched against the files this work produced, so they were not measured against it.",
		noun, joinList(patterns))
}

// undecidedDetail says why the produced files decide nothing about a step.
func undecidedDetail(removal bool) string {
	if removal {
		return "Nothing in the files decides this step: its line is about removing something, and the files that are there cannot show what was taken away. Only reading the work settles it."
	}
	return "Nothing in the files decides this step: it declares no files to write and names no file or folder, so only reading the work settles whether it is done."
}

// joinList renders a short list inside a sentence a person reads.
func joinList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// contractFinding is the one blocker a refuted contract raises, so the
// refutation reaches a person instead of dying in a log (Spec S07.7: every
// verification finding terminates in a human-visible sink).
//
// It cites the lowest-numbered frozen criterion the plan's coverage map
// gives this step (Spec S06.6). A step no criterion covers cites nothing,
// which by the S07.5 citation rule can only be a note — validateFindings
// demotes it, and it still reaches the requester as a review comment.
//
// The finding key (criterion, step anchor, category) is round-stable, so a
// refutation that survives rework recurs unresolved and trips the S07.6
// convergence stop rather than drifting into a new goalpost each round.
func contractFinding(sc StepContract, step intake.Step, coverage map[string][]string) Finding {
	return Finding{
		Severity:  SeverityBlocker,
		Category:  CatACBlocker,
		Criterion: coveringCriterion(step.ID, coverage),
		Anchor:    "step:" + sc.StepID,
		Text: fmt.Sprintf("Step %s of the approved plan is not finished. It was agreed done when: %s. %s The approved plan is what this work is measured against, so it is recorded as not done rather than passed.",
			sc.StepID, strings.TrimRight(strings.TrimSpace(sc.DoneWhen), "."), sc.Detail),
	}
}

// coveringCriterion returns the lowest-numbered frozen criterion whose
// coverage entry names the step, or "" when none does.
func coveringCriterion(stepID string, coverage map[string][]string) string {
	var keys []string
	for ac, steps := range coverage {
		for _, s := range steps {
			if s == stepID {
				keys = append(keys, ac)
				break
			}
		}
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Slice(keys, func(i, j int) bool {
		ni, oki := acNumber(keys[i])
		nj, okj := acNumber(keys[j])
		switch {
		case oki && okj:
			return ni < nj
		case oki != okj:
			return oki
		default:
			return keys[i] < keys[j]
		}
	})
	return keys[0]
}

// acNumber parses the "AC-<n>" criterion form the citation rule validates
// against.
func acNumber(key string) (int, bool) {
	rest, ok := strings.CutPrefix(key, "AC-")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0, false
	}
	return n, true
}

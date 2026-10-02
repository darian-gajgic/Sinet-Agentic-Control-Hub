package verify

import (
	"context"
	"fmt"
	"strings"
)

// The judge's input slice for a REPO-BACKED deliverable (Spec S07.5: "the
// artifact + its diff against the previous revision [S13] ... never the
// execution transcript"; S07.9 P-T06-3: judges receive extracted claims/diffs,
// not presentation). A repo-backed revision IS its tree at the snapshot pin
// (CONVENTIONS §78), so what the judge may quote is the reviewable CHANGE —
// the file inventory, the per-file diffs (an added file's diff is its
// content), and the full content of modified files — read from the
// platform-owned store at the pinned refs through the seam below. The step
// report the executor wrote is what the executor SAYS it did: it rides the
// slice as a labelled claims item and is never the artifact.

// ChangeSource is the S13 tree seam of the judge's input slice: the
// reviewable change of one minted revision, from the platform-owned project
// store at the pinned refs (Spec S13.1/S13.2; §78's read-only verbs) — NEVER
// the sandbox or the stripped verification workspace (Spec S07.3 rule 1).
// The stage's review sink implements it over review.Store; verify never
// imports review (CONVENTIONS §22). Nil on the Verifier keeps today's slice.
type ChangeSource interface {
	// RevisionChange returns the change of revision d.Revision against its
	// previous revision (revision 1 against the recorded pre-task base, Spec
	// S13.1), pins taken from the review store's own revision rows. The lane
	// is chosen by the PIN, never by the type (§78): a revision that pins no
	// snapshot answers a RevisionChange whose AbsentReason says so, and the
	// slice keeps today's shape. bodyBudget bounds the bytes of diff/content
	// bodies the seam serves: a body that does not fit what is LEFT of the
	// budget is not served and costs the budget nothing — its row carries
	// BodySkipped (its diff) or ContentSkipped (its content), since the judge
	// is shown whole files or none of a file. Every modified/renamed text row
	// whose content is not served carries ContentSkipped. A diff the store
	// could compare only in part is never served and costs nothing: its row
	// carries DiffTruncated with no Diff, and it is not a cut. The bodies are
	// served in the order RenderChangeSlice shows them, so a body the judge
	// is shown is never priced against one it is not. The inventory is
	// always whole. A pin the store no longer holds is an ERROR (content
	// drift), never a fall-through to the report.
	RevisionChange(ctx context.Context, d Deliverable, bodyBudget int) (RevisionChange, error)
}

// Change kinds of an inventory row — the same words review serves (one
// vocabulary, two readers; the internal/project precedent).
const (
	KindAdded    = "added"
	KindModified = "modified"
	KindDeleted  = "deleted"
	KindRenamed  = "renamed"
)

// ChangedFile is one row of the change inventory plus the bodies the judge
// may quote from it.
type ChangedFile struct {
	Path string `json:"path"`
	// OldPath is set for a rename only.
	OldPath string `json:"old_path,omitempty"`
	Kind    string `json:"kind"`
	OldSize int64  `json:"old_size"`
	NewSize int64  `json:"new_size"`
	// Binary is git's own verdict: an inventory row, no diff, no content, and
	// its bytes never reach the judge.
	Binary    bool `json:"binary"`
	Additions int  `json:"additions,omitempty"`
	Deletions int  `json:"deletions,omitempty"`

	// Diff is the file's unified diff between the two pins by the one diff
	// authority (review.gitDiff), bounded by review's per-file cap and
	// honest about it. An added file's diff IS its content. DiffReason is the
	// SOURCE's own sentence for a body it could not serve whole; it is a
	// review-page sentence written for a person who can open the file, so the
	// renderer states the cause to the judge in its own words and this text
	// never reaches the wire (CONVENTIONS §38).
	Diff          string `json:"diff,omitempty"`
	DiffTruncated bool   `json:"diff_truncated,omitempty"`
	DiffReason    string `json:"diff_reason,omitempty"`
	// Content is the new-side full content of a MODIFIED or RENAMED text file
	// (the diff alone shows hunks; the judge needs the whole file to quote
	// from), bounded by review's content cap. Empty for added files (their
	// diff is their content), deleted files and binaries.
	Content          string `json:"content,omitempty"`
	ContentTruncated bool   `json:"content_truncated,omitempty"`
	ContentReason    string `json:"content_reason,omitempty"`
	// BodySkipped marks a row whose diff body the seam did not serve because
	// it did not fit what was left of its body budget: the inventory row
	// stands, and the judge sees the file NAMED among the omitted.
	// ContentSkipped marks a modified/renamed row whose new-side content the
	// seam did not serve: only its content is omitted, so a diff the judge
	// can be shown is never dropped for a content it cannot, and a content
	// that fits is never dropped for a diff that did not.
	BodySkipped    bool `json:"body_skipped,omitempty"`
	ContentSkipped bool `json:"content_skipped,omitempty"`
}

// RevisionChange is the reviewable change of one revision pair, as the seam
// returns it.
type RevisionChange struct {
	OldN int `json:"old_n"`
	NewN int `json:"new_n"`
	// OldPin/NewPin are the snapshot commits compared; OldIsBase marks the
	// pre-task base as the old side (revision 1).
	OldPin    string `json:"old_pin,omitempty"`
	NewPin    string `json:"new_pin"`
	OldIsBase bool   `json:"old_is_base,omitempty"`
	// Files is the whole inventory in path order — never truncated.
	Files []ChangedFile `json:"files"`
	// AbsentReason, non-empty, says why there is no tree slice (the revision
	// pins no snapshot; no pre-task base recorded; no tree source composed):
	// the slice keeps today's content shape and the reason is RECORDED.
	AbsentReason string `json:"absent_reason,omitempty"`
}

// JudgeArtifactBytesCap bounds the BODIES of the tree slice on the judge's
// wire — the per-file diffs plus the full content of modified files, together
// — so the whole judge prompt stays inside the ratified stage-fit budget
// (Spec S05.3: ≤ ⚙ context.stage_fit_target of the seat's window). A
// structural constant with its reason, not a ⚙ (S18 ratifies no key; the
// §78 sibling caps' precedent; settings-tab ledger):
//
//	judge seat window 200k tokens (worker.DefaultWindowTokens)
//	× stage_fit_target 0.50 = 100k tokens for the whole prompt;
//	192 KiB of code-shaped text ≈ 65k tokens at ~3 bytes/token (conservative
//	— code tokenizes denser than prose), leaving ≥ 35k tokens for the frozen
//	ACs, the rubric, the V1 outcomes, prior findings, the executor's report,
//	the axis instructions and the answer.
//
// The §78 caps (512 KiB whole change / 256 KiB per file / 1 MiB content) are
// WIRE caps for a person reading one page; this is the judge's own bound. The
// inventory is never bounded; the cut lands at a FILE boundary; every cut is
// said on the wire and recorded (JudgeSaw). The S05.3 budget watcher stays
// the measured backstop (FitExceeded is recorded, never silent).
const JudgeArtifactBytesCap = 192 << 10

// JudgeSaw records, on the round record and the verify.round row, what the
// judge's artifact slice held — so a verdict can be read against its evidence
// (Spec S07.11 "every verdict is recorded with its reasons").
type JudgeSaw struct {
	// Kind is "content" (the artifact-of-record text — today's shape) or
	// "tree" (inventory + diffs + content of a repo-backed revision).
	Kind string `json:"kind"`
	// AbsentReason, on a content-kind slice of a Change-wired drain, is the
	// seam's stated reason for having no tree slice.
	AbsentReason string `json:"absent_reason,omitempty"`

	OldN      int    `json:"old_n,omitempty"`
	NewN      int    `json:"new_n,omitempty"`
	OldPin    string `json:"old_pin,omitempty"`
	NewPin    string `json:"new_pin,omitempty"`
	OldIsBase bool   `json:"old_is_base,omitempty"`

	// Files is the inventory size (every row is always shown).
	Files int `json:"files"`
	// DiffsShown / DiffsOmitted: rows with text whose diff the judge saw
	// whole, and — by path — those it did not see, each named on the wire
	// with its reason: cut at the bound, too large to be compared, or no
	// text changed (only the file's mode or path). A file added or removed
	// EMPTY has no text and is in neither list.
	DiffsShown   int      `json:"diffs_shown"`
	DiffsOmitted []string `json:"diffs_omitted,omitempty"`
	// ContentShown / ContentOmitted: modified/renamed text rows whose full
	// content the judge saw, and those it did not (cut at the bound, or no
	// whole copy available).
	ContentShown   int      `json:"content_shown"`
	ContentOmitted []string `json:"content_omitted,omitempty"`

	DiffBytes     int `json:"diff_bytes"`
	ContentBytes  int `json:"content_bytes"`
	ArtifactBytes int `json:"artifact_bytes"`
	// ReportBytes is the size of the executor's report shown as claims.
	ReportBytes int `json:"report_bytes,omitempty"`
	// Truncated is true when any file is named in DiffsOmitted or
	// ContentOmitted (Spec S07.11; R9) — a mode-only or pure-rename row
	// included, since it is named there.
	Truncated bool `json:"truncated"`
}

// Kinds of judge slice, as JudgeSaw.Kind records them.
const (
	sawTree    = "tree"
	sawContent = "content"
)

// JudgeSlice is what one judged round puts in front of the judge: the
// quotable artifact, its diff, and — on a repo-backed revision — the
// executor's report as labelled claims. The drain builds it once per round
// (pipeline.go) and BuildJudgeInput turns it into the S07.5 Extra items.
type JudgeSlice struct {
	Artifact string
	Diff     string
	// Report is the executor's step report, shown as claims and never
	// quotable. Empty on a content-pinned revision, whose artifact of record
	// IS the content.
	Report string
	// Saw is what the slice holds, for the round record (Spec S07.11).
	Saw JudgeSaw
}

// Quotable is the text an axis-1 evidence quote must be an exact substring of
// (Spec S07.5 "mandatory extractive evidence quote"): the artifact item, plus
// the diff item when there is one. The executor's report is outside it by
// construction — a judge that can only repeat what the executor claimed has
// checked nothing (Spec S07.9 P-T06-3).
func (in JudgeInput) Quotable() string {
	if in.Diff == "" {
		return in.Artifact
	}
	return in.Artifact + "\n" + in.Diff
}

// RenderChangeSlice renders a RevisionChange into the two judge items — the
// quotable ARTIFACT (the whole inventory, the full content of the modified
// files that fit, and the honest omitted lists) and the DIFF (the shown
// per-file unified diffs in path order, concatenated: their own headers name
// the files) — under JudgeArtifactBytesCap, and reports what it showed. Pure.
//
// The bound falls on a FILE boundary, twice: the diffs are shown in path
// order until one does not fit and everything from there on is omitted (a
// contiguous path-order prefix, R3), then the contents are shown under what
// is left, by the same rule. Part of a file's diff would describe a change
// the change does not make, which is worse than naming the file and showing
// nothing (the review.changeDiff reason, CONVENTIONS §78) — so a diff the
// source could serve only in part is never shown either.
func RenderChangeSlice(rc RevisionChange) (string, string, JudgeSaw) {
	saw := JudgeSaw{
		Kind:      sawTree,
		OldN:      rc.OldN,
		NewN:      rc.NewN,
		OldPin:    rc.OldPin,
		NewPin:    rc.NewPin,
		OldIsBase: rc.OldIsBase,
		Files:     len(rc.Files),
	}
	notes := map[string]rowNotes{}
	note := func(path string, set func(*rowNotes)) {
		n := notes[path]
		set(&n)
		notes[path] = n
	}
	bound := false // the judge's own bound cut the DIFF section (the content section says its own)
	var diff strings.Builder
	cut := false
	for _, row := range rc.Files {
		// A binary file is an inventory row and nothing else: its bytes never
		// reach the judge (Spec S13.2, G2 Def.14).
		if row.Binary {
			continue
		}
		// EVERY file with text is accounted for: shown whole, or named among
		// the omitted WITH the reason it is missing. A row whose diff body
		// came back empty — review could not compare two over-cap sides, or
		// nothing but the file's mode or path changed — is a file the judge
		// did not see, and letting it pass silently made the wire say the
		// rest were "all there in full" (Spec S07.5; §78 honest about the
		// bound). Only the BOUND cuts the rest of the section: a file with no
		// text to serve says so and the files after it are still shown.
		switch {
		case emptyFile(row):
			// A file added or removed EMPTY has no text: there is no change
			// to show, nothing is missing, and the row says so.
			note(row.Path, func(n *rowNotes) { n.diff = emptyFileNote(row) })
		case !row.BodySkipped && (row.Diff == "" || row.DiffTruncated):
			// No whole diff was served for this row, so the bound is not why
			// it is missing — even past the cut, the true reason is the one
			// said here.
			saw.DiffsOmitted = append(saw.DiffsOmitted, row.Path)
			note(row.Path, func(n *rowNotes) { n.diff = noDiffNote(row) })
		case cut || row.BodySkipped:
			past := cut
			cut, bound = true, true
			saw.DiffsOmitted = append(saw.DiffsOmitted, row.Path)
			note(row.Path, func(n *rowNotes) { n.diff = boundNote(false, past) })
		case saw.DiffBytes+len(row.Diff) > JudgeArtifactBytesCap:
			cut, bound = true, true
			saw.DiffsOmitted = append(saw.DiffsOmitted, row.Path)
			note(row.Path, func(n *rowNotes) { n.diff = boundNote(false, false) })
		default:
			diff.WriteString(row.Diff)
			saw.DiffBytes += len(row.Diff)
			saw.DiffsShown++
		}
	}
	// The contents come out of what the diffs left, in the same order under
	// the same rule. A modified file's content is what lets the judge quote
	// outside the hunks, so it is carried — and it is the first thing the
	// bound drops. A file emptied in place has no text at this version, which
	// is an ANSWER the judge needs: it is shown, as the empty file it is.
	contents := make([]ChangedFile, 0, len(rc.Files))
	cut = false
	for _, row := range rc.Files {
		if row.Binary || (row.Kind != KindModified && row.Kind != KindRenamed) {
			continue
		}
		switch {
		case row.ContentSkipped:
			past := cut
			cut = true
			saw.ContentOmitted = append(saw.ContentOmitted, row.Path)
			note(row.Path, func(n *rowNotes) { n.content = boundNote(true, past) })
		case row.Content == "" && row.ContentTruncated:
			saw.ContentOmitted = append(saw.ContentOmitted, row.Path)
			note(row.Path, func(n *rowNotes) { n.content = noWholeContentNote })
		case row.Content == "":
			// No text came back for this file: it is shown as what it is,
			// which costs the bound nothing and is an answer the judge needs.
			contents = append(contents, row)
			saw.ContentShown++
		case cut || saw.DiffBytes+saw.ContentBytes+len(row.Content) > JudgeArtifactBytesCap:
			past := cut
			cut = true
			saw.ContentOmitted = append(saw.ContentOmitted, row.Path)
			note(row.Path, func(n *rowNotes) { n.content = boundNote(true, past) })
		default:
			contents = append(contents, row)
			saw.ContentBytes += len(row.Content)
			saw.ContentShown++
		}
	}
	saw.Truncated = len(saw.DiffsOmitted)+len(saw.ContentOmitted) > 0
	artifact := renderChangeArtifact(rc, saw, contents, notes, bound)
	saw.ArtifactBytes = len(artifact)
	return artifact, diff.String(), saw
}

// rowNotes are the judge-facing sentences that ride an inventory row: why
// this file's changes, or its content, are not in the slice (or are in it
// only in part).
type rowNotes struct{ diff, content string }

// The renderer says WHY in its own words. Review's per-file truncation
// reasons are review-PAGE sentences written for a person who can click the
// file open ("open the file to read it"); the judge can open nothing, so
// those sentences never enter the slice — only the fact they carry does
// (CONVENTIONS §38: say it in words the reader can act on).
const (
	noDiffTooLarge     = "its changes are NOT shown: the file is too large to be compared in one piece here"
	noDiffSameText     = "its changes are NOT shown: both versions hold the same text — what changed is the file's mode or its path"
	noWholeContentNote = "its content is NOT shown: no whole copy of its text was available here"
	emptyContentLine   = "(no text: the platform's copy served none for this file at this version — the sizes on its row above say whether the file is now empty)"
)

// boundNote is the judge's OWN bound speaking. A body that did not fit what
// was left says so; a body PAST the cut says that instead, because it may
// well have fitted and "it did not fit" would be untrue of it.
func boundNote(content, past bool) string {
	what := "its changes are"
	if content {
		what = "its whole content is"
	}
	if past {
		return what + " NOT shown: the slice was already cut at an earlier file in this list, and nothing after that point is shown"
	}
	return fmt.Sprintf("%s NOT shown: it did not fit the %d KB of file text this judge reads under",
		what, JudgeArtifactBytesCap>>10)
}

// emptyFile is a file added or removed with no text at all: git's diff of an
// empty blob against nothing is empty, and that is the whole change.
func emptyFile(row ChangedFile) bool {
	if row.Diff != "" || row.BodySkipped || row.DiffTruncated {
		return false
	}
	return (row.Kind == KindAdded && row.NewSize == 0) || (row.Kind == KindDeleted && row.OldSize == 0)
}

func emptyFileNote(row ChangedFile) string {
	if row.Kind == KindAdded {
		return "it has no changes to show: the file was added empty"
	}
	return "it has no changes to show: the file was empty when it was removed"
}

// noDiffNote says why a file WITH text has no diff body in the slice.
func noDiffNote(row ChangedFile) string {
	if row.DiffTruncated {
		return noDiffTooLarge
	}
	return noDiffSameText
}

// renderChangeArtifact writes the quotable artifact item: what this is, the
// whole inventory, then — in plain words — how much of the change the judge
// is being shown, which files were left out, and the contents that fit
// (CONVENTIONS §38: the bound is said, never silent).
func renderChangeArtifact(rc RevisionChange, saw JudgeSaw, contents []ChangedFile, notes map[string]rowNotes, bound bool) string {
	var sb strings.Builder
	sb.WriteString("The work under judgment is the change this version makes to the project's files, read from the platform's own copy of the project at the recorded commits.\n\n")
	fmt.Fprintf(&sb, "Version %d (commit %s) compared with %s.\n\n", rc.NewN, rc.NewPin, oldSideOf(rc))
	if len(rc.Files) == 0 {
		sb.WriteString("No file differs between the two: this version changes nothing in the project.\n")
		return sb.String()
	}
	fmt.Fprintf(&sb, "%d %s changed. Every one of them is listed here; this list is never shortened.\n\n",
		len(rc.Files), filesWord(len(rc.Files)))
	for _, row := range rc.Files {
		writeInventoryRow(&sb, row, notes[row.Path])
	}

	// Every file with text is either in the diff item whole or named here as
	// missing, with the reason on its own line above: the judge is never left
	// to infer from silence what it was not shown.
	diffable := saw.DiffsShown + len(saw.DiffsOmitted)
	sb.WriteByte('\n')
	switch {
	case diffable == 0:
		sb.WriteString("No file in this change has text to compare, so the verify/diff item is empty.\n")
	case len(saw.DiffsOmitted) == 0:
		fmt.Fprintf(&sb, "The file-by-file changes are in the verify/diff item: all %d %s with text are there in full, in the order listed above.\n",
			diffable, filesWord(diffable))
	case saw.DiffsShown == 0:
		fmt.Fprintf(&sb, "The verify/diff item is EMPTY: not one of the %d %s with text is shown. Each one says why on its own line above.\n",
			diffable, filesWord(diffable))
	default:
		fmt.Fprintf(&sb, "The file-by-file changes are in the verify/diff item: %d of %d %s with text are there in full, in the order listed above. Each file left out says why on its own line above.\n",
			saw.DiffsShown, diffable, filesWord(diffable))
	}
	if len(saw.DiffsOmitted) > 0 {
		if bound {
			fmt.Fprintf(&sb, "A file is shown whole or not at all — part of a file's changes would describe a change this version does not make — so what did not fit the %d KB of file text this judge reads under was left out whole, and so were the changes of every file after it.\n",
				JudgeArtifactBytesCap>>10)
		}
		fmt.Fprintf(&sb, "Changes NOT shown, by file: %s\n", strings.Join(saw.DiffsOmitted, ", "))
	}

	contentRows := saw.ContentShown + len(saw.ContentOmitted)
	if contentRows == 0 {
		return sb.String()
	}
	sb.WriteByte('\n')
	switch {
	case len(saw.ContentOmitted) == 0:
		fmt.Fprintf(&sb, "The whole new content of every file changed in place follows (%d of %d).\n", saw.ContentShown, contentRows)
	case saw.ContentShown == 0:
		fmt.Fprintf(&sb, "The whole content of none of the %d %s changed in place is here; each one says why on its own line above.\n",
			contentRows, filesWord(contentRows))
	default:
		fmt.Fprintf(&sb, "The whole new content of %d of the %d files changed in place follows; each one left out says why on its own line above.\n",
			saw.ContentShown, contentRows)
	}
	if len(saw.ContentOmitted) > 0 {
		fmt.Fprintf(&sb, "Content NOT shown, by file: %s\n", strings.Join(saw.ContentOmitted, ", "))
	}
	for _, row := range contents {
		fmt.Fprintf(&sb, "\n----- %s, the whole file at version %d -----\n", row.Path, rc.NewN)
		if row.Content == "" {
			sb.WriteString(emptyContentLine + "\n")
		} else {
			sb.WriteString(row.Content)
			if !strings.HasSuffix(row.Content, "\n") {
				sb.WriteByte('\n')
			}
		}
		fmt.Fprintf(&sb, "----- end %s -----\n", row.Path)
	}
	return sb.String()
}

// writeInventoryRow writes one inventory line: what happened to the file, its
// sizes, and — when a body of it is missing from the slice or is in it only
// in part — the renderer's own sentence saying so.
func writeInventoryRow(sb *strings.Builder, f ChangedFile, n rowNotes) {
	fmt.Fprintf(sb, "  %-9s %s", f.Kind, f.Path)
	if f.OldPath != "" {
		fmt.Fprintf(sb, " (it was %s)", f.OldPath)
	}
	switch {
	case f.Binary:
		sb.WriteString(" — a binary file: it gets this row and nothing more, and its bytes are never shown")
	case f.Kind == KindAdded:
		fmt.Fprintf(sb, " — new, %d bytes, %d %s added", f.NewSize, f.Additions, linesWord(f.Additions))
	case f.Kind == KindDeleted:
		fmt.Fprintf(sb, " — removed, it was %d bytes, %d %s deleted", f.OldSize, f.Deletions, linesWord(f.Deletions))
	default:
		fmt.Fprintf(sb, " — was %d bytes, now %d bytes; %d %s added, %d %s deleted",
			f.OldSize, f.NewSize, f.Additions, linesWord(f.Additions), f.Deletions, linesWord(f.Deletions))
	}
	sb.WriteByte('\n')
	if n.diff != "" {
		fmt.Fprintf(sb, "            %s\n", n.diff)
	}
	if n.content != "" {
		fmt.Fprintf(sb, "            %s\n", n.content)
	}
}

func oldSideOf(rc RevisionChange) string {
	if rc.OldIsBase {
		return fmt.Sprintf("the state the project was in before this task started (commit %s)", rc.OldPin)
	}
	return fmt.Sprintf("version %d (commit %s)", rc.OldN, rc.OldPin)
}

func filesWord(n int) string {
	if n == 1 {
		return "file"
	}
	return "files"
}

func linesWord(n int) string {
	if n == 1 {
		return "line"
	}
	return "lines"
}

// changeSlice is the judge slice of a repo-backed revision: the rendered
// change is the artifact, and the executor's report rides beside it as claims
// (Spec S07.5; S07.9 P-T06-3).
func changeSlice(rc RevisionChange, report string) JudgeSlice {
	artifact, diff, saw := RenderChangeSlice(rc)
	saw.ReportBytes = len(report)
	return JudgeSlice{Artifact: artifact, Diff: diff, Report: report, Saw: saw}
}

// contentSlice is the judge slice of a content-pinned revision — the
// artifact of record as its own quotable text, byte for byte what the judge
// has always received. absentReason carries the seam's own sentence for why
// there is no tree slice, which is an ANSWER and is recorded (CONVENTIONS
// §78); it is empty when no seam was asked at all.
func contentSlice(d Deliverable, absentReason string) JudgeSlice {
	return JudgeSlice{
		Artifact: d.Content,
		Diff:     d.Diff,
		Saw: JudgeSaw{
			Kind:          sawContent,
			AbsentReason:  absentReason,
			ArtifactBytes: len(d.Content),
		},
	}
}

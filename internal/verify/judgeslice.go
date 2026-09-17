package verify

import "context"

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
//
// INERT TYPE SURFACE at P3-TQ-8 grounding (CONVENTIONS §3 Amendment-A
// carve-out): the types and the seam exist so the committed acceptance tests
// compile; nothing here is consumed yet. The executor packet wires and fills
// them.

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
	// bodies the seam reads (rows past it keep their inventory data and carry
	// BodySkipped); the inventory is always whole. A pin the store no longer
	// holds is an ERROR (content drift), never a fall-through to the report.
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
	// honest about it. An added file's diff IS its content.
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
	// BodySkipped marks a row whose bodies the seam did not read because its
	// body budget was already spent: the inventory row stands, the judge sees
	// the file NAMED among the omitted.
	BodySkipped bool `json:"body_skipped,omitempty"`
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
	// DiffsShown / DiffsOmitted: diffable (non-binary) rows whose diff the
	// judge saw whole, and — by path — those omitted at the bound.
	DiffsShown   int      `json:"diffs_shown"`
	DiffsOmitted []string `json:"diffs_omitted,omitempty"`
	// ContentShown / ContentOmitted: modified/renamed text rows whose full
	// content the judge saw, and those omitted at the bound.
	ContentShown   int      `json:"content_shown"`
	ContentOmitted []string `json:"content_omitted,omitempty"`

	DiffBytes     int `json:"diff_bytes"`
	ContentBytes  int `json:"content_bytes"`
	ArtifactBytes int `json:"artifact_bytes"`
	// ReportBytes is the size of the executor's report shown as claims.
	ReportBytes int `json:"report_bytes,omitempty"`
	// Truncated is true when any diff or content body was omitted.
	Truncated bool `json:"truncated"`
}

// RenderChangeSlice renders a RevisionChange into the two judge items — the
// quotable ARTIFACT (inventory, the full content of modified files, the
// honest omitted lists) and the DIFF (the shown per-file unified diffs in path
// order) — under JudgeArtifactBytesCap, and reports what it showed. Pure.
//
// INERT at grounding: returns zero values. The executor packet fills it.
func RenderChangeSlice(rc RevisionChange) (artifact, diff string, saw JudgeSaw) {
	return "", "", JudgeSaw{}
}

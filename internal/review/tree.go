package review

import (
	"context"
	"errors"
)

// A repo-backed revision IS the tree at its snapshot pin (Spec S13.1: repo-backed
// types pin a snapshot-commit sha; S13.2: the unified diff is computed host-side,
// `git diff` between revision pins). The step report a round produces is a
// COMPANION object on the revision row, never the deliverable itself. This file
// is the tree half of the S13.1/S13.2 data layer: the seam the platform store is
// reached through, the served shapes, the bounds, and the verbs.
//
// P3-SIT-1 GROUNDING — INERT TYPE SURFACE (CONVENTIONS §3, amendment-A carve-out):
// the types, the seam and the verb signatures exist so the committed acceptance
// tests compile and FAIL on behaviour; every verb below answers
// errTreeNotBuilt until the packet's implementation commit replaces it.

// TreeSource is the composition-root seam to the platform-owned project store
// (the BaseContentSource precedent, R25): review never imports internal/project;
// the shell wires *projectSeams here. Every method is READ-ONLY over pinned
// commits — nothing here stages, commits or moves a ref.
type TreeSource interface {
	// TreeBase resolves the deliverable's pre-task base commit — the recorded
	// refs/sinet/base/<pipeline> that revision 1 is presented against (Spec
	// S13.1/S13.5). ok=false means no base is recorded; that is an honest
	// absence, never a guess.
	TreeBase(ctx context.Context, deliverableID string) (sha string, ok bool, err error)
	// TreeChanges lists every file that differs between two pinned trees of the
	// deliverable's project store, computed host-side by git and ordered by
	// path. Both shas must resolve to commits the store holds; a sha it does
	// not hold is an error (the pin names an object the platform lost).
	TreeChanges(ctx context.Context, deliverableID, oldSHA, newSHA string) ([]ChangedFile, error)
	// TreeBlob reads one file at a pinned tree, BYTE-EXACT (no trim, no added
	// newline), returning at most limit bytes (limit <= 0 reads the whole
	// blob) and the blob's full size. ok=false means the path is not in that
	// tree.
	TreeBlob(ctx context.Context, deliverableID, sha, path string, limit int64) (data []byte, size int64, ok bool, err error)
}

// Change kinds of one inventory row (Spec S13.2 "everything arrives as a
// reviewable change"). Closed vocabulary; the shell adapter passes the project
// store's own words through, so the two vocabularies are one list with two
// readers.
const (
	KindAdded    = "added"
	KindModified = "modified"
	KindDeleted  = "deleted"
	KindRenamed  = "renamed"
)

// ChangedFile is one row of the change inventory between two pinned trees.
type ChangedFile struct {
	Path string `json:"path"`
	// OldPath is set for a rename only: the path the content had on the old
	// side (the row's Path is where it is now).
	OldPath string `json:"old_path,omitempty"`
	Kind    string `json:"kind"`
	OldSize int64  `json:"old_size"`
	NewSize int64  `json:"new_size"`
	// Binary is git's own verdict (the numstat `-` column): a binary file has an
	// inventory row and no diff text, and its bytes are never served inline.
	Binary bool `json:"binary"`
	// Additions/Deletions are git's numstat counts for text files (omitted for
	// binaries, where they do not exist).
	Additions int `json:"additions,omitempty"`
	Deletions int `json:"deletions,omitempty"`
}

// Change is the reviewable change of a repo-backed revision pair: the whole
// inventory (never truncated — the inventory is what makes a change
// reviewable), the two pins, and whether the old side is the pre-task base.
// It rides the deliverable DETAIL for the current revision's default pair
// (the S15.3 snapshot; file bodies are one read away) and every tree
// Comparison.
type Change struct {
	DeliverableID string `json:"deliverable_id"`
	OldN          int    `json:"old_n"`
	NewN          int    `json:"new_n"`
	// OldPin/NewPin are the snapshot commits compared (OldPin is the base
	// commit when OldIsBase; empty when the base is absent).
	OldPin    string `json:"old_pin,omitempty"`
	NewPin    string `json:"new_pin"`
	OldIsBase bool   `json:"old_is_base"`
	// Files is the whole inventory in path order — never nil on the wire (an
	// empty change serves `[]`).
	Files []ChangedFile `json:"files"`
	// AbsentReason states why no inventory could be computed (no tree source
	// composed; no pre-task base recorded), served as an answer rather than
	// failing the read (§38: absences are rendered, never errors that hide the
	// subject). Files is empty when it is set.
	AbsentReason string `json:"absent_reason,omitempty"`
}

// FileContent is one file of one revision as the code view reads it: the tree
// file at the revision's snapshot pin, or — when the path names no tree file —
// the revision's own content object of that name (the companion report;
// content-pinned lanes serve their objects the same way).
type FileContent struct {
	DeliverableID string `json:"deliverable_id"`
	RevisionN     int    `json:"revision_n"`
	// Pin is the revision's content pin: the snapshot sha (repo-backed) or the
	// content hash (content-pinned).
	Pin  string `json:"pin"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	// Binary is the git heuristic (a NUL byte within the first 8000 bytes);
	// a binary file's Content is empty — bytes are never served inline.
	Binary  bool   `json:"binary"`
	Content string `json:"content"`
	// Truncated + TruncationReason: the content stops at TreeFileBytesCap on a
	// line boundary and says so — a cap hit is never a silent cut.
	Truncated        bool   `json:"truncated"`
	TruncationReason string `json:"truncation_reason,omitempty"`
}

// Bounds on served tree payloads (S15.3 snapshot economy). Structural
// constants with their reasons, not ⚙ (S18 ratifies no such key; interim
// under the standing settings-tab directive):
//
//   - TreeDiffBytesCap bounds the unified text of a WHOLE change on the
//     compare read: react-diff-view parses and tokenizes it in the browser,
//     and per-file reads are one call away. Cut at a FILE boundary.
//   - TreeFileDiffBytesCap bounds ONE file's unified diff. Cut at a hunk
//     boundary when one fits, else at a line boundary.
//   - TreeFileBytesCap bounds ONE file's content on the files read: a code
//     view is read by a person. Cut at a line boundary.
//
// Every cap hit is served as Truncated=true with its reason.
const (
	TreeDiffBytesCap     = 512 << 10
	TreeFileDiffBytesCap = 256 << 10
	TreeFileBytesCap     = 1 << 20
)

var errTreeNotBuilt = errors.New("review: tree reads are P3-SIT-1's implementation (inert grounding surface)")

// Change computes the change inventory between two revisions of a repo-backed
// deliverable — oldN 0 is the pre-task base (Spec S13.1) — without reading any
// file body. A revision with no snapshot pin, or a nil TreeSource, answers a
// Change carrying AbsentReason.
func (s *Store) Change(ctx context.Context, deliverableID string, oldN, newN int) (Change, error) {
	return Change{}, errTreeNotBuilt
}

// CompareFile is Compare restricted to one changed file of a repo-backed
// revision pair: the Change holds that one inventory row and Unified its
// diff, bounded by TreeFileDiffBytesCap. A path the change does not cover is
// ErrNotFound.
func (s *Store) CompareFile(ctx context.Context, deliverableID string, oldN, newN int, path string) (Comparison, error) {
	return Comparison{}, errTreeNotBuilt
}

// RevisionFile serves one file of one revision (see FileContent): the tree
// file at the pin first, else the revision's content object of that name. An
// unknown path is ErrNotFound.
func (s *Store) RevisionFile(ctx context.Context, deliverableID string, n int, path string) (FileContent, error) {
	return FileContent{}, errTreeNotBuilt
}

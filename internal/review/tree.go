package review

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
)

// A repo-backed revision IS the tree at its snapshot pin (Spec S13.1: repo-backed
// types pin a snapshot-commit sha; S13.2: the unified diff is computed host-side,
// `git diff` between revision pins). The step report a round produces is a
// COMPANION object on the revision row, never the deliverable itself. This file
// is the tree half of the S13.1/S13.2 data layer: the seam the platform store is
// reached through, the served shapes, the bounds, and the verbs.
//
// THE LANE IS CHOSEN BY THE PIN, NEVER BY THE TYPE. A revision with a snapshot
// sha is repo-backed whatever word its deliverable row carries, which is what
// lets rows minted before the type was right serve their trees with no data
// migration (Spec S13.1 makes a minted revision immutable, and migration 0007
// makes the row's type immutable); a revision with no snapshot sha keeps the
// content-pinned behaviour byte for byte.

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

// binarySniffBytes is git's own window for deciding a blob is binary: a NUL in
// the first 8000 bytes. The same rule is applied here so the file read and the
// inventory row agree about which files have no text to show.
const binarySniffBytes = 8000

// treePinMissing is how a TreeSource reports that a PINNED COMMIT is gone from
// the project store. It is a structural contract rather than a shared sentinel
// because review cannot import the package that raises it (CONVENTIONS §23),
// and it has to be recognisable: a lost pin is the one tree failure that is a
// platform integrity fault rather than an absence, and S13.1 makes a minted
// revision's pin a promise the platform kept a copy.
type treePinMissing interface{ TreePinMissing() bool }

// pinDrift maps a seam error about a lost pin to ErrContentDrift, which the
// transport answers 500 content_drift — the objects-route precedent. The
// platform states it in its OWN sentence naming the pin; git's stderr is not a
// thing a requester is ever shown (§30/§38), and the pin is the one fact that
// makes the failure actionable. Any other seam error passes through unchanged.
func pinDrift(deliverableID, pin string, err error) error {
	var missing treePinMissing
	if errors.As(err, &missing) && missing.TreePinMissing() {
		return fmt.Errorf("%w: the project store no longer holds the saved files that %s pins at %s",
			ErrContentDrift, deliverableID, pin)
	}
	return err
}

// safeTreePath is the exec-boundary rule for a path that reaches git as an
// argument: non-empty, repo-relative, no parent-directory step, no control byte
// and no surrounding whitespace.
//
// This is the BACKSTOP, not the validation: the comment and file reads refuse a
// malformed path at the transport with a 400 a caller can act on. It exists
// because one ingress is not a caller at all — a finding's anchor is MODEL
// output (Spec S07.5 emits opaque "file:line" strings), and a NUL inside one
// reached fork/exec and failed the whole findings record. A path that does not
// pass here is simply not in the anchor map, so the S13.3 ladder degrades the
// anchor to file-level or orphan and the finding still lands (P-T12-2:
// delivery is never conditional on anchoring).
func safeTreePath(p string) bool {
	if p == "" || p != strings.TrimSpace(p) || strings.HasPrefix(p, "/") {
		return false
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// Change computes the change inventory between two revisions of a repo-backed
// deliverable — oldN 0 is the pre-task base (Spec S13.1) — without reading any
// file body. A revision with no snapshot pin, or a nil TreeSource, answers a
// Change carrying AbsentReason.
func (s *Store) Change(ctx context.Context, deliverableID string, oldN, newN int) (Change, error) {
	if newN < 1 || oldN < 0 || oldN == newN {
		return Change{}, fmt.Errorf("%w: comparing needs two different versions (old %d, new %d)", ErrBadInput, oldN, newN)
	}
	if _, err := s.Deliverable(ctx, deliverableID); err != nil {
		return Change{}, err
	}
	newRev, err := s.RevisionAt(ctx, deliverableID, newN)
	if err != nil {
		return Change{}, err
	}
	ch := Change{DeliverableID: deliverableID, OldN: oldN, NewN: newN, Files: []ChangedFile{}}
	if newRev.SnapshotSHA == "" {
		ch.AbsentReason = fmt.Sprintf("version %d is not stored as a snapshot of the project's files, so there is no file-by-file list for it", newN)
		return ch, nil
	}
	ch.NewPin = newRev.SnapshotSHA
	if s.Tree == nil {
		ch.AbsentReason = "the project's file store is not available in this process, so the files in this version cannot be listed"
		return ch, nil
	}
	if oldN == 0 {
		base, ok, err := s.Tree.TreeBase(ctx, deliverableID)
		if err != nil {
			return Change{}, pinDrift(deliverableID, ch.NewPin, err)
		}
		if !ok || base == "" {
			ch.AbsentReason = "no record of how the project stood before this task started, so there is nothing to compare this version against"
			return ch, nil
		}
		ch.OldPin, ch.OldIsBase = base, true
	} else {
		oldRev, err := s.RevisionAt(ctx, deliverableID, oldN)
		if err != nil {
			return Change{}, err
		}
		if oldRev.SnapshotSHA == "" {
			ch.AbsentReason = fmt.Sprintf("version %d is not stored as a snapshot of the project's files, so the two versions cannot be compared file by file", oldN)
			return ch, nil
		}
		ch.OldPin = oldRev.SnapshotSHA
	}
	files, err := s.Tree.TreeChanges(ctx, deliverableID, ch.OldPin, ch.NewPin)
	if err != nil {
		return Change{}, pinDrift(deliverableID, ch.NewPin, err)
	}
	if files != nil {
		ch.Files = files
	}
	return ch, nil
}

// treeCompare serves the tree lane of Compare: the inventory plus the per-file
// unified diffs of everything in it, in path order. onlyPath narrows both to one
// file (R4) — the same read at a second grain, never a second computation.
func (s *Store) treeCompare(ctx context.Context, d Deliverable, oldN, newN int, onlyPath string) (Comparison, error) {
	out := Comparison{DeliverableID: d.ID, Type: d.Type, OldN: oldN, NewN: newN, Surface: SurfaceLineDiff}
	ch, err := s.Change(ctx, d.ID, oldN, newN)
	if err != nil {
		return Comparison{}, err
	}
	if ch.AbsentReason != "" {
		// An absence is an ANSWER: the surface says why there is no file list
		// rather than failing the read (§38).
		out.Change = &ch
		return out, nil
	}
	if onlyPath != "" {
		row, ok := rowFor(ch.Files, onlyPath)
		if !ok {
			return Comparison{}, fmt.Errorf("%w: %s did not change between versions %d and %d", ErrNotFound, onlyPath, oldN, newN)
		}
		ch.Files = []ChangedFile{row}
		out.Change = &ch
		out.Unified, out.Truncated, out.TruncationReason, err = s.fileDiff(ctx, d.ID, ch, row)
		return out, err
	}
	out.Change = &ch
	out.Unified, out.Truncated, out.TruncationReason, err = s.changeDiff(ctx, d.ID, ch)
	return out, err
}

func rowFor(files []ChangedFile, path string) (ChangedFile, bool) {
	for _, f := range files {
		if f.Path == path {
			return f, true
		}
	}
	return ChangedFile{}, false
}

// changeDiff renders the whole change's unified text, bounded by
// TreeDiffBytesCap and cut at a FILE boundary.
//
// The cut is per-file rather than mid-text because the result has to stay a
// unified diff the S15.8 widget can parse: half a file's hunks is a body whose
// headers promise content that is not there. A file whose own bytes exceed the
// whole-change budget cannot be shown here honestly either — a diff computed
// from a partly-read blob would report changes that are an artefact of where
// the read stopped — so it and everything after it are omitted, and the reason
// says how much of the change the text holds.
func (s *Store) changeDiff(ctx context.Context, deliverableID string, ch Change) (string, bool, string, error) {
	var (
		sb       strings.Builder
		shown    int
		diffable int
	)
	for _, row := range ch.Files {
		if !row.Binary {
			diffable++
		}
	}
	for _, row := range ch.Files {
		if row.Binary {
			continue
		}
		oldText, oldWhole, err := s.side(ctx, deliverableID, ch.OldPin, oldPathOf(row), row.Kind != KindAdded)
		if err != nil {
			return "", false, "", err
		}
		newText, newWhole, err := s.side(ctx, deliverableID, ch.NewPin, row.Path, row.Kind != KindDeleted)
		if err != nil {
			return "", false, "", err
		}
		if !oldWhole || !newWhole {
			return sb.String(), true, partialChangeReason(shown, diffable), nil
		}
		u, err := gitDiff(row.Path, oldText, newText)
		if err != nil {
			return "", false, "", err
		}
		if sb.Len()+len(u) > TreeDiffBytesCap {
			return sb.String(), true, partialChangeReason(shown, diffable), nil
		}
		sb.WriteString(u)
		shown++
	}
	return sb.String(), false, "", nil
}

func partialChangeReason(shown, total int) string {
	if shown == 0 {
		return fmt.Sprintf("none of the %d changed files fit in one view — open each one on its own to read its changes", total)
	}
	return fmt.Sprintf("this change is too large to show at once: the text below covers the first %d of %d changed files — open the rest one at a time",
		shown, total)
}

// fileDiff renders ONE file's unified diff.
//
// THE DIFF IS CUT, NEVER THE INPUT. A diff computed from two truncated prefixes
// is an artefact of where the read stopped, not a description of the change: two
// 600 KB files differing only at the end read to a common prefix diff to
// NOTHING, and a body saying "no changes here" about a file that changed is
// worse than saying nothing at all. So each side is read whole up to
// TreeFileBytesCap — the largest cap the package serves under, so no blob is
// ever read past it — and a file bigger than that gets an inventory row, a
// truncation reason naming its size, and no diff text. Everything that fits is
// diffed in FULL and the resulting text is cut at the last hunk boundary under
// TreeFileDiffBytesCap, so every hunk served is a whole hunk and the reason
// names how big the real diff was.
func (s *Store) fileDiff(ctx context.Context, deliverableID string, ch Change, row ChangedFile) (string, bool, string, error) {
	if row.Binary {
		return "", false, "", nil
	}
	oldText, oldWhole, err := s.side(ctx, deliverableID, ch.OldPin, oldPathOf(row), row.Kind != KindAdded)
	if err != nil {
		return "", false, "", err
	}
	newText, newWhole, err := s.side(ctx, deliverableID, ch.NewPin, row.Path, row.Kind != KindDeleted)
	if err != nil {
		return "", false, "", err
	}
	if !oldWhole || !newWhole {
		// Both sides present and one of them unreadable in full means any diff
		// would be that artefact, so none is served. A one-sided change (a file
		// added or deleted whole) has nothing to pair against, so its first part
		// is a truthful prefix of the addition or the removal and IS served —
		// the acceptance battery's over-cap added file is exactly that shape.
		oneSided := row.Kind == KindAdded || row.Kind == KindDeleted
		size := byteSize(largerOf(row.OldSize, row.NewSize))
		if !oneSided {
			return "", true, fmt.Sprintf("%s is %s, which is too large to compare here — open the file to read it", row.Path, size), nil
		}
		text, err := gitDiff(row.Path, trimToLine(oldText), trimToLine(newText))
		if err != nil {
			return "", false, "", err
		}
		return cutDiff(text, TreeFileDiffBytesCap), true,
			fmt.Sprintf("%s is %s, which is more than can be shown in one view — the text below covers only the start of it", row.Path, size), nil
	}
	text, err := gitDiff(row.Path, oldText, newText)
	if err != nil {
		return "", false, "", err
	}
	full := len(text)
	if full <= TreeFileDiffBytesCap {
		return text, false, "", nil
	}
	text = cutDiff(text, TreeFileDiffBytesCap)
	return text, true, fmt.Sprintf("the changes to %s run to %s; the text below stops after %s, at the end of a hunk",
		row.Path, byteSize(int64(full)), byteSize(int64(len(text)))), nil
}

// side reads one end of a file's diff. present=false is the added/deleted end
// (the empty side); a path the tree does not hold reads empty too. The returned
// flag is false when the blob is LARGER than TreeFileBytesCap, which means the
// text is a prefix and the caller must decide what can honestly be said about
// it rather than diffing it.
func (s *Store) side(ctx context.Context, deliverableID, pin, path string, present bool) (string, bool, error) {
	if !present || pin == "" || path == "" {
		return "", true, nil
	}
	data, size, ok, err := s.Tree.TreeBlob(ctx, deliverableID, pin, path, TreeFileBytesCap)
	if err != nil {
		return "", false, pinDrift(deliverableID, pin, err)
	}
	if !ok {
		return "", true, nil
	}
	return string(data), size <= TreeFileBytesCap, nil
}

func oldPathOf(row ChangedFile) string {
	if row.Kind == KindRenamed && row.OldPath != "" {
		return row.OldPath
	}
	return row.Path
}

func largerOf(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// CompareFile is Compare restricted to one changed file of a repo-backed
// revision pair: the Change holds that one inventory row and Unified its
// diff, bounded by TreeFileDiffBytesCap. A path the change does not cover is
// ErrNotFound.
func (s *Store) CompareFile(ctx context.Context, deliverableID string, oldN, newN int, path string) (Comparison, error) {
	if path == "" {
		return Comparison{}, fmt.Errorf("%w: narrowing a comparison needs a file path", ErrBadInput)
	}
	d, err := s.Deliverable(ctx, deliverableID)
	if err != nil {
		return Comparison{}, err
	}
	newRev, err := s.RevisionAt(ctx, deliverableID, newN)
	if err != nil {
		return Comparison{}, err
	}
	// A content-pinned version has no file tree to narrow, and that IS the
	// caller's mistake to fix: a bad request, not an absence.
	if newRev.SnapshotSHA == "" {
		return Comparison{}, fmt.Errorf("%w: this version is not stored as a snapshot of the project's files, so a comparison cannot be narrowed to one file", ErrBadInput)
	}
	// A missing tree source is the PLATFORM's absence, not the caller's, and
	// treeCompare already states it the way the detail's change does. Answering
	// 400 here told a requester to fix a request that was correct.
	return s.treeCompare(ctx, d, oldN, newN, path)
}

// RevisionFile serves one file of one revision (see FileContent): the tree
// file at the pin first, else the revision's content object of that name. An
// unknown path is ErrNotFound.
func (s *Store) RevisionFile(ctx context.Context, deliverableID string, n int, path string) (FileContent, error) {
	if path == "" {
		return FileContent{}, fmt.Errorf("%w: reading a file needs a path", ErrBadInput)
	}
	rev, err := s.RevisionAt(ctx, deliverableID, n)
	if err != nil {
		return FileContent{}, err
	}
	out := FileContent{DeliverableID: deliverableID, RevisionN: n, Path: path, Pin: rev.ContentSHA256}
	if rev.SnapshotSHA != "" {
		out.Pin = rev.SnapshotSHA
	}
	if rev.SnapshotSHA != "" && s.Tree != nil {
		if !safeTreePath(path) {
			return FileContent{}, fmt.Errorf("%w: %q is not a file path inside the project", ErrBadInput, path)
		}
		data, size, ok, err := s.Tree.TreeBlob(ctx, deliverableID, rev.SnapshotSHA, path, TreeFileBytesCap)
		if err != nil {
			// A lost pin is drift, and it must NOT fall through to the round
			// report below: serving the companion object for a request about a
			// code file would answer a different question than the one asked.
			return FileContent{}, pinDrift(deliverableID, rev.SnapshotSHA, err)
		}
		if ok {
			out.Size = size
			if isBinaryContent(data) {
				// A binary's bytes are never served inline; the objects route
				// is the one bytes channel (Spec S13.2, G2 Def.14).
				out.Binary = true
				return out, nil
			}
			out.Content = string(data)
			if size > TreeFileBytesCap {
				// The read already stopped at the cap; the cut here is to the
				// last whole LINE of what came back, not to the cap again.
				out.Content = trimToLine(out.Content)
				out.Truncated = true
				out.TruncationReason = fmt.Sprintf("%s is %s; the text below stops after %s, at the end of a line",
					path, byteSize(size), byteSize(int64(len(out.Content))))
			}
			return out, nil
		}
	}
	// No tree file of that name: the revision's own content objects — the
	// companion report on a repo-backed revision, and the whole deliverable on
	// a content-pinned one. Read hash-verified through the object dir.
	files, err := s.RevisionFiles(ctx, deliverableID, n)
	if err != nil {
		return FileContent{}, err
	}
	content, ok := files[path]
	if !ok {
		return FileContent{}, fmt.Errorf("%w: %s holds no file %q at version %d", ErrNotFound, deliverableID, path, n)
	}
	out.Size = int64(len(content))
	if isBinaryContent([]byte(content)) {
		out.Binary = true
		return out, nil
	}
	out.Content = content
	if out.Size > TreeFileBytesCap {
		out.Content = cutAtLine(content, TreeFileBytesCap)
		out.Truncated = true
		out.TruncationReason = fmt.Sprintf("%s is %s; the text below stops after %s, at the end of a line",
			path, byteSize(out.Size), byteSize(int64(len(out.Content))))
	}
	return out, nil
}

// anchorFiles is the file map the S13.3 anchor model reads for one revision.
//
// For a repo-backed revision the anchored paths are TREE paths (Spec S13.3:
// file_path is the repo-relative path, line_no the line of that file at the
// revision), so they are read from the tree at the pin — but only the paths the
// anchors actually name, so the cost is the number of anchors rather than the
// size of the tree. The revision's own content objects stay in the map beneath
// them, which is what keeps a comment on the companion report anchoring; the
// tree wins a collision, because on a repo-backed revision the tree IS the
// deliverable. A content-pinned revision's map is exactly RevisionFiles, as
// before.
func (s *Store) anchorFiles(ctx context.Context, deliverableID string, n int, paths []string) (map[string]string, error) {
	files, err := s.RevisionFiles(ctx, deliverableID, n)
	if err != nil {
		return nil, err
	}
	if s.Tree == nil || len(paths) == 0 {
		return files, nil
	}
	rev, err := s.RevisionAt(ctx, deliverableID, n)
	if err != nil {
		return nil, err
	}
	if rev.SnapshotSHA == "" {
		return files, nil
	}
	seen := map[string]bool{}
	for _, p := range paths {
		// safeTreePath is the exec boundary: a model-emitted anchor carrying a
		// NUL reached fork/exec and failed the whole findings record. A path
		// that cannot be one is simply absent from the map, and the ladder
		// degrades the anchor rather than losing the point.
		if seen[p] || !safeTreePath(p) {
			continue
		}
		seen[p] = true
		data, size, ok, err := s.Tree.TreeBlob(ctx, deliverableID, rev.SnapshotSHA, p, TreeFileBytesCap)
		if err != nil {
			return nil, pinDrift(deliverableID, rev.SnapshotSHA, err)
		}
		// A path that is absent, binary or past the read cap simply is not in
		// the map, and the ladder degrades it the way it degrades any anchor
		// whose file it cannot see — to a file-level placement or an orphan,
		// quote kept (Spec S13.3 steps 4-5). A partial body is never offered,
		// because anchoring against one would place comments by line numbers
		// that only exist in the prefix.
		if !ok || size > TreeFileBytesCap || isBinaryContent(data) {
			continue
		}
		files[p] = string(data)
	}
	return files, nil
}

// anchoredPaths is the set of file paths a batch of comments names — the bound
// on what anchorFiles reads.
func anchoredPaths(comments []Comment) []string {
	var out []string
	for _, c := range comments {
		if c.Anchor.FilePath != "" {
			out = append(out, c.Anchor.FilePath)
		}
	}
	return out
}

// findingAnchorPaths is the same bound for findings, whose anchors arrive as the
// opaque "path:line" strings S07.5 emits. It reads the SAME shape
// ParseFindingAnchor does — the path before a trailing line number — without
// deciding anything: a candidate that names no file of the revision simply
// finds nothing in the map, and the finding records file-level as it always
// did.
func findingAnchorPaths(findings []FindingInput) []string {
	var out []string
	for _, f := range findings {
		if path, ok := anchorPathCandidate(f.RawAnchor); ok {
			out = append(out, path)
		}
	}
	return out
}

func anchorPathCandidate(raw string) (string, bool) {
	i := strings.LastIndexByte(raw, ':')
	if i <= 0 || i == len(raw)-1 {
		return "", false
	}
	for _, c := range raw[i+1:] {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	return raw[:i], true
}

// isBinaryContent applies git's own heuristic: a NUL byte inside the first
// 8000 bytes means there is no text to show.
func isBinaryContent(data []byte) bool {
	if len(data) > binarySniffBytes {
		data = data[:binarySniffBytes]
	}
	return bytes.IndexByte(data, 0) >= 0
}

// cutAtLine truncates to at most limit bytes, ending at a line boundary when
// there is one — so a served prefix is a set of whole lines and its last line
// is not half a statement.
func cutAtLine(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	if i := strings.LastIndexByte(s[:limit], '\n'); i >= 0 {
		return s[:i+1]
	}
	return s[:limit]
}

// trimToLine drops a trailing partial line from a prefix that was already cut
// by a bounded READ. It is not cutAtLine's job: that one asks "what fits under
// this many bytes", and the answer here is "all of this except the half line
// the reader stopped in the middle of".
func trimToLine(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[:i+1]
	}
	return s
}

// cutDiff truncates a unified diff to at most limit bytes, preferring the last
// HUNK boundary that fits so the served text is whole hunks. The first hunk is
// never cut away — a diff body with headers and no hunk at all says less than a
// partial hunk does — so a single oversized hunk falls back to a line boundary.
func cutDiff(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	first := hunkStart(text, 0)
	if first >= 0 && first < limit {
		best := -1
		for at := hunkStart(text, first+1); at >= 0 && at <= limit; at = hunkStart(text, at+1) {
			best = at
		}
		if best > 0 {
			return text[:best]
		}
	}
	return cutAtLine(text, limit)
}

// hunkStart is the index of the first hunk header at or after from — a line
// beginning "@@ ".
func hunkStart(text string, from int) int {
	if from < 0 || from > len(text) {
		return -1
	}
	for at := from; at < len(text); {
		i := strings.Index(text[at:], "@@ ")
		if i < 0 {
			return -1
		}
		abs := at + i
		if abs == 0 || text[abs-1] == '\n' {
			return abs
		}
		at = abs + 1
	}
	return -1
}

// byteSize renders a size the way a person reads one. Requester-facing prose
// carries no byte counts in the raw (§38).
func byteSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d bytes", n)
	}
}

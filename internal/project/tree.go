package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Read-only tree verbs over pinned commits of a project store (Spec S13.1: a
// repo-backed revision pins a snapshot commit; S13.2: the reviewable change is
// computed host-side by git between revision pins). Consumed by review through
// the composition root's TreeSource adapter (internal/shell), never by import.
//
// EVERY VERB HERE IS A READ, and that is a property rather than a habit
// (CONVENTIONS §59 "reading a fact must not create it"): the reviewable change
// is derived from commits the execute leg already made, so none of these calls
// stages, commits, checks out, moves a ref or touches a worktree. The whole
// file runs git PLUMBING over the bare store — the no-stash/amend/force/push
// source scan (conformance_test.go) still holds over it.

// TreeChange is one file that differs between two pinned trees, as git reports
// it. Kind is one of "added" | "modified" | "deleted" | "renamed" — the same
// words review serves, so the adapter passes them through unchanged.
type TreeChange struct {
	Path    string
	OldPath string // renames only
	Kind    string
	OldSize int64
	NewSize int64
	Binary  bool // git's numstat `-` verdict
	// Additions/Deletions are git's numstat counts (0 for binaries).
	Additions int
	Deletions int
}

// The change kinds, as review names them. The vocabulary lives in review (it is
// what the wire carries) and is repeated here as the value this package emits:
// the adapter passes these words through verbatim, so a divergence would be one
// list with two spellings rather than one with two readers. The shell's
// composition test binds the two sides together.
const (
	kindAdded    = "added"
	kindModified = "modified"
	kindDeleted  = "deleted"
	kindRenamed  = "renamed"
)

// ErrPinMissing reports that a PINNED COMMIT is not in the store — the platform
// recorded a revision against an object the store no longer holds.
//
// It is deliberately distinct from "that path is not in this tree", which is an
// ordinary absence (ok=false): one says a copy of the work is gone and is a
// platform integrity failure, the other says a file was never there and is an
// ordinary answer. Collapsing the two is how a lost pin came back as a 404 over
// the round report instead of the drift error it is.
var ErrPinMissing = errors.New("project: the store holds no such commit")

// pinMissing carries which pin was lost, and answers TreePinMissing so consumers
// that cannot import this package can still recognise the condition.
// internal/review is walled off from internal/project (CONVENTIONS §23), so a
// shared sentinel is unreachable between them and a METHOD is the seam — the
// net.Error.Timeout() shape.
type pinMissing struct {
	projectID string
	sha       string
}

func (e *pinMissing) Error() string {
	return fmt.Sprintf("project: %s holds no commit %s", e.projectID, e.sha)
}

func (e *pinMissing) Is(target error) bool { return target == ErrPinMissing }

// TreePinMissing marks this as a lost pin for a consumer outside this package's
// import reach.
func (e *pinMissing) TreePinMissing() bool { return true }

// BaseSHA reads a pipeline's recorded attempt-1 base commit
// (refs/sinet/base/<pipeline>, Spec S13.5) — revision 1's old side (Spec
// S13.1). "" when none is recorded; never a guess.
func (s *Store) BaseSHA(ctx context.Context, projectID, pipelineID string) (string, error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return "", err
	}
	return s.refSHA(ctx, e.StorePath, baseRef(pipelineID, 1))
}

// TreeChanges lists every path that differs between two commits of the
// project store, in path order, with kind, sizes, git's binary verdict and
// numstat counts — computed by git plumbing over the store, never by reading
// file bodies into the platform. Both shas must be commits the store holds.
//
// Three plumbing reads compose one inventory, because no single git format
// carries all three facts: `--raw` names the status (and pairs a rename's two
// paths), `--numstat` carries the line counts and git's own binary verdict (the
// `-` column), and `ls-tree -l` carries the blob sizes. The two diff-tree calls
// run with identical flags over identical commits, so their rename pairing is
// the same pairing; the ls-tree reads are narrowed to the changed paths, so the
// cost is the size of the CHANGE rather than the size of the repository.
func (s *Store) TreeChanges(ctx context.Context, projectID, oldSHA, newSHA string) ([]TreeChange, error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, sha := range []string{oldSHA, newSHA} {
		if sha == "" {
			return nil, fmt.Errorf("project: tree changes need two commits (got %q → %q)", oldSHA, newSHA)
		}
		if !s.objectExists(ctx, e.StorePath, sha) {
			return nil, &pinMissing{projectID: projectID, sha: sha}
		}
	}
	rows, err := s.rawChanges(ctx, e.StorePath, oldSHA, newSHA)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if err := s.applyNumstat(ctx, e.StorePath, oldSHA, newSHA, rows); err != nil {
		return nil, err
	}
	if err := s.applySizes(ctx, e.StorePath, oldSHA, newSHA, rows); err != nil {
		return nil, err
	}
	out := make([]TreeChange, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// rawChanges parses `diff-tree --raw`: one record per changed path, carrying the
// status letter and — for a rename — both paths, old first.
func (s *Store) rawChanges(ctx context.Context, store, oldSHA, newSHA string) (map[string]*TreeChange, error) {
	out, err := s.plumb(ctx, store, "diff-tree", "-r", "-M", "-z", "--raw", oldSHA, newSHA)
	if err != nil {
		return nil, err
	}
	rows := map[string]*TreeChange{}
	fields := nulFields(out)
	for i := 0; i < len(fields); {
		head := fields[i]
		i++
		if !strings.HasPrefix(head, ":") {
			return nil, fmt.Errorf("project: diff-tree --raw record %q does not start with ':'", head)
		}
		// :<srcmode> <dstmode> <srcsha> <dstsha> <status>
		cols := strings.Fields(head[1:])
		if len(cols) < 5 {
			return nil, fmt.Errorf("project: diff-tree --raw header %q is malformed", head)
		}
		status := cols[4]
		kind, err := changeKind(status)
		if err != nil {
			return nil, err
		}
		if i >= len(fields) {
			return nil, fmt.Errorf("project: diff-tree --raw record %q names no path", head)
		}
		path := fields[i]
		i++
		row := &TreeChange{Path: path, Kind: kind}
		if kind == kindRenamed {
			if i >= len(fields) {
				return nil, fmt.Errorf("project: diff-tree --raw rename %q names one path", path)
			}
			row.OldPath, row.Path = path, fields[i]
			i++
		}
		rows[row.Path] = row
	}
	return rows, nil
}

// applyNumstat fills the line counts and git's binary verdict. A binary file's
// numstat columns are both `-`: git says the diff is not line-shaped, which is
// exactly the fact the review surface needs to serve an inventory row and no
// diff text for it.
func (s *Store) applyNumstat(ctx context.Context, store, oldSHA, newSHA string, rows map[string]*TreeChange) error {
	out, err := s.plumb(ctx, store, "diff-tree", "-r", "-M", "-z", "--numstat", oldSHA, newSHA)
	if err != nil {
		return err
	}
	fields := nulFields(out)
	for i := 0; i < len(fields); {
		cols := strings.SplitN(fields[i], "\t", 3)
		i++
		if len(cols) < 3 {
			return fmt.Errorf("project: diff-tree --numstat record %q is malformed", cols)
		}
		add, del, path := cols[0], cols[1], cols[2]
		if path == "" {
			// A rename: the record's path column is empty and the two paths
			// follow as their own NUL-terminated fields, old first.
			if i+1 >= len(fields) {
				return fmt.Errorf("project: diff-tree --numstat rename record names fewer than two paths")
			}
			path = fields[i+1]
			i += 2
		}
		row, ok := rows[path]
		if !ok {
			return fmt.Errorf("project: diff-tree --numstat names %q, which --raw did not", path)
		}
		if add == "-" || del == "-" {
			row.Binary = true
			continue
		}
		if row.Additions, err = strconv.Atoi(add); err != nil {
			return fmt.Errorf("project: numstat additions %q for %s: %w", add, path, err)
		}
		if row.Deletions, err = strconv.Atoi(del); err != nil {
			return fmt.Errorf("project: numstat deletions %q for %s: %w", del, path, err)
		}
	}
	return nil
}

// applySizes fills each side's blob size from the two trees, asking for the
// changed paths ALONE — a deleted path is looked up on the old side, an added
// one on the new, a rename on both under its own name per side. A path absent
// from a side simply has no row there, and its size stays 0.
func (s *Store) applySizes(ctx context.Context, store, oldSHA, newSHA string, rows map[string]*TreeChange) error {
	var oldPaths, newPaths []string
	for _, r := range rows {
		switch r.Kind {
		case kindAdded:
			newPaths = append(newPaths, r.Path)
		case kindDeleted:
			oldPaths = append(oldPaths, r.Path)
		case kindRenamed:
			oldPaths = append(oldPaths, r.OldPath)
			newPaths = append(newPaths, r.Path)
		default:
			oldPaths = append(oldPaths, r.Path)
			newPaths = append(newPaths, r.Path)
		}
	}
	oldSizes, err := s.blobSizes(ctx, store, oldSHA, oldPaths)
	if err != nil {
		return err
	}
	newSizes, err := s.blobSizes(ctx, store, newSHA, newPaths)
	if err != nil {
		return err
	}
	for _, r := range rows {
		from := r.Path
		if r.Kind == kindRenamed {
			from = r.OldPath
		}
		r.OldSize, r.NewSize = oldSizes[from], newSizes[r.Path]
	}
	return nil
}

// blobSizes reads the sizes of the named paths at one commit. An empty path set
// asks nothing: `ls-tree -- ` with no pathspec would list the WHOLE tree, which
// is the one answer this must never give.
func (s *Store) blobSizes(ctx context.Context, store, sha string, paths []string) (map[string]int64, error) {
	sizes := map[string]int64{}
	if len(paths) == 0 {
		return sizes, nil
	}
	// --literal-pathspecs is a GIT-LEVEL option (ls-tree rejects it as its own),
	// and it is load-bearing rather than defensive: without it a file literally
	// named ":x" is read as pathspec MAGIC, matches nothing, and that row comes
	// back with no size at all.
	args := append([]string{"--literal-pathspecs", "ls-tree", "-r", "-l", "-z", sha, "--"}, paths...)
	out, err := s.plumb(ctx, store, args...)
	if err != nil {
		return nil, err
	}
	for _, rec := range nulFields(out) {
		// <mode> <type> <sha> <size>\t<path>
		tab := strings.IndexByte(rec, '\t')
		if tab < 0 {
			return nil, fmt.Errorf("project: ls-tree record %q carries no path", rec)
		}
		cols := strings.Fields(rec[:tab])
		if len(cols) < 4 {
			return nil, fmt.Errorf("project: ls-tree header %q is malformed", rec[:tab])
		}
		if cols[1] != "blob" {
			continue // a submodule/tree entry has no blob size to serve
		}
		n, err := strconv.ParseInt(cols[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("project: ls-tree size %q: %w", cols[3], err)
		}
		sizes[rec[tab+1:]] = n
	}
	return sizes, nil
}

// TreeBlob reads one path's blob at a commit, byte-exact, returning at most
// limit bytes (limit <= 0: whole) and the blob's full size. ok=false when the
// path is not in that tree.
//
// The size is read FIRST and the bytes are then streamed under the limit, so a
// caller serving under a cap never pulls a whole large file into the platform
// to throw most of it away — and `Size` still tells the truth about the file,
// which is what makes a truncated answer honest rather than merely short.
func (s *Store) TreeBlob(ctx context.Context, projectID, treeish, path string, limit int64) (data []byte, size int64, ok bool, err error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return nil, 0, false, err
	}
	spec := treeish + ":" + cleanTreePath(path)
	// The TYPE first, for two reasons that both used to reach the wire. A
	// missing path and a missing COMMIT both exit non-zero here, and only the
	// second is a lost pin — so a failure asks the store whether it still holds
	// the commit rather than guessing. And a DIRECTORY resolves perfectly well
	// as an object: `cat-file -s` answers a tree's size and `cat-file blob` then
	// fails on it, which is how a request for a folder came back as git's own
	// "bad file" text at 500 instead of "that is not a file".
	code, kind, stderr, err := s.gitRaw(ctx, e.StorePath, identity{}, "cat-file", "-t", spec)
	if err != nil {
		return nil, 0, false, err
	}
	if code != 0 {
		if !s.objectExists(ctx, e.StorePath, treeish) {
			return nil, 0, false, &pinMissing{projectID: projectID, sha: treeish}
		}
		// The commit is here and the path is not: an ordinary absence, and the
		// caller's ladder decides what to do about it.
		return nil, 0, false, nil
	}
	if strings.TrimSpace(kind) != "blob" {
		// A tree or a submodule is not a file. An absence, so the read falls to
		// its own next step and an anchor degrades down the S13.3 ladder.
		return nil, 0, false, nil
	}
	code, out, stderr, err := s.gitRaw(ctx, e.StorePath, identity{}, "cat-file", "-s", spec)
	if err != nil {
		return nil, 0, false, err
	}
	if code != 0 {
		return nil, 0, false, fmt.Errorf("project: cat-file -s %s (exit %d): %s", spec, code, strings.TrimSpace(stderr))
	}
	size, err = strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return nil, 0, false, fmt.Errorf("project: cat-file -s %s: %w", spec, err)
	}
	want := size
	if limit > 0 && limit < want {
		want = limit
	}
	data, err = s.blobPrefix(ctx, e.StorePath, spec, want)
	if err != nil {
		return nil, 0, false, err
	}
	return data, size, true, nil
}

// blobPrefix streams the first want bytes of a blob. git is stopped by closing
// the pipe once the prefix is in hand; a short read is the only failure this
// can report, because the byte count asked for is derived from the object's own
// recorded size.
func (s *Store) blobPrefix(ctx context.Context, store, spec string, want int64) ([]byte, error) {
	cmd := exec.CommandContext(ctx, gitBin, "-c", "core.quotepath=false", "-c", "gc.auto=0", "cat-file", "blob", spec)
	cmd.Dir = store
	cmd.Env = baseEnv(identity{})
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("project: cat-file blob %s: %w", spec, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("project: cat-file blob %s: %w", spec, err)
	}
	data := make([]byte, want)
	n, readErr := io.ReadFull(pipe, data)
	pipe.Close()
	waitErr := cmd.Wait()
	if int64(n) != want {
		return nil, fmt.Errorf("project: cat-file blob %s read %d of %d bytes: %w (%s)",
			spec, n, want, firstNonNil(readErr, waitErr), strings.TrimSpace(stderr.String()))
	}
	// The prefix is complete, so a non-zero exit here is git being stopped on a
	// closed pipe — the deliberate end of a bounded read, not a failure.
	return data, nil
}

func firstNonNil(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// plumb runs a read-only plumbing command whose output is NUL-separated, and
// returns it UNTRIMMED: the shared git() helper trims, which would eat a
// trailing empty path and make a record boundary ambiguous.
func (s *Store) plumb(ctx context.Context, store string, args ...string) (string, error) {
	code, out, stderr, err := s.gitRaw(ctx, store, identity{}, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("project: git %s (exit %d): %s", subcommandOf(args), code, strings.TrimSpace(stderr))
	}
	return out, nil
}

// subcommandOf names the git subcommand for an error message, skipping any
// git-level options that precede it.
func subcommandOf(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return "plumbing"
}

// cleanTreePath drops a leading "./" so a path used as an object spec names the
// same entry the tree lists it under.
func cleanTreePath(p string) string {
	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	return p
}

// nulFields splits NUL-separated plumbing output, dropping the trailing empty
// field every such stream ends with.
func nulFields(out string) []string {
	parts := strings.Split(out, "\x00")
	fields := parts[:0]
	for _, p := range parts {
		if p != "" {
			fields = append(fields, p)
		}
	}
	return fields
}

// changeKind maps git's status letter to the served word. A type change (T —
// a file that became a symlink, or the reverse) is a MODIFICATION of what that
// path holds, which is what a reviewer is being shown. Anything else is loud:
// an unrecognised status would otherwise reach the wire as an empty kind.
func changeKind(status string) (string, error) {
	if status == "" {
		return "", fmt.Errorf("project: diff-tree record carries no status")
	}
	switch status[0] {
	case 'A':
		return kindAdded, nil
	case 'M', 'T':
		return kindModified, nil
	case 'D':
		return kindDeleted, nil
	case 'R':
		return kindRenamed, nil
	}
	return "", fmt.Errorf("project: unexpected diff-tree status %q", status)
}

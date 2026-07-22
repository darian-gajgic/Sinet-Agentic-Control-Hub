package project

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// Git topology (Spec S13.5): branch-per-pipeline run-branch worktrees of the
// registered project store, platform-owned tree-level snapshot commits, fresh
// branch per attempt, utility checkouts, workspace GC, and the minted-revision
// platform refs. The overlayfs half of the S02.10 workspace ("how it mounts
// inside the per-run sandbox") is S11.3's and rides the sandbox host batch;
// this package owns the workspace lifecycle and GC policy (S11.3 opening,
// verbatim split) and hands the sandbox a PATH.

const snapshotMessage = "sinet: platform snapshot (Spec S13.5)"

// Workspace is a pipeline's run-branch worktree of the project store.
type Workspace struct {
	ProjectID  string `json:"project_id"`
	PipelineID string `json:"pipeline_id"`
	Attempt    int    `json:"attempt"`
	Path       string `json:"path"`
	Branch     string `json:"branch"`
	// Base is the branch base — project HEAD (default branch) at creation,
	// recorded durably as refs/sinet/base/<pipeline> (revision 1's old side,
	// Spec S13.1). The base content is resolvable from it (Compare old-side 0).
	Base string `json:"base"`
}

// runBranch names a pipeline's run branch (Spec S13.5: one long-lived run
// branch per pipeline; a fresh branch per abandoned/base-moved attempt).
func runBranch(pipelineID string, attempt int) string {
	if attempt <= 1 {
		return "sinet/run/" + pipelineID
	}
	// A sibling leaf (not a child path) so a later attempt never collides with
	// the attempt-1 ref under refs/heads/ (the git dir/file ref conflict).
	return fmt.Sprintf("sinet/run/%s-a%d", pipelineID, attempt)
}

// baseRef names an attempt's durably-recorded base — the default-branch HEAD
// at THAT attempt's creation (Spec S13.5: a fresh attempt branches from the
// current HEAD, base recorded per-attempt). Attempt 1's base is revision 1's
// old side (Spec S13.1); a sibling-leaf name keeps later attempts D/F-safe.
func baseRef(pipelineID string, attempt int) string {
	if attempt <= 1 {
		return "refs/sinet/base/" + pipelineID
	}
	return fmt.Sprintf("refs/sinet/base/%s-a%d", pipelineID, attempt)
}

func (s *Store) worktreePath(projectID, pipelineID string, attempt int) string {
	name := pathComponent(pipelineID)
	if attempt > 1 {
		name = fmt.Sprintf("%s-a%d", name, attempt)
	}
	return filepath.Join(s.root, "worktrees", pathComponent(projectID), name)
}

// EnsureWorkspace attaches a pipeline to its run-branch worktree of the
// project store, creating the worktree (and, first time, the run branch off
// the project's default-branch HEAD) on demand (Spec S13.5/S02.10). Idempotent:
// a second call returns the same worktree, so commits accumulate across stages
// and review rounds on the SAME branch. Only ACTIVE entries have workspaces.
func (s *Store) EnsureWorkspace(ctx context.Context, projectID, pipelineID string) (Workspace, error) {
	return s.ensureWorkspaceAttempt(ctx, projectID, pipelineID, 1)
}

func (s *Store) ensureWorkspaceAttempt(ctx context.Context, projectID, pipelineID string, attempt int) (Workspace, error) {
	if pipelineID == "" {
		return Workspace{}, fmt.Errorf("%w: workspace needs a pipeline id", ErrBadInput)
	}
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return Workspace{}, err
	}
	if !e.Active() {
		return Workspace{}, fmt.Errorf("%w: %q", ErrNotActive, projectID)
	}
	branch := runBranch(pipelineID, attempt)
	path := s.worktreePath(projectID, pipelineID, attempt)

	// Record THIS attempt's base durably the first time: the default-branch
	// HEAD now (Spec S13.5 — a fresh attempt branches from the CURRENT HEAD;
	// attempt 1's base is revision 1's old side, Spec S13.1). Once recorded the
	// base is immutable so a re-attach reuses the exact branch point.
	bref := baseRef(pipelineID, attempt)
	base, err := s.refSHA(ctx, e.StorePath, bref)
	if err != nil {
		return Workspace{}, err
	}
	if base == "" {
		base, err = s.refSHA(ctx, e.StorePath, "refs/heads/"+e.DefaultBranch)
		if err != nil {
			return Workspace{}, err
		}
		if base == "" {
			return Workspace{}, fmt.Errorf("%w: project %q default branch %q has no HEAD", ErrBadInput, projectID, e.DefaultBranch)
		}
		if _, err := s.git(ctx, e.StorePath, identity{}, "update-ref", bref, base); err != nil {
			return Workspace{}, err
		}
	}

	ws := Workspace{ProjectID: projectID, PipelineID: pipelineID, Attempt: attempt, Path: path, Branch: branch, Base: base}
	if s.worktreeRegistered(ctx, e.StorePath, path) {
		return ws, nil
	}
	// Prune stale admin entries opportunistically (Spec S13.5), then create.
	_, _ = s.git(ctx, e.StorePath, identity{}, "worktree", "prune")
	branchExists, err := s.refSHA(ctx, e.StorePath, "refs/heads/"+branch)
	if err != nil {
		return Workspace{}, err
	}
	if branchExists != "" {
		if _, err := s.git(ctx, e.StorePath, identity{}, "worktree", "add", path, branch); err != nil {
			return Workspace{}, err
		}
	} else {
		if _, err := s.git(ctx, e.StorePath, identity{}, "worktree", "add", "-b", branch, path, base); err != nil {
			return Workspace{}, err
		}
	}
	return ws, nil
}

// FreshAttempt starts a NEW run branch and worktree for a pipeline (Spec
// S13.5: the attempt model — when the base moved or an attempt is abandoned, a
// NEW branch, never rewriting the old one; the old attempt's branch STAYS
// until revision-retention GC clears it). The new attempt branches from the
// CURRENT default-branch HEAD, recorded as this attempt's own base ref.
func (s *Store) FreshAttempt(ctx context.Context, projectID, pipelineID string, attempt int) (Workspace, error) {
	if attempt < 2 {
		return Workspace{}, fmt.Errorf("%w: a fresh attempt is >= 2 (attempt 1 is EnsureWorkspace)", ErrBadInput)
	}
	return s.ensureWorkspaceAttempt(ctx, projectID, pipelineID, attempt)
}

// Snapshot takes a platform-owned, TREE-LEVEL snapshot commit of a worktree —
// capturing bash side effects (modified AND untracked files, and deletions),
// junk-excluded via the platform ignore rules, authored as the platform with
// per-invocation identity (Spec S13.5; S02.4d). It leaves the worktree's git
// state coherent for the engine's continued use (the commit advances the run
// branch that is checked out; the tree matches the new HEAD). When the tree is
// unchanged since the last snapshot it returns the existing tip — never an
// --allow-empty commit (Spec S13.5; still-valid D7 material). "" is returned
// only for a genuinely empty worktree with no HEAD.
func (s *Store) Snapshot(ctx context.Context, worktree string) (string, error) {
	id := platformIdentity
	// Stage every worktree change (tree-level), applying the platform excludes
	// so junk never enters a snapshot (structural code data, Spec S13.5).
	if _, err := s.git(ctx, worktree, id, "-c", "core.excludesFile="+s.excludes, "add", "-A"); err != nil {
		return "", err
	}
	// No-change → the prior snapshot (git diff --cached --quiet: exit 0 = the
	// staged tree equals HEAD, exit 1 = differs).
	code, _, stderr, err := s.gitRaw(ctx, worktree, id, "diff", "--cached", "--quiet")
	if err != nil {
		return "", err
	}
	head, err := s.headSHA(ctx, worktree)
	if err != nil {
		return "", err
	}
	switch code {
	case 0:
		return head, nil // unchanged (or empty repo → "")
	case 1:
		// something to snapshot: commit it.
	default:
		return "", fmt.Errorf("project: git diff --cached --quiet (exit %d): %s", code, strings.TrimSpace(stderr))
	}
	if _, err := s.git(ctx, worktree, id, "commit", "--no-verify", "--no-gpg-sign", "-m", snapshotMessage); err != nil {
		return "", err
	}
	return s.headSHA(ctx, worktree)
}

// AdvanceRunBranch fast-forwards a run branch to newSHA, refusing any
// non-fast-forward move (Spec S13.5: the platform NEVER moves a run-branch ref
// non-fast-forward while a deliverable is under review — the machinery only
// ever fast-forwards, so a rewrite is structurally impossible, no in-review
// bookkeeping required). There is NO amend path anywhere in the package.
func (s *Store) AdvanceRunBranch(ctx context.Context, projectID, pipelineID string, attempt int, newSHA string) error {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return err
	}
	branch := runBranch(pipelineID, attempt)
	ref := "refs/heads/" + branch
	cur, err := s.refSHA(ctx, e.StorePath, ref)
	if err != nil {
		return err
	}
	if cur == "" {
		return fmt.Errorf("%w: run branch %q does not exist", ErrNotFound, branch)
	}
	if cur == newSHA {
		return nil
	}
	ff, err := s.isAncestor(ctx, e.StorePath, cur, newSHA)
	if err != nil {
		return err
	}
	if !ff {
		return fmt.Errorf("%w: %s -> %s", ErrNotFastForward, cur[:min(8, len(cur))], newSHA[:min(8, len(newSHA))])
	}
	// --create-reflog off is irrelevant; the CAS old-value guards the move.
	_, err = s.git(ctx, e.StorePath, identity{}, "update-ref", ref, newSHA, cur)
	return err
}

// CreateRevisionRef creates the platform retention ref for a minted revision
// in the project store, pointing at its snapshot commit (Spec S13.1:
// refs/sinet/deliverable/<id>/rev-<n>; the exact string review.RevisionRef
// produces). Minted commits are thereby git-reachable independent of branches
// — workspace GC MUST NOT delete the only copy of a minted revision's objects.
// Driven from OUTSIDE internal/review (its storage+eventlog-only posture
// holds); the composition wires the git side and the review-side fill together.
func (s *Store) CreateRevisionRef(ctx context.Context, projectID, ref, snapshotSHA string) error {
	if ref == "" || snapshotSHA == "" {
		return fmt.Errorf("%w: revision ref needs a ref and a sha", ErrBadInput)
	}
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return err
	}
	if !s.objectExists(ctx, e.StorePath, snapshotSHA) {
		return fmt.Errorf("%w: snapshot %s not present in project %q", ErrBadInput, snapshotSHA, projectID)
	}
	_, err = s.git(ctx, e.StorePath, identity{}, "update-ref", ref, snapshotSHA)
	return err
}

// BaseContent materialises a pipeline's pre-task base tree (the base ref
// refs/sinet/base/<pipeline>) as a path→content file map — revision 1's old
// side (Spec S13.1; R25). Empty when the pipeline has no recorded base
// (a non-repo deliverable then keeps the empty-base behaviour).
func (s *Store) BaseContent(ctx context.Context, projectID, pipelineID string) (map[string]string, error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	// Revision 1's old side is attempt 1's base (the pre-task state, Spec
	// S13.1).
	base, err := s.refSHA(ctx, e.StorePath, baseRef(pipelineID, 1))
	if err != nil || base == "" {
		return nil, err
	}
	// -z gives NUL-terminated paths so a path with an embedded newline or a
	// quirky name is unambiguous (quotepath is already off).
	names, err := s.git(ctx, e.StorePath, identity{}, "ls-tree", "-r", "-z", "--name-only", base)
	if err != nil {
		return nil, err
	}
	files := map[string]string{}
	for _, name := range strings.Split(names, "\x00") {
		if name == "" {
			continue
		}
		// Byte-exact blob read: cat-file blob returns the blob VERBATIM — no
		// added trailing newline, no trimming. Leading blank lines, trailing
		// newlines and empty files are preserved so Compare line numbers and
		// old-side anchor mapping are exact (P-T12-2: silent mis-anchoring is
		// the worst class).
		content, err := s.blob(ctx, e.StorePath, base, name)
		if err != nil {
			return nil, err
		}
		files[name] = content
	}
	return files, nil
}

// blob returns a tree-ish path's blob content byte-exact (no trim, no added
// newline). A read error is loud, never a silent empty file.
func (s *Store) blob(ctx context.Context, store, treeish, path string) (string, error) {
	code, stdout, stderr, err := s.gitRaw(ctx, store, identity{}, "cat-file", "blob", treeish+":"+path)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("project: cat-file blob %s:%s (exit %d): %s", treeish, path, code, strings.TrimSpace(stderr))
	}
	return stdout, nil
}

// RepoHead returns the current HEAD of an ACTIVE project's default branch —
// the honest repo-HEAD member of the S02.6 freshness fingerprint (Spec S13.7
// "the registry feeds … workspace creation" and the S02.6 fingerprint; R33).
// "" for a headless project (never faked).
func (s *Store) RepoHead(ctx context.Context, projectID string) (string, error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return "", err
	}
	if !e.Active() {
		return "", fmt.Errorf("%w: %q", ErrNotActive, projectID)
	}
	return s.refSHA(ctx, e.StorePath, "refs/heads/"+e.DefaultBranch)
}

// ExistingWorkspace returns a pipeline's attempt-1 run-branch worktree PATH
// when it already exists (created by the execute leg), WITHOUT creating one —
// the snapshot/mint resolution for legs (intake, verify) that must not
// materialize a workspace (F4). ok=false when no worktree exists.
func (s *Store) ExistingWorkspace(ctx context.Context, projectID, pipelineID string) (string, bool, error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return "", false, err
	}
	path := s.worktreePath(projectID, pipelineID, 1)
	if s.worktreeRegistered(ctx, e.StorePath, path) {
		return path, true, nil
	}
	return "", false, nil
}

// worktreeRegistered reports whether path is a registered (non-prunable)
// worktree of store.
func (s *Store) worktreeRegistered(ctx context.Context, store, path string) bool {
	list, err := s.worktrees(ctx, store)
	if err != nil {
		return false
	}
	abs, _ := filepath.Abs(path)
	for _, w := range list {
		if w.Path == abs || w.Path == path {
			return !w.Prunable
		}
	}
	return false
}

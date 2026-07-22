package project

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// GitCommitter is the S09.2 knowledge-dir git commit-on-approval with the
// approver as author (D9 attribution). It satisfies internal/memory's
// Committer interface STRUCTURALLY — Commit(ctx, dir, paths, author, message)
// (hash, error) — WITHOUT importing internal/memory: the capability walls
// (CONVENTIONS §17) keep this git package clean of the knowledge write gate,
// and the shell hands the committer to memory.NewGate(...).Committer. A nil
// committer stays valid (the B3-1 posture: file_commit stays NULL and the
// migration trigger permits exactly the one NULL→hash fill when wired).
type GitCommitter struct{ s *Store }

// Committer returns the knowledge-dir git committer over this store.
func (s *Store) Committer() *GitCommitter { return &GitCommitter{s: s} }

// Commit ensures dir is (in) a git repository (init-if-absent — the knowledge
// file stores are "git-versioned" per Spec S13.10; nothing else initialises
// them), stages paths, and commits them with author = the approver (per-
// invocation identity via env, never global config mutation — Spec S09.2/D9).
// It returns the commit hash. Re-committing identical content is a no-op that
// returns the covering commit (HEAD) rather than erroring on an empty diff
// (R27: idempotent re-approval paths).
func (c *GitCommitter) Commit(ctx context.Context, dir string, paths []string, author, message string) (string, error) {
	if dir == "" || author == "" {
		return "", fmt.Errorf("%w: commit needs a dir and an author", ErrBadInput)
	}
	top, err := c.s.topLevel(ctx, dir)
	if err != nil {
		return "", err
	}
	if top == "" {
		// Not inside a repository: initialise one at dir (init-if-absent).
		if err := c.s.initRepo(ctx, dir, "main"); err != nil {
			return "", err
		}
	}
	id := identity{name: author, email: noreplyEmail(author)}
	if len(paths) > 0 {
		args := append([]string{"add", "--"}, paths...)
		if _, err := c.s.git(ctx, dir, id, args...); err != nil {
			return "", err
		}
	}
	// No-op tolerance (R27): nothing staged relative to HEAD → return the
	// covering commit. git diff --cached --quiet: exit 0 = no change.
	code, _, stderr, err := c.s.gitRaw(ctx, dir, id, "diff", "--cached", "--quiet")
	if err != nil {
		return "", err
	}
	if code == 0 {
		head, err := c.s.headSHA(ctx, dir)
		if err != nil {
			return "", err
		}
		if head != "" {
			return head, nil
		}
		return "", fmt.Errorf("%w: nothing to commit in a fresh repository", ErrBadInput)
	}
	if code != 1 {
		return "", fmt.Errorf("project: git diff --cached --quiet (exit %d): %s", code, strings.TrimSpace(stderr))
	}
	if _, err := c.s.git(ctx, dir, id, "commit", "--no-verify", "--no-gpg-sign", "-m", message); err != nil {
		return "", err
	}
	return c.s.headSHA(ctx, dir)
}

// EnsureRepo initialises dir as a git repository if it is not already inside
// one (Spec S13.10 "git-versioned file stores"). The shell calls it on the
// knowledge root so every scope dir shares one repo; the committer's own
// init-if-absent is the standalone fallback. dir is created if absent (the
// knowledge root may not exist until the first knowledge write).
func (s *Store) EnsureRepo(ctx context.Context, dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("project: knowledge dir: %w", err)
	}
	top, err := s.topLevel(ctx, dir)
	if err != nil {
		return err
	}
	if top != "" {
		return nil
	}
	return s.initRepo(ctx, dir, "main")
}

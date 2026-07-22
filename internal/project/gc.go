package project

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// Utility checkouts and workspace GC (Spec S13.5; S02.10 "workspace GC is a
// platform duty"). Utility checkouts are plain worktrees for platform use
// (accept staging B4-3, preview-at-rev B4-4): created locked, swept with
// `git worktree prune`, NEVER auto-deleted while dirty. GC flags orphan/dirty
// worktrees but never auto-deletes them. Prune runs opportunistically on the
// package's own create/teardown paths (no new timer — no ⚙ cadence exists;
// wiring timed sweeps into the recovery ladder is not this packet's, the
// ladder step-7 posture stays observation-only).

// worktreeInfo is one parsed `git worktree list --porcelain` block.
type worktreeInfo struct {
	Path     string
	HEAD     string
	Branch   string
	Detached bool
	Locked   bool
	Prunable bool
	Main     bool // the store's own worktree (first block)
}

// worktrees lists a store's worktrees.
func (s *Store) worktrees(ctx context.Context, store string) ([]worktreeInfo, error) {
	out, err := s.git(ctx, store, identity{}, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var (
		list []worktreeInfo
		cur  *worktreeInfo
		idx  int
	)
	flush := func() {
		if cur != nil {
			cur.Main = idx == 0
			list = append(list, *cur)
			idx++
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			flush()
			continue
		}
		key, val, _ := strings.Cut(line, " ")
		switch key {
		case "worktree":
			flush()
			cur = &worktreeInfo{Path: val}
		case "HEAD":
			if cur != nil {
				cur.HEAD = val
			}
		case "branch":
			if cur != nil {
				cur.Branch = val
			}
		case "detached":
			if cur != nil {
				cur.Detached = true
			}
		case "locked":
			if cur != nil {
				cur.Locked = true
			}
		case "prunable":
			if cur != nil {
				cur.Prunable = true
			}
		}
	}
	flush()
	return list, nil
}

// isDirty reports whether a worktree holds uncommitted work — modified OR
// untracked files (Spec S13.5: dirty worktrees are never auto-deleted).
func (s *Store) isDirty(ctx context.Context, worktree string) (bool, error) {
	out, err := s.git(ctx, worktree, identity{}, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// UtilityCheckout is a locked plain worktree for platform use.
type UtilityCheckout struct {
	ProjectID string `json:"project_id"`
	Ref       string `json:"ref"`
	Path      string `json:"path"`
}

// AddUtilityCheckout creates a LOCKED, detached worktree at a committish (Spec
// S13.5: accept staging / preview-at-rev). Locked-while-in-use so a stray
// prune never removes it; the consumer releases it (B4-3/B4-4 arrive here).
func (s *Store) AddUtilityCheckout(ctx context.Context, projectID, committish string) (UtilityCheckout, error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return UtilityCheckout{}, err
	}
	// Prune opportunistically on the create path (Spec S13.5).
	_, _ = s.git(ctx, e.StorePath, identity{}, "worktree", "prune")
	path := filepath.Join(s.root, "utility", sanitize(projectID), fmt.Sprintf("u-%d", s.clock().UnixNano()))
	if _, err := s.git(ctx, e.StorePath, identity{}, "worktree", "add", "--lock",
		"--reason", "sinet platform utility checkout (Spec S13.5)", "--detach", path, committish); err != nil {
		return UtilityCheckout{}, err
	}
	return UtilityCheckout{ProjectID: projectID, Ref: committish, Path: path}, nil
}

// ReleaseUtilityCheckout unlocks and removes a utility checkout — but REFUSES
// (ErrDirty) if it holds uncommitted work: a dirty checkout is never
// auto-deleted (Spec S13.5). Prunes afterward.
func (s *Store) ReleaseUtilityCheckout(ctx context.Context, projectID, path string) error {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return err
	}
	dirty, err := s.isDirty(ctx, path)
	if err != nil {
		return err
	}
	if dirty {
		return fmt.Errorf("%w: utility checkout %q", ErrDirty, path)
	}
	if _, err := s.git(ctx, e.StorePath, identity{}, "worktree", "unlock", path); err != nil {
		return err
	}
	if _, err := s.git(ctx, e.StorePath, identity{}, "worktree", "remove", path); err != nil {
		return err
	}
	_, err = s.git(ctx, e.StorePath, identity{}, "worktree", "prune")
	return err
}

// PruneWorktrees runs `git worktree prune`, clearing admin metadata for
// worktrees whose directories are already gone (Spec S13.5). It never deletes
// a live worktree.
func (s *Store) PruneWorktrees(ctx context.Context, projectID string) error {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return err
	}
	_, err = s.git(ctx, e.StorePath, identity{}, "worktree", "prune")
	return err
}

// WorktreeFlag is one GC flag (Spec S02.10: orphan/dirty worktrees are
// FLAGGED, never auto-deleted).
type WorktreeFlag struct {
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	Reason string `json:"reason"`
}

// FlagWorktrees reports GC-relevant worktrees — those holding uncommitted work
// and those git considers prunable (orphaned gitdir) — WITHOUT deleting any
// (Spec S02.10 platform GC duty; the recovery ladder step-7 observation-only
// posture is unchanged). The store's own worktree is never flagged.
func (s *Store) FlagWorktrees(ctx context.Context, projectID string) ([]WorktreeFlag, error) {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return nil, err
	}
	list, err := s.worktrees(ctx, e.StorePath)
	if err != nil {
		return nil, err
	}
	var flags []WorktreeFlag
	for _, w := range list {
		if w.Main {
			continue
		}
		if w.Prunable {
			flags = append(flags, WorktreeFlag{Path: w.Path, Branch: w.Branch, Reason: "orphan worktree (prunable gitdir)"})
			continue
		}
		dirty, err := s.isDirty(ctx, w.Path)
		if err != nil {
			// A worktree whose directory vanished reads as prunable next sweep;
			// don't fail the whole flag pass on one unreadable path.
			continue
		}
		if dirty {
			flags = append(flags, WorktreeFlag{Path: w.Path, Branch: w.Branch, Reason: "holds uncommitted work"})
		}
	}
	return flags, nil
}

// CollectRunWorktree is revision-retention GC for an abandoned run attempt
// (Spec S13.5/S02.10): it removes a run-branch worktree and deletes its branch
// — but ONLY when clean (a dirty worktree is refused with ErrDirty, never
// auto-deleted). Minted-revision commits survive branch deletion because their
// refs/sinet/deliverable refs keep them reachable (Spec S13.1, R20): branch
// deletion is admissible precisely because the platform refs protect the
// minted objects.
func (s *Store) CollectRunWorktree(ctx context.Context, projectID, pipelineID string, attempt int) error {
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return err
	}
	branch := runBranch(pipelineID, attempt)
	path := s.worktreePath(projectID, pipelineID, attempt)
	if s.worktreeRegistered(ctx, e.StorePath, path) {
		dirty, err := s.isDirty(ctx, path)
		if err == nil && dirty {
			return fmt.Errorf("%w: run worktree %q", ErrDirty, path)
		}
		if _, err := s.git(ctx, e.StorePath, identity{}, "worktree", "remove", path); err != nil {
			return err
		}
	}
	if exists, _ := s.refSHA(ctx, e.StorePath, "refs/heads/"+branch); exists != "" {
		if _, err := s.git(ctx, e.StorePath, identity{}, "branch", "-D", branch); err != nil {
			return err
		}
	}
	_, err = s.git(ctx, e.StorePath, identity{}, "worktree", "prune")
	return err
}

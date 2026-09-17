package project

import (
	"context"
	"errors"
)

// Read-only tree verbs over pinned commits of a project store (Spec S13.1: a
// repo-backed revision pins a snapshot commit; S13.2: the reviewable change is
// computed host-side by git between revision pins). Consumed by review through
// the composition root's TreeSource adapter (internal/shell), never by import.
//
// P3-SIT-1 GROUNDING — INERT SURFACE (CONVENTIONS §3, amendment-A carve-out):
// the signatures exist so the committed acceptance tests compile and FAIL on
// behaviour; every verb answers errTreeNotBuilt until the packet's
// implementation commit replaces it. Kept in its own file so the concurrently
// edited workspace.go is untouched.

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

var errTreeNotBuilt = errors.New("project: tree reads are P3-SIT-1's implementation (inert grounding surface)")

// BaseSHA reads a pipeline's recorded attempt-1 base commit
// (refs/sinet/base/<pipeline>, Spec S13.5) — revision 1's old side (Spec
// S13.1). "" when none is recorded; never a guess.
func (s *Store) BaseSHA(ctx context.Context, projectID, pipelineID string) (string, error) {
	return "", errTreeNotBuilt
}

// TreeChanges lists every path that differs between two commits of the
// project store, in path order, with kind, sizes, git's binary verdict and
// numstat counts — computed by git plumbing over the store, never by reading
// file bodies into the platform. Both shas must be commits the store holds.
func (s *Store) TreeChanges(ctx context.Context, projectID, oldSHA, newSHA string) ([]TreeChange, error) {
	return nil, errTreeNotBuilt
}

// TreeBlob reads one path's blob at a commit, byte-exact, returning at most
// limit bytes (limit <= 0: whole) and the blob's full size. ok=false when the
// path is not in that tree.
func (s *Store) TreeBlob(ctx context.Context, projectID, treeish, path string, limit int64) (data []byte, size int64, ok bool, err error) {
	return nil, 0, false, errTreeNotBuilt
}

package project

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// The S13.6 accept git verbs, riding the hermetic per-invocation core
// (CONVENTIONS §23/§27): the applies-cleanly depth-1 merge gate and the
// platform squash into ONE attributed commit. The outward ref-update to the
// remote is the broker's (S13.6 step 4, P-T12-1) — this package still performs
// no outward ref-update and no history rewrite (the §23 source-scan tripwire
// still passes: none of the banned mechanism tokens appear here). The accept
// commit is a FRESH commit on HEAD via commit-object write, never a rewrite of
// the run branch.

// MergeResult is the outcome of the applies-cleanly gate (Spec S13.6 step 1).
type MergeResult struct {
	// Clean is true when the candidate applies onto `onto` with no conflict.
	Clean bool
	// Tree is the merged tree object (the accept commit's tree). When `onto`
	// has not diverged from the candidate's base it equals the candidate's own
	// tree (Spec S13.6 step 2: "tree is the accepted revision's"); when HEAD
	// moved with non-conflicting work it is the incorporated merge — never a
	// silent drop of either side.
	Tree string
	// Conflicts is git's conflict report (the merge card body) when !Clean.
	Conflicts string
}

// AppliesClean applies a candidate revision commit onto `onto` via a depth-1
// merge (git merge-tree --write-tree), the merge-queue idiom of Spec S13.6
// step 1. `onto` is current default-branch HEAD for the first accept, or
// HEAD-plus-first (the first in-flight accept's commit) for a second — the
// depth-1 queue. It NEVER touches the working tree or any ref, so a second
// in-flight accept validates without disturbing the first. A collision
// (Clean=false) is surfaced to the caller as a reviewable merge card — never a
// silent overwrite (S1.11 verbatim).
func (s *Store) AppliesClean(ctx context.Context, projectID, onto, candidate string) (MergeResult, error) {
	if onto == "" || candidate == "" {
		return MergeResult{}, fmt.Errorf("%w: applies-clean needs onto and candidate commits", ErrBadInput)
	}
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return MergeResult{}, err
	}
	code, stdout, stderr, err := s.gitRaw(ctx, e.StorePath, identity{}, "merge-tree", "--write-tree", onto, candidate)
	if err != nil {
		return MergeResult{}, err
	}
	switch code {
	case 0:
		tree := strings.TrimSpace(stdout)
		if tree == "" {
			return MergeResult{}, fmt.Errorf("project: merge-tree returned no tree")
		}
		return MergeResult{Clean: true, Tree: tree}, nil
	case 1:
		// Conflict: stdout carries the merged tree OID then the conflict
		// report; hand the whole thing to the merge card.
		return MergeResult{Clean: false, Conflicts: strings.TrimSpace(stdout + "\n" + stderr)}, nil
	default:
		return MergeResult{}, fmt.Errorf("project: merge-tree (exit %d): %s", code, strings.TrimSpace(stderr))
	}
}

// SquashInput is the platform squash into one attributed commit (Spec S13.6
// step 2).
type SquashInput struct {
	// Tree is the accepted tree (the AppliesClean output); Parent is `onto`
	// (current HEAD), so the accept commit is a fresh commit on HEAD.
	Tree, Parent string
	// UserID is the accepting user (the ID-based noreply committer email
	// survives renames, Spec S13.6 step 2); AuthorName is the display name.
	UserID, AuthorName string
	// Message is the full commit message (Conventional-Commit subject + body
	// with the task/session provenance link + the deterministic attribution
	// trailers). The trailers are rendered ABOVE this package (the structural
	// template lives in internal/accept; §8 reading 2) and passed in whole.
	Message string
	// When is the author/committer timestamp (recorded UTC for determinism).
	When time.Time
	// Sign, when non-nil, SSHSIG-signs the commit payload (namespace "git")
	// and the result is embedded as the gpgsig header — the broker-mediated
	// signing seam (Spec S13.6 step 5). The git-ssh-key never leaves the
	// broker: only the armored signature crosses this seam. nil = unsigned.
	Sign func(payload []byte) (armoredSig []byte, err error)
}

// SquashAccept creates ONE clean attributed commit placed on current HEAD
// (Spec S13.6 step 2). Its tree is the AppliesClean RESULT: when HEAD has not
// diverged this equals the accepted revision's own tree ("tree is the accepted
// revision's", S13.6 step 2); when HEAD moved with non-conflicting interim
// work the merged tree incorporates BOTH — committing the raw revision tree
// would SILENTLY DISCARD the interim work, contradicting S13.6's own
// no-silent-overwrite rule, so the ratified reading commits the merge result
// (coordinator-ratified drain round 1, F6). Author = committer = the accepting
// user, set per-invocation via the object bytes (no global/host git config is
// ever touched — §23). No ref is moved: the commit is created as
// an object the broker then CAS-pushes (S13.6 step 4); the local default
// branch advances only on push success (AdvanceDefaultBranch). Returns the new
// commit sha. There is NO --amend — the accept commit is fresh, not a rewrite.
func (s *Store) SquashAccept(ctx context.Context, projectID string, in SquashInput) (string, error) {
	if in.Tree == "" || in.Parent == "" {
		return "", fmt.Errorf("%w: squash needs a tree and a parent", ErrBadInput)
	}
	if in.UserID == "" || in.Message == "" {
		return "", fmt.Errorf("%w: squash needs an accepting user and a message", ErrBadInput)
	}
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return "", err
	}
	name := in.AuthorName
	if name == "" {
		name = in.UserID
	}
	when := in.When
	if when.IsZero() {
		when = time.Now()
	}
	obj, err := buildCommitObject(in.Tree, in.Parent, name, noreplyEmail(in.UserID), when, in.Message, in.Sign)
	if err != nil {
		return "", err
	}
	// Write the commit object into the project store's object database. A
	// content-addressed write is idempotent; the accepting-user identity rides
	// the object bytes, never a config mutation (§23).
	sha, err := s.gitStdin(ctx, e.StorePath, obj, "hash-object", "-t", "commit", "-w", "--stdin")
	if err != nil {
		return "", err
	}
	return sha, nil
}

// AdvanceDefaultBranch fast-forwards the project's LOCAL default branch to
// newSHA (Spec S13.6): the broker has already CAS-pushed newSHA to the remote's
// protected ref, so this syncs the local store to match. It re-reads the local
// head and CAS-updates against it; the move is a no-op when newSHA is already
// incorporated (an ancestor of the local head — the depth-1 stacked case where
// a later accept already advanced local past this one), a fast-forward when the
// local head is behind, and a LOUD error on genuine divergence. Never a rewrite.
func (s *Store) AdvanceDefaultBranch(ctx context.Context, projectID, newSHA string) error {
	if newSHA == "" {
		return fmt.Errorf("%w: advance needs a new sha", ErrBadInput)
	}
	e, err := s.Get(ctx, projectID)
	if err != nil {
		return err
	}
	ref := "refs/heads/" + e.DefaultBranch
	cur, err := s.refSHA(ctx, e.StorePath, ref)
	if err != nil {
		return err
	}
	if cur == newSHA {
		return nil
	}
	if cur != "" {
		if incorporated, err := s.isAncestor(ctx, e.StorePath, newSHA, cur); err != nil {
			return err
		} else if incorporated {
			return nil // already incorporated (a later stacked accept advanced past this)
		}
		ff, err := s.isAncestor(ctx, e.StorePath, cur, newSHA)
		if err != nil {
			return err
		}
		if !ff {
			return fmt.Errorf("%w: default branch %s -> %s", ErrNotFastForward, cur[:min(8, len(cur))], newSHA[:min(8, len(newSHA))])
		}
	}
	// CAS on the re-read current value so a concurrent local move fails loudly.
	_, err = s.git(ctx, e.StorePath, identity{}, "update-ref", ref, newSHA, cur)
	return err
}

// buildCommitObject assembles a git commit object, optionally SSH-signed. The
// signed payload is the object WITHOUT the gpgsig header (exactly what git
// itself signs and re-verifies); the armored SSHSIG is embedded as a gpgsig
// header with each continuation line prefixed by one space (the git multi-line
// header format), inserted right after the committer line.
func buildCommitObject(tree, parent, name, email string, when time.Time, message string, sign func([]byte) ([]byte, error)) ([]byte, error) {
	ts := when.UTC().Unix()
	who := fmt.Sprintf("%s <%s> %d +0000", name, email, ts)
	var head bytes.Buffer
	fmt.Fprintf(&head, "tree %s\n", tree)
	fmt.Fprintf(&head, "parent %s\n", parent)
	fmt.Fprintf(&head, "author %s\n", who)
	fmt.Fprintf(&head, "committer %s\n", who)

	// Ensure the message ends with exactly one trailing newline.
	body := strings.TrimRight(message, "\n") + "\n"

	if sign == nil {
		var obj bytes.Buffer
		obj.Write(head.Bytes())
		obj.WriteString("\n")
		obj.WriteString(body)
		return obj.Bytes(), nil
	}

	// Sign the UNSIGNED object (headers + blank + body), then embed.
	var payload bytes.Buffer
	payload.Write(head.Bytes())
	payload.WriteString("\n")
	payload.WriteString(body)
	sig, err := sign(payload.Bytes())
	if err != nil {
		return nil, fmt.Errorf("project: sign accept commit: %w", err)
	}
	sigLines := strings.Split(strings.TrimRight(string(sig), "\n"), "\n")
	if len(sigLines) == 0 || sigLines[0] == "" {
		return nil, fmt.Errorf("project: empty commit signature")
	}
	var obj bytes.Buffer
	obj.Write(head.Bytes())
	obj.WriteString("gpgsig " + sigLines[0] + "\n")
	for _, l := range sigLines[1:] {
		obj.WriteString(" " + l + "\n")
	}
	obj.WriteString("\n")
	obj.WriteString(body)
	return obj.Bytes(), nil
}

// gitStdin runs git with stdin (the commit object for hash-object) and returns
// trimmed stdout, hermetic like every other git act (§23).
func (s *Store) gitStdin(ctx context.Context, dir string, stdin []byte, args ...string) (string, error) {
	full := append([]string{"-c", "core.quotepath=false", "-c", "gc.auto=0"}, args...)
	cmd := exec.CommandContext(ctx, gitBin, full...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = baseEnv(identity{})
	cmd.Stdin = bytes.NewReader(stdin)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("project: git %s: %s: %w", args[0], strings.TrimSpace(errBuf.String()), err)
	}
	return strings.TrimSpace(out.String()), nil
}

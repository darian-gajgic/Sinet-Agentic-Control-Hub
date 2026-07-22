package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// The host git CLI, consumed UNMODIFIED as an external subprocess (the
// components.lock `git (host CLI)` os-mechanism entry, materialized at B4-1;
// its own note names S13.5 as the heavier consumer landing here). Every
// platform-owned git act is HERMETIC and per-invocation:
//
//   - no host/global config is read or written (GIT_CONFIG_GLOBAL/SYSTEM ->
//     /dev/null; GIT_CONFIG_NOSYSTEM=1) — the S13.6 step-2 "no global host
//     state" rule generalized to every platform git act (Spec S13.5/S13.6);
//   - identity rides env (GIT_AUTHOR_*/GIT_COMMITTER_*), never a config
//     mutation (Spec S13.5 "per-invocation identity");
//   - only the local file transport is permitted (GIT_ALLOW_PROTOCOL=file):
//     the dev-mode no-network-remotes constraint enforced at the process
//     boundary — the registry's remote URL is stored data, never dialed;
//   - behaviour is asserted by the fixture tests on THIS host git, never
//     parsed from --help (CONVENTIONS §10 rule 4).

// gitBin is the git executable name, resolved via PATH. Absence is a LOUD
// error on every git path (the review/diff.go precedent), never a silent
// degrade.
const gitBin = "git"

// identity is a per-invocation git author/committer identity.
type identity struct {
	name  string
	email string
}

// platformIdentity authors platform-owned snapshot commits (Spec S13.5:
// snapshot commits are platform-owned, per-invocation identity). The email is
// a stable no-reply form so a snapshot never attributes to a person.
var platformIdentity = identity{name: "Sinet platform", email: "platform@sinet.invalid"}

// noreplyEmail is the ID-based no-reply committer email (Spec S13.6 step 2:
// "the ID-based noreply form (survives renames)"). Dev-mode has no connected
// GitHub address, so the platform-local no-reply form is used throughout.
func noreplyEmail(userID string) string {
	return userID + "@users.noreply.sinet.invalid"
}

// baseEnv is the hermetic environment shared by every git invocation.
func baseEnv(id identity) []string {
	env := []string{
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ALLOW_PROTOCOL=file",
		// A hermetic HOME keeps a stray ~/.gitconfig or credential helper out
		// of reach even if GIT_CONFIG_GLOBAL were ignored by an old git.
		"HOME=/nonexistent",
	}
	if p := os.Getenv("PATH"); p != "" {
		env = append(env, "PATH="+p)
	}
	if id.name != "" {
		env = append(env,
			"GIT_AUTHOR_NAME="+id.name, "GIT_AUTHOR_EMAIL="+id.email,
			"GIT_COMMITTER_NAME="+id.name, "GIT_COMMITTER_EMAIL="+id.email)
	}
	return env
}

// git runs a git subcommand in dir with the given identity, returning trimmed
// stdout. A non-zero exit is an error carrying git's stderr (loud, never a
// silent fallback). `-c gc.auto=0` stops opportunistic gc from ever pruning
// minted-revision objects mid-operation (Spec S13.1: workspace GC must not
// delete the only copy of a minted revision's objects).
func (s *Store) git(ctx context.Context, dir string, id identity, args ...string) (string, error) {
	code, out, stderr, err := s.gitRaw(ctx, dir, id, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("project: git %s (exit %d): %s", args[0], code, strings.TrimSpace(stderr))
	}
	return strings.TrimSpace(out), nil
}

// gitRaw runs git and returns the exit code without treating a non-zero code
// as a spawn error — the caller interprets the code (the review/diff.go
// pattern for `git diff --quiet`, where exit 1 means "differs"). A negative
// code with an error is a spawn/OS failure (git absent).
func (s *Store) gitRaw(ctx context.Context, dir string, id identity, args ...string) (code int, stdout, stderr string, err error) {
	full := append([]string{"-c", "core.quotepath=false", "-c", "gc.auto=0"}, args...)
	cmd := exec.CommandContext(ctx, gitBin, full...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = baseEnv(id)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	if runErr == nil {
		return 0, outBuf.String(), errBuf.String(), nil
	}
	var ee *exec.ExitError
	if errors.As(runErr, &ee) {
		return ee.ExitCode(), outBuf.String(), errBuf.String(), nil
	}
	return -1, outBuf.String(), errBuf.String(), fmt.Errorf("project: git %s: %w", args[0], runErr)
}

// initRepo initialises a git repository at dir with a fixed default branch,
// creating dir if absent (Spec S13.7: a v0 project without a repo gets one
// created; the committer's init-if-absent rides this too, Spec S09.2).
func (s *Store) initRepo(ctx context.Context, dir, defaultBranch string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("project: repo dir: %w", err)
	}
	_, err := s.git(ctx, dir, identity{}, "init", "-q", "--initial-branch="+defaultBranch, dir)
	return err
}

// isRepo reports whether dir is inside a git working tree.
func (s *Store) isRepo(ctx context.Context, dir string) bool {
	out, err := s.git(ctx, dir, identity{}, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// topLevel returns the working-tree root containing dir, or "" if dir is not
// in a repository.
func (s *Store) topLevel(ctx context.Context, dir string) (string, error) {
	code, out, _, err := s.gitRaw(ctx, dir, identity{}, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

// headSHA returns the commit HEAD resolves to, or "" when the repository has
// no commits yet (an unborn branch).
func (s *Store) headSHA(ctx context.Context, dir string) (string, error) {
	code, out, _, err := s.gitRaw(ctx, dir, identity{}, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

// refSHA returns the commit a ref points at, or "" when the ref is absent.
func (s *Store) refSHA(ctx context.Context, store, ref string) (string, error) {
	code, out, _, err := s.gitRaw(ctx, store, identity{}, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

// objectExists reports whether the object sha is present in store (the
// minted-revision reachability probe: after a branch/worktree is swept, a
// ref-protected snapshot commit survives — Spec S13.1).
func (s *Store) objectExists(ctx context.Context, store, sha string) bool {
	code, _, _, err := s.gitRaw(ctx, store, identity{}, "cat-file", "-e", sha+"^{commit}")
	return err == nil && code == 0
}

// isAncestor reports whether ancestor is an ancestor of descendant (the
// fast-forward test behind never-force ref moves, Spec S13.5).
func (s *Store) isAncestor(ctx context.Context, store, ancestor, descendant string) (bool, error) {
	code, _, stderr, err := s.gitRaw(ctx, store, identity{}, "merge-base", "--is-ancestor", ancestor, descendant)
	if err != nil {
		return false, err
	}
	switch code {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("project: merge-base --is-ancestor (exit %d): %s", code, strings.TrimSpace(stderr))
	}
}

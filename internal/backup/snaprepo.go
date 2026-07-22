package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// The snapshot repo (Spec S13.10; D9): the operator-owned private repo the
// encrypted blobs commit to, keep-N on one branch. The platform keeps a
// persistent LOCAL working clone and is the SOLE writer, so the local branch
// tip is always the authoritative remote tip after the last successful push —
// no fetch is needed in the snapshot flow (the drill, which proves recovery
// from the actual remote, fetches). Commits are local and credential-free; the
// PUSH is the broker's (the one credentialed, networked op). Dev fixtures use
// file:// bare remotes in t.TempDir(); the live leg is pure-config (ssh remote
// + broker key), no code change (R24).

// zeroSHA is the CAS expect value for a ref that does not yet exist — the first
// snapshot push (git accepts --force-with-lease=<ref>:<40 zeros> meaning "the
// ref must be absent"). The broker forbids an EMPTY expect (bare lease), so the
// first push carries the explicit zero-sha.
const zeroSHA = "0000000000000000000000000000000000000000"

// snapCommit is the outcome of committing a blob to the local snapshot repo.
type snapCommit struct {
	NewSHA    string // the new commit (broker push source)
	ExpectSHA string // the branch tip BEFORE this commit (the CAS lease value)
	BlobBase  string // the blob's base name in the tree (its chunk files are <base>.NNN)
}

// writeSnapshotCommit ensures the local snapshot clone, writes the blob (chunked
// if it exceeds the ~90 MB threshold), prunes the dropBases (keep-N retention,
// computed by the caller from the ledger's authoritative order), and commits —
// returning the new commit and the CAS expect (the prior tip). It performs NO
// network op; the caller hands NewSHA/ExpectSHA to the broker push.
func (s *Snapshotter) writeSnapshotCommit(ctx context.Context, blob []byte, base string, dropBases []string) (snapCommit, error) {
	rc := s.cfg.Repo
	if err := s.ensureRepo(ctx, rc); err != nil {
		return snapCommit{}, err
	}
	branchRef := "refs/heads/" + rc.Branch
	expect, err := s.refSHA(ctx, rc.LocalDir, branchRef)
	if err != nil {
		return snapCommit{}, err
	}
	if expect == "" {
		expect = zeroSHA // first snapshot: the branch does not exist yet
	}

	// Write the blob's chunk files (single chunk for household-sized blobs).
	chunks := splitBlob(blob, s.cfg.ChunkThresholdBytes)
	for i, c := range chunks {
		name := fmt.Sprintf("%s.%03d", base, i)
		if err := os.WriteFile(filepath.Join(rc.LocalDir, name), c, 0o600); err != nil {
			return snapCommit{}, fmt.Errorf("backup: write blob chunk: %w", err)
		}
	}
	if err := s.keepN(ctx, rc, dropBases); err != nil {
		return snapCommit{}, err
	}

	if _, err := s.git(ctx, rc.LocalDir, "add", "-A"); err != nil {
		return snapCommit{}, err
	}
	if _, err := s.git(ctx, rc.LocalDir, "commit", "--no-verify", "--no-gpg-sign",
		"-m", "sinet: platform snapshot "+base+" (Spec S13.10)"); err != nil {
		return snapCommit{}, err
	}
	newSHA, err := s.refSHA(ctx, rc.LocalDir, "HEAD")
	if err != nil {
		return snapCommit{}, err
	}
	return snapCommit{NewSHA: newSHA, ExpectSHA: expect, BlobBase: base}, nil
}

// ensureRepo makes LocalDir a working clone of the snapshot repo on the
// configured branch. A persistent clone is reused; a fresh one is cloned (or,
// for an empty remote, initialized) and the branch ensured.
func (s *Snapshotter) ensureRepo(ctx context.Context, rc RepoConfig) error {
	if isDir(filepath.Join(rc.LocalDir, ".git")) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(rc.LocalDir), 0o700); err != nil {
		return fmt.Errorf("backup: snapshot clone parent: %w", err)
	}
	// Clone the remote (empty or not); an empty remote yields an unborn HEAD.
	if _, err := s.git(ctx, "", "clone", rc.Remote, rc.LocalDir); err != nil {
		return err
	}
	// Ensure the target branch exists (checkout -B works on an unborn HEAD too).
	if _, err := s.git(ctx, rc.LocalDir, "checkout", "-B", rc.Branch); err != nil {
		return err
	}
	return nil
}

// keepN removes the chunk files of the dropBases from the snapshot repo tree
// (Spec S13.10 keep-N on one branch). The caller computes dropBases from the
// snapshot_ledger's authoritative seq order, so retention is independent of
// filename sorting (robust when several snapshots share a timestamp). The
// snapshot_ledger stays append-only — retention prunes the REPO blobs, not the
// ledger rows.
func (s *Snapshotter) keepN(ctx context.Context, rc RepoConfig, dropBases []string) error {
	if len(dropBases) == 0 {
		return nil
	}
	drop := map[string]bool{}
	for _, b := range dropBases {
		drop[b] = true
	}
	entries, err := os.ReadDir(rc.LocalDir)
	if err != nil {
		return fmt.Errorf("backup: read snapshot dir: %w", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), snapshotPrefix) {
			continue
		}
		if drop[chunkBase(e.Name())] {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for _, f := range files {
		if _, err := s.git(ctx, rc.LocalDir, "rm", "-q", "--", f); err != nil {
			return err
		}
	}
	return nil
}

// git runs a hermetic git subcommand for the snapshot repo (no host/global
// config; file transport for dev fixtures, ssh for the live leg — derived from
// the remote). Absence of git is a loud error.
func (s *Snapshotter) git(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-c", "core.quotepath=false", "-c", "gc.auto=0"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = snapGitEnv(s.cfg.Repo.Remote)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("backup: git %s: %s: %w", args[0], strings.TrimSpace(errBuf.String()), err)
	}
	return strings.TrimSpace(out.String()), nil
}

// refSHA returns the commit a ref resolves to, or "" when absent.
func (s *Snapshotter) refSHA(ctx context.Context, dir, ref string) (string, error) {
	full := []string{"-c", "gc.auto=0", "rev-parse", "--verify", "--quiet", ref + "^{commit}"}
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = dir
	cmd.Env = snapGitEnv(s.cfg.Repo.Remote)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", nil // ref absent
		}
		return "", fmt.Errorf("backup: rev-parse %s: %w", ref, err)
	}
	return strings.TrimSpace(out.String()), nil
}

func snapGitEnv(remote string) []string {
	proto := "ssh"
	if strings.HasPrefix(remote, "file://") {
		proto = "file"
	}
	env := []string{
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ALLOW_PROTOCOL=" + proto,
		"HOME=/nonexistent",
		// Snapshot commits are platform-owned, per-invocation identity (S13.5
		// generalized) — never a person, never a host config mutation.
		"GIT_AUTHOR_NAME=Sinet platform",
		"GIT_AUTHOR_EMAIL=platform@sinet.invalid",
		"GIT_COMMITTER_NAME=Sinet platform",
		"GIT_COMMITTER_EMAIL=platform@sinet.invalid",
	}
	if p := os.Getenv("PATH"); p != "" {
		env = append(env, "PATH="+p)
	}
	return env
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

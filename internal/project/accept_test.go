package project

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The S13.6 accept git-verb battery: the applies-cleanly depth-1 merge, the
// platform squash into one attributed commit (author=committer=accepting user,
// no host config touched), the SSH-signed variant (the gpgsig embed verifies),
// and the local default-branch advance. All hermetic fixtures.

// TestAppliesCleanAndSquash: a clean candidate applies onto HEAD (merged tree
// equals the candidate's tree when HEAD has not diverged), squashes into one
// attributed commit on HEAD, and the local default branch advances.
func TestAppliesCleanAndSquash(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("proj", "u1", "P", map[string]string{"a.txt": "base\n"})

	ws, err := f.store.EnsureWorkspace(ctx, "proj", "pipe1")
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	f.writeFiles(ws.Path, map[string]string{"a.txt": "candidate\n"})
	candidate, err := f.store.Snapshot(ctx, ws.Path)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	head, err := f.store.RepoHead(ctx, "proj")
	if err != nil {
		t.Fatalf("RepoHead: %v", err)
	}

	mr, err := f.store.AppliesClean(ctx, "proj", head, candidate)
	if err != nil {
		t.Fatalf("AppliesClean: %v", err)
	}
	if !mr.Clean {
		t.Fatalf("clean candidate reported a collision: %s", mr.Conflicts)
	}
	candTree := f.git(e.StorePath, "rev-parse", candidate+"^{tree}")
	if mr.Tree != candTree {
		t.Errorf("merged tree %s != candidate tree %s (no divergence)", mr.Tree, candTree)
	}

	commit, err := f.store.SquashAccept(ctx, "proj", SquashInput{
		Tree: mr.Tree, Parent: head, UserID: "u1", AuthorName: "User One",
		Message: "feat: accept the candidate\n\nTask: t1\n", When: time.Unix(1_700_000_000, 0),
	})
	if err != nil {
		t.Fatalf("SquashAccept: %v", err)
	}
	// One attributed commit: tree = accepted revision's, parent = HEAD,
	// author = committer = accepting user (ID-based noreply email).
	if got := f.git(e.StorePath, "rev-parse", commit+"^{tree}"); got != candTree {
		t.Errorf("accept commit tree %s != candidate tree %s", got, candTree)
	}
	if got := f.git(e.StorePath, "rev-parse", commit+"^"); got != head {
		t.Errorf("accept commit parent %s != HEAD %s", got, head)
	}
	body := f.git(e.StorePath, "cat-file", "-p", commit)
	if !strings.Contains(body, "author User One <u1@users.noreply.sinet.invalid>") {
		t.Errorf("accept commit not attributed to the accepting user:\n%s", body)
	}
	if !strings.Contains(body, "committer User One <u1@users.noreply.sinet.invalid>") {
		t.Errorf("committer is not the accepting user:\n%s", body)
	}

	// The local default branch advances to the accept commit (ff, CAS).
	if err := f.store.AdvanceDefaultBranch(ctx, "proj", commit, head); err != nil {
		t.Fatalf("AdvanceDefaultBranch: %v", err)
	}
	if got, _ := f.store.RepoHead(ctx, "proj"); got != commit {
		t.Errorf("default branch = %s, want %s", got, commit)
	}
}

// TestSquashAcceptSigned: the broker-mediated signing seam produces an
// SSH-signed accept commit whose gpgsig header verifies (Spec S13.6 step 5).
func TestSquashAcceptSigned(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("proj", "u1", "P", map[string]string{"a.txt": "base\n"})
	ws, _ := f.store.EnsureWorkspace(ctx, "proj", "pipe1")
	f.writeFiles(ws.Path, map[string]string{"a.txt": "signed candidate\n"})
	candidate, _ := f.store.Snapshot(ctx, ws.Path)
	head, _ := f.store.RepoHead(ctx, "proj")
	mr, _ := f.store.AppliesClean(ctx, "proj", head, candidate)

	// A signer that mimics the broker: ssh-keygen -Y sign with a temp key.
	keyDir := t.TempDir()
	keyPath := filepath.Join(keyDir, "gitkey")
	f.run("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", keyPath, "-C", "u1")
	signer := func(payload []byte) ([]byte, error) {
		pf := filepath.Join(keyDir, "payload")
		if err := os.WriteFile(pf, payload, 0o600); err != nil {
			return nil, err
		}
		cmd := exec.Command("ssh-keygen", "-Y", "sign", "-f", keyPath, "-n", "git", pf)
		cmd.Env = gitEnv()
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, errFrom(out, err)
		}
		return os.ReadFile(pf + ".sig")
	}
	commit, err := f.store.SquashAccept(ctx, "proj", SquashInput{
		Tree: mr.Tree, Parent: head, UserID: "u1", AuthorName: "User One",
		Message: "feat: signed accept\n", When: time.Unix(1_700_000_000, 0), Sign: signer,
	})
	if err != nil {
		t.Fatalf("SquashAccept signed: %v", err)
	}
	if !strings.Contains(f.git(e.StorePath, "cat-file", "-p", commit), "gpgsig -----BEGIN SSH SIGNATURE-----") {
		t.Fatal("signed accept commit has no gpgsig header")
	}
	// The signature verifies against an allowed-signers file.
	pub, _ := os.ReadFile(keyPath + ".pub")
	allowed := filepath.Join(keyDir, "allowed")
	os.WriteFile(allowed, []byte("u1@users.noreply.sinet.invalid namespaces=\"git\" "+string(pub)), 0o600)
	out := f.gitEnvCmd(e.StorePath, "-c", "gpg.format=ssh", "-c", "gpg.ssh.allowedSignersFile="+allowed, "verify-commit", commit)
	if !strings.Contains(out, "Good \"git\" signature") {
		t.Errorf("accept commit signature did not verify:\n%s", out)
	}
}

// TestAppliesCleanCollision: a candidate that conflicts with a moved HEAD
// reports a collision (the merge-card path), never a silent overwrite.
func TestAppliesCleanCollision(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	e := f.activeProject("proj", "u1", "P", map[string]string{"a.txt": "line1\nline2\n"})
	ws, _ := f.store.EnsureWorkspace(ctx, "proj", "pipe1")
	f.writeFiles(ws.Path, map[string]string{"a.txt": "line1\nCANDIDATE\n"})
	candidate, _ := f.store.Snapshot(ctx, ws.Path)
	base, _ := f.store.RepoHead(ctx, "proj")

	// Move HEAD with a CONFLICTING change to the same line.
	f.git(e.StorePath, "-c", "user.name=x", "-c", "user.email=x@x", "commit", "--allow-empty", "-m", "noop")
	f.writeFiles(e.StorePath, map[string]string{"a.txt": "line1\nHEADSIDE\n"})
	f.git(e.StorePath, "add", "-A")
	f.git(e.StorePath, "-c", "user.name=x", "-c", "user.email=x@x", "commit", "-m", "head moved")
	moved := f.git(e.StorePath, "rev-parse", "HEAD")
	if moved == base {
		t.Fatal("setup: HEAD did not move")
	}

	mr, err := f.store.AppliesClean(ctx, "proj", moved, candidate)
	if err != nil {
		t.Fatalf("AppliesClean: %v", err)
	}
	if mr.Clean {
		t.Fatal("a conflicting candidate was reported clean (silent-overwrite risk)")
	}
	if mr.Conflicts == "" {
		t.Error("collision produced no conflict report for the merge card")
	}
}

// run executes a command in test setup, failing on error.
func (f *fix) run(name string, args ...string) {
	f.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("%s: %v\n%s", name, err, out)
	}
}

// gitEnvCmd runs a git command returning combined output WITHOUT failing on a
// non-zero exit (verify-commit prints to stderr).
func (f *fix) gitEnvCmd(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func errFrom(out []byte, err error) error {
	return &cmdError{out: string(out), err: err}
}

type cmdError struct {
	out string
	err error
}

func (e *cmdError) Error() string { return e.err.Error() + ": " + e.out }

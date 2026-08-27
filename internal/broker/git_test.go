package broker

import (
	"encoding/base64"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The S13.6 broker-git conformance battery — THE BROKER IS THE GUARDRAIL
// (P-T12-1). Every git leg runs over file:// fixtures in t.TempDir(); zero
// network.

func gitEnv() []string {
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_ALLOW_PROTOCOL=file", "HOME=/nonexistent",
		"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@x")
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// pushFixture builds a bare remote (main at commit E), a source clone with a
// new commit C on top, and returns their paths plus E and C.
func pushFixture(t *testing.T) (remote, source, remoteURL, eSHA, cSHA string) {
	t.Helper()
	remote = filepath.Join(t.TempDir(), "remote.git")
	git(t, "", "init", "--bare", "-b", "main", remote)
	source = filepath.Join(t.TempDir(), "source")
	git(t, "", "init", "-b", "main", source)
	os.WriteFile(filepath.Join(source, "f.txt"), []byte("v1\n"), 0o600)
	git(t, source, "add", "-A")
	git(t, source, "commit", "-q", "-m", "e")
	remoteURL = "file://" + remote
	git(t, source, "push", remoteURL, "HEAD:refs/heads/main")
	eSHA = git(t, source, "rev-parse", "HEAD")
	os.WriteFile(filepath.Join(source, "f.txt"), []byte("v2\n"), 0o600)
	git(t, source, "add", "-A")
	git(t, source, "commit", "-q", "-m", "c")
	cSHA = git(t, source, "rev-parse", "HEAD")
	return
}

func remoteMain(t *testing.T, remote string) string {
	t.Helper()
	return git(t, "", "--git-dir="+remote, "rev-parse", "refs/heads/main")
}

// TestP_T12_1_ProtectedRefRefusal is the MANDATORY conformance entry (Spec
// S13.6/P-T12-1): a non-accept push to a protected ref MUST observe a
// broker-side refusal, and the remote ref MUST NOT move. An authorized push to
// the same ref then succeeds.
func TestP_T12_1_ProtectedRefRefusal(t *testing.T) {
	remote, source, remoteURL, eSHA, cSHA := pushFixture(t)
	srv := &Server{store: nil, log: discardLog()}

	base := Request{
		Op: OpPush, RepoDir: source, Remote: remoteURL,
		Refs:      []RefUpdate{{Ref: "refs/heads/main", ExpectSHA: eSHA, SrcSHA: cSHA}},
		Protected: []string{"refs/heads/main"},
	}

	// Unauthorized push to the protected ref: refused, remote unchanged.
	unauth := base
	unauth.Authorized = false
	if resp := srv.dispatch(unauth); resp.OK || resp.Rejected {
		t.Fatalf("protected-ref push without authorization was not refused: %+v", resp)
	}
	if got := remoteMain(t, remote); got != eSHA {
		t.Fatalf("SECURITY: the remote ref moved on a refused push: %s -> %s", eSHA, got)
	}

	// Authorized accept push: succeeds, remote advances to C.
	auth := base
	auth.Authorized = true
	if resp := srv.dispatch(auth); !resp.OK {
		t.Fatalf("authorized accept push failed: %+v", resp)
	}
	if got := remoteMain(t, remote); got != cSHA {
		t.Errorf("authorized push did not advance the remote: %s != %s", got, cSHA)
	}
}

// TestCASOnlyBareForceRefused: a push without an explicit --force-with-lease
// expect-sha is refused (the bare form is forbidden, P-T12-1) — even to an
// UNprotected ref, so the CAS-always rule holds everywhere.
func TestCASOnlyBareForceRefused(t *testing.T) {
	_, source, remoteURL, _, cSHA := pushFixture(t)
	srv := &Server{store: nil, log: discardLog()}
	resp := srv.dispatch(Request{
		Op: OpPush, RepoDir: source, Remote: remoteURL, Authorized: true,
		Refs: []RefUpdate{{Ref: "refs/heads/feature", ExpectSHA: "", SrcSHA: cSHA}},
	})
	if resp.OK {
		t.Fatal("a bare (value-less lease) push was not refused")
	}
	if !strings.Contains(resp.Error, "expect-sha") {
		t.Errorf("refusal reason %q does not name the missing expect-sha", resp.Error)
	}
}

// TestPushWithNoRemoteIsRefused pins the guardrail P3-GF11 leaves standing
// (brief R3). A push request carrying no remote is MALFORMED — every word of
// S13.6 step 4 is defined over a remote and a credential — and the broker
// refuses it before it touches git, with nothing moving anywhere.
//
// The refusal is deliberately NOT relaxed into a silent success. GF11 stops the
// accept from composing such a request for a project the S13.7 registry records
// no remote for, and that decision lives in the accept, which can read the
// registry. Teaching the broker to no-op instead would mask a genuinely missing
// remote on a store that should have one, and would put registry knowledge into
// a component that deliberately imports no DB (P-T12-1; CONVENTIONS §29). No
// landed test covered this limb, so a refactor could have quietly deleted the
// honest-absence answer the accept now depends on.
func TestPushWithNoRemoteIsRefused(t *testing.T) {
	remote, source, _, eSHA, cSHA := pushFixture(t)
	srv := &Server{store: nil, log: discardLog()}
	resp := srv.dispatch(Request{
		Op: OpPush, RepoDir: source, Remote: "", Authorized: true,
		Refs:      []RefUpdate{{Ref: "refs/heads/main", ExpectSHA: eSHA, SrcSHA: cSHA}},
		Protected: []string{"refs/heads/main"},
	})
	if resp.OK {
		t.Fatalf("a push request with no remote was not refused: %+v", resp)
	}
	// A refusal, not a lease rejection: there is nothing to have a stale lease
	// AGAINST, and the two answers route to different places in the accept.
	if resp.Rejected {
		t.Errorf("the missing remote was reported as a lease rejection: %+v", resp)
	}
	if !strings.Contains(resp.Error, "no remote") {
		t.Errorf("refusal reason %q does not name the missing remote", resp.Error)
	}
	if got := remoteMain(t, remote); got != eSHA {
		t.Errorf("SECURITY: a ref moved on a refused push: %s -> %s", eSHA, got)
	}
}

// TestLeaseRejectionRoutesToMergeCard: a stale lease is a NORMAL collision
// (Rejected=true), never a blind retry and never a generic error — the accept
// path routes it back to a merge card (S13.6 step 4).
func TestLeaseRejectionRoutesToMergeCard(t *testing.T) {
	remote, source, remoteURL, eSHA, cSHA := pushFixture(t)
	// Move the remote ref out from under the lease (a concurrent change).
	other := filepath.Join(t.TempDir(), "other")
	git(t, "", "clone", remoteURL, other)
	os.WriteFile(filepath.Join(other, "g.txt"), []byte("x\n"), 0o600)
	git(t, other, "add", "-A")
	git(t, other, "commit", "-q", "-m", "moved")
	git(t, other, "push", remoteURL, "HEAD:refs/heads/main")
	moved := remoteMain(t, remote)
	if moved == eSHA {
		t.Fatal("setup: remote did not move")
	}

	srv := &Server{store: nil, log: discardLog()}
	// Push with the STALE lease (eSHA), but the remote is now `moved`.
	resp := srv.dispatch(Request{
		Op: OpPush, RepoDir: source, Remote: remoteURL, Authorized: true,
		Refs:      []RefUpdate{{Ref: "refs/heads/main", ExpectSHA: eSHA, SrcSHA: cSHA}},
		Protected: []string{"refs/heads/main"},
	})
	if !resp.Rejected {
		t.Fatalf("a stale lease was not reported as a rejection (merge card): %+v", resp)
	}
	if resp.OK {
		t.Error("a rejected push reported OK")
	}
	if got := remoteMain(t, remote); got != moved {
		t.Errorf("SECURITY: a rejected push still moved the remote to %s", got)
	}
}

// TestBrokerSSHSignKeyNeverLeaves: the broker signs data with the git-ssh-key
// via gpg.format=ssh (ssh-keygen -Y sign) and returns only the SSHSIG — the
// private key never appears in the response — and the signature verifies
// (Spec S13.6 step 5).
func TestBrokerSSHSignKeyNeverLeaves(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "gitkey")
	run(t, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", keyPath, "-C", "op")
	priv, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		t.Fatal(err)
	}

	store, err := OpenStore(t.TempDir(), "op")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("git", KindGitSSHKey, string(priv)); err != nil {
		t.Fatal(err)
	}
	srv := &Server{store: store, log: discardLog()}

	resp := srv.dispatch(Request{Op: OpSignData, Profile: "git", Namespace: "git", Data: b64("accept: squash (Spec S13.6)")})
	if !resp.OK || resp.Sig == "" {
		t.Fatalf("sign-data failed: %+v", resp)
	}
	// The private key NEVER leaves the broker: it appears in no response field.
	if strings.Contains(resp.Sig, "PRIVATE KEY") || strings.Contains(decodeB64(t, resp.Sig), "PRIVATE KEY") {
		t.Fatal("SECURITY: the private key material appeared in the sign response")
	}
	// The returned SSHSIG verifies against the public key (the message rides
	// stdin; -f allowed-signers, -n namespace, -s signature).
	sig := decodeB64(t, resp.Sig)
	sigFile := filepath.Join(dir, "payload.sig")
	os.WriteFile(sigFile, []byte(sig), 0o600)
	allowed := filepath.Join(dir, "allowed")
	os.WriteFile(allowed, []byte("op namespaces=\"git\" "+string(pub)), 0o600)
	verify := exec.Command("ssh-keygen", "-Y", "verify", "-f", allowed, "-I", "op", "-n", "git", "-s", sigFile)
	verify.Env = gitEnv()
	verify.Stdin = strings.NewReader("accept: squash (Spec S13.6)")
	if out, err := verify.CombinedOutput(); err != nil {
		t.Fatalf("verify SSHSIG: %v\n%s", err, out)
	}
}

// TestPushKindConstraint: a push profile that is not a git-ssh-key is refused
// (the destination constraint, S11.5).
func TestPushKindConstraint(t *testing.T) {
	_, source, remoteURL, eSHA, cSHA := pushFixture(t)
	store, err := OpenStore(t.TempDir(), "op")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("engine", KindEngineCred, "token"); err != nil {
		t.Fatal(err)
	}
	srv := &Server{store: store, log: discardLog()}
	resp := srv.dispatch(Request{
		Op: OpPush, RepoDir: source, Remote: remoteURL, Authorized: true, Profile: "engine",
		Refs: []RefUpdate{{Ref: "refs/heads/main", ExpectSHA: eSHA, SrcSHA: cSHA}},
	})
	if resp.OK {
		t.Fatal("push with a non-git-ssh-key profile was not refused")
	}
}

// TestNoGitsignString: gitsign/Sigstore is NEVER used (renders Unverified on
// GitHub — worse than no signature). A source-scan conformance assertion (Spec
// S13.6 step 5; the §23 tripwire precedent).
func TestNoGitsignString(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(body)), "gitsign") {
			t.Errorf("%s references gitsign — NEVER used (Spec S13.6: renders Unverified on GitHub)", f)
		}
	}
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func decodeB64(t *testing.T, s string) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

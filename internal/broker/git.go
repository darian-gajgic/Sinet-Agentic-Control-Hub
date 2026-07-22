package broker

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// git.go is the broker's git verbs (Spec S13.6): the CAS push and the SSHSIG
// signing that run WITH the owner's git-ssh-key OUTSIDE any sandbox and return
// only the result (S11.5). The key NEVER leaves the broker — it is decrypted
// only into a 0600 temp file inside the broker's own 0700 store dir, used, and
// removed. Git is a subprocess (stdlib os/exec) — the broker imports no DB
// (CONVENTIONS §29; git ops carry no persistence).
//
// THE BROKER IS THE GUARDRAIL (P-T12-1). The push refuses, before touching any
// credential or spawning git:
//   - a bare / value-less lease — every ref carries --force-with-lease=<ref>:
//     <expect> with a non-empty expect (the bare form is trivially defeated by
//     background ref updates);
//   - a push to a protected ref (S13.7 registry policy, handed in as request
//     data) that does not carry the accept-authorization the control-plane
//     accept/snapshot path is the sole setter of ("main only via accepts").
// Neither the sandbox (which never receives the broker git UDS) nor any other
// control-plane path can push to a protected ref.

// gitBin is the host git CLI (the components.lock `git (host CLI)`
// os-mechanism entry; consumed UNMODIFIED as a subprocess).
const gitBin = "git"

// sshKeygenBin performs SSHSIG signing (gpg.format=ssh, the format git itself
// uses); the OS-packaged openssh binary, consumed as a subprocess.
const sshKeygenBin = "ssh-keygen"

// gitPush executes the S13.6 CAS push. It enforces the P-T12-1 invariants and
// classifies the outcome: success, a lease rejection (a collision → merge
// card, never a blind retry), a broker refusal, or a generic failure.
func (s *Server) gitPush(req Request) Response {
	if len(req.Refs) == 0 {
		return Response{OK: false, Error: "push with no refs"}
	}
	protected := make(map[string]bool, len(req.Protected))
	for _, r := range req.Protected {
		protected[r] = true
	}
	for _, u := range req.Refs {
		if u.Ref == "" || u.SrcSHA == "" {
			return Response{OK: false, Error: "push ref update missing ref or source"}
		}
		// CAS-always (P-T12-1): the bare --force-with-lease is forbidden.
		if u.ExpectSHA == "" {
			return Response{OK: false, Error: "refused: push without an explicit --force-with-lease expect-sha"}
		}
		// Protected-ref requires accept-authorization (P-T12-1: main only via
		// accepts). This is THE broker refusal.
		if protected[u.Ref] && !req.Authorized {
			s.log.Warn("broker: refused push to a protected ref without accept-authorization",
				"ref", u.Ref, "remote", req.Remote)
			return Response{OK: false, Error: "refused: push to a protected ref without an approved accept"}
		}
	}
	if req.Remote == "" {
		return Response{OK: false, Error: "push with no remote"}
	}

	env := hermeticGitEnv(req.Remote)
	// ssh:// transport uses the broker-held git-ssh-key; the key is written to
	// a 0600 temp file in the broker's own dir and removed after (never
	// returned, never in a sandbox). file:// dev pushes carry no key.
	if req.Profile != "" {
		kind, key, err := s.store.resolve(req.Profile)
		if err != nil {
			return errResp(err)
		}
		if kind != KindGitSSHKey {
			return errResp(fmt.Errorf("%w: push with a %s", ErrWrongKind, kind))
		}
		keyPath, cleanup, err := s.writeKeyFile(req.Profile, key)
		if err != nil {
			return Response{OK: false, Error: "broker error"}
		}
		defer cleanup()
		env = append(env,
			"GIT_SSH_COMMAND=ssh -i "+keyPath+
				" -o IdentitiesOnly=yes -o BatchMode=yes -o StrictHostKeyChecking=accept-new")
	}

	args := []string{"-C", req.RepoDir, "push"}
	if req.Atomic {
		args = append(args, "--atomic")
	}
	for _, u := range req.Refs {
		args = append(args, "--force-with-lease="+u.Ref+":"+u.ExpectSHA)
	}
	args = append(args, req.Remote)
	for _, u := range req.Refs {
		args = append(args, u.SrcSHA+":"+u.Ref)
	}

	cmd := exec.Command(gitBin, args...)
	cmd.Env = env
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if isLeaseRejection(stderr.String()) {
			// A definitive rejection from git (the ref moved since approval):
			// a normal collision, NOT a crash window. The control plane routes
			// it back to a merge card (S13.6 step 4).
			s.log.Info("broker: push rejected (stale lease) — routes to a merge card",
				"remote", req.Remote)
			return Response{OK: false, Rejected: true, Error: "push rejected: ref moved since approval"}
		}
		s.log.Warn("broker: push failed", "remote", req.Remote, "err", err, "stderr", strings.TrimSpace(stderr.String()))
		return Response{OK: false, Error: "push failed"}
	}
	return Response{OK: true}
}

// signData produces an SSHSIG over data with the broker-held git-ssh-key
// (Spec S13.6 step 5, gpg.format=ssh). The armored signature is returned; the
// key never leaves the broker. The platform embeds the signature as the
// commit's gpgsig header (the accept-commit signing path).
func (s *Server) signData(req Request) Response {
	kind, key, err := s.store.resolve(req.Profile)
	if err != nil {
		return errResp(err)
	}
	if kind != KindGitSSHKey {
		return errResp(fmt.Errorf("%w: sign-data with a %s", ErrWrongKind, kind))
	}
	data, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		return Response{OK: false, Error: "broker error"}
	}
	ns := req.Namespace
	if ns == "" {
		return Response{OK: false, Error: "sign-data without a namespace"}
	}
	keyPath, cleanup, err := s.writeKeyFile(req.Profile, key)
	if err != nil {
		return Response{OK: false, Error: "broker error"}
	}
	defer cleanup()
	payloadPath := keyPath + ".payload"
	if err := os.WriteFile(payloadPath, data, 0o600); err != nil {
		return Response{OK: false, Error: "broker error"}
	}
	defer os.Remove(payloadPath)
	defer os.Remove(payloadPath + ".sig")
	// ssh-keygen -Y sign writes <payload>.sig (the armored SSHSIG). The key
	// stays in the broker's dir; only the signature is read back.
	cmd := exec.Command(sshKeygenBin, "-Y", "sign", "-f", keyPath, "-n", ns, payloadPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		s.log.Warn("broker: ssh signing failed", "err", err, "stderr", strings.TrimSpace(stderr.String()))
		return Response{OK: false, Error: "signing failed"}
	}
	sig, err := os.ReadFile(payloadPath + ".sig")
	if err != nil {
		return Response{OK: false, Error: "broker error"}
	}
	return Response{OK: true, Sig: base64.StdEncoding.EncodeToString(sig)}
}

// writeKeyFile decrypts a git-ssh-key into a 0600 temp file in the broker's
// own 0700 store dir and returns a cleanup that scrubs and removes it. The
// plaintext key exists on disk only for the duration of the one git/ssh-keygen
// invocation and never leaves the broker's trust boundary (D2/S11.5).
func (s *Server) writeKeyFile(profile, key string) (path string, cleanup func(), err error) {
	path = filepath.Join(s.store.dir, "."+profile+".gitkey")
	if err := os.WriteFile(path, []byte(key), 0o600); err != nil {
		return "", nil, fmt.Errorf("broker: write key file: %w", err)
	}
	return path, func() {
		// Best-effort overwrite before unlink to shorten the plaintext window.
		if f, e := os.OpenFile(path, os.O_WRONLY, 0o600); e == nil {
			z := make([]byte, len(key))
			_, _ = f.Write(z)
			_ = f.Close()
		}
		_ = os.Remove(path)
	}, nil
}

// hermeticGitEnv is the broker's per-invocation git environment: no host/global
// config, no interactive prompts, and GIT_ALLOW_PROTOCOL restricted to exactly
// the remote's transport (file for dev fixtures, ssh for the live leg). The
// same hermetic posture as internal/project's git core (CONVENTIONS §23),
// re-established here because the broker holds the key and runs the push.
func hermeticGitEnv(remote string) []string {
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
	}
	if p := os.Getenv("PATH"); p != "" {
		env = append(env, "PATH="+p)
	}
	return env
}

// isLeaseRejection reports whether git's stderr is a definitive ref-rejection
// (a --force-with-lease lease that went stale, or any non-fast-forward
// rejection) rather than a transport/other failure. A rejection is a normal
// collision → merge card (S13.6 step 4); it is NEVER a crash window.
func isLeaseRejection(stderr string) bool {
	return strings.Contains(stderr, "[rejected]") ||
		strings.Contains(stderr, "stale info") ||
		strings.Contains(stderr, "! [remote rejected]")
}

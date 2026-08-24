// Package broker is the credential broker of Spec S11.5 — the ssh-agent-
// shaped, host-side daemon that keeps every secret OUTSIDE the task sandbox
// (D2: the sandbox is the boundary; no credential ever enters it). It is the
// internals of sinet-broker.service (S01.2 owns the unit; S11 the internals):
// a separate process so the large-attack-surface control plane never holds
// decrypted member credentials (S01.2). The broker authenticates the ASKER by
// UDS peer credentials (SO_PEERCRED) and never trusts what the asker claims;
// it is never modeled on git-credential helpers, which return the secret
// (the anti-model, S11.5).
//
// Two credential postures, kept distinct (S11.5):
//   - engine-cred  → the engine's OWN model-endpoint credential, DELIVERED to
//     the engine at spawn (S01.6 "engines receive credentials at start"). This
//     is the one kind that leaves the broker, into the engine process.
//   - signing-key  → an OWNER secret (git/publish/sign). The broker executes
//     WITH it and returns ONLY the result (an HMAC here); the key never leaves
//     — the ssh-agent posture, "authenticate the asker, never hand it the
//     secret" (S11.5). Outward effects route to the 4.2 gated-proposal flow.
//
// At-rest encryption (reading, section-cited — S11.5; CONVENTIONS
// new-convention rule): S11.5's ratified at-rest format is systemd-creds +
// sops/age. systemd-creds is host-integrated (a DEFERRED host change — no host
// changes at B1) and filippo.io/age v1.3.1 pulls a non-trivial module tree, so
// — like the other host-integrated changes that batch at the B2 gate (the srt
// install, the egress substrate) — sops/age are NOT consumed at B1 and
// materialize no lock entry.
// The dev-mode store is real per-person encryption at rest via stdlib
// AES-256-GCM under a per-broker master key in a 0700 host-only dir (S11.5:
// "0700, host-only, never in any sandbox"). Adopting age (+ deciding sops) is
// queued for the B1 gate, to land with the host-integrated broker key
// management (systemd-creds) and the operator age-identity escrow (item 6).
package broker

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Credential kinds — the S11.5 posture split and the destination constraint
// (a resolve may only touch an engine-cred; a sign only a signing-key).
const (
	KindEngineCred = "engine-cred" // delivered to the engine at spawn (injection)
	KindSigningKey = "signing-key" // owner secret; result-only (never handed out)
	// KindGitSSHKey is an owner's git SSH private key (S13.6). It is
	// result-only like a signing-key — the broker runs `git push` and
	// `ssh-keygen -Y sign` WITH it and returns only the result; the key NEVER
	// leaves the broker (D2/S11.5). Kept a distinct kind so the destination
	// constraint is exact: it is served ONLY to the push/sign-data ops, never
	// resolved (engine injection) or HMAC-signed.
	KindGitSSHKey = "git-ssh-key"
)

func validKind(kind string) bool {
	switch kind {
	case KindEngineCred, KindSigningKey, KindGitSSHKey:
		return true
	}
	return false
}

// Store errors.
var (
	ErrNoProfile   = errors.New("broker: no such auth-profile")
	ErrWrongKind   = errors.New("broker: operation not permitted for this credential kind")
	ErrBadProfile  = errors.New("broker: invalid auth-profile name")
	ErrCorruptData = errors.New("broker: stored credential failed authenticated decryption")
)

// Store is a per-person encrypted credential store. One Store instance serves
// one person (one uid); the broker runs one per-user UDS + Store pair.
type Store struct {
	dir    string // <root>/<user>, 0700
	user   string
	master []byte // 32-byte AES-256 key; from master.key (0600) in dir
}

// profileRecord is one stored credential on disk. Kind is plaintext (not a
// secret); the secret itself is AES-256-GCM ciphertext, audience-bound to
// user|profile|kind via the AAD (S11.5 audience binding) so a blob can never
// be decrypted under a different identity.
type profileRecord struct {
	Kind  string `json:"kind"`
	Nonce string `json:"nonce"` // base64
	CT    string `json:"ct"`    // base64 ciphertext (secret)
}

// OpenStore opens (creating if absent) the per-person store under root for
// user. It generates the per-broker master key on first use (0600 in the 0700
// store dir). The master key stands in for the systemd-creds-wrapped key of
// the host-integrated broker (deferred); the on-disk secrets are always
// encrypted at rest regardless (S11.5).
func OpenStore(root, user string) (*Store, error) {
	if err := validProfile(user); err != nil {
		return nil, fmt.Errorf("broker: invalid user %q: %w", user, err)
	}
	dir := filepath.Join(root, user)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("broker: store dir: %w", err)
	}
	// Enforce 0700 even if the dir pre-existed with looser bits (host-only).
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("broker: store dir perms: %w", err)
	}
	master, err := loadOrCreateMaster(dir)
	if err != nil {
		return nil, err
	}
	return &Store{dir: dir, user: user, master: master}, nil
}

func loadOrCreateMaster(dir string) ([]byte, error) {
	path := filepath.Join(dir, "master.key")
	b, err := os.ReadFile(path)
	if err == nil {
		if len(b) != 32 {
			return nil, fmt.Errorf("broker: master.key is %d bytes, want 32", len(b))
		}
		return b, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("broker: read master.key: %w", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("broker: generate master key: %w", err)
	}
	// O_EXCL: never clobber an existing key (a race would orphan ciphertext).
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return loadOrCreateMaster(dir)
		}
		return nil, fmt.Errorf("broker: create master.key: %w", err)
	}
	if _, err := f.Write(key); err != nil {
		f.Close()
		return nil, fmt.Errorf("broker: write master key: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("broker: close master key: %w", err)
	}
	return key, nil
}

// Put stores (or replaces) a credential under a profile name. An admin/setup
// operation: in production this is the operator migrating a key into the
// broker at a future gate (per the packet's credential-hygiene rule), never
// an agent-reachable path.
func (s *Store) Put(profile, kind, secret string) error {
	if err := validProfile(profile); err != nil {
		return err
	}
	if !validKind(kind) {
		return fmt.Errorf("broker: unknown credential kind %q", kind)
	}
	gcm, err := s.aead()
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("broker: nonce: %w", err)
	}
	ct := gcm.Seal(nil, nonce, []byte(secret), s.aad(profile, kind))
	rec := profileRecord{
		Kind:  kind,
		Nonce: base64.StdEncoding.EncodeToString(nonce),
		CT:    base64.StdEncoding.EncodeToString(ct),
	}
	blob, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("broker: marshal record: %w", err)
	}
	tmp := filepath.Join(s.dir, "."+profile+".tmp")
	final := s.recordPath(profile)
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return fmt.Errorf("broker: write record: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("broker: commit record: %w", err)
	}
	return nil
}

// resolve reads and authenticated-decrypts a profile, returning its kind and
// secret. The AAD binds the plaintext to user|profile|kind: a blob copied to a
// different profile name or kind fails to authenticate (S11.5 audience
// binding).
func (s *Store) resolve(profile string) (kind, secret string, err error) {
	if err := validProfile(profile); err != nil {
		return "", "", err
	}
	blob, err := os.ReadFile(s.recordPath(profile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("%w: %q", ErrNoProfile, profile)
		}
		return "", "", fmt.Errorf("broker: read record: %w", err)
	}
	var rec profileRecord
	if err := json.Unmarshal(blob, &rec); err != nil {
		return "", "", fmt.Errorf("broker: parse record: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(rec.Nonce)
	if err != nil {
		return "", "", fmt.Errorf("broker: decode nonce: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(rec.CT)
	if err != nil {
		return "", "", fmt.Errorf("broker: decode ciphertext: %w", err)
	}
	gcm, err := s.aead()
	if err != nil {
		return "", "", err
	}
	pt, err := gcm.Open(nil, nonce, ct, s.aad(profile, rec.Kind))
	if err != nil {
		return "", "", fmt.Errorf("%w: %q", ErrCorruptData, profile)
	}
	return rec.Kind, string(pt), nil
}

// kindOf reports a profile's credential kind WITHOUT decrypting the secret (the
// Kind is plaintext in the record) — a secret-free presence check the decision
// path uses to derive a user's signing posture (S13.6/F3). exists=false when
// the profile is absent.
func (s *Store) kindOf(profile string) (kind string, exists bool, err error) {
	if err := validProfile(profile); err != nil {
		return "", false, err
	}
	blob, err := os.ReadFile(s.recordPath(profile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("broker: read record: %w", err)
	}
	var rec profileRecord
	if err := json.Unmarshal(blob, &rec); err != nil {
		return "", false, fmt.Errorf("broker: parse record: %w", err)
	}
	return rec.Kind, true, nil
}

func (s *Store) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(s.master)
	if err != nil {
		return nil, fmt.Errorf("broker: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("broker: gcm: %w", err)
	}
	return gcm, nil
}

// aad binds a ciphertext to the identity that may open it (S11.5 audience
// binding): user, profile, and kind.
func (s *Store) aad(profile, kind string) []byte {
	return []byte("sinet-broker\x00" + s.user + "\x00" + profile + "\x00" + kind)
}

func (s *Store) recordPath(profile string) string {
	return filepath.Join(s.dir, profile+recordExt)
}

// validProfile rejects names that could escape the store directory or collide
// with the master key. Auth-profile names are references, not paths.
func validProfile(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty", ErrBadProfile)
	}
	if name == "master" || name == "master.key" {
		return fmt.Errorf("%w: reserved name %q", ErrBadProfile, name)
	}
	if strings.ContainsAny(name, "/\\\x00.") {
		return fmt.Errorf("%w: %q contains a path or dot character", ErrBadProfile, name)
	}
	return nil
}

// zero best-effort scrubs a secret slice once it is no longer needed. Go's GC
// makes this advisory, but it shortens the window the plaintext lives.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

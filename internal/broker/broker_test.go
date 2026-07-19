package broker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fixtureSecret = "sk-FIXTURE-do-not-log-1234567890"

func TestStoreRoundTripAndEncryptedAtRest(t *testing.T) {
	root := t.TempDir()
	s, err := OpenStore(root, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put("model", KindEngineCred, fixtureSecret); err != nil {
		t.Fatal(err)
	}
	kind, secret, err := s.resolve("model")
	if err != nil {
		t.Fatal(err)
	}
	if kind != KindEngineCred || secret != fixtureSecret {
		t.Fatalf("round-trip mismatch: kind=%q secret=%q", kind, secret)
	}

	// Encrypted at rest: the plaintext secret must NOT appear in the on-disk
	// record (S11.5 "encrypted at rest regardless").
	blob, err := os.ReadFile(filepath.Join(root, "alice", "model.cred"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, []byte(fixtureSecret)) {
		t.Fatal("SECURITY: plaintext secret found in the at-rest store file")
	}

	// The store dir is 0700 and the master key 0600, host-only (S11.5).
	if fi, err := os.Stat(filepath.Join(root, "alice")); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("store dir perms = %v, want 0700", fi.Mode().Perm())
	}
	if fi, err := os.Stat(filepath.Join(root, "alice", "master.key")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("master.key perms = %v, want 0600", fi.Mode().Perm())
	}
}

func TestStoreAudienceBinding(t *testing.T) {
	root := t.TempDir()
	s, err := OpenStore(root, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put("real", KindEngineCred, fixtureSecret); err != nil {
		t.Fatal(err)
	}
	// Copy the ciphertext blob to a different profile name: the AAD binds it to
	// user|profile|kind, so decryption under the new name must fail (S11.5
	// audience binding), never silently return the secret.
	blob, _ := os.ReadFile(filepath.Join(root, "bob", "real.cred"))
	if err := os.WriteFile(filepath.Join(root, "bob", "forged.cred"), blob, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.resolve("forged"); err == nil {
		t.Fatal("SECURITY: a blob copied to another profile name decrypted successfully")
	}
}

func TestPersistenceAcrossReopen(t *testing.T) {
	root := t.TempDir()
	s1, _ := OpenStore(root, "carol")
	if err := s1.Put("model", KindEngineCred, fixtureSecret); err != nil {
		t.Fatal(err)
	}
	s2, err := OpenStore(root, "carol") // reuses the same master.key
	if err != nil {
		t.Fatal(err)
	}
	if _, secret, err := s2.resolve("model"); err != nil || secret != fixtureSecret {
		t.Fatalf("reopen lost the secret: secret=%q err=%v", secret, err)
	}
}

// startServer launches a broker Server on a per-user UDS under t.TempDir and
// returns the socket path plus the log buffer (to assert secrets never log).
func startServer(t *testing.T, allowStore bool) (socket string, logs *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	store, err := OpenStore(root, "me")
	if err != nil {
		t.Fatal(err)
	}
	socket = filepath.Join(t.TempDir(), "broker.sock")
	ln, err := Listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	logs = &bytes.Buffer{}
	srv := NewServer(store, uint32(os.Getuid()), slog.New(slog.NewTextHandler(logs, nil)))
	srv.AllowStore = allowStore
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = srv.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	return socket, logs
}

// TestBrokerRoundTripInjectionAtSpawn is the packet's broker round-trip
// acceptance: store a fixture secret → the engine env receives it at spawn
// (via EnvInjector, the CredInject seam) → the secret never appears in the
// broker/client logs (and the broker touches no DB — it has none).
func TestBrokerRoundTripInjectionAtSpawn(t *testing.T) {
	socket, logs := startServer(t, true)

	// Store the fixture (admin op) over the attested socket.
	c, err := Dial(socket)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Store("model-endpoint", KindEngineCred, fixtureSecret); err != nil {
		t.Fatalf("store: %v", err)
	}
	c.Close()

	// Injection at spawn (S11.5; S01.6): the CredInject seam resolves fresh
	// and appends the credential to the engine env.
	inject := EnvInjector(socket, "model-endpoint", "SINET_MODEL_TOKEN")
	base := []string{"PATH=/usr/bin", "HOME=/x"}
	env, err := inject(base)
	if err != nil {
		t.Fatalf("inject: %v", err)
	}
	var got string
	for _, kv := range env {
		if strings.HasPrefix(kv, "SINET_MODEL_TOKEN=") {
			got = strings.TrimPrefix(kv, "SINET_MODEL_TOKEN=")
		}
	}
	if got != fixtureSecret {
		t.Fatalf("engine env did not receive the injected secret: %q", got)
	}
	// The base env must be preserved.
	if len(env) != len(base)+1 {
		t.Errorf("injected env has %d vars, want %d", len(env), len(base)+1)
	}

	// The secret must NEVER appear in logs (and the broker has no DB — nothing
	// is persisted beyond the encrypted store).
	if strings.Contains(logs.String(), fixtureSecret) {
		t.Fatal("SECURITY: the fixture secret appeared in the broker log")
	}
}

func TestResolveAndSignKindConstraints(t *testing.T) {
	socket, _ := startServer(t, true)
	c, err := Dial(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.Store("model", KindEngineCred, fixtureSecret); err != nil {
		t.Fatal(err)
	}
	if err := c.Store("git-key", KindSigningKey, "signing-material-xyz"); err != nil {
		t.Fatal(err)
	}

	// resolve serves only engine-cred; a signing-key must never leave.
	if _, _, err := c.Resolve("git-key"); err == nil {
		t.Fatal("SECURITY: resolve returned a signing-key (must be sign-only)")
	}
	// sign serves only signing-key.
	if _, err := c.Sign("model", []byte("data")); err == nil {
		t.Fatal("sign accepted an engine-cred profile (kind constraint)")
	}
	// engine-cred resolves fine.
	if secret, _, err := c.Resolve("model"); err != nil || secret != fixtureSecret {
		t.Fatalf("resolve engine-cred: secret=%q err=%v", secret, err)
	}
}

func TestSignReturnsResultNotKey(t *testing.T) {
	socket, logs := startServer(t, true)
	c, err := Dial(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	const key = "owner-signing-key-abcdefteh"
	if err := c.Store("git-sign", KindSigningKey, key); err != nil {
		t.Fatal(err)
	}
	data := []byte("commit-payload")
	sig, err := c.Sign("git-sign", data)
	if err != nil {
		t.Fatal(err)
	}
	// The broker returns only the result (S11.5 ssh-agent posture): verify the
	// HMAC, and confirm the key never left (never on the wire result, never
	// logged).
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(data)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		t.Fatal("sign returned an incorrect HMAC")
	}
	if strings.Contains(logs.String(), key) {
		t.Fatal("SECURITY: signing key appeared in the broker log")
	}
}

// TestPeerCredAttestation proves the SO_PEERCRED check: a Server whose
// ownerUID is not ours refuses our connection (authenticate the asker, S11.5).
func TestPeerCredAttestation(t *testing.T) {
	root := t.TempDir()
	store, _ := OpenStore(root, "me")
	socket := filepath.Join(t.TempDir(), "b.sock")
	ln, err := Listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	// ownerUID deliberately not our uid: the peer-cred check must reject us.
	srv := NewServer(store, uint32(os.Getuid())+1, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	srv.AllowStore = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx, ln)

	c, err := Dial(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Store("x", KindEngineCred, "y"); err == nil {
		t.Fatal("SECURITY: a non-owner peer was served")
	}
}

func TestStoreOpGatedByAllowStore(t *testing.T) {
	socket, _ := startServer(t, false) // AllowStore = false
	c, err := Dial(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Store("x", KindEngineCred, "y"); err == nil {
		t.Fatal("store op served despite AllowStore=false")
	}
}

func TestInvalidProfileNamesRejected(t *testing.T) {
	root := t.TempDir()
	s, _ := OpenStore(root, "me")
	for _, bad := range []string{"", "master", "../escape", "a/b", "dot.name", "nul\x00"} {
		if err := s.Put(bad, KindEngineCred, "x"); err == nil {
			t.Errorf("profile name %q accepted (must be rejected)", bad)
		}
	}
}

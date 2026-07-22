package backup_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/backup"
	"github.com/klauspost/compress/zstd"
)

// TestSecretsExcludedFromPayload: the pipeline reads platform.db + file stores
// ONLY, never the broker credential store — so a broker secret can never appear
// in the (decrypted) snapshot payload (S13.10; G2 D2.5).
func TestSecretsExcludedFromPayload(t *testing.T) {
	const secret = "BROKER-SECRET-MUST-NOT-LEAK-a1b2c3"
	// A file store the snapshot DOES back up (worker templates / knowledge).
	store := filepath.Join(t.TempDir(), "worker-templates")
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "w.md"), []byte("public worker template"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A broker-store-shaped file sitting on the host with a real secret — the
	// pipeline reads platform.db + FileStores ONLY (never the broker store), so
	// it can never enter the payload (Config has no broker-store field).
	brokerish := filepath.Join(t.TempDir(), "broker-store")
	if err := os.MkdirAll(brokerish, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(brokerish, "op.cred"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarnessWith(t, func(c *backup.Config) {
		c.FileStores = []backup.NamedDir{{Name: "worker-templates", Dir: store}}
	})

	row, err := h.snap.Snapshot(h.ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	archive := h.decryptNewest(t, row.BlobName)
	if bytes.Contains(archive, []byte(secret)) {
		t.Fatal("SECURITY: a broker secret appeared in the snapshot payload")
	}
	if !bytes.Contains(archive, []byte("public worker template")) {
		t.Error("the file-store export is missing from the archive")
	}
	if !bytes.Contains(archive, []byte("platform.dump.sql")) {
		t.Error("the platform.db dump is missing from the archive")
	}
}

// TestLedgerRowInSubsequentArchive (R19): archive N+1 carries archive N's
// ledger row — the durable-set membership that makes swap-detection chain.
func TestLedgerRowInSubsequentArchive(t *testing.T) {
	h := newHarness(t)
	row1, err := h.snap.Snapshot(h.ctx)
	if err != nil {
		t.Fatalf("Snapshot 1: %v", err)
	}
	h.advance(3600)
	row2, err := h.snap.Snapshot(h.ctx)
	if err != nil {
		t.Fatalf("Snapshot 2: %v", err)
	}
	// Snapshot 2's dump must contain snapshot 1's ledger row (its sha256).
	archive2 := h.decryptNewest(t, row2.BlobName)
	if !bytes.Contains(archive2, []byte(row1.SHA256)) {
		t.Errorf("snapshot 2's archive does not carry snapshot 1's ledger sha %s", row1.SHA256[:12])
	}
}

// TestIdentityAndRecipientFromFilePath: LoadCrypto reads an age identity FILE
// (a temp keypair in t.TempDir — never the real escrow file) and derives its
// recipient at runtime; the derived pair round-trips. No key material is
// hardcoded (R20/R25).
func TestIdentityAndRecipientFromFilePath(t *testing.T) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "age-identity.txt")
	if err := os.WriteFile(path, []byte("# created: test\n# public key: "+id.Recipient().String()+"\n"+id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := backup.LoadCrypto(path)
	if err != nil {
		t.Fatalf("LoadCrypto: %v", err)
	}
	if len(c.Recipients) != 1 || c.Identity == nil {
		t.Fatalf("LoadCrypto derived %d recipients / identity=%v", len(c.Recipients), c.Identity != nil)
	}
	// The recipient was DERIVED from the identity (identity -> recipient).
	if c.Recipients[0].(*age.X25519Recipient).String() != id.Recipient().String() {
		t.Error("derived recipient does not match the identity's own recipient")
	}
	// Round-trip: encrypt to the derived recipient, decrypt with the identity.
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, c.Recipients...)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(w, "escrow round-trip")
	w.Close()
	r, err := age.Decrypt(bytes.NewReader(buf.Bytes()), c.Identity)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	if string(got) != "escrow round-trip" {
		t.Errorf("round-trip got %q", got)
	}
}

// TestChunkedBlobRoundTrip: forcing a tiny chunk threshold splits the blob into
// multiple part files; the drill reassembles and verifies them (the ~90 MB
// blob-discipline path, R19).
func TestChunkedBlobRoundTrip(t *testing.T) {
	h := newHarnessWith(t, func(c *backup.Config) { c.ChunkThresholdBytes = 64 })
	row, err := h.snap.Snapshot(h.ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	// The remote tree holds multiple chunk files for this snapshot.
	work := filepath.Join(t.TempDir(), "inspect")
	runGit(t, "", "clone", "file://"+h.remote, work)
	entries, _ := os.ReadDir(work)
	var parts int
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), row.BlobName+".") {
			parts++
		}
	}
	if parts < 2 {
		t.Fatalf("expected the blob chunked into >=2 parts, got %d", parts)
	}
	h.advance(3600)
	res, err := h.snap.RestoreDrill(h.ctx)
	if err != nil {
		t.Fatalf("drill on chunked blob: %v", err)
	}
	if !res.OK {
		t.Errorf("drill failed on a chunked blob: %+v", res)
	}
}

// decryptNewest fetches and decrypts a snapshot blob from the remote with the
// harness identity, returning the plaintext archive (zstd|tar) for inspection.
func (h *harness) decryptNewest(t *testing.T, base string) []byte {
	t.Helper()
	work := filepath.Join(t.TempDir(), "decrypt-"+base)
	runGit(t, "", "clone", "file://"+h.remote, work)
	entries, _ := os.ReadDir(work)
	var buf bytes.Buffer
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), base+".") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		t.Fatalf("no chunks for %s in remote", base)
	}
	sortStrings(names)
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(work, n))
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(b)
	}
	r, err := age.Decrypt(bytes.NewReader(buf.Bytes()), h.id)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	zstdTar, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	// Peel zstd so callers can grep the tar (dump + file stores) as plaintext.
	zr, err := zstd.NewReader(bytes.NewReader(zstdTar))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return plain
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

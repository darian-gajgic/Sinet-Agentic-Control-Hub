package backup

import (
	"fmt"
	"io"
	"os"

	"filippo.io/age"
)

// Snapshot-lane crypto (Spec S13.10; the adopted age Go module,
// components.lock `age (snapshot encryption)`). Client-side encryption is
// UNCONDITIONAL: nothing leaves the host unencrypted (a private repo is
// access-controlled, not end-to-end encrypted, and the payload includes every
// member's memories and receipts). age has NO sender authentication —
// encryption proves who can READ, never who wrote; integrity is the snapshot
// ledger's job (P-T12-4), NOT age's.

// Crypto is the resolved snapshot-lane key material: the recipients a snapshot
// encrypts TO, and the identity the restore drill decrypts WITH. Both derive
// from the operator's age identity file — the recipient is DERIVED from the
// identity (identity -> recipient), never pasted anywhere (operator directive,
// R20). The identity IS the escrow material (the drill proves recovery with
// it, not merely a live-session key).
type Crypto struct {
	Recipients []age.Recipient
	Identity   age.Identity
}

// DefaultIdentityPath is the escrow identity file placed by the operator
// escrow ceremony (0600). The pipeline reads identity + recipient from this
// path at LIVE bring-up ONLY; its contents are NEVER read by tests, which
// inject a fresh temp keypair (R20/R23). No key material is hardcoded here —
// this is a path, not a key.
const DefaultIdentityPath = "~/.config/sinet/age-identity.txt"

// LoadCrypto reads the age identity file at path and derives its recipient(s)
// — the runtime identity->recipient derivation (R20). Called only at LIVE
// bring-up (the one-shot snapshot/drill CLI modes resolve the configured
// path). A parse failure is loud: the snapshot lane never silently proceeds
// without encryption (client-side encryption is unconditional, S13.10).
func LoadCrypto(path string) (Crypto, error) {
	f, err := os.Open(path)
	if err != nil {
		return Crypto{}, fmt.Errorf("backup: open age identity %q: %w", path, err)
	}
	defer f.Close()
	return parseCrypto(f)
}

func parseCrypto(r io.Reader) (Crypto, error) {
	ids, err := age.ParseIdentities(r)
	if err != nil {
		return Crypto{}, fmt.Errorf("backup: parse age identities: %w", err)
	}
	if len(ids) == 0 {
		return Crypto{}, fmt.Errorf("backup: age identity file has no identities")
	}
	c := Crypto{}
	for _, id := range ids {
		c.Identity = id // the drill decrypts with any of them; keep the last
		x, ok := id.(*age.X25519Identity)
		if !ok {
			return Crypto{}, fmt.Errorf("backup: unsupported age identity type %T (X25519 expected)", id)
		}
		c.Recipients = append(c.Recipients, x.Recipient())
	}
	return c, nil
}

// encryptTo wraps dst so writes are age-encrypted to the recipients (S13.10:
// tar | zstd | age -r <recipients>). The caller MUST Close the returned writer
// to flush age's trailer.
func encryptTo(dst io.Writer, recipients []age.Recipient) (io.WriteCloser, error) {
	if len(recipients) == 0 {
		return nil, fmt.Errorf("backup: refusing to encrypt to zero recipients (client-side encryption is unconditional, S13.10)")
	}
	return age.Encrypt(dst, recipients...)
}

// decryptWith wraps src so reads are age-decrypted with the escrow identity
// (the restore drill, R22). A wrong identity fails loudly — the drill's decrypt
// step proves the recovery material.
func decryptWith(src io.Reader, id age.Identity) (io.Reader, error) {
	if id == nil {
		return nil, fmt.Errorf("backup: no escrow identity to decrypt with")
	}
	return age.Decrypt(src, id)
}

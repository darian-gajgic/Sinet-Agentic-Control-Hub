package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters for newly written PIN hashes. Spec S01.9 ratifies the
// algorithm ("per-user PIN/password (argon2id)") but no parameters; these
// are the RFC 9106 §4 second recommended option (t=3, 64 MiB, p=4) as code
// constants — Spec S18 ratifies no auth ⚙ keys, so none are registry
// settings. Verification reads its parameters from the stored PHC string,
// so a future parameter move never invalidates stored credentials.
const (
	argonTime      = 3
	argonMemoryKiB = 64 * 1024
	argonThreads   = 4
	argonKeyLen    = 32
	argonSaltLen   = 16
)

// Decode-side parameter ceilings: a stored hash demanding more than this is
// treated as malformed rather than obeyed (bounds the work a corrupted row
// can request from the verifier).
const (
	maxArgonMemoryKiB = 1 << 20 // 1 GiB
	maxArgonTime      = 64
	maxArgonThreads   = 16
)

// ErrBadPINHash reports a stored credential this code cannot interpret —
// a platform defect or row corruption, never a user error.
var ErrBadPINHash = errors.New("auth: malformed stored PIN hash")

// b64 is the PHC-string base64 flavor: standard alphabet, no padding.
var b64 = base64.RawStdEncoding

// hashPIN derives a fresh argon2id credential in PHC string form
// ($argon2id$v=19$m=…,t=…,p=…$salt$key).
func hashPIN(pin string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate salt: %w", err)
	}
	return encodePHC(pin, salt, argonTime, argonMemoryKiB, argonThreads), nil
}

// encodePHC derives the argon2id key for pin under the given parameters and
// encodes the PHC string. Split from hashPIN so tests can pin a golden
// vector with a fixed salt.
func encodePHC(pin string, salt []byte, time, memoryKiB uint32, threads uint8) string {
	key := argon2.IDKey([]byte(pin), salt, time, memoryKiB, threads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memoryKiB, time, threads,
		b64.EncodeToString(salt), b64.EncodeToString(key))
}

// verifyPINHash checks pin against a stored PHC string, reading the
// parameters from the string itself. It is deliberately expensive (argon2id)
// and must be called outside any write transaction — the pool is a single
// connection and a hash inside a tx would stall every writer (Spec S02.1).
func verifyPINHash(pin, phc string) (bool, error) {
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, ErrBadPINHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, ErrBadPINHash
	}
	var (
		memoryKiB, time uint32
		threads         uint8
	)
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memoryKiB, &time, &threads); err != nil {
		return false, ErrBadPINHash
	}
	if memoryKiB == 0 || memoryKiB > maxArgonMemoryKiB ||
		time == 0 || time > maxArgonTime ||
		threads == 0 || threads > maxArgonThreads {
		return false, ErrBadPINHash
	}
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false, ErrBadPINHash
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, ErrBadPINHash
	}
	got := argon2.IDKey([]byte(pin), salt, time, memoryKiB, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

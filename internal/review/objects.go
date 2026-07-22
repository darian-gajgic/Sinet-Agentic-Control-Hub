package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// The content-addressed local object dir (Spec S13.2, G2 Def.14): revision
// bytes live at <root>/objects/<aa>/<sha256>, keyed by their own hash, so
// a minted revision's content is retained platform-owned regardless of
// workspace GC (Spec S13.1: GC MUST NOT delete the only copy of a minted
// revision's objects). Object bytes are host-local review material OUTSIDE
// the 11.3 snapshot payload; the committed hash refs (the revision row)
// make loss detectable and regeneration targeted.

// putObject stores bytes content-addressed and returns their hash.
// Writing is idempotent: an existing object is verified by size, never
// rewritten (content addressing makes collisions = corruption, surfaced
// loudly at read time by the hash check).
func (s *Store) putObject(data []byte) (string, error) {
	sum := sha256.Sum256(data)
	h := hex.EncodeToString(sum[:])
	dir := filepath.Join(s.Root, "objects", h[:2])
	path := filepath.Join(dir, h)
	if _, err := os.Stat(path); err == nil {
		return h, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("review: object dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", fmt.Errorf("review: object write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("review: object write: %w", err)
	}
	return h, nil
}

// ObjectPath returns the on-disk path of a content-addressed object.
func (s *Store) ObjectPath(sha string) string {
	return filepath.Join(s.Root, "objects", sha[:2], sha)
}

// readObject loads an object and re-verifies its hash — content is never
// trusted from disk without the pin check (the B2-5 hash-verify posture).
func (s *Store) readObject(sha string) ([]byte, error) {
	if len(sha) < 3 {
		return nil, fmt.Errorf("%w: object ref %q", ErrBadInput, sha)
	}
	data, err := os.ReadFile(s.ObjectPath(sha))
	if err != nil {
		return nil, fmt.Errorf("review: object %s: %w", sha, err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != sha {
		return nil, fmt.Errorf("%w: object %s bytes hash differently", ErrContentDrift, sha)
	}
	return data, nil
}

// hashBytes returns the hex sha256 of data.
func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

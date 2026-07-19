// Package gates is the gates module seam of Spec S01.1: the D7
// checkpoint-and-gate machinery at the storage layer — checkpoint-per-paid-
// call (Spec S02.4) and the two-phase effect journal that makes outward
// effects exactly-once (Spec S02.7).
//
// The journal copies the gh-aw safe-outputs shape (Spec S16.5 Layer C):
// engines emit structured action REQUESTS — never direct outward calls —
// and a separate gate decides execution. The effects gate is not a quality
// gate; it is the boundary where "approved" and "executed" are different
// recorded facts (4.2), with dedup at the effect boundary making the
// at-least-once provider reality invisible (DBOS guarantee vocabulary,
// Spec S02.7).
//
// Ask/approval UI, per-provider execution, and the idempotency registry
// (data, not code) arrive with the adapters and operator surfaces (B1+,
// Spec S03/S15); the durable journal states and their discipline live here.
package gates

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// canonicalJSON returns the platform-canonical form of a JSON payload:
// decoded with number literals preserved, re-encoded compact with object
// keys sorted (encoding/json map ordering). Proposal and verification both
// normalize through this one function, so `payload_hash` comparisons are
// internally consistent (Spec S02.7 "payload normalized + hashed");
// cross-system canonicality is not a goal.
func canonicalJSON(raw json.RawMessage) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("gates: payload is not valid JSON: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("gates: payload has trailing data")
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("gates: canonicalize payload: %w", err)
	}
	return out, nil
}

// hashPayload is the normalized payload hash pinned at approval and
// re-verified before execution (Spec S02.7).
func hashPayload(canonical []byte) string {
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// newUUID returns a random RFC 4122 version-4 UUID. The effect UUID doubles
// as the idempotency key (Spec S02.7), so it comes from crypto/rand.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("gates: uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

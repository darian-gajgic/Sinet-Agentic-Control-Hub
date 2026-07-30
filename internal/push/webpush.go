package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"strconv"
	"time"
)

// webpush.go is the sender half of Web Push, implemented on the Go standard
// library and nothing else: RFC 8291 "Message Encryption for Web Push"
// (aes128gcm) and RFC 8292 "Voluntary Application Server Identification"
// (VAPID). Both are consumed by send.go; nothing else in the platform speaks
// them.
//
// WHY OWNED RATHER THAN ADOPTED (B6-9 OQ1(a), ratified). The whole path is
// stdlib at this repo's Go 1.26.5 — crypto/ecdh parses the subscription's
// p256dh directly, crypto/hkdf has been stdlib since Go 1.24, crypto/aes +
// crypto/cipher do the GCM, crypto/ecdsa signs the JWT — so adopting a library
// would buy an S16.4 rail walk and a funeral plan for code the standard library
// already provides. The counter-argument is crypto-implementation risk, and it
// is answered by the strongest check available anywhere in this repo: RFC 8291
// Appendix A publishes a COMPLETE worked example with every intermediate value,
// so webpush_test.go pins ecdh_secret → PRK_key → IKM → PRK → CEK → NONCE →
// header → ciphertext against the standard's own numbers, and then decrypts the
// result back as a receiver would. Crypto that only passes against its own
// output proves nothing; this passes against the RFC's.
//
// THE ONE CLASSIC FAILURE THIS FILE EXISTS TO AVOID is in vapidAuthorization:
// JWS ES256 is a raw 64-octet R‖S pair, and ecdsa.SignASN1 produces a DER
// SEQUENCE. The DER form is a valid signature of the same message, so it looks
// fine in a local round trip and every push service on earth rejects it. The
// signature here is assembled with big.Int.FillBytes into two fixed 32-octet
// halves, and the test refuses an ASN.1 signature through the same verifier the
// header is built with.

// The RFC 8291 §3.4 / RFC 8188 §2.1 derivation labels, verbatim. Each carries
// its terminating 0x00 exactly as the standard writes it.
const (
	keyInfoPrefix = "WebPush: info\x00"
	cekInfo       = "Content-Encoding: aes128gcm\x00"
	nonceInfo     = "Content-Encoding: nonce\x00"
)

// The aes128gcm record framing (RFC 8188 §2).
const (
	// saltLen is the 16-octet per-message salt in the content-coding header.
	saltLen = 16
	// recordSize is the `rs` field of the header. 4096 is the value RFC 8291's
	// own Appendix A example carries, and every push service accepts it; the
	// platform emits ONE record per message, so rs only has to exceed the
	// record, which the 4 KB payload ceiling already guarantees.
	recordSize = 4096
	// paddingDelimiter marks the LAST record of a message (RFC 8188 §2); the
	// platform sends exactly one, so it is always this value and never 0x01.
	paddingDelimiter = 0x02
	// uncompressedPointLen is a P-256 public key in the 65-octet uncompressed
	// form both the subscription's p256dh and the header's keyid use.
	uncompressedPointLen = 65
	// authSecretLen is the subscription's 16-octet auth secret (RFC 8291 §3.2).
	authSecretLen = 16
	// MaxPayloadBytes is the ceiling Apple's push service documents for the
	// encrypted body ("The payload size is over the limit of 4 KB",
	// developer.apple.com "Sending web push notifications in web apps and
	// browsers", re-verified 2026-07-30). It is a wire fact of the services this
	// platform talks to, not a tunable, so it is a constant here and a refusal
	// rather than a truncation at the call site — a silently shortened
	// ciphertext would decrypt to nothing on the device.
	MaxPayloadBytes = 4096
)

// b64 is the unpadded base64url alphabet every Web Push field uses: the
// subscription keys, the VAPID JWT and its public key, and the Topic header.
var b64 = base64.RawURLEncoding

// ErrBadSubscription marks subscription key material this platform cannot use.
// It is a permanent condition of the stored row, not a transport failure, so
// the caller treats it as a dead subscription rather than retrying it.
var ErrBadSubscription = errors.New("push: unusable subscription key material")

// Keys is one subscription's RFC 8291 key material as the browser serves it.
type Keys struct {
	// P256DH is the base64url 65-octet uncompressed P-256 public key.
	P256DH string
	// Auth is the base64url 16-octet authentication secret.
	Auth string
}

// decode parses the two base64url fields and validates their lengths. Both are
// boundary checks on material that arrived from a browser, which is the one
// place this package validates anything.
func (k Keys) decode() (uaPublic, authSecret []byte, err error) {
	uaPublic, err = b64.DecodeString(k.P256DH)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: p256dh is not base64url: %w", ErrBadSubscription, err)
	}
	if len(uaPublic) != uncompressedPointLen {
		return nil, nil, fmt.Errorf("%w: p256dh is %d octets, want %d", ErrBadSubscription, len(uaPublic), uncompressedPointLen)
	}
	authSecret, err = b64.DecodeString(k.Auth)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: auth is not base64url: %w", ErrBadSubscription, err)
	}
	if len(authSecret) != authSecretLen {
		return nil, nil, fmt.Errorf("%w: auth is %d octets, want %d", ErrBadSubscription, len(authSecret), authSecretLen)
	}
	return uaPublic, authSecret, nil
}

// Encrypt seals one plaintext to a subscription as a complete aes128gcm body
// (RFC 8291): the 86-octet content-coding header followed by a single record.
//
// The ephemeral key pair and the salt are fresh per message, which is what the
// scheme rests on — reusing either against the same subscription would reuse a
// (key, nonce) pair. `rnd` is nil in production (crypto/rand); the test passes
// the RFC's own values so the derivation can be pinned to the published vector.
func Encrypt(k Keys, plaintext []byte, rnd io.Reader) ([]byte, error) {
	if rnd == nil {
		rnd = rand.Reader
	}
	asPriv, err := ecdh.P256().GenerateKey(rnd)
	if err != nil {
		return nil, fmt.Errorf("push: ephemeral key: %w", err)
	}
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rnd, salt); err != nil {
		return nil, fmt.Errorf("push: salt: %w", err)
	}
	return encryptWith(k, plaintext, asPriv, salt)
}

// encryptWith is Encrypt with the two per-message randoms supplied. It is the
// whole of RFC 8291 §3.3–§3.4 and RFC 8188 §2, in the standard's own order, and
// it is what the Appendix A vector drives.
func encryptWith(k Keys, plaintext []byte, asPriv *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	uaPublic, authSecret, err := k.decode()
	if err != nil {
		return nil, err
	}
	if len(salt) != saltLen {
		return nil, fmt.Errorf("push: salt is %d octets, want %d", len(salt), saltLen)
	}
	uaKey, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		return nil, fmt.Errorf("%w: p256dh is not a P-256 point: %w", ErrBadSubscription, err)
	}
	asPublic := asPriv.PublicKey().Bytes()

	cek, nonce, err := deriveKeys(asPriv, uaKey, uaPublic, asPublic, authSecret, salt)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, fmt.Errorf("push: aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("push: gcm: %w", err)
	}
	// One record: the plaintext, the last-record delimiter, and no padding.
	record := make([]byte, 0, len(plaintext)+1)
	record = append(record, plaintext...)
	record = append(record, paddingDelimiter)

	out := header(salt, asPublic)
	return gcm.Seal(out, nonce, record, nil), nil
}

// deriveKeys is the RFC 8291 §3.4 two-stage HKDF: the auth secret mixes the
// ECDH output into an IKM bound to BOTH public keys, and the per-message salt
// then derives the content key and nonce from that IKM.
//
// The parameters are ROLE-NEUTRAL on purpose. `priv`/`peer` are whichever side
// is deriving; `uaPublic`/`asPublic` are the two keys in the order key_info
// fixes them, which is the same order for both sides. That is what lets the
// round-trip test derive as a receiver through this exact function rather than
// through a second copy of it that could agree with a bug.
func deriveKeys(priv *ecdh.PrivateKey, peer *ecdh.PublicKey, uaPublic, asPublic, authSecret, salt []byte) (cek, nonce []byte, err error) {
	ecdhSecret, err := priv.ECDH(peer)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: ECDH with p256dh failed: %w", ErrBadSubscription, err)
	}
	// PRK_key = HMAC-SHA-256 (auth_secret, ecdh_secret)
	prkKey, err := hkdf.Extract(sha256.New, ecdhSecret, authSecret)
	if err != nil {
		return nil, nil, fmt.Errorf("push: hkdf extract (auth): %w", err)
	}
	// key_info = "WebPush: info" || 0x00 || ua_public || as_public
	keyInfo := make([]byte, 0, len(keyInfoPrefix)+len(uaPublic)+len(asPublic))
	keyInfo = append(keyInfo, keyInfoPrefix...)
	keyInfo = append(keyInfo, uaPublic...)
	keyInfo = append(keyInfo, asPublic...)
	ikm, err := hkdf.Expand(sha256.New, prkKey, string(keyInfo), 32)
	if err != nil {
		return nil, nil, fmt.Errorf("push: hkdf expand (ikm): %w", err)
	}
	// PRK = HMAC-SHA-256 (salt, IKM), then the RFC 8188 content-coding labels.
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, nil, fmt.Errorf("push: hkdf extract (salt): %w", err)
	}
	if cek, err = hkdf.Expand(sha256.New, prk, cekInfo, 16); err != nil {
		return nil, nil, fmt.Errorf("push: hkdf expand (cek): %w", err)
	}
	if nonce, err = hkdf.Expand(sha256.New, prk, nonceInfo, 12); err != nil {
		return nil, nil, fmt.Errorf("push: hkdf expand (nonce): %w", err)
	}
	return cek, nonce, nil
}

// header builds the RFC 8188 §2.1 content-coding header:
// salt(16) || rs(4, big-endian) || idlen(1) || keyid, where keyid is the
// sender's own ephemeral public key. 86 octets for a P-256 sender.
func header(salt, asPublic []byte) []byte {
	out := make([]byte, 0, saltLen+4+1+len(asPublic))
	out = append(out, salt...)
	out = binary.BigEndian.AppendUint32(out, recordSize)
	out = append(out, byte(len(asPublic)))
	return append(out, asPublic...)
}

// ── RFC 8292 VAPID ──────────────────────────────────────────────────────────

// vapidExpiry is how far ahead a VAPID JWT's `exp` is set.
//
// Apple's push service refuses a token whose expiry is "more than one day into
// the future" AND asks senders not to "refresh your JWT more frequently than
// once per hour" (developer.apple.com, re-verified 2026-07-30). Those are two
// different rules and half a day sits comfortably inside both: the token stays
// valid far longer than the re-sign floor, so honouring the floor costs
// nothing, and it is well under the one-day ceiling even if the host's clock is
// hours out. Structural, not ⚙ (S18 ratifies no push key — the §7 sseBatchSize
// precedent, interim under the standing settings-tab directive).
const vapidExpiry = 12 * time.Hour

// vapidRenewMargin is how close to expiry a cached token is replaced. It is
// what keeps the cache from ever handing out a token a push service would
// refuse for staleness, and — with vapidExpiry above it — it means one re-sign
// per (push service, ~11 hours), which is far inside Apple's once-an-hour
// floor.
const vapidRenewMargin = time.Hour

// vapidClaims is the RFC 8292 §2 claim set. `aud` is the ORIGIN of the push
// resource (never its path — the path is the subscription capability and has no
// business in a token), `exp` bounds the token, and `sub` is the contact the
// push service reaches a human operator on.
type vapidClaims struct {
	Aud string `json:"aud"`
	Exp int64  `json:"exp"`
	Sub string `json:"sub"`
}

// vapidJWT signs one RFC 8292 token. The signature is the raw 64-octet R‖S of
// JWS ES256 (RFC 7518 §3.4) — see the file header for why ecdsa.SignASN1 is the
// wrong shape here and why the error is invisible in a local test.
func vapidJWT(key *ecdsa.PrivateKey, aud, sub string, exp time.Time, rnd io.Reader) (string, error) {
	if rnd == nil {
		rnd = rand.Reader
	}
	// The JOSE header is a constant for this one algorithm, so it is written as
	// a literal rather than marshaled: there is nothing here to vary.
	head := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, err := json.Marshal(vapidClaims{Aud: aud, Exp: exp.Unix(), Sub: sub})
	if err != nil {
		return "", fmt.Errorf("push: vapid claims: %w", err)
	}
	signingInput := head + "." + b64.EncodeToString(claims)

	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rnd, key, digest[:])
	if err != nil {
		return "", fmt.Errorf("push: vapid sign: %w", err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signingInput + "." + b64.EncodeToString(sig), nil
}

// vapidAudience is the `aud` claim for one push resource: the ORIGIN of the
// endpoint and nothing else (RFC 8292 §2).
func vapidAudience(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("push: endpoint is not a URL: %w", err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("push: endpoint %q is not an https URL", endpoint)
	}
	return u.Scheme + "://" + u.Host, nil
}

// vapidPublicKey renders the signing key's public half as RFC 8292's `k`
// parameter: the base64url 65-octet uncompressed point. It is the same string
// the browser is given as `applicationServerKey` at subscribe time, and the two
// MUST match or the push service refuses the request — which is why there is
// one function producing it.
func vapidPublicKey(key *ecdsa.PrivateKey) (string, error) {
	pub, err := key.ECDH()
	if err != nil {
		return "", fmt.Errorf("push: vapid public key: %w", err)
	}
	return b64.EncodeToString(pub.PublicKey().Bytes()), nil
}

// authorizationHeader assembles the RFC 8292 §3 header value.
func authorizationHeader(jwt, publicKey string) string {
	return "vapid t=" + jwt + ", k=" + publicKey
}

// ttlHeader renders the RFC 8030 TTL. Apple refuses a request whose TTL header
// "is either missing or isn't a positive number" (re-verified 2026-07-30), so
// this is never omitted and never zero.
func ttlHeader(d time.Duration) string {
	secs := int64(d / time.Second)
	if secs < 1 {
		secs = 1
	}
	return strconv.FormatInt(secs, 10)
}

// topicHeader derives an RFC 8030 Topic from a card id: the push service
// coalesces messages sharing one, so a re-nag REPLACES the notification already
// on the device instead of stacking a second copy of the same decision.
//
// It is a hash rather than the id because a Topic is metadata the relay reads,
// and the id names one household decision. Apple bounds it at "a maximum of 32
// characters from the URL or filename-safe Base64 characters sets"
// (re-verified 2026-07-30), and 24 octets of sha256 encode to exactly 32.
func topicHeader(cardID string) string {
	sum := sha256.Sum256([]byte(cardID))
	return b64.EncodeToString(sum[:24])
}

// bigIntsFromSignature splits a raw JWS ES256 signature. It exists so the test
// can verify the header this file produces THROUGH the same split a push
// service performs, which is the only way an ASN.1 signature can be shown to
// fail rather than merely be a different length.
func bigIntsFromSignature(sig []byte) (r, s *big.Int, ok bool) {
	if len(sig) != 64 {
		return nil, nil, false
	}
	return new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:]), true
}

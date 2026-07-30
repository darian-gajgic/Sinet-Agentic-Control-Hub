package push

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// The RFC 8291 Appendix A worked example, verbatim (re-verified against
// https://www.rfc-editor.org/rfc/rfc8291.txt on 2026-07-30).
//
// This is the strongest non-tautological check available to this package: the
// standard publishes every intermediate value of one complete encryption, so
// each derivation step is asserted against a number the platform did not
// produce. An implementation that agreed only with itself would pass a
// round-trip test and still be rejected by every push service on earth.
const (
	rfcPlaintextB64 = "V2hlbiBJIGdyb3cgdXAsIEkgd2FudCB0byBiZSBhIHdhdGVybWVsb24"
	rfcUAPublic     = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	rfcUAPrivate    = "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"
	rfcASPublic     = "BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8"
	rfcASPrivate    = "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"
	rfcSalt         = "DGv6ra1nlYgDCS1FRnbzlw"
	rfcAuthSecret   = "BTBZMqHH6r4Tts7J_aSIgg"

	rfcECDHSecret = "kyrL1jIIOHEzg3sM2ZWRHDRB62YACZhhSlknJ672kSs"
	rfcPRKKey     = "Snr3JMxaHVDXHWJn5wdC52WjpCtd2EIEGBykDcZW32k"
	rfcKeyInfo    = "V2ViUHVzaDogaW5mbwAEJXGyvs3942BVGq8e0PTNNmwRzr5VX4m8t7GGpTM5FzFo7OLr4BhZe9MEebhuPI-OztV3ylkYfpJGmQ22ggCLDgT-M_SrDepxkU21WCP3O1SUj0EwbZIHMtu5pZpTKGSCIA5Zent7wmC6HCJ5mFgJkuk5cwAvMBKiiujwa7t45ewP"
	rfcIKM        = "S4lYMb_L0FxCeq0WhDx813KgSYqU26kOyzWUdsXYyrg"
	rfcPRK        = "09_eUZGrsvxChDCGRCdkLiDXrReGOEVeSCdCcPBSJSc"
	rfcCEKInfo    = "Q29udGVudC1FbmNvZGluZzogYWVzMTI4Z2NtAA"
	rfcCEK        = "oIhVW04MRdy2XN9CiKLxTg"
	rfcNonceInfo  = "Q29udGVudC1FbmNvZGluZzogbm9uY2UA"
	rfcNonce      = "4h_95klXJ5E_qnoN"

	rfcHeader     = "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8"
	rfcCiphertext = "8pfeW0KbunFT06SuDKoJH9Ql87S1QUrdirN6GcG7sFz1y1sqLgVi1VhjVkHsUoEsbI_0LpXMuGvnzQ"
)

func mustDecode(t *testing.T, label, s string) []byte {
	t.Helper()
	b, err := b64.DecodeString(s)
	if err != nil {
		t.Fatalf("decode %s: %v", label, err)
	}
	return b
}

func rfcKeys() Keys { return Keys{P256DH: rfcUAPublic, Auth: rfcAuthSecret} }

// TestRFC8291AppendixAEveryIntermediateValue pins the derivation STEP BY STEP
// against the standard's published example. A single wrong label, a swapped
// HKDF argument or a reversed key concatenation moves exactly one of these and
// names itself.
func TestRFC8291AppendixAEveryIntermediateValue(t *testing.T) {
	uaPublic := mustDecode(t, "ua_public", rfcUAPublic)
	asPublic := mustDecode(t, "as_public", rfcASPublic)
	authSecret := mustDecode(t, "auth", rfcAuthSecret)
	salt := mustDecode(t, "salt", rfcSalt)

	asPriv, err := ecdh.P256().NewPrivateKey(mustDecode(t, "as_private", rfcASPrivate))
	if err != nil {
		t.Fatalf("as_private: %v", err)
	}
	if got := b64.EncodeToString(asPriv.PublicKey().Bytes()); got != rfcASPublic {
		t.Fatalf("as_private does not carry the RFC's as_public: got %s", got)
	}
	uaKey, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		t.Fatalf("ua_public: %v", err)
	}

	// Step 1 — the ECDH shared secret.
	ecdhSecret, err := asPriv.ECDH(uaKey)
	if err != nil {
		t.Fatalf("ECDH: %v", err)
	}
	if got := b64.EncodeToString(ecdhSecret); got != rfcECDHSecret {
		t.Errorf("ecdh_secret = %s, want %s", got, rfcECDHSecret)
	}

	// Step 2 — PRK_key = HKDF-Extract(auth_secret, ecdh_secret). The auth secret
	// is the SALT of this extract, which is the argument order most easily got
	// backwards; reversing it moves this line and nothing else.
	prkKey := hkdfExtract(t, ecdhSecret, authSecret)
	if got := b64.EncodeToString(prkKey); got != rfcPRKKey {
		t.Errorf("PRK_key = %s, want %s", got, rfcPRKKey)
	}

	// Step 3 — key_info, byte for byte. The concatenation order is ua_public
	// THEN as_public; swapping them still produces a 32-octet IKM.
	keyInfo := append(append([]byte(keyInfoPrefix), uaPublic...), asPublic...)
	if got := b64.EncodeToString(keyInfo); got != rfcKeyInfo {
		t.Errorf("key_info = %s, want %s", got, rfcKeyInfo)
	}

	// Step 4 — IKM.
	ikm := hkdfExpand(t, prkKey, string(keyInfo), 32)
	if got := b64.EncodeToString(ikm); got != rfcIKM {
		t.Errorf("IKM = %s, want %s", got, rfcIKM)
	}

	// Step 5 — the content-encoding PRK, from the per-message salt.
	prk := hkdfExtract(t, ikm, salt)
	if got := b64.EncodeToString(prk); got != rfcPRK {
		t.Errorf("PRK = %s, want %s", got, rfcPRK)
	}

	// Step 6 — the two labels and what they derive.
	if got := b64.EncodeToString([]byte(cekInfo)); got != rfcCEKInfo {
		t.Errorf("cek_info = %s, want %s", got, rfcCEKInfo)
	}
	if got := b64.EncodeToString([]byte(nonceInfo)); got != rfcNonceInfo {
		t.Errorf("nonce_info = %s, want %s", got, rfcNonceInfo)
	}
	cek := hkdfExpand(t, prk, cekInfo, 16)
	if got := b64.EncodeToString(cek); got != rfcCEK {
		t.Errorf("CEK = %s, want %s", got, rfcCEK)
	}
	nonce := hkdfExpand(t, prk, nonceInfo, 12)
	if got := b64.EncodeToString(nonce); got != rfcNonce {
		t.Errorf("NONCE = %s, want %s", got, rfcNonce)
	}

	// The same two values through the production derivation, so the steps above
	// are pinning what Encrypt actually uses rather than a parallel copy.
	prodCEK, prodNonce, err := deriveKeys(asPriv, uaKey, uaPublic, asPublic, authSecret, salt)
	if err != nil {
		t.Fatalf("deriveKeys: %v", err)
	}
	if !bytes.Equal(prodCEK, cek) || !bytes.Equal(prodNonce, nonce) {
		t.Errorf("deriveKeys disagrees with the step-by-step derivation:\n cek  %x vs %x\n nonce %x vs %x",
			prodCEK, cek, prodNonce, nonce)
	}

	// Step 7 — the 86-octet header, and step 8, the sealed record.
	body, err := encryptWith(rfcKeys(), mustDecode(t, "plaintext", rfcPlaintextB64), asPriv, salt)
	if err != nil {
		t.Fatalf("encryptWith: %v", err)
	}
	wantHeader := mustDecode(t, "header", rfcHeader)
	if len(wantHeader) != 86 {
		t.Fatalf("the RFC's header is %d octets, want 86 — the constant above is mistranscribed", len(wantHeader))
	}
	if got := body[:len(wantHeader)]; !bytes.Equal(got, wantHeader) {
		t.Errorf("header = %s\n  want   %s", b64.EncodeToString(got), rfcHeader)
	}
	if got := b64.EncodeToString(body[len(wantHeader):]); got != rfcCiphertext {
		t.Errorf("ciphertext = %s\n      want   %s", got, rfcCiphertext)
	}
}

// hkdfExtract/hkdfExpand spell the two HKDF calls out at the call site, so the
// assertions above read as the RFC's own steps rather than as a second copy of
// the production derivation.
func hkdfExtract(t *testing.T, secret, salt []byte) []byte {
	t.Helper()
	out, err := hkdf.Extract(sha256.New, secret, salt)
	if err != nil {
		t.Fatalf("hkdf extract: %v", err)
	}
	return out
}

func hkdfExpand(t *testing.T, prk []byte, info string, n int) []byte {
	t.Helper()
	out, err := hkdf.Expand(sha256.New, prk, info, n)
	if err != nil {
		t.Fatalf("hkdf expand: %v", err)
	}
	return out
}

// The two failures decryptAsReceiver can report. They are test-local because
// the platform never decrypts: only a user agent does.
var (
	errShort       = errors.New("body is shorter than an aes128gcm header")
	errNoDelimiter = errors.New("record carries no RFC 8188 padding delimiter")
)

// TestEncryptRoundTripsThroughAReceiver decrypts a freshly sealed body exactly
// as a user agent does — the OTHER side of the ECDH, with the subscription's
// private key against the sender's ephemeral public key lifted out of the
// header's own keyid field. It runs on random keys and a random salt, so it
// covers the production path the fixed-vector test cannot: that the header the
// sender writes is the one a receiver reads its key material back out of.
func TestEncryptRoundTripsThroughAReceiver(t *testing.T) {
	uaPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ua key: %v", err)
	}
	authSecret := make([]byte, authSecretLen)
	if _, err := rand.Read(authSecret); err != nil {
		t.Fatalf("auth secret: %v", err)
	}
	keys := Keys{
		P256DH: b64.EncodeToString(uaPriv.PublicKey().Bytes()),
		Auth:   b64.EncodeToString(authSecret),
	}
	plaintext := []byte(`{"web_push":8030,"notification":{"title":"round trip"}}`)

	body, err := Encrypt(keys, plaintext, nil)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := decryptAsReceiver(uaPriv, authSecret, body)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("round trip = %q, want %q", got, plaintext)
	}

	// The non-tautological control: the same body against a DIFFERENT
	// subscription's private key must not decrypt. Without it, a receiver that
	// ignored the key material would pass the assertion above.
	other, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("other key: %v", err)
	}
	if _, err := decryptAsReceiver(other, authSecret, body); err == nil {
		t.Error("a body sealed to one subscription decrypted under another's key")
	}
	// And the same key with a different auth secret, which is the half the ECDH
	// alone does not cover.
	wrongAuth := make([]byte, authSecretLen)
	copy(wrongAuth, authSecret)
	wrongAuth[0] ^= 0xff
	if _, err := decryptAsReceiver(uaPriv, wrongAuth, body); err == nil {
		t.Error("a body decrypted under the wrong auth secret")
	}
}

// TestEncryptIsFreshPerMessage pins the property the scheme rests on: two
// encryptions of the same plaintext to the same subscription share no salt and
// no ephemeral key, so no (key, nonce) pair is ever reused.
func TestEncryptIsFreshPerMessage(t *testing.T) {
	uaPriv, _ := ecdh.P256().GenerateKey(rand.Reader)
	authSecret := make([]byte, authSecretLen)
	if _, err := rand.Read(authSecret); err != nil {
		t.Fatalf("auth: %v", err)
	}
	keys := Keys{P256DH: b64.EncodeToString(uaPriv.PublicKey().Bytes()), Auth: b64.EncodeToString(authSecret)}

	a, err := Encrypt(keys, []byte("same"), nil)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	b, err := Encrypt(keys, []byte("same"), nil)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Equal(a[:saltLen], b[:saltLen]) {
		t.Error("two messages share a salt")
	}
	if bytes.Equal(a[saltLen+5:86], b[saltLen+5:86]) {
		t.Error("two messages share an ephemeral public key")
	}
	if bytes.Equal(a, b) {
		t.Error("two encryptions of one plaintext produced identical bodies")
	}
}

// TestEncryptRefusesUnusableKeyMaterial covers the one boundary this package
// validates: what a browser handed us. Each case is a permanent property of the
// stored row, so each must be ErrBadSubscription rather than a retryable error.
func TestEncryptRefusesUnusableKeyMaterial(t *testing.T) {
	good := Keys{P256DH: rfcUAPublic, Auth: rfcAuthSecret}
	cases := []struct {
		name string
		keys Keys
	}{
		{"p256dh not base64url", Keys{P256DH: "not base64!", Auth: good.Auth}},
		{"p256dh wrong length", Keys{P256DH: b64.EncodeToString(make([]byte, 32)), Auth: good.Auth}},
		{"p256dh not on the curve", Keys{P256DH: b64.EncodeToString(append([]byte{0x04}, make([]byte, 64)...)), Auth: good.Auth}},
		{"auth not base64url", Keys{P256DH: good.P256DH, Auth: "not base64!"}},
		{"auth wrong length", Keys{P256DH: good.P256DH, Auth: b64.EncodeToString(make([]byte, 8))}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Encrypt(c.keys, []byte("x"), nil); err == nil {
				t.Fatal("accepted unusable key material")
			} else if !strings.Contains(err.Error(), ErrBadSubscription.Error()) {
				t.Errorf("error %v does not classify as ErrBadSubscription", err)
			}
		})
	}
	if _, err := Encrypt(good, []byte("x"), nil); err != nil {
		t.Errorf("the control case failed: %v", err)
	}
}

// ── RFC 8292 VAPID ──────────────────────────────────────────────────────────

// TestVAPIDSignatureIsRawRS is the check that catches the classic silent
// failure: ecdsa.SignASN1 produces a valid signature of the same message in the
// WRONG ENCODING, which passes a naive local round trip and is rejected by every
// push service. The assertion is not "the length is 64" — it is that the
// signature VERIFIES through the split a push service performs, with the ASN.1
// form driven through the same verifier and refused.
func TestVAPIDSignatureIsRawRS(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	tok, err := vapidJWT(key, "https://push.example", "mailto:ops@example", time.Now().Add(time.Hour), rand.Reader)
	if err != nil {
		t.Fatalf("vapidJWT: %v", err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT has %d parts, want 3", len(parts))
	}
	sig := mustDecode(t, "signature", parts[2])
	if len(sig) != 64 {
		t.Fatalf("signature is %d octets, want the raw 64-octet R‖S of JWS ES256", len(sig))
	}
	if !verifyES256(key, parts[0]+"."+parts[1], sig) {
		t.Fatal("the raw R‖S signature does not verify")
	}

	// The probe. A DER signature over the SAME input is a correct signature and
	// this verifier must still refuse it — that refusal is exactly what a push
	// service does, and it is why FillBytes is used above.
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	der, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatalf("SignASN1: %v", err)
	}
	if len(der) == 64 {
		t.Fatal("the DER probe happens to be 64 octets; the length assertion above cannot distinguish it")
	}
	if verifyES256(key, parts[0]+"."+parts[1], der) {
		t.Error("an ASN.1 signature was accepted by the raw-R‖S verifier")
	}
	if verifyES256(key, parts[0]+"."+parts[1], der[:64]) {
		t.Error("a truncated ASN.1 signature was accepted by the raw-R‖S verifier")
	}

	// A short R or S must still occupy its full 32 octets. FillBytes is what
	// guarantees it; a big.Int.Bytes() concatenation would silently shorten the
	// signature roughly one time in 256 and be rejected only sometimes.
	for i := 0; i < 64; i++ {
		tok, err := vapidJWT(key, "https://push.example", "mailto:ops@example", time.Now().Add(time.Hour), rand.Reader)
		if err != nil {
			t.Fatalf("vapidJWT: %v", err)
		}
		p := strings.Split(tok, ".")
		if s := mustDecode(t, "signature", p[2]); len(s) != 64 {
			t.Fatalf("signature %d is %d octets, want 64", i, len(s))
		}
	}
}

// verifyES256 is what a push service does with the `t=` token: split the
// signature into two fixed 32-octet halves and verify. It is deliberately the
// only verifier in this test, so an ASN.1 signature cannot pass by being
// checked with a decoder that understands it.
func verifyES256(key *ecdsa.PrivateKey, signingInput string, sig []byte) bool {
	r, s, ok := bigIntsFromSignature(sig)
	if !ok {
		return false
	}
	digest := sha256.Sum256([]byte(signingInput))
	return ecdsa.Verify(&key.PublicKey, digest[:], r, s)
}

// TestVAPIDClaimsAreTheRFC8292Set pins `aud` to the endpoint's ORIGIN (never
// its path, which is the subscription capability), the contact `sub`, and an
// `exp` inside Apple's documented one-day ceiling.
func TestVAPIDClaimsAreTheRFC8292Set(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

	aud, err := vapidAudience("https://web.push.apple.com/QDzVuUUFuFXY-abc123/def456")
	if err != nil {
		t.Fatalf("vapidAudience: %v", err)
	}
	if aud != "https://web.push.apple.com" {
		t.Errorf("aud = %q, want the origin alone", aud)
	}

	tok, err := vapidJWT(key, aud, "mailto:ops@example", now.Add(vapidExpiry), rand.Reader)
	if err != nil {
		t.Fatalf("vapidJWT: %v", err)
	}
	parts := strings.Split(tok, ".")
	var head struct {
		Typ string `json:"typ"`
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(mustDecode(t, "header", parts[0]), &head); err != nil {
		t.Fatalf("header: %v", err)
	}
	if head.Typ != "JWT" || head.Alg != "ES256" {
		t.Errorf("JOSE header = %+v, want {JWT ES256}", head)
	}
	var claims vapidClaims
	if err := json.Unmarshal(mustDecode(t, "claims", parts[1]), &claims); err != nil {
		t.Fatalf("claims: %v", err)
	}
	if claims.Aud != aud {
		t.Errorf("aud = %q, want %q", claims.Aud, aud)
	}
	if claims.Sub != "mailto:ops@example" {
		t.Errorf("sub = %q", claims.Sub)
	}
	if got := time.Unix(claims.Exp, 0).UTC(); !got.Equal(now.Add(vapidExpiry)) {
		t.Errorf("exp = %s, want %s", got, now.Add(vapidExpiry))
	}
	// Apple: "The JWT expiration parameter is more than one day into the future"
	// is a refusal (BadJwtToken), re-verified 2026-07-30.
	if vapidExpiry >= 24*time.Hour {
		t.Errorf("vapidExpiry %s reaches Apple's one-day ceiling", vapidExpiry)
	}
	// And the renewal margin has to leave room for the once-an-hour refresh
	// floor: with a margin below the expiry, one token covers many hours.
	if vapidRenewMargin >= vapidExpiry {
		t.Errorf("vapidRenewMargin %s >= vapidExpiry %s: every send would re-sign", vapidRenewMargin, vapidExpiry)
	}

	// A non-https endpoint is refused rather than signed for.
	for _, bad := range []string{"http://push.example/x", "not a url", "://", "https:///path"} {
		if _, err := vapidAudience(bad); err == nil {
			t.Errorf("vapidAudience(%q) accepted a non-https endpoint", bad)
		}
	}
}

// TestVAPIDPublicKeyIsTheSubscribeKey pins the equality the push service
// enforces: the `k=` parameter and the `applicationServerKey` the browser
// subscribed with must be the same 65-octet uncompressed point.
func TestVAPIDPublicKeyIsTheSubscribeKey(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pub, err := vapidPublicKey(key)
	if err != nil {
		t.Fatalf("vapidPublicKey: %v", err)
	}
	raw := mustDecode(t, "k", pub)
	if len(raw) != uncompressedPointLen || raw[0] != 0x04 {
		t.Fatalf("k is %d octets starting %#x, want a 65-octet uncompressed point", len(raw), raw[0])
	}
	if _, err := ecdh.P256().NewPublicKey(raw); err != nil {
		t.Errorf("k does not parse as a P-256 point: %v", err)
	}
	if got := authorizationHeader("TOK", pub); got != "vapid t=TOK, k="+pub {
		t.Errorf("Authorization = %q", got)
	}
}

// TestWireHeadersMatchTheServicesDocumentedRules pins the three RFC 8030
// headers against what the push services actually enforce.
func TestWireHeadersMatchTheServicesDocumentedRules(t *testing.T) {
	// Apple refuses a TTL that "is either missing or isn't a positive number".
	if got := ttlHeader(24 * time.Hour); got != "86400" {
		t.Errorf("TTL = %q, want 86400", got)
	}
	if got := ttlHeader(0); got != "1" {
		t.Errorf("TTL for a zero duration = %q, want a positive number", got)
	}
	if got := ttlHeader(-5 * time.Minute); got != "1" {
		t.Errorf("TTL for a negative duration = %q, want a positive number", got)
	}
	// Topic: "a maximum of 32 characters from the URL or filename-safe Base64
	// character sets".
	topic := topicHeader("ask:ask-verify-0001")
	if len(topic) != 32 {
		t.Errorf("Topic is %d characters, want 32", len(topic))
	}
	for _, r := range topic {
		if !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_", r) {
			t.Errorf("Topic %q carries %q, outside the base64url alphabet", topic, r)
		}
	}
	// The same card coalesces; a different one does not. That is the whole point
	// of sending a Topic, so it is asserted rather than assumed.
	if topicHeader("ask:ask-verify-0001") != topic {
		t.Error("Topic is not stable for one card")
	}
	if topicHeader("ask:ask-verify-0002") == topic {
		t.Error("two different cards share a Topic and would coalesce")
	}
	// The Topic names no part of the card id it came from.
	if strings.Contains(topic, "ask") || strings.Contains(topic, "verify") {
		t.Errorf("Topic %q leaks the card id it was derived from", topic)
	}
}

// DecryptForTest exposes the receiver half to the external push_test package,
// which is where the wire-level assertions live. It exists ONLY in test files:
// the platform is a sender and never decrypts, so no production code carries a
// decrypt path.
func DecryptForTest(uaPriv *ecdh.PrivateKey, authSecret, body []byte) ([]byte, error) {
	return decryptAsReceiver(uaPriv, authSecret, body)
}

// decryptAsReceiver is the user-agent half of RFC 8291, used only by the round-
// trip test: it reads the sender's ephemeral key out of the header's keyid
// field and derives the same CEK/nonce from the other side of the ECDH.
func decryptAsReceiver(uaPriv *ecdh.PrivateKey, authSecret, body []byte) ([]byte, error) {
	if len(body) < saltLen+5 {
		return nil, errShort
	}
	salt := body[:saltLen]
	idLen := int(body[saltLen+4])
	if len(body) < saltLen+5+idLen {
		return nil, errShort
	}
	asPublic := body[saltLen+5 : saltLen+5+idLen]
	ciphertext := body[saltLen+5+idLen:]

	asKey, err := ecdh.P256().NewPublicKey(asPublic)
	if err != nil {
		return nil, err
	}
	uaPublic := uaPriv.PublicKey().Bytes()
	cek, nonce, err := deriveKeys(uaPriv, asKey, uaPublic, asPublic, authSecret, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	record, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	// Strip the RFC 8188 padding delimiter and any padding after it.
	for i := len(record) - 1; i >= 0; i-- {
		if record[i] == 0 {
			continue
		}
		if record[i] != paddingDelimiter && record[i] != 0x01 {
			return nil, errNoDelimiter
		}
		return record[:i], nil
	}
	return nil, errNoDelimiter
}

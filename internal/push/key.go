package push

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// key.go holds the one long-lived secret this family creates: the VAPID
// application-server signing key (RFC 8292).
//
// CUSTODY AT v0 IS THE CONTROL PLANE'S, AND IT IS AN ACCEPTED INTERIM
// (B6-9 OQ8(b), ratified). The key lives under the systemd StateDirectory at
// 0600, generated at first need. Moving custody to the credential broker — so
// the key never leaves it and signing becomes a broker operation, the
// git-signing-key posture — is a NAMED hardening-session item, not something to
// improvise mid-packet: the broker would need a new operation and a
// secrets-at-rest placement, both of which are sudo-adjacent host work. The
// JWT cadence makes the round trip negligible when it happens (one signature
// per push service per several hours), so nothing about this design fights the
// move.
//
// ROTATION IS DOCUMENTED, NOT AUTOMATED, and the reason is not laziness:
// rotating the VAPID key INVALIDATES EVERY SUBSCRIPTION in the household —
// every browser subscribed with the old public key as its
// `applicationServerKey`, and a push signed by a different key is refused. So a
// rotation is an operator ceremony (retire the key, re-enrol every device) and
// the runbook says so. A platform that rotated on a timer would silently stop
// notifying the household.

const vapidKeyFile = "vapid-key.pem"

// vapidKey is the loaded signing key plus its base64url public half, computed
// once because it is served on every enrolment read.
type vapidKey struct {
	key       *ecdsa.PrivateKey
	publicKey string
}

// loadOrCreateVAPIDKey reads the key from the state directory, minting it on
// first use. The file is written 0600 through O_EXCL, so two processes racing
// at first boot cannot both believe they created it — one loses the create and
// reads the winner's key, which is what keeps a household's subscriptions valid.
func loadOrCreateVAPIDKey(stateDir string) (*vapidKey, error) {
	path := filepath.Join(stateDir, vapidKeyFile)
	if key, err := readVAPIDKey(path); err == nil {
		return key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("push: state dir: %w", err)
	}
	fresh, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("push: generate VAPID key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(fresh)
	if err != nil {
		return nil, fmt.Errorf("push: marshal VAPID key: %w", err)
	}
	block := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			// Somebody else created it between the read and the create. Their
			// key is the household's key.
			return readVAPIDKey(path)
		}
		return nil, fmt.Errorf("push: create VAPID key: %w", err)
	}
	if _, werr := f.Write(block); werr != nil {
		f.Close()
		return nil, fmt.Errorf("push: write VAPID key: %w", werr)
	}
	if cerr := f.Close(); cerr != nil {
		return nil, fmt.Errorf("push: close VAPID key: %w", cerr)
	}
	return newVAPIDKey(fresh)
}

func readVAPIDKey(path string) (*vapidKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("push: %s is not PEM", path)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("push: parse VAPID key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, fmt.Errorf("push: VAPID key is not an ECDSA P-256 key (RFC 8292 requires ES256)")
	}
	return newVAPIDKey(key)
}

func newVAPIDKey(key *ecdsa.PrivateKey) (*vapidKey, error) {
	pub, err := vapidPublicKey(key)
	if err != nil {
		return nil, err
	}
	return &vapidKey{key: key, publicKey: pub}, nil
}

// randomID mints a subscription id. Ids are platform-minted rather than
// caller-supplied, so no client can choose one.
func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail on any platform this runs on; a panic here
		// is honest, because a silent fallback to a weak id would be worse.
		panic("push: crypto/rand: " + err.Error())
	}
	return "push-" + hex.EncodeToString(b)
}

// normalizeOrigin reduces the enrolling page's location to a bare origin and
// refuses anything a browser could not have subscribed from.
//
// The rule mirrors the platform's own: Push requires a SECURE CONTEXT, which is
// https anywhere plus http on loopback. Storing anything else would mean
// composing a deep link a device can never open, so the refusal is at the
// boundary rather than at send time.
func normalizeOrigin(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("origin is empty: the enrolling page's own origin is what a deep link has to land on")
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("origin is not a URL: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("origin %q names no host", raw)
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(u.Hostname()) {
			return "", fmt.Errorf("origin %q is plain http off loopback, which cannot hold a push subscription", raw)
		}
	default:
		return "", fmt.Errorf("origin %q has scheme %q, want https (or http on loopback)", raw, u.Scheme)
	}
	return u.Scheme + "://" + u.Host, nil
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

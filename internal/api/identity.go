package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
)

// Identity is the caller identity a request carries once the identity
// middleware has run. Person identity is authoritative only at layer 3 of
// the Spec S01.9 stack — a server-side session; Dev marks the dev-posture
// fallback identity, which is resolution only and exists so dev mode works
// without tailscale, certs, or a login (P3/CONVENTIONS.md §7).
type Identity struct {
	UserID string
	Dev    bool
}

// ErrNoIdentity is the anonymous outcome: the request carries no valid
// session (and no dev fallback applies). The middleware maps it to 401 on
// session-required routes and to plain anonymity on the pre-session
// surface — reaching that surface already proves tailnet membership (Spec
// S01.9 layer 1).
var ErrNoIdentity = errors.New("api: no identity")

// Authenticator is the identity-middleware seam of the S01.9 authentication
// stack: it resolves a request to the caller's identity. It returns
// ErrNoIdentity for an anonymous request; any other error is an internal
// resolution failure (fail closed, 500). SessionAuthenticator is the real
// implementation; tests may substitute.
type Authenticator interface {
	Authenticate(r *http.Request) (Identity, error)
}

// SessionCookieName carries the session bearer token (Spec S01.9 layer 3).
const SessionCookieName = "sinet_session"

// HeaderDeviceLogin is the device-hint identity header of the S01.4 front
// chain: tailscale serve injects Tailscale-User-* on proxied tailnet
// requests and strips inbound copies, which is what makes the value
// spoof-resistant end to end through the chain. It is device identity, not
// person identity, and never authority by itself (Spec S01.9 layer 2). In
// dev posture tests inject it directly — same parse, no chain.
const HeaderDeviceLogin = "Tailscale-User-Login"

// maxDeviceHintLen bounds the accepted header value.
const maxDeviceHintLen = 256

// DeviceHint parses the device-hint header. Only Tailscale-User-Login is
// consumed: it is the one header of the injected set to which Spec S01.9
// assigns meaning. A malformed value reads as absent — the hint is
// advisory, never load-bearing.
func DeviceHint(r *http.Request) string {
	v := strings.TrimSpace(r.Header.Get(HeaderDeviceLogin))
	if len(v) > maxDeviceHintLen {
		return ""
	}
	for _, c := range v {
		if unicode.IsControl(c) {
			return ""
		}
	}
	return v
}

// SessionAuthenticator is the production Authenticator (Spec S01.9 layer
// 3): the session cookie resolves against the session store. When
// DevFallback is true (dev posture only — the shell keys it off the
// absence of systemd env, never a build flag), a request without a valid
// session resolves to the fixed dev identity instead of anonymity, so dev
// flows and the SSE demo work without a login; sessions still win when
// present, so the full login flow is exercisable in dev.
type SessionAuthenticator struct {
	Sessions    *auth.Store
	DevFallback bool
}

// Authenticate implements Authenticator.
func (a SessionAuthenticator) Authenticate(r *http.Request) (Identity, error) {
	if c, err := r.Cookie(SessionCookieName); err == nil && c.Value != "" {
		userID, verr := a.Sessions.Validate(r.Context(), c.Value)
		if verr == nil {
			return Identity{UserID: userID}, nil
		}
		if !errors.Is(verr, auth.ErrNoSession) {
			return Identity{}, verr // infra failure — fail closed
		}
	}
	if a.DevFallback {
		return Identity{UserID: auth.DevUserID, Dev: true}, nil
	}
	return Identity{}, ErrNoIdentity
}

// identityKey carries the resolved Identity in the request context.
type identityKey struct{}

// identity is the outer middleware: resolve the caller and attach the
// identity to the request context. Anonymous requests pass through
// unattached — enforcement is requireIdentity's, per route; an internal
// resolution failure is fail-closed 500 (never a silent anonymous
// downgrade).
func (s *Server) identity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := s.auth.Authenticate(r)
		switch {
		case errors.Is(err, ErrNoIdentity):
			next.ServeHTTP(w, r)
		case err != nil:
			s.logger.Error("identity: resolution failed", "err", err)
			http.Error(w, "identity resolution failed", http.StatusInternalServerError)
		default:
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, id)))
		}
	})
}

// requireIdentity guards session-required routes: no resolved identity is
// 401 (Spec S15.2 — nothing renders from private access; the browser is a
// display, never an authority).
func (s *Server) requireIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := IdentityFrom(r.Context()); !ok {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// IdentityFrom returns the request identity attached by the middleware.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

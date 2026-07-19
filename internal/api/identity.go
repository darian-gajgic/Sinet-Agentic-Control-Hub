package api

import (
	"context"
	"net/http"
)

// Identity is the caller identity a request carries once the identity
// middleware has run. Person identity is authoritative only at layer 3 of
// the Spec S01.9 stack (server-side sessions); that stack is P3-B0-5's.
type Identity struct {
	UserID string
}

// Authenticator is the identity-middleware seam of the S01.9 authentication
// stack: it resolves a request to the caller's identity. The session + PIN
// machinery, the Tailscale-User-* device hint, and every authorization
// decision arrive with their owning packet (B0-5); this interface exists so
// the request chain has the seam from the first build and handlers never
// read auth state from anywhere else.
type Authenticator interface {
	Authenticate(r *http.Request) (Identity, error)
}

// DevAuthenticator is the dev-mode implementation: every request is the
// fixed dev identity. It is sanctioned only behind the loopback-only dev
// posture (the listener lint guarantees no non-loopback exposure, P-T13-2);
// it makes no authorization decisions because none exist at B0.
type DevAuthenticator struct {
	UserID string // "" = "dev"
}

// Authenticate implements Authenticator.
func (d DevAuthenticator) Authenticate(*http.Request) (Identity, error) {
	id := d.UserID
	if id == "" {
		id = "dev"
	}
	return Identity{UserID: id}, nil
}

// identityKey carries the resolved Identity in the request context.
type identityKey struct{}

// identity is the middleware: resolve the caller and attach it to the
// request context; a resolution failure is 401 (fail-closed at the seam).
func (s *Server) identity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := s.auth.Authenticate(r)
		if err != nil {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, id)))
	})
}

// IdentityFrom returns the request identity attached by the middleware.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

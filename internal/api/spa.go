package api

import (
	"net/http"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/webui"
)

// withSPA puts the SPA embedded in this binary (Spec S01.5; S01.4 keeps Caddy
// to routing, not asset hosting) in front of the API mux, for the requests the
// machine surface does not own.
//
// The wall is structural rather than a route: /api/* and /events* go to the mux
// unconditionally — real handlers, its 404s and its 405s alike — so no machine
// path can be answered with HTML a caller would try to parse as JSON, and the
// landed method semantics move by exactly nothing. Everything else asked for
// with GET or HEAD is an app route: an embedded asset when one matches, and
// index.html otherwise, which is what makes Spec S15.11's stable URLs
// hard-navigable (deep links and bookmarks survive a full page load). Non-GET
// requests to app paths belong to nobody and keep falling to the mux's 404.
//
// The SPA surface is PRE-SESSION. The login page IS the SPA, so the assets and
// the fallback must serve before anyone has a session — and reaching them at
// all already proves tailnet membership, which is exactly the Spec S01.9
// layer-1 rationale that puts /api/health and the login endpoints in the
// pre-session class (P3/CONVENTIONS.md §9). This widens that route CLASS; its
// five API members are unchanged, and everything the SPA then calls stays
// fail-closed 401, /events included.
func (s *Server) withSPA(mux *http.ServeMux) http.Handler {
	assets := webui.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isAPIPath(r.URL.Path):
			mux.ServeHTTP(w, r)
		case r.Method == http.MethodGet || r.Method == http.MethodHead:
			assets.ServeHTTP(w, r)
		default:
			mux.ServeHTTP(w, r)
		}
	})
}

// isAPIPath reports whether a path belongs to the machine surface — the S15.2
// endpoint families or the one SSE endpoint (S15.3) — rather than to the app.
func isAPIPath(p string) bool {
	for _, root := range []string{"/api", "/events"} {
		if p == root || strings.HasPrefix(p, root+"/") {
			return true
		}
	}
	return false
}

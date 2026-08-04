package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
)

// assertServedByTheAppShell is the post-B6-4 form of "this path is not an API
// endpoint".
//
// Before the SPA existed, an unrouted path proved its own absence with a 404.
// Now every unrouted GET falls to the app shell — that is what makes Spec
// S15.11's stable URLs hard-navigable — so the invariant is unchanged and the
// EVIDENCE moved: the path is answered by the shell, never by a handler, so no
// caller can mistake it for a working endpoint.
func assertServedByTheAppShell(t *testing.T, path string, code int, body string) {
	t.Helper()
	if code != http.StatusOK || !strings.Contains(strings.ToLower(body), "<!doctype html") {
		t.Errorf("GET %s = %d %q — want the app shell: no API answers a versioned path (S15.2)", path, code, body)
	}
}

func spaGet(t *testing.T, srv *api.Server, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(method, path, nil))
	return rr
}

// The SPA is the login page, so it must serve with no session at all: reaching
// it already proves tailnet membership (Spec S01.9 layer 1, CONVENTIONS §9).
func TestSPAIsPreSessionAndTheAPIBehindItIsNot(t *testing.T) {
	// Production posture: no dev identity fallback, and no session cookie —
	// the fail-closed state every session-required route sees before a login.
	srv, _ := newTestServer(t, serverOpts{prod: true})

	for _, path := range []string{"/", "/login", "/inbox/ask-7", "/tasks/t-1"} {
		rr := spaGet(t, srv, http.MethodGet, path)
		if rr.Code != http.StatusOK {
			t.Errorf("GET %s with no session = %d, want 200 — the login page IS the SPA", path, rr.Code)
		}
		if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("GET %s Content-Type = %q, want html", path, ct)
		}
	}

	// The surface behind it did not widen by one route.
	for _, path := range []string{"/events", "/api/runs", "/api/approvals", "/api/settings"} {
		if rr := spaGet(t, srv, http.MethodGet, path); rr.Code != http.StatusUnauthorized {
			t.Errorf("GET %s with no session = %d, want 401 — the API stays fail-closed", path, rr.Code)
		}
	}
}

// The never-HTML wall, both directions: the machine surface never answers with
// the app shell, and app routes never answer with an API error.
func TestTheMachineSurfaceNeverFallsThroughToHTML(t *testing.T) {
	srv, _ := newTestServer(t, serverOpts{})

	for _, path := range []string{"/api/nonexistent", "/api/runs/nope/nope/nope", "/events/nope", "/api"} {
		rr := spaGet(t, srv, http.MethodGet, path)
		if rr.Code == http.StatusOK {
			t.Errorf("GET %s = 200 — an unknown machine path must not be served", path)
		}
		if strings.Contains(strings.ToLower(rr.Body.String()), "<!doctype html") {
			t.Errorf("GET %s answered with HTML — the SPA fallback is for app routes only", path)
		}
	}

	// And the landed method semantics are untouched by the SPA sitting in
	// front of the mux: a real route with the wrong method is still 405, and an
	// unrouted machine path is still 404 for every method.
	if rr := spaGet(t, srv, http.MethodPost, "/api/health"); rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/health = %d, want 405", rr.Code)
	}
	if rr := spaGet(t, srv, http.MethodPost, "/api/v1/meters/pause"); rr.Code != http.StatusNotFound {
		t.Errorf("POST /api/v1/meters/pause = %d, want 404", rr.Code)
	}
	// A non-GET app path belongs to nobody either.
	if rr := spaGet(t, srv, http.MethodPost, "/inbox"); rr.Code != http.StatusNotFound {
		t.Errorf("POST /inbox = %d, want 404 — the SPA answers GET/HEAD only", rr.Code)
	}
}

// HEAD is how a browser and the front chain probe a document; it must reach the
// same surface GET does.
func TestSPAAnswersHEAD(t *testing.T) {
	srv, _ := newTestServer(t, serverOpts{})

	rr := spaGet(t, srv, http.MethodHead, "/")
	if rr.Code != http.StatusOK {
		t.Fatalf("HEAD / = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("HEAD / Content-Type = %q, want html", ct)
	}
}

// The SPA route class shadows nothing: every landed pre-session endpoint still
// answers as itself with the shell in front of the mux.
func TestSPAShadowsNoLandedRoute(t *testing.T) {
	_, ts := newTestServer(t, serverOpts{})

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("get health: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/health = %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), `"event_head"`) {
		t.Errorf("GET /api/health did not serve the readiness snapshot: %q", body)
	}
}

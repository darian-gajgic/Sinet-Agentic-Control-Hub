package preview

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeAdmin is a local httptest stand-in for the Caddy admin API — loopback,
// in-process. The LIVE host Caddy admin API (fronting 8481/8482) is NEVER dialed
// by tests (host hazard). It tracks route ids so add/remove round-trips and
// 404-tolerant idempotency are assertable.
type fakeAdmin struct {
	mu    sync.Mutex
	ids   map[string]bool
	calls []call
}

type call struct{ method, path, body string }

func newFakeAdmin() (*fakeAdmin, *httptest.Server) {
	fa := &fakeAdmin{ids: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		fa.mu.Lock()
		fa.calls = append(fa.calls, call{r.Method, r.URL.Path, string(b)})
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/routes"):
			var route map[string]any
			_ = json.Unmarshal(b, &route)
			if id, ok := route["@id"].(string); ok {
				fa.ids[id] = true
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/id/"):
			id := strings.TrimPrefix(r.URL.Path, "/id/")
			if fa.ids[id] {
				delete(fa.ids, id)
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusNotFound) // idempotency: tolerated
			}
		default:
			w.WriteHeader(http.StatusOK)
		}
		fa.mu.Unlock()
	}))
	return fa, srv
}

func (fa *fakeAdmin) methods() []string {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	out := make([]string, len(fa.calls))
	for i, c := range fa.calls {
		out[i] = c.method + " " + c.path
	}
	return out
}

// TestCaddyAddRemoveRoundTrip: route add on launch / remove on teardown, both
// idempotent, against a CONFIGURED endpoint — asserting the exact admin calls
// (Spec S13.8; R12). No live admin API is dialed.
func TestCaddyAddRemoveRoundTrip(t *testing.T) {
	fa, srv := newFakeAdmin()
	defer srv.Close()
	c := NewCaddyClient(srv.URL, "srv0")
	ctx := context.Background()
	id := routeID("p1")

	if err := c.AddRoute(ctx, id, "preview-p1.host", "127.0.0.1:47600"); err != nil {
		t.Fatalf("AddRoute: %v", err)
	}
	// Add re-does idempotently: DELETE /id then POST /routes with the body.
	if err := c.AddRoute(ctx, id, "preview-p1.host", "127.0.0.1:47600"); err != nil {
		t.Fatalf("AddRoute (idempotent): %v", err)
	}
	if err := c.RemoveRoute(ctx, id); err != nil {
		t.Fatalf("RemoveRoute: %v", err)
	}
	// Remove is idempotent — a second remove tolerates the 404.
	if err := c.RemoveRoute(ctx, id); err != nil {
		t.Fatalf("RemoveRoute (idempotent): %v", err)
	}

	got := fa.methods()
	want := []string{
		"DELETE /id/" + id,                           // AddRoute #1 clears any prior
		"POST /config/apps/http/servers/srv0/routes", // AddRoute #1 adds
		"DELETE /id/" + id,                           // AddRoute #2 clears
		"POST /config/apps/http/servers/srv0/routes", // AddRoute #2 re-adds
		"DELETE /id/" + id,                           // RemoveRoute #1
		"DELETE /id/" + id,                           // RemoveRoute #2 (404 tolerated)
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("admin calls =\n %v\nwant\n %v", got, want)
	}
}

// TestRouteShapePortBasedNeverPath: every emitted route is subdomain/port-based
// with NO path matcher, and preserves the WebSocket upgrade/Connection header
// flow (no header handler strips them) — or arbitrary apps / Vite HMR break
// (Spec S13.8; R13).
func TestRouteShapePortBasedNeverPath(t *testing.T) {
	route := buildRoute("sinet-preview-p1", "preview-p1.host", "127.0.0.1:47600")
	blob, err := json.Marshal(route)
	if err != nil {
		t.Fatal(err)
	}
	js := string(blob)
	if strings.Contains(js, `"path"`) {
		t.Errorf("route contains a path matcher — forbidden (breaks arbitrary apps, R13): %s", js)
	}
	if !strings.Contains(js, `"host"`) {
		t.Errorf("route lacks a host (subdomain) matcher: %s", js)
	}
	if !strings.Contains(js, `"reverse_proxy"`) {
		t.Errorf("route lacks a reverse_proxy handler: %s", js)
	}
	// No `headers` handler that could strip/override Upgrade/Connection.
	if strings.Contains(js, `"handler":"headers"`) || strings.Contains(js, `"Connection"`) || strings.Contains(js, `"Upgrade"`) {
		t.Errorf("route has a header handler touching upgrade flow — Vite HMR would die (R13): %s", js)
	}
	if !strings.Contains(js, `"dial":"127.0.0.1:47600"`) {
		t.Errorf("route upstream is not the loopback pool port: %s", js)
	}
}

// TestRoutingDisabledNoOp: an unconfigured admin endpoint disables routing — the
// honest dev posture; no admin call is made (Spec S13.8; R12).
func TestRoutingDisabledNoOp(t *testing.T) {
	c := NewCaddyClient("", "")
	if c.Enabled() {
		t.Fatalf("empty endpoint should disable routing")
	}
	// No panic, no dial — pure no-ops.
	if err := c.AddRoute(context.Background(), "x", "h", "127.0.0.1:1"); err != nil {
		t.Fatalf("disabled AddRoute: %v", err)
	}
	if err := c.RemoveRoute(context.Background(), "x"); err != nil {
		t.Fatalf("disabled RemoveRoute: %v", err)
	}
}

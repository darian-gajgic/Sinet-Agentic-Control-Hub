package webui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// builtTree is a stand-in for a real Vite build: an entry document, one hashed
// asset of each kind, and the two public files. The serve rules are asserted
// against it so they hold whether or not a node build has ever run here; the
// real embedded output is exercised separately by TestRealEmbeddedBuild.
func builtTree() fs.FS {
	return fstest.MapFS{
		"index.html":                {Data: []byte("<!doctype html><title>Sinet</title><div id=root></div>")},
		"assets/index-BT34zZPF.js":  {Data: []byte("export const x = 1\n")},
		"assets/index-xgtUdbc9.css": {Data: []byte(":root{color:#fff}\n")},
		"manifest.webmanifest":      {Data: []byte(`{"name":"Sinet"}`)},
		"icon.svg":                  {Data: []byte("<svg xmlns='http://www.w3.org/2000/svg'/>")},
	}
}

func get(t *testing.T, assets fs.FS, path string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	handlerFor(assets).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	return rr
}

func TestRootServesTheEntryDocumentNoStore(t *testing.T) {
	rr := get(t, builtTree(), "/")

	if rr.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store — a deploy must take effect on reload", got)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rr.Body.String(), `id=root`) {
		t.Errorf("body is not the entry document: %q", rr.Body.String())
	}
}

func TestHashedAssetsAreImmutableWithCorrectMIME(t *testing.T) {
	assets := builtTree()
	for _, c := range []struct{ path, wantType string }{
		{"/assets/index-BT34zZPF.js", "text/javascript"},
		{"/assets/index-xgtUdbc9.css", "text/css"},
	} {
		rr := get(t, assets, c.path)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", c.path, rr.Code)
		}
		if got := rr.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
			t.Errorf("GET %s Cache-Control = %q, want immutable — the name carries a content hash", c.path, got)
		}
		if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, c.wantType) {
			t.Errorf("GET %s Content-Type = %q, want %s", c.path, ct, c.wantType)
		}
	}
}

// The manifest keeps its name across builds, so it must NOT be immutable, and
// its type is one Go's mime table does not carry.
func TestUnhashedPublicFilesAreNoStoreWithTheirOwnType(t *testing.T) {
	assets := builtTree()

	rr := get(t, assets, "/manifest.webmanifest")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /manifest.webmanifest = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/manifest+json" {
		t.Errorf("manifest Content-Type = %q, want application/manifest+json", got)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("manifest Cache-Control = %q, want no-store — the name does not change between builds", got)
	}

	if ct := get(t, assets, "/icon.svg").Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/svg+xml") {
		t.Errorf("icon Content-Type = %q, want image/svg+xml", ct)
	}
}

// Spec S15.11: every approval card and task has a stable URL, so a hard
// navigation to one must serve the app rather than 404.
func TestAppRoutesFallBackToTheEntryDocument(t *testing.T) {
	assets := builtTree()
	for _, path := range []string{"/inbox", "/inbox/ask-7", "/tasks/t-1", "/settings", "/deep/unknown/path"} {
		rr := get(t, assets, path)
		if rr.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 (SPA fallback)", path, rr.Code)
		}
		if !strings.Contains(rr.Body.String(), `id=root`) {
			t.Errorf("GET %s did not serve the entry document", path)
		}
		if got := rr.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s Cache-Control = %q, want no-store", path, got)
		}
	}
}

// The fallback stops at the hashed-asset directory: answering a missing script
// with HTML turns a stale deploy into an unreadable parse error in the browser.
func TestMissingHashedAssetIs404NeverHTML(t *testing.T) {
	rr := get(t, builtTree(), "/assets/index-DELETED.js")

	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET /assets/index-DELETED.js = %d, want 404", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "id=root") {
		t.Error("a missing hashed asset served the SPA shell")
	}
}

// The honest-absence leg: a binary with no build says so, and every route says
// the same thing rather than 404-ing or serving an empty shell.
func TestUnbuiltBinaryServesTheHonestPlaceholder(t *testing.T) {
	unbuilt := fstest.MapFS{".gitkeep": {Data: []byte{}}}
	if built(unbuilt) {
		t.Fatal("a tree with only .gitkeep must not read as built")
	}

	for _, path := range []string{"/", "/inbox/ask-7"} {
		rr := get(t, unbuilt, path)
		if rr.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 with the honest page", path, rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "UI not built into this binary") {
			t.Errorf("GET %s did not serve the honest-absence page: %q", path, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "npm run build") {
			t.Errorf("GET %s placeholder does not say how to fix it", path)
		}
	}
}

// TestRealEmbeddedBuild is the integration leg over the REAL embedded output.
// It auto-runs whenever this binary carries a build (CI always does — the web
// build runs before the Go steps) and prints a sanctioned skip otherwise: the
// web-assets skip class, the tier-R analog of CONVENTIONS §10.
func TestRealEmbeddedBuild(t *testing.T) {
	if !Built() {
		t.Skip("SANCTIONED SKIP (CONVENTIONS §41, web-assets): no SPA build embedded — run `cd web && npm run build`")
	}
	assets := Assets()

	rr := httptest.NewRecorder()
	Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `<div id="root">`) {
		t.Fatalf("GET / over the real build = %d: %q", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("real index Cache-Control = %q, want no-store", got)
	}

	// Vite's real output shape: hashed files under assets/, served immutable.
	entries, err := fs.ReadDir(assets, "assets")
	if err != nil {
		t.Fatalf("read assets/: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("the build embedded no hashed assets")
	}
	for _, e := range entries {
		rr := httptest.NewRecorder()
		Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/assets/"+e.Name(), nil))
		if rr.Code != http.StatusOK {
			t.Errorf("GET /assets/%s = %d", e.Name(), rr.Code)
		}
		if got := rr.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
			t.Errorf("GET /assets/%s Cache-Control = %q", e.Name(), got)
		}
	}

	// The PWA scaffold is really in the binary (S15.1 "installed as a PWA").
	rr = httptest.NewRecorder()
	Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("GET /manifest.webmanifest = %d — the install manifest is not embedded", rr.Code)
	}
}

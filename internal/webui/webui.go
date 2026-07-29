// Package webui embeds the built SPA and serves it out of the one release
// binary.
//
// Spec S01.5 puts the built SPA assets inside the single static Go binary, and
// S01.4 keeps Caddy to routing rather than asset hosting: everything a browser
// loads for the control plane is served from here. The npm tree that produces
// the assets lives in `web/` and carries no Go file — Vite writes its build
// into this package's dist/ (P3/CONVENTIONS.md §41) — so this is the only Go
// home for the frontend.
//
// The committed tree carries dist/.gitkeep and nothing else: built output is
// git-ignored, so a fresh clone compiles and `go test ./...` is green with no
// node build ever run. A binary built that way serves an honest "UI not built"
// page rather than a broken shell.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// The embedded build output. `all:` is required: it keeps dotfiles (the
// committed .gitkeep) in the tree, so the directive resolves on an unbuilt
// checkout instead of failing the build.
//
//go:embed all:dist
var embedded embed.FS

const (
	distDir = "dist"
	// indexFile is the SPA entry document and the SPA-fallback body.
	indexFile = "index.html"
	// assetsPrefix is Vite's hashed-output directory. Content hashes are in the
	// file names, which is what makes immutable caching safe there and nowhere
	// else.
	assetsPrefix = "assets/"
)

// contentTypes overrides extensions Go's mime table does not carry. Set as a
// response header rather than through mime.AddExtensionType so this package
// never mutates process-global state.
var contentTypes = map[string]string{
	".webmanifest": "application/manifest+json",
}

// Assets returns the embedded build output rooted at dist/.
func Assets() fs.FS {
	sub, err := fs.Sub(embedded, distDir)
	if err != nil {
		// Unreachable: distDir is embedded above, so the sub-tree always
		// resolves. Returning the whole FS would serve paths under a "dist/"
		// prefix nobody requests, so fail loudly instead of serving wrong.
		panic("webui: embedded " + distDir + " missing: " + err.Error())
	}
	return sub
}

// Built reports whether this binary carries a built SPA.
func Built() bool { return built(Assets()) }

func built(assets fs.FS) bool {
	_, err := fs.Stat(assets, indexFile)
	return err == nil
}

// Handler serves the embedded SPA: hashed assets with immutable caching, the
// entry document with no-store so a deploy takes effect on reload, and the
// SPA fallback that makes Spec S15.11's stable URLs hard-navigable.
//
// It answers app routes only. Distinguishing an app route from an API path is
// the caller's job — the API surface must never fall through to HTML.
func Handler() http.Handler { return handlerFor(Assets()) }

func handlerFor(assets fs.FS) http.Handler {
	return &handler{assets: assets}
}

type handler struct{ assets fs.FS }

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || name == "." {
		h.serveIndex(w, r)
		return
	}

	if info, err := fs.Stat(h.assets, name); err == nil && info.Mode().IsRegular() {
		h.serveAsset(w, r, name)
		return
	}

	// A missing file under the hashed-asset directory is a 404, never the SPA
	// shell: answering a missing script or stylesheet with HTML turns a stale
	// deploy into an unreadable parse error in the browser.
	if strings.HasPrefix(name, assetsPrefix) {
		http.NotFound(w, r)
		return
	}

	// Everything else is an app route (Spec S15.11 deep links and bookmarks).
	h.serveIndex(w, r)
}

func (h *handler) serveAsset(w http.ResponseWriter, r *http.Request, name string) {
	if ct, ok := contentTypes[strings.ToLower(path.Ext(name))]; ok {
		w.Header().Set("Content-Type", ct)
	}
	if strings.HasPrefix(name, assetsPrefix) {
		// Vite writes a content hash into every name here, so the bytes behind a
		// name never change.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		// Everything else (the manifest, the icon) keeps its name across builds,
		// and the embedded FS has no modification times to revalidate against.
		w.Header().Set("Cache-Control", "no-store")
	}
	http.ServeFileFS(w, r, h.assets, name)
}

// serveIndex serves the SPA entry document, or the honest-absence page when
// this binary carries no build.
func (h *handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !built(h.assets) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// 200, not an error status: the control plane is serving correctly and
		// this page says exactly what is missing. Readiness is /api/health's
		// answer, not this route's.
		_, _ = w.Write([]byte(notBuiltPage))
		return
	}
	http.ServeFileFS(w, r, h.assets, indexFile)
}

// notBuiltPage is the honest absence (P3/CONVENTIONS.md §41): a binary built
// without the SPA says so and says how to fix it, rather than serving a 404 or
// an empty shell that looks like a broken app.
const notBuiltPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sinet — UI not built into this binary</title>
</head>
<body>
<h1>UI not built into this binary</h1>
<p>This <code>sinet</code> binary was compiled without the SPA assets. The API
and the event stream are serving normally; only the web interface is absent.</p>
<p>Build the SPA, then rebuild the binary:</p>
<pre>cd web &amp;&amp; npm ci &amp;&amp; npm run build
go build ./cmd/sinet</pre>
</body>
</html>
`

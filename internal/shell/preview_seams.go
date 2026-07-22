package shell

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/portpool"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/preview"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/sandbox"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/units"
)

// The S13.8 preview module, constructed at the composition root from the
// PRODUCTION stores (R19) so it is compiled into the binary and reachable
// (go list -deps ./cmd/sinet includes internal/preview). The HTTP endpoints
// that expose launch/stop/list/before-vs-after (Spec S15.2 preview sessions
// under /api/deliverables) are the named B6 seam — NOT built here; the api
// layer holds this Manager for them. The in-process portpool allocator is the
// dev/v0 path (file-stub, R9); the sinet-portpool.service daemon serves the
// same allocator over its socket for production.

// previewDeps are the production stores the preview surface composes over.
type previewDeps struct {
	Reviews  *review.Store
	Projects *project.Store
	Sandbox  *sandbox.Composer
	Events   *eventlog.Log
	Settings *settings.Registry
	StateDir string
	Log      *slog.Logger
}

// buildPreviewSurface constructs the S13.8 preview Manager. The Caddy admin
// endpoint, the preview base host, and the port range are STRUCTURAL deployment
// config (composition-root override; NO invented ⚙ key — §8 reading 3), set at
// bring-up via env/config (the SINET_SRT_PATH precedent). Dev defaults: admin
// endpoint UNSET → routing disabled; base host = the tailnet FQDN; range = the
// portpool structural default.
func buildPreviewSurface(d previewDeps) (*preview.Manager, error) {
	lo, hi := envPortRange() // structural override (F18); 0,0 ⇒ portpool default
	ports, err := portpool.New(portpool.Config{
		Dir: filepath.Join(d.StateDir, "portpool", "reservations"),
		Lo:  lo, Hi: hi,
	})
	if err != nil {
		return nil, err
	}
	adminEndpoint := os.Getenv("SINET_PREVIEW_CADDY_ADMIN") // "" ⇒ routing disabled (dev)
	baseHost := os.Getenv("SINET_PREVIEW_BASE_HOST")
	if baseHost == "" {
		baseHost = units.FQDN
	}
	return preview.New(preview.Config{
		Reviews:  d.Reviews,
		Projects: d.Projects,
		Ports:    ports,
		Sandbox:  d.Sandbox,
		Caddy:    preview.NewCaddyClient(adminEndpoint, ""),
		Events:   d.Events,
		Settings: d.Settings,
		BaseHost: baseHost,
		// Throwaway clones live under a platform-owned dir, never system temp (F16).
		Scratch: filepath.Join(d.StateDir, "preview-clones"),
		Log:     d.Log,
	})
}

// envPortRange reads the structural port-range override (F18): SINET_PREVIEW_
// PORT_LO / _HI (both must parse to positive ints, lo<=hi) — else 0,0 (the
// portpool structural default). No ⚙ key; a bring-up deployment config value.
func envPortRange() (lo, hi int) {
	l, lerr := strconv.Atoi(os.Getenv("SINET_PREVIEW_PORT_LO"))
	h, herr := strconv.Atoi(os.Getenv("SINET_PREVIEW_PORT_HI"))
	if lerr == nil && herr == nil && l > 0 && h >= l {
		return l, h
	}
	return 0, 0
}

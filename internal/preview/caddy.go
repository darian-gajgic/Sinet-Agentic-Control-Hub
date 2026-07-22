package preview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// caddy.go is the minimal Caddy admin-API client, contained in the preview
// module exactly as the S16.3 Caddy row's funeral plan demands ("admin-API
// usage contained in the preview module"). It adds a reverse-proxy route on
// launch and removes it on teardown, idempotent both ways, against a CONFIGURED
// admin endpoint (composition-root config; dev default UNSET → routing disabled,
// recorded on the session state, R12). Routes are subdomain/port-based and
// NEVER path-prefix — a path matcher breaks arbitrary apps (Spec S13.8; R13).
//
// Host hazard: the endpoint is INJECTED config. Tests inject a local httptest
// fake; the LIVE host Caddy admin API (127.0.0.1:2019-class, fronting the
// production chain on 8481/8482) is NEVER dialed by this package's tests — the
// live wiring is pure config at bring-up.

// defaultCaddyServer is the Caddy server name whose routes previews attach to.
// STRUCTURAL config (composition-root override); the live server name is a
// bring-up config value, not an invented ⚙ key.
const defaultCaddyServer = "srv0"

// CaddyClient adds/removes preview routes over the Caddy admin API.
type CaddyClient struct {
	endpoint string // "" ⇒ routing disabled (dev default); no admin call is made
	server   string
	http     *http.Client
}

// NewCaddyClient returns a client for the configured admin endpoint. An empty
// endpoint disables routing (the module records the fact on the state and binds
// nothing — the honest dev posture, R12). server defaults to defaultCaddyServer.
func NewCaddyClient(endpoint, server string) *CaddyClient {
	if server == "" {
		server = defaultCaddyServer
	}
	return &CaddyClient{endpoint: endpoint, server: server, http: &http.Client{Timeout: 5 * time.Second}}
}

// Enabled reports whether an admin endpoint is configured (routing active).
func (c *CaddyClient) Enabled() bool { return c.endpoint != "" }

// buildRoute generates the port-based reverse-proxy route (Spec S13.8; R13):
// match on the preview HOST (subdomain), proxy to the loopback upstream (the
// allocated pool port), NEVER a path matcher. reverse_proxy forwards the
// WebSocket upgrade/Connection headers transparently (no header handler strips
// or overrides them) so Vite HMR survives. The `@id` makes the route
// addressable for idempotent add/remove.
func buildRoute(id, host, upstream string) map[string]any {
	return map[string]any{
		"@id":   id,
		"match": []any{map[string]any{"host": []any{host}}},
		"handle": []any{map[string]any{
			"handler":   "reverse_proxy",
			"upstreams": []any{map[string]any{"dial": upstream}},
		}},
	}
}

// AddRoute adds (idempotently) a preview route: it first removes any prior route
// with this id, then POSTs the new route to the server's routes array. A no-op
// when routing is disabled (R12).
func (c *CaddyClient) AddRoute(ctx context.Context, id, host, upstream string) error {
	if !c.Enabled() {
		return nil
	}
	if err := c.deleteID(ctx, id); err != nil {
		return err
	}
	body, err := json.Marshal(buildRoute(id, host, upstream))
	if err != nil {
		return err
	}
	url := c.endpoint + "/config/apps/http/servers/" + c.server + "/routes"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("preview: caddy add route: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("preview: caddy add route: status %d", resp.StatusCode)
	}
	return nil
}

// RemoveRoute removes a preview route, idempotently (a 404 is tolerated — the
// route was already gone). A no-op when routing is disabled (R12/R16).
func (c *CaddyClient) RemoveRoute(ctx context.Context, id string) error {
	if !c.Enabled() {
		return nil
	}
	return c.deleteID(ctx, id)
}

// deleteID issues DELETE /id/<id>, tolerating 404 (idempotency both ways, R12).
func (c *CaddyClient) deleteID(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.endpoint+"/id/"+id, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("preview: caddy delete route: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode/100 == 2 {
		return nil
	}
	return fmt.Errorf("preview: caddy delete route: status %d", resp.StatusCode)
}

// routeID is the addressable @id for a preview session's route.
func routeID(sessionID string) string { return "sinet-preview-" + sessionID }

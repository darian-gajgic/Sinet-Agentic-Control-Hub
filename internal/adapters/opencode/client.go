package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// The HTTP surface of `opencode serve` 1.18.3, as measured (S03.3 rule 4:
// behavior, never docs). Every request — including the health probe — carries
// the instance's HTTP Basic credentials: the server password is enforced on
// EVERY endpoint and `/api/health` is not exempt [SPIKE P2-S1]. The Basic
// USERNAME is part of the credential at this pin: `opencode`; any other
// username is refused 401 (live-recorded 2026-08-23 on the pinned binary — a
// delta from the spike's "-u <anyuser>:<pw>" note, honored as measured).
//
// HTTP-only origination (S03.4): every turn is originated here and no TUI is
// ever attached to a Sinet-owned serve — #16367's attach hang is orthogonal to
// this path (measured immune) but #36835 makes TUI-initiated asks unanswerable
// over REST.
//
// `POST /global/upgrade` is not reachable from this file, deliberately: the
// self-upgrade endpoint MUST never be exposed or called (S03.3 rule 2).

// Endpoint paths. The v1 event stream is the ONE the adapter subscribes to:
// the global v2 `/api/event` does not carry permission events in 1.18.3
// [SPIKE P2-S1 Probe 6-B].
const (
	pathEvents      = "/event"
	pathHealth      = "/api/health"
	pathSessions    = "/session"
	pathStatus      = "/session/status"
	pathPermissions = "/permission"
)

// requestTimeout bounds every unary call so a battery can never hang on a serve
// that stopped answering. Plain constant, the `cancelGrace` precedent.
const requestTimeout = 30 * time.Second

// Permission replies the adapter is allowed to send. `always` is absent BY
// CONSTRUCTION: v1 `always` rules are in-memory AND server-wide, so they leak
// approvals across a person's sessions and vanish on restart — persistent
// allow-policy is Sinet's own layer [SPIKE P2-S1 Probe 6-A].
const (
	replyOnce   = "once"
	replyReject = "reject"
)

type client struct {
	base     string
	password string
	hc       *http.Client
}

func newClient(inst Instance, hc *http.Client) *client {
	if hc == nil {
		// http.DefaultClient rather than a fresh one per session: it has no
		// client-level timeout (the event stream is long-lived and every unary
		// call carries its own deadline instead) and it shares one connection
		// pool, so a session cannot leak a transport.
		hc = http.DefaultClient
	}
	return &client{base: inst.BaseURL, password: inst.Password, hc: hc}
}

func (c *client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("opencode: marshal %s body: %w", path, err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(BasicAuthUser, c.password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// do runs one bounded unary call and decodes out when non-nil.
func (c *client) do(ctx context.Context, method, path string, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("opencode: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("opencode: read %s %s: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// The body is engine-authored; it is quoted, never interpreted, and the
		// instance password never appears in it.
		return fmt.Errorf("opencode: %s %s: HTTP %d: %s", method, path, resp.StatusCode, firstN(string(raw), 512))
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("opencode: decode %s %s: %w", method, path, err)
	}
	return nil
}

// events subscribes to the v1 SSE stream. No Last-Event-ID is ever sent: this
// engine IGNORES it (the fix was declined upstream), so claiming replay would
// be claiming a guarantee that does not exist — a reconnect reconciles by STATE
// instead (S14.5; R17).
func (c *client) events(ctx context.Context) (io.ReadCloser, error) {
	req, err := c.newRequest(ctx, http.MethodGet, pathEvents, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencode: subscribe %s: %w", pathEvents, err)
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("opencode: subscribe %s: HTTP %d: %s", pathEvents, resp.StatusCode, firstN(string(raw), 512))
	}
	return resp.Body, nil
}

// createSession creates the engine session and returns the id AS REPORTED by
// the engine — never an id the platform chose (S02.4b).
func (c *client) createSession(ctx context.Context) (string, string, error) {
	var out sessionInfo
	if err := c.do(ctx, http.MethodPost, pathSessions, map[string]any{}, &out); err != nil {
		return "", "", err
	}
	if out.ID == "" {
		return "", "", fmt.Errorf("opencode: %s returned no session id", pathSessions)
	}
	return out.ID, out.Directory, nil
}

type promptPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// promptBody is the `POST /session/{id}/prompt_async` body: per-message model
// ref and the agent selected BY NAME (1.18.3 accepts no inline agent
// definition; the definition rides the compiled config, S03.5).
type promptBody struct {
	Model  modelRef     `json:"model"`
	Agent  string       `json:"agent,omitempty"`
	System string       `json:"system,omitempty"`
	Parts  []promptPart `json:"parts"`
}

func (c *client) promptAsync(ctx context.Context, sessionID string, body promptBody) error {
	return c.do(ctx, http.MethodPost, pathSessions+"/"+sessionID+"/prompt_async", body, nil)
}

// permissions enumerates ALL entries of the flat pending array (S03.4: the
// pull-based enumeration, keyed by requestID).
func (c *client) permissions(ctx context.Context) ([]permissionRequest, error) {
	var out []permissionRequest
	if err := c.do(ctx, http.MethodGet, pathPermissions, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// reply answers one pending ask. The newer permission.reply endpoint is used
// rather than the legacy permission.respond because only this one carries the
// `message` that becomes the model's CorrectedError feedback (S03.4).
func (c *client) reply(ctx context.Context, requestID, reply, message string) error {
	if reply != replyOnce && reply != replyReject {
		// Structurally unreachable — the decision decoder emits only these two.
		// The guard is here because the value that must NEVER go out is one
		// typo away, and its blast radius is another person's session.
		return fmt.Errorf("opencode: refusing to send permission reply %q (only %q/%q are permitted, S03.4)",
			reply, replyOnce, replyReject)
	}
	body := map[string]string{"reply": reply}
	if message != "" {
		body["message"] = message
	}
	return c.do(ctx, http.MethodPost, pathPermissions+"/"+requestID+"/reply", body, nil)
}

// abort is the cooperative first rung of the S03.1 cancel ladder.
func (c *client) abort(ctx context.Context, sessionID string) error {
	return c.do(ctx, http.MethodPost, pathSessions+"/"+sessionID+"/abort", nil, nil)
}

// statuses reads the per-session status map (idle | busy | retry).
func (c *client) statuses(ctx context.Context) (map[string]sessionStatus, error) {
	var out map[string]sessionStatus
	if err := c.do(ctx, http.MethodGet, pathStatus, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// sessions lists the sessions the serve still holds. Sessions live in SQLite
// and survive a restart; pending asks do not (S03.4).
func (c *client) sessions(ctx context.Context) ([]sessionInfo, error) {
	var out []sessionInfo
	if err := c.do(ctx, http.MethodGet, pathSessions, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) health(ctx context.Context) error {
	var out struct {
		Healthy bool `json:"healthy"`
	}
	if err := c.do(ctx, http.MethodGet, pathHealth, nil, &out); err != nil {
		return err
	}
	if !out.Healthy {
		return fmt.Errorf("opencode: %s reported unhealthy", pathHealth)
	}
	return nil
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

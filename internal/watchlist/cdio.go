package watchlist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// cdio.go — the config-as-code driver for the ADOPTED changedetection.io
// page-diff organ (Spec S14.6 ¶T1; G2 Def.12). Integration is REST + config
// ONLY: no upstream code is vendored, patched or forked anywhere (adopt, never
// fork — S16.1/G2 D2.2).
//
// The integration splits at what the pinned REST surface can express: the
// per-watch config (url, title, `llm_intent`, `fetch_backend`,
// `include_filters`, `subtractive_selectors`) is driven from here, while the
// GLOBAL `LLMSettings.api_base`/`.model` that point the organ's native triage
// at Sinet's local endpoint have no route at 0.55.8 and are an operator
// install-time step (see LocalEndpointConfig).
//
// SINET'S WATCH ROWS ARE THE SOURCE OF TRUTH. Reconciliation drives the organ
// to match them: a watch Sinet owns but the organ lacks is created, one whose
// fields drifted is rewritten, and one the organ carries under Sinet's tag with
// no matching row is DELETED — so a hand-edit in the organ's own UI is
// reconciled away, never silently honored.
//
// HITS ARE POLLED, NOT PUSHED (coordinator disposition OQ2(b)): Sinet reads
// `GET /api/v1/watch` and `/difference/<from>/<to>` on its own loop, and the
// organ's apprise `json://` notifications are left unused. That adds no inbound
// route, no new secret and no new auth class; S14.3 already sanctions polling
// ("transport is a client detail, never an architecture fork"). The push
// ingress is PRE-REGISTERED as the upgrade — its shape is known (apprise 1.9.9
// Custom-JSON posts exactly {version,title,message,attachments,type}, with
// `:key=value` URL args merging extra keys into the same top-level object, and
// changedetection.io Jinja-renders {{watch_uuid}}/{{watch_url}}/{{diff}}/
// {{llm_intent}}/{{llm_summary}} into body, title AND url) — and it lands only
// when its auth class is ratified.
//
// THE HOST INSTALL IS A B5-GATE OPERATOR ACT: this package installs nothing,
// starts nothing, and changes no host state. Absence is honest — the page tier
// degrades to absent while the feed and API tiers keep working, and the organ
// surfaces as a `watchdog.organ_absence:watchlist` digest flag.

// Pin is the adopted changedetection.io version, test-coupled to the
// components.lock entry. An installed-vs-pin delta is reported LOUDLY and never
// silently retargeted (CONVENTIONS §10; S03.3 rule 4).
const Pin = "0.55.8"

// OrganName is the organ id used in the watchdog liveness seam; a down organ
// surfaces as `watchdog.organ_absence:watchlist`.
const OrganName = "watchlist"

// Discovery env overrides — structural deployment config passed through at the
// composition root (the SINET_SRT_PATH / SINET_PROMPTFOO_PATH precedent), not
// ⚙ keys. CDIOAPIKeyEnv holds a broker/engine-cred-class secret: it is read
// once into the client and is NEVER logged, never marshaled, and never placed
// on a struct that reaches a park record or the DB (D2/S11.5).
const (
	CDIOURLEnv    = "SINET_CDIO_URL"
	CDIOAPIKeyEnv = "SINET_CDIO_API_KEY"
)

// authHeader is the organ's API-key header at the pinned tag
// (changedetectionio/api/auth.py@0.55.8); a bad or missing key is HTTP 403.
const authHeader = "x-api-key"

// sinetTagTitle is the tag every Sinet-managed watch carries, so reconciliation
// can tell Sinet's watches from an operator's own and delete only its own.
const sinetTagTitle = "sinet"

// cdioTimeout bounds one organ call. Structural, not ⚙.
const cdioTimeout = 20 * time.Second

// CDIO is the REST client for the pinned organ API surface.
type CDIO struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewCDIO builds a client. baseURL is the organ's root (e.g.
// "http://127.0.0.1:5000"); an empty baseURL yields nil — the page tier is
// simply absent and every caller degrades honestly.
func NewCDIO(baseURL, apiKey string, client *http.Client) *CDIO {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	if client == nil {
		client = &http.Client{Timeout: cdioTimeout}
	}
	return &CDIO{baseURL: baseURL, apiKey: apiKey, client: client}
}

// DiscoverCDIO resolves the organ from the environment overrides. It returns
// nil when no base URL is configured — absence is the honest default, since the
// host install is a gate act.
func DiscoverCDIO(client *http.Client) *CDIO {
	return NewCDIO(os.Getenv(CDIOURLEnv), os.Getenv(CDIOAPIKeyEnv), client)
}

// SystemInfo is the `GET /api/v1/systeminfo` body at the pinned tag: exactly
// queue_size, overdue_watches, uptime, watch_count, version
// (changedetectionio/api/SystemInfo.py@0.55.8).
type SystemInfo struct {
	QueueSize      int      `json:"queue_size"`
	OverdueWatches []string `json:"overdue_watches"`
	Uptime         float64  `json:"uptime"`
	WatchCount     int      `json:"watch_count"`
	Version        string   `json:"version"`
}

// WatchSummary is one entry of the `GET /api/v1/watch` list. At the pinned tag
// the list is a JSON OBJECT keyed by watch uuid whose entries carry exactly
// nine allowlisted keys (changedetectionio/api/Watch.py@0.55.8) — not the full
// watch document.
type WatchSummary struct {
	LastChanged int64    `json:"last_changed"`
	LastChecked int64    `json:"last_checked"`
	LastError   any      `json:"last_error"`
	Link        string   `json:"link"`
	PageTitle   string   `json:"page_title"`
	Tags        []string `json:"tags"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Viewed      bool     `json:"viewed"`
}

// WatchSpec is the subset of the organ's WatchBase schema Sinet writes. Only
// keys the pinned schema declares appear here: `PUT /api/v1/watch/<uuid>`
// rejects an unknown field with 400 "Unknown field(s): …".
type WatchSpec struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
	// LLMIntent is the organ's NATIVE per-watch LLM triage instruction —
	// "Plain-English description of what the user cares about"
	// (changedetectionio/model/__init__.py@0.55.8 `llm_intent`, resolved
	// watch → tag → global by changedetectionio/llm/evaluator.py). Pointing the
	// organ's LLM settings at Sinet's local OpenAI-compatible endpoint
	// (LLMSettings.api_base + .model) makes this a FIRST-PASS "does this diff
	// matter?" triage only: it never replaces Sinet's own second pass and never
	// gates a card.
	LLMIntent string `json:"llm_intent,omitempty"`
	// IncludeFilters are CSS/XPath selectors narrowing the watch to the REGION
	// of interest — S14.6 ¶T1's "region-filtered verbatim diffs". The field is
	// WatchBase's own (`include_filters`, "CSS/XPath selectors to extract
	// content", changedetectionio/model/__init__.py@0.55.8).
	IncludeFilters []string `json:"include_filters,omitempty"`
	// SubtractiveSelectors REMOVE elements before the diff (WatchBase's
	// `subtractive_selectors`). Sinet applies a GENERIC page-chrome list to
	// every page watch — site-agnostic and safe, unlike an invented per-site
	// include filter.
	SubtractiveSelectors []string `json:"subtractive_selectors,omitempty"`
	// FetchBackend selects the fetcher: "system" (the organ's global default),
	// "html_requests" (plain HTTP), or "html_webdriver" (the browser leg). At
	// 0.55.8 the browser leg is an EXTERNAL CDP service selected on the organ
	// by PLAYWRIGHT_DRIVER_URL — the organ ships no browser and playwright is
	// not one of its Python dependencies — so requesting html_webdriver is a
	// configuration request, never a claim that a browser exists.
	FetchBackend string `json:"fetch_backend,omitempty"`
	// Tag is the legacy tag-TITLE string the create endpoint accepts and
	// translates into a tag uuid. Only sent on create.
	Tag string `json:"tag,omitempty"`
}

// Fetch-backend values at the pinned tag.
const (
	FetchBackendSystem  = "system"
	FetchBackendPlain   = "html_requests"
	FetchBackendBrowser = "html_webdriver"
)

func (c *CDIO) do(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("watchlist: marshal %s %s: %w", method, path, err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return 0, nil, fmt.Errorf("watchlist: build %s %s: %w", method, path, err)
	}
	req.Header.Set(authHeader, c.apiKey)
	req.Header.Set("User-Agent", UserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %s %s: %v", ErrOrganAbsent, method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("watchlist: read %s %s: %w", method, path, err)
	}
	return resp.StatusCode, raw, nil
}

// statusErr renders a non-success status. The organ's own error bodies are
// short strings; they never carry the API key, and the key is never echoed
// here.
func statusErr(method, path string, code int, raw []byte) error {
	detail := strings.TrimSpace(string(raw))
	if len(detail) > 200 {
		detail = detail[:200]
	}
	return fmt.Errorf("watchlist: %s %s: status %d: %s", method, path, code, detail)
}

// SystemInfo probes the organ. It is the R6/R8 liveness call: an error means
// the page tier is absent.
func (c *CDIO) SystemInfo(ctx context.Context) (SystemInfo, error) {
	code, raw, err := c.do(ctx, http.MethodGet, "/api/v1/systeminfo", nil)
	if err != nil {
		return SystemInfo{}, err
	}
	if code != http.StatusOK {
		return SystemInfo{}, statusErr(http.MethodGet, "/api/v1/systeminfo", code, raw)
	}
	var out SystemInfo
	if err := json.Unmarshal(raw, &out); err != nil {
		return SystemInfo{}, fmt.Errorf("watchlist: parse systeminfo: %w", err)
	}
	return out, nil
}

// ListWatches returns the organ's watches keyed by uuid.
func (c *CDIO) ListWatches(ctx context.Context) (map[string]WatchSummary, error) {
	code, raw, err := c.do(ctx, http.MethodGet, "/api/v1/watch", nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, statusErr(http.MethodGet, "/api/v1/watch", code, raw)
	}
	out := map[string]WatchSummary{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("watchlist: parse watch list: %w", err)
	}
	return out, nil
}

// CreateWatch creates a watch and returns its uuid. At the pinned tag success
// is 201 with body {"uuid": "…"}.
func (c *CDIO) CreateWatch(ctx context.Context, spec WatchSpec) (string, error) {
	code, raw, err := c.do(ctx, http.MethodPost, "/api/v1/watch", spec)
	if err != nil {
		return "", err
	}
	if code != http.StatusCreated {
		return "", statusErr(http.MethodPost, "/api/v1/watch", code, raw)
	}
	var out struct {
		UUID string `json:"uuid"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("watchlist: parse create-watch response: %w", err)
	}
	if out.UUID == "" {
		return "", fmt.Errorf("watchlist: create watch returned no uuid")
	}
	return out.UUID, nil
}

// UpdateWatch rewrites a watch's Sinet-owned fields. At the pinned tag success
// is 200 with the plain body "OK"; a missing watch is 404.
func (c *CDIO) UpdateWatch(ctx context.Context, uuid string, spec WatchSpec) error {
	spec.Tag = "" // create-only; PUT takes the tag membership as it stands
	path := "/api/v1/watch/" + url.PathEscape(uuid)
	code, raw, err := c.do(ctx, http.MethodPut, path, spec)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return statusErr(http.MethodPut, path, code, raw)
	}
	return nil
}

// DeleteWatch removes a watch. At the pinned tag success is 204 and a missing
// watch is 400 (not 404) — a delete of something already gone is treated as
// success so reconciliation is idempotent.
func (c *CDIO) DeleteWatch(ctx context.Context, uuid string) error {
	path := "/api/v1/watch/" + url.PathEscape(uuid)
	code, raw, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	if code == http.StatusNoContent || code == http.StatusBadRequest {
		return nil
	}
	return statusErr(http.MethodDelete, path, code, raw)
}

// Difference returns the rendered diff between two snapshots. At the pinned tag
// the response is text/plain (NOT JSON), and the path keywords `previous` /
// `latest` are accepted in place of timestamps.
func (c *CDIO) Difference(ctx context.Context, uuid, from, to string) (string, error) {
	path := fmt.Sprintf("/api/v1/watch/%s/difference/%s/%s",
		url.PathEscape(uuid), url.PathEscape(from), url.PathEscape(to))
	code, raw, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", statusErr(http.MethodGet, path, code, raw)
	}
	return string(raw), nil
}

// EnsureTag returns the uuid of Sinet's management tag, creating it when the
// organ does not carry it yet.
func (c *CDIO) EnsureTag(ctx context.Context) (string, error) {
	code, raw, err := c.do(ctx, http.MethodGet, "/api/v1/tags", nil)
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", statusErr(http.MethodGet, "/api/v1/tags", code, raw)
	}
	var tags map[string]struct {
		Title string `json:"title"`
		UUID  string `json:"uuid"`
	}
	if err := json.Unmarshal(raw, &tags); err != nil {
		return "", fmt.Errorf("watchlist: parse tag list: %w", err)
	}
	for id, t := range tags {
		if strings.EqualFold(t.Title, sinetTagTitle) {
			if t.UUID != "" {
				return t.UUID, nil
			}
			return id, nil
		}
	}
	code, raw, err = c.do(ctx, http.MethodPost, "/api/v1/tag", map[string]string{"title": sinetTagTitle})
	if err != nil {
		return "", err
	}
	if code != http.StatusCreated {
		return "", statusErr(http.MethodPost, "/api/v1/tag", code, raw)
	}
	var out struct {
		UUID string `json:"uuid"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("watchlist: parse create-tag response: %w", err)
	}
	return out.UUID, nil
}

// defaultSubtractiveSelectors is the generic page-chrome Sinet strips from
// every page watch before the diff (S14.6 ¶T1 region filtering, the
// noise-reduction half). These are site-AGNOSTIC element selectors, so applying
// them by default cannot silently hide the region being watched — unlike a
// guessed per-site include filter. Structural, not ⚙.
var defaultSubtractiveSelectors = []string{"nav", "header", "footer", "script", "style", "noscript", "svg"}

// RegionFilters returns the per-row include filters — the POSITIVE half of
// S14.6 ¶T1 region filtering, narrowing a watch to the region of interest.
//
// It is EMPTY for every seeded row at v0, deliberately and not by omission: a
// CSS/XPath selector is per-site tuning that can only be written against the
// page as it actually renders, and a guessed selector is worse than none —
// it would silently filter away the very pricing table the row exists to watch,
// and the watch would go quiet rather than fail loudly. The mechanism is live
// and reaches the organ; the selectors are operator data supplied through the
// R3 override at bring-up, when the pages can be inspected. The generic
// subtractive list above carries the noise reduction until then.
func RegionFilters(r Row) []string { return nil }

// specFor renders the organ-side config for one page row. The escalation
// ladder's second rung is expressed here as configuration: a row escalated past
// DecayPlain asks the organ for its browser backend.
func specFor(r Row) WatchSpec {
	backend := FetchBackendPlain
	if r.DecayStage >= DecayEscalated {
		backend = FetchBackendBrowser
	}
	return WatchSpec{
		URL:                  r.URL,
		Title:                r.ID,
		LLMIntent:            r.watchIntent(),
		FetchBackend:         backend,
		IncludeFilters:       RegionFilters(r),
		SubtractiveSelectors: defaultSubtractiveSelectors,
	}
}

// LocalEndpointConfig is the organ-side configuration S14.6 ¶T1 requires for
// "its native LLM rules pointed at Sinet's local OpenAI-compatible endpoint",
// stated as DATA so the B5 gate proposal carries something concrete.
//
// It is NOT settable over REST, and that is a verified property of the pin, not
// an omission: at tag 0.55.8 `changedetectionio/flask_app.py` registers exactly
// thirteen `add_resource` routes (watch, watch/<uuid>, history, single-history,
// difference, favicon, tags, tag, search, import, notifications, systeminfo,
// full-spec) and NONE of them reads or writes application settings — a grep of
// that file for `LLMSettings`, `api_base` or `application_settings` matches
// nothing. `LLMSettings.api_base` / `.model` are GLOBAL organ settings written
// through its own settings UI or datastore file.
//
// So Sinet drives the per-watch half over REST (LLMIntent above, WatchBase's
// `llm_intent`) and the endpoint half is an operator install-time step. The
// per-watch intent is what makes the first pass meaningful; without the
// endpoint configured the organ simply runs no LLM triage, Sinet's own second
// pass is unaffected, and nothing silently degrades.
type LocalEndpointConfig struct {
	// APIBase is LLMSettings.api_base — Sinet's local OpenAI-compatible
	// endpoint (the llama-swap front, loopback).
	APIBase string
	// Model is LLMSettings.model — a duty-alias-shaped seat name.
	Model string
}

// RequiredLocalEndpointConfig renders the organ-side settings the B5-gate
// install must apply for the S14.6 ¶T1 first-pass triage to run.
func RequiredLocalEndpointConfig(apiBase, model string) LocalEndpointConfig {
	return LocalEndpointConfig{APIBase: apiBase, Model: model}
}

// watchIntent is the plain-English first-pass triage instruction handed to the
// organ's native LLM rules. It states the lane at stake so the organ's cheap
// pass can drop cosmetic diffs before Sinet's own second pass ever sees them.
func (r Row) watchIntent() string {
	subject := r.Lane
	if subject == "" {
		subject = "an AI provider Sinet tracks"
	}
	return "Report a change only when it affects pricing, plan terms, rate limits, " +
		"available models, API endpoints, or billing regime for " + subject +
		". Ignore navigation, styling, marketing copy, testimonials and dates."
}

// ReconcileReport summarizes one reconciliation pass.
type ReconcileReport struct {
	Created int
	Updated int
	Deleted int
	TagID   string
}

// Reconcile drives the organ to match Sinet's page rows (config-as-code). It is
// idempotent: a row already carrying a live external ref is updated in place, a
// row whose ref is stale re-binds to a matching remote watch or creates a new
// one, and every Sinet-tagged remote watch without a matching row is deleted.
//
// The organ's create endpoint does NOT detect duplicate urls, so re-binding by
// url before creating is what keeps a restart from growing the watch set.
func (c *CDIO) Reconcile(ctx context.Context, store *Store, rows []Row) (ReconcileReport, error) {
	var rep ReconcileReport
	tagID, err := c.EnsureTag(ctx)
	if err != nil {
		return rep, err
	}
	rep.TagID = tagID

	remote, err := c.ListWatches(ctx)
	if err != nil {
		return rep, err
	}
	byURL := make(map[string]string, len(remote))
	for uuid, w := range remote {
		byURL[w.URL] = uuid
	}

	wanted := make(map[string]bool, len(rows))
	for _, r := range rows {
		spec := specFor(r)
		uuid := r.ExternalRef
		if _, live := remote[uuid]; uuid == "" || !live {
			uuid = byURL[r.URL]
		}
		if uuid == "" {
			spec.Tag = sinetTagTitle
			newUUID, err := c.CreateWatch(ctx, spec)
			if err != nil {
				return rep, err
			}
			uuid, rep.Created = newUUID, rep.Created+1
		} else {
			if err := c.UpdateWatch(ctx, uuid, spec); err != nil {
				return rep, err
			}
			rep.Updated++
		}
		wanted[uuid] = true
		if r.ExternalRef != uuid {
			if err := store.SetExternalRef(ctx, r.ID, uuid); err != nil {
				return rep, err
			}
		}
	}

	// A Sinet-tagged remote watch with no matching row is Sinet's own stale
	// state or a hand-edit in the organ's UI: it is reconciled away, never
	// orphaned. Watches the operator created outside Sinet's tag are untouched.
	for uuid, w := range remote {
		if wanted[uuid] || !hasTag(w.Tags, tagID) {
			continue
		}
		if err := c.DeleteWatch(ctx, uuid); err != nil {
			return rep, err
		}
		rep.Deleted++
	}
	return rep, nil
}

func hasTag(tags []string, id string) bool {
	for _, t := range tags {
		if t == id {
			return true
		}
	}
	return false
}

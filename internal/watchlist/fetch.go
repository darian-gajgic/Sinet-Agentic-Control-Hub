package watchlist

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// fetch.go — the Sinet-native conditional-GET fetcher shared by the feed
// poller (T2/T4) and the aggregator jobs (T3). stdlib only: no Go module is
// added by this packet (CONVENTIONS §2 stdlib-first).

// UserAgent identifies Sinet's poller to every origin it touches. An explicit,
// contactable agent string is basic politeness on a public-feed poller and is
// what lets an origin rate-limit us specifically rather than opaquely.
const UserAgent = "sinet-watchlist/1 (+https://github.com/darian-gajgic/Sinet-Agentic-Control-Hub)"

// Fetch bounds. Structural constants (S18 ratifies no key — the §7
// sseBatchSize precedent): a watch source that needs more than this is a
// decayed source, not a bigger buffer.
const (
	// fetchTimeout bounds one request end to end.
	fetchTimeout = 30 * time.Second
	// maxBodyBytes bounds a response read. models.dev's api.json and the
	// genai-prices data.json are the largest documents in the seed set at a
	// few hundred KiB; 8 MiB leaves two orders of headroom and still refuses a
	// runaway body.
	maxBodyBytes = 8 << 20
	// maxRedirects bounds redirect following.
	maxRedirects = 5
)

// ErrBodyTooLarge: the response exceeded maxBodyBytes. It is a fetch failure
// (it feeds the fail streak), never a panic and never fatal.
var ErrBodyTooLarge = errors.New("watchlist: response body exceeded the fetch cap")

// Fetcher performs conditional GETs. The zero value is unusable; use
// NewFetcher.
type Fetcher struct {
	client *http.Client
}

// NewFetcher returns a fetcher over client; a nil client gets one with the
// structural timeout and a bounded redirect chain.
func NewFetcher(client *http.Client) *Fetcher {
	if client == nil {
		client = &http.Client{
			Timeout: fetchTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("watchlist: stopped after %d redirects", maxRedirects)
				}
				return nil
			},
		}
	}
	return &Fetcher{client: client}
}

// Response is one conditional-GET outcome.
type Response struct {
	// NotModified is true for a 304 — the cheap no-change path: no body was
	// transferred and the stored validators still stand.
	NotModified bool
	// Unchanged is true when a 200 body hashed identically to the stored
	// content hash — the second dedup layer, for origins that emit no
	// validators.
	Unchanged bool
	Body      []byte
	// ContentHash is sha256 over the NORMALIZED body (CRLF folded, trailing
	// whitespace trimmed) so a cosmetic line-ending change is not a hit.
	ContentHash  string
	ETag         string
	LastModified string
	StatusCode   int
}

// Get performs a conditional GET against url, replaying the row's stored
// validators as If-None-Match / If-Modified-Since. A non-2xx, non-304 status
// is an error (it feeds the fail streak).
func (f *Fetcher) Get(ctx context.Context, url string, prev FetchState) (Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Response{}, fmt.Errorf("watchlist: build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", UserAgent)
	// Accept-Encoding is deliberately NOT set here, and nothing downstream
	// decompresses. Setting it by hand disables net/http's transparent gzip
	// negotiation: the transport then hands back the raw compressed body and
	// every parser downstream sees gzip magic instead of XML or JSON. Left
	// unset, the transport ADDS `Accept-Encoding: gzip` itself on every request
	// it sends (it does so whenever the caller set neither Accept-Encoding nor
	// Range on a non-HEAD request), decompresses the answer, and strips
	// Content-Encoding. So a response reaching this function is never
	// compressed — an origin that compresses "unsolicited" is answering a
	// header the transport sent on our behalf, which is the same negotiated
	// path. Anything that hand-rolls a gunzip here would be code no request
	// through this transport can reach.
	if prev.ETag != "" {
		req.Header.Set("If-None-Match", prev.ETag)
	}
	if prev.LastModified != "" {
		req.Header.Set("If-Modified-Since", prev.LastModified)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("watchlist: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	out := Response{
		StatusCode:   resp.StatusCode,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}
	if out.ETag == "" {
		out.ETag = prev.ETag
	}
	if out.LastModified == "" {
		out.LastModified = prev.LastModified
	}

	if resp.StatusCode == http.StatusNotModified {
		out.NotModified = true
		out.ContentHash = prev.ContentHash
		return out, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return out, fmt.Errorf("watchlist: fetch %s: status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return out, fmt.Errorf("watchlist: read %s: %w", url, err)
	}
	if len(body) > maxBodyBytes {
		return out, fmt.Errorf("%w: %s", ErrBodyTooLarge, url)
	}
	out.Body = body
	out.ContentHash = contentHash(body)
	out.Unchanged = prev.ContentHash != "" && out.ContentHash == prev.ContentHash
	return out, nil
}

// contentHash is sha256 over the normalized body: CRLF folded to LF and
// trailing whitespace trimmed, so a cosmetic line-ending or trailing-newline
// change never reads as a content change.
func contentHash(body []byte) string {
	norm := strings.TrimRight(strings.ReplaceAll(string(body), "\r\n", "\n"), " \t\n")
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}

// shortHash is a stable short digest used for per-entry identity and incident
// fingerprints.
func shortHash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])[:16]
}

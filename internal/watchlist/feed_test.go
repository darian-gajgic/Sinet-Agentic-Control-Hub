package watchlist_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/watchlist"
)

const atomBody = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:media="http://search.yahoo.com/mrss/">
  <title>anomalyco/opencode Releases</title>
  <updated>2026-07-26T10:00:00Z</updated>
  <entry>
    <id>tag:github.com,2008:Repository/1/v2.4.0</id>
    <title>v2.4.0</title>
    <updated>2026-07-25T09:00:00Z</updated>
    <link rel="alternate" type="text/html" href="https://example.invalid/releases/v2.4.0"/>
    <content type="html">Anthropic OAuth handling changed.</content>
    <media:thumbnail url="https://example.invalid/x.png"/>
    <somethingUnknown>ignored by a forward-tolerant parser</somethingUnknown>
  </entry>
  <entry>
    <id>tag:github.com,2008:Repository/1/v2.3.0</id>
    <title>v2.3.0</title>
    <updated>2026-07-20T09:00:00Z</updated>
    <link rel="alternate" href="https://example.invalid/releases/v2.3.0"/>
    <summary>Routine release.</summary>
  </entry>
</feed>`

const rssBody = `<?xml version="1.0"?>
<rss version="2.0"><channel>
  <title>r/LocalLLaMA</title>
  <item>
    <guid>t3_abc</guid>
    <title>New quant lands</title>
    <link>https://example.invalid/r/1</link>
    <pubDate>Sat, 25 Jul 2026 09:00:00 GMT</pubDate>
    <description>A description.</description>
  </item>
</channel></rss>`

// TestParseFeedIsForwardTolerant: unknown elements and namespaces are ignored;
// both dialects collapse onto one Entry shape.
func TestParseFeedIsForwardTolerant(t *testing.T) {
	title, entries, err := watchlist.ParseFeed([]byte(atomBody), "atom")
	if err != nil {
		t.Fatalf("ParseFeed(atom): %v", err)
	}
	if title != "anomalyco/opencode Releases" || len(entries) != 2 {
		t.Fatalf("atom: title=%q entries=%d", title, len(entries))
	}
	if entries[0].Title != "v2.4.0" || entries[0].Link != "https://example.invalid/releases/v2.4.0" {
		t.Errorf("atom entry 0 = %+v", entries[0])
	}
	if !strings.Contains(entries[0].Summary, "OAuth handling changed") {
		t.Errorf("atom content did not become the summary: %q", entries[0].Summary)
	}

	if _, entries, err = watchlist.ParseFeed([]byte(rssBody), "rss"); err != nil {
		t.Fatalf("ParseFeed(rss): %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "t3_abc" {
		t.Fatalf("rss entries = %+v", entries)
	}

	// A wrong or missing hint still parses (the dialect is sniffed).
	if _, e, err := watchlist.ParseFeed([]byte(rssBody), ""); err != nil || len(e) != 1 {
		t.Errorf("hintless parse failed: %d entries, err %v", len(e), err)
	}
	if _, e, err := watchlist.ParseFeed([]byte(atomBody), "rss"); err == nil && len(e) > 0 {
		t.Error("an atom body parsed as rss produced entries — the root-element check should refuse it")
	}
}

// TestMalformedFeedIsAFetchFailureNotAPanic (acceptance item 16).
func TestMalformedFeedIsAFetchFailureNotAPanic(t *testing.T) {
	for name, body := range map[string]string{
		"truncated xml": `<feed><entry><title>oops`,
		"not xml":       `{"this":"is json"}`,
		"empty":         ``,
		"wrong root":    `<html><body>a page, not a feed</body></html>`,
	} {
		if _, _, err := watchlist.ParseFeed([]byte(body), ""); err == nil {
			t.Errorf("%s parsed as a feed — a malformed feed must be an error", name)
		}
	}
}

// TestConditionalGETReplaysValidators is acceptance item 16: the poller sends
// If-None-Match / If-Modified-Since from stored state and treats 304 as
// no-change without transferring a body.
func TestConditionalGETReplaysValidators(t *testing.T) {
	var gotINM, gotIMS, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotINM, gotIMS, gotUA = r.Header.Get("If-None-Match"), r.Header.Get("If-Modified-Since"), r.Header.Get("User-Agent")
		if gotINM == `W/"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `W/"v1"`)
		w.Header().Set("Last-Modified", "Sat, 25 Jul 2026 09:00:00 GMT")
		_, _ = w.Write([]byte(atomBody))
	}))
	defer srv.Close()

	f := watchlist.NewFetcher(srv.Client())
	first, err := f.Get(context.Background(), srv.URL, watchlist.FetchState{})
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if first.NotModified || first.ETag != `W/"v1"` || len(first.Body) == 0 {
		t.Fatalf("first response = %+v", first)
	}
	if gotUA == "" || !strings.HasPrefix(gotUA, "sinet-watchlist/") {
		t.Errorf("the poller sent User-Agent %q — an explicit, contactable agent is required", gotUA)
	}

	second, err := f.Get(context.Background(), srv.URL, watchlist.FetchState{
		ETag: first.ETag, LastModified: first.LastModified, ContentHash: first.ContentHash,
	})
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if gotINM != `W/"v1"` || gotIMS != "Sat, 25 Jul 2026 09:00:00 GMT" {
		t.Errorf("validators not replayed: If-None-Match=%q If-Modified-Since=%q", gotINM, gotIMS)
	}
	if !second.NotModified {
		t.Error("a 304 must read as NotModified — the cheap no-change path")
	}
	if len(second.Body) != 0 {
		t.Error("a 304 transferred a body")
	}
}

// TestContentHashDedupSuppressesAByteIdenticalRefetch is acceptance item 16: an
// origin that emits no validators still gets deduped by content hash, and a
// cosmetic line-ending change is not a hit.
func TestContentHashDedupSuppressesAByteIdenticalRefetch(t *testing.T) {
	body := atomBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	f := watchlist.NewFetcher(srv.Client())
	first, err := f.Get(context.Background(), srv.URL, watchlist.FetchState{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Unchanged {
		t.Error("a first fetch cannot be Unchanged")
	}
	second, err := f.Get(context.Background(), srv.URL, watchlist.FetchState{ContentHash: first.ContentHash})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Unchanged {
		t.Error("a byte-identical refetch was not deduped by content hash")
	}

	// CRLF + a trailing newline normalize away.
	body = strings.ReplaceAll(atomBody, "\n", "\r\n") + "\n"
	third, err := f.Get(context.Background(), srv.URL, watchlist.FetchState{ContentHash: first.ContentHash})
	if err != nil {
		t.Fatal(err)
	}
	if !third.Unchanged {
		t.Error("a CRLF/trailing-newline-only change read as a content change")
	}
}

// TestNon2xxIsAFetchFailure: an error status feeds the fail streak rather than
// being mistaken for an empty document.
func TestNon2xxIsAFetchFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer srv.Close()
	if _, err := watchlist.NewFetcher(srv.Client()).Get(context.Background(), srv.URL, watchlist.FetchState{}); err == nil {
		t.Error("a 410 was not a fetch failure")
	}
}

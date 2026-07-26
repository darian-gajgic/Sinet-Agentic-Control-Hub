package watchlist

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// feed.go — Atom + RSS parsing on `encoding/xml`, FORWARD-TOLERANT: unknown
// elements are ignored, a malformed feed is a fetch failure and never a panic
// and never fatal (the S03.1 stream discipline applied to feeds). Zero new Go
// modules.

// Entry is one normalized feed item. Atom and RSS collapse onto this shape so
// the classifier and the drift emitter never branch on feed dialect.
type Entry struct {
	// ID is the entry's own stable identifier (`<id>` / `<guid>`); empty when
	// the feed publishes none.
	ID string
	// Title / Link / Published are the human-facing fields the second pass
	// classifies and the card renders.
	Title     string
	Link      string
	Published string
	// Summary is the entry body excerpt, bounded at entrySummaryCap.
	Summary string
}

// entrySummaryCap bounds the excerpt carried off a feed entry into the
// classifier prompt and the finding payload (refs-not-blobs, P-T07-5).
// Structural, not ⚙.
const entrySummaryCap = 2000

// Hash is the per-entry content hash used for dedup: an entry already reported
// for a row is not re-raised when the body hash moves for an unrelated reason.
// It keys on the publisher's own identity when there is one, and on the visible
// content otherwise.
func (e Entry) Hash() string {
	if e.ID != "" {
		return shortHash(e.ID)
	}
	return shortHash(e.Title, e.Link, e.Published)
}

// atomFeed / rssFeed are the two parsed dialects. Every field is optional:
// encoding/xml silently ignores elements with no matching field, which is
// exactly the forward-tolerance the spec demands.
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID        string     `xml:"id"`
	Title     string     `xml:"title"`
	Updated   string     `xml:"updated"`
	Published string     `xml:"published"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Links     []atomLink `xml:"link"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	GUID        string `xml:"guid"`
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
}

// ParseFeed decodes an Atom or RSS body into normalized entries. hint is the
// row's parser-hint (`atom` / `rss` / ""); an empty or unrecognized hint tries
// Atom first, then RSS, so a mis-hinted row still parses. A body that is
// neither is an error — a fetch failure, never a panic.
func ParseFeed(body []byte, hint string) (title string, entries []Entry, err error) {
	switch strings.ToLower(strings.TrimSpace(hint)) {
	case "atom":
		return parseAtom(body)
	case "rss":
		return parseRSS(body)
	}
	if t, e, err := parseAtom(body); err == nil {
		return t, e, nil
	}
	t, e, rssErr := parseRSS(body)
	if rssErr != nil {
		return "", nil, fmt.Errorf("watchlist: body parses as neither Atom nor RSS: %w", rssErr)
	}
	return t, e, nil
}

func parseAtom(body []byte) (string, []Entry, error) {
	var f atomFeed
	if err := xml.Unmarshal(body, &f); err != nil {
		return "", nil, fmt.Errorf("watchlist: parse atom: %w", err)
	}
	if f.XMLName.Local != "feed" {
		return "", nil, fmt.Errorf("watchlist: parse atom: root element is %q, not feed", f.XMLName.Local)
	}
	out := make([]Entry, 0, len(f.Entries))
	for _, e := range f.Entries {
		published := e.Updated
		if published == "" {
			published = e.Published
		}
		summary := e.Summary
		if summary == "" {
			summary = e.Content
		}
		out = append(out, Entry{
			ID:        strings.TrimSpace(e.ID),
			Title:     strings.TrimSpace(e.Title),
			Link:      atomHref(e.Links),
			Published: strings.TrimSpace(published),
			Summary:   excerpt(summary),
		})
	}
	return strings.TrimSpace(f.Title), out, nil
}

func parseRSS(body []byte) (string, []Entry, error) {
	var f rssFeed
	if err := xml.Unmarshal(body, &f); err != nil {
		return "", nil, fmt.Errorf("watchlist: parse rss: %w", err)
	}
	if f.XMLName.Local != "rss" {
		return "", nil, fmt.Errorf("watchlist: parse rss: root element is %q, not rss", f.XMLName.Local)
	}
	out := make([]Entry, 0, len(f.Channel.Items))
	for _, it := range f.Channel.Items {
		out = append(out, Entry{
			ID:        strings.TrimSpace(it.GUID),
			Title:     strings.TrimSpace(it.Title),
			Link:      strings.TrimSpace(it.Link),
			Published: strings.TrimSpace(it.PubDate),
			Summary:   excerpt(it.Description),
		})
	}
	return strings.TrimSpace(f.Channel.Title), out, nil
}

// atomHref picks an entry's canonical link: the alternate link when the feed
// labels one, else the first.
func atomHref(links []atomLink) string {
	for _, l := range links {
		if l.Rel == "alternate" || l.Rel == "" {
			return strings.TrimSpace(l.Href)
		}
	}
	if len(links) > 0 {
		return strings.TrimSpace(links[0].Href)
	}
	return ""
}

func excerpt(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > entrySummaryCap {
		return s[:entrySummaryCap]
	}
	return s
}

package push

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// send.go composes the one payload shape this platform sends and writes it to a
// stored subscription endpoint. It is the ONLY outbound-HTTP path in the
// package, and the endpoint it POSTs to always comes from a row in
// push_subscriptions — there is no other absolute URL here (S01.8 register
// row 2; the negative is scanned by push_wall_test.go).
//
// ONE COMPOSER, TWO RENDERERS. The body is the W3C Push API §3.3 declarative
// push message. On Safari/iOS ≥18.4 the user agent renders it directly with no
// service worker involved; everywhere else the same JSON arrives as the payload
// of the service worker's `push` event and web/public/sw.js renders it. That is
// the backwards compatibility the format was designed for, so the platform
// composes once and never branches on the recipient's browser — which it could
// not do anyway, since a subscription says nothing about what rendered it.

// The declarative push message (W3C Push API §3.3, Working Draft 2025-12-01,
// re-verified 2026-07-30). `web_push` MUST be the integer 8030 — it is the
// RFC 8030 disambiguator that tells a user agent this JSON is a notification
// and not somebody's application payload — and `notification` MUST carry both
// `title` and `navigate`.
//
// `mutable` is deliberately ABSENT. Its documented effect is to dispatch a push
// event to a service worker instead of rendering directly; the platform wants
// the direct render where it is available, and the classic leg gets the event
// regardless because those browsers have no declarative path at all.
const webPushDisambiguator = 8030

type declarativeMessage struct {
	WebPush      int                 `json:"web_push"`
	Notification declarativeNotifier `json:"notification"`
}

type declarativeNotifier struct {
	Title    string `json:"title"`
	Navigate string `json:"navigate"`
	// Icon is a self-hosted asset on the platform's own origin. Nothing here
	// reaches a third party (S01.8).
	Icon string `json:"icon,omitempty"`
	// Tag coalesces on the device: a re-nag REPLACES the notification already on
	// screen for the same decision rather than stacking a second copy.
	Tag string `json:"tag,omitempty"`
	// AppBadge is the identity's pending-approvals count as the home-screen
	// badge (S15.11). It is a STRING because that is the shape WebKit ships —
	// its own published example is `"app_badge": "1"` (webkit.org/blog/16535,
	// re-verified 2026-07-30) — and it is a WebKit EXTENSION: the W3C Working
	// Draft's notification member list does not contain it at all (checked
	// against the full spec text, same date). A browser that does not know the
	// member ignores it, and the classic leg sets the badge through
	// navigator.setAppBadge from this same field.
	AppBadge string `json:"app_badge"`
}

// Message is one decision, ready to be sealed to a device. The navigate PATH is
// carried rather than a URL: `navigate` must be absolute, and the absolute form
// differs per subscription because each row records the origin its own device
// enrolled from (B6-9 OQ7).
type Message struct {
	// CardID is the routable inbox id ("<kind>:<native id>"), used as the audit
	// ref and as the coalescing Topic. It is NEVER put in the notification text.
	CardID string
	// Class is the S07.7 SLA class this send is due under: "approval" or
	// "safety". It decides the Urgency header and the TTL.
	Class string
	// Title is content-light platform vocabulary. No string lifted from a card
	// snapshot reaches it — see composePayload's own assertion in the tests.
	Title string
	// Path is the deep link, composed by the caller to byte-match what the
	// SPA's own hrefFor produces for this card.
	Path string
	// Badge is the identity's pending count (B6-9 OQ5).
	Badge int
	// IconPath is the self-hosted notification icon.
	IconPath string
	// TTL is how long the push service may hold the message. It is the class's
	// own cadence: a message that could not be delivered before the next one is
	// due has been superseded, and holding it would deliver a stale nag after a
	// fresh one.
	TTL time.Duration
}

// Urgency is the RFC 8030 header value for this message's class. Apple accepts
// exactly {very-low, low, normal, high} and refuses anything else with
// BadUrgency (re-verified 2026-07-30).
func (m Message) Urgency() string {
	if m.Class == ClassSafety {
		return "high"
	}
	return "normal"
}

// The two SLA classes, as the ratified G1 Def.3 set names them.
const (
	ClassApproval = "approval"
	ClassSafety   = "safety"
)

// Doer is the outbound HTTP seam. Production is an *http.Client; every test
// passes a fake push service, so no test ever opens a non-loopback connection.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Sender writes messages to subscriptions and audits every attempt.
type Sender struct {
	store *Store
	http  Doer
	// contact is the RFC 8292 `sub` claim. See vapidSub.
	tokens sync.Mutex
	cached map[string]cachedToken
}

type cachedToken struct {
	jwt    string
	expiry time.Time
}

// NewSender builds the send path. `doer` nil = an http.Client bounded by
// SendTimeout.
func NewSender(store *Store, doer Doer) *Sender {
	if doer == nil {
		doer = &http.Client{Timeout: SendTimeout}
	}
	return &Sender{store: store, http: doer, cached: map[string]cachedToken{}}
}

// Result is one send attempt's recorded outcome.
type Result struct {
	Outcome string
	Status  int
	Detail  string
}

// Send seals one message to one subscription, writes it to that subscription's
// endpoint, and audits the attempt. It returns the outcome rather than an error
// for a refusal: a push service saying no is a recorded fact about that device,
// not a failure of the evaluation that produced the message.
//
// A 404 or 410 means the subscription no longer exists at the push service, so
// the row is REMOVED with its own `push.unsubscribed` event (B6-9 OQ2). Every
// other failure is audited and left alone — nothing here retries, because a
// retry loop against a service that is refusing is how a platform turns one
// problem into a rate-limit ban.
func (s *Sender) Send(ctx context.Context, sub Subscription, msg Message) Result {
	res := s.attempt(ctx, sub, msg)
	// THE ONE BOUNDARY WHERE A DETAIL BECOMES A RECORD. Everything past this
	// line — the audit payload and the value handed back to the caller — is
	// scrubbed, because two of the three ways a detail is produced can carry the
	// endpoint: `*url.Error` from http.Client.Do ALWAYS embeds the full URL it
	// was given, and a push service is free to echo the request URL back in its
	// own error body — with or without the scheme. An endpoint is a capability,
	// so it is scrubbed here rather than at each producer, where the next
	// producer would have to remember; and this is the one place that knows
	// WHICH endpoint the detail was produced against.
	res.Detail = scrubDetail(res.Detail, sub.Endpoint)
	if res.Outcome == OutcomeGone {
		if err := s.store.removeDead(ctx, sub); err != nil {
			s.store.logger.Warn("push: remove dead subscription", "subscription", sub.ID, "err", err)
		}
	}
	if err := s.store.recordSend(ctx, sub, msg, res); err != nil {
		s.store.logger.Error("push: audit send", "subscription", sub.ID, "err", err)
	}
	return res
}

func (s *Sender) attempt(ctx context.Context, sub Subscription, msg Message) Result {
	body, err := composePayload(sub, msg)
	if err != nil {
		return Result{Outcome: OutcomeRefused, Detail: err.Error()}
	}
	sealed, err := Encrypt(sub.Keys, body, nil)
	if err != nil {
		// Unusable key material is a permanent property of the row, so it is
		// reported as such rather than retried every evaluation forever.
		return Result{Outcome: OutcomeRefused, Detail: err.Error()}
	}
	if len(sealed) > MaxPayloadBytes {
		// Refused, never truncated: a shortened ciphertext does not decrypt, so
		// the device would get nothing and the log would say it was sent.
		return Result{Outcome: OutcomeRefused,
			Detail: fmt.Sprintf("sealed payload is %d bytes, over the %d-byte push-service limit", len(sealed), MaxPayloadBytes)}
	}
	auth, err := s.authorization(sub.Endpoint, sub.Origin)
	if err != nil {
		return Result{Outcome: OutcomeRefused, Detail: err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(sealed))
	if err != nil {
		return Result{Outcome: OutcomeRefused, Detail: err.Error()}
	}
	req.Header.Set("Content-Encoding", "aes128gcm")
	// The WebKit explainer's disambiguator: "Push messages received without that
	// content type are considered to have a legacy disposition." It is a
	// PROPOSAL rather than spec text (the W3C Working Draft names no
	// Content-Type at all, checked 2026-07-30), and it is sent because it is the
	// documented interop lever, not because it is normative.
	req.Header.Set("Content-Type", "application/notification+json")
	req.Header.Set("TTL", ttlHeader(msg.TTL))
	req.Header.Set("Urgency", msg.Urgency())
	req.Header.Set("Topic", topicHeader(msg.CardID))
	req.Header.Set("Authorization", auth)

	resp, err := s.http.Do(req)
	if err != nil {
		return Result{Outcome: OutcomeUnreachable, Detail: err.Error()}
	}
	defer resp.Body.Close()
	detail := readServiceReason(resp.Body)

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return Result{Outcome: OutcomeSent, Status: resp.StatusCode}
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return Result{Outcome: OutcomeGone, Status: resp.StatusCode, Detail: detail}
	default:
		return Result{Outcome: OutcomeRefused, Status: resp.StatusCode, Detail: detail}
	}
}

// serviceReasonCap bounds what a push service's error body may write into this
// platform's own audit log. The reason is worth keeping — Apple's
// `BadJwtToken` / `BadVapidPublicKey` strings are the whole diagnosis when a
// drill fails — but it is a third party's text arriving in our record, so it is
// bounded rather than trusted to be small.
const serviceReasonCap = 200

// urlShaped matches any scheme-bearing URL. It is deliberately greedy about the
// tail — a push endpoint's secret is its PATH, so stopping at the host would
// leave the capability intact.
var urlShaped = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.\-]*://[^\s"'` + "`" + `]*`)

// scrubDetail makes a free-text failure reason safe to record.
//
// It removes every URL and bounds the length. The URL removal is the
// load-bearing half: a transport failure is the ORDINARY shape of a failed send
// (a sleeping host, a dropped tailnet), and `*url.Error.Error()` renders as
// `Post "https://…/<capability path>": dial tcp …` — so without this the
// commonest failure would write the one value migration 0021, OQ2, OQ9 and this
// package's own doc comment all say never leaves. What survives is the
// classification a drill actually needs ("dial tcp: connection refused", or the
// service's own `BadJwtToken`), which names no capability.
//
// A SCHEME IS NOT WHAT MAKES IT A CAPABILITY (drain r2, R2). `urlShaped`
// requires `://`, so a service echoing the request back as
// `"path":"/push/<token>"` or as `host/path` wrote the same secret into the row
// with the scheme filed off. The host is discoverable — it is a vendor relay —
// so a bare path in an audit row is a reconstructable capability. The endpoint
// this attempt was made against is the one literal this function can recognise
// without guessing, and `Send` holds it, so it is passed in and its scheme-less
// spellings are removed too. A detail from any other source is unaffected: the
// removal is by exact match, not by shape.
func scrubDetail(detail, endpoint string) string {
	if detail == "" {
		return ""
	}
	out := urlShaped.ReplaceAllString(detail, endpointWithheld)
	for _, shape := range endpointShapes(endpoint) {
		out = strings.ReplaceAll(out, shape, endpointWithheld)
	}
	out = strings.TrimSpace(out)
	if len([]rune(out)) > serviceReasonCap {
		out = string([]rune(out)[:serviceReasonCap]) + "…"
	}
	return out
}

const endpointWithheld = "<endpoint withheld>"

// endpointShapes is one endpoint's scheme-less spellings, LONGEST FIRST so
// `host/path` is replaced before the `path` it contains.
//
// A path of at most one character is not returned: the secret in a push
// endpoint is its path, and `/` is every URL's — removing that would gut every
// unrelated message rather than protect anything.
func endpointShapes(endpoint string) []string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return nil
	}
	var paths []string
	for _, p := range []string{u.EscapedPath(), u.Path} {
		if len(p) > 1 && !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	}
	shapes := make([]string, 0, len(paths)*2)
	for _, p := range paths {
		shapes = append(shapes, u.Host+p)
	}
	return append(shapes, paths...)
}

func readServiceReason(r io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(r, serviceReasonCap+1))
	if err != nil || len(raw) == 0 {
		return ""
	}
	s := strings.TrimSpace(string(raw))
	if len([]rune(s)) > serviceReasonCap {
		s = string([]rune(s)[:serviceReasonCap]) + "…"
	}
	return s
}

// composePayload builds the declarative message for ONE subscription.
//
// Everything in it is either platform vocabulary or a value this function
// composed: the title is the caller's content-light line, `navigate` is the
// subscription's own recorded origin plus the caller's path, the icon is a
// self-hosted asset, the tag is the card id and the badge is a count. No string
// is lifted out of a card snapshot anywhere on this path, which is what keeps a
// decision's content off the vendor relay even before the encryption does.
func composePayload(sub Subscription, msg Message) ([]byte, error) {
	if msg.Title == "" {
		return nil, fmt.Errorf("push: a notification with no title is not a valid declarative message")
	}
	if !strings.HasPrefix(msg.Path, "/") {
		return nil, fmt.Errorf("push: navigate path %q is not rooted", msg.Path)
	}
	navigate := sub.Origin + msg.Path
	icon := ""
	if msg.IconPath != "" {
		icon = sub.Origin + msg.IconPath
	}
	return json.Marshal(declarativeMessage{
		WebPush: webPushDisambiguator,
		Notification: declarativeNotifier{
			Title:    msg.Title,
			Navigate: navigate,
			Icon:     icon,
			Tag:      msg.CardID,
			AppBadge: strconv.Itoa(msg.Badge),
		},
	})
}

// authorization returns the RFC 8292 header for one endpoint, reusing the token
// already signed for that push service.
//
// Apple asks senders not to "refresh your JWT more frequently than once per
// hour" (re-verified 2026-07-30) and refuses an `exp` more than a day out.
// Those are two different rules, and the cache honours both without counting:
// a token is signed for vapidExpiry ahead and reused until it is within
// vapidRenewMargin of expiring, which is one signature per push service per
// several hours.
func (s *Sender) authorization(endpoint, origin string) (string, error) {
	aud, err := vapidAudience(endpoint)
	if err != nil {
		return "", err
	}
	contact := vapidSub(origin)
	// The cache is keyed on BOTH claims a token carries. Two subscriptions to
	// one push service can legitimately carry different origins (a phone
	// enrolled over the tailnet, a laptop over loopback in dev), and a token
	// signed for one would then be sent on behalf of the other.
	key := aud + "\x00" + contact
	now := s.store.now()

	s.tokens.Lock()
	defer s.tokens.Unlock()
	if tok, ok := s.cached[key]; ok && now.Add(vapidRenewMargin).Before(tok.expiry) {
		return authorizationHeader(tok.jwt, s.store.PublicKey()), nil
	}
	expiry := now.Add(vapidExpiry)
	jwt, err := vapidJWT(s.store.signingKey(), aud, contact, expiry, nil)
	if err != nil {
		return "", err
	}
	s.cached[key] = cachedToken{jwt: jwt, expiry: expiry}
	return authorizationHeader(jwt, s.store.PublicKey()), nil
}

// vapidSub is the RFC 8292 §2 `sub` contact claim: a URI identifying the
// APPLICATION SERVER, so a push service has somebody to reach about its traffic.
//
// It is the SUBSCRIPTION'S OWN RECORDED ORIGIN — the address this household
// reaches its own platform at, which is the honest self-grounding answer and is
// already stored per row for the same reason `navigate` needs it (OQ7). The
// platform has no operator email anywhere in its schema and inventing one would
// be a fabricated fact about a person.
//
// CORRECTED (drain r1, D7): this used to return the AUDIENCE — the push
// service's own origin — which told the service that the application server's
// contact was the service itself. Apple accepts it, so nothing broke; it was
// simply not what the claim means, with the right value sitting unused one field
// away. A dev-posture loopback origin is http rather than the https the RFC
// prefers, and that is the only case where it is imperfect — and loopback never
// reaches a real push service.
func vapidSub(origin string) string { return origin }

// sendEvent is the contract-minimum payload of `push.sent` (B6-9 OQ9): refs, a
// closed outcome vocabulary and counts. There is no notification text here, no
// endpoint, and no key material — the subscription travels as its hash.
type sendEvent struct {
	Subscription string `json:"subscription"`
	EndpointHash string `json:"endpoint_hash"`
	Card         string `json:"card"`
	Class        string `json:"class"`
	Outcome      string `json:"outcome"`
	Status       int    `json:"status,omitempty"`
	Detail       string `json:"detail,omitempty"`
	Badge        int    `json:"badge"`
}

// recordSend writes the audit row. It has TWO duties and that is deliberate:
// it is the record that a push went out, and it is the ONLY store of "when did
// this card last go out" that the dueness derivation reads (§29/§31
// derive-from-log). There is no side table and no in-memory countdown.
func (s *Store) recordSend(ctx context.Context, sub Subscription, msg Message, res Result) error {
	now := s.now().UTC()
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		return s.appendTx(ctx, tx, sub.UserID, EventSent, sendEvent{
			Subscription: sub.ID, EndpointHash: sub.EndpointHash,
			Card: msg.CardID, Class: msg.Class,
			Outcome: res.Outcome, Status: res.Status, Detail: res.Detail,
			Badge: msg.Badge,
		}, now)
	})
}

// LastPushed answers, for one identity, when a push for each card last WENT OUT
// — successful or not.
//
// EVERY ATTEMPT SETS THE CLOCK, including a refusal, and that is the
// non-hammering reading: if a failed send left the card due, an unreachable
// push service would be re-dialled on every evaluation pass for as long as it
// stayed down. The cost is that a transient failure delays the next attempt by
// one cadence, which the runbook records.
//
// `since` bounds the scan, and the bound cannot change an answer: the caller
// passes a horizon several cadences deep, so a card whose newest push is older
// than it is due by arithmetic whether the row is read or not.
func (s *Store) LastPushed(ctx context.Context, userID string, since time.Time) (map[string]time.Time, error) {
	// Ordering is by event_seq, never by ts (S02.2, P-T07-4): stored stamps are
	// RFC3339Nano, which trims trailing zeros, so a lexicographic MAX over them
	// would rank a whole second ABOVE a fractional one. `since` is only a coarse
	// horizon, where that sub-second imprecision cannot matter.
	rows, err := s.db.QueryContext(ctx, `
		SELECT card, ts FROM (
		  SELECT json_extract(payload, '$.card') AS card, ts,
		         ROW_NUMBER() OVER (PARTITION BY json_extract(payload, '$.card') ORDER BY event_seq DESC) AS rn
		    FROM run_events
		   WHERE type = ? AND user_id = ? AND ts >= ?
		) WHERE rn = 1 AND card IS NOT NULL AND card <> ''`,
		EventSent, userID, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("push: last-pushed: %w", err)
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var card, ts string
		if err := rows.Scan(&card, &ts); err != nil {
			return nil, fmt.Errorf("push: last-pushed scan: %w", err)
		}
		out[card] = parseTS(ts)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("push: last-pushed rows: %w", err)
	}
	return out, nil
}

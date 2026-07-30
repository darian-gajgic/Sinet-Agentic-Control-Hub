package push_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/push"
)

// A FAKE PUSH SERVICE, ON LOOPBACK TLS. Nothing in this file dials a real push
// service, 8481/8482, a live unit, or the network: httptest binds 127.0.0.1 and
// every subscription here points at it. Zero pushes leave this machine.
//
// It is a TLS server rather than a plain one because the platform REFUSES a
// non-https endpoint at enrolment — every real push service is https and
// RFC 8292 binds the token to the endpoint's origin — so a plain-HTTP fake
// would have forced the production rule to be loosened for the convenience of a
// test. httptest's own client trusts its own certificate, so the strict rule
// and the hermetic test hold together.
type fakeService struct {
	*httptest.Server
	requests []recorded
	status   int
	body     string
	// keys is the receiving side, so the test can decrypt what arrived and see
	// what the platform actually said — which is the only way to assert the
	// payload contract against the WIRE rather than against the composer.
	priv *ecdh.PrivateKey
	auth []byte
}

type recorded struct {
	header http.Header
	body   []byte
	path   string
}

func newFakeService(t *testing.T) *fakeService {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatalf("auth: %v", err)
	}
	f := &fakeService{status: http.StatusCreated, priv: priv, auth: auth}
	f.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.requests = append(f.requests, recorded{header: r.Header.Clone(), body: body, path: r.URL.Path})
		w.WriteHeader(f.status)
		if f.body != "" {
			_, _ = w.Write([]byte(f.body))
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeService) keys() push.Keys {
	b := base64.RawURLEncoding
	return push.Keys{P256DH: b.EncodeToString(f.priv.PublicKey().Bytes()), Auth: b.EncodeToString(f.auth)}
}

func (f *fakeService) enrolment() push.Enrolment {
	return push.Enrolment{
		Endpoint: f.URL + "/push/alice-phone", Keys: f.keys(),
		Origin: "https://sinet.example.ts.net", Label: "phone",
	}
}

func msg(card string) push.Message {
	return push.Message{
		CardID: card, Class: push.ClassApproval,
		Title: "Sinet — a decision is waiting", Path: "/inbox/" + card,
		Badge: 3, IconPath: "/icon-192.png", TTL: 24 * time.Hour,
	}
}

// TestSendWritesTheDeclarativeContractToTheWire is the payload assertion, taken
// from what ARRIVED rather than from what was composed: the fake service
// decrypts the body with the subscription's own private key, which is the only
// reading that proves the encryption, the headers and the contract together.
func TestSendWritesTheDeclarativeContractToTheWire(t *testing.T) {
	e := newEnv(t)
	svc := newFakeService(t)
	sub, _, err := e.store.Enrol(e.ctx, "alice", svc.enrolment())
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	sender := push.NewSender(e.store, svc.Client())

	res := sender.Send(e.ctx, sub, msg("ask:ask-verify-0001"))
	if res.Outcome != push.OutcomeSent {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Detail)
	}
	if len(svc.requests) != 1 {
		t.Fatalf("the fake service saw %d requests, want 1", len(svc.requests))
	}
	req := svc.requests[0]

	// The RFC 8030 headers, against what the services documented (re-verified
	// 2026-07-30): TTL is mandatory and positive, Urgency is from the closed
	// set, Topic is ≤32 base64url characters.
	if got := req.header.Get("Content-Encoding"); got != "aes128gcm" {
		t.Errorf("Content-Encoding = %q", got)
	}
	if got := req.header.Get("Content-Type"); got != "application/notification+json" {
		t.Errorf("Content-Type = %q", got)
	}
	ttl, err := strconv.Atoi(req.header.Get("TTL"))
	if err != nil || ttl <= 0 {
		t.Errorf("TTL = %q, want a positive number", req.header.Get("TTL"))
	}
	if got := req.header.Get("Urgency"); got != "normal" {
		t.Errorf("Urgency = %q, want normal for an approval-class push", got)
	}
	if topic := req.header.Get("Topic"); len(topic) != 32 {
		t.Errorf("Topic = %q (%d chars), want 32", topic, len(topic))
	}
	auth := req.header.Get("Authorization")
	if !strings.HasPrefix(auth, "vapid t=") || !strings.Contains(auth, ", k="+e.store.PublicKey()) {
		t.Errorf("Authorization = %q, want the RFC 8292 form carrying THIS installation's public key", auth)
	}
	if len(req.body) > 4096 {
		t.Errorf("sealed body is %d bytes, over the documented 4 KB service limit", len(req.body))
	}

	// The payload, read as the device reads it.
	plain := decryptOnTheWire(t, svc, req.body)
	var got map[string]any
	if err := json.Unmarshal(plain, &got); err != nil {
		t.Fatalf("the delivered payload is not JSON: %v", err)
	}
	if got["web_push"] != float64(8030) {
		t.Errorf("web_push = %v, want the 8030 disambiguator", got["web_push"])
	}
	if _, ok := got["mutable"]; ok {
		t.Error("mutable is present; v0 sends the immutable form")
	}
	n, _ := got["notification"].(map[string]any)
	if n == nil {
		t.Fatal("no notification object")
	}
	if n["title"] != "Sinet — a decision is waiting" {
		t.Errorf("title = %v", n["title"])
	}
	if n["navigate"] != "https://sinet.example.ts.net/inbox/ask:ask-verify-0001" {
		t.Errorf("navigate = %v, want the subscription's own origin plus the deep-link path", n["navigate"])
	}
	// app_badge is a STRING inside notification — the shape WebKit ships
	// (webkit.org/blog/16535's own example is "app_badge": "1").
	if n["app_badge"] != "3" {
		t.Errorf("app_badge = %#v, want the string \"3\"", n["app_badge"])
	}
	if n["icon"] != "https://sinet.example.ts.net/icon-192.png" {
		t.Errorf("icon = %v, want a self-hosted asset on the platform's own origin", n["icon"])
	}
}

// TestPushCarriesNothingLiftedFromTheCard is the leak probe rubric 3 asks for: a
// card whose snapshot holds secret-shaped content must contribute NOTHING to the
// payload. The composer takes a title, a path and a count and has no access to a
// snapshot at all, and this drives that as a property of the wire.
func TestPushCarriesNothingLiftedFromTheCard(t *testing.T) {
	e := newEnv(t)
	svc := newFakeService(t)
	sub, _, err := e.store.Enrol(e.ctx, "alice", svc.enrolment())
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	sender := push.NewSender(e.store, svc.Client())
	sender.Send(e.ctx, sub, msg("effect:e-rotate"))

	plain := string(decryptOnTheWire(t, svc, svc.requests[0].body))
	for _, secret := range []string{
		"sk-ant-planted-secret-value",
		"rotate the production credential",
		"alice@example.com",
		sub.Endpoint,
		sub.Keys.P256DH,
		sub.Keys.Auth,
	} {
		if strings.Contains(plain, secret) {
			t.Errorf("the payload carries %q", secret)
		}
	}
	// The control: the payload is not empty and DOES carry what it is supposed
	// to, so "no hits" means "nothing leaked" rather than "nothing was sent".
	if !strings.Contains(plain, "effect:e-rotate") || !strings.Contains(plain, "a decision is waiting") {
		t.Fatalf("the payload does not carry its own composed fields: %s", plain)
	}
}

// TestDeadEndpointIsRemovedWithItsEvent drives the OQ2 disposition on 404/410,
// and the control is the other direction: a 500 leaves the row alone.
func TestDeadEndpointIsRemovedWithItsEvent(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			e := newEnv(t)
			svc := newFakeService(t)
			svc.status = status
			svc.body = `{"reason":"BadDeviceToken"}`
			sub, _, err := e.store.Enrol(e.ctx, "alice", svc.enrolment())
			if err != nil {
				t.Fatalf("enrol: %v", err)
			}
			res := push.NewSender(e.store, svc.Client()).Send(e.ctx, sub, msg("ask:a1"))
			if res.Outcome != push.OutcomeGone {
				t.Fatalf("outcome = %s, want gone", res.Outcome)
			}
			if subs, _ := e.store.List(e.ctx, "alice"); len(subs) != 0 {
				t.Errorf("the dead subscription survived: %d rows", len(subs))
			}
			un := e.events(push.EventUnsubscribed)
			if len(un) != 1 || un[0]["reason"] != "push_service_gone" {
				t.Errorf("push.unsubscribed rows = %+v, want one recording the push service's own verdict", un)
			}
			sent := e.events(push.EventSent)
			if len(sent) != 1 || sent[0]["outcome"] != push.OutcomeGone {
				t.Errorf("push.sent rows = %+v", sent)
			}
		})
	}

	// The control. A 500 is a refusal, NOT a death: the subscription stays, the
	// attempt is audited, and nothing retries.
	e := newEnv(t)
	svc := newFakeService(t)
	svc.status = http.StatusInternalServerError
	svc.body = `{"reason":"InternalServerError"}`
	sub, _, err := e.store.Enrol(e.ctx, "alice", svc.enrolment())
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	res := push.NewSender(e.store, svc.Client()).Send(e.ctx, sub, msg("ask:a1"))
	if res.Outcome != push.OutcomeRefused {
		t.Fatalf("outcome = %s, want refused", res.Outcome)
	}
	if subs, _ := e.store.List(e.ctx, "alice"); len(subs) != 1 {
		t.Error("a 500 removed the subscription")
	}
	if len(svc.requests) != 1 {
		t.Errorf("a refusal was retried: %d requests", len(svc.requests))
	}
	if len(e.events(push.EventUnsubscribed)) != 0 {
		t.Error("a 500 minted an unsubscribe")
	}
	rows := e.events(push.EventSent)
	if len(rows) != 1 || rows[0]["outcome"] != push.OutcomeRefused {
		t.Fatalf("push.sent = %+v", rows)
	}
	if rows[0]["status"] != float64(500) {
		t.Errorf("the audit row does not carry the status: %+v", rows[0])
	}
	if d, _ := rows[0]["detail"].(string); !strings.Contains(d, "InternalServerError") {
		t.Errorf("the audit row drops the push service's own reason, which is the whole diagnosis: %q", d)
	}
	// The audit row carries no notification text and no endpoint, ever.
	raw, _ := json.Marshal(rows[0])
	if strings.Contains(string(raw), "decision is waiting") || strings.Contains(string(raw), sub.Endpoint) {
		t.Errorf("the audit row carries notification text or the endpoint: %s", raw)
	}
}

// TestATransportFailureNeverWRITESTheCapabilityItFAILEDToReach is D1.
//
// A transport failure is the ORDINARY shape of a failed send — a sleeping host,
// a dropped tailnet — and `*url.Error` from http.Client.Do always embeds the
// full URL it was handed. That URL is the subscription's capability: anyone
// holding it can push to that device, which is exactly why OQ2 ruled
// hashes-never-raw and why migration 0021, contract.go and this package's own
// doc comment all say the endpoint never reaches a payload. So the commonest
// failure path is the one that has to be checked, and this checks the ROW.
func TestATransportFailureNeverWritesTheCapabilityItFailedToReach(t *testing.T) {
	e := newEnv(t)
	svc := newFakeService(t)
	sub, _, err := e.store.Enrol(e.ctx, "alice", svc.enrolment())
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	client := svc.Client()
	svc.Close() // the ordinary case: the service is not answering

	res := push.NewSender(e.store, client).Send(e.ctx, sub, msg("ask:a1"))
	if res.Outcome != push.OutcomeUnreachable {
		t.Fatalf("outcome = %s, want unreachable", res.Outcome)
	}
	rows := e.events(push.EventSent)
	if len(rows) != 1 {
		t.Fatalf("push.sent rows = %d, want 1", len(rows))
	}
	detail, _ := rows[0]["detail"].(string)
	if detail == "" {
		t.Fatal("the transport failure recorded no detail at all, so this test cannot see what it would have written")
	}
	// The whole row, not just the field: a capability must not reach the record
	// by any route.
	raw, _ := json.Marshal(rows[0])
	for _, secret := range []string{sub.Endpoint, "/push/alice-phone", "https://", "http://"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("the audit row carries %q — the endpoint is a capability and never leaves: %s", secret, raw)
		}
	}
	// The RETURNED result is scrubbed too, so a caller that logs it cannot
	// re-introduce what the row refused.
	if strings.Contains(res.Detail, "://") {
		t.Errorf("Send returned a detail carrying a URL: %q", res.Detail)
	}
	// The control, without which "no hits" could mean "the detail was emptied":
	// the classification a drill needs survives.
	if !strings.Contains(detail, "<endpoint withheld>") {
		t.Errorf("the scrub did not mark where the URL was; detail = %q", detail)
	}
	if len(detail) < 20 {
		t.Errorf("the detail was gutted rather than scrubbed: %q", detail)
	}
}

// TestScrubDetailRemovesEveryURLShapeAndKeepsTheRest drives the boundary
// directly, including the case a push service's own error BODY echoes the
// request URL back at us — the second way a capability could reach a row.
func TestScrubDetailRemovesEveryURLShapeAndKeepsTheRest(t *testing.T) {
	cases := []struct{ name, in, wantOut, wantKept string }{
		{"a url.Error from the transport",
			`Post "https://web.push.apple.com/QDzVu-secret-token": dial tcp 17.0.0.1:443: connect: connection refused`,
			"", "connection refused"},
		{"a service echoing the endpoint in its body",
			`{"reason":"BadDeviceToken","url":"https://fcm.googleapis.com/fcm/send/abc-secret"}`,
			"", "BadDeviceToken"},
		{"two URLs in one message",
			`redirected https://a.example/one to https://b.example/two`,
			"", "redirected"},
		{"an honest reason with no URL survives whole",
			`{"reason":"BadJwtToken"}`, `{"reason":"BadJwtToken"}`, "BadJwtToken"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := push.ScrubDetailForTest(c.in)
			if strings.Contains(got, "://") {
				t.Errorf("a URL survived the scrub: %q", got)
			}
			for _, leak := range []string{"secret-token", "abc-secret", "a.example", "b.example"} {
				if strings.Contains(got, leak) {
					t.Errorf("scrub left %q in %q", leak, got)
				}
			}
			if !strings.Contains(got, c.wantKept) {
				t.Errorf("scrub removed the diagnosis: %q does not carry %q", got, c.wantKept)
			}
			if c.wantOut != "" && got != c.wantOut {
				t.Errorf("scrub changed a clean detail: %q, want %q", got, c.wantOut)
			}
		})
	}
	// The cap still applies on this path, so a service's oversized body cannot
	// write an unbounded string into the log.
	long := push.ScrubDetailForTest(strings.Repeat("x", 5000))
	if len([]rune(long)) > 210 {
		t.Errorf("scrubbed detail is %d runes, over the bound", len([]rune(long)))
	}
	if push.ScrubDetailForTest("") != "" {
		t.Error("an empty detail did not stay empty")
	}
}

// TestUnreachableServiceIsAuditedNotRetried: a transport failure is a recorded
// outcome, not an exception and not a retry loop.
func TestUnreachableServiceIsAuditedNotRetried(t *testing.T) {
	e := newEnv(t)
	svc := newFakeService(t)
	sub, _, err := e.store.Enrol(e.ctx, "alice", svc.enrolment())
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	client := svc.Client()
	svc.Close() // the service goes away between enrolment and send

	res := push.NewSender(e.store, client).Send(e.ctx, sub, msg("ask:a1"))
	if res.Outcome != push.OutcomeUnreachable {
		t.Fatalf("outcome = %s, want unreachable", res.Outcome)
	}
	if subs, _ := e.store.List(e.ctx, "alice"); len(subs) != 1 {
		t.Error("an unreachable service removed the subscription")
	}
	rows := e.events(push.EventSent)
	if len(rows) != 1 || rows[0]["outcome"] != push.OutcomeUnreachable {
		t.Fatalf("push.sent = %+v", rows)
	}
}

// TestVAPIDTokenIsReusedPerPushServiceAndPerHour drives Apple's own guidance
// with a fake clock: within the reuse window the SAME token goes out, a
// different push service gets its own, and a clock jump past the renewal margin
// re-signs.
func TestVAPIDTokenIsReusedPerPushServiceAndPerHour(t *testing.T) {
	e := newEnv(t)
	appleish := newFakeService(t)
	other := newFakeService(t)
	subA, _, err := e.store.Enrol(e.ctx, "alice", appleish.enrolment())
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	subB, _, err := e.store.Enrol(e.ctx, "alice", other.enrolment())
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	sender := push.NewSender(e.store, appleish.Client())

	sender.Send(e.ctx, subA, msg("ask:a1"))
	e.now = e.now.Add(59 * time.Minute)
	sender.Send(e.ctx, subA, msg("ask:a2"))
	if len(appleish.requests) != 2 {
		t.Fatalf("requests = %d", len(appleish.requests))
	}
	first := appleish.requests[0].header.Get("Authorization")
	if again := appleish.requests[1].header.Get("Authorization"); again != first {
		t.Error("the token was re-signed inside the hour Apple asks senders to stay under")
	}

	// A different push service is a different audience, so it gets its own.
	sender2 := push.NewSender(e.store, other.Client())
	sender2.Send(e.ctx, subB, msg("ask:a3"))
	if got := other.requests[0].header.Get("Authorization"); got == first {
		t.Error("two different push services were sent the same audience-bound token")
	}

	// The `sub` claim names the APPLICATION SERVER, which is this household's
	// own origin — not the push service's (drain r1, D7). A token whose contact
	// is the audience tells the service the sender is the service.
	claims := decodeClaims(t, first)
	if claims["sub"] != "https://sinet.example.ts.net" {
		t.Errorf("sub = %v, want the subscription's own recorded origin", claims["sub"])
	}
	if claims["sub"] == claims["aud"] {
		t.Errorf("sub and aud are the same value (%v): the contact claim names the push service rather than us", claims["sub"])
	}
	if claims["aud"] != strings.TrimSuffix(appleish.URL, "/") {
		t.Errorf("aud = %v, want the push resource's origin", claims["aud"])
	}

	// Past the renewal margin the token IS replaced — the control, without
	// which a cache that never expired would pass the assertion above.
	e.now = e.now.Add(12 * time.Hour)
	sender.Send(e.ctx, subA, msg("ask:a4"))
	if again := appleish.requests[2].header.Get("Authorization"); again == first {
		t.Error("the token was never renewed")
	}
}

// decodeClaims lifts the JWT claim set out of an Authorization header.
func decodeClaims(t *testing.T, header string) map[string]any {
	t.Helper()
	tok := strings.TrimPrefix(header, "vapid t=")
	tok = strings.SplitN(tok, ",", 2)[0]
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("Authorization does not carry a JWT: %q", header)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("claims: %v", err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("claims: %v", err)
	}
	return out
}

// TestOversizedPayloadIsRefusedNotTruncated: a silently shortened ciphertext
// does not decrypt, so the device would get nothing while the log said sent.
func TestOversizedPayloadIsRefusedNotTruncated(t *testing.T) {
	e := newEnv(t)
	svc := newFakeService(t)
	sub, _, err := e.store.Enrol(e.ctx, "alice", svc.enrolment())
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	big := msg("ask:a1")
	big.Title = strings.Repeat("t", 5000)
	res := push.NewSender(e.store, svc.Client()).Send(e.ctx, sub, big)
	if res.Outcome != push.OutcomeRefused {
		t.Fatalf("outcome = %s, want refused", res.Outcome)
	}
	if len(svc.requests) != 0 {
		t.Error("an oversized payload was written to the wire anyway")
	}
	if !strings.Contains(res.Detail, "over the") {
		t.Errorf("the refusal does not name its bound: %q", res.Detail)
	}
	rows := e.events(push.EventSent)
	if len(rows) != 1 || rows[0]["outcome"] != push.OutcomeRefused {
		t.Errorf("a refused-before-the-wire send was not audited: %+v", rows)
	}
}

// TestSafetyClassRidesHighUrgency pins the one wire difference the SLA class
// makes, both directions.
func TestSafetyClassRidesHighUrgency(t *testing.T) {
	e := newEnv(t)
	svc := newFakeService(t)
	sub, _, err := e.store.Enrol(e.ctx, "alice", svc.enrolment())
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	sender := push.NewSender(e.store, svc.Client())
	safety := msg("ask:s1")
	safety.Class = push.ClassSafety
	safety.TTL = time.Hour
	sender.Send(e.ctx, sub, safety)
	sender.Send(e.ctx, sub, msg("ask:a1"))

	if got := svc.requests[0].header.Get("Urgency"); got != "high" {
		t.Errorf("safety Urgency = %q, want high", got)
	}
	if got := svc.requests[1].header.Get("Urgency"); got != "normal" {
		t.Errorf("approval Urgency = %q, want normal", got)
	}
	if got := svc.requests[0].header.Get("TTL"); got != "3600" {
		t.Errorf("safety TTL = %q, want the class's own cadence in seconds", got)
	}
	if rows := e.events(push.EventSent); len(rows) != 2 || rows[0]["class"] != push.ClassSafety {
		t.Errorf("the audit rows do not carry the class they went out under: %+v", rows)
	}
}

// TestLastPushedIsTheEventLogAndNothingElse drives the dueness store: the
// newest attempt per card, any outcome, from the log — with a bound that cannot
// change an answer and a control proving the ordering is by event_seq.
func TestLastPushedIsTheEventLogAndNothingElse(t *testing.T) {
	e := newEnv(t)
	svc := newFakeService(t)
	sub, _, err := e.store.Enrol(e.ctx, "alice", svc.enrolment())
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	sender := push.NewSender(e.store, svc.Client())

	sender.Send(e.ctx, sub, msg("ask:a1"))
	e.now = e.now.Add(2 * time.Hour)
	svc.status = http.StatusInternalServerError
	sender.Send(e.ctx, sub, msg("ask:a1")) // a REFUSAL still sets the clock
	svc.status = http.StatusCreated
	sender.Send(e.ctx, sub, msg("ask:a2"))

	horizon := e.now.Add(-96 * time.Hour)
	last, err := e.store.LastPushed(e.ctx, "alice", horizon)
	if err != nil {
		t.Fatalf("LastPushed: %v", err)
	}
	if len(last) != 2 {
		t.Fatalf("LastPushed = %v, want two cards", last)
	}
	if !last["ask:a1"].Equal(e.now) {
		t.Errorf("ask:a1 last pushed %s, want the REFUSED attempt at %s — every attempt sets the clock, or an unreachable service is re-dialled every pass", last["ask:a1"], e.now)
	}
	if !last["ask:a2"].Equal(e.now) {
		t.Errorf("ask:a2 last pushed %s", last["ask:a2"])
	}

	// A FRESH store over the same DB answers identically: dueness comes from
	// stored state, never from anything this process remembers.
	fresh, err := e.open().LastPushed(e.ctx, "alice", horizon)
	if err != nil {
		t.Fatalf("LastPushed (fresh): %v", err)
	}
	if len(fresh) != len(last) || !fresh["ask:a1"].Equal(last["ask:a1"]) {
		t.Errorf("a fresh store answered differently: %v vs %v", fresh, last)
	}

	// Owner scope: bob's reading is empty even though alice's rows exist.
	if bobs, err := e.store.LastPushed(e.ctx, "bob", horizon); err != nil || len(bobs) != 0 {
		t.Errorf("bob read alice's push history: %v (%v)", bobs, err)
	}
	// The horizon bounds the scan, and it is a bound that cannot change an
	// answer: a card older than it is due by arithmetic anyway.
	if beyond, err := e.store.LastPushed(e.ctx, "alice", e.now.Add(time.Hour)); err != nil || len(beyond) != 0 {
		t.Errorf("the horizon does not bound the scan: %v (%v)", beyond, err)
	}
}

// decryptOnTheWire is the receiving half, used so payload assertions read what
// ARRIVED rather than what was composed.
func decryptOnTheWire(t *testing.T, svc *fakeService, body []byte) []byte {
	t.Helper()
	plain, err := push.DecryptForTest(svc.priv, svc.auth, body)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	return plain
}

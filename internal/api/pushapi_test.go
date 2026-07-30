package api_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/push"
)

// The /api/push transport (B6-9). Every test here is hermetic: no route is
// dialled outward and the endpoints in the bodies are loopback TLS URLs.

// do issues a request as `who` against a server with the push channel wired.
func (e *notifyEnv) doPush(who, method, path, body string) (int, string) {
	e.t.Helper()
	return e.doPushOn(e.store, who, method, path, body)
}

func (e *notifyEnv) doPushOn(store *push.Store, who, method, path, body string) (int, string) {
	e.t.Helper()
	rr := httptest.NewRecorder()
	srv := api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store, Auth: fixedIdentity{who},
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       e.b.db, Meter: fakeMeter{},
		Now:  func() time.Time { return e.now },
		Push: store, PushSender: push.NewSender(store, e.svc.Client()),
	})
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rr.Code, rr.Body.String()
}

// enrolBody composes a browser-shaped enrolment body with REAL key material, so
// nothing here depends on a subscription the crypto would refuse.
func enrolBody(t *testing.T, endpoint, origin, label string) string {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatalf("auth: %v", err)
	}
	b := base64.RawURLEncoding
	raw, err := json.Marshal(map[string]any{
		"endpoint": endpoint,
		"keys":     map[string]string{"p256dh": b.EncodeToString(priv.PublicKey().Bytes()), "auth": b.EncodeToString(auth)},
		"origin":   origin,
		"label":    label,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

func decodePushView(t *testing.T, body string) api.PushSubscriptionsView {
	t.Helper()
	var v api.PushSubscriptionsView
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decode push view: %v: %s", err, body)
	}
	return v
}

// TestPushEnrolmentIsRetrySafeOverTheWire drives the whole verb loop through
// HTTP: enrol, re-enrol (the retry), list, remove, remove again.
func TestPushEnrolmentIsRetrySafeOverTheWire(t *testing.T) {
	e := newNotifyEnv(t)
	endpoint := e.svc.URL + "/push/alice-phone"
	body := enrolBody(t, endpoint, "https://sinet.example.ts.net", "Alice's phone")

	code, out := e.doPush("alice", http.MethodPost, "/api/push/subscriptions", body)
	if code != http.StatusOK {
		t.Fatalf("enrol: %d %s", code, out)
	}
	var first struct {
		Subscription push.Metadata `json:"subscription"`
		Replaced     bool          `json:"replaced"`
	}
	if err := json.Unmarshal([]byte(out), &first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if first.Replaced {
		t.Error("a first enrolment reported itself a replacement")
	}
	if first.Subscription.Label != "Alice's phone" || first.Subscription.Origin != "https://sinet.example.ts.net" {
		t.Errorf("subscription = %+v", first.Subscription)
	}

	// The retry: the same browser posting the endpoint it already has.
	code, out = e.doPush("alice", http.MethodPost, "/api/push/subscriptions", body)
	if code != http.StatusOK {
		t.Fatalf("re-enrol: %d %s", code, out)
	}
	var again struct {
		Replaced bool `json:"replaced"`
	}
	_ = json.Unmarshal([]byte(out), &again)
	if !again.Replaced {
		t.Error("a repeat enrolment did not report the already-resolved state")
	}
	view := decodePushView(t, e.mustPush(t, "alice", http.MethodGet, "/api/push/subscriptions", ""))
	if len(view.Subscriptions) != 1 {
		t.Fatalf("alice holds %d subscriptions after a retry, want 1", len(view.Subscriptions))
	}

	// Remove, then remove again: applied:false and no error.
	rm := `{"endpoint":` + strconv.Quote(endpoint) + `}`
	for i, want := range []bool{true, false} {
		code, out = e.doPush("alice", http.MethodPost, "/api/push/subscriptions/remove", rm)
		if code != http.StatusOK {
			t.Fatalf("remove %d: %d %s", i, code, out)
		}
		var res struct {
			Applied bool `json:"applied"`
		}
		_ = json.Unmarshal([]byte(out), &res)
		if res.Applied != want {
			t.Errorf("remove %d applied=%v, want %v", i, res.Applied, want)
		}
	}
}

// TestPushReadsAreScopedThreeWaysAndServeMetadataOnly is the OQ2 disposition
// over HTTP: the operator reads the household, a member reads their own, and
// NOBODY is served an endpoint.
func TestPushReadsAreScopedThreeWaysAndServeMetadataOnly(t *testing.T) {
	e := newNotifyEnv(t)
	aliceEndpoint := e.svc.URL + "/push/alice-secret-capability"
	bobEndpoint := e.svc.URL + "/push/bob-secret-capability"
	e.mustPush(t, "alice", http.MethodPost, "/api/push/subscriptions",
		enrolBody(t, aliceEndpoint, "https://sinet.example.ts.net", "phone"))
	e.mustPush(t, "bob", http.MethodPost, "/api/push/subscriptions",
		enrolBody(t, bobEndpoint, "https://sinet.example.ts.net", "tablet"))

	opBody := e.mustPush(t, "op", http.MethodGet, "/api/push/subscriptions", "")
	opView := decodePushView(t, opBody)
	if len(opView.Subscriptions) != 2 {
		t.Errorf("the operator reads %d subscriptions, want the household's 2", len(opView.Subscriptions))
	}
	if !strings.Contains(opView.Scope, "household") {
		t.Errorf("the operator's reading does not say which reading it is: %q", opView.Scope)
	}

	aliceBody := e.mustPush(t, "alice", http.MethodGet, "/api/push/subscriptions", "")
	aliceView := decodePushView(t, aliceBody)
	if len(aliceView.Subscriptions) != 1 || aliceView.Subscriptions[0].Owner != "alice" {
		t.Errorf("alice reads %+v, want her own row", aliceView.Subscriptions)
	}
	carolView := decodePushView(t, e.mustPush(t, "carol", http.MethodGet, "/api/push/subscriptions", ""))
	if len(carolView.Subscriptions) != 0 {
		t.Errorf("an identity with no devices read %d rows", len(carolView.Subscriptions))
	}

	// THE ENDPOINT NEVER COMES BACK OUT — not to its owner, not to the operator.
	for who, body := range map[string]string{"operator": opBody, "owner": aliceBody} {
		if strings.Contains(body, "secret-capability") {
			t.Errorf("the %s's read serves an endpoint: %s", who, body)
		}
		if !strings.Contains(body, push.HashEndpoint(aliceEndpoint)) {
			t.Errorf("the %s's read does not name the subscription by hash", who)
		}
	}
	// The VAPID public key rides the same read, so the enrolment surface never
	// carries one as a client constant.
	if opView.VAPIDPublicKey == "" || opView.VAPIDPublicKey != e.store.PublicKey() {
		t.Errorf("vapid_public_key = %q, want this installation's own", opView.VAPIDPublicKey)
	}
	if !strings.Contains(opView.Observable, "encrypted") {
		t.Errorf("the read does not state what leaves this machine: %q", opView.Observable)
	}
}

// TestOneMemberCannotRemoveAnothersDevice: the remove is keyed on the caller, so
// a body naming somebody else's endpoint matches nothing.
func TestOneMemberCannotRemoveAnothersDevice(t *testing.T) {
	e := newNotifyEnv(t)
	endpoint := e.svc.URL + "/push/alice-phone"
	e.mustPush(t, "alice", http.MethodPost, "/api/push/subscriptions",
		enrolBody(t, endpoint, "https://sinet.example.ts.net", "phone"))

	body := `{"endpoint":` + strconv.Quote(endpoint) + `}`
	out := e.mustPush(t, "bob", http.MethodPost, "/api/push/subscriptions/remove", body)
	if !strings.Contains(out, `"applied":false`) {
		t.Errorf("bob's remove of alice's device: %s", out)
	}
	// Even the OPERATOR does not remove somebody else's device here: the verb is
	// the person's own, and the operator's own list read is where they see it.
	out = e.mustPush(t, "op", http.MethodPost, "/api/push/subscriptions/remove", body)
	if !strings.Contains(out, `"applied":false`) {
		t.Errorf("the operator removed alice's device through her own verb: %s", out)
	}
	view := decodePushView(t, e.mustPush(t, "alice", http.MethodGet, "/api/push/subscriptions", ""))
	if len(view.Subscriptions) != 1 {
		t.Fatalf("alice's device was removed by somebody else")
	}
}

// TestPushEnrolmentRefusalsAreClassifiedByCode: every boundary refusal is a 400
// with the server's own code, never a 500 and never a message a client has to
// parse.
func TestPushEnrolmentRefusalsAreClassifiedByCode(t *testing.T) {
	e := newNotifyEnv(t)
	for _, c := range []struct{ name, body string }{
		{"not JSON", `{`},
		{"empty endpoint", enrolBody(t, "", "https://sinet.example.ts.net", "x")},
		{"http endpoint", enrolBody(t, "http://push.example/x", "https://sinet.example.ts.net", "x")},
		{"bad origin", enrolBody(t, e.svc.URL+"/p", "ftp://nope", "x")},
		{"no keys", `{"endpoint":"https://push.example/x","origin":"https://sinet.example.ts.net"}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			code, out := e.doPush("alice", http.MethodPost, "/api/push/subscriptions", c.body)
			if code != http.StatusBadRequest {
				t.Fatalf("status %d, want 400: %s", code, out)
			}
			if !strings.Contains(out, `"error":"bad_request"`) {
				t.Errorf("body does not carry the server's own code: %s", out)
			}
		})
	}
}

// TestAnEnrolmentRefusalNamesTheFaultAndNotTheValue is drain r2, R3.
//
// The send path's scrub (D1, and drain r2 R2) covers what a push SERVICE says
// back; the enrolment refusal is the other direction — this platform quoting
// the caller's own value into its answer. Every endpoint refused here is one
// the platform declined to store, and it is echoed only to the caller who
// supplied it, so this is a habit rather than a live leak. It is closed anyway:
// a refusal is worth more when it says what was WRONG, and a message that
// carries the value teaches the next reader that endpoints are quotable.
func TestAnEnrolmentRefusalNamesTheFaultAndNotTheValue(t *testing.T) {
	e := newNotifyEnv(t)
	const secret = "CAPABILITY-SECRET-PATH"
	for _, c := range []struct{ name, endpoint, wantSays string }{
		{"a non-https endpoint", "http://web.push.apple.com/" + secret, "https"},
		{"an unparseable endpoint", "https://web.push.apple.com/" + secret + "%zz", "escape"},
	} {
		t.Run(c.name, func(t *testing.T) {
			code, out := e.doPush("alice", http.MethodPost, "/api/push/subscriptions",
				enrolBody(t, c.endpoint, "https://sinet.example.ts.net", "phone"))
			if code != http.StatusBadRequest {
				t.Fatalf("status %d, want 400: %s", code, out)
			}
			if strings.Contains(out, secret) || strings.Contains(out, "web.push.apple.com") {
				t.Errorf("the refusal quotes the endpoint back: %s", out)
			}
			// The control: a refusal that says nothing actionable would pass the
			// line above and be useless to whoever is holding the phone.
			if !strings.Contains(out, c.wantSays) {
				t.Errorf("the refusal does not say what was wrong (want it to name %q): %s", c.wantSays, out)
			}
		})
	}
}

// TestPushRoutesAreSessionRequiredAndUnversioned keeps this family inside the
// walks every other family is subject to.
func TestPushRoutesAreSessionRequiredAndUnversioned(t *testing.T) {
	e := newNotifyEnv(t)
	srv := api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store, DevPosture: false,
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       e.b.db, Push: e.store,
	})
	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/api/push/subscriptions"},
		{http.MethodPost, "/api/push/subscriptions"},
		{http.MethodPost, "/api/push/subscriptions/remove"},
	} {
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, httptest.NewRequest(r.method, r.path, strings.NewReader("{}")))
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with no session: %d, want 401", r.method, r.path, rr.Code)
		}
	}
	// Not wired in this process = 503, never a pretend-empty answer.
	bare := api.New(api.Config{
		Log: e.b.log, Sessions: e.b.store, Auth: fixedIdentity{"alice"},
		Settings: approvalSettings(),
		HealthFn: func() api.Health { return api.Health{Ready: true} },
		DB:       e.b.db,
	})
	rr := httptest.NewRecorder()
	bare.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/push/subscriptions", nil))
	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "not_wired") {
		t.Errorf("unwired push read: %d %s", rr.Code, rr.Body.String())
	}
}

func (e *notifyEnv) mustPush(t *testing.T, who, method, path, body string) string {
	t.Helper()
	code, out := e.doPush(who, method, path, body)
	if code != http.StatusOK {
		t.Fatalf("%s %s as %s: %d: %s", method, path, who, code, out)
	}
	return out
}

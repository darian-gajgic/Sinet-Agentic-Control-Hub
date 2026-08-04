package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
)

// client returns a browser-shaped caller for ts: the test server's TLS
// trust plus a cookie jar, so the session cookie persists across requests
// (and, on the prod-posture https server, Secure cookies round-trip exactly
// as in a real browser).
func client(t *testing.T, ts *httptest.Server) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	// A fresh client per caller: ts.Client() returns one shared instance,
	// so mutating its Jar would cross-contaminate "browsers".
	return &http.Client{Transport: ts.Client().Transport, Jar: jar}
}

// postJSON posts body as application/json with optional headers and returns
// the response (caller closes).
func postJSON(t *testing.T, c *http.Client, url string, body any, hdr map[string]string) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func wantStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		t.Fatalf("%s %s = %d, want %d (body: %s)",
			resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, want, strings.TrimSpace(string(body)))
	}
}

// bootstrapOperator creates the first user through the API's bootstrap
// window (empty users table → anonymous create of the operator).
func bootstrapOperator(t *testing.T, c *http.Client, base string) {
	t.Helper()
	resp := postJSON(t, c, base+"/api/auth/users", map[string]string{
		"user_id": "op", "display_name": "Operator", "role": "operator", "pin": "246810",
	}, nil)
	wantStatus(t, resp, http.StatusCreated)
}

// TestAuthFlowProdPosture walks the whole S01.9 layer-3 surface in
// production posture: bootstrap → login (cookie) → session state →
// step-up → protected SSE route → logout.
func TestAuthFlowProdPosture(t *testing.T) {
	_, ts := newTestServer(t, serverOpts{prod: true})
	c := client(t, ts)

	// Bootstrap window: first (anonymous) create must be the operator.
	resp := postJSON(t, c, ts.URL+"/api/auth/users", map[string]string{
		"user_id": "op", "display_name": "Op", "role": "member", "pin": "246810",
	}, nil)
	wantStatus(t, resp, http.StatusBadRequest)
	bootstrapOperator(t, c, ts.URL)

	// Window closed: a second anonymous create is refused.
	resp = postJSON(t, c, ts.URL+"/api/auth/users", map[string]string{
		"user_id": "x", "display_name": "X", "role": "member", "pin": "1234",
	}, nil)
	wantStatus(t, resp, http.StatusUnauthorized)

	// The picker lists the user pre-session.
	resp, err := c.Get(ts.URL + "/api/auth/users")
	if err != nil {
		t.Fatalf("get users: %v", err)
	}
	var picker struct {
		Users []struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
			PINSet bool   `json:"pin_set"`
		} `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&picker); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	resp.Body.Close()
	if len(picker.Users) != 1 || picker.Users[0].UserID != "op" || !picker.Users[0].PINSet {
		t.Fatalf("picker = %+v", picker)
	}

	// Wrong PIN → 401; correct → 200 + session cookie.
	wantStatus(t, postJSON(t, c, ts.URL+"/api/auth/login",
		map[string]string{"user_id": "op", "pin": "wrong!"}, nil), http.StatusUnauthorized)
	resp = postJSON(t, c, ts.URL+"/api/auth/login",
		map[string]string{"user_id": "op", "pin": "246810"}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d", resp.StatusCode)
	}
	var cookie *http.Cookie
	for _, ck := range resp.Cookies() {
		if ck.Name == api.SessionCookieName {
			cookie = ck
		}
	}
	resp.Body.Close()
	if cookie == nil {
		t.Fatal("login set no session cookie")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || !cookie.Secure || cookie.Path != "/" {
		t.Fatalf("cookie attributes = %+v, want HttpOnly Secure SameSite=Lax Path=/", cookie)
	}

	// Session state reflects the person identity.
	resp, err = c.Get(ts.URL + "/api/auth/session")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	var st struct {
		Authenticated bool `json:"authenticated"`
		Dev           bool `json:"dev"`
		User          *struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	resp.Body.Close()
	if !st.Authenticated || st.Dev || st.User == nil || st.User.UserID != "op" || st.User.Role != "operator" {
		t.Fatalf("session state = %+v", st)
	}

	// The session admits the protected surface (SSE handshake).
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/events?after_seq=0", nil)
	req.Header.Set("Accept", "text/event-stream")
	sseResp, err := c.Do(req)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if sseResp.StatusCode != http.StatusOK {
		t.Fatalf("events with session = %d, want 200", sseResp.StatusCode)
	}
	sseResp.Body.Close()

	// Step-up re-prompt: wrong PIN refused, correct verified.
	wantStatus(t, postJSON(t, c, ts.URL+"/api/auth/verify-pin",
		map[string]string{"pin": "nope"}, nil), http.StatusUnauthorized)
	wantStatus(t, postJSON(t, c, ts.URL+"/api/auth/verify-pin",
		map[string]string{"pin": "246810"}, nil), http.StatusNoContent)

	// Logout revokes; the protected surface closes again.
	wantStatus(t, postJSON(t, c, ts.URL+"/api/auth/logout", map[string]string{}, nil), http.StatusNoContent)
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/events", nil)
	after, err := c.Do(req)
	if err != nil {
		t.Fatalf("events after logout: %v", err)
	}
	after.Body.Close()
	if after.StatusCode != http.StatusUnauthorized {
		t.Fatalf("events after logout = %d, want 401", after.StatusCode)
	}
}

// TestDeviceHintGrantAutoLogin proves the identity-header parse in dev-
// injectable form (the packet's dev-mode evidence): the Tailscale-User-Login
// header — injected directly here exactly as tests may — prefills the
// picker via /api/auth/session and completes login only through an explicit
// operator grant (Spec S01.9 layer 2; G3 Def.1).
func TestDeviceHintGrantAutoLogin(t *testing.T) {
	const device = "tablet@example.ts"
	_, ts := newTestServer(t, serverOpts{prod: true})
	op := client(t, ts)
	bootstrapOperator(t, op, ts.URL)
	wantStatus(t, postJSON(t, op, ts.URL+"/api/auth/login",
		map[string]string{"user_id": "op", "pin": "246810"}, nil), http.StatusOK)
	wantStatus(t, postJSON(t, op, ts.URL+"/api/auth/users", map[string]string{
		"user_id": "mem", "display_name": "Member", "role": "member",
		"pin": "1357", "actor_pin": "246810",
	}, nil), http.StatusCreated)

	// Shared-device default: no grant → the hint never completes login.
	tablet := client(t, ts)
	hdr := map[string]string{api.HeaderDeviceLogin: device}
	wantStatus(t, postJSON(t, tablet, ts.URL+"/api/auth/login",
		map[string]string{"user_id": "mem"}, hdr), http.StatusUnauthorized)

	// Hint surfaces pre-session (picker prefill), without a grant mapping.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/auth/session", nil)
	req.Header.Set(api.HeaderDeviceLogin, device)
	resp, err := tablet.Do(req)
	if err != nil {
		t.Fatalf("session with hint: %v", err)
	}
	var st struct {
		Hint *struct {
			DeviceLogin string `json:"device_login"`
			UserID      string `json:"user_id"`
			AutoLogin   bool   `json:"auto_login"`
		} `json:"hint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if st.Hint == nil || st.Hint.DeviceLogin != device || st.Hint.AutoLogin {
		t.Fatalf("pre-grant hint = %+v", st.Hint)
	}

	// Operator grants the device to mem (step-up re-prompted).
	wantStatus(t, postJSON(t, op, ts.URL+"/api/auth/grants", map[string]string{
		"device_login": device, "user_id": "mem", "pin": "246810",
	}, nil), http.StatusCreated)

	// The hint now resolves to auto-login…
	resp, err = tablet.Do(req.Clone(req.Context()))
	if err != nil {
		t.Fatalf("session with hint: %v", err)
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if st.Hint == nil || !st.Hint.AutoLogin || st.Hint.UserID != "mem" {
		t.Fatalf("post-grant hint = %+v", st.Hint)
	}

	// …but only for the granted user, and only with the header present.
	wantStatus(t, postJSON(t, tablet, ts.URL+"/api/auth/login",
		map[string]string{"user_id": "op"}, hdr), http.StatusUnauthorized)
	wantStatus(t, postJSON(t, client(t, ts), ts.URL+"/api/auth/login",
		map[string]string{"user_id": "mem"}, nil), http.StatusUnauthorized)
	wantStatus(t, postJSON(t, tablet, ts.URL+"/api/auth/login",
		map[string]string{"user_id": "mem"}, hdr), http.StatusOK)

	// The minted session is a real person session.
	resp, err = tablet.Get(ts.URL + "/api/auth/session")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	var who struct {
		User *struct {
			UserID string `json:"user_id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&who); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if who.User == nil || who.User.UserID != "mem" {
		t.Fatalf("auto-login identity = %+v", who.User)
	}

	// Revoke restores the shared default.
	wantStatus(t, postJSON(t, op, ts.URL+"/api/auth/grants/revoke", map[string]string{
		"device_login": device, "pin": "246810",
	}, nil), http.StatusNoContent)
	wantStatus(t, postJSON(t, client(t, ts), ts.URL+"/api/auth/login",
		map[string]string{"user_id": "mem"}, hdr), http.StatusUnauthorized)
}

// TestLockoutOverAPI: repeated failures surface as 401 until the ceiling,
// then 429 — including for the correct PIN while locked.
func TestLockoutOverAPI(t *testing.T) {
	_, ts := newTestServer(t, serverOpts{prod: true})
	c := client(t, ts)
	bootstrapOperator(t, c, ts.URL)

	for i := 0; i < 4; i++ {
		wantStatus(t, postJSON(t, c, ts.URL+"/api/auth/login",
			map[string]string{"user_id": "op", "pin": "wrong!"}, nil), http.StatusUnauthorized)
	}
	// 5th consecutive failure engages the lockout.
	wantStatus(t, postJSON(t, c, ts.URL+"/api/auth/login",
		map[string]string{"user_id": "op", "pin": "wrong!"}, nil), http.StatusTooManyRequests)
	wantStatus(t, postJSON(t, c, ts.URL+"/api/auth/login",
		map[string]string{"user_id": "op", "pin": "246810"}, nil), http.StatusTooManyRequests)
}

// TestDevIdentityHasNoAuthority: the dev fallback reads, but admin verbs
// refuse it — resolution only.
func TestDevIdentityHasNoAuthority(t *testing.T) {
	_, ts := newTestServer(t, serverOpts{}) // dev posture
	c := client(t, ts)

	// Reads work (dev identity passes requireIdentity).
	resp, err := c.Get(ts.URL + "/events?after_seq=0")
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dev events = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Admin verbs refuse the dev identity.
	wantStatus(t, postJSON(t, c, ts.URL+"/api/auth/grants", map[string]string{
		"device_login": "x@y", "user_id": "op", "pin": "246810",
	}, nil), http.StatusForbidden)

	// Bootstrap still works in dev (anonymous path), and real sessions win
	// over the fallback once logged in.
	bootstrapOperator(t, c, ts.URL)
	wantStatus(t, postJSON(t, c, ts.URL+"/api/auth/login",
		map[string]string{"user_id": "op", "pin": "246810"}, nil), http.StatusOK)
	resp, err = c.Get(ts.URL + "/api/auth/session")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	var st struct {
		Dev  bool `json:"dev"`
		User *struct {
			UserID string `json:"user_id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if st.Dev || st.User == nil || st.User.UserID != "op" {
		t.Fatalf("post-login state = %+v, want real op session over dev fallback", st)
	}
}

// TestAuthRequestHygiene: non-JSON content types and malformed bodies are
// refused at the door.
func TestAuthRequestHygiene(t *testing.T) {
	_, ts := newTestServer(t, serverOpts{prod: true})

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/login",
		strings.NewReader(`{"user_id":"op","pin":"246810"}`))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	wantStatus(t, resp, http.StatusUnsupportedMediaType)

	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/auth/login",
		strings.NewReader(`{"user_id":`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	wantStatus(t, resp, http.StatusBadRequest)
}

// TestDevPostureCookieNotSecure: dev serves plain HTTP on loopback, so the
// cookie drops Secure there (and only there — see the prod-posture flow
// test for the Secure assertion).
func TestDevPostureCookieNotSecure(t *testing.T) {
	_, ts := newTestServer(t, serverOpts{}) // dev posture
	c := client(t, ts)
	bootstrapOperator(t, c, ts.URL)
	resp := postJSON(t, c, ts.URL+"/api/auth/login",
		map[string]string{"user_id": "op", "pin": "246810"}, nil)
	defer resp.Body.Close()
	for _, ck := range resp.Cookies() {
		if ck.Name == api.SessionCookieName {
			if ck.Secure {
				t.Fatal("dev-posture cookie is Secure; plain-HTTP dev logins would break")
			}
			return
		}
	}
	t.Fatal("no session cookie set")
}

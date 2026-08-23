package opencode

// fake_test.go — the tier-F fake serve: an httptest server on an EPHEMERAL
// loopback port speaking the RECORDED 1.18.3 HTTP surface (testdata/PROVENANCE.md).
// It records every request it receives, including the Authorization header, so
// the suite can assert the S03.2 Basic-auth obligation on EVERY endpoint (R4)
// and the S14.5 no-engine-SSE-replay posture (R17) mechanically.
//
// Nothing here talks to a provider, a network host, or a real engine: tier F is
// $0 and runs in CI where no engine is installed (CONVENTIONS §10).

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The recorded ids the fixtures carry (live re-record 2026-08-23 at the pin).
const (
	fixtureSessionID = "ses_fd0765487ffeqQo1T87YKoDdXI"
	fixtureAskID     = "per_02f89ad800013Uga1M33kAythZ"
	fixtureCallID    = "call_fake1"
	fixtureMessageID = "msg_02f89ac6a0013cPhtKFQIODXeZ"
	fixtureDirectory = "/srv/sinet/work/u1"
)

type fakeRequest struct {
	Method      string
	Path        string
	Auth        string
	LastEventID string
	Body        string
}

type fakeReply struct {
	RequestID string
	Reply     string
	Message   string
}

type fakeServe struct {
	t        *testing.T
	srv      *httptest.Server
	password string

	mu          sync.Mutex
	reqs        []fakeRequest
	subs        []chan []byte
	permissions []json.RawMessage
	replies     []fakeReply
	sessions    []string
	status      map[string]json.RawMessage
	promptCount int

	// onPrompt runs (synchronously, before the 204) when a prompt lands.
	onPrompt func(f *fakeServe, n int)
	// onReply runs (synchronously) when a permission reply lands.
	onReply func(f *fakeServe, r fakeReply)
}

func newFakeServe(t *testing.T) *fakeServe {
	t.Helper()
	f := &fakeServe{
		t:        t,
		password: "fake-instance-password-3d9f",
		status:   map[string]json.RawMessage{},
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.route))
	t.Cleanup(func() {
		f.closeSubs()
		f.srv.Close()
	})
	return f
}

func (f *fakeServe) instance() Instance {
	return Instance{BaseURL: f.srv.URL, Password: f.password, Root: f.t.TempDir()}
}

func (f *fakeServe) authorized(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	// Recorded live at 1.18.3 (2026-08-23): the Basic USERNAME must be
	// "opencode"; any other username is refused with 401.
	return ok && user == BasicAuthUser && pass == f.password
}

func (f *fakeServe) record(r *http.Request, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, fakeRequest{
		Method:      r.Method,
		Path:        r.URL.Path,
		Auth:        r.Header.Get("Authorization"),
		LastEventID: r.Header.Get("Last-Event-ID"),
		Body:        string(body),
	})
}

func (f *fakeServe) requests() []fakeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeRequest(nil), f.reqs...)
}

func (f *fakeServe) sawPath(path string) bool {
	for _, r := range f.requests() {
		if r.Path == path {
			return true
		}
	}
	return false
}

func (f *fakeServe) repliesSeen() []fakeReply {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeReply(nil), f.replies...)
}

func (f *fakeServe) route(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil && r.Method == http.MethodPost {
		body, _ = readAllLimited(r)
	}
	f.record(r, body)
	if !f.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="opencode"`)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"_tag":"UnauthorizedError","message":"Authentication required"}`))
		return
	}
	switch {
	case r.URL.Path == "/api/health" && r.Method == http.MethodGet:
		writeJSON(w, map[string]any{"healthy": true})
	case r.URL.Path == "/global/health" && r.Method == http.MethodGet:
		writeJSON(w, map[string]any{"healthy": true, "version": Pin})
	case r.URL.Path == "/event" && r.Method == http.MethodGet:
		f.serveEvents(w)
	case r.URL.Path == "/session" && r.Method == http.MethodPost:
		f.mu.Lock()
		f.sessions = append(f.sessions, fixtureSessionID)
		f.mu.Unlock()
		writeJSON(w, map[string]any{
			"id": fixtureSessionID, "slug": "silent-star", "version": Pin,
			"projectID": "global", "directory": fixtureDirectory, "title": "New session",
			"cost": 0, "tokens": map[string]any{"input": 0, "output": 0, "reasoning": 0,
				"cache": map[string]any{"read": 0, "write": 0}},
		})
	case r.URL.Path == "/session" && r.Method == http.MethodGet:
		f.mu.Lock()
		live := make([]map[string]any, 0, len(f.sessions))
		for _, id := range f.sessions {
			live = append(live, map[string]any{"id": id, "directory": fixtureDirectory})
		}
		f.mu.Unlock()
		writeJSON(w, live)
	case r.URL.Path == "/session/status" && r.Method == http.MethodGet:
		f.mu.Lock()
		out := map[string]json.RawMessage{}
		for k, v := range f.status {
			out[k] = v
		}
		f.mu.Unlock()
		writeJSON(w, out)
	case r.URL.Path == "/permission" && r.Method == http.MethodGet:
		f.mu.Lock()
		list := append([]json.RawMessage(nil), f.permissions...)
		f.mu.Unlock()
		if list == nil {
			list = []json.RawMessage{}
		}
		writeJSON(w, list)
	case strings.HasPrefix(r.URL.Path, "/permission/") && strings.HasSuffix(r.URL.Path, "/reply"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/permission/"), "/reply")
		var in struct {
			Reply   string `json:"reply"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &in)
		rep := fakeReply{RequestID: id, Reply: in.Reply, Message: in.Message}
		f.mu.Lock()
		f.replies = append(f.replies, rep)
		f.permissions = nil
		onReply := f.onReply
		f.mu.Unlock()
		if onReply != nil {
			onReply(f, rep)
		}
		writeJSON(w, true)
	case strings.HasSuffix(r.URL.Path, "/prompt_async") && r.Method == http.MethodPost:
		f.mu.Lock()
		f.promptCount++
		n := f.promptCount
		onPrompt := f.onPrompt
		f.mu.Unlock()
		if onPrompt != nil {
			onPrompt(f, n)
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.HasSuffix(r.URL.Path, "/abort") && r.Method == http.MethodPost:
		writeJSON(w, true)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"_tag":"NotFoundError"}`))
	}
}

func (f *fakeServe) serveEvents(w http.ResponseWriter) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		f.t.Fatalf("fake serve: ResponseWriter is not a Flusher")
	}
	ch := make(chan []byte, 256)
	f.mu.Lock()
	f.subs = append(f.subs, ch)
	f.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	// The engine opens every stream with a server.connected frame.
	fmt.Fprintf(w, "data: %s\n\n", `{"id":"evt_connected","type":"server.connected","properties":{}}`)
	flusher.Flush()
	for frame := range ch {
		fmt.Fprintf(w, "data: %s\n\n", frame)
		flusher.Flush()
	}
}

// publish pushes one recorded frame to every live subscriber.
func (f *fakeServe) publish(frame []byte) {
	f.mu.Lock()
	subs := append([]chan []byte(nil), f.subs...)
	f.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- frame:
		default:
			f.t.Errorf("fake serve: subscriber buffer full, dropped a frame")
		}
	}
}

// publishFixture pushes every data frame of a testdata fixture.
func (f *fakeServe) publishFixture(name string) {
	for _, frame := range fixtureFrames(f.t, name) {
		f.publish(frame)
	}
}

// closeSubs ends every live SSE response (the shape a serve restart or a
// dropped connection produces on the client).
func (f *fakeServe) closeSubs() {
	f.mu.Lock()
	subs := f.subs
	f.subs = nil
	f.mu.Unlock()
	for _, ch := range subs {
		close(ch)
	}
}

// restart models the S03.4 restart-ask-loss shape: pending asks evaporate
// (in-memory Maps, rejected silently), sessions survive (SQLite).
func (f *fakeServe) restart() {
	f.mu.Lock()
	f.permissions = nil
	f.mu.Unlock()
	f.closeSubs()
}

// setPermissions installs the GET /permission body (the flat array).
func (f *fakeServe) setPermissions(entries ...json.RawMessage) {
	f.mu.Lock()
	f.permissions = entries
	f.mu.Unlock()
}

func (f *fakeServe) setStatus(sessionID string, raw string) {
	f.mu.Lock()
	f.status[sessionID] = json.RawMessage(raw)
	f.mu.Unlock()
}

// fixtureAsk is the recorded PermissionRequest for the gate fixtures.
func fixtureAsk() json.RawMessage {
	return json.RawMessage(fmt.Sprintf(
		`{"id":%q,"sessionID":%q,"permission":"bash","patterns":["echo hello-from-fake"],`+
			`"metadata":{"command":"echo hello-from-fake"},"always":["echo *"],`+
			`"tool":{"messageID":%q,"callID":%q}}`,
		fixtureAskID, fixtureSessionID, fixtureMessageID, fixtureCallID))
}

// fixtureFrames returns the data frames of a testdata SSE fixture, skipping
// SSE comment lines and blank separators.
func fixtureFrames(t *testing.T, name string) [][]byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	var out [][]byte
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		out = append(out, []byte(strings.TrimPrefix(line, "data: ")))
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

func readAllLimited(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, 8<<20))
}

// basicHeader is the Authorization value the adapter must send on every call.
func basicHeader(password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(BasicAuthUser+":"+password))
}

package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// newLog builds a real event log over a temp platform.db with registry
// defaults.
func newLog(t *testing.T) *eventlog.Log {
	t.Helper()
	ctx := context.Background()
	reg := settings.New()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), storage.DBFileName), reg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return eventlog.New(db, reg)
}

func appendEvents(t *testing.T, log *eventlog.Log, types ...string) []int64 {
	t.Helper()
	var seqs []int64
	for _, typ := range types {
		seq, err := log.Append(context.Background(), eventlog.Append{
			UserID:        "tester",
			Type:          typ,
			SchemaVersion: 1,
			Payload:       json.RawMessage(`{"n":1}`),
		})
		if err != nil {
			t.Fatalf("append %s: %v", typ, err)
		}
		seqs = append(seqs, seq)
	}
	return seqs
}

// fixedSettings serves one duration for every ⚙ key.
type fixedSettings struct{ d time.Duration }

func (f fixedSettings) Duration(string) (time.Duration, error) { return f.d, nil }

type serverOpts struct {
	log      *eventlog.Log
	auth     api.Authenticator
	settings api.Settings
	health   func() api.Health
	stopping chan struct{}
	poll     time.Duration
}

func newTestServer(t *testing.T, o serverOpts) (*api.Server, *httptest.Server) {
	t.Helper()
	if o.log == nil {
		o.log = newLog(t)
	}
	if o.auth == nil {
		o.auth = api.DevAuthenticator{}
	}
	if o.settings == nil {
		o.settings = fixedSettings{d: 20 * time.Second}
	}
	if o.health == nil {
		o.health = func() api.Health { return api.Health{Ready: true, Mode: "running", Version: "test"} }
	}
	if o.poll == 0 {
		o.poll = 10 * time.Millisecond
	}
	srv := api.New(api.Config{
		Log:          o.log,
		Auth:         o.auth,
		Settings:     o.settings,
		HealthFn:     o.health,
		Stopping:     o.stopping,
		PollInterval: o.poll,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

func TestHealthReadyAndStarting(t *testing.T) {
	ready := false
	log := newLog(t)
	appendEvents(t, log, "a", "b")
	_, ts := newTestServer(t, serverOpts{
		log: log,
		health: func() api.Health {
			return api.Health{Ready: ready, Mode: "running", Version: "test", EventHead: 2}
		},
	})

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("not ready: status %d, want 503", resp.StatusCode)
	}
	ready = true
	resp2, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("ready: status %d, want 200", resp2.StatusCode)
	}
	var h api.Health
	if err := json.NewDecoder(resp2.Body).Decode(&h); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !h.Ready || h.Mode != "running" || h.Version != "test" || h.EventHead != 2 {
		t.Fatalf("health = %+v", h)
	}
}

type failAuth struct{}

func (failAuth) Authenticate(*http.Request) (api.Identity, error) {
	return api.Identity{}, fmt.Errorf("no identity")
}

func TestIdentityMiddlewareFailsClosed(t *testing.T) {
	_, ts := newTestServer(t, serverOpts{auth: failAuth{}})
	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", resp.StatusCode)
	}
}

func TestDevAuthenticatorDefaultIdentity(t *testing.T) {
	id, err := api.DevAuthenticator{}.Authenticate(nil)
	if err != nil || id.UserID != "dev" {
		t.Fatalf("dev identity = %+v, %v", id, err)
	}
	id, _ = api.DevAuthenticator{UserID: "op"}.Authenticate(nil)
	if id.UserID != "op" {
		t.Fatalf("dev identity override = %+v", id)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	_, ts := newTestServer(t, serverOpts{})
	resp, err := http.Post(ts.URL+"/api/health", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 405", resp.StatusCode)
	}
}

// ── SSE plumbing ──

// frame is one parsed SSE frame (comment-only keepalives are surfaced with
// comment set).
type frame struct {
	id, event, data, comment string
}

// stream opens /events with optional header/query and returns a frame
// reader bound to ctx.
func stream(t *testing.T, ctx context.Context, url string, hdr map[string]string) (*bufio.Reader, func()) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		resp.Body.Close()
		t.Fatalf("content-type %q", ct)
	}
	return bufio.NewReader(resp.Body), func() { resp.Body.Close() }
}

// readFrame reads one frame (blank-line terminated). It returns an error on
// stream end.
func readFrame(r *bufio.Reader) (frame, error) {
	var f frame
	seen := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return frame{}, err
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "":
			if seen {
				return f, nil
			}
		case strings.HasPrefix(line, "id: "):
			f.id, seen = line[len("id: "):], true
		case strings.HasPrefix(line, "event: "):
			f.event, seen = line[len("event: "):], true
		case strings.HasPrefix(line, "data: "):
			f.data, seen = line[len("data: "):], true
		case strings.HasPrefix(line, ":"):
			f.comment, seen = strings.TrimSpace(line[1:]), true
		}
	}
}

// readEvent reads the next non-comment frame.
func readEvent(t *testing.T, r *bufio.Reader) frame {
	t.Helper()
	for {
		f, err := readFrame(r)
		if err != nil {
			t.Fatalf("stream ended: %v", err)
		}
		if f.data != "" || f.event != "" {
			return f
		}
	}
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestSSEBacklogOrderAndShape(t *testing.T) {
	log := newLog(t)
	seqs := appendEvents(t, log, "t.one", "t.two", "t.three")
	_, ts := newTestServer(t, serverOpts{log: log})

	r, done := stream(t, testCtx(t), ts.URL+"/events?after_seq=0", nil)
	defer done()
	for i, want := range []string{"t.one", "t.two", "t.three"} {
		f := readEvent(t, r)
		if f.event != want || f.id != fmt.Sprint(seqs[i]) {
			t.Fatalf("frame %d = id %s event %s, want id %d event %s", i, f.id, f.event, seqs[i], want)
		}
		var w struct {
			Seq     int64           `json:"seq"`
			UserID  string          `json:"user_id"`
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal([]byte(f.data), &w); err != nil {
			t.Fatalf("frame %d data: %v", i, err)
		}
		if w.Seq != seqs[i] || w.Type != want || w.UserID != "tester" || string(w.Payload) != `{"n":1}` {
			t.Fatalf("frame %d wire = %+v", i, w)
		}
	}
}

func TestSSEBacklogCrossesBatchBoundary(t *testing.T) {
	log := newLog(t)
	const n = 300 // > one 256-event read batch (Spec S02.1 short-batch hygiene)
	types := make([]string, n)
	for i := range types {
		types[i] = "bulk.event"
	}
	appendEvents(t, log, types...)
	_, ts := newTestServer(t, serverOpts{log: log})

	r, done := stream(t, testCtx(t), ts.URL+"/events?after_seq=0", nil)
	defer done()
	last := int64(0)
	for i := 0; i < n; i++ {
		f := readEvent(t, r)
		var w struct {
			Seq int64 `json:"seq"`
		}
		if err := json.Unmarshal([]byte(f.data), &w); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if w.Seq != last+1 {
			t.Fatalf("frame %d: seq %d, want %d (gap or reorder)", i, w.Seq, last+1)
		}
		last = w.Seq
	}
}

func TestSSEResumeCursors(t *testing.T) {
	log := newLog(t)
	appendEvents(t, log, "a", "b", "c", "d")
	_, ts := newTestServer(t, serverOpts{log: log})
	ctx := testCtx(t)

	// ?after_seq resumes past the cursor.
	r, done := stream(t, ctx, ts.URL+"/events?after_seq=2", nil)
	if f := readEvent(t, r); f.id != "3" {
		t.Fatalf("after_seq=2: first id %s, want 3", f.id)
	}
	done()

	// Last-Event-ID (EventSource reconnect) wins over after_seq.
	r, done = stream(t, ctx, ts.URL+"/events?after_seq=1", map[string]string{"Last-Event-ID": "3"})
	if f := readEvent(t, r); f.id != "4" {
		t.Fatalf("Last-Event-ID=3: first id %s, want 4", f.id)
	}
	done()
}

func TestSSEBadCursorRejected(t *testing.T) {
	_, ts := newTestServer(t, serverOpts{})
	resp, err := http.Get(ts.URL + "/events?after_seq=nope")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestSSEDefaultCursorTailsFromHead(t *testing.T) {
	log := newLog(t)
	appendEvents(t, log, "old.one", "old.two")
	srv, ts := newTestServer(t, serverOpts{log: log})

	r, done := stream(t, testCtx(t), ts.URL+"/events", nil)
	defer done()
	appendEvents(t, log, "new.one")
	srv.Nudge()
	if f := readEvent(t, r); f.event != "new.one" {
		t.Fatalf("first frame %q, want new.one (backlog must not replay without a cursor)", f.event)
	}
}

func TestSSELiveAppendViaPoll(t *testing.T) {
	log := newLog(t)
	_, ts := newTestServer(t, serverOpts{log: log, poll: 10 * time.Millisecond})

	r, done := stream(t, testCtx(t), ts.URL+"/events?after_seq=0", nil)
	defer done()
	appendEvents(t, log, "live.event") // no nudge: the poll baseline must deliver
	if f := readEvent(t, r); f.event != "live.event" {
		t.Fatalf("frame %q, want live.event", f.event)
	}
}

func TestSSEKeepaliveComment(t *testing.T) {
	_, ts := newTestServer(t, serverOpts{settings: fixedSettings{d: 30 * time.Millisecond}})
	r, done := stream(t, testCtx(t), ts.URL+"/events", nil)
	defer done()
	f, err := readFrame(r)
	if err != nil {
		t.Fatalf("stream ended: %v", err)
	}
	if f.comment != "keepalive" {
		t.Fatalf("frame %+v, want keepalive comment", f)
	}
}

func TestSSEStopDrainsFinalBatchThenEnds(t *testing.T) {
	log := newLog(t)
	stopping := make(chan struct{})
	srv, ts := newTestServer(t, serverOpts{log: log, stopping: stopping, poll: time.Hour})

	r, done := stream(t, testCtx(t), ts.URL+"/events?after_seq=0", nil)
	defer done()

	// Appended before the stop signal: must be delivered by the final
	// drain even though the poll baseline never fires (poll = 1h).
	appendEvents(t, log, "final.event")
	closeOnce(stopping)
	srv.Nudge()

	if f := readEvent(t, r); f.event != "final.event" {
		t.Fatalf("frame %q, want final.event", f.event)
	}
	if _, err := readFrame(r); err == nil {
		t.Fatal("stream still open after stop; want end of stream")
	}
}

func closeOnce(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

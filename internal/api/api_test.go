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

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/chat"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/preview"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// backend is a real migrated platform.db with its event log and auth store.
type backend struct {
	db    *storage.DB
	log   *eventlog.Log
	store *auth.Store
	// reg is the settings registry the DB was opened against. It is exposed so
	// the drain-D4 Layer-2 rig can build a real local.Registry over it.
	reg *settings.Registry
	// mem/memGate are composed ONCE per backend by the rigs that need them, so
	// every server built over the same backend shares one knowledge root — a
	// per-server root would put an entry's file where the next server cannot
	// read it. Nil until a rig composes them.
	mem     *memory.Store
	memGate *memory.Gate
	// chat is the S15.7 assistant store, composed once per fixture world with a
	// pinned clock + id seam (B6-7) so the committed bodies reproduce.
	chat *chat.Store
	// rev/acc/prev are the S13 family, composed ONCE per fixture world (B6-8).
	// The review store in particular has to be shared: its Root is where minted
	// revision BYTES live, and a per-server root would put a revision's content
	// where the next server cannot read it — which is exactly why the review
	// fixtures could not be producer-driven before. Nil until a rig composes them.
	rev  *review.Store
	acc  *accept.Accepter
	prev *preview.Manager
	// work is the S08 worker registry, composed ONCE per world (B6-8 part B)
	// with a pinned clock and id seam. It has to be shared for the review
	// store's reason: its Root is where template FILES live, and a per-server
	// root would put a version's definition where the next server cannot
	// hash-verify it. Nil until a rig composes it.
	work *worker.Store
}

func newBackend(t *testing.T) *backend {
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
	log := eventlog.New(db, reg)
	return &backend{db: db, log: log, store: auth.New(db, log), reg: reg}
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
	b        *backend
	prod     bool // true = production posture (no dev identity fallback)
	auth     api.Authenticator
	settings api.Settings
	health   func() api.Health
	stopping chan struct{}
	poll     time.Duration
	// nil = no metering seam: tokens stay 0 and the COST is served as an
	// absence, not a zero (meterReading — a money zero is a price nobody set).
	meter api.MeterReader
}

func newTestServer(t *testing.T, o serverOpts) (*api.Server, *httptest.Server) {
	t.Helper()
	if o.b == nil {
		o.b = newBackend(t)
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
		Log:          o.b.log,
		Sessions:     o.b.store,
		DevPosture:   !o.prod,
		Auth:         o.auth,
		Settings:     o.settings,
		HealthFn:     o.health,
		Stopping:     o.stopping,
		PollInterval: o.poll,
		DB:           o.b.db,
		Meter:        o.meter,
	})
	// Production posture serves the browser leg over TLS (terminated at
	// tailscale serve in the real chain, Spec S01.4), and its session
	// cookie is Secure — so prod-posture tests must speak https for the
	// cookie jar to behave like a browser. Dev posture is plain loopback
	// HTTP, exactly like `sinet control` in dev.
	var ts *httptest.Server
	if o.prod {
		ts = httptest.NewTLSServer(srv.Handler())
	} else {
		ts = httptest.NewServer(srv.Handler())
	}
	t.Cleanup(ts.Close)
	return srv, ts
}

func TestHealthReadyAndStarting(t *testing.T) {
	ready := false
	b := newBackend(t)
	appendEvents(t, b.log, "a", "b")
	_, ts := newTestServer(t, serverOpts{
		b: b,
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

// TestProdPostureRequiresSession: in production posture an anonymous
// request reaches the pre-session surface (health, session state) but never
// a session-required route — fail-closed 401 (Spec S01.9, S15.2).
func TestProdPostureRequiresSession(t *testing.T) {
	_, ts := newTestServer(t, serverOpts{prod: true})

	for path, want := range map[string]int{
		"/api/health":       http.StatusOK,
		"/api/auth/session": http.StatusOK,
		"/api/auth/users":   http.StatusOK,
		"/events":           http.StatusUnauthorized,
		"/api/auth/grants":  http.StatusUnauthorized,
	} {
		resp, err := ts.Client().Get(ts.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("GET %s = %d, want %d", path, resp.StatusCode, want)
		}
	}
}

// failAuth simulates an internal identity-resolution failure (not
// anonymity): the middleware must fail closed with 500, never downgrade to
// anonymous.
type failAuth struct{}

func (failAuth) Authenticate(*http.Request) (api.Identity, error) {
	return api.Identity{}, fmt.Errorf("store unavailable")
}

func TestIdentityResolutionFailureFailsClosed(t *testing.T) {
	_, ts := newTestServer(t, serverOpts{auth: failAuth{}})
	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500 (fail closed, no anonymous downgrade)", resp.StatusCode)
	}
}

// TestDevFallbackIdentity: dev posture resolves an unauthenticated request
// to the fixed dev identity — resolution only, so dev flows and the SSE
// demo work without a login (P3/CONVENTIONS.md §7).
func TestDevFallbackIdentity(t *testing.T) {
	_, ts := newTestServer(t, serverOpts{}) // default = dev posture
	resp, err := http.Get(ts.URL + "/api/auth/session")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	var got struct {
		Authenticated bool `json:"authenticated"`
		Dev           bool `json:"dev"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Authenticated || !got.Dev {
		t.Fatalf("session state = %+v, want authenticated dev fallback", got)
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
	b := newBackend(t)
	seqs := appendEvents(t, b.log, "t.one", "t.two", "t.three")
	_, ts := newTestServer(t, serverOpts{b: b})

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
	b := newBackend(t)
	const n = 300 // > one 256-event read batch (Spec S02.1 short-batch hygiene)
	types := make([]string, n)
	for i := range types {
		types[i] = "bulk.event"
	}
	appendEvents(t, b.log, types...)
	_, ts := newTestServer(t, serverOpts{b: b})

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
	b := newBackend(t)
	appendEvents(t, b.log, "a", "b", "c", "d")
	_, ts := newTestServer(t, serverOpts{b: b})
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
	b := newBackend(t)
	appendEvents(t, b.log, "old.one", "old.two")
	srv, ts := newTestServer(t, serverOpts{b: b})

	r, done := stream(t, testCtx(t), ts.URL+"/events", nil)
	defer done()
	appendEvents(t, b.log, "new.one")
	srv.Nudge()
	if f := readEvent(t, r); f.event != "new.one" {
		t.Fatalf("first frame %q, want new.one (backlog must not replay without a cursor)", f.event)
	}
}

func TestSSELiveAppendViaPoll(t *testing.T) {
	b := newBackend(t)
	_, ts := newTestServer(t, serverOpts{b: b, poll: 10 * time.Millisecond})

	r, done := stream(t, testCtx(t), ts.URL+"/events?after_seq=0", nil)
	defer done()
	appendEvents(t, b.log, "live.event") // no nudge: the poll baseline must deliver
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
	b := newBackend(t)
	stopping := make(chan struct{})
	srv, ts := newTestServer(t, serverOpts{b: b, stopping: stopping, poll: time.Hour})

	r, done := stream(t, testCtx(t), ts.URL+"/events?after_seq=0", nil)
	defer done()

	// Appended before the stop signal: must be delivered by the final
	// drain even though the poll baseline never fires (poll = 1h).
	appendEvents(t, b.log, "final.event")
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

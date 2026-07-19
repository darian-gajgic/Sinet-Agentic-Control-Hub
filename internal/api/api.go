// Package api is the API seam of Spec S01.3: every surface — SPA, chat,
// any future CLI or ingress channel — is an equal client of this one HTTP
// API and the one SSE endpoint; nothing renders from private access (Spec
// S15.2). The control plane serves it on 127.0.0.1 only, behind the S01.4
// front chain (tailscale serve → Caddy → here); the loopback posture is
// asserted fail-closed by the shell's listener-binding lint (P-T13-2).
//
// B0-3 ships the skeleton: the health/readiness surface and the SSE stream
// over the event log's event_seq cursor. Endpoint families beyond these
// (Spec S15.2 table) land with their data owners. The API is unversioned at
// v0 — no /v1 prefix; SPA and API ship in one binary and evolution is
// additive-first (Spec S15.2).
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// Settings is the api-facing view of the settings registry (Spec S01.10):
// effective ⚙ values by dotted key.
type Settings interface {
	Duration(key string) (time.Duration, error)
}

// keySSEKeepalive is the comment-frame keepalive cadence on the SSE stream,
// owned by Spec S14 (⚙ obs.sse_keepalive) and consumed by this transport.
const keySSEKeepalive = "obs.sse_keepalive"

// Health is one readiness snapshot, provided by the shell.
type Health struct {
	// Ready is true once the S01.6 startup sequence has completed
	// (recovery ladder run, READY notified) and admission has resumed.
	Ready bool `json:"ready"`
	// Mode is the shell lifecycle mode: "running", "draining",
	// "maintenance" (Spec S01.6 maintenance switch), or "stopping".
	Mode string `json:"mode"`
	// Version is the binary identity (one release artifact, Spec S01.5).
	Version string `json:"version"`
	// EventHead is the highest event_seq — the tail cursor bootstrap for
	// snapshot-then-tail clients (Spec S15.3).
	EventHead int64 `json:"event_head"`
}

// Config assembles a Server. Log, Auth, Settings and HealthFn are
// mandatory; the rest have defaults.
type Config struct {
	Log      *eventlog.Log
	Auth     Authenticator
	Settings Settings
	// HealthFn returns the current readiness snapshot.
	HealthFn func() Health
	// Stopping is closed when the shell begins shutdown: SSE handlers
	// drain their final batch and return so the graceful HTTP shutdown can
	// complete (Spec S01.6 shutdown).
	Stopping <-chan struct{}
	// PollInterval is the idle re-poll cadence of the SSE tail loop. It is
	// deliberately not a ⚙ setting — no such key is ratified; transport
	// refinement belongs to Spec S14 (B5). 0 = default 250ms.
	PollInterval time.Duration
	Logger       *slog.Logger // nil = slog.Default
}

// Server is the HTTP API of the control plane.
type Server struct {
	log      *eventlog.Log
	auth     Authenticator
	settings Settings
	healthFn func() Health
	stopping <-chan struct{}
	poll     time.Duration
	logger   *slog.Logger
	nudge    *broadcast
}

// New assembles the Server.
func New(cfg Config) *Server {
	s := &Server{
		log:      cfg.Log,
		auth:     cfg.Auth,
		settings: cfg.Settings,
		healthFn: cfg.HealthFn,
		stopping: cfg.Stopping,
		poll:     cfg.PollInterval,
		logger:   cfg.Logger,
		nudge:    newBroadcast(),
	}
	if s.poll <= 0 {
		s.poll = 250 * time.Millisecond
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.stopping == nil {
		s.stopping = make(chan struct{})
	}
	return s
}

// Handler returns the routed HTTP handler, every route wrapped in the
// identity middleware (Spec S01.9 seam; authoritative stack lands at B0-5).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /events", s.handleEvents)
	return s.identity(mux)
}

// Nudge signals every open SSE stream to poll the log now. It is a
// best-effort edge trigger over the poll baseline; the shell calls it after
// its own lifecycle appends.
func (s *Server) Nudge() { s.nudge.fire() }

// handleHealth is the health/readiness surface: 200 with the snapshot once
// the S01.6 startup sequence has completed, 503 (still with the snapshot)
// while starting or stopping — the front chain tolerates a not-yet-ready
// backend (Spec S01.6).
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	h := s.healthFn()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if !h.Ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	if err := json.NewEncoder(w).Encode(h); err != nil {
		s.logger.Warn("health: encode", "err", err)
	}
}

// broadcast is a reusable edge trigger: wait returns a channel that fire
// closes; every fire replaces it, so each returned channel signals at most
// once and late waiters get a fresh one.
type broadcast struct {
	mu   sync.Mutex
	next chan struct{}
}

func newBroadcast() *broadcast { return &broadcast{next: make(chan struct{})} }

func (b *broadcast) fire() {
	b.mu.Lock()
	close(b.next)
	b.next = make(chan struct{})
	b.mu.Unlock()
}

func (b *broadcast) wait() <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.next
}

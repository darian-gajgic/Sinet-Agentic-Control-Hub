// Package api is the API seam of Spec S01.3: every surface — SPA, chat,
// any future CLI or ingress channel — is an equal client of this one HTTP
// API and the one SSE endpoint; nothing renders from private access (Spec
// S15.2). The control plane serves it on 127.0.0.1 only, behind the S01.4
// front chain (tailscale serve → Caddy → here); the loopback posture is
// asserted fail-closed by the shell's listener-binding lint (P-T13-2).
//
// B0-3 shipped the skeleton (health + the SSE stream over the event log's
// event_seq cursor); B0-5 makes identity real: the S01.9 session/PIN stack
// behind the identity-middleware seam, plus the login/session endpoints
// (S15.2: "Login/session endpoints are S01.9's"). Endpoint families beyond
// these (Spec S15.2 table) land with their data owners. The API is
// unversioned at v0 — no /v1 prefix; SPA and API ship in one binary and
// evolution is additive-first (Spec S15.2).
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/preview"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// Settings is the api-facing view of the settings registry (Spec S01.10):
// effective ⚙ values by dotted key.
type Settings interface {
	Duration(key string) (time.Duration, error)
}

// RunMeter is the S14.3 run-card counter seam result: the run's monotonic
// token total and API-equivalent cost so far (§4). Subscription lanes are
// Unpriced → the cost is the done-directly heuristic figure (S10, on
// receipts), never money-by-generation.
type RunMeter struct {
	Tokens          int64
	APIEquivCostUSD float64
	Unpriced        bool
}

// MeterReader is the narrow metering read-seam for the run card: it folds a
// run's checkpoint usage into its counters. The shell adapts
// metering.Ledger.RunConsumption to it; nil leaves the counters at zero (the
// snapshot still projects, best-effort).
type MeterReader interface {
	RunMeter(ctx context.Context, runID string) (RunMeter, error)
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

// Config assembles a Server. Log, Sessions, Settings and HealthFn are
// mandatory; the rest have defaults.
type Config struct {
	Log *eventlog.Log
	// Sessions is the S01.9 auth data layer (sessions, PINs, grants).
	Sessions *auth.Store
	// DevPosture marks the dev-mode process (the shell keys it off the
	// absence of systemd env, P3/CONVENTIONS.md §7): the identity
	// middleware falls back to the fixed dev identity and cookies drop the
	// Secure attribute (dev serves plain HTTP on loopback).
	DevPosture bool
	// Auth overrides the identity-middleware seam implementation; nil =
	// SessionAuthenticator over Sessions/DevPosture (the production stack).
	Auth     Authenticator
	Settings Settings
	// HealthFn returns the current readiness snapshot.
	HealthFn func() Health
	// Stopping is closed when the shell begins shutdown: SSE handlers
	// drain their final batch and return so the graceful HTTP shutdown can
	// complete (Spec S01.6 shutdown).
	Stopping <-chan struct{}
	// Intake is the walking-skeleton pipeline surface (intake_handlers.go);
	// nil leaves those routes answering 503 (surface not wired).
	Intake IntakeSurface
	// Accept + FollowUp are the S13.6/S13.9 operator surfaces, composed at the
	// shell root and HELD here for the B6 endpoints (the S01.9 PIN step-up rides
	// that surface, seam §3). They are wired but NOT yet routed — the invocation
	// endpoints land with the B6 operator surfaces (F1).
	Accept   *accept.Accepter
	FollowUp *intake.FollowUp
	// Preview is the S13.8 preview surface (launch/stop/list/before-vs-after),
	// composed at the shell root and HELD here for the B6 /api/deliverables
	// preview endpoints (S15.2); wired but NOT yet routed (F17).
	Preview *preview.Manager
	// PollInterval is the idle re-poll cadence of the SSE tail loop. It is
	// deliberately not a ⚙ setting — no such key is ratified; transport
	// refinement belongs to Spec S14 (B5). 0 = default 250ms.
	PollInterval time.Duration
	// DB is the read-only handle for the S14.3 snapshot projections (brief §3):
	// short SELECTs over the existing tables, owner-scoped per S01.9 (OQ1). nil
	// disables snapshot-then-tail — the raw owner-scoped tail still serves
	// (back-compat with the B0-3 endpoint).
	DB *storage.DB
	// Meter is the run-card metering seam (RunMeter); nil leaves the token/cost
	// counters at zero.
	Meter  MeterReader
	Logger *slog.Logger // nil = slog.Default
}

// Server is the HTTP API of the control plane.
type Server struct {
	log        *eventlog.Log
	sessions   *auth.Store
	devPosture bool
	auth       Authenticator
	settings   Settings
	healthFn   func() Health
	stopping   <-chan struct{}
	poll       time.Duration
	logger     *slog.Logger
	nudge      *broadcast
	intake     IntakeSurface
	// accept + followUp are held for the B6 operator endpoints (F1); not yet
	// routed.
	accept   *accept.Accepter
	followUp *intake.FollowUp
	// preview is the S13.8 preview surface, held for the B6
	// /api/deliverables preview endpoints (S15.2); not yet routed (F17).
	preview *preview.Manager
	// proj is the S14.3 snapshot projector (brief §3); nil when no DB is wired
	// (the raw tail still serves).
	proj *projector
}

// New assembles the Server.
func New(cfg Config) *Server {
	s := &Server{
		log:        cfg.Log,
		sessions:   cfg.Sessions,
		devPosture: cfg.DevPosture,
		auth:       cfg.Auth,
		settings:   cfg.Settings,
		healthFn:   cfg.HealthFn,
		stopping:   cfg.Stopping,
		poll:       cfg.PollInterval,
		logger:     cfg.Logger,
		nudge:      newBroadcast(),
		intake:     cfg.Intake,
		accept:     cfg.Accept,
		followUp:   cfg.FollowUp,
		preview:    cfg.Preview,
	}
	if cfg.DB != nil {
		s.proj = &projector{db: cfg.DB, meter: cfg.Meter}
	}
	if s.auth == nil {
		s.auth = SessionAuthenticator{Sessions: cfg.Sessions, DevFallback: cfg.DevPosture}
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

// Handler returns the routed HTTP handler. Every route runs behind the
// identity-resolving middleware (the Spec S01.9 seam). The pre-session
// surface — readiness, session state, the user picker, login, and the
// bootstrap-window user create — serves without a session (reaching it
// already proves tailnet membership, Spec S01.9 layer 1); everything else
// requires an authenticated identity, enforced fail-closed.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Pre-session surface.
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/auth/session", s.handleAuthSession)
	mux.HandleFunc("GET /api/auth/users", s.handleAuthUsers)
	mux.HandleFunc("POST /api/auth/login", s.handleAuthLogin)
	mux.HandleFunc("POST /api/auth/users", s.handleAuthUserCreate)

	// Session-required surface.
	protected := func(pattern string, h http.HandlerFunc) {
		mux.Handle(pattern, s.requireIdentity(h))
	}
	protected("GET /events", s.handleEvents)
	protected("POST /api/auth/logout", s.handleAuthLogout)
	protected("POST /api/auth/verify-pin", s.handleAuthVerifyPIN)
	protected("POST /api/auth/pin", s.handleAuthSetPIN)
	protected("GET /api/auth/grants", s.handleAuthGrants)
	protected("POST /api/auth/grants", s.handleAuthGrantCreate)
	protected("POST /api/auth/grants/revoke", s.handleAuthGrantRevoke)

	// Walking-skeleton surface (B2-4; provisional pending S15.2/B6).
	protected("POST /api/intake/requests", s.handleIntakeSubmit)
	protected("GET /api/tasks/{task}", s.handleTask)
	protected("POST /api/tasks/{task}/advance", s.handleTaskAdvance)
	protected("POST /api/asks/{ask}/answer", s.handleAskAnswer)
	protected("GET /api/runs/{run}/receipt", s.handleRunReceipt)

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

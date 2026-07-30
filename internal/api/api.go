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
	"strings"
	"sync"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/accept"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/chat"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/intake"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/preview"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/push"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
)

// Settings is the api-facing view of the settings registry (Spec S01.10):
// effective ⚙ values by dotted key.
type Settings interface {
	Duration(key string) (time.Duration, error)
	// Int reads an integer-typed key. The S15.11 notifier's three cadence keys
	// (verification.card_push_hours, safety_reping_hours, card_remind_hours) are
	// declared Int hours, so Duration cannot read them — this is the additive
	// widening B6-9 needed, on the same narrow seam every other ⚙ read here
	// goes through rather than a second settings interface. The int64 return is
	// the registry's own signature, matched rather than adapted: an interface
	// that narrowed it would force a conversion at every implementor.
	Int(key string) (int64, error)
}

// RunMeter is the S14.3 run-card counter seam result: the run's monotonic
// token total and API-equivalent cost so far (§4). Subscription lanes are
// Unpriced → the cost is the done-directly heuristic figure (S10, on
// receipts), never money-by-generation.
type RunMeter struct {
	Tokens          int64
	APIEquivCostUSD float64
	Unpriced        bool
	// Calls is how many usage rows the seam folded to produce the figures above
	// — the DISCRIMINATOR between "measured, and it came to zero" and "nothing
	// has been measured yet", which the folded magnitudes alone cannot tell
	// apart. It exists because the local tier deliberately prices a TRUE $0
	// (a zero-allowance row), so a run whose only work was local folds to zero
	// tokens, zero cost and zero unpriced calls exactly like a run nobody has
	// touched. A seam that does not report it leaves it 0 and the magnitudes
	// decide, which is the pre-existing reading (see meterReading).
	Calls int64
}

// LaneMeter is the S14.3 fleet per-lane meter snapshot (§3): the S10.4
// weighted consumption (always available) plus utilization and budget-remaining
// against the operator-declared budget. Utilization/BudgetRemaining are nil
// until a budget is declared (S10.4 — v0 declares none), so the fleet lane
// carries a real consumption figure regardless, never just a run count.
type LaneMeter struct {
	WeightedConsumption float64
	Utilization         *float64
	BudgetRemaining     *float64
	// CacheReadWeight + Assumed carry the S10.4 gauge's own "assumed" label
	// (G1 Def.10) onto the meters read: the cache-read weight is an assumption
	// until subscription quota semantics publish, and the reading says so.
	CacheReadWeight float64
	Assumed         bool
	// BudgetDeclared is the gauge's Budget.Declared bit. At v0 no operator
	// budget is persisted anywhere, so it is false and the ABSENCE (with its
	// reason) is what the meters surface serves — never a zero (S10.1).
	BudgetDeclared bool
}

// MeterReader is the narrow metering read-seam for the S14.3 snapshots: the
// run-card counters and the fleet per-lane meter. Both reads are owner-keyed
// (RunMeter folds one run's checkpoints; LaneMeter reads one (owner, lane)
// consumption). The shell adapts metering.Ledger + PressureGauge to it; nil
// leaves the token counter at zero and the COST an honest absence — never a
// zero, which would be a price nobody set (see meterReading). The snapshot
// still projects; only the figures the seam would have carried go missing.
type MeterReader interface {
	RunMeter(ctx context.Context, runID string) (RunMeter, error)
	LaneMeter(ctx context.Context, userID, lane string) (LaneMeter, error)
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
	// Registry is the FULL S01.10 settings registry behind the S15.9 settings
	// family (settings.go, B6-3A). Settings above stays the narrow Duration
	// seam every other consumer reads ⚙ values through — this is the surface
	// that serves and edits the registry ITSELF, which needs the declarations,
	// the emitters, the effective bounds and the audit table. It is held
	// directly rather than behind an interface for the same reason
	// *gates.Journal and *accept.Accepter are (§39): internal/settings is a
	// leaf over storage+eventlog, so the dependency is narrow and acyclic, and
	// an eight-method interface would only be a second name for one type.
	// nil leaves the settings routes answering 503.
	Registry *settings.Registry
	// Prices is the S10.3 stored price table (migration 0019), adapted at the
	// shell root; nil leaves the price routes answering 503.
	Prices PriceSurface
	// HealthFn returns the current readiness snapshot.
	HealthFn func() Health
	// Stopping is closed when the shell begins shutdown: SSE handlers
	// drain their final batch and return so the graceful HTTP shutdown can
	// complete (Spec S01.6 shutdown).
	Stopping <-chan struct{}
	// Intake is the walking-skeleton pipeline surface (intake_handlers.go);
	// nil leaves those routes answering 503 (surface not wired).
	Intake IntakeSurface
	// Review is the S13.1–S13.4 deliverable/review data layer behind the S15.2
	// deliverables family (deliverables.go, B6-3B): the revision lineage, the
	// per-type comparison, the anchored comments and their placements. It is
	// held directly rather than behind an interface for the same reason
	// *gates.Journal and *accept.Accepter are (§39): internal/review is a leaf
	// over storage+eventlog, so the dependency is narrow and acyclic, and an
	// interface would only be a second name for one type. nil leaves the
	// deliverables routes answering 503.
	Review *review.Store
	// Accept + FollowUp are the S13.6/S13.9 operator surfaces, composed at the
	// shell root (the S01.9 PIN step-up rides this surface, seam §3). FollowUp
	// is routed by B6-2A (actions.go); Accept is routed by B6-3B (accept.go).
	Accept   *accept.Accepter
	FollowUp *intake.FollowUp
	// Preview is the S13.8 preview surface (launch/stop/list/before-vs-after),
	// composed at the shell root and ROUTED by B6-3B (previewapi.go); nil leaves
	// those routes answering 503.
	Preview *preview.Manager
	// Memory + MemoryGate are the S09 knowledge subsystem behind the S15.2
	// memory family (memory.go, B6-3C), and the SPLIT between them is the point:
	// reads live on the store, and every L2 write verb lives on the gate, whose
	// calls carry an authenticated human actor (S09.1 capability withholding).
	// The api is the human-facing surface the §17 import wall exists to admit —
	// it bars the engine-facing packages, never this one — and both are held
	// directly for the *review.Store / *settings.Registry reason (§39/§40-B):
	// internal/memory is a leaf over storage+eventlog, so the dependency is
	// narrow and acyclic. nil leaves the memory routes answering 503.
	Memory     *memory.Store
	MemoryGate *memory.Gate
	// Effects is the S02.7 two-phase effect journal, ROUTED by the S15.6
	// approvals family (approvals.go): an effect card's approve/deny answer is
	// a journal act. Nothing here executes an effect — approval and execution
	// stay different recorded facts and execution is the journal's own
	// two-phase path (D7). nil leaves effect answers refusing 503.
	Effects *gates.Journal
	// Cancel is the S02.3 cancel choreography (feature 4.5, actions.go), the
	// ONLY path from the HTTP verbs to the ratified cancel mapping. nil leaves
	// the cancel routes answering 503.
	Cancel CancelSurface
	// Now is the decision-row clock (drain r1 D3). It exists for the same
	// reason review.Store.Now and gates.JournalConfig.Now do: the acts that
	// mint a Human-decision row are the ones a test has to drive through the
	// REAL verb to prove a derive reads real producer output, and a wall-clock
	// stamp inside the mint makes that output non-reproducible. nil is
	// time.Now, so production behavior is unchanged.
	Now func() time.Time
	// The B6-2B seams — the S10.4 meters mutations and the oversight verbs over
	// LANDED internals. Each is nil-able and nil leaves its route answering 503;
	// none of them re-implements anything (meters_verbs.go, oversight.go).
	//
	// Budgets/Pause persist the S10.4 operator switches (migration 0017); Hints
	// carries the S15.5 board drag onto the scheduler's queue row; Watchdog
	// routes the landed S14.4 Suppress; Resume takes the ratified S02.3
	// parked→running edge for a person.
	Budgets  BudgetStore
	Pause    PauseStore
	Hints    HintSurface
	Watchdog SuppressSurface
	Resume   ResumeSurface
	// Benchmark is the S14.7 / BENCH-REG practice behind the B6-2C verdict
	// backend (benchmark.go): the blind form's data, the §3.3 one-act verdict,
	// the §4.2.5 decline, the §12 alarm disposition and the §4.2.1 consent flip.
	// Adapted at the shell root — internal/api imports internal/benchmark in
	// neither direction — and nil leaves those routes answering 503.
	Benchmark BenchmarkSurface
	// History is the S14.10 three-layer queryable-history surface (B5-8B):
	// Layer-0 named cost views, the Layer-1 canned catalog, redact-before-match
	// search and the Layer-2 escalation. B6-1 ROUTES it under /api/events
	// (historyapi.go) — the transport the S15 conversational assistant will
	// call, which "consumes these layers and nothing else". nil leaves those
	// routes answering 503; Layers 0 and 1 are the floor and say so.
	History *history.Store
	// Workforce is the S08 worker registry behind the S15.10 workforce map
	// (workforce.go, B6-8 part B): the roster, each worker's equipment and
	// permissions, and the multi-stage chains. It is READ-ONLY here — the map
	// has no mutation affordance and this transport calls no worker verb (S15.10
	// parks editing to 15.5). Held directly for the *review.Store / *chat.Store
	// reason (§39/§40-C/§44): internal/worker is a leaf over storage+eventlog,
	// so the dependency is narrow and acyclic. nil leaves the route answering
	// 503.
	Workforce *worker.Store
	// Push is the S15.11 Web Push channel (push.go + notifier.go, B6-9): the
	// per-device subscription registry, the RFC 8291/8292 sender and the VAPID
	// key. Held directly for the *review.Store / *chat.Store reason
	// (§39/§40-C/§44): internal/push is a leaf over storage+eventlog, so the
	// dependency is narrow and acyclic. It performs no evaluation — WHICH cards
	// are waiting for whom is this package's own derivation and re-deriving it
	// there would be the twin-maintained-copy hazard. nil leaves the push
	// routes answering 503 and EvaluatePush a no-op.
	Push *push.Store
	// PushSender is the outbound half, held separately so a test can drive the
	// whole notifier against a fake push service without the production
	// http.Client existing. nil = a sender over Push.
	PushSender PushSender
	// Chat is the S15.7 conversational assistant's durable state behind the
	// `/api/chat` family (chatapi.go, B6-7): the per-user session registry, the
	// immutable transcript, the turn lifecycle and the file exchange. Held
	// directly for the *review.Store / *memory.Store reason (§39/§40-C):
	// internal/chat is a leaf over storage+eventlog, so the dependency is
	// narrow and acyclic. It ROUTES nothing — the turn verbs in chatapi.go
	// dispatch to History and Intake above, which is why the assistant
	// "consumes these layers and nothing else" (S14.10) is checkable by
	// reading one file. nil leaves the chat routes answering 503.
	Chat *chat.Store
	// PollInterval is the idle re-poll cadence of the SSE tail loop. It is
	// deliberately not a ⚙ setting — no such key is ratified; transport
	// refinement belongs to Spec S14 (B5). 0 = default 250ms.
	PollInterval time.Duration
	// DB is the read-only handle for the S14.3 snapshot projections (brief §3):
	// short SELECTs over the existing tables, owner-scoped per S01.9 (OQ1). nil
	// disables snapshot-then-tail — the raw owner-scoped tail still serves
	// (back-compat with the B0-3 endpoint).
	DB *storage.DB
	// Meter is the run-card metering seam (RunMeter); nil leaves the token
	// counter at zero and serves the cost as an absence rather than a zero.
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
	// registry is the full S01.10 registry behind the S15.9 settings family;
	// prices is the S10.3 stored price table (settings.go, B6-3A).
	registry *settings.Registry
	prices   PriceSurface
	healthFn func() Health
	stopping <-chan struct{}
	poll     time.Duration
	logger   *slog.Logger
	clock    func() time.Time
	nudge    *broadcast
	intake   IntakeSurface
	// review is the S13.1–S13.4 store behind the deliverables family (B6-3B).
	review *review.Store
	// accept is the S13.6 orchestration behind the one outward act (B6-3B).
	accept *accept.Accepter
	// followUp is the S13.9 spawn verb, routed by B6-2A (actions.go).
	followUp *intake.FollowUp
	// effects is the S02.7 journal behind the S15.6 effect approvals.
	effects *gates.Journal
	// cancel is the S02.3 cancel choreography behind the 4.5 cancel verbs.
	cancel CancelSurface
	// The B6-2B decision-plane seams (meters_verbs.go, oversight.go).
	budgets  BudgetStore
	pause    PauseStore
	hints    HintSurface
	watchdog SuppressSurface
	resume   ResumeSurface
	// benchmark is the S14.7 practice behind the B6-2C verdict backend.
	benchmark BenchmarkSurface
	// preview is the S13.8 preview surface behind the preview verbs (B6-3B).
	preview *preview.Manager
	// memory + memGate are the S09 read store and the station-3 write gate
	// behind the memory family (B6-3C).
	memory  *memory.Store
	memGate *memory.Gate
	// history is the S14.10 query surface, routed under /api/events (B6-1).
	history *history.Store
	// chat is the S15.7 assistant's store behind /api/chat (B6-7).
	chat *chat.Store
	// push + pushSender are the S15.11 channel behind /api/push and the
	// notifier (B6-9).
	push       *push.Store
	pushSender PushSender
	// workforce is the S08 registry behind /api/workforce (B6-8 part B), read
	// only — no verb in that file touches it.
	workforce *worker.Store
	// proj is the S14.3 snapshot projector (brief §3); nil when no DB is wired
	// (the raw tail still serves).
	proj *projector
	// routes records every pattern Handler registered, in registration order.
	//
	// It exists so the S15.12 "the SPA consumes every API" check (B6-9 R17) can
	// be a comparison between two INDEPENDENTLY PRODUCED lists rather than a
	// list compared to itself: this one is written by the registration calls
	// themselves, so a route added without a client is visible by construction
	// and a hand-maintained inventory — the tautology that hid a live defect at
	// B6-8 — cannot exist here.
	routes []Route
}

// Route is one registered pattern: the method, the path, and whether it sits
// behind the S01.9 identity requirement.
type Route struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Session bool   `json:"session_required"`
}

// Routes returns the registered surface. Handler must have been called; it is
// what does the registering.
func (s *Server) Routes() []Route {
	out := make([]Route, len(s.routes))
	copy(out, s.routes)
	return out
}

// record splits a "METHOD /path" pattern into a Route.
func (s *Server) record(pattern string, session bool) {
	method, path, found := strings.Cut(pattern, " ")
	if !found {
		method, path = "", pattern
	}
	s.routes = append(s.routes, Route{Method: method, Path: path, Session: session})
}

// New assembles the Server.
func New(cfg Config) *Server {
	s := &Server{
		log:        cfg.Log,
		sessions:   cfg.Sessions,
		devPosture: cfg.DevPosture,
		auth:       cfg.Auth,
		settings:   cfg.Settings,
		registry:   cfg.Registry,
		prices:     cfg.Prices,
		healthFn:   cfg.HealthFn,
		stopping:   cfg.Stopping,
		poll:       cfg.PollInterval,
		logger:     cfg.Logger,
		nudge:      newBroadcast(),
		intake:     cfg.Intake,
		review:     cfg.Review,
		accept:     cfg.Accept,
		followUp:   cfg.FollowUp,
		preview:    cfg.Preview,
		memory:     cfg.Memory,
		memGate:    cfg.MemoryGate,
		history:    cfg.History,
		chat:       cfg.Chat,
		push:       cfg.Push,
		pushSender: cfg.PushSender,
		workforce:  cfg.Workforce,
		effects:    cfg.Effects,
		cancel:     cfg.Cancel,
		clock:      cfg.Now,
		budgets:    cfg.Budgets,
		pause:      cfg.Pause,
		hints:      cfg.Hints,
		watchdog:   cfg.Watchdog,
		resume:     cfg.Resume,
		benchmark:  cfg.Benchmark,
	}
	if cfg.DB != nil {
		s.proj = &projector{db: cfg.DB, meter: cfg.Meter, now: s.clock}
	}
	if s.push != nil && s.pushSender == nil {
		s.pushSender = push.NewSender(s.push, nil)
	}
	if s.auth == nil {
		s.auth = SessionAuthenticator{Sessions: cfg.Sessions, DevFallback: cfg.DevPosture}
	}
	if s.poll <= 0 {
		s.poll = 250 * time.Millisecond
	}
	if s.clock == nil {
		s.clock = time.Now
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
	s.routes = nil

	// Pre-session surface.
	open := func(pattern string, h http.HandlerFunc) {
		s.record(pattern, false)
		mux.HandleFunc(pattern, h)
	}
	open("GET /api/health", s.handleHealth)
	open("GET /api/auth/session", s.handleAuthSession)
	open("GET /api/auth/users", s.handleAuthUsers)
	open("POST /api/auth/login", s.handleAuthLogin)
	open("POST /api/auth/users", s.handleAuthUserCreate)

	// Session-required surface.
	protected := func(pattern string, h http.HandlerFunc) {
		s.record(pattern, true)
		mux.Handle(pattern, s.requireIdentity(h))
	}
	protected("GET /events", s.handleEvents)
	protected("POST /api/auth/logout", s.handleAuthLogout)
	protected("POST /api/auth/verify-pin", s.handleAuthVerifyPIN)
	protected("POST /api/auth/pin", s.handleAuthSetPIN)
	protected("GET /api/auth/grants", s.handleAuthGrants)
	protected("POST /api/auth/grants", s.handleAuthGrantCreate)
	protected("POST /api/auth/grants/revoke", s.handleAuthGrantRevoke)

	// The S15.2 read families (B6-1, reads.go): runs + tasks, owner-scoped,
	// filterable, bounded. GET /api/tasks/{task} and GET
	// /api/runs/{run}/receipt keep their walking-skeleton paths and are now
	// owner-scoped server-side; the mutation verbs below stay as B2-4 left
	// them (the decision plane is B6-2's).
	protected("GET /api/runs", s.handleRunList)
	protected("GET /api/runs/{run}", s.handleRunDetail)
	protected("GET /api/runs/{run}/receipt", s.handleRunReceipt)
	protected("GET /api/tasks", s.handleTaskList)
	protected("GET /api/tasks/{task}", s.handleTask)
	protected("GET /api/meters", s.handleMeters)

	// The S14.10 query layers, routed (B6-1, historyapi.go). Layer 2 has its
	// own route because escalation is an act, never a fallback.
	protected("GET /api/events/views", s.handleHistoryViews)
	protected("GET /api/events/views/{view}", s.handleHistoryView)
	protected("GET /api/events/catalog", s.handleHistoryCatalog)
	protected("GET /api/events/query/{query}", s.handleHistoryQuery)
	protected("GET /api/events/ask", s.handleHistoryAsk)
	protected("GET /api/events/search", s.handleHistorySearch)
	protected("GET /api/events/open-sql", s.handleHistoryOpenSQL)

	// Walking-skeleton mutation surface (B2-4). Paths are unchanged — evolution
	// is additive-first (S15.2) — and the S15.6 decision plane below joins them
	// rather than replacing them.
	protected("POST /api/intake/requests", s.handleIntakeSubmit)
	protected("POST /api/tasks/{task}/advance", s.handleTaskAdvance)
	protected("POST /api/asks/{ask}/answer", s.handleAskAnswer)

	// The S15.6 decision plane (B6-2A): the one risk-ranked approval inbox, its
	// hash-pinned answer verbs, the 4.5 cancel verbs and the S13.9 follow-up
	// spawn. Every route is owner-scoped server-side and fail-closed (S01.9);
	// none performs a direct outward effect (D7).
	protected("GET /api/approvals", s.handleApprovalList)
	protected("POST /api/approvals/answer-batch", s.handleApprovalAnswerBatch)
	protected("POST /api/approvals/{id}/answer", s.handleApprovalAnswer)
	protected("POST /api/runs/{run}/cancel", s.handleRunCancel)
	protected("POST /api/tasks/{task}/cancel", s.handleTaskCancel)
	protected("POST /api/deliverables/{deliverable}/follow-up", s.handleFollowUpSpawn)

	// The B6-2B half of the decision plane: the S10.4 meters mutations (budget +
	// pause), the S15.5 board-drag hint, and the four oversight verbs over the
	// landed watchdog / run-FSM / drift / conformance internals. Same rules —
	// owner-scoped server-side, fail-closed, no direct outward effect, no
	// version prefix (S15.2 additive-first).
	protected("POST /api/meters/budget", s.handleBudgetDeclare)
	protected("POST /api/meters/pause", s.handlePauseSet)
	protected("POST /api/tasks/{task}/priority-hint", s.handlePriorityHint)
	protected("POST /api/watchdog/flags/suppress", s.handleFlagSuppress)
	protected("POST /api/runs/{run}/resume", s.handleRunResume)
	protected("POST /api/approvals/{id}/dismiss", s.handleDriftDismiss)
	protected("POST /api/approvals/{id}/acknowledge", s.handleConformanceAcknowledge)

	// The B6-2C third of the decision plane: the BENCH-REG verdict backend
	// (benchmark.go). The two card verbs join the approvals family their cards
	// are listed in; the blind form's data and the §4.2.1 consent flip are their
	// own routes because neither is a card decision. Same rules — owner-scoped
	// server-side, fail-closed, no direct outward effect, no version prefix.
	protected("GET /api/benchmark/verdicts", s.handleBenchmarkVerdicts)
	protected("POST /api/benchmark/opt-in", s.handleBenchmarkOptIn)
	protected("POST /api/approvals/{id}/verdict", s.handleBenchmarkVerdict)
	protected("POST /api/approvals/{id}/decline", s.handleBenchmarkDecline)
	protected("POST /api/approvals/{id}/dispose", s.handleBenchmarkAlarmDispose)

	// The S15.9 settings family (B6-3A, settings.go): the one registry read
	// (schema + UISchema + values + effective clamp bounds + the S18.4 R9
	// registered block), the per-setting audit history from settings_events,
	// the validated write verbs, and the S18.3 price-table surface. Reads are
	// session-required and owner-scope only the per-user overrides; WRITES are
	// operator-only, because the registry's actor model admits no member kind
	// (OQ8). No route here performs an outward effect — a settings write is a
	// control-plane-internal state act (D7).
	//
	// `prices` is a literal segment and every declared key is dotted, so it can
	// never be shadowed by {key}; TestPriceRouteCannotBeShadowedByAKey pins it.
	protected("GET /api/settings", s.handleSettingsRead)
	protected("GET /api/settings/prices", s.handlePriceRead)
	protected("POST /api/settings/prices", s.handlePriceAdd)
	protected("GET /api/settings/{key}/history", s.handleSettingsHistory)
	protected("POST /api/settings/{key}", s.handleSettingsSet)
	protected("POST /api/settings/{key}/bounds", s.handleSettingsBounds)

	// The S15.2 deliverables family (B6-3B: deliverables.go, accept.go,
	// previewapi.go) — the S13 content family. Reads are owner-scoped
	// server-side with 404 before 403; the comment verb is Create only, because
	// S13.3 makes a comment immutable and never deleted and THE drain is its one
	// consumer (OQ2); and the accept is the ONE outward act on this whole API,
	// which exits through the S02.7 journal and the broker inside
	// accept.Accepter and nowhere else (D7).
	//
	// `previews` is its own root because a preview session outlives the surface
	// it was launched from: a person lists and stops sessions, not deliverables.
	protected("GET /api/deliverables", s.handleDeliverableList)
	protected("GET /api/deliverables/{deliverable}", s.handleDeliverableDetail)
	protected("GET /api/deliverables/{deliverable}/compare", s.handleDeliverableCompare)
	protected("GET /api/deliverables/{deliverable}/comments", s.handleCommentList)
	protected("POST /api/deliverables/{deliverable}/comments", s.handleCommentCreate)
	// The object BYTES behind an ObjectRef (B6-8 OQ2b): the S13.2 image trio and
	// the binary card's download-to-inspect need content, and metadata plus a
	// hash is not content. The sha resolves only against THIS deliverable's own
	// revisions, so the route is not a sha oracle (objects.go).
	protected("GET /api/deliverables/{deliverable}/objects/{sha}", s.handleDeliverableObject)
	protected("GET /api/deliverables/{deliverable}/accept-card", s.handleAcceptCard)
	protected("POST /api/deliverables/{deliverable}/accept", s.handleAccept)
	protected("POST /api/deliverables/{deliverable}/preview", s.handlePreviewLaunch)
	protected("POST /api/deliverables/{deliverable}/preview/compare", s.handlePreviewCompare)
	protected("GET /api/previews", s.handlePreviewList)
	protected("POST /api/previews/{session}/stop", s.handlePreviewStop)

	// The S15.2 memory family (B6-3C: memory.go) — the S09 content family. Reads
	// are scoped server-side to what the caller may SEE (own entries + house +
	// their projects, 404 before 403), and every write is a call on the S09.4
	// station-3 gate: this transport constructs no knowledge SQL and implements
	// none of the gate's walls a second time. Own-store writes are tier Medium
	// (S15.2), so there is no PIN step-up and no batch verb; no route here
	// performs an outward effect — a knowledge write is control-plane-internal
	// state (D7).
	//
	// `conflicts` sits one segment deeper than {entry}, so an entry id can never
	// shadow it and it can never swallow an entry read.
	// The S15.7 conversational assistant (B6-7: chatapi.go) — the family the
	// S15.2 table never listed, added additive-first under its own root. Reads
	// and writes are OWNER-ONLY server-side: the store takes a viewer and has no
	// role parameter, so the operator does not read another member's transcripts
	// (a deliberate narrowing of §30's operator-sees-all, which is an
	// observability rule; a conversation is content — the §40-C line). Messages
	// are immutable: there is no edit verb and no message delete, and a session
	// delete is the owner's hard delete of the whole thread. No route here
	// performs an outward effect — a turn reads the query layers or hands a
	// request to intake, both control-plane-internal (D7).
	protected("GET /api/chat/sessions", s.handleChatSessionList)
	protected("POST /api/chat/sessions", s.handleChatSessionCreate)
	protected("GET /api/chat/sessions/{session}", s.handleChatSessionDetail)
	protected("POST /api/chat/sessions/{session}/rename", s.handleChatSessionRename)
	protected("POST /api/chat/sessions/{session}/delete", s.handleChatSessionDelete)
	protected("POST /api/chat/sessions/{session}/turns", s.handleChatTurnSubmit)
	protected("POST /api/chat/turns/{turn}/stop", s.handleChatTurnStop)
	protected("GET /api/chat/files", s.handleChatFileList)
	protected("POST /api/chat/files", s.handleChatFileUpload)
	protected("POST /api/chat/files/{file}/delete", s.handleChatFileDelete)

	// The S15.10 workforce map (B6-8 part B: workforce.go) — ONE read and
	// nothing else. The roster is scoped server-side to what the caller may see
	// (own personal workers plus the household roster; the operator reads the
	// whole registry, because a worker is audited machinery gated by D10 acts and
	// not personal content), and the per-version outcome figures are scoped by
	// the RUN's owner besides. There is no mutation verb here at any shape:
	// editing through the map is parked to 15.5, so the surface performs no act
	// and has no audit row to name.
	protected("GET /api/workforce", s.handleWorkforceRead)

	// The S15.11 Web Push family (B6-9: push.go) — the family the S15.2 table
	// never listed, added additive-first under its own root like /api/chat. It
	// records which devices the notifier may reach and nothing else: reads are
	// owner-scoped server-side and answer with METADATA (an endpoint is a
	// capability URL and is never served, to anybody), and neither verb performs
	// an outward effect — enrolling a device releases nothing, and the sending
	// is the notifier's, driven by the shell.
	//
	// `remove` sits one segment deeper than the collection so it can never be
	// read as a subscription id, and it is a POST because the body carries the
	// endpoint the browser holds rather than the id the platform minted.
	protected("GET /api/push/subscriptions", s.handlePushList)
	protected("POST /api/push/subscriptions", s.handlePushEnrol)
	protected("POST /api/push/subscriptions/remove", s.handlePushRemove)

	protected("GET /api/memory", s.handleMemoryList)
	protected("POST /api/memory", s.handleMemoryCreate)
	protected("POST /api/memory/conflicts/{conflict}/resolve", s.handleMemoryConflictResolve)
	protected("GET /api/memory/{entry}", s.handleMemoryDetail)
	protected("POST /api/memory/{entry}/new-version", s.handleMemoryNewVersion)
	protected("POST /api/memory/{entry}/remove", s.handleMemoryRemove)
	protected("POST /api/memory/{entry}/delete", s.handleMemoryDelete)

	// The SPA embedded in this binary sits in FRONT of the mux rather than in
	// it, so the machine surface keeps its own 404s and 405s untouched and no
	// API path can ever be answered with HTML (spa.go).
	return s.identity(s.withSPA(mux))
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

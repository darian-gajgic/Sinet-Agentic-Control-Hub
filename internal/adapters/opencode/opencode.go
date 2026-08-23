// Package opencode is the pinned `opencode serve` substrate of Spec S03.2
// behind the D3 contract (internal/adapters). It carries the Z.AI and local
// lanes as CONFIG DATA; nothing lane-specific is compiled in (S03.6: adding a
// lane is a provider entry plus billing flags, never a new substrate).
//
// Engine pin: components.lock entry "opencode-ai (engine)". The engine runs
// UNMODIFIED (S16.1, S16.6): every platform opinion — lowering, parsing,
// gating, checkpoints — lives on this side of the seam, integration is
// config/env/HTTP only, and `POST /global/upgrade` is never exposed or called
// (S03.3 rule 2). Bumps land only through the S03.3 operator-gated procedure.
//
// The seven D3 verbs map onto this substrate as:
//
//	start        → POST /session + POST /session/{id}/prompt_async
//	stream       → v1 SSE GET /event, normalized to the closed platform set
//	checkpoint   → one Usage event per paid call; the Driver persists the row
//	pause        → interrupt at a message boundary + park (never mid-tool-call)
//	resume       → answer the held ask, or re-prompt the surviving session
//	cancel       → POST /session/{id}/abort → process-group TERM → KILL
//	report_usage → per-message .tokens forwarded; Sinet prices, never opencode
package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
)

// DefaultBinary is the engine executable resolved via PATH when the instance
// manager is not configured with an explicit path.
const DefaultBinary = "opencode"

// Pin is the binding engine version (components.lock "opencode-ai (engine)";
// Spec S16.3) — the adapter's exported pin per CONVENTIONS §10.
// TestPinMatchesLock keeps it mechanically coupled to the lock entry; a
// pin↔installed delta is reported LOUDLY and never silently retargeted.
const Pin = "1.18.3"

// BasicAuthUser is the HTTP Basic USERNAME the pinned engine accepts alongside
// OPENCODE_SERVER_PASSWORD. It is part of the credential, not decoration:
// live-recorded 2026-08-23 on the pinned binary, any other username is refused
// 401. (SPIKE P2-S1 recorded `-u <anyuser>:<pw>` → 200; the measured behavior
// at the pin on this host is narrower, and the measurement wins.)
const BasicAuthUser = "opencode"

// cancelGrace is the abort→group-TERM grace of the cancel ladder. Not a ⚙ —
// S18 ratifies no such key (the sseBatchSize precedent, CONVENTIONS §7).
const cancelGrace = 5 * time.Second

// streamReconnects bounds how many times a dropped event stream is re-opened
// before the turn is assembled from what was witnessed. One is enough to cover
// a health blip; more would be a supervisor, which this adapter is not.
const streamReconnects = 1

// decisionKey carries a gate decision through adapters.Answer.UpdatedInput.
//
// The shared Answer struct has no approve/reject member: on the Claude lane the
// answer is always an allow (optionally with replacement input), so rejection
// never needed a field. This substrate's `permission.reply` takes {reply,
// message} and has NO input-substitution channel at all, which makes
// UpdatedInput unusable for its Claude-lane meaning here and free to carry the
// decision instead. Use ApproveAnswer / RejectAnswer rather than building the
// envelope by hand. A raw replacement input is REFUSED (ErrUpdatedInputUnsupported):
// silently dropping a human's edit would execute the parked call unreviewed,
// which is the exact S03.4 failure the park exists to prevent.
const decisionKey = "sinet.decision"

const (
	decisionApprove = "approve"
	decisionReject  = "reject"
)

// Errors callers branch on.
var (
	// ErrConfinementUnsupported refuses a per-RUN sandbox on this substrate.
	// S11.1 places confinement around the whole per-USER server (a per-user
	// bwrap+netns jail scoped to that person's workspaces and model egress) and
	// treats opencode's own permission config as an inner soft layer only — so
	// per-run confinement classes never apply here. Running unconfined while a
	// caller believed otherwise is the failure this refusal prevents; it is the
	// permanently correct semantics, not an interim guard.
	ErrConfinementUnsupported = errors.New("opencode: per-run confinement does not apply to a per-user serve (S11.1: the jail is per-user)")

	// ErrNativeSpawnTool refuses any input that would re-admit the engine's
	// native subagent tool (S03.5 sole-controller posture).
	ErrNativeSpawnTool = errors.New("opencode: engine-native subagent spawning is structurally disabled")

	// ErrUpdatedInputUnsupported refuses an answer carrying replacement tool
	// input: 1.18.3's permission.reply cannot substitute a gated call's input.
	ErrUpdatedInputUnsupported = errors.New("opencode: this engine cannot substitute a gated call's input on resume")

	// ErrNoEngineSession refuses a resume from a park record that never
	// recorded an engine session id.
	ErrNoEngineSession = errors.New("opencode: park record without an engine session id")

	// ErrInstanceUnavailable reports that no per-user serve endpoint could be
	// obtained for this invocation.
	ErrInstanceUnavailable = errors.New("opencode: no serve instance available")
)

// ProviderConfig is the lane's provider block, compiled into the engine config
// as DATA (S03.6: lane onboarding is a provider entry plus billing flags, never
// a new substrate). LN-1 defines the shape and feeds it fixture values only;
// the lane packets supply real endpoints. Provider CREDENTIALS never belong
// here — they ride the per-user credential root or the broker's spawn-time
// injection (D2/S11.5).
type ProviderConfig map[string]ProviderEntry

// ProviderEntry is one opencode provider definition.
type ProviderEntry struct {
	NPM     string                `json:"npm,omitempty"`
	Name    string                `json:"name,omitempty"`
	Options map[string]any        `json:"options,omitempty"`
	Models  map[string]ModelEntry `json:"models,omitempty"`
}

// ModelEntry is one model under a provider entry.
type ModelEntry struct {
	Name string `json:"name,omitempty"`
}

// Adapter is the opencode-substrate implementation of the D3 contract
// (Spec S03.2). One Adapter value serves many runs; each Start/Resume yields
// one Session (one engine turn) against this user's serve instance.
type Adapter struct {
	// Instances supplies the per-user serve endpoint (the R3 seam). The
	// dev-posture Manager implements it now; the D6 `sinet-engine@<user>` unit
	// replaces the provider without touching this adapter.
	Instances Instances

	// Providers is the lane provider block compiled into every invocation's
	// config. Lane values are DATA, never compiled-in identifiers.
	Providers ProviderConfig

	// Root is the platform-owned parent of the per-user XDG roots (0700 each).
	// A request's OwnerCredRef overrides it for that user (D2: the credential
	// ref is a PATH, never material).
	Root string

	// Env overrides the ambient environment as the lowering input base (tests).
	// Nil = os.Environ().
	Env []string

	// Now is the clock seam (tests). Nil = time.Now.
	Now func() time.Time

	// Log receives forward-tolerance skips and lifecycle notices.
	// Nil = slog.Default (ops log, S01.11 posture).
	Log *slog.Logger

	// HTTP is the client used for the serve API. Nil builds one per session;
	// it carries no client-level timeout because the event stream is
	// long-lived — every unary call bounds itself instead.
	HTTP *http.Client

	// CancelGrace overrides the abort→group-TERM grace (tests). 0 = cancelGrace.
	CancelGrace time.Duration
}

var _ adapters.Adapter = (*Adapter)(nil)

// Substrate implements adapters.Adapter.
func (a *Adapter) Substrate() string { return adapters.SubstrateOpencode }

func (a *Adapter) environ() []string {
	if a.Env != nil {
		return a.Env
	}
	return os.Environ()
}

func (a *Adapter) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a *Adapter) logger() *slog.Logger {
	if a.Log != nil {
		return a.Log
	}
	return slog.Default()
}

func (a *Adapter) logf(format string, args ...any) {
	a.logger().Info(fmt.Sprintf(format, args...))
}

// warnf is the LOUD half of the same ops channel: engine behavior the platform
// absorbed but an operator must be able to see (S01.11 posture).
func (a *Adapter) warnf(format string, args ...any) {
	a.logger().Warn(fmt.Sprintf(format, args...))
}

func (a *Adapter) cancelGrace() time.Duration {
	if a.CancelGrace > 0 {
		return a.CancelGrace
	}
	return cancelGrace
}

// Start implements adapters.Adapter (S03.1 start): acquire this user's serve
// instance with the compiled config, subscribe to the event stream BEFORE the
// session exists so nothing the turn emits can be missed, create the session,
// and prompt.
func (a *Adapter) Start(ctx context.Context, req adapters.StartRequest) (adapters.Session, error) {
	l, inst, err := a.prepare(ctx, req)
	if err != nil {
		return nil, err
	}
	c := newClient(inst, a.HTTP)
	stream, err := c.events(ctx)
	if err != nil {
		return nil, err
	}
	sessionID, directory, err := c.createSession(ctx)
	if err != nil {
		stream.Close()
		return nil, err
	}
	s := a.newSession(req, l, c, sessionID, directory)
	if err := c.promptAsync(ctx, sessionID, s.promptBody(l.prompt)); err != nil {
		stream.Close()
		return nil, err
	}
	go s.pump(ctx, stream)
	return s, nil
}

// Resume implements adapters.Adapter (S03.1 resume; S03.4 obligation): the
// ENTIRE invocation is re-supplied from the park record — compiled config,
// permission map, model, agent — because the engine restores nothing.
//
// Two paths, and which one applies is DIFFED against the engine, never assumed:
//
//   - the serve still holds the ask ⇒ answer it (`POST /permission/{id}/reply`),
//     a defer-style unpark that resumes the same in-flight turn at no extra cost;
//   - the ask is gone but the session survives ⇒ the park died with a serve
//     restart or crash (pending asks are in-memory Maps, rejected SILENTLY on
//     shutdown and lost on crash — #36347, unfixed). The orphaned call is marked
//     cancelled in the platform's OWN record, and recovery is a re-prompt of the
//     surviving session id: a normal PAID turn.
//
// Freshness-gating (S02.6) happens before this call, in the caller.
func (a *Adapter) Resume(ctx context.Context, rec adapters.ParkRecord, ans *adapters.Answer) (adapters.Session, error) {
	if rec.Cursor.SessionID == "" {
		return nil, ErrNoEngineSession
	}
	answering := ans != nil && ans.AskID != ""
	if rec.Reason == adapters.ParkReasonGateAsk && !answering {
		return nil, fmt.Errorf("opencode: resuming a gate park needs the answer that unparks it (ask %q)", askIDOf(rec))
	}
	reply, feedback, err := decodeDecision(ans)
	if err != nil {
		return nil, err
	}
	l, inst, err := a.prepare(ctx, rec.Start)
	if err != nil {
		return nil, err
	}
	c := newClient(inst, a.HTTP)

	if answering {
		held, err := heldAsk(ctx, c, rec.Cursor.SessionID, ans.AskID)
		if err != nil {
			return nil, err
		}
		if held {
			stream, err := c.events(ctx)
			if err != nil {
				return nil, err
			}
			s := a.newSession(rec.Start, l, c, rec.Cursor.SessionID, rec.Cursor.CwdKey)
			// The turn this unparks is still in flight on the serve; without
			// this the stream's idle would be read as a turn that never began.
			s.p.assumeInFlight()
			if err := c.reply(ctx, ans.AskID, reply, feedback); err != nil {
				stream.Close()
				return nil, err
			}
			go s.pump(ctx, stream)
			return s, nil
		}
		a.warnf("opencode: ask %q is gone from permission.list on session %s — the park died with a serve restart "+
			"(pending asks are in-memory and rejected silently); recovering by re-prompting the surviving session (a paid turn)",
			ans.AskID, rec.Cursor.SessionID)
	}

	// Re-prompt path: the session must actually still exist. Prompting an id the
	// serve no longer holds would look like recovery and be nothing.
	alive, err := sessionAlive(ctx, c, rec.Cursor.SessionID)
	if err != nil {
		return nil, err
	}
	if !alive {
		return nil, fmt.Errorf("%w: session %s is gone from the serve; recovery is the S02.5 fork ladder's, not this adapter's",
			ErrNoEngineSession, rec.Cursor.SessionID)
	}
	stream, err := c.events(ctx)
	if err != nil {
		return nil, err
	}
	s := a.newSession(rec.Start, l, c, rec.Cursor.SessionID, rec.Cursor.CwdKey)
	if answering {
		// The platform's own record of the orphan: opencode leaves the
		// transcript part cosmetically `running` and it is never trusted.
		s.orphaned = orphanPayload(rec, ans.AskID)
	}
	if err := c.promptAsync(ctx, rec.Cursor.SessionID, s.promptBody(resumePrompt(rec, ans, feedback))); err != nil {
		stream.Close()
		return nil, err
	}
	go s.pump(ctx, stream)
	return s, nil
}

// prepare lowers the invocation and obtains this user's serve instance.
func (a *Adapter) prepare(ctx context.Context, req adapters.StartRequest) (*lowered, Instance, error) {
	if req.Confiner != nil {
		return nil, Instance{}, fmt.Errorf("%w (run %q, class %q)", ErrConfinementUnsupported, req.RunID, req.Class)
	}
	l, err := a.lower(req)
	if err != nil {
		return nil, Instance{}, err
	}
	env := l.env
	if req.CredInject != nil {
		// Broker credential injection at spawn (Spec S11.5; S01.6 "engines
		// receive credentials at start"): resolved FRESH, never stored. On this
		// substrate the injection transforms the SERVE environment, because the
		// server — not a per-run process — is what holds the provider session.
		injected, err := req.CredInject(env)
		if err != nil {
			return nil, Instance{}, fmt.Errorf("opencode: inject credentials (S11.5): %w", err)
		}
		env = injected
	}
	if a.Instances == nil {
		return nil, Instance{}, fmt.Errorf("%w: adapter has no instance provider", ErrInstanceUnavailable)
	}
	inst, err := a.Instances.Acquire(ctx, a.instanceSpec(req, l, env))
	if err != nil {
		return nil, Instance{}, fmt.Errorf("%w: %s", ErrInstanceUnavailable, err)
	}
	return l, inst, nil
}

// ApproveAnswer builds the approve decision for a gate ask: `reply:"once"` on
// the wire. Never `always` — v1 always-rules are in-memory AND server-wide, so
// they leak approvals across a person's sessions and vanish on restart; durable
// allow-policy is Sinet's own layer (S03.4).
func ApproveAnswer(askID string) *adapters.Answer {
	return &adapters.Answer{AskID: askID}
}

// RejectAnswer builds the reject-with-feedback decision: `reply:"reject"` plus
// the message the engine hands the model as a CorrectedError.
func RejectAnswer(askID, feedback string) *adapters.Answer {
	env, err := json.Marshal(map[string]string{decisionKey: decisionReject, "message": feedback})
	if err != nil {
		return &adapters.Answer{AskID: askID}
	}
	return &adapters.Answer{AskID: askID, UpdatedInput: env}
}

// decodeDecision maps an Answer onto the two replies this adapter may send.
func decodeDecision(ans *adapters.Answer) (reply, message string, err error) {
	if ans == nil || ans.AskID == "" {
		return "", "", nil
	}
	if len(ans.UpdatedInput) == 0 {
		return replyOnce, "", nil
	}
	var env struct {
		Decision string `json:"sinet.decision"`
		Message  string `json:"message"`
	}
	if uerr := json.Unmarshal(ans.UpdatedInput, &env); uerr == nil {
		switch env.Decision {
		case decisionApprove:
			return replyOnce, "", nil
		case decisionReject:
			return replyReject, env.Message, nil
		}
	}
	return "", "", fmt.Errorf("%w: use ApproveAnswer/RejectAnswer — dropping the edit silently would execute the parked call unreviewed (S03.4)",
		ErrUpdatedInputUnsupported)
}

// heldAsk diffs the platform ask-record against the engine's flat pending array
// (S03.4). Shutdown rejection is SILENT — no bus event ever announces a dead
// park — so this diff is the only honest way to learn which path resume takes.
func heldAsk(ctx context.Context, c *client, sessionID, askID string) (bool, error) {
	list, err := c.permissions(ctx)
	if err != nil {
		return false, err
	}
	for _, pr := range list {
		if pr.ID == askID && (pr.SessionID == "" || pr.SessionID == sessionID) {
			return true, nil
		}
	}
	return false, nil
}

func sessionAlive(ctx context.Context, c *client, sessionID string) (bool, error) {
	list, err := c.sessions(ctx)
	if err != nil {
		return false, err
	}
	for _, s := range list {
		if s.ID == sessionID {
			return true, nil
		}
	}
	return false, nil
}

func askIDOf(rec adapters.ParkRecord) string {
	var pr permissionRequest
	if err := json.Unmarshal(rec.DeferredToolUse, &pr); err != nil {
		return ""
	}
	return pr.ID
}

// resumePrompt is the user turn a re-prompt re-supplies. A caller-supplied
// continuation wins; a rejection's feedback is the next best thing to say; the
// original prompt is the last resort, because a dead park means the turn's work
// was never done.
func resumePrompt(rec adapters.ParkRecord, ans *adapters.Answer, feedback string) string {
	if ans != nil && ans.Continuation != "" {
		return ans.Continuation
	}
	if feedback != "" {
		return feedback
	}
	return rec.Start.Worker.Prompt
}

// orphanPayload is the platform's own record that a parked call never ran.
func orphanPayload(rec adapters.ParkRecord, askID string) json.RawMessage {
	var pr permissionRequest
	_ = json.Unmarshal(rec.DeferredToolUse, &pr)
	payload, err := json.Marshal(struct {
		ToolUseID string `json:"tool_use_id,omitempty"`
		Tool      string `json:"tool,omitempty"`
		AskID     string `json:"ask_id"`
		Status    string `json:"status"`
		Cancelled bool   `json:"cancelled"`
		Excerpt   string `json:"excerpt"`
	}{pr.Tool.CallID, pr.Permission, askID, "cancelled", true,
		"the serve lost this ask on restart; the call never ran. opencode leaves its own transcript part " +
			"cosmetically `running`, which is not trusted (S03.4)"})
	if err != nil {
		return json.RawMessage(`{"cancelled":true}`)
	}
	return payload
}

// ── session ──────────────────────────────────────────────────────────────────

// session is one live engine turn on this user's serve instance.
type session struct {
	a         *Adapter
	req       adapters.StartRequest
	low       *lowered
	c         *client
	p         *parser
	sessionID string

	events chan adapters.Event
	done   chan struct{}

	mu             sync.Mutex
	cursor         adapters.Cursor
	outcome        adapters.Outcome
	waitErr        error
	paused         bool
	pauseAborted   bool
	cancelStage    int
	atBoundary     bool
	reconciledIdle bool
	streamDetail   string
	orphaned       json.RawMessage
}

var _ adapters.Session = (*session)(nil)

func (a *Adapter) newSession(req adapters.StartRequest, l *lowered, c *client, sessionID, directory string) *session {
	s := &session{
		a: a, req: req, low: l, c: c, sessionID: sessionID,
		p:      newParser(sessionID, a.logf),
		events: make(chan adapters.Event, 64),
		done:   make(chan struct{}),
		// Before the first message nothing is mid-flight: interrupting here
		// parks with zero progress, which is safe (P-T01-4).
		atBoundary: true,
	}
	s.cursor = adapters.Cursor{
		Substrate: adapters.SubstrateOpencode,
		// The engine mints and REPORTS its own session id; the platform never
		// requests one on this substrate (S02.4b).
		SessionID: sessionID,
		CwdKey:    directory,
		// opencode keeps its store in SQLite (`event`/`event_sequence`), not a
		// JSONL transcript: there is nothing to copy aside, so the Driver's
		// copy-aside is naturally inert rather than disabled (S02.4).
		TranscriptPath: "",
	}
	s.p.onAsk = func(ask *adapters.Ask, raw json.RawMessage) {
		// The park record is attached BEFORE the event leaves, so the durable
		// ask row the Driver writes carries the full invocation snapshot.
		ask.Park = s.parkRecord(s.Cursor(), adapters.ParkReasonGateAsk, raw)
	}
	return s
}

func (s *session) promptBody(text string) promptBody {
	return promptBody{
		Model:  s.low.model,
		Agent:  s.low.agentName,
		System: s.low.systemAppend,
		Parts:  []promptPart{{Type: "text", Text: text}},
	}
}

func (s *session) Events() <-chan adapters.Event { return s.events }

func (s *session) Cursor() adapters.Cursor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cursor
}

func (s *session) Fingerprint() string { return s.low.fingerprint }

// Pause implements the S03.1 pause verb: interrupt-at-safe-boundary + park.
// The boundary is a completed message with no tool call in flight — never
// mid-tool-call (P-T01-4). If the session is at one now the abort lands
// immediately, otherwise at the next boundary. The park record arrives via Wait.
func (s *session) Pause(ctx context.Context) error {
	s.mu.Lock()
	s.paused = true
	fire := s.atBoundary && s.cancelStage == 0 && !s.pauseAborted
	if fire {
		s.pauseAborted = true
	}
	s.mu.Unlock()
	if !fire {
		return nil
	}
	return s.c.abort(ctx, s.sessionID)
}

// Cancel implements the S03.1 cancel ladder: the cooperative engine abort
// first, then the process group after a grace.
//
// The group rung ends this user's whole serve instance, because that is what
// the process group IS on a per-user server — so it only fires when the
// cooperative abort failed to end the turn. Per-(user,lane) concurrency is 1 at
// v0, so the instance being reaped is serving this run and no other.
func (s *session) Cancel(ctx context.Context) error {
	s.mu.Lock()
	if s.cancelStage != 0 {
		s.mu.Unlock()
		return nil
	}
	s.cancelStage = 1
	s.mu.Unlock()

	if err := s.c.abort(ctx, s.sessionID); err != nil {
		s.a.warnf("opencode: cooperative abort of session %s failed: %v", s.sessionID, err)
	}
	grace := s.a.cancelGrace()
	go func() {
		select {
		case <-s.done:
		case <-time.After(grace):
			s.mu.Lock()
			s.cancelStage = 2
			s.mu.Unlock()
			if s.a.Instances == nil {
				return
			}
			s.a.warnf("opencode: session %s survived the cooperative abort for %s — reaping the serve process group",
				s.sessionID, grace)
			if err := s.a.Instances.Stop(context.Background(), s.req.UserID); err != nil {
				s.a.warnf("opencode: reap serve instance for user %q: %v", s.req.UserID, err)
			}
		}
	}()
	return nil
}

func (s *session) Wait(ctx context.Context) (adapters.Outcome, error) {
	select {
	case <-ctx.Done():
		return adapters.Outcome{}, ctx.Err()
	case <-s.done:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outcome, s.waitErr
}

// pump consumes the event stream, then assembles the Outcome.
func (s *session) pump(ctx context.Context, stream io.ReadCloser) {
	if len(s.orphaned) > 0 {
		s.events <- adapters.Event{Kind: adapters.KindToolResult, Payload: s.orphaned}
	}
	s.consume(ctx, stream)
	out, extra := s.assembleOutcome()
	for _, ev := range extra {
		s.events <- ev
	}
	s.mu.Lock()
	s.outcome = out
	s.mu.Unlock()
	close(s.events)
	close(s.done)
}

// consume reads frames until the turn reaches a terminal state, reconnecting a
// dropped stream a bounded number of times.
func (s *session) consume(ctx context.Context, stream io.ReadCloser) {
	for attempt := 0; ; attempt++ {
		err := readFrames(stream, func(raw []byte) bool { return s.handleFrame(ctx, raw) }, s.a.logf)
		stream.Close()
		if s.terminal() {
			return
		}
		if err != nil {
			s.noteStream(fmt.Sprintf("event stream read: %v", err))
		}
		if attempt >= streamReconnects || ctx.Err() != nil {
			s.noteStream("the engine closed the event stream before the turn reached idle")
			return
		}
		// A reconnect carries NO Last-Event-ID: this engine ignores it (the fix
		// was declined upstream), so the reconnect reconciles by STATE instead
		// — and the platform's own /events resume stays reconstructed from
		// run_events alone (S14.5).
		next, err := s.c.events(ctx)
		if err != nil {
			s.noteStream(fmt.Sprintf("event stream reconnect: %v", err))
			return
		}
		stream = next
		if s.reconcile(ctx) {
			stream.Close()
			return
		}
	}
}

func (s *session) terminal() bool {
	s.mu.Lock()
	reconciled := s.reconciledIdle
	s.mu.Unlock()
	return s.p.ask != nil || s.p.sawIdle || reconciled
}

func (s *session) noteStream(detail string) {
	s.mu.Lock()
	if s.streamDetail == "" {
		s.streamDetail = detail
	}
	s.mu.Unlock()
}

// handleFrame folds one frame into the contract and reports whether to keep
// reading.
func (s *session) handleFrame(ctx context.Context, raw []byte) bool {
	evs := s.p.feed(raw)
	s.syncCursor()
	for _, ev := range evs {
		s.events <- ev
	}
	if s.p.ask != nil {
		// The turn parked. Enumerate the WHOLE pending array before stopping:
		// a parallel turn parks every call simultaneously, and each sibling
		// owes its own durable ask record (S03.4).
		s.enumeratePendingAsks(ctx)
		return false
	}
	s.checkPauseBoundary(ctx)
	return !s.p.sawIdle
}

func (s *session) syncCursor() {
	s.mu.Lock()
	s.cursor.MessageIndex = s.p.paidCalls
	if s.p.directory != "" {
		s.cursor.CwdKey = s.p.directory
	}
	s.mu.Unlock()
}

func (s *session) enumeratePendingAsks(ctx context.Context) {
	list, err := s.c.permissions(ctx)
	if err != nil {
		s.a.warnf("opencode: could not enumerate pending permissions on session %s: %v — "+
			"siblings of a parallel park may lack a durable record", s.sessionID, err)
		return
	}
	for _, pr := range list {
		if pr.SessionID != "" && pr.SessionID != s.sessionID {
			continue
		}
		for _, ev := range s.p.emitAsk(pr, nil) {
			s.events <- ev
		}
	}
}

// checkPauseBoundary fires a pending pause at the first safe boundary.
func (s *session) checkPauseBoundary(ctx context.Context) {
	s.mu.Lock()
	s.atBoundary = !s.p.toolRunning
	fire := s.paused && s.atBoundary && s.cancelStage == 0 && !s.pauseAborted
	if fire {
		s.pauseAborted = true
	}
	s.mu.Unlock()
	if fire {
		if err := s.c.abort(ctx, s.sessionID); err != nil {
			s.a.warnf("opencode: pause abort of session %s failed: %v", s.sessionID, err)
		}
	}
}

// reconcile re-derives the turn's state after a stream drop, by STATE rather
// than by replay: the pending-permission diff first (a park that happened while
// the stream was down is still a park), then the session status.
func (s *session) reconcile(ctx context.Context) bool {
	if list, err := s.c.permissions(ctx); err == nil {
		for _, pr := range list {
			if pr.SessionID != "" && pr.SessionID != s.sessionID {
				continue
			}
			for _, ev := range s.p.emitAsk(pr, nil) {
				s.events <- ev
			}
		}
		if s.p.ask != nil {
			return true
		}
	} else {
		s.a.warnf("opencode: reconcile could not read the pending permissions of session %s: %v", s.sessionID, err)
	}
	statuses, err := s.c.statuses(ctx)
	if err != nil {
		s.a.warnf("opencode: reconcile could not read session status: %v", err)
		return false
	}
	st, listed := statuses[s.sessionID]
	if !listed || st.Type == "idle" {
		s.mu.Lock()
		s.reconciledIdle = true
		s.mu.Unlock()
		return true
	}
	return false
}

// assembleOutcome maps what was witnessed onto the contract Outcome.
//
// OutcomeDiedAtGate is unreachable here and is never minted: it exists for the
// S03.4 ceiling-ordering race, and `opencode serve` 1.18.3 exposes no per-turn
// engine budget belt at all, so there is no engine ceiling that could pre-empt
// a park. Outcome.GateFallback stays false for the same kind of reason: the
// Claude lane's single-tool-call defer trap has no analogue here.
func (s *session) assembleOutcome() (adapters.Outcome, []adapters.Event) {
	p := s.p
	s.mu.Lock()
	cur := s.cursor
	paused := s.paused
	canceled := s.cancelStage > 0
	streamDetail := s.streamDetail
	idle := p.sawIdle || s.reconciledIdle
	s.mu.Unlock()

	// A turn that ended by the model stopping, with no engine-reported error.
	naturalEnd := idle && p.errDetail == "" && p.lastFinish == finishStop

	var out adapters.Outcome
	switch {
	case p.ask != nil:
		rec := s.parkRecord(cur, adapters.ParkReasonGateAsk, p.askRaw)
		// A COPY, never a mutation: the Ask that rode the gate_ask event is
		// already the consumer's — the Driver may still be marshalling it into
		// the durable ask row while this runs (proven by -race). The Outcome
		// carries its own value, whose park record is snapshotted at the
		// outcome's cursor rather than at the moment of observation.
		ask := *p.ask
		ask.Park = rec
		out = adapters.Outcome{Kind: adapters.OutcomeParked, Park: rec, Ask: &ask}

	case naturalEnd:
		// A cancel or pause that raced a completed turn LOSES: the engine
		// finished the work and the result stands.
		out = adapters.Outcome{Kind: adapters.OutcomeCompleted, ResultText: s.resultText()}
		switch {
		case canceled:
			out.Detail = "cancel requested but the turn completed first"
		case paused:
			out.Detail = "pause requested but the turn completed first"
		}

	case canceled:
		out = adapters.Outcome{Kind: adapters.OutcomeCanceled, Detail: "cancel ladder (S03.1)"}

	case paused:
		out = adapters.Outcome{Kind: adapters.OutcomeParked,
			Park: s.parkRecord(cur, adapters.ParkReasonPause, nil)}

	case p.errDetail != "":
		out = adapters.Outcome{Kind: adapters.OutcomeCrashed, Detail: p.errDetail}

	case idle:
		// The engine says the turn ended; it just never said `stop` (an
		// interrupted or reconciled turn). A success the engine reported is
		// never re-minted as a crash — an unusable result fails its consumer's
		// contract, loudly, where the run's recovery posture applies.
		out = adapters.Outcome{Kind: adapters.OutcomeCompleted, ResultText: s.resultText()}

	default:
		detail := "the event stream ended before the turn reached idle"
		if streamDetail != "" {
			detail = streamDetail
		}
		out = adapters.Outcome{Kind: adapters.OutcomeCrashed, Detail: detail}
	}
	return out, []adapters.Event{{Kind: adapters.KindDone, Payload: s.donePayload(out)}}
}

// resultText reports the turn's answer VERBATIM (R19; CONVENTIONS §60).
//
// The claudecli precedence has three steps because that engine emits a terminal
// envelope with its own `result` field. This one emits no envelope at all: the
// final assistant message IS the result, so step (1) is structurally absent and
// step (2) — the stream's own witnessed final assistant text — is the answer.
// Nothing is repaired, completed or fabricated here; delimiter completion lives
// at the stage layer.
func (s *session) resultText() string {
	if s.p.finalText != "" {
		return s.p.finalText
	}
	s.a.warnf("opencode: session %s ended with no assistant text in the stream — the caller gets no text (S03.1)",
		s.sessionID)
	return ""
}

// parkRecord snapshots the FULL invocation for resume (S03.4 obligation: the
// engine restores nothing, so a resume built from less silently executes parked
// calls unreviewed).
func (s *session) parkRecord(cur adapters.Cursor, reason string, deferred json.RawMessage) *adapters.ParkRecord {
	return &adapters.ParkRecord{
		RunID:           s.req.RunID,
		Substrate:       adapters.SubstrateOpencode,
		Cursor:          cur,
		Reason:          reason,
		DeferredToolUse: deferred,
		Start:           s.req,
		Fingerprint:     s.low.fingerprint,
		ParkedAt:        s.a.now(),
	}
}

func (s *session) donePayload(out adapters.Outcome) json.RawMessage {
	payload, err := json.Marshal(struct {
		Outcome   string `json:"outcome"`
		Finish    string `json:"finish,omitempty"`
		PaidCalls int64  `json:"paid_calls"`
		Detail    string `json:"detail,omitempty"`
	}{string(out.Kind), s.p.lastFinish, s.p.paidCalls, out.Detail})
	if err != nil {
		return json.RawMessage("{}")
	}
	return payload
}

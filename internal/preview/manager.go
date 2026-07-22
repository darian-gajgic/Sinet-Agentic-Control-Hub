package preview

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/sandbox"
)

// manager.go is the S13.8 preview lifecycle manager: launch → disposition →
// substrate → routing → state, and idempotent teardown. Live-preview state is
// in-memory (Spec S13.8: previews are disposable — no durable preview table,
// R23) plus the R22 event trace. Launching is Low-tier reversible (R22): no
// effect journal, no approval, no PIN step-up.

// ⚙ keys consumed by dotted key through the narrow Settings interface (Spec
// S13.8; R15) — never constants.
const (
	keyIdleStop      = "preview.idle_stop"
	keyMaxConcurrent = "preview.max_concurrent"
)

// Reviews resolves deliverables, revision pins, and the accepted revision — the
// durable inputs a preview reads (never mints/drains/accepts, R24).
type Reviews interface {
	Deliverable(ctx context.Context, id string) (review.Deliverable, error)
	RevisionAt(ctx context.Context, id string, n int) (review.Revision, error)
	AcceptedRevision(ctx context.Context, id string) (review.Revision, error)
}

// Projects stages read-only worktree-at-rev utility checkouts and reads the
// registry preview command (Spec S13.5/S13.7).
type Projects interface {
	Get(ctx context.Context, projectID string) (project.Entry, error)
	AddUtilityCheckout(ctx context.Context, projectID, committish string) (project.UtilityCheckout, error)
	ReleaseUtilityCheckout(ctx context.Context, projectID, path string) error
}

// Ports is the sinet-portpool allocator (in-process; the file-stub, R9).
type Ports interface {
	Allocate(owner string) (int, error)
	Release(port int) error
}

// Sandbox is the EXISTING internal/sandbox composition surface — a read-only
// consumer view (Caps + the C2 egress policy). *sandbox.Composer satisfies it.
// Nil ⇒ sandboxed lanes report the honest unavailable state (R7).
type Sandbox interface {
	Caps() sandbox.Capabilities
	EgressPolicyFor(c sandbox.Class, singleHost []string) (sandbox.EgressPolicy, error)
}

// Events appends the R22 lifecycle events. *eventlog.Log satisfies it.
type Events interface {
	Append(ctx context.Context, a eventlog.Append) (int64, error)
}

// Settings reads the two ⚙ keys by dotted key. *settings.Registry satisfies it.
type Settings interface {
	Int(key string) (int64, error)
	Duration(key string) (time.Duration, error)
}

// Config assembles a Manager.
type Config struct {
	Reviews  Reviews
	Projects Projects
	Ports    Ports
	Sandbox  Sandbox // nil ⇒ sandboxed lanes are unavailable
	Caddy    *CaddyClient
	Events   Events
	Settings Settings
	// BaseHost is the tailnet base host for preview subdomains (STRUCTURAL
	// config; empty ⇒ units.FQDN default via the composition root). The
	// per-preview host is preview-<id>.<BaseHost>.
	BaseHost string
	// Lookup is the runner/ttyd discovery seam (nil ⇒ DefaultToolLookup).
	Lookup ToolLookup
	// Now is the clock seam (nil ⇒ time.Now).
	Now func() time.Time
	Log *slog.Logger
}

// Manager runs preview lifecycles.
type Manager struct {
	cfg      Config
	log      *slog.Logger
	lookup   ToolLookup
	sessions map[string]*Session
	mu       sync.Mutex
}

// New validates the required dependencies and returns a Manager.
func New(cfg Config) (*Manager, error) {
	if cfg.Reviews == nil || cfg.Projects == nil || cfg.Ports == nil || cfg.Events == nil || cfg.Settings == nil {
		return nil, fmt.Errorf("preview: Reviews, Projects, Ports, Events and Settings are required")
	}
	if cfg.Caddy == nil {
		cfg.Caddy = NewCaddyClient("", "") // routing disabled (dev default)
	}
	if cfg.BaseHost == "" {
		cfg.BaseHost = "preview.invalid" // structural placeholder; the shell sets units.FQDN
	}
	lookup := cfg.Lookup
	if lookup == nil {
		lookup = DefaultToolLookup
	}
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	return &Manager{cfg: cfg, log: log, lookup: lookup, sessions: map[string]*Session{}}, nil
}

// LaunchRequest names a deliverable revision to preview.
type LaunchRequest struct {
	DeliverableID string
	// Revision is the revision to preview; 0 ⇒ the deliverable's current one.
	Revision int
	// Role tags a before-vs-after side; "" ⇒ RoleSingle.
	Role Role
	User string
}

// Launch resolves a deliverable revision's disposition and, for a launchable
// lane, brings the preview up (Spec S13.8). Every type gets a defined state —
// there is no refusal branch (R4). A backed (port-holding) preview is stored
// and event-recorded; a non-backed disposition (self-preview / sidecar /
// no-preview / unavailable / at-capacity) is returned as data without holding a
// port or emitting a start event (nothing started).
func (m *Manager) Launch(ctx context.Context, req LaunchRequest) (*Session, error) {
	if req.DeliverableID == "" || req.User == "" {
		return nil, fmt.Errorf("%w: launch needs a deliverable and a user", ErrBadInput)
	}
	role := req.Role
	if role == "" {
		role = RoleSingle
	}
	d, err := m.cfg.Reviews.Deliverable(ctx, req.DeliverableID)
	if err != nil {
		return nil, err
	}
	n := req.Revision
	if n == 0 {
		n = d.CurrentRevision
	}
	if n < 1 {
		return nil, fmt.Errorf("%w: %s", ErrNoRevision, req.DeliverableID)
	}
	rev, err := m.cfg.Reviews.RevisionAt(ctx, req.DeliverableID, n)
	if err != nil {
		return nil, err
	}

	s := &Session{
		ID: newSessionID(), DeliverableID: req.DeliverableID, Revision: n,
		Role: role, User: req.User, projectID: d.ProjectID,
		CreatedTS: nowOr(m.cfg.Now).UTC().Format(time.RFC3339Nano),
	}

	committish := rev.SnapshotSHA
	if committish == "" {
		committish = rev.PlatformRef
	}
	// Not repo-backed: dispatch by type only, no checkout (a self-preview
	// notebook or an honest no-preview — never a broken iframe, R4).
	if d.ProjectID == "" || committish == "" {
		return m.finishUnbacked(s, d.Type), nil
	}

	checkout, err := m.cfg.Projects.AddUtilityCheckout(ctx, d.ProjectID, committish)
	if err != nil {
		return nil, err
	}
	s.checkout = checkout.Path

	registryCmd := m.registryPreviewCommand(ctx, d.ProjectID)
	lane, runner := Detect(checkout.Path, d.Type, registryCmd)
	s.Lane = lane
	s.Runner = runner

	switch lane {
	case LaneStatic:
		return m.launchStatic(ctx, s)
	case LaneDevServer:
		return m.launchSandboxed(ctx, s, runner)
	case LaneCLI:
		return m.launchCLI(ctx, s, runner)
	case LaneNotebook:
		return m.finishAndRelease(ctx, s, StateSelfPreview, "notebook self-preview (S13.8)", m.artifactURL(s)), nil
	case LaneSidecar:
		return m.finishAndRelease(ctx, s, StateRequiresContainer,
			"project needs a sidecar (Postgres/Redis-class); the rootless-podman container tier is deferred (S13.8)", ""), nil
	default: // LaneNone
		return m.finishAndRelease(ctx, s, StateNoPreview, "no preview available for this type (S13.8)", ""), nil
	}
}

// finishUnbacked handles a deliverable with no repo-backed revision: a notebook
// self-previews, anything else gets the honest no-preview state. No checkout.
func (m *Manager) finishUnbacked(s *Session, dtype string) *Session {
	if lane, _ := Detect("", dtype, ""); lane == LaneNotebook {
		s.Lane = LaneNotebook
		s.State = StateSelfPreview
		s.Reason = "notebook self-preview (S13.8)"
		s.URL = m.artifactURL(s)
		return s
	}
	s.Lane = LaneNone
	s.State = StateNoPreview
	s.Reason = "no repo-backed revision to preview (S13.8)"
	return s
}

// finishAndRelease records a non-backed state and releases the read-only
// checkout (nothing runs, so nothing is held). Idempotent-safe.
func (m *Manager) finishAndRelease(ctx context.Context, s *Session, state State, reason, url string) *Session {
	s.State = state
	s.Reason = reason
	s.URL = url
	m.releaseCheckout(ctx, s)
	return s
}

// launchStatic serves the read-only checkout with the platform static server on
// 127.0.0.1:<pool port> — the ONE lane that runs host-side platform code rather
// than a sandboxed toolchain: it executes NOTHING and serves read-only bytes
// (Spec S13.8 index.html → static server; R11). Fully live at v0. Every
// listener it opens is loopback-only (asserted, P-T13-2).
func (m *Manager) launchStatic(ctx context.Context, s *Session) (*Session, error) {
	if full := m.atCapacity(); full {
		return m.finishAndRelease(ctx, s, StateAtCapacity, m.capacityReason(), ""), nil
	}
	port, err := m.cfg.Ports.Allocate(s.ID)
	if err != nil {
		return m.finishAndRelease(ctx, s, StateAtCapacity, "port pool exhausted (S13.8)", ""), nil
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	if err := assertLoopbackListen(addr); err != nil {
		m.cfg.Ports.Release(port)
		return nil, err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		m.cfg.Ports.Release(port)
		return nil, fmt.Errorf("preview: bind static server: %w", err)
	}
	srv := &http.Server{Handler: http.FileServer(http.Dir(s.checkout)), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	s.stop = func() { _ = srv.Close() }
	s.PoolPort = port
	s.BackendAddr = addr
	s.Ports = []Port{{Number: port, Label: "static"}}
	return m.goLive(ctx, s)
}

// launchSandboxed composes the dev-server runner through the EXISTING sandbox
// surface as a C2 preview (rw workspace + registries-allowlist-as-data), the
// throwaway clone absorbing all writes so the read-only checkout is never
// mutated (Spec S13.8; R6/R7). No new class, no loosened profile, no extra
// binds. Absent composition capability → the honest unavailable state, never an
// unconfined run (R7). Live serving of the in-netns dev server rides the
// deferred host egress + socket-activation substrate (S11.4/§8 reading 5), so
// at v0 the disposition composes and reports the honest deferred state; the
// bring-up substrate flips it live with no code change.
func (m *Manager) launchSandboxed(ctx context.Context, s *Session, runner *Runner) (*Session, error) {
	if m.cfg.Sandbox == nil || !m.cfg.Sandbox.Caps().Available() {
		return m.finishAndRelease(ctx, s, StateUnavailable, "host cannot compose the sandbox boundary (S16.3) — never an unconfined preview (R7)", ""), nil
	}
	tool := runner.Argv[0]
	toolPath, ok := m.lookup(tool)
	if !ok {
		return m.finishAndRelease(ctx, s, StateNoPreview, "required tool not present: "+tool+" (never installed, S16.2/R2)", ""), nil
	}
	if full := m.atCapacity(); full {
		return m.finishAndRelease(ctx, s, StateAtCapacity, m.capacityReason(), ""), nil
	}
	clone, err := throwawayClone(s.checkout)
	if err != nil {
		m.releaseCheckout(ctx, s)
		return nil, err
	}
	s.clone = clone
	host := m.previewHost(s)
	s.HostInjections = hostInjections(runner.Tool, host)
	argv := append([]string{toolPath}, runner.Argv[1:]...)
	if err := m.composeCheck(argv, clone, s.HostInjections); err != nil {
		m.teardownSandboxed(ctx, s)
		return m.markUnavailable(s, "sandbox composition failed: "+err.Error()), nil
	}
	// Composition verified; the in-netns→host live path is the deferred
	// substrate. Tear the clone + checkout down (nothing runs live yet).
	m.teardownSandboxed(ctx, s)
	return m.markUnavailable(s,
		"dev-server preview composes (class C2) but live serving needs the deferred host egress + socket-activation substrate (S11.4/§8 reading 5); a bring-up step flips it live with no code change"), nil
}

// launchCLI serves a captured CLI over ttyd inside the sandbox, over the same
// probe→pool→route path (Spec S13.8; R20). Absent ttyd → the honest CLI
// no-preview state (never installed on this host until the gate ratifies it,
// R21). Like the dev-server lane, live serving rides the deferred substrate.
func (m *Manager) launchCLI(ctx context.Context, s *Session, runner *Runner) (*Session, error) {
	ttydPath, ok := m.lookup("ttyd")
	if !ok {
		return m.finishAndRelease(ctx, s, StateNoPreview, "ttyd not present — CLI preview unavailable until the gate installs it (R20/R21)", ""), nil
	}
	if len(runner.Argv) == 0 {
		return m.finishAndRelease(ctx, s, StateNoPreview, "no CLI command captured to serve over ttyd (R20)", ""), nil
	}
	if m.cfg.Sandbox == nil || !m.cfg.Sandbox.Caps().Available() {
		return m.finishAndRelease(ctx, s, StateUnavailable, "host cannot compose the sandbox boundary for the ttyd lane (S16.3/R7)", ""), nil
	}
	if full := m.atCapacity(); full {
		return m.finishAndRelease(ctx, s, StateAtCapacity, m.capacityReason(), ""), nil
	}
	// The ttyd invocation binds inside the netns; its port rides the same
	// probe→pool→route path. Composition is verified; live serving is deferred.
	inv := ttydInvocation(ttydPath, ttydDefaultPort, runner.Argv)
	if err := m.composeCheck(inv, s.checkout, nil); err != nil {
		m.releaseCheckout(ctx, s)
		return m.markUnavailable(s, "ttyd sandbox composition failed: "+err.Error()), nil
	}
	m.releaseCheckout(ctx, s)
	return m.markUnavailable(s,
		"ttyd CLI preview composes but live serving needs the deferred substrate (S11.4/§8 reading 5); a bring-up step flips it live with no code change"), nil
}

// composeCheck builds the C2 preview plan through the EXISTING sandbox surface
// and discards it — it PROVES the runner composes as a read-only consumer (no
// new class, no loosened profile) without running the process. It never widens
// confinement (R7).
func (m *Manager) composeCheck(argv []string, workspace string, inj []HostInjection) error {
	egress, err := m.cfg.Sandbox.EgressPolicyFor(sandbox.C2, nil)
	if err != nil {
		return err
	}
	sp := sandbox.Spawn{Argv: argv, Env: injectionEnv(inj), Workspace: workspace}
	plan, err := sandbox.BuildPlan(m.cfg.Sandbox.Caps(), sandbox.C2, sp, egress, "")
	if err != nil {
		return err
	}
	plan.Cleanup()
	return nil
}

// goLive stores a backed session, adds its Caddy route, and emits the start
// event (Spec S13.8; R12/R16/R22).
func (m *Manager) goLive(ctx context.Context, s *Session) (*Session, error) {
	host := m.previewHost(s)
	rid := routeID(s.ID)
	if err := m.cfg.Caddy.AddRoute(ctx, rid, host, s.BackendAddr); err != nil {
		m.stopBackend(s)
		m.cfg.Ports.Release(s.PoolPort)
		m.releaseCheckout(ctx, s)
		return nil, err
	}
	if m.cfg.Caddy.Enabled() {
		s.RouteID = rid
		s.Routed = true
		s.URL = "https://" + host + "/"
	} else {
		s.URL = "http://" + s.BackendAddr + "/" // routing disabled (dev): loopback
	}
	s.State = StateLive
	s.Reason = ""
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	m.emit(ctx, EventStarted, s)
	return s, nil
}

// Stop tears a live preview down idempotently (Spec S13.8 R16): stop the
// backend, release the port to the pool, remove the Caddy route, delete the
// clone, release the utility checkout — each tolerant of already-done — and
// emit the stop event. An unknown session id is a no-op (idempotent teardown).
func (m *Manager) Stop(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	s, ok := m.sessions[sessionID]
	if ok {
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()
	if !ok {
		return nil // idempotent
	}
	m.stopBackend(s)
	if s.PoolPort != 0 {
		if err := m.cfg.Ports.Release(s.PoolPort); err != nil {
			return err
		}
	}
	if s.RouteID != "" {
		if err := m.cfg.Caddy.RemoveRoute(ctx, s.RouteID); err != nil {
			return err
		}
	}
	m.deleteClone(s)
	m.releaseCheckout(ctx, s)
	m.emit(ctx, EventStopped, s)
	return nil
}

// List returns the currently live (backed) preview sessions (Spec S15.2 lists
// preview sessions under /api/deliverables; DATA for B6, R19).
func (m *Manager) List() []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	return out
}

// atCapacity reports whether the ⚙ preview.max_concurrent cap is reached (Spec
// S13.8; R15). Only backed (port-holding) sessions count. A settings read error
// fails closed (treated as at-capacity) rather than launching past an unknown
// cap.
func (m *Manager) atCapacity() bool {
	capN, err := m.cfg.Settings.Int(keyMaxConcurrent)
	if err != nil {
		m.log.Warn("preview: read ⚙ preview.max_concurrent failed — failing closed", "err", err)
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	live := 0
	for _, s := range m.sessions {
		if s.backed() {
			live++
		}
	}
	return int64(live) >= capN
}

func (m *Manager) capacityReason() string {
	capN, _ := m.cfg.Settings.Int(keyMaxConcurrent)
	return fmt.Sprintf("at the concurrent-preview cap (⚙ preview.max_concurrent=%d); preview dev servers compete with interactive host use (S13.8/3.11)", capN)
}

// markUnavailable records the honest unavailable/deferred state on a session
// (no port, not stored — nothing runs live).
func (m *Manager) markUnavailable(s *Session, reason string) *Session {
	s.State = StateUnavailable
	s.Reason = reason
	return s
}

// registryPreviewCommand returns the owner-approved preview command for an
// ACTIVE project, or "" (heuristics then apply, R3). A registry read error is
// non-fatal — the heuristics are the fallback.
func (m *Manager) registryPreviewCommand(ctx context.Context, projectID string) string {
	e, err := m.cfg.Projects.Get(ctx, projectID)
	if err != nil || !e.Active() {
		return ""
	}
	return e.Capture.Commands.Preview
}

func (m *Manager) previewHost(s *Session) string {
	return "preview-" + s.ID + "." + m.cfg.BaseHost
}

// artifactURL is the logical S15 view ref for a self-previewing artifact
// (rendering is B6's; this is a data ref, not a bound listener).
func (m *Manager) artifactURL(s *Session) string {
	return fmt.Sprintf("sinet://deliverable/%s/rev-%d", s.DeliverableID, s.Revision)
}

func (m *Manager) stopBackend(s *Session) {
	if s.stop != nil {
		s.stop()
		s.stop = nil
	}
}

func (m *Manager) releaseCheckout(ctx context.Context, s *Session) {
	if s.checkout == "" || s.projectID == "" {
		return
	}
	if err := m.cfg.Projects.ReleaseUtilityCheckout(ctx, s.projectID, s.checkout); err != nil {
		// A dirtied checkout is a loud defect (never auto-deleted, S13.5) — log
		// it; the checkout stays for inspection.
		m.log.Warn("preview: release utility checkout failed", "path", s.checkout, "err", err)
	}
	s.checkout = ""
}

func (m *Manager) deleteClone(s *Session) {
	if s.clone == "" {
		return
	}
	_ = os.RemoveAll(s.clone)
	s.clone = ""
}

// teardownSandboxed disposes the clone then releases the checkout.
func (m *Manager) teardownSandboxed(ctx context.Context, s *Session) {
	m.deleteClone(s)
	m.releaseCheckout(ctx, s)
}

// emit appends a Low-tier preview lifecycle event — platform-scope (run_id
// NULL, no generation — previews are not runs), owner-attributed to the
// launching user (15.6), refs + ids not blobs (P-T07-5). NO effect journal, NO
// approval, NO PIN step-up (Spec S13.8 R22). A failure is logged, never fatal to
// the lifecycle.
func (m *Manager) emit(ctx context.Context, typ string, s *Session) {
	payload, err := json.Marshal(map[string]any{
		"session":     s.ID,
		"deliverable": s.DeliverableID,
		"revision":    s.Revision,
		"role":        string(s.Role),
		"lane":        string(s.Lane),
		"state":       string(s.State),
		"pool_port":   s.PoolPort,
	})
	if err != nil {
		m.log.Warn("preview: marshal event payload", "err", err)
		return
	}
	if _, err := m.cfg.Events.Append(ctx, eventlog.Append{
		UserID: s.User, Type: typ, SchemaVersion: eventSchemaVersion,
		Payload: payload, Time: nowOr(m.cfg.Now),
	}); err != nil {
		m.log.Warn("preview: append lifecycle event", "type", typ, "err", err)
	}
}

// assertLoopbackListen fails closed for a non-loopback bind (Spec S01.1; the
// module never binds non-loopback, P-T13-2, R13).
func assertLoopbackListen(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("preview: bind addr %q is not host:port: %w", addr, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("preview: refusing non-loopback bind %q (S01.1; tailnet front chain is the only exposure)", addr)
	}
	return nil
}

// newSessionID is a DNS-safe short id for the preview subdomain.
func newSessionID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return "p" + hex.EncodeToString(b[:])
}

// throwawayClone copies a read-only checkout into a fresh temp dir that absorbs
// all preview writes — the discardable substrate that keeps the checkout pristine
// (Spec S13.8: "a throwaway clone when dependency-dir-sized"; R6). The .git
// worktree pointer is skipped (the preview runs source, not history). In
// production the overlay upper (bwrap --overlay over the ro checkout) is the
// alternative substrate; both are spec-sanctioned and the preview hands a
// writable path either way (the workspace-overlay wiring is S11.3/S02.10, a
// deferred host component that composes identically).
func throwawayClone(src string) (string, error) {
	dst, err := os.MkdirTemp("", "sinet-preview-clone-")
	if err != nil {
		return "", fmt.Errorf("preview: clone dir: %w", err)
	}
	if err := copyTree(src, dst); err != nil {
		_ = os.RemoveAll(dst)
		return "", err
	}
	return dst, nil
}

// copyTree recursively copies src into dst, skipping the .git worktree pointer.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil // a git-worktree .git is a FILE pointer — skip just it
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !d.Type().IsRegular() {
			return nil // skip symlinks/devices — a preview runs plain source
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
}

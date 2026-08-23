package opencode

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The per-user serve lifecycle (Spec S03.2; S01.2/S01.3 process seam).
//
// `opencode serve` is a per-user persistent HTTP server: ONE instance per
// person, bound to 127.0.0.1 on an ALLOCATED port (never a fixed constant), and
// running under a fully disjoint XDG quartet under a platform-owned per-user
// root at 0700 — measured to contain auth.json, the SQLite DB, config, cache
// and logs with zero cross-leak and zero strays on the serve path [SPIKE G1-S3
// F1]. Each instance is its own PROCESS GROUP, so shutdown reaps the whole tree.
//
// DEV POSTURE. This Manager is the platform spawning per-user engine
// subprocesses itself. It is one implementation of the `Instances` seam, not
// the seam itself: the D6 host ceremony's `sinet-engine@<user>` unit will own
// the process instead, and swapping the provider leaves the Adapter untouched.
// The `sinet-engine@` unit skeleton in internal/units therefore stays a DRAFT
// here on purpose.
//
// CONFINEMENT. S11.1 places the confinement boundary around this whole per-user
// server — a per-user bwrap+netns jail scoped to that person's workspaces and
// model egress — and calls opencode's own `permission` config an inner SOFT
// layer only. Per-RUN confinement classes therefore never apply to this
// substrate, which is why a non-nil per-run Confiner is a named refusal in the
// Adapter rather than a silent unconfined run. Composing the per-user jail
// itself is lane-commissioning scope (LN-2/D6).

// Structural constants. S18 ratifies no key for any of them (the
// `sseBatchSize`/`cancelGrace` precedent, CONVENTIONS §7/§10); making one
// operator-tunable is an S00.9 amendment adding an S18 row.
const (
	defaultBootTimeout    = 90 * time.Second
	defaultStopGrace      = 5 * time.Second
	defaultHealthInterval = 15 * time.Second
	bootPollInterval      = 50 * time.Millisecond
	// passwordBytes sizes the per-instance server password (256 bits from
	// crypto/rand — it is the ONLY thing between a loopback listener and any
	// local process).
	passwordBytes = 32
	// serveLogCap bounds the retained serve output kept for crash detail.
	serveLogCap = 8 << 10
)

// listenLine is the engine's own boot announcement; its URL is authoritative
// (it reports the port actually bound, whatever was requested).
const listenLine = "http://127.0.0.1:"

// ErrBootTimeout reports that a spawned serve never announced a listening URL
// within its budget. Callers skip-with-notice rather than hang (R21).
var ErrBootTimeout = errors.New("opencode: serve did not report a listening URL within the boot budget")

// Instance is one per-user serve endpoint: everything the adapter needs to
// talk to it, and nothing about how it came to exist.
type Instance struct {
	// BaseURL is the loopback origin the engine itself reported.
	BaseURL string
	// Password is the per-instance server password. It lives in process memory
	// and the child's environment only.
	Password string
	// Root is the platform-owned per-user root (0700) the XDG quartet sits
	// under.
	Root string
}

// InstanceSpec is the lowered request for a per-user instance — the hand-off
// the Adapter makes to whatever owns the process.
type InstanceSpec struct {
	UserID string
	Root   string
	// Cwd is the working directory the serve runs in. At 1.18.3 `POST /session`
	// has no per-session directory member on the stable surface, so the serve's
	// own cwd is what a session reports as its directory; per-session
	// workspaces are S11.3/LN-2 scope.
	Cwd string
	// Fingerprint is the invocation-config fingerprint. A change means the
	// compiled config changed, and this engine resolves config at STARTUP.
	Fingerprint string
	// ConfigJSON is the compiled config body (the config channel).
	ConfigJSON []byte
	// Env is the lowered environment (the env channel), already carrying the
	// XDG quartet and the disable switches, and already through CredInject.
	Env []string
}

// Instances is the per-user serve endpoint seam. The dev-posture Manager below
// implements it; the D6 host ceremony replaces the implementation, not the
// contract.
type Instances interface {
	// Acquire returns this user's live instance, starting one if needed.
	Acquire(ctx context.Context, spec InstanceSpec) (Instance, error)
	// Stop ends this user's instance, reaping it by process group.
	Stop(ctx context.Context, userID string) error
}

// Manager is the dev-posture per-user serve spawner: the platform starting
// engine subprocesses itself, one per person, each its own process group.
type Manager struct {
	// Binary is the engine executable (path or PATH name). Empty = DefaultBinary.
	Binary string
	// BootTimeout bounds the wait for a spawned serve to announce its listener.
	BootTimeout time.Duration
	// StopGrace is the TERM→KILL grace of the group reap.
	StopGrace time.Duration
	// HealthInterval is the health-watch poll period.
	HealthInterval time.Duration
	// HTTP is the client the health watch uses. Nil builds one.
	HTTP *http.Client
	// Log receives lifecycle notices. Nil = slog.Default.
	Log *slog.Logger

	mu   sync.Mutex
	live map[string]*instanceProc
}

var _ Instances = (*Manager)(nil)

type instanceProc struct {
	inst     Instance
	cmd      *exec.Cmd
	pgid     int
	identity string
	out      *boundedBuffer
	exited   chan struct{}

	stopWatch context.CancelFunc
	watch     *HealthWatch
}

// identity is what makes a running instance REUSABLE for a spec. It is not the
// S02.4(e) fingerprint: that one is a config fingerprint by spec, and two more
// things are startup-bound on this engine and cannot be changed on a live
// server — the per-user root the XDG quartet points at, and the process cwd,
// which is what a session reports as its `directory` (1.18.3's stable
// `POST /session` has no per-session directory member). Leaving either out of
// the identity would silently run a run in a PREVIOUS run's workspace.
func identityOf(spec InstanceSpec) string {
	return spec.Fingerprint + "\x00" + spec.Root + "\x00" + spec.Cwd
}

func (m *Manager) bootTimeout() time.Duration {
	if m.BootTimeout > 0 {
		return m.BootTimeout
	}
	return defaultBootTimeout
}

func (m *Manager) stopGrace() time.Duration {
	if m.StopGrace > 0 {
		return m.StopGrace
	}
	return defaultStopGrace
}

func (m *Manager) healthInterval() time.Duration {
	if m.HealthInterval > 0 {
		return m.HealthInterval
	}
	return defaultHealthInterval
}

func (m *Manager) binary() string {
	if m.Binary != "" {
		return m.Binary
	}
	return DefaultBinary
}

func (m *Manager) log() *slog.Logger {
	if m.Log != nil {
		return m.Log
	}
	return slog.Default()
}

// Acquire returns this user's live instance, spawning one when none is running.
//
// A change to any STARTUP-BOUND part of the invocation RESTARTS the instance:
// 1.18.3 resolves the config (agents, the permission map, the provider block —
// there is no inline per-request agent definition), the XDG roots and the
// process cwd when the server boots, so none of them can reach a running
// server any other way. Any ask the old instance was holding dies with it,
// which is exactly the restart-ask-loss the S03.4 containment path already
// covers: the platform ask record survives, and the resume diffs it against
// `permission.list`.
//
// The honest consequence: with per-RUN isolated workspaces the cwd differs run
// to run, so "one instance per user" holds per WORKSPACE in practice. That is
// the same per-user-server-vs-per-run-workspace tension S11.3's per-session
// workspaces resolve, and resolving it is LN-2/D6 scope; reusing an instance
// across cwds would silently run a run in a previous run's workspace, which is
// the one outcome not on the table.
func (m *Manager) Acquire(ctx context.Context, spec InstanceSpec) (Instance, error) {
	if spec.UserID == "" {
		return Instance{}, errors.New("opencode: instance spec without a user id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.live == nil {
		m.live = map[string]*instanceProc{}
	}
	want := identityOf(spec)
	if p := m.live[spec.UserID]; p != nil {
		if p.identity == want && p.alive() {
			return p.inst, nil
		}
		m.log().Info("opencode: replacing per-user serve instance",
			"user", spec.UserID, "reason", stopReason(p, want))
		m.reap(p)
		delete(m.live, spec.UserID)
	}
	p, err := m.spawn(ctx, spec)
	if err != nil {
		return Instance{}, err
	}
	m.live[spec.UserID] = p
	return p.inst, nil
}

func stopReason(p *instanceProc, want string) string {
	if !p.alive() {
		return "instance exited"
	}
	if p.identity != want {
		return "startup-bound invocation state changed — compiled config, per-user root or run cwd " +
			"(1.18.3 resolves all three at startup)"
	}
	return "replaced"
}

func (p *instanceProc) alive() bool {
	select {
	case <-p.exited:
		return false
	default:
		return true
	}
}

// Stop reaps one user's instance by PROCESS GROUP.
func (m *Manager) Stop(_ context.Context, userID string) error {
	m.mu.Lock()
	p := m.live[userID]
	delete(m.live, userID)
	m.mu.Unlock()
	if p == nil {
		return nil
	}
	return m.reap(p)
}

// Close reaps every live instance. It is the teardown path every caller — the
// shell, and every test — runs on exit.
func (m *Manager) Close() error {
	m.mu.Lock()
	live := m.live
	m.live = nil
	m.mu.Unlock()
	var firstErr error
	for _, p := range live {
		if err := m.reap(p); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *Manager) spawn(ctx context.Context, spec InstanceSpec) (*instanceProc, error) {
	if err := os.MkdirAll(spec.Root, 0o700); err != nil {
		return nil, fmt.Errorf("opencode: per-user root: %w", err)
	}
	port, err := freeLoopbackPort()
	if err != nil {
		return nil, err
	}
	password, err := mintPassword()
	if err != nil {
		return nil, err
	}
	// The password and the compiled config reach the child through its
	// environment and nowhere else: never a file, never argv, never a log line
	// (D2 hygiene extended, CONVENTIONS §10).
	env := append(append([]string(nil), spec.Env...),
		"OPENCODE_SERVER_PASSWORD="+password)
	if len(spec.ConfigJSON) > 0 {
		env = append(env, "OPENCODE_CONFIG_CONTENT="+string(spec.ConfigJSON))
	}

	cmd := exec.Command(m.binary(), "serve", "--hostname", "127.0.0.1", "--port", fmt.Sprint(port))
	cmd.Dir = spec.Cwd
	cmd.Env = env
	out := &boundedBuffer{cap: serveLogCap}
	cmd.Stdout = out
	cmd.Stderr = out
	// Own process group: the reap signals the whole tree, never the platform.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("opencode: spawn %s serve: %w", m.binary(), err)
	}
	p := &instanceProc{cmd: cmd, identity: identityOf(spec), out: out, exited: make(chan struct{})}
	if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
		p.pgid = pgid
	} else {
		p.pgid = cmd.Process.Pid
	}
	go func() {
		_ = cmd.Wait()
		close(p.exited)
	}()

	url, err := m.awaitListening(ctx, p)
	if err != nil {
		m.reap(p)
		return nil, err
	}
	p.inst = Instance{BaseURL: url, Password: password, Root: spec.Root}

	// The health watch rides the same Basic credentials as every other call —
	// /api/health is not exempt from the server password [SPIKE P2-S1].
	watchCtx, cancel := context.WithCancel(context.Background())
	p.stopWatch = cancel
	p.watch = NewHealthWatch(p.inst, m.HTTP, m.healthInterval(), m.log())
	go p.watch.Run(watchCtx)

	m.log().Info("opencode: per-user serve instance up",
		"user", spec.UserID, "url", url, "root", spec.Root, "pgid", p.pgid)
	return p, nil
}

// awaitListening waits for the engine's own boot announcement, bounded. A
// timeout is a named error, never a hang: the caller reports it or skips.
func (m *Manager) awaitListening(ctx context.Context, p *instanceProc) (string, error) {
	deadline := time.Now().Add(m.bootTimeout())
	for {
		// raw(), not String(): the trimmed form drops the very line terminator
		// that proves the announcement finished arriving.
		if url := parseListenURL(p.out.raw()); url != "" {
			return url, nil
		}
		select {
		case <-p.exited:
			return "", fmt.Errorf("opencode: serve exited during boot: %s", p.out.String())
		case <-ctx.Done():
			return "", fmt.Errorf("%w: %v", ErrBootTimeout, ctx.Err())
		default:
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("%w (%s): %s", ErrBootTimeout, m.bootTimeout(), p.out.String())
		}
		time.Sleep(bootPollInterval)
	}
}

// parseListenURL extracts the engine's announced URL from its output. The
// announcement is only trusted once its LINE IS COMPLETE: a port still being
// written ("…:41" of ":41005") would otherwise be read as a valid URL and every
// subsequent call would go to the wrong port — or to another process.
func parseListenURL(out string) string {
	i := strings.Index(out, listenLine)
	if i < 0 {
		return ""
	}
	rest := out[i:]
	end := strings.IndexAny(rest, "\r\n")
	if end < 0 {
		return "" // the line has not finished arriving
	}
	url := strings.TrimRight(strings.TrimSpace(rest[:end]), "/")
	port := strings.TrimPrefix(url, listenLine)
	if port == "" {
		return ""
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return url
}

// reap ends one instance by process GROUP: TERM, then KILL after the grace.
func (m *Manager) reap(p *instanceProc) error {
	if p.stopWatch != nil {
		p.stopWatch()
	}
	if p.cmd.Process == nil {
		return nil
	}
	m.signalGroup(p, syscall.SIGTERM)
	select {
	case <-p.exited:
		return nil
	case <-time.After(m.stopGrace()):
	}
	m.signalGroup(p, syscall.SIGKILL)
	select {
	case <-p.exited:
		return nil
	case <-time.After(m.stopGrace()):
		return fmt.Errorf("opencode: serve process group %d survived TERM and KILL", p.pgid)
	}
}

func (m *Manager) signalGroup(p *instanceProc, sig syscall.Signal) {
	if p.pgid > 0 {
		if err := syscall.Kill(-p.pgid, sig); err == nil {
			return
		}
	}
	// Group gone or never formed: fall back to the process itself.
	_ = p.cmd.Process.Signal(sig)
}

// freeLoopbackPort allocates an ephemeral loopback port. Ports are ALLOCATED,
// never fixed constants: `--port 0` means the engine's own default 4096 at this
// pin (measured), which would collide across users and with anything else on
// the host.
func freeLoopbackPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("opencode: allocate loopback port: %w", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		return 0, fmt.Errorf("opencode: release allocated port: %w", err)
	}
	return port, nil
}

// mintPassword mints the per-instance server password from crypto/rand. It
// lives in process memory and the child's environment only — never in logs,
// run_events, the DB, fixtures, park records, or test output.
func mintPassword() (string, error) {
	var b [passwordBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("opencode: mint server password: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// HealthWatch polls `/api/health` with the instance's Basic credentials. It is
// a health SIGNAL, not a supervisor: a blip is what the S03.4 containment path
// keys off (diff the platform ask record against `permission.list`), and
// nothing here restarts an engine behind the platform's back.
type HealthWatch struct {
	c        *client
	interval time.Duration
	log      *slog.Logger

	mu      sync.Mutex
	healthy bool
	lastErr error
}

// NewHealthWatch builds the health watcher for one instance.
func NewHealthWatch(inst Instance, hc *http.Client, interval time.Duration, log *slog.Logger) *HealthWatch {
	if interval <= 0 {
		interval = defaultHealthInterval
	}
	if log == nil {
		log = slog.Default()
	}
	return &HealthWatch{c: newClient(inst, hc), interval: interval, log: log}
}

// Probe runs one bounded health poll.
func (w *HealthWatch) Probe(ctx context.Context) error {
	err := w.c.health(ctx)
	w.mu.Lock()
	was := w.healthy
	w.healthy, w.lastErr = err == nil, err
	w.mu.Unlock()
	if err != nil && was {
		// The instance URL is safe to log; the password never is.
		w.log.Warn("opencode: serve health blip", "url", w.c.base, "err", err)
	}
	return err
}

// Run polls until ctx is done.
func (w *HealthWatch) Run(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	_ = w.Probe(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = w.Probe(ctx)
		}
	}
}

// Healthy reports the last poll's verdict.
func (w *HealthWatch) Healthy() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.healthy
}

// boundedBuffer keeps the tail of a serve's output for crash detail.
type boundedBuffer struct {
	mu  sync.Mutex
	buf []byte
	cap int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.cap {
		b.buf = b.buf[len(b.buf)-b.cap:]
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	return strings.TrimSpace(b.raw())
}

// raw returns the retained output UNTRIMMED. Line-completeness checks must read
// this, not String: trimming removes the terminator they test for.
func (b *boundedBuffer) raw() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

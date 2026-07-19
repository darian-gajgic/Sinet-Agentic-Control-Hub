// Package shell is the platform shell of Spec S01: it composes the control
// plane (settings → storage → event log → API, P3/CONVENTIONS.md §6) and
// owns its lifecycle — the S01.6 startup sequence, clean SIGTERM shutdown,
// the maintenance-mode switch, and the scheduling of periodic state
// maintenance (⚙ state.wal_truncate_interval).
//
// The sleep/wake duty of Spec S01.7 (logind delay-mode inhibitor,
// PrepareForSleep + clock-jump wake detection, the wake-side reconcile
// order) is a named seam of this package that lands with the recovery
// ladder (B0-4): the ladder is level-triggered, so the wake path is the
// same Reconcile call the startup sequence already makes. Dev mode runs
// without logind, as it runs without systemd (Spec S01.6).
//
// Ops logs go to stderr; under systemd that is journald, which is the ops
// log only — the platform.db event log is the only audit truth (Spec
// S01.11).
package shell

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/buildinfo"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/sdnotify"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// shutdownTimeout bounds the S01.6 shutdown path (stop admission, flush,
// exit). It is not a ⚙ setting — no such key is ratified; the generated
// unit's TimeoutStopSec exceeds it (S01.6: "TimeoutStopSec MUST exceed the
// flush budget").
const shutdownTimeout = 20 * time.Second

// Options configures one control-plane process. The zero value is the
// production posture: systemd-provided directories and notify socket when
// present, dev-mode fallbacks when not.
type Options struct {
	ConfigDir string // "" = $CONFIGURATION_DIRECTORY, else /etc/sinet
	StateDir  string // "" = bootstrap db_path, else storage.ResolvePath
	HTTPAddr  string // "" = bootstrap http_addr, else DefaultHTTPAddr

	Logger    *slog.Logger       // nil = text handler on stderr
	Notifier  *sdnotify.Notifier // nil = sdnotify.FromEnv()
	Ladder    RecoveryLadder     // nil = B0 stub (ladder lands at B0-4)
	Admission Admission          // nil = scheduler.StubAdmission

	// ReadyFunc, when set, is called once the S01.6 startup sequence has
	// completed, with the bound listener address — the hook tests and
	// tooling use instead of parsing logs.
	ReadyFunc func(addr net.Addr)
}

// Main is the `sinet control` mode entry: parse flags, arm signal handling
// (SIGTERM per Spec S01.6; SIGINT for dev), run. It returns the process
// exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	_ = stdout // ops logs go to stderr (journald under systemd, Spec S01.11)
	fs := flag.NewFlagSet("sinet control", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configDir := fs.String("config-dir", "", "bootstrap config directory (default $CONFIGURATION_DIRECTORY, else /etc/sinet)")
	stateDir := fs.String("state-dir", "", "state directory holding platform.db (default $STATE_DIRECTORY, else $XDG_STATE_HOME/sinet)")
	httpAddr := fs.String("http-addr", "", "loopback listen address (default from bootstrap config, else "+DefaultHTTPAddr+")")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "sinet control: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	if err := Run(ctx, Options{
		ConfigDir: *configDir,
		StateDir:  *stateDir,
		HTTPAddr:  *httpAddr,
		Logger:    logger,
	}); err != nil {
		logger.Error("sinet control: fatal", "err", err)
		return 1
	}
	return 0
}

// Run starts the control plane and blocks until ctx is canceled (SIGTERM)
// or a fatal error, then shuts down cleanly per Spec S01.6. A nil error
// means a clean stop.
func Run(ctx context.Context, opts Options) error {
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	notifier := opts.Notifier
	if notifier == nil {
		notifier = sdnotify.FromEnv()
	}
	ladder := opts.Ladder
	if ladder == nil {
		ladder = stubLadder{logger: logger}
	}
	admission := opts.Admission
	if admission == nil {
		admission = &scheduler.StubAdmission{Logger: logger}
	}

	// ── S01.6 step 1: bootstrap config + settings registry, then the
	// composition order of P3/CONVENTIONS.md §6.
	cfg, err := loadBootstrap(opts.ConfigDir)
	if err != nil {
		return err
	}
	if opts.HTTPAddr != "" {
		cfg.HTTPAddr = opts.HTTPAddr
	}
	dbPath := cfg.DBPath
	if opts.StateDir != "" {
		dbPath = filepath.Join(opts.StateDir, storage.DBFileName)
	}
	if dbPath, err = storage.ResolvePath(dbPath); err != nil {
		return err
	}

	reg := settings.New()
	db, err := storage.Open(ctx, dbPath, reg)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Migrate(ctx); err != nil {
		return err
	}
	log := eventlog.New(db, reg)
	if err := reg.Attach(ctx, db, log); err != nil {
		return err
	}
	// Overrides are loaded; reopen the connection under effective ⚙ values
	// (Spec S01.6 step 1, S02.1).
	if err := db.ReapplySettings(ctx); err != nil {
		return err
	}
	logger.Info("state: platform.db open", "path", dbPath)

	// ── S01.6 step 2: listener-binding lint, fail-closed (P-T13-2).
	if err := assertLoopbackAddr(cfg.HTTPAddr); err != nil {
		return err
	}
	auditUnitListeners(logger)

	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("shell: listen %s: %w", cfg.HTTPAddr, err)
	}
	if err := assertLoopbackListener(ln); err != nil {
		ln.Close()
		return err
	}

	// The API serves from here on; the front chain tolerates a
	// not-yet-ready backend and the health surface reports 503 until the
	// startup sequence completes (Spec S01.6).
	st := newState()
	maint := NewMaintenance(reg, admission, logger)
	srv := api.New(api.Config{
		Log:      log,
		Auth:     api.DevAuthenticator{},
		Settings: reg,
		HealthFn: healthFn(st, maint, log),
		Stopping: st.stopping,
		Logger:   logger,
	})
	httpSrv := &http.Server{
		Handler: srv.Handler(),
		// No WriteTimeout: /events is a long-lived stream by design.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpSrv.Serve(ln) }()
	logger.Info("http: serving loopback-only", "addr", ln.Addr().String())

	// Background duties tied to the process lifetime (not to ctx: the
	// watchdog heartbeat must keep beating through the drain).
	procCtx, procCancel := context.WithCancel(context.Background())
	defer procCancel()

	// ── S01.6 step 3: the recovery ladder (seam; B0-4).
	if err := ladder.Reconcile(ctx); err != nil {
		shutdownHTTP(httpSrv, logger)
		return fmt.Errorf("shell: recovery ladder: %w", err)
	}

	if _, err := appendLifecycle(ctx, log, EventPlatformStarted); err != nil {
		shutdownHTTP(httpSrv, logger)
		return err
	}
	srv.Nudge()

	// ── S01.6 step 4: READY + WatchdogSec heartbeat.
	if err := notifier.Ready(); err != nil {
		logger.Warn("sdnotify: READY", "err", err)
	}
	if iv, ok := notifier.HeartbeatInterval(); ok {
		go heartbeat(procCtx, notifier, iv, logger)
		logger.Info("watchdog: heartbeat armed", "interval", iv)
	}
	st.setReady(true)

	// ── S01.6 step 5: resume admission (scheduler claiming; B1 seam).
	if err := admission.ResumeAdmission(ctx); err != nil {
		logger.Error("admission: resume", "err", err)
	}

	// Periodic WAL truncation — scheduling is the shell's duty (Spec
	// S02.1; ⚙ state.wal_truncate_interval).
	go walTruncateLoop(procCtx, reg, db, logger)

	// TODO(S01.7, lands with B0-4's ladder): sleep/wake seam — logind
	// delay-mode inhibitor, PrepareForSleep(true) O(1) flush,
	// PrepareForSleep(false) + clock-jump wake detection driving
	// ladder.Reconcile and the P-T13-1 network-identity reconcile.

	logger.Info("sinet-control: ready", "version", buildinfo.Version(), "mode", maint.Mode())
	if opts.ReadyFunc != nil {
		opts.ReadyFunc(ln.Addr())
	}

	// ── Run until SIGTERM (ctx) or server death.
	var runErr error
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		runErr = fmt.Errorf("shell: http server died: %w", err)
	}

	// ── Shutdown (Spec S01.6): stop admission, O(1) flush — identical in
	// shape to the pre-sleep path (S01.7) — and exit.
	sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	st.setReady(false)
	st.setStopping()
	if err := notifier.Stopping(); err != nil {
		logger.Warn("sdnotify: STOPPING", "err", err)
	}
	var errs []error
	if runErr != nil {
		errs = append(errs, runErr)
	}
	if err := admission.StopAdmission(sctx); err != nil {
		errs = append(errs, fmt.Errorf("stop admission: %w", err))
	}
	if _, err := appendLifecycle(sctx, log, EventPlatformStopping); err != nil {
		errs = append(errs, err)
	}
	srv.Nudge()
	close(st.stopping) // SSE streams drain their final batch and end
	if err := httpSrv.Shutdown(sctx); err != nil {
		errs = append(errs, fmt.Errorf("http shutdown: %w", err))
	}
	if err := db.CheckpointTruncate(sctx); err != nil {
		// A blocked checkpoint is not data loss (WAL is durable); report,
		// don't fail the clean stop.
		logger.Warn("shutdown flush: wal_checkpoint(TRUNCATE)", "err", err)
	}
	if err := db.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close db: %w", err))
	}
	logger.Info("sinet-control: stopped")
	return errors.Join(errs...)
}

// state is the shell's lifecycle state surfaced on /api/health.
type state struct {
	mu       sync.Mutex
	ready    bool
	stopped  bool
	stopping chan struct{}
}

func newState() *state {
	return &state{stopping: make(chan struct{})}
}

func (s *state) setReady(v bool) {
	s.mu.Lock()
	s.ready = v
	s.mu.Unlock()
}

func (s *state) setStopping() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
}

func (s *state) snapshot() (ready, stopped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ready, s.stopped
}

// healthFn assembles the health snapshot: readiness per the S01.6 sequence,
// the maintenance mode (surfaces stay readable, Spec S01.6), and the event
// head as the snapshot-then-tail cursor bootstrap (Spec S15.3).
func healthFn(st *state, maint *Maintenance, log *eventlog.Log) func() api.Health {
	return func() api.Health {
		ready, stopped := st.snapshot()
		mode := maint.Mode()
		if stopped {
			mode = "stopping"
		}
		h := api.Health{Ready: ready, Mode: mode, Version: buildinfo.Version()}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if head, err := log.Head(ctx); err == nil {
			h.EventHead = head
		}
		return h
	}
}

// heartbeat sends WATCHDOG=1 at the sdnotify-recommended cadence (half the
// WatchdogSec budget, Spec S01.2/S01.6 step 4) until the process winds down.
func heartbeat(ctx context.Context, n *sdnotify.Notifier, interval time.Duration, logger *slog.Logger) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := n.Heartbeat(); err != nil {
				logger.Warn("sdnotify: WATCHDOG", "err", err)
			}
		}
	}
}

// truncater is the WAL-truncation seam of the maintenance loop (satisfied
// by *storage.DB; substituted in tests).
type truncater interface {
	CheckpointTruncate(ctx context.Context) error
}

// walTruncateLoop runs PRAGMA wal_checkpoint(TRUNCATE) every ⚙
// state.wal_truncate_interval so a long-running process never grows the WAL
// without bound (Spec S02.1 read hygiene). The interval is re-read each
// cycle — the key is live-apply.
func walTruncateLoop(ctx context.Context, settings Settings, db truncater, logger *slog.Logger) {
	for {
		interval, err := settings.Duration(keyWALTruncateInterval)
		if err != nil {
			// The key is declared (Spec S18); failure here is a build
			// defect, not a runtime condition to limp through.
			logger.Error("wal truncate: read ⚙ "+keyWALTruncateInterval, "err", err)
			return
		}
		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
			if err := db.CheckpointTruncate(ctx); err != nil {
				// A busy reader can block truncation; the next cycle
				// retries (Spec S02.1).
				logger.Warn("wal truncate", "err", err)
			}
		}
	}
}

// shutdownHTTP tears the server down on a failed startup (fail-closed exits
// must not leak the listener).
func shutdownHTTP(srv *http.Server, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Warn("http shutdown on failed startup", "err", err)
	}
}

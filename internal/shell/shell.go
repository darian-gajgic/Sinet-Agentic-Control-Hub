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
	"encoding/json"
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
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/buildinfo"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/sandbox"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/sdnotify"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/worker"
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

	// The S02 run machinery over the open DB: FSM store, checkpoint store,
	// effect journal, and the recovery ladder they feed (Spec S02.3–S02.7);
	// the S05 Task Context Ledger store beside them (B2-1).
	runs := run.NewStore(db, log)
	checkpoints := gates.NewCheckpoints(db, log)
	taskLedger := ledger.NewStore(db, log)
	// The two-phase effect journal (Spec S02.7) — shared by the recovery ladder
	// (in-doubt reconcile) and the S13.6 accept orchestration (the accept is a
	// class-A effect through it, F1).
	effects, err := gates.NewJournal(gates.JournalConfig{DB: db, Settings: reg})
	if err != nil {
		return err
	}
	ladder := opts.Ladder
	if ladder == nil {
		ladder, err = recovery.New(recovery.Config{
			DB:          db,
			Log:         log,
			Runs:        runs,
			Checkpoints: checkpoints,
			Effects:     effects,
			Settings:    reg,
			Logger:      logger,
			// Units/Harvest default to the B0 probes: real systemd and
			// engine-store observation arrive with run units and adapters
			// (B1, Spec S02.5 step 1, S03).
		})
		if err != nil {
			return err
		}
	}

	// The S10 scheduler + metering over the open DB (Spec S10): the claim
	// loop (started below, after step 5) CAS-claims queued runs under
	// per-(user, lane) slots and dispatches them, owner-attributed —
	// fire-and-forget is banned (S16.6). The price table ships EMPTY at v0
	// (its genai-prices seed is a vendored S16.3 adoption, Spec S10.3), so
	// receipts render UNPRICED — the S10.1 honest posture.
	//
	// B2-4 closes the walking skeleton: the claude-cli adapter registers
	// behind the Dispatcher seam (the registration deferred at B1-2), the
	// stage skeleton routes dispatched runs (intake → execute → verify,
	// Spec S19.5 B2), and the intake API surface goes live.
	var sched *scheduler.Scheduler
	var intakeSurface api.IntakeSurface
	var acceptSurf *acceptSurface
	admission := opts.Admission
	if admission == nil {
		priceTable := metering.NewEffectiveDatedTable("empty-v0")
		exceptions := metering.NoMeteredExceptions()
		meterLedger := metering.NewLedger(db, priceTable, exceptions, reg)
		// The S11 composer probes the host once at startup (probe-at-
		// compose, S16.3) and logs the result. ENGINE spawns run through it
		// only OUTSIDE dev posture: a confined engine holds a credential
		// SENTINEL and cannot authenticate until the S11.5 credential-
		// injection proxy lands (B2-gate host batch with srt activation +
		// the egress substrate), so dev mode keeps the sanctioned B1-1
		// unconfined dev spawn (CONVENTIONS §10/§12) — loudly logged.
		// Verification CHECK runs (no credentials by construction) use the
		// composer in every posture.
		composer := sandbox.NewComposer(reg, logger)
		logger.Info("sandbox: host probe (S11.2/S16.3)", "available", composer.Caps().Available(), "notes", composer.Caps().Notes)
		var engineConfiner adapters.Confiner
		if devPosture() {
			logger.Warn("stage: dev posture — engine spawns UNCONFINED (B1-1 sanctioned dev posture; " +
				"confined+authenticated engines need the S11.5 injection proxy, a B2-gate host component)")
		} else {
			engineConfiner = composer
		}
		stateDir := filepath.Dir(dbPath)

		// The S09 knowledge store over the same DB plus the git-versioned
		// knowledge dirs (Spec S09.2), its S05.4 assembly source, and the
		// B2 seed-object governance re-homing (Spec S09.10) — skipped until
		// an operator account exists (house scope needs its D10 holder).
		memStore, err := memory.NewStore(db, log, reg, filepath.Join(stateDir, "knowledge"))
		if err != nil {
			return err
		}
		// The S13.5/S13.7 project store (git topology + registry) and the
		// S09.2 knowledge-dir committer (commit-on-approval, D9). The knowledge
		// root is initialised as ONE git repo so every scope dir shares it
		// (Spec S13.10 "git-versioned file stores").
		proj, err := project.New(project.Config{DB: db, Log: log, Root: filepath.Join(stateDir, "projects")})
		if err != nil {
			return err
		}
		if err := proj.EnsureRepo(ctx, filepath.Join(stateDir, "knowledge")); err != nil {
			return fmt.Errorf("shell: init knowledge git repo (Spec S13.10): %w", err)
		}
		committer := proj.Committer()
		pseams := &projectSeams{proj: proj, runs: runs, db: db}

		// The knowledge write gate carries the committer at every construction
		// site (Spec S09.2/D9; R28): a file-backed approval commits with the
		// approver as author and fills knowledge_entries.file_commit NULL→hash.
		seedGate := memory.NewGate(memStore)
		seedGate.Committer = committer
		switch n, err := seedGate.EnsureB2SeedGovernance(ctx); {
		case errors.Is(err, memory.ErrNoOperator):
			logger.Info("memory: B2 seed governance deferred — no operator account yet (Spec S09.10, D10)")
		case err != nil:
			return fmt.Errorf("shell: B2 seed governance: %w", err)
		case n > 0:
			logger.Info("memory: B2 seed objects re-homed under S09.10 governance", "created", n)
		}
		// The composer playbook as a governed S09.10 house object (B3-5;
		// seed-content ratification is a B3 gate item — the recorded
		// provenance says so). Same D10 deferral as the B2 seeds.
		playbookGate := memory.NewGate(memStore)
		playbookGate.Committer = committer
		switch created, err := playbookGate.EnsureComposerPlaybook(ctx); {
		case errors.Is(err, memory.ErrNoOperator):
			logger.Info("memory: composer-playbook governance deferred — no operator account yet (Spec S09.10, D10)")
		case err != nil:
			return fmt.Errorf("shell: composer playbook governance: %w", err)
		case created:
			logger.Info("memory: composer playbook seeded under S09.10 governance (ratification = B3 gate item)")
		}

		// The S08 worker store: registry rows + git-versioned template
		// files under the guardrail split, with the overlay slice read
		// through the S09 machinery (empty at v0 by the S09.4 dormancy).
		// Composed here so the store, file root, and battery substrate are
		// live platform state; selection wires it into dispatch at B3-3
		// (Spec S08.8).
		workStore, err := worker.NewStore(worker.Config{
			DB: db, Log: log, Settings: reg,
			Root:     filepath.Join(stateDir, "workers"),
			Overlays: memoryOverlays{s: memStore},
		})
		if err != nil {
			return err
		}
		logger.Info("worker: store open (Spec S08.1)", "root", workStore.Root())

		// The S13.1–S13.4 review store (B4-1), shared by the stage pipeline
		// (minting/drain) and the S13.6 accept orchestration (the accepted state
		// verb, F1).
		reviewStore := &review.Store{
			DB: db, Log: log, Settings: reg,
			Root: filepath.Join(stateDir, "review"),
		}

		sk, err := stage.New(stage.Config{
			DB:          db,
			Log:         log,
			Runs:        runs,
			Checkpoints: checkpoints,
			Ledger:      taskLedger,
			Settings:    reg,
			Knowledge:   &memory.Source{S: memStore},
			// S08.8 selection goes live (B3-3): the router over the worker
			// store, duty-map recommended defaults, flat-rate coverage on
			// the configured lane, consumption pressure as the flat-lane
			// ordering input (D5: never dollars), the S12 tie-break seam
			// absent until B4.
			Workers:       workStore,
			RoutePressure: routePressure{g: metering.NewPressureGauge(db, reg)},
			// The S08.6 composer's policy-input seams: the current approved
			// composer playbook (the governed S09.10 house object) and the
			// lane's pinned engine version keying validation records.
			ComposerPlaybook: composerPlaybook(memStore),
			EnginePin:        claudecli.Pin,
			Adapters: map[string]adapters.Adapter{
				// The Anthropic lane (Spec S03.2): the pinned `claude` CLI
				// resolved via PATH; conformance vs the components.lock pin
				// is the adapter suite's duty (S03.3).
				adapters.SubstrateClaudeCLI: &claudecli.Adapter{Settings: reg, Log: logger},
			},
			Confiner:     engineConfiner,
			ArtifactRoot: filepath.Join(stateDir, "artifacts"),
			RunRoot:      filepath.Join(stateDir, "runs"),
			CopyAsideDir: filepath.Join(stateDir, "copy-aside"),
			// The S13.1–S13.4 review store (B4-1): revision minting at the
			// verification handoff, one comment schema for humans and
			// findings, THE S13.4 drain; object dir under the state dir.
			Review: reviewStore,
			// The S13.5/S13.7 git-topology + registry seams (B4-2), wired over
			// internal/project through the projectSeams adapter (stage/intake/
			// review never import internal/project, brief R35 / CONVENTIONS §23).
			Registry:           registrySeam{proj: proj},
			Fingerprint:        pseams.Fingerprint,
			CitedEntryVersions: memoryCitedEntryVersions(memStore),
			Snapshot:           pseams.Snapshot,
			CreateRevisionRef:  pseams.CreateRevisionRef,
			BaseContent:        pseams,
			WorkspaceCwd:       pseams.WorkspaceCwd,
			// The S13.7 onboarding-as-task seams over internal/project (R5).
			OnboardStart: func(ctx context.Context, projectID, owner, name, source string) (json.RawMessage, error) {
				return proj.OnboardStart(ctx, project.OnboardInput{ProjectID: projectID, Owner: owner, Name: name, Source: source})
			},
			OnboardApprove: func(ctx context.Context, projectID, owner string, edited json.RawMessage) error {
				return proj.OnboardApprove(ctx, projectID, owner, edited)
			},
			// CheckPacks ships empty: the software pack is per-project
			// registry machinery (Spec S13, B4) — software-domain verifies
			// fail LOUDLY rather than run a degraded launch domain.
			CheckRunner: &verify.SandboxCheckRunner{Confiner: composer, WorkDir: filepath.Join(stateDir, "check-work")},
			Logger:      logger,
		})
		if err != nil {
			return err
		}
		sched, err = scheduler.New(scheduler.Config{
			DB:         db,
			Runs:       runs,
			Settings:   reg,
			Dispatcher: sk,
			Pressure:   metering.NewPressureGauge(db, reg),
			Receipts:   metering.NewReceipts(db, meterLedger, exceptions),
			Logger:     logger,
		})
		if err != nil {
			return err
		}
		sk.Bind(sched)
		// Late-bind the pipeline into the project seams: run→project resolution
		// reads the durable intake match (st.Registry.Project) through it (F3).
		pseams.pipe = sk.Pipeline()
		admission = sched
		intakeSurface = sk.Surface()

		// The S13.6 accept orchestration + the S13.9 follow-up verb, composed
		// from the production stores (F1): compiled in and reachable, held by
		// the api layer for the B6 operator-surface endpoints (the S01.9 PIN
		// step-up rides that surface, seam §3 — no endpoints built here).
		who := os.Getenv("USER")
		if who == "" {
			who = "sinet"
		}
		acceptSurf, err = buildAcceptSurface(acceptDeps{
			Proj: proj, Journal: effects, Review: reviewStore, Runs: runs,
			DB: db, Log: log, Settings: reg,
			BrokerSocket:   filepath.Join(stateDir, "broker", who+".sock"),
			Pipeline:       sk.Pipeline(),
			ProjectForTask: pseams.projectForTask,
		})
		if err != nil {
			return err
		}
		logger.Info("accept: S13.6 accept + S13.9 follow-up wired (operator endpoints are B6)")
	}

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
	// The S01.9 authentication stack (B0-5): sessions + PIN over
	// platform.db behind the identity-middleware seam. Dev posture keeps
	// the fixed dev fallback identity so the process works without
	// tailscale, certs, or a login; production requires sessions.
	srv := api.New(api.Config{
		Log:        log,
		Sessions:   auth.New(db, log),
		DevPosture: devPosture(),
		Settings:   reg,
		HealthFn:   healthFn(st, maint, log),
		Stopping:   st.stopping,
		Intake:     intakeSurface,
		Accept:     acceptAccepter(acceptSurf),
		FollowUp:   acceptFollowUp(acceptSurf),
		Logger:     logger,
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
	// After the ladder returns in-doubt class-A effects to `approved`, re-drive
	// any in-doubt ACCEPT effects: reconcile leads to the push being re-driven
	// against the SAME pinned expect-sha (S13.6 R8 / F4). Non-fatal — a
	// transient broker outage retries next boot.
	if acceptSurf != nil {
		if n, err := acceptSurf.Accepter.ReDriveApproved(ctx); err != nil {
			logger.Warn("accept: re-drive of in-doubt accepts failed (retries next boot)", "err", err)
		} else if n > 0 {
			logger.Info("accept: re-drove in-doubt accept effects after recovery (Spec S13.6 R8)", "count", n)
		}
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

	// Periodic recovery sweep — the same level-triggered Reconcile as
	// startup step 3 (Spec S02.5; ⚙ recovery.sweep_interval). The ladder's
	// own clock-jump grace covers the wake case until the S01.7 logind
	// wiring lands.
	go recoverySweepLoop(procCtx, reg, ladder, logger)

	// The scheduler claim loop — a process-lifetime goroutine (Spec S10.7),
	// the same shape as the WAL/recovery loops. It claims only while
	// admission is open (opened at step 5 above). An injected admission
	// (tests) is not a scheduler and brings its own driving; sched is nil then.
	if sched != nil {
		go sched.Run(procCtx)
	}

	// TODO(S01.7): logind sleep/wake seam — delay-mode inhibitor,
	// PrepareForSleep(true) O(1) flush, PrepareForSleep(false) driving an
	// immediate ladder.Reconcile plus the P-T13-1 network-identity
	// reconcile. The ladder (B0-4) is level-triggered and already applies
	// ⚙ recovery.wake_grace on first-pass/clock-jump evidence; the D-Bus
	// wiring is a later packet (needs an adoption-rail decision — no
	// stdlib D-Bus).

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

// recoverySweepLoop runs the level-triggered recovery pass every ⚙
// recovery.sweep_interval (Spec S02.5). The interval is re-read each cycle
// — the key is live-apply. A failing sweep is logged and retried next
// cycle (level-triggered); only startup treats a ladder error as fatal.
func recoverySweepLoop(ctx context.Context, settings Settings, ladder RecoveryLadder, logger *slog.Logger) {
	for {
		interval, err := settings.Duration(keyRecoverySweepInterval)
		if err != nil {
			// The key is declared (Spec S18); failure here is a build
			// defect, not a runtime condition to limp through.
			logger.Error("recovery sweep: read ⚙ "+keyRecoverySweepInterval, "err", err)
			return
		}
		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
			if err := ladder.Reconcile(ctx); err != nil {
				logger.Warn("recovery sweep", "err", err)
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

// The composition-root Dispatcher is the stage skeleton since B2-4
// (internal/stage): it registers the claude-cli adapter behind the
// scheduler's Dispatcher seam (the registration deferred at B1-2), routes
// dispatched runs by role (intake → execute → verify) and chains the
// walking skeleton. The B1-2/B1-3 runDispatcher this replaced lives on in
// the skeleton's Dispatch shape.

// memoryOverlays adapts memory.Store to the worker.OverlaySource seam:
// the S08.4 instance compile reads the requester's overlay L2 slice
// through the settled S09 machinery — no second system. Structurally
// empty at v0 (worker-overlay scope is write-dormant, Spec S09.4).
type memoryOverlays struct{ s *memory.Store }

func (m memoryOverlays) OverlaySlice(ctx context.Context, owner, templateID string) ([]worker.OverlayItem, error) {
	entries, err := m.s.OverlaySlice(ctx, owner, templateID)
	if err != nil {
		return nil, err
	}
	items := make([]worker.OverlayItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, worker.OverlayItem{
			EntryID: e.ID, Title: e.Title, Content: e.Content, Version: e.Version,
		})
	}
	return items, nil
}

// composerPlaybook adapts memory.Store.HouseObject to the S08.6 composer's
// playbook seam: the CURRENT APPROVED version of the governed S09.10 house
// object (stage never imports the memory store, CONVENTIONS §17). An
// absent object surfaces as an error — the playbook is a required policy
// input and composition refuses without it.
func composerPlaybook(s *memory.Store) func(ctx context.Context) (worker.Playbook, error) {
	return func(ctx context.Context) (worker.Playbook, error) {
		e, err := s.HouseObject(ctx, worker.ComposerPlaybookTopicKey)
		if err != nil {
			return worker.Playbook{}, err
		}
		return worker.Playbook{EntryID: e.ID, Version: e.Version, Content: e.Content}, nil
	}
}

// memoryCitedEntryVersions is the S09.6 cited-entry resolver (R32): given
// cited knowledge-entry keys, it returns {key: version} for those currently
// ACTIVE. A superseded, removed, or tombstoned entry no longer resolves active
// → its key is OMITTED, so the intake fingerprint's mapDrift fires vanished-
// key drift (retirement semantics). internal/intake never imports
// internal/memory — this func seam is wired at the composition root.
func memoryCitedEntryVersions(s *memory.Store) func(ctx context.Context, keys []string) (map[string]string, error) {
	return func(ctx context.Context, keys []string) (map[string]string, error) {
		out := make(map[string]string, len(keys))
		for _, k := range keys {
			e, err := s.Get(ctx, k)
			if errors.Is(err, memory.ErrNotFound) {
				continue // genuinely absent → vanished (removed/never existed)
			}
			if err != nil {
				// A real DB error must PROPAGATE — not be mistaken for a
				// vanished entry, which would fabricate drift (F11).
				return nil, fmt.Errorf("shell: resolve cited entry %q: %w", k, err)
			}
			if e.Status != memory.StatusActive || e.Tombstone {
				continue // superseded/removed/tombstoned → vanished
			}
			out[k] = strconv.FormatInt(e.Version, 10)
		}
		return out, nil
	}
}

// routePressure adapts the S10.4 consumption-pressure gauge to the S08.8
// flat-lane ordering seam (D5: among flat-rate lanes selection uses
// consumption pressure, never dollars). Dev-mode budget posture is
// undeclared (S10.4), under which the weighted consumption itself is the
// comparable figure.
type routePressure struct{ g *metering.PressureGauge }

func (p routePressure) Pressure(ctx context.Context, owner, lane string) (float64, error) {
	gauge, err := p.g.Read(ctx, owner, lane, metering.UndeclaredBudget())
	if err != nil {
		return 0, err
	}
	if gauge.Applicable {
		return gauge.Pressure, nil
	}
	return gauge.WeightedConsumption, nil
}

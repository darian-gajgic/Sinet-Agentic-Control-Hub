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

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/adapters/claudecli"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/api"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/auth"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/buildinfo"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/chat"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/conformance"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/gates"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/history"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/ledger"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/memory"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/metering"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/preview"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/project"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/push"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/recovery"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/sandbox"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/scheduler"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/sdnotify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/settings"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/stage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/watchdog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
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
			// The B6-2C fork-refusal seam: a crashed BENCH-REG §2 direct arm is
			// FINALIZED, never forked. §2's "run once, single-shot, no retries" is
			// frozen registered text, and a fork would be a second billed engine
			// run on the same statement whose output the capture could not take
			// anyway (it is keyed on the parent run id). internal/recovery names
			// no run class of its own — the convention lives in internal/stage and
			// is composed here, where both packages are visible.
			NoFork: stage.IsDirectArmRun,
			// Units/Harvest default to the B0 probes: real systemd and
			// engine-store observation arrive with run units and adapters
			// (B1, Spec S02.5 step 1, S03).
		})
		if err != nil {
			return err
		}
	}

	// The S14.5 conformance registry (B5-4): the scheduling HOME for every
	// "proven, not assumed" obligation — the standing suites + drills seeded as
	// platform rows (id, owning section, fixtures, triggers, schedule, last
	// run/result). Seeded unconditionally (platform obligations need no operator
	// account, unlike the S09.10 house seeds); idempotent. Results land as
	// eval.score_recorded via RecordResult and a red row surfaces as an inbox
	// card through the api projection (owner-scoped). Dueness is a passive read
	// surface (OQ3); the browse/render surface is B6's (the buildAcceptSurface
	// reachability precedent, R22 — this compiles internal/conformance into the
	// binary).
	confStore := conformance.NewStore(db, log, reg)
	confRows, err := confStore.EnsureSeeded(ctx)
	if err != nil {
		return fmt.Errorf("shell: seed conformance registry (Spec S14.5): %w", err)
	}
	logger.Info("conformance: S14.5 registry seeded (B5-4)", "rows", confRows)

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
	var cancelSurface api.CancelSurface
	var acceptSurf *acceptSurface
	var previewSurf *preview.Manager
	// reviewStore is the S13.1–S13.4 store (B4-1): the stage pipeline mints and
	// drains through it, the S13.6 accept commits its state verb, and from B6-3B
	// the S15.2 deliverables family reads and comments through it. Declared here
	// so the api composition below can hold it.
	var reviewStore *review.Store
	// The S09 knowledge subsystem, held for the api composition below (B6-3C):
	// memReads is the read store, memWrites the station-3 human write gate with
	// the committer and the contradiction screen already wired. They are handed
	// over as a PAIR because the S15.2 memory family is one family with one
	// readiness — and separately because the split between them IS the S09.1
	// capability wall.
	var memReads *memory.Store
	var memWrites *memory.Gate
	// workRoster is the S08 worker registry, held for the api composition below
	// (B6-8 part B): the S15.10 workforce map reads the roster, each worker's
	// equipment and permissions, and the multi-stage chains through it. Held
	// read-only — the map has no mutation affordance and the transport calls no
	// worker verb.
	var workRoster *worker.Store
	var localSurf *localSurface
	// The S14.4 watchdog suite (B5-3): composed in the production path, driven by
	// shell-owned goroutines (sweep + Tier-0 tail + dead-man probe) after step 5.
	var wd *watchdog.Watchdog
	var wlSurf *watchlistSurface
	// The S14.7 benchmark practice (B5-7): composed in the production path,
	// driven by a shell-owned sampling loop after step 5.
	var benchSurf *benchmarkSurface
	// The S14.9 retention substrate (B5-8A): composed in the production path,
	// driven by shell-owned index and compaction loops after step 5. Its run-end
	// summary hook is installed on the run store at composition time.
	var retSurf *retentionSurface
	// The S14.10 query surface (B5-8B): composed in the production path and
	// HELD on the api Config for the S15 assistant. It has no loop — a query
	// surface's WHEN is "when a question is asked" — and no route yet (B6).
	var histSurf *history.Store
	// The S15.7 conversational assistant's store (B6-7): the per-user session
	// registry, the immutable transcript, the turn lifecycle and the file
	// exchange under <state>/exchange. Composed in the production path and held
	// on the api Config, which ROUTES it (/api/chat). Like the query surface it
	// has no loop — a conversation's WHEN is "when somebody types".
	var chatSurf *chat.Store
	var pushSurf *push.Store
	// meterReader is the S14.3 run-card counter seam (brief §4), wired into the
	// api from the production ledger below; nil under injected admission (the
	// snapshot still projects, counters best-effort).
	var meterReader api.MeterReader
	// The S10.4 operator switches, made durable at migration 0017 (B6-2B). They
	// are composed unconditionally because three subsystems read them — the
	// scheduler's admission pass, the stage skeleton's leg loop, and the two
	// HTTP verbs — and a store that some of them had and others did not would be
	// a switch that half-worked.
	budgetStore := metering.NewBudgets(db)
	pauseStore := metering.NewPause(db)
	// The S10.3 price table, made durable at migration 0019 (B6-3A). Composed
	// unconditionally for the budget store's reason: the pricing path and the
	// S15.9 settings surface must read ONE table, and a process where only one
	// of them had it would be two price truths. It builds from the stored rows
	// here and re-composes on every operator edit (live-apply), so a price
	// change needs no restart.
	//
	// It ships EMPTY and nothing seeds it: with no row for a {model, lane} at a
	// date every usage row prices UNPRICED rather than $0, which is the ratified
	// v0 posture (S10.1; CONVENTIONS §11), and filling the table is an operator
	// act this surface now makes possible.
	priceTable := metering.NewLivePriceTable(metering.NewPriceStore(db, log))
	if err := priceTable.Reload(ctx); err != nil {
		return fmt.Errorf("shell: compose price table: %w", err)
	}
	// resumeSurface is S14.4's "resume — I was wrong" (B6-2B): the ratified
	// S02.3 parked→running edge, taken by a person, over the same stage surface
	// the cancel verbs use.
	var resumeSurface api.ResumeSurface
	// hintSurface carries the S15.5 board drag onto the scheduler's queue row;
	// nil under injected admission, where there is no queue to reorder.
	var hintSurface api.HintSurface
	admission := opts.Admission
	if admission == nil {
		exceptions := metering.NoMeteredExceptions()
		meterLedger := metering.NewLedger(db, priceTable, exceptions, reg)
		meterReader = projMeter{ledger: meterLedger, gauge: metering.NewPressureGauge(db, reg), budgets: budgetStore}
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
		pseams := &projectSeams{proj: proj, runs: runs, db: db, prices: priceTable}
		// The BENCH-REG §2 direct-arm seams (B6-2C). Both halves are late-bound
		// below — the intake pipeline is the skeleton's and the benchmark store is
		// composed after the scheduler the practice enqueues onto — so the holder
		// is created here and handed to stage.Config as bound methods, the
		// pseams.pipe precedent applied to one more seam pair.
		bseams := &directArmSeams{}

		// The knowledge write gate carries the committer at every construction
		// site (Spec S09.2/D9; R28): a file-backed approval commits with the
		// approver as author and fills knowledge_entries.file_commit NULL→hash.
		seedGate := newWriteGate(memStore, committer, nil) // seeding runs before the local surface; the screen wires at the runtime gate below
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
		playbookGate := newWriteGate(memStore, committer, nil)
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
		workRoster = workStore

		// The S14.8 regression-eval surface (B5-5): the per-version floor
		// registry, the revalidation runbook (flag → run → compare → dated
		// stamp), and the sweep cadence. Floors register at the same 8.3
		// knowledge-gate entry the B2 seed objects just rode. The runbook's own
		// triggers are operator/coordinator acts and B5-6's drift emitter, so
		// nothing calls it yet — it is composed here so the wiring is live and
		// wall-clean (the held-dormant SandboxBroker precedent).
		evalSurf, err := buildEvalSurface(ctx, db, confStore, reg, workStore)
		if err != nil {
			return err
		}
		logger.Info("evals: S14.8 regression-eval surface wired (B5-5)",
			"floors_registered", evalSurf.FloorsRegistered)

		// The S13.1–S13.4 review store (B4-1), shared by the stage pipeline
		// (minting/drain) and the S13.6 accept orchestration (the accepted state
		// verb, F1).
		reviewStore = &review.Store{
			DB: db, Log: log, Settings: reg,
			Root: filepath.Join(stateDir, "review"),
		}

		// The S12 local-tier surface (B4-5): the duty-alias client, the class-(b)
		// intake/tie-break seams, the effective DutyMap (with the utility seat)
		// + LocalAvailable, and the eager-unload surface (held for the B6
		// endpoints, api unchanged). Unconfigured is the dev default — every
		// seam nil, the pipeline/routing degrade per the S12.4/S06 rows (R17).
		localSurf, err = buildLocalSurface(localDeps{
			Settings: reg, DB: db, Checkpoints: checkpoints, Events: log, Runs: runs, Log: logger,
			// The 7.3 revalidation-trigger seam (R12): the swap gate flags worker
			// versions referencing the swapped model. Wall-clean — a func seam.
			FlagByModel: workStore.FlagByModel,
		})
		if err != nil {
			return err
		}
		logger.Info("local: S12 duty surface wired (B4-5/B4-7)", "configured", localSurf.Available)
		// The S09.7 contradiction screen (B4-7 R16) is composed on localSurf.Screen
		// and wired into the runtime knowledge-write gate here — the advisory
		// local-model duty refines a same-topic_key conflict's question card; the
		// deterministic detection is never suppressed, and a nil screen
		// (unconfigured stack) leaves it alone. The gate is the ONE write path
		// the S15.2 memory family routes (B6-3C), which is why the api holds
		// exactly this construction rather than building its own.
		localSurf.WriteGate = newWriteGate(memStore, committer, localSurf.Screen)
		memReads, memWrites = memStore, localSurf.WriteGate
		// The ⚙-gated GameMode D-Bus subscription (probe-bound; inert unless the
		// operator session-bus address is configured — R13/R14). The scripts
		// leg is operator-installed config, not a runtime goroutine.
		localSurf.startGameMode(ctx)

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
			// ordering input (D5: never dollars). The S12 tie-break + duty
			// seams and the utility DutyMap seat wire when the local stack is
			// configured (B4-5, localSurf); nil/false is the degrade posture.
			Workers:        workStore,
			DutyMap:        localSurf.DutyMap,
			LocalAvailable: localSurf.Available,
			TieBreak:       localSurf.TieBreak,
			Classifier:     localSurf.Classifier,
			Utility:        localSurf.Utility,
			SpotCheck:      localSurf.SpotCheck,
			RoutePressure:  routePressure{g: metering.NewPressureGauge(db, reg)},
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
			// The S10.4 pause read seam (B6-2B): the execute leg consults it
			// between stage sessions and parks the run when its owner has paused
			// their automation. A park, never a kill (CONVENTIONS §31).
			Paused: pauseStore.Paused,
			// The BENCH-REG §2 direct-arm leg's seams (B6-2C): the frozen task
			// statement from the S06 artifact of record, and the single shot's
			// durable capture. internal/stage never imports internal/benchmark —
			// the §34/§35 seam posture — so they cross as func fields.
			BenchmarkStatement: bseams.Statement,
			BenchmarkCapture:   bseams.Capture,
			Logger:             logger,
		})
		if err != nil {
			return err
		}
		// The S10.9 GPU-admission policy hook (B4-6/R13): the local VRAM admitter
		// gates dispatch onto the local lane (Spec S12.7 mechanics). Nil when the
		// stack is unconfigured (no gate) — assigned via a concrete-nil check so a
		// nil admitter is a true nil interface (not a typed nil). Wired-but-dormant:
		// class-(a) local execution has no v0 consumer, so no run is on the local lane.
		var gpuAdmitter scheduler.GPUAdmitter
		localLane := ""
		if localSurf.VRAM != nil {
			gpuAdmitter = localSurf.VRAM
			localLane = localSurf.LocalLane
		}
		sched, err = scheduler.New(scheduler.Config{
			DB:         db,
			Runs:       runs,
			Settings:   reg,
			Dispatcher: sk,
			Pressure:   metering.NewPressureGauge(db, reg),
			Receipts:   metering.NewReceipts(db, meterLedger, exceptions),
			// The S10.4 denominator and the pause switch (B6-2B): with a budget
			// declared the background admission stop becomes real; with none, the
			// gauge still reports not-applicable exactly as before.
			Budgets:     budgetStore,
			Pause:       pauseStore,
			GPUAdmitter: gpuAdmitter,
			LocalLane:   localLane,
			Logger:      logger,
		})
		if err != nil {
			return err
		}
		sk.Bind(sched)
		// Late-bind the pipeline into the project seams: run→project resolution
		// reads the durable intake match (st.Registry.Project) through it (F3).
		pseams.pipe = sk.Pipeline()
		// Same late bind for the §2 statement seam: both arms answer the ONE
		// confirmed specification of record, resolved through the same
		// briefFromState the benchmark package's own brief seam uses (§3.1).
		bseams.pipe = sk.Pipeline()
		admission = sched
		surface := sk.Surface()
		intakeSurface = surface
		// The S15.6 cancel choreography (4.5, B6-2A) rides the same surface:
		// one composition, two seams. Human-driven only — the HTTP verbs are
		// its sole callers (NO AUTO-KILL, S14.4 / G1 D1.3).
		cancelSurface = surface
		// …and so does the S14.4 resume verb (B6-2B): three seams, one
		// composition. Human-driven only, and a resume takes no cancel path.
		resumeSurface = surface
		hintSurface = sched

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

		// The S13.8 preview module (B4-4): disposable try-out environments +
		// before-vs-after, composed from the production stores (R19) and held for
		// the B6 /api/deliverables preview endpoints (S15.2). Routing is disabled
		// until the Caddy admin endpoint is configured at bring-up (structural
		// config; the live front chain is never dialed here — host hazard).
		previewSurf, err = buildPreviewSurface(previewDeps{
			Reviews: reviewStore, Projects: proj, Sandbox: composer,
			Events: log, Settings: reg, StateDir: stateDir, Log: logger,
		})
		if err != nil {
			return err
		}
		logger.Info("preview: S13.8 preview module wired (operator endpoints are B6)",
			"routing_enabled", os.Getenv("SINET_PREVIEW_CADDY_ADMIN") != "")

		// The S14.4 watchdog suite (B5-3): Tier-0 counters + the Tier-1
		// disambiguator (localSurf.Duty, $0) + Tier-2 pause-and-flag, plus the
		// registered health checks + the dead-man canary. The spend rule reads
		// the priced ledger (dormant at v0 — empty price table); the Tier-1
		// terminal-run $0 row rides the AdvisoryMeter platform-run seam (OQ4). The
		// recurring listener audit reuses the shell's loopback lint (R25); the
		// organ-liveness / wake-reconcile / WAL seams are honestly absent at v0
		// (their host machinery — is-active from the sinet user, logind wake
		// wiring — is a later packet). The shell OWNS WHEN (the goroutines below);
		// the suite owns the LOGIC.
		// The S14.6 watchlist executor (B5-6A): the watch-row config store and
		// its seeds, the config-as-code driver for the adopted
		// changedetection.io page tier, the Sinet-native conditional-GET feed
		// poller, the weekly aggregator jobs incl. the genai-prices
		// refresh-as-proposal, the local-tier second pass, and the drift.finding
		// emitter whose cards derive from the log. The organ is DISCOVERED, never
		// installed (its host install is a B5-gate act) — absent, the page tier
		// degrades and the feed/API tiers keep working. The shell OWNS WHEN (the
		// goroutine below); the package owns the LOGIC.
		wlSurf, err = buildWatchlistSurface(ctx, db, log, reg, localSurf.Duty,
			advisoryMeter(runs, checkpoints), evalSurf.Runbook, confStore, logger)
		if err != nil {
			return err
		}
		logger.Info("watchlist: S14.6 executor wired (B5-6A)",
			"rows_seeded", wlSurf.Rows, "page_tier_configured", wlSurf.PageTier,
			"canary_legs_armed", wlSurf.CanaryArmed)

		// The S14.7 benchmark practice (B5-7): the machinery around the SIGNED
		// registration Spec/benchmark-preregistration-v1.md. It samples accepted
		// deliverables at the ⚙ rate, runs the §2 direct arm as ordinary
		// background work under the requester's own budget, pairs both arms
		// blind, records the §14 pair record before any reveal, tracks epochs,
		// computes the §7 posterior G, and evaluates the §11 gate and §12 alarm.
		// It SETS NOTHING: the gate is a readout, the alarm is a card plus a
		// visible freeze, and none of BENCH-REG's registered numbers is this
		// platform's to change (§17). The shell OWNS WHEN (the loop below).
		benchSurf, err = buildBenchmarkSurface(ctx, db, log, reg, runs, sched,
			meterLedger, evalSurf.Store, sk.Pipeline(), reviewStore, logger)
		if err != nil {
			return err
		}
		// The B6-2C direct-arm capture home, late-bound: the dispatcher's
		// `.direct` leg writes through this seam and the practice's DirectText
		// reads the same column back.
		bseams.store = benchSurf.Store
		logger.Info("benchmark: S14.7 practice wired (B5-7); §2 direct-arm leg + §3 driver live (B6-2C)",
			"domains", benchSurf.Domains, "gate_limb_d_evaluable", benchSurf.FloorsWired)

		// The S14.9 retention substrate (B5-8A): the run summary written at the
		// run's terminal transition, the compaction pass that strips bulky trace
		// payloads past ⚙ retention.compaction_horizon, the keep-forever
		// allowlist that makes the 11.3 exporter boundary structural, and the
		// S14.10 ¶4 derived index (FTS5 + rollups) over what remains. Installing
		// the run-end hook here is what makes "at run end, never later" hold for
		// every terminal transition in the tree.
		if err := assertStackAbsentMarker(); err != nil {
			return err
		}
		retSurf, err = buildRetentionSurface(ctx, db, log, reg, runs, localSurf.Duty, checkpoints, logger)
		if err != nil {
			return err
		}
		logger.Info("retention: S14.9 substrate wired (B5-8A)",
			"keep_forever_seeded", retSurf.KeepForever, "narrator_wired", retSurf.NarratorWired)

		// The Spec S14.10 query surface (B5-8B): Layer 0's named cost views,
		// the Layer-1 canned catalog with grammar-constrained intent, and
		// redact-before-match search over B5-8A's corpus. It has no loop — a
		// query surface's WHEN is "when a question is asked" — and no HTTP
		// route: the transport is B6's (the §31/§35 held-not-routed precedent,
		// as with Accept/FollowUp/Preview).
		var closeHistoryRO func() error
		histSurf, closeHistoryRO, err = buildHistorySurface(ctx, db, log, runs, checkpoints,
			localSurf.Duty, localSurf.CalStore, logger)
		if err != nil {
			return err
		}
		defer func() { _ = closeHistoryRO() }()

		// The S15.7 assistant's durable state (B6-7). Its exchange home sits
		// under the platform state dir beside knowledge/, projects/ and
		// artifacts/ — control-plane territory, never inside a repo or
		// workspace lowerdir, because no sandbox may mount it (S11.3).
		// The titling duty rides ⚙ local.alias's EXISTING `utility` seat through
		// the same advisory-run metering every other run-less local call uses
		// (S12.1 R18); with no local stack the seat is absent and the session is
		// named mechanically, which is the honest degradation.
		chatSurf, err = chat.New(chat.Config{
			DB: db, Log: log, Root: filepath.Join(stateDir, "exchange"),
			Duty:     localSurf.Duty,
			Advisory: chatAdvisory(advisoryMeter(runs, checkpoints)),
		})
		if err != nil {
			return fmt.Errorf("shell: wire the S15.7 assistant store: %w", err)
		}
		logger.Info("chat: S15.7 assistant store wired (B6-7)",
			"exchange_root", filepath.Join(stateDir, "exchange"),
			"title_seat_wired", chatSurf.TitleSeatWired())

		// A chat turn is running only while its driving request is in flight in
		// this process, so any running row surviving a restart was orphaned by a
		// crash. Its produced-files window would otherwise stay open forever and
		// disqualify every later upload by that owner — a silent, unrecoverable
		// degradation. Close them once, here, before the surface serves.
		if orphans, err := chatSurf.ReconcileOrphanedTurns(ctx); err != nil {
			return fmt.Errorf("shell: reconcile orphaned chat turns: %w", err)
		} else if orphans > 0 {
			logger.Info("chat: closed orphaned turn windows left by a previous process",
				"turns", orphans)
		}

		// The S15.11 Web Push channel (B6-9): the per-device subscription
		// registry, the RFC 8291/8292 sender, and the VAPID signing key.
		//
		// THE KEY LIVES IN THE STATE DIRECTORY at 0600, generated at first
		// need (OQ8(b), an accepted interim). Moving custody to the credential
		// broker — so the key never leaves it and signing is a broker
		// operation, the git-signing-key posture — is a NAMED hardening-session
		// item: it needs a new broker operation and a secrets-at-rest
		// placement, both sudo-adjacent host work. The JWT cadence is one
		// signature per push service per several hours, so nothing about this
		// design fights the move.
		pushSurf, err = push.New(push.Config{DB: db, Log: log, StateDir: stateDir})
		if err != nil {
			return fmt.Errorf("shell: wire the S15.11 push channel: %w", err)
		}
		logger.Info("push: S15.11 Web Push channel wired (B6-9)",
			"vapid_public_key", pushSurf.PublicKey())

		wd = watchdog.New(watchdog.Deps{
			DB: db, Log: log, Runs: runs, Settings: reg,
			Duty:  localSurf.Duty,
			Meter: watchdog.AdvisoryMeter(advisoryMeter(runs, checkpoints)),
			Spend: meterLedger,
			// The organ-liveness seam goes live at B5-6A: the watchlist
			// executor probes changedetection.io's GET /api/v1/systeminfo, and a
			// down organ becomes a watchdog.organ_absence:watchlist digest flag.
			Organs: watchlistOrgans(wlSurf),
			Listener: func(context.Context) ([]watchdog.ForeignListener, error) {
				// Recurring in-process listener audit (R25, drain D7): enumerate
				// THIS process's listening sockets via /proc and flag any bound
				// beyond loopback — a real runtime-drift check, not a re-lint of
				// the immutable cfg.HTTPAddr. It never dials the live front chain
				// (host hazard); the external chain is out of this process's scope.
				return auditOwnListeners()
			},
			Logger: logger,
		})
		logger.Info("watchdog: S14.4 suite wired (B5-3)", "tier1_local", localSurf.Duty != nil)
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
		// The B6-3A settings family: the FULL registry behind /api/settings
		// (Settings above stays the narrow Duration seam the SSE keepalive
		// reads through), and the S10.3 stored price table behind its S18.3
		// surface.
		Registry: reg,
		Prices:   priceSurfaceSeam(priceTable),
		HealthFn: healthFn(st, maint, log),
		Stopping: st.stopping,
		Intake:   intakeSurface,
		Cancel:   cancelSurface, // S15.6 / 4.5 cancel verbs (B6-2A)
		Effects:  effects,       // S02.7 journal behind the S15.6 effect approvals
		// The B6-2B decision-plane seams: the S10.4 meters mutations, the S15.5
		// board drag, and the two oversight verbs over landed internals. wd is
		// nil under injected admission, which leaves the suppress route at 503
		// rather than pretending a watchdog is running.
		Budgets:  budgetAdapter{b: budgetStore},
		Pause:    pauseStore,
		Hints:    hintSurface,
		Watchdog: watchdogSuppressSeam(wd),
		Resume:   resumeSurface,
		// The B6-2C verdict backend (benchmark_verbs.go): the blind form, the
		// §3.3 one-act verdict, the §4.2.5 decline, the §12 alarm disposition
		// and the §4.2.1 consent flip. nil under injected admission, which
		// leaves those routes at 503 rather than pretending a practice runs.
		Benchmark: benchmarkVerbSeam(benchSurf),
		// The B6-3B deliverables family: the S13 review store behind the reads,
		// diffs and comments; the S13.6 accept orchestration behind the one
		// outward act; and the S13.8 preview surface behind the try-it verbs.
		Review:   reviewStore,
		Accept:   acceptAccepter(acceptSurf),
		FollowUp: acceptFollowUp(acceptSurf),
		Preview:  previewSurf,
		// The B6-3C memory family: the S09 read store and the station-3 write
		// gate. Both nil under injected admission, which leaves those routes at
		// 503 rather than pretending a knowledge store is open.
		Memory:     memReads,
		MemoryGate: memWrites,
		History:    histSurf, // S14.10 layers, consumed by the S15.7 turn verbs
		Chat:       chatSurf, // S15.7 sessions/transcript/turns/exchange (B6-7)
		Push:       pushSurf, // S15.11 subscriptions + the RFC 8291/8292 sender (B6-9)
		// The B6-8 part-B workforce map: the S08 registry, read only. nil under
		// injected admission, which leaves the route at 503 rather than
		// pretending an empty registry is the roster.
		Workforce: workRoster,
		DB:        db,          // S14.3 snapshot projections (owner-scoped, OQ1)
		Meter:     meterReader, // S14.3 run-card counters (§4)
		Logger:    logger,
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

	// The S14.4 watchdog drivers (B5-3): the shell owns WHEN (Spec S10.7 /
	// CONVENTIONS §7/§8 — the WAL/recovery-loop precedent). The event-driven
	// Tier-0 tail catches loops near-real-time; the sweep runs the time-based
	// signals + registered checks; the dead-man probe runs daily. Intervals are
	// structural constants (no new ⚙; the independent-observer property for the
	// dead-man canary is exactly that the shell — not the watchdog's own loop —
	// drives it). Composed only in the production path (wd nil under injected
	// admission).
	if wd != nil {
		go watchdogTailLoop(procCtx, wd, log, logger)
		go watchdogSweepLoop(procCtx, wd, logger)
		go watchdogDeadManLoop(procCtx, wd, logger)
	}

	// The S15.11 notifier driver (B6-9): the shell owns WHEN, exactly as it does
	// for the watchdog sweep and the dead-man probe.
	//
	// THE INTERVAL IS A POLL BASELINE, NOT A COUNTDOWN. What makes a card due is
	// derived from stored state on every pass — the card's own ObservedTS, the ⚙
	// cadences read live, and the last push for that card read from the event
	// log — so a restart loses nothing and a card born while the host was
	// suspended is picked up on the next tick. That is the §32 rule as the
	// dead-man loop's own comment states it: dueness from the log, never a
	// wall-clock ticker that resets on restart and freezes across suspend.
	//
	// The EDGE TRIGGER is what makes "safety escalations push immediately" true
	// rather than "within the interval": pushNudge fires after an append that
	// could have opened a card, and the loop evaluates then instead of waiting.
	if pushSurf != nil {
		go notifierLoop(procCtx, srv, log, logger)
	}

	// The S14.6 watchlist driver (B5-6A): the shell owns WHEN. It ticks on a
	// structural interval and polls only the rows whose STORED cadence has
	// elapsed, so the watchlist resumes correctly across a restart and a
	// wall-clock jump.
	if wlSurf != nil {
		go watchlistLoop(procCtx, wlSurf, logger)
	}

	// The S14.6 ¶3 API canary layer (B5-6B): the same WHEN posture on its own
	// coarser tick. Its real-request legs ship DISARMED, so unarmed it runs the
	// local-tier logprob canary and the conformance-dueness surfacer only.
	if wlSurf != nil && wlSurf.Canaries != nil {
		go canaryLoop(procCtx, wlSurf, logger)
	}

	// The S14.7 sampling hook (B5-7): the shell owns WHEN. It ticks on a
	// structural interval and consumes the accepted-deliverable events past a
	// DURABLE cursor, so a restart neither re-samples an accept nor silently
	// skips one — sampling is uniform-random among eligible tasks and that
	// uniformity is frozen (BENCH-REG §4.1).
	if benchSurf != nil {
		go benchmarkLoop(procCtx, benchSurf, logger)
	}

	// The S14.9/S14.10 retention drivers (B5-8A): the shell owns WHEN. The index
	// loop advances the derived FTS5/rollup index past its DURABLE cursor and
	// narrates pending summaries; the compaction loop runs the daily-equivalent
	// strip. Both verbs are idempotent and restart-safe from stored state, so a
	// missed tick costs promptness and never correctness. Neither interval is a
	// ⚙ row — the horizon is (⚙ retention.compaction_horizon, per-user, months).
	if retSurf != nil {
		go retentionIndexLoop(procCtx, retSurf, logger)
		go retentionCompactLoop(procCtx, retSurf, logger)
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

// watchdogTailInterval is the poll cadence of the event-driven Tier-0 tail — a
// structural constant (the After-based tail; a runaway loop is caught within a
// couple of seconds, well before it burns a paid window). Not a ⚙ (the
// sseBatchSize precedent, §7).
const watchdogTailInterval = 2 * time.Second

// watchdogTailLoop drives the event-driven Tier-0 tail (Spec S14.4; brief R28):
// it bootstraps the cursor at the current head (a live loop keeps emitting, so
// no history re-scan is needed) and re-evaluates the runs that emit new events.
func watchdogTailLoop(ctx context.Context, wd *watchdog.Watchdog, log *eventlog.Log, logger *slog.Logger) {
	cursor, err := log.Head(ctx)
	if err != nil {
		logger.Warn("watchdog: tail head bootstrap", "err", err)
	}
	t := time.NewTicker(watchdogTailInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			next, err := wd.Tail(ctx, cursor)
			if err != nil {
				logger.Warn("watchdog: Tier-0 tail", "err", err)
				continue
			}
			cursor = next
		}
	}
}

// The S15.11 notifier's three structural constants (no ⚙ key is ratified for
// any of them — the sseBatchSize precedent §7, interim under the standing
// settings-tab directive). Each has a different reason:
//
//   - notifierTailInterval is the EDGE TRIGGER's cadence. It is the watchdog
//     tail's own 2 s, for the same reason: a card can be born at any moment and
//     S15.11 says a safety escalation pushes IMMEDIATELY, which must not quietly
//     mean "at the next sweep". A tick costs one PRAGMA-free `SELECT MAX` on the
//     event log's primary key and evaluates nothing unless the head moved.
//   - notifierSweepInterval is the RE-NAG baseline: a card that nobody has
//     touched still ages past its threshold, and no event announces that. Five
//     minutes is far finer than the finest ratified cadence (1 h safety re-ping),
//     so the sweep can never be what makes a re-nag late.
//   - notifierBurstGap is a burst floor on the edge path only. A busy run emits
//     events continuously and an evaluation reads the whole inbox derivation, so
//     without it a running task would re-derive every 2 s for no new decision. A
//     skipped tick does NOT advance the cursor, so nothing is dropped — the
//     next tick still sees the head it has not evaluated. Nothing about DUENESS
//     depends on this number: it decides how often the platform LOOKS, never
//     what it finds (§32).
const (
	notifierTailInterval  = 2 * time.Second
	notifierSweepInterval = 5 * time.Minute
	notifierBurstGap      = 10 * time.Second
)

// notifierLoop drives the S15.11 notifier (B6-9). The shell owns WHEN, exactly
// as it does for the watchdog sweep and the dead-man probe; the evaluation owns
// WHAT is due, and derives it entirely from stored state.
//
// THE EDGE TRIGGER IS "THE HEAD MOVED", deliberately, rather than a list of
// card-opening event types. Such a list would be a second copy of what the
// inbox derivation already knows about which producers can open a card — the
// twin-maintained-copy hazard in a third place — and it would fail silently, by
// omission, the day a new producer lands. "Something happened, so re-derive" is
// the same discipline the frontend applies to its own feed: the read is the
// truth, the frame is only the trigger. The evaluation is idempotent, so a
// trigger that turns out to have opened nothing sends nothing.
func notifierLoop(ctx context.Context, srv *api.Server, log *eventlog.Log, logger *slog.Logger) {
	cursor, err := log.Head(ctx)
	if err != nil {
		logger.Warn("push: notifier head bootstrap", "err", err)
	}
	tail := time.NewTicker(notifierTailInterval)
	defer tail.Stop()
	sweep := time.NewTicker(notifierSweepInterval)
	defer sweep.Stop()

	var lastEval time.Time
	evaluate := func() {
		if err := srv.EvaluatePush(ctx); err != nil {
			logger.Warn("push: notifier evaluation", "err", err)
		}
		lastEval = time.Now()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tail.C:
			head, err := log.Head(ctx)
			if err != nil {
				logger.Warn("push: notifier head", "err", err)
				continue
			}
			if head == cursor || time.Since(lastEval) < notifierBurstGap {
				// The cursor is NOT advanced when the gap holds it back, so the
				// next tick still sees a head it has not evaluated.
				continue
			}
			cursor = head
			evaluate()
		case <-sweep.C:
			evaluate()
		}
	}
}

// watchdogSweepLoop drives the time-based signals + registered checks on the
// structural SweepInterval (Spec S14.4; brief R28).
func watchdogSweepLoop(ctx context.Context, wd *watchdog.Watchdog, logger *slog.Logger) {
	t := time.NewTicker(watchdog.SweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := wd.Sweep(ctx); err != nil {
				logger.Warn("watchdog: sweep", "err", err)
			}
		}
	}
}

// watchdogDeadManLoop drives the dead-man canary V2 (Spec S14.4; brief R28-DMC;
// drain D6) — the shell is the independent observer. It ticks on the SWEEP
// cadence and probes only when a probe is DUE; dueness is derived from the event
// log (the last canary's birth), NOT a 24h wall-clock ticker, which resets on
// every restart and freezes across suspend — on this laptop that ticker could
// leave the daily probe rarely or never firing, silently. The 24h interval stays
// a structural constant (watchdog.DeadManInterval); only WHEN it is measured
// changed. On a fresh log no probe has ever run, so the FIRST tick is due
// (first-probe-due — no 24h wait); dueness is checked on the tick rather than
// inline at start so the probe never races the bring-up/shutdown lifecycle.
func watchdogDeadManLoop(ctx context.Context, wd *watchdog.Watchdog, logger *slog.Logger) {
	t := time.NewTicker(watchdog.SweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := wd.DeadManProbeIfDue(ctx); err != nil {
				logger.Warn("watchdog: dead-man probe", "err", err)
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

// budgetAdapter adapts the S10.4 budget store to the api.BudgetStore seam
// (B6-2B). The transport speaks its own record type so internal/api never
// imports internal/metering — the same wall projMeter keeps below, for the same
// reason (§11: money and consumption are READ through a seam).
//
// It converts and nothing else: the unit stays the gauge's weighted-consumption
// unit on both sides, and nothing here multiplies, prices or rescales anything.
type budgetAdapter struct{ b *metering.Budgets }

func (a budgetAdapter) DeclareBudget(ctx context.Context, rec api.BudgetRecord) (api.BudgetRecord, bool, error) {
	prior, existed, err := a.b.Declare(ctx, metering.BudgetRow{
		UserID: rec.Owner, Lane: rec.Lane, PeriodTokens: rec.PeriodTokens,
		PeriodStart: rec.PeriodStart, PeriodDays: rec.PeriodDays,
		DeclaredTS: rec.DeclaredTS, DeclaredBy: rec.DeclaredBy,
	})
	if err != nil {
		return api.BudgetRecord{}, false, err
	}
	return budgetRecord(prior), existed, nil
}

func (a budgetAdapter) Budget(ctx context.Context, userID, lane string) (api.BudgetRecord, bool, error) {
	row, ok, err := a.b.Row(ctx, userID, lane)
	if err != nil || !ok {
		return api.BudgetRecord{}, false, err
	}
	return budgetRecord(row), true, nil
}

func budgetRecord(row metering.BudgetRow) api.BudgetRecord {
	return api.BudgetRecord{
		Owner: row.UserID, Lane: row.Lane, PeriodTokens: row.PeriodTokens,
		PeriodStart: row.PeriodStart, PeriodDays: row.PeriodDays,
		DeclaredTS: row.DeclaredTS, DeclaredBy: row.DeclaredBy,
	}
}

// watchdogSuppressSeam hands the S14.4 suppression verb its watchdog, or a true
// nil interface when no watchdog is composed (injected admission). The
// concrete-nil check is the gpuAdmitter precedent: assigning a typed nil pointer
// straight into an interface produces a NON-nil interface, and the route would
// then call through it instead of answering the honest 503.
func watchdogSuppressSeam(wd *watchdog.Watchdog) api.SuppressSurface {
	if wd == nil {
		return nil
	}
	return wd
}

// projMeter adapts the S10 metering ledger + S10.4 pressure gauge to the
// api.MeterReader seam for the S14.3 snapshots (brief §4/§3). RunMeter folds a
// run's checkpoint usage into a monotonic token total and the API-equivalent
// cost so far (subscription lanes price UNPRICED under the empty v0 table — the
// counter is honest, never money-by-generation, S10.1). LaneMeter reads one
// (owner, lane) weighted consumption + utilization/budget-remaining against the
// DECLARED operator budget.
//
// The budget read is what makes the READ surface agree with itself (drain D2).
// Before B6-2B no budget was persisted anywhere, so this passed
// UndeclaredBudget() unconditionally and that was honest. Once the S10.4 verb
// can declare one, a hardcoded undeclared read makes `GET /api/meters`
// CONTRADICT ITSELF — the Layer-0 view block reporting a declared budget while
// the gauge block beside it reports none and serves no pressure — and the S14.3
// fleet snapshot shows the same absence. The budget is therefore read from the
// same store the verb writes, per (person, lane), which is the grain the gauge's
// one-denominator rule needs (D4). A nil store keeps the pre-0017 posture.
type projMeter struct {
	ledger  *metering.Ledger
	gauge   *metering.PressureGauge
	budgets *metering.Budgets
}

func (m projMeter) RunMeter(ctx context.Context, runID string) (api.RunMeter, error) {
	rc, err := m.ledger.RunConsumption(ctx, runID)
	if err != nil {
		return api.RunMeter{}, err
	}
	var tokens int64
	for _, li := range rc.Items {
		tokens += li.PromptTokens + li.BilledOutputTokens + li.CacheReadTokens + li.CacheCreationTokens
	}
	return api.RunMeter{
		Tokens:          tokens,
		APIEquivCostUSD: rc.TotalPricedUSD,
		Unpriced:        rc.TotalUnpricedCalls > 0,
		// The rows the ledger actually folded. Zero of them is the only honest
		// way to say "nothing has been measured for this run yet"; the folded
		// magnitudes cannot, because a local zero-allowance row prices a true $0.
		Calls: rc.TotalCalls,
	}, nil
}

func (m projMeter) LaneMeter(ctx context.Context, userID, lane string) (api.LaneMeter, error) {
	budget := metering.UndeclaredBudget()
	if m.budgets != nil {
		b, err := m.budgets.Budget(ctx, userID, lane)
		if err != nil {
			return api.LaneMeter{}, err
		}
		budget = b
	}
	g, err := m.gauge.Read(ctx, userID, lane, budget)
	if err != nil {
		return api.LaneMeter{}, err
	}
	lm := api.LaneMeter{
		WeightedConsumption: g.WeightedConsumption,
		CacheReadWeight:     g.CacheReadWeight,
		Assumed:             g.Assumed,
		BudgetDeclared:      g.Budget.Declared,
	}
	if g.Applicable { // a budget is declared → utilization + remaining are meaningful
		u := g.Pressure
		lm.Utilization = &u
		rem := float64(g.Budget.PeriodTokens) - g.WeightedConsumption
		lm.BudgetRemaining = &rem
	}
	return lm, nil
}

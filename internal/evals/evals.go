// Package evals is the Spec S14.8 regression-eval home: the per-asset eval
// objects, the registered floors that make a rubric falsifiable, the
// revalidation runbook, the settings-backed sweep cadence, the S03.3 (b)-limb
// engine-bump quality probe, and the pinned-runner seam its adopted promptfoo
// adapter implements.
//
// It is the EXECUTOR side of the S14.5 conformance registry: the registry is a
// scheduling home and never runs anything, so every result this package
// produces lands through conformance.RecordResult as an eval.score_recorded
// event (Spec S14.2 family 14; P-T11-4 runners-never-stores — tools are
// runners, Sinet's DB is the store).
//
// Import posture: storage + conformance + verify + stdlib, and nothing else.
// ⚙ values are read through the narrow Settings interface below rather than by
// importing internal/settings, and the event-log edge belongs to conformance —
// this package appends nothing directly. It NEVER imports internal/worker,
// internal/local, internal/stage, internal/api, or internal/gates (AST-pinned,
// subpackages included): the flag, green-stamp, and swap surfaces those
// packages own ride func seams injected at the shell root (the
// local.FlagByModelFunc precedent, CONVENTIONS §28). The verify edge is the
// eval-object binding's:
// the v0 software content IS the existing rubric-software v2 bundle and the
// golden-software case set — this package invents no case content.
//
// B5-6's behavioral canaries reuse the Runner seam; B5-7 evaluates the
// BENCH-REG §10.1(d) gate limb over the floors registered here. Neither is
// built in this package.
package evals

import (
	"errors"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/conformance"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// Settings is this package's narrow view of the settings registry (CONVENTIONS
// §2: consumers read ⚙ values by dotted key through per-package interfaces).
// *settings.Registry satisfies it.
type Settings interface {
	Int(key string) (int64, error)
}

// keyEvalSweepInterval is the ⚙ regression-sweep cadence (Spec S14.8 ¶3
// "on-trigger + ⚙ eval.sweep_interval full sweep"), declared at S14 in months.
// Read live at evaluation time — never a hardcoded interval.
const keyEvalSweepInterval = "eval.sweep_interval"

// platformUser owns the floor rows: floors are platform data (15.6).
const platformUser = "platform"

// Asset kinds an eval object and its floor can key on (Spec S14.8 ¶2 binds eval
// sets to worker template versions and rubric versions; the (b)-limb probe
// records against the engine).
const (
	AssetRubric = "rubric"
	AssetWorker = "worker"
	AssetEngine = "engine"
)

// Package errors. Callers branch on these.
var (
	// ErrUnknownEvalSet: an eval-hook ref names a set that does not exist
	// in-tree. A binding never invents a set to satisfy a dangling ref.
	ErrUnknownEvalSet = errors.New("evals: eval-hook ref names no in-tree eval set")
	// ErrNoFloor: the asset version has no registered floor, so its
	// falsifiability cannot be evaluated (BENCH-REG §10.1(d)).
	ErrNoFloor = errors.New("evals: no registered floor for this asset version")
	// ErrFloorRegistered: a floor is registered once per asset version.
	ErrFloorRegistered = errors.New("evals: a floor is already registered for this asset version")
	// ErrNoRunner: no eval runner is configured (the promptfoo binary is
	// absent and no runner was injected).
	ErrNoRunner = errors.New("evals: no eval runner configured")
	// ErrNoStampHook: the runbook was asked to stamp without its worker-
	// lifecycle seam wired at the composition root.
	ErrNoStampHook = errors.New("evals: revalidation-stamp seam not wired")
)

// Store is the S14.8 platform surface over platform.db: the floor registry, the
// sweep cadence, and the recording path into the S14.5 registry.
type Store struct {
	db       *storage.DB
	conf     *conformance.Store
	settings Settings
	// Now is the test clock seam; nil ⇒ time.Now.
	Now func() time.Time
}

// NewStore returns the S14.8 store over db, recording results through the S14.5
// registry conf and reading the sweep cadence from settings.
func NewStore(db *storage.DB, conf *conformance.Store, settings Settings) *Store {
	return &Store{db: db, conf: conf, settings: settings}
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

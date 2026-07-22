// Package project is the Spec S13.5/S13.7 git topology and project/repository
// registry: the machinery for everything a task's workspace and its known
// projects rest on — the repo_registry (registered projects with their
// captured conventions, commands, danger zones and protected refs), the
// onboarding-as-task scan/draft, branch-per-pipeline run-branch worktrees with
// platform-owned tree-level snapshot commits (the S02.4d checkpoint artifact
// ref and the S13.1 revision raw material), utility checkouts and workspace
// GC, the minted-revision platform refs, and the knowledge-dir git committer
// (Spec S09.2 commit-on-approval).
//
// Boundaries and import discipline (CONVENTIONS §17/§35): this package imports
// storage + eventlog (+ stdlib) ONLY. It is consumed by the composition root
// (shell) and the stage runtime through narrow interfaces; it never imports
// internal/memory, internal/adapters, internal/stage, internal/intake, or
// internal/review — the S09.1 capability walls stay transitively clean. The
// host git CLI is invoked as an external subprocess, hermetically and
// per-invocation (git.go); nothing here dials a network remote (the dev-mode
// packet constraint) and nothing mutates global/host git config.
package project

import (
	"errors"
	"fmt"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// Entry lifecycle (Spec S13.7): pending while onboarding is in flight, active
// once the owner has approved the draft. Only active entries feed anything.
const (
	StatePending = "pending"
	StateActive  = "active"
)

// DangerZone is one registry danger-zone row (Spec S13.7; S05.1 §2 shape:
// path/action + rule + source hash). SourceHash is the fingerprint of the
// content the zone was derived from — the ledger §2 pin and the drift check
// input (Spec S05.6 row 2).
type DangerZone struct {
	Path       string `json:"path"`
	Action     string `json:"action,omitempty"`
	Rule       string `json:"rule"`
	SourceHash string `json:"source_hash,omitempty"`
}

// Commands is the captured build/test/lint/run/preview command set (Spec
// S13.7). The preview slot's consumer is S13.8's (B4-4); data + read surface
// only here.
type Commands struct {
	Build   string `json:"build,omitempty"`
	Test    string `json:"test,omitempty"`
	Lint    string `json:"lint,omitempty"`
	Run     string `json:"run,omitempty"`
	Preview string `json:"preview,omitempty"`
}

// Capture is one immutable captured-content version (Spec S13.7 "captured
// content is versioned"). Every re-capture is a new version; the registry
// entry points at the current one.
type Capture struct {
	Version     int          `json:"version"`
	Conventions []string     `json:"conventions,omitempty"`
	Commands    Commands     `json:"commands"`
	DangerZones []DangerZone `json:"danger_zones,omitempty"`
	// ScanHash fingerprints the scanned source the capture was derived from —
	// a drift check compares it against a live re-scan (Spec S13.7 re-scan on
	// drift).
	ScanHash   string `json:"scan_hash,omitempty"`
	CapturedBy string `json:"captured_by"`
	CapturedTS string `json:"captured_ts"`
}

// Entry is one repo_registry row plus its current captured content.
type Entry struct {
	ProjectID     string   `json:"project_id"`
	Owner         string   `json:"owner"`
	Name          string   `json:"name"`
	StorePath     string   `json:"store_path"`
	RemoteURL     string   `json:"remote_url,omitempty"`
	DefaultBranch string   `json:"default_branch"`
	Members       []string `json:"members,omitempty"`
	// ProtectedRefs is the broker protected-ref policy input (Spec S13.6, data
	// only here). Default at onboarding = [default branch] ("main only via
	// accepts").
	ProtectedRefs  []string `json:"protected_refs,omitempty"`
	State          string   `json:"state"`
	CaptureVersion int      `json:"capture_version"`
	// Capture is the current captured content (empty until the first capture).
	Capture   Capture `json:"capture,omitempty"`
	CreatedTS string  `json:"created_ts"`
	UpdatedTS string  `json:"updated_ts"`
}

// Active reports whether the entry has been owner-approved (Spec S13.7: only
// active entries feed intake/ledger/workspace/broker).
func (e Entry) Active() bool { return e.State == StateActive }

// Store is the S13.5/S13.7 data + git layer.
type Store struct {
	db  *storage.DB
	log *eventlog.Log
	// root is the projects directory under the state dir — the sibling of
	// knowledge/artifacts/runs (shell composition). It roots the cloned
	// project stores, the run-branch worktrees, the utility checkouts, and the
	// platform excludes file.
	root     string
	excludes string
	now      func() time.Time
}

// Config assembles the store. DB, Log and Root are required.
type Config struct {
	DB   *storage.DB
	Log  *eventlog.Log
	Root string
	// Now is the test clock seam; nil = time.Now.
	Now func() time.Time
}

// New opens the project store, materialising the platform excludes file
// (structural, idempotent).
func New(cfg Config) (*Store, error) {
	if cfg.DB == nil || cfg.Log == nil || cfg.Root == "" {
		return nil, errors.New("project: DB, Log and Root are required")
	}
	s := &Store{db: cfg.DB, log: cfg.Log, root: cfg.Root, now: cfg.Now}
	excl, err := s.writeExcludesFile()
	if err != nil {
		return nil, fmt.Errorf("project: excludes file: %w", err)
	}
	s.excludes = excl
	return s, nil
}

// Root returns the projects root directory.
func (s *Store) Root() string { return s.root }

func (s *Store) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Store) nowRFC3339() string {
	return s.clock().UTC().Format(time.RFC3339Nano)
}

// Errors callers branch on.
var (
	// ErrNotFound is returned for an unknown registry entry.
	ErrNotFound = errors.New("project: registry entry not found")
	// ErrNotActive rejects consuming an entry that has not been owner-approved
	// (Spec S13.7: only active entries feed anything).
	ErrNotActive = errors.New("project: registry entry is not active")
	// ErrNotOwner enforces D10: only the entry's owner approves onboarding of
	// their own object.
	ErrNotOwner = errors.New("project: only the owning user may approve this entry (D10)")
	// ErrNotFastForward rejects a non-fast-forward run-branch ref move (Spec
	// S13.5: the platform NEVER moves a run-branch ref non-fast-forward while
	// a deliverable is in review — the machinery only ever fast-forwards).
	ErrNotFastForward = errors.New("project: run-branch refs only ever fast-forward (Spec S13.5)")
	// ErrDirty rejects auto-deleting a worktree that holds uncommitted work
	// (Spec S13.5/S02.10: dirty checkouts are flagged, never auto-deleted).
	ErrDirty = errors.New("project: worktree holds uncommitted work — flagged, never auto-deleted")
	// ErrBadInput covers malformed store inputs.
	ErrBadInput = errors.New("project: invalid input")
	// ErrAlreadyRegistered rejects a duplicate registration of a project id.
	ErrAlreadyRegistered = errors.New("project: project id already registered")
)

// Event types minted by this package — provisional pending the S14 event
// contract (B5), extending the CONVENTIONS §7/§8/§13–§22 naming note. Platform
// scope (run_id NULL), owner-attributed (15.6), refs + hashes not blobs
// (P-T07-5).
const (
	// EventRegistered records a new pending registry entry.
	EventRegistered = "registry.registered"
	// EventCaptured records one captured-content version (re-capture bumps).
	EventCaptured = "registry.captured"
	// EventActivated records the owner approval that makes an entry active.
	EventActivated = "registry.activated"
	// EventDrift records a detected captured-content drift (source-hash
	// mismatch vs a live re-scan) — surfaced, never a silent overwrite.
	EventDrift = "registry.drift"
)

const eventSchemaVersion = 1

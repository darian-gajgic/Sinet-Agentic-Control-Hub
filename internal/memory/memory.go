// Package memory is the S09 memory & knowledge subsystem — layers, scopes,
// writers, storage, retrieval, provenance, removal, precedence, conflicts,
// and the governance of every knowledge object other sections declare
// (Spec S09).
//
// The package realizes the S09.1 writer rule as CAPABILITY WITHHOLDING at
// the storage layer: reads and the server-scoped knowledge_search live on
// *Store — the only type engine-facing code ever holds — while every L2
// write verb lives on *Gate, whose calls carry an authenticated HUMAN
// actor and are reachable only from human-facing surfaces. Workers never
// hold store handles (D6), the platform tool surface exposes search only,
// and no L1 writer exists at v0 (Spec S09.4 activation table) — so a
// worker write into L2 is structurally impossible, not prompt-forbidden. A
// standing conformance battery (memory package tests) pins the split.
//
// Brief assembly and the trace manifest are Spec S05's; this package
// contributes the Knowledge source behind the S05.4 seam (source.go).
// Retention/forgetting duties run on the S10 scheduler at v1; at v0 the
// write path stamps the lifecycle fields so intervals apply retroactively
// (Spec S09.4 station 5, S09.8).
package memory

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Scope is the S09.2 scope axis, orthogonal to layers.
type Scope string

const (
	ScopeUser          Scope = "user"
	ScopeWorkerOverlay Scope = "worker_overlay"
	ScopeProject       Scope = "project"
	ScopeHouse         Scope = "house"
)

// Layer is the S09.1 layer axis. L0 task scratch never has rows here — it
// lives in the ledger and the run workspace (Spec S09.1).
type Layer string

const (
	LayerL1 Layer = "L1"
	LayerL2 Layer = "L2"
)

// Kind is the S09.2 kind enum (taxonomy/trigger_rules added for the
// S09.10 governed objects).
type Kind string

const (
	KindLesson       Kind = "lesson"
	KindPreference   Kind = "preference"
	KindPlaybook     Kind = "playbook"
	KindRubric       Kind = "rubric"
	KindStyle        Kind = "style"
	KindExemplar     Kind = "exemplar"
	KindConvention   Kind = "convention"
	KindObservation  Kind = "observation"
	KindTaxonomy     Kind = "taxonomy"
	KindTriggerRules Kind = "trigger_rules"
)

// Origin is the S09.5 provenance origin enum.
type Origin string

const (
	OriginProposedFrom Origin = "proposed_from"
	OriginHumanDirect  Origin = "human_direct"
	OriginAdoptedFrom  Origin = "adopted_from"
	OriginImported     Origin = "imported"
)

// Status is the S09.2 status enum.
type Status string

const (
	StatusProposed Status = "proposed"
	StatusActive   Status = "active"
	StatusRetired  Status = "retired"
	StatusRemoved  Status = "removed"
)

// Selectors are the S09.2 rule inputs — registry facts and phrases, never
// embeddings. Matching is conservative (Spec S05.4 pure lookup): a
// selector demanding a fact the platform cannot confirm does not match, so
// an entry never injects outside its declared scope. Domain and task-type
// facts arrive with the project registry (Spec S13/S1.6, B4); until then
// entries carrying them stay searchable but do not inject.
type Selectors struct {
	Domain   string   `json:"domain,omitempty"`
	Project  string   `json:"project,omitempty"`
	TaskType string   `json:"task_type,omitempty"`
	Triggers []string `json:"triggers,omitempty"`
}

// Specificity counts the declared selector dimensions — the S09.8 drop
// order sorts by it (lowest first).
func (s Selectors) Specificity() int {
	n := 0
	if s.Domain != "" {
		n++
	}
	if s.Project != "" {
		n++
	}
	if s.TaskType != "" {
		n++
	}
	if len(s.Triggers) > 0 {
		n++
	}
	return n
}

// Entry is one knowledge row (Spec S09.2).
type Entry struct {
	ID       string
	Owner    string
	Scope    Scope
	ScopeRef string
	Layer    Layer
	Kind     Kind
	Title    string
	// Content is the row-backed body; file-backed entries carry FilePath
	// (+ FileCommit once the S13 committer runs) instead (Spec S09.2:
	// content XOR file_ref).
	Content    string
	FilePath   string
	FileCommit string
	TopicKey   string
	Selectors  Selectors
	Status     Status
	Version    int64
	Supersedes string

	Origin        Origin
	OriginRef     string
	ProposerModel string
	ApprovedBy    string
	ApprovedTS    string

	VerifiedBy           string
	VerifiedTS           string
	ReverifyIntervalDays int64
	ExpiresTS            string
	LastInjectedTS       string

	Tombstone     bool
	TombstoneNote string
	CreatedTS     string
	UpdatedTS     string
}

// Errors callers branch on.
var (
	// ErrNotFound is returned for an unknown entry id.
	ErrNotFound = errors.New("memory: entry not found")
	// ErrInvalidEntry covers validation failures of a write verb.
	ErrInvalidEntry = errors.New("memory: invalid entry")
	// ErrNotHuman is returned when a gate actor has no person row — the
	// human gate accepts humans only (Spec S09.1: every L2 entry enters via
	// an approval or is directly human-written).
	ErrNotHuman = errors.New("memory: gate actor is not a known person")
	// ErrOperatorRequired is returned for house-scope writes by a
	// non-operator (D10; Spec S09.4 station 3).
	ErrOperatorRequired = errors.New("memory: house scope requires the operator")
	// ErrNotOwner is returned when a verb reserved to the entry owner is
	// called by someone else (S4.1 personal-memory rights).
	ErrNotOwner = errors.New("memory: not the entry owner")
	// ErrScopeDormant is returned for worker-overlay writes at v0: the
	// scope activates at v1 with the S08 overlay structure (Spec S09.4).
	ErrScopeDormant = errors.New("memory: worker-overlay scope activates at v1")
	// ErrLayerForbidden is returned when a write names L1 content: no L1
	// writer is active at v0 (Spec S09.4 activation table — schema only).
	ErrLayerForbidden = errors.New("memory: no L1 writer is active at v0")
	// ErrNotActive is returned when a verb needs an active entry.
	ErrNotActive = errors.New("memory: entry is not active")
	// ErrNoOperator is returned by seed governance when no operator account
	// exists yet (house scope needs its D10 holder).
	ErrNoOperator = errors.New("memory: no operator account exists")
)

// marshalSelectors canonicalizes selectors for the row.
func marshalSelectors(s Selectors) (string, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("memory: marshal selectors: %w", err)
	}
	return string(raw), nil
}

func unmarshalSelectors(raw string) (Selectors, error) {
	var s Selectors
	if strings.TrimSpace(raw) == "" {
		return s, nil
	}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return s, fmt.Errorf("memory: decode selectors: %w", err)
	}
	return s, nil
}

// newEntryID mints a knowledge entry id.
func newEntryID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("memory: crypto/rand: %v", err))
	}
	return fmt.Sprintf("k-%x", b)
}

// rfc3339 formats a timestamp the way every other table family does
// (timestamps recorded, never an ordering authority — Spec S02.2).
func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

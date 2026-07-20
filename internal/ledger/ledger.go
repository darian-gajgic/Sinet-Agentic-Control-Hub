// Package ledger is the ledger module seam of Spec S01.1: the Task Context
// Ledger (Spec S05) — the platform-owned, engine-agnostic context artifact
// of a task. Exactly one ledger exists per task; engines never write it
// directly — stage sessions mutate it only through the platform verbs of
// Spec S05.1, and every accepted write is one `ledger_update` run-event
// carrying the full document (Spec S02.2), so the D7 checkpoint payload is
// self-contained in platform.db even if the workspace is destroyed. The
// working copy anywhere else is a projection; the event stream is the
// authoritative copy.
//
// The one artifact serves three ratified duties (Spec S05.2): the D7
// checkpoint payload (block (c), by ledger_version + content hash), the
// S02.6/4.3 freshness input, and the stage-brief source — every stage's
// fresh session is built from the ledger, never from a carried transcript
// (Spec S05.3, fresh-context-per-stage). Brief assembly and the
// deterministic injection machinery live in assemble.go.
package ledger

import (
	"errors"
	"fmt"
	"strings"
)

// Author values of a decisions entry (Spec S05.1 §3).
const (
	AuthorCoordinator = "coordinator" // a stage session, via ledger.decide
	AuthorHuman       = "human"       // control plane recording a human answer/approval
	AuthorPlatform    = "platform"    // control plane recording a platform adjustment (4.3)
)

// Status is the work-item status enum of Spec S05.1 §4 [coordinator-draft
// enum]. Sessions may claim at most StatusDoneUnverified; StatusVerified is
// set only by the control plane from a recorded verification verdict
// (Spec S07) — a session-claimed "verified" is rejected at the write layer.
type Status string

const (
	StatusPending        Status = "pending"
	StatusInProgress     Status = "in_progress"
	StatusDoneUnverified Status = "done_unverified"
	StatusVerified       Status = "verified"
	StatusBlocked        Status = "blocked"
)

var validStatus = map[Status]bool{
	StatusPending: true, StatusInProgress: true, StatusDoneUnverified: true,
	StatusVerified: true, StatusBlocked: true,
}

// Document is the full Task Context Ledger — header + the six ratified
// sections (Spec S05.1, section set ratified G1 Def.12). It is the exact
// content persisted in every ledger_update event payload. Field order is
// the canonical serialization order; the revision content hash is computed
// over the compact JSON marshaling of this struct.
type Document struct {
	// Header (Spec S05.1): ledger_version is monotonic, bumped on every
	// accepted write; every stage brief and every checkpoint records the
	// ledger_version it was built from.
	TaskID        string `json:"task_id"`
	Owner         string `json:"owner"`
	LedgerVersion int64  `json:"ledger_version"`
	SpecVersion   string `json:"spec_version,omitempty"`
	PlanVersion   string `json:"plan_version,omitempty"`
	UpdatedAt     string `json:"updated_at"` // RFC3339Nano UTC; recorded, never an ordering authority (P-T07-4)

	// §1 objective_ac — PINNED: injected verbatim and whole into every
	// stage brief and post-compaction re-injection; never summarized.
	ObjectiveAC ObjectiveAC `json:"objective_ac"`
	// §2 constraints_danger_zones — PINNED, same rule as §1.
	Constraints ConstraintsDangerZones `json:"constraints_danger_zones"`
	// §3 decisions — append-only; a reversal is a new entry citing supersedes.
	Decisions []Decision `json:"decisions,omitempty"`
	// §4 state — work items + current/next_actions/blockers.
	State WorkState `json:"state"`
	// §5 artifacts — restorable references, append + describe, never deleted.
	Artifacts []Artifact `json:"artifacts,omitempty"`
	// §6 learned_this_task — expires with the task; never injected into any
	// other task (Spec S05.1 lifetime; the assembly enforces the firewall).
	LearnedThisTask []Learned `json:"learned_this_task,omitempty"`
}

// ObjectiveAC is §1: the confirmed objective verbatim plus numbered
// acceptance criteria in dual phrasing (plain always, structured sub-line
// where machine-checkable, G1 P10), and the spec version it entered under.
type ObjectiveAC struct {
	Objective          string                `json:"objective"`
	AcceptanceCriteria []AcceptanceCriterion `json:"acceptance_criteria"`
	SpecVersion        string                `json:"spec_version,omitempty"`
}

// AcceptanceCriterion is one numbered criterion of §1.
type AcceptanceCriterion struct {
	N          int    `json:"n"`
	Plain      string `json:"plain"`
	Structured string `json:"structured,omitempty"`
}

// ConstraintsDangerZones is §2: task-level constraints plus the registry
// danger-zone snapshot, recorded with the spec version it was set under
// (its mutation rule follows §1; registry drift while running is the 4.3
// pass's concern, Spec S05.6).
type ConstraintsDangerZones struct {
	Constraints []string     `json:"constraints"`
	DangerZones []DangerZone `json:"danger_zones"`
	SpecVersion string       `json:"spec_version,omitempty"`
}

// DangerZone is one snapshot row of §2: path/action + rule + source hash.
type DangerZone struct {
	Path       string `json:"path"`
	Rule       string `json:"rule"`
	SourceHash string `json:"source_hash,omitempty"`
}

// Decision is one append-only §3 entry [coordinator-draft fields].
type Decision struct {
	Seq        int64  `json:"seq"`
	TS         string `json:"ts"`
	Stage      string `json:"stage"`
	Author     string `json:"author"` // coordinator | human | platform
	Text       string `json:"text"`
	Reason     string `json:"reason"`
	Supersedes int64  `json:"supersedes,omitempty"` // seq of the reversed entry
}

// WorkState is §4.
type WorkState struct {
	Items       []WorkItem `json:"items,omitempty"`
	Current     string     `json:"current,omitempty"`
	NextActions []string   `json:"next_actions,omitempty"`
	Blockers    []string   `json:"blockers,omitempty"`
}

// WorkItem is one §4 work item.
type WorkItem struct {
	ID          string `json:"id"`
	Summary     string `json:"summary"`
	ACRefs      []int  `json:"ac_refs,omitempty"`
	Status      Status `json:"status"`
	EvidenceRef string `json:"evidence_ref,omitempty"`
}

// Artifact is one §5 restorable reference: bulk content is never inlined —
// the ref must suffice to restore the item (Spec S05.1 restorable-reference
// rule, R07 §4.2).
type Artifact struct {
	ID             string `json:"id"`
	Ref            string `json:"ref"`
	Kind           string `json:"kind,omitempty"`
	Description    string `json:"description"`
	SHA256         string `json:"sha256,omitempty"`
	ProducingStage string `json:"producing_stage"`
}

// Learned is one §6 instance note.
type Learned struct {
	Note  string `json:"note"`
	TS    string `json:"ts"`
	Stage string `json:"stage"`
}

// Errors callers branch on (write-rule rejections are behavior under test,
// Spec S05.1).
var (
	// ErrNoTask rejects ledger operations for a run that has no task — the
	// ledger is a per-task artifact (Spec S05.1: exactly one per task).
	ErrNoTask = errors.New("ledger: run has no task")
	// ErrNoLedger is returned when the task has no ledger revision yet.
	ErrNoLedger = errors.New("ledger: task has no ledger")
	// ErrVersionNotFound is returned by point reads at a revision that does
	// not exist in the event stream.
	ErrVersionNotFound = errors.New("ledger: ledger_version not found")
	// ErrPinnedImmutable rejects a rewrite of a pinned section without a new
	// spec version through re-approval (Spec S05.1 §1/§2 mutation rule).
	ErrPinnedImmutable = errors.New("ledger: pinned section changes only by a new spec version (Spec S05.1)")
	// ErrSpecVersionMismatch rejects a §2 write whose spec version is not the
	// header's current one (§2 rides the same approval as §1).
	ErrSpecVersionMismatch = errors.New("ledger: spec version does not match the ledger header")
	// ErrVerifiedFromSession is the ratified write-layer rejection: sessions
	// may claim at most done_unverified (Spec S05.1 §4, G1 Def.12).
	ErrVerifiedFromSession = errors.New("ledger: session may claim at most done_unverified (Spec S05.1)")
	// ErrVerifiedItemProtected rejects session writes to an item the control
	// plane has already verified — verified status is control-plane-owned.
	ErrVerifiedItemProtected = errors.New("ledger: verified item is control-plane-owned (Spec S05.1)")
	// ErrNotVerifiable rejects SetVerified on an item not claimed
	// done_unverified — a verdict verifies claimed-done work (Spec S07 flow).
	ErrNotVerifiable = errors.New("ledger: only a done_unverified item can be verified")
	// ErrUnknownItem is returned for references to a work item that does not
	// exist.
	ErrUnknownItem = errors.New("ledger: unknown work item")
	// ErrUnknownSupersedes rejects a decision citing a nonexistent seq.
	ErrUnknownSupersedes = errors.New("ledger: supersedes cites an unknown decision seq")
	// ErrInvalidWrite covers schema validation failures of a verb call.
	ErrInvalidWrite = errors.New("ledger: invalid write")
	// ErrStageIncomplete is the stage-close gate rejection (Spec S05.1): a
	// stage cannot close until its state update accounts for every assigned
	// work item.
	ErrStageIncomplete = errors.New("ledger: stage close leaves assigned work unaccounted (Spec S05.1 stage-close gate)")
)

// oneLine validates the spec's "one-line" fields (§3 reason, §5
// description).
func oneLine(field, s string) error {
	if s == "" {
		return fmt.Errorf("%w: empty %s", ErrInvalidWrite, field)
	}
	if strings.ContainsAny(s, "\n\r") {
		return fmt.Errorf("%w: %s must be one line", ErrInvalidWrite, field)
	}
	return nil
}

// item returns the §4 work item by id, or nil.
func (d *Document) item(id string) *WorkItem {
	for i := range d.State.Items {
		if d.State.Items[i].ID == id {
			return &d.State.Items[i]
		}
	}
	return nil
}

// decision reports whether a §3 seq exists.
func (d *Document) decision(seq int64) bool {
	for _, dec := range d.Decisions {
		if dec.Seq == seq {
			return true
		}
	}
	return false
}

// CheckStageClose is the stage-close gate of Spec S05.1: every work item
// the stage was assigned must be accounted — done_unverified, verified,
// blocked, or handed forward (listed in state.next_actions for the
// successor stage). The control plane enforces handoff completeness
// structurally, not by convention; underspecified handoffs are the known
// failure mode of fresh-context designs (R07 §4.2).
func CheckStageClose(doc Document, assigned []string) error {
	forwarded := make(map[string]bool, len(doc.State.NextActions))
	for _, n := range doc.State.NextActions {
		forwarded[n] = true
	}
	var unaccounted []string
	for _, id := range assigned {
		it := doc.item(id)
		if it == nil {
			return fmt.Errorf("%w: %q", ErrUnknownItem, id)
		}
		switch it.Status {
		case StatusDoneUnverified, StatusVerified, StatusBlocked:
			continue
		}
		if forwarded[it.ID] {
			continue
		}
		unaccounted = append(unaccounted, it.ID)
	}
	if len(unaccounted) > 0 {
		return fmt.Errorf("%w: %s", ErrStageIncomplete, strings.Join(unaccounted, ", "))
	}
	return nil
}

// Package worker is the S08 worker store and composition machinery: the
// Sinet-owned superset schema (registry rows plus git-versioned template
// files, Spec S08.1), the guardrail split (Spec S08.2), per-invocation
// hash-pinned compilation onto the S03.5 engine lowering (Spec S08.3),
// template → overlay → instance mechanics on the S09 machinery (Spec
// S08.4), the specialization content policy checks (Spec S08.5), the
// four-station validation battery under compose-when-earned (Spec S08.6),
// honest capability marking with structural degraded mode (Spec S08.7),
// and the revalidation/import regime (Spec S08.10). Deterministic
// automations (kind=automation, Spec S08.9) ride the same schema and
// lifecycle; their dialect parser and no-model executor live in the
// automation subpackage.
//
// Boundaries: engine formats are compile targets, never the store (Spec
// S08.1); routing/selection is Spec S08.8 (B3-3) — this package builds the
// store/compile/battery substrate selection consumes, with the selection
// seams left named. Knowledge storage and injection are Spec S09's; what a
// worker overlay IS structurally is owned here, and the overlay slice is
// read through the S09 machinery (OverlaySource) — structurally empty at
// v0, where the worker-overlay scope is write-dormant (Spec S09.4
// activation table).
package worker

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

// Kind is the S08.1 worker kind.
type Kind string

const (
	// KindAgentic is a model-driven worker compiled onto an engine lane.
	KindAgentic Kind = "agentic"
	// KindAutomation is a deterministic C0 workflow in Sinet's own dialect,
	// executed by the platform with no model in the loop (Spec S08.9).
	KindAutomation Kind = "automation"
)

// TemplateScope is the S08.1 sharing scope (D10, 12.5): personal by
// default; household by operator-approved promotion.
type TemplateScope string

const (
	ScopePersonal  TemplateScope = "personal"
	ScopeHousehold TemplateScope = "household"
)

// TemplateStatus is the S08.1 lifecycle status.
type TemplateStatus string

const (
	StatusDraft     TemplateStatus = "draft"
	StatusValidated TemplateStatus = "validated"
	StatusActive    TemplateStatus = "active"
	StatusFlagged   TemplateStatus = "flagged"
	StatusRetired   TemplateStatus = "retired"
)

// Origin is the S08.1 provenance origin of a template version.
type Origin string

const (
	OriginComposed     Origin = "composed"
	OriginHumanWritten Origin = "human-written"
	OriginImported     Origin = "imported"
	OriginAdoptedFrom  Origin = "adopted-from"
)

// Maturity is the S08.7 domain verification maturity.
type Maturity string

const (
	MaturityFull     Maturity = "full"
	MaturityDegraded Maturity = "degraded"
)

// GapDisposition is the lifecycle of a gap record (Spec S08.6, S08.8).
type GapDisposition string

const (
	GapOpen      GapDisposition = "open"
	GapProposed  GapDisposition = "proposed"
	GapComposed  GapDisposition = "composed"
	GapDismissed GapDisposition = "dismissed"
)

// EgressClass is the guardrail egress posture (Spec S08.2, S11.6 v0 rows).
type EgressClass string

const (
	EgressNone       EgressClass = "none"
	EgressRegistries EgressClass = "registries"
	EgressSingleHost EgressClass = "single-host"
)

// egressReach ranks egress classes for the ceiling comparison (station 2):
// greater reach is a looser posture.
func egressReach(e EgressClass) int {
	switch e {
	case EgressNone:
		return 0
	case EgressSingleHost:
		return 1
	case EgressRegistries:
		return 2
	}
	return 0
}

// Template is one worker_templates row (Spec S08.1).
type Template struct {
	ID            string
	Owner         string
	Name          string
	Scope         TemplateScope
	Kind          Kind
	Domain        string
	Status        TemplateStatus
	ActiveVersion string
	CreatedTS     string
	UpdatedTS     string
}

// Version is one immutable worker_template_versions row (Spec S08.1,
// S08.4): file ref, provenance block, approval and graduation stamps.
type Version struct {
	ID          string
	TemplateID  string
	Version     int64
	Supersedes  string
	FilePath    string
	FileSHA256  string
	FileCommit  string
	Requested   RequestedGrants
	AuthorKind  string // "human" | "composer"
	Composer    string // model+version when composer-drafted
	PlaybookVer string // composer playbook version when composer-drafted
	EvidenceRef string
	Origin      Origin
	OriginRef   string
	CreatedBy   string
	CreatedTS   string
	ApprovedBy  string
	ApprovedTS  string
	GraduatedTS string
}

// RequestedGrants is the control-plane draft of enforcement requests that
// accompanies a version (Spec S08.6 composer output "requested grants").
// It is guardrail-CLASS content and therefore lives in rows, never in the
// file (Spec S08.2); approval copies requested → granted only after the
// station-2 permission audit.
type RequestedGrants struct {
	Tools              []string    `json:"tools,omitempty"`
	Class              string      `json:"class"`
	Egress             EgressClass `json:"egress"`
	EgressHosts        []string    `json:"egress_hosts,omitempty"`
	GatedTools         []string    `json:"gated_tools,omitempty"`
	PermissionMode     string      `json:"permission_mode,omitempty"`
	BudgetUSD          float64     `json:"budget_usd,omitempty"`
	BudgetSteps        int64       `json:"budget_steps,omitempty"`
	ScheduleAttachable bool        `json:"schedule_attachable,omitempty"`
}

// Guardrails is one worker_guardrails row — the enforcement state of Spec
// S08.2, exclusively: written only by the control plane on a human
// approval, present in no file, recompiled into the engine invocation on
// every run.
type Guardrails struct {
	VersionID          string
	GrantedTools       []string
	GatedTools         []string
	PermissionMode     string
	Class              string
	Egress             EgressClass
	EgressHosts        []string
	BudgetUSD          float64
	BudgetSteps        int64
	GatePolicy         string
	FirstNRemaining    int64
	ScheduleAttachable bool
	CreatedTS          string
	UpdatedTS          string
}

// Domain is one domains row (Spec S08.1, S08.7).
type Domain struct {
	Name      string
	Maturity  Maturity
	RubricRef string
	UpdatedTS string
}

// GapRecord is one gap_records row (Spec S08.1, S08.6): the persistent
// record of a no-fit outcome, accumulated so recurrence can earn
// composition.
type GapRecord struct {
	Signature   string
	Family      string
	TaskRefs    []string
	Occurrences int64
	LastSeenTS  string
	Disposition GapDisposition
}

// ValidationRecord is one validation_records row (Spec S08.1, S08.6): a
// battery pass keyed (version × model × engine pin); for kind=automation
// the engine-pin slot holds the dialect version (Spec S08.9).
type ValidationRecord struct {
	ID         int64
	VersionID  string
	Model      string
	EnginePin  string
	Lint       StationResult
	Audit      AuditResult
	DryRunRef  string
	Green      bool
	Approver   string
	ApprovedTS string
	CreatedTS  string
}

// Errors callers branch on.
var (
	// ErrNotFound is returned for unknown template/version/domain ids.
	ErrNotFound = errors.New("worker: not found")
	// ErrInvalid covers validation failures of a store verb.
	ErrInvalid = errors.New("worker: invalid")
	// ErrNotHuman: the acting id has no person row — every store verb that
	// creates, approves, or disposes definitions is human-gated (Spec
	// S08.6 station 4, S08.10 imports; D10).
	ErrNotHuman = errors.New("worker: actor is not a known person")
	// ErrNotOwner: a verb reserved to the template owner was called by
	// someone else (D10 own-object approval).
	ErrNotOwner = errors.New("worker: not the template owner")
	// ErrOperatorRequired: household promotion and domain-maturity flips
	// require the operator (D10; Spec S08.4, S08.7).
	ErrOperatorRequired = errors.New("worker: operator required")
	// ErrGuardrailField: a guardrail-class field appeared in a definition
	// file — a structural reject, never a warning (Spec S08.2, S08.6
	// station 1).
	ErrGuardrailField = errors.New("worker: guardrail-class field in a definition file (S08.2 structural reject)")
	// ErrLintReject: station 1 found hard errors (schema, references,
	// instruction-pattern screen).
	ErrLintReject = errors.New("worker: station-1 lint reject")
	// ErrNotValidated: approval requires a green validation record for the
	// version (Spec S08.6: validation = structural checks passed plus
	// human sign-off).
	ErrNotValidated = errors.New("worker: version has no green validation record")
	// ErrAboveCeiling: approval attempted with unacknowledged above-ceiling
	// grant lines (Spec S08.6 station 2: the ceiling table is 4.4's
	// instrument; flagged lines need an explicit approver acknowledgement).
	ErrAboveCeiling = errors.New("worker: above-ceiling grants not acknowledged")
	// ErrTamperedFile: the template file on disk no longer matches the
	// version row's pinned hash (Spec S08.3: the hash check catches the
	// tamper).
	ErrTamperedFile = errors.New("worker: template file does not match its pinned hash")
	// ErrNotActive is returned when a verb needs an active template.
	ErrNotActive = errors.New("worker: template is not active")
	// ErrKindMismatch: an agentic-only or automation-only verb was called
	// across kinds (e.g. compiling an automation onto an engine — no model
	// in the loop, Spec S08.9).
	ErrKindMismatch = errors.New("worker: wrong worker kind for this operation")
	// ErrDegraded: the operation is refused while the domain is degraded
	// (Spec S08.7 structural enforcement, e.g. schedule attachability).
	ErrDegraded = errors.New("worker: domain verification maturity is degraded (S08.7)")
)

func newID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("worker: crypto/rand: %v", err))
	}
	return fmt.Sprintf("%s-%x", prefix, b)
}

// rfc3339 formats timestamps the way every other table family does
// (recorded, never an ordering authority — Spec S02.2).
func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

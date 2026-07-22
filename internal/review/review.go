// Package review is the S13.1–S13.4 deliverable/review data layer: the
// Gerrit/Phorge deliverable-and-revisions shape (long-lived deliverable,
// immutable revisions 1..N, lineage never compressed), the one comment
// schema shared by human review comments and verification findings, the
// FC-v1 §2 anchor model with server-side re-validation and the 5-step
// re-anchoring ladder, per-type comparison behavior (Spec S13.2), and THE
// findings→retry drain point (Spec S13.4).
//
// Boundaries: revision RENDERING (diff widget, image trio, synthetic strip)
// is Spec S15's; git topology and snapshot-commit pins are Spec S13.5's
// (B4-2 — the snapshot_sha column and platform ref namespace are the
// seams); the V3 accept flow is Spec S13.6's. Imports are storage +
// eventlog only; the verification pipeline consumes this package through
// the verify.ReviewSink seam, never the reverse.
package review

import (
	"errors"
	"fmt"
)

// Deliverable states (Spec S13.1). Accept mechanics are Spec S13.6's
// (B4-3); the states exist from birth so the schema never migrates for
// them.
const (
	StateInReview   = "in-review"
	StateAccepted   = "accepted"
	StateSuperseded = "superseded"
)

// Comment kinds: one schema for human comments and verification findings
// (Spec S13.1/S13.3).
const (
	KindHuman   = "human"
	KindFinding = "finding"
)

// Severities (Spec S13.4: severity distinguishes blockers from notes —
// only genuine blockers trigger another round; polish travels as notes).
const (
	SeverityBlocker = "blocker"
	SeverityNote    = "note"
)

// AnchorStatus is the recorded anchor state of a comment against one
// revision (Spec S13.3: recorded per comment per revision).
type AnchorStatus string

const (
	// AnchorExact: line_text found at the anchored line number itself.
	AnchorExact AnchorStatus = "exact"
	// AnchorMapped: line_text found at the diff-mapped position (ladder
	// step 1+2).
	AnchorMapped AnchorStatus = "mapped"
	// AnchorDrifted: line_text found within ⚙ review.anchor_drift_lines of
	// the (mapped) position (ladder step 3).
	AnchorDrifted AnchorStatus = "drifted"
	// AnchorFile: degraded to a file-level comment, original quote kept
	// visible (ladder step 4; Gerrit's next-best-location contract).
	AnchorFile AnchorStatus = "file"
	// AnchorOrphan: no live location in the revision — an EXPLICIT state
	// that keeps the quote and the original-revision link, stays visible
	// on the review surface (synthetic strip), and still drains as
	// file-level. Never silently dropped (Spec S13.3; P-T12-2).
	AnchorOrphan AnchorStatus = "orphan"
)

// Comment lifecycle (Spec S13.3).
const (
	StatusOpen     = "open"
	StatusConsumed = "consumed"
)

// Sides of a diff view (FC-v1 §2 anchor record).
const (
	SideOld = "old"
	SideNew = "new"
)

// AnchorRecord is the FC-v1 §2 binding anchor shape: line_text is the
// quote selector, (side, line_no) the position selector — the W3C
// redundancy principle in compact, closed-world form.
type AnchorRecord struct {
	FilePath string `json:"file_path"`
	Side     string `json:"side"` // old | new
	LineNo   int    `json:"line_no"`
	LineText string `json:"line_text"`
}

// Placement is one recorded anchor state: where (and whether) a comment
// anchors in one revision.
type Placement struct {
	CommentID int64        `json:"comment_id"`
	RevisionN int          `json:"revision_n"`
	Status    AnchorStatus `json:"status"`
	// Anchor is the live position for exact/mapped/drifted, the file for
	// file-level (LineNo 0), and zero-valued for orphan (the quote lives
	// on the comment row).
	Anchor AnchorRecord `json:"anchor,omitempty"`
}

// Comment is one review_comments row.
type Comment struct {
	ID            int64  `json:"id"`
	Owner         string `json:"owner"`
	DeliverableID string `json:"deliverable_id"`
	RevisionN     int    `json:"revision_n"` // the original-revision link
	RunID         string `json:"run_id,omitempty"`
	Kind          string `json:"kind"`
	Severity      string `json:"severity"`
	Category      string `json:"category,omitempty"`
	Criterion     string `json:"criterion,omitempty"`
	Body          string `json:"body"`
	Suggested     string `json:"suggested_change,omitempty"`
	// Anchor is the original AS-CLAIMED anchor (nil-equivalent when the
	// comment was born file-/deliverable-level: LineNo 0).
	Anchor AnchorRecord `json:"anchor,omitempty"`
	// OriginAnchor keeps a non-positional as-supplied anchor string
	// verbatim (verification findings' section/claim-id anchors).
	OriginAnchor string `json:"origin_anchor,omitempty"`
	// BornStatus is the server-validated anchor status against RevisionN.
	BornStatus AnchorStatus `json:"anchor_status"`
	Status     string       `json:"status"`
	// FindingNumber is the [F1..Fn] number stamped at consumption (Spec
	// S13.4: the drain is the numbering authority; 0 while open).
	FindingNumber int    `json:"finding_number,omitempty"`
	ConsumedTS    string `json:"consumed_at,omitempty"`
	ConsumedBy    string `json:"consumed_by,omitempty"`
	CreatedTS     string `json:"created_ts"`
}

// ObjectRef is one content-addressed object pin (Spec S13.2, G2 Def.14):
// the metadata card fields plus the hash ref into the local object dir.
type ObjectRef struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Type   string `json:"type,omitempty"`
}

// Revision is one deliverable_revisions row.
type Revision struct {
	DeliverableID string      `json:"deliverable_id"`
	N             int         `json:"n"`
	Owner         string      `json:"owner"`
	RunID         string      `json:"run_id,omitempty"`
	AttemptRef    string      `json:"attempt_ref,omitempty"`
	PinKind       string      `json:"pin_kind"` // content | objects
	ContentSHA256 string      `json:"content_sha256,omitempty"`
	SnapshotSHA   string      `json:"snapshot_sha,omitempty"` // S13.5 fill (B4-2)
	PlatformRef   string      `json:"platform_ref"`
	Objects       []ObjectRef `json:"objects,omitempty"`
	VerdictRef    int64       `json:"verdict_ref,omitempty"` // verify.round event seq
	CreatedTS     string      `json:"created_ts"`
}

// Deliverable is one deliverables row (Spec S13.1).
type Deliverable struct {
	ID              string `json:"id"`
	Owner           string `json:"owner"`
	TaskID          string `json:"task_id"`
	ProjectID       string `json:"project_id,omitempty"`
	SubjectRef      string `json:"subject_ref,omitempty"`
	Type            string `json:"type"`
	CurrentRevision int    `json:"current_revision"`
	State           string `json:"state"`
	CreatedTS       string `json:"created_ts"`
	UpdatedTS       string `json:"updated_ts"`
}

// RevisionRef is the platform retention ref for a minted revision:
// refs/sinet/deliverable/<id>/rev-<n> (Spec S13.1 [coordinator-draft]).
// The ref itself is created in the platform-owned project store when git
// topology lands (Spec S13.5, B4-2); the namespace is fixed here.
func RevisionRef(deliverableID string, n int) string {
	return fmt.Sprintf("refs/sinet/deliverable/%s/rev-%d", deliverableID, n)
}

// RenderContract is the escape-first rendering contract recorded as
// normative DATA (Spec S13.3; FC-v1 §2): all deliverable and comment
// content renders escaped/text-first on every surface; exactly ONE
// sanctioned raw-HTML channel exists. Enforcement mechanics are Spec
// S15's; the S15 renderer consumes this record.
type RenderContract struct {
	EscapedTextFirst bool
	// SanctionedRawHTML names every channel allowed to render raw markup.
	// The contract admits exactly one (pinned by test).
	SanctionedRawHTML []string
}

// EscapeFirst is the one escape-first contract instance.
func EscapeFirst() RenderContract {
	return RenderContract{
		EscapedTextFirst:  true,
		SanctionedRawHTML: []string{"sandboxed-rendered-document-view"}, // [XREF:S15]
	}
}

// Errors callers branch on.
var (
	ErrNotFound = errors.New("review: not found")
	// ErrRevisionImmutable: re-minting an existing revision number with
	// different content (Spec S13.1: revisions are immutable once minted).
	ErrRevisionImmutable = errors.New("review: a minted revision is immutable")
	// ErrContentDrift: object-dir bytes no longer match a revision's pin.
	ErrContentDrift = errors.New("review: revision content does not match its pin")
	// ErrBadInput covers malformed store inputs.
	ErrBadInput = errors.New("review: invalid input")
)

// Event types minted by this package — provisional pending the S14 event
// contract (B5), extending the CONVENTIONS §7/§8/§13–§21 naming note.
// Payloads are refs + hashes, never bodies (P-T07-5).
const (
	// EventMinted is one minted revision (run-scoped on the minting run).
	EventMinted = "deliverable.minted"
	// EventComment is one comment-recording act (may cover a batch of
	// findings recorded in one transaction).
	EventComment = "review.comment"
	// EventDrained is one S13.4 drain batch: what that rework received.
	EventDrained = "review.drained"
)

const eventSchemaVersion = 1

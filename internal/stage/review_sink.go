package stage

import (
	"context"
	"fmt"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/review"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/run"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/verify"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/worker"
)

// reviewSink adapts review.Store to the verify.ReviewSink seam: the S07
// verification handoff mints revisions into the S13.1 schema, judged
// rounds land as durable review comments, and the retry package's numbered
// findings come from THE S13.4 drain. The adapter owns the identity
// mapping (task → deliverable id, the single-blob logical file name) and
// the Finding ↔ comment-row translation; all behavior is the review
// store's.

// DeliverableFileName is the logical anchor path of a single-blob
// deliverable revision (trees arrive with Spec S13.5, B4-2). It matches
// the anchors the S15 review surface renders and the file_path findings
// parse against.
const DeliverableFileName = "deliverable.md"

// DefinitionFileName is the logical anchor path of a composed
// worker-definition deliverable (its canonical template render).
const DefinitionFileName = "definition.md"

// TaskDeliverableID derives the deliverable id of THE task output (Spec
// S13.1: one deliverable per task output).
func TaskDeliverableID(taskID string) string { return "dlv-" + taskID }

// DefinitionDeliverableID derives the deliverable id of a task's composed
// worker definition (one composition per task, CONVENTIONS §21).
func DefinitionDeliverableID(taskID string) string { return "dlv-" + taskID + "-def" }

// presentDefinition mints a freshly composed worker definition as a
// deliverable at birth (Spec S13.1 "automation definitions ride the same
// machinery"): revision 1 = the canonical render, presented against the
// pre-task state, commentable through the one comment schema. The
// approval-as-diff station consumes this schema; comments on a definition
// have no rework consumer at v0 (composition is one-shot, Spec S08.6) —
// they sit open on the review surface, honestly undrained.
func (s *Skeleton) presentDefinition(ctx context.Context, r run.Run, requester string, v worker.Version) error {
	src, err := s.cfg.Workers.VersionSource(ctx, v.ID)
	if err != nil {
		return err
	}
	dl, err := s.cfg.Review.EnsureDeliverable(ctx, review.EnsureInput{
		ID:         DefinitionDeliverableID(r.TaskID),
		Owner:      requester,
		TaskID:     r.TaskID,
		SubjectRef: v.ID,
		Type:       "worker-definition",
	})
	if err != nil {
		return err
	}
	_, err = s.cfg.Review.MintRevision(ctx, review.MintInput{
		DeliverableID: dl.ID,
		N:             1,
		RunID:         r.ID,
		AttemptRef:    r.ID + "#compose",
		Files:         map[string]string{DefinitionFileName: src},
	})
	return err
}

type reviewSink struct{ s *Skeleton }

func (rs reviewSink) store() *review.Store { return rs.s.cfg.Review }

// ensure resolves (and on the first mint creates) the task deliverable for
// a verify-side handle.
func (rs reviewSink) ensure(ctx context.Context, d verify.Deliverable) (review.Deliverable, error) {
	r, err := rs.s.cfg.Runs.Get(ctx, d.RunID)
	if err != nil {
		return review.Deliverable{}, fmt.Errorf("stage: review sink: %w", err)
	}
	dtype := d.Type
	if dtype == "" {
		dtype = "text"
	}
	return rs.store().EnsureDeliverable(ctx, review.EnsureInput{
		ID:     TaskDeliverableID(d.TaskID),
		Owner:  r.UserID,
		TaskID: d.TaskID,
		Type:   dtype,
	})
}

func (rs reviewSink) MintCandidate(ctx context.Context, d verify.Deliverable, round int) error {
	dl, err := rs.ensure(ctx, d)
	if err != nil {
		return err
	}
	// The round boundary is revision raw material (Spec S13.5, R19): a
	// project-backed run's workspace snapshot sha pins the minted revision;
	// a workspace-less run (the content-pin lane — the walking-skeleton
	// content deliverable, composer definitions) has the seam return "" and
	// records snapshot_sha NULL, a valid honest state. A snapshot ERROR on a
	// repo-backed run is LOUD (CONVENTIONS §14: never faked, F2) — the mint
	// does not proceed to a state indistinguishable from the content-pin lane.
	snapshotSHA := ""
	if rs.s.cfg.Snapshot != nil {
		sha, serr := rs.s.cfg.Snapshot(ctx, d.RunID)
		if serr != nil {
			return fmt.Errorf("stage: round-boundary snapshot for %s: %w", d.RunID, serr)
		}
		snapshotSHA = sha
	}
	if _, err = rs.store().MintRevision(ctx, review.MintInput{
		DeliverableID: dl.ID,
		N:             d.Revision,
		RunID:         d.RunID,
		AttemptRef:    verify.MintRef(d.RunID, round),
		Files:         map[string]string{DeliverableFileName: d.Content},
		SnapshotSHA:   snapshotSHA,
	}); err != nil {
		return err
	}
	// The minted-revision platform ref refs/sinet/deliverable/<id>/rev-<n> is
	// created in the project store OUTSIDE internal/review (Spec S13.1, R20):
	// the composition wires the git side and the review-side fill together.
	if snapshotSHA != "" && rs.s.cfg.CreateRevisionRef != nil {
		if err := rs.s.cfg.CreateRevisionRef(ctx, d.RunID, review.RevisionRef(dl.ID, d.Revision), snapshotSHA); err != nil {
			return err
		}
	}
	return nil
}

func (rs reviewSink) RecordVerdict(ctx context.Context, d verify.Deliverable, eventSeq int64) error {
	return rs.store().SetVerdictRef(ctx, TaskDeliverableID(d.TaskID), d.Revision, eventSeq)
}

func (rs reviewSink) RecordFindings(ctx context.Context, d verify.Deliverable, findings []verify.Finding) error {
	if len(findings) == 0 {
		return nil
	}
	r, err := rs.s.cfg.Runs.Get(ctx, d.RunID)
	if err != nil {
		return fmt.Errorf("stage: review sink: %w", err)
	}
	in := make([]review.FindingInput, 0, len(findings))
	for _, f := range findings {
		kind := review.KindFinding
		if f.Requester {
			kind = review.KindHuman
		}
		in = append(in, review.FindingInput{
			Author:    r.UserID,
			RunID:     d.RunID,
			Kind:      kind,
			Severity:  string(f.Severity),
			Category:  string(f.Category),
			Criterion: f.Criterion,
			Body:      f.Text,
			Suggested: f.SuggestedChange,
			RawAnchor: f.Anchor,
		})
	}
	_, err = rs.store().AddFindings(ctx, TaskDeliverableID(d.TaskID), d.Revision, in)
	return err
}

func (rs reviewSink) RecordGuidance(ctx context.Context, d verify.Deliverable, author string, comments []verify.RequesterComment) error {
	if len(comments) == 0 {
		return nil
	}
	in := make([]review.FindingInput, 0, len(comments))
	for _, c := range comments {
		sev := review.SeverityNote
		if c.Blocking {
			sev = review.SeverityBlocker
		}
		in = append(in, review.FindingInput{
			Author:    author,
			RunID:     d.RunID,
			Kind:      review.KindHuman,
			Severity:  sev,
			Criterion: c.Criterion,
			Body:      c.Text,
			RawAnchor: c.Anchor,
		})
	}
	_, err := rs.store().AddFindings(ctx, TaskDeliverableID(d.TaskID), d.Revision, in)
	return err
}

func (rs reviewSink) DrainOpen(ctx context.Context, d verify.Deliverable, attemptRef string) ([]verify.Finding, error) {
	batch, err := rs.store().Drain(ctx, review.DrainRequest{
		DeliverableID: TaskDeliverableID(d.TaskID),
		AttemptRef:    attemptRef,
		RunID:         d.RunID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]verify.Finding, 0, len(batch))
	for _, b := range batch {
		f := verify.Finding{
			N:               b.Number,
			Severity:        verify.Severity(b.Comment.Severity),
			Category:        verify.Category(b.Comment.Category),
			Criterion:       b.Comment.Criterion,
			Anchor:          drainedAnchor(b),
			Text:            b.Comment.Body,
			SuggestedChange: b.Comment.Suggested,
			Requester:       b.Comment.Kind == review.KindHuman,
		}
		if f.Category == "" && f.Requester {
			// The B2-3 requester-channel category convention.
			f.Category = verify.CatSanityBlocker
		}
		out = append(out, f)
	}
	return out, nil
}

// drainedAnchor renders a drained point's anchor — or its file/orphan
// degradation — for the numbered retry-package point (Spec S13.4:
// delivery is never conditional on anchoring success).
func drainedAnchor(b review.Drained) string {
	p := b.Placement
	switch p.Status {
	case review.AnchorExact, review.AnchorMapped, review.AnchorDrifted:
		return fmt.Sprintf("%s:%d (%s; %q)", p.Anchor.FilePath, p.Anchor.LineNo, p.Status, p.Anchor.LineText)
	case review.AnchorFile:
		if b.Comment.OriginAnchor != "" {
			return fmt.Sprintf("%s (file-level)", b.Comment.OriginAnchor)
		}
		if p.Anchor.FilePath != "" {
			return fmt.Sprintf("%s (file-level)", p.Anchor.FilePath)
		}
		return "(file-level)"
	default: // orphan: quote + original-revision link kept (P-T12-2)
		c := b.Comment
		if c.Anchor.LineNo > 0 {
			return fmt.Sprintf("orphan (was %s:%d @ rev %d; %q)", c.Anchor.FilePath, c.Anchor.LineNo, c.RevisionN, c.Anchor.LineText)
		}
		if c.OriginAnchor != "" {
			return fmt.Sprintf("orphan (was %q @ rev %d)", c.OriginAnchor, c.RevisionN)
		}
		return fmt.Sprintf("orphan (rev %d)", c.RevisionN)
	}
}

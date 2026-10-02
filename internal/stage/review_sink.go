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
		// A composition is minted BY the run that composed it, so the producing
		// run and the minting run are the same run (Spec S08.6: composition is
		// one-shot). Stamping it keeps "every minted revision names its producer"
		// true of the whole family rather than of the drain alone.
		ProducedBy: r.ID,
		Files:      map[string]string{DefinitionFileName: src},
	})
	return err
}

type reviewSink struct{ s *Skeleton }

func (rs reviewSink) store() *review.Store { return rs.s.cfg.Review }

// ensure resolves (and on the first mint creates) the task deliverable for
// a verify-side handle.
//
// snapshotSHA decides what KIND of thing this deliverable is, which is why it
// has to be known before the row is written: for a launch-domain software task
// whose run is repo-backed, the deliverable is the TREE at that snapshot (Spec
// S13.1 "repo-backed types pin a snapshot-commit sha"; S13.2's type table has a
// code row and no markdown-for-an-application row), and the round's written
// report is a companion object beside it. d.Type is the V0 SHAPE type of that
// report — correct for the shape check it was computed for, and a false
// statement about a 23-file application when it is copied onto the row. The
// row's type is identity-immutable once written (migration 0007), so the
// distinction cannot be repaired later; it is made here or never.
func (rs reviewSink) ensure(ctx context.Context, d verify.Deliverable, snapshotSHA string) (review.Deliverable, error) {
	r, err := rs.s.cfg.Runs.Get(ctx, d.RunID)
	if err != nil {
		return review.Deliverable{}, fmt.Errorf("stage: review sink: %w", err)
	}
	dtype := d.Type
	if dtype == "" {
		dtype = "text"
	}
	if snapshotSHA != "" && d.Domain == verify.DomainSoftware {
		dtype = "code"
	}
	return rs.store().EnsureDeliverable(ctx, review.EnsureInput{
		ID:     TaskDeliverableID(d.TaskID),
		Owner:  r.UserID,
		TaskID: d.TaskID,
		Type:   dtype,
	})
}

func (rs reviewSink) MintCandidate(ctx context.Context, d verify.Deliverable, round int) error {
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
	// The snapshot is taken BEFORE the row is ensured, because whether this run
	// is repo-backed is what decides the deliverable's type and that type is
	// immutable from the insert (see ensure).
	dl, err := rs.ensure(ctx, d, snapshotSHA)
	if err != nil {
		return err
	}
	// The MINTING run is the verify leg (d.RunID) and stays that way — the
	// drain's events, checkpoints, judge assemblies and verification tax ride it
	// (Spec S07.11). But the settled S08.8 selection that produced this content
	// is recorded on the EXECUTE leg and only there (`routing.decided` is
	// emitted by the execute dispatch), so the revision carries the producing
	// run beside its minting run: without that join key the S13.6 accept asks
	// the verify leg a question only the execute leg answers, and correct work
	// can never be accepted. It is the SAME lineage-walked identity the
	// verification checks use (executeRunID, P3-RW-6 R12), so a recovery fork
	// names the run that actually did the work rather than its superseded
	// parent. True for every round: rework content is regenerated by revise
	// sessions that ride the verify run but dispatch on the task's recorded
	// execute selection (engines.go, engineRevise).
	if _, err = rs.store().MintRevision(ctx, review.MintInput{
		DeliverableID: dl.ID,
		N:             d.Revision,
		RunID:         d.RunID,
		AttemptRef:    verify.MintRef(d.RunID, round),
		ProducedBy:    rs.s.executeRunID(ctx, d.TaskID),
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

// RevisionChange serves the judge's input slice for a repo-backed revision
// (Spec S07.5 "the artifact + its diff against the previous revision [S13]"):
// the whole change inventory, each text file's unified diff, and the full
// new-side content of the files changed in place — read from the
// platform-owned project store at the PINNED refs through review's read-only
// tree verbs (Spec S13.1/S13.2; CONVENTIONS §78), never from the sandbox and
// never from the stripped verification workspace (Spec S07.3 rule 1).
//
// The pins come from the review store's OWN revision rows, which is why only
// the revision NUMBER is taken from the deliverable: the revise path copies a
// stale snapshot sha onto rework revisions, and a judge slice built on it
// would compare the wrong two trees. Revision 1 compares against the recorded
// pre-task base (old side 0, Spec S13.1).
//
// A revision that pins no snapshot — the content-pin lane — answers an
// AbsentReason in review's own sentence; that is an answer, not a failure.
// A pin the store no longer holds is review.ErrContentDrift and fails the
// round, because serving the executor's report instead would answer a
// different question than the one the judge was asked (§78 F1).
func (rs reviewSink) RevisionChange(ctx context.Context, d verify.Deliverable, bodyBudget int) (verify.RevisionChange, error) {
	id := TaskDeliverableID(d.TaskID)
	ch, err := rs.store().Change(ctx, id, d.Revision-1, d.Revision)
	if err != nil {
		return verify.RevisionChange{}, fmt.Errorf("stage: the change of %s version %d: %w", id, d.Revision, err)
	}
	out := verify.RevisionChange{
		OldN: ch.OldN, NewN: ch.NewN, OldPin: ch.OldPin, NewPin: ch.NewPin,
		OldIsBase: ch.OldIsBase, AbsentReason: ch.AbsentReason,
	}
	if ch.AbsentReason != "" {
		return out, nil
	}
	// The inventory is always whole — it is what makes a change reviewable —
	// and only the BODIES are bounded (Spec S05.3 stage fit).
	out.Files = make([]verify.ChangedFile, 0, len(ch.Files))
	for _, f := range ch.Files {
		out.Files = append(out.Files, verify.ChangedFile{
			Path: f.Path, OldPath: f.OldPath, Kind: f.Kind,
			OldSize: f.OldSize, NewSize: f.NewSize, Binary: f.Binary,
			Additions: f.Additions, Deletions: f.Deletions,
		})
	}
	read := 0
	// The bodies are read in the order the judge is SHOWN them (Spec S07.5;
	// R3), so every body the judge sees is priced against exactly the bytes
	// shown before it, and a reason the wire gives ("it did not fit") is true
	// of what the judge reads: first the diffs in path order up to the first
	// one that does not fit — the contiguous prefix the renderer shows — then
	// the contents of the files changed in place, then, from what is left,
	// the diffs past that cut, which the renderer names but never shows.
	//
	// A body is kept only if it fits what is LEFT of the budget, and a body
	// that does not fit costs the budget NOTHING: the judge is shown whole
	// files or none of a file, so bytes spent on a body no one can be shown
	// would buy the slice nothing and take the remaining room away from every
	// later file. One 100 KB file in the middle of a change must not cost a
	// 40-byte fix at the end its place on the wire.
	//
	// A diff review could serve only in part (Truncated: its prefix stops at
	// a hunk boundary under review's own cap) is never served and never
	// charged, in any pass: the judge is never shown part of a file's diff,
	// so its row is named "too large to be compared" and is NOT a bound cut
	// — the reads go on past it, and "it did not fit" is said only of a body
	// that really did not fit what was left.
	cutAt := len(out.Files)
	for i := range out.Files {
		row := &out.Files[i]
		if row.Binary {
			continue
		}
		if read >= bodyBudget {
			row.BodySkipped, cutAt = true, i
			break
		}
		cmp, err := rs.store().CompareFile(ctx, id, ch.OldN, ch.NewN, row.Path)
		if err != nil {
			return verify.RevisionChange{}, fmt.Errorf("stage: the changes to %s at %s version %d: %w", row.Path, id, d.Revision, err)
		}
		if tooLargeToCompare(row, cmp) {
			continue
		}
		if len(cmp.Unified) > bodyBudget-read {
			row.BodySkipped, cutAt = true, i
			break
		}
		row.Diff, row.DiffTruncated, row.DiffReason = cmp.Unified, cmp.Truncated, cmp.TruncationReason
		read += len(cmp.Unified)
	}
	// Then the new-side content of the files changed in place: the diff shows
	// hunks, and a judge that may only quote hunks cannot say what the file
	// around them now does. An added file's diff IS its content, so it is
	// never read twice; a deleted file has no new side; a binary has no text.
	// A file whose diff is not shown still has its content read: the content
	// is what the judge can quote of it. The renderer shows the contents as a
	// contiguous path-order prefix too, so the first content that does not
	// fit ends the reads here — every one after it is marked, not read.
	contentCut := false
	for i := range out.Files {
		row := &out.Files[i]
		if row.Binary || (row.Kind != verify.KindModified && row.Kind != verify.KindRenamed) {
			continue
		}
		if contentCut || read >= bodyBudget {
			row.ContentSkipped, contentCut = true, true
			continue
		}
		fc, err := rs.store().RevisionFile(ctx, id, ch.NewN, row.Path)
		if err != nil {
			return verify.RevisionChange{}, fmt.Errorf("stage: %s at %s version %d: %w", row.Path, id, d.Revision, err)
		}
		if len(fc.Content) > bodyBudget-read {
			// Only the CONTENT is not served: the row's diff, if it was
			// served, still stands.
			row.ContentSkipped, contentCut = true, true
			continue
		}
		row.Content, row.ContentTruncated, row.ContentReason = fc.Content, fc.Truncated, fc.TruncationReason
		read += len(fc.Content)
	}
	// Last, the diffs past the cut, from what the shown bodies left. The
	// renderer names them as past the cut and shows none of them; reading
	// them still tells it which carry no text at all (a mode or path change),
	// whose true reason it states instead.
	for i := cutAt + 1; i < len(out.Files); i++ {
		row := &out.Files[i]
		if row.Binary {
			continue
		}
		cmp, err := rs.store().CompareFile(ctx, id, ch.OldN, ch.NewN, row.Path)
		if err != nil {
			return verify.RevisionChange{}, fmt.Errorf("stage: the changes to %s at %s version %d: %w", row.Path, id, d.Revision, err)
		}
		if tooLargeToCompare(row, cmp) {
			continue
		}
		if len(cmp.Unified) > bodyBudget-read {
			row.BodySkipped = true
			continue
		}
		row.Diff, row.DiffTruncated, row.DiffReason = cmp.Unified, cmp.Truncated, cmp.TruncationReason
		read += len(cmp.Unified)
	}
	return out, nil
}

// tooLargeToCompare marks row as a diff review could serve only in part and
// reports whether it did: such a row carries DiffTruncated and review's
// reason with no Diff body, costs the judge's budget nothing and never cuts
// the diff section (the renderer names it "too large to be compared").
func tooLargeToCompare(row *verify.ChangedFile, cmp review.Comparison) bool {
	if !cmp.Truncated {
		return false
	}
	row.DiffTruncated, row.DiffReason = true, cmp.TruncationReason
	return true
}

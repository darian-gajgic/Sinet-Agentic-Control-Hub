package review

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// CommentInput is one human review comment (6.2) entering the schema.
// Findings enter through AddFindings — one schema, two ingresses (Spec
// S13.1/S13.3).
type CommentInput struct {
	DeliverableID string
	// RevisionN anchors the comment to a revision (0 = current).
	RevisionN int
	// Author is the commenting user (owner-attributed, 15.6).
	Author   string
	Body     string
	Severity string // blocker | note; "" = note
	// Anchor is the claimed anchor (nil = file-/deliverable-level comment;
	// FileLevel names the file, "" = deliverable-level). Claimed positions
	// are validated server-side and NEVER trusted (FC-v1 §2).
	Anchor    *AnchorRecord
	FileLevel string
	Suggested string
	// RunID scopes the review.comment event to a run when the comment is
	// recorded by run machinery; "" = a platform-scope act (the human
	// surface).
	RunID string
}

// AddComment records one human comment with server-side anchor validation
// (Spec S13.3). The returned comment carries its born anchor status.
func (s *Store) AddComment(ctx context.Context, in CommentInput) (Comment, error) {
	if in.DeliverableID == "" || in.Author == "" || in.Body == "" {
		return Comment{}, fmt.Errorf("%w: comment needs deliverable, author and body", ErrBadInput)
	}
	sev := in.Severity
	if sev == "" {
		sev = SeverityNote
	}
	if sev != SeverityBlocker && sev != SeverityNote {
		return Comment{}, fmt.Errorf("%w: severity %q", ErrBadInput, in.Severity)
	}
	d, err := s.Deliverable(ctx, in.DeliverableID)
	if err != nil {
		return Comment{}, err
	}
	revN := in.RevisionN
	if revN == 0 {
		revN = d.CurrentRevision
	}
	if revN < 1 {
		return Comment{}, fmt.Errorf("%w: deliverable %s has no minted revision to comment on", ErrBadInput, in.DeliverableID)
	}
	row := commentRow{
		owner: in.Author, deliverableID: in.DeliverableID, revisionN: revN,
		runID: in.RunID, kind: KindHuman, severity: sev,
		body: in.Body, suggested: in.Suggested,
	}
	if in.Anchor != nil {
		files, err := s.RevisionFiles(ctx, in.DeliverableID, revN)
		if err != nil {
			return Comment{}, err
		}
		drift, err := s.drift()
		if err != nil {
			return Comment{}, err
		}
		status, placed := validateBirth(files, *in.Anchor, drift)
		row.claim = in.Anchor
		row.born = status
		row.birthPlacement = placed
	} else {
		row.born = AnchorFile
		if in.FileLevel != "" {
			row.claim = &AnchorRecord{FilePath: in.FileLevel}
		}
	}
	ids, err := s.insertComments(ctx, []commentRow{row})
	if err != nil {
		return Comment{}, err
	}
	return s.CommentByID(ctx, ids[0])
}

// FindingInput is one verification finding (or requester point delivered
// through the machinery) entering the schema (Spec S13.1: findings add
// severity, description, suggested change and — at drain time — the
// finding number).
type FindingInput struct {
	Author    string // the run owner (15.6)
	RunID     string
	Kind      string // finding | human (requester points, 6.2)
	Severity  string
	Category  string
	Criterion string
	Body      string
	Suggested string
	// RawAnchor is the opaque anchor string the S07 pipeline emits
	// ("file:line / section / claim id"); it parses positional where it
	// names revision content and records file-level otherwise, kept
	// verbatim in origin_anchor — never refused (P-T12-2).
	RawAnchor string
}

// AddFindings records a judged round's findings (and requester points)
// against one revision in one transaction — the S07.6 "notes ride along
// to the requester as review comments" landing point.
func (s *Store) AddFindings(ctx context.Context, deliverableID string, revisionN int, findings []FindingInput) ([]int64, error) {
	if len(findings) == 0 {
		return nil, nil
	}
	if _, err := s.Deliverable(ctx, deliverableID); err != nil {
		return nil, err
	}
	files, err := s.RevisionFiles(ctx, deliverableID, revisionN)
	if err != nil {
		return nil, err
	}
	drift, err := s.drift()
	if err != nil {
		return nil, err
	}
	rows := make([]commentRow, 0, len(findings))
	for _, f := range findings {
		if f.Author == "" || f.Body == "" {
			return nil, fmt.Errorf("%w: finding needs author and body", ErrBadInput)
		}
		kind := f.Kind
		if kind == "" {
			kind = KindFinding
		}
		if kind != KindFinding && kind != KindHuman {
			return nil, fmt.Errorf("%w: finding kind %q", ErrBadInput, f.Kind)
		}
		sev := f.Severity
		if sev != SeverityBlocker && sev != SeverityNote {
			return nil, fmt.Errorf("%w: finding severity %q", ErrBadInput, f.Severity)
		}
		row := commentRow{
			owner: f.Author, deliverableID: deliverableID, revisionN: revisionN,
			runID: f.RunID, kind: kind, severity: sev,
			category: f.Category, criterion: f.Criterion,
			body: f.Body, suggested: f.Suggested,
			originAnchor: f.RawAnchor,
		}
		if a, ok := ParseFindingAnchor(files, f.RawAnchor); ok {
			status, placed := validateBirth(files, a, drift)
			row.claim = &a
			row.born = status
			row.birthPlacement = placed
		} else {
			row.born = AnchorFile
		}
		rows = append(rows, row)
	}
	return s.insertComments(ctx, rows)
}

// commentRow is the internal insert shape.
type commentRow struct {
	owner, deliverableID  string
	revisionN             int
	runID, kind, severity string
	category, criterion   string
	body, suggested       string
	originAnchor          string
	claim                 *AnchorRecord // as-claimed anchor (may be file-only)
	born                  AnchorStatus
	birthPlacement        AnchorRecord // validated position for exact/drifted
}

// insertComments writes comment rows + their birth anchor states + one
// review.comment event in ONE transaction (append-only rows; the event is
// refs + hashes, P-T07-5).
func (s *Store) insertComments(ctx context.Context, rows []commentRow) ([]int64, error) {
	now := s.nowRFC3339()
	ids := make([]int64, 0, len(rows))
	err := s.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		type evComment struct {
			ID       int64  `json:"id"`
			Kind     string `json:"kind"`
			Severity string `json:"severity"`
			Status   string `json:"anchor_status"`
			BodySHA  string `json:"body_sha256"`
		}
		var evs []evComment
		for _, r := range rows {
			var fp, side, lineText any
			var lineNo any
			if r.claim != nil {
				if r.claim.FilePath != "" {
					fp = r.claim.FilePath
				}
				if r.claim.LineNo > 0 {
					sd := r.claim.Side
					if sd == "" {
						sd = SideNew
					}
					side, lineNo, lineText = sd, r.claim.LineNo, r.claim.LineText
				}
			}
			res, err := tx.ExecContext(ctx,
				`INSERT INTO review_comments (user_id, deliverable_id, revision_n, run_id, kind, severity,
				                              category, criterion, body, suggested_change,
				                              file_path, side, line_no, line_text, origin_anchor,
				                              anchor_status, status, created_ts)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'open', ?)`,
				r.owner, r.deliverableID, r.revisionN, r.runID, r.kind, r.severity,
				r.category, r.criterion, r.body, r.suggested,
				fp, side, lineNo, lineText, r.originAnchor, string(r.born), now)
			if err != nil {
				return fmt.Errorf("review: insert comment: %w", err)
			}
			id, err := res.LastInsertId()
			if err != nil {
				return err
			}
			ids = append(ids, id)
			// Birth anchor state: the validated placement against the
			// comment's own revision (Spec S13.3 per-comment-per-revision
			// record starts at birth).
			birth := Placement{CommentID: id, RevisionN: r.revisionN, Status: r.born}
			switch r.born {
			case AnchorExact, AnchorDrifted:
				birth.Anchor = r.birthPlacement
			case AnchorFile:
				if r.claim != nil && r.claim.FilePath != "" {
					birth.Anchor = AnchorRecord{FilePath: r.claim.FilePath}
				}
			}
			if err := insertPlacementTx(ctx, tx, birth, now); err != nil {
				return err
			}
			evs = append(evs, evComment{
				ID: id, Kind: r.kind, Severity: r.severity,
				Status: string(r.born), BodySHA: hashBytes([]byte(r.body)),
			})
		}
		first := rows[0]
		payload, err := json.Marshal(map[string]any{
			"deliverable_id": first.deliverableID,
			"revision_n":     first.revisionN,
			"comments":       evs,
		})
		if err != nil {
			return err
		}
		a := eventlog.Append{
			UserID: first.owner, Type: EventComment,
			SchemaVersion: eventSchemaVersion, Payload: payload, Time: s.now(),
		}
		if first.runID != "" {
			owner, gen, err := s.runScope(ctx, tx, first.runID)
			if err != nil {
				return err
			}
			a.RunID, a.Generation, a.UserID = first.runID, gen, owner
		}
		_, err = s.Log.AppendTx(ctx, tx, a)
		return err
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func insertPlacementTx(ctx context.Context, tx *sql.Tx, p Placement, now string) error {
	var fp, side, lineText any
	var lineNo any
	if p.Anchor.FilePath != "" {
		fp = p.Anchor.FilePath
	}
	if p.Anchor.LineNo > 0 {
		side, lineNo, lineText = p.Anchor.Side, p.Anchor.LineNo, p.Anchor.LineText
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO review_comment_anchors (comment_id, revision_n, status, file_path, side, line_no, line_text, computed_ts)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (comment_id, revision_n) DO NOTHING`,
		p.CommentID, p.RevisionN, string(p.Status), fp, side, lineNo, lineText, now)
	if err != nil {
		return fmt.Errorf("review: record placement: %w", err)
	}
	return nil
}

// CommentByID reads one comment row.
func (s *Store) CommentByID(ctx context.Context, id int64) (Comment, error) {
	c, err := scanComment(s.DB.QueryRowContext(ctx, selectComment+` WHERE comment_id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Comment{}, fmt.Errorf("%w: comment %d", ErrNotFound, id)
	}
	return c, err
}

// OpenComments lists every open comment on a deliverable in insertion
// order (any origin revision — the drain ports them all, Spec S13.4).
func (s *Store) OpenComments(ctx context.Context, deliverableID string) ([]Comment, error) {
	rows, err := s.DB.QueryContext(ctx,
		selectComment+` WHERE deliverable_id = ? AND status = 'open' ORDER BY comment_id`, deliverableID)
	if err != nil {
		return nil, fmt.Errorf("review: list open comments: %w", err)
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

const selectComment = `SELECT comment_id, user_id, deliverable_id, revision_n, run_id, kind, severity,
       category, criterion, body, suggested_change, file_path, side, line_no, line_text,
       origin_anchor, anchor_status, status, finding_number, consumed_ts, consumed_by, created_ts
  FROM review_comments`

type rowScanner interface{ Scan(dest ...any) error }

func scanComment(r rowScanner) (Comment, error) {
	var (
		c        Comment
		fp, side sql.NullString
		lineNo   sql.NullInt64
		lineText sql.NullString
		fnum     sql.NullInt64
		consTS   sql.NullString
	)
	err := r.Scan(&c.ID, &c.Owner, &c.DeliverableID, &c.RevisionN, &c.RunID, &c.Kind, &c.Severity,
		&c.Category, &c.Criterion, &c.Body, &c.Suggested, &fp, &side, &lineNo, &lineText,
		&c.OriginAnchor, (*string)(&c.BornStatus), &c.Status, &fnum, &consTS, &c.ConsumedBy, &c.CreatedTS)
	if err != nil {
		return Comment{}, err
	}
	if fp.Valid {
		c.Anchor.FilePath = fp.String
	}
	if side.Valid {
		c.Anchor.Side = side.String
	}
	if lineNo.Valid {
		c.Anchor.LineNo = int(lineNo.Int64)
	}
	if lineText.Valid {
		c.Anchor.LineText = lineText.String
	}
	if fnum.Valid {
		c.FindingNumber = int(fnum.Int64)
	}
	if consTS.Valid {
		c.ConsumedTS = consTS.String
	}
	return c, nil
}

// PlacedComments returns every comment of a deliverable with its placement
// against revision n — the render path (FC-v1 §2: anchors are re-validated
// server-side on EVERY render; a comment without a live anchor renders in
// the always-visible synthetic strip, so no comment can lack a render
// location). Missing placements are computed up the ladder and recorded;
// recorded placements are re-validated against the (immutable) revision
// content and a disagreement is a loud error, never a silent mis-anchor
// (P-T12-2).
func (s *Store) PlacedComments(ctx context.Context, deliverableID string, n int) ([]Comment, []Placement, error) {
	comments, err := s.allComments(ctx, deliverableID)
	if err != nil {
		return nil, nil, err
	}
	placements, err := s.placeAll(ctx, deliverableID, n, comments)
	if err != nil {
		return nil, nil, err
	}
	return comments, placements, nil
}

func (s *Store) allComments(ctx context.Context, deliverableID string) ([]Comment, error) {
	rows, err := s.DB.QueryContext(ctx,
		selectComment+` WHERE deliverable_id = ? ORDER BY comment_id`, deliverableID)
	if err != nil {
		return nil, fmt.Errorf("review: list comments: %w", err)
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// placeAll computes/loads placements for the given comments against
// revision n, recording newly computed ones (Spec S13.3: every port is
// recorded; re-validation on every read).
func (s *Store) placeAll(ctx context.Context, deliverableID string, n int, comments []Comment) ([]Placement, error) {
	drift, err := s.drift()
	if err != nil {
		return nil, err
	}
	fileCache := map[int]map[string]string{}
	filesAt := func(rev int) (map[string]string, error) {
		if rev < 1 {
			return nil, nil // pre-task base: S13.5 materializes it (B4-2)
		}
		if f, ok := fileCache[rev]; ok {
			return f, nil
		}
		f, err := s.RevisionFiles(ctx, deliverableID, rev)
		if err != nil {
			return nil, err
		}
		fileCache[rev] = f
		return f, nil
	}

	stored, err := s.storedPlacements(ctx, deliverableID, n)
	if err != nil {
		return nil, err
	}
	var (
		out     []Placement
		toWrite []Placement
	)
	for _, c := range comments {
		computed, err := s.computePlacement(c, n, filesAt, drift)
		if err != nil {
			return nil, err
		}
		if have, ok := stored[c.ID]; ok {
			// Re-validation: recompute and compare — stored positions are
			// never trusted blind (FC-v1 §2). Immutable revisions make a
			// mismatch structurally impossible; observing one is loud.
			if have.Status != computed.Status || have.Anchor != computed.Anchor {
				return nil, fmt.Errorf("review: placement of comment %d on rev %d fails re-validation (stored %s@%+v, recomputed %s@%+v)",
					c.ID, n, have.Status, have.Anchor, computed.Status, computed.Anchor)
			}
			out = append(out, have)
			continue
		}
		out = append(out, computed)
		toWrite = append(toWrite, computed)
	}
	if len(toWrite) > 0 {
		now := s.nowRFC3339()
		err := s.DB.WriteTx(ctx, func(tx *sql.Tx) error {
			for _, p := range toWrite {
				if err := insertPlacementTx(ctx, tx, p, now); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// computePlacement derives one comment's placement against revision n: the
// birth validation for its own revision, the port ladder for any other.
func (s *Store) computePlacement(c Comment, n int, filesAt func(int) (map[string]string, error), drift int) (Placement, error) {
	p := Placement{CommentID: c.ID, RevisionN: n}
	dstFiles, err := filesAt(n)
	if err != nil {
		return Placement{}, err
	}
	claim := c.Anchor
	if c.Anchor.LineNo == 0 && c.Anchor.FilePath == "" {
		// Born deliverable-level: file-level placement always.
		p.Status = AnchorFile
		return p, nil
	}
	if n == c.RevisionN {
		status, placed := validateBirth(dstFiles, claim, drift)
		if c.Anchor.LineNo == 0 {
			// Born file-only: stays file-level while the file exists.
			if _, ok := dstFiles[claim.FilePath]; ok {
				p.Status, p.Anchor = AnchorFile, AnchorRecord{FilePath: claim.FilePath}
			} else {
				p.Status = AnchorFile
			}
			return p, nil
		}
		p.Status = status
		switch status {
		case AnchorExact, AnchorDrifted:
			p.Anchor = placed
		case AnchorFile:
			if _, ok := dstFiles[claim.FilePath]; ok {
				p.Anchor = AnchorRecord{FilePath: claim.FilePath}
			}
		}
		return p, nil
	}
	// Port: source content is the anchor-side content of the origin view —
	// side old lives in revision N−1, side new in revision N (Spec S13.3
	// ladder step 1).
	srcRev := c.RevisionN
	if claim.Side == SideOld {
		srcRev = c.RevisionN - 1
	}
	srcFiles, err := filesAt(srcRev)
	if err != nil {
		return Placement{}, err
	}
	status, placed := portAnchor(srcFiles, dstFiles, claim, drift)
	p.Status = status
	switch status {
	case AnchorExact, AnchorMapped, AnchorDrifted, AnchorFile:
		p.Anchor = placed
	}
	return p, nil
}

func (s *Store) storedPlacements(ctx context.Context, deliverableID string, n int) (map[int64]Placement, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT a.comment_id, a.status, a.file_path, a.side, a.line_no, a.line_text
		   FROM review_comment_anchors a
		   JOIN review_comments c ON c.comment_id = a.comment_id
		  WHERE c.deliverable_id = ? AND a.revision_n = ?`, deliverableID, n)
	if err != nil {
		return nil, fmt.Errorf("review: load placements: %w", err)
	}
	defer rows.Close()
	out := map[int64]Placement{}
	for rows.Next() {
		var (
			p        Placement
			fp, side sql.NullString
			lineNo   sql.NullInt64
			lineText sql.NullString
		)
		if err := rows.Scan(&p.CommentID, (*string)(&p.Status), &fp, &side, &lineNo, &lineText); err != nil {
			return nil, err
		}
		p.RevisionN = n
		if fp.Valid {
			p.Anchor.FilePath = fp.String
		}
		if side.Valid {
			p.Anchor.Side = side.String
		}
		if lineNo.Valid {
			p.Anchor.LineNo = int(lineNo.Int64)
		}
		if lineText.Valid {
			p.Anchor.LineText = lineText.String
		}
		out[p.CommentID] = p
	}
	return out, rows.Err()
}

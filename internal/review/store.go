package review

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// Settings is this package's view of the settings registry (CONVENTIONS
// §2: consumers read ⚙ values by dotted key through narrow per-package
// interfaces). *settings.Registry satisfies it.
type Settings interface {
	Int(key string) (int64, error)
}

// keyAnchorDrift is ⚙ review.anchor_drift_lines (Spec S13.3 ladder step 3;
// FC-v1 §2 carried N15 behavior) — read per port, never a constant.
const keyAnchorDrift = "review.anchor_drift_lines"

// Store is the S13.1–S13.4 data layer over platform.db plus the
// content-addressed object dir under Root.
type Store struct {
	DB       *storage.DB
	Log      *eventlog.Log
	Settings Settings
	// Root is the platform-owned review store directory (object dir home).
	Root string
	// PixelDiff is the OPTIONAL S13.2 server-side pixel-diff aid for image
	// comparisons. Nil at v0: the comparison surface is the two revisions'
	// object refs; the 2-up/swipe/onion trio renders from them (Spec S15).
	PixelDiff PixelDiffAid
	// BaseContent resolves the pre-task base (project HEAD at branch base)
	// content for a repo-backed deliverable — revision 1's old side (Spec
	// S13.5/S13.1). Nil, or a non-repo deliverable (ok=false), keeps the
	// empty-base behaviour. Wired by the composition root to the git topology
	// package; review's import set stays storage+eventlog only (the PixelDiff
	// seam precedent, R25).
	BaseContent BaseContentSource
	// Now is the test clock seam.
	Now func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Store) nowRFC3339() string {
	return s.now().UTC().Format(time.RFC3339Nano)
}

func (s *Store) drift() (int, error) {
	if s.Settings == nil {
		return 0, fmt.Errorf("review: settings registry not wired (⚙ %s)", keyAnchorDrift)
	}
	v, err := s.Settings.Int(keyAnchorDrift)
	if err != nil {
		return 0, fmt.Errorf("review: read ⚙ %s: %w", keyAnchorDrift, err)
	}
	return int(v), nil
}

// runScope reads a run's owner and current generation for run-scoped event
// appends (control-plane acts append at the run's current generation,
// CONVENTIONS §8).
func (s *Store) runScope(ctx context.Context, tx *sql.Tx, runID string) (owner string, generation int64, err error) {
	err = tx.QueryRowContext(ctx,
		`SELECT user_id, generation FROM runs WHERE run_id = ?`, runID).
		Scan(&owner, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, fmt.Errorf("%w: run %q", ErrNotFound, runID)
	}
	if err != nil {
		return "", 0, fmt.Errorf("review: read run %q: %w", runID, err)
	}
	return owner, generation, nil
}

// EnsureInput identifies (and on first sight creates) a deliverable row.
type EnsureInput struct {
	ID         string
	Owner      string
	TaskID     string
	ProjectID  string
	SubjectRef string
	Type       string
}

// EnsureDeliverable returns the deliverable row, creating it in-review at
// current_revision 0 when absent (Spec S13.1: the long-lived entity; one
// per task output — identity is the caller-derived id).
func (s *Store) EnsureDeliverable(ctx context.Context, in EnsureInput) (Deliverable, error) {
	if in.ID == "" || in.Owner == "" || in.TaskID == "" || in.Type == "" {
		return Deliverable{}, fmt.Errorf("%w: ensure needs id, owner, task and type", ErrBadInput)
	}
	if d, err := s.Deliverable(ctx, in.ID); err == nil {
		return d, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Deliverable{}, err
	}
	now := s.nowRFC3339()
	err := s.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		// S13.1's row contract names a PROJECT REF, and both mint call sites
		// (internal/stage/review_sink.go) know the task but not the project —
		// so the row was born projectless and the accept door then refused a
		// repo-backed accept with "belongs to no project" (P3-RW-18 D2-R1).
		// The edge is platform data, not a caller's to carry: task_project is
		// the migration-owned view (0016, re-created by 0022 and 0023) that
		// resolves approved claims, else the intake registry pin, else an
		// onboarding id. Resolving it IN the insert means this row cannot
		// disagree with the views, because it IS the view — the reads.go §37
		// "SAME expression" precedent. An explicitly passed project wins; a
		// task with no registered project stays honestly ''.
		_, err := tx.ExecContext(ctx,
			`INSERT INTO deliverables (deliverable_id, user_id, task_id, project_id, subject_ref, dtype,
			                           current_revision, state, created_ts, updated_ts)
			 VALUES (?, ?, ?,
			         COALESCE(NULLIF(?, ''), (SELECT project_id FROM task_project WHERE task_id = ?), ''),
			         ?, ?, 0, ?, ?, ?)
			 ON CONFLICT (deliverable_id) DO NOTHING`,
			in.ID, in.Owner, in.TaskID, in.ProjectID, in.TaskID, in.SubjectRef, in.Type, StateInReview, now, now)
		return err
	})
	if err != nil {
		return Deliverable{}, fmt.Errorf("review: create deliverable %q: %w", in.ID, err)
	}
	return s.Deliverable(ctx, in.ID)
}

// deliverableProject is the serve-time project expression, shared by every
// read that hands a ProjectID out (P3-RW-18 D2-R2).
//
// Rows minted before D2-R1 carry an empty project_id — the live walk's dlv-t-1e211253dfa21c28
// among them — and the fact they are missing is not lost, only unrecorded on
// that row: task_project still knows it. Serving through the join heals every
// consumer (the accept door, the card, the lists) with no data rewrite, which
// is why this packet adds no migration. It resolves recorded facts only: a
// task with no registered project still serves an empty project_id, because inventing a linkage
// would be worse than admitting there is none.
const deliverableProject = `COALESCE(NULLIF(d.project_id, ''), tp.project_id, '')`

// deliverableProjectJoin is the LEFT JOIN deliverableProject reads through. The
// join is LEFT so a projectless task keeps its row rather than dropping out.
const deliverableProjectJoin = ` LEFT JOIN task_project tp ON tp.task_id = d.task_id`

// Deliverable reads one deliverable row.
func (s *Store) Deliverable(ctx context.Context, id string) (Deliverable, error) {
	var d Deliverable
	err := s.DB.QueryRowContext(ctx,
		`SELECT d.deliverable_id, d.user_id, d.task_id, `+deliverableProject+`, d.subject_ref, d.dtype,
		        d.current_revision, d.state, d.created_ts, d.updated_ts
		 FROM deliverables d`+deliverableProjectJoin+` WHERE d.deliverable_id = ?`, id).
		Scan(&d.ID, &d.Owner, &d.TaskID, &d.ProjectID, &d.SubjectRef, &d.Type,
			&d.CurrentRevision, &d.State, &d.CreatedTS, &d.UpdatedTS)
	if errors.Is(err, sql.ErrNoRows) {
		return Deliverable{}, fmt.Errorf("%w: deliverable %q", ErrNotFound, id)
	}
	if err != nil {
		return Deliverable{}, fmt.Errorf("review: read deliverable %q: %w", id, err)
	}
	return d, nil
}

// MintInput mints one revision (Spec S13.1: minting happens when a
// candidate passes to review — one per round; the verification handoff
// wires it via verify.ReviewSink). Exactly one of Files (content pin) or
// Objects (content-addressed binary pin) is set.
type MintInput struct {
	DeliverableID string
	// N is the revision number to mint; it must be current_revision+1 —
	// revisions are 1..N with every hop persisted (Spec S13.1). Re-minting
	// the CURRENT revision with identical content is an idempotent no-op
	// (the S07.7 resume re-enters on the pinned revision); different
	// content is ErrRevisionImmutable.
	N          int
	RunID      string
	AttemptRef string
	// Files is the text content per logical path (single-blob deliverables
	// use one entry; trees arrive with S13.5).
	Files map[string]string
	// Objects carries binary payloads by name (bytes stored
	// content-addressed; the row pins the hash refs).
	Objects map[string][]byte
	// Types optionally labels object MIME/kind per name (metadata cards).
	Types map[string]string
	// SnapshotSHA is the S13.5 repo-backed snapshot-commit pin for a
	// repo-backed revision (Spec S13.1: repo-backed types pin a snapshot-commit
	// sha). Settable at insert; empty leaves snapshot_sha NULL — the honest
	// content-pin lane (composer definitions, non-repo tasks). The platform ref
	// is created OUTSIDE this package (the composition wires the git side and
	// this fill together, R20).
	SnapshotSHA string
}

// MintRevision mints revision N and advances current_revision, appending
// the deliverable.minted event run-scoped in the same transaction.
func (s *Store) MintRevision(ctx context.Context, in MintInput) (Revision, error) {
	if in.DeliverableID == "" || in.N < 1 {
		return Revision{}, fmt.Errorf("%w: mint needs a deliverable and n >= 1", ErrBadInput)
	}
	if (len(in.Files) == 0) == (len(in.Objects) == 0) {
		return Revision{}, fmt.Errorf("%w: mint takes exactly one of files or objects", ErrBadInput)
	}
	d, err := s.Deliverable(ctx, in.DeliverableID)
	if err != nil {
		return Revision{}, err
	}

	// Store bytes content-addressed FIRST (idempotent, outside the tx —
	// the single-connection pool never nests foreign work in a WriteTx).
	var refs []ObjectRef
	pinKind := "objects"
	contentSHA := ""
	if len(in.Files) > 0 {
		pinKind = "content"
		names := make([]string, 0, len(in.Files))
		for name := range in.Files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			data := []byte(in.Files[name])
			h, err := s.putObject(data)
			if err != nil {
				return Revision{}, err
			}
			refs = append(refs, ObjectRef{Name: name, Size: int64(len(data)), SHA256: h, Type: "text"})
		}
		contentSHA = contentPin(in.Files, refs)
	} else {
		names := make([]string, 0, len(in.Objects))
		for name := range in.Objects {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			data := in.Objects[name]
			h, err := s.putObject(data)
			if err != nil {
				return Revision{}, err
			}
			refs = append(refs, ObjectRef{Name: name, Size: int64(len(data)), SHA256: h, Type: in.Types[name]})
		}
	}

	// Idempotent re-mint of an existing revision: same pin → no-op read
	// back; a different pin violates immutability (Spec S13.1).
	if existing, err := s.RevisionAt(ctx, in.DeliverableID, in.N); err == nil {
		if existing.PinKind == pinKind && existing.ContentSHA256 == contentSHA && sameRefs(existing.Objects, refs) {
			return existing, nil
		}
		return Revision{}, fmt.Errorf("%w: %s rev %d is minted with a different pin",
			ErrRevisionImmutable, in.DeliverableID, in.N)
	} else if !errors.Is(err, ErrNotFound) {
		return Revision{}, err
	}
	if in.N != d.CurrentRevision+1 {
		return Revision{}, fmt.Errorf("%w: mint n=%d but current is %d (revisions are 1..N, every hop persisted)",
			ErrBadInput, in.N, d.CurrentRevision)
	}

	objJSON, err := json.Marshal(refs)
	if err != nil {
		return Revision{}, fmt.Errorf("review: marshal object refs: %w", err)
	}
	now := s.nowRFC3339()
	rev := Revision{
		DeliverableID: in.DeliverableID, N: in.N, Owner: d.Owner,
		RunID: in.RunID, AttemptRef: in.AttemptRef,
		PinKind: pinKind, ContentSHA256: contentSHA,
		SnapshotSHA: in.SnapshotSHA,
		PlatformRef: RevisionRef(in.DeliverableID, in.N),
		Objects:     refs, CreatedTS: now,
	}
	err = s.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO deliverable_revisions (deliverable_id, n, user_id, run_id, attempt_ref,
			                                    pin_kind, content_sha256, snapshot_sha, platform_ref, objects, created_ts)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			rev.DeliverableID, rev.N, rev.Owner, rev.RunID, rev.AttemptRef,
			rev.PinKind, rev.ContentSHA256, nullString(rev.SnapshotSHA), rev.PlatformRef, string(objJSON), now); err != nil {
			return fmt.Errorf("review: mint revision: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE deliverables SET current_revision = ?, updated_ts = ? WHERE deliverable_id = ?`,
			rev.N, now, rev.DeliverableID); err != nil {
			return fmt.Errorf("review: advance current_revision: %w", err)
		}
		payload, err := json.Marshal(map[string]any{
			"deliverable_id": rev.DeliverableID,
			"n":              rev.N,
			"pin_kind":       rev.PinKind,
			"content_sha256": rev.ContentSHA256,
			"platform_ref":   rev.PlatformRef,
			"attempt_ref":    rev.AttemptRef,
			"objects":        len(refs),
		})
		if err != nil {
			return err
		}
		a := eventlog.Append{
			UserID: d.Owner, Type: EventMinted,
			SchemaVersion: eventSchemaVersion, Payload: payload, Time: s.now(),
		}
		if rev.RunID != "" {
			owner, gen, err := s.runScope(ctx, tx, rev.RunID)
			if err != nil {
				return err
			}
			a.RunID, a.Generation, a.UserID = rev.RunID, gen, owner
		}
		_, err = s.Log.AppendTx(ctx, tx, a)
		return err
	})
	if err != nil {
		return Revision{}, err
	}
	return rev, nil
}

// contentPin computes the revision content hash: the single file's content
// hash for single-blob revisions (identity with the S07/B2-5 round-record
// pin), or the hash of the sorted (path, sha256) manifest for trees.
func contentPin(files map[string]string, refs []ObjectRef) string {
	if len(files) == 1 {
		for _, content := range files {
			return hashBytes([]byte(content))
		}
	}
	manifest, _ := json.Marshal(refs)
	return hashBytes(manifest)
}

// nullString maps an empty string to a SQL NULL (snapshot_sha is NULL until
// the S13.5 fill).
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func sameRefs(a, b []ObjectRef) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// RevisionAt reads one minted revision row.
func (s *Store) RevisionAt(ctx context.Context, deliverableID string, n int) (Revision, error) {
	var (
		r       Revision
		objJSON string
		snap    sql.NullString
		verdict sql.NullInt64
	)
	err := s.DB.QueryRowContext(ctx,
		`SELECT deliverable_id, n, user_id, run_id, attempt_ref, pin_kind, content_sha256,
		        snapshot_sha, platform_ref, objects, verdict_ref, created_ts
		 FROM deliverable_revisions WHERE deliverable_id = ? AND n = ?`, deliverableID, n).
		Scan(&r.DeliverableID, &r.N, &r.Owner, &r.RunID, &r.AttemptRef, &r.PinKind,
			&r.ContentSHA256, &snap, &r.PlatformRef, &objJSON, &verdict, &r.CreatedTS)
	if errors.Is(err, sql.ErrNoRows) {
		return Revision{}, fmt.Errorf("%w: %s rev %d", ErrNotFound, deliverableID, n)
	}
	if err != nil {
		return Revision{}, fmt.Errorf("review: read revision: %w", err)
	}
	if snap.Valid {
		r.SnapshotSHA = snap.String
	}
	if verdict.Valid {
		r.VerdictRef = verdict.Int64
	}
	if err := json.Unmarshal([]byte(objJSON), &r.Objects); err != nil {
		return Revision{}, fmt.Errorf("review: decode object refs: %w", err)
	}
	return r, nil
}

// Revisions lists a deliverable's full lineage 1..N (never compressed).
func (s *Store) Revisions(ctx context.Context, deliverableID string) ([]Revision, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT n FROM deliverable_revisions WHERE deliverable_id = ? ORDER BY n`, deliverableID)
	if err != nil {
		return nil, fmt.Errorf("review: list revisions: %w", err)
	}
	var ns []int
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		ns = append(ns, n)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	out := make([]Revision, 0, len(ns))
	for _, n := range ns {
		r, err := s.RevisionAt(ctx, deliverableID, n)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// RevisionFiles loads a content-pinned revision's text files from the
// object dir, hash-verified against the pin (refusing drifted content —
// the B2-5 posture). Object-pinned revisions return an empty map (their
// comparison surface is metadata cards, Spec S13.2).
func (s *Store) RevisionFiles(ctx context.Context, deliverableID string, n int) (map[string]string, error) {
	r, err := s.RevisionAt(ctx, deliverableID, n)
	if err != nil {
		return nil, err
	}
	files := map[string]string{}
	if r.PinKind != "content" {
		return files, nil
	}
	for _, ref := range r.Objects {
		data, err := s.readObject(ref.SHA256)
		if err != nil {
			return nil, err
		}
		files[ref.Name] = string(data)
	}
	if got := contentPin(files, r.Objects); got != r.ContentSHA256 {
		return nil, fmt.Errorf("%w: %s rev %d content pin %s, recomputed %s",
			ErrContentDrift, deliverableID, n, r.ContentSHA256, got)
	}
	return files, nil
}

// SetSnapshotSHA fills a revision's S13.5 snapshot-commit pin (fill-once,
// NULL → sha; the migration trigger enforces it — a second, DIFFERENT fill
// aborts). Idempotent when the same sha is re-supplied (the S07.7 resume
// re-enters on the pinned revision). The platform ref
// refs/sinet/deliverable/<id>/rev-<n> is created OUTSIDE this package at the
// same point (the composition wires the git side, R20).
func (s *Store) SetSnapshotSHA(ctx context.Context, deliverableID string, n int, sha string) error {
	if sha == "" {
		return fmt.Errorf("%w: snapshot sha is empty", ErrBadInput)
	}
	rev, err := s.RevisionAt(ctx, deliverableID, n)
	if err != nil {
		return err
	}
	if rev.SnapshotSHA == sha {
		return nil // idempotent re-fill with the same sha
	}
	return s.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE deliverable_revisions SET snapshot_sha = ? WHERE deliverable_id = ? AND n = ? AND snapshot_sha IS NULL`,
			sha, deliverableID, n)
		if err != nil {
			return fmt.Errorf("review: set snapshot sha: %w", err)
		}
		if rows, err := res.RowsAffected(); err != nil {
			return err
		} else if rows != 1 {
			return fmt.Errorf("%w: %s rev %d has no unfilled snapshot slot", ErrBadInput, deliverableID, n)
		}
		return nil
	})
}

// SetVerdictRef fills the revision's verification-verdict ref (fill-once,
// NULL → verify.round event seq; the migration trigger enforces it).
func (s *Store) SetVerdictRef(ctx context.Context, deliverableID string, n int, eventSeq int64) error {
	return s.DB.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE deliverable_revisions SET verdict_ref = ? WHERE deliverable_id = ? AND n = ? AND verdict_ref IS NULL`,
			eventSeq, deliverableID, n)
		if err != nil {
			return fmt.Errorf("review: set verdict ref: %w", err)
		}
		if rows, err := res.RowsAffected(); err != nil {
			return err
		} else if rows != 1 {
			return fmt.Errorf("%w: %s rev %d has no unfilled verdict slot", ErrBadInput, deliverableID, n)
		}
		return nil
	})
}

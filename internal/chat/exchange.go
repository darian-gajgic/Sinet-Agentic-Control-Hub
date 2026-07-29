package chat

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// exchange.go — the S15.7 JARVIS file exchange (explicitly NOT parked at v0,
// S15.13): upload, list, delete, owner-attributed and fail-closed.
//
// THE MANIFEST IS THE RECORD; THE FILESYSTEM IS STORAGE (OQ4(a)). A directory
// listing cannot say WHO owns a byte string, so ownership, name, size and hash
// live in chat_exchange_files and every read starts there. That is what makes
// "fail-closed on ownership" enforceable: an object with no manifest row is not
// an orphan the platform shares, it is an object the platform cannot see.
//
// THE LAYOUT IS OPAQUE ON PURPOSE. Bytes land at
// <root>/<sha256(owner)[:32]>/<file-id>:
//
//   - The owner segment is a DIGEST rather than a sanitized user id, because a
//     sanitizer maps distinct identities onto one folder as soon as two ids
//     differ only in a character it strips — and two people sharing a directory
//     is precisely the cross-owner leak this family exists to prevent.
//   - The leaf is the generated file id, never the uploaded name. A hostile
//     filename therefore never reaches the filesystem at all: it is a LABEL in
//     the manifest and nothing else. The name is still validated (below),
//     because a name that can carry a path is a name that will eventually be
//     used as one by some future reader.
//
// AND THE PATH BUILDER STILL RESOLVES BEFORE IT TRUSTS (Spec S11.7). Opaque
// naming is a design property; symlink confinement is a check. `objectPath`
// resolves the root and the owner directory through EvalSymlinks and refuses
// anything that lands outside the root — so a symlink planted in the exchange
// home cannot make an upload write, or a delete unlink, a file elsewhere on
// the host. The preview checkoutFS shape (§25), applied to a write path.
//
// FILES REACH RUNS ONLY AS INTAKE INPUTS. The exchange is control-plane
// territory; no sandbox mounts it, wholesale or otherwise (S11.3 is
// allowlist-only). A handed file travels to a task as a requester-supplied
// input DESCRIPTOR through the S06 pipeline, never as a mounted directory.

const (
	// MaxFileBytes bounds one uploaded object. The exchange is where a person
	// hands the assistant a document to reason about or to attach to a task —
	// not a media store and not a backup target — and every byte crosses the
	// control plane's single writer connection on its way to a manifest row
	// (S02.1). Structural, not ⚙: S15.7 names no key and none exists.
	MaxFileBytes = 8 << 20
	// MaxFilesPerOwner bounds one person's exchange folder. Retention at v0 is
	// keep-until-deleted with NO sweeper (OQ4), so the only thing standing
	// between an unbounded folder and the disk is this number; a bound that
	// refuses loudly is better than a sweeper that deletes quietly.
	MaxFilesPerOwner = 200
	// MaxFileNameRunes bounds the stored label. It is a filename, not a
	// document.
	MaxFileNameRunes = 200
	// FileListCap bounds the list read. Same page-size reasoning as the rest of
	// this family; it is deliberately above MaxFilesPerOwner so the list can
	// always show the whole folder — a bound that hid the object you are at
	// your quota for would be an unhelpful truth.
	FileListCap = 256
)

// File is one exchange-manifest row.
type File struct {
	ID        string `json:"file_id"`
	Owner     string `json:"owner"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	// OriginSession/OriginTurn attribute the object to where it arrived. They
	// are what lets a late arrival say which turn caused it instead of being
	// guessed at (OQ7).
	OriginSession string `json:"origin_session,omitempty"`
	OriginTurn    string `json:"origin_turn,omitempty"`
	UploadedTS    string `json:"uploaded_ts"`
}

const fileColumns = `file_id, user_id, name, size_bytes, sha256,
	origin_session, origin_turn, uploaded_ts`

func scanFile(r interface{ Scan(...any) error }) (File, error) {
	var f File
	err := r.Scan(&f.ID, &f.Owner, &f.Name, &f.SizeBytes, &f.SHA256,
		&f.OriginSession, &f.OriginTurn, &f.UploadedTS)
	return f, err
}

// SafeName validates an uploaded filename as a LABEL. It rejects anything that
// could be read as a path — separators, the parent reference, a drive-ish or
// absolute shape — plus control characters, rather than silently rewriting
// them: a name quietly mangled into safety tells the person their file is
// called something it is not.
func SafeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("%w: a file needs a name", ErrInvalid)
	}
	if utf8.RuneCountInString(name) > MaxFileNameRunes {
		return "", fmt.Errorf("%w: filename exceeds %d characters", ErrInvalid, MaxFileNameRunes)
	}
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("%w: filename is not valid UTF-8", ErrInvalid)
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("%w: %q is a directory reference, not a filename", ErrInvalid, name)
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("%w: a filename may not contain a path", ErrInvalid)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%w: a filename may not contain control characters", ErrInvalid)
		}
	}
	return name, nil
}

// ownerDir is the confined per-owner directory. See the digest note above.
func (s *Store) ownerDir(owner string) string {
	sum := sha256.Sum256([]byte(owner))
	return filepath.Join(s.root, hex.EncodeToString(sum[:])[:32])
}

// objectPath resolves the on-disk path of one object, refusing anything that
// escapes the exchange root through a symlink (resolve-then-deny, S11.7). It
// resolves the OWNER DIRECTORY rather than the leaf, because on an upload the
// leaf does not exist yet and a check that only runs on reads is not a check.
//
// create says whether the owner directory may be made; a read path passes false
// so a missing directory is a miss rather than a side effect.
func (s *Store) objectPath(owner, fileID string, create bool) (string, error) {
	// The leaf is a generated id, never caller text. Validating it anyway is
	// what stops a future caller from passing something else and finding out
	// the hard way.
	if fileID == "" || strings.ContainsAny(fileID, `/\.`) {
		return "", fmt.Errorf("%w: bad object id", ErrInvalid)
	}
	dir := s.ownerDir(owner)
	if create {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", fmt.Errorf("chat: make exchange dir: %w", err)
		}
	}
	rootReal, err := filepath.EvalSymlinks(s.root)
	if err != nil {
		return "", fmt.Errorf("chat: resolve exchange root: %w", err)
	}
	dirReal, err := filepath.EvalSymlinks(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("chat: resolve exchange dir: %w", err)
	}
	if dirReal != rootReal && !strings.HasPrefix(dirReal, rootReal+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: the exchange path resolves outside the exchange root", ErrInvalid)
	}
	return filepath.Join(dirReal, fileID), nil
}

// UploadResult reports what an upload did. It is the same already-resolved
// vocabulary DeleteFileResult speaks: Applied is false when the object was
// already there.
type UploadResult struct {
	File File `json:"file"`
	// Applied is false on a repeat. An upload carrying the SAME owner, the same
	// name and the same sha256 is the same object arriving twice — a phone
	// retrying a request it never saw the answer to — and answering it with the
	// row that already exists is the S15.2 retry-safety rule. A second row would
	// consume a second quota slot and mint a second event for one act.
	Applied bool `json:"applied"`
}

// Upload stores one object and its manifest row. The bytes land first and the
// row second, so a failed insert leaves no manifest entry pointing at nothing;
// the orphaned bytes are then removed. The reverse order would leave a row
// claiming an object that was never written, which is the worse lie.
func (s *Store) Upload(ctx context.Context, owner, name string, data []byte, originSession, originTurn string) (UploadResult, error) {
	if owner == "" {
		return UploadResult{}, fmt.Errorf("%w: an upload needs an owner (15.6)", ErrInvalid)
	}
	safe, err := SafeName(name)
	if err != nil {
		return UploadResult{}, err
	}
	if len(data) == 0 {
		return UploadResult{}, fmt.Errorf("%w: the uploaded file is empty", ErrInvalid)
	}
	if len(data) > MaxFileBytes {
		return UploadResult{}, fmt.Errorf("%w: file is %d bytes, the bound is %d", ErrTooLarge, len(data), MaxFileBytes)
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	// Retry-safety BEFORE the quota, deliberately: a repeat is not a new object,
	// so it must not be refused by a bound it does not consume.
	if existing, ok, err := s.sameObject(ctx, owner, safe, digest); err != nil {
		return UploadResult{}, err
	} else if ok {
		return UploadResult{File: existing, Applied: false}, nil
	}
	var count int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM chat_exchange_files WHERE user_id = ?`, owner).Scan(&count); err != nil {
		return UploadResult{}, fmt.Errorf("chat: count exchange files: %w", err)
	}
	if count >= MaxFilesPerOwner {
		return UploadResult{}, fmt.Errorf("%w: the exchange folder holds %d files, the bound is %d", ErrTooMany, count, MaxFilesPerOwner)
	}
	// An origin ref is accepted only if the caller owns what it names —
	// otherwise the attribution would let one person label their upload as
	// having come out of someone else's turn.
	if originSession != "" {
		if _, err := s.Session(ctx, owner, originSession); err != nil {
			return UploadResult{}, err
		}
	}
	if originTurn != "" {
		if _, err := s.Turn(ctx, owner, originTurn); err != nil {
			return UploadResult{}, err
		}
	}

	f := File{ID: s.newID("cf-"), Owner: owner, Name: safe, SizeBytes: int64(len(data)),
		SHA256: digest, OriginSession: originSession,
		OriginTurn: originTurn, UploadedTS: s.stamp()}

	path, err := s.objectPath(owner, f.ID, true)
	if err != nil {
		return UploadResult{}, err
	}
	if err := writeNewFile(path, data); err != nil {
		return UploadResult{}, err
	}
	payload, err := json.Marshal(struct {
		File          string `json:"file"`
		Owner         string `json:"owner"`
		Name          string `json:"name"`
		SizeBytes     int64  `json:"size_bytes"`
		SHA256        string `json:"sha256"`
		OriginSession string `json:"origin_session,omitempty"`
		OriginTurn    string `json:"origin_turn,omitempty"`
	}{f.ID, owner, f.Name, f.SizeBytes, f.SHA256, f.OriginSession, f.OriginTurn})
	if err != nil {
		_ = os.Remove(path)
		return UploadResult{}, fmt.Errorf("chat: encode %s payload: %w", EventFileUploaded, err)
	}
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO chat_exchange_files
			   (file_id, user_id, name, size_bytes, sha256, origin_session, origin_turn, uploaded_ts)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			f.ID, owner, f.Name, f.SizeBytes, f.SHA256, f.OriginSession, f.OriginTurn, f.UploadedTS); err != nil {
			return fmt.Errorf("chat: insert exchange row: %w", err)
		}
		_, err := s.log.AppendTx(ctx, tx, eventlog.Append{
			UserID: owner, Type: EventFileUploaded,
			SchemaVersion: chatEventSchemaVersion, Payload: payload,
		})
		return err
	})
	if err != nil {
		_ = os.Remove(path)
		return UploadResult{}, err
	}
	return UploadResult{File: f, Applied: true}, nil
}

// writeNewFile writes the object bytes through O_EXCL. os.WriteFile FOLLOWS a
// symlink standing at the destination, so a link planted at the leaf would make
// the write land outside the exchange root even though objectPath resolved the
// directory (S11.7 resolve-then-deny, extended to the leaf). O_CREATE|O_EXCL is
// refused by the kernel on an existing path INCLUDING a dangling symlink, which
// is the one case a stat-then-write check cannot see.
func writeNewFile(path string, data []byte) error {
	fh, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("chat: create exchange object: %w", err)
	}
	if _, err := fh.Write(data); err != nil {
		fh.Close()
		_ = os.Remove(path)
		return fmt.Errorf("chat: write exchange object: %w", err)
	}
	if err := fh.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("chat: close exchange object: %w", err)
	}
	return nil
}

// sameObject finds the manifest row a repeated upload is a repeat OF: same
// owner, same label, same bytes. Newest first, so the answer is the row a later
// read would serve.
func (s *Store) sameObject(ctx context.Context, owner, name, digest string) (File, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+fileColumns+` FROM chat_exchange_files
		  WHERE user_id = ? AND name = ? AND sha256 = ? ORDER BY seq DESC LIMIT 1`,
		owner, name, digest)
	f, err := scanFile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, false, nil
	}
	if err != nil {
		return File{}, false, fmt.Errorf("chat: read exchange row: %w", err)
	}
	return f, true, nil
}

// ListFiles serves one person's exchange folder, newest first.
func (s *Store) ListFiles(ctx context.Context, viewer string) ([]File, error) {
	if viewer == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+fileColumns+` FROM chat_exchange_files
		  WHERE user_id = ? ORDER BY uploaded_ts DESC, file_id DESC LIMIT ?`, viewer, FileListCap)
	if err != nil {
		return nil, fmt.Errorf("chat: list exchange files: %w", err)
	}
	defer rows.Close()
	out := []File{}
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, fmt.Errorf("chat: scan exchange row: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FileRow reads one manifest row if the viewer owns it. Same one-answer rule as
// Session: not-yours and not-there are indistinguishable from outside.
func (s *Store) FileRow(ctx context.Context, viewer, id string) (File, error) {
	if viewer == "" || id == "" {
		return File{}, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+fileColumns+` FROM chat_exchange_files WHERE file_id = ? AND user_id = ?`, id, viewer)
	f, err := scanFile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, fmt.Errorf("chat: read exchange row: %w", err)
	}
	return f, nil
}

// DeleteFileResult reports what a delete removed.
type DeleteFileResult struct {
	File File `json:"file"`
	// Applied is false on a repeat: the object was already gone and the
	// already-resolved state is served (S15.2 retry-safety).
	Applied bool `json:"applied"`
}

// Delete removes one object and its manifest row. The ROW goes inside the
// transaction that mints the event; the bytes are unlinked afterwards, because
// a row that survived a failed unlink would keep serving a file the person
// deleted, whereas bytes that survive a committed delete are invisible garbage.
func (s *Store) Delete(ctx context.Context, viewer, id string) (DeleteFileResult, error) {
	f, err := s.FileRow(ctx, viewer, id)
	if errors.Is(err, ErrNotFound) {
		return DeleteFileResult{File: File{ID: id}, Applied: false}, nil
	}
	if err != nil {
		return DeleteFileResult{}, err
	}
	payload, err := json.Marshal(struct {
		File   string `json:"file"`
		Owner  string `json:"owner"`
		Name   string `json:"name"`
		SHA256 string `json:"sha256"`
	}{f.ID, f.Owner, f.Name, f.SHA256})
	if err != nil {
		return DeleteFileResult{}, fmt.Errorf("chat: encode %s payload: %w", EventFileDeleted, err)
	}
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM chat_exchange_files WHERE file_id = ? AND user_id = ?`, id, viewer); err != nil {
			return fmt.Errorf("chat: delete exchange row: %w", err)
		}
		_, err := s.log.AppendTx(ctx, tx, eventlog.Append{
			UserID: viewer, Type: EventFileDeleted,
			SchemaVersion: chatEventSchemaVersion, Payload: payload,
		})
		return err
	})
	if err != nil {
		return DeleteFileResult{}, err
	}
	if path, perr := s.objectPath(viewer, id, false); perr == nil {
		_ = os.Remove(path)
	}
	return DeleteFileResult{File: f, Applied: true}, nil
}

// rowQuerier is whatever the produced-files diff reads through — the store's
// pool for a plain read, or the settle transaction itself so the window closes
// and is measured atomically.
type rowQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// producedSince is the OQ7 window diff: manifest rows that appeared in this
// owner's folder AFTER the turn opened. A row already attributed to a DIFFERENT
// turn is excluded — it is that turn's product, not this one's — which is the
// whole of the "origin-attributed" rule on the window side.
//
// THE WINDOW IS A SEQUENCE COMPARISON, NOT A TIMESTAMP ONE. sinceSeq is the
// manifest watermark the turn recorded when it opened, and the predicate is
// STRICTLY greater, so a file that already existed can never be reported as
// produced by the turn. Timestamps cannot express that: *_ts is second-
// resolution RFC3339, so an upload landing in the same second as a turn's start
// is indistinguishable from one landing during it — and a chip claiming a turn
// produced a file it merely found is a fabricated attribution, which the
// honest-absence rule bars outright.
//
// AND WINDOWS CAN OVERLAP, SO AMBIGUITY ATTRIBUTES TO NOBODY. The one-turn rule
// is per SESSION, deliberately — a person with two conversations open can run a
// turn in each — so one owner's windows may overlap, and an upload during the
// overlap belongs to no turn in particular. OQ7 answers exactly this: the origin
// ref decides, and where none resolves it the file attributes to no turn. The
// NOT EXISTS below is that rule: a file with no origin ref is claimed only when
// no OTHER turn of this owner had a window containing it — a turn still running,
// or one whose closed window (exchange_seq, settled_exchange_seq] spans the row.
// Both overlapping turns therefore abstain, rather than both claiming the same
// file and printing the same chip twice under different authorship.
//
// The upper bound is the CALL SITE: this runs once, inside SettleTurn's own
// transaction, so every row that exists and is past the watermark arrived while
// the turn ran. There is no filesystem watcher and no ticker anywhere in this
// family (§32).
func (s *Store) producedSince(ctx context.Context, q rowQuerier, owner, turnID string, sinceSeq int64) ([]string, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT f.file_id FROM chat_exchange_files f
		  WHERE f.user_id = ? AND f.seq > ?
		    AND (f.origin_turn = ?
		         OR (f.origin_turn = '' AND NOT EXISTS (
		               SELECT 1 FROM chat_turns x
		                WHERE x.user_id = f.user_id AND x.turn_id <> ?
		                  AND x.exchange_seq < f.seq
		                  AND (x.state = 'running' OR f.seq <= x.settled_exchange_seq))))
		  ORDER BY f.seq LIMIT ?`,
		owner, sinceSeq, turnID, turnID, FileListCap)
	if err != nil {
		return nil, fmt.Errorf("chat: produced-files diff: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("chat: scan produced file: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// attributedTo is the OQ7 late-arrival half: objects that name this turn as
// their origin, whenever they arrived. A file produced after a turn settled
// still belongs to the turn that caused it, and the manifest is where it says
// so — the alternative is guessing from a timestamp, which is how a chip ends
// up on the wrong turn.
func (s *Store) attributedTo(ctx context.Context, owner, turnID string) ([]string, error) {
	if turnID == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT file_id FROM chat_exchange_files
		  WHERE user_id = ? AND origin_turn = ? ORDER BY uploaded_ts, file_id LIMIT ?`,
		owner, turnID, FileListCap)
	if err != nil {
		return nil, fmt.Errorf("chat: attributed-files read: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("chat: scan attributed file: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

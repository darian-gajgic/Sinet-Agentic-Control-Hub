package memory

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

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// Committer is the git seam of Spec S09.2: knowledge files are committed
// by the control plane on approval with the approver as author (D9
// attribution); the git MECHANICS are Spec S13's and land at B4. A nil
// committer leaves the commit pending — the row's file_commit stays NULL
// and fills once the S13 machinery runs (the migration trigger permits
// exactly that NULL→hash fill).
type Committer interface {
	Commit(ctx context.Context, dir string, paths []string, author, message string) (hash string, err error)
}

// ContradictionScreen is the S09.7 advisory contradiction screen — a
// local-model duty alias (Spec S12, B4). It is a NAMED SEAM at v0: nil
// means the advisory pass is absent and detection stays the deterministic
// same-topic_key lookup; the screen only ever refines the question text,
// never suppresses a deterministic hit (surfaced, never silently
// resolved). Its precision/recall is measured at
// TBD-BRINGUP(contradiction-screen P/R) [G3 Def.8].
type ContradictionScreen interface {
	Screen(ctx context.Context, a, b Entry) (contradicts bool, rationale string, err error)
}

// Gate is the S09.4 station-3 human gate — the ONLY write path into L2.
// Every verb takes the acting human's user id (resolved by an
// authenticated surface, Spec S01.9) and verifies it against the users
// table: no person row, no write (Spec S09.1 "humans only"). Engine-facing
// code never holds a Gate; a standing conformance test pins that
// (gate_conformance_test.go).
type Gate struct {
	s *Store
	// Committer is the S13 git seam (nil = commit pending, B4).
	Committer Committer
	// Screen is the S09.7 advisory contradiction screen (nil at v0, B4).
	Screen ContradictionScreen
}

// NewGate wraps the store's write side.
func NewGate(s *Store) *Gate { return &Gate{s: s} }

// Draft is one human-supplied entry body.
type Draft struct {
	Scope     Scope
	ScopeRef  string
	Kind      Kind
	Title     string
	Content   string
	TopicKey  string
	Selectors Selectors
	// FileBacked stores the content as a file in the scope's knowledge dir
	// (row keeps the file ref; Spec S09.2 row-is-index, file-is-content).
	FileBacked bool
	// FileName overrides the file basename (default <entry_id>.md).
	FileName string
}

// person resolves the acting human's row. The reserved ids and the dev
// fallback identity have no person row, so they are structurally refused
// here (Spec S01.9; S09.1 humans-only).
func (g *Gate) person(ctx context.Context, actor string) (role string, err error) {
	if actor == "" {
		return "", fmt.Errorf("%w: empty actor", ErrNotHuman)
	}
	err = g.s.db.QueryRowContext(ctx,
		`SELECT role FROM users WHERE user_id = ?`, actor).Scan(&role)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("%w: %q", ErrNotHuman, actor)
	}
	if err != nil {
		return "", fmt.Errorf("memory: resolve actor: %w", err)
	}
	return role, nil
}

// authorize applies the S09.4 station-3 approval ladder for a write into
// scope: owners approve own-scope entries, house promotion requires the
// operator (D10). Project-owner approval narrows to the project owner when
// the project registry lands (Spec S13/S1.6, B4); until then a project
// write is owner-attributed to the actor. Worker-overlay writes activate
// at v1 with the S08 overlay structure.
func (g *Gate) authorize(role string, scope Scope, scopeRef string) error {
	switch scope {
	case ScopeHouse:
		if role != "operator" {
			return ErrOperatorRequired
		}
		if scopeRef != "" {
			return fmt.Errorf("%w: house scope carries no scope_ref", ErrInvalidEntry)
		}
	case ScopeUser:
		if scopeRef != "" {
			return fmt.Errorf("%w: user scope carries no scope_ref", ErrInvalidEntry)
		}
	case ScopeProject:
		if scopeRef == "" {
			return fmt.Errorf("%w: project scope requires the project as scope_ref", ErrInvalidEntry)
		}
	case ScopeWorkerOverlay:
		return ErrScopeDormant
	default:
		return fmt.Errorf("%w: unknown scope %q", ErrInvalidEntry, scope)
	}
	return nil
}

func validateDraft(d Draft) error {
	switch d.Kind {
	case KindLesson, KindPreference, KindPlaybook, KindRubric, KindStyle,
		KindExemplar, KindConvention, KindTaxonomy, KindTriggerRules:
	case KindObservation:
		// Observations are L1; no L1 writer is active at v0 (Spec S09.4
		// activation table) — refusing the kind here IS the dormancy.
		return ErrLayerForbidden
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidEntry, d.Kind)
	}
	if strings.TrimSpace(d.Title) == "" {
		return fmt.Errorf("%w: title required", ErrInvalidEntry)
	}
	if strings.TrimSpace(d.Content) == "" {
		return fmt.Errorf("%w: content required", ErrInvalidEntry)
	}
	if d.FileName != "" {
		if d.FileName != filepath.Base(d.FileName) || d.FileName == "." || d.FileName == ".." {
			return fmt.Errorf("%w: file name must be a plain basename", ErrInvalidEntry)
		}
	}
	return nil
}

// scopeDir maps a scope to its knowledge directory (Spec S09.2:
// knowledge/house/, knowledge/users/<user>/, knowledge/projects/<project>/).
func (s *Store) scopeDir(scope Scope, scopeRef, owner string) (string, error) {
	switch scope {
	case ScopeHouse:
		return filepath.Join(s.root, "house"), nil
	case ScopeUser:
		return filepath.Join(s.root, "users", owner), nil
	case ScopeProject:
		return filepath.Join(s.root, "projects", scopeRef), nil
	}
	return "", fmt.Errorf("%w: scope %q has no knowledge dir at v0", ErrInvalidEntry, scope)
}

// writeOpts parameterizes the one write path.
type writeOpts struct {
	origin         Origin
	originRef      string
	supersedes     string
	adoptionSource string
	fixedID        string // stable governance ids (seeds); "" mints one
}

// CreateManual writes a human-direct L2 entry — the v0-live manual path of
// the Spec S09.4 activation table (workspace UI / operator surface).
func (g *Gate) CreateManual(ctx context.Context, actor string, d Draft) (Entry, error) {
	return g.writeEntry(ctx, actor, d, writeOpts{origin: OriginHumanDirect})
}

// Import writes an imported L2 entry (Spec S09.4: file import / the N20
// house-KB day-one import — the operator import IS the approval).
// originRef records the import provenance verbatim.
func (g *Gate) Import(ctx context.Context, actor string, d Draft, originRef string) (Entry, error) {
	return g.writeEntry(ctx, actor, d, writeOpts{origin: OriginImported, originRef: originRef})
}

// NewVersion supersedes an entry with a corrected body: a NEW version row
// carrying supersedes_id; the superseded version is retired, never edited
// or deleted (Spec S09.4 station 4, S09.8).
func (g *Gate) NewVersion(ctx context.Context, actor, supersedesID string, d Draft) (Entry, error) {
	return g.writeEntry(ctx, actor, d, writeOpts{origin: OriginHumanDirect, supersedes: supersedesID})
}

// Adopt copies another person's visible entry into the adopter's own user
// scope with the origin recorded — a copy, never a live shared reference,
// so later edits or poisoned updates cannot propagate (Spec S09.6).
func (g *Gate) Adopt(ctx context.Context, actor, sourceEntryID string) (Entry, error) {
	src, err := g.s.Get(ctx, sourceEntryID)
	if err != nil {
		return Entry{}, err
	}
	if src.Status != StatusActive || src.Tombstone {
		return Entry{}, fmt.Errorf("%w: adoption source %q", ErrNotActive, sourceEntryID)
	}
	content := src.Content
	if src.FilePath != "" {
		raw, err := os.ReadFile(filepath.Join(g.s.root, src.FilePath))
		if err != nil {
			return Entry{}, fmt.Errorf("memory: read adoption source file: %w", err)
		}
		content = string(raw)
	}
	originRef, err := json.Marshal(struct {
		Entry string `json:"entry"`
		Owner string `json:"owner"`
	}{Entry: src.ID, Owner: src.Owner})
	if err != nil {
		return Entry{}, fmt.Errorf("memory: marshal adoption origin: %w", err)
	}
	d := Draft{
		Scope: ScopeUser, Kind: src.Kind, Title: src.Title, Content: content,
		TopicKey: src.TopicKey, Selectors: src.Selectors,
	}
	return g.writeEntry(ctx, actor, d, writeOpts{
		origin: OriginAdoptedFrom, originRef: string(originRef), adoptionSource: src.ID,
	})
}

// writeEntry is the one write path behind every gate verb: authorization,
// draft validation, lifecycle stamping, file placement, the row, write-time
// conflict detection, the audit event — one transaction around the durable
// parts (Spec S09.4 station 4, S09.7).
func (g *Gate) writeEntry(ctx context.Context, actor string, d Draft, o writeOpts) (Entry, error) {
	origin, originRef, supersedes, adoptionSource := o.origin, o.originRef, o.supersedes, o.adoptionSource
	role, err := g.person(ctx, actor)
	if err != nil {
		return Entry{}, err
	}
	if err := g.authorize(role, d.Scope, d.ScopeRef); err != nil {
		return Entry{}, err
	}
	if err := validateDraft(d); err != nil {
		return Entry{}, err
	}
	numbers, err := g.s.Numbers()
	if err != nil {
		return Entry{}, err
	}

	entryID := o.fixedID
	if entryID == "" {
		entryID = newEntryID()
	}
	now := rfc3339(g.s.now())
	e := Entry{
		ID: entryID, Owner: actor, Scope: d.Scope, ScopeRef: d.ScopeRef,
		Layer: LayerL2, Kind: d.Kind, Title: d.Title, Content: d.Content,
		TopicKey: d.TopicKey, Selectors: d.Selectors, Status: StatusActive,
		Version: 1, Supersedes: supersedes,
		Origin: origin, OriginRef: originRef,
		ApprovedBy: actor, ApprovedTS: now,
		// The approval is the verification act at write time; recorded so
		// station-5 intervals apply retroactively at v1 activation
		// (Spec S09.4).
		VerifiedBy: actor, VerifiedTS: now,
		ReverifyIntervalDays: reverifyIntervalDays(d.Scope, d.Kind, numbers),
		CreatedTS:            now, UpdatedTS: now,
	}

	if supersedes != "" {
		old, err := g.s.Get(ctx, supersedes)
		if err != nil {
			return Entry{}, err
		}
		if old.Tombstone {
			return Entry{}, fmt.Errorf("%w: cannot supersede a tombstone", ErrInvalidEntry)
		}
		if old.Owner != actor && role != "operator" {
			return Entry{}, fmt.Errorf("%w: entry %q", ErrNotOwner, supersedes)
		}
		if old.Scope != d.Scope || old.ScopeRef != d.ScopeRef {
			return Entry{}, fmt.Errorf("%w: a new version keeps its scope (Spec S09.4)", ErrInvalidEntry)
		}
		e.Version = old.Version + 1
		e.Owner = old.Owner
	}

	// File placement precedes the transaction (a crash leaves an orphan
	// file, never a row without content).
	var absFile string
	if d.FileBacked {
		dir, err := g.s.scopeDir(e.Scope, e.ScopeRef, e.Owner)
		if err != nil {
			return Entry{}, err
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Entry{}, fmt.Errorf("memory: knowledge dir: %w", err)
		}
		name := d.FileName
		if name == "" {
			name = e.ID + ".md"
		}
		absFile = filepath.Join(dir, name)
		if err := os.WriteFile(absFile, []byte(d.Content), 0o600); err != nil {
			return Entry{}, fmt.Errorf("memory: write knowledge file: %w", err)
		}
		rel, err := filepath.Rel(g.s.root, absFile)
		if err != nil {
			return Entry{}, fmt.Errorf("memory: knowledge file path: %w", err)
		}
		e.FilePath = rel
		e.Content = ""
	}

	selectors, err := marshalSelectors(e.Selectors)
	if err != nil {
		return Entry{}, err
	}
	conflicts, err := g.detectConflicts(ctx, e, d.Content)
	if err != nil {
		return Entry{}, err
	}

	err = g.s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO knowledge_entries (
				entry_id, user_id, scope, scope_ref, layer, kind, title,
				content, file_path, topic_key, selectors, status, version,
				supersedes_id, origin, origin_ref, proposer_model,
				approved_by, approved_ts, verified_by, verified_ts,
				reverify_interval_days, created_ts, updated_ts)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.ID, e.Owner, e.Scope, e.ScopeRef, e.Layer, e.Kind, e.Title,
			nullable(e.Content), nullable(e.FilePath), nullable(e.TopicKey), selectors,
			e.Status, e.Version, nullable(e.Supersedes), e.Origin, e.OriginRef,
			e.ProposerModel, e.ApprovedBy, e.ApprovedTS, e.VerifiedBy, e.VerifiedTS,
			nullableInt(e.ReverifyIntervalDays), e.CreatedTS, e.UpdatedTS); err != nil {
			return fmt.Errorf("memory: insert entry: %w", err)
		}
		if supersedes != "" {
			if _, err := tx.ExecContext(ctx, `
				UPDATE knowledge_entries SET status = 'retired', updated_ts = ?
				 WHERE entry_id = ? AND status IN ('active', 'proposed')`,
				now, supersedes); err != nil {
				return fmt.Errorf("memory: retire superseded version: %w", err)
			}
		}
		if adoptionSource != "" {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO knowledge_adoptions (user_id, source_entry_id, adopted_entry_id, adopted_ts)
				VALUES (?, ?, ?, ?)`,
				actor, adoptionSource, e.ID, now); err != nil {
				return fmt.Errorf("memory: record adoption: %w", err)
			}
		}
		for _, c := range conflicts {
			if err := insertConflict(ctx, tx, g.s.log, c, now); err != nil {
				return err
			}
		}
		payload, err := json.Marshal(struct {
			EntryID     string `json:"entry_id"`
			Scope       Scope  `json:"scope"`
			ScopeRef    string `json:"scope_ref,omitempty"`
			Kind        Kind   `json:"kind"`
			Origin      Origin `json:"origin"`
			Version     int64  `json:"version"`
			Supersedes  string `json:"supersedes,omitempty"`
			ContentHash string `json:"content_hash"`
			FilePath    string `json:"file_path,omitempty"`
		}{e.ID, e.Scope, e.ScopeRef, e.Kind, e.Origin, e.Version, e.Supersedes,
			contentHash(d.Content), e.FilePath})
		if err != nil {
			return fmt.Errorf("memory: marshal write event: %w", err)
		}
		_, err = g.s.log.AppendTx(ctx, tx, eventAppend(actor, EventWrite, payload))
		return err
	})
	if err != nil {
		if absFile != "" {
			_ = os.Remove(absFile)
		}
		return Entry{}, err
	}

	// Commit-on-approval with the approver as author (Spec S09.2); the git
	// mechanics are the S13 seam — nil leaves file_commit pending (B4).
	if d.FileBacked && g.Committer != nil {
		dir, _ := g.s.scopeDir(e.Scope, e.ScopeRef, e.Owner)
		hash, err := g.Committer.Commit(ctx, dir, []string{absFile}, actor,
			fmt.Sprintf("knowledge: %s %s v%d (%s)", e.Kind, e.ID, e.Version, e.Origin))
		if err != nil {
			return Entry{}, fmt.Errorf("memory: commit knowledge file: %w", err)
		}
		err = g.s.db.WriteTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				`UPDATE knowledge_entries SET file_commit = ? WHERE entry_id = ? AND file_commit IS NULL`,
				hash, e.ID)
			return err
		})
		if err != nil {
			return Entry{}, fmt.Errorf("memory: record file commit: %w", err)
		}
		e.FileCommit = hash
	}
	return e, nil
}

// Remove sets an entry to removed: it leaves every future assembly
// immediately (deterministic selection makes removal exact, Spec S09.5);
// content stays in git history and the row stays for audit. Returns the
// influence list the platform surfaces (runs the entry was injected into).
func (g *Gate) Remove(ctx context.Context, actor, entryID, reason string) ([]InfluenceRef, error) {
	role, err := g.person(ctx, actor)
	if err != nil {
		return nil, err
	}
	e, err := g.s.Get(ctx, entryID)
	if err != nil {
		return nil, err
	}
	if e.Tombstone {
		return nil, fmt.Errorf("%w: entry %q is a tombstone", ErrNotActive, entryID)
	}
	if e.Owner != actor && role != "operator" {
		return nil, fmt.Errorf("%w: entry %q", ErrNotOwner, entryID)
	}
	now := rfc3339(g.s.now())
	err = g.s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE knowledge_entries SET status = 'removed', updated_ts = ?
			 WHERE entry_id = ? AND status <> 'removed'`, now, entryID)
		if err != nil {
			return fmt.Errorf("memory: remove entry: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: entry %q", ErrNotActive, entryID)
		}
		payload, err := json.Marshal(struct {
			EntryID string `json:"entry_id"`
			Reason  string `json:"reason"`
		}{entryID, reason})
		if err != nil {
			return fmt.Errorf("memory: marshal remove event: %w", err)
		}
		_, err = g.s.log.AppendTx(ctx, tx, eventAppend(actor, EventRemove, payload))
		return err
	})
	if err != nil {
		return nil, err
	}
	return g.s.Influence(ctx, entryID)
}

// TrueDelete purges an entry's content and leaves the tombstone row — id,
// dates, "removed at owner request" (G2 Def.9; S4.1 owner right, owner
// only). The file purge covers the working tree; git-history purge is a
// Spec S13 operation, and the DB-file purge completes at the next VACUUM
// INTO snapshot (Spec S02.1 path).
func (g *Gate) TrueDelete(ctx context.Context, actor, entryID string) error {
	if _, err := g.person(ctx, actor); err != nil {
		return err
	}
	e, err := g.s.Get(ctx, entryID)
	if err != nil {
		return err
	}
	if e.Tombstone {
		return nil // idempotent: already a tombstone
	}
	if e.Owner != actor {
		return fmt.Errorf("%w: true deletion is the owner's right (S4.1)", ErrNotOwner)
	}
	if e.FilePath != "" {
		if err := os.Remove(filepath.Join(g.s.root, e.FilePath)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("memory: purge knowledge file: %w", err)
		}
	}
	now := rfc3339(g.s.now())
	return g.s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			UPDATE knowledge_entries SET
				status = 'removed', tombstone = 1,
				tombstone_note = 'removed at owner request',
				title = '', content = NULL, file_path = NULL, file_commit = NULL,
				topic_key = NULL, selectors = '{}', origin_ref = '',
				proposer_model = '', updated_ts = ?
			 WHERE entry_id = ?`, now, entryID); err != nil {
			return fmt.Errorf("memory: purge entry: %w", err)
		}
		payload, err := json.Marshal(struct {
			EntryID string `json:"entry_id"`
			Note    string `json:"note"`
		}{entryID, "removed at owner request"})
		if err != nil {
			return fmt.Errorf("memory: marshal delete event: %w", err)
		}
		_, err = g.s.log.AppendTx(ctx, tx, eventAppend(actor, EventDelete, payload))
		return err
	})
}

// ResolveConflict closes a conflict question (the human answer to the
// S09.7 card; how each side is corrected is a further gate write).
func (g *Gate) ResolveConflict(ctx context.Context, actor string, conflictID int64) error {
	if _, err := g.person(ctx, actor); err != nil {
		return err
	}
	now := rfc3339(g.s.now())
	return g.s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE knowledge_conflicts SET status = 'resolved', resolved_by = ?, resolved_ts = ?
			 WHERE conflict_id = ? AND status = 'open'`, actor, now, conflictID)
		if err != nil {
			return fmt.Errorf("memory: resolve conflict: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: open conflict %d", ErrNotFound, conflictID)
		}
		return nil
	})
}

// conflictHit is one detected write-time conflict (Spec S09.7).
type conflictHit struct {
	entryID      string // the new entry
	otherEntryID string
	affected     string // the other entry's owner — the question's addressee
	topicKey     string
	question     string
}

// detectConflicts runs the S09.7 write-time duty: same-topic_key lookup
// across the scopes the writer can see, plus the advisory screen when
// present. Hits become conflicts_with edges and question cards to the
// affected owner — surfaced, never silently resolved.
func (g *Gate) detectConflicts(ctx context.Context, e Entry, content string) ([]conflictHit, error) {
	if e.TopicKey == "" {
		return nil, nil
	}
	rows, err := g.s.db.QueryContext(ctx, `
		SELECT `+entryColumns+` FROM knowledge_entries
		 WHERE topic_key = ? AND status = 'active' AND tombstone = 0
		   AND entry_id <> ? AND (? = '' OR entry_id <> ?)
		   AND (scope = 'house'
		        OR (scope = 'user' AND user_id = ?)
		        OR (scope = 'project' AND user_id = ?))
		 ORDER BY entry_id`,
		e.TopicKey, e.ID, e.Supersedes, e.Supersedes, e.Owner, e.Owner)
	if err != nil {
		return nil, fmt.Errorf("memory: conflict lookup: %w", err)
	}
	defer rows.Close()
	var hits []conflictHit
	for rows.Next() {
		other, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: scan conflict candidate: %w", err)
		}
		question := fmt.Sprintf(
			"New %s entry %q (v%d) shares topic %q with your entry %q — do they conflict? Correct, retire, or keep both.",
			e.Kind, e.ID, e.Version, e.TopicKey, other.ID)
		if g.Screen != nil {
			probe := e
			probe.Content = content
			if contradicts, rationale, err := g.Screen.Screen(ctx, probe, other); err == nil && contradicts && rationale != "" {
				question += " Advisory screen: " + rationale
			}
			// A screen error degrades to the deterministic hit alone — the
			// duty is advisory (Spec S09.7; the §14 absent-seam precedent).
		}
		hits = append(hits, conflictHit{
			entryID: e.ID, otherEntryID: other.ID, affected: other.Owner,
			topicKey: e.TopicKey, question: question,
		})
	}
	return hits, rows.Err()
}

// insertConflict writes one edge + its question and the audit event,
// deduplicating open edges per unordered pair.
func insertConflict(ctx context.Context, tx *sql.Tx, log logAppender, c conflictHit, now string) error {
	var n int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM knowledge_conflicts
		 WHERE status = 'open'
		   AND ((entry_id = ? AND other_entry_id = ?) OR (entry_id = ? AND other_entry_id = ?))`,
		c.entryID, c.otherEntryID, c.otherEntryID, c.entryID).Scan(&n); err != nil {
		return fmt.Errorf("memory: conflict dedup: %w", err)
	}
	if n > 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO knowledge_conflicts (user_id, entry_id, other_entry_id, topic_key, question, status, detected_ts)
		VALUES (?, ?, ?, ?, ?, 'open', ?)`,
		c.affected, c.entryID, c.otherEntryID, c.topicKey, c.question, now); err != nil {
		return fmt.Errorf("memory: insert conflict: %w", err)
	}
	payload, err := json.Marshal(struct {
		EntryID      string `json:"entry_id"`
		OtherEntryID string `json:"other_entry_id"`
		TopicKey     string `json:"topic_key"`
		Affected     string `json:"affected_owner"`
	}{c.entryID, c.otherEntryID, c.topicKey, c.affected})
	if err != nil {
		return fmt.Errorf("memory: marshal conflict event: %w", err)
	}
	_, err = log.AppendTx(ctx, tx, eventAppend(c.affected, EventConflict, payload))
	return err
}

// logAppender is the eventlog surface conflict writes need.
type logAppender interface {
	AppendTx(ctx context.Context, tx *sql.Tx, a eventlog.Append) (int64, error)
}

// eventAppend builds a platform-scope knowledge event attributed to its
// acting/affected person (15.6; run_id NULL per Spec S02.2).
func eventAppend(userID, typ string, payload []byte) eventlog.Append {
	return eventlog.Append{
		UserID: userID, Type: typ,
		SchemaVersion: knowledgeEventSchemaVersion, Payload: payload,
	}
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

// reverifyIntervalDays stamps the S09.8 lifecycle row for a new entry:
// lessons re-verify at ⚙ memory.reverify_lessons_days; house-scope
// behavior-steering objects at ⚙ memory.reverify_house_days; preferences
// carry no interval unless a human flags one.
func reverifyIntervalDays(scope Scope, kind Kind, n Numbers) int64 {
	if kind == KindPreference {
		return 0
	}
	if kind == KindLesson {
		return n.ReverifyLessonsDays
	}
	if scope == ScopeHouse {
		return n.ReverifyHouseDays
	}
	return 0
}

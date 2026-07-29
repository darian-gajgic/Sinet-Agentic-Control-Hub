package memory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// browse.go — the READ side a human-facing browse surface needs (Spec S09.2
// scopes; S09.5 provenance; S09.7 conflict edges). Everything here is a read:
// the S09.1 capability split is untouched, every write verb still lives on
// *Gate, and nothing in this file is reachable from engine-facing code (the
// §17 import wall bars those packages from the package entirely).
//
// Why these reads exist: ListOwned answers "everything stored about ME"
// (S4.1), and that is the only listing the store had. A person's VISIBLE set
// is larger — house defaults address everyone (S4.5) and project knowledge is
// shared among that project's members by scope membership (S09.6) — and it is
// the same three-scope predicate the server-side knowledge_search already
// applies (S09.3). It belongs here, in the package that owns what a scope
// means, rather than being re-derived by a transport.

// entryConflictsCap bounds one entry's served conflict edges. An entry has a
// handful (open edges are deduplicated per unordered pair at write time), but
// expectation is not a bound and every read here shares the control plane's one
// writer connection (Spec S02.1) — the §38 precedent.
const entryConflictsCap = 200

// BrowseQuery bounds one visible-set read. Viewer and Limit are required;
// Projects is the set of registered projects the viewer owns or is an invited
// member of (Spec S13.7 registry membership) — the caller resolves it, because
// the project registry is not this package's table. The three filters are
// optional and each narrows by an exact stored value.
type BrowseQuery struct {
	Viewer   string
	Projects []string
	Scope    Scope
	Kind     Kind
	Status   Status
	Limit    int
}

// visiblePredicate builds THE scope predicate of this section — own rows in
// every scope owned (S4.1), house rows (house defaults address everyone, S4.5),
// and project rows of the projects the viewer belongs to (S09.6) — as one SQL
// fragment with its arguments.
//
// It exists so the set read and the single-entry read cannot disagree. Two
// hand-maintained copies of one rule are two rules the day one of them is
// edited, and the copy that drifts is the one that decides whether a person
// sees another person's memory. The operator role bit is not a parameter: house
// authority (D10) is the right to CURATE the house scope, not a read into
// somebody's personal store, so there is no operator limb here to forget.
func visiblePredicate(viewer string, projects []string) (string, []any) {
	sql := `(user_id = ? OR scope = 'house'`
	args := []any{viewer}
	if len(projects) > 0 {
		sql += ` OR (scope = 'project' AND scope_ref IN (` +
			strings.TrimSuffix(strings.Repeat("?,", len(projects)), ",") + `))`
		for _, p := range projects {
			args = append(args, p)
		}
	}
	return sql + `)`, args
}

// ListVisible lists the entries a person may see, newest first, under the one
// predicate above. Tombstones and removed rows are INCLUDED — the stub is an
// audit surface (S09.5) and the caller narrows with Status when it wants the
// live set.
func (s *Store) ListVisible(ctx context.Context, q BrowseQuery) ([]Entry, error) {
	if q.Viewer == "" {
		return nil, fmt.Errorf("%w: a visible-set read needs its viewer (Spec S09.3 server-side scoping)", ErrInvalidEntry)
	}
	if q.Limit <= 0 {
		return nil, fmt.Errorf("%w: a visible-set read needs a positive bound", ErrInvalidEntry)
	}
	visible, args := visiblePredicate(q.Viewer, q.Projects)

	query := `SELECT ` + entryColumns + ` FROM knowledge_entries WHERE ` + visible
	for _, f := range []struct {
		clause string
		val    string
	}{
		{` AND scope = ?`, string(q.Scope)},
		{` AND kind = ?`, string(q.Kind)},
		{` AND status = ?`, string(q.Status)},
	} {
		if f.val != "" {
			query += f.clause
			args = append(args, f.val)
		}
	}
	query += ` ORDER BY created_ts DESC, entry_id DESC LIMIT ?`
	args = append(args, q.Limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: list visible: %w", err)
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: scan entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Visible answers the same question for ONE entry, through the same predicate:
// may this viewer see it? An unknown id is not visible — a caller that needs to
// tell "gone" from "not yours" reads the entry itself and gets ErrNotFound,
// which is what keeps 404-before-403 the caller's decision rather than this
// read's.
func (s *Store) Visible(ctx context.Context, entryID, viewer string, projects []string) (bool, error) {
	if viewer == "" {
		return false, fmt.Errorf("%w: a visibility check needs its viewer (Spec S09.3 server-side scoping)", ErrInvalidEntry)
	}
	visible, args := visiblePredicate(viewer, projects)
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_entries WHERE entry_id = ? AND `+visible,
		append([]any{entryID}, args...)...).Scan(&n); err != nil {
		return false, fmt.Errorf("memory: visibility of %q: %w", entryID, err)
	}
	return n > 0, nil
}

// Conflict is one knowledge_conflicts edge — the durable home of the S09.7
// question card. Affected is the owner the question is addressed to.
type Conflict struct {
	ID           int64  `json:"conflict_id"`
	Affected     string `json:"affected_owner"`
	EntryID      string `json:"entry_id"`
	OtherEntryID string `json:"other_entry_id"`
	TopicKey     string `json:"topic_key,omitempty"`
	Question     string `json:"question"`
	Status       string `json:"status"`
	DetectedTS   string `json:"detected_ts"`
	ResolvedBy   string `json:"resolved_by,omitempty"`
	ResolvedTS   string `json:"resolved_ts,omitempty"`
}

const conflictColumns = `conflict_id, user_id, entry_id, other_entry_id, topic_key,
	question, status, detected_ts, coalesce(resolved_by, ''), coalesce(resolved_ts, '')`

func scanConflict(r rowScanner) (Conflict, error) {
	var c Conflict
	err := r.Scan(&c.ID, &c.Affected, &c.EntryID, &c.OtherEntryID, &c.TopicKey,
		&c.Question, &c.Status, &c.DetectedTS, &c.ResolvedBy, &c.ResolvedTS)
	return c, err
}

// EntryConflicts returns the OPEN edges naming an entry on either side, oldest
// first. Detection is symmetric (a hit records one edge for the pair), so both
// entries of a conflicting pair surface the same question — which is what
// "surfaced, never silently resolved" means on a resource that shows one side.
func (s *Store) EntryConflicts(ctx context.Context, entryID string) ([]Conflict, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+conflictColumns+` FROM knowledge_conflicts
		 WHERE status = 'open' AND (entry_id = ? OR other_entry_id = ?)
		 ORDER BY conflict_id LIMIT ?`, entryID, entryID, entryConflictsCap)
	if err != nil {
		return nil, fmt.Errorf("memory: read conflicts for %q: %w", entryID, err)
	}
	defer rows.Close()
	var out []Conflict
	for rows.Next() {
		c, err := scanConflict(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: scan conflict: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// OpenConflictsFor returns the OPEN edges addressed to one person, oldest
// first — the read a decision surface makes to answer "what is waiting on me?"
// (Spec S15.6; S09.7).
//
// It is scoped by the ADDRESSEE and by nothing else, deliberately: `affected` is
// the owner the question was put to, and mayAnswerConflict — the one expression
// of both visibility and authority on this edge — is that same equality. So a
// caller can only ever be handed the conflicts it could also resolve, and a
// surface built on this read cannot show a card whose door would refuse it.
// EntryConflicts answers a different question (the edges ON one entry, for the
// entry's own page) and is not a substitute for either half of this one.
func (s *Store) OpenConflictsFor(ctx context.Context, affected string) ([]Conflict, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+conflictColumns+` FROM knowledge_conflicts
		 WHERE status = 'open' AND user_id = ?
		 ORDER BY conflict_id LIMIT ?`, affected, entryConflictsCap)
	if err != nil {
		return nil, fmt.Errorf("memory: read open conflicts for %q: %w", affected, err)
	}
	defer rows.Close()
	var out []Conflict
	for rows.Next() {
		c, err := scanConflict(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: scan conflict: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Conflict returns one edge by id — the read a surface makes before routing a
// resolution, so the answer reaches the person the question was addressed to.
func (s *Store) Conflict(ctx context.Context, conflictID int64) (Conflict, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+conflictColumns+` FROM knowledge_conflicts WHERE conflict_id = ?`, conflictID)
	c, err := scanConflict(row)
	if err == sql.ErrNoRows {
		return Conflict{}, fmt.Errorf("%w: conflict %d", ErrNotFound, conflictID)
	}
	if err != nil {
		return Conflict{}, fmt.Errorf("memory: read conflict %d: %w", conflictID, err)
	}
	return c, nil
}

package worker

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// search.go — the S08.8 selection index over delegation descriptions
// (migration 0006): FTS5 rows derived from the ACTIVE version's template
// file, refreshed inside the same transaction as every active_version move
// (Approve / Repoint — Spec S08.4: active_version is the only mutable
// pointer, and those are its only writers). Status is never trusted to the
// index: the selection query joins worker_templates and filters
// status = 'active' from the authoritative row (Spec S08.8 step 1).

// refreshSearchTx replaces the template's worker_search row from an
// already-parsed active definition. Callers hold the definition because
// every calling verb has just hash-verified and parsed the file.
func refreshSearchTx(ctx context.Context, tx *sql.Tx, templateID string, def Definition) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM worker_search WHERE template_id = ?`, templateID); err != nil {
		return fmt.Errorf("worker: clear search row: %w", err)
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO worker_search (name, description, triggers, template_id) VALUES (?, ?, ?, ?)`,
		def.Name, def.Description, strings.Join(def.Selectors.Triggers, " "), templateID)
	if err != nil {
		return fmt.Errorf("worker: write search row: %w", err)
	}
	return nil
}

// activeDefinition loads and parses the hash-verified file of a template's
// active version (the S08.3 tamper check runs on every read).
func (s *Store) activeDefinition(ctx context.Context, t Template) (Version, Definition, error) {
	if t.ActiveVersion == "" {
		return Version{}, Definition{}, fmt.Errorf("%w: template %q has no active version", ErrNotActive, t.ID)
	}
	v, err := s.VersionByID(ctx, t.ActiveVersion)
	if err != nil {
		return Version{}, Definition{}, err
	}
	src, err := s.readTemplateFile(v)
	if err != nil {
		return Version{}, Definition{}, err
	}
	def, findings, err := ParseTemplate(src)
	if err != nil {
		return Version{}, Definition{}, err
	}
	for _, f := range findings {
		if f.Code == FindingGuardrail {
			return Version{}, Definition{}, fmt.Errorf("%w: %s", ErrGuardrailField, f.Message)
		}
	}
	return v, def, nil
}

// ftsRank is one FTS candidate row: template id plus its bm25 rank
// (smaller = better in SQLite's convention; the router folds it into the
// deterministic score).
type ftsRank struct {
	TemplateID string
	Rank       float64
}

// searchDescriptions runs the S08.8 FTS5 leg over delegation descriptions.
// Free text is sanitized into quoted OR-joined terms — the §17 memory
// precedent (operators and column filters cannot ride a query), OR-joined
// because any description-term hit contributes rank (an AND-join over task
// prose would demand the whole request verbatim). Zero terms → no FTS leg.
func (s *Store) searchDescriptions(ctx context.Context, text string) ([]ftsRank, error) {
	q := ftsOrQuery(text)
	if q == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT template_id, rank FROM worker_search WHERE worker_search MATCH ? ORDER BY rank, template_id`, q)
	if err != nil {
		return nil, fmt.Errorf("worker: search descriptions: %w", err)
	}
	defer rows.Close()
	var out []ftsRank
	for rows.Next() {
		var r ftsRank
		if err := rows.Scan(&r.TemplateID, &r.Rank); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ftsMaxQueryTerms bounds the OR-query (deterministic, first-N tokens of
// the task text; plain constant — S18 declares no such key).
const ftsMaxQueryTerms = 32

func ftsOrQuery(text string) string {
	fields := strings.Fields(text)
	if len(fields) > ftsMaxQueryTerms {
		fields = fields[:ftsMaxQueryTerms]
	}
	quoted := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, `.,;:!?()[]{}<>'"`)
		if f == "" {
			continue
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(f, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " OR ")
}

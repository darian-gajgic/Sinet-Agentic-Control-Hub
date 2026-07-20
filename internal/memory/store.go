package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/storage"
)

// Settings is this package's narrow view of the settings registry
// (CONVENTIONS §2/§6); *settings.Registry satisfies it.
type Settings interface {
	Int(key string) (int64, error)
	Float(key string) (float64, error)
}

// Dotted ⚙ keys consumed by this package (owned by Spec S09; declared in
// the S18 registry since B0-2 — consumed here, never redeclared).
const (
	keyL1TTLDays          = "memory.l1_ttl_days"
	keyReverifyLessons    = "memory.reverify_lessons_days"
	keyReverifyHouse      = "memory.reverify_house_days"
	keyProposalsPerTask   = "memory.proposals_per_task_max"
	keyDigestInterval     = "memory.digest_interval_days"
	keyDistillThreshold   = "memory.distill_threshold_lessons"
	keyBudgetHouse        = "memory.injection_budget_tokens.house"
	keyBudgetProject      = "memory.injection_budget_tokens.project"
	keyBudgetUser         = "memory.injection_budget_tokens.user"
	keyBudgetOverlay      = "memory.injection_budget_tokens.worker_overlay"
	keyVectorGateMissRate = "memory.vector_gate.task_miss_rate"
	keyVectorGateCorpus   = "memory.vector_gate.corpus_entries"
)

// Event types minted by this package — platform-scope except search_miss
// (run-scoped). Names provisional pending the S14 event contract (B5),
// extending the CONVENTIONS §7/§8/§13–§15 naming note.
const (
	EventWrite      = "knowledge.write"
	EventRemove     = "knowledge.remove"
	EventDelete     = "knowledge.delete"
	EventConflict   = "knowledge.conflict"
	EventCuration   = "knowledge.curation"
	EventSearchMiss = "knowledge.search_miss"
)

// knowledgeEventSchemaVersion versions this package's event payloads.
const knowledgeEventSchemaVersion = 1

// queryExcerptCap bounds the query text recorded on a search-miss event
// (refs-not-blobs, P-T07-5; the adapters.ExcerptCap precedent — a plain
// structural constant, no ⚙ key is ratified).
const queryExcerptCap = 4096

// Store is the read side of the knowledge store: lookups, owner-rights
// listings, and the server-scoped knowledge_search. It carries NO write
// verb on knowledge tables — writes live on *Gate (capability withholding,
// Spec S09.1). Engine-facing code holds a *Store only.
type Store struct {
	db       *storage.DB
	log      *eventlog.Log
	settings Settings
	// root is the knowledge-dir root: <root>/house, <root>/users/<user>,
	// <root>/projects/<project> — the per-scope git-versioned directories
	// of Spec S09.2.
	root string

	// Now is the test clock seam.
	Now func() time.Time
}

// NewStore opens the knowledge store over platform.db and the knowledge
// directory root.
func NewStore(db *storage.DB, log *eventlog.Log, settings Settings, root string) (*Store, error) {
	if db == nil || log == nil || settings == nil {
		return nil, fmt.Errorf("memory: store needs db, event log and settings")
	}
	if root == "" {
		return nil, fmt.Errorf("memory: store needs a knowledge dir root (Spec S09.2)")
	}
	return &Store{db: db, log: log, settings: settings, root: root}, nil
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Root returns the knowledge-dir root.
func (s *Store) Root() string { return s.root }

// Numbers is the operative ⚙ snapshot of Spec S09's settings table, read
// by dotted key through the registry at call time (never cached, never
// hardcoded). Station-5/vector-gate machinery is dormant at v0; the
// numbers are live so activation is a config flip (Spec S09.4).
type Numbers struct {
	L1TTLDays               int64
	ReverifyLessonsDays     int64
	ReverifyHouseDays       int64
	ProposalsPerTaskMax     int64
	DigestIntervalDays      int64
	DistillThresholdLessons int64
	VectorGateTaskMissRate  float64
	VectorGateCorpusEntries int64
}

// Numbers reads the lifecycle/pipeline ⚙ set (the injection budgets are
// read per-assembly in the Knowledge source).
func (s *Store) Numbers() (Numbers, error) {
	var n Numbers
	var err error
	read := func(dst *int64, key string) {
		if err != nil {
			return
		}
		var v int64
		if v, err = s.settings.Int(key); err != nil {
			err = fmt.Errorf("memory: read ⚙ %s: %w", key, err)
			return
		}
		*dst = v
	}
	read(&n.L1TTLDays, keyL1TTLDays)
	read(&n.ReverifyLessonsDays, keyReverifyLessons)
	read(&n.ReverifyHouseDays, keyReverifyHouse)
	read(&n.ProposalsPerTaskMax, keyProposalsPerTask)
	read(&n.DigestIntervalDays, keyDigestInterval)
	read(&n.DistillThresholdLessons, keyDistillThreshold)
	read(&n.VectorGateCorpusEntries, keyVectorGateCorpus)
	if err != nil {
		return Numbers{}, err
	}
	rate, ferr := s.settings.Float(keyVectorGateMissRate)
	if ferr != nil {
		return Numbers{}, fmt.Errorf("memory: read ⚙ %s: %w", keyVectorGateMissRate, ferr)
	}
	n.VectorGateTaskMissRate = rate
	return n, nil
}

const entryColumns = `entry_id, user_id, scope, scope_ref, layer, kind, title,
	coalesce(content, ''), coalesce(file_path, ''), coalesce(file_commit, ''),
	coalesce(topic_key, ''), selectors, status, version, coalesce(supersedes_id, ''),
	origin, origin_ref, proposer_model, approved_by, coalesce(approved_ts, ''),
	verified_by, coalesce(verified_ts, ''), coalesce(reverify_interval_days, 0),
	coalesce(expires_ts, ''), coalesce(last_injected_ts, ''),
	tombstone, tombstone_note, created_ts, updated_ts`

type rowScanner interface{ Scan(dest ...any) error }

func scanEntry(r rowScanner) (Entry, error) {
	var e Entry
	var selectors string
	var tombstone int64
	err := r.Scan(&e.ID, &e.Owner, &e.Scope, &e.ScopeRef, &e.Layer, &e.Kind, &e.Title,
		&e.Content, &e.FilePath, &e.FileCommit,
		&e.TopicKey, &selectors, &e.Status, &e.Version, &e.Supersedes,
		&e.Origin, &e.OriginRef, &e.ProposerModel, &e.ApprovedBy, &e.ApprovedTS,
		&e.VerifiedBy, &e.VerifiedTS, &e.ReverifyIntervalDays,
		&e.ExpiresTS, &e.LastInjectedTS,
		&tombstone, &e.TombstoneNote, &e.CreatedTS, &e.UpdatedTS)
	if err != nil {
		return Entry{}, err
	}
	e.Tombstone = tombstone == 1
	sel, err := unmarshalSelectors(selectors)
	if err != nil {
		return Entry{}, err
	}
	e.Selectors = sel
	return e, nil
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func getEntry(ctx context.Context, q queryer, id string) (Entry, error) {
	row := q.QueryRowContext(ctx, `SELECT `+entryColumns+` FROM knowledge_entries WHERE entry_id = ?`, id)
	e, err := scanEntry(row)
	if err == sql.ErrNoRows {
		return Entry{}, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	if err != nil {
		return Entry{}, fmt.Errorf("memory: read entry %q: %w", id, err)
	}
	return e, nil
}

// Get returns one entry by id (tombstones included — the stub is an audit
// surface, Spec S09.5).
func (s *Store) Get(ctx context.Context, id string) (Entry, error) {
	return getEntry(ctx, s.db, id)
}

// ListOwned lists every row stored about one person, across the scopes
// they own — the S4.1 list-everything right (Spec S09.2).
func (s *Store) ListOwned(ctx context.Context, owner string) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+entryColumns+` FROM knowledge_entries WHERE user_id = ? ORDER BY created_ts, entry_id`, owner)
	if err != nil {
		return nil, fmt.Errorf("memory: list owned: %w", err)
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

// SearchHit is one knowledge_search result. Content excerpts are the
// row-backed content; file-backed entries return their file path (native
// grep over mounted slices is the file path's channel, Spec S09.3).
type SearchHit struct {
	EntryID  string `json:"entry_id"`
	Title    string `json:"title"`
	Scope    Scope  `json:"scope"`
	ScopeRef string `json:"scope_ref,omitempty"`
	Kind     Kind   `json:"kind"`
	TopicKey string `json:"topic_key,omitempty"`
	Version  int64  `json:"version"`
	FilePath string `json:"file_path,omitempty"`
}

// Search is the knowledge_search tool's query path (Spec S09.3) — ONE
// module, server-side scoping. The adapter injects the run id; this
// function derives owner and project from platform-owned facts and appends
// the scope predicate itself. Scope is never a caller parameter, and the
// free-text query is sanitized into quoted FTS5 terms so it can never
// extend the match surface, let alone the SQL predicate.
func (s *Store) Search(ctx context.Context, runID, query string) ([]SearchHit, error) {
	if runID == "" {
		return nil, fmt.Errorf("memory: search without run id (server-side scoping, Spec S09.3)")
	}
	var owner, taskID string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, coalesce(task_id, '') FROM runs WHERE run_id = ?`, runID).Scan(&owner, &taskID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("memory: search for unknown run %q", runID)
	}
	if err != nil {
		return nil, fmt.Errorf("memory: resolve run owner: %w", err)
	}
	project, err := projectForTask(ctx, s.db, taskID)
	if err != nil {
		return nil, err
	}
	match := ftsQuery(query)
	if match == "" {
		return nil, nil
	}
	// Worker-overlay entries are outside the v0 injection/search surface
	// (Spec S09.4 activation table); L1 observations have no writer at v0.
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.entry_id, e.title, e.scope, e.scope_ref, e.kind,
		       coalesce(e.topic_key, ''), e.version, coalesce(e.file_path, '')
		  FROM knowledge_fts f
		  JOIN knowledge_entries e ON e.rowid = f.rowid
		 WHERE knowledge_fts MATCH ?
		   AND e.status = 'active' AND e.tombstone = 0
		   AND (e.scope = 'house'
		        OR (e.scope = 'user' AND e.user_id = ?)
		        OR (e.scope = 'project' AND ? <> '' AND e.scope_ref = ?))
		 ORDER BY rank, e.entry_id`,
		match, owner, project, project)
	if err != nil {
		return nil, fmt.Errorf("memory: knowledge_search: %w", err)
	}
	defer rows.Close()
	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.EntryID, &h.Title, &h.Scope, &h.ScopeRef, &h.Kind,
			&h.TopicKey, &h.Version, &h.FilePath); err != nil {
			return nil, fmt.Errorf("memory: scan hit: %w", err)
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		if err := s.recordSearchMiss(ctx, runID, owner, query); err != nil {
			return nil, err
		}
	}
	return hits, nil
}

// recordSearchMiss appends the retrieval-miss event (Spec S09.3: every
// knowledge_search miss is logged; the metric definition is S14's, the
// ⚙ memory.vector_gate.* triggers evaluate it post-15.3).
func (s *Store) recordSearchMiss(ctx context.Context, runID, owner, query string) error {
	q := query
	if len(q) > queryExcerptCap {
		q = q[:queryExcerptCap]
	}
	payload, err := json.Marshal(struct {
		Query string `json:"query"`
	}{Query: q})
	if err != nil {
		return fmt.Errorf("memory: marshal search miss: %w", err)
	}
	return s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		var gen int64
		if err := tx.QueryRowContext(ctx,
			`SELECT generation FROM runs WHERE run_id = ?`, runID).Scan(&gen); err != nil {
			return fmt.Errorf("memory: read run generation: %w", err)
		}
		_, err := s.log.AppendTx(ctx, tx, eventlog.Append{
			RunID: runID, Generation: gen, UserID: owner,
			Type: EventSearchMiss, SchemaVersion: knowledgeEventSchemaVersion,
			Payload: payload,
		})
		return err
	})
}

// ftsQuery sanitizes free text into an FTS5 query: each whitespace token
// becomes a quoted string (embedded quotes doubled), AND-joined. Free text
// can therefore never carry FTS5 operators, NEAR clauses, or column
// filters — the deterministic-selection posture on the search path.
func ftsQuery(query string) string {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(fields))
	for _, f := range fields {
		quoted = append(quoted, `"`+strings.ReplaceAll(f, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " ")
}

// projectForTask derives the task's project from platform-owned facts.
// Until the project registry lands (Spec S13/S1.6, B4) the one such fact
// is the S02.8 claim registry written at plan approval; no claim row ⇒ no
// project ⇒ project-scope knowledge stays out (conservative, like the §14
// claim intersection).
func projectForTask(ctx context.Context, q queryer, taskID string) (string, error) {
	if taskID == "" {
		return "", nil
	}
	var project string
	err := q.QueryRowContext(ctx, `
		SELECT project FROM artifact_claims
		 WHERE task_id = ? AND project <> '' AND status <> 'superseded'
		 ORDER BY claim_id LIMIT 1`, taskID).Scan(&project)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("memory: derive project: %w", err)
	}
	return project, nil
}

// InfluenceRef is one run a knowledge entry was injected into — the S2.9
// forward trace, computed as a join over trace manifests, not a system
// (Spec S09.5).
type InfluenceRef struct {
	RunID    string `json:"run_id"`
	EventSeq int64  `json:"event_seq"`
}

// Influence lists the runs whose trace manifest contains the entry
// (Spec S09.5: influence(entry) = runs whose manifest contains it; every
// injection logs the item id per Spec S05.4).
func (s *Store) Influence(ctx context.Context, entryID string) ([]InfluenceRef, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT run_id, event_seq FROM run_events
		 WHERE type = 'context.manifest' AND run_id IS NOT NULL
		   AND payload LIKE '%' || ? || '%'
		 ORDER BY event_seq`,
		`"`+manifestItemID(entryID)+`"`)
	if err != nil {
		return nil, fmt.Errorf("memory: influence query: %w", err)
	}
	defer rows.Close()
	var refs []InfluenceRef
	for rows.Next() {
		var r InfluenceRef
		if err := rows.Scan(&r.RunID, &r.EventSeq); err != nil {
			return nil, fmt.Errorf("memory: scan influence: %w", err)
		}
		refs = append(refs, r)
	}
	return refs, rows.Err()
}

// manifestItemID is the manifest item identity of an entry — shared by the
// Knowledge source and the influence join.
func manifestItemID(entryID string) string { return "knowledge/" + entryID }

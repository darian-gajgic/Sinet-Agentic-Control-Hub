// Package history is the Spec S14.10 queryable-history surface: the three
// layers the conversational assistant consumes "and nothing else" [XREF:S15].
//
//	Layer 0 — deterministic named SQL views (no model). Every S2.5 cost
//	          question is a view; the caller SELECTS one. It never computes
//	          money by generation.
//	Layer 1 — the canned parameterized catalog: named queries with typed
//	          slots, the local model classifying intent and filling slots
//	          grammar-constrained. This is the RELIABILITY FLOOR, and every
//	          answer renders with its query named.
//	Layer 2 — open SQL under the full S12.3 guardrail stack. ESCALATION,
//	          never default, and every answer is flagged lower-confidence.
//
// The ordering is structural (S14.10 ¶3: "canned queries remain the floor;
// Layer 2 is escalation, never default"): a question resolves at the lowest
// layer that can answer it, and a Layer-1 failure produces a DISAMBIGUATION
// CARD — never a guess, and never a silent fall-through to open SQL.
//
// It is a LEAF (import wall in importwall_test.go): storage + eventlog +
// settings + stdlib, plus internal/local for the two duty-calling layers
// (intent classification and the Layer-2 generation seat) and internal/redact
// for the codor-C2 search path. Everything else — the advisory run a duty call
// meters against, the clock — rides func seams composed at the shell root. The
// package owns the LOGIC; the shell owns WHEN.
//
// Migration 0016 owns the views; migration 0015 (B5-8A) owns the redacted FTS
// corpus this package searches over.
package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// Layer numbers the S14.10 escalation ladder. It is carried on every answer so
// a reader can always tell which floor answered.
const (
	LayerDeterministic = 0 // Layer 0: a named view, no model in the path
	LayerCanned        = 1 // Layer 1: a catalog query, model-classified intent
	LayerOpenSQL       = 2 // Layer 2: generated SQL under the guardrail stack
)

// Confidence labels an answer's epistemic standing. The three values are the
// three layers: there is no fourth, and no answer is unlabelled.
const (
	// ConfidenceDeterministic — a named view. No model was in the path at all.
	ConfidenceDeterministic = "deterministic"
	// ConfidenceCanned — a catalog query whose SQL is ours; a model chose WHICH
	// query and filled its typed slots, and the query is named on the answer.
	ConfidenceCanned = "canned"
	// ConfidenceLower — Layer 2 (S14.10 ¶3: "every answer is flagged
	// lower-confidence"). No Layer-2 answer can carry anything else.
	ConfidenceLower = "lower-confidence"
)

// Structural constants (S18 ratifies no `history.` key; the §35 precedent).
// Each carries its reason and is interim under the standing settings-tab
// directive.
const (
	// DefaultRowLimit bounds an answer a caller did not bound itself. Its
	// reason: an answer is read by a person, and a page of rows is the unit a
	// person reads; the bound changes how much of an answer arrives at once,
	// never which rows are eligible.
	DefaultRowLimit = 200
	// MaxRowLimit is the ceiling on a caller-supplied limit. Its reason: the
	// single-writer connection (S02.1) is shared with the control plane, so a
	// read that materializes an unbounded result set is a liveness risk to
	// everything else on the handle.
	MaxRowLimit = 1000
)

// Errors callers distinguish.
var (
	// ErrNoSuchView / ErrNoSuchQuery — the name is not in the closed registry.
	// Names are never interpolated from user text; a miss is a caller defect.
	ErrNoSuchView  = errors.New("history: no such Layer-0 view")
	ErrNoSuchQuery = errors.New("history: no such catalog query")
	// ErrSlot — a required slot is missing, or an unknown slot was supplied.
	ErrSlot = errors.New("history: slot error")
)

// Scope is the S01.9 owner scope applied to every read (CONVENTIONS §30 OQ1):
// the OPERATOR sees all rows including platform-scope; a MEMBER sees only rows
// whose user_id is their identity. The zero value is a member with no identity,
// which matches nothing — the safe direction.
type Scope struct {
	Operator bool
	UserID   string
}

// scopeArg renders the scope as the single bound value the parameterized owner
// predicate reads (catalog.go OwnerScopeOpen/Close). The operator binds the
// EMPTY STRING, which the predicate short-circuits to "every row"; a member
// binds their identity. user_id is CHECK'd non-empty in every table (0001), so
// the empty string can only ever mean "operator" and never collides with a real
// owner.
func (s Scope) scopeArg() string {
	if s.Operator {
		return ""
	}
	return s.UserID
}

// AdvisoryRun opens the platform-scope advisory run one duty call meters
// against, and returns its id plus a settle func. It is a SEAM composed at the
// shell root: the S12.1 R18 rule is that every local call writes one D7
// checkpoint on a consuming run, and a question asked of the history surface
// has no run of its own. internal/history cannot import internal/stage (the
// import wall), so the shell supplies stage.BeginAdvisory here.
//
// nil is an HONEST state, not a failure: with no advisory run the duty legs are
// unavailable and Layer 1 answers with a disambiguation card listing the
// catalog, which is exactly what it does when the model abstains.
type AdvisoryRun func(ctx context.Context, label string) (runID string, settle func())

// Config wires a Store.
type Config struct {
	DB  *storage.DB
	Log *eventlog.Log
	// Duty is the local-tier caller. Nil-safe by construction (*local.Duty's
	// own nil receiver yields ErrStackAbsent), so an absent stack degrades to
	// the disambiguation card rather than failing the question.
	Duty *local.Duty
	// Cal is the S12.5 calibration store behind the intent-filling margin gate.
	// nil = the honest-uncalibrated posture (the internal/local ReChecker
	// precedent): no margin gate, the model's answer stands, and the abstain
	// member remains the floor that always applies.
	Cal *local.CalStore
	// Advisory opens the metering run for a duty call. nil = no duty legs.
	Advisory AdvisoryRun
	// ReadOnly is the SECOND handle Layer 2 executes generated SQL on
	// (S14.10 ¶3, R29): read-only by DSN and by query_only, opened from the
	// same *storage.DB. nil = Layer 2 is unavailable, which is an honest
	// absence — Layers 0 and 1 are unaffected and they are the floor.
	//
	// It is a separate handle because the writing handle keeps a single
	// connection, so a query_only pragma on IT would disable the control
	// plane's own writes.
	ReadOnly *storage.ReadOnly
	// Now is the clock seam. nil = time.Now.
	Now func() time.Time
}

// Store is the S14.10 query surface.
type Store struct {
	db       *storage.DB
	ro       *storage.ReadOnly
	log      *eventlog.Log
	duty     *local.Duty
	cal      *local.CalStore
	advisory AdvisoryRun
	now      func() time.Time
}

// New builds a Store. DB is required; every other field degrades honestly.
func New(cfg Config) (*Store, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("history: DB is required")
	}
	s := &Store{
		db:       cfg.DB,
		ro:       cfg.ReadOnly,
		log:      cfg.Log,
		duty:     cfg.Duty,
		cal:      cfg.Cal,
		advisory: cfg.Advisory,
		now:      cfg.Now,
	}
	if s.now == nil {
		s.now = func() time.Time { return time.Now().UTC() }
	}
	return s, nil
}

// Disambiguation is the S12.4 intent-filling row's mandated failure mode:
// "below threshold ⇒ a 'which of these did you mean?' card — never a guess."
// It is the ONLY thing an unresolved question produces.
type Disambiguation struct {
	Question string   `json:"question"`
	Reason   string   `json:"reason"`
	Choices  []Choice `json:"choices"`
}

// Choice is one candidate query on a disambiguation card.
type Choice struct {
	Query       string `json:"query"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

// Answer is the ONE shape every layer returns. Query and Confidence are set
// together by the package's constructors and by nothing else, so R26 ("answers
// render with the query named") and the layer's confidence label cannot come
// apart: there is no code path that produces an answer without both.
type Answer struct {
	Layer      int             `json:"layer"`
	Query      string          `json:"query"`
	Confidence string          `json:"confidence"`
	Question   string          `json:"question,omitempty"`
	Columns    []string        `json:"columns"`
	Rows       [][]any         `json:"rows"`
	RowCount   int             `json:"row_count"`
	Truncated  bool            `json:"truncated"`
	Notes      []string        `json:"notes,omitempty"`
	Card       *Disambiguation `json:"card,omitempty"`
	// Audit is the Layer-2 record (S14.10 ¶3), carried on the answer as well as
	// written to the log so a refusal can be shown without a second read. It is
	// nil for Layers 0 and 1, which generate nothing.
	Audit *OpenSQLAudit `json:"audit,omitempty"`
}

// newAnswer is the ONLY constructor. It takes the layer and the query name
// together and derives the confidence label from the layer, so an answer can
// neither be anonymous nor mislabel its own floor.
func newAnswer(layer int, query string) Answer {
	a := Answer{Layer: layer, Query: query, Columns: []string{}, Rows: [][]any{}}
	switch layer {
	case LayerDeterministic:
		a.Confidence = ConfidenceDeterministic
	case LayerCanned:
		a.Confidence = ConfidenceCanned
	default:
		a.Confidence = ConfidenceLower
	}
	return a
}

// cardAnswer builds the honest non-answer: a Layer-1 result carrying only the
// disambiguation card. It still names a query — the catalog itself — because an
// unnamed answer is exactly what R26 forbids.
func cardAnswer(question, reason string, choices []Choice) Answer {
	a := newAnswer(LayerCanned, CatalogQueryName)
	a.Question = question
	a.Card = &Disambiguation{
		Question: question,
		Reason:   reason,
		Choices:  choices,
	}
	return a
}

// CatalogQueryName is the name an answer carries when the answer IS the
// catalog (a disambiguation card). It is not a query in the catalog.
const CatalogQueryName = "catalog.disambiguation"

// clampLimit applies the structural row bound.
func clampLimit(limit int) int {
	if limit <= 0 {
		return DefaultRowLimit
	}
	if limit > MaxRowLimit {
		return MaxRowLimit
	}
	return limit
}

// rowSource is anything a read can run on. It exists so Layer 2 executes on
// the READ-ONLY handle while Layers 0 and 1 stay on the control plane's own —
// the two differ in nothing but which connection they are allowed to touch.
type rowSource interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// query runs one read on the control plane's handle.
func (s *Store) query(ctx context.Context, a *Answer, sqlText string, args []any, limit int) error {
	return s.queryOn(ctx, s.db, a, sqlText, args, limit)
}

// queryOn runs one read and folds it into an answer's columns and rows. Reads
// are short-batch and outside any caller transaction (S02.1 read hygiene). It
// fetches one row beyond the limit so Truncated is OBSERVED rather than
// inferred from a full page.
func (s *Store) queryOn(ctx context.Context, src rowSource, a *Answer, sqlText string, args []any, limit int) error {
	rows, err := src.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return fmt.Errorf("history: run %q: %w", a.Query, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("history: columns for %q: %w", a.Query, err)
	}
	a.Columns = cols
	for rows.Next() {
		if len(a.Rows) >= limit {
			a.Truncated = true
			break
		}
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return fmt.Errorf("history: scan %q: %w", a.Query, err)
		}
		for i, c := range cells {
			// The driver hands TEXT back as []byte on some paths; render it as
			// a string so the answer marshals as text rather than base64.
			if b, ok := c.([]byte); ok {
				cells[i] = string(b)
			}
		}
		a.Rows = append(a.Rows, cells)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("history: read %q: %w", a.Query, err)
	}
	a.RowCount = len(a.Rows)
	return nil
}

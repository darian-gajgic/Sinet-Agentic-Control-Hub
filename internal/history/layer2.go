package history

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
)

// layer2.go — S14.10 ¶3: "Layer 2 — open SQL, enabled at v0 [G3 D3.5]:
// Arctic-Text2SQL-R1-7B generates against allowlisted views only, under the
// full guardrail stack — read-only connection (query_only), single-statement
// parse rejecting DDL/DML, LIMIT + timeout injection, every generated query
// audit-logged — and every answer is flagged lower-confidence in the UI …
// Canned queries remain the floor; Layer 2 is escalation, never default."
//
// THE SIX LIMBS, and where each one lives:
//
//	read-only connection    internal/storage ReadOnly (mode=ro + query_only,
//	                        verified at every open by attempting a write)
//	allowlisted views only  guard.go, against the Layer-0 registry
//	single-statement parse  guard.go, on tokens — and it EXTRACTS, because the
//	                        seat emits reasoning before the statement
//	LIMIT injection         guard.go, as a WRAP around the model's own text
//	timeout injection       here, as a context deadline
//	audit-logged            here, on EVERY exit including refusals
//	lower-confidence flag   history.go: newAnswer derives it from the layer, so
//	                        no Layer-2 answer can carry anything else
//
// ESCALATION, NEVER DEFAULT, is enforced as the ABSENCE of a path: this is a
// separate exported verb, and nothing in Layer 0 or Layer 1 calls it. An
// unresolved Layer-1 question produces a disambiguation card and stops
// (TestLayer2IsNotReachedBySilentFallthrough). Reaching Layer 2 is an act.

// Layer-2 structural constants (OQ6, ratified: S18 ratifies no `sql.`,
// `query.` or `history.` key, so these are structural constants carrying their
// reasons — the §35 precedent — and interim under the standing settings-tab
// directive. The ⚙ tally is unchanged at 118/33).
const (
	// OpenSQLMaxRows bounds a generated answer more tightly than the rest of
	// the surface. Its reason: this is the one layer whose statement was
	// written by a model, so the bound is not about how much a person reads —
	// it is about how much a statement nobody reviewed can materialize on a
	// database file the control plane is also using.
	OpenSQLMaxRows = 200

	// OpenSQLTimeout bounds execution by wall clock. Its reason is MEASURED
	// (2026-07-27): a runaway recursive query never returns, and a row limit
	// does NOT bound it — an aggregate over an infinite input yields no row to
	// count against the limit, so the limit is never reached. The clock is the
	// only limb that stops it. Five seconds is far beyond any honest query over
	// a household-scale database and far short of a hang.
	OpenSQLTimeout = 5 * time.Second

	// openSQLMaxTokens caps the generation. Its reason is the ADVERSE
	// MEASUREMENT this whole layer answers: the seat is a reasoning model that
	// emits a long chain of thought BEFORE the statement, and a cap sized for a
	// terse answer truncates the statement before it arrives. The raw alias
	// scored 0/30 against a 600-character output contract for exactly that
	// reason (P3/measurements/2026-07-22-sql-open-arctic.md). The cap must fit
	// the reasoning AND the answer.
	openSQLMaxTokens = 2048
)

// OpenSQLQueryName is the name a Layer-2 answer carries. R26 ("answers render
// with the query named") has no exception for generated SQL: the name says the
// statement was generated, and the statement itself is on the answer.
const OpenSQLQueryName = "open_sql.generated"

// EventQueryAudited is the S14.10 ¶3 audit record: "every generated query
// audit-logged". One row per Layer-2 attempt, INCLUDING refusals — a guardrail
// whose refusals are not recorded cannot be reviewed, and the refusals are the
// interesting rows.
const EventQueryAudited = "history.query_audited"

// auditOwnerPlatform scopes an audit row whose asker carried no identity (the
// operator), matching the platform-scope convention the rest of the log uses.
const auditOwnerPlatform = "platform"

// Layer-2 outcomes, as recorded in the audit payload.
const (
	OutcomeExecuted    = "executed"
	OutcomeRefused     = "refused"
	OutcomeNoSQL       = "no_sql"
	OutcomeUnavailable = "unavailable"
	OutcomeFailed      = "failed"
)

// ErrOpenSQLUnavailable reports that the Layer-2 surface is not wired — no
// read-only handle, or no local tier. It is an HONEST absence, not a failure of
// the question: Layers 0 and 1 are unaffected, and they are the floor.
var ErrOpenSQLUnavailable = errors.New("history: Layer-2 open SQL is not available")

// openSQLLimit applies the Layer-2 row bound.
func openSQLLimit(limit int) int {
	limit = clampLimit(limit)
	if limit > OpenSQLMaxRows {
		return OpenSQLMaxRows
	}
	return limit
}

// OpenSQLAudit is one audit record, as written to the log and as returned to
// the caller so a refusal can be shown without a second read.
type OpenSQLAudit struct {
	Question      string   `json:"question"`
	Outcome       string   `json:"outcome"`
	SQLGenerated  string   `json:"sql_generated,omitempty"`
	SQLExecuted   string   `json:"sql_executed,omitempty"`
	Views         []string `json:"views,omitempty"`
	LimitInjected int      `json:"limit_injected"`
	TimeoutMS     int64    `json:"timeout_ms"`
	Refusal       string   `json:"refusal,omitempty"`
	RowCount      int      `json:"row_count"`
	Truncated     bool     `json:"truncated"`
	Model         string   `json:"model,omitempty"`
	Alias         string   `json:"alias"`
	AsOperator    bool     `json:"as_operator"`
	OwnerScoped   int      `json:"owner_scoped"`
	RawOutputLen  int      `json:"raw_output_len"`
}

// AskOpenSQL is the Layer-2 escalation verb: generate SQL for a question on the
// `sql-open` duty alias, put it through the whole guardrail stack, and run what
// survives on the read-only handle.
//
// Every return path — including every refusal and every failure to generate
// anything at all — writes one audit row first. The answer is always flagged
// lower-confidence, because newAnswer derives the flag from the layer.
func (s *Store) AskOpenSQL(ctx context.Context, question string, scope Scope, limit int) (Answer, error) {
	rec := OpenSQLAudit{
		Question:      question,
		Alias:         local.AliasSQLOpen,
		LimitInjected: openSQLLimit(limit),
		TimeoutMS:     OpenSQLTimeout.Milliseconds(),
		AsOperator:    scope.Operator,
	}
	a := newAnswer(LayerOpenSQL, OpenSQLQueryName)
	a.Question = question

	if s.ro == nil || s.duty == nil || s.advisory == nil {
		rec.Outcome = OutcomeUnavailable
		rec.Refusal = "the Layer-2 surface is not wired (read-only handle, local tier or advisory run absent)"
		s.auditOpenSQL(ctx, scope, rec)
		a.Audit = &rec
		return a, fmt.Errorf("%w: %s", ErrOpenSQLUnavailable, rec.Refusal)
	}

	schema, err := s.viewSchema(ctx, scope)
	if err != nil {
		rec.Outcome = OutcomeFailed
		rec.Refusal = err.Error()
		s.auditOpenSQL(ctx, scope, rec)
		a.Audit = &rec
		return a, err
	}

	runID, settle := s.advisory(ctx, "history-open-sql")
	if settle != nil {
		defer settle()
	}
	res, err := s.duty.Call(ctx, runID, local.DutyRequest{
		Alias:     local.AliasSQLOpen,
		System:    openSQLSystem,
		User:      openSQLPrompt(question, schema),
		Name:      "history_open_sql",
		MaxTokens: openSQLMaxTokens,
		// NOT a classification, and deliberately NOT schema-constrained: the
		// seat is a reasoning model whose method is to think in prose before
		// answering. Constraining it to a JSON shape, or suppressing the
		// reasoning, would degrade the very behavior it was chosen for. The
		// output is untrusted either way — the guardrail is what makes it safe,
		// not the prompt.
	})
	if err != nil {
		rec.Outcome = OutcomeUnavailable
		rec.Refusal = "the open-SQL seat is unavailable: " + err.Error()
		s.auditOpenSQL(ctx, scope, rec)
		a.Audit = &rec
		return a, fmt.Errorf("%w: %v", ErrOpenSQLUnavailable, err)
	}
	rec.Model, rec.RawOutputLen = res.Model, len(res.Content)

	extracted, err := ExtractSQL(res.Content)
	rec.SQLGenerated = extracted
	if err != nil {
		// Two distinct outcomes, kept distinct because they mean different
		// things about the seat: NO SQL is the honest-failure path (the model
		// answered in prose, or refused, or its reasoning never reached a
		// statement — nothing is salvaged), while a REFUSAL here means the
		// statement was preceded by something that was trying to be a different
		// statement, and extraction declined to defuse it quietly.
		rec.Outcome, rec.Refusal = OutcomeNoSQL, "the model produced no statement to run"
		if errors.Is(err, ErrRefused) {
			rec.Outcome, rec.Refusal = OutcomeRefused, err.Error()
		}
		s.auditOpenSQL(ctx, scope, rec)
		a.Notes = append(a.Notes, rec.Refusal)
		a.Audit = &rec
		return a, err
	}

	guarded, err := Guard(extracted, scope, limit)
	if err != nil {
		rec.Outcome = OutcomeRefused
		rec.Refusal = err.Error()
		s.auditOpenSQL(ctx, scope, rec)
		a.Notes = append(a.Notes, rec.Refusal)
		a.Audit = &rec
		return a, err
	}
	rec.SQLExecuted, rec.Views = guarded.Statement, guarded.Views
	rec.LimitInjected, rec.OwnerScoped = guarded.Limit, guarded.Scoped

	// The timeout limb. It is a deadline on the query, not on the whole verb:
	// generation already happened, and the thing being bounded is the statement
	// nobody reviewed.
	qctx, cancel := context.WithTimeout(ctx, OpenSQLTimeout)
	defer cancel()

	a.Notes = append(a.Notes,
		"generated by "+res.Model+" on duty alias "+local.AliasSQLOpen+
			" and executed under the S12.3 guardrail stack on a read-only connection",
		"reads: "+strings.Join(guarded.Views, ", "))
	if err := s.queryOn(qctx, s.ro, &a, guarded.Statement, guarded.Args, guarded.Limit); err != nil {
		rec.Outcome = OutcomeFailed
		rec.Refusal = err.Error()
		s.auditOpenSQL(ctx, scope, rec)
		a.Audit = &rec
		return a, err
	}
	rec.Outcome, rec.RowCount, rec.Truncated = OutcomeExecuted, a.RowCount, a.Truncated
	s.auditOpenSQL(ctx, scope, rec)
	a.Audit = &rec
	return a, nil
}

// auditOpenSQL writes the R33 record. A failure to write it is deliberately
// NOT propagated: the audit is about what the guardrail did, and losing the
// ability to record must never become a way to change the outcome the caller
// already got. It is surfaced on the answer's notes instead.
func (s *Store) auditOpenSQL(ctx context.Context, scope Scope, rec OpenSQLAudit) {
	if s.log == nil {
		return
	}
	owner := scope.UserID
	if owner == "" {
		owner = auditOwnerPlatform
	}
	payload, err := json.Marshal(rec)
	if err != nil {
		return
	}
	// context.WithoutCancel: a cancelled or timed-out query is precisely the
	// case whose audit row matters most, so the record is not lost with it.
	_, _ = s.log.Append(context.WithoutCancel(ctx), eventlog.Append{
		UserID:        owner,
		Type:          EventQueryAudited,
		SchemaVersion: 1,
		Payload:       payload,
		Time:          s.now(),
	})
}

// viewSchema renders the allowlisted views and their columns for the prompt.
// The model is told what it may read, and the guardrail assumes it will not
// listen.
func (s *Store) viewSchema(ctx context.Context, scope Scope) (string, error) {
	var b strings.Builder
	for _, v := range Views() {
		if v.OwnerColumn == viewOwnerColumnNone && !scope.Operator {
			continue
		}
		cols, err := s.viewColumns(ctx, v.Name)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "  %s(%s)\n      %s\n", v.Name, strings.Join(cols, ", "), v.Description)
	}
	return b.String(), nil
}

// viewColumns reads a view's column names off the read-only handle.
func (s *Store) viewColumns(ctx context.Context, name string) ([]string, error) {
	// The name comes from the closed registry, never from user or model text.
	rows, err := s.ro.QueryContext(ctx, "SELECT * FROM "+name+" LIMIT 0")
	if err != nil {
		return nil, fmt.Errorf("history: read the shape of view %q: %w", name, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("history: columns of view %q: %w", name, err)
	}
	return cols, nil
}

const openSQLSystem = `You write ONE read-only SQLite SELECT statement answering a question about a platform's own history.
Think through the schema first if it helps; put the final statement last, in a ` + "```sql" + ` code block.
You may read ONLY the views listed. There are no other tables. Do not write to anything, do not name a table that is not listed, and write exactly one statement.`

func openSQLPrompt(question, schema string) string {
	var b strings.Builder
	b.WriteString("THE VIEWS YOU MAY READ:\n")
	b.WriteString(schema)
	b.WriteString("\nTHE QUESTION:\n")
	b.WriteString(question)
	b.WriteString("\n\nAnswer with one SELECT statement over the views above.\n")
	return b.String()
}

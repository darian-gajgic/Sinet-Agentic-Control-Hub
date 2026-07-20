package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/ledger"
)

// Source is the Knowledge source behind the S05.4 assembly seam: it
// selects house/project/user knowledge slices by pure lookup over
// platform-owned facts (owner, derived project, stage identity), applies
// the per-scope ⚙ injection budgets with the S09.8 deterministic drop
// order, records every dropped item for the trace manifest as
// over_budget_dropped, raises curation cards to the scope owner, and flags
// known-conflicting pairs entering one frame (Spec S09.3, S09.7, S09.8).
// No agent-supplied identifier ever reaches a lookup: every input comes
// from the assembly's own SliceQuery.
type Source struct {
	S *Store
}

var _ ledger.Source = (*Source)(nil)

// tokenEstimate is the deterministic size estimate the injection budgets
// meter: bytes/4, rounded up — a structural estimator, not a ⚙ value (the
// §11/§14 non-⚙ constant precedent; budgets themselves are the ⚙ keys).
func tokenEstimate(content string) int64 { return int64((len(content) + 3) / 4) }

// budgets reads the four per-scope ⚙ injection budgets at assembly time
// (G2 Def.7; worker_overlay is read with its siblings and applies when the
// scope activates at v1).
func (src *Source) budgets() (map[Scope]int64, error) {
	out := map[Scope]int64{}
	for scope, key := range map[Scope]string{
		ScopeHouse:         keyBudgetHouse,
		ScopeProject:       keyBudgetProject,
		ScopeUser:          keyBudgetUser,
		ScopeWorkerOverlay: keyBudgetOverlay,
	} {
		v, err := src.S.settings.Int(key)
		if err != nil {
			return nil, fmt.Errorf("memory: read ⚙ %s: %w", key, err)
		}
		out[scope] = v
	}
	return out, nil
}

// candidate is one selected entry with its injectable content resolved.
type candidate struct {
	entry   Entry
	content string
	tokens  int64
}

// Items implements ledger.Source.
func (src *Source) Items(ctx context.Context, q ledger.SliceQuery) ([]ledger.Item, error) {
	if q.Owner == "" {
		return nil, fmt.Errorf("memory: knowledge selection without owner (Spec S09.3 server-side scoping)")
	}
	project, err := projectForTask(ctx, src.S.db, q.TaskID)
	if err != nil {
		return nil, err
	}
	budgets, err := src.budgets()
	if err != nil {
		return nil, err
	}

	scopes := []Scope{ScopeHouse, ScopeProject, ScopeUser}
	if q.Clean {
		// The S05.4 clean-context exception structurally excludes the
		// executor's personal scope; house/project knowledge remains
		// admissible where a clean caller asks for it.
		scopes = []Scope{ScopeHouse, ScopeProject}
	}

	var items []ledger.Item
	var keptIDs []string
	var dropped []scopeDrops

	for _, scope := range scopes {
		cands, err := src.selectScope(ctx, scope, q.Owner, project)
		if err != nil {
			return nil, err
		}
		keep, over := applyBudget(cands, budgets[scope])
		for _, c := range keep {
			keptIDs = append(keptIDs, c.entry.ID)
			items = append(items, src.item(c, scope, project, ""))
		}
		if len(over) > 0 {
			dropped = append(dropped, scopeDrops{scope: scope, drops: over})
			for _, c := range over {
				items = append(items, src.item(c, scope, project, ledger.DispositionOverBudgetDropped))
			}
		}
	}

	// S09.7: known-conflicting pairs entering one frame are flagged in the
	// trace manifest and the open question re-raised.
	pairs, err := src.openConflictPairs(ctx, keptIDs)
	if err != nil {
		return nil, err
	}
	if len(pairs) > 0 {
		flag := map[string][]string{}
		for _, p := range pairs {
			flag[p.a] = append(flag[p.a], manifestItemID(p.b))
			flag[p.b] = append(flag[p.b], manifestItemID(p.a))
		}
		for i := range items {
			if c, ok := flag[strings.TrimPrefix(items[i].ItemID, "knowledge/")]; ok {
				sort.Strings(c)
				items[i].ConflictsWith = c
			}
		}
	}

	if err := src.bookkeeping(ctx, q, keptIDs, pairs, dropped, budgets); err != nil {
		return nil, err
	}
	return items, nil
}

// selectScope loads the active entries of one scope the query's facts
// select, with conservative selector matching (see Selectors) and content
// resolved (file-backed entries read from the knowledge dir). Ordered by
// entry id for determinism.
func (src *Source) selectScope(ctx context.Context, scope Scope, owner, project string) ([]candidate, error) {
	var rows *sql.Rows
	var err error
	switch scope {
	case ScopeHouse:
		rows, err = src.S.db.QueryContext(ctx, `
			SELECT `+entryColumns+` FROM knowledge_entries
			 WHERE scope = 'house' AND status = 'active' AND tombstone = 0 AND layer = 'L2'
			 ORDER BY entry_id`)
	case ScopeProject:
		if project == "" {
			return nil, nil
		}
		rows, err = src.S.db.QueryContext(ctx, `
			SELECT `+entryColumns+` FROM knowledge_entries
			 WHERE scope = 'project' AND scope_ref = ? AND status = 'active' AND tombstone = 0 AND layer = 'L2'
			 ORDER BY entry_id`, project)
	case ScopeUser:
		rows, err = src.S.db.QueryContext(ctx, `
			SELECT `+entryColumns+` FROM knowledge_entries
			 WHERE scope = 'user' AND user_id = ? AND status = 'active' AND tombstone = 0 AND layer = 'L2'
			 ORDER BY entry_id`, owner)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memory: select %s knowledge: %w", scope, err)
	}
	defer rows.Close()
	var out []candidate
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: scan knowledge entry: %w", err)
		}
		if !selectorsMatch(e.Selectors, project) {
			continue
		}
		content := e.Content
		if e.FilePath != "" {
			raw, err := os.ReadFile(filepath.Join(src.S.root, e.FilePath))
			if err != nil {
				return nil, fmt.Errorf("memory: read knowledge file for %q: %w", e.ID, err)
			}
			content = string(raw)
		}
		out = append(out, candidate{entry: e, content: content, tokens: tokenEstimate(content)})
	}
	return out, rows.Err()
}

// selectorsMatch applies the conservative rule of Spec S05.4 pure lookup:
// every declared selector must match a platform-confirmed fact. The
// project fact exists (claim registry); domain, task-type, and trigger
// phrases have no confirmable fact until the project registry lands
// (Spec S13/S1.6, B4), so entries declaring them do not inject — the safe
// direction, mirroring the §14 claim intersection.
func selectorsMatch(s Selectors, project string) bool {
	if s.Domain != "" || s.TaskType != "" || len(s.Triggers) > 0 {
		return false
	}
	if s.Project != "" && s.Project != project {
		return false
	}
	return true
}

// applyBudget enforces one scope's ⚙ token budget with the ratified drop
// order (Spec S09.8): over budget, drop the lowest selector specificity
// first, then the oldest last_injected_at, then entry id — deterministic
// and total.
func applyBudget(cands []candidate, budget int64) (keep, dropped []candidate) {
	if len(cands) == 0 {
		return nil, nil
	}
	keep = append(keep, cands...)
	var total int64
	for _, c := range keep {
		total += c.tokens
	}
	// Drop priority: lowest specificity first; ties by oldest
	// last_injected_ts (never-injected counts oldest); then entry id.
	priority := make([]candidate, len(keep))
	copy(priority, keep)
	sort.SliceStable(priority, func(i, j int) bool {
		si, sj := priority[i].entry.Selectors.Specificity(), priority[j].entry.Selectors.Specificity()
		if si != sj {
			return si < sj
		}
		if priority[i].entry.LastInjectedTS != priority[j].entry.LastInjectedTS {
			return priority[i].entry.LastInjectedTS < priority[j].entry.LastInjectedTS
		}
		return priority[i].entry.ID < priority[j].entry.ID
	})
	for _, victim := range priority {
		if total <= budget {
			break
		}
		for i, c := range keep {
			if c.entry.ID == victim.entry.ID {
				keep = append(keep[:i], keep[i+1:]...)
				break
			}
		}
		total -= victim.tokens
		dropped = append(dropped, victim)
	}
	return keep, dropped
}

// item builds the ledger item for one candidate.
func (src *Source) item(c candidate, scope Scope, project, disposition string) ledger.Item {
	precedence := ledger.PrecedenceUser
	switch scope {
	case ScopeHouse:
		precedence = ledger.PrecedenceHouse
	case ScopeProject:
		precedence = ledger.PrecedenceProject
	}
	sourcePath := "platform.db:knowledge_entries/" + c.entry.ID
	if c.entry.FilePath != "" {
		sourcePath = "knowledge/" + c.entry.FilePath
	}
	rule := fmt.Sprintf("scope=%s owner=%s kind=%s specificity=%d (S09.3 registry-keyed)",
		scope, c.entry.Owner, c.entry.Kind, c.entry.Selectors.Specificity())
	if c.entry.Selectors.Project != "" {
		rule += " project=" + project
	}
	return ledger.Item{
		ItemID:       manifestItemID(c.entry.ID),
		SourcePath:   sourcePath,
		Content:      c.content,
		Version:      strconv.FormatInt(c.entry.Version, 10),
		SelectorRule: rule,
		Precedence:   precedence,
		Disposition:  disposition,
	}
}

// conflictPair is one open edge between two entries in the same frame.
type conflictPair struct {
	id   int64
	a, b string
}

func (src *Source) openConflictPairs(ctx context.Context, keptIDs []string) ([]conflictPair, error) {
	if len(keptIDs) < 2 {
		return nil, nil
	}
	in := strings.Repeat("?,", len(keptIDs))
	in = in[:len(in)-1]
	args := make([]any, 0, 2*len(keptIDs))
	for _, id := range keptIDs {
		args = append(args, id)
	}
	for _, id := range keptIDs {
		args = append(args, id)
	}
	rows, err := src.S.db.QueryContext(ctx, `
		SELECT conflict_id, entry_id, other_entry_id FROM knowledge_conflicts
		 WHERE status = 'open' AND entry_id IN (`+in+`) AND other_entry_id IN (`+in+`)
		 ORDER BY conflict_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: open conflict pairs: %w", err)
	}
	defer rows.Close()
	var pairs []conflictPair
	for rows.Next() {
		var p conflictPair
		if err := rows.Scan(&p.id, &p.a, &p.b); err != nil {
			return nil, fmt.Errorf("memory: scan conflict pair: %w", err)
		}
		pairs = append(pairs, p)
	}
	return pairs, rows.Err()
}

// scopeDrops collects one scope's over-budget drops for its curation card.
type scopeDrops struct {
	scope Scope
	drops []candidate
}

// bookkeeping performs the selection's platform acts in one transaction:
// last_injected_at on the injected entries (the S09.4 station-5 staleness
// signal), the S09.8 curation cards for over-budget scopes (level-
// triggered: one open card per scope+owner), and the S09.7 re-raise of
// unresolved conflict questions entering the frame. It runs pre-assembly
// by construction (sources are collected before the assembly transaction,
// CONVENTIONS §13); the trace manifest remains the audit truth of what was
// injected.
func (src *Source) bookkeeping(ctx context.Context, q ledger.SliceQuery, keptIDs []string, pairs []conflictPair, drops []scopeDrops, budgets map[Scope]int64) error {
	if len(keptIDs) == 0 && len(pairs) == 0 && len(drops) == 0 {
		return nil
	}
	now := rfc3339(src.S.now())
	return src.S.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if len(keptIDs) > 0 {
			in := strings.Repeat("?,", len(keptIDs))
			args := make([]any, 0, len(keptIDs)+1)
			args = append(args, now)
			for _, id := range keptIDs {
				args = append(args, id)
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE knowledge_entries SET last_injected_ts = ? WHERE entry_id IN (`+in[:len(in)-1]+`)`,
				args...); err != nil {
				return fmt.Errorf("memory: record last_injected: %w", err)
			}
		}

		// Run-scoped acts need the assembling run; a query without one
		// (direct source use in tests) skips them.
		if q.RunID == "" {
			return nil
		}
		var gen int64
		var runOwner string
		err := tx.QueryRowContext(ctx,
			`SELECT generation, user_id FROM runs WHERE run_id = ?`, q.RunID).Scan(&gen, &runOwner)
		if err != nil {
			return fmt.Errorf("memory: read assembling run: %w", err)
		}

		for _, p := range pairs {
			payload, err := json.Marshal(struct {
				ConflictID   int64  `json:"conflict_id"`
				EntryID      string `json:"entry_id"`
				OtherEntryID string `json:"other_entry_id"`
				Reraised     bool   `json:"reraised"`
				Stage        string `json:"stage"`
			}{p.id, p.a, p.b, true, q.Stage})
			if err != nil {
				return fmt.Errorf("memory: marshal conflict re-raise: %w", err)
			}
			if _, err := src.S.log.AppendTx(ctx, tx, eventlogRunAppend(q.RunID, gen, runOwner, EventConflict, payload)); err != nil {
				return err
			}
		}

		for _, d := range drops {
			if err := src.raiseCurationCard(ctx, tx, q, gen, runOwner, d, budgets[d.scope], now); err != nil {
				return err
			}
		}
		return nil
	})
}

// raiseCurationCard raises the S09.8 over-budget curation card to the
// scope owner as a durable ask (level-triggered: skipped while one is
// open for the scope+owner). Budget pressure converts knowledge bloat into
// a visible curation task; at v1 a distillation proposal takes its place.
func (src *Source) raiseCurationCard(ctx context.Context, tx *sql.Tx, q ledger.SliceQuery, gen int64, runOwner string, d scopeDrops, budget int64, now string) error {
	cardOwner := q.Owner
	if d.scope == ScopeHouse {
		// House curation addresses the operator (D10); fall back to the
		// run owner if no operator row exists (dev posture).
		var op string
		err := tx.QueryRowContext(ctx,
			`SELECT user_id FROM users WHERE role = 'operator' ORDER BY created_ts, user_id LIMIT 1`).Scan(&op)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("memory: resolve operator: %w", err)
		}
		if op != "" {
			cardOwner = op
		}
	}
	prefix := fmt.Sprintf("ask-knowledge-curation-%s-%s-", d.scope, cardOwner)
	var open int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM asks WHERE status = 'open' AND ask_id LIKE ? ESCAPE '\'`,
		likeEscape(prefix)+"%").Scan(&open); err != nil {
		return fmt.Errorf("memory: curation card dedup: %w", err)
	}
	if open > 0 {
		return nil
	}
	type droppedRef struct {
		EntryID string `json:"entry_id"`
		Title   string `json:"title"`
		Tokens  int64  `json:"tokens"`
	}
	refs := make([]droppedRef, 0, len(d.drops))
	for _, c := range d.drops {
		refs = append(refs, droppedRef{EntryID: c.entry.ID, Title: c.entry.Title, Tokens: c.tokens})
	}
	askID := prefix + strconv.FormatInt(src.S.now().UnixNano(), 10)
	snapshot, err := json.Marshal(struct {
		Kind         string       `json:"kind"`
		Scope        Scope        `json:"scope"`
		Owner        string       `json:"owner"`
		BudgetTokens int64        `json:"budget_tokens"`
		Dropped      []droppedRef `json:"dropped"`
		Stage        string       `json:"stage"`
		IssuedTS     string       `json:"issued_ts"`
		Summary      string       `json:"summary"`
	}{
		Kind: "knowledge_curation", Scope: d.scope, Owner: cardOwner,
		BudgetTokens: budget, Dropped: refs, Stage: q.Stage, IssuedTS: now,
		Summary: fmt.Sprintf("%s-scope knowledge exceeded its ⚙ injection budget (%d tokens): %d entr%s dropped from assembly — curate (retire, distill, or re-scope).",
			d.scope, budget, len(refs), pluralY(len(refs))),
	})
	if err != nil {
		return fmt.Errorf("memory: marshal curation card: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO asks (ask_id, run_id, user_id, snapshot, status, observed_ts)
		 VALUES (?, ?, ?, ?, 'open', ?)`,
		askID, q.RunID, cardOwner, string(snapshot), now); err != nil {
		return fmt.Errorf("memory: insert curation card: %w", err)
	}
	payload, err := json.Marshal(struct {
		AskID   string `json:"ask_id"`
		Scope   Scope  `json:"scope"`
		Owner   string `json:"owner"`
		Dropped int    `json:"dropped"`
		Budget  int64  `json:"budget_tokens"`
	}{askID, d.scope, cardOwner, len(refs), budget})
	if err != nil {
		return fmt.Errorf("memory: marshal curation event: %w", err)
	}
	_, err = src.S.log.AppendTx(ctx, tx, eventlogRunAppend(q.RunID, gen, runOwner, EventCuration, payload))
	return err
}

func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// likeEscape escapes LIKE metacharacters in a literal prefix.
func likeEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	return strings.ReplaceAll(s, `_`, `\_`)
}

// eventlogRunAppend builds a run-scoped knowledge event append.
func eventlogRunAppend(runID string, gen int64, userID, typ string, payload []byte) eventlog.Append {
	return eventlog.Append{
		RunID: runID, Generation: gen, UserID: userID, Type: typ,
		SchemaVersion: knowledgeEventSchemaVersion, Payload: payload,
	}
}

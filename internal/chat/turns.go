package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/dariannixda-eng/Sinet-Agentic-Control-Hub/internal/eventlog"
)

// turns.go — the S15.7 turn lifecycle: a turn opens with the message that
// caused it, settles with what it produced, or is abandoned by the explicit
// hard-stop.
//
// A TURN IS SERVER-SIDE STATE, WHICH IS WHAT MAKES SURVIVES-NAVIGATION REAL.
// The widget's "in-flight" is the outstanding POST; the RECORD of the turn is
// this row, so leaving the view and coming back — or reloading the page
// entirely — re-reads a running turn and re-attaches to it (OQ5(a)). Nothing
// about that survival lives in the browser, which is also why the web-storage
// scan can stay at zero.

// inflight maps a running turn to the cancel func of the request driving it, so
// the hard-stop verb — which arrives on a DIFFERENT request — can cancel the
// work server-side "where cancellable" (OQ6). It is the internal/stage cancel
// registry in miniature (§39): in-process only, emptied when the driving
// request returns, and never the record of anything. The RECORD is the row.
type inflight struct {
	mu sync.Mutex
	m  map[string]context.CancelFunc
}

func (f *inflight) set(turnID string, cancel context.CancelFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.m == nil {
		f.m = map[string]context.CancelFunc{}
	}
	f.m[turnID] = cancel
}

func (f *inflight) clear(turnID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.m, turnID)
}

func (f *inflight) cancel(turnID string) bool {
	f.mu.Lock()
	c, ok := f.m[turnID]
	f.mu.Unlock()
	if ok && c != nil {
		c()
	}
	return ok
}

// Drive registers a running turn's cancellable context and returns it plus a
// release func. The caller (the transport) drives the turn's act under the
// returned context, so a hard-stop arriving meanwhile cancels it. Release is
// deferred, so an abandoned entry can never outlive the request.
func (s *Store) Drive(ctx context.Context, turnID string) (context.Context, func()) {
	cctx, cancel := context.WithCancel(ctx)
	s.flight.set(turnID, cancel)
	return cctx, func() {
		s.flight.clear(turnID)
		cancel()
	}
}

const turnColumns = `turn_id, session_id, user_id, kind, state,
	outcome_kind, outcome, produced, exchange_seq, started_ts, settled_ts`

func scanTurn(r interface{ Scan(...any) error }) (Turn, error) {
	var t Turn
	var outcome, produced string
	if err := r.Scan(&t.ID, &t.SessionID, &t.Owner, &t.Kind, &t.State,
		&t.OutcomeKind, &outcome, &produced, &t.exchangeSeq, &t.StartedTS, &t.SettledTS); err != nil {
		return Turn{}, err
	}
	if outcome != "" {
		t.Outcome = json.RawMessage(outcome)
	}
	if produced != "" {
		if err := json.Unmarshal([]byte(produced), &t.Produced); err != nil {
			return Turn{}, fmt.Errorf("chat: decode produced list: %w", err)
		}
	}
	return t, nil
}

// BeginTurn records the message that opens a turn and the turn itself, in ONE
// transaction with the chat.turn_started append. It then names an unnamed
// session from that first message (title.go) — a step that can fail without
// touching the turn, because a session nobody named is a cosmetic absence and a
// turn that will not start is a broken surface.
func (s *Store) BeginTurn(ctx context.Context, viewer, sessionID, kind, text string) (Turn, Message, error) {
	if !Kinds(kind) {
		return Turn{}, Message{}, fmt.Errorf("%w: unknown turn kind %q", ErrInvalid, kind)
	}
	if strings.TrimSpace(text) == "" {
		return Turn{}, Message{}, fmt.Errorf("%w: a turn needs a message", ErrInvalid)
	}
	if n := utf8.RuneCountInString(text); n > MaxMessageRunes {
		return Turn{}, Message{}, fmt.Errorf("%w: message is %d characters, the bound is %d", ErrInvalid, n, MaxMessageRunes)
	}
	if _, err := s.Session(ctx, viewer, sessionID); err != nil {
		return Turn{}, Message{}, err
	}
	// One thread per conversation. Two turns racing to settle would give their
	// produced-files windows an overlap no attribution could honestly resolve.
	if running, ok, err := s.RunningTurn(ctx, viewer, sessionID); err != nil {
		return Turn{}, Message{}, err
	} else if ok {
		return running, Message{}, ErrTurnRunning
	}

	now := s.stamp()
	t := Turn{ID: s.newID("ct-"), SessionID: sessionID, Owner: viewer,
		Kind: kind, State: StateRunning, StartedTS: now}
	m := Message{ID: s.newID("cm-"), SessionID: sessionID, Owner: viewer,
		Role: "user", Body: text, TurnID: t.ID, CreatedTS: now}
	payload, err := json.Marshal(struct {
		Session string `json:"session"`
		Turn    string `json:"turn"`
		Kind    string `json:"kind"`
		Message string `json:"message"`
		TextLen int    `json:"text_len"`
	}{sessionID, t.ID, kind, m.ID, utf8.RuneCountInString(text)})
	if err != nil {
		return Turn{}, Message{}, fmt.Errorf("chat: encode %s payload: %w", EventTurnStarted, err)
	}
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO chat_messages (message_id, session_id, user_id, role, body, turn_id, created_ts)
			 VALUES (?, ?, ?, 'user', ?, ?, ?)`,
			m.ID, sessionID, viewer, text, t.ID, now); err != nil {
			return fmt.Errorf("chat: insert message: %w", err)
		}
		// The exchange watermark is read INSIDE the transaction that opens the
		// turn, so no upload can slip between the reading and the recording and
		// end up on the wrong side of the window (exchange.go: producedSince).
		if err := tx.QueryRowContext(ctx,
			`SELECT IFNULL(MAX(seq), 0) FROM chat_exchange_files`).Scan(&t.exchangeSeq); err != nil {
			return fmt.Errorf("chat: read exchange watermark: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO chat_turns (turn_id, session_id, user_id, kind, state, exchange_seq, started_ts)
			 VALUES (?, ?, ?, ?, 'running', ?, ?)`,
			t.ID, sessionID, viewer, kind, t.exchangeSeq, now); err != nil {
			return fmt.Errorf("chat: insert turn: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE chat_sessions SET updated_ts = ? WHERE session_id = ? AND user_id = ?`,
			now, sessionID, viewer); err != nil {
			return fmt.Errorf("chat: touch session: %w", err)
		}
		_, err := s.log.AppendTx(ctx, tx, eventlog.Append{
			UserID: viewer, Type: EventTurnStarted,
			SchemaVersion: chatEventSchemaVersion, Payload: payload,
		})
		return err
	})
	if err != nil {
		return Turn{}, Message{}, err
	}
	s.titleFromFirstMessage(ctx, viewer, sessionID, text)
	return t, m, nil
}

// SettleTurn records what a turn produced and computes its produced-files diff,
// in ONE transaction with the chat.turn_settled append.
//
// A turn that already ended is NOT an error the caller has to special-case: the
// resolved turn comes back with ErrTurnResolved, and the transport serves it as
// the already-resolved state (the S15.2 retry-safety shape). That is also what
// makes OQ6's "a completed-anyway result renders late with honest state" true —
// the act finishing after a hard-stop finds an abandoned row and does not
// overwrite it, so the record still says the person stopped.
func (s *Store) SettleTurn(ctx context.Context, viewer, turnID, outcomeKind string, outcome json.RawMessage) (Turn, error) {
	switch outcomeKind {
	case OutcomeAnswer, OutcomeTask, OutcomeError:
	default:
		return Turn{}, fmt.Errorf("%w: unknown outcome kind %q", ErrInvalid, outcomeKind)
	}
	cur, err := s.Turn(ctx, viewer, turnID)
	if err != nil {
		return Turn{}, err
	}
	if cur.State != StateRunning {
		return cur, ErrTurnResolved
	}
	now := s.stamp()
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		// The window CLOSES here, and the diff is computed inside the same
		// transaction that closes it — so no upload can land between the two and
		// end up inside a window nobody recorded. It reads through the tx rather
		// than the store: the control plane's pool is ONE connection (S02.1), and
		// a second reader opened underneath an open write transaction deadlocks.
		var settleSeq int64
		if err := tx.QueryRowContext(ctx,
			`SELECT IFNULL(MAX(seq), 0) FROM chat_exchange_files`).Scan(&settleSeq); err != nil {
			return fmt.Errorf("chat: read exchange watermark: %w", err)
		}
		produced, err := s.producedSince(ctx, tx, viewer, turnID, cur.exchangeSeq)
		if err != nil {
			return err
		}
		producedJSON, err := json.Marshal(produced)
		if err != nil {
			return fmt.Errorf("chat: encode produced list: %w", err)
		}
		payload, err := json.Marshal(turnSettledPayload(cur, outcomeKind, outcome, len(produced)))
		if err != nil {
			return fmt.Errorf("chat: encode %s payload: %w", EventTurnSettled, err)
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE chat_turns SET state = ?, outcome_kind = ?, outcome = ?, produced = ?,
			        settled_exchange_seq = ?, settled_ts = ?
			  WHERE turn_id = ? AND user_id = ? AND state = ?`,
			StateSettled, outcomeKind, string(outcome), string(producedJSON), settleSeq, now,
			turnID, viewer, StateRunning)
		if err != nil {
			return fmt.Errorf("chat: settle turn: %w", err)
		}
		// The state predicate is the race guard: a hard-stop that committed
		// between the read above and this write leaves zero rows touched, and
		// the abandoned record stands.
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return ErrTurnResolved
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE chat_sessions SET updated_ts = ? WHERE session_id = ? AND user_id = ?`,
			now, cur.SessionID, viewer); err != nil {
			return fmt.Errorf("chat: touch session: %w", err)
		}
		_, err = s.log.AppendTx(ctx, tx, eventlog.Append{
			UserID: viewer, Type: EventTurnSettled,
			SchemaVersion: chatEventSchemaVersion, Payload: payload,
		})
		return err
	})
	if errors.Is(err, ErrTurnResolved) {
		resolved, rerr := s.Turn(ctx, viewer, turnID)
		if rerr != nil {
			return Turn{}, rerr
		}
		return resolved, ErrTurnResolved
	}
	if err != nil {
		return Turn{}, err
	}
	return s.Turn(ctx, viewer, turnID)
}

// turnSettledPayload builds the refs-not-blobs settle payload: which verb
// answered, which ladder layer and named query, the born task's id, and the
// COUNT of produced files. The answer body itself never rides the log — it is a
// column in chat_turns and a read away.
func turnSettledPayload(cur Turn, outcomeKind string, outcome json.RawMessage, produced int) any {
	p := struct {
		Session  string `json:"session"`
		Turn     string `json:"turn"`
		Kind     string `json:"kind"`
		Outcome  string `json:"outcome"`
		Layer    *int   `json:"layer,omitempty"`
		Query    string `json:"query,omitempty"`
		Task     string `json:"task,omitempty"`
		Produced int    `json:"produced"`
	}{Session: cur.SessionID, Turn: cur.ID, Kind: cur.Kind, Outcome: outcomeKind, Produced: produced}
	// The layer/query/task refs are read back out of the outcome the store was
	// handed, by key, so the audit says which layer answered without this
	// package knowing anything about how an answer is built.
	var probe struct {
		Layer  *int   `json:"layer"`
		Query  string `json:"query"`
		TaskID string `json:"task_id"`
	}
	if len(outcome) > 0 && json.Unmarshal(outcome, &probe) == nil {
		p.Layer, p.Query, p.Task = probe.Layer, probe.Query, probe.TaskID
	}
	return p
}

// AbandonTurn is the S15.7 hard-stop: an explicit user act with a record (OQ6).
// It marks the turn abandoned, mints its event, and cancels the driving
// request's context where one is registered — "where cancellable", which at v0
// means the ladder read or duty call the transport is inside.
//
// It does NOT cancel a handed-off task. Chat claims no lifecycle authority over
// a task; 4.5's cancel verb is the task's own and stays there.
func (s *Store) AbandonTurn(ctx context.Context, viewer, turnID string) (Turn, error) {
	cur, err := s.Turn(ctx, viewer, turnID)
	if err != nil {
		return Turn{}, err
	}
	if cur.State != StateRunning {
		return cur, ErrTurnResolved
	}
	payload, err := json.Marshal(struct {
		Session string `json:"session"`
		Turn    string `json:"turn"`
		Kind    string `json:"kind"`
		Reason  string `json:"reason"`
	}{cur.SessionID, cur.ID, cur.Kind, ReasonHardStop})
	if err != nil {
		return Turn{}, fmt.Errorf("chat: encode %s payload: %w", EventTurnAbandoned, err)
	}
	now := s.stamp()
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		// An abandoned turn claims nothing, but its window still CLOSES: it was
		// open while files arrived, and another turn that overlapped it must not
		// then claim them unambiguously (OQ7 — the origin ref decides, else no
		// turn). A window left open forever would silently disqualify everything.
		var settleSeq int64
		if err := tx.QueryRowContext(ctx,
			`SELECT IFNULL(MAX(seq), 0) FROM chat_exchange_files`).Scan(&settleSeq); err != nil {
			return fmt.Errorf("chat: read exchange watermark: %w", err)
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE chat_turns SET state = ?, settled_exchange_seq = ?, settled_ts = ?
			  WHERE turn_id = ? AND user_id = ? AND state = ?`,
			StateAbandoned, settleSeq, now, turnID, viewer, StateRunning)
		if err != nil {
			return fmt.Errorf("chat: abandon turn: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return ErrTurnResolved
		}
		_, err = s.log.AppendTx(ctx, tx, eventlog.Append{
			UserID: viewer, Type: EventTurnAbandoned,
			SchemaVersion: chatEventSchemaVersion, Payload: payload,
		})
		return err
	})
	if errors.Is(err, ErrTurnResolved) {
		resolved, rerr := s.Turn(ctx, viewer, turnID)
		if rerr != nil {
			return Turn{}, rerr
		}
		return resolved, ErrTurnResolved
	}
	if err != nil {
		return Turn{}, err
	}
	s.flight.cancel(turnID)
	return s.Turn(ctx, viewer, turnID)
}

// ReconcileOrphanedTurns closes the windows of turns that no live process owns,
// and is called ONCE at bootstrap before the surface serves.
//
// A turn is `running` only while the request driving it is in flight IN THIS
// PROCESS: the transport settles or abandons before it returns, and `inflight`
// — the only thing that can cancel one — is an in-process map emptied when that
// request returns. So a `running` row observed at startup is orphaned BY
// DEFINITION: the process that owned it died mid-turn, and nothing will ever
// settle or abandon it.
//
// Left alone, such a row never writes settled_exchange_seq, so its window stays
// open forever — and an open window is an unconditional disqualifier, so every
// later upload by that owner would be attributed to no turn, for good. That is a
// silent degradation with no recovery, and strictly worse than the ambiguity the
// disqualifier exists to prevent: honest absence is the answer to "which turn
// produced this", not to "does attribution work at all".
//
// The reconciliation is an abandonment, not a settlement: what the turn was
// doing when the process died is unknown, and a settled record would claim an
// outcome nobody observed. The record survives with its window closed, which is
// the §37-C shape — the ending is recorded even though the act did not finish.
func (s *Store) ReconcileOrphanedTurns(ctx context.Context) (int, error) {
	var closed int
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT turn_id, session_id, user_id, kind FROM chat_turns WHERE state = ?`,
			StateRunning)
		if err != nil {
			return fmt.Errorf("chat: read orphaned turns: %w", err)
		}
		type orphan struct{ turnID, sessionID, userID, kind string }
		var orphans []orphan
		for rows.Next() {
			var o orphan
			if err := rows.Scan(&o.turnID, &o.sessionID, &o.userID, &o.kind); err != nil {
				rows.Close()
				return fmt.Errorf("chat: scan orphaned turn: %w", err)
			}
			orphans = append(orphans, o)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("chat: read orphaned turns: %w", err)
		}
		rows.Close()
		if len(orphans) == 0 {
			return nil
		}
		// The watermark here is EXACT rather than approximate: uploads run in the
		// same process, so nothing can have entered the manifest while that
		// process was dead, and MAX(seq) at bootstrap is MAX(seq) at the crash.
		//
		// Honest note: this write records the truth but is not behaviourally
		// observable, and no test claims otherwise. Every running turn is
		// reconciled in this one pass, and any turn opened afterwards starts at
		// this same watermark — so no later window can overlap an orphan's, and
		// the value it closes at cannot change an attribution. What actually
		// unblocks the owner is the state flip below.
		var settleSeq int64
		if err := tx.QueryRowContext(ctx,
			`SELECT IFNULL(MAX(seq), 0) FROM chat_exchange_files`).Scan(&settleSeq); err != nil {
			return fmt.Errorf("chat: read exchange watermark: %w", err)
		}
		now := s.stamp()
		for _, o := range orphans {
			payload, err := json.Marshal(struct {
				Session string `json:"session"`
				Turn    string `json:"turn"`
				Kind    string `json:"kind"`
				Reason  string `json:"reason"`
			}{o.sessionID, o.turnID, o.kind, ReasonProcessRestart})
			if err != nil {
				return fmt.Errorf("chat: encode %s payload: %w", EventTurnAbandoned, err)
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE chat_turns SET state = ?, settled_exchange_seq = ?, settled_ts = ?
				  WHERE turn_id = ? AND state = ?`,
				StateAbandoned, settleSeq, now, o.turnID, StateRunning); err != nil {
				return fmt.Errorf("chat: reconcile orphaned turn: %w", err)
			}
			if _, err := s.log.AppendTx(ctx, tx, eventlog.Append{
				UserID: o.userID, Type: EventTurnAbandoned,
				SchemaVersion: chatEventSchemaVersion, Payload: payload,
			}); err != nil {
				return err
			}
			closed++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return closed, nil
}

// Turn reads one turn if the viewer owns it, and serves its produced-files list
// as the stored window diff UNION anything the manifest attributes to it by
// origin ref — the two mechanisms OQ7 names, one for what appeared during the
// turn and one for what arrived afterwards saying which turn caused it.
func (s *Store) Turn(ctx context.Context, viewer, turnID string) (Turn, error) {
	if viewer == "" || turnID == "" {
		return Turn{}, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+turnColumns+` FROM chat_turns WHERE turn_id = ? AND user_id = ?`, turnID, viewer)
	t, err := scanTurn(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Turn{}, ErrNotFound
	}
	if err != nil {
		return Turn{}, fmt.Errorf("chat: read turn: %w", err)
	}
	late, err := s.attributedTo(ctx, viewer, turnID)
	if err != nil {
		return Turn{}, err
	}
	t.Produced = union(t.Produced, late)
	return t, nil
}

// Turns serves one session's turns, oldest first, bounded.
func (s *Store) Turns(ctx context.Context, viewer, sessionID string) ([]Turn, error) {
	if _, err := s.Session(ctx, viewer, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+turnColumns+` FROM chat_turns WHERE session_id = ? AND user_id = ?
		  ORDER BY started_ts, turn_id LIMIT ?`, sessionID, viewer, TranscriptCap)
	if err != nil {
		return nil, fmt.Errorf("chat: list turns: %w", err)
	}
	out := []Turn{}
	for rows.Next() {
		t, err := scanTurn(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("chat: scan turn: %w", err)
		}
		out = append(out, t)
	}
	err = rows.Err()
	// The cursor is closed EXPLICITLY before the per-row reads below: the
	// control plane's pool is ONE connection (S02.1), so a still-open cursor
	// deadlocks the queries that follow (the §38 hung-test reading).
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range out {
		late, err := s.attributedTo(ctx, viewer, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Produced = union(out[i].Produced, late)
	}
	return out, nil
}

// RunningTurn reports the session's in-flight turn, if any. It is the read a
// re-opened view uses to re-attach to a turn started before the person
// navigated away (R14).
func (s *Store) RunningTurn(ctx context.Context, viewer, sessionID string) (Turn, bool, error) {
	if viewer == "" || sessionID == "" {
		return Turn{}, false, nil
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+turnColumns+` FROM chat_turns
		  WHERE session_id = ? AND user_id = ? AND state = ?
		  ORDER BY started_ts DESC, turn_id DESC LIMIT 1`, sessionID, viewer, StateRunning)
	t, err := scanTurn(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Turn{}, false, nil
	}
	if err != nil {
		return Turn{}, false, fmt.Errorf("chat: read running turn: %w", err)
	}
	return t, true, nil
}

// union merges two id lists, keeping first-seen order and dropping duplicates.
func union(a, b []string) []string {
	if len(b) == 0 {
		return a
	}
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, id := range list {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

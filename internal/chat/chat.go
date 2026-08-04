// Package chat is the durable state of the Spec S15.7 conversational
// assistant: the per-user session registry, the immutable transcript, the turn
// lifecycle and the JARVIS file exchange. It is a LEAF over storage + eventlog
// (+ internal/local for the one titling duty), held directly in api.Config the
// way *review.Store, *settings.Registry and *memory.Store are (CONVENTIONS
// §39/§40-B/§40-C): the dependency is narrow and acyclic, and an interface
// would only be a second name for one type.
//
// WHAT THIS PACKAGE IS NOT. It routes nothing and answers nothing. The three
// verbs S15.7 grants the assistant — a platform-state question, a start-a-task
// handoff, and the sessions/exchange machinery around them — are dispatched by
// the transport (internal/api/chatapi.go), which already holds the LANDED S14.10
// query surface and the S06 intake surface and calls them directly. This
// package builds no SQL over runs, tasks or events, calls no model except the
// one titling duty below, and re-implements no guardrail: it records what
// happened and serves it back.
//
// VISIBILITY IS OWNER-ONLY, AND IT IS STRUCTURAL RATHER THAN POLICED (P3-B6-7
// OQ1, a deliberate STATED deviation). Every read below takes a VIEWER and has
// no role parameter at all — there is no operator limb to forget to omit. The
// operator does not read another member's transcripts. This extends the §40-C
// content-vs-telemetry line: §30's operator-sees-all is an OBSERVABILITY rule
// about runs and telemetry, and a conversation is personal CONTENT, the same
// class as a memory entry. D10 is authority over the household's shared ground,
// not a key to what someone said in a chat window. Both directions are pinned
// by test — the owner's own rows must be PRESENT, so an all-empty read cannot
// pass for a scoped one.
//
// FAIL-CLOSED BY CONSTRUCTION. Every table CHECKs user_id non-empty (migration
// 0020), and every predicate here binds the viewer's identity, so the zero-value
// viewer — an unresolved identity — matches nothing. That is the safe
// direction (the history.Scope zero-value convention).
package chat

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/eventlog"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/local"
	"github.com/darian-gajgic/Sinet-Agentic-Control-Hub/internal/storage"
)

// Event types minted by this package (registered in internal/eventlog's v0
// contract under family 12, Platform — the broadened asset-lifecycle scope,
// CONVENTIONS §29). Payloads are refs-not-blobs: no message body ever rides one.
const (
	EventSessionCreated = "chat.session_created"
	EventSessionRenamed = "chat.session_renamed"
	EventSessionDeleted = "chat.session_deleted"
	EventTurnStarted    = "chat.turn_started"
	EventTurnSettled    = "chat.turn_settled"
	EventTurnAbandoned  = "chat.turn_abandoned"
	EventFileUploaded   = "chat.file_uploaded"
	EventFileDeleted    = "chat.file_deleted"
)

// Why a turn was abandoned. A hard-stop and a crash both end a turn without
// settling it, but they are different facts and the audit must not blur them:
// one is a person's act, the other is the platform reconciling a window nobody
// closed. Both ride chat.turn_abandoned with the same payload shape.
const (
	ReasonHardStop       = "hard_stop"
	ReasonProcessRestart = "process_restart"
)

const chatEventSchemaVersion = 1

// The structural bounds of this family. NONE of them is a ⚙ setting: S15.7 names
// no key, the S18 registry declares none, and inventing one is barred
// (CONVENTIONS §41/§42 — a structural constant with a named reason instead,
// interim under the standing settings-tab directive).
const (
	// SessionListCap bounds one person's session list. A household member
	// accumulates conversations over months; the list is a page they scroll,
	// not an archive they page through, and an unbounded read shares the
	// control plane's ONE writer connection (S02.1).
	SessionListCap = 200
	// TranscriptCap bounds one session's messages and turns. A conversation
	// that long has stopped being a conversation; the bound exists because
	// expectation is not a bound (the §38 D8 reading).
	TranscriptCap = 500
	// MaxMessageRunes bounds a typed message. It is generous for anything a
	// person types at a composer and far below the 1 MiB transport bound the
	// body already passes — the point is that a message is speech, not a file
	// upload, and the exchange is where files go.
	MaxMessageRunes = 8000
	// MaxTitleRunes bounds a session label, whoever set it. A title is a list
	// row's worth of text; the duty is asked for one line and a human rename is
	// held to the same shape so the list cannot be made unreadable.
	MaxTitleRunes = 80
)

// Errors this package returns. The transport maps them by TYPE, never by
// message text (CONVENTIONS §38).
var (
	// ErrNotFound covers "no such row" AND "not visible to this viewer" — one
	// error, deliberately: an id that answers differently for a stranger than
	// for a non-existent row is an existence oracle for someone else's
	// conversation (the 404-before-403 discipline, §39/§40).
	ErrNotFound = errors.New("chat: not found")
	// ErrInvalid is a malformed or out-of-bounds input.
	ErrInvalid = errors.New("chat: invalid")
	// ErrTooLarge / ErrTooMany are the exchange's structural bounds refusing.
	ErrTooLarge = errors.New("chat: too large")
	ErrTooMany  = errors.New("chat: too many")
	// ErrTurnResolved is a stop or a settle arriving at a turn that already
	// ended. It is not a failure — the caller serves the resolved state back
	// (the S15.2 retry-safety rule).
	ErrTurnResolved = errors.New("chat: turn already resolved")
	// ErrTurnRunning refuses a second concurrent turn in one session: a
	// conversation has one thread, and two turns racing to settle would make
	// the produced-files diff windows overlap with no honest attribution.
	ErrTurnRunning = errors.New("chat: a turn is already running in this session")
)

// The closed turn-kind vocabulary (migration 0020's CHECK, restated here as the
// Go-side gate). The CLIENT routes a turn to one of these (OQ3(a)); NO MODEL
// CLASSIFIES A TURN, because a misfiring classifier could start a task nobody
// asked for or escalate a question to open SQL nobody escalated.
const (
	KindAsk     = "ask"      // Layer 1 intent-filled — the NL floor
	KindView    = "view"     // Layer 0 deterministic named view
	KindQuery   = "query"    // Layer 1 direct canned query
	KindOpenSQL = "open_sql" // Layer 2 — reachable only because the caller named it
	KindTask    = "task"     // the S06 intake handoff
)

// Kinds reports whether a turn kind is in the closed set.
func Kinds(kind string) bool {
	switch kind {
	case KindAsk, KindView, KindQuery, KindOpenSQL, KindTask:
		return true
	}
	return false
}

// Turn states.
const (
	StateRunning   = "running"
	StateSettled   = "settled"
	StateAbandoned = "abandoned"
)

// Outcome kinds.
const (
	OutcomeAnswer = "answer" // a served history.Answer, verbatim
	OutcomeTask   = "task"   // a born task, from the S06 handoff
	OutcomeError  = "error"  // the act failed and the record says so
)

// Title provenance (migration 0020's CHECK).
const (
	TitleUnset      = ""
	TitleMechanical = "mechanical"
	TitleDuty       = "duty"
	TitleHuman      = "human"
)

// Session is one conversation's registry row.
type Session struct {
	ID    string `json:"session_id"`
	Owner string `json:"owner"`
	// Title is EMPTY until the first turn names it. That emptiness is the
	// honest untitled state a surface renders; it is never a placeholder.
	Title       string `json:"title"`
	TitleSource string `json:"title_source"`
	CreatedTS   string `json:"created_ts"`
	UpdatedTS   string `json:"updated_ts"`
}

// Message is one immutable transcript entry.
type Message struct {
	ID        string `json:"message_id"`
	SessionID string `json:"session_id"`
	Owner     string `json:"owner"`
	Role      string `json:"role"`
	Body      string `json:"body"`
	TurnID    string `json:"turn_id"`
	CreatedTS string `json:"created_ts"`
}

// Turn is one exchange's lifecycle record: which verb the client routed it to,
// whether it is still running, and what it produced.
type Turn struct {
	ID        string `json:"turn_id"`
	SessionID string `json:"session_id"`
	Owner     string `json:"owner"`
	Kind      string `json:"kind"`
	State     string `json:"state"`
	// OutcomeKind + Outcome are what the turn produced. Outcome is served
	// VERBATIM as the store made it — for a ladder turn it is the
	// history.Answer, so nothing on the way to a screen re-derives a
	// confidence or restyles a lower-confidence flag (G3 D3.5).
	OutcomeKind string          `json:"outcome_kind,omitempty"`
	Outcome     json.RawMessage `json:"outcome,omitempty"`
	// Produced is the exchange-manifest diff attributed to this turn (OQ7).
	Produced  []string `json:"produced,omitempty"`
	StartedTS string   `json:"started_ts"`
	SettledTS string   `json:"settled_ts,omitempty"`

	// exchangeSeq is the manifest watermark read when the turn opened. It is
	// internal machinery for the produced-files window (exchange.go) and not a
	// fact about the conversation, so it stays off the wire.
	exchangeSeq int64
}

// Store is the whole surface. It is nil-safe in exactly one direction: a nil
// *Store is what the shell composes when the family is unwired, and the
// transport answers 503 rather than calling into it.
type Store struct {
	db  *storage.DB
	log *eventlog.Log
	// root is the exchange home: <root>/<owner-digest>/<file-id>. See
	// exchange.go for why the layout is opaque.
	root string
	// duty + advisory are the first-message auto-titling leg (S15.7; S12.4
	// `utility`). Both nil-safe: with no stack the title falls back to the
	// deterministic mechanical derivation, which is the honest degradation.
	duty     *local.Duty
	advisory AdvisoryRun
	// flight is the in-process registry of running turns' cancel funcs, so the
	// hard-stop verb can cancel the request driving a turn (turns.go).
	flight inflight

	// Now is the clock seam (the review.Store / settings.Registry precedent):
	// a row minted through a REAL verb has to be reproducible for a golden
	// fixture, and a wall-clock stamp inside the mint makes it not.
	Now func() time.Time
	// NewID is the identity seam, and it exists for the SAME reason Now does.
	// Ids here are minted rather than caller-supplied, so a golden fixture
	// produced through the real verbs would otherwise carry a fresh random
	// string on every run — a fixture that churns is a fixture nobody reviews.
	// nil = the crypto/rand generator below, so production behavior is
	// unchanged and no caller can choose an id.
	NewID func(prefix string) string
}

// AdvisoryRun opens the platform-scope advisory run a duty call meters against
// and returns its id plus a settle func. It is a SEAM composed at the shell
// root, exactly as internal/history's is: the S12.1 R18 rule is that every
// local call writes one D7 checkpoint on a consuming run, and naming a chat
// session has no run of its own. internal/chat cannot import internal/stage
// (the import wall), so the shell supplies stage.BeginAdvisory here.
//
// nil is an HONEST state, not a failure: with no advisory run the titling leg
// is unavailable and the mechanical title stands.
type AdvisoryRun func(ctx context.Context, label string) (runID string, settle func())

// Config assembles a Store.
type Config struct {
	DB  *storage.DB
	Log *eventlog.Log
	// Root is the exchange home under the platform data root. It is never
	// inside a repo or workspace lowerdir: the exchange is control-plane
	// territory and no sandbox mounts it (Spec S11.3 allowlist-only).
	Root string
	// Duty is the local-tier caller behind auto-titling. Nil-safe by
	// construction (*local.Duty's own nil receiver yields ErrStackAbsent).
	Duty *local.Duty
	// Advisory opens the metering run for that call. nil = no duty leg.
	Advisory AdvisoryRun
	Now      func() time.Time
	NewID    func(prefix string) string
}

// New opens the chat store.
func New(cfg Config) (*Store, error) {
	if cfg.DB == nil || cfg.Log == nil {
		return nil, fmt.Errorf("chat: store needs a db and an event log")
	}
	if cfg.Root == "" {
		return nil, fmt.Errorf("chat: store needs an exchange root (Spec S15.7)")
	}
	return &Store{db: cfg.DB, log: cfg.Log, root: cfg.Root,
		duty: cfg.Duty, advisory: cfg.Advisory, Now: cfg.Now, NewID: cfg.NewID}, nil
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// stamp renders the clock as the UTC RFC3339 text every *_ts column stores.
// Rendering UTC at the single point of writing is what keeps the lexicographic
// comparisons the read predicates make honest (the §38 D1 reading).
func (s *Store) stamp() string { return s.now().UTC().Format(time.RFC3339) }

// newID mints an opaque prefixed id through the seam, or through crypto/rand.
func (s *Store) newID(prefix string) string {
	if s.NewID != nil {
		return s.NewID(prefix)
	}
	return randomID(prefix)
}

// randomID mints an opaque prefixed id. crypto/rand because an exchange file id
// is also its filesystem name (exchange.go) and a guessable name would be a way
// to probe for another owner's object even behind the manifest check.
func randomID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on any platform this runs on; if it ever
		// did, a predictable id is worse than a loud stop.
		panic("chat: crypto/rand: " + err.Error())
	}
	return prefix + hex.EncodeToString(b[:])
}

// ---- Sessions ----

const sessionColumns = `session_id, user_id, title, title_source, created_ts, updated_ts`

func scanSession(r interface{ Scan(...any) error }) (Session, error) {
	var v Session
	err := r.Scan(&v.ID, &v.Owner, &v.Title, &v.TitleSource, &v.CreatedTS, &v.UpdatedTS)
	return v, err
}

// CreateSession opens a session for one person. The title is deliberately EMPTY:
// a session is named by its first turn (or by its owner), and a placeholder
// standing in for a name nobody chose is exactly the fabrication the
// honest-absence rule bars.
func (s *Store) CreateSession(ctx context.Context, owner string) (Session, error) {
	if owner == "" {
		return Session{}, fmt.Errorf("%w: a session needs an owner (15.6)", ErrInvalid)
	}
	now := s.stamp()
	v := Session{ID: s.newID("cs-"), Owner: owner, CreatedTS: now, UpdatedTS: now}
	payload, err := json.Marshal(struct {
		Session string `json:"session"`
		Owner   string `json:"owner"`
	}{v.ID, owner})
	if err != nil {
		return Session{}, fmt.Errorf("chat: encode %s payload: %w", EventSessionCreated, err)
	}
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO chat_sessions (session_id, user_id, title, title_source, created_ts, updated_ts)
			 VALUES (?, ?, '', '', ?, ?)`, v.ID, owner, now, now); err != nil {
			return fmt.Errorf("chat: insert session: %w", err)
		}
		_, err := s.log.AppendTx(ctx, tx, eventlog.Append{
			UserID: owner, Type: EventSessionCreated,
			SchemaVersion: chatEventSchemaVersion, Payload: payload,
		})
		return err
	})
	if err != nil {
		return Session{}, err
	}
	return v, nil
}

// ListSessions serves one person's sessions, newest first, bounded. There is no
// viewer role parameter: see the package note.
func (s *Store) ListSessions(ctx context.Context, viewer string) ([]Session, error) {
	if viewer == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sessionColumns+` FROM chat_sessions
		  WHERE user_id = ? ORDER BY created_ts DESC, session_id DESC LIMIT ?`,
		viewer, SessionListCap)
	if err != nil {
		return nil, fmt.Errorf("chat: list sessions: %w", err)
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		v, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("chat: scan session: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Session returns one session IF the viewer owns it, and ErrNotFound otherwise —
// one answer for "no such row" and "not yours", so an id cannot be used to probe
// for another person's conversation.
func (s *Store) Session(ctx context.Context, viewer, id string) (Session, error) {
	if viewer == "" || id == "" {
		return Session{}, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+sessionColumns+` FROM chat_sessions WHERE session_id = ? AND user_id = ?`, id, viewer)
	v, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("chat: read session: %w", err)
	}
	return v, nil
}

// RenameSession is the human override of an auto-derived title (S15.7: the
// title is data a human may edit). It stamps title_source='human', which is what
// makes the titling duty's own rule — never overwrite a human-set title —
// enforceable rather than remembered.
func (s *Store) RenameSession(ctx context.Context, viewer, id, title string) (Session, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Session{}, fmt.Errorf("%w: a rename needs a title", ErrInvalid)
	}
	if utf8.RuneCountInString(title) > MaxTitleRunes {
		return Session{}, fmt.Errorf("%w: title exceeds %d characters", ErrInvalid, MaxTitleRunes)
	}
	if _, err := s.Session(ctx, viewer, id); err != nil {
		return Session{}, err
	}
	payload, err := json.Marshal(struct {
		Session string `json:"session"`
		Source  string `json:"source"`
	}{id, TitleHuman})
	if err != nil {
		return Session{}, fmt.Errorf("chat: encode %s payload: %w", EventSessionRenamed, err)
	}
	now := s.stamp()
	err = s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE chat_sessions SET title = ?, title_source = ?, updated_ts = ?
			  WHERE session_id = ? AND user_id = ?`,
			title, TitleHuman, now, id, viewer); err != nil {
			return fmt.Errorf("chat: rename session: %w", err)
		}
		_, err := s.log.AppendTx(ctx, tx, eventlog.Append{
			UserID: viewer, Type: EventSessionRenamed,
			SchemaVersion: chatEventSchemaVersion, Payload: payload,
		})
		return err
	})
	if err != nil {
		return Session{}, err
	}
	return s.Session(ctx, viewer, id)
}

// DeleteResult reports what a hard delete removed.
type DeleteResult struct {
	SessionID string `json:"session_id"`
	Messages  int64  `json:"messages"`
	Turns     int64  `json:"turns"`
	// Applied is false on a repeat: the session was already gone and the
	// already-resolved state is served rather than a refusal (S15.2
	// retry-safety, the landed applied:false shape).
	Applied bool `json:"applied"`
}

// DeleteSession is the owner-only HARD delete (OQ1): the rows go. The minted
// event survives as the audit of what was removed — a delete whose record
// disappears with the data cannot be reviewed. It carries counts, never content.
func (s *Store) DeleteSession(ctx context.Context, viewer, id string) (DeleteResult, error) {
	if _, err := s.Session(ctx, viewer, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			return DeleteResult{SessionID: id, Applied: false}, nil
		}
		return DeleteResult{}, err
	}
	res := DeleteResult{SessionID: id, Applied: true}
	err := s.db.WriteTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM chat_messages WHERE session_id = ?`, id).Scan(&res.Messages); err != nil {
			return fmt.Errorf("chat: count messages: %w", err)
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM chat_turns WHERE session_id = ?`, id).Scan(&res.Turns); err != nil {
			return fmt.Errorf("chat: count turns: %w", err)
		}
		// The FK cascade is declared, but foreign_keys enforcement is a
		// connection pragma this package does not own, so the children are
		// deleted explicitly. Belt over a declared brace: a transcript that
		// outlived its session would be unreachable rows nobody could see or
		// remove.
		if _, err := tx.ExecContext(ctx, `DELETE FROM chat_messages WHERE session_id = ?`, id); err != nil {
			return fmt.Errorf("chat: delete messages: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM chat_turns WHERE session_id = ?`, id); err != nil {
			return fmt.Errorf("chat: delete turns: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM chat_sessions WHERE session_id = ? AND user_id = ?`, id, viewer); err != nil {
			return fmt.Errorf("chat: delete session: %w", err)
		}
		payload, err := json.Marshal(struct {
			Session  string `json:"session"`
			Messages int64  `json:"messages"`
			Turns    int64  `json:"turns"`
		}{id, res.Messages, res.Turns})
		if err != nil {
			return fmt.Errorf("chat: encode %s payload: %w", EventSessionDeleted, err)
		}
		_, err = s.log.AppendTx(ctx, tx, eventlog.Append{
			UserID: viewer, Type: EventSessionDeleted,
			SchemaVersion: chatEventSchemaVersion, Payload: payload,
		})
		return err
	})
	if err != nil {
		return DeleteResult{}, err
	}
	return res, nil
}

// Messages serves one session's transcript, oldest first, bounded.
func (s *Store) Messages(ctx context.Context, viewer, sessionID string) ([]Message, error) {
	if _, err := s.Session(ctx, viewer, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT message_id, session_id, user_id, role, body, turn_id, created_ts
		   FROM chat_messages WHERE session_id = ? AND user_id = ?
		  ORDER BY created_ts, message_id LIMIT ?`, sessionID, viewer, TranscriptCap)
	if err != nil {
		return nil, fmt.Errorf("chat: list messages: %w", err)
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Owner, &m.Role, &m.Body, &m.TurnID, &m.CreatedTS); err != nil {
			return nil, fmt.Errorf("chat: scan message: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

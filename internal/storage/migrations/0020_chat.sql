-- 0020_chat.sql — the durable home of the S15.7 conversational assistant: its
-- per-user session registry, the immutable transcript, the turn lifecycle and
-- the JARVIS file-exchange manifest.
--
-- Exact DDL per the S02.2 schema-workshop mandate; applied in ONE transaction
-- with its PRAGMA user_version bump by the migration runner
-- (internal/storage/migrate.go, Spec S02.1). A committed migration is immutable
-- (CONVENTIONS §6) — 0001–0019 stay byte-untouched, this file included once it
-- lands.
--
-- WHY TABLES AND NOT DERIVE-FROM-LOG (P3-B6-7, OQ1(a)). S15.7 demands a
-- "server-side per-user session registry, fail-closed" and message/session state
-- that is platform-owned, so a widget swap loses no data and a reload restores
-- the transcript from SERVED state. Message bodies are CONTENT — the same class
-- as an ask snapshot, a knowledge entry or a deliverable revision, every one of
-- which is table-backed — and putting them in event payloads would break
-- refs-not-blobs and turn every transcript read into a log scan. The events this
-- family mints therefore carry LIFECYCLE FACTS with refs, never the bodies
-- (CONVENTIONS §29; Spec S15.2 rule 3).
--
-- FOUR TABLES, and the split between messages and turns is load-bearing:
--
--   chat_sessions       — the registry. Owner-attributed (15.6), hard-deletable
--                         by its owner; the minted lifecycle events survive the
--                         delete and are the audit of what was removed.
--   chat_messages       — the transcript. INSERT-ONLY and never edited: a
--                         message is what a person said, and a record of speech
--                         that can be rewritten is not a record. A session
--                         delete removes them wholesale (below) — immutable
--                         means never EDITED, not never removed.
--   chat_turns          — the turn lifecycle. Deliberately NOT folded into
--                         chat_messages even though a turn is 1:1 with the
--                         message that opened it: a turn row MUTATES
--                         (running → settled | abandoned) and a message row
--                         must not, so folding them would force the
--                         immutability trigger to be column-selective — the
--                         kind of guard that rots silently. Two lifecycles,
--                         two tables, two clean triggers.
--   chat_exchange_files — the exchange MANIFEST, which is the record; the
--                         filesystem underneath is storage. A directory listing
--                         cannot say WHO owns a byte string, so ownership lives
--                         here as a column and the store reads files only
--                         through its own confined path builder (S11.7
--                         resolve-then-deny; Spec S15.7 "owner-attributed
--                         artifacts, fail-closed on ownership").
--
-- OWNER-SCOPE IS FAIL-CLOSED BY CONSTRUCTION. Every table CHECKs user_id
-- non-empty, exactly as 0001 does, so the history.Scope convention — the
-- operator binds the empty string, a member binds their identity, the zero value
-- matches nothing — can never collide with a real owner. Chat visibility is
-- narrower than the observability rule: see the OWNER-ONLY note in
-- internal/chat/chat.go (P3-B6-7 OQ1, the §40-C content-vs-telemetry line).
--
-- NO MONEY, NO PERCENT, NO ESTIMATE is stored, computed or derived in this file.

CREATE TABLE chat_sessions (
    session_id   TEXT PRIMARY KEY,
    -- The owner. Every record carries who it belongs to (feature 15.6).
    user_id      TEXT NOT NULL CHECK (user_id <> ''),
    -- title is the human-readable label. It is EMPTY until the session's first
    -- turn names it, and that emptiness is the honest untitled state the surface
    -- renders — never a placeholder standing in for a name nobody chose.
    title        TEXT NOT NULL DEFAULT '',
    -- title_source says where the label came from, because a label a model
    -- drafted and a label a person typed are different facts and the surface
    -- must be able to tell them apart. 'mechanical' is the deterministic
    -- fallback (a truncation of the first message) taken whenever the duty is
    -- unavailable or refuses — a mechanical derivation, never a fabricated
    -- model output (Spec S12.4; CONVENTIONS §37 honest-absence).
    title_source TEXT NOT NULL DEFAULT ''
                 CHECK (title_source IN ('', 'mechanical', 'duty', 'human')),
    created_ts   TEXT NOT NULL,
    updated_ts   TEXT NOT NULL
);

-- The list read: one owner's sessions, newest first.
CREATE INDEX chat_sessions_owner_idx ON chat_sessions (user_id, created_ts);

CREATE TABLE chat_messages (
    message_id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES chat_sessions (session_id) ON DELETE CASCADE,
    -- Denormalized owner: the scope predicate must not depend on a join
    -- surviving a session delete, and 15.6 puts the owner on the RECORD.
    user_id    TEXT NOT NULL CHECK (user_id <> ''),
    -- role is a closed set with ONE member at v0. The assistant produces no
    -- PROSE here: its side of an exchange is the turn's structured outcome — an
    -- answer, a card, a refusal, a born task — rendered as data (OQ10). A row
    -- holding a sentence the platform composed to look like speech is exactly
    -- the fabrication the honest-absence rule bars, so no such row is written.
    -- An assistant-prose producer would widen this CHECK in its own migration,
    -- which is a visible act rather than a silent one.
    role       TEXT NOT NULL CHECK (role IN ('user')),
    body       TEXT NOT NULL,
    -- The turn this message opened (chat_turns.turn_id). Not a foreign key: the
    -- message is written first, inside the same transaction that opens the turn.
    turn_id    TEXT NOT NULL CHECK (turn_id <> ''),
    created_ts TEXT NOT NULL
);

CREATE INDEX chat_messages_session_idx ON chat_messages (session_id, created_ts, message_id);

CREATE TRIGGER chat_messages_immutable
    BEFORE UPDATE ON chat_messages
BEGIN
    SELECT RAISE(ABORT, 'a chat message is immutable: it records what a person said, and a transcript that can be rewritten is not a record (Spec S15.7)');
END;

CREATE TABLE chat_turns (
    turn_id     TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES chat_sessions (session_id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL CHECK (user_id <> ''),
    -- kind is the verb the CLIENT routed this turn to, from a closed vocabulary
    -- (OQ3(a)): the three S15.7 verbs, with the query ladder's own layers named
    -- so the record says which layer answered. No model classifies a turn — a
    -- misfiring classifier could start a task nobody asked for.
    --   ask      → Layer 1 intent-filled (the NL floor; its own card is the
    --              honest refusal when nothing resolves)
    --   view     → Layer 0 deterministic named view
    --   query    → Layer 1 direct canned query
    --   open_sql → Layer 2, reachable ONLY because the caller named it
    --   task     → the S06 intake handoff
    kind        TEXT NOT NULL CHECK (kind IN ('ask', 'view', 'query', 'open_sql', 'task')),
    state       TEXT NOT NULL CHECK (state IN ('running', 'settled', 'abandoned')),
    -- outcome_kind + outcome record what the turn produced. outcome is opaque
    -- JSON read back whole: for a ladder turn it is the history.Answer the store
    -- made, VERBATIM, so nothing re-derives a confidence or restyles a
    -- lower-confidence flag on the way to a screen (G3 D3.5).
    outcome_kind TEXT NOT NULL DEFAULT ''
                 CHECK (outcome_kind IN ('', 'answer', 'task', 'error')),
    outcome     TEXT NOT NULL DEFAULT '',
    -- produced is the JSON array of exchange-manifest ids that appeared in this
    -- owner's folder while the turn ran (OQ7(a)) — a DIFF over stored rows,
    -- computed once at settle. No watcher, no ticker (§32).
    produced    TEXT NOT NULL DEFAULT '',
    -- exchange_seq is the manifest WATERMARK read when the turn opened: the
    -- highest chat_exchange_files.seq that already existed. The window diff is
    -- `seq > exchange_seq`, which is why a file that was already there can
    -- never be reported as produced BY this turn. A timestamp cannot carry that
    -- guarantee: *_ts is second-resolution RFC3339, so an upload landing in the
    -- same second as a turn's start is indistinguishable from one landing
    -- during it, and attributing it would fabricate authorship.
    exchange_seq INTEGER NOT NULL DEFAULT 0,
    started_ts  TEXT NOT NULL,
    settled_ts  TEXT NOT NULL DEFAULT ''
);

CREATE INDEX chat_turns_session_idx ON chat_turns (session_id, started_ts, turn_id);

CREATE TABLE chat_exchange_files (
    -- seq is the manifest's MONOTONIC row identity, and it is the ordering the
    -- produced-files window is computed over. AUTOINCREMENT rather than the
    -- implicit rowid: a plain rowid is reused after the highest row is deleted,
    -- and this table's rows ARE deleted, so a reused identity could place a new
    -- object BEFORE a turn that started after it. Ordering by uploaded_ts
    -- cannot do this job at all — the stamp is second-resolution.
    seq         INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id     TEXT NOT NULL UNIQUE,
    user_id     TEXT NOT NULL CHECK (user_id <> ''),
    -- name is the ORIGINAL filename, kept as the label only. It is never a path
    -- component: the bytes are stored under the opaque file_id, so a hostile
    -- name cannot reach the filesystem at all (see internal/chat/exchange.go).
    name        TEXT NOT NULL CHECK (name <> ''),
    size_bytes  INTEGER NOT NULL CHECK (size_bytes >= 0),
    sha256      TEXT NOT NULL CHECK (sha256 <> ''),
    -- Origin attribution: which session and turn this arrived on ('' when it
    -- arrived outside any turn). It is what lets the produced-files diff say
    -- WHICH turn a late-arriving file belongs to instead of guessing (OQ7).
    origin_session TEXT NOT NULL DEFAULT '',
    origin_turn    TEXT NOT NULL DEFAULT '',
    uploaded_ts TEXT NOT NULL
);

CREATE INDEX chat_exchange_owner_idx ON chat_exchange_files (user_id, uploaded_ts, file_id);

CREATE TRIGGER chat_exchange_files_immutable
    BEFORE UPDATE ON chat_exchange_files
BEGIN
    SELECT RAISE(ABORT, 'an exchange manifest row is immutable: it states the size and hash of bytes that were stored, and a stored byte string does not change (Spec S15.7; 15.6)');
END;

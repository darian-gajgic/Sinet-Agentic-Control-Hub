-- 0001_core_schema.sql — the S02.2 owner-attributed table families plus the
-- settings rows and their audit trail (Spec S02.2, S01.9, S01.10).
--
-- Every row carries its owning user_id (feature 15.6) even though v0
-- operates single-user. user_id columns deliberately carry no FOREIGN KEY at
-- B0: the auth stack (Spec S01.9) lands in a later packet and attribution
-- must work from the first boot; run-machinery FKs are declared now.
-- Timestamps are RFC 3339 TEXT and are never an ordering authority —
-- ordering comes only from event_seq (Spec S02.5, P-T07-4).

-- users: identity + per-person credential-store refs (15.6, 10.x, D2).
-- One role bit, operator vs member, implements D10 co-approval (Spec S01.9).
CREATE TABLE users (
    user_id              TEXT PRIMARY KEY CHECK (user_id <> ''),
    display_name         TEXT NOT NULL DEFAULT '',
    role                 TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('operator', 'member')),
    credential_store_ref TEXT NOT NULL DEFAULT '',
    created_ts           TEXT NOT NULL
);

-- tasks: user-facing task + kanban status, orthogonal to run machinery
-- (9.1, S1.3).
CREATE TABLE tasks (
    task_id       TEXT PRIMARY KEY CHECK (task_id <> ''),
    user_id       TEXT NOT NULL CHECK (user_id <> ''),
    title         TEXT NOT NULL DEFAULT '',
    kanban_status TEXT NOT NULL DEFAULT '',
    created_ts    TEXT NOT NULL
);

-- runs: the run-lifecycle FSM row (Spec S02.2, S02.3). Stored states only —
-- stalled/wedged are derived at reconcile, never stored. The lease is
-- wall-clock DB columns evaluated suspend-aware; generation is the per-run
-- fencing counter stamped into every event append (Spec S02.5, D6).
CREATE TABLE runs (
    run_id              TEXT PRIMARY KEY CHECK (run_id <> ''),
    user_id             TEXT NOT NULL CHECK (user_id <> ''),
    task_id             TEXT REFERENCES tasks (task_id),
    state               TEXT NOT NULL CHECK (state IN (
                            'new', 'queued', 'claimed', 'running', 'parked',
                            'draining', 'completed', 'crashed', 'finalized',
                            'tombstoned', 'died-at-gate')),
    substrate           TEXT NOT NULL DEFAULT '',
    lane                TEXT NOT NULL DEFAULT '',
    lease_holder        TEXT,
    lease_deadline_ts   TEXT,
    heartbeat_event_seq INTEGER,
    generation          INTEGER NOT NULL DEFAULT 0 CHECK (generation >= 0),
    unit_name           TEXT,
    workspace_ref       TEXT,
    ceiling_time_s      INTEGER,
    ceiling_steps       INTEGER,
    ceiling_cost_usd    REAL,
    created_ts          TEXT NOT NULL,
    updated_ts          TEXT NOT NULL
);

-- run_events: THE append-only event log — the D7 event record, the
-- observability substrate, and the platform's only audit truth (Spec S02.2,
-- S01.11; journald is ops-only). event_seq is the sole ordering authority.
-- run_id is NULL for platform-scope events (settings, auth, lifecycle);
-- run-scoped events carry the run's generation for fencing.
CREATE TABLE run_events (
    event_seq      INTEGER PRIMARY KEY,
    run_id         TEXT REFERENCES runs (run_id),
    generation     INTEGER CHECK (generation IS NULL OR generation >= 0),
    user_id        TEXT NOT NULL CHECK (user_id <> ''),
    type           TEXT NOT NULL CHECK (type <> ''),
    schema_version INTEGER NOT NULL CHECK (schema_version >= 1),
    payload        TEXT NOT NULL,
    ts             TEXT NOT NULL,
    CHECK ((run_id IS NULL) = (generation IS NULL))
);

CREATE INDEX run_events_run_idx ON run_events (run_id, event_seq)
    WHERE run_id IS NOT NULL;

-- History is NEVER rewritten — append-only (Spec S02.1).
CREATE TRIGGER run_events_no_update
    BEFORE UPDATE ON run_events
BEGIN
    SELECT RAISE(ABORT, 'run_events is append-only (Spec S02.1)');
END;

CREATE TRIGGER run_events_no_delete
    BEFORE DELETE ON run_events
BEGIN
    SELECT RAISE(ABORT, 'run_events is append-only (Spec S02.1)');
END;

-- checkpoints: one row per paid model call, written in the same transaction
-- as its run-event append (Spec S02.4, D7). Blocks: (a) usage, (b) session
-- cursor, (c) Ledger revision, (d) artifact snapshot ref, (e) version
-- fields for the freshness pass.
CREATE TABLE checkpoints (
    checkpoint_id           INTEGER PRIMARY KEY,
    run_id                  TEXT NOT NULL REFERENCES runs (run_id),
    user_id                 TEXT NOT NULL CHECK (user_id <> ''),
    event_seq               INTEGER NOT NULL REFERENCES run_events (event_seq),
    usage_json              TEXT NOT NULL DEFAULT '{}',
    session_substrate       TEXT NOT NULL DEFAULT '',
    session_id              TEXT NOT NULL DEFAULT '',
    message_index           INTEGER,
    cwd_key                 TEXT NOT NULL DEFAULT '',
    transcript_path         TEXT NOT NULL DEFAULT '',
    ledger_revision         TEXT NOT NULL DEFAULT '',
    artifact_snapshot_ref   TEXT NOT NULL DEFAULT '',
    model_id                TEXT NOT NULL DEFAULT '',
    invocation_fingerprint  TEXT NOT NULL DEFAULT '',
    tool_schema_version     TEXT NOT NULL DEFAULT '',
    prompt_schema_version   TEXT NOT NULL DEFAULT '',
    created_ts              TEXT NOT NULL
);

CREATE INDEX checkpoints_run_idx ON checkpoints (run_id, checkpoint_id);

-- asks: every gate/question the moment observed, with the full
-- invocation-reconstruction snapshot as the resume input (Spec S02.2,
-- S02.5 step 5; engine-side asks are volatile, these rows are
-- authoritative).
CREATE TABLE asks (
    ask_id           TEXT PRIMARY KEY CHECK (ask_id <> ''),
    run_id           TEXT NOT NULL REFERENCES runs (run_id),
    user_id          TEXT NOT NULL CHECK (user_id <> ''),
    snapshot         TEXT NOT NULL,
    status           TEXT NOT NULL CHECK (status <> ''),
    observed_ts      TEXT NOT NULL,
    answered_ts      TEXT,
    answer           TEXT,
    engine_expiry_ts TEXT
);

CREATE INDEX asks_status_idx ON asks (status);

-- effects: the two-phase effect journal (Spec S02.7). The row is written
-- 'executing' BEFORE the provider call; idempotency_key is the effect UUID;
-- 'unknown' is a first-class terminal for idempotency-less channels
-- (P-T07-3).
CREATE TABLE effects (
    effect_id           TEXT PRIMARY KEY CHECK (effect_id <> ''),
    run_id              TEXT REFERENCES runs (run_id),
    user_id             TEXT NOT NULL CHECK (user_id <> ''),
    class               TEXT NOT NULL CHECK (class IN ('A', 'B', 'C', 'D')),
    payload             TEXT NOT NULL,
    payload_hash        TEXT NOT NULL CHECK (payload_hash <> ''),
    state               TEXT NOT NULL CHECK (state IN (
                            'proposed', 'approved', 'executing',
                            'succeeded', 'failed', 'unknown')),
    approved_by         TEXT,
    approved_ts         TEXT,
    idempotency_key     TEXT,
    provider_window_ref TEXT,
    attempts            INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    result              TEXT,
    created_ts          TEXT NOT NULL,
    updated_ts          TEXT NOT NULL
);

CREATE INDEX effects_state_idx ON effects (state);

-- queue: CAS claiming — status, claimed_by, lease columns, priority lane
-- (Spec S02.2; claiming machinery is the scheduler's, Spec S10).
CREATE TABLE queue (
    queue_id          INTEGER PRIMARY KEY,
    run_id            TEXT NOT NULL REFERENCES runs (run_id),
    user_id           TEXT NOT NULL CHECK (user_id <> ''),
    status            TEXT NOT NULL CHECK (status <> ''),
    claimed_by        TEXT,
    lease_deadline_ts TEXT,
    priority_lane     TEXT NOT NULL DEFAULT '',
    enqueued_ts       TEXT NOT NULL
);

CREATE INDEX queue_claim_idx ON queue (status, priority_lane);

-- lanes: per-(user, model/lane) concurrency caps as data (3.11, D4).
CREATE TABLE lanes (
    user_id         TEXT NOT NULL CHECK (user_id <> ''),
    lane            TEXT NOT NULL CHECK (lane <> ''),
    concurrency_cap INTEGER NOT NULL CHECK (concurrency_cap >= 0),
    PRIMARY KEY (user_id, lane)
);

-- artifact_claims: the S1.11 write-set registry — path/glob sets declared
-- at plan time, glob-intersected at admission (Spec S02.8).
CREATE TABLE artifact_claims (
    claim_id         INTEGER PRIMARY KEY,
    task_id          TEXT NOT NULL REFERENCES tasks (task_id),
    project          TEXT NOT NULL DEFAULT '',
    user_id          TEXT NOT NULL CHECK (user_id <> ''),
    path_globs       TEXT NOT NULL,
    mode             TEXT NOT NULL CHECK (mode IN ('R', 'W')),
    declared_at_plan TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL CHECK (status <> ''),
    created_ts       TEXT NOT NULL
);

-- engine_sessions: session registry including the copied-aside transcript
-- path — the reboot-case mining index (Spec S02.4, P-T07-2).
CREATE TABLE engine_sessions (
    session_key          INTEGER PRIMARY KEY,
    run_id               TEXT NOT NULL REFERENCES runs (run_id),
    user_id              TEXT NOT NULL CHECK (user_id <> ''),
    substrate            TEXT NOT NULL DEFAULT '',
    engine_session_id    TEXT NOT NULL DEFAULT '',
    transcript_copy_path TEXT NOT NULL DEFAULT '',
    created_ts           TEXT NOT NULL,
    updated_ts           TEXT NOT NULL
);

-- receipts: derived from checkpoint usage rows, materialized per run-end
-- (Spec S02.2; the materialization design is Spec S10's).
CREATE TABLE receipts (
    receipt_id      INTEGER PRIMARY KEY,
    run_id          TEXT NOT NULL REFERENCES runs (run_id),
    user_id         TEXT NOT NULL CHECK (user_id <> ''),
    usage_json      TEXT NOT NULL DEFAULT '{}',
    materialized_ts TEXT NOT NULL
);

-- settings: override rows only — the default lives in code and
-- reset-to-default deletes the row (Spec S01.10). user_id '' is the
-- platform scope; per-user rows exist only for per-user-scoped keys and
-- never carry bounds. floor/ceiling are the operator-edited bounds of G1
-- rider 1; NULL means the declared clamp applies.
CREATE TABLE settings (
    key        TEXT NOT NULL CHECK (key <> ''),
    user_id    TEXT NOT NULL DEFAULT '',
    value      TEXT,
    floor      REAL,
    ceiling    REAL,
    updated_ts TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    PRIMARY KEY (key, user_id),
    CHECK (value IS NOT NULL OR floor IS NOT NULL OR ceiling IS NOT NULL),
    CHECK (floor IS NULL OR ceiling IS NULL OR floor <= ceiling),
    CHECK (user_id = '' OR (floor IS NULL AND ceiling IS NULL))
);

-- settings_events: the per-write audit trail {actor, key, old, new,
-- timestamp, reason} (Spec S01.10; G3 Def.2). Append-only like all history.
CREATE TABLE settings_events (
    settings_event_id INTEGER PRIMARY KEY,
    actor             TEXT NOT NULL CHECK (actor <> ''),
    key               TEXT NOT NULL CHECK (key <> ''),
    user_id           TEXT NOT NULL DEFAULT '',
    old               TEXT,
    new               TEXT,
    ts                TEXT NOT NULL,
    reason            TEXT NOT NULL DEFAULT ''
);

CREATE INDEX settings_events_key_idx ON settings_events (key);

CREATE TRIGGER settings_events_no_update
    BEFORE UPDATE ON settings_events
BEGIN
    SELECT RAISE(ABORT, 'settings_events is append-only (Spec S01.10)');
END;

CREATE TRIGGER settings_events_no_delete
    BEFORE DELETE ON settings_events
BEGIN
    SELECT RAISE(ABORT, 'settings_events is append-only (Spec S01.10)');
END;

-- 0015_retention_history.sql — the Spec S14.9 retention substrate plus the
-- S14.10 indexing layer (DDL detail is P3's, S02.2).
--
-- What lands here:
--   A. run_events: the NARROWED append-only trigger set. S02.1's "history is
--      NEVER rewritten" stays checkable; S14.9's mandated payload strip becomes
--      executable — and nothing else does.
--   B. run_events: generated columns over event-JSON hot fields, a
--      dotted_order-style ordered-tree path, and the indexes that turn the
--      B5-3/B5-2 derive queries from full scans into index seeks (the F16 carry).
--   C. run_summaries: the S14.9 run-summary record written at run end.
--   D. retention_keep_forever + run_events_export: the S14.9 keep-forever
--      allowlist and the 11.3 exporter boundary BY CONSTRUCTION.
--   E. history_fts + run_event_rollup + retention_index_cursor: the S14.10
--      FTS5 and rollup layer, both derived state rebuildable from the log.
--   F. benchmark_pairs: the B5-7 §35 drain D5(b) declined-row freeze.
--
-- Applied in one transaction with its PRAGMA user_version bump by the migration
-- runner (internal/storage/migrate.go, Spec S02.1). A committed migration is
-- immutable (CONVENTIONS §6) — 0001–0014 stay byte-untouched, which is why the
-- trigger change below is a DROP + CREATE here and never an edit to 0001.

-- ── A. The narrowed append-only trigger set on run_events (S02.1 / S14.9) ────
--
-- 0001 locked run_events with RAISE(ABORT) on BOTH UPDATE and DELETE. S14.9
-- mandates that the scheduled compaction pass "strips bulky event payloads",
-- which the blanket UPDATE trigger makes impossible to execute at all. The
-- resolution is to NARROW, never to delete the invariant:
--
--   * run_events_no_delete is UNTOUCHED. A row is never removed — event_seq is
--     the sole ordering authority (S14.1) and checkpoints.event_seq is a FK
--     into this table (0001), so compaction elides BODIES and never rows.
--   * Identity columns stay immutable, absolutely. An UPDATE naming any of
--     event_seq/run_id/generation/user_id/type/schema_version/ts aborts —
--     history is never re-authored, re-owned, re-typed or re-dated.
--   * payload admits EXACTLY ONE transition: an arbitrary body → the single
--     compacted marker, once, and only on a row older than the SMALLEST horizon
--     ⚙ retention.compaction_horizon can ever hold (Min: 1 month, S18). Every
--     other payload write aborts, including marker → anything and a second
--     strip of an already-compacted row.
--
-- The horizon that actually applies is per-user, live-read and months-valued
-- (⚙ retention.compaction_horizon, PerUser) — a SQL trigger cannot read the
-- settings registry, so the trigger enforces the RATIFIED FLOOR of that clamp
-- and the pass enforces the owner's own horizon on top (internal/retention,
-- proven by test). The floor is the limb that can be made structural, and it is.
DROP TRIGGER run_events_no_update;

-- Identity is immutable. BEFORE UPDATE OF fires only when the SET clause names
-- one of the listed columns (the 0014 benchmark_pairs shape), so a payload-only
-- strip passes here and lands on the payload trigger below.
CREATE TRIGGER run_events_identity_immutable
    BEFORE UPDATE OF event_seq, run_id, generation, user_id, type, schema_version, ts
    ON run_events
BEGIN
    SELECT RAISE(ABORT, 'run_events is append-only: identity columns are immutable — history is never rewritten (Spec S02.1)');
END;

-- The payload admits exactly one transition: body → the compacted marker, once,
-- on a row past the ratified horizon floor. The marker literal below is the
-- byte-exact twin of retention.CompactedPayload in Go; a test pins them equal.
CREATE TRIGGER run_events_payload_compaction_only
    BEFORE UPDATE OF payload ON run_events
    WHEN NEW.payload <> '{"compacted":true,"reason":"trace payload stripped past the retention compaction horizon (Spec S14.9)"}'
      OR OLD.payload = '{"compacted":true,"reason":"trace payload stripped past the retention compaction horizon (Spec S14.9)"}'
      OR OLD.ts > strftime('%Y-%m-%dT%H:%M:%S', 'now', '-1 month')
BEGIN
    SELECT RAISE(ABORT, 'run_events is append-only: payload admits only the one-way S14.9 compaction strip, once, past the horizon floor — every other write is a rewrite of history (Spec S02.1)');
END;

-- ── B. Indexing: generated columns + the F16 index discharge (S14.10 ¶4) ─────
--
-- run_events carried EIGHT columns and exactly ONE index (the partial
-- (run_id, event_seq)) before this migration. Every derive query in
-- internal/api/projection.go and internal/watchdog filters `WHERE type = ?`,
-- so each was a full table scan (the B5-3 F16 carry). ALTER TABLE ADD COLUMN
-- admits VIRTUAL generated columns only (never STORED) — which is also the
-- shape we want: no table rewrite, computed on read, indexable.
--
-- Both generated columns read the payload, so both go NULL on a compacted row.
-- That is honest: the hot field genuinely no longer exists there.

-- change_class: the S14.6 drift change class, the one payload field an existing
-- projection filters on (internal/api/projection.go driftCards json_extracts it).
ALTER TABLE run_events ADD COLUMN change_class TEXT
    GENERATED ALWAYS AS (json_extract(payload, '$.change_class')) VIRTUAL;

-- dotted_order: the LangSmith-shaped materialized path for ordered-tree reads.
-- v0 payloads carry NO universal parent linkage — the S04.4 spawn record is the
-- closest thing and it carries a DEPTH, not a parent event or run (helpers run
-- in-process under the parent's own run). So the path is built from the linkage
-- that genuinely exists: the run, the payload's depth where the producer wrote
-- one, and event_seq zero-padded so lexicographic order IS event order. Where
-- there is no depth linkage the path degrades to the run's own path at depth 0
-- — a real absence rendered as the run's own level, never a fabricated parent.
-- A prefix scan on '<run_id>.' reads that run's subtree in order.
ALTER TABLE run_events ADD COLUMN dotted_order TEXT
    GENERATED ALWAYS AS (
        COALESCE(run_id, 'platform') || '.' ||
        printf('%02d', COALESCE(json_extract(payload, '$.depth'), 0)) || '.' ||
        printf('%020d', event_seq)
    ) VIRTUAL;

-- Each index names the query shape it serves; none is speculative.
-- `WHERE type = ? ORDER BY event_seq` — watchdogFlags, benchmarkAlarms,
-- maxSeqByKey, watchdog suppress history, benchmark gate/pair reads.
CREATE INDEX run_events_type_seq_idx ON run_events (type, event_seq);
-- The owner-scoped variant of the same shape (S01.9 scoping is applied to every
-- projection, so the member path must seek too, not just the operator path).
CREATE INDEX run_events_type_user_idx ON run_events (type, user_id, event_seq);
-- `WHERE type = ? AND ts >= ?` — driftCards' bounded window and the watchdog
-- sweep's time-bounded spend read.
CREATE INDEX run_events_type_ts_idx ON run_events (type, ts);
-- `WHERE run_id = ? AND type IN (...)` — the per-run derives (wedged, watchdog
-- open flags, benchmark statement source, run-summary aggregation).
CREATE INDEX run_events_run_type_idx ON run_events (run_id, type, event_seq)
    WHERE run_id IS NOT NULL;
-- `WHERE user_id = ? AND ts <= ?` — the compaction pass's per-owner horizon
-- scan and the S14.10 person + date-range filter axes.
CREATE INDEX run_events_user_ts_idx ON run_events (user_id, ts);
-- The drift change-class axis. Partial: only drift rows carry the field, so the
-- index stays proportional to the drift log, not to the whole event log.
CREATE INDEX run_events_change_class_idx ON run_events (change_class, event_seq)
    WHERE change_class IS NOT NULL;

-- ── C. run_summaries — the S14.9 run summary (11.1) ──────────────────────────
--
-- Written at the run's TERMINAL transition, in the same transaction as the
-- state update and its run_events append (S02.3 / CONVENTIONS §6, §8). The
-- deterministic aggregate IS the record; the local tier only enriches it
-- (S14.9 ¶1), so the aggregate columns are frozen by trigger and only the
-- narrative block moves. Control-plane state like runs/asks/conformance_registry
-- — not an event log; the EVENT is run.summary_written and it carries the ref.
--
-- This is also what makes compaction survivable: past the horizon the trace is
-- gone and this row is what remains of the run's story.
CREATE TABLE run_summaries (
    run_id           TEXT PRIMARY KEY REFERENCES runs (run_id),
    user_id          TEXT NOT NULL CHECK (user_id <> ''),
    -- event_seq: the run.summary_written row this summary was written with.
    event_seq        INTEGER NOT NULL REFERENCES run_events (event_seq),
    -- The S14.9 ¶1 structured story. objective/final_state are surfaced as
    -- columns because every reader wants them; the rest of the deterministic
    -- aggregate (stages, tool-call counts, verdicts, decisions, receipts) is
    -- one JSON document so a new aggregate field needs no migration.
    final_state      TEXT NOT NULL CHECK (final_state <> ''),
    objective        TEXT NOT NULL DEFAULT '',
    aggregate_json   TEXT NOT NULL,
    -- inputs_digest: the S14.2 contract-minimum "generation inputs digest" —
    -- a sha256 over exactly the inputs the aggregate was computed from, so a
    -- reader can tell whether a summary rests on the same evidence as another.
    inputs_digest    TEXT NOT NULL CHECK (inputs_digest <> ''),
    -- The local-tier enrichment (S14.9 ¶1 "+ the local tier"). It NEVER gates:
    -- a run ends, and its summary is written, with the stack absent.
    --   pending — a narrator is wired and has not run yet
    --   written — the narrative below is the narrator's output
    --   absent  — no local stack (ErrStackAbsent): aggregate-only, flagged
    --   failed  — the narrator errored; the note records why, nothing is faked
    narrative        TEXT NOT NULL DEFAULT '',
    narrative_status TEXT NOT NULL
        CHECK (narrative_status IN ('pending', 'written', 'absent', 'failed')),
    narrative_note   TEXT NOT NULL DEFAULT '',
    narrative_model  TEXT NOT NULL DEFAULT '',
    written_ts       TEXT NOT NULL,
    updated_ts       TEXT NOT NULL
);

CREATE INDEX run_summaries_owner_idx ON run_summaries (user_id, written_ts);

-- The deterministic aggregate is the record: it is computed once, at run end,
-- from the run's own trace, and it is never recomputed under later evidence.
-- Only the narrative block is mutable.
CREATE TRIGGER run_summaries_aggregate_frozen
    BEFORE UPDATE OF run_id, user_id, event_seq, final_state, objective,
        aggregate_json, inputs_digest, written_ts
    ON run_summaries
BEGIN
    SELECT RAISE(ABORT, 'the run-summary deterministic aggregate is written once at run end and never rewritten (Spec S14.9)');
END;

-- Run summaries are keep-forever (Spec S14.9). A summary is never deleted.
CREATE TRIGGER run_summaries_no_delete
    BEFORE DELETE ON run_summaries
BEGIN
    SELECT RAISE(ABORT, 'run summaries are keep-forever; a summary is never deleted (Spec S14.9)');
END;

-- ── D. The keep-forever allowlist + the 11.3 exporter boundary ───────────────
--
-- S14.9 ¶3: "the snapshot exporter reads an allowlisted view containing only
-- the keep-forever set; raw trace payloads are structurally unreachable from
-- it." S13.10 agrees ("raw trace payload bodies excluded; full traces stay
-- local"), and S02.9's horizon phrasing is satisfied a fortiori — excluding
-- every raw trace body necessarily excludes those past the horizon. A per-user
-- horizon could not be expressed as one global boundary anyway.
--
-- The allowlist is DATA seeded from the S14.2 family registry in
-- internal/eventlog (internal/retention.KeepForeverTypes) — one source of
-- truth, seeded idempotently at boot, never a second hand-written type list.
-- No-delete: keep-forever is one-way. If a later packet demotes a class, the
-- seeded row stays and MORE data is preserved, never less.
CREATE TABLE retention_keep_forever (
    type    TEXT PRIMARY KEY CHECK (type <> ''),
    family  TEXT NOT NULL CHECK (family <> ''),
    -- note records WHY the class is keep-forever, in the S14.9 ¶2 vocabulary.
    note    TEXT NOT NULL DEFAULT '',
    user_id TEXT NOT NULL DEFAULT 'platform' CHECK (user_id = 'platform')
);

CREATE TRIGGER retention_keep_forever_no_delete
    BEFORE DELETE ON retention_keep_forever
BEGIN
    SELECT RAISE(ABORT, 'the keep-forever allowlist is one-way; a class is never demoted by deletion (Spec S14.9)');
END;

-- The exporter's ONLY run_events source. Every row is present (event_seq must
-- stay gap-free across the S02.9 restore check) and every non-keep-forever
-- payload BODY is replaced by the boundary marker — at any age, keep-forever
-- status being the whole test. A raw trace payload cannot be selected through
-- this view, so it cannot reach a dump, an archive, or the broker.
CREATE VIEW run_events_export AS
    SELECT e.event_seq,
           e.run_id,
           e.generation,
           e.user_id,
           e.type,
           e.schema_version,
           CASE WHEN k.type IS NOT NULL
                THEN e.payload
                ELSE '{"exported":false,"reason":"raw trace payload body stays local (Spec S14.9 11.3 boundary, S13.10)"}'
           END AS payload,
           e.ts
      FROM run_events e
      LEFT JOIN retention_keep_forever k ON k.type = e.type;

-- ── E. The S14.10 ¶4 index layer: FTS5 + rollups, both derived ───────────────
--
-- FTS5 over run summaries, verdict texts and drift summaries (S14.10 ¶4). Plain
-- FTS5 maintained Go-side (the 0006 precedent, not external-content): the three
-- sources live in two different places (run_summaries rows and run_events
-- payloads) so no single content table exists. rowid IS the source event_seq,
-- which makes every write idempotent (delete-then-insert by rowid) and the whole
-- index rebuildable from the log.
CREATE VIRTUAL TABLE history_fts USING fts5 (
    body,
    kind UNINDEXED,
    ref UNINDEXED,
    user_id UNINDEXED
);

-- Event-count rollup by (day, owner, type). Derived state, never a system of
-- record (S14.1: tools are runners or projectors, never stores) — DELETE-able,
-- rebuildable from run_events by replay, and read by nothing that could not
-- also compute it directly from the log.
CREATE TABLE run_event_rollup (
    day     TEXT NOT NULL CHECK (day <> ''),
    user_id TEXT NOT NULL CHECK (user_id <> ''),
    type    TEXT NOT NULL CHECK (type <> ''),
    n       INTEGER NOT NULL DEFAULT 0 CHECK (n >= 0),
    PRIMARY KEY (day, user_id, type)
);

-- The durable indexer cursor. Durable rather than head-bootstrapped because a
-- restart must not leave a run summary or a verdict permanently unsearchable;
-- a missed row here is a silent hole in the search surface, not a lost draw.
CREATE TABLE retention_index_cursor (
    row_id     TEXT PRIMARY KEY CHECK (row_id = 'indexer'),
    last_seq   INTEGER NOT NULL DEFAULT 0 CHECK (last_seq >= 0),
    updated_ts TEXT NOT NULL,
    user_id    TEXT NOT NULL DEFAULT 'platform' CHECK (user_id = 'platform')
);

INSERT INTO retention_index_cursor (row_id, last_seq, updated_ts)
    VALUES ('indexer', 0, '1970-01-01T00:00:00Z');

-- ── F. B5-7 drain D5(b): the declined-pair row freeze ────────────────────────
--
-- CONVENTIONS §35 drain r1 D5(b): "A DECLINED pair's row stays SQL-mutable
-- while a recorded one is frozen … B5-8's retention pass may harden the row
-- alongside its keep-forever enforcement." A decline is as final as a record —
-- the record of record for it is the append-only event and the row is working
-- state over that — so the working state stops moving at exactly the same
-- point. Sibling of 0014's benchmark_pairs_recorded_frozen, same shape.
CREATE TRIGGER benchmark_pairs_declined_frozen
    BEFORE UPDATE ON benchmark_pairs
    WHEN OLD.state = 'declined'
BEGIN
    SELECT RAISE(ABORT, 'a declined benchmark pair is frozen; the decline is recorded in the append-only event (CONVENTIONS §35 D5(b))');
END;

PRAGMA user_version = 15;

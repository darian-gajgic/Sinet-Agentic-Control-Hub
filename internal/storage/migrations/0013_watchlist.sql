-- 0013_watchlist.sql — the S14.6 watch-row config store: the control-plane
-- table of watchlist sources, held as config rows `{url|feed, type,
-- parser-hint, lane|candidate}` (Spec S14.6 ¶1; report-02 §6 "watch items are
-- config rows"). G2 Def.12's "no new DB" is satisfied: this is a table in
-- platform.db, not a second store. Exact DDL per the S02.2 schema-workshop
-- mandate; applied in one transaction with its PRAGMA user_version bump by the
-- migration runner (internal/storage/migrate.go, Spec S02.1). A committed
-- migration is immutable (CONVENTIONS §6) — 0001–0012 stay byte-untouched.
--
-- Rows are platform-scope, owner-attributed (user_id = 'platform', the 15.6
-- convention; the 0009 snapshot_ledger / 0011 conformance_registry CHECK
-- shape), so the table joins the S02.9 durable set and every S13.10 archive by
-- construction — no extra work, do not exclude it.
--
-- Row identity and structure are immutable; only the per-row FETCH STATE moves
-- (this table is control-plane state like runs/asks/conformance_registry, not
-- an event log). Hits become drift.finding events and surface as inbox drift
-- cards by derivation over run_events (Spec S14.6 ¶2) — no card table exists.

CREATE TABLE watch_rows (
    -- row_id: the stable watch identity. Seeded rows carry a deterministic id
    -- so re-seeding is idempotent and a lock entry's watch reference resolves
    -- by construction (S16.4 #9).
    row_id            TEXT PRIMARY KEY CHECK (row_id <> ''),
    -- kind: the S14.6 ¶1 "type" field. page = a changedetection.io page-diff
    -- watch (tier T1); feed = a Sinet-polled Atom/RSS feed (T2/T4); api = a
    -- Sinet-polled structured aggregator endpoint (T3); study = a standing
    -- registration/reading obligation with no polled source (the S16.8 gate
    -- rows, the components.lock review rows, the G4 [schema] STUDY row).
    kind              TEXT NOT NULL CHECK (kind IN ('page', 'feed', 'api', 'study')),
    -- url_or_feed: the S14.6 ¶1 "url|feed" field. A study row may carry none
    -- (its obligation is a review, not a fetch); every other kind must.
    url_or_feed       TEXT NOT NULL DEFAULT '' CHECK (kind = 'study' OR url_or_feed <> ''),
    -- parser_hint: the S14.6 ¶1 "parser-hint" field (atom | rss | json | ''),
    -- consumed by the poller to pick its decoder without sniffing.
    parser_hint       TEXT NOT NULL DEFAULT '',
    -- lane_or_candidate: the S14.6 ¶1 "lane|candidate" field — the active lane
    -- a hit touches (anthropic | zai | local) or the watch-only candidate name.
    lane_or_candidate TEXT NOT NULL DEFAULT '',
    -- tier: the report-02 §6 source tier for a provider-watch row (1
    -- authoritative page poll/diff · 2 repo/canary feeds · 3 structured
    -- aggregator APIs · 4 community keyword feeds). For a registration/study
    -- row outside that taxonomy it records the review-urgency class on the same
    -- 1–4 scale (1 = highest) — the scale is reused deliberately rather than
    -- faking a fetch tier for a row that never fetches.
    tier              INTEGER NOT NULL CHECK (tier BETWEEN 1 AND 4),
    -- cadence: the machine dueness token (continuous | weekly | monthly |
    -- quarterly — report-02 §6's cadence set). These are STRUCTURAL: S18
    -- ratifies no watchlist-cadence key and none may be invented
    -- (CONVENTIONS §7 sseBatchSize precedent).
    cadence           TEXT NOT NULL CHECK (cadence IN ('continuous', 'weekly', 'monthly', 'quarterly')),
    enabled           INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    -- source_group: the seeding provenance group (report-02 tier, S16.8, the
    -- components.lock review set, the G4 study set) — the read surface the
    -- S16.7 quarterly dependency pass filters on (R21).
    source_group      TEXT NOT NULL DEFAULT '',
    -- notes: every row cites its ratifying source here (R3).
    notes             TEXT NOT NULL DEFAULT '',
    -- migrate_bias: the P-T11-3 standing bias to migrate a scrape target to a
    -- feed/API the moment one exists — a spec-named POSTURE recorded as data
    -- and surfaced on the decay card, never a comment (R4).
    migrate_bias      INTEGER NOT NULL DEFAULT 0 CHECK (migrate_bias IN (0, 1)),

    -- ── per-row MUTABLE fetch state (the only updatable columns) ──────────
    -- last_fetch_at: dueness is derived from this + cadence, so the loop
    -- survives restart and suspend (never an in-memory ticker).
    last_fetch_at     TEXT,
    -- last_etag / last_modified: the conditional-GET validators replayed as
    -- If-None-Match / If-Modified-Since; a 304 is a cheap no-change.
    last_etag         TEXT NOT NULL DEFAULT '',
    last_modified     TEXT NOT NULL DEFAULT '',
    -- last_content_hash: sha256 over the normalized body — the second dedup
    -- layer for servers that emit no validators.
    last_content_hash TEXT NOT NULL DEFAULT '',
    -- seen_entries: the bounded newline-joined set of per-ENTRY content hashes
    -- already reported for a feed row, so a re-published item is not re-raised
    -- when the body hash moves for an unrelated reason.
    seen_entries      TEXT NOT NULL DEFAULT '',
    -- structural_baseline: the api-row diff baseline — a compact JSON map of
    -- leaf path → value hash, so added/removed/CHANGED keys are all derivable
    -- without keeping the whole document. Empty when the row has never been
    -- fetched or when the document exceeded the structural digest cap (the
    -- diff then degrades honestly to whole-body "changed", recorded on the
    -- finding).
    structural_baseline TEXT NOT NULL DEFAULT '',
    -- fail_streak: consecutive failed fetch cycles; ≥ ⚙
    -- watchlist.fetch_fail_streak announces the dead watch (P-T11-3).
    fail_streak       INTEGER NOT NULL DEFAULT 0 CHECK (fail_streak >= 0),
    -- decay_stage: the P-T11-3 fetcher escalation ladder — 0 plain, 1
    -- escalated (browser fetcher requested on the page tier), 2 decayed (a
    -- card proposing an alternative source has been raised).
    decay_stage       INTEGER NOT NULL DEFAULT 0 CHECK (decay_stage BETWEEN 0 AND 2),
    -- external_ref: the changedetection.io watch uuid for a page row, so
    -- reconciliation is idempotent and a stale remote watch is deleted,
    -- never orphaned.
    external_ref      TEXT NOT NULL DEFAULT '',

    user_id           TEXT NOT NULL DEFAULT 'platform' CHECK (user_id = 'platform')
);

-- Dueness scans read (enabled, cadence, last_fetch_at); the poller runs on
-- every loop tick, so the scan is indexed.
CREATE INDEX watch_rows_due ON watch_rows (enabled, cadence, last_fetch_at);

-- Row identity and structure are immutable; only the fetch-state set may
-- change. BEFORE UPDATE OF fires only when the SET clause names one of the
-- listed columns, so a fetch-state update passes cleanly (the
-- conformance_registry_structure_immutable shape, CONVENTIONS §32).
CREATE TRIGGER watch_rows_structure_immutable
    BEFORE UPDATE OF row_id, kind, url_or_feed, parser_hint, lane_or_candidate,
        tier, cadence, enabled, source_group, notes, migrate_bias, user_id
    ON watch_rows
BEGIN
    SELECT RAISE(ABORT, 'watch-row identity/structure is immutable; only the fetch-state columns are mutable (Spec S14.6; S16.2 removal discipline)');
END;

-- S16.2 removal discipline: a watch row leaves the store only by executing its
-- funeral plan as a dated edit with the reason — never a silent delete (the
-- conformance_registry / repo_registry no-delete precedent).
CREATE TRIGGER watch_rows_no_delete
    BEFORE DELETE ON watch_rows
BEGIN
    SELECT RAISE(ABORT, 'a watch row is never deleted; removal is a dated funeral edit (Spec S16.2)');
END;

PRAGMA user_version = 13;

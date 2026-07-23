-- 0011_conformance_registry.sql — the S14.5 conformance registry: the
-- control-plane table of standing conformance suites and drills (id, owning
-- section, fixtures, trigger set, schedule, last run, last result), the single
-- scheduling HOME for every "proven, not assumed" obligation the spec declares
-- (Spec S14 term coinage line 7; body S14.5). Exact DDL per the S02.2
-- schema-workshop mandate. Applied in one transaction with its PRAGMA
-- user_version bump by the migration runner (internal/storage/migrate.go, Spec
-- S02.1); a committed migration is immutable (CONVENTIONS §6). The table lives
-- in platform.db, so it joins the S02.9 durable set and every S13.10 archive by
-- construction (the 0009 header precedent) — no extra work, do not exclude it.
--
-- Rows are platform-scope, owner-attributed (user_id = 'platform', the 15.6
-- convention; the snapshot_ledger CHECK shape). Row identity and structure are
-- immutable; only last_run/last_result are mutable result state (this table is
-- control-plane state like runs/asks, not an event log — Spec S14.5). Results
-- land as eval.score_recorded events through the recording verb; a red row
-- surfaces as an inbox card by derivation (internal/api projection, owner-
-- scoped) — no asks row (a suite run has no platform run; asks.run_id is NOT
-- NULL). Suites assert BEHAVIOR, never docs (Spec S03.3 rule 4).

CREATE TABLE conformance_registry (
    -- row_id / owning_section: the S14 term-definition "id" and "owning
    -- section" (declared-by) fields — which section owns the suite's CONTENT
    -- (S14.5: content stays with the declaring section, the registry only
    -- schedules).
    row_id         TEXT PRIMARY KEY CHECK (row_id <> ''),
    owning_section TEXT NOT NULL CHECK (owning_section <> ''),
    -- fixtures: the named in-tree run handle(s) as DATA (package path +
    -- test-name + env gates; the F17 one-named-run-handle precedent). Every
    -- reference resolves to a real in-tree test/procedure by source scan — no
    -- row claims a suite that does not exist (Spec S14.5; the honest-dormancy
    -- discipline, CONVENTIONS §26/§27).
    fixtures       TEXT NOT NULL,
    -- trigger_set / schedule: the S14.5 table's "Triggers / schedule" column as
    -- data (schedule = the verbatim spec descriptor; trigger_set = the machine
    -- trigger tokens, comma-joined).
    trigger_set    TEXT NOT NULL,
    schedule       TEXT NOT NULL,
    -- cadence: the machine token dueness derives from (structural
    -- quarterly/weekly/daily per OQ6, or setting:<dotted-key>:<unit> read live
    -- at evaluation time — never a hardcoded ⚙ value, CONVENTIONS §2).
    cadence        TEXT NOT NULL CHECK (cadence <> ''),
    -- canary_cadence: an OPTIONAL secondary ⚙ key registered as VISIBLE DATA
    -- only (read for display, never for dueness) — the forced-escalation row's
    -- daily escalation-canary schedule (⚙ verification.canary_interval_hours,
    -- OQ4(a)); '' for every other row.
    canary_cadence TEXT NOT NULL DEFAULT '',
    -- affect_class: the R14 predicate as row data — a red result is flag-now
    -- exactly when the row is lane- or storage-affecting; 'none' is an ordinary
    -- approval-class card. Flag-now vs digest are alert-routing classes (S14.4),
    -- distinct from the inbox risk tiers.
    affect_class   TEXT NOT NULL CHECK (affect_class IN ('lane', 'storage', 'none')),
    -- derive_kind: '' = a RECORDED row (last_run/last_result are written by the
    -- recording verb, and a red result fires a conformance card). A non-empty
    -- value = a DERIVE row registered for visibility only — its last_run/
    -- last_result DERIVE from the named existing events, its own subsystem owns
    -- the alert, and it never double-fires a conformance card (OQ2 restore-drill
    -- mirror-not-re-emit; OQ5(c) dead-man).
    derive_kind    TEXT NOT NULL DEFAULT '' CHECK (derive_kind IN ('', 'restore_drill', 'dead_man')),
    notes          TEXT NOT NULL DEFAULT '',
    -- last_run / last_result: the two MUTABLE result-state fields (S14.5 term
    -- definition). NULL = never run. last_result is green|red|unknown; a red
    -- recorded row raises the inbox card.
    last_run       TEXT,
    last_result    TEXT CHECK (last_result IS NULL OR last_result IN ('green', 'red', 'unknown')),
    user_id        TEXT NOT NULL DEFAULT 'platform' CHECK (user_id = 'platform')
);

-- Row identity and structure are immutable; only last_run/last_result may
-- change (Spec S14.5 R5 — a result update never rewrites identity/owning-
-- section/fixtures/triggers/schedule). BEFORE UPDATE OF fires only when the
-- SET clause names one of the listed columns, so a last_run/last_result update
-- passes cleanly (the repo_registry identity-immutable precedent, CONVENTIONS
-- §23; the raw-SQL-battery pattern, §22).
CREATE TRIGGER conformance_registry_structure_immutable
    BEFORE UPDATE OF row_id, owning_section, fixtures, trigger_set, schedule,
        cadence, canary_cadence, affect_class, derive_kind, notes, user_id
    ON conformance_registry
BEGIN
    SELECT RAISE(ABORT, 'conformance-registry row identity/structure is immutable; only last_run/last_result are mutable result state (Spec S14.5)');
END;

-- A conformance obligation is never deleted: it is a standing "proven, not
-- assumed" commitment (Spec S14.5; the repo_registry/snapshot_ledger no-delete
-- precedent).
CREATE TRIGGER conformance_registry_no_delete
    BEFORE DELETE ON conformance_registry
BEGIN
    SELECT RAISE(ABORT, 'a conformance obligation row is never deleted (Spec S14.5)');
END;

PRAGMA user_version = 11;

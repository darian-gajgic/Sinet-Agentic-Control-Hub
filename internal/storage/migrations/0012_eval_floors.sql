-- 0012_eval_floors.sql — the S14.8 regression-eval platform data: the per-
-- version FLOOR REGISTRY (eval_floors) and the dated REVALIDATION STAMPS
-- (revalidation_stamps) the runbook writes when a flagged asset returns to
-- active service. Applied in one transaction with its PRAGMA user_version bump
-- by the migration runner (internal/storage/migrate.go, Spec S02.1); a
-- committed migration is immutable (CONVENTIONS §6).
--
-- Both tables live in platform.db, so they join the S02.9 durable set and every
-- S13.10 archive by construction (the 0009/0011 header precedent).

-- eval_floors: the registered floor per (asset id, asset VERSION) — Spec S14.8
-- ¶2 "floors register per rubric/eval-set version at 8.3 knowledge-gate entry"
-- and BENCH-REG §10.1(d) ("the gate cannot be evaluated — and therefore cannot
-- open — while the domain lacks registered suites or any suite is red"). This
-- table is the limb-(d) INPUT; the gate-limb evaluation and display are B5-7's.
--
-- A floor is registered ONCE per version and never edited: re-floor = a new
-- asset version with its own registration (the immutability the per-version
-- keying exists to give). Operator ratification is the ONE mutable field —
-- floors seed from a measurement and become ratified at their gate.
CREATE TABLE eval_floors (
    asset_kind      TEXT NOT NULL CHECK (asset_kind IN ('rubric', 'worker')),
    asset_id        TEXT NOT NULL CHECK (asset_id <> ''),
    asset_version   INTEGER NOT NULL CHECK (asset_version >= 1),
    -- catch_floor: the registered planted-defect CATCH floor — the rate at or
    -- above which the version's planted-defect suite is considered to work
    -- (Spec S14.8 ¶5 rubric falsifiability).
    catch_floor     REAL NOT NULL CHECK (catch_floor >= 0.0 AND catch_floor <= 1.0),
    -- tnr_report_only: the companion true-negative rate, carried REPORT-ONLY.
    -- An over-strict judge (low TNR) is the safe direction for a quality gate,
    -- so this number never reds an asset; it is registered so the honesty is on
    -- the record. NULL = no companion measurement.
    tnr_report_only REAL CHECK (tnr_report_only IS NULL OR (tnr_report_only >= 0.0 AND tnr_report_only <= 1.0)),
    -- basis: how the number was DERIVED (a floor is measurement-derived, never
    -- invented) — the measurement record it comes from.
    basis           TEXT NOT NULL CHECK (basis <> ''),
    -- ratified: 0 = registered but pending operator ratification at its phase
    -- gate; 1 = ratified. The only mutable column.
    ratified        INTEGER NOT NULL DEFAULT 0 CHECK (ratified IN (0, 1)),
    registered_ts   TEXT NOT NULL,
    user_id         TEXT NOT NULL DEFAULT 'platform' CHECK (user_id = 'platform'),
    PRIMARY KEY (asset_id, asset_version)
);

-- A registered floor is immutable except for its ratification flip: a floor
-- that could be dialed down after the fact would falsify nothing. BEFORE UPDATE
-- OF fires only when the SET clause names one of the listed columns, so the
-- ratified flip passes cleanly (the 0011 conformance_registry precedent).
CREATE TRIGGER eval_floors_immutable
    BEFORE UPDATE OF asset_kind, asset_id, asset_version, catch_floor,
        tnr_report_only, basis, registered_ts, user_id
    ON eval_floors
BEGIN
    SELECT RAISE(ABORT, 'a registered eval floor is immutable; re-floor = a new asset version (Spec S14.8)');
END;

CREATE TRIGGER eval_floors_no_delete
    BEFORE DELETE ON eval_floors
BEGIN
    SELECT RAISE(ABORT, 'a registered eval floor is never deleted (BENCH-REG §10.1(d))');
END;

-- revalidation_stamps: the S14.8 ¶3 / S08.10(a) DATED REVALIDATION STAMP — the
-- record through which a flagged worker template version returns to active
-- service. It answers who, when, which suites ran, and against which pinned
-- baseline; a red comparison writes its stamp too (the asset stays flagged and
-- its red is on the record). Append-only: a stamp is history, never edited.
CREATE TABLE revalidation_stamps (
    stamp_id        INTEGER PRIMARY KEY,
    template_id     TEXT NOT NULL CHECK (template_id <> ''),
    version_id      TEXT NOT NULL REFERENCES worker_template_versions (version_id),
    -- trigger_kind/trigger_subject: which of the three S14.8 ¶3 trigger classes
    -- raised this revalidation, and what it named (the engine pin, the swapped
    -- model, the drift finding).
    trigger_kind    TEXT NOT NULL CHECK (trigger_kind IN ('drift_finding', 'engine_bump', 'model_swap')),
    trigger_subject TEXT NOT NULL,
    -- suites: which golden sets / planted-defect suites were run.
    suites          TEXT NOT NULL CHECK (suites <> ''),
    -- baseline: the PINNED baseline the comparison ran against (Spec S14.8 ¶3
    -- "compare TPR/TNR and task metrics against the pinned baseline").
    baseline        TEXT NOT NULL CHECK (baseline <> ''),
    green           INTEGER NOT NULL CHECK (green IN (0, 1)),
    actor           TEXT NOT NULL CHECK (actor <> ''),
    stamped_ts      TEXT NOT NULL,
    -- user_id: the template OWNER (15.6 owner attribution).
    user_id         TEXT NOT NULL CHECK (user_id <> '')
);

CREATE INDEX revalidation_stamps_version ON revalidation_stamps (version_id, stamp_id);

CREATE TRIGGER revalidation_stamps_append_only_update
    BEFORE UPDATE ON revalidation_stamps
BEGIN
    SELECT RAISE(ABORT, 'a revalidation stamp is append-only (Spec S14.8 ¶3)');
END;

CREATE TRIGGER revalidation_stamps_no_delete
    BEFORE DELETE ON revalidation_stamps
BEGIN
    SELECT RAISE(ABORT, 'a revalidation stamp is never deleted (Spec S14.8 ¶3)');
END;

PRAGMA user_version = 12;

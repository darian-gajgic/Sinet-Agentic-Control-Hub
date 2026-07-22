-- 0010_calibration_battery.sql — S12.5 confidence calibration records + S12.9
-- T15 battery results as queryable platform data (OQ1(a) disposition, ratified
-- P3/STATE.md 2026-07-22). Both families are keyed (duty, model hash, engine
-- build) — EXACTLY the key the S12.10 swap gate keeps valid (P-T15-1, S12.10):
-- an alias retarget is refused unless a fresh calibration record AND the
-- affected battery suites exist for the NEW (duty, model hash, engine build).
-- EntailmentGate.Calibrated (S07.4) reads the calibration record for the
-- entailment duty. The Def.8 result files in P3/measurements/ remain the
-- human-readable record; these tables are the durable, queryable home S12.5
-- ("calibrated values are platform data") + S12.9 ("results recorded in
-- platform.db") name. Platform-owned data (user_id 'platform', 15.6);
-- recalibration/re-run REPLACES a key's row (latest-wins; not an event log).

CREATE TABLE calibration_records (
    duty                TEXT    NOT NULL,
    model_hash          TEXT    NOT NULL,
    engine_build        TEXT    NOT NULL,
    threshold           REAL    NOT NULL,
    acceptance_bar      REAL    NOT NULL,
    achieved_error      REAL    NOT NULL,
    accept_rate         REAL    NOT NULL,
    labeled_n           INTEGER NOT NULL,
    meets_bar           INTEGER NOT NULL,
    isotonic_json       TEXT    NOT NULL,
    margin_summary_json TEXT    NOT NULL DEFAULT '{}',
    calibrated_ts       TEXT    NOT NULL,
    user_id             TEXT    NOT NULL DEFAULT 'platform',
    PRIMARY KEY (duty, model_hash, engine_build)
);

CREATE TABLE battery_results (
    battery_version TEXT    NOT NULL,
    duty            TEXT    NOT NULL,
    model_hash      TEXT    NOT NULL,
    engine_build    TEXT    NOT NULL,
    cases_total     INTEGER NOT NULL,
    cases_passed    INTEGER NOT NULL,
    tokens_per_s    REAL    NOT NULL DEFAULT 0,
    wilson_lo       REAL    NOT NULL,
    wilson_hi       REAL    NOT NULL,
    ran_ts          TEXT    NOT NULL,
    user_id         TEXT    NOT NULL DEFAULT 'platform',
    PRIMARY KEY (battery_version, duty, model_hash, engine_build)
);

CREATE INDEX battery_results_by_target ON battery_results (duty, model_hash, engine_build);

PRAGMA user_version = 10;

-- 0002_run_recovery_lineage.sql — fork-from-checkpoint lineage columns on
-- runs (Spec S02.3, S02.5 step 3).
--
-- S02.5 step 3 requires recovery attempts to "reuse one durable dispatch id
-- per interruption (an ambiguous failure cannot double-start)" with
-- ⚙ recovery.max_attempts bounding repeat offenders, and S02.3 supersedes a
-- crashed run "by fork-from-last-checkpoint as a new run with generation+1".
-- That needs three durable columns 0001 does not carry:
--
--   parent_run_id     — the supersession lineage link (successor -> crashed
--                       parent);
--   dispatch_id       — the durable one-dispatch-per-interruption id; the
--                       partial UNIQUE index makes the successor INSERT the
--                       CAS that renders a replayed fork inert;
--   recovery_attempts — lineage-cumulative fork count, checked against
--                       ⚙ recovery.max_attempts before each fork.
--
-- 0001 is immutable once committed (P3/CONVENTIONS.md §6), so this is a new
-- numbered migration.

ALTER TABLE runs ADD COLUMN parent_run_id TEXT REFERENCES runs (run_id);
ALTER TABLE runs ADD COLUMN dispatch_id TEXT;
ALTER TABLE runs ADD COLUMN recovery_attempts INTEGER NOT NULL DEFAULT 0
    CHECK (recovery_attempts >= 0);

CREATE UNIQUE INDEX runs_dispatch_idx ON runs (dispatch_id)
    WHERE dispatch_id IS NOT NULL;

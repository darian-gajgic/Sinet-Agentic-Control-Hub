-- 0026_revision_produced_by.sql — the revision's attribution JOIN KEY
-- (P3-GF10; Spec S13.1, S13.6 step 3, S08.8).
--
-- Exact DDL per the S02.2 schema-workshop mandate; applied in ONE transaction
-- with its PRAGMA user_version bump by the migration runner
-- (internal/storage/migrate.go, Spec S02.1). A committed migration is immutable
-- (CONVENTIONS §6) — 0001-0025 stay byte-untouched, this file included once it
-- lands.
--
-- WHAT THE ROW COULD NOT ANSWER. `run_id` (0007) is the MINTING run, and the
-- S13.4 drain mints every revision on the VERIFY leg, because the drain's
-- events, judge assemblies, checkpoints and verification tax ride that run
-- (Spec S07.11). The settled S08.8 selection — the model and lane that made the
-- content — is recorded by the EXECUTE dispatch and only there. So the S13.6
-- step-3 accept, resolving attribution through the minting run, asked the
-- verify leg a question only the execute leg records the answer to, found
-- nothing, and closed the door on correct work.
--
-- WHY A JOIN KEY AND NOT COPIED FACTS. S08.8's own pattern is that
-- version→outcome is a JOIN, not a system: `routing.decided` and
-- `engine_sessions` stay the ONLY attribution stores, and the revision gains
-- the key that reaches them. Copying model/lane/substrate strings onto this row
-- would be a side store of what the log already records.
--
-- WHY IT IS RECORDED AT MINT, NOT DERIVED AT READ. Which attempt produced
-- revision N is a per-revision fact, fixed when the content is frozen. The
-- lineage it is walked from can advance afterwards (Spec S02.5 recovery forks),
-- so a read-time derivation would answer a later question than the one the
-- revision froze.
--
-- NULLABLE, AND NOT BACKFILLED. Revisions minted before this migration record
-- no producing run and are not rewritten: they resolve through their minting
-- run exactly as they always did, which is the honest state for a row whose
-- producer was never recorded. A NULL is therefore "unrecorded", never "none".
--
-- NO FOREIGN KEY, matching the sibling `run_id` in 0007: the minting ref
-- carries no reference wall either, and a revision's honest record of which run
-- produced it should not be refusable by the order two rows are written in.
ALTER TABLE deliverable_revisions ADD COLUMN produced_by_run_id TEXT;

-- Fill-once, the 0007 `snapshot_sha` pattern (0007's
-- `deliverable_revisions_immutable` trigger enumerates its columns by name and
-- so does not cover a column added later). Set at insert and immutable after: a
-- re-mint under an advanced lineage never rewrites the recorded producer, and
-- the still-NULL slot on a pre-0026 row stays open for a later backfill that
-- fills it exactly once.
CREATE TRIGGER deliverable_revisions_produced_by_fill_once
    BEFORE UPDATE OF produced_by_run_id ON deliverable_revisions
    WHEN OLD.produced_by_run_id IS NOT NULL
BEGIN
    SELECT RAISE(ABORT, 'produced_by_run_id fills once, NULL -> the producing run (Spec S13.1)');
END;

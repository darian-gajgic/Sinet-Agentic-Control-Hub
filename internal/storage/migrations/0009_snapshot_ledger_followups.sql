-- 0009_snapshot_ledger_followups.sql — the two S13 table families deferred
-- past migration 0008: snapshot_ledger (S13.10, the 11.3 integrity ledger,
-- P-T12-4) and the S13.9 follow-up successor link. Exact DDL per the S02.2
-- schema-workshop mandate (the S13.1 family table gives indicative names).
-- Applied in one transaction with its PRAGMA user_version bump by the
-- migration runner (internal/storage/migrate.go, Spec S02.1); a committed
-- migration is immutable (CONVENTIONS §6). Both families join the S02.9
-- durable set by reference (Spec S13.1) — they live in platform.db and are
-- therefore inside every subsequent snapshot the S13.10 pipeline takes.

-- snapshot_ledger: one row per encrypted platform-state snapshot (Spec
-- S13.10). The integrity triple (sha256 + size_bytes + created_ts) is the
-- P-T12-4 load-bearing record: age has NO sender authentication, so a
-- compromised remote swapping a blob is defeated ONLY by verifying the
-- fetched blob against this ledger. The triple is recorded here AND — because
-- this table is in the durable set — inside every SUBSEQUENT archive, so
-- archive N+1 carries archive N's row (Spec S13.10). Rows are platform-scope:
-- the whole durable set is platform state (user_id = 'platform', the 15.6
-- attribution convention). seq is the monotonic keep-N retention ordering
-- (the last ⚙ backup.keep blobs are the retained set; older blobs are pruned
-- from the snapshot repo, the ledger stays append-only). blob_name is the
-- blob's file name in the snapshot-repo tree; repo_commit is the snapshot-repo
-- commit it landed in (the blob's identity in the repo, Spec S13.10). The
-- ledger is append-only in spirit and integrity-critical — no row is ever
-- updated or deleted (P-T12-4).
CREATE TABLE snapshot_ledger (
    seq         INTEGER PRIMARY KEY,
    user_id     TEXT NOT NULL DEFAULT 'platform' CHECK (user_id = 'platform'),
    snapshot_id TEXT NOT NULL UNIQUE CHECK (snapshot_id <> ''),
    sha256      TEXT NOT NULL CHECK (length(sha256) = 64),
    size_bytes  INTEGER NOT NULL CHECK (size_bytes >= 0),
    created_ts  TEXT NOT NULL,
    blob_name   TEXT NOT NULL CHECK (blob_name <> ''),
    repo_commit TEXT NOT NULL DEFAULT ''
);

CREATE TRIGGER snapshot_ledger_no_update
    BEFORE UPDATE ON snapshot_ledger
BEGIN
    SELECT RAISE(ABORT, 'the snapshot ledger is append-only and integrity-critical (Spec S13.10 P-T12-4)');
END;

CREATE TRIGGER snapshot_ledger_no_delete
    BEFORE DELETE ON snapshot_ledger
BEGIN
    SELECT RAISE(ABORT, 'the snapshot ledger is append-only and integrity-critical (Spec S13.10 P-T12-4)');
END;

-- task_successor_of: the S13.9 follow-up successor link. A finished/accepted
-- deliverable spawns a successor task in one action; the successor row carries
-- a successor_of link to (deliverable, revision) (Spec S13.9). Lineage is
-- resolvable in BOTH directions (S13.9 "lineage is visible in both
-- directions"): forward — a task -> its predecessor (deliverable, revision) by
-- the PK; reverse — a (deliverable, revision) -> its successor task(s) by the
-- indexed source pair. task_id PRIMARY KEY: a successor task has at most ONE
-- predecessor (S13.9 "the successor row carries a successor_of link"). The
-- composite FK to deliverable_revisions(deliverable_id, n) makes the source
-- revision REFERENTIALLY REAL — a successor can only be linked to a revision
-- that exists. Owner-attributed (15.6). The link is immutable — lineage is
-- never rewritten (revision/extension/counterpart are intake PRESETS over this
-- same link, not schema variants, Spec S13.9).
CREATE TABLE task_successor_of (
    task_id        TEXT PRIMARY KEY REFERENCES tasks (task_id),
    deliverable_id TEXT NOT NULL,
    revision_n     INTEGER NOT NULL CHECK (revision_n >= 1),
    user_id        TEXT NOT NULL CHECK (user_id <> ''),
    created_ts     TEXT NOT NULL,
    FOREIGN KEY (deliverable_id, revision_n) REFERENCES deliverable_revisions (deliverable_id, n)
);

CREATE INDEX task_successor_of_source_idx ON task_successor_of (deliverable_id, revision_n);

CREATE TRIGGER task_successor_of_no_update
    BEFORE UPDATE ON task_successor_of
BEGIN
    SELECT RAISE(ABORT, 'follow-up lineage is immutable (Spec S13.9)');
END;

CREATE TRIGGER task_successor_of_no_delete
    BEFORE DELETE ON task_successor_of
BEGIN
    SELECT RAISE(ABORT, 'follow-up lineage is never dropped (Spec S13.9)');
END;

-- 0004_knowledge.sql — the S09 memory/knowledge schema core (Spec S09.2):
-- knowledge_entries + lesson_proposals + knowledge_adoptions, plus the
-- S09.7 conflict edges and the FTS5 index behind knowledge_search
-- (Spec S09.3).
--
-- The full schema ships whole at v0 (Spec S09.4 activation table): all
-- layers × scopes, owner-attributed (15.6). What stays dormant (L1
-- writers, stations 1–2/5) is dormant by having NO writer — never by a
-- reduced schema. Every row carries its owning user_id (the S09.2
-- owner_user field, named user_id per the 0001 convention).

-- knowledge_entries: the L1/L2 row store — the row is the selector index,
-- the file (where file-backed) is the content, git is the history
-- (Spec S09.2). L0 has no rows here: task scratch lives in the ledger and
-- the run workspace (Spec S09.1).
CREATE TABLE knowledge_entries (
    entry_id               TEXT PRIMARY KEY CHECK (entry_id <> ''),
    user_id                TEXT NOT NULL CHECK (user_id <> ''),
    scope                  TEXT NOT NULL CHECK (scope IN ('user', 'worker_overlay', 'project', 'house')),
    scope_ref              TEXT NOT NULL DEFAULT '',
    layer                  TEXT NOT NULL CHECK (layer IN ('L1', 'L2')),
    kind                   TEXT NOT NULL CHECK (kind IN (
                               'lesson', 'preference', 'playbook', 'rubric',
                               'style', 'exemplar', 'convention',
                               'observation', 'taxonomy', 'trigger_rules')),
    title                  TEXT NOT NULL,
    -- content XOR file-backing (Spec S09.2); a tombstone row has neither
    -- (true deletion purges both, G2 Def.9).
    content                TEXT,
    file_path              TEXT,
    file_commit            TEXT,
    topic_key              TEXT,
    selectors              TEXT NOT NULL DEFAULT '{}',
    status                 TEXT NOT NULL CHECK (status IN ('proposed', 'active', 'retired', 'removed')),
    version                INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
    supersedes_id          TEXT REFERENCES knowledge_entries (entry_id),
    -- Provenance (Spec S09.5): every L2 entry carries origin, proposer
    -- (model if drafted), approver + timestamp; the version chain rides
    -- supersedes_id; the git hash rides file_commit.
    origin                 TEXT NOT NULL CHECK (origin IN ('proposed_from', 'human_direct', 'adopted_from', 'imported')),
    origin_ref             TEXT NOT NULL DEFAULT '',
    proposer_model         TEXT NOT NULL DEFAULT '',
    approved_by            TEXT NOT NULL DEFAULT '',
    approved_ts            TEXT,
    -- Verification lifecycle (Spec S09.4 station 5, S09.8): recorded at v0
    -- so intervals apply retroactively at v1 activation.
    verified_by            TEXT NOT NULL DEFAULT '',
    verified_ts            TEXT,
    reverify_interval_days INTEGER CHECK (reverify_interval_days IS NULL OR reverify_interval_days > 0),
    expires_ts             TEXT,
    last_injected_ts       TEXT,
    -- True deletion leaves the minimal audit stub (G2 Def.9).
    tombstone              INTEGER NOT NULL DEFAULT 0 CHECK (tombstone IN (0, 1)),
    tombstone_note         TEXT NOT NULL DEFAULT '',
    created_ts             TEXT NOT NULL,
    updated_ts             TEXT NOT NULL,
    -- Observations are the L1 kind; everything else is L2 (Spec S09.1).
    CHECK ((kind = 'observation') = (layer = 'L1')),
    CHECK (tombstone = 0 OR status = 'removed'),
    -- Live rows carry content or a file ref; only a tombstone carries
    -- neither (Spec S09.2, G2 Def.9).
    CHECK (tombstone = 1 OR content IS NOT NULL OR file_path IS NOT NULL)
);

CREATE INDEX knowledge_entries_owner_idx ON knowledge_entries (user_id, scope, status);
CREATE INDEX knowledge_entries_topic_idx ON knowledge_entries (topic_key)
    WHERE topic_key IS NOT NULL;
CREATE INDEX knowledge_entries_scope_idx ON knowledge_entries (scope, scope_ref, status);

-- Rows are never deleted: removal is a status change, true deletion is a
-- content purge that leaves the tombstone row (Spec S09.5; G2 Def.9).
CREATE TRIGGER knowledge_entries_no_delete
    BEFORE DELETE ON knowledge_entries
BEGIN
    SELECT RAISE(ABORT, 'knowledge entries are never deleted: removal/true deletion leave the audit row (Spec S09.5)');
END;

-- Identity fields never change in place; supersession is a NEW version row
-- (Spec S09.4 station 4: never in-place mutation).
CREATE TRIGGER knowledge_entries_identity_immutable
    BEFORE UPDATE OF entry_id, user_id, scope, scope_ref, layer, kind,
                     origin, version, supersedes_id ON knowledge_entries
BEGIN
    SELECT RAISE(ABORT, 'knowledge entry identity is immutable: supersession is a new version row (Spec S09.2, S09.4)');
END;

-- Content changes in place only toward NULL/empty (the true-deletion purge
-- path); an edit is a new version row. file_commit may fill once (NULL →
-- hash) when the control plane commits the file (Spec S09.2).
CREATE TRIGGER knowledge_entries_content_immutable
    BEFORE UPDATE OF title, content, file_path, file_commit ON knowledge_entries
    WHEN (NEW.content IS NOT NULL AND OLD.content IS NOT NULL AND NEW.content <> OLD.content)
      OR (NEW.content IS NOT NULL AND OLD.content IS NULL)
      OR (NEW.file_path IS NOT NULL AND OLD.file_path IS NOT NULL AND NEW.file_path <> OLD.file_path)
      OR (NEW.file_path IS NOT NULL AND OLD.file_path IS NULL)
      OR (NEW.file_commit IS NOT NULL AND OLD.file_commit IS NOT NULL AND NEW.file_commit <> OLD.file_commit)
      OR (NEW.title <> '' AND OLD.title <> '' AND NEW.title <> OLD.title)
BEGIN
    SELECT RAISE(ABORT, 'knowledge entry content is immutable in place: edit = new version, purge = true deletion (Spec S09.2, S09.5)');
END;

-- lesson_proposals: the pending queue (Spec S09.2). Proposals are NOT
-- memory: no assembly, tool, or search path reads them into any run
-- context — enforced structurally by keeping them out of the FTS index and
-- out of every selection query. Station-2 drafting is dormant at v0
-- (Spec S09.4): the schema ships whole, no writer is active.
CREATE TABLE lesson_proposals (
    proposal_id    TEXT PRIMARY KEY CHECK (proposal_id <> ''),
    user_id        TEXT NOT NULL CHECK (user_id <> ''),
    scope          TEXT NOT NULL CHECK (scope IN ('user', 'worker_overlay', 'project', 'house')),
    scope_ref      TEXT NOT NULL DEFAULT '',
    kind           TEXT NOT NULL CHECK (kind IN (
                       'lesson', 'preference', 'playbook', 'rubric',
                       'style', 'exemplar', 'convention',
                       'observation', 'taxonomy', 'trigger_rules')),
    title          TEXT NOT NULL DEFAULT '',
    content        TEXT NOT NULL DEFAULT '',
    evidence_refs  TEXT NOT NULL DEFAULT '[]',
    risk_rank      INTEGER NOT NULL DEFAULT 0,
    batch_id       TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL CHECK (status IN ('open', 'approved', 'edited_approved', 'dismissed')),
    proposer_model TEXT NOT NULL DEFAULT '',
    decided_by     TEXT,
    decided_ts     TEXT,
    created_ts     TEXT NOT NULL
);

CREATE INDEX lesson_proposals_status_idx ON lesson_proposals (status, user_id);

-- The drafted evidence is audit content: only the decision fields move.
CREATE TRIGGER lesson_proposals_draft_immutable
    BEFORE UPDATE OF proposal_id, user_id, scope, scope_ref, kind, title,
                     content, evidence_refs, risk_rank, batch_id,
                     proposer_model, created_ts ON lesson_proposals
BEGIN
    SELECT RAISE(ABORT, 'lesson proposal drafts are immutable: only the decision fields change (Spec S09.2)');
END;

CREATE TRIGGER lesson_proposals_no_delete
    BEFORE DELETE ON lesson_proposals
BEGIN
    SELECT RAISE(ABORT, 'lesson proposals are audit rows and are never deleted (Spec S09.2)');
END;

-- knowledge_adoptions: adoption copies an entry into the adopter's own
-- scope with the origin recorded — never a live shared reference
-- (Spec S09.6). user_id is the adopter (the row's owner, 15.6).
CREATE TABLE knowledge_adoptions (
    adoption_id      INTEGER PRIMARY KEY,
    user_id          TEXT NOT NULL CHECK (user_id <> ''),
    source_entry_id  TEXT NOT NULL REFERENCES knowledge_entries (entry_id),
    adopted_entry_id TEXT NOT NULL REFERENCES knowledge_entries (entry_id),
    adopted_ts       TEXT NOT NULL
);

CREATE TRIGGER knowledge_adoptions_no_update
    BEFORE UPDATE ON knowledge_adoptions
BEGIN
    SELECT RAISE(ABORT, 'adoption records are append-only (Spec S09.6)');
END;

CREATE TRIGGER knowledge_adoptions_no_delete
    BEFORE DELETE ON knowledge_adoptions
BEGIN
    SELECT RAISE(ABORT, 'adoption records are append-only (Spec S09.6)');
END;

-- knowledge_conflicts: the S09.7 conflicts_with edge plus its question to
-- the affected owner — surfaced, never silently resolved. user_id is the
-- affected owner the question addresses.
CREATE TABLE knowledge_conflicts (
    conflict_id    INTEGER PRIMARY KEY,
    user_id        TEXT NOT NULL CHECK (user_id <> ''),
    entry_id       TEXT NOT NULL REFERENCES knowledge_entries (entry_id),
    other_entry_id TEXT NOT NULL REFERENCES knowledge_entries (entry_id),
    topic_key      TEXT NOT NULL DEFAULT '',
    question       TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL CHECK (status IN ('open', 'resolved')),
    detected_ts    TEXT NOT NULL,
    resolved_by    TEXT,
    resolved_ts    TEXT
);

CREATE INDEX knowledge_conflicts_status_idx ON knowledge_conflicts (status, user_id);

CREATE TRIGGER knowledge_conflicts_no_delete
    BEFORE DELETE ON knowledge_conflicts
BEGIN
    SELECT RAISE(ABORT, 'conflict edges are audit rows and are never deleted (Spec S09.7)');
END;

-- knowledge_fts: the FTS5 index behind the knowledge_search tool
-- (Spec S09.3) — external-content over knowledge_entries ONLY (proposals
-- are structurally outside every search path, Spec S09.2). The scope
-- predicate is appended server-side at query time; this index carries no
-- authorization.
CREATE VIRTUAL TABLE knowledge_fts USING fts5(
    title,
    content,
    content='knowledge_entries'
);

CREATE TRIGGER knowledge_entries_fts_insert
    AFTER INSERT ON knowledge_entries
BEGIN
    INSERT INTO knowledge_fts (rowid, title, content)
    VALUES (new.rowid, new.title, coalesce(new.content, ''));
END;

CREATE TRIGGER knowledge_entries_fts_update
    AFTER UPDATE OF title, content ON knowledge_entries
BEGIN
    INSERT INTO knowledge_fts (knowledge_fts, rowid, title, content)
    VALUES ('delete', old.rowid, old.title, coalesce(old.content, ''));
    INSERT INTO knowledge_fts (rowid, title, content)
    VALUES (new.rowid, new.title, coalesce(new.content, ''));
END;

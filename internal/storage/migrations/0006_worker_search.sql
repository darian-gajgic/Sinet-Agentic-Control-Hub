-- 0006_worker_search.sql — the S08.8 selection index: FTS5 over worker
-- delegation descriptions ("Selector match over the template registry: task
-- selectors + FTS5 over delegation descriptions", Spec S08.8 step 1). The
-- delegation-grade description lives in the template FILE (Spec S08.1
-- identity/purpose block), so the index row is derived data maintained by
-- the store whenever active_version moves (Approve / Repoint — the only
-- verbs that move the pointer, Spec S08.4); status filtering is NEVER
-- trusted to this table — selection always joins worker_templates and
-- filters status = 'active' from the authoritative row (Spec S08.8).
--
-- Plain FTS5 (not external-content): the indexed text has no home table —
-- it is parsed out of the hash-verified template file. FTS5 availability at
-- the pinned driver is proven by the standing memory-package test (0004
-- precedent).
CREATE VIRTUAL TABLE worker_search USING fts5 (
    name,
    description,
    triggers,
    template_id UNINDEXED
);

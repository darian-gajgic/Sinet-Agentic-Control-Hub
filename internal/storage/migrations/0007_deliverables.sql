-- 0007_deliverables.sql — the S13.1/S13.3 deliverable, revision and review
-- comment schema (Spec S13.1 table families; exact DDL per the S02.2
-- schema-workshop mandate). Owner-attributed per 15.6; append-only /
-- immutability disciplines are triggers (the 0004/0005 precedent). These
-- families join the S02.9 durable set by reference: they live in
-- platform.db and are part of the 11.3 snapshot payload (pipeline S13.10).
-- Revision CONTENT bytes are host-local review material in the
-- content-addressed object dir, outside the snapshot payload; the committed
-- hash refs here make loss detectable (Spec S13.2, G2 Def.14).

-- deliverables: the long-lived reviewable entity a task produces — one per
-- task output (Spec S13.1). dtype is deliberately an OPEN set (no CHECK):
-- no type is refused at v0; a type without a rich comparison surface gets
-- the honest labeled fallback (Spec S13.2). subject_ref carries the
-- reviewed object's own id where one exists (worker-definition
-- deliverables name their template version — Spec S13.1 "automation
-- definitions ride the same machinery").
CREATE TABLE deliverables (
    deliverable_id   TEXT PRIMARY KEY CHECK (deliverable_id <> ''),
    user_id          TEXT NOT NULL CHECK (user_id <> ''),
    task_id          TEXT NOT NULL CHECK (task_id <> ''),
    project_id       TEXT NOT NULL DEFAULT '',
    subject_ref      TEXT NOT NULL DEFAULT '',
    dtype            TEXT NOT NULL CHECK (dtype <> ''),
    current_revision INTEGER NOT NULL DEFAULT 0 CHECK (current_revision >= 0),
    state            TEXT NOT NULL CHECK (state IN ('in-review', 'accepted', 'superseded')),
    created_ts       TEXT NOT NULL,
    updated_ts       TEXT NOT NULL
);

CREATE INDEX deliverables_task_idx ON deliverables (task_id);
CREATE INDEX deliverables_owner_idx ON deliverables (user_id, state);

CREATE TRIGGER deliverables_no_delete
    BEFORE DELETE ON deliverables
BEGIN
    SELECT RAISE(ABORT, 'deliverables are never deleted: supersession is a state change (Spec S13.1)');
END;

CREATE TRIGGER deliverables_identity_immutable
    BEFORE UPDATE OF deliverable_id, user_id, task_id, project_id,
                     subject_ref, dtype, created_ts ON deliverables
BEGIN
    SELECT RAISE(ABORT, 'deliverable identity is immutable (Spec S13.1)');
END;

-- Revision hops are persisted and never rewound: current_revision only
-- moves forward (GitHub force-push timeline compression is the named
-- anti-pattern — Spec S13.1).
CREATE TRIGGER deliverables_revision_monotone
    BEFORE UPDATE OF current_revision ON deliverables
    WHEN NEW.current_revision < OLD.current_revision
BEGIN
    SELECT RAISE(ABORT, 'current_revision never rewinds: lineage is never compressed (Spec S13.1)');
END;

-- deliverable_revisions: immutable numbered snapshots 1..N (Spec S13.1).
-- The content pin covers both kinds: content-pinned revisions carry the
-- revision content hash (+ the file listing in objects, each blob
-- content-addressed in the local object dir); object-pinned (binary)
-- revisions carry object-dir hash refs alone (Spec S13.2, G2 Def.14).
-- snapshot_sha is the S13.5 repo-backed snapshot-commit pin — NULL until
-- git topology lands (B4-2), then fill-once. platform_ref is the retention
-- ref namespace refs/sinet/deliverable/<id>/rev-<n> (Spec S13.1); the ref
-- itself is created in the project store at B4-2. verdict_ref is the
-- verification-verdict ref [XREF:S07] — the verify.round event_seq,
-- fill-once after the round judges this revision.
CREATE TABLE deliverable_revisions (
    deliverable_id TEXT NOT NULL REFERENCES deliverables (deliverable_id),
    n              INTEGER NOT NULL CHECK (n >= 1),
    user_id        TEXT NOT NULL CHECK (user_id <> ''),
    run_id         TEXT NOT NULL DEFAULT '',
    attempt_ref    TEXT NOT NULL DEFAULT '',
    pin_kind       TEXT NOT NULL CHECK (pin_kind IN ('content', 'objects')),
    content_sha256 TEXT NOT NULL DEFAULT '',
    snapshot_sha   TEXT,
    platform_ref   TEXT NOT NULL CHECK (platform_ref <> ''),
    objects        TEXT NOT NULL DEFAULT '[]',
    verdict_ref    INTEGER,
    created_ts     TEXT NOT NULL,
    PRIMARY KEY (deliverable_id, n),
    CHECK (pin_kind <> 'content' OR content_sha256 <> ''),
    CHECK (pin_kind <> 'objects' OR objects <> '[]')
);

CREATE TRIGGER deliverable_revisions_no_delete
    BEFORE DELETE ON deliverable_revisions
BEGIN
    SELECT RAISE(ABORT, 'revisions are immutable and lineage is never compressed (Spec S13.1)');
END;

CREATE TRIGGER deliverable_revisions_immutable
    BEFORE UPDATE OF deliverable_id, n, user_id, run_id, attempt_ref,
                     pin_kind, content_sha256, platform_ref, objects,
                     created_ts ON deliverable_revisions
BEGIN
    SELECT RAISE(ABORT, 'a minted revision is immutable (Spec S13.1)');
END;

CREATE TRIGGER deliverable_revisions_snapshot_fill_once
    BEFORE UPDATE OF snapshot_sha ON deliverable_revisions
    WHEN OLD.snapshot_sha IS NOT NULL
BEGIN
    SELECT RAISE(ABORT, 'snapshot_sha fills once, NULL -> sha (Spec S13.1/S13.5)');
END;

CREATE TRIGGER deliverable_revisions_verdict_fill_once
    BEFORE UPDATE OF verdict_ref ON deliverable_revisions
    WHEN OLD.verdict_ref IS NOT NULL
BEGIN
    SELECT RAISE(ABORT, 'verdict_ref fills once, NULL -> verify.round seq (Spec S13.1)');
END;

-- review_comments: ONE schema for human review comments (6.2) and
-- verification findings (Spec S13.1/S13.3; [XREF:S07]). comment_id is an
-- INTEGER PRIMARY KEY (rowid alias) so insertion order is explicit,
-- FK-able, and preserved by the 11.3 text dump — the drain's deterministic
-- ordering input (Spec S13.4 "stable within the attempt").
--
-- The anchor columns are the ORIGINAL anchor record of FC-v1 §2 —
-- {file_path, side, line_no, line_text} bound to (deliverable, revision_n)
-- — the AS-CLAIMED quote + position selectors, immutable; per-revision
-- validated placements live in review_comment_anchors. The positional trio
-- (side, line_no, line_text) is all-or-nothing and requires file_path;
-- file_path may stand alone (a file-level comment naming a file); all-NULL
-- is a deliverable-level/cross-cutting comment. Lifecycle is open ->
-- consumed, one way, batch-stamped: the CHECKs force the three consumption
-- stamps (consumed_ts, consumed_by, finding_number) to appear exactly with
-- 'consumed' (Spec S13.3/S13.4).
--
-- The no-comment-without-render-location invariant (FC-v1 §2) is the CHECK
-- set: anchor_status is total over {exact, drifted, file}; a positional
-- birth status requires the claimed anchor it validated, and 'file' —
-- claimed-but-unfound (quote kept visible) or never-claimed — renders in
-- the file strip / the always-visible synthetic strip. No representable
-- row lacks a render location.
CREATE TABLE review_comments (
    comment_id       INTEGER PRIMARY KEY,
    user_id          TEXT NOT NULL CHECK (user_id <> ''),
    deliverable_id   TEXT NOT NULL REFERENCES deliverables (deliverable_id),
    revision_n       INTEGER NOT NULL CHECK (revision_n >= 1),
    run_id           TEXT NOT NULL DEFAULT '',
    kind             TEXT NOT NULL CHECK (kind IN ('human', 'finding')),
    severity         TEXT NOT NULL CHECK (severity IN ('blocker', 'note')),
    category         TEXT NOT NULL DEFAULT '',
    criterion        TEXT NOT NULL DEFAULT '',
    body             TEXT NOT NULL CHECK (body <> ''),
    suggested_change TEXT NOT NULL DEFAULT '',
    file_path        TEXT,
    side             TEXT CHECK (side IS NULL OR side IN ('old', 'new')),
    line_no          INTEGER CHECK (line_no IS NULL OR line_no >= 1),
    line_text        TEXT,
    origin_anchor    TEXT NOT NULL DEFAULT '',
    anchor_status    TEXT NOT NULL CHECK (anchor_status IN ('exact', 'drifted', 'file')),
    status           TEXT NOT NULL CHECK (status IN ('open', 'consumed')),
    finding_number   INTEGER CHECK (finding_number IS NULL OR finding_number >= 1),
    consumed_ts      TEXT,
    consumed_by      TEXT NOT NULL DEFAULT '',
    created_ts       TEXT NOT NULL,
    CHECK ((side IS NULL) = (line_no IS NULL)),
    CHECK ((line_no IS NULL) = (line_text IS NULL)),
    CHECK (line_no IS NULL OR file_path IS NOT NULL),
    CHECK (anchor_status = 'file' OR line_no IS NOT NULL),
    CHECK ((status = 'consumed') = (consumed_ts IS NOT NULL)),
    CHECK ((status = 'consumed') = (consumed_by <> '')),
    CHECK ((status = 'consumed') = (finding_number IS NOT NULL))
);

CREATE INDEX review_comments_open_idx ON review_comments (deliverable_id, status);
CREATE INDEX review_comments_revision_idx ON review_comments (deliverable_id, revision_n);

CREATE TRIGGER review_comments_no_delete
    BEFORE DELETE ON review_comments
BEGIN
    SELECT RAISE(ABORT, 'review comments are never dropped (Spec S13.3; P-T12-2)');
END;

CREATE TRIGGER review_comments_immutable
    BEFORE UPDATE OF comment_id, user_id, deliverable_id, revision_n,
                     run_id, kind, severity, category, criterion, body,
                     suggested_change, file_path, side, line_no, line_text,
                     origin_anchor, anchor_status, created_ts ON review_comments
BEGIN
    SELECT RAISE(ABORT, 'a review comment is immutable: only the one-way consumption stamp moves (Spec S13.3)');
END;

CREATE TRIGGER review_comments_consume_one_way
    BEFORE UPDATE OF status, finding_number, consumed_ts, consumed_by ON review_comments
    WHEN OLD.status <> 'open'
BEGIN
    SELECT RAISE(ABORT, 'consumed is final: a re-review that deems a point unfixed produces a FRESH finding (Spec S13.3)');
END;

-- review_comment_anchors: anchor status per comment per REVISION (Spec
-- S13.3 "anchor status is recorded per comment per revision"). One row per
-- (comment, revision), written by server-side validation at birth and by
-- every port up the 5-step re-anchoring ladder; placements are pure
-- functions of immutable revisions, so a recorded row never changes —
-- re-validation recomputes and must agree (silent mis-anchoring is worse
-- than orphaning, P-T12-2). ORPHAN is an explicit state here — the
-- comment row keeps the quote and the original-revision link; the orphan
-- placement renders in the synthetic strip and still drains (file-level).
CREATE TABLE review_comment_anchors (
    comment_id  INTEGER NOT NULL REFERENCES review_comments (comment_id),
    revision_n  INTEGER NOT NULL CHECK (revision_n >= 1),
    status      TEXT NOT NULL CHECK (status IN ('exact', 'mapped', 'drifted', 'file', 'orphan')),
    file_path   TEXT,
    side        TEXT CHECK (side IS NULL OR side IN ('old', 'new')),
    line_no     INTEGER CHECK (line_no IS NULL OR line_no >= 1),
    line_text   TEXT,
    computed_ts TEXT NOT NULL,
    PRIMARY KEY (comment_id, revision_n),
    CHECK ((side IS NULL) = (line_no IS NULL)),
    CHECK ((line_no IS NULL) = (line_text IS NULL)),
    CHECK (line_no IS NULL OR file_path IS NOT NULL),
    CHECK (status IN ('file', 'orphan') OR line_no IS NOT NULL),
    CHECK ((status NOT IN ('file', 'orphan')) OR line_no IS NULL),
    CHECK (status <> 'orphan' OR file_path IS NULL)
);

CREATE TRIGGER review_comment_anchors_no_update
    BEFORE UPDATE ON review_comment_anchors
BEGIN
    SELECT RAISE(ABORT, 'a recorded placement never changes: revisions are immutable, re-validation must agree (Spec S13.3)');
END;

CREATE TRIGGER review_comment_anchors_no_delete
    BEFORE DELETE ON review_comment_anchors
BEGIN
    SELECT RAISE(ABORT, 'anchor history is never dropped (Spec S13.3)');
END;

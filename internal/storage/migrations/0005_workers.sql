-- 0005_workers.sql — the S08 worker store (Spec S08.1): registry rows plus
-- git-versioned template files in a Sinet-owned superset schema, with the
-- guardrail split (Spec S08.2) made structural: behavioral content lives in
-- template files; ALL enforcement state lives EXCLUSIVELY in
-- worker_guardrails, written only by the control plane on a human approval.
-- The control plane is the sole writer of every table here (Spec S02); the
-- template store and platform.db sit outside every task sandbox, so a
-- running worker can write neither (Spec S08.2, S11).

-- domains: verification maturity per domain (Spec S08.1, S08.7). Degraded
-- mode is structural, not advisory: the delivery/schedule enforcement reads
-- this row. Day-one rows: software = full; web-research joins full at v0.1
-- and ships degraded until then; all other domains enter degraded with
-- their packs (Spec S08.1, 2.1, S19). Maturity flips only through D10
-- (operator approval, enforced in code — Spec S08.7).
CREATE TABLE domains (
    domain                TEXT PRIMARY KEY CHECK (domain <> ''),
    verification_maturity TEXT NOT NULL CHECK (verification_maturity IN ('full', 'degraded')),
    rubric_ref            TEXT NOT NULL DEFAULT '',
    updated_ts            TEXT NOT NULL
);

INSERT INTO domains (domain, verification_maturity, rubric_ref, updated_ts) VALUES
    ('software', 'full', 'seed-verify-rubric-software', strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    ('web-research', 'degraded', '', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

-- worker_templates: one row per worker; the row is the index, the file is
-- the behavioral content, git is the history (Spec S08.1 — the ratified
-- row-index/file-content pattern). Multi-user from day one: every row
-- carries its owner (15.6). active_version is the ONLY mutable pointer
-- (Spec S08.1); rollback = repoint (Spec S08.4).
CREATE TABLE worker_templates (
    template_id    TEXT PRIMARY KEY CHECK (template_id <> ''),
    user_id        TEXT NOT NULL CHECK (user_id <> ''),
    name           TEXT NOT NULL CHECK (name <> ''),
    scope          TEXT NOT NULL CHECK (scope IN ('personal', 'household')),
    kind           TEXT NOT NULL CHECK (kind IN ('agentic', 'automation')),
    domain         TEXT NOT NULL REFERENCES domains (domain),
    status         TEXT NOT NULL CHECK (status IN ('draft', 'validated', 'active', 'flagged', 'retired')),
    active_version TEXT REFERENCES worker_template_versions (version_id),
    created_ts     TEXT NOT NULL,
    updated_ts     TEXT NOT NULL,
    UNIQUE (user_id, name)
);

-- worker_template_versions: immutable version rows — every edit is a new
-- row plus a new file commit; history is never rewritten (Spec S08.4).
-- The provenance block (Spec S08.1): author, evidence refs, approver,
-- origin. requested_grants is the CONTROL-PLANE draft of enforcement
-- requests accompanying the file (the file itself may request equipment
-- only; a guardrail-class field in the file is a structural lint reject,
-- Spec S08.2/S08.6 station 1). graduated_ts is the graduation event
-- (first-N complete, Spec S08.1/S08.6).
CREATE TABLE worker_template_versions (
    version_id                TEXT PRIMARY KEY CHECK (version_id <> ''),
    template_id               TEXT NOT NULL REFERENCES worker_templates (template_id),
    version                   INTEGER NOT NULL CHECK (version >= 1),
    supersedes_id             TEXT REFERENCES worker_template_versions (version_id),
    file_path                 TEXT NOT NULL CHECK (file_path <> ''),
    file_sha256               TEXT NOT NULL CHECK (length(file_sha256) = 64),
    -- NULL until the S13 committer lands (B4); one NULL→hash fill allowed.
    file_commit               TEXT,
    requested_grants          TEXT NOT NULL,
    author_kind               TEXT NOT NULL CHECK (author_kind IN ('human', 'composer')),
    composer_model            TEXT NOT NULL DEFAULT '',
    composer_playbook_version TEXT NOT NULL DEFAULT '',
    evidence_ref              TEXT NOT NULL DEFAULT '',
    origin                    TEXT NOT NULL CHECK (origin IN ('composed', 'human-written', 'imported', 'adopted-from')),
    origin_ref                TEXT NOT NULL DEFAULT '',
    created_by                TEXT NOT NULL CHECK (created_by <> ''),
    created_ts                TEXT NOT NULL,
    approved_by               TEXT NOT NULL DEFAULT '',
    approved_ts               TEXT,
    graduated_ts              TEXT,
    UNIQUE (template_id, version)
);

CREATE INDEX idx_worker_versions_template ON worker_template_versions (template_id, version);

-- worker_guardrails: the enforcement state of Spec S08.2, EXCLUSIVELY —
-- granted tools, permission map, confinement class (C0–C2 at v0), egress
-- class, budget/ceiling set, gate policy, the first-N counter, schedule
-- attachability. Keyed by version_id; written only by the control plane on
-- a human approval; present in no file; recompiled into the engine
-- invocation on every run (Spec S08.2, S08.3). Workers can NEVER alter
-- these rows: the change surface does not exist inside a run (14.2).
CREATE TABLE worker_guardrails (
    version_id           TEXT PRIMARY KEY REFERENCES worker_template_versions (version_id),
    granted_tools        TEXT NOT NULL,
    permission_map       TEXT NOT NULL,
    confinement_class    TEXT NOT NULL CHECK (confinement_class IN ('C0', 'C1', 'C2')),
    egress_class         TEXT NOT NULL CHECK (egress_class IN ('none', 'registries', 'single-host')),
    egress_hosts         TEXT NOT NULL DEFAULT '[]',
    budget_ceiling_usd   REAL NOT NULL CHECK (budget_ceiling_usd >= 0),
    budget_ceiling_steps INTEGER NOT NULL CHECK (budget_ceiling_steps >= 0),
    gate_policy          TEXT NOT NULL DEFAULT '{}',
    first_n_remaining    INTEGER NOT NULL CHECK (first_n_remaining >= 0),
    schedule_attachable  INTEGER NOT NULL DEFAULT 0 CHECK (schedule_attachable IN (0, 1)),
    created_ts           TEXT NOT NULL,
    updated_ts           TEXT NOT NULL
);

-- validation_records: one row per battery pass per (version × model ×
-- engine pin); for kind=automation the engine-pin slot holds the dialect
-- version (Spec S08.1, S08.9). Revalidation stamps are NEW rows (Spec
-- S08.10), never edits.
CREATE TABLE validation_records (
    record_id    INTEGER PRIMARY KEY,
    version_id   TEXT NOT NULL REFERENCES worker_template_versions (version_id),
    model        TEXT NOT NULL DEFAULT '',
    engine_pin   TEXT NOT NULL CHECK (engine_pin <> ''),
    lint_result  TEXT NOT NULL,
    audit_result TEXT NOT NULL,
    dryrun_ref   TEXT NOT NULL DEFAULT '',
    green        INTEGER NOT NULL CHECK (green IN (0, 1)),
    approver     TEXT NOT NULL DEFAULT '',
    approved_ts  TEXT,
    created_ts   TEXT NOT NULL
);

CREATE INDEX idx_validation_records_version ON validation_records (version_id, created_ts);

-- gap_records: the persistent record of no-fit routing outcomes (Spec
-- S08.1, S08.6, S08.8): accumulated so recurrence can earn composition —
-- the roster specializes because recurring work earned it (14.4). The
-- accumulator columns are mutable; occurrence_count never decreases.
CREATE TABLE gap_records (
    signature        TEXT PRIMARY KEY CHECK (signature <> ''),
    family           TEXT NOT NULL CHECK (family <> ''),
    task_refs        TEXT NOT NULL DEFAULT '[]',
    occurrence_count INTEGER NOT NULL CHECK (occurrence_count >= 1),
    last_seen_ts     TEXT NOT NULL,
    disposition      TEXT NOT NULL CHECK (disposition IN ('open', 'proposed', 'composed', 'dismissed'))
);

-- ── Structural immutability (Spec S08.2, S08.4; the 0004 trigger
--    discipline): version history is never rewritten, guardrails change
--    only through the narrow control-plane paths, nothing here is ever
--    deleted (retire is a status flip). Raw-SQL holders hit the same
--    walls.

-- Templates: identity fields are frozen; name/status/active_version/
-- updated_ts are the mutable surface (Spec S08.1: active_version is the
-- only mutable pointer; status is lifecycle state; name mirrors the
-- active version's definition).
CREATE TRIGGER worker_templates_identity_frozen
BEFORE UPDATE ON worker_templates
WHEN NEW.template_id IS NOT OLD.template_id
  OR NEW.user_id IS NOT OLD.user_id
  OR NEW.scope IS NOT OLD.scope AND NOT (OLD.scope = 'personal' AND NEW.scope = 'household')
  OR NEW.kind IS NOT OLD.kind
  OR NEW.domain IS NOT OLD.domain
  OR NEW.created_ts IS NOT OLD.created_ts
BEGIN
    SELECT RAISE(ABORT, 'worker_templates: identity fields are immutable (S08.1; promotion personal→household is the one scope edge, S08.4)');
END;

CREATE TRIGGER worker_templates_no_delete
BEFORE DELETE ON worker_templates
BEGIN
    SELECT RAISE(ABORT, 'worker_templates: rows are never deleted — retire is a status flip (S08.4, S08.9)');
END;

-- Version rows: immutable except the three fill-once columns —
-- file_commit (NULL→hash when the S13 committer lands), approval
-- (''/NULL→value at station 4), graduation (NULL→value at first-N
-- complete).
CREATE TRIGGER worker_versions_immutable
BEFORE UPDATE ON worker_template_versions
WHEN NEW.version_id IS NOT OLD.version_id
  OR NEW.template_id IS NOT OLD.template_id
  OR NEW.version IS NOT OLD.version
  OR NEW.supersedes_id IS NOT OLD.supersedes_id
  OR NEW.file_path IS NOT OLD.file_path
  OR NEW.file_sha256 IS NOT OLD.file_sha256
  OR NEW.requested_grants IS NOT OLD.requested_grants
  OR NEW.author_kind IS NOT OLD.author_kind
  OR NEW.composer_model IS NOT OLD.composer_model
  OR NEW.composer_playbook_version IS NOT OLD.composer_playbook_version
  OR NEW.evidence_ref IS NOT OLD.evidence_ref
  OR NEW.origin IS NOT OLD.origin
  OR NEW.origin_ref IS NOT OLD.origin_ref
  OR NEW.created_by IS NOT OLD.created_by
  OR NEW.created_ts IS NOT OLD.created_ts
  OR (OLD.file_commit IS NOT NULL AND NEW.file_commit IS NOT OLD.file_commit)
  OR (OLD.approved_by <> '' AND NEW.approved_by IS NOT OLD.approved_by)
  OR (OLD.approved_ts IS NOT NULL AND NEW.approved_ts IS NOT OLD.approved_ts)
  OR (OLD.graduated_ts IS NOT NULL AND NEW.graduated_ts IS NOT OLD.graduated_ts)
BEGIN
    SELECT RAISE(ABORT, 'worker_template_versions: version rows are immutable — every edit is a new version (S08.4)');
END;

CREATE TRIGGER worker_versions_no_delete
BEFORE DELETE ON worker_template_versions
BEGIN
    SELECT RAISE(ABORT, 'worker_template_versions: history is never rewritten (S08.4)');
END;

-- Guardrails: written at approval; afterwards only the first-N counter
-- may move, and only downward (count-based, Spec S08.6; resets are NEW
-- rows on NEW versions).
CREATE TRIGGER worker_guardrails_frozen
BEFORE UPDATE ON worker_guardrails
WHEN NEW.version_id IS NOT OLD.version_id
  OR NEW.granted_tools IS NOT OLD.granted_tools
  OR NEW.permission_map IS NOT OLD.permission_map
  OR NEW.confinement_class IS NOT OLD.confinement_class
  OR NEW.egress_class IS NOT OLD.egress_class
  OR NEW.egress_hosts IS NOT OLD.egress_hosts
  OR NEW.budget_ceiling_usd IS NOT OLD.budget_ceiling_usd
  OR NEW.budget_ceiling_steps IS NOT OLD.budget_ceiling_steps
  OR NEW.gate_policy IS NOT OLD.gate_policy
  OR NEW.schedule_attachable IS NOT OLD.schedule_attachable
  OR NEW.created_ts IS NOT OLD.created_ts
  OR NEW.first_n_remaining > OLD.first_n_remaining
BEGIN
    SELECT RAISE(ABORT, 'worker_guardrails: enforcement state changes only via a new approval; first-N only counts down (S08.2, S08.6)');
END;

CREATE TRIGGER worker_guardrails_no_delete
BEFORE DELETE ON worker_guardrails
BEGIN
    SELECT RAISE(ABORT, 'worker_guardrails: enforcement state is never deleted (S08.2)');
END;

-- Validation records: append-only; the approver stamp fills once at
-- station 4 (Spec S08.6).
CREATE TRIGGER validation_records_immutable
BEFORE UPDATE ON validation_records
WHEN NEW.record_id IS NOT OLD.record_id
  OR NEW.version_id IS NOT OLD.version_id
  OR NEW.model IS NOT OLD.model
  OR NEW.engine_pin IS NOT OLD.engine_pin
  OR NEW.lint_result IS NOT OLD.lint_result
  OR NEW.audit_result IS NOT OLD.audit_result
  OR NEW.dryrun_ref IS NOT OLD.dryrun_ref
  OR NEW.green IS NOT OLD.green
  OR NEW.created_ts IS NOT OLD.created_ts
  OR (OLD.approver <> '' AND NEW.approver IS NOT OLD.approver)
  OR (OLD.approved_ts IS NOT NULL AND NEW.approved_ts IS NOT OLD.approved_ts)
BEGIN
    SELECT RAISE(ABORT, 'validation_records: battery results are append-only; revalidation is a new row (S08.6, S08.10)');
END;

CREATE TRIGGER validation_records_no_delete
BEFORE DELETE ON validation_records
BEGIN
    SELECT RAISE(ABORT, 'validation_records: battery results are never deleted (S08.6)');
END;

CREATE TRIGGER gap_records_monotonic
BEFORE UPDATE ON gap_records
WHEN NEW.signature IS NOT OLD.signature
  OR NEW.occurrence_count < OLD.occurrence_count
BEGIN
    SELECT RAISE(ABORT, 'gap_records: signature is the identity and occurrences never decrease (S08.6)');
END;

CREATE TRIGGER gap_records_no_delete
BEFORE DELETE ON gap_records
BEGIN
    SELECT RAISE(ABORT, 'gap_records: gap history is never deleted — dispositions record outcomes (S08.6)');
END;

CREATE TRIGGER domains_no_delete
BEFORE DELETE ON domains
BEGIN
    SELECT RAISE(ABORT, 'domains: domain rows are never deleted (S08.7)');
END;

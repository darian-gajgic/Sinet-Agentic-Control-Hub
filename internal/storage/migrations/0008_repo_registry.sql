-- 0008_repo_registry.sql — the S13.7 project/repository registry (Spec S13.1
-- table family `repo_registry`; exact DDL per the S02.2 schema-workshop
-- mandate). Owner-attributed per 15.6; captured content is VERSIONED via
-- immutable capture rows (Spec S13.7 "captured content is versioned" — a
-- silent in-place overwrite of captured content is not admissible). These
-- rows join the S02.9 durable set by reference (Spec S13.1). Applied in one
-- transaction with its PRAGMA user_version bump by the migration runner
-- (internal/storage/migrate.go, Spec S02.1); a committed migration is
-- immutable (CONVENTIONS §6).
--
-- snapshot_ledger (also listed in the S13.1 family table) is NOT created
-- here: it is S13.10's and lands with its own migration (B4-3).

-- repo_registry: one entry per known project/repository (Spec S13.7). Holds,
-- per entry: name/alias, local project-store path, remote URL (stored data,
-- never dialed in dev mode), owning user, invited members, default branch,
-- protected refs (broker policy input, S13.6 — data only here), and a pointer
-- to the current captured-content version (repo_registry_captures). Entry
-- lifecycle is pending (onboarding in flight) -> active (owner approved); an
-- entry feeds nothing until active (Spec S13.7 onboarding-as-task).
CREATE TABLE repo_registry (
    project_id      TEXT PRIMARY KEY CHECK (project_id <> ''),
    user_id         TEXT NOT NULL CHECK (user_id <> ''),
    name            TEXT NOT NULL CHECK (name <> ''),
    store_path      TEXT NOT NULL CHECK (store_path <> ''),
    remote_url      TEXT NOT NULL DEFAULT '',
    default_branch  TEXT NOT NULL CHECK (default_branch <> ''),
    members         TEXT NOT NULL DEFAULT '[]',
    protected_refs  TEXT NOT NULL DEFAULT '[]',
    state           TEXT NOT NULL CHECK (state IN ('pending', 'active')),
    capture_version INTEGER NOT NULL DEFAULT 0 CHECK (capture_version >= 0),
    created_ts      TEXT NOT NULL,
    updated_ts      TEXT NOT NULL
);

CREATE INDEX repo_registry_owner_idx ON repo_registry (user_id, state);

CREATE TRIGGER repo_registry_no_delete
    BEFORE DELETE ON repo_registry
BEGIN
    SELECT RAISE(ABORT, 'registry entries are never deleted: retirement is a state change (Spec S13.7)');
END;

CREATE TRIGGER repo_registry_identity_immutable
    BEFORE UPDATE OF project_id, user_id, store_path, created_ts ON repo_registry
BEGIN
    SELECT RAISE(ABORT, 'registry entry identity is immutable (Spec S13.7; owner-attributed 15.6)');
END;

-- Lifecycle is one-way pending -> active (Spec S13.7): active is the approved
-- state and never reverts to pending.
CREATE TRIGGER repo_registry_state_one_way
    BEFORE UPDATE OF state ON repo_registry
    WHEN OLD.state = 'active' AND NEW.state <> 'active'
BEGIN
    SELECT RAISE(ABORT, 'an active registry entry never reverts to pending (Spec S13.7)');
END;

-- The captured-content pointer only advances (each re-capture is a new
-- immutable version): silent overwrite of captured content is inadmissible
-- (Spec S13.7).
CREATE TRIGGER repo_registry_capture_monotone
    BEFORE UPDATE OF capture_version ON repo_registry
    WHEN NEW.capture_version < OLD.capture_version
BEGIN
    SELECT RAISE(ABORT, 'capture_version never rewinds: re-capture is a new version (Spec S13.7)');
END;

-- repo_registry_captures: one immutable row per captured-content version
-- (Spec S13.7 "captured content is versioned"; the version bumps on every
-- re-capture/re-scan/edit and stays traceable). Each version holds the full
-- conventions, commands (build/test/lint/run/preview), and danger zones
-- (path/action + rule + source hash — the S05.1 §2 shape the ledger pins),
-- plus scan_hash, the fingerprint of the scanned source that a drift check
-- compares against live content (Spec S13.7 re-scan-on-drift; S05.6 row 2).
-- Rows are immutable and never deleted: history of what the platform captured
-- is auditable after the fact.
CREATE TABLE repo_registry_captures (
    project_id   TEXT NOT NULL REFERENCES repo_registry (project_id),
    version      INTEGER NOT NULL CHECK (version >= 1),
    conventions  TEXT NOT NULL DEFAULT '[]',
    commands     TEXT NOT NULL DEFAULT '{}',
    danger_zones TEXT NOT NULL DEFAULT '[]',
    scan_hash    TEXT NOT NULL DEFAULT '',
    captured_by  TEXT NOT NULL CHECK (captured_by <> ''),
    captured_ts  TEXT NOT NULL,
    PRIMARY KEY (project_id, version)
);

CREATE TRIGGER repo_registry_captures_immutable
    BEFORE UPDATE ON repo_registry_captures
BEGIN
    SELECT RAISE(ABORT, 'a captured-content version is immutable: re-capture is a NEW version (Spec S13.7)');
END;

CREATE TRIGGER repo_registry_captures_no_delete
    BEFORE DELETE ON repo_registry_captures
BEGIN
    SELECT RAISE(ABORT, 'capture history is never dropped (Spec S13.7)');
END;

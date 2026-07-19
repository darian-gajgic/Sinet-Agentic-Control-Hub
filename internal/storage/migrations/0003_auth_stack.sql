-- 0003_auth_stack.sql — the S01.9 authentication stack: per-user PIN
-- credentials (argon2id), server-side sessions as owner-attributed rows,
-- and per-device trusted auto-login grants (Spec S01.9; 15.6; G3 Def.1).
--
-- The audit truth for every auth act (login, failure, grant, re-prompt) is
-- the run_events log, not these tables (Spec S01.9, S01.11): sessions and
-- grants are current state and may be deleted; history lives in events.

-- Per-user PIN credential, stored as an argon2id PHC-format string (Spec
-- S01.9 layer 3: "user picker + per-user PIN/password (argon2id)").
-- '' means no credential is set (the user cannot complete a PIN login).
ALTER TABLE users ADD COLUMN pin_hash TEXT NOT NULL DEFAULT '';

-- Consecutive failed PIN verifications and the lockout deadline they feed.
-- The attempt ceiling and lockout window are code constants at B0 — Spec
-- S18 ratifies no ⚙ key for them (see internal/auth).
ALTER TABLE users ADD COLUMN pin_failed_attempts INTEGER NOT NULL DEFAULT 0
    CHECK (pin_failed_attempts >= 0);
ALTER TABLE users ADD COLUMN pin_locked_until_ts TEXT;

-- sessions: layer 3 of the S01.9 stack — server-side sessions as
-- owner-attributed rows in platform.db (15.6). session_id is the SHA-256
-- hex of the bearer token; the token itself is never stored. Expiry is
-- wall-clock and absolute: a suspend never extends a login. device_login
-- records the Tailscale-User-Login device hint observed at mint (audit
-- context only, never authority — Spec S01.9 layer 2).
CREATE TABLE sessions (
    session_id   TEXT PRIMARY KEY CHECK (session_id <> ''),
    user_id      TEXT NOT NULL REFERENCES users (user_id),
    device_login TEXT NOT NULL DEFAULT '',
    created_ts   TEXT NOT NULL,
    expires_ts   TEXT NOT NULL
);

CREATE INDEX sessions_user_idx ON sessions (user_id);

-- device_grants: the explicit operator grant of Spec S01.9 layer 2 — on a
-- personal device the Tailscale-User-Login hint may complete login as
-- user_id. The default for every device is shared (no row): PIN always
-- required (G3 Def.1). device_login is the whole device signal the ratified
-- chain provides (one active Tailscale account per device), so one row maps
-- one device account to exactly one platform user.
CREATE TABLE device_grants (
    device_login TEXT PRIMARY KEY CHECK (device_login <> ''),
    user_id      TEXT NOT NULL REFERENCES users (user_id),
    granted_by   TEXT NOT NULL CHECK (granted_by <> ''),
    created_ts   TEXT NOT NULL
);

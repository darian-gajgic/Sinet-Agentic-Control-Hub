-- 0021_push_subscriptions.sql — the durable home of the S15.11 Web Push
-- subscriptions: which of a person's devices the notifier may reach, and with
-- what key material.
--
-- Exact DDL per the S02.2 schema-workshop mandate; applied in ONE transaction
-- with its PRAGMA user_version bump by the migration runner
-- (internal/storage/migrate.go, Spec S02.1). A committed migration is immutable
-- (CONVENTIONS §6) — 0001–0020 stay byte-untouched, this file included once it
-- lands.
--
-- WHY A TABLE AND NOT DERIVE-FROM-LOG. A subscription is live capability
-- material the sender reads on every evaluation, not a record of something that
-- happened: the endpoint URL and the two keys are what an outbound request is
-- BUILT from, and they are replaced and removed over a device's life. That is
-- control-plane state, the same class as `sessions` (0003) — which is the
-- precedent this table follows in every respect, including living in
-- platform.db beside it. The `push.*` events this family mints carry LIFECYCLE
-- FACTS with a HASH of the endpoint, never the endpoint itself (CONVENTIONS
-- §29; Spec S15.2 rule 3).
--
-- THE ENDPOINT IS A CAPABILITY URL, AND THE SCHEMA SAYS SO TWICE. Anyone
-- holding it can send a push to that device through the vendor's relay, so it is
-- stored (the sender needs it) and it is NEVER served: `endpoint_hash` exists so
-- every read surface, every event payload and every operator-facing listing can
-- name a subscription without handing over the capability. The hash is the
-- REFERENCE; the endpoint is the secret.
--
-- OWNER-SCOPE IS FAIL-CLOSED BY CONSTRUCTION: user_id CHECKs non-empty exactly
-- as 0001 does, so a zero-value identity can never collide with a real owner.
-- UNIQUE (user_id, endpoint) is what makes a re-enrolment a REPLACE rather than
-- a duplicate — a browser hands back the same endpoint for the same
-- subscription, and a phone that re-subscribes must not become two devices.
--
-- NO MONEY, NO PERCENT, NO ESTIMATE is stored, computed or derived in this file.

CREATE TABLE push_subscriptions (
    subscription_id TEXT PRIMARY KEY CHECK (subscription_id <> ''),
    -- The owner. Every record carries who it belongs to (feature 15.6).
    user_id         TEXT NOT NULL CHECK (user_id <> ''),
    -- The RFC 8030 push resource the sender POSTs to. Secret (see above).
    endpoint        TEXT NOT NULL CHECK (endpoint <> ''),
    -- sha256 hex of the endpoint: the reference every surface and every event
    -- payload uses in its place. Stored rather than computed at read time so a
    -- listing and an audit row can never disagree about which row they mean.
    endpoint_hash   TEXT NOT NULL CHECK (endpoint_hash <> ''),
    -- The RFC 8291 subscription keys, base64url as the browser serves them:
    -- p256dh is the 65-octet uncompressed P-256 point, auth the 16-octet secret.
    p256dh          TEXT NOT NULL CHECK (p256dh <> ''),
    auth            TEXT NOT NULL CHECK (auth <> ''),
    -- The ORIGIN the enrolling page was served from, recorded per subscription
    -- (B6-9 OQ7). A push payload's `navigate` field must be an absolute URL and
    -- the control plane binds loopback behind the S01.4 front chain, so it
    -- cannot see the origin a person reaches it by. The enrolling page can, and
    -- it is the origin that device's deep link has to land on.
    origin          TEXT NOT NULL CHECK (origin <> ''),
    -- The S01.9 layer-2 device hint observed at enrolment, and the person's own
    -- label for the device. Both are AUDIT CONTEXT and never authority — the
    -- authenticated identity in user_id is the only thing that scopes this row.
    device_login    TEXT NOT NULL DEFAULT '',
    label           TEXT NOT NULL DEFAULT '',
    created_ts      TEXT NOT NULL,
    updated_ts      TEXT NOT NULL,
    -- One row per (person, endpoint): a re-subscribe REPLACES rather than
    -- duplicating, which is what makes the enrol verb retry-safe (S15.2).
    UNIQUE (user_id, endpoint)
);

-- The notifier's own read: every subscription of one identity.
CREATE INDEX push_subscriptions_user ON push_subscriptions (user_id, created_ts);

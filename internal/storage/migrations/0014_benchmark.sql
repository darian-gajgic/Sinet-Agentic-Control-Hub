-- 0014_benchmark.sql — the S14.7 benchmark-practice control-plane state: the
-- in-flight blind pair, the per-domain epoch/first-open bookkeeping, the
-- sampler cursor, and the per-user standing opt-in.
--
-- The PROTOCOL is Spec/benchmark-preregistration-v1.md (BENCH-REG), the signed
-- registration. This migration stores the practice's state; it encodes NONE of
-- the registered numbers (those are marked read-only data in internal/benchmark,
-- S18.4 R9) and re-derives nothing. Exact DDL per the S02.2 schema-workshop
-- mandate; applied in one transaction with its PRAGMA user_version bump by the
-- migration runner (internal/storage/migrate.go, Spec S02.1). A committed
-- migration is immutable (CONVENTIONS §6) — 0001–0013 stay byte-untouched.
--
-- These are control-plane state tables like runs/asks/conformance_registry, not
-- an event log: pair identity is immutable, the §3 flow's working state moves.
-- The RECORD is the event (benchmark.pair_recorded, BENCH-REG §14) — the row's
-- move to `recorded` and that append commit in ONE transaction, and the reveal
-- reads the committed record, never this table's mutable state (BENCH-REG §3.4).

-- ── The standing benchmark opt-in (BENCH-REG §4.2.1) ─────────────────────────
--
-- Eligibility limb 1 is "its requester has the standing benchmark opt-in
-- enabled". It is an opt-IN, so the default is OFF and no migration turns it on
-- for anyone. It is a per-user consent datum, not a ⚙ setting: S18 declares no
-- key for it and the frozen tally (118 keys / 33 domains) is not this packet's
-- to move. The flip is a package verb that logs the consent as a human decision
-- (decision.recorded); the UI surface is B6's. The 0003 ALTER TABLE users
-- precedent applies.
ALTER TABLE users ADD COLUMN benchmark_opt_in INTEGER NOT NULL DEFAULT 0
    CHECK (benchmark_opt_in IN (0, 1));

-- ── The blind pair (BENCH-REG §3, §14) ───────────────────────────────────────
CREATE TABLE benchmark_pairs (
    -- ── IMMUTABLE identity (trigger-enforced below) ──────────────────────────
    -- pair_id: the BENCH-REG §14 "pair id".
    pair_id          TEXT PRIMARY KEY CHECK (pair_id <> ''),
    -- user_id: the requester — the voter, the billed party (3.4) and the row's
    -- owner (15.6). Pending-verdict cards are scoped to this user.
    user_id          TEXT NOT NULL CHECK (user_id <> ''),
    -- domain: the REGISTERED domain name (BENCH-REG §10 — v0:
    -- 'software-development'), never the tree's internal code name. The
    -- code→registered mapping is one pinned, tested translation in the package.
    domain           TEXT NOT NULL CHECK (domain <> ''),
    -- domain_source records HOW the domain was resolved for this pair (the
    -- accepting run's verdict record, or the routing/worker-template domain),
    -- so a later reader can tell a first-class resolution from a fallback.
    domain_source    TEXT NOT NULL DEFAULT '',
    task_id          TEXT NOT NULL CHECK (task_id <> ''),
    deliverable_id   TEXT NOT NULL CHECK (deliverable_id <> ''),
    -- task_class: the BENCH-REG §14 "task class" — the intake family/tier
    -- reading recorded at sampling, never re-derived later.
    task_class       TEXT NOT NULL DEFAULT '',
    -- platform_run_id: the run that produced the accepted deliverable — the
    -- platform arm (BENCH-REG §2).
    platform_run_id  TEXT NOT NULL DEFAULT '',
    -- phase / rate_pct: the ⚙ benchmark.sampling_rate phase and the percent
    -- actually used for THIS draw, recorded on the pair so accrual is auditable
    -- against the schedule that produced it (BENCH-REG §4.1).
    phase            TEXT NOT NULL CHECK (phase IN ('pre-gate', 'maintenance')),
    rate_pct         INTEGER NOT NULL CHECK (rate_pct BETWEEN 0 AND 100),
    -- source_seq: the event_seq of the deliverable.accepted row that triggered
    -- the sampling draw — the derive-from-log provenance of the pair.
    source_seq       INTEGER NOT NULL DEFAULT 0,
    sampled_ts       TEXT NOT NULL,

    -- ── MUTABLE working state (the §3 flow) ──────────────────────────────────
    -- state: sampled → declined | dispatched → rendered → recorded.
    state            TEXT NOT NULL CHECK (state IN
                        ('sampled', 'declined', 'dispatched', 'rendered', 'recorded')),
    -- direct_run_id: the §2 direct-arm run, enqueued as ordinary background
    -- work under the requester's own user_id (G2 Def.4; Spec S10.4/S10.7).
    direct_run_id    TEXT NOT NULL DEFAULT '',
    -- position: the §3.2 per-pair randomized left/right assignment. It maps A/B
    -- back to the arms, so it is arm-identity data: no pre-record read surface
    -- selects it (BENCH-REG §3.4, reveal only after the record is written).
    position         TEXT NOT NULL DEFAULT ''
                        CHECK (position IN ('', 'platform-a', 'platform-b')),
    -- render_a / render_b: the two deliverables as rendered through the ONE
    -- uniform presentation template (§3.2) — position-keyed, never arm-keyed,
    -- so they are exactly what the blind voter sees. They live here rather than
    -- in an object store because they are small text artifacts OF the practice:
    -- keeping them in platform.db puts them inside the S02.9 durable set and
    -- every S13.10 archive by construction. The §14 record carries REFS to them
    -- (refs-not-blobs on the event, P-T07-5), never the bodies.
    render_a         TEXT NOT NULL DEFAULT '',
    render_b         TEXT NOT NULL DEFAULT '',
    -- length_platform / length_direct: the §3.2 length confound, REPORTED
    -- alongside and never "corrected" — the template never truncates.
    length_platform  INTEGER NOT NULL DEFAULT 0,
    length_direct    INTEGER NOT NULL DEFAULT 0,
    -- verdict / guess: the §3.3 form. There is no verdict without a guess; the
    -- package's answer verb cannot express one (the guess rides the same act).
    verdict          TEXT NOT NULL DEFAULT ''
                        CHECK (verdict IN ('', 'A', 'B', 'tie', 'both-bad')),
    guess            TEXT NOT NULL DEFAULT '' CHECK (guess IN ('', 'A', 'B')),
    -- declined: the §14 decline flag. A declined pair still writes its record,
    -- with no verdict, no guess and no artifacts — selective participation must
    -- be visible, not silent (§4.2.5).
    declined         INTEGER NOT NULL DEFAULT 0 CHECK (declined IN (0, 1)),
    -- epoch_id: assigned at RECORD time, because a pair's epoch is decided by
    -- the direct arm's observed model identity, which is only known once the
    -- direct arm has run (BENCH-REG §9).
    epoch_id         TEXT NOT NULL DEFAULT '',
    -- Both arms' EXACT OBSERVED model identities and measured consumption
    -- (§14). The direct arm's surface cannot be pinned, so its identity is
    -- recorded as observed, per pair (§6).
    platform_model   TEXT NOT NULL DEFAULT '',
    direct_model     TEXT NOT NULL DEFAULT '',
    platform_usd     REAL NOT NULL DEFAULT 0,
    direct_usd       REAL NOT NULL DEFAULT 0,
    -- direct_measured: whether the direct arm's consumption is a real reading.
    -- Only measured pairs feed the §13 done-directly median.
    direct_measured  INTEGER NOT NULL DEFAULT 0 CHECK (direct_measured IN (0, 1)),
    -- parity_note: a §6 arm-parity note — e.g. an ordinary platform ceiling
    -- that truncated the direct arm. A parity gap beyond §6's declaration is new
    -- registration material and is flagged, never silently absorbed.
    parity_note      TEXT NOT NULL DEFAULT '',
    recorded_ts      TEXT,
    updated_ts       TEXT NOT NULL
);

-- Accrual statistics scan (domain, epoch) over recorded pairs; the pending-card
-- projection scans (user_id, state); the sampler dedups by source event.
CREATE INDEX benchmark_pairs_domain_idx ON benchmark_pairs (domain, epoch_id, state);
CREATE INDEX benchmark_pairs_owner_idx ON benchmark_pairs (user_id, state);
CREATE UNIQUE INDEX benchmark_pairs_source_idx ON benchmark_pairs (source_seq)
    WHERE source_seq > 0;

-- Pair identity is immutable; only the §3 working state moves. BEFORE UPDATE OF
-- fires only when the SET clause names one of the listed columns, so an ordinary
-- state advance passes cleanly (the conformance_registry / watch_rows shape,
-- CONVENTIONS §32/§34).
CREATE TRIGGER benchmark_pairs_identity_immutable
    BEFORE UPDATE OF pair_id, user_id, domain, domain_source, task_id,
        deliverable_id, task_class, platform_run_id, phase, rate_pct,
        source_seq, sampled_ts
    ON benchmark_pairs
BEGIN
    SELECT RAISE(ABORT, 'benchmark pair identity is immutable; only the BENCH-REG §3 working state is mutable');
END;

-- A recorded pair is frozen: the §14 record has been written and the arms
-- revealed, and §17 forbids retroactive recomputation under a later
-- registration. Nothing may edit it afterwards.
CREATE TRIGGER benchmark_pairs_recorded_frozen
    BEFORE UPDATE ON benchmark_pairs
    WHEN OLD.state = 'recorded'
BEGIN
    SELECT RAISE(ABORT, 'a recorded benchmark pair is frozen; results are never recomputed under a later registration (BENCH-REG §17)');
END;

-- Benchmark records are keep-forever (BENCH-REG §14; Spec S14.9 keep-forever
-- set). A pair never leaves the store.
CREATE TRIGGER benchmark_pairs_no_delete
    BEFORE DELETE ON benchmark_pairs
BEGIN
    SELECT RAISE(ABORT, 'benchmark records are keep-forever; a pair is never deleted (BENCH-REG §14; Spec S14.9)');
END;

-- ── Per-domain practice bookkeeping (BENCH-REG §9, §11) ──────────────────────
--
-- This table is bookkeeping OF the practice, not the gate "setting" anything:
-- NO subsystem outside internal/benchmark reads it, and the S14.7 SET-nothing
-- clause holds — the gate evaluates and displays.
CREATE TABLE benchmark_domains (
    -- domain: the REGISTERED name (BENCH-REG §10).
    domain               TEXT PRIMARY KEY CHECK (domain <> ''),
    -- first_gate_opened_ts: the durable "has this domain's gate EVER opened"
    -- marker the §4.1 phase schedule keys on (pre-gate → maintenance). It is
    -- write-once: limb (d) depends on external mutable suite state, so
    -- retrospective re-derivation would be unsound.
    first_gate_opened_ts TEXT,
    -- The current epoch (BENCH-REG §9): a maximal interval during which the
    -- direct arm's observed model identity is unchanged.
    current_epoch_id     TEXT NOT NULL CHECK (current_epoch_id <> ''),
    epoch_started_ts     TEXT NOT NULL,
    -- epoch_identity: the direct-arm model identity this epoch is defined by.
    -- Empty until the domain's first pair is recorded.
    epoch_identity       TEXT NOT NULL DEFAULT '',
    epoch_seq            INTEGER NOT NULL DEFAULT 1 CHECK (epoch_seq >= 1),
    user_id              TEXT NOT NULL DEFAULT 'platform' CHECK (user_id = 'platform')
);

-- The first-open marker is write-once: once a domain's gate has opened, that
-- fact is history and the maintenance phase it selects never silently reverts.
CREATE TRIGGER benchmark_domains_first_open_write_once
    BEFORE UPDATE OF first_gate_opened_ts ON benchmark_domains
    WHEN OLD.first_gate_opened_ts IS NOT NULL
BEGIN
    SELECT RAISE(ABORT, 'the benchmark gate first-open marker is write-once (BENCH-REG §4.1 phase schedule)');
END;

CREATE TRIGGER benchmark_domains_no_delete
    BEFORE DELETE ON benchmark_domains
BEGIN
    SELECT RAISE(ABORT, 'a benchmark domain registration is never deleted (BENCH-REG §10)');
END;

-- ── The sampler cursor ───────────────────────────────────────────────────────
--
-- The sampling hook fires at verdict-eligibility, observed as deliverable.accepted
-- events on a shell-driven tail. The cursor is DURABLE rather than
-- bootstrapped at the head, because a restart must not silently skip eligible
-- tasks: sampling is uniform-random among ELIGIBLE tasks (frozen, BENCH-REG
-- §4.1) and a lost accept is a lost draw, not a neutral one. Exactly one row.
CREATE TABLE benchmark_sampler (
    row_id     TEXT PRIMARY KEY CHECK (row_id = 'sampler'),
    last_seq   INTEGER NOT NULL DEFAULT 0 CHECK (last_seq >= 0),
    updated_ts TEXT NOT NULL,
    user_id    TEXT NOT NULL DEFAULT 'platform' CHECK (user_id = 'platform')
);

INSERT INTO benchmark_sampler (row_id, last_seq, updated_ts)
    VALUES ('sampler', 0, '1970-01-01T00:00:00Z');

PRAGMA user_version = 14;

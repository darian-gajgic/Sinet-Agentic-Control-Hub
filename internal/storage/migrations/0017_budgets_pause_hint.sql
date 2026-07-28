-- 0017_budgets_pause_hint.sql — the S10.4 operator switches the B6-2B decision
-- plane persists, plus the S15.5 board-drag hint, plus the recreate the first of
-- those makes necessary.
--
-- Exact DDL per the S02.2 schema-workshop mandate; applied in ONE transaction
-- with its PRAGMA user_version bump by the migration runner
-- (internal/storage/migrate.go, Spec S02.1). A committed migration is immutable
-- (CONVENTIONS §6) — 0001–0016 stay byte-untouched, this file included once it
-- lands.
--
-- Four things land, each with its own reason (P3-B6-2B, OQ3 a–d):
--
--   (a) budgets            — the S10.4 operator-declared automation budget, per
--                            (person, lane). No existing home: `lanes` carries
--                            only concurrency_cap, and a ⚙ key is barred (the
--                            S18 tally is frozen at 118/33 and a (person, lane,
--                            period) map was never ratified). metering.Budget is
--                            a caller-supplied VALUE OBJECT whose persistence
--                            the package itself deferred to "a later phase";
--                            this is that phase.
--   (b) users.automation_paused — the S10.4 pause-my-automation switch. A
--                            per-person durable flag, not a ⚙ (same tally), on
--                            the 0014 `benchmark_opt_in` precedent: an
--                            ALTER TABLE users column with a 0 default, so no
--                            migration turns it on for anybody.
--   (c) queue.hint_rank    — the S15.5 drag hint. It is claim-loop working
--                            state exactly like `status` and `priority_lane`,
--                            so it belongs on the queue row the claim loop
--                            already reads.
--   (d) cost_budget_remainder recreated — 0016's view hardcodes
--                            budget_declared = 0 and an absence reason saying
--                            "no operator budget is persisted at v0", which (a)
--                            makes FALSE the moment somebody declares one. 0016
--                            itself is immutable, so the recreate rides here.
--
-- NO MONEY IS COMPUTED IN THIS FILE, exactly as in 0016 (S14.10 ¶1): the USD
-- columns stay NULL and the budget columns are read straight off the row.

-- ── (a) the S10.4 operator-declared automation budget ────────────────────────
--
-- The unit is the GAUGE'S OWN unit — weighted-consumption units, the same thing
-- metering.Gauge.WeightedConsumption accumulates (S10.4). It is NEVER dollars:
-- a flat-rate lane has no per-token price, and ranking or budgeting a flat lane
-- in currency is the D5 violation the whole metering layer is built to avoid.
--
-- The grain is (person, lane) because the gauge's grain is (person, lane): one
-- denominator per gauge reading, never an inferred provider window (D4).
--
-- period_start + period_days are the PERIOD DEFINITION: the gauge sums
-- consumption from period_start, and period_days records the length the
-- operator declared it for so the declaration says what it means. Nothing here
-- rolls a period over — a rollover would be a timer, and dueness comes from
-- stored state, never from a ticker (CONVENTIONS §34); re-declaring is the
-- operator act that starts the next period.
CREATE TABLE budgets (
    -- user_id: whose automation this budget bounds (15.6 owner attribution).
    user_id      TEXT    NOT NULL CHECK (user_id <> ''),
    -- lane: the lane the gauge reads against (Spec S02.2 runs.lane vocabulary).
    lane         TEXT    NOT NULL CHECK (lane <> ''),
    -- period_tokens: the budget itself, in weighted-consumption units. Zero is
    -- refused rather than stored: a zero budget is not a budget, it is a stop,
    -- and the switch for that is automation_paused below.
    period_tokens INTEGER NOT NULL CHECK (period_tokens > 0),
    -- period_start: the instant the period's consumption is summed from
    -- (RFC3339Nano UTC, the stored-timestamp convention of every other table).
    period_start TEXT    NOT NULL CHECK (period_start <> ''),
    -- period_days: the declared length of the period, in whole days.
    period_days  INTEGER NOT NULL CHECK (period_days > 0),
    -- declared_ts / declared_by: the audit face of the row. The full old→new
    -- record is the decision.recorded event (S14.2 family 5); these two make the
    -- row itself say who last set it and when, without a join.
    declared_ts  TEXT    NOT NULL CHECK (declared_ts <> ''),
    declared_by  TEXT    NOT NULL CHECK (declared_by <> ''),
    PRIMARY KEY (user_id, lane)
);

-- ── (b) the S10.4 pause-my-automation switch ─────────────────────────────────
--
-- "Pause my automation" stops AUTOMATION: the scheduler admits no background
-- work for a paused person and in-flight work parks at its next stage-session
-- boundary. Interactive work is never gated by it (S10.4 headroom rule) — that
-- is a property of the consulting code, not of this column, and it is asserted
-- there.
--
-- Default 0 and no backfill: pausing is a person's own act, and a migration
-- that paused anybody would be the platform deciding for them.
ALTER TABLE users ADD COLUMN automation_paused INTEGER NOT NULL DEFAULT 0
    CHECK (automation_paused IN (0, 1));

-- ── (c) the S15.5 board-drag priority hint ───────────────────────────────────
--
-- "The sanctioned v0 drag interaction is reordering one's own queued tasks as a
-- scheduler priority hint" (S15.5). It is a HINT: the claim loop reads it as a
-- tie-break WITHIN one (user, workload class) bucket, so it can never outrank
-- the S10.7 class ladder and can never reach across owners.
--
-- Lower sorts earlier; 0 is "no hint" and is where every existing and every new
-- queue row starts. The default is what makes this column additive: every
-- ordering decision the claim loop made before this migration it still makes.
ALTER TABLE queue ADD COLUMN hint_rank INTEGER NOT NULL DEFAULT 0;

-- ── (d) cost_budget_remainder, recreated so it stays TRUE ────────────────────
--
-- The 0016 view said "no operator budget is persisted at v0". With (a) that
-- sentence is false for anyone who has declared one, and a Layer-0 view that
-- states a false reason is worse than no view: the whole point of the honest
-- absence is that the absence is REAL (S10.1; the §35 honest-absence precedent).
--
-- The recreate is ADDITIVE — every 0016 column keeps its name and meaning, so
-- the Layer-1 canned query and any caller selecting those columns are unaffected
-- — and it is TRUE in BOTH states:
--
--   * undeclared → budget_declared 0, the budget columns NULL, and the absence
--     reason says a remainder has no minuend. Unchanged behavior.
--   * declared   → budget_declared 1 and the real declared budget, in its own
--     unit, from the row.
--
-- The USD columns stay NULL in BOTH states, and that is not a gap — it is D5.
-- A flat-rate lane's budget is denominated in weighted-consumption units; there
-- is no dollar budget to subtract a dollar spend from, so budget_usd and
-- remainder_usd have no honest value and remainder_status (which has always been
-- ABOUT the dollar remainder) stays UNAVAILABLE with the reason saying why.
--
-- The unit-denominated remainder is NOT computed here, deliberately. It is
-- period_tokens minus the gauge's weighted consumption, and that fold weights
-- cache reads by ⚙ pressure.cache_read_weight — a value a SQL view cannot read,
-- because the settings table stores OVERRIDE rows only and every default lives
-- in code (0001 settings, Spec S01.10). A view that folded it anyway would be a
-- SECOND implementation of the S10.4 gauge running on a missing weight: two
-- denominators where D4 and CONVENTIONS §11 allow exactly one. The gauge's own
-- reading is served per (person, lane) on /api/meters, and this view says so.
--
-- One naming care: the view's grain is per PERSON and a budget's grain is per
-- (person, lane), so the budget columns are a ROLL-UP and are named as one —
-- budget_lanes counts the declared lanes and budget_period_tokens_total sums
-- them. The total is a total, never a denominator: each lane's budget is its own
-- denominator and the gauge reads it that way (D4).
DROP VIEW cost_budget_remainder;

CREATE VIEW cost_budget_remainder AS
    SELECT u.user_id                                       AS user_id,
           CASE WHEN b.user_id IS NULL THEN 0 ELSE 1 END   AS budget_declared,
           NULL                                            AS budget_usd,
           NULL                                            AS remainder_usd,
           'UNAVAILABLE'                                   AS remainder_status,
           CASE WHEN b.user_id IS NULL
                THEN 'no operator budget is declared for this person, so a remainder has no minuend. This is a recorded absence, not a zero (S10.1).'
                ELSE 'a declared budget is denominated in weighted-consumption units, never dollars — a flat-rate lane has no dollar remainder (D5). The remainder in the budget''s own unit is the S10.4 gauge''s reading per (person, lane), served on /api/meters; it folds cache reads at the effective value of a settings key this view cannot read, so it is never recomputed here (one denominator, D4).'
           END                                             AS absence_reason,
           c.priced_usd                                    AS consumed_priced_usd,
           c.pricing_status                                AS pricing_status,
           -- The additive half: the declared budget itself, read from the row.
           b.lanes                                         AS budget_lanes,
           b.period_tokens                                 AS budget_period_tokens_total,
           CASE WHEN b.user_id IS NULL THEN NULL
                ELSE 'weighted-consumption units (S10.4) — never dollars (D5)'
           END                                             AS budget_unit,
           b.earliest_period_start                         AS budget_period_start,
           b.last_declared_ts                              AS budget_last_declared_ts
      -- Both sides matter: a person with receipts and no budget still gets the
      -- absence row (the 0016 contract), and a person who has declared a budget
      -- but not yet consumed anything still gets their declaration back.
      FROM (SELECT user_id FROM cost_per_person
            UNION
            SELECT user_id FROM budgets) u
      LEFT JOIN cost_per_person c ON c.user_id = u.user_id
      LEFT JOIN (SELECT user_id,
                        count(*)          AS lanes,
                        sum(period_tokens) AS period_tokens,
                        min(period_start)  AS earliest_period_start,
                        max(declared_ts)   AS last_declared_ts
                   FROM budgets
                  GROUP BY user_id) b ON b.user_id = u.user_id;

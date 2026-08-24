-- 0025_plan_budgets.sql — the S10.4 automation budget for a PLAN-metered lane
-- (P3-LN-6).
--
-- Exact DDL per the S02.2 schema-workshop mandate; applied in ONE transaction
-- with its PRAGMA user_version bump by the migration runner
-- (internal/storage/migrate.go, Spec S02.1). A committed migration is immutable
-- (CONVENTIONS §6) — 0001-0024 stay byte-untouched, this file included once it
-- lands.
--
-- WHY A SECOND TABLE RATHER THAN A WIDER `budgets` (0017). The token budget is
-- the tier-1 denominator and this is the tier-3 one, and four things differ,
-- none of them stylistic:
--
--   (1) THE UNIT. `budgets.period_tokens` is an INTEGER in weighted-consumption
--       units. A plan budget is in the WINDOW's own unit — credits, requests —
--       and is REAL, because a proposal is a fraction of an allowance.
--   (2) THE PERIOD SHAPE. `budgets.period_days INTEGER CHECK (period_days > 0)`
--       cannot express this plan's rolling FIVE HOUR window.
--   (3) THE KEY. A lane's windows can be denominated differently: the Kimi Code
--       plan counts REQUESTS over five hours and CREDITS over seven days. One
--       row per (person, lane) cannot hold both, and one nullable unit column
--       on a shared table is how somebody later adds a credit to a token.
--   (4) 0017 IS IMMUTABLE. Widening it is not available even if the three above
--       were not decisive.
--
-- The grain is the one S18.3 ratified for this surface — "Per-person automation
-- budgets: per (person, lane, period)" — with `window` naming the plan
-- document's own quota row. No vocabulary is minted here: `rolling-5h` and
-- `weekly` come from the documents in internal/metering/plandata/.
--
-- NOTHING HERE IS DENOMINATED IN MONEY (D5). A flat-rate lane has no per-unit
-- price, and budgeting one in currency is the inversion S10 exists to prevent.
-- The denominator is the OPERATOR's declared figure and never the provider's
-- published allowance, which is a SEED and rides the row as provenance only
-- (S10.4: "never an inferred provider window (D4)").
--
-- Nothing rolls a period over. A rollover would be a timer, and dueness comes
-- from stored state, never from a ticker (CONVENTIONS §34); re-declaring is the
-- operator act that starts the next period, exactly as 0017 records for tokens.
CREATE TABLE plan_budgets (
    -- user_id: whose automation this budget bounds (15.6 owner attribution).
    user_id      TEXT NOT NULL CHECK (user_id <> ''),
    -- lane: the lane the tier-3 reading is taken on (Spec S02.2 runs.lane).
    lane         TEXT NOT NULL CHECK (lane <> ''),
    -- window: the plan document's own quota name. It is part of the key because
    -- a lane's windows are separate allowances in possibly different units.
    window       TEXT NOT NULL CHECK (window <> ''),
    -- period_units: the budget itself, in the window's own unit. Zero is
    -- refused rather than stored: a zero budget is not a budget, it is a stop,
    -- and the switch for that is users.automation_paused (0017).
    period_units REAL NOT NULL CHECK (period_units > 0),
    -- unit: the window's own unit, stored so no surface re-derives it and none
    -- can render a credit budget under a "requests" label.
    unit         TEXT NOT NULL CHECK (unit <> ''),
    -- period_start: the instant this period's consumption is summed from
    -- (RFC3339Nano UTC, the stored-timestamp convention of every other table).
    period_start TEXT NOT NULL CHECK (period_start <> ''),
    -- period_hours: the declared length of the period. HOURS, not days: the
    -- shortest window this platform budgets against is five hours long.
    period_hours REAL NOT NULL CHECK (period_hours > 0),
    -- source: how the figure came to exist, in a CLOSED vocabulary. The two
    -- mean different things to a person reading their own budget — a seeded row
    -- is a proposal the platform derived and labeled "assumed", an operator-set
    -- row is a figure somebody chose — so a value outside the set is refused at
    -- write rather than stored and rendered as if it meant something.
    source       TEXT NOT NULL CHECK (source IN ('proposal-seeded', 'operator-set')),
    -- seeded_from: which allowance row a proposal was taken from. Empty on an
    -- operator-set row, and required on a seeded one — a proposal that cannot
    -- name what it was proposed from is a number with no provenance.
    seeded_from  TEXT NOT NULL CHECK (seeded_from <> '' OR source = 'operator-set'),
    -- fraction: what share of that allowance was taken (⚙
    -- budget.background_window_fraction at the time of seeding). Provenance,
    -- never an input to the arithmetic; 0 on an operator-set row.
    fraction     REAL NOT NULL CHECK (fraction >= 0 AND fraction <= 1),
    -- declared_ts / declared_by: the audit face of the row. The full old→new
    -- record is the decision.recorded event (S14.2 family 5); these two make the
    -- row itself say who last set it and when, without a join.
    declared_ts  TEXT NOT NULL CHECK (declared_ts <> ''),
    declared_by  TEXT NOT NULL CHECK (declared_by <> ''),
    PRIMARY KEY (user_id, lane, window)
);

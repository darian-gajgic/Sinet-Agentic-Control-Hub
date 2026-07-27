-- 0016_queryable_history.sql — the Spec S14.10 query surface: Layer 0's named
-- deterministic views (¶1), the routing-quality view (¶5), and the allowlist
-- Layer 2 generates against (¶3). DDL detail is P3's [S02.2].
--
-- What lands here:
--   A. The Layer-0 COST views — one named view per S2.5 cost question:
--      per run / task / project / person / period, budget remainder, burn
--      rate, limit-event history, done-directly.
--   B. The routing-quality view (S14.10 ¶5 / S2.6).
--
-- THE LOAD-BEARING RULE OF THIS FILE (S14.10 ¶1): "The assistant *selects*
-- views — it never computes money by generation." Every money figure below is
-- READ from a receipt that internal/metering already priced, per row, at the
-- row's own timestamp (S10.3). There is no token count multiplied by a rate
-- anywhere in this file, and internal/history asserts that as a checkable
-- negative over this text. Pricing stays in exactly one place.
--
-- HONEST ABSENCES, never a fabricated number (S10.1: "No row silently prices
-- to $0"; the §35 precedent). Three genuine absences are rendered as such:
--   * the v0 price table ships EMPTY, so a receipt with unpriced calls
--     renders pricing_status = 'UNPRICED' and a NULL amount — never 0.0;
--   * a run with no materialized receipt renders 'NO-RECEIPT';
--   * no operator budget is persisted anywhere at v0 (internal/metering
--     Budget is caller-supplied and no table holds one), so the budget
--     remainder view reports the absence with its reason instead of a figure.
--
-- Applied in one transaction with its PRAGMA user_version bump by the
-- migration runner (internal/storage/migrate.go, Spec S02.1). 0001–0015 stay
-- byte-untouched.

-- ── A. Layer 0: the named cost views (S14.10 ¶1) ─────────────────────────────
--
-- The substrate is the receipts table (0001): one row per finished run whose
-- usage_json IS the marshaled internal/metering Receipt — already priced,
-- already carrying its approximation tier, its unpriced-call count and the
-- registered done-directly line. Reading json_extract over that document is a
-- SELECT of a priced figure, which is exactly what S14.10 ¶1 permits.

-- cost_per_run — the per-run altitude, and the base every other cost view
-- aggregates. One row per run that has a receipt, plus the honest 'NO-RECEIPT'
-- row for a terminal run that never materialized one.
CREATE VIEW cost_per_run AS
    SELECT r.run_id                                                    AS run_id,
           r.user_id                                                   AS user_id,
           r.task_id                                                   AS task_id,
           r.lane                                                      AS lane,
           r.state                                                     AS state,
           r.created_ts                                                AS run_created_ts,
           rc.materialized_ts                                          AS receipt_ts,
           CASE
               WHEN rc.receipt_id IS NULL THEN 'NO-RECEIPT'
               WHEN COALESCE(json_extract(rc.usage_json, '$.total_calls'), 0) = 0 THEN 'NO-CALLS'
               WHEN COALESCE(json_extract(rc.usage_json, '$.total_unpriced_calls'), 0)
                    >= COALESCE(json_extract(rc.usage_json, '$.total_calls'), 0) THEN 'UNPRICED'
               WHEN COALESCE(json_extract(rc.usage_json, '$.total_unpriced_calls'), 0) > 0 THEN 'PARTIAL'
               ELSE 'PRICED'
           END                                                         AS pricing_status,
           -- The amount is NULL unless at least one call priced. A silent 0.0
           -- would be indistinguishable from genuinely-free work (S10.1).
           CASE
               WHEN rc.receipt_id IS NULL THEN NULL
               WHEN COALESCE(json_extract(rc.usage_json, '$.total_unpriced_calls'), 0)
                    >= COALESCE(json_extract(rc.usage_json, '$.total_calls'), 0) THEN NULL
               ELSE json_extract(rc.usage_json, '$.total_priced_usd')
           END                                                         AS priced_usd,
           COALESCE(json_extract(rc.usage_json, '$.total_calls'), 0)          AS total_calls,
           COALESCE(json_extract(rc.usage_json, '$.total_unpriced_calls'), 0) AS unpriced_calls,
           json_extract(rc.usage_json, '$.currency')                   AS currency,
           json_extract(rc.usage_json, '$.worst_tier')                 AS worst_tier
      FROM runs r
      LEFT JOIN receipts rc ON rc.run_id = r.run_id;

-- cost_per_task — the task altitude. A task's runs are its intake/compose/
-- execute/verify legs, so this is the figure a requester means by "what did
-- this cost". unpriced_runs is carried beside the total so a partially priced
-- task cannot read as a complete one.
CREATE VIEW cost_per_task AS
    SELECT t.task_id                                       AS task_id,
           t.user_id                                       AS user_id,
           t.title                                         AS title,
           t.kanban_status                                 AS kanban_status,
           count(c.run_id)                                 AS runs,
           sum(CASE WHEN c.pricing_status IN ('UNPRICED', 'NO-RECEIPT') THEN 1 ELSE 0 END) AS unpriced_runs,
           CASE WHEN count(c.priced_usd) = 0 THEN NULL ELSE sum(c.priced_usd) END          AS priced_usd,
           CASE WHEN count(c.priced_usd) = 0 THEN 'UNPRICED' ELSE 'PARTIAL-OR-PRICED' END  AS pricing_status,
           sum(c.total_calls)                              AS total_calls,
           sum(c.unpriced_calls)                           AS unpriced_calls
      FROM tasks t
      LEFT JOIN cost_per_run c ON c.task_id = t.task_id
     GROUP BY t.task_id, t.user_id, t.title, t.kanban_status;

-- cost_per_person — the person altitude (3.4: work is billed to the
-- requester, so the owner column IS the person).
CREATE VIEW cost_per_person AS
    SELECT c.user_id                                       AS user_id,
           count(*)                                        AS runs,
           sum(CASE WHEN c.pricing_status IN ('UNPRICED', 'NO-RECEIPT') THEN 1 ELSE 0 END) AS unpriced_runs,
           CASE WHEN count(c.priced_usd) = 0 THEN NULL ELSE sum(c.priced_usd) END          AS priced_usd,
           CASE WHEN count(c.priced_usd) = 0 THEN 'UNPRICED' ELSE 'PARTIAL-OR-PRICED' END  AS pricing_status,
           sum(c.total_calls)                              AS total_calls,
           sum(c.unpriced_calls)                           AS unpriced_calls,
           min(c.run_created_ts)                           AS first_run_ts,
           max(c.run_created_ts)                           AS last_run_ts
      FROM cost_per_run c
     GROUP BY c.user_id;

-- cost_per_period — the period altitude, by UTC day of the receipt's
-- materialization (the moment the cost became known). A caller aggregates the
-- day rows into weeks/months rather than the view guessing a calendar.
CREATE VIEW cost_per_period AS
    SELECT substr(rc.materialized_ts, 1, 10)               AS day,
           rc.user_id                                      AS user_id,
           count(*)                                        AS receipts,
           sum(CASE WHEN COALESCE(json_extract(rc.usage_json, '$.total_unpriced_calls'), 0) > 0
                    THEN 1 ELSE 0 END)                     AS receipts_with_unpriced,
           CASE WHEN sum(CASE WHEN COALESCE(json_extract(rc.usage_json, '$.total_unpriced_calls'), 0)
                                   >= COALESCE(json_extract(rc.usage_json, '$.total_calls'), 0)
                              THEN 0 ELSE 1 END) = 0
                THEN NULL
                ELSE sum(json_extract(rc.usage_json, '$.total_priced_usd'))
           END                                             AS priced_usd,
           CASE WHEN sum(CASE WHEN COALESCE(json_extract(rc.usage_json, '$.total_unpriced_calls'), 0)
                                   >= COALESCE(json_extract(rc.usage_json, '$.total_calls'), 0)
                              THEN 0 ELSE 1 END) = 0
                THEN 'UNPRICED' ELSE 'PARTIAL-OR-PRICED'
           END                                             AS pricing_status,
           sum(COALESCE(json_extract(rc.usage_json, '$.total_calls'), 0))          AS total_calls,
           sum(COALESCE(json_extract(rc.usage_json, '$.total_unpriced_calls'), 0)) AS unpriced_calls
      FROM receipts rc
     GROUP BY day, rc.user_id;

-- cost_burn_rate — the burn-rate question. A rate is a period figure divided
-- by its span, and the only span the substrate can defend is the observed one:
-- the days between the person's first and last receipt, inclusive. Days with
-- no activity are INSIDE that span and count against the rate, which is what
-- makes it a burn rate rather than a busy-day average. observed_days is
-- carried so a one-day sample cannot masquerade as a trend.
CREATE VIEW cost_burn_rate AS
    SELECT p.user_id                                       AS user_id,
           count(*)                                        AS active_days,
           CAST(julianday(max(p.day)) - julianday(min(p.day)) + 1 AS INTEGER) AS observed_days,
           min(p.day)                                      AS first_day,
           max(p.day)                                      AS last_day,
           CASE WHEN count(p.priced_usd) = 0 THEN NULL ELSE sum(p.priced_usd) END AS priced_usd,
           CASE WHEN count(p.priced_usd) = 0 THEN NULL
                ELSE sum(p.priced_usd) / (julianday(max(p.day)) - julianday(min(p.day)) + 1)
           END                                             AS usd_per_day,
           CASE WHEN count(p.priced_usd) = 0 THEN 'UNPRICED' ELSE 'PARTIAL-OR-PRICED' END AS pricing_status,
           -- The rate is a division of an already-priced total by a count of
           -- DAYS. No token is multiplied by any rate here (S14.10 ¶1).
           'usd per observed day, over the span between the first and last receipt' AS rate_basis
      FROM cost_per_period p
     GROUP BY p.user_id;

-- cost_budget_remainder — the HONEST ABSENCE. S14.10 ¶1 names "budget
-- remainders" among the cost questions, but no operator budget is persisted
-- anywhere at v0: internal/metering's Budget is a caller-supplied value object
-- (pressure.go) and the 13.4 budget surface is a later phase. A remainder
-- therefore has no minuend. The view reports the absence and its reason rather
-- than a fabricated figure or a silent zero (S10.1; the §35 honest-absence
-- precedent) — and it still emits one row per person, so a caller asking "what
-- is my remainder" gets an answer about the right subject.
CREATE VIEW cost_budget_remainder AS
    SELECT c.user_id                                       AS user_id,
           0                                               AS budget_declared,
           NULL                                            AS budget_usd,
           NULL                                            AS remainder_usd,
           'UNAVAILABLE'                                   AS remainder_status,
           'no operator budget is persisted at v0 — internal/metering Budget is caller-supplied (S10.4) and the 13.4 budget surface is a later phase, so a remainder has no minuend. This is a recorded absence, not a zero (S10.1).' AS absence_reason,
           c.priced_usd                                    AS consumed_priced_usd,
           c.pricing_status                                AS pricing_status
      FROM cost_per_person c;

-- cost_done_directly — the S10.10 done-directly line. The registered formula
-- lives in Spec/benchmark-preregistration-v1.md §13 and its numbers are
-- READ-ONLY (BENCH-REG §17). This view therefore CITES: every column below is
-- lifted verbatim from the receipt's own direct_use block, which
-- internal/metering built against the registration. No threshold, label or
-- number of §13 is restated in this file — internal/history asserts that as a
-- source scan.
CREATE VIEW cost_done_directly AS
    SELECT rc.run_id                                            AS run_id,
           rc.user_id                                           AS user_id,
           rc.materialized_ts                                   AS receipt_ts,
           json_extract(rc.usage_json, '$.direct_use.label')        AS label,
           json_extract(rc.usage_json, '$.direct_use.formula_ref')  AS formula_ref,
           json_extract(rc.usage_json, '$.direct_use.currency')     AS currency,
           CASE WHEN json_extract(rc.usage_json, '$.direct_use.unpriced') THEN 'UNPRICED' ELSE 'PRICED' END AS pricing_status,
           CASE WHEN json_extract(rc.usage_json, '$.direct_use.unpriced')
                THEN NULL ELSE json_extract(rc.usage_json, '$.direct_use.heuristic_usd')
           END                                                  AS heuristic_usd,
           json_extract(rc.usage_json, '$.direct_use.reason')            AS absence_reason,
           json_extract(rc.usage_json, '$.direct_use.measured_stage_seam') AS measured_stage_seam
      FROM receipts rc;

-- task_project — the honest task→project linkage, isolated in its own view so
-- the weakness is stated in ONE place.
--
-- RECORDED READING: neither `runs` nor `tasks` carries a project column, and
-- `deliverables.project_id` is written by nothing (both production call sites
-- omit it). The only POPULATED relational edge is `artifact_claims.project`,
-- written at intake from the resolved registry slice
-- (internal/intake/answer.go). It has no FK to repo_registry, it is '' when
-- triage matched no registry entry, and a task has several claim rows (one per
-- glob set). So: '' rows are excluded, the rows are collapsed to ONE per task,
-- and `project_choices` carries how many distinct projects were seen — a task
-- that somehow claimed two projects renders the ambiguity instead of hiding
-- it. Collapsing is not cosmetic: a fan-out would double-count money.
CREATE VIEW task_project AS
    SELECT ac.task_id                   AS task_id,
           min(ac.project)              AS project_id,
           count(DISTINCT ac.project)   AS project_choices
      FROM artifact_claims ac
     WHERE ac.project <> ''
     GROUP BY ac.task_id;

-- cost_per_project — the project altitude, over that linkage. Work with no
-- registry-resolved project is not dropped: it lands in the explicit
-- '(no project)' bucket, so the sum over this view still equals the sum over
-- cost_per_run and no spend goes missing from the answer.
CREATE VIEW cost_per_project AS
    SELECT COALESCE(tp.project_id, '(no project)')  AS project_id,
           COALESCE(rr.name, '')                    AS project_name,
           COALESCE(rr.state, '')                   AS project_state,
           c.user_id                                AS user_id,
           count(*)                                 AS runs,
           sum(CASE WHEN c.pricing_status IN ('UNPRICED', 'NO-RECEIPT') THEN 1 ELSE 0 END) AS unpriced_runs,
           CASE WHEN count(c.priced_usd) = 0 THEN NULL ELSE sum(c.priced_usd) END          AS priced_usd,
           CASE WHEN count(c.priced_usd) = 0 THEN 'UNPRICED' ELSE 'PARTIAL-OR-PRICED' END  AS pricing_status,
           sum(c.total_calls)                       AS total_calls,
           sum(c.unpriced_calls)                    AS unpriced_calls,
           max(COALESCE(tp.project_choices, 1))     AS project_choices,
           'via artifact_claims.project (intake-resolved); runs/tasks carry no project column at v0' AS linkage
      FROM cost_per_run c
      LEFT JOIN task_project tp ON tp.task_id = c.task_id
      LEFT JOIN repo_registry rr ON rr.project_id = tp.project_id
     GROUP BY COALESCE(tp.project_id, '(no project)'), c.user_id;

-- limit_event_history — the S14.10 ¶1 "limit-event history" cost question: the
-- provider limit signals a person actually hit, with their class, signal and
-- reset time (the S14.2 usage_limits family's RequiredFields). The canonical
-- type is `limit.event`; `engine.rate_limit` is its registered legacy alias
-- (§29 R14), so a pre-rename row is the same record and is included — a
-- history that silently began at a rename is not a history. internal/history
-- pins both literals to the eventlog registry by test, so a retype fails
-- loudly rather than emptying this view.
CREATE VIEW limit_event_history AS
    SELECT e.event_seq                                  AS event_seq,
           e.ts                                         AS ts,
           e.user_id                                    AS user_id,
           COALESCE(e.run_id, '')                       AS run_id,
           e.type                                       AS type,
           json_extract(e.payload, '$.class')           AS limit_class,
           json_extract(e.payload, '$.provider_signal') AS provider_signal,
           json_extract(e.payload, '$.resets_at')       AS resets_at,
           json_extract(e.payload, '$.lane')            AS lane
      FROM run_events e
     WHERE e.type IN ('limit.event', 'engine.rate_limit');

-- ── B. The routing-quality view (S14.10 ¶5, S2.6) ────────────────────────────
--
-- "A periodic view joins routing.decided causes to outcomes (rework rates,
-- verdicts, receipts) so routing itself is auditable over time."
--
-- RECORDED READING on "periodic": this is a VIEW, evaluated on read, so it is
-- strictly fresher than any periodically-refreshed rollup would be and needs
-- no scheduler job to stay true. The S14.10 ¶4 rollup tables exist for the
-- count surface (0015 run_event_rollup) where the aggregation is over the
-- whole log; routing decisions are bounded per run, so a view is the right
-- shape and a stale materialization would be strictly worse.
--
-- Every outcome column is a correlated read over the SAME run, served by
-- 0015's run_events_run_type_idx (run_id, type, event_seq). Causes come from
-- the routing.decided payload verbatim (internal/worker Decision) — never
-- re-derived here.
CREATE VIEW routing_quality AS
    SELECT e.event_seq                                          AS event_seq,
           e.ts                                                 AS decided_ts,
           e.run_id                                             AS run_id,
           e.user_id                                            AS user_id,
           r.task_id                                            AS task_id,
           json_extract(e.payload, '$.cause')                   AS cause,
           json_extract(e.payload, '$.score')                   AS score,
           COALESCE(json_extract(e.payload, '$.worker'), '')      AS worker_template_id,
           COALESCE(json_extract(e.payload, '$.worker_name'), '') AS worker_name,
           COALESCE(json_extract(e.payload, '$.version'), '')     AS worker_version,
           COALESCE(json_extract(e.payload, '$.generalist'), 0)   AS generalist,
           COALESCE(json_extract(e.payload, '$.degraded'), 0)     AS degraded,
           json_extract(e.payload, '$.model')                   AS model,
           json_extract(e.payload, '$.lane')                    AS lane,
           COALESCE(json_extract(e.payload, '$.effort'), '')      AS effort,
           json_extract(e.payload, '$.plain_reason')            AS plain_reason,
           -- The worker's domain, when routing named a worker. The S08.4
           -- version→outcome join key is (worker, version); the domain rides
           -- the template row.
           COALESCE(wt.domain, '')                              AS domain,
           r.state                                              AS run_final_state,
           r.lane                                               AS run_lane,
           -- Outcomes: verdicts. Both the canonical type and its registered
           -- legacy alias, for the same reason limit_event_history carries
           -- one (§29 R14).
           (SELECT count(*) FROM run_events v
             WHERE v.run_id = e.run_id AND v.type IN ('verdict.recorded', 'verify.round')) AS verdicts,
           (SELECT count(*) FROM run_events v
             WHERE v.run_id = e.run_id AND v.type IN ('verdict.recorded', 'verify.round')
               AND json_extract(v.payload, '$.verdict') IN ('SHIP', 'SHIP-with-notes'))    AS verdicts_ship,
           (SELECT count(*) FROM run_events v
             WHERE v.run_id = e.run_id AND v.type IN ('verdict.recorded', 'verify.round')
               AND json_extract(v.payload, '$.verdict') IN ('REVISE', 'ESCALATE', 'REOPEN-SPEC')) AS verdicts_adverse,
           -- Rework: the highest verify round the run reached. Round 1 is the
           -- first pass, so rounds beyond it ARE the rework — this is the
           -- numerator S14.10 ¶5's "rework rates" is a rate of, left as a
           -- count so the caller chooses the denominator it means.
           (SELECT COALESCE(max(json_extract(v.payload, '$.round')), 0) FROM run_events v
             WHERE v.run_id = e.run_id AND v.type IN ('verdict.recorded', 'verify.round'))  AS verify_rounds,
           CASE WHEN (SELECT COALESCE(max(json_extract(v.payload, '$.round')), 0) FROM run_events v
                       WHERE v.run_id = e.run_id AND v.type IN ('verdict.recorded', 'verify.round')) > 1
                THEN 1 ELSE 0 END                                                           AS reworked,
           -- Outcomes: the receipt, read through the Layer-0 view so the
           -- pricing honesty (UNPRICED vs a silent zero) is carried here too
           -- and is stated in exactly one place.
           c.pricing_status                                     AS pricing_status,
           c.priced_usd                                         AS priced_usd,
           c.total_calls                                        AS total_calls
      FROM run_events e
      JOIN runs r ON r.run_id = e.run_id
      LEFT JOIN worker_templates wt ON wt.template_id = json_extract(e.payload, '$.worker')
      LEFT JOIN cost_per_run c ON c.run_id = e.run_id
     WHERE e.type = 'routing.decided';

PRAGMA user_version = 16;

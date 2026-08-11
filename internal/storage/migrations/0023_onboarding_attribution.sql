-- 0023_onboarding_attribution.sql — the onboarding task reaches the task→project
-- edge: a third arm resolved by joining the registry (P3-RW-7; S13.7, S14.10,
-- S15.2/S15.5).
--
-- WHAT WAS WRONG. 0016 collapsed `artifact_claims` alone; 0022 completed it with
-- the intake-time registry pin. Both edges belong to the INTAKE pipeline, and
-- there is a task class that is not one. S13.7: "onboarding a repository is
-- itself a task the platform performs" — `stage.OnboardTaskID(p)` = `onboard-<p>`
-- (internal/stage/onboard.go), a task that never reaches plan approval (so it
-- writes no claims) and never runs triage (so it appends no `intake.state`
-- event, and there is no `$.registry.project` pin to read). The result was that
-- the ONE task whose id names its project read '(no project)' — on the board, in
-- the task-detail lineage, in the assistant's Layer-0/Layer-2 answers and in
-- `cost_per_project`. The gap was pure omission: this view predates the
-- onboarding task class.
--
-- WHAT THIS FILE DOES. `task_project` becomes COALESCE(claims-collapse,
-- pin-edge, onboard-edge) — the two landed arms are carried over VERBATIM and
-- keep their precedence, and the new arm answers only where both are NULL.
-- 0016's claims collapse stays the `cl` subquery for its own reason (a fan-out
-- would double-count money); the pin probe keeps its CROSS JOIN, which is
-- load-bearing (see 0022's note: it pins the join order so runs_task_idx and
-- run_events_run_type_idx both serve seeks). The weakness stays stated in ONE
-- place — the surfaces all read this view, so they cannot disagree.
--
-- THE ARM IS A JOIN, NOT ID SURGERY (the coordinator's OQ1 disposition,
-- 2026-08-11: option B arm (i)). `repo_registry.project_id` is a durable,
-- owner-attributed row that EXISTS BEFORE THE TASK DOES — project.Register runs
-- inside Store.Onboard, which StartOnboarding calls before it inserts the task
-- row — and that production never deletes (0008's repo_registry_no_delete
-- trigger; internal/project/gc.go is worktree-GC only). So the arm reads a real
-- relational record rather than carving a name out of a string: an `onboard-*`
-- id with no registry row behind it resolves to an honest NULL and stays in the
-- explicit '(no project)' bucket, rendered, never fabricated.
--
-- COST, MEASURED (2026-08-12, the pinned modernc.org/sqlite, no ANALYZE — the
-- plan production actually gets). The arm is a correlated scalar subquery whose
-- predicate (`t.task_id = 'onboard-' || rr.project_id`) is not sargable on the
-- registry side, so the registry is walked:
--
--     CORRELATED SCALAR SUBQUERY 6
--     SCAN rr USING COVERING INDEX sqlite_autoindex_repo_registry_1
--
-- — a covering walk of the primary-key index of a table holding one row per
-- project the operator has ever onboarded. COALESCE evaluates left to right, so
-- the arm is only reached for a task that neither claimed nor pinned a project.
-- What must NOT move is the pin probe's plan, and it does not: the same EXPLAIN
-- still carries `SEARCH r USING INDEX runs_task_idx (task_id=?)` and
-- `SEARCH e USING INDEX run_events_run_type_idx (run_id=? AND type=?)`. Both
-- legs stay pinned by FULL index name in internal/history's TestPinProbeSeeks
-- and are re-pinned under a registry-BEARING world by
-- TestOnboardArmDoesNotDisturbThePinProbePlan.
--
-- `project_choices` stays the claims' count where claims exist and is 1
-- otherwise, for 0022's reason extended one arm: a registry id, like a pin,
-- resolves exactly one project.
--
-- MONEY FOLLOWS ATTRIBUTION, unchanged in principle from 0022: `cost_per_project`
-- is re-created over the completed edge, so an onboarding ceremony's own spend
-- lands under the project it onboards. COALESCE still yields ONE row per task,
-- so 0016's sum-preservation invariant holds — the sum over this view still
-- equals the sum over `cost_per_run`, no spend goes missing and none
-- double-counts. Every figure is still READ from an already-priced receipt;
-- nothing here multiplies anything (S14.10 ¶1).
--
-- OWNER SCOPE IS UNCHANGED. `task_project` still carries NO owner column, so it
-- stays operator-only in the Layer-0 registry and at Layer 2, and the §38 D5
-- transport pre-check keeps answering 403 on that metadata bit (S01.9).
--
-- Applied in one transaction with its PRAGMA user_version bump by the migration
-- runner (internal/storage/migrate.go, Spec S02.1). 0001–0022 stay
-- byte-untouched.

-- cost_per_project depends on task_project, so it is dropped first and rebuilt
-- last. Both are re-created below; nothing else in the schema moves. No index is
-- added: the registry is scanned by design (see COST above).
DROP VIEW cost_per_project;
DROP VIEW task_project;

CREATE VIEW task_project AS
    SELECT task_id, project_id, project_choices
      FROM (
            SELECT t.task_id                        AS task_id,
                   COALESCE(
                       cl.project_id,
                       (SELECT json_extract(e.payload, '$.registry.project')
                          FROM runs r
                          CROSS JOIN run_events e ON e.run_id = r.run_id
                         WHERE r.task_id = t.task_id
                           AND e.type = 'intake.state'
                         ORDER BY e.event_seq DESC LIMIT 1),
                       (SELECT rr.project_id
                          FROM repo_registry rr
                         WHERE t.task_id = 'onboard-' || rr.project_id)
                   )                                AS project_id,
                   COALESCE(cl.project_choices, 1)  AS project_choices
              FROM tasks t
              -- 0016's claims collapse, carried over unchanged.
              LEFT JOIN (
                    SELECT ac.task_id                 AS task_id,
                           min(ac.project)            AS project_id,
                           count(DISTINCT ac.project) AS project_choices
                      FROM artifact_claims ac
                     WHERE ac.project <> ''
                     GROUP BY ac.task_id
                   ) cl ON cl.task_id = t.task_id
           )
     WHERE project_id IS NOT NULL AND project_id <> '';

-- cost_per_project — the project altitude, over the completed linkage. The shape
-- is 0016's, unchanged column for column; what moved is the edge underneath and
-- the `linkage` note, which must stay TRUE for whatever ships. Work with no
-- resolved project is still not dropped: it lands in the explicit '(no project)'
-- bucket, so the sum over this view still equals the sum over cost_per_run.
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
           'via artifact_claims.project at plan approval, else the task''s latest intake.state $.registry.project pin from triage, else the repo_registry entry an onboarding task''s own id names (onboard-<project_id>); runs/tasks carry no project column at v0' AS linkage
      FROM cost_per_run c
      LEFT JOIN task_project tp ON tp.task_id = c.task_id
      LEFT JOIN repo_registry rr ON rr.project_id = tp.project_id
     GROUP BY COALESCE(tp.project_id, '(no project)'), c.user_id;

PRAGMA user_version = 23;

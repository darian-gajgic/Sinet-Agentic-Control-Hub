-- 0022_pin_attribution.sql — pre-approval project attribution: the durable
-- intake-time registry pin reaches the task→project edge (P3-RW-3; S06.2,
-- S02.8, S14.10, S15.2/S15.5).
--
-- WHAT WAS WRONG. 0016's `task_project` collapsed `artifact_claims` alone, on
-- the RECORDED READING that claims are "the only POPULATED relational edge".
-- That reading was true when it was written and is still true — but it is a
-- statement about RELATIONAL columns, and there is a second edge that is not
-- one. The S06.2 registry match happens at TRIAGE: intake.Pipeline.Start
-- writes the task row, the intake run and the first `intake.state` event in
-- ONE transaction, and the matched slice rides that event's payload at
-- `$.registry.project`. The S02.8 claims carrying the same value are written
-- only at PLAN APPROVAL (internal/intake/answer.go, writeClaimsTx, whose
-- project IS `st.Registry.Project`). So a task that named its project at
-- intake read '(no project)' on the operator's board for its whole
-- pre-approval life — and the gap was purely TEMPORAL, never a disagreement:
-- pin and claim are the same value by construction and cannot conflict.
--
-- WHAT THIS FILE DOES. `task_project` becomes COALESCE(claims-collapse,
-- pin-edge) — claims first because an approved task's claims are the record of
-- what it actually declared, the pin as the completion for everything before
-- approval. 0016's claims collapse is carried over VERBATIM as the `cl`
-- subquery: '' rows excluded, one row per task, `project_choices` counted, for
-- 0016's own reason (a fan-out would double-count money). The weakness stays
-- stated in ONE place — an api-layer COALESCE would have put linkage logic in a
-- second place and healed the board while the assistant and every cost view
-- kept answering '(no project)' about the same task.
--
-- MONEY FOLLOWS ATTRIBUTION (the coordinator's OQ-(b) disposition, 2026-08-10).
-- `cost_per_project` is re-created over the completed edge, so a pinned task's
-- intake-ceremony spend lands under its project from birth — including intakes
-- that are never approved, which is exactly the spend "what did this project
-- cost" is asking about. 0016's sum-preservation invariant is untouched and
-- asserted: COALESCE yields ONE row per task, so the sum over this view still
-- equals the sum over `cost_per_run`, no spend goes missing and none
-- double-counts. Every figure here is still READ from an already-priced
-- receipt; nothing in this file multiplies anything (S14.10 ¶1).
--
-- OWNER SCOPE IS UNCHANGED. `task_project` still carries NO owner column, so it
-- stays operator-only in the Layer-0 registry and at Layer 2, and the §38 D5
-- transport pre-check keeps answering 403 on that metadata bit (S01.9).
--
-- Applied in one transaction with its PRAGMA user_version bump by the migration
-- runner (internal/storage/migrate.go, Spec S02.1). 0001–0021 stay
-- byte-untouched.

-- cost_per_project depends on task_project, so it is dropped first and rebuilt
-- last. Both are re-created below; nothing else in the schema moves.
DROP VIEW cost_per_project;
DROP VIEW task_project;

-- runs_task_idx — the task→runs hop the pin edge makes, on EXPLAIN evidence
-- rather than on expectation (S14.10 indexing bullet, §38 D3/D8).
--
-- MEASURED (2026-08-10, the pinned modernc.org/sqlite, no ANALYZE — nothing in
-- internal/storage runs one, so the planner works from its default heuristics
-- and this is the plan production actually gets). Without this index the
-- correlated pin probe drives from `run_events` through
-- run_events_type_seq_idx (type=?) and walks the whole intake.state history
-- descending, testing each row's run against the task — a seek, but one whose
-- cost grows with the event log rather than with the task. With it, and with
-- the join order pinned below, both legs seek: runs_task_idx (task_id=?) then
-- 0015's run_events_run_type_idx (run_id=? AND type=?). Pinned by full index
-- name in internal/history's TestPinProbeSeeks.
CREATE INDEX runs_task_idx ON runs (task_id);

-- task_project — the honest task→project linkage, still isolated in its own
-- view so the weakness is stated in ONE place.
--
-- The pin edge is the LATEST `intake.state` event for the task, by event_seq —
-- the same ordering authority and the same "latest state wins" reading as
-- intake.Pipeline.LoadState, because the State is marshaled whole onto every
-- intake.state event and the newest one is the record of what triage resolved.
-- `json_extract` yields NULL when no registry slice was matched, and the outer
-- filter drops the empty string too, so an unmatched task is EXCLUDED here and
-- lands in the explicit '(no project)' bucket at the surfaces that read this —
-- rendered, never dropped and never fabricated.
--
-- CROSS JOIN is load-bearing, not decoration: it pins the join order so `runs`
-- is the outer loop, which is what lets runs_task_idx serve the hop and
-- run_events_run_type_idx serve the event seek. A plain JOIN lets the planner
-- reverse them and walk the type index instead.
--
-- `project_choices` stays the claims' count where claims exist, and is 1 for a
-- pin — a pin resolves exactly one registry id. A task that somehow claimed two
-- projects still renders the ambiguity instead of hiding it.
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
                         ORDER BY e.event_seq DESC LIMIT 1)
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

-- cost_per_project — the project altitude, over the completed linkage. The
-- shape is 0016's, unchanged column for column; what moved is the edge
-- underneath and the `linkage` note, which must stay TRUE for whatever ships.
-- Work with no resolved project is still not dropped: it lands in the explicit
-- '(no project)' bucket, so the sum over this view still equals the sum over
-- cost_per_run.
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
           'via artifact_claims.project at plan approval, else the task''s latest intake.state $.registry.project pin from triage; runs/tasks carry no project column at v0' AS linkage
      FROM cost_per_run c
      LEFT JOIN task_project tp ON tp.task_id = c.task_id
      LEFT JOIN repo_registry rr ON rr.project_id = tp.project_id
     GROUP BY COALESCE(tp.project_id, '(no project)'), c.user_id;

PRAGMA user_version = 22;
